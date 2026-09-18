// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"bytes"
	"errors"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/dynssz-gen/testpkg"
	"github.com/pk910/dynamic-ssz/sszutils"
	"golang.org/x/tools/go/packages"
)

// parseTypeSpec mimics the parsing logic from run() for testing
func parseTypeSpec(typeStr string) (typeName, outputFile string, viewTypes []string, isViewOnly bool) {
	parts := strings.Split(typeStr, ":")
	typeName = parts[0]

	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			continue
		}

		switch {
		case strings.HasPrefix(part, "views="):
			viewsStr := strings.TrimPrefix(part, "views=")
			viewTypes = strings.Split(viewsStr, ";")
			for j := range viewTypes {
				viewTypes[j] = strings.TrimSpace(viewTypes[j])
			}
		case strings.HasPrefix(part, "output="):
			outputFile = strings.TrimPrefix(part, "output=")
		case part == "viewonly":
			isViewOnly = true
		default:
			if outputFile == "" {
				outputFile = part
			}
		}
	}
	return
}

// Test helper functions for parsing logic
// -remove moves every distinct configured output file aside, tolerating a
// missing one; the files are restored on failure and deleted on success.
func TestStashOutputs(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "gen_a.go")
	other := filepath.Join(dir, "gen_b.go")
	for _, f := range []string{stale, other} {
		if werr := os.WriteFile(f, []byte("package x\n"), 0o600); werr != nil {
			t.Fatal(werr)
		}
	}
	specs := []typeSpec{{OutputFile: stale}, {OutputFile: stale}, {OutputFile: other}, {OutputFile: filepath.Join(dir, "missing.go")}}
	stash, err := stashOutputs(specs)
	if err != nil {
		t.Fatalf("stashOutputs: %v", err)
	}
	for _, f := range []string{stale, other} {
		if _, serr := os.Stat(f); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("%s still in place (%v)", f, serr)
		}
	}
	stash.restore()
	for _, f := range []string{stale, other} {
		if data, rerr := os.ReadFile(f); rerr != nil || string(data) != "package x\n" {
			t.Fatalf("%s not restored: %v", f, rerr)
		}
	}
	stash, err = stashOutputs(specs)
	if err != nil {
		t.Fatalf("stashOutputs again: %v", err)
	}
	stash.discard()
	for _, f := range []string{stale, other} {
		if _, serr := os.Stat(f); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("%s survived discard (%v)", f, serr)
		}
		if _, serr := os.Stat(f + stashSuffix); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("%s stash survived discard (%v)", f, serr)
		}
	}
}

// -remove through run(): a successful run replaces the stale output and
// leaves no stash behind; a failing run restores the stale output.
func TestRun_RemoveStashesAndRestores(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gen.go")
	stale := []byte("package testpkg // stale\n")
	if err := os.WriteFile(out, stale, 0o600); err != nil {
		t.Fatal(err)
	}

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg",
		TypeNames:   "RepeatedSame",
		OutputFile:  out,
		Remove:      true,
	}
	if err := run(&config); err != nil {
		t.Fatalf("run with -remove: %v", err)
	}
	generated, err := os.ReadFile(out)
	if err != nil || bytes.Equal(generated, stale) || !bytes.Contains(generated, []byte("RepeatedSame")) {
		t.Fatalf("stale output not replaced: %v", err)
	}
	if _, serr := os.Stat(out + stashSuffix); !errors.Is(serr, os.ErrNotExist) {
		t.Fatalf("stash survived a successful run (%v)", serr)
	}

	config.TypeNames = "NonExistentType"
	if rerr := run(&config); rerr == nil || !strings.Contains(rerr.Error(), "not found") {
		t.Fatalf("run err = %v, want the missing type", rerr)
	}
	restored, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(restored, generated) {
		t.Fatalf("output not restored after a failed run: %v", err)
	}
	if _, serr := os.Stat(out + stashSuffix); !errors.Is(serr, os.ErrNotExist) {
		t.Fatalf("stash survived a failed run (%v)", serr)
	}
}

// Two spellings of one output file name one target: the spellings that clean
// to the same path group together, and any other pair is refused rather than
// generated twice and written onto each other.
func TestOutputPathAliases(t *testing.T) {
	specs, err := parseTypeSpecs("A:output=gen.go,B:output=./gen.go", "")
	if err != nil {
		t.Fatalf("parseTypeSpecs: %v", err)
	}
	for _, spec := range specs {
		if spec.OutputFile != "gen.go" {
			t.Fatalf("output %q, want gen.go", spec.OutputFile)
		}
	}
	if err := checkOutputCollisions(specs); err != nil {
		t.Fatalf("equal spellings were refused: %v", err)
	}

	abs, absErr := filepath.Abs("gen.go")
	if absErr != nil {
		t.Fatal(absErr)
	}
	mixed := []typeSpec{{TypeName: "A", OutputFile: "gen.go"}, {TypeName: "B", OutputFile: abs}}
	if err := checkOutputCollisions(mixed); err == nil || !strings.Contains(err.Error(), "name the same file") {
		t.Fatalf("err = %v, want the collision refusal", err)
	}

	// A type with no output file of its own is grouped by the run's default
	// output, so it names nothing here.
	if err := checkOutputCollisions([]typeSpec{{TypeName: "A"}, {TypeName: "B"}}); err != nil {
		t.Fatalf("specs without an output were refused: %v", err)
	}

	// The run refuses a colliding set before it generates anything.
	if err := run(&Config{TypeSpecs: mixed, PackagePath: "."}); err == nil || !strings.Contains(err.Error(), "name the same file") {
		t.Fatalf("run: err = %v, want the collision refusal", err)
	}

	// A linked directory is another spelling of the directory it points at, so
	// the two paths name one file.
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	linked := []typeSpec{
		{TypeName: "A", OutputFile: filepath.Join(target, "gen.go")},
		{TypeName: "B", OutputFile: filepath.Join(link, "gen.go")},
	}
	if err := checkOutputCollisions(linked); err == nil || !strings.Contains(err.Error(), "name the same file") {
		t.Fatalf("err = %v, want the collision refusal", err)
	}
}

// Resolving an output path fails only when the process has no working
// directory to resolve against, which is what a deleted one leaves behind.
func TestOutputPathResolveFailure(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatal(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()
	if err := os.Chdir(gone); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}

	if err := checkOutputCollisions([]typeSpec{{TypeName: "A", OutputFile: "gen.go"}}); err == nil || !strings.Contains(err.Error(), "resolve output") {
		t.Fatalf("err = %v, want the resolve failure", err)
	}
}

