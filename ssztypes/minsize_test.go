// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"errors"
	"reflect"
	"testing"
)

// minSizeSpecs answers whole expressions, as the engine's evaluator does.
type minSizeSpecs map[string]uint64

func (s minSizeSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	v, ok := s[name]
	return ok, v, nil
}

// minSizeErrSpecs refuses every expression.
type minSizeErrSpecs struct{}

func (minSizeErrSpecs) ResolveSpecValue(string) (bool, uint64, error) {
	return false, 0, errors.New("refused")
}

// A declared floor parses into its literal and expression; the expression is
// resolved with the literal as its fallback, and a literal alone stands.
func TestMinSizeDeclaration(t *testing.T) {
	const expr = "(SPEC_A):32+(SPEC_B):4*8+(SPEC_BITS/8):3+4"
	literal, got, ok := ParseMinSizeDeclaration(reflect.StructTag(`ssz-static:"false" ssz-minsize:"71" dynssz-minsize:"` + expr + `"`))
	if !ok || literal != 71 || got != expr {
		t.Fatalf("parsed %d %q %v", literal, got, ok)
	}
	if floor := EvaluateMinSize(minSizeSpecs{expr: 31}, literal, expr); floor != 31 {
		t.Fatalf("resolved floor = %d, want 31", floor)
	}
	if floor := EvaluateMinSize(minSizeSpecs{}, literal, expr); floor != 71 {
		t.Fatalf("fallback floor = %d, want 71", floor)
	}
	if floor := EvaluateMinSize(nil, literal, expr); floor != 71 {
		t.Fatalf("floor without specs = %d, want 71", floor)
	}
	if floor := EvaluateMinSize(nil, 20, ""); floor != 20 {
		t.Fatalf("literal floor = %d, want 20", floor)
	}
	if _, _, ok := ParseMinSizeDeclaration(`ssz-minsize:"x"`); ok {
		t.Error("an invalid literal parsed")
	}
	// A floor beyond the size limit, or one whose expression the evaluator
	// refuses, states no floor rather than refusing the type.
	if floor := EvaluateMinSize(minSizeSpecs{expr: 1 << 40}, literal, expr); floor != 0 {
		t.Errorf("floor beyond the size limit = %d, want none", floor)
	}
	if floor := EvaluateMinSize(minSizeErrSpecs{}, literal, expr); floor != 0 {
		t.Errorf("floor with a refused expression = %d, want none", floor)
	}
}

// A bit list holds its termination bit, a union its selector, an optional its
// presence byte and a big.Int its sign byte, so each states a one-byte
// floor; a list or an optional list states none.
func TestSetMinSizeOneByteFloors(t *testing.T) {
	for sszType, want := range map[SszType]int64{
		SszBitlistType:            1,
		SszProgressiveBitlistType: 1,
		SszUnionType:              1,
		SszCompatibleUnionType:    1,
		SszOptionalType:           1,
		SszBigIntType:             1,
		SszListType:               0,
		SszOptionalListType:       0,
	} {
		desc := &TypeDescriptor{SszType: sszType, SszTypeFlags: SszTypeFlagIsDynamic}
		desc.SetMinSize()
		if desc.MinSize != want {
			t.Errorf("%v: MinSize = %d, want %d", sszType, desc.MinSize, want)
		}
	}
}
