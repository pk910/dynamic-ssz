// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"math"
	"math/big"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"github.com/pk910/dynamic-ssz/ssztypes"
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
// Parameters:
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
func (ctx *ReflectionCtx) unmarshalType(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error { //nolint:gocyclo // SSZ unmarshaling handles many type cases
	if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && targetType.SszType != ssztypes.SszOptionalType {
		// target is a pointer type, resolve type & value to actual value type
		if targetValue.IsNil() {
			// create new instance of target type for null pointers
			newValue := reflect.New(targetType.Type.Elem())
			targetValue.Set(newValue)
		}
		targetValue = targetValue.Elem()
	}

	if ctx.verbose {
		isFastsszUnmarshaler := targetType.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
		hasDynamicSize := targetType.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicSize != 0
		useFastSsz := !ctx.noFastSsz && isFastsszUnmarshaler && !hasDynamicSize
		ctx.logCb("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), targetType.Type.Name(), targetType.Kind, useFastSsz, isFastsszUnmarshaler, hasDynamicSize)
	}

	// Fast path: skip compat interface checks for types that don't implement any
	if targetType.SszCompatFlags != 0 || targetType.SszType == ssztypes.SszCustomType {
		hasDynamicSize := targetType.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicSize != 0
		isFastsszUnmarshaler := targetType.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
		useDynamicUnmarshal := targetType.SszCompatFlags&ssztypes.SszCompatFlagDynamicUnmarshaler != 0
		useDynamicDecoder := targetType.SszCompatFlags&ssztypes.SszCompatFlagDynamicDecoder != 0
		useFastSsz := !ctx.noFastSsz && isFastsszUnmarshaler && !hasDynamicSize
		if !useFastSsz && targetType.SszType == ssztypes.SszCustomType {
			useFastSsz = true
		}

		if useFastSsz {
			if unmarshaller, ok := getPtr(targetValue).Interface().(sszutils.FastsszUnmarshaler); ok {
				sszLen := decoder.GetLength()
				if targetType.Size > 0 {
					typeSize := int64(targetType.Size)
					if typeSize > math.MaxInt {
						return sszutils.NewSszErrorf(sszutils.ErrPlatformOverflow, "type size %d exceeds platform int max", targetType.Size)
					}
					sszLen = int(typeSize)
				}
				sszBuf, err := decoder.DecodeBytesBuf(sszLen)
				if err != nil {
					return err
				}
				return unmarshaller.UnmarshalSSZ(sszBuf)
			}
		}

		if useDynamicDecoder {
			if !decoder.Seekable() || !useDynamicUnmarshal {
				// prefer dynamic unmarshaller for seekable decoders (buffer based)
				if sszDecoder, ok := getPtr(targetValue).Interface().(sszutils.DynamicDecoder); ok {
					return sszDecoder.UnmarshalSSZDecoder(ctx.ds, decoder)
				}
			}
		}

		if useDynamicUnmarshal {
			if unmarshaller, ok := getPtr(targetValue).Interface().(sszutils.DynamicUnmarshaler); ok {
				sszLen := decoder.GetLength()
				if targetType.Size > 0 {
					typeSize := int64(targetType.Size)
					if typeSize > math.MaxInt {
						return sszutils.NewSszErrorf(sszutils.ErrPlatformOverflow, "type size %d exceeds platform int max", targetType.Size)
					}
					sszLen = int(typeSize)
				}
				sszBuf, err := decoder.DecodeBytesBuf(sszLen)
				if err != nil {
					return err
				}
				return unmarshaller.UnmarshalSSZDyn(ctx.ds, sszBuf)
			}
		}
	}

	var err error
	switch targetType.SszType {
	// complex types
	case ssztypes.SszTypeWrapperType:
		err = ctx.unmarshalTypeWrapper(targetType, targetValue, decoder, idt)
		if err != nil {
			return err
		}
	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		err = ctx.unmarshalContainer(targetType, targetValue, decoder, idt)
		if err != nil {
			return err
		}
	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		if targetType.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			err = ctx.unmarshalDynamicVector(targetType, targetValue, decoder, idt)
		} else {
			err = ctx.unmarshalVector(targetType, targetValue, decoder, idt)
		}
		if err != nil {
			return err
		}
	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		if targetType.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			err = ctx.unmarshalDynamicList(targetType, targetValue, decoder, idt)
		} else {
			err = ctx.unmarshalList(targetType, targetValue, decoder, idt)
		}
		if err != nil {
			return err
		}
	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		err = ctx.unmarshalBitlist(targetType, targetValue, decoder)
		if err != nil {
			return err
		}
	case ssztypes.SszCompatibleUnionType:
		err = ctx.unmarshalCompatibleUnion(targetType, targetValue, decoder, idt)
		if err != nil {
			return err
		}

	// primitive types
	case ssztypes.SszBoolType:
		var boolVal bool
		boolVal, err = decoder.DecodeBool()
		if err != nil {
			return err
		}
		targetValue.SetBool(boolVal)
	case ssztypes.SszUint8Type:
		var u8Val uint8
		u8Val, err = decoder.DecodeUint8()
		if err != nil {
			return err
		}
		targetValue.SetUint(uint64(u8Val))
	case ssztypes.SszUint16Type:
		var u16Val uint16
		u16Val, err = decoder.DecodeUint16()
		if err != nil {
			return err
		}
		targetValue.SetUint(uint64(u16Val))
	case ssztypes.SszUint32Type:
		var u32Val uint32
		u32Val, err = decoder.DecodeUint32()
		if err != nil {
			return err
		}
		targetValue.SetUint(uint64(u32Val))
	case ssztypes.SszUint64Type:
		var u64Val uint64
		u64Val, err = decoder.DecodeUint64()
		if err != nil {
			return err
		}

		if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			timeVal := time.Unix(int64(u64Val), 0)
			targetValue.Set(reflect.ValueOf(timeVal))
		} else {
			targetValue.SetUint(u64Val)
		}

	// extended types
	case ssztypes.SszInt8Type:
		var i8Val uint8
		i8Val, err = decoder.DecodeUint8()
		if err != nil {
			return err
		}
		targetValue.SetInt(int64(i8Val))
	case ssztypes.SszInt16Type:
		var i16Val uint16
		i16Val, err = decoder.DecodeUint16()
		if err != nil {
			return err
		}
		targetValue.SetInt(int64(i16Val))
	case ssztypes.SszInt32Type:
		var i32Val uint32
		i32Val, err = decoder.DecodeUint32()
		if err != nil {
			return err
		}
		targetValue.SetInt(int64(i32Val))
	case ssztypes.SszInt64Type:
		var i64Val uint64
		i64Val, err = decoder.DecodeUint64()
		if err != nil {
			return err
		}
		targetValue.SetInt(int64(i64Val))
	case ssztypes.SszFloat32Type:
		var f32Val uint32
		f32Val, err = decoder.DecodeUint32()
		if err != nil {
			return err
		}
		targetValue.SetFloat(float64(math.Float32frombits(f32Val)))
	case ssztypes.SszFloat64Type:
		var f64Val uint64
		f64Val, err = decoder.DecodeUint64()
		if err != nil {
			return err
		}
		targetValue.SetFloat(math.Float64frombits(f64Val))
	case ssztypes.SszOptionalType:
		err = ctx.unmarshalOptional(targetType, targetValue, decoder, idt)
		if err != nil {
			return err
		}
	case ssztypes.SszBigIntType:
		err = ctx.unmarshalBigInt(targetType, targetValue, decoder, idt)
		if err != nil {
			return err
		}
	default:
		return sszutils.NewSszErrorf(sszutils.ErrNotImplemented, "unknown type: %v", targetType)
	}

	return nil
}

// unmarshalTypeWrapper unmarshals a TypeWrapper by extracting the wrapped data and unmarshaling it as the wrapped type.
//
// Parameters:
//   - targetType: The TypeDescriptor containing wrapper field metadata
//   - targetValue: The reflect.Value of the wrapper to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails or data is malformed
//
// The function validates that the Data field is present and unmarshals the wrapped value using its type descriptor.
func (ctx *ReflectionCtx) unmarshalTypeWrapper(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	if ctx.verbose {
		ctx.logCb("%sunmarshalTypeWrapper: %s\n", strings.Repeat(" ", idt), targetType.Type.Name())
	}

	// Get the Data field from the TypeWrapper
	dataField := targetValue.Field(0)

	// Unmarshal the wrapped value using its type descriptor
	err := ctx.unmarshalType(targetType.ElemDesc, dataField, decoder, idt+2)
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
// Parameters:
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
func (ctx *ReflectionCtx) unmarshalContainer(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	// Fast path: containers with no dynamic fields (e.g. Validator)
	if len(targetType.ContainerDesc.DynFields) == 0 {
		sszSize := uint32(decoder.GetLength())
		if sszSize < targetType.Len {
			return sszutils.ErrUnexpectedEOF
		}

		fields := targetType.ContainerDesc.Fields
		for i := 0; i < len(fields); i++ {
			field := &fields[i]
			fieldSize := int(field.Type.Size)
			expectedPos := decoder.GetPosition() + fieldSize

			fieldValue := targetValue.Field(i)
			if err := ctx.unmarshalType(field.Type, fieldValue, decoder, idt+2); err != nil {
				return sszutils.ErrorWithPath(err, field.Name)
			}

			if decoder.GetPosition() != expectedPos {
				return sszutils.NewSszErrorf(sszutils.ErrOffset, "container field did not consume expected ssz range (pos: %v, expected: %v)", decoder.GetPosition(), expectedPos)
			}
		}
		return nil
	}

	canSeek := decoder.Seekable()

	var dynamicOffsets []uint32
	var startPos int

	if canSeek {
		startPos = decoder.GetPosition()
	} else {
		dynamicOffsets = sszutils.GetOffsetSlice(len(targetType.ContainerDesc.DynFields))
		defer sszutils.PutOffsetSlice(dynamicOffsets)
	}
	sszSize := uint32(decoder.GetLength())
	if sszSize < targetType.Len {
		return sszutils.ErrUnexpectedEOF
	}

	dynIdx := 0
	fields := targetType.ContainerDesc.Fields
	for i := 0; i < len(fields); i++ {
		field := &fields[i]

		fieldSize := int(field.Type.Size)
		if fieldSize > 0 {
			// static size field
			// fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)
			expectedPos := decoder.GetPosition() + fieldSize

			fieldValue := targetValue.Field(i)
			err := ctx.unmarshalType(field.Type, fieldValue, decoder, idt+2)
			if err != nil {
				return sszutils.ErrorWithPath(err, field.Name)
			}

			if decoder.GetPosition() != expectedPos {
				return sszutils.NewSszErrorf(sszutils.ErrOffset, "container field did not consume expected ssz range (pos: %v, expected: %v)", decoder.GetPosition(), expectedPos)
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
					return sszutils.ErrorWithPathf(err, "%s:o", field.Name)
				}

				// store dynamic field offset for later
				dynamicOffsets[dynIdx] = fieldOffset
				dynIdx++
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
			dynOffset = dynamicOffsets[0]
		}

		if dynOffset != targetType.Len { // check first dynamic field offset
			return sszutils.ErrorWithPathf(
				sszutils.NewSszErrorf(sszutils.ErrOffset, "first dynamic field offset does not match expected offset (offset: %v, expected: %v)", dynOffset, targetType.Len),
				"%s:o", targetType.ContainerDesc.DynFields[0].Field.Name,
			)
		}

		for i, field := range targetType.ContainerDesc.DynFields {
			startOffset := dynOffset

			var endOffset uint32
			if i < dynamicFieldCount-1 {
				if canSeek {
					dynOffset = decoder.DecodeOffsetAt(startPos + int(targetType.ContainerDesc.DynFields[i+1].HeaderOffset))
				} else {
					dynOffset = dynamicOffsets[i+1]
				}

				endOffset = dynOffset
			} else {
				endOffset = sszSize
			}

			// check offset integrity (not before previous field offset & not after range end)
			if endOffset > sszSize || endOffset < startOffset {
				return sszutils.ErrorWithPathf(
					sszutils.NewSszErrorf(sszutils.ErrOffset, "dynamic field offset out of range (end: %v, start: %v, ssz size: %v)", endOffset, startOffset, sszSize),
					"%s:o", field.Field.Name,
				)
			}

			// fmt.Printf("%sfield %d:\t dynamic [%v:%v]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], startOffset, endOffset, field.Name)

			sszSize := endOffset - startOffset
			decoder.PushLimit(int(sszSize))

			fieldDescriptor := field.Field
			fieldValue := targetValue.Field(int(field.Index))
			err := ctx.unmarshalType(fieldDescriptor.Type, fieldValue, decoder, idt+2)
			if err != nil {
				return sszutils.ErrorWithPath(err, fieldDescriptor.Name)
			}

			consumedDiff := decoder.PopLimit()
			if consumedDiff != 0 {
				return sszutils.NewSszErrorf(sszutils.ErrOffset, "struct field did not consume expected ssz range (diff: %v, expected: %v)", consumedDiff, sszSize)
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
// Parameters:
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
func (ctx *ReflectionCtx) unmarshalVector(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	vecLen := int64(targetType.Len)
	if vecLen > math.MaxInt {
		return sszutils.NewSszErrorf(sszutils.ErrPlatformOverflow, "vector length %d exceeds platform int max", targetType.Len)
	}

	fieldType := targetType.ElemDesc
	arrLen := int(vecLen)

	var newValue reflect.Value
	switch targetType.Kind {
	case reflect.Slice:
		// Optimization: avoid reflect.MakeSlice for common byte slice types
		if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 && targetType.ElemDesc.Type.Kind() == reflect.Uint8 {
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

	if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
		// shortcut for performance: use copy on []byte arrays

		if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
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
					return sszutils.NewSszError(sszutils.ErrInvalidValueRange, "bitvector padding bits are not zero")
				}
			}
		}
	} else if arrLen > 0 && targetType.Kind == reflect.Slice && fieldType.SszType == ssztypes.SszUint64Type && fieldType.GoTypeFlags == 0 && fieldType.Kind == reflect.Uint64 {
		// Fast path: bulk decode uint64 vectors without per-element reflection dispatch
		u64s := make([]uint64, arrLen)
		if err := sszutils.DecodeUint64Slice(decoder, u64s); err != nil {
			return err
		}
		targetValue.Set(reflect.ValueOf(u64s))
		return nil
	} else {
		if err := ctx.unmarshalFixedElements(fieldType, newValue, arrLen, decoder, idt, "vector"); err != nil {
			return err
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
// Parameters:
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
func (ctx *ReflectionCtx) unmarshalDynamicVector(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	dynVecLen := int64(targetType.Len)
	if dynVecLen > math.MaxInt {
		return sszutils.NewSszErrorf(sszutils.ErrPlatformOverflow, "dynamic vector length %d exceeds platform int max", targetType.Len)
	}

	vectorLen := int(dynVecLen)
	requiredOffsetBytes := vectorLen * 4
	canSeek := decoder.Seekable()

	// check if there's enough data for all offsets
	sszLen := decoder.GetLength()
	if sszLen < requiredOffsetBytes {
		return sszutils.NewSszErrorf(sszutils.ErrUnexpectedEOF, "dynamic vector expects at least %v bytes for offsets, got %v", requiredOffsetBytes, sszLen)
	}

	var sliceOffsets []uint32
	var startPos int

	if canSeek {
		// skip offsets, read later
		startPos = decoder.GetPosition()
		decoder.SkipBytes(requiredOffsetBytes)
	} else {
		// read all item offsets
		sliceOffsets = sszutils.GetOffsetSlice(vectorLen)
		defer sszutils.PutOffsetSlice(sliceOffsets)

		for i := 0; i < vectorLen; i++ {
			offset, err := decoder.DecodeOffset()
			if err != nil {
				return sszutils.ErrorWithPathf(err, "[%d:o]", i)
			}

			sliceOffsets[i] = offset
		}
	}

	fieldType := targetType.ElemDesc

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	var offset uint32

	if canSeek {
		offset = decoder.DecodeOffsetAt(startPos)
	} else {
		offset = sliceOffsets[0]
	}

	if offset != uint32(vectorLen*4) {
		return sszutils.NewSszErrorf(sszutils.ErrOffset, "dynamic vector offset of first item does not match expected offset (offset: %v, expected: %v)", offset, vectorLen*4)
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
		if fieldType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
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
				endOffset = sliceOffsets[i+1]
			}
		} else {
			endOffset = uint32(sszLen)
		}

		offset = endOffset

		if endOffset < startOffset || endOffset > uint32(sszLen) {
			return sszutils.ErrorWithPathf(
				sszutils.NewSszErrorf(sszutils.ErrOffset, "dynamic vector item offset out of range (end: %v, start: %v, ssz size: %v)", endOffset, startOffset, sszLen),
				"[%d:o]", i,
			)
		}

		itemSize := endOffset - startOffset
		decoder.PushLimit(int(itemSize))
		err := ctx.unmarshalType(fieldType, itemVal, decoder, idt+2)
		if err != nil {
			return sszutils.ErrorWithPathf(err, "[%d]", i)
		}

		consumedDiff := decoder.PopLimit()
		if consumedDiff != 0 {
			return sszutils.ErrorWithPathf(
				sszutils.NewSszErrorf(sszutils.ErrOffset, "dynamic vector item did not consume expected ssz range (diff: %v, expected: %v)", consumedDiff, itemSize),
				"[%d]", i,
			)
		}
	}

	targetValue.Set(newValue)

	return nil
}

// unmarshalFixedElements decodes a sequence of fixed-size elements into target slice/array positions.
// It handles both pointer and non-pointer element types.
func (ctx *ReflectionCtx) unmarshalFixedElements(fieldType *ssztypes.TypeDescriptor, newValue reflect.Value, count int, decoder sszutils.Decoder, idt int, context string) error {
	fieldSize := int64(fieldType.Size)
	if fieldSize > math.MaxInt {
		return sszutils.NewSszErrorf(sszutils.ErrPlatformOverflow, "field size %d exceeds platform int max", fieldType.Size)
	}

	itemSize := int(fieldSize)
	isPointer := fieldType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0

	for i := 0; i < count; i++ {
		var itemVal reflect.Value
		if isPointer {
			itemVal = reflect.New(fieldType.Type.Elem())
			newValue.Index(i).Set(itemVal.Elem().Addr())
		} else {
			itemVal = newValue.Index(i)
		}

		expectedPos := decoder.GetPosition() + itemSize

		if err := ctx.unmarshalType(fieldType, itemVal, decoder, idt+2); err != nil {
			return sszutils.ErrorWithPathf(err, "[%d]", i)
		}

		if decoder.GetPosition() != expectedPos {
			return sszutils.ErrorWithPathf(
				sszutils.NewSszErrorf(sszutils.ErrOffset, "%s item did not consume expected ssz range (pos: %v, expected: %v)", context, decoder.GetPosition(), expectedPos),
				"[%d]", i,
			)
		}
	}

	return nil
}

// unmarshalList decodes SSZ-encoded list data.
//
// This function handles lists with fixed-size elements. The list length is determined by
// dividing the SSZ data length by the element size.
//
// Parameters:
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
func (ctx *ReflectionCtx) unmarshalList(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	fieldType := targetType.ElemDesc
	sszLen := decoder.GetLength()

	elemSize := int64(fieldType.Size)
	if elemSize > math.MaxInt {
		return sszutils.NewSszErrorf(sszutils.ErrPlatformOverflow, "field size %d exceeds platform int max", fieldType.Size)
	}

	// Calculate slice length once
	itemSize := int(elemSize)
	sliceLen := sszLen / itemSize
	if sszLen%itemSize != 0 {
		return sszutils.NewSszErrorf(sszutils.ErrOffset, "invalid list length, expected multiple of %v, got %v", itemSize, sszLen)
	}

	// slice with static size items
	// fmt.Printf("new slice %v  %v\n", fieldType.Name(), sliceLen)

	fieldT := targetType.Type
	if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	var newValue reflect.Value

	// Fast path: bulk decode uint64 slices without per-element reflection dispatch
	if sliceLen > 0 && targetType.Kind == reflect.Slice && fieldType.SszType == ssztypes.SszUint64Type && fieldType.GoTypeFlags == 0 && fieldType.Kind == reflect.Uint64 {
		u64s := make([]uint64, sliceLen)
		if err := sszutils.DecodeUint64Slice(decoder, u64s); err != nil {
			return err
		}
		targetValue.Set(reflect.ValueOf(u64s))
		return nil
	}

	if targetType.Kind == reflect.Slice {
		// Optimization: avoid reflect.MakeSlice for common byte slice types
		if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 && fieldType.Type.Kind() == reflect.Uint8 {
			byteSlice := make([]byte, sliceLen)
			newValue = reflect.ValueOf(byteSlice)
		} else {
			newValue = reflect.MakeSlice(fieldT, sliceLen, sliceLen)
		}
	} else {
		newValue = reflect.New(fieldT).Elem()
	}

	switch {
	case sliceLen == 0:
		// do nothing
	case targetType.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0:
		buf, err := decoder.DecodeBytesBuf(sliceLen)
		if err != nil {
			return err
		}
		newValue.SetString(string(buf))
	case targetType.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0:
		// shortcut for performance: use copy on []byte arrays
		_, err := decoder.DecodeBytes(newValue.Bytes())
		if err != nil {
			return err
		}
	default:
		// decode list items
		if err := ctx.unmarshalFixedElements(fieldType, newValue, sliceLen, decoder, idt, "list"); err != nil {
			return err
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
// Parameters:
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
func (ctx *ReflectionCtx) unmarshalDynamicList(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	sszLen := decoder.GetLength()
	if sszLen == 0 {
		return nil
	}

	// need at least 4 bytes to read the first offset
	if sszLen < 4 {
		return sszutils.NewSszErrorf(sszutils.ErrUnexpectedEOF, "dynamic list expects at least 4 bytes for first offset, got %v", sszLen)
	}

	// derive number of items from first item offset
	canSeek := decoder.Seekable()

	firstOffset, err := decoder.DecodeOffset()
	if err != nil {
		return err
	}
	sliceLen := int(firstOffset / 4)

	// check if there's enough data for all offsets
	requiredOffsetBytes := sliceLen * 4
	if sszLen < requiredOffsetBytes {
		return sszutils.NewSszErrorf(sszutils.ErrUnexpectedEOF, "dynamic list expects at least %v bytes for offsets, got %v", requiredOffsetBytes, sszLen)
	}

	// read all item offsets
	var sliceOffsets []uint32
	var startPos int

	if canSeek {
		startPos = decoder.GetPosition() - 4
		decoder.SkipBytes(requiredOffsetBytes - 4)
	} else {
		sliceOffsets = sszutils.GetOffsetSlice(sliceLen)
		defer sszutils.PutOffsetSlice(sliceOffsets)

		sliceOffsets[0] = firstOffset
		for i := 1; i < sliceLen; i++ {
			offset, err := decoder.DecodeOffset()
			if err != nil {
				return sszutils.ErrorWithPathf(err, "[%d:o]", i)
			}
			sliceOffsets[i] = offset
		}
	}

	fieldType := targetType.ElemDesc

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	newValue := reflect.MakeSlice(fieldT, sliceLen, sliceLen)

	if sliceLen > 0 {
		offset := firstOffset

		// decode slice items
		for i := 0; i < sliceLen; i++ {
			var itemVal reflect.Value
			if fieldType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
				// fmt.Printf("new slice item %v\n", fieldType.Name())
				itemVal = reflect.New(fieldType.Type.Elem())
				newValue.Index(i).Set(itemVal)
			} else {
				itemVal = newValue.Index(i)
			}

			startOffset := offset
			var endOffset uint32

			if i == sliceLen-1 {
				endOffset = uint32(sszLen)
			} else {
				if canSeek {
					endOffset = decoder.DecodeOffsetAt(startPos + (i+1)*4)
				} else {
					endOffset = sliceOffsets[i+1]
				}
			}

			if endOffset < startOffset || endOffset > uint32(sszLen) {
				return sszutils.ErrorWithPathf(
					sszutils.NewSszErrorf(sszutils.ErrOffset, "dynamic list item offset out of range (end: %v, start: %v, ssz size: %v)", endOffset, startOffset, sszLen),
					"[%d:o]", i,
				)
			}

			itemSize := endOffset - startOffset

			decoder.PushLimit(int(itemSize))
			err := ctx.unmarshalType(fieldType, itemVal, decoder, idt+2)
			if err != nil {
				return sszutils.ErrorWithPathf(err, "[%d]", i)
			}

			consumedDiff := decoder.PopLimit()
			if consumedDiff != 0 {
				return sszutils.ErrorWithPathf(
					sszutils.NewSszErrorf(sszutils.ErrOffset, "dynamic list item did not consume expected ssz range (diff: %v, expected: %v)", consumedDiff, itemSize),
					"[%d]", i,
				)
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
// Parameters:
//   - targetType: The TypeDescriptor containing bitlist metadata
//   - targetValue: The reflect.Value of the bitlist to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//
// Returns:
//   - error: An error if decoding fails or bitlist is invalid
func (ctx *ReflectionCtx) unmarshalBitlist(_ *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder) error {
	sszLen := decoder.GetLength()

	if sszLen == 0 {
		return sszutils.NewSszError(sszutils.ErrBitlistNotTerminated, "bitlist misses mandatory termination bit")
	}

	// Bitlists can only be []byte (validated by typecache)
	byteSlice := make([]byte, sszLen)
	_, err := decoder.DecodeBytes(byteSlice)
	if err != nil {
		return err
	}

	if byteSlice[sszLen-1] == 0x00 {
		return sszutils.NewSszError(sszutils.ErrBitlistNotTerminated, "bitlist misses mandatory termination bit")
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
// Parameters:
//   - targetType: The TypeDescriptor containing union metadata and variant descriptors
//   - targetValue: The reflect.Value of the CompatibleUnion to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails
func (ctx *ReflectionCtx) unmarshalCompatibleUnion(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	if decoder.GetLength() < 1 {
		return sszutils.NewSszError(sszutils.ErrUnexpectedEOF, "CompatibleUnion requires at least 1 byte for selector")
	}

	// Read the variant byte
	variant, err := decoder.DecodeUint8()
	if err != nil {
		return err
	}

	// Get the variant descriptor
	variantDesc, ok := targetType.UnionVariants[variant]
	if !ok {
		return sszutils.NewSszError(sszutils.ErrInvalidUnionVariant, "invalid union variant")
	}

	// Create a new value of the variant type
	variantValue := reflect.New(variantDesc.Type).Elem()

	// Unmarshal the data
	err = ctx.unmarshalType(variantDesc, variantValue, decoder, idt+2)
	if err != nil {
		return sszutils.ErrorWithPathf(err, "[v:%d]", variant)
	}

	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	targetValue.Field(0).SetUint(uint64(variant))
	targetValue.Field(1).Set(variantValue)

	return nil
}

// unmarshalOptional decodes an Optional by unmarshalling its data field.
//
// Parameters:
//   - targetType: The TypeDescriptor containing optional metadata
//   - targetValue: The reflect.Value of the optional to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails
func (ctx *ReflectionCtx) unmarshalOptional(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, idt int) error {
	if decoder.GetLength() < 1 {
		return sszutils.NewSszError(sszutils.ErrUnexpectedEOF, "optional requires at least 1 byte for availability")
	}

	// Read the availability byte
	availability, err := decoder.DecodeUint8()
	if err != nil {
		return err
	}

	if availability == 0 {
		targetValue.Set(reflect.Zero(targetType.Type))
		return nil
	}

	if targetValue.IsNil() {
		// create new instance of target type for null pointers
		newValue := reflect.New(targetType.Type.Elem())
		targetValue.Set(newValue)
	}

	err = ctx.unmarshalType(targetType.ElemDesc, targetValue.Elem(), decoder, idt+2)
	if err != nil {
		return err
	}

	return nil
}

// unmarshalBigInt decodes a BigInt by unmarshalling its data field.
//
// Parameters:
//   - targetType: The TypeDescriptor containing big int metadata
//   - targetValue: The reflect.Value of the big int to populate
//   - decoder: The decoder instance used to read SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails
func (ctx *ReflectionCtx) unmarshalBigInt(_ *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder, _ int) error {
	dataLen := decoder.GetLength()
	bigInt := new(big.Int)

	if dataLen > 0 {
		bigIntBytes, err := decoder.DecodeBytesBuf(dataLen)
		if err != nil {
			return err
		}

		bigInt.SetBytes(bigIntBytes)
	}

	targetValue.Set(reflect.ValueOf(*bigInt))

	return nil
}
