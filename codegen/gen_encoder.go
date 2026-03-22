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

// encoderContext contains the state and utilities for generating marshal methods.
//
// This context structure maintains the necessary state during the marshal code generation
// process, including code building utilities, type formatting, and options that control
// the generation behavior.
//
// Fields:
//   - appendCode: Function to append formatted code with proper indentation
//   - typePrinter: Type name formatter and import tracker
//   - options: Code generation options controlling output behavior
//   - usedDynSpecs: Flag tracking whether generated code uses dynamic SSZ functionality
type encoderContext struct {
	appendCode        func(indent int, code string, args ...any)
	typePrinter       *TypePrinter
	options           *CodeGeneratorOptions
	exprVars          *exprVarGenerator
	staticSizeVars    *staticSizeVarGenerator
	usedDynSpecs      bool
	usedSeekable      bool
	usedContext       bool
	sizeFnNameMap     map[*ssztypes.TypeDescriptor]int
	sizeFnSignature   map[string]string
	sizeFnNameCounter int
	indexCounter      int
}

// generateEncoder generates encoder methods for a specific type.
//
// This function creates the complete set of encoder methods for a type, including:
//   - MarshalSSZEncoder for marshaling to a given encoder
//
// Parameters:
//   - rootTypeDesc: Type descriptor containing complete SSZ encoding metadata
//   - codeBuilder: String builder to append generated method code to
//   - typePrinter: Type formatter for handling imports and type names
//   - viewName: Name of the view type for function name postfix (empty string for data type)
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateEncoder(rootTypeDesc *ssztypes.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, viewName string, options *CodeGeneratorOptions) error {
	codeBuf := strings.Builder{}
	ctx := &encoderContext{
		appendCode: func(indent int, code string, args ...any) {
			if len(args) > 0 {
				code = fmt.Sprintf(code, args...)
			}
			codeBuf.WriteString(indentStr(code, indent))
		},
		typePrinter:     typePrinter,
		options:         options,
		sizeFnNameMap:   make(map[*ssztypes.TypeDescriptor]int),
		sizeFnSignature: make(map[string]string),
	}

	ctx.exprVars = newExprVarGenerator("ctx.exprs", typePrinter, options)
	ctx.exprVars.isSlice = true
	ctx.staticSizeVars = newStaticSizeVarGenerator("size", typePrinter, options, ctx.exprVars)

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate marshaling code
	if err := ctx.marshalType(rootTypeDesc, "t", typePathList{}, 0, true); err != nil {
		return err
	}

	// Generate size function code
	sizeFnCode, err := ctx.generateSizeFnCode(0)
	if err != nil {
		return err
	}

	if ctx.exprVars.varCounter > 0 {
		ctx.usedContext = true
		ctx.usedDynSpecs = true
	}

	fnName := "MarshalSSZEncoder"
	if viewName != "" {
		fnName = fmt.Sprintf("marshalSSZEncoderView_%s", viewName)
	}
	if viewName == "" {
		appendCode(codeBuilder, 0, "// MarshalSSZEncoder marshals the %s to the given SSZ encoder using dynamic specifications.\n", typeName)
	}
	appendCode(codeBuilder, 0, "func (t %s) %s(ds sszutils.DynamicSpecs, enc sszutils.Encoder) (err error) {\n", typeName, fnName)

	if ctx.usedContext {
		appendCode(codeBuilder, 1, ctx.generateEncodeContext(0))
		appendCode(codeBuilder, 1, "ctx := &encoderCtx{ds: ds}\n")
	}

	if ctx.usedSeekable {
		appendCode(codeBuilder, 1, "canSeek := enc.Seekable()\n")
	}
	appendCode(codeBuilder, 1, ctx.exprVars.getCode())
	appendCode(codeBuilder, 1, sizeFnCode)
	appendCode(codeBuilder, 1, ctx.staticSizeVars.getCode())
	appendCode(codeBuilder, 1, codeBuf.String())
	appendCode(codeBuilder, 1, "return nil\n")
	appendCode(codeBuilder, 0, "}\n\n")

	return nil
}

