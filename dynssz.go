// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package dynssz provides dynamic SSZ encoding and decoding with runtime reflection support.
package dynssz

import (
	"fmt"
	"io"
	"reflect"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/reflection"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
	"github.com/pk910/dynamic-ssz/treeproof"
)

// DynSsz is a dynamic SSZ encoder/decoder that uses runtime reflection to handle dynamic field sizes.
// It provides flexible SSZ encoding/decoding for any Go data structures that can adapt to different
// specifications through dynamic field sizing. While commonly used with Ethereum data structures
// and presets (mainnet, minimal, custom), it works with any SSZ-compatible types.
//
// The instance maintains caches for type descriptors and specification values to optimize performance.
// It's recommended to reuse the same DynSsz instance across operations to benefit from caching.
//
// Key features:
//   - Hybrid approach: automatically uses fastssz for static types, reflection for dynamic types
//   - Type caching: reduces overhead for repeated operations on the same types
//   - Specification support: handles dynamic field sizes based on runtime specifications
//   - Thread-safe: can be safely used from multiple goroutines
//
// Example usage:
//
//	specs := map[string]any{
//	    "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
//	    "SYNC_COMMITTEE_SIZE":       uint64(512),
//	}
//	ds := dynssz.NewDynSsz(specs)
//
//	// Marshal
//	data, err := ds.MarshalSSZ(myStruct)
//
//	// Unmarshal
//	err = ds.UnmarshalSSZ(&myStruct, data)
//
//	// Hash tree root
//	root, err := ds.HashTreeRoot(myStruct)
type DynSsz struct {
	typeCache      *ssztypes.TypeCache         // Cache for type descriptors
	specValues     map[string]any              // Dynamic specification values
	specValueCache map[string]*cachedSpecValue // Cache for parsed specification expressions
	options        *DynSszOptions
}

// NewDynSsz creates a new instance of the DynSsz encoder/decoder.
//
// The specs map contains dynamic properties and configurations that control SSZ serialization
// and deserialization. These specifications allow the library to handle different configurations
// by defining dynamic field sizes at runtime. While commonly used with Ethereum presets
// (mainnet, minimal, custom), they can define any dynamic sizing parameters for your data structures.
//
// For non-Ethereum use cases, you can define any specifications relevant to your data structures.
//
// The library supports mathematical expressions in dynssz-size tags that reference these
// specification values, enabling complex dynamic sizing behavior.
//
// Parameters:
//   - specs: A map of specification names to their values. Can be nil for default behavior.
//
// Returns:
//   - *DynSsz: A new DynSsz instance ready for encoding/decoding operations
//
// Example:
//
//	// Ethereum mainnet specifications
//	specs := map[string]any{
//	    "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
//	    "SYNC_COMMITTEE_SIZE":       uint64(512),
//	}
//	ds := dynssz.NewDynSsz(specs)
//
//	// Custom application specifications
//	customSpecs := map[string]any{
//	    "MAX_ITEMS":           uint64(1000),
//	    "BUFFER_SIZE":         uint64(4096),
//	    "CUSTOM_ARRAY_LENGTH": uint64(256),
//	}
//	dsCustom := dynssz.NewDynSsz(customSpecs)
func NewDynSsz(specs map[string]any, options ...DynSszOption) *DynSsz {
	if specs == nil {
		specs = map[string]any{}
	}

	opts := &DynSszOptions{
		LogCb: func(format string, args ...any) {
			fmt.Printf(format, args...)
		},
	}

	for _, option := range options {
		option(opts)
	}

	dynssz := &DynSsz{
		specValues:     specs,
		specValueCache: map[string]*cachedSpecValue{},
		options:        opts,
	}
	dynssz.typeCache = ssztypes.NewTypeCache(dynssz)

	return dynssz
}

// GetTypeCache returns the type cache for the DynSsz instance.
//
// The type cache stores computed type descriptors for types used in encoding/decoding operations.
// Type descriptors contain optimized information about how to serialize/deserialize specific types,
// including field offsets, size information, and whether fastssz can be used.
//
// This method is primarily useful for debugging, performance analysis, or advanced use cases
// where you need to inspect or manage the cached type information.
//
// Returns:
//   - *TypeCache: The type cache instance containing all cached type descriptors
//
// Example:
//
//	ds := dynssz.NewDynSsz(specs)
//	cache := ds.GetTypeCache()
//
//	// Inspect cached types
//	types := cache.GetAllTypes()
//	fmt.Printf("Cache contains %d types\n", len(types))
func (d *DynSsz) GetTypeCache() *ssztypes.TypeCache {
	return d.typeCache
}

