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

// TestGenerateDecoderUnsupportedType tests that generateDecoder returns an error
// for unsupported SSZ types.
func TestGenerateDecoderUnsupportedType(t *testing.T) {
	desc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateDecoder(desc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for unsupported SSZ type, got nil")
	}
}

// TestGenerateDecoderBigIntType tests generateDecoder with BigInt type.
func TestGenerateDecoderBigIntType(t *testing.T) {
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

	err := generateDecoder(bigIntDesc, codeBuilder, typePrinter, "", options)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestGenerateDecoderOptionalError tests generateDecoder error propagation for optional types.
func TestGenerateDecoderOptionalError(t *testing.T) {
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

	err := generateDecoder(optionalDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for optional with unsupported inner type")
	}
}

// TestGenerateDecoderDynamicUnmarshaler tests the DynamicUnmarshaler compat flag path.
func TestGenerateDecoderDynamicUnmarshaler(t *testing.T) {
	childDesc := &ssztypes.TypeDescriptor{
		Type:           testDummyReflectType,
		SszType:        ssztypes.SszContainerType,
		Kind:           reflect.Struct,
		Size:           8,
		SszCompatFlags: ssztypes.SszCompatFlagDynamicUnmarshaler,
		ContainerDesc:  &ssztypes.ContainerDescriptor{},
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "Child", Type: childDesc},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateDecoder(containerDesc, codeBuilder, typePrinter, "", options)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestGenerateDecoderVectorError tests generateDecoder error for vector with unsupported elements.
func TestGenerateDecoderVectorError(t *testing.T) {
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

	err := generateDecoder(vectorDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for vector with unsupported element type")
	}
}

// TestGenerateDecoderListError tests generateDecoder error for list with unsupported elements.
func TestGenerateDecoderListError(t *testing.T) {
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

	err := generateDecoder(listDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for list with unsupported element type")
	}
}

// TestGenerateDecoderContainerError tests generateDecoder error propagation for containers.
func TestGenerateDecoderContainerError(t *testing.T) {
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

	err := generateDecoder(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with unsupported field type")
	}
}