// A stash left behind by an interrupted run is never overwritten: the run is
// refused and nothing else is moved.
func TestStashOutputs_RefusesExistingStash(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "gen_a.go")
	second := filepath.Join(dir, "gen_b.go")
	for _, f := range []string{first, second, second + stashSuffix} {
		if err := os.WriteFile(f, []byte("package x // "+filepath.Base(f)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := stashOutputs([]typeSpec{{OutputFile: first}, {OutputFile: second}})
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("err = %v, want the stash refusal", err)
	}
	for _, f := range []string{first, second, second + stashSuffix} {
		if data, rerr := os.ReadFile(f); rerr != nil || string(data) != "package x // "+filepath.Base(f)+"\n" {
			t.Fatalf("%s changed: %v", f, rerr)
		}
	}
	if _, serr := os.Stat(first + stashSuffix); !errors.Is(serr, os.ErrNotExist) {
		t.Fatalf("%s was left moved aside (%v)", first, serr)
	}
}

// A generation that panics gets its output files back before the panic
// continues.
func TestWithStashedOutputs_RestoresOnPanic(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gen.go")
	if err := os.WriteFile(out, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != "generator panic" {
			t.Fatalf("recovered %v, want the generation panic", r)
		}
		if data, err := os.ReadFile(out); err != nil || string(data) != "package x\n" {
			t.Fatalf("output not restored after the panic: %v", err)
		}
		if _, serr := os.Stat(out + stashSuffix); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("stash survived the panic (%v)", serr)
		}
	}()
	_ = withStashedOutputs([]typeSpec{{OutputFile: out}}, func() error {
		if _, serr := os.Stat(out); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("output still in place during generation (%v)", serr)
		}
		panic("generator panic")
	})
}

// A file that cannot be moved aside fails the stash and puts back what was
// already moved.
func TestStashOutputs_MoveFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only directory does not stop root")
	}
	dir := t.TempDir()
	movable := filepath.Join(dir, "gen_a.go")
	if err := os.WriteFile(movable, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	stuck := filepath.Join(locked, "gen_b.go")
	if err := os.WriteFile(stuck, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	_, err := stashOutputs([]typeSpec{{OutputFile: movable}, {OutputFile: stuck}})
	if err == nil || !strings.Contains(err.Error(), "move") {
		t.Fatalf("err = %v, want the move failure", err)
	}
	if _, serr := os.Stat(movable); serr != nil {
		t.Fatalf("the file moved before the failure was not put back: %v", serr)
	}

	// The same failure ends a run before the package is loaded.
	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg",
		TypeNames:   "RepeatedSame",
		OutputFile:  stuck,
		Remove:      true,
	}
	if rerr := run(&config); rerr == nil || !strings.Contains(rerr.Error(), "move") {
		t.Fatalf("run err = %v, want the move failure", rerr)
	}

	// A restore or discard that cannot complete is logged, not fatal: the
	// stash of a file in a directory locked after the move stays in place,
	// and a stash removed by hand is nothing to discard.
	if cerr := os.Chmod(locked, 0o700); cerr != nil {
		t.Fatal(cerr)
	}
	stash, err := stashOutputs([]typeSpec{{OutputFile: stuck}})
	if err != nil {
		t.Fatalf("stashOutputs: %v", err)
	}
	if cerr := os.Chmod(locked, 0o500); cerr != nil {
		t.Fatal(cerr)
	}
	stash.restore()
	if cerr := os.Chmod(locked, 0o700); cerr != nil {
		t.Fatal(cerr)
	}
	if _, serr := os.Stat(stuck + stashSuffix); serr != nil {
		t.Fatalf("stash of a locked directory vanished: %v", serr)
	}
	if rerr := os.Rename(stuck+stashSuffix, stuck); rerr != nil {
		t.Fatal(rerr)
	}
	stash, err = stashOutputs([]typeSpec{{OutputFile: stuck}})
	if err != nil {
		t.Fatalf("stashOutputs: %v", err)
	}
	if rerr := os.Remove(stuck + stashSuffix); rerr != nil {
		t.Fatal(rerr)
	}
	stash.discard()
}

func TestTypeNameParsing(t *testing.T) {
	tests := []struct {
		input            string
		expectedType     string
		expectedFile     string
		expectedViews    []string
		expectedViewOnly bool
	}{
		// Basic cases
		{"TestStruct", "TestStruct", "", nil, false},
		{"TestStruct:output.go", "TestStruct", "output.go", nil, false},
		{"MyType:path/to/file.go", "MyType", "path/to/file.go", nil, false},
		{"SimpleType:", "SimpleType", "", nil, false},

		// With output= prefix
		{"TestType:output=file.go", "TestType", "file.go", nil, false},
		{"TestType:output=path/to/file.go", "TestType", "path/to/file.go", nil, false},

		// With views
		{"TestType:views=View1", "TestType", "", []string{"View1"}, false},
		{"TestType:views=View1;View2", "TestType", "", []string{"View1", "View2"}, false},
		{"TestType:output.go:views=View1", "TestType", "output.go", []string{"View1"}, false},
		{"TestType:views=View1:output.go", "TestType", "output.go", []string{"View1"}, false},
		{"TestType:output=file.go:views=View1;View2", "TestType", "file.go", []string{"View1", "View2"}, false},

		// With viewonly
		{"TestType:viewonly", "TestType", "", nil, true},
		{"TestType:output.go:viewonly", "TestType", "output.go", nil, true},
		{"TestType:viewonly:output.go", "TestType", "output.go", nil, true},
		{"TestType:output=file.go:viewonly", "TestType", "file.go", nil, true},

		// Combined views and viewonly
		{"TestType:views=V1;V2:viewonly", "TestType", "", []string{"V1", "V2"}, true},
		{"TestType:viewonly:views=V1;V2", "TestType", "", []string{"V1", "V2"}, true},
		{"TestType:output.go:views=V1:viewonly", "TestType", "output.go", []string{"V1"}, true},
		{"TestType:views=V1:output.go:viewonly", "TestType", "output.go", []string{"V1"}, true},
		{"TestType:viewonly:output.go:views=V1", "TestType", "output.go", []string{"V1"}, true},

		// Empty parts (consecutive colons) should be skipped
		{"TestType::views=View1", "TestType", "", []string{"View1"}, false},
		{"TestType:::viewonly", "TestType", "", nil, true},
		{"TestType::output.go", "TestType", "output.go", nil, false},
		{"TestType:output.go::views=V1", "TestType", "output.go", []string{"V1"}, false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			typeName, outFile, views, viewOnly := parseTypeSpec(test.input)

			if typeName != test.expectedType {
				t.Errorf("Expected type name %s, got %s", test.expectedType, typeName)
			}
			if outFile != test.expectedFile {
				t.Errorf("Expected output file %q, got %q", test.expectedFile, outFile)
			}
			if len(views) != len(test.expectedViews) {
				t.Errorf("Expected views %v, got %v", test.expectedViews, views)
			} else {
				for i, v := range views {
					if v != test.expectedViews[i] {
						t.Errorf("Expected view[%d] %s, got %s", i, test.expectedViews[i], v)
					}
				}
			}
			if viewOnly != test.expectedViewOnly {
				t.Errorf("Expected viewonly %v, got %v", test.expectedViewOnly, viewOnly)
			}
		})
	}
}

func TestTypeListParsing(t *testing.T) {
	typeNames := "Type1, Type2 ,Type3:file3.go, Type4:file4.go "
	requestedTypes := strings.Split(typeNames, ",")
	for i, typeName := range requestedTypes {
		requestedTypes[i] = strings.TrimSpace(typeName)
	}

	expected := []string{"Type1", "Type2", "Type3:file3.go", "Type4:file4.go"}
	if len(requestedTypes) != len(expected) {
		t.Errorf("Expected %d types, got %d", len(expected), len(requestedTypes))
	}

	for i, expectedType := range expected {
		if requestedTypes[i] != expectedType {
			t.Errorf("Expected type %s at index %d, got %s", expectedType, i, requestedTypes[i])
		}
	}
}

