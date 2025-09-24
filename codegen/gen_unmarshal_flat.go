package codegen

import (
	"fmt"
	"reflect"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

type unmarshalContext struct {
	appendCode     func(indent int, code string, args ...any)
	appendSizeCode func(indent int, code string, args ...any)
	typePrinter    *TypePrinter
	options        *CodeGeneratorOptions
	usedDynSsz     bool
	valVarCounter  int
	sizeVarMap     map[*dynssz.TypeDescriptor]string
}

func generateUnmarshalFlat(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
	codeBuf := strings.Builder{}
	sizeCodeBuf := strings.Builder{}
	ctx := &unmarshalContext{
		appendCode: func(indent int, code string, args ...any) {
			if len(args) > 0 {
				code = fmt.Sprintf(code, args...)
			}
			codeBuf.WriteString(indentStr(code, indent))
		},
		appendSizeCode: func(indent int, code string, args ...any) {
			if len(args) > 0 {
				code = fmt.Sprintf(code, args...)
			}
			sizeCodeBuf.WriteString(indentStr(code, indent))
		},
		typePrinter: typePrinter,
		options:     options,
		sizeVarMap:  make(map[*dynssz.TypeDescriptor]string),
	}

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc.Type)

	// Generate unmarshal code
	if err := ctx.unmarshalType(rootTypeDesc, "t", 1, true, false); err != nil {
		return false, err
	}

	genDynamicFn := !options.WithoutDynamicExpressions
	genStaticFn := options.WithoutDynamicExpressions || options.CreateLegacyFn

	if genDynamicFn {
		if ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) (err error) {\n", typeName))
			codeBuilder.WriteString(sizeCodeBuf.String())
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) (err error) {\n", typeName))
			codeBuilder.WriteString("\treturn t.UnmarshalSSZ(buf)\n")
			codeBuilder.WriteString("}\n\n")
		}
	}

	if genStaticFn {
		if !ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) UnmarshalSSZ(buf []byte) (err error) {\n", typeName))
			codeBuilder.WriteString(sizeCodeBuf.String())
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) UnmarshalSSZ(buf []byte) (err error) {\n", typeName))
			codeBuilder.WriteString(fmt.Sprintf("\treturn t.UnmarshalSSZDyn(%s.GetGlobalDynSsz(), buf)\n", dynsszAlias))
			codeBuilder.WriteString("}\n\n")
		}
	}

	return ctx.usedDynSsz, nil
}

func (ctx *unmarshalContext) getValVar() string {
	ctx.valVarCounter++
	return fmt.Sprintf("val%d", ctx.valVarCounter)
}

func (ctx *unmarshalContext) isInlinable(desc *dynssz.TypeDescriptor) bool {
	// Inline primitive types
	if desc.SszType == dynssz.SszBoolType || desc.SszType == dynssz.SszUint8Type || desc.SszType == dynssz.SszUint16Type || desc.SszType == dynssz.SszUint32Type || desc.SszType == dynssz.SszUint64Type {
		return true
	}

	// Inline byte arrays/slices
	if (desc.SszType == dynssz.SszVectorType || desc.SszType == dynssz.SszListType) && desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		return true
	}

	// Inline types with fastssz unmarshaler
	hasDynamicSize := desc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszUnmarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}
	if useFastSsz {
		return true
	}

	// Inline types with generated unmarshal methods
	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicUnmarshaler != 0 {
		return true
	}

	return false
}

