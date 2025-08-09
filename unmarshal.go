// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"strings"
)

// unmarshalType is the core recursive function for decoding SSZ-encoded data into Go values.
//
// This function serves as the primary dispatcher within the unmarshalling process, handling both
// primitive and composite types. It uses the TypeDescriptor's metadata to determine the most
// efficient decoding path, automatically leveraging fastssz when possible for optimal performance.
//
// Parameters:
//   - targetType: The TypeDescriptor containing optimized metadata about the type to decode
//   - targetValue: The reflect.Value where decoded data will be stored
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging (when enabled)
//
// Returns:
//   - int: The number of bytes consumed from the SSZ data
//   - error: An error if decoding fails
//
// The function handles:
//   - Automatic nil pointer initialization
//   - FastSSZ delegation for compatible types without dynamic sizing
//   - Primitive type decoding (bool, uint8, uint16, uint32, uint64)
//   - Delegation to specialized functions for composite types (structs, arrays, slices)
//   - Validation that consumed bytes match expected sizes

func (d *DynSsz) unmarshalType(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	consumedBytes := 0

	if targetType.IsPtr {
		// target is a pointer type, resolve type & value to actual value type
		if targetValue.IsNil() {
			// create new instance of target type for null pointers
			newValue := reflect.New(targetType.Type.Elem())
			targetValue.Set(newValue)
		}
		targetValue = targetValue.Elem()
	}

	useFastSsz := !d.NoFastSsz && targetType.IsFastSSZMarshaler && !targetType.HasDynamicSize

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), targetType.Type.Name(), targetType.Kind, useFastSsz, targetType.IsFastSSZMarshaler, targetType.HasDynamicSize)
	}

	if useFastSsz {
		unmarshaller, ok := targetValue.Addr().Interface().(fastsszUnmarshaler)
		if ok {
			err := unmarshaller.UnmarshalSSZ(ssz)
			if err != nil {
				return 0, err
			}

			consumedBytes = len(ssz)
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz {
		// can't use fastssz, use dynamic unmarshaling
		switch targetType.Kind {
		case reflect.Struct:
			consumed, err := d.unmarshalStruct(targetType, targetValue, ssz, idt)
			if err != nil {
				return 0, err
			}
			consumedBytes = consumed
		case reflect.Array:
			consumed, err := d.unmarshalArray(targetType, targetValue, ssz, idt)
			if err != nil {
				return 0, err
			}
			consumedBytes = consumed
		case reflect.Slice:
			consumed, err := d.unmarshalSlice(targetType, targetValue, ssz, idt)
			if err != nil {
				return 0, err
			}
			consumedBytes = consumed
		case reflect.String:
			consumed, err := d.unmarshalString(targetType, targetValue, ssz, idt)
			if err != nil {
				return 0, err
			}
			consumedBytes = consumed

		// primitive types
		case reflect.Bool:
			targetValue.SetBool(unmarshalBool(ssz))
			consumedBytes = 1
		case reflect.Uint8:
			targetValue.SetUint(uint64(unmarshallUint8(ssz)))
			consumedBytes = 1
		case reflect.Uint16:
			targetValue.SetUint(uint64(unmarshallUint16(ssz)))
			consumedBytes = 2
		case reflect.Uint32:
			targetValue.SetUint(uint64(unmarshallUint32(ssz)))
			consumedBytes = 4
		case reflect.Uint64:
			targetValue.SetUint(uint64(unmarshallUint64(ssz)))
			consumedBytes = 8

		default:
			return 0, fmt.Errorf("unknown type: %v", targetType)
		}
	}

	return consumedBytes, nil
}

// unmarshalStruct decodes SSZ-encoded data into a Go struct.
//
// This function implements the SSZ specification for struct decoding, which requires:
//   - Fixed-size fields appear first in the encoding
//   - Variable-size fields are referenced by 4-byte offsets in the fixed section
//   - Variable-size field data appears after all fixed fields
//
// The function uses the pre-computed TypeDescriptor to efficiently navigate the struct's
// layout without repeated reflection calls.
//
// Parameters:
//   - targetType: The TypeDescriptor containing struct field metadata
//   - targetValue: The reflect.Value of the struct to populate
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed from the SSZ data
//   - error: An error if decoding fails or data is malformed
//
// The function validates offset integrity to ensure variable fields don't overlap
// and that all data is consumed correctly.

func (d *DynSsz) unmarshalStruct(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	offset := 0
	dynamicFieldCount := len(targetType.DynFields)
	dynamicOffsets := make([]int, 0, dynamicFieldCount)
	sszSize := len(ssz)

	for i := 0; i < len(targetType.Fields); i++ {
		field := targetType.Fields[i]

		fieldSize := int(field.Type.Size)
		if fieldSize > 0 {
			// static size field
			if offset+fieldSize > sszSize {
				return 0, fmt.Errorf("unexpected end of SSZ. field %v expects %v bytes, got %v", field.Name, fieldSize, sszSize-offset)
			}

			// fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)

			fieldSsz := ssz[offset : offset+fieldSize]
			fieldValue := targetValue.Field(i)
			consumedBytes, err := d.unmarshalType(field.Type, fieldValue, fieldSsz, idt+2)
			if err != nil {
				return 0, fmt.Errorf("failed decoding field %v: %v", field.Name, err)
			}
			if consumedBytes != fieldSize {
				return 0, fmt.Errorf("struct field did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, fieldSize)
			}

		} else {
			// dynamic size field
			// get the 4 byte offset where the fields ssz range starts
			fieldSize = 4
			if offset+fieldSize > sszSize {
				return 0, fmt.Errorf("unexpected end of SSZ. dynamic field %v expects %v bytes (offset), got %v", field.Name, fieldSize, sszSize-offset)
			}
			fieldOffset := readOffset(ssz[offset : offset+fieldSize])

			// fmt.Printf("%sfield %d:\t offset [%v:%v] %v\t %v \t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name, fieldOffset)

			// store dynamic field offset for later
			dynamicOffsets = append(dynamicOffsets, int(fieldOffset))
		}
		offset += fieldSize
	}

	// finished parsing the static size fields, process dynamic fields
	for i, field := range targetType.DynFields {
		var endOffset int
		startOffset := dynamicOffsets[i]
		if i < dynamicFieldCount-1 {
			endOffset = dynamicOffsets[i+1]
		} else {
			endOffset = len(ssz)
		}

		// check offset integrity (not before previous field offset & not after range end)
		if startOffset < offset || endOffset > sszSize {
			return 0, ErrOffset
		}

		// fmt.Printf("%sfield %d:\t dynamic [%v:%v]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], startOffset, endOffset, field.Name)

		var fieldSsz []byte
		if endOffset > startOffset {
			fieldSsz = ssz[startOffset:endOffset]
		} else {
			fieldSsz = []byte{}
		}

		fieldDescriptor := field.Field
		fieldValue := targetValue.Field(int(fieldDescriptor.Index))
		consumedBytes, err := d.unmarshalType(fieldDescriptor.Type, fieldValue, fieldSsz, idt+2)
		if err != nil {
			return 0, fmt.Errorf("failed decoding field %v: %v", fieldDescriptor.Name, err)
		}
		if consumedBytes != endOffset-startOffset {
			return 0, fmt.Errorf("struct field did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, endOffset-startOffset)
		}

		offset += consumedBytes
	}

	return offset, nil
}

// unmarshalArray decodes SSZ-encoded data into a Go array.
//
// Arrays in SSZ are encoded as fixed-size sequences. Since the array length is known
// from the type, the function can calculate each element's size by dividing the total
// SSZ data length by the array length.
//
// Parameters:
//   - targetType: The TypeDescriptor containing array metadata
//   - targetValue: The reflect.Value of the array to populate
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed (should equal len(ssz))
//   - error: An error if decoding fails
//
// Special handling:
//   - Byte arrays use reflect.Copy for efficient bulk copying
//   - Pointer elements are automatically initialized
//   - Each element must consume exactly itemSize bytes

func (d *DynSsz) unmarshalArray(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	var consumedBytes int

	fieldType := targetType.ElemDesc
	arrLen := int(targetType.Len)

	// check if slice has dynamic size items
	if fieldType.Size < 0 {
		return d.unmarshalDynamicSlice(targetType, targetValue, ssz, idt)
	}

	if targetType.IsByteArray {
		// shortcut for performance: use copy on []byte arrays
		reflect.Copy(targetValue, reflect.ValueOf(ssz[0:arrLen]))
		consumedBytes = arrLen
	} else {
		offset := 0
		itemSize := len(ssz) / arrLen
		for i := 0; i < arrLen; i++ {
			var itemVal reflect.Value
			if fieldType.IsPtr {
				// fmt.Printf("new array item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType.Type.Elem())
				targetValue.Index(i).Set(itemVal.Elem().Addr())
			} else {
				itemVal = targetValue.Index(i)
			}

			itemSsz := ssz[offset : offset+itemSize]

			consumed, err := d.unmarshalType(fieldType, itemVal, itemSsz, idt+2)
			if err != nil {
				return 0, err
			}
			if consumed != itemSize {
				return 0, fmt.Errorf("unmarshalling array item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
			}

			offset += itemSize
		}

		consumedBytes = offset
	}

	return consumedBytes, nil
}

// unmarshalSlice decodes SSZ-encoded data into a Go slice.
//
// This function handles slices with fixed-size elements. For slices with variable-size
// elements, it delegates to unmarshalDynamicSlice. The slice length is determined by
// dividing the SSZ data length by the element size.
//
// Parameters:
//   - targetType: The TypeDescriptor containing slice metadata
//   - targetValue: The reflect.Value where the slice will be stored
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed (should equal len(ssz))
//   - error: An error if decoding fails or data length is invalid
//
// The function:
//   - Creates a new slice with the calculated length
//   - Delegates to unmarshalDynamicSlice for variable-size elements
//   - Uses optimized copying for byte slices
//   - Validates that each element consumes exactly the expected bytes

func (d *DynSsz) unmarshalSlice(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	var consumedBytes int

	fieldType := targetType.ElemDesc
	sszLen := len(ssz)

	// check if slice has dynamic size items
	if fieldType.Size < 0 {
		return d.unmarshalDynamicSlice(targetType, targetValue, ssz, idt)
	}

	// Calculate slice length once
	itemSize := int(fieldType.Size)
	sliceLen, ok := divideInt(sszLen, itemSize)
	if !ok {
		return 0, fmt.Errorf("invalid slice length, expected multiple of %v, got %v", itemSize, sszLen)
	}

	// slice with static size items
	// fmt.Printf("new slice %v  %v\n", fieldType.Name(), sliceLen)

	fieldT := targetType.Type
	if targetType.IsPtr {
		fieldT = fieldT.Elem()
	}

	newValue := reflect.MakeSlice(fieldT, sliceLen, sliceLen)
	targetValue.Set(newValue)

	if targetType.IsByteArray {
		// shortcut for performance: use copy on []byte arrays
		reflect.Copy(newValue, reflect.ValueOf(ssz[0:sliceLen]))
		consumedBytes = sliceLen
	} else {
		offset := 0
		if sliceLen > 0 {
			// decode slice items
			for i := 0; i < sliceLen; i++ {
				var itemVal reflect.Value
				if fieldType.IsPtr {
					// fmt.Printf("new slice item %v\n", fieldType.Name())
					itemVal = reflect.New(fieldType.Type.Elem())
					newValue.Index(i).Set(itemVal.Elem().Addr())
				} else {
					itemVal = newValue.Index(i)
				}

				itemSsz := ssz[offset : offset+itemSize]

				consumed, err := d.unmarshalType(fieldType, itemVal, itemSsz, idt+2)
				if err != nil {
					return 0, err
				}
				if consumed != itemSize {
					return 0, fmt.Errorf("slice item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
				}

				offset += itemSize
			}
		}

		consumedBytes = offset
	}

	return consumedBytes, nil
}

// unmarshalDynamicSlice decodes slices with variable-size elements from SSZ format.
//
// For slices with variable-size elements, SSZ uses an offset-based encoding:
//   - The first 4 bytes contain the offset to the first element's data
//   - The number of elements is derived by dividing this offset by 4
//   - Each subsequent 4-byte value is an offset to the next element
//   - Element data appears after all offsets, in order
//
// Parameters:
//   - targetType: The TypeDescriptor with slice metadata
//   - targetValue: The reflect.Value where the slice will be stored
//   - ssz: The SSZ-encoded data containing offsets and elements
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed (should equal len(ssz))
//   - error: An error if offsets are invalid or decoding fails
//
// The function validates that:
//   - Offsets are monotonically increasing
//   - No offset points outside the data bounds
//   - Each element consumes exactly the expected bytes

func (d *DynSsz) unmarshalDynamicSlice(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	if len(ssz) == 0 {
		return 0, nil
	}

	// derive number of items from first item offset
	firstOffset := readOffset(ssz[0:4])
	sliceLen := int(firstOffset / 4)

	// read all item offsets
	sliceOffsets := make([]int, sliceLen)
	sliceOffsets[0] = int(firstOffset)
	for i := 1; i < sliceLen; i++ {
		sliceOffsets[i] = int(readOffset(ssz[i*4 : (i+1)*4]))
	}

	fieldType := targetType.ElemDesc

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.IsPtr {
		fieldT = fieldT.Elem()
	}

	var newValue reflect.Value
	if targetType.Kind == reflect.Array {
		newValue = reflect.New(fieldT).Elem()
	} else {
		newValue = reflect.MakeSlice(fieldT, sliceLen, sliceLen)
	}

	offset := int(firstOffset)
	sszLen := len(ssz)

	if sliceLen > 0 {
		// decode slice items
		for i := 0; i < sliceLen; i++ {
			var itemVal reflect.Value
			if fieldType.IsPtr {
				// fmt.Printf("new slice item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType.Type.Elem())
				newValue.Index(i).Set(itemVal)
			} else {
				itemVal = newValue.Index(i)
			}

			startOffset := sliceOffsets[i]
			var endOffset int
			if i == sliceLen-1 {
				endOffset = sszLen
			} else {
				endOffset = sliceOffsets[i+1]
			}

			itemSize := endOffset - startOffset
			if itemSize < 0 || endOffset > sszLen {
				return 0, ErrOffset
			}

			itemSsz := ssz[startOffset:endOffset]

			consumed, err := d.unmarshalType(fieldType, itemVal, itemSsz, idt+2)
			if err != nil {
				return 0, err
			}
			if consumed != itemSize {
				return 0, fmt.Errorf("dynamic slice item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
			}

			offset += itemSize
		}
	}

	targetValue.Set(newValue)

	return offset, nil
}

// unmarshalString decodes SSZ-encoded data into a Go string value.
//
// Strings in SSZ can be either fixed-size or dynamic:
//   - Fixed-size strings: Read exact number of bytes (including any null bytes)
//   - Dynamic strings: Use all available bytes
//
// Parameters:
//   - targetType: The TypeDescriptor containing string metadata and size constraints
//   - targetValue: The reflect.Value where the decoded string will be stored
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: The number of bytes consumed from the SSZ data
//   - error: An error if decoding fails (e.g., insufficient data for fixed-size string)
//
// For fixed-size strings, the function reads exactly the specified number of bytes
// without any trimming. For dynamic strings, all available bytes are used.
func (d *DynSsz) unmarshalString(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	consumedBytes := 0

	// Handle fixed-size vs dynamic strings
	if targetType.Size > 0 {
		// Fixed-size string: read exact number of bytes
		fixedSize := int(targetType.Size)
		if len(ssz) < fixedSize {
			return 0, fmt.Errorf("not enough data for fixed-size string: need %d bytes, have %d", fixedSize, len(ssz))
		}
		// Read the fixed-size bytes without trimming
		stringBytes := ssz[:fixedSize]
		targetValue.SetString(string(stringBytes))
		consumedBytes = fixedSize
	} else {
		// Dynamic string: use all available bytes
		targetValue.SetString(string(ssz))
		consumedBytes = len(ssz)
	}

	return consumedBytes, nil
}