// Test the run function directly for validation errors
func TestRun_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectedErr string
	}{
		{
			name:        "missing package path",
			config:      Config{TypeNames: "TestType"},
			expectedErr: "package path is required (-package)",
		},
		{
			name:        "missing type names",
			config:      Config{PackagePath: "testpkg"},
			expectedErr: "type names are required (-types)",
		},
		{
			name: "missing output file",
			config: Config{
				PackagePath: "fmt",      // Use a valid package to avoid package loading errors
				TypeNames:   "TestType", // This type won't exist, but we'll hit the output file check first
				// OutputFile is intentionally missing
			},
			expectedErr: "output file is required (-output)",
		},
		{
			name: "package load error",
			config: Config{
				PackagePath: "nonexistent",
				TypeNames:   "TestType",
				OutputFile:  "output.go",
			},
			expectedErr: "package nonexistent has errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(&tt.config)
			if err == nil {
				t.Errorf("Expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

// main() tests — call main() directly with reset flag state

func TestMain_VersionFlag(_ *testing.T) {
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}()

	flag.CommandLine = flag.NewFlagSet("dynssz-gen", flag.ContinueOnError)
	os.Args = []string{"dynssz-gen", "-version"}
	main()
}

func TestMain_NoArgs(_ *testing.T) {
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}()

	flag.CommandLine = flag.NewFlagSet("dynssz-gen", flag.ContinueOnError)
	os.Args = []string{"dynssz-gen"}
	main()
}

func TestMain_RunError(t *testing.T) {
	if os.Getenv("TEST_DYNSSZ_MAIN_RUN_ERROR") == "1" {
		flag.CommandLine = flag.NewFlagSet("dynssz-gen", flag.ContinueOnError)
		os.Args = []string{"dynssz-gen", "-package", "nonexistent/bad/pkg", "-types", "Foo", "-output", "out.go"}
		main() // calls log.Fatal → os.Exit(1)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_RunError$") //nolint:gosec // G204: test helper with controlled input
	cmd.Env = append(os.Environ(), "TEST_DYNSSZ_MAIN_RUN_ERROR=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code from main() with bad package")
	}
}

// getVersionString tests

func TestGetVersionString_NoBuildMetadata(t *testing.T) {
	origCommit := codegen.BuildCommit
	origTime := codegen.BuildTime
	defer func() {
		codegen.BuildCommit = origCommit
		codegen.BuildTime = origTime
	}()

	codegen.BuildCommit = ""
	codegen.BuildTime = ""
	v := getVersionString()
	if !strings.HasPrefix(v, "v") {
		t.Fatalf("expected version to start with 'v', got: %s", v)
	}
	if strings.Contains(v, "commit") {
		t.Fatalf("expected no commit info, got: %s", v)
	}
}

func TestGetVersionString_CommitOnly(t *testing.T) {
	origCommit := codegen.BuildCommit
	origTime := codegen.BuildTime
	defer func() {
		codegen.BuildCommit = origCommit
		codegen.BuildTime = origTime
	}()

	codegen.BuildCommit = "abc123"
	codegen.BuildTime = ""
	v := getVersionString()
	if !strings.Contains(v, "commit: abc123") {
		t.Fatalf("expected commit info, got: %s", v)
	}
	if strings.Contains(v, "built:") {
		t.Fatalf("expected no build time, got: %s", v)
	}
}

func TestGetVersionString_CommitAndTime(t *testing.T) {
	origCommit := codegen.BuildCommit
	origTime := codegen.BuildTime
	defer func() {
		codegen.BuildCommit = origCommit
		codegen.BuildTime = origTime
	}()

	codegen.BuildCommit = "abc123"
	codegen.BuildTime = "2024-01-01"
	v := getVersionString()
	if !strings.Contains(v, "commit: abc123") || !strings.Contains(v, "built: 2024-01-01") {
		t.Fatalf("expected commit and time info, got: %s", v)
	}
}

// run() error paths

func TestRun_TypeNotFound(t *testing.T) {
	config := Config{
		PackagePath: "fmt",
		TypeNames:   "NonExistentType",
		OutputFile:  "output.go",
	}

	err := run(&config)
	if err == nil {
		t.Fatal("expected error for type not found")
	}
	if !strings.Contains(err.Error(), "type NonExistentType not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// An alias cannot be a generation target: the declaration is checked before
// go/types may have resolved the alias away.
func TestRun_AliasTarget(t *testing.T) {
	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg",
		TypeNames:   "AliasedTarget",
		OutputFile:  filepath.Join(t.TempDir(), "output.go"),
	}

	err := run(&config)
	if err == nil {
		t.Fatal("expected error for an alias target")
	}
	if !strings.Contains(err.Error(), "is an alias") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// An Annotate type argument matches its target as a type: through an alias or
// a pointer, but never a same-named type of another package or another
// instantiation of the same generic type.
func TestAnnotateTypeArgMatchesIdentity(t *testing.T) {
	local := types.NewPackage("example.com/local", "local")
	other := types.NewPackage("example.com/other", "other")
	localFoo := types.NewNamed(types.NewTypeName(token.NoPos, local, "Foo", nil), types.NewSlice(types.Typ[types.Uint64]), nil)
	otherFoo := types.NewNamed(types.NewTypeName(token.NoPos, other, "Foo", nil), types.NewSlice(types.Typ[types.Uint64]), nil)
	localAlias := types.NewAlias(types.NewTypeName(token.NoPos, local, "FooAlias", nil), localFoo)

	tparam := types.NewTypeParam(types.NewTypeName(token.NoPos, local, "T", nil), types.NewInterfaceType(nil, nil))
	generic := types.NewNamed(types.NewTypeName(token.NoPos, local, "List", nil), types.NewSlice(tparam), nil)
	generic.SetTypeParams([]*types.TypeParam{tparam})
	list32, err := types.Instantiate(nil, generic, []types.Type{types.Typ[types.Uint32]}, false)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	list64, err := types.Instantiate(nil, generic, []types.Type{types.Typ[types.Uint64]}, false)
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	list64Named, ok := list64.(*types.Named)
	if !ok {
		t.Fatalf("instantiation is %T", list64)
	}

	pkgFor := func(arg ast.Expr, typ types.Type) *packages.Package {
		return &packages.Package{PkgPath: local.Path(), Types: local, TypesInfo: &types.Info{Types: map[ast.Expr]types.TypeAndValue{arg: {Type: typ}}}}
	}
	for name, tc := range map[string]struct {
		arg    types.Type
		target *types.Named
		want   bool
	}{
		"local type":              {localFoo, localFoo, true},
		"alias of local type":     {localAlias, localFoo, true},
		"pointer to local type":   {types.NewPointer(localFoo), localFoo, true},
		"same name, other pkg":    {otherFoo, localFoo, false},
		"basic type":              {types.Typ[types.Uint64], localFoo, false},
		"same instantiation":      {list64, list64Named, true},
		"different instantiation": {list32, list64Named, false},
	} {
		arg := ast.NewIdent("X")
		if got := annotateTypeArgMatches(pkgFor(arg, tc.arg), arg, tc.target); got != tc.want {
			t.Errorf("%s: matches = %v, want %v", name, got, tc.want)
		}
	}
	if !annotateTypeArgMatches(nil, ast.NewIdent("Foo"), localFoo) {
		t.Error("without type information the spelled name should match")
	}
}

func TestRun_ObjectNotAType(t *testing.T) {
	// Println is a Func, not a TypeName
	config := Config{
		PackagePath: "fmt",
		TypeNames:   "Println",
		OutputFile:  "output.go",
	}

	err := run(&config)
	if err == nil {
		t.Fatal("expected error for non-type object")
	}
	if !strings.Contains(err.Error(), "is not a type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// run() verbose paths + codegen flow

func TestRun_VerboseWithValidType(t *testing.T) {
	// Using fmt.Stringer (an interface type) - codegen will fail at GenerateToMap
	config := Config{
		PackagePath:               "fmt",
		PackageName:               "customname",
		TypeNames:                 "Stringer",
		OutputFile:                "output.go",
		Verbose:                   true,
		Legacy:                    true,
		WithoutDynamicExpressions: true,
		WithoutFastSsz:            true,
		WithStreaming:             true,
		WithExtendedTypes:         true,
	}

	err := run(&config)
	// We expect an error from codegen since fmt.Stringer is not an SSZ type
	if err == nil {
		t.Fatal("expected error from codegen for non-SSZ type")
	}
	if !strings.Contains(err.Error(), "failed to generate code") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_FullSuccessPath(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen_output.go")

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "SimpleTypes1",
		OutputFile:  outFile,
		Verbose:     true,
		PackageName: "tests",
	}

	err := run(&config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the output file was created
	if _, err := os.Stat(outFile); os.IsNotExist(err) {
		t.Fatal("expected output file to be created")
	}
}

func TestRun_WriteFileError(t *testing.T) {
	// output path is in a nonexistent directory
	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "SimpleTypes1",
		OutputFile:  "/nonexistent-dir/subdir/output.go",
		PackageName: "tests",
	}

	err := run(&config)
	if err == nil {
		t.Fatal("expected error for bad output path")
	}
	if !strings.Contains(err.Error(), "failed to write output file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_TypeSpecificOutputFile(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen_output.go")

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "SimpleTypes1:" + outFile,
		PackageName: "tests",
	}

	err := run(&config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(outFile); os.IsNotExist(err) {
		t.Fatal("expected output file to be created")
	}
}

// testNamedFoo and testNamedT are the targets the AST-only matcher tests spell
// by name.
var (
	testNamedFoo = types.NewNamed(types.NewTypeName(token.NoPos, nil, "Foo", nil), types.Typ[types.Uint64], nil)
	testNamedT   = types.NewNamed(types.NewTypeName(token.NoPos, nil, "T", nil), types.Typ[types.Uint64], nil)
)

// lookupNamed returns the named type called name in pkg, or nil.
func lookupNamed(pkg *packages.Package, name string) *types.Named {
	obj := pkg.Types.Scope().Lookup(name)
	if obj == nil {
		return nil
	}
	named, _ := obj.Type().(*types.Named)
	return named
}

// TestAnnotationResolver covers annotatedNamedType / annotationResolver including
// the edge cases the generator's gate inputs do not normally produce: a non-named
// type and a named type from a different package both resolve to no annotation.
func TestAnnotationResolver(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedImports | packages.NeedDeps}
	pkgs, err := packages.Load(cfg,
		"github.com/pk910/dynamic-ssz/codegen/tests",
		"github.com/pk910/dynamic-ssz/sszutils")
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	var testsPkg, otherPkg *packages.Package
	for _, p := range pkgs {
		switch p.PkgPath {
		case "github.com/pk910/dynamic-ssz/codegen/tests":
			testsPkg = p
		case "github.com/pk910/dynamic-ssz/sszutils":
			otherPkg = p
		}
	}
	if testsPkg == nil || otherPkg == nil {
		t.Fatal("expected both packages to load")
	}

	resolve := annotationResolver(testsPkg)

	// An annotation written against an alias of a type is the type's
	// annotation, found by the type's own name.
	if aliasAnnotated := testsPkg.Types.Scope().Lookup("AliasAnnotated"); aliasAnnotated != nil {
		if got := resolve(aliasAnnotated.Type()); got != `ssz-max:"6"` {
			t.Errorf("annotation registered through an alias = %q, want ssz-max 6", got)
		}
	} else {
		t.Error("AliasAnnotated not found in codegen/tests")
	}

	// An alias of an annotated type resolves to that type's annotation, also
	// behind a pointer.
	if annotated := testsPkg.Types.Scope().Lookup("AnnotatedList"); annotated != nil {
		alias := types.NewAlias(types.NewTypeName(token.NoPos, testsPkg.Types, "AnnotatedListAlias", nil), annotated.Type())
		if got, want := resolve(alias), resolve(annotated.Type()); got != want || got == "" {
			t.Errorf("alias annotation = %q, want %q", got, want)
		}
		if got, want := resolve(types.NewPointer(alias)), resolve(annotated.Type()); got != want {
			t.Errorf("pointer-to-alias annotation = %q, want %q", got, want)
		}
	} else {
		t.Error("AnnotatedList not found in codegen/tests")
	}

	// Named, same-package, annotated type → its tag (also via a pointer).
	inner := testsPkg.Types.Scope().Lookup("nestedDelegatedInner")
	if inner == nil {
		t.Fatal("nestedDelegatedInner not found in tests package")
	}
	if got := resolve(inner.Type()); !strings.Contains(got, "ssz-static") {
		t.Errorf("same-package type: expected ssz-static tag, got %q", got)
	}
	if got := resolve(types.NewPointer(inner.Type())); !strings.Contains(got, "ssz-static") {
		t.Errorf("pointer to type: expected ssz-static tag, got %q", got)
	}

	// Non-named type → no annotation.
	if got := resolve(types.Typ[types.Uint64]); got != "" {
		t.Errorf("non-named type: expected empty, got %q", got)
	}

	// Named type from another package → no annotation (resolved only within pkg).
	other := otherPkg.Types.Scope().Lookup("HashWalker")
	if other == nil {
		t.Fatal("HashWalker not found in sszutils package")
	}
	if got := resolve(other.Type()); got != "" {
		t.Errorf("cross-package type: expected empty, got %q", got)
	}
}

// TestRun_ShallowBuildGate generates types that reference external, fully-delegated
// types, exercising end-to-end: the annotation resolver (annotatedNamedType +
// findAnnotateCall), the parser's shallow-build gate for both ssz-static:"true"
// (static, runtime delegated size) and ssz-static:"false" (dynamic), and the
// streaming offset header for an under-filled fixed vector of dynamic elements.
func TestRun_ShallowBuildGate(t *testing.T) {
	cases := []struct{ name, typeName string }{
		{"StaticDelegated", "NestedDelegatedContainer"},
		{"DynamicDelegated", "NestedDelegatedDynContainer"},
		{"StreamingVector", "SimpleTypes2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outFile := filepath.Join(t.TempDir(), "gen_output.go")
			config := Config{
				PackagePath:   "github.com/pk910/dynamic-ssz/codegen/tests",
				TypeNames:     tc.typeName,
				OutputFile:    outFile,
				PackageName:   "tests",
				WithStreaming: true,
			}
			if err := run(&config); err != nil {
				t.Fatalf("run(%s): %v", tc.typeName, err)
			}
			if _, err := os.Stat(outFile); os.IsNotExist(err) {
				t.Fatalf("expected output file for %s", tc.typeName)
			}
		})
	}
}

// Annotate tag parsing tests

func TestParseAnnotateTag_Empty(t *testing.T) {
	opts, err := parseAnnotateTag("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected no options, got %d", len(opts))
	}
}

func TestParseAnnotateTag_SszMax(t *testing.T) {
	opts, err := parseAnnotateTag(`ssz-max:"4096"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (max size hints), got %d", len(opts))
	}
}

func TestParseAnnotateTag_SszSize(t *testing.T) {
	opts, err := parseAnnotateTag(`ssz-size:"32"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (size hints), got %d", len(opts))
	}
}

func TestParseAnnotateTag_SszType(t *testing.T) {
	opts, err := parseAnnotateTag(`ssz-type:"list"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (type hints), got %d", len(opts))
	}
}

func TestParseAnnotateTag_Multiple(t *testing.T) {
	opts, err := parseAnnotateTag(`ssz-max:"4096" dynssz-max:"MAX_BLOBS"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (max size hints with dynamic), got %d", len(opts))
	}
}

// findAnnotateCall tests

// One go/packages load is cached per target: loading a fixture package
// type-checks it and its dependencies, which costs seconds, and the tests
// below only read the result, so one load serves them all.
var (
	loadedPackagesMu sync.Mutex
	loadedPackages   = map[string]*packages.Package{}
)

func loadTestPackage(t *testing.T, target string) *packages.Package {
	t.Helper()
	loadedPackagesMu.Lock()
	defer loadedPackagesMu.Unlock()
	if pkg, ok := loadedPackages[target]; ok {
		return pkg
	}
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
		packages.NeedSyntax | packages.NeedImports | packages.NeedDeps}
	pkgs, err := packages.Load(cfg, target)
	if err != nil {
		t.Fatalf("failed to load package %s: %v", target, err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("no packages loaded for %s", target)
	}
	loadedPackages[target] = pkgs[0]
	return pkgs[0]
}

func TestFindAnnotateCall_Found(t *testing.T) {
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/codegen/tests")

	// The merged tag also carries the generated ssz-static declaration.
	tag := findAnnotateCall(pkg, lookupNamed(pkg, "AnnotatedList"))
	if !strings.Contains(tag, `ssz-max:"20"`) {
		t.Fatalf("expected tag to contain ssz-max:\"20\", got: %q", tag)
	}
}

func TestFindAnnotateCall_Found2(t *testing.T) {
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/codegen/tests")

	tag := findAnnotateCall(pkg, lookupNamed(pkg, "AnnotatedList2"))
	if !strings.Contains(tag, `ssz-max:"10"`) {
		t.Fatalf("expected tag to contain ssz-max:\"10\", got: %q", tag)
	}
}

func TestFindAnnotateCall_NotFound(t *testing.T) {
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/codegen/tests")

	tag := findAnnotateCall(pkg, lookupNamed(pkg, "NonExistentType"))
	if tag != "" {
		t.Fatalf("expected empty tag for non-existent type, got: %q", tag)
	}
}

func TestRun_AnnotatedType(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen_output.go")

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "AnnotatedList",
		OutputFile:  outFile,
		PackageName: "tests",
	}

	err := run(&config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the output file was created and contains generated code
	data, readErr := os.ReadFile(outFile)
	if readErr != nil {
		t.Fatalf("failed to read output file: %v", readErr)
	}

	content := string(data)
	if !strings.Contains(content, "AnnotatedList") {
		t.Fatal("expected generated code to reference AnnotatedList")
	}
	if !strings.Contains(content, "MarshalSSZDyn") {
		t.Fatal("expected generated code to contain MarshalSSZDyn method")
	}
}

func TestRun_AnnotatedTypeVerbose(t *testing.T) {
	// Covers main.go:229-230 (verbose logging for annotated types)
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen_output.go")

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "AnnotatedList",
		OutputFile:  outFile,
		PackageName: "tests",
		Verbose:     true,
	}

	err := run(&config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindAnnotateCall_InitFunction(t *testing.T) {
	// Covers main.go:373-380 (init() function body scanning)
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/codegen/tests")

	tag := findAnnotateCall(pkg, lookupNamed(pkg, "InitAnnotatedList"))
	if tag != `ssz-max:"8"` {
		t.Fatalf("expected tag from init(), got: %q", tag)
	}
}

func TestFindAnnotateCall_InterpretedString(t *testing.T) {
	// Covers main.go:432-437 (interpreted string literal path)
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/codegen/tests")

	tag := findAnnotateCall(pkg, lookupNamed(pkg, "InterpretedAnnotatedList"))
	if tag != `ssz-max:"12"` {
		t.Fatalf("expected tag from interpreted string, got: %q", tag)
	}
}

func TestParseAnnotateTag_InvalidTag(t *testing.T) {
	_, err := parseAnnotateTag(`ssz-size:"notanumber"`)
	if err == nil {
		t.Fatal("expected error for invalid tag")
	}
}

func TestParseViewTypeRef(t *testing.T) {
	tests := []struct {
		input    string
		wantPkg  string
		wantType string
	}{
		{"LocalType", "", "LocalType"},
		{"github.com/pkg.RemoteType", "github.com/pkg", "RemoteType"},
		{"pkg/sub.Type", "pkg/sub", "Type"},
	}
	for _, tt := range tests {
		ref := parseViewTypeRef(tt.input)
		if ref.PackagePath != tt.wantPkg {
			t.Errorf("parseViewTypeRef(%q).PackagePath = %q, want %q", tt.input, ref.PackagePath, tt.wantPkg)
		}
		if ref.TypeName != tt.wantType {
			t.Errorf("parseViewTypeRef(%q).TypeName = %q, want %q", tt.input, ref.TypeName, tt.wantType)
		}
	}
}

func TestParseTypeSpecs(t *testing.T) {
	t.Run("OutputPrefix", func(t *testing.T) {
		specs, err := parseTypeSpecs("MyType:output=custom.go", "default.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if specs[0].OutputFile != "custom.go" {
			t.Errorf("expected custom.go, got %s", specs[0].OutputFile)
		}
	})

	t.Run("EmptyParts", func(t *testing.T) {
		specs, err := parseTypeSpecs("MyType::views=MyView:viewonly", "out.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !specs[0].IsViewOnly {
			t.Error("expected viewonly to be set")
		}
	})

	t.Run("ViewOnlyNeedsViews", func(t *testing.T) {
		_, err := parseTypeSpecs("MyType:viewonly", "out.go")
		if err == nil || !strings.Contains(err.Error(), "viewonly needs view types") {
			t.Fatalf("expected viewonly-needs-views error, got: %v", err)
		}
	})

	t.Run("MissingOutput", func(t *testing.T) {
		_, err := parseTypeSpecs("MyType", "")
		if err == nil {
			t.Fatal("expected error for missing output")
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		specs, err := parseTypeSpecs(",  ,", "out.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(specs) != 0 {
			t.Errorf("expected 0 specs, got %d", len(specs))
		}
	})
}

func TestRun_ViewTypeNotFound(t *testing.T) {
	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "SimpleTypes1:output.go:views=NonExistentView",
		OutputFile:  "output.go",
		PackageName: "tests",
	}

	err := run(&config)
	if err == nil {
		t.Fatal("expected error for non-existent view type")
	}
	if !strings.Contains(err.Error(), "view type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_ExternalViewType(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen_output.go")

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "ViewTypes1_Base:" + outFile + ":views=ViewTypes1_View1;github.com/pk910/dynamic-ssz/codegen/tests/views.ViewTypes1_View3",
		PackageName: "tests",
	}

	err := run(&config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_ExternalViewTypeError(t *testing.T) {
	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "SimpleTypes1:output.go:views=nonexistent/pkg.BadType",
		OutputFile:  "output.go",
		PackageName: "tests",
	}

	err := run(&config)
	if err == nil {
		t.Fatal("expected error for bad external view type")
	}
}

func TestRun_VerboseViewTypes(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen_output.go")

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "ViewTypes1_Base:" + outFile + ":views=ViewTypes1_View1",
		PackageName: "tests",
		Verbose:     true,
	}

	err := run(&config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_ViewOnlyType(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen_output.go")

	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "ViewTypes3_Base:" + outFile + ":views=ViewTypes3_View1:viewonly",
		PackageName: "tests",
	}

	err := run(&config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// loadPackages swap — covers the top-level error branches in run() and
// loadExternalPackage that the real go/packages.Load essentially never hits
// in production.
// -----------------------------------------------------------------------------

func withLoader(t *testing.T, stub func(*packages.Config, ...string) ([]*packages.Package, error)) {
	t.Helper()
	old := loadPackages
	loadPackages = stub
	t.Cleanup(func() { loadPackages = old })
}

func TestRun_LoadPackagesError(t *testing.T) {
	withLoader(t, func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return nil, errors.New("boom")
	})

	err := run(&Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg",
		TypeNames:   "InvalidAnnotated",
		OutputFile:  "out.go",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to load package") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_LoadPackagesEmpty(t *testing.T) {
	withLoader(t, func(_ *packages.Config, _ ...string) ([]*packages.Package, error) {
		return nil, nil
	})

	err := run(&Config{
		PackagePath: "anything",
		TypeNames:   "Foo",
		OutputFile:  "out.go",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no packages found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// loadExternalPackage error path: delegate to the real loader for the main
// package, but return an error when asked to load the external one.
func TestRun_LoadExternalPackagesError(t *testing.T) {
	realLoader := loadPackages
	withLoader(t, func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
		if len(paths) == 1 && paths[0] == "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg" {
			return realLoader(cfg, paths...)
		}
		return nil, errors.New("external boom")
	})

	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "gen.go")
	err := run(&Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg",
		PackageName: "testpkg",
		TypeNames:   "InvalidAnnotated:" + outFile + ":views=github.com/unreachable/pkg.Foo",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to load external package") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_LoadExternalPackagesEmpty(t *testing.T) {
	realLoader := loadPackages
	withLoader(t, func(cfg *packages.Config, paths ...string) ([]*packages.Package, error) {
		if len(paths) == 1 && paths[0] == "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg" {
			return realLoader(cfg, paths...)
		}
		return nil, nil // empty slice + nil error ⇒ "not found"
	})

	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "gen.go")
	err := run(&Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg",
		PackageName: "testpkg",
		TypeNames:   "InvalidAnnotated:" + outFile + ":views=github.com/empty/pkg.Foo",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "external package") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// External package caching + verbose external load — covers the
// already-cached early return and the verbose-logging branch in
// loadExternalPackage.
// -----------------------------------------------------------------------------

func TestRun_ExternalPackageCachedAndVerbose(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "gen.go")

	// Two view types from the same external package. The second resolve
	// hits the externalPackages cache, covering the cache-hit early return.
	err := run(&Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		PackageName: "tests",
		TypeNames:   "ViewTypes1_Base:" + outFile + ":views=github.com/pk910/dynamic-ssz/codegen/tests/views.ViewTypes1_View3;github.com/pk910/dynamic-ssz/codegen/tests/views.ViewTypes1_View4",
		Verbose:     true,
	})
	// We don't care about the final generation result here — the path we
	// need covered runs during spec validation before codegen.
	// But a missing external view type will return an error; use a type
	// that actually exists and rely on two distinct references to the same
	// external package.
	if err != nil && !strings.Contains(err.Error(), "view type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// resolveTypeRef's "object exists but isn't a type" branch for external
// packages — hit by using fmt.Println (a *types.Func) as a view type.
// -----------------------------------------------------------------------------

func TestRun_ExternalViewNotAType(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "gen.go")
	err := run(&Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		PackageName: "tests",
		TypeNames:   "ViewTypes1_Base:" + outFile + ":views=fmt.Println",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Verbose + view-only — covers the `mode = "view-only"` branch in the
// per-type verbose log line.
// -----------------------------------------------------------------------------

func TestRun_VerboseViewOnly(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "gen.go")
	err := run(&Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		PackageName: "tests",
		TypeNames:   "ViewTypes3_Base:" + outFile + ":views=ViewTypes3_View1:viewonly",
		Verbose:     true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Annotate-tag parse error — a testpkg type declares an Annotate tag whose
// value can't be parsed numerically, forcing run() to return a wrapped
// parseErr.
// -----------------------------------------------------------------------------

func TestRun_BadAnnotateTagInSource(t *testing.T) {
	tmp := t.TempDir()
	outFile := filepath.Join(tmp, "gen.go")
	err := run(&Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg",
		PackageName: "testpkg",
		TypeNames:   "InvalidAnnotated",
		OutputFile:  outFile,
	})
	if err == nil {
		t.Fatal("expected error for bad annotate tag")
	}
	if !strings.Contains(err.Error(), "failed to parse Annotate tag") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// findAnnotateCall: aliased sszutils import. testpkg/aliased.go imports the
// package as `szs`, so the scanner picks up the alias from imp.Name.
// -----------------------------------------------------------------------------

func TestFindAnnotateCall_AliasedImport(t *testing.T) {
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg")
	tag := findAnnotateCall(pkg, lookupNamed(pkg, "AliasedAnnotated"))
	if tag != `ssz-max:"16"` {
		t.Fatalf("expected aliased tag, got %q", tag)
	}
}

// findAnnotateCall for a type whose Annotate lives inside an init() body
// alongside an AssignStmt — covers the non-ExprStmt continue branch in
// findAnnotateCallInDecl.
func TestFindAnnotateCall_InitMixedStmts(t *testing.T) {
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg")
	// This type's Annotate is registered via an AssignStmt in init() — the
	// scanner only finds Annotate in ExprStmts, so it must NOT match.
	// But the loop must still iterate past the assign stmt without crashing
	// and past the unrelated-call ExprStmt.
	tag := findAnnotateCall(pkg, lookupNamed(pkg, "NonExprInitMarker"))
	if tag != "" {
		t.Fatalf("expected empty tag (Annotate was in AssignStmt not ExprStmt), got %q", tag)
	}

	// Meanwhile InvalidAnnotated still resolves correctly, proving the
	// scanner didn't get confused by the mixed init() body.
	tag = findAnnotateCall(pkg, lookupNamed(pkg, "InvalidAnnotated"))
	if tag == "" {
		t.Fatal("expected InvalidAnnotated tag to still be found")
	}
}

// -----------------------------------------------------------------------------
// Synthetic AST tests for findAnnotateCallInDecl / matchAnnotateCall
// defensive branches that are unreachable via valid Go source.
// -----------------------------------------------------------------------------

// astExprFromString parses a single expression string into an ast.Expr.
func astExprFromString(t *testing.T, src string) ast.Expr {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return expr
}

// astFileFromString parses a full source string into an *ast.File.
func astFileFromString(t *testing.T, src string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func TestMatchAnnotateCall_NotCall(t *testing.T) {
	expr := astExprFromString(t, `42`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestMatchAnnotateCall_WrongArgCount(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Annotate[Foo]("a", "b")`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for 2-arg call, got %q", got)
	}
}

func TestMatchAnnotateCall_NotIndexExpr(t *testing.T) {
	// Plain call, no type-parameter index expression — takes the `!ok`
	// branch on the IndexExpr type assertion.
	expr := astExprFromString(t, `sszutils.Annotate("tag")`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for non-index call, got %q", got)
	}
}

func TestMatchAnnotateCall_SelectorNameNotAnnotate(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Other[Foo]("tag")`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for non-Annotate selector, got %q", got)
	}
}

func TestMatchAnnotateCall_SelectorXNotIdent(t *testing.T) {
	// sel.X is pkg.sub (a SelectorExpr), not an Ident — takes the `!ok`
	// branch on the X type assertion.
	expr := astExprFromString(t, `pkg.sub.Annotate[Foo]("tag")`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for non-ident selector X, got %q", got)
	}
}

func TestMatchAnnotateCall_AliasMismatch(t *testing.T) {
	expr := astExprFromString(t, `other.Annotate[Foo]("tag")`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for wrong alias, got %q", got)
	}
}

func TestMatchAnnotateCall_TypeArgNotIdent(t *testing.T) {
	// Index is a non-identifier type expression (pointer).
	expr := astExprFromString(t, `sszutils.Annotate[*Foo]("tag")`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for non-ident type arg, got %q", got)
	}
}

func TestMatchAnnotateCall_TypeArgNameMismatch(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Annotate[Bar]("tag")`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for wrong type name, got %q", got)
	}
}

func TestMatchAnnotateCall_ArgNotBasicLit(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Annotate[Foo](tagVar)`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for non-literal arg, got %q", got)
	}
}

func TestMatchAnnotateCall_ArgNotStringLit(t *testing.T) {
	// An integer BasicLit is not a string — covers lit.Kind != STRING.
	expr := astExprFromString(t, `sszutils.Annotate[Foo](42)`)
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty for non-string lit, got %q", got)
	}
}

func TestMatchAnnotateCall_InterpretedStringUnquoteError(t *testing.T) {
	// Hand-build a CallExpr whose string arg is syntactically invalid when
	// unquoted. parser.ParseExpr won't produce this shape from real Go
	// source (it would reject the literal), so we construct the AST nodes
	// directly.
	lit := &ast.BasicLit{
		Kind:  token.STRING,
		Value: `"unterminated`, // doesn't start with ` and has no closing quote
	}
	call := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "sszutils"},
				Sel: &ast.Ident{Name: "Annotate"},
			},
			Index: &ast.Ident{Name: "Foo"},
		},
		Args: []ast.Expr{lit},
	}
	if got := matchAnnotateCall(nil, call, "sszutils", testNamedFoo); got != "" {
		t.Errorf("expected empty when strconv.Unquote fails, got %q", got)
	}
}

func TestMatchAnnotateCall_RawString(t *testing.T) {
	// Happy-path raw-string branch for completeness (already covered
	// indirectly, but nice to have explicit unit coverage here too).
	expr := astExprFromString(t, "sszutils.Annotate[Foo](`tag-x`)")
	if got := matchAnnotateCall(nil, expr, "sszutils", testNamedFoo); got != "tag-x" {
		t.Errorf("expected tag-x, got %q", got)
	}
}

// findAnnotateCallsInVarDecl has a `continue` for non-ValueSpec entries inside
// a VAR GenDecl. Valid Go won't produce that, so we hand-craft a GenDecl
// with mixed spec types.
func TestFindAnnotateCallsInVarDecl_NonValueSpec(t *testing.T) {
	decl := &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{
			&ast.ImportSpec{}, // deliberately wrong spec type for a VAR decl
		},
	}
	if got := findAnnotateCallsInVarDecl(nil, decl, "sszutils", testNamedFoo); len(got) != 0 {
		t.Errorf("expected no tags from GenDecl with non-ValueSpec, got %q", got)
	}
}

func TestFindAnnotateCallsInVarDecl_GenDeclNotVar(t *testing.T) {
	// TYPE decls are ignored outright.
	src := `package p
type T int
`
	f := astFileFromString(t, src)
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			t.Fatalf("expected a GenDecl, got %T", decl)
		}
		if got := findAnnotateCallsInVarDecl(nil, gen, "sszutils", testNamedT); len(got) != 0 {
			t.Errorf("expected no tags, got %q", got)
		}
	}
}

func TestFindAnnotateCallsInInit_OtherFunc(t *testing.T) {
	// Only init functions are scanned.
	src := `package p
func helper() { sszutils.Annotate[Foo]("tag") }
`
	f := astFileFromString(t, src)
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			t.Fatalf("expected a FuncDecl, got %T", decl)
		}
		if got := findAnnotateCallsInInit(nil, fn, "sszutils", testNamedFoo); len(got) != 0 {
			t.Errorf("expected no tags from a helper function, got %q", got)
		}
	}
}

// TestRun_ExternalViewLoading exercises external view-type package loading in
// run(): the first view ref (canonical path to a package the base does not
// import) triggers a fresh load + verbose log; the second ref to the same
// package by a relative spelling hits the canonical-path cache.
func TestRun_ExternalViewLoading(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.go")
	cfg := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/viewfix",
		TypeNames:   "Base:views=github.com/pk910/dynamic-ssz/dynssz-gen/testpkg/viewfix/sub.View1;./testpkg/viewfix/sub.View2",
		OutputFile:  out,
		Verbose:     true,
	}
	if err := run(&cfg); err != nil {
		t.Fatalf("run with external view types failed: %v", err)
	}
}

