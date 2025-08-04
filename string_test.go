// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz_test

import (
	"bytes"
	"testing"

	. "github.com/pk910/dynamic-ssz"
)

func TestStringMarshaling(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected []byte
	}{
		{"empty string", "", []byte{}},
		{"simple string", "hello", []byte("hello")},
		{"unicode string", "hello 世界", []byte("hello 世界")},
		{"string with null bytes", "hello\x00world", []byte("hello\x00world")},
	}

	dynssz := NewDynSsz(nil)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := dynssz.MarshalSSZ(tc.value)
			if err != nil {
				t.Fatalf("MarshalSSZ failed: %v", err)
			}
			if !bytes.Equal(result, tc.expected) {
				t.Errorf("MarshalSSZ result mismatch: got %v, want %v", result, tc.expected)
			}
		})
	}
}

func TestStringUnmarshaling(t *testing.T) {
	testCases := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"empty string", []byte{}, ""},
		{"simple string", []byte("hello"), "hello"},
		{"unicode string", []byte("hello 世界"), "hello 世界"},
		{"string with null bytes", []byte("hello\x00world"), "hello\x00world"},
	}

	dynssz := NewDynSsz(nil)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result string
			err := dynssz.UnmarshalSSZ(&result, tc.data)
			if err != nil {
				t.Fatalf("UnmarshalSSZ failed: %v", err)
			}
			if result != tc.expected {
				t.Errorf("UnmarshalSSZ result mismatch: got %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestStringInStruct(t *testing.T) {
	type TestStruct struct {
		Name string
		Age  uint32
	}

	dynssz := NewDynSsz(nil)

	// Test marshaling
	original := TestStruct{Name: "Alice", Age: 30}
	data, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Test unmarshaling
	var decoded TestStruct
	err = dynssz.UnmarshalSSZ(&decoded, data)
	if err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}

	if decoded.Name != original.Name || decoded.Age != original.Age {
		t.Errorf("Round-trip failed: got %+v, want %+v", decoded, original)
	}
}

func TestStringHashTreeRoot(t *testing.T) {
	testCases := []struct {
		name   string
		value  string
		// Note: These expected values would need to be calculated based on SSZ spec
		// For now, we just ensure it doesn't error
	}{
		{"empty string", ""},
		{"simple string", "hello"},
		{"unicode string", "hello 世界"},
	}

	dynssz := NewDynSsz(nil)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := dynssz.HashTreeRoot(tc.value)
			if err != nil {
				t.Fatalf("HashTreeRoot failed: %v", err)
			}
			// Just ensure we got a 32-byte root
			if len(root) != 32 {
				t.Errorf("HashTreeRoot returned wrong size: got %d bytes, want 32", len(root))
			}
		})
	}
}

func TestStringSlice(t *testing.T) {
	dynssz := NewDynSsz(nil)

	// Test slice of strings
	original := []string{"hello", "world", "test"}
	data, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	var decoded []string
	err = dynssz.UnmarshalSSZ(&decoded, data)
	if err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}

	if len(decoded) != len(original) {
		t.Fatalf("Length mismatch: got %d, want %d", len(decoded), len(original))
	}

	for i, v := range decoded {
		if v != original[i] {
			t.Errorf("Element %d mismatch: got %q, want %q", i, v, original[i])
		}
	}
}

func TestStringArray(t *testing.T) {
	dynssz := NewDynSsz(nil)

	// Test array of strings
	original := [3]string{"one", "two", "three"}
	data, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	var decoded [3]string
	err = dynssz.UnmarshalSSZ(&decoded, data)
	if err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}

	for i, v := range decoded {
		if v != original[i] {
			t.Errorf("Element %d mismatch: got %q, want %q", i, v, original[i])
		}
	}
}

func TestStringSizeSSZ(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected int
	}{
		{"empty string", "", 0},
		{"simple string", "hello", 5},
		{"unicode string", "hello 世界", len("hello 世界")},
	}

	dynssz := NewDynSsz(nil)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			size, err := dynssz.SizeSSZ(tc.value)
			if err != nil {
				t.Fatalf("SizeSSZ failed: %v", err)
			}
			if size != tc.expected {
				t.Errorf("SizeSSZ mismatch: got %d, want %d", size, tc.expected)
			}
		})
	}
}