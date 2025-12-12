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
	"github.com/pk910/dynamic-ssz/stream"
)

// unmarshalReaderContext holds context for streaming unmarshal operations
type unmarshalReaderContext struct {
	buffer []byte
	reader *stream.LimitedReader
}

// newUnmarshalReaderContext creates a new unmarshal reader context
func newUnmarshalReaderContext(reader io.Reader, bufSize uint32) *unmarshalReaderContext {
	if bufSize <= 0 {
		bufSize = defaultBufferSize
	}
	return &unmarshalReaderContext{
		buffer: make([]byte, bufSize),
		reader: stream.NewLimitedReader(reader),
	}
}

// unmarshalTypeReader is the core recursive function for decoding SSZ-encoded data into Go values using a reader.
//
// This function serves as the primary dispatcher within the unmarshalling process, handling both
// primitive and composite types. It uses the TypeDescriptor's metadata to determine the most
// efficient decoding path, automatically leveraging fastssz when possible for optimal performance.
//
// Parameters:
//   - ctx: The unmarshal reader context
//   - targetType: The TypeDescriptor containing optimized metadata about the type to decode
//   - targetValue: The reflect.Value where decoded data will be stored
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

func (d *DynSsz) unmarshalTypeReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
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
	useDynamicReader := targetType.SszCompatFlags&SszCompatFlagDynamicReader != 0
	useDynamicUnmarshal := targetType.SszCompatFlags&SszCompatFlagDynamicUnmarshaler != 0 && !(hasDynamicSize || targetType.Size > d.BufferSize)
	useFastSsz := !d.NoFastSsz && isFastsszUnmarshaler && !(hasDynamicSize || targetType.Size > d.BufferSize)
	if !useFastSsz && targetType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), targetType.Type.Name(), targetType.Kind, useFastSsz, isFastsszUnmarshaler, hasDynamicSize)
	}

	if useDynamicReader {
		// Use dynamic unmarshaler - can always be used even with dynamic specs
		unmarshaller, ok := targetValue.Addr().Interface().(sszutils.DynamicReader)
		if ok {
			err := unmarshaller.UnmarshalSSZDynReader(d, ctx.reader)
			if err != nil {
				return err
			}
		} else {
			useDynamicReader = false
		}
	}

	if !useDynamicReader && useFastSsz {
		unmarshaller, ok := targetValue.Addr().Interface().(sszutils.FastsszUnmarshaler)
		if ok {
			buf := ctx.buffer[:targetType.Size]
			if _, err := io.ReadFull(ctx.reader, buf); err != nil {
				return err
			}

			err := unmarshaller.UnmarshalSSZ(buf)
			if err != nil {
				return err
			}
		} else {
			useFastSsz = false
		}
	}

	if !useDynamicReader && !useFastSsz && useDynamicUnmarshal {
		// Use dynamic unmarshaler - can always be used even with dynamic specs
		unmarshaller, ok := targetValue.Addr().Interface().(sszutils.DynamicUnmarshaler)
		if ok {
			buf := ctx.buffer[:targetType.Size]
			if _, err := io.ReadFull(ctx.reader, buf); err != nil {
				return err
			}

			err := unmarshaller.UnmarshalSSZDyn(d, buf)
			if err != nil {
				return err
			}
		} else {
			useDynamicUnmarshal = false
		}
	}

	if !useDynamicReader && !useFastSsz && !useDynamicUnmarshal {
		// can't use fastssz, use dynamic unmarshaling
		var err error
		switch targetType.SszType {
		// complex types
		case SszTypeWrapperType:
			err = d.unmarshalTypeWrapperReader(ctx, targetType, targetValue, idt)
			if err != nil {
				return err
			}
		case SszContainerType, SszProgressiveContainerType:
			err = d.unmarshalContainerReader(ctx, targetType, targetValue, idt)
			if err != nil {
				return err
			}
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			if targetType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = d.unmarshalDynamicVectorReader(ctx, targetType, targetValue, idt)
			} else {
				err = d.unmarshalVectorReader(ctx, targetType, targetValue, idt)
			}
			if err != nil {
				return err
			}
		case SszListType, SszProgressiveListType:
			if targetType.ElemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				err = d.unmarshalDynamicListReader(ctx, targetType, targetValue, idt)
			} else {
				err = d.unmarshalListReader(ctx, targetType, targetValue, idt)
			}
			if err != nil {
				return err
			}
		case SszBitlistType, SszProgressiveBitlistType:
			err = d.unmarshalBitlistReader(ctx, targetType, targetValue, idt)
			if err != nil {
				return err
			}
		case SszCompatibleUnionType:
			err = d.unmarshalCompatibleUnionReader(ctx, targetType, targetValue, idt)
			if err != nil {
				return err
			}

		// primitive types
		case SszBoolType:
			boolVal, err := sszutils.UnmarshalBoolReader(ctx.reader)
			if err != nil {
				return err
			}
			targetValue.SetBool(boolVal)
		case SszUint8Type:
			uint8Val, err := sszutils.UnmarshallUint8Reader(ctx.reader)
			if err != nil {
				return err
			}
			targetValue.SetUint(uint64(uint8Val))
		case SszUint16Type:
			uint16Val, err := sszutils.UnmarshallUint16Reader(ctx.reader)
			if err != nil {
				return err
			}
			targetValue.SetUint(uint64(uint16Val))
		case SszUint32Type:
			uint32Val, err := sszutils.UnmarshallUint32Reader(ctx.reader)
			if err != nil {
				return err
			}
			targetValue.SetUint(uint64(uint32Val))
		case SszUint64Type:
			uint64Val, err := sszutils.UnmarshallUint64Reader(ctx.reader)
			if err != nil {
				return err
			}
			if targetType.GoTypeFlags&GoTypeFlagIsTime != 0 {
				timeVal := time.Unix(int64(uint64Val), 0)
				var timeRefVal reflect.Value
				if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
					timeRefVal = reflect.New(targetType.Type.Elem())
					timeRefVal.Elem().Set(reflect.ValueOf(timeVal))
				} else {
					timeRefVal = reflect.ValueOf(timeVal)
				}

				targetValue.Set(timeRefVal)
			} else {
				targetValue.SetUint(uint64(uint64Val))
			}

		default:
			return fmt.Errorf("unknown type: %v", targetType)
		}
	}

	return nil
}

