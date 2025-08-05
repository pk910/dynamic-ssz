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