func (ctx *unmarshalContext) getStaticSizeVar(desc *dynssz.TypeDescriptor) (string, error) {
	if sizeVar, ok := ctx.sizeVarMap[desc]; ok {
		return sizeVar, nil
	}

	sizeVar := fmt.Sprintf("size%d", len(ctx.sizeVarMap)+1)
	var err error

	// recursive resolve static size with size expressions
	switch desc.SszType {
	case dynssz.SszTypeWrapperType:
		sizeVar, err = ctx.getStaticSizeVar(desc.ElemDesc)
		if err != nil {
			return "", err
		}
	case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
		fieldSizeVars := []string{}
		staticSize := 0
		for _, field := range desc.ContainerDesc.Fields {
			if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
				return "", fmt.Errorf("dynamic field not supported for static size calculation")
			} else if field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
				fieldSizeVar, err := ctx.getStaticSizeVar(field.Type)
				if err != nil {
					return "", err
				}

				fieldSizeVars = append(fieldSizeVars, fieldSizeVar)
			} else {
				staticSize += int(field.Type.Size)
			}
		}

		fieldSizeVars = append(fieldSizeVars, fmt.Sprintf("%d", staticSize))
		if len(fieldSizeVars) == 1 {
			return fieldSizeVars[0], nil
		}
		ctx.appendSizeCode(1, "%s := %s // size expression for '%s'\n", sizeVar, strings.Join(fieldSizeVars, "+"), ctx.typePrinter.TypeStringWithoutTracking(desc.Type))
	case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
		sizeExpression := desc.SizeExpression
		if ctx.options.WithoutDynamicExpressions {
			sizeExpression = nil
		}

		if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			return "", fmt.Errorf("dynamic vector not supported for static size calculation")
		} else {
			itemSizeVar := ""
			if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
				itemSizeVar, err = ctx.getStaticSizeVar(desc.ElemDesc)
				if err != nil {
					return "", err
				}
			} else {
				itemSizeVar = fmt.Sprintf("%d", desc.ElemDesc.Size)
			}

			if sizeExpression != nil {
				ctx.appendSizeCode(1, "%s := %s // size expression for '%s'\n", sizeVar, itemSizeVar, ctx.typePrinter.TypeStringWithoutTracking(desc.Type))
				ctx.appendSizeCode(1, "{\n")
				ctx.appendSizeCode(2, "hasLimit, limit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
				ctx.appendSizeCode(2, "if err != nil {\n\treturn err\n}\n")
				ctx.appendSizeCode(2, "if !hasLimit {\n\tlimit = %d\n}\n", desc.Len)
				ctx.appendSizeCode(2, "%s = %s * int(limit)\n", sizeVar, sizeVar)
				ctx.appendSizeCode(1, "}\n")
				ctx.usedDynSsz = true
			} else {
				ctx.appendSizeCode(1, "%s := %s * %d\n", sizeVar, itemSizeVar, desc.Len)
			}
		}

	default:
		return "", fmt.Errorf("unknown type for static size calculation: %v", desc.SszType)
	}

	ctx.sizeVarMap[desc] = sizeVar

	return sizeVar, nil
}

