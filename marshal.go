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

	useFastSsz := !d.NoFastSsz && sourceType.HasFastSSZMarshaler && !sourceType.HasDynamicSize
	if !useFastSsz && sourceType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), sourceType.Type.Name(), sourceType.Kind, useFastSsz, sourceType.HasFastSSZMarshaler, sourceType.HasDynamicSize)
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
		var err error
		switch sourceType.SszType {
		// complex types
		case SszContainerType:
			buf, err = d.marshalContainer(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			if sourceType.ElemDesc.IsDynamic {
				buf, err = d.marshalDynamicVector(sourceType, sourceValue, buf, idt)
			} else {
				buf, err = d.marshalVector(sourceType, sourceValue, buf, idt)
			}
			if err != nil {
				return nil, err
			}
		case SszListType, SszBitlistType:
			if sourceType.ElemDesc.IsDynamic {
				buf, err = d.marshalDynamicList(sourceType, sourceValue, buf, idt)
			} else {
				buf, err = d.marshalList(sourceType, sourceValue, buf, idt)
			}
			if err != nil {
				return nil, err
			}

		// primitive types
		case SszBoolType:
			buf = marshalBool(buf, sourceValue.Bool())
		case SszUint8Type:
			buf = marshalUint8(buf, uint8(sourceValue.Uint()))
		case SszUint16Type:
			buf = marshalUint16(buf, uint16(sourceValue.Uint()))
		case SszUint32Type:
			buf = marshalUint32(buf, uint32(sourceValue.Uint()))
		case SszUint64Type:
			buf = marshalUint64(buf, uint64(sourceValue.Uint()))
		default:
			return nil, fmt.Errorf("unknown type: %v", sourceType)
		}
	}

	return buf, nil
}

// marshalContainer handles the encoding of container values into SSZ-encoded data.
//
// This function implements the SSZ specification for container encoding, which requires:
//   - Fixed-size fields are encoded first in field definition order
//   - Variable-size fields are encoded after all fixed fields
//   - Variable-size fields are prefixed with 4-byte offsets in the fixed section
//
// The function uses the pre-computed TypeDescriptor to efficiently navigate the container's
// layout without repeated reflection calls.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing container field metadata
//   - sourceValue: The reflect.Value of the container to encode (must be a struct)
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with the encoded struct
//   - error: An error if any field encoding fails

func (d *DynSsz) marshalContainer(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	offset := 0
	startLen := len(buf)
	fieldCount := len(sourceType.ContainerDesc.Fields)

	for i := 0; i < fieldCount; i++ {
		field := sourceType.ContainerDesc.Fields[i]
		fieldSize := field.Type.Size
		if fieldSize > 0 {
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

	for _, field := range sourceType.ContainerDesc.DynFields {
		// set field offset
		fieldOffset := int(field.Offset)
		binary.LittleEndian.PutUint32(buf[fieldOffset+startLen:fieldOffset+startLen+4], uint32(offset))

		//fmt.Printf("%sfield %d:\t dynamic [%v:]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], offset, field.Name)

		fieldDescriptor := field.Field
		fieldValue := sourceValue.Field(int(field.Index))
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

// marshalVector encodes vector values into SSZ-encoded data.
//
// Vectors in SSZ are encoded as fixed-size sequences where each element is encoded
// sequentially without any length prefix (since the length is known from the type).
// For byte arrays ([N]byte) or slices ([]byte), the function uses an optimized path that directly
// appends the bytes without element-wise iteration.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing vector metadata including element type and length
//   - sourceValue: The reflect.Value of the vector to encode
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

func (d *DynSsz) marshalVector(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	sliceLen := sourceValue.Len()
	if uint32(sliceLen) > sourceType.Len {
		return nil, ErrListTooBig
	}

	appendZero := 0
	if uint32(sliceLen) < sourceType.Len {
		appendZero = int(sourceType.Len) - sliceLen
	}

	if sourceType.IsByteArray || sourceType.IsString {
		// shortcut for performance: use append on []byte arrays
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(sourceType.Type)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}

		var bytes []byte
		if sourceType.IsString {
			bytes = []byte(sourceValue.String())
		} else {
			bytes = sourceValue.Bytes()
		}

		buf = append(buf, bytes...)

		if appendZero > 0 {
			zeroBytes := make([]uint8, appendZero)
			buf = append(buf, zeroBytes...)
		}
	} else {
		for i := 0; i < int(sliceLen); i++ {
			itemVal := sourceValue.Index(i)
			newBuf, err := d.marshalType(sourceType.ElemDesc, itemVal, buf, idt+2)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		}

		if appendZero > 0 {
			totalZeroBytes := int(sourceType.ElemDesc.Size) * appendZero
			zeroBuf := make([]byte, totalZeroBytes)
			buf = append(buf, zeroBuf...)
		}
	}

	return buf, nil
}

// marshalDynamicVector encodes vectors with variable-size elements into SSZ format.
//
// For vectors with variable-size elements, SSZ requires a special encoding:
//   1. A series of 4-byte offsets, one per element, indicating where each element's data begins
//   2. The actual encoded data for each element, in order
//
// The offsets are relative to the start of the vector encoding (not the entire message).
// This allows decoders to locate each variable-size element without parsing all preceding elements.
//
// Parameters:
//   - sourceType: The TypeDescriptor with vector metadata
//   - sourceValue: The reflect.Value of the vector to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with offsets followed by encoded elements
//   - error: An error if encoding fails or size constraints are violated
//
// The function handles size hints for padding with zero values when the list
// length is less than the expected size. Zero values are efficiently batched
// to minimize encoding overhead.

func (d *DynSsz) marshalDynamicVector(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	fieldType := sourceType.ElemDesc
	sliceLen := sourceValue.Len()

	appendZero := 0
	if sourceType.Kind == reflect.Slice || sourceType.Kind == reflect.String {
		sliceLen := sourceValue.Len()
		if uint32(sliceLen) > sourceType.Len {
			return nil, ErrListTooBig
		}
		if uint32(sliceLen) < sourceType.Len {
			appendZero = int(sourceType.Len) - sliceLen
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

// marshalList encodes list values into SSZ-encoded data.
//
// This function handles lists with fixed-size elements. The encoding follows SSZ specifications
// where lists are encoded as their elements in sequence without a length prefix.
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

func (d *DynSsz) marshalList(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	if sourceType.IsString {
		stringBytes := []byte(sourceValue.String())
		buf = append(buf, stringBytes...)
	} else if sourceType.IsByteArray {
		buf = append(buf, sourceValue.Bytes()...)
	} else {
		sliceLen := sourceValue.Len()
		fieldType := sourceType.ElemDesc

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
	}

	return buf, nil
}

// marshalDynamicList encodes lists with variable-size elements into SSZ format.
//
// For lists with variable-size elements, SSZ requires a special encoding:
//   1. A series of 4-byte offsets, one per element, indicating where each element's data begins
//   2. The actual encoded data for each element, in order
//
// The offsets are relative to the start of the list encoding (not the entire message).
// This allows decoders to locate each variable-size element without parsing all preceding elements.
//
// Parameters:
//   - sourceType: The TypeDescriptor with list metadata
//   - sourceValue: The reflect.Value of the list to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with offsets followed by encoded elements
//   - error: An error if encoding fails or size constraints are violated

func (d *DynSsz) marshalDynamicList(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	fieldType := sourceType.ElemDesc
	sliceLen := sourceValue.Len()

	startOffset := len(buf)
	totalOffsets := sliceLen
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

	return buf, nil
}