// A type-spec segment that is neither a recognized option nor a Go file name
// is a typo the caller must see, not a filename to create; empty type and
// view names are rejected the same way.
func TestTypeSpecRejectsJunk(t *testing.T) {
	rejected := []string{
		"TestType:junk",
		"TestType:output=a.go:more:junk",
		"TestType:views=",
		"TestType:views=A;;B",
		":output=a.go",
	}
	for _, input := range rejected {
		if _, err := parseTypeSpecs(input, "out.go"); err == nil {
			t.Errorf("parseTypeSpecs(%q) accepted junk", input)
		}
	}

	// The legacy positional output form stays accepted for Go file names.
	specs, err := parseTypeSpecs("TestType:custom.go", "out.go")
	if err != nil || specs[0].OutputFile != "custom.go" {
		t.Fatalf("positional output form broke: %v %+v", err, specs)
	}
}

// A pattern matching several packages names them instead of silently using
// the first.
func TestRun_MultiPackagePattern(t *testing.T) {
	config := Config{
		PackagePath: "./...",
		TypeNames:   "NoSuchType",
		OutputFile:  "output.go",
	}
	err := run(&config)
	if err == nil || !strings.Contains(err.Error(), "matches") {
		t.Fatalf("expected a multi-package error, got %v", err)
	}
}

