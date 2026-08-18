// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"math/bits"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ sszutils.HashWalker = (*proofCapture)(nil)

// proofCapture is a HashWalker that computes a value's hash tree root through
// an embedded hasher while capturing just enough of the tree to serve proofs
// for a schedule of generalized indices. Off-path subtrees pass straight
// through the hasher's flat, batched merkleization and never materialize
// nodes. Scheduled scopes run on the hasher's non-incremental path, so their
// completed children appear as a plain chunk sequence in the hasher's
// buffer; the capture consumes that sequence in blocks as it grows —
// off-path runs reduce to aligned subtree roots in batched level order,
// scheduled slots keep their structure — and hands the scope's root back to
// the hasher as a single chunk at close, so the buffer never holds more
// than one block per scheduled scope. Subtrees whose layout depends on
// runtime data (retainAll in the schedule) are captured whole through a
// tree wrapper fed in parallel. Async hashing is supported: the buffer
// reads drain any background reduction overlapping the region.
type proofCapture struct {
	*hasher.Hasher

	scopes []proofCaptureScope

	// mute counts scope nesting inside an off-path subtree: the stream is
	// forwarded untouched, and the subtree's root lands in the enclosing
	// scope's buffer region by itself.
	mute int

	// retain captures a retainAll subtree through a tree wrapper fed in
	// parallel with the hasher. retainIdx maps the wrapper's scope indices
	// to the nesting, retainOrdinal is the subtree's child ordinal in the
	// enclosing scheduled scope.
	retain        *treeBuilder
	retainSched   *ProofSchedule
	retainIdx     []int
	retainOrdinal uint64

	hashFn       hasher.HashFn
	finalizeOpts []FinalizeOption
	scratch      []byte

	// err records a capture failure detected mid-stream (a scheduled scope
	// closing with a shape the schedule cannot describe); ProofTree reports
	// it instead of serving a wrong proof.
	err error
}

// proofCaptureScope is one open scheduled scope: its start position in the
// hasher's buffer, the number of child chunks already consumed out of that
// region, its child ordinal in the enclosing scope, the schedule that
// applies to its children, the pruned nodes of children that were
// themselves scheduled (consumed by the sweep), and the completed-subtree
// entries accumulated so far.
type proofCaptureScope struct {
	sched      *ProofSchedule
	start      int
	drained    uint64
	ordinal    uint64
	childNodes map[uint64]*Node
	entries    []captureEntry
}

// captureEntry is one completed, position-aligned subtree of a scheduled
// scope: the chunk slot it starts at, its depth (0 = single chunk), its
// root value, and — when the subtree lies on a proof path — its retained
// node structure. Adjacent equal-depth aligned entries merge as they are
// pushed, so the entry list stays logarithmic between scheduled slots.
type captureEntry struct {
	slot     uint64
	depth    int
	retained bool
	node     *Node
	value    [32]byte
}

// newProofCapture creates a HashWalker that computes the hash tree root on
// the given hasher while capturing a pruned proof tree for the given
// retention schedule. The hasher must be freshly reset. hashFn is used for
// the capture's own sibling-range reductions and must be safe for
// concurrent use; nil selects the fast hashing backend.
func newProofCapture(sched *ProofSchedule, hh *hasher.Hasher, hashFn hasher.HashFn, workers int) *proofCapture {
	// The base scope hands the root schedule to the value's own top-level
	// scope, which opens as its first child.
	base := &ProofSchedule{children: map[uint64]*ProofSchedule{0: sched}}
	pc := &proofCapture{
		Hasher: hh,
		hashFn: hashFn,
		scopes: []proofCaptureScope{{sched: base}},
	}
	// Rebuilt subtrees (Put-style replays, retained shapes) finalize with
	// the same backend and worker configuration as the rest of the capture.
	if hashFn != nil {
		pc.finalizeOpts = append(pc.finalizeOpts, WithHashFn(hashFn))
	}
	if workers > 1 {
		pc.finalizeOpts = append(pc.finalizeOpts, WithAsyncHashing(workers))
	}
	return pc
}

// Node returns the pruned proof tree after the walk completed, satisfying
// ProofHashWalker; a capture that diverged from the hash tree root panics.
func (pc *proofCapture) Node() *Node {
	tree, err := pc.ProofTree()
	if err != nil {
		panic(err.Error())
	}
	return tree
}

// ProofTree returns the pruned proof tree after the walk completed. A
// stream without a captured top-level subtree (a bare basic value) serves
// its root as a single leaf. Capture failures detected mid-stream are
// reported here, and the tree's root is verified against the hasher's root.
// Scheduled scopes hand their folded roots to the hasher stream, so for
// them the fold itself is the root computation; the parity between fold and
// hasher reduction is pinned by the package's stream tests.
func (pc *proofCapture) ProofTree() (*Node, error) {
	if pc.err != nil {
		return nil, pc.err
	}
	root, err := pc.HashRoot()
	if err != nil {
		return nil, err
	}

	tree := pc.scopes[0].childNodes[0]
	if tree == nil {
		tree = newOwnedLeaf(bytes.Clone(root[:]))
	}

	if !bytes.Equal(tree.value, root[:]) {
		return nil, sszutils.NewSszError(sszutils.ErrInvalidValueRange, "proof tree root diverges from hash tree root")
	}

	return tree, nil
}

