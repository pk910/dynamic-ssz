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

// TestGenerateUnmarshalUnsupportedType tests that generateUnmarshal returns an error
// for unsupported SSZ types.
func TestGenerateUnmarshalUnsupportedType(t *testing.T) {
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

			err := generateUnmarshal(desc, codeBuilder, typePrinter, options)
			if err == nil {
				t.Error("expected error for unsupported SSZ type, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported SSZ type") {
				t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
			}
		})
	}
}

// TestUnmarshalContextGetStaticSizeVarDynamicField tests that getStaticSizeVar returns
// an error when trying to calculate static size for a container with dynamic fields.
func TestUnmarshalContextGetStaticSizeVarDynamicField(t *testing.T) {
	dynamicFieldDesc := &dynssz.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      dynssz.SszListType,
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Slice,
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

	ctx := &unmarshalContext{
		appendCode:     func(int, string, ...any) {},
		appendSizeCode: func(int, string, ...any) {},
		appendExprCode: func(int, string, ...any) {},
		typePrinter:    NewTypePrinter("test/package"),
		options:        &CodeGeneratorOptions{},
		sizeVarMap:     make(map[[32]byte]string),
		exprVarMap:     make(map[[32]byte]string),
	}

	_, err := ctx.getStaticSizeVar(containerDesc)
	if err == nil {
		t.Error("expected error for dynamic field in container, got nil")
	}
	if !strings.Contains(err.Error(), "dynamic field not supported") {
		t.Errorf("expected error containing 'dynamic field not supported', got: %v", err)
	}
}

// TestUnmarshalContextGetStaticSizeVarDynamicVector tests that getStaticSizeVar returns
// an error when trying to calculate static size for a vector with dynamic elements.
func TestUnmarshalContextGetStaticSizeVarDynamicVector(t *testing.T) {
	dynamicElemDesc := &dynssz.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      dynssz.SszListType,
		SszTypeFlags: dynssz.SszTypeFlagIsDynamic,
		Kind:         reflect.Slice,
	}

	vectorDesc := &dynssz.TypeDescriptor{
		Type:     testDummyArrayReflectType,
		SszType:  dynssz.SszVectorType,
		Kind:     reflect.Array,
		ElemDesc: dynamicElemDesc,
		Len:      10,
	}

	ctx := &unmarshalContext{
		appendCode:     func(int, string, ...any) {},
		appendSizeCode: func(int, string, ...any) {},
		appendExprCode: func(int, string, ...any) {},
		typePrinter:    NewTypePrinter("test/package"),
		options:        &CodeGeneratorOptions{},
		sizeVarMap:     make(map[[32]byte]string),
		exprVarMap:     make(map[[32]byte]string),
	}

	_, err := ctx.getStaticSizeVar(vectorDesc)
	if err == nil {
		t.Error("expected error for dynamic vector elements, got nil")
	}
	if !strings.Contains(err.Error(), "dynamic vector not supported") {
		t.Errorf("expected error containing 'dynamic vector not supported', got: %v", err)
	}
}

// TestUnmarshalContextGetStaticSizeVarUnknownType tests that getStaticSizeVar returns
// an error for unknown SSZ types.
func TestUnmarshalContextGetStaticSizeVarUnknownType(t *testing.T) {
	unknownDesc := &dynssz.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: dynssz.SszListType,
		Kind:    reflect.Slice,
	}

	ctx := &unmarshalContext{
		appendCode:     func(int, string, ...any) {},
		appendSizeCode: func(int, string, ...any) {},
		appendExprCode: func(int, string, ...any) {},
		typePrinter:    NewTypePrinter("test/package"),
		options:        &CodeGeneratorOptions{},
		sizeVarMap:     make(map[[32]byte]string),
		exprVarMap:     make(map[[32]byte]string),
	}

	_, err := ctx.getStaticSizeVar(unknownDesc)
	if err == nil {
		t.Error("expected error for unknown type in static size calculation, got nil")
	}
	if !strings.Contains(err.Error(), "unknown type for static size calculation") {
		t.Errorf("expected error containing 'unknown type for static size calculation', got: %v", err)
	}
}

// TestUnmarshalContainerWithNestedUnsupportedType tests that unmarshalContainer propagates
// errors from nested unsupported types.
func TestUnmarshalContainerWithNestedUnsupportedType(t *testing.T) {
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

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalDynamicContainerFieldError tests unmarshalContainer error propagation
// for dynamic fields with unsupported types.
func TestUnmarshalDynamicContainerFieldError(t *testing.T) {
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

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for dynamic field with nested unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalVectorWithNestedUnsupportedType tests that unmarshalVector propagates
// errors from nested unsupported element types.
func TestUnmarshalVectorWithNestedUnsupportedType(t *testing.T) {
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

	err := generateUnmarshal(vectorDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported element type in vector, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalListWithNestedUnsupportedType tests that unmarshalList propagates
// errors from nested unsupported element types.
func TestUnmarshalListWithNestedUnsupportedType(t *testing.T) {
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

	err := generateUnmarshal(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported element type in list, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalUnionWithNestedUnsupportedType tests that unmarshalUnion propagates
// errors from nested unsupported variant types.
func TestUnmarshalUnionWithNestedUnsupportedType(t *testing.T) {
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

	err := generateUnmarshal(unionDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported variant type in union, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalTypeWrapperWithNestedUnsupportedType tests that unmarshalType handles
// TypeWrapper with nested unsupported types.
func TestUnmarshalTypeWrapperWithNestedUnsupportedType(t *testing.T) {
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

	err := generateUnmarshal(wrapperDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for nested unsupported type in TypeWrapper, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalProgressiveListError tests that progressive lists properly propagate errors.
func TestUnmarshalProgressiveListError(t *testing.T) {
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

	err := generateUnmarshal(listDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for progressive list with unsupported element type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}
