// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package third registers an annotation for a type declared in another
// package (viewfix/sub) that it neither declares nor generates; testpkg
// imports it, so the registration applies wherever testpkg's code runs.
package third

import (
	"github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/viewfix/sub"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ = sszutils.Annotate[sub.Nums](`ssz-minsize:"9"`)
