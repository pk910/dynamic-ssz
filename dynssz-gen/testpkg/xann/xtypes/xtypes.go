// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package xtypes declares annotated types used from another package.
package xtypes

import "github.com/pk910/dynamic-ssz/sszutils"

// Roots is a fixed-size vector of roots.
type Roots [][32]byte

var _ = sszutils.Annotate[Roots](`ssz-size:"2,32"`)

// Nums is a bounded list.
type Nums []uint32

var _ = sszutils.Annotate[Nums](`ssz-max:"3"`)