// A generated type with a limit-less list produces a generator warning, which
// the CLI prints to stderr without failing the run.
func TestRun_EmitsWarnings(t *testing.T) {
	config := Config{
		PackagePath:       "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:         "UnboundedList",
		OutputFile:        filepath.Join(t.TempDir(), "out.go"),
		PackageName:       "tests",
		WithExtendedTypes: true,
	}
	if err := run(&config); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

// An invalid package name reaches the code generator's identifier check and
// fails the run with its message.
func TestRun_InvalidPackageName(t *testing.T) {
	config := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:   "SimpleTypes1",
		OutputFile:  "output.go",
		PackageName: "1nvalid",
	}
	err := run(&config)
	if err == nil || !strings.Contains(err.Error(), "invalid package name") {
		t.Fatalf("expected the package-name rejection, got: %v", err)
	}
}

// seedTransitiveImports records every transitively imported package once and
// skips entries without loaded type information.
func TestSeedTransitiveImports(t *testing.T) {
	typed := &packages.Package{PkgPath: "a", Types: types.NewPackage("a", "a")}
	typed.Imports = map[string]*packages.Package{
		"nilpkg":  nil,
		"untyped": {PkgPath: "untyped"},
	}
	root := &packages.Package{
		PkgPath: "root",
		Imports: map[string]*packages.Package{
			"a":     typed,
			"a-dup": typed,
		},
	}

	cache := map[string]*packages.Package{"a-dup": typed}
	seedTransitiveImports(cache, root)

	if cache["a"] != typed {
		t.Fatal("typed import must be seeded")
	}
	if _, ok := cache["nilpkg"]; ok {
		t.Fatal("nil import must be skipped")
	}
	if _, ok := cache["untyped"]; ok {
		t.Fatal("import without type information must be skipped")
	}
}

