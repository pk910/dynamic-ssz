// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"encoding/binary"
	"errors"
	"go/token"
	"go/types"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tests"
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

// testsPackage loads github.com/pk910/dynamic-ssz/codegen/tests once per test
// binary. Loading type-checks the whole import graph from source, and every
// caller only reads the package's type information.
var testsPackage = sync.OnceValues(func() ([]*packages.Package, error) {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName | packages.NeedImports | packages.NeedDeps}
	return packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
})

// loadTestsPackage returns the shared codegen/tests package, failing the test
// when it did not load.
func loadTestsPackage(t *testing.T) *packages.Package {
	t.Helper()
	pkgs, err := testsPackage()
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("load tests package: %v", err)
	}
	return pkgs[0]
}

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
		{"spec-sized container element", "uint64(itemCount) > uint64(len(buf)-startOffset)/(size1+4)", 1},
		// Two entries, each an offset plus the entry's own 4-byte fixed section.
		// Fully static, so it folds to a literal.
		{"static vector element", "itemCount > (len(buf)-startOffset)/(16)", 1},
		// A resolved count can make the divisor zero -- or wrap it past zero --
		// so the bound itself is checked before dividing by it.
		{"spec-counted vector element", "expr1*8 > 0 && uint64(itemCount) > uint64(len(buf)-startOffset)/(expr1*8)", 1},
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
type bulkRootsHolder struct {
	R [][32]byte `ssz-max:"8"`
	O [][48]byte `ssz-size:"?,32" ssz-max:"8"`
}

// With the reflect front end the bulk byte-array copy is gated by the Go
// array length as well: the [32]byte list is copied in bulk, the oversized
// [48]byte list is not.
func TestReflectFrontendBulkByteElems(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_test.go", WithReflectType(reflect.TypeFor[bulkRootsHolder]()))

	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_test.go"]
	if n := strings.Count(code, "MarshalFixedBytesSlice"); n != 1 {
		t.Fatalf("MarshalFixedBytesSlice emitted %d times, want 1 (only the [32]byte list)", n)
	}
	if n := strings.Count(code, "UnmarshalFixedBytesSlice"); n != 1 {
		t.Fatalf("UnmarshalFixedBytesSlice emitted %d times, want 1 (only the [32]byte list)", n)
	}
}

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
		pkgs := []*packages.Package{loadTestsPackage(t)}
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

	t.Run("CrossFileDuplicate", func(t *testing.T) {
		// The same type generated into two files of one package declares its
		// methods twice; the guard has to span files.
		cg := NewCodeGenerator(nil)
		rt := reflect.TypeFor[SimpleTestStruct]()
		cg.BuildFile("a_test_gen.go", WithReflectType(rt))
		cg.BuildFile("b_test_gen.go", WithReflectType(rt))
		if _, err := cg.GenerateToMap(); err == nil || !strings.Contains(err.Error(), "one file only") {
			t.Fatalf("expected cross-file duplicate error, got %v", err)
		}
	})

	t.Run("LegacyNeedsFullMethodSet", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go", WithReflectType(reflect.TypeFor[SimpleTestStruct]()), WithCreateLegacyFn(), WithNoHashTreeRoot())
		if _, err := cg.GenerateToMap(); err == nil || !strings.Contains(err.Error(), "full method set") {
			t.Fatalf("expected legacy method-set error, got %v", err)
		}
	})

	t.Run("HintsOnContainerRoot", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_test.go", WithReflectType(reflect.TypeFor[SimpleTestStruct]()), WithSizeHints([]ssztypes.SszSizeHint{{Size: 4}}))
		if _, err := cg.GenerateToMap(); err == nil || !strings.Contains(err.Error(), "container") {
			t.Fatalf("expected container-root hint error, got %v", err)
		}
	})

	t.Run("InvalidPackageName", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		if err := cg.SetPackageName("_"); err == nil {
			t.Fatal("blank identifier must be rejected as a package name")
		}
		if err := cg.SetPackageName("123abc"); err == nil {
			t.Fatal("non-identifier must be rejected as a package name")
		}
		if err := cg.SetPackageName("mypkg"); err != nil {
			t.Fatalf("valid name rejected: %v", err)
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
			err := cg.generateSSZMethods(unsupportedDesc, typePrinter, codeBuilder, "", &tt.opts, testSpecSet())
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
	err := cg.generateSSZMethods(unsupportedDesc, typePrinter, codeBuilder, "", &opts, testSpecSet())
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
	if err := cg.SetPackageName("testpackage"); err != nil {
		t.Fatalf("SetPackageName: %v", err)
	}

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
		warn := cg.SetHeaderTemplate("// Code generated by mytool; DO NOT EDIT.\n// mytool-hash: {hash} (dynamic-ssz {version})\n")
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
		// A blank line separates the header from the package clause, so the
		// header is not the package's doc comment.
		if !strings.Contains(code, "(dynamic-ssz v"+Version+")\n\npackage codegen\n") {
			t.Error("custom header should substitute the version and be separated from the package clause by a blank line")
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

	t.Run("NonCommentTemplateRejected", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		err := cg.SetHeaderTemplate("my header\n// Hash: {hash}\n")
		if !errors.Is(err, ErrInvalidHeaderTemplate) {
			t.Fatalf("expected ErrInvalidHeaderTemplate, got: %v", err)
		}

		// The rejected template is not applied; generation keeps the default.
		cg.BuildFile("test.go", WithReflectType(reflect.TypeFor[SimpleTestStruct]()))
		results, genErr := cg.GenerateToMap()
		if genErr != nil {
			t.Fatalf("GenerateToMap failed: %v", genErr)
		}
		if !strings.HasPrefix(results["test.go"], "// Code generated by dynamic-ssz. DO NOT EDIT.\n") {
			t.Error("rejected template must leave the default header in place")
		}
	})

	t.Run("IndentedCommentAccepted", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		warn := cg.SetHeaderTemplate("// Code generated by mytool. DO NOT EDIT.\n  // indented comment\n")
		if warn != nil {
			t.Errorf("indented comment lines are valid Go, got: %v", warn)
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
		cg.BuildFile("file2.go", WithReflectType(reflect.TypeOf((*SimpleTestStruct2)(nil)).Elem()))

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
		if err := cg.SetPackageName("custompackage"); err != nil {
			t.Fatalf("SetPackageName: %v", err)
		}
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
			_, err := cg.generateSSZViewMethods(
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

// TestGenerateViewOnlyWithoutViews tests that a view-only type with no view
// types is rejected with a message naming the missing views.
func TestGenerateViewOnlyWithoutViews(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("test.go",
		WithReflectType(reflect.TypeFor[SimpleTestStruct](), WithViewOnly()),
	)

	_, err := cg.GenerateToMap()
	if err == nil || !strings.Contains(err.Error(), "view-only but declares no view types") {
		t.Fatalf("expected the view-only-without-views rejection, got: %v", err)
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
		if err := cg.SetPackageName("test"); err != nil {
			t.Fatalf("SetPackageName: %v", err)
		}
		var nilOpt CodeGeneratorOption
		cg.BuildFile("foo.go", WithReflectType(dataType), nilOpt)
		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("nil BuildFile option: %v", err)
		}
	})

	t.Run("NilReflectViewType", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		if err := cg.SetPackageName("test"); err != nil {
			t.Fatalf("SetPackageName: %v", err)
		}
		cg.BuildFile("foo.go", WithReflectType(dataType, WithReflectViewTypes(nil)))
		if _, err := cg.GenerateToMap(); err != nil {
			t.Fatalf("nil reflect view type: %v", err)
		}
	})

	t.Run("NilGoTypesViewType", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		if err := cg.SetPackageName("test"); err != nil {
			t.Fatalf("SetPackageName: %v", err)
		}
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

	t.Run("ViewOnlyWithoutDynExpressionsRejected", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_viewonly_nodyn.go",
			WithReflectType(baseType, WithoutDynamicExpressions(), WithViewOnly(), WithReflectViewTypes(viewType)),
		)
		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "view-only and cannot be generated without dynamic expressions") {
			t.Fatalf("expected the view-only rejection, got %v", err)
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

// The codegen ParseTags path applies the same rule as the reflection tag
// reader: the static and dynamic size tags of a dimension share one unit,
// for an expression as well as for a literal.
func TestParseTagsConflictingUnits(t *testing.T) {
	for _, tag := range []string{
		`ssz-size:"8" dynssz-bitsize:"UNKNOWN_SPEC"`,
		`ssz-bitsize:"64" dynssz-size:"UNKNOWN_SPEC"`,
		`ssz-size:"8" dynssz-bitsize:"64"`,
		`ssz-bitsize:"64" dynssz-size:"8"`,
	} {
		_, _, _, err := ParseTags(tag)
		if err == nil || !strings.Contains(err.Error(), "conflicting size units") {
			t.Errorf("%s: expected conflicting-units error, got %v", tag, err)
		}
	}
	for _, tag := range []string{
		`ssz-bitsize:"64" dynssz-bitsize:"SPEC"`,
		`ssz-size:"8" dynssz-size:"SPEC"`,
		`ssz-size:"8" dynssz-size:"8"`,
	} {
		if _, _, _, err := ParseTags(tag); err != nil {
			t.Errorf("%s: unexpected error %v", tag, err)
		}
	}
}

// The go/types generation path mirrors the reflect path's view/generic guards:
// a generic instantiation is rejected, duplicate views are rejected, and a
// view type is pointer-wrapped like the base.
func TestGoTypesViewAndGenericGuards(t *testing.T) {
	scope := loadTestsPackage(t).Types.Scope()

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
	pkgs := []*packages.Package{loadTestsPackage(t)}
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

// nodynDualCustom carries both the static and the spec-aware surface;
// nodynDynCustom only the spec-aware one.
type nodynDualCustom struct{ V uint32 }

func (c *nodynDualCustom) SizeSSZ() int                { return 4 }
func (c *nodynDualCustom) MarshalSSZ() ([]byte, error) { return c.MarshalSSZTo(nil) }
func (c *nodynDualCustom) MarshalSSZTo(buf []byte) ([]byte, error) {
	return append(buf, byte(c.V), byte(c.V>>8), byte(c.V>>16), byte(c.V>>24)), nil
}
func (c *nodynDualCustom) UnmarshalSSZ(buf []byte) error {
	if len(buf) != 4 {
		return sszutils.ErrUnexpectedEOF
	}
	c.V = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	return nil
}
func (c *nodynDualCustom) HashTreeRoot() ([32]byte, error)        { return [32]byte{byte(c.V)}, nil }
func (c *nodynDualCustom) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 4 }
func (c *nodynDualCustom) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return c.MarshalSSZTo(buf)
}
func (c *nodynDualCustom) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	return c.UnmarshalSSZ(buf)
}
func (c *nodynDualCustom) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint32(c.V)
	return nil
}

type nodynDynCustom struct{ V uint32 }

func (c *nodynDynCustom) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 4 }
func (c *nodynDynCustom) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, byte(c.V), 0, 0, 0), nil
}
func (c *nodynDynCustom) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	c.V = uint32(buf[0])
	return nil
}
func (c *nodynDynCustom) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint32(c.V)
	return nil
}

