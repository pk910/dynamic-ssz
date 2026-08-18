// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz_test

import (
	"bytes"
	"errors"
	"runtime"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
	"github.com/pk910/dynamic-ssz/treeproof"
)

type proofTestInner struct {
	A uint64
	B [32]byte `ssz-size:"32"`
}

type proofTestContainer struct {
	F0  uint64
	F1  [48]byte          `ssz-size:"48"`
	F2  []*proofTestInner `ssz-max:"10"`
	F3  []uint64          `ssz-max:"100"`
	F4  []byte            `ssz-max:"70"`
	F5  [][32]byte        `ssz-size:"4,32"`
	F6  []uint64          `ssz-size:"6"`
	F7  [5]byte           `ssz-size:"5"`
	F8  bool
	F9  []proofTestInner  `ssz-max:"3"`
	F10 []uint16          `ssz-max:"20"`
	F11 []uint32          `ssz-max:"20"`
	F12 []bool            `ssz-max:"20"`
	F13 string            `ssz-max:"64"`
	F14 [2]proofTestInner `ssz-size:"2"`
	F15 *proofTestInner
	F16 [][]byte `ssz-type:"?,uint128" ssz-size:"?,16" ssz-max:"4"`
	F17 [][]byte `ssz-type:"?,uint256" ssz-size:"?,32" ssz-max:"4"`
}

func proofTestValue() *proofTestContainer {
	v := &proofTestContainer{
		F0:  0x1122334455667788,
		F3:  []uint64{9, 8, 7, 6, 5, 4, 3, 2, 1},
		F4:  bytes.Repeat([]byte{0xaa, 0xbb}, 33),
		F6:  []uint64{60, 61, 62, 63, 64, 65},
		F7:  [5]byte{1, 2, 3, 4, 5},
		F8:  true,
		F10: []uint16{10, 11, 12},
		F11: []uint32{20, 21},
		F12: []bool{true, false, true},
		F13: "a string that spans multiple chunks of merkleized data",
	}
	for i := range v.F1 {
		v.F1[i] = byte(i + 1)
	}
	for i := range 3 {
		inner := &proofTestInner{A: uint64(i + 100)}
		for j := range inner.B {
			inner.B[j] = byte(i*32 + j)
		}
		v.F2 = append(v.F2, inner)
	}
	v.F5 = make([][32]byte, 4)
	for i := range v.F5 {
		for j := range v.F5[i] {
			v.F5[i][j] = byte(i + j)
		}
	}
	v.F14[0] = proofTestInner{A: 200}
	v.F14[1] = proofTestInner{A: 201}
	v.F16 = [][]byte{bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 16), bytes.Repeat([]byte{3}, 16)}
	v.F17 = [][]byte{bytes.Repeat([]byte{4}, 32)}
	return v
}

// proofTestSetup builds the reference full proof tree and a proof generator
// for a source value.
func proofTestSetup(t *testing.T, ds *dynssz.DynSsz, source any) (*treeproof.Node, []byte, func(gindices []int) ([]*treeproof.Proof, error)) {
	t.Helper()

	tree, err := ds.GetTree(source)
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	root := tree.Hash()

	getProofs := func(gindices []int) ([]*treeproof.Proof, error) {
		return ds.GetProofs(source, gindices)
	}

	return tree, root, getProofs
}

// collectGindices walks the proof tree and returns every reachable
// generalized index, including zero-padding nodes.
func collectGindices(node *treeproof.Node, gindex int, gindices *[]int) {
	if node == nil {
		return
	}
	*gindices = append(*gindices, gindex)
	collectGindices(node.Left(), gindex*2, gindices)
	collectGindices(node.Right(), gindex*2+1, gindices)
}

func assertProofEqual(t *testing.T, gindex int, want, got *treeproof.Proof) {
	t.Helper()
	if got.Index != gindex {
		t.Fatalf("gindex %d: result gindex = %d", gindex, got.Index)
	}
	if !bytes.Equal(got.Leaf, want.Leaf) {
		t.Fatalf("gindex %d: leaf = %x, want %x", gindex, got.Leaf, want.Leaf)
	}
	if len(got.Hashes) != len(want.Hashes) {
		t.Fatalf("gindex %d: %d hashes, want %d", gindex, len(got.Hashes), len(want.Hashes))
	}
	for i := range got.Hashes {
		if !bytes.Equal(got.Hashes[i], want.Hashes[i]) {
			t.Fatalf("gindex %d: hash %d = %x, want %x", gindex, i, got.Hashes[i], want.Hashes[i])
		}
	}
}

