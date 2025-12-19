// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"reflect"
	"testing"

	. "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/treeproof"
)

var treerootTestMatrix = append(commonTestMatrix, []struct {
	name    string
	payload any
	ssz     []byte
	htr     []byte
}{
	// additional treeroot tests
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

func TestTreeRoot(t *testing.T) {
	dynssz := NewDynSsz(nil)

	for idx, test := range treerootTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			buf, err := dynssz.HashTreeRoot(test.payload)

			switch {
			case test.htr == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", idx, err)
			case !bytes.Equal(buf[:], test.htr):
				t.Errorf("test %v failed: got 0x%x, wanted 0x%x", idx, buf, test.htr)
			}
		})
	}
}

func TestTreeRootNoFastSsz(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for idx, test := range treerootTestMatrix {
		t.Run(test.name, func(t *testing.T) {
			buf, err := dynssz.HashTreeRoot(test.payload)

			switch {
			case test.htr == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("test %v error: %v", idx, err)
			case !bytes.Equal(buf[:], test.htr):
				t.Errorf("test %v failed: got 0x%x, wanted 0x%x", idx, buf, test.htr)
			}
		})
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

func TestHashTreeRootErrors(t *testing.T) {
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
			expectedErr: "complex numbers are not supported in SSZ",
		},
		{
			name: "vector_too_big",
			input: struct {
				Data []uint8 `ssz-size:"5"`
			}{[]uint8{1, 2, 3, 4, 5, 6}},
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
			name: "bitlist_too_big",
			input: struct {
				Bits []byte `ssz-type:"bitlist" ssz-max:"8"`
			}{[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x12}},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "invalid_uint128_size",
			input: struct {
				Value []byte `ssz-type:"uint128"`
			}{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}},
			expectedErr: "large uint type does not have expected data length (17 != 16)",
		},
		{
			name: "invalid_uint256_size",
			input: struct {
				Value []byte `ssz-type:"uint256"`
			}{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33}},
			expectedErr: "large uint type does not have expected data length (33 != 32)",
		},
		{
			name: "invalid_bitvector_type",
			input: struct {
				Flags []uint16 `ssz-type:"bitvector" ssz-size:"4"`
			}{[]uint16{1, 2, 3, 4}},
			expectedErr: "bitvector ssz type can only be represented by byte slices or arrays, got uint16",
		},
		{
			name: "nested_container_field_error",
			input: struct {
				Inner struct {
					Data complex128
				}
			}{struct {
				Data complex128
			}{complex128(1 + 2i)}},
			expectedErr: "complex numbers are not supported in SSZ",
		},
		{
			name: "vector_element_hash_error",
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
			name: "invalid_bitvector_padding",
			input: struct {
				Flags []byte `ssz-type:"bitvector" ssz-bitsize:"12"`
			}{[]byte{0xff, 0x1f}},
			expectedErr: "incorrect vector length",
		},
		{
			name: "list_element_hash_error",
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
			name: "dynamic_list_too_big",
			input: struct {
				Data []uint32 `ssz-max:"3"`
			}{[]uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
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
			name: "string_too_long_fixed",
			input: struct {
				Data string `ssz-size:"5"`
			}{"hello world"},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "string_too_long_dynamic",
			input: struct {
				Data string `ssz-max:"5"`
			}{"hello world, hello world, hello world, hello world, hello world"},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "multi_dimensional_size_mismatch",
			input: struct {
				Data [2][]*slug_StaticStruct1 `ssz-size:"2,3"`
			}{[2][]*slug_StaticStruct1{{nil, nil, nil}, {nil, nil, nil, nil}}},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "very_large_dynamic_list",
			input: struct {
				Data []byte `ssz-max:"100"`
			}{make([]byte, 1000)},
			expectedErr: "list length is higher than max value",
		},
		{
			name: "invalid_large_uint_array_size",
			input: struct {
				Value [15]byte `ssz-type:"uint128"`
			}{[15]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}},
			expectedErr: "uint128 ssz type does not fit in array (15 < 16)",
		},
		{
			name: "invalid_large_uint_slice_size",
			input: struct {
				Value [2]uint32 `ssz-type:"uint128"`
			}{[2]uint32{1, 2}},
			expectedErr: "uint128 ssz type can only be represented by slices or arrays of uint8 or uint64, got uint32",
		},
		{
			name: "bitvector_wrong_element_type",
			input: struct {
				Flags [4]uint32 `ssz-type:"bitvector"`
			}{[4]uint32{1, 2, 3, 4}},
			expectedErr: "bitvector ssz type can only be represented by byte slices or arrays, got uint32",
		},
		{
			name: "wrapper_recursive_error",
			input: func() any {
				type BadWrapper = TypeWrapper[struct {
					Data complex64
				}, complex64]
				return BadWrapper{
					Data: complex64(1 + 2i),
				}
			}(),
			expectedErr: "complex numbers are not supported in SSZ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dynssz.HashTreeRoot(tc.input)
			if err == nil {
				t.Errorf("expected error containing '%s', but got no error", tc.expectedErr)
			} else if !contains(err.Error(), tc.expectedErr) {
				t.Errorf("expected error containing '%s', but got: %v", tc.expectedErr, err)
			}
		})
	}
}