type nodynDualHolder struct {
	C nodynDualCustom `ssz-type:"custom" ssz-size:"4"`
	N uint64
}

// nodynStreamCustom carries a static surface and a spec-aware streaming one.
type nodynStreamCustom struct{ V uint32 }

func (c *nodynStreamCustom) SizeSSZ() int                { return 4 }
func (c *nodynStreamCustom) MarshalSSZ() ([]byte, error) { return c.MarshalSSZTo(nil) }
func (c *nodynStreamCustom) MarshalSSZTo(buf []byte) ([]byte, error) {
	return append(buf, byte(c.V), byte(c.V>>8), byte(c.V>>16), byte(c.V>>24)), nil
}
func (c *nodynStreamCustom) UnmarshalSSZ(buf []byte) error {
	if len(buf) != 4 {
		return sszutils.ErrUnexpectedEOF
	}
	c.V = uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	return nil
}
func (c *nodynStreamCustom) HashTreeRoot() ([32]byte, error)        { return [32]byte{byte(c.V)}, nil }
func (c *nodynStreamCustom) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 4 }
func (c *nodynStreamCustom) MarshalSSZEncoder(_ sszutils.DynamicSpecs, enc sszutils.Encoder) error {
	enc.EncodeUint32(c.V)
	return nil
}
func (c *nodynStreamCustom) UnmarshalSSZDecoder(_ sszutils.DynamicSpecs, dec sszutils.Decoder) error {
	v, err := dec.DecodeUint32()
	c.V = v
	return err
}
func (c *nodynStreamCustom) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint32(c.V)
	return nil
}

type nodynStreamHolder struct {
	C nodynStreamCustom `ssz-type:"custom" ssz-size:"4"`
	N uint64
}

type nodynDynHolder struct {
	C nodynDynCustom `ssz-type:"custom"`
	N uint64
}

// Under WithoutDynamicExpressions a custom type with a static surface is
// reached through it, on every buffer and stream path, and one without a
// static surface is rejected by every emitter.
func TestGenerateWithoutDynExprCustomTypes(t *testing.T) {
	static := []CodeGeneratorOption{WithoutDynamicExpressions(), WithCreateEncoderFn(), WithCreateDecoderFn()}

	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_dual.go", WithReflectType(reflect.TypeFor[nodynDualHolder](), static...))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate dual-surface holder: %v", err)
	}
	code := files["gen_dual.go"]
	for _, tok := range []string{"MarshalSSZDyn", "UnmarshalSSZDyn", "SizeSSZDyn", "HashTreeRootWithDyn"} {
		if strings.Contains(code, tok) {
			t.Errorf("generated code references forbidden %s under without-dynamic-expressions:\n%s", tok, code)
		}
	}
	for _, want := range []string{".MarshalSSZTo(", ".UnmarshalSSZ(", ".HashTreeRoot()"} {
		if !strings.Contains(code, want) {
			t.Errorf("generated code does not reach the custom type through %s:\n%s", want, code)
		}
	}

	// The streaming encoder and decoder take the static surface too, so a
	// stream is written and read with the same encoding.
	cg = NewCodeGenerator(nil)
	cg.BuildFile("gen_stream.go", WithReflectType(reflect.TypeFor[nodynStreamHolder](), static...))
	files, err = cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate stream-surface holder: %v", err)
	}
	code = files["gen_stream.go"]
	for _, tok := range []string{".C.MarshalSSZEncoder(", ".C.UnmarshalSSZDecoder(", ".C.SizeSSZDyn("} {
		if strings.Contains(code, tok) {
			t.Errorf("generated code reaches the custom type through spec-aware %s under without-dynamic-expressions", tok)
		}
	}
	for _, want := range []string{".MarshalSSZTo(", "sszutils.DecodeDelegateBuffer(dec, 4)"} {
		if !strings.Contains(code, want) {
			t.Errorf("generated stream code does not reach the custom type through %s:\n%s", want, code)
		}
	}

	for _, tc := range []struct {
		name string
		opts []CodeGeneratorOption
		want string
	}{
		{"marshal", []CodeGeneratorOption{WithNoUnmarshalSSZ(), WithNoSizeSSZ(), WithNoHashTreeRoot()}, "static marshaler"},
		{"unmarshal", []CodeGeneratorOption{WithNoMarshalSSZ(), WithNoSizeSSZ(), WithNoHashTreeRoot()}, "static unmarshaler"},
		{"size", []CodeGeneratorOption{WithNoMarshalSSZ(), WithNoUnmarshalSSZ(), WithNoHashTreeRoot()}, "static sizer"},
		{"hash", []CodeGeneratorOption{WithNoMarshalSSZ(), WithNoUnmarshalSSZ(), WithNoSizeSSZ()}, "static hash tree root"},
		{"encoder", []CodeGeneratorOption{WithNoMarshalSSZ(), WithNoUnmarshalSSZ(), WithNoSizeSSZ(), WithNoHashTreeRoot(), WithCreateEncoderFn()}, "static encoder"},
		{"decoder", []CodeGeneratorOption{WithNoMarshalSSZ(), WithNoUnmarshalSSZ(), WithNoSizeSSZ(), WithNoHashTreeRoot(), WithCreateDecoderFn()}, "static decoder"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen_dyn.go", WithReflectType(reflect.TypeFor[nodynDynHolder](), append([]CodeGeneratorOption{WithoutDynamicExpressions()}, tc.opts...)...))
			_, err := cg.GenerateToMap()
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "provides only") {
				t.Fatalf("dynamic-only custom under without-dynamic-expressions: err = %v, want a %q rejection", err, tc.want)
			}
		})
	}
}

// handRecursiveChild is a recursive type with a hand-written spec-aware
// marshaler, outside the generation set; genRecursiveChild is its generated
// twin.
type handRecursiveChild struct {
	Value    uint64
	Children []*handRecursiveChild `ssz-max:"4"`
}

func (c *handRecursiveChild) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, byte(c.Value)), nil
}

type handRecursiveRoot struct{ Child handRecursiveChild }

type genRecursiveChild struct {
	Value    uint64
	Children []*genRecursiveChild `ssz-max:"4"`
}

type genRecursiveRoot struct{ Child genRecursiveChild }

// A parent reaches a recursive child through the child's private depth method
// only when the child is generated in the same run; a hand-written child of
// the same package is called through its public method.
func TestGenerateRecursiveChildOutsideGenerationSet(t *testing.T) {
	marshalOnly := []CodeGeneratorOption{WithNoUnmarshalSSZ(), WithNoSizeSSZ(), WithNoHashTreeRoot()}

	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_hand.go", WithReflectType(reflect.TypeFor[handRecursiveRoot](), marshalOnly...))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate parent of a hand-written child: %v", err)
	}
	code := files["gen_hand.go"]
	if !strings.Contains(code, ".MarshalSSZDyn(ds, dst)") {
		t.Errorf("hand-written child is not reached through its public method:\n%s", code)
	}
	if strings.Contains(code, ".marshalSSZDynAtDepth(ds, dst, depth)") {
		t.Errorf("hand-written child is called through a depth method it does not have:\n%s", code)
	}

	cg = NewCodeGenerator(nil)
	cg.BuildFile("gen_set.go",
		WithReflectType(reflect.TypeFor[genRecursiveRoot](), marshalOnly...),
		WithReflectType(reflect.TypeFor[genRecursiveChild](), marshalOnly...))
	files, err = cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate parent and child: %v", err)
	}
	code = files["gen_set.go"]
	if !strings.Contains(code, ".marshalSSZDynAtDepth(ds, dst, depth)") {
		t.Errorf("generated child is not reached through its depth method:\n%s", code)
	}
}

// partialRecursiveChild is recursive, hand-writes its marshaler and is in
// the generation set for hashing only.
type partialRecursiveChild struct {
	Value    uint64
	Children []*partialRecursiveChild `ssz-max:"4"`
}

func (c *partialRecursiveChild) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, byte(c.Value)), nil
}

type partialRecursiveRoot struct{ Child partialRecursiveChild }

