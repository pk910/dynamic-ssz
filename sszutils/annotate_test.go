// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"reflect"
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

func TestAnnotate_Overwrite(t *testing.T) {
	type overwriteType []uint32

	Annotate[overwriteType](`ssz-max:"10"`)
	Annotate[overwriteType](`ssz-max:"20"`)

	tag, ok := LookupAnnotation(reflect.TypeFor[overwriteType]())
	if !ok {
		t.Fatal("expected annotation to be found")
	}

	if tag != `ssz-max:"20"` {
		t.Fatalf("expected overwritten tag %q, got %q", `ssz-max:"20"`, tag)
	}
}
