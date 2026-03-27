// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"sync"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// TypeCache manages cached type descriptors
type TypeCache struct {
	specs         sszutils.DynamicSpecs
	mutex         sync.RWMutex
	descriptors   map[reflect.Type]*TypeDescriptor
	CompatFlags   map[string]SszCompatFlag
	ExtendedTypes bool
}

// NewTypeCache creates a new type cache
func NewTypeCache(specs sszutils.DynamicSpecs) *TypeCache {
	return &TypeCache{
		specs:         specs,
		descriptors:   make(map[reflect.Type]*TypeDescriptor),
		CompatFlags:   map[string]SszCompatFlag{},
		ExtendedTypes: false,
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

func (tc *TypeCache) getCompatFlag(t reflect.Type) SszCompatFlag {
	typeName := t.Name()
	typePkgPath := t.PkgPath()
	if typePkgPath == "" && t.Kind() == reflect.Ptr {
		typePkgPath = t.Elem().PkgPath()
	}

	typeKey := typeName
	if typePkgPath != "" {
		typeKey = typePkgPath + "." + typeName
	}

	return tc.CompatFlags[typeKey]
}

// buildTypeDescriptor computes a type descriptor for the given type
//
//nolint:gocyclo // SSZ type descriptor builder is inherently complex
func (tc *TypeCache) buildTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	desc := &TypeDescriptor{
		Type: t,
	}

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		desc.GoTypeFlags |= GoTypeFlagIsPointer
		t = t.Elem()
	}

	// Track whether hints were provided externally (from field tags) rather than
	// from the annotation registry. External hints override the type's own annotation,
	// so we must not delegate to generated methods that have the annotation baked in.
	hasExternalHints := len(sizeHints) > 0 || len(maxSizeHints) > 0

	// Check annotation registry for type-level metadata when no external hints provided
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
		if tag, ok := sszutils.LookupAnnotation(t); ok {
			var parseErr error

			typeHints, sizeHints, maxSizeHints, parseErr = ParseTags(tag)
			if parseErr != nil {
				return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "failed to parse annotation for type %v: %v", t, parseErr)
			}

			// ParseTags can't resolve dynamic expressions (no DynamicSpecs).
			// Resolve them now using tc.specs.
			if tc.specs != nil {
				for i := range sizeHints {
					if sizeHints[i].Expr != "" {
						if ok, val, err := tc.specs.ResolveSpecValue(sizeHints[i].Expr); err == nil && ok {
							sizeHints[i].Size = uint32(val)
							sizeHints[i].Custom = true
						}
					}
				}

				for i := range maxSizeHints {
					if maxSizeHints[i].Expr != "" {
						if ok, val, err := tc.specs.ResolveSpecValue(maxSizeHints[i].Expr); err == nil && ok {
							maxSizeHints[i].Size = val
							maxSizeHints[i].Custom = true
						}
					}
				}
			}
		}
	}

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
		sszType = getWellKnownExternalType(t.PkgPath(), t.Name())
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

		// extended types (not supported by SSZ spec)
		case reflect.Int8:
			sszType = SszInt8Type
		case reflect.Int16:
			sszType = SszInt16Type
		case reflect.Int32:
			sszType = SszInt32Type
		case reflect.Int64:
			sszType = SszInt64Type
		case reflect.Float32:
			sszType = SszFloat32Type
		case reflect.Float64:
			sszType = SszFloat64Type

		// unsupported types
		case reflect.Int, reflect.Uint:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "signed or unsigned integers with unspecified size are not supported in SSZ")
		case reflect.Complex64, reflect.Complex128:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "complex numbers are not supported in SSZ (use unsigned integers instead)")
		case reflect.Map:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "maps are not supported in SSZ (use structs or arrays instead)")
		case reflect.Chan:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "channels are not supported in SSZ")
		case reflect.Func:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "functions are not supported in SSZ")
		case reflect.Interface:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "interfaces are not supported in SSZ (use concrete types)")
		case reflect.UnsafePointer:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "unsafe pointers are not supported in SSZ")
		default:
			break
		}

		// special case for bitlists
		if sszType == SszListType && strings.Contains(t.Name(), "Bitlist") {
			sszType = SszBitlistType
		}
	}

	desc.SszType = sszType

	// Check type compatibility and compute size
	switch sszType {
	case SszUnspecifiedType:
		return nil, sszutils.NewSszErrorf(sszutils.ErrUnsupportedType, "unsupported type kind: %v", t.Kind())

	// basic types
	case SszBoolType:
		if desc.Kind != reflect.Bool {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bool ssz type can only be represented by bool types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "bool ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "bool ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint8 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint8 ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint16 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 2 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint16 ssz type must be ssz-size:2, got %v", sizeHints[0].Size)
		}
		desc.Size = 2
	case SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint32 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 4 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint32 ssz type must be ssz-size:4, got %v", sizeHints[0].Size)
		}
		desc.Size = 4
	case SszUint64Type:
		if desc.Kind != reflect.Uint64 && desc.GoTypeFlags&GoTypeFlagIsTime == 0 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint64 ssz type can only be represented by uint64 or time.Time types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint64 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
		}
		desc.Size = 8
	case SszUint128Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint128 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := tc.buildUintDescriptor(desc, t, 16, "uint128") // handle as [16]uint8 or [2]uint64
		if err != nil {
			return nil, err
		}
	case SszUint256Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint256 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := tc.buildUintDescriptor(desc, t, 32, "uint256") // handle as [32]uint8 or [4]uint64
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
			desc.Size = sizeHints[0].Size
		} else {
			desc.Size = 0
			desc.SszTypeFlags |= SszTypeFlagIsDynamic
		}

	// extended types (not supported by SSZ spec)
	case SszInt8Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int8 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int8 ssz type can only be represented by int8 types, got %v", desc.Kind)
		}
		desc.Size = 1
	case SszInt16Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int16 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int16 ssz type can only be represented by int16 types, got %v", desc.Kind)
		}
		desc.Size = 2
	case SszInt32Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int32 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int32 ssz type can only be represented by int32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case SszInt64Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int64 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int64 ssz type can only be represented by int64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case SszFloat32Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float32 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "float32 ssz type can only be represented by float32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case SszFloat64Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float64 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "float64 ssz type can only be represented by float64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case SszOptionalType:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "optional types are not supported in SSZ (use extended types option to enable it)")
		}
		err := tc.buildOptionalDescriptor(desc, t, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszBigIntType:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "big integers are not supported in SSZ (use extended types option to enable it)")
		}
		err := tc.buildBigIntDescriptor(desc)
		if err != nil {
			return nil, err
		}
	}

	if desc.SszTypeFlags&SszTypeFlagHasBitSize != 0 && (desc.SszType != SszBitvectorType && desc.SszType != SszBitlistType) {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "bit size tag is only allowed for bitvector or bitlist types, got %v", desc.SszType)
	}

	if desc.SszTypeFlags&SszTypeFlagHasDynamicSize == 0 && getFastsszConvertCompatibility(t) {
		desc.SszCompatFlags |= SszCompatFlagFastSSZMarshaler
	}
	if desc.SszTypeFlags&SszTypeFlagHasDynamicMax == 0 {
		if getFastsszHashCompatibility(t) {
			desc.SszCompatFlags |= SszCompatFlagFastSSZHasher
		}
		if method := getHashTreeRootWithCompatibility(t); method != nil {
			desc.HashTreeRootWithMethod = method
			desc.SszCompatFlags |= SszCompatFlagHashTreeRootWith
		}
	}

	// Check for dynamic interface implementations
	if getDynamicMarshalerCompatibility(t) {
		desc.SszCompatFlags |= SszCompatFlagDynamicMarshaler
	}
	if getDynamicUnmarshalerCompatibility(t) {
		desc.SszCompatFlags |= SszCompatFlagDynamicUnmarshaler
	}
	if getDynamicEncoderCompatibility(t) {
		desc.SszCompatFlags |= SszCompatFlagDynamicEncoder
	}
	if getDynamicDecoderCompatibility(t) {
		desc.SszCompatFlags |= SszCompatFlagDynamicDecoder
	}
	if getDynamicSizerCompatibility(t) {
		desc.SszCompatFlags |= SszCompatFlagDynamicSizer
	}
	if getDynamicHashRootCompatibility(t) {
		desc.SszCompatFlags |= SszCompatFlagDynamicHashRoot
	}

	desc.SszCompatFlags |= tc.getCompatFlag(t)

	// When field-level hints override the type's own annotation, don't delegate
	// to the type's generated methods — they have the annotation's limits baked in.
	// Process inline instead so the field-level hints are respected.
	if hasExternalHints && desc.SszType != SszCustomType {
		desc.SszCompatFlags &^= SszCompatFlagDynamicMarshaler |
			SszCompatFlagDynamicUnmarshaler |
			SszCompatFlagDynamicSizer |
			SszCompatFlagDynamicHashRoot |
			SszCompatFlagDynamicEncoder |
			SszCompatFlagDynamicDecoder |
			SszCompatFlagFastSSZMarshaler |
			SszCompatFlagFastSSZHasher |
			SszCompatFlagHashTreeRootWith
	}

	if desc.SszType == SszCustomType {
		isCompatible := desc.SszCompatFlags&SszCompatFlagFastSSZMarshaler != 0 && desc.SszCompatFlags&SszCompatFlagFastSSZHasher != 0
		// isCompatible = isCompatible || (desc.SszCompatFlags&SszCompatFlagDynamicMarshaler != 0 && desc.SszCompatFlags&SszCompatFlagDynamicUnmarshaler != 0 && desc.SszCompatFlags&SszCompatFlagDynamicSizer != 0 && desc.SszCompatFlags&SszCompatFlagDynamicHashRoot != 0)
		if !isCompatible {
			return nil, sszutils.NewSszError(sszutils.ErrMissingInterface, "custom ssz type requires fastssz marshaler, unmarshaler and hasher implementations")
		}
	}

	return desc, nil
}

