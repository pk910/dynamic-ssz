// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// typeKey represents a composite cache key for type descriptors.
// When schema type differs from runtime type (fork views), we need to cache
// descriptors based on both types since the same runtime type may have
// different SSZ layouts depending on the schema.
type typeKey struct {
	runtime reflect.Type // The type where actual data lives
	schema  reflect.Type // The type that defines SSZ layout (may differ for views)
}

// TypeCache manages cached type descriptors
type TypeCache struct {
	specs       sszutils.DynamicSpecs
	mutex       sync.RWMutex
	descriptors map[typeKey]*TypeDescriptor
	CompatFlags map[string]SszCompatFlag
}

// NewTypeCache creates a new type cache
func NewTypeCache(specs sszutils.DynamicSpecs) *TypeCache {
	return &TypeCache{
		specs:       specs,
		descriptors: make(map[typeKey]*TypeDescriptor),
		CompatFlags: map[string]SszCompatFlag{},
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
	// When no view descriptor is used, runtime and schema types are the same
	return tc.GetTypeDescriptorWithSchema(t, t, sizeHints, maxSizeHints, typeHints)
}

// GetTypeDescriptorWithSchema returns a cached type descriptor for a (runtime, schema) type pair.
//
// This method supports fork-dependent SSZ schemas (view descriptors) where the schema type
// defines the SSZ layout while the runtime type holds the actual data. This allows different
// SSZ serializations of the same runtime data based on the schema provided.
//
// When runtimeType == schemaType, this behaves identically to GetTypeDescriptor.
// When they differ, the descriptor is built using schema's field definitions (names, tags,
// order) but accessing data from the runtime type's fields.
//
// Parameters:
//   - runtimeType: The reflect.Type where actual data lives
//   - schemaType: The reflect.Type that defines SSZ layout (field order, tags, limits)
//   - sizeHints: Optional size hints from parent structures' tags
//   - maxSizeHints: Optional max size hints from parent structures' tags
//   - typeHints: Optional type hints from parent structures' tags
//
// Returns:
//   - *TypeDescriptor: The type descriptor for the (runtime, schema) pair
//   - error: An error if types are incompatible or analysis fails
//
// Example with view descriptor:
//
//	// Runtime type (superset)
//	type BeaconBlockBody struct { ... }
//	// Schema/view type for Altair fork
//	type BodyAltairView struct { ... }
//
//	desc, err := cache.GetTypeDescriptorWithSchema(
//	    reflect.TypeOf(BeaconBlockBody{}),
//	    reflect.TypeOf(BodyAltairView{}),
//	    nil, nil, nil,
//	)
func (tc *TypeCache) GetTypeDescriptorWithSchema(runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	key := typeKey{runtime: runtimeType, schema: schemaType}

	// Check cache first (read lock)
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
		tc.mutex.RLock()
		if desc, exists := tc.descriptors[key]; exists {
			tc.mutex.RUnlock()
			return desc, nil
		}
		tc.mutex.RUnlock()
	}

	// If not in cache, build and cache it (write lock)
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	return tc.getTypeDescriptor(runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
}

// getTypeDescriptor returns a cached type descriptor for a (runtime, schema) pair.
// When runtimeType == schemaType, this is the standard descriptor building.
// When they differ, it handles view descriptors where schema defines SSZ layout.
func (tc *TypeCache) getTypeDescriptor(runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	key := typeKey{runtime: runtimeType, schema: schemaType}
	cacheable := len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0

	if desc, exists := tc.descriptors[key]; exists && cacheable {
		return desc, nil
	}

	desc, err := tc.buildTypeDescriptor(runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
	if err != nil {
		return nil, err
	}

	// Cache only if no size hints (cacheable)
	if cacheable {
		tc.descriptors[key] = desc
	}

	return desc, nil
}

func (tc *TypeCache) getCompatFlag(runtimeType, schemaType reflect.Type) SszCompatFlag {
	runtimeTypeName := runtimeType.Name()
	runtimeTypePkgPath := runtimeType.PkgPath()
	if runtimeTypePkgPath == "" && runtimeType.Kind() == reflect.Ptr {
		runtimeTypePkgPath = runtimeType.Elem().PkgPath()
	}

	runtimeTypeKey := runtimeTypeName
	if runtimeTypePkgPath != "" {
		runtimeTypeKey = runtimeTypePkgPath + "." + runtimeTypeName
	}

	schemaTypeName := schemaType.Name()
	schemaTypePkgPath := schemaType.PkgPath()
	if schemaTypePkgPath == "" && schemaType.Kind() == reflect.Ptr {
		schemaTypePkgPath = schemaType.Elem().PkgPath()
	}

	schemaTypeKey := schemaTypeName
	if schemaTypePkgPath != "" {
		schemaTypeKey = schemaTypePkgPath + "." + schemaTypeName
	}

	if runtimeTypeKey != schemaTypeKey {
		runtimeTypeKey = fmt.Sprintf("%v|%v", runtimeTypeKey, schemaTypeKey)
	}

	return tc.CompatFlags[runtimeTypeKey]
}

// buildTypeDescriptor computes a type descriptor for a (runtime, schema) type pair.
//
// When runtimeType == schemaType, this produces a standard descriptor.
// When they differ (view descriptor scenario), the schema type defines the SSZ layout
// (field order, tags, annotations) while the runtime type provides the actual data storage.
//
// The descriptor's Type field stores the runtime type (where data lives), while the
// SSZ structure (fields, sizes, limits) is derived from the schema type.
func (tc *TypeCache) buildTypeDescriptor(runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	// Use runtime type for the descriptor's Type field (where data is accessed)
	desc := &TypeDescriptor{
		Type:       runtimeType,
		SchemaType: schemaType,
	}

	// Verify runtime and schema types have compatible base kinds
	if runtimeType != schemaType {
		if runtimeType.Kind() != schemaType.Kind() {
			return nil, fmt.Errorf("incompatible types: runtime kind %v != schema kind %v", runtimeType.Kind(), schemaType.Kind())
		}

		var view any
		if schemaType.Kind() == reflect.Ptr {
			view = reflect.Zero(schemaType).Interface()
		} else {
			view = reflect.Zero(reflect.PointerTo(schemaType)).Interface()
		}
		desc.CodegenInfo = &view
	}

	// Handle pointer types - dereference both runtime and schema
	if schemaType.Kind() == reflect.Ptr {
		desc.GoTypeFlags |= GoTypeFlagIsPointer
		schemaType = schemaType.Elem()
		runtimeType = runtimeType.Elem()

		if runtimeType != schemaType && runtimeType.Kind() != schemaType.Kind() {
			return nil, fmt.Errorf("incompatible pointer types: runtime kind %v != schema kind %v", runtimeType.Kind(), schemaType.Kind())
		}
	}

	// Use schema type for determining the SSZ layout
	t := schemaType

	desc.Kind = t.Kind()

	// check dynamic size and max size
	if len(sizeHints) > 0 {
		if sizeHints[0].Expr != "" {
			desc.SizeExpression = &sizeHints[0].Expr
		}
		if sizeHints[0].Bits {
			desc.SszTypeFlags |= SszTypeFlagHasBitSize
		}
		for _, hint := range sizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= SszTypeFlagHasDynamicSize
			}

			if hint.Expr != "" {
				desc.SszTypeFlags |= SszTypeFlagHasSizeExpr
			}
		}
	}

	if len(maxSizeHints) > 0 {
		if !maxSizeHints[0].NoValue {
			desc.SszTypeFlags |= SszTypeFlagHasLimit
			desc.Limit = maxSizeHints[0].Size
		}

		if maxSizeHints[0].Expr != "" {
			desc.MaxExpression = &maxSizeHints[0].Expr
		}

		for _, hint := range maxSizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= SszTypeFlagHasDynamicMax
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= SszTypeFlagHasMaxExpr
			}
		}
	}

	// determine ssz type
	sszType := SszUnspecifiedType
	if len(typeHints) > 0 {
		sszType = typeHints[0].Type
	}

	if desc.Kind == reflect.String {
		desc.GoTypeFlags |= GoTypeFlagIsString
	}
	if t.PkgPath() == "time" && t.Name() == "Time" {
		desc.GoTypeFlags |= GoTypeFlagIsTime
	}

	// auto-detect ssz type if not specified
	if sszType == SszUnspecifiedType {
		// detect some well-known and widely used types
		switch {
		case t.PkgPath() == "time" && t.Name() == "Time":
			sszType = SszUint64Type
		case t.PkgPath() == "github.com/holiman/uint256" && t.Name() == "Int":
			sszType = SszUint256Type
		case t.PkgPath() == "github.com/prysmaticlabs/go-bitfield" && t.Name() == "Bitlist":
			sszType = SszBitlistType
		case t.PkgPath() == "github.com/OffchainLabs/go-bitfield" && t.Name() == "Bitlist":
			sszType = SszBitlistType
		case t.PkgPath() == "github.com/pk910/dynamic-ssz" && strings.HasPrefix(t.Name(), "CompatibleUnion["):
			sszType = SszCompatibleUnionType
		case t.PkgPath() == "github.com/pk910/dynamic-ssz" && strings.HasPrefix(t.Name(), "TypeWrapper["):
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
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("bool ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("bool ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return nil, fmt.Errorf("uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint8 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("uint8 ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return nil, fmt.Errorf("uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint16 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 2 {
			return nil, fmt.Errorf("uint16 ssz type must be ssz-size:2, got %v", sizeHints[0].Size)
		}
		desc.Size = 2
	case SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return nil, fmt.Errorf("uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint32 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 4 {
			return nil, fmt.Errorf("uint32 ssz type must be ssz-size:4, got %v", sizeHints[0].Size)
		}
		desc.Size = 4
	case SszUint64Type:
		if desc.Kind != reflect.Uint64 && desc.GoTypeFlags&GoTypeFlagIsTime == 0 {
			return nil, fmt.Errorf("uint64 ssz type can only be represented by uint64 or time.Time types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint64 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return nil, fmt.Errorf("uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
		}
		desc.Size = 8
	case SszUint128Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint128 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := tc.buildUint128Descriptor(desc, t) // handle as [16]uint8 or [2]uint64
		if err != nil {
			return nil, err
		}
	case SszUint256Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint256 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := tc.buildUint256Descriptor(desc, t) // handle as [32]uint8 or [4]uint64
		if err != nil {
			return nil, err
		}

	// complex types
	case SszTypeWrapperType:
		err := tc.buildTypeWrapperDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return nil, err
		}
	case SszContainerType, SszProgressiveContainerType:
		err := tc.buildContainerDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return nil, err
		}
	case SszVectorType, SszBitvectorType:
		err := tc.buildVectorDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszListType, SszBitlistType, SszProgressiveListType, SszProgressiveBitlistType:
		err := tc.buildListDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszCompatibleUnionType:
		err := tc.buildCompatibleUnionDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return nil, err
		}
	case SszCustomType:
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			desc.Size = uint32(sizeHints[0].Size)
		} else {
			desc.Size = 0
			desc.SszTypeFlags |= SszTypeFlagIsDynamic
		}
	}

	if desc.SszTypeFlags&SszTypeFlagHasBitSize != 0 && (desc.SszType != SszBitvectorType && desc.SszType != SszBitlistType) {
		return nil, fmt.Errorf("bit size tag is only allowed for bitvector or bitlist types, got %v", desc.SszType)
	}

	if desc.SszTypeFlags&SszTypeFlagHasDynamicSize == 0 && getFastsszConvertCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagFastSSZMarshaler
	}
	if desc.SszTypeFlags&SszTypeFlagHasDynamicMax == 0 {
		if getFastsszHashCompatibility(runtimeType) {
			desc.SszCompatFlags |= SszCompatFlagFastSSZHasher
		}
		if method := getHashTreeRootWithCompatibility(runtimeType); method != nil {
			desc.HashTreeRootWithMethod = method
			desc.SszCompatFlags |= SszCompatFlagHashTreeRootWith
		}
	}

	// Check for dynamic interface implementations
	if getDynamicMarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicMarshaler
	}
	if getDynamicUnmarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicUnmarshaler
	}
	if getDynamicEncoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicEncoder
	}
	if getDynamicDecoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicDecoder
	}
	if getDynamicSizerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicSizer
	}
	if getDynamicHashRootCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicHashRoot
	}

	// Check for dynamic view interface implementations (for fork-dependent SSZ schemas).
	// View interfaces are checked on runtimeType because the methods are implemented
	// on the runtime type, while schemaType only defines the SSZ layout.
	if getDynamicViewMarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewMarshaler
	}
	if getDynamicViewUnmarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewUnmarshaler
	}
	if getDynamicViewEncoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewEncoder
	}
	if getDynamicViewDecoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewDecoder
	}
	if getDynamicViewSizerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewSizer
	}
	if getDynamicViewHashRootCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewHashRoot
	}

	desc.SszCompatFlags |= tc.getCompatFlag(runtimeType, schemaType)

	if desc.SszType == SszCustomType {
		isCompatible := desc.SszCompatFlags&SszCompatFlagFastSSZMarshaler != 0 && desc.SszCompatFlags&SszCompatFlagFastSSZHasher != 0
		//isCompatible = isCompatible || (desc.SszCompatFlags&SszCompatFlagDynamicMarshaler != 0 && desc.SszCompatFlags&SszCompatFlagDynamicUnmarshaler != 0 && desc.SszCompatFlags&SszCompatFlagDynamicSizer != 0 && desc.SszCompatFlags&SszCompatFlagDynamicHashRoot != 0)
		if !isCompatible {
			return nil, fmt.Errorf("custom ssz type requires fastssz marshaler, unmarshaler and hasher implementations")
		}
	}

	return desc, nil
}

