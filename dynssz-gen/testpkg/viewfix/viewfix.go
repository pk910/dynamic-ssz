// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package viewfix is a dynssz-gen coverage fixture for external view-type
// loading. It deliberately does NOT import its sub package, so a view type
// referenced from sub triggers a fresh external package load.
package viewfix

// Base is a simple container with view types defined in the sub package.
type Base struct {
	F1 uint64
	F2 uint64
}
