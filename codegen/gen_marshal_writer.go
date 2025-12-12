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

// marshalWriterContext contains the state and utilities for generating marshal writer methods.
//
// This context structure maintains the necessary state during the marshal writer code generation
// process, including code building utilities, type formatting, and options that control
// the generation behavior.
//
// Fields:
//   - appendCode: Function to append formatted code with proper indentation
//   - typePrinter: Type name formatter and import tracker
//   - options: Code generation options controlling output behavior
//   - usedDynSpecs: Flag tracking whether generated code uses dynamic SSZ functionality
type marshalWriterContext struct {
	appendCode   func(indent int, code string, args ...any)
	typePrinter  *TypePrinter
	options      *CodeGeneratorOptions
	usedDynSpecs bool

	sizeFnNameMap     map[*dynssz.TypeDescriptor]int
	sizeFnNameCounter int
}

// generateMarshalWriter generates marshal writer methods for a specific type.
//
// This function creates the complete set of marshal writer methods for a type, including:
//   - MarshalSSZDynWriter for dynamic specification support with streaming
//
// The generated methods handle SSZ encoding according to the type's descriptor,
// supporting both static and dynamic sizing, nested types, and streaming output
// through io.Writer interface.
//
// Parameters:
//   - rootTypeDesc: Type descriptor containing complete SSZ encoding metadata
//   - codeBuilder: String builder to append generated method code to
//   - typePrinter: Type formatter for handling imports and type names
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateMarshalWriter(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder,
	typePrinter *TypePrinter, options *CodeGeneratorOptions) error {
	codeBuf := strings.Builder{}
	ctx := &marshalWriterContext{
		appendCode: func(indent int, code string, args ...any) {
			appendCode(&codeBuf, indent, code, args...)
		},
		typePrinter: typePrinter,
		options:     options,

		sizeFnNameMap: make(map[*dynssz.TypeDescriptor]int),
	}

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate marshaling code
	if err := ctx.marshalWriterType(rootTypeDesc, "t", 1, true); err != nil {
		return err
	}

	genDynamicFn := !options.WithoutDynamicExpressions

	// Generate size function code
	sizeFnCode, err := ctx.generateSizeFnCode(1)
	if err != nil {
		return err
	}

	if genDynamicFn {
		ioAlias := typePrinter.AddImport("io", "io")
		if ctx.usedDynSpecs {
			codeBuilder.WriteString(fmt.Sprintf(
				"func (t %s) MarshalSSZDynWriter(ds sszutils.DynamicSpecs, w %s.Writer) error {\n",
				typeName, ioAlias))
			codeBuilder.WriteString(sizeFnCode)
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			codeBuilder.WriteString(fmt.Sprintf(
				"func (t %s) MarshalSSZDynWriter(_ sszutils.DynamicSpecs, w %s.Writer) error {\n",
				typeName, ioAlias))
			codeBuilder.WriteString(sizeFnCode)
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn nil\n")
			codeBuilder.WriteString("}\n\n")
		}
	}

	return nil
}

// getPtrPrefix returns & for types that are heavy to copy
func (ctx *marshalWriterContext) getPtrPrefix(desc *dynssz.TypeDescriptor) string {
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
func (ctx *marshalWriterContext) isInlineable(desc *dynssz.TypeDescriptor) bool {
	if desc.SszType == dynssz.SszBoolType || desc.SszType == dynssz.SszUint8Type ||
		desc.SszType == dynssz.SszUint16Type || desc.SszType == dynssz.SszUint32Type ||
		desc.SszType == dynssz.SszUint64Type {
		return true
	}

	if desc.SszType == dynssz.SszVectorType || desc.SszType == dynssz.SszListType {
		return desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0
	}

	return false
}

// getStaticSizeVar generates a variable name for cached static size calculations.
func (ctx *marshalWriterContext) getSizeFnCall(desc *dynssz.TypeDescriptor, varName string) string {
	if sizeFnIdx, ok := ctx.sizeFnNameMap[desc]; ok {
		return fmt.Sprintf("sizeFn%d(%s)", sizeFnIdx, varName)
	}

	ctx.sizeFnNameCounter++
	ctx.sizeFnNameMap[desc] = ctx.sizeFnNameCounter

	return fmt.Sprintf("sizeFn%d(%s)", ctx.sizeFnNameCounter, varName)
}

func (ctx *marshalWriterContext) generateSizeFnCode(indent int) (string, error) {
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

		sizeFnMap := make(map[*dynssz.TypeDescriptor]*sizeFnPtr)
		for desc2, idx := range ctx.sizeFnNameMap {
			if desc2 == desc {
				continue
			}
			sizeFnMap[desc2] = &sizeFnPtr{
				fnName:       fmt.Sprintf("sizeFn%d", idx),
				fnArgs:       []string{},
				needDynSpecs: false,
			}
		}

		sizeCtx := newSizeContext(ctx.typePrinter, ctx.options)
		sizeCtx.useTypeFnMap = sizeFnMap
		appendCode(&codeBuf, indent, "// size for %s\n", ctx.typePrinter.TypeString(desc))
		appendCode(&codeBuf, indent, "%s := func(t %s) (size int) {\n", fnName, ctx.typePrinter.TypeString(desc))
		if err := sizeCtx.sizeType(desc, "t", "size", 0, false); err != nil {
			return "", err
		}
		appendCode(&codeBuf, indent+1, "%s", sizeCtx.codeBuf.String())
		appendCode(&codeBuf, indent+1, "return size\n")
		appendCode(&codeBuf, indent, "}\n")
	}

	return codeBuf.String(), nil
}

// marshalWriterType generates marshal writer code for any SSZ type, delegating to specific marshalers.
func (ctx *marshalWriterContext) marshalWriterType(desc *dynssz.TypeDescriptor, varName string, indent int, isRoot bool) error {
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n",
			varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	// Handle types that have generated methods we can call
	hasDynamicSize := desc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 &&
		!ctx.options.WithoutDynamicExpressions
	isFastsszMarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
	isDynamicWriter := desc.SszCompatFlags&dynssz.SszCompatFlagDynamicWriter != 0 &&
		!ctx.options.WithoutDynamicExpressions
	isDynamicMarshaler := desc.SszCompatFlags&dynssz.SszCompatFlagDynamicMarshaler != 0 &&
		!ctx.options.WithoutDynamicExpressions
	useFastSsz := !ctx.options.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
	if !useFastSsz && !isDynamicWriter && !isDynamicMarshaler && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	// Prefer DynamicWriter interface for streaming
	if isDynamicWriter && !isRoot {
		ctx.appendCode(indent, "if err := %s.MarshalSSZDynWriter(ds, w); err != nil {\n\treturn err\n}\n",
			varName)
		ctx.usedDynSpecs = true
		return nil
	}

	// Fall back to DynamicMarshaler with buffer if available
	if isDynamicMarshaler && !isRoot {
		ctx.appendCode(indent, "{\n")
		ctx.appendCode(indent, "\tvar buf []byte\n")
		ctx.appendCode(indent, "\tvar err error\n")
		ctx.appendCode(indent, "\tif buf, err = %s.MarshalSSZDyn(ds, buf); err != nil {\n\t\treturn err\n\t}\n",
			varName)
		ctx.appendCode(indent, "\tif _, err = w.Write(buf); err != nil {\n\t\treturn err\n\t}\n")
		ctx.appendCode(indent, "}\n")
		ctx.usedDynSpecs = true
		return nil
	}

	// Fall back to FastSSZ marshaler with buffer if available
	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "{\n")
		ctx.appendCode(indent, "\tvar buf []byte\n")
		ctx.appendCode(indent, "\tvar err error\n")
		ctx.appendCode(indent, "\tif buf, err = %s.MarshalSSZTo(buf); err != nil {\n\t\treturn err\n\t}\n",
			varName)
		ctx.appendCode(indent, "\tif _, err = w.Write(buf); err != nil {\n\t\treturn err\n\t}\n")
		ctx.appendCode(indent, "}\n")
		return nil
	}

	switch desc.SszType {
	case dynssz.SszBoolType:
		ctx.appendCode(indent, "if err := sszutils.MarshalBoolWriter(w, bool(%s)); err != nil {\n\treturn err\n}\n",
			varName)

	case dynssz.SszUint8Type:
		ctx.appendCode(indent, "if err := sszutils.MarshalUint8Writer(w, uint8(%s)); err != nil {\n\treturn err\n}\n",
			varName)

	case dynssz.SszUint16Type:
		ctx.appendCode(indent, "if err := sszutils.MarshalUint16Writer(w, uint16(%s)); err != nil {\n\treturn err\n}\n",
			varName)

	case dynssz.SszUint32Type:
		ctx.appendCode(indent, "if err := sszutils.MarshalUint32Writer(w, uint32(%s)); err != nil {\n\treturn err\n}\n",
			varName)

	case dynssz.SszUint64Type:
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsTime != 0 {
			ctx.appendCode(indent,
				"if err := sszutils.MarshalUint64Writer(w, uint64(%s.Unix())); err != nil {\n\treturn err\n}\n",
				varName)
		} else {
			ctx.appendCode(indent,
				"if err := sszutils.MarshalUint64Writer(w, uint64(%s)); err != nil {\n\treturn err\n}\n",
				varName)
		}

	case dynssz.SszTypeWrapperType:
		ctx.appendCode(indent, "{\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s.Data", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s.Data\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
		}
		if err := ctx.marshalWriterType(desc.ElemDesc, valVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

	case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
		return ctx.marshalWriterContainer(desc, varName, indent)

	case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
		return ctx.marshalWriterVector(desc, varName, indent)

	case dynssz.SszListType, dynssz.SszProgressiveListType:
		return ctx.marshalWriterList(desc, varName, indent)

	case dynssz.SszBitlistType, dynssz.SszProgressiveBitlistType:
		return ctx.marshalWriterBitlist(desc, varName, indent)

	case dynssz.SszCompatibleUnionType:
		return ctx.marshalWriterUnion(desc, varName, indent)

	case dynssz.SszCustomType:
		ctx.appendCode(indent, "return sszutils.ErrNotImplemented\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// marshalWriterContainer generates marshal writer code for SSZ container (struct) types.
func (ctx *marshalWriterContext) marshalWriterContainer(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	hasDynamic := false
	staticSize := uint32(0)
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			hasDynamic = true
			staticSize += 4
		} else {
			staticSize += field.Type.Size
		}
	}

	// Write fixed fields and offsets for dynamic fields
	if hasDynamic {
		ctx.appendCode(indent, "currentOffset := uint32(%d) // Fixed section size\n", staticSize)
	}

	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
			// Write offset for dynamic field
			ctx.appendCode(indent, "// Offset #%d '%s'\n", idx, field.Name)
			ctx.appendCode(indent, "if err := sszutils.MarshalOffsetWriter(w, currentOffset); err != nil {\n\treturn err\n}\n")
			ctx.appendCode(indent, "currentOffset += uint32(%s)\n", ctx.getSizeFnCall(field.Type, fmt.Sprintf("%s.%s", varName, field.Name)))
		} else {
			// Marshal fixed fields
			ctx.appendCode(indent, "{ // Field #%d '%s'\n", idx, field.Name)
			valVar := "t"
			if ctx.isInlineable(field.Type) {
				valVar = fmt.Sprintf("%s.%s", varName, field.Name)
			} else {
				ctx.appendCode(indent, "\tt := %s%s.%s\n", ctx.getPtrPrefix(field.Type), varName, field.Name)
			}
			if err := ctx.marshalWriterType(field.Type, valVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	if hasDynamic {
		// Marshal dynamic fields
		for idx, field := range desc.ContainerDesc.Fields {
			if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
				ctx.appendCode(indent, "{ // Dynamic Field #%d '%s'\n", idx, field.Name)
				valVar := "t"
				if ctx.isInlineable(field.Type) {
					valVar = fmt.Sprintf("%s.%s", varName, field.Name)
				} else {
					ctx.appendCode(indent, "\tt := %s%s.%s\n", ctx.getPtrPrefix(field.Type), varName, field.Name)
				}
				if err := ctx.marshalWriterType(field.Type, valVar, indent+1, false); err != nil {
					return err
				}
				ctx.appendCode(indent, "}\n")
			}
		}
	}

	return nil
}

// marshalWriterVector generates marshal writer code for SSZ vector (fixed-size array) types.
func (ctx *marshalWriterContext) marshalWriterVector(desc *dynssz.TypeDescriptor, varName string, indent int) error {
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
			ctx.appendCode(indent, "if err != nil {\n\treturn err\n}\n")
			if desc.BitSize > 0 {
				ctx.appendCode(indent, "if !hasLimit {\n\tbitlimit = %d\n}\n", desc.BitSize)
			} else {
				ctx.appendCode(indent, "if !hasLimit {\n\tbitlimit = %d\n}\n", desc.Len*8)
			}
			ctx.appendCode(indent, "limit := (bitlimit+7)/8\n")
			bitlimitVar = "int(bitlimit)"
		} else {
			ctx.appendCode(indent, "hasLimit, limit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
			ctx.appendCode(indent, "if err != nil {\n\treturn err\n}\n")
			ctx.appendCode(indent, "if !hasLimit {\n\tlimit = %d\n}\n", desc.Len)
		}

		limitVar = "int(limit)"
		hasLimitVar = true

		if desc.Kind == reflect.Array {
			// check if dynamic limit is greater than the length of the array
			ctx.appendCode(indent, "if limit > %d {\n", desc.Len)
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
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 ||
			desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			if bitlimitVar != "" {
				ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
				ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", varName, lenVar)
				ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
				ctx.appendCode(indent, "}\n")
			}
			if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
				ctx.appendCode(indent, "if _, err := w.Write([]byte(%s[:%s])); err != nil {\n\treturn err\n}\n",
					varName, lenVar)
			} else {
				ctx.appendCode(indent, "if _, err := w.Write(%s[:%s]); err != nil {\n\treturn err\n}\n",
					varName, lenVar)
			}
		} else {
			ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", lenVar)
			valVar := "t"
			if ctx.isInlineable(desc.ElemDesc) {
				valVar = fmt.Sprintf("%s[i]", varName)
			} else {
				ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
			}
			if err := ctx.marshalWriterType(desc.ElemDesc, valVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\tif err := sszutils.AppendZeroPaddingWriter(w, (%s-%s)*%d); err != nil {\n\t\treturn err\n\t}\n",
				limitVar, lenVar, desc.ElemDesc.Size)
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements - need to buffer for offset calculation

		// Write offsets
		ctx.appendCode(indent, "currentOffset := uint32(%s) * 4\n", limitVar)
		ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", lenVar)
		ctx.appendCode(indent, "\tif err := sszutils.MarshalOffsetWriter(w, currentOffset); err != nil {\n\t\treturn err\n\t}\n")
		fieldSizeFn := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[i]", varName))
		ctx.appendCode(indent, "\tcurrentOffset += uint32(%s)\n", fieldSizeFn)
		ctx.appendCode(indent, "}\n")
		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "var zeroItem %s\n", ctx.typePrinter.TypeString(desc.ElemDesc))
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)

			if desc.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent, "\tzeroItem = new(%s)\n", ctx.typePrinter.InnerTypeString(desc.ElemDesc))
			}

			fieldSizeFn := ctx.getSizeFnCall(desc.ElemDesc, "zeroItem")
			ctx.appendCode(indent, "\tzeroLen := %s\n", fieldSizeFn)
			ctx.appendCode(indent, "\tfor i := %s; i < %s; i++ {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\t\tif err := sszutils.MarshalOffsetWriter(w, currentOffset); err != nil {\n\t\t\treturn err\n\t\t}\n")
			ctx.appendCode(indent, "\t\tcurrentOffset += uint32(zeroLen)\n")
			ctx.appendCode(indent, "\t}\n")
			ctx.appendCode(indent, "}\n")
		}

		// Write elements
		ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", lenVar)
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s[i]", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
		}
		if err := ctx.marshalWriterType(desc.ElemDesc, valVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

		if desc.Kind != reflect.Array {
			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\tfor i := %s; i < %s; i++ {\n", lenVar, limitVar)
			if err := ctx.marshalWriterType(desc.ElemDesc, "zeroItem", indent+2, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "\t}\n")
			ctx.appendCode(indent, "}\n")
		}
	}

	return nil
}

