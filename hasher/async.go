// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"sync"
	"sync/atomic"
)

// Async hashing offloads large, self-contained subtree reductions to
// background goroutines while the main goroutine keeps walking the value and
// appending chunks. It is disabled by default and enabled process-wide via
// EnableAsyncHashing.
//
// The unit of background work is a contiguous, power-of-two-chunk region of
// the buffer whose reduction result has a known size: either a complete
// aligned subtree (reduced to a single root) or a run of deferred element
// subtrees (reduced to one root per element). The region's bytes are copied
// into a pooled job buffer, a single-chunk (or per-element) hole is left in
// the hasher's buffer, and a goroutine reduces the copy level by level with
// wide hash calls. drainJobs fills the holes with the results and runs before
// any operation that reads or restructures a region that may contain holes.
//
// Because each job is reduced sequentially inside one goroutine, there are no
// per-level synchronization barriers; the only waits are the semaphore
// acquisition when all workers are busy (which bounds live job memory) and
// the drains at finalization points.
const (
	// asyncMinChunks is the minimum region width (in 32-byte chunks) worth
	// handing to a background reduction; smaller regions reduce inline.
	asyncMinChunks = 8192

	// lazyFlushChunks is the pending-run width at which Collapse flushes
	// deferred subtrees when async hashing is enabled. Runs this wide reduce
	// in a single background job. With async hashing disabled, Collapse
	// flushes on every call and the pending run stays at the cadence the
	// caller collapses with.
	lazyFlushChunks = 32768

	// asyncActiveChunks is the active-region width at which Collapse runs
	// progressive group finalization when async hashing is enabled.
	// Deferring finalization gives element-root jobs time to complete in the
	// background, so the drain at finalization rarely waits.
	asyncActiveChunks = 131072
)

// asyncSem limits concurrent background reductions. A nil pointer means async
// hashing is disabled. Slots are acquired before a job's input is copied, so
// at most cap(sem) job input buffers are live at any moment.
var asyncSem atomic.Pointer[chan struct{}]

// EnableAsyncHashing enables background subtree reduction for all hashers
// whose hash function is safe for concurrent use (hashers created via
// NewHasherWithHashFn, including the FastHasherPool). At most workers
// reductions run concurrently. workers <= 0 disables async hashing.
//
// Roots are unaffected: async and synchronous hashing produce identical
// results. Peak buffer usage grows by up to a few megabytes per hasher
// because deferred subtrees accumulate into wider runs before reduction.
func EnableAsyncHashing(workers int) {
	if workers <= 0 {
		DisableAsyncHashing()
		return
	}
	sem := make(chan struct{}, workers)
	asyncSem.Store(&sem)
}

// DisableAsyncHashing turns background subtree reduction off. Reductions
// already in flight complete independently; only new work is affected.
func DisableAsyncHashing() {
	asyncSem.Store(nil)
}

// asyncSlots returns the semaphore when async hashing is enabled and this
// hasher's hash function may be called from other goroutines, nil otherwise.
func (h *Hasher) asyncSlots() chan struct{} {
	if !h.asyncSafe {
		return nil
	}
	if sem := asyncSem.Load(); sem != nil {
		return *sem
	}
	return nil
}

// hashJob is one background reduction. out is written before done closes.
type hashJob struct {
	out  []byte
	done chan struct{}
}

// pendingJob tracks an in-flight job and the buffer offset its result
// belongs at. The offset is stable while the job is outstanding: every path
// that moves bytes at or below a hole drains first.
type pendingJob struct {
	job    *hashJob
	dstOff int
}

// jobBufPool holds job input buffers. Buffers are returned by the worker
// goroutine as soon as the reduction result is copied out, so pool residency
// is bounded by the semaphore, not by drain timing.
var jobBufPool = sync.Pool{New: func() any {
	b := make([]byte, 0, lazyFlushChunks*32)
	return &b
}}

