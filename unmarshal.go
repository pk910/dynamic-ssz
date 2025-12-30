// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"fmt"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// unmarshalType is the core recursive generic function for decoding SSZ-encoded data into Go values.
//
// This function serves as the primary dispatcher within the unmarshalling process, handling both
// primitive and composite types. It uses the TypeDescriptor's metadata to determine the most
// efficient decoding path, automatically leveraging fastssz when possible for optimal performance.
//
// The generic type parameter D allows the compiler to generate specialized code for each decoder
// implementation, eliminating interface dispatch overhead.
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor containing optimized metadata about the type to decode
//   - targetValue: The reflect.Value where decoded data will be stored
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging (when enabled)
//
// Returns:
//   - error: An error if decoding fails
//
// The function handles:
//   - Automatic nil pointer initialization
//   - FastSSZ delegation for compatible types without dynamic sizing
//   - Primitive type decoding (bool, uint8, uint16, uint32, uint64)
//   - Delegation to specialized functions for composite types (structs, arrays, slices)
//   - Validation that consumed bytes match expected sizes
func unmarshalType[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		// target is a pointer type, resolve type & value to actual value type
		if targetValue.IsNil() {
			// create new instance of target type for null pointers
			newValue := reflect.New(targetType.Type.Elem())
			targetValue.Set(newValue)
		}
		targetValue = targetValue.Elem()
	}

	hasDynamicSize := targetType.SszTypeFlags&SszTypeFlagHasDynamicSize != 0
	isFastsszUnmarshaler := targetType.SszCompatFlags&SszCompatFlagFastSSZMarshaler != 0
	useDynamicUnmarshal := targetType.SszCompatFlags&SszCompatFlagDynamicUnmarshaler != 0
	useDynamicDecoder := targetType.SszCompatFlags&SszCompatFlagDynamicDecoder != 0
	useFastSsz := !d.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
	if !useFastSsz && targetType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		d.LogCb("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), targetType.Type.Name(), targetType.Kind, useFastSsz, isFastsszUnmarshaler, hasDynamicSize)
	}

	if useFastSsz {
		unmarshaller, ok := targetValue.Addr().Interface().(sszutils.FastsszUnmarshaler)
		if ok {
			sszLen := decoder.GetLength()
			if targetType.Size > 0 {
				sszLen = int(targetType.Size)
			}
			sszBuf, err := decoder.DecodeBytesBuf(sszLen)
			if err != nil {
				return err
			}

			err = unmarshaller.UnmarshalSSZ(sszBuf)
			if err != nil {
				return err
			}
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz && useDynamicDecoder {
		if decoder.CanSeek() && useDynamicUnmarshal {
			// prefer static unmarshaller for non-seekable decoders (buffer based)
			useDynamicDecoder = false
		} else if sszDecoder, ok := targetValue.Addr().Interface().(sszutils.DynamicDecoder); ok {
			err := sszDecoder.UnmarshalSSZDecoder(d, decoder)
			if err != nil {
				return err
			}
		} else {
			useDynamicDecoder = false
		}
	}

	if !useFastSsz && !useDynamicDecoder && useDynamicUnmarshal {
		// Use dynamic unmarshaler - can always be used even with dynamic specs
		unmarshaller, ok := targetValue.Addr().Interface().(sszutils.DynamicUnmarshaler)
		if ok {
			sszLen := decoder.GetLength()
			if targetType.Size > 0 {
				sszLen = int(targetType.Size)
			}

			sszBuf, err := decoder.DecodeBytesBuf(sszLen)
			if err != nil {
				return err
			}

			err = unmarshaller.UnmarshalSSZDyn(d, sszBuf)
			if err != nil {
				return err
			}
		} else {
			useDynamicUnmarshal = false
		}
	}

	if !useFastSsz && !useDynamicDecoder && !useDynamicUnmarshal {
		// can't use fastssz, use dynamic unmarshaling
		var err error
		switch targetType.SszType {
		// complex types
		case SszTypeWrapperType:
			err = unmarshalTypeWrapper(d, targetType, targetValue, decoder, idt)
			if err != nil {
				return err
			}
		case SszContainerType, SszProgressiveContainerType:
			err = unmarshalContainer(d, targetType, targetValue, decoder, idt)
			if err != nil {
				return err
			}
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			if targetType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = unmarshalDynamicVector(d, targetType, targetValue, decoder, idt)
			} else {
				err = unmarshalVector(d, targetType, targetValue, decoder, idt)
			}
			if err != nil {
				return err
			}
		case SszListType, SszProgressiveListType:
			if targetType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = unmarshalDynamicList(d, targetType, targetValue, decoder, idt)
			} else {
				err = unmarshalList(d, targetType, targetValue, decoder, idt)
			}
			if err != nil {
				return err
			}
		case SszBitlistType, SszProgressiveBitlistType:
			err = unmarshalBitlist(d, targetType, targetValue, decoder)
			if err != nil {
				return err
			}
		case SszCompatibleUnionType:
			err = unmarshalCompatibleUnion(d, targetType, targetValue, decoder, idt)
			if err != nil {
				return err
			}

		// primitive types
		case SszBoolType:
			val, err := decoder.DecodeBool()
			if err != nil {
				return err
			}
			targetValue.SetBool(val)
		case SszUint8Type:
			val, err := decoder.DecodeUint8()
			if err != nil {
				return err
			}
			targetValue.SetUint(uint64(val))
		case SszUint16Type:
			val, err := decoder.DecodeUint16()
			if err != nil {
				return err
			}
			targetValue.SetUint(uint64(val))
		case SszUint32Type:
			val, err := decoder.DecodeUint32()
			if err != nil {
				return err
			}
			targetValue.SetUint(uint64(val))
		case SszUint64Type:
			val, err := decoder.DecodeUint64()
			if err != nil {
				return err
			}

			if targetType.GoTypeFlags&GoTypeFlagIsTime != 0 {
				timeVal := time.Unix(int64(val), 0)
				var timeRefVal reflect.Value
				if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
					timeRefVal = reflect.New(targetType.Type.Elem())
					timeRefVal.Elem().Set(reflect.ValueOf(timeVal))
				} else {
					timeRefVal = reflect.ValueOf(timeVal)
				}

				targetValue.Set(timeRefVal)
			} else {
				targetValue.SetUint(uint64(val))
			}
		default:
			return fmt.Errorf("unknown type: %v", targetType)
		}
	}

	return nil
}

