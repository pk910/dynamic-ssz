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

	// stable containers
	{
		struct {
			F1 uint8 `ssz-container:"stable-container,8"`
			F2 slug_StaticStruct1
			F3 *slug_StaticStruct1
		}{
			42,
			slug_StaticStruct1{true, []uint8{1, 33, 7}},
			nil,
		},
		fromHex("0x032a01012107"),
	},
	{
		struct {
			F1 uint8 `ssz-container:"stable-container,16"`
			F2 slug_StaticStruct1
			F3 *slug_StaticStruct1
			F4 *slug_DynStruct1
			F5 *slug_DynStruct1
		}{
			42,
			slug_StaticStruct1{true, []uint8{1, 33, 7}},
			nil,
			&slug_DynStruct1{true, []uint8{4, 8, 4}},
			nil,
		},
		fromHex("0x0b002a01012107090000000105000000040804"),
	},
	{
		struct {
			F1 slug_StableStaticStruct1
			F2 slug_StableStaticStruct1
		}{
			slug_StableStaticStruct1{&trueValue, &testUint8Arr, nil},
			slug_StableStaticStruct1{nil, &testUint8Arr, &testUint16},
		},
		fromHex("0x080000000e00000003000101020306000102033713"),
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
