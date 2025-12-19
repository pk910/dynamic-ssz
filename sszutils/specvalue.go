// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

func ResolveSpecValueWithDefault(ds DynamicSpecs, name string, defaultValue uint64) (uint64, error) {
	hasLimit, limit, err := ds.ResolveSpecValue(name)
	if err != nil {
		return 0, err
	}
	if !hasLimit {
		return defaultValue, nil
	}
	return limit, nil
}
