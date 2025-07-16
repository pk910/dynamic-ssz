// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"

	"github.com/casbin/govaluate"
)

type cachedSpecValue struct {
	resolved bool
	value    uint64
}

func (d *DynSsz) getSpecValue(name string) (bool, uint64, error) {
	if cachedValue := d.specValueCache[name]; cachedValue != nil {
		return cachedValue.resolved, cachedValue.value, nil
	}

	cachedValue := &cachedSpecValue{}
	expression, err := govaluate.NewEvaluableExpression(name)
	if err != nil {
		return false, 0, fmt.Errorf("error parsing dynamic spec expression: %v", err)
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
