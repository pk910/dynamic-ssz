// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file implements cached type descriptors with unsafe pointer optimization.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"sync"
)

// TypeCache manages cached type descriptors
type TypeCache struct {
	dynssz      *DynSsz
	mutex       sync.RWMutex
	descriptors map[reflect.Type]*TypeDescriptor
}

// TypeDescriptor represents a cached, optimized descriptor for a type's SSZ encoding/decoding
type TypeDescriptor struct {
	Kind                reflect.Kind
	Type                reflect.Type
	Size                int32                // SSZ size (-1 if dynamic)
	Len                 uint32               // Length of array/slice
	Fields              []FieldDescriptor    // For structs
	DynFields           []DynFieldDescriptor // Dynamic struct fields
	ElemDesc            *TypeDescriptor      // For slices/arrays
	SizeHints           []SszSizeHint        // Size hints from tags
	MaxSizeHints        []SszMaxSizeHint     // Max size hints from tags
	HasDynamicSize      bool                 // Whether this type uses dynamic spec size value that differs from the default
	HasDynamicMax       bool                 // Whether this type uses dynamic spec max value that differs from the default
	IsFastSSZMarshaler  bool                 // Whether the type implements fastssz.Marshaler
	IsFastSSZHasher     bool                 // Whether the type implements fastssz.HashRoot
	HasHashTreeRootWith bool                 // Whether the type implements HashTreeRootWith
	IsPtr               bool                 // Whether this is a pointer type
	IsByteArray         bool                 // Whether this is a byte array
}

// FieldDescriptor represents a cached descriptor for a struct field
type FieldDescriptor struct {
	Name      string
	Offset    uintptr         // Unsafe offset within the struct
	Type      *TypeDescriptor // Type descriptor
	Index     int16           // Index of the field in the struct
	Size      int32           // SSZ size (-1 if dynamic)
	IsPtr     bool            // Whether field is a pointer
	IsDynamic bool            // Whether field has dynamic size
}

// DynFieldDescriptor represents a dynamic field descriptor for a struct
type DynFieldDescriptor struct {
	Field  *FieldDescriptor
	Offset uint32
}

// NewTypeCache creates a new type cache
func NewTypeCache(dynssz *DynSsz) *TypeCache {
	return &TypeCache{
		dynssz:      dynssz,
		descriptors: make(map[reflect.Type]*TypeDescriptor),
	}
}

// GetTypeDescriptor returns a cached type descriptor for the given type, computing it if necessary.
//
// This method is the primary interface for obtaining type descriptors, which contain optimized
// metadata about how to serialize, deserialize, and hash types according to SSZ specifications.
// Type descriptors are cached for performance, avoiding repeated reflection and analysis of the
// same types.
//
// The method is thread-safe and ensures sequential processing to prevent duplicate computation
// of type descriptors when called concurrently for the same type.
//
// Parameters:
//   - t: The reflect.Type for which to obtain a descriptor
//   - sizeHints: Optional size hints from parent structures' tags. Pass nil for top-level types.
//   - maxSizeHints: Optional max size hints from parent structures' tags. Pass nil for top-level types.
//
// Returns:
//   - *TypeDescriptor: The type descriptor containing metadata for SSZ operations
//   - error: An error if the type cannot be analyzed or contains unsupported features
//
// Type descriptors are only cached when no size hints are provided (i.e., for root types).
// When size hints are present, the descriptor is computed dynamically to accommodate the
// specific constraints.
//
// Example:
//
//	typeDesc, err := cache.GetTypeDescriptor(reflect.TypeOf(myStruct), nil, nil)
//	if err != nil {
//	    log.Fatal("Failed to get type descriptor:", err)
//	}
//	fmt.Printf("Type size: %d bytes (dynamic: %v)\n", typeDesc.Size, typeDesc.Size < 0)
func (tc *TypeCache) GetTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint) (*TypeDescriptor, error) {
	// Check cache first (read lock)
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 {
		tc.mutex.RLock()
		if desc, exists := tc.descriptors[t]; exists {
			tc.mutex.RUnlock()
			return desc, nil
		}
		tc.mutex.RUnlock()
	}

	// If not in cache, build and cache it (write lock)
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	return tc.getTypeDescriptor(t, sizeHints, maxSizeHints)
}

