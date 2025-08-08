package dynssz

import (
	"bytes"
	"testing"
)

func TestUnmarshalSSZReader(t *testing.T) {
	// Test simple struct with static fields
	type SimpleStruct struct {
		A uint32
		B uint64
		C bool
	}

	original := SimpleStruct{
		A: 42,
		B: 123456789,
		C: true,
	}

	ds := NewDynSsz(nil)

	// Marshal to get test data
	data, err := ds.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Unmarshal using reader
	var result SimpleStruct
	reader := bytes.NewReader(data)
	err = ds.UnmarshalSSZReader(&result, reader, int64(len(data)))
	if err != nil {
		t.Fatalf("UnmarshalSSZReader failed: %v", err)
	}

	// Compare results
	if result != original {
		t.Errorf("Unmarshaled struct differs from original\nGot:      %+v\nExpected: %+v", result, original)
	}
}

func TestUnmarshalSSZReaderDynamic(t *testing.T) {
	// Test struct with dynamic fields
	type DynamicStruct struct {
		A uint32
		B []uint32 `ssz-max:"10"`
		C uint16
		D []byte `ssz-max:"100"`
	}

	original := DynamicStruct{
		A: 100,
		B: []uint32{1, 2, 3, 4, 5},
		C: 200,
		D: []byte("hello world"),
	}

	ds := NewDynSsz(nil)

	// Marshal to get test data
	data, err := ds.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Unmarshal using reader
	var result DynamicStruct
	reader := bytes.NewReader(data)
	err = ds.UnmarshalSSZReader(&result, reader, int64(len(data)))
	if err != nil {
		t.Fatalf("UnmarshalSSZReader failed: %v", err)
	}

	// Compare results
	if result.A != original.A || result.C != original.C {
		t.Errorf("Static fields differ: got A=%d, C=%d, expected A=%d, C=%d", 
			result.A, result.C, original.A, original.C)
	}

	if len(result.B) != len(original.B) {
		t.Errorf("Slice B length differs: got %d, expected %d", len(result.B), len(original.B))
	} else {
		for i, v := range result.B {
			if v != original.B[i] {
				t.Errorf("Slice B[%d] differs: got %d, expected %d", i, v, original.B[i])
			}
		}
	}

	if !bytes.Equal(result.D, original.D) {
		t.Errorf("Byte slice D differs: got %v, expected %v", result.D, original.D)
	}
}

func TestUnmarshalSSZReaderNestedDynamic(t *testing.T) {
	// Test nested dynamic structures
	type Inner struct {
		X []uint16 `ssz-max:"5"`
		Y uint32
	}

	type Outer struct {
		A uint64
		B []Inner `ssz-max:"3"`
		C []byte  `ssz-max:"20"`
	}

	original := Outer{
		A: 999,
		B: []Inner{
			{X: []uint16{1, 2}, Y: 10},
			{X: []uint16{3, 4, 5}, Y: 20},
		},
		C: []byte("test"),
	}

	ds := NewDynSsz(nil)

	// Marshal to get test data
	data, err := ds.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Unmarshal using reader
	var result Outer
	reader := bytes.NewReader(data)
	err = ds.UnmarshalSSZReader(&result, reader, int64(len(data)))
	if err != nil {
		t.Fatalf("UnmarshalSSZReader failed: %v", err)
	}

	// Compare results
	if result.A != original.A {
		t.Errorf("Field A differs: got %d, expected %d", result.A, original.A)
	}

	if len(result.B) != len(original.B) {
		t.Errorf("Slice B length differs: got %d, expected %d", len(result.B), len(original.B))
	} else {
		for i, inner := range result.B {
			origInner := original.B[i]
			if inner.Y != origInner.Y {
				t.Errorf("Inner[%d].Y differs: got %d, expected %d", i, inner.Y, origInner.Y)
			}
			if len(inner.X) != len(origInner.X) {
				t.Errorf("Inner[%d].X length differs: got %d, expected %d", i, len(inner.X), len(origInner.X))
			} else {
				for j, x := range inner.X {
					if x != origInner.X[j] {
						t.Errorf("Inner[%d].X[%d] differs: got %d, expected %d", i, j, x, origInner.X[j])
					}
				}
			}
		}
	}

	if !bytes.Equal(result.C, original.C) {
		t.Errorf("Byte slice C differs: got %v, expected %v", result.C, original.C)
	}
}

func TestUnmarshalSSZReaderByteSlice(t *testing.T) {
	// Test byte slice handling
	original := []byte("hello, world! this is a test of byte slices")

	ds := NewDynSsz(nil)

	// Marshal to get test data
	data, err := ds.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Unmarshal using reader
	var result []byte
	reader := bytes.NewReader(data)
	err = ds.UnmarshalSSZReader(&result, reader, int64(len(data)))
	if err != nil {
		t.Fatalf("UnmarshalSSZReader failed: %v", err)
	}

	// Compare results
	if !bytes.Equal(result, original) {
		t.Errorf("Byte slice differs\nGot:      %v\nExpected: %v", result, original)
	}
}