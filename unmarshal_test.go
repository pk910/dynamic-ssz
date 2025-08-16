// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	. "github.com/pk910/dynamic-ssz"
)

var unmarshalTestMatrix = []struct {
	payload  any
	expected []byte
}{
	// primitive types
	{bool(false), fromHex("0x00")},
	{bool(true), fromHex("0x01")},
	{uint8(0), fromHex("0x00")},
	{uint8(255), fromHex("0xff")},
	{uint8(42), fromHex("0x2a")},
	{uint16(0), fromHex("0x0000")},
	{uint16(65535), fromHex("0xffff")},
	{uint16(1337), fromHex("0x3905")},
	{uint32(0), fromHex("0x00000000")},
	{uint32(4294967295), fromHex("0xffffffff")},
	{uint32(817482215), fromHex("0xe7c9b930")},
	{uint64(0), fromHex("0x0000000000000000")},
	{uint64(18446744073709551615), fromHex("0xffffffffffffffff")},
	{uint64(848028848028), fromHex("0x9c4f7572c5000000")},

	// arrays & slices
	{[]uint8{}, fromHex("0x")},
	{[]uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405")},
	{[5]uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405")},
	{[10]uint8{1, 2, 3, 4, 5}, fromHex("0x01020304050000000000")},

	// complex types
	{
		struct {
			F1 bool
			F2 uint8
			F3 uint16
			F4 uint32
			F5 uint64
		}{true, 1, 2, 3, 4},
		fromHex("0x01010200030000000400000000000000"),
	},
	{
		struct {
			F1 bool
			F2 []uint8  // dynamic field
			F3 []uint16 `ssz-size:"5"` // static field due to tag
			F4 uint32
		}{true, []uint8{1, 1, 1, 1}, []uint16{2, 2, 2, 2, 0}, 3},
		fromHex("0x0113000000020002000200020000000300000001010101"),
	},

	{
		struct {
			F1 uint8
			F2 [][]uint8 `ssz-size:"?,2"`
			F3 uint8
		}{42, [][]uint8{{2, 2}, {3, 0}}, 43},
		fromHex("0x2a060000002b02020300"),
	},
	{
		struct {
			F1 uint8
			F2 []slug_DynStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []slug_DynStruct1{{true, []uint8{4}}, {true, []uint8{4, 8, 4}}, {false, []uint8{}}}, 43},
		fromHex("0x2a060000002b0c000000120000001a00000001050000000401050000000408040005000000"),
	},
	{
		struct {
			F1 uint8
			F2 []*slug_StaticStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []*slug_StaticStruct1{{false, []uint8{0, 0, 0}}, {true, []uint8{4, 8, 4}}, {false, []uint8{0, 0, 0}}}, 43},
		fromHex("0x2a0000000001040804000000002b"),
	},
	{
		struct {
			F1 uint8
			F2 [][2][]struct {
				F1 uint16
			} `ssz-size:"?,2"`
			F3 uint8
		}{42, [][2][]struct {
			F1 uint16
		}{{{{F1: 2}, {F1: 3}}, {{F1: 4}, {F1: 5}}}, {{{F1: 8}, {F1: 9}}, {{F1: 10}, {F1: 11}}}}, 43},
		fromHex("0x2a060000002b0800000018000000080000000c0000000200030004000500080000000c000000080009000a000b00"),
	},
	// ssz-type annotation tests
	{
		struct {
			BitlistData []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{[]byte{0x0f, 0x01}}, // bitlist with 4 bits set, length indicator
		fromHex("0x040000000f01"),
	},

	// uint128 type tests
	{
		struct {
			Value [16]byte `ssz-type:"uint128"`
		}{[16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		fromHex("0x0102030405060708090a0b0c0d0e0f10"),
	},
	{
		struct {
			Value [2]uint64 `ssz-type:"uint128"`
		}{[2]uint64{0x0807060504030201, 0x100f0e0d0c0b0a09}},
		fromHex("0x0102030405060708090a0b0c0d0e0f10"),
	},
	{
		struct {
			Value []byte `ssz-type:"uint128" ssz-size:"16"`
		}{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		fromHex("0x0102030405060708090a0b0c0d0e0f10"),
	},

	// uint256 type tests
	{
		struct {
			Balance [32]byte `ssz-type:"uint256"`
		}{[32]byte{
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
			17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
		}},
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
	},
	{
		struct {
			Balance [4]uint64 `ssz-type:"uint256"`
		}{[4]uint64{0x0807060504030201, 0x100f0e0d0c0b0a09, 0x1817161514131211, 0x201f1e1d1c1b1a19}},
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
	},
	{
		struct {
			Balance []byte `ssz-type:"uint256" ssz-size:"32"`
		}{[]byte{
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
			17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
		}},
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
	},

	// bitvector type tests
	{
		struct {
			Flags [4]byte `ssz-type:"bitvector"`
		}{[4]byte{0xff, 0x0f, 0x00, 0xf0}},
		fromHex("0xff0f00f0"),
	},

	// explicit basic type annotations
	{
		struct {
			Value uint32 `ssz-type:"uint32"`
		}{0x12345678},
		fromHex("0x78563412"),
	},
	{
		struct {
			Value bool `ssz-type:"bool"`
		}{true},
		fromHex("0x01"),
	},

	// vector type annotation
	{
		struct {
			Values []uint64 `ssz-type:"vector" ssz-size:"3"`
		}{[]uint64{1, 2, 3}},
		fromHex("0x010000000000000002000000000000000300000000000000"),
	},

	// container type annotation
	{
		struct {
			Data struct {
				A uint32
				B uint64
			} `ssz-type:"container"`
		}{struct {
			A uint32
			B uint64
		}{A: 100, B: 200}},
		fromHex("0x64000000c800000000000000"),
	},

	// progressive bitlist test - matches Python test_progressive_bitlist.py output
	{
		func() any {
			// Create bitlist with 1000 bits where every 3rd bit is set (pattern: [false, false, true, ...])
			bits := make([]bool, 1000)
			for i := 0; i < 1000; i++ {
				bits[i] = (i%3 == 2)
			}
			// Convert to bitlist format with delimiter bit
			bytesNeeded := (len(bits) + 1 + 7) / 8
			bl := make([]byte, bytesNeeded)
			for i, bit := range bits {
				if bit {
					bl[i/8] |= 1 << (i % 8)
				}
			}

			// Set delimiter bit at position 1000 (1000 % 8 = 0, byte 125)
			bl[125] |= 0x01 // delimiter bit at position 7 of byte 125

			return struct {
				F1 []byte `ssz-type:"progressive-bitlist"`
			}{bl}
		}(),
		fromHex("04000000244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244992244901"),
	},

	// Progressive container tests
	{
		struct {
			Field0 uint64 `ssz-index:"0"`
			Field1 uint32 `ssz-index:"1"`
			Field2 bool   `ssz-index:"2"`
			Field3 uint16 `ssz-index:"3"`
		}{12345, 67890, true, 999},
		fromHex("0x39300000000000003209010001e703"),
	},
	{
		struct {
			Field0 uint64 `ssz-index:"0"`
			Field1 uint32 `ssz-index:"1"`
			Field2 bool   `ssz-index:"2"`
			Field3 uint16 `ssz-index:"3"`
		}{0, 0, false, 0},
		fromHex("0x000000000000000000000000000000"),
	},

	// CompatibleUnion tests
	{
		struct {
			Field0 uint16
			Field1 CompatibleUnion[struct {
				Field1 uint32
				Field2 [2]uint8
			}]
			Field3 uint16
		}{0x1337, CompatibleUnion[struct {
			Field1 uint32
			Field2 [2]uint8
		}]{Variant: 0, Data: uint32(0x12345678)}, 0x4242},
		fromHex("0x37130800000042420178563412"),
	},

	// string types
	{
		struct {
			Data string `ssz-max:"100"`
		}{""},
		fromHex("0x04000000"),
	},
	{
		struct {
			Data string `ssz-max:"100"`
		}{"hello"},
		fromHex("0x0400000068656c6c6f"),
	},
	{
		struct {
			Data string `ssz-max:"100"`
		}{"hello 世界"},
		fromHex("0x0400000068656c6c6f20e4b896e7958c"),
	},
	{
		struct {
			Data string `ssz-size:"32"`
		}{"hello" + string(make([]byte, 27))}, // padded to 32 bytes
		fromHex("0x68656c6c6f000000000000000000000000000000000000000000000000000000"),
	},
	{
		struct {
			Data string `ssz-size:"32"`
		}{"abcdefghijklmnopqrstuvwxyz123456"},
		fromHex("0x6162636465666768696a6b6c6d6e6f707172737475767778797a313233343536"),
	},

	// TypeWrapper test cases
	{
		func() any {
			type WrappedByteArray = TypeWrapper[struct {
				Data []byte `ssz-size:"32"`
			}, []byte]

			testData := make([]byte, 32)
			for i := range testData {
				testData[i] = byte(i)
			}
			wrapper := WrappedByteArray{
				Data: testData,
			}
			return wrapper
		}(),
		fromHex("0x000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
	},
	{
		func() any {
			type WrappedUint32List = TypeWrapper[struct {
				Data []uint32 `ssz-max:"1024"`
			}, []uint32]
			wrapper := WrappedUint32List{}
			wrapper.Set([]uint32{1, 2, 3, 4, 5})
			return wrapper
		}(),
		fromHex("0x0100000002000000030000000400000005000000"),
	},
	{
		func() any {
			type WrappedBool = TypeWrapper[struct {
				Data bool
			}, bool]
			wrapper := WrappedBool{}
			wrapper.Set(true)
			return wrapper
		}(),
		fromHex("0x01"),
	},
}

func TestUnmarshal(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for idx, test := range unmarshalTestMatrix {
		obj := &struct {
			Data any
		}{}
		// reflection hack: create new instance of payload with zero values and assign to obj.Data
		reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(test.payload)))

		err := dynssz.UnmarshalSSZ(obj.Data, test.expected)

		switch {
		case test.expected == nil && err != nil:
			// expected error
		case err != nil:
			t.Errorf("test %v error: %v", idx, err)
		default:
			objJson, err1 := json.Marshal(obj.Data)
			payloadJson, err2 := json.Marshal(test.payload)
			if err1 != nil {
				t.Errorf("failed json encode: %v", err1)
			}
			if err2 != nil {
				t.Errorf("failed json encode: %v", err2)
			}
			if !bytes.Equal(objJson, payloadJson) {
				t.Errorf("test %v failed: got %v, wanted %v", idx, string(objJson), string(payloadJson))
			}
		}
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

func TestUnmarshalErrors(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	testCases := []struct {
		name        string
		target      any
		data        []byte
		expectedErr string
	}{
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
			expectedErr: "field A expects 100 bytes, got 5",
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
			name: "vector_item_size_mismatch",
			target: new(struct {
				Data [2]uint32
			}),
			data:        fromHex("0x0100000002000000030000"),
			expectedErr: "did not consume full ssz range (consumed: 8, ssz size: 11)",
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
			expectedErr: "did not consume full ssz range (consumed: 4, ssz size: 8)",
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
		},
		{
			name: "invalid_uint256_size",
			target: new(struct {
				Value []byte `ssz-type:"uint256" ssz-size:"31"`
			}),
			data:        fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
			expectedErr: "field Value expects 32 bytes, got 31",
		},
		{
			name: "string_fixed_size_mismatch",
			target: new(struct {
				Data string `ssz-size:"5"`
			}),
			data:        fromHex("0x68656c6c6f20776f726c64"),
			expectedErr: "did not consume full ssz range (consumed: 5, ssz size: 11)",
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
