// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"testing"
)

// TestStrictTypeAnnotations tests various ssz-type annotations to improve coverage
func TestStrictTypeAnnotations(t *testing.T) {
	dynssz := NewDynSsz(nil)

	testCases := []struct {
		name   string
		value  interface{}
		hasErr bool
	}{
		{
			name: "auto type annotation",
			value: struct {
				Value uint64 `ssz-type:"auto"`
			}{42},
		},
		{
			name: "question mark type annotation",
			value: struct {
				Value uint64 `ssz-type:"?"`
			}{42},
		},
		{
			name: "list type annotation",
			value: struct {
				Values []uint32 `ssz-type:"list" ssz-max:"10"`
			}{[]uint32{1, 2, 3}},
		},
		{
			name: "uint8 type annotation",
			value: struct {
				Value uint8 `ssz-type:"uint8"`
			}{255},
		},
		{
			name: "uint16 type annotation",
			value: struct {
				Value uint16 `ssz-type:"uint16"`
			}{65535},
		},
		{
			name: "uint64 type annotation",
			value: struct {
				Value uint64 `ssz-type:"uint64"`
			}{12345},
		},
		{
			name: "multi-dimensional with type hints",
			value: struct {
				Values [][]uint8 `ssz-type:"list,list" ssz-size:"?,?" ssz-max:"10,10"`
			}{[][]uint8{{1, 2}, {3, 4}}},
		},
		{
			name: "bitvector type annotation",
			value: struct {
				Flags [3]byte `ssz-type:"bitvector" ssz-bitsize:"12"`
			}{[3]byte{0xff, 0x0f, 0x00}},
		},
		{
			name: "invalid ssz-type",
			value: struct {
				Value uint32 `ssz-type:"invalid"`
			}{42},
			hasErr: true,
		},
		{
			name: "uint128 with wrong size array",
			value: struct {
				Value [8]byte `ssz-type:"uint128"`
			}{[8]byte{1, 2, 3, 4, 5, 6, 7, 8}},
			hasErr: true,
		},
		{
			name: "uint256 with wrong size array",
			value: struct {
				Value [16]byte `ssz-type:"uint256"`
			}{[16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
			hasErr: true,
		},
		{
			name: "uint128 with wrong element type",
			value: struct {
				Value [16]uint16 `ssz-type:"uint128"`
			}{[16]uint16{}},
			hasErr: true,
		},
		{
			name: "uint256 with slice of uint16",
			value: struct {
				Value []uint16 `ssz-type:"uint256" ssz-size:"16"`
			}{make([]uint16, 16)},
			hasErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test marshaling
			_, err := dynssz.MarshalSSZ(tc.value)
			if tc.hasErr && err == nil {
				t.Errorf("expected error but got none")
			} else if !tc.hasErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Test hash tree root
			_, err = dynssz.HashTreeRoot(tc.value)
			if tc.hasErr && err == nil {
				t.Errorf("expected error for HashTreeRoot but got none")
			} else if !tc.hasErr && err != nil {
				t.Errorf("unexpected error for HashTreeRoot: %v", err)
			}
		})
	}
}
