// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
)

// marshalType is the core recursive function for marshalling Go values into SSZ-encoded data.
//
// This function serves as the primary dispatcher within the marshalling process, handling both primitive
// and composite types. It uses the TypeDescriptor's metadata to determine the most efficient encoding
// path, automatically leveraging fastssz when possible for optimal performance.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing optimized metadata about the type to be encoded
//   - sourceValue: The reflect.Value holding the data to be encoded
//   - buf: The byte slice buffer where encoded data is appended
//   - idt: Indentation level for verbose logging (when enabled)
//
// Returns:
//   - []byte: The updated buffer containing the appended SSZ-encoded data
//   - error: An error if encoding fails
//
// The function handles:
//   - Automatic nil pointer dereferencing
//   - FastSSZ delegation for compatible types without dynamic sizing
//   - Primitive type encoding (bool, uint8, uint16, uint32, uint64)
//   - Delegation to specialized functions for composite types (structs, arrays, slices)

func (d *DynSsz) marshalType(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	if sourceType.IsPtr {
		if sourceValue.IsNil() {
			sourceValue = reflect.New(sourceType.Type.Elem()).Elem()
		} else {
			sourceValue = sourceValue.Elem()
		}
	}

	useFastSsz := !d.NoFastSsz && sourceType.IsFastSSZMarshaler && !sourceType.HasDynamicSize

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), sourceType.Type.Name(), sourceType.Kind, useFastSsz, sourceType.IsFastSSZMarshaler, sourceType.HasDynamicSize)
	}

	if useFastSsz {
		marshaller, ok := sourceValue.Addr().Interface().(fastsszMarshaler)
		if ok {
			newBuf, err := marshaller.MarshalSSZTo(buf)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz {
		// can't use fastssz, use dynamic marshaling
		switch sourceType.Kind {
		case reflect.Struct:
			newBuf, err := d.marshalStruct(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		case reflect.Array:
			newBuf, err := d.marshalArray(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		case reflect.Slice:
			newBuf, err := d.marshalSlice(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		case reflect.Bool:
			buf = marshalBool(buf, sourceValue.Bool())
		case reflect.Uint8:
			buf = marshalUint8(buf, uint8(sourceValue.Uint()))
		case reflect.Uint16:
			buf = marshalUint16(buf, uint16(sourceValue.Uint()))
		case reflect.Uint32:
			buf = marshalUint32(buf, uint32(sourceValue.Uint()))
		case reflect.Uint64:
			buf = marshalUint64(buf, uint64(sourceValue.Uint()))
		case reflect.String:
			// Convert string to []byte
			stringBytes := []byte(sourceValue.String())
			
			// Check if this is a fixed-size string
			if sourceType.Size > 0 {
				// Fixed-size string: pad or truncate to exact size
				fixedSize := int(sourceType.Size)
				if len(stringBytes) > fixedSize {
					// Truncate if too long
					buf = append(buf, stringBytes[:fixedSize]...)
				} else {
					// Pad with zeros if too short
					buf = append(buf, stringBytes...)
					padding := fixedSize - len(stringBytes)
					for i := 0; i < padding; i++ {
						buf = append(buf, 0)
					}
				}
			} else {
				// Dynamic string: append as-is
				buf = append(buf, stringBytes...)
			}
		default:
			return nil, fmt.Errorf("unknown type: %v", sourceType)
		}
	}

	return buf, nil
}

// marshalStruct handles the encoding of Go struct values into SSZ-encoded data.
//
// This function implements the SSZ specification for struct encoding, which requires:
//   - Fixed-size fields are encoded first in field definition order
//   - Variable-size fields are encoded after all fixed fields
//   - Variable-size fields are prefixed with 4-byte offsets in the fixed section
//
// The function uses the pre-computed TypeDescriptor to efficiently navigate the struct's
// layout without repeated reflection calls.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing struct field metadata
//   - sourceValue: The reflect.Value of the struct to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with the encoded struct
//   - error: An error if any field encoding fails

func (d *DynSsz) marshalStruct(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	offset := 0
	startLen := len(buf)
	fieldCount := len(sourceType.Fields)

	for i := 0; i < fieldCount; i++ {
		field := sourceType.Fields[i]
		fieldSize := field.Size
		if field.Size > 0 {
			//fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)

			fieldValue := sourceValue.Field(i)
			newBuf, err := d.marshalType(field.Type, fieldValue, buf, idt+2)
			if err != nil {
				return nil, fmt.Errorf("failed encoding field %v: %v", field.Name, err)
			}
			buf = newBuf

		} else {
			fieldSize = 4
			buf = binary.LittleEndian.AppendUint32(buf, 0)
			//fmt.Printf("%sfield %d:\t offset [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)
		}
		offset += int(fieldSize)
	}

	for _, field := range sourceType.DynFields {
		// set field offset
		fieldOffset := int(field.Offset)
		binary.LittleEndian.PutUint32(buf[fieldOffset+startLen:fieldOffset+startLen+4], uint32(offset))

		//fmt.Printf("%sfield %d:\t dynamic [%v:]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], offset, field.Name)

		fieldDescriptor := field.Field
		fieldValue := sourceValue.Field(int(fieldDescriptor.Index))
		bufLen := len(buf)
		newBuf, err := d.marshalType(fieldDescriptor.Type, fieldValue, buf, idt+2)
		if err != nil {
			return nil, fmt.Errorf("failed decoding field %v: %v", fieldDescriptor.Name, err)
		}
		buf = newBuf
		offset += len(buf) - bufLen
	}

	return buf, nil
}

// marshalArray encodes Go array values into SSZ-encoded data.
//
// Arrays in SSZ are encoded as fixed-size sequences where each element is encoded
// sequentially without any length prefix (since the length is known from the type).
// For byte arrays ([N]byte), the function uses an optimized path that directly
// appends the bytes without element-wise iteration.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing array metadata including element type and length
//   - sourceValue: The reflect.Value of the array to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with the encoded array
//   - error: An error if any element encoding fails
//
// Special handling:
//   - Byte arrays use reflect.Value.Bytes() for efficient bulk copying
//   - Non-addressable arrays are made addressable via a temporary pointer

func (d *DynSsz) marshalArray(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	fieldType := sourceType.ElemDesc
	if fieldType.Size < 0 {
		return d.marshalDynamicSlice(sourceType, sourceValue, buf, idt)
	}

	arrLen := sourceType.Len
	if sourceType.IsByteArray {
		// shortcut for performance: use append on []byte arrays
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(sourceType.Type)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}
		buf = append(buf, sourceValue.Bytes()...)
	} else {
		for i := 0; i < int(arrLen); i++ {
			itemVal := sourceValue.Index(i)
			newBuf, err := d.marshalType(sourceType.ElemDesc, itemVal, buf, idt+2)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		}
	}

	return buf, nil
}

// marshalSlice encodes Go slice values into SSZ-encoded data.
//
// This function handles slices with fixed-size elements. For slices with variable-size
// elements, it delegates to marshalDynamicSlice. The encoding follows SSZ specifications
// where slices are encoded as their elements in sequence without a length prefix.
//
// If the slice has size hints from parent structures (via ssz-size tags), the function
// ensures the encoded length matches the hint, padding with zero values if necessary.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing slice metadata and element type information
//   - sourceValue: The reflect.Value of the slice to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with the encoded slice
//   - error: An error if encoding fails or slice exceeds size constraints
//
// Special handling:
//   - Delegates to marshalDynamicSlice for variable-size elements
//   - Byte slices use optimized bulk append
//   - Zero-padding is applied when slice length is less than size hint
//   - Returns ErrListTooBig if slice exceeds maximum size from hints

func (d *DynSsz) marshalSlice(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	fieldType := sourceType.ElemDesc
	if fieldType.Size < 0 {
		return d.marshalDynamicSlice(sourceType, sourceValue, buf, idt)
	}

	sliceLen := sourceValue.Len()
	appendZero := 0
	if len(sourceType.SizeHints) > 0 && !sourceType.SizeHints[0].Dynamic {
		if uint32(sliceLen) > sourceType.SizeHints[0].Size {
			return nil, ErrListTooBig
		}
		if uint32(sliceLen) < sourceType.SizeHints[0].Size {
			appendZero = int(sourceType.SizeHints[0].Size - uint32(sliceLen))
		}
	}

	if sourceType.IsByteArray {
		// shortcut for performance: use append on []byte arrays
		buf = append(buf, sourceValue.Bytes()...)

		if appendZero > 0 {
			zeroBytes := make([]uint8, appendZero)
			buf = append(buf, zeroBytes...)
		}
	} else {

		for i := 0; i < sliceLen; i++ {
			itemVal := sourceValue.Index(i)
			if fieldType.IsPtr {
				if itemVal.IsNil() {
					itemVal = reflect.New(fieldType.Type.Elem())
				}
			}

			newBuf, err := d.marshalType(fieldType, itemVal, buf, idt+2)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		}

		if appendZero > 0 {
			totalZeroBytes := int(fieldType.Size) * appendZero
			zeroBuf := make([]byte, totalZeroBytes)
			buf = append(buf, zeroBuf...)
		}
	}

	return buf, nil
}

// marshalDynamicSlice encodes slices with variable-size elements into SSZ format.
//
// For slices with variable-size elements, SSZ requires a special encoding:
//   1. A series of 4-byte offsets, one per element, indicating where each element's data begins
//   2. The actual encoded data for each element, in order
//
// The offsets are relative to the start of the slice encoding (not the entire message).
// This allows decoders to locate each variable-size element without parsing all preceding elements.
//
// Parameters:
//   - sourceType: The TypeDescriptor with slice metadata
//   - sourceValue: The reflect.Value of the slice to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with offsets followed by encoded elements
//   - error: An error if encoding fails or size constraints are violated
//
// The function handles size hints for padding with zero values when the slice
// length is less than the expected size. Zero values are efficiently batched
// to minimize encoding overhead.

func (d *DynSsz) marshalDynamicSlice(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	fieldType := sourceType.ElemDesc
	sliceLen := sourceValue.Len()

	appendZero := 0
	if len(sourceType.SizeHints) > 0 && !sourceType.SizeHints[0].Dynamic {
		if uint32(sliceLen) > sourceType.SizeHints[0].Size {
			return nil, ErrListTooBig
		}
		if uint32(sliceLen) < sourceType.SizeHints[0].Size {
			appendZero = int(sourceType.SizeHints[0].Size - uint32(sliceLen))
		}
	}

	startOffset := len(buf)
	totalOffsets := sliceLen + appendZero
	offsetBuf := make([]byte, 4*totalOffsets)
	buf = append(buf, offsetBuf...)

	offset := 4 * totalOffsets
	bufLen := len(buf)

	for i := 0; i < sliceLen; i++ {
		itemVal := sourceValue.Index(i)

		newBuf, err := d.marshalType(fieldType, itemVal, buf, idt+2)
		if err != nil {
			return nil, err
		}
		newBufLen := len(newBuf)
		buf = newBuf

		binary.LittleEndian.PutUint32(buf[startOffset+(i*4):startOffset+((i+1)*4)], uint32(offset))

		offset += newBufLen - bufLen
		bufLen = newBufLen
	}

	if appendZero > 0 {
		var zeroVal reflect.Value

		if fieldType.IsPtr {
			zeroVal = reflect.New(fieldType.Type.Elem())
		} else {
			zeroVal = reflect.New(fieldType.Type).Elem()
		}

		zeroBuf, err := d.marshalType(fieldType, zeroVal, []byte{}, idt+2)
		if err != nil {
			return nil, err
		}
		zeroBufLen := len(zeroBuf)

		// Batch append all zero values at once for better performance
		totalZeroBytes := zeroBufLen * appendZero
		zeroData := make([]byte, 0, totalZeroBytes)
		for i := 0; i < appendZero; i++ {
			zeroData = append(zeroData, zeroBuf...)
			binary.LittleEndian.PutUint32(buf[startOffset+((sliceLen+i)*4):startOffset+(((sliceLen+i)+1)*4)], uint32(offset))
			offset += zeroBufLen
		}

		buf = append(buf, zeroData...)
	}

	return buf, nil
}
