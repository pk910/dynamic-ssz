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

// TestGenerateSizeUnsupportedType tests that generateSize returns an error
// for unsupported SSZ types.
func TestGenerateSizeUnsupportedType(t *testing.T) {
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

			err := generateSize(desc, codeBuilder, typePrinter, options)
			if err == nil {
				t.Error("expected error for unsupported SSZ type, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported SSZ type") {
				t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
			}
		})
	}
}

// TestSizeContainerWithNestedUnsupportedType tests that sizeContainer propagates
// errors from nested unsupported types.
func TestSizeContainerWithNestedUnsupportedType(t *testing.T) {
	unsupportedFieldDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Struct,
		Size:    0,
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

	err := generateSize(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestSizeDynamicContainerFieldError tests sizeContainer error propagation
// for dynamic fields with unsupported types.
func TestSizeDynamicContainerFieldError(t *testing.T) {
	unsupportedNestedDesc := &dynssz.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      dynssz.SszType(255),
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
		Size:         0,
	}

	dynamicFieldDesc := &dynssz.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      dynssz.SszListType,
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Slice,
		ElemDesc:     unsupportedNestedDesc,
		Limit:        100,
	}

	containerDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &dynssz.ContainerDescriptor{
			Fields: []dynssz.FieldDescriptor{
				{
					Name: "DynamicField",
					Type: dynamicFieldDesc,
				},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateSize(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for dynamic field with nested unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestSizeListWithNestedUnsupportedType tests that sizeList propagates
// errors from nested unsupported element types when dynamic elements are present.
func TestSizeListWithNestedUnsupportedType(t *testing.T) {
	unsupportedElemDesc := &dynssz.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      dynssz.SszType(255),
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
		Size:         0,
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

	err := generateSize(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported element type in list, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestSizeUnionWithNestedUnsupportedType tests that sizeUnion propagates
// errors from nested unsupported variant types.
func TestSizeUnionWithNestedUnsupportedType(t *testing.T) {
	unsupportedVariantDesc := &dynssz.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      dynssz.SszType(255),
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Struct,
		Size:         0,
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

	err := generateSize(unionDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported variant type in union, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestSizeTypeWrapperWithNestedUnsupportedType tests that sizeType handles
// TypeWrapper with nested unsupported types.
func TestSizeTypeWrapperWithNestedUnsupportedType(t *testing.T) {
	unsupportedElemDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Struct,
		Size:    0,
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

	err := generateSize(wrapperDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type in TypeWrapper, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestSizeProgressiveListError tests that progressive lists properly propagate errors.
func TestSizeProgressiveListError(t *testing.T) {
	unsupportedElemDesc := &dynssz.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      dynssz.SszType(255),
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
		Size:         0,
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

	err := generateSize(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for progressive list with unsupported element type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}