// buildTypeWrapperDescriptor builds a descriptor for TypeWrapper types with runtime/schema pairing.
//
// For TypeWrappers, the wrapped type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ annotations while the runtime type provides actual data access.
func (tc *TypeCache) buildTypeWrapperDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return fmt.Errorf("TypeWrapper ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	// Extract schema wrapper information (determines SSZ layout)
	schemaWrapperValue := reflect.New(schemaType)
	schemaMethod := schemaWrapperValue.MethodByName("GetDescriptorType")
	if !schemaMethod.IsValid() {
		return fmt.Errorf("GetDescriptorType method not found on schema type %s", schemaType)
	}

	schemaResults := schemaMethod.Call(nil)
	if len(schemaResults) == 0 {
		return fmt.Errorf("GetDescriptorType returned no results for schema type")
	}

	schemaDescriptorType, ok := schemaResults[0].Interface().(reflect.Type)
	if !ok {
		return fmt.Errorf("GetDescriptorType did not return a reflect.Type for schema type")
	}

	// Extract wrapper information from schema descriptor (includes SSZ annotations)
	schemaWrapperInfo, err := extractWrapperDescriptorInfo(schemaDescriptorType, tc.specs)
	if err != nil {
		return fmt.Errorf("failed to extract schema wrapper descriptor info: %w", err)
	}

	// Determine runtime wrapped type
	var runtimeWrappedType reflect.Type
	if runtimeType != schemaType {
		// Extract runtime wrapper information for the wrapped type
		runtimeWrapperValue := reflect.New(runtimeType)
		runtimeMethod := runtimeWrapperValue.MethodByName("GetDescriptorType")
		if !runtimeMethod.IsValid() {
			return fmt.Errorf("GetDescriptorType method not found on runtime type %s", runtimeType)
		}

		runtimeResults := runtimeMethod.Call(nil)
		if len(runtimeResults) == 0 {
			return fmt.Errorf("GetDescriptorType returned no results for runtime type")
		}

		runtimeDescriptorType, ok := runtimeResults[0].Interface().(reflect.Type)
		if !ok {
			return fmt.Errorf("GetDescriptorType did not return a reflect.Type for runtime type")
		}

		// Get the wrapped type from runtime descriptor
		runtimeWrapperInfo, err := extractWrapperDescriptorInfo(runtimeDescriptorType, tc.specs)
		if err != nil {
			return fmt.Errorf("failed to extract runtime wrapper descriptor info: %w", err)
		}
		runtimeWrappedType = runtimeWrapperInfo.Type
	} else {
		runtimeWrappedType = schemaWrapperInfo.Type
	}

	// Build type descriptor for the wrapped type traversing both type trees
	wrappedDesc, err := tc.getTypeDescriptor(runtimeWrappedType, schemaWrapperInfo.Type, schemaWrapperInfo.SizeHints, schemaWrapperInfo.MaxSizeHints, schemaWrapperInfo.TypeHints)
	if err != nil {
		return fmt.Errorf("failed to build descriptor for wrapped type: %w", err)
	}

	// Store wrapper information
	desc.ElemDesc = wrappedDesc

	// The TypeWrapper inherits properties from the wrapped type
	desc.Size = wrappedDesc.Size
	desc.SszTypeFlags |= wrappedDesc.SszTypeFlags & (SszTypeFlagIsDynamic | SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

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
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, fieldType, nil, nil, nil)
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
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, fieldType, nil, nil, nil)
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

