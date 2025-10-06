// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
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
	appendCode      func(indent int, code string, args ...any)
	typePrinter     *TypePrinter
	options         *CodeGeneratorOptions
	usedDynSpecs    bool
	indexVarCounter int
	sizeVarCounter  int
	limitVarCounter int
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
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateSize(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) error {
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
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate size calculation code
	if err := ctx.sizeType(rootTypeDesc, "t", "size", 1, true); err != nil {
		return err
	}

	genDynamicFn := !options.WithoutDynamicExpressions
	genStaticFn := options.WithoutDynamicExpressions || options.CreateLegacyFn

	if genDynamicFn && !ctx.usedDynSpecs {
		genStaticFn = true
	}

	if genStaticFn {
		if !ctx.usedDynSpecs {
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

	if genDynamicFn {
		if ctx.usedDynSpecs {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) SizeSSZDyn(ds sszutils.DynamicSpecs) (size int) {\n", typeName))
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn size\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) SizeSSZDyn(_ sszutils.DynamicSpecs) (size int) {\n", typeName))
			codeBuilder.WriteString("\treturn t.SizeSSZ()\n")
			codeBuilder.WriteString("}\n\n")
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
func (ctx *sizeContext) getPtrPrefix(desc *dynssz.TypeDescriptor) string {
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
		return ""
	}
	if desc.Kind == reflect.Array {
		fieldSize := uint32(8)
		if desc.ElemDesc.GoTypeFlags&dynssz.GoTypeFlagIsPointer == 0 && desc.ElemDesc.Size > 0 {
			fieldSize = desc.ElemDesc.Size
		}
		if desc.Len*fieldSize > 32 {
			// big array with > 32 bytes
			return "&"
		}
	}
	if desc.Kind == reflect.Struct {
		// use pointer to struct to avoid copying
		return "&"
	}
	return ""
}

// sizeType generates size calculation code for any SSZ type, delegating to specific sizers.
func (ctx *sizeContext) sizeType(desc *dynssz.TypeDescriptor, varName string, sizeVar string, indent int, isRoot bool) error {
	// Handle types that have generated methods we can call
	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicSizer != 0 && !isRoot {
		ctx.appendCode(indent, "%s += %s.SizeSSZDyn(ds)\n", sizeVar, varName)
		ctx.usedDynSpecs = true
		return nil
	}

	useFastSsz := !ctx.options.NoFastSsz && desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	if !useFastSsz && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "%s += %s.SizeSSZ()\n", sizeVar, varName)
		return nil
	}

	// create temporary instance for nil pointers
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
		if len(varName) > 1 {
			ctx.appendCode(indent, "t := %s\n", varName)
			varName = "t"
		}
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	switch desc.SszType {
	case dynssz.SszBoolType:
		ctx.appendCode(indent, "%s += 1\n", sizeVar)

	case dynssz.SszUint8Type:
		ctx.appendCode(indent, "%s += 1\n", sizeVar)

	case dynssz.SszUint16Type:
		ctx.appendCode(indent, "%s += 2\n", sizeVar)

	case dynssz.SszUint32Type:
		ctx.appendCode(indent, "%s += 4\n", sizeVar)

	case dynssz.SszUint64Type:
		ctx.appendCode(indent, "%s += 8\n", sizeVar)

	case dynssz.SszTypeWrapperType:
		innerVarName := fmt.Sprintf("%s.Data", varName)
		if err := ctx.sizeType(desc.ElemDesc, innerVarName, sizeVar, indent, false); err != nil {
			return err
		}

	case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
		return ctx.sizeContainer(desc, varName, sizeVar, indent)

	case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
		return ctx.sizeVector(desc, varName, sizeVar, indent)

	case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
		return ctx.sizeList(desc, varName, sizeVar, indent)

	case dynssz.SszCompatibleUnionType:
		return ctx.sizeUnion(desc, varName, sizeVar, indent)

	case dynssz.SszCustomType:
		ctx.appendCode(indent, "// Custom type - size unknown\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// sizeContainer generates size calculation code for SSZ container (struct) types.
func (ctx *sizeContext) sizeContainer(desc *dynssz.TypeDescriptor, varName string, sizeVar string, indent int) error {
	// Fixed part size
	staticSize := 0
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			// Dynamic field - add offset size
			staticSize += 4
			ctx.appendCode(indent, "// Field #%d '%s' offset (4 bytes)\n", idx, field.Name)
		} else if field.Type.Size > 0 && (field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr == 0 || ctx.options.WithoutDynamicExpressions) {
			staticSize += int(field.Type.Size)
			ctx.appendCode(indent, "// Field #%d '%s' static (%d bytes)\n", idx, field.Name, field.Type.Size)
		}
	}

	ctx.appendCode(indent, "%s += %d\n", sizeVar, staticSize)

	// Add calculated size for static fields
	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 && (field.Type.Size == 0 || (field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions)) {
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
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
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
func (ctx *sizeContext) sizeVector(desc *dynssz.TypeDescriptor, varName string, sizeVar string, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	usedVar := false
	useVar := func() {
		if usedVar {
			return
		}
		ctx.appendCode(indent, "t := %s%s\n", ctx.getPtrPrefix(desc), varName)
		varName = "t"
		usedVar = true
	}

	limitVar := ctx.getLimitVar()
	if sizeExpression != nil {
		ctx.usedDynSpecs = true
		ctx.appendCode(indent, "hasLimit, %s, err := ds.ResolveSpecValue(\"%s\")\n", limitVar, *sizeExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn 0\n")
		ctx.appendCode(indent, "}\n")
		ctx.appendCode(indent, "if !hasLimit {\n")
		ctx.appendCode(indent, "\t%s = %d\n", limitVar, desc.Len)
		ctx.appendCode(indent, "}\n")
	} else {
		ctx.appendCode(indent, "%s := %d\n", limitVar, desc.Len)
	}

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
			useVar()
			ctx.appendCode(indent, "if len(%s) > 0 {\n", varName)
			innerVarName := fmt.Sprintf("%s[0]", varName)
			innerSizeVar := ctx.getSizeVar()
			ctx.appendCode(indent, "\t%s := 0\n", innerSizeVar)
			if err := ctx.sizeType(desc.ElemDesc, innerVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t%s += %s * %s\n", sizeVar, innerSizeVar, limitVar)
			ctx.appendCode(indent, "}\n")
		} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 || desc.ElemDesc.Size == 1 {
			// For byte arrays, size is just the vector length
			ctx.appendCode(indent, "%s += int(%s)\n", sizeVar, limitVar)
		} else {
			// Fixed size elements - simple multiplication
			ctx.appendCode(indent, "%s += int(%s) * %d\n", sizeVar, limitVar, desc.ElemDesc.Size)
		}
	} else {
		// Dynamic size elements - need to iterate
		ctx.appendCode(indent, "%s += int(%s) * 4\n", sizeVar, limitVar)

		if desc.Kind == reflect.Array {
			indexVar := ctx.getIndexVar()
			ctx.appendCode(indent, "for %s := 0; %s < int(%s); %s++ {\n", indexVar, indexVar, limitVar, indexVar)
			itemVarName := fmt.Sprintf("%s[%s]", varName, indexVar)
			if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		} else {
			useVar()
			ctx.appendCode(indent, "vlen := len(%s)\n", varName)
			ctx.appendCode(indent, "if vlen > int(%s) {\n", limitVar)
			ctx.appendCode(indent, "\tvlen = int(%s)\n", limitVar)
			ctx.appendCode(indent, "}\n")
			indexVar := ctx.getIndexVar()
			ctx.appendCode(indent, "for %s := 0; %s < vlen; %s++ {\n", indexVar, indexVar, indexVar)
			itemVarName := fmt.Sprintf("%s[%s]", varName, indexVar)
			if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")

			// Add size for zero-padding
			ctx.appendCode(indent, "if vlen < int(%s) {\n", limitVar)
			typeName := ctx.typePrinter.TypeString(desc.ElemDesc)
			ctx.appendCode(indent, "\tzeroItem := &%s{}\n", typeName)
			innerSizeVar := ctx.getSizeVar()
			ctx.appendCode(indent, "\t%s := 0\n", innerSizeVar)
			if err := ctx.sizeType(desc.ElemDesc, "zeroItem", innerSizeVar, indent+2, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t%s += %s * (vlen - int(%s))\n", sizeVar, innerSizeVar, limitVar)
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// sizeList generates size calculation code for SSZ list (variable-size array) types.
func (ctx *sizeContext) sizeList(desc *dynssz.TypeDescriptor, varName string, sizeVar string, indent int) error {
	// For byte slices, size is just the length
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		ctx.appendCode(indent, "%s += len(%s)\n", sizeVar, varName)
		return nil
	}

	usedVar := false
	useVar := func() {
		if usedVar {
			return
		}
		ctx.appendCode(indent, "t := %s%s\n", ctx.getPtrPrefix(desc), varName)
		varName = "t"
		usedVar = true
	}

	// Handle lists with dynamic elements
	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
		useVar()
		ctx.appendCode(indent, "vlen := len(%s)\n", varName)

		// Add offset space
		ctx.appendCode(indent, "%s += vlen * 4 // Offsets\n", sizeVar)

		// Add size of each element
		indexVar := ctx.getIndexVar()
		ctx.appendCode(indent, "for %s := 0; %s < vlen; %s++ {\n", indexVar, indexVar, indexVar)
		itemVarName := fmt.Sprintf("%s[%s]", varName, indexVar)
		if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// Fixed size elements
		if desc.ElemDesc.Size > 0 && (desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr == 0 || ctx.options.WithoutDynamicExpressions) {
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
func (ctx *sizeContext) sizeUnion(desc *dynssz.TypeDescriptor, varName string, sizeVar string, indent int) error {
	ctx.appendCode(indent, "t := %s\n", varName)
	varName = "t"

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
		hasDynamicSize := variantDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0
		hasSizeExpression := variantDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0
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
