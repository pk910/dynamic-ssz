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

// sizeContext contains the state and utilities for generating size calculation methods.
//
// This context structure maintains the necessary state during the size method code generation
// process, including code building utilities, type formatting, and options that control
// the generation behavior.
//
// Fields:
//   - appendCode: Function to append formatted code with proper indentation
//   - typePrinter: Type name formatter and import tracker
//   - options: Code generation options controlling output behavior
//   - usedDynSpecs: Flag tracking whether generated code uses dynamic SSZ functionality
type sizeContext struct {
	codeBuf         *strings.Builder
	appendCode      func(indent int, code string, args ...any)
	typePrinter     *TypePrinter
	options         *CodeGeneratorOptions
	exprVars        *exprVarGenerator
	usedDynSpecs    bool
	indexVarCounter int
	sizeVarCounter  int
	limitVarCounter int

	useTypeFnMap map[*ssztypes.TypeDescriptor]*sizeFnPtr
}

func newSizeContext(typePrinter *TypePrinter, options *CodeGeneratorOptions) *sizeContext {
	codeBuf := strings.Builder{}
	var ctx *sizeContext
	ctx = &sizeContext{
		codeBuf: &codeBuf,
		appendCode: func(indent int, code string, args ...any) {
			appendCode(ctx.codeBuf, indent, code, args...)
		},
		typePrinter:  typePrinter,
		options:      options,
		exprVars:     newExprVarGenerator("expr", typePrinter, options),
		useTypeFnMap: make(map[*ssztypes.TypeDescriptor]*sizeFnPtr),
	}
	ctx.exprVars.retVars = "0"
	return ctx
}

type sizeFnPtr struct {
	fnName       string
	fnArgs       []string
	needDynSpecs bool
	used         bool
}

func (s *sizeFnPtr) getFnCall(varName string) string {
	args := make([]string, 0, 1+len(s.fnArgs))
	args = append(args, s.fnArgs...)
	args = append(args, varName)
	s.used = true
	return fmt.Sprintf("%s(%s)", s.fnName, strings.Join(args, ", "))
}

// generateSize generates size calculation methods for a specific type.
//
// This function creates the complete set of size calculation methods for a type, including:
//   - SizeSSZDyn for dynamic specification support with runtime size calculation
//   - SizeSSZ for static/legacy compatibility with compile-time known sizes
//
// The generated methods calculate the exact SSZ encoding size for a type instance,
// handling variable-length fields, dynamic expressions, and nested types. Size
// calculation is essential for efficient buffer allocation during marshaling.
//
// Parameters:
//   - rootTypeDesc: Type descriptor containing complete SSZ size calculation metadata
//   - codeBuilder: String builder to append generated method code to
//   - typePrinter: Type formatter for handling imports and type names
//   - viewName: Name of the view type for function name postfix (empty string for data type)
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateSize(rootTypeDesc *ssztypes.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, viewName string, options *CodeGeneratorOptions) error {
	ctx := newSizeContext(typePrinter, options)

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate size calculation code
	if err := ctx.sizeType(rootTypeDesc, "t", "size", 0, true); err != nil {
		return err
	}

	if ctx.exprVars.varCounter > 0 {
		ctx.usedDynSpecs = true
	}

	genDynamicFn := !options.WithoutDynamicExpressions || viewName != ""
	genStaticFn := (options.WithoutDynamicExpressions || options.CreateLegacyFn) && viewName == ""

	if genDynamicFn && !ctx.usedDynSpecs && viewName == "" {
		genStaticFn = true
	}

	if genStaticFn {
		if !ctx.usedDynSpecs {
			appendCode(codeBuilder, 0, "func (t %s) SizeSSZ() (size int) {\n", typeName)
			if rootTypeDesc.Size > 0 {
				appendCode(codeBuilder, 1, "return %d\n", rootTypeDesc.Size)
			} else {
				appendCode(codeBuilder, 1, ctx.exprVars.getCode())
				appendCode(codeBuilder, 1, ctx.codeBuf.String())
				appendCode(codeBuilder, 1, "return size\n")
			}
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			appendCode(codeBuilder, 0, "func (t %s) SizeSSZ() (size int) {\n", typeName)
			appendCode(codeBuilder, 1, "return t.SizeSSZDyn(%s.GetGlobalDynSsz())\n", dynsszAlias)
			appendCode(codeBuilder, 0, "}\n\n")
		}
	}

	if genDynamicFn {
		fnName := "SizeSSZDyn"
		if viewName != "" {
			fnName = fmt.Sprintf("sizeSSZView_%s", viewName)
		}
		if ctx.usedDynSpecs || viewName != "" {
			appendCode(codeBuilder, 0, "func (t %s) %s(ds sszutils.DynamicSpecs) (size int) {\n", typeName, fnName)
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, ctx.codeBuf.String())
			appendCode(codeBuilder, 1, "return size\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			appendCode(codeBuilder, 0, "func (t %s) %s(_ sszutils.DynamicSpecs) (size int) {\n", typeName, fnName)
			appendCode(codeBuilder, 1, "return t.SizeSSZ()\n")
			appendCode(codeBuilder, 0, "}\n\n")
			genStaticFn = true
		}
	}

	return nil
}

func (ctx *sizeContext) getIndexVar() string {
	ctx.indexVarCounter++
	return fmt.Sprintf("i%d", ctx.indexVarCounter)
}

func (ctx *sizeContext) getSizeVar() string {
	ctx.sizeVarCounter++
	return fmt.Sprintf("s%d", ctx.sizeVarCounter)
}

func (ctx *sizeContext) getLimitVar() string {
	ctx.limitVarCounter++
	return fmt.Sprintf("l%d", ctx.limitVarCounter)
}

// getPtrPrefix returns & for types that are heavy to copy
func (ctx *sizeContext) getPtrPrefix(desc *ssztypes.TypeDescriptor) string {
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
func (ctx *sizeContext) getValueVar(desc *ssztypes.TypeDescriptor, varName string, targetType string) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
		varName = fmt.Sprintf("*%s", varName)
	}

	if targetType != "" && ctx.typePrinter.InnerTypeString(desc) != targetType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// sizeType generates size calculation code for any SSZ type, delegating to specific sizers.
func (ctx *sizeContext) sizeType(desc *ssztypes.TypeDescriptor, varName string, sizeVar string, indent int, isRoot bool) error {
	// Handle types that have generated methods we can call
	if ptr, ok := ctx.useTypeFnMap[desc]; ok {
		ctx.appendCode(indent, "%s += %s\n", sizeVar, ptr.getFnCall(varName))
		if ptr.needDynSpecs {
			ctx.usedDynSpecs = true
		}
		return nil
	}

	isView := desc.GoTypeFlags&ssztypes.GoTypeFlagIsView != 0
	if !isRoot && isView {
		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewSizer != 0 {
			ctx.appendCode(indent, "if viewFn := %s.SizeSSZDynView((%s)(nil)); viewFn != nil {\n", varName, ctx.typePrinter.ViewTypeString(desc, true))
			ctx.appendCode(indent+1, "%s += viewFn(ds)\n", sizeVar)
			ctx.appendCode(indent, "}\n")
			ctx.usedDynSpecs = true
			return nil
		}
	}

	if !isRoot && !isView {
		hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
		useFastSsz := !ctx.options.NoFastSsz && desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 && !hasDynamicSize
		if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
			useFastSsz = true
		}

		if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicSizer != 0 {
			ctx.appendCode(indent, "%s += %s.SizeSSZDyn(ds)\n", sizeVar, varName)
			ctx.usedDynSpecs = true
			return nil
		}

		if useFastSsz {
			ctx.appendCode(indent, "%s += %s.SizeSSZ()\n", sizeVar, varName)
			return nil
		}
	}

	// create temporary instance for nil pointers
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		if len(varName) > 1 {
			ctx.appendCode(indent, "t := %s\n", varName)
			varName = "t"
		}
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	switch desc.SszType {
	case ssztypes.SszBoolType:
		ctx.appendCode(indent, "%s += 1\n", sizeVar)

	case ssztypes.SszUint8Type:
		ctx.appendCode(indent, "%s += 1\n", sizeVar)

	case ssztypes.SszUint16Type:
		ctx.appendCode(indent, "%s += 2\n", sizeVar)

	case ssztypes.SszUint32Type:
		ctx.appendCode(indent, "%s += 4\n", sizeVar)

	case ssztypes.SszUint64Type:
		ctx.appendCode(indent, "%s += 8\n", sizeVar)

	case ssztypes.SszTypeWrapperType:
		innerVarName := fmt.Sprintf("%s.Data", varName)
		if err := ctx.sizeType(desc.ElemDesc, innerVarName, sizeVar, indent, false); err != nil {
			return err
		}

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return ctx.sizeContainer(desc, varName, sizeVar, indent)

	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return ctx.sizeVector(desc, varName, sizeVar, indent)

	case ssztypes.SszListType, ssztypes.SszBitlistType, ssztypes.SszProgressiveListType, ssztypes.SszProgressiveBitlistType:
		return ctx.sizeList(desc, varName, sizeVar, indent)

	case ssztypes.SszCompatibleUnionType:
		return ctx.sizeUnion(desc, varName, sizeVar, indent)

	case ssztypes.SszCustomType:
		ctx.appendCode(indent, "// Custom type - size unknown\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// sizeContainer generates size calculation code for SSZ container (struct) types.
func (ctx *sizeContext) sizeContainer(desc *ssztypes.TypeDescriptor, varName string, sizeVar string, indent int) error {
	// Fixed part size
	staticSize := 0
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			// Dynamic field - add offset size
			staticSize += 4
			ctx.appendCode(indent, "// Field #%d '%s' offset (4 bytes)\n", idx, field.Name)
		} else if field.Type.Size > 0 && (field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr == 0 || ctx.options.WithoutDynamicExpressions) {
			staticSize += int(field.Type.Size)
			ctx.appendCode(indent, "// Field #%d '%s' static (%d bytes)\n", idx, field.Name, field.Type.Size)
		}
	}

	if staticSize > 0 {
		ctx.appendCode(indent, "%s += %d\n", sizeVar, staticSize)
	}

	// Add calculated size for static fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 && (field.Type.Size == 0 || (field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions)) {
			// Need to calculate size
			ctx.appendCode(indent, "{ // Field #%d '%s'\n", idx, field.Name)
			fieldVarName := fmt.Sprintf("%s.%s", varName, field.Name)
			if err := ctx.sizeType(field.Type, fieldVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	// Dynamic part size
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			ctx.appendCode(indent, "{ // Dynamic field #%d '%s'\n", idx, field.Name)
			fieldVarName := fmt.Sprintf("%s.%s", varName, field.Name)
			if err := ctx.sizeType(field.Type, fieldVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// sizeVector generates size calculation code for SSZ vector (fixed-size array) types.
func (ctx *sizeContext) sizeVector(desc *ssztypes.TypeDescriptor, varName string, sizeVar string, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	usedVar := false
	useVar := func() {
		if usedVar {
			return
		}
		if len(varName) > 1 {
			ctx.appendCode(indent, "t := %s%s\n", ctx.getPtrPrefix(desc), varName)
			varName = "t"
		}
		usedVar = true
	}

	limitVar := ctx.getLimitVar()
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
			limitVar = fmt.Sprintf("int((%s+7)/8)", exprVar)
		} else {
			limitVar = fmt.Sprintf("int(%s)", exprVar)
		}
	} else {
		ctx.appendCode(indent, "%s := %d\n", limitVar, desc.Len)
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
			useVar()
			ctx.appendCode(indent, "if len(%s) > 0 {\n", varName)
			innerVarName := fmt.Sprintf("%s[0]", varName)
			innerSizeVar := ctx.getSizeVar()
			ctx.appendCode(indent, "\t%s := 0\n", innerSizeVar)
			if err := ctx.sizeType(desc.ElemDesc, innerVarName, innerSizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t%s += %s * %s\n", sizeVar, innerSizeVar, limitVar)
			ctx.appendCode(indent, "}\n")
		} else if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 || desc.ElemDesc.Size == 1 {
			// For byte arrays, size is just the vector length
			ctx.appendCode(indent, "%s += %s\n", sizeVar, limitVar)
		} else {
			// Fixed size elements - simple multiplication
			ctx.appendCode(indent, "%s += %s * %d\n", sizeVar, limitVar, desc.ElemDesc.Size)
		}
	} else {
		// Dynamic size elements - need to iterate
		ctx.appendCode(indent, "%s += %s * 4\n", sizeVar, limitVar)

		if desc.Kind == reflect.Array {
			indexVar := ctx.getIndexVar()
			ctx.appendCode(indent, "for %s := range %s {\n", indexVar, limitVar)
			itemVarName := fmt.Sprintf("%s[%s]", varName, indexVar)
			if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		} else {
			useVar()
			ctx.appendCode(indent, "vlen := len(%s)\n", varName)
			ctx.appendCode(indent, "if vlen > %s {\n", limitVar)
			ctx.appendCode(indent, "\tvlen = %s\n", limitVar)
			ctx.appendCode(indent, "}\n")
			indexVar := ctx.getIndexVar()
			ctx.appendCode(indent, "for %s := range vlen {\n", indexVar)
			itemVarName := fmt.Sprintf("%s[%s]", varName, indexVar)
			if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")

			// Add size for zero-padding
			ctx.appendCode(indent, "if vlen < %s {\n", limitVar)
			typeName := ctx.typePrinter.InnerTypeString(desc.ElemDesc)
			ctx.appendCode(indent, "\tzeroItem := &%s{}\n", typeName)
			innerSizeVar := ctx.getSizeVar()
			ctx.appendCode(indent, "\t%s := 0\n", innerSizeVar)
			if err := ctx.sizeType(desc.ElemDesc, "zeroItem", innerSizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t%s += %s * (%s - vlen)\n", sizeVar, innerSizeVar, limitVar)
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// sizeList generates size calculation code for SSZ list (variable-size array) types.
func (ctx *sizeContext) sizeList(desc *ssztypes.TypeDescriptor, varName string, sizeVar string, indent int) error {
	// For byte slices, size is just the length
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
		ctx.appendCode(indent, "%s += len(%s)\n", sizeVar, ctx.getValueVar(desc, varName, ""))
		return nil
	}

	usedVar := false
	useVar := func() {
		if usedVar {
			return
		}
		if len(varName) > 1 {
			ctx.appendCode(indent, "t := %s%s\n", ctx.getPtrPrefix(desc), varName)
			varName = "t"
		}
		usedVar = true
	}

	// Handle lists with dynamic elements
	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
		useVar()
		ctx.appendCode(indent, "vlen := len(%s)\n", varName)

		// Add offset space
		ctx.appendCode(indent, "%s += vlen * 4 // Offsets\n", sizeVar)

		// Add size of each element
		indexVar := ctx.getIndexVar()
		ctx.appendCode(indent, "for %s := range vlen {\n", indexVar)
		itemVarName := fmt.Sprintf("%s[%s]", varName, indexVar)
		if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// Fixed size elements
		if desc.ElemDesc.Size > 0 && (desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr == 0 || ctx.options.WithoutDynamicExpressions) {
			if desc.ElemDesc.Size == 1 {
				ctx.appendCode(indent, "%s += len(%s)\n", sizeVar, varName)
			} else {
				ctx.appendCode(indent, "%s += len(%s) * %d\n", sizeVar, varName, desc.ElemDesc.Size)
			}
		} else {
			useVar()
			ctx.appendCode(indent, "vlen := len(%s)\n", varName)
			ctx.appendCode(indent, "if vlen > 0 {\n")
			innerSizeVar := ctx.getSizeVar()
			ctx.appendCode(indent, "\t%s := 0\n", innerSizeVar)
			itemVarName := fmt.Sprintf("%s[0]", varName)
			if err := ctx.sizeType(desc.ElemDesc, itemVarName, innerSizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t%s += %s * vlen\n", sizeVar, innerSizeVar)
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// sizeUnion generates size calculation code for SSZ union types.
func (ctx *sizeContext) sizeUnion(desc *ssztypes.TypeDescriptor, varName string, sizeVar string, indent int) error {
	if len(varName) > 1 {
		ctx.appendCode(indent, "t := %s\n", varName)
		varName = "t"
	}

	ctx.appendCode(indent, "%s += 1 // Union selector\n", sizeVar)
	ctx.appendCode(indent, "switch %s.Variant {\n", varName)

	variants := make([]int, 0, len(desc.UnionVariants))
	for variant := range desc.UnionVariants {
		variants = append(variants, int(variant))
	}
	slices.Sort(variants)

	for _, variant := range variants {
		variantDesc := desc.UnionVariants[uint8(variant)]
		variantType := ctx.typePrinter.TypeString(variantDesc)
		hasDynamicSize := variantDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0
		hasSizeExpression := variantDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0
		ctx.appendCode(indent, "case %d:\n", variant)
		if variantDesc.Size == 0 || hasDynamicSize || hasSizeExpression {
			ctx.appendCode(indent, "\tv, ok := %s.Data.(%s)\n", varName, variantType)
			ctx.appendCode(indent, "\tif !ok {\n")
			ctx.appendCode(indent, "\t\treturn 0\n")
			ctx.appendCode(indent, "\t}\n")
			if err := ctx.sizeType(variantDesc, "v", sizeVar, indent+1, false); err != nil {
				return err
			}
		} else {
			ctx.appendCode(indent, "\t%s += %d\n", sizeVar, variantDesc.Size)
		}
	}

	ctx.appendCode(indent, "}\n")

	return nil
}
