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
	indexCounter       int
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
	// Streaming code always uses dynamic expressions since the decoder interface
	// requires DynamicSpecs. Override WithoutDynamicExpressions for this generator.
	if options.WithoutDynamicExpressions {
		optsCopy := *options
		optsCopy.WithoutDynamicExpressions = false
		options = &optsCopy
	}

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
	if err := ctx.unmarshalType(rootTypeDesc, "t", typePathList{}, 0, true, false); err != nil {
		return err
	}

	if ctx.exprVars.varCounter > 0 {
		ctx.usedDynSpecs = true
	}

	fnName := "UnmarshalSSZDecoder"
	if viewName != "" {
		fnName = fmt.Sprintf("unmarshalSSZDecoderView_%s", viewName)
	}
	if viewName == "" {
		appendCode(codeBuilder, 0, "// UnmarshalSSZDecoder unmarshals the %s from the given SSZ decoder using dynamic specifications.\n", typeName)
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

// getValueVar returns the variable name for the value of a type, dereferencing pointer types and converting to the target type if needed
func (ctx *decoderContext) getValueVar(desc *ssztypes.TypeDescriptor, varName, targetType string) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
		varName = fmt.Sprintf("*%s", varName)
	}

	if targetType != "" && ctx.typePrinter.InnerTypeString(desc) != targetType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// getValVar generates a unique variable name for temporary values.
func (ctx *decoderContext) getValVar() string {
	ctx.valVarCounter++
	return fmt.Sprintf("val%d", ctx.valVarCounter)
}

