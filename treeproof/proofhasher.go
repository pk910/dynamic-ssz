// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"crypto/sha256"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ sszutils.HashWalker = (*ProofHasher)(nil)

// ProofHasher is a HashWalker that computes a value's hash tree root through
// an embedded hasher while capturing just enough of the tree to serve proofs
// for a schedule of generalized indices. Off-path subtrees pass straight
// through the hasher's flat, batched merkleization and never materialize
// nodes. Scheduled scopes run on the hasher's non-incremental path, so their
// completed children lie as a plain chunk sequence in the hasher's buffer;
// when such a scope closes, its pruned node tree is assembled directly from
// that buffer region, with off-path sibling ranges reduced in batched level
// order. Subtrees whose layout depends on runtime data (retainAll in the
// schedule) are captured whole through a tree wrapper fed in parallel.
// Async hashing is supported: the buffer reads drain any background
// reduction overlapping the region.
//
// Scopes are matched to their reductions by buffer index, as the hasher
// matches its layers, so a legacy walker that reduces a closed region again
// (the fastssz mix-in pattern) folds the region's captured children into the
// node the reduction produces instead of confusing the scope stack.
type ProofHasher struct {
	*hasher.Hasher

	scopes []proofHasherScope

	// muted holds the start indices of the open scopes inside an off-path
	// subtree: the stream is forwarded untouched, and the subtree's root
	// lands in the enclosing scope's buffer region by itself.
	muted []int

	// retain captures a retainAll subtree through a tree wrapper fed in
	// parallel with the hasher. retainIdx pairs the hasher's open scope
	// indices with the wrapper's, retainLast is the pair closed last, and
	// retainOrdinal is the subtree's child ordinal in the enclosing scheduled
	// scope.
	retain        *Wrapper
	retainIdx     []retainScope
	retainLast    retainScope
	retainOrdinal uint64

	hashFn  hasher.HashFn
	scratch []byte

	// captureErr records the first failure of the capture itself: a shape
	// mismatch in a scheduled scope, or a hash backend refusing a captured
	// subtree. The hasher records its own failures; both refuse the root.
	captureErr error
}

// proofHasherScope is one open scheduled scope: its start position in the
// hasher's buffer, its child ordinal in the enclosing scope, the tree type it
// was opened with, the schedule that applies to its children, and the pruned
// nodes of children that were themselves scheduled.
type proofHasherScope struct {
	sched      *ProofSchedule
	start      int
	ordinal    uint64
	treeType   sszutils.TreeType
	childNodes map[uint64]*Node
	slots      []uint64
}

// retainScope pairs a scope index of the hasher's stream with the index the
// retained subtree's wrapper returned for the same scope.
type retainScope struct {
	hIdx int
	wIdx int
}

// NewProofHasher creates a HashWalker that computes the hash tree root on
// the given hasher while capturing a pruned proof tree for the given
// retention schedule. The hasher must be freshly reset. hashFn compresses
// the capture's own nodes — sibling-range reductions, pruned branches and
// retained subtrees — and must be safe for concurrent use; nil selects the
// function hasher.FastHasherPool currently hands out.
func NewProofHasher(sched *ProofSchedule, hh *hasher.Hasher, hashFn hasher.HashFn) *ProofHasher {
	// The base scope hands the root schedule to the value's own top-level
	// scope, which opens as its first child.
	base := &ProofSchedule{children: map[uint64]*ProofSchedule{0: sched}}
	return &ProofHasher{
		Hasher: hh,
		hashFn: hashFn,
		scopes: []proofHasherScope{{sched: base}},
	}
}

// ProofTree returns the pruned proof tree after the walk completed. A
// stream without a captured top-level subtree (a bare basic value) serves
// its root as a single leaf. The tree's root is verified against the
// hasher's root, so a divergence in the capture bookkeeping surfaces as an
// error instead of a wrong proof.
func (ph *ProofHasher) ProofTree() (*Node, error) {
	root, err := ph.HashRoot()
	if err != nil {
		return nil, err
	}

	tree := ph.scopes[0].childNodes[0]
	if tree == nil {
		tree = newLeaf(root[:])
	}

	if !tree.hasValue || tree.value != root {
		return nil, sszutils.NewSszError(sszutils.ErrInvalidValueRange, "proof tree root diverges from hash tree root")
	}

	return tree, nil
}

