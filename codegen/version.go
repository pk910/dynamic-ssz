// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"runtime/debug"
)

var Version = "unknown"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/pk910/dynamic-ssz" {
				Version = dep.Version
				break
			}
		}
	}
}
