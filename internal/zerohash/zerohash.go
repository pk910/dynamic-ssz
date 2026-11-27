// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package zerohash gives the packages of this module the zero subtree roots
// of a hash algorithm in place, where the public API hands out copies.
package zerohash

// Table returns the zero subtree roots of a *hasher.Algorithm, one per tree
// depth. The array is the one every hasher of that hash reads: it must not
// be written. An Algorithm whose hash could not be identified has none, and
// what its function reported is returned instead. The hasher package
// installs the function when it is initialized.
var Table func(algorithm any) (*[65][32]byte, error)
