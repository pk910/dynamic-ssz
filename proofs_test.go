// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"testing"

	"github.com/pk910/dynamic-ssz/treeproof"
)

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
	ds := NewDynSsz(nil)
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
	ds := NewDynSsz(nil)

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
