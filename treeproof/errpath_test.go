// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"errors"
	"strings"
	"testing"
)

// expectPanicWithError recovers from a panic and checks that the recovered
// value matches the expected error.
func expectPanicWithError(t *testing.T, expected error, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error panic, got: %T %v", r, r)
		}
		if !errors.Is(err, expected) {
			t.Fatalf("expected panic with %q, got: %q", expected, err)
		}
	}()
	fn()
}

// --- VerifyMultiproof: missing required nodes error path ---

func TestVerifyMultiproofMissingNodeInjected(t *testing.T) {
	root, leaves, allNodes := buildMerkleTree(4)
	indices := []int{4}
	leafData := [][]byte{leaves[0]}

	// Real required indices would be [5, 3]. Return only [3] so that
	// sibling 5 is never populated, triggering the missing-node error.
	getRequiredIndicesFn = func([]int) []int {
		return []int{3}
	}
	defer func() { getRequiredIndicesFn = getRequiredIndices }()

	proofHashes := [][]byte{allNodes[3]}

	_, err := VerifyMultiproof(root, proofHashes, leafData, indices)
	if err == nil {
		t.Fatal("expected missing-node error")
	}
	if !strings.Contains(err.Error(), "proof is missing required nodes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- treeFromNodesProgressiveImpl: left child build error ---

func TestTreeFromNodesProgressiveLeftError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	_, err := TreeFromNodesProgressive([]*Node{LeafFromUint64(1)})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- treeFromNodesProgressiveImpl: recursive right child error ---

func TestTreeFromNodesProgressiveRecursiveError(t *testing.T) {
	injected := errors.New("injected")
	calls := 0
	treeFromNodesFn = func(leaves []*Node, limit int) (*Node, error) {
		calls++
		if calls == 2 {
			return nil, injected
		}
		return TreeFromNodes(leaves, limit)
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	// Two leaves: first call builds left child (succeeds), recursive call
	// builds the right subtree's left child (fails on second call).
	_, err := TreeFromNodesProgressive([]*Node{LeafFromUint64(1), LeafFromUint64(2)})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- TreeFromNodesWithMixin: tree build error ---

func TestTreeFromNodesWithMixinInjectedError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	_, err := TreeFromNodesWithMixin([]*Node{LeafFromUint64(1)}, 1, 1)
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- TreeFromNodesProgressiveWithMixin: progressive build error ---

func TestTreeFromNodesProgressiveWithMixinInjectedError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	_, err := TreeFromNodesProgressiveWithMixin([]*Node{LeafFromUint64(1)}, 1)
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- TreeFromNodesProgressiveWithActiveFields: progressive build error ---

func TestTreeFromNodesProgressiveWithActiveFieldsInjectedError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	_, err := TreeFromNodesProgressiveWithActiveFields([]*Node{LeafFromUint64(1)}, []byte{0x01})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- Wrapper.Collapse: no-op coverage ---

func TestWrapperCollapseNoop(t *testing.T) {
	w := NewWrapper()
	w.AddNode(LeafFromUint64(1))
	w.Collapse()
	if len(w.nodes) != 1 {
		t.Fatal("Collapse should not modify the wrapper")
	}
}

// --- Wrapper.Commit: panic on tree build error ---

func TestWrapperCommitPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.Commit(0)
	})
}

// --- Wrapper.CommitWithMixin: panic on tree build error ---

func TestWrapperCommitWithMixinPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.CommitWithMixin(0, 1, 1)
	})
}

// --- Wrapper.CommitProgressive: panic on tree build error ---

func TestWrapperCommitProgressivePanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.CommitProgressive(0)
	})
}

// --- Wrapper.CommitProgressiveWithMixin: panic on tree build error ---

func TestWrapperCommitProgressiveWithMixinPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.CommitProgressiveWithMixin(0, 1)
	})
}

// --- Wrapper.CommitProgressiveWithActiveFields: panic on tree build error ---

func TestWrapperCommitProgressiveWithActiveFieldsPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesFn = TreeFromNodes }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.CommitProgressiveWithActiveFields(0, []byte{0x01})
	})
}
