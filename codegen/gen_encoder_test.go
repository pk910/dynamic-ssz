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

// TestGenerateEncoderUnsupportedType tests that generateEncoder returns an error
// for unsupported SSZ types.
func TestGenerateEncoderUnsupportedType(t *testing.T) {
	desc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(desc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for unsupported SSZ type, got nil")
	}
}

// TestGenerateEncoderBigIntType tests generateEncoder with BigInt type.
func TestGenerateEncoderBigIntType(t *testing.T) {
	bigIntDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszBigIntType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Struct,
		Size:         0,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true, ExtendedTypes: true}

	err := generateEncoder(bigIntDesc, codeBuilder, typePrinter, "", options)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestGenerateEncoderOptionalError tests generateEncoder error propagation for optional types.
func TestGenerateEncoderOptionalError(t *testing.T) {
	unsupportedDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	optionalDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszOptionalType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Pointer,
		ElemDesc:     unsupportedDesc,
		GoTypeFlags:  ssztypes.GoTypeFlagIsPointer,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true, ExtendedTypes: true}

	err := generateEncoder(optionalDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for optional with unsupported inner type")
	}
}

// TestGenerateEncoderContainerError tests generateEncoder error propagation for containers.
func TestGenerateEncoderContainerError(t *testing.T) {
	unsupportedFieldDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "Bad", Type: unsupportedFieldDesc},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with unsupported field type")
	}
}

// TestGenerateEncoderVectorError tests encoder error for vector with unsupported elements.
func TestGenerateEncoderVectorError(t *testing.T) {
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
	}

	vectorDesc := &ssztypes.TypeDescriptor{
		Type:     testDummyArrayReflectType,
		SszType:  ssztypes.SszVectorType,
		Kind:     reflect.Array,
		ElemDesc: unsupportedElemDesc,
		Len:      10,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(vectorDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for vector with unsupported element type")
	}
}

// TestGenerateEncoderListError tests encoder error for list with unsupported elements.
func TestGenerateEncoderListError(t *testing.T) {
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
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
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(listDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for list with unsupported element type")
	}
}

// TestGenerateEncoderUnionError tests encoder error for union with unsupported variant.
func TestGenerateEncoderUnionError(t *testing.T) {
	unsupportedVariantDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
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
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(unionDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for union with unsupported variant type")
	}
}

// TestGenerateEncoderWithoutDynamicExpressions tests that WithoutDynamicExpressions
// is properly overridden for encoder generation.
func TestGenerateEncoderWithoutDynamicExpressions(t *testing.T) {
	desc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszBoolType,
		Kind:    reflect.Bool,
		Size:    1,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{
		CreateEncoderFn:           true,
		WithoutDynamicExpressions: true, // should be overridden for encoder
	}

	err := generateEncoder(desc, codeBuilder, typePrinter, "", options)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