// buildTypeWrapperDescriptor builds a descriptor for TypeWrapper types
func (tc *TypeCache) buildTypeWrapperDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "TypeWrapper ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	// Create a zero value instance to call the GetDescriptorType method
	wrapperValue := reflect.New(t)
	method := wrapperValue.MethodByName("GetDescriptorType")
	if !method.IsValid() {
		return sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "GetDescriptorType method not found on type %s", t)
	}

	// Call the method to get the descriptor type
	results := method.Call(nil)
	if len(results) == 0 {
		return sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType returned no results")
	}

	descriptorType, ok := results[0].Interface().(reflect.Type)
	if !ok {
		return sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType did not return a reflect.Type")
	}

	// Extract wrapper information from descriptor struct (includes SSZ annotations)
	wrapperInfo, err := extractWrapperDescriptorInfo(descriptorType, tc.specs)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(wrapper)")
	}

	// Build type descriptor for the wrapped type using the extracted information
	wrappedDesc, err := tc.getTypeDescriptor(wrapperInfo.Type, wrapperInfo.SizeHints, wrapperInfo.MaxSizeHints, wrapperInfo.TypeHints)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(wrapper)")
	}

	// Store wrapper information
	desc.ElemDesc = wrappedDesc

	// The TypeWrapper inherits properties from the wrapped type
	desc.Size = wrappedDesc.Size
	desc.SszTypeFlags |= wrappedDesc.SszTypeFlags & (SszTypeFlagIsDynamic | SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	return nil
}