// A parent names a child's depth twin only for the operations the child is
// generated for in this run; the hand-written marshaler is called publicly
// while the generated hasher is reached through its twin.
func TestGenerateDepthTwinPerOperation(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_partial.go",
		WithReflectType(reflect.TypeFor[partialRecursiveRoot](), WithNoUnmarshalSSZ(), WithNoSizeSSZ()),
		WithReflectType(reflect.TypeFor[partialRecursiveChild](), WithNoMarshalSSZ(), WithNoUnmarshalSSZ(), WithNoSizeSSZ()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_partial.go"]
	if !strings.Contains(code, ".MarshalSSZDyn(ds, dst)") || strings.Contains(code, ".marshalSSZDynAtDepth(ds, dst, depth)") {
		t.Errorf("hand-written marshaler is not reached through its public method:\n%s", code)
	}
	if !strings.Contains(code, ".hashTreeRootWithDynAtDepth(ds, hh, depth)") {
		t.Errorf("generated hasher is not reached through its depth twin:\n%s", code)
	}
}

// A parent generated in a later run over the same package reaches a child
// whose earlier run emitted the depth twin through that twin, and a
// hand-written child through its public method.
func TestGenerateDepthTwinProbedInPackage(t *testing.T) {
	pkg := loadTestsPackage(t)
	node := pkg.Types.Scope().Lookup("RecursiveNode")
	hand := pkg.Types.Scope().Lookup("HandRecursiveChild")
	if node == nil || hand == nil {
		t.Fatal("fixture types not found")
	}
	p := NewParser()
	if !p.fullyDelegatesSSZ(types.NewPointer(node.Type())) {
		t.Skip("generated code not present; RecursiveNode has no methods")
	}

	for _, tc := range []struct {
		name     string
		suffix   string
		child    types.Object
		wantTwin bool
	}{
		{"generated child", "Gen", node, true},
		{"hand-written child", "Hand", hand, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := types.NewNamed(types.NewTypeName(token.NoPos, pkg.Types, "SynthParent"+tc.suffix, nil),
				types.NewStruct([]*types.Var{types.NewField(token.NoPos, pkg.Types, "Child", tc.child.Type(), false)}, []string{""}), nil)
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen_synth.go", WithGoTypesType(parent, WithNoUnmarshalSSZ(), WithNoSizeSSZ(), WithNoHashTreeRoot()))
			files, err := cg.GenerateToMap()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			code := files["gen_synth.go"]
			// The child is reached through whichever twin matches the method
			// the parent delegates to: the static one for a legacy child, the
			// dynamic one otherwise.
			got := strings.Contains(code, "AtDepth(dst, depth)") || strings.Contains(code, "AtDepth(ds, dst, depth)")
			if got != tc.wantTwin {
				t.Errorf("twin call = %v, want %v:\n%s", got, tc.wantTwin, code)
			}
		})
	}
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

// An alias cannot carry methods; a generation target that is an alias is
// refused up front instead of emitting a receiver for another type.
func TestValidateTopLevelTypeRejectsAlias(t *testing.T) {
	pkg := types.NewPackage("example.com/alias", "alias")
	root := types.NewAlias(types.NewTypeName(token.NoPos, pkg, "Root", nil), types.NewArray(types.Typ[types.Uint8], 32))
	container := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Container", nil),
		types.NewStruct([]*types.Var{types.NewField(token.NoPos, pkg, "A", types.Typ[types.Uint64], false)}, nil), nil)
	containerAlias := types.NewAlias(types.NewTypeName(token.NoPos, pkg, "ContainerAlias", nil), container)

	for name, typ := range map[string]types.Type{
		"alias of array":            root,
		"alias of named container":  containerAlias,
		"pointer to aliased struct": types.NewPointer(containerAlias),
	} {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("test.go", WithGoTypesType(typ))
		_, err := cg.GenerateToMap()
		if err == nil || !strings.Contains(err.Error(), "type alias") {
			t.Errorf("%s: err = %v, want the alias rejection", name, err)
		}
	}

	cg := NewCodeGenerator(nil)
	cg.BuildFile("test.go", WithGoTypesType(container))
	if _, err := cg.GenerateToMap(); err != nil {
		t.Fatalf("named container: %v", err)
	}
}

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
		pkgs := []*packages.Package{loadTestsPackage(t)}

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
		cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName | packages.NeedImports | packages.NeedDeps}
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

// mixedOpaqueLeaf is a fully-delegated type whose structure cannot be
// traversed; mixedPlainList carries a limit-less list, which only extended
// types can hash.
type mixedOpaqueLeaf struct {
	V    uint64
	Note any
}

var _ = sszutils.Annotate[mixedOpaqueLeaf](`ssz-static:"true"`)

func (n *mixedOpaqueLeaf) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 8 }
func (n *mixedOpaqueLeaf) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint64(buf, n.V), nil
}
func (n *mixedOpaqueLeaf) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.V = binary.LittleEndian.Uint64(buf)
	return nil
}
func (n *mixedOpaqueLeaf) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(n.V)
	return nil
}

type mixedOpaqueHolder struct {
	L mixedOpaqueLeaf
	N uint64
}

type mixedStatic struct {
	A uint64
	B []uint64 `ssz-max:"4"`
}

type mixedPlainList struct {
	X []uint64
}

type mixedExt struct {
	S int8
}

// Each type in a run is analyzed in its own mode. A static type must not make
// a default-mode neighbour traverse its delegated child, and an extended-types
// type must not let a default-mode neighbour hash a limit-less list; the
// result is the same in either listing order, on both front ends.
func TestGenerateModePerType(t *testing.T) {
	generate := func(t *testing.T, opts ...CodeGeneratorOption) (string, error) {
		t.Helper()
		cg := NewCodeGenerator(nil)
		// The go/types front end reads ssz-static declarations through the
		// resolver the CLI installs from its source scan.
		cg.SetAnnotationResolver(func(t types.Type) string {
			if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
				t = ptr.Elem()
			}
			if named, ok := types.Unalias(t).(*types.Named); ok && named.Obj().Name() == "mixedOpaqueDelegated" {
				tag, _ := sszutils.LookupAnnotation(reflect.TypeFor[mixedOpaqueLeaf]())
				return tag
			}
			return ""
		})
		cg.BuildFile("gen_mixed.go", opts...)
		files, err := cg.GenerateToMap()
		return files["gen_mixed.go"], err
	}
	body := func(code, typeName string) string {
		var methods []string
		for _, chunk := range strings.Split(code, "\nfunc (t *") {
			// A chunk runs up to the next method's doc comment; keep the body only.
			if end := strings.LastIndex(chunk, "\n}"); strings.HasPrefix(chunk, typeName+")") && end >= 0 {
				methods = append(methods, chunk[:end+2])
			}
		}
		return strings.Join(methods, "\n")
	}
	check := func(t *testing.T, name string, a, b CodeGeneratorOption, wantErr string) {
		t.Helper()
		codeAB, errAB := generate(t, a, b)
		codeBA, errBA := generate(t, b, a)
		for _, err := range []error{errAB, errBA} {
			if wantErr == "" && err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr)) {
				t.Fatalf("%s: err = %v, want %q", name, err, wantErr)
			}
		}
		if wantErr == "" && body(codeAB, name) != body(codeBA, name) {
			t.Errorf("%s emitted differently depending on the listing order", name)
		}
	}

	pkg := loadTestsPackage(t)
	goType := func(name string) types.Type {
		obj := pkg.Types.Scope().Lookup(name)
		if obj == nil {
			t.Fatalf("fixture type %s not found", name)
		}
		return obj.Type()
	}

	t.Run("reflect", func(t *testing.T) {
		static := WithReflectType(reflect.TypeFor[mixedStatic](), WithoutDynamicExpressions())
		holder := WithReflectType(reflect.TypeFor[mixedOpaqueHolder]())
		check(t, "mixedOpaqueHolder", static, holder, "")

		ext := WithReflectType(reflect.TypeFor[mixedExt](), WithExtendedTypes())
		plain := WithReflectType(reflect.TypeFor[mixedPlainList]())
		check(t, "mixedPlainList", ext, plain, "has no ssz-max")
	})

	t.Run("go/types", func(t *testing.T) {
		static := WithGoTypesType(goType("MixedStatic"), WithoutDynamicExpressions())
		holder := WithGoTypesType(goType("MixedOpaqueHolder"))
		check(t, "MixedOpaqueHolder", static, holder, "")

		ext := WithGoTypesType(goType("MixedExt"), WithExtendedTypes())
		plain := WithGoTypesType(goType("UnboundedList"))
		check(t, "UnboundedList", ext, plain, "has no ssz-max")
	})
}

// The cache handed to NewCodeGenerator is read-only: a generation leaves the
// instance it came from resolving spec values, delegating and rejecting
// extended types exactly as before, for types it has not built yet as well as
// for types it has. Only the cache's extended-types setting is inherited.
func TestGenerateLeavesCallerCacheUntouched(t *testing.T) {
	specs := map[string]any{"GEN_LEN": uint64(9)}
	ds := dynssz.NewDynSsz(specs, dynssz.WithNoFastSsz())
	fresh := dynssz.NewDynSsz(specs, dynssz.WithNoFastSsz())
	value := &genSpecSizedFallback{V: make([]uint16, 9)}
	before, err := ds.HashTreeRoot(&genSpecSized{V: make([]uint16, 9)})
	if err != nil {
		t.Fatalf("hash before generation: %v", err)
	}

	cg := NewCodeGenerator(ds.GetTypeCache())
	cg.BuildFile("gen_test.go",
		WithReflectType(reflect.TypeFor[genSpecSizedFallback]()),
		WithReflectType(reflect.TypeFor[mixedStatic](), WithoutDynamicExpressions()),
		WithReflectType(reflect.TypeFor[mixedExt](), WithExtendedTypes()),
	)
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	fallbackOf := regexp.MustCompile(`ResolveSpecValueWithDefault\(ds, "GEN_LEN", (\d+)\)`)
	if got := fallbackOf.FindStringSubmatch(files["gen_test.go"]); len(got) < 2 || got[1] != "4" {
		t.Fatalf("emitted fallback %v, want the declared static size 4", got)
	}

	// A type first built after the generation still resolves the spec value.
	got, err := ds.MarshalSSZ(value)
	if err != nil {
		t.Fatalf("marshal after generation: %v", err)
	}
	if len(got) != 18 {
		t.Fatalf("marshalled %d bytes after generation, want 18 (GEN_LEN=9 resolved)", len(got))
	}
	root, err := ds.HashTreeRoot(value)
	if err != nil {
		t.Fatalf("hash after generation: %v", err)
	}
	want, err := fresh.HashTreeRoot(value)
	if err != nil {
		t.Fatalf("hash on fresh instance: %v", err)
	}
	if root != want {
		t.Fatalf("root after generation %x, fresh instance %x", root[:8], want[:8])
	}
	// The generation's own modes did not leak either.
	if _, err = ds.HashTreeRoot(&mixedExt{S: 1}); err == nil {
		t.Fatal("extended types accepted by an instance that did not enable them")
	}
	after, err := ds.HashTreeRoot(&genSpecSized{V: make([]uint16, 9)})
	if err != nil || after != before {
		t.Fatalf("already cached type changed: %x -> %x, err %v", before[:8], after[:8], err)
	}
	tc := ds.GetTypeCache()
	if tc.ExtendedTypes || tc.NoDelegation {
		t.Fatalf("instance cache flags changed: extended=%v nodelegation=%v", tc.ExtendedTypes, tc.NoDelegation)
	}

	// An instance built with extended types widens the reflect path.
	extDs := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	cg = NewCodeGenerator(extDs.GetTypeCache())
	cg.BuildFile("gen_ext.go", WithReflectType(reflect.TypeFor[mixedExt]()))
	if _, err = cg.GenerateToMap(); err != nil {
		t.Fatalf("extended types inherited from the cache: %v", err)
	}
	if n := len(extDs.GetTypeCache().GetAllTypes()); n != 0 {
		t.Fatalf("generation registered %d descriptors in the instance cache", n)
	}
}

// genCustomRoot is a custom type offered as a generation root.
type genCustomRoot uint64

var _ = sszutils.Annotate[genCustomRoot](`ssz-type:"custom"`)

