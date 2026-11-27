// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code derived from https://github.com/ferranbt/fastssz/blob/v1.0.0/wrapper.go
// Copyright (c) 2020 Ferran Borreguero
// Licensed under the MIT License
// The code has been modified for dynamic-ssz proof generation needs.

package treeproof

import (
	"fmt"
	"sort"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ sszutils.HashWalker = (*Wrapper)(nil)

// zeroChunk is one empty chunk, used to reserve a slot for a subtree root.
var zeroChunk [32]byte

// Wrapper implements the sszutils.HashWalker interface to construct a complete
// Merkle tree instead of computing a single hash. This allows generating proofs
// for any field within an SSZ structure by building the tree during the same
// traversal that would normally produce only the hash tree root.
//
// It lays its bytes out exactly as hasher.Hasher does, in one buffer, and cuts
// them into leaves only when a region is reduced. Chunks are counted from the
// start of the region being reduced, which is how the SSZ spec defines them,
// so both walkers build the same tree from any call sequence -- including one
// a delegate leaves on a partial chunk.
//
// Usage:
//
//	w := treeproof.NewWrapper()
//	// Use w as a HashWalker (e.g., via DynSsz.HashTreeRootWith or generated code)
//	myStruct.HashTreeRootWithDyn(specs, w)
//	tree := w.Node()
//	proof, _ := tree.Prove(generalizedIndex)
type Wrapper struct {
	// buf holds the same bytes hasher.Hasher would hold at the same point.
	buf []byte
	// nodes records the subtree roots that live in buf, ascending by offset:
	// the root of nodes[i] occupies buf[nodes[i].off : nodes[i].off+32]. Those
	// bytes are written only when something reads them (materialize), so a
	// subtree is hashed once, in the batched pass Node.Finalize runs.
	nodes []nodeRef
	// scopes holds one entry per open scope, in the order they were opened.
	scopes []wScope
	tmp    []byte
	// alg is the Algorithm the tree is built for. Building needs it only
	// where a subtree's root is read before the walk ends.
	alg *hasher.Algorithm

	// walkErr records the first failure of the walk: a scope reduced in a
	// shape it was not opened for, a reduction handed more chunks than its
	// limit holds, or a hash function that refused. The
	// walkers have to answer alike, and hasher.Hasher refuses the same thing.
	walkErr error
}

// nodeRef is a subtree root and the 32 bytes of buf it occupies.
type nodeRef struct {
	off  int
	node *node
	// filled reports whether node's root bytes were written into buf.
	filled bool
}

// wScope is an open scope: where its content begins, whether it packs basic
// values, and whether it was opened for the progressive tree shape.
type wScope struct {
	off         int
	packed      bool
	declared    bool
	progressive bool
}

// NewWrapper creates a new Wrapper ready to construct a Merkle tree, of
// hasher.SHA256 unless WithAlgorithm names another Algorithm. The tree
// answers with the root a hasher of that Algorithm gives for the same value.
func NewWrapper(opts ...Option) *Wrapper {
	return &Wrapper{
		buf: make([]byte, 0),
		tmp: make([]byte, 64),
		alg: algorithmOf(opts),
	}
}

// Algorithm returns the Algorithm the tree is built for.
func (w *Wrapper) Algorithm() *hasher.Algorithm {
	if w.alg == nil {
		return algorithmOf(nil)
	}
	return w.alg
}

// --- Wrapper implements the HashWalker interface ---

// WithTemp provides a temporary scratch buffer to the given function, allowing
// callers to perform intermediate computations without additional allocations.
func (w *Wrapper) WithTemp(fn func(tmp []byte) []byte) {
	w.tmp = fn(w.tmp)
}

// Index opens a scope at the current buffer position and returns it, like
// hasher.Hasher.Index. Close it with a Merkleize* call.
func (w *Wrapper) Index() int {
	w.scopes = append(w.scopes, wScope{off: len(w.buf)})
	return len(w.buf)
}

// StartTree opens a new SSZ object scope at the current buffer position and
// returns it. A packed scope makes Put* append the value's packed bytes
// instead of a chunk of its own.
func (w *Wrapper) StartTree(treeType sszutils.TreeType) int {
	w.scopes = append(w.scopes, wScope{
		off:         len(w.buf),
		packed:      treeType&sszutils.TreeTypePacked != 0,
		declared:    treeType&^sszutils.TreeTypePacked != sszutils.TreeTypeNone,
		progressive: treeType&^sszutils.TreeTypePacked == sszutils.TreeTypeProgressive,
	})
	return len(w.buf)
}

// CurrentIndex returns the current buffer position.
func (w *Wrapper) CurrentIndex() int {
	return len(w.buf)
}

// HashErr reports the first failure the walker recorded: a scope reduced in a
// shape it was not opened for, or a reduction handed more chunks than its
// limit holds. The walk continues after such a refusal so the caller sees one
// error rather than a cascade, which leaves the state it built unusable.
func (w *Wrapper) HashErr() error {
	return w.walkErr
}

// setWalkErr records the first failure of the walk. Later ones are dropped:
// the first is the failure the caller has to act on, and what followed ran
// over bytes it never properly produced.
func (w *Wrapper) setWalkErr(err error) {
	if err != nil && w.walkErr == nil {
		w.walkErr = err
	}
}

// checkChunkLimit records a reduction handed more chunks than its limit holds.
// A limit of zero states none, which an unbounded list asks for. The reduction
// still runs, over a tree deep enough for the chunks, so the walker keeps
// answering as hasher.Hasher does; the root is refused.
func (w *Wrapper) checkChunkLimit(count, limit uint64) {
	if limit == 0 || count <= limit || w.walkErr != nil {
		return
	}
	w.walkErr = sszutils.ErrChunkLimitFn(count, limit)
}

// checkShape records a scope reduced in a shape it was not opened for. A
// scope that declared no shape accepts either reduction. The reduction still
// runs; the root is refused.
func (w *Wrapper) checkShape(indx int, progressive bool) {
	n := len(w.scopes)
	if n == 0 || w.scopes[n-1].off != indx || w.walkErr != nil {
		return
	}
	if scope := w.scopes[n-1]; scope.declared && scope.progressive != progressive {
		w.walkErr = sszutils.ErrScopeShapeMismatch
	}
}

// closeScope drops the innermost scope when it is the one being reduced, as
// hasher.Hasher only pops a layer whose start matches the reduced region.
func (w *Wrapper) closeScope(indx int) {
	if n := len(w.scopes); n > 0 && w.scopes[n-1].off == indx {
		w.scopes = w.scopes[:n-1]
	}
}

// inPackedScope reports whether the innermost open scope packs basic values.
func (w *Wrapper) inPackedScope() bool {
	n := len(w.scopes)
	return n > 0 && w.scopes[n-1].packed
}

// Collapse is a no-op for the tree proof wrapper.
func (w *Wrapper) Collapse() {}

// Append appends raw bytes to the buffer without padding.
func (w *Wrapper) Append(i []byte) {
	w.buf = append(w.buf, i...)
}

// AppendUint64 appends a little-endian uint64 (8 bytes) to the buffer.
func (w *Wrapper) AppendUint64(i uint64) {
	w.buf = sszutils.MarshalUint64(w.buf, i)
}

// AppendUint32 appends a little-endian uint32 (4 bytes) to the buffer.
func (w *Wrapper) AppendUint32(i uint32) {
	w.buf = sszutils.MarshalUint32(w.buf, i)
}

// AppendUint16 appends a little-endian uint16 (2 bytes) to the buffer.
func (w *Wrapper) AppendUint16(i uint16) {
	w.buf = sszutils.MarshalUint16(w.buf, i)
}

// AppendUint8 appends a single byte to the buffer.
func (w *Wrapper) AppendUint8(i uint8) {
	w.buf = sszutils.MarshalUint8(w.buf, i)
}

// AppendBool appends a single-byte boolean (0x00 or 0x01) to the buffer.
func (w *Wrapper) AppendBool(b bool) {
	w.buf = sszutils.MarshalBool(w.buf, b)
}

// AppendBytes32 appends b and pads it to a whole number of chunks counted from
// where the value begins, as hasher.Hasher does.
func (w *Wrapper) AppendBytes32(b []byte) {
	w.buf = append(w.buf, b...)
	if rest := len(b) % 32; rest != 0 {
		w.buf = sszutils.AppendZeroPadding(w.buf, 32-rest)
	}
}

// FillUpTo32 pads the buffer with zero bytes to a 32-byte boundary.
func (w *Wrapper) FillUpTo32() {
	if rest := len(w.buf) % 32; rest != 0 {
		w.buf = sszutils.AppendZeroPadding(w.buf, 32-rest)
	}
}

// fillRegionUpTo32 pads the buffer so the region that began at indx holds a
// whole number of chunks, which is the boundary its reduction counts from.
func (w *Wrapper) fillRegionUpTo32(indx int) {
	if rest := (len(w.buf) - indx) % 32; rest != 0 {
		w.buf = sszutils.AppendZeroPadding(w.buf, 32-rest)
	}
}

// putChunk appends one chunk and hands write the room for the value's own
// bytes, as hasher.Hasher's Put* methods do for a basic value.
func (w *Wrapper) putChunk(size int, write func(chunk []byte)) {
	n := len(w.buf)
	w.buf = append(w.buf, zeroChunk[:]...)
	write(w.buf[n : n+size])
}

// PutBool adds a boolean as a chunk of its own, or its packed byte inside a
// packed scope.
func (w *Wrapper) PutBool(b bool) {
	if w.inPackedScope() {
		w.AppendBool(b)
		return
	}
	w.putChunk(1, func(chunk []byte) {
		if b {
			chunk[0] = 1
		}
	})
}

// PutUint8 adds a uint8 as a chunk of its own, or its packed byte inside a
// packed scope.
func (w *Wrapper) PutUint8(i uint8) {
	if w.inPackedScope() {
		w.AppendUint8(i)
		return
	}
	w.putChunk(1, func(chunk []byte) { chunk[0] = i })
}

// PutUint16 adds a uint16 as a chunk of its own, or its packed bytes inside a
// packed scope.
func (w *Wrapper) PutUint16(i uint16) {
	if w.inPackedScope() {
		w.AppendUint16(i)
		return
	}
	w.putChunk(2, func(chunk []byte) { sszutils.MarshalUint16(chunk[:0], i) })
}

// PutUint32 adds a uint32 as a chunk of its own, or its packed bytes inside a
// packed scope.
func (w *Wrapper) PutUint32(i uint32) {
	if w.inPackedScope() {
		w.AppendUint32(i)
		return
	}
	w.putChunk(4, func(chunk []byte) { sszutils.MarshalUint32(chunk[:0], i) })
}

// PutUint64 adds a uint64 as a chunk of its own, or its packed bytes inside a
// packed scope.
func (w *Wrapper) PutUint64(i uint64) {
	if w.inPackedScope() {
		w.AppendUint64(i)
		return
	}
	w.putChunk(8, func(chunk []byte) { sszutils.MarshalUint64(chunk[:0], i) })
}

// PutBytes adds a byte value: raw bytes inside a packed scope, one chunk when
// it fits, otherwise a subtree of its chunks.
func (w *Wrapper) PutBytes(b []byte) {
	if blen := len(b); blen <= 32 {
		if blen == 32 || w.inPackedScope() {
			w.buf = append(w.buf, b...)
			return
		}
		// Pads to the value's own boundary, so an empty value contributes nothing.
		w.AppendBytes32(b)
		return
	}

	indx := len(w.buf)
	w.AppendBytes32(b)
	w.reduceBinary(indx)
}

// PutUint64Array adds a list or vector of uint64 values, packed, and reduces
// it. With maxCapacity the result carries a length mixin.
func (w *Wrapper) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	indx := len(w.buf)
	for _, i := range b {
		w.AppendUint64(i)
	}

	if len(maxCapacity) == 0 {
		w.reduceBinary(indx)
		return
	}
	numItems := uint64(len(b))
	limit := sszutils.CalculateLimit(maxCapacity[0], numItems, 8)
	w.reduceBinaryWithMixin(indx, numItems, limit)
}

