// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"strings"
	"testing"
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
