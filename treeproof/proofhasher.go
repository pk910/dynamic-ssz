// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"

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
type ProofHasher struct {
	*hasher.Hasher

	scopes []proofHasherScope

	// mute counts scope nesting inside an off-path subtree: the stream is
	// forwarded untouched, and the subtree's root lands in the enclosing
	// scope's buffer region by itself.
	mute int

	// retain captures a retainAll subtree through a tree wrapper fed in
	// parallel with the hasher. retainIdx maps the wrapper's scope indices
	// to the nesting, retainOrdinal is the subtree's child ordinal in the
	// enclosing scheduled scope.
	retain        *Wrapper
	retainIdx     []int
	retainOrdinal uint64

	hashFn  hasher.HashFn
	scratch []byte
}

// proofHasherScope is one open scheduled scope: its start position in the
// hasher's buffer, its child ordinal in the enclosing scope, the schedule
// that applies to its children, and the pruned nodes of children that were
// themselves scheduled.
type proofHasherScope struct {
	sched      *ProofSchedule
	start      int
	ordinal    uint64
	childNodes map[uint64]*Node
	slots      []uint64
}

// NewProofHasher creates a HashWalker that computes the hash tree root on
// the given hasher while capturing a pruned proof tree for the given
// retention schedule. The hasher must be freshly reset. hashFn is used for
// the capture's own sibling-range reductions and must be safe for
// concurrent use; nil selects the fast hashing backend.
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
		tree = newOwnedLeaf(bytes.Clone(root[:]))
	}

	if !bytes.Equal(tree.value, root[:]) {
		return nil, sszutils.NewSszError(sszutils.ErrInvalidValueRange, "proof tree root diverges from hash tree root")
	}

	return tree, nil
}