func (ctx *unmarshalContext) unmarshalType(desc *dynssz.TypeDescriptor, varName string, indent int, isRoot bool, noBufCheck bool) error {
	// Handle types that have generated methods we can call
	hasDynamicSize := desc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszUnmarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "if err = %s.UnmarshalSSZ(buf); err != nil {\n\treturn err\n}\n", varName)
		return nil
	}

	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicUnmarshaler != 0 && !isRoot {
		ctx.appendCode(indent, "if err = %s.UnmarshalSSZDyn(ds, buf); err != nil {\n\treturn err\n}\n", varName)
		ctx.usedDynSsz = true
		return nil
	}

	switch desc.SszType {
	case dynssz.SszBoolType:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
		}
		ptrVarName := varName
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent, "%s = %s(sszutils.UnmarshalBool(buf))\n", ptrVarName, ctx.typePrinter.TypeString(desc.Type))
	case dynssz.SszUint8Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
		}
		ptrVarName := varName
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent, "%s = %s(sszutils.UnmarshallUint8(buf))\n", ptrVarName, ctx.typePrinter.TypeString(desc.Type))

	case dynssz.SszUint16Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 2 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
		}
		ptrVarName := varName
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent, "%s = %s(sszutils.UnmarshallUint16(buf))\n", ptrVarName, ctx.typePrinter.TypeString(desc.Type))

	case dynssz.SszUint32Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 4 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
		}
		ptrVarName := varName
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent, "%s = %s(sszutils.UnmarshallUint32(buf))\n", ptrVarName, ctx.typePrinter.TypeString(desc.Type))

	case dynssz.SszUint64Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 8 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
		}
		ptrVarName := varName
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsTime != 0 {
			ctx.appendCode(indent, "%s = %s(time.Unix(int64(sszutils.UnmarshallUint64(buf)), 0).UTC())\n", ptrVarName, ctx.typePrinter.TypeString(desc.Type))
			ctx.typePrinter.AddImport("time", "time")
		} else {
			ctx.appendCode(indent, "%s = %s(sszutils.UnmarshallUint64(buf))\n", ptrVarName, ctx.typePrinter.TypeString(desc.Type))
		}

	case dynssz.SszTypeWrapperType:
		if err := ctx.unmarshalType(desc.ElemDesc, fmt.Sprintf("%s.Data", varName), indent+1, false, noBufCheck); err != nil {
			return err
		}

	case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
		return ctx.unmarshalContainer(desc, varName, indent)

	case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
		return ctx.unmarshalVector(desc, varName, indent, noBufCheck)

	case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
		return ctx.unmarshalList(desc, varName, indent)

	case dynssz.SszCompatibleUnionType:
		return ctx.unmarshalUnion(desc, varName, indent)

	case dynssz.SszCustomType:
		ctx.appendCode(indent, "return sszutils.ErrNotImplemented\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

func (ctx *unmarshalContext) unmarshalContainer(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	staticSize := 0
	staticSizeVars := []string{}
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			staticSize += 4
		} else {
			if field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 {
				sizeVar, err := ctx.getStaticSizeVar(field.Type)
				if err != nil {
					return err
				}
				staticSizeVars = append(staticSizeVars, sizeVar)
			} else {
				staticSize += int(field.Type.Size)
			}
		}
	}
	staticSizeVars = append(staticSizeVars, fmt.Sprintf("%d", staticSize))

	if len(staticSizeVars) > 1 {
		ctx.appendCode(indent, "exproffset := 0\n")
	}

	// Read fixed fields and offsets
	offset := 0
	offsetPrefix := ""
	ctx.appendCode(indent, "buflen := len(buf)\n")
	ctx.appendCode(indent, "if buflen < %s {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", strings.Join(staticSizeVars, "+"))
	dynamicFields := make([]int, 0)

	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			// Read offset
			ctx.appendCode(indent, "// Field #%d '%s' (offset)\n", idx, field.Name)
			fmtSpace := ""
			if offsetPrefix != "" {
				fmtSpace = " "
			}
			ctx.appendCode(indent, "offset%d := int(sszutils.UnmarshallUint32(buf[%s%d%s:%s%s%d]))\n", idx, offsetPrefix, offset, fmtSpace, fmtSpace, offsetPrefix, offset+4)
			if len(dynamicFields) > 0 {
				ctx.appendCode(indent, "if offset%d < offset%d || offset%d > buflen {\n\treturn sszutils.ErrOffset\n}\n", idx, dynamicFields[len(dynamicFields)-1], idx)
			} else {
				ctx.appendCode(indent, "if offset%d < %s || offset%d > buflen {\n\treturn sszutils.ErrOffset\n}\n", idx, strings.Join(staticSizeVars, "+"), idx)
			}
			offset += 4
			dynamicFields = append(dynamicFields, idx)
		} else {
			// Unmarshal fixed field
			ctx.appendCode(indent, "{ // Field #%d '%s' (static)\n", idx, field.Name)
			if field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 {
				fieldSizeVar, err := ctx.getStaticSizeVar(field.Type)
				if err != nil {
					return err
				}
				ctx.appendCode(indent, "\tbuf := buf[%s%d : %s%s+%d]\n", offsetPrefix, offset, offsetPrefix, fieldSizeVar, offset)
				ctx.appendCode(indent, "\texproffset += int(%s)\n", fieldSizeVar)
				offsetPrefix = "exproffset+"
			} else {
				fmtSpace := ""
				if offsetPrefix != "" {
					fmtSpace = " "
				}
				ctx.appendCode(indent, "\tbuf := buf[%s%d%s:%s%s%d]\n", offsetPrefix, offset, fmtSpace, fmtSpace, offsetPrefix, offset+int(field.Type.Size))
				offset += int(field.Type.Size)
			}

			valVar := fmt.Sprintf("%s.%s", varName, field.Name)
			isInlinable := ctx.isInlinable(field.Type)
			if !isInlinable {
				valVar = ctx.getValVar()
				ctx.appendCode(indent, "\t%s := %s.%s\n", valVar, varName, field.Name)
			}
			if field.Type.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.TypeString(field.Type.Type.Elem()))
			}

			if err := ctx.unmarshalType(field.Type, valVar, indent+1, false, true); err != nil {
				return err
			}

			if !isInlinable {
				ctx.appendCode(indent, "\t%s.%s = %s\n", varName, field.Name, valVar)
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	// Read dynamic fields
	for idx, fieldIdx := range dynamicFields {
		field := desc.ContainerDesc.Fields[fieldIdx]
		ctx.appendCode(indent, "{ // Field #%d '%s' (dynamic)\n", fieldIdx, field.Name)

		endOffset := ""
		if idx < len(dynamicFields)-1 {
			endOffset = fmt.Sprintf("offset%d", dynamicFields[idx+1])
		}
		ctx.appendCode(indent, "\tbuf := buf[offset%d:%s]\n", fieldIdx, endOffset)

		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\t%s := %s.%s\n", valVar, varName, field.Name)

		if field.Type.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.TypeString(field.Type.Type.Elem()))
		}

		if err := ctx.unmarshalType(field.Type, valVar, indent+1, false, true); err != nil {
			return err
		}

		ctx.appendCode(indent, "\t%s.%s = %s\n", varName, field.Name, valVar)
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

func (ctx *unmarshalContext) unmarshalVector(desc *dynssz.TypeDescriptor, varName string, indent int, noBufCheck bool) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	if sizeExpression != nil {
		ctx.usedDynSsz = true
		ctx.appendCode(indent, "hasLimit, limit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
		ctx.appendCode(indent, "if err != nil {\n\treturn err\n}\n")
		ctx.appendCode(indent, "if !hasLimit {\n\tlimit = %d\n}\n", desc.Len)
		limitVar = "int(limit)"
	} else {
		limitVar = fmt.Sprintf("%d", desc.Len)
	}

	// create slice if needed
	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "if len(%s) < %s {\n", varName, limitVar)
		ctx.appendCode(indent, "\t%s = make(%s, %s)\n", varName, ctx.typePrinter.TypeString(desc.Type), limitVar)
		ctx.appendCode(indent, "} else if len(%s) > %s {\n", varName, limitVar)
		ctx.appendCode(indent, "\t%s = %s[:%s]\n", varName, varName, limitVar)
		ctx.appendCode(indent, "}\n")
	}

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			if !noBufCheck {
				ctx.appendCode(indent, "if %s > len(buf) {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", limitVar)
			}
			if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
				typename := ctx.typePrinter.TypeString(desc.Type)
				ctx.appendCode(indent, "%s = %s(buf)\n", varName, typename)
			} else {
				ctx.appendCode(indent, "copy(%s[:], buf)\n", varName)
			}
			return nil
		}

		// static elements
		var fieldSizeVar string
		var err error
		if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 {
			fieldSizeVar, err = ctx.getStaticSizeVar(desc.ElemDesc)
			if err != nil {
				return err
			}
		} else {
			fieldSizeVar = fmt.Sprintf("%d", desc.ElemDesc.Size)
		}

		if !noBufCheck {
			ctx.appendCode(indent, "if %s*%s > len(buf) {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", limitVar, fieldSizeVar)
		}

		ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", limitVar)

		valVar := fmt.Sprintf("%s[i]", varName)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent, "\t%s := %s[i]\n", valVar, varName)
		}
		if desc.ElemDesc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.TypeString(desc.ElemDesc.Type.Elem()))
		}

		ctx.appendCode(indent, "\tbuf := buf[%s*i : %s*(i+1)]\n", fieldSizeVar, fieldSizeVar)
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}

		if !isInlinable {
			ctx.appendCode(indent, "\t%s[i] = %s\n", varName, valVar)
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// dynamic elements
		ctx.appendCode(indent, "if %s*4 > len(buf) {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", limitVar)
		ctx.appendCode(indent, "startOffset := int(sszutils.UnmarshallUint32(buf[0:4]))\n")
		ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", limitVar)
		ctx.appendCode(indent, "\tvar endOffset int\n")
		ctx.appendCode(indent, "\tif i < %s-1 {\n", limitVar)
		ctx.appendCode(indent, "\t\tendOffset = int(sszutils.UnmarshallUint32(buf[(i+1)*4 : (i+2)*4]))\n")
		ctx.appendCode(indent, "\t} else {\n")
		ctx.appendCode(indent, "\t\tendOffset = len(buf)\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tif endOffset < startOffset || endOffset > len(buf) {\n")
		ctx.appendCode(indent, "\t\treturn sszutils.ErrOffset\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tbuf := buf[startOffset:endOffset]\n")
		ctx.appendCode(indent, "\tstartOffset = endOffset\n")

		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\t%s := %s[i]\n", valVar, varName)
		if desc.ElemDesc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.TypeString(desc.ElemDesc.Type.Elem()))
		}
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t%s[i] = %s\n", varName, valVar)
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

