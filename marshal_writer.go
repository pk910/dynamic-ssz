package dynssz

import (
	"fmt"
	"reflect"
	"strings"
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
func (d *DynSsz) marshalTypeWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value, idt int) error {

	// For small static types or when buffer can hold entire result, use regular marshal
	if !typeDesc.HasDynamicSize && typeDesc.Size > 0 && typeDesc.Size <= uint32(cap(ctx.buffer)) && !d.NoStreamBuffering {
		buf := ctx.buffer[:0]
		result, err := d.marshalType(typeDesc, value, buf, idt)
		if err != nil {
			return err
		}
		_, err = ctx.writer.Write(result)
		return err
	}

	if typeDesc.IsPtr {
		if value.IsNil() {
			// Handle nil pointers by creating zero value
			value = reflect.New(typeDesc.Type.Elem())
		}
		value = value.Elem()
	}

	useFastSsz := !d.NoFastSsz && typeDesc.HasFastSSZMarshaler && !typeDesc.HasDynamicSize
	if !useFastSsz && typeDesc.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v)\n", strings.Repeat(" ", idt), typeDesc.Type.Name(), typeDesc.Kind, useFastSsz, typeDesc.HasFastSSZMarshaler, typeDesc.HasDynamicSize)
	}

	if useFastSsz {
		if marshaler, ok := value.Addr().Interface().(fastsszMarshaler); ok {
			// Use buffer for fastssz marshal
			buf := ctx.buffer[:0]
			result, err := marshaler.MarshalSSZTo(buf)
			if err != nil {
				return err
			}
			_, err = ctx.writer.Write(result)
			return err
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz {
		// can't use fastssz, use dynamic marshaling

		switch typeDesc.SszType {
		// Handle complex types
		case SszContainerType:
			return d.marshalStructWriter(ctx, typeDesc, value, idt)
		case SszVectorType, SszBitvectorType, SszUint128Type, SszUint256Type:
			return d.marshalVectorWriter(ctx, typeDesc, value, idt)
		case SszListType, SszBitlistType:
			return d.marshalListWriter(ctx, typeDesc, value, idt)

		// Handle primitive types
		case SszBoolType:
			return writeBool(ctx.writer, value.Bool())
		case SszUint8Type:
			return writeUint8(ctx.writer, uint8(value.Uint()))
		case SszUint16Type:
			return writeUint16(ctx.writer, uint16(value.Uint()))
		case SszUint32Type:
			return writeUint32(ctx.writer, uint32(value.Uint()))
		case SszUint64Type:
			return writeUint64(ctx.writer, value.Uint())
		default:
			return fmt.Errorf("unsupported type for streaming marshal: %v", typeDesc.Kind)
		}
	}

	return nil
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
func (d *DynSsz) marshalStructWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value, idt int) error {
	currentOffset := typeDesc.Len

	// First pass: write fixed fields and collect dynamic fields
	dynamicFieldIdx := 0
	for i := range typeDesc.ContainerDesc.Fields {
		field := &typeDesc.ContainerDesc.Fields[i]
		fieldValue := value.Field(i)

		if field.Type.IsDynamic {
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
			ctx.writer.pushLimit(uint64(field.Type.Size))
			err := d.marshalTypeWriter(ctx, field.Type, fieldValue, idt+2)
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(field.Type.Size) {
				return fmt.Errorf("static field %v did not write expected ssz range (written: %v, expected: %v)", field.Name, writtenBytes, field.Type.Size)
			}
		}
	}

	// Second pass: write dynamic fields
	savedNode := ctx.currentNode
	for _, field := range typeDesc.ContainerDesc.DynFields {
		fieldValue := value.Field(int(field.Index))

		ctx.enterDynamicField()
		fieldSize := ctx.currentNode.size
		ctx.writer.pushLimit(uint64(fieldSize))

		err := d.marshalTypeWriter(ctx, field.Field.Type, fieldValue, idt+2)
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

// marshalVectorWriter handles streaming serialization of vector types with both static and dynamic elements.
//
// Vectors in SSZ have a fixed number of elements known at compile time. However, the elements themselves
// may be either static (fixed-size) or dynamic (variable-size). This function handles both cases with
// optimized paths for common scenarios like byte arrays.
//
// For vectors with dynamic elements, the SSZ format requires:
//  1. Writing N offsets (4 bytes each) for N elements
//  2. Writing the actual element data in sequence
//
// Special optimizations:
//   - Byte arrays ([N]uint8) are written directly as a single write operation
//   - Static element vectors use sequential writes without offset calculation
//
// Parameters:
//   - ctx: The marshal context with writer, buffer, and size tree for dynamic elements
//   - typeDesc: Vector type metadata including length and element type descriptor
//   - value: The reflect.Value of the vector to serialize
//
// Returns:
//   - error: An error if writing fails or size tree is missing for dynamic elements
//
// The size tree must be pre-populated for vectors with dynamic elements to ensure correct
// offset calculation during the streaming process.
func (d *DynSsz) marshalVectorWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value, idt int) error {
	elemType := typeDesc.ElemDesc

	arrayLen := value.Len()
	if uint32(arrayLen) > typeDesc.Len {
		return ErrListTooBig
	}

	appendZero := 0
	if uint32(arrayLen) < typeDesc.Len {
		appendZero = int(typeDesc.Len) - arrayLen
	}

	if typeDesc.IsByteArray || typeDesc.IsString {
		// shortcut for performance: use append on []byte arrays
		if !value.CanAddr() {
			// workaround for unaddressable static arrays
			valPtr := reflect.New(typeDesc.Type)
			valPtr.Elem().Set(value)
			value = valPtr.Elem()
		}

		var bytes []byte
		if typeDesc.IsString {
			bytes = []byte(value.String())
		} else {
			bytes = value.Bytes()
		}

		if appendZero > 0 {
			zeroBytes := make([]uint8, appendZero)
			bytes = append(bytes, zeroBytes...)
		}

		_, err := ctx.writer.Write(bytes)
		return err
	}

	// Handle arrays with dynamic elements
	if elemType.IsDynamic {
		if ctx.currentNode == nil {
			return fmt.Errorf("marshal dynamic vector without size tree")
		}

		currentOffset := 4 * (arrayLen + appendZero) // Space for offsets

		// First pass: write offsets
		for i := 0; i < arrayLen+appendZero; i++ {
			err := writeUint32(ctx.writer, uint32(currentOffset))
			if err != nil {
				return err
			}

			// Get size from tree if available
			if childSize, ok := ctx.getChildSize(i); ok {
				currentOffset += int(childSize)
			} else {
				return fmt.Errorf("dynamic vector has missing size tree node")
			}
		}

		// Second pass: write elements
		savedNode := ctx.currentNode
		for i := 0; i < arrayLen; i++ {
			ctx.enterDynamicField()

			elementSize := ctx.currentNode.size
			ctx.writer.pushLimit(uint64(elementSize))
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i), idt+2)
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elementSize) {
				return fmt.Errorf("dynamic vector element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elementSize)
			}

			ctx.exitDynamicField(savedNode)
		}

		var zeroVal reflect.Value

		if appendZero > 0 {
			if elemType.IsPtr {
				zeroVal = reflect.New(elemType.Type.Elem())
			} else {
				zeroVal = reflect.New(elemType.Type).Elem()
			}
		}

		for i := 0; i < appendZero; i++ {
			ctx.enterDynamicField()

			elementSize := ctx.currentNode.size
			ctx.writer.pushLimit(uint64(elementSize))
			err := d.marshalTypeWriter(ctx, elemType, zeroVal, idt+2)
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elementSize) {
				return fmt.Errorf("dynamic vector element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elementSize)
			}

			ctx.exitDynamicField(savedNode)
		}
	} else {
		// Static elements - write directly
		for i := 0; i < arrayLen; i++ {
			ctx.writer.pushLimit(uint64(elemType.Size))
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i), idt+2)
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elemType.Size) {
				return fmt.Errorf("static vector element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elemType.Size)
			}
		}

		var zeroVal reflect.Value

		if appendZero > 0 {
			if elemType.IsPtr {
				zeroVal = reflect.New(elemType.Type.Elem())
			} else {
				zeroVal = reflect.New(elemType.Type).Elem()
			}
		}

		for i := 0; i < appendZero; i++ {
			ctx.writer.pushLimit(uint64(elemType.Size))
			err := d.marshalTypeWriter(ctx, elemType, zeroVal, idt+2)
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elemType.Size) {
				return fmt.Errorf("static vector padding element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elemType.Size)
			}
		}
	}

	return nil
}

