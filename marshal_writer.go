// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// marshalWriterContext holds context for streaming marshal operations
type marshalWriterContext struct {
	writer io.Writer
	buffer []byte
}

// newMarshalWriterContext creates a new marshal writer context
func newMarshalWriterContext(w io.Writer, bufSize uint32) *marshalWriterContext {
	if bufSize <= 0 {
		bufSize = defaultBufferSize
	}
	return &marshalWriterContext{
		writer: w,
		buffer: make([]byte, 0, bufSize),
	}
}

// marshalTypeWriter is the core recursive function for marshalling Go values into SSZ-encoded data streams.
//
// This function serves as the primary dispatcher within the marshalling process, handling both primitive
// and composite types. It uses the TypeDescriptor's metadata to determine the most efficient encoding
// path, automatically leveraging fastssz when possible for optimal performance.
//
// Parameters:
//   - ctx: The marshal writer context
//   - sourceType: The TypeDescriptor containing optimized metadata about the type to be encoded
//   - sourceValue: The reflect.Value holding the data to be encoded
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

func (d *DynSsz) marshalTypeWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	if sourceType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		if sourceValue.IsNil() {
			sourceValue = reflect.New(sourceType.Type.Elem()).Elem()
		} else {
			sourceValue = sourceValue.Elem()
		}
	}

	hasDynamicSize := sourceType.SszTypeFlags&SszTypeFlagHasDynamicSize != 0
	isFastsszMarshaler := sourceType.SszCompatFlags&SszCompatFlagFastSSZMarshaler != 0
	useDynamicWriter := sourceType.SszCompatFlags&SszCompatFlagDynamicWriter != 0
	useDynamicMarshal := sourceType.SszCompatFlags&SszCompatFlagDynamicMarshaler != 0 && !(hasDynamicSize || sourceType.Size > d.BufferSize)
	useFastSsz := !d.NoFastSsz && isFastsszMarshaler && !(hasDynamicSize || sourceType.Size > d.BufferSize)
	if !useFastSsz && sourceType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), sourceType.Type.Name(), sourceType.Kind, useFastSsz, isFastsszMarshaler, hasDynamicSize)
	}

	if useDynamicWriter {
		// Use dynamic marshaler - can always be used even with dynamic specs
		marshaller, ok := getPtr(sourceValue).Interface().(sszutils.DynamicWriter)
		if ok {
			err := marshaller.MarshalSSZDynWriter(d, ctx.writer)
			if err != nil {
				return err
			}
		} else {
			useDynamicWriter = false
		}
	}

	if !useDynamicWriter && useFastSsz {
		marshaller, ok := getPtr(sourceValue).Interface().(sszutils.FastsszMarshaler)
		if ok {
			buf := ctx.buffer[:0]
			newBuf, err := marshaller.MarshalSSZTo(buf)
			if err != nil {
				return err
			}
			_, err = ctx.writer.Write(newBuf)
			if err != nil {
				return err
			}
		} else {
			useFastSsz = false
		}
	}

	if !useDynamicWriter && !useFastSsz && useDynamicMarshal {
		// Use dynamic marshaler - can always be used even with dynamic specs
		marshaller, ok := getPtr(sourceValue).Interface().(sszutils.DynamicMarshaler)
		if ok {
			buf := ctx.buffer[:0]
			newBuf, err := marshaller.MarshalSSZDyn(d, buf)
			if err != nil {
				return err
			}
			_, err = ctx.writer.Write(newBuf)
			if err != nil {
				return err
			}
		} else {
			useDynamicMarshal = false
		}
	}

	if !useDynamicWriter && !useFastSsz && !useDynamicMarshal {
		// can't use fastssz, use dynamic marshaling
		var err error
		switch sourceType.SszType {
		// complex types
		case SszTypeWrapperType:
			err = d.marshalTypeWrapperWriter(ctx, sourceType, sourceValue, idt)
			if err != nil {
				return err
			}
		case SszContainerType, SszProgressiveContainerType:
			err = d.marshalContainerWriter(ctx, sourceType, sourceValue, idt)
			if err != nil {
				return err
			}
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			if sourceType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = d.marshalDynamicVectorWriter(ctx, sourceType, sourceValue, idt)
			} else {
				err = d.marshalVectorWriter(ctx, sourceType, sourceValue, idt)
			}
			if err != nil {
				return err
			}
		case SszListType, SszProgressiveListType:
			if sourceType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = d.marshalDynamicListWriter(ctx, sourceType, sourceValue, idt)
			} else {
				err = d.marshalListWriter(ctx, sourceType, sourceValue, idt)
			}
			if err != nil {
				return err
			}
		case SszBitlistType, SszProgressiveBitlistType:
			err = d.marshalBitlistWriter(ctx, sourceType, sourceValue, idt)
			if err != nil {
				return err
			}
		case SszCompatibleUnionType:
			err = d.marshalCompatibleUnionWriter(ctx, sourceType, sourceValue, idt)
			if err != nil {
				return err
			}

		// primitive types
		case SszBoolType:
			err = sszutils.MarshalBoolWriter(ctx.writer, sourceValue.Bool())
		case SszUint8Type:
			err = sszutils.MarshalUint8Writer(ctx.writer, uint8(sourceValue.Uint()))
		case SszUint16Type:
			err = sszutils.MarshalUint16Writer(ctx.writer, uint16(sourceValue.Uint()))
		case SszUint32Type:
			err = sszutils.MarshalUint32Writer(ctx.writer, uint32(sourceValue.Uint()))
		case SszUint64Type:
			if sourceType.GoTypeFlags&GoTypeFlagIsTime != 0 {
				timeValue, isTime := sourceValue.Interface().(time.Time)
				if !isTime {
					return fmt.Errorf("time.Time type expected, got %v", sourceType.Type.Name())
				}
				err = sszutils.MarshalUint64Writer(ctx.writer, uint64(timeValue.Unix()))
			} else {
				err = sszutils.MarshalUint64Writer(ctx.writer, uint64(sourceValue.Uint()))
			}
		default:
			return fmt.Errorf("unknown type: %v", sourceType)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// marshalTypeWrapperWriter marshals a TypeWrapper by extracting its data field and marshaling it as the wrapped type
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

func (d *DynSsz) marshalTypeWrapperWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	if d.Verbose {
		fmt.Printf("%smarshalTypeWrapper: %s\n", strings.Repeat(" ", idt), sourceType.Type.Name())
	}

	// Extract the Data field from the TypeWrapper
	dataField := sourceValue.Field(0)
	if !dataField.IsValid() {
		return fmt.Errorf("TypeWrapper missing 'Data' field")
	}

	// Marshal the wrapped value using its type descriptor
	return d.marshalTypeWriter(ctx, sourceType.ElemDesc, dataField, idt+2)
}

// marshalContainerWriter handles the encoding of container values into SSZ-encoded data.
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

func (d *DynSsz) marshalContainerWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	currentOffset := sourceType.Len
	fieldCount := len(sourceType.ContainerDesc.Fields)

	for i := 0; i < fieldCount; i++ {
		field := sourceType.ContainerDesc.Fields[i]
		fieldSize := field.Type.Size
		fieldValue := sourceValue.Field(i)

		if fieldSize > 0 {
			//fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)
			err := d.marshalTypeWriter(ctx, field.Type, fieldValue, idt+2)
			if err != nil {
				return fmt.Errorf("failed encoding field %v: %v", field.Name, err)
			}
		} else {
			// Dynamic field - write offset
			err := sszutils.MarshalOffsetWriter(ctx.writer, currentOffset)
			if err != nil {
				return err
			}

			size, err := d.getSszValueSize(field.Type, fieldValue)
			if err != nil {
				return err
			}

			// Get size from tree if available
			currentOffset += size
		}
	}

	for _, field := range sourceType.ContainerDesc.DynFields {
		//fmt.Printf("%sfield %d:\t dynamic [%v:]\t %v\n", strings.Repeat(" ", idt+1), field.Index[0], offset, field.Name)

		fieldDescriptor := field.Field
		fieldValue := sourceValue.Field(int(field.Index))
		err := d.marshalTypeWriter(ctx, fieldDescriptor.Type, fieldValue, idt+2)
		if err != nil {
			return fmt.Errorf("failed decoding field %v: %v", fieldDescriptor.Name, err)
		}
	}

	return nil
}

// marshalVectorWriter encodes vector values into SSZ-encoded data.
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

func (d *DynSsz) marshalVectorWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
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

		_, err := ctx.writer.Write(bytes[:dataLen])
		if err != nil {
			return err
		}

		if appendZero > 0 {
			err = sszutils.AppendZeroPaddingWriter(ctx.writer, appendZero)
			if err != nil {
				return err
			}
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
			err := d.marshalTypeWriter(ctx, sourceType.ElemDesc, itemVal, idt+2)
			if err != nil {
				return err
			}
		}

		if appendZero > 0 {
			totalZeroBytes := int(sourceType.ElemDesc.Size) * appendZero
			err := sszutils.AppendZeroPaddingWriter(ctx.writer, totalZeroBytes)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// marshalDynamicVectorWriter encodes vectors with variable-size elements into SSZ format.
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

func (d *DynSsz) marshalDynamicVectorWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	fieldType := sourceType.ElemDesc
	sliceLen := uint32(sourceValue.Len())

	appendZero := uint32(0)
	if sourceType.Kind == reflect.Slice || sourceType.Kind == reflect.String {
		sliceLen := uint32(sourceValue.Len())
		if sliceLen > sourceType.Len {
			return sszutils.ErrListTooBig
		}
		if uint32(sliceLen) < sourceType.Len {
			appendZero = sourceType.Len - sliceLen
		}
	}

	totalOffsets := sliceLen + appendZero

	// First pass: write offsets
	currentOffset := 4 * totalOffsets

	var zeroVal reflect.Value

	for i := uint32(0); i < sliceLen; i++ {
		err := sszutils.MarshalOffsetWriter(ctx.writer, currentOffset)
		if err != nil {
			return err
		}

		// Get size of the element
		size, err := d.getSszValueSize(fieldType, sourceValue.Index(int(i)))
		if err != nil {
			return err
		}

		currentOffset += size
	}

	if appendZero > 0 {
		if fieldType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
			zeroVal = reflect.New(fieldType.Type.Elem())
		} else {
			zeroVal = reflect.New(fieldType.Type).Elem()
		}

		// Get size of the element
		size, err := d.getSszValueSize(fieldType, zeroVal)
		if err != nil {
			return err
		}

		for i := uint32(0); i < appendZero; i++ {
			err := sszutils.MarshalOffsetWriter(ctx.writer, currentOffset)
			if err != nil {
				return err
			}

			currentOffset += size
		}
	}

	// Second pass: write elements
	for i := uint32(0); i < sliceLen; i++ {
		itemVal := sourceValue.Index(int(i))

		err := d.marshalTypeWriter(ctx, fieldType, itemVal, idt+2)
		if err != nil {
			return err
		}
	}

	if appendZero > 0 {
		for i := uint32(0); i < appendZero; i++ {
			err := d.marshalTypeWriter(ctx, fieldType, zeroVal, idt+2)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// marshalListWriter encodes list values into SSZ-encoded data.
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

func (d *DynSsz) marshalListWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	if sourceType.GoTypeFlags&GoTypeFlagIsString != 0 {
		stringBytes := []byte(sourceValue.String())
		_, err := ctx.writer.Write(stringBytes)
		if err != nil {
			return err
		}
	} else if sourceType.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		_, err := ctx.writer.Write(sourceValue.Bytes())
		if err != nil {
			return err
		}
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

			err := d.marshalTypeWriter(ctx, fieldType, itemVal, idt+2)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// marshalDynamicListWriter encodes lists with variable-size elements into SSZ format.
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

func (d *DynSsz) marshalDynamicListWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	fieldType := sourceType.ElemDesc
	sliceLen := uint32(sourceValue.Len())

	currentOffset := 4 * (sliceLen) // Space for offsets

	// First pass: write offsets
	for i := uint32(0); i < sliceLen; i++ {
		err := sszutils.MarshalOffsetWriter(ctx.writer, currentOffset)
		if err != nil {
			return err
		}

		size, err := d.getSszValueSize(fieldType, sourceValue.Index(int(i)))
		if err != nil {
			return err
		}

		currentOffset += size
	}

	// Second pass: write elements
	for i := uint32(0); i < sliceLen; i++ {
		itemVal := sourceValue.Index(int(i))
		err := d.marshalTypeWriter(ctx, fieldType, itemVal, idt+2)
		if err != nil {
			return err
		}
	}

	return nil
}

// marshalBitlist encodes bitlist values into SSZ-encoded data.
//
// This function handles bitlist encoding. The encoding follows SSZ specifications
// where bitlists are encoded as their bits in sequence without a length prefix.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing bitlist metadata
//   - sourceValue: The reflect.Value of the bitlist to encode
//   - buf: The buffer to append encoded data to
//
// Returns:
//   - []byte: The updated buffer with the encoded bitlist
//   - error: An error if encoding fails or bitlist exceeds size constraints

func (d *DynSsz) marshalBitlistWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	var bytes []byte
	if sourceType.GoTypeFlags&GoTypeFlagIsString != 0 {
		bytes = []byte(sourceValue.String())
	} else if sourceType.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		bytes = sourceValue.Bytes()
	} else {
		return fmt.Errorf("bitlist type can only be represented by byte slices or arrays, got %v", sourceType.Kind)
	}

	// check if last byte contains termination bit
	if len(bytes) == 0 {
		// empty bitlist, simply append termination bit (0x01)
		// this is a fallback for uninitialized bitlists
		bytes = []byte{0x01}
	} else if bytes[len(bytes)-1] == 0x00 {
		return sszutils.ErrBitlistNotTerminated
	}

	_, err := ctx.writer.Write(bytes)
	if err != nil {
		return err
	}

	return nil
}

// marshalCompatibleUnionWriter encodes CompatibleUnion values into SSZ-encoded data.
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
func (d *DynSsz) marshalCompatibleUnionWriter(ctx *marshalWriterContext, sourceType *TypeDescriptor, sourceValue reflect.Value, idt int) error {
	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	variant := uint8(sourceValue.Field(0).Uint())
	dataField := sourceValue.Field(1)

	// Append variant byte
	_, err := ctx.writer.Write([]byte{variant})
	if err != nil {
		return err
	}

	// Get the variant descriptor
	variantDesc, ok := sourceType.UnionVariants[variant]
	if !ok {
		return sszutils.ErrInvalidUnionVariant
	}

	// Marshal the data using the variant's type descriptor
	err = d.marshalTypeWriter(ctx, variantDesc, dataField.Elem(), idt+2)
	if err != nil {
		return fmt.Errorf("failed to marshal union variant %d: %w", variant, err)
	}

	return nil
}