// getCastedValueVar returns the variable name for the value of a type, converting to the source type if needed
func (ctx *decoderContext) getCastedValueVar(desc *ssztypes.TypeDescriptor, varName, sourceType string) string {
	if targetType := ctx.typePrinter.InnerTypeString(desc); targetType != sourceType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// getIndexVar returns a unique index variable name
func (ctx *decoderContext) getIndexVar() (string, func()) {
	ctx.indexCounter++
	thisIndex := ctx.indexCounter
	return fmt.Sprintf("idx%d", thisIndex), func() {
		ctx.indexCounter = thisIndex - 1
	}
}

// isInlinable determines if a type can be unmarshaled inline without temporary variables.
func (ctx *decoderContext) isInlinable(desc *ssztypes.TypeDescriptor) bool {
	// Inline primitive types
	if desc.SszType == ssztypes.SszBoolType || desc.SszType == ssztypes.SszUint8Type || desc.SszType == ssztypes.SszUint16Type || desc.SszType == ssztypes.SszUint32Type || desc.SszType == ssztypes.SszUint64Type || desc.SszType == ssztypes.SszInt8Type || desc.SszType == ssztypes.SszInt16Type || desc.SszType == ssztypes.SszInt32Type || desc.SszType == ssztypes.SszInt64Type || desc.SszType == ssztypes.SszFloat32Type || desc.SszType == ssztypes.SszFloat64Type {
		return true
	}

	// Inline byte arrays/slices
	if (desc.SszType == ssztypes.SszVectorType || desc.SszType == ssztypes.SszListType) && desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
		return true
	}

	// Inline types with fastssz unmarshaler
	hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0
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
func (ctx *decoderContext) unmarshalType(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int, isRoot, noBufCheck bool) error {
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
			sizeStr := "dec.GetLength()" //nolint:goconst // generated code template string
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

	hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0
	isFastsszUnmarshaler := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
		useFastSsz = true
	}

	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicDecoder != 0 && !isRoot && !isView {
		ctx.appendCode(indent, "if err = %s.UnmarshalSSZDecoder(ds, dec); err != nil {\n\treturn %s\n}\n", varName, typePath.getErrorWith("err"))
		ctx.usedDynSpecs = true
		return nil
	}

	if useFastSsz && !isRoot && !isView {
		sizeStr := "dec.GetLength()"
		if desc.Size > 0 {
			sizeStr = fmt.Sprintf("%d", desc.Size)
		}
		ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(%s); err != nil {\n", sizeStr)
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else if err = %s.UnmarshalSSZ(buf); err != nil {\n", varName)
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "}\n")
		return nil
	}

	if !isRoot && !isView {
		hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0
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
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "bool"))
		ctx.appendCode(indent, "}\n")
	case ssztypes.SszUint8Type, ssztypes.SszInt8Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint8(); err != nil {\n")
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "uint8"))
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszUint16Type, ssztypes.SszInt16Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint16(); err != nil {\n")
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "uint16"))
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszUint32Type, ssztypes.SszInt32Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint32(); err != nil {\n")
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent+1, "%s = %s(val)\n", ptrVarName, ctx.typePrinter.InnerTypeString(desc))
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszUint64Type, ssztypes.SszInt64Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint64(); err != nil {\n")
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			timeImport := ctx.typePrinter.AddImport("time", "time")
			ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, fmt.Sprintf("%s.Unix(int64(val), 0).UTC()", timeImport), fmt.Sprintf("%s.Time", timeImport)))
		} else {
			ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, "val", "uint64"))
		}
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszTypeWrapperType:
		fieldName := getTypeWrapperFieldName(desc)
		if fieldName == "" {
			return fmt.Errorf("could not determine data field name for wrapper descriptor")
		}
		if err := ctx.unmarshalType(desc.ElemDesc, fmt.Sprintf("%s.%s", varName, fieldName), typePath, indent, false, noBufCheck); err != nil {
			return err
		}

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return ctx.unmarshalContainer(desc, varName, typePath, indent)

	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return ctx.unmarshalVector(desc, varName, typePath, indent, noBufCheck)

	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		return ctx.unmarshalList(desc, varName, typePath, indent)

	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		return ctx.unmarshalBitlist(desc, varName, typePath, indent)

	case ssztypes.SszCompatibleUnionType:
		return ctx.unmarshalUnion(desc, varName, typePath, indent)

	case ssztypes.SszCustomType:
		ctx.appendCode(indent, "return %s\n", typePath.getErrorWith(errCodeCustomTypeNotSupported))

	// extended types
	case ssztypes.SszFloat32Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint32(); err != nil {\n")
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		mathImport := ctx.typePrinter.AddImport("math", "math")
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, fmt.Sprintf("%s.Float32frombits(val)", mathImport), "float32"))
		ctx.appendCode(indent, "}\n")
	case ssztypes.SszFloat64Type:
		ctx.appendCode(indent, "if val, err := dec.DecodeUint64(); err != nil {\n")
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "} else {\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		mathImport := ctx.typePrinter.AddImport("math", "math")
		ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, fmt.Sprintf("%s.Float64frombits(val)", mathImport), "float64"))
		ctx.appendCode(indent, "}\n")
	case ssztypes.SszOptionalType:
		return ctx.unmarshalOptional(desc, varName, typePath, indent)
	case ssztypes.SszBigIntType:
		return ctx.unmarshalBigInt(desc, varName, typePath, indent)

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// unmarshalContainer generates unmarshal code for SSZ container (struct) types.
func (ctx *decoderContext) unmarshalContainer(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	staticSize := 0
	staticSizeVars := []string{}
	hasDynamicFields := false
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			staticSize += 4
			hasDynamicFields = true
		} else {
			if field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 {
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

	totalStaticSizeExpr := strings.Join(staticSizeVars, "+")
	if len(staticSizeVars) > 1 {
		ctx.appendCode(indent, "totalSize := %s\n", totalStaticSizeExpr)
		totalStaticSizeExpr = "totalSize"
	}

	// Read fixed fields and offsets
	ctx.appendCode(indent, "maxOffset := uint32(dec.GetLength())\n")

	startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
	if hasDynamicFields {
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)
		ctx.startPosVarCounter++
	}
	errCode := fmt.Sprintf("sszutils.ErrFixedFieldsEOFFn(maxOffset, uint32(%s))", totalStaticSizeExpr)
	ctx.appendCode(indent, "if maxOffset < uint32(%s) {\n\treturn %s\n}\n", totalStaticSizeExpr, typePath.getErrorWith(errCode))
	dynamicFields := make([]int, 0)

	for idx, field := range desc.ContainerDesc.Fields {
		fieldPath := typePath.append(field.Name)
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			// Read offset
			ctx.appendCode(indent, "// Field #%d '%s' (offset)\n", idx, field.Name)
			ctx.appendCode(indent, "offset%d, err := dec.DecodeOffset()\n", idx)
			ctx.appendCode(indent, "if err != nil {\n\treturn %s\n}\n", fieldPath.getErrorWith("err"))
			if len(dynamicFields) > 0 {
				errCode = fmt.Sprintf("sszutils.ErrOffsetOutOfRangeFn(offset%d, offset%d, maxOffset)", idx, dynamicFields[len(dynamicFields)-1])
				ctx.appendCode(indent, "if offset%d < offset%d || offset%d > maxOffset {\n\treturn %s\n}\n", idx, dynamicFields[len(dynamicFields)-1], idx, fieldPath.getErrorWith(errCode))
			} else {
				errCode = fmt.Sprintf("sszutils.ErrFirstOffsetMismatchFn(offset%d, %s)", idx, totalStaticSizeExpr)
				ctx.appendCode(indent, "if offset%d != uint32(%s) {\n\treturn %s\n}\n", idx, totalStaticSizeExpr, fieldPath.getErrorWith(errCode))
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

			if err := ctx.unmarshalType(field.Type, valVar, fieldPath, indent+inlineIndent, false, true); err != nil {
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
		fieldPath := typePath.append(field.Name)
		ctx.appendCode(indent, "{ // Field #%d '%s' (dynamic)\n", fieldIdx, field.Name)
		errCode := fmt.Sprintf("sszutils.ErrFieldNotConsumedFn(dec.GetPosition(), %s+int(offset%d))", startPosVar, fieldIdx)
		ctx.appendCode(indent+1, "if dec.GetPosition() != %s+int(offset%d) {\n\treturn %s\n}\n", startPosVar, fieldIdx, fieldPath.getErrorWith(errCode))

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

		if err := ctx.unmarshalType(field.Type, valVar, fieldPath, indent+1, false, true); err != nil {
			return err
		}

		errCode = errCodeTrailingData
		ctx.appendCode(indent+1, "if diff := dec.PopLimit(); diff != 0 {\n\treturn %s\n}\n", fieldPath.getErrorWith(errCode))
		ctx.appendCode(indent+1, "%s.%s = %s\n", varName, field.Name, valVar)
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// unmarshalVector generates unmarshal code for SSZ vector (fixed-size array) types.
func (ctx *decoderContext) unmarshalVector(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int, noBufCheck bool) error {
	sizeExpression := desc.SizeExpression

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
			errCode := fmt.Sprintf("sszutils.ErrVectorSizeExceedsArrayFn(%s, %d)", limitVar, desc.Len)
			ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
			ctx.appendCode(indent, "}\n")
		}
	} else {
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 && desc.BitSize > 0 && desc.BitSize%8 != 0 {
			bitlimitVar = fmt.Sprintf("%d", desc.BitSize)
		}
		limitVar = fmt.Sprintf("%d", desc.Len)
	}

	valueVar := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		targetType := ""
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
			targetType = typeNameString
		}
		valueVar = ctx.getValueVar(desc, varName, targetType)
	}

	indexValueVar := valueVar
	if strings.HasPrefix(valueVar, "*") {
		indexValueVar = fmt.Sprintf("(%s)", valueVar)
	}

	// create slice if needed
	if desc.Kind != reflect.Array && desc.GoTypeFlags&ssztypes.GoTypeFlagIsString == 0 {
		ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, %s)\n", valueVar, valueVar, limitVar)
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
			if !noBufCheck {
				errCode := fmt.Sprintf("sszutils.ErrByteVectorEOFFn(dec.GetLength(), %s)", limitVar)
				ctx.appendCode(indent, "if %s > dec.GetLength() {\n\treturn %s\n}\n", limitVar, typePath.getErrorWith(errCode))
			}
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(%s); err != nil {\n", limitVar)
				ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
				ctx.appendCode(indent, "} else {\n")
				ctx.appendCode(indent+1, "%s = %s\n", valueVar, ctx.getCastedValueVar(desc, "buf", ""))
				ctx.appendCode(indent, "}\n")
			} else {
				ctx.appendCode(indent, "if _, err = dec.DecodeBytes(%s[:%s]); err != nil {\n\treturn err\n}\n", indexValueVar, limitVar)
				if bitlimitVar != "" {
					ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
					ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", indexValueVar, limitVar)
					errCode := errCodeBitvectorPadding
					ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
					ctx.appendCode(indent, "}\n")
				}
			}
			return nil
		}

		// static elements
		var fieldSizeVar string
		var err error
		if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 {
			fieldSizeVar, err = ctx.staticSizeVars.getStaticSizeVar(desc.ElemDesc)
			if err != nil {
				return err
			}
		} else {
			fieldSizeVar = fmt.Sprintf("%d", desc.ElemDesc.Size)
		}

		if !noBufCheck {
			errCode := fmt.Sprintf("sszutils.ErrVectorElementsEOFFn(dec.GetLength(), int(%s)*%s)", limitVar, fieldSizeVar)
			ctx.appendCode(indent, "if %s*%s > dec.GetLength() {\n\treturn %s\n}\n", limitVar, fieldSizeVar, typePath.getErrorWith(errCode))
		}

		// bulk uint64 lists
		if desc.ElemDesc.SszType == ssztypes.SszUint64Type && desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
			ctx.appendCode(indent, "if err = sszutils.DecodeUint64Slice(dec, %s[:%s]); err != nil {\n\treturn %s\n}\n", indexValueVar, limitVar, typePath.getErrorWith("err"))
			return nil
		}

		startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
		ctx.startPosVarCounter++
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "for %s := range %s {\n", indexVar, limitVar)

		valVar := fmt.Sprintf("%s[%s]", indexValueVar, indexVar)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent, "\t%s := %s[%s]\n", valVar, indexValueVar, indexVar)
		}
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		fieldPath := typePath.append("[%d]", indexVar)
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, fieldPath, indent+1, false, true); err != nil {
			return err
		}

		ctx.appendCode(indent+1, "if dec.GetPosition() != %s+int(%s*(%s+1)) {\n", startPosVar, fieldSizeVar, indexVar)
		errCode := fmt.Sprintf("sszutils.ErrStaticElementNotConsumedFn(dec.GetPosition(), %s+int(%s*(%s+1)))", startPosVar, fieldSizeVar, indexVar)
		ctx.appendCode(indent+2, "return %s\n", fieldPath.getErrorWith(errCode))
		ctx.appendCode(indent+1, "}\n")

		if !isInlinable {
			ctx.appendCode(indent, "\t%s[%s] = %s\n", indexValueVar, indexVar, valVar)
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// dynamic elements
		ctx.appendCode(indent, "sszLen := dec.GetLength()\n")
		errCode := fmt.Sprintf("sszutils.ErrVectorOffsetsEOFFn(dec.GetLength(), int(%s)*4)", limitVar)
		ctx.appendCode(indent, "if %s*4 > sszLen {\n\treturn %s\n}\n", limitVar, typePath.getErrorWith(errCode))
		startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
		ctx.startPosVarCounter++
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)

		// check first offset
		ctx.appendCode(indent, "startOffset, err := dec.DecodeOffset()\n")
		ctx.appendCode(indent, "if err != nil {\n\treturn %s\n}\n", typePath.getErrorWith("err"))
		errCode = fmt.Sprintf("sszutils.ErrFirstOffsetMismatchFn(startOffset, %s*4)", limitVar)
		ctx.appendCode(indent, "if startOffset != %s*4 {\n\treturn %s\n}\n", limitVar, typePath.getErrorWith(errCode))

		// read offsets
		ctx.appendCode(indent, "var offsets []uint32\n")
		ctx.appendCode(indent, "if canSeek {\n")
		ctx.appendCode(indent+1, "dec.SkipBytes((%s - 1) * 4)\n", limitVar)
		ctx.appendCode(indent, "} else if %s > 1 {\n", limitVar)
		ctx.appendCode(indent+1, "offsetSlices[%d] = sszutils.ExpandSlice(offsetSlices[%d], %s-1)\n", ctx.offsetSliceCounter, ctx.offsetSliceCounter, limitVar)
		ctx.appendCode(indent+1, "offsets = offsetSlices[%d]\n", ctx.offsetSliceCounter)

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent+1, "for %s := range %s-1 {\n", indexVar, limitVar)
		ctx.appendCode(indent+2, "offset, err := dec.DecodeOffset()\n")
		ctx.appendCode(indent+2, "if err != nil {\n")
		ctx.appendCode(indent+3, "return %s\n", typePath.append("[%d:o]", indexVar).getErrorWith("err"))
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+2, "offsets[%s] = offset\n", indexVar)
		ctx.appendCode(indent+1, "}\n")
		ctx.appendCode(indent, "}\n")
		ctx.useSeekable = true
		ctx.offsetSliceCounter++
		if ctx.offsetSliceCounter > ctx.offsetSliceLimit {
			ctx.offsetSliceLimit = ctx.offsetSliceCounter
		}

		ctx.appendCode(indent, "for %s := range %s {\n", indexVar, limitVar)

		fieldPath := typePath.append("[%d]", indexVar)

		ctx.appendCode(indent+1, "var endOffset uint32\n")
		ctx.appendCode(indent+1, "if %s < %s-1 {\n", indexVar, limitVar)
		ctx.appendCode(indent+2, "if canSeek {\n")
		ctx.appendCode(indent+3, "endOffset = dec.DecodeOffsetAt(%s + int((%s+1)*4))\n", startPosVar, indexVar)
		ctx.appendCode(indent+2, "} else {\n")
		ctx.appendCode(indent+3, "endOffset = offsets[%s]\n", indexVar)
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+1, "} else {\n")
		ctx.appendCode(indent+2, "endOffset = uint32(sszLen)\n")
		ctx.appendCode(indent+1, "}\n")

		ctx.appendCode(indent+1, "if endOffset < startOffset || endOffset > uint32(sszLen) {\n")
		errCode = "sszutils.ErrElementOffsetOutOfRangeFn(endOffset, startOffset, sszLen)"
		ctx.appendCode(indent+2, "return %s\n", fieldPath.getErrorWith(errCode))
		ctx.appendCode(indent+1, "}\n")

		ctx.appendCode(indent+1, "itemSize := endOffset - startOffset\n")
		ctx.appendCode(indent+1, "dec.PushLimit(int(itemSize))\n")
		ctx.appendCode(indent+1, "startOffset = endOffset\n")

		valVar := ctx.getValVar()
		ctx.appendCode(indent+1, "%s := %s[%s]\n", valVar, indexValueVar, indexVar)
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		if err := ctx.unmarshalType(desc.ElemDesc, valVar, fieldPath, indent+1, false, true); err != nil {
			return err
		}

		errCode = errCodeTrailingData
		ctx.appendCode(indent+1, "if diff := dec.PopLimit(); diff != 0 {\n\treturn %s\n}\n", fieldPath.getErrorWith(errCode))

		ctx.appendCode(indent+1, "%s[%s] = %s\n", indexValueVar, indexVar, valVar)
		ctx.appendCode(indent, "}\n")

		ctx.offsetSliceCounter--
	}

	return nil
}

// unmarshalList generates unmarshal code for SSZ list (variable-size array) types.
func (ctx *decoderContext) unmarshalList(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	maxExpression := desc.MaxExpression

	hasMax := false
	maxVar := ""

	switch {
	case maxExpression != nil:
		exprVar := ctx.exprVars.getExprVar(*maxExpression, desc.Limit)

		hasMax = true
		maxVar = fmt.Sprintf("int(%s)", exprVar)
	case desc.Limit > 0:
		maxVar = fmt.Sprintf("%d", desc.Limit)
		hasMax = true
	default:
		maxVar = "0"
	}

	valueVar := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		targetType := ""
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
			targetType = typeNameString
		}
		valueVar = ctx.getValueVar(desc, varName, targetType)
	}

	indexValueVar := valueVar
	if strings.HasPrefix(valueVar, "*") {
		indexValueVar = fmt.Sprintf("(%s)", valueVar)
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
			if hasMax {
				ctx.appendCode(indent, "if dec.GetLength() > %s {\n", maxVar)
				errCode := fmt.Sprintf("sszutils.ErrListLengthFn(dec.GetLength(), %s)", maxVar)
				ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith(errCode))
				ctx.appendCode(indent, "}\n")
			}
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(dec.GetLength()); err != nil {\n")
				ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
				ctx.appendCode(indent, "} else {\n")
				ctx.appendCode(indent+1, "%s = %s\n", valueVar, ctx.getCastedValueVar(desc, "buf", ""))
				ctx.appendCode(indent, "}\n")
			} else {
				ctx.appendCode(indent, "listLen := dec.GetLength()\n")
				if desc.Kind != reflect.Array {
					ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, listLen)\n", valueVar, valueVar)
				}
				ctx.appendCode(indent, "if _, err = dec.DecodeBytes(%s[:listLen]); err != nil {\n\treturn %s\n}\n", indexValueVar, typePath.getErrorWith("err"))
			}
			return nil
		}

		// bulk uint64 lists
		if desc.ElemDesc.SszType == ssztypes.SszUint64Type && desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
			ctx.appendCode(indent, "sszLen := dec.GetLength()\n")
			ctx.appendCode(indent, "itemCount := sszLen / 8\n")
			errCode := "sszutils.ErrListNotAlignedFn(sszLen, 8)"
			ctx.appendCode(indent, "if sszLen%%8 != 0 {\n\treturn %s\n}\n", typePath.getErrorWith(errCode))
			if hasMax {
				errCode = fmt.Sprintf("sszutils.ErrListLengthFn(itemCount, %s)", maxVar)
				ctx.appendCode(indent, "if itemCount > %s {\n\treturn %s\n}\n", maxVar, typePath.getErrorWith(errCode))
			}
			if desc.Kind != reflect.Array {
				ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", valueVar, valueVar)
			}
			ctx.appendCode(indent, "if err = sszutils.DecodeUint64Slice(dec, %s); err != nil {\n\treturn %s\n}\n", valueVar, typePath.getErrorWith("err"))
			return nil
		}

		// static elements
		var fieldSizeVar string
		var err error
		if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 {
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
			errCode := fmt.Sprintf("sszutils.ErrListNotAlignedFn(sszLen, %s)", fieldSizeVar)
			ctx.appendCode(indent, "if sszLen%%%s != 0 {\n\treturn %s\n}\n", fieldSizeVar, typePath.getErrorWith(errCode))
		}
		if hasMax {
			errCode := fmt.Sprintf("sszutils.ErrListLengthFn(itemCount, %s)", maxVar)
			ctx.appendCode(indent, "if itemCount > %s {\n\treturn %s\n}\n", maxVar, typePath.getErrorWith(errCode))
		}
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", valueVar, valueVar)
		}

		startPosVar := fmt.Sprintf("startPos%d", ctx.startPosVarCounter)
		ctx.startPosVarCounter++
		ctx.appendCode(indent, "%s := dec.GetPosition()\n", startPosVar)

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "for %s := range itemCount {\n", indexVar)

		valVar := fmt.Sprintf("%s[%s]", indexValueVar, indexVar)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent+1, "%s := %s[%s]\n", valVar, indexValueVar, indexVar)
		}
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		fieldPath := typePath.append("[%d]", indexVar)
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, fieldPath, indent+1, false, true); err != nil {
			return err
		}

		ctx.appendCode(indent+1, "if dec.GetPosition() != %s+int(%s*(%s+1)) {\n", startPosVar, fieldSizeVar, indexVar)
		errCode := fmt.Sprintf("sszutils.ErrStaticElementNotConsumedFn(dec.GetPosition(), %s+int(%s*(%s+1)))", startPosVar, fieldSizeVar, indexVar)
		ctx.appendCode(indent+2, "return %s\n", fieldPath.getErrorWith(errCode))
		ctx.appendCode(indent+1, "}\n")

		if !isInlinable {
			ctx.appendCode(indent+1, "%s[%s] = %s\n", indexValueVar, indexVar, valVar)
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
		ctx.appendCode(indent+1, "if err != nil {\n\treturn %s\n}\n", typePath.getErrorWith("err"))
		ctx.appendCode(indent, "}\n")
		ctx.appendCode(indent, "itemCount := int(startOffset / 4)\n")

		// read offsets
		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "var offsets []uint32\n")
		ctx.appendCode(indent, "if canSeek {\n")
		ctx.appendCode(indent+1, "dec.SkipBytes((itemCount - 1) * 4)\n")
		ctx.appendCode(indent, "} else if itemCount > 1 {\n")
		ctx.appendCode(indent+1, "offsetSlices[%d] = sszutils.ExpandSlice(offsetSlices[%d], itemCount-1)\n", ctx.offsetSliceCounter, ctx.offsetSliceCounter)
		ctx.appendCode(indent+1, "offsets = offsetSlices[%d]\n", ctx.offsetSliceCounter)
		ctx.appendCode(indent+1, "for %s := range itemCount-1 {\n", indexVar)
		ctx.appendCode(indent+2, "offset, err := dec.DecodeOffset()\n")
		ctx.appendCode(indent+2, "if err != nil {\n")
		ctx.appendCode(indent+3, "return %s\n", typePath.append("[%d:o]", indexVar).getErrorWith("err"))
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+2, "offsets[%s] = offset\n", indexVar)
		ctx.appendCode(indent+1, "}\n")
		ctx.appendCode(indent, "}\n")
		ctx.useSeekable = true
		ctx.offsetSliceCounter++
		if ctx.offsetSliceCounter > ctx.offsetSliceLimit {
			ctx.offsetSliceLimit = ctx.offsetSliceCounter
		}

		errCode := "sszutils.ErrInvalidListStartOffsetFn(startOffset, sszLen)"
		ctx.appendCode(indent, "if startOffset%%4 != 0 || uint32(sszLen) < startOffset {\n\treturn %s\n}\n", typePath.getErrorWith(errCode))
		if hasMax {
			errCode = fmt.Sprintf("sszutils.ErrListLengthFn(itemCount, %s)", maxVar)
			ctx.appendCode(indent, "if itemCount > %s {\n\treturn %s\n}\n", maxVar, typePath.getErrorWith(errCode))
		}
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", valueVar, valueVar)
		}

		fieldPath := typePath.append("[%d]", indexVar)
		ctx.appendCode(indent, "for %s := range itemCount {\n", indexVar)

		ctx.appendCode(indent+1, "var endOffset uint32\n")
		ctx.appendCode(indent+1, "if %s < itemCount-1 {\n", indexVar)
		ctx.appendCode(indent+2, "if canSeek {\n")
		ctx.appendCode(indent+3, "endOffset = dec.DecodeOffsetAt(%s + int((%s+1)*4))\n", startPosVar, indexVar)
		ctx.appendCode(indent+2, "} else {\n")
		ctx.appendCode(indent+3, "endOffset = offsets[%s]\n", indexVar)
		ctx.appendCode(indent+2, "}\n")
		ctx.appendCode(indent+1, "} else {\n")
		ctx.appendCode(indent+2, "endOffset = uint32(sszLen)\n")
		ctx.appendCode(indent+1, "}\n")
		ctx.appendCode(indent+1, "if endOffset < startOffset || endOffset > uint32(sszLen) {\n")
		errCode = "sszutils.ErrElementOffsetOutOfRangeFn(endOffset, startOffset, sszLen)"
		ctx.appendCode(indent+2, "return %s\n", fieldPath.getErrorWith(errCode))
		ctx.appendCode(indent+1, "}\n")

		ctx.appendCode(indent+1, "itemSize := endOffset - startOffset\n")
		ctx.appendCode(indent+1, "dec.PushLimit(int(itemSize))\n")
		ctx.appendCode(indent+1, "startOffset = endOffset\n")

		valVar := ctx.getValVar()
		ctx.appendCode(indent+1, "%s := %s[%s]\n", valVar, indexValueVar, indexVar)
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, fieldPath, indent+1, false, true); err != nil {
			return err
		}
		errCode = errCodeTrailingData
		ctx.appendCode(indent+1, "if diff := dec.PopLimit(); diff != 0 {\n\treturn %s\n}\n", fieldPath.getErrorWith(errCode))
		ctx.appendCode(indent+1, "%s[%s] = %s\n", indexValueVar, indexVar, valVar)
		ctx.appendCode(indent, "}\n")

		ctx.offsetSliceCounter--
	}

	return nil
}

// unmarshalBitlist generates unmarshal code for SSZ bitlist types.
func (ctx *decoderContext) unmarshalBitlist(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	maxExpression := desc.MaxExpression

	hasMax := false
	maxVar := ""

	switch {
	case maxExpression != nil:
		exprVar := ctx.exprVars.getExprVar(*maxExpression, desc.Limit)

		hasMax = true
		maxVar = fmt.Sprintf("int(%s)", exprVar)
	case desc.Limit > 0:
		maxVar = fmt.Sprintf("%d", desc.Limit)
		hasMax = true
	default:
		maxVar = "0"
	}

	ctx.appendCode(indent, "blen := dec.GetLength()\n")

	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, blen)\n", varName, varName)
	}
	ctx.appendCode(indent, "if _, err = dec.DecodeBytes(%s[:blen]); err != nil {\n\treturn %s\n}\n", varName, typePath.getErrorWith("err"))
	ctx.appendCode(indent, "if blen == 0 || %s[blen-1] == 0x00 {\n", varName)
	errCode := errCodeBitlistNotTerminated
	ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
	ctx.appendCode(indent, "}\n")

	if hasMax {
		bitsPkgName := ctx.typePrinter.AddImport("math/bits", "bits")
		ctx.appendCode(indent, "bitCount := 8*(blen-1) + int(%s.Len8(%s[blen-1])) - 1\n", bitsPkgName, varName)
		errCode := fmt.Sprintf("sszutils.ErrBitlistLengthFn(bitCount, %s)", maxVar)
		ctx.appendCode(indent, "if bitCount > %s {\n\treturn %s\n}\n", maxVar, typePath.getErrorWith(errCode))
	}

	return nil
}

// unmarshalUnion generates unmarshal code for SSZ union types.
func (ctx *decoderContext) unmarshalUnion(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	// Read selector
	ctx.appendCode(indent, "selector, err := dec.DecodeUint8()\n")
	ctx.appendCode(indent, "if err != nil {\n\treturn %s\n}\n", typePath.getErrorWith("err"))
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
		if err := ctx.unmarshalType(variantDesc, valVar, typePath.append(fmt.Sprintf("[v:%d]", variant)), indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t%s.Data = %s\n", varName, valVar)
	}

	ctx.appendCode(indent, "default:\n")
	errCode := errCodeInvalidUnionVariant
	ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
	ctx.appendCode(indent, "}\n")

	return nil
}

// unmarshalOptional generates unmarshal code for SSZ optional types.
func (ctx *decoderContext) unmarshalOptional(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	ctx.appendCode(indent, "if hasVal, err := dec.DecodeBool(); err != nil {\n")
	ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
	ctx.appendCode(indent, "} else if hasVal {\n")
	ptrVarName := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		ptrVarName = fmt.Sprintf("*(%s)", varName)
	}
	valVar := ctx.getValVar()
	ctx.appendCode(indent+1, "\tvar %s %s\n", valVar, ctx.typePrinter.TypeString(desc.ElemDesc))
	if err := ctx.unmarshalType(desc.ElemDesc, valVar, typePath, indent+1, false, true); err != nil {
		return err
	}
	ctx.appendCode(indent+1, "\t%s = %s\n", ptrVarName, valVar)
	ctx.appendCode(indent, "} else {\n")
	ctx.appendCode(indent+1, "\t%s = nil\n", varName)
	ctx.appendCode(indent, "}\n")
	return nil
}

// unmarshalBigInt generates unmarshal code for SSZ big int types.
func (ctx *decoderContext) unmarshalBigInt(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	ctx.appendCode(indent, "if buf, err := dec.DecodeBytesBuf(dec.GetLength()); err != nil {\n")
	ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith("err"))
	ctx.appendCode(indent, "} else {\n")
	ptrVarName := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		ptrVarName = fmt.Sprintf("*(%s)", varName)
	}
	mathImport := ctx.typePrinter.AddImport("math/big", "big")
	ctx.appendCode(indent+1, "%s = %s\n", ptrVarName, ctx.getCastedValueVar(desc, fmt.Sprintf("*(%s.NewInt(0).SetBytes(buf))", mathImport), "big.Int"))
	ctx.appendCode(indent, "}\n")
	return nil
}
