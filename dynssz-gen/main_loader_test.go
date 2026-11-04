// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestMain memoizes successful package loads for the test binary. The
// generator type-checks the whole import graph from source on every run, and
// the tests run it dozens of times against the same few packages, none of
// which change while the tests run. Failed loads are not cached, and tests
// that swap the loader (withLoader) restore this wrapper afterwards.
func TestMain(m *testing.M) {
	load := loadPackages
	var (
		mu    sync.Mutex
		cache = make(map[string][]*packages.Package, 8)
	)
	loadPackages = func(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
		key := strings.Join(patterns, "\x00")
		if cfg != nil {
			key = fmt.Sprintf("%d\x00%s\x00%s", cfg.Mode, cfg.Dir, key)
		}
		mu.Lock()
		pkgs, ok := cache[key]
		mu.Unlock()
		if ok {
			return pkgs, nil
		}
		pkgs, err := load(cfg, patterns...)
		if err == nil {
			mu.Lock()
			cache[key] = pkgs
			mu.Unlock()
		}
		return pkgs, err
	}
	os.Exit(m.Run())
}