// marshalListWriter handles streaming serialization of slice types with support for dynamic sizing and padding.
//
// Lists in SSZ are variable-length sequences that may have size constraints specified via tags.
// This function handles multiple list variants including byte slices (with optimizations), lists
// with fixed-size limits (requiring padding), and slices containing dynamic elements.
//
// The encoding process handles:
//   - Dynamic lists: Similar to vectors, requiring offset tables for variable-length elements
//   - Fixed-size lists: Must be padded with zero values to reach the specified size
//   - Byte slices: Optimized with direct write operations
//   - Size validation: Ensures lists don't exceed maximum sizes
//
// For fixed-size lists (ssz-size tag), padding calculation:
//   - If list length < required size: Zero elements are appended
//   - If list length > required size: Returns ErrListTooBig
//   - If list length = required size: Written as-is
//
// Parameters:
//   - ctx: The marshal context containing writer, buffer, and dynamic size tracking
//   - typeDesc: List metadata including element type and size constraints from tags
//   - value: The reflect.Value of the list to serialize
//
// Returns:
//   - error: An error if writing fails, size constraints are violated, or size tree is missing
//
// The function ensures compliance with SSZ specifications for list types, including proper
// offset calculation for dynamic elements and correct padding for fixed-size lists.
func (d *DynSsz) marshalListWriter(ctx *marshalWriterContext, typeDesc *TypeDescriptor, value reflect.Value, idt int) error {
	sliceLen := value.Len()
	elemType := typeDesc.ElemDesc

	// Handle lists with dynamic elements
	if elemType.IsDynamic {
		if ctx.currentNode == nil {
			return fmt.Errorf("marshal dynamic list without size tree")
		}

		totalElems := sliceLen
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

		// Second pass: write elements
		savedNode := ctx.currentNode
		for i := 0; i < sliceLen; i++ {
			ctx.enterDynamicField()

			err := d.marshalTypeWriter(ctx, elemType, value.Index(i), idt+2)
			if err != nil {
				return err
			}

			ctx.exitDynamicField(savedNode)
		}

		ctx.currentNode = savedNode
	} else if elemType.IsByteArray {
		// Write byte slice directly
		var bytes []byte
		if typeDesc.IsString {
			bytes = []byte(value.String())
		} else {
			bytes = value.Bytes()
		}

		_, err := ctx.writer.Write(bytes)
		if err != nil {
			return err
		}

		return nil
	} else {
		// Static elements - write directly
		for i := 0; i < sliceLen; i++ {
			ctx.writer.pushLimit(uint64(elemType.Size))
			err := d.marshalTypeWriter(ctx, elemType, value.Index(i), idt+2)
			writtenBytes := ctx.writer.popLimit()

			if err != nil {
				return err
			}

			if writtenBytes != uint64(elemType.Size) {
				return fmt.Errorf("static slice element %d did not write expected ssz range (written: %v, expected: %v)", i, writtenBytes, elemType.Size)
			}
		}
	}

	return nil
}