// PutBitlist adds a bitlist (with sentinel bit) and reduces its chunks with a
// length mixin. maxSize sets the tree limit.
func (w *Wrapper) PutBitlist(bb []byte, maxSize uint64) {
	var size uint64
	w.tmp, size = hasher.ParseBitlist(w.tmp[:0], bb)
	bitlist := w.tmp
	w.tmp = w.tmp[:cap(w.tmp)]

	indx := len(w.buf)
	w.AppendBytes32(bitlist)
	w.reduceBinaryWithMixin(indx, size, sszutils.CalculateBitlistLimit(maxSize))
}

// PutProgressiveBitlist adds a bitlist (with sentinel bit) and reduces its
// chunks with the progressive algorithm and a length mixin.
func (w *Wrapper) PutProgressiveBitlist(bb []byte) {
	var size uint64
	// Use the progressive parse so the content keeps all-zero top chunks: the
	// chunk count defines the progressive tree shape.
	w.tmp, size = hasher.ParseProgressiveBitlist(w.tmp[:0], bb)
	bitlist := w.tmp
	w.tmp = w.tmp[:cap(w.tmp)]

	indx := len(w.buf)
	w.AppendBytes32(bitlist)
	w.reduceProgressiveWithMixin(indx, size)
}

// Merkleize reduces the region that began at indx into one subtree.
func (w *Wrapper) Merkleize(indx int) {
	indx = w.clampIndex(indx)
	w.checkShape(indx, false)
	w.closeScope(indx)
	w.reduceBinary(indx)
}

