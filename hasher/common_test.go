// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher_test

import (
	"bytes"
	"sync/atomic"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
	"github.com/pk910/dynamic-ssz/treeproof"
)

type isolationElem struct {
	X uint64
	Y [48]byte
}

// isolationRootOnly has a structure and hashes itself through the
// argument-less method only, which both engines call under SHA256 alone.
type isolationRootOnly struct {
	A uint64
	B uint64
}

func (r *isolationRootOnly) HashTreeRoot() ([32]byte, error) {
	return [32]byte{0xaa}, nil
}

type isolationValue struct {
	A uint64
	L []uint64        `ssz-max:"1024"`
	E []isolationElem `ssz-max:"16"`
	R isolationRootOnly
}

// A run under one hash touches nothing of another: it builds none of the
// other hash's zero subtree roots and never calls its function. Every entry
// point that takes an Algorithm is run here.
func TestAlgorithmsAreIsolated(t *testing.T) { //nolint:paralleltest // the built-in algorithms are process-wide
	families := map[string][2]*hasher.Algorithm{
		"sha256":    {hasher.SHA256, hasher.SHA256Native},
		"keccak256": {hasher.Keccak256, hasher.Keccak256Native},
	}

	for name, family := range families {
		for _, alg := range family {
			var others []*hasher.Algorithm
			for otherName, otherFamily := range families {
				if otherName != name {
					others = append(others, otherFamily[:]...)
				}
			}

			hasher.ResetZeroTables()
			var calls, accelerated atomic.Int64
			restores := make([]func(), 0, len(others)+1)
			for _, other := range others {
				restores = append(restores, hasher.CountCompressions(other, &calls))
			}
			// A run of the native Algorithm stays out of the accelerated
			// backend of its own hash too.
			if alg == family[1] {
				restores = append(restores, hasher.CountCompressions(family[0], &accelerated))
			}

			runEveryEntryPoint(t, name, alg)

			for _, restore := range restores {
				restore()
			}
			if n := calls.Load(); n != 0 {
				t.Errorf("%s: a run called the function of another hash %d times", name, n)
			}
			if n := accelerated.Load(); n != 0 {
				t.Errorf("%s: a native run called the accelerated backend %d times", name, n)
			}
			for _, other := range others {
				if hasher.ZeroTableBuilt(other) {
					t.Errorf("%s: a run built the zero subtree roots of another hash", name)
				}
			}
		}
	}
	hasher.ResetZeroTables()
}

func runEveryEntryPoint(t *testing.T, name string, alg *hasher.Algorithm) {
	t.Helper()
	fail := func(step string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %s: %v", name, step, err)
		}
	}
	value := &isolationValue{A: 1, L: []uint64{1, 2, 3}, E: []isolationElem{{X: 1}, {X: 2}}, R: isolationRootOnly{A: 4, B: 5}}
	option := dynssz.WithHashAlgorithm(alg)
	proofOption := treeproof.WithAlgorithm(alg)

	for engine, ds := range map[string]*dynssz.DynSsz{
		"instance default": dynssz.NewDynSsz(nil, dynssz.WithDefaultHashAlgorithm(alg)),
		"call option":      dynssz.NewDynSsz(nil),
		"reflection":       dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation()),
	} {
		root, err := ds.HashTreeRoot(value, option)
		fail(engine+" HashTreeRoot", err)

		hh := alg.Pool().Get()
		fail(engine+" HashTreeRootWith", ds.HashTreeRootWith(value, hh, option))
		walked, err := hh.HashRoot()
		alg.Pool().Put(hh)
		fail(engine+" HashRoot", err)
		if walked != root {
			t.Fatalf("%s: %s: the walker root differs from the root", name, engine)
		}

		tree, err := ds.GetTree(value, option)
		fail(engine+" GetTree", err)
		if !bytes.Equal(tree.Hash(), root[:]) {
			t.Fatalf("%s: %s: the tree root differs from the root", name, engine)
		}

		// Field L is the second of four: gindex 5, its data tree 10 with 256
		// chunks. 10*256 is a value, 10*256+128 padding, twice that below a
		// zero chunk.
		for _, index := range []int{10 * 256, 10*256 + 128, 3} {
			proof, proveErr := tree.Prove(index)
			fail(engine+" Prove", proveErr)
			ok, verifyErr := treeproof.VerifyProof(root[:], proof, proofOption)
			fail(engine+" VerifyProof", verifyErr)
			if !ok {
				t.Fatalf("%s: %s: proof of %d does not verify", name, engine, index)
			}
		}
		if _, err = tree.Get((10*256 + 128) * 2); err == nil {
			t.Fatalf("%s: %s: found a node below a zero chunk", name, engine)
		}

		for _, indices := range [][]int{{10 * 256, 10*256 + 1}, {10 * 256, 3}, {10*256 + 128, 4}} {
			proof, proveErr := tree.ProveMulti(indices)
			fail(engine+" ProveMulti", proveErr)
			restored := proof.Compress(proofOption).Decompress(proofOption)
			ok, verifyErr := treeproof.VerifyMultiproof(root[:], restored.Hashes, restored.Leaves, restored.Indices, proofOption)
			fail(engine+" VerifyMultiproof", verifyErr)
			if !ok {
				t.Fatalf("%s: %s: multiproof of %v does not verify", name, engine, indices)
			}
		}
		tree.Show(1)
	}

	// A full leaf layer verifies without proof hashes, through a hasher.
	w := treeproof.NewWrapper(proofOption)
	idx := w.StartTree(sszutils.TreeTypeBinary)
	leaves := make([][]byte, 4)
	for i := range leaves {
		leaves[i] = make([]byte, 32)
		leaves[i][0] = byte(i + 1)
		w.Append(leaves[i])
	}
	w.Merkleize(idx)
	root, err := w.HashRoot()
	fail("wrapper HashRoot", err)
	ok, err := treeproof.VerifyMultiproof(root[:], nil, leaves, []int{4, 5, 6, 7}, proofOption)
	fail("VerifyMultiproof of a full leaf layer", err)
	if !ok {
		t.Fatalf("%s: the full leaf layer does not verify", name)
	}

	// A hasher and a pool made around the bare function belong to its hash.
	pool := &hasher.HasherPool{HashFn: alg.HashFn()}
	if !pool.Algorithm().Compatible(alg) || !hasher.NewHasherWithHashFn(alg.HashFn()).Algorithm().Compatible(alg) {
		t.Fatalf("%s: a bare function was given another hash", name)
	}
	_ = alg.ZeroHash(3)
	_ = alg.Compatible(hasher.SHA256)
	_ = alg.Compatible(hasher.Keccak256)
}
