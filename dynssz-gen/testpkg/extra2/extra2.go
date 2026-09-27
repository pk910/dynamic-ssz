// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynssz-gen test fixtures.

// Package extra2 registers the key extra registers, for the same type. Two
// separately loaded packages initialize in import path order, so extra2
// initializes after extra and its registration merges in front of extra's.
package extra2

import (
	"github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/viewfix/sub"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ = sszutils.Annotate[sub.Nums](`ssz-minsize:"5"`)
