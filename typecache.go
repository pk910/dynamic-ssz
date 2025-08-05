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
	Kind                   reflect.Kind
	Type                   reflect.Type
	Size                   int32                     // SSZ size (-1 if dynamic)
	Len                    uint32                    // Length of array/slice
	Fields                 []FieldDescriptor         // For structs
	DynFields              []DynFieldDescriptor      // Dynamic struct fields
	UnionVariants          map[uint8]*TypeDescriptor // Union variant types by index (for CompatibleUnion)
	ElemDesc               *TypeDescriptor           // For slices/arrays
	SizeHints              []SszSizeHint             // Size hints from tags
	MaxSizeHints           []SszMaxSizeHint          // Max size hints from tags
	TypeHints              []SszTypeHint             // Type hints from tags
	HasDynamicSize         bool                      // Whether this type uses dynamic spec size value that differs from the default
	HasDynamicMax          bool                      // Whether this type uses dynamic spec max value that differs from the default
	IsFastSSZMarshaler     bool                      // Whether the type implements fastssz.Marshaler
	IsFastSSZHasher        bool                      // Whether the type implements fastssz.HashRoot
	HasHashTreeRootWith    bool                      // Whether the type implements HashTreeRootWith
	IsPtr                  bool                      // Whether this is a pointer type
	IsByteArray            bool                      // Whether this is a byte array
	IsString               bool                      // Whether this is a string type
	IsProgressiveContainer bool                      // Whether this is a progressive container
	IsCompatibleUnion      bool                      // Whether this is a CompatibleUnion type
}

// FieldDescriptor represents a cached descriptor for a struct field
type FieldDescriptor struct {
	Name      string
	Offset    uintptr         // Unsafe offset within the struct
	Type      *TypeDescriptor // Type descriptor
	Index     int16           // Index of the field in the struct
	SszIndex  uint16          // SSZ index for progressive containers
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
//   - typeHints: Optional type hints from parent structures' tags. Pass nil for top-level types.
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
func (tc *TypeCache) GetTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	// Check cache first (read lock)
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
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

	return tc.getTypeDescriptor(t, sizeHints, maxSizeHints, typeHints)
}

// getTypeDescriptor returns a cached type descriptor, computing it if necessary
func (tc *TypeCache) getTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	cacheable := len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0
	if desc, exists := tc.descriptors[t]; exists && cacheable {
		return desc, nil
	}

	desc, err := tc.buildTypeDescriptor(t, sizeHints, maxSizeHints, typeHints)
	if err != nil {
		return nil, err
	}

	// Cache only if no size hints (cacheable)
	if cacheable {
		tc.descriptors[t] = desc
	}

	return desc, nil
}

