// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"errors"
	"reflect"
	"testing"
)

type specSetCacheSpecs struct {
	mockDynamicSpecs
	sets map[reflect.Type][]uint64
}

func (c *specSetCacheSpecs) LoadSpecSet(key reflect.Type) []uint64 {
	return c.sets[key]
}

func (c *specSetCacheSpecs) StoreSpecSet(key reflect.Type, set []uint64) []uint64 {
	if cached, ok := c.sets[key]; ok {
		return cached
	}
	c.sets[key] = set
	return set
}

type specSetTypeA struct{}
type specSetTypeB struct{}

func TestGetCachedSpecSet(t *testing.T) {
	builds := 0
	build := func(ds DynamicSpecs) ([]uint64, error) {
		builds++
		v, err := ResolveSpecValueWithDefault(ds, "MAX", 4)
		if err != nil {
			return nil, err
		}
		return []uint64{v}, nil
	}

	t.Run("without cache builds every call", func(t *testing.T) {
		builds = 0
		ds := &mockDynamicSpecs{values: map[string]uint64{"MAX": 9}}
		for i := 0; i < 3; i++ {
			set, err := GetCachedSpecSet[specSetTypeA](ds, build)
			if err != nil || len(set) != 1 || set[0] != 9 {
				t.Fatalf("call %d: set %v, err %v", i, set, err)
			}
		}
		if builds != 3 {
			t.Fatalf("built %d times, want 3", builds)
		}
	})

	t.Run("with cache builds once per type", func(t *testing.T) {
		builds = 0
		ds := &specSetCacheSpecs{mockDynamicSpecs: mockDynamicSpecs{values: map[string]uint64{"MAX": 9}}, sets: map[reflect.Type][]uint64{}}
		first, err := GetCachedSpecSet[specSetTypeA](ds, build)
		if err != nil {
			t.Fatal(err)
		}
		second, err := GetCachedSpecSet[specSetTypeA](ds, build)
		if err != nil {
			t.Fatal(err)
		}
		if &first[0] != &second[0] {
			t.Fatal("second call did not return the cached set")
		}
		if _, err := GetCachedSpecSet[specSetTypeB](ds, build); err != nil {
			t.Fatal(err)
		}
		if builds != 2 {
			t.Fatalf("built %d times, want 2 (one per type)", builds)
		}
		if len(ds.sets) != 2 {
			t.Fatalf("cached %d sets, want 2", len(ds.sets))
		}
	})

	t.Run("build error is returned and not cached", func(t *testing.T) {
		builds = 0
		ds := &specSetCacheSpecs{mockDynamicSpecs: mockDynamicSpecs{values: map[string]uint64{"MAX": 0}}, sets: map[reflect.Type][]uint64{}}
		failing := func(ds DynamicSpecs) ([]uint64, error) {
			builds++
			return nil, NewSszError(ErrInvalidConstraint, "spec value MAX resolved to 0")
		}
		for i := 0; i < 2; i++ {
			set, err := GetCachedSpecSet[specSetTypeA](ds, failing)
			if set != nil || !errors.Is(err, ErrInvalidConstraint) {
				t.Fatalf("call %d: set %v, err %v", i, set, err)
			}
		}
		if builds != 2 || len(ds.sets) != 0 {
			t.Fatalf("built %d times with %d cached sets, want 2 and 0", builds, len(ds.sets))
		}
	})
}
