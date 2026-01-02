// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

var (
	byteType = types.Typ[types.Uint8]
)

// CodegenInfo contains type information specific to code generation from go/types analysis.
//
// This structure bridges the gap between compile-time type analysis (using go/types)
// and runtime code generation. It stores type information that was obtained through
// static analysis rather than runtime reflection, enabling more sophisticated
// code generation scenarios.
//
// Fields:
//   - Type: The types.Type from go/types package representing the analyzed type
//
// This information is embedded in TypeDescriptor.CodegenInfo to provide access
// to compile-time type information during code generation.
type CodegenInfo struct {
	Type types.Type
}

// Parser provides compile-time type analysis for SSZ code generation.
//
// The Parser analyzes Go types using the go/types package to create TypeDescriptors
// suitable for code generation. Unlike runtime reflection, this approach can analyze
// types that may not be available at runtime and provides richer type information
// for complex code generation scenarios.
//
// Key capabilities:
//   - Compile-time type analysis using go/types
//   - SSZ type inference and validation
//   - Struct tag parsing for SSZ annotations
//   - Interface compatibility checking
//   - Type descriptor caching for performance
//
// The parser handles all SSZ-compatible types including basic types, containers,
// vectors, lists, and custom types like unions and type wrappers.
//
// Fields:
//   - cache: Type descriptor cache to avoid recomputing analysis for the same types
type Parser struct {
	cache       map[string]*ssztypes.TypeDescriptor
	CompatFlags map[string]ssztypes.SszCompatFlag
}

// NewParser creates a new compile-time type parser for code generation.
//
// The parser is initialized with an empty cache and is ready to analyze types
// using the go/types package. The parser can be reused across multiple type
// analysis operations to benefit from caching.
//
// Returns:
//   - *Parser: A new parser instance ready for type analysis
//
// Example:
//
//	parser := NewParser()
//	desc, err := parser.GetTypeDescriptor(myGoType, nil, nil, nil)
//	if err != nil {
//	    log.Fatal("Type analysis failed:", err)
//	}
func NewParser() *Parser {
	return &Parser{
		cache:       make(map[string]*ssztypes.TypeDescriptor),
		CompatFlags: map[string]ssztypes.SszCompatFlag{},
	}
}

// GetTypeDescriptor analyzes a Go type and creates an SSZ type descriptor for code generation.
//
// This method is the main entry point for type analysis. It examines the provided
// go/types.Type and creates a comprehensive TypeDescriptor containing all information
// needed for SSZ code generation, including size calculations, encoding strategies,
// and interface compatibility.
//
// The analysis process includes:
//   - Type structure examination and validation
//   - SSZ type inference and mapping
//   - Size and constraint analysis from hints
//   - Interface compatibility checking
//   - Nested type analysis for containers and collections
//
// Parameters:
//   - typ: The go/types.Type to analyze
//   - typeHints: Optional hints for explicit SSZ type mapping
//   - sizeHints: Optional size constraints and expressions
//   - maxSizeHints: Optional maximum size limits for variable-length types
//
// Returns:
//   - *ssztypes.TypeDescriptor: Complete type descriptor for code generation
//   - error: An error if the type is incompatible with SSZ or analysis fails
//
// Example:
//
//	parser := NewParser()
//	typeHints := []ssztypes.SszTypeHint{{Type: ssztypes.SszListType}}
//	sizeHints := []ssztypes.SszSizeHint{{Size: 1024}}
//
//	desc, err := parser.GetTypeDescriptor(structType, typeHints, sizeHints, nil)
//	if err != nil {
//	    return fmt.Errorf("failed to analyze type: %w", err)
//	}
func (p *Parser) GetTypeDescriptor(typ types.Type, typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint) (*ssztypes.TypeDescriptor, error) {
	desc, err := p.buildTypeDescriptor(typ, typeHints, sizeHints, maxSizeHints)
	if err != nil {
		return nil, err
	}

	return desc, nil
}

func (p *Parser) getCompatFlag(typ types.Type) ssztypes.SszCompatFlag {
	typeName := typ.String()
	return p.CompatFlags[typeName]
}

