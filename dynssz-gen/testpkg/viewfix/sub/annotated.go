// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sub

import "github.com/pk910/dynamic-ssz/sszutils"

// Roots is a fixed-size vector of roots, annotated in this package and
// referenced from testpkg and from DataView.
type Roots [][32]byte

var _ = sszutils.Annotate[Roots](`ssz-size:"2,32"`)

// Nums is a bounded list, annotated here like Roots.
type Nums []uint32

var _ = sszutils.Annotate[Nums](`ssz-max:"3"`)

// DataView exposes viewfix.Data through the annotated field types above.
type DataView struct {
	Roots Roots
	Nums  Nums
	C     uint8
}
