// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"
	"testing"
)

// minSizeSpecs answers whole expressions, as the engine's evaluator does.
type minSizeSpecs map[string]uint64

func (s minSizeSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	v, ok := s[name]
	return ok, v, nil
}

// A declared floor parses into its literal and expression; the expression is
// resolved with the literal as its fallback, and a literal alone stands.
func TestMinSizeDeclaration(t *testing.T) {
	const expr = "(SPEC_A):32+(SPEC_B):4*8+(SPEC_BITS/8):3+4"
	literal, got, ok := ParseMinSizeDeclaration(reflect.StructTag(`ssz-static:"false" ssz-minsize:"71" dynssz-minsize:"` + expr + `"`))
	if !ok || literal != 71 || got != expr {
		t.Fatalf("parsed %d %q %v", literal, got, ok)
	}
	if floor, err := EvaluateMinSize(minSizeSpecs{expr: 31}, literal, expr); err != nil || floor != 31 {
		t.Fatalf("resolved floor = %d, err = %v, want 31", floor, err)
	}
	if floor, err := EvaluateMinSize(minSizeSpecs{}, literal, expr); err != nil || floor != 71 {
		t.Fatalf("fallback floor = %d, err = %v, want 71", floor, err)
	}
	if floor, err := EvaluateMinSize(nil, literal, expr); err != nil || floor != 71 {
		t.Fatalf("floor without specs = %d, err = %v, want 71", floor, err)
	}
	if floor, err := EvaluateMinSize(nil, 20, ""); err != nil || floor != 20 {
		t.Fatalf("literal floor = %d, err = %v, want 20", floor, err)
	}
	if _, _, ok := ParseMinSizeDeclaration(`ssz-minsize:"x"`); ok {
		t.Error("an invalid literal parsed")
	}
	if _, err := EvaluateMinSize(minSizeSpecs{expr: 1 << 40}, literal, expr); err == nil {
		t.Error("a floor beyond the size limit was accepted")
	}
}
