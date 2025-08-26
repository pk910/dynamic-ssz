// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file implements cached type descriptors with unsafe pointer optimization.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"strings"
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
	Type                   reflect.Type
	Kind                   reflect.Kind              // Go kind of the type
	Size                   uint32                    // SSZ size (-1 if dynamic)
	Len                    uint32                    // Length of array/slice
	Limit                  uint64                    // Limit of array/slice (ssz-max tag)
	ContainerDesc          *ContainerDescriptor      // For structs
	UnionVariants          map[uint8]*TypeDescriptor // Union variant types by index (for CompatibleUnion)
	ElemDesc               *TypeDescriptor           // For slices/arrays
	HashTreeRootWithMethod *reflect.Method           // Cached HashTreeRootWith method for performance
	SszType                SszType                   // SSZ type of the type
	SizeExpression         string                    // The dynamic expression used to calculate the size of the type
	MaxExpression          string                    // The dynamic expression used to calculate the max size of the type
	IsDynamic              bool                      // Whether this type is a dynamic type (or has nested dynamic types)
	HasLimit               bool                      // Whether this type has a limit (ssz-max tag)
	HasDynamicSize         bool                      // Whether this type or any of its nested types uses dynamic spec size value that differs from the default
	HasDynamicMax          bool                      // Whether this type or any of its nested types uses dynamic spec max value that differs from the default
	HasSizeExpr            bool                      // Whether this type or any of its nested types uses a dynamic expression to calculate the size or max size
	HasMaxExpr             bool                      // Whether this type or any of its nested types uses a dynamic expression to calculate the max size
	HasFastSSZMarshaler    bool                      // Whether the type implements fastssz.Marshaler
	HasFastSSZHasher       bool                      // Whether the type implements fastssz.HashRoot
	HasHashTreeRootWith    bool                      // Whether the type implements HashTreeRootWith
	IsPtr                  bool                      // Whether this is a pointer type
	IsByteArray            bool                      // Whether this is a byte array
	IsString               bool                      // Whether this is a string type
}

// FieldDescriptor represents a cached descriptor for a struct field
type ContainerDescriptor struct {
	Fields    []FieldDescriptor    // For structs
	DynFields []DynFieldDescriptor // Dynamic struct fields
}

