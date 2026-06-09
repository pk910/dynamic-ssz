// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package sub holds view types for the viewfix.Base fixture.
package sub

// View1 exposes a subset of viewfix.Base.
type View1 struct {
	F1 uint64
}

// View2 exposes all of viewfix.Base.
type View2 struct {
	F1 uint64
	F2 uint64
}
