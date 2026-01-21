// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection_test

import (
	"bytes"
	"reflect"
	"testing"

	. "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/ssztypes"
)

var unmarshalTestMatrix = append(commonTestMatrix, []struct {
	name    string
	payload any
	ssz     []byte
	htr     []byte
}{
	// additional unmarshal tests
}...)

func TestUnmarshal(t *testing.T) {
	dynssz := NewDynSsz(nil)

	for _, test := range unmarshalTestMatrix {
		obj := &struct {
			Data any
		}{}
		// reflection hack: create new instance of payload with zero values and assign to obj.Data
		reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

		err := dynssz.UnmarshalSSZ(obj.Data, test.ssz)

		switch {
		case test.ssz == nil && err != nil:
			// expected error
		case err != nil:
			t.Errorf("test %v error: %v", test.name, err)
		default:
			htr, err := dynssz.HashTreeRoot(obj.Data)
			if err != nil {
				t.Errorf("test %v error: %v", test.name, err)
			}
			if !bytes.Equal(htr[:], test.htr) {
				t.Errorf("test %v failed: got %x, wanted %x", test.name, htr[:], test.htr)
			}
		}
	}
}

func TestUnmarshalNoFastSsz(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	for _, test := range unmarshalTestMatrix {
		obj := &struct {
			Data any
		}{}
		// reflection hack: create new instance of payload with zero values and assign to obj.Data
		reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

		err := dynssz.UnmarshalSSZ(obj.Data, test.ssz)

		switch {
		case test.ssz == nil && err != nil:
			// expected error
		case err != nil:
			t.Errorf("test %v error: %v", test.name, err)
		default:
			htr, err := dynssz.HashTreeRoot(obj.Data)
			if err != nil {
				t.Errorf("test %v error: %v", test.name, err)
			}
			if !bytes.Equal(htr[:], test.htr) {
				t.Errorf("test %v failed: got %x, wanted %x", test.name, htr[:], test.htr)
			}
		}
	}
}

func TestUnmarshalReader(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	for _, test := range unmarshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			obj := &struct {
				Data any
			}{}
			// reflection hack: create new instance of payload with zero values and assign to obj.Data
			reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

			err := dynssz.UnmarshalSSZReader(obj.Data, bytes.NewReader(test.ssz), len(test.ssz))

			switch {
			case test.ssz == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", test.name, err)
			default:
				htr, err := dynssz.HashTreeRoot(obj.Data)
				if err != nil {
					t.Errorf("test %v error: %v", test.name, err)
				}
				if !bytes.Equal(htr[:], test.htr) {
					t.Errorf("test %v failed: got %x, wanted %x", test.name, htr[:], test.htr)
				}
			}
		})
	}
}

