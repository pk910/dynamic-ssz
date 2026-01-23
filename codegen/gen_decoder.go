// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// decoderContext contains the state and utilities for generating unmarshal methods.
//
// This context structure maintains the necessary state during the unmarshal code generation
// process, including code building utilities, variable management, and options that control
// the generation behavior. It manages both main unmarshaling logic and helper size
// calculation code.
//
// Fields:
//   - appendCode: Function to append main unmarshaling code with proper indentation
//   - appendSizeCode: Function to append size calculation helper code
//   - typePrinter: Type name formatter and import tracker
//   - options: Code generation options controlling output behavior
//   - usedDynSpecs: Flag tracking whether generated code uses dynamic spec expressions
//   - valVarCounter: Counter for generating unique value variable names
//   - sizeVarCounter: Counter for generating unique size variable names
//   - sizeVarMap: Map tracking size variables for type descriptors to avoid duplication
type decoderContext struct {
	appendCode         func(indent int, code string, args ...any)
	typePrinter        *TypePrinter
	options            *CodeGeneratorOptions
	exprVars           *exprVarGenerator
	staticSizeVars     *staticSizeVarGenerator
	usedDynSpecs       bool
	useSeekable        bool
	valVarCounter      int
	startPosVarCounter int

	offsetSliceCounter int
	offsetSliceLimit   int
}

// generateDecoder generates decoder methods for a specific type.
//
// This function creates the complete set of decoder methods for a type, including:
//   - UnmarshalSSZDecoder for dynamic specification support with runtime parsing
//
// Parameters:
//   - rootTypeDesc: Type descriptor containing complete SSZ decoder metadata
//   - codeBuilder: String builder to append generated method code to
//   - typePrinter: Type formatter for handling imports and type names
//   - viewName: Name of the view type for function name postfix (empty string for data type)
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateDecoder(rootTypeDesc *ssztypes.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, viewName string, options *CodeGeneratorOptions) error {
	codeBuf := strings.Builder{}
	ctx := &decoderContext{
		appendCode: func(indent int, code string, args ...any) {
			if len(args) > 0 {
				code = fmt.Sprintf(code, args...)
			}
			codeBuf.WriteString(indentStr(code, indent))
		},
		typePrinter: typePrinter,
		options:     options,
	}

	ctx.exprVars = newExprVarGenerator("expr", typePrinter, options)
	ctx.staticSizeVars = newStaticSizeVarGenerator("size", typePrinter, options, ctx.exprVars)

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate unmarshal code
	if err := ctx.unmarshalType(rootTypeDesc, "t", 0, true, false); err != nil {
		return err
	}

	if ctx.exprVars.varCounter > 0 {
		ctx.usedDynSpecs = true
	}

	fnName := "UnmarshalSSZDecoder"
	if viewName != "" {
		fnName = fmt.Sprintf("unmarshalSSZDecoderView_%s", viewName)
	}
	if ctx.usedDynSpecs {
		appendCode(codeBuilder, 0, "func (t %s) %s(ds sszutils.DynamicSpecs, dec sszutils.Decoder) (err error) {\n", typeName, fnName)
	} else {
		appendCode(codeBuilder, 0, "func (t %s) %s(_ sszutils.DynamicSpecs, dec sszutils.Decoder) (err error) {\n", typeName, fnName)
	}

	appendCode(codeBuilder, 1, ctx.exprVars.getCode())
	appendCode(codeBuilder, 1, ctx.staticSizeVars.getCode())
	if ctx.useSeekable {
		appendCode(codeBuilder, 1, "canSeek := dec.Seekable()\n")
	}
	if ctx.offsetSliceLimit > 0 {
		appendCode(codeBuilder, 1, "offsetSlices := [%d][]uint32{\n", ctx.offsetSliceLimit)
		for i := 0; i < ctx.offsetSliceLimit; i++ {
			appendCode(codeBuilder, 2, "sszutils.GetOffsetSlice(0),\n")
		}
		appendCode(codeBuilder, 1, "}\n")
		appendCode(codeBuilder, 1, "defer func() {\n")
		for i := 0; i < ctx.offsetSliceLimit; i++ {
			appendCode(codeBuilder, 2, "sszutils.PutOffsetSlice(offsetSlices[%d])\n", i)
		}
		appendCode(codeBuilder, 1, "}()\n")
	}
	appendCode(codeBuilder, 1, codeBuf.String())
	appendCode(codeBuilder, 1, "return nil\n")
	appendCode(codeBuilder, 0, "}\n\n")

	return nil
}

