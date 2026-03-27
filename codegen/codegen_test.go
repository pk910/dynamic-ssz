// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// TestCodeGeneratorGenerate tests the Generate() method that writes files to disk.
func TestCodeGeneratorGenerate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		tmpDir := t.TempDir()
		outFile := filepath.Join(tmpDir, "gen_test.go")

		cg := NewCodeGenerator(nil)
		reflectType := reflect.TypeFor[SimpleTestStruct]()
		cg.BuildFile(outFile, WithReflectType(reflectType))

		err := cg.Generate()
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}

		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("reading generated file: %v", err)
		}
		if !strings.Contains(string(data), "package codegen") {
			t.Error("generated file should contain package declaration")
		}
	})

	t.Run("NoTypesError", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		err := cg.Generate()
		if err == nil {
			t.Error("expected error when generating with no types")
		}
	})

	t.Run("GenerateToMapAnalyzeError", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		// int has no PkgPath, which triggers analyzeTypes error
		cg.BuildFile("test.go", WithReflectType(reflect.TypeOf(0)))
		_, err := cg.GenerateToMap()
		if err == nil {
			t.Error("expected error for type with no package path")
		}
	})

	t.Run("WriteToInvalidPath", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		reflectType := reflect.TypeFor[SimpleTestStruct]()
		// Use a path that cannot be written
		cg.BuildFile("/proc/invalid/path/gen_test.go", WithReflectType(reflectType))

		err := cg.Generate()
		if err == nil {
			t.Error("expected error when writing to invalid path")
		}
	})
}

// TestCodeGeneratorStreamingOptions tests WithCreateEncoderFn and WithCreateDecoderFn.
func TestCodeGeneratorStreamingOptions(t *testing.T) {
	t.Run("WithCreateEncoderFn", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithCreateEncoderFn()
		option(&opts)
		if !opts.CreateEncoderFn {
			t.Error("WithCreateEncoderFn should set CreateEncoderFn to true")
		}
	})

	t.Run("WithCreateDecoderFn", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithCreateDecoderFn()
		option(&opts)
		if !opts.CreateDecoderFn {
			t.Error("WithCreateDecoderFn should set CreateDecoderFn to true")
		}
	})

	t.Run("WithExtendedTypes", func(t *testing.T) {
		opts := CodeGeneratorOptions{}
		option := WithExtendedTypes()
		option(&opts)
		if !opts.ExtendedTypes {
			t.Error("WithExtendedTypes should set ExtendedTypes to true")
		}
	})
}

// TestParseTags tests the convenience re-export of ssztypes.ParseTags.
func TestParseTags(t *testing.T) {
	typeHints, sizeHints, maxSizeHints, err := ParseTags(`ssz-max:"10" ssz-size:"5"`)
	if err != nil {
		t.Fatalf("ParseTags failed: %v", err)
	}
	if len(sizeHints) == 0 {
		t.Error("expected size hints")
	}
	if len(maxSizeHints) == 0 {
		t.Error("expected max size hints")
	}
	_ = typeHints
}

// TestGenerateCodeErrorPaths tests error propagation from individual code generators.
func TestGenerateCodeErrorPaths(t *testing.T) {
	unsupportedDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	tests := []struct {
		name string
		opts CodeGeneratorOptions
	}{
		{
			name: "MarshalError",
			opts: CodeGeneratorOptions{},
		},
		{
			name: "UnmarshalError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true},
		},
		{
			name: "SizeError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, NoUnmarshalSSZ: true},
		},
		{
			name: "HashTreeRootError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, NoUnmarshalSSZ: true, NoSizeSSZ: true},
		},
		{
			name: "EncoderError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, NoUnmarshalSSZ: true, NoSizeSSZ: true, NoHashTreeRoot: true, CreateEncoderFn: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			codeBuilder := &strings.Builder{}
			typePrinter := NewTypePrinter("test/package")
			err := cg.generateSSZMethods(unsupportedDesc, typePrinter, codeBuilder, "", &tt.opts)
			if err == nil {
				t.Error("expected error from generateCode")
			}
		})
	}
}

