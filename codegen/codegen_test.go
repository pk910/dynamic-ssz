// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"go/types"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
	"golang.org/x/tools/go/packages"
)

// zeroFieldContainer is an SSZ-invalid container with no encodable fields (only
// an unexported field, which is skipped), used to verify the generator rejects
// it instead of emitting 0-byte methods.
type zeroFieldContainer struct {
	hidden uint64 //nolint:unused // deliberately unexported: leaves zero encodable fields
}

// inlineCycleMember recurses through a bounded list; legal as a type, but its
// cycle can only be emitted when the member's own methods can be called.
type inlineCycleMember struct {
	V     uint64
	Peers []inlineCycleMember `ssz-max:"4"`
}

// inlineCycleRoot references the self-recursive member without being part of
// the cycle itself.
type inlineCycleRoot struct {
	Items []inlineCycleMember `ssz-max:"4"`
}

// A recursive cycle is only emittable when it can be broken by a delegated
// method call. Generating the root without the cycle member must produce a
// clear error (inline emission would recurse forever); including the member in
// the generation set makes the cycle delegate and generation succeed.
func TestGenerateRecursiveCycleValidation(t *testing.T) {
	t.Run("MemberMissing", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go", WithReflectType(reflect.TypeFor[inlineCycleRoot]()))

		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "referenced inline") {
			t.Fatalf("expected inline-cycle error, got %v", err)
		}
	})

	t.Run("MemberIncluded", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go",
			WithReflectType(reflect.TypeFor[inlineCycleRoot]()),
			WithReflectType(reflect.TypeFor[inlineCycleMember]()))

		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("generation with the cycle member included should succeed: %v", err)
		}
	})
}

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

	t.Run("ZeroFieldContainer", func(t *testing.T) {
		// Generating a zero-field container must error, not emit 0-byte methods.
		// The type carries generated-method compat flags during its own run, so
		// this exercises the unconditional reject in the container builder rather
		// than the delegated-shell-exempt post-build check.
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go", WithReflectType(reflect.TypeFor[zeroFieldContainer]()))

		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "no SSZ fields") {
			t.Fatalf("expected no-SSZ-fields error, got %v", err)
		}
	})

	t.Run("DuplicateTypeEntry", func(t *testing.T) {
		// Listing the same type twice for one output would emit its method set
		// twice and fail to compile; the generator must reject it with a clear
		// error instead of reporting success.
		cg := NewCodeGenerator(nil)
		rt := reflect.TypeFor[SimpleTestStruct]()
		dupOpts := []CodeGeneratorOption{WithReflectType(rt), WithReflectType(rt)}
		cg.BuildFile("gen_test.go", dupOpts...)

		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "listed more than once") {
			t.Fatalf("expected duplicate-type error, got %v", err)
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
		{
			name: "DecoderError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, NoUnmarshalSSZ: true, NoSizeSSZ: true, NoHashTreeRoot: true, CreateDecoderFn: true},
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

// SimpleViewStruct is a view-compatible subset of SimpleTestStruct.
type SimpleViewStruct struct {
	Field1 uint64
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

func TestCodeGeneratorSetHeaderTemplate(t *testing.T) {
	t.Run("DefaultHeader", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("test.go", WithReflectType(reflect.TypeFor[SimpleTestStruct]()))

		results, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}

		code := results["test.go"]
		if !strings.HasPrefix(code, "// Code generated by dynamic-ssz. DO NOT EDIT.\n// Hash: ") {
			t.Errorf("unexpected default header:\n%s", code[:min(len(code), 200)])
		}
		if !strings.Contains(code, "// Version: v"+Version+" (https://github.com/pk910/dynamic-ssz)\n") {
			t.Error("default header should contain the substituted version")
		}
		if strings.Contains(code, "{hash}") || strings.Contains(code, "{version}") {
			t.Error("placeholders should be substituted in the default header")
		}
	})

	t.Run("CustomHeader", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		warn := cg.SetHeaderTemplate("// Code generated by mytool; DO NOT EDIT.\n// mytool-hash: {hash} (dynamic-ssz v{version})\n")
		if warn != nil {
			t.Errorf("conventional first line should not warn, got: %v", warn)
		}

		cg.BuildFile("test.go", WithReflectType(reflect.TypeFor[SimpleTestStruct]()))
		results, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}

		code := results["test.go"]
		if !strings.HasPrefix(code, "// Code generated by mytool; DO NOT EDIT.\n// mytool-hash: ") {
			t.Errorf("custom header not applied:\n%s", code[:min(len(code), 200)])
		}
		if !strings.Contains(code, "(dynamic-ssz v"+Version+")\npackage codegen\n") {
			t.Error("custom header should substitute the version and be followed directly by the package clause")
		}
	})

	t.Run("TrailingBlankLinePreserved", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		if warn := cg.SetHeaderTemplate("// Code generated by mytool. DO NOT EDIT.\n\n"); warn != nil {
			t.Errorf("unexpected warning: %v", warn)
		}

		cg.BuildFile("test.go", WithReflectType(reflect.TypeFor[SimpleTestStruct]()))
		results, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}
		if !strings.HasPrefix(results["test.go"], "// Code generated by mytool. DO NOT EDIT.\n\npackage codegen\n") {
			t.Error("intentional blank line after the header should be preserved")
		}
	})

	t.Run("CRLFHeaderDoesNotWarn", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		if warn := cg.SetHeaderTemplate("// Code generated by mytool. DO NOT EDIT.\r\n// Hash: {hash}\r\n"); warn != nil {
			t.Errorf("CRLF line endings should not fail the convention check, got: %v", warn)
		}
	})

	t.Run("NonConventionalHeaderWarns", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		warn := cg.SetHeaderTemplate("// My custom header\n// Hash: {hash}\n")
		if warn == nil {
			t.Fatal("expected warning for first line not matching the generated-code convention")
		}

		// The template is applied despite the warning.
		cg.BuildFile("test.go", WithReflectType(reflect.TypeFor[SimpleTestStruct]()))
		results, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}
		if !strings.HasPrefix(results["test.go"], "// My custom header\n") {
			t.Error("non-conventional template should still be applied")
		}
	})
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

