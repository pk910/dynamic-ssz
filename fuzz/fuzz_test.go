// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package fuzz

import (
	"bytes"
	"testing"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
)

// Test structures for fuzzing
type SimpleStruct struct {
	A uint64
	B uint32
	C uint16
	D uint8
	E bool
}

type ComplexStruct struct {
	Numbers   []uint64  `ssz-max:"1024"`
	Bytes     []byte    `ssz-max:"2048"`
	Uint128   [16]byte  `ssz-type:"uint128"`
	Uint256   [4]uint64 `ssz-type:"uint256"`
	Nested    SimpleStruct
	NestedPtr *SimpleStruct
	Arrays    [4]uint32
}

type ComplexUints struct {
	Uint128 [16]byte  `ssz-type:"uint128"`
	Uint256 [4]uint64 `ssz-type:"uint256"`
}

type ByteArray32 [32]byte

type VariableStruct struct {
	List1 []uint64      `ssz-max:"100"`
	List2 []ByteArray32 `ssz-max:"50"`
	Data  []byte        `ssz-max:"1000"`
}

type NestedComplex struct {
	Arrays [4][]uint16
}

// FuzzMarshalUnmarshal tests marshal/unmarshal operations
func FuzzMarshalUnmarshal(f *testing.F) {
	// Add seed corpus
	f.Add(int64(1))
	f.Add(int64(42))
	f.Add(time.Now().UnixNano())

	f.Fuzz(func(t *testing.T, seed int64) {
		fuzzer := NewFuzzer(seed)

		testCases := []interface{}{
			&SimpleStruct{},
			&ComplexStruct{},
			&VariableStruct{},
			&ByteArray32{},
			&ComplexUints{},
			&NestedComplex{},
		}

		for _, testCase := range testCases {
			// Generate random data
			fuzzer.FuzzValue(testCase)

			// Test marshal/unmarshal roundtrip
			err := fuzzer.FuzzMarshalUnmarshal(testCase)
			if err != nil {
				t.Errorf("FuzzMarshalUnmarshal failed for %T: %v", testCase, err)
			}
		}
	})
}

// FuzzSize tests size calculation operations
func FuzzSize(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(42))
	f.Add(time.Now().UnixNano())

	f.Fuzz(func(t *testing.T, seed int64) {
		fuzzer := NewFuzzer(seed)

		testCases := []interface{}{
			&SimpleStruct{},
			&ComplexStruct{},
			&VariableStruct{},
			&ByteArray32{},
			&ComplexUints{},
			&NestedComplex{},
		}

		for _, testCase := range testCases {
			fuzzer.FuzzValue(testCase)

			err := fuzzer.FuzzSize(testCase)
			if err != nil {
				t.Errorf("FuzzSize failed for %T: %v", testCase, err)
			}
		}
	})
}

// FuzzHashTreeRoot tests hash tree root calculation operations
func FuzzHashTreeRoot(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(42))
	f.Add(time.Now().UnixNano())

	f.Fuzz(func(t *testing.T, seed int64) {
		fuzzer := NewFuzzer(seed)

		testCases := []interface{}{
			&SimpleStruct{},
			&ComplexStruct{},
			&VariableStruct{},
			&ByteArray32{},
			&ComplexUints{},
			&NestedComplex{},
		}

		for _, testCase := range testCases {
			fuzzer.FuzzValue(testCase)

			err := fuzzer.FuzzHashTreeRoot(testCase)
			if err != nil {
				t.Errorf("FuzzHashTreeRoot failed for %T: %v", testCase, err)
			}
		}
	})
}

// Test individual operations with fuzzing
func TestFuzzerBasic(t *testing.T) {
	fuzzer := NewFuzzer(42)
	ds := dynssz.NewDynSsz(nil)

	// Test simple struct
	simple := &SimpleStruct{}
	fuzzer.FuzzValue(simple)

	// Test marshal/unmarshal
	marshaled, err := ds.MarshalSSZ(simple)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	unmarshaled := &SimpleStruct{}
	err = ds.UnmarshalSSZ(unmarshaled, marshaled)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Test size calculation
	size, err := ds.SizeSSZ(simple)
	if err != nil {
		t.Fatalf("Size calculation failed: %v", err)
	}

	if size != len(marshaled) {
		t.Errorf("Size mismatch: calculated %d, actual %d", size, len(marshaled))
	}

	// Test hash tree root
	_, err = ds.HashTreeRoot(simple)
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	remarshaled, err := ds.MarshalSSZ(simple)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if !bytes.Equal(marshaled, remarshaled) {
		t.Errorf("Marshal mismatch: %x != %x", marshaled, remarshaled)
	}
}

// Benchmark fuzzing operations
func BenchmarkFuzzMarshalUnmarshal(b *testing.B) {
	fuzzer := NewFuzzer(42)
	testStruct := &ComplexStruct{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzer.FuzzValue(testStruct)
		err := fuzzer.FuzzMarshalUnmarshal(testStruct)
		if err != nil {
			b.Fatalf("FuzzMarshalUnmarshal failed: %v", err)
		}
	}
}

func BenchmarkFuzzSize(b *testing.B) {
	fuzzer := NewFuzzer(42)
	testStruct := &ComplexStruct{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzer.FuzzValue(testStruct)
		err := fuzzer.FuzzSize(testStruct)
		if err != nil {
			b.Fatalf("FuzzSize failed: %v", err)
		}
	}
}

func BenchmarkFuzzHashTreeRoot(b *testing.B) {
	fuzzer := NewFuzzer(42)
	testStruct := &ComplexStruct{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fuzzer.FuzzValue(testStruct)
		err := fuzzer.FuzzHashTreeRoot(testStruct)
		if err != nil {
			b.Fatalf("FuzzHashTreeRoot failed: %v", err)
		}
	}
}
