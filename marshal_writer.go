package dynssz

import (
	"fmt"
	"reflect"
)

// marshalTypeWriter marshals a value to a writer using streaming
func (d *DynSsz) marshalTypeWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	if typeDesc.IsPtr {
		if value.IsNil() {
			// Handle nil pointers by creating zero value
			value = reflect.New(typeDesc.Type.Elem())
		}
		value = value.Elem()
	}

	// For small static types or when buffer can hold entire result, use regular marshal
	/* // TODO: re-enable this for production
	if !typeDesc.HasDynamicSize && typeDesc.Size > 0 && typeDesc.Size <= int32(cap(ctx.buffer)) {
		buf := ctx.buffer[:0]
		result, err := d.marshalType(typeDesc, value, buf, 0)
		if err != nil {
			return err
		}
		_, err = ctx.writer.Write(result)
		return err
	}
	*/

	// Use fastssz if available and not disabled
	if !d.NoFastSsz && typeDesc.IsFastSSZMarshaler {
		if marshaler, ok := value.Addr().Interface().(fastsszMarshaler); ok {
			// Use buffer for fastssz marshal
			buf := ctx.buffer[:0]
			result, err := marshaler.MarshalSSZTo(buf)
			if err != nil {
				return err
			}
			_, err = ctx.writer.Write(result)
			return err
		}
	}

	switch typeDesc.Kind {
	// Handle complex types
	case reflect.Struct:
		return d.marshalStructWriter(ctx, typeDesc, value)
	case reflect.Array:
		return d.marshalArrayWriter(ctx, typeDesc, value)
	case reflect.Slice:
		return d.marshalSliceWriter(ctx, typeDesc, value)
	case reflect.String:
		return d.marshalStringWriter(ctx, typeDesc, value)

	// Handle primitive types
	case reflect.Bool:
		return writeBool(ctx.writer, value.Bool())
	case reflect.Uint8:
		return writeUint8(ctx.writer, uint8(value.Uint()))
	case reflect.Uint16:
		return writeUint16(ctx.writer, uint16(value.Uint()))
	case reflect.Uint32:
		return writeUint32(ctx.writer, uint32(value.Uint()))
	case reflect.Uint64:
		return writeUint64(ctx.writer, value.Uint())
	default:
		return fmt.Errorf("unsupported type for streaming marshal: %v", typeDesc.Kind)
	}
}

// marshalStructWriter marshals a struct to a writer
func (d *DynSsz) marshalStructWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	currentOffset := typeDesc.Len

	// First pass: write fixed fields and collect dynamic fields
	dynamicFieldIdx := 0
	for i := range typeDesc.Fields {
		field := &typeDesc.Fields[i]
		fieldValue := value.Field(i)

		if field.Size < 0 {
			// Dynamic field - write offset
			err := writeUint32(ctx.writer, uint32(currentOffset))
			if err != nil {
				return err
			}

			// Get size from tree if available
			if currentNode := ctx.currentNode; currentNode != nil {
				if childSize, ok := ctx.getChildSize(dynamicFieldIdx); ok {
					currentOffset += childSize
				} else {
					return fmt.Errorf("dynamic field %v has missing size tree node", field.Name)
				}
			} else {
				return fmt.Errorf("marshal dynamic struct without size tree")
			}

			dynamicFieldIdx++
		} else {
			// Static field - write directly
			err := d.marshalTypeWriter(ctx, field.Type, fieldValue)
			if err != nil {
				return err
			}
		}
	}

	// Second pass: write dynamic fields
	savedNode := ctx.currentNode
	for _, field := range typeDesc.DynFields {
		fieldValue := value.Field(int(field.Field.Index))

		ctx.enterDynamicField()

		err := d.marshalTypeWriter(ctx, field.Field.Type, fieldValue)
		if err != nil {
			return err
		}

		ctx.exitDynamicField(savedNode)
	}

	return nil
}