// HashErr reports the first failure of the walk: the hasher's, or failing
// that the capture's own.
func (ph *ProofHasher) HashErr() error {
	if err := ph.Hasher.HashErr(); err != nil {
		return err
	}
	return ph.captureErr
}

// HashRoot returns the hasher's root, refused when the capture recorded a
// failure the hasher did not see itself.
func (ph *ProofHasher) HashRoot() ([32]byte, error) {
	root, err := ph.Hasher.HashRoot()
	if err != nil {
		return [32]byte{}, err
	}
	if ph.captureErr != nil {
		return [32]byte{}, ph.captureErr
	}
	return root, nil
}

// setCaptureErr records the first failure of the capture. Later ones are
// dropped: the first is the failure the caller has to act on.
func (ph *ProofHasher) setCaptureErr(err error) {
	if err != nil && ph.captureErr == nil {
		ph.captureErr = err
	}
}

// scheduledScope returns the scheduled scope receiving current children,
// nil while the stream is inside a muted or retained subtree.
func (ph *ProofHasher) scheduledScope() *proofHasherScope {
	if len(ph.muted) > 0 || ph.retain != nil {
		return nil
	}
	return &ph.scopes[len(ph.scopes)-1]
}

// childOrdinal returns the ordinal the next child takes in the given scope:
// the number of 32-byte chunks its completed children occupy in the
// hasher's buffer.
func (ph *ProofHasher) childOrdinal(scope *proofHasherScope) uint64 {
	return uint64(ph.CurrentIndex()-scope.start) / 32
}

// hashFunc returns the function the capture compresses with: the one the
// caller gave, else the one hasher.FastHasherPool currently hands out, so
// the capture compresses as the root does.
func (ph *ProofHasher) hashFunc() hasher.HashFn {
	if ph.hashFn != nil {
		return ph.hashFn
	}
	return hasher.FastHasherPool.CurrentHashFn()
}

// pairRoot compresses one sibling pair through the capture's hash function.
// The backend rejects only malformed buffer sizes, which a pair is not; a
// refusal falls back to the built-in compression so the node still holds a
// root.
func (ph *ProofHasher) pairRoot(left, right []byte) [32]byte {
	var pair [64]byte
	copy(pair[:32], left)
	copy(pair[32:], right)
	if err := ph.hashFunc()(pair[:32], pair[:]); err != nil {
		copy(pair[:32], left)
		return sha256.Sum256(pair[:])
	}
	return [32]byte(pair[:32])
}

// newBranch builds a finalized branch over two finalized children.
func (ph *ProofHasher) newBranch(left, right *Node) *Node {
	return &Node{
		left:     left,
		right:    right,
		value:    ph.pairRoot(left.value[:], right.value[:]),
		hasValue: true,
	}
}

// --- retained-subtree stream mirroring ---

func (ph *ProofHasher) Append(i []byte) {
	ph.Hasher.Append(i)
	if ph.retain != nil {
		ph.retain.Append(i)
	}
}

func (ph *ProofHasher) AppendBool(b bool) {
	ph.Hasher.AppendBool(b)
	if ph.retain != nil {
		ph.retain.AppendBool(b)
	}
}

func (ph *ProofHasher) AppendUint8(i uint8) {
	ph.Hasher.AppendUint8(i)
	if ph.retain != nil {
		ph.retain.AppendUint8(i)
	}
}

func (ph *ProofHasher) AppendUint16(i uint16) {
	ph.Hasher.AppendUint16(i)
	if ph.retain != nil {
		ph.retain.AppendUint16(i)
	}
}

func (ph *ProofHasher) AppendUint32(i uint32) {
	ph.Hasher.AppendUint32(i)
	if ph.retain != nil {
		ph.retain.AppendUint32(i)
	}
}

func (ph *ProofHasher) AppendUint64(i uint64) {
	ph.Hasher.AppendUint64(i)
	if ph.retain != nil {
		ph.retain.AppendUint64(i)
	}
}

