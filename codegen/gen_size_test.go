// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// TestGenerateSizeUnsupportedType tests that generateSize returns an error
// for unsupported SSZ types.
func TestGenerateSizeUnsupportedType(t *testing.T) {
	tests := []struct {
		name    string
		sszType ssztypes.SszType
	}{
		{"UnsupportedType_255", ssztypes.SszType(255)},
		{"UnsupportedType_100", ssztypes.SszType(100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := &ssztypes.TypeDescriptor{
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
	unsupportedFieldDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
		Size:    0,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
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
	unsupportedNestedDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszType(255),
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
		Size:         0,
	}

	dynamicFieldDesc := &ssztypes.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      ssztypes.SszListType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Slice,
		ElemDesc:     unsupportedNestedDesc,
		Limit:        100,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
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
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszType(255),
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
		Size:         0,
	}

	listDesc := &ssztypes.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      ssztypes.SszListType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
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
	unsupportedVariantDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszType(255),
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Struct,
		Size:         0,
	}

	unionDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszCompatibleUnionType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Struct,
		UnionVariants: map[uint8]*ssztypes.TypeDescriptor{
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
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
		Size:    0,
	}

	wrapperDesc := &ssztypes.TypeDescriptor{
		Type:     testDummyReflectType,
		SszType:  ssztypes.SszTypeWrapperType,
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

// TestSizeExtendedTypes tests that sizeType handles extended SSZ types correctly.
func TestSizeExtendedTypes(t *testing.T) {
	extendedTypes := []struct {
		name    string
		sszType ssztypes.SszType
		size    uint32
	}{
		{"Int8", ssztypes.SszInt8Type, 1},
		{"Int16", ssztypes.SszInt16Type, 2},
		{"Int32", ssztypes.SszInt32Type, 4},
		{"Int64", ssztypes.SszInt64Type, 8},
		{"Float32", ssztypes.SszFloat32Type, 4},
		{"Float64", ssztypes.SszFloat64Type, 8},
		{"CustomType", ssztypes.SszCustomType, 0},
	}

	for _, et := range extendedTypes {
		t.Run(et.name, func(t *testing.T) {
			fieldDesc := &ssztypes.TypeDescriptor{
				Type:    testDummyReflectType,
				SszType: et.sszType,
				Kind:    reflect.Struct,
				Size:    et.size,
			}

			// Wrap in container so sizeType is invoked for the field
			// Use Size=0 to force sizeContainer to go through sizeType path
			containerDesc := &ssztypes.TypeDescriptor{
				Type:    testDummyReflectType,
				SszType: ssztypes.SszContainerType,
				Kind:    reflect.Struct,
				ContainerDesc: &ssztypes.ContainerDescriptor{
					Fields: []ssztypes.FieldDescriptor{
						{Name: "F1", Type: fieldDesc},
					},
				},
			}

			// Force the field to not take the static shortcut
			// by setting SszTypeFlagHasSizeExpr and NOT setting WithoutDynamicExpressions
			fieldDesc.SszTypeFlags |= ssztypes.SszTypeFlagHasSizeExpr
			expr := "SOME_EXPR"
			fieldDesc.SizeExpression = &expr

			codeBuilder := &strings.Builder{}
			typePrinter := NewTypePrinter("test/package")
			options := &CodeGeneratorOptions{ExtendedTypes: true}

			err := generateSize(containerDesc, codeBuilder, typePrinter, options)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestSizeOptionalError tests that sizeOptional propagates errors.
func TestSizeOptionalError(t *testing.T) {
	unsupportedDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	optionalDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszOptionalType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Ptr,
		ElemDesc:     unsupportedDesc,
		GoTypeFlags:  ssztypes.GoTypeFlagIsPointer,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "Opt", Type: optionalDesc},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{ExtendedTypes: true}

	err := generateSize(containerDesc, codeBuilder, typePrinter, options)
	if err == nil {
		t.Error("expected error for optional with unsupported inner type")
	}
}

// TestSizeProgressiveListError tests that progressive lists properly propagate errors.
func TestSizeProgressiveListError(t *testing.T) {
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszType(255),
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
		Size:         0,
	}

	listDesc := &ssztypes.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      ssztypes.SszProgressiveListType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
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