// unmarshalTypeWrapper unmarshals a TypeWrapper by extracting the wrapped data and unmarshaling it as the wrapped type.
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor containing wrapper field metadata
//   - targetValue: The reflect.Value of the wrapper to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails or data is malformed
//
// The function validates that the Data field is present and unmarshals the wrapped value using its type descriptor.
func unmarshalTypeWrapper[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	if d.Verbose {
		d.LogCb("%sunmarshalTypeWrapper: %s\n", strings.Repeat(" ", idt), targetType.Type.Name())
	}

	// Get the Data field from the TypeWrapper
	dataField := targetValue.Field(0)

	// Unmarshal the wrapped value using its type descriptor
	err := unmarshalType(d, targetType.ElemDesc, dataField, decoder, idt+2)
	if err != nil {
		return err
	}

	return nil
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
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor containing container field metadata
//   - targetValue: The reflect.Value of the container to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails or data is malformed
//
// The function validates offset integrity to ensure variable fields don't overlap
// and that all data is consumed correctly.
func unmarshalContainer[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	canSeek := decoder.CanSeek()

	var dynamicOffsets []int
	var startPos int

	if canSeek {
		startPos = decoder.GetPosition()
	} else {
		dynamicOffsets = defaultOffsetSlicePool.Get()
		defer defaultOffsetSlicePool.Put(dynamicOffsets)
	}
	sszSize := uint32(decoder.GetLength())
	if sszSize < targetType.Len {
		return sszutils.ErrUnexpectedEOF
	}

	for i := 0; i < len(targetType.ContainerDesc.Fields); i++ {
		field := targetType.ContainerDesc.Fields[i]

		fieldSize := int(field.Type.Size)
		if fieldSize > 0 {
			// static size field
			// fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)
			expectedPos := decoder.GetPosition() + fieldSize

			fieldValue := targetValue.Field(i)
			err := unmarshalType(d, field.Type, fieldValue, decoder, idt+2)
			if err != nil {
				return fmt.Errorf("failed decoding field %v: %v", field.Name, err)
			}

			if decoder.GetPosition() != expectedPos {
				return fmt.Errorf("container field did not consume expected ssz range (pos: %v, expected: %v)", decoder.GetPosition(), expectedPos)
			}

		} else {
			// dynamic size field
			// get the 4 byte offset where the fields ssz range starts

			// fmt.Printf("%sfield %d:\t offset [%v:%v] %v\t %v \t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name, fieldOffset)

			if canSeek {
				decoder.SkipBytes(4)
			} else {
				fieldOffset, err := decoder.DecodeOffset()
				if err != nil {
					return err
				}

				// store dynamic field offset for later
				dynamicOffsets = append(dynamicOffsets, int(fieldOffset))
			}
		}
	}

	// finished parsing the static size fields, process dynamic fields
	dynamicFieldCount := len(targetType.ContainerDesc.DynFields)

	if dynamicFieldCount > 0 {
		var dynOffset uint32
		if canSeek {
			dynOffset = decoder.DecodeOffsetAt(startPos + int(targetType.ContainerDesc.DynFields[0].HeaderOffset))
		} else {
			dynOffset = uint32(dynamicOffsets[0])
		}

		if dynOffset != targetType.Len { // check first dynamic field offset
			return sszutils.ErrOffset
		}

		for i, field := range targetType.ContainerDesc.DynFields {
			startOffset := dynOffset

			var endOffset uint32
			if i < dynamicFieldCount-1 {
				if canSeek {
					dynOffset = decoder.DecodeOffsetAt(startPos + int(targetType.ContainerDesc.DynFields[i+1].HeaderOffset))
				} else {
					dynOffset = uint32(dynamicOffsets[i+1])
				}

				endOffset = dynOffset
			} else {
				endOffset = sszSize
			}

			// check offset integrity (not before previous field offset & not after range end)
			if endOffset > sszSize || endOffset < startOffset {
				return sszutils.ErrOffset
			}

			// fmt.Printf("%sfield %d:\t dynamic [%v:%v]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], startOffset, endOffset, field.Name)

			sszSize := endOffset - startOffset
			decoder.PushLimit(int(sszSize))

			fieldDescriptor := field.Field
			fieldValue := targetValue.Field(int(field.Index))
			err := unmarshalType(d, fieldDescriptor.Type, fieldValue, decoder, idt+2)
			if err != nil {
				return fmt.Errorf("failed decoding field %v: %v", fieldDescriptor.Name, err)
			}

			consumedDiff := decoder.PopLimit()
			if consumedDiff != 0 {
				return fmt.Errorf("struct field did not consume expected ssz range (diff: %v, expected: %v)", consumedDiff, sszSize)
			}
		}
	}

	return nil
}

// unmarshalVector decodes SSZ-encoded vector data.
//
// Vectors in SSZ are encoded as fixed-size sequences. Since the vector length is known
// from the type, the function can calculate each element's size by dividing the total
// SSZ data length by the vector length.
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor containing vector metadata
//   - targetValue: The reflect.Value of the vector to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails
//
// Special handling:
//   - Byte arrays use unsafe.Slice for efficient bulk copying without allocation
//   - Pointer elements are automatically initialized
//   - Each element must consume exactly itemSize bytes
func unmarshalVector[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	fieldType := targetType.ElemDesc
	arrLen := int(targetType.Len)

	var newValue reflect.Value
	switch targetType.Kind {
	case reflect.Slice:
		// Optimization: avoid reflect.MakeSlice for common byte slice types
		if targetType.GoTypeFlags&GoTypeFlagIsByteArray != 0 && targetType.ElemDesc.Type.Kind() == reflect.Uint8 {
			byteSlice := make([]byte, arrLen)
			newValue = reflect.ValueOf(byteSlice)
		} else {
			newValue = reflect.MakeSlice(targetType.Type, arrLen, arrLen)
		}
	case reflect.Array:
		newValue = targetValue
	default:
		newValue = reflect.New(targetType.Type).Elem()
	}

	if targetType.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		// shortcut for performance: use copy on []byte arrays

		if targetType.GoTypeFlags&GoTypeFlagIsString != 0 {
			buf, err := decoder.DecodeBytesBuf(arrLen)
			if err != nil {
				return err
			}
			newValue.SetString(string(buf))
		} else {
			var buf []byte
			if targetType.Kind == reflect.Array {
				// Use unsafe to avoid reflect.Value.Slice allocation
				ptr := unsafe.Pointer(newValue.UnsafeAddr())
				buf = unsafe.Slice((*byte)(ptr), arrLen)
			} else {
				buf = newValue.Bytes()
			}

			sszLen := decoder.GetLength()
			_, err := decoder.DecodeBytes(buf)
			if err != nil {
				return err
			}

			if targetType.BitSize > 0 && targetType.BitSize < uint32(sszLen)*8 {
				// check padding bits
				paddingMask := uint8((uint16(0xff) << (targetType.BitSize % 8)) & 0xff)
				paddingBits := buf[arrLen-1] & paddingMask
				if paddingBits != 0 {
					return fmt.Errorf("bitvector padding bits are not zero")
				}
			}
		}
	} else {
		itemSize := int(fieldType.Size)

		for i := 0; i < arrLen; i++ {
			var itemVal reflect.Value
			if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				// fmt.Printf("new array item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType.Type.Elem())
				newValue.Index(i).Set(itemVal.Elem().Addr())
			} else {
				itemVal = newValue.Index(i)
			}

			expectedPos := decoder.GetPosition() + itemSize

			err := unmarshalType(d, fieldType, itemVal, decoder, idt+2)
			if err != nil {
				return err
			}

			if decoder.GetPosition() != expectedPos {
				return fmt.Errorf("unmarshalling vector item did not consume expected ssz range (pos: %v, expected: %v)", decoder.GetPosition(), expectedPos)
			}
		}
	}

	if targetType.Kind != reflect.Array {
		targetValue.Set(newValue)
	}

	return nil
}