func (ph *ProofHasher) AppendBytes32(b []byte) {
	ph.Hasher.AppendBytes32(b)
	if ph.retain != nil {
		ph.retain.AppendBytes32(b)
	}
}

func (ph *ProofHasher) FillUpTo32() {
	ph.Hasher.FillUpTo32()
	if ph.retain != nil {
		ph.retain.FillUpTo32()
	}
}

func (ph *ProofHasher) PutBool(b bool) {
	ph.Hasher.PutBool(b)
	if ph.retain != nil {
		ph.retain.PutBool(b)
	}
}

func (ph *ProofHasher) PutUint8(i uint8) {
	ph.Hasher.PutUint8(i)
	if ph.retain != nil {
		ph.retain.PutUint8(i)
	}
}

func (ph *ProofHasher) PutUint16(i uint16) {
	ph.Hasher.PutUint16(i)
	if ph.retain != nil {
		ph.retain.PutUint16(i)
	}
}

func (ph *ProofHasher) PutUint32(i uint32) {
	ph.Hasher.PutUint32(i)
	if ph.retain != nil {
		ph.retain.PutUint32(i)
	}
}

func (ph *ProofHasher) PutUint64(i uint64) {
	ph.Hasher.PutUint64(i)
	if ph.retain != nil {
		ph.retain.PutUint64(i)
	}
}

// WithTemp serves the callback from the hasher's scratch buffer only; the
// callback runs once regardless of capture state.
func (ph *ProofHasher) WithTemp(fn func(tmp []byte) []byte) {
	ph.Hasher.WithTemp(fn)
}

// --- subtree-valued put calls ---

// putScheduled looks up the schedule of the child the next Put-style
// subtree call produces. Must be called before forwarding the call. Inside a
// packed scope the call appends packed bytes, not a child of its own, so the
// chunk slots are read from the buffer and nothing is scheduled here.
func (ph *ProofHasher) putScheduled() (uint64, *ProofSchedule) {
	scope := ph.scheduledScope()
	if scope == nil || scope.treeType&sszutils.TreeTypePacked != 0 {
		return 0, nil
	}
	ordinal := ph.childOrdinal(scope)
	return ordinal, scope.sched.child(ordinal)
}

// putSubtree finishes a Put-style call that nets one child subtree: a child
// the schedule descends into rebuilds its structure through a tree wrapper,
// matching the full tree's shape for these calls. A call that contributed
// no chunk (an empty value) registers nothing.
func (ph *ProofHasher) putSubtree(ordinal uint64, sched *ProofSchedule, replay func(w *Wrapper)) {
	if sched == nil {
		return
	}
	w := NewWrapperWithHashFn(ph.hashFn)
	replay(w)
	if w.CurrentIndex() == 0 {
		return
	}
	ph.registerChild(ordinal, ph.wrapperNode(w))
}

// wrapperNode takes the finished tree of a capture wrapper, finalized
// through the capture's hash function. A refusal is recorded and leaves the
// child unregistered; the root is then refused with that error.
func (ph *ProofHasher) wrapperNode(w *Wrapper) *Node {
	node, err := w.Root()
	if err == nil {
		err = node.Finalize(WithHashFn(ph.hashFn))
	}
	if err != nil {
		ph.setCaptureErr(err)
		return nil
	}
	return node
}

func (ph *ProofHasher) PutBytes(b []byte) {
	ordinal, sched := ph.putScheduled()
	ph.Hasher.PutBytes(b)
	if ph.retain != nil {
		ph.retain.PutBytes(b)
		return
	}
	ph.putSubtree(ordinal, sched, func(w *Wrapper) { w.PutBytes(b) })
}

func (ph *ProofHasher) PutBitlist(bb []byte, maxSize uint64) {
	ordinal, sched := ph.putScheduled()
	ph.Hasher.PutBitlist(bb, maxSize)
	if ph.retain != nil {
		ph.retain.PutBitlist(bb, maxSize)
		return
	}
	ph.putSubtree(ordinal, sched, func(w *Wrapper) { w.PutBitlist(bb, maxSize) })
}