// buildTypeDescriptor computes a type descriptor for the given type
func (tc *TypeCache) buildTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	desc := &TypeDescriptor{
		Type:         t,
		SizeHints:    sizeHints,
		MaxSizeHints: maxSizeHints,
		TypeHints:    typeHints,
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
		err := tc.buildArrayDescriptor(desc, t, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case reflect.Slice:
		err := tc.buildSliceDescriptor(desc, t, sizeHints, maxSizeHints, typeHints)
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

	// String handling
	case reflect.String:
		err := tc.buildStringDescriptor(desc, sizeHints, maxSizeHints)
		if err != nil {
			return nil, err
		}
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
	// Check for CompatibleUnion type before general struct handling
	if IsCompatibleUnionType(t) {
		err := tc.buildCompatibleUnionDescriptor(desc, t)
		if err != nil {
			return err
		}
		return nil
	}

	desc.Fields = make([]FieldDescriptor, t.NumField())
	totalSize := int32(0)
	isDynamic := false

	// Check for progressive container detection
	hasAnyIndexTag := false
	allFieldsHaveIndex := true
	fieldIndices := make(map[uint16]bool)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldDesc := FieldDescriptor{
			Name:   field.Name,
			Offset: field.Offset,
			Index:  int16(i),
			IsPtr:  field.Type.Kind() == reflect.Ptr,
		}

		// Get ssz-index tag
		sszIndex, err := tc.dynssz.getSszIndexTag(&field)
		if err != nil {
			return err
		}

		if sszIndex != nil {
			fieldDesc.SszIndex = *sszIndex
			hasAnyIndexTag = true
			if fieldIndices[*sszIndex] {
				return fmt.Errorf("duplicate ssz-index %d found in field %s", *sszIndex, field.Name)
			}
			fieldIndices[*sszIndex] = true
		} else {
			allFieldsHaveIndex = false
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

		typeHints, err := tc.dynssz.getSszTypeTag(&field)
		if err != nil {
			return err
		}

		// Build type descriptor for field
		fieldDesc.Type, err = tc.getTypeDescriptor(field.Type, sizeHints, maxSizeHints, typeHints)
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

	// Determine if this is a progressive container
	// A container is progressive if it has ssz-index annotations on fields
	// If it's progressive, all fields must have increasing ssz-index tags
	if hasAnyIndexTag {
		if !allFieldsHaveIndex {
			return fmt.Errorf("progressive container requires all fields to have ssz-index tags")
		}

		// For progressive containers, ensure all ssz-index values are properly set
		// and validate they are in increasing order
		for i := 0; i < len(desc.Fields); i++ {
			field := &desc.Fields[i]
			structField := t.Field(i)
			if sszIndex, err := tc.dynssz.getSszIndexTag(&structField); err != nil {
				return err
			} else if sszIndex != nil {
				field.SszIndex = *sszIndex
			} else {
				return fmt.Errorf("progressive container field %s missing ssz-index tag", field.Name)
			}
		}

		// Verify indices are increasing
		for i := 1; i < len(desc.Fields); i++ {
			if desc.Fields[i].SszIndex <= desc.Fields[i-1].SszIndex {
				return fmt.Errorf("progressive container requires increasing ssz-index values (field %s has index %d, previous field has %d)",
					desc.Fields[i].Name, desc.Fields[i].SszIndex, desc.Fields[i-1].SszIndex)
			}
		}

		desc.IsProgressiveContainer = true
	}

	if isDynamic {
		desc.Size = -1
	} else {
		desc.Size = totalSize
	}

	return nil
}

// buildCompatibleUnionDescriptor builds a descriptor for CompatibleUnion types
func (tc *TypeCache) buildCompatibleUnionDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	// CompatibleUnion is always dynamic size (1 byte for type + variable data)
	desc.Size = -1
	desc.IsCompatibleUnion = true

	// Try to extract the descriptor type from the generic type parameter
	descriptorType, err := tc.extractGenericTypeParameter(t)
	if err != nil {
		return err
	}

	// Populate union variants immediately since we have the descriptor type
	desc.UnionVariants = make(map[uint8]*TypeDescriptor)

	// Extract variant information from descriptor struct (includes SSZ annotations)
	variantInfo, err := ExtractUnionDescriptorInfo(descriptorType, tc.dynssz)
	if err != nil {
		return fmt.Errorf("failed to extract union variant info: %w", err)
	}

	// Build type descriptors for each variant using the extracted information
	for variantIndex, info := range variantInfo {
		variantDesc, err := tc.getTypeDescriptor(info.Type, info.SizeHints, info.MaxSizeHints, info.TypeHints)
		if err != nil {
			return fmt.Errorf("failed to build descriptor for union variant %d: %w", variantIndex, err)
		}

		desc.UnionVariants[variantIndex] = variantDesc
	}

	return nil
}

// buildArrayDescriptor builds a descriptor for array types
func (tc *TypeCache) buildArrayDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	childSizeHints := []SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	childTypeHints := []SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	fieldType := t.Elem()
	if fieldType == byteType {
		desc.IsByteArray = true
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, childSizeHints, childMaxSizeHints, childTypeHints)
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
func (tc *TypeCache) buildSliceDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	childSizeHints := []SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	childTypeHints := []SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	fieldType := t.Elem()
	if fieldType == byteType {
		desc.IsByteArray = true
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, childSizeHints, childMaxSizeHints, childTypeHints)
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

// buildStringDescriptor builds a descriptor for string types
func (tc *TypeCache) buildStringDescriptor(desc *TypeDescriptor, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint) error {
	desc.IsString = true

	// Apply size hints
	desc.SizeHints = sizeHints
	desc.MaxSizeHints = maxSizeHints

	// Check if we have a size hint to make this a fixed-size string
	if len(sizeHints) > 0 && !sizeHints[0].Dynamic && sizeHints[0].Size > 0 {
		// Fixed-size string
		desc.Size = int32(sizeHints[0].Size)
	} else {
		// Dynamic string (default)
		desc.Size = -1
	}

	// Dynamic strings might have spec values
	if len(sizeHints) > 0 && sizeHints[0].SpecVal {
		desc.HasDynamicSize = true
	}
	if len(maxSizeHints) > 0 && maxSizeHints[0].SpecVal {
		desc.HasDynamicMax = true
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

// extractGenericTypeParameter extracts the generic type parameter from a CompatibleUnion type.
// This uses reflection to call the GetDescriptorType method on the union type.
func (tc *TypeCache) extractGenericTypeParameter(unionType reflect.Type) (reflect.Type, error) {
	// Create a zero value of the union type to call methods on
	unionValue := reflect.New(unionType)

	// Get the GetDescriptorType method
	method := unionValue.MethodByName("GetDescriptorType")
	if !method.IsValid() {
		return nil, fmt.Errorf("GetDescriptorType method not found on type %s", unionType)
	}

	// Call the method to get the descriptor type
	results := method.Call(nil)
	if len(results) == 0 {
		return nil, fmt.Errorf("GetDescriptorType returned no results")
	}

	// Extract the reflect.Type from the result
	descriptorType, ok := results[0].Interface().(reflect.Type)
	if !ok {
		return nil, fmt.Errorf("GetDescriptorType did not return a reflect.Type")
	}

	return descriptorType, nil
}
