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

// marshalType is the entry point for marshalling Go values into SSZ-encoded data, using reflection to navigate
// the type tree and encode each value appropriately. It serves as the core function for the recursive encoding process,
// handling both primitive and composite types.
//
// Parameters:
// - sourceType: The reflect.Type of the value to be encoded. This provides the necessary type information to guide
//   the encoding process for both simple and complex types.
// - sourceValue: The reflect.Value that holds the data to be encoded. This function uses sourceValue to extract
//   the actual data for encoding into SSZ format.
// - buf: A byte slice that serves as the initial buffer for the encoded data. As the function processes each value,
//   it appends the encoded bytes to this buffer, growing it as necessary to accommodate the encoded data.
// - sizeHints: A slice of sszSizeHint, populated from 'ssz-size' and 'dynssz-size' tag annotations from parent
//   structures. These hints are crucial for encoding types like slices and arrays that may have dynamic lengths, ensuring
//   that the encoded data reflects the correct size information.
// - idt: An indentation level, primarily used for debugging or logging to help track the recursion depth and encoding
//   sequence of the data structure.
//
// Returns:
// - A byte slice containing the SSZ-encoded data. This is the final encoded version of sourceValue, ready for storage
//   or transmission.
// - An error if the encoding process encounters any issues, such as an unsupported type or a mismatch between the
//   sourceValue and the expected type structure.
//
// This function serves as the primary dispatcher within the marshalling process, encoding primitive types directly and
// delegating the encoding of composite types (e.g., structs, arrays, slices) to specialized functions like marshalStruct,
// marshalArray, and marshalSlice. For composite types, marshalType orchestrates the encoding by preparing the necessary
// context and parameters, then calling the appropriate specialized function based on the type of the sourceValue.
// This division of responsibility allows marshalType to efficiently handle the encoding process across a wide range of
// data types by leveraging type-specific encoding logic for complex structures. The recursion in the encoding process
// ensures that nested structures are fully and accurately encoded.

func (d *DynSsz) marshalType(sourceType reflect.Type, sourceValue reflect.Value, buf []byte, sizeHints []sszSizeHint, idt int) ([]byte, error) {
	if sourceType.Kind() == reflect.Ptr {
		sourceType = sourceType.Elem()

		if sourceValue.IsNil() {
			sourceValue = reflect.New(sourceType).Elem()
		} else {
			sourceValue = sourceValue.Elem()
		}
	}

	// use fastssz to marshal if:
	// - type implements fastssz Marshaler interface
	// - this type or any child types does not use spec specific field sizes
	fastsszCompat, err := d.getFastsszConvertCompatibility(sourceType, sizeHints)
	if err != nil {
		return nil, fmt.Errorf("failed checking fastssz compatibility: %v", err)
	}

	useFastSsz := !d.NoFastSsz && fastsszCompat.isMarshaler && !fastsszCompat.hasDynamicSpecSizes

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), sourceType.Name(), sourceType.Kind(), useFastSsz, fastsszCompat.isMarshaler, fastsszCompat.hasDynamicSpecSizes)
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
		switch sourceType.Kind() {
		case reflect.Struct:
			newBuf, err := d.marshalStruct(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		case reflect.Array:
			newBuf, err := d.marshalArray(sourceType, sourceValue, buf, sizeHints, idt)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		case reflect.Slice:
			newBuf, err := d.marshalSlice(sourceType, sourceValue, buf, sizeHints, idt)
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
		default:
			return nil, fmt.Errorf("unknown type: %v", sourceType)
		}
	}

	return buf, nil
}

// marshalStruct handles the encoding of Go struct values into SSZ-encoded data. It iterates through each field of the struct,
// leveraging reflection to access field types and values, and delegates the encoding of each field to the marshalType function.
//
// Parameters:
// - sourceType: The reflect.Type of the struct to be encoded. This provides the necessary type information to guide
//   the encoding process for the struct's fields.
// - sourceValue: The reflect.Value that holds the struct data to be encoded. marshalStruct iterates over each field
//   of the struct and uses sourceValue to extract the data for encoding.
// - buf: A byte slice that serves as the initial buffer for the encoded data. As the function processes each field,
//   it appends the encoded bytes to this buffer, dynamically expanding it as needed to accommodate the encoded data.
// - idt: An indentation level, primarily used for debugging or logging to help track the recursion depth and encoding
//   sequence of the struct fields.
//
// Returns:
// - A byte slice containing the SSZ-encoded data of the struct. This byte slice represents the serialized version of
//   sourceValue, with all struct fields encoded according to SSZ specifications.
// - An error if the encoding process encounters any issues, such as an unsupported field type or a mismatch between
//   the sourceValue's actual data and what is expected for SSZ encoding.
//
// marshalStruct specifically focuses on the structural aspects of encoding a struct, calculating offsets and sizes for
// each field as needed, and invoking marshalType for the actual encoding logic. This approach allows for precise control
// over the encoding of each field, ensuring that the resulting SSZ data accurately reflects the structure and content
// of the original Go struct.