// resolveSchemaType determines the schema type to use for SSZ operations.
//
// This method implements view descriptor selection:
//   - If callConfig has a viewDescriptor, use that type
//   - Otherwise, default to the runtime type (schema == runtime)
//
// Parameters:
//   - runtimeType: The reflect.Type of the actual data (where data lives)
//   - cfg: The call configuration containing optional viewDescriptor
//
// Returns:
//   - reflect.Type: The schema type to use for SSZ layout definition
func (d *DynSsz) resolveSchemaType(runtimeType reflect.Type, cfg *callConfig) reflect.Type {
	// Check for explicit view descriptor from call options
	if cfg != nil && cfg.viewDescriptor != nil {
		viewType := reflect.TypeOf(cfg.viewDescriptor)
		// Handle both pointer and value forms
		if viewType.Kind() == reflect.Ptr && runtimeType.Kind() != reflect.Ptr {
			viewType = viewType.Elem()
		} else if viewType.Kind() != reflect.Ptr && runtimeType.Kind() == reflect.Ptr {
			viewType = reflect.New(viewType).Type()
		}
		return viewType
	}

	// Default to runtime type (schema == runtime)
	return runtimeType
}

// MarshalSSZ serializes the given source into its SSZ (Simple Serialize) representation.
//
// This method dynamically handles the serialization of Go types to SSZ format, supporting both
// static and dynamic field sizes. For types without dynamic specifications, it automatically
// uses fastssz for optimal performance. For types with dynamic field sizes (based on runtime
// specifications), it uses reflection-based processing.
//
// The method allocates a new byte slice for the result. For high-performance scenarios with
// frequent allocations, consider using MarshalSSZTo with a pre-allocated buffer.
//
// Parameters:
//   - source: Any Go value to be serialized. Must be a type supported by SSZ encoding.
//
// Returns:
//   - []byte: The SSZ-encoded data as a new byte slice
//   - error: An error if serialization fails due to unsupported types, encoding errors, or size mismatches
//
// Supported types include:
//   - Basic types: bool, uint8, uint16, uint32, uint64
//   - Arrays and slices of supported types
//   - Structs with appropriate SSZ tags
//   - Pointers to supported types
//   - Types implementing fastssz.Marshaler interface
//
// Example:
//
//	header := &phase0.BeaconBlockHeader{
//	    Slot:          12345,
//	    ProposerIndex: 42,
//	    // ... other fields
//	}
//
//	data, err := ds.MarshalSSZ(header)
//	if err != nil {
//	    log.Fatal("Failed to marshal:", err)
//	}
//	fmt.Printf("Encoded %d bytes\n", len(data))
func (d *DynSsz) MarshalSSZ(source any, opts ...CallOption) ([]byte, error) {
	cfg := applyCallOptions(opts)

	// Skip view descriptor logic for types implementing DynamicMarshaler (they handle their own serialization)
	if cfg.viewDescriptor == nil {
		if marshaler, ok := source.(sszutils.DynamicMarshaler); ok {
			var buf []byte
			if sizer, ok := source.(sszutils.DynamicSizer); ok {
				size := sizer.SizeSSZDyn(d)
				buf = make([]byte, 0, size)
			} else {
				buf = make([]byte, 0, 1024)
			}
			return marshaler.MarshalSSZDyn(d, buf)
		}
	} else if viewMarshaler, ok := source.(sszutils.DynamicViewMarshaler); ok {
		if marshalFn := viewMarshaler.MarshalSSZDynView(cfg.viewDescriptor); marshalFn != nil {
			var buf []byte
			if sizer, ok := source.(sszutils.DynamicViewSizer); ok {
				sizeFn := sizer.SizeSSZDynView(cfg.viewDescriptor)
				if sizeFn != nil {
					size := sizeFn(d)
					buf = make([]byte, 0, size)
				} else {
					buf = make([]byte, 0, 1024)
				}
			} else {
				buf = make([]byte, 0, 1024)
			}
			return marshalFn(d, buf)
		}
	}

	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	// Resolve schema type from view descriptor option
	schemaType := d.resolveSchemaType(sourceType, cfg)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptorWithSchema(sourceType, schemaType, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	ctx := reflection.NewReflectionCtx(d, d.options.LogCb, d.options.Verbose, d.options.NoFastSsz)

	size, err := ctx.SizeSSZ(sourceTypeDesc, sourceValue)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 0, size)
	encoder := sszutils.NewBufferEncoder(buf)
	err = ctx.MarshalSSZ(sourceTypeDesc, sourceValue, encoder)
	if err != nil {
		return nil, err
	}

	newBuf := encoder.GetBuffer()
	if uint32(len(newBuf)) != size {
		return nil, fmt.Errorf("ssz length does not match expected length (expected: %v, got: %v)", size, len(newBuf))
	}

	return newBuf, nil
}