// GetProofs must produce byte-identical proofs to the tree-based Prove for
// every generalized index of the tree, both individually and in one shared
// batch.
func TestGetProofsMatchesTree(t *testing.T) {
	tests := []struct {
		name   string
		source *proofTestContainer
	}{
		{"filled", proofTestValue()},
		{"zero value", &proofTestContainer{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := dynssz.NewDynSsz(nil)
			tree, root, getProofs := proofTestSetup(t, ds, tt.source)

			var gindices []int
			collectGindices(tree, 1, &gindices)
			if len(gindices) < 50 {
				t.Fatalf("expected a richer tree, got %d nodes", len(gindices))
			}

			for _, gindex := range gindices {
				want, proveErr := tree.Prove(gindex)
				if proveErr != nil {
					t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
				}
				got, proofErr := getProofs([]int{gindex})
				if proofErr != nil {
					t.Fatalf("GetProofs(%d): %v", gindex, proofErr)
				}
				assertProofEqual(t, gindex, want, got[0])

				if ok, verifyErr := treeproof.VerifyProof(root, got[0]); verifyErr != nil || !ok {
					t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verifyErr)
				}
			}

			// The whole set in one batch shares paths but must match too.
			batch, err := getProofs(gindices)
			if err != nil {
				t.Fatalf("batched GetProofs: %v", err)
			}
			for i, gindex := range gindices {
				want, proveErr := tree.Prove(gindex)
				if proveErr != nil {
					t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
				}
				assertProofEqual(t, gindex, want, batch[i])
			}
		})
	}
}

// Paths with no nodes must be rejected: below zero-padding chunks, below
// data chunks, below a list length chunk, and invalid indices.
func TestGetProofsInvalidPaths(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source := proofTestValue()
	tree, _, getProofs := proofTestSetup(t, ds, source)

	var gindices []int
	collectGindices(tree, 1, &gindices)
	maxGindex := 0
	for _, gindex := range gindices {
		if gindex > maxGindex {
			maxGindex = gindex
		}
	}

	// Deep below any existing node; below F4's first data chunk (field 4 of
	// 32 slots, chunks side, first chunk); below F2's length chunk.
	f4Chunk := (32+4)*2*4 + 0
	f2Length := (32+2)*2 + 1
	for _, gindex := range []int{maxGindex * 1024, f4Chunk * 2, f2Length * 2} {
		if _, err := tree.Prove(gindex); err == nil {
			t.Fatalf("tree.Prove(%d) unexpectedly succeeded", gindex)
		}
		if _, err := getProofs([]int{gindex}); err == nil {
			t.Fatalf("GetProofs(%d) unexpectedly succeeded", gindex)
		}
	}

	if _, err := getProofs([]int{0}); err == nil {
		t.Fatal("GetProofs(0) unexpectedly succeeded")
	}
}

type proofTestBitlist struct {
	Bits []byte `ssz-type:"bitlist" ssz-max:"50"`
}

type proofTestProgressive struct {
	Slot   uint64   `ssz-index:"0"`
	Root   [32]byte `ssz-index:"1" ssz-size:"32"`
	Values []uint64 `ssz-index:"3" ssz-max:"64"`
}

type proofTestProgList struct {
	L []uint64 `ssz-type:"progressive-list" ssz-max:"4096"`
	B []byte   `ssz-type:"progressive-bitlist" ssz-max:"4096"`
}

type proofTestUnionDesc struct {
	VarA proofTestInner
	VarB proofTestBadList
}

type proofTestUnionHolder struct {
	U dynssz.CompatibleUnion[proofTestUnionDesc]
}

type proofTestOptional struct {
	Opt  *uint32           `ssz-type:"optional"`
	Node *proofTestInner   `ssz-type:"optional"`
	L    []*proofTestInner `ssz-max:"4"`
}

// Exotic shapes (bitlists, progressive containers and lists, unions,
// optionals): every generalized index must match the tree-based Prove path,
// whether it proves through the streamed capture or through subtree
// materialization.
func TestGetProofsExoticShapes(t *testing.T) {
	union, err := dynssz.NewCompatibleUnion[proofTestUnionDesc](1, proofTestInner{A: 9, B: [32]byte{1, 2}})
	if err != nil {
		t.Fatalf("NewCompatibleUnion: %v", err)
	}
	optVal := uint32(42)

	tests := []struct {
		name     string
		source   any
		extended bool
	}{
		{"bitlist", &proofTestBitlist{Bits: []byte{0xff, 0x35, 0x01}}, false},
		{"progressive container", &proofTestProgressive{Slot: 3, Root: [32]byte{9}, Values: []uint64{1, 2, 3, 4, 5}}, false},
		{"progressive list and bitlist", &proofTestProgList{L: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9}, B: []byte{0xaa, 0x55, 0x01}}, false},
		{"union", &proofTestUnionHolder{U: *union}, false},
		{"optional", &proofTestOptional{Opt: &optVal, Node: &proofTestInner{A: 5}, L: []*proofTestInner{{A: 6}}}, true},
		{"optional empty", &proofTestOptional{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []dynssz.DynSszOption
			if tt.extended {
				opts = append(opts, dynssz.WithExtendedTypes())
			}
			ds := dynssz.NewDynSsz(nil, opts...)
			tree, root, getProofs := proofTestSetup(t, ds, tt.source)

			var gindices []int
			collectGindices(tree, 1, &gindices)

			batch, err := getProofs(gindices)
			if err != nil {
				t.Fatalf("batched GetProofs: %v", err)
			}
			for i, gindex := range gindices {
				want, proveErr := tree.Prove(gindex)
				if proveErr != nil {
					t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
				}
				assertProofEqual(t, gindex, want, batch[i])

				if ok, verifyErr := treeproof.VerifyProof(root, batch[i]); verifyErr != nil || !ok {
					t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verifyErr)
				}
			}
		})
	}
}