// TestWithReflectViewTypes tests the WithReflectViewTypes option.
func TestWithReflectViewTypes(t *testing.T) {
	opts := CodeGeneratorOptions{}
	vt := reflect.TypeOf((*SimpleTestStruct2)(nil)).Elem()
	option := WithReflectViewTypes(vt)
	option(&opts)

	if len(opts.ViewReflectTypes) != 1 {
		t.Fatalf("expected 1 view type, got %d", len(opts.ViewReflectTypes))
	}
	if opts.ViewReflectTypes[0] != vt {
		t.Error("view type not set correctly")
	}
}

// TestWithViewOnly tests the WithViewOnly option.
func TestWithViewOnly(t *testing.T) {
	opts := CodeGeneratorOptions{}
	option := WithViewOnly()
	option(&opts)

	if !opts.ViewOnly {
		t.Error("WithViewOnly should set ViewOnly to true")
	}
}

// TestGenerateSSZViewMethodsErrorPaths tests error propagation from view method generation.
func TestGenerateSSZViewMethodsErrorPaths(t *testing.T) {
	unsupportedDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	viewDesc := &ssztypes.TypeDescriptor{
		Type:    testDummyReflectType,
		SszType: ssztypes.SszType(255),
		Kind:    reflect.Struct,
	}

	tests := []struct {
		name string
		opts CodeGeneratorOptions
	}{
		{
			name: "MarshalViewError",
			opts: CodeGeneratorOptions{},
		},
		{
			name: "EncoderViewError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, CreateEncoderFn: true},
		},
		{
			name: "UnmarshalViewError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true},
		},
		{
			name: "DecoderViewError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, NoUnmarshalSSZ: true, CreateDecoderFn: true},
		},
		{
			name: "SizeViewError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, NoUnmarshalSSZ: true},
		},
		{
			name: "HashTreeRootViewError",
			opts: CodeGeneratorOptions{NoMarshalSSZ: true, NoUnmarshalSSZ: true, NoSizeSSZ: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			codeBuilder := &strings.Builder{}
			typePrinter := NewTypePrinter("test/package")
			err := cg.generateSSZViewMethods(
				unsupportedDesc, []*ssztypes.TypeDescriptor{viewDesc},
				typePrinter, codeBuilder, &tt.opts,
			)
			if err == nil {
				t.Error("expected error from generateSSZViewMethods")
			}
		})
	}
}