// getValVar generates a unique variable name for temporary values.
func (ctx *decoderContext) getValVar() string {
	ctx.valVarCounter++
	return fmt.Sprintf("val%d", ctx.valVarCounter)
}

// getCastedValueVar returns the variable name for the value of a type, converting to the source type if needed
func (ctx *decoderContext) getCastedValueVar(desc *ssztypes.TypeDescriptor, varName string, sourceType string) string {
	if targetType := ctx.typePrinter.InnerTypeString(desc); targetType != sourceType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// isInlinable determines if a type can be unmarshaled inline without temporary variables.
func (ctx *decoderContext) isInlinable(desc *ssztypes.TypeDescriptor) bool {
	// Inline primitive types
	if desc.SszType == ssztypes.SszBoolType || desc.SszType == ssztypes.SszUint8Type || desc.SszType == ssztypes.SszUint16Type || desc.SszType == ssztypes.SszUint32Type || desc.SszType == ssztypes.SszUint64Type {
		return true
	}

	// Inline byte arrays/slices
	if (desc.SszType == ssztypes.SszVectorType || desc.SszType == ssztypes.SszListType) && desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
		return true
	}

	// Inline types with fastssz unmarshaler
	hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszUnmarshaler := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
		useFastSsz = true
	}
	if useFastSsz {
		return true
	}

	// Inline types with generated unmarshal methods
	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicUnmarshaler != 0 {
		return true
	}

	// Inline types with generated decoder methods
	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicDecoder != 0 {
		return true
	}

	return false
}

