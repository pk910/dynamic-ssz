// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

// ResolveSpecValueWithDefault resolves a named specification value using ds,
// returning defaultValue if the name is not found.
//
// This helper is called by generated code to resolve dynssz-size/dynssz-max
// expressions, passing the static ssz-size/ssz-max as defaultValue. A spec value
// that resolves to 0 would form a zero-length vector or a zero-capacity list,
// both of which are invalid per the SSZ spec, so it falls back to the positive
// static value, or errors when there is no positive static fallback — mirroring
// the reflection path. A name that is not present in the spec set keeps the
// static value unchanged (the static placeholder convention).
func ResolveSpecValueWithDefault(ds DynamicSpecs, name string, defaultValue uint64) (uint64, error) {
	hasLimit, limit, err := ds.ResolveSpecValue(name)
	if err != nil {
		return 0, err
	}
	if !hasLimit {
		return defaultValue, nil
	}
	if limit == 0 {
		if defaultValue == 0 {
			return 0, NewSszErrorf(ErrInvalidConstraint, "spec value %q resolved to 0 with no positive static fallback", name)
		}
		return defaultValue, nil
	}
	return limit, nil
}
