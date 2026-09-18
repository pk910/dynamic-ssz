// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package walker

import (
	"math/rand"
	"testing"

	"github.com/pk910/dynamic-ssz/treeproof"
)

// Both walkers build the same tree from the same program. The comparison runs
// after every call, so a divergence names the call that caused it rather than
// only the root at the end.
func TestWalkerParity(t *testing.T) {
	for seed := int64(0); seed < 2000; seed++ {
		prog, openAfter := GenerateWithScopes(rand.New(rand.NewSource(seed)))
		// Every prefix is compared as a program of its own, closed off, so a
		// difference names the call that introduced it rather than the root at
		// the end. The whole program is the last prefix.
		for k := 1; k <= len(prog); k++ {
			full := append(append([]Op(nil), prog[:k]...), CloseScopes(openAfter[k-1])...)
			if err := Compare(full); err != nil {
				t.Fatalf("seed %d, after step %d (%s): %v\nprogram:\n%s",
					seed, k-1, prog[k-1], err, String(full))
			}
		}
	}
}

// The tree the walker builds is not only the right root: every leaf of it
// proves against that root. A program whose tree cannot be finalized is one
// the hasher also refuses, which compareWalkers already pins.
func TestWalkerParityProofs(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		prog := Generate(rand.New(rand.NewSource(seed)))
		w := treeproof.NewWrapper()
		if _, err := Run(w, prog, nil); err != nil {
			continue
		}
		root, err := w.Root()
		if err != nil {
			continue
		}
		if err := root.Finalize(); err != nil {
			t.Fatalf("seed %d: finalize: %v\nprogram:\n%s", seed, err, String(prog))
		}
		hash := root.Hash()
		for _, gindex := range leafIndices(root, 1, 0) {
			proof, err := root.Prove(gindex)
			if err != nil {
				t.Fatalf("seed %d: prove %d: %v", seed, gindex, err)
			}
			ok, err := treeproof.VerifyProof(hash, proof)
			if err != nil || !ok {
				t.Fatalf("seed %d: proof for %d verified = %v, %v\nprogram:\n%s", seed, gindex, ok, err, String(prog))
			}
		}
	}
}

// leafIndices collects the generalized index of every leaf, bounded so a wide
// tree does not turn one program into thousands of proofs.
func leafIndices(n *treeproof.Node, gindex, depth int) []int {
	if n == nil || depth > 12 {
		return nil
	}
	if n.Left() == nil && n.Right() == nil {
		return []int{gindex}
	}
	idx := leafIndices(n.Left(), gindex*2, depth+1)
	if len(idx) < 64 {
		idx = append(idx, leafIndices(n.Right(), gindex*2+1, depth+1)...)
	}
	return idx
}