// unmarshalType generates unmarshal code for any SSZ type, delegating to specific unmarshalers.
func (ctx *decoderContext) unmarshalType(desc *ssztypes.TypeDescriptor, varName string, indent int, isRoot bool, noBufCheck bool) error {
	// Handle types that have generated methods we can call
	isView := desc.GoTypeFlags&ssztypes.GoTypeFlagIsView != 0
	if !isRoot && isView {
		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewDecoder != 0 {
			ctx.appendCode(indent, "if viewFn := %s.UnmarshalSSZDecoderView((%s)(nil)); viewFn != nil {\n", varName, ctx.typePrinter.ViewTypeString(desc, true))
			ctx.appendCode(indent+1, "if err = viewFn(ds, dec); err != nil {\n\treturn err\n}\n")
			ctx.appendCode(indent, "} else {\n\treturn sszutils.ErrNotImplemented\n}\n")
			ctx.usedDynSpecs = true
			return nil
		}

		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewUnmarshaler != 0 {
			sizeStr := "dec.GetLength()"
			if desc.Size > 0 {
				sizeStr = fmt.Sprintf("%d", desc.Size)
			}
			ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(%s); err != nil {\n", sizeStr)
			ctx.appendCode(indent+1, "return err\n")
			ctx.appendCode(indent, "} else if viewFn := %s.UnmarshalSSZDynView((%s)(nil)); viewFn != nil {\n", varName, ctx.typePrinter.ViewTypeString(desc, true))
			ctx.appendCode(indent+1, "if err = viewFn(ds, buf); err != nil {\n\treturn err\n}\n")
			ctx.appendCode(indent, "} else {\n\treturn sszutils.ErrNotImplemented\n}\n")
			ctx.usedDynSpecs = true
			return nil
		}
	}

	if !isRoot && !isView {
		hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
		isFastsszUnmarshaler := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
		useFastSsz := !ctx.options.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
		if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
			useFastSsz = true
		}

		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicDecoder != 0 {
			ctx.appendCode(indent, "if err = %s.UnmarshalSSZDecoder(ds, dec); err != nil {\n\treturn err\n}\n", varName)
			ctx.usedDynSpecs = true
			return nil
		}

		if useFastSsz {
			sizeStr := "dec.GetLength()"
			if desc.Size > 0 {
				sizeStr = fmt.Sprintf("%d", desc.Size)
			}
			ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(%s); err != nil {\n", sizeStr)
			ctx.appendCode(indent+1, "return err\n")
			ctx.appendCode(indent, "} else if err = %s.UnmarshalSSZ(buf); err != nil {\n", varName)
			ctx.appendCode(indent+1, "return err\n")
			ctx.appendCode(indent, "}\n")
			return nil
		}

		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicUnmarshaler != 0 {
			sizeStr := "dec.GetLength()"
			if desc.Size > 0 {
				sizeStr = fmt.Sprintf("%d", desc.Size)
			}
			ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(%s); err != nil {\n", sizeStr)
			ctx.appendCode(indent+1, "return err\n")
			ctx.appendCode(indent, "} else if err = %s.UnmarshalSSZDyn(ds, buf); err != nil {\n", varName)
			ctx.appendCode(indent+1, "return err\n")
			ctx.appendCode(indent, "}\n")
			ctx.usedDynSpecs = true
			return nil
		}
	}

	switch desc.SszType {
	case ssztypes.SszBoolType:
		ctx.appendCode(indent, "if val, err := dec.DecodeBool(); err != nil {\n")
		ctx.appendCode(indent+1, "return err\n")
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "bool"))
		ctx.appendCode(indent, "}\n")
	case ssztypes.SszUint8Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint8(); err != nil {\n")
		ctx.appendCode(indent+1, "return err\n")
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "uint8"))
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszUint16Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint16(); err != nil {\n")
		ctx.appendCode(indent+1, "return err\n")
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "uint16"))
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszUint32Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint32(); err != nil {\n")
		ctx.appendCode(indent+1, "return err\n")
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s(val)\n", ptrVarName, ctx.typePrinter.InnerTypeString(desc))
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszUint64Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint64(); err != nil {\n")
		ctx.appendCode(indent+1, "return err\n")
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "time.Unix(int64(val), 0).UTC()", "time.Time"))
			ctx.typePrinter.AddImport("time", "time")
		} else {
			ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "uint64"))
		}
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszTypeWrapperType:
		if err := ctx.unmarshalType(desc.ElemDesc, fmt.Sprintf("%s.Data", varName), indent, false, noBufCheck); err != nil {
			return err
		}

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return ctx.unmarshalContainer(desc, varName, indent)

	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return ctx.unmarshalVector(desc, varName, indent, noBufCheck)

	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		return ctx.unmarshalList(desc, varName, indent)

	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		return ctx.unmarshalBitlist(desc, varName, indent)

	case ssztypes.SszCompatibleUnionType:
		return ctx.unmarshalUnion(desc, varName, indent)

	case ssztypes.SszCustomType:
		ctx.appendCode(indent, "return sszutils.ErrNotImplemented\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// unmarshalContainer generates unmarshal code for SSZ container (struct) types.
func (ctx *decoderContext) unmarshalContainer(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	staticSize := 0
	staticSizeVars := []string{}
	hasDynamicFields := false
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			staticSize += 4
			hasDynamicFields = true
		} else {
			if field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
				sizeVar, err := ctx.staticSizeVars.getStaticSizeVar(field.Type)
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

	// Read fixed fields and offsets
	ctx.appendCode(indent, "maxOffset := uint32(dec.GetLength())\n")

	startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
	if hasDynamicFields {
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)
		ctx.startPosVarCounter++
	}
	ctx.appendCode(indent, "if maxOffset < uint32(%s) {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", strings.Join(staticSizeVars, "+"))
	dynamicFields := make([]int, 0)

	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			// Read offset
			ctx.appendCode(indent, "// Field #%d '%s' (offset)\n", idx, field.Name)
			ctx.appendCode(indent, "offset%d, err := dec.DecodeOffset()\n", idx)
			ctx.appendCode(indent, "if err != nil {\n\treturn err\n}\n")
			if len(dynamicFields) > 0 {
				ctx.appendCode(indent, "if offset%d < offset%d || offset%d > maxOffset {\n\treturn sszutils.ErrOffset\n}\n", idx, dynamicFields[len(dynamicFields)-1], idx)
			} else {
				ctx.appendCode(indent, "if offset%d != uint32(%s) {\n\treturn sszutils.ErrOffset\n}\n", idx, strings.Join(staticSizeVars, "+"))
			}
			dynamicFields = append(dynamicFields, idx)
		} else {
			// Unmarshal fixed field
			valVar := fmt.Sprintf("%s.%s", varName, field.Name)
			inlineIndent := 0
			isInlinable := ctx.isInlinable(field.Type)
			if !isInlinable {
				valVar = ctx.getValVar()
				ctx.appendCode(indent, "{ // Field #%d '%s' (static)\n", idx, field.Name)
				ctx.appendCode(indent+1, "%s := %s.%s\n", valVar, varName, field.Name)
				inlineIndent = 1
			} else {
				ctx.appendCode(indent, "// Field #%d '%s' (static)\n", idx, field.Name)
			}
			if field.Type.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent+inlineIndent, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(field.Type))
			}

			if err := ctx.unmarshalType(field.Type, valVar, indent+inlineIndent, false, true); err != nil {
				return err
			}

			if !isInlinable {
				ctx.appendCode(indent, "\t%s.%s = %s\n", varName, field.Name, valVar)
				ctx.appendCode(indent, "}\n")
			}
		}
	}

	// Read dynamic fields
	for idx, fieldIdx := range dynamicFields {
		field := desc.ContainerDesc.Fields[fieldIdx]
		ctx.appendCode(indent, "{ // Field #%d '%s' (dynamic)\n", fieldIdx, field.Name)
		ctx.appendCode(indent+1, "if dec.GetPosition() != %s+int(offset%d) {\n\treturn sszutils.ErrOffset\n}\n", startPosVar, fieldIdx)

		endOffset := ""
		if idx < len(dynamicFields)-1 {
			endOffset = fmt.Sprintf("offset%d", dynamicFields[idx+1])
		} else {
			endOffset = "maxOffset"
		}

		ctx.appendCode(indent+1, "dec.PushLimit(int(%s - offset%d))\n", endOffset, fieldIdx)

		valVar := ctx.getValVar()
		ctx.appendCode(indent+1, "%s := %s.%s\n", valVar, varName, field.Name)

		if field.Type.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(field.Type))
		}

		if err := ctx.unmarshalType(field.Type, valVar, indent+1, false, true); err != nil {
			return err
		}

		ctx.appendCode(indent+1, "if diff := dec.PopLimit(); diff != 0 {\n\treturn sszutils.ErrOffset\n}\n")
		ctx.appendCode(indent+1, "%s.%s = %s\n", varName, field.Name, valVar)
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// unmarshalVector generates unmarshal code for SSZ vector (fixed-size array) types.
func (ctx *decoderContext) unmarshalVector(desc *ssztypes.TypeDescriptor, varName string, indent int, noBufCheck bool) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	bitlimitVar := ""

	if sizeExpression != nil {
		defaultValue := uint64(desc.Len)
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 && desc.BitSize > 0 {
			defaultValue = uint64(desc.BitSize)
		}

		exprVar := ctx.exprVars.getExprVar(*sizeExpression, defaultValue)

		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 {
			ctx.appendCode(indent, "bitlimit := %s\n", exprVar)
			ctx.appendCode(indent, "limit := (bitlimit+7)/8\n")
			bitlimitVar = "int(bitlimit)"
			limitVar = "int(limit)"
		} else {
			limitVar = fmt.Sprintf("int(%s)", exprVar)
		}

		if desc.Kind == reflect.Array {
			// check if dynamic limit is greater than the length of the array
			ctx.appendCode(indent, "if %s > %d {\n", limitVar, desc.Len)
			ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
			ctx.appendCode(indent, "}\n")
		}
	} else {
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 && desc.BitSize > 0 && desc.BitSize%8 != 0 {
			bitlimitVar = fmt.Sprintf("%d", desc.BitSize)
		}
		limitVar = fmt.Sprintf("%d", desc.Len)
	}

	// create slice if needed
	if desc.Kind != reflect.Array && desc.GoTypeFlags&ssztypes.GoTypeFlagIsString == 0 {
		ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, %s)\n", varName, varName, limitVar)
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
			if !noBufCheck {
				ctx.appendCode(indent, "if %s > dec.GetLength() {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", limitVar)
			}
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(%s); err != nil {\n", limitVar)
				ctx.appendCode(indent+1, "return err\n")
				ctx.appendCode(indent, "} else {\n")
				ptrVarName := varName
				if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
					ptrVarName = fmt.Sprintf("*(%s)", varName)
				}
				ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "buf", ""))
				ctx.appendCode(indent, "}\n")
			} else {
				ctx.appendCode(indent, "if _, err = dec.DecodeBytes(%s[:%s]); err != nil {\n\treturn err\n}\n", varName, limitVar)
				if bitlimitVar != "" {
					ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
					ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", varName, limitVar)
					ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
					ctx.appendCode(indent, "}\n")
				}
			}
			return nil
		}

		// static elements
		var fieldSizeVar string
		var err error
		if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
			fieldSizeVar, err = ctx.staticSizeVars.getStaticSizeVar(desc.ElemDesc)
			if err != nil {
				return err
			}
		} else {
			fieldSizeVar = fmt.Sprintf("%d", desc.ElemDesc.Size)
		}

		if !noBufCheck {
			ctx.appendCode(indent, "if %s*%s > dec.GetLength() {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", limitVar, fieldSizeVar)
		}

		startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
		ctx.startPosVarCounter++
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)

		ctx.appendCode(indent, "for i := range %s {\n", limitVar)

		valVar := fmt.Sprintf("%s[i]", varName)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent, "\t%s := %s[i]\n", valVar, varName)
		}
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}

		ctx.appendCode(indent+1, "if dec.GetPosition() != %s+int(%s*(i+1)) {\n\treturn sszutils.ErrOffset\n}\n", startPosVar, fieldSizeVar)

		if !isInlinable {
			ctx.appendCode(indent, "\t%s[i] = %s\n", varName, valVar)
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// dynamic elements
		ctx.appendCode(indent, "sszLen := dec.GetLength()\n")
		ctx.appendCode(indent, "if %s*4 > sszLen {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", limitVar)
		startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
		ctx.startPosVarCounter++
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)

		// check first offset
		ctx.appendCode(indent, "startOffset, err := dec.DecodeOffset()\n")
		ctx.appendCode(indent, "if err != nil {\n\treturn err\n}\n")
		ctx.appendCode(indent, "if startOffset != %s*4 {\n\treturn sszutils.ErrOffset\n}\n", limitVar)

		// read offsets
		ctx.appendCode(indent, "var offsets []uint32\n")
		ctx.appendCode(indent, "if canSeek {\n")
		ctx.appendCode(indent+1, "dec.SkipBytes((%s - 1) * 4)\n", limitVar)
		ctx.appendCode(indent, "} else if %s > 1 {\n", limitVar)
		ctx.appendCode(indent+1, "offsetSlices[%d] = sszutils.ExpandSlice(offsetSlices[%d], %s-1)\n", ctx.offsetSliceCounter, ctx.offsetSliceCounter, limitVar)
		ctx.appendCode(indent+1, "offsets = offsetSlices[%d]\n", ctx.offsetSliceCounter)
		ctx.appendCode(indent+1, "for i := range %s-1 {\n", limitVar)
		ctx.appendCode(indent+2, "offset, err := dec.DecodeOffset()\n")
		ctx.appendCode(indent+2, "if err != nil {\n")
		ctx.appendCode(indent+3, "return err\n")
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+2, "offsets[i] = offset\n")
		ctx.appendCode(indent+1, "}\n")
		ctx.appendCode(indent, "}\n")
		ctx.useSeekable = true
		ctx.offsetSliceCounter++
		if ctx.offsetSliceCounter > ctx.offsetSliceLimit {
			ctx.offsetSliceLimit = ctx.offsetSliceCounter
		}

		ctx.appendCode(indent, "for i := range %s {\n", limitVar)

		ctx.appendCode(indent+1, "var endOffset uint32\n")
		ctx.appendCode(indent+1, "if i < %s-1 {\n", limitVar)
		ctx.appendCode(indent+2, "if canSeek {\n")
		ctx.appendCode(indent+3, "endOffset = dec.DecodeOffsetAt(%s + int((i+1)*4))\n", startPosVar)
		ctx.appendCode(indent+2, "} else {\n")
		ctx.appendCode(indent+3, "endOffset = offsets[i]\n")
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+1, "} else {\n")
		ctx.appendCode(indent+2, "endOffset = uint32(sszLen)\n")
		ctx.appendCode(indent+1, "}\n")

		ctx.appendCode(indent+1, "if endOffset < startOffset || endOffset > uint32(sszLen) {\n")
		ctx.appendCode(indent+2, "return sszutils.ErrOffset\n")
		ctx.appendCode(indent+1, "}\n")

		ctx.appendCode(indent+1, "itemSize := endOffset - startOffset\n")
		ctx.appendCode(indent+1, "dec.PushLimit(int(itemSize))\n")
		ctx.appendCode(indent+1, "startOffset = endOffset\n")

		valVar := ctx.getValVar()
		ctx.appendCode(indent+1, "%s := %s[i]\n", valVar, varName)
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}

		ctx.appendCode(indent+1, "if diff := dec.PopLimit(); diff != 0 {\n\treturn sszutils.ErrOffset\n}\n")

		ctx.appendCode(indent+1, "%s[i] = %s\n", varName, valVar)
		ctx.appendCode(indent, "}\n")

		ctx.offsetSliceCounter--
	}

	return nil
}

// unmarshalList generates unmarshal code for SSZ list (variable-size array) types.
func (ctx *decoderContext) unmarshalList(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(dec.GetLength()); err != nil {\n")
				ctx.appendCode(indent+1, "return err\n")
				ctx.appendCode(indent, "} else {\n")
				ptrVarName := varName
				if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
					ptrVarName = fmt.Sprintf("*(%s)", varName)
				}
				ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "buf", ""))
				ctx.appendCode(indent, "}\n")
			} else {
				ctx.appendCode(indent, "listLen := dec.GetLength()\n")
				if desc.Kind != reflect.Array {
					ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, listLen)\n", varName, varName)
				}
				ctx.appendCode(indent, "if _, err = dec.DecodeBytes(%s[:listLen]); err != nil {\n\treturn err\n}\n", varName)
			}
			return nil
		}

		// static elements
		var fieldSizeVar string
		var err error
		if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
			fieldSizeVar, err = ctx.staticSizeVars.getStaticSizeVar(desc.ElemDesc)
			if err != nil {
				return err
			}
		} else {
			fieldSizeVar = fmt.Sprintf("%d", desc.ElemDesc.Size)
		}

		if fieldSizeVar == "1" {
			ctx.appendCode(indent, "itemCount := dec.GetLength()\n")
		} else {
			ctx.appendCode(indent, "sszLen := dec.GetLength()\n")
			ctx.appendCode(indent, "itemCount := sszLen / %s\n", fieldSizeVar)
			ctx.appendCode(indent, "if sszLen%%%s != 0 {\n\treturn sszutils.ErrUnexpectedEOF\n}\n", fieldSizeVar)
		}
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", varName, varName)
		}

		startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
		ctx.startPosVarCounter++
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)

		ctx.appendCode(indent, "for i := range itemCount {\n")

		valVar := fmt.Sprintf("%s[i]", varName)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent+1, "%s := %s[i]\n", valVar, varName)
		}
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}

		ctx.appendCode(indent+1, "if dec.GetPosition() != %s+int(%s*(i+1)) {\n\treturn sszutils.ErrOffset\n}\n", startPosVar, fieldSizeVar)

		if !isInlinable {
			ctx.appendCode(indent+1, "%s[i] = %s\n", varName, valVar)
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// dynamic elements

		// check first offset
		ctx.appendCode(indent, "startOffset := uint32(0)\n")
		startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
		ctx.startPosVarCounter++
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)
		ctx.appendCode(indent, "sszLen := uint32(dec.GetLength())\n")
		ctx.appendCode(indent, "if sszLen > 0 {\n")
		ctx.appendCode(indent+1, "startOffset, err = dec.DecodeOffset()\n")
		ctx.appendCode(indent+1, "if err != nil {\n\treturn err\n}\n")
		ctx.appendCode(indent, "}\n")
		ctx.appendCode(indent, "itemCount := int(startOffset / 4)\n")

		// read offsets
		ctx.appendCode(indent, "var offsets []uint32\n")
		ctx.appendCode(indent, "if canSeek {\n")
		ctx.appendCode(indent+1, "dec.SkipBytes((itemCount - 1) * 4)\n")
		ctx.appendCode(indent, "} else if itemCount > 1 {\n")
		ctx.appendCode(indent+1, "offsetSlices[%d] = sszutils.ExpandSlice(offsetSlices[%d], itemCount-1)\n", ctx.offsetSliceCounter, ctx.offsetSliceCounter)
		ctx.appendCode(indent+1, "offsets = offsetSlices[%d]\n", ctx.offsetSliceCounter)
		ctx.appendCode(indent+1, "for i := range itemCount-1 {\n")
		ctx.appendCode(indent+2, "offset, err := dec.DecodeOffset()\n")
		ctx.appendCode(indent+2, "if err != nil {\n")
		ctx.appendCode(indent+3, "return err\n")
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+2, "offsets[i] = offset\n")
		ctx.appendCode(indent+1, "}\n")
		ctx.appendCode(indent, "}\n")
		ctx.useSeekable = true
		ctx.offsetSliceCounter++
		if ctx.offsetSliceCounter > ctx.offsetSliceLimit {
			ctx.offsetSliceLimit = ctx.offsetSliceCounter
		}

		ctx.appendCode(indent, "if startOffset%4 != 0 || uint32(sszLen) < startOffset {\n\treturn sszutils.ErrUnexpectedEOF\n}\n")
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", varName, varName)
		}
		ctx.appendCode(indent, "for i := range itemCount {\n")

		ctx.appendCode(indent+1, "var endOffset uint32\n")
		ctx.appendCode(indent+1, "if i < itemCount-1 {\n")
		ctx.appendCode(indent+2, "if canSeek {\n")
		ctx.appendCode(indent+3, "endOffset = dec.DecodeOffsetAt(%s + int((i+1)*4))\n", startPosVar)
		ctx.appendCode(indent+2, "} else {\n")
		ctx.appendCode(indent+3, "endOffset = offsets[i]\n")
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+1, "} else {\n")
		ctx.appendCode(indent+2, "endOffset = uint32(sszLen)\n")
		ctx.appendCode(indent+1, "}\n")
		ctx.appendCode(indent+1, "if endOffset < startOffset || endOffset > uint32(sszLen) {\n")
		ctx.appendCode(indent+2, "return sszutils.ErrOffset\n")
		ctx.appendCode(indent+1, "}\n")

		ctx.appendCode(indent+1, "itemSize := endOffset - startOffset\n")
		ctx.appendCode(indent+1, "dec.PushLimit(int(itemSize))\n")
		ctx.appendCode(indent+1, "startOffset = endOffset\n")

		valVar := ctx.getValVar()
		ctx.appendCode(indent+1, "%s := %s[i]\n", valVar, varName)
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent+1, "if diff := dec.PopLimit(); diff != 0 {\n\treturn sszutils.ErrOffset\n}\n")
		ctx.appendCode(indent+1, "%s[i] = %s\n", varName, valVar)
		ctx.appendCode(indent, "}\n")

		ctx.offsetSliceCounter--
	}

	return nil
}

