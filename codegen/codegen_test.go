// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"errors"
	"go/types"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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

// limitlessListType carries a list with no ssz-max and a bitlist with no limit
// either, so neither has an SSZ hash tree root.
type limitlessListType struct {
	X []uint64
	B []byte `ssz-type:"bitlist"`
}

// regionBound* cover the shapes a dynamic list's region bound has to state: an
// element whose fixed section is spec-driven, an element that is a vector of
// dynamic entries (statically and spec-driven counted), and an element that is
// itself a list, which has no floor at all.
type regionBoundElem struct {
	Fixed []uint16 `ssz-size:"4" dynssz-size:"BOUND_SIZE"`
	Tail  []uint8  `ssz-max:"8"`
}

type regionBoundDyn struct {
	Tail []uint8 `ssz-max:"8"`
}

type regionBoundWrapped = dynssz.TypeWrapper[struct {
	Data [2][]uint8 `ssz-max:"?,8"`
}, [2][]uint8]

type regionBoundTypes struct {
	SpecSized   []regionBoundElem    `ssz-max:"64"`
	StaticVec   [][2]regionBoundDyn  `ssz-max:"64"`
	SpecVec     [][]regionBoundDyn   `ssz-size:"?,2" dynssz-size:"?,BOUND_COUNT" ssz-max:"64"`
	Wrapped     []regionBoundWrapped `ssz-type:"?,wrapper" ssz-max:"64"`
	ListOfLists [][]uint8            `ssz-max:"64,8"`
}

// The decoder bounds a declared element count by the element's minimum size,
// which must be emitted as an expression wherever a spec value feeds it: the
// generator only sees the static tag values, and a caller running a preset that
// resolves them smaller would have valid input refused.
func TestGenerateListRegionBound(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_test.go", WithReflectType(reflect.TypeFor[regionBoundTypes]()))

	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generation: %v", err)
	}
	code := files["gen_test.go"]

	tests := []struct {
		name  string
		want  string
		count int
	}{
		// A spec-driven fixed section: 4 offset bytes for the dynamic tail plus
		// the resolved size of Fixed. Never zero, so no guard.
		{"spec-sized container element", "itemCount > (len(buf)-startOffset)/(size1+4)", 1},
		// Two entries, each an offset plus the entry's own 4-byte fixed section.
		// Fully static, so it folds to a literal.
		{"static vector element", "itemCount > (len(buf)-startOffset)/(16)", 1},
		// A resolved count can make the divisor zero -- or wrap it past zero --
		// so the bound itself is checked before dividing by it.
		{"spec-counted vector element", "int(expr1)*8 > 0 && itemCount > (len(buf)-startOffset)/(int(expr1)*8)", 1},
		// A wrapper contributes nothing of its own: the bound is the wrapped
		// vector's, two entries of one offset each.
		{"wrapper element", "itemCount > (len(buf)-startOffset)/(8)", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Count(code, tt.want); got != tt.count {
				t.Errorf("emitted %d occurrences of %q, want %d", got, tt.want, tt.count)
			}
		})
	}

	t.Run("list element states no bound", func(t *testing.T) {
		// An empty list costs nothing, so no count is refusable. ListOfLists is
		// the only field whose element is a list, so the other three each get one
		// bound in the buffer unmarshaler.
		if got := strings.Count(code, "ErrListRegionTooSmallFn"); got != 4 {
			t.Errorf("emitted %d region bounds, want one per bounded field", got)
		}
	})
}

