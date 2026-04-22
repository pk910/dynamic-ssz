// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package testpkg is a fixture for dynssz-gen coverage tests.
//
// It intentionally contains types and patterns that only exist to exercise
// code paths in dynssz-gen's AST scanning and tag parsing — specifically:
//   - An SSZ type whose Annotate tag fails to parse (InvalidAnnotated).
//   - An init() function with a non-ExprStmt (tests the "skip" branch in
//     findAnnotateCallInDecl).
//   - An unrelated sszutils call in init() (tests the "not Annotate" branch
//     in matchAnnotateCall).
//
// None of these types are meant for real use. The file is kept outside
// codegen/tests so their presence doesn't contaminate that package's
// generated code.
package testpkg

import (
	"github.com/pk910/dynamic-ssz/sszutils"
)

// InvalidAnnotated carries a syntactically valid ssz-size tag that fails
// numeric parsing — lets tests trigger the Annotate-tag error path in
// dynssz-gen's run().
type InvalidAnnotated []byte

var _ = sszutils.Annotate[InvalidAnnotated](`ssz-size:"notanumber"`)

// NonExprInitMarker exists solely so there's a type whose name can be
// searched for — the init() below contains a non-ExprStmt that the scanner
// must skip over.
type NonExprInitMarker []byte

// unrelatedCall is a package-level helper used in the init() below so we
// can have an ExprStmt that selects a function which is *not* Annotate
// (hits the `sel.Sel.Name != "Annotate"` branch in matchAnnotateCall).
func unrelatedCall() {}

func init() {
	// AssignStmt, not ExprStmt — the scanner must `continue` past this
	// without crashing when looking for Annotate calls in init() bodies.
	_ = sszutils.Annotate[NonExprInitMarker](`ssz-max:"4"`)

	// ExprStmt whose call target is a selector but with Sel.Name != "Annotate".
	// This sits under the same sszutils alias, so the scanner will recurse
	// into the Sel check and take the "not Annotate" branch.
	unrelatedCall()
}