// marshalArrayWriter marshals an array to a writer
func (d *DynSsz) marshalArrayWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	arrayLen := int(typeDesc.Len)
	elemType := typeDesc.ElemDesc

	// Handle byte arrays specially
	if elemType.Kind == reflect.Uint8 {
		// Write byte array directly
		bytes := make([]byte, arrayLen)
		for i := 0; i < arrayLen; i++ {
			bytes[i] = uint8(value.Index(i).Uint())
		}
		_, err := ctx.writer.Write(bytes)
		return err
	}

	// Handle arrays with dynamic elements
	if elemType.Size < 0 {
		if ctx.currentNode == nil {
			return fmt.Errorf("marshal dynamic array without size tree")
		}

		currentOffset := 4 * arrayLen // Space for offsets

		// First pass: write offsets
		for i := 0; i < arrayLen; i++ {
			err := writeUint32(ctx.writer, uint32(currentOffset))
			if err != nil {
				return err
			}

			// Get size from tree if available
			if childSize, ok := ctx.getChildSize(i); ok {
				currentOffset += int(childSize)
			} else {
				return fmt.Errorf("dynamic array has missing size tree node")
			}
		}

		// Second pass: write elements
		savedNode := ctx.currentNode
		for i := 0; i < arrayLen; i++ {
			ctx.enterDynamicField()

			err := d.marshalTypeWriter(ctx, elemType, value.Index(i))
			if err != nil {
				return err
			}

			ctx.exitDynamicField(savedNode)
		}
	} else {
		// Static elements - write directly
		for i := 0; i < arrayLen; i++ {
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// marshalSliceWriter marshals a slice to a writer
func (d *DynSsz) marshalSliceWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	sliceLen := value.Len()
	elemType := typeDesc.ElemDesc

	// Calculate padding for fixed-size slices
	appendZero := 0
	if len(typeDesc.SizeHints) > 0 && !typeDesc.SizeHints[0].Dynamic {
		if uint32(sliceLen) > typeDesc.SizeHints[0].Size {
			return ErrListTooBig
		}
		if uint32(sliceLen) < typeDesc.SizeHints[0].Size {
			appendZero = int(typeDesc.SizeHints[0].Size) - sliceLen
		}
	}

	// Handle byte slices specially
	if elemType.Kind == reflect.Uint8 {
		// Write byte slice directly
		bytes := make([]byte, sliceLen)
		for i := 0; i < sliceLen; i++ {
			bytes[i] = uint8(value.Index(i).Uint())
		}
		_, err := ctx.writer.Write(bytes)
		if err != nil {
			return err
		}

		// Write padding zeros
		if appendZero > 0 {
			zeros := make([]byte, appendZero)
			_, err = ctx.writer.Write(zeros)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// Handle slices with dynamic elements
	if elemType.Size < 0 {
		if ctx.currentNode == nil {
			return fmt.Errorf("marshal dynamic slice without size tree")
		}

		totalElems := sliceLen + appendZero
		currentOffset := 4 * totalElems // Space for offsets

		// First pass: write offsets
		for i := 0; i < sliceLen; i++ {
			err := writeUint32(ctx.writer, uint32(currentOffset))
			if err != nil {
				return err
			}

			// Get size from tree if available
			if childSize, ok := ctx.getChildSize(i); ok {
				currentOffset += int(childSize)
			} else {
				return fmt.Errorf("dynamic slice element %d has missing size tree node", i)
			}
		}

		// Write offsets for padding zeros
		if appendZero > 0 {
			zeroVal := reflect.New(elemType.Type).Elem()
			zeroSize, err := d.getSszValueSize(elemType, zeroVal)
			if err != nil {
				return err
			}

			for i := 0; i < appendZero; i++ {
				err := writeUint32(ctx.writer, uint32(currentOffset))
				if err != nil {
					return err
				}
				currentOffset += int(zeroSize)
			}
		}

		// Second pass: write elements
		savedNode := ctx.currentNode
		for i := 0; i < sliceLen; i++ {
			ctx.enterDynamicField()

			err := d.marshalTypeWriter(ctx, elemType, value.Index(i))
			if err != nil {
				return err
			}

			ctx.exitDynamicField(savedNode)
		}

		// Write padding zeros
		if appendZero > 0 {
			zeroVal := reflect.New(elemType.Type).Elem()
			for i := 0; i < appendZero; i++ {
				err := d.marshalTypeWriter(ctx, elemType, zeroVal)
				if err != nil {
					return err
				}
			}
		}
		ctx.currentNode = savedNode
	} else {
		// Static elements - write directly
		for i := 0; i < sliceLen; i++ {
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i))
			if err != nil {
				return err
			}
		}

		// Write padding zeros
		if appendZero > 0 {
			zeroVal := reflect.New(elemType.Type).Elem()
			for i := 0; i < appendZero; i++ {
				err := d.marshalTypeWriter(ctx, elemType, zeroVal)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// marshalStringWriter marshals a string to a writer
func (d *DynSsz) marshalStringWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	str := value.String()
	strBytes := []byte(str)

	if typeDesc.Size > 0 {
		// Fixed-size string
		if len(strBytes) > int(typeDesc.Size) {
			return fmt.Errorf("string too long for fixed size: %d > %d", len(strBytes), typeDesc.Size)
		}

		// Write string bytes
		_, err := ctx.writer.Write(strBytes)
		if err != nil {
			return err
		}

		// Pad with zeros
		padding := int(typeDesc.Size) - len(strBytes)
		if padding > 0 {
			zeros := make([]byte, padding)
			_, err = ctx.writer.Write(zeros)
			if err != nil {
				return err
			}
		}
	} else {
		// Dynamic string - write as-is
		_, err := ctx.writer.Write(strBytes)
		if err != nil {
			return err
		}
	}

	return nil
}
