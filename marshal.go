// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pk910/dynamic-ssz/sszutils"
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
	useFastSsz := !d.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && sourceType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), sourceType.Type.Name(), sourceType.Kind, useFastSsz, isFastsszMarshaler, hasDynamicSize)
	}

	if useFastSsz {
		marshaller, ok := sourceValue.Addr().Interface().(sszutils.FastsszMarshaler)
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

	if !useFastSsz && useDynamicMarshal {
		// Use dynamic marshaler - can always be used even with dynamic specs
		marshaller, ok := sourceValue.Addr().Interface().(sszutils.DynamicMarshaler)
		if ok {
			newBuf, err := marshaller.MarshalSSZDyn(d, buf)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		} else {
			useDynamicMarshal = false
		}
	}

	if !useFastSsz && !useDynamicMarshal {
		// can't use fastssz, use dynamic marshaling
		var err error
		switch sourceType.SszType {
		// complex types
		case SszTypeWrapperType:
			buf, err = d.marshalTypeWrapper(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}
		case SszContainerType, SszProgressiveContainerType:
			buf, err = d.marshalContainer(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			if sourceType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				buf, err = d.marshalDynamicVector(sourceType, sourceValue, buf, idt)
			} else {
				buf, err = d.marshalVector(sourceType, sourceValue, buf, idt)
			}
			if err != nil {
				return nil, err
			}
		case SszListType, SszBitlistType, SszProgressiveListType, SszProgressiveBitlistType:
			if sourceType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				buf, err = d.marshalDynamicList(sourceType, sourceValue, buf, idt)
			} else {
				buf, err = d.marshalList(sourceType, sourceValue, buf, idt)
			}
			if err != nil {
				return nil, err
			}
		case SszCompatibleUnionType:
			buf, err = d.marshalCompatibleUnion(sourceType, sourceValue, buf, idt)
			if err != nil {
				return nil, err
			}

		// primitive types
		case SszBoolType:
			buf = sszutils.MarshalBool(buf, sourceValue.Bool())
		case SszUint8Type:
			buf = sszutils.MarshalUint8(buf, uint8(sourceValue.Uint()))
		case SszUint16Type:
			buf = sszutils.MarshalUint16(buf, uint16(sourceValue.Uint()))
		case SszUint32Type:
			buf = sszutils.MarshalUint32(buf, uint32(sourceValue.Uint()))
		case SszUint64Type:
			if sourceType.GoTypeFlags&GoTypeFlagIsTime != 0 {
				timeValue, isTime := sourceValue.Interface().(time.Time)
				if !isTime {
					return nil, fmt.Errorf("time.Time type expected, got %v", sourceType.Type.Name())
				}
				buf = sszutils.MarshalUint64(buf, uint64(timeValue.Unix()))
			} else {
				buf = sszutils.MarshalUint64(buf, uint64(sourceValue.Uint()))
			}
		default:
			return nil, fmt.Errorf("unknown type: %v", sourceType)
		}
	}

	return buf, nil
}

// marshalTypeWrapper marshals a TypeWrapper by extracting its data field and marshaling it as the wrapped type
//
// Parameters:
//   - sourceType: The TypeDescriptor containing wrapper field metadata
//   - sourceValue: The reflect.Value of the wrapper to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with the encoded wrapper
//   - error: An error if any field encoding fails
//
// The function validates that the Data field is present and marshals the wrapped value using its type descriptor.

func (d *DynSsz) marshalTypeWrapper(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	if d.Verbose {
		fmt.Printf("%smarshalTypeWrapper: %s\n", strings.Repeat(" ", idt), sourceType.Type.Name())
	}

	// Extract the Data field from the TypeWrapper
	dataField := sourceValue.Field(0)
	if !dataField.IsValid() {
		return nil, fmt.Errorf("TypeWrapper missing 'Data' field")
	}

	// Marshal the wrapped value using its type descriptor
	return d.marshalType(sourceType.ElemDesc, dataField, buf, idt+2)
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
		if sourceType.Kind == reflect.Array {
			sliceLen = int(sourceType.Len)
		} else {
			return nil, sszutils.ErrListTooBig
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

		buf = append(buf, bytes[:dataLen]...)

		if appendZero > 0 {
			buf = sszutils.AppendZeroPadding(buf, appendZero)
		}
	} else {
		for i := 0; i < dataLen; i++ {
			itemVal := sourceValue.Index(i)
			newBuf, err := d.marshalType(sourceType.ElemDesc, itemVal, buf, idt+2)
			if err != nil {
				return nil, err
			}
			buf = newBuf
		}

		if appendZero > 0 {
			totalZeroBytes := int(sourceType.ElemDesc.Size) * appendZero
			buf = sszutils.AppendZeroPadding(buf, totalZeroBytes)
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
			return nil, sszutils.ErrListTooBig
		}
		if uint32(sliceLen) < sourceType.Len {
			appendZero = int(sourceType.Len) - sliceLen
		}
	}

	startOffset := len(buf)
	totalOffsets := sliceLen + appendZero
	buf = sszutils.AppendZeroPadding(buf, 4*totalOffsets) // Reserve space for offsets

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

		if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
			zeroVal = reflect.New(fieldType.Type.Elem())
		} else {
			zeroVal = reflect.New(fieldType.Type).Elem()
		}

		zeroBuf, err := d.marshalType(fieldType, zeroVal, []byte{}, idt+2)
		if err != nil {
			return nil, err
		}
		zeroBufLen := len(zeroBuf)

		for i := 0; i < appendZero; i++ {
			buf = append(buf, zeroBuf...)
			binary.LittleEndian.PutUint32(buf[startOffset+((sliceLen+i)*4):startOffset+(((sliceLen+i)+1)*4)], uint32(offset))
			offset += zeroBufLen
		}
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
	if sourceType.GoTypeFlags&GoTypeFlagIsString != 0 {
		stringBytes := []byte(sourceValue.String())
		buf = append(buf, stringBytes...)
	} else if sourceType.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		buf = append(buf, sourceValue.Bytes()...)
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
	buf = sszutils.AppendZeroPadding(buf, 4*totalOffsets) // Reserve space for offsets

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

// marshalCompatibleUnion encodes CompatibleUnion values into SSZ-encoded data.
//
// According to the spec:
// - The encoding is: selector.to_bytes(1, "little") + serialize(value.data)
// - The selector index is based at 0 if a ProgressiveContainer type option is present
// - Otherwise, it is based at 1
//
// Parameters:
//   - sourceType: The TypeDescriptor containing union metadata and variant descriptors
//   - sourceValue: The reflect.Value of the CompatibleUnion to encode
//   - buf: The buffer to append encoded data to
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - []byte: The updated buffer with the encoded union
//   - error: An error if encoding fails
func (d *DynSsz) marshalCompatibleUnion(sourceType *TypeDescriptor, sourceValue reflect.Value, buf []byte, idt int) ([]byte, error) {
	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	variant := uint8(sourceValue.Field(0).Uint())
	dataField := sourceValue.Field(1)

	// Append variant byte
	buf = append(buf, variant)

	// Get the variant descriptor
	variantDesc, ok := sourceType.UnionVariants[variant]
	if !ok {
		return nil, sszutils.ErrInvalidUnionVariant
	}

	// Marshal the data using the variant's type descriptor
	newBuf, err := d.marshalType(variantDesc, dataField.Elem(), buf, idt+2)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal union variant %d: %w", variant, err)
	}

	return newBuf, nil
}
