// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz_test

import (
	"bytes"
	"testing"

	. "github.com/pk910/dynamic-ssz"
)

// TestStringInContainerBasics tests string marshaling, unmarshaling, and size calculation within containers
func TestStringInContainerBasics(t *testing.T) {
	type StringContainer struct {
		Data string `ssz-max:"100"`
	}

	testCases := []struct {
		name         string
		value        string
		expectedSize int
	}{
		{"empty", "", 4},       // 4 bytes for offset
		{"simple", "hello", 9}, // 4 bytes offset + 5 bytes data
		{"unicode", "hello 世界", 4 + len("hello 世界")},
		{"with_nulls", "hello\x00world", 15}, // 4 bytes offset + 11 bytes data
	}

	dynssz := NewDynSsz(nil)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			container := StringContainer{Data: tc.value}

			// Test marshaling
			encoded, err := dynssz.MarshalSSZ(container)
			if err != nil {
				t.Fatalf("MarshalSSZ failed: %v", err)
			}

			// Test unmarshaling
			var decoded StringContainer
			err = dynssz.UnmarshalSSZ(&decoded, encoded)
			if err != nil {
				t.Fatalf("UnmarshalSSZ failed: %v", err)
			}
			if decoded.Data != tc.value {
				t.Errorf("UnmarshalSSZ mismatch: got %q, want %q", decoded.Data, tc.value)
			}

			// Test size calculation
			size, err := dynssz.SizeSSZ(container)
			if err != nil {
				t.Fatalf("SizeSSZ failed: %v", err)
			}
			if size != tc.expectedSize {
				t.Errorf("SizeSSZ mismatch: got %d, want %d", size, tc.expectedSize)
			}
		})
	}
}

// TestStringVsByteContainerEquivalence tests that containers with string vs []byte fields
// produce identical encoding, decoding, and hash roots
func TestStringVsByteContainerEquivalence(t *testing.T) {
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

			// Test encoding
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

			// Test hash roots
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

			// Test decoding round-trip
			var decodedStr StringContainer
			err = dynssz.UnmarshalSSZ(&decodedStr, strEncoded)
			if err != nil {
				t.Fatalf("Failed to unmarshal string container: %v", err)
			}

			if decodedStr.Data != tc.value {
				t.Errorf("String round-trip failed: got %q, want %q", decodedStr.Data, tc.value)
			}

			var decodedByte ByteContainer
			err = dynssz.UnmarshalSSZ(&decodedByte, byteEncoded)
			if err != nil {
				t.Fatalf("Failed to unmarshal byte container: %v", err)
			}

			if !bytes.Equal(decodedByte.Data, []byte(tc.value)) {
				t.Errorf("Byte round-trip failed: got %q, want %q", decodedByte.Data, tc.value)
			}
		})
	}
}

// TestFixedArraysOfStrings tests fixed-size arrays of strings within containers
func TestFixedArraysOfStrings(t *testing.T) {
	type StringArrayContainer struct {
		Data [3]string `ssz-max:"0,1000"`
	}

	type ByteArrayContainer struct {
		Data [3][]byte `ssz-max:"0,1000"`
	}

	dynssz := NewDynSsz(nil)

	// Test data
	strArray := [3]string{"hello", "world", "test"}
	byteArray := [3][]byte{[]byte("hello"), []byte("world"), []byte("test")}

	strContainer := StringArrayContainer{Data: strArray}
	byteContainer := ByteArrayContainer{Data: byteArray}

	// Test encoding
	strEncoded, err := dynssz.MarshalSSZ(strContainer)
	if err != nil {
		t.Fatalf("Failed to marshal string array container: %v", err)
	}

	byteEncoded, err := dynssz.MarshalSSZ(byteContainer)
	if err != nil {
		t.Fatalf("Failed to marshal byte array container: %v", err)
	}

	if !bytes.Equal(strEncoded, byteEncoded) {
		t.Errorf("Encoding mismatch between [3]string and [3][]byte containers")
	}

	// Test hash
	strHash, err := dynssz.HashTreeRoot(strContainer)
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	byteHash, err := dynssz.HashTreeRoot(byteContainer)
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	if strHash != byteHash {
		t.Errorf("Hash mismatch between [3]string and [3][]byte containers")
	}
}

