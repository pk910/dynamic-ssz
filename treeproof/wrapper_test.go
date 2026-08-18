// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
)

// NewWrapper returns the full-tree *treeBuilder without options and the proof
// capture engine with WithProofCapture; both serve the ProofHashWalker contract
// and stay assertable to their concrete types.
func TestNewWrapperDispatch(t *testing.T) {
	if _, ok := NewWrapper().(*treeBuilder); !ok {
		t.Fatal("NewWrapper() should return a *treeBuilder")
	}

	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	if _, ok := NewWrapper(WithProofCapture(&ProofSchedule{}, hh, nil)).(*proofCapture); !ok {
		t.Fatal("NewWrapper(WithProofCapture) should return the proof capture engine")
	}
}