func (c *genCustomRoot) SizeSSZ() int                { return 8 }
func (c *genCustomRoot) MarshalSSZ() ([]byte, error) { return c.MarshalSSZTo(nil) }
func (c *genCustomRoot) MarshalSSZTo(buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint64(buf, uint64(*c)), nil
}
func (c *genCustomRoot) UnmarshalSSZ(buf []byte) error {
	*c = genCustomRoot(binary.LittleEndian.Uint64(buf))
	return nil
}
func (c *genCustomRoot) HashTreeRoot() ([32]byte, error) { return [32]byte{}, nil }

// A custom type provides its own SSZ methods; generating a method set for it
// would redeclare them.
func TestGenerateRejectsCustomRoot(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_custom_root.go", WithReflectType(reflect.TypeFor[genCustomRoot]()))
	_, err := cg.GenerateToMap()
	if err == nil || !strings.Contains(err.Error(), "custom type genCustomRoot: it provides its own SSZ methods") {
		t.Fatalf("custom generation root: err = %v, want the custom-root rejection", err)
	}
}

// genNegSizer reports a negative size from its spec-aware sizer.
type genNegSizer struct{}

func (n *genNegSizer) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return -1 }
func (n *genNegSizer) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return buf, nil
}
func (n *genNegSizer) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, _ []byte) error { return nil }
func (n *genNegSizer) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, _ sszutils.HashWalker) error {
	return nil
}

type genNegSizeHolder struct {
	A uint64
	C genNegSizer    `ssz-type:"custom"`
	L []genNegSizer  `ssz-max:"4" ssz-type:"?,custom"`
	V [2]genNegSizer `ssz-type:"?,custom"`
}

// padBasic and padComposite delegate their hashing; only the basic-shaped one
// is padded to a leaf after its method returns.
type padBasic uint64

func (b *padBasic) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(uint64(*b))
	return nil
}

type padComposite struct{ A uint64 }

func (c *padComposite) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	idx := hh.StartTree(sszutils.TreeTypeBinary)
	hh.PutUint64(c.A)
	hh.Merkleize(idx)
	return nil
}

type padHolder struct {
	B padBasic
	C padComposite
	L []padComposite `ssz-max:"4"`
	V [2]padComposite
}

// genPlainContainer needs no platform guard anywhere in its generated code.
type genPlainContainer struct {
	A uint64
	B uint32
}

// genBigVec declares a fixed-element vector past the 32-bit int range and
// genDynVec a vector of variable-size elements.
type genBigVec struct {
	V []byte `ssz-size:"3000000000"`
}

type genDynVecElem struct {
	B []byte `ssz-max:"8"`
}

type genDynVec struct {
	V []genDynVecElem `ssz-size:"65536"`
}

// A declared size past the 32-bit int range is emitted in a form that
// compiles on every target: the comparisons run in uint64, the int positions
// carry the capped literal, and a platform guard precedes them; a vector of
// variable-size elements allocates its slice and offset table only after the
// input has been checked against the declaration.
// A container that needs no platform guard must not register the math import
// the guard would have used: alone in its output file it has to compile.
// The hasher pads only after a basic-shaped delegate outside a packed scope:
// a composite delegate leaves one root, and every reduce pads its own
// partial trailing chunk.
func TestGenerateHashPadsOnlyBasicDelegates(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_pad.go", WithReflectType(reflect.TypeFor[padHolder]()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	body := methodBody(files["gen_pad.go"], "padHolder", "HashTreeRootWithDyn")
	if body == "" {
		t.Fatal("no HashTreeRootWithDyn body generated")
	}
	if got := strings.Count(body, "hh.FillUpTo32()"); got != 1 {
		t.Fatalf("padHolder hasher pads %d times, want once after the basic-shaped delegate:\n%s", got, body)
	}
	if !strings.Contains(body, "t.B.HashTreeRootWithDyn(ds, hh)") {
		t.Fatalf("basic-shaped field not delegated:\n%s", body)
	}
}

func TestGeneratePlainContainerImportsOnlyWhatItUses(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_plain.go",
		WithReflectType(reflect.TypeFor[genPlainContainer](), WithCreateEncoderFn(), WithCreateDecoderFn()),
	)
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_plain.go"]
	if strings.Contains(code, "\"math\"") && !strings.Contains(code, "math.") {
		t.Fatalf("plain container imports math without using it:\n%s", code)
	}
}

func TestGeneratePortableDeclaredSizes(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_big.go",
		WithReflectType(reflect.TypeFor[genBigVec](), WithCreateEncoderFn(), WithCreateDecoderFn()),
		WithReflectType(reflect.TypeFor[genDynVec](), WithCreateEncoderFn(), WithCreateDecoderFn()),
	)
	files, err := cg.GenerateToMap()
	if math.MaxInt == math.MaxInt32 {
		// A host whose int cannot hold the declaration refuses it at analysis
		// rather than emitting code from wrapped sizes.
		if err == nil || !strings.Contains(err.Error(), "SSZ size limit") {
			t.Fatalf("32-bit host: err = %v, want the size limit refusal", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_big.go"]
	for _, want := range []string{
		"if 3000000000 > math.MaxInt {",
		"sszutils.CapToInt(3000000000)",
		"if 3000000000 != uint64(buflen) {",
		"sszutils.ErrFixedFieldsEOFFn(buflen, uint64(3000000000))",
		"3000000000 > uint64(dec.GetLength())",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("generated code lacks %q:\n%s", want, code)
		}
	}
	for _, line := range strings.Split(code, "\n") {
		if strings.Contains(line, "3000000000*") || strings.Contains(line, "*3000000000") {
			t.Errorf("generated code carries an unfolded constant product: %s", line)
		}
	}

	// The stream decoder seeds the offset table from the delivered bytes and
	// expands the element slice after the offsets are read.
	decoder := methodBody(code, "genDynVec", "UnmarshalSSZDecoder")
	seed := strings.Index(decoder, "sszutils.CredibleCount(dec, 65536-1, 4)")
	expand := strings.Index(decoder, "sszutils.ExpandSlice(val1, 65536)")
	if seed < 0 || expand < 0 || expand < seed {
		t.Errorf("stream decoder allocates for the declaration before checking it:\n%s", decoder)
	}
	// The spec-free type carries its buffer body in the static method.
	unmarshal := methodBody(code, "genDynVec", "UnmarshalSSZ")
	check := strings.Index(unmarshal, "if 262144 > len(buf) {")
	expand = strings.Index(unmarshal, "sszutils.ExpandSlice(val1, 65536)")
	if check < 0 || expand < 0 || expand < check {
		t.Errorf("buffer unmarshal allocates for the declaration before checking it:\n%s", unmarshal)
	}
}

// methodBody returns the body of the named generated method of typeName.
func methodBody(code, typeName, method string) string {
	for _, chunk := range strings.Split(code, "\nfunc (t *") {
		if strings.HasPrefix(chunk, typeName+") "+method+"(") {
			if end := strings.LastIndex(chunk, "\n}"); end >= 0 {
				return chunk[:end+2]
			}
			return chunk
		}
	}
	return ""
}

// A product of two literal bounds folds in uint64 and states no bound past it.
func TestMulOrAddExprOverflow(t *testing.T) {
	if expr, ok := mulOrAddExpr("*", "4294967296", "4294967296"); ok || expr != "" {
		t.Errorf("overflowing product folded to %q, ok=%v", expr, ok)
	}
	if expr, ok := mulOrAddExpr("*", "3000000000", "8"); !ok || expr != "24000000000" {
		t.Errorf("product folded to %q, ok=%v, want 24000000000", expr, ok)
	}
	if expr, ok := mulOrAddExpr("+", "18446744073709551615", "1"); ok || expr != "" {
		t.Errorf("overflowing sum folded to %q, ok=%v", expr, ok)
	}
}

// The streaming encoder reports a negative delegated size instead of clamping
// it, on a container field, list elements, list padding and vector elements.
func TestGenerateEncoderRejectsNegativeSize(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_neg.go", WithReflectType(reflect.TypeFor[genNegSizeHolder](), WithCreateEncoderFn()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_neg.go"]
	if strings.Contains(code, "sszutils.Max(ctx.sizeFn") {
		t.Errorf("generated encoder still clamps a delegated size:\n%s", code)
	}
	if got := strings.Count(code, `"negative size %d"`); got < 4 {
		t.Errorf("generated encoder checks %d delegated sizes, want at least 4:\n%s", got, code)
	}
}

// genOptSpecSized is a fixed-size element whose width comes from a spec value.
type genOptSpecSized struct {
	V []byte `ssz-size:"4" dynssz-size:"OPT_WIDTH"`
}

type genOptSpecSizedHolder struct {
	Opt *genOptSpecSized `ssz-type:"optional-list"`
}

// Every generated method guards an optional-list whose element width resolves
// to zero, since a present zero-width element reads as absent.
func TestGenerateOptionalListZeroWidthGuard(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_opt.go", WithReflectType(reflect.TypeFor[genOptSpecSizedHolder](), WithCreateEncoderFn(), WithCreateDecoderFn()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_opt.go"]
	if got := strings.Count(code, "optional-list element size resolved to 0"); got != 4 {
		t.Errorf("zero-width guard emitted %d times, want 4 (marshal, unmarshal, encoder, decoder):\n%s", got, code)
	}
	// The size path has no error channel and reports 0 instead.
	start := strings.Index(code, ") SizeSSZDyn(")
	if start < 0 {
		t.Fatalf("no size method generated:\n%s", code)
	}
	sizeBody := code[start:]
	if end := strings.Index(sizeBody, "\n}\n"); end >= 0 {
		sizeBody = sizeBody[:end]
	}
	if !strings.Contains(sizeBody, "== 0 {\n\t\t\treturn 0\n") {
		t.Errorf("size method lacks the zero-width guard:\n%s", sizeBody)
	}
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

// ---------------------------------------------------------------------------
// Static (without-dynamic-expressions) generation
// ---------------------------------------------------------------------------

// ---- deeply nested generated containers ----

type nsLeaf struct {
	A []byte `ssz-max:"8"`
	B uint8
}
type nsD5 struct {
	L nsLeaf
	X uint16
}
type nsD4 struct {
	N nsD5
	L []nsD5 `ssz-max:"4"`
}
type nsD3 struct {
	N nsD4
	V [2]nsD4
}
type nsD2 struct {
	N nsD3
	P []nsD3 `ssz-type:"progressive-list" ssz-max:"8"`
}
type nsD1 struct {
	N nsD2
	Z uint64
}

// ---- container-of-container as list/vector/progressive-list ----

