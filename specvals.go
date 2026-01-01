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

func (d *DynSsz) ResolveSpecValue(name string) (bool, uint64, error) {
	if cachedValue := d.specValueCache[name]; cachedValue != nil {
		return cachedValue.resolved, cachedValue.value, nil
	}

	cachedValue := &cachedSpecValue{}
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

	d.specValueCache[name] = cachedValue
	return cachedValue.resolved, cachedValue.value, nil
}
