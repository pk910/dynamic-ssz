// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// marshalType is the core recursive generic function for marshalling Go values into SSZ-encoded data.
//
// This function serves as the primary dispatcher within the marshalling process, handling both primitive
// and composite types. It uses the TypeDescriptor's metadata to determine the most efficient encoding
// path, automatically leveraging fastssz when possible for optimal performance.
//
// The generic type parameter E allows the compiler to generate specialized code for each encoder
// implementation, eliminating interface dispatch overhead.
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor containing optimized metadata about the type to be encoded
//   - sourceValue: The reflect.Value holding the data to be encoded
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging (when enabled)
//
// Returns:
//   - error: An error if encoding fails
//
// The function handles:
//   - Automatic nil pointer dereferencing
//   - FastSSZ delegation for compatible types without dynamic sizing
//   - Primitive type encoding (bool, uint8, uint16, uint32, uint64)
//   - Delegation to specialized functions for composite types (structs, arrays, slices)
func marshalType[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	if sourceType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		if sourceValue.IsNil() {
			sourceValue = reflect.New(sourceType.Type.Elem()).Elem()
		} else {
			sourceValue = sourceValue.Elem()
		}
	}

	hasDynamicSize := sourceType.SszTypeFlags&SszTypeFlagHasDynamicSize != 0
	isFastsszMarshaler := sourceType.SszCompatFlags&SszCompatFlagFastSSZMarshaler != 0
	useDynamicMarshal := sourceType.SszCompatFlags&SszCompatFlagDynamicMarshaler != 0
	useDynamicEncoder := sourceType.SszCompatFlags&SszCompatFlagDynamicEncoder != 0
	useFastSsz := !d.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && sourceType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		d.LogCb("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), sourceType.Type.Name(), sourceType.Kind, useFastSsz, isFastsszMarshaler, hasDynamicSize)
	}

	if useFastSsz {
		marshaller, ok := getPtr(sourceValue).Interface().(sszutils.FastsszMarshaler)
		if ok {
			newBuf, err := marshaller.MarshalSSZTo(encoder.GetBuffer())
			if err != nil {
				return err
			}
			encoder.SetBuffer(newBuf)
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz && useDynamicEncoder {
		if encoder.Seekable() && useDynamicMarshal {
			// prefer static marshaller for non-seekable encoders (buffer based)
			useDynamicEncoder = false
		} else if sszEncoder, ok := getPtr(sourceValue).Interface().(sszutils.DynamicEncoder); ok {
			err := sszEncoder.MarshalSSZEncoder(d, encoder)
			if err != nil {
				return err
			}
		} else {
			useDynamicEncoder = false
		}
	}

	if !useFastSsz && !useDynamicEncoder && useDynamicMarshal {
		// Use dynamic marshaler - can always be used even with dynamic specs
		marshaller, ok := getPtr(sourceValue).Interface().(sszutils.DynamicMarshaler)
		if ok {
			newBuf, err := marshaller.MarshalSSZDyn(d, encoder.GetBuffer())
			if err != nil {
				return err
			}
			encoder.SetBuffer(newBuf)
		} else {
			useDynamicMarshal = false
		}
	}

	if !useFastSsz && !useDynamicEncoder && !useDynamicMarshal {
		// can't use fastssz, use dynamic marshaling
		var err error
		switch sourceType.SszType {
		// complex types
		case SszTypeWrapperType:
			err = marshalTypeWrapper(d, sourceType, sourceValue, encoder, idt)
			if err != nil {
				return err
			}
		case SszContainerType, SszProgressiveContainerType:
			err = marshalContainer(d, sourceType, sourceValue, encoder, idt)
			if err != nil {
				return err
			}
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			if sourceType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = marshalDynamicVector(d, sourceType, sourceValue, encoder, idt)
			} else {
				err = marshalVector(d, sourceType, sourceValue, encoder, idt)
			}
			if err != nil {
				return err
			}
		case SszListType, SszProgressiveListType:
			if sourceType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = marshalDynamicList(d, sourceType, sourceValue, encoder, idt)
			} else {
				err = marshalList(d, sourceType, sourceValue, encoder, idt)
			}
			if err != nil {
				return err
			}
		case SszBitlistType, SszProgressiveBitlistType:
			err = marshalBitlist(d, sourceType, sourceValue, encoder, idt)
			if err != nil {
				return err
			}
		case SszCompatibleUnionType:
			err = marshalCompatibleUnion(d, sourceType, sourceValue, encoder, idt)
			if err != nil {
				return err
			}

		// primitive types
		case SszBoolType:
			encoder.EncodeBool(sourceValue.Bool())
		case SszUint8Type:
			encoder.EncodeUint8(uint8(sourceValue.Uint()))
		case SszUint16Type:
			encoder.EncodeUint16(uint16(sourceValue.Uint()))
		case SszUint32Type:
			encoder.EncodeUint32(uint32(sourceValue.Uint()))
		case SszUint64Type:
			if sourceType.GoTypeFlags&GoTypeFlagIsTime != 0 {
				timeValue, isTime := sourceValue.Interface().(time.Time)
				if !isTime {
					return fmt.Errorf("time.Time type expected, got %v", sourceType.Type.Name())
				}
				encoder.EncodeUint64(uint64(timeValue.Unix()))
			} else {
				encoder.EncodeUint64(uint64(sourceValue.Uint()))
			}
		default:
			return fmt.Errorf("unknown type: %v", sourceType)
		}
	}

	return nil
}

