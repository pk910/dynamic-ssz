// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection_test

import (
	"reflect"
	"testing"

	. "github.com/pk910/dynamic-ssz"
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/ssztypes"
)

var ssizeTestMatrix = append(commonTestMatrix, []struct {
	name    string
	payload any
	ssz     []byte
	htr     []byte
}{
	// nil pointer tests
	{
		"nil_pointer_1",
		(*struct{ A uint32 })(nil),
		fromHex("0x00000000"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},

	// dynamicssz value tests
	{
		"type_dynamicssz_val_1",
		TestContainerWithDynamicSsz2{1, 2, true, 4},
		fromHex("0x010000000000000002000000010400"),
		fromHex("0x4138be0e47d6daea84065f2a1e4435e16d2b269f9c2c8fcf9e6cf03de1d5026e"),
	},
	{
		"type_dynamicssz_val_2",
		TestContainerWithDynamicSsz3{1, 2, true, 4},
		fromHex("0x010000000000000002000000010400"),
		fromHex("0x4138be0e47d6daea84065f2a1e4435e16d2b269f9c2c8fcf9e6cf03de1d5026e"),
	},
	{
		"type_dynamicssz_val_3",
		struct {
			Field0 uint64
			Field1 []TestContainerWithDynamicSsz2
		}{1, []TestContainerWithDynamicSsz2{{1, 2, true, 4}, {5, 6, true, 8}}},
		fromHex("0x01000000000000000c000000010000000000000002000000010400050000000000000006000000010800"),
		fromHex("0x80b99000797f72ef1a9deae3e42fc1447648feaf1d7cd8dc1a4e20c7c64350ed"),
	},

	// fastssz value tests
	{
		"type_fastssz_val_1",
		TestContainerWithFastSsz2{1, 2, true, 4},
		fromHex("0x010000000000000002000000010400"),
		fromHex("0x4138be0e47d6daea84065f2a1e4435e16d2b269f9c2c8fcf9e6cf03de1d5026e"),
	},
	{
		"type_fastssz_val_2",
		struct {
			Field0 uint64
			Field1 []TestContainerWithFastSsz2
		}{1, []TestContainerWithFastSsz2{{1, 2, true, 4}, {5, 6, true, 8}}},
		fromHex("0x01000000000000000c000000010000000000000002000000010400050000000000000006000000010800"),
		fromHex("0x80b99000797f72ef1a9deae3e42fc1447648feaf1d7cd8dc1a4e20c7c64350ed"),
	},

	// uint128 and uint256 tests
	{
		"type_uint128_val_1",
		dynssz.TypeWrapper[struct {
			Field0 [16]byte `ssz-type:"uint128"`
		}, [16]byte]{Data: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		fromHex("0x00000000000000000000000000000000"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"type_uint256_val_1",
		dynssz.TypeWrapper[struct {
			Field0 [32]byte `ssz-type:"uint256"`
		}, [32]byte]{Data: [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}},
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
}...)

func TestSizeSSZ(t *testing.T) {
	dynssz := NewDynSsz(nil)

	for _, test := range ssizeTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			size, err := dynssz.SizeSSZ(test.payload)

			switch {
			case test.ssz == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", test.name, err)
			case size != len(test.ssz):
				t.Errorf("test %v failed: got %d, wanted %d", test.name, size, len(test.ssz))
			}
		})
	}
}

func TestSizeSSZNoFastSsz(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	for _, test := range marshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			size, err := dynssz.SizeSSZ(test.payload)

			switch {
			case test.ssz == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", test.name, err)
			case size != len(test.ssz):
				t.Errorf("test %v failed: got %d, wanted %d", test.name, size, len(test.ssz))
			}
		})
	}
}