// scheduledScope returns the scheduled scope receiving current children,
// nil while the stream is inside a muted or retained subtree.
func (ph *ProofHasher) scheduledScope() *proofHasherScope {
	if ph.mute > 0 || ph.retain != nil {
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
// subtree call produces. Must be called before forwarding the call.
func (ph *ProofHasher) putScheduled() (uint64, *ProofSchedule) {
	scope := ph.scheduledScope()
	if scope == nil {
		return 0, nil
	}
	ordinal := ph.childOrdinal(scope)
	return ordinal, scope.sched.child(ordinal)
}

// putSubtree finishes a Put-style call that nets one child subtree: a child
// the schedule descends into rebuilds its structure through a tree wrapper,
// matching the full tree's shape for these calls.
func (ph *ProofHasher) putSubtree(ordinal uint64, sched *ProofSchedule, replay func(w *Wrapper)) {
	if sched == nil {
		return
	}
	w := NewWrapper()
	replay(w)
	node := w.Node()
	node.Hash()
	ph.registerChild(ordinal, node)
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
	return ph.startScope(treeType)
}

// Index opens a scope like StartTree; legacy generated code uses it as its
// scope-open call.
func (ph *ProofHasher) Index() int {
	return ph.startScope(sszutils.TreeTypeNone)
}

func (ph *ProofHasher) startScope(treeType sszutils.TreeType) int {
	if ph.retain != nil {
		ph.retainIdx = append(ph.retainIdx, ph.retain.StartTree(treeType))
		return ph.Hasher.StartTree(treeType)
	}
	if ph.mute > 0 {
		ph.mute++
		return ph.Hasher.StartTree(treeType)
	}

	parent := &ph.scopes[len(ph.scopes)-1]
	ordinal := ph.childOrdinal(parent)

	child := parent.sched.child(ordinal)
	switch {
	case child == nil:
		ph.mute = 1
		return ph.Hasher.StartTree(treeType)
	case child.retainAll:
		ph.retain = NewWrapper()
		ph.retainOrdinal = ordinal
		ph.retainIdx = append(ph.retainIdx[:0], ph.retain.StartTree(treeType))
		return ph.Hasher.StartTree(treeType)
	default:
		// Scheduled scopes run non-incrementally, so their completed
		// children stay a plain chunk sequence in the hasher's buffer until
		// this scope's own merkleization.
		idx := ph.Hasher.StartTree(sszutils.TreeTypeNone)
		ph.scopes = append(ph.scopes, proofHasherScope{sched: child, start: idx, ordinal: ordinal})
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

func (ph *ProofHasher) Merkleize(indx int) {
	ph.closeScope(proofClose{kind: sszutils.TreeTypeNone},
		func() { ph.Hasher.Merkleize(indx) },
		func(w *Wrapper, wIdx int) { w.Merkleize(wIdx) })
}

func (ph *ProofHasher) MerkleizeWithMixin(indx int, num, limit uint64) {
	ph.closeScope(proofClose{kind: sszutils.TreeTypeNone, mixin: true, num: num, limit: limit},
		func() { ph.Hasher.MerkleizeWithMixin(indx, num, limit) },
		func(w *Wrapper, wIdx int) { w.MerkleizeWithMixin(wIdx, num, limit) })
}

func (ph *ProofHasher) MerkleizeProgressive(indx int) {
	ph.closeScope(proofClose{kind: sszutils.TreeTypeProgressive},
		func() { ph.Hasher.MerkleizeProgressive(indx) },
		func(w *Wrapper, wIdx int) { w.MerkleizeProgressive(wIdx) })
}

func (ph *ProofHasher) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	ph.closeScope(proofClose{kind: sszutils.TreeTypeProgressive, mixin: true, num: num},
		func() { ph.Hasher.MerkleizeProgressiveWithMixin(indx, num) },
		func(w *Wrapper, wIdx int) { w.MerkleizeProgressiveWithMixin(wIdx, num) })
}

func (ph *ProofHasher) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	ph.closeScope(proofClose{kind: sszutils.TreeTypeProgressive, mixin: true, activeFields: activeFields},
		func() { ph.Hasher.MerkleizeProgressiveWithActiveFields(indx, activeFields) },
		func(w *Wrapper, wIdx int) { w.MerkleizeProgressiveWithActiveFields(wIdx, activeFields) })
}

// closeScope routes a Merkleize variant through the capture bookkeeping:
// retained subtrees mirror the call into their wrapper, muted subtrees just
// unwind, and a closing scheduled scope assembles its pruned node from its
// buffer region before the forwarded call collapses it.
func (ph *ProofHasher) closeScope(pc proofClose, fwd func(), retainFn func(w *Wrapper, wIdx int)) {
	switch {
	case ph.retain != nil:
		fwd()
		wIdx := ph.retainIdx[len(ph.retainIdx)-1]
		ph.retainIdx = ph.retainIdx[:len(ph.retainIdx)-1]
		retainFn(ph.retain, wIdx)
		if len(ph.retainIdx) == 0 {
			node := ph.retain.Node()
			node.Hash()
			ph.retain = nil
			ph.registerChild(ph.retainOrdinal, node)
		}
	case ph.mute > 0:
		fwd()
		ph.mute--
	case len(ph.scopes) > 1:
		scope := ph.scopes[len(ph.scopes)-1]
		ph.scopes = ph.scopes[:len(ph.scopes)-1]
		// The chunk region is only readable until the forwarded call
		// collapses it.
		node := ph.assemble(&scope, ph.BufferSince(scope.start), pc)
		fwd()
		ph.registerChild(scope.ordinal, node)
	default:
		// A Merkleize without a tracked scope (a stream bypassing StartTree)
		// has nothing to capture.
		fwd()
	}
}

// registerChild records a completed child subtree's pruned node in the
// innermost scheduled scope. Its root chunk lands in the hasher's buffer
// through the forwarded stream itself.
func (ph *ProofHasher) registerChild(ordinal uint64, node *Node) {
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
	region = ph.alignRegion(region)

	if pc.kind == sszutils.TreeTypeProgressive {
		// A schedule never descends into progressive shapes, so a scheduled
		// scope closing progressively means the stream and the descriptor
		// disagree; replaying the children through a tree wrapper stays
		// correct either way.
		return ph.assembleViaWrapper(scope, region, pc)
	}

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
		root := hashPair(node.value, lengthChunk[:])
		node = &Node{left: node, right: newOwnedLeaf(bytes.Clone(lengthChunk[:])), value: root}
	}
	return node
}

// assembleViaWrapper rebuilds a scope through the tree wrapper, used when
// the closing variant has no static shape.
func (ph *ProofHasher) assembleViaWrapper(scope *proofHasherScope, region []byte, pc proofClose) *Node {
	w := NewWrapper()
	idx := w.StartTree(sszutils.TreeTypeNone)
	for slot := uint64(0); slot < uint64(len(region))/32; slot++ {
		if node := scope.childNodes[slot]; node != nil {
			w.AddNode(node)
		} else {
			w.AddNode(newOwnedLeaf(bytes.Clone(region[slot*32 : slot*32+32])))
		}
	}
	switch {
	case pc.activeFields != nil:
		w.MerkleizeProgressiveWithActiveFields(idx, pc.activeFields)
	case pc.mixin:
		w.MerkleizeProgressiveWithMixin(idx, pc.num)
	default:
		w.MerkleizeProgressive(idx)
	}
	node := w.Node()
	node.Hash()
	return node
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
		return newOwnedLeaf(bytes.Clone(root[:]))
	}
	if level == 0 {
		if node := scope.childNodes[base]; node != nil {
			return node
		}
		if base >= chunkCount {
			return getEmptyNode(0)
		}
		return newOwnedLeaf(bytes.Clone(region[base*32 : base*32+32]))
	}

	left := ph.buildPruned(scope, region, level-1, base)
	right := ph.buildPruned(scope, region, level-1, base+uint64(1)<<(level-1))
	return &Node{left: left, right: right, value: hashPair(left.value, right.value)}
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

	hashFn := ph.hashFn
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
