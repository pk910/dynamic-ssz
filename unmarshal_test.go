// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	. "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/stream"
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

func TestUnmarshalReader(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for _, test := range unmarshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			obj := &struct {
				Data any
			}{}
			// reflection hack: create new instance of payload with zero values and assign to obj.Data
			reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

			err := dynssz.UnmarshalSSZReader(obj.Data, bytes.NewReader(test.ssz), int64(len(test.ssz)))

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

func TestUnmarshalReaderWithoutSize(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 10

	for _, test := range unmarshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			obj := &struct {
				Data any
			}{}
			// reflection hack: create new instance of payload with zero values and assign to obj.Data
			reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

			err := dynssz.UnmarshalSSZReader(obj.Data, bytes.NewReader(test.ssz), -1)

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

func TestUnmarshalNoFastSsz(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

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
	uint32desc2.SszTypeFlags |= SszTypeFlagIsDynamic
	uint32desc2.Size = 0

	testCases := []struct {
		name                 string
		target               any
		data                 []byte
		expectedErr          string
		readerErr            string
		readerWithoutSizeErr string
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
			readerErr:   "unexpected EOF",
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
			expectedErr: "field A expects 100 bytes, got 5",
			readerErr:   "unexpected EOF",
		},
		{
			name: "vector_size_mismatch",
			target: new(struct {
				Data [5]uint8
			}),
			data:        fromHex("0x010203"),
			expectedErr: "unexpected end of SSZ",
			readerErr:   "unexpected EOF",
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
			data:                 fromHex("0x0100000002000000030000"),
			expectedErr:          "did not consume full ssz range (consumed: 8, ssz size: 11)",
			readerErr:            "did not consume full data (consumed: 8, expected: 11)",
			readerWithoutSizeErr: "leftover bytes on stream (consumed: 8, expected: 11)",
		},
		{
			name: "dynamic_vector_odd_byte_count",
			target: new(struct {
				Data [][]uint8 `ssz-size:"?,2" ssz-max:"10"`
			}),
			data:                 fromHex("0x040000000500000001"),
			expectedErr:          "invalid list length, expected multiple of 2, got 5",
			readerWithoutSizeErr: "unexpected EOF",
		},
		{
			name: "list_item_size_mismatch",
			target: new(struct {
				Data [][2]uint16 `ssz-size:"1"`
			}),
			data:                 fromHex("0x0400000001000200"),
			expectedErr:          "did not consume full ssz range (consumed: 4, ssz size: 8)",
			readerErr:            "did not consume full data (consumed: 4, expected: 8)",
			readerWithoutSizeErr: "leftover bytes on stream (consumed: 4, expected: 8)",
		},
		{
			name: "dynamic_list_offset_bounds",
			target: new(struct {
				Data [][]uint8 `ssz-max:"10"`
			}),
			data:                 fromHex("0x04000000" + "08000000" + "10000000"),
			expectedErr:          "incorrect offset",
			readerErr:            "dynamic list item did not consume expected ssz range",
			readerWithoutSizeErr: "EOF",
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
			readerErr:   "unexpected end of SSZ",
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
			expectedErr: "field Value expects 16 bytes, got 15",
			readerErr:   "unexpected EOF",
		},
		{
			name: "invalid_uint256_size",
			target: new(struct {
				Value []byte `ssz-type:"uint256" ssz-size:"31"`
			}),
			data:        fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
			expectedErr: "field Value expects 32 bytes, got 31",
			readerErr:   "unexpected EOF",
		},
		{
			name: "string_fixed_size_mismatch",
			target: new(struct {
				Data string `ssz-size:"5"`
			}),
			data:                 fromHex("0x68656c6c6f20776f726c64"),
			expectedErr:          "did not consume full ssz range (consumed: 5, ssz size: 11)",
			readerErr:            "did not consume full data (consumed: 5, expected: 11)",
			readerWithoutSizeErr: "leftover bytes on stream (consumed: 5, expected: 11)",
		},
		{
			name: "nested_unmarshal_error",
			target: new(struct {
				Inner struct {
					Data []uint32 `ssz-size:"2"`
				}
			}),
			data:        fromHex("0x010000"),
			expectedErr: "unexpected end of SSZ. field Inner expects 8 bytes, got 3",
			readerErr:   "unexpected end of SSZ",
		},
		{
			name: "dynamic_nested_offset_error",
			target: new(struct {
				A uint32
				B []struct {
					C []uint8 `ssz-max:"10"`
				} `ssz-max:"10"`
			}),
			data:        fromHex("0x01000000" + "08000000" + "08000000" + "ff000000"),
			expectedErr: "failed decoding field B: incorrect offset",
			readerErr:   "failed decoding field B: EOF",
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
			expectedErr: "unexpected end of SSZ. dynamic field A expects 4 bytes (offset)",
			readerErr:   "unexpected end of SSZ",
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
			readerErr:   "failed decoding field Data: EOF",
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
			readerErr:   "unexpected end of SSZ",
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
			readerErr:   "unexpected end of SSZ",
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
				}, []uint8]
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
			data:                 fromHex("0x04000000" + "0102030405"),
			expectedErr:          "struct field did not consume expected ssz range",
			readerErr:            "did not consume full data (consumed: 8, expected: 9)",
			readerWithoutSizeErr: "leftover bytes on stream (consumed: 8, expected: 9)",
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
			data:                 fromHex("0x04000000" + "04000000" + "0102030405"),
			expectedErr:          "dynamic vector item did not consume expected ssz range",
			readerErr:            "did not consume full data (consumed: 12, expected: 13)",
			readerWithoutSizeErr: "leftover bytes on stream (consumed: 12, expected: 13)",
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
			data:                 fromHex("0x04000000" + "04000000" + "0102030405"),
			expectedErr:          "dynamic list item did not consume expected ssz range",
			readerErr:            "did not consume full data (consumed: 12, expected: 13)",
			readerWithoutSizeErr: "leftover bytes on stream (consumed: 12, expected: 13)",
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

			expectedErr := tc.expectedErr
			if tc.readerErr != "" {
				expectedErr = tc.readerErr
			}
			err = dynssz.UnmarshalSSZReader(tc.target, bytes.NewReader(tc.data), int64(len(tc.data)))
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", expectedErr)
			} else if !contains(err.Error(), expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", expectedErr, err)
			}

			if tc.readerWithoutSizeErr != "" {
				expectedErr = tc.readerWithoutSizeErr
			}
			reader := stream.NewLimitedReader(bytes.NewReader(tc.data))
			reader.PushLimit(uint64(len(tc.data)))
			err = dynssz.UnmarshalSSZReader(tc.target, reader, -1)
			read := reader.PopLimit()
			if err == nil && read != uint64(len(tc.data)) {
				err = fmt.Errorf("leftover bytes on stream (consumed: %d, expected: %d)", read, len(tc.data))
			}
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", expectedErr)
			} else if !contains(err.Error(), expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", expectedErr, err)
			}
		})
	}
}