// maybeSweep gives the current scheduled scope a chance to consume a full
// block of accumulated chunks. Called from the chunk-producing stream
// methods, so packed scopes (raw appends without child scopes) stay bounded
// too; the sweep itself early-exits until a block completed.
func (pc *proofCapture) maybeSweep() {
	if scope := pc.scheduledScope(); scope != nil {
		pc.sweep(scope, false)
	}
}

// scheduledScope returns the scheduled scope receiving current children,
// nil while the stream is inside a muted or retained subtree.
func (pc *proofCapture) scheduledScope() *proofCaptureScope {
	if pc.mute > 0 || pc.retain != nil {
		return nil
	}
	return &pc.scopes[len(pc.scopes)-1]
}

// childOrdinal returns the ordinal the next child takes in the given scope:
// the chunks already consumed by the sweep plus the completed chunks its
// children currently occupy in the hasher's buffer.
func (pc *proofCapture) childOrdinal(scope *proofCaptureScope) uint64 {
	return scope.drained + uint64(pc.CurrentIndex()-scope.start)/32
}

// --- retained-subtree stream mirroring ---

// appendSplit forwards a bulk payload to the hasher in block-bounded
// pieces, sweeping between them, and returns the remainder. The scope's
// current chunk is topped up first so the region reaches a sweepable
// alignment — without that, a partial leading chunk would leave every sweep
// misaligned and the region unbounded.
func (pc *proofCapture) appendSplit(scope *proofCaptureScope, i []byte) []byte {
	if rem := (pc.CurrentIndex() - scope.start) % 32; rem != 0 {
		// The payload exceeds a block, so it always covers the topping-up.
		pc.Hasher.Append(i[:32-rem])
		i = i[32-rem:]
	}
	for len(i) > captureBlockChunks*32 {
		pc.sweep(scope, false)
		pc.Hasher.Append(i[:captureBlockChunks*32])
		i = i[captureBlockChunks*32:]
	}
	pc.sweep(scope, false)
	return i
}

func (pc *proofCapture) Append(i []byte) {
	// Bulk appends into a scheduled scope land in block-sized pieces so the
	// sweeps between them keep the hasher's buffer bounded. Inside muted and
	// retained subtrees the payload passes through whole; the hasher's own
	// incremental machinery covers those.
	if scope := pc.scheduledScope(); scope != nil && len(i) > captureBlockChunks*32 {
		i = pc.appendSplit(scope, i)
	}
	pc.maybeSweep()
	pc.Hasher.Append(i)
	if pc.retain != nil {
		pc.retain.Append(i)
	}
}

func (pc *proofCapture) AppendBool(b bool) {
	pc.maybeSweep()
	pc.Hasher.AppendBool(b)
	if pc.retain != nil {
		pc.retain.AppendBool(b)
	}
}

func (pc *proofCapture) AppendUint8(i uint8) {
	pc.maybeSweep()
	pc.Hasher.AppendUint8(i)
	if pc.retain != nil {
		pc.retain.AppendUint8(i)
	}
}

func (pc *proofCapture) AppendUint16(i uint16) {
	pc.maybeSweep()
	pc.Hasher.AppendUint16(i)
	if pc.retain != nil {
		pc.retain.AppendUint16(i)
	}
}

func (pc *proofCapture) AppendUint32(i uint32) {
	pc.maybeSweep()
	pc.Hasher.AppendUint32(i)
	if pc.retain != nil {
		pc.retain.AppendUint32(i)
	}
}

func (pc *proofCapture) AppendUint64(i uint64) {
	pc.maybeSweep()
	pc.Hasher.AppendUint64(i)
	if pc.retain != nil {
		pc.retain.AppendUint64(i)
	}
}

func (pc *proofCapture) AppendBytes32(b []byte) {
	if scope := pc.scheduledScope(); scope != nil && len(b) > captureBlockChunks*32 {
		b = pc.appendSplit(scope, b)
	}
	pc.maybeSweep()
	pc.Hasher.AppendBytes32(b)
	if pc.retain != nil {
		pc.retain.AppendBytes32(b)
	}
}

func (pc *proofCapture) FillUpTo32() {
	pc.maybeSweep()
	pc.Hasher.FillUpTo32()
	if pc.retain != nil {
		pc.retain.FillUpTo32()
	}
}

func (pc *proofCapture) PutBool(b bool) {
	pc.Hasher.PutBool(b)
	if pc.retain != nil {
		pc.retain.PutBool(b)
	}
}

func (pc *proofCapture) PutUint8(i uint8) {
	pc.Hasher.PutUint8(i)
	if pc.retain != nil {
		pc.retain.PutUint8(i)
	}
}

func (pc *proofCapture) PutUint16(i uint16) {
	pc.Hasher.PutUint16(i)
	if pc.retain != nil {
		pc.retain.PutUint16(i)
	}
}

