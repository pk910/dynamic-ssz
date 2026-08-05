// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
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
// into an input buffer from a bounded free list, a single-chunk (or
// per-element) hole is left in the hasher's buffer, and a goroutine reduces
// the copy level by level with wide hash calls. The reduction result lands
// at the front of the input buffer; the worker cannot write it into the
// hasher's buffer directly because appends may reallocate the backing array
// while the job runs — only the hole's offset is stable, not its address.
// drainJobs copies the results into the holes and runs before any operation
// that reads or restructures a region that may contain holes.
//
// All per-job state lives in a fixed FIFO ring of slots owned by the hasher,
// so steady-state hashing allocates nothing: slots and their done channels
// are reused, input buffers cycle through the free list, and jobs are
// spawned as plain method goroutines. A slot is occupied from enqueue until
// drain and the ring drains its oldest entry when full, so outstanding jobs
// are bounded by the ring size (twice the worker count). At most workers of
// them are incomplete at any moment (the semaphore is held for the duration
// of the reduction), which keeps the oldest entry of a full ring cheap to
// wait on.
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

	// asyncBufSize is the size of the pre-sized job input buffers in the
	// free list. Jobs wider than this (large progressive groups) use a
	// one-off allocation that is not retained.
	asyncBufSize = lazyFlushChunks * 32

	// asyncRingInline is the job-ring size served by the hasher's inline
	// backing array; larger rings (workers > 8) allocate once.
	asyncRingInline = 16
)

// asyncShared is the process-wide async hashing state. sem bounds concurrent
// reductions; a slot is held from before the job input is copied until the
// reduction finishes. bufs is a token free list with one token per job-ring
// slot (twice the worker count): a token is either a reusable input buffer
// or nil before the buffer is first materialized (or after an oversized
// one-off was dropped), so job input memory is strictly bounded.
type asyncShared struct {
	sem  chan struct{}
	bufs chan []byte
}

// asyncState points to the current shared state; nil means async hashing is
// disabled. In-flight jobs keep a reference to the state they started with,
// so reconfiguration never disturbs them.
var asyncState atomic.Pointer[asyncShared]

// EnableAsyncHashing enables background subtree reduction for all hashers
// whose hash function is safe for concurrent use (hashers created via
// NewHasherWithHashFn, including the FastHasherPool). At most workers
// reductions run concurrently, and job input memory is bounded by twice
// workers pre-sized buffers. workers <= 0 disables async hashing.
//
// Roots are unaffected: async and synchronous hashing produce identical
// results. Peak buffer usage grows by up to a few megabytes per hasher
// because deferred subtrees accumulate into wider runs before reduction.
func EnableAsyncHashing(workers int) {
	if workers <= 0 {
		DisableAsyncHashing()
		return
	}
	st := &asyncShared{
		sem:  make(chan struct{}, workers),
		bufs: make(chan []byte, 2*workers),
	}
	for range cap(st.bufs) {
		st.bufs <- nil
	}
	asyncState.Store(st)
}

// DisableAsyncHashing turns background subtree reduction off. Reductions
// already in flight complete independently; only new work is affected.
func DisableAsyncHashing() {
	asyncState.Store(nil)
}

// asyncShared returns the shared state when async hashing is enabled and
// this hasher's hash function may be called from other goroutines, nil
// otherwise.
func (h *Hasher) asyncShared() *asyncShared {
	if !h.asyncSafe {
		return nil
	}
	return asyncState.Load()
}

// asyncJob is one job-ring slot. done is allocated once and reused across
// jobs. in is the job's input buffer; after the done token arrived it holds
// the reduction result in its first outLen bytes. tokened records whether
// the job holds a free-list token to restore at drain.
type asyncJob struct {
	st      *asyncShared
	in      []byte
	done    chan struct{}
	dstOff  int
	outLen  int
	tokened bool
}

// enqueueReduce copies buf[start:start+width*32] into an input buffer and
// spawns a goroutine that reduces it pairwise, level by level, down to
// outChunks chunks. The claimed job-ring slot records where drainJobs must
// copy the result. width and outChunks must be powers of two with
// outChunks < width. Blocks while all worker slots are busy.
func (h *Hasher) enqueueReduce(st *asyncShared, start, width, outChunks, dstOff int) {
	st.sem <- struct{}{}

	need := width * 32
	in, tokened := h.getJobBuf(st, need)
	in = in[:need]
	copy(in, h.buf[start:start+need])

	slot := h.claimJobSlot(st)
	slot.st = st
	slot.in = in
	slot.dstOff = dstOff
	slot.outLen = outChunks * 32
	slot.tokened = tokened

	go h.runAsyncJob(slot, width, outChunks)
}