// MerkleizeWithMixin reduces the region that began at indx with a length
// mixin, used for SSZ list merkleization. num and limit are uint64 so a list
// capacity above the platform int range hashes correctly instead of panicking.
func (w *Wrapper) MerkleizeWithMixin(indx int, num, limit uint64) {
	indx = w.clampIndex(indx)
	w.checkShape(indx, false)
	w.closeScope(indx)
	w.reduceBinaryWithMixin(indx, num, limit)
}

// MerkleizeProgressive reduces the region that began at indx with the
// progressive algorithm.
func (w *Wrapper) MerkleizeProgressive(indx int) {
	indx = w.clampIndex(indx)
	w.checkShape(indx, true)
	w.closeScope(indx)
	w.reduceProgressive(indx)
}

// MerkleizeProgressiveWithMixin reduces the region progressively with a length
// mixin.
func (w *Wrapper) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	indx = w.clampIndex(indx)
	w.checkShape(indx, true)
	w.closeScope(indx)
	w.reduceProgressiveWithMixin(indx, num)
}

// MerkleizeProgressiveWithActiveFields reduces the region progressively and
// mixes in an active-fields bitvector.
func (w *Wrapper) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	indx = w.clampIndex(indx)
	w.checkShape(indx, true)
	w.closeScope(indx)
	leaves := w.regionLeaves(indx)
	res, err := treeFromNodesProgressiveWithActiveFields(leaves, activeFields)
	if err != nil {
		panic(err)
	}
	w.setRegionNode(indx, res)
}

