// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz_test

import (
	"bytes"
	"testing"

	. "github.com/pk910/dynamic-ssz"
)

var treerootTestMatrix = []struct {
	payload  any
	expected []byte
}{
	// primitive types
	{bool(false), fromHex("0x0000000000000000000000000000000000000000000000000000000000000000")},
	{bool(true), fromHex("0x0100000000000000000000000000000000000000000000000000000000000000")},
	{uint8(0), fromHex("0x0000000000000000000000000000000000000000000000000000000000000000")},
	{uint8(255), fromHex("0xff00000000000000000000000000000000000000000000000000000000000000")},
	{uint8(42), fromHex("0x2a00000000000000000000000000000000000000000000000000000000000000")},
	{uint16(0), fromHex("0x0000000000000000000000000000000000000000000000000000000000000000")},
	{uint16(65535), fromHex("0xffff000000000000000000000000000000000000000000000000000000000000")},
	{uint16(1337), fromHex("0x3905000000000000000000000000000000000000000000000000000000000000")},
	{uint32(0), fromHex("0x0000000000000000000000000000000000000000000000000000000000000000")},
	{uint32(4294967295), fromHex("0xffffffff00000000000000000000000000000000000000000000000000000000")},
	{uint32(817482215), fromHex("0xe7c9b93000000000000000000000000000000000000000000000000000000000")},
	{uint64(0), fromHex("0x0000000000000000000000000000000000000000000000000000000000000000")},
	{uint64(18446744073709551615), fromHex("0xffffffffffffffff000000000000000000000000000000000000000000000000")},
	{uint64(848028848028), fromHex("0x9c4f7572c5000000000000000000000000000000000000000000000000000000")},

	// arrays & slices
	{[]uint8{}, fromHex("0x0000000000000000000000000000000000000000000000000000000000000000")},
	{[]uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405000000000000000000000000000000000000000000000000000000")},
	{[5]uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405000000000000000000000000000000000000000000000000000000")},
	{[10]uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405000000000000000000000000000000000000000000000000000000")},

	// complex types
	{
		struct {
			F1 bool
			F2 uint8
			F3 uint16
			F4 uint32
			F5 uint64
		}{true, 1, 2, 3, 4},
		fromHex("0x03cf6524e0c5dee777f18d8a15b724aa70da9d9393e3a47434fe352eff0e7375"),
	},
	{
		struct {
			F1 bool
			F2 []uint8  `ssz-max:"10"` // dynamic field
			F3 []uint16 `ssz-size:"5"` // static field due to tag
			F4 uint32
		}{true, []uint8{1, 1, 1, 1}, []uint16{2, 2, 2, 2}, 3},
		fromHex("0xdf5e70a7c5c545445d8adced00e0d531b2101fcb5e43158621a256434423ded9"),
	},
	{
		struct {
			F1 uint8
			F2 [][]uint8 `ssz-size:"?,2" ssz-max:"10"`
			F3 uint8
		}{42, [][]uint8{{2, 2}, {3}}, 43},
		fromHex("0xf49f73d6aa7e15c5d26bea0830d9f342be22b7f4d4683391059f20e3dbce4b0a"),
	},
	{
		struct {
			F1 uint8
			F2 []slug_DynStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []slug_DynStruct1{{true, []uint8{4}}, {true, []uint8{4, 8, 4}}}, 43},
		fromHex("0xeb722b1df677b9949255b1e9aefddde783d6fac52dbc0a28e788d6a9306be7fd"),
	},
	{
		struct {
			F1 uint8
			F2 []*slug_StaticStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []*slug_StaticStruct1{nil, {true, []uint8{4, 8, 4}}}, 43},
		fromHex("0xd0816b4909b1eb8345e88fdf833ec5ec545b4d8e46ea6c71ee5c9fa93256275d"),
	},
	{
		struct {
			F1 uint8
			F2 [][]struct {
				F1 uint16
			} `ssz-size:"?,2" ssz-max:"10"`
			F3 uint8
		}{42, [][]struct {
			F1 uint16
		}{{{F1: 2}, {F1: 3}}, {{F1: 4}, {F1: 5}}}, 43},
		fromHex("0xc7b4839f561b9eed7da50de309ddb8bcde2a33a61a259b7377164251df4eac3c"),
	},
	{
		struct {
			F1 uint8
			F2 [][2][]struct {
				F1 uint16
			} `ssz-size:"?,2,?" ssz-max:"10,?,10"`
			F3 uint8
		}{42, [][2][]struct {
			F1 uint16
		}{{{{F1: 2}, {F1: 3}}, {{F1: 4}, {F1: 5}}}, {{{F1: 8}, {F1: 9}}, {{F1: 10}, {F1: 11}}}}, 43},
		fromHex("0x7d0b409af96c93a86b93503d0b53bdc1b90426224da00d610568c71d4a2d3e02"),
	},
	{
		struct {
			F1 uint8
			F2 [][2][]struct {
				F1 uint16
			} `ssz-size:"?,2,?" ssz-max:"10,?,?"`
			F3 uint8
		}{42, [][2][]struct {
			F1 uint16
		}{{{{F1: 2}, {F1: 3}}, {{F1: 4}, {F1: 5}}}, {{{F1: 8}, {F1: 9}}, {{F1: 10}, {F1: 11}}}}, 43},
		fromHex("0x031d9f2e588f41ecc10851cef557fd52c25414e44ff5fd0e8289c5a3c9efeaaf"),
	},
	{
		struct {
			F1 [][]uint16 `ssz-size:"?,2" ssz-max:"10"`
		}{[][]uint16{{2, 3}, {4, 5}, {8, 9}, {10, 11}}},
		fromHex("0xc32aa2ec48fe03bd08bb351ae8c370ba19dc7e20551e5ca56c0730de5651293d"),
	},
}

func TestTreeRoot(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for idx, test := range treerootTestMatrix {
		buf, err := dynssz.HashTreeRoot(test.payload)

		switch {
		case test.expected == nil && err != nil:
			// expected error
		case err != nil:
			t.Errorf("test %v error: %v", idx, err)
		case !bytes.Equal(buf[:], test.expected):
			t.Errorf("test %v failed: got 0x%x, wanted 0x%x", idx, buf, test.expected)
		}
	}
}