func (ph *ProofHasher) PutProgressiveBitlist(bb []byte) {
	ordinal, sched := ph.putScheduled()
	ph.Hasher.PutProgressiveBitlist(bb)
	if ph.retain != nil {
		ph.retain.PutProgressiveBitlist(bb)
		return
	}
	ph.putSubtree(ordinal, sched, func(w *Wrapper) { w.PutProgressiveBitlist(bb) })
}

func (ph *ProofHasher) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	ordinal, sched := ph.putScheduled()
	ph.Hasher.PutUint64Array(b, maxCapacity...)
	if ph.retain != nil {
		ph.retain.PutUint64Array(b, maxCapacity...)
		return
	}
	ph.putSubtree(ordinal, sched, func(w *Wrapper) { w.PutUint64Array(b, maxCapacity...) })
}

// PutRootVector is not part of the HashWalker interface; it is mirrored for
// callers holding the concrete type. The retained and scheduled replays
// reproduce the hasher's root-vector structure through the wrapper's scope
// calls.
func (ph *ProofHasher) PutRootVector(b [][]byte, maxCapacity ...uint64) error {
	ordinal, sched := ph.putScheduled()
	if err := ph.Hasher.PutRootVector(b, maxCapacity...); err != nil {
		return err
	}
	replay := func(w *Wrapper) {
		idx := w.Index()
		for _, root := range b {
			w.AppendBytes32(root)
		}
		if len(maxCapacity) == 0 {
			w.Merkleize(idx)
		} else {
			numItems := uint64(len(b))
			w.MerkleizeWithMixin(idx, numItems, sszutils.CalculateLimit(maxCapacity[0], numItems, 32))
		}
	}
	if ph.retain != nil {
		replay(ph.retain)
		return nil
	}
	ph.putSubtree(ordinal, sched, replay)
	return nil
}

// --- scope tracking ---

func (ph *ProofHasher) StartTree(treeType sszutils.TreeType) int {
	return ph.startScope(treeType, false)
}

// Index opens a scope like StartTree; legacy generated code uses it as its
// scope-open call. It is forwarded as the hasher's Index, which reduces the
// scope in place and never defers it, so a caller may reduce the region
// again.
func (ph *ProofHasher) Index() int {
	return ph.startScope(sszutils.TreeTypeNone, true)
}

// openHasherScope opens a scope on the hasher with the given tree type, or
// through Index for a legacy caller.
func (ph *ProofHasher) openHasherScope(treeType sszutils.TreeType, legacy bool) int {
	if legacy {
		return ph.Hasher.Index()
	}
	return ph.Hasher.StartTree(treeType)
}

// openRetainScope opens the same scope on the retained subtree's wrapper.
func (ph *ProofHasher) openRetainScope(treeType sszutils.TreeType, legacy bool) int {
	if legacy {
		return ph.retain.Index()
	}
	return ph.retain.StartTree(treeType)
}