func (d *DynSsz) marshalStruct(sourceType reflect.Type, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	offset := 0
	startLen := len(buf)
	dynamicFields := []*reflect.StructField{}
	dynamicOffsets := []int{}
	dynamicSizeHints := [][]sszSizeHint{}

	for i := 0; i < sourceType.NumField(); i++ {
		field := sourceType.Field(i)

		fieldSize, _, sizeHints, err := d.getSszFieldSize(&field)
		if err != nil {
			return nil, err
		}

		if fieldSize > 0 {
			//fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)

			fieldValue := sourceValue.Field(i)
			newBuf, err := d.marshalType(field.Type, fieldValue, buf, sizeHints, idt+2)
			if err != nil {
				return nil, fmt.Errorf("failed encoding field %v: %v", field.Name, err)
			}
			buf = newBuf

		} else {
			fieldSize = 4
			buf = append(buf, 0, 0, 0, 0)
			//fmt.Printf("%sfield %d:\t offset [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)

			dynamicFields = append(dynamicFields, &field)
			dynamicOffsets = append(dynamicOffsets, offset)
			dynamicSizeHints = append(dynamicSizeHints, sizeHints)
		}
		offset += fieldSize
	}

	for i, field := range dynamicFields {
		// set field offset
		fieldOffset := dynamicOffsets[i]
		offsetBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(offsetBuf, uint32(offset))
		copy(buf[fieldOffset+startLen:fieldOffset+startLen+4], offsetBuf)

		//fmt.Printf("%sfield %d:\t dynamic [%v:]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], offset, field.Name)

		fieldValue := sourceValue.Field(field.Index[0])
		bufLen := len(buf)
		newBuf, err := d.marshalType(field.Type, fieldValue, buf, dynamicSizeHints[i], idt+2)
		if err != nil {
			return nil, fmt.Errorf("failed decoding field %v: %v", field.Name, err)
		}
		buf = newBuf
		offset += len(buf) - bufLen
	}

	return buf, nil
}

// marshalArray encodes Go array values into SSZ-encoded data. It processes each element of the array, using reflection to
// access element types and values, and delegates the encoding of individual elements to the marshalType function.
//
// Parameters:
// - sourceType: The reflect.Type of the array to be encoded, offering the type information needed to encode each element
//   within the array correctly.
// - sourceValue: The reflect.Value that holds the array data to be encoded. marshalArray iterates over each element
//   of the array, using sourceValue to extract the data for encoding.
// - buf: A byte slice that acts as the starting buffer for the encoded data. As the function encodes each element,
//   it appends the encoded bytes to this buffer, expanding it as necessary to fit the resulting encoded data.
// - sizeHints: A slice of sszSizeHint, informed by 'ssz-size' and 'dynssz-size' tag annotations from parent structures.
//   These hints assist in encoding elements that have dynamic sizes, ensuring accurate size information in the encoded output.
// - idt: An indentation level used for debugging or logging, facilitating the tracking of the encoding depth and sequence
//   of array elements.
//
// Returns:
// - A byte slice containing the SSZ-encoded data of the array. This byte slice represents the serialized form of sourceValue,
//   with all array elements encoded according to SSZ specifications.
// - An error if the encoding process encounters any issues, such as an unsupported element type or discrepancies between
//   the actual data of sourceValue and the requirements for SSZ encoding.
//
// marshalArray focuses on the encoding of arrays by navigating through each element and ensuring accurate representation
// in the SSZ-encoded output. The function relies on marshalType for the encoding of individual elements, allowing for
// a consistent and recursive encoding approach that handles both simple and complex types within the array.