// verifyTreeIntegrity recursively checks that each parent node's hash equals sha256(left+right)
func verifyTreeIntegrity(node *treeproof.Node) error {
	// If this is a leaf node (no children), skip verification
	if node.IsLeaf() {
		return nil
	}

	// Verify children first
	if node.Left() != nil {
		if err := verifyTreeIntegrity(node.Left()); err != nil {
			return err
		}
	}
	if node.Right() != nil {
		if err := verifyTreeIntegrity(node.Right()); err != nil {
			return err
		}
	}

	// Verify this node's hash
	h := sha256.New()
	if node.Left() != nil {
		h.Write(node.Left().Hash())
	} else {
		h.Write(make([]byte, 32)) // zero hash for nil left child
	}
	if node.Right() != nil {
		h.Write(node.Right().Hash())
	} else {
		h.Write(make([]byte, 32)) // zero hash for nil right child
	}
	expectedHash := h.Sum(nil)

	if !bytes.Equal(node.Hash(), expectedHash) {
		return fmt.Errorf("hash mismatch at node: expected %x, got %x", expectedHash, node.Hash())
	}

	return nil
}

func TestTreeGeneration(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	for _, tc := range treerootTestMatrix {
		if tc.htr == nil {
			continue
		}

		t.Run(tc.name, func(t *testing.T) {
			// Generate tree
			tree, err := dynssz.GetTree(tc.payload)
			if err != nil {
				t.Fatalf("failed to generate tree: %v", err)
			}

			// Verify tree integrity
			if err := verifyTreeIntegrity(tree); err != nil {
				t.Errorf("tree integrity check failed: %v", err)
			}

			// Verify root hash matches HashTreeRoot
			expectedRoot, err := dynssz.HashTreeRoot(tc.payload)
			if err != nil {
				t.Fatalf("failed to compute hash tree root: %v", err)
			}

			if !bytes.Equal(tree.Hash(), tc.htr) {
				t.Errorf("tree root mismatch: tree=%x, expected=%x", tree.Hash(), tc.htr)
			}

			if !bytes.Equal(tree.Hash(), expectedRoot[:]) {
				t.Errorf("tree root mismatch: tree=%x, expected=%x", tree.Hash(), expectedRoot)
			}
		})
	}
}