// buildContainerDescriptor builds a descriptor for ssz container types with runtime/schema pairing.
//
// This method supports fork-dependent SSZ schemas (view descriptors) where the schema type
// defines the SSZ layout (field order, tags, limits) while the runtime type holds the actual data.
//
// When runtimeType == schemaType, this behaves identically to the original buildContainerDescriptor.
// When they differ, the method:
//   - Iterates over schema fields to define SSZ layout
//   - Maps each schema field to the corresponding runtime field by name
//   - Stores the runtime field index in FieldIndex for direct field access
//   - Builds child descriptors using (runtimeFieldType, schemaFieldType) pairs
//
// Parameters:
//   - desc: The TypeDescriptor being built
//   - runtimeType: The type where actual data lives (must be a struct)
//   - schemaType: The type that defines SSZ layout (must be a struct)
//
// Returns:
//   - error: An error if schema fields cannot be mapped to runtime fields
func (tc *TypeCache) buildContainerDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return fmt.Errorf("container ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	// Determine if runtime and schema types differ (view descriptor mode)
	isViewDescriptor := runtimeType != schemaType

	// Pre-build a map of runtime field names to indices for efficient lookup in view descriptor mode
	var runtimeFieldMap map[string]int
	if isViewDescriptor {
		runtimeFieldMap = make(map[string]int, runtimeType.NumField())
		for i := 0; i < runtimeType.NumField(); i++ {
			runtimeFieldMap[runtimeType.Field(i).Name] = i
		}
	}

	desc.ContainerDesc = &ContainerDescriptor{
		Fields:    make([]FieldDescriptor, schemaType.NumField()),
		DynFields: make([]DynFieldDescriptor, 0),
	}

	totalSize := uint32(0)
	isDynamic := false

	// Check for progressive container detection
	hasAnyIndexTag := false
	fieldIndices := make(map[uint16]bool)
	sszIndexes := make([]*uint16, schemaType.NumField())

	// Iterate over schema fields - they define the SSZ layout
	for i := 0; i < schemaType.NumField(); i++ {
		schemaField := schemaType.Field(i)
		fieldDesc := FieldDescriptor{
			Name: schemaField.Name,
		}

		// Resolve the corresponding runtime field
		var runtimeFieldIndex int

		if isViewDescriptor {
			// In view descriptor mode, map schema field to runtime field by name
			idx, found := runtimeFieldMap[schemaField.Name]
			if !found {
				return fmt.Errorf("schema field %q not found in runtime type %s", schemaField.Name, runtimeType.Name())
			}
			runtimeFieldIndex = idx
		} else {
			// When schema == runtime, field indices match directly
			runtimeFieldIndex = i
		}

		// Store the runtime field index for direct field access during encode/decode/hash
		fieldDesc.FieldIndex = uint16(runtimeFieldIndex)
		runtimeFieldType := runtimeType.Field(runtimeFieldIndex).Type

		// Get ssz-index tag from schema field (for progressive containers)
		sszIndex, err := getSszIndexTag(&schemaField)
		if err != nil {
			return err
		}

		if sszIndex != nil {
			sszIndexes[i] = sszIndex
			fieldDesc.SszIndex = *sszIndex
			hasAnyIndexTag = true
			if fieldIndices[*sszIndex] {
				return fmt.Errorf("duplicate ssz-index %d found in field %s", *sszIndex, schemaField.Name)
			}
			fieldIndices[*sszIndex] = true
		}

		// Get size hints from schema field tags (schema defines SSZ constraints)
		sizeHints, err := getSszSizeTag(tc.specs, &schemaField)
		if err != nil {
			return err
		}

		maxSizeHints, err := getSszMaxSizeTag(tc.specs, &schemaField)
		if err != nil {
			return err
		}

		typeHints, err := getSszTypeTag(&schemaField)
		if err != nil {
			return err
		}

		// Build child type descriptor using (runtimeFieldType, schemaFieldType) pair.
		// This is the key to supporting nested view descriptors: the schema field type
		// may itself be a view type that differs from the runtime field type.
		schemaFieldType := schemaField.Type
		fieldDesc.Type, err = tc.getTypeDescriptor(runtimeFieldType, schemaFieldType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return fmt.Errorf("failed to build descriptor for field %s: %w", schemaField.Name, err)
		}

		sszSize := fieldDesc.Type.Size
		if fieldDesc.Type.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
			isDynamic = true
			sszSize = 4 // Offset size for dynamic fields

			desc.ContainerDesc.DynFields = append(desc.ContainerDesc.DynFields, DynFieldDescriptor{
				Field:        &desc.ContainerDesc.Fields[i],
				HeaderOffset: uint32(totalSize),
				Index:        int16(runtimeFieldIndex), // Use runtime field index for data access
			})
		}

		desc.SszTypeFlags |= fieldDesc.Type.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)
		totalSize += sszSize
		desc.ContainerDesc.Fields[i] = fieldDesc
	}

	// Determine if this is a progressive container
	// A container is progressive if it has ssz-index annotations on fields
	// If it's progressive, all fields must have increasing ssz-index tags
	if hasAnyIndexTag {
		// For progressive containers, ensure all ssz-index values are properly set
		// and validate they are in increasing order
		for i := 0; i < len(desc.ContainerDesc.Fields); i++ {
			field := &desc.ContainerDesc.Fields[i]
			if sszIndex := sszIndexes[i]; sszIndex == nil {
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

	desc.Len = totalSize
	if isDynamic {
		desc.Size = 0
		desc.SszTypeFlags |= SszTypeFlagIsDynamic
	} else {
		desc.Size = totalSize
	}

	return nil
}

// buildCompatibleUnionDescriptor builds a descriptor for CompatibleUnion types with runtime/schema pairing.
//
// For CompatibleUnion types, the variant types may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (variant order, annotations) while the runtime type provides
// the actual variant types for data access.
func (tc *TypeCache) buildCompatibleUnionDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	// CompatibleUnion is always dynamic size (1 byte for type + variable data)
	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	// Extract the schema descriptor type from the generic type parameter (determines SSZ layout)
	schemaDescriptorType, err := tc.extractGenericTypeParameter(schemaType)
	if err != nil {
		return err
	}

	// Populate union variants immediately since we have the descriptor type
	desc.UnionVariants = make(map[uint8]*TypeDescriptor)

	// Extract variant information from schema descriptor struct (includes SSZ annotations)
	schemaVariantInfo, err := extractUnionDescriptorInfo(schemaDescriptorType, tc.specs)
	if err != nil {
		return fmt.Errorf("failed to extract union variant info from schema: %w", err)
	}

	// Check if we're using a view descriptor (runtime and schema types differ)
	isViewDescriptor := runtimeType != schemaType

	// Extract runtime variant info if using view descriptor
	var runtimeVariantMap map[string]reflect.Type
	if isViewDescriptor {
		runtimeDescriptorType, err := tc.extractGenericTypeParameter(runtimeType)
		if err != nil {
			return fmt.Errorf("failed to extract runtime union descriptor type: %w", err)
		}
		runtimeVariantInfo, err := extractUnionDescriptorInfo(runtimeDescriptorType, tc.specs)
		if err != nil {
			return fmt.Errorf("failed to extract union variant info from runtime: %w", err)
		}
		// Build map of runtime variant names to types
		runtimeVariantMap = make(map[string]reflect.Type, len(runtimeVariantInfo))
		for _, info := range runtimeVariantInfo {
			runtimeVariantMap[info.Name] = info.Type
		}
	}

	// Build type descriptors for each variant using schema for layout, runtime for data
	for variantIndex, schemaInfo := range schemaVariantInfo {
		var runtimeVariantType reflect.Type
		if isViewDescriptor {
			var ok bool
			runtimeVariantType, ok = runtimeVariantMap[schemaInfo.Name]
			if !ok {
				return fmt.Errorf("runtime union missing variant %q defined in schema", schemaInfo.Name)
			}
		} else {
			runtimeVariantType = schemaInfo.Type
		}

		variantDesc, err := tc.getTypeDescriptor(runtimeVariantType, schemaInfo.Type, schemaInfo.SizeHints, schemaInfo.MaxSizeHints, schemaInfo.TypeHints)
		if err != nil {
			return fmt.Errorf("failed to build descriptor for union variant %d: %w", variantIndex, err)
		}

		desc.UnionVariants[variantIndex] = variantDesc
	}

	return nil
}

// buildVectorDescriptor builds a descriptor for ssz vector types with runtime/schema pairing.
//
// For vectors, the element type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (element type, size hints) while the runtime type provides
// the actual element type for data access.
func (tc *TypeCache) buildVectorDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	// Use schema type for SSZ layout determination
	t := schemaType

	if desc.Kind != reflect.Array && desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return fmt.Errorf("vector ssz type can only be represented by array or slice types, got %v", desc.Kind)
	}

	if desc.Kind == reflect.Array {
		desc.Len = uint32(t.Len())
		if len(sizeHints) > 0 {
			byteLen := sizeHints[0].Size
			if sizeHints[0].Bits {
				desc.BitSize = sizeHints[0].Size
				byteLen = (byteLen + 7) / 8 // ceil up to the next multiple of 8
			}
			if byteLen > desc.Len {
				return fmt.Errorf("size hint for vector type is greater than the length of the array (%d > %d)", byteLen, desc.Len)
			}
			desc.Len = uint32(byteLen)
		}
	} else if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
		byteLen := sizeHints[0].Size
		if sizeHints[0].Bits {
			desc.BitSize = sizeHints[0].Size
			byteLen = (byteLen + 7) / 8 // ceil up to the next multiple of 8
		}
		desc.Len = uint32(byteLen)
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

	// Determine element types for runtime and schema
	var runtimeElemType, schemaElemType reflect.Type
	if desc.Kind == reflect.String {
		// Strings are treated as []byte
		runtimeElemType = byteType
		schemaElemType = byteType
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	} else {
		// Get element type from both runtime and schema types
		schemaElemType = t.Elem()
		runtimeElemType = runtimeType.Elem()
		if schemaElemType == byteType {
			desc.GoTypeFlags |= GoTypeFlagIsByteArray
		}
	}

	// Build element descriptor using (runtimeElemType, schemaElemType) pair
	// This supports nested view descriptors within vector elements
	elemDesc, err := tc.getTypeDescriptor(runtimeElemType, schemaElemType, childSizeHints, childMaxSizeHints, childTypeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	if desc.SszType == SszBitvectorType && desc.ElemDesc.Kind != reflect.Uint8 {
		return fmt.Errorf("bitvector ssz type can only be represented by byte slices or arrays, got %v", desc.ElemDesc.Kind.String())
	}

	if elemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
		desc.Size = 0
		desc.SszTypeFlags |= SszTypeFlagIsDynamic
	} else {
		desc.Size = elemDesc.Size * desc.Len
	}

	return nil
}

