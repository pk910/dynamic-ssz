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
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateMarshal(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) error {
	codeBuf := strings.Builder{}
	ctx := &marshalContext{
		appendCode: func(indent int, code string, args ...any) {
			appendCode(&codeBuf, indent, code, args...)
		},
		typePrinter: typePrinter,
		options:     options,
	}

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate marshaling code
	if err := ctx.marshalType(rootTypeDesc, "t", 1, true); err != nil {
		return err
	}

	genDynamicFn := !options.WithoutDynamicExpressions
	genStaticFn := options.WithoutDynamicExpressions || options.CreateLegacyFn
	genLegacyFn := options.CreateLegacyFn

	if genDynamicFn && !ctx.usedDynSpecs {
		genStaticFn = true
	}

	if genLegacyFn {
		dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
		codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZ() ([]byte, error) {\n", typeName))
		codeBuilder.WriteString(fmt.Sprintf("\treturn %s.GetGlobalDynSsz().MarshalSSZ(t)\n", dynsszAlias))
		codeBuilder.WriteString("}\n")
	}

	if genStaticFn {
		if !ctx.usedDynSpecs {
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

	if genDynamicFn {
		if ctx.usedDynSpecs {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) (dst []byte, err error) {\n", typeName))
			codeBuilder.WriteString("\tdst = buf\n")
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn dst, nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) (dst []byte, err error) {\n", typeName))
			codeBuilder.WriteString("\treturn t.MarshalSSZTo(buf)\n")
			codeBuilder.WriteString("}\n\n")
			genStaticFn = true
		}
	}

	return nil
}

// getPtrPrefix returns & for types that are heavy to copy
func (ctx *marshalContext) getPtrPrefix(desc *dynssz.TypeDescriptor) string {
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
func (ctx *marshalContext) isInlineable(desc *dynssz.TypeDescriptor) bool {
	if desc.SszType == dynssz.SszBoolType || desc.SszType == dynssz.SszUint8Type || desc.SszType == dynssz.SszUint16Type || desc.SszType == dynssz.SszUint32Type || desc.SszType == dynssz.SszUint64Type {
		return true
	}

	if desc.SszType == dynssz.SszVectorType || desc.SszType == dynssz.SszListType {
		return desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0
	}

	return false
}

// marshalType generates marshal code for any SSZ type, delegating to specific marshalers.
func (ctx *marshalContext) marshalType(desc *dynssz.TypeDescriptor, varName string, indent int, isRoot bool) error {
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	// Handle types that have generated methods we can call
	hasDynamicSize := desc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszMarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	isDynamicMarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagDynamicMarshaler != 0 && !ctx.options.WithoutDynamicExpressions
	isDynamicWriter := desc.SszCompatFlags&dynssz.SszCompatFlagDynamicWriter != 0 && !ctx.options.WithoutDynamicExpressions
	useFastSsz := !ctx.options.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && !isDynamicMarshaler && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	if isDynamicMarshaler && !isRoot {
		ctx.appendCode(indent, "if dst, err = %s.MarshalSSZDyn(ds, dst); err != nil {\n\treturn dst, err\n}\n", varName)
		ctx.usedDynSpecs = true
		return nil
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "if dst, err = %s.MarshalSSZTo(dst); err != nil {\n\treturn dst, err\n}\n", varName)
		return nil
	}

	if isDynamicWriter && !isRoot {
		streamAlias := ctx.typePrinter.AddImport("github.com/pk910/dynamic-ssz/stream", "stream")
		ctx.appendCode(indent, "writer := %s.NewBufferWriter(dst)\n", streamAlias)
		ctx.appendCode(indent, "if err = %s.MarshalSSZDynWriter(ds, writer); err != nil {\n\treturn dst, err\n}\n", varName)
		ctx.appendCode(indent, "dst = writer.Bytes()\n")
		ctx.usedDynSpecs = true
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
		ctx.appendCode(indent, "return dst, sszutils.ErrNotImplemented\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// marshalContainer generates marshal code for SSZ container (struct) types.
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
			ctx.appendCode(indent, "\tsszutils.UpdateOffset(dst[offset%d:offset%d+4], len(dst)-dstlen)\n", idx, idx)
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
func (ctx *marshalContext) marshalVector(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	bitlimitVar := ""
	hasLimitVar := false
	if sizeExpression != nil {
		ctx.usedDynSpecs = true
		if desc.SszTypeFlags&dynssz.SszTypeFlagHasBitSize != 0 {
			ctx.appendCode(indent, "hasLimit, bitlimit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
			ctx.appendCode(indent, "if err != nil {\n\treturn dst, err\n}\n")
			if desc.BitSize > 0 {
				ctx.appendCode(indent, "if !hasLimit {\n\tbitlimit = %d\n}\n", desc.BitSize)
			} else {
				ctx.appendCode(indent, "if !hasLimit {\n\tbitlimit = %d\n}\n", desc.Len*8)
			}
			ctx.appendCode(indent, "limit := (bitlimit+7)/8\n")
			bitlimitVar = "int(bitlimit)"
		} else {
			ctx.appendCode(indent, "hasLimit, limit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
			ctx.appendCode(indent, "if err != nil {\n\treturn dst, err\n}\n")
			ctx.appendCode(indent, "if !hasLimit {\n\tlimit = %d\n}\n", desc.Len)
		}

		limitVar = "int(limit)"
		hasLimitVar = true

		if desc.Kind == reflect.Array {
			// check if dynamic limit is greater than the length of the array
			ctx.appendCode(indent, "if limit > %d {\n", desc.Len)
			ctx.appendCode(indent, "\treturn dst, sszutils.ErrVectorLength\n")
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
		ctx.appendCode(indent, "\treturn dst, sszutils.ErrVectorLength\n")
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
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 || desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			if bitlimitVar != "" {
				ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
				ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", varName, lenVar)
				ctx.appendCode(indent, "\treturn dst, sszutils.ErrVectorLength\n")
				ctx.appendCode(indent, "}\n")
			}
			if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
				ctx.appendCode(indent, "dst = append(dst, %s[:%s]...)\n", varName, lenVar)
			} else {
				ctx.appendCode(indent, "dst = append(dst, []byte(%s[:%s])...)\n", varName, lenVar)
			}
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
			ctx.appendCode(indent, "\tdst = sszutils.AppendZeroPadding(dst, (%s-%s)*%d)\n", limitVar, lenVar, desc.ElemDesc.Size)
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements
		// reserve space for offsets
		ctx.appendCode(indent, "dstlen := len(dst)\n")
		ctx.appendCode(indent, "dst = sszutils.AppendZeroPadding(dst, %s*4)\n", limitVar)
		ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", lenVar)
		ctx.appendCode(indent, "\tsszutils.UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)\n")
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
			ctx.appendCode(indent, "\t\tsszutils.UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)\n")
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
func (ctx *marshalContext) marshalList(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	hasMax := false
	maxVar := ""

	if maxExpression != nil {
		ctx.usedDynSpecs = true
		ctx.appendCode(indent, "hasMax, max, err := ds.ResolveSpecValue(\"%s\")\n", *maxExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn dst, err\n")
		ctx.appendCode(indent, "}\n")
		hasMax = true
		maxVar = "int(max)"
		if desc.Limit > 0 {
			ctx.appendCode(indent, "if !hasMax {\n")
			ctx.appendCode(indent, "\tmax = %d\n", desc.Limit)
			ctx.appendCode(indent, "}\n")
		}
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
		ctx.appendCode(indent, "\treturn dst, sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			ctx.appendCode(indent, "dst = append(dst, %s[:]...)\n", varName)
		} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			ctx.appendCode(indent, "dst = append(dst, []byte(%s[:])...)\n", varName)
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
		ctx.appendCode(indent, "dstlen := len(dst)\n")
		addVlen()
		ctx.appendCode(indent, "dst = sszutils.AppendZeroPadding(dst, vlen*4)\n")
		ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
		ctx.appendCode(indent, "\tsszutils.UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)\n")
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
func (ctx *marshalContext) marshalBitlist(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	hasMax := false
	maxVar := ""

	if maxExpression != nil {
		ctx.usedDynSpecs = true
		ctx.appendCode(indent, "hasMax, max, err := ds.ResolveSpecValue(\"%s\")\n", *maxExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn dst, err\n")
		ctx.appendCode(indent, "}\n")
		hasMax = true
		maxVar = "int(max)"
		if desc.Limit > 0 {
			ctx.appendCode(indent, "if !hasMax {\n")
			ctx.appendCode(indent, "\tmax = %d\n", desc.Limit)
			ctx.appendCode(indent, "}\n")
		}
	} else if desc.Limit > 0 {
		maxVar = fmt.Sprintf("%d", desc.Limit)
		hasMax = true
	} else {
		maxVar = "0"
	}

	ctx.appendCode(indent, "vlen := len(%s)\n", varName)

	if hasMax {
		ctx.appendCode(indent, "if vlen > %s {\n", maxVar)
		ctx.appendCode(indent, "\treturn dst, sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	ctx.appendCode(indent, "bval := []byte(%s[:])\n", varName)
	ctx.appendCode(indent, "if vlen == 0 {\n")
	ctx.appendCode(indent, "\tbval = []byte{0x01}\n")
	ctx.appendCode(indent, "} else if bval[vlen-1] == 0x00 {\n")
	ctx.appendCode(indent, "\treturn dst, sszutils.ErrBitlistNotTerminated\n")
	ctx.appendCode(indent, "}\n")

	if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 || desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		ctx.appendCode(indent, "dst = append(dst, bval...)\n")
	} else {
		return fmt.Errorf("bitlist type can only be represented by byte slices or arrays, got %v", desc.Kind)
	}

	return nil
}

// marshalUnion generates marshal code for SSZ union types.
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
		variantType := ctx.typePrinter.TypeString(variantDesc)
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
