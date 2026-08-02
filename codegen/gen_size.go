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
	staticSizeVars  *staticSizeVarGenerator
	usedDynSpecs    bool
	indexVarCounter int
	sizeVarCounter  int
	limitVarCounter int

	useTypeFnMap map[*ssztypes.TypeDescriptor]*sizeFnPtr
	recursion    *recursionBound
	// depthAware is set while emitting a type that lies on a recursive cycle,
	// where the body runs inside a depth-carrying method and can pass the depth
	// on to a cyclic child.
	depthAware bool
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
	ctx.staticSizeVars = newStaticSizeVarGenerator(typePrinter, options, ctx.exprVars)

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
	ctx.recursion = newRecursionBound(rootTypeDesc, options)
	ctx.depthAware = ctx.recursion.threads(rootTypeDesc)

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate size calculation code
	if err := ctx.sizeType(rootTypeDesc, "t", "size", 0, true); err != nil {
		return err
	}

	if ctx.exprVars.varCounter > 0 || ctx.staticSizeVars.varCounter > 0 {
		ctx.usedDynSpecs = true
	}

	genDynamicFn := !options.WithoutDynamicExpressions || viewName != ""
	genStaticFn := (options.WithoutDynamicExpressions || options.CreateLegacyFn) && viewName == ""

	if genDynamicFn && !ctx.usedDynSpecs && viewName == "" {
		genStaticFn = true
	}

	if genStaticFn {
		if !ctx.usedDynSpecs {
			appendCode(codeBuilder, 0, "// SizeSSZ returns the SSZ encoded size of the %s.\n", typeName)
			emitMethodHeader(codeBuilder, ctx.recursion, rootTypeDesc, typeName, "SizeSSZ", "", "", "size int", "0")
			if rootTypeDesc.Size > 0 {
				appendCode(codeBuilder, 1, "return %d\n", rootTypeDesc.Size)
			} else {
				appendCode(codeBuilder, 1, ctx.exprVars.getCode())
				appendCode(codeBuilder, 1, ctx.staticSizeVars.getCode())
				appendCode(codeBuilder, 1, ctx.codeBuf.String())
				appendCode(codeBuilder, 1, "return size\n")
			}
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			appendCode(codeBuilder, 0, "// SizeSSZ returns the SSZ encoded size of the %s.\n", typeName)
			emitMethodHeader(codeBuilder, ctx.recursion, rootTypeDesc, typeName, "SizeSSZ", "", "", "size int", "0")
			appendCode(codeBuilder, 1, "return t.%s(%s.GetGlobalDynSsz()%s)\n", depthForwardName("SizeSSZDyn", ctx.depthAware), dynsszAlias, depthForwardArg(ctx.depthAware))
			appendCode(codeBuilder, 0, "}\n\n")
		}
	}

	if genDynamicFn {
		fnName := "SizeSSZDyn"
		if viewName != "" {
			fnName = fmt.Sprintf("sizeSSZView_%s", viewName)
		}
		if ctx.usedDynSpecs || viewName != "" {
			if viewName == "" {
				appendCode(codeBuilder, 0, "// SizeSSZDyn returns the SSZ encoded size of the %s using dynamic specifications.\n", typeName)
			}
			emitMethodHeader(codeBuilder, ctx.recursion, rootTypeDesc, typeName, fnName, "ds sszutils.DynamicSpecs", "ds", "size int", "0")
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, ctx.staticSizeVars.getCode())
			appendCode(codeBuilder, 1, ctx.codeBuf.String())
			appendCode(codeBuilder, 1, "return size\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			if viewName == "" {
				appendCode(codeBuilder, 0, "// SizeSSZDyn returns the SSZ encoded size of the %s using dynamic specifications.\n", typeName)
			}
			emitMethodHeader(codeBuilder, ctx.recursion, rootTypeDesc, typeName, fnName, "ds sszutils.DynamicSpecs", "ds", "size int", "0")
			appendCode(codeBuilder, 1, "_ = ds\n")
			appendCode(codeBuilder, 1, "return t.%s(%s)\n", depthForwardName("SizeSSZ", ctx.depthAware), depthForwardArgBare(ctx.depthAware))
			appendCode(codeBuilder, 0, "}\n\n")
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
func (ctx *sizeContext) getValueVar(desc *ssztypes.TypeDescriptor, varName, targetType string) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
		varName = fmt.Sprintf("*%s", varName)
	}

	if targetType != "" && ctx.typePrinter.InnerTypeString(desc) != targetType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// sizeType generates size calculation code for any SSZ type, delegating to specific sizers.
