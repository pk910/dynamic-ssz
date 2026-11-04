// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"reflect"
	"slices"
	"strings"
	"sync"
)

// annotationEntry holds the tags registered for one type, newest first, and
// the single tag string they merge into.
type annotationEntry struct {
	tags   []string
	merged string
}

// typeAnnotations is a global registry mapping reflect.Type to the SSZ tags
// registered for it. Populated by Annotate[T]() calls, typically at package
// init time; annotationMutex serializes the registrations.
var (
	typeAnnotations sync.Map // map[reflect.Type]annotationEntry
	annotationMutex sync.Mutex
)

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

	// Multiple Annotate calls for the same type (e.g. a hand-written constraint
	// annotation plus a generated ssz-static declaration) are merged into a
	// single space-separated tag rather than overwriting one another. The new tag
	// is prepended so that for a duplicated key the most recent registration wins
	// (reflect.StructTag.Lookup returns the first occurrence). A tag already
	// registered is not added again, so repeated calls -- a package initialised
	// twice in one test binary, say -- leave the entry as it is instead of
	// growing it. The lock makes the read-modify-write one step: registrations
	// run at package-init time, but nothing stops a caller from registering
	// later.
	annotationMutex.Lock()
	defer annotationMutex.Unlock()

	merged := []string{tag}
	if existing, ok := typeAnnotations.Load(t); ok {
		if entry, _ := existing.(annotationEntry); len(entry.tags) > 0 {
			// Nothing is added twice, whether it comes back as the tag that was
			// registered or as the whole string the registrations merged into.
			if tag == entry.merged || slices.Contains(entry.tags, tag) {
				return true
			}
			merged = append(merged, entry.tags...)
		}
	}

	typeAnnotations.Store(t, annotationEntry{tags: merged, merged: strings.Join(merged, " ")})

	return true // allows use in var _ = Annotate[T](...)
}

// LookupAnnotation returns the raw SSZ tag string registered for the
// given reflect.Type via Annotate[T](), or ("", false) if none was registered.
func LookupAnnotation(t reflect.Type) (string, bool) {
	if t == nil {
		return "", false
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	v, ok := typeAnnotations.Load(t)
	if !ok {
		return "", false
	}

	entry, ok := v.(annotationEntry)
	if !ok || entry.merged == "" {
		return "", false
	}

	return entry.merged, true
}