// getPtrPrefix returns & for types that are heavy to copy
func (ctx *encoderContext) getPtrPrefix(desc *ssztypes.TypeDescriptor) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		return ""
	}
	if desc.Kind == reflect.Array {
		return "&"
	}
	if desc.Kind == reflect.Struct {
		// use pointer to struct to avoid copying
		return "&"
	}
	return ""
}

// getValueVar returns the variable name for the value of a type, dereferencing pointer types and converting to the target type if needed
func (ctx *encoderContext) getValueVar(desc *ssztypes.TypeDescriptor, varName, targetType string) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
		varName = fmt.Sprintf("*%s", varName)
	}

	if targetType != "" && ctx.typePrinter.InnerTypeString(desc) != targetType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// getIndexVar returns a unique index variable name
func (ctx *encoderContext) getIndexVar() (string, func()) {
	ctx.indexCounter++
	thisIndex := ctx.indexCounter
	return fmt.Sprintf("idx%d", thisIndex), func() {
		ctx.indexCounter = thisIndex - 1
	}
}

// isInlineable checks if a type can be inlined directly into the hash tree root code
func (ctx *encoderContext) isInlineable(desc *ssztypes.TypeDescriptor) bool {
	if desc.SszType == ssztypes.SszBoolType || desc.SszType == ssztypes.SszUint8Type || desc.SszType == ssztypes.SszUint16Type || desc.SszType == ssztypes.SszUint32Type || desc.SszType == ssztypes.SszUint64Type || desc.SszType == ssztypes.SszInt8Type || desc.SszType == ssztypes.SszInt16Type || desc.SszType == ssztypes.SszInt32Type || desc.SszType == ssztypes.SszInt64Type || desc.SszType == ssztypes.SszFloat32Type || desc.SszType == ssztypes.SszFloat64Type {
		return true
	}

	if desc.SszType == ssztypes.SszVectorType || desc.SszType == ssztypes.SszListType {
		return desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0
	}

	return false
}

// getStaticSizeVar generates a variable name for cached static size calculations.
func (ctx *encoderContext) getSizeFnCall(desc *ssztypes.TypeDescriptor, varName string) string {
	if sizeFnIdx, ok := ctx.sizeFnNameMap[desc]; ok {
		return fmt.Sprintf("ctx.sizeFn%d(ctx, %s)", sizeFnIdx, varName)
	}

	ctx.sizeFnNameCounter++
	ctx.sizeFnNameMap[desc] = ctx.sizeFnNameCounter
	ctx.usedContext = true

	return fmt.Sprintf("ctx.sizeFn%d(ctx, %s)", ctx.sizeFnNameCounter, varName)
}

func (ctx *encoderContext) generateSizeFnCode(indent int) (string, error) {
	if len(ctx.sizeFnNameMap) == 0 {
		return "", nil
	}

	codeBuf := strings.Builder{}

	fnTypeList := make([]*ssztypes.TypeDescriptor, 0, len(ctx.sizeFnNameMap))
	for desc := range ctx.sizeFnNameMap {
		fnTypeList = append(fnTypeList, desc)
	}
	slices.SortFunc(fnTypeList, func(a, b *ssztypes.TypeDescriptor) int {
		nameA := ctx.sizeFnNameMap[a]
		nameB := ctx.sizeFnNameMap[b]
		return nameA - nameB
	})

	resetRetVars := ctx.exprVars.withRetVars("0")
	defer resetRetVars()

	for _, desc := range fnTypeList {
		fnName := fmt.Sprintf("sizeFn%d", ctx.sizeFnNameMap[desc])
		sizeCtx := newSizeContext(ctx.typePrinter, ctx.options)
		sizeCtx.exprVars = ctx.exprVars

		sizeFnMap := make(map[*ssztypes.TypeDescriptor]*sizeFnPtr)
		for desc2, idx := range ctx.sizeFnNameMap {
			if desc2 == desc {
				continue
			}
			sizeFnMap[desc2] = &sizeFnPtr{
				fnName:       fmt.Sprintf("ctx.sizeFn%d", idx),
				fnArgs:       []string{"ctx"},
				needDynSpecs: false,
			}
		}
		sizeCtx.useTypeFnMap = sizeFnMap

		ctx.sizeFnSignature[fnName] = fmt.Sprintf("func(ctx *encoderCtx, t %s) (size int)", ctx.typePrinter.TypeString(desc))

		appendCode(&codeBuf, indent, "// size for %s\n", ctx.typePrinter.TypeString(desc))
		appendCode(&codeBuf, indent, "ctx.%s = func(ctx *encoderCtx, t %s) (size int) {\n", fnName, ctx.typePrinter.TypeString(desc))
		if err := sizeCtx.sizeType(desc, "t", "size", 0, false); err != nil {
			return "", err
		}
		appendCode(&codeBuf, indent+1, "%s", sizeCtx.codeBuf.String())
		appendCode(&codeBuf, indent+1, "return size\n")
		appendCode(&codeBuf, indent, "}\n")
	}

	return codeBuf.String(), nil
}

