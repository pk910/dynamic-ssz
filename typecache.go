// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file implements cached type descriptors with unsafe pointer optimization.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"encoding/json"
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
	Kind               reflect.Kind
	Type               reflect.Type
	Size               int32                // SSZ size (-1 if dynamic)
	Len                uint32               // Length of array/slice
	Fields             []FieldDescriptor    // For structs
	DynFields          []DynFieldDescriptor // Dynamic struct fields
	ElemDesc           *TypeDescriptor      // For slices/arrays
	SizeHints          []SszSizeHint        // Size hints from tags
	MaxSizeHints       []SszMaxSizeHint     // Max size hints from tags
	HasDynamicSize     bool                 // Whether this type uses dynamic spec size value that differs from the default
	HasDynamicMax      bool                 // Whether this type uses dynamic spec max value that differs from the default
	IsFastSSZMarshaler bool                 // Whether the type implements fastssz.Marshaler
	IsFastSSZHasher    bool                 // Whether the type implements fastssz.HashRoot
	IsPtr              bool                 // Whether this is a pointer type
	IsByteArray        bool                 // Whether this is a byte array
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

// GetTypeDescriptor returns a cached type descriptor, computing it if necessary, ensuring sequential processing
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

// GetTypeDescriptor returns a cached type descriptor, computing it if necessary
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
	default:
		return nil, fmt.Errorf("unhandled reflection kind: %v", t.Kind())
	}

	if !desc.HasDynamicSize {
		desc.IsFastSSZMarshaler = tc.dynssz.getFastsszConvertCompatibility(t)
	}
	if !desc.HasDynamicMax {
		desc.IsFastSSZHasher = tc.dynssz.getFastsszHashCompatibility(t)
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

// DumpTypeDescriptor returns a JSON representation of a type descriptor for debugging
func (tc *TypeCache) DumpTypeDescriptor(t reflect.Type) (string, error) {
	desc, err := tc.GetTypeDescriptor(t, nil, nil)
	if err != nil {
		return "", err
	}

	jsonBytes, err := json.MarshalIndent(desc, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

// DumpAllCachedTypes returns a JSON representation of all cached type descriptors
func (tc *TypeCache) DumpAllCachedTypes() (string, error) {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	typeMap := make(map[string]*TypeDescriptor)
	idx := 0
	for typ, desc := range tc.descriptors {
		typeMap[fmt.Sprintf("%d-%s", idx, typ.String())] = desc
		idx++
	}

	jsonBytes, err := json.MarshalIndent(typeMap, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}
