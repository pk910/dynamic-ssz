package dynssz

import (
	"fmt"
	"io"
	"reflect"
)

// unmarshalTypeReader is the core streaming deserialization dispatcher for reading SSZ-encoded data from an io.Reader.
//
// This function serves as the primary entry point for the streaming unmarshal process, routing to
// appropriate handlers based on the type's characteristics. It provides memory-efficient decoding
// by reading data incrementally from the source, making it suitable for large files, network streams,
// or any scenario where loading complete data into memory would be impractical.
//
// The streaming approach offers significant advantages:
//   - Constant memory usage regardless of input size
//   - Ability to process data as it arrives from network connections
//   - Efficient handling of large blockchain state files
//   - Support for infinite streams with proper boundaries
//
// Parameters:
//   - ctx: The unmarshal reader context containing buffers and the limitedReader for optimization
//   - typeDesc: Pre-computed type metadata for efficient decoding
//   - value: The reflect.Value where decoded data will be stored
//
// Returns:
//   - error: An error if decoding fails due to I/O issues, format errors, or type mismatches
//
// The function handles:
//   - Automatic nil pointer initialization
//   - FastSSZ integration for small static types
//   - Direct primitive type reading with proper endianness
//   - Complex type delegation to specialized streaming handlers
//   - Buffer reuse for small reads to minimize allocations
func (d *DynSsz) unmarshalTypeReader(ctx *unmarshalReaderContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	// For small static types, read into context buffer and use regular unmarshal
	if !typeDesc.HasDynamicSize && typeDesc.Size > 0 && typeDesc.Size <= int32(len(ctx.buffer)) && !d.NoStreamBuffering {
		buf := ctx.buffer[:typeDesc.Size]
		if _, err := io.ReadFull(ctx.limitedReader, buf); err != nil {
			return err
		}
		_, err := d.unmarshalType(typeDesc, value, buf, 0)
		return err
	}

	if typeDesc.IsPtr {
		if value.IsNil() {
			// Create new instance for nil pointer
			value.Set(reflect.New(typeDesc.Type.Elem()))
		}
		value = value.Elem()
	}

	// Use fastssz if available and not disabled for small structures
	if !d.NoFastSsz && typeDesc.IsFastSSZMarshaler && typeDesc.Size > 0 && typeDesc.Size <= int32(len(ctx.buffer)) {
		buf := ctx.buffer[:typeDesc.Size]
		if _, err := io.ReadFull(ctx.limitedReader, buf); err != nil {
			return err
		}
		if unmarshaler, ok := value.Addr().Interface().(fastsszUnmarshaler); ok {
			return unmarshaler.UnmarshalSSZ(buf)
		}
	}

	// Handle different types
	switch typeDesc.Kind {
	case reflect.Bool:
		val, err := readBool(ctx.limitedReader)
		if err != nil {
			return err
		}
		value.SetBool(val)
		return nil
	case reflect.Uint8:
		val, err := readUint8(ctx.limitedReader)
		if err != nil {
			return err
		}
		value.SetUint(uint64(val))
		return nil
	case reflect.Uint16:
		val, err := readUint16(ctx.limitedReader)
		if err != nil {
			return err
		}
		value.SetUint(uint64(val))
		return nil
	case reflect.Uint32:
		val, err := readUint32(ctx.limitedReader)
		if err != nil {
			return err
		}
		value.SetUint(uint64(val))
		return nil
	case reflect.Uint64:
		val, err := readUint64(ctx.limitedReader)
		if err != nil {
			return err
		}
		value.SetUint(val)
		return nil
	case reflect.Struct:
		return d.unmarshalStructReader(ctx, typeDesc, value)
	case reflect.Array:
		return d.unmarshalArrayReader(ctx, typeDesc, value)
	case reflect.Slice:
		return d.unmarshalSliceReader(ctx, typeDesc, value)
	case reflect.String:
		return d.unmarshalStringReader(ctx, typeDesc, value)
	default:
		return fmt.Errorf("unsupported type for streaming unmarshal: %v", typeDesc.Kind)
	}
}

