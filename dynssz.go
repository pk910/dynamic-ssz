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
// The instance holds caches for referenced types, so it's recommended to reuse the same instance to speed up the encoding/decoding process.
type DynSsz struct {
	typeCache      *TypeCache
	specValues     map[string]any
	specValueCache map[string]*cachedSpecValue
	NoFastSsz      bool
	Verbose        bool
}

// GetTypeCache returns the type cache for the DynSsz instance.
// The type cache is used to store the type descriptors for the types that are used in the encoding/decoding process.
// It's recommended to reuse the same instance to speed up the encoding/decoding process.
func (d *DynSsz) GetTypeCache() *TypeCache {
	return d.typeCache
}

// NewDynSsz creates a new instance of the DynSsz encoder/decoder.
// The 'specs' map contains dynamic properties and configurations that will be applied during SSZ serialization and deserialization processes.
// This allows for flexible and dynamic handling of SSZ encoding/decoding based on the given specifications, making it suitable for various Ethereum presets and custom scenarios.
// Returns a pointer to the newly created DynSsz instance, ready for use in serializing and deserializing operations.
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
// It dynamically handles the serialization of types, including those with dynamic field sizes,
// by leveraging reflection at runtime. This method integrates with fastssz for types
// without dynamic specifications, optimizing performance. It returns the serialized data as a byte slice,
// or an error if serialization fails.
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

	hh := DefaultHasherPool.Get()
	defer func() {
		DefaultHasherPool.Put(hh)
	}()

	err = d.buildRootFromType(sourceTypeDesc, sourceValue, hh, 0)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}