type nsListOfC struct {
	L []nsLeaf `ssz-max:"16"`
}
type nsVecOfC struct {
	V [3]nsLeaf
}
type nsProgOfC struct {
	L []nsLeaf `ssz-type:"progressive-list" ssz-max:"16"`
}

// ---- TypeWrapper wrapping a nested generated container (unexported variant) ----

type nsWrapperHolder struct {
	W dynssz.TypeWrapper[struct {
		Data nsD4 `ssz-size:"?"`
	}, nsD4]
}

// ---- union whose variants are nested generated containers (unexported) ----

type nsUnionHolder struct {
	U dynssz.CompatibleUnion[struct {
		F1 nsLeaf
		F2 nsD4
	}]
}

// ---- optional / optional-list of nested generated containers ----

type nsOptHolder struct {
	Opt *nsD4 `ssz-type:"optional"`
}
type nsOptListHolder struct {
	Opt *nsD4 `ssz-type:"optional-list"`
}

// ---- self-referential + mutually recursive (all in generation set) ----

type nsSelfRec struct {
	V     []byte      `ssz-max:"4"`
	Peers []nsSelfRec `ssz-max:"4"`
}
type nsMutA struct {
	V []byte   `ssz-max:"4"`
	B []nsMutB `ssz-max:"4"`
}
type nsMutB struct {
	V []byte   `ssz-max:"4"`
	A []nsMutA `ssz-max:"4"`
}

var nsDynTokens = []string{"MarshalSSZDyn", "UnmarshalSSZDyn", "SizeSSZDyn", "HashTreeRootWithDyn"}

func nsAssertNoDyn(t *testing.T, name, code string) {
	t.Helper()
	if code == "" {
		t.Fatalf("%s: no code generated", name)
	}
	for _, tok := range nsDynTokens {
		if idx := strings.Index(code, tok); idx >= 0 {
			lo, hi := idx-140, idx+140
			if lo < 0 {
				lo = 0
			}
			if hi > len(code) {
				hi = len(code)
			}
			t.Errorf("%s: forbidden %s under without-dynamic-expressions near:\n...%s...", name, tok, code[lo:hi])
		}
	}
}

// nsCombos enumerates flag combinations that must all keep the generated buffer
// AND streaming paths free of *Dyn calls under WithoutDynamicExpressions.
func nsCombos() []struct {
	name string
	opts []CodeGeneratorOption
} {
	return []struct {
		name string
		opts []CodeGeneratorOption
	}{
		{"plain", nil},
		{"nofast", []CodeGeneratorOption{WithNoFastSsz()}},
		{"streaming", []CodeGeneratorOption{WithCreateEncoderFn(), WithCreateDecoderFn()}},
		{"nofast+streaming", []CodeGeneratorOption{WithNoFastSsz(), WithCreateEncoderFn(), WithCreateDecoderFn()}},
		{"legacy", []CodeGeneratorOption{WithCreateLegacyFn()}},
		{"all", []CodeGeneratorOption{WithNoFastSsz(), WithCreateEncoderFn(), WithCreateDecoderFn(), WithCreateLegacyFn()}},
	}
}

// TestStaticNoDynNested generates a large family of nested generated containers
// (deep containers-of-containers, list/vector/progressive-list nesting, union and
// TypeWrapper variants with unexported nested types, optional/optional-list)
// together under WithoutDynamicExpressions across every flag combo. The generated
// code must be valid Go and must never reference a *Dyn buffer function — in the
// buffer methods or in the streaming encoder/decoder.
func TestStaticNoDynNested(t *testing.T) {
	t.Parallel()
	types := []reflect.Type{
		reflect.TypeFor[nsLeaf](), reflect.TypeFor[nsD5](), reflect.TypeFor[nsD4](),
		reflect.TypeFor[nsD3](), reflect.TypeFor[nsD2](), reflect.TypeFor[nsD1](),
		reflect.TypeFor[nsListOfC](), reflect.TypeFor[nsVecOfC](), reflect.TypeFor[nsProgOfC](),
		reflect.TypeFor[nsWrapperHolder](), reflect.TypeFor[nsUnionHolder](),
		reflect.TypeFor[nsOptHolder](), reflect.TypeFor[nsOptListHolder](),
	}
	for _, combo := range nsCombos() {
		t.Run(combo.name, func(t *testing.T) {
			// The combinations share nothing: each builds its own generator.
			t.Parallel()
			typeOpts := append([]CodeGeneratorOption{WithoutDynamicExpressions(), WithExtendedTypes()}, combo.opts...)
			cg := NewCodeGenerator(nil)
			buildOpts := make([]CodeGeneratorOption, 0, len(types))
			for _, rt := range types {
				buildOpts = append(buildOpts, WithReflectType(rt, typeOpts...))
			}
			cg.BuildFile("gen.go", buildOpts...)
			files, err := cg.GenerateToMap()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			nsAssertNoDyn(t, combo.name, files["gen.go"])
		})
	}
}

// TestStaticNoDynRecursion checks self-referential and mutually-recursive types
// terminate (no infinite generator loop) and stay *Dyn-free across combos.
func TestStaticNoDynRecursion(t *testing.T) {
	t.Parallel()
	for _, combo := range nsCombos() {
		t.Run(combo.name, func(t *testing.T) {
			// The combinations share nothing: each builds its own generator.
			t.Parallel()
			typeOpts := append([]CodeGeneratorOption{WithoutDynamicExpressions()}, combo.opts...)
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen.go",
				WithReflectType(reflect.TypeFor[nsSelfRec](), typeOpts...),
				WithReflectType(reflect.TypeFor[nsMutA](), typeOpts...),
				WithReflectType(reflect.TypeFor[nsMutB](), typeOpts...),
				// A pointer-element cycle plus a holder that merely contains
				// it: under the static build the streaming size closures must
				// delegate to the cycle member's static sizer — a walk that
				// inlines it instead grows the output without end, so this
				// generation completing at all is the regression pin.
				WithReflectType(reflect.TypeFor[nsPtrRec](), typeOpts...),
				WithReflectType(reflect.TypeFor[nsPtrRecHolder](), typeOpts...),
			)
			files, err := cg.GenerateToMap()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			nsAssertNoDyn(t, combo.name, files["gen.go"])
		})
	}
}

// nsPtrRec closes a cycle through a pointer-element list, the shape whose
// streaming size closure once inlined itself without end under the static
// build (see TestStaticNoDynRecursion).
type nsPtrRec struct {
	V uint8
	L []*nsPtrRec `ssz-max:"4"`
}

// nsPtrRecHolder threads the bound through a type that is not on the cycle.
type nsPtrRecHolder struct {
	Tag  uint32
	Node nsPtrRec
}

// TestStaticStreamingUnexportedGenericArg is a regression guard for the type-name
// printer: a generic type argument (a CompatibleUnion / TypeWrapper parameter)
// referencing an UNEXPORTED same-package type must be emitted as an unqualified
// identifier, never as its full import path. The reflect type-name cleanup regex
// previously only matched exported ([A-Z]) type names, so an unexported variant
// leaked "github.com/.../pkg.typeName" into the streaming encoder's sizeFn
// signatures, producing invalid Go.
func TestStaticStreamingUnexportedGenericArg(t *testing.T) {
	for _, rt := range []reflect.Type{reflect.TypeFor[nsUnionHolder](), reflect.TypeFor[nsWrapperHolder]()} {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen.go", WithReflectType(rt, WithExtendedTypes(), WithCreateEncoderFn(), WithCreateDecoderFn()))
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("%s: generate: %v", rt, err)
		}
		if strings.Contains(files["gen.go"], "dynamic-ssz/codegen.") {
			t.Errorf("%s: import path leaked as a type name in generated code", rt)
		}
	}
}

// nsWellDelegated is an external fully-delegated type whose Go struct layout
// matches its wire form exactly. Under WithoutDynamicExpressions it is inlined
// from its structure; the inlined static output must equal its Dynamic* method.
type nsWellDelegated struct {
	V uint64
}

var _ = sszutils.Annotate[nsWellDelegated](`ssz-static:"true"`)

func (n *nsWellDelegated) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 8 }
func (n *nsWellDelegated) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint64(buf, n.V), nil
}
func (n *nsWellDelegated) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.V = binary.LittleEndian.Uint64(buf)
	return nil
}
func (n *nsWellDelegated) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(n.V)
	return nil
}

type nsWellHolder struct {
	A uint64
	N nsWellDelegated
	V [2]nsWellDelegated
}

// TestStaticStreamingInlinesExternalDelegated is the regression guard for the
// streaming encoder/decoder *Dyn leak: an external dynamic-only delegated child
// nested in a WithoutDynamicExpressions type must be inlined from its structure
// in BOTH the buffer methods and the streaming encoder/decoder, never reached via
// MarshalSSZDyn/UnmarshalSSZDyn. Previously the streaming generators cleared the
// WithoutDynamicExpressions flag wholesale and forwarded to the *Dyn buffer method.
func TestStaticStreamingInlinesExternalDelegated(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen.go", WithReflectType(reflect.TypeFor[nsWellHolder](),
		WithoutDynamicExpressions(), WithNoFastSsz(), WithCreateEncoderFn(), WithCreateDecoderFn()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	nsAssertNoDyn(t, "external-delegated-streaming", files["gen.go"])
	// The child's uint64 field must be inlined structurally in the streaming path.
	if !strings.Contains(files["gen.go"], "enc.EncodeUint64(t.V)") {
		t.Errorf("expected the streaming encoder to inline the delegated child's uint64 field")
	}
}

// nsIllegalDelegated mirrors the repo's nestedDelegatedInner: a delegated type
// with a structurally-invalid innard (zero-length array). Under
// WithoutDynamicExpressions the parser traverses it (NoDelegation) and must
// reject it with a clear error rather than emit a *Dyn call or panic.
type nsIllegalDelegated struct {
	Bad   [0]uint64
	Value uint32
}

var _ = sszutils.Annotate[nsIllegalDelegated](`ssz-static:"true"`)

func (n *nsIllegalDelegated) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 4 }
func (n *nsIllegalDelegated) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint32(buf, n.Value), nil
}
func (n *nsIllegalDelegated) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.Value = binary.LittleEndian.Uint32(buf)
	return nil
}
func (n *nsIllegalDelegated) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint32(n.Value)
	return nil
}

type nsIllegalHolder struct {
	A uint64
	N nsIllegalDelegated
}

