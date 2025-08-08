package dynssz

import (
	"fmt"
	"reflect"
)

// marshalTypeWriter is the core streaming serialization dispatcher for writing SSZ-encoded data to an io.Writer.
//
// This function serves as the primary entry point for the streaming marshal process, determining the most
// efficient encoding path based on the type's characteristics. It automatically handles pointer dereferencing,
// fastssz delegation for compatible types, and routes to specialized streaming functions for each kind.
//
// The streaming approach is particularly beneficial for:
//   - Large data structures that would consume significant memory if buffered
//   - Network protocols requiring incremental data transmission
//   - File I/O operations where disk streaming is more efficient than bulk writes
//   - Embedded systems or environments with memory constraints
//
// Parameters:
//   - ctx: The marshal writer context containing the output writer, buffer, and size tree for dynamic fields
//   - typeDesc: The TypeDescriptor with pre-computed metadata about the type being encoded
//   - value: The reflect.Value containing the actual data to be serialized
//
// Returns:
//   - error: An error if encoding fails due to I/O issues, unsupported types, or size mismatches
//
// The function handles:
//   - Automatic nil pointer handling by creating zero values
//   - FastSSZ integration for types implementing the marshaler interface
//   - Primitive type encoding using optimized write functions
//   - Complex type delegation to specialized streaming handlers
//   - Buffer optimization for small writes to reduce I/O overhead
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

// marshalStructWriter handles streaming serialization of struct types according to SSZ specifications.
//
// This function implements the SSZ encoding rules for structures, which require careful handling
// of dynamic fields. The SSZ format mandates a two-pass approach for structs with variable-length
// fields: first writing fixed fields and offsets, then writing the actual dynamic field data.
//
// The encoding process follows these steps:
//  1. Fixed fields are written directly in order
//  2. For each dynamic field, a 4-byte offset is written indicating where its data begins
//  3. After all fixed fields and offsets, dynamic field data is written sequentially
//
// Parameters:
//   - ctx: The marshal context containing the writer, buffer, and dynamic size tree
//   - typeDesc: Pre-computed struct metadata including field descriptors and offsets
//   - value: The reflect.Value of the struct instance to serialize
//
// Returns:
//   - error: An error if writing fails or if the size tree is missing for dynamic fields
//
// The function requires a pre-computed size tree (via getSszValueSizeWithTree) when the struct
// contains dynamic fields. This tree provides the exact sizes needed for offset calculation
// without requiring multiple passes over the data.
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
			ctx.writer.pushLimit(uint64(field.Size))
			err := d.marshalTypeWriter(ctx, field.Type, fieldValue)
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(field.Size) {
				return fmt.Errorf("static field %v did not write expected ssz range (written: %v, expected: %v)", field.Name, writtenBytes, field.Size)
			}
		}
	}

	// Second pass: write dynamic fields
	savedNode := ctx.currentNode
	for _, field := range typeDesc.DynFields {
		fieldValue := value.Field(int(field.Field.Index))

		ctx.enterDynamicField()
		fieldSize := ctx.currentNode.size
		ctx.writer.pushLimit(uint64(fieldSize))

		err := d.marshalTypeWriter(ctx, field.Field.Type, fieldValue)
		writtenBytes := ctx.writer.popLimit()

		if err != nil {
			return err
		}

		if writtenBytes != uint64(fieldSize) {
			return fmt.Errorf("dynamic field %v did not write expected ssz range (written: %v, expected: %v)", field.Field.Name, writtenBytes, fieldSize)
		}

		ctx.exitDynamicField(savedNode)
	}

	return nil
}