func (pc *proofCapture) PutUint32(i uint32) {
	pc.Hasher.PutUint32(i)
	if pc.retain != nil {
		pc.retain.PutUint32(i)
	}
}

func (pc *proofCapture) PutUint64(i uint64) {
	pc.Hasher.PutUint64(i)
	if pc.retain != nil {
		pc.retain.PutUint64(i)
	}
}

// WithTemp serves the callback from the hasher's scratch buffer only; the
// callback runs once regardless of capture state.
func (pc *proofCapture) WithTemp(fn func(tmp []byte) []byte) {
	pc.Hasher.WithTemp(fn)
}

// --- subtree-valued put calls ---

// putScheduled looks up the schedule of the child the next Put-style
// subtree call produces. Must be called before forwarding the call.
func (pc *proofCapture) putScheduled() (uint64, *ProofSchedule) {
	scope := pc.scheduledScope()
	if scope == nil {
		return 0, nil
	}
	pc.sweep(scope, false)
	ordinal := pc.childOrdinal(scope)
	return ordinal, scope.sched.child(ordinal)
}

func (pc *proofCapture) PutBytes(b []byte) {
	ordinal, sched := pc.putScheduled()
	pc.Hasher.PutBytes(b)
	if pc.retain != nil {
		pc.retain.PutBytes(b)
		return
	}
	pc.putPruned(ordinal, sched, b, 0, false, 0)
}

func (pc *proofCapture) PutBitlist(bb []byte, maxSize uint64) {
	ordinal, sched := pc.putScheduled()
	pc.Hasher.PutBitlist(bb, maxSize)
	if pc.retain != nil {
		pc.retain.PutBitlist(bb, maxSize)
		return
	}
	if sched == nil || sched.empty() {
		return
	}
	bitlist, size := hasher.ParseBitlist(nil, bb)
	pc.putPruned(ordinal, sched, bitlist, sszutils.CalculateBitlistLimit(maxSize), true, size)
}

func (pc *proofCapture) PutProgressiveBitlist(bb []byte) {
	ordinal, sched := pc.putScheduled()
	pc.Hasher.PutProgressiveBitlist(bb)
	if pc.retain != nil {
		pc.retain.PutProgressiveBitlist(bb)
		return
	}
	if sched == nil || sched.empty() {
		return
	}
	bitlist, size := hasher.ParseProgressiveBitlist(nil, bb)
	content := pc.prunedProgressiveChunkTree(sched, bitlist, 0, 0)
	var lengthChunk [32]byte
	sszutils.MarshalUint64(lengthChunk[:0], size)
	root := hashPair(content.value, lengthChunk[:])
	pc.registerChild(ordinal, &Node{left: content, right: newOwnedLeaf(bytes.Clone(lengthChunk[:])), value: root})
}

// prunedProgressiveChunkTree builds the pruned progressive content tree of a
// flat chunk sequence, mirroring subtree_fill_progressive: pair(pruned
// binary subtree of group g, recurse), terminated by the zero chunk once the
// data ends.
func (pc *proofCapture) prunedProgressiveChunkTree(sched *ProofSchedule, chunks []byte, g int, base uint64) *Node {
	chunkCount := uint64(len(chunks)+31) / 32
	if base >= chunkCount {
		return getEmptyNode(0)
	}
	size := uint64(1) << uint(2*g)
	left := pc.prunedChunkTree(sched, chunks, 2*g, base)
	right := pc.prunedProgressiveChunkTree(sched, chunks, g+1, base+size)
	return &Node{left: left, right: right, value: hashPair(left.value, right.value)}
}

func (pc *proofCapture) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	ordinal, sched := pc.putScheduled()
	pc.Hasher.PutUint64Array(b, maxCapacity...)
	if pc.retain != nil {
		pc.retain.PutUint64Array(b, maxCapacity...)
		return
	}
	if sched == nil || sched.empty() {
		return
	}
	chunks := make([]byte, 0, (uint64(len(b)*8)+31)/32*32)
	for _, v := range b {
		chunks = sszutils.MarshalUint64(chunks, v)
	}
	if len(maxCapacity) == 0 {
		pc.putPruned(ordinal, sched, chunks, 0, false, 0)
		return
	}
	numItems := uint64(len(b))
	pc.putPruned(ordinal, sched, chunks, sszutils.CalculateLimit(maxCapacity[0], numItems, 8), true, numItems)
}

// PutRootVector is not part of the HashWalker interface; it is mirrored for
// callers holding the concrete type. The retained and scheduled replays
// reproduce the hasher's root-vector structure through the wrapper's scope
// calls.
func (pc *proofCapture) PutRootVector(b [][]byte, maxCapacity ...uint64) error {
	ordinal, sched := pc.putScheduled()
	if err := pc.Hasher.PutRootVector(b, maxCapacity...); err != nil {
		return err
	}
	if pc.retain != nil {
		replayRootVector(pc.retain, b, maxCapacity...)
		return nil
	}
	if sched == nil || sched.empty() {
		return nil
	}
	chunks := make([]byte, 0, len(b)*32)
	for _, root := range b {
		chunks = append(chunks, root...)
	}
	if len(maxCapacity) == 0 {
		pc.putPruned(ordinal, sched, chunks, 0, false, 0)
		return nil
	}
	numItems := uint64(len(b))
	pc.putPruned(ordinal, sched, chunks, sszutils.CalculateLimit(maxCapacity[0], numItems, 32), true, numItems)
	return nil
}

