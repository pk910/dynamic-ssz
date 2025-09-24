package codegen

import (
	"fmt"
	"reflect"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

type sizeContext struct {
	appendCode  func(indent int, code string, args ...any)
	typePrinter *TypePrinter
	options     *CodeGeneratorOptions
	usedDynSsz  bool
}

func generateSizeFlat(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
	codeBuf := strings.Builder{}
	ctx := &sizeContext{
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

	// Generate size calculation code
	if err := ctx.sizeType(rootTypeDesc, "t", 1, true); err != nil {
		return false, err
	}

	genDynamicFn := !options.WithoutDynamicExpressions
	genStaticFn := options.WithoutDynamicExpressions || options.CreateLegacyFn

	if genDynamicFn {
		if ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) SizeSSZDyn(ds sszutils.DynamicSpecs) (size int) {\n", typeName))
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn size\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) SizeSSZDyn(_ sszutils.DynamicSpecs) (size int) {\n", typeName))
			codeBuilder.WriteString("\treturn t.SizeSSZ()\n")
			codeBuilder.WriteString("}\n\n")
		}
	}

	if genStaticFn {
		if !ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) SizeSSZ() (size int) {\n", typeName))
			if rootTypeDesc.Size > 0 {
				codeBuilder.WriteString(fmt.Sprintf("\treturn %d\n", rootTypeDesc.Size))
			} else {
				codeBuilder.WriteString(codeBuf.String())
				codeBuilder.WriteString("\treturn size\n")
			}
			codeBuilder.WriteString("}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) SizeSSZ() (size int) {\n", typeName))
			codeBuilder.WriteString(fmt.Sprintf("\treturn t.SizeSSZDyn(%s.GetGlobalDynSsz())\n", dynsszAlias))
			codeBuilder.WriteString("}\n\n")
		}
	}

	return ctx.usedDynSsz, nil
}