// unmarshalDynamicVector decodes vectors with variable-size elements from SSZ format.
//
// For vectors with variable-size elements, SSZ uses an offset-based encoding:
//   - The given number of offsets are decoded first, 4 bytes each
//   - Element data appears after all offsets, in order
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor with vector metadata
//   - targetValue: The reflect.Value where the vector will be stored
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if offsets are invalid or decoding fails
//
// The function validates that:
//   - Offsets are monotonically increasing
//   - No offset points outside the data bounds
//   - Each element consumes exactly the expected bytes
func unmarshalDynamicVector[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	vectorLen := int(targetType.Len)
	requiredOffsetBytes := vectorLen * 4
	canSeek := decoder.CanSeek()

	// check if there's enough data for all offsets
	sszLen := decoder.GetLength()
	if sszLen < requiredOffsetBytes {
		return fmt.Errorf("unexpected end of SSZ. dynamic vector expects at least %v bytes for offsets, got %v", requiredOffsetBytes, sszLen)
	}

	var sliceOffsets []int
	var startPos int

	if canSeek {
		// skip offsets, read later
		startPos = decoder.GetPosition()
		decoder.SkipBytes(requiredOffsetBytes)
	} else {
		// read all item offsets
		sliceOffsets = defaultOffsetSlicePool.Get()
		defer defaultOffsetSlicePool.Put(sliceOffsets)

		if cap(sliceOffsets) < vectorLen {
			sliceOffsets = make([]int, vectorLen)
		} else {
			sliceOffsets = sliceOffsets[:vectorLen]
		}

		for i := 0; i < vectorLen; i++ {
			offset, err := decoder.DecodeOffset()
			if err != nil {
				return err
			}

			sliceOffsets[i] = int(offset)
		}
	}

	fieldType := targetType.ElemDesc

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	var offset uint32

	if canSeek {
		offset = decoder.DecodeOffsetAt(startPos)
	} else {
		offset = uint32(sliceOffsets[0])
	}

	if offset != uint32(vectorLen*4) {
		return fmt.Errorf("dynamic vector offset of first item does not match expected offset (offset: %v, expected: %v)", offset, vectorLen*4)
	}

	var newValue reflect.Value
	if targetType.Kind == reflect.Array {
		newValue = targetValue
	} else {
		newValue = reflect.MakeSlice(fieldT, vectorLen, vectorLen)
	}

	// decode slice items
	for i := 0; i < vectorLen; i++ {
		var itemVal reflect.Value
		if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
			// fmt.Printf("new slice item %v\n", fieldType.Name())
			itemVal = reflect.New(fieldType.Type.Elem())
			newValue.Index(i).Set(itemVal)
		} else {
			itemVal = newValue.Index(i)
		}

		startOffset := offset

		var endOffset uint32
		if i < vectorLen-1 {
			if canSeek {
				endOffset = decoder.DecodeOffsetAt(startPos + (i+1)*4)
			} else {
				endOffset = uint32(sliceOffsets[i+1])
			}
		} else {
			endOffset = uint32(sszLen)
		}

		offset = endOffset

		if endOffset < startOffset || endOffset > uint32(sszLen) {
			return sszutils.ErrOffset
		}

		itemSize := endOffset - startOffset
		decoder.PushLimit(int(itemSize))
		err := unmarshalType(d, fieldType, itemVal, decoder, idt+2)
		if err != nil {
			return err
		}

		consumedDiff := decoder.PopLimit()
		if consumedDiff != 0 {
			return fmt.Errorf("dynamic vector item did not consume expected ssz range (diff: %v, expected: %v)", consumedDiff, itemSize)
		}
	}

	targetValue.Set(newValue)

	return nil
}

// unmarshalList decodes SSZ-encoded list data.
//
// This function handles lists with fixed-size elements. The list length is determined by
// dividing the SSZ data length by the element size.
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor containing list metadata
//   - targetValue: The reflect.Value where the list will be stored
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails or data length is invalid
//
// The function:
//   - Uses optimized copying for byte lists
//   - Validates that each element consumes exactly the expected bytes
func unmarshalList[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	fieldType := targetType.ElemDesc
	sszLen := decoder.GetLength()

	// Calculate slice length once
	itemSize := int(fieldType.Size)
	sliceLen := sszLen / itemSize
	if sszLen%itemSize != 0 {
		return fmt.Errorf("invalid list length, expected multiple of %v, got %v", itemSize, sszLen)
	}

	// slice with static size items
	// fmt.Printf("new slice %v  %v\n", fieldType.Name(), sliceLen)

	fieldT := targetType.Type
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	var newValue reflect.Value
	if targetType.Kind == reflect.Slice {
		// Optimization: avoid reflect.MakeSlice for common byte slice types
		if targetType.GoTypeFlags&GoTypeFlagIsByteArray != 0 && fieldType.Type.Kind() == reflect.Uint8 {
			byteSlice := make([]byte, sliceLen)
			newValue = reflect.ValueOf(byteSlice)
		} else {
			newValue = reflect.MakeSlice(fieldT, sliceLen, sliceLen)
		}
	} else {
		newValue = reflect.New(fieldT).Elem()
	}

	if sliceLen == 0 {
		// do nothing
	} else if targetType.GoTypeFlags&GoTypeFlagIsString != 0 {
		buf, err := decoder.DecodeBytesBuf(sliceLen)
		if err != nil {
			return err
		}
		newValue.SetString(string(buf))
	} else if targetType.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		// shortcut for performance: use copy on []byte arrays
		_, err := decoder.DecodeBytes(newValue.Bytes())
		if err != nil {
			return err
		}
	} else {
		// decode list items

		for i := 0; i < sliceLen; i++ {
			var itemVal reflect.Value
			if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				// fmt.Printf("new list item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType.Type.Elem())
				newValue.Index(i).Set(itemVal.Elem().Addr())
			} else {
				itemVal = newValue.Index(i)
			}

			expectedPos := decoder.GetPosition() + itemSize

			err := unmarshalType(d, fieldType, itemVal, decoder, idt+2)
			if err != nil {
				return err
			}

			if decoder.GetPosition() != expectedPos {
				return fmt.Errorf("list item did not consume expected ssz range (pos: %v, expected: %v)", decoder.GetPosition(), expectedPos)
			}
		}
	}

	targetValue.Set(newValue)

	return nil
}

// unmarshalDynamicList decodes lists with variable-size elements from SSZ format.
//
// For lists with variable-size elements, SSZ uses an offset-based encoding:
//   - The first 4 bytes contain the offset to the first element's data
//   - The number of elements is derived by dividing this offset by 4
//   - Each subsequent 4-byte value is an offset to the next element
//   - Element data appears after all offsets, in order
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor with list metadata
//   - targetValue: The reflect.Value where the list will be stored
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if offsets are invalid or decoding fails
//
// The function validates that:
//   - Offsets are monotonically increasing
//   - No offset points outside the data bounds
//   - Each element consumes exactly the expected bytes
func unmarshalDynamicList[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	sszLen := decoder.GetLength()
	if sszLen == 0 {
		return nil
	}

	// need at least 4 bytes to read the first offset
	if sszLen < 4 {
		return fmt.Errorf("unexpected end of SSZ. dynamic list expects at least 4 bytes for first offset, got %v", sszLen)
	}

	// derive number of items from first item offset
	canSeek := decoder.CanSeek()

	firstOffset, err := decoder.DecodeOffset()
	if err != nil {
		return err
	}
	sliceLen := int(firstOffset / 4)

	// check if there's enough data for all offsets
	requiredOffsetBytes := sliceLen * 4
	if sszLen < requiredOffsetBytes {
		return fmt.Errorf("unexpected end of SSZ. dynamic list expects at least %v bytes for offsets, got %v", requiredOffsetBytes, sszLen)
	}

	// read all item offsets
	var sliceOffsets []int
	var startPos int

	if canSeek {
		startPos = decoder.GetPosition() - 4
		decoder.SkipBytes(requiredOffsetBytes - 4)
	} else {
		sliceOffsets = defaultOffsetSlicePool.Get()
		defer defaultOffsetSlicePool.Put(sliceOffsets)
		if cap(sliceOffsets) < sliceLen {
			sliceOffsets = make([]int, sliceLen)
		} else {
			sliceOffsets = sliceOffsets[:sliceLen]
		}
		sliceOffsets[0] = int(firstOffset)
		for i := 1; i < sliceLen; i++ {
			offset, err := decoder.DecodeOffset()
			if err != nil {
				return err
			}
			sliceOffsets[i] = int(offset)
		}
	}

	fieldType := targetType.ElemDesc

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	newValue := reflect.MakeSlice(fieldT, sliceLen, sliceLen)

	if sliceLen > 0 {
		offset := int(firstOffset)

		// decode slice items
		for i := 0; i < sliceLen; i++ {
			var itemVal reflect.Value
			if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				// fmt.Printf("new slice item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType.Type.Elem())
				newValue.Index(i).Set(itemVal)
			} else {
				itemVal = newValue.Index(i)
			}

			startOffset := offset
			var endOffset int

			if i == sliceLen-1 {
				endOffset = sszLen
			} else {
				if canSeek {
					endOffset = int(decoder.DecodeOffsetAt(startPos + (i+1)*4))
				} else {
					endOffset = sliceOffsets[i+1]
				}
			}

			if endOffset < startOffset || endOffset > sszLen {
				return sszutils.ErrOffset
			}

			itemSize := endOffset - startOffset

			decoder.PushLimit(itemSize)
			err := unmarshalType(d, fieldType, itemVal, decoder, idt+2)
			if err != nil {
				return err
			}

			consumedDiff := decoder.PopLimit()
			if consumedDiff != 0 {
				return fmt.Errorf("dynamic list item did not consume expected ssz range (diff: %v, expected: %v)", consumedDiff, itemSize)
			}

			offset += itemSize
		}
	}

	targetValue.Set(newValue)

	return nil
}