// --- region reduction ---

// regionLeaves pads the region that began at indx and cuts it into leaves: a
// chunk that begins where a subtree root sits is that subtree, every other
// chunk is a leaf of those bytes. This is the only place bytes become leaves.
func (w *Wrapper) regionLeaves(indx int) []*node {
	w.fillRegionUpTo32(indx)
	if len(w.buf) == indx {
		return nil
	}

	leaves := make([]*node, 0, (len(w.buf)-indx)/32)
	// The registry is ordered by offset and the region is walked in order, so
	// one cursor covers it: no search per chunk.
	i := sort.Search(len(w.nodes), func(i int) bool { return w.nodes[i].off+32 > indx })
	byteLeaves := 0
	for off := indx; off < len(w.buf); off += 32 {
		for i < len(w.nodes) && w.nodes[i].off+32 <= off {
			i++
		}
		if i < len(w.nodes) && w.nodes[i].off == off {
			leaves = append(leaves, w.nodes[i].node)
			i++
			continue
		}
		// A chunk that is not a subtree root may still overlap one, when a
		// region began off a chunk boundary; its bytes are what the hasher
		// reduces there, so they have to be in the buffer.
		for j := i; j < len(w.nodes) && w.nodes[j].off < off+32; j++ {
			w.materializeOne(j)
		}
		leaves = append(leaves, &pendingLeaf)
		byteLeaves++
	}

	// Leaves of the region's own bytes are allocated as one block, as
	// treeFromNodesToDepth does for branches: a node held on its own keeps its
	// block alive.
	if byteLeaves > 0 {
		block := make([]node, byteLeaves)
		for k := range leaves {
			if leaves[k] == &pendingLeaf {
				off := indx + k*32
				initLeaf(&block[0], w.buf[off:off+32])
				leaves[k] = &block[0]
				block = block[1:]
			}
		}
	}
	return leaves
}

