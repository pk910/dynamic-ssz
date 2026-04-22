// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// testSingleFieldStruct is a struct with a single field used for wrapper tests.
type testSingleFieldStruct struct {
	Payload uint32
}

// testTwoFieldStruct is a struct with two fields used for wrapper tests.
type testTwoFieldStruct struct {
	A uint32
	B uint32
}

// testWrapperListStruct is a single-field struct wrapping a dynamic list,
// used to exercise the TypeWrapper success paths in the generators.
type testWrapperListStruct struct {
	Payload []uint32
}

// TestWriteIndentedTrailingContent tests writeIndented with content that does not end in newline.
func TestWriteIndentedTrailingContent(t *testing.T) {
	var b strings.Builder
	writeIndented(&b, "hello", 1)
	if b.String() != "\thello" {
		t.Errorf("expected '\\thello', got %q", b.String())
	}
}

// TestWriteIndentedMultiLine tests writeIndented with multiple lines including trailing content.
func TestWriteIndentedMultiLine(t *testing.T) {
	var b strings.Builder
	writeIndented(&b, "a\nb\nc", 1)
	expected := "\ta\n\tb\n\tc"
	if b.String() != expected {
		t.Errorf("expected %q, got %q", expected, b.String())
	}
}

// TestEscapeBackticks tests escapeBackticks with and without backticks.
func TestEscapeBackticks(t *testing.T) {
	// Without backticks - should return unchanged
	result := escapeBackticks("hello world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}

	// With backticks - strconv.Quote keeps them as-is in double-quoted strings,
	// but the function still processes them through Quote to handle any other special chars.
	input := "hello \x60world\x60"
	result = escapeBackticks(input)
	if !strings.Contains(result, "world") {
		t.Errorf("expected result containing 'world', got %q", result)
	}

	// With special chars alongside backticks
	input = "test\x60\ttab\x60"
	result = escapeBackticks(input)
	if !strings.Contains(result, "\\t") {
		t.Errorf("expected tab to be escaped, got %q", result)
	}
}

// TestIndentStr tests indentStr with various inputs.
func TestIndentStr(t *testing.T) {
	// Zero indent
	result := indentStr("hello", 0)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}

	// Empty string
	result = indentStr("", 1)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}

	// String without trailing newline
	result = indentStr("hello", 1)
	if result != "\thello" {
		t.Errorf("expected '\\thello', got %q", result)
	}
}

// TestGetTypeWrapperFieldName covers the branches of getTypeWrapperFieldName
// that determine the wrapper's data-field name from either go/types CodegenInfo
// or the reflection-based descriptor.
func TestGetTypeWrapperFieldName(t *testing.T) {
	t.Run("WrongSszType", func(t *testing.T) {
		desc := &ssztypes.TypeDescriptor{SszType: ssztypes.SszContainerType}
		if got := getTypeWrapperFieldName(desc); got != "" {
			t.Errorf("expected empty string for non-wrapper type, got %q", got)
		}
	})

	// Helper: build a TypeDescriptor with CodegenInfo pointing at a go/types type.
	newDescWithCodegenInfo := func(goType types.Type) *ssztypes.TypeDescriptor {
		info := &CodegenInfo{Type: goType}
		var anyInfo any = info
		return &ssztypes.TypeDescriptor{
			SszType:     ssztypes.SszTypeWrapperType,
			CodegenInfo: &anyInfo,
		}
	}

	t.Run("CodegenInfoNonStructInner", func(t *testing.T) {
		// Inner type is a slice, not a struct → returns "".
		sliceType := types.NewSlice(types.Typ[types.Uint32])
		desc := newDescWithCodegenInfo(sliceType)
		if got := getTypeWrapperFieldName(desc); got != "" {
			t.Errorf("expected empty string for non-struct inner, got %q", got)
		}
	})

	t.Run("CodegenInfoStructWrongFieldCount", func(t *testing.T) {
		// Struct with two fields → wrong count, returns "".
		fieldA := types.NewVar(token.NoPos, nil, "A", types.Typ[types.Uint32])
		fieldB := types.NewVar(token.NoPos, nil, "B", types.Typ[types.Uint32])
		structType := types.NewStruct([]*types.Var{fieldA, fieldB}, []string{"", ""})
		desc := newDescWithCodegenInfo(structType)
		if got := getTypeWrapperFieldName(desc); got != "" {
			t.Errorf("expected empty string for struct with 2 fields, got %q", got)
		}
	})

	t.Run("CodegenInfoStructOneFieldSuccess", func(t *testing.T) {
		// Struct with a single named field → returns that field's name.
		field := types.NewVar(token.NoPos, nil, "Payload", types.Typ[types.Uint32])
		structType := types.NewStruct([]*types.Var{field}, []string{""})
		desc := newDescWithCodegenInfo(structType)
		if got := getTypeWrapperFieldName(desc); got != "Payload" {
			t.Errorf("expected 'Payload', got %q", got)
		}
	})

	t.Run("CodegenInfoPointerToStruct", func(t *testing.T) {
		// CodegenInfo wrapping a pointer to a single-field struct unwraps correctly.
		field := types.NewVar(token.NoPos, nil, "Payload", types.Typ[types.Uint32])
		structType := types.NewStruct([]*types.Var{field}, []string{""})
		ptrType := types.NewPointer(structType)
		desc := newDescWithCodegenInfo(ptrType)
		if got := getTypeWrapperFieldName(desc); got != "Payload" {
			t.Errorf("expected 'Payload' through pointer, got %q", got)
		}
	})

	t.Run("ReflectionStructOneFieldSuccess", func(t *testing.T) {
		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
			Type:    reflect.TypeFor[testSingleFieldStruct](),
		}
		if got := getTypeWrapperFieldName(desc); got != "Payload" {
			t.Errorf("expected 'Payload' from reflection, got %q", got)
		}
	})

	t.Run("ReflectionStructWrongFieldCount", func(t *testing.T) {
		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
			Type:    reflect.TypeFor[testTwoFieldStruct](),
		}
		if got := getTypeWrapperFieldName(desc); got != "" {
			t.Errorf("expected empty string for 2-field reflection struct, got %q", got)
		}
	})

	t.Run("ReflectionNilType", func(t *testing.T) {
		desc := &ssztypes.TypeDescriptor{SszType: ssztypes.SszTypeWrapperType}
		if got := getTypeWrapperFieldName(desc); got != "" {
			t.Errorf("expected empty string for nil reflect type, got %q", got)
		}
	})

	t.Run("ReflectionNonStruct", func(t *testing.T) {
		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
			Type:    reflect.TypeFor[uint32](),
		}
		if got := getTypeWrapperFieldName(desc); got != "" {
			t.Errorf("expected empty string for non-struct reflect type, got %q", got)
		}
	})
}
