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
	TypeHints           []SszTypeHint        // Type hints from tags
	SszType             SszType              // SSZ type of the type
	HasDynamicSize      bool                 // Whether this type uses dynamic spec size value that differs from the default
	HasDynamicMax       bool                 // Whether this type uses dynamic spec max value that differs from the default
	IsFastSSZMarshaler  bool                 // Whether the type implements fastssz.Marshaler
	IsFastSSZHasher     bool                 // Whether the type implements fastssz.HashRoot
	HasHashTreeRootWith bool                 // Whether the type implements HashTreeRootWith
	IsPtr               bool                 // Whether this is a pointer type
	IsByteArray         bool                 // Whether this is a byte array
	IsString            bool                 // Whether this is a string type
}

// FieldDescriptor represents a cached descriptor for a struct field
type FieldDescriptor struct {
	Name string
	Type *TypeDescriptor // Type descriptor
}

// DynFieldDescriptor represents a dynamic field descriptor for a struct
type DynFieldDescriptor struct {
	Field  *FieldDescriptor
	Offset uint32
	Index  int16 // Index of the field in the struct
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

	// determine ssz type
	sszType := SszUnspecifiedType
	if len(typeHints) > 0 {
		sszType = typeHints[0].Type
	}

	// auto-detect ssz type if not specified
	if sszType == SszUnspecifiedType {
		switch desc.Kind {
		// basic types
		case reflect.Bool:
			sszType = SszBoolType
		case reflect.Uint8:
			sszType = SszUint8Type
		case reflect.Uint16:
			sszType = SszUint16Type
		case reflect.Uint32:
			sszType = SszUint32Type
		case reflect.Uint64:
			sszType = SszUint64Type

		// complex types
		case reflect.Struct:
			sszType = SszContainerType
		case reflect.Array:
			sszType = SszVectorType
		case reflect.Slice:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = SszVectorType
			} else {
				sszType = SszListType
			}
		case reflect.String:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = SszVectorType
			} else {
				sszType = SszListType
			}
			desc.IsString = true

		// unsupported types
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
	}

	desc.SszType = sszType

	// Check type compatibility and compute size
	switch sszType {
	// basic types
	case SszBoolType:
		if desc.Kind != reflect.Bool {
			return nil, fmt.Errorf("bool ssz type can only be represented by bool types, got %v", desc.Kind)
		}
		desc.Size = 1
	case SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return nil, fmt.Errorf("uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		desc.Size = 1
	case SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return nil, fmt.Errorf("uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		desc.Size = 2
	case SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return nil, fmt.Errorf("uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case SszUint64Type:
		if desc.Kind != reflect.Uint64 {
			return nil, fmt.Errorf("uint64 ssz type can only be represented by uint64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case SszUint128Type:
		err := tc.buildUint128Descriptor(desc, t) // handle as [16]uint8 or [2]uint64
		if err != nil {
			return nil, err
		}
	case SszUint256Type:
		err := tc.buildUint256Descriptor(desc, t) // handle as [32]uint8 or [4]uint64
		if err != nil {
			return nil, err
		}

	// complex types
	case SszContainerType:
		err := tc.buildContainerDescriptor(desc, t)
		if err != nil {
			return nil, err
		}
	case SszVectorType, SszBitvectorType:
		err := tc.buildVectorDescriptor(desc, t, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszListType, SszBitlistType:
		err := tc.buildListDescriptor(desc, t, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	}

	if !desc.HasDynamicSize {
		desc.IsFastSSZMarshaler = tc.dynssz.getFastsszConvertCompatibility(t)
	}
	if !desc.HasDynamicMax {
		desc.IsFastSSZHasher = tc.dynssz.getFastsszHashCompatibility(t)
		desc.HasHashTreeRootWith = tc.dynssz.getHashTreeRootWithCompatibility(t)
	}

	if desc.SszType == SszCustomType && (!desc.IsFastSSZMarshaler || !desc.IsFastSSZHasher) {
		return nil, fmt.Errorf("custom ssz type requires fastssz marshaler and hasher implementations")
	}

	return desc, nil
}

// buildUint128Descriptor builds a descriptor for uint128 types
func (tc *TypeCache) buildUint128Descriptor(desc *TypeDescriptor, t reflect.Type) error {
	if desc.Kind != reflect.Slice && desc.Kind != reflect.Array {
		return fmt.Errorf("uint128 ssz type can only be represented by slice or array types, got %v", desc.Kind)
	}

	fieldType := t.Elem()
	elemKind := fieldType.Kind()
	if elemKind != reflect.Uint8 && elemKind != reflect.Uint64 {
		return fmt.Errorf("uint128 ssz type can only be represented by slices or arrays of uint8 or uint64, got %v", elemKind)
	} else if elemKind == reflect.Uint8 {
		desc.IsByteArray = true
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, nil, nil, nil)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.Size = 16 // hardcoded size for uint128
	desc.Len = uint32(desc.Size / elemDesc.Size)

	if desc.Kind == reflect.Array {
		dstLen := uint32(t.Len())
		if dstLen < desc.Len {
			return fmt.Errorf("uint128 ssz type does not fit in array (%d < %d)", dstLen, desc.Len)
		}
	}

	return nil
}

// buildUint256Descriptor builds a descriptor for uint256 types
func (tc *TypeCache) buildUint256Descriptor(desc *TypeDescriptor, t reflect.Type) error {
	if desc.Kind != reflect.Slice && desc.Kind != reflect.Array {
		return fmt.Errorf("uint256 ssz type can only be represented by slice or array types, got %v", desc.Kind)
	}

	fieldType := t.Elem()
	elemKind := fieldType.Kind()
	if elemKind != reflect.Uint8 && elemKind != reflect.Uint64 {
		return fmt.Errorf("uint256 ssz type can only be represented by slices or arrays of uint8 or uint64, got %v", elemKind)
	} else if elemKind == reflect.Uint8 {
		desc.IsByteArray = true
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, nil, nil, nil)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.Size = 32 // hardcoded size for uint256
	desc.Len = uint32(desc.Size / elemDesc.Size)

	if desc.Kind == reflect.Array {
		dstLen := uint32(t.Len())
		if dstLen < desc.Len {
			return fmt.Errorf("uint256 ssz type does not fit in array (%d < %d)", dstLen, desc.Len)
		}
	}

	return nil
}

// buildContainerDescriptor builds a descriptor for ssz container types
func (tc *TypeCache) buildContainerDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return fmt.Errorf("container ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	desc.Fields = make([]FieldDescriptor, t.NumField())
	totalSize := int32(0)
	isDynamic := false

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldDesc := FieldDescriptor{
			Name: field.Name,
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

		sszSize := fieldDesc.Type.Size
		if sszSize < 0 {
			isDynamic = true
			sszSize = 4 // Offset size for dynamic fields

			desc.DynFields = append(desc.DynFields, DynFieldDescriptor{
				Field:  &desc.Fields[i],
				Offset: uint32(totalSize),
				Index:  int16(i),
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

// buildVectorDescriptor builds a descriptor for ssz vector types
func (tc *TypeCache) buildVectorDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	if desc.Kind != reflect.Array && desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return fmt.Errorf("vector ssz type can only be represented by array or slice types, got %v", desc.Kind)
	}

	if desc.Kind == reflect.Array {
		desc.Len = uint32(t.Len())
		if len(sizeHints) > 0 && sizeHints[0].Size > desc.Len {
			return fmt.Errorf("size hint for vector type is greater than the length of the array (%d > %d)", sizeHints[0].Size, desc.Len)
		}
	} else if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
		desc.Len = uint32(sizeHints[0].Size)
	} else {
		return fmt.Errorf("missing size hint for vector type")
	}

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

	var fieldType reflect.Type
	if desc.Kind == reflect.String {
		fieldType = byteType
		desc.IsByteArray = true
	} else {
		fieldType = t.Elem()
		if fieldType == byteType {
			desc.IsByteArray = true
		}
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

	if elemDesc.Size < 0 {
		desc.Size = -1
	} else {
		desc.Size = elemDesc.Size * int32(desc.Len)
	}

	return nil
}

// buildListDescriptor builds a descriptor for ssz list types
func (tc *TypeCache) buildListDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	if desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return fmt.Errorf("list ssz type can only be represented by slice types, got %v", desc.Kind)
	}

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

	var fieldType reflect.Type
	if desc.Kind == reflect.String {
		fieldType = byteType
		desc.IsByteArray = true
	} else {
		fieldType = t.Elem()
		if fieldType == byteType {
			desc.IsByteArray = true
		}
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
