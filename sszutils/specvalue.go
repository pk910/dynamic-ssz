// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "reflect"

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

// SpecSetCache is implemented by a DynamicSpecs that keeps the resolved spec
// sets of generated types, one per schema they serve, for the lifetime of its
// spec values. The key is the library's; an implementation holds it opaque.
// DynSsz implements it; a DynamicSpecs without it resolves the set on every
// call.
//
// The load and the store are separate so the builder never crosses the
// interface: an argument of an interface method call escapes, and the builder
// is a method value, so handing it over would allocate its closure on every
// method entry. GetCachedSpecSet calls it directly instead, on a miss only.
type SpecSetCache interface {
	// LoadSpecSet returns the set cached under key, or nil.
	LoadSpecSet(key reflect.Type) []uint64
	// StoreSpecSet caches set under key and returns the set now cached, which
	// is another caller's when it stored first.
	StoreSpecSet(key reflect.Type, set []uint64) []uint64
}

// specSetKey is the cache key of the spec set of the generated type T
// serving the schema V: T itself for its own methods, or one of its view
// types. The expressions and their order follow the walk over the pair, so a
// view served by two types has a set per type, and a view type generated on
// its own has its own. The function type over the pair is one canonical type
// per pair, which keys as one word.
func specSetKey[T, V any]() reflect.Type {
	return reflect.TypeFor[func(T) V]()
}

// GetCachedSpecSet returns the spec set of the generated type T under ds,
// building it with build on the first call for a caching ds and on every
// call otherwise. The set is shared between callers and must not be written.
//
// Generated code calls this once per method entry with the type's
// buildDynSSZSpecSet, which resolves every spec expression the type's methods
// use.
func GetCachedSpecSet[T any](ds DynamicSpecs, build func(DynamicSpecs) ([]uint64, error)) ([]uint64, error) {
	return cachedSpecSet(ds, specSetKey[T, T](), build)
}

// GetCachedViewSpecSet is GetCachedSpecSet for the methods of T serving its
// view V, which resolve the view's expressions by a builder of their own.
func GetCachedViewSpecSet[T, V any](ds DynamicSpecs, build func(DynamicSpecs) ([]uint64, error)) ([]uint64, error) {
	return cachedSpecSet(ds, specSetKey[T, V](), build)
}

func cachedSpecSet(ds DynamicSpecs, key reflect.Type, build func(DynamicSpecs) ([]uint64, error)) ([]uint64, error) {
	cache, ok := ds.(SpecSetCache)
	if !ok {
		return build(ds)
	}
	if set := cache.LoadSpecSet(key); set != nil {
		return set, nil
	}
	set, err := build(ds)
	if err != nil {
		return nil, err
	}
	return cache.StoreSpecSet(key, set), nil
}
