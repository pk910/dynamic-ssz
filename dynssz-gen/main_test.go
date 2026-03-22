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

// Test helper functions for parsing logic
func TestTypeNameParsing(t *testing.T) {
	tests := []struct {
		input        string
		expectedType string
		expectedFile string
	}{
		{"TestStruct", "TestStruct", ""},
		{"TestStruct:output.go", "TestStruct", "output.go"},
		{"MyType:path/to/file.go", "MyType", "path/to/file.go"},
		{"SimpleType:", "SimpleType", ""},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			parts := strings.Split(test.input, ":")
			typeName := parts[0]
			outFile := ""
			if len(parts) > 1 {
				outFile = parts[1]
			}

			if typeName != test.expectedType {
				t.Errorf("Expected type name %s, got %s", test.expectedType, typeName)
			}
			if outFile != test.expectedFile {
				t.Errorf("Expected output file %s, got %s", test.expectedFile, outFile)
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