func (d *DynSsz) marshalArray(sourceType reflect.Type, sourceValue reflect.Value, buf []byte, sizeHints []sszSizeHint, idt int) ([]byte, error) {

	childSizeHints := []sszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	fieldType := sourceType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	arrLen := sourceType.Len()
	if fieldType == byteType {
		// shortcut for performance: use append on []byte arrays
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(sourceType)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}
		buf = append(buf, sourceValue.Bytes()...)
	} else {
		for i := 0; i < arrLen; i++ {
			itemVal := sourceValue.Index(i)
			if fieldIsPtr {
				itemVal = itemVal.Elem()
			}

			newBuf, err := d.marshalType(fieldType, itemVal, buf, childSizeHints, idt+2)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		}
	}

	return buf, nil
}

// marshalSlice encodes Go slice values with static size items into SSZ-encoded data. For slices containing elements
// with dynamic sizes, it internally calls marshalDynamicSlice to accommodate the variability in element sizes.
// This function processes each element of the slice using reflection to access their types and values, and relies
// on marshalType for encoding individual static size elements.
//
// Parameters:
// - sourceType: The reflect.Type of the slice to be encoded, providing the type information necessary for correctly encoding
//   each element within the slice.
// - sourceValue: The reflect.Value holding the data of the slice to be encoded. marshalSlice iterates over each element,
//   utilizing sourceValue to extract the data for encoding.
// - buf: A byte slice that serves as the initial buffer for the encoded data. As each element is encoded, the resulting bytes
//   are appended to this buffer, which is dynamically expanded as needed to fit the encoded data.
// - sizeHints: A slice of sszSizeHint, derived from 'ssz-size' and 'dynssz-size' tag annotations from parent structures,
//   crucial for encoding slices with elements that have dynamic lengths. This assists in providing accurate size information
//   in the encoded output, especially for dynamic elements.
// - idt: An indentation level, primarily for debugging or logging purposes, to aid in tracking the encoding process's depth
//   and sequence for the slice elements.
//
// Returns:
// - The byte slice containing the SSZ-encoded data of the slice, representing the serialized version of sourceValue
//   with all elements encoded in compliance with SSZ specifications.
// - An error if any issues arise during the encoding process, such as encountering an unsupported element type or if there
//   is a mismatch between the sourceValue data and SSZ encoding requirements.
//
// marshalSlice adeptly manages the encoding of slices by navigating through each element and ensuring they are accurately
// represented in the SSZ-encoded output. It seamlessly transitions to marshalDynamicSlice for slices with dynamically sized
// elements, leveraging a recursive encoding strategy to handle various data types within the slice effectively.

func (d *DynSsz) marshalSlice(sourceType reflect.Type, sourceValue reflect.Value, buf []byte, sizeHints []sszSizeHint, idt int) ([]byte, error) {
	childSizeHints := []sszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	fieldType := sourceType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	isDynSlice := false
	fieldSize := 0
	if len(sizeHints) > 1 && sizeHints[1].dynamic {
		isDynSlice = true
	} else {
		size, _, err := d.getSszSize(fieldType, childSizeHints)
		if err != nil {
			return nil, err
		}
		if size < 0 {
			isDynSlice = true
		}
		fieldSize = size
	}

	if isDynSlice {
		return d.marshalDynamicSlice(sourceType, sourceValue, buf, sizeHints, idt)
	}

	sliceLen := sourceValue.Len()
	appendZero := 0
	if len(sizeHints) > 0 && !sizeHints[0].dynamic {
		if uint64(sliceLen) > sizeHints[0].size {
			return nil, ErrListTooBig
		}
		if uint64(sliceLen) < sizeHints[0].size {
			appendZero = int(sizeHints[0].size - uint64(sliceLen))
		}
	}

	if fieldType == byteType {
		// shortcut for performance: use append on []byte arrays
		buf = append(buf, sourceValue.Bytes()...)

		if appendZero > 0 {
			zeroBytes := make([]uint8, appendZero)
			buf = append(buf, zeroBytes...)
		}
	} else {

		for i := 0; i < sliceLen; i++ {
			itemVal := sourceValue.Index(i)
			if fieldIsPtr {
				if itemVal.IsNil() {
					itemVal = reflect.New(fieldType).Elem()
				} else {
					itemVal = itemVal.Elem()
				}
			}

			newBuf, err := d.marshalType(fieldType, itemVal, buf, childSizeHints, idt+2)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		}

		if appendZero > 0 {
			zeroBuf := make([]byte, fieldSize)
			for i := 0; i < appendZero; i++ {
				buf = append(buf, zeroBuf...)
			}
		}
	}

	return buf, nil
}