// getTypeDescriptor returns a cached type descriptor, computing it if necessary
func (tc *TypeCache) getTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint) (*TypeDescriptor, error) {
	if desc, exists := tc.descriptors[t]; exists && len(sizeHints) == 0 && len(maxSizeHints) == 0 {
		return desc, nil
	}

	desc, err := tc.buildTypeDescriptor(t, sizeHints, maxSizeHints)
	if err != nil {
		return nil, err
	}

	// Cache only if no size hints (cacheable)
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 {
		tc.descriptors[t] = desc
	}

	return desc, nil
}

// buildTypeDescriptor computes a type descriptor for the given type
func (tc *TypeCache) buildTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint) (*TypeDescriptor, error) {
	desc := &TypeDescriptor{
		Type:         t,
		SizeHints:    sizeHints,
		MaxSizeHints: maxSizeHints,
	}

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		desc.IsPtr = true
		t = t.Elem()
	}

	desc.Kind = t.Kind()

	// check dynamic size and max size
	if len(sizeHints) > 0 {
		for _, hint := range sizeHints {
			if hint.SpecVal {
				desc.HasDynamicSize = true
			}
		}
	}

	if len(maxSizeHints) > 0 {
		for _, hint := range maxSizeHints {
			if hint.SpecVal {
				desc.HasDynamicMax = true
			}
		}
	}

	// Compute size and build field descriptors
	switch desc.Kind {
	case reflect.Struct:
		err := tc.buildStructDescriptor(desc, t)
		if err != nil {
			return nil, err
		}
	case reflect.Array:
		err := tc.buildArrayDescriptor(desc, t, sizeHints, maxSizeHints)
		if err != nil {
			return nil, err
		}
	case reflect.Slice:
		err := tc.buildSliceDescriptor(desc, t, sizeHints, maxSizeHints)
		if err != nil {
			return nil, err
		}
	case reflect.Bool:
		desc.Size = 1
	case reflect.Uint8:
		desc.Size = 1
	case reflect.Uint16:
		desc.Size = 2
	case reflect.Uint32:
		desc.Size = 4
	case reflect.Uint64:
		desc.Size = 8

	// Explicitly unsupported types with helpful error messages
	case reflect.String:
		return nil, fmt.Errorf("strings are not supported in SSZ (use []byte instead)")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return nil, fmt.Errorf("signed integers are not supported in SSZ (use unsigned integers instead)")
	case reflect.Float32, reflect.Float64:
		return nil, fmt.Errorf("floating-point numbers are not supported in SSZ")
	case reflect.Complex64, reflect.Complex128:
		return nil, fmt.Errorf("complex numbers are not supported in SSZ")
	case reflect.Map:
		return nil, fmt.Errorf("maps are not supported in SSZ (use structs or arrays instead)")
	case reflect.Chan:
		return nil, fmt.Errorf("channels are not supported in SSZ")
	case reflect.Func:
		return nil, fmt.Errorf("functions are not supported in SSZ")
	case reflect.Interface:
		return nil, fmt.Errorf("interfaces are not supported in SSZ (use concrete types)")
	case reflect.UnsafePointer:
		return nil, fmt.Errorf("unsafe pointers are not supported in SSZ")
	default:
		return nil, fmt.Errorf("unsupported type kind: %v", t.Kind())
	}

	if !desc.HasDynamicSize {
		desc.IsFastSSZMarshaler = tc.dynssz.getFastsszConvertCompatibility(t)
	}
	if !desc.HasDynamicMax {
		desc.IsFastSSZHasher = tc.dynssz.getFastsszHashCompatibility(t)
		desc.HasHashTreeRootWith = tc.dynssz.getHashTreeRootWithCompatibility(t)
	}

	return desc, nil
}

// buildStructDescriptor builds a descriptor for struct types
func (tc *TypeCache) buildStructDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	desc.Fields = make([]FieldDescriptor, t.NumField())
	totalSize := int32(0)
	isDynamic := false

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldDesc := FieldDescriptor{
			Name:   field.Name,
			Offset: field.Offset,
			Index:  int16(i),
			IsPtr:  field.Type.Kind() == reflect.Ptr,
		}

		// Get size hints from tags
		sizeHints, err := tc.dynssz.getSszSizeTag(&field)
		if err != nil {
			return err
		}

		maxSizeHints, err := tc.dynssz.getSszMaxSizeTag(&field)
		if err != nil {
			return err
		}

		// Build type descriptor for field
		fieldDesc.Type, err = tc.getTypeDescriptor(field.Type, sizeHints, maxSizeHints)
		if err != nil {
			return err
		}

		fieldDesc.Size = fieldDesc.Type.Size
		sszSize := fieldDesc.Size
		if fieldDesc.Size < 0 {
			fieldDesc.IsDynamic = true
			isDynamic = true
			sszSize = 4 // Offset size for dynamic fields

			desc.DynFields = append(desc.DynFields, DynFieldDescriptor{
				Field:  &desc.Fields[i],
				Offset: uint32(totalSize),
			})
		}

		if fieldDesc.Type.HasDynamicSize {
			desc.HasDynamicSize = true
		}

		if fieldDesc.Type.HasDynamicMax {
			desc.HasDynamicMax = true
		}

		totalSize += sszSize
		desc.Fields[i] = fieldDesc
	}

	if isDynamic {
		desc.Size = -1
	} else {
		desc.Size = totalSize
	}

	return nil
}