// unmarshalTypeWrapperReader unmarshals a TypeWrapper by extracting the wrapped data and unmarshaling it as the wrapped type
//
// Parameters:
//   - targetType: The TypeDescriptor containing wrapper field metadata
//   - targetValue: The reflect.Value of the wrapper to populate
//   - ssz: The SSZ-encoded data to decode
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed from the SSZ data
//   - error: An error if decoding fails or data is malformed
//
// The function validates that the Data field is present and unmarshals the wrapped value using its type descriptor.

func (d *DynSsz) unmarshalTypeWrapperReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
	if d.Verbose {
		fmt.Printf("%sunmarshalTypeWrapper: %s\n", strings.Repeat(" ", idt), targetType.Type.Name())
	}

	// Get the Data field from the TypeWrapper
	dataField := targetValue.Field(0)
	if !dataField.IsValid() {
		return fmt.Errorf("TypeWrapper missing 'Data' field")
	}

	// Unmarshal the wrapped value using its type descriptor
	err := d.unmarshalTypeReader(ctx, targetType.ElemDesc, dataField, idt+2)
	if err != nil {
		return err
	}

	return nil
}

// unmarshalContainerReader decodes SSZ-encoded container data.
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
//   - ctx: The unmarshal reader context
//   - targetType: The TypeDescriptor containing container field metadata
//   - targetValue: The reflect.Value of the container to populate
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails or data is malformed
//
// The function validates offset integrity to ensure variable fields don't overlap
// and that all data is consumed correctly.

func (d *DynSsz) unmarshalContainerReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
	dynamicFieldCount := len(targetType.ContainerDesc.DynFields)
	dynamicOffsets := defaultOffsetSlicePool.Get()
	defer defaultOffsetSlicePool.Put(dynamicOffsets)

	for i := 0; i < len(targetType.ContainerDesc.Fields); i++ {
		field := targetType.ContainerDesc.Fields[i]

		fieldSize := int(field.Type.Size)
		if fieldSize > 0 {
			// static size field
			ctx.reader.PushLimit(uint64(fieldSize))

			// fmt.Printf("%sfield %d:\t static [%v:%v] %v\t %v\n", strings.Repeat(" ", idt+1), i, offset, offset+fieldSize, fieldSize, field.Name)

			fieldValue := targetValue.Field(i)
			err := d.unmarshalTypeReader(ctx, field.Type, fieldValue, idt+2)
			if err != nil {
				return fmt.Errorf("failed decoding field %v: %v", field.Name, err)
			}

			consumedBytes := ctx.reader.PopLimit()
			if consumedBytes != uint64(fieldSize) {
				return fmt.Errorf("container field did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, fieldSize)
			}
		} else {
			// dynamic size field
			fieldSize = 4

			fieldOffset, err := sszutils.ReadOffsetReader(ctx.reader)
			if err != nil {
				return err
			}

			dynamicOffsets = append(dynamicOffsets, int(fieldOffset))
		}
	}

	// finished parsing the static size fields, process dynamic fields
	for i, field := range targetType.ContainerDesc.DynFields {
		// Calculate field size from this offset to the next offset or EOF
		var fieldSize uint64
		hasKnownSize := false
		if i+1 < dynamicFieldCount {
			// Next field starts at next offset
			fieldSize = uint64(dynamicOffsets[i+1] - dynamicOffsets[i])
			ctx.reader.PushLimit(fieldSize)
			hasKnownSize = true
		}

		fieldDescriptor := field.Field
		fieldValue := targetValue.Field(int(field.Index))
		err := d.unmarshalTypeReader(ctx, fieldDescriptor.Type, fieldValue, idt+2)
		if err != nil {
			return fmt.Errorf("failed decoding field %v: %v", fieldDescriptor.Name, err)
		}

		if hasKnownSize {
			consumedBytes := ctx.reader.PopLimit()
			if consumedBytes != fieldSize {
				return fmt.Errorf("struct field did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, fieldSize)
			}
		}
	}

	return nil
}

// unmarshalVectorReader decodes SSZ-encoded vector data.
//
// Vectors in SSZ are encoded as fixed-size sequences. Since the vector length is known
// from the type, the function can calculate each element's size by dividing the total
// SSZ data length by the vector length.
//
// Parameters:
//   - ctx: The unmarshal reader context
//   - targetType: The TypeDescriptor containing vector metadata
//   - targetValue: The reflect.Value of the vector to populate
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

func (d *DynSsz) unmarshalVectorReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
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

	if targetType.GoTypeFlags&GoTypeFlagIsString != 0 {
		strBytes := make([]byte, arrLen)
		if _, err := io.ReadFull(ctx.reader, strBytes); err != nil {
			return err
		}
		newValue.SetString(string(strBytes))
	} else if targetType.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		bytes := newValue.Slice(0, arrLen).Bytes()

		read, err := io.ReadFull(ctx.reader, bytes)
		if err != nil {
			return err
		} else if read != arrLen {
			return sszutils.ErrUnexpectedEOF
		}

		// shortcut for performance: use copy on []byte arrays
		if targetType.BitSize > 0 && targetType.BitSize < uint32(read)*8 {
			// check padding bits
			paddingMask := uint8((uint16(0xff) << (targetType.BitSize % 8)) & 0xff)
			paddingBits := bytes[arrLen-1] & paddingMask
			if paddingBits != 0 {
				return fmt.Errorf("bitvector padding bits are not zero")
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

			ctx.reader.PushLimit(uint64(itemSize))

			err := d.unmarshalTypeReader(ctx, fieldType, itemVal, idt+2)
			if err != nil {
				return err
			}

			consumedBytes := ctx.reader.PopLimit()
			if consumedBytes != uint64(itemSize) {
				return fmt.Errorf("vector item did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, itemSize)
			}
		}
	}

	if targetType.Kind != reflect.Array {
		targetValue.Set(newValue)
	}

	return nil
}

// unmarshalDynamicVectorReader decodes vectors with variable-size elements from SSZ format.
//
// For vectors with variable-size elements, SSZ uses an offset-based encoding:
//   - The given number of offsets are decoded first, 4 bytes each
//   - Element data appears after all offsets, in order
//
// Parameters:
//   - ctx: The unmarshal reader context
//   - targetType: The TypeDescriptor with vector metadata
//   - targetValue: The reflect.Value where the vector will be stored
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if offsets are invalid or decoding fails
//
// The function validates that:
//   - Offsets are monotonically increasing
//   - No offset points outside the data bounds
//   - Each element consumes exactly the expected bytes

func (d *DynSsz) unmarshalDynamicVectorReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
	vectorLen := int(targetType.Len)

	// read all item offsets
	sliceOffsets := defaultOffsetSlicePool.Get()
	defer defaultOffsetSlicePool.Put(sliceOffsets)
	if cap(sliceOffsets) < vectorLen {
		sliceOffsets = make([]int, vectorLen)
	} else {
		sliceOffsets = sliceOffsets[:vectorLen]
	}
	for i := 0; i < vectorLen; i++ {
		fieldOffset, err := sszutils.ReadOffsetReader(ctx.reader)
		if err != nil {
			return err
		}
		sliceOffsets[i] = int(fieldOffset)
	}

	fieldType := targetType.ElemDesc

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	offset := sliceOffsets[0]
	if offset != vectorLen*4 {
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

		var fieldSize uint64
		hasKnownSize := false
		if i < vectorLen-1 {
			// Next field starts at next offset
			fieldSize = uint64(sliceOffsets[i+1] - sliceOffsets[i])
			ctx.reader.PushLimit(fieldSize)
			hasKnownSize = true
		}

		err := d.unmarshalTypeReader(ctx, fieldType, itemVal, idt+2)
		if err != nil {
			return err
		}

		if hasKnownSize {
			consumedBytes := ctx.reader.PopLimit()
			if consumedBytes != fieldSize {
				return fmt.Errorf("dynamic vector item did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, fieldSize)
			}
		}
	}

	targetValue.Set(newValue)

	return nil
}

// unmarshalListReader decodes SSZ-encoded list data.
//
// This function handles lists with fixed-size elements. For lists with variable-size
// elements, it delegates to unmarshalDynamicListReader. The list length is determined by
// dividing the SSZ data length by the element size.
//
// Parameters:
//   - ctx: The unmarshal reader context
//   - targetType: The TypeDescriptor containing list metadata
//   - targetValue: The reflect.Value where the list will be stored
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if decoding fails or data length is invalid
//
// The function:
//   - Handles both fixed-size and variable-size elements
//   - Uses optimized copying for byte lists
//   - Validates that each element consumes exactly the expected bytes

func (d *DynSsz) unmarshalListReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
	elemDesc := targetType.ElemDesc

	sszLen, knownSize := ctx.reader.BytesRemaining()
	elemSize := uint64(elemDesc.Size)

	// --- String fast path (string is encoded like a byte list) ---
	if targetType.GoTypeFlags&GoTypeFlagIsString != 0 {
		// Must be a byte list.
		if elemDesc.Type.Kind() != reflect.Uint8 || elemSize != 1 {
			return fmt.Errorf("string list must have uint8 elements (size=1), got %v (size=%d)", elemDesc.Type, elemDesc.Size)
		}

		// Known-size: read exactly sszLen bytes.
		if knownSize {
			if sszLen == 0 {
				targetValue.SetString("")
				return nil
			}
			buf := make([]byte, int(sszLen))
			n, err := io.ReadFull(ctx.reader, buf)
			if err != nil {
				return err
			}
			if n != len(buf) {
				return sszutils.ErrUnexpectedEOF
			}
			targetValue.SetString(string(buf))
			return nil
		}

		// Unknown-size: read until EOF (io.ErrUnexpectedEOF is a final partial chunk).
		var b strings.Builder
		for {
			n, err := io.ReadFull(ctx.reader, ctx.buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				if err == io.ErrUnexpectedEOF {
					if n > 0 {
						b.Write(ctx.buffer[:n])
					}
					break
				}
				return err
			}
			if n > 0 {
				b.Write(ctx.buffer[:n])
			}
		}

		targetValue.SetString(b.String())
		return nil
	}

	// If we know the total size, it must be a multiple of elemSize.
	var count int
	if knownSize {
		if elemSize == 0 {
			return fmt.Errorf("invalid element size 0 for %s", elemDesc.Type.Name())
		}
		if sszLen%elemSize != 0 {
			return fmt.Errorf("invalid list length, expected multiple of %v, got %v", elemSize, sszLen)
		}
		count = int(sszLen / elemSize)
	}

	// Resolve the concrete container type we allocate into.
	containerT := targetType.Type
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		containerT = containerT.Elem()
	}
	isSlice := targetType.Kind == reflect.Slice
	if !isSlice && targetType.Kind != reflect.Array {
		return fmt.Errorf("unsupported list container kind %v", targetType.Kind)
	}

	// Allocate destination container.
	// - knownSize: exact-sized slice
	// - unknownSize: start empty slice, or zero array and fill incrementally
	var out reflect.Value
	if isSlice {
		if knownSize {
			// Optimization: avoid reflect.MakeSlice for common byte slices.
			if targetType.GoTypeFlags&GoTypeFlagIsByteArray != 0 && elemDesc.Type.Kind() == reflect.Uint8 {
				out = reflect.ValueOf(make([]byte, count))
			} else {
				out = reflect.MakeSlice(containerT, count, count)
			}
		} else {
			// Unknown size: start with len=0, small cap.
			out = reflect.MakeSlice(containerT, 0, 16)
		}

	} else {
		out = reflect.New(containerT).Elem()
		if knownSize && count > out.Len() {
			return sszutils.ErrListTooBig
		}
	}

	// --- Fast paths for string / []byte / [N]byte ---
	if targetType.GoTypeFlags&GoTypeFlagIsString != 0 {
		// Read the entire remaining stream.
		// If you have a bounded reader, this stays safe even for unknownSize.
		var b strings.Builder
		for {
			n, err := io.ReadFull(ctx.reader, ctx.buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				// io.ReadFull returns ErrUnexpectedEOF when it reads some bytes then hits EOF;
				// we still want to write those bytes.
				if err == io.ErrUnexpectedEOF {
					b.Write(ctx.buffer[:n])
					break
				}
				return err
			}
			b.Write(ctx.buffer[:n])
		}
		out.SetString(b.String())
		targetValue.Set(out)
		return nil
	}

	if targetType.GoTypeFlags&GoTypeFlagIsByteArray != 0 && elemDesc.Type.Kind() == reflect.Uint8 {
		// Byte list/array shortcut.
		if isSlice {
			// out is []byte (either exact-sized if known, or empty if unknown)
			if knownSize {
				n, err := io.ReadFull(ctx.reader, out.Bytes())
				if err != nil {
					return err
				}
				if n != len(out.Bytes()) {
					return sszutils.ErrUnexpectedEOF
				}
			} else {
				// Unknown size: append chunks.
				for {
					n, err := io.ReadFull(ctx.reader, ctx.buffer)
					if err != nil {
						if err == io.EOF {
							break
						}
						if err == io.ErrUnexpectedEOF {
							out = reflect.AppendSlice(out, reflect.ValueOf(ctx.buffer[:n]))
							break
						}
						return err
					}
					out = reflect.AppendSlice(out, reflect.ValueOf(ctx.buffer[:n]))
				}
			}

		} else {
			dst := out.Slice(0, out.Len()).Bytes()
			if knownSize {
				// Only fill the first "count" bytes (count == sszLen for byte lists).
				if count > len(dst) {
					return sszutils.ErrListTooBig
				}
				n, err := io.ReadFull(ctx.reader, dst[:count])
				if err != nil {
					return err
				}
				if n != count {
					return sszutils.ErrUnexpectedEOF
				}
			} else {
				// Unknown size: fill incrementally, no overflow.
				offset := 0
				for {
					n, err := io.ReadFull(ctx.reader, ctx.buffer)
					if err != nil {
						if err == io.EOF {
							break
						}
						if err == io.ErrUnexpectedEOF {
							if offset+n > len(dst) {
								return sszutils.ErrListTooBig
							}
							copy(dst[offset:offset+n], ctx.buffer[:n])
							break
						}
						return err
					}
					if offset+n > len(dst) {
						return sszutils.ErrListTooBig
					}
					copy(dst[offset:offset+n], ctx.buffer[:n])
					offset += n
				}
			}
		}

		targetValue.Set(out)
		return nil
	}

	// --- Generic static-size element decoding ---
	decodeOne := func(item reflect.Value) (consumed uint64, err error) {
		ctx.reader.PushLimit(elemSize)
		err = d.unmarshalTypeReader(ctx, elemDesc, item, idt+2)
		consumed = ctx.reader.PopLimit()
		return consumed, err
	}

	// Known-size: decode exactly count items into pre-sized container.
	if knownSize {
		for i := 0; i < count; i++ {
			var itemVal reflect.Value

			// Get/set element slot (handle pointer elements).
			slot := out.Index(i)
			if elemDesc.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				itemVal = reflect.New(elemDesc.Type.Elem())
				slot.Set(itemVal)
			} else {
				itemVal = slot
			}

			consumed, err := decodeOne(itemVal)
			if err != nil {
				return err
			}
			if consumed != elemSize {
				return fmt.Errorf(
					"list item did not consume expected ssz range (consumed: %v, expected: %v)",
					consumed, elemSize,
				)
			}
		}

		targetValue.Set(out)
		return nil
	}

	// Unknown-size: decode until EOF.
	i := 0
	for {
		// Ensure capacity / bounds.
		var itemVal reflect.Value
		if isSlice {
			// Create a new element value to decode into, then append.
			var elem reflect.Value
			if elemDesc.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				elem = reflect.New(elemDesc.Type.Elem())
				itemVal = elem
			} else {
				elem = reflect.New(elemDesc.Type).Elem()
				itemVal = elem
			}

			consumed, err := decodeOne(itemVal)
			if err != nil {
				if consumed == 0 {
					break // clean end-of-list
				}
				return err
			}
			if consumed != elemSize {
				return fmt.Errorf(
					"list item did not consume expected ssz range (consumed: %v, expected: %v)",
					consumed, elemSize,
				)
			}

			// Append decoded element.
			if elemDesc.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				out = reflect.Append(out, itemVal)
			} else {
				out = reflect.Append(out, itemVal)
			}

		} else {
			// Arrays: decode into out[i] until EOF, but do not overflow.
			if i >= out.Len() {
				return sszutils.ErrListTooBig
			}

			slot := out.Index(i)
			if elemDesc.GoTypeFlags&GoTypeFlagIsPointer != 0 {
				itemVal = reflect.New(elemDesc.Type.Elem())
				slot.Set(itemVal)
			} else {
				itemVal = slot
			}

			consumed, err := decodeOne(itemVal)
			if err != nil {
				if consumed == 0 {
					break // clean end-of-list
				}
				return err
			}
			if consumed != elemSize {
				return fmt.Errorf(
					"list item did not consume expected ssz range (consumed: %v, expected: %v)",
					consumed, elemSize,
				)
			}

			i++
		}
	}

	targetValue.Set(out)
	return nil
}

// unmarshalDynamicListReader decodes lists with variable-size elements from SSZ format.
//
// For lists with variable-size elements, SSZ uses an offset-based encoding:
//   - The first 4 bytes contain the offset to the first element's data
//   - The number of elements is derived by dividing this offset by 4
//   - Each subsequent 4-byte value is an offset to the next element
//   - Element data appears after all offsets, in order
//
// Parameters:
//   - ctx: The unmarshal reader context
//   - targetType: The TypeDescriptor with list metadata
//   - targetValue: The reflect.Value where the list will be stored
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if offsets are invalid or decoding fails
//
// The function validates that:
//   - Offsets are monotonically increasing
//   - No offset points outside the data bounds
//   - Each element consumes exactly the expected bytes

func (d *DynSsz) unmarshalDynamicListReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
	// derive number of items from first item offset
	firstOffset, err := sszutils.ReadOffsetReader(ctx.reader)
	if err != nil && err != sszutils.ErrUnexpectedEOF {
		return err
	}
	sliceLen := int(firstOffset / 4)

	// fmt.Printf("new dynamic slice %v  %v\n", fieldType.Name(), sliceLen)
	fieldT := targetType.Type
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	var newValue reflect.Value
	if targetType.Kind == reflect.Slice {
		newValue = reflect.MakeSlice(fieldT, sliceLen, sliceLen)
	} else {
		newValue = reflect.New(fieldT).Elem()
	}

	if sliceLen > 0 {
		// read all item offsets
		sliceOffsets := defaultOffsetSlicePool.Get()
		defer defaultOffsetSlicePool.Put(sliceOffsets)
		if cap(sliceOffsets) < sliceLen {
			sliceOffsets = make([]int, sliceLen)
		} else {
			sliceOffsets = sliceOffsets[:sliceLen]
		}
		sliceOffsets[0] = int(firstOffset)
		for i := 1; i < sliceLen; i++ {
			offset, err := sszutils.ReadOffsetReader(ctx.reader)
			if err != nil {
				return err
			}
			sliceOffsets[i] = int(offset)
		}

		fieldType := targetType.ElemDesc

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

			itemSize := 0
			knownSize := false
			if i < sliceLen-1 {
				knownSize = true
				itemSize = sliceOffsets[i+1] - sliceOffsets[i]
				ctx.reader.PushLimit(uint64(itemSize))
			}

			err := d.unmarshalTypeReader(ctx, fieldType, itemVal, idt+2)
			if err != nil {
				return err
			}

			if knownSize {
				consumed := ctx.reader.PopLimit()
				if consumed != uint64(itemSize) {
					return fmt.Errorf("dynamic list item did not consume expected ssz range (consumed: %v, expected: %v)", consumed, itemSize)
				}
			}
		}
	}

	targetValue.Set(newValue)

	return nil
}