func (p *Parser) buildTypeDescriptor(typ types.Type, typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint) (*ssztypes.TypeDescriptor, error) {
	cacheable := len(typeHints) == 0 && len(sizeHints) == 0 && len(maxSizeHints) == 0
	typeKey := fmt.Sprintf("%v", typ.String())
	if cacheable && p.cache[typeKey] != nil {
		return p.cache[typeKey], nil
	}

	// Create descriptor
	codegenInfo := &CodegenInfo{Type: typ}
	var anyCodegenInfo any = codegenInfo
	desc := &ssztypes.TypeDescriptor{
		CodegenInfo: &anyCodegenInfo,
	}

	if cacheable {
		p.cache[typeKey] = desc
	}

	originalType := typ
	innerType := typ

	for {
		// Resolve named types
		if named, ok := typ.(*types.Named); ok {
			typ = named.Underlying()
		} else if ptr, ok := typ.(*types.Pointer); ok {
			typ = ptr.Elem()
			desc.GoTypeFlags |= ssztypes.GoTypeFlagIsPointer
			innerType = typ
		} else if alias, ok := typ.(*types.Alias); ok {
			typ = alias.Underlying()
		} else {
			break
		}
	}

	// Set kind based on underlying type
	switch t := typ.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			desc.Kind = reflect.Bool
		case types.Uint8:
			desc.Kind = reflect.Uint8
		case types.Uint16:
			desc.Kind = reflect.Uint16
		case types.Uint32:
			desc.Kind = reflect.Uint32
		case types.Uint64, types.Uint:
			desc.Kind = reflect.Uint64
		case types.String:
			desc.Kind = reflect.String
			desc.GoTypeFlags |= ssztypes.GoTypeFlagIsString
		default:
			desc.Kind = reflect.Invalid
		}
	case *types.Array:
		desc.Kind = reflect.Array
	case *types.Slice:
		desc.Kind = reflect.Slice
	case *types.Struct:
		desc.Kind = reflect.Struct
	default:
		desc.Kind = reflect.Invalid
	}

	// Check dynamic size and max size hints (like reflection code)
	if len(sizeHints) > 0 {
		if sizeHints[0].Expr != "" {
			desc.SizeExpression = &sizeHints[0].Expr
		}
		if sizeHints[0].Bits {
			desc.SszTypeFlags |= ssztypes.SszTypeFlagHasBitSize
			desc.BitSize = sizeHints[0].Size
		}
		for _, hint := range sizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasDynamicSize
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasSizeExpr
			}
		}
	}

	if len(maxSizeHints) > 0 {
		if !maxSizeHints[0].NoValue {
			desc.SszTypeFlags |= ssztypes.SszTypeFlagHasLimit
			desc.Limit = maxSizeHints[0].Size
		}
		if maxSizeHints[0].Expr != "" {
			desc.MaxExpression = &maxSizeHints[0].Expr
		}
		for _, hint := range maxSizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasDynamicMax
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasMaxExpr
			}
		}
	}

	// Determine SSZ type - first use type hints if specified
	sszType := ssztypes.SszUnspecifiedType
	if len(typeHints) > 0 {
		sszType = typeHints[0].Type
	}

	if desc.Kind == reflect.String {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsString
	}

	// Auto-detect ssz type if not specified
	if sszType == ssztypes.SszUnspecifiedType {
		// Detect well-known types first (named types)
		var obj *types.TypeName
		if alias, ok := innerType.(*types.Alias); ok {
			innerType = types.Unalias(alias)
		}
		if named, ok := innerType.(*types.Named); ok {
			obj = named.Obj()
		}

		if obj != nil && obj.Pkg() != nil {
			pkgPath := obj.Pkg().Path()
			typeName := obj.Name()

			switch {
			case pkgPath == "time" && typeName == "Time":
				sszType = ssztypes.SszUint64Type
				desc.GoTypeFlags |= ssztypes.GoTypeFlagIsTime
			case pkgPath == "github.com/holiman/uint256" && typeName == "Int":
				sszType = ssztypes.SszUint256Type
			case pkgPath == "github.com/prysmaticlabs/go-bitfield" && typeName == "Bitlist":
				sszType = ssztypes.SszBitlistType
			case pkgPath == "github.com/OffchainLabs/go-bitfield" && typeName == "Bitlist":
				sszType = ssztypes.SszBitlistType
			case pkgPath == "github.com/pk910/dynamic-ssz" && typeName == "CompatibleUnion":
				sszType = ssztypes.SszCompatibleUnionType
			case pkgPath == "github.com/pk910/dynamic-ssz" && typeName == "TypeWrapper":
				sszType = ssztypes.SszTypeWrapperType
			}
		}
	}

	if sszType == ssztypes.SszUnspecifiedType {
		switch desc.Kind {
		// basic types
		case reflect.Bool:
			sszType = ssztypes.SszBoolType
		case reflect.Uint8:
			sszType = ssztypes.SszUint8Type
		case reflect.Uint16:
			sszType = ssztypes.SszUint16Type
		case reflect.Uint32:
			sszType = ssztypes.SszUint32Type
		case reflect.Uint64:
			sszType = ssztypes.SszUint64Type

		// complex types
		case reflect.Struct:
			sszType = ssztypes.SszContainerType
		case reflect.Array:
			sszType = ssztypes.SszVectorType
		case reflect.Slice:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = ssztypes.SszVectorType
			} else {
				sszType = ssztypes.SszListType
			}
		case reflect.String:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = ssztypes.SszVectorType
			} else {
				sszType = ssztypes.SszListType
			}

		// unsupported types
		default:
			// Check for unsupported basic types
			if basic, ok := typ.(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
					return nil, fmt.Errorf("signed integers are not supported in SSZ (use unsigned integers instead)")
				case types.Float32, types.Float64:
					return nil, fmt.Errorf("floating-point numbers are not supported in SSZ")
				case types.Complex64, types.Complex128:
					return nil, fmt.Errorf("complex numbers are not supported in SSZ")
				}
			}
			// Check for other unsupported types
			switch typ.(type) {
			case *types.Map:
				return nil, fmt.Errorf("maps are not supported in SSZ (use structs or arrays instead)")
			case *types.Chan:
				return nil, fmt.Errorf("channels are not supported in SSZ")
			case *types.Signature:
				return nil, fmt.Errorf("functions are not supported in SSZ")
			case *types.Interface:
				return nil, fmt.Errorf("interfaces are not supported in SSZ (use concrete types)")
			default:
				return nil, fmt.Errorf("unsupported type kind: %v", desc.Kind)
			}
		}
	}

	desc.SszType = sszType

	// Check type compatibility and build descriptor based on SSZ type
	switch sszType {
	// basic types
	case ssztypes.SszBoolType:
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
	case ssztypes.SszUint8Type:
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
	case ssztypes.SszUint16Type:
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
	case ssztypes.SszUint32Type:
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
	case ssztypes.SszUint64Type:
		if desc.Kind != reflect.Uint64 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
			return nil, fmt.Errorf("uint64 ssz type can only be represented by uint64 or time.Time types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint64 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return nil, fmt.Errorf("uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
		}
		desc.Size = 8
	case ssztypes.SszUint128Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint128 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := p.buildUint128Descriptor(desc, typ)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszUint256Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint256 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := p.buildUint256Descriptor(desc, typ)
		if err != nil {
			return nil, err
		}

	// complex types
	case ssztypes.SszTypeWrapperType:
		if named, ok := innerType.(*types.Named); ok {
			err := p.buildTypeWrapperDescriptor(desc, named, typeHints, sizeHints, maxSizeHints)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("TypeWrapper must be a named type")
		}
	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		if struc, ok := typ.(*types.Struct); ok {
			err := p.buildContainerDescriptor(desc, struc)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("container ssz type can only be represented by struct types, got %v", desc.Kind)
		}
	case ssztypes.SszVectorType, ssztypes.SszBitvectorType:
		err := p.buildVectorDescriptor(desc, typ, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		err := p.buildListDescriptor(desc, typ, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		err := p.buildBitlistDescriptor(desc, typ, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszCompatibleUnionType:
		if named, ok := innerType.(*types.Named); ok {
			err := p.buildCompatibleUnionDescriptor(desc, named)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("CompatibleUnion must be a named type")
		}
	case ssztypes.SszCustomType:
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			desc.Size = uint32(sizeHints[0].Size)
			if sizeHints[0].Bits {
				desc.BitSize = sizeHints[0].Size
				desc.Size = (desc.Size + 7) / 8 // ceil up to the next multiple of 8
			}
		} else {
			desc.Size = 0
			desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
		}
	}

	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 && desc.SszType != ssztypes.SszBitvectorType && desc.SszType != ssztypes.SszBitlistType {
		return nil, fmt.Errorf("bit size tag is only allowed for bitvector or bitlist types, got %v", desc.SszType)
	}

	// Check interface compatibility (like reflection-based code)
	otherType := originalType
	if ptr, ok := otherType.(*types.Pointer); ok {
		otherType = ptr.Elem()
	} else {
		otherType = types.NewPointer(otherType)
	}

	if (desc.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicSize == 0 || desc.SszType == ssztypes.SszCustomType) && (p.getFastsszConvertCompatibility(originalType) || p.getFastsszConvertCompatibility(otherType)) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagFastSSZMarshaler
	}
	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicMax == 0 || desc.SszType == ssztypes.SszCustomType {
		if p.getFastsszHashCompatibility(originalType) || p.getFastsszHashCompatibility(otherType) {
			desc.SszCompatFlags |= ssztypes.SszCompatFlagFastSSZHasher
		}
		if p.getHashTreeRootWithCompatibility(originalType) || p.getHashTreeRootWithCompatibility(otherType) {
			desc.SszCompatFlags |= ssztypes.SszCompatFlagHashTreeRootWith
		}
	}

	// Check for dynamic interface implementations
	if p.getDynamicMarshalerCompatibility(originalType) || p.getDynamicMarshalerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicMarshaler
	}
	if p.getDynamicUnmarshalerCompatibility(originalType) || p.getDynamicUnmarshalerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicUnmarshaler
	}
	if p.getDynamicEncoderCompatibility(originalType) || p.getDynamicEncoderCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicEncoder
	}
	if p.getDynamicDecoderCompatibility(originalType) || p.getDynamicDecoderCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicDecoder
	}
	if p.getDynamicSizerCompatibility(originalType) || p.getDynamicSizerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicSizer
	}
	if p.getDynamicHashRootCompatibility(originalType) || p.getDynamicHashRootCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicHashRoot
	}

	desc.SszCompatFlags |= p.getCompatFlag(innerType)

	if desc.SszType == ssztypes.SszCustomType {
		isCompatible := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZHasher != 0
		//isCompatible = isCompatible || (desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicMarshaler != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicUnmarshaler != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicSizer != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicHashRoot != 0)

		if !isCompatible {
			return nil, fmt.Errorf("custom ssz type requires fastssz marshaler, unmarshaler and hasher implementations")
		}
	}

	return desc, nil
}