// pendingLeaf marks, while regionLeaves walks a region, a chunk that becomes a
// leaf of its own bytes. regionLeaves replaces every mark before returning.
var pendingLeaf node

// setRegionNode replaces the region that began at indx with res: the region's
// bytes and subtree roots are gone, and res occupies one chunk at indx.
func (w *Wrapper) setRegionNode(indx int, res *node) {
	w.truncateNodes(indx)
	w.buf = append(w.buf[:indx], zeroChunk[:]...)
	w.nodes = append(w.nodes, nodeRef{off: indx, node: res})
}

func (w *Wrapper) reduceBinary(indx int) {
	leaves := w.regionLeaves(indx)
	res, err := treeFromNodesToDepthFn(leaves, chunkLimitDepth(uint64(len(leaves))))
	if err != nil {
		panic(err)
	}
	w.setRegionNode(indx, res)
}

func (w *Wrapper) reduceBinaryWithMixin(indx int, num, limit uint64) {
	leaves := w.regionLeaves(indx)
	w.checkChunkLimit(uint64(len(leaves)), limit)
	res, err := treeFromNodesWithMixin64(leaves, num, limit)
	if err != nil {
		panic(err)
	}
	w.setRegionNode(indx, res)
}

func (w *Wrapper) reduceProgressive(indx int) {
	leaves := w.regionLeaves(indx)
	res, err := treeFromNodesProgressive(leaves)
	if err != nil {
		panic(err)
	}
	w.setRegionNode(indx, res)
}

func (w *Wrapper) reduceProgressiveWithMixin(indx int, num uint64) {
	leaves := w.regionLeaves(indx)
	res, err := treeFromNodesProgressiveWithMixin64(leaves, num)
	if err != nil {
		panic(err)
	}
	w.setRegionNode(indx, res)
}

// --- subtree bookkeeping ---

// nodeIndexAt returns the index of the subtree root that begins at off, or -1.
func (w *Wrapper) nodeIndexAt(off int) int {
	i := sort.Search(len(w.nodes), func(i int) bool { return w.nodes[i].off >= off })
	if i < len(w.nodes) && w.nodes[i].off == off {
		return i
	}
	return -1
}

// truncateNodes drops every subtree root whose chunk reaches off or beyond,
// because the bytes from off on are about to be replaced. A root that begins
// below off keeps only the bytes below it, so it is written into the buffer
// first and then dropped: what is left of it is bytes, not a subtree.
func (w *Wrapper) truncateNodes(off int) {
	i := sort.Search(len(w.nodes), func(i int) bool { return w.nodes[i].off+32 > off })
	if i < len(w.nodes) && w.nodes[i].off < off {
		w.materializeOne(i)
	}
	w.nodes = w.nodes[:i]
}