// buildUint128Descriptor builds a descriptor for uint128 types
func (tc *TypeCache) buildUintDescriptor(desc *TypeDescriptor, t reflect.Type, byteLen uint32, typeName string) error {
	if desc.Kind != reflect.Slice && desc.Kind != reflect.Array {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "%s ssz type can only be represented by slice or array types, got %v", typeName, desc.Kind)
	}

	fieldType := t.Elem()
	elemKind := fieldType.Kind()
	if elemKind != reflect.Uint8 && elemKind != reflect.Uint64 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "%s ssz type can only be represented by slices or arrays of uint8 or uint64, got %v", typeName, elemKind)
	} else if elemKind == reflect.Uint8 {
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, nil, nil, nil)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.Size = byteLen
	desc.Len = desc.Size / elemDesc.Size

	if desc.Kind == reflect.Array {
		dstLen := uint32(t.Len())
		if dstLen < desc.Len {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "%s ssz type does not fit in array (%d < %d)", typeName, dstLen, desc.Len)
		}
	}

	return nil
}

// buildContainerDescriptor builds a descriptor for ssz container types
func (tc *TypeCache) buildContainerDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "container ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	fieldCount := t.NumField()
	desc.ContainerDesc = &ContainerDescriptor{
		Fields:    make([]FieldDescriptor, fieldCount),
		DynFields: make([]DynFieldDescriptor, 0),
	}

	totalSize := uint32(0)
	isDynamic := false

	// Check for progressive container detection
	hasAnyIndexTag := false
	var fieldIndices map[uint16]struct{}
	var sszIndexes []*uint16

	for i := 0; i < fieldCount; i++ {
		field := t.Field(i)
		fieldDesc := FieldDescriptor{
			Name: field.Name,
		}

		// Get ssz-index tag
		sszIndex, err := getSszIndexTag(&field)
		if err != nil {
			return sszutils.ErrorWithPath(err, field.Name)
		}

		if sszIndex != nil {
			if sszIndexes == nil {
				sszIndexes = make([]*uint16, fieldCount)
				fieldIndices = make(map[uint16]struct{}, fieldCount)
			}
			sszIndexes[i] = sszIndex
			fieldDesc.SszIndex = *sszIndex
			hasAnyIndexTag = true
			if _, exists := fieldIndices[*sszIndex]; exists {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "duplicate ssz-index %d found in field %s", *sszIndex, field.Name)
			}
			fieldIndices[*sszIndex] = struct{}{}
		}

		// Get size hints from tags
		sizeHints, err := getSszSizeTag(tc.specs, &field)
		if err != nil {
			return sszutils.ErrorWithPath(err, field.Name)
		}

		maxSizeHints, err := getSszMaxSizeTag(tc.specs, &field)
		if err != nil {
			return sszutils.ErrorWithPath(err, field.Name)
		}

		typeHints, err := getSszTypeTag(&field)
		if err != nil {
			return sszutils.ErrorWithPath(err, field.Name)
		}

		// Build type descriptor for field
		fieldDesc.Type, err = tc.getTypeDescriptor(field.Type, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return sszutils.ErrorWithPath(err, field.Name)
		}

		sszSize := fieldDesc.Type.Size
		if fieldDesc.Type.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
			isDynamic = true
			sszSize = 4 // Offset size for dynamic fields

			desc.ContainerDesc.DynFields = append(desc.ContainerDesc.DynFields, DynFieldDescriptor{
				Field:        &desc.ContainerDesc.Fields[i],
				HeaderOffset: totalSize,
				Index:        int16(i),
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
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "progressive container field %s missing ssz-index tag", field.Name)
			}
		}

		// Verify indices are increasing
		for i := 1; i < len(desc.ContainerDesc.Fields); i++ {
			if desc.ContainerDesc.Fields[i].SszIndex <= desc.ContainerDesc.Fields[i-1].SszIndex {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "progressive container requires increasing ssz-index values (field %s has index %d, previous field has %d)",
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

// buildCompatibleUnionDescriptor builds a descriptor for CompatibleUnion types
func (tc *TypeCache) buildCompatibleUnionDescriptor(desc *TypeDescriptor, t reflect.Type) error {
	// CompatibleUnion is always dynamic size (1 byte for type + variable data)
	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	// Try to extract the descriptor type from the generic type parameter
	descriptorType, err := tc.extractGenericTypeParameter(t)
	if err != nil {
		return err
	}

	// Populate union variants immediately since we have the descriptor type
	desc.UnionVariants = make(map[uint8]*TypeDescriptor)

	// Extract variant information from descriptor struct (includes SSZ annotations)
	variantInfo, err := extractUnionDescriptorInfo(descriptorType, tc.specs)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(union)")
	}

	// Build type descriptors for each variant using the extracted information
	for variantIndex, info := range variantInfo {
		variantDesc, err := tc.getTypeDescriptor(info.Type, info.SizeHints, info.MaxSizeHints, info.TypeHints)
		if err != nil {
			return sszutils.ErrorWithPathf(err, "(variant:%d)", variantIndex)
		}

		desc.UnionVariants[variantIndex] = variantDesc
	}

	return nil
}

// buildCompatibleUnionDescriptor builds a descriptor for CompatibleUnion types
func (tc *TypeCache) buildOptionalDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	// Optional is always dynamic size (1 byte for presence + variable data)
	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	if desc.GoTypeFlags&GoTypeFlagIsPointer == 0 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "optional ssz type can only be represented by pointer types, got %v", desc.Kind)
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

	elemDesc, err := tc.getTypeDescriptor(t, childSizeHints, childMaxSizeHints, childTypeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc

	// The Optional inherits properties from the child type
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagIsDynamic | SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	return nil
}

