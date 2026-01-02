// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"
	"strings"
	"testing"
)

type dummyDynamicSpecs struct {
	specValues map[string]uint64
}

func (d *dummyDynamicSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	value, ok := d.specValues[name]
	return ok, value, nil
}

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
				if len(info) != 2 {
					t.Errorf("expected 2 variants, got %d", len(info))
				}

				// Check that both variants exist
				if _, ok := info[0]; !ok {
					t.Error("variant 0 not found")
				}
				if _, ok := info[1]; !ok {
					t.Error("variant 1 not found")
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
				if _, ok := info[0]; !ok {
					t.Error("variant 0 not found")
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
			errorContains: "failed to parse ssz-size tag for field",
		},
		{
			name: "invalid ssz-max",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-max:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "failed to parse ssz-max tag for field",
		},
		{
			name: "invalid ssz-type",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-type:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "failed to parse ssz-type tag for field",
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

	// Verify that indices 0-3 are present
	for i := uint8(0); i < 4; i++ {
		if _, ok := info[i]; !ok {
			t.Errorf("expected variant at index %d", i)
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
		variant := info[uint8(i)]
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

func TestExtractWrapperDescriptorInfo(t *testing.T) {
	ds := &dummyDynamicSpecs{}

	tests := []struct {
		name           string
		descriptorType reflect.Type
		expectError    bool
		errorContains  string
		validateInfo   func(*testing.T, *wrapperDescriptorInfo)
	}{
		{
			name: "valid descriptor with size hint",
			descriptorType: reflect.TypeOf(struct {
				Data []byte `ssz-size:"32"`
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info *wrapperDescriptorInfo) {
				if info.Type.Kind() != reflect.Slice {
					t.Error("expected slice type")
				}
			},
		},
		{
			name: "valid descriptor with max size hint",
			descriptorType: reflect.TypeOf(struct {
				Data []uint64 `ssz-max:"1024"`
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info *wrapperDescriptorInfo) {
				if info.Type.Kind() != reflect.Slice {
					t.Error("expected slice type")
				}
			},
		},
		{
			name: "valid descriptor with type hint",
			descriptorType: reflect.TypeOf(struct {
				Data uint64 `ssz-type:"uint64"`
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info *wrapperDescriptorInfo) {
				if info.Type.Kind() != reflect.Uint64 {
					t.Error("expected uint64 type")
				}
			},
		},
		{
			name:           "non-struct descriptor",
			descriptorType: reflect.TypeOf(42),
			expectError:    true,
			errorContains:  "wrapper descriptor must be a struct",
		},
		{
			name:           "descriptor with no fields",
			descriptorType: reflect.TypeOf(struct{}{}),
			expectError:    true,
			errorContains:  "wrapper descriptor must have exactly 1 field",
		},
		{
			name: "descriptor with multiple fields",
			descriptorType: reflect.TypeOf(struct {
				Field1 []byte
				Field2 string
			}{}),
			expectError:   true,
			errorContains: "wrapper descriptor must have exactly 1 field",
		},

		{
			name: "descriptor with invalid ssz-size",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-size:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "failed to parse ssz-size tag for field",
		},
		{
			name: "descriptor with invalid ssz-max",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-max:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "failed to parse ssz-max tag for field",
		},
		{
			name: "descriptor with invalid ssz-type",
			descriptorType: reflect.TypeOf(struct {
				Data []uint8 `ssz-type:"invalid"`
			}{}),
			expectError:   true,
			errorContains: "failed to parse ssz-type tag for field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := extractWrapperDescriptorInfo(tt.descriptorType, ds)

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
