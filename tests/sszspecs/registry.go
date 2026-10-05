// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package sszspecs runs the official ethereum/ssz-specs vectors against
// dynamic-ssz. The Go types the vectors declare are generated from their
// descriptors by ./gen, and their SSZ methods by dynssz-gen; see
// setup_test_data.sh. Without generated files the package still compiles and
// its test skips.
package sszspecs

import "reflect"

// vectorTypes maps a descriptor key to a constructor for the generated Go
// type that declaration names. The generated files fill it.
var vectorTypes = map[string]func() any{}

// unsupportedTypes maps a descriptor key to why the library has no spelling
// for that legal declaration.
var unsupportedTypes = map[string]string{}

// illegalTypes maps the id of an illegal-declaration case to the Go type
// that spells the declaration.
var illegalTypes = map[string]reflect.Type{}

// inexpressibleTypes maps the id of an illegal-declaration case to why Go
// cannot spell the declaration at all, which refuses it by construction.
var inexpressibleTypes = map[string]string{}