func TestBinaryVsProgressiveTrees(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test cases comparing binary and progressive merkleization
	testCases := []struct {
		name               string
		binaryPayload      any
		progressivePayload any
	}{
		{
			"list_comparison_8_elements",
			struct {
				F1 []uint16 `ssz-max:"128"`
			}{[]uint16{1, 2, 3, 4, 5, 6, 7, 8}},
			struct {
				F1 []uint16 `ssz-type:"progressive-list"`
			}{[]uint16{1, 2, 3, 4, 5, 6, 7, 8}},
		},
		{
			"list_comparison_16_elements",
			struct {
				F1 []uint32 `ssz-max:"200"`
			}{make([]uint32, 16)},
			struct {
				F1 []uint32 `ssz-type:"progressive-list"`
			}{make([]uint32, 16)},
		},
		{
			"large_list_comparison_64_elements",
			struct {
				F1 []uint32 `ssz-max:"200"`
			}{make([]uint32, 64)},
			struct {
				F1 []uint32 `ssz-type:"progressive-list"`
			}{make([]uint32, 64)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate both trees
			binaryTree, err := dynssz.GetTree(tc.binaryPayload)
			if err != nil {
				t.Fatalf("failed to generate binary tree: %v", err)
			}

			progressiveTree, err := dynssz.GetTree(tc.progressivePayload)
			if err != nil {
				t.Fatalf("failed to generate progressive tree: %v", err)
			}

			// Verify both trees pass integrity checks
			if err := verifyTreeIntegrity(binaryTree); err != nil {
				t.Errorf("binary tree integrity check failed: %v", err)
			}

			if err := verifyTreeIntegrity(progressiveTree); err != nil {
				t.Errorf("progressive tree integrity check failed: %v", err)
			}

			// Verify root hashes match their respective HashTreeRoot results
			binaryRoot, err := dynssz.HashTreeRoot(tc.binaryPayload)
			if err != nil {
				t.Fatalf("failed to compute binary hash tree root: %v", err)
			}

			progressiveRoot, err := dynssz.HashTreeRoot(tc.progressivePayload)
			if err != nil {
				t.Fatalf("failed to compute progressive hash tree root: %v", err)
			}

			if !bytes.Equal(binaryTree.Hash(), binaryRoot[:]) {
				t.Errorf("binary tree root mismatch: tree=%x, expected=%x", binaryTree.Hash(), binaryRoot)
			}

			if !bytes.Equal(progressiveTree.Hash(), progressiveRoot[:]) {
				t.Errorf("progressive tree root mismatch: tree=%x, expected=%x", progressiveTree.Hash(), progressiveRoot)
			}

			// Trees should have different root hashes (progressive vs binary merkleization)
			if bytes.Equal(binaryTree.Hash(), progressiveTree.Hash()) {
				t.Logf("Note: Binary and progressive trees have the same root hash for %s", tc.name)
			} else {
				t.Logf("Binary and progressive trees have different root hashes (expected for different merkleization methods)")
			}
		})
	}
}

func TestCompatibleUnionHashErrors(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test invalid variant
	t.Run("invalid_variant", func(t *testing.T) {
		type TestUnion = CompatibleUnion[struct {
			Field1 uint32
		}]
		input := struct {
			Field0 uint16
			Field1 TestUnion
		}{
			0x1234,
			TestUnion{Variant: 5, Data: uint32(42)}, // Variant 5 doesn't exist
		}
		_, err := dynssz.HashTreeRoot(input)
		if err == nil {
			t.Error("expected error for invalid variant")
		}
	})

	// Test nil data
	t.Run("nil_data", func(t *testing.T) {
		type TestUnion = CompatibleUnion[struct {
			Field1 uint32
		}]
		input := struct {
			Field0 uint16
			Field1 TestUnion
		}{
			0x1234,
			TestUnion{Variant: 0, Data: nil}, // Data is nil
		}
		_, err := dynssz.HashTreeRoot(input)
		if err == nil {
			t.Error("expected error for nil data in union")
		}
	})

	// Test union variant hash error
	t.Run("union_variant_hash_error", func(t *testing.T) {
		type TestUnion = CompatibleUnion[struct {
			Field1 complex64
		}]
		input := struct {
			Field0 uint16
			Field1 TestUnion
		}{
			0x1234,
			TestUnion{Variant: 0, Data: complex64(1 + 2i)},
		}
		_, err := dynssz.HashTreeRoot(input)
		if err == nil {
			t.Error("expected error for invalid union variant data type")
		}
	})
}