// buildListDescriptor builds a descriptor for ssz list types with runtime/schema pairing.
//
// For lists, the element type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (element type, size hints) while the runtime type provides
// the actual element type for data access.
func (tc *TypeCache) buildListDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	// Use schema type for SSZ layout determination
	t := schemaType

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

	// Determine element types for runtime and schema
	var runtimeElemType, schemaElemType reflect.Type
	if desc.Kind == reflect.String {
		// Strings are treated as []byte
		runtimeElemType = byteType
		schemaElemType = byteType
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	} else {
		// Get element type from both runtime and schema types
		schemaElemType = t.Elem()
		runtimeElemType = runtimeType.Elem()
		if schemaElemType == byteType {
			desc.GoTypeFlags |= GoTypeFlagIsByteArray
		}
	}

	// Build element descriptor using (runtimeElemType, schemaElemType) pair
	// This supports nested view descriptors within list elements
	elemDesc, err := tc.getTypeDescriptor(runtimeElemType, schemaElemType, childSizeHints, childMaxSizeHints, childTypeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	if desc.SszType == SszBitlistType || desc.SszType == SszProgressiveBitlistType {
		if desc.Kind != reflect.Slice {
			return fmt.Errorf("bitlist ssz type can only be represented by byte slices, got %v", desc.Kind.String())
		}
		if desc.ElemDesc.Kind != reflect.Uint8 {
			return fmt.Errorf("bitlist ssz type can only be represented by byte slices, got []%v", desc.ElemDesc.Kind.String())
		}
	}

	if len(sizeHints) > 0 && sizeHints[0].Size > 0 && !sizeHints[0].Dynamic {
		if elemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
			desc.Size = 0 // Dynamic elements = dynamic size
			desc.SszTypeFlags |= SszTypeFlagIsDynamic
		} else {
			byteLen := sizeHints[0].Size
			if sizeHints[0].Bits {
				desc.BitSize = sizeHints[0].Size
				byteLen = (byteLen + 7) / 8 // ceil up to the next multiple of 8
			}
			desc.Size = elemDesc.Size * byteLen
		}
	} else {
		desc.Size = 0 // Dynamic slice
		desc.SszTypeFlags |= SszTypeFlagIsDynamic
	}

	return nil
}

