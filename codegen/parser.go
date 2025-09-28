package codegen

import (
	"fmt"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

type CodegenInfo struct {
	Type types.Type
}

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) GetTypeDescriptor(typ types.Type, typeHints []dynssz.SszTypeHint, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint) (*dynssz.TypeDescriptor, error) {
	desc, err := p.buildTypeDescriptor(typ, typeHints, sizeHints, maxSizeHints)
	if err != nil {
		return nil, err
	}

	return desc, nil
}

func (p *Parser) buildTypeDescriptor(typ types.Type, typeHints []dynssz.SszTypeHint, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint) (*dynssz.TypeDescriptor, error) {
	// Create descriptor
	codegenInfo := &CodegenInfo{Type: typ}
	var anyCodegenInfo any = codegenInfo
	desc := &dynssz.TypeDescriptor{
		CodegenInfo: &anyCodegenInfo,
	}

	originalType := typ

	// Resolve named types
	if named, ok := typ.(*types.Named); ok {
		typ = named.Underlying()
	}

	// Handle pointer types
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
		desc.GoTypeFlags |= dynssz.GoTypeFlagIsPointer

		// Resolve named types after resolving pointer
		if named, ok := typ.(*types.Named); ok {
			typ = named.Underlying()
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
			desc.GoTypeFlags |= dynssz.GoTypeFlagIsString
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
		fmt.Printf("Unknown type: %T\n", t)
		desc.Kind = reflect.Invalid
	}

	// Check dynamic size and max size hints (like reflection code)
	if len(sizeHints) > 0 {
		if sizeHints[0].Expr != "" {
			desc.SizeExpression = &sizeHints[0].Expr
		}
		for _, hint := range sizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= dynssz.SszTypeFlagHasDynamicSize
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= dynssz.SszTypeFlagHasSizeExpr
			}
		}
	}

	if len(maxSizeHints) > 0 {
		if !maxSizeHints[0].NoValue {
			desc.SszTypeFlags |= dynssz.SszTypeFlagHasLimit
			desc.Limit = maxSizeHints[0].Size
		}
		if maxSizeHints[0].Expr != "" {
			desc.MaxExpression = &maxSizeHints[0].Expr
		}
		for _, hint := range maxSizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= dynssz.SszTypeFlagHasDynamicMax
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= dynssz.SszTypeFlagHasMaxExpr
			}
		}
	}

	// Determine SSZ type - first use type hints if specified
	sszType := dynssz.SszUnspecifiedType
	if len(typeHints) > 0 {
		sszType = typeHints[0].Type
	}

	if desc.Kind == reflect.String {
		desc.GoTypeFlags |= dynssz.GoTypeFlagIsString
	}

	// Auto-detect ssz type if not specified
	if sszType == dynssz.SszUnspecifiedType {
		// Detect well-known types first (named types)
		if named, ok := originalType.(*types.Named); ok {
			obj := named.Obj()
			if obj.Pkg() != nil {
				pkgPath := obj.Pkg().Path()
				typeName := obj.Name()

				switch {
				case pkgPath == "time" && typeName == "Time":
					sszType = dynssz.SszUint64Type
					desc.GoTypeFlags |= dynssz.GoTypeFlagIsTime
				case pkgPath == "github.com/holiman/uint256" && typeName == "Int":
					sszType = dynssz.SszUint256Type
				case pkgPath == "github.com/pk910/dynamic-ssz" && strings.HasPrefix(typeName, "CompatibleUnion["):
					sszType = dynssz.SszCompatibleUnionType
				case pkgPath == "github.com/pk910/dynamic-ssz" && strings.HasPrefix(typeName, "TypeWrapper["):
					sszType = dynssz.SszTypeWrapperType
				}
			}
		}
	}

	if sszType == dynssz.SszUnspecifiedType {
		switch desc.Kind {
		// basic types
		case reflect.Bool:
			sszType = dynssz.SszBoolType
		case reflect.Uint8:
			sszType = dynssz.SszUint8Type
		case reflect.Uint16:
			sszType = dynssz.SszUint16Type
		case reflect.Uint32:
			sszType = dynssz.SszUint32Type
		case reflect.Uint64:
			sszType = dynssz.SszUint64Type

		// complex types
		case reflect.Struct:
			sszType = dynssz.SszContainerType
		case reflect.Array:
			sszType = dynssz.SszVectorType
		case reflect.Slice:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = dynssz.SszVectorType
			} else {
				sszType = dynssz.SszListType
			}
		case reflect.String:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = dynssz.SszVectorType
			} else {
				sszType = dynssz.SszListType
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
	case dynssz.SszBoolType:
		if desc.Kind != reflect.Bool {
			return nil, fmt.Errorf("bool ssz type can only be represented by bool types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("bool ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case dynssz.SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return nil, fmt.Errorf("uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("uint8 ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case dynssz.SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return nil, fmt.Errorf("uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 2 {
			return nil, fmt.Errorf("uint16 ssz type must be ssz-size:2, got %v", sizeHints[0].Size)
		}
		desc.Size = 2
	case dynssz.SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return nil, fmt.Errorf("uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 4 {
			return nil, fmt.Errorf("uint32 ssz type must be ssz-size:4, got %v", sizeHints[0].Size)
		}
		desc.Size = 4
	case dynssz.SszUint64Type:
		if desc.Kind != reflect.Uint64 && desc.GoTypeFlags&dynssz.GoTypeFlagIsTime == 0 {
			return nil, fmt.Errorf("uint64 ssz type can only be represented by uint64 or time.Time types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return nil, fmt.Errorf("uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
		}
		desc.Size = 8
	case dynssz.SszUint128Type:
		err := p.buildUint128Descriptor(desc, typ)
		if err != nil {
			return nil, err
		}
	case dynssz.SszUint256Type:
		err := p.buildUint256Descriptor(desc, typ)
		if err != nil {
			return nil, err
		}

	// complex types
	case dynssz.SszTypeWrapperType:
		if named, ok := originalType.(*types.Named); ok {
			err := p.buildTypeWrapperDescriptor(desc, named, typeHints, sizeHints, maxSizeHints)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("TypeWrapper must be a named type")
		}
	case dynssz.SszContainerType:
		if struc, ok := typ.(*types.Struct); ok {
			err := p.buildContainerDescriptor(desc, struc)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("container ssz type can only be represented by struct types, got %v", desc.Kind)
		}
	case dynssz.SszVectorType:
		err := p.buildVectorDescriptor(desc, typ, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case dynssz.SszListType:
		err := p.buildListDescriptor(desc, typ, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case dynssz.SszBitvectorType:
		err := p.buildBitvectorDescriptor(desc, typ, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case dynssz.SszBitlistType:
		err := p.buildBitlistDescriptor(desc, typ, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case dynssz.SszCompatibleUnionType:
		if named, ok := originalType.(*types.Named); ok {
			err := p.buildCompatibleUnionDescriptor(desc, named)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("CompatibleUnion must be a named type")
		}
	case dynssz.SszCustomType:
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			desc.Size = uint32(sizeHints[0].Size)
		} else {
			desc.Size = 0
			desc.SszTypeFlags |= dynssz.SszTypeFlagIsDynamic
		}
	}

	// Check interface compatibility (like reflection-based code)
	if desc.SszTypeFlags&dynssz.SszTypeFlagHasDynamicSize == 0 && p.getFastsszConvertCompatibility(originalType) {
		desc.SszCompatFlags |= dynssz.SszCompatFlagFastSSZMarshaler
	}
	if desc.SszTypeFlags&dynssz.SszTypeFlagHasDynamicMax == 0 {
		if p.getFastsszHashCompatibility(originalType) {
			desc.SszCompatFlags |= dynssz.SszCompatFlagFastSSZHasher
		}
		if p.getHashTreeRootWithCompatibility(originalType) {
			desc.SszCompatFlags |= dynssz.SszCompatFlagHashTreeRootWith
		}
	}

	// Check for dynamic interface implementations
	if p.getDynamicMarshalerCompatibility(originalType) {
		desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicMarshaler
	}
	if p.getDynamicUnmarshalerCompatibility(originalType) {
		desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicUnmarshaler
	}
	if p.getDynamicSizerCompatibility(originalType) {
		desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicSizer
	}
	if p.getDynamicHashRootCompatibility(originalType) {
		desc.SszCompatFlags |= dynssz.SszCompatFlagDynamicHashRoot
	}

	if desc.SszType == dynssz.SszCustomType && (desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler == 0 || desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZHasher == 0) {
		return nil, fmt.Errorf("custom ssz type requires fastssz marshaler and hasher implementations")
	}

	return desc, nil
}

func (p *Parser) buildUint128Descriptor(desc *dynssz.TypeDescriptor, typ types.Type) error {
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
		desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray
	}

	return nil
}

func (p *Parser) buildUint256Descriptor(desc *dynssz.TypeDescriptor, typ types.Type) error {
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
		desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray
	}

	return nil
}

func (p *Parser) buildContainerDescriptor(desc *dynssz.TypeDescriptor, struc *types.Struct) error {
	fields := []dynssz.FieldDescriptor{}
	dynFields := []dynssz.DynFieldDescriptor{}
	size := uint32(0)
	isDynamic := false

	for i := 0; i < struc.NumFields(); i++ {
		field := struc.Field(i)
		if !field.Exported() || field.Name() == "_" {
			continue
		}

		typeHints, sizeHints, maxSizeHints, skip, err := p.parseFieldTags(struc.Tag(i))
		if err != nil {
			return fmt.Errorf("failed to parse tags for field %v: %v", field.Name(), err)
		}
		if skip {
			continue
		}

		// Build type descriptor with field-specific hints
		typeDesc, err := p.buildTypeDescriptor(field.Type(), typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return fmt.Errorf("failed to build field %v descriptor: %v", field.Name(), err)
		}

		fieldDesc := dynssz.FieldDescriptor{
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

		if typeDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			// Dynamic field
			dynFieldDesc := dynssz.DynFieldDescriptor{
				Field:  &fieldDesc,
				Offset: size,
				Index:  int16(len(fields)),
			}
			dynFields = append(dynFields, dynFieldDesc)
			isDynamic = true
		} else {
			size += typeDesc.Size
		}

		fields = append(fields, fieldDesc)
	}

	containerDesc := &dynssz.ContainerDescriptor{
		Fields:    fields,
		DynFields: dynFields,
	}
	desc.ContainerDesc = containerDesc

	if isDynamic {
		desc.SszTypeFlags |= dynssz.SszTypeFlagIsDynamic
		desc.Size = 0
	} else {
		desc.Size = size
	}

	return nil
}

func (p *Parser) buildVectorDescriptor(desc *dynssz.TypeDescriptor, typ types.Type, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint, typeHints []dynssz.SszTypeHint) error {
	var elemType types.Type
	var length uint32

	switch t := typ.(type) {
	case *types.Array:
		elemType = t.Elem()
		length = uint32(t.Len())
	case *types.Slice:
		elemType = t.Elem()
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			length = sizeHints[0].Size
		} else {
			return fmt.Errorf("vector slice type requires explicit size hint")
		}
	case *types.Basic:
		if t.Kind() == types.String {
			// String as vector
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				length = sizeHints[0].Size
				desc.Size = length
				desc.Len = length
				desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray
				return nil
			} else {
				return fmt.Errorf("string vector type requires explicit size hint")
			}
		}
		return fmt.Errorf("unsupported vector base type: %v", t.Kind())
	default:
		return fmt.Errorf("unsupported vector type: %T", typ)
	}

	childTypeHints := []dynssz.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}
	childSizeHints := []dynssz.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}
	childMaxSizeHints := []dynssz.SszMaxSizeHint{}
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
		desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray
	}

	// Calculate size
	if elemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
		desc.SszTypeFlags |= dynssz.SszTypeFlagIsDynamic
		desc.Size = 0
	} else {
		desc.Size = length * elemDesc.Size
	}

	return nil
}