// unmarshalBitlist decodes bitlist values from SSZ-encoded data.
//
// This function handles bitlist decoding. The decoding follows SSZ specifications
// where bitlists are encoded as their bits in sequence without a length prefix, but with a termination bit.
// The termination bit is a single `1` bit appended immediately after the final data bit, then padded to a full byte.
// The position of this termination bit defines the logical length of the bitlist. Bitlists without a termination bit are not allowed.
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor containing bitlist metadata
//   - targetValue: The reflect.Value of the bitlist to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//
// Returns:
//   - error: An error if decoding fails or bitlist is invalid
func unmarshalBitlist[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D) error {
	sszLen := decoder.GetLength()

	if sszLen == 0 {
		return sszutils.ErrBitlistNotTerminated
	}

	// Bitlists can only be []byte (validated by typecache)
	byteSlice := make([]byte, sszLen)
	_, err := decoder.DecodeBytes(byteSlice)
	if err != nil {
		return err
	}

	if byteSlice[sszLen-1] == 0x00 {
		return sszutils.ErrBitlistNotTerminated
	}

	targetValue.Set(reflect.ValueOf(byteSlice))

	return nil
}

// unmarshalCompatibleUnion decodes SSZ-encoded data into a CompatibleUnion.
//
// According to the spec:
//   - The encoding is: selector.to_bytes(1, "little") + serialize(value.data)
//   - The selector index is based at 0 if a ProgressiveContainer type option is present
//   - Otherwise, it is based at 1
//
// Type Parameters:
//   - D: A decoder type implementing sszutils.Decoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - targetType: The TypeDescriptor containing union metadata and variant descriptors
//   - targetValue: The reflect.Value of the CompatibleUnion to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails
func unmarshalCompatibleUnion[D sszutils.Decoder](d *DynSsz, targetType *TypeDescriptor, targetValue reflect.Value, decoder D, idt int) error {
	if decoder.GetLength() < 1 {
		return fmt.Errorf("CompatibleUnion requires at least 1 byte for selector")
	}

	// Read the variant byte
	variant, err := decoder.DecodeUint8()
	if err != nil {
		return err
	}

	// Get the variant descriptor
	variantDesc, ok := targetType.UnionVariants[variant]
	if !ok {
		return sszutils.ErrInvalidUnionVariant
	}

	// Create a new value of the variant type
	variantValue := reflect.New(variantDesc.Type).Elem()

	// Unmarshal the data
	err = unmarshalType(d, variantDesc, variantValue, decoder, idt+2)
	if err != nil {
		return fmt.Errorf("failed to unmarshal union variant %d: %w", variant, err)
	}

	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	targetValue.Field(0).SetUint(uint64(variant))
	targetValue.Field(1).Set(variantValue)

	return nil
}
