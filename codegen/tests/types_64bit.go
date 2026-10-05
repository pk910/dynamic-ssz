// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

//go:build amd64 || arm64

package tests

// BadHugeArray's Go array declares 2^33 variable-size elements: a length no
// tag supplies, so only the length bound itself refuses it.
type BadHugeArray struct {
	V [1 << 33]DynVecElem
}
