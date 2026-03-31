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

// TestEncoderVectorWithUnsupportedElement tests error propagation from vector element generation.
func TestEncoderVectorWithUnsupportedElement(t *testing.T) {
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
		Size:     10,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(vectorDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for vector with unsupported element type")
	}
}

// TestEncoderListWithUnsupportedElement tests error propagation from list element generation.
func TestEncoderListWithUnsupportedElement(t *testing.T) {
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszType(255),
		Kind:         reflect.Uint8,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
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

// TestEncoderOptionalWithUnsupportedInner tests error propagation from optional inner type.
func TestEncoderOptionalWithUnsupportedInner(t *testing.T) {
	unsupportedInner := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
	}

	optDesc := &ssztypes.TypeDescriptor{
		Type:     testDummyReflectType,
		SszType:  ssztypes.SszOptionalType,
		Kind:     reflect.Pointer,
		ElemDesc: unsupportedInner,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "F", Type: optDesc},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true, ExtendedTypes: true}

	err := generateEncoder(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for optional with unsupported inner type")
	}
}

// TestEncoderUnionWithUnsupportedVariant tests error propagation from union variant.
func TestEncoderUnionWithUnsupportedVariant(t *testing.T) {
	unsupportedVariant := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
	}

	unionDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszCompatibleUnionType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Struct,
		UnionVariants: map[uint8]*ssztypes.TypeDescriptor{
			0: unsupportedVariant,
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

// TestEncoderTypeWrapperWithUnsupportedInner tests error propagation from wrapper inner type.
func TestEncoderTypeWrapperWithUnsupportedInner(t *testing.T) {
	unsupportedInner := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
	}

	wrapperDesc := &ssztypes.TypeDescriptor{
		Type:     testDummyReflectType,
		SszType:  ssztypes.SszTypeWrapperType,
		Kind:     reflect.Struct,
		ElemDesc: unsupportedInner,
		Size:     8,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "W", Type: wrapperDesc},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for wrapper with unsupported inner type")
	}
}

// TestEncoderContainerWithVectorOfUnsupported tests error propagation when a
// container has a vector field whose elements are unsupported.
func TestEncoderContainerWithVectorOfUnsupported(t *testing.T) {
	unsupportedElem := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
	}

	vectorField := &ssztypes.TypeDescriptor{
		Type:     testDummyArrayReflectType,
		SszType:  ssztypes.SszVectorType,
		Kind:     reflect.Array,
		ElemDesc: unsupportedElem,
		Len:      4,
		Size:     4,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "V", Type: vectorField},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with vector of unsupported elements")
	}
}

// TestEncoderContainerWithListOfUnsupported tests error propagation when a
// container has a dynamic list field whose elements are unsupported.
func TestEncoderContainerWithListOfUnsupported(t *testing.T) {
	unsupportedElem := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszType(255),
		Kind:         reflect.Uint8,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
	}

	listField := &ssztypes.TypeDescriptor{
		Type:         testDummySliceReflectType,
		SszType:      ssztypes.SszListType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Slice,
		ElemDesc:     unsupportedElem,
		Limit:        10,
	}

	containerDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "L", Type: listField},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with list of unsupported elements")
	}
}

// TestEncoderProgressiveContainerError tests that progressive containers
// properly propagate errors for encoder generation.
func TestEncoderProgressiveContainerError(t *testing.T) {
	unsupportedField := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
	}

	desc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszProgressiveContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "F", Type: unsupportedField},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(desc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for progressive container with unsupported field")
	}
}

// TestEncoderProgressiveListError tests that progressive lists properly
// propagate errors for encoder generation.
func TestEncoderProgressiveListError(t *testing.T) {
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
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
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(listDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for progressive list with unsupported element type")
	}
}

// TestEncoderNestedContainerUnsupportedField tests that a container with a nested
// container whose field is unsupported propagates the error through two levels.
func TestEncoderNestedContainerUnsupportedField(t *testing.T) {
	unsupportedField := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8,
	}

	innerContainer := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "Bad", Type: unsupportedField},
			},
		},
	}

	outerContainer := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "Inner", Type: innerContainer},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(outerContainer, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for nested container with unsupported field")
	}
}

// TestEncoderContainerStaticPlusDynamicError tests that a container with a
// static uint32 field and a dynamic unsupported field propagates the error
// from the dynamic field path.
func TestEncoderContainerStaticPlusDynamicError(t *testing.T) {
	staticField := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszUint32Type,
		Kind:    reflect.Uint32,
		Size:    4,
	}

	unsupportedDynField := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszType(255),
		Kind:         reflect.Uint8,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
	}

	desc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		ContainerDesc: &ssztypes.ContainerDescriptor{
			Fields: []ssztypes.FieldDescriptor{
				{Name: "S", Type: staticField},
				{Name: "D", Type: unsupportedDynField},
			},
		},
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{CreateEncoderFn: true}

	err := generateEncoder(desc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with static + dynamic unsupported field")
	}
}
