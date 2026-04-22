// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"errors"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/codegen"
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

func TestFindAnnotateCall_Found(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	tag := findAnnotateCall(pkgs[0], "AnnotatedList")
	if tag != `ssz-max:"20"` {
		t.Fatalf("expected tag ssz-max:\"20\", got: %q", tag)
	}
}

func TestFindAnnotateCall_Found2(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	tag := findAnnotateCall(pkgs[0], "AnnotatedList2")
	if tag != `ssz-max:"10"` {
		t.Fatalf("expected tag ssz-max:\"10\", got: %q", tag)
	}
}

func TestFindAnnotateCall_NotFound(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	tag := findAnnotateCall(pkgs[0], "NonExistentType")
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
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	tag := findAnnotateCall(pkgs[0], "InitAnnotatedList")
	if tag != `ssz-max:"8"` {
		t.Fatalf("expected tag from init(), got: %q", tag)
	}
}

func TestFindAnnotateCall_InterpretedString(t *testing.T) {
	// Covers main.go:432-437 (interpreted string literal path)
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/codegen/tests")
	if err != nil {
		t.Fatalf("failed to load package: %v", err)
	}

	tag := findAnnotateCall(pkgs[0], "InterpretedAnnotatedList")
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
		specs, err := parseTypeSpecs("MyType::viewonly", "out.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !specs[0].IsViewOnly {
			t.Error("expected viewonly to be set")
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
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}
	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	tag := findAnnotateCall(pkgs[0], "AliasedAnnotated")
	if tag != `ssz-max:"16"` {
		t.Fatalf("expected aliased tag, got %q", tag)
	}
}

// findAnnotateCall for a type whose Annotate lives inside an init() body
// alongside an AssignStmt — covers the non-ExprStmt continue branch in
// findAnnotateCallInDecl.
func TestFindAnnotateCall_InitMixedStmts(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}
	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/dynssz-gen/testpkg")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// This type's Annotate is registered via an AssignStmt in init() — the
	// scanner only finds Annotate in ExprStmts, so it must NOT match.
	// But the loop must still iterate past the assign stmt without crashing
	// and past the unrelated-call ExprStmt.
	tag := findAnnotateCall(pkgs[0], "NonExprInitMarker")
	if tag != "" {
		t.Fatalf("expected empty tag (Annotate was in AssignStmt not ExprStmt), got %q", tag)
	}

	// Meanwhile InvalidAnnotated still resolves correctly, proving the
	// scanner didn't get confused by the mixed init() body.
	tag = findAnnotateCall(pkgs[0], "InvalidAnnotated")
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
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestMatchAnnotateCall_WrongArgCount(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Annotate[Foo]("a", "b")`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for 2-arg call, got %q", got)
	}
}

func TestMatchAnnotateCall_NotIndexExpr(t *testing.T) {
	// Plain call, no type-parameter index expression — takes the `!ok`
	// branch on the IndexExpr type assertion.
	expr := astExprFromString(t, `sszutils.Annotate("tag")`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for non-index call, got %q", got)
	}
}

func TestMatchAnnotateCall_SelectorNameNotAnnotate(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Other[Foo]("tag")`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for non-Annotate selector, got %q", got)
	}
}

func TestMatchAnnotateCall_SelectorXNotIdent(t *testing.T) {
	// sel.X is pkg.sub (a SelectorExpr), not an Ident — takes the `!ok`
	// branch on the X type assertion.
	expr := astExprFromString(t, `pkg.sub.Annotate[Foo]("tag")`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for non-ident selector X, got %q", got)
	}
}

func TestMatchAnnotateCall_AliasMismatch(t *testing.T) {
	expr := astExprFromString(t, `other.Annotate[Foo]("tag")`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for wrong alias, got %q", got)
	}
}

func TestMatchAnnotateCall_TypeArgNotIdent(t *testing.T) {
	// Index is a non-identifier type expression (pointer).
	expr := astExprFromString(t, `sszutils.Annotate[*Foo]("tag")`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for non-ident type arg, got %q", got)
	}
}

func TestMatchAnnotateCall_TypeArgNameMismatch(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Annotate[Bar]("tag")`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for wrong type name, got %q", got)
	}
}

func TestMatchAnnotateCall_ArgNotBasicLit(t *testing.T) {
	expr := astExprFromString(t, `sszutils.Annotate[Foo](tagVar)`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty for non-literal arg, got %q", got)
	}
}

func TestMatchAnnotateCall_ArgNotStringLit(t *testing.T) {
	// An integer BasicLit is not a string — covers lit.Kind != STRING.
	expr := astExprFromString(t, `sszutils.Annotate[Foo](42)`)
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "" {
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
	if got := matchAnnotateCall(call, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty when strconv.Unquote fails, got %q", got)
	}
}

func TestMatchAnnotateCall_RawString(t *testing.T) {
	// Happy-path raw-string branch for completeness (already covered
	// indirectly, but nice to have explicit unit coverage here too).
	expr := astExprFromString(t, "sszutils.Annotate[Foo](`tag-x`)")
	if got := matchAnnotateCall(expr, "sszutils", "Foo"); got != "tag-x" {
		t.Errorf("expected tag-x, got %q", got)
	}
}

// findAnnotateCallInDecl has a `continue` for non-ValueSpec entries inside
// a VAR GenDecl. Valid Go won't produce that, so we hand-craft a GenDecl
// with mixed spec types.
func TestFindAnnotateCallInDecl_NonValueSpec(t *testing.T) {
	decl := &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{
			&ast.ImportSpec{}, // deliberately wrong spec type for a VAR decl
		},
	}
	if got := findAnnotateCallInDecl(decl, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty from GenDecl with non-ValueSpec, got %q", got)
	}
}

func TestFindAnnotateCallInDecl_GenDeclNotVar(t *testing.T) {
	// TYPE decls are ignored outright — exercises the early return at the
	// top of findAnnotateCallInDecl.
	src := `package p
type T int
`
	f := astFileFromString(t, src)
	for _, decl := range f.Decls {
		if got := findAnnotateCallInDecl(decl, "sszutils", "T"); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	}
}

func TestFindAnnotateCallInDecl_OtherDeclKind(t *testing.T) {
	// A LabeledStmt is not *ast.GenDecl or *ast.FuncDecl — it's also not a
	// top-level Decl, but we can still hand it as an untyped Decl to force
	// the switch's default (no branch taken). We use a BadDecl for clarity.
	var d ast.Decl = &ast.BadDecl{}
	if got := findAnnotateCallInDecl(d, "sszutils", "Foo"); got != "" {
		t.Errorf("expected empty from BadDecl, got %q", got)
	}
}
