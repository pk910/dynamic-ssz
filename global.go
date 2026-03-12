// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"sync"
	"sync/atomic"
)

var (
	globalDynSsz atomic.Pointer[DynSsz]
	globalMu     sync.Mutex
)

// GetGlobalDynSsz returns the global DynSsz instance, creating one with default
// settings if none exists. Safe for concurrent use.
func GetGlobalDynSsz() *DynSsz {
	if ds := globalDynSsz.Load(); ds != nil {
		return ds
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	// Double-check after acquiring lock.
	if ds := globalDynSsz.Load(); ds != nil {
		return ds
	}

	ds := NewDynSsz(nil)
	globalDynSsz.Store(ds)

	return ds
}

// SetGlobalSpecs replaces the global DynSsz instance with a new one configured
// with the given specification values. Safe for concurrent use.
func SetGlobalSpecs(specs map[string]any) {
	globalMu.Lock()
	defer globalMu.Unlock()

	globalDynSsz.Store(NewDynSsz(specs))
}