// unmarshalBitlist generates unmarshal code for SSZ bitlist types.
func (ctx *decoderContext) unmarshalBitlist(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	ctx.appendCode(indent, "blen := dec.GetLength()\n")

	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, blen)\n", varName, varName)
	}
	ctx.appendCode(indent, "if _, err = dec.DecodeBytes(%s[:blen]); err != nil {\n\treturn err\n}\n", varName)
	ctx.appendCode(indent, "if blen == 0 || %s[blen-1] == 0x00 {\n", varName)
	ctx.appendCode(indent, "\treturn sszutils.ErrBitlistNotTerminated\n")
	ctx.appendCode(indent, "}\n")

	return nil
}

// unmarshalUnion generates unmarshal code for SSZ union types.
func (ctx *decoderContext) unmarshalUnion(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	// Read selector
	ctx.appendCode(indent, "selector, err := dec.DecodeUint8()\n")
	ctx.appendCode(indent, "if err != nil {\n\treturn err\n}\n")
	ctx.appendCode(indent, "%s.Variant = selector\n", varName)
	ctx.appendCode(indent, "switch selector {\n")

	variants := make([]int, 0, len(desc.UnionVariants))
	for variant := range desc.UnionVariants {
		variants = append(variants, int(variant))
	}
	slices.Sort(variants)

	for _, variant := range variants {
		variantDesc := desc.UnionVariants[uint8(variant)]
		variantType := ctx.typePrinter.TypeString(variantDesc)
		ctx.appendCode(indent, "case %d:\n", variant)
		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\tvar %s %s\n", valVar, variantType)
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