// TestFixedSizeStringVsByteArray verifies that fixed-size strings behave
// identically to fixed-size byte arrays
func TestFixedSizeStringVsByteArray(t *testing.T) {
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
		{"over_32_truncated", "abcdefghijklmnopqrstuvwxyz1234567890"}, // Should be truncated
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create byte array with same data
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

			// Marshal both
			strData, err := dynssz.MarshalSSZ(strStruct)
			if err != nil {
				t.Fatalf("Failed to marshal string struct: %v", err)
			}

			byteStructData, err := dynssz.MarshalSSZ(byteStruct)
			if err != nil {
				t.Fatalf("Failed to marshal byte struct: %v", err)
			}

			// They should be identical
			if !bytes.Equal(strData, byteStructData) {
				t.Errorf("Marshaled data mismatch:\nString: %x\nBytes:  %x", strData, byteStructData)
			}

			// Hash both
			strHash, err := dynssz.HashTreeRoot(strStruct)
			if err != nil {
				t.Fatalf("Failed to hash string struct: %v", err)
			}

			byteHash, err := dynssz.HashTreeRoot(byteStruct)
			if err != nil {
				t.Fatalf("Failed to hash byte struct: %v", err)
			}

			// Hashes should be identical
			if strHash != byteHash {
				t.Errorf("Hash mismatch:\nString: %x\nBytes:  %x", strHash, byteHash)
			}

			// Test unmarshaling
			var decodedStr WithFixedString
			err = dynssz.UnmarshalSSZ(&decodedStr, strData)
			if err != nil {
				t.Fatalf("Failed to unmarshal string struct: %v", err)
			}

			// For strings longer than 32, we expect truncation
			// For strings shorter than 32, we expect null padding
			expectedStr := tc.value
			if len(expectedStr) > 32 {
				expectedStr = expectedStr[:32]
			} else if len(expectedStr) < 32 {
				// Pad with null bytes
				paddingBytes := make([]byte, 32-len(expectedStr))
				expectedStr = expectedStr + string(paddingBytes)
			}

			if decodedStr.Data != expectedStr {
				t.Errorf("Unmarshal mismatch: got %q, want %q", decodedStr.Data, expectedStr)
			}
		})
	}
}

// TestMixedStringTypes tests that mixing fixed and dynamic strings works correctly
func TestMixedStringTypes(t *testing.T) {
	type MixedStruct struct {
		FixedStr1  string `ssz-size:"16"`
		DynamicStr string `ssz-max:"100"`
		FixedStr2  string `ssz-size:"8"`
		ID         uint32
	}

	dynssz := NewDynSsz(nil)

	test := MixedStruct{
		FixedStr1:  "hello",
		DynamicStr: "world",
		FixedStr2:  "test",
		ID:         42,
	}

	data, err := dynssz.MarshalSSZ(test)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Expected layout:
	// 16 bytes for FixedStr1 (padded)
	// 4 bytes offset for DynamicStr
	// 8 bytes for FixedStr2 (padded)
	// 4 bytes for ID
	// Then the dynamic string data
	expectedFixedSize := 16 + 4 + 8 + 4
	if len(data) < expectedFixedSize {
		t.Errorf("Data too short: got %d bytes, expected at least %d", len(data), expectedFixedSize)
	}

	// Unmarshal and verify
	var decoded MixedStruct
	err = dynssz.UnmarshalSSZ(&decoded, data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Fixed-size strings will have null padding
	expectedFixedStr1 := test.FixedStr1 + string(make([]byte, 16-len(test.FixedStr1)))
	expectedFixedStr2 := test.FixedStr2 + string(make([]byte, 8-len(test.FixedStr2)))
	
	if decoded.FixedStr1 != expectedFixedStr1 {
		t.Errorf("FixedStr1 mismatch: got %q, want %q", decoded.FixedStr1, expectedFixedStr1)
	}
	if decoded.DynamicStr != test.DynamicStr {
		t.Errorf("DynamicStr mismatch: got %q, want %q", decoded.DynamicStr, test.DynamicStr)
	}
	if decoded.FixedStr2 != expectedFixedStr2 {
		t.Errorf("FixedStr2 mismatch: got %q, want %q", decoded.FixedStr2, expectedFixedStr2)
	}
	if decoded.ID != test.ID {
		t.Errorf("ID mismatch: got %d, want %d", decoded.ID, test.ID)
	}
}

// TestStringSliceVsByteSliceWithMaxSize verifies that []string and [][]byte
// have the same hash root when they have max size hints (proper SSZ lists)
func TestStringSliceVsByteSliceWithMaxSize(t *testing.T) {
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
			// They should encode identically
			strSliceEncoded, err := dynssz.MarshalSSZ(tc.strings)
			if err != nil {
				t.Fatalf("Failed to marshal []string: %v", err)
			}

			bytesSliceEncoded, err := dynssz.MarshalSSZ(tc.bytes)
			if err != nil {
				t.Fatalf("Failed to marshal [][]byte: %v", err)
			}

			if !bytes.Equal(strSliceEncoded, bytesSliceEncoded) {
				t.Errorf("[]string and [][]byte should encode identically")
				t.Logf("[]string encoded: %x", strSliceEncoded)
				t.Logf("[][]byte encoded: %x", bytesSliceEncoded)
			}

			// They should have identical hash roots
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

	// Also test with max size hints
	type WithMaxSize struct {
		StringList []string `ssz-max:"10"`
		BytesList  [][]byte `ssz-max:"10"`
	}

	test := WithMaxSize{
		StringList: []string{"hello", "world"},
		BytesList:  [][]byte{[]byte("hello"), []byte("world")},
	}

	strHash, err := dynssz.HashTreeRoot(test.StringList)
	if err != nil {
		t.Fatalf("Failed to hash string list with max: %v", err)
	}

	bytesHash, err := dynssz.HashTreeRoot(test.BytesList)
	if err != nil {
		t.Fatalf("Failed to hash bytes list with max: %v", err)
	}

	if strHash != bytesHash {
		t.Errorf("[]string and [][]byte with max size should have identical hash roots")
		t.Logf("[]string (max 10) hash: %x", strHash)
		t.Logf("[][]byte (max 10) hash: %x", bytesHash)
	}
}