func (p *Parser) buildUint128Descriptor(desc *ssztypes.TypeDescriptor, typ types.Type) error {
	// Handle as [16]uint8, [2]uint64
	var elemType types.Type
	if arr, ok := typ.(*types.Array); ok {
		elemType = arr.Elem()
		if arr.Len() == 16 {
			if elem, ok := arr.Elem().(*types.Basic); ok && elem.Kind() == types.Uint8 {
				desc.Size = 16
			}
		} else if arr.Len() == 2 {
			if elem, ok := arr.Elem().(*types.Basic); ok && elem.Kind() == types.Uint64 {
				desc.Size = 16
			}
		}
	} else if named, ok := typ.(*types.Slice); ok {
		elemType = named.Elem()
		if elem, ok := named.Elem().(*types.Basic); ok {
			if elem.Kind() == types.Uint8 {
				desc.Size = 16
			} else if elem.Kind() == types.Uint64 {
				desc.Size = 16
			}
		}
	}

	if desc.Size == 0 {
		return fmt.Errorf("uint128 ssz type can only be represented by [16]uint8 or [2]uint64 types")
	}

	// Build element descriptor
	elemDesc, err := p.buildTypeDescriptor(elemType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to build vector element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc
	desc.Len = uint32(desc.Size / elemDesc.Size)

	// Set byte array flag for byte types
	if p.isByteType(elemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	return nil
}

func (p *Parser) buildUint256Descriptor(desc *ssztypes.TypeDescriptor, typ types.Type) error {
	// Handle as [32]uint8, [4]uint64
	var elemType types.Type
	if arr, ok := typ.(*types.Array); ok {
		elemType = arr.Elem()
		if arr.Len() == 32 {
			if elem, ok := arr.Elem().(*types.Basic); ok && elem.Kind() == types.Uint8 {
				desc.Size = 32
			}
		} else if arr.Len() == 4 {
			if elem, ok := arr.Elem().(*types.Basic); ok && elem.Kind() == types.Uint64 {
				desc.Size = 32
			}
		}
	} else if named, ok := typ.(*types.Slice); ok {
		elemType = named.Elem()
		if elem, ok := named.Elem().(*types.Basic); ok {
			if elem.Kind() == types.Uint8 {
				desc.Size = 32
			} else if elem.Kind() == types.Uint64 {
				desc.Size = 32
			}
		}
	}

	if desc.Size == 0 {
		return fmt.Errorf("uint256 ssz type can only be represented by [32]uint8 or [4]uint64 types")
	}

	// Build element descriptor
	elemDesc, err := p.buildTypeDescriptor(elemType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to build vector element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc
	desc.Len = uint32(desc.Size / elemDesc.Size)

	// Set byte array flag for byte types
	if p.isByteType(elemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	return nil
}

func (p *Parser) buildContainerDescriptor(desc *ssztypes.TypeDescriptor, struc *types.Struct) error {
	fields := []ssztypes.FieldDescriptor{}
	dynFields := []ssztypes.DynFieldDescriptor{}
	size := uint32(0)
	isDynamic := false

	for i := 0; i < struc.NumFields(); i++ {
		field := struc.Field(i)
		fieldName := field.Name()
		if !field.Exported() || fieldName == "_" {
			continue
		}

		typeHints, sizeHints, maxSizeHints, err := p.parseFieldTags(struc.Tag(i))
		if err != nil {
			return fmt.Errorf("failed to parse tags for field %v: %v", field.Name(), err)
		}

		// Build type descriptor with field-specific hints
		typeDesc, err := p.buildTypeDescriptor(field.Type(), typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return fmt.Errorf("failed to build field %v descriptor: %v", field.Name(), err)
		}

		fieldDesc := ssztypes.FieldDescriptor{
			Name: field.Name(),
			Type: typeDesc,
		}

		// Handle ssz-index for progressive containers - extract from original tag parsing
		if indexStr := p.extractSszIndex(struc.Tag(i)); indexStr != "" {
			idx, err := strconv.ParseUint(indexStr, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid ssz-index: %v", indexStr)
			}
			fieldDesc.SszIndex = uint16(idx)
		}

		if typeDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			// Dynamic field
			dynFieldDesc := ssztypes.DynFieldDescriptor{
				Field:        &fieldDesc,
				HeaderOffset: size,
				Index:        int16(len(fields)),
			}
			dynFields = append(dynFields, dynFieldDesc)
			isDynamic = true
			size += 4
		} else {
			size += typeDesc.Size
		}

		desc.SszTypeFlags |= fieldDesc.Type.SszTypeFlags & (ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)
		fields = append(fields, fieldDesc)
	}

	containerDesc := &ssztypes.ContainerDescriptor{
		Fields:    fields,
		DynFields: dynFields,
	}
	desc.ContainerDesc = containerDesc

	desc.Len = size
	if isDynamic {
		desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
		desc.Size = 0
	} else {
		desc.Size = size
	}

	return nil
}

func (p *Parser) buildVectorDescriptor(desc *ssztypes.TypeDescriptor, typ types.Type, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, typeHints []ssztypes.SszTypeHint) error {
	var elemType types.Type
	var length uint32

	switch t := typ.(type) {
	case *types.Array:
		elemType = t.Elem()
		length = uint32(t.Len())
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			byteSize := sizeHints[0].Size
			if sizeHints[0].Bits {
				byteSize = (byteSize + 7) / 8 // ceil up to the next multiple of 8
			}
			if byteSize > length {
				return fmt.Errorf("size hint for vector type is greater than the length of the array (%d > %d)", byteSize, length)
			}
			length = byteSize
		}
	case *types.Slice:
		elemType = t.Elem()
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			length = sizeHints[0].Size
			if sizeHints[0].Bits {
				length = (length + 7) / 8 // ceil up to the next multiple of 8
			}
		} else {
			return fmt.Errorf("vector slice type requires explicit size hint")
		}
	case *types.Basic:
		if t.Kind() == types.String {
			// String as vector
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				length = sizeHints[0].Size
				if sizeHints[0].Bits {
					length = (length + 7) / 8 // ceil up to the next multiple of 8
				}
				desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
				elemType = byteType
			} else {
				return fmt.Errorf("string vector type requires explicit size hint")
			}
		} else {
			return fmt.Errorf("unsupported vector base type: %v", t.Kind())
		}
	default:
		return fmt.Errorf("unsupported vector type: %T", typ)
	}

	childTypeHints := []ssztypes.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}
	childSizeHints := []ssztypes.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}
	childMaxSizeHints := []ssztypes.SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	// Build element descriptor
	elemDesc, err := p.buildTypeDescriptor(elemType, childTypeHints, childSizeHints, childMaxSizeHints)
	if err != nil {
		return fmt.Errorf("failed to build vector element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc
	desc.Len = length

	// Set byte array flag for byte types
	if p.isByteType(elemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	// Calculate size
	if elemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
		desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
		desc.Size = 0
	} else {
		desc.Size = length * elemDesc.Size
	}

	return nil
}

func (p *Parser) buildListDescriptor(desc *ssztypes.TypeDescriptor, typ types.Type, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, typeHints []ssztypes.SszTypeHint) error {
	var elemType types.Type

	switch t := typ.(type) {
	case *types.Slice:
		elemType = t.Elem()
	case *types.Basic:
		if t.Kind() == types.String {
			// String as list - set byte array flag and make dynamic
			desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
			desc.Size = 0
			desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
			elemType = byteType
		} else {
			return fmt.Errorf("unsupported list base type: %v", t.Kind())
		}
	default:
		return fmt.Errorf("unsupported list type: %T", typ)
	}

	childTypeHints := []ssztypes.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}
	childSizeHints := []ssztypes.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}
	childMaxSizeHints := []ssztypes.SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	// Build element descriptor
	elemDesc, err := p.buildTypeDescriptor(elemType, childTypeHints, childSizeHints, childMaxSizeHints)
	if err != nil {
		return fmt.Errorf("failed to build list element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc

	// Set byte array flag for byte types
	if p.isByteType(elemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	// Lists are always dynamic
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	desc.Size = 0

	return nil
}

func (p *Parser) buildBitlistDescriptor(desc *ssztypes.TypeDescriptor, typ types.Type, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, typeHints []ssztypes.SszTypeHint) error {
	var elemType types.Type

	switch t := typ.(type) {
	case *types.Slice:
		elemType = t.Elem()
	default:
		return fmt.Errorf("bitlist type can only be represented by slice types, got %T", typ)
	}

	// Build element descriptor
	elemDesc, err := p.buildTypeDescriptor(elemType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to build bitlist element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc

	// Bitlist must use byte (uint8) elements
	if elemDesc.Kind != reflect.Uint8 {
		return fmt.Errorf("bitlist ssz type can only be represented by byte slices, got []%v", elemDesc.Kind)
	}

	// Bitlists are always dynamic
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	desc.Size = 0
	desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray

	return nil
}

func (p *Parser) buildCompatibleUnionDescriptor(desc *ssztypes.TypeDescriptor, named *types.Named) error {
	// Extract generic type arguments from CompatibleUnion[T]
	typeArgs := named.TypeArgs()
	if typeArgs == nil || typeArgs.Len() != 1 {
		return fmt.Errorf("CompatibleUnion must have exactly 1 type argument")
	}

	descriptorType := typeArgs.At(0) // T - the descriptor struct

	// The descriptor must be a struct type
	descriptorStruct, ok := descriptorType.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("CompatibleUnion descriptor must be a struct, got %T", descriptorType.Underlying())
	}

	// Build union variants
	variantInfo := make(map[uint8]*ssztypes.TypeDescriptor)

	for i := 0; i < descriptorStruct.NumFields(); i++ {
		field := descriptorStruct.Field(i)
		variantIndex := uint8(i) // Field order determines variant index

		// Extract SSZ annotations from the field
		typeHints, sizeHints, maxSizeHints, err := p.parseFieldTags(descriptorStruct.Tag(i))
		if err != nil {
			return fmt.Errorf("failed to parse union variant field %s tags: %v", field.Name(), err)
		}

		// Build variant type descriptor
		variantDesc, err := p.buildTypeDescriptor(field.Type(), typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return fmt.Errorf("failed to build union variant %d descriptor: %v", variantIndex, err)
		}

		variantInfo[variantIndex] = variantDesc
	}

	if len(variantInfo) == 0 {
		return fmt.Errorf("union descriptor struct has no fields")
	}

	desc.UnionVariants = variantInfo
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	desc.Size = 0

	return nil
}

func (p *Parser) buildTypeWrapperDescriptor(desc *ssztypes.TypeDescriptor, named *types.Named, typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint) error {
	// Extract generic type arguments from TypeWrapper[D, T]
	typeArgs := named.TypeArgs()
	if typeArgs == nil || typeArgs.Len() != 2 {
		return fmt.Errorf("TypeWrapper must have exactly 2 type arguments")
	}

	descriptorType := typeArgs.At(0) // D - the descriptor struct
	wrappedType := typeArgs.At(1)    // T - the actual value type

	// The descriptor must be a struct type
	descriptorStruct, ok := descriptorType.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("TypeWrapper descriptor must be a struct, got %T", descriptorType.Underlying())
	}

	// The descriptor must have exactly 1 field
	if descriptorStruct.NumFields() != 1 {
		return fmt.Errorf("TypeWrapper descriptor must have exactly 1 field, got %d", descriptorStruct.NumFields())
	}

	// Extract SSZ annotations from the descriptor field
	field := descriptorStruct.Field(0)
	fieldTypeHints, fieldSizeHints, fieldMaxSizeHints, err := p.parseFieldTags(descriptorStruct.Tag(0))
	if err != nil {
		return fmt.Errorf("failed to parse TypeWrapper descriptor field tags: %v", err)
	}

	// Verify the field type matches the wrapped type
	if !types.Identical(field.Type(), wrappedType) {
		return fmt.Errorf("TypeWrapper descriptor field type %v does not match wrapped type %v", field.Type(), wrappedType)
	}

	// Build the wrapped type descriptor using the extracted annotations
	wrappedDesc, err := p.buildTypeDescriptor(wrappedType, fieldTypeHints, fieldSizeHints, fieldMaxSizeHints)
	if err != nil {
		return fmt.Errorf("failed to build TypeWrapper wrapped type descriptor: %v", err)
	}

	// Store wrapper information
	desc.ElemDesc = wrappedDesc

	// The TypeWrapper inherits properties from the wrapped type
	desc.Size = wrappedDesc.Size
	desc.SszTypeFlags |= wrappedDesc.SszTypeFlags & (ssztypes.SszTypeFlagIsDynamic | ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	return nil
}

func (p *Parser) parseFieldTags(tag string) (typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, err error) {
	if tag == "" {
		return nil, nil, nil, nil
	}

	structTag := reflect.StructTag(tag)

	// Parse type hints (matching getSszTypeTag logic)
	if sszType, ok := structTag.Lookup("ssz-type"); ok {
		for _, typeStr := range strings.Split(sszType, ",") {
			typeStr = strings.TrimSpace(typeStr)
			hint := ssztypes.SszTypeHint{}

			hint.Type, err = ssztypes.ParseSszType(typeStr)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error parsing ssz-type tag: %v", err)
			}

			typeHints = append(typeHints, hint)
		}
	}

	// Parse size hints (matching getSszSizeTag logic)
	var sszSizeParts, sszBitsizeParts []string

	sszSizeLen := 0

	if fieldSszSizeStr, fieldHasSszSize := structTag.Lookup("ssz-size"); fieldHasSszSize {
		sszSizeParts = strings.Split(fieldSszSizeStr, ",")
		sszSizeLen = len(sszSizeParts)
	}

	if fieldSszBitsizeStr, fieldHasSszBitsize := structTag.Lookup("ssz-bitsize"); fieldHasSszBitsize {
		sszBitsizeParts = strings.Split(fieldSszBitsizeStr, ",")
		if len(sszBitsizeParts) > sszSizeLen {
			sszSizeLen = len(sszBitsizeParts)
		}
	}

	if sszSizeLen > 0 {
		for i := 0; i < sszSizeLen; i++ {
			sszSizeStr := getTagPart(sszSizeParts, i)
			sszBitsizeStr := getTagPart(sszBitsizeParts, i)

			hint := ssztypes.SszSizeHint{}

			if sszBitsizeStr != "?" {
				sizeInt, err := strconv.ParseUint(strings.TrimSpace(sszBitsizeStr), 10, 32)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("error parsing ssz-size tag: %v", err)
				}
				hint.Size = uint32(sizeInt)
				hint.Bits = true
			} else if sszSizeStr != "?" {
				sizeInt, err := strconv.ParseUint(strings.TrimSpace(sszSizeStr), 10, 32)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("error parsing ssz-size tag: %v", err)
				}
				hint.Size = uint32(sizeInt)
			} else {
				hint.Dynamic = true
			}

			sizeHints = append(sizeHints, hint)
		}
	}

	// Parse dynamic size hints
	sszSizeParts, sszBitsizeParts = nil, nil
	sszSizeLen = 0

	if fieldDynSszSizeStr, fieldHasDynSszSize := structTag.Lookup("dynssz-size"); fieldHasDynSszSize {
		sszSizeParts = strings.Split(fieldDynSszSizeStr, ",")
		sszSizeLen = len(sszSizeParts)
	}

	if fieldDynSszBitsizeStr, fieldHasDynSszBitsize := structTag.Lookup("dynssz-bitsize"); fieldHasDynSszBitsize {
		sszBitsizeParts = strings.Split(fieldDynSszBitsizeStr, ",")
		if len(sszBitsizeParts) > sszSizeLen {
			sszSizeLen = len(sszBitsizeParts)
		}
	}

	if sszSizeLen > 0 {
		for i := 0; i < sszSizeLen; i++ {
			sszSizeStr := getTagPart(sszSizeParts, i)
			sszBitsizeStr := getTagPart(sszBitsizeParts, i)

			sszSize := ssztypes.SszSizeHint{}
			isExpr := false
			sizeExpr := "?"

			if sszBitsizeStr != "?" {
				sizeExpr = sszBitsizeStr
				sszSize.Bits = true
			} else if sszSizeStr != "?" {
				sizeExpr = sszSizeStr
			}

			if sizeExpr == "?" {
				sszSize.Dynamic = true
			} else if sszSizeInt, err := strconv.ParseUint(sizeExpr, 10, 32); err == nil {
				sszSize.Size = uint32(sszSizeInt)
			} else {
				// For go/types parser, we can't resolve spec values at compile time
				// So we treat all non-numeric values as expressions
				isExpr = true
				sszSize.Dynamic = true
				sszSize.Custom = true
				if i < len(sizeHints) {
					sizeHints[i].Expr = sizeExpr
					continue
				}
			}

			if i >= len(sizeHints) {
				sizeHints = append(sizeHints, sszSize)
			} else if sizeHints[i].Size != sszSize.Size {
				// update if resolved size differs from default
				sizeHints[i] = sszSize
			}

			if isExpr {
				sizeHints[i].Expr = sizeExpr
			}
		}
	}

	// Parse max size hints (matching getSszMaxSizeTag logic)
	if sszMax, ok := structTag.Lookup("ssz-max"); ok {
		for _, maxStr := range strings.Split(sszMax, ",") {
			maxStr = strings.TrimSpace(maxStr)
			hint := ssztypes.SszMaxSizeHint{}

			if maxStr == "?" {
				hint.NoValue = true
			} else {
				maxInt, err := strconv.ParseUint(maxStr, 10, 64)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("error parsing ssz-max tag: %v", err)
				}
				hint.Size = maxInt
			}

			maxSizeHints = append(maxSizeHints, hint)
		}
	}

	// Parse dynamic max size hints
	fieldDynSszMaxStr, fieldHasDynSszMax := structTag.Lookup("dynssz-max")
	if fieldHasDynSszMax {
		for i, sszMaxSizeStr := range strings.Split(fieldDynSszMaxStr, ",") {
			sszMaxSize := ssztypes.SszMaxSizeHint{}
			isExpr := false

			if sszMaxSizeStr == "?" {
				sszMaxSize.NoValue = true
			} else if sszSizeInt, err := strconv.ParseUint(sszMaxSizeStr, 10, 64); err == nil {
				sszMaxSize.Size = sszSizeInt
			} else {
				// For go/types parser, we can't resolve spec values at compile time
				// So we treat all non-numeric values as expressions
				isExpr = true
				sszMaxSize.Custom = true
				if i < len(maxSizeHints) {
					maxSizeHints[i].Expr = sszMaxSizeStr
				}
				continue
			}

			if i >= len(maxSizeHints) {
				maxSizeHints = append(maxSizeHints, sszMaxSize)
			} else if maxSizeHints[i].Size != sszMaxSize.Size {
				// update if resolved max size differs from default
				maxSizeHints[i] = sszMaxSize
			}

			if isExpr {
				maxSizeHints[i].Expr = sszMaxSizeStr
			}
		}
	}

	return typeHints, sizeHints, maxSizeHints, nil
}