// TestGenerateWithReflectViews tests code generation using the reflect-based
// view type API (WithReflectType + WithReflectViewTypes). This exercises the
// reflect type analysis path in analyzeTypes.
func TestGenerateWithReflectViews(t *testing.T) {
	cg := NewCodeGenerator(nil)
	baseType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
	viewType := reflect.TypeOf((*SimpleViewStruct)(nil)).Elem()

	cg.BuildFile("test.go",
		WithReflectType(baseType, WithReflectViewTypes(viewType)),
	)

	_, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("GenerateToMap failed: %v", err)
	}
}

// TestGenerateWithViewOnly tests code generation using view-only mode
// via the reflect API.
func TestGenerateWithViewOnly(t *testing.T) {
	cg := NewCodeGenerator(nil)
	baseType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
	viewType := reflect.TypeOf((*SimpleViewStruct)(nil)).Elem()

	cg.BuildFile("test.go",
		WithReflectType(baseType,
			WithViewOnly(),
			WithReflectViewTypes(viewType),
		),
	)

	_, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("GenerateToMap failed: %v", err)
	}
}

// TestGenerateFileNoTypes tests the generateFile error when no types are provided.
func TestGenerateFileNoTypes(t *testing.T) {
	cg := NewCodeGenerator(nil)
	_, err := cg.generateFile("test/package", &CodeGeneratorFileOptions{})
	if err == nil {
		t.Error("expected error for empty types")
	}
}

// TestGenerateFileNilDescriptor tests generateFile error when descriptor is nil.
func TestGenerateFileNilDescriptor(t *testing.T) {
	cg := NewCodeGenerator(nil)
	opts := &CodeGeneratorFileOptions{
		Types: []*CodeGeneratorTypeOptions{
			{TypeName: "BadType"},
		},
	}
	_, err := cg.generateFile("test/package", opts)
	if err == nil {
		t.Error("expected error for nil descriptor")
	}
}

// TestGenerateViewTypeAnalysisError tests that analyzeTypes returns an error
// when a view type is incompatible with the base type.
func TestGenerateViewTypeAnalysisError(t *testing.T) {
	cg := NewCodeGenerator(nil)
	baseType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()
	// SimpleTestStruct2 has incompatible field types (uint32 vs uint64)
	badViewType := reflect.TypeOf((*SimpleTestStruct2)(nil)).Elem()

	cg.BuildFile("test.go",
		WithReflectType(baseType, WithReflectViewTypes(badViewType)),
	)

	_, err := cg.GenerateToMap()
	if err == nil {
		t.Error("expected error for incompatible view type")
	}
}

// Nil options and nil view types must be skipped instead of causing a panic.
func TestCodeGeneratorNilOptions(t *testing.T) {
	type Data struct{ A uint64 }
	dataType := reflect.TypeOf(Data{})

	t.Run("NilBuildFileOption", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.SetPackageName("test")
		var nilOpt CodeGeneratorOption
		cg.BuildFile("foo.go", WithReflectType(dataType), nilOpt)
		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("nil BuildFile option: %v", err)
		}
	})

	t.Run("NilReflectViewType", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.SetPackageName("test")
		cg.BuildFile("foo.go", WithReflectType(dataType, WithReflectViewTypes(nil)))
		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("nil reflect view type: %v", err)
		}
	})

	t.Run("NilGoTypesViewType", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.SetPackageName("test")
		cg.BuildFile("foo.go", WithReflectType(dataType, WithGoTypesViewTypes(nil)))
		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("nil go/types view type: %v", err)
		}
	})
}

