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

// TestGenerateUnmarshalUnsupportedType tests that generateUnmarshal returns an error
// for unsupported SSZ types.
func TestGenerateUnmarshalUnsupportedType(t *testing.T) {
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

			err := generateUnmarshal(desc, codeBuilder, typePrinter, "", options)
			if err == nil {
				t.Error("expected error for unsupported SSZ type, got nil")
			}
			if !strings.Contains(err.Error(), "unsupported SSZ type") {
				t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
			}
		})
	}
}

// TestUnmarshalContainerWithNestedUnsupportedType tests that unmarshalContainer propagates
// errors from nested unsupported types.
func TestUnmarshalContainerWithNestedUnsupportedType(t *testing.T) {
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

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, "", options)
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
	unsupportedNestedDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	dynamicFieldDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
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

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, "", options)
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
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(vectorDesc, codeBuilder, typePrinter, "", options)
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
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
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

	err := generateUnmarshal(listDesc, codeBuilder, typePrinter, "", options)
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(unionDesc, codeBuilder, typePrinter, "", options)
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
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
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

	err := generateUnmarshal(wrapperDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for nested unsupported type in TypeWrapper, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalOptionalError tests that unmarshalOptional propagates errors from inner types.
func TestUnmarshalOptionalError(t *testing.T) {
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
	options := &CodeGeneratorOptions{ExtendedTypes: true}

	err := generateUnmarshal(optionalDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for optional with unsupported inner type")
	}
}

// TestUnmarshalBigIntType tests that unmarshalBigInt generates code.
func TestUnmarshalBigIntType(t *testing.T) {
	bigIntDesc := &ssztypes.TypeDescriptor{
		Type:         testDummyReflectType,
		SszType:      ssztypes.SszBigIntType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Kind:         reflect.Struct,
		Size:         0,
	}

	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	options := &CodeGeneratorOptions{ExtendedTypes: true}

	err := generateUnmarshal(bigIntDesc, codeBuilder, typePrinter, "", options)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestUnmarshalProgressiveListError tests that progressive lists properly propagate errors.
func TestUnmarshalProgressiveListError(t *testing.T) {
	unsupportedElemDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Uint8, // Use Uint8 to avoid getPtrPrefix returning "*" which triggers InnerTypeString
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

	err := generateUnmarshal(listDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for progressive list with unsupported element type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported SSZ type") {
		t.Errorf("expected error containing 'unsupported SSZ type', got: %v", err)
	}
}

// TestUnmarshalVectorWithUnsupportedElement tests error propagation from vector element generation.
func TestUnmarshalVectorWithUnsupportedElement(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(vectorDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for vector with unsupported element type")
	}
}

// TestUnmarshalListWithUnsupportedElement tests error propagation from list element generation.
func TestUnmarshalListWithUnsupportedElement(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(listDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for list with unsupported element type")
	}
}

// TestUnmarshalOptionalWithUnsupportedInner tests error propagation from optional inner type.
func TestUnmarshalOptionalWithUnsupportedInner(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for optional with unsupported inner type")
	}
}

// TestUnmarshalUnionWithUnsupportedVariant tests error propagation from union variant.
func TestUnmarshalUnionWithUnsupportedVariant(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(unionDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for union with unsupported variant type")
	}
}

// TestUnmarshalTypeWrapperWithUnsupportedInner tests error propagation from wrapper inner type.
func TestUnmarshalTypeWrapperWithUnsupportedInner(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for wrapper with unsupported inner type")
	}
}

// TestUnmarshalContainerWithVectorOfUnsupported tests error propagation when a
// container has a vector field whose elements are unsupported.
func TestUnmarshalContainerWithVectorOfUnsupported(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with vector of unsupported elements")
	}
}

// TestUnmarshalContainerWithListOfUnsupported tests error propagation when a
// container has a dynamic list field whose elements are unsupported.
func TestUnmarshalContainerWithListOfUnsupported(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(containerDesc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with list of unsupported elements")
	}
}

// TestUnmarshalProgressiveContainerError tests that progressive containers
// properly propagate errors for unmarshal generation.
func TestUnmarshalProgressiveContainerError(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(desc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for progressive container with unsupported field")
	}
}

// TestUnmarshalNestedContainerUnsupportedField tests that a container with a nested
// container whose field is unsupported propagates the error through two levels.
func TestUnmarshalNestedContainerUnsupportedField(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(outerContainer, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for nested container with unsupported field")
	}
}

// TestUnmarshalContainerStaticPlusDynamicError tests that a container with a
// static uint32 field and a dynamic unsupported field propagates the error
// from the dynamic field path.
func TestUnmarshalContainerStaticPlusDynamicError(t *testing.T) {
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
	options := &CodeGeneratorOptions{}

	err := generateUnmarshal(desc, codeBuilder, typePrinter, "", options)
	if err == nil {
		t.Error("expected error for container with static + dynamic unsupported field")
	}
}