// MarshalSSZTo serializes the given source into its SSZ (Simple Serialize) representation and writes the output to the provided buffer.
//
// This method provides direct control over the output buffer, enabling performance optimizations such as buffer reuse
// across multiple serialization operations. Like MarshalSSZ, it dynamically handles serialization for types with both
// static and dynamic field sizes, automatically using fastssz when possible for optimal performance.
//
// The method appends the serialized data to the provided buffer, which allows for efficient concatenation of multiple
// serialized objects without additional allocations.
//
// Parameters:
//   - source: Any Go value to be serialized. Must be a type supported by SSZ encoding.
//   - buf: Pre-allocated byte slice where the serialized data will be appended. Can be nil or empty.
//
// Returns:
//   - []byte: The updated buffer containing the original data plus the newly serialized data
//   - error: An error if serialization fails due to unsupported types, encoding errors, or size mismatches
//
// Example:
//
//	buf := make([]byte, 0, 1024) // Pre-allocate with expected capacity
//	for _, block := range blocks {
//	    buf, err = ds.MarshalSSZTo(block, buf)
//	    if err != nil {
//	        log.Fatal("Failed to marshal block:", err)
//	    }
//	}
//	fmt.Printf("Serialized %d blocks into %d bytes\n", len(blocks), len(buf))
func (d *DynSsz) MarshalSSZTo(source any, buf []byte, opts ...CallOption) ([]byte, error) {
	cfg := applyCallOptions(opts)

	// Skip view descriptor logic for types implementing DynamicMarshaler
	if cfg.viewDescriptor == nil {
		if marshaler, ok := source.(sszutils.DynamicMarshaler); ok {
			return marshaler.MarshalSSZDyn(d, buf)
		}
	} else if viewMarshaler, ok := source.(sszutils.DynamicViewMarshaler); ok {
		if marshalFn := viewMarshaler.MarshalSSZDynView(cfg.viewDescriptor); marshalFn != nil {
			return marshalFn(d, buf)
		}
	}

	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	// Resolve schema type from view descriptor option
	schemaType := d.resolveSchemaType(sourceType, cfg)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptorWithSchema(sourceType, schemaType, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	ctx := reflection.NewReflectionCtx(d, d.options.LogCb, d.options.Verbose, d.options.NoFastSsz)

	encoder := sszutils.NewBufferEncoder(buf)
	err = ctx.MarshalSSZ(sourceTypeDesc, sourceValue, encoder)
	if err != nil {
		return nil, err
	}

	return encoder.GetBuffer(), nil
}

// MarshalSSZWriter serializes the given source into its SSZ representation and writes it directly to an io.Writer.
//
// This method provides memory-efficient streaming serialization for SSZ encoding, particularly beneficial
// for large data structures that would be expensive to buffer entirely in memory. Unlike MarshalSSZ which
// returns a complete byte slice, this method writes data incrementally to the provided writer, enabling
// direct output to files, network connections, or other I/O destinations.
//
// The implementation employs several optimizations:
//   - Internal buffering (default 1KB) to reduce system call overhead for small writes
//   - Automatic delegation to regular MarshalSSZ for structures smaller than the buffer size
//   - Pre-computed dynamic size trees for efficient offset calculation in complex structures
//   - Seamless integration with fastssz for types without dynamic fields
//
// For structures with dynamic fields, the method builds a size tree during the first pass to calculate
// all necessary offsets, then streams the actual data in a second pass. This two-pass approach ensures
// correct SSZ encoding while maintaining streaming efficiency.
//
// Parameters:
//   - source: Any Go value to be serialized. Must be a type supported by SSZ encoding.
//   - w: The io.Writer destination for the SSZ-encoded output. Common writers include:
//   - os.File for file output
//   - net.Conn for network transmission
//   - bytes.Buffer for in-memory buffering
//   - Any custom io.Writer implementation
//
// Returns:
//   - error: An error if serialization fails due to:
//   - Type validation errors
//   - I/O write failures
//   - Size calculation errors for dynamic fields
//   - Unsupported type structures
//
// Example usage:
//
//	// Write directly to a file
//	file, err := os.Create("beacon_state.ssz")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer file.Close()
//
//	err = ds.MarshalSSZWriter(state, file)
//	if err != nil {
//	    log.Fatal("Failed to write state:", err)
//	}
//
//	// Stream over network
//	conn, err := net.Dial("tcp", "localhost:8080")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
//
//	err = ds.MarshalSSZWriter(block, conn)
func (d *DynSsz) MarshalSSZWriter(source any, w io.Writer, opts ...CallOption) error {
	cfg := applyCallOptions(opts)
	encoder := sszutils.NewStreamEncoder(w)

	// Skip view descriptor logic for types implementing DynamicEncoder
	if cfg.viewDescriptor == nil {
		if sszEncoder, ok := source.(sszutils.DynamicEncoder); ok {
			err := sszEncoder.MarshalSSZEncoder(d, encoder)
			if err != nil {
				return err
			}

			return encoder.GetWriteError()
		}
	} else if viewEncoder, ok := source.(sszutils.DynamicViewEncoder); ok {
		if marshalFn := viewEncoder.MarshalSSZEncoderView(cfg.viewDescriptor); marshalFn != nil {
			err := marshalFn(d, encoder)
			if err != nil {
				return err
			}
			return encoder.GetWriteError()
		}
	}

	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	// Resolve schema type from view descriptor option
	schemaType := d.resolveSchemaType(sourceType, cfg)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptorWithSchema(sourceType, schemaType, nil, nil, nil)
	if err != nil {
		return err
	}

	ctx := reflection.NewReflectionCtx(d, d.options.LogCb, d.options.Verbose, d.options.NoFastSsz)

	err = ctx.MarshalSSZ(sourceTypeDesc, sourceValue, encoder)
	if err != nil {
		return err
	}

	return encoder.GetWriteError()
}

// SizeSSZ calculates the size of the given source object when serialized using SSZ encoding.
//
// This method is useful for pre-allocating buffers with the exact size needed for serialization,
// avoiding unnecessary allocations and resizing. It dynamically evaluates the size based on the
// actual values in the source object, accurately handling variable-length fields such as slices
// and dynamic arrays.
//
// For types without dynamic fields, the size is calculated using the optimized fastssz SizeSSZ method
// when available. For types with dynamic fields, it traverses the entire structure to compute the
// exact serialized size.
//
// Parameters:
//   - source: Any Go value whose SSZ-encoded size needs to be calculated
//
// Returns:
//   - int: The exact number of bytes that would be produced by MarshalSSZ for this source
//   - error: An error if the size calculation fails due to unsupported types or invalid data
//
// Example:
//
//	state := &phase0.BeaconState{
//	    // ... populated state fields
//	}
//
//	size, err := ds.SizeSSZ(state)
//	if err != nil {
//	    log.Fatal("Failed to calculate size:", err)
//	}
//
//	// Pre-allocate buffer with exact size
//	buf := make([]byte, 0, size)
//	buf, err = ds.MarshalSSZTo(state, buf)
func (d *DynSsz) SizeSSZ(source any, opts ...CallOption) (int, error) {
	cfg := applyCallOptions(opts)

	// Skip view descriptor logic for types implementing DynamicSizer
	if cfg.viewDescriptor == nil {
		if sizer, ok := source.(sszutils.DynamicSizer); ok {
			return sizer.SizeSSZDyn(d), nil
		}
	} else if viewSizer, ok := source.(sszutils.DynamicViewSizer); ok {
		sizeFn := viewSizer.SizeSSZDynView(cfg.viewDescriptor)
		if sizeFn != nil {
			return sizeFn(d), nil
		}
	}

	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	// Resolve schema type from view descriptor option
	schemaType := d.resolveSchemaType(sourceType, cfg)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptorWithSchema(sourceType, schemaType, nil, nil, nil)
	if err != nil {
		return 0, err
	}

	ctx := reflection.NewReflectionCtx(d, d.options.LogCb, d.options.Verbose, d.options.NoFastSsz)

	size, err := ctx.SizeSSZ(sourceTypeDesc, sourceValue)
	if err != nil {
		return 0, err
	}

	return int(size), nil
}

// UnmarshalSSZ decodes the given SSZ-encoded data into the target object.
//
// This method is the counterpart to MarshalSSZ, reconstructing Go values from their SSZ representation.
// It dynamically handles decoding for types with both static and dynamic field sizes, automatically
// using fastssz for optimal performance when applicable.
//
// The target must be a pointer to a value of the appropriate type. The method will allocate memory
// for slices and initialize pointer fields as needed during decoding.
//
// Parameters:
//   - target: A pointer to the Go value where the decoded data will be stored. Must be a pointer.
//   - ssz: The SSZ-encoded data to decode
//
// Returns:
//   - error: An error if decoding fails due to:
//   - Invalid SSZ format
//   - Type mismatches between the data and target
//   - Insufficient or excess data
//   - Unsupported types
//
// The method ensures that all bytes in the ssz parameter are consumed during decoding. If there are
// leftover bytes, an error is returned indicating incomplete consumption.
//
// Example:
//
//	var header phase0.BeaconBlockHeader
//	err := ds.UnmarshalSSZ(&header, encodedData)
//	if err != nil {
//	    log.Fatal("Failed to unmarshal:", err)
//	}
//	fmt.Printf("Decoded header for slot %d\n", header.Slot)
func (d *DynSsz) UnmarshalSSZ(target any, ssz []byte, opts ...CallOption) error {
	cfg := applyCallOptions(opts)

	// Skip view descriptor logic for types implementing DynamicUnmarshaler
	if cfg.viewDescriptor == nil {
		if unmarshaler, ok := target.(sszutils.DynamicUnmarshaler); ok {
			return unmarshaler.UnmarshalSSZDyn(d, ssz)
		}
	} else if viewUnmarshaler, ok := target.(sszutils.DynamicViewUnmarshaler); ok {
		if unmarshalFn := viewUnmarshaler.UnmarshalSSZDynView(cfg.viewDescriptor); unmarshalFn != nil {
			return unmarshalFn(d, ssz)
		}
	}

	targetType := reflect.TypeOf(target)
	targetValue := reflect.ValueOf(target)

	// Resolve schema type from view descriptor option
	schemaType := d.resolveSchemaType(targetType, cfg)

	targetTypeDesc, err := d.typeCache.GetTypeDescriptorWithSchema(targetType, schemaType, nil, nil, nil)
	if err != nil {
		return err
	}

	if targetTypeDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer == 0 {
		return fmt.Errorf("target must be a pointer")
	}

	if targetValue.IsNil() {
		return fmt.Errorf("target pointer must not be nil")
	}

	ctx := reflection.NewReflectionCtx(d, d.options.LogCb, d.options.Verbose, d.options.NoFastSsz)

	decoder := sszutils.NewBufferDecoder(ssz)
	decoder.PushLimit(len(ssz))

	err = ctx.UnmarshalSSZ(targetTypeDesc, targetValue, decoder)
	if err != nil {
		return err
	}

	consumedDiff := decoder.PopLimit()
	if consumedDiff != 0 {
		return fmt.Errorf("did not consume full ssz range (diff: %v, ssz size: %v)", consumedDiff, len(ssz))
	}

	return nil
}

// UnmarshalSSZReader decodes SSZ-encoded data from an io.Reader directly into the target object.
//
// This method implements memory-efficient streaming deserialization for SSZ data, reading incrementally
// from any io.Reader source. Unlike UnmarshalSSZ which requires the complete data in memory as a byte
// slice, this method processes data in chunks, making it ideal for large files, network streams, or
// memory-constrained environments.
//
// The implementation handles SSZ's offset-based encoding for dynamic fields by:
//   - Reading offsets to determine field boundaries for variable-length data
//   - Using limited readers to enforce exact byte consumption per field
//   - Processing static fields directly from the stream
//   - Dynamically allocating slices based on discovered sizes
//
// For optimal performance with small static types (≤ buffer size), the method automatically
// reads into an internal buffer and delegates to the regular unmarshal function.
//
// Parameters:
//   - target: A pointer to the Go value where decoded data will be stored. Must be a pointer
//     to a type compatible with SSZ decoding. The method will allocate memory for slices
//     and initialize pointer fields as needed during decoding.
//   - r: An io.Reader source containing the SSZ-encoded data. Common readers include:
//   - os.File for file input
//   - net.Conn for network reception
//   - bytes.Reader for in-memory data
//   - Any custom io.Reader implementation
//   - size: The expected total size of the SSZ data in bytes.
//
// Returns:
//   - error: An error if decoding fails due to:
//   - I/O read failures
//   - Invalid SSZ format or structure
//   - Type mismatches between data and target
//   - Unexpected EOF or excess data
//   - Size constraint violations
//
// The method ensures strict compliance with SSZ specifications, validating that:
//   - All expected bytes are consumed (when size is specified)
//   - Dynamic field offsets are valid and properly ordered
//   - Field boundaries are respected
//   - No data is left unread
//
// Example usage:
//
//	// Read from file
//	file, err := os.Open("beacon_state.ssz")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer file.Close()
//
//	// Get file size for exact reading
//	info, _ := file.Stat()
//	var state phase0.BeaconState
//	err = ds.UnmarshalSSZReader(&state, file, info.Size())
//	if err != nil {
//	    log.Fatal("Failed to read state:", err)
//	}
//
//	// Read from network with unknown size
//	conn, _ := net.Dial("tcp", "localhost:8080")
//	var block phase0.BeaconBlock
//	err = ds.UnmarshalSSZReader(&block, conn, -1)
func (d *DynSsz) UnmarshalSSZReader(target any, r io.Reader, size int, opts ...CallOption) error {
	cfg := applyCallOptions(opts)
	decoder := sszutils.NewStreamDecoder(r, size)
	decoder.PushLimit(size)

	// Skip view descriptor logic for types implementing DynamicDecoder
	if cfg.viewDescriptor == nil {
		if sszDecoder, ok := target.(sszutils.DynamicDecoder); ok {
			err := sszDecoder.UnmarshalSSZDecoder(d, decoder)
			if err != nil {
				return err
			}

			consumedDiff := decoder.PopLimit()
			if consumedDiff != 0 {
				return fmt.Errorf("did not consume full ssz range (diff: %v, ssz size: %v)", consumedDiff, size)
			}

			return nil
		}
	} else if viewDecoder, ok := target.(sszutils.DynamicViewDecoder); ok {
		if unmarshalFn := viewDecoder.UnmarshalSSZDecoderView(cfg.viewDescriptor); unmarshalFn != nil {
			err := unmarshalFn(d, decoder)
			if err != nil {
				return err
			}

			consumedDiff := decoder.PopLimit()
			if consumedDiff != 0 {
				return fmt.Errorf("did not consume full ssz range (diff: %v, ssz size: %v)", consumedDiff, size)
			}

			return nil
		}
	}

	targetType := reflect.TypeOf(target)
	targetValue := reflect.ValueOf(target)

	// Resolve schema type from view descriptor option
	schemaType := d.resolveSchemaType(targetType, cfg)

	targetTypeDesc, err := d.typeCache.GetTypeDescriptorWithSchema(targetType, schemaType, nil, nil, nil)
	if err != nil {
		return err
	}

	if targetTypeDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer == 0 {
		return fmt.Errorf("target must be a pointer")
	}

	if targetValue.IsNil() {
		return fmt.Errorf("target pointer must not be nil")
	}

	ctx := reflection.NewReflectionCtx(d, d.options.LogCb, d.options.Verbose, d.options.NoFastSsz)

	err = ctx.UnmarshalSSZ(targetTypeDesc, targetValue, decoder)
	if err != nil {
		return err
	}

	consumedDiff := decoder.PopLimit()
	if consumedDiff != 0 {
		return fmt.Errorf("did not consume full ssz range (diff: %v, ssz size: %v)", consumedDiff, size)
	}

	return nil
}

// HashTreeRoot computes the hash tree root of the given source object according to SSZ specifications.
//
// The hash tree root is a cryptographic commitment to the entire data structure, used extensively
// in Ethereum's consensus layer for creating Merkle proofs and maintaining state roots. This method
// implements the SSZ hash tree root algorithm, which recursively hashes all fields and combines
// them using binary Merkle trees.
//
// For optimal performance, the method uses a hasher pool to reuse hasher instances across calls.
// When NoFastHash is false (default), it uses the optimized gohashtree implementation. For types
// without dynamic fields, it automatically delegates to fastssz's HashTreeRoot method when available.
//
// Parameters:
//   - source: Any Go value for which to compute the hash tree root
//
// Returns:
//   - [32]byte: The computed hash tree root
//   - error: An error if the computation fails due to unsupported types or hashing errors
//
// The method handles all SSZ-supported types including:
//   - Basic types (bool, uint8, uint16, uint32, uint64)
//   - Fixed-size and variable-size arrays
//   - Structs with nested fields
//   - Slices with proper limit handling
//   - Bitlists with maximum size constraints
//
// Example:
//
//	block := &phase0.BeaconBlock{
//	    Slot:          12345,
//	    ProposerIndex: 42,
//	    // ... other fields
//	}
//
//	root, err := ds.HashTreeRoot(block)
//	if err != nil {
//	    log.Fatal("Failed to compute root:", err)
//	}
//	fmt.Printf("Block root: %x\n", root)
func (d *DynSsz) HashTreeRoot(source any, opts ...CallOption) ([32]byte, error) {
	var pool *hasher.HasherPool
	if d.options.NoFastHash {
		pool = &hasher.DefaultHasherPool
	} else {
		pool = &hasher.FastHasherPool
	}

	hh := pool.Get()
	defer func() {
		pool.Put(hh)
	}()

	err := d.HashTreeRootWith(source, hh, opts...)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}

// HashTreeRootWith computes the hash tree root of the given source object according to SSZ specifications.
//
// This method is similar to HashTreeRoot, but allows for custom hasher instances to be used.
// It dynamically handles hashing for types with both static and dynamic field sizes, automatically
// using fastssz for optimal performance when applicable.
//
// Parameters:
//   - source: Any Go value for which to compute the hash tree root
//   - hh: The HashWalker instance to use for hashing
//
// Returns:
//   - error: An error if the computation fails due to unsupported types or hashing errors
//
// The method handles all SSZ-supported types including:
//   - Basic types (bool, uint8, uint16, uint32, uint64)
//   - Fixed-size and variable-size arrays
//   - Structs with nested fields
//   - Slices with proper limit handling
//   - Bitlists with maximum size constraints
//
// Example:
//
//	block := &phase0.BeaconBlock{
//	    Slot:          12345,
//	    ProposerIndex: 42,
//	    // ... other fields
//	}
//
//	hh := &hasher.Hasher{}
//	err := ds.HashTreeRootWith(block, hh)
//	if err != nil {
//	    log.Fatal("Failed to compute root:", err)
//	}
//	fmt.Printf("Block root: %x\n", hh.HashRoot())
func (d *DynSsz) HashTreeRootWith(source any, hh sszutils.HashWalker, opts ...CallOption) error {
	cfg := applyCallOptions(opts)

	// Skip view descriptor logic for types implementing DynamicHashRoot
	if cfg.viewDescriptor == nil {
		if hasher, ok := source.(sszutils.DynamicHashRoot); ok {
			err := hasher.HashTreeRootWithDyn(d, hh)
			if err != nil {
				return err
			}
			return nil
		}
	} else if viewHasher, ok := source.(sszutils.DynamicViewHashRoot); ok {
		if hashFn := viewHasher.HashTreeRootWithDynView(cfg.viewDescriptor); hashFn != nil {
			err := hashFn(d, hh)
			if err != nil {
				return err
			}
			return nil
		}
	}

	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	// Resolve schema type from view descriptor option
	schemaType := d.resolveSchemaType(sourceType, cfg)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptorWithSchema(sourceType, schemaType, nil, nil, nil)
	if err != nil {
		return err
	}

	ctx := reflection.NewReflectionCtx(d, d.options.LogCb, d.options.Verbose, d.options.NoFastSsz)

	err = ctx.HashTreeRoot(sourceTypeDesc, sourceValue, hh)
	if err != nil {
		return err
	}

	return nil
}

// GetTree builds and returns the complete Merkle tree for the given value.
//
// This method constructs a full Merkle tree representation of the SSZ-encoded structure,
// which is useful for proof generation, debugging, and understanding the internal tree structure.
// The returned tree can be used to generate Merkle proofs for any field or value within the structure.
//
// The tree construction follows the same SSZ merkleization rules as HashTreeRoot, but instead
// of returning just the root hash, it provides access to the complete tree with all intermediate
// nodes. This enables:
//   - Generating Merkle proofs for specific fields using tree.Prove(index)
//   - Debugging tree structure with tree.Show(maxDepth)
//   - Understanding how different fields map to generalized indices
//   - Analyzing the progressive vs binary tree structures
//
// Parameters:
//   - source: Any Go value to be converted to a Merkle tree. Must be SSZ-compatible.
//
// Returns:
//   - *treeproof.Node: The root node of the complete Merkle tree
//   - error: An error if tree construction fails due to unsupported types or encoding errors
//
// The returned tree supports:
//   - Navigation: Use Get(index) to fetch nodes by generalized index
//   - Proof generation: Use Prove(index) to generate Merkle proofs
//   - Debugging: Use Show(maxDepth) to visualize the tree structure
//   - Multi-proofs: Use ProveMulti(indices) for efficient batch proofs
//
// Example:
//
//	// Build tree for a beacon block
//	tree, err := ds.GetTree(beaconBlock)
//	if err != nil {
//	    log.Fatal("Failed to build tree:", err)
//	}
//
//	// Show tree structure (limited to 3 levels deep)
//	tree.Show(3)
//
//	// Generate proof for a specific field at generalized index 25
//	proof, err := tree.Prove(25)
//	if err != nil {
//	    log.Fatal("Failed to generate proof:", err)
//	}
//
//	// Verify the proof against the tree root
//	isValid, err := treeproof.VerifyProof(tree.Hash(), proof)
//
// Note: For progressive containers (with ssz-index tags), the tree structure will be
// progressive rather than binary, which affects the generalized indices of fields.
func (d *DynSsz) GetTree(source any, opts ...CallOption) (*treeproof.Node, error) {
	w := treeproof.NewWrapper()

	if err := d.HashTreeRootWith(source, w, opts...); err != nil {
		return nil, err
	}

	return w.Node(), nil
}

// ValidateType validates whether a given type is compatible with SSZ encoding/decoding.
//
// This method performs a comprehensive analysis of the provided type to determine if it can be
// successfully serialized and deserialized according to SSZ specifications. It recursively
// validates all nested types within structs, arrays, and slices, ensuring complete compatibility
// throughout the type hierarchy.
//
// The validation process checks for:
//   - Supported primitive types (bool, uint8, uint16, uint32, uint64)
//   - Valid composite types (arrays, slices, structs)
//   - Proper SSZ tags on slice fields (ssz-size, ssz-max, dynssz-size, dynssz-max)
//   - Correct tag syntax and values
//   - No unsupported types (strings, maps, channels, signed integers, floats, etc.)
//
// When a view descriptor is provided via WithViewDescriptor option, the method validates:
//   - The schema type (view descriptor) is compatible with SSZ
//   - Schema fields can be mapped to corresponding runtime fields by name
//   - Field type compatibility between schema and runtime types
//
// This method is particularly useful for:
//   - Pre-validation before attempting marshalling/unmarshalling operations
//   - Development-time type checking to catch errors early
//   - Runtime validation of dynamically constructed types
//   - Ensuring type compatibility when integrating with external systems
//   - Validating view descriptor compatibility with runtime types
//
// Parameters:
//   - t: The reflect.Type to validate for SSZ compatibility
//   - opts: Optional CallOptions, including WithViewDescriptor for fork-dependent validation
//
// Returns:
//   - error: nil if the type is valid for SSZ encoding/decoding, or a descriptive error
//     explaining why the type is incompatible. The error message includes details about
//     the specific field or type that caused the validation failure.
//
// Example usage:
//
//	type MyStruct struct {
//	    ValidField   uint64
//	    InvalidField string  // This will cause validation to fail
//	}
//
//	err := ds.ValidateType(reflect.TypeOf(MyStruct{}))
//	if err != nil {
//	    log.Fatal("Type validation failed:", err)
//	    // Output: Type validation failed: field 'InvalidField': unsupported type 'string'
//	}
//
//	// With view descriptor validation:
//	err = ds.ValidateType(reflect.TypeOf(RuntimeBody{}), WithViewDescriptor(&AltairBodyView{}))
//
// The method validates at the type level without requiring an instance of the type,
// making it suitable for early validation scenarios. For performance-critical paths,
// validation results can be cached as type compatibility doesn't change at runtime.
func (d *DynSsz) ValidateType(t reflect.Type, opts ...CallOption) error {
	cfg := applyCallOptions(opts)
	schemaType := d.resolveSchemaType(t, cfg)

	// Attempt to get type descriptor which will validate the type structure.
	// When a view descriptor is specified, this validates the schema/runtime pairing.
	_, err := d.typeCache.GetTypeDescriptorWithSchema(t, schemaType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("type validation failed: %w", err)
	}

	return nil
}
