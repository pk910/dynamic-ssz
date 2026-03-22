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

// unmarshalContext contains the state and utilities for generating unmarshal methods.
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
type unmarshalContext struct {
	appendCode     func(indent int, code string, args ...any)
	typePrinter    *TypePrinter
	options        *CodeGeneratorOptions
	exprVars       *exprVarGenerator
	staticSizeVars *staticSizeVarGenerator
	usedDynSpecs   bool
	valVarCounter  int
	indexCounter   int
}

// generateUnmarshal generates unmarshal methods for a specific type.
//
// This function creates the complete set of unmarshal methods for a type, including:
//   - UnmarshalSSZDyn for dynamic specification support with runtime parsing
//   - UnmarshalSSZ for static/legacy compatibility with compile-time known layouts
//
// The generated methods handle SSZ decoding according to the type's descriptor,
// supporting variable-length fields, dynamic expressions, nested types, and
// proper error handling for malformed or oversized data.
//
// Parameters:
//   - rootTypeDesc: Type descriptor containing complete SSZ decoding metadata
//   - codeBuilder: String builder to append generated method code to
//   - typePrinter: Type formatter for handling imports and type names
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateUnmarshal(rootTypeDesc *ssztypes.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) error {
	codeBuf := strings.Builder{}
	ctx := &unmarshalContext{
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

	genDynamicFn := !options.WithoutDynamicExpressions
	genStaticFn := options.WithoutDynamicExpressions || options.CreateLegacyFn

	if genDynamicFn && !ctx.usedDynSpecs {
		genStaticFn = true
	}

	if genStaticFn {
		if !ctx.usedDynSpecs {
			appendCode(codeBuilder, 0, "// UnmarshalSSZ unmarshals the %s from SSZ-encoded bytes.\n", typeName)
			appendCode(codeBuilder, 0, "func (t %s) UnmarshalSSZ(buf []byte) (err error) {\n", typeName)
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, ctx.staticSizeVars.getCode())
			appendCode(codeBuilder, 1, codeBuf.String())
			appendCode(codeBuilder, 1, "return nil\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			appendCode(codeBuilder, 0, "// UnmarshalSSZ unmarshals the %s from SSZ-encoded bytes.\n", typeName)
			appendCode(codeBuilder, 0, "func (t %s) UnmarshalSSZ(buf []byte) (err error) {\n", typeName)
			appendCode(codeBuilder, 1, "return t.UnmarshalSSZDyn(%s.GetGlobalDynSsz(), buf)\n", dynsszAlias)
			appendCode(codeBuilder, 0, "}\n\n")
		}
	}

	if genDynamicFn {
		if ctx.usedDynSpecs {
			appendCode(codeBuilder, 0, "// UnmarshalSSZDyn unmarshals the %s from SSZ-encoded bytes using dynamic specifications.\n", typeName)
			appendCode(codeBuilder, 0, "func (t %s) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) (err error) {\n", typeName)
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, ctx.staticSizeVars.getCode())
			appendCode(codeBuilder, 1, codeBuf.String())
			appendCode(codeBuilder, 1, "return nil\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			appendCode(codeBuilder, 0, "// UnmarshalSSZDyn unmarshals the %s from SSZ-encoded bytes using dynamic specifications.\n", typeName)
			appendCode(codeBuilder, 0, "func (t %s) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) (err error) {\n", typeName)
			appendCode(codeBuilder, 1, "return t.UnmarshalSSZ(buf)\n")
			appendCode(codeBuilder, 0, "}\n\n")
		}
	}

	return nil
}

// getValVar generates a unique variable name for temporary values.
func (ctx *unmarshalContext) getValVar() string {
	ctx.valVarCounter++
	return fmt.Sprintf("val%d", ctx.valVarCounter)
}

// getCastedValueVar returns the variable name for the value of a type, converting to the source type if needed
func (ctx *unmarshalContext) getCastedValueVar(desc *ssztypes.TypeDescriptor, varName, sourceType string) string {
	if targetType := ctx.typePrinter.InnerTypeString(desc); targetType != sourceType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// getIndexVar returns a unique index variable name
func (ctx *unmarshalContext) getIndexVar() (string, func()) {
	ctx.indexCounter++
	thisIndex := ctx.indexCounter
	return fmt.Sprintf("idx%d", thisIndex), func() {
		ctx.indexCounter = thisIndex - 1
	}
}

// isInlinable determines if a type can be unmarshaled inline without temporary variables.
func (ctx *unmarshalContext) isInlinable(desc *ssztypes.TypeDescriptor) bool {
	// Inline primitive types
	if desc.SszType == ssztypes.SszBoolType || desc.SszType == ssztypes.SszUint8Type || desc.SszType == ssztypes.SszUint16Type || desc.SszType == ssztypes.SszUint32Type || desc.SszType == ssztypes.SszUint64Type || desc.SszType == ssztypes.SszInt8Type || desc.SszType == ssztypes.SszInt16Type || desc.SszType == ssztypes.SszInt32Type || desc.SszType == ssztypes.SszInt64Type || desc.SszType == ssztypes.SszFloat32Type || desc.SszType == ssztypes.SszFloat64Type {
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

	return false
}

// unmarshalType generates unmarshal code for any SSZ type, delegating to specific unmarshalers.
func (ctx *unmarshalContext) unmarshalType(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int, isRoot, noBufCheck bool) error {
	// Handle types that have generated methods we can call
	hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	isFastsszUnmarshaler := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0
	useFastSsz := !ctx.options.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
	if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "if err = %s.UnmarshalSSZ(buf); err != nil {\n\treturn %s\n}\n", varName, typePath.getErrorWith("err"))
		return nil
	}

	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicUnmarshaler != 0 && !isRoot {
		ctx.appendCode(indent, "if err = %s.UnmarshalSSZDyn(ds, buf); err != nil {\n\treturn %s\n}\n", varName, typePath.getErrorWith("err"))
		ctx.usedDynSpecs = true
		return nil
	}

	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicDecoder != 0 && !isRoot {
		ctx.appendCode(indent, "dec := sszutils.NewBufferDecoder(buf)\n")
		ctx.appendCode(indent, "if err = %s.UnmarshalSSZDecoder(ds, dec); err != nil {\n\treturn %s\n}\n", varName, typePath.getErrorWith("err"))
		ctx.usedDynSpecs = true
		return nil
	}

	switch desc.SszType {
	case ssztypes.SszBoolType:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(1, \"bool\")"))
		}
		ctx.appendCode(indent, "if buf[0] != 1 && buf[0] != 0 {\n\treturn sszutils.ErrInvalidBoolValueFn()\n}\n")
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent, "%s = buf[0] == 1\n", ptrVarName)
	case ssztypes.SszUint8Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(1, \"uint8\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc, "buf[0]", "uint8"),
		)

	case ssztypes.SszUint16Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 2 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(2, \"uint16\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc,
				fmt.Sprintf("%s.LittleEndian.Uint16(buf)", ctx.typePrinter.AddImport("encoding/binary", "binary")),
				"uint16",
			),
		)

	case ssztypes.SszUint32Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 4 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(4, \"uint32\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc,
				fmt.Sprintf("%s.LittleEndian.Uint32(buf)", ctx.typePrinter.AddImport("encoding/binary", "binary")),
				"uint32",
			),
		)

	case ssztypes.SszUint64Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 8 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(8, \"uint64\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			ctx.appendCode(indent,
				"%s = %s\n",
				ptrVarName,
				ctx.getCastedValueVar(desc,
					fmt.Sprintf("time.Unix(int64(%s.LittleEndian.Uint64(buf)), 0).UTC()", ctx.typePrinter.AddImport("encoding/binary", "binary")),
					"time.Time",
				),
			)
			ctx.typePrinter.AddImport("time", "time")
		} else {
			ctx.appendCode(indent,
				"%s = %s\n",
				ptrVarName,
				ctx.getCastedValueVar(desc,
					fmt.Sprintf("%s.LittleEndian.Uint64(buf)", ctx.typePrinter.AddImport("encoding/binary", "binary")),
					"uint64",
				),
			)
		}

	case ssztypes.SszTypeWrapperType:
		if err := ctx.unmarshalType(desc.ElemDesc, fmt.Sprintf("%s.Data", varName), typePath, indent+1, false, noBufCheck); err != nil {
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
	case ssztypes.SszInt8Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(1, \"int8\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc, "int8(buf[0])", "int8"),
		)
	case ssztypes.SszInt16Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 2 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(2, \"int16\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		binaryImport := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc, fmt.Sprintf("int16(%s.LittleEndian.Uint16(buf))", binaryImport), "int16"),
		)
	case ssztypes.SszInt32Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 4 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(4, \"int32\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		binaryImport := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc, fmt.Sprintf("int32(%s.LittleEndian.Uint32(buf))", binaryImport), "int32"),
		)
	case ssztypes.SszInt64Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 8 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(8, \"int64\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		binaryImport := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc, fmt.Sprintf("int64(%s.LittleEndian.Uint64(buf))", binaryImport), "int64"),
		)
	case ssztypes.SszFloat32Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 4 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(4, \"float32\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		mathImport := ctx.typePrinter.AddImport("math", "math")
		binaryImport := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc, fmt.Sprintf("%s.Float32frombits(%s.LittleEndian.Uint32(buf))", mathImport, binaryImport), "float32"),
		)
	case ssztypes.SszFloat64Type:
		if !noBufCheck {
			ctx.appendCode(indent, "if len(buf) < 8 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrNeedBytesFn(8, \"float64\")"))
		}
		ptrVarName := varName
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ptrVarName = fmt.Sprintf("*(%s)", varName)
		}
		mathImport := ctx.typePrinter.AddImport("math", "math")
		binaryImport := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent,
			"%s = %s\n",
			ptrVarName,
			ctx.getCastedValueVar(desc, fmt.Sprintf("%s.Float64frombits(%s.LittleEndian.Uint64(buf))", mathImport, binaryImport), "float64"),
		)
	case ssztypes.SszOptionalType:
		return ctx.unmarshalOptional(desc, varName, typePath, indent)
	case ssztypes.SszBigIntType:
		return ctx.unmarshalBigInt(desc, varName, indent)

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// unmarshalOptional generates unmarshal code for SSZ optional types.
func (ctx *unmarshalContext) unmarshalOptional(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn %s\n}\n", typePath.getErrorWith("sszutils.ErrOptionalFlagEOFFn()"))
	ctx.appendCode(indent, "if buf[0] == 1 {\n")

	// Check that buf has enough bytes for the presence flag plus the value
	elemSize := desc.ElemDesc.Size
	if elemSize > 0 {
		ctx.appendCode(indent+1, "if len(buf) < %d {\n\treturn %s\n}\n", 1+elemSize, typePath.getErrorWith("sszutils.ErrOptionalValueEOFFn()"))
	}

	valVar := ctx.getValVar()
	ctx.appendCode(indent+1, "var %s %s\n", valVar, ctx.typePrinter.TypeString(desc.ElemDesc))
	ctx.appendCode(indent+1, "buf := buf[1:]\n")
	if err := ctx.unmarshalType(desc.ElemDesc, valVar, typePath, indent+1, false, true); err != nil {
		return err
	}
	ctx.appendCode(indent+1, "%s = &%s\n", varName, valVar)
	ctx.appendCode(indent, "} else {\n")
	ctx.appendCode(indent+1, "%s = nil\n", varName)
	ctx.appendCode(indent, "}\n")
	return nil
}