type proofTestBadList struct {
	Good uint64
	Bad  []uint64 `ssz-max:"2"`
}

// An over-limit list fails both when descended into and when its root is
// needed as an off-path sibling.
func TestGetProofsOverLimitList(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source := &proofTestBadList{Good: 1, Bad: []uint64{1, 2, 3}}

	// The walk hashes the whole value, so the over-limit list fails the
	// proof generation regardless of which path is requested.
	if _, err := ds.GetProofs(source, []int{3 * 2}); !errors.Is(err, sszutils.ErrListTooBig) {
		t.Fatalf("expected ErrListTooBig for list descent, got %v", err)
	}
	if _, err := ds.GetProofs(source, []int{2}); !errors.Is(err, sszutils.ErrListTooBig) {
		t.Fatalf("expected ErrListTooBig for sibling path, got %v", err)
	}
}

type proofTestBadElem struct {
	Inner []*proofTestBadList `ssz-max:"4"`
	Vec   [2]proofTestBadList `ssz-size:"2"`
}

// A failing element root surfaces both from off-path range gathering and
// from on-path descents in lists and vectors.
func TestGetProofsBadElementRoots(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source := &proofTestBadElem{
		Inner: []*proofTestBadList{
			{Good: 1},
			{Good: 2, Bad: []uint64{1, 2, 3}},
		},
	}
	vecSource := &proofTestBadElem{Inner: []*proofTestBadList{{Good: 1}}}
	vecSource.Vec[1].Bad = []uint64{1, 2, 3}

	// Inner is field 0 of a 2-slot container; element 1's over-limit inner
	// list fails the walk for any requested path.
	elem0 := (2+0)*2*4 + 0
	if _, err := ds.GetProofs(source, []int{elem0 * 2}); err == nil {
		t.Fatal("expected error from off-path element root")
	}
	elem1Bad := ((2+0)*2*4 + 1) * 2 * 2
	if _, err := ds.GetProofs(source, []int{elem1Bad}); err == nil {
		t.Fatal("expected error from on-path element descent")
	}
	// Same through the vector: element 1 of the 2-slot vector at field 1.
	vecElem1Bad := ((2+1)*2 + 1) * 2 * 2
	if _, err := ds.GetProofs(vecSource, []int{vecElem1Bad}); err == nil {
		t.Fatal("expected error from vector element descent")
	}
}

type proofTestNoLimit struct {
	L []uint64
}

// A list without a declared limit has no SSZ root to descend into.
func TestGetProofsNoLimitList(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source := &proofTestNoLimit{L: []uint64{1, 2, 3}}

	if _, err := ds.GetProofs(source, []int{2}); err == nil {
		t.Fatal("expected error for descent into a list without ssz root")
	}
}

type proofTestExtInner struct {
	V uint64
}

type proofTestExtList struct {
	L []proofTestExtInner
}

// With extended types, a composite list without a limit merkleizes to its
// occupied chunks; descents must match the tree.
func TestGetProofsExtendedNoLimitList(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	source := &proofTestExtList{L: []proofTestExtInner{{V: 1}, {V: 2}, {V: 3}}}
	tree, _, getProofs := proofTestSetup(t, ds, source)

	var gindices []int
	collectGindices(tree, 1, &gindices)
	for _, gindex := range gindices {
		want, proveErr := tree.Prove(gindex)
		if proveErr != nil {
			t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
		}
		got, proofErr := getProofs([]int{gindex})
		if proofErr != nil {
			t.Fatalf("GetProofs(%d): %v", gindex, proofErr)
		}
		assertProofEqual(t, gindex, want, got[0])
	}
}

type proofTestWrapperDesc struct {
	Data []uint64 `ssz-max:"16"`
}

