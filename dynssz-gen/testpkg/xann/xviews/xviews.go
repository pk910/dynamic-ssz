// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package xviews holds a view of xann.Data whose field types are annotated
// here; xann does not import it, so it is loaded separately.
package xviews

import "github.com/pk910/dynamic-ssz/sszutils"

// RootsV is a fixed-size vector of roots.
type RootsV [][32]byte

var _ = sszutils.Annotate[RootsV](`ssz-size:"2,32"`)

// NumsV is a bounded list.
type NumsV []uint32

var _ = sszutils.Annotate[NumsV](`ssz-max:"3"`)

// View exposes xann.Data through annotated field types.
type View struct {
	Roots RootsV
	Nums  NumsV
	C     uint8
}