func TestSizeSSZErrors(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type InvalidDynamicType struct{}
	invalidTypeDesc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(InvalidDynamicType{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	invalidTypeDesc.SszType = ssztypes.SszCustomType
	invalidTypeDesc.Size = 0
	invalidTypeDesc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic

	type InvalidStaticType struct{}
	invalidTypeDesc2, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(InvalidStaticType{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	invalidTypeDesc2.SszType = ssztypes.SszCustomType
	invalidTypeDesc2.Size = 10

	testCases := []struct {
		name        string
		input       any
		expectedErr string
	}{
		{
			name:        "unknown_type",
			input:       complex64(1 + 2i),
			expectedErr: "not supported in SSZ",
		},
		{
			name: "type_wrapper_missing_data",
			input: struct {
				TypeWrapper struct{} `ssz-type:"wrapper"`
			}{},
			expectedErr: "method not found on type",
		},
		{
			name: "invalid_bitvector_type",
			input: struct {
				Flags []uint16 `ssz-type:"bitvector" ssz-size:"4"`
			}{[]uint16{1, 2, 3, 4}},
			expectedErr: "bitvector ssz type can only be represented by byte slices or arrays, got uint16",
		},
		{
			name: "invalid_bitlist_type",
			input: struct {
				Bits []uint64 `ssz-type:"bitlist"`
			}{[]uint64{0xff, 0xff}},
			expectedErr: "bitlist ssz type can only be represented by byte slices, got []uint64",
		},
		{
			name: "invalid_custom_type",
			input: struct {
				Data map[string]int
			}{map[string]int{"a": 1}},
			expectedErr: "maps are not supported in SSZ",
		},
		{
			name: "invalid_interface_type",
			input: struct {
				Data interface{}
			}{42},
			expectedErr: "interfaces are not supported in SSZ",
		},
		{
			name: "channel_type",
			input: struct {
				Ch chan int
			}{make(chan int)},
			expectedErr: "channels are not supported in SSZ",
		},
		{
			name: "function_type",
			input: struct {
				Fn func() error
			}{func() error { return nil }},
			expectedErr: "functions are not supported in SSZ",
		},
		{
			name: "invalid_union_variant",
			input: struct {
				Field0 uint16
				Field1 CompatibleUnion[struct {
					Field1 uint32
				}]
			}{
				0x1234,
				CompatibleUnion[struct {
					Field1 uint32
				}]{Variant: 99, Data: uint32(42)}, // Invalid variant
			},
			expectedErr: "invalid union variant",
		},

		// invalid type tests
		{
			name: "invalid_dynamic_type_in_vector",
			input: struct {
				Data [3]InvalidDynamicType
			}{[3]InvalidDynamicType{{}, {}, {}}},
			expectedErr: "unhandled reflection kind in size check",
		},
		{
			name: "invalid_dynamic_type_in_empty_vector",
			input: struct {
				Data []InvalidDynamicType `ssz-size:"3"`
			}{[]InvalidDynamicType{}},
			expectedErr: "unhandled reflection kind in size check",
		},
		{
			name:        "invalid_static_type_in_vector",
			input:       [3]InvalidStaticType{{}, {}, {}},
			expectedErr: "unhandled reflection kind in size check",
		},
		{
			name: "invalid_dynamic_type_in_list",
			input: struct {
				Data []InvalidDynamicType
			}{[]InvalidDynamicType{{}, {}, {}}},
			expectedErr: "unhandled reflection kind in size check",
		},
		{
			name: "invalid_dynamic_type_in_union",
			input: CompatibleUnion[struct {
				V1 InvalidDynamicType
			}]{Variant: 0, Data: InvalidDynamicType{}},
			expectedErr: "unhandled reflection kind in size check",
		},
		{
			name: "invalid_dynamic_type_in_type_wrapper",
			input: TypeWrapper[struct {
				Data []InvalidDynamicType
			}, []InvalidDynamicType]{Data: []InvalidDynamicType{{}, {}, {}}},
			expectedErr: "unhandled reflection kind in size check",
		},
		{
			name: "invalid_static_type_in_empty_vector",
			input: TypeWrapper[struct {
				Data []InvalidStaticType `ssz-size:"3"`
			}, []InvalidStaticType]{Data: []InvalidStaticType{}},
			expectedErr: "unhandled reflection kind in size check",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dynssz.SizeSSZ(tc.input)
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", tc.expectedErr)
			} else if !contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestCustomFallbackSizeSSZ(t *testing.T) {
	type TestStruct struct {
		ID []uint32
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
	structDesc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicSizer

	_, err = dynssz.SizeSSZ(&TestContainer{})
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}
