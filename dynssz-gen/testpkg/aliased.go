// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package testpkg

// Import sszutils under an alias so dynssz-gen's AST scanner has to pick up
// the alias name from imp.Name (rather than defaulting to the package name).
// This exercises the aliased-import branch in findAnnotateCall.
import szs "github.com/pk910/dynamic-ssz/sszutils"

// AliasedAnnotated is a byte list whose Annotate call is routed through an
// aliased sszutils import in this file's imports.
type AliasedAnnotated []byte

var _ = szs.Annotate[AliasedAnnotated](`ssz-max:"16"`)
