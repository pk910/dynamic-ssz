// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"slices"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
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
	appendExprCode    func(indent int, code string, args ...any)
	typePrinter       *TypePrinter
	options           *CodeGeneratorOptions
	usedDynSpecs      bool
	usedCanSeek       bool
	usedContext       bool
	exprVarCounter    int
	exprVarMap        map[[32]byte]string
	sizeFnNameMap     map[*dynssz.TypeDescriptor]int
	sizeFnSignature   map[string]string
	sizeFnNameCounter int
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
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateEncoder(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) error {
	codeBuf := strings.Builder{}
	exprCodeBuf := strings.Builder{}
	ctx := &encoderContext{
		appendCode: func(indent int, code string, args ...any) {
			if len(args) > 0 {
				code = fmt.Sprintf(code, args...)
			}
			codeBuf.WriteString(indentStr(code, indent))
		},
		appendExprCode: func(indent int, code string, args ...any) {
			if len(args) > 0 {
				code = fmt.Sprintf(code, args...)
			}
			exprCodeBuf.WriteString(indentStr(code, indent))
		},
		typePrinter:     typePrinter,
		options:         options,
		exprVarMap:      make(map[[32]byte]string),
		sizeFnNameMap:   make(map[*dynssz.TypeDescriptor]int),
		sizeFnSignature: make(map[string]string),
	}

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate marshaling code
	if err := ctx.marshalType(rootTypeDesc, "t", 1, true); err != nil {
		return err
	}

	// Generate size function code
	sizeFnCode, err := ctx.generateSizeFnCode(1)
	if err != nil {
		return err
	}

	codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZEncoder(ds sszutils.DynamicSpecs, enc sszutils.Encoder) (err error) {\n", typeName))

	if ctx.usedContext {
		codeBuilder.WriteString(ctx.generateEncodeContext(1))
		codeBuilder.WriteString("\tctx := &encoderCtx{ds: ds}\n")
	}

	if ctx.usedCanSeek {
		codeBuilder.WriteString("\tcanSeek := enc.CanSeek()\n")
	}
	codeBuilder.WriteString(exprCodeBuf.String())
	codeBuilder.WriteString(sizeFnCode)
	codeBuilder.WriteString(codeBuf.String())
	codeBuilder.WriteString("\treturn nil\n")
	codeBuilder.WriteString("}\n\n")

	return nil
}

// getPtrPrefix returns & for types that are heavy to copy
func (ctx *encoderContext) getPtrPrefix(desc *dynssz.TypeDescriptor) string {
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
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

// isInlineable checks if a type can be inlined directly into the hash tree root code
func (ctx *encoderContext) isInlineable(desc *dynssz.TypeDescriptor) bool {
	if desc.SszType == dynssz.SszBoolType || desc.SszType == dynssz.SszUint8Type || desc.SszType == dynssz.SszUint16Type || desc.SszType == dynssz.SszUint32Type || desc.SszType == dynssz.SszUint64Type {
		return true
	}

	if desc.SszType == dynssz.SszVectorType || desc.SszType == dynssz.SszListType {
		return desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0
	}

	return false
}

// getExprVar generates a variable name for cached limit expression calculations.
func (ctx *encoderContext) getExprVar(expr string, defaultValue uint64) string {
	if expr == "" {
		return fmt.Sprintf("%v", defaultValue)
	}

	exprKey := sha256.Sum256([]byte(fmt.Sprintf("%s\n%v", expr, defaultValue)))
	if exprVar, ok := ctx.exprVarMap[exprKey]; ok {
		return exprVar
	}

	exprVar := fmt.Sprintf("ctx.exprs[%d]", ctx.exprVarCounter)
	ctx.exprVarCounter++
	ctx.usedContext = true

	ctx.appendExprCode(1, "%s, err = sszutils.ResolveSpecValueWithDefault(ds, \"%s\", %d)\n", exprVar, expr, defaultValue)
	ctx.appendExprCode(1, "if err != nil {\n\treturn err\n}\n")
	ctx.usedDynSpecs = true

	ctx.exprVarMap[exprKey] = exprVar

	return exprVar
}

// getStaticSizeVar generates a variable name for cached static size calculations.
func (ctx *encoderContext) getSizeFnCall(desc *dynssz.TypeDescriptor, varName string) string {
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

	fnTypeList := make([]*dynssz.TypeDescriptor, 0, len(ctx.sizeFnNameMap))
	for desc := range ctx.sizeFnNameMap {
		fnTypeList = append(fnTypeList, desc)
	}
	slices.SortFunc(fnTypeList, func(a, b *dynssz.TypeDescriptor) int {
		nameA := ctx.sizeFnNameMap[a]
		nameB := ctx.sizeFnNameMap[b]
		return nameA - nameB
	})

	for _, desc := range fnTypeList {
		fnName := fmt.Sprintf("sizeFn%d", ctx.sizeFnNameMap[desc])
		sizeCtx := newSizeContext(ctx.typePrinter, ctx.options)

		sizeFnMap := make(map[*dynssz.TypeDescriptor]*sizeFnPtr)
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

		sizeCtx.getExprVarFn = func(expr string, defaultValue uint64) string {
			return ctx.getExprVar(expr, defaultValue)
		}

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
	if len(ctx.sizeFnSignature) == 0 {
		return ""
	}

	codeBuf := strings.Builder{}
	maxFnNameLen := 5 // "exprs"

	fnNameList := make([]string, 0, len(ctx.sizeFnSignature))
	for fnName := range ctx.sizeFnSignature {
		fnNameList = append(fnNameList, fnName)
		if len(fnName) > maxFnNameLen {
			maxFnNameLen = len(fnName)
		}
	}
	slices.SortFunc(fnNameList, func(a, b string) int {
		return strings.Compare(a, b)
	})

	padField := func(field string) string {
		return field + strings.Repeat(" ", maxFnNameLen-len(field))
	}

	appendCode(&codeBuf, indent, "type encoderCtx struct {\n")
	appendCode(&codeBuf, indent, "\t%s sszutils.DynamicSpecs\n", padField("ds"))
	appendCode(&codeBuf, indent, "\t%s [%d]uint64\n", padField("exprs"), ctx.exprVarCounter)

	for _, fnName := range fnNameList {
		appendCode(&codeBuf, indent, "\t%s %s\n", padField(fnName), ctx.sizeFnSignature[fnName])
	}

	appendCode(&codeBuf, indent, "}\n")

	return codeBuf.String()
}

// marshalType generates marshal code for any SSZ type, delegating to specific marshalers.
func (ctx *encoderContext) marshalType(desc *dynssz.TypeDescriptor, varName string, indent int, isRoot bool) error {
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	// Handle types that have generated methods we can call
	hasDynamicSize := desc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszMarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "if buf, err := %s.MarshalSSZTo(enc.GetBuffer()); err != nil {\n\treturn err\n} else {\n\tenc.SetBuffer(buf)\n}\n", varName)
		return nil
	}

	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicEncoder != 0 && !isRoot {
		ctx.appendCode(indent, "if err = %s.MarshalSSZEncoder(ds, enc); err != nil {\n\treturn err\n}\n", varName)
		ctx.usedDynSpecs = true
		return nil
	}

	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicMarshaler != 0 && !isRoot {
		ctx.appendCode(indent, "if buf, err := %s.MarshalSSZDyn(ds, enc.GetBuffer()); err != nil {\n\treturn err\n} else {\n\tenc.SetBuffer(buf)\n}\n", varName)
		ctx.usedDynSpecs = true
		return nil
	}

	switch desc.SszType {
	case dynssz.SszBoolType:
		ctx.appendCode(indent, "enc.EncodeBool(bool(%s))\n", varName)

	case dynssz.SszUint8Type:
		ctx.appendCode(indent, "enc.EncodeUint8(uint8(%s))\n", varName)

	case dynssz.SszUint16Type:
		ctx.appendCode(indent, "enc.EncodeUint16(uint16(%s))\n", varName)

	case dynssz.SszUint32Type:
		ctx.appendCode(indent, "enc.EncodeUint32(uint32(%s))\n", varName)

	case dynssz.SszUint64Type:
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsTime != 0 {
			ctx.appendCode(indent, "enc.EncodeUint64(uint64(%s.Unix()))\n", varName)
		} else {
			ctx.appendCode(indent, "enc.EncodeUint64(uint64(%s))\n", varName)
		}

	case dynssz.SszTypeWrapperType:
		ctx.appendCode(indent, "{\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s.Data", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s.Data\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
		}
		if err := ctx.marshalType(desc.ElemDesc, valVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

	case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
		return ctx.marshalContainer(desc, varName, indent)

	case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
		return ctx.marshalVector(desc, varName, indent)

	case dynssz.SszListType, dynssz.SszProgressiveListType:
		return ctx.marshalList(desc, varName, indent)

	case dynssz.SszBitlistType, dynssz.SszProgressiveBitlistType:
		return ctx.marshalBitlist(desc, varName, indent)

	case dynssz.SszCompatibleUnionType:
		return ctx.marshalUnion(desc, varName, indent)

	case dynssz.SszCustomType:
		ctx.appendCode(indent, "return sszutils.ErrNotImplemented\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// marshalContainer generates marshal code for SSZ container (struct) types.
func (ctx *encoderContext) marshalContainer(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	hasDynamic := false
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			hasDynamic = true
			break
		}
	}

	if hasDynamic {
		ctx.usedCanSeek = true
		ctx.appendCode(indent, "dstlen := enc.GetPosition()\n")
		ctx.appendCode(indent, "dynoff := uint32(%v)\n", desc.Len)
	}

	// Write offsets for dynamic fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
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
			if err := ctx.marshalType(field.Type, valVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	// Marshal dynamic fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
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
			if err := ctx.marshalType(field.Type, valVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// marshalVector generates marshal code for SSZ vector (fixed-size array) types.
func (ctx *encoderContext) marshalVector(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	bitlimitVar := ""
	hasLimitVar := false
	if sizeExpression != nil {
		defaultValue := uint64(desc.Len)
		if desc.SszTypeFlags&dynssz.SszTypeFlagHasBitSize != 0 {
			if desc.BitSize > 0 {
				defaultValue = uint64(desc.BitSize)
			} else {
				defaultValue = uint64(desc.Len * 8)
			}
		}

		exprVar := ctx.getExprVar(*sizeExpression, defaultValue)

		if desc.SszTypeFlags&dynssz.SszTypeFlagHasBitSize != 0 {
			bitlimitVar = exprVar
			limitVar = fmt.Sprintf("int((%s+7)/8)", exprVar)
		} else {
			limitVar = fmt.Sprintf("int(%s)", exprVar)
		}

		hasLimitVar = true

		if desc.Kind == reflect.Array {
			// check if dynamic limit is greater than the length of the array
			ctx.appendCode(indent, "if %s > %d {\n", limitVar, desc.Len)
			ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
			ctx.appendCode(indent, "}\n")
		}
	} else {
		if desc.SszTypeFlags&dynssz.SszTypeFlagHasBitSize != 0 && desc.BitSize > 0 && desc.BitSize%8 != 0 {
			bitlimitVar = fmt.Sprintf("%d", desc.BitSize)
		}
		limitVar = fmt.Sprintf("%d", desc.Len)
	}

	lenVar := ""
	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "vlen := len(%s)\n", varName)
		ctx.appendCode(indent, "if vlen > %s {\n", limitVar)
		ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
		ctx.appendCode(indent, "}\n")
		lenVar = "vlen"
	} else if hasLimitVar {
		ctx.appendCode(indent, "vlen := %d\n", desc.Len)
		ctx.appendCode(indent, "if vlen > %s {\n\tvlen = %s\n}\n", limitVar, limitVar)
		lenVar = "vlen"
	} else {
		lenVar = fmt.Sprintf("%d", desc.Len)
	}

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			ctx.appendCode(indent, "enc.EncodeBytes([]byte(%s[:%s]))\n", varName, lenVar)
		} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			if bitlimitVar != "" {
				ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
				ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", varName, lenVar)
				ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
				ctx.appendCode(indent, "}\n")
			}
			ctx.appendCode(indent, "enc.EncodeBytes(%s[:%s])\n", varName, lenVar)
		} else {
			ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", lenVar)
			valVar := "t"
			if ctx.isInlineable(desc.ElemDesc) {
				valVar = fmt.Sprintf("%s[i]", varName)
			} else {
				ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
			}
			if err := ctx.marshalType(desc.ElemDesc, valVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\tenc.EncodeZeroPadding((%s-%s)*%d)\n", limitVar, lenVar, desc.ElemDesc.Size)
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements
		// reserve space for offsets
		ctx.appendCode(indent, "dstlen := enc.GetPosition()\n")

		ctx.usedCanSeek = true
		ctx.appendCode(indent, "if canSeek {\n")
		ctx.appendCode(indent, "\tenc.EncodeZeroPadding(%s*4)\n", limitVar)
		ctx.appendCode(indent, "} else {\n")

		sizeFnCall := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[i]", varName))
		ctx.appendCode(indent, "\toffset := %s * 4\n", lenVar)
		ctx.appendCode(indent, "\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\tfor i := 0; i < %s-1; i++ {\n", lenVar)
		ctx.appendCode(indent, "\t\toffset += %s\n", sizeFnCall)
		ctx.appendCode(indent, "\t\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\t}\n")

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "\tif %s < %s {\n", lenVar, limitVar)
			sizeFnCall := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[%s]", varName, lenVar))
			ctx.appendCode(indent, "\t\toffset += %s\n", sizeFnCall)
			ctx.appendCode(indent, "\t\tenc.EncodeOffset(uint32(offset))\n")
			if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
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

		ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", lenVar)

		ctx.appendCode(indent, "\tif canSeek {\n")
		ctx.appendCode(indent, "\t\tenc.EncodeOffsetAt(dstlen+(i*4), uint32(enc.GetPosition() - dstlen))\n")
		ctx.appendCode(indent, "\t}\n")

		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s[i]", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
		}
		if err := ctx.marshalType(desc.ElemDesc, valVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)
			if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent, "\tzeroItem := new(%s)\n", ctx.typePrinter.InnerTypeString(desc.ElemDesc))
			} else {
				ctx.appendCode(indent, "\tvar zeroItem %s\n", ctx.typePrinter.TypeString(desc.ElemDesc))
			}
			ctx.appendCode(indent, "\tfor i := %s; i < %s; i++ {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\t\tif canSeek {\n")
			ctx.appendCode(indent, "\t\t\tenc.EncodeOffsetAt(dstlen+(i*4), uint32(enc.GetPosition() - dstlen))\n")
			ctx.appendCode(indent, "\t\t}\n")
			if err := ctx.marshalType(desc.ElemDesc, "zeroItem", indent+2, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t}\n")
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// marshalList generates marshal code for SSZ list (variable-size array) types.
func (ctx *encoderContext) marshalList(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	hasMax := false
	maxVar := ""

	if maxExpression != nil {
		exprVar := ctx.getExprVar(*maxExpression, uint64(desc.Limit))

		hasMax = true
		maxVar = fmt.Sprintf("int(%s)", exprVar)
	} else if desc.Limit > 0 {
		maxVar = fmt.Sprintf("%d", desc.Limit)
		hasMax = true
	} else {
		maxVar = "0"
	}

	hasVlen := false
	addVlen := func() {
		if hasVlen {
			return
		}
		ctx.appendCode(indent, "vlen := len(%s)\n", varName)
		hasVlen = true
	}

	if hasMax {
		addVlen()
		ctx.appendCode(indent, "if vlen > %s {\n", maxVar)
		ctx.appendCode(indent, "\treturn sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			ctx.appendCode(indent, "enc.EncodeBytes([]byte(%s))\n", varName)
		} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			ctx.appendCode(indent, "enc.EncodeBytes(%s[:])\n", varName)
		} else {
			addVlen()
			ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
			valVar := "t"
			if ctx.isInlineable(desc.ElemDesc) {
				valVar = fmt.Sprintf("%s[i]", varName)
			} else {
				ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
			}
			if err := ctx.marshalType(desc.ElemDesc, valVar, indent+1, false); err != nil {
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
		ctx.appendCode(indent, "\tenc.EncodeZeroPadding(vlen*4)\n")
		ctx.appendCode(indent, "} else if vlen > 0 {\n")
		sizeFnCall := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[i]", varName))
		ctx.appendCode(indent, "\toffset := vlen * 4\n")
		ctx.appendCode(indent, "\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\tfor i := 0; i < vlen-1; i++ {\n")
		ctx.appendCode(indent, "\t\toffset += %s\n", sizeFnCall)
		ctx.appendCode(indent, "\t\tenc.EncodeOffset(uint32(offset))\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "}\n")

		ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
		ctx.appendCode(indent, "\tif canSeek {\n")
		ctx.appendCode(indent, "\t\tenc.EncodeOffsetAt(dstlen+(i*4), uint32(enc.GetPosition() - dstlen))\n")
		ctx.appendCode(indent, "\t}\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s[i]", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
		}
		if err := ctx.marshalType(desc.ElemDesc, valVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// marshalBitlist generates marshal code for SSZ bitlist types.
func (ctx *encoderContext) marshalBitlist(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	hasMax := false
	maxVar := ""

	if maxExpression != nil {
		exprVar := ctx.getExprVar(*maxExpression, uint64(desc.Limit))

		hasMax = true
		maxVar = fmt.Sprintf("int(%s)", exprVar)
	} else if desc.Limit > 0 {
		maxVar = fmt.Sprintf("%d", desc.Limit)
		hasMax = true
	} else {
		maxVar = "0"
	}

	ctx.appendCode(indent, "vlen := len(%s)\n", varName)

	if hasMax {
		ctx.appendCode(indent, "if vlen > %s {\n", maxVar)
		ctx.appendCode(indent, "\treturn sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	ctx.appendCode(indent, "bval := []byte(%s[:])\n", varName)
	ctx.appendCode(indent, "if vlen == 0 {\n")
	ctx.appendCode(indent, "\tbval = []byte{0x01}\n")
	ctx.appendCode(indent, "} else if bval[vlen-1] == 0x00 {\n")
	ctx.appendCode(indent, "\treturn sszutils.ErrBitlistNotTerminated\n")
	ctx.appendCode(indent, "}\n")

	ctx.appendCode(indent, "enc.EncodeBytes(bval)\n")

	return nil
}

// marshalUnion generates marshal code for SSZ union types.
func (ctx *encoderContext) marshalUnion(desc *dynssz.TypeDescriptor, varName string, indent int) error {
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
		ctx.appendCode(indent, "case %d:\n", variant)
		ctx.appendCode(indent, "\tv, ok := %s.Data.(%s)\n", varName, variantType)
		ctx.appendCode(indent, "\tif !ok {\n")
		ctx.appendCode(indent, "\t\treturn sszutils.ErrInvalidUnionVariant\n")
		ctx.appendCode(indent, "\t}\n")
		if err := ctx.marshalType(variantDesc, "v", indent+1, false); err != nil {
			return err
		}
	}
	ctx.appendCode(indent, "default:\n")
	ctx.appendCode(indent, "\treturn sszutils.ErrInvalidUnionVariant\n")
	ctx.appendCode(indent, "}\n")

	return nil
}