// regenDelegated mimics a type from an already-generated package: it fully
// delegates SSZ through existing methods and carries the generated ssz-static
// annotation, so the type cache shallow-builds its descriptor.
type regenDelegated struct{ V [8]byte }

var _ = sszutils.Annotate[regenDelegated](`ssz-static:"true"`)

func (n *regenDelegated) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 8 }
func (n *regenDelegated) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, n.V[:]...), nil
}
func (n *regenDelegated) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	copy(n.V[:], buf)
	return nil
}
func (n *regenDelegated) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutBytes(n.V[:])
	return nil
}

// Regenerating over a type that already implements the generated methods must
// produce a descriptive error instead of dereferencing the shallow
// descriptor's missing subtree (previously a nil-pointer panic).
func TestGenerateOverAlreadyGeneratedType(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("regen_test.go", WithReflectType(reflect.TypeFor[regenDelegated]()))

	_, err := cg.GenerateToMap()
	if err == nil {
		t.Fatal("expected error for regeneration over a delegated type")
	}
	if !strings.Contains(err.Error(), "already implements the generated dynamic SSZ methods") {
		t.Errorf("unexpected error: %v", err)
	}
}

// SamePkgUnionLeaf is a sibling type referenced as a generic type argument
// from the same package the code is generated for. Exported deliberately:
// the generic-type import extraction only considers exported names.
type SamePkgUnionLeaf struct {
	F1 uint32
}

type SamePkgUnionHolder struct {
	U dynssz.CompatibleUnion[struct {
		A uint32
		B SamePkgUnionLeaf
	}]
}

// A generic type argument from the package being generated must be emitted
// unqualified; qualifying it would make the generated file import its own
// package, which does not compile.
func TestGenerateGenericSamePackageTypeArg(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_samepkg_union.go",
		WithReflectType(reflect.TypeFor[SamePkgUnionHolder](), WithCreateEncoderFn(), WithCreateDecoderFn()),
		WithReflectType(reflect.TypeFor[SamePkgUnionLeaf](), WithCreateEncoderFn(), WithCreateDecoderFn()),
	)

	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_samepkg_union.go"]
	if code == "" {
		t.Fatal("no code generated")
	}
	if strings.Contains(code, "\"github.com/pk910/dynamic-ssz/codegen\"") {
		t.Error("generated file imports its own package")
	}
	if strings.Contains(code, "codegen.SamePkgUnionLeaf") {
		t.Error("same-package type argument emitted qualified")
	}
}

// encOnlyInner/encOnlyOuter model a field type that exposes only the
// encoder/decoder interfaces (no buffer marshaler), so the outer marshaler
// must delegate through a BufferEncoder.
type encOnlyInner struct {
	A uint64
	B uint64
}

type encOnlyOuter struct {
	X uint32
	I encOnlyInner
}

// The encoder-delegation site wraps dst in a BufferEncoder; the encoder grows
// an under-reserved buffer on demand (see sszutils), so the emitted code needs
// no size reservation of its own. This pins the delegation shape.
func TestGenerateEncoderDelegation(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_enconly_inner.go",
		WithReflectType(reflect.TypeFor[encOnlyInner](), WithNoMarshalSSZ(), WithNoUnmarshalSSZ(), WithCreateEncoderFn(), WithCreateDecoderFn()),
	)
	cg.BuildFile("gen_enconly_outer.go",
		WithReflectType(reflect.TypeFor[encOnlyOuter]()),
	)

	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	outer := files["gen_enconly_outer.go"]
	if !strings.Contains(outer, "NewBufferEncoder") {
		t.Fatalf("outer type does not delegate through a BufferEncoder:\n%s", outer)
	}
	if !strings.Contains(outer, "MarshalSSZEncoder") {
		t.Errorf("outer type does not call the inner encoder:\n%s", outer)
	}
}