// A type wrapper is transparent to proof paths: descents pass through to the
// wrapped value.
func TestGetProofsTypeWrapper(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source, err := dynssz.NewTypeWrapper[proofTestWrapperDesc]([]uint64{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatalf("NewTypeWrapper: %v", err)
	}
	tree, _, getProofs := proofTestSetup(t, ds, source)

	var gindices []int
	collectGindices(tree, 1, &gindices)
	for _, gindex := range gindices {
		want, proveErr := tree.Prove(gindex)
		if proveErr != nil {
			t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
		}
		got, proofErr := getProofs([]int{gindex})
		if proofErr != nil {
			t.Fatalf("GetProofs(%d): %v", gindex, proofErr)
		}
		assertProofEqual(t, gindex, want, got[0])
	}
}

type proofTestBadBitvector struct {
	BV []byte `ssz-type:"bitvector" ssz-bitsize:"4"`
}

// A value that fails validation during the walk (stray bitvector padding
// bits) surfaces the error from proof generation.
func TestGetProofsPackedMarshalError(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source := &proofTestBadBitvector{BV: []byte{0xff}}

	if _, err := ds.GetProofs(source, []int{2}); err == nil {
		t.Fatal("expected error from bitvector with stray padding bits")
	}
}

type proofWrapU64Desc struct {
	V uint64
}

type proofTestWrapElemList struct {
	L []*dynssz.TypeWrapper[proofWrapU64Desc, uint64] `ssz-max:"8"`
}

// Wrapped basic elements pack like their inner type; descents must match the
// tree.
func TestGetProofsWrappedElements(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source := &proofTestWrapElemList{}
	for i := range 5 {
		w, err := dynssz.NewTypeWrapper[proofWrapU64Desc](uint64(i + 1))
		if err != nil {
			t.Fatalf("NewTypeWrapper: %v", err)
		}
		source.L = append(source.L, w)
	}
	tree, _, getProofs := proofTestSetup(t, ds, source)

	var gindices []int
	collectGindices(tree, 1, &gindices)
	for _, gindex := range gindices {
		want, proveErr := tree.Prove(gindex)
		if proveErr != nil {
			t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
		}
		got, proofErr := getProofs([]int{gindex})
		if proofErr != nil {
			t.Fatalf("GetProofs(%d): %v", gindex, proofErr)
		}
		assertProofEqual(t, gindex, want, got[0])
	}
}

type proofTestAsyncElem struct {
	V uint64
	W uint64
}

type proofTestAsyncContainer struct {
	Count uint64
	Items []proofTestAsyncElem `ssz-max:"65536"`
	Tail  [32]byte             `ssz-size:"32"`
}

// With async hashing enabled, proofs must stay byte-identical to the tree
// path: a muted large list reduces through background jobs while a
// scheduled one runs non-incrementally, and both agree with GetTree+Prove.
func TestGetProofsAsyncHashing(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithAsyncHashing(4))
	source := &proofTestAsyncContainer{Count: 7}
	source.Items = make([]proofTestAsyncElem, 40000)
	for i := range source.Items {
		source.Items[i].V = uint64(i)
		source.Items[i].W = uint64(i * 2)
	}
	tree, root, getProofs := proofTestSetup(t, ds, source)

	// Count and Tail leave the big list muted; the element target schedules
	// the list scope itself.
	countG := 4 + 0
	tailG := 4 + 2
	elemG := (4+1)*2*65536 + 31337
	lengthG := (4+1)*2 + 1

	proofs, err := getProofs([]int{countG, tailG, elemG, elemG * 2, lengthG, 1})
	if err != nil {
		t.Fatalf("GetProofs: %v", err)
	}
	for i, gindex := range []int{countG, tailG, elemG, elemG * 2, lengthG, 1} {
		want, proveErr := tree.Prove(gindex)
		if proveErr != nil {
			t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
		}
		assertProofEqual(t, gindex, want, proofs[i])
		if ok, verifyErr := treeproof.VerifyProof(root, proofs[i]); verifyErr != nil || !ok {
			t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verifyErr)
		}
	}
}

// WithNoFastHash covers proof generation too: sibling-range reduction runs
// on the native Go sha256 implementation and stays byte-identical to the
// tree path.
func TestGetProofsNoFastHash(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastHash())
	source := &proofTestAsyncContainer{Count: 3}
	source.Items = make([]proofTestAsyncElem, 500)
	for i := range source.Items {
		source.Items[i].V = uint64(i)
		source.Items[i].W = uint64(i * 3)
	}
	tree, root, getProofs := proofTestSetup(t, ds, source)

	elemG := (4+1)*2*65536 + 123
	for _, gindex := range []int{4, elemG, elemG * 2} {
		want, proveErr := tree.Prove(gindex)
		if proveErr != nil {
			t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
		}
		got, err := getProofs([]int{gindex})
		if err != nil {
			t.Fatalf("GetProofs(%d): %v", gindex, err)
		}
		assertProofEqual(t, gindex, want, got[0])
		if ok, verifyErr := treeproof.VerifyProof(root, got[0]); verifyErr != nil || !ok {
			t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verifyErr)
		}
	}
}