// marshalArrayWriter handles streaming serialization of array types with both static and dynamic elements.
//
// Arrays in SSZ have a fixed number of elements known at compile time. However, the elements themselves
// may be either static (fixed-size) or dynamic (variable-size). This function handles both cases with
// optimized paths for common scenarios like byte arrays.
//
// For arrays with dynamic elements, the SSZ format requires:
//  1. Writing N offsets (4 bytes each) for N elements
//  2. Writing the actual element data in sequence
//
// Special optimizations:
//   - Byte arrays ([N]uint8) are written directly as a single write operation
//   - Static element arrays use sequential writes without offset calculation
//
// Parameters:
//   - ctx: The marshal context with writer, buffer, and size tree for dynamic elements
//   - typeDesc: Array type metadata including length and element type descriptor
//   - value: The reflect.Value of the array to serialize
//
// Returns:
//   - error: An error if writing fails or size tree is missing for dynamic elements
//
// The size tree must be pre-populated for arrays with dynamic elements to ensure correct
// offset calculation during the streaming process.
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

			elementSize := ctx.currentNode.size
			ctx.writer.pushLimit(uint64(elementSize))
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i))
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elementSize) {
				return fmt.Errorf("dynamic array element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elementSize)
			}

			ctx.exitDynamicField(savedNode)
		}
	} else {
		// Static elements - write directly
		for i := 0; i < arrayLen; i++ {
			elementSize := ctx.currentNode.size
			ctx.writer.pushLimit(uint64(elementSize))
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i))
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elementSize) {
				return fmt.Errorf("static array element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elementSize)
			}
		}
	}

	return nil
}

// marshalSliceWriter handles streaming serialization of slice types with support for dynamic sizing and padding.
//
// Slices in SSZ are variable-length sequences that may have size constraints specified via tags.
// This function handles multiple slice variants including byte slices (with optimizations), slices
// with fixed-size limits (requiring padding), and slices containing dynamic elements.
//
// The encoding process handles:
//   - Dynamic slices: Similar to arrays, requiring offset tables for variable-length elements
//   - Fixed-size slices: Must be padded with zero values to reach the specified size
//   - Byte slices: Optimized with direct write operations
//   - Size validation: Ensures slices don't exceed maximum sizes
//
// For fixed-size slices (ssz-size tag), padding calculation:
//   - If slice length < required size: Zero elements are appended
//   - If slice length > required size: Returns ErrListTooBig
//   - If slice length = required size: Written as-is
//
// Parameters:
//   - ctx: The marshal context containing writer, buffer, and dynamic size tracking
//   - typeDesc: Slice metadata including element type and size constraints from tags
//   - value: The reflect.Value of the slice to serialize
//
// Returns:
//   - error: An error if writing fails, size constraints are violated, or size tree is missing
//
// The function ensures compliance with SSZ specifications for list types, including proper
// offset calculation for dynamic elements and correct padding for fixed-size lists.
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
	} else if elemType.Kind == reflect.Uint8 {
		// Write byte slice directly
		bytes := value.Bytes()
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
	} else {
		// Static elements - write directly
		for i := 0; i < sliceLen; i++ {
			ctx.writer.pushLimit(uint64(elemType.Size))
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i))
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elemType.Size) {
				return fmt.Errorf("static slice element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elemType.Size)
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

// marshalStringWriter handles streaming serialization of string types as UTF-8 encoded byte sequences.
//
// In SSZ, strings are treated as byte arrays containing UTF-8 encoded text. This function supports
// both fixed-size strings (which must be padded with zeros) and variable-size strings. The encoding
// follows the same rules as byte slices but with string-specific handling.
//
// String encoding behavior:
//   - Fixed-size strings: Padded with null bytes (0x00) to reach the specified size
//   - Variable-size strings: Written as-is without padding
//   - Size validation: Fixed-size strings cannot exceed their declared size
//
// Parameters:
//   - ctx: The marshal context containing the output writer
//   - typeDesc: String type metadata including size constraints
//   - value: The reflect.Value containing the string to serialize
//
// Returns:
//   - error: An error if the string exceeds fixed size limits or if writing fails
//
// Example:
//
//	A fixed-size string field with size 10 containing "hello" would be encoded as:
//	[0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x00, 0x00, 0x00, 0x00, 0x00]
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
