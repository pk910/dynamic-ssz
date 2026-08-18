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

	// asyncBufSize is the size of every job input buffer. Jobs are capped
	// at lazyFlushChunks, so all buffers are interchangeable and no other
	// size ever needs allocating: wider regions reduce as a sequence of
	// cap-sized jobs whose remainder carries over to the next round.
	asyncBufSize = lazyFlushChunks * 32

	// asyncCapDepth is the subtree depth a full cap-sized reduction
	// produces: a lazyFlushChunks-wide run of leaves becomes one node at
	// this depth.
	asyncCapDepth = 15

	// asyncRingInline is the job-ring size served by the hasher's inline
	// backing array; larger rings (workers > 8) allocate once.
	asyncRingInline = 16
)

// asyncShared is the process-wide async hashing state. sem bounds concurrent
// reductions; a slot is held from before the job input is copied until the
// reduction finishes. bufs is a token free list with one token per job-ring
// slot (twice the worker count): a token is either a reusable input buffer
// or nil before the buffer is first materialized, so job input memory is
// strictly bounded.
type asyncShared struct {
	sem  chan struct{}
	bufs chan []byte
}

// asyncState points to the current shared state; nil means async hashing is
// disabled. In-flight jobs keep a reference to the state they started with,
// so reconfiguration never disturbs them.
var asyncState atomic.Pointer[asyncShared]

// asyncRunners counts the persistent runner goroutines and asyncIdle the
// ones parked on the handoff channel. Runners start on demand: enabling
// spawns the first, and an enqueue that finds no idle runner adds one until
// the worker limit is reached. Runners are never stopped — the semaphore
// bounds how many are actually working, and the count stays at the highest
// demand ever seen (at most the highest worker limit configured).
var (
	asyncRunners atomic.Int64
	asyncIdle    atomic.Int64
)

// ensureAsyncRunner spawns a runner goroutine when none is idle and the
// worker limit of the given state allows another. The idle check is
// approximate: in the worst case a send waits for a busy runner to finish
// its current job, and the next enqueue tops the pool up.
func ensureAsyncRunner(st *asyncShared) {
	if asyncIdle.Load() > 0 {
		return
	}
	for {
		cur := asyncRunners.Load()
		if cur >= int64(cap(st.sem)) {
			return
		}
		if asyncRunners.CompareAndSwap(cur, cur+1) {
			go asyncJobRunner()
			return
		}
	}
}

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
	ensureAsyncRunner(st)
}

// DisableAsyncHashing turns background subtree reduction off. Reductions
// already in flight complete independently; only new work is affected.
func DisableAsyncHashing() {
	asyncState.Store(nil)
}