// unmarshalBigInt generates unmarshal code for SSZ big int types.
func (ctx *unmarshalContext) unmarshalBigInt(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	bigImport := ctx.typePrinter.AddImport("math/big", "big")
	ptrVarName := varName
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		ptrVarName = fmt.Sprintf("*(%s)", varName)
	}
	ctx.appendCode(indent,
		"%s = %s\n",
		ptrVarName,
		ctx.getCastedValueVar(desc, fmt.Sprintf("*(%s.NewInt(0).SetBytes(buf))", bigImport), "big.Int"),
	)
	return nil
}

// unmarshalContainer generates unmarshal code for SSZ container (struct) types.
func (ctx *unmarshalContext) unmarshalContainer(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	staticSize := 0
	staticSizeVars := []string{}
	for _, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			staticSize += 4
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

	totalStaticSizeExpr := strings.Join(staticSizeVars, "+")
	if len(staticSizeVars) > 1 {
		ctx.appendCode(indent, "exproffset := 0\n")
		ctx.appendCode(indent, "totalSize := %s\n", totalStaticSizeExpr)
		totalStaticSizeExpr = "totalSize"
	}

	// Read fixed fields and offsets
	offset := 0
	offsetPrefix := ""
	ctx.appendCode(indent, "buflen := len(buf)\n")
	errCode := fmt.Sprintf("sszutils.ErrFixedFieldsEOFFn(buflen, %s)", totalStaticSizeExpr)
	ctx.appendCode(indent, "if buflen < %s {\n\treturn %s\n}\n", totalStaticSizeExpr, typePath.getErrorWith(errCode))
	dynamicFields := make([]int, 0)

	for idx, field := range desc.ContainerDesc.Fields {
		if field.Type.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			// Read offset
			ctx.appendCode(indent, "// Field #%d '%s' (offset)\n", idx, field.Name)
			fmtSpace := ""
			if offsetPrefix != "" {
				fmtSpace = " "
			}
			binaryPkgName := ctx.typePrinter.AddImport("encoding/binary", "binary")
			ctx.appendCode(indent, "offset%d := int(%s.LittleEndian.Uint32(buf[%s%d%s:%s%s%d]))\n", idx, binaryPkgName, offsetPrefix, offset, fmtSpace, fmtSpace, offsetPrefix, offset+4)
			fieldOffsetPath := typePath.append(fmt.Sprintf("%s:o", field.Name))
			if len(dynamicFields) > 0 {
				errCode = fmt.Sprintf("sszutils.ErrOffsetOutOfRangeFn(offset%d, offset%d, buflen)", idx, dynamicFields[len(dynamicFields)-1])
				ctx.appendCode(indent, "if offset%d < offset%d || offset%d > buflen {\n\treturn %s\n}\n", idx, dynamicFields[len(dynamicFields)-1], idx, fieldOffsetPath.getErrorWith(errCode))
			} else {
				errCode = fmt.Sprintf("sszutils.ErrFirstOffsetMismatchFn(offset%d, %s)", idx, totalStaticSizeExpr)
				ctx.appendCode(indent, "if offset%d != %s {\n\treturn %s\n}\n", idx, totalStaticSizeExpr, fieldOffsetPath.getErrorWith(errCode))
			}
			offset += 4
			dynamicFields = append(dynamicFields, idx)
		} else {
			// Unmarshal fixed field
			ctx.appendCode(indent, "{ // Field #%d '%s' (static)\n", idx, field.Name)
			if field.Type.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions {
				fieldSizeVar, err := ctx.staticSizeVars.getStaticSizeVar(field.Type)
				if err != nil {
					return err
				}
				ctx.appendCode(indent, "\tbuf := buf[%s%d : %s%s+%d]\n", offsetPrefix, offset, offsetPrefix, fieldSizeVar, offset)
				ctx.appendCode(indent, "\texproffset += int(%s)\n", fieldSizeVar)
				offsetPrefix = "exproffset+"
			} else {
				fmtSpace := ""
				if offsetPrefix != "" {
					fmtSpace = " "
				}
				ctx.appendCode(indent, "\tbuf := buf[%s%d%s:%s%s%d]\n", offsetPrefix, offset, fmtSpace, fmtSpace, offsetPrefix, offset+int(field.Type.Size))
				offset += int(field.Type.Size)
			}

			valVar := fmt.Sprintf("%s.%s", varName, field.Name)
			isInlinable := ctx.isInlinable(field.Type)
			if !isInlinable {
				valVar = ctx.getValVar()
				ctx.appendCode(indent, "\t%s := %s.%s\n", valVar, varName, field.Name)
			}
			if field.Type.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
				ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(field.Type))
			}

			if err := ctx.unmarshalType(field.Type, valVar, typePath.append(field.Name), indent+1, false, true); err != nil {
				return err
			}

			if !isInlinable {
				ctx.appendCode(indent, "\t%s.%s = %s\n", varName, field.Name, valVar)
			}
			ctx.appendCode(indent, "}\n")
		}
	}

	// Read dynamic fields
	for idx, fieldIdx := range dynamicFields {
		field := desc.ContainerDesc.Fields[fieldIdx]
		ctx.appendCode(indent, "{ // Field #%d '%s' (dynamic)\n", fieldIdx, field.Name)

		endOffset := ""
		if idx < len(dynamicFields)-1 {
			endOffset = fmt.Sprintf("offset%d", dynamicFields[idx+1])
		}
		ctx.appendCode(indent, "\tbuf := buf[offset%d:%s]\n", fieldIdx, endOffset)

		valVar := fmt.Sprintf("%s.%s", varName, field.Name)
		isInlinable := ctx.isInlinable(field.Type)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent, "\t%s := %s.%s\n", valVar, varName, field.Name)
		}

		if field.Type.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(field.Type))
		}

		if err := ctx.unmarshalType(field.Type, valVar, typePath.append(field.Name), indent+1, false, true); err != nil {
			return err
		}

		if !isInlinable {
			ctx.appendCode(indent, "\t%s.%s = %s\n", varName, field.Name, valVar)
		}
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// unmarshalVector generates unmarshal code for SSZ vector (fixed-size array) types.
func (ctx *unmarshalContext) unmarshalVector(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int, noBufCheck bool) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	bitlimitVar := ""
	needExpression := desc.GoTypeFlags&ssztypes.GoTypeFlagIsString == 0 || !noBufCheck

	if sizeExpression != nil && needExpression {
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

	// create slice if needed
	if desc.Kind != reflect.Array && desc.GoTypeFlags&ssztypes.GoTypeFlagIsString == 0 {
		ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, %s)\n", varName, varName, limitVar)
	}

	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
			if !noBufCheck {
				errCode := fmt.Sprintf("sszutils.ErrByteVectorEOFFn(len(buf), %s)", limitVar)
				ctx.appendCode(indent, "if %s > len(buf) {\n\treturn %s\n}\n", limitVar, typePath.getErrorWith(errCode))
			}
			if bitlimitVar != "" {
				ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
				ctx.appendCode(indent, "if buf[%s-1] & paddingMask != 0 {\n", limitVar)
				errCode := errCodeBitvectorPadding
				ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
				ctx.appendCode(indent, "}\n")
			}
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				ptrVarName := varName
				if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
					ptrVarName = fmt.Sprintf("*(%s)", varName)
				}
				typename := ctx.typePrinter.InnerTypeString(desc)
				ctx.appendCode(indent, "%s = %s(buf)\n", ptrVarName, typename)
			} else {
				ctx.appendCode(indent, "copy(%s[:], buf)\n", varName)
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
			ctx.appendCode(indent, "if %s*%s > len(buf) {\n", limitVar, fieldSizeVar)
			errCode := fmt.Sprintf("sszutils.ErrVectorElementsEOFFn(len(buf), %s*%s)", limitVar, fieldSizeVar)
			ctx.appendCode(indent+1, "return %s\n", typePath.getErrorWith(errCode))
			ctx.appendCode(indent, "}\n")
		}

		// bulk uint64 lists
		if desc.ElemDesc.SszType == ssztypes.SszUint64Type && desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
			ctx.appendCode(indent, "sszutils.UnmarshalUint64Slice(%s[:%s], buf)\n", varName, limitVar)
			return nil
		}

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "for %s := range %s {\n", indexVar, limitVar)

		valVar := fmt.Sprintf("%s[%s]", varName, indexVar)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent+1, "%s := %s[%s]\n", valVar, varName, indexVar)
		}
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		ctx.appendCode(indent, "\tbuf := buf[%s*%s : %s*(%s+1)]\n", fieldSizeVar, indexVar, fieldSizeVar, indexVar)
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, typePath.append("[%d]", indexVar), indent+1, false, true); err != nil {
			return err
		}

		if !isInlinable {
			ctx.appendCode(indent, "\t%s[%s] = %s\n", varName, indexVar, valVar)
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// dynamic elements
		binaryPkgName := ctx.typePrinter.AddImport("encoding/binary", "binary")
		errCode := fmt.Sprintf("sszutils.ErrVectorOffsetsEOFFn(len(buf), %s*4)", limitVar)
		ctx.appendCode(indent, "if %s*4 > len(buf) {\n\treturn %s\n}\n", limitVar, typePath.getErrorWith(errCode))
		ctx.appendCode(indent, "startOffset := int(%s.LittleEndian.Uint32(buf[0:4]))\n", binaryPkgName)

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "for %s := range %s {\n", indexVar, limitVar)
		ctx.appendCode(indent, "\tvar endOffset int\n")
		ctx.appendCode(indent, "\tif %s < %s-1 {\n", indexVar, limitVar)
		ctx.appendCode(indent, "\t\tendOffset = int(%s.LittleEndian.Uint32(buf[(%s+1)*4 : (%s+2)*4]))\n", binaryPkgName, indexVar, indexVar)
		ctx.appendCode(indent, "\t} else {\n")
		ctx.appendCode(indent, "\t\tendOffset = len(buf)\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tif endOffset < startOffset || endOffset > len(buf) {\n")
		errCode = "sszutils.ErrElementOffsetOutOfRangeFn(endOffset, startOffset, len(buf))"
		ctx.appendCode(indent, "\t\treturn %s\n", typePath.getErrorWith(errCode))
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tbuf := buf[startOffset:endOffset]\n")
		ctx.appendCode(indent, "\tstartOffset = endOffset\n")

		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\t%s := %s[%s]\n", valVar, varName, indexVar)
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, typePath.append("[%d]", indexVar), indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t%s[%s] = %s\n", varName, indexVar, valVar)
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// unmarshalList generates unmarshal code for SSZ list (variable-size array) types.
func (ctx *unmarshalContext) unmarshalList(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	if desc.ElemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		// static byte arrays
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				ptrVarName := varName
				if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
					ptrVarName = fmt.Sprintf("*(%s)", varName)
				}
				typename := ctx.typePrinter.InnerTypeString(desc)
				ctx.appendCode(indent, "%s = %s(buf)\n", ptrVarName, typename)
			} else {
				if desc.Kind != reflect.Array {
					ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, len(buf))\n", varName, varName)
				}
				ctx.appendCode(indent, "copy(%s[:], buf)\n", varName)
			}
			return nil
		}

		// bulk uint64 lists
		if desc.ElemDesc.SszType == ssztypes.SszUint64Type && desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
			ctx.appendCode(indent, "itemCount := len(buf) / 8\n")
			errCode := "sszutils.ErrListNotAlignedFn(len(buf), 8)"
			ctx.appendCode(indent, "if len(buf)%%8 != 0 {\n\treturn %s\n}\n", typePath.getErrorWith(errCode))
			if desc.Kind != reflect.Array {
				ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", varName, varName)
			}
			ctx.appendCode(indent, "sszutils.UnmarshalUint64Slice(%s, buf)\n", varName)
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
			ctx.appendCode(indent, "itemCount := len(buf)\n")
		} else {
			ctx.appendCode(indent, "itemCount := len(buf) / %s\n", fieldSizeVar)
			errCode := fmt.Sprintf("sszutils.ErrListNotAlignedFn(len(buf), %s)", fieldSizeVar)
			ctx.appendCode(indent, "if len(buf)%%%s != 0 {\n\treturn %s\n}\n", fieldSizeVar, typePath.getErrorWith(errCode))
		}
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", varName, varName)
		}

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "for %s := range itemCount {\n", indexVar)

		valVar := fmt.Sprintf("%s[%s]", varName, indexVar)
		isInlinable := ctx.isInlinable(desc.ElemDesc)
		if !isInlinable {
			valVar = ctx.getValVar()
			ctx.appendCode(indent, "\t%s := %s[%s]\n", valVar, varName, indexVar)
		}
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}

		ctx.appendCode(indent, "\tbuf := buf[%s*%s : %s*(%s+1)]\n", fieldSizeVar, indexVar, fieldSizeVar, indexVar)
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, typePath.append("[%d]", indexVar), indent+1, false, true); err != nil {
			return err
		}

		if !isInlinable {
			ctx.appendCode(indent, "\t%s[%s] = %s\n", varName, indexVar, valVar)
		}
		ctx.appendCode(indent, "}\n")
	} else {
		// dynamic elements
		binaryPkgName := ctx.typePrinter.AddImport("encoding/binary", "binary")
		ctx.appendCode(indent, "startOffset := int(0)\n")
		ctx.appendCode(indent, "if len(buf) != 0 {\n")
		errCode := "sszutils.ErrListOffsetsEOFFn(len(buf), 4)"
		ctx.appendCode(indent, "\tif len(buf) < 4 {\n\t\treturn %s\n\t}\n", typePath.getErrorWith(errCode))
		ctx.appendCode(indent, "\tstartOffset = int(%s.LittleEndian.Uint32(buf[0:4]))\n", binaryPkgName)
		ctx.appendCode(indent, "}\n")
		ctx.appendCode(indent, "itemCount := startOffset / 4\n")
		errCode = "sszutils.ErrInvalidListStartOffsetFn(startOffset, len(buf))"
		ctx.appendCode(indent, "if startOffset%%4 != 0 || len(buf) < startOffset {\n\treturn %s\n}\n", typePath.getErrorWith(errCode))
		if desc.Kind != reflect.Array {
			ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, itemCount)\n", varName, varName)
		}

		indexVar, indexDefer := ctx.getIndexVar()
		defer indexDefer()

		ctx.appendCode(indent, "for %s := range itemCount {\n", indexVar)
		ctx.appendCode(indent, "\tvar endOffset int\n")
		ctx.appendCode(indent, "\tif %s < itemCount-1 {\n", indexVar)
		ctx.appendCode(indent, "\t\tendOffset = int(%s.LittleEndian.Uint32(buf[(%s+1)*4 : (%s+2)*4]))\n", binaryPkgName, indexVar, indexVar)
		ctx.appendCode(indent, "\t} else {\n")
		ctx.appendCode(indent, "\t\tendOffset = len(buf)\n")
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tif endOffset < startOffset || endOffset > len(buf) {\n")
		childTypePath := typePath.append("[%d]", indexVar)
		errCode = "sszutils.ErrElementOffsetOutOfRangeFn(endOffset, startOffset, len(buf))"
		ctx.appendCode(indent, "\t\treturn %s\n", childTypePath.getErrorWith(errCode))
		ctx.appendCode(indent, "\t}\n")
		ctx.appendCode(indent, "\tbuf := buf[startOffset:endOffset]\n")
		ctx.appendCode(indent, "\tstartOffset = endOffset\n")
		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\t%s := %s[%s]\n", valVar, varName, indexVar)
		if desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
			ctx.appendCode(indent+1, "if %s == nil {\n\t%s = new(%s)\n}\n", valVar, valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		}
		if err := ctx.unmarshalType(desc.ElemDesc, valVar, childTypePath, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t%s[%s] = %s\n", varName, indexVar, valVar)
		ctx.appendCode(indent, "}\n")
	}

	return nil
}

