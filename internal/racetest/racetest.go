// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package racetest reports whether the binary was built with the race
// detector. A test that walks hundreds of megabytes through a single
// goroutine costs the detector minutes and gives it nothing to find; such a
// test skips under it and runs in full in the builds without it.
package racetest

import "runtime/debug"

// Enabled reports whether the race detector is compiled into this binary.
func Enabled() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "-race" {
			return setting.Value == "true"
		}
	}
	return false
}
