// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"fmt"
	"math"
	"strconv"

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

	// Fast path: a spec value provided directly under this name keeps its exact
	// type and full uint64 precision. govaluate evaluates everything as float64,
	// which silently loses precision near uint64 max, so it is only used for
	// actual expressions below.
	if raw, ok := d.specValues[name]; ok {
		value, resolved, err := specValueToUint64(raw)
		if err != nil {
			return false, 0, fmt.Errorf("invalid dynamic spec value %q: %w", name, err)
		}
		if resolved {
			cachedValue.resolved = true
			cachedValue.value = value
			d.specCacheMutex.Lock()
			d.specValueCache[name] = cachedValue
			d.specCacheMutex.Unlock()
			return true, value, nil
		}
	}

	expression, err := govaluate.NewEvaluableExpression(name)
	if err != nil {
		return false, 0, fmt.Errorf("error parsing dynamic spec expression: %w", err)
	}

	result, err := expression.Evaluate(d.specValues)
	if err == nil {
		if value, ok := result.(float64); ok {
			resolved, rerr := specFloatToUint64(value)
			if rerr != nil {
				return false, 0, fmt.Errorf("invalid dynamic spec expression %q: %w", name, rerr)
			}
			cachedValue.resolved = true
			cachedValue.value = resolved
		}
	}

	d.specCacheMutex.Lock()
	d.specValueCache[name] = cachedValue
	d.specCacheMutex.Unlock()

	return cachedValue.resolved, cachedValue.value, nil
}

// specValueToUint64 converts a directly-provided spec value to uint64, preserving
// full precision for integer types. A value stored directly under a referenced
// spec key but carrying an unsupported type (or a non-numeric string) returns an
// error so the misconfiguration surfaces instead of silently falling back to the
// static limit.
func specValueToUint64(raw any) (value uint64, ok bool, err error) {
	switch v := raw.(type) {
	case uint64:
		return v, true, nil
	case uint:
		return uint64(v), true, nil
	case uint32:
		return uint64(v), true, nil
	case uint16:
		return uint64(v), true, nil
	case uint8:
		return uint64(v), true, nil
	case uintptr:
		return uint64(v), true, nil
	case int:
		return intSpecToUint64(int64(v))
	case int64:
		return intSpecToUint64(v)
	case int32:
		return intSpecToUint64(int64(v))
	case int16:
		return intSpecToUint64(int64(v))
	case int8:
		return intSpecToUint64(int64(v))
	case float64:
		u, ferr := specFloatToUint64(v)
		return u, ferr == nil, ferr
	case float32:
		u, ferr := specFloatToUint64(float64(v))
		return u, ferr == nil, ferr
	case string:
		u, perr := strconv.ParseUint(v, 10, 64)
		if perr != nil {
			return 0, false, fmt.Errorf("string value %q is not a valid uint64", v)
		}
		return u, true, nil
	default:
		return 0, false, fmt.Errorf("unsupported type %T", raw)
	}
}

func intSpecToUint64(v int64) (uint64, bool, error) {
	if v < 0 {
		return 0, false, fmt.Errorf("negative value %d", v)
	}
	return uint64(v), true, nil
}

// specFloatToUint64 validates a float spec value and rounds it up to the next
// whole unit (partial bytes/bits cannot be serialized).
func specFloatToUint64(v float64) (uint64, error) {
	switch {
	case math.IsNaN(v):
		return 0, fmt.Errorf("value is NaN")
	case math.IsInf(v, 0):
		return 0, fmt.Errorf("value is infinite")
	case v < 0:
		return 0, fmt.Errorf("negative value %v", v)
	case v >= math.Ldexp(1, 64): // >= 2^64 overflows uint64
		return 0, fmt.Errorf("value %v overflows uint64", v)
	}
	u := uint64(v)
	if float64(u) < v {
		u++
	}
	return u, nil
}
