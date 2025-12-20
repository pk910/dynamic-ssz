// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"reflect"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

// TestGenerateHashTreeRootUnsupportedType tests that generateHashTreeRoot returns an error
// for unsupported SSZ types.
func TestGenerateHashTreeRootUnsupportedType(t *testing.T) {
	tests := []struct {
		name    string
		sszType dynssz.SszType
	}{
		{"UnsupportedType_255", dynssz.SszType(255)},
		{"UnsupportedType_100", dynssz.SszType(100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := &dynssz.TypeDescriptor{
				Type:    testDummyReflectType,
				SszType: tt.sszType,
				Kind:    reflect.Struct,
			}

			codeBuilder := &strings.Builder{}
			typePrinter := NewTypePrinter("test/package")
			options := &CodeGeneratorOptions{}

			err := generateHashTreeRoot(desc, codeBuilder, typePrinter, options)
			if err == nil {
				t.Error("expected error for unsupported SSZ type, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported SSZ type") {
				t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
			}
		})
	}
}

// TestHashTreeRootContainerWithNestedUnsupportedType tests that hashContainer propagates
// errors from nested unsupported types.
func TestHashTreeRootContainerWithNestedUnsupportedType(t *testing.T) {
	unsupportedFieldDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Struct,
	}

	containerDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &dynssz.ContainerDescriptor{
			Fields: []dynssz.FieldDescriptor{
				{
					Name: "UnsupportedField",
					Type: unsupportedFieldDesc,
				},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateHashTreeRoot(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestHashTreeRootProgressiveContainerError tests that progressive containers also
// properly propagate errors for hash tree root.
func TestHashTreeRootProgressiveContainerError(t *testing.T) {
	unsupportedFieldDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Struct,
	}

	containerDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszProgressiveContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &dynssz.ContainerDescriptor{
			Fields: []dynssz.FieldDescriptor{
				{
					Name:     "UnsupportedField",
					SszIndex: 0,
					Type:     unsupportedFieldDesc,
				},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateHashTreeRoot(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for progressive container with unsupported field, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestHashTreeRootVectorWithNestedUnsupportedType tests that hashVector propagates
// errors from nested unsupported element types.
func TestHashTreeRootVectorWithNestedUnsupportedType(t *testing.T) {
	unsupportedElemDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
	}

	vectorDesc := &dynssz.TypeDescriptor{
		Type:     testDummyArrayReflectType,
		SszType:  dynssz.SszVectorType,
		Kind:     reflect.Array,
		ElemDesc: unsupportedElemDesc,
		Len:      10,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateHashTreeRoot(vectorDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported element type in vector, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestHashTreeRootListWithNestedUnsupportedType tests that hashList propagates
// errors from nested unsupported element types.
func TestHashTreeRootListWithNestedUnsupportedType(t *testing.T) {
	unsupportedElemDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
	}

	listDesc := &dynssz.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      dynssz.SszListType,
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Slice,
		ElemDesc:     unsupportedElemDesc,
		Limit:        100,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateHashTreeRoot(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported element type in list, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestHashTreeRootProgressiveListError tests that progressive lists properly propagate errors.
func TestHashTreeRootProgressiveListError(t *testing.T) {
	unsupportedElemDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
	}

	listDesc := &dynssz.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      dynssz.SszProgressiveListType,
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Slice,
		ElemDesc:     unsupportedElemDesc,
		Limit:        100,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateHashTreeRoot(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for progressive list with unsupported element type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestHashTreeRootUnionWithNestedUnsupportedType tests that hashUnion propagates
// errors from nested unsupported variant types.
func TestHashTreeRootUnionWithNestedUnsupportedType(t *testing.T) {
	unsupportedVariantDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Struct,
	}

	unionDesc := &dynssz.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      dynssz.SszCompatibleUnionType,
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Struct,
		UnionVariants: map[uint8]*dynssz.TypeDescriptor{
			0: unsupportedVariantDesc,
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateHashTreeRoot(unionDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported variant type in union, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestHashTreeRootTypeWrapperWithNestedUnsupportedType tests that hashType handles
// TypeWrapper with nested unsupported types.
func TestHashTreeRootTypeWrapperWithNestedUnsupportedType(t *testing.T) {
	unsupportedElemDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Struct,
	}

	wrapperDesc := &dynssz.TypeDescriptor{
		Type:     testDummyReflectType,
		SszType:  dynssz.SszTypeWrapperType,
		Kind:     reflect.Struct,
		ElemDesc: unsupportedElemDesc,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateHashTreeRoot(wrapperDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type in TypeWrapper, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}