// materialize writes the root bytes of every subtree overlapping [from, to)
// into the buffer, so a reader of those bytes sees what the hasher holds
// there. A subtree is hashed at most once.
func (w *Wrapper) materialize(from, to int) {
	i := sort.Search(len(w.nodes), func(i int) bool { return w.nodes[i].off+32 > from })
	for ; i < len(w.nodes) && w.nodes[i].off < to; i++ {
		w.materializeOne(i)
	}
}

// materializeOne writes one subtree's root bytes into the chunk it occupies.
func (w *Wrapper) materializeOne(i int) {
	if w.nodes[i].filled || w.nodes[i].node == nil {
		return
	}
	// A subtree the backend refused has no root to write, so the refusal is
	// recorded here where the walk can report it and the chunk is left
	// unfilled: nothing was cached, so a later call answers it once the
	// backend does.
	h := hashingOf(w.Algorithm())
	if err := w.nodes[i].node.finalize(&h); err != nil {
		w.setWalkErr(err)

		return
	}
	copy(w.buf[w.nodes[i].off:w.nodes[i].off+32], nodeValue(w.nodes[i].node, &h))
	w.nodes[i].filled = true
}

// clampIndex bounds a caller-supplied scope index to the buffer so an
// out-of-range value cannot trigger a slice-bounds panic, matching
// hasher.Hasher.
func (w *Wrapper) clampIndex(i int) int {
	if i < 0 {
		return 0
	}
	if i > len(w.buf) {
		return len(w.buf)
	}
	return i
}

// --- terminal state ---

// rootNode returns the single subtree the walker has left, or an error naming
// what is missing. The conditions are hasher.Hasher's: no scope may be open
// and the buffer must hold exactly one chunk.
func (w *Wrapper) rootNode() (*node, error) {
	if w.walkErr != nil {
		return nil, w.walkErr
	}
	if len(w.scopes) > 0 {
		return nil, fmt.Errorf("unfinished hashing scopes")
	}
	if len(w.buf) != 32 {
		return nil, fmt.Errorf("incomplete merkleization: wrapper holds %d bytes, want 32", len(w.buf))
	}
	if i := w.nodeIndexAt(0); i >= 0 {
		return w.nodes[i].node, nil
	}
	return leafFromBytes(w.buf), nil
}

// Node returns the root of the constructed tree. It panics when the
// merkleization is incomplete.
func (w *Wrapper) Node() *Node {
	n, err := w.rootNode()
	if err != nil {
		panic(err.Error())
	}
	return &Node{n: n, alg: w.Algorithm()}
}

// Root returns the root of the constructed tree.
func (w *Wrapper) Root() (*Node, error) {
	n, err := w.rootNode()
	if err != nil {
		return nil, err
	}
	return &Node{n: n, alg: w.Algorithm()}, nil
}

// HashRoot returns the 32-byte hash tree root of the constructed tree.
func (w *Wrapper) HashRoot() ([32]byte, error) {
	n, err := w.rootNode()
	if err != nil {
		return [32]byte{}, err
	}
	h := hashingOf(w.Algorithm())
	if err := n.finalize(&h); err != nil {
		w.setWalkErr(err)

		return [32]byte{}, err
	}

	return [32]byte(nodeValue(n, &h)), nil
}

// Hash returns the last chunk of the walker's state -- the latest reduction
// result, or the bytes appended since -- as hasher.Hasher.Hash does. The
// returned slice is only valid until the next walker operation.
//
// Materializing the chunk performs the subtree's reduction, so the backend can
// refuse it here and at no earlier call: the chunk is a root only while
// HashErr, read after Hash, is nil.
func (w *Wrapper) Hash() []byte {
	start := 0
	if len(w.buf) > 32 {
		start = len(w.buf) - 32
	}
	w.materialize(start, len(w.buf))
	return w.buf[start:]
}