// unmarshalBitlistReader decodes bitlist values from SSZ-encoded data.
//
// This function handles bitlist decoding. The decoding follows SSZ specifications
// where bitlists are encoded as their bits in sequence without a length prefix, but with a termination bit.
// The termination bit is a single `1` bit appended immediately after the final data bit, then padded to a full byte.
// The position of this termination bit defines the logical length of the bitlist. Bitlists without a termination bit are not allowed.
//
// Parameters:
//   - targetType: The TypeDescriptor containing bitlist metadata
//   - targetValue: The reflect.Value of the bitlist to populate
//   - ssz: The SSZ-encoded data to decode
//
// Returns:
//   - int: Total bytes consumed from the SSZ data
//   - error: An error if decoding fails or bitlist is invalid

func (d *DynSsz) unmarshalBitlistReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
	sszLen, knownSize := ctx.reader.BytesRemaining()

	// Resolve concrete container type (dereference pointer target).
	fieldT := targetType.Type
	if targetType.GoTypeFlags&GoTypeFlagIsPointer != 0 {
		fieldT = fieldT.Elem()
	}

	isSlice := targetType.Kind == reflect.Slice
	if !isSlice && targetType.Kind != reflect.Array {
		return fmt.Errorf("bitlist must be []byte or [N]byte, got kind %v", targetType.Kind)
	}

	// Allocate destination container.
	var out reflect.Value
	if isSlice {
		if knownSize {
			out = reflect.ValueOf(make([]byte, int(sszLen)))
		} else {
			out = reflect.MakeSlice(fieldT, 0, 16)
		}
	} else {
		out = reflect.New(fieldT).Elem()
		if knownSize && int(sszLen) > out.Len() {
			return sszutils.ErrListTooBig
		}
	}

	// Termination rule: last byte must be non-zero.
	validateTerminated := func(total int, last byte) error {
		if total == 0 || last == 0x00 {
			return sszutils.ErrBitlistNotTerminated
		}
		return nil
	}

	// ---- Known total length: read exactly sszLen bytes ----
	if knownSize {
		if sszLen == 0 {
			return sszutils.ErrBitlistNotTerminated
		}

		var dst []byte
		if isSlice {
			dst = out.Bytes()
		} else {
			dst = out.Slice(0, int(sszLen)).Bytes()
		}

		n, err := io.ReadFull(ctx.reader, dst)
		if err != nil {
			return err
		}
		if n != len(dst) {
			return sszutils.ErrUnexpectedEOF
		}
		if err := validateTerminated(len(dst), dst[len(dst)-1]); err != nil {
			return err
		}

		targetValue.Set(out)
		return nil
	}

	// ---- Unknown total length: read until EOF ----
	if isSlice {
		last := byte(0)
		total := 0

		for {
			n, err := io.ReadFull(ctx.reader, ctx.buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				if err == io.ErrUnexpectedEOF {
					if n > 0 {
						last = ctx.buffer[n-1]
						out = reflect.AppendSlice(out, reflect.ValueOf(ctx.buffer[:n]))
						total += n
					}
					break
				}
				return err
			}

			if n > 0 {
				last = ctx.buffer[n-1]
				out = reflect.AppendSlice(out, reflect.ValueOf(ctx.buffer[:n]))
				total += n
			}
		}

		if err := validateTerminated(total, last); err != nil {
			return err
		}

		targetValue.Set(out)
		return nil
	} else {
		dst := out.Slice(0, out.Len()).Bytes()
		offset := 0
		last := byte(0)

		for {
			n, err := io.ReadFull(ctx.reader, ctx.buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				if err == io.ErrUnexpectedEOF {
					if n > 0 {
						if offset+n > len(dst) {
							return sszutils.ErrListTooBig
						}
						copy(dst[offset:offset+n], ctx.buffer[:n])
						offset += n
						last = ctx.buffer[n-1]
					}
					break
				}
				return err
			}

			if n > 0 {
				if offset+n > len(dst) {
					return sszutils.ErrListTooBig
				}
				copy(dst[offset:offset+n], ctx.buffer[:n])
				offset += n
				last = ctx.buffer[n-1]
			}
		}

		if err := validateTerminated(offset, last); err != nil {
			return err
		}

		targetValue.Set(out)
		return nil
	}
}

