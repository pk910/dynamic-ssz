// Package dynssz provides dynamic SSZ (Simple Serialize) encoding and decoding for Ethereum data structures.
// Unlike static code generation approaches, dynssz uses runtime reflection to handle dynamic field sizes,
// making it suitable for various Ethereum presets beyond the mainnet. It seamlessly integrates with fastssz for
// optimal performance when static definitions are applicable.
//
// Copyright (c) 2024 by pk910. See LICENSE file for details.
package dynssz

import (
	"fmt"
	"reflect"
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
	typeCache      *TypeCache                  // Cache for type descriptors
	specValues     map[string]any              // Dynamic specification values
	specValueCache map[string]*cachedSpecValue // Cache for parsed specification expressions

	// NoFastSsz disables the use of fastssz for static types.
	// When true, all encoding/decoding uses reflection-based processing.
	// Generally not recommended unless you need consistent behavior across all types.
	NoFastSsz bool

	// NoFastHash disables the use of optimized hash tree root calculation.
	// When true, uses the standard hasher instead of the fast gohashtree implementation.
	NoFastHash bool

	// Verbose enables detailed logging of encoding/decoding operations.
	// Useful for debugging but impacts performance.
	Verbose bool
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
func NewDynSsz(specs map[string]any) *DynSsz {
	if specs == nil {
		specs = map[string]any{}
	}

	dynssz := &DynSsz{
		specValues:     specs,
		specValueCache: map[string]*cachedSpecValue{},
	}
	dynssz.typeCache = NewTypeCache(dynssz)

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
func (d *DynSsz) GetTypeCache() *TypeCache {
	return d.typeCache
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
func (d *DynSsz) MarshalSSZ(source any) ([]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	size, err := d.getSszValueSize(sourceTypeDesc, sourceValue)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 0, size)
	newBuf, err := d.marshalType(sourceTypeDesc, sourceValue, buf, 0)
	if err != nil {
		return nil, err
	}

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
func (d *DynSsz) MarshalSSZTo(source any, buf []byte) ([]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	newBuf, err := d.marshalType(sourceTypeDesc, sourceValue, buf, 0)
	if err != nil {
		return nil, err
	}

	return newBuf, nil
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
func (d *DynSsz) SizeSSZ(source any) (int, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil, nil)
	if err != nil {
		return 0, err
	}

	size, err := d.getSszValueSize(sourceTypeDesc, sourceValue)
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
func (d *DynSsz) UnmarshalSSZ(target any, ssz []byte) error {
	targetType := reflect.TypeOf(target)
	targetValue := reflect.ValueOf(target)

	targetTypeDesc, err := d.typeCache.GetTypeDescriptor(targetType, nil, nil, nil)
	if err != nil {
		return err
	}

	consumedBytes, err := d.unmarshalType(targetTypeDesc, targetValue, ssz, 0)
	if err != nil {
		return err
	}

	if consumedBytes != len(ssz) {
		return fmt.Errorf("did not consume full ssz range (consumed: %v, ssz size: %v)", consumedBytes, len(ssz))
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
func (d *DynSsz) HashTreeRoot(source any) ([32]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil, nil)
	if err != nil {
		return [32]byte{}, err
	}

	var pool *HasherPool
	if d.NoFastHash {
		pool = &DefaultHasherPool
	} else {
		pool = &FastHasherPool
	}

	hh := pool.Get()
	defer func() {
		pool.Put(hh)
	}()

	err = d.buildRootFromType(sourceTypeDesc, sourceValue, hh, false, 0)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
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
// This method is particularly useful for:
//   - Pre-validation before attempting marshalling/unmarshalling operations
//   - Development-time type checking to catch errors early
//   - Runtime validation of dynamically constructed types
//   - Ensuring type compatibility when integrating with external systems
//
// Parameters:
//   - t: The reflect.Type to validate for SSZ compatibility
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
// The method validates at the type level without requiring an instance of the type,
// making it suitable for early validation scenarios. For performance-critical paths,
// validation results can be cached as type compatibility doesn't change at runtime.
func (d *DynSsz) ValidateType(t reflect.Type) error {
	// Attempt to get type descriptor which will validate the type structure
	_, err := d.typeCache.GetTypeDescriptor(t, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("type validation failed: %w", err)
	}

	return nil
}