// replayRootVector reproduces the hasher's root-vector structure through the
// tree wrapper's scope calls for retained subtrees.
func replayRootVector(w *treeBuilder, b [][]byte, maxCapacity ...uint64) {
	idx := w.Index()
	for _, root := range b {
		w.AppendBytes32(root)
	}
	if len(maxCapacity) == 0 {
		w.Merkleize(idx)
		return
	}
	numItems := uint64(len(b))
	w.MerkleizeWithMixin(idx, numItems, sszutils.CalculateLimit(maxCapacity[0], numItems, 32))
}

// --- scope tracking ---

func (pc *proofCapture) StartTree(treeType sszutils.TreeType) int {
	return pc.startScope(treeType)
}

// Index opens a scope like StartTree; legacy generated code uses it as its
// scope-open call.
func (pc *proofCapture) Index() int {
	return pc.startScope(sszutils.TreeTypeNone)
}

func (pc *proofCapture) startScope(treeType sszutils.TreeType) int {
	if pc.retain != nil {
		pc.retainIdx = append(pc.retainIdx, pc.retain.StartTree(treeType))
		return pc.Hasher.StartTree(treeType)
	}
	if pc.mute > 0 {
		pc.mute++
		return pc.Hasher.StartTree(treeType)
	}

	parent := &pc.scopes[len(pc.scopes)-1]
	pc.sweep(parent, false)
	ordinal := pc.childOrdinal(parent)

	child := parent.sched.child(ordinal)
	switch {
	case child == nil || child.empty():
		// No proof path descends into this subtree: the hasher computes it
		// normally, and when its root chunk is a proof target the sweep
		// retains it as a leaf.
		pc.mute = 1
		return pc.Hasher.StartTree(treeType)
	case child.retainAll:
		pc.retain = newTreeBuilder()
		pc.retainSched = child
		pc.retainOrdinal = ordinal
		pc.retainIdx = append(pc.retainIdx[:0], pc.retain.StartTree(treeType))
		return pc.Hasher.StartTree(treeType)
	default:
		// Scheduled scopes run non-incrementally, so their completed
		// children stay a plain chunk sequence in the hasher's buffer until
		// this scope's own merkleization.
		idx := pc.Hasher.StartTree(sszutils.TreeTypeNone)
		pc.scopes = append(pc.scopes, proofCaptureScope{sched: child, start: idx, ordinal: ordinal})
		return idx
	}
}

// proofClose describes the merkleization variant that closed a scheduled
// scope, so the pruned assembly reproduces the same tree shape.
type proofClose struct {
	kind         sszutils.TreeType
	mixin        bool
	num          uint64
	limit        uint64
	activeFields []byte
}

func (pc *proofCapture) Merkleize(indx int) {
	pc.closeScope(proofClose{kind: sszutils.TreeTypeNone},
		func() { pc.Hasher.Merkleize(indx) },
		func(w *treeBuilder, wIdx int) { w.Merkleize(wIdx) })
}

func (pc *proofCapture) MerkleizeWithMixin(indx int, num, limit uint64) {
	pc.closeScope(proofClose{kind: sszutils.TreeTypeNone, mixin: true, num: num, limit: limit},
		func() { pc.Hasher.MerkleizeWithMixin(indx, num, limit) },
		func(w *treeBuilder, wIdx int) { w.MerkleizeWithMixin(wIdx, num, limit) })
}

func (pc *proofCapture) MerkleizeProgressive(indx int) {
	pc.closeScope(proofClose{kind: sszutils.TreeTypeProgressive},
		func() { pc.Hasher.MerkleizeProgressive(indx) },
		func(w *treeBuilder, wIdx int) { w.MerkleizeProgressive(wIdx) })
}

func (pc *proofCapture) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	pc.closeScope(proofClose{kind: sszutils.TreeTypeProgressive, mixin: true, num: num},
		func() { pc.Hasher.MerkleizeProgressiveWithMixin(indx, num) },
		func(w *treeBuilder, wIdx int) { w.MerkleizeProgressiveWithMixin(wIdx, num) })
}

func (pc *proofCapture) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	pc.closeScope(proofClose{kind: sszutils.TreeTypeProgressive, mixin: true, activeFields: activeFields},
		func() { pc.Hasher.MerkleizeProgressiveWithActiveFields(indx, activeFields) },
		func(w *treeBuilder, wIdx int) { w.MerkleizeProgressiveWithActiveFields(wIdx, activeFields) })
}

