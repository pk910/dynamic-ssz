// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"math"
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
// alone. A floor is a bound and never a refusal: one that does not resolve,
// or lies beyond the SSZ size limit, states no floor, as generated code
// treats it.
func EvaluateMinSize(specs sszutils.DynamicSpecs, literal uint64, expr string) int64 {
	floor := literal
	if expr != "" {
		if specs == nil {
			specs = noSpecs{}
		}
		resolved, err := sszutils.ResolveSpecValueWithDefault(specs, expr, literal)
		if err != nil {
			return 0
		}
		floor = resolved
	}
	// The floor enters the int domain of the decoders, so it is bounded there
	// as well as to the SSZ size range.
	if floor > uint64(sszutils.MaxSszSize) || floor > math.MaxInt {
		return 0
	}
	return int64(floor)
}

// noSpecs defines no spec value, so every part takes its fallback.
type noSpecs struct{}

func (noSpecs) ResolveSpecValue(string) (bool, uint64, error) { return false, 0, nil }
