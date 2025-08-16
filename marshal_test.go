// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz_test

import (
	"bytes"
	"testing"

	. "github.com/pk910/dynamic-ssz"
)

var marshalTestMatrix = []struct {
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
		}{true, []uint8{1, 1, 1, 1}, []uint16{2, 2, 2, 2}, 3},
		fromHex("0x0113000000020002000200020000000300000001010101"),
	},
	{
		struct {
			F1 uint8
			F2 [][]uint8 `ssz-size:"?,2"`
			F3 uint8
		}{42, [][]uint8{{2, 2}, {3}}, 43},
		fromHex("0x2a060000002b02020300"),
	},
	{
		struct {
			F1 uint8
			F2 []slug_DynStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []slug_DynStruct1{{true, []uint8{4}}, {true, []uint8{4, 8, 4}}}, 43},
		fromHex("0x2a060000002b0c000000120000001a00000001050000000401050000000408040005000000"),
	},
	{
		struct {
			F1 uint8
			F2 []*slug_StaticStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []*slug_StaticStruct1{nil, {true, []uint8{4, 8, 4}}}, 43},
		fromHex("0x2a0000000001040804000000002b"),
	},
	{
		struct {
			F1 uint8
			F2 []*slug_StaticStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []*slug_StaticStruct1{nil, nil, nil, nil}, 43},
		nil, // size too long error
	},
	{
		struct {
			F1 uint8
			F2 [2][]*slug_StaticStruct1 `ssz-size:"2,3"`
			F3 uint8
		}{42, [2][]*slug_StaticStruct1{{nil, nil, nil}, {nil, nil, nil, nil}}, 43},
		nil, // size too long error
	},
	{
		struct {
			F1 uint8
			F2 [][]struct {
				F1 uint16
			} `ssz-size:"?,2"`
			F3 uint8
		}{42, [][]struct {
			F1 uint16
		}{{{F1: 2}, {F1: 3}}, {{F1: 4}, {F1: 5}}}, 43},
		fromHex("0x2a060000002b0200030004000500"),
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
	{
		struct {
			F1 [][]uint16 `ssz-size:"?,2" ssz-max:"10"`
		}{[][]uint16{{2, 3}, {4, 5}, {8, 9}, {10, 11}}},
		fromHex("0x040000000200030004000500080009000a000b00"),
	},
	{
		[2]uint16{1, 2},
		fromHex("0x01000200"),
	},

	// ssz-type annotation tests
	{
		struct {
			BitlistData []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{[]byte{0x0f, 0x01}}, // bitlist with 4 bits set, length indicator
		fromHex("0x040000000f01"),
	},

	// nil pointer tests
	{
		(*struct{ A uint32 })(nil),
		fromHex("0x00000000"),
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
		}{"hello"},
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
			return WrappedByteArray{
				Data: testData,
			}
		}(),
		fromHex("0x000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
	},
	{
		func() any {
			type WrappedUint32List = TypeWrapper[struct {
				Data []uint32 `ssz-max:"1024"`
			}, []uint32]
			return WrappedUint32List{
				Data: []uint32{1, 2, 3, 4, 5},
			}
		}(),
		fromHex("0x0100000002000000030000000400000005000000"),
	},
	{
		func() any {
			type WrappedBool = TypeWrapper[struct {
				Data bool
			}, bool]
			return WrappedBool{
				Data: true,
			}
		}(),
		fromHex("0x01"),
	},
}

func TestMarshal(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for idx, test := range marshalTestMatrix {
		buf, err := dynssz.MarshalSSZ(test.payload)

		switch {
		case test.expected == nil && err != nil:
			// expected error
		case err != nil:
			t.Errorf("test %v error: %v", idx, err)
		case !bytes.Equal(buf, test.expected):
			t.Errorf("test %v failed: got 0x%x, wanted 0x%x", idx, buf, test.expected)
		}
	}
}

func TestMarshalTo(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for idx, test := range marshalTestMatrix {
		size, err := dynssz.SizeSSZ(test.payload)
		if err != nil {
			t.Errorf("test %v error: %v", idx, err)
		}

		buf := make([]byte, 0, size)
		buf, err = dynssz.MarshalSSZTo(test.payload, buf)

		switch {
		case test.expected == nil && err != nil:
			// expected error
		case err != nil:
			t.Errorf("test %v error: %v", idx, err)
		case !bytes.Equal(buf, test.expected):
			t.Errorf("test %v failed: got 0x%x, wanted 0x%x", idx, buf, test.expected)
		}
	}
}