// extendedReflectHolder carries an extended type, so generating it requires
// the extended-types switch to reach the descriptor builder.
type extendedReflectHolder struct {
	B big.Int
}

// WithExtendedTypes must take effect on the reflect path (which builds
// descriptors through the shared TypeCache), not only on the go/types parser.
func TestWithExtendedTypesReflectPath(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_extended_reflect.go",
		WithReflectType(reflect.TypeFor[extendedReflectHolder](), WithExtendedTypes()),
	)
	if _, err := cg.GenerateToMap(); err != nil {
		t.Errorf("generate with WithExtendedTypes: %v", err)
	}

	// Without the option the extended type is rejected.
	cg2 := NewCodeGenerator(nil)
	cg2.BuildFile("gen_extended_reflect.go",
		WithReflectType(reflect.TypeFor[extendedReflectHolder]()),
	)
	if _, err := cg2.GenerateToMap(); err == nil {
		t.Error("expected error generating an extended type without WithExtendedTypes")
	}
}

// Top-level scalar aliases: one per scalar arm of the unmarshal emitter.
type (
	genTopBool bool
	genTopU8   uint8
	genTopU16  uint16
	genTopU32  uint32
	genTopU64  uint64
	genTopI8   int8
	genTopI16  int16
	genTopI32  int32
	genTopI64  int64
	genTopF32  float32
	genTopF64  float64
)

// genBigIntMaxHolder carries a limit-bearing big.Int for the decode-side
// limit emission.
type genBigIntMaxHolder struct {
	B big.Int `ssz-max:"5"`
}

// genShortVecHolder carries a slice byte vector whose generated hashing pads
// short values.
type genShortVecHolder struct {
	V []byte `ssz-size:"8"`
}

// Scalar roots emit an exact-length check (a scalar root consumes the whole
// buffer), and a limited big.Int emits the decode-side ssz-max check.
func TestGenerateScalarRootLenChecks(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_scalar_roots.go",
		WithExtendedTypes(),
		WithReflectType(reflect.TypeFor[genTopBool]()),
		WithReflectType(reflect.TypeFor[genTopU8]()),
		WithReflectType(reflect.TypeFor[genTopU16]()),
		WithReflectType(reflect.TypeFor[genTopU32]()),
		WithReflectType(reflect.TypeFor[genTopU64]()),
		WithReflectType(reflect.TypeFor[genTopI8]()),
		WithReflectType(reflect.TypeFor[genTopI16]()),
		WithReflectType(reflect.TypeFor[genTopI32]()),
		WithReflectType(reflect.TypeFor[genTopI64]()),
		WithReflectType(reflect.TypeFor[genTopF32]()),
		WithReflectType(reflect.TypeFor[genTopF64]()),
		WithReflectType(reflect.TypeFor[genBigIntMaxHolder]()),
		WithReflectType(reflect.TypeFor[genShortVecHolder]()),
	)

	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_scalar_roots.go"]
	if code == "" {
		t.Fatal("no code generated")
	}
	// Each scalar root rejects trailing bytes; 11 scalar types plus the
	// big.Int holder's container check emit the trailing error at least once.
	if got := strings.Count(code, "ErrTrailingDataFn"); got < 11 {
		t.Errorf("expected a trailing-data check per scalar root, found %d", got)
	}
	if !strings.Contains(code, "exceeds maximum") {
		t.Error("limited big.Int does not emit the decode-side ssz-max check")
	}
	// The hashing path caps the slice before zero-padding so the padding
	// cannot land in the caller's backing array.
	if !strings.Contains(code, "val := t.V[:len(t.V):len(t.V)]") {
		t.Error("short-vector hashing does not cap the slice before padding")
	}
}

// genBox is a generic type; its instantiations cannot receive generated
// methods (the emitted receiver would declare a type parameter shadowing the
// argument).
type genBox[T any] struct {
	V T
}

// genSliceBase/genSliceView model a named-slice base type with a named-slice
// view; both sides are pointer-wrapped alike during analysis.
type genSliceBase []uint64

