// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// proofHasherStream drives every HashWalker method the engines emit: two
// nested scopes, one Index-opened scope, chunk appends of all widths, and
// the Put-style subtree calls.
func proofHasherStream(hh sszutils.HashWalker) {
	root := hh.StartTree(sszutils.TreeTypeNone)

	// child 0: a scope with mixed appends
	inner := hh.StartTree(sszutils.TreeTypeNone)
	hh.AppendUint64(1)
	hh.AppendUint32(2)
	hh.AppendUint16(3)
	hh.AppendUint8(4)
	hh.AppendBool(true)
	hh.FillUpTo32()
	hh.Append([]byte{9, 9})
	hh.FillUpTo32()
	hh.AppendBytes32([]byte{5, 6, 7})
	hh.Merkleize(inner)

	// child 1: an Index-opened scope (legacy style)
	legacy := hh.Index()
	hh.PutUint64(11)
	hh.PutUint32(12)
	hh.Merkleize(legacy)

	// children 2..8: single-chunk puts and subtree puts
	hh.PutBool(true)
	hh.PutUint8(21)
	hh.PutUint16(22)
	hh.PutUint64(23)
	hh.PutBytes([]byte{1, 2, 3})
	hh.PutBytes(bytes.Repeat([]byte{7}, 96))
	hh.PutBitlist([]byte{0xaa, 0x01}, 64)

	// child 9: a list scope with a mixin; elements carry a subtree put so
	// muted regions exercise the subtree-put path too
	list := hh.StartTree(sszutils.TreeTypeBinary)
	for i := range 5 {
		elem := hh.StartTree(sszutils.TreeTypeNone)
		hh.PutUint64(uint64(100 + i))
		hh.PutUint64(uint64(200 + i))
		hh.PutBitlist([]byte{byte(i), 0x01}, 32)
		hh.Merkleize(elem)
		hh.Collapse()
	}
	hh.MerkleizeWithMixin(list, 5, 8)

	// child 10: a progressive scope (retained whole by any schedule) with a
	// nested plain-progressive subscope
	prog := hh.StartTree(sszutils.TreeTypeProgressive)
	nested := hh.StartTree(sszutils.TreeTypeProgressive)
	hh.PutUint64(30)
	hh.MerkleizeProgressive(nested)
	hh.PutUint64(31)
	hh.PutUint64(32)
	hh.PutUint64(33)
	hh.MerkleizeProgressiveWithMixin(prog, 3)

	// child 11: a uint64 array subtree and a root vector subtree
	hh.PutUint64Array([]uint64{41, 42, 43}, 16)
	roots := [][]byte{bytes.Repeat([]byte{3}, 32), bytes.Repeat([]byte{4}, 32)}
	if rv, ok := hh.(interface {
		PutRootVector(b [][]byte, maxCapacity ...uint64) error
	}); ok {
		if err := rv.PutRootVector(roots, 8); err != nil {
			panic(err)
		}
	} else {
		// The tree wrapper has no PutRootVector; replay the same structure.
		idx := hh.Index()
		for _, r := range roots {
			hh.AppendBytes32(r)
		}
		hh.MerkleizeWithMixin(idx, uint64(len(roots)), sszutils.CalculateLimit(8, uint64(len(roots)), 32))
	}

	// children 12..13: a progressive bitlist subtree and an uncapped root
	// vector subtree
	hh.PutProgressiveBitlist([]byte{0x3c, 0x01})
	roots2 := [][]byte{bytes.Repeat([]byte{5}, 32)}
	if rv, ok := hh.(interface {
		PutRootVector(b [][]byte, maxCapacity ...uint64) error
	}); ok {
		if err := rv.PutRootVector(roots2); err != nil {
			panic(err)
		}
	} else {
		idx := hh.Index()
		hh.AppendBytes32(roots2[0])
		hh.Merkleize(idx)
	}

	hh.Merkleize(root)
}