// marshalDynamicSlice encodes Go slice values with dynamically sized items into SSZ-encoded data. This function
// handles the complexity of encoding each element by utilizing the inherent offsets within the SSZ format, ensuring
// accurate representation of variable-sized elements in the encoded output.
//
// Parameters:
// - sourceType: The reflect.Type of the slice to be encoded, providing the type information necessary for the dynamic
//   encoding of each element within the slice.
// - sourceValue: The reflect.Value holding the slice data to be encoded. This function iterates through each element
//   within the slice, encoding them based on their actual sizes and appending the results to the buffer.
// - buf: A byte slice that serves as the initial buffer for the encoded data. As elements are encoded, their bytes are
//   appended to this buffer, which is expanded as necessary to accommodate the encoded data.
// - sizeHints: A slice of sszSizeHint, derived from 'ssz-size' and 'dynssz-size' tag annotations from parent structures,
//   used to inform the encoding process for elements with sizes that cannot be determined solely by their type.
// - idt: An indentation level, primarily used for debugging or logging, to aid in tracking the encoding process's depth
//   and the sequence of the dynamically sized elements.
//
// Returns:
// - A byte slice containing the SSZ-encoded data of the dynamic slice, representing the serialized version of sourceValue,
//   with each element encoded to reflect its dynamic size.
// - An error, if any issues are encountered during the encoding process, such as unsupported element types or mismatches
//   between the sourceValue's data and the requirements for SSZ encoding.
//
// marshalDynamicSlice is adept at encoding slices containing elements of variable sizes. It leverages the structured
// nature of SSZ to encode each element according to its actual size, ensuring the final encoded data accurately reflects
// the content and structure of the original slice.

func (d *DynSsz) marshalDynamicSlice(sourceType reflect.Type, sourceValue reflect.Value, buf []byte, sizeHints []sszSizeHint, idt int) ([]byte, error) {
	childSizeHints := []sszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	sliceLen := sourceValue.Len()

	appendZero := 0
	if len(sizeHints) > 0 && !sizeHints[0].dynamic {
		if uint64(sliceLen) > sizeHints[0].size {
			return nil, ErrListTooBig
		}
		if uint64(sliceLen) < sizeHints[0].size {
			appendZero = int(sizeHints[0].size - uint64(sliceLen))
		}
	}

	startOffset := len(buf)
	offsetBuf := make([]byte, 4*(sliceLen+appendZero))
	buf = append(buf, offsetBuf...)

	fieldType := sourceType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	offset := 4 * (sliceLen + appendZero)
	bufLen := len(buf)

	for i := 0; i < sliceLen; i++ {
		itemVal := sourceValue.Index(i)
		if fieldIsPtr {
			itemVal = itemVal.Elem()
		}

		newBuf, err := d.marshalType(fieldType, itemVal, buf, childSizeHints, idt+2)
		if err != nil {
			return nil, err
		}
		newBufLen := len(newBuf)
		buf = newBuf

		offsetBuf := make([]byte, 4)
		binary.LittleEndian.PutUint32(offsetBuf, uint32(offset))
		copy(buf[startOffset+(i*4):startOffset+((i+1)*4)], offsetBuf)

		offset += newBufLen - bufLen
		bufLen = newBufLen
	}

	if appendZero > 0 {
		zeroVal := reflect.New(fieldType).Elem()

		zeroBuf, err := d.marshalType(fieldType, zeroVal, []byte{}, childSizeHints, idt+2)
		if err != nil {
			return nil, err
		}
		zeroBufLen := len(zeroBuf)

		for i := 0; i < appendZero; i++ {
			buf = append(buf, zeroBuf...)

			offsetBuf := make([]byte, 4)
			binary.LittleEndian.PutUint32(offsetBuf, uint32(offset))
			copy(buf[startOffset+((sliceLen+i)*4):startOffset+(((sliceLen+i)+1)*4)], offsetBuf)

			offset += zeroBufLen
		}

	}

	return buf, nil
}