// getJobBuf obtains a job input buffer of at least need bytes. It never
// blocks on the free list — hashers share it, and blocking while every
// hasher's tokens sit in completed-but-undrained jobs would deadlock.
// Instead it drains this hasher's own oldest job to recover a token, and as
// a last resort (all tokens held by other hashers) takes an untracked
// one-off buffer. An undersized token (oversized job) is also swapped for a
// one-off, with the token restored as nil at drain.
func (h *Hasher) getJobBuf(st *asyncShared, need int) ([]byte, bool) {
	for {
		select {
		case in := <-st.bufs:
			if cap(in) >= need {
				return in, true
			}
			// Unmaterialized or undersized token: allocate the pre-sized
			// buffer so it joins the reuse cycle at drain. Oversized jobs
			// get a one-off instead, restored as a nil token at drain.
			return make([]byte, max(need, asyncBufSize)), true
		default:
		}
		if h.jobCount == 0 {
			return make([]byte, need), false
		}
		h.drainOldestJob()
	}
}

// claimJobSlot returns the next free ring slot, draining the oldest job
// first when the ring is full. The ring is sized to twice the worker count
// and only (re)sized while empty, so slot pointers held by running jobs
// stay valid.
func (h *Hasher) claimJobSlot(st *asyncShared) *asyncJob {
	if h.jobCount == 0 && len(h.jobRing) < cap(st.bufs) {
		if size := cap(st.bufs); size <= asyncRingInline {
			h.jobRing = h.jobRingBuf[:size]
		} else {
			h.jobRing = make([]asyncJob, size)
		}
	}
	for h.jobCount >= len(h.jobRing) {
		h.drainOldestJob()
	}
	slot := &h.jobRing[(h.jobHead+h.jobCount)%len(h.jobRing)]
	h.jobCount++
	if slot.done == nil {
		slot.done = make(chan struct{}, 1)
	}
	return slot
}

// runAsyncJob reduces the slot's input buffer level by level down to
// outChunks chunks, releases the worker slot, and signals completion. The
// result stays at the front of the input buffer for drainJobs to consume.
// Runs as its own goroutine.
func (h *Hasher) runAsyncJob(slot *asyncJob, width, outChunks int) {
	fn := h.hash
	in := slot.in
	for w := width; w > outChunks; w /= 2 {
		_ = fn(in[:w/2*32], in[:w*32])
	}
	<-slot.st.sem
	slot.done <- struct{}{}
}

// drainOldestJob waits for the ring's oldest job, writes its result into the
// hole it was carved from, and restores the job's free-list token if it
// holds one: pre-sized buffers are returned for reuse, oversized one-offs
// are replaced by a nil token. Tokenless buffers (taken when every token was
// held by other hashers) are simply dropped.
func (h *Hasher) drainOldestJob() {
	slot := &h.jobRing[h.jobHead]
	<-slot.done
	if end := slot.dstOff + slot.outLen; end <= len(h.buf) {
		copy(h.buf[slot.dstOff:end], slot.in[:slot.outLen])
	}
	if slot.tokened {
		if cap(slot.in) == asyncBufSize {
			slot.st.bufs <- slot.in[:0]
		} else {
			slot.st.bufs <- nil
		}
	}
	slot.st = nil
	slot.in = nil
	h.jobHead = (h.jobHead + 1) % len(h.jobRing)
	h.jobCount--
}

// drainJobs waits for all outstanding reductions and writes their results
// into the holes they were carved from. Must run before anything reads or
// restructures a buffer region that may contain holes.
func (h *Hasher) drainJobs() {
	for h.jobCount > 0 {
		h.drainOldestJob()
	}
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
func (h *Hasher) flushPendingAsyncRoot(st *asyncShared, layer *treeLayer, start, width, count int) {
	h.enqueueReduce(st, start, width, 1, start)
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
func (h *Hasher) flushPendingAsyncRoots(st *asyncShared, start, width, count int) {
	h.enqueueReduce(st, start, width, count, start)
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
