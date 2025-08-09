package dynssz

import (
	"bytes"
	"io"
	"strings"
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

// Test comprehensive payload matrix using streaming writer
func TestMarshalSSZWriterMatrix(t *testing.T) {
	// Import test matrix from marshal_test.go
	var testMatrix = []struct {
		name     string
		payload  any
		expected []byte
	}{
		// primitive types
		{"bool_false", bool(false), fromHex("0x00")},
		{"bool_true", bool(true), fromHex("0x01")},
		{"uint8_zero", uint8(0), fromHex("0x00")},
		{"uint8_max", uint8(255), fromHex("0xff")},
		{"uint8_mid", uint8(42), fromHex("0x2a")},
		{"uint16_zero", uint16(0), fromHex("0x0000")},
		{"uint16_max", uint16(65535), fromHex("0xffff")},
		{"uint16_mid", uint16(1337), fromHex("0x3905")},
		{"uint32_zero", uint32(0), fromHex("0x00000000")},
		{"uint32_max", uint32(4294967295), fromHex("0xffffffff")},
		{"uint32_mid", uint32(817482215), fromHex("0xe7c9b930")},
		{"uint64_zero", uint64(0), fromHex("0x0000000000000000")},
		{"uint64_max", uint64(18446744073709551615), fromHex("0xffffffffffffffff")},
		{"uint64_mid", uint64(848028848028), fromHex("0x9c4f7572c5000000")},

		// arrays & slices
		{"empty_slice", []uint8{}, fromHex("0x")},
		{"uint8_slice", []uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405")},
		{"uint8_array", [5]uint8{1, 2, 3, 4, 5}, fromHex("0x0102030405")},
		{"uint8_array_partial", [10]uint8{1, 2, 3, 4, 5}, fromHex("0x01020304050000000000")},

		// complex types
		{
			"complex_struct",
			struct {
				F1 bool
				F2 uint8
				F3 uint16
				F4 uint32
				F5 uint64
			}{true, 1, 2, 3, 4},
			fromHex("0x01010200030000000400000000000000"),
		},
		{
			"dynamic_struct",
			struct {
				F1 bool
				F2 []uint8  `ssz-max:"10"`
				F3 []uint16 `ssz-max:"10"`
				F4 uint32
			}{true, []uint8{1, 1, 1, 1}, []uint16{2, 2, 2, 2, 2}, 3},
			fromHex("0x010d00000011000000030000000101010102000200020002000200"),
		},

		// string types
		{
			"empty_string",
			struct {
				Data string `ssz-max:"100"`
			}{""},
			fromHex("0x04000000"),
		},
		{
			"hello_string",
			struct {
				Data string `ssz-max:"100"`
			}{"hello"},
			fromHex("0x0400000068656c6c6f"),
		},
		{
			"unicode_string",
			struct {
				Data string `ssz-max:"100"`
			}{"hello 世界"},
			fromHex("0x0400000068656c6c6f20e4b896e7958c"),
		},
		{
			"fixed_string",
			struct {
				Data string `ssz-size:"32"`
			}{"hello"},
			fromHex("0x68656c6c6f000000000000000000000000000000000000000000000000000000"),
		},
	}

	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.NoByteSliceOptimization = true

	for _, test := range testMatrix {
		t.Run(test.name, func(t *testing.T) {
			// Test with writer
			var buf bytes.Buffer
			err := dynssz.MarshalSSZWriter(test.payload, &buf)

			switch {
			case test.expected == nil && err != nil:
				// expected error
			case err != nil:
				t.Errorf("MarshalSSZWriter error: %v", err)
			case !bytes.Equal(buf.Bytes(), test.expected):
				t.Errorf("MarshalSSZWriter failed: got %s, wanted %s", toHex(buf.Bytes()), toHex(test.expected))
			}

			// Compare with regular marshal
			if test.expected != nil {
				regularResult, err := dynssz.MarshalSSZ(test.payload)
				if err != nil {
					t.Errorf("MarshalSSZ error: %v", err)
				} else if !bytes.Equal(buf.Bytes(), regularResult) {
					t.Errorf("Writer result differs from regular marshal\nWriter: %x\nRegular: %x", buf.Bytes(), regularResult)
				}
			}
		})
	}
}

// Test arrays and slices with dynamic/static complex items
func TestMarshalSSZWriterComplexArraysSlices(t *testing.T) {
	dynssz := NewDynSsz(map[string]any{
		"MAX_ELEMENTS": uint64(5),
		"BUFFER_SIZE":  uint64(100),
	})
	dynssz.NoFastSsz = true
	dynssz.NoByteSliceOptimization = true

	// Test array with dynamic items
	t.Run("array_with_dynamic_items", func(t *testing.T) {
		type DynamicItem struct {
			Data []byte `ssz-max:"10"`
			ID   uint32
		}
		
		type ArrayContainer struct {
			Items [3]DynamicItem
		}

		payload := ArrayContainer{
			Items: [3]DynamicItem{
				{Data: []byte("hello"), ID: 1},
				{Data: []byte("world"), ID: 2},
				{Data: []byte("test"), ID: 3},
			},
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		// Compare with regular marshal
		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})

	// Test array with static items
	t.Run("array_with_static_items", func(t *testing.T) {
		type StaticItem struct {
			ID   uint32
			Flag bool
		}
		
		type ArrayContainer struct {
			Items [4]StaticItem
		}

		payload := ArrayContainer{
			Items: [4]StaticItem{
				{ID: 1, Flag: true},
				{ID: 2, Flag: false},
				{ID: 3, Flag: true},
				{ID: 4, Flag: false},
			},
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})

	// Test slice with dynamic items
	t.Run("slice_with_dynamic_items", func(t *testing.T) {
		type DynamicItem struct {
			Name string `ssz-max:"20"`
			Tags []uint16 `ssz-max:"5"`
		}
		
		type SliceContainer struct {
			Items []DynamicItem `ssz-max:"3"`
		}

		payload := SliceContainer{
			Items: []DynamicItem{
				{Name: "item1", Tags: []uint16{1, 2}},
				{Name: "item2", Tags: []uint16{3, 4, 5}},
			},
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})

	// Test slice with dynamic spec values
	t.Run("slice_with_spec_values", func(t *testing.T) {
		type SpecItem struct {
			Data []byte `dynssz-max:"BUFFER_SIZE"`
		}
		
		type SpecContainer struct {
			Items []SpecItem `dynssz-max:"MAX_ELEMENTS"`
		}

		payload := SpecContainer{
			Items: []SpecItem{
				{Data: []byte("spec test 1")},
				{Data: []byte("spec test 2")},
			},
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})
}

// Test pointer scenarios and edge cases
func TestMarshalSSZWriterPointers(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.NoByteSliceOptimization = true

	// Test simple pointer handling
	t.Run("simple_pointers", func(t *testing.T) {
		type PointerStruct struct {
			Count *uint32
			Flag  *bool
		}

		count := uint32(42)
		flag := true
		payload := PointerStruct{
			Count: &count,
			Flag:  &flag,
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})

	// Test struct with pointer field
	t.Run("struct_with_pointers", func(t *testing.T) {
		type InnerStruct struct {
			Value uint32
		}
		
		type OuterStruct struct {
			Inner *InnerStruct
			Data  []byte `ssz-max:"10"`
		}

		payload := OuterStruct{
			Inner: &InnerStruct{Value: 123},
			Data:  []byte("test"),
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})

	// Test nil pointer handling
	t.Run("nil_pointers", func(t *testing.T) {
		type NilStruct struct {
			Value *uint32
		}

		payload := NilStruct{Value: nil}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})
}

// Test edge cases with ssz-size constraints
func TestMarshalSSZWriterEdgeCases(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.NoByteSliceOptimization = true

	// Test slice with ssz-size but too few items
	t.Run("slice_too_few_items", func(t *testing.T) {
		type ConstrainedStruct struct {
			Items []uint32 `ssz-size:"5"`
		}

		payload := ConstrainedStruct{
			Items: []uint32{1, 2, 3}, // Only 3 items, expects 5
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		// This should succeed and pad with zeros
		if err != nil {
			t.Errorf("Expected success for too few items, got: %v", err)
		}

		expected, err := dynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})

	// Test slice with ssz-size but too many items (should error)
	t.Run("slice_too_many_items", func(t *testing.T) {
		type ConstrainedStruct struct {
			Items []uint32 `ssz-size:"3"`
		}

		payload := ConstrainedStruct{
			Items: []uint32{1, 2, 3, 4, 5}, // 5 items, expects 3
		}

		var buf bytes.Buffer
		err := dynssz.MarshalSSZWriter(payload, &buf)
		if err == nil {
			t.Error("Expected error for too many items but got nil")
		}
	})

	// Test with dynamic size using spec values
	t.Run("dynamic_size_with_spec", func(t *testing.T) {
		specDynssz := NewDynSsz(map[string]any{
			"CUSTOM_SIZE": uint64(4),
		})
		specDynssz.NoFastSsz = true
		specDynssz.NoByteSliceOptimization = true

		type SpecStruct struct {
			Data []byte `dynssz-size:"CUSTOM_SIZE"`
		}

		payload := SpecStruct{
			Data: []byte{1, 2, 3, 4},
		}

		var buf bytes.Buffer
		err := specDynssz.MarshalSSZWriter(payload, &buf)
		if err != nil {
			t.Errorf("MarshalSSZWriter failed: %v", err)
		}

		expected, err := specDynssz.MarshalSSZ(payload)
		if err != nil {
			t.Errorf("Regular marshal failed: %v", err)
		} else if !bytes.Equal(buf.Bytes(), expected) {
			t.Errorf("Results differ:\nWriter: %s\nRegular: %s", toHex(buf.Bytes()), toHex(expected))
		}
	})
}

// Test streaming writer with limited writers and error conditions
func TestMarshalSSZWriterLimitedWriter(t *testing.T) {
	type TestStruct struct {
		A uint32
		B []uint32 `ssz-max:"10"`
		C string   `ssz-max:"50"`
	}

	payload := TestStruct{
		A: 42,
		B: []uint32{1, 2, 3, 4, 5},
		C: "hello world",
	}

	dynssz := NewDynSsz(nil)

	t.Run("sufficient_buffer", func(t *testing.T) {
		buf := make([]byte, 100)
		writer := bytes.NewBuffer(buf[:0])
		
		err := dynssz.MarshalSSZWriter(payload, writer)
		if err != nil {
			t.Errorf("Expected success but got error: %v", err)
		}
	})

	t.Run("write_error", func(t *testing.T) {
		writer := &failingWriter{failAt: 10}
		
		err := dynssz.MarshalSSZWriter(payload, writer)
		if err == nil {
			t.Error("Expected write error but got nil")
		}
		if !strings.Contains(err.Error(), "short write") {
			t.Errorf("Expected write error message, got: %v", err)
		}
	})
}

// Test primitive write functions
func TestWritePrimitives(t *testing.T) {
	tests := []struct {
		name     string
		writeFunc func(io.Writer, interface{}) error
		value    interface{}
		expected []byte
	}{
		{"write_bool_false", func(w io.Writer, v interface{}) error { return writeBool(w, v.(bool)) }, false, []byte{0x00}},
		{"write_bool_true", func(w io.Writer, v interface{}) error { return writeBool(w, v.(bool)) }, true, []byte{0x01}},
		{"write_uint8_zero", func(w io.Writer, v interface{}) error { return writeUint8(w, v.(uint8)) }, uint8(0), []byte{0x00}},
		{"write_uint8_max", func(w io.Writer, v interface{}) error { return writeUint8(w, v.(uint8)) }, uint8(255), []byte{0xff}},
		{"write_uint16_zero", func(w io.Writer, v interface{}) error { return writeUint16(w, v.(uint16)) }, uint16(0), []byte{0x00, 0x00}},
		{"write_uint16_max", func(w io.Writer, v interface{}) error { return writeUint16(w, v.(uint16)) }, uint16(65535), []byte{0xff, 0xff}},
		{"write_uint32_zero", func(w io.Writer, v interface{}) error { return writeUint32(w, v.(uint32)) }, uint32(0), []byte{0x00, 0x00, 0x00, 0x00}},
		{"write_uint32_max", func(w io.Writer, v interface{}) error { return writeUint32(w, v.(uint32)) }, uint32(4294967295), []byte{0xff, 0xff, 0xff, 0xff}},
		{"write_uint64_zero", func(w io.Writer, v interface{}) error { return writeUint64(w, v.(uint64)) }, uint64(0), []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{"write_uint64_max", func(w io.Writer, v interface{}) error { return writeUint64(w, v.(uint64)) }, uint64(18446744073709551615), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := test.writeFunc(&buf, test.value)
			if err != nil {
				t.Errorf("Write function failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), test.expected) {
				t.Errorf("Write result mismatch: got %s, expected %s", toHex(buf.Bytes()), toHex(test.expected))
			}
		})
	}
}

// Test error conditions for primitive writes
func TestWritePrimitivesErrors(t *testing.T) {
	writer := &failingWriter{failAt: 0}
	
	tests := []struct {
		name string
		writeFunc func() error
	}{
		{"write_bool_error", func() error { return writeBool(writer, true) }},
		{"write_uint8_error", func() error { return writeUint8(writer, 42) }},
		{"write_uint16_error", func() error { return writeUint16(writer, 1337) }},
		{"write_uint32_error", func() error { return writeUint32(writer, 123456) }},
		{"write_uint64_error", func() error { return writeUint64(writer, 123456789) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.writeFunc()
			if err == nil {
				t.Error("Expected error but got nil")
			}
			if !strings.Contains(err.Error(), "short write") {
				t.Errorf("Expected write error message, got: %v", err)
			}
		})
	}
}

// Helper types and functions
type failingWriter struct {
	written int
	failAt  int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.written >= w.failAt {
		return 0, io.ErrShortWrite
	}
	toWrite := len(p)
	if w.written + toWrite > w.failAt {
		toWrite = w.failAt - w.written
	}
	w.written += toWrite
	if toWrite < len(p) {
		return toWrite, io.ErrShortWrite
	}
	return toWrite, nil
}

func fromHex(hexStr string) []byte {
	if len(hexStr) < 2 || hexStr[:2] != "0x" {
		return nil
	}
	hexStr = hexStr[2:]
	if len(hexStr) == 0 {
		return []byte{}
	}
	
	result := make([]byte, len(hexStr)/2)
	for i := 0; i < len(result); i++ {
		var b byte
		for j := 0; j < 2; j++ {
			c := hexStr[i*2+j]
			var v byte
			if c >= '0' && c <= '9' {
				v = c - '0'
			} else if c >= 'a' && c <= 'f' {
				v = c - 'a' + 10
			} else if c >= 'A' && c <= 'F' {
				v = c - 'A' + 10
			}
			b = b*16 + v
		}
		result[i] = b
	}
	return result
}

func toHex(data []byte) string {
	if len(data) == 0 {
		return "0x"
	}
	hexStr := make([]byte, len(data)*2+2)
	hexStr[0] = '0'
	hexStr[1] = 'x'
	
	for i, b := range data {
		hexStr[i*2+2] = "0123456789abcdef"[b>>4]
		hexStr[i*2+3] = "0123456789abcdef"[b&0xf]
	}
	return string(hexStr)
}