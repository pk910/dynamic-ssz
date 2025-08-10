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

	useFastSsz := !d.NoFastSsz && targetType.HasFastSSZMarshaler && !targetType.HasDynamicSize
	if !useFastSsz && targetType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), targetType.Type.Name(), targetType.Kind, useFastSsz, targetType.HasFastSSZMarshaler, targetType.HasDynamicSize)
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
		var err error
		switch targetType.SszType {
		// complex types
		case SszContainerType, SszProgressiveContainerType:
			consumedBytes, err = d.unmarshalContainer(targetType, targetValue, ssz, idt)
			if err != nil {
				return 0, err
			}
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			if targetType.ElemDesc.IsDynamic {
				consumedBytes, err = d.unmarshalDynamicVector(targetType, targetValue, ssz, idt)
			} else {
				consumedBytes, err = d.unmarshalVector(targetType, targetValue, ssz, idt)
			}
			if err != nil {
				return 0, err
			}
		case SszListType, SszBitlistType, SszProgressiveListType, SszProgressiveBitlistType:
			if targetType.ElemDesc.IsDynamic {
				consumedBytes, err = d.unmarshalDynamicList(targetType, targetValue, ssz, idt)
			} else {
				consumedBytes, err = d.unmarshalList(targetType, targetValue, ssz, idt)
			}
			if err != nil {
				return 0, err
			}
		case SszCompatibleUnionType:
			consumedBytes, err = d.unmarshalCompatibleUnion(targetType, targetValue, ssz, idt)
			if err != nil {
				return 0, err
			}

		// primitive types
		case SszBoolType:
			targetValue.SetBool(unmarshalBool(ssz))
			consumedBytes = 1
		case SszUint8Type:
			targetValue.SetUint(uint64(unmarshallUint8(ssz)))
			consumedBytes = 1
		case SszUint16Type:
			targetValue.SetUint(uint64(unmarshallUint16(ssz)))
			consumedBytes = 2
		case SszUint32Type:
			targetValue.SetUint(uint64(unmarshallUint32(ssz)))
			consumedBytes = 4
		case SszUint64Type:
			targetValue.SetUint(uint64(unmarshallUint64(ssz)))
			consumedBytes = 8

		default:
			return 0, fmt.Errorf("unknown type: %v", targetType)
		}
	}

	return consumedBytes, nil
}

// unmarshalContainer decodes SSZ-encoded container data.
//
// This function implements the SSZ specification for container decoding, which requires:
//   - Fixed-size fields appear first in the encoding
//   - Variable-size fields are referenced by 4-byte offsets in the fixed section
//   - Variable-size field data appears after all fixed fields
//
// The function uses the pre-computed TypeDescriptor to efficiently navigate the container's
// layout without repeated reflection calls.
//
// Parameters:
//   - targetType: The TypeDescriptor containing container field metadata
//   - targetValue: The reflect.Value of the container to populate
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed from the SSZ data
//   - error: An error if decoding fails or data is malformed
//
// The function validates offset integrity to ensure variable fields don't overlap
// and that all data is consumed correctly.

