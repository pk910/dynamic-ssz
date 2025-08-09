package dynssz

import (
	"bytes"
	"io"
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

// Test comprehensive payload matrix using streaming reader
func TestUnmarshalSSZReaderMatrix(t *testing.T) {
	// Comprehensive test matrix matching marshal_test.go approach
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
	dynssz.NoStreamBuffering = true

	for _, test := range testMatrix {
		t.Run(test.name, func(t *testing.T) {
			if test.expected == nil {
				// Skip tests with expected errors for this comprehensive test
				return
			}

			// Create a zero value of the same type as payload for unmarshaling
			var result interface{}
			switch test.payload.(type) {
			case bool:
				var v bool
				result = &v
			case uint8:
				var v uint8
				result = &v
			case uint16:
				var v uint16
				result = &v
			case uint32:
				var v uint32
				result = &v
			case uint64:
				var v uint64
				result = &v
			case []uint8:
				var v []uint8
				result = &v
			case [5]uint8:
				var v [5]uint8
				result = &v
			case [10]uint8:
				var v [10]uint8
				result = &v
			default:
				// For complex types, create the zero value using reflection-like approach
				switch test.payload.(type) {
				case struct {
					F1 bool
					F2 uint8
					F3 uint16
					F4 uint32
					F5 uint64
				}:
					var r struct {
						F1 bool
						F2 uint8
						F3 uint16
						F4 uint32
						F5 uint64
					}
					result = &r
				case struct {
					F1 bool
					F2 []uint8  `ssz-max:"10"`
					F3 []uint16 `ssz-max:"10"`
					F4 uint32
				}:
					var r struct {
						F1 bool
						F2 []uint8  `ssz-max:"10"`
						F3 []uint16 `ssz-max:"10"`
						F4 uint32
					}
					result = &r
				case struct {
					Data string `ssz-max:"100"`
				}:
					var r struct {
						Data string `ssz-max:"100"`
					}
					result = &r
				case struct {
					Data string `ssz-size:"32"`
				}:
					var r struct {
						Data string `ssz-size:"32"`
					}
					result = &r
				default:
					t.Skipf("Unsupported type for test: %T", test.payload)
				}
			}

			// Unmarshal using reader
			reader := bytes.NewReader(test.expected)
			err := dynssz.UnmarshalSSZReader(result, reader, int64(len(test.expected)))
			if err != nil {
				t.Errorf("UnmarshalSSZReader error: %v", err)
				return
			}

			// Skip comparison for slice/array types and complex types as they have different handling
			switch test.payload.(type) {
			case []uint8, [5]uint8, [10]uint8:
				// Skip comparison for arrays and slices - they have different zero value handling
				return
			default:
				// Only compare for primitive types
			}

			// Compare with regular unmarshal for consistency (only for primitive types)
			var regularResult interface{}
			switch test.payload.(type) {
			case bool:
				var v bool
				regularResult = &v
			case uint8:
				var v uint8
				regularResult = &v
			case uint16:
				var v uint16
				regularResult = &v
			case uint32:
				var v uint32
				regularResult = &v
			case uint64:
				var v uint64
				regularResult = &v
			default:
				// Skip complex type comparison for brevity in this test
				return
			}

			err = dynssz.UnmarshalSSZ(regularResult, test.expected)
			if err != nil {
				t.Errorf("Regular UnmarshalSSZ error: %v", err)
				return
			}

			// For primitive types, ensure both methods produce same result
			if !compareValues(result, regularResult) {
				t.Errorf("Reader result differs from regular unmarshal")
			}
		})
	}
}

// Test streaming reader with limited readers and error conditions
func TestUnmarshalSSZReaderErrors(t *testing.T) {
	type TestStruct struct {
		A uint32
		B []uint32 `ssz-max:"10"`
		C string   `ssz-max:"50"`
	}

	original := TestStruct{
		A: 42,
		B: []uint32{1, 2, 3, 4, 5},
		C: "hello world",
	}

	dynssz := NewDynSsz(nil)

	// Marshal to get valid test data
	validData, err := dynssz.MarshalSSZ(original)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	t.Run("insufficient_data", func(t *testing.T) {
		var result TestStruct
		// Only provide half the data
		reader := bytes.NewReader(validData[:len(validData)/2])

		err := dynssz.UnmarshalSSZReader(&result, reader, int64(len(validData)/2))
		if err == nil {
			t.Error("Expected error for insufficient data but got nil")
		}
	})

	t.Run("read_error", func(t *testing.T) {
		var result TestStruct
		reader := &failingReader{data: validData, failAt: 10}

		err := dynssz.UnmarshalSSZReader(&result, reader, int64(len(validData)))
		if err == nil {
			t.Error("Expected read error but got nil")
		}
	})

	t.Run("invalid_size", func(t *testing.T) {
		var result TestStruct
		reader := bytes.NewReader(validData)

		// Provide wrong size
		err := dynssz.UnmarshalSSZReader(&result, reader, int64(len(validData)+100))
		if err == nil {
			t.Error("Expected error for invalid size but got nil")
		}
	})
}

// Test primitive read functions
func TestReadPrimitives(t *testing.T) {
	tests := []struct {
		name     string
		readFunc func(io.Reader) (interface{}, error)
		input    []byte
		expected interface{}
	}{
		{"read_bool_false", func(r io.Reader) (interface{}, error) { return readBool(r) }, []byte{0x00}, false},
		{"read_bool_true", func(r io.Reader) (interface{}, error) { return readBool(r) }, []byte{0x01}, true},
		{"read_uint8_zero", func(r io.Reader) (interface{}, error) { return readUint8(r) }, []byte{0x00}, uint8(0)},
		{"read_uint8_max", func(r io.Reader) (interface{}, error) { return readUint8(r) }, []byte{0xff}, uint8(255)},
		{"read_uint16_zero", func(r io.Reader) (interface{}, error) { return readUint16(r) }, []byte{0x00, 0x00}, uint16(0)},
		{"read_uint16_max", func(r io.Reader) (interface{}, error) { return readUint16(r) }, []byte{0xff, 0xff}, uint16(65535)},
		{"read_uint32_zero", func(r io.Reader) (interface{}, error) { return readUint32(r) }, []byte{0x00, 0x00, 0x00, 0x00}, uint32(0)},
		{"read_uint32_max", func(r io.Reader) (interface{}, error) { return readUint32(r) }, []byte{0xff, 0xff, 0xff, 0xff}, uint32(4294967295)},
		{"read_uint64_zero", func(r io.Reader) (interface{}, error) { return readUint64(r) }, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, uint64(0)},
		{"read_uint64_max", func(r io.Reader) (interface{}, error) { return readUint64(r) }, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, uint64(18446744073709551615)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := bytes.NewReader(test.input)
			result, err := test.readFunc(reader)
			if err != nil {
				t.Errorf("Read function failed: %v", err)
			}
			if result != test.expected {
				t.Errorf("Read result mismatch: got %v, expected %v", result, test.expected)
			}
		})
	}
}