type proofsEntryInner struct {
	A uint64
	B [32]byte `ssz-size:"32"`
}

type proofsEntryContainer struct {
	Items []*proofsEntryInner `ssz-max:"8"`
	Count uint64
}

// GetProofs must return verifiable proofs matching the tree-based path; the
// reflection engine itself is tested in the reflection package.
func TestGetProofs(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	source := &proofsEntryContainer{
		Items: []*proofsEntryInner{{A: 1, B: [32]byte{1}}, {A: 2, B: [32]byte{2}}},
		Count: 2,
	}

	tree, err := ds.GetTree(source)
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	root := tree.Hash()

	// Items element 0: field 0 of 2 slots -> chunks side -> slot 0 of 8.
	gindices := []int{1, 2*2*8 + 0, 3}
	proofs, err := ds.GetProofs(source, gindices)
	if err != nil {
		t.Fatalf("GetProofs: %v", err)
	}
	if len(proofs) != len(gindices) {
		t.Fatalf("got %d proofs, want %d", len(proofs), len(gindices))
	}
	for i, gindex := range gindices {
		want, proveErr := tree.Prove(gindex)
		if proveErr != nil {
			t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
		}
		if proofs[i].Index != gindex || !bytes.Equal(proofs[i].Leaf, want.Leaf) {
			t.Fatalf("gindex %d: proof mismatch", gindex)
		}
		if ok, verifyErr := treeproof.VerifyProof(root, proofs[i]); verifyErr != nil || !ok {
			t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verifyErr)
		}
	}
}

func TestGetProofsInvalidInput(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	if _, err := ds.GetProofs(nil, []int{1}); err == nil {
		t.Fatal("GetProofs(nil) unexpectedly succeeded")
	}
	if _, err := ds.GetProofs(&proofsEntryContainer{}, []int{0}); err == nil {
		t.Fatal("GetProofs with gindex 0 unexpectedly succeeded")
	}
	if _, err := ds.GetProofs(make(chan int), []int{1}); err == nil {
		t.Fatal("GetProofs with unsupported type unexpectedly succeeded")
	}
}

// A pruned tree serves proofs for the scheduled indices and for the roots of
// collapsed sibling subtrees on their paths; anything deeper inside a
// collapsed subtree fails with the tree's regular not-found error instead of
// fabricating a proof.
func TestGetTreePrunedIndexProve(t *testing.T) {
	type prunedProveContainer struct {
		Values []uint64 `ssz-max:"4096"`
		Extra  uint64
	}
	source := &prunedProveContainer{Values: make([]uint64, 4096), Extra: 7}
	for i := range source.Values {
		source.Values[i] = uint64(i)
	}
	ds := dynssz.NewDynSsz(nil)

	full, err := ds.GetTree(source)
	if err != nil {
		t.Fatal(err)
	}
	onPath := (2+0)*2*1024 + 2 // chunk 2 of the Values data tree
	pruned, err := ds.GetTree(source, dynssz.WithTreeIndices(onPath))
	if err != nil {
		t.Fatal(err)
	}

	proof, err := pruned.Prove(onPath)
	if err != nil {
		t.Fatalf("scheduled index: %v", err)
	}
	if ok, verr := treeproof.VerifyProof(full.Hash(), proof); verr != nil || !ok {
		t.Fatalf("scheduled proof invalid: %v %v", ok, verr)
	}

	offPath := (2+0)*2*1024 + 900
	if _, offErr := pruned.Prove(offPath); offErr == nil {
		t.Fatal("off-path index unexpectedly proved")
	}
	if _, multiErr := pruned.ProveMulti([]int{onPath, offPath}); multiErr == nil {
		t.Fatal("ProveMulti with an off-path index unexpectedly succeeded")
	}

	// The root of a collapsed sibling subtree on the retained path stays
	// provable: its hash is the placeholder node's value.
	siblingRoot := (onPath / 2) ^ 1
	sibProof, err := pruned.Prove(siblingRoot)
	if err != nil {
		t.Fatalf("collapsed sibling root: %v", err)
	}
	if ok, verr := treeproof.VerifyProof(full.Hash(), sibProof); verr != nil || !ok {
		t.Fatalf("collapsed-sibling proof invalid: %v %v", ok, verr)
	}
}

