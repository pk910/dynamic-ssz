// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"
	"strings"
	"testing"
)

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
