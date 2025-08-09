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

	// ssz-type annotation tests
	{
		struct {
			BitlistData []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{[]byte{0x0f, 0x01}}, // bitlist with 4 bits set, length indicator
		fromHex("0x040000000f01"),
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
