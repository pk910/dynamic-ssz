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

// testDummyType is a dummy type for testing code generation error paths.
type testDummyType struct{}

// testDummyArrayType is a dummy array type for testing vector error paths.
type testDummyArrayType [10]testDummyType

// testDummySliceType is a dummy slice type for testing list error paths.
type testDummySliceType []testDummyType

// testDummyReflectType returns a reflect.Type for testing structs.
var testDummyReflectType = reflect.TypeOf(testDummyType{})

// testDummyArrayReflectType returns a reflect.Type for testing arrays/vectors.
var testDummyArrayReflectType = reflect.TypeOf(testDummyArrayType{})

// testDummySliceReflectType returns a reflect.Type for testing slices/lists.
var testDummySliceReflectType = reflect.TypeOf(testDummySliceType{})

// TestGenerateMarshalUnsupportedType tests that generateMarshal returns an error
// for unsupported SSZ types.
func TestGenerateMarshalUnsupportedType(t *testing.T) {
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

			err := generateMarshal(desc, codeBuilder, typePrinter, options)
			if err == nil {
				t.Error("expected error for unsupported SSZ type, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported SSZ type") {
				t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
			}
		})
	}
}

// TestMarshalContainerWithNestedUnsupportedType tests that marshalContainer propagates
// errors from nested unsupported types.
func TestMarshalContainerWithNestedUnsupportedType(t *testing.T) {
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

	err := generateMarshal(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestMarshalDynamicContainerFieldError tests marshalContainer error propagation
// for dynamic fields with unsupported types.
func TestMarshalDynamicContainerFieldError(t *testing.T) {
	unsupportedNestedDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszType(255),
		Kind:    reflect.Struct,
	}

	dynamicFieldDesc := &dynssz.TypeDescriptor{
		Type:         testDummyReflectType,
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

	err := generateMarshal(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for dynamic field with nested unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestMarshalVectorWithNestedUnsupportedType tests that marshalVector propagates
// errors from nested unsupported element types.
func TestMarshalVectorWithNestedUnsupportedType(t *testing.T) {
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

	err := generateMarshal(vectorDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported element type in vector, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestMarshalListWithNestedUnsupportedType tests that marshalList propagates
// errors from nested unsupported element types.
func TestMarshalListWithNestedUnsupportedType(t *testing.T) {
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

	err := generateMarshal(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported element type in list, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestMarshalUnionWithNestedUnsupportedType tests that marshalUnion propagates
// errors from nested unsupported variant types.
func TestMarshalUnionWithNestedUnsupportedType(t *testing.T) {
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

	err := generateMarshal(unionDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported variant type in union, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestMarshalTypeWrapperWithNestedUnsupportedType tests that marshalType handles
// TypeWrapper with nested unsupported types.
func TestMarshalTypeWrapperWithNestedUnsupportedType(t *testing.T) {
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

	err := generateMarshal(wrapperDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type in TypeWrapper, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestMarshalProgressiveContainerError tests that progressive containers also
// properly propagate errors.
func TestMarshalProgressiveContainerError(t *testing.T) {
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
					Name: "UnsupportedField",
					Type: unsupportedFieldDesc,
				},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{}

	err := generateMarshal(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for progressive container with unsupported field, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestMarshalProgressiveListError tests that progressive lists properly propagate errors.
func TestMarshalProgressiveListError(t *testing.T) {
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

	err := generateMarshal(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for progressive list with unsupported element type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}