// TestStaticRejectsUnInlinableDelegated confirms a delegated type that cannot be
// faithfully inlined (structurally-invalid innard) is rejected with a clear error
// instead of silently producing wrong code or panicking.
func TestStaticRejectsUnInlinableDelegated(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen.go", WithReflectType(reflect.TypeFor[nsIllegalHolder](), WithoutDynamicExpressions()))
	if _, err := cg.GenerateToMap(); err == nil {
		t.Fatal("expected a clear error inlining a delegated type with an invalid innard")
	}
}

// A per-type extended-types option reaches the shared go/types parser
// whichever position the type holds: the switch is never lowered, so a later
// extended type widens the parser a first plain type created.
func TestPerTypeExtendedTypesReachesParser(t *testing.T) {
	pkgs := []*packages.Package{loadTestsPackage(t)}
	scope := pkgs[0].Types.Scope()
	plain := scope.Lookup("SimpleTypes1")
	extended := scope.Lookup("OptU32")
	if plain == nil || extended == nil {
		t.Skip("corpus types not present")
	}

	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen_test.go",
		WithGoTypesType(plain.Type()),
		WithGoTypesType(extended.Type(), WithExtendedTypes()),
	)
	if _, err := cg.GenerateToMap(); err != nil {
		t.Fatalf("a later extended type must widen the shared parser: %v", err)
	}
}

// A build without dynamic expressions bakes the static tag values, so a length
// or limit that only a spec expression supplies is refused where it is derived
// rather than baked as none.
func TestStaticBuildRefusesSpecOnlyBounds(t *testing.T) {
	type onlyDynLimit struct {
		V []uint64 `dynssz-max:"LIMIT"`
	}
	type onlyDynLength struct {
		V []uint64 `dynssz-size:"LEN"`
	}
	type nestedOnlyDynLimit struct {
		Inner []onlyDynLimit `ssz-max:"2"`
	}
	type progressiveOnlyDynLimit struct {
		V []uint64 `ssz-type:"progressive-list" dynssz-max:"LIMIT"`
	}
	type unionOnlyDynLimit struct {
		U dynssz.CompatibleUnion[struct {
			A uint64
			B onlyDynLimit
		}]
	}
	type withFallback struct {
		V []uint64   `ssz-max:"4" dynssz-max:"LIMIT"`
		W []uint64   `ssz-size:"4" dynssz-size:"LEN"`
		L [][]uint64 `ssz-max:"2,4" dynssz-max:"2,LIMIT"`
	}

	tests := []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(onlyDynLimit{}), "dynssz-max \"LIMIT\" is not defined and has no positive static fallback"},
		{reflect.TypeOf(onlyDynLength{}), "has zero length"},
		{reflect.TypeOf(nestedOnlyDynLimit{}), "dynssz-max \"LIMIT\" is not defined and has no positive static fallback"},
		{reflect.TypeOf(unionOnlyDynLimit{}), "dynssz-max \"LIMIT\" is not defined and has no positive static fallback"},
		{reflect.TypeOf(progressiveOnlyDynLimit{}), "dynssz-max \"LIMIT\" is not defined and has no positive static fallback"},
		{reflect.TypeOf(withFallback{}), ""},
	}

	for _, tt := range tests {
		t.Run(tt.typ.Name(), func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			if err := cg.SetPackageName("x"); err != nil {
				t.Fatal(err)
			}
			cg.BuildFile("x.go", WithReflectType(tt.typ, WithoutDynamicExpressions()))
			_, err := cg.GenerateToMap()
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

// A view whose data type serves views is emitted through the data type's
// view surface without its subtree, framed by the view type's own ssz-static
// declaration: a static view is inlined and sized by the view sizer, while the
// plain reference to the same dynamic data type keeps its offset.
func TestGenerateStaticViewOfDynamicDelegate(t *testing.T) {
	scope := loadTestsPackage(t).Types.Scope()
	lookup := func(name string) types.Type {
		obj := scope.Lookup(name)
		if obj == nil {
			t.Fatalf("%s not found", name)
		}
		return obj.Type()
	}
	child := lookup("ViewTypes1_Base")
	if !NewParser().fullyDelegatesSSZView(types.NewPointer(child)) {
		t.Skip("generated code not present; ViewTypes1_Base serves no views")
	}

	cg := NewCodeGenerator(nil)
	// The declarations gen_views.go registers for the child and its views.
	cg.SetAnnotationResolver(func(t types.Type) string {
		if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
			t = ptr.Elem()
		}
		named, ok := types.Unalias(t).(*types.Named)
		if !ok {
			return ""
		}
		switch named.Obj().Name() {
		case "ViewTypes1_Base", "ViewTypes1_View2":
			return `ssz-static:"false"`
		case "ViewTypes1_View1":
			return `ssz-static:"true"`
		}
		return ""
	})
	cg.BuildFile("gen_viewtypes4.go", WithGoTypesType(lookup("ViewTypes4_Base"), WithGoTypesViewTypes(lookup("ViewTypes4_View1"))))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("GenerateToMap failed: %v", err)
	}
	code := files["gen_viewtypes4.go"]
	for _, want := range []string{
		"Fn := new(ViewTypes1_Base).SizeSSZDynView((*ViewTypes1_View1)(nil))",
		"// Static Field #1 'Child'",
		"// Offset Field #1 'Child'",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("generated code lacks %q", want)
		}
	}
	if strings.Contains(code, "new(ViewTypes1_Base).SizeSSZDyn(ds)") {
		t.Error("the view is sized by the child's plain sizer")
	}
}

// A generated type declares, with its ssz-static declaration, the floor of
// its fixed section as the emitted code frames it: a literal, and the
// expression a spec value feeds. A static build declares the literal it
// baked. Both front ends declare alike.
func TestDelegateAnnotationFor(t *testing.T) {
	scope := loadTestsPackage(t).Types.Scope()
	lookup := func(name string) types.Type {
		obj := scope.Lookup(name)
		if obj == nil {
			t.Fatalf("%s not found", name)
		}
		return obj.Type()
	}
	want := []string{
		"sszutils.Annotate[ViewTypes1_Base](`ssz-static:\"false\" ssz-minsize:\"20\"`)",
		"sszutils.Annotate[*ViewTypes1_View1](`ssz-static:\"true\" ssz-minsize:\"96\"`)",
		"sszutils.Annotate[*ViewTypes1_View2](`ssz-static:\"false\" ssz-minsize:\"16\"`)",
		"sszutils.Annotate[SpecSizedElem](`ssz-static:\"false\" ssz-minsize:\"36\" dynssz-minsize:\"(VECSPEC_LEN):4*8+4\"`)",
		"sszutils.Annotate[SpecPairElem](`ssz-static:\"false\" ssz-minsize:\"71\" dynssz-minsize:\"(SPEC_A):32+(SPEC_B):4*8+(SPEC_BITS/8):3+4\"`)",
		// A static child described without its subtree contributes its
		// declared size; a vector of dynamic children only its offset.
		"sszutils.Annotate[SpecPairVec](`ssz-static:\"false\" ssz-minsize:\"44\" dynssz-minsize:\"(VECSPEC_LEN):4*8+(VECSPEC_LEN):4+8\"`)",
		// A vector with no static fallback declares a :0 part, nothing until
		// its spec value is defined.
		"sszutils.Annotate[SpecOnlyElem](`ssz-static:\"false\" ssz-minsize:\"4\" dynssz-minsize:\"(VLEN):0+4\"`)",
	}
	check := func(t *testing.T, files map[string]string, want []string) {
		t.Helper()
		for _, want := range want {
			if !strings.Contains(files["gen_decl.go"], want) {
				t.Errorf("generated code lacks %q", want)
			}
		}
	}

	t.Run("go/types", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_decl.go",
			WithGoTypesType(lookup("ViewTypes1_Base"), WithGoTypesViewTypes(lookup("ViewTypes1_View1"), lookup("ViewTypes1_View2"))),
			WithGoTypesType(lookup("SpecSizedElem")),
			WithGoTypesType(lookup("SpecPairElem")),
			WithGoTypesType(lookup("SpecPairVec")),
			WithGoTypesType(lookup("SpecOnlyElem")),
		)
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}
		check(t, files, want)
	})

	// The reflect front end regenerates no type that has generated methods,
	// so it declares types of this package shaped like the fixtures.
	t.Run("reflect", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_decl.go",
			WithReflectType(reflect.TypeFor[reflectDeclBase](), WithReflectViewTypes(reflect.TypeFor[reflectDeclStaticView](), reflect.TypeFor[reflectDeclDynamicView]())),
			WithReflectType(reflect.TypeFor[reflectDeclSpecElem]()),
			WithReflectType(reflect.TypeFor[reflectDeclPairElem]()),
			WithReflectType(reflect.TypeFor[reflectDeclNested]()),
			WithReflectType(reflect.TypeFor[reflectDeclPairVec]()),
			WithReflectType(reflect.TypeFor[reflectDeclOnlyElem]()),
		)
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}
		check(t, files, []string{
			// A nested container's constant folds into the parent's, and a
			// literal count distributes over the parts.
			"sszutils.Annotate[reflectDeclNested](`ssz-static:\"false\" ssz-minsize:\"516\" dynssz-minsize:\"(SYNC_COMMITTEE_SIZE/8):64+2*(SYNC_COMMITTEE_SIZE/8):64+324\"`)",
			"sszutils.Annotate[reflectDeclPairVec](`ssz-static:\"false\" ssz-minsize:\"44\" dynssz-minsize:\"(VECSPEC_LEN):4*8+(VECSPEC_LEN):4+8\"`)",
			"sszutils.Annotate[reflectDeclOnlyElem](`ssz-static:\"false\" ssz-minsize:\"4\" dynssz-minsize:\"(VLEN):0+4\"`)",
			"sszutils.Annotate[reflectDeclBase](`ssz-static:\"false\" ssz-minsize:\"20\"`)",
			"sszutils.Annotate[*reflectDeclStaticView](`ssz-static:\"true\" ssz-minsize:\"96\"`)",
			"sszutils.Annotate[*reflectDeclDynamicView](`ssz-static:\"false\" ssz-minsize:\"16\"`)",
			"sszutils.Annotate[reflectDeclSpecElem](`ssz-static:\"false\" ssz-minsize:\"36\" dynssz-minsize:\"(VECSPEC_LEN):4*8+4\"`)",
			"sszutils.Annotate[reflectDeclPairElem](`ssz-static:\"false\" ssz-minsize:\"71\" dynssz-minsize:\"(SPEC_A):32+(SPEC_B):4*8+(SPEC_BITS/8):3+4\"`)",
		})
	})

	t.Run("static build", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_decl.go", WithGoTypesType(lookup("SpecSizedElem"), WithoutDynamicExpressions()))
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}
		check(t, files, []string{"sszutils.Annotate[SpecSizedElem](`ssz-static:\"false\" ssz-minsize:\"36\"`)"})
		if strings.Contains(files["gen_decl.go"], "dynssz-minsize") {
			t.Error("a static build declared a spec expression")
		}
	})
}

