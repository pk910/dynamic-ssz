// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"strings"
	"testing"
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

// Test config struct creation
func TestConfigCreation(t *testing.T) {
	config := Config{
		PackagePath:               "testpkg",
		PackageName:               "customname",
		TypeNames:                 "Type1,Type2",
		OutputFile:                "output.go",
		Verbose:                   true,
		Legacy:                    true,
		WithoutDynamicExpressions: true,
		WithoutFastSsz:            true,
	}

	if config.PackagePath != "testpkg" {
		t.Errorf("Expected PackagePath 'testpkg', got '%s'", config.PackagePath)
	}
	if config.TypeNames != "Type1,Type2" {
		t.Errorf("Expected TypeNames 'Type1,Type2', got '%s'", config.TypeNames)
	}
	if !config.Verbose {
		t.Errorf("Expected Verbose true, got %v", config.Verbose)
	}
	if !config.Legacy {
		t.Errorf("Expected Legacy true, got %v", config.Legacy)
	}
}
