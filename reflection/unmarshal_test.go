// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection_test

import (
	"bytes"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	. "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
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

func TestUnmarshalExtendedTypes(t *testing.T) {
	dynssz := NewDynSsz(nil, WithExtendedTypes())

	for _, test := range commonExtendedTypesTestMatrix {
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

func TestUnmarshalExtendedTypesNoFastSsz(t *testing.T) {
	dynssz := NewDynSsz(nil, WithExtendedTypes(), WithNoFastSsz())

	for _, test := range commonExtendedTypesTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			obj := &struct {
				Data any
			}{}
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
		})
	}
}

func TestUnmarshalExtendedTypesReader(t *testing.T) {
	dynssz := NewDynSsz(nil, WithExtendedTypes(), WithNoFastSsz())

	for _, test := range commonExtendedTypesTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			obj := &struct {
				Data any
			}{}
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

func TestUnmarshalExtendedTypesDisabled(t *testing.T) {
	dynssz := NewDynSsz(nil) // no WithExtendedTypes()

	testCases := []struct {
		name        string
		target      any
		data        []byte
		expectedErr string
	}{
		{
			name:        "int8_disabled",
			target:      new(int8),
			data:        fromHex("0x2a"),
			expectedErr: "signed integers are not supported in SSZ",
		},
		{
			name:        "float32_disabled",
			target:      new(float32),
			data:        fromHex("0xc3f54840"),
			expectedErr: "floating-point numbers are not supported in SSZ",
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

func TestUnmarshalExtendedTypesErrors(t *testing.T) {
	dynssz := NewDynSsz(nil, WithExtendedTypes())

	testCases := []struct {
		name        string
		target      any
		data        []byte
		expectedErr string
	}{
		{
			name: "optional_truncated",
			target: new(struct {
				Opt *int16 `ssz-type:"optional"`
			}),
			data:        fromHex("0x04000000"),
			expectedErr: "need 1 byte for optional presence flag",
		},
		{
			name: "optional_present_truncated",
			target: new(struct {
				Opt *uint32 `ssz-type:"optional"`
			}),
			data:        fromHex("0x0400000001"),
			expectedErr: "unexpected end of SSZ",
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
			expectedErr: "is not a multiple of element size",
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
			name: "list_length_limit_exceeded",
			target: new(struct {
				Data []uint16 `ssz-max:"6"`
			}),
			data:        fromHex("0x040000000100020003000400050006000700080009000a00"),
			expectedErr: "exceeds maximum",
		},
		{
			name: "dynamic_list_length_limit_exceeded",
			target: new(struct {
				Data [][]uint8 `ssz-max:"2"`
			}),
			// 3 dynamic elements: offsets 0c000000 0d000000 0e000000, data: 0a 0b 0c
			data:        fromHex("0x040000000c0000000d0000000e0000000a0b0c"),
			expectedErr: "exceeds maximum",
		},
		{
			name: "byte_list_length_limit_exceeded",
			target: new(struct {
				Data []byte `ssz-max:"4"`
			}),
			data:        fromHex("0x04000000010203040506"),
			expectedErr: "exceeds maximum",
		},
		{
			name: "string_list_length_limit_exceeded",
			target: new(struct {
				Data string `ssz-max:"3"`
			}),
			data:        fromHex("0x040000004142434445"),
			expectedErr: "exceeds maximum",
		},
		{
			name: "bitlist_length_limit_exceeded",
			target: new(struct {
				Data []byte `ssz-type:"bitlist" ssz-max:"4"`
			}),
			// 0xff = 8 data bits (7 bits + termination at bit 7), exceeds max 4
			data:        fromHex("0x04000000ff"),
			expectedErr: "exceeds maximum",
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
			expectedErr: "wrapper descriptor must have exactly 1 field",
		},
		{
			name: "compatible_union_missing_selector",
			target: new(CompatibleUnion[struct {
				A uint32
				B uint64
			}]),
			data:        []byte{},
			expectedErr: "need 1 byte for union selector",
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
			expectedErr: "element offset",
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
			expectedErr: "bitlist missing termination bit",
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
			expectedErr: "does not match expected",
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
			expectedErr: "not enough data for vector offsets",
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
			expectedErr: "not enough data for list offsets",
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
			expectedErr: "not enough data for list offsets",
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
			expectedErr: "field consumed to position",
		},
		{
			name: "internal_container_field_size_mismatch_mixed",
			target: new(struct {
				Data Uint32WithInvalidSize
				Dyn  []uint8 `ssz-max:"100"`
			}),
			data:        fromHex("0x0102030405060708" + "0c000000"),
			expectedErr: "field consumed to position",
		},
		{
			name: "internal_container_static_field_error_mixed",
			target: new(struct {
				A bool
				B []uint8 `ssz-max:"100"`
			}),
			data:        fromHex("0x02" + "05000000"),
			expectedErr: "invalid value range",
		},
		{
			name: "internal_container_dynamic_field_size_mismatch",
			target: new(struct {
				Data Uint32AsDynamicType
			}),
			data:        fromHex("0x04000000" + "0102030405"),
			expectedErr: "bytes trailing data",
		},
		{
			name: "internal_vector_item_size_mismatch",
			target: new(struct {
				Data [2]Uint32WithInvalidSize
			}),
			data:        fromHex("0x0102030405060708090a0b0c0d0e0f10"),
			expectedErr: "element consumed to position",
		},
		{
			name: "internal_vector_dynamic_item_size_mismatch",
			target: new(struct {
				Data [1]Uint32AsDynamicType
			}),
			data:        fromHex("0x04000000" + "04000000" + "0102030405"),
			expectedErr: "bytes trailing data",
		},
		{
			name: "internal_list_item_size_mismatch",
			target: new(struct {
				Data []Uint32WithInvalidSize
			}),
			data:        fromHex("0x04000000" + "0102030405060708"),
			expectedErr: "element consumed to position",
		},
		{
			name: "internal_list_dynamic_item_size_mismatch",
			target: new(struct {
				Data []Uint32AsDynamicType
			}),
			data:        fromHex("0x04000000" + "04000000" + "0102030405"),
			expectedErr: "bytes trailing data",
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

	// Truncated reader with unmarshaler-only type triggers DecodeBytesBuf
	// error in the view unmarshaler path. The type only implements
	// DynamicViewUnmarshaler (not DynamicViewDecoder), so the buffer path
	// is used even for streaming decoders.
	t.Run("ViewUnmarshaler_TruncatedReader", func(t *testing.T) {
		var container viewUnmarshalerOnlyContainer
		err := ds.UnmarshalSSZReader(&container, bytes.NewReader(sszData[:4]), 12, WithViewDescriptor((*TestViewType1)(nil)))
		if err == nil {
			t.Fatal("expected error for truncated SSZ reader data")
		}
	})

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

func TestUnmarshalDynamicDecoder(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	data := fromHex("0x010000000000000002000000010400")
	var result TestContainerWithDynamicDecoder
	err := dynssz.UnmarshalSSZ(&result, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Field0 != 1 || result.Field1 != 2 || !result.Field2 || result.Field3 != 4 {
		t.Errorf("unexpected values: %+v", result)
	}

	// Test reader-based (non-seekable) path
	var result2 TestContainerWithDynamicDecoder
	err = dynssz.UnmarshalSSZReader(&result2, bytes.NewReader(data), len(data))
	if err != nil {
		t.Fatalf("unexpected reader error: %v", err)
	}
	if result2.Field0 != 1 || result2.Field1 != 2 || !result2.Field2 || result2.Field3 != 4 {
		t.Errorf("unexpected reader values: %+v", result2)
	}
}

func TestUnmarshalDynamicDecoderError(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	data := fromHex("0x0100000000000000")
	var result TestContainerWithDynamicDecoderError
	err := dynssz.UnmarshalSSZ(&result, data)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "test UnmarshalSSZDecoder error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnmarshalPointerDynamicVector(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Inner struct {
		Data []uint8 `ssz-max:"10"`
	}

	type Container struct {
		Items []*Inner `ssz-size:"2"`
	}

	// Encode first
	original := Container{
		Items: []*Inner{{Data: []uint8{1, 2}}, {Data: []uint8{3, 4, 5}}},
	}
	encoded, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Decode
	var result Container
	err = dynssz.UnmarshalSSZ(&result, encoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(result.Items) != 2 || result.Items[0] == nil || result.Items[1] == nil {
		t.Fatal("expected 2 non-nil items")
	}
	if !bytes.Equal(result.Items[0].Data, []uint8{1, 2}) {
		t.Errorf("item 0 mismatch: got %v", result.Items[0].Data)
	}
	if !bytes.Equal(result.Items[1].Data, []uint8{3, 4, 5}) {
		t.Errorf("item 1 mismatch: got %v", result.Items[1].Data)
	}

	// Test with reader (non-seekable)
	var result2 Container
	err = dynssz.UnmarshalSSZReader(&result2, bytes.NewReader(encoded), len(encoded))
	if err != nil {
		t.Fatalf("reader unmarshal error: %v", err)
	}
	if len(result2.Items) != 2 || result2.Items[0] == nil || result2.Items[1] == nil {
		t.Fatal("expected 2 non-nil items from reader")
	}
}

func TestUnmarshalPointerDynamicList(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Inner struct {
		Data []uint8 `ssz-max:"10"`
	}

	type Container struct {
		Items []*Inner `ssz-max:"10"`
	}

	original := Container{
		Items: []*Inner{{Data: []uint8{1, 2}}, {Data: []uint8{3, 4, 5}}},
	}
	encoded, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result Container
	err = dynssz.UnmarshalSSZ(&result, encoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(result.Items) != 2 || result.Items[0] == nil || result.Items[1] == nil {
		t.Fatal("expected 2 non-nil items")
	}

	// Test with reader (non-seekable)
	var result2 Container
	err = dynssz.UnmarshalSSZReader(&result2, bytes.NewReader(encoded), len(encoded))
	if err != nil {
		t.Fatalf("reader unmarshal error: %v", err)
	}
	if len(result2.Items) != 2 || result2.Items[0] == nil || result2.Items[1] == nil {
		t.Fatal("expected 2 non-nil items from reader")
	}
}

func TestUnmarshalStringList(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Container struct {
		Names []string `ssz-max:"10"`
	}

	original := Container{
		Names: []string{"hello", "world"},
	}
	encoded, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result Container
	err = dynssz.UnmarshalSSZ(&result, encoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(result.Names) != 2 || result.Names[0] != "hello" || result.Names[1] != "world" {
		t.Errorf("string list mismatch: got %v", result.Names)
	}
}

func TestUnmarshalPointerToVector(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Inner struct {
		Data []uint8 `ssz-max:"10"`
	}

	type Container struct {
		Items *[2]Inner
	}

	original := Container{
		Items: &[2]Inner{{Data: []uint8{1, 2}}, {Data: []uint8{3, 4, 5}}},
	}
	encoded, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result Container
	err = dynssz.UnmarshalSSZ(&result, encoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.Items == nil {
		t.Fatal("expected non-nil items pointer")
	}
	if !bytes.Equal(result.Items[0].Data, []uint8{1, 2}) {
		t.Errorf("item 0 mismatch: got %v", result.Items[0].Data)
	}
}

func TestUnmarshalPointerToList(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Inner struct {
		Data []uint8 `ssz-max:"10"`
	}

	type Container struct {
		Items *[]Inner `ssz-max:"10"`
	}

	original := Container{
		Items: &[]Inner{{Data: []uint8{1, 2}}, {Data: []uint8{3, 4, 5}}},
	}
	encoded, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result Container
	err = dynssz.UnmarshalSSZ(&result, encoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.Items == nil {
		t.Fatal("expected non-nil items pointer")
	}
	if len(*result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(*result.Items))
	}
}

func TestUnmarshalPointerStaticListElements(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Container struct {
		Items []*slug_StaticStruct1 `ssz-max:"10"`
	}

	original := Container{
		Items: []*slug_StaticStruct1{{true, []uint8{1, 2, 3}}, {false, []uint8{4, 5, 6}}},
	}
	encoded, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result Container
	err = dynssz.UnmarshalSSZ(&result, encoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(result.Items) != 2 || result.Items[0] == nil || result.Items[1] == nil {
		t.Fatal("expected 2 non-nil items")
	}
	if !result.Items[0].F1 {
		t.Error("item 0 F1 should be true")
	}
}

func TestUnmarshalEmptyDynamicList(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Inner struct {
		Data []uint8 `ssz-max:"10"`
	}

	type Container struct {
		F1    uint32
		Items []Inner `ssz-max:"10"`
	}

	original := Container{F1: 42, Items: []Inner{}}
	encoded, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result Container
	err = dynssz.UnmarshalSSZ(&result, encoded)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.F1 != 42 {
		t.Errorf("F1 mismatch: got %d", result.F1)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty list, got %d items", len(result.Items))
	}
}

func TestUnmarshalDynamicDecoderWithUnmarshal(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Container struct {
		Data TestContainerWithDynamicDecoderAndUnmarshaler
	}

	data := fromHex("0x010000000000000002000000010400")
	var result Container
	err := dynssz.UnmarshalSSZ(&result, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Data.Field0 != 1 || result.Data.Field1 != 2 {
		t.Errorf("unexpected values: %+v", result.Data)
	}
}

func TestUnmarshalReaderErrors(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	testCases := []struct {
		name        string
		target      any
		data        []byte
		expectedErr string
	}{
		{
			name: "reader_container_dynamic_offset_truncated",
			target: new(struct {
				A []uint8 `ssz-max:"100"`
			}),
			data:        fromHex("0x0400"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "reader_dynamic_vector_offset_error",
			target: new(struct {
				Data [2]struct {
					Inner []uint8 `ssz-max:"10"`
				}
			}),
			data:        fromHex("0x04000000" + "08000000"),
			expectedErr: "not enough data for vector offsets",
		},
		{
			name: "reader_dynamic_list_offset_error",
			target: new(struct {
				Data []struct {
					Inner []uint8 `ssz-max:"10"`
				} `ssz-max:"10"`
			}),
			data:        fromHex("0x04000000" + "0800000004"),
			expectedErr: "not enough data for list offsets",
		},
		{
			name: "reader_bitlist_zero_length",
			target: new(struct {
				Data []byte `ssz-type:"bitlist"`
			}),
			data:        fromHex("0x04000000"),
			expectedErr: "bitlist missing termination bit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := dynssz.UnmarshalSSZReader(tc.target, bytes.NewReader(tc.data), len(tc.data))
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", tc.expectedErr)
			} else if !contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", tc.expectedErr, err)
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

func TestUnmarshalExtendedTypesReaderErrors(t *testing.T) {
	dynssz := NewDynSsz(nil, WithExtendedTypes(), WithNoFastSsz())

	testCases := []struct {
		name        string
		target      any
		data        []byte
		expectedErr string
	}{
		{
			name:        "int8_truncated",
			target:      new(int8),
			data:        []byte{},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "int16_truncated",
			target:      new(int16),
			data:        []byte{0x01},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "int32_truncated",
			target:      new(int32),
			data:        []byte{0x01, 0x02, 0x03},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "int64_truncated",
			target:      new(int64),
			data:        fromHex("0x01020304050607"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "float32_truncated",
			target:      new(float32),
			data:        []byte{0x01, 0x02, 0x03},
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:        "float64_truncated",
			target:      new(float64),
			data:        fromHex("0x01020304050607"),
			expectedErr: "unexpected end of SSZ",
		},
		{
			name: "optional_availability_truncated",
			target: new(struct {
				Opt *int16 `ssz-type:"optional"`
			}),
			data:        fromHex("0x04000000"),
			expectedErr: "need 1 byte for optional presence flag",
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

func TestUnmarshalTruncatedReaderErrors(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())
	dynssz_fastssz := NewDynSsz(nil)
	dynssz_ext := NewDynSsz(nil, WithExtendedTypes(), WithNoFastSsz())

	type ByteVectorContainer struct {
		Data [8]byte
	}
	type StringVectorContainer struct {
		Data string `ssz-size:"8"`
	}
	type DynFieldContainer struct {
		F1 uint32
		F2 []uint8 `ssz-max:"100"`
	}
	type DynVectorContainer struct {
		Items [2]struct {
			Inner []uint8 `ssz-max:"10"`
		}
	}
	type DynStringContainer struct {
		Value string `ssz-max:"100"`
	}
	type ByteListContainer struct {
		Data []byte `ssz-max:"100"`
	}
	type DynListContainer struct {
		Items []struct {
			Inner []uint8 `ssz-max:"10"`
		} `ssz-max:"10"`
	}
	type BitlistContainer struct {
		Data []byte `ssz-type:"bitlist"`
	}
	type UnionInner struct {
		A uint32
		B uint64
	}
	type UnionContainer struct {
		Data CompatibleUnion[UnionInner]
	}
	type OptionalContainer struct {
		Opt *uint32 `ssz-type:"optional"`
	}
	type BigIntContainer struct {
		Value big.Int `ssz-max:"256"`
	}

	marshalValid := func(t *testing.T, d *DynSsz, v any) []byte {
		t.Helper()
		data, err := d.MarshalSSZ(v)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		return data
	}

	testCases := []struct {
		name        string
		dynssz      *DynSsz
		target      func() any
		fullData    func(t *testing.T) []byte
		truncateAt  int
		expectedErr string
	}{
		{
			name:   "fastssz_decode_bytes_buf_error",
			dynssz: dynssz_fastssz,
			target: func() any { return new(TestContainerWithFastSsz) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz_fastssz, &TestContainerWithFastSsz{1, 2, true, 4})
			},
			truncateAt:  5,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "dynssz_decode_bytes_buf_error",
			dynssz: dynssz,
			target: func() any { return new(TestContainerWithDynamicSsz) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &TestContainerWithDynamicSsz{1, 2, true, 4})
			},
			truncateAt:  5,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "container_dynamic_offset_non_seekable",
			dynssz: dynssz,
			target: func() any { return new(DynFieldContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &DynFieldContainer{42, []uint8{1, 2, 3}})
			},
			truncateAt:  6,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "string_vector_decode_error",
			dynssz: dynssz,
			target: func() any { return new(StringVectorContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()

				return marshalValid(t, dynssz, &StringVectorContainer{"abcdefgh"})
			},
			truncateAt:  4,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "byte_vector_decode_error",
			dynssz: dynssz,
			target: func() any { return new(ByteVectorContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &ByteVectorContainer{[8]byte{1, 2, 3, 4, 5, 6, 7, 8}})
			},
			truncateAt:  4,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "dynamic_vector_offset_non_seekable",
			dynssz: dynssz,
			target: func() any { return new(DynVectorContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &DynVectorContainer{Items: [2]struct {
					Inner []uint8 `ssz-max:"10"`
				}{{[]uint8{1, 2}}, {[]uint8{3, 4, 5}}}})
			},
			truncateAt:  5,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "dynamic_string_decode_error",
			dynssz: dynssz,
			target: func() any { return new(DynStringContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &DynStringContainer{Value: "hello world"})
			},
			truncateAt:  6,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "byte_list_decode_error",
			dynssz: dynssz,
			target: func() any { return new(ByteListContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &ByteListContainer{Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}})
			},
			truncateAt:  4,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "dynamic_list_first_offset_error",
			dynssz: dynssz,
			target: func() any { return new(DynListContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &DynListContainer{Items: []struct {
					Inner []uint8 `ssz-max:"10"`
				}{{[]uint8{1}}}})
			},
			truncateAt:  5,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "dynamic_list_subsequent_offset_error",
			dynssz: dynssz,
			target: func() any { return new(DynListContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &DynListContainer{Items: []struct {
					Inner []uint8 `ssz-max:"10"`
				}{{[]uint8{1}}, {[]uint8{2}}}})
			},
			truncateAt:  9,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "bitlist_decode_bytes_error",
			dynssz: dynssz,
			target: func() any { return new(BitlistContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz, &BitlistContainer{Data: []byte{0xff, 0x01}})
			},
			truncateAt:  5,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "union_variant_decode_error",
			dynssz: dynssz,
			target: func() any { return new(UnionContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				// selector(1) + uint32(4) + uint64(8) = 13 bytes for union data
				// container offset(4) + union data(13) = 17 bytes total
				return fromHex("0x04000000" + "00" + "01000000" + "0200000000000000")
			},
			truncateAt:  4,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "optional_availability_decode_error",
			dynssz: dynssz_ext,
			target: func() any { return new(OptionalContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				v := uint32(42)
				return marshalValid(t, dynssz_ext, &OptionalContainer{Opt: &v})
			},
			truncateAt:  4,
			expectedErr: "unexpected end of SSZ",
		},
		{
			name:   "bigint_decode_bytes_buf_error",
			dynssz: dynssz_ext,
			target: func() any { return new(BigIntContainer) },
			fullData: func(t *testing.T) []byte {
				t.Helper()
				return marshalValid(t, dynssz_ext, &BigIntContainer{Value: *big.NewInt(42)})
			},
			truncateAt:  4,
			expectedErr: "unexpected end of SSZ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fullData := tc.fullData(t)
			truncated := fullData[:tc.truncateAt]
			target := tc.target()

			err := tc.dynssz.UnmarshalSSZReader(target, bytes.NewReader(truncated), len(fullData))
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", tc.expectedErr)
			} else if !contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestUnmarshalDynamicDecoderInterfaceCheckFailure(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type PlainStruct struct {
		Field0 uint64
	}

	// Get descriptor for both value and pointer types and set flag on both
	typeDesc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(PlainStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	typeDesc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicDecoder

	ptrDesc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(&PlainStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get pointer type descriptor: %v", err)
	}
	ptrDesc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicDecoder

	data := fromHex("0x0100000000000000")
	var result PlainStruct
	err = dynssz.UnmarshalSSZ(&result, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Field0 != 1 {
		t.Errorf("expected Field0=1, got %d", result.Field0)
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

// TestUnmarshalPtrStringVector tests unmarshal of *string with ssz-size (vector).
func TestUnmarshalPtrStringVector(t *testing.T) {
	type PtrStrVecContainer struct {
		S *string `ssz-size:"8"`
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithNoFastHash())

	payload := PtrStrVecContainer{S: strPtr("testdata")}
	sszBytes, err := ds.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var result PtrStrVecContainer
	err = ds.UnmarshalSSZ(&result, sszBytes)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if result.S == nil {
		t.Fatal("expected non-nil string pointer")
	}
	if *result.S != "testdata" {
		t.Fatalf("expected 'testdata', got %q", *result.S)
	}
}

// TestUnmarshalPtrStringList tests unmarshal of *string with ssz-max (list).
func TestUnmarshalPtrStringList(t *testing.T) {
	type PtrStrLstContainer struct {
		F1 uint32
		S  *string `ssz-max:"16"`
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithNoFastHash())

	payload := PtrStrLstContainer{F1: 42, S: strPtr("hello")}
	sszBytes, err := ds.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var result PtrStrLstContainer
	err = ds.UnmarshalSSZ(&result, sszBytes)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if result.S == nil {
		t.Fatal("expected non-nil string pointer")
	}
	if *result.S != "hello" {
		t.Fatalf("expected 'hello', got %q", *result.S)
	}
}

func strPtr(s string) *string { return &s }

// viewUnmarshalerOnlyContainer implements only DynamicViewUnmarshaler
// (not DynamicViewDecoder). This forces the buffer-based view unmarshaler
// path even for streaming decoders.
type viewUnmarshalerOnlyContainer struct {
	Field0 uint64
	Field1 uint32
}

var _ sszutils.DynamicViewUnmarshaler = (*viewUnmarshalerOnlyContainer)(nil)

func (c *viewUnmarshalerOnlyContainer) UnmarshalSSZDynView(view any) func(sszutils.DynamicSpecs, []byte) error {
	switch view.(type) {
	case *TestViewType1:
		return func(_ sszutils.DynamicSpecs, buf []byte) error {
			if len(buf) < 12 {
				return fmt.Errorf("buffer too short")
			}
			c.Field0 = sszutils.UnmarshallUint64(buf[:8])
			c.Field1 = sszutils.UnmarshallUint32(buf[8:12])
			return nil
		}
	default:
		return nil
	}
}