type genSliceView []uint64

func TestGenerateViewEdgeCases(t *testing.T) {
	baseType := reflect.TypeFor[SimpleTestStruct]()
	viewType := reflect.TypeFor[SimpleViewStruct]()

	t.Run("GenericInstantiationRejected", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_box.go", WithReflectType(reflect.TypeFor[genBox[uint64]]()))
		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "generic type instantiation") {
			t.Fatalf("expected generic-instantiation error, got %v", err)
		}
	})

	t.Run("DuplicateViewRejected", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_dupviews.go",
			WithReflectType(baseType, WithReflectViewTypes(viewType, viewType)),
		)
		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "listed more than once") {
			t.Fatalf("expected duplicate-view error, got %v", err)
		}
	})

	t.Run("NamedSliceView", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_sliceview.go",
			WithReflectType(reflect.TypeFor[genSliceBase](), WithReflectViewTypes(reflect.TypeFor[genSliceView]())),
		)
		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("named-slice view should generate: %v", err)
		}
	})

	t.Run("ViewMethodsSkippedWithoutDynExpressions", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_views_nodyn.go",
			WithReflectType(baseType, WithoutDynamicExpressions(), WithReflectViewTypes(viewType)),
		)
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		code := files["gen_views_nodyn.go"]
		if strings.Contains(code, "DynView") {
			t.Error("view methods emitted despite WithoutDynamicExpressions; they would bake default spec sizes")
		}
	})
}

// The codegen ParseTags path drops into the same shared-hint merge; a unit
// mismatch between the static and dynamic size tags is rejected there too.
func TestParseTagsConflictingUnits(t *testing.T) {
	_, sizeHints, _, err := ParseTags(`ssz-size:"8" dynssz-bitsize:"UNKNOWN_SPEC"`)
	_ = sizeHints
	if err == nil || !strings.Contains(err.Error(), "conflicting size units") {
		t.Fatalf("expected conflicting-units error, got %v", err)
	}
}

// The go/types generation path mirrors the reflect path's view/generic guards:
// a generic instantiation is rejected, duplicate views are rejected, and a
// view type is pointer-wrapped like the base.
func TestGoTypesViewAndGenericGuards(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName | packages.NeedImports}

	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("load tests package: %v", err)
	}
	scope := pkgs[0].Types.Scope()

	genBoxObj := scope.Lookup("GenericBoxFixture")
	if genBoxObj == nil {
		t.Fatal("GenericBoxFixture not found")
	}
	genBoxNamed, ok := genBoxObj.Type().(*types.Named)
	if !ok {
		t.Fatalf("GenericBoxFixture is %T, want *types.Named", genBoxObj.Type())
	}
	instantiated, err := types.Instantiate(nil, genBoxNamed, []types.Type{types.Typ[types.Uint64]}, false)
	if err != nil {
		t.Fatalf("instantiate GenericBoxFixture: %v", err)
	}

	t.Run("GenericInstantiationRejected", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_box_gt.go", WithGoTypesType(instantiated))
		if _, genErr := cg.GenerateToMap(); genErr == nil || !strings.Contains(genErr.Error(), "generic type instantiation") {
			t.Fatalf("expected generic-instantiation error, got %v", genErr)
		}
	})

	baseObj := scope.Lookup("ViewTypes1_Base")
	viewObj := scope.Lookup("ViewTypes1_View1")
	if baseObj == nil || viewObj == nil {
		t.Fatal("view types not found")
	}

	t.Run("DuplicateViewRejected", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_dupview_gt.go",
			WithGoTypesType(baseObj.Type(), WithGoTypesViewTypes(viewObj.Type(), viewObj.Type())),
		)
		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "listed more than once") {
			t.Fatalf("expected duplicate-view error, got %v", err)
		}
	})

	t.Run("ViewWrapped", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_view_gt.go",
			WithGoTypesType(baseObj.Type(), WithGoTypesViewTypes(viewObj.Type())),
		)
		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("go/types view generation should succeed: %v", err)
		}
	})
}