func (ph *ProofHasher) startScope(treeType sszutils.TreeType, legacy bool) int {
	if ph.retain != nil {
		wIdx := ph.openRetainScope(treeType, legacy)
		hIdx := ph.openHasherScope(treeType, legacy)
		ph.retainIdx = append(ph.retainIdx, retainScope{hIdx: hIdx, wIdx: wIdx})
		return hIdx
	}
	if len(ph.muted) > 0 {
		idx := ph.openHasherScope(treeType, legacy)
		ph.muted = append(ph.muted, idx)
		return idx
	}

	parent := &ph.scopes[len(ph.scopes)-1]
	ordinal := ph.childOrdinal(parent)

	child := parent.sched.child(ordinal)
	switch {
	case child == nil:
		idx := ph.openHasherScope(treeType, legacy)
		ph.muted = append(ph.muted, idx)
		return idx
	case child.retainAll:
		ph.retain = NewWrapperWithHashFn(ph.hashFn)
		ph.retainOrdinal = ordinal
		wIdx := ph.openRetainScope(treeType, legacy)
		hIdx := ph.openHasherScope(treeType, legacy)
		ph.retainIdx = append(ph.retainIdx[:0], retainScope{hIdx: hIdx, wIdx: wIdx})
		return hIdx
	default:
		// Scheduled scopes run non-incrementally, so their completed
		// children stay a plain chunk sequence in the hasher's buffer until
		// this scope's own merkleization. The packed flag travels on: it
		// decides how the scope's basic values are laid out, not how the
		// scope reduces. The declared shape is checked here at close.
		idx := ph.openHasherScope(sszutils.TreeTypeNone|treeType&sszutils.TreeTypePacked, legacy)
		ph.scopes = append(ph.scopes, proofHasherScope{
			sched:    child,
			start:    idx,
			ordinal:  ordinal,
			treeType: treeType,
		})
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

// progressive reports whether the variant reduces to the progressive shape.
func (pc proofClose) progressive() bool {
	return pc.kind == sszutils.TreeTypeProgressive
}

// closeWrapperScope reduces the wrapper scope at idx with the variant.
func closeWrapperScope(w *Wrapper, idx int, pc proofClose) {
	switch {
	case pc.progressive() && pc.activeFields != nil:
		w.MerkleizeProgressiveWithActiveFields(idx, pc.activeFields)
	case pc.progressive() && pc.mixin:
		w.MerkleizeProgressiveWithMixin(idx, pc.num)
	case pc.progressive():
		w.MerkleizeProgressive(idx)
	case pc.mixin:
		w.MerkleizeWithMixin(idx, pc.num, pc.limit)
	default:
		w.Merkleize(idx)
	}
}

func (ph *ProofHasher) Merkleize(indx int) {
	ph.closeScope(indx, proofClose{kind: sszutils.TreeTypeNone},
		func() { ph.Hasher.Merkleize(indx) })
}

func (ph *ProofHasher) MerkleizeWithMixin(indx int, num, limit uint64) {
	ph.closeScope(indx, proofClose{kind: sszutils.TreeTypeNone, mixin: true, num: num, limit: limit},
		func() { ph.Hasher.MerkleizeWithMixin(indx, num, limit) })
}

func (ph *ProofHasher) MerkleizeProgressive(indx int) {
	ph.closeScope(indx, proofClose{kind: sszutils.TreeTypeProgressive},
		func() { ph.Hasher.MerkleizeProgressive(indx) })
}

func (ph *ProofHasher) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	ph.closeScope(indx, proofClose{kind: sszutils.TreeTypeProgressive, mixin: true, num: num},
		func() { ph.Hasher.MerkleizeProgressiveWithMixin(indx, num) })
}

func (ph *ProofHasher) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	ph.closeScope(indx, proofClose{kind: sszutils.TreeTypeProgressive, mixin: true, activeFields: activeFields},
		func() { ph.Hasher.MerkleizeProgressiveWithActiveFields(indx, activeFields) })
}

// clampIndex bounds a caller-supplied scope index to the hasher's buffer, as
// the hasher does before matching it against its layers.
func (ph *ProofHasher) clampIndex(indx int) int {
	if indx < 0 {
		return 0
	}
	if end := ph.CurrentIndex(); indx > end {
		return end
	}
	return indx
}

// closeScope routes a Merkleize variant through the capture bookkeeping:
// retained subtrees mirror the call into their wrapper, muted subtrees just
// unwind, and a closing scheduled scope assembles its pruned node from its
// buffer region before the forwarded call collapses it. A reduction that
// matches no open scope but falls inside the current scheduled scope folds
// the captured children of its region into the node it produces, keeping
// the capture consistent with the buffer for legacy walkers that reduce a
// region twice.
func (ph *ProofHasher) closeScope(indx int, pc proofClose, fwd func()) {
	indx = ph.clampIndex(indx)
	switch {
	case ph.retain != nil:
		fwd()
		closeWrapperScope(ph.retain, ph.retainIndex(indx), pc)
		if len(ph.retainIdx) == 0 {
			node := ph.wrapperNode(ph.retain)
			ph.retain = nil
			ph.registerChild(ph.retainOrdinal, node)
		}
	case len(ph.muted) > 0:
		fwd()
		if ph.muted[len(ph.muted)-1] == indx {
			ph.muted = ph.muted[:len(ph.muted)-1]
		}
	case len(ph.scopes) > 1 && ph.scopes[len(ph.scopes)-1].start == indx:
		scope := ph.scopes[len(ph.scopes)-1]
		ph.scopes = ph.scopes[:len(ph.scopes)-1]
		ph.checkShape(&scope, pc)
		// The chunk region is only readable until the forwarded call
		// collapses it.
		node := ph.assemble(&scope, ph.BufferSince(scope.start), pc)
		fwd()
		ph.registerChild(scope.ordinal, node)
	case indx >= ph.scopes[len(ph.scopes)-1].start && indx < ph.CurrentIndex():
		scope := &ph.scopes[len(ph.scopes)-1]
		first := uint64(indx-scope.start) / 32
		node := ph.assembleViaWrapper(ph.foldChildren(scope, first), ph.BufferSince(indx), pc)
		fwd()
		ph.registerChild(first, node)
	default:
		// A reduction of an empty region, or one reaching below the current
		// scope: nothing of the capture is affected.
		fwd()
	}
}