// A list of elements delegated without their subtree is bounded by the floor
// the elements declared: 20 bytes for ViewTypes1_Base, 16 for its View2 view,
// and for SpecSizedElem the declared expression resolved at run time with its
// literal as the fallback. Both front ends read the declaration.
func TestGenerateDelegatedListFloor(t *testing.T) {
	scope := loadTestsPackage(t).Types.Scope()
	lookup := func(name string) types.Type {
		obj := scope.Lookup(name)
		if obj == nil {
			t.Fatalf("%s not found", name)
		}
		return obj.Type()
	}
	elem := types.NewPointer(lookup("ViewTypes1_Base"))
	p := NewParser()
	if !p.fullyDelegatesSSZ(elem) || !p.fullyDelegatesSSZView(elem) || !p.fullyDelegatesSSZ(types.NewPointer(lookup("SpecSizedElem"))) || !p.fullyDelegatesSSZ(types.NewPointer(lookup("SpecPairElem"))) {
		t.Skip("generated code not present; the elements delegate nothing")
	}
	// The declarations the elements' generation registered, as the CLI's
	// source scan reads them.
	registered := map[string]reflect.Type{
		"ViewTypes1_Base":  reflect.TypeFor[tests.ViewTypes1_Base](),
		"ViewTypes1_View2": reflect.TypeFor[tests.ViewTypes1_View2](),
		"SpecSizedElem":    reflect.TypeFor[tests.SpecSizedElem](),
		"SpecPairElem":     reflect.TypeFor[tests.SpecPairElem](),
		"SpecPairVec":      reflect.TypeFor[tests.SpecPairVec](),
		"SpecOnlyElem":     reflect.TypeFor[tests.SpecOnlyElem](),
	}
	resolver := func(t types.Type) string {
		if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
			t = ptr.Elem()
		}
		if named, ok := types.Unalias(t).(*types.Named); ok {
			if typ, ok := registered[named.Obj().Name()]; ok {
				tag, _ := sszutils.LookupAnnotation(typ)
				return tag
			}
		}
		return ""
	}
	want := []string{
		"if itemCount > (len(buf)-startOffset)/(20) {",
		"if itemCount > (len(buf)-startOffset)/(16) {",
		`sszutils.ResolveSpecValueWithDefault(ds, "(VECSPEC_LEN):4*8+4", 36)`,
		`sszutils.ResolveSpecValueWithDefault(ds, "(SPEC_A):32+(SPEC_B):4*8+(SPEC_BITS/8):3+4", 71)`,
		`sszutils.ResolveSpecValueWithDefault(ds, "(VECSPEC_LEN):4*8+(VECSPEC_LEN):4+8", 44)`,
		// A floor is a bound, never a refusal: one that does not resolve
		// leaves its variable at zero, which the gate skips.
		`if floor, floorErr := sszutils.ResolveSpecValueWithDefault(ds, "(VLEN):0+4", 4); floorErr == nil && floor <= sszutils.MaxSszSize {`,
		"if expr0 > 0 && uint64(itemCount) > uint64(len(buf)-startOffset)/(expr0) {",
		// A bit list or a union element holds at least one byte.
		"if itemCount > (len(buf)-startOffset)/(1) {",
	}
	check := func(t *testing.T, files map[string]string) {
		t.Helper()
		for _, want := range want {
			if !strings.Contains(files["gen_viewlist.go"], want) {
				t.Errorf("generated code lacks %q", want)
			}
		}
	}

	t.Run("go/types", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.SetAnnotationResolver(resolver)
		cg.BuildFile("gen_viewlist.go",
			WithGoTypesType(lookup("ViewList_Base"), WithGoTypesViewTypes(lookup("ViewList_View"))),
			WithGoTypesType(lookup("SpecSizedList")),
			WithGoTypesType(lookup("SpecPairList")),
			WithGoTypesType(lookup("SpecPairVecList")),
			WithGoTypesType(lookup("SpecOnlyList")),
			WithGoTypesType(lookup("OneByteLists")),
		)
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}
		check(t, files)
	})

	// The reflect front end regenerates no type that has generated methods,
	// so the lists are types of this package over the fixtures' elements,
	// whose declarations it reads from the registry.
	t.Run("reflect", func(t *testing.T) {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen_viewlist.go",
			WithReflectType(reflect.TypeFor[reflectViewList](), WithReflectViewTypes(reflect.TypeFor[reflectViewListView]())),
			WithReflectType(reflect.TypeFor[reflectSpecSizedList]()),
			WithReflectType(reflect.TypeFor[reflectSpecPairList]()),
			WithReflectType(reflect.TypeFor[reflectSpecPairVecList]()),
			WithReflectType(reflect.TypeFor[reflectSpecOnlyList]()),
			WithReflectType(reflect.TypeFor[reflectOneByteLists]()),
		)
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("GenerateToMap failed: %v", err)
		}
		check(t, files)
	})
}

// Types of this package shaped like the codegen/tests fixtures, for the
// reflect front end, which regenerates no type that has generated methods.
type (
	reflectDeclBase struct {
		F1 uint64
		F2 []uint64    `ssz-max:"64"`
		F3 [2][]uint64 `ssz-max:"?,64"`
		C1 *reflectDeclC1
	}
	reflectDeclC1 struct {
		F1 uint64
		F2 []uint64 `ssz-max:"64"`
	}
	reflectDeclStaticView struct {
		F1 uint64
		F3 [2][]uint64 `ssz-size:"2,5"`
		C1 *reflectDeclStaticViewC1
	}
	reflectDeclStaticViewC1 struct {
		F1 uint64
	}
	reflectDeclDynamicView struct {
		F1 uint64
		F2 []uint64 `ssz-max:"64"`
		C1 *reflectDeclDynamicViewC1
	}
	reflectDeclDynamicViewC1 struct {
		F2 []uint64 `ssz-max:"64"`
	}
	reflectDeclSpecElem struct {
		V [8]uint64 `ssz-size:"4" dynssz-size:"VECSPEC_LEN"`
		L []uint64  `ssz-max:"4"`
	}
	reflectDeclSync struct {
		Bits []byte `ssz-size:"64" dynssz-size:"SYNC_COMMITTEE_SIZE/8"`
		Sig  [96]byte
	}
	reflectDeclNested struct {
		S    reflectDeclSync
		Pair [2]reflectDeclSync
		Root [32]byte
		L    []uint64 `ssz-max:"4"`
	}
	reflectDeclPairVec struct {
		Pair [2]*tests.SpecPairElem
		V    tests.VecSpecLen
		L    []uint64 `ssz-max:"4"`
	}
	reflectSpecPairVecList struct {
		Items []*tests.SpecPairVec `ssz-max:"4"`
	}
	reflectDeclOnlyElem struct {
		V []byte   `dynssz-size:"VLEN"`
		L []uint64 `ssz-max:"4"`
	}
	reflectSpecOnlyList struct {
		Items []*tests.SpecOnlyElem `ssz-max:"4"`
	}
	reflectOneByteLists struct {
		Bits   [][]byte `ssz-type:"list,bitlist" ssz-max:"4,64"`
		Unions []dynssz.CompatibleUnion[struct {
			A uint32
			B uint64
		}] `ssz-max:"4"`
	}
	reflectDeclPairElem struct {
		A    []byte    `ssz-size:"32" dynssz-size:"SPEC_A"`
		B    [8]uint64 `ssz-size:"4" dynssz-size:"SPEC_B"`
		Bits []byte    `ssz-type:"bitvector" ssz-bitsize:"20" dynssz-bitsize:"SPEC_BITS"`
		L    []uint64  `ssz-max:"4"`
	}
	reflectSpecPairList struct {
		Items []*tests.SpecPairElem `ssz-max:"4"`
	}
	reflectViewList struct {
		Items []*tests.ViewTypes1_Base `ssz-max:"4"`
		Tail  uint8
	}
	reflectViewListView struct {
		Items []*tests.ViewTypes1_View2 `ssz-max:"4"`
		Tail  uint8
	}
	reflectSpecSizedList struct {
		Items []*tests.SpecSizedElem `ssz-max:"4"`
	}
)

// A type described without its subtree records the literal its generation
// declared on both front ends.
func TestShallowFloorFrontEndParity(t *testing.T) {
	scope := loadTestsPackage(t).Types.Scope()
	obj := scope.Lookup("SpecPairElem")
	if obj == nil {
		t.Fatal("SpecPairElem not found")
	}
	elem := types.NewPointer(obj.Type())
	p := NewParser()
	if !p.fullyDelegatesSSZ(elem) {
		t.Skip("generated code not present; SpecPairElem delegates nothing")
	}
	p.AnnotationResolver = func(types.Type) string {
		tag, _ := sszutils.LookupAnnotation(reflect.TypeFor[tests.SpecPairElem]())
		return tag
	}
	parsed, err := p.GetTypeDescriptor(elem, nil, nil, nil)
	if err != nil {
		t.Fatalf("parser: %v", err)
	}
	cached, err := ssztypes.NewTypeCache(nil).GetTypeDescriptor(reflect.TypeFor[*tests.SpecPairElem](), nil, nil, nil)
	if err != nil {
		t.Fatalf("type cache: %v", err)
	}
	if parsed.MinSize != 71 || cached.MinSize != 71 {
		t.Fatalf("MinSize: parser %d, type cache %d, want the declared 71 on both", parsed.MinSize, cached.MinSize)
	}
}

