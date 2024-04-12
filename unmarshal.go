// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
)

// unmarshalType decodes SSZ-encoded data into a Go value based on reflection. It serves as the
// recursive core of the dynamic SSZ decoding process, handling both primitive and composite types.
//
// Parameters:
// - targetType: The reflect.Type of the value to be decoded. This provides the necessary
//   type information for reflection-based decoding.
// - targetValue: The reflect.Value where the decoded data should be stored. This function
//   directly modifies targetValue to set the decoded values.
// - ssz: A byte slice containing the SSZ-encoded data to be decoded.
// - sizeHints: A slice of sszSizeHint, which contains size hints for decoding dynamic sizes
//   within the SSZ data. These hints are populated from 'ssz-size' and 'dynssz-size' tag annotations
//   from parent structures, which are crucial for correctly decoding types like slices and arrays
//   with dynamic lengths.
// - idt: An indentation level used for debugging or logging purposes, helping track the recursion depth.
//
// Returns:
// - The number of bytes consumed from the SSZ data for the current decoding operation. This helps
//   subsequent decoding steps to know where in the SSZ data to start decoding the next piece of data.
// - An error, if the decoding fails at any point due to reasons such as type mismatches, unexpected
//   SSZ data length, etc.
//
// This function directly handles the decoding of primitive types by interpreting the SSZ data according
// to the targetType. For composite types (e.g., structs, arrays, slices), it delegates to more specific
// unmarshal functions (like unmarshalStruct, unmarshalArray) tailored to each type. It uses recursion
// to navigate and decode nested structures, ensuring every part of the targetValue is correctly populated
// with data from the SSZ input.

func (d *DynSsz) unmarshalType(targetType reflect.Type, targetValue reflect.Value, ssz []byte, sizeHints []sszSizeHint, idt int) (int, error) {
	consumedBytes := 0

	if targetType.Kind() == reflect.Ptr {
		// target is a pointer type, resolve type & value to actual value type
		targetType = targetType.Elem()
		if targetValue.IsNil() {
			// create new instance of target type for null pointers
			newValue := reflect.New(targetType)
			targetValue.Set(newValue)
		}
		targetValue = targetValue.Elem()
	}

	// fmt.Printf("%stype: %s\t kind: %v\n", strings.Repeat(" ", idt), targetType.Name(), targetType.Kind())

	switch targetType.Kind() {
	case reflect.Struct:
		usedFastSsz := false

		// use fastssz to unmarshal structs if:
		// - struct implements fastssz Unmarshaller interface
		// - this structure or any child structure does not use spec specific field sizes
		fastsszCompat, err := d.getFastsszCompatibility(targetType)
		if err != nil {
			return 0, fmt.Errorf("failed checking fastssz compatibility: %v", err)
		}
		if !d.NoFastSsz && fastsszCompat.isUnmarshaler && !fastsszCompat.hasDynamicSpecValues {
			// fmt.Printf("%s fastssz for type %s: %v\n", strings.Repeat(" ", idt), targetType.Name(), hasSpecVals)
			unmarshaller, ok := targetValue.Addr().Interface().(fastsszUnmarshaler)
			if ok {
				err := unmarshaller.UnmarshalSSZ(ssz)
				if err != nil {
					return 0, err
				}
				consumedBytes = len(ssz)
				usedFastSsz = true
			}
		}

		if !usedFastSsz {
			// can't use fastssz, use dynamic unmarshaling
			consumed, err := d.unmarshalStruct(targetType, targetValue, ssz, idt)
			if err != nil {
				return 0, err
			}
			consumedBytes = consumed
		}
	case reflect.Array:
		consumed, err := d.unmarshalArray(targetType, targetValue, ssz, sizeHints, idt)
		if err != nil {
			return 0, err
		}
		consumedBytes = consumed
	case reflect.Slice:
		consumed, err := d.unmarshalSlice(targetType, targetValue, ssz, sizeHints, idt)
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

	return consumedBytes, nil
}

// unmarshalStruct decodes SSZ-encoded data into a Go struct by calculating field offsets within the SSZ stream
// and delegating the type-specific decoding of each field to the generic unmarshalType function.
//
// Parameters:
// - targetType: The reflect.Type of the struct to be decoded, providing necessary type information for decoding.
// - targetValue: The reflect.Value where the decoded data is stored. This function prepares each struct field for decoding by unmarshalType,
//   based on their calculated offsets within the SSZ data.
// - ssz: A byte slice containing the SSZ-encoded data to be decoded.
// - idt: An indentation level, primarily used for debugging or logging to track recursion depth and field processing order.
//
// Returns:
// - The total number of bytes consumed from the SSZ data for decoding the struct. This information is crucial for the decoding of subsequent
//   data structures or fields.
// - An error, if the decoding process encounters any issues, such as incorrect SSZ format or mismatches between the SSZ data and targetType.
//
// The function's core responsibility is to navigate the struct's layout in the SSZ-encoded data, adjusting SSZ slices for each field and
// invoking unmarshalType with these parameters. This strategy efficiently decouples structural navigation from type-specific decoding logic.

func (d *DynSsz) unmarshalStruct(targetType reflect.Type, targetValue reflect.Value, ssz []byte, idt int) (int, error) {
	offset := 0
	dynamicFields := []*reflect.StructField{}
	dynamicOffsets := []int{}
	dynamicSizeHints := [][]sszSizeHint{}
	sszSize := len(ssz)

	for i := 0; i < targetType.NumField(); i++ {
		field := targetType.Field(i)

		fieldSize, _, sizeHints, err := d.getSszFieldSize(&field)
		if err != nil {
			return 0, err
		}

		if fieldSize > 0 {
			// static size field
			if offset+fieldSize > sszSize {
				return 0, fmt.Errorf("unexpected end of SSZ. field %v expects %v bytes, got %v", field.Name, fieldSize, sszSize-offset)
			}

			// fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)

			fieldSsz := ssz[offset : offset+fieldSize]
			fieldValue := targetValue.Field(i)
			consumedBytes, err := d.unmarshalType(field.Type, fieldValue, fieldSsz, sizeHints, idt+2)
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

			// store dynamic fields for later
			dynamicFields = append(dynamicFields, &field)
			dynamicOffsets = append(dynamicOffsets, int(fieldOffset))
			dynamicSizeHints = append(dynamicSizeHints, sizeHints)
		}
		offset += fieldSize
	}

	// finished parsing the static size fields, process dynamic fields
	dynamicFieldCount := len(dynamicFields)
	for i, field := range dynamicFields {
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

		fieldValue := targetValue.Field(field.Index[0])
		consumedBytes, err := d.unmarshalType(field.Type, fieldValue, fieldSsz, dynamicSizeHints[i], idt+2)
		if err != nil {
			return 0, fmt.Errorf("failed decoding field %v: %v", field.Name, err)
		}
		if consumedBytes != endOffset-startOffset {
			return 0, fmt.Errorf("struct field did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, endOffset-startOffset)
		}

		offset += consumedBytes
	}

	return offset, nil
}

// unmarshalArray decodes SSZ-encoded data into a Go array, calculating the necessary offsets for each element within the SSZ stream
// and delegating the decoding of each element's type to the generic unmarshalType function.
//
// Parameters:
// - targetType: The reflect.Type of the array to be decoded, providing the type information needed for decoding the array elements.
// - targetValue: The reflect.Value where the decoded array data should be stored. This function prepares each element of the array
//   for decoding by unmarshalType, based on their calculated offsets within the SSZ data.
// - ssz: A byte slice containing the SSZ-encoded data to be decoded into the array.
// - sizeHints: A slice of sszSizeHint populated from 'ssz-size' and 'dynssz-size' tag annotations from parent structures,
//   essential for decoding arrays with elements that have dynamic lengths.
// - idt: An indentation level, used for debugging or logging to aid in tracking the recursion depth and element processing order.
//
// Returns:
// - The number of bytes consumed from the SSZ data for decoding the array. This metric is crucial for the decoding process of
//   subsequent structures or elements.
// - An error, if the decoding process encounters any issues such as incorrect SSZ format, mismatches between the SSZ data
//   and targetType, etc.
//
// unmarshalArray navigates the layout of the array in the SSZ-encoded data, adjusting SSZ slices for each element and
// invoking unmarshalType with these parameters for decoding. This division of tasks allows unmarshalArray to focus
// on the structural navigation within the SSZ data, while unmarshalType applies the specific decoding logic for the type of each element.

func (d *DynSsz) unmarshalArray(targetType reflect.Type, targetValue reflect.Value, ssz []byte, sizeHints []sszSizeHint, idt int) (int, error) {
	var consumedBytes int

	childSizeHints := []sszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	fieldType := targetType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	arrLen := targetType.Len()
	if fieldType == byteType {
		// shortcut for performance: use copy on []byte arrays
		reflect.Copy(targetValue, reflect.ValueOf(ssz[0:arrLen]))
		consumedBytes = arrLen
	} else {
		offset := 0
		itemSize := len(ssz) / arrLen
		for i := 0; i < arrLen; i++ {
			var itemVal reflect.Value
			if fieldIsPtr {
				// fmt.Printf("new array item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType).Elem()
				targetValue.Index(i).Set(itemVal.Addr())
			} else {
				itemVal = targetValue.Index(i)
			}

			itemSsz := ssz[offset : offset+itemSize]

			consumed, err := d.unmarshalType(fieldType, itemVal, itemSsz, childSizeHints, idt+2)
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

// unmarshalSlice decodes SSZ-encoded data into a Go slice with static length items, calculating offsets within the SSZ stream
// and delegating the decoding of each element's type to the generic unmarshalType function. For slices containing elements
// with dynamic sizes, it internally forwards the call to unmarshalDynamicSlice to handle the variability in element sizes.
//
// Parameters:
// - targetType: The reflect.Type of the slice to be decoded, providing the type information necessary for decoding the slice elements.
// - targetValue: The reflect.Value where the decoded slice data should be stored. This function prepares each element of the slice
//   for decoding by unmarshalType, based on their calculated offsets within the SSZ data, or forwards to unmarshalDynamicSlice
//   if the elements require dynamic size handling.
// - ssz: A byte slice containing the SSZ-encoded data to be decoded into the slice.
// - sizeHints: A slice of sszSizeHint, populated from 'ssz-size' and 'dynssz-size' tag annotations from parent structures,
//   crucial for decoding slices and elements that have dynamic lengths.
// - idt: An indentation level, primarily used for debugging or logging to facilitate tracking of the recursion depth and element processing order.
//
// Returns:
// - The number of bytes consumed from the SSZ data for decoding the slice. This figure is vital for the decoding of subsequent
//   data structures or elements.
// - An error, if any issues arise during the decoding process, such as incorrect SSZ format, mismatches between the SSZ data
//   and targetType, etc.
//
// unmarshalSlice effectively handles slices by navigating their layout within the SSZ-encoded data, adjusting SSZ slices for each
// element, and invoking unmarshalType for the decoding. When faced with elements of dynamic size, it seamlessly transitions to
// unmarshalDynamicSlice, ensuring all elements, regardless of their size variability, are accurately decoded.

func (d *DynSsz) unmarshalSlice(targetType reflect.Type, targetValue reflect.Value, ssz []byte, sizeHints []sszSizeHint, idt int) (int, error) {
	var consumedBytes int

	childSizeHints := []sszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	fieldType := targetType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	sliceLen := 0
	sszLen := len(ssz)

	// check if slice has dynamic size items
	size, _, err := d.getSszSize(fieldType, childSizeHints)
	if err != nil {
		return 0, err
	}

	if size > 0 {
		ok := false
		sliceLen, ok = divideInt(sszLen, size)
		if !ok {
			return 0, fmt.Errorf("invalid slice length, expected multiple of %v, got %v", size, sszLen)
		}
	} else if len(ssz) > 0 {
		// slice with dynamic size items
		return d.unmarshalDynamicSlice(targetType, targetValue, ssz, childSizeHints, idt)
	}

	// slice with static size items
	// fmt.Printf("new slice %v  %v\n", fieldType.Name(), sliceLen)
	newValue := reflect.MakeSlice(targetType, sliceLen, sliceLen)
	targetValue.Set(newValue)

	if fieldType == byteType {
		// shortcut for performance: use copy on []byte arrays
		reflect.Copy(newValue, reflect.ValueOf(ssz[0:sliceLen]))
		consumedBytes = sliceLen
	} else {
		offset := 0
		if sliceLen > 0 {
			itemSize := sszLen / sliceLen

			// decode slice items
			for i := 0; i < sliceLen; i++ {
				var itemVal reflect.Value
				if fieldIsPtr {
					// fmt.Printf("new slice item %v\n", fieldType.Name())
					itemVal = reflect.New(fieldType).Elem()
					newValue.Index(i).Set(itemVal.Addr())
				} else {
					itemVal = newValue.Index(i)
				}

				itemSsz := ssz[offset : offset+itemSize]

				consumed, err := d.unmarshalType(fieldType, itemVal, itemSsz, childSizeHints, idt+2)
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

// unmarshalDynamicSlice decodes SSZ-encoded data into a Go slice with dynamically sized items, leveraging the offsets encoded
// within the SSZ stream itself to navigate and decode each element. This method is essential for accurately handling slices where
// element sizes can vary, such as slices of slices, slices of arrays with dynamic sizes, or slices of structs containing dynamically
// sized fields.
//
// Parameters:
// - targetType: The reflect.Type of the slice to be decoded, providing the type information necessary for decoding the dynamically
//   sized elements.
// - targetValue: The reflect.Value where the decoded data will be stored, populated with the decoded elements of the slice as
//   determined by the SSZ data's internal offsets.
// - ssz: A byte slice containing the SSZ-encoded data to be decoded into the slice.
// - sizeHints: A slice of sszSizeHint, derived from 'ssz-size' and 'dynssz-size' tag annotations from parent structures. While this
//   function primarily uses encoded offsets for decoding, sizeHints may still play a role in certain contexts, particularly when
//   dealing with nested dynamic structures.
// - idt: An indentation level, used primarily for debugging or logging purposes, to facilitate tracking of the decoding process's
//   depth and sequence.
//
// Returns:
// - The number of bytes consumed from the SSZ data during the decoding process. This information is crucial for correctly parsing
//   any subsequent structures or fields.
// - An error if any issues arise during decoding, such as mismatches between the SSZ data and the expected targetType, incorrect
//   SSZ format, or inconsistencies with the expected sizes and the encoded offsets.
//
// By directly utilizing the offsets encoded within the SSZ stream, unmarshalDynamicSlice ensures precise decoding of each element
// within a dynamic slice. This method efficiently handles the complexity of variable-sized elements, ensuring the integrity and
// intended structure of the decoded data are maintained.

func (d *DynSsz) unmarshalDynamicSlice(targetType reflect.Type, targetValue reflect.Value, ssz []byte, sizeHints []sszSizeHint, idt int) (int, error) {
	// derive number of items from first item offset
	firstOffset := readOffset(ssz[0:4])
	sliceLen := int(firstOffset / 4)

	// read all item offsets
	sliceOffsets := make([]int, sliceLen)
	sliceOffsets[0] = int(firstOffset)
	for i := 1; i < sliceLen; i++ {
		sliceOffsets[i] = int(readOffset(ssz[i*4 : (i+1)*4]))
	}

	fieldType := targetType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	newValue := reflect.MakeSlice(targetType, sliceLen, sliceLen)
	targetValue.Set(newValue)

	offset := int(firstOffset)
	if sliceLen > 0 {
		// decode slice items
		for i := 0; i < sliceLen; i++ {
			var itemVal reflect.Value
			if fieldIsPtr {
				// fmt.Printf("new slice item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType).Elem()
				newValue.Index(i).Set(itemVal.Addr())
			} else {
				itemVal = newValue.Index(i)
			}

			startOffset := sliceOffsets[i]
			endOffset := 0
			if i == sliceLen-1 {
				endOffset = len(ssz)
			} else {
				endOffset = sliceOffsets[i+1]
			}
			itemSize := endOffset - startOffset

			itemSsz := ssz[startOffset:endOffset]

			consumed, err := d.unmarshalType(fieldType, itemVal, itemSsz, sizeHints, idt+2)
			if err != nil {
				return 0, err
			}
			if consumed != itemSize {
				return 0, fmt.Errorf("dynamic slice item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
			}

			offset += itemSize
		}
	}

	return offset, nil

}