func (d *DynSsz) unmarshalContainer(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	offset := 0
	dynamicFieldCount := len(targetType.ContainerDesc.DynFields)
	dynamicOffsets := make([]int, 0, dynamicFieldCount)
	sszSize := len(ssz)

	for i := 0; i < len(targetType.ContainerDesc.Fields); i++ {
		field := targetType.ContainerDesc.Fields[i]

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
				return 0, fmt.Errorf("container field did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, fieldSize)
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
	for i, field := range targetType.ContainerDesc.DynFields {
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
		fieldValue := targetValue.Field(int(field.Index))
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

// unmarshalVector decodes SSZ-encoded vector data.
//
// Vectors in SSZ are encoded as fixed-size sequences. Since the vector length is known
// from the type, the function can calculate each element's size by dividing the total
// SSZ data length by the vector length.
//
// Parameters:
//   - targetType: The TypeDescriptor containing vector metadata
//   - targetValue: The reflect.Value of the vector to populate
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed from the SSZ data
//   - error: An error if decoding fails
//
// Special handling:
//   - Byte arrays use reflect.Copy for efficient bulk copying
//   - Pointer elements are automatically initialized
//   - Each element must consume exactly itemSize bytes

func (d *DynSsz) unmarshalVector(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	var consumedBytes int

	fieldType := targetType.ElemDesc
	arrLen := int(targetType.Len)

	var newValue reflect.Value
	switch targetType.Kind {
	case reflect.Slice:
		newValue = reflect.MakeSlice(targetType.Type, arrLen, arrLen)
	case reflect.Array:
		newValue = targetValue
	default:
		newValue = reflect.New(targetType.Type).Elem()
	}

	if targetType.IsString {
		newValue.SetString(string(ssz[0:arrLen]))
		consumedBytes = arrLen
	} else if targetType.IsByteArray {
		// shortcut for performance: use copy on []byte arrays
		reflect.Copy(newValue, reflect.ValueOf(ssz[0:arrLen]))
		consumedBytes = arrLen
	} else {
		offset := 0
		itemSize := len(ssz) / arrLen
		for i := 0; i < arrLen; i++ {
			var itemVal reflect.Value
			if fieldType.IsPtr {
				// fmt.Printf("new array item %v\n", fieldType.Name())
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
				return 0, fmt.Errorf("unmarshalling vector item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
			}

			offset += itemSize
		}

		consumedBytes = offset
	}

	if targetType.Kind != reflect.Array {
		targetValue.Set(newValue)
	}

	return consumedBytes, nil
}

// unmarshalDynamicVector decodes vectors with variable-size elements from SSZ format.
//
// For vectors with variable-size elements, SSZ uses an offset-based encoding:
//   - The given number of offsets are decoded first, 4 bytes each
//   - Element data appears after all offsets, in order
//
// Parameters:
//   - targetType: The TypeDescriptor with vector metadata
//   - targetValue: The reflect.Value where the vector will be stored
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

func (d *DynSsz) unmarshalDynamicVector(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	if len(ssz) == 0 {
		return 0, nil
	}

	vectorLen := int(targetType.Len)

	// read all item offsets
	sliceOffsets := make([]int, vectorLen)
	for i := 0; i < vectorLen; i++ {
		sliceOffsets[i] = int(readOffset(ssz[i*4 : (i+1)*4]))
	}

	fieldType := targetType.ElemDesc

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.IsPtr {
		fieldT = fieldT.Elem()
	}

	offset := sliceOffsets[0]
	if offset != vectorLen*4 {
		return 0, fmt.Errorf("dynamic vector offset of first item does not match expected offset (offset: %v, expected: %v)", offset, vectorLen*4)
	}

	var newValue reflect.Value
	if targetType.Kind == reflect.Array {
		newValue = targetValue
	} else {
		newValue = reflect.MakeSlice(fieldT, vectorLen, vectorLen)
	}

	sszLen := len(ssz)

	// decode slice items
	for i := 0; i < vectorLen; i++ {
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
		if i == vectorLen-1 {
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
			return 0, fmt.Errorf("dynamic vector item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
		}

		offset += itemSize
	}

	targetValue.Set(newValue)

	return offset, nil
}

// unmarshalList decodes SSZ-encoded list data.
//
// This function handles lists with fixed-size elements. For lists with variable-size
// elements, it delegates to unmarshalDynamicList. The list length is determined by
// dividing the SSZ data length by the element size.
//
// Parameters:
//   - targetType: The TypeDescriptor containing list metadata
//   - targetValue: The reflect.Value where the list will be stored
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed (should equal len(ssz))
//   - error: An error if decoding fails or data length is invalid
//
// The function:
//   - Handles both fixed-size and variable-size elements
//   - Delegate to unmarshalDynamicList for variable-size elements
//   - Uses optimized copying for byte lists
//   - Validates that each element consumes exactly the expected bytes

func (d *DynSsz) unmarshalList(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	var consumedBytes int

	fieldType := targetType.ElemDesc
	sszLen := len(ssz)

	// Calculate slice length once
	itemSize := int(fieldType.Size)
	sliceLen, ok := divideInt(sszLen, itemSize)
	if !ok {
		return 0, fmt.Errorf("invalid list length, expected multiple of %v, got %v", itemSize, sszLen)
	}

	// slice with static size items
	// fmt.Printf("new slice %v  %v\n", fieldType.Name(), sliceLen)

	fieldT := targetType.Type
	if targetType.IsPtr {
		fieldT = fieldT.Elem()
	}

	var newValue reflect.Value
	if targetType.Kind == reflect.Slice {
		newValue = reflect.MakeSlice(fieldT, sliceLen, sliceLen)
	} else {
		newValue = reflect.New(fieldT).Elem()
	}

	if targetType.IsString {
		newValue.SetString(string(ssz))
		consumedBytes = len(ssz)
	} else if targetType.IsByteArray {
		// shortcut for performance: use copy on []byte arrays
		reflect.Copy(newValue, reflect.ValueOf(ssz[0:sliceLen]))
		consumedBytes = sliceLen
	} else {
		offset := 0
		if sliceLen > 0 {
			// decode list items
			for i := 0; i < sliceLen; i++ {
				var itemVal reflect.Value
				if fieldType.IsPtr {
					// fmt.Printf("new list item %v\n", fieldType.Name())
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
					return 0, fmt.Errorf("list item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
				}

				offset += itemSize
			}
		}

		consumedBytes = offset
	}

	targetValue.Set(newValue)

	return consumedBytes, nil
}

// unmarshalDynamicList decodes lists with variable-size elements from SSZ format.
//
// For lists with variable-size elements, SSZ uses an offset-based encoding:
//   - The first 4 bytes contain the offset to the first element's data
//   - The number of elements is derived by dividing this offset by 4
//   - Each subsequent 4-byte value is an offset to the next element
//   - Element data appears after all offsets, in order
//
// Parameters:
//   - targetType: The TypeDescriptor with list metadata
//   - targetValue: The reflect.Value where the list will be stored
//   - ssz: The SSZ-encoded data containing offsets and elements
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed from the SSZ data
//   - error: An error if offsets are invalid or decoding fails
//
// The function validates that:
//   - Offsets are monotonically increasing
//   - No offset points outside the data bounds
//   - Each element consumes exactly the expected bytes

func (d *DynSsz) unmarshalDynamicList(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
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
	if targetType.Kind == reflect.Slice {
		newValue = reflect.MakeSlice(fieldT, sliceLen, sliceLen)
	} else {
		newValue = reflect.New(fieldT).Elem()
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
				return 0, fmt.Errorf("dynamic list item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
			}

			offset += itemSize
		}
	}

	targetValue.Set(newValue)

	return offset, nil
}

// unmarshalCompatibleUnion decodes SSZ-encoded data into a CompatibleUnion.
//
// According to the spec:
// - The encoding is: selector.to_bytes(1, "little") + serialize(value.data)
// - The selector index is based at 0 if a ProgressiveContainer type option is present
// - Otherwise, it is based at 1
//
// Parameters:
//   - targetType: The TypeDescriptor containing union metadata and variant descriptors
//   - targetValue: The reflect.Value of the CompatibleUnion to populate
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed
//   - error: An error if decoding fails
func (d *DynSsz) unmarshalCompatibleUnion(targetType *TypeDescriptor, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	if len(ssz) < 1 {
		return 0, fmt.Errorf("CompatibleUnion requires at least 1 byte for selector")
	}

	// Read the selector byte
	selector := ssz[0]

	// Adjust selector to variant index based on whether first variant is ProgressiveContainer
	variant := selector
	if len(targetType.UnionVariants) > 0 {
		// Check if the first variant (index 0) is a ProgressiveContainer
		firstVariant, hasFirst := targetType.UnionVariants[0]
		if !hasFirst || firstVariant.SszType != SszProgressiveContainerType {
			// No ProgressiveContainer at index 0, so selector is based at 1
			if selector == 0 {
				return 0, fmt.Errorf("invalid selector value 0 when union is 1-indexed")
			}
			variant = selector - 1
		}
	}

	// Get the variant descriptor
	variantDesc, ok := targetType.UnionVariants[variant]
	if !ok {
		return 0, fmt.Errorf("unknown union variant index: %d (selector: %d)", variant, selector)
	}

	// Create a new value of the variant type
	variantValue := reflect.New(variantDesc.Type).Elem()

	// Unmarshal the data
	consumed, err := d.unmarshalType(variantDesc, variantValue, ssz[1:], idt+2)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal union variant %d: %w", variant, err)
	}

	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	targetValue.Field(0).SetUint(uint64(variant))
	targetValue.Field(1).Set(variantValue)

	return consumed + 1, nil // +1 for the selector byte
}