// enqueueReduce copies buf[start:start+width*32] into a job buffer and
// spawns a goroutine that reduces it pairwise, level by level, down to
// outChunks chunks. The result is recorded for drainJobs to copy to
// buf[dstOff:]. width and outChunks must be powers of two with
// outChunks < width. Blocks while all worker slots are busy.
func (h *Hasher) enqueueReduce(slots chan struct{}, start, width, outChunks, dstOff int) {
	slots <- struct{}{}

	bp, _ := jobBufPool.Get().(*[]byte)
	if cap(*bp) < width*32 {
		*bp = make([]byte, 0, width*32)
	}
	in := (*bp)[:width*32]
	copy(in, h.buf[start:start+width*32])

	job := &hashJob{
		out:  make([]byte, outChunks*32),
		done: make(chan struct{}),
	}
	h.pendingJobs = append(h.pendingJobs, pendingJob{job: job, dstOff: dstOff})

	fn := h.hash
	go func() {
		for w := width; w > outChunks; w /= 2 {
			_ = fn(in[:w/2*32], in[:w*32])
		}
		copy(job.out, in[:outChunks*32])
		jobBufPool.Put(bp)
		<-slots
		close(job.done)
	}()
}

// drainJobs waits for all outstanding reductions and writes their results
// into the holes they were carved from. Must run before anything reads or
// restructures a buffer region that may contain holes.
func (h *Hasher) drainJobs() {
	if len(h.pendingJobs) == 0 {
		return
	}
	for i := range h.pendingJobs {
		pj := &h.pendingJobs[i]
		<-pj.job.done
		if end := pj.dstOff + len(pj.job.out); end <= len(h.buf) {
			copy(h.buf[pj.dstOff:end], pj.job.out)
		}
		h.pendingJobs[i] = pendingJob{}
	}
	h.pendingJobs = h.pendingJobs[:0]
}

// layerLeaves returns the number of leaves the layer has accumulated before
// the current pending run: nodes tracked in counts contribute 2^depth leaves
// each, and chunks not yet accounted (appended since the last state sync,
// excluding the pending run itself) contribute one leaf each.
func (h *Hasher) layerLeaves(layer *treeLayer, pendStart int) uint64 {
	var leaves, accounted uint64
	for d := 0; d <= layer.maxDepth; d++ {
		leaves += uint64(layer.counts[d]) << uint(d)
		accounted += uint64(layer.counts[d])
	}
	if chunks := uint64(pendStart-h.binaryRegionStart(layer)) / 32; chunks > accounted {
		leaves += chunks - accounted
	}
	return leaves
}

// flushPendingAsyncRoot hands a complete power-of-two run of deferred
// subtrees to a background job that reduces it to a single subtree root. The
// run must be leaf-aligned in the layer's tree (the caller checks). A
// one-chunk hole replaces the run and the completed subtree is recorded in
// the layer's collapse state at depth log2(count) — the layer's leaves are
// element roots, so the intra-element levels do not count.
func (h *Hasher) flushPendingAsyncRoot(slots chan struct{}, layer *treeLayer, start, width, count int) {
	h.enqueueReduce(slots, start, width, 1, start)
	h.compactAsyncRun(start, width, 1)

	d := 0
	for c := count; c > 1; c >>= 1 {
		d++
	}
	if !layer.collapsed {
		layer.collapsed = true
		layer.counts = [maxTreeDepth]uint32{}
		layer.maxDepth = 0
	}
	layer.counts[d]++
	if d > layer.maxDepth {
		layer.maxDepth = d
	}
}

// flushPendingAsyncRoots hands a run of deferred subtrees to a background
// job that reduces it to one root per element. A count-chunk hole replaces
// the run; the roots are plain depth-0 leaves, so the collapse state needs
// no update (the next state sync accounts them).
func (h *Hasher) flushPendingAsyncRoots(slots chan struct{}, start, width, count int) {
	h.enqueueReduce(slots, start, width, count, start)
	h.compactAsyncRun(start, width, count)
}

// compactAsyncRun shrinks a width-chunk run at start down to outChunks hole
// chunks, moving any bytes appended after the run down to stay contiguous.
func (h *Hasher) compactAsyncRun(start, width, outChunks int) {
	runEnd := start + width*32
	holeEnd := start + outChunks*32
	if tail := len(h.buf) - runEnd; tail > 0 {
		copy(h.buf[holeEnd:], h.buf[runEnd:])
		h.buf = h.buf[:holeEnd+tail]
	} else {
		h.buf = h.buf[:holeEnd]
	}
}