func (p *Parser) buildListDescriptor(desc *dynssz.TypeDescriptor, typ types.Type, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint, typeHints []dynssz.SszTypeHint) error {
	var elemType types.Type

	switch t := typ.(type) {
	case *types.Slice:
		elemType = t.Elem()
	case *types.Basic:
		if t.Kind() == types.String {
			// String as list - set byte array flag and make dynamic
			desc.SszTypeFlags |= dynssz.SszTypeFlagIsDynamic
			desc.Size = 0
			desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray
			return nil
		}
		return fmt.Errorf("unsupported list base type: %v", t.Kind())
	default:
		return fmt.Errorf("unsupported list type: %T", typ)
	}

	childTypeHints := []dynssz.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}
	childSizeHints := []dynssz.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}
	childMaxSizeHints := []dynssz.SszMaxSizeHint{}
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
		desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray
	}

	// Lists are always dynamic
	desc.SszTypeFlags |= dynssz.SszTypeFlagIsDynamic
	desc.Size = 0

	return nil
}

func (p *Parser) buildBitvectorDescriptor(desc *dynssz.TypeDescriptor, typ types.Type, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint, typeHints []dynssz.SszTypeHint) error {
	var elemType types.Type
	var length uint32

	switch t := typ.(type) {
	case *types.Array:
		elemType = t.Elem()
		length = uint32(t.Len())
	case *types.Slice:
		elemType = t.Elem()
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			length = sizeHints[0].Size
		} else {
			return fmt.Errorf("bitvector slice type requires explicit size hint")
		}
	default:
		return fmt.Errorf("unsupported bitvector type: %T", typ)
	}

	// Build element descriptor
	elemDesc, err := p.buildTypeDescriptor(elemType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to build bitvector element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc

	// Bitvector must use byte (uint8) elements
	if elemDesc.Kind != reflect.Uint8 {
		return fmt.Errorf("bitvector ssz type can only be represented by byte slices or arrays, got %v", elemDesc.Kind)
	}

	desc.Len = length
	// Bitvector size is calculated as (length + 7) / 8 bytes
	desc.Size = (length + 7) / 8
	desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray

	return nil
}