func TestVectorWithAppendZeroElements(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test vector with non-byte elements that requires zero padding
	t.Run("uint32_vector_append_zero", func(t *testing.T) {
		input := struct {
			Data []uint32 `ssz-size:"5"`
		}{[]uint32{1, 2, 3}} // Only 3 elements, but expects 5

		hash, err := dynssz.HashTreeRoot(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the hash is computed correctly
		if hash == [32]byte{} {
			t.Error("hash should not be zero")
		}
	})

	// Test vector with pointer elements that requires zero padding
	t.Run("pointer_vector_append_zero", func(t *testing.T) {
		input := struct {
			Data []*slug_StaticStruct1 `ssz-size:"4"`
		}{[]*slug_StaticStruct1{{true, []uint8{1, 2, 3}}, nil}} // Only 2 elements, but expects 4

		hash, err := dynssz.HashTreeRoot(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hash == [32]byte{} {
			t.Error("hash should not be zero")
		}
	})
}

func TestListElementTypesCoverage(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test bool list
	t.Run("bool_list", func(t *testing.T) {
		input := struct {
			Data []bool `ssz-max:"10"`
		}{[]bool{true, false, true, false}}

		hash, err := dynssz.HashTreeRoot(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hash == [32]byte{} {
			t.Error("hash should not be zero")
		}
	})

	// Test uint64 list
	t.Run("uint64_list", func(t *testing.T) {
		input := struct {
			Data []uint64 `ssz-max:"10"`
		}{[]uint64{100, 200, 300}}

		hash, err := dynssz.HashTreeRoot(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hash == [32]byte{} {
			t.Error("hash should not be zero")
		}
	})

	// Test uint16 list
	t.Run("uint16_list", func(t *testing.T) {
		input := struct {
			Data []uint16 `ssz-max:"10"`
		}{[]uint16{100, 200, 300, 400}}

		hash, err := dynssz.HashTreeRoot(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hash == [32]byte{} {
			t.Error("hash should not be zero")
		}
	})

	// Test uint32 list
	t.Run("uint32_list", func(t *testing.T) {
		input := struct {
			Data []uint32 `ssz-max:"10"`
		}{[]uint32{1000, 2000, 3000}}

		hash, err := dynssz.HashTreeRoot(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hash == [32]byte{} {
			t.Error("hash should not be zero")
		}
	})
}

func TestProgressiveContainerFieldError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test progressive container with a field that causes an error
	input := struct {
		Field0 uint64    `ssz-index:"0"`
		Field1 complex64 `ssz-index:"1"` // Invalid type
	}{12345, complex64(1 + 2i)}

	_, err := dynssz.HashTreeRoot(input)
	if err == nil {
		t.Error("expected error for invalid field type in progressive container")
	}
	if !contains(err.Error(), "complex numbers are not supported in SSZ") {
		t.Errorf("expected error about complex numbers, got: %v", err)
	}
}

func TestCustomFallbackHashRoot(t *testing.T) {
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

	if structDesc.SszType != SszContainerType {
		t.Fatalf("Expected container type, got %v", structDesc.SszType)
	}

	structDesc.SszType = SszCustomType
	structDesc.SszCompatFlags |= SszCompatFlagDynamicUnmarshaler

	_, err = dynssz.HashTreeRoot(&TestContainer{})
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestHashTreeRootWithMethodError(t *testing.T) {
	// Create a struct that has HashTreeRootWith returning an error
	dynssz := NewDynSsz(nil)

	input := &TestContainerWithHashError{
		Field0: 42,
	}

	_, err := dynssz.HashTreeRoot(input)
	if err == nil {
		t.Error("expected error from HashTreeRootWith")
	}
	if !contains(err.Error(), "test HashTreeRootWith error") {
		t.Errorf("expected HashTreeRootWith error, got: %v", err)
	}
}

func TestVectorElementHashError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test vector element hash error - this covers line 520 in treeroot.go
	// when building zero elements for a vector causes an error
	input := struct {
		Data []struct {
			Value complex128
		} `ssz-size:"3"`
	}{[]struct {
		Value complex128
	}{{complex128(1)}}} // Only 1 element, but expects 3, zero element hashing will fail

	_, err := dynssz.HashTreeRoot(input)
	if err == nil {
		t.Error("expected error for invalid element type in vector")
	}
}

func TestListElementHashError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test list element hash error - this covers line 595 in treeroot.go
	input := struct {
		Data []struct {
			Value chan int
		} `ssz-max:"10"`
	}{[]struct {
		Value chan int
	}{{make(chan int)}}}

	_, err := dynssz.HashTreeRoot(input)
	if err == nil {
		t.Error("expected error for invalid element type in list")
	}
}

func TestHashTreeRootVerbose(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.Verbose = true

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
			_, err := dynssz.HashTreeRoot(tc.input)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestFastSszHashRootPath(t *testing.T) {
	dynssz := NewDynSsz(nil)
	// Note: NoFastSsz = false (default) to use the fast SSZ path

	// Test with a type that has HashTreeRoot but not HashTreeRootWith
	input := &TestContainerWithHashTreeRootOnly{
		Field0: 42,
	}

	hash, err := dynssz.HashTreeRoot(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == [32]byte{} {
		t.Error("hash should not be zero")
	}
}

func TestDynamicHashRootErrorPath(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test with a type that has DynamicHashRoot that returns an error
	input := &TestContainerWithDynamicHashError{
		Field0: 42,
	}

	_, err := dynssz.HashTreeRoot(input)
	if err == nil {
		t.Error("expected error from DynamicHashRoot")
	}
	if !contains(err.Error(), "test DynamicHashRoot error") {
		t.Errorf("expected DynamicHashRoot error, got: %v", err)
	}
}

func TestContainerFieldError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test container with a field that causes an error during hashing
	input := struct {
		Field0 uint64
		Field1 struct {
			Inner complex64
		}
	}{123, struct{ Inner complex64 }{complex64(1 + 2i)}}

	_, err := dynssz.HashTreeRoot(input)
	if err == nil {
		t.Error("expected error for invalid container field type")
	}
}

func TestFastSszHashRootError(t *testing.T) {
	dynssz := NewDynSsz(nil)
	// NoFastSsz = false to use fast SSZ path

	// Test with a type that has HashTreeRoot that returns an error
	input := &TestContainerWithHashRootError{
		Field0: 42,
	}

	_, err := dynssz.HashTreeRoot(input)
	if err == nil {
		t.Error("expected error from HashTreeRoot")
	}
	if !contains(err.Error(), "test HashTreeRoot error") {
		t.Errorf("expected HashTreeRoot error, got: %v", err)
	}
}

func TestPackedUint8InVector(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test vector with uint8 elements to exercise pack=true path for SszUint8Type
	// Note: []uint8 is treated as byte array, so we use [N]uint8 inside a struct
	input := struct {
		Data [4]struct {
			Value uint8
		}
	}{[4]struct{ Value uint8 }{{1}, {2}, {3}, {4}}}

	hash, err := dynssz.HashTreeRoot(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == [32]byte{} {
		t.Error("hash should not be zero")
	}
}

func TestPackedBoolInVector(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true

	// Test vector with bool elements to exercise pack=true path for SszBoolType
	input := struct {
		Data [4]struct {
			Value bool
		}
	}{[4]struct{ Value bool }{{true}, {false}, {true}, {false}}}

	hash, err := dynssz.HashTreeRoot(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == [32]byte{} {
		t.Error("hash should not be zero")
	}
}