// asyncShared returns the shared state when async hashing is enabled
// process-wide and this hasher is gated in via SetAsyncHashing, nil
// otherwise.
func (h *Hasher) asyncShared() *asyncShared {
	if !h.async {
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
	fn      HashFn
	in      []byte
	done    chan struct{}
	dstOff  int
	outLen  int
	tokened bool
}

// asyncHandoff passes fully-prepared job slots to the persistent runner
// goroutines. It is never closed — reconfiguration just adjusts how many
// runners exist and how many may work at once, so senders never race a
// shutdown. A send parks at most until a runner finishes its current job:
// the semaphore admits no more jobs than there are runners.
var asyncHandoff = make(chan *asyncJob, 64)

// enqueueReduce copies buf[start:start+width*32] into an input buffer and
// spawns a runner goroutine that reduces it pairwise, level by level, down
// to outChunks chunks. The claimed job-ring slot records where drainJobs
// must copy the result. width and outChunks must be powers of two with
// outChunks < width. Blocks while all worker slots are busy.
func (h *Hasher) enqueueReduce(st *asyncShared, start, width, outChunks, dstOff int) {
	st.sem <- struct{}{}

	need := width * 32
	in, tokened := h.getJobBuf(st, need)
	in = in[:need]
	copy(in, h.buf[start:start+need])

	slot := h.claimJobSlot(st)
	slot.st = st
	slot.fn = h.hash
	slot.in = in
	slot.dstOff = dstOff
	slot.outLen = outChunks * 32
	slot.tokened = tokened

	if end := dstOff + outChunks*32; end > h.jobMaxEnd {
		h.jobMaxEnd = end
	}

	ensureAsyncRunner(st)
	asyncHandoff <- slot
}

// asyncJobRunner is a persistent worker goroutine: it reduces handed-off
// jobs level by level down to their output size, releases the worker slot,
// and signals completion. The result stays at the front of the input buffer
// for drainJobs to consume. Jobs are self-contained, so which runner picks
// which slot does not matter.
func asyncJobRunner() {
	for {
		asyncIdle.Add(1)
		slot := <-asyncHandoff
		asyncIdle.Add(-1)

		in := slot.in
		outChunks := slot.outLen / 32
		for w := len(in) / 32; w > outChunks; w /= 2 {
			_ = slot.fn(in[:w/2*32], in[:w*32])
		}
		<-slot.st.sem
		slot.done <- struct{}{}
	}
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
			// Unmaterialized token: allocate the pre-sized buffer so it
			// joins the reuse cycle at drain.
			return make([]byte, asyncBufSize), true
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

// finishOldestJob waits for the ring's oldest job, optionally copies its
// result into the hole it was carved from, and restores the job's free-list
// token if it holds one: pre-sized buffers are returned for reuse, anything
// else becomes a nil token. Tokenless buffers (taken when every token was
// held by other hashers) are simply dropped.
func (h *Hasher) finishOldestJob(copyResult bool) {
	slot := &h.jobRing[h.jobHead]
	<-slot.done
	if copyResult {
		// The destination is expected to exist: every path that shrinks the
		// buffer below a hole drains or discards first, so an out-of-range
		// copy is an invariant violation and panics via the bounds check
		// rather than silently producing a wrong root.
		end := slot.dstOff + slot.outLen
		copy(h.buf[slot.dstOff:end], slot.in[:slot.outLen])
	}
	if slot.tokened {
		// Tokened buffers always carry the pre-sized capacity: jobs are
		// capped at the buffer size and tokens materialize at exactly that
		// size, so the buffer itself is the token.
		slot.st.bufs <- slot.in[:0]
	}
	slot.st = nil
	slot.fn = nil
	slot.in = nil
	h.jobHead = (h.jobHead + 1) % len(h.jobRing)
	h.jobCount--
}

// drainOldestJob waits for the ring's oldest job and writes its result into
// the hole it was carved from.
func (h *Hasher) drainOldestJob() {
	h.finishOldestJob(true)
}

// drainJobs waits for all outstanding reductions and writes their results
// into the holes they were carved from. Must run before anything reads or
// restructures a buffer region that may contain holes.
func (h *Hasher) drainJobs() {
	for h.jobCount > 0 {
		h.drainOldestJob()
	}
	h.jobMaxEnd = 0
}

// discardJobs awaits outstanding reductions and drops their results. This is
// for abandoning a computation (Reset): the buffer regions the jobs were
// destined for no longer exist.
func (h *Hasher) discardJobs() {
	for h.jobCount > 0 {
		h.finishOldestJob(false)
	}
	h.jobMaxEnd = 0
}

// drainJobsFor drains outstanding reductions only when the region starting
// at indx can overlap a hole. Holes only exist below the highest outstanding
// hole end, so scopes that operate entirely above it — every child scope
// opened after its parents' flushes — skip the wait and keep the background
// reductions overlapped with the walk.
func (h *Hasher) drainJobsFor(indx int) {
	if h.jobCount > 0 && indx < h.jobMaxEnd {
		h.drainJobs()
	}
}

// asyncRootCompatible reports whether a completed subtree node of nodeDepth
// may be appended for the run starting at pendStart: every leaf before the
// run must already be represented by nodes at least that deep. That both
// keeps the buffer's deepest-first node order (which the depth-driven
// collapse pairing depends on) and makes the run leaf-aligned for its node.
// counts are only meaningful once the layer has collapsed — a reused layer
// slot carries stale counts until then — so an uncollapsed layer is only
// compatible while nothing precedes the run at all.
func (h *Hasher) asyncRootCompatible(layer *treeLayer, pendStart, nodeDepth int) bool {
	before := (pendStart - h.binaryRegionStart(layer)) / 32
	if !layer.collapsed {
		return before == 0
	}
	accounted := 0
	for d := 0; d <= layer.maxDepth; d++ {
		if d < nodeDepth && layer.counts[d] != 0 {
			return false
		}
		accounted += int(layer.counts[d])
	}
	return before == accounted
}

// flushPendingAsync reduces the layer's deferred run in cap-sized background
// jobs and leaves any sub-cap remainder pending for the next round, so every
// job matches the pre-sized input buffers exactly. A binary layer gets one
// completed subtree node per job, recorded at depth log2(elements-per-cap) —
// the layer's leaves are element roots, so the intra-element levels do not
// count. A progressive layer gets one root per element instead: group
// boundaries are not aligned to run boundaries, so completed subtrees cannot
// span them. Reports whether at least one job was emitted; if not (a binary
// run whose node would not be compatible with what precedes it), the caller
// reduces synchronously.
func (h *Hasher) flushPendingAsync(st *asyncShared, layer *treeLayer) bool {
	elem := layer.pendElemChunks
	batchElems := lazyFlushChunks / elem
	nodeDepth := 0
	for c := batchElems; c > 1; c >>= 1 {
		nodeDepth++
	}
	emitted := false
	for layer.pendCount >= batchElems {
		start := layer.pendStart
		if layer.progressive {
			h.enqueueReduce(st, start, lazyFlushChunks, batchElems, start)
			h.compactAsyncRun(start, lazyFlushChunks, batchElems)
			layer.pendStart = start + batchElems*32
		} else {
			if !h.asyncRootCompatible(layer, start, nodeDepth) {
				break
			}
			h.enqueueReduce(st, start, lazyFlushChunks, 1, start)
			h.compactAsyncRun(start, lazyFlushChunks, 1)
			if !layer.collapsed {
				layer.collapsed = true
				layer.counts = [maxTreeDepth]uint32{}
				layer.maxDepth = 0
			}
			layer.counts[nodeDepth]++
			if nodeDepth > layer.maxDepth {
				layer.maxDepth = nodeDepth
			}
			layer.pendStart = start + 32
		}
		layer.pendCount -= batchElems
		emitted = true
	}
	return emitted
}

// precollapseProgressiveRun replaces cap-aligned spans of a progressive
// layer's trailing depth-0 run with cap-depth nodes reduced in the
// background, so groups wider than one job buffer are later consumed from a
// handful of nodes instead of one oversized reduction. Progressive callers
// may only use this from level 8 up: every group in the active region is
// then a multiple of the cap, so cap-aligned nodes never straddle a group
// boundary. Binary layers (raw packed lists and vectors) have no group
// boundaries and use it at any size. Reports whether jobs were emitted —
// the caller must then leave the region untouched for the round, because
// the node holes are only stable while nothing consumes or compacts around
// them.
func (h *Hasher) precollapseCapRun(st *asyncShared, layer *treeLayer) bool {
	// Account chunks appended since the last state sync. A layer that never
	// collapsed carries stale counts from a previous scope in its reused
	// slot, so its state is rebuilt from the buffer instead.
	if layer.collapsed {
		h.syncCollapseState(layer)
	} else {
		layer.counts = [maxTreeDepth]uint32{}
		layer.maxDepth = 0
		if chunks := (len(h.buf) - h.binaryRegionStart(layer)) / 32; chunks > 0 {
			layer.counts[0] = uint32(chunks)
		}
	}

	// Everything deeper than the run must itself be a cap-depth node: that
	// guarantees the run starts cap-aligned, and it keeps the buffer's
	// deepest-first node order intact when the new nodes are recorded at
	// cap depth (nodes at depths in between would sit left of the new ones
	// positionally but be consumed after them).
	for d := 1; d <= layer.maxDepth; d++ {
		if d != asyncCapDepth && layer.counts[d] != 0 {
			return false
		}
	}
	emitted := false
	runStart := h.binaryRegionStart(layer) + int(layer.counts[asyncCapDepth])*32
	for layer.counts[0] >= lazyFlushChunks {
		h.enqueueReduce(st, runStart, lazyFlushChunks, 1, runStart)
		h.compactAsyncRun(runStart, lazyFlushChunks, 1)
		layer.counts[0] -= lazyFlushChunks
		layer.counts[asyncCapDepth]++
		if asyncCapDepth > layer.maxDepth {
			layer.maxDepth = asyncCapDepth
		}
		layer.collapsed = true
		runStart += 32
		emitted = true
	}
	return emitted
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
