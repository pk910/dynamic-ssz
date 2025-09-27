package codegen

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

type marshalContext struct {
	appendCode  func(indent int, code string, args ...any)
	typePrinter *TypePrinter
	options     *CodeGeneratorOptions
	usedDynSsz  bool
}

func generateMarshal(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
	codeBuf := strings.Builder{}
	ctx := &marshalContext{
		appendCode: func(indent int, code string, args ...any) {
			if len(args) > 0 {
				code = fmt.Sprintf(code, args...)
			}
			codeBuf.WriteString(indentStr(code, indent))
		},
		typePrinter: typePrinter,
		options:     options,
	}

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc.Type)

	// Generate marshaling code
	if err := ctx.marshalType(rootTypeDesc, "t", 1, true); err != nil {
		return false, err
	}

	genDynamicFn := !options.WithoutDynamicExpressions
	genStaticFn := options.WithoutDynamicExpressions || options.CreateLegacyFn
	genLegacyFn := options.CreateLegacyFn

	if genDynamicFn {
		if ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) (dst []byte, err error) {\n", typeName))
			codeBuilder.WriteString("\tdst = buf\n")
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn dst, nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) (dst []byte, err error) {\n", typeName))
			codeBuilder.WriteString("\treturn t.MarshalSSZTo(buf)\n")
			codeBuilder.WriteString("}\n\n")
		}
	}

	if genStaticFn {
		if !ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZTo(buf []byte) (dst []byte, err error) {\n", typeName))
			codeBuilder.WriteString("\tdst = buf\n")
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn dst, nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZTo(buf []byte) (dst []byte, err error) {\n", typeName))
			codeBuilder.WriteString(fmt.Sprintf("\treturn t.MarshalSSZDyn(%s.GetGlobalDynSsz(), buf)\n", dynsszAlias))
			codeBuilder.WriteString("}\n\n")
		}
	}

	if genLegacyFn {
		dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
		codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZ() ([]byte, error) {\n", typeName))
		codeBuilder.WriteString(fmt.Sprintf("\treturn %s.GetGlobalDynSsz().MarshalSSZ(t)\n", dynsszAlias))
		codeBuilder.WriteString("}\n")
	}

	return ctx.usedDynSsz, nil
}

func (ctx *marshalContext) marshalType(desc *dynssz.TypeDescriptor, varName string, indent int, isRoot bool) error {
	// Handle types that have generated methods we can call
	hasDynamicSize := desc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszMarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "if dst, err = %s.MarshalSSZTo(dst); err != nil {\n\treturn dst, err\n}\n", varName)
		return nil
	}

	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicMarshaler != 0 && !isRoot {
		ctx.appendCode(indent, "if dst, err = %s.MarshalSSZDyn(ds, dst); err != nil {\n\treturn dst, err\n}\n", varName)
		ctx.usedDynSsz = true
		return nil
	}

	switch desc.SszType {
	case dynssz.SszBoolType:
		ctx.appendCode(indent, "dst = sszutils.MarshalBool(dst, bool(%s))\n", varName)

	case dynssz.SszUint8Type:
		ctx.appendCode(indent, "dst = sszutils.MarshalUint8(dst, uint8(%s))\n", varName)

	case dynssz.SszUint16Type:
		ctx.appendCode(indent, "dst = sszutils.MarshalUint16(dst, uint16(%s))\n", varName)

	case dynssz.SszUint32Type:
		ctx.appendCode(indent, "dst = sszutils.MarshalUint32(dst, uint32(%s))\n", varName)

	case dynssz.SszUint64Type:
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsTime != 0 {
			ctx.appendCode(indent, "dst = sszutils.MarshalUint64(dst, uint64(%s.Unix()))\n", varName)
		} else {
			ctx.appendCode(indent, "dst = sszutils.MarshalUint64(dst, uint64(%s))\n", varName)
		}

	case dynssz.SszTypeWrapperType:
		ctx.appendCode(indent, "{\n\tt := %s.Data\n", varName)
		if err := ctx.marshalType(desc.ElemDesc, "t", indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

	case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
		return ctx.marshalContainer(desc, varName, indent)

	case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
		return ctx.marshalVector(desc, varName, indent)

	case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
		return ctx.marshalList(desc, varName, indent)

	case dynssz.SszCompatibleUnionType:
		return ctx.marshalUnion(desc, varName, indent)

	case dynssz.SszCustomType:
		ctx.appendCode(indent, "return dst, sszutils.ErrNotImplemented\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

func (ctx *marshalContext) marshalContainer(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	hasDynamic := false
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			hasDynamic = true
			break
		}
	}

	if hasDynamic {
		ctx.appendCode(indent, "dstlen := len(dst)\n")
	}

	// Write offsets for dynamic fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			ctx.appendCode(indent, "// Offset #%d '%s'\n", idx, field.Name)
			ctx.appendCode(indent, "offset%d := len(dst)\n", idx)
			ctx.appendCode(indent, "dst = sszutils.MarshalOffset(dst, 0)\n")
		} else {
			// Marshal fixed fields
			ctx.appendCode(indent, "{ // Field #%d '%s'\n", idx, field.Name)
			ctx.appendCode(indent, "\tt := %s.%s\n", varName, field.Name)
			if err := ctx.marshalType(field.Type, "t", indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	// Marshal dynamic fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			ctx.appendCode(indent, "{ // Dynamic Field #%d '%s'\n", idx, field.Name)
			ctx.appendCode(indent, "\tsszutils.UpdateOffset(dst[offset%d:offset%d+4], len(dst)-dstlen)\n", idx, idx)
			ctx.appendCode(indent, "\tt := %s.%s\n", varName, field.Name)
			if err := ctx.marshalType(field.Type, "t", indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

func (ctx *marshalContext) marshalVector(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	if sizeExpression != nil {
		ctx.usedDynSsz = true
		ctx.appendCode(indent, "hasLimit, limit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn dst, err\n")
		ctx.appendCode(indent, "}\n")
		ctx.appendCode(indent, "if !hasLimit {\n")
		ctx.appendCode(indent, "\tlimit = %d\n", desc.Len)
		ctx.appendCode(indent, "}\n")
	} else {
		ctx.appendCode(indent, "limit := %d\n", desc.Len)
	}

	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "vlen := len(%s)\n", varName)
		ctx.appendCode(indent, "if vlen > int(limit) {\n")
		ctx.appendCode(indent, "\treturn dst, sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			ctx.appendCode(indent, "dst = append(dst, %s[:limit]...)\n", varName)
		} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			ctx.appendCode(indent, "dst = append(dst, []byte(%s[:limit])...)\n", varName)
		} else {
			capVar := "vlen"
			if desc.Kind == reflect.Array {
				capVar = "limit"
			}
			ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", capVar)
			ctx.appendCode(indent, "\tt := %s[i]\n", varName)
			if err := ctx.marshalType(desc.ElemDesc, "t", indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}

		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "if vlen < int(limit) {\n")
			ctx.appendCode(indent, "\tdst = sszutils.AppendZeroPadding(dst, (int(limit)-vlen)*%d)\n", desc.ElemDesc.Size)
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements
		// reserve space for offsets
		ctx.appendCode(indent, "dstlen := len(dst)\n")
		ctx.appendCode(indent, "dst = sszutils.AppendZeroPadding(dst, limit*4)\n")
		ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
		ctx.appendCode(indent, "\tsszutils.UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)\n")
		ctx.appendCode(indent, "\tt := %s[i]\n", varName)
		if err := ctx.marshalType(desc.ElemDesc, "t", indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

		ctx.appendCode(indent, "if vlen < int(limit) {\n")
		typeName := ctx.typePrinter.TypeString(desc.ElemDesc.Type)
		ctx.appendCode(indent, "\tzeroItem := &%s{}\n", typeName)
		ctx.appendCode(indent, "\tfor i := vlen; i < int(limit); i++ {\n")
		ctx.appendCode(indent, "\t\tsszutils.UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)\n")
		if err := ctx.marshalType(desc.ElemDesc, "zeroItem", indent+2, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

func (ctx *marshalContext) marshalList(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	if maxExpression != nil {
		ctx.usedDynSsz = true
		ctx.appendCode(indent, "hasMax, max, err := ds.ResolveSpecValue(\"%s\")\n", *maxExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn dst, err\n")
		ctx.appendCode(indent, "}\n")
		if desc.Limit > 0 {
			ctx.appendCode(indent, "if !hasMax {\n")
			ctx.appendCode(indent, "\tmax = %d\n", desc.Limit)
			ctx.appendCode(indent, "}\n")
		}
	} else if desc.Limit > 0 {
		ctx.appendCode(indent, "max := %d\n", desc.Limit)
		ctx.appendCode(indent, "hasMax := true\n")
	} else {
		ctx.appendCode(indent, "max := 0\n")
		ctx.appendCode(indent, "hasMax := false\n")
	}

	ctx.appendCode(indent, "vlen := len(%s)\n", varName)
	ctx.appendCode(indent, "if hasMax && vlen > int(max) {\n")
	ctx.appendCode(indent, "\treturn dst, sszutils.ErrListTooBig\n")
	ctx.appendCode(indent, "}\n")

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			ctx.appendCode(indent, "dst = append(dst, %s[:]...)\n", varName)
		} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			ctx.appendCode(indent, "dst = append(dst, []byte(%s[:])...)\n", varName)
		} else {
			ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
			ctx.appendCode(indent, "\tt := %s[i]\n", varName)
			if err := ctx.marshalType(desc.ElemDesc, "t", indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements
		// reserve space for offsets
		ctx.appendCode(indent, "dstlen := len(dst)\n")
		ctx.appendCode(indent, "dst = sszutils.AppendZeroPadding(dst, vlen*4)\n")
		ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
		ctx.appendCode(indent, "\tsszutils.UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)\n")
		ctx.appendCode(indent, "\tt := %s[i]\n", varName)
		if err := ctx.marshalType(desc.ElemDesc, "t", indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

func (ctx *marshalContext) marshalUnion(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	ctx.appendCode(indent, "dst = sszutils.MarshalUint8(dst, %s.Variant)\n", varName)
	ctx.appendCode(indent, "switch %s.Variant {\n", varName)

	variants := make([]int, 0, len(desc.UnionVariants))
	for variant := range desc.UnionVariants {
		variants = append(variants, int(variant))
	}
	slices.Sort(variants)

	for _, variant := range variants {
		variantDesc := desc.UnionVariants[uint8(variant)]
		variantType := ctx.typePrinter.TypeString(variantDesc.Type)
		ctx.appendCode(indent, "case %d:\n", variant)
		ctx.appendCode(indent, "\tv, ok := %s.Data.(%s)\n", varName, variantType)
		ctx.appendCode(indent, "\tif !ok {\n")
		ctx.appendCode(indent, "\t\treturn dst, sszutils.ErrInvalidUnionVariant\n")
		ctx.appendCode(indent, "\t}\n")
		if err := ctx.marshalType(variantDesc, "v", indent+1, false); err != nil {
			return err
		}
	}
	ctx.appendCode(indent, "default:\n")
	ctx.appendCode(indent, "\treturn dst, sszutils.ErrInvalidUnionVariant\n")
	ctx.appendCode(indent, "}\n")

	return nil
}