// buildArrayDescriptor builds a descriptor for array types
func (tc *TypeCache) buildArrayDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint) error {
	childSizeHints := []SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	fieldType := t.Elem()
	if fieldType == byteType {
		desc.IsByteArray = true
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, childSizeHints, childMaxSizeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.Len = uint32(t.Len())
	if elemDesc.HasDynamicSize {
		desc.HasDynamicSize = true
	}
	if elemDesc.HasDynamicMax {
		desc.HasDynamicMax = true
	}

	if elemDesc.Size < 0 {
		desc.Size = -1
	} else {
		desc.Size = elemDesc.Size * int32(t.Len())
	}

	return nil
}

// buildSliceDescriptor builds a descriptor for slice types
func (tc *TypeCache) buildSliceDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint) error {
	childSizeHints := []SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	fieldType := t.Elem()
	if fieldType == byteType {
		desc.IsByteArray = true
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, childSizeHints, childMaxSizeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	if elemDesc.HasDynamicSize {
		desc.HasDynamicSize = true
	}
	if elemDesc.HasDynamicMax {
		desc.HasDynamicMax = true
	}

	if len(sizeHints) > 0 && sizeHints[0].Size > 0 && !sizeHints[0].Dynamic {
		if elemDesc.Size < 0 {
			desc.Size = -1 // Dynamic elements = dynamic size
		} else {
			desc.Size = elemDesc.Size * int32(sizeHints[0].Size)
		}
	} else {
		desc.Size = -1 // Dynamic slice
	}

	return nil
}

// GetAllTypes returns a slice of all types currently cached in the TypeCache.
//
// This method is useful for cache inspection, debugging, and understanding which types
// have been processed and cached during the application's lifetime. The returned slice
// contains the reflect.Type values in no particular order.
//
// The method acquires a read lock to ensure thread-safe access to the cache.
//
// Returns:
//   - []reflect.Type: A slice containing all cached types
//
// Example:
//
//	cachedTypes := cache.GetAllTypes()
//	fmt.Printf("TypeCache contains %d types\n", len(cachedTypes))
//	for _, t := range cachedTypes {
//	    fmt.Printf("  - %s\n", t.String())
//	}
func (tc *TypeCache) GetAllTypes() []reflect.Type {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	types := make([]reflect.Type, 0, len(tc.descriptors))
	for t := range tc.descriptors {
		types = append(types, t)
	}

	return types
}

// RemoveType removes a specific type from the cache.
//
// This method is useful for cache management scenarios where you need to force
// recomputation of a type descriptor, such as after configuration changes or
// when testing different type configurations.
//
// The method acquires a write lock to ensure thread-safe removal.
//
// Parameters:
//   - t: The reflect.Type to remove from the cache
//
// Example:
//
//	// Remove a type to force recomputation
//	cache.RemoveType(reflect.TypeOf(MyStruct{}))
//
//	// Next call to GetTypeDescriptor will rebuild the descriptor
//	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(MyStruct{}), nil, nil)
func (tc *TypeCache) RemoveType(t reflect.Type) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	delete(tc.descriptors, t)
}

// RemoveAllTypes clears all cached type descriptors from the cache.
//
// This method is useful for:
//   - Resetting the cache after configuration changes
//   - Memory management in long-running applications
//   - Testing scenarios requiring a clean cache state
//
// The method acquires a write lock to ensure thread-safe clearing.
// After calling this method, all subsequent type descriptor requests
// will trigger recomputation.
//
// Example:
//
//	// Clear cache after updating specifications
//	ds.UpdateSpecs(newSpecs)
//	cache.RemoveAllTypes()
//
//	// All types will be recomputed with new specs
//	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(MyStruct{}), nil, nil)
func (tc *TypeCache) RemoveAllTypes() {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	// Create new map to clear all references
	tc.descriptors = make(map[reflect.Type]*TypeDescriptor)
}
