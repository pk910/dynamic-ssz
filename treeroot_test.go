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
		fromHex("0xcb141fb9e033499344f568ea05a6a77ada886fc6e856ece01ae5a329e184fbd1"),
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
		fromHex("0x609aed07225400cb21de97260b267aab012358a235d1a1e9fc4df94859208c83"),
	},
	{
		struct {
			F1 uint8
			F2 []*slug_StaticStruct1 `ssz-size:"3"`
			F3 uint8
		}{42, []*slug_StaticStruct1{nil, {true, []uint8{4, 8, 4}}}, 43},
		fromHex("0xcb36f82247d205d8fc9dc60d04a245fb588be35315b4c3406ed2b68f69de7eda"),
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
		fromHex("0x253a3f3ffab684c2d4f4930b7923f31aadc3eff94b3eb8b4b7b9aa1363efcf52"),
	},

	// ssz-type annotation tests
	{
		struct {
			BitlistData []byte `ssz-type:"bitlist" ssz-max:"100"`
		}{[]byte{0x0f, 0x01}}, // bitlist with 4 bits set, length indicator
		fromHex("0xac0d43079c4f10cade6386f382829a4a00e4d9832cb66a068969c761bce57d96"),
	},
	
	// uint128 type tests
	{
		struct {
			Value [16]byte `ssz-type:"uint128"`
		}{[16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		fromHex("0x0102030405060708090a0b0c0d0e0f1000000000000000000000000000000000"),
	},
	{
		struct {
			Value [2]uint64 `ssz-type:"uint128"`
		}{[2]uint64{0x0807060504030201, 0x100f0e0d0c0b0a09}},
		fromHex("0x0102030405060708090a0b0c0d0e0f1000000000000000000000000000000000"),
	},
	{
		struct {
			Value []byte `ssz-type:"uint128" ssz-size:"16"`
		}{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}},
		fromHex("0x0102030405060708090a0b0c0d0e0f1000000000000000000000000000000000"),
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
		fromHex("0xff0f00f000000000000000000000000000000000000000000000000000000000"),
	},
	
	// explicit basic type annotations
	{
		struct {
			Value uint32 `ssz-type:"uint32"`
		}{0x12345678},
		fromHex("0x7856341200000000000000000000000000000000000000000000000000000000"),
	},
	{
		struct {
			Value bool `ssz-type:"bool"`
		}{true},
		fromHex("0x0100000000000000000000000000000000000000000000000000000000000000"),
	},
	
	// vector type annotation
	{
		struct {
			Values []uint64 `ssz-type:"vector" ssz-size:"3"`
		}{[]uint64{1, 2, 3}},
		fromHex("0x0100000000000000020000000000000003000000000000000000000000000000"),
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
		fromHex("0x40fb670c297a5c70d0b09f5f39cc5f1a442c79e86d7aaebe34a775c35c84e2e5"),
	},

	// string types
	{
		struct {
			Data string `ssz-max:"100"`
		}{""},
		fromHex("0x28ba1834a3a7b657460ce79fa3a1d909ab8828fd557659d4d0554a9bdbc0ec30"),
	},
	{
		struct {
			Data string `ssz-max:"100"`
		}{"hello"},
		fromHex("0x19da29a0796bb0ad502164fb6362e551756896856128aa64e415d5304a317b40"),
	},
	{
		struct {
			Data string `ssz-max:"100"`
		}{"hello 世界"},
		fromHex("0xd08864f0ff9f68f992a72baefd9550f1f6735b7b0e334d80623021cc5a59eff1"),
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

func TestStringVsByteContainerTreeRootEquivalence(t *testing.T) {
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

			strHash, err := dynssz.HashTreeRoot(strContainer)
			if err != nil {
				t.Fatalf("Failed to hash string container: %v", err)
			}

			byteHash, err := dynssz.HashTreeRoot(byteContainer)
			if err != nil {
				t.Fatalf("Failed to hash byte container: %v", err)
			}

			if strHash != byteHash {
				t.Errorf("Hash mismatch:\nString: %x\nBytes:  %x", strHash, byteHash)
			}
		})
	}
}

func TestFixedSizeStringVsByteArrayTreeRoot(t *testing.T) {
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

			strHash, err := dynssz.HashTreeRoot(strStruct)
			if err != nil {
				t.Fatalf("Failed to hash string struct: %v", err)
			}

			byteHash, err := dynssz.HashTreeRoot(byteStruct)
			if err != nil {
				t.Fatalf("Failed to hash byte struct: %v", err)
			}

			if strHash != byteHash {
				t.Errorf("Hash mismatch:\nString: %x\nBytes:  %x", strHash, byteHash)
			}
		})
	}
}

func TestStringSliceVsByteSliceTreeRoot(t *testing.T) {
	dynssz := NewDynSsz(nil)

	testCases := []struct {
		name    string
		strings []string
		bytes   [][]byte
	}{
		{
			"single_element",
			[]string{"hello"},
			[][]byte{[]byte("hello")},
		},
		{
			"multiple_elements",
			[]string{"one", "two", "three"},
			[][]byte{[]byte("one"), []byte("two"), []byte("three")},
		},
		{
			"with_empty",
			[]string{"", "test", ""},
			[][]byte{{}, []byte("test"), {}},
		},
		{
			"empty_slice",
			[]string{},
			[][]byte{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			strSliceHash, err := dynssz.HashTreeRoot(tc.strings)
			if err != nil {
				t.Fatalf("Failed to hash []string: %v", err)
			}

			bytesSliceHash, err := dynssz.HashTreeRoot(tc.bytes)
			if err != nil {
				t.Fatalf("Failed to hash [][]byte: %v", err)
			}

			if strSliceHash != bytesSliceHash {
				t.Errorf("[]string and [][]byte should have identical hash roots")
				t.Logf("[]string hash: %x", strSliceHash)
				t.Logf("[][]byte hash: %x", bytesSliceHash)
			}
		})
	}
}