// FieldDescriptor represents a cached descriptor for a struct field
type FieldDescriptor struct {
	Name     string
	Type     *TypeDescriptor // Type descriptor
	SszIndex uint16          // SSZ index for progressive containers
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
		Type: t,
	}

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		desc.IsPtr = true
		t = t.Elem()
	}

	desc.Kind = t.Kind()

	// check dynamic size and max size
	if len(sizeHints) > 0 {
		desc.SizeExpression = sizeHints[0].Expr
		for _, hint := range sizeHints {
			if hint.Custom {
				desc.HasDynamicSize = true
			}

			if hint.Expr != "" {
				desc.HasSizeExpr = true
			}
		}
	}

	if len(maxSizeHints) > 0 {
		if !maxSizeHints[0].NoValue {
			desc.HasLimit = true
			desc.Limit = maxSizeHints[0].Size
		}
		desc.MaxExpression = maxSizeHints[0].Expr

		for _, hint := range maxSizeHints {
			if hint.Custom {
				desc.HasDynamicMax = true
			}
			if hint.Expr != "" {
				desc.HasMaxExpr = true
			}
		}
	}

	// determine ssz type
	sszType := SszUnspecifiedType
	if len(typeHints) > 0 {
		sszType = typeHints[0].Type
	}

	if desc.Kind == reflect.String {
		desc.IsString = true
	}

	// auto-detect ssz type if not specified
	if sszType == SszUnspecifiedType {
		// detect some well-known and widely used types
		switch {
		case t.PkgPath() == "github.com/holiman/uint256" && t.Name() == "Int":
			sszType = SszUint256Type
		case t.PkgPath() == "github.com/pk910/dynamic-ssz" && strings.HasPrefix(t.Name(), "CompatibleUnion["):
			sszType = SszCompatibleUnionType
		}
		if t.PkgPath() == typeWrapperType.PkgPath() && strings.HasPrefix(t.Name(), "TypeWrapper[") {
			sszType = SszTypeWrapperType
		}
	}
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

		// special case for bitlists
		if sszType == SszListType && strings.Contains(t.Name(), "Bitlist") {
			sszType = SszBitlistType
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
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("bool ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return nil, fmt.Errorf("uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("uint8 ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return nil, fmt.Errorf("uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 2 {
			return nil, fmt.Errorf("uint16 ssz type must be ssz-size:2, got %v", sizeHints[0].Size)
		}
		desc.Size = 2
	case SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return nil, fmt.Errorf("uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 4 {
			return nil, fmt.Errorf("uint32 ssz type must be ssz-size:4, got %v", sizeHints[0].Size)
		}
		desc.Size = 4
	case SszUint64Type:
		if desc.Kind != reflect.Uint64 {
			return nil, fmt.Errorf("uint64 ssz type can only be represented by uint64 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return nil, fmt.Errorf("uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
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
	case SszTypeWrapperType:
		err := tc.buildTypeWrapperDescriptor(desc, t)
		if err != nil {
			return nil, err
		}
	case SszContainerType, SszProgressiveContainerType:
		err := tc.buildContainerDescriptor(desc, t)
		if err != nil {
			return nil, err
		}
	case SszVectorType, SszBitvectorType:
		err := tc.buildVectorDescriptor(desc, t, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszListType, SszBitlistType, SszProgressiveListType, SszProgressiveBitlistType:
		err := tc.buildListDescriptor(desc, t, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszCompatibleUnionType:
		err := tc.buildCompatibleUnionDescriptor(desc, t)
		if err != nil {
			return nil, err
		}
	case SszCustomType:
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			desc.Size = uint32(sizeHints[0].Size)
		} else {
			desc.Size = 0
			desc.IsDynamic = true
		}
	}

	if !desc.HasDynamicSize {
		desc.HasFastSSZMarshaler = tc.dynssz.getFastsszConvertCompatibility(t)
	}
	if !desc.HasDynamicMax {
		desc.HasFastSSZHasher = tc.dynssz.getFastsszHashCompatibility(t)
		desc.HashTreeRootWithMethod = tc.dynssz.getHashTreeRootWithCompatibility(t)
		desc.HasHashTreeRootWith = desc.HashTreeRootWithMethod != nil
	}

	if desc.SszType == SszCustomType && (!desc.HasFastSSZMarshaler || !desc.HasFastSSZHasher) {
		return nil, fmt.Errorf("custom ssz type requires fastssz marshaler and hasher implementations")
	}

	return desc, nil
}

// buildTypeWrapperDescriptor builds a descriptor for TypeWrapper types
func (tc *TypeCache) buildTypeWrapperDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return fmt.Errorf("TypeWrapper ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	// Create a zero value instance to call the GetDescriptorType method
	wrapperValue := reflect.New(t)
	method := wrapperValue.MethodByName("GetDescriptorType")
	if !method.IsValid() {
		return fmt.Errorf("GetDescriptorType method not found on type %s", t)
	}

	// Call the method to get the descriptor type
	results := method.Call(nil)
	if len(results) == 0 {
		return fmt.Errorf("GetDescriptorType returned no results")
	}

	descriptorType, ok := results[0].Interface().(reflect.Type)
	if !ok {
		return fmt.Errorf("GetDescriptorType did not return a reflect.Type")
	}

	// Extract wrapper information from descriptor struct (includes SSZ annotations)
	wrapperInfo, err := extractWrapperDescriptorInfo(descriptorType, tc.dynssz)
	if err != nil {
		return fmt.Errorf("failed to extract wrapper descriptor info: %w", err)
	}

	// Build type descriptor for the wrapped type using the extracted information
	wrappedDesc, err := tc.getTypeDescriptor(wrapperInfo.Type, wrapperInfo.SizeHints, wrapperInfo.MaxSizeHints, wrapperInfo.TypeHints)
	if err != nil {
		return fmt.Errorf("failed to build descriptor for wrapped type: %w", err)
	}

	// Store wrapper information
	desc.ElemDesc = wrappedDesc

	// The TypeWrapper inherits properties from the wrapped type
	desc.Size = wrappedDesc.Size
	desc.IsDynamic = wrappedDesc.IsDynamic
	desc.HasDynamicSize = wrappedDesc.HasDynamicSize
	desc.HasDynamicMax = wrappedDesc.HasDynamicMax
	desc.HasSizeExpr = wrappedDesc.HasSizeExpr
	desc.HasMaxExpr = wrappedDesc.HasMaxExpr

	return nil
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

	desc.ContainerDesc = &ContainerDescriptor{
		Fields:    make([]FieldDescriptor, t.NumField()),
		DynFields: make([]DynFieldDescriptor, 0),
	}

	totalSize := uint32(0)
	isDynamic := false

	// Check for progressive container detection
	hasAnyIndexTag := false
	allFieldsHaveIndex := true
	fieldIndices := make(map[uint16]bool)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldDesc := FieldDescriptor{
			Name: field.Name,
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

		sszSize := fieldDesc.Type.Size
		if fieldDesc.Type.IsDynamic {
			isDynamic = true
			sszSize = 4 // Offset size for dynamic fields

			desc.ContainerDesc.DynFields = append(desc.ContainerDesc.DynFields, DynFieldDescriptor{
				Field:  &desc.ContainerDesc.Fields[i],
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

		if fieldDesc.Type.HasSizeExpr {
			desc.HasSizeExpr = true
		}

		if fieldDesc.Type.HasMaxExpr {
			desc.HasMaxExpr = true
		}

		totalSize += sszSize
		desc.ContainerDesc.Fields[i] = fieldDesc
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
		for i := 0; i < len(desc.ContainerDesc.Fields); i++ {
			field := &desc.ContainerDesc.Fields[i]
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
		for i := 1; i < len(desc.ContainerDesc.Fields); i++ {
			if desc.ContainerDesc.Fields[i].SszIndex <= desc.ContainerDesc.Fields[i-1].SszIndex {
				return fmt.Errorf("progressive container requires increasing ssz-index values (field %s has index %d, previous field has %d)",
					desc.ContainerDesc.Fields[i].Name, desc.ContainerDesc.Fields[i].SszIndex, desc.ContainerDesc.Fields[i-1].SszIndex)
			}
		}

		desc.SszType = SszProgressiveContainerType
	}

	if isDynamic {
		desc.Size = 0
		desc.IsDynamic = true
	} else {
		desc.Size = totalSize
	}

	return nil
}

// buildCompatibleUnionDescriptor builds a descriptor for CompatibleUnion types
func (tc *TypeCache) buildCompatibleUnionDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	// CompatibleUnion is always dynamic size (1 byte for type + variable data)
	desc.Size = 0
	desc.IsDynamic = true

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

// buildVectorDescriptor builds a descriptor for ssz vector types
func (tc *TypeCache) buildVectorDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	if desc.Kind != reflect.Array && desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return fmt.Errorf("vector ssz type can only be represented by array or slice types, got %v", desc.Kind)
	}

	if desc.Kind == reflect.Array {
		desc.Len = uint32(t.Len())
		if len(sizeHints) > 0 {
			if sizeHints[0].Size > desc.Len {
				return fmt.Errorf("size hint for vector type is greater than the length of the array (%d > %d)", sizeHints[0].Size, desc.Len)
			}
			desc.Len = uint32(sizeHints[0].Size)
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
	if elemDesc.HasSizeExpr {
		desc.HasSizeExpr = true
	}
	if elemDesc.HasMaxExpr {
		desc.HasMaxExpr = true
	}

	if desc.SszType == SszBitvectorType && desc.ElemDesc.Kind != reflect.Uint8 {
		return fmt.Errorf("bitvector ssz type can only be represented by byte slices or arrays, got %v", desc.ElemDesc.Kind.String())
	}

	if elemDesc.IsDynamic {
		desc.Size = 0
		desc.IsDynamic = true
	} else {
		desc.Size = elemDesc.Size * desc.Len
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
	if elemDesc.HasSizeExpr {
		desc.HasSizeExpr = true
	}
	if elemDesc.HasMaxExpr {
		desc.HasMaxExpr = true
	}

	if (desc.SszType == SszBitlistType || desc.SszType == SszProgressiveBitlistType) && desc.ElemDesc.Kind != reflect.Uint8 {
		return fmt.Errorf("bitlist ssz type can only be represented by byte slices or arrays, got %v", desc.ElemDesc.Kind.String())
	}

	if len(sizeHints) > 0 && sizeHints[0].Size > 0 && !sizeHints[0].Dynamic {
		if elemDesc.IsDynamic {
			desc.Size = 0 // Dynamic elements = dynamic size
			desc.IsDynamic = true
		} else {
			desc.Size = elemDesc.Size * sizeHints[0].Size
		}
	} else {
		desc.Size = 0 // Dynamic slice
		desc.IsDynamic = true
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