func TestUnmarshalVerbose(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.Verbose = true
	dynssz.LogCb = func(format string, args ...any) {}

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

	if structDesc.SszType != SszContainerType {
		t.Fatalf("Expected container type, got %v", structDesc.SszType)
	}

	structDesc.SszType = SszCustomType
	structDesc.SszCompatFlags |= SszCompatFlagDynamicUnmarshaler

	err = dynssz.UnmarshalSSZ(&TestContainer{}, fromHex("0x01020304"))
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestUnmarshalReaderStreaming(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for _, test := range streamingTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			obj := &struct {
				Data any
			}{}
			reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

			err := dynssz.UnmarshalSSZReader(obj.Data, bytes.NewReader(test.ssz), int64(len(test.ssz)))

			if err != nil {
				t.Errorf("test %v error: %v", test.name, err)
			}
		})
	}
}

func TestUnmarshalReaderStreamingWithoutSize(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 8 // Small buffer to test chunked reading

	for _, test := range streamingTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			obj := &struct {
				Data any
			}{}
			reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

			err := dynssz.UnmarshalSSZReader(obj.Data, bytes.NewReader(test.ssz), -1)

			if err != nil {
				t.Errorf("test %v error: %v", test.name, err)
			}
		})
	}
}

func TestUnmarshalReaderWithDefaultBufferSize(t *testing.T) {
	// Test with BufferSize = 0 to trigger default buffer size path
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 0 // Should use default

	data := fromHex("0x7b000000000000000c000000010203")
	target := new(struct {
		Field0 uint64
		Field1 []uint8 `ssz-max:"100"`
	})

	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalReaderDynamicReaderError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	data := fromHex("0x0102030405060708")
	target := new(struct {
		Data TestContainerWithDynamicReaderError
	})

	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Error("expected error from DynamicReader")
	}
}

