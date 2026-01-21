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

var marshalTestMatrix = append(commonTestMatrix, []struct {
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
}...)

func TestMarshal(t *testing.T) {
	dynssz := NewDynSsz(nil)

	for _, test := range marshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			buf, err := dynssz.MarshalSSZ(test.payload)

			switch {
			case test.ssz == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", test.name, err)
			case !bytes.Equal(buf, test.ssz):
				t.Errorf("test %v failed: got 0x%x, wanted 0x%x", test.name, buf, test.ssz)
			}
		})
	}
}

func TestMarshalTo(t *testing.T) {
	dynssz := NewDynSsz(nil)

	for _, test := range marshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			size, err := dynssz.SizeSSZ(test.payload)
			if err != nil {
				t.Errorf("test %v error: %v", test.name, err)
			}

			buf := make([]byte, 0, size)
			buf, err = dynssz.MarshalSSZTo(test.payload, buf)

			switch {
			case test.ssz == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", test.name, err)
			case !bytes.Equal(buf, test.ssz):
				t.Errorf("test %v failed: got 0x%x, wanted 0x%x", test.name, buf, test.ssz)
			}
		})
	}
}

func TestMarshalNoFastSsz(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	for _, test := range marshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			buf, err := dynssz.MarshalSSZ(test.payload)

			switch {
			case test.ssz == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", test.name, err)
			case !bytes.Equal(buf, test.ssz):
				t.Errorf("test %v failed: got 0x%x, wanted 0x%x", test.name, buf, test.ssz)
			}
		})
	}
}

