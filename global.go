// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

var globalDynSsz *DynSsz

func GetGlobalDynSsz() *DynSsz {
	if globalDynSsz == nil {
		globalDynSsz = NewDynSsz(nil)
	}
	return globalDynSsz
}

func SetGlobalSpecs(specs map[string]any) {
	globalDynSsz = NewDynSsz(specs)
}