// runProofHasher drives the stream through a ProofHasher with the given
// schedule and returns the pruned tree.
func runProofHasher(t *testing.T, sched *ProofSchedule) *Node {
	t.Helper()
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	ph := NewProofHasher(sched, hh, nil)
	proofHasherStream(ph)
	tree, err := ph.ProofTree()
	if err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	return tree
}

// The pruned tree must reproduce the full wrapper tree's root and proofs for
// every retained path, across all capture states: scheduled scopes, muted
// subtrees, retained subtrees, and Put-style subtree children.
func TestProofHasherStreamParity(t *testing.T) {
	ref := NewWrapper()
	proofHasherStream(ref)
	refTree := ref.Node()
	refRoot := refTree.Hash()

	var gindices []int
	collectStreamGindices(refTree, 1, &gindices)

	// Retain everything: the pruned tree must serve every proof of the full
	// tree.
	full := runProofHasher(t, retainAllSchedule)
	if !bytes.Equal(full.Hash(), refRoot) {
		t.Fatalf("retainAll root = %x, want %x", full.Hash(), refRoot)
	}
	for _, gindex := range gindices {
		want, refErr := refTree.Prove(gindex)
		got, err := full.Prove(gindex)
		if refErr != nil {
			if err == nil {
				t.Fatalf("gindex %d: reference errored (%v), pruned did not", gindex, refErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("gindex %d: %v", gindex, err)
		}
		assertSameProof(t, gindex, want, got)
	}

	// A partial schedule: descend into the mixed-append scope (child 0), the
	// bitlist subtree (child 8, via retainAll from a manual schedule), and
	// list element 3 (child 9 -> chunks -> slot 3). Everything else runs
	// muted through the hasher.
	sched := &ProofSchedule{children: map[uint64]*ProofSchedule{
		0:  {children: map[uint64]*ProofSchedule{1: {}}},
		7:  {children: map[uint64]*ProofSchedule{1: {}}},
		8:  {retainAll: true},
		9:  {children: map[uint64]*ProofSchedule{3: {children: map[uint64]*ProofSchedule{0: {}}}}},
		11: {retainAll: true},
		12: {retainAll: true},
		13: {retainAll: true},
	}}
	pruned := runProofHasher(t, sched)
	if !bytes.Equal(pruned.Hash(), refRoot) {
		t.Fatalf("pruned root = %x, want %x", pruned.Hash(), refRoot)
	}
	// The scheduled paths serve the same proofs as the full tree.
	scopeG := 16 + 0 // child 0 of the 14-child (16-slot) root scope
	bitlistG := 16 + 8
	listElemG := (16+9)*2*8 + 3
	putBytesG := 16 + 7
	arrayG := 16 + 11
	progBitsG := 16 + 12
	rootVecG := 16 + 13
	for _, gindex := range []int{1, scopeG, scopeG*2 + 1, bitlistG, bitlistG * 2, bitlistG*2 + 1, listElemG, listElemG * 2,
		putBytesG * 2, putBytesG*2 + 1, arrayG * 2, arrayG*2 + 1, progBitsG * 2, rootVecG * 2} {
		want, refErr := refTree.Prove(gindex)
		if refErr != nil {
			t.Fatalf("reference Prove(%d): %v", gindex, refErr)
		}
		got, err := pruned.Prove(gindex)
		if err != nil {
			t.Fatalf("pruned Prove(%d): %v", gindex, err)
		}
		assertSameProof(t, gindex, want, got)
	}
}

func collectStreamGindices(node *Node, gindex int, out *[]int) {
	if node == nil || gindex > 1<<20 {
		return
	}
	*out = append(*out, gindex)
	collectStreamGindices(node.left, gindex*2, out)
	collectStreamGindices(node.right, gindex*2+1, out)
}

func assertSameProof(t *testing.T, gindex int, want, got *Proof) {
	t.Helper()
	if !bytes.Equal(got.Leaf, want.Leaf) {
		t.Fatalf("gindex %d: leaf = %x, want %x", gindex, got.Leaf, want.Leaf)
	}
	if len(got.Hashes) != len(want.Hashes) {
		t.Fatalf("gindex %d: %d hashes, want %d", gindex, len(got.Hashes), len(want.Hashes))
	}
	for i := range got.Hashes {
		if !bytes.Equal(got.Hashes[i], want.Hashes[i]) {
			t.Fatalf("gindex %d: hash %d mismatch", gindex, i)
		}
	}
}

// ProofTree reports the walk's failure modes: an unclosed scope, a stream
// that never produced a root, and a shadow diverging from the hasher root.
func TestProofHasherProofTreeErrors(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)

	// No content at all: HashRoot fails.
	ph := NewProofHasher(&ProofSchedule{}, hh, nil)
	if _, err := ph.ProofTree(); err == nil {
		t.Fatal("expected error for empty stream")
	}

	// A bare chunk without any scope: served as a leaf.
	hh.Reset()
	ph = NewProofHasher(&ProofSchedule{}, hh, nil)
	ph.PutUint64(7)
	tree, err := ph.ProofTree()
	if err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	if !tree.IsLeaf() {
		t.Fatal("expected leaf tree for bare chunk")
	}

	// A corrupted captured node diverges from the hasher root.
	hh.Reset()
	ph = NewProofHasher(&ProofSchedule{}, hh, nil)
	idx := ph.StartTree(sszutils.TreeTypeNone)
	ph.PutUint64(7)
	ph.PutUint64(8)
	ph.Merkleize(idx)
	ph.scopes[0].childNodes[0].value[0] ^= 0xff
	if _, err := ph.ProofTree(); err == nil {
		t.Fatal("expected divergence error for corrupted capture")
	}
}

// rangeRoot pads odd tails and missing levels with zero hashes and falls
// back to pairwise hashing when the batched backend fails.
func TestProofHasherRangeRoot(t *testing.T) {
	ph := &ProofHasher{}
	shadow := bytes.Repeat([]byte{1}, 3*32)

	// count 1 with extra levels: zero-hash chain.
	root := ph.rangeRoot(shadow, 2, 3, 3)
	want := hashPair(hashPair(hashPair(shadow[64:96], hasher.GetZeroHash(0)), hasher.GetZeroHash(1)), hasher.GetZeroHash(2))
	if !bytes.Equal(root[:], want) {
		t.Fatalf("rangeRoot = %x, want %x", root, want)
	}

	// The pairwise fallback must produce the same root as the backend.
	viaBackend := ph.rangeRoot(shadow, 0, 2, 3)
	broken := &ProofHasher{}
	origFn := hasher.FastHasherPool.HashFn
	hasher.FastHasherPool.HashFn = func(_, _ []byte) error { return errors.New("backend failure") }
	viaFallback := broken.rangeRoot(shadow, 0, 2, 3)
	hasher.FastHasherPool.HashFn = origFn
	if viaBackend != viaFallback {
		t.Fatalf("fallback root %x != backend root %x", viaFallback, viaBackend)
	}
}

// A scheduled scope closed by a progressive variant means the stream and
// the descriptor-derived schedule disagree; the wrapper replay keeps the
// pruned tree correct for all three progressive variants.
func TestProofHasherProgressiveMismatch(t *testing.T) {
	variants := []struct {
		name  string
		close func(hh sszutils.HashWalker, idx int)
	}{
		{"plain", func(hh sszutils.HashWalker, idx int) { hh.MerkleizeProgressive(idx) }},
		{"mixin", func(hh sszutils.HashWalker, idx int) { hh.MerkleizeProgressiveWithMixin(idx, 3) }},
		{"active fields", func(hh sszutils.HashWalker, idx int) { hh.MerkleizeProgressiveWithActiveFields(idx, []byte{0x07}) }},
	}

	for _, tt := range variants {
		t.Run(tt.name, func(t *testing.T) {
			stream := func(hh sszutils.HashWalker) {
				root := hh.StartTree(sszutils.TreeTypeNone)
				child := hh.StartTree(sszutils.TreeTypeNone)
				hh.PutUint64(1)
				hh.PutUint64(2)
				hh.Merkleize(child)
				hh.PutUint64(3)
				hh.PutUint64(4)
				tt.close(hh, root)
			}

			ref := NewWrapper()
			stream(ref)
			refTree := ref.Node()
			refRoot := refTree.Hash()

			hh := hasher.FastHasherPool.Get()
			defer hasher.FastHasherPool.Put(hh)
			// The root scope is scheduled with a retained child, but the
			// stream closes it progressively.
			ph := NewProofHasher(&ProofSchedule{children: map[uint64]*ProofSchedule{0: {children: map[uint64]*ProofSchedule{0: {}}}}}, hh, nil)
			stream(ph)
			tree, err := ph.ProofTree()
			if err != nil {
				t.Fatalf("ProofTree: %v", err)
			}
			if !bytes.Equal(tree.Hash(), refRoot) {
				t.Fatalf("root = %x, want %x", tree.Hash(), refRoot)
			}
			want, refErr := refTree.Prove(2)
			if refErr != nil {
				t.Fatalf("reference Prove: %v", refErr)
			}
			got, err := tree.Prove(2)
			if err != nil {
				t.Fatalf("pruned Prove: %v", err)
			}
			assertSameProof(t, 2, want, got)
		})
	}
}

// PutRootVector propagates the hasher's validation error.
func TestProofHasherPutRootVectorError(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	ph := NewProofHasher(&ProofSchedule{}, hh, nil)
	if err := ph.PutRootVector([][]byte{{1, 2}}); err == nil {
		t.Fatal("expected error for malformed root")
	}
	hh.Reset()
}

// A stream that bypasses the proof capture entirely still serves its root
// as a single leaf: the buffer holds the truth.
func TestProofHasherProofTreeBypassedStream(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	ph := NewProofHasher(&ProofSchedule{}, hh, nil)
	ph.Hasher.PutUint64(7)
	tree, err := ph.ProofTree()
	if err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	if !tree.IsLeaf() {
		t.Fatal("expected leaf tree for bypassed stream")
	}
}

// rangeRoot serves fully padded ranges from the zero-hash table.
func TestProofHasherRangeRootPadding(t *testing.T) {
	ph := &ProofHasher{}
	root := ph.rangeRoot(nil, 4, 2, 2)
	if !bytes.Equal(root[:], hasher.GetZeroHash(2)) {
		t.Fatalf("padded rangeRoot = %x, want zero hash", root)
	}
}

// alignRegion pads a trailing partial chunk; aligned regions pass through
// untouched.
func TestProofHasherAlignRegion(t *testing.T) {
	ph := &ProofHasher{}
	aligned := bytes.Repeat([]byte{1}, 64)
	if got := ph.alignRegion(aligned); &got[0] != &aligned[0] {
		t.Fatal("aligned region must pass through without copying")
	}
	got := ph.alignRegion([]byte{1, 2, 3})
	if len(got) != 32 || got[0] != 1 || got[3] != 0 || got[31] != 0 {
		t.Fatalf("misaligned region not zero-padded: %x", got)
	}
}

// A Merkleize on the base level without a tracked scope forwards untouched.
func TestProofHasherBaseMerkleize(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	ph := NewProofHasher(&ProofSchedule{}, hh, nil)
	ph.PutUint64(1)
	ph.PutUint64(2)
	ph.Merkleize(0)
	tree, err := ph.ProofTree()
	if err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	if !tree.IsLeaf() {
		t.Fatal("expected leaf tree")
	}
}