func TestMarshalWriter(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	for _, test := range marshalTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			memWriter := bytes.NewBuffer(nil)

			err := dynssz.MarshalSSZWriter(test.payload, memWriter)

			switch {
			case test.ssz == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", test.name, err)
			case !bytes.Equal(memWriter.Bytes(), test.ssz):
				t.Errorf("test %v failed: got 0x%x, wanted 0x%x", test.name, memWriter.Bytes(), test.ssz)
			}
		})
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
			name: "invalid_bitvector_padding",
			input: struct {
				Flags []byte `ssz-type:"bitvector" ssz-bitsize:"12"`
			}{[]byte{0xff, 0x1f}},
			expectedErr: "bitvector padding bits are not zero",
		},
		{
			name: "invalid_bitlist_type",
			input: struct {
				Bits []uint64 `ssz-type:"bitlist"`
			}{[]uint64{0xff, 0xff}},
			expectedErr: "bitlist ssz type can only be represented by byte slices, got []uint64",
		},
		{
			name: "unterminated_bitlist",
			input: struct {
				Bits []byte `ssz-type:"bitlist"`
			}{[]byte{0x00}},
			expectedErr: "bitlist misses mandatory termination bit",
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
		{
			name: "fastssz_marshal_error",
			input: struct {
				F1 *TestContainerWithMarshalError
			}{},
			expectedErr: "test MarshalSSZTo error",
		},
		{
			name: "dynssz_marshal_error",
			input: struct {
				F1 *TestContainerWithDynamicMarshalError
			}{},
			expectedErr: "test MarshalSSZDyn error",
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
		{
			name: "invalid_union_variant_in_list",
			input: struct {
				Field0 uint16
				Field1 []CompatibleUnion[struct {
					Field1 uint32
				}]
			}{
				0x1234,
				[]CompatibleUnion[struct {
					Field1 uint32
				}]{{Variant: 99, Data: uint32(42)}}, // Invalid variant
			},
			expectedErr: "invalid union variant",
		},
		{
			name: "invalid_union_variant_in_vector",
			input: struct {
				Field0 uint16
				Field1 [2]CompatibleUnion[struct {
					Field1 uint32
				}]
			}{
				0x1234,
				[2]CompatibleUnion[struct {
					Field1 uint32
				}]{{Variant: 99, Data: uint32(42)}}, // Invalid variant
			},
			expectedErr: "invalid union variant",
		},
		{
			name: "invalid_union_variant_in_type_wrapper",
			input: struct {
				Field0 uint16
				Field1 TypeWrapper[struct {
					Field1 CompatibleUnion[struct {
						Field1 uint32
					}]
				}, CompatibleUnion[struct {
					Field1 uint32
				}]]
			}{
				0x1234,
				TypeWrapper[struct {
					Field1 CompatibleUnion[struct {
						Field1 uint32
					}]
				}, CompatibleUnion[struct {
					Field1 uint32
				}]]{
					Data: CompatibleUnion[struct {
						Field1 uint32
					}]{Variant: 99, Data: uint32(42)},
				}, // Invalid variant
			},
			expectedErr: "invalid union variant",
		},
		{
			name: "invalid_union_variant_in_union",
			input: struct {
				Field0 uint16
				Field1 CompatibleUnion[struct {
					Field CompatibleUnion[struct {
						Field1 uint32
					}]
				}]
			}{
				0x1234,
				CompatibleUnion[struct {
					Field CompatibleUnion[struct {
						Field1 uint32
					}]
				}]{Variant: 1, Data: CompatibleUnion[struct {
					Field1 uint32
				}]{Variant: 99, Data: uint32(42)}}, // Invalid variant
			},
			expectedErr: "invalid union variant",
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

			_, err = dynssz.MarshalSSZTo(tc.input, make([]byte, 0, 100))
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", tc.expectedErr)
			} else if !contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestMarshalVerbose(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz(), WithVerbose(), WithLogCb(func(format string, args ...any) {}))

	// Test with various types to exercise verbose logging paths
	testCases := []struct {
		name  string
		input any
	}{
		{"simple_struct", struct {
			Field0 uint64
			Field1 uint32
		}{123, 456}},
		{"progressive_container", struct {
			Field0 uint64 `ssz-index:"0"`
			Field1 uint32 `ssz-index:"1"`
		}{123, 456}},
		{"vector", struct {
			Data [3]uint32
		}{[3]uint32{1, 2, 3}}},
		{"type_wrapper", func() any {
			type W = TypeWrapper[struct {
				Data uint32
			}, uint32]
			return W{Data: 42}
		}()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dynssz.MarshalSSZ(tc.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestMarshalEmptyBitlist(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test empty bitlist - should add termination bit automatically
	// Use MarshalSSZTo to test the actual marshalBitlist function
	input := struct {
		Bits []byte `ssz-type:"bitlist" ssz-max:"100"`
	}{[]byte{}} // Empty bitlist

	// Pre-allocate buffer with known size to avoid size calculation issues
	buf, err := dynssz.MarshalSSZTo(input, make([]byte, 0, 100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should add termination bit 0x01 after the offset
	if len(buf) < 5 || buf[4] != 0x01 {
		t.Errorf("expected empty bitlist to have termination bit, got %x", buf)
	}
}

func TestMarshalDynamicVectorSizeError(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test dynamic vector exceeding size limit
	input := struct {
		Data []struct {
			Value []uint8 `ssz-max:"10"`
		} `ssz-size:"2"`
	}{[]struct {
		Value []uint8 `ssz-max:"10"`
	}{{[]uint8{1}}, {[]uint8{2}}, {[]uint8{3}}}} // 3 elements when max is 2

	_, err := dynssz.MarshalSSZ(input)
	if err == nil {
		t.Error("expected error for dynamic vector exceeding size")
	}
}

func TestMarshalInvalidSizeError(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	type Uint32WithInvalidSize uint32
	uint32desc, err := dynssz.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(Uint32WithInvalidSize(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}
	uint32desc.Size = 8

	// Test dynamic vector exceeding size limit
	input := struct {
		Data Uint32WithInvalidSize
	}{Uint32WithInvalidSize(42)}

	_, err = dynssz.MarshalSSZ(input)
	if err == nil {
		t.Error("ssz length does not match expected length")
	}
}

func TestMarshalDynamicVectorAppendZeroPointer(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test dynamic vector with pointer elements needing zero padding
	input := struct {
		Data []*slug_DynStruct1 `ssz-size:"3"`
	}{[]*slug_DynStruct1{{true, []uint8{1}}}} // 1 element when size is 3

	buf, err := dynssz.MarshalSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buf) == 0 {
		t.Error("buffer should not be empty")
	}
}

func TestMarshalListNilPointerElement(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test list with nil pointer element
	input := struct {
		Data []*slug_StaticStruct1 `ssz-max:"10"`
	}{[]*slug_StaticStruct1{nil, {true, []uint8{1, 2, 3}}, nil}}

	buf, err := dynssz.MarshalSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buf) == 0 {
		t.Error("buffer should not be empty")
	}
}

func TestSizeSSZUint128(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for uint128
	input := struct {
		Value [16]byte `ssz-type:"uint128"`
	}{[16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 16 {
		t.Errorf("expected size 16, got %d", size)
	}
}

func TestSizeSSZUint256(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for uint256
	input := struct {
		Value [32]byte `ssz-type:"uint256"`
	}{[32]byte{}}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 32 {
		t.Errorf("expected size 32, got %d", size)
	}
}

func TestSizeSSZFastSszPath(t *testing.T) {
	dynssz := NewDynSsz(nil)
	// NoFastSsz = false to use FastSSZ path

	// Test with a type that implements FastSSZ and returns size
	input := &TestContainerWithFastSsz{
		Field0: 123,
		Field1: 456,
		Field2: true,
		Field3: 789,
	}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 15 {
		t.Errorf("expected size 15, got %d", size)
	}
}

func TestSizeSSZDynamicSizerPath(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test with a type that implements DynamicSizer
	input := &TestContainerWithDynamicSsz{
		Field0: 123,
		Field1: 456,
		Field2: true,
		Field3: 789,
	}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 15 {
		t.Errorf("expected size 15, got %d", size)
	}
}

func TestSizeSSZTypeWrapper(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for TypeWrapper
	type WrappedUint32List = TypeWrapper[struct {
		Data []uint32 `ssz-max:"1024"`
	}, []uint32]

	input := struct {
		Field WrappedUint32List
	}{
		WrappedUint32List{Data: []uint32{1, 2, 3, 4, 5}},
	}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4 bytes offset + 5 * 4 bytes = 24
	if size != 24 {
		t.Errorf("expected size 24, got %d", size)
	}
}

func TestSizeSSZVectorShortLength(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for vector with elements shorter than declared size
	// This tests the appendZero path
	input := struct {
		Data []uint32 `ssz-size:"5"`
	}{[]uint32{1, 2, 3}} // Only 3 elements but declared as 5

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 5 * 4 bytes = 20
	if size != 20 {
		t.Errorf("expected size 20, got %d", size)
	}
}

func TestSizeSSZVectorStaticElements(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for vector with static elements (dataLen > 0)
	input := struct {
		Data [5]uint32
	}{[5]uint32{1, 2, 3, 4, 5}}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 5 * 4 bytes = 20
	if size != 20 {
		t.Errorf("expected size 20, got %d", size)
	}
}

func TestSizeSSZVectorEmptyStaticElements(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for empty vector with static elements (dataLen = 0)
	input := struct {
		Data []uint32 `ssz-size:"5"`
	}{[]uint32{}} // Empty slice but declared as vector of 5

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 5 * 4 bytes = 20
	if size != 20 {
		t.Errorf("expected size 20, got %d", size)
	}
}

func TestSizeSSZVectorDynamicElements(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for vector with dynamic elements
	input := struct {
		Data [][]uint32 `ssz-size:"3" ssz-max:"?,10"`
	}{[][]uint32{{1, 2}, {3, 4, 5}}} // 2 elements but declared as 3

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Container offset (4) + 3 * 4 bytes (offsets) + 2*4 bytes + 3*4 bytes + 0 bytes = 4 + 12 + 8 + 12 + 0 = 36
	if size != 36 {
		t.Errorf("expected size 36, got %d", size)
	}
}

func TestSizeSSZListDynamicElements(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for list with dynamic elements
	input := struct {
		Data [][]uint32 `ssz-max:"10,10"`
	}{[][]uint32{{1, 2}, {3, 4, 5}, {6}}}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Container offset (4) + 3 * 4 bytes (offsets) + 2*4 bytes + 3*4 bytes + 1*4 bytes = 4 + 12 + 8 + 12 + 4 = 40
	if size != 40 {
		t.Errorf("expected size 40, got %d", size)
	}
}

func TestSizeSSZListStaticElements(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for list with static elements
	input := struct {
		Data []uint64 `ssz-max:"10"`
	}{[]uint64{1, 2, 3}}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Container offset (4) + 3 * 8 bytes = 4 + 24 = 28
	if size != 28 {
		t.Errorf("expected size 28, got %d", size)
	}
}

func TestSizeSSZCompatibleUnion(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test size calculation for compatible union
	type TestUnion = CompatibleUnion[struct {
		Field1 uint32
		Field2 [8]uint8
	}]

	input := struct {
		Field TestUnion
	}{
		TestUnion{Variant: 0, Data: uint32(42)},
	}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4 bytes offset + 1 byte selector + 4 bytes uint32 = 9
	if size != 9 {
		t.Errorf("expected size 9, got %d", size)
	}
}

func TestSizeSSZFastSszFallback(t *testing.T) {
	dynssz := NewDynSsz(nil)
	// Don't set NoFastSsz = true, but manually set CompatFlags to trigger fallback

	// Clear any cached types
	dynssz.GetTypeCache().RemoveAllTypes()

	// Set compat flag for a type that doesn't actually implement FastSSZ
	dynssz.GetTypeCache().CompatFlags["struct { Field0 uint64 }"] = ssztypes.SszCompatFlagFastSSZMarshaler

	// Test with type that has CompatFlag but doesn't implement the interface
	input := struct {
		Field0 uint64
	}{123}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fallback to manual calculation: 8 bytes
	if size != 8 {
		t.Errorf("expected size 8, got %d", size)
	}
}

func TestSizeSSZDynamicSizerFallback(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Clear any cached types
	dynssz.GetTypeCache().RemoveAllTypes()

	// Set compat flag for DynamicSizer on a type that doesn't implement it
	dynssz.GetTypeCache().CompatFlags["struct { Field0 uint32 }"] = ssztypes.SszCompatFlagDynamicSizer

	// Test with type that has CompatFlag but doesn't implement the interface
	input := struct {
		Field0 uint32
	}{123}

	size, err := dynssz.SizeSSZ(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fallback to manual calculation: 4 bytes
	if size != 4 {
		t.Errorf("expected size 4, got %d", size)
	}
}

func TestMarshalEmptyDynamicList(t *testing.T) {
	dynssz := NewDynSsz(nil, WithNoFastSsz())

	// Test empty dynamic list - buffer vs streaming should produce identical output
	// This tests the fix for the bug where non-seekable encoder incorrectly wrote
	// an offset for empty dynamic lists
	type DynamicElement struct {
		Value []uint8 `ssz-max:"10"`
	}

	input := struct {
		Static  uint32
		Dynamic []DynamicElement `ssz-max:"10"`
	}{
		Static:  42,
		Dynamic: []DynamicElement{}, // Empty dynamic list
	}

	// Marshal using buffer (seekable)
	bufResult, err := dynssz.MarshalSSZ(input)
	if err != nil {
		t.Fatalf("MarshalSSZ error: %v", err)
	}

	// Marshal using writer (non-seekable streaming)
	memWriter := bytes.NewBuffer(nil)
	err = dynssz.MarshalSSZWriter(input, memWriter)
	if err != nil {
		t.Fatalf("MarshalSSZWriter error: %v", err)
	}

	// Both methods must produce identical output
	if !bytes.Equal(bufResult, memWriter.Bytes()) {
		t.Errorf("Empty dynamic list encoding mismatch:\n  Buffer (seekable):  %x\n  Writer (streaming): %x",
			bufResult, memWriter.Bytes())
	}

	// Verify expected SSZ format: 4 bytes (Static) + 4 bytes (offset) = 8 bytes
	// The offset should point to position 8, where the empty list starts (and ends)
	expectedSSZ := []byte{
		0x2a, 0x00, 0x00, 0x00, // Static = 42
		0x08, 0x00, 0x00, 0x00, // Offset = 8 (pointing to end of fixed part)
	}
	if !bytes.Equal(bufResult, expectedSSZ) {
		t.Errorf("Expected SSZ: %x, got: %x", expectedSSZ, bufResult)
	}
}

func TestCustomFallbackMarshal(t *testing.T) {
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

	_, err = dynssz.MarshalSSZ(&TestContainer{})
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestViewMarshaler(t *testing.T) {
	// Test DynamicViewMarshaler interface via MarshalSSZWriter (non-seekable encoder)
	// The non-seekable path calls MarshalSSZDynView
	// Expected SSZ: uint64(123) + uint32(456) = 8 + 4 = 12 bytes
	// Little endian: 0x7b00000000000000 + 0xc8010000
	expectedSSZ := fromHex("0x7b00000000000000c8010000")

	testCases := []struct {
		name        string
		view        any
		expectError string
	}{
		{
			name: "ViewMarshaler_Success",
			view: (*TestViewType1)(nil),
		},
		{
			name:        "ViewMarshaler_Error",
			view:        (*TestViewType2)(nil),
			expectError: "test view marshaler error",
		},
		{
			name: "ViewMarshaler_NoCodeForView_FallbackToReflection",
			view: (*TestViewTypeUnknown)(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds := NewDynSsz(nil)
			container := TestContainerWithViewMarshaler{Field0: 123, Field1: 456}
			var buf bytes.Buffer
			err := ds.MarshalSSZWriter(&container, &buf, WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Errorf("expected error containing '%s', but got no error", tc.expectError)
				} else if !contains(err.Error(), tc.expectError) {
					t.Errorf("expected error containing '%s', but got: %v", tc.expectError, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				} else if !bytes.Equal(buf.Bytes(), expectedSSZ) {
					t.Errorf("expected SSZ %x, got %x", expectedSSZ, buf.Bytes())
				}
			}
		})
	}
}

func TestViewEncoder(t *testing.T) {
	// Test DynamicViewEncoder interface via MarshalSSZ (seekable encoder)
	// The seekable path calls MarshalSSZEncoderView first
	// Expected SSZ: uint64(123) + uint32(456) = 8 + 4 = 12 bytes
	// Little endian: 0x7b00000000000000 + 0xc8010000
	expectedSSZ := fromHex("0x7b00000000000000c8010000")

	testCases := []struct {
		name        string
		view        any
		expectError string
	}{
		{
			name: "ViewEncoder_Success",
			view: (*TestViewType1)(nil),
		},
		{
			name:        "ViewEncoder_Error",
			view:        (*TestViewType2)(nil),
			expectError: "test view encoder error",
		},
		{
			name: "ViewEncoder_NoCodeForView_FallbackToReflection",
			view: (*TestViewTypeUnknown)(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds := NewDynSsz(nil)
			container := TestContainerWithViewMarshaler{Field0: 123, Field1: 456}
			data, err := ds.MarshalSSZ(&container, WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Errorf("expected error containing '%s', but got no error", tc.expectError)
				} else if !contains(err.Error(), tc.expectError) {
					t.Errorf("expected error containing '%s', but got: %v", tc.expectError, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				} else if !bytes.Equal(data, expectedSSZ) {
					t.Errorf("expected SSZ %x, got %x", expectedSSZ, data)
				}
			}
		})
	}
}
