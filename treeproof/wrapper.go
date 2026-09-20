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
	// hashFn compresses this tree, where a caller gave one; the package
	// default serves the rest.
	hashFn hasher.HashFn

	// walkErr records the first failure of the walk: a scope reduced in a
	// shape it was not opened for, a reduction handed more chunks than its
	// limit holds, or a hash function that refused. The
	// walkers have to answer alike, and hasher.Hasher refuses the same thing.
	walkErr error
}

// nodeRef is a subtree root and the 32 bytes of buf it occupies.
type nodeRef struct {
	off  int
	node *Node
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

// NewWrapper creates a new Wrapper ready to construct a Merkle tree.
//
// It names no backend, so its trees compress with whatever
// hasher.FastHasherPool holds at the time -- the built-in compression unless a
// caller installed one. hasher.NewHasher takes the built-in whatever the pool
// holds, so a caller who installs a non-sha256 backend on the pool and reads
// one walker against the other names it on both: NewWrapperWithHashFn here,
// hasher.NewHasherWithHashFn there.
func NewWrapper() *Wrapper {
	return &Wrapper{
		buf: make([]byte, 0),
		tmp: make([]byte, 64),
	}
}

// NewWrapperWithHashFn returns a Wrapper that compresses with fn, so a tree
// answers with the same root the hasher gives for the same value: a caller
// that installs a backend installs it for both walkers or for neither.
func NewWrapperWithHashFn(fn hasher.HashFn) *Wrapper {
	w := NewWrapper()
	w.hashFn = fn

	return w
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
	res, err := TreeFromNodesProgressiveWithActiveFields(leaves, activeFields)
	if err != nil {
		panic(err)
	}
	w.setRegionNode(indx, res)
}

// --- region reduction ---

// regionLeaves pads the region that began at indx and cuts it into leaves: a
// chunk that begins where a subtree root sits is that subtree, every other
// chunk is a leaf of those bytes. This is the only place bytes become leaves.
func (w *Wrapper) regionLeaves(indx int) []*Node {
	w.fillRegionUpTo32(indx)
	if len(w.buf) == indx {
		return nil
	}

	leaves := make([]*Node, 0, (len(w.buf)-indx)/32)
	// The registry is ordered by offset and the region is walked in order, so
	// one cursor covers it: no search per chunk.
	i := sort.Search(len(w.nodes), func(i int) bool { return w.nodes[i].off+32 > indx })
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
		leaves = append(leaves, LeafFromBytes(w.buf[off:off+32]))
	}
	return leaves
}

// setRegionNode replaces the region that began at indx with res: the region's
// bytes and subtree roots are gone, and res occupies one chunk at indx.
func (w *Wrapper) setRegionNode(indx int, res *Node) {
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
	res, err := TreeFromNodesProgressive(leaves)
	if err != nil {
		panic(err)
	}
	w.setRegionNode(indx, res)
}