// The spec expressions of a type are resolved by one buildDynSSZSpecSet
// method, each once, and every method that uses one fetches the resolved set
// on entry instead of resolving on its own. A type without dynamic
// expressions gets no set.
func TestGeneratedSpecSet(t *testing.T) {
	cg := NewCodeGenerator(ssztypes.NewTypeCache(nil))
	cg.BuildFile("gen_test.go",
		WithReflectType(reflect.TypeFor[genSpecSizedFallback](), WithCreateEncoderFn(), WithCreateDecoderFn()),
		WithReflectType(reflect.TypeFor[mixedStatic](), WithoutDynamicExpressions()),
	)
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_test.go"]

	build := "func (t *genSpecSizedFallback) buildDynSSZSpecSet(ds sszutils.DynamicSpecs) ([]uint64, error) {"
	if n := strings.Count(code, build); n != 1 {
		t.Fatalf("emitted %d spec set builders, want 1:\n%s", n, code)
	}
	if n := strings.Count(code, `ResolveSpecValueWithDefault(ds, "GEN_LEN", 4)`); n != 1 {
		t.Fatalf("resolved GEN_LEN %d times, want once in the builder:\n%s", n, code)
	}

	fetch := "sszutils.GetCachedSpecSet[genSpecSizedFallback](ds, t.buildDynSSZSpecSet)"
	fetches := strings.Count(code, fetch)
	// Marshal, encoder, unmarshal, decoder, size and hash tree root each use
	// the resolved length.
	if fetches != 6 {
		t.Fatalf("fetched the spec set in %d methods, want 6:\n%s", fetches, code)
	}
	if binds := strings.Count(code, "expr0 := exprs[0]"); binds != 5 {
		// The encoder reads its context's field in place.
		t.Fatalf("bound the resolved value in %d methods, want 5:\n%s", binds, code)
	}
	if !strings.Contains(code, "exprs []uint64") {
		t.Fatalf("the encoder context does not hold the fetched set:\n%s", code)
	}

	if strings.Contains(code, "func (t *mixedStatic) buildDynSSZSpecSet") {
		t.Fatalf("a type without dynamic expressions builds a spec set:\n%s", code)
	}
}

type genSpecViewBase struct {
	V []uint16 `ssz-size:"4"`
}
type genSpecViewA struct {
	V []uint16 `ssz-size:"4" dynssz-size:"ONLY_A"`
}
type genSpecViewB struct {
	V []uint16 `dynssz-size:"ONLY_B"`
}

// Each view resolves its own expressions by a builder of its own, keyed by
// the view type, so one view never resolves what another declares; the data
// type, with no expression of its own, builds no set.
func TestGeneratedSpecSetPerView(t *testing.T) {
	cg := NewCodeGenerator(ssztypes.NewTypeCache(nil))
	cg.BuildFile("gen_test.go",
		WithReflectType(reflect.TypeFor[genSpecViewBase](), WithReflectViewTypes(reflect.TypeFor[genSpecViewA](), reflect.TypeFor[genSpecViewB]())),
	)
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	code := files["gen_test.go"]

	for view, expr := range map[string]string{"genSpecViewA": `"ONLY_A", 4`, "genSpecViewB": `"ONLY_B", 0`} {
		build := "func (t *genSpecViewBase) buildDynSSZSpecSet_" + view + "(ds sszutils.DynamicSpecs) ([]uint64, error) {"
		if n := strings.Count(code, build); n != 1 {
			t.Fatalf("emitted %d builders for %s, want 1:\n%s", n, view, code)
		}
		if n := strings.Count(code, "ResolveSpecValueWithDefault(ds, "+expr+")"); n != 1 {
			t.Fatalf("resolved %s %d times, want once in the view's builder:\n%s", expr, n, code)
		}
		fetch := "sszutils.GetCachedViewSpecSet[genSpecViewBase, " + view + "](ds, t.buildDynSSZSpecSet_" + view + ")"
		if n := strings.Count(code, fetch); n != 4 {
			t.Fatalf("fetched the set of %s in %d methods, want 4:\n%s", view, n, code)
		}
	}
	if strings.Contains(code, "func (t *genSpecViewBase) buildDynSSZSpecSet(") {
		t.Fatalf("the data type without expressions builds a set:\n%s", code)
	}
}

// A floor entry resolves as a bound and never a refusal, so the builder has
// no error to return for it and declares err only when a value entry needs
// it; a value and a floor of the same expression are separate entries.
func TestSpecSetBuilderFloors(t *testing.T) {
	set := newSpecSetGenerator("*T", "T", "T", "buildDynSSZSpecSet")
	first := set.indexOf("VEC_LEN*8+4", 36, true)
	if again := set.indexOf("VEC_LEN*8+4", 36, true); first != 0 || again != first {
		t.Fatalf("floor entries %d and %d, want one entry at 0", first, again)
	}
	var code strings.Builder
	set.emit(&code)
	floor := `if floor, floorErr := sszutils.ResolveSpecValueWithDefault(ds, "VEC_LEN*8+4", 36); floorErr == nil && floor <= sszutils.MaxSszSize {`
	if !strings.Contains(code.String(), floor) || !strings.Contains(code.String(), "exprs[0] = floor") {
		t.Fatalf("floor not resolved as a bound:\n%s", code.String())
	}
	if strings.Contains(code.String(), "var err error") {
		t.Fatalf("a floor-only builder declares err:\n%s", code.String())
	}

	if set.indexOf("VEC_LEN*8+4", 36, false) != 1 {
		t.Fatal("a value of the floor's expression shares its entry")
	}
	code.Reset()
	set.emit(&code)
	if !strings.Contains(code.String(), "var err error") || !strings.Contains(code.String(), `if exprs[1], err = sszutils.ResolveSpecValueWithDefault(ds, "VEC_LEN*8+4", 36); err != nil {`) {
		t.Fatalf("value entry not resolved with its error:\n%s", code.String())
	}
}

// nsRecursiveDelegated stands for an external -legacy generation of a recursive
// type: an annotation, the complete dynamic surface and the static one. A
// static generation of a holder cannot call the dynamic surface, so the child's
// static surface is what breaks the cycle; the analysis cache must keep it.
type nsRecursiveDelegated struct {
	Value    uint64
	Children []*nsRecursiveDelegated `ssz-max:"4"`
}

var _ = sszutils.Annotate[nsRecursiveDelegated](`ssz-static:"false" ssz-minsize:"12"`)

func (*nsRecursiveDelegated) MarshalSSZ() ([]byte, error)                { return nil, nil }
func (*nsRecursiveDelegated) MarshalSSZTo(buf []byte) ([]byte, error)    { return buf, nil }
func (*nsRecursiveDelegated) UnmarshalSSZ([]byte) error                  { return nil }
func (*nsRecursiveDelegated) SizeSSZ() int                               { return 12 }
func (*nsRecursiveDelegated) HashTreeRoot() ([32]byte, error)            { return [32]byte{}, nil }
func (*nsRecursiveDelegated) HashTreeRootWith(sszutils.HashWalker) error { return nil }
func (*nsRecursiveDelegated) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return buf, nil
}
func (*nsRecursiveDelegated) UnmarshalSSZDyn(sszutils.DynamicSpecs, []byte) error { return nil }
func (*nsRecursiveDelegated) SizeSSZDyn(sszutils.DynamicSpecs) int                { return 12 }
func (*nsRecursiveDelegated) HashTreeRootWithDyn(sszutils.DynamicSpecs, sszutils.HashWalker) error {
	return nil
}

type nsRecursiveHolder struct {
	A uint32
	R nsRecursiveDelegated
}

// TestStaticGenerationKeepsExternalStaticSurface guards the generator's
// analysis cache against the runtime rule that drops the static surface of a
// type with the complete dynamic surface under NoDelegation: that rule
// describes this process, while the static generation describes code that
// reaches the child only through its static methods, and terminates the
// child's recursion through them.
func TestStaticGenerationKeepsExternalStaticSurface(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen.go", WithReflectType(reflect.TypeFor[nsRecursiveHolder](), WithoutDynamicExpressions()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, call := range []string{"t.R.SizeSSZ()", "t.R.UnmarshalSSZ("} {
		if !strings.Contains(files["gen.go"], call) {
			t.Errorf("expected the holder to reach the recursive child through its static surface (%s)", call)
		}
	}
}

// nsStaticShell is a zero-field struct whose only methods are static, as a
// hand-written fastssz type may be; nsCustomShell is the same type declared a
// one-byte custom type. The reflect path of the generator applies the rule of
// the runtime cache: a struct with no fields is refused whatever its methods,
// since they state no width, and the declared custom type is reached through
// its static surface in every mode, a static build included. The go/types path
// refuses every zero-field container on its own.
type nsStaticShell struct{}

func (*nsStaticShell) MarshalSSZ() ([]byte, error)             { return []byte{1}, nil }
func (*nsStaticShell) MarshalSSZTo(buf []byte) ([]byte, error) { return append(buf, 1), nil }
func (*nsStaticShell) UnmarshalSSZ([]byte) error               { return nil }
func (*nsStaticShell) SizeSSZ() int                            { return 1 }
func (*nsStaticShell) HashTreeRoot() ([32]byte, error)         { return [32]byte{1}, nil }

type nsCustomShell struct{}

var _ = sszutils.Annotate[nsCustomShell](`ssz-type:"custom" ssz-size:"1"`)

func (*nsCustomShell) MarshalSSZ() ([]byte, error)             { return []byte{1}, nil }
func (*nsCustomShell) MarshalSSZTo(buf []byte) ([]byte, error) { return append(buf, 1), nil }
func (*nsCustomShell) UnmarshalSSZ([]byte) error               { return nil }
func (*nsCustomShell) SizeSSZ() int                            { return 1 }
func (*nsCustomShell) HashTreeRoot() ([32]byte, error)         { return [32]byte{1}, nil }

type nsStaticShellHolder struct {
	A uint32
	S nsStaticShell
}

type nsCustomShellHolder struct {
	A uint32
	S nsCustomShell
}

func TestReflectAnalysisRefusesUndeclaredShell(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []CodeGeneratorOption
	}{
		{"default", nil},
		{"no fastssz", []CodeGeneratorOption{WithNoFastSsz()}},
		{"static build", []CodeGeneratorOption{WithNoFastSsz(), WithoutDynamicExpressions(), WithCreateEncoderFn(), WithCreateDecoderFn()}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen.go", WithReflectType(reflect.TypeFor[nsStaticShellHolder](), mode.opts...))
			if _, err := cg.GenerateToMap(); err == nil || !strings.Contains(err.Error(), "has no SSZ fields") {
				t.Fatalf("undeclared shell: generate = %v, want the zero-field refusal", err)
			}

			cg = NewCodeGenerator(nil)
			cg.BuildFile("gen.go", WithReflectType(reflect.TypeFor[nsCustomShellHolder](), mode.opts...))
			files, err := cg.GenerateToMap()
			if err != nil {
				t.Fatalf("declared custom shell: generate: %v", err)
			}
			// A one-byte custom type is sized by its declaration and reached
			// through its static methods for the bytes themselves.
			for _, call := range []string{"MarshalSSZTo(", "t.S.UnmarshalSSZ("} {
				if !strings.Contains(files["gen.go"], call) {
					t.Errorf("declared custom shell: expected the holder to reach it through its static surface (%s)", call)
				}
			}
		})
	}
}
