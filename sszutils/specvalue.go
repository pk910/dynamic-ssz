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
		// An absent spec value leaves the static fallback, and a zero fallback
		// is the "no static value" placeholder rather than a real one: the type
		// said its value comes from the spec, and the spec does not have it. It
		// is the same dead end as resolving to zero, and reporting it names the
		// key that is missing instead of leaving a zero to surface later as a
		// capacity of nothing.
		if defaultValue == 0 {
			return 0, NewSszErrorf(ErrInvalidConstraint, "spec value %q is not defined and has no positive static fallback", name)
		}

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
