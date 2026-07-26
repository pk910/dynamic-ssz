// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestExtractUnionDescriptorInfo(t *testing.T) {
	ds := &dummyDynamicSpecs{}

	tests := []struct {
		name           string
		descriptorType reflect.Type
		expectError    bool
		errorContains  string
		validateInfo   func(*testing.T, map[uint8]unionVariantInfo)
	}{
		{
			name: "valid union descriptor",
			descriptorType: reflect.TypeOf(struct {
				VariantA struct {
					Field []byte `ssz-size:"32"`
				}
				VariantB struct {
					Field []uint64 `ssz-max:"1024"`
				}
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info map[uint8]unionVariantInfo) {
				t.Helper()

				if len(info) != 2 {
					t.Errorf("expected 2 variants, got %d", len(info))
				}

				// Check that both variants exist
				if _, ok := info[1]; !ok {
					t.Error("variant 1 not found")
				}
				if _, ok := info[2]; !ok {
					t.Error("variant 2 not found")
				}
			},
		},
		{
			name: "union with type hints",
			descriptorType: reflect.TypeOf(struct {
				VariantA struct {
					Field uint64 `ssz-type:"uint64"`
				}
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info map[uint8]unionVariantInfo) {
				t.Helper()

				if _, ok := info[1]; !ok {
					t.Error("variant 1 not found")
				}
			},
		},
		{
			name:           "non-struct descriptor",
			descriptorType: reflect.TypeOf("not a struct"),
			expectError:    true,
			errorContains:  "union descriptor must be a struct",
		},
		{
			name:           "empty union descriptor",
			descriptorType: reflect.TypeOf(struct{}{}),
			expectError:    true,
			errorContains:  "union descriptor struct has no fields",
		},
		{
			name: "invalid ssz-size",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-size:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "error parsing ssz-size tag for",
		},
		{
			name: "invalid ssz-max",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-max:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "error parsing ssz-max tag for",
		},
		{
			name: "invalid ssz-type",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-type:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "invalid ssz-type tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := extractUnionDescriptorInfo(tt.descriptorType, ds)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("error should contain %q, got %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if info == nil {
				t.Fatal("info should not be nil")
			}

			if tt.validateInfo != nil {
				tt.validateInfo(t, info)
			}
		})
	}
}

func TestCompatibleUnionVariantIndexing(t *testing.T) {
	// Test that variant indices are assigned based on field order
	type OrderedUnion struct {
		First  struct{ Value uint8 }
		Second struct{ Value uint16 }
		Third  struct{ Value uint32 }
		Fourth struct{ Value uint64 }
	}

	ds := &dummyDynamicSpecs{}
	info, err := extractUnionDescriptorInfo(reflect.TypeOf(OrderedUnion{}), ds)
	if err != nil {
		t.Fatalf("failed to extract union info: %v", err)
	}

	// Verify that selectors 1-4 are present
	for i := uint8(1); i <= 4; i++ {
		if _, ok := info[i]; !ok {
			t.Errorf("expected variant at selector %d", i)
		}
	}

	// Verify field types match expected order
	expectedKinds := []reflect.Kind{
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
	}

	for i, expectedKind := range expectedKinds {
		variant := info[uint8(i)+1]
		if variant.Type.Kind() != reflect.Struct {
			t.Errorf("variant %d should be struct", i)
			continue
		}

		if variant.Type.NumField() != 1 {
			t.Errorf("variant %d should have 1 field", i)
			continue
		}

		field := variant.Type.Field(0)
		if field.Type.Kind() != expectedKind {
			t.Errorf("variant %d field type mismatch: got %v, want %v", i, field.Type.Kind(), expectedKind)
		}
	}
}

func TestUnionEdgeCases(t *testing.T) {
	t.Run("union with anonymous fields", func(t *testing.T) {
		type AnonymousUnion struct {
			VariantA struct {
				X int
				Y int
			}
			VariantB struct {
				A string
				B string
			}
		}

		ds := &dummyDynamicSpecs{}
		info, err := extractUnionDescriptorInfo(reflect.TypeOf(AnonymousUnion{}), ds)
		if err != nil {
			t.Fatalf("failed to extract union info: %v", err)
		}

		if len(info) != 2 {
			t.Errorf("expected 2 variants, got %d", len(info))
		}

		// Both variants should be embedded structs
		for i := uint8(0); i < 2; i++ {
			if variant, ok := info[i]; ok {
				if variant.Type.Kind() != reflect.Struct {
					t.Errorf("variant %d should be struct", i)
				}
			}
		}
	})
}

// A union descriptor cannot hold more than 127 variants: default numbering
// would run past the EIP-8016 selector range.
func TestExtractUnionDescriptorTooManyVariants(t *testing.T) {
	fields := make([]reflect.StructField, 128)
	for i := range fields {
		fields[i] = reflect.StructField{
			Name: fmt.Sprintf("V%d", i),
			Type: reflect.TypeOf(uint32(0)),
		}
	}
	descriptorType := reflect.StructOf(fields)

	if _, err := extractUnionDescriptorInfo(descriptorType, &dummyDynamicSpecs{}); err == nil {
		t.Fatal("descriptor with 128 variants should be rejected")
	}
}

// Classic union descriptor edge cases not reachable through the public API
// golden tests: the variant count cap and per-field tag parse failures.
func TestExtractClassicUnionDescriptorInfo(t *testing.T) {
	ds := &dummyDynamicSpecs{}

	t.Run("tooManyVariants", func(t *testing.T) {
		// 129 fields: even with a leading None marker the remaining positions
		// would exceed selector 127
		fields := make([]reflect.StructField, 129)
		for i := range fields {
			fields[i] = reflect.StructField{
				Name: fmt.Sprintf("V%d", i),
				Type: reflect.TypeOf(uint32(0)),
			}
		}
		if _, _, err := extractClassicUnionDescriptorInfo(reflect.StructOf(fields), ds); err == nil {
			t.Fatal("descriptor with 129 variants should be rejected")
		}
	})

	t.Run("invalidSszSize", func(t *testing.T) {
		descriptorType := reflect.TypeOf(struct {
			Data []uint8 `ssz-size:"invalid"`
		}{})
		if _, _, err := extractClassicUnionDescriptorInfo(descriptorType, ds); err == nil || !strings.Contains(err.Error(), "ssz-size") {
			t.Fatalf("expected ssz-size parse error, got: %v", err)
		}
	})

	t.Run("invalidSszMax", func(t *testing.T) {
		descriptorType := reflect.TypeOf(struct {
			Data []uint8 `ssz-max:"invalid"`
		}{})
		if _, _, err := extractClassicUnionDescriptorInfo(descriptorType, ds); err == nil || !strings.Contains(err.Error(), "ssz-max") {
			t.Fatalf("expected ssz-max parse error, got: %v", err)
		}
	})

	t.Run("invalidSszType", func(t *testing.T) {
		descriptorType := reflect.TypeOf(struct {
			Data []uint8 `ssz-type:"invalid"`
		}{})
		if _, _, err := extractClassicUnionDescriptorInfo(descriptorType, ds); err == nil || !strings.Contains(err.Error(), "ssz-type") {
			t.Fatalf("expected ssz-type parse error, got: %v", err)
		}
	})
}
