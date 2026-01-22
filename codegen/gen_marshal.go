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

// marshalContext contains the state and utilities for generating marshal methods.
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
type marshalContext struct {
	appendCode   func(indent int, code string, args ...any)
	typePrinter  *TypePrinter
	options      *CodeGeneratorOptions
	usedDynSpecs bool
	exprVars     *exprVarGenerator
}

// generateMarshal generates marshal methods for a specific type.
//
// This function creates the complete set of marshal methods for a type, including:
//   - MarshalSSZDyn for dynamic specification support
//   - MarshalSSZTo for static/legacy compatibility
//   - MarshalSSZ for legacy fastssz compatibility (if requested)
//
// The generated methods handle SSZ encoding according to the type's descriptor,
// supporting both static and dynamic sizing, nested types, and performance
// optimizations through fastssz delegation where appropriate.
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
func generateMarshal(rootTypeDesc *ssztypes.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, viewName string, options *CodeGeneratorOptions) error {
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
		exprVars:    newExprVarGenerator("expr", typePrinter, options),
	}

	ctx.exprVars.retVars = "dst, err"

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate marshaling code
	if err := ctx.marshalType(rootTypeDesc, "t", 0, true); err != nil {
		return err
	}

	if ctx.exprVars.varCounter > 0 {
		ctx.usedDynSpecs = true
	}

	genDynamicFn := !options.WithoutDynamicExpressions || viewName != ""
	genStaticFn := (options.WithoutDynamicExpressions || options.CreateLegacyFn) && viewName == ""
	genLegacyFn := options.CreateLegacyFn && viewName == ""

	if genDynamicFn && !ctx.usedDynSpecs && viewName == "" {
		genStaticFn = true
	}

	if genLegacyFn {
		dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
		appendCode(codeBuilder, 0, "func (t %s) MarshalSSZ() ([]byte, error) {\n", typeName)
		appendCode(codeBuilder, 1, "return %s.GetGlobalDynSsz().MarshalSSZ(t)\n", dynsszAlias)
		appendCode(codeBuilder, 0, "}\n")
	}

	if genStaticFn {
		if !ctx.usedDynSpecs {
			appendCode(codeBuilder, 0, "func (t %s) MarshalSSZTo(buf []byte) (dst []byte, err error) {\n", typeName)
			appendCode(codeBuilder, 1, "dst = buf\n")
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, codeBuf.String())
			appendCode(codeBuilder, 1, "return dst, nil\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			appendCode(codeBuilder, 0, "func (t %s) MarshalSSZTo(buf []byte) (dst []byte, err error) {\n", typeName)
			appendCode(codeBuilder, 1, "return t.MarshalSSZDyn(%s.GetGlobalDynSsz(), buf)\n", dynsszAlias)
			appendCode(codeBuilder, 0, "}\n\n")
		}
	}

	if genDynamicFn {
		fnName := "MarshalSSZDyn"
		if viewName != "" {
			fnName = fmt.Sprintf("marshalSSZView_%s", viewName)
		}
		if ctx.usedDynSpecs || viewName != "" {
			appendCode(codeBuilder, 0, "func (t %s) %s(ds sszutils.DynamicSpecs, buf []byte) (dst []byte, err error) {\n", typeName, fnName)
			appendCode(codeBuilder, 1, "dst = buf\n")
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, codeBuf.String())
			appendCode(codeBuilder, 1, "return dst, nil\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			appendCode(codeBuilder, 0, "func (t %s) %s(_ sszutils.DynamicSpecs, buf []byte) (dst []byte, err error) {\n", typeName, fnName)
			appendCode(codeBuilder, 1, "return t.MarshalSSZTo(buf)\n")
			appendCode(codeBuilder, 0, "}\n\n")
			genStaticFn = true
		}
	}

	return nil
}

// getPtrPrefix returns & for types that are heavy to copy
func (ctx *marshalContext) getPtrPrefix(desc *ssztypes.TypeDescriptor) string {
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
func (ctx *marshalContext) getValueVar(desc *ssztypes.TypeDescriptor, varName string, targetType string) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
		varName = fmt.Sprintf("*%s", varName)
	}

	if targetType != "" && ctx.typePrinter.InnerTypeString(desc) != targetType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// isInlineable checks if a type can be inlined directly into the hash tree root code
func (ctx *marshalContext) isInlineable(desc *ssztypes.TypeDescriptor) bool {
	if desc.SszType == ssztypes.SszBoolType || desc.SszType == ssztypes.SszUint8Type || desc.SszType == ssztypes.SszUint16Type || desc.SszType == ssztypes.SszUint32Type || desc.SszType == ssztypes.SszUint64Type {
		return true
	}

	if desc.SszType == ssztypes.SszVectorType || desc.SszType == ssztypes.SszListType {
		return desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0
	}

	return false
}

// marshalType generates marshal code for any SSZ type, delegating to specific marshalers.
func (ctx *marshalContext) marshalType(desc *ssztypes.TypeDescriptor, varName string, indent int, isRoot bool) error {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	// Handle types that have generated methods we can call
	hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszMarshaler := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "if dst, err = %s.MarshalSSZTo(dst); err != nil {\n\treturn nil, err\n}\n", varName)
		return nil
	}

	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicMarshaler != 0 && !isRoot {
		ctx.appendCode(indent, "if dst, err = %s.MarshalSSZDyn(ds, dst); err != nil {\n\treturn nil, err\n}\n", varName)
		ctx.usedDynSpecs = true
		return nil
	}

	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicEncoder != 0 && !isRoot {
		ctx.appendCode(indent, "enc := sszutils.NewBufferEncoder(dst)\n")
		ctx.appendCode(indent, "if err = %s.MarshalSSZEncoder(ds, enc); err != nil {\n\treturn nil, err\n}\n", varName)
		ctx.appendCode(indent, "dst = enc.GetBuffer()\n")
		ctx.usedDynSpecs = true
		return nil
	}

	switch desc.SszType {
	case ssztypes.SszBoolType:
		ctx.appendCode(indent,
			"dst = sszutils.MarshalBool(dst, %s)\n",
			ctx.getValueVar(desc, varName, "bool"),
		)
	case ssztypes.SszUint8Type:
		ctx.appendCode(indent,
			"dst = append(dst, %s)\n",
			ctx.getValueVar(desc, varName, "byte"),
		)
	case ssztypes.SszUint16Type:
		ctx.appendCode(indent,
			"dst = %s.LittleEndian.AppendUint16(dst, %s)\n",
			ctx.typePrinter.AddImport("encoding/binary", "binary"),
			ctx.getValueVar(desc, varName, "uint16"),
		)
	case ssztypes.SszUint32Type:
		ctx.appendCode(indent,
			"dst = %s.LittleEndian.AppendUint32(dst, %s)\n",
			ctx.typePrinter.AddImport("encoding/binary", "binary"),
			ctx.getValueVar(desc, varName, "uint32"),
		)
	case ssztypes.SszUint64Type:
		valueVar := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			valueVar = fmt.Sprintf("%s.Unix()", varName)
		}
		ctx.appendCode(indent,
			"dst = %s.LittleEndian.AppendUint64(dst, %s)\n",
			ctx.typePrinter.AddImport("encoding/binary", "binary"),
			ctx.getValueVar(desc, valueVar, "uint64"),
		)

	case ssztypes.SszTypeWrapperType:
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

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return ctx.marshalContainer(desc, varName, indent)

	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return ctx.marshalVector(desc, varName, indent)

	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		return ctx.marshalList(desc, varName, indent)

	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		return ctx.marshalBitlist(desc, varName, indent)

	case ssztypes.SszCompatibleUnionType:
		return ctx.marshalUnion(desc, varName, indent)

	case ssztypes.SszCustomType:
		ctx.appendCode(indent, "return nil, sszutils.ErrNotImplemented\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// marshalContainer generates marshal code for SSZ container (struct) types.
func (ctx *marshalContext) marshalContainer(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	hasDynamic := false
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			hasDynamic = true
			break
		}
	}

	if hasDynamic {
		ctx.appendCode(indent, "dstlen := len(dst)\n")
	}

	// Write offsets for dynamic fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			ctx.appendCode(indent, "// Offset #%d '%s'\n", idx, field.Name)
			ctx.appendCode(indent, "offset%d := len(dst)\n", idx)
			ctx.appendCode(indent, "dst = append(dst, 0, 0, 0, 0)\n")
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
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			ctx.appendCode(indent, "{ // Dynamic Field #%d '%s'\n", idx, field.Name)
			binaryPkgName := ctx.typePrinter.AddImport("encoding/binary", "binary")
			ctx.appendCode(indent, "\t%s.LittleEndian.PutUint32(dst[offset%d:], uint32(len(dst)-dstlen))\n", binaryPkgName, idx)
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
func (ctx *marshalContext) marshalVector(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
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
			ctx.appendCode(indent, "\treturn nil, sszutils.ErrVectorLength\n")
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
	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "vlen := len(%s)\n", valueVar)
		ctx.appendCode(indent, "if vlen > %s {\n", limitVar)
		ctx.appendCode(indent, "\treturn nil, sszutils.ErrVectorLength\n")
		ctx.appendCode(indent, "}\n")
		lenVar = "vlen"
	} else if hasLimitVar {
		ctx.appendCode(indent, "vlen := %d\n", desc.Len)
		ctx.appendCode(indent, "if vlen > %s {\n\tvlen = %s\n}\n", limitVar, limitVar)
		lenVar = "vlen"
	} else {
		lenVar = fmt.Sprintf("%d", desc.Len)
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 || desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
			if strings.HasPrefix(valueVar, "*") {
				valueVar = fmt.Sprintf("(%s)", valueVar)
			}
			if bitlimitVar != "" {
				ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
				ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", valueVar, lenVar)
				ctx.appendCode(indent, "\treturn nil, sszutils.ErrVectorLength\n")
				ctx.appendCode(indent, "}\n")
			}
			ctx.appendCode(indent, "dst = append(dst, %s[:%s]...)\n", valueVar, lenVar)
		} else {
			ctx.appendCode(indent, "for i := range %s {\n", lenVar)
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
			ctx.appendCode(indent, "\tdst = sszutils.AppendZeroPadding(dst, (%s - %s) * %d)\n", limitVar, lenVar, desc.ElemDesc.Size)
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements
		// reserve space for offsets
		ctx.appendCode(indent, "dstlen := len(dst)\n")
		ctx.appendCode(indent, "dst = sszutils.AppendZeroPadding(dst, %s*4)\n", limitVar)
		ctx.appendCode(indent, "for i := range %s {\n", lenVar)
		binaryPkgName := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent, "\t%s.LittleEndian.PutUint32(dst[dstlen+(i*4):], uint32(len(dst)-dstlen))\n", binaryPkgName)
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
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent, "\tzeroItem := new(%s)\n", ctx.typePrinter.InnerTypeString(desc.ElemDesc))
			} else {
				ctx.appendCode(indent, "\tvar zeroItem %s\n", ctx.typePrinter.TypeString(desc.ElemDesc))
			}
			ctx.appendCode(indent, "\tfor i := %s; i < %s; i++ {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\t\t%s.LittleEndian.PutUint32(dst[dstlen+(i*4):], uint32(len(dst)-dstlen))\n", binaryPkgName)
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
func (ctx *marshalContext) marshalList(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	hasMax := false
	maxVar := ""

	if maxExpression != nil {
		exprVar := ctx.exprVars.getExprVar(*maxExpression, uint64(desc.Limit))

		hasMax = true
		maxVar = fmt.Sprintf("int(%s)", exprVar)
	} else if desc.Limit > 0 {
		maxVar = fmt.Sprintf("%d", desc.Limit)
		hasMax = true
	} else {
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
		ctx.appendCode(indent, "\treturn nil, sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
			if strings.HasPrefix(valueVar, "*") {
				valueVar = fmt.Sprintf("(%s)", valueVar)
			}
			ctx.appendCode(indent, "dst = append(dst, %s[:]...)\n", valueVar)
		} else {
			addVlen()
			ctx.appendCode(indent, "for i := range vlen {\n")
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
		ctx.appendCode(indent, "dstlen := len(dst)\n")
		addVlen()
		ctx.appendCode(indent, "dst = sszutils.AppendZeroPadding(dst, vlen*4)\n")
		ctx.appendCode(indent, "for i := range vlen {\n")
		binaryPkgName := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent, "\t%s.LittleEndian.PutUint32(dst[dstlen+(i*4):], uint32(len(dst)-dstlen))\n", binaryPkgName)
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
func (ctx *marshalContext) marshalBitlist(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	hasMax := false
	maxVar := ""

	if maxExpression != nil {
		exprVar := ctx.exprVars.getExprVar(*maxExpression, uint64(desc.Limit))

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
		ctx.appendCode(indent, "\treturn nil, sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	ctx.appendCode(indent, "bval := []byte(%s[:])\n", varName)
	ctx.appendCode(indent, "if vlen == 0 {\n")
	ctx.appendCode(indent, "\tbval = []byte{0x01}\n")
	ctx.appendCode(indent, "} else if bval[vlen-1] == 0x00 {\n")
	ctx.appendCode(indent, "\treturn nil, sszutils.ErrBitlistNotTerminated\n")
	ctx.appendCode(indent, "}\n")

	ctx.appendCode(indent, "dst = append(dst, bval...)\n")

	return nil
}

// marshalUnion generates marshal code for SSZ union types.
func (ctx *marshalContext) marshalUnion(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	ctx.appendCode(indent, "dst = append(dst, %s.Variant)\n", varName)
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
		ctx.appendCode(indent, "\t\treturn nil, sszutils.ErrInvalidUnionVariant\n")
		ctx.appendCode(indent, "\t}\n")
		if err := ctx.marshalType(variantDesc, "v", indent+1, false); err != nil {
			return err
		}
	}
	ctx.appendCode(indent, "default:\n")
	ctx.appendCode(indent, "\treturn nil, sszutils.ErrInvalidUnionVariant\n")
	ctx.appendCode(indent, "}\n")

	return nil
}