// buildBigIntDescriptor builds a descriptor for ssz big int types
func (tc *TypeCache) buildBigIntDescriptor(desc *TypeDescriptor) error {
	if desc.Kind != reflect.Struct {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bigint type can only be represented by struct types, got %v", desc.Kind)
	}

	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	return nil
}

// buildVectorDescriptor builds a descriptor for ssz vector types
func (tc *TypeCache) buildVectorDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	if desc.Kind != reflect.Array && desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "vector ssz type can only be represented by array or slice types, got %v", desc.Kind)
	}

	switch {
	case desc.Kind == reflect.Array:
		desc.Len = uint32(t.Len())
		if len(sizeHints) > 0 {
			byteLen := sizeHints[0].Size
			if sizeHints[0].Bits {
				desc.BitSize = sizeHints[0].Size
				byteLen = (byteLen + 7) / 8 // ceil up to the next multiple of 8
			}
			if byteLen > desc.Len {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "size hint for vector type is greater than the length of the array (%d > %d)", byteLen, desc.Len)
			}
			desc.Len = byteLen
		}
	case len(sizeHints) > 0 && sizeHints[0].Size > 0:
		byteLen := sizeHints[0].Size
		if sizeHints[0].Bits {
			desc.BitSize = sizeHints[0].Size
			byteLen = (byteLen + 7) / 8 // ceil up to the next multiple of 8
		}
		desc.Len = byteLen
	default:
		return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "missing size hint for vector type")
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
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	} else {
		fieldType = t.Elem()
		if fieldType == byteType {
			desc.GoTypeFlags |= GoTypeFlagIsByteArray
		}
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, childSizeHints, childMaxSizeHints, childTypeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	if desc.SszType == SszBitvectorType && desc.ElemDesc.Kind != reflect.Uint8 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitvector ssz type can only be represented by byte slices or arrays, got %v", desc.ElemDesc.Kind.String())
	}

	if elemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
		desc.Size = 0
		desc.SszTypeFlags |= SszTypeFlagIsDynamic
	} else {
		desc.Size = elemDesc.Size * desc.Len
	}

	return nil
}

