// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"fmt"

	"github.com/casbin/govaluate"
)

type cachedSpecValue struct {
	resolved bool
	value    uint64
}

// ResolveSpecValue resolves a dynamic specification value by name. The name can
// be a simple identifier (e.g., "MAX_VALIDATORS_PER_COMMITTEE") or a mathematical
// expression referencing spec values. Results are cached for subsequent lookups.
//
// Returns whether the value was resolved, the uint64 value, and any parse error.
// If the name references undefined spec values, resolved will be false with no error.
func (d *DynSsz) ResolveSpecValue(name string) (bool, uint64, error) {
	d.specCacheMutex.RLock()
	cachedValue := d.specValueCache[name]
	d.specCacheMutex.RUnlock()
	if cachedValue != nil {
		return cachedValue.resolved, cachedValue.value, nil
	}

	cachedValue = &cachedSpecValue{}
	expression, err := govaluate.NewEvaluableExpression(name)
	if err != nil {
		return false, 0, fmt.Errorf("error parsing dynamic spec expression: %w", err)
	}

	result, err := expression.Evaluate(d.specValues)
	if err == nil {
		value, ok := result.(float64)
		if ok {
			cachedValue.resolved = true
			cachedValue.value = uint64(value)
			if float64(cachedValue.value) < value {
				// rounding issue - always round up to full bytes as we can't serialize parial bytes
				cachedValue.value++
			}
		}
	}

	// fmt.Printf("spec lookup %v,  ok: %v, value: %v\n", name, cachedValue.resolved, cachedValue.value)
	d.specCacheMutex.Lock()
	d.specValueCache[name] = cachedValue
	d.specCacheMutex.Unlock()

	return cachedValue.resolved, cachedValue.value, nil
}
