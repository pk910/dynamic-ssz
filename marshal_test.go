// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"bytes"
	"reflect"
	"testing"
)

type slug_DynStruct1 struct {
	f1 bool
	f2 []uint8
}

type slug_StaticStruct1 struct {
	f1 bool
	f2 []uint8 `ssz-size:"3"`
}

func TestMarshal(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	testMatrix := []struct {
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
		{[]uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405")},

		// complex types
		{
			struct {
				f1 bool
				f2 uint8
				f3 uint16
				f4 uint32
				f5 uint64
			}{true, 1, 2, 3, 4},
			fromHex("0x01010200030000000400000000000000"),
		},
		{
			struct {
				f1 bool
				f2 []uint8  // dynamic field
				f3 []uint16 `ssz-size:"5"` // static field due to tag
				f4 uint32
			}{true, []uint8{1, 1, 1, 1}, []uint16{2, 2, 2, 2}, 3},
			fromHex("0x0113000000020002000200020000000300000001010101"),
		},
		{
			struct {
				f1 uint8
				f2 [][]uint8 `ssz-size:"?,2"`
				f3 uint8
			}{42, [][]uint8{{2, 2}, {3}}, 43},
			fromHex("0x2a060000002b02020300"),
		},
		{
			struct {
				f1 uint8
				f2 []slug_DynStruct1 `ssz-size:"3"`
				f3 uint8
			}{42, []slug_DynStruct1{{true, []uint8{4}}, {true, []uint8{4, 8, 4}}}, 43},
			fromHex("0x2a060000002b0c000000120000001a00000001050000000401050000000408040005000000"),
		},
		{
			struct {
				f1 uint8
				f2 []*slug_StaticStruct1 `ssz-size:"3"`
				f3 uint8
			}{42, []*slug_StaticStruct1{nil, {true, []uint8{4, 8, 4}}}, 43},
			fromHex("0x2a0000000001040804000000002b"),
		},
		{
			struct {
				f1 uint8
				f2 []*slug_StaticStruct1 `ssz-size:"3"`
				f3 uint8
			}{42, []*slug_StaticStruct1{nil, nil, nil, nil}, 43},
			nil, // size too long error
		},
	}

	for idx, test := range testMatrix {
		var err error
		buf := []byte{}
		buf, err = dynssz.marshalType(reflect.TypeOf(test.payload), reflect.ValueOf(test.payload), buf, nil, 0)

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
