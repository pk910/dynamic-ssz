package dynssz

import (
	"bytes"
	"testing"
)

func TestMarshalSSZWriter(t *testing.T) {
	// Test simple struct with static fields
	type SimpleStruct struct {
		A uint32
		B uint64
		C bool
	}

	s := SimpleStruct{
		A: 42,
		B: 123456789,
		C: true,
	}

	ds := NewDynSsz(nil)
	
	// Marshal to buffer using writer
	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(s, &buf)
	if err != nil {
		t.Fatalf("MarshalSSZWriter failed: %v", err)
	}

	// Compare with regular marshal
	expected, err := ds.MarshalSSZ(s)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("Writer output differs from regular marshal\nGot:      %x\nExpected: %x", buf.Bytes(), expected)
	}
}

func TestMarshalSSZWriterDynamic(t *testing.T) {
	// Test struct with dynamic fields
	type DynamicStruct struct {
		A uint32
		B []uint32 `ssz-max:"10"`
		C uint16
		D []byte `ssz-max:"100"`
	}

	s := DynamicStruct{
		A: 100,
		B: []uint32{1, 2, 3, 4, 5},
		C: 200,
		D: []byte("hello world"),
	}

	ds := NewDynSsz(nil)

	// Marshal to buffer using writer
	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(s, &buf)
	if err != nil {
		t.Fatalf("MarshalSSZWriter failed: %v", err)
	}

	// Compare with regular marshal
	expected, err := ds.MarshalSSZ(s)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("Writer output differs from regular marshal\nGot:      %x\nExpected: %x", buf.Bytes(), expected)
	}
}

func TestMarshalSSZWriterNestedDynamic(t *testing.T) {
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

	o := Outer{
		A: 999,
		B: []Inner{
			{X: []uint16{1, 2}, Y: 10},
			{X: []uint16{3, 4, 5}, Y: 20},
		},
		C: []byte("test"),
	}

	ds := NewDynSsz(nil)

	// Marshal to buffer using writer
	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(o, &buf)
	if err != nil {
		t.Fatalf("MarshalSSZWriter failed: %v", err)
	}

	// Compare with regular marshal
	expected, err := ds.MarshalSSZ(o)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("Writer output differs from regular marshal\nGot:      %x\nExpected: %x", buf.Bytes(), expected)
	}
}