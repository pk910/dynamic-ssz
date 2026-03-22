// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"reflect"
	"sync"
)

// typeAnnotations is a global registry mapping reflect.Type to raw SSZ tag
// strings. Populated by Annotate[T]() calls, typically at package init time.
var typeAnnotations sync.Map // map[reflect.Type]string

// Annotate registers SSZ annotations for a named (non-struct) type T.
// The tag string uses the same format as Go struct field tags:
//
//	var _ = sszutils.Annotate[BlobKZGCommitments](`ssz-max:"4096"`)
//	var _ = sszutils.Annotate[BlobKZGCommitments](`ssz-max:"4096" dynssz-max:"MAX_BLOB_COMMITMENTS"`)
//
// This is the canonical way to attach SSZ metadata to non-struct types
// that lack struct field tags. Both the reflection path and the code
// generator consume these annotations.
//
// Call this at package level (var block or init function) so the
// annotation is registered before any marshal/unmarshal/codegen operation.
func Annotate[T any](tag string) bool {
	t := reflect.TypeFor[T]()

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	typeAnnotations.Store(t, tag)

	return true // allows use in var _ = Annotate[T](...)
}

// LookupAnnotation returns the raw SSZ tag string registered for the
// given reflect.Type via Annotate[T](), or ("", false) if none was registered.
func LookupAnnotation(t reflect.Type) (string, bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	v, ok := typeAnnotations.Load(t)
	if !ok {
		return "", false
	}

	tag, ok := v.(string)
	if !ok {
		return "", false
	}

	return tag, true
}
