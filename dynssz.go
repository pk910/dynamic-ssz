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
	"sync"

	fastssz "github.com/ferranbt/fastssz"
)

type DynSsz struct {
	fastsszConvertCompatMutex sync.Mutex
	fastsszConvertCompatCache map[reflect.Type]*fastsszConvertCompatibility
	fastsszHashCompatMutex    sync.Mutex
	fastsszHashCompatCache    map[reflect.Type]*fastsszHashCompatibility
	typeSizeMutex             sync.RWMutex
	typeSizeCache             map[reflect.Type]*cachedSszSize
	typeDynMaxCacheMutex      sync.RWMutex
	typeDynMaxCache           map[reflect.Type]*bool
	specValues                map[string]any
	specValueCache            map[string]*cachedSpecValue
	NoFastSsz                 bool
	Verbose                   bool
}

// NewDynSsz creates a new instance of the DynSsz encoder/decoder.
// The 'specs' map contains dynamic properties and configurations that will be applied during SSZ serialization and deserialization processes.
// This allows for flexible and dynamic handling of SSZ encoding/decoding based on the given specifications, making it suitable for various Ethereum presets and custom scenarios.
// Returns a pointer to the newly created DynSsz instance, ready for use in serializing and deserializing operations.
func NewDynSsz(specs map[string]any) *DynSsz {
	if specs == nil {
		specs = map[string]any{}
	}
	return &DynSsz{
		fastsszConvertCompatCache: map[reflect.Type]*fastsszConvertCompatibility{},
		fastsszHashCompatCache:    map[reflect.Type]*fastsszHashCompatibility{},
		typeSizeCache:             map[reflect.Type]*cachedSszSize{},
		typeDynMaxCache:           map[reflect.Type]*bool{},
		specValues:                specs,
		specValueCache:            map[string]*cachedSpecValue{},
	}
}

// MarshalSSZ serializes the given source into its SSZ (Simple Serialize) representation.
// It dynamically handles the serialization of types, including those with dynamic field sizes,
// by leveraging reflection at runtime. This method integrates with fastssz for types
// without dynamic specifications, optimizing performance. It returns the serialized data as a byte slice,
// or an error if serialization fails.
func (d *DynSsz) MarshalSSZ(source any) ([]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	size, err := d.getSszValueSize(sourceType, sourceValue, []sszSizeHint{})
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 0, size)
	newBuf, err := d.marshalType(sourceType, sourceValue, buf, []sszSizeHint{}, 0)
	if err != nil {
		return nil, err
	}

	if len(newBuf) != size {
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

	newBuf, err := d.marshalType(sourceType, sourceValue, buf, []sszSizeHint{}, 0)
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

	size, err := d.getSszValueSize(sourceType, sourceValue, []sszSizeHint{})
	if err != nil {
		return 0, err
	}
	return size, nil
}

// UnmarshalSSZ decodes the given SSZ-encoded data into the target object.
// The 'ssz' byte slice contains the SSZ-encoded data, and 'target' is a pointer to the Go value that will hold the decoded data.
// This method dynamically handles the decoding, accommodating for types with dynamic field sizes.
// It seamlessly integrates with fastssz for types without dynamic specifications to ensure efficient decoding.
// Returns an error if decoding fails or if the provided ssz data has not been fully used for decoding.
func (d *DynSsz) UnmarshalSSZ(target any, ssz []byte) error {
	targetType := reflect.TypeOf(target)
	targetValue := reflect.ValueOf(target)

	consumedBytes, err := d.unmarshalType(targetType, targetValue, ssz, []sszSizeHint{}, 0)
	if err != nil {
		return err
	}

	if consumedBytes != len(ssz) {
		return fmt.Errorf("did not consume full ssz range (consumed: %v, ssz size: %v)", consumedBytes, len(ssz))
	}

	return nil
}

func (d *DynSsz) HashTreeRoot(source any) ([32]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	hh := fastssz.DefaultHasherPool.Get()
	defer func() {
		fastssz.DefaultHasherPool.Put(hh)
	}()

	err := d.buildRootFromType(sourceType, sourceValue, hh, nil, nil, 0)
	if err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}