// Failures between the temp write and the final rename abort the whole write
// set and clean the temp files up.
func TestWriteOutputFilesInjectedFailures(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "out.go")

	renameFile = func(string, string) error { return errors.New("rename failed") }
	t.Cleanup(func() { renameFile = os.Rename })
	_, err := writeOutputFiles(map[string]string{target: "data"}, true)
	if err == nil || !strings.Contains(err.Error(), "rename failed") {
		t.Fatalf("expected the injected rename failure, got: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(base, "*.tmp*")); len(leftovers) != 0 {
		t.Fatalf("temp files not cleaned up: %v", leftovers)
	}
	renameFile = os.Rename

	createTempFile = func(dir, pattern string) (*os.File, error) {
		return os.Open(os.DevNull) // read-only: the write must fail
	}
	t.Cleanup(func() { createTempFile = os.CreateTemp })
	if _, err := writeTempFile(target, []byte("data")); err == nil {
		t.Fatal("expected a write failure on the read-only file")
	}
	createTempFile = os.CreateTemp
}

// A header template that is not made of comment lines is rejected before any
// code is generated.
func TestRun_InvalidHeaderTemplate(t *testing.T) {
	config := Config{
		PackagePath:    "github.com/pk910/dynamic-ssz/codegen/tests",
		TypeNames:      "SimpleTypes1",
		OutputFile:     "output.go",
		HeaderTemplate: "not a comment\n// Hash: {hash}\n",
	}
	err := run(&config)
	if err == nil || !errors.Is(err, codegen.ErrInvalidHeaderTemplate) {
		t.Fatalf("expected the invalid-header-template rejection, got: %v", err)
	}
}

