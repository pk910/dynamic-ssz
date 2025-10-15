// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"go/types"
	"reflect"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

// Simple test types for API testing
type SimpleTestStruct struct {
	Field1 uint64 `ssz-size:"8"`
	Field2 bool
}

type SimpleTestStruct2 struct {
	Field1 uint32
	Field2 uint16
}

func TestCodeGeneratorOptions(t *testing.T) {
	t.Run("WithNoMarshalSSZ", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithNoMarshalSSZ()
		option(&opts)
		if !opts.NoMarshalSSZ {
			t.Error("WithNoMarshalSSZ should set NoMarshalSSZ to true")
		}
	})

	t.Run("WithNoUnmarshalSSZ", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithNoUnmarshalSSZ()
		option(&opts)
		if !opts.NoUnmarshalSSZ {
			t.Error("WithNoUnmarshalSSZ should set NoUnmarshalSSZ to true")
		}
	})

	t.Run("WithNoSizeSSZ", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithNoSizeSSZ()
		option(&opts)
		if !opts.NoSizeSSZ {
			t.Error("WithNoSizeSSZ should set NoSizeSSZ to true")
		}
	})

	t.Run("WithNoHashTreeRoot", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithNoHashTreeRoot()
		option(&opts)
		if !opts.NoHashTreeRoot {
			t.Error("WithNoHashTreeRoot should set NoHashTreeRoot to true")
		}
	})

	t.Run("WithCreateLegacyFn", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithCreateLegacyFn()
		option(&opts)
		if !opts.CreateLegacyFn {
			t.Error("WithCreateLegacyFn should set CreateLegacyFn to true")
		}
	})

	t.Run("WithoutDynamicExpressions", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithoutDynamicExpressions()
		option(&opts)
		if !opts.WithoutDynamicExpressions {
			t.Error("WithoutDynamicExpressions should set WithoutDynamicExpressions to true")
		}
	})

	t.Run("WithNoFastSsz", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithNoFastSsz()
		option(&opts)
		if !opts.NoFastSsz {
			t.Error("WithNoFastSsz should set NoFastSsz to true")
		}
	})
}

func TestCodeGeneratorHints(t *testing.T) {
	t.Run("WithSizeHints", func(t *testing.T) {
		hints := []dynssz.SszSizeHint{
			{Size: 32, Expr: "BYTES_PER_FIELD_ELEMENT"},
			{Size: 64, Expr: "SLOTS_PER_EPOCH"},
		}
		opts := CodeGeneratorOptions{}
		option := WithSizeHints(hints)
		option(&opts)

		if len(opts.SizeHints) != 2 {
			t.Errorf("Expected 2 size hints, got %d", len(opts.SizeHints))
		}
		if opts.SizeHints[0].Size != 32 || opts.SizeHints[0].Expr != "BYTES_PER_FIELD_ELEMENT" {
			t.Error("First size hint not set correctly")
		}
		if opts.SizeHints[1].Size != 64 || opts.SizeHints[1].Expr != "SLOTS_PER_EPOCH" {
			t.Error("Second size hint not set correctly")
		}
	})

	t.Run("WithMaxSizeHints", func(t *testing.T) {
		hints := []dynssz.SszMaxSizeHint{
			{Size: 1048576, Expr: "MAX_VALIDATORS"},
			{Size: 4096, Expr: "MAX_COMMITTEES"},
		}
		opts := CodeGeneratorOptions{}
		option := WithMaxSizeHints(hints)
		option(&opts)

		if len(opts.MaxSizeHints) != 2 {
			t.Errorf("Expected 2 max size hints, got %d", len(opts.MaxSizeHints))
		}
		if opts.MaxSizeHints[0].Size != 1048576 || opts.MaxSizeHints[0].Expr != "MAX_VALIDATORS" {
			t.Error("First max size hint not set correctly")
		}
		if opts.MaxSizeHints[1].Size != 4096 || opts.MaxSizeHints[1].Expr != "MAX_COMMITTEES" {
			t.Error("Second max size hint not set correctly")
		}
	})

	t.Run("WithTypeHints", func(t *testing.T) {
		hints := []dynssz.SszTypeHint{
			{Type: dynssz.SszListType},
			{Type: dynssz.SszContainerType},
		}
		opts := CodeGeneratorOptions{}
		option := WithTypeHints(hints)
		option(&opts)

		if len(opts.TypeHints) != 2 {
			t.Errorf("Expected 2 type hints, got %d", len(opts.TypeHints))
		}
		if opts.TypeHints[0].Type != dynssz.SszListType {
			t.Error("First type hint not set correctly")
		}
		if opts.TypeHints[1].Type != dynssz.SszContainerType {
			t.Error("Second type hint not set correctly")
		}
	})
}