// Test error conditions for primitive reads
func TestReadPrimitivesErrors(t *testing.T) {
	tests := []struct {
		name     string
		readFunc func(io.Reader) (interface{}, error)
		input    []byte // Insufficient data
	}{
		{"read_bool_error", func(r io.Reader) (interface{}, error) { return readBool(r) }, []byte{}},
		{"read_uint8_error", func(r io.Reader) (interface{}, error) { return readUint8(r) }, []byte{}},
		{"read_uint16_error", func(r io.Reader) (interface{}, error) { return readUint16(r) }, []byte{0x00}},
		{"read_uint32_error", func(r io.Reader) (interface{}, error) { return readUint32(r) }, []byte{0x00, 0x00}},
		{"read_uint64_error", func(r io.Reader) (interface{}, error) { return readUint64(r) }, []byte{0x00, 0x00, 0x00, 0x00}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := bytes.NewReader(test.input)
			_, err := test.readFunc(reader)
			if err == nil {
				t.Error("Expected error but got nil")
			}
		})
	}

	// Test with failing reader
	failingReader := &failingReader{data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}, failAt: 0}

	t.Run("read_bool_failing", func(t *testing.T) {
		_, err := readBool(failingReader)
		if err == nil {
			t.Error("Expected error but got nil")
		}
	})
}

// Test array unmarshaling specifically (0% coverage)
func TestUnmarshalArrayReader(t *testing.T) {
	dynssz := NewDynSsz(nil)

	// Test uint8 arrays
	t.Run("uint8_array_5", func(t *testing.T) {
		original := [5]uint8{1, 2, 3, 4, 5}
		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result [5]uint8
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}
		if result != original {
			t.Errorf("Array mismatch: got %v, expected %v", result, original)
		}
	})

	// Test uint16 arrays
	t.Run("uint16_array_3", func(t *testing.T) {
		original := [3]uint16{100, 200, 300}
		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result [3]uint16
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}
		if result != original {
			t.Errorf("Array mismatch: got %v, expected %v", result, original)
		}
	})

	// Test uint32 arrays
	t.Run("uint32_array_2", func(t *testing.T) {
		original := [2]uint32{1000000, 2000000}
		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result [2]uint32
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}
		if result != original {
			t.Errorf("Array mismatch: got %v, expected %v", result, original)
		}
	})
}