// buildListDescriptor builds a descriptor for ssz list types
func (tc *TypeCache) buildListDescriptor(desc *TypeDescriptor, t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	if desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "list ssz type can only be represented by slice types, got %v", desc.Kind)
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
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	} else {
		fieldType = t.Elem()
		if fieldType == byteType {
			desc.GoTypeFlags |= GoTypeFlagIsByteArray
		}
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, childSizeHints, childMaxSizeHints, childTypeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	if desc.SszType == SszBitlistType || desc.SszType == SszProgressiveBitlistType {
		if desc.Kind != reflect.Slice {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitlist ssz type can only be represented by byte slices, got %v", desc.Kind.String())
		}
		if desc.ElemDesc.Kind != reflect.Uint8 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitlist ssz type can only be represented by byte slices, got []%v", desc.ElemDesc.Kind.String())
		}
	}

	if len(sizeHints) > 0 && sizeHints[0].Size > 0 && !sizeHints[0].Dynamic {
		// Lists cannot have a fixed ssz-size; that's a vector.
		// Lists use ssz-max to specify the maximum length.
		return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "list types cannot have a fixed ssz-size (use ssz-max for lists, or ssz-size with vector type)")
	}

	desc.Size = 0 // Dynamic slice
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

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

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

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
		return nil, sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "GetDescriptorType method not found on type %s", unionType)
	}

	// Call the method to get the descriptor type
	results := method.Call(nil)
	if len(results) == 0 {
		return nil, sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType returned no results")
	}

	// Extract the reflect.Type from the result
	descriptorType, ok := results[0].Interface().(reflect.Type)
	if !ok {
		return nil, sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType did not return a reflect.Type")
	}

	return descriptorType, nil
}

// GetTypeHash computes a SHA-256 hash of the TypeDescriptor's JSON
// representation. This hash uniquely identifies the type's SSZ layout and is
// used by the code generator to detect when a type's structure has changed and
// regeneration is needed.
func (td *TypeDescriptor) GetTypeHash() [32]byte {
	jsonDesc, _ := json.Marshal(td)
	hash := sha256.Sum256(jsonDesc)
	return hash
}