func (ctx *sizeContext) sizeType(desc *dynssz.TypeDescriptor, varName string, indent int, isRoot bool) error {
	// Handle types that have generated methods we can call
	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicSizer != 0 && !isRoot {
		ctx.appendCode(indent, "size += %s.SizeSSZDyn(ds)\n", varName)
		ctx.usedDynSsz = true
		return nil
	}

	useFastSsz := !ctx.options.NoFastSsz && desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	if !useFastSsz && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "size += %s.SizeSSZ()\n", varName)
		return nil
	}

	switch desc.SszType {
	case dynssz.SszBoolType:
		ctx.appendCode(indent, "size += 1\n")

	case dynssz.SszUint8Type:
		ctx.appendCode(indent, "size += 1\n")

	case dynssz.SszUint16Type:
		ctx.appendCode(indent, "size += 2\n")

	case dynssz.SszUint32Type:
		ctx.appendCode(indent, "size += 4\n")

	case dynssz.SszUint64Type:
		ctx.appendCode(indent, "size += 8\n")

	case dynssz.SszTypeWrapperType:
		ctx.appendCode(indent, "{\n\tt := %s.Data\n", varName)
		if err := ctx.sizeType(desc.ElemDesc, "t", indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

	case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
		return ctx.sizeContainer(desc, varName, indent)

	case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
		return ctx.sizeVector(desc, varName, indent)

	case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
		return ctx.sizeList(desc, varName, indent)

	case dynssz.SszCompatibleUnionType:
		return ctx.sizeUnion(desc, varName, indent)

	case dynssz.SszCustomType:
		ctx.appendCode(indent, "// Custom type - size unknown\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

func (ctx *sizeContext) sizeContainer(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	// Fixed part size
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			// Dynamic field - add offset size
			ctx.appendCode(indent, "size += 4 // Offset for field #%d '%s'\n", idx, field.Name)
		} else {
			// Fixed field - add its size directly
			if field.Type.Size > 0 && field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr == 0 {
				ctx.appendCode(indent, "size += %d // Field #%d '%s'\n", field.Type.Size, idx, field.Name)
			} else {
				// Need to calculate size
				ctx.appendCode(indent, "{ // Field #%d '%s'\n", idx, field.Name)
				if err := ctx.sizeType(field.Type, fmt.Sprintf("%s.%s", varName, field.Name), indent+1, false); err != nil {
					return err
				}
				ctx.appendCode(indent, "}\n")
			}
		}
	}

	// Dynamic part size
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			ctx.appendCode(indent, "{ // Dynamic field #%d '%s'\n", idx, field.Name)
			if err := ctx.sizeType(field.Type, fmt.Sprintf("%s.%s", varName, field.Name), indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

func (ctx *sizeContext) sizeVector(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	if sizeExpression != nil {
		ctx.usedDynSsz = true
		ctx.appendCode(indent, "hasLimit, limit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn 0\n")
		ctx.appendCode(indent, "}\n")
		ctx.appendCode(indent, "if !hasLimit {\n")
		ctx.appendCode(indent, "\tlimit = %d\n", desc.Len)
		ctx.appendCode(indent, "}\n")
	} else {
		ctx.appendCode(indent, "limit := %d\n", desc.Len)
	}

	if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		// For byte arrays, size is just the vector length
		ctx.appendCode(indent, "size += int(limit)\n")
	} else if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// Fixed size elements - simple multiplication
		ctx.appendCode(indent, "size += int(limit) * %d\n", desc.ElemDesc.Size)
	} else {
		// Dynamic size elements - need to iterate
		if desc.Kind == reflect.Array {
			ctx.appendCode(indent, "for i := 0; i < int(limit); i++ {\n")
			ctx.appendCode(indent, "\tt := %s[i]\n", varName)
			if err := ctx.sizeType(desc.ElemDesc, "t", indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		} else {
			ctx.appendCode(indent, "vlen := len(%s)\n", varName)
			ctx.appendCode(indent, "if vlen > int(limit) {\n")
			ctx.appendCode(indent, "\tvlen = int(limit)\n")
			ctx.appendCode(indent, "}\n")
			ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
			ctx.appendCode(indent, "\tt := %s[i]\n", varName)
			if err := ctx.sizeType(desc.ElemDesc, "t", indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")

			// Add size for zero-padding
			ctx.appendCode(indent, "if vlen < int(limit) {\n")
			typeName := ctx.typePrinter.TypeString(desc.ElemDesc.Type)
			ctx.appendCode(indent, "\tzeroItem := &%s{}\n", typeName)
			ctx.appendCode(indent, "\tfor i := vlen; i < int(limit); i++ {\n")
			if err := ctx.sizeType(desc.ElemDesc, "zeroItem", indent+2, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t}\n")
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

func (ctx *sizeContext) sizeList(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	// For byte slices, size is just the length
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		ctx.appendCode(indent, "size += len(%s)\n", varName)
		return nil
	}

	ctx.appendCode(indent, "vlen := len(%s)\n", varName)

	// Handle lists with dynamic elements
	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
		// Add offset space
		ctx.appendCode(indent, "size += vlen * 4 // Offsets\n")

		// Add size of each element
		ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
		ctx.appendCode(indent, "\tt := %s[i]\n", varName)
		if err := ctx.sizeType(desc.ElemDesc, "t", indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// Fixed size elements
		if desc.ElemDesc.Size > 0 {
			ctx.appendCode(indent, "size += vlen * %d\n", desc.ElemDesc.Size)
		} else {
			ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
			ctx.appendCode(indent, "\tt := %s[i]\n", varName)
			if err := ctx.sizeType(desc.ElemDesc, "t", indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

func (ctx *sizeContext) sizeUnion(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	ctx.appendCode(indent, "size += 1 // Union selector\n")
	ctx.appendCode(indent, "switch %s.Variant {\n", varName)

	for variant, variantDesc := range desc.UnionVariants {
		variantType := ctx.typePrinter.TypeString(variantDesc.Type)
		ctx.appendCode(indent, "case %d:\n", variant)
		ctx.appendCode(indent, "\tv, ok := %s.Data.(%s)\n", varName, variantType)
		ctx.appendCode(indent, "\tif !ok {\n")
		ctx.appendCode(indent, "\t\treturn 0\n")
		ctx.appendCode(indent, "\t}\n")
		if err := ctx.sizeType(variantDesc, "v", indent+1, false); err != nil {
			return err
		}
	}

	ctx.appendCode(indent, "}\n")

	return nil
}