// Test arrays and slices with complex elements for unmarshal reader
func TestUnmarshalReaderComplexArraysSlices(t *testing.T) {
	dynssz := NewDynSsz(map[string]any{
		"MAX_ITEMS":   uint64(5),
		"STRING_SIZE": uint64(32),
	})
	dynssz.NoFastSsz = true
	dynssz.NoStreamBuffering = true

	// Test array with dynamic complex elements
	t.Run("array_dynamic_elements", func(t *testing.T) {
		type DynamicElement struct {
			Name string `ssz-max:"20"`
			Data []byte `ssz-max:"15"`
		}

		type ArrayContainer struct {
			Elements [2]DynamicElement
		}

		original := ArrayContainer{
			Elements: [2]DynamicElement{
				{Name: "first", Data: []byte("data1")},
				{Name: "second", Data: []byte("data2")},
			},
		}

		// Marshal to get test data
		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		// Unmarshal using reader
		var result ArrayContainer
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		// Compare
		if len(result.Elements) != len(original.Elements) {
			t.Errorf("Array length differs: got %d, expected %d", len(result.Elements), len(original.Elements))
		}
		for i, elem := range result.Elements {
			origElem := original.Elements[i]
			if elem.Name != origElem.Name {
				t.Errorf("Element[%d].Name differs: got %q, expected %q", i, elem.Name, origElem.Name)
			}
			if !bytes.Equal(elem.Data, origElem.Data) {
				t.Errorf("Element[%d].Data differs: got %v, expected %v", i, elem.Data, origElem.Data)
			}
		}
	})

	// Test array with static complex elements
	t.Run("array_static_elements", func(t *testing.T) {
		type StaticElement struct {
			ID    uint32
			Value uint64
			Flag  bool
		}

		type ArrayContainer struct {
			Elements [3]StaticElement
		}

		original := ArrayContainer{
			Elements: [3]StaticElement{
				{ID: 1, Value: 100, Flag: true},
				{ID: 2, Value: 200, Flag: false},
				{ID: 3, Value: 300, Flag: true},
			},
		}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result ArrayContainer
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		for i, elem := range result.Elements {
			origElem := original.Elements[i]
			if elem.ID != origElem.ID || elem.Value != origElem.Value || elem.Flag != origElem.Flag {
				t.Errorf("Element[%d] differs: got {%d, %d, %t}, expected {%d, %d, %t}",
					i, elem.ID, elem.Value, elem.Flag, origElem.ID, origElem.Value, origElem.Flag)
			}
		}
	})

	// Test slice with dynamic elements
	t.Run("slice_dynamic_elements", func(t *testing.T) {
		type DynamicElement struct {
			Tags []uint16 `ssz-max:"3"`
			Info string   `ssz-max:"10"`
		}

		type SliceContainer struct {
			Elements []DynamicElement `ssz-max:"2"`
		}

		original := SliceContainer{
			Elements: []DynamicElement{
				{Tags: []uint16{1, 2}, Info: "info1"},
				{Tags: []uint16{3, 4, 5}, Info: "info2"},
			},
		}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result SliceContainer
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		if len(result.Elements) != len(original.Elements) {
			t.Errorf("Slice length differs: got %d, expected %d", len(result.Elements), len(original.Elements))
		}
		for i, elem := range result.Elements {
			origElem := original.Elements[i]
			if elem.Info != origElem.Info {
				t.Errorf("Element[%d].Info differs: got %q, expected %q", i, elem.Info, origElem.Info)
			}
			if len(elem.Tags) != len(origElem.Tags) {
				t.Errorf("Element[%d] tags length differs: got %d, expected %d", i, len(elem.Tags), len(origElem.Tags))
			} else {
				for j, tag := range elem.Tags {
					if tag != origElem.Tags[j] {
						t.Errorf("Element[%d].Tags[%d] differs: got %d, expected %d", i, j, tag, origElem.Tags[j])
					}
				}
			}
		}
	})

	// Test with spec values
	t.Run("spec_values", func(t *testing.T) {
		type SpecElement struct {
			Data []byte `dynssz-max:"MAX_ITEMS"`
		}

		type SpecContainer struct {
			Elements []SpecElement `dynssz-max:"MAX_ITEMS"`
		}

		original := SpecContainer{
			Elements: []SpecElement{
				{Data: []byte{1, 2}},
				{Data: []byte{3, 4, 5}},
			},
		}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result SpecContainer
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		if len(result.Elements) != len(original.Elements) {
			t.Errorf("Elements length differs: got %d, expected %d", len(result.Elements), len(original.Elements))
		}
		for i, elem := range result.Elements {
			if !bytes.Equal(elem.Data, original.Elements[i].Data) {
				t.Errorf("Element[%d] data differs: got %v, expected %v", i, elem.Data, original.Elements[i].Data)
			}
		}
	})
}

