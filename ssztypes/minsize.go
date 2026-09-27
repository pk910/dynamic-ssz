// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"
	"strconv"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// ParseMinSizeDeclaration reads the floor a variable-size type declares
// beside ssz-static: ssz-minsize, the bytes every value holds in its fixed
// section when no spec value is defined, and dynssz-minsize, the expression
// the spec resolves to those bytes, which resolves each spec-decided part on
// its own through the evaluator's :N fallback (see ResolveSpecValue). A
// literal that does not parse states no floor.
func ParseMinSizeDeclaration(tag reflect.StructTag) (literal uint64, expr string, ok bool) {
	if value, found := tag.Lookup("ssz-minsize"); found {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return 0, "", false
		}
		literal = parsed
	}
	expr, _ = tag.Lookup("dynssz-minsize")
	return literal, expr, true
}

// EvaluateMinSize resolves a declared floor against specs: the expression
// with the literal as its fallback, as a size tag resolves, or the literal
// alone. A floor beyond the SSZ size limit is refused, as generated code
// refuses it.
func EvaluateMinSize(specs sszutils.DynamicSpecs, literal uint64, expr string) (int64, error) {
	floor := literal
	if expr != "" {
		if specs == nil {
			specs = noSpecs{}
		}
		resolved, err := sszutils.ResolveSpecValueWithDefault(specs, expr, literal)
		if err != nil {
			return 0, err
		}
		floor = resolved
	}
	if floor > uint64(sszutils.MaxSszSize) {
		return 0, sszutils.NewSszErrorf(sszutils.SizeLimitSentinel(floor), "declared minimum size %d exceeds the SSZ size limit", floor)
	}
	return int64(floor), nil
}

// noSpecs defines no spec value, so every part takes its fallback.
type noSpecs struct{}

func (noSpecs) ResolveSpecValue(string) (bool, uint64, error) { return false, 0, nil }