// A write into a nonexistent directory fails before any target is touched,
// and failure messages name the problem without leaking temp-file names.
func TestWriteTempFileFailure(t *testing.T) {
	if _, err := writeTempFile("/nonexistent-dir-zz/x.go", []byte("data")); err == nil {
		t.Fatal("expected a creation error")
	} else if strings.Contains(err.Error(), ".tmp") {
		t.Fatalf("error leaks the temp-file name: %v", err)
	}

	base := t.TempDir()
	goodTarget := filepath.Join(base, "a_out.go")
	dirTarget := filepath.Join(base, "b_dir")
	if err := os.Mkdir(dirTarget, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := writeOutputFiles(map[string]string{goodTarget: "data", dirTarget: "data"}, false)
	if err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected a directory-target error, got: %v", err)
	}
	if strings.Contains(err.Error(), ".tmp") {
		t.Fatalf("error leaks the temp-file name: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(base, "*.tmp*")); len(leftovers) != 0 {
		t.Fatalf("temp files not cleaned up: %v", leftovers)
	}
	if _, statErr := os.Stat(goodTarget); statErr == nil {
		t.Fatal("no target may be written when another target fails")
	}
	dir := t.TempDir()
	target := dir + "/out.go"
	tmp, err := writeTempFile(target, []byte("data"))
	if err != nil {
		t.Fatalf("temp write: %v", err)
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Fatal("target must not exist before the rename")
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatalf("rename: %v", err)
	}
	content, _ := os.ReadFile(target)
	if string(content) != "data" {
		t.Fatalf("content = %q", content)
	}

	plain := errors.New("plain failure")
	if got := errCause(plain); got != plain {
		t.Fatalf("errCause must pass through non-path errors, got: %v", got)
	}
}

// A type registered more than once resolves a duplicated key to the same
// registration in the generator as in the runtime registry: the last one in
// the package's initialization order.
func TestFindAnnotateCall_RepeatedRegistrations(t *testing.T) {
	pkg := loadTestPackage(t, "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg")

	for _, tc := range []struct {
		name string
		typ  reflect.Type
		key  string
		want string
	}{
		{"RepeatedAnnotated", reflect.TypeOf(testpkg.RepeatedAnnotated(nil)), "ssz-size", "8"},
		{"RepeatedBlock", reflect.TypeOf(testpkg.RepeatedBlock(nil)), "ssz-max", "8"},
		{"RepeatedSame", reflect.TypeOf(testpkg.RepeatedSame(nil)), "ssz-max", "4"},
	} {
		generated := findAnnotateCall(pkg, lookupNamed(pkg, tc.name))
		runtime, ok := sszutils.LookupAnnotation(tc.typ)
		if !ok {
			t.Fatalf("%s: no runtime annotation", tc.name)
		}
		if got := reflect.StructTag(generated).Get(tc.key); got != tc.want {
			t.Errorf("%s: generator resolves %s to %q (tag %q), want %q", tc.name, tc.key, got, generated, tc.want)
		}
		if got := reflect.StructTag(runtime).Get(tc.key); got != tc.want {
			t.Errorf("%s: runtime resolves %s to %q (tag %q), want %q", tc.name, tc.key, got, runtime, tc.want)
		}
		if generated != runtime {
			t.Errorf("%s: generator tag %q != runtime tag %q", tc.name, generated, runtime)
		}
	}
}