// marshalWriterList generates marshal writer code for SSZ list (variable-size array) types.
func (ctx *marshalWriterContext) marshalWriterList(desc *dynssz.TypeDescriptor, varName string,
	indent int) error {
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
		ctx.appendCode(indent, "\treturn err\n")
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
		ctx.appendCode(indent, "\treturn sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	if desc.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic == 0 {
		// static elements
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
			ctx.appendCode(indent, "if _, err := w.Write([]byte(%s[:])); err != nil {\n\treturn err\n}\n",
				varName)
		} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			ctx.appendCode(indent, "if _, err := w.Write(%s[:]); err != nil {\n\treturn err\n}\n",
				varName)
		} else {
			addVlen()
			ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
			valVar := "t"
			if ctx.isInlineable(desc.ElemDesc) {
				valVar = fmt.Sprintf("%s[i]", varName)
			} else {
				ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
			}
			if err := ctx.marshalWriterType(desc.ElemDesc, valVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		}
	} else {
		// dynamic elements - need to calculate sizes for offsets

		// Write offsets
		addVlen()
		ctx.appendCode(indent, "currentOffset := uint32(vlen) * 4\n")
		ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
		ctx.appendCode(indent, "\tif err := sszutils.MarshalOffsetWriter(w, currentOffset); err != nil {\n\t\treturn err\n\t}\n")
		fieldSizeFn := ctx.getSizeFnCall(desc.ElemDesc, fmt.Sprintf("%s[i]", varName))
		ctx.appendCode(indent, "\tcurrentOffset += uint32(%s)\n", fieldSizeFn)
		ctx.appendCode(indent, "}\n")

		// Write elements
		ctx.appendCode(indent, "for i := 0; i < vlen; i++ {\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s[i]", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc), varName)
		}
		if err := ctx.marshalWriterType(desc.ElemDesc, valVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// marshalWriterBitlist generates marshal writer code for SSZ bitlist types.
func (ctx *marshalWriterContext) marshalWriterBitlist(desc *dynssz.TypeDescriptor, varName string,
	indent int) error {
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
		ctx.appendCode(indent, "\treturn err\n")
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
		ctx.appendCode(indent, "\treturn sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	ctx.appendCode(indent, "bval := []byte(%s[:])\n", varName)
	ctx.appendCode(indent, "if vlen == 0 {\n")
	ctx.appendCode(indent, "\tbval = []byte{0x01}\n")
	ctx.appendCode(indent, "} else if bval[vlen-1] == 0x00 {\n")
	ctx.appendCode(indent, "\treturn sszutils.ErrBitlistNotTerminated\n")
	ctx.appendCode(indent, "}\n")

	if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 ||
		desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		ctx.appendCode(indent, "if _, err := w.Write(bval); err != nil {\n\treturn err\n}\n")
	} else {
		return fmt.Errorf("bitlist type can only be represented by byte slices or arrays, got %v", desc.Kind)
	}

	return nil
}

// marshalWriterUnion generates marshal writer code for SSZ union types.
func (ctx *marshalWriterContext) marshalWriterUnion(desc *dynssz.TypeDescriptor, varName string,
	indent int) error {
	ctx.appendCode(indent, "if err := sszutils.MarshalUint8Writer(w, %s.Variant); err != nil {\n\treturn err\n}\n",
		varName)
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
		if err := ctx.marshalWriterType(variantDesc, "v", indent+1, false); err != nil {
			return err
		}
	}
	ctx.appendCode(indent, "default:\n")
	ctx.appendCode(indent, "\treturn sszutils.ErrInvalidUnionVariant\n")
	ctx.appendCode(indent, "}\n")

	return nil
}