func (ctx *encoderContext) generateEncodeContext(indent int) string {
	codeBuf := strings.Builder{}
	maxFnNameLen := 5 // "exprs"

	fnNameList := make([]string, 0, len(ctx.sizeFnSignature))
	for fnName := range ctx.sizeFnSignature {
		fnNameList = append(fnNameList, fnName)
		if len(fnName) > maxFnNameLen {
			maxFnNameLen = len(fnName)
		}
	}
	slices.SortFunc(fnNameList, strings.Compare)

	padField := func(field string) string {
		return field + strings.Repeat(" ", maxFnNameLen-len(field))
	}

	appendCode(&codeBuf, indent, "type encoderCtx struct {\n")
	appendCode(&codeBuf, indent, "\t%s sszutils.DynamicSpecs\n", padField("ds"))
	appendCode(&codeBuf, indent, "\t%s [%d]uint64\n", padField("exprs"), ctx.exprVars.varCounter)

	for _, fnName := range fnNameList {
		appendCode(&codeBuf, indent, "\t%s %s\n", padField(fnName), ctx.sizeFnSignature[fnName])
	}

	appendCode(&codeBuf, indent, "}\n")

	return codeBuf.String()
}

// marshalType generates marshal code for any SSZ type, delegating to specific marshalers.
func (ctx *encoderContext) marshalType(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int, isRoot bool) error {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.SszType != ssztypes.SszOptionalType {
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	// Handle types that have generated methods we can call
	isView := desc.GoTypeFlags&ssztypes.GoTypeFlagIsView != 0
	if !isRoot && isView {
		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewEncoder != 0 {
			ctx.appendCode(indent, "if viewFn := %s.MarshalSSZEncoderView((%s)(nil)); viewFn != nil {\n", varName, ctx.typePrinter.ViewTypeString(desc, true))
			ctx.appendCode(indent+1, "if err = viewFn(ds, enc); err != nil {\n\treturn err\n}\n")
			ctx.appendCode(indent, "} else {\n\treturn sszutils.ErrNotImplemented\n}\n")
			ctx.usedDynSpecs = true
			return nil
		}

		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewMarshaler != 0 {
			ctx.appendCode(indent, "if viewFn := %s.MarshalSSZDynView((%s)(nil)); viewFn != nil {\n", varName, ctx.typePrinter.ViewTypeString(desc, true))
			ctx.appendCode(indent+1, "if buf, err := viewFn(ds, enc.GetBuffer()); err != nil {\n\treturn err\n} else {\n\tenc.SetBuffer(buf)\n}\n")
			ctx.appendCode(indent, "} else {\n\treturn sszutils.ErrNotImplemented\n}\n")
			ctx.usedDynSpecs = true
			return nil
		}
	}

	hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszMarshaler := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot && !isView {
		ctx.appendCode(indent, "if buf, err := %s.MarshalSSZTo(enc.GetBuffer()); err != nil {\n\treturn %s\n} else {\n\tenc.SetBuffer(buf)\n}\n", varName, typePath.getErrorWith("err"))
		return nil
	}

	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicEncoder != 0 && !isRoot && !isView {
		ctx.appendCode(indent, "if err = %s.MarshalSSZEncoder(ds, enc); err != nil {\n\treturn %s\n}\n", varName, typePath.getErrorWith("err"))
		ctx.usedDynSpecs = true
		return nil
	}

	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicMarshaler != 0 && !isRoot && !isView {
		ctx.appendCode(indent, "if buf, err := %s.MarshalSSZDyn(ds, enc.GetBuffer()); err != nil {\n\treturn %s\n} else {\n\tenc.SetBuffer(buf)\n}\n", varName, typePath.getErrorWith("err"))
		ctx.usedDynSpecs = true
		return nil
	}

	switch desc.SszType {
	case ssztypes.SszBoolType:
		ctx.appendCode(indent, "enc.EncodeBool(%s)\n", ctx.getValueVar(desc, varName, "bool"))

	case ssztypes.SszUint8Type:
		ctx.appendCode(indent, "enc.EncodeUint8(%s)\n", ctx.getValueVar(desc, varName, "byte"))

	case ssztypes.SszUint16Type:
		ctx.appendCode(indent, "enc.EncodeUint16(%s)\n", ctx.getValueVar(desc, varName, "uint16"))

	case ssztypes.SszUint32Type:
		ctx.appendCode(indent, "enc.EncodeUint32(%s)\n", ctx.getValueVar(desc, varName, "uint32"))

	case ssztypes.SszUint64Type:
		valueVar := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			valueVar = fmt.Sprintf("%s.Unix()", varName)
		}
		ctx.appendCode(indent, "enc.EncodeUint64(%s)\n", ctx.getValueVar(desc, valueVar, "uint64"))

	case ssztypes.SszTypeWrapperType:
		ctx.appendCode(indent, "{\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s.Data", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s.Data\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
		}
		if err := ctx.marshalType(desc.ElemDesc, valVar, typePath, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return ctx.marshalContainer(desc, varName, typePath, indent)

	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return ctx.marshalVector(desc, varName, typePath, indent)

	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		return ctx.marshalList(desc, varName, typePath, indent)

	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		return ctx.marshalBitlist(desc, varName, typePath, indent)

	case ssztypes.SszCompatibleUnionType:
		return ctx.marshalUnion(desc, varName, typePath, indent)

	case ssztypes.SszCustomType:
		errCode := errCodeCustomTypeNotSupported
		ctx.appendCode(indent, "return %s\n", typePath.getErrorWith(errCode))

	// extended types
	case ssztypes.SszInt8Type:
		ctx.appendCode(indent, "enc.EncodeUint8(%s)\n", ctx.getValueVar(desc, varName, "byte"))
	case ssztypes.SszInt16Type:
		ctx.appendCode(indent, "enc.EncodeUint16(%s)\n", ctx.getValueVar(desc, varName, "uint16"))
	case ssztypes.SszInt32Type:
		ctx.appendCode(indent, "enc.EncodeUint32(%s)\n", ctx.getValueVar(desc, varName, "uint32"))
	case ssztypes.SszInt64Type:
		ctx.appendCode(indent, "enc.EncodeUint64(%s)\n", ctx.getValueVar(desc, varName, "uint64"))
	case ssztypes.SszFloat32Type:
		mathImport := ctx.typePrinter.AddImport("math", "math")
		ctx.appendCode(indent, "enc.EncodeUint32(%s.Float32bits(%s))\n", mathImport, ctx.getValueVar(desc, varName, "float32"))
	case ssztypes.SszFloat64Type:
		mathImport := ctx.typePrinter.AddImport("math", "math")
		ctx.appendCode(indent, "enc.EncodeUint64(%s.Float64bits(%s))\n", mathImport, ctx.getValueVar(desc, varName, "float64"))
	case ssztypes.SszOptionalType:
		return ctx.marshalOptional(desc, varName, typePath, indent)
	case ssztypes.SszBigIntType:
		return ctx.marshalBigInt(desc, varName, indent)

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// marshalOptional generates marshal code for SSZ optional types.
func (ctx *encoderContext) marshalOptional(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	ctx.appendCode(indent, "if %s == nil {\n", varName)
	ctx.appendCode(indent+1, "enc.EncodeBool(false)\n")
	ctx.appendCode(indent, "} else {\n")
	ctx.appendCode(indent+1, "enc.EncodeBool(true)\n")
	innerVarName := fmt.Sprintf("(*%s)", varName)
	if err := ctx.marshalType(desc.ElemDesc, innerVarName, typePath, indent+1, false); err != nil {
		return err
	}
	ctx.appendCode(indent, "}\n")
	return nil
}

// marshalBigInt generates marshal code for SSZ big int types.
func (ctx *encoderContext) marshalBigInt(_ *ssztypes.TypeDescriptor, varName string, indent int) error {
	ctx.appendCode(indent, "enc.EncodeBytes(%s.Bytes())\n", varName)
	return nil
}

// marshalContainer generates marshal code for SSZ container (struct) types.
func (ctx *encoderContext) marshalContainer(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	// Pre-scan for dynamic fields since staticSizeVars are only needed for dynoff
	hasDynamic := false
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			hasDynamic = true
			break
		}
	}

	staticSize := 0
	staticSizeVars := []string{}
	for _, field := range desc.ContainerDesc.Fields {
		switch {
		case field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0:
			staticSize += 4
		case hasDynamic && field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions:
			sizeVar, err := ctx.staticSizeVars.getStaticSizeVar(field.Type)
			if err != nil {
				return err
			}
			staticSizeVars = append(staticSizeVars, sizeVar)
		default:
			staticSize += int(field.Type.Size)
		}
	}
	staticSizeVars = append(staticSizeVars, fmt.Sprintf("%d", staticSize))

	if hasDynamic {
		ctx.usedSeekable = true
		ctx.appendCode(indent, "dstlen := enc.GetPosition()\n")
		ctx.appendCode(indent, "dynoff := uint32(%v)\n", strings.Join(staticSizeVars, " + "))
	}

	// Write offsets for dynamic fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			ctx.appendCode(indent, "// Offset #%d '%s'\n", idx, field.Name)
			ctx.appendCode(indent, "offset%d := enc.GetPosition()\n", idx)
			ctx.appendCode(indent, "if canSeek {\n")
			ctx.appendCode(indent+1, "enc.EncodeOffset(0)\n")
			ctx.appendCode(indent, "} else {\n")
			ctx.appendCode(indent+1, "enc.EncodeOffset(dynoff)\n")
			sizeFnCall := ctx.getSizeFnCall(field.Type, fmt.Sprintf("%s.%s", varName, field.Name))
			ctx.appendCode(indent+1, "dynoff += uint32(%s)\n", sizeFnCall)
			ctx.appendCode(indent, "}\n")
		} else {
			// Marshal fixed fields
			ctx.appendCode(indent, "{ // Field #%d '%s'\n", idx, field.Name)
			valVar := "t"
			if ctx.isInlineable(field.Type) {
				valVar = fmt.Sprintf("%s.%s", varName, field.Name)
			} else {
				ctx.appendCode(indent, "\tt := %s%s.%s\n", ctx.getPtrPrefix(field.Type), varName, field.Name)
			}
			if err := ctx.marshalType(field.Type, valVar, typePath.append(field.Name), indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	// Marshal dynamic fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			continue
		}

		ctx.appendCode(indent, "{ // Dynamic Field #%d '%s'\n", idx, field.Name)

		ctx.appendCode(indent, "\tif canSeek {\n")
		ctx.appendCode(indent, "\t\tenc.EncodeOffsetAt(offset%d, uint32(enc.GetPosition()-dstlen))\n", idx)
		ctx.appendCode(indent, "\t}\n")

		valVar := "t"
		if ctx.isInlineable(field.Type) {
			valVar = fmt.Sprintf("%s.%s", varName, field.Name)
		} else {
			ctx.appendCode(indent, "\tt := %s%s.%s\n", ctx.getPtrPrefix(field.Type), varName, field.Name)
		}
		if err := ctx.marshalType(field.Type, valVar, typePath.append(field.Name), indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// marshalVector generates marshal code for SSZ vector (fixed-size array) types.
func (ctx *encoderContext) marshalVector(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	bitlimitVar := ""
	hasLimitVar := false
	if sizeExpression != nil {
		defaultValue := uint64(desc.Len)
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 {
			if desc.BitSize > 0 {
				defaultValue = uint64(desc.BitSize)
			} else {
				defaultValue = uint64(desc.Len * 8)
			}
		}

		exprVar := ctx.exprVars.getExprVar(*sizeExpression, defaultValue)

		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 {
			bitlimitVar = exprVar
			limitVar = fmt.Sprintf("int((%s+7)/8)", exprVar)
		} else {
			limitVar = fmt.Sprintf("int(%s)", exprVar)
		}

		hasLimitVar = true

		if desc.Kind == reflect.Array {
			// check if dynamic limit is greater than the length of the array
			ctx.appendCode(indent, "if %s > %d {\n", limitVar, desc.Len)
			errCode := fmt.Sprintf("sszutils.ErrVectorSizeExceedsArrayFn(%s, %d)", limitVar, desc.Len)
			ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith(errCode))
			ctx.appendCode(indent, "}\n")
		}
	} else {
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 && desc.BitSize > 0 && desc.BitSize%8 != 0 {
			bitlimitVar = fmt.Sprintf("%d", desc.BitSize)
		}
		limitVar = fmt.Sprintf("%d", desc.Len)
	}

	valueVar := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
		valueVar = ctx.getValueVar(desc, varName, "string")
	}

	lenVar := ""
	switch {
	case desc.Kind != reflect.Array:
		ctx.appendCode(indent, "%s := len(%s)\n", varNameVLen, valueVar)
		ctx.appendCode(indent, "if %s > %s {\n", varNameVLen, limitVar)
		errCode := fmt.Sprintf("sszutils.ErrVectorLengthFn(%s, %s)", varNameVLen, limitVar)
		ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith(errCode))
		ctx.appendCode(indent, "}\n")
		lenVar = varNameVLen
	case hasLimitVar:
		ctx.appendCode(indent, varNameVLen+" := %d\n", desc.Len)
		ctx.appendCode(indent, "if "+varNameVLen+" > %s {\n\t"+varNameVLen+" = %s\n}\n", limitVar, limitVar)
		lenVar = varNameVLen
	default:
		lenVar = fmt.Sprintf("%d", desc.Len)
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static elements
		switch {
		case desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0:
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				valueVar = fmt.Sprintf("[]byte(%s)", valueVar)
			}
			if strings.HasPrefix(valueVar, "*") {
				valueVar = fmt.Sprintf("(%s)", valueVar)
			}
			if bitlimitVar != "" {
				ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
				ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", valueVar, lenVar)
				errCode := errCodeBitvectorPadding
				ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
				ctx.appendCode(indent, "}\n")
			}
			ctx.appendCode(indent, "enc.EncodeBytes(%s[:%s])\n", valueVar, lenVar)
		case desc.ElemDesc.SszType == ssztypes.SszUint64Type && desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0:
			ctx.appendCode(indent, "sszutils.EncodeUint64Slice(enc, %s[:%s])\n", varName, lenVar)
		default:
			indexVar, indexDefer := ctx.getIndexVar()
			defer indexDefer()

			ctx.appendCode(indent, "for %s := range %s {\n", indexVar, lenVar)
			valVar := "t"
			if ctx.isInlineable(desc.ElemDesc) {
				valVar = fmt.Sprintf("%s[%s]", varName, indexVar)
			} else {
				ctx.appendCode(indent, "\tt := %s%s[%s]\n", ctx.getPtrPrefix(desc.ElemDesc), varName, indexVar)
			}
			if err := ctx.marshalType(desc.ElemDesc, valVar, typePath.append("[%d]", indexVar), indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\tenc.EncodeZeroPadding((%s - %s) * %d)\n", limitVar, lenVar, desc.ElemDesc.Size)
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements
		// reserve space for offsets
		ctx.appendCode(indent, "dstlen := enc.GetPosition()\n")

		ctx.usedSeekable = true
		ctx.appendCode(indent, "if canSeek {\n")
		ctx.appendCode(indent, "\tenc.EncodeZeroPadding(%s * 4)\n", limitVar)
		ctx.appendCode(indent, "} else {\n")

		sizeFnCall := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[i]", varName))
		ctx.appendCode(indent, "\toffset := %s * 4\n", lenVar)
		ctx.appendCode(indent, "\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\tfor i := range %s-1 {\n", lenVar)
		ctx.appendCode(indent, "\t\toffset += %s\n", sizeFnCall)
		ctx.appendCode(indent, "\t\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\t}\n")

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "\tif %s < %s {\n", lenVar, limitVar)
			sizeFnCall := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[%s]", varName, lenVar))
			ctx.appendCode(indent, "\t\toffset += %s\n", sizeFnCall)
			ctx.appendCode(indent, "\t\tenc.EncodeOffset(uint32(offset))\n")
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent, "\t\tzeroItem := new(%s)\n", ctx.typePrinter.InnerTypeString(desc.ElemDesc))
			} else {
				ctx.appendCode(indent, "\t\tvar zeroItem %s\n", ctx.typePrinter.TypeString(desc.ElemDesc))
			}

			zeroItemSizeFnCall := ctx.getSizeFnCall(desc.ElemDesc, "zeroItem")
			ctx.appendCode(indent, "\t\tzeroSize := %s\n", zeroItemSizeFnCall)
			ctx.appendCode(indent, "\t\tfor i := %s; i < %s-1; i++ {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\t\t\toffset += zeroSize\n")
			ctx.appendCode(indent, "\t\t\tenc.EncodeOffset(uint32(offset))\n")
			ctx.appendCode(indent, "\t\t}\n")
			ctx.appendCode(indent, "\t}\n")
		}

		ctx.appendCode(indent, "}\n")

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()
		ctx.appendCode(indent, "for %s := range %s {\n", indexVar, lenVar)

		ctx.appendCode(indent, "\tif canSeek {\n")
		ctx.appendCode(indent, "\t\tenc.EncodeOffsetAt(dstlen+(%s*4), uint32(enc.GetPosition() - dstlen))\n", indexVar)
		ctx.appendCode(indent, "\t}\n")

		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s[%s]", varName, indexVar)
		} else {
			ctx.appendCode(indent, "\tt := %s%s[%s]\n", ctx.getPtrPrefix(desc.ElemDesc), varName, indexVar)
		}
		if err := ctx.marshalType(desc.ElemDesc, valVar, typePath.append("[%d]", indexVar), indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent, "\tzeroItem := new(%s)\n", ctx.typePrinter.InnerTypeString(desc.ElemDesc))
			} else {
				ctx.appendCode(indent, "\tvar zeroItem %s\n", ctx.typePrinter.TypeString(desc.ElemDesc))
			}
			ctx.appendCode(indent, "\tfor %s := %s; %s < %s; %s++ {\n", indexVar, lenVar, indexVar, limitVar, indexVar)
			ctx.appendCode(indent, "\t\tif canSeek {\n")
			ctx.appendCode(indent, "\t\t\tenc.EncodeOffsetAt(dstlen+(%s*4), uint32(enc.GetPosition()-dstlen))\n", indexVar)
			ctx.appendCode(indent, "\t\t}\n")
			if err := ctx.marshalType(desc.ElemDesc, "zeroItem", typePath.append("[+%d]", indexVar), indent+2, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t}\n")
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// marshalList generates marshal code for SSZ list (variable-size array) types.
func (ctx *encoderContext) marshalList(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

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
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
		valueVar = ctx.getValueVar(desc, varName, "string")
	}

	hasVlen := false
	addVlen := func() {
		if hasVlen {
			return
		}
		ctx.appendCode(indent, "vlen := len(%s)\n", valueVar)
		hasVlen = true
	}

	if hasMax {
		addVlen()
		ctx.appendCode(indent, "if vlen > %s {\n", maxVar)
		errCode := fmt.Sprintf("sszutils.ErrListLengthFn(vlen, %s)", maxVar)
		ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
		ctx.appendCode(indent, "}\n")
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static elements
		switch {
		case desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0:
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				valueVar = fmt.Sprintf("[]byte(%s)", valueVar)
			}
			if strings.HasPrefix(valueVar, "*") {
				valueVar = fmt.Sprintf("(%s)", valueVar)
			}
			ctx.appendCode(indent, "enc.EncodeBytes(%s[:])\n", valueVar)
		case desc.ElemDesc.SszType == ssztypes.SszUint64Type && desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0:
			addVlen()
			ctx.appendCode(indent, "sszutils.EncodeUint64Slice(enc, %s[:vlen])\n", varName)
		default:
			addVlen()
			indexVar, indexDefer := ctx.getIndexVar()
			defer indexDefer()

			ctx.appendCode(indent, "for %s := range vlen {\n", indexVar)
			valVar := "t"
			if ctx.isInlineable(desc.ElemDesc) {
				valVar = fmt.Sprintf("%s[%s]", varName, indexVar)
			} else {
				ctx.appendCode(indent, "\tt := %s%s[%s]\n", ctx.getPtrPrefix(desc.ElemDesc), varName, indexVar)
			}
			if err := ctx.marshalType(desc.ElemDesc, valVar, typePath.append("[%d]", indexVar), indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements
		// reserve space for offsets
		ctx.appendCode(indent, "dstlen := enc.GetPosition()\n")
		addVlen()
		ctx.appendCode(indent, "if canSeek {\n")
		ctx.appendCode(indent, "\tenc.EncodeZeroPadding(vlen * 4)\n")
		ctx.appendCode(indent, "} else if vlen > 0 {\n")
		sizeFnCall := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[i]", varName))
		ctx.appendCode(indent, "\toffset := vlen * 4\n")
		ctx.appendCode(indent, "\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\tfor i := range vlen-1 {\n")
		ctx.appendCode(indent, "\t\toffset += %s\n", sizeFnCall)
		ctx.appendCode(indent, "\t\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "}\n")

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "for %s := range vlen {\n", indexVar)
		ctx.appendCode(indent, "\tif canSeek {\n")
		ctx.appendCode(indent, "\t\tenc.EncodeOffsetAt(dstlen+(%s*4), uint32(enc.GetPosition()-dstlen))\n", indexVar)
		ctx.appendCode(indent, "\t}\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s[%s]", varName, indexVar)
		} else {
			ctx.appendCode(indent, "\tt := %s%s[%s]\n", ctx.getPtrPrefix(desc.ElemDesc), varName, indexVar)
		}
		if err := ctx.marshalType(desc.ElemDesc, valVar, typePath.append("[%d]", indexVar), indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

//nolint:dupl // intentionally similar to marshalContext.marshalBitlist
func (ctx *encoderContext) marshalBitlist(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

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

	ctx.appendCode(indent, "vlen := len(%s)\n", varName)

	if hasMax {
		ctx.appendCode(indent, "if vlen > %s {\n", maxVar)
		errCode := fmt.Sprintf("sszutils.ErrBitlistLengthFn(vlen, %s)", maxVar)
		ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
		ctx.appendCode(indent, "}\n")
	}

	ctx.appendCode(indent, "bval := []byte(%s[:])\n", varName)
	ctx.appendCode(indent, "if vlen == 0 {\n")
	ctx.appendCode(indent, "\tbval = []byte{0x01}\n")
	ctx.appendCode(indent, "} else if bval[vlen-1] == 0x00 {\n")
	errCode := errCodeBitlistNotTerminated
	ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
	ctx.appendCode(indent, "}\n")

	ctx.appendCode(indent, "enc.EncodeBytes(bval)\n")

	return nil
}

// marshalUnion generates marshal code for SSZ union types.
func (ctx *encoderContext) marshalUnion(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	ctx.appendCode(indent, "enc.EncodeUint8(%s.Variant)\n", varName)
	ctx.appendCode(indent, "switch %s.Variant {\n", varName)

	variants := make([]int, 0, len(desc.UnionVariants))
	for variant := range desc.UnionVariants {
		variants = append(variants, int(variant))
	}
	slices.Sort(variants)

	for _, variant := range variants {
		variantDesc := desc.UnionVariants[uint8(variant)]
		variantType := ctx.typePrinter.TypeString(variantDesc)
		variantPath := typePath.append(fmt.Sprintf("[v:%d]", variant))
		ctx.appendCode(indent, "case %d:\n", variant)
		ctx.appendCode(indent, "\tv, ok := %s.Data.(%s)\n", varName, variantType)
		ctx.appendCode(indent, "\tif !ok {\n")
		errCode := errCodeUnionTypeMismatch
		ctx.appendCode(indent, "\t\treturn %s\n", variantPath.getErrorWith(errCode))
		ctx.appendCode(indent, "\t}\n")
		if err := ctx.marshalType(variantDesc, "v", variantPath, indent+1, false); err != nil {
			return err
		}
	}
	ctx.appendCode(indent, "default:\n")
	errCode := errCodeInvalidUnionVariant
	ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
	ctx.appendCode(indent, "}\n")

	return nil
}