func (ctx *unmarshalContext) unmarshalList(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
				typename := ctx.typePrinter.TypeString(desc.Type)
				ctx.appendCode(indent, "%s = %s(buf)\n", varName, typename)
			} else {
				if desc.Kind != reflect.Array {
					ctx.appendCode(indent, "limit := len(buf)\n")
					ctx.appendCode(indent, "if len(%s) < limit {\n", varName)
					ctx.appendCode(indent, "\t%s = make(%s, limit)\n", varName, ctx.typePrinter.TypeString(desc.Type))
					ctx.appendCode(indent, "} else if len(%s) > limit {\n", varName)
					ctx.appendCode(indent, "\t%s = %s[:limit]\n", varName, varName)
					ctx.appendCode(indent, "}\n")
				}
				ctx.appendCode(indent, "copy(%s[:], buf)\n", varName)
			}
			return nil
		}

		// static elements
		var fieldSizeVar string
		var err error
		if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 {
			fieldSizeVar, err = ctx.getStaticSizeVar(desc.ElemDesc)
			if err != nil {
				return err
			}
		} else {
			fieldSizeVar = fmt.Sprintf("%d", desc.ElemDesc.Size)
		}

		if fieldSizeVar == "1" {
			ctx.appendCode(indent, "itemCount := len(buf)\n")
		} else {
			ctx.appendCode(indent, "itemCount := len(buf) / %s\n", fieldSizeVar)
			ctx.appendCode(indent, "if len(buf)%%%s != 0 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", fieldSizeVar)
		}
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "if len(%s) < itemCount {\n", varName)
			ctx.appendCode(indent, "\t%s = make(%s, itemCount)\n", varName, ctx.typePrinter.TypeString(desc.Type))
			ctx.appendCode(indent, "} else if len(%s) > itemCount {\n", varName)
			ctx.appendCode(indent, "\t%s = %s[:itemCount]\n", varName, varName)
			ctx.appendCode(indent, "}\n")
		}

		ctx.appendCode(indent, "for i := 0; i < itemCount; i++ {\n")

		valVar := fmt.Sprintf("%s[i]", varName)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent, "\t%s := %s[i]\n", valVar, varName)
		}
		if desc.ElemDesc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.TypeString(desc.ElemDesc.Type.Elem()))
		}

		ctx.appendCode(indent, "\tbuf := buf[%s*i : %s*(i+1)]\n", fieldSizeVar, fieldSizeVar)
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}

		if !isInlinable {
			ctx.appendCode(indent, "\t%s[i] = %s\n", varName, valVar)
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// dynamic elements
		ctx.appendCode(indent, "startOffset := int(0)\n")
		ctx.appendCode(indent, "if len(buf) != 0 {\n")
		ctx.appendCode(indent, "\tif len(buf) < 4 {\n\t\treturn sszutils.ErrUnexpectedEOF\n\t}\n")
		ctx.appendCode(indent, "\tstartOffset = int(sszutils.UnmarshallUint32(buf[0:4]))\n")
		ctx.appendCode(indent, "}\n")
		ctx.appendCode(indent, "itemCount := startOffset / 4\n")
		ctx.appendCode(indent, "if startOffset%4 != 0 || len(buf) < startOffset {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "if len(%s) < itemCount {\n", varName)
			ctx.appendCode(indent, "\t%s = make(%s, itemCount)\n", varName, ctx.typePrinter.TypeString(desc.Type))
			ctx.appendCode(indent, "} else if len(%s) > itemCount {\n", varName)
			ctx.appendCode(indent, "\t%s = %s[:itemCount]\n", varName, varName)
			ctx.appendCode(indent, "}\n")
		}
		ctx.appendCode(indent, "for i := 0; i < itemCount; i++ {\n")
		ctx.appendCode(indent, "\tvar endOffset int\n")
		ctx.appendCode(indent, "\tif i < itemCount-1 {\n")
		ctx.appendCode(indent, "\t\tendOffset = int(sszutils.UnmarshallUint32(buf[(i+1)*4 : (i+2)*4]))\n")
		ctx.appendCode(indent, "\t} else {\n")
		ctx.appendCode(indent, "\t\tendOffset = len(buf)\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tif endOffset < startOffset || endOffset > len(buf) {\n")
		ctx.appendCode(indent, "\t\treturn sszutils.ErrOffset\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tbuf := buf[startOffset:endOffset]\n")
		ctx.appendCode(indent, "\tstartOffset = endOffset\n")
		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\t%s := %s[i]\n", valVar, varName)
		if desc.ElemDesc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.TypeString(desc.ElemDesc.Type.Elem()))
		}
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t%s[i] = %s\n", varName, valVar)
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

func (ctx *unmarshalContext) unmarshalUnion(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	// Read selector
	ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
	ctx.appendCode(indent, "selector := sszutils.UnmarshallUint8(buf[0:1])\n")
	ctx.appendCode(indent, "%s.Variant = selector\n", varName)
	ctx.appendCode(indent, "switch selector {\n")

	for variant, variantDesc := range desc.UnionVariants {
		variantType := ctx.typePrinter.TypeString(variantDesc.Type)
		ctx.appendCode(indent, "case %d:\n", variant)
		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\t%s := &%s{}\n", valVar, variantType)
		if err := ctx.unmarshalType(variantDesc, valVar, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t%s.Data = %s\n", varName, valVar)
	}

	ctx.appendCode(indent, "default:\n")
	ctx.appendCode(indent, "\treturn sszutils.ErrInvalidUnionVariant\n")
	ctx.appendCode(indent, "}\n")

	return nil
}