// unmarshalBitlist generates unmarshal code for SSZ bitlist types.
func (ctx *unmarshalContext) unmarshalBitlist(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	ctx.appendCode(indent, "blen := len(buf)\n")
	ctx.appendCode(indent, "if blen == 0 || buf[blen-1] == 0x00 {\n")
	errCode := errCodeBitlistNotTerminated
	ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
	ctx.appendCode(indent, "}\n")

	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "%s = sszutils.ExpandSlice(%s, blen)\n", varName, varName)
	}
	ctx.appendCode(indent, "copy(%s[:], buf)\n", varName)

	return nil
}

// unmarshalUnion generates unmarshal code for SSZ union types.
func (ctx *unmarshalContext) unmarshalUnion(desc *ssztypes.TypeDescriptor, varName string, typePath typePathList, indent int) error {
	// Read selector
	errCode := "sszutils.ErrUnionSelectorEOFFn()"
	ctx.appendCode(indent, "if len(buf) < 1 {\n\treturn %s\n}\n", typePath.getErrorWith(errCode))
	ctx.appendCode(indent, "selector := buf[0]\n")
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

		childTypePath := typePath.append(fmt.Sprintf("[v:%d]", variant))

		// Check that buf has enough bytes for the selector plus the variant value
		elemSize := variantDesc.Size
		if elemSize > 0 {
			errCode = fmt.Sprintf("sszutils.ErrUnionVariantEOFFn(len(buf), %d)", 1+elemSize)
			ctx.appendCode(indent+1, "if len(buf) < %d {\n\treturn %s\n}\n", 1+elemSize, childTypePath.getErrorWith(errCode))
		}

		valVar := ctx.getValVar()
		ctx.appendCode(indent, "\tvar %s %s\n", valVar, variantType)
		ctx.appendCode(indent, "\tbuf := buf[1:]\n")
		if err := ctx.unmarshalType(variantDesc, valVar, childTypePath, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "\t%s.Data = %s\n", varName, valVar)
	}

	ctx.appendCode(indent, "default:\n")
	errCode = errCodeInvalidUnionVariant
	ctx.appendCode(indent, "\treturn %s\n", typePath.getErrorWith(errCode))
	ctx.appendCode(indent, "}\n")

	return nil
}