func TestStringVsByteContainerMarshalEquivalence(t *testing.T) {
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
		})
	}
}

func TestFixedSizeStringVsByteArrayMarshal(t *testing.T) {
	type WithFixedString struct {
		Data string `ssz-size:"32"`
		ID   uint32
	}

	type WithByteArray struct {
		Data [32]byte
		ID   uint32
	}

	dynssz := NewDynSsz(nil)

	testCases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"short", "hello"},
		{"exact_32", "abcdefghijklmnopqrstuvwxyz123456"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var byteData [32]byte
			copy(byteData[:], []byte(tc.value))

			strStruct := WithFixedString{
				Data: tc.value,
				ID:   42,
			}

			byteStruct := WithByteArray{
				Data: byteData,
				ID:   42,
			}

			strData, err := dynssz.MarshalSSZ(strStruct)
			if err != nil {
				t.Fatalf("Failed to marshal string struct: %v", err)
			}

			byteStructData, err := dynssz.MarshalSSZ(byteStruct)
			if err != nil {
				t.Fatalf("Failed to marshal byte struct: %v", err)
			}

			if !bytes.Equal(strData, byteStructData) {
				t.Errorf("Marshaled data mismatch:\nString: %x\nBytes:  %x", strData, byteStructData)
			}
		})
	}
}

func TestMarshalErrors(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

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
			name: "vector_too_big",
			input: struct {
				Data []uint8 `ssz-size:"5"`
			}{[]uint8{1, 2, 3, 4, 5, 6}},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "vector_too_big_nested",
			input: struct {
				Data []*slug_StaticStruct1 `ssz-size:"3"`
			}{[]*slug_StaticStruct1{nil, nil, nil, nil}},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "type_wrapper_missing_data",
			input: struct {
				TypeWrapper struct{} `ssz-type:"wrapper"`
			}{},
			expectedErr: "method not found on type",
		},

		{
			name: "invalid_uint128_size",
			input: struct {
				Value []byte `ssz-type:"uint128" ssz-size:"15"`
			}{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "invalid_uint256_size",
			input: struct {
				Value []byte `ssz-type:"uint256" ssz-size:"31"`
			}{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}},
			expectedErr: "list length is higher than max value",
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
			expectedErr: "bitlist ssz type can only be represented by byte slices or arrays, got uint64",
		},
		{
			name: "string_too_long_fixed",
			input: struct {
				Data string `ssz-size:"5"`
			}{"hello world"},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "nested_container_field_error",
			input: struct {
				Inner struct {
					Data []uint32 `ssz-size:"2"`
				}
			}{struct {
				Data []uint32 `ssz-size:"2"`
			}{[]uint32{1, 2, 3}}},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "dynamic_container_field_error",
			input: struct {
				Static  uint32
				Dynamic []struct {
					Data []uint8 `ssz-size:"3"`
				} `ssz-max:"10"`
			}{
				Static: 42,
				Dynamic: []struct {
					Data []uint8 `ssz-size:"3"`
				}{{[]uint8{1, 2, 3, 4}}},
			},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "vector_element_marshal_error",
			input: struct {
				Data [3]struct {
					Inner complex64
				}
			}{[3]struct {
				Inner complex64
			}{{complex64(1)}, {complex64(2)}, {complex64(3)}}},
			expectedErr: "complex numbers are not supported in SSZ",
		},
		{
			name: "dynamic_vector_element_marshal_error",
			input: struct {
				Data []struct {
					Inner complex128
				} `ssz-max:"10"`
			}{[]struct {
				Inner complex128
			}{{complex128(1)}, {complex128(2)}}},
			expectedErr: "complex numbers are not supported in SSZ",
		},
		{
			name: "list_element_marshal_error",
			input: struct {
				Data []struct {
					Value func()
				} `ssz-max:"10"`
			}{[]struct {
				Value func()
			}{{nil}, {nil}}},
			expectedErr: "functions are not supported in SSZ",
		},
		{
			name: "multi_dimensional_size_mismatch",
			input: struct {
				Data [2][]*slug_StaticStruct1 `ssz-size:"2,3"`
			}{[2][]*slug_StaticStruct1{{nil, nil, nil}, {nil, nil, nil, nil}}},
			expectedErr: "list length is higher than max value",
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dynssz.MarshalSSZ(tc.input)
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", tc.expectedErr)
			} else if !contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", tc.expectedErr, err)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