func TestCodeGeneratorTypeOptions(t *testing.T) {
	t.Run("WithReflectType", func(t *testing.T) {
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		typeOpts := []CodeGeneratorOption{
			WithNoHashTreeRoot(),
			WithCreateLegacyFn(),
		}

		opts := CodeGeneratorOptions{}
		option := WithReflectType(reflectType, typeOpts...)
		option(&opts)

		if len(opts.Types) != 1 {
			t.Errorf("Expected 1 type, got %d", len(opts.Types))
		}
		if opts.Types[0].ReflectType != reflectType {
			t.Error("ReflectType not set correctly")
		}
		if len(opts.Types[0].Opts) != 2 {
			t.Errorf("Expected 2 type options, got %d", len(opts.Types[0].Opts))
		}
	})

	t.Run("WithGoTypesType", func(t *testing.T) {
		// Create a mock types.Type for testing
		var goType types.Type = types.Typ[types.Uint64]
		typeOpts := []CodeGeneratorOption{
			WithNoMarshalSSZ(),
		}

		opts := CodeGeneratorOptions{}
		option := WithGoTypesType(goType, typeOpts...)
		option(&opts)

		if len(opts.Types) != 1 {
			t.Errorf("Expected 1 type, got %d", len(opts.Types))
		}
		if opts.Types[0].GoTypesType != goType {
			t.Error("GoTypesType not set correctly")
		}
		if len(opts.Types[0].Opts) != 1 {
			t.Errorf("Expected 1 type option, got %d", len(opts.Types[0].Opts))
		}
	})
}

func TestNewCodeGenerator(t *testing.T) {
	t.Run("WithDynSsz", func(t *testing.T) {
		specs := map[string]any{
			"SLOTS_PER_EPOCH": uint64(32),
			"MAX_VALIDATORS":  uint64(1048576),
		}
		dynSsz := dynssz.NewDynSsz(specs)
		cg := NewCodeGenerator(dynSsz)

		if cg == nil {
			t.Fatal("NewCodeGenerator returned nil")
		}
	})

	t.Run("WithNilDynSsz", func(t *testing.T) {
		cg := NewCodeGenerator(nil)

		if cg == nil {
			t.Fatal("NewCodeGenerator with nil DynSsz returned nil")
		}
	})
}

func TestCodeGeneratorSetPackageName(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.SetPackageName("testpackage")

	// Package name is internal, so we can't directly test it
	// But we can verify it doesn't panic and the generator is still usable
	if cg == nil {
		t.Error("SetPackageName should not break the generator")
	}
}

func TestCodeGeneratorBuildFile(t *testing.T) {
	cg := NewCodeGenerator(nil)

	t.Run("SingleType", func(t *testing.T) {
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		cg.BuildFile("test.go", WithReflectType(reflectType))

		// BuildFile is internal, so we can't directly verify the state
		// But we can verify it doesn't panic
	})

	t.Run("MultipleTypes", func(t *testing.T) {
		reflectType1 := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		reflectType2 := reflect.TypeOf((*SimpleTestStruct2)(nil)).Elem()

		cg.BuildFile("test.go",
			WithReflectType(reflectType1),
			WithReflectType(reflectType2),
		)
	})

	t.Run("WithAllOptions", func(t *testing.T) {
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		sizeHints := []dynssz.SszSizeHint{{Size: 32, Expr: "FIELD_SIZE"}}
		maxSizeHints := []dynssz.SszMaxSizeHint{{Size: 1024, Expr: "MAX_SIZE"}}
		typeHints := []dynssz.SszTypeHint{{Type: dynssz.SszContainerType}}

		cg.BuildFile("test.go",
			WithReflectType(reflectType,
				WithNoHashTreeRoot(),
				WithCreateLegacyFn(),
			),
			WithSizeHints(sizeHints),
			WithMaxSizeHints(maxSizeHints),
			WithTypeHints(typeHints),
			WithoutDynamicExpressions(),
			WithNoFastSsz(),
		)
	})
}

func TestCodeGeneratorAPI(t *testing.T) {
	t.Run("NoTypesError", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		_, err := cg.GenerateToMap()
		if err == nil {
			t.Error("Expected error when generating with no types")
		}
		if !strings.Contains(err.Error(), "no types requested") {
			t.Errorf("Expected 'no types requested' error, got: %v", err)
		}
	})

	t.Run("BasicGeneration", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		cg.BuildFile("test.go", WithReflectType(reflectType))

		results, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if _, exists := results["test.go"]; !exists {
			t.Error("Expected test.go in results")
		}
	})

	t.Run("MultipleFiles", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()

		cg.BuildFile("file1.go", WithReflectType(reflectType))
		cg.BuildFile("file2.go", WithReflectType(reflectType))

		results, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("CustomPackageName", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.SetPackageName("custompackage")
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()

		cg.BuildFile("test.go", WithReflectType(reflectType))

		results, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}

		code := results["test.go"]
		if !strings.Contains(code, "package custompackage") {
			t.Error("Generated code should use custom package name")
		}
	})
}