// closeScope routes a Merkleize variant through the capture bookkeeping:
// retained subtrees mirror the call into their wrapper, muted subtrees just
// unwind, and a closing scheduled scope assembles its pruned node from its
// buffer region before the forwarded call collapses it.
func (pc *proofCapture) closeScope(variant proofClose, fwd func(), retainFn func(w *treeBuilder, wIdx int)) {
	switch {
	case pc.retain != nil:
		fwd()
		wIdx := pc.retainIdx[len(pc.retainIdx)-1]
		pc.retainIdx = pc.retainIdx[:len(pc.retainIdx)-1]
		retainFn(pc.retain, wIdx)
		if len(pc.retainIdx) == 0 {
			node := pc.retain.Node()
			node.Finalize(pc.finalizeOpts...)
			pc.retain = nil
			pc.registerChild(pc.retainOrdinal, pruneTree(node, pc.retainSched.targets))
			pc.retainSched = nil
		}
	case pc.mute > 0:
		fwd()
		pc.mute--
	case len(pc.scopes) > 1:
		scope := pc.scopes[len(pc.scopes)-1]
		pc.scopes = pc.scopes[:len(pc.scopes)-1]
		// Consume the remaining child chunks, fold the scope's pruned tree,
		// and hand its root back to the hasher as the region's single chunk.
		// The original merkleize variant must not run — it would reduce that
		// chunk to the scope's tree depth again — so the scope's hasher
		// layer closes through a plain single-chunk merkleize instead, which
		// is an identity and keeps the hasher's own root chain intact.
		pc.sweep(&scope, true)
		node, root := pc.foldScope(&scope, variant)
		pc.Hasher.Append(root[:])
		pc.Hasher.Merkleize(scope.start)
		pc.registerChild(scope.ordinal, node)
	default:
		// A Merkleize without a tracked scope (a stream bypassing StartTree)
		// has nothing to capture.
		fwd()
	}
}

// registerChild records a completed child subtree's pruned node in the
// innermost scheduled scope. Its root chunk lands in the hasher's buffer
// through the forwarded stream itself.
func (pc *proofCapture) registerChild(ordinal uint64, node *Node) {
	scope := &pc.scopes[len(pc.scopes)-1]
	if scope.childNodes == nil {
		scope.childNodes = make(map[uint64]*Node, 1)
	}
	scope.childNodes[ordinal] = node
}

// captureBlockChunks is the sweep granularity: a scheduled scope's pending
// child chunks are consumed from the hasher's buffer once this many
// accumulated, and off-path runs reduce in slices of at most this size, so
// buffers stay a fixed, cache-friendly size regardless of scope width.
const captureBlockChunks = 1024

// sweep consumes the completed child chunks a scheduled scope has
// accumulated in the hasher's buffer: scheduled slots become retained
// entries (adopting their captured node, when one was registered), off-path
// runs reduce to aligned subtree roots in batched level order, and the
// hasher's buffer is truncated back to the scope start. Without force the
// sweep waits for a full block and for a chunk-aligned region (mid-chunk
// appends leave a partial tail); with force a trailing partial chunk is
// consumed zero-extended, matching the padding the hasher itself would
// apply.
func (pc *proofCapture) sweep(scope *proofCaptureScope, force bool) {
	regionBytes := pc.CurrentIndex() - scope.start
	if regionBytes == 0 || (!force && (regionBytes%32 != 0 || uint64(regionBytes)/32 < captureBlockChunks)) {
		return
	}
	chunks := uint64(regionBytes+31) / 32
	region := pc.BufferSince(scope.start)

	var tail [32]byte
	for pos := uint64(0); pos < chunks; {
		slot := scope.drained + pos
		chunk := region[pos*32:]
		if len(chunk) < 32 {
			// Zero-extended trailing partial chunk (force only).
			copy(tail[:], chunk)
			chunk = tail[:]
		}
		if scope.sched.child(slot) != nil {
			node := scope.childNodes[slot]
			delete(scope.childNodes, slot)
			entry := captureEntry{slot: slot, depth: 0, retained: true, node: node}
			copy(entry.value[:], chunk)
			pc.pushEntry(scope, entry)
			pos++
			continue
		}
		end := pos + 1
		for end < chunks && end-pos < captureBlockChunks && scope.sched.child(scope.drained+end) == nil {
			end++
		}
		if end == chunks && regionBytes%32 != 0 {
			// The run ends in the partial tail; reduce the complete chunks
			// and re-enter for the padded remainder.
			if end-1 > pos {
				pc.reduceRun(scope, region[pos*32:(end-1)*32], slot)
				pos = end - 1
				continue
			}
			entry := captureEntry{slot: slot, depth: 0}
			copy(entry.value[:], tail[:])
			pc.pushEntry(scope, entry)
			pos++
			continue
		}
		pc.reduceRun(scope, region[pos*32:end*32], slot)
		pos = end
	}
	scope.drained += chunks
	pc.TruncateBuffer(scope.start)
}

// reduceRun reduces a run of off-path chunks to entries: the run splits into
// maximal position-aligned power-of-two segments, and each segment reduces
// to its subtree root with batched level-order hashing. In a progressively
// scheduled scope, runs additionally split at group boundaries and align by
// the offset within the group, since subtree positions are group-relative
// there.
func (pc *proofCapture) reduceRun(scope *proofCaptureScope, run []byte, startSlot uint64) {
	if scope.sched.progressive {
		for len(run) > 0 {
			groupStart := progGroupStart(startSlot)
			groupSize := groupSpan(groupStart)
			seg := groupStart + groupSize - startSlot
			if n := uint64(len(run)) / 32; seg > n {
				seg = n
			}
			pc.reduceRunAligned(scope, run[:seg*32], startSlot, startSlot-groupStart)
			run = run[seg*32:]
			startSlot += seg
		}
		return
	}
	pc.reduceRunAligned(scope, run, startSlot, startSlot)
}

