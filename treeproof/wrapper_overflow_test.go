// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"math"
	"testing"
)

func TestMerkleizeWithMixinNumOverflow(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for num > MaxInt64")
		}
		msg, ok := r.(string)
		if !ok || msg == "" {
			t.Fatalf("expected string panic message, got: %v", r)
		}
	}()

	w.MerkleizeWithMixin(0, math.MaxUint64, 1)
}

func TestMerkleizeWithMixinLimitOverflow(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for limit > MaxInt64")
		}
		msg, ok := r.(string)
		if !ok || msg == "" {
			t.Fatalf("expected string panic message, got: %v", r)
		}
	}()

	w.MerkleizeWithMixin(0, 1, math.MaxUint64)
}

func TestMerkleizeProgressiveWithMixinNumOverflow(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for num > MaxInt64")
		}
		msg, ok := r.(string)
		if !ok || msg == "" {
			t.Fatalf("expected string panic message, got: %v", r)
		}
	}()

	w.MerkleizeProgressiveWithMixin(0, math.MaxUint64)
}
