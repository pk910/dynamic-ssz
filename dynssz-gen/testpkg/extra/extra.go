// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynssz-gen test fixtures.

// Package extra registers an annotation for a type declared in viewfix/sub
// with a key testpkg registers too. Nothing imports it, so a generator run
// only sees it when it is loaded separately, as a view package is; it then
// initializes before the generated package and loses the duplicate key.
package extra

import (
	"github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/viewfix/sub"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ = sszutils.Annotate[sub.Nums](`ssz-minsize:"7"`)
