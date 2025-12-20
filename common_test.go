// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz_test

import (
	"fmt"
	"io"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var commonTestMatrix = []struct {
	name    string
	payload any
	ssz     []byte
	htr     []byte
}{
	// primitive types
	{
		"bool_false",
		bool(false),
		fromHex("0x00"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"bool_true",
		bool(true),
		fromHex("0x01"),
		fromHex("0x0100000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint8_min",
		uint8(0),
		fromHex("0x00"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint8_max",
		uint8(255),
		fromHex("0xff"),
		fromHex("0xff00000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint8_val1",
		uint8(42),
		fromHex("0x2a"),
		fromHex("0x2a00000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint16_min",
		uint16(0),
		fromHex("0x0000"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint16_max",
		uint16(65535),
		fromHex("0xffff"),
		fromHex("0xffff000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint16_val1",
		uint16(1337),
		fromHex("0x3905"),
		fromHex("0x3905000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint32_min",
		uint32(0),
		fromHex("0x00000000"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint32_max",
		uint32(4294967295),
		fromHex("0xffffffff"),
		fromHex("0xffffffff00000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint32_val1",
		uint32(817482215),
		fromHex("0xe7c9b930"),
		fromHex("0xe7c9b93000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint64_min",
		uint64(0),
		fromHex("0x0000000000000000"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"uint64_max",
		uint64(18446744073709551615),
		fromHex("0xffffffffffffffff"),
		fromHex("0xffffffffffffffff000000000000000000000000000000000000000000000000"),
	},
	{
		"uint64_val1",
		uint64(848028848028),
		fromHex("0x9c4f7572c5000000"),
		fromHex("0x9c4f7572c5000000000000000000000000000000000000000000000000000000"),
	},

	// arrays & slices
	{
		"array_empty",
		[]uint8{},
		fromHex("0x"),
		fromHex("0x0000000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"array_val1",
		[]uint8{1, 2, 3, 4, 5},
		fromHex("0x0102030405"),
		fromHex("0x0102030405000000000000000000000000000000000000000000000000000000"),
	},
	{
		"array_val2",
		[5]uint8{1, 2, 3, 4, 5},
		fromHex("0x0102030405"),
		fromHex("0x0102030405000000000000000000000000000000000000000000000000000000"),
	},
	{
		"array_val3",
		[10]uint8{1, 2, 3, 4, 5},
		fromHex("0x01020304050000000000"),
		fromHex("0x0102030405000000000000000000000000000000000000000000000000000000"),
	},

	// complex types
	{
		"complex_struct1",
		struct {
			F1 bool
			F2 uint8
			F3 uint16
			F4 uint32
			F5 uint64
		}{true, 1, 2, 3, 4},
		fromHex("0x01010200030000000400000000000000"),
		fromHex("0x03cf6524e0c5dee777f18d8a15b724aa70da9d9393e3a47434fe352eff0e7375"),
	},
	{
		"complex_struct2",
		struct {
			F1 bool
			F2 []uint8  `ssz-max:"10"` // dynamic field
			F3 []uint16 `ssz-size:"5"` // static field due to tag
			F4 uint32
		}{true, []uint8{1, 1, 1, 1}, []uint16{2, 2, 2, 2}, 3},
		fromHex("0x0113000000020002000200020000000300000001010101"),
		fromHex("0xcb141fb9e033499344f568ea05a6a77ada886fc6e856ece01ae5a329e184fbd1"),
	},
	{
		"complex_struct3",
		struct {
			F1 uint8
			F2 [][]uint8 `ssz-size:"?,2" ssz-max:"10"`
			F3 uint8
		}{42, [][]uint8{{2, 2}, {3}}, 43},
		fromHex("0x2a060000002b02020300"),
		fromHex("0xf49f73d6aa7e15c5d26bea0830d9f342be22b7f4d4683391059f20e3dbce4b0a"),
	},
	{
		"complex_struct4",
		struct {
			F1 uint8
			F2 []slug_DynStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []slug_DynStruct1{{true, []uint8{4}}, {true, []uint8{4, 8, 4}}}, 43},
		fromHex("0x2a060000002b0c000000120000001a00000001050000000401050000000408040005000000"),
		fromHex("0x609aed07225400cb21de97260b267aab012358a235d1a1e9fc4df94859208c83"),
	},
	{
		"complex_struct5",
		struct {
			F1 uint8
			F2 []*slug_StaticStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []*slug_StaticStruct1{nil, {true, []uint8{4, 8, 4}}}, 43},
		fromHex("0x2a0000000001040804000000002b"),
		fromHex("0xcb36f82247d205d8fc9dc60d04a245fb588be35315b4c3406ed2b68f69de7eda"),
	},
	{
		"complex_struct7",
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
		fromHex("0xf487ff96faa706a7842188212604b54466e355624e96e3e0aef72e066b38b929"),
	},
	{
		"complex_struct8",
		struct {
			F1 uint8
			F2 [][2][]struct {
				F1 uint16
			} `ssz-size:"?,2,?" ssz-max:"10,?,10"`
			F3 uint8
		}{42, [][2][]struct {
			F1 uint16
		}{{{{F1: 2}, {F1: 3}}, {{F1: 4}, {F1: 5}}}, {{{F1: 8}, {F1: 9}}, {{F1: 10}, {F1: 11}}}}, 43},
		fromHex("0x2a060000002b0800000018000000080000000c0000000200030004000500080000000c000000080009000a000b00"),
		fromHex("0x7d0b409af96c93a86b93503d0b53bdc1b90426224da00d610568c71d4a2d3e02"),
	},
	{
		"complex_struct9",
		struct {
			F1 [][]uint16 `ssz-size:"?,2" ssz-max:"10"`
		}{[][]uint16{{2, 3}, {4, 5}, {8, 9}, {10, 11}}},
		fromHex("0x040000000200030004000500080009000a000b00"),
		fromHex("0x253a3f3ffab684c2d4f4930b7923f31aadc3eff94b3eb8b4b7b9aa1363efcf52"),
	},
	{
		"complex_struct10",
		struct {
			F1 []uint16 `ssz-size:"2"`
		}{[]uint16{2, 3}},
		fromHex("0x02000300"),
		fromHex("0x0200030000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"complex_struct11",
		struct {
			F1 []uint16 `ssz-type:"list" ssz-size:"?"`
		}{[]uint16{2, 3}},
		fromHex("0x0400000002000300"),
		fromHex("0x0200030000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"complex_struct13",
		struct {
			F1 []uint8 `ssz-type:"bitvector" ssz-bitsize:"12"`
		}{[]uint8{0xff, 0x0f}},
		fromHex("0xff0f"),
		fromHex("0xff0f000000000000000000000000000000000000000000000000000000000000"),
	},

	{
		"complex_struct14",
		func() any {
			list := make([]uint32, 128)
			list[0] = 123
			list[1] = 654
			list[127] = 222

			return struct {
				F1 []uint32 `ssz-type:"progressive-list"`
			}{list}
		}(),
		fromHex("0x040000007b0000008e0200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000de000000"),
		fromHex("0xcafb653b8b774afa1a755897c6afc68bb08af48b30a3c08ca5b72ddf79bdb20f"),
	},

	// progressive bitlist test - matches Python test_progressive_bitlist.py output
	{
		"progressive_bitlist_1",
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
		fromHex("0xba990efa7343179a41d01614759e0ab696a8869fade3f576a8abe6e9880eeaa3"),
	},

	// progressive list with 100 uint16 elements
	{
		"progressive_list_2",
		struct {
			F1 []uint16 `ssz-type:"progressive-list"`
		}{[]uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94, 95, 96, 97, 98, 99, 100}},
		fromHex("0x040000000100020003000400050006000700080009000a000b000c000d000e000f0010001100120013001400150016001700180019001a001b001c001d001e001f0020002100220023002400250026002700280029002a002b002c002d002e002f0030003100320033003400350036003700380039003a003b003c003d003e003f0040004100420043004400450046004700480049004a004b004c004d004e004f0050005100520053005400550056005700580059005a005b005c005d005e005f0060006100620063006400"),
		fromHex("0xafc3646489c444662626be91d6630ba5671cb302733bd50822544f8c6be96005"),
	},

	// Progressive container tests
	{
		"progressive_container_1",
		struct {
			Field0 uint64 `ssz-index:"0"`
			Field1 uint32 `ssz-index:"1"`
			Field2 bool   `ssz-index:"2"`
			Field3 uint16 `ssz-index:"3"`
		}{12345, 67890, true, 999},
		fromHex("0x39300000000000003209010001e703"),
		fromHex("0x0e4ca0d5f6b209257cdaa08a60240a3043fb0ab891fa32f5d483d569605bb4df"),
	},
	{
		"progressive_container_2",
		struct {
			Field0 uint64 `ssz-index:"0"`
			Field1 uint32 `ssz-index:"1"`
			Field2 bool   `ssz-index:"2"`
			Field3 uint16 `ssz-index:"3"`
		}{0, 0, false, 0},
		fromHex("0x000000000000000000000000000000"),
		fromHex("0x7e3741b0db51cdff09176571314e17e2e216bf4264841eb3d6aa78c7c435658e"),
	},
	// progressive container with sparse indices
	{
		"progressive_container_3",
		struct {
			Field0 uint64 `ssz-index:"0"`
			Field1 uint32 `ssz-index:"1"`
			Field2 bool   `ssz-index:"4"`
			Field3 uint16 `ssz-index:"5"`
		}{12345, 67890, true, 999},
		fromHex("0x39300000000000003209010001e703"),
		fromHex("0xa022dead859d4c67b19c5caa2cd26b1f004479465133ae8f2decd234f41df8f5"),
	},

	// CompatibleUnion tests
	{
		"compatible_union_1",
		struct {
			Field0 uint16
			Field1 dynssz.CompatibleUnion[struct {
				Field1 uint32
				Field2 [2]uint8
			}]
			Field3 uint16
		}{0x1337, dynssz.CompatibleUnion[struct {
			Field1 uint32
			Field2 [2]uint8
		}]{Variant: 0, Data: uint32(0x12345678)}, 0x4242},
		fromHex("0x37130800000042420078563412"),
		fromHex("0x631276fc281634b5224241dd547762be15e2f54e361c6bdc8f921a4d5125e954"),
	},
	{
		"compatible_union_2",
		struct {
			Field0 uint16
			Field1 dynssz.CompatibleUnion[struct {
				Field1 []uint32
				Field2 [2]uint8
			}] `ssz-type:"compatible-union"`
			Field3 uint16
		}{0x1337, dynssz.CompatibleUnion[struct {
			Field1 []uint32
			Field2 [2]uint8
		}]{Variant: 1, Data: [2]uint8{0x78, 0x56}}, 0x4242},
		fromHex("0x3713080000004242017856"),
		fromHex("0xa667d80855a0a42d447357c8dc753ce188ed7d30daceee9bb7ecc592d729bbeb"),
	},
	{
		"complex_struct15",
		[2]uint16{1, 2},
		fromHex("0x01000200"),
		fromHex("0x0100020000000000000000000000000000000000000000000000000000000000"),
	},

	// ssz-type annotation tests
	{
		"type_annotation_1",
		struct {
			BitlistData []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{[]byte{0x0f, 0x01}}, // bitlist with 4 bits set, length indicator
		fromHex("0x040000000f01"),
		fromHex("0xac0d43079c4f10cade6386f382829a4a00e4d9832cb66a068969c761bce57d96"),
	},
	{
		"pointer_bitlist",
		struct {
			Data *[]byte `ssz-type:"bitlist" ssz-max:"100"`
		}{func() *[]byte { b := []byte{0x0f, 0x01}; return &b }()}, // pointer bitlist
		fromHex("0x040000000f01"),
		fromHex("0xac0d43079c4f10cade6386f382829a4a00e4d9832cb66a068969c761bce57d96"),
	},

	// uint128 type tests
	{
		"uint128_1",
		struct {
			Value [16]byte `ssz-type:"uint128"`
		}{[16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		fromHex("0x0102030405060708090a0b0c0d0e0f10"),
		fromHex("0x0102030405060708090a0b0c0d0e0f1000000000000000000000000000000000"),
	},
	{
		"uint128_2",
		struct {
			Value [2]uint64 `ssz-type:"uint128"`
		}{[2]uint64{0x0807060504030201, 0x100f0e0d0c0b0a09}},
		fromHex("0x0102030405060708090a0b0c0d0e0f10"),
		fromHex("0x0102030405060708090a0b0c0d0e0f1000000000000000000000000000000000"),
	},
	{
		"uint128_3",
		struct {
			Value []byte `ssz-type:"uint128" ssz-size:"16"`
		}{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		fromHex("0x0102030405060708090a0b0c0d0e0f10"),
		fromHex("0x0102030405060708090a0b0c0d0e0f1000000000000000000000000000000000"),
	},

	// uint256 type tests
	{
		"uint256_1",
		struct {
			Balance [32]byte `ssz-type:"uint256"`
		}{[32]byte{
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
			17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
		}},
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
	},
	{
		"uint256_2",
		struct {
			Balance [4]uint64 `ssz-type:"uint256"`
		}{[4]uint64{0x0807060504030201, 0x100f0e0d0c0b0a09, 0x1817161514131211, 0x201f1e1d1c1b1a19}},
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
	},
	{
		"uint256_3",
		struct {
			Balance []byte `ssz-type:"uint256" ssz-size:"32"`
		}{[]byte{
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
			17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
		}},
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
		fromHex("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
	},

	// time.Time type tests
	{
		"time_1",
		struct {
			Time1 time.Time
		}{time.Unix(1718236800, 0)},
		fromHex("0x80366a6600000000"),
		fromHex("0x80366a6600000000000000000000000000000000000000000000000000000000"),
	},

	// bitvector type tests
	{
		"bitvector_1",
		struct {
			Flags [4]byte `ssz-type:"bitvector"`
		}{[4]byte{0xff, 0x0f, 0x00, 0xf0}},
		fromHex("0xff0f00f0"),
		fromHex("0xff0f00f000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"bitvector_2",
		struct {
			Flags [3]byte `ssz-type:"bitvector" ssz-bitsize:"12"`
		}{[3]byte{0xff, 0x0f, 0x00}},
		fromHex("0xff0f"),
		fromHex("0xff0f000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"bitvector_3",
		struct {
			Flags [4]byte `ssz-type:"bitvector" ssz-size:"4"`
		}{[4]byte{0xff, 0x0f, 0x00, 0xf0}},
		fromHex("0xff0f00f0"),
		fromHex("0xff0f00f000000000000000000000000000000000000000000000000000000000"),
	},

	// explicit basic type annotations
	{
		"type_uint32_1",
		struct {
			Value uint32 `ssz-type:"uint32"`
		}{0x12345678},
		fromHex("0x78563412"),
		fromHex("0x7856341200000000000000000000000000000000000000000000000000000000"),
	},
	{
		"type_bool_1",
		struct {
			Value bool `ssz-type:"bool"`
		}{true},
		fromHex("0x01"),
		fromHex("0x0100000000000000000000000000000000000000000000000000000000000000"),
	},

	// vector type annotation
	{
		"type_vector_1",
		struct {
			Values []uint64 `ssz-type:"vector" ssz-size:"3"`
		}{[]uint64{1, 2, 3}},
		fromHex("0x010000000000000002000000000000000300000000000000"),
		fromHex("0x0100000000000000020000000000000003000000000000000000000000000000"),
	},
	{
		"type_vector_2",
		struct {
			Values []struct {
				F1 []uint64
				F2 uint64
			} `ssz-type:"vector" ssz-size:"3"`
		}{},
		fromHex("0x040000000c00000018000000240000000c00000000000000000000000c00000000000000000000000c0000000000000000000000"),
		fromHex("0x8a76fb51c2335e4235aea0146626d464fe6dacad7a68f4efca90806241b3b213"),
	},
	{
		"type_vector_3",
		func() any {
			type TestType = dynssz.TypeWrapper[struct {
				Data []struct {
					F1 []uint64
					F2 uint64
				} `ssz-type:"vector" ssz-size:"3"`
			}, []struct {
				F1 []uint64
				F2 uint64
			}]
			return TestType{
				Data: []struct {
					F1 []uint64
					F2 uint64
				}{},
			}
		}(),
		fromHex("0x0c00000018000000240000000c00000000000000000000000c00000000000000000000000c0000000000000000000000"),
		fromHex("0x8a76fb51c2335e4235aea0146626d464fe6dacad7a68f4efca90806241b3b213"),
	},
	{
		"type_vector_4",
		func() any {
			type TestType = dynssz.TypeWrapper[struct {
				Data []struct {
					F2 uint64
				} `ssz-type:"vector" ssz-size:"3"`
			}, []struct {
				F2 uint64
			}]
			return TestType{
				Data: []struct {
					F2 uint64
				}{},
			}
		}(),
		fromHex("0x000000000000000000000000000000000000000000000000"),
		fromHex("0xdb56114e00fdd4c1f85c892bf35ac9a89289aaecb1ebd0a96cde606a748b5d71"),
	},

	// list with size hint
	{
		"list_with_size1",
		struct {
			F1 []uint16 `ssz-type:"list" ssz-size:"2"`
		}{[]uint16{2, 3}},
		fromHex("0x02000300"),
		fromHex("0x0200030000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"list_with_size2",
		struct {
			F1 [][]uint16 `ssz-type:"list" ssz-size:"2"`
		}{[][]uint16{{2, 3}}},
		fromHex("0x040000000400000002000300"),
		fromHex("0x0200030000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"list_with_size3",
		struct {
			F1 []uint8 `ssz-type:"bitlist" ssz-bitsize:"16"`
		}{[]uint8{0x02, 0x03}},
		fromHex("0x0203"),
		fromHex("0x32cdafa273f9ccca9f53cad6960d5b1e40721b247be996a439925e34531fa248"),
	},

	// container type annotation
	{
		"type_container_1",
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
		fromHex("0x40fb670c297a5c70d0b09f5f39cc5f1a442c79e86d7aaebe34a775c35c84e2e5"),
	},

	// string types
	{
		"string_1",
		struct {
			Data string `ssz-max:"100"`
		}{""},
		fromHex("0x04000000"),
		fromHex("0x28ba1834a3a7b657460ce79fa3a1d909ab8828fd557659d4d0554a9bdbc0ec30"),
	},
	{
		"string_2",
		struct {
			Data string `ssz-max:"100"`
		}{"hello"},
		fromHex("0x0400000068656c6c6f"),
		fromHex("0x19da29a0796bb0ad502164fb6362e551756896856128aa64e415d5304a317b40"),
	},
	{
		"string_3",
		struct {
			Data string `ssz-max:"100"`
		}{"hello 世界"},
		fromHex("0x0400000068656c6c6f20e4b896e7958c"),
		fromHex("0xd08864f0ff9f68f992a72baefd9550f1f6735b7b0e334d80623021cc5a59eff1"),
	},
	{
		"string_4",
		struct {
			Data string `ssz-size:"32"`
		}{"hello"},
		fromHex("0x68656c6c6f000000000000000000000000000000000000000000000000000000"),
		fromHex("0x68656c6c6f000000000000000000000000000000000000000000000000000000"),
	},
	{
		"string_5",
		struct {
			Data string `ssz-size:"32"`
		}{"abcdefghijklmnopqrstuvwxyz123456"},
		fromHex("0x6162636465666768696a6b6c6d6e6f707172737475767778797a313233343536"),
		fromHex("0x6162636465666768696a6b6c6d6e6f707172737475767778797a313233343536"),
	},
	{
		"string_6",
		struct {
			Data string `ssz-type:"progressive-list"`
		}{"abcdefghijklmnopqrstuvwxyz123456"},
		fromHex("0x040000006162636465666768696a6b6c6d6e6f707172737475767778797a313233343536"),
		fromHex("0x41ba7be636dd08b32cca499285494e18f8849fbba06a7ced2d0d692777228e10"),
	},

	// TypeWrapper test cases
	{
		"type_wrapper_1",
		func() any {
			type WrappedByteArray = dynssz.TypeWrapper[struct {
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
		fromHex("0x000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
	},
	{
		"type_wrapper_2",
		func() any {
			type WrappedUint32List = dynssz.TypeWrapper[struct {
				Data []uint32 `ssz-max:"1024"`
			}, []uint32]
			return WrappedUint32List{
				Data: []uint32{1, 2, 3, 4, 5},
			}
		}(),
		fromHex("0x0100000002000000030000000400000005000000"),
		fromHex("0xde9d30df4d1e540e54fce5b71ac54721913eef9742795d09fa170354d01b80db"),
	},
	{
		"type_wrapper_3",
		func() any {
			type WrappedBool = dynssz.TypeWrapper[struct {
				Data bool
			}, bool]
			return WrappedBool{
				Data: true,
			}
		}(),
		fromHex("0x01"),
		fromHex("0x0100000000000000000000000000000000000000000000000000000000000000"),
	},
	{
		"type_wrapper_4",
		func() any {
			type WrappedByteArray = dynssz.TypeWrapper[struct {
				Data [32]byte
			}, [32]byte]
			var testData [32]byte
			for i := range testData {
				testData[i] = byte(i)
			}
			return WrappedByteArray{
				Data: testData,
			}
		}(),
		fromHex("0x000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
		fromHex("0x000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
	},
	{
		"type_wrapper_5",
		func() any {
			type WrappedUint16List = dynssz.TypeWrapper[struct {
				Data []uint16 `ssz-max:"30"`
			}, []uint16]
			return WrappedUint16List{
				Data: []uint16{14028, 14029, 14030},
			}
		}(),
		fromHex("0xcc36cd36ce36"),
		fromHex("0xee1b490c066fd9628f79bae66126af845bd7d5bbe406b6344fc88d9e1fb25c41"),
	},

	// types with fastssz methods
	{
		"type_fastssz_1",
		TestContainerWithFastSsz{1, 2, true, 4},
		fromHex("0x010000000000000002000000010400"),
		fromHex("0x4138be0e47d6daea84065f2a1e4435e16d2b269f9c2c8fcf9e6cf03de1d5026e"),
	},
	{
		"type_dynamicssz_1",
		TestContainerWithDynamicSsz{1, 2, true, 4},
		fromHex("0x010000000000000002000000010400"),
		fromHex("0x4138be0e47d6daea84065f2a1e4435e16d2b269f9c2c8fcf9e6cf03de1d5026e"),
	},
	{
		"type_fastssz_2",
		struct {
			Field0 uint64
			Field1 []TestContainerWithFastSsz
		}{1, []TestContainerWithFastSsz{{1, 2, true, 4}, {5, 6, true, 8}}},
		fromHex("0x01000000000000000c000000010000000000000002000000010400050000000000000006000000010800"),
		fromHex("0x80b99000797f72ef1a9deae3e42fc1447648feaf1d7cd8dc1a4e20c7c64350ed"),
	},
	{
		"type_dynamicssz_2",
		struct {
			Field0 uint64
			Field1 []TestContainerWithDynamicSsz
		}{1, []TestContainerWithDynamicSsz{{1, 2, true, 4}, {5, 6, true, 8}}},
		fromHex("0x01000000000000000c000000010000000000000002000000010400050000000000000006000000010800"),
		fromHex("0x80b99000797f72ef1a9deae3e42fc1447648feaf1d7cd8dc1a4e20c7c64350ed"),
	},
}

// TestContainerWithFastSsz is a test container with fast ssz methods.
type TestContainerWithFastSsz struct {
	Field0 uint64
	Field1 uint32
	Field2 bool
	Field3 uint16
}

var _ sszutils.FastsszUnmarshaler = (*TestContainerWithFastSsz)(nil)
var _ sszutils.FastsszMarshaler = (*TestContainerWithFastSsz)(nil)
var _ sszutils.FastsszHashRoot = (*TestContainerWithFastSsz)(nil)

func (c *TestContainerWithFastSsz) UnmarshalSSZ(buf []byte) error {
	if c == nil {
		c = new(TestContainerWithFastSsz)
	}
	c.Field0 = uint64(sszutils.UnmarshallUint64(buf[:8]))
	c.Field1 = uint32(sszutils.UnmarshallUint32(buf[8:12]))
	c.Field2 = sszutils.UnmarshalBool(buf[12:13])
	c.Field3 = uint16(sszutils.UnmarshallUint16(buf[13:15]))
	return nil
}
func (c *TestContainerWithFastSsz) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, 15)
	return c.MarshalSSZTo(buf)
}
func (c *TestContainerWithFastSsz) MarshalSSZTo(buf []byte) ([]byte, error) {
	buf = sszutils.MarshalUint64(buf, uint64(c.Field0))
	buf = sszutils.MarshalUint32(buf, uint32(c.Field1))
	buf = sszutils.MarshalBool(buf, c.Field2)
	buf = sszutils.MarshalUint16(buf, uint16(c.Field3))
	return buf, nil
}
func (c *TestContainerWithFastSsz) SizeSSZ() int {
	return 15
}

func (c *TestContainerWithFastSsz) HashTreeRoot() ([32]byte, error) {
	pool := &hasher.DefaultHasherPool

	hh := pool.Get()
	defer func() {
		pool.Put(hh)
	}()

	err := c.HashTreeRootWith(hh)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}

func (c *TestContainerWithFastSsz) HashTreeRootWith(hh sszutils.HashWalker) (err error) {
	indx := hh.Index()
	hh.PutUint64(c.Field0)
	hh.PutUint32(c.Field1)
	hh.PutBool(c.Field2)
	hh.PutUint16(c.Field3)
	hh.Merkleize(indx)
	return
}

type TestContainerWithFastSsz2 TestContainerWithFastSsz

func (c TestContainerWithFastSsz2) UnmarshalSSZ(buf []byte) error {
	return (*TestContainerWithFastSsz)(&c).UnmarshalSSZ(buf)
}
func (c TestContainerWithFastSsz2) MarshalSSZ() ([]byte, error) {
	return (*TestContainerWithFastSsz)(&c).MarshalSSZ()
}
func (c TestContainerWithFastSsz2) MarshalSSZTo(buf []byte) ([]byte, error) {
	return (*TestContainerWithFastSsz)(&c).MarshalSSZTo(buf)
}
func (c TestContainerWithFastSsz2) SizeSSZ() int {
	return (*TestContainerWithFastSsz)(&c).SizeSSZ()
}
func (c TestContainerWithFastSsz2) HashTreeRoot() ([32]byte, error) {
	return (*TestContainerWithFastSsz)(&c).HashTreeRoot()
}
func (c TestContainerWithFastSsz2) HashTreeRootWith(hh sszutils.HashWalker) error {
	return (*TestContainerWithFastSsz)(&c).HashTreeRootWith(hh)
}

// TestContainerWithDynamicSsz is a test container with dynamic ssz methods.
type TestContainerWithDynamicSsz struct {
	Field0 uint64
	Field1 uint32
	Field2 bool
	Field3 uint16
}

var _ sszutils.DynamicUnmarshaler = (*TestContainerWithDynamicSsz)(nil)
var _ sszutils.DynamicMarshaler = (*TestContainerWithDynamicSsz)(nil)
var _ sszutils.DynamicHashRoot = (*TestContainerWithDynamicSsz)(nil)

func (c *TestContainerWithDynamicSsz) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	if c == nil {
		c = new(TestContainerWithDynamicSsz)
	}
	c.Field0 = uint64(sszutils.UnmarshallUint64(buf[:8]))
	c.Field1 = uint32(sszutils.UnmarshallUint32(buf[8:12]))
	c.Field2 = sszutils.UnmarshalBool(buf[12:13])
	c.Field3 = uint16(sszutils.UnmarshallUint16(buf[13:15]))
	return nil
}
func (c *TestContainerWithDynamicSsz) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	buf = sszutils.MarshalUint64(buf, uint64(c.Field0))
	buf = sszutils.MarshalUint32(buf, uint32(c.Field1))
	buf = sszutils.MarshalBool(buf, c.Field2)
	buf = sszutils.MarshalUint16(buf, uint16(c.Field3))
	return buf, nil
}
func (c *TestContainerWithDynamicSsz) SizeSSZDyn(_ sszutils.DynamicSpecs) int {
	return 15
}

func (c *TestContainerWithDynamicSsz) HashTreeRootDyn(ds sszutils.DynamicSpecs) ([32]byte, error) {
	pool := &hasher.DefaultHasherPool

	hh := pool.Get()
	defer func() {
		pool.Put(hh)
	}()

	err := c.HashTreeRootWithDyn(ds, hh)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}

func (c *TestContainerWithDynamicSsz) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) (err error) {
	indx := hh.Index()
	hh.PutUint64(c.Field0)
	hh.PutUint32(c.Field1)
	hh.PutBool(c.Field2)
	hh.PutUint16(c.Field3)
	hh.Merkleize(indx)
	return
}

// TestContainerWithDynamicSsz2 is a test container with dynamic ssz methods.
type TestContainerWithDynamicSsz2 TestContainerWithDynamicSsz

func (c TestContainerWithDynamicSsz2) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) error {
	return (*TestContainerWithDynamicSsz)(&c).UnmarshalSSZDyn(ds, buf)
}
func (c TestContainerWithDynamicSsz2) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return (*TestContainerWithDynamicSsz)(&c).MarshalSSZDyn(ds, buf)
}
func (c TestContainerWithDynamicSsz2) SizeSSZDyn(ds sszutils.DynamicSpecs) int {
	return (*TestContainerWithDynamicSsz)(&c).SizeSSZDyn(ds)
}
func (c TestContainerWithDynamicSsz2) HashTreeRootDyn(ds sszutils.DynamicSpecs) ([32]byte, error) {
	return (*TestContainerWithDynamicSsz)(&c).HashTreeRootDyn(ds)
}
func (c TestContainerWithDynamicSsz2) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	return (*TestContainerWithDynamicSsz)(&c).HashTreeRootWithDyn(ds, hh)
}

// TestContainerWithDynamicSsz3 is a test container with dynamic ssz methods.
type TestContainerWithDynamicSsz3 TestContainerWithDynamicSsz

func (c TestContainerWithDynamicSsz3) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) error {
	return (*TestContainerWithDynamicSsz)(&c).UnmarshalSSZDyn(ds, buf)
}
func (c TestContainerWithDynamicSsz3) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return (*TestContainerWithDynamicSsz)(&c).MarshalSSZDyn(ds, buf)
}
func (c TestContainerWithDynamicSsz3) HashTreeRootDyn(ds sszutils.DynamicSpecs) ([32]byte, error) {
	return (*TestContainerWithDynamicSsz)(&c).HashTreeRootDyn(ds)
}
func (c TestContainerWithDynamicSsz3) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	return (*TestContainerWithDynamicSsz)(&c).HashTreeRootWithDyn(ds, hh)
}

// TestContainerWithHashError is a test container with HashTreeRootWith returning an error.
type TestContainerWithHashError struct {
	Field0 uint64
}

var _ sszutils.FastsszHashRoot = (*TestContainerWithHashError)(nil)

func (c *TestContainerWithHashError) HashTreeRoot() ([32]byte, error) {
	pool := &hasher.DefaultHasherPool

	hh := pool.Get()
	defer func() {
		pool.Put(hh)
	}()

	err := c.HashTreeRootWith(hh)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}

func (c *TestContainerWithHashError) HashTreeRootWith(hh sszutils.HashWalker) error {
	return fmt.Errorf("test HashTreeRootWith error")
}

// TestContainerWithHashTreeRootOnly has HashTreeRoot but not HashTreeRootWith.
type TestContainerWithHashTreeRootOnly struct {
	Field0 uint64
}

var _ sszutils.FastsszHashRoot = (*TestContainerWithHashTreeRootOnly)(nil)

func (c *TestContainerWithHashTreeRootOnly) HashTreeRoot() ([32]byte, error) {
	var result [32]byte
	result[0] = byte(c.Field0)
	return result, nil
}

// TestContainerWithDynamicHashError has DynamicHashRoot that returns an error.
type TestContainerWithDynamicHashError struct {
	Field0 uint64
}

var _ sszutils.DynamicHashRoot = (*TestContainerWithDynamicHashError)(nil)

func (c *TestContainerWithDynamicHashError) HashTreeRootDyn(ds sszutils.DynamicSpecs) ([32]byte, error) {
	return [32]byte{}, fmt.Errorf("test DynamicHashRoot error")
}

func (c *TestContainerWithDynamicHashError) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	return fmt.Errorf("test DynamicHashRoot error")
}

// TestContainerWithHashRootError has HashTreeRoot that returns an error.
type TestContainerWithHashRootError struct {
	Field0 uint64
}

var _ sszutils.FastsszHashRoot = (*TestContainerWithHashRootError)(nil)

func (c *TestContainerWithHashRootError) HashTreeRoot() ([32]byte, error) {
	return [32]byte{}, fmt.Errorf("test HashTreeRoot error")
}

// TestContainerWithMarshalError has MarshalSSZTo that returns an error.
type TestContainerWithMarshalError struct {
	Field0 uint64
}

var _ sszutils.FastsszMarshaler = (*TestContainerWithMarshalError)(nil)
var _ sszutils.FastsszUnmarshaler = (*TestContainerWithMarshalError)(nil)
var _ sszutils.FastsszHashRoot = (*TestContainerWithMarshalError)(nil)

func (c *TestContainerWithMarshalError) MarshalSSZ() ([]byte, error) {
	return nil, fmt.Errorf("test MarshalSSZTo error")
}

func (c *TestContainerWithMarshalError) MarshalSSZTo(buf []byte) ([]byte, error) {
	return nil, fmt.Errorf("test MarshalSSZTo error")
}

func (c *TestContainerWithMarshalError) SizeSSZ() int {
	return 8
}

func (c *TestContainerWithMarshalError) UnmarshalSSZ(buf []byte) error {
	return fmt.Errorf("test UnmarshalSSZ error")
}

func (c *TestContainerWithMarshalError) HashTreeRoot() ([32]byte, error) {
	return [32]byte{}, fmt.Errorf("test HashTreeRoot error")
}

// TestContainerWithDynamicMarshalError has MarshalSSZDyn that returns an error.
type TestContainerWithDynamicMarshalError struct {
	Field0 uint64
}

var _ sszutils.DynamicMarshaler = (*TestContainerWithDynamicMarshalError)(nil)
var _ sszutils.DynamicUnmarshaler = (*TestContainerWithDynamicMarshalError)(nil)

func (c *TestContainerWithDynamicMarshalError) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return nil, fmt.Errorf("test MarshalSSZDyn error")
}

func (c *TestContainerWithDynamicMarshalError) SizeSSZDyn(ds sszutils.DynamicSpecs) int {
	return 8
}

func (c *TestContainerWithDynamicMarshalError) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) error {
	return fmt.Errorf("test UnmarshalSSZDyn error")
}

// TestContainerWithSizerError has SizeSSZ that returns an error behavior.
type TestContainerWithSizerError struct {
	Field0 uint64
}

var _ sszutils.FastsszMarshaler = (*TestContainerWithSizerError)(nil)

func (c *TestContainerWithSizerError) MarshalSSZ() ([]byte, error) {
	return make([]byte, 8), nil
}

func (c *TestContainerWithSizerError) MarshalSSZTo(buf []byte) ([]byte, error) {
	return append(buf, make([]byte, 8)...), nil
}

func (c *TestContainerWithSizerError) SizeSSZ() int {
	return 8
}

// streamingTestMatrix contains test cases for streaming marshal/unmarshal
var streamingTestMatrix = []struct {
	name    string
	payload any
	ssz     []byte
}{
	// DynamicWriter tests
	{
		"dynamic_writer_container",
		struct {
			Data TestContainerWithDynamicWriter
		}{TestContainerWithDynamicWriter{Field0: 0x123456789ABCDEF0, Field1: 0x12345678}},
		fromHex("0xf0debc9a7856341278563412"),
	},

	// DynamicReader tests
	{
		"dynamic_reader_container",
		struct {
			Data TestContainerWithDynamicReader
		}{TestContainerWithDynamicReader{Field0: 0x123456789ABCDEF0, Field1: 0x12345678}},
		fromHex("0xf0debc9a7856341278563412"),
	},

	// time.Time tests
	{
		"time_field",
		struct {
			Timestamp time.Time
		}{time.Unix(1718236800, 0)},
		fromHex("0x80366a6600000000"),
	},
	{
		"time_field_zero",
		struct {
			Timestamp time.Time
		}{time.Unix(0, 0)},
		fromHex("0x0000000000000000"),
	},

	// String as list tests
	{
		"string_list_empty",
		struct {
			Data string `ssz-max:"100"`
		}{""},
		fromHex("0x04000000"),
	},
	{
		"string_list_short",
		struct {
			Data string `ssz-max:"100"`
		}{"ab"},
		fromHex("0x040000006162"),
	},
	{
		"string_list_unicode",
		struct {
			Data string `ssz-max:"100"`
		}{"日本"},
		fromHex("0x04000000e697a5e69cac"),
	},

	// Bitlist edge cases
	{
		"bitlist_single_termination_bit",
		struct {
			Bits []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{[]byte{0x01}}, // just termination bit
		fromHex("0x0400000001"),
	},
	{
		"bitlist_with_data",
		struct {
			Bits []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{[]byte{0xFF, 0x03}}, // 10 bits set + termination
		fromHex("0x04000000ff03"),
	},

	// Dynamic vector with short slice (padding needed)
	{
		"dynamic_vector_padding",
		struct {
			Data []*slug_DynStruct1 `ssz-size:"3"`
		}{[]*slug_DynStruct1{{true, []uint8{1}}}},
		// container offset (4) + 3 item offsets (12,18,23) + item0 (6 bytes) + item1 (5 bytes) + item2 (5 bytes)
		fromHex("0x040000000c000000120000001700000001050000000100050000000005000000"),
	},

	// List with nil pointer elements
	{
		"list_with_nil_pointers",
		struct {
			Data []*slug_StaticStruct1 `ssz-max:"10"`
		}{[]*slug_StaticStruct1{nil, nil}},
		// offset (4 bytes) + 2 items * 4 bytes (bool=0 + 3 byte array=000000)
		fromHex("0x040000000000000000000000"),
	},

	// Static vector with various element counts
	{
		"static_vector_elements",
		struct {
			Data [3]uint64
		}{[3]uint64{1, 2, 3}},
		fromHex("0x010000000000000002000000000000000300000000000000"),
	},

	// Dynamic list with multiple items
	{
		"dynamic_list_multi",
		struct {
			Data [][]uint8 `ssz-max:"10,10"`
		}{[][]uint8{{1, 2}, {3, 4, 5}}},
		// container offset (4) + 2 offsets (8,10) + item0 (0102) + item1 (030405)
		fromHex("0x04000000080000000a0000000102030405"),
	},

	// Byte slice vector
	{
		"byte_vector_slice",
		struct {
			Data []byte `ssz-size:"8"`
		}{[]byte{1, 2, 3, 4, 5, 6, 7, 8}},
		fromHex("0x0102030405060708"),
	},

	// Byte vector short (padding)
	{
		"byte_vector_short",
		struct {
			Data []byte `ssz-size:"8"`
		}{[]byte{1, 2, 3}},
		fromHex("0x0102030000000000"),
	},

	// String vector (fixed size)
	{
		"string_vector_fixed",
		struct {
			Data string `ssz-size:"8"`
		}{"hello"},
		fromHex("0x68656c6c6f000000"),
	},

	// Non-byte element vector short
	{
		"uint32_vector_short",
		struct {
			Data []uint32 `ssz-size:"4"`
		}{[]uint32{1, 2}},
		fromHex("0x01000000020000000000000000000000"),
	},
}

// TestContainerWithDynamicWriter implements DynamicWriter interface for testing
type TestContainerWithDynamicWriter struct {
	Field0 uint64
	Field1 uint32
}

var _ sszutils.DynamicWriter = (*TestContainerWithDynamicWriter)(nil)
var _ sszutils.DynamicSizer = (*TestContainerWithDynamicWriter)(nil)

func (c *TestContainerWithDynamicWriter) MarshalSSZDynWriter(_ sszutils.DynamicSpecs, w io.Writer) error {
	buf := make([]byte, 12)
	sszutils.MarshalUint64(buf[:0], c.Field0)
	sszutils.MarshalUint32(buf[8:8], c.Field1)
	_, err := w.Write(buf)
	return err
}

func (c *TestContainerWithDynamicWriter) SizeSSZDyn(_ sszutils.DynamicSpecs) int {
	return 12
}

// TestContainerWithDynamicWriterError implements DynamicWriter that returns errors
type TestContainerWithDynamicWriterError struct {
	Field0 uint64
}

var _ sszutils.DynamicWriter = (*TestContainerWithDynamicWriterError)(nil)

func (c *TestContainerWithDynamicWriterError) MarshalSSZDynWriter(_ sszutils.DynamicSpecs, w io.Writer) error {
	return io.ErrClosedPipe
}

func (c *TestContainerWithDynamicWriterError) SizeSSZDyn(_ sszutils.DynamicSpecs) int {
	return 8
}

// TestContainerWithDynamicReader implements DynamicReader interface for testing
type TestContainerWithDynamicReader struct {
	Field0 uint64
	Field1 uint32
}

var _ sszutils.DynamicReader = (*TestContainerWithDynamicReader)(nil)

func (c *TestContainerWithDynamicReader) UnmarshalSSZDynReader(_ sszutils.DynamicSpecs, r io.Reader) error {
	buf := make([]byte, 12)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	c.Field0 = sszutils.UnmarshallUint64(buf[:8])
	c.Field1 = sszutils.UnmarshallUint32(buf[8:12])
	return nil
}

// TestContainerWithDynamicReaderError implements DynamicReader that returns errors
type TestContainerWithDynamicReaderError struct {
	Field0 uint64
}

var _ sszutils.DynamicReader = (*TestContainerWithDynamicReaderError)(nil)

func (c *TestContainerWithDynamicReaderError) UnmarshalSSZDynReader(_ sszutils.DynamicSpecs, r io.Reader) error {
	return io.ErrClosedPipe
}