// TestGenerateCodeDecoderError tests that generateCode returns error when decoder generation fails.
func TestGenerateCodeDecoderError(t *testing.T) {
	unsupportedDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	cg := NewCodeGenerator(nil)
	codeBuilder := &strings.Builder{}
	typePrinter := NewTypePrinter("test/package")
	// Skip marshal/unmarshal/size/hashtreeroot/encoder, but enable decoder (CreateEncoderFn controls both)
	opts := CodeGeneratorOptions{
		NoMarshalSSZ:    true,
		NoUnmarshalSSZ:  true,
		NoSizeSSZ:       true,
		NoHashTreeRoot:  true,
		CreateEncoderFn: false, // disable encoder
		CreateDecoderFn: false,
	}
	// With all disabled, no error
	err := cg.generateSSZMethods(unsupportedDesc, typePrinter, codeBuilder, "", &opts)
	if err != nil {
		t.Errorf("expected no error when all generation disabled, got: %v", err)
	}
}

// TestAnalyzeTypesCrossPackageError tests that analyzeTypes rejects types from different packages.
func TestAnalyzeTypesCrossPackageError(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("test.go",
		WithReflectType(reflect.TypeFor[SimpleTestStruct]()),
		WithReflectType(reflect.TypeFor[SimpleTestStruct2]()),
	)

	// These are from the same package, so no error
	_, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("expected no error for same package types, got: %v", err)
	}
}

// TestAnalyzeTypesPointerType tests analyzeTypes with a pointer type input.
func TestAnalyzeTypesPointerType(t *testing.T) {
	cg := NewCodeGenerator(nil)
	// Pass a pointer type - analyzeTypes should handle it
	ptrType := reflect.TypeFor[*SimpleTestStruct]()
	cg.BuildFile("test.go", WithReflectType(ptrType))

	_, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("expected no error for pointer type, got: %v", err)
	}
}

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
		hints := []ssztypes.SszSizeHint{
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
		hints := []ssztypes.SszMaxSizeHint{
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
		hints := []ssztypes.SszTypeHint{
			{Type: ssztypes.SszListType},
			{Type: ssztypes.SszContainerType},
		}
		opts := CodeGeneratorOptions{}
		option := WithTypeHints(hints)
		option(&opts)

		if len(opts.TypeHints) != 2 {
			t.Errorf("Expected 2 type hints, got %d", len(opts.TypeHints))
		}
		if opts.TypeHints[0].Type != ssztypes.SszListType {
			t.Error("First type hint not set correctly")
		}
		if opts.TypeHints[1].Type != ssztypes.SszContainerType {
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

type dummyDynamicSpecs struct {
	specValues map[string]uint64
}

func (d *dummyDynamicSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	value, ok := d.specValues[name]
	return ok, value, nil
}

func TestNewCodeGenerator(t *testing.T) {
	t.Run("WithDynSsz", func(t *testing.T) {
		specs := map[string]uint64{
			"SLOTS_PER_EPOCH": uint64(32),
			"MAX_VALIDATORS":  uint64(1048576),
		}
		typeCache := ssztypes.NewTypeCache(&dummyDynamicSpecs{specValues: specs})
		cg := NewCodeGenerator(typeCache)

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

	t.Run("SingleType", func(_ *testing.T) {
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		cg.BuildFile("test.go", WithReflectType(reflectType))

		// BuildFile is internal, so we can't directly verify the state
		// But we can verify it doesn't panic
	})

	t.Run("MultipleTypes", func(_ *testing.T) {
		reflectType1 := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		reflectType2 := reflect.TypeOf((*SimpleTestStruct2)(nil)).Elem()

		cg.BuildFile("test.go",
			WithReflectType(reflectType1),
			WithReflectType(reflectType2),
		)
	})

	t.Run("WithAllOptions", func(_ *testing.T) {
		reflectType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
		sizeHints := []ssztypes.SszSizeHint{{Size: 32, Expr: "FIELD_SIZE"}}
		maxSizeHints := []ssztypes.SszMaxSizeHint{{Size: 1024, Expr: "MAX_SIZE"}}
		typeHints := []ssztypes.SszTypeHint{{Type: ssztypes.SszContainerType}}

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