func (ctx *sizeContext) sizeType(desc *ssztypes.TypeDescriptor, varName, sizeVar string, indent int, isRoot bool) error {
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
			viewFn, viewArg := descendCall(ctx.depthAware, ctx.recursion, desc, "SizeSSZDynView")
			ctx.appendCode(indent, "if viewFn := %s.%s((%s)(nil)%s); viewFn != nil {\n", varName, viewFn, ctx.typePrinter.ViewTypeString(desc, true), viewArg)
			ctx.appendCode(indent+1, "%s += viewFn(ds)\n", sizeVar)
			ctx.appendCode(indent, "}\n")
			ctx.usedDynSpecs = true
			return nil
		}
	}

	if !isRoot && !isView {
		hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
		// Under WithoutDynamicExpressions the generated code must be fully static
		// and must never call a *Dyn method. A child exposing a static SizeSSZ
		// (every dynssz-generated child in this mode, plus external fastssz types)
		// is reached through it even when fastssz delegation is otherwise disabled.
		useFastSsz := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 && !hasDynamicSize &&
			(!ctx.options.NoFastSsz || ctx.options.WithoutDynamicExpressions)
		if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
			useFastSsz = true
		}

		if ctx.options.WithoutDynamicExpressions {
			if useFastSsz {
				fn, arg := descendCall(ctx.depthAware, ctx.recursion, desc, "SizeSSZ")
				ctx.appendCode(indent, "%s += %s.%s(%s)\n", sizeVar, varName, fn, strings.TrimPrefix(arg, ", "))
				return nil
			}
			// Never call a *Dyn method. A dynamic-only type is inlined by falling
			// through to the structural switch below; only types with no
			// traversable structure (custom or shallow-delegated externals) are
			// rejected.
			if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicSizer != 0 &&
				(desc.SszType == ssztypes.SszCustomType || isShallowDelegatedDescriptor(desc)) {
				return fmt.Errorf("cannot generate static sizer for %s under without-dynamic-expressions: it provides only dynamic (spec-aware) SSZ methods and has no static SizeSSZ or inlinable structure; add it to the generation set or provide a static sizer", ctx.typePrinter.TypeString(desc))
			}
		} else {
			if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicSizer != 0 {
				fn, arg := descendCall(ctx.depthAware, ctx.recursion, desc, "SizeSSZDyn")
				ctx.appendCode(indent, "%s += %s.%s(ds%s)\n", sizeVar, varName, fn, arg)
				ctx.usedDynSpecs = true
				return nil
			}

			if useFastSsz {
				fn, arg := descendCall(ctx.depthAware, ctx.recursion, desc, "SizeSSZ")
				ctx.appendCode(indent, "%s += %s.%s(%s)\n", sizeVar, varName, fn, strings.TrimPrefix(arg, ", "))
				return nil
			}
		}
	}

	// create temporary instance for nil pointers
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.SszType != ssztypes.SszOptionalType && desc.SszType != ssztypes.SszOptionalListType {
		if len(varName) > 1 {
			// Localize into a fresh name; a literal "t" would collide with a size
			// closure's own "t" parameter (e.g. a wrapper of a pointer type).
			local := localizedVarName(varName, indent)
			ctx.appendCode(indent, "%s := %s\n", local, varName)
			varName = local
		}
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	// A cycle member reached without delegation is inlined here, so the level
	// it counts is charged inline (see emitInlineDepthCharge).
	emitInlineDepthCharge(ctx.depthAware, isRoot, isView, ctx.recursion, desc, ctx.appendCode, indent, "0")

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
		fieldName := getTypeWrapperFieldName(desc)
		if fieldName == "" {
			return fmt.Errorf("could not determine data field name for wrapper descriptor")
		}
		innerVarName := fmt.Sprintf("%s.%s", varName, fieldName)
		if err := ctx.sizeType(desc.ElemDesc, innerVarName, sizeVar, indent, false); err != nil {
			return err
		}

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return ctx.sizeContainer(desc, varName, sizeVar, indent)

	case ssztypes.SszVectorType, ssztypes.SszBitvectorType, ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return ctx.sizeVector(desc, varName, sizeVar, indent)

	case ssztypes.SszListType, ssztypes.SszBitlistType, ssztypes.SszProgressiveListType, ssztypes.SszProgressiveBitlistType:
		return ctx.sizeList(desc, varName, sizeVar, indent)

	case ssztypes.SszUnionType, ssztypes.SszCompatibleUnionType:
		return ctx.sizeUnion(desc, varName, sizeVar, indent)

	case ssztypes.SszCustomType:
		ctx.appendCode(indent, "// Custom type - size unknown\n")

	// extended types
	case ssztypes.SszInt8Type:
		ctx.appendCode(indent, "%s += 1\n", sizeVar)
	case ssztypes.SszInt16Type:
		ctx.appendCode(indent, "%s += 2\n", sizeVar)
	case ssztypes.SszInt32Type:
		ctx.appendCode(indent, "%s += 4\n", sizeVar)
	case ssztypes.SszInt64Type:
		ctx.appendCode(indent, "%s += 8\n", sizeVar)
	case ssztypes.SszFloat32Type:
		ctx.appendCode(indent, "%s += 4\n", sizeVar)
	case ssztypes.SszFloat64Type:
		ctx.appendCode(indent, "%s += 8\n", sizeVar)
	case ssztypes.SszOptionalType:
		return ctx.sizeOptional(desc, varName, sizeVar, indent)
	case ssztypes.SszOptionalListType:
		return ctx.sizeOptionalList(desc, varName, sizeVar, indent)
	case ssztypes.SszBigIntType:
		// A static ssz-max is enforced by the marshal/HTR paths (which return an
		// error); the size path has no error channel, so an over-max value yields
		// size 0 to signal it is not serializable rather than a misleading size.
		if desc.MaxExpression == nil && desc.Limit > 0 {
			ctx.appendCode(indent, "if uint64(1+len(%s.Bytes())) > %d {\n\treturn 0\n}\n", varName, desc.Limit)
		}
		ctx.appendCode(indent, "%s += 1 + len(%s.Bytes())\n", sizeVar, varName)

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// sizeOptional generates size calculation code for SSZ optional types.
func (ctx *sizeContext) sizeOptional(desc *ssztypes.TypeDescriptor, varName, sizeVar string, indent int) error {
	ctx.appendCode(indent, "%s += 1 // presence byte\n", sizeVar)
	ctx.appendCode(indent, "if %s != nil {\n", varName)
	innerVarName := fmt.Sprintf("(*%s)", varName)
	if err := ctx.sizeType(desc.ElemDesc, innerVarName, sizeVar, indent+1, false); err != nil {
		return err
	}
	ctx.appendCode(indent, "}\n")
	return nil
}

// sizeOptionalList generates size calculation code for optional-list (canonical List[T, 1]).
//
// nil → 0 bytes. Non-nil → size of the single element, plus 4 bytes for the
// offset header when the element is dynamic.
func (ctx *sizeContext) sizeOptionalList(desc *ssztypes.TypeDescriptor, varName, sizeVar string, indent int) error {
	ctx.appendCode(indent, "if %s != nil {\n", varName)
	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
		ctx.appendCode(indent+1, "%s += 4 // optional-list offset header\n", sizeVar)
	}
	innerVarName := fmt.Sprintf("(*%s)", varName)
	if err := ctx.sizeType(desc.ElemDesc, innerVarName, sizeVar, indent+1, false); err != nil {
		return err
	}
	ctx.appendCode(indent, "}\n")
	return nil
}