// GetAllTypes returns a slice of all type keys currently cached in the TypeCache.
//
// This method is useful for cache inspection, debugging, and understanding which types
// have been processed and cached during the application's lifetime. The returned slice
// contains pairs of (runtime, schema) types in no particular order.
//
// When runtime == schema (normal usage), these represent standard type descriptors.
// When they differ, the pair represents a view descriptor for fork-dependent SSZ.
//
// The method acquires a read lock to ensure thread-safe access to the cache.
//
// Returns:
//   - [][2]reflect.Type: A slice of [runtime, schema] type pairs
//
// Example:
//
//	cachedTypes := cache.GetAllTypes()
//	fmt.Printf("TypeCache contains %d type pairs\n", len(cachedTypes))
//	for _, pair := range cachedTypes {
//	    if pair[0] == pair[1] {
//	        fmt.Printf("  - %s\n", pair[0].String())
//	    } else {
//	        fmt.Printf("  - %s (view: %s)\n", pair[0].String(), pair[1].String())
//	    }
//	}
func (tc *TypeCache) GetAllTypes() [][2]reflect.Type {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	types := make([][2]reflect.Type, 0, len(tc.descriptors))
	for key := range tc.descriptors {
		types = append(types, [2]reflect.Type{key.runtime, key.schema})
	}

	return types
}