// retainIndex maps a reduction inside the retained subtree to the wrapper's
// index for it: the innermost open scope when it matches, the scope closed
// last for a legacy walker reducing it again, and otherwise the position
// relative to the innermost open scope.
func (ph *ProofHasher) retainIndex(indx int) int {
	if n := len(ph.retainIdx); n > 0 && ph.retainIdx[n-1].hIdx == indx {
		ph.retainLast = ph.retainIdx[n-1]
		ph.retainIdx = ph.retainIdx[:n-1]
		return ph.retainLast.wIdx
	}
	if ph.retainLast.hIdx == indx {
		return ph.retainLast.wIdx
	}
	base := retainScope{}
	if n := len(ph.retainIdx); n > 0 {
		base = ph.retainIdx[n-1]
	}
	return base.wIdx + indx - base.hIdx
}

// checkShape records a scheduled scope reduced in a shape it was not opened
// for. The hasher runs the scope undeclared, so the check it would have made
// is made here; a scope that declared no shape accepts either reduction.
func (ph *ProofHasher) checkShape(scope *proofHasherScope, pc proofClose) {
	declared := scope.treeType &^ sszutils.TreeTypePacked
	if declared == sszutils.TreeTypeNone {
		return
	}
	if (declared == sszutils.TreeTypeProgressive) != pc.progressive() {
		ph.setCaptureErr(sszutils.ErrScopeShapeMismatch)
	}
}

// foldChildren removes the captured children at ordinal first and beyond
// from the scope and returns them keyed by their slot in the folded region.
func (ph *ProofHasher) foldChildren(scope *proofHasherScope, first uint64) map[uint64]*Node {
	var folded map[uint64]*Node
	for ordinal, node := range scope.childNodes {
		if ordinal < first {
			continue
		}
		if folded == nil {
			folded = make(map[uint64]*Node, len(scope.childNodes))
		}
		folded[ordinal-first] = node
		delete(scope.childNodes, ordinal)
	}
	return folded
}

// registerChild records a completed child subtree's pruned node in the
// innermost scheduled scope. Its root chunk lands in the hasher's buffer
// through the forwarded stream itself. A nil node (a refused capture) is
// not recorded; the slot then reads from the buffer.
func (ph *ProofHasher) registerChild(ordinal uint64, node *Node) {
	if node == nil {
		return
	}
	scope := &ph.scopes[len(ph.scopes)-1]
	if scope.childNodes == nil {
		scope.childNodes = make(map[uint64]*Node, 1)
	}
	scope.childNodes[ordinal] = node
}

// alignRegion returns the region as full 32-byte chunks, zero-padding a
// trailing partial chunk through the scratch buffer. Engine streams close
// scopes chunk-aligned, so the copy is the exception.
func (ph *ProofHasher) alignRegion(region []byte) []byte {
	if len(region)%32 == 0 {
		return region
	}
	need := (len(region)/32 + 1) * 32
	if cap(ph.scratch) < need {
		ph.scratch = make([]byte, need)
	}
	aligned := ph.scratch[:need]
	n := copy(aligned, region)
	for i := n; i < need; i++ {
		aligned[i] = 0
	}
	return aligned
}