// groupSpan returns the chunk count of the progressive group starting at
// the given group-start slot.
func groupSpan(groupStart uint64) uint64 {
	size := uint64(1)
	for base := uint64(0); base != groupStart; {
		base += size
		size <<= 2
	}
	return size
}

// reduceRunAligned reduces a run whose subtree alignment is governed by
// alignPos (the absolute slot for binary scopes, the group offset for
// progressive ones).
func (pc *proofCapture) reduceRunAligned(scope *proofCaptureScope, run []byte, startSlot, alignPos uint64) {
	for n := uint64(len(run)) / 32; n > 0; {
		k := uint64(1) << (bits.Len64(n) - 1)
		if alignPos != 0 {
			if a := alignPos & (-alignPos); a < k {
				k = a
			}
		}
		entry := captureEntry{slot: startSlot, depth: bits.Len64(k) - 1}
		entry.value = pc.reduceAligned(run[:k*32])
		pc.pushEntry(scope, entry)
		run = run[k*32:]
		startSlot += k
		alignPos += k
		n -= k
	}
}

// reduceAligned reduces a position-aligned power-of-two chunk segment to its
// subtree root in batched level order. The segment aliases the hasher's
// buffer, so the reduction runs on a copy.
func (pc *proofCapture) reduceAligned(seg []byte) [32]byte {
	if len(seg) == 32 {
		return [32]byte(seg)
	}
	if cap(pc.scratch) < len(seg) {
		pc.scratch = make([]byte, len(seg))
	}
	buf := pc.scratch[:len(seg)]
	copy(buf, seg)

	hashFn := pc.hashFn
	if hashFn == nil {
		hashFn = hasher.FastHasherPool.HashFn
	}
	for len(buf) > 32 {
		if err := hashFn(buf[:len(buf)/2], buf); err != nil {
			// The backend rejects only malformed buffer sizes, which this
			// packing cannot produce; reduce the level pairwise instead.
			for i := 0; i < len(buf)/64; i++ {
				root := hashPair(buf[i*64:i*64+32], buf[i*64+32:i*64+64])
				copy(buf[i*32:], root)
			}
		}
		buf = buf[:len(buf)/2]
	}
	return [32]byte(buf)
}

// pushEntry appends a completed subtree and merges the tail while adjacent
// entries form position-aligned equal-depth pairs. Value pairs merge by
// hash alone; a pair with retained structure materializes its parent node,
// so the pruned tree's path nodes emerge during the stream.
func (pc *proofCapture) pushEntry(scope *proofCaptureScope, e captureEntry) {
	scope.entries = append(scope.entries, e)
	progressive := scope.sched.progressive
	for len(scope.entries) >= 2 {
		n := len(scope.entries)
		a, b := &scope.entries[n-2], &scope.entries[n-1]
		if a.depth != b.depth || b.slot != a.slot+1<<uint(a.depth) {
			break
		}
		alignPos := a.slot
		if progressive {
			groupStart := progGroupStart(a.slot)
			if progGroupStart(b.slot) != groupStart {
				break
			}
			alignPos = a.slot - groupStart
		}
		if alignPos&(2<<uint(a.depth)-1) != 0 {
			break
		}
		merged := mergeEntries(a, b)
		scope.entries = scope.entries[:n-2]
		scope.entries = append(scope.entries, merged)
	}
}

// mergeEntries combines two adjacent equal-depth aligned subtrees into their
// parent.
func mergeEntries(a, b *captureEntry) captureEntry {
	merged := captureEntry{slot: a.slot, depth: a.depth + 1}
	if a.retained || b.retained {
		left, right := a.materialize(), b.materialize()
		parent := &Node{left: left, right: right, value: hashPair(left.value, right.value)}
		merged.retained = true
		merged.node = parent
		copy(merged.value[:], parent.value)
		return merged
	}
	copy(merged.value[:], hashPair(a.value[:], b.value[:]))
	return merged
}

// materialize returns the entry's node structure: its retained node, or a
// value leaf for an off-path subtree that becomes a pruned sibling.
func (e *captureEntry) materialize() *Node {
	if e.node != nil {
		return e.node
	}
	return newOwnedLeaf(bytes.Clone(e.value[:]))
}