// The proof capture consumes scheduled scopes' child chunks in blocks, so
// its allocations stay bounded by the block size regardless of how wide the
// value is — materializing anything proportional to the full tree is a
// regression.
func TestGetProofsBoundedAllocation(t *testing.T) {
	type wideContainer struct {
		Values []uint64   `ssz-max:"16777216"`
		Roots  [][32]byte `ssz-max:"1048576" ssz-size:"?,32"`
	}
	source := &wideContainer{Values: make([]uint64, 1<<20), Roots: make([][32]byte, 1<<17)}
	for i := range source.Values {
		source.Values[i] = uint64(i)
	}
	ds := dynssz.NewDynSsz(nil)

	gindices := []int{(2+0)*2*1<<22 + 5, (2+1)*2*1<<20 + 9}
	if _, err := ds.GetProofs(source, gindices); err != nil { // warm the type cache
		t.Fatal(err)
	}

	runtime.GC()
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	proofs, err := ds.GetProofs(source, gindices)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&m1)

	if len(proofs) != len(gindices) {
		t.Fatalf("got %d proofs, want %d", len(proofs), len(gindices))
	}
	// ~36MB of value data; the capture should allocate a small fraction of
	// it (block buffers, entries, proof nodes), nowhere near the chunk count.
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 8<<20 {
		t.Fatalf("GetProofs allocated %d bytes for a %d-chunk value; capture is not streaming", alloc, 1<<18+1<<17)
	}
}

type proofTestRootOnly struct {
	Slot   uint64   `ssz-index:"0"`
	Values []uint64 `ssz-index:"3" ssz-max:"64"`
}

// Root-only targets (the value root or any subtree root) must not force the
// capture to reconstruct runtime-shaped subtrees: the hasher computes them
// normally and the root chunk serves the proof. Progressive shapes
// previously panicked here.
func TestGetProofsRootOnlyTargets(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	sources := []any{
		&proofTestRootOnly{Slot: 1, Values: []uint64{1, 2, 3}},
		&proofTestBitlist{Bits: []byte{0xaa, 0x01}},
		&proofTestContainer{F0: 7},
	}
	for _, source := range sources {
		tree, err := ds.GetTree(source)
		if err != nil {
			t.Fatalf("GetTree: %v", err)
		}
		root := tree.Hash()

		proofs, err := ds.GetProofs(source, []int{1})
		if err != nil {
			t.Fatalf("GetProofs(1): %v", err)
		}
		if !bytes.Equal(proofs[0].Leaf, root) {
			t.Fatalf("root proof leaf %x != root %x", proofs[0].Leaf, root)
		}
	}
}

type proofTestMaxInner struct{ A uint64 }
type proofTestMaxList struct {
	L []*proofTestMaxInner `ssz-max:"18446744073709551615"`
}
type proofTestMaxUint256 struct {
	L [][]byte `ssz-type:"?,uint256" ssz-size:"?,32" ssz-max:"18446744073709551615"`
}

// Depth-64 trees span the whole uint64 slot space; the fold's range
// arithmetic must not wrap. Interior targets on both halves of the mixin
// chunk tree stay provable.
func TestGetProofsMaxLimitDepth(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	sources := []any{
		&proofTestMaxList{L: []*proofTestMaxInner{{A: 1}, {A: 2}}},
		&proofTestMaxList{},
		&proofTestMaxUint256{L: [][]byte{bytes.Repeat([]byte{4}, 32)}},
	}
	for _, source := range sources {
		tree, err := ds.GetTree(source)
		if err != nil {
			t.Fatalf("GetTree: %v", err)
		}
		root := tree.Hash()

		// gindex 4: leftmost path into the chunk tree; gindex 5: interior of
		// the right half (slot 2^63 retention).
		for _, gindex := range []int{4, 5} {
			want, proveErr := tree.Prove(gindex)
			if proveErr != nil {
				t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
			}
			got, err := ds.GetProofs(source, []int{gindex})
			if err != nil {
				t.Fatalf("GetProofs(%d): %v", gindex, err)
			}
			assertProofEqual(t, gindex, want, got[0])
			if ok, verr := treeproof.VerifyProof(root, got[0]); verr != nil || !ok {
				t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verr)
			}
		}
	}
}