func TestUnmarshalErrors(t *testing.T) {
	dynssz := NewDynSsz(nil)

	type Uint32WithInvalidSize uint32
	uint32desc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(Uint32WithInvalidSize(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	uint32desc.Size = 8

	type Uint32AsDynamicType uint32
	uint32desc2, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(Uint32AsDynamicType(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	uint32desc2.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	uint32desc2.Size = 0

	testCases := []struct {
		name        string
		target      any
		data        []byte
		expectedErr string
	}{
		{
			name:        "no_pointer_type",
			target:      uint64(1),
			data:        fromHex("0x00000000"),
			expectedErr: "target must be a pointer",
		},
		{
			name:        "unknown_type",
			target:      new(complex64),
			data:        fromHex("0x00000000"),
			expectedErr: "complex numbers are not supported in SSZ",
		},
		{
			name:        "truncated_data_bool",
			target:      new(bool),
			data:        []byte{},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "invalid_bool_value",
			target:      new(bool),
			data:        []byte{2},
			expectedErr: "invalid value range",
		},
		{
			name:        "truncated_data_uint8",
			target:      new(uint8),
			data:        []byte{},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "truncated_data_uint16",
			target:      new(uint16),
			data:        []byte{0x01},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "truncated_data_uint32",
			target:      new(uint32),
			data:        []byte{0x01, 0x02, 0x03},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "truncated_data_uint64",
			target:      new(uint64),
			data:        []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "truncated_container_field",
			target: new(struct {
				A uint32
				B uint64
			}),
			data:        fromHex("0x01020304050607"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "container_field_size_mismatch",
			target: new(struct {
				A [5]uint8
			}),
			data:        fromHex("0x010203"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "dynamic_field_offset_too_small",
			target: new(struct {
				A []uint8 `ssz-max:"100"`
			}),
			data:        fromHex("0x01000000"),
			expectedErr: "incorrect offset",
		},
		{
			name: "dynamic_field_offset_too_large",
			target: new(struct {
				A []uint8 `ssz-max:"100"`
			}),
			data:        fromHex("0xff000000"),
			expectedErr: "incorrect offset",
		},
		{
			name: "dynamic_field_truncated",
			target: new(struct {
				A []uint8 `ssz-size:"100"`
			}),
			data:        fromHex("0x0400000001"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "vector_size_mismatch",
			target: new(struct {
				Data [5]uint8
			}),
			data:        fromHex("0x010203"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "bitvector_padding_mismatch",
			target: new(struct {
				Flags [4]byte `ssz-type:"bitvector" ssz-bitsize:"12"`
			}),
			data:        fromHex("0xff1f"),
			expectedErr: "bitvector padding bits are not zero",
		},
		{
			name: "vector_item_size_mismatch",
			target: new(struct {
				Data [2]uint32
			}),
			data:        fromHex("0x0100000002000000030000"),
			expectedErr: "did not consume full ssz range (diff: 3, ssz size: 11)",
		},
		{
			name: "dynamic_vector_odd_byte_count",
			target: new(struct {
				Data [][]uint8 `ssz-size:"?,2" ssz-max:"10"`
			}),
			data:        fromHex("0x040000000500000001"),
			expectedErr: "invalid list length, expected multiple of 2, got 5",
		},
		{
			name: "list_item_size_mismatch",
			target: new(struct {
				Data [][2]uint16 `ssz-size:"1"`
			}),
			data:        fromHex("0x0400000001000200"),
			expectedErr: "did not consume full ssz range (diff: 4, ssz size: 8)",
		},
		{
			name: "dynamic_list_offset_bounds",
			target: new(struct {
				Data [][]uint8 `ssz-max:"10"`
			}),
			data:        fromHex("0x040000000800000010000000"),
			expectedErr: "incorrect offset",
		},
		{
			name: "type_wrapper_missing_data",
			target: new(struct {
				TypeWrapper struct{} `ssz-type:"wrapper"`
			}),
			data:        fromHex("0x"),
			expectedErr: "method not found on type",
		},
		{
			name: "compatible_union_missing_selector",
			target: new(CompatibleUnion[struct {
				A uint32
				B uint64
			}]),
			data:        []byte{},
			expectedErr: "requires at least 1 byte for selector",
		},
		{
			name: "truncated_compatible_union",
			target: new(CompatibleUnion[struct {
				A uint32
				B uint64
			}]),
			data:        []byte{0x00},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "invalid_compatible_union_variant",
			target: new(CompatibleUnion[struct {
				A uint32
				B uint64
			}]),
			data:        []byte{0x05},
			expectedErr: "invalid union variant",
		},
		{
			name: "nil_target",
			target: (*struct {
				A uint32
			})(nil),
			data:        fromHex("0x01020304"),
			expectedErr: "target pointer must not be nil",
		},
		{
			name: "invalid_uint128_size",
			target: new(struct {
				Value []byte `ssz-type:"uint128" ssz-size:"15"`
			}),
			data:        fromHex("0x0102030405060708090a0b0c0d0e0f"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "invalid_uint256_size",
			target: new(struct {
				Value []byte `ssz-type:"uint256" ssz-size:"31"`
			}),
			data:        fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "string_fixed_size_mismatch",
			target: new(struct {
				Data string `ssz-size:"5"`
			}),
			data:        fromHex("0x68656c6c6f20776f726c64"),
			expectedErr: "did not consume full ssz range (diff: 6, ssz size: 11)",
		},
		{
			name: "nested_unmarshal_error",
			target: new(struct {
				Inner struct {
					Data []uint32 `ssz-size:"2"`
				}
			}),
			data:        fromHex("0x010000"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "dynamic_nested_offset_error",
			target: new(struct {
				A uint32
				B []struct {
					C []uint8 `ssz-max:"10"`
				} `ssz-max:"10"`
			}),
			data:        fromHex("0x010000000800000008000000ff000000"),
			expectedErr: "failed decoding field B: incorrect offset",
		},
		{
			name: "map_type",
			target: new(struct {
				Data map[string]int
			}),
			data:        fromHex("0x04000000"),
			expectedErr: "maps are not supported in SSZ",
		},
		{
			name: "interface_type",
			target: new(struct {
				Data interface{}
			}),
			data:        fromHex("0x04000000"),
			expectedErr: "interfaces are not supported in SSZ",
		},
		{
			name: "channel_type",
			target: new(struct {
				Ch chan int
			}),
			data:        fromHex("0x00000000"),
			expectedErr: "channels are not supported in SSZ",
		},
		{
			name: "function_type",
			target: new(struct {
				Fn func() error
			}),
			data:        fromHex("0x00000000"),
			expectedErr: "functions are not supported in SSZ",
		},
		{
			name: "corrupted_dynamic_offsets",
			target: new(struct {
				A []uint8  `ssz-max:"10"`
				B []uint16 `ssz-max:"10"`
			}),
			data:        fromHex("0x080000000400000001020304"),
			expectedErr: "incorrect offset",
		},
		{
			name: "bitlist_not_terminated",
			target: new(struct {
				Data []byte `ssz-type:"bitlist"`
			}),
			data:        fromHex("0x0400000000"),
			expectedErr: "bitlist misses mandatory termination bit",
		},
		{
			name: "dynamic_field_offset_truncated",
			target: new(struct {
				A []uint8 `ssz-max:"100"`
			}),
			data:        fromHex("0x0400"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "static_vector_item_error",
			target: new(struct {
				Data [2]bool
			}),
			data:        fromHex("0x0002"),
			expectedErr: "invalid value range",
		},
		{
			name: "dynamic_vector_first_offset_mismatch",
			target: new(struct {
				Data [2]struct {
					Inner []uint8 `ssz-max:"10"`
				}
			}),
			// Container offset (4) + wrong first vector offset (12 instead of 8) + second offset (16) + items
			data:        fromHex("0x04000000" + "0c000000" + "10000000" + "04000000" + "04000000"),
			expectedErr: "does not match expected offset",
		},
		{
			name: "dynamic_vector_invalid_offset",
			target: new(struct {
				Data [2]struct {
					Inner []uint8 `ssz-max:"10"`
				}
			}),
			// Container offset (4) + first offset (8) + overlapping second offset (4) + item data
			data:        fromHex("0x04000000" + "08000000" + "04000000" + "04000000"),
			expectedErr: "incorrect offset",
		},
		{
			name: "dynamic_vector_item_error",
			target: new(struct {
				Data [2]struct {
					Inner []bool `ssz-max:"10"`
				}
			}),
			// Container offset (4) + vector offsets (8, 13) + item0 (offset=4, bool=1) + item1 (offset=4, invalid bool=2)
			data:        fromHex("0x04000000" + "08000000" + "0d000000" + "04000000" + "01" + "04000000" + "02"),
			expectedErr: "invalid value range",
		},
		{
			name: "static_list_item_error",
			target: new(struct {
				Data []bool `ssz-max:"10"`
			}),
			// Container offset (4) + valid bool (1) + valid bool (0) + invalid bool (2)
			data:        fromHex("0x04000000" + "01" + "00" + "02"),
			expectedErr: "invalid value range",
		},
		{
			name: "dynamic_list_item_error",
			target: new(struct {
				Data []struct {
					Inner []bool `ssz-max:"10"`
				} `ssz-max:"10"`
			}),
			// Container offset (4) + list item offset (4) + item struct (offset=4, invalid bool=2)
			data:        fromHex("0x04000000" + "04000000" + "04000000" + "02"),
			expectedErr: "invalid value range",
		},
		{
			name: "dynamic_vector_truncated_offsets",
			target: new(struct {
				Data [2]struct {
					Inner []uint8 `ssz-max:"10"`
				}
			}),
			// Container offset (4) + only partial offsets (not enough for 2 items)
			data:        fromHex("0x04000000" + "08000000"),
			expectedErr: "dynamic vector expects at least 8 bytes for offsets",
		},
		{
			name: "dynamic_list_truncated_first_offset",
			target: new(struct {
				Data []struct {
					Inner []uint8 `ssz-max:"10"`
				} `ssz-max:"10"`
			}),
			// Container offset (4) + only 2 bytes (not enough for first offset)
			data:        fromHex("0x04000000" + "0800"),
			expectedErr: "dynamic list expects at least 4 bytes for first offset",
		},
		{
			name: "dynamic_list_truncated_offsets",
			target: new(struct {
				Data []struct {
					Inner []uint8 `ssz-max:"10"`
				} `ssz-max:"10"`
			}),
			// Container offset (4) + first offset claims 2 items (8) but only 5 bytes total
			data:        fromHex("0x04000000" + "0800000004"),
			expectedErr: "dynamic list expects at least 8 bytes for offsets",
		},
		{
			name: "type_wrapper_invalid_descriptor",
			target: new(struct {
				Inner TypeWrapper[struct {
				}, []uint8]
			}),
			// Container offset (4) + first offset claims 2 items (8) but only 5 bytes total
			data:        fromHex("0x04000000" + "01020304"),
			expectedErr: "wrapper descriptor must have exactly 1 field",
		},
		{
			name: "type_wrapper_inner_error",
			target: new(struct {
				Inner TypeWrapper[struct {
					Data struct {
						Inner []uint8 `ssz-max:"10"`
					}
				}, struct {
					Inner []uint8 `ssz-max:"10"`
				}]
			}),
			// Container offset (4) + first offset claims 2 items (8) but only 5 bytes total
			data:        fromHex("0x04000000" + "01020304"),
			expectedErr: "incorrect offset",
		},
		{
			name: "fastssz_unmarshal_error",
			target: new(struct {
				F1 *TestContainerWithMarshalError `ssz-type:"custom" ssz-size:"4"`
			}),
			data:        fromHex("0x04000000" + "01020304"),
			expectedErr: "test UnmarshalSSZ error",
		},
		{
			name: "dynssz_unmarshal_error",
			target: new(struct {
				F1 TestContainerWithDynamicMarshalError
			}),
			data:        fromHex("0x04000000" + "0102030405060708"),
			expectedErr: "test UnmarshalSSZDyn error",
		},
		{
			name: "invalid_compatible_union_variant",
			target: new(CompatibleUnion[struct {
				Field0 uint16
				Field1 CompatibleUnion[struct {
					Field1 uint32
				}]
			}]),
			data:        fromHex("0x04000000" + "ff02030405060708"),
			expectedErr: "invalid union variant",
		},

		// internal defensive check errors
		{
			name: "internal_container_field_size_mismatch",
			target: new(struct {
				Data Uint32WithInvalidSize
			}),
			data:        fromHex("0x0102030405060708"),
			expectedErr: "container field did not consume expected ssz range",
		},
		{
			name: "internal_container_dynamic_field_size_mismatch",
			target: new(struct {
				Data Uint32AsDynamicType
			}),
			data:        fromHex("0x04000000" + "0102030405"),
			expectedErr: "struct field did not consume expected ssz range",
		},
		{
			name: "internal_vector_item_size_mismatch",
			target: new(struct {
				Data [2]Uint32WithInvalidSize
			}),
			data:        fromHex("0x0102030405060708090a0b0c0d0e0f10"),
			expectedErr: "vector item did not consume expected ssz range",
		},
		{
			name: "internal_vector_dynamic_item_size_mismatch",
			target: new(struct {
				Data [1]Uint32AsDynamicType
			}),
			data:        fromHex("0x04000000" + "04000000" + "0102030405"),
			expectedErr: "dynamic vector item did not consume expected ssz range",
		},
		{
			name: "internal_list_item_size_mismatch",
			target: new(struct {
				Data []Uint32WithInvalidSize
			}),
			data:        fromHex("0x04000000" + "0102030405060708"),
			expectedErr: "list item did not consume expected ssz range",
		},
		{
			name: "internal_list_dynamic_item_size_mismatch",
			target: new(struct {
				Data []Uint32AsDynamicType
			}),
			data:        fromHex("0x04000000" + "04000000" + "0102030405"),
			expectedErr: "dynamic list item did not consume expected ssz range",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := dynssz.UnmarshalSSZ(tc.target, tc.data)
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", tc.expectedErr)
			} else if !contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestUnmarshalVerbose(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz(), WithVerbose(), WithLogCb(func(format string, args ...any) {}))

	// Test with various types to exercise verbose logging paths
	testCases := []struct {
		name   string
		target any
		data   []byte
	}{
		{"simple_struct", &struct {
			Field0 uint64
			Field1 uint32
		}{},
			fromHex("0x7b00000000000000c8010000")},
		{"progressive_container", &struct {
			Field0 uint64 `ssz-index:"0"`
			Field1 uint32 `ssz-index:"1"`
		}{},
			fromHex("0x7b00000000000000c8010000")},
		{"vector", &struct {
			Data [3]uint32
		}{},
			fromHex("0x010000000200000003000000")},
		{"type_wrapper", &TypeWrapper[struct {
			Data uint32
		}, uint32]{},
			fromHex("0x2a000000")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := dynssz.UnmarshalSSZ(tc.target, tc.data)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestStringVsByteContainerUnmarshalEquivalence(t *testing.T) {
	type StringContainer struct {
		Data string `ssz-max:"100"`
	}

	type ByteContainer struct {
		Data []byte `ssz-max:"100"`
	}

	testCases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"single_char", "a"},
		{"hello", "hello"},
		{"exactly_32_bytes", "abcdefghijklmnopqrstuvwxyz123456"},
		{"over_32_bytes", "abcdefghijklmnopqrstuvwxyz1234567890"},
		{"unicode", "hello 世界"},
		{"binary", "test\x00\x01\x02\xff"},
	}

	dynssz := NewDynSsz(nil)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			strContainer := StringContainer{Data: tc.value}
			byteContainer := ByteContainer{Data: []byte(tc.value)}

			strEncoded, err := dynssz.MarshalSSZ(strContainer)
			if err != nil {
				t.Fatalf("Failed to marshal string container: %v", err)
			}

			byteEncoded, err := dynssz.MarshalSSZ(byteContainer)
			if err != nil {
				t.Fatalf("Failed to marshal byte container: %v", err)
			}

			if !bytes.Equal(strEncoded, byteEncoded) {
				t.Errorf("Encoding mismatch:\nString: %x\nBytes:  %x", strEncoded, byteEncoded)
			}

			var decodedStr StringContainer
			err = dynssz.UnmarshalSSZ(&decodedStr, strEncoded)
			if err != nil {
				t.Fatalf("Failed to unmarshal string container: %v", err)
			}

			if decodedStr.Data != tc.value {
				t.Errorf("String round-trip failed: got %q, want %q", decodedStr.Data, tc.value)
			}

			var decodedByte ByteContainer
			err = dynssz.UnmarshalSSZ(&decodedByte, byteEncoded)
			if err != nil {
				t.Fatalf("Failed to unmarshal byte container: %v", err)
			}

			if !bytes.Equal(decodedByte.Data, []byte(tc.value)) {
				t.Errorf("Byte round-trip failed: got %q, want %q", decodedByte.Data, tc.value)
			}
		})
	}
}

func TestMixedStringTypesUnmarshal(t *testing.T) {
	type MixedStruct struct {
		FixedStr1  string `ssz-size:"16"`
		DynamicStr string `ssz-max:"100"`
		FixedStr2  string `ssz-size:"8"`
		ID         uint32
	}

	dynssz := NewDynSsz(nil)

	test := MixedStruct{
		FixedStr1:  "hello",
		DynamicStr: "world",
		FixedStr2:  "test",
		ID:         42,
	}

	data, err := dynssz.MarshalSSZ(test)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded MixedStruct
	err = dynssz.UnmarshalSSZ(&decoded, data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	expectedFixedStr1 := test.FixedStr1 + string(make([]byte, 16-len(test.FixedStr1)))
	expectedFixedStr2 := test.FixedStr2 + string(make([]byte, 8-len(test.FixedStr2)))

	if decoded.FixedStr1 != expectedFixedStr1 {
		t.Errorf("FixedStr1 mismatch: got %q, want %q", decoded.FixedStr1, expectedFixedStr1)
	}
	if decoded.DynamicStr != test.DynamicStr {
		t.Errorf("DynamicStr mismatch: got %q, want %q", decoded.DynamicStr, test.DynamicStr)
	}
	if decoded.FixedStr2 != expectedFixedStr2 {
		t.Errorf("FixedStr2 mismatch: got %q, want %q", decoded.FixedStr2, expectedFixedStr2)
	}
	if decoded.ID != test.ID {
		t.Errorf("ID mismatch: got %d, want %d", decoded.ID, test.ID)
	}
}

// TestViewUnmarshaler tests the DynamicViewUnmarshaler interface via UnmarshalSSZ (buffer-based, seekable).
func TestViewUnmarshaler(t *testing.T) {
	ds := NewDynSsz(nil)

	testCases := []struct {
		name        string
		view        any
		expectValue TestContainerWithViewUnmarshaler
		expectError string
	}{
		{
			name:        "ViewUnmarshaler_Success",
			view:        (*TestViewType1)(nil),
			expectValue: TestContainerWithViewUnmarshaler{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
		{
			name:        "ViewUnmarshaler_Error",
			view:        (*TestViewType2)(nil),
			expectError: "test view unmarshaler error",
		},
		{
			name:        "ViewUnmarshaler_NoCodeForView_FallbackToReflection",
			view:        (*TestViewTypeUnknown)(nil),
			expectValue: TestContainerWithViewUnmarshaler{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
	}

	sszData := fromHex("0x0102030405060708090a0b0c")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var container TestContainerWithViewUnmarshaler
			err := ds.UnmarshalSSZ(&container, sszData, WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectError)
				}
				if !contains(err.Error(), tc.expectError) {
					t.Fatalf("expected error containing %q, got %v", tc.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if container != tc.expectValue {
				t.Fatalf("expected %+v, got %+v", tc.expectValue, container)
			}
		})
	}
}

// TestViewDecoder tests the DynamicViewDecoder interface via UnmarshalSSZReader (stream-based, non-seekable).
func TestViewDecoder(t *testing.T) {
	ds := NewDynSsz(nil)

	testCases := []struct {
		name        string
		view        any
		expectValue TestContainerWithViewUnmarshaler
		expectError string
	}{
		{
			name:        "ViewDecoder_Success",
			view:        (*TestViewType1)(nil),
			expectValue: TestContainerWithViewUnmarshaler{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
		{
			name:        "ViewDecoder_Error",
			view:        (*TestViewType2)(nil),
			expectError: "test view decoder error",
		},
		{
			name:        "ViewDecoder_NoCodeForView_FallbackToReflection",
			view:        (*TestViewTypeUnknown)(nil),
			expectValue: TestContainerWithViewUnmarshaler{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
	}

	sszData := fromHex("0x0102030405060708090a0b0c")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var container TestContainerWithViewUnmarshaler
			err := ds.UnmarshalSSZReader(&container, bytes.NewReader(sszData), len(sszData), WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectError)
				}
				if !contains(err.Error(), tc.expectError) {
					t.Fatalf("expected error containing %q, got %v", tc.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if container != tc.expectValue {
				t.Fatalf("expected %+v, got %+v", tc.expectValue, container)
			}
		})
	}
}

// TestViewUnmarshalerFlagsDetection verifies that the type cache correctly detects view unmarshaler/decoder flags.
func TestViewUnmarshalerFlagsDetection(t *testing.T) {
	ds := NewDynSsz(nil)

	// Test that view unmarshaler flags are detected on runtime type
	typeDesc, err := ds.GetTypeCache().GetTypeDescriptor(
		reflect.TypeOf(TestContainerWithViewUnmarshaler{}),
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}

	// Check for DynamicViewUnmarshaler flag
	if typeDesc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewUnmarshaler == 0 {
		t.Error("Expected DynamicViewUnmarshaler flag to be set")
	}

	// Check for DynamicViewDecoder flag
	if typeDesc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewDecoder == 0 {
		t.Error("Expected DynamicViewDecoder flag to be set")
	}

	// Test that the flags are set based on runtime type when using view descriptor
	viewTypeDesc, err := ds.GetTypeCache().GetTypeDescriptorWithSchema(
		reflect.TypeOf(TestContainerWithViewUnmarshaler{}),
		reflect.TypeOf(TestViewType1{}),
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("Failed to get type descriptor with view: %v", err)
	}

	// Verify both flags are set
	if viewTypeDesc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewUnmarshaler == 0 {
		t.Error("Expected DynamicViewUnmarshaler flag to be set with view descriptor")
	}
	if viewTypeDesc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewDecoder == 0 {
		t.Error("Expected DynamicViewDecoder flag to be set with view descriptor")
	}
}

func TestCustomFallbackUnmarshal(t *testing.T) {
	type TestStruct struct {
		ID uint32
	}

	type TestContainer struct {
		Data TestStruct
	}

	dynssz := NewDynSsz(nil)

	typeDesc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(TestContainer{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}

	structDesc := typeDesc.ContainerDesc.Fields[0].Type
	if structDesc == nil {
		t.Fatalf("Expected struct descriptor, got nil")
	}

	if structDesc.SszType != ssztypes.SszContainerType {
		t.Fatalf("Expected container type, got %v", structDesc.SszType)
	}

	structDesc.SszType = ssztypes.SszCustomType
	structDesc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicUnmarshaler

	err = dynssz.UnmarshalSSZ(&TestContainer{}, fromHex("0x01020304"))
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}