// A limit is part of the type in SSZ: List[T, N] and Bitlist[N] need N to
// merkleize, so a list without one has no hash tree root and hashing it is an
// extension. Serialization never needs a limit, so only the hash method is
// refused, and only without extended types.
func TestGenerateLimitlessListRoot(t *testing.T) {
	t.Run("HashRefused", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go", WithReflectType(reflect.TypeFor[limitlessListType]()))

		_, err := cg.GenerateToMap()
		if !errors.Is(err, sszutils.ErrExtendedTypeDisabled) {
			t.Fatalf("err = %v, want ErrExtendedTypeDisabled", err)
		}
	})

	t.Run("SerializationAllowed", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go",
			WithReflectType(reflect.TypeFor[limitlessListType]()),
			WithNoHashTreeRoot())

		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("serialization needs no limit: %v", err)
		}
	})

	// The go/types parser splits lists and bitlists into separate builders, so
	// it can classify one and miss the other. Both must be refused there too --
	// this is the front end dynssz-gen uses, and a type that hashes in one
	// engine but not the other is the divergence this rule exists to prevent.
	t.Run("GoTypesParserRefusesBoth", func(t *testing.T) {
		cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName | packages.NeedImports}
		pkgs, loadErr := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
		if loadErr != nil || len(pkgs) == 0 {
			t.Fatalf("load tests package: %v", loadErr)
		}
		scope := pkgs[0].Types.Scope()

		for _, typeName := range []string{"UnboundedList", "UnboundedBitlist"} {
			t.Run(typeName, func(t *testing.T) {
				obj := scope.Lookup(typeName)
				if obj == nil {
					t.Fatalf("%s not found", typeName)
				}

				cg := NewCodeGenerator(nil)
				cg.BuildFile("gen_test.go", WithGoTypesType(obj.Type()))

				if _, genErr := cg.GenerateToMap(); !errors.Is(genErr, sszutils.ErrExtendedTypeDisabled) {
					t.Fatalf("err = %v, want ErrExtendedTypeDisabled", genErr)
				}
			})
		}
	})

	t.Run("ExtendedTypesWarns", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go",
			WithReflectType(reflect.TypeFor[limitlessListType]()),
			WithExtendedTypes())

		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("extended types should allow the unbounded root: %v", err)
		}

		warnings := cg.Warnings()
		if len(warnings) != 2 {
			t.Fatalf("warnings = %v, want one per limit-less field", warnings)
		}
		for _, warning := range warnings {
			if !strings.Contains(warning, "has no ssz-max") {
				t.Errorf("warning %q does not name the missing limit", warning)
			}
		}
	})
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