// RemoveType removes a specific type (with runtime == schema) from the cache.
//
// This method is useful for cache management scenarios where you need to force
// recomputation of a type descriptor, such as after configuration changes or
// when testing different type configurations.
//
// The method acquires a write lock to ensure thread-safe removal.
// Note: This only removes entries where runtime type equals schema type.
// Use RemoveTypeKey to remove view descriptor entries.
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
	tc.RemoveTypeKey(t, t)
}

// RemoveTypeKey removes a specific (runtime, schema) type pair from the cache.
//
// This method supports removing view descriptor entries where runtime differs from schema.
//
// Parameters:
//   - runtimeType: The runtime type of the cache entry to remove
//   - schemaType: The schema type of the cache entry to remove
func (tc *TypeCache) RemoveTypeKey(runtimeType, schemaType reflect.Type) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	if runtimeType.Kind() == reflect.Ptr {
		runtimeType = runtimeType.Elem()
	}
	if schemaType.Kind() == reflect.Ptr {
		schemaType = schemaType.Elem()
	}

	delete(tc.descriptors, typeKey{runtime: runtimeType, schema: schemaType})
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
	tc.descriptors = make(map[typeKey]*TypeDescriptor)
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

func (td *TypeDescriptor) GetTypeHash() ([32]byte, error) {
	jsonDesc, err := json.Marshal(td)
	if err != nil {
		return [32]byte{}, err
	}

	hash := sha256.Sum256(jsonDesc)
	return hash, nil
}