func (w *Wrapper) reduceProgressiveWithMixin(indx int, num uint64) {
	leaves := w.regionLeaves(indx)
	res, err := TreeFromNodesProgressiveWithMixin64(leaves, num)
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
	if err := w.nodes[i].node.finalize(finalizeConfig{fn: w.hashFn}); err != nil {
		w.setWalkErr(err)

		return
	}
	copy(w.buf[w.nodes[i].off:w.nodes[i].off+32], w.nodes[i].node.Hash())
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

// --- legacy node API ---

// alignForNode pads to the next chunk boundary of the enclosing scope, so a
// node added through the legacy Add* API occupies a chunk of its own and keeps
// its identity as a leaf. hasher.Hasher has no counterpart to these methods,
// so nothing constrains this to the byte layout of a value write.
func (w *Wrapper) alignForNode() {
	off := 0
	if n := len(w.scopes); n > 0 {
		off = w.scopes[n-1].off
	}
	w.fillRegionUpTo32(off)
}

// AddNode places a node at the next chunk boundary, where it occupies one
// chunk.
func (w *Wrapper) AddNode(n *Node) {
	w.alignForNode()
	off := len(w.buf)
	w.buf = append(w.buf, zeroChunk[:]...)
	w.nodes = append(w.nodes, nodeRef{off: off, node: n})
}

// AddUint64 adds a uint64 as a leaf node.
func (w *Wrapper) AddUint64(i uint64) {
	w.AddNode(LeafFromUint64(i))
}

// AddUint32 adds a uint32 as a leaf node.
func (w *Wrapper) AddUint32(i uint32) {
	w.AddNode(LeafFromUint32(i))
}

// AddUint16 adds a uint16 as a leaf node.
func (w *Wrapper) AddUint16(i uint16) {
	w.AddNode(LeafFromUint16(i))
}

// AddUint8 adds a uint8 as a leaf node.
func (w *Wrapper) AddUint8(i uint8) {
	w.AddNode(LeafFromUint8(i))
}

// AddBytes adds a byte slice as a leaf node (<=32 bytes) or as a subtree of
// its chunks. An empty value is a leaf of zeros, the meaning this method has
// carried since v1: it builds a tree from values, so every value it is given
// takes a place in the leaf order. PutBytes, the walker operation, is the one
// that contributes nothing for an empty value, because there the value writes
// bytes into a region.
func (w *Wrapper) AddBytes(b []byte) {
	if len(b) <= 32 {
		w.AddNode(LeafFromBytes(b))
		return
	}
	w.alignForNode()
	indx := len(w.buf)
	w.AppendBytes32(b)
	w.reduceBinary(indx)
}

// addEmpty adds an empty (all-zeros) leaf node.
func (w *Wrapper) addEmpty() {
	w.AddNode(EmptyLeaf())
}

// The methods below preserve the exported v1.x Wrapper API (Commit*/AddEmpty)
// as thin aliases over the current Merkleize* methods, so consumers built
// against earlier v1 releases keep compiling. New code should use Merkleize*.

// Commit reduces the region that began at index i into a binary subtree.
//
// Deprecated: use Merkleize.
func (w *Wrapper) Commit(i int) { w.Merkleize(i) }

// CommitWithMixin reduces the region that began at index i with a length mixin.
//
// Deprecated: use MerkleizeWithMixin.
func (w *Wrapper) CommitWithMixin(i, num, limit int) {
	w.MerkleizeWithMixin(i, uint64(num), uint64(limit))
}

// CommitProgressive reduces the region that began at index i progressively.
//
// Deprecated: use MerkleizeProgressive.
func (w *Wrapper) CommitProgressive(i int) { w.MerkleizeProgressive(i) }

// CommitProgressiveWithMixin reduces the region that began at index i
// progressively, with a length mixin.
//
// Deprecated: use MerkleizeProgressiveWithMixin.
func (w *Wrapper) CommitProgressiveWithMixin(i, num int) {
	w.MerkleizeProgressiveWithMixin(i, uint64(num))
}

// CommitProgressiveWithActiveFields reduces the region that began at index i
// progressively, with an active-fields bitvector mixin.
//
// Deprecated: use MerkleizeProgressiveWithActiveFields.
func (w *Wrapper) CommitProgressiveWithActiveFields(i int, activeFields []byte) {
	w.MerkleizeProgressiveWithActiveFields(i, activeFields)
}

// AddEmpty adds an empty (all-zeros) leaf node.
//
// Deprecated: retained for v1 API compatibility.
func (w *Wrapper) AddEmpty() { w.addEmpty() }

// --- terminal state ---

// rootNode returns the single subtree the walker has left, or an error naming
// what is missing. The conditions are hasher.Hasher's: no scope may be open
// and the buffer must hold exactly one chunk.
func (w *Wrapper) rootNode() (*Node, error) {
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
	return LeafFromBytes(w.buf), nil
}

// Node returns the root of the constructed tree. It panics when the
// merkleization is incomplete.
func (w *Wrapper) Node() *Node {
	n, err := w.rootNode()
	if err != nil {
		panic(err.Error())
	}
	return n
}

// Root returns the root of the constructed tree.
func (w *Wrapper) Root() (*Node, error) {
	return w.rootNode()
}

// HashRoot returns the 32-byte hash tree root of the constructed tree.
func (w *Wrapper) HashRoot() ([32]byte, error) {
	n, err := w.rootNode()
	if err != nil {
		return [32]byte{}, err
	}
	if err := n.finalize(finalizeConfig{fn: w.hashFn}); err != nil {
		w.setWalkErr(err)

		return [32]byte{}, err
	}

	return [32]byte(n.Hash()), nil
}

// Hash returns the last chunk of the walker's state -- the latest reduction
// result, or the bytes appended since -- as hasher.Hasher.Hash does. The
// returned slice is only valid until the next walker operation.
func (w *Wrapper) Hash() []byte {
	start := 0
	if len(w.buf) > 32 {
		start = len(w.buf) - 32
	}
	w.materialize(start, len(w.buf))
	return w.buf[start:]
}