// marshalTypeWrapper marshals a TypeWrapper by extracting its data field and marshaling it as the wrapped type.
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor containing wrapper field metadata
//   - sourceValue: The reflect.Value of the wrapper to encode
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if any field encoding fails
//
// The function validates that the Data field is present and marshals the wrapped value using its type descriptor.
func marshalTypeWrapper[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	if d.Verbose {
		d.LogCb("%smarshalTypeWrapper: %s\n", strings.Repeat(" ", idt), sourceType.Type.Name())
	}

	// Extract the Data field from the TypeWrapper
	dataField := sourceValue.Field(0)

	// Marshal the wrapped value using its type descriptor
	return marshalType(d, sourceType.ElemDesc, dataField, encoder, idt+2)
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
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor containing container field metadata
//   - sourceValue: The reflect.Value of the container to encode (must be a struct)
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if any field encoding fails
func marshalContainer[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	offset := 0
	dynObjOffset := 0
	canSeek := encoder.Seekable()
	startLen := encoder.GetPosition()
	fieldCount := len(sourceType.ContainerDesc.Fields)

	for i := 0; i < fieldCount; i++ {
		field := sourceType.ContainerDesc.Fields[i]
		fieldSize := field.Type.Size
		if fieldSize > 0 {
			//fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)

			fieldValue := sourceValue.Field(i)
			err := marshalType(d, field.Type, fieldValue, encoder, idt+2)
			if err != nil {
				return fmt.Errorf("failed encoding field %v: %w", field.Name, err)
			}

		} else {
			fieldSize = 4
			if canSeek {
				// we can seek, so we'll update the offset later
				encoder.EncodeOffset(0)
			} else {
				// we can't seek, so we need to calculate the object size now
				size, err := d.getSszValueSize(field.Type, sourceValue.Field(i))
				if err != nil {
					return fmt.Errorf("failed to get size of dynamic field %v: %w", field.Name, err)
				}

				encoder.EncodeOffset(sourceType.Len + uint32(dynObjOffset))
				dynObjOffset += int(size)
			}
			//fmt.Printf("%sfield %d:\t offset [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)
		}
		offset += int(fieldSize)
	}

	curPos := encoder.GetPosition()
	for _, field := range sourceType.ContainerDesc.DynFields {
		// set field offset
		if canSeek {
			fieldOffset := int(field.HeaderOffset)
			encoder.EncodeOffsetAt(fieldOffset+startLen, uint32(offset))
		}

		//fmt.Printf("%sfield %d:\t dynamic [%v:]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], offset, field.Name)

		fieldDescriptor := field.Field
		fieldValue := sourceValue.Field(int(field.Index))
		err := marshalType(d, fieldDescriptor.Type, fieldValue, encoder, idt+2)
		if err != nil {
			return fmt.Errorf("failed encoding field %v: %w", fieldDescriptor.Name, err)
		}

		if canSeek {
			newPos := encoder.GetPosition()
			offset += newPos - curPos
			curPos = newPos
		}
	}

	return nil
}

// marshalVector encodes vector values into SSZ-encoded data.
//
// Vectors in SSZ are encoded as fixed-size sequences where each element is encoded
// sequentially without any length prefix (since the length is known from the type).
// For byte arrays ([N]byte) or slices ([]byte), the function uses an optimized path that directly
// appends the bytes without element-wise iteration.
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor containing vector metadata including element type and length
//   - sourceValue: The reflect.Value of the vector to encode
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if any element encoding fails
//
// Special handling:
//   - Byte arrays use reflect.Value.Bytes() for efficient bulk copying
//   - Non-addressable arrays are made addressable via a temporary pointer
func marshalVector[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	sliceLen := sourceValue.Len()
	if uint32(sliceLen) > sourceType.Len {
		if sourceType.Kind == reflect.Array {
			sliceLen = int(sourceType.Len)
		} else {
			return sszutils.ErrListTooBig
		}
	}

	appendZero := 0
	dataLen := int(sourceType.Len)
	if uint32(sliceLen) < sourceType.Len {
		appendZero = int(sourceType.Len) - sliceLen
		dataLen = sliceLen
	}

	if sourceType.GoTypeFlags&(GoTypeFlagIsByteArray|GoTypeFlagIsString) != 0 {
		// shortcut for performance: use append on []byte arrays
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(sourceType.Type)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}

		var bytes []byte
		if sourceType.GoTypeFlags&GoTypeFlagIsString != 0 {
			bytes = []byte(sourceValue.String())
		} else {
			bytes = sourceValue.Bytes()
		}

		encoder.EncodeBytes(bytes[:dataLen])

		if appendZero > 0 {
			encoder.EncodeZeroPadding(appendZero)
		} else if sourceType.BitSize > 0 && sourceType.BitSize < uint32(len(bytes))*8 {
			// check padding bits
			paddingMask := uint8((uint16(0xff) << (sourceType.BitSize % 8)) & 0xff)
			paddingBits := bytes[dataLen-1] & paddingMask
			if paddingBits != 0 {
				return fmt.Errorf("bitvector padding bits are not zero")
			}
		}
	} else {
		for i := 0; i < dataLen; i++ {
			itemVal := sourceValue.Index(i)
			err := marshalType(d, sourceType.ElemDesc, itemVal, encoder, idt+2)
			if err != nil {
				return err
			}
		}

		if appendZero > 0 {
			totalZeroBytes := int(sourceType.ElemDesc.Size) * appendZero
			encoder.EncodeZeroPadding(totalZeroBytes)
		}
	}

	return nil
}

// marshalDynamicVector encodes vectors with variable-size elements into SSZ format.
//
// For vectors with variable-size elements, SSZ requires a special encoding:
//  1. A series of 4-byte offsets, one per element, indicating where each element's data begins
//  2. The actual encoded data for each element, in order
//
// The offsets are relative to the start of the vector encoding (not the entire message).
// This allows decoders to locate each variable-size element without parsing all preceding elements.
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor with vector metadata
//   - sourceValue: The reflect.Value of the vector to encode
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if encoding fails or size constraints are violated
//
// The function handles size hints for padding with zero values when the list
// length is less than the expected size. Zero values are efficiently batched
// to minimize encoding overhead.
func marshalDynamicVector[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	fieldType := sourceType.ElemDesc
	sliceLen := sourceValue.Len()

	appendZero := 0
	if sourceType.Kind == reflect.Slice || sourceType.Kind == reflect.String {
		sliceLen := sourceValue.Len()
		if uint32(sliceLen) > sourceType.Len {
			return sszutils.ErrListTooBig
		}
		if uint32(sliceLen) < sourceType.Len {
			appendZero = int(sourceType.Len) - sliceLen
		}
	}

	canSeek := encoder.Seekable()
	startOffset := encoder.GetPosition()
	totalOffsets := sliceLen + appendZero
	offset := 4 * totalOffsets

	var zeroVal reflect.Value
	if appendZero > 0 {
		if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
			zeroVal = reflect.New(fieldType.Type.Elem())
		} else {
			zeroVal = reflect.New(fieldType.Type).Elem()
		}
	}

	if canSeek {
		encoder.EncodeZeroPadding(4 * totalOffsets) // Reserve space for offsets
	} else {
		// need to calculate the object sizes now
		for i := 0; i < sliceLen; i++ {
			itemVal := sourceValue.Index(i)
			size, err := d.getSszValueSize(fieldType, itemVal)
			if err != nil {
				return fmt.Errorf("failed to get size of dynamic vector element %v: %w", itemVal.Type().Name(), err)
			}

			encoder.EncodeOffset(uint32(offset))
			offset += int(size)
		}
		if appendZero > 0 {
			size, err := d.getSszValueSize(fieldType, zeroVal)
			if err != nil {
				return fmt.Errorf("failed to get size of zero vector element %v: %w", zeroVal.Type().Name(), err)
			}

			for i := 0; i < appendZero; i++ {
				encoder.EncodeOffset(uint32(offset))
				offset += int(size)
			}

		}
	}

	bufLen := encoder.GetPosition()

	for i := 0; i < sliceLen; i++ {
		itemVal := sourceValue.Index(i)

		err := marshalType(d, fieldType, itemVal, encoder, idt+2)
		if err != nil {
			return err
		}

		if canSeek {
			encoder.EncodeOffsetAt(startOffset+(i*4), uint32(offset))

			newPos := encoder.GetPosition()
			offset += newPos - bufLen
			bufLen = newPos
		}
	}

	for i := 0; i < appendZero; i++ {
		err := marshalType(d, fieldType, zeroVal, encoder, idt+2)
		if err != nil {
			return err
		}

		if canSeek {
			encoder.EncodeOffsetAt(startOffset+((sliceLen+i)*4), uint32(offset))

			newPos := encoder.GetPosition()
			offset += newPos - bufLen
			bufLen = newPos
		}
	}

	return nil
}

// marshalList encodes list values into SSZ-encoded data.
//
// This function handles lists with fixed-size elements. The encoding follows SSZ specifications
// where lists are encoded as their elements in sequence without a length prefix.
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor containing slice metadata and element type information
//   - sourceValue: The reflect.Value of the slice to encode
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if encoding fails or slice exceeds size constraints
//
// Special handling:
//   - Byte slices use optimized bulk append
//   - Returns ErrListTooBig if slice exceeds maximum size from hints
func marshalList[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	if sourceType.GoTypeFlags&GoTypeFlagIsString != 0 {
		stringBytes := []byte(sourceValue.String())
		encoder.EncodeBytes(stringBytes)
	} else if sourceType.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		encoder.EncodeBytes(sourceValue.Bytes())
	} else {
		sliceLen := sourceValue.Len()
		fieldType := sourceType.ElemDesc

		for i := 0; i < sliceLen; i++ {
			itemVal := sourceValue.Index(i)
			if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				if itemVal.IsNil() {
					itemVal = reflect.New(fieldType.Type.Elem())
				}
			}

			err := marshalType(d, fieldType, itemVal, encoder, idt+2)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// marshalDynamicList encodes lists with variable-size elements into SSZ format.
//
// For lists with variable-size elements, SSZ requires a special encoding:
//  1. A series of 4-byte offsets, one per element, indicating where each element's data begins
//  2. The actual encoded data for each element, in order
//
// The offsets are relative to the start of the list encoding (not the entire message).
// This allows decoders to locate each variable-size element without parsing all preceding elements.
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor with list metadata
//   - sourceValue: The reflect.Value of the list to encode
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if encoding fails or size constraints are violated
func marshalDynamicList[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	fieldType := sourceType.ElemDesc
	sliceLen := sourceValue.Len()

	canSeek := encoder.Seekable()
	startOffset := encoder.GetPosition()
	totalOffsets := sliceLen
	offset := 4 * totalOffsets

	if canSeek {
		encoder.EncodeZeroPadding(4 * totalOffsets) // Reserve space for offsets
	} else if sliceLen > 0 {
		// need to calculate the object sizes now
		encoder.EncodeOffset(uint32(offset))

		for i := 0; i < sliceLen-1; i++ {
			itemVal := sourceValue.Index(i)
			size, err := d.getSszValueSize(fieldType, itemVal)
			if err != nil {
				return fmt.Errorf("failed to get size of dynamic list element %v: %w", itemVal.Type().Name(), err)
			}

			offset += int(size)
			encoder.EncodeOffset(uint32(offset))
		}
	}

	bufLen := encoder.GetPosition()

	for i := 0; i < sliceLen; i++ {
		itemVal := sourceValue.Index(i)

		err := marshalType(d, fieldType, itemVal, encoder, idt+2)
		if err != nil {
			return err
		}

		if canSeek {
			encoder.EncodeOffsetAt(startOffset+(i*4), uint32(offset))

			newPos := encoder.GetPosition()
			offset += newPos - bufLen
			bufLen = newPos
		}
	}

	return nil
}

// marshalBitlist encodes bitlist values into SSZ-encoded data.
//
// This function handles bitlist encoding. The encoding follows SSZ specifications
// where bitlists are encoded as their bits in sequence without a length prefix.
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor containing bitlist metadata
//   - sourceValue: The reflect.Value of the bitlist to encode
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if encoding fails or bitlist exceeds size constraints
func marshalBitlist[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	bytes := sourceValue.Bytes()

	// check if last byte contains termination bit
	if len(bytes) == 0 {
		// empty bitlist, simply append termination bit (0x01)
		// this is a fallback for uninitialized bitlists
		bytes = []byte{0x01}
	} else if bytes[len(bytes)-1] == 0x00 {
		return sszutils.ErrBitlistNotTerminated
	}

	encoder.EncodeBytes(bytes)

	return nil
}

// marshalCompatibleUnion encodes CompatibleUnion values into SSZ-encoded data.
//
// According to the spec:
//   - The encoding is: selector.to_bytes(1, "little") + serialize(value.data)
//   - The selector index is based at 0 if a ProgressiveContainer type option is present
//   - Otherwise, it is based at 1
//
// Type Parameters:
//   - E: An encoder type implementing sszutils.Encoder
//
// Parameters:
//   - d: The DynSsz instance providing configuration and caching
//   - sourceType: The TypeDescriptor containing union metadata and variant descriptors
//   - sourceValue: The reflect.Value of the CompatibleUnion to encode
//   - encoder: The encoder instance used to write SSZ-encoded data
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if encoding fails
func marshalCompatibleUnion[E sszutils.Encoder](d *DynSsz, sourceType *TypeDescriptor, sourceValue reflect.Value, encoder E, idt int) error {
	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	variant := uint8(sourceValue.Field(0).Uint())
	dataField := sourceValue.Field(1)

	// Append variant byte
	encoder.EncodeUint8(variant)

	// Get the variant descriptor
	variantDesc, ok := sourceType.UnionVariants[variant]
	if !ok {
		return sszutils.ErrInvalidUnionVariant
	}

	// Marshal the data using the variant's type descriptor
	err := marshalType(d, variantDesc, dataField.Elem(), encoder, idt+2)
	if err != nil {
		return fmt.Errorf("failed to marshal union variant %d: %w", variant, err)
	}

	return nil
}