func (p *Parser) extractSszIndex(tag string) string {
	if tag == "" {
		return ""
	}
	structTag := reflect.StructTag(tag)
	if index, ok := structTag.Lookup("ssz-index"); ok {
		return index
	}
	return ""
}

func (p *Parser) isByteType(typ types.Type) bool {
	basic, ok := typ.(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

// Interface compatibility checks using proper go/types interface implementation checking

func (p *Parser) getFastsszConvertCompatibility(typ types.Type) bool {
	methodSet := types.NewMethodSet(typ)
	return (p.hasMethodWithSignature(methodSet, "MarshalSSZTo", []string{"[]byte"}, []string{"[]byte", "error"}) &&
		p.hasMethodWithSignature(methodSet, "SizeSSZ", []string{}, []string{"int"}) &&
		p.hasMethodWithSignature(methodSet, "UnmarshalSSZ", []string{"[]byte"}, []string{"error"}))
}

func (p *Parser) getFastsszHashCompatibility(typ types.Type) bool {
	methodSet := types.NewMethodSet(typ)
	return (p.hasMethodWithSignature(methodSet, "HashTreeRoot", []string{}, []string{"[32]byte", "error"}))
}

func (p *Parser) getHashTreeRootWithCompatibility(typ types.Type) bool {
	// Check if type has HashTreeRootWith method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "HashTreeRootWith", []string{"-"}, []string{"error"})
}

func (p *Parser) getDynamicMarshalerCompatibility(typ types.Type) bool {
	// Check if type has MarshalSSZDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "MarshalSSZDyn", []string{"DynamicSpecs", "[]byte"}, []string{"[]byte", "error"})
}

func (p *Parser) getDynamicUnmarshalerCompatibility(typ types.Type) bool {
	// Check if type has UnmarshalSSZDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "UnmarshalSSZDyn", []string{"DynamicSpecs", "[]byte"}, []string{"error"})
}

func (p *Parser) getDynamicEncoderCompatibility(typ types.Type) bool {
	// Check if type has MarshalSSZEncoder method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "MarshalSSZEncoder", []string{"DynamicSpecs", "Encoder"}, []string{"error"})
}

func (p *Parser) getDynamicDecoderCompatibility(typ types.Type) bool {
	// Check if type has UnmarshalSSZDecoder method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "UnmarshalSSZDecoder", []string{"DynamicSpecs", "Decoder"}, []string{"error"})
}

func (p *Parser) getDynamicSizerCompatibility(typ types.Type) bool {
	// Check if type has SizeSSZDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "SizeSSZDyn", []string{"DynamicSpecs"}, []string{"int"})
}

func (p *Parser) getDynamicHashRootCompatibility(typ types.Type) bool {
	// Check if type has HashTreeRootDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "HashTreeRootDynWith", []string{"DynamicSpecs", "HashWalker"}, []string{"error"})
}

// Interface implementation checks using go/types proper interface checking

// Simple helper to check if a type has required methods
func (p *Parser) hasMethodWithSignature(methodSet *types.MethodSet, methodName string, paramTypes, returnTypes []string) bool {
	for i := 0; i < methodSet.Len(); i++ {
		method := methodSet.At(i)
		if method.Obj().Name() != methodName {
			continue
		}

		// Check method signature
		sig, ok := method.Type().(*types.Signature)
		if !ok {
			continue
		}

		// Check parameter count and types
		if sig.Params().Len() != len(paramTypes) {
			continue
		}

		// Check return value count and types
		if sig.Results().Len() != len(returnTypes) {
			continue
		}

		// Check parameter types
		for j := 0; j < sig.Params().Len(); j++ {
			paramType := sig.Params().At(j).Type()
			expectedType := paramTypes[j]
			if !p.typeMatches(paramType, expectedType) {
				goto nextMethod
			}
		}

		// Check return types
		for j := 0; j < sig.Results().Len(); j++ {
			returnType := sig.Results().At(j).Type()
			expectedType := returnTypes[j]
			if !p.typeMatches(returnType, expectedType) {
				goto nextMethod
			}
		}

		return true

	nextMethod:
	}
	return false
}

func (p *Parser) typeMatches(typ types.Type, expectedTypeStr string) bool {
	switch expectedTypeStr {
	case "-":
		return true
	case "[]byte":
		if slice, ok := typ.(*types.Slice); ok {
			if basic, ok := slice.Elem().(*types.Basic); ok {
				return basic.Kind() == types.Uint8
			}
		}
	case "[32]byte":
		if array, ok := typ.(*types.Array); ok && array.Len() == 32 {
			if basic, ok := array.Elem().(*types.Basic); ok {
				return basic.Kind() == types.Uint8
			}
		}
	case "error":
		if named, ok := typ.(*types.Named); ok {
			return named.Obj().Name() == "error" && named.Obj().Pkg() == nil
		}
	case "int":
		if basic, ok := typ.(*types.Basic); ok {
			return basic.Kind() == types.Int
		}
	case "DynamicSpecs", "HashWalker", "Encoder", "Decoder":
		return true
	}
	return false
}

func getTagPart(parts []string, index int) string {
	if index < len(parts) {
		return parts[index]
	}
	return "?"
}