func (p *Parser) buildBitlistDescriptor(desc *dynssz.TypeDescriptor, typ types.Type, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint, typeHints []dynssz.SszTypeHint) error {
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
		return fmt.Errorf("bitlist ssz type can only be represented by byte slices or arrays, got %v", elemDesc.Kind)
	}

	// Bitlists are always dynamic
	desc.SszTypeFlags |= dynssz.SszTypeFlagIsDynamic
	desc.Size = 0
	desc.GoTypeFlags |= dynssz.GoTypeFlagIsByteArray

	return nil
}

func (p *Parser) buildCompatibleUnionDescriptor(desc *dynssz.TypeDescriptor, named *types.Named) error {
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
	variantInfo := make(map[uint8]*dynssz.TypeDescriptor)

	for i := 0; i < descriptorStruct.NumFields(); i++ {
		field := descriptorStruct.Field(i)
		variantIndex := uint8(i) // Field order determines variant index

		// Extract SSZ annotations from the field
		typeHints, sizeHints, maxSizeHints, skip, err := p.parseFieldTags(descriptorStruct.Tag(i))
		if err != nil {
			return fmt.Errorf("failed to parse union variant field %s tags: %v", field.Name(), err)
		}
		if skip {
			continue
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
	desc.SszTypeFlags |= dynssz.SszTypeFlagIsDynamic
	desc.Size = 0

	return nil
}

func (p *Parser) buildTypeWrapperDescriptor(desc *dynssz.TypeDescriptor, named *types.Named, typeHints []dynssz.SszTypeHint, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint) error {
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
	fieldTypeHints, fieldSizeHints, fieldMaxSizeHints, skip, err := p.parseFieldTags(descriptorStruct.Tag(0))
	if err != nil {
		return fmt.Errorf("failed to parse TypeWrapper descriptor field tags: %v", err)
	}
	if skip {
		return fmt.Errorf("TypeWrapper descriptor field cannot be skipped")
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
	desc.SszTypeFlags |= wrappedDesc.SszTypeFlags & (dynssz.SszTypeFlagIsDynamic | dynssz.SszTypeFlagHasDynamicSize | dynssz.SszTypeFlagHasDynamicMax | dynssz.SszTypeFlagHasSizeExpr | dynssz.SszTypeFlagHasMaxExpr)

	return nil
}

func (p *Parser) parseFieldTags(tag string) (typeHints []dynssz.SszTypeHint, sizeHints []dynssz.SszSizeHint, maxSizeHints []dynssz.SszMaxSizeHint, skip bool, err error) {
	if tag == "" {
		return nil, nil, nil, false, nil
	}

	structTag := reflect.StructTag(tag)

	// Check for ssz:"-" which means skip field
	if sszTag, ok := structTag.Lookup("ssz"); ok && sszTag == "-" {
		return nil, nil, nil, true, nil
	}

	// Parse type hints (matching getSszTypeTag logic)
	if sszType, ok := structTag.Lookup("ssz-type"); ok {
		for _, typeStr := range strings.Split(sszType, ",") {
			typeStr = strings.TrimSpace(typeStr)
			hint := dynssz.SszTypeHint{}

			switch typeStr {
			case "?", "auto":
				hint.Type = dynssz.SszUnspecifiedType
			case "custom":
				hint.Type = dynssz.SszCustomType
			case "wrapper", "type-wrapper":
				hint.Type = dynssz.SszTypeWrapperType
			case "bool":
				hint.Type = dynssz.SszBoolType
			case "uint8":
				hint.Type = dynssz.SszUint8Type
			case "uint16":
				hint.Type = dynssz.SszUint16Type
			case "uint32":
				hint.Type = dynssz.SszUint32Type
			case "uint64":
				hint.Type = dynssz.SszUint64Type
			case "uint128":
				hint.Type = dynssz.SszUint128Type
			case "uint256":
				hint.Type = dynssz.SszUint256Type
			case "container":
				hint.Type = dynssz.SszContainerType
			case "list":
				hint.Type = dynssz.SszListType
			case "vector":
				hint.Type = dynssz.SszVectorType
			case "bitlist":
				hint.Type = dynssz.SszBitlistType
			case "bitvector":
				hint.Type = dynssz.SszBitvectorType
			case "progressive-list":
				hint.Type = dynssz.SszProgressiveListType
			case "progressive-bitlist":
				hint.Type = dynssz.SszProgressiveBitlistType
			case "progressive-container":
				hint.Type = dynssz.SszProgressiveContainerType
			case "compatible-union", "union":
				hint.Type = dynssz.SszCompatibleUnionType
			default:
				return nil, nil, nil, false, fmt.Errorf("invalid ssz-type tag: %v", typeStr)
			}

			typeHints = append(typeHints, hint)
		}
	}

	// Parse size hints (matching getSszSizeTag logic)
	if sszSize, ok := structTag.Lookup("ssz-size"); ok {
		for _, sizeStr := range strings.Split(sszSize, ",") {
			sizeStr = strings.TrimSpace(sizeStr)
			hint := dynssz.SszSizeHint{}

			if sizeStr == "?" {
				hint.Dynamic = true
			} else {
				sizeInt, err := strconv.ParseUint(sizeStr, 10, 32)
				if err != nil {
					return nil, nil, nil, false, fmt.Errorf("error parsing ssz-size tag: %v", err)
				}
				hint.Size = uint32(sizeInt)
			}

			sizeHints = append(sizeHints, hint)
		}
	}

	// Parse dynamic size hints
	if dynsszSize, ok := structTag.Lookup("dynssz-size"); ok {
		hint := dynssz.SszSizeHint{
			Custom: true,
			Expr:   dynsszSize,
		}

		// Try to parse as number first
		if sizeInt, err := strconv.ParseUint(dynsszSize, 10, 32); err == nil {
			hint.Size = uint32(sizeInt)
		} else {
			// It's an expression
			hint.Dynamic = true
		}

		sizeHints = append(sizeHints, hint)
	}

	// Parse max size hints (matching getSszMaxSizeTag logic)
	if sszMax, ok := structTag.Lookup("ssz-max"); ok {
		for _, maxStr := range strings.Split(sszMax, ",") {
			maxStr = strings.TrimSpace(maxStr)
			hint := dynssz.SszMaxSizeHint{}

			if maxStr == "?" {
				hint.NoValue = true
			} else {
				maxInt, err := strconv.ParseUint(maxStr, 10, 64)
				if err != nil {
					return nil, nil, nil, false, fmt.Errorf("error parsing ssz-max tag: %v", err)
				}
				hint.Size = maxInt
			}

			maxSizeHints = append(maxSizeHints, hint)
		}
	}

	// Parse dynamic max size hints
	if dynsszMax, ok := structTag.Lookup("dynssz-max"); ok {
		hint := dynssz.SszMaxSizeHint{
			Custom: true,
			Expr:   dynsszMax,
		}

		// Try to parse as number first
		if maxInt, err := strconv.ParseUint(dynsszMax, 10, 64); err == nil {
			hint.Size = maxInt
		} else {
			// It's an expression - for now we can't resolve it without a spec context
			hint.NoValue = true
		}

		maxSizeHints = append(maxSizeHints, hint)
	}

	return typeHints, sizeHints, maxSizeHints, false, nil
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

// Interface compatibility checks (equivalent to reflection-based logic)

func (p *Parser) getFastsszConvertCompatibility(typ types.Type) bool {
	// Check if type implements fastssz.Marshaler interface
	return p.implementsInterface(typ, "github.com/ferranbt/fastssz", "Marshaler")
}

func (p *Parser) getFastsszHashCompatibility(typ types.Type) bool {
	// Check if type implements fastssz.HashRoot interface
	return p.implementsInterface(typ, "github.com/ferranbt/fastssz", "HashRoot")
}

func (p *Parser) getHashTreeRootWithCompatibility(typ types.Type) bool {
	// Check if type implements HashTreeRootWith method
	return p.hasMethod(typ, "HashTreeRootWith")
}

func (p *Parser) getDynamicMarshalerCompatibility(typ types.Type) bool {
	// Check if type implements DynamicMarshaler interface
	return p.implementsInterface(typ, "github.com/pk910/dynamic-ssz", "DynamicMarshaler")
}

func (p *Parser) getDynamicUnmarshalerCompatibility(typ types.Type) bool {
	// Check if type implements DynamicUnmarshaler interface
	return p.implementsInterface(typ, "github.com/pk910/dynamic-ssz", "DynamicUnmarshaler")
}

func (p *Parser) getDynamicSizerCompatibility(typ types.Type) bool {
	// Check if type implements DynamicSizer interface
	return p.implementsInterface(typ, "github.com/pk910/dynamic-ssz", "DynamicSizer")
}

func (p *Parser) getDynamicHashRootCompatibility(typ types.Type) bool {
	// Check if type implements DynamicHashRoot interface
	return p.implementsInterface(typ, "github.com/pk910/dynamic-ssz", "DynamicHashRoot")
}

// Helper methods for interface checking

func (p *Parser) implementsInterface(typ types.Type, pkgPath, interfaceName string) bool {
	// For go/types, we need to check if the type implements the interface
	// This is a simplified check - in a full implementation, you'd need to
	// resolve the interface type and check method signatures

	// Get the method set for the type
	methodSet := types.NewMethodSet(typ)

	// Check for required methods based on interface
	switch interfaceName {
	case "Marshaler":
		// fastssz.Marshaler requires: MarshalSSZ() ([]byte, error)
		return p.hasMethodWithSignature(methodSet, "MarshalSSZ", []string{}, []string{"[]byte", "error"})
	case "HashRoot":
		// fastssz.HashRoot requires: HashTreeRoot() ([32]byte, error)
		return p.hasMethodWithSignature(methodSet, "HashTreeRoot", []string{}, []string{"[32]byte", "error"})
	case "DynamicMarshaler":
		// DynamicMarshaler requires: MarshalDynamicSSZ() ([]byte, error)
		return p.hasMethodWithSignature(methodSet, "MarshalDynamicSSZ", []string{}, []string{"[]byte", "error"})
	case "DynamicUnmarshaler":
		// DynamicUnmarshaler requires: UnmarshalDynamicSSZ([]byte) error
		return p.hasMethodWithSignature(methodSet, "UnmarshalDynamicSSZ", []string{"[]byte"}, []string{"error"})
	case "DynamicSizer":
		// DynamicSizer requires: SizeSSZ() int
		return p.hasMethodWithSignature(methodSet, "SizeSSZ", []string{}, []string{"int"})
	case "DynamicHashRoot":
		// DynamicHashRoot requires: HashTreeRoot() ([32]byte, error)
		return p.hasMethodWithSignature(methodSet, "HashTreeRoot", []string{}, []string{"[32]byte", "error"})
	}

	return false
}

func (p *Parser) hasMethod(typ types.Type, methodName string) bool {
	methodSet := types.NewMethodSet(typ)
	for i := 0; i < methodSet.Len(); i++ {
		if methodSet.At(i).Obj().Name() == methodName {
			return true
		}
	}
	return false
}

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
	}
	return false
}