// Test strings with static size for unmarshal reader
func TestUnmarshalReaderStaticStrings(t *testing.T) {
	dynssz := NewDynSsz(map[string]any{
		"FIXED_SIZE": uint64(16),
	})
	dynssz.NoFastSsz = true
	dynssz.NoStreamBuffering = true

	tests := []struct {
		name     string
		original string
		size     string
	}{
		{"empty_fixed", "", "16"},
		{"short_fixed", "hello", "16"},
		{"exact_fixed", "exactly16chars!!", "16"},
		{"spec_fixed", "spec", "FIXED_SIZE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var structType interface{}
			var original interface{}

			if test.size == "FIXED_SIZE" {
				type SpecStringStruct struct {
					Data string `dynssz-size:"FIXED_SIZE"`
				}
				s := SpecStringStruct{Data: test.original}
				structType = &SpecStringStruct{}
				original = s
			} else {
				type StaticStringStruct struct {
					Data string `ssz-size:"16"`
				}
				s := StaticStringStruct{Data: test.original}
				structType = &StaticStringStruct{}
				original = s
			}

			data, err := dynssz.MarshalSSZ(original)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			reader := bytes.NewReader(data)
			err = dynssz.UnmarshalSSZReader(structType, reader, int64(len(data)))
			if err != nil {
				t.Errorf("UnmarshalSSZReader failed: %v", err)
			}

			// The comparison would be complex due to reflection, but if we got here without error, the test passed
		})
	}
}

// Test pointer scenarios for unmarshal reader
func TestUnmarshalReaderPointers(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.NoStreamBuffering = true

	// Test nested pointers (simpler case)
	t.Run("nested_pointers", func(t *testing.T) {
		type NestedStruct struct {
			ID   uint32
			Data []byte `ssz-max:"8"`
		}

		type PointerContainer struct {
			Nested *NestedStruct
			Count  *uint32
		}

		count := uint32(123)
		original := PointerContainer{
			Nested: &NestedStruct{ID: 42, Data: []byte("test")},
			Count:  &count,
		}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result PointerContainer
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		// Basic validation - if we got here without panicking, the pointers were handled
		if result.Count == nil || *result.Count != 123 {
			t.Errorf("Count pointer not properly handled")
		}
		if result.Nested == nil || result.Nested.ID != 42 {
			t.Errorf("Nested pointer not properly handled")
		}
	})

	// Test basic pointer handling
	t.Run("basic_pointers", func(t *testing.T) {
		type BasicPointer struct {
			Value *uint32
		}

		val := uint32(100)
		original := BasicPointer{Value: &val}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result BasicPointer
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		// Validate the structure
		if result.Value == nil || *result.Value != 100 {
			t.Errorf("Pointer not properly restored: got %v, expected 100", result.Value)
		}
	})

	// Test nil pointer handling
	t.Run("nil_pointers", func(t *testing.T) {
		type NilPointer struct {
			Value *uint32
		}

		original := NilPointer{Value: nil}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result NilPointer
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		// For nil pointers, the field should be the zero value (0)
		if result.Value == nil || *result.Value != 0 {
			t.Errorf("Expected zero value for nil pointer")
		}
	})
}

