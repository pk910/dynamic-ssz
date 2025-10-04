// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code derived from https://github.com/ferranbt/fastssz/blob/v1.0.0/wrapper.go
// Copyright (c) 2020 Ferran Borreguero
// Licensed under the MIT License
// The code has been modified for dynamic-ssz proof generation needs.

package treeproof

import (
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ sszutils.HashWalker = (*Wrapper)(nil)

// ProofTree hashes a HashRoot object with a Hasher from
// the default HasherPool
func ProofTree(ds *dynssz.DynSsz, v any) (*Node, error) {
	w := &Wrapper{
		tmp: make([]byte, 64),
	}
	if err := ds.HashTreeRootWith(v, w); err != nil {
		return nil, err
	}
	return w.Node(), nil
}

type Wrapper struct {
	nodes []*Node
	buf   []byte
	tmp   []byte
}

/// --- wrapper implements the HashWalker interface ---

func (w *Wrapper) WithTemp(fn func(tmp []byte) []byte) {
	w.tmp = fn(w.tmp)
}

func (w *Wrapper) Index() int {
	return len(w.nodes)
}

func (w *Wrapper) Append(i []byte) {
	w.buf = append(w.buf, i...)
}

func (w *Wrapper) AppendUint64(i uint64) {
	w.buf = sszutils.MarshalUint64(w.buf, i)
}

func (w *Wrapper) AppendUint32(i uint32) {
	w.buf = sszutils.MarshalUint32(w.buf, i)
}

func (w *Wrapper) AppendUint16(i uint16) {
	w.buf = sszutils.MarshalUint16(w.buf, i)
}

func (w *Wrapper) AppendUint8(i uint8) {
	w.buf = sszutils.MarshalUint8(w.buf, i)
}

func (w *Wrapper) AppendBool(b bool) {
	w.buf = sszutils.MarshalBool(w.buf, b)
}

func (w *Wrapper) AppendBytes32(b []byte) {
	w.buf = append(w.buf, b...)
	w.FillUpTo32()
}

func (w *Wrapper) FillUpTo32() {
	// pad zero bytes to the left
	if rest := len(w.buf) % 32; rest != 0 {
		w.buf = sszutils.AppendZeroPadding(w.buf, 32-rest)
	}
}

func (w *Wrapper) Merkleize(indx int) {
	if len(w.buf) != 0 {
		w.appendBytesAsNodes(w.buf)
		w.buf = w.buf[:0]
	}
	w.Commit(indx)
}

func (w *Wrapper) MerkleizeWithMixin(indx int, num, limit uint64) {
	if len(w.buf) != 0 {
		w.appendBytesAsNodes(w.buf)
		w.buf = w.buf[:0]
	}
	w.CommitWithMixin(indx, int(num), int(limit))
}

func (w *Wrapper) MerkleizeProgressive(indx int) {
	if len(w.buf) != 0 {
		w.appendBytesAsNodes(w.buf)
		w.buf = w.buf[:0]
	}
	w.CommitProgressive(indx)
}

func (w *Wrapper) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	if len(w.buf) != 0 {
		w.appendBytesAsNodes(w.buf)
		w.buf = w.buf[:0]
	}
	w.CommitProgressiveWithMixin(indx, int(num))
}

func (w *Wrapper) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	if len(w.buf) != 0 {
		w.appendBytesAsNodes(w.buf)
		w.buf = w.buf[:0]
	}
	w.CommitProgressiveWithActiveFields(indx, activeFields)
}

func (w *Wrapper) PutBitlist(bb []byte, maxSize uint64) {
	b, size := hasher.ParseBitlist(w.tmp[:0], bb)
	w.tmp = b

	indx := w.Index()
	w.appendBytesAsNodes(b)
	w.CommitWithMixin(indx, int(size), int((maxSize+255)/256))
}

func (w *Wrapper) PutProgressiveBitlist(bb []byte) {
	b, size := hasher.ParseBitlist(w.tmp[:0], bb)
	w.tmp = b

	indx := w.Index()
	w.appendBytesAsNodes(b)
	w.CommitProgressiveWithMixin(indx, int(size))
}

func (w *Wrapper) appendBytesAsNodes(b []byte) {
	// if byte list is empty, fill with zeros
	if len(b) == 0 {
		b = sszutils.AppendZeroPadding(b, 32)
	}
	// if byte list isn't filled with 32-bytes padded, pad
	if rest := len(b) % 32; rest != 0 {
		b = sszutils.AppendZeroPadding(b, 32-rest)
	}
	for i := 0; i < len(b); i += 32 {
		val := append([]byte{}, b[i:min(len(b), i+32)]...)
		w.nodes = append(w.nodes, LeafFromBytes(val))
	}
}

func (w *Wrapper) PutBool(b bool) {
	w.AddNode(LeafFromBool(b))
}

func (w *Wrapper) PutBytes(b []byte) {
	w.AddBytes(b)
}

func (w *Wrapper) PutUint16(i uint16) {
	w.AddUint16(i)
}

func (w *Wrapper) PutUint64(i uint64) {
	w.AddUint64(i)
}

func (w *Wrapper) PutUint8(i uint8) {
	w.AddUint8(i)
}

func (w *Wrapper) PutUint32(i uint32) {
	w.AddUint32(i)
}

func (w *Wrapper) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	indx := w.Index()
	for _, i := range b {
		w.AppendUint64(i)
	}

	// pad zero bytes to the left
	w.FillUpTo32()

	if len(maxCapacity) == 0 {
		// Array with fixed size
		w.Merkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := sszutils.CalculateLimit(maxCapacity[0], numItems, 8)

		w.MerkleizeWithMixin(indx, numItems, limit)
	}
}

/// --- legacy ones ---

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

func (w *Wrapper) AddBytes(b []byte) {
	if len(b) <= 32 {
		w.AddNode(LeafFromBytes(b))
	} else {
		indx := w.Index()
		w.appendBytesAsNodes(b)
		w.Commit(indx)
	}
}

func (w *Wrapper) AddUint64(i uint64) {
	w.AddNode(LeafFromUint64(i))
}

func (w *Wrapper) AddUint32(i uint32) {
	w.AddNode(LeafFromUint32(i))
}

func (w *Wrapper) AddUint16(i uint16) {
	w.AddNode(LeafFromUint16(i))
}

func (w *Wrapper) AddUint8(i uint8) {
	w.AddNode(LeafFromUint8(i))
}

func (w *Wrapper) AddNode(n *Node) {
	if w.nodes == nil {
		w.nodes = []*Node{}
	}
	w.nodes = append(w.nodes, n)
}

func (w *Wrapper) Node() *Node {
	if len(w.nodes) != 1 {
		panic("BAD")
	}
	return w.nodes[0]
}

func (w *Wrapper) Hash() []byte {
	return w.nodes[len(w.nodes)-1].Hash()
}

func (w *Wrapper) Commit(i int) {
	// create tree from nodes
	res, err := TreeFromNodes(w.nodes[i:], w.getLimit(i))
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

func (w *Wrapper) CommitWithMixin(i, num, limit int) {
	// create tree from nodes
	res, err := TreeFromNodesWithMixin(w.nodes[i:], num, limit)
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]

	// add the new node
	w.AddNode(res)
}

// CommitProgressive creates a progressive tree from nodes
func (w *Wrapper) CommitProgressive(i int) {
	// create progressive tree from nodes
	res, err := TreeFromNodesProgressive(w.nodes[i:])
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

// CommitProgressiveWithMixin creates a progressive tree with length mixin
func (w *Wrapper) CommitProgressiveWithMixin(i, num int) {
	// create progressive tree from nodes
	res, err := TreeFromNodesProgressiveWithMixin(w.nodes[i:], num)
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

// CommitProgressiveWithActiveFields creates a progressive tree with active fields bitvector
func (w *Wrapper) CommitProgressiveWithActiveFields(i int, activeFields []byte) {
	// create progressive tree from nodes
	res, err := TreeFromNodesProgressiveWithActiveFields(w.nodes[i:], activeFields)
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

func (w *Wrapper) AddEmpty() {
	w.AddNode(EmptyLeaf())
}

func (w *Wrapper) getLimit(i int) int {
	size := len(w.nodes[i:])
	return int(sszutils.NextPowerOfTwo(uint64(size)))
}