// prunedChunkTree builds the pruned tree of a Put-style subtree straight
// from its flat chunk data: scheduled slots keep their chunk leaves (forcing
// path structure), off-path ranges reduce to value leaves in batched level
// order, and slots beyond the chunks become zero subtrees. No intermediate
// full tree is materialized, so deep proofs into large Put-style values stay
// data-bounded.
func (pc *proofCapture) prunedChunkTree(sched *ProofSchedule, chunks []byte, level int, base uint64) *Node {
	chunkCount := uint64(len(chunks)+31) / 32

	if !schedRangeScheduled(sched, base, level) {
		if base >= chunkCount {
			return getEmptyNode(level)
		}
		root := pc.chunkRangeRoot(chunks, base, level, chunkCount)
		return newOwnedLeaf(bytes.Clone(root[:]))
	}
	if level == 0 {
		if base >= chunkCount {
			return getEmptyNode(0)
		}
		var chunk [32]byte
		copy(chunk[:], chunks[base*32:])
		return newOwnedLeaf(bytes.Clone(chunk[:]))
	}
	left := pc.prunedChunkTree(sched, chunks, level-1, base)
	right := pc.prunedChunkTree(sched, chunks, level-1, base+uint64(1)<<uint(level-1))
	return &Node{left: left, right: right, value: hashPair(left.value, right.value)}
}

// chunkRangeRoot reduces a fully off-path chunk range to its root in
// batched level order, padding odd tails and missing levels with zero
// hashes.
func (pc *proofCapture) chunkRangeRoot(chunks []byte, base uint64, level int, chunkCount uint64) [32]byte {
	count := chunkCount - base
	if width := uint64(1) << uint(level); count > width {
		count = width
	}

	// One extra chunk keeps room for odd-tail zero padding and a trailing
	// partial chunk.
	if need := (int(count) + 1) * 32; cap(pc.scratch) < need {
		pc.scratch = make([]byte, need)
	}
	buf := pc.scratch[:count*32]
	n := copy(buf, chunks[base*32:])
	clear(buf[n:])

	hashFn := pc.hashFn
	if hashFn == nil {
		hashFn = hasher.FastHasherPool.HashFn
	}
	for l := 0; l < level; l++ {
		if count == 1 {
			root := hashPair(buf[:32], hasher.GetZeroHash(l))
			copy(buf[:32], root)
			continue
		}
		if count%2 == 1 {
			buf = pc.scratch[:(count+1)*32]
			copy(buf[count*32:], hasher.GetZeroHash(l))
			count++
		}
		if err := hashFn(buf[:count/2*32], buf[:count*32]); err != nil {
			// The backend rejects only malformed buffer sizes, which this
			// packing cannot produce; reduce the level pairwise instead.
			for i := uint64(0); i < count/2; i++ {
				root := hashPair(buf[i*64:i*64+32], buf[i*64+32:i*64+64])
				copy(buf[i*32:], root)
			}
		}
		count /= 2
		buf = buf[:count*32]
	}
	return [32]byte(buf[:32])
}

// schedRangeScheduled reports whether any of the schedule's slots lies in
// [base, base+2^level), without wrapping on depth-64 trees.
func schedRangeScheduled(sched *ProofSchedule, base uint64, level int) bool {
	if level >= 64 {
		return len(sched.children) != 0
	}
	span := uint64(1) << uint(level)
	for slot := range sched.children {
		if slot >= base && slot-base < span {
			return true
		}
	}
	return false
}

// putPruned registers the pruned tree of a Put-style subtree built from its
// flat chunk data, wrapping a length mixin when the call carries one.
// Schedules retaining only the subtree's root register nothing: the sweep
// retains the root chunk as a leaf.
func (pc *proofCapture) putPruned(ordinal uint64, sched *ProofSchedule, chunks []byte, limitChunks uint64, mixin bool, num uint64) {
	if sched == nil || sched.empty() {
		return
	}
	chunkCount := uint64(len(chunks)+31) / 32
	depth := chunkDepthSlots(chunkCount)
	if limitChunks != 0 {
		depth = chunkDepthSlots(limitChunks)
	}
	node := pc.prunedChunkTree(sched, chunks, depth, 0)
	if mixin {
		var lengthChunk [32]byte
		sszutils.MarshalUint64(lengthChunk[:0], num)
		root := hashPair(node.value, lengthChunk[:])
		node = &Node{left: node, right: newOwnedLeaf(bytes.Clone(lengthChunk[:])), value: root}
	}
	pc.registerChild(ordinal, node)
}

// pruneTree prunes a fully hashed subtree back to the given relative proof
// paths: branches no path descends into collapse to value leaves. nil
// targets mean no pruning information — the subtree is kept whole.
func pruneTree(node *Node, targets []uint64) *Node {
	if targets == nil {
		return node
	}
	return pruneNode(node, targets)
}

func pruneNode(node *Node, targets []uint64) *Node {
	if node == nil || (node.left == nil && node.right == nil) {
		return node
	}
	var leftTargets, rightTargets []uint64
	for _, rel := range targets {
		if rel <= 1 {
			// A path ending here proves this subtree's root; the collapsed
			// value serves it.
			continue
		}
		side, _ := consumePath(&rel, 1)
		if side == 0 {
			leftTargets = append(leftTargets, rel)
		} else {
			rightTargets = append(rightTargets, rel)
		}
	}
	if leftTargets == nil && rightTargets == nil {
		return newOwnedLeaf(bytes.Clone(node.value))
	}
	left := pruneNode(node.left, leftTargets)
	right := pruneNode(node.right, rightTargets)
	if left == node.left && right == node.right {
		return node
	}
	return &Node{left: left, right: right, isEmpty: node.isEmpty, value: node.value}
}