// Runtime-shaped subtrees (progressive lists) build whole but are pruned
// back to the requested proof paths before retention, so the retained node
// count stays path-bounded instead of element-bounded.
func TestGetProofsPrunedRetainedSubtree(t *testing.T) {
	type progHolder struct {
		L []uint64 `ssz-type:"progressive-list" ssz-max:"1048576"`
	}
	source := &progHolder{L: make([]uint64, 1<<15)}
	for i := range source.L {
		source.L[i] = uint64(i)
	}
	ds := dynssz.NewDynSsz(nil)

	full, err := ds.GetTree(source)
	if err != nil {
		t.Fatal(err)
	}
	// A deep target inside the progressive subtree: reuse a gindex the full
	// tree can prove.
	var gindices []int
	collectGindices(full, 1, &gindices)
	target := gindices[len(gindices)/2]

	pruned, err := ds.GetTree(source, dynssz.WithTreeIndices(target))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pruned.Hash(), full.Hash()) {
		t.Fatalf("pruned root mismatch")
	}
	want, proveErr := full.Prove(target)
	if proveErr != nil {
		t.Fatalf("full Prove(%d): %v", target, proveErr)
	}
	got, err := pruned.Prove(target)
	if err != nil {
		t.Fatalf("pruned Prove(%d): %v", target, err)
	}
	assertProofEqual(t, target, want, got)

	var countNodes func(n *treeproof.Node) int
	countNodes = func(n *treeproof.Node) int {
		if n == nil {
			return 0
		}
		return 1 + countNodes(n.Left()) + countNodes(n.Right())
	}
	fullCount, prunedCount := countNodes(full), countNodes(pruned)
	if prunedCount*10 > fullCount {
		t.Fatalf("pruned progressive tree has %d nodes, full %d: retention is not path-bounded", prunedCount, fullCount)
	}
}

// Deep proofs into progressive lists stream through the scheduled capture:
// a warmed run must stay data-bounded instead of materializing the list's
// node tree (which previously peaked at ~100MB for this shape).
func TestGetProofsProgressiveAllocationBound(t *testing.T) {
	type progWide struct {
		L []uint64 `ssz-type:"progressive-list" ssz-max:"16777216"`
	}
	source := &progWide{L: make([]uint64, 1<<20)}
	for i := range source.L {
		source.L[i] = uint64(i)
	}
	ds := dynssz.NewDynSsz(nil)

	full, err := ds.GetTree(source)
	if err != nil {
		t.Fatal(err)
	}
	var gindices []int
	collectGindices(full, 1, &gindices)
	target := gindices[len(gindices)*3/4]

	warmProofs, err := ds.GetProofs(source, []int{target}) // warm caches and pools
	if err != nil {
		t.Fatal(err)
	}
	want, proveErr := full.Prove(target)
	if proveErr != nil {
		t.Fatal(proveErr)
	}
	assertProofEqual(t, target, want, warmProofs[0])

	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	if _, err := ds.GetProofs(source, []int{target}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&m1)
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 32<<20 {
		t.Fatalf("progressive deep proof allocated %d bytes; capture is not streaming", alloc)
	}
}

// Deep proofs into Put-style values build straight from the flat chunk data:
// a warmed run must not materialize the value's node tree.
func TestGetProofsPutBytesAllocationBound(t *testing.T) {
	type blobHolder struct {
		Blob  []byte `ssz-size:"1048576"`
		Other uint64
	}
	source := &blobHolder{Blob: bytes.Repeat([]byte{7}, 1<<20)}
	ds := dynssz.NewDynSsz(nil)

	full, err := ds.GetTree(source)
	if err != nil {
		t.Fatal(err)
	}
	target := (2+0)*2048*2 + 99 // deep chunk inside the blob's 32768-chunk tree
	want, proveErr := full.Prove(target)
	if proveErr != nil {
		t.Fatal(proveErr)
	}

	warm, err := ds.GetProofs(source, []int{target})
	if err != nil {
		t.Fatal(err)
	}
	assertProofEqual(t, target, want, warm[0])

	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	if _, err := ds.GetProofs(source, []int{target}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&m1)
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 4<<20 {
		t.Fatalf("deep PutBytes proof allocated %d bytes; subtree is not chunk-built", alloc)
	}
}

type proofTestProgComposite struct {
	L []*proofTestInner `ssz-type:"progressive-list" ssz-max:"4096"`
}

// Deep proofs into composite progressive-list elements decompose through the
// stable progressive positions; every generalized index must match the
// tree-based Prove path.
func TestGetProofsProgressiveComposite(t *testing.T) {
	source := &proofTestProgComposite{}
	for i := range 7 {
		inner := &proofTestInner{A: uint64(i + 1)}
		inner.B[0] = byte(i + 1)
		source.L = append(source.L, inner)
	}
	ds := dynssz.NewDynSsz(nil)
	tree, root, getProofs := proofTestSetup(t, ds, source)

	var gindices []int
	collectGindices(tree, 1, &gindices)
	for _, gindex := range gindices {
		want, proveErr := tree.Prove(gindex)
		if proveErr != nil {
			t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
		}
		got, err := getProofs([]int{gindex})
		if err != nil {
			t.Fatalf("GetProofs(%d): %v", gindex, err)
		}
		assertProofEqual(t, gindex, want, got[0])
		if ok, verr := treeproof.VerifyProof(root, got[0]); verr != nil || !ok {
			t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verr)
		}
	}
}

