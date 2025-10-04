// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.
//go:build cgo
// +build cgo

package hasher

import (
	"github.com/pk910/dynamic-ssz/hasher/cgo"
)

func init() {
	FastHasherPool.HashFn = cgo.HashtreeHashByteSlice
}