// unmarshalCompatibleUnionReader decodes SSZ-encoded data into a CompatibleUnion.
//
// According to the spec:
// - The encoding is: selector.to_bytes(1, "little") + serialize(value.data)
// - The selector index is based at 0 if a ProgressiveContainer type option is present
// - Otherwise, it is based at 1
//
// Parameters:
//   - ctx: The unmarshal reader context
//   - targetType: The TypeDescriptor containing union metadata and variant descriptors
//   - targetValue: The reflect.Value of the CompatibleUnion to populate
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - int: Total bytes consumed
//   - error: An error if decoding fails
func (d *DynSsz) unmarshalCompatibleUnionReader(ctx *unmarshalReaderContext, targetType *TypeDescriptor, targetValue reflect.Value, idt int) error {
	// Read the variant byte
	var variantBytes [1]byte
	n, err := io.ReadFull(ctx.reader, variantBytes[:])
	if err != nil {
		return err
	}
	if n != 1 {
		return sszutils.ErrUnexpectedEOF
	}
	variant := variantBytes[0]

	// Get the variant descriptor
	variantDesc, ok := targetType.UnionVariants[variant]
	if !ok {
		return sszutils.ErrInvalidUnionVariant
	}

	// Create a new value of the variant type
	variantValue := reflect.New(variantDesc.Type).Elem()

	// Unmarshal the data
	err = d.unmarshalTypeReader(ctx, variantDesc, variantValue, idt+2)
	if err != nil {
		return fmt.Errorf("failed to unmarshal union variant %d: %w", variant, err)
	}

	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	targetValue.Field(0).SetUint(uint64(variant))
	targetValue.Field(1).Set(variantValue)

	return nil
}