// unmarshalStructReader handles streaming deserialization of struct types from SSZ-encoded data.
//
// This function implements the SSZ decoding rules for structures, which use an offset-based
// encoding scheme for dynamic fields. The decoder must first read all fixed fields and offset
// values, then use these offsets to determine boundaries for variable-length field data.
//
// The decoding process follows SSZ specifications:
//  1. Read fixed fields directly in their declared order
//  2. For each dynamic field position, read a 4-byte offset
//  3. After all fixed fields, read dynamic fields using offset boundaries
//  4. Each dynamic field's size is calculated as (next_offset - current_offset)
//
// Parameters:
//   - ctx: The unmarshal context containing reusable buffers and limitedReader
//   - typeDesc: Pre-computed struct metadata with field descriptors
//   - value: The reflect.Value of the struct to populate
//
// Returns:
//   - error: An error if reading fails or data doesn't match expected format
//
// The function enforces strict boundary checking using the limit stack to ensure each
// field consumes exactly its allocated bytes according to the offset table. This prevents
// fields from reading beyond their boundaries and corrupting subsequent data.
func (d *DynSsz) unmarshalStructReader(ctx *unmarshalReaderContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	dynamicOffsets := make([]uint32, 0, len(typeDesc.DynFields))

	// First pass: read static fields and dynamic field offsets directly from stream
	for i, field := range typeDesc.Fields {
		fieldValue := value.Field(i)

		if field.Size < 0 {
			// Dynamic field - read offset directly from stream
			offset, err := readUint32(ctx.limitedReader)
			if err != nil {
				return err
			}
			dynamicOffsets = append(dynamicOffsets, offset)
		} else {
			// Static field - push limit and read directly from stream
			ctx.limitedReader.pushLimit(uint64(field.Size))

			err := d.unmarshalTypeReader(ctx, field.Type, fieldValue)

			consumedBytes := ctx.limitedReader.popLimit()
			consumedExpectedBytes := consumedBytes == uint64(field.Size)

			if err == io.EOF && consumedExpectedBytes {
				err = nil
			}
			if err != nil {
				return err
			}

			if !consumedExpectedBytes {
				return fmt.Errorf("struct field %s did not consume expected ssz range (consumed: %v, expected: %v)", field.Name, consumedBytes, field.Size)
			}
		}
	}

	// Process dynamic fields with proper boundaries
	for i, dynFieldInfo := range typeDesc.DynFields {
		fieldValue := value.Field(int(dynFieldInfo.Field.Index))

		// Calculate field size from this offset to the next offset or EOF
		var fieldSize uint64
		hasKnownSize := false
		if i+1 < len(typeDesc.DynFields) {
			// Next field starts at next offset
			fieldSize = uint64(dynamicOffsets[i+1] - dynamicOffsets[i])
			hasKnownSize = true
		}

		// Push limit for this field if size is known
		if hasKnownSize {
			ctx.limitedReader.pushLimit(fieldSize)
		}

		err := d.unmarshalTypeReader(ctx, dynFieldInfo.Field.Type, fieldValue)

		var consumedBytes uint64
		if hasKnownSize {
			consumedBytes = ctx.limitedReader.popLimit()
		}

		consumedExpectedBytes := !hasKnownSize || consumedBytes == fieldSize

		if err == io.EOF && consumedExpectedBytes {
			err = nil
		}
		if err != nil {
			return err
		}

		if hasKnownSize && !consumedExpectedBytes {
			return fmt.Errorf("struct field did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, fieldSize)
		}
	}

	return nil
}

// unmarshalArrayReader handles streaming deserialization of array types with fixed element counts.
//
// Arrays in SSZ have a compile-time known number of elements, but these elements may be either
// static (fixed-size) or dynamic (variable-size). This function efficiently handles both cases
// with specialized paths for common patterns like byte arrays.
//
// For arrays with dynamic elements, the SSZ format uses offset-based encoding:
//  1. First, N offsets (4 bytes each) are read for N elements
//  2. Each offset indicates where that element's data begins
//  3. Element sizes are calculated from consecutive offset differences
//  4. The last element extends to the end of the available data
//
// Special optimizations:
//   - Byte arrays ([N]uint8) use direct byte-by-byte reading
//   - Static element arrays read elements sequentially without offsets
//   - Offset tables are buffered when they fit in the context buffer
//
// Parameters:
//   - ctx: The unmarshal context with reusable buffers and limitedReader for efficiency
//   - typeDesc: Array metadata including length and element type
//   - value: The reflect.Value of the array to populate
//
// Returns:
//   - error: An error if reading fails or offsets are invalid
//
// The function ensures each element is read within its exact boundaries using the limit
// stack, preventing any element from consuming data belonging to subsequent elements.
func (d *DynSsz) unmarshalArrayReader(ctx *unmarshalReaderContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	arrayLen := int(typeDesc.Len)
	elemType := typeDesc.ElemDesc

	// Handle arrays with dynamic elements
	if elemType.Size < 0 {
		// Dynamic array - read offsets first
		offsetCount := arrayLen
		offsetBytes := offsetCount * 4

		if offsetBytes > len(ctx.buffer) {
			// append missing bytes to buffer
			ctx.buffer = append(ctx.buffer, make([]byte, offsetBytes-len(ctx.buffer))...)
		}

		// Read all offsets into context buffer
		offsets := ctx.buffer[:offsetBytes]
		if _, err := io.ReadFull(ctx.limitedReader, offsets); err != nil {
			return err
		}

		// Extract offsets from buffer and process elements with boundaries
		offsetList := make([]uint32, arrayLen)
		for i := 0; i < arrayLen; i++ {
			offsetList[i] = unmarshallUint32(offsets[i*4 : (i+1)*4])
		}

		// Process elements in order with proper boundaries
		for i := 0; i < arrayLen; i++ {
			elemValue := value.Index(i)

			// Calculate element size from this offset to next offset or EOF
			var elemSize uint64
			hasKnownSize := false
			if i+1 < arrayLen {
				elemSize = uint64(offsetList[i+1] - offsetList[i])
				hasKnownSize = true
			}
			// Last element reads until EOF (handled by parent limit)

			// Push limit for this element if size is known
			if hasKnownSize {
				ctx.limitedReader.pushLimit(elemSize)
			}

			err := d.unmarshalTypeReader(ctx, elemType, elemValue)

			var consumedBytes uint64
			if hasKnownSize {
				consumedBytes = ctx.limitedReader.popLimit()
			}

			if err != nil {
				return err
			}

			if hasKnownSize && consumedBytes != elemSize {
				return fmt.Errorf("array element did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, elemSize)
			}
		}
	} else if elemType.Kind == reflect.Uint8 {
		// Read directly from stream
		bytes := value.Bytes()
		if _, err := io.ReadFull(ctx.limitedReader, bytes); err != nil {
			return err
		}
		return nil
	} else {
		// Static elements - read each element directly from stream
		for i := 0; i < arrayLen; i++ {
			elemValue := value.Index(i)
			err := d.unmarshalTypeReader(ctx, elemType, elemValue)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// unmarshalSliceReader handles streaming deserialization of slice types with dynamic length determination.
//
// Unlike arrays, slices in SSZ have variable length that must be determined during decoding.
// For slices with dynamic elements, the length is inferred from the offset table structure.
// For slices with static elements, the length is determined by reading until EOF.
//
// The decoding strategy varies by element type:
//   - Dynamic elements: First offset divided by 4 gives the element count
//   - Static elements: Read elements sequentially until no more data
//   - Byte slices: Optimized path reading all remaining bytes at once
//
// For dynamic element slices, the process involves:
//  1. Read first offset to determine total element count (offset / 4)
//  2. Read remaining offsets to build complete offset table
//  3. Use offset boundaries to read each element's data
//  4. Allocate and populate the slice with decoded elements
//
// Parameters:
//   - ctx: The unmarshal context containing optimization buffers and limitedReader
//   - typeDesc: Slice metadata including element type descriptor
//   - value: The reflect.Value where the decoded slice will be set
//
// Returns:
//   - error: An error if reading fails, offsets are invalid, or allocation fails
//
// The function handles memory allocation efficiently, creating slices with exact capacity
// to avoid reallocation during element population. For byte slices, it uses chunked
// reading to handle potentially large data without excessive memory usage.
func (d *DynSsz) unmarshalSliceReader(ctx *unmarshalReaderContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	elemType := typeDesc.ElemDesc

	// For dynamic slices, we need to determine the number of elements from offsets
	if elemType.Size < 0 {
		// Read first offset to determine element count
		firstOffset, err := readUint32(ctx.limitedReader)
		if err != nil {
			return err
		}

		if firstOffset%4 != 0 {
			return fmt.Errorf("invalid first offset: %d is not divisible by 4", firstOffset)
		}

		// Read remaining offsets into context buffer if they fit
		elemCount := int(firstOffset / 4)
		remainingOffsetBytes := (elemCount - 1) * 4
		if remainingOffsetBytes > len(ctx.buffer) {
			// append missing bytes to buffer
			ctx.buffer = append(ctx.buffer, make([]byte, remainingOffsetBytes-len(ctx.buffer))...)
		}

		offsetList := make([]uint32, elemCount)
		offsetList[0] = firstOffset

		if remainingOffsetBytes > 0 {
			offsetBuf := ctx.buffer[:remainingOffsetBytes]
			if _, err := io.ReadFull(ctx.limitedReader, offsetBuf); err != nil {
				return err
			}
			// Extract remaining offsets
			for i := 1; i < elemCount; i++ {
				offsetList[i] = unmarshallUint32(offsetBuf[(i-1)*4 : i*4])
			}
		}

		// Create slice
		sliceValue := reflect.MakeSlice(value.Type(), elemCount, elemCount)

		// Read elements in order with proper boundaries
		for i := 0; i < elemCount; i++ {
			elemValue := sliceValue.Index(i)

			// Calculate element size from this offset to next offset or EOF
			var elemSize uint64
			hasKnownSize := false
			if i+1 < elemCount {
				elemSize = uint64(offsetList[i+1] - offsetList[i])
				hasKnownSize = true
			}
			// Last element reads until EOF (handled by parent limit)

			// Push limit for this element if size is known
			if hasKnownSize {
				ctx.limitedReader.pushLimit(elemSize)
			}

			err := d.unmarshalTypeReader(ctx, elemType, elemValue)

			var consumedBytes uint64
			if hasKnownSize {
				consumedBytes = ctx.limitedReader.popLimit()
			}

			consumedExpectedBytes := !hasKnownSize || consumedBytes == elemSize

			if err == io.EOF && consumedExpectedBytes {
				err = nil
			}
			if err != nil {
				return err
			}

			if hasKnownSize && !consumedExpectedBytes {
				return fmt.Errorf("dynamic slice element did not consume expected ssz range (consumed: %v, expected: %v)", consumedBytes, elemSize)
			}
		}

		value.Set(sliceValue)
	} else if elemType.Kind == reflect.Uint8 { // Handle byte slices specially - read entire remaining stream
		// Read all available bytes from the reader
		var bytes []byte

		// Try to read in chunks using context buffer
		for {
			n, err := ctx.limitedReader.Read(ctx.buffer)
			if n > 0 {
				bytes = append(bytes, ctx.buffer[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if bytesRemaining, hasLimit := ctx.limitedReader.bytesRemaining(); hasLimit && bytesRemaining == 0 {
				break
			}
		}

		value.SetBytes(bytes)

		if typeDesc.Size > 0 {
			if len(bytes) != int(typeDesc.Size) {
				return fmt.Errorf("byte slice did not consume expected ssz range (consumed: %v, expected: %v)", len(bytes), typeDesc.Size)
			}

			return nil
		}

		return io.EOF
	} else {
		// Static elements
		capacity := 16
		if remaining, hasLimit := ctx.limitedReader.bytesRemaining(); hasLimit {
			capacity = int(remaining / uint64(elemType.Size))
			if remaining%uint64(elemType.Size) != 0 {
				return fmt.Errorf("slice element size is not a multiple of the remaining bytes")
			}
		}

		sliceValue := reflect.MakeSlice(value.Type(), 0, capacity) // Start with small capacity

		for {
			// Check if there's enough data for another element
			remaining, hasLimit := ctx.limitedReader.bytesRemaining()
			if hasLimit && remaining < uint64(elemType.Size) {
				break
			}

			// Grow slice by one element
			elemValue := reflect.New(elemType.Type).Elem()

			// Push limit for this element
			ctx.limitedReader.pushLimit(uint64(elemType.Size))

			err := d.unmarshalTypeReader(ctx, elemType, elemValue)
			actualConsumed := ctx.limitedReader.popLimit()

			if actualConsumed == 0 {
				break
			}
			if (err == io.EOF || err == nil) && uint64(elemType.Size) != actualConsumed {
				return fmt.Errorf("slice element did not consume expected ssz range (consumed: %v, expected: %v)", actualConsumed, elemType.Size)
			}
			if err != nil {
				return err
			}

			sliceValue = reflect.Append(sliceValue, elemValue)
		}

		value.Set(sliceValue)
		return io.EOF
	}

	return nil
}

// unmarshalStringReader handles streaming deserialization of string types from UTF-8 encoded bytes.
//
// In SSZ, strings are encoded as UTF-8 byte sequences. This function reads these bytes and
// converts them back to Go strings. For fixed-size strings, null padding bytes are automatically
// trimmed during decoding.
//
// String decoding behavior:
//   - Variable-size strings: Read all available bytes as UTF-8 text
//   - Fixed-size strings: Read exact size, then trim trailing null bytes
//   - Chunked reading: Uses context buffer for memory-efficient processing
//
// The function handles large strings efficiently by reading in chunks rather than
// allocating a single large buffer upfront. This approach is particularly beneficial
// when processing strings from network streams or large files.
//
// Parameters:
//   - ctx: The unmarshal context with reusable buffer and limitedReader for chunked reading
//   - typeDesc: String metadata including size constraints
//   - value: The reflect.Value where the decoded string will be set
//
// Returns:
//   - error: An error if reading fails or size constraints are violated
//
// For fixed-size strings, the function ensures exactly the specified number of bytes
// are consumed from the reader, even if the actual string content is shorter (due to
// null padding). This maintains proper alignment for subsequent fields.
func (d *DynSsz) unmarshalStringReader(ctx *unmarshalReaderContext, typeDesc *TypeDescriptor, value reflect.Value) error {
	// Read all available bytes from the reader for the string
	var bytes []byte

	// Try to read in chunks using context buffer
	for {
		n, err := ctx.limitedReader.Read(ctx.buffer)
		if n > 0 {
			bytes = append(bytes, ctx.buffer[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// For fixed-size strings, trim null bytes
	if typeDesc.Size > 0 {
		if len(bytes) != int(typeDesc.Size) {
			return fmt.Errorf("string did not consume expected ssz range (consumed: %v, expected: %v)", len(bytes), typeDesc.Size)
		}

		// Find the first null byte
		for i, b := range bytes {
			if b == 0 {
				bytes = bytes[:i]
				break
			}
		}
	}

	value.SetString(string(bytes))

	return nil
}