// assemble builds the pruned node tree of a closed scheduled scope from its
// buffer region, its scheduled children's nodes, and the merkleization
// variant that closed it.
func (ph *ProofHasher) assemble(scope *proofHasherScope, region []byte, pc proofClose) *Node {
	if pc.progressive() {
		// A schedule never descends into progressive shapes, so a scheduled
		// scope closing progressively means the stream and the descriptor
		// disagree; replaying the children through a tree wrapper stays
		// correct either way.
		return ph.assembleViaWrapper(scope.childNodes, region, pc)
	}
	region = ph.alignRegion(region)

	for slot := range scope.sched.children {
		scope.slots = append(scope.slots, slot)
	}

	chunkCount := uint64(len(region)) / 32
	depth := chunkDepthSlots(chunkCount)
	if pc.mixin && pc.limit != 0 {
		depth = chunkDepthSlots(pc.limit)
	}

	node := ph.buildPruned(scope, region, depth, 0)
	if pc.mixin {
		var lengthChunk [32]byte
		sszutils.MarshalUint64(lengthChunk[:0], pc.num)
		node = ph.newBranch(node, newLeaf(lengthChunk[:]))
	}
	return node
}

// assembleViaWrapper rebuilds a region through the tree wrapper, each chunk
// slot a leaf or the captured node of the child that produced it, and
// closes it with the given variant. It serves the closes that have no static
// shape and the reductions that match no open scope.
func (ph *ProofHasher) assembleViaWrapper(childNodes map[uint64]*Node, region []byte, pc proofClose) *Node {
	region = ph.alignRegion(region)
	w := NewWrapperWithHashFn(ph.hashFn)
	idx := w.StartTree(sszutils.TreeTypeNone)
	for slot := uint64(0); slot < uint64(len(region))/32; slot++ {
		if node := childNodes[slot]; node != nil {
			w.AddNode(node)
		} else {
			w.AddNode(newLeaf(region[slot*32 : slot*32+32]))
		}
	}
	closeWrapperScope(w, idx, pc)
	return ph.wrapperNode(w)
}

// buildPruned builds the pruned subtree of the given height starting at leaf
// slot base: ranges containing scheduled slots keep their branch structure,
// everything else reduces to a single value node, and slots beyond the
// region's chunks become shared zero-padding nodes.
func (ph *ProofHasher) buildPruned(scope *proofHasherScope, region []byte, level int, base uint64) *Node {
	chunkCount := uint64(len(region)) / 32

	scheduled := false
	for _, slot := range scope.slots {
		if slot >= base && slot < base+uint64(1)<<level {
			scheduled = true
			break
		}
	}

	if !scheduled {
		if base >= chunkCount {
			return getEmptyNode(level)
		}
		root := ph.rangeRoot(region, base, level, chunkCount)
		return newLeaf(root[:])
	}
	if level == 0 {
		if node := scope.childNodes[base]; node != nil {
			return node
		}
		if base >= chunkCount {
			return getEmptyNode(0)
		}
		return newLeaf(region[base*32 : base*32+32])
	}

	left := ph.buildPruned(scope, region, level-1, base)
	right := ph.buildPruned(scope, region, level-1, base+uint64(1)<<(level-1))
	return ph.newBranch(left, right)
}

// rangeRoot reduces a fully off-path chunk range to its root in batched
// level order, padding odd tails and missing levels with zero hashes.
func (ph *ProofHasher) rangeRoot(region []byte, base uint64, level int, chunkCount uint64) [32]byte {
	if base >= chunkCount {
		return [32]byte(hasher.GetZeroHash(level))
	}

	count := chunkCount - base
	if width := uint64(1) << level; count > width {
		count = width
	}

	// One extra chunk keeps room for the odd-tail zero padding. The region
	// aliases the hasher's buffer, so the reduction runs on a copy.
	if need := (int(count) + 1) * 32; cap(ph.scratch) < need {
		ph.scratch = make([]byte, need)
	}
	buf := ph.scratch[:count*32]
	copy(buf, region[base*32:])

	hashFn := ph.hashFunc()
	for l := 0; l < level; l++ {
		if count == 1 {
			root := ph.pairRoot(buf[:32], hasher.GetZeroHash(l))
			copy(buf[:32], root[:])
			continue
		}
		if count%2 == 1 {
			buf = ph.scratch[:(count+1)*32]
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