// sizeContainer generates size calculation code for SSZ container (struct) types.
func (ctx *sizeContext) sizeContainer(desc *ssztypes.TypeDescriptor, varName, sizeVar string, indent int) error {
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
func (ctx *sizeContext) sizeVector(desc *ssztypes.TypeDescriptor, varName, sizeVar string, indent int) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	valueVar := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		targetType := ""
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
			targetType = typeNameString
		}
		valueVar = ctx.getValueVar(desc, varName, targetType)
	}

	usedVar := false
	useVar := func() {
		if usedVar {
			return
		}
		if len(valueVar) > 1 {
			ctx.appendCode(indent, "%s := %s%s\n", localizedVarName(varName, indent), ctx.getPtrPrefix(desc), valueVar)
			valueVar = localizedVarName(varName, indent)
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
		switch {
		case desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions:
			// The element size follows from its size expression, not from the
			// values present: missing rows are zero-padded on the wire, so the
			// size must not depend on how many elements are set (SizeSSZ has to
			// equal len(MarshalSSZ) even for a fully-empty value).
			innerSizeVar, err := ctx.staticSizeVars.getStaticSizeVar(desc.ElemDesc)
			if err != nil {
				return err
			}
			ctx.appendCode(indent, "%s += int(%s) * %s\n", sizeVar, innerSizeVar, limitVar)
		case desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 || desc.ElemDesc.Size == 1:
			// For byte arrays, size is just the vector length
			ctx.appendCode(indent, "%s += %s\n", sizeVar, limitVar)
		default:
			// Fixed size elements - simple multiplication
			ctx.appendCode(indent, "%s += %s * %d\n", sizeVar, limitVar, desc.ElemDesc.Size)
		}
	} else {
		// Dynamic size elements - need to iterate
		ctx.appendCode(indent, "%s += %s * 4\n", sizeVar, limitVar)

		if desc.Kind == reflect.Array {
			indexVar := ctx.getIndexVar()
			ctx.appendCode(indent, "for %s := range %s {\n", indexVar, limitVar)
			// A deref'd pointer receiver (e.g. *[N]E -> "*t") must be
			// parenthesized before indexing: (*t)[i], not *t[i].
			itemVarName := fmt.Sprintf("%s[%s]", indexBase(valueVar), indexVar)
			if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")
		} else {
			useVar()
			ctx.appendCode(indent, "vlen := len(%s)\n", valueVar)
			ctx.appendCode(indent, "if vlen > %s {\n", limitVar)
			ctx.appendCode(indent, "\tvlen = %s\n", limitVar)
			ctx.appendCode(indent, "}\n")
			indexVar := ctx.getIndexVar()
			ctx.appendCode(indent, "for %s := range vlen {\n", indexVar)
			itemVarName := fmt.Sprintf("%s[%s]", valueVar, indexVar)
			if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
				return err
			}
			ctx.appendCode(indent, "}\n")

			// Add size for zero-padding. The padding item is a zero value of the
			// element's own Go type (a nil pointer for pointer elements, which
			// the element sizer handles); a composite literal would be invalid
			// for non-composite pointees such as *string.
			ctx.appendCode(indent, "if vlen < %s {\n", limitVar)
			ctx.appendCode(indent, "\tvar zeroItem %s\n", ctx.typePrinter.TypeString(desc.ElemDesc))
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
func (ctx *sizeContext) sizeList(desc *ssztypes.TypeDescriptor, varName, sizeVar string, indent int) error {
	valueVar := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		targetType := ""
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
			targetType = typeNameString
		}
		valueVar = ctx.getValueVar(desc, varName, targetType)
	}

	// For byte slices, size is just the length
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
		if desc.SszType == ssztypes.SszBitlistType || desc.SszType == ssztypes.SszProgressiveBitlistType {
			// Bitlists always have at least 1 byte (sentinel byte for empty bitlists)
			ctx.appendCode(indent, "if len(%s) == 0 {\n", valueVar)
			ctx.appendCode(indent, "\t%s += 1\n", sizeVar)
			ctx.appendCode(indent, "} else {\n")
			ctx.appendCode(indent, "\t%s += len(%s)\n", sizeVar, valueVar)
			ctx.appendCode(indent, "}\n")
		} else {
			ctx.appendCode(indent, "%s += len(%s)\n", sizeVar, valueVar)
		}
		return nil
	}

	usedVar := false
	useVar := func() {
		if usedVar {
			return
		}
		if len(valueVar) > 1 {
			ctx.appendCode(indent, "%s := %s%s\n", localizedVarName(varName, indent), ctx.getPtrPrefix(desc), valueVar)
			valueVar = localizedVarName(varName, indent)
		}
		usedVar = true
	}

	// Handle lists with dynamic elements
	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
		useVar()
		ctx.appendCode(indent, "vlen := len(%s)\n", valueVar)

		// Add offset space
		ctx.appendCode(indent, "%s += vlen * 4 // Offsets\n", sizeVar)

		// Add size of each element
		indexVar := ctx.getIndexVar()
		ctx.appendCode(indent, "for %s := range vlen {\n", indexVar)
		itemVarName := fmt.Sprintf("%s[%s]", valueVar, indexVar)
		if err := ctx.sizeType(desc.ElemDesc, itemVarName, sizeVar, indent+1, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// Fixed size elements
		if desc.ElemDesc.Size > 0 && (desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr == 0 || ctx.options.WithoutDynamicExpressions) {
			if desc.ElemDesc.Size == 1 {
				ctx.appendCode(indent, "%s += len(%s)\n", sizeVar, valueVar)
			} else {
				ctx.appendCode(indent, "%s += len(%s) * %d\n", sizeVar, valueVar, desc.ElemDesc.Size)
			}
		} else {
			useVar()
			ctx.appendCode(indent, "vlen := len(%s)\n", valueVar)
			ctx.appendCode(indent, "if vlen > 0 {\n")
			innerSizeVar := ctx.getSizeVar()
			ctx.appendCode(indent, "\t%s := 0\n", innerSizeVar)
			itemVarName := fmt.Sprintf("%s[0]", valueVar)
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
func (ctx *sizeContext) sizeUnion(desc *ssztypes.TypeDescriptor, varName, sizeVar string, indent int) error {
	if len(varName) > 1 {
		localVar := localizedVarName(varName, indent)
		ctx.appendCode(indent, "%s := %s\n", localVar, varName)
		varName = localVar
	}

	ctx.appendCode(indent, "%s += 1 // Union selector\n", sizeVar)
	ctx.appendCode(indent, "switch %s.Variant {\n", varName)

	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasNoneVariant != 0 {
		// The None option carries no value beyond the selector byte.
		ctx.appendCode(indent, "case 0:\n")
	}

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
			sizeStart := ctx.codeBuf.Len()
			if err := ctx.sizeType(variantDesc, "v", sizeVar, indent+1, false); err != nil {
				return err
			}
			// A variant whose size is fully determined by a size expression (e.g.
			// a static container with a dynssz-size field) never reads the asserted
			// value, which would leave `v` declared-and-unused. Reference it
			// explicitly so the generated code compiles.
			if !codeReferencesIdent(ctx.codeBuf.String()[sizeStart:], "v") {
				ctx.appendCode(indent, "\t_ = v\n")
			}
		} else {
			// A fixed-size variant still validates the data type: sizing
			// mismatched data as the declared fixed size would fabricate a
			// plausible-looking size for a value the marshalers reject.
			ctx.appendCode(indent, "\tif _, ok := %s.Data.(%s); !ok {\n", varName, variantType)
			ctx.appendCode(indent, "\t\treturn 0\n")
			ctx.appendCode(indent, "\t}\n")
			ctx.appendCode(indent, "\t%s += %d\n", sizeVar, variantDesc.Size)
		}
	}

	// An unknown selector cannot be sized; report 0 like the mismatched-data
	// branches above instead of returning a plausible-looking partial size for
	// a value the marshalers reject.
	ctx.appendCode(indent, "default:\n")
	ctx.appendCode(indent, "\treturn 0\n")

	ctx.appendCode(indent, "}\n")

	return nil
}

// codeReferencesIdent reports whether the generated code snippet uses ident as a
// standalone token, ignoring // line comments. It lets the union-variant sizing
// code decide whether the asserted value var is actually read; a variant whose
// size is a pure expression never reads it, which would otherwise leave the
// variable declared-and-unused.
func codeReferencesIdent(code, ident string) bool {
	isIdentByte := func(b byte) bool {
		return b == '_' ||
			(b >= 'a' && b <= 'z') ||
			(b >= 'A' && b <= 'Z') ||
			(b >= '0' && b <= '9')
	}
	for _, line := range strings.Split(code, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		for start := 0; start < len(line); {
			j := strings.Index(line[start:], ident)
			if j < 0 {
				break
			}
			pos := start + j
			var before, after byte = ' ', ' '
			if pos > 0 {
				before = line[pos-1]
			}
			if pos+len(ident) < len(line) {
				after = line[pos+len(ident)]
			}
			if !isIdentByte(before) && !isIdentByte(after) {
				return true
			}
			start = pos + len(ident)
		}
	}
	return false
}