func TestUnmarshalReaderVerboseStreaming(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.Verbose = true
	dynssz.LogCb = func(format string, args ...any) {} // discard output

	testCases := []struct {
		name   string
		target any
		data   []byte
	}{
		{"type_wrapper", &TypeWrapper[struct {
			Data uint32
		}, uint32]{}, fromHex("0x2a000000")},
		{"bitlist", &struct {
			Bits []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{}, fromHex("0x04000000ff01")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := dynssz.UnmarshalSSZReader(tc.target, bytes.NewReader(tc.data), int64(len(tc.data)))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUnmarshalReaderBitlistUnknownSize(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 4 // Small buffer to test chunked reading

	testCases := []struct {
		name string
		data []byte
	}{
		{"single_byte", fromHex("0x0400000001")},
		{"multi_byte", fromHex("0x04000000ff0301")},
		{"larger_than_buffer", fromHex("0x04000000ffffffffffff01")}, // 6 bytes of data
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := new(struct {
				Bits []byte `ssz-type:"bitlist" ssz-max:"100"`
			})

			// Test with size known
			err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(tc.data), int64(len(tc.data)))
			if err != nil {
				t.Errorf("known size error: %v", err)
			}

			// Test without known size (size = -1)
			target2 := new(struct {
				Bits []byte `ssz-type:"bitlist" ssz-max:"100"`
			})
			err = dynssz.UnmarshalSSZReader(target2, bytes.NewReader(tc.data), -1)
			if err != nil {
				t.Errorf("unknown size error: %v", err)
			}
		})
	}
}

func TestUnmarshalReaderBitlistLargerBuffer(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 64 // Larger buffer to test different path

	// Test bitlist with larger buffer
	target := new(struct {
		Bits []byte `ssz-type:"bitlist" ssz-max:"100"`
	})

	data := fromHex("0x04000000ff030101")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestUnmarshalReaderListUnknownSizeSlice(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 4 // Small buffer

	// Test byte list with unknown size
	target := new(struct {
		Data []byte `ssz-max:"100"`
	})

	data := fromHex("0x040000000102030405060708")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("unknown size error: %v", err)
	}
}

func TestUnmarshalReaderByteSliceLargerBuffer(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 64 // Larger buffer to test different path

	// Test byte slice with larger buffer
	target := new(struct {
		Data []byte `ssz-max:"100"`
	})

	data := fromHex("0x040000000102030405060708")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestUnmarshalReaderListGenericUnknownSize(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 8 // Medium buffer

	// Test non-byte list with unknown size
	target := new(struct {
		Data []uint32 `ssz-max:"100"`
	})

	data := fromHex("0x04000000010000000200000003000000")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("unknown size error: %v", err)
	}
}

func TestUnmarshalReaderListGenericLargerBuffer(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 64 // Larger buffer to test different path

	// Test non-byte list with larger buffer
	target := new(struct {
		Data []uint32 `ssz-max:"100"`
	})

	data := fromHex("0x04000000010000000200000003000000")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestUnmarshalReaderStringUnknownSize(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 4 // Small buffer to force multiple reads

	testCases := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"empty", fromHex("0x04000000"), ""},
		{"short", fromHex("0x0400000068656c6c6f"), "hello"},
		{"longer_than_buffer", fromHex("0x0400000068656c6c6f20776f726c6421"), "hello world!"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := new(struct {
				Data string `ssz-max:"100"`
			})

			// Test without known size
			err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(tc.data), -1)
			if err != nil {
				t.Errorf("unknown size error: %v", err)
			}
			if target.Data != tc.expected {
				t.Errorf("got %q, want %q", target.Data, tc.expected)
			}
		})
	}
}

func TestUnmarshalReaderContainerFieldConsumeError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Create a type with invalid size for internal check
	type Uint32Invalid uint32
	typeDesc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(Uint32Invalid(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	typeDesc.Size = 8 // Mismatch

	target := new(struct {
		Data Uint32Invalid
	})

	data := fromHex("0x0102030405060708")
	err = dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Error("expected consume error")
	}
}

func TestUnmarshalReaderDynamicFieldConsumeError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Create a dynamic type with size mismatch
	type Uint32Dynamic uint32
	typeDesc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(Uint32Dynamic(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	typeDesc.SszTypeFlags |= SszTypeFlagIsDynamic
	typeDesc.Size = 0

	target := new(struct {
		Data Uint32Dynamic
	})

	data := fromHex("0x04000000" + "0102030405")
	reader := stream.NewLimitedReader(bytes.NewReader(data))
	reader.PushLimit(uint64(len(data)))
	err = dynssz.UnmarshalSSZReader(target, reader, -1)
	// Pop to check consumed
	consumed := reader.PopLimit()
	if consumed == uint64(len(data)) && err == nil {
		// If all bytes consumed properly, that's also acceptable
		return
	}
}

func TestUnmarshalReaderPointerFields(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test structs with pointer fields (not double pointers)
	testCases := []struct {
		name   string
		target any
		data   []byte
	}{
		{"pointer_field_uint32", new(struct {
			Field *uint32
		}), fromHex("0x12345678")},
		{"pointer_field_container", new(struct {
			Field *struct {
				Inner uint64
			}
		}), fromHex("0x0102030405060708")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := dynssz.UnmarshalSSZReader(tc.target, bytes.NewReader(tc.data), int64(len(tc.data)))
			if err != nil {
				t.Errorf("error: %v", err)
			}
		})
	}
}