// TestGenerateViewSameAsDataType verifies that listing the data type itself as
// a view is rejected. Emitting it would produce a `case *T` alongside the
// dispatcher's own `case nil, *T`, a duplicate type-switch case that does not
// compile — yet generation used to report success.
func TestGenerateViewSameAsDataType(t *testing.T) {
	cg := NewCodeGenerator(nil)
	baseType := reflect.TypeOf((*SimpleTestStruct)(nil)).Elem()

	cg.BuildFile("test.go",
		WithReflectType(baseType, WithReflectViewTypes(baseType)),
	)

	_, err := cg.GenerateToMap()
	if err == nil {
		t.Fatal("expected error when the data type is listed as its own view")
	}
	if !strings.Contains(err.Error(), "same as the data type") {
		t.Fatalf("unexpected error: %v", err)
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
	if !strings.Contains(code, "val := t.V[:vlen:vlen]") {
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

var _ = sszutils.Annotate[genSliceBase](`ssz-max:"64"`)

type genSliceView []uint64

var _ = sszutils.Annotate[genSliceView](`ssz-max:"64"`)

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

	t.Run("DuplicateViewMixedPointerForm", func(t *testing.T) {
		// The same view given once as T and once as *T normalizes to one type,
		// so it must still be rejected as a duplicate (dedup keys on the
		// pointer-normalized type).
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_dupview_mixed_gt.go",
			WithGoTypesType(baseObj.Type(), WithGoTypesViewTypes(viewObj.Type(), types.NewPointer(viewObj.Type()))),
		)
		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "listed more than once") {
			t.Fatalf("expected duplicate-view error for mixed T/*T forms, got %v", err)
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

// The go/types parser must count only declared methods as SSZ delegation. A
// type that satisfies the interfaces solely through methods promoted from an
// embedded field is walked as a container (so its sibling fields survive), not
// treated as fully delegating.
func TestGoTypesPromotedMethodsNotDelegation(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName | packages.NeedImports}
	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("load tests package: %v", err)
	}
	scope := pkgs[0].Types.Scope()
	inner := scope.Lookup("PromotedDelegInner")
	outer := scope.Lookup("PromotedDelegOuter")
	if inner == nil || outer == nil {
		t.Fatal("fixture types not found")
	}

	p := NewParser()
	innerPtr := types.NewPointer(inner.Type())
	outerPtr := types.NewPointer(outer.Type())

	// The declaring type fully delegates; the embedder (promotion only) does not.
	if !p.fullyDelegatesSSZ(innerPtr) {
		t.Error("PromotedDelegInner should fully delegate (it declares the methods)")
	}
	if p.fullyDelegatesSSZ(outerPtr) {
		t.Error("PromotedDelegOuter must NOT delegate through promoted methods")
	}
	if p.getDynamicMarshalerCompatibility(outerPtr) {
		t.Error("promoted MarshalSSZDyn must not count as a declared marshaler")
	}

	// End to end: generating the embedder must emit a full walk that encodes the
	// sibling Label field, not a delegation that drops it.
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_promoted_gt.go", WithGoTypesType(outer.Type()))
	files, genErr := cg.GenerateToMap()
	if genErr != nil {
		t.Fatalf("generate: %v", genErr)
	}
	if !strings.Contains(files["gen_promoted_gt.go"], "Label") {
		t.Error("generated code does not handle the sibling Label field")
	}
}

// nodynChild is a container reached as a nested field/element by the parents
// below. Its dynamic []byte field makes it a variable-size type, so parents
// delegate to it rather than treating it as a fixed blob.
type nodynChild struct {
	A []byte `ssz-max:"8"`
	B uint8
}

type nodynParentProg struct {
	L []nodynChild `ssz-type:"progressive-list" ssz-max:"100"`
}
type nodynParentList struct {
	L []nodynChild `ssz-max:"100"`
}
type nodynParentVec struct {
	V [3]nodynChild
}
type nodynParentField struct {
	C nodynChild
	N uint16
}

// The invariant (maintainer, non-negotiable): with WithoutDynamicExpressions the
// generated code must NEVER reference a *Dyn buffer function. A parent nesting a
// generated child must reach it through the child's static MarshalSSZTo /
// UnmarshalSSZ / SizeSSZ / HashTreeRootWith methods — never MarshalSSZDyn etc.,
// and never by wrapping the streaming Encoder/Decoder into the static buffer
// path. This holds across every combination of -without-fastssz and
// -with-streaming.
func TestGenerateWithoutDynExprNeverEmitsDynBuffer(t *testing.T) {
	dynTokens := []string{"MarshalSSZDyn", "UnmarshalSSZDyn", "SizeSSZDyn", "HashTreeRootWithDyn"}

	combos := []struct {
		name string
		opts []CodeGeneratorOption
	}{
		{"plain", nil},
		{"nofast", []CodeGeneratorOption{WithNoFastSsz()}},
		{"streaming", []CodeGeneratorOption{WithCreateEncoderFn(), WithCreateDecoderFn()}},
		{"nofast+streaming", []CodeGeneratorOption{WithNoFastSsz(), WithCreateEncoderFn(), WithCreateDecoderFn()}},
	}

	for _, combo := range combos {
		t.Run(combo.name, func(t *testing.T) {
			typeOpts := append([]CodeGeneratorOption{WithoutDynamicExpressions()}, combo.opts...)
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen_nodyn.go",
				WithReflectType(reflect.TypeFor[nodynParentProg](), typeOpts...),
				WithReflectType(reflect.TypeFor[nodynParentList](), typeOpts...),
				WithReflectType(reflect.TypeFor[nodynParentVec](), typeOpts...),
				WithReflectType(reflect.TypeFor[nodynParentField](), typeOpts...),
				WithReflectType(reflect.TypeFor[nodynChild](), typeOpts...),
			)
			files, err := cg.GenerateToMap()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			code := files["gen_nodyn.go"]
			if code == "" {
				t.Fatal("no code generated")
			}
			for _, tok := range dynTokens {
				if strings.Contains(code, tok) {
					t.Errorf("generated code references forbidden %s under without-dynamic-expressions:\n%s", tok, code)
				}
			}
			// The parents must actually reach the child via its static methods.
			for _, want := range []string{".MarshalSSZTo(", ".UnmarshalSSZ(", ".SizeSSZ()", ".HashTreeRootWith("} {
				if !strings.Contains(code, want) {
					t.Errorf("expected the parent to call the child's static %s, not found:\n%s", want, code)
				}
			}
		})
	}
}

// nodynExtInlineHolder nests regenDelegated, an EXTERNAL fully-delegated type
// that implements only the Dynamic* methods and carries ssz-static:"true".
// Under WithoutDynamicExpressions its dynamic methods cannot be called, so it is
// inlined from its traversed structure instead of forwarded to *Dyn or errored.
type nodynExtInlineHolder struct {
	A uint64
	N regenDelegated
}

func TestGenerateWithoutDynExprInlinesExternalDynOnly(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_nodyn_inline.go",
		WithReflectType(reflect.TypeFor[nodynExtInlineHolder](), WithoutDynamicExpressions()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_nodyn_inline.go"]
	for _, tok := range []string{"MarshalSSZDyn", "UnmarshalSSZDyn", "SizeSSZDyn", "HashTreeRootWithDyn"} {
		if strings.Contains(code, tok) {
			t.Errorf("external dynamic-only type must be inlined, not forwarded to %s:\n%s", tok, code)
		}
	}
	// The inlined structure must appear (the delegated type's V [8]byte field).
	if !strings.Contains(code, ".V[") {
		t.Errorf("expected the external type's structure to be inlined (field V), got:\n%s", code)
	}
}

// A cyclic type generated with WithoutDynamicExpressions terminates the cycle
// through the member's static MarshalSSZTo (generation-set members carry the
// fastssz-style flag in this mode) even with -without-fastssz, since the static
// path is mandatory there.
func TestGenerateWithoutDynExprRecursiveCycle(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_rec_nodyn.go",
		WithReflectType(reflect.TypeFor[inlineCycleRoot](), WithoutDynamicExpressions(), WithNoFastSsz()),
		WithReflectType(reflect.TypeFor[inlineCycleMember](), WithoutDynamicExpressions(), WithNoFastSsz()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("recursive cycle should generate statically under without-dynamic-expressions: %v", err)
	}
	if strings.Contains(files["gen_rec_nodyn.go"], "Dyn(") {
		t.Errorf("static recursive output must contain no *Dyn call:\n%s", files["gen_rec_nodyn.go"])
	}
}

// topLevelWrapperStruct is a user-declared struct carrying type-wrapper
// semantics through a type-level annotation. A top-level entry has no field
// tag, so the annotation is the only channel available to it.
type topLevelWrapperStruct struct {
	Items []topLevelWrapperItem `ssz-max:"8"`
}

type topLevelWrapperItem struct {
	Val  uint64
	Tail []byte `ssz-max:"4"`
}

var _ = sszutils.Annotate[topLevelWrapperStruct](`ssz-type:"wrapper"`)

// topLevelWrapperAlias names the library's generic TypeWrapper, which is only
// expressible as a transparent alias.
type topLevelWrapperAlias = dynssz.TypeWrapper[struct {
	Data []byte `ssz-size:"32"`
}, []byte]

type topLevelUnionAlias = dynssz.CompatibleUnion[struct {
	A uint32
	B uint64
}]

type topLevelClassicUnionAlias = dynssz.Union[struct {
	A uint32
	B uint64
}]

// TestValidateTopLevelTypeWrapperShapes pins which wrapper-shaped types may be
// listed as standalone -types entries.
//
// The gate keyed on the descriptor's SSZ type, so it rejected anything that
// merely *mapped* to a wrapper or union — including an ordinary named struct
// that can receive methods perfectly well, and which generated fine before the
// gate existed. Only the library's generics genuinely cannot: they are
// nameable solely through a transparent alias, so a method receiver would name
// the foreign generic type.
func TestValidateTopLevelTypeWrapperShapes(t *testing.T) {
	tests := []struct {
		name       string
		reflectTyp reflect.Type
		wantErr    bool
	}{
		{"declared struct with wrapper semantics", reflect.TypeFor[topLevelWrapperStruct](), false},
		{"pointer to declared wrapper struct", reflect.TypeFor[*topLevelWrapperStruct](), false},
		{"generic TypeWrapper via alias", reflect.TypeFor[topLevelWrapperAlias](), true},
		{"generic CompatibleUnion via alias", reflect.TypeFor[topLevelUnionAlias](), true},
		{"generic Union via alias", reflect.TypeFor[topLevelClassicUnionAlias](), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			cg.BuildFile("test.go", WithReflectType(tt.reflectTyp))

			out, err := cg.GenerateToMap()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected the alias-only generic to be rejected")
				}
				if !strings.Contains(err.Error(), "nameable only via a type alias") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("declared named type rejected: %v", err)
			}
			code, ok := out["test.go"]
			if !ok {
				t.Fatal("no output emitted")
			}
			// The methods must land on the declared type, not on a foreign
			// generic receiver.
			if !strings.Contains(code, "func (t *topLevelWrapperStruct) MarshalSSZTo(") {
				t.Fatalf("generated code has no marshal method for the declared type:\n%s", code)
			}
		})
	}

	// The generator's real entry point is go/types, not reflect, so the gate has
	// to reach the same verdict there.
	t.Run("goTypes", func(t *testing.T) {
		cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName | packages.NeedImports}
		pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
		if err != nil || len(pkgs) == 0 {
			t.Fatalf("load tests package: %v", err)
		}

		declared := pkgs[0].Types.Scope().Lookup("TopLevelStructWrapper")
		if declared == nil {
			t.Fatal("TopLevelStructWrapper not found")
		}

		t.Run("declared struct accepted", func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen_wrapper_gt.go", WithGoTypesType(declared.Type()))
			if _, genErr := cg.GenerateToMap(); genErr != nil {
				t.Fatalf("declared named type rejected via go/types: %v", genErr)
			}
		})

		t.Run("pointer to declared struct accepted", func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen_wrapper_ptr_gt.go", WithGoTypesType(types.NewPointer(declared.Type())))
			if _, genErr := cg.GenerateToMap(); genErr != nil {
				t.Fatalf("pointer to declared named type rejected via go/types: %v", genErr)
			}
		})

		// The library generic instantiated the way an alias declares it: the one
		// shape that genuinely cannot receive methods.
		libPkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz")
		if err != nil || len(libPkgs) == 0 {
			t.Fatalf("load library package: %v", err)
		}
		descObj := pkgs[0].Types.Scope().Lookup("TopLevelStructWrapperItem")
		if descObj == nil {
			t.Fatal("TopLevelStructWrapperItem not found")
		}

		for _, generic := range []string{"CompatibleUnion", "Union"} {
			t.Run("generic "+generic+" rejected", func(t *testing.T) {
				obj := libPkgs[0].Types.Scope().Lookup(generic)
				if obj == nil {
					t.Skipf("%s not found in the library package", generic)
				}
				named, ok := obj.Type().(*types.Named)
				if !ok {
					t.Skipf("%s is %T, want *types.Named", generic, obj.Type())
				}
				inst, err := types.Instantiate(nil, named, []types.Type{descObj.Type()}, false)
				if err != nil {
					t.Fatalf("instantiate %s: %v", generic, err)
				}

				cg := NewCodeGenerator(nil)
				cg.BuildFile("gen_generic_gt.go", WithGoTypesType(inst))
				_, genErr := cg.GenerateToMap()
				if genErr == nil {
					t.Fatalf("expected the alias-only generic %s to be rejected", generic)
				}
				if !strings.Contains(genErr.Error(), "nameable only via a type alias") &&
					!strings.Contains(genErr.Error(), "generic type instantiation") {
					t.Fatalf("unexpected error for %s: %v", generic, genErr)
				}
			})
		}
	})
}