// foldScope folds a closed scope's entries into its pruned node and the
// root the hasher stream continues with. The fold pads to
// the scope's tree depth with zero subtrees, keeping empty-node structure
// under scheduled slots so proofs of emptiness stay servable.
func (pc *proofCapture) foldScope(scope *proofCaptureScope, variant proofClose) (*Node, [32]byte) {
	if variant.kind == sszutils.TreeTypeProgressive != scope.sched.progressive || variant.activeFields != nil {
		// The stream's tree shape disagrees with the schedule's
		// decomposition (or an active-fields close reached a scheduled
		// scope, which the schedule never descends into); the entries cannot
		// reproduce the other shape. Record the failure for ProofTree
		// instead of serving a wrong proof.
		if pc.err == nil {
			pc.err = sszutils.NewSszError(sszutils.ErrInvalidValueRange, "proof capture cannot fold a scope whose tree shape disagrees with the schedule")
		}
		entry := captureEntry{depth: 0}
		return entry.materialize(), entry.value
	}
	if variant.kind == sszutils.TreeTypeProgressive {
		content := pc.foldProgressive(scope, 0, 0)
		if variant.mixin {
			var lengthChunk [32]byte
			sszutils.MarshalUint64(lengthChunk[:0], variant.num)
			contentNode := content.materialize()
			root := hashPair(contentNode.value, lengthChunk[:])
			node := &Node{left: contentNode, right: newOwnedLeaf(bytes.Clone(lengthChunk[:])), value: root}
			return node, [32]byte(root)
		}
		node := content.materialize()
		return node, [32]byte(node.value)
	}

	depth := chunkDepthSlots(scope.drained)
	if variant.mixin && variant.limit != 0 {
		depth = chunkDepthSlots(variant.limit)
	}

	content := pc.foldRange(scope, scope.entries, depth, 0)
	if variant.mixin {
		var lengthChunk [32]byte
		sszutils.MarshalUint64(lengthChunk[:0], variant.num)
		contentNode := content.materialize()
		root := hashPair(contentNode.value, lengthChunk[:])
		node := &Node{left: contentNode, right: newOwnedLeaf(bytes.Clone(lengthChunk[:])), value: root}
		return node, [32]byte(root)
	}
	node := content.materialize()
	return node, [32]byte(node.value)
}

// foldRange folds the entries covering [base, base+2^level) into a single
// entry at the given level, padding missing ranges with zero subtrees:
// shared empty nodes where a scheduled slot needs structure below, plain
// zero-hash values elsewhere.
func (pc *proofCapture) foldRange(scope *proofCaptureScope, entries []captureEntry, level int, base uint64) captureEntry {
	if len(entries) == 0 {
		entry := captureEntry{slot: base, depth: level}
		copy(entry.value[:], hasher.GetZeroHash(level))
		if pc.rangeScheduled(scope, base, level) {
			entry.retained = true
			entry.node = getEmptyNode(level)
		}
		return entry
	}
	if len(entries) == 1 && entries[0].depth == level {
		return entries[0]
	}
	half := uint64(1) << uint(level-1)
	mid := base + half
	split := len(entries)
	for i, e := range entries {
		if e.slot >= mid {
			split = i
			break
		}
	}
	left := pc.foldRange(scope, entries[:split], level-1, base)
	right := pc.foldRange(scope, entries[split:], level-1, mid)
	merged := mergeEntries(&left, &right)
	return merged
}

// foldProgressive folds a progressively scheduled scope's entries into the
// content subtree starting at group g (base = the group's first chunk
// slot): pair(binary fold of the group, recurse into the rest), terminated
// by the zero chunk once the data ends, matching subtree_fill_progressive.
func (pc *proofCapture) foldProgressive(scope *proofCaptureScope, g int, base uint64) captureEntry {
	if base >= scope.drained {
		// The spine terminates with zero_node(0). Interior targets pinned at
		// or beyond the terminator keep it retained, so the spine path to
		// the zero chunk stays provable, exactly as in the full tree.
		entry := captureEntry{slot: base, depth: 0}
		for slot := range scope.sched.children {
			if slot >= base {
				entry.retained = true
				entry.node = getEmptyNode(0)
				break
			}
		}
		return entry
	}
	size := uint64(1) << uint(2*g)
	var groupEntries []captureEntry
	for i, e := range scope.entries {
		if e.slot >= base && e.slot-base < size {
			if groupEntries == nil {
				groupEntries = scope.entries[i:i]
			}
			groupEntries = scope.entries[i-len(groupEntries) : i+1]
		}
	}
	left := pc.foldRange(scope, groupEntries, 2*g, base)
	right := pc.foldProgressive(scope, g+1, base+size)
	return mergeEntries(&left, &right)
}

// rangeScheduled reports whether any scheduled slot lies in
// [base, base+2^level).
func (pc *proofCapture) rangeScheduled(scope *proofCaptureScope, base uint64, level int) bool {
	return schedRangeScheduled(scope.sched, base, level)
}
