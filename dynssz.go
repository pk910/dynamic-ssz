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

// GetTypeCache returns the type cache for the DynSsz instance.
//
// The type cache stores computed type descriptors for types used in encoding/decoding operations.
// Type descriptors contain optimized information about how to serialize/deserialize specific types,
// including field offsets, size information, and whether fastssz can be used.
//
// This method is primarily useful for debugging, performance analysis, or advanced use cases
// where you need to inspect the cached type information.
//
// Returns:
//   - *TypeCache: The type cache instance containing all cached type descriptors
//
// Example:
//
//	ds := dynssz.NewDynSsz(specs)
//	cache := ds.GetTypeCache()
//
//	// Dump type descriptor for debugging
//	json, err := cache.DumpTypeDescriptor(reflect.TypeOf(myStruct))
func (d *DynSsz) GetTypeCache() *TypeCache {
	return d.typeCache
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

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil)
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
// This method allows direct control over the serialization output buffer, allowing optimizations like buffer reuse.
// The 'source' parameter is the structure to be serialized, and 'buf' is the pre-allocated slice where the serialized data will be written.
// It dynamically handles serialization for types with dynamic field sizes, seamlessly integrating with fastssz when possible.
// Returns the updated buffer containing the serialized data and an error if serialization fails.
func (d *DynSsz) MarshalSSZTo(source any, buf []byte) ([]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil)
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
// This function is useful for pre-determining the amount of space needed to serialize a given source object.
// The 'source' parameter can be any Go value. It dynamically evaluates the size, accommodating types
// with dynamic field sizes efficiently. Returns the calculated size as an int and an error if the process fails.
func (d *DynSsz) SizeSSZ(source any) (int, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil)
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
// The 'ssz' byte slice contains the SSZ-encoded data, and 'target' is a pointer to the Go value that will hold the decoded data.
// This method dynamically handles the decoding, accommodating for types with dynamic field sizes.
// It seamlessly integrates with fastssz for types without dynamic specifications to ensure efficient decoding.
// Returns an error if decoding fails or if the provided ssz data has not been fully used for decoding.
func (d *DynSsz) UnmarshalSSZ(target any, ssz []byte) error {
	targetType := reflect.TypeOf(target)
	targetValue := reflect.ValueOf(target)

	targetTypeDesc, err := d.typeCache.GetTypeDescriptor(targetType, nil, nil)
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

// HashTreeRoot computes the hash tree root of the given source object.
// This method uses the default hasher pool to get a new hasher instance,
// builds the root from the source object, and returns the computed hash root.
// It returns the computed hash root and an error if the process fails.
func (d *DynSsz) HashTreeRoot(source any) ([32]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	sourceTypeDesc, err := d.typeCache.GetTypeDescriptor(sourceType, nil, nil)
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

	err = d.buildRootFromType(sourceTypeDesc, sourceValue, hh, 0)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}