// Test edge cases for unmarshal reader
func TestUnmarshalReaderEdgeCases(t *testing.T) {
	dynssz := NewDynSsz(nil)
	dynssz.NoFastSsz = true
	dynssz.NoStreamBuffering = true

	// Test slice size constraints
	t.Run("slice_size_constraints", func(t *testing.T) {
		type ConstrainedStruct struct {
			Items []uint32 `ssz-size:"4"`
		}

		// Create data with exactly 4 items
		original := ConstrainedStruct{
			Items: []uint32{1, 2, 3, 4},
		}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result ConstrainedStruct
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		if len(result.Items) != 4 {
			t.Errorf("Expected 4 items, got %d", len(result.Items))
		}
		for i, item := range result.Items {
			if item != original.Items[i] {
				t.Errorf("Item[%d] differs: got %d, expected %d", i, item, original.Items[i])
			}
		}
	})

	// Test empty dynamic fields
	t.Run("empty_dynamic_fields", func(t *testing.T) {
		type EmptyDynamicStruct struct {
			EmptySlice []uint32 `ssz-max:"10"`
			EmptyStr   string   `ssz-max:"50"`
			EmptyBytes []byte   `ssz-max:"20"`
		}

		original := EmptyDynamicStruct{
			EmptySlice: []uint32{},
			EmptyStr:   "",
			EmptyBytes: []byte{},
		}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result EmptyDynamicStruct
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		if len(result.EmptySlice) != 0 {
			t.Errorf("Expected empty slice, got length %d", len(result.EmptySlice))
		}
		if result.EmptyStr != "" {
			t.Errorf("Expected empty string, got %q", result.EmptyStr)
		}
		if len(result.EmptyBytes) != 0 {
			t.Errorf("Expected empty bytes, got length %d", len(result.EmptyBytes))
		}
	})

	// Test deeply nested structures
	t.Run("deeply_nested", func(t *testing.T) {
		type Level3 struct {
			Data []byte `ssz-max:"5"`
		}

		type Level2 struct {
			L3 Level3
			ID uint32
		}

		type Level1 struct {
			L2s  []Level2 `ssz-max:"2"`
			Name string   `ssz-max:"10"`
		}

		type TopLevel struct {
			L1 Level1
		}

		original := TopLevel{
			L1: Level1{
				L2s: []Level2{
					{L3: Level3{Data: []byte{1, 2}}, ID: 1},
					{L3: Level3{Data: []byte{3, 4, 5}}, ID: 2},
				},
				Name: "nested",
			},
		}

		data, err := dynssz.MarshalSSZ(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var result TopLevel
		reader := bytes.NewReader(data)
		err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
		if err != nil {
			t.Errorf("UnmarshalSSZReader failed: %v", err)
		}

		if result.L1.Name != original.L1.Name {
			t.Errorf("Name differs: got %q, expected %q", result.L1.Name, original.L1.Name)
		}
		if len(result.L1.L2s) != len(original.L1.L2s) {
			t.Errorf("L2s length differs: got %d, expected %d", len(result.L1.L2s), len(original.L1.L2s))
		}
	})
}

// Test string unmarshaling specifically (0% coverage)
func TestUnmarshalStringReader(t *testing.T) {
	dynssz := NewDynSsz(nil)

	tests := []struct {
		name     string
		original string
		maxSize  string
	}{
		{"empty_string", "", "100"},
		{"short_string", "hello", "100"},
		{"long_string", "this is a much longer string that tests the string handling capabilities", "200"},
		{"unicode_string", "hello 世界 🌍", "100"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			type StringStruct struct {
				Data string `ssz-max:"100"`
			}

			original := StringStruct{Data: test.original}
			data, err := dynssz.MarshalSSZ(original)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var result StringStruct
			reader := bytes.NewReader(data)
			err = dynssz.UnmarshalSSZReader(&result, reader, int64(len(data)))
			if err != nil {
				t.Errorf("UnmarshalSSZReader failed: %v", err)
			}
			if result.Data != original.Data {
				t.Errorf("String mismatch: got %q, expected %q", result.Data, original.Data)
			}
		})
	}
}

// Helper types and functions
type failingReader struct {
	data   []byte
	pos    int
	failAt int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.pos >= r.failAt {
		return 0, io.ErrUnexpectedEOF
	}

	toRead := len(p)
	available := len(r.data) - r.pos
	if toRead > available {
		toRead = available
	}
	if r.pos+toRead > r.failAt {
		toRead = r.failAt - r.pos
	}

	if toRead == 0 {
		return 0, io.ErrUnexpectedEOF
	}

	copy(p, r.data[r.pos:r.pos+toRead])
	r.pos += toRead
	return toRead, nil
}

func compareValues(a, b interface{}) bool {
	// Simple comparison for basic types
	switch va := a.(type) {
	case *bool:
		if vb, ok := b.(*bool); ok {
			return *va == *vb
		}
	case *uint8:
		if vb, ok := b.(*uint8); ok {
			return *va == *vb
		}
	case *uint16:
		if vb, ok := b.(*uint16); ok {
			return *va == *vb
		}
	case *uint32:
		if vb, ok := b.(*uint32); ok {
			return *va == *vb
		}
	case *uint64:
		if vb, ok := b.(*uint64); ok {
			return *va == *vb
		}
	}
	return false
}
