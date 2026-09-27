// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package testpkg

import (
	"github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/viewfix/sub"

	// third registers an annotation for a sub type; linked with this package.
	_ "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/third"
)

// Holder references types declared and annotated in an imported package.
type Holder struct {
	A sub.Roots
	B sub.Nums
	C uint8
}