type proofTestProgContainer struct {
	Slot   uint64            `ssz-index:"0"`
	Root   [32]byte          `ssz-index:"1" ssz-size:"32"`
	Values []uint64          `ssz-index:"3" ssz-max:"64"`
	Inner  proofTestInner    `ssz-index:"6"`
	List   []*proofTestInner `ssz-index:"9" ssz-type:"progressive-list" ssz-max:"4096"`
}

// Deep proofs into progressive containers decompose through the fields'
// stable ssz-index chunk positions, including gap slots and the
// active-fields mixin; every generalized index must match the tree-based
// Prove path.
func TestGetProofsProgressiveContainer(t *testing.T) {
	source := &proofTestProgContainer{
		Slot:   11,
		Root:   [32]byte{1, 2, 3},
		Values: []uint64{4, 5, 6},
	}
	for i := range 5 {
		inner := &proofTestInner{A: uint64(i + 1)}
		inner.B[0] = byte(i + 1)
		source.List = append(source.List, inner)
	}
	ds := dynssz.NewDynSsz(nil)

	for name, src := range map[string]any{"filled": source, "zero": &proofTestProgContainer{}} {
		t.Run(name, func(t *testing.T) {
			tree, root, getProofs := proofTestSetup(t, ds, src)

			var gindices []int
			collectGindices(tree, 1, &gindices)
			for _, gindex := range gindices {
				want, proveErr := tree.Prove(gindex)
				if proveErr != nil {
					t.Fatalf("tree.Prove(%d): %v", gindex, proveErr)
				}
				got, err := getProofs([]int{gindex})
				if err != nil {
					t.Fatalf("GetProofs(%d): %v", gindex, err)
				}
				assertProofEqual(t, gindex, want, got[0])
				if ok, verr := treeproof.VerifyProof(root, got[0]); verr != nil || !ok {
					t.Fatalf("VerifyProof(%d) = %v, %v", gindex, ok, verr)
				}
			}
		})
	}
}

// Deep proofs into a progressive container's fields stream through the
// scheduled capture: a warmed run must stay data-bounded instead of
// building the container's subtree whole.
func TestGetProofsProgressiveContainerAllocationBound(t *testing.T) {
	type progState struct {
		Slot       uint64   `ssz-index:"0"`
		Validators []uint64 `ssz-index:"2" ssz-type:"progressive-list" ssz-max:"16777216"`
	}
	source := &progState{Slot: 3, Validators: make([]uint64, 1<<20)}
	for i := range source.Validators {
		source.Validators[i] = uint64(i)
	}
	ds := dynssz.NewDynSsz(nil)

	full, err := ds.GetTree(source)
	if err != nil {
		t.Fatal(err)
	}
	var gindices []int
	collectGindices(full, 1, &gindices)
	target := gindices[len(gindices)*3/4]

	warmProofs, err := ds.GetProofs(source, []int{target}) // warm caches and pools
	if err != nil {
		t.Fatal(err)
	}
	want, proveErr := full.Prove(target)
	if proveErr != nil {
		t.Fatal(proveErr)
	}
	assertProofEqual(t, target, want, warmProofs[0])

	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	if _, err := ds.GetProofs(source, []int{target}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&m1)
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 32<<20 {
		t.Fatalf("progressive container proof allocated %d bytes; capture is not streaming", alloc)
	}
}

// Deep proofs into an optional's value stream through the scheduled capture
// (Optional[T] merkleizes like List[T, 1]): a warmed run must stay
// data-bounded instead of building the optional's subtree whole.
func TestGetProofsOptionalAllocationBound(t *testing.T) {
	type optInner struct {
		Values []uint64 `ssz-max:"16777216"`
	}
	type optHolder struct {
		Opt *optInner `ssz-type:"optional"`
	}
	source := &optHolder{Opt: &optInner{Values: make([]uint64, 1<<20)}}
	for i := range source.Opt.Values {
		source.Opt.Values[i] = uint64(i)
	}
	ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())

	full, err := ds.GetTree(source)
	if err != nil {
		t.Fatal(err)
	}
	var gindices []int
	collectGindices(full, 1, &gindices)
	target := gindices[len(gindices)*3/4]

	warmProofs, err := ds.GetProofs(source, []int{target}) // warm caches and pools
	if err != nil {
		t.Fatal(err)
	}
	want, proveErr := full.Prove(target)
	if proveErr != nil {
		t.Fatal(proveErr)
	}
	assertProofEqual(t, target, want, warmProofs[0])

	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	if _, err := ds.GetProofs(source, []int{target}); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&m1)
	if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 32<<20 {
		t.Fatalf("optional deep proof allocated %d bytes; capture is not streaming", alloc)
	}
}