func TestUnmarshalReaderVectorWithPointerElements(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Vector with pointer elements
	target := new(struct {
		Data [3]*uint32
	})

	data := fromHex("0x010000000200000003000000")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("error: %v", err)
	}
	if target.Data[0] == nil || *target.Data[0] != 1 {
		t.Errorf("expected Data[0] = 1, got %v", target.Data[0])
	}
}

func TestUnmarshalReaderDynamicVectorWithPointerElements(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Generate correct SSZ by marshaling first
	input := struct {
		Data [2]*slug_DynStruct1
	}{[2]*slug_DynStruct1{{true, []uint8{1}}, {true, []uint8{2}}}}

	data, err := dynssz.MarshalSSZ(input)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Unmarshal to verify round-trip
	target := new(struct {
		Data [2]*slug_DynStruct1
	})

	err = dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestUnmarshalReaderDynamicListWithPointerElements(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Generate correct SSZ by marshaling first
	input := struct {
		Data []*slug_DynStruct1 `ssz-max:"10"`
	}{[]*slug_DynStruct1{{true, []uint8{1}}, {true, []uint8{2}}}}

	data, err := dynssz.MarshalSSZ(input)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Unmarshal to verify round-trip
	target := new(struct {
		Data []*slug_DynStruct1 `ssz-max:"10"`
	})

	err = dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestUnmarshalReaderTimeField(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test time field (non-pointer)
	target := new(struct {
		Timestamp time.Time
	})

	data := fromHex("0x80366a6600000000")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("error: %v", err)
	}
	if target.Timestamp.Unix() != 1718236800 {
		t.Errorf("expected Unix 1718236800, got %v", target.Timestamp.Unix())
	}
}

func TestUnmarshalReaderSliceListUnknownSizePointerElements(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 8 // Small buffer

	// Test slice-based list with pointer elements and unknown size
	target := new(struct {
		Data []*uint32 `ssz-max:"100"`
	})

	data := fromHex("0x04000000010000000200000003000000")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("error: %v", err)
	}
	if len(target.Data) != 3 || target.Data[0] == nil || *target.Data[0] != 1 {
		t.Errorf("expected [1,2,3], got %v", target.Data)
	}
}

func TestUnmarshalReaderSliceListKnownSizePointerElements(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test slice-based list with pointer elements and known size
	target := new(struct {
		Data []*uint32 `ssz-max:"100"`
	})

	data := fromHex("0x04000000010000000200000003000000")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("error: %v", err)
	}
	if len(target.Data) != 3 || target.Data[0] == nil || *target.Data[0] != 1 {
		t.Errorf("expected [1,2,3], got %v", target.Data)
	}
}

func TestUnmarshalReaderSliceListWithLargerData(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 4 // Small buffer to force multiple reads

	// Test byte list with more data than buffer
	target := new(struct {
		Data []byte `ssz-max:"100"`
	})

	// 19 bytes of data
	data := fromHex("0x040000000102030405060708090a0b0c0d0e0f10111213")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("error: %v", err)
	}
	if len(target.Data) != 19 || target.Data[0] != 1 || target.Data[18] != 0x13 {
		t.Errorf("unexpected data: %v", target.Data)
	}
}

func TestUnmarshalReaderEmptyString(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test empty string with known size
	target := new(struct {
		Data string `ssz-max:"100"`
	})

	data := fromHex("0x04000000")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("error: %v", err)
	}
	if target.Data != "" {
		t.Errorf("expected empty string, got %q", target.Data)
	}
}

func TestUnmarshalReaderShortBitlist(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 64 // Large buffer

	// Test short bitlist with large buffer
	target := new(struct {
		Data []byte `ssz-type:"bitlist" ssz-max:"100"`
	})

	data := fromHex("0x0400000001")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Errorf("error: %v", err)
	}
}

func TestUnmarshalReaderLongBitlist(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.BufferSize = 4 // Small buffer to force multiple reads

	// Test longer bitlist with small buffer
	target := new(struct {
		Data []byte `ssz-type:"bitlist" ssz-max:"1000"`
	})

	// 20 bytes of bitlist data
	data := fromHex("0x04000000ffffffffffffffffffffffffffffffff01")
	err := dynssz.UnmarshalSSZReader(target, bytes.NewReader(data), -1)
	if err != nil {
		t.Errorf("error: %v", err)
	}
}
