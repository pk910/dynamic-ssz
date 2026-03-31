// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"flag"
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
			err := run(tt.config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
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

	err := run(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