// genSpecSized takes its length from a spec value with no static fallback;
// genSpecSizedFallback declares one.
type genSpecSized struct {
	V []uint16 `dynssz-size:"GEN_LEN"`
}
type genSpecSizedFallback struct {
	V []uint16 `ssz-size:"4" dynssz-size:"GEN_LEN"`
}

// genFixedSpecs stands in for a generator handed a type cache that already has
// spec values loaded, e.g. one taken from a running DynSsz.
type genFixedSpecs struct{ values map[string]uint64 }

func (s genFixedSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	value, ok := s.values[name]
	return ok, value, nil
}

// Generated code resolves spec expressions against the specs it runs under, so
// generation must not resolve them itself: the value a generating machine holds
// is not the value the output should carry, and a length it cannot resolve is
// still a length rather than a reason to emit a list.
func TestGenerationDoesNotResolveSpecValues(t *testing.T) {
	fallbackOf := regexp.MustCompile(`ResolveSpecValueWithDefault\(ds, "GEN_LEN", (\d+)\)`)

	for _, tt := range []struct {
		name     string
		typ      reflect.Type
		fallback string
	}{
		{name: "no static fallback", typ: reflect.TypeFor[genSpecSized](), fallback: "0"},
		{name: "static fallback", typ: reflect.TypeFor[genSpecSizedFallback](), fallback: "4"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, specs := range []sszutils.DynamicSpecs{nil, genFixedSpecs{values: map[string]uint64{"GEN_LEN": 9}}} {
				cg := NewCodeGenerator(ssztypes.NewTypeCache(specs))
				cg.BuildFile("gen_test.go", WithReflectType(tt.typ))

				files, err := cg.GenerateToMap()
				if err != nil {
					t.Fatalf("a length supplied by an expression must generate: %v", err)
				}

				got := fallbackOf.FindStringSubmatch(files["gen_test.go"])
				if got == nil {
					t.Fatalf("no spec resolution emitted:\n%s", files["gen_test.go"])
				}
				if got[1] != tt.fallback {
					t.Errorf("emitted fallback %s, want %s (the declared static size)", got[1], tt.fallback)
				}
			}
		})
	}
}
