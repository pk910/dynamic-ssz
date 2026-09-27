// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package xann holds a container whose fields use annotated types declared in
// another package, and the data type served through the xviews views.
package xann

import "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/xann/xtypes"

// Holder references annotated types declared in xtypes.
type Holder struct {
	A xtypes.Roots
	B xtypes.Nums
	C uint8
}

// Data is served through xviews.View, whose fields carry the annotations.
type Data struct {
	Roots [][32]byte
	Nums  []uint32
	C     uint8
}
