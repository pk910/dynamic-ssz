// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"reflect"
	"strings"
	"testing"
)

type testAnnotatedList []uint32

type testAnnotatedList2 []uint64

func TestAnnotate_BasicRegistration(t *testing.T) {
	Annotate[testAnnotatedList](`ssz-max:"20"`)

	tag, ok := LookupAnnotation(reflect.TypeFor[testAnnotatedList]())
	if !ok {
		t.Fatal("expected annotation to be found")
	}

	if tag != `ssz-max:"20"` {
		t.Fatalf("expected tag %q, got %q", `ssz-max:"20"`, tag)
	}
}

func TestAnnotate_MultipleTags(t *testing.T) {
	Annotate[testAnnotatedList2](`ssz-max:"4096" dynssz-max:"MAX_BLOBS"`)

	tag, ok := LookupAnnotation(reflect.TypeFor[testAnnotatedList2]())
	if !ok {
		t.Fatal("expected annotation to be found")
	}

	if tag != `ssz-max:"4096" dynssz-max:"MAX_BLOBS"` {
		t.Fatalf("unexpected tag: %q", tag)
	}
}

func TestAnnotate_LookupMiss(t *testing.T) {
	type unregisteredType []byte

	_, ok := LookupAnnotation(reflect.TypeFor[unregisteredType]())
	if ok {
		t.Fatal("expected no annotation for unregistered type")
	}
}

func TestAnnotate_PointerLookup(t *testing.T) {
	// Registration uses non-pointer, lookup uses pointer — should still match
	Annotate[testAnnotatedList](`ssz-max:"20"`)

	tag, ok := LookupAnnotation(reflect.PointerTo(reflect.TypeFor[testAnnotatedList]()))
	if !ok {
		t.Fatal("expected annotation to be found via pointer type")
	}

	if tag != `ssz-max:"20"` {
		t.Fatalf("expected tag %q, got %q", `ssz-max:"20"`, tag)
	}
}

func TestAnnotate_PointerType(t *testing.T) {
	// Annotate with a pointer type parameter — should store the element type
	type ptrTarget []uint32

	Annotate[*ptrTarget](`ssz-max:"15"`)

	tag, ok := LookupAnnotation(reflect.TypeFor[ptrTarget]())
	if !ok {
		t.Fatal("expected annotation to be found for pointer-registered type")
	}

	if tag != `ssz-max:"15"` {
		t.Fatalf("expected tag %q, got %q", `ssz-max:"15"`, tag)
	}
}

func TestLookupAnnotation_NonStringValue(t *testing.T) {
	// Directly store a non-string value to cover the defensive type assertion
	typeAnnotations.Store(reflect.TypeFor[int](), 42)

	_, ok := LookupAnnotation(reflect.TypeFor[int]())
	if ok {
		t.Fatal("expected false for non-string value in registry")
	}

	// Clean up
	typeAnnotations.Delete(reflect.TypeFor[int]())
}

func TestAnnotate_Merge(t *testing.T) {
	type mergeType []uint32

	// Annotations from different places (e.g. a hand-written constraint and a
	// generated ssz-static declaration) are merged rather than overwriting.
	Annotate[mergeType](`ssz-max:"20"`)
	Annotate[mergeType](`ssz-static:"false"`)

	tag, ok := LookupAnnotation(reflect.TypeFor[mergeType]())
	if !ok {
		t.Fatal("expected annotation to be found")
	}
	if !strings.Contains(tag, `ssz-max:"20"`) || !strings.Contains(tag, `ssz-static:"false"`) {
		t.Fatalf("expected merged tag to contain both annotations, got %q", tag)
	}

	// Re-registering an identical full tag must not change the annotation.
	Annotate[mergeType](tag)
	tag2, _ := LookupAnnotation(reflect.TypeFor[mergeType]())
	if tag2 != tag {
		t.Fatalf("re-registering the identical tag changed it: %q -> %q", tag, tag2)
	}
}

// LookupAnnotation must return ("", false) for a nil type instead of panicking.
func TestLookupAnnotation_Nil(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LookupAnnotation panicked on nil type: %v", r)
		}
	}()

	tag, ok := LookupAnnotation(nil)
	if ok || tag != "" {
		t.Errorf("LookupAnnotation(nil) = (%q, %v), want (\"\", false)", tag, ok)
	}
}
