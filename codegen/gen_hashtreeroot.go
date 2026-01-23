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

// hashTreeRootContext contains the state and utilities for generating hash tree root methods.
//
// This context structure maintains the necessary state during the hash tree root code
// generation process, including code building utilities, variable management, and
// options that control the generation behavior.
//
// Fields:
//   - appendCode: Function to append formatted code with proper indentation
//   - typePrinter: Type name formatter and import tracker
//   - options: Code generation options controlling output behavior
//   - usedDynSpecs: Flag tracking whether generated code uses dynamic SSZ functionality
//   - valVarCounter: Counter for generating unique variable names during code generation
type hashTreeRootContext struct {
	appendCode    func(indent int, code string, args ...any)
	typePrinter   *TypePrinter
	options       *CodeGeneratorOptions
	exprVars      *exprVarGenerator
	usedDynSpecs  bool
	valVarCounter int
}

// generateHashTreeRoot generates hash tree root methods for a specific type.
//
// This function creates the complete set of hash tree root methods for a type, including:
//   - HashTreeRootWithDyn for dynamic specification support
//   - HashTreeRootWith for static/legacy compatibility
//   - HashTreeRoot for legacy fastssz compatibility (if requested)
//
// The generated methods compute SSZ hash tree roots according to the SSZ specification,
// handling nested structures, variable-length fields, and proper Merkle tree
// construction. Hash tree roots are essential for cryptographic commitments
// and Merkle proof generation.
//
// Parameters:
//   - rootTypeDesc: Type descriptor containing complete SSZ hashing metadata
//   - codeBuilder: String builder to append generated method code to
//   - typePrinter: Type formatter for handling imports and type names
//   - viewName: Name of the view type for function name postfix (empty string for data type)
//   - options: Generation options controlling which methods to create
//
// Returns:
//   - error: An error if code generation fails
func generateHashTreeRoot(rootTypeDesc *ssztypes.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, viewName string, options *CodeGeneratorOptions) error {
	codeBuf := strings.Builder{}
	ctx := &hashTreeRootContext{
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

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate hash tree root code
	if err := ctx.hashType(rootTypeDesc, "t", 0, true, false); err != nil {
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

	// Static hash tree root function
	if genStaticFn {
		hasherAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz/hasher", "hasher")
		appendCode(codeBuilder, 0, "func (t %s) HashTreeRoot() (root [32]byte, err error) {\n", typeName)
		appendCode(codeBuilder, 1, "err = %s.WithDefaultHasher(func(hh sszutils.HashWalker) (err error) {\n", hasherAlias)
		appendCode(codeBuilder, 2, "err = t.HashTreeRootWith(hh)\n")
		appendCode(codeBuilder, 2, "if err == nil {\n")
		appendCode(codeBuilder, 3, "root, err = hh.HashRoot()\n")
		appendCode(codeBuilder, 2, "}\n")
		appendCode(codeBuilder, 2, "return\n")
		appendCode(codeBuilder, 1, "})\n")
		appendCode(codeBuilder, 1, "return\n")
		appendCode(codeBuilder, 0, "}\n")
	}

	if genStaticFn {
		if !ctx.usedDynSpecs {
			appendCode(codeBuilder, 0, fmt.Sprintf("func (t %s) HashTreeRootWith(hh sszutils.HashWalker) error {\n", typeName))
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, codeBuf.String())
			appendCode(codeBuilder, 1, "return nil\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			appendCode(codeBuilder, 0, "func (t %s) HashTreeRootWith(hh sszutils.HashWalker) error {\n", typeName)
			appendCode(codeBuilder, 1, "return t.HashTreeRootWithDyn(%s.GetGlobalDynSsz(), hh)\n", dynsszAlias)
			appendCode(codeBuilder, 0, "}\n\n")
		}
	}

	// Dynamic hash tree root function
	if genDynamicFn && viewName == "" {
		hasherAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz/hasher", "hasher")
		appendCode(codeBuilder, 0, "func (t %s) HashTreeRootDyn(ds sszutils.DynamicSpecs) (root [32]byte, err error) {\n", typeName)
		appendCode(codeBuilder, 1, "err = %s.WithDefaultHasher(func(hh sszutils.HashWalker) (err error) {\n", hasherAlias)
		appendCode(codeBuilder, 2, "err = t.HashTreeRootWithDyn(ds, hh)\n")
		appendCode(codeBuilder, 2, "if err == nil {\n")
		appendCode(codeBuilder, 3, "root, err = hh.HashRoot()\n")
		appendCode(codeBuilder, 2, "}\n")
		appendCode(codeBuilder, 2, "return\n")
		appendCode(codeBuilder, 1, "})\n")
		appendCode(codeBuilder, 1, "return\n")
		appendCode(codeBuilder, 0, "}\n\n")
	}

	if genDynamicFn {
		fnName := "HashTreeRootWithDyn"
		if viewName != "" {
			fnName = fmt.Sprintf("hashTreeRootView_%s", viewName)
		}
		if ctx.usedDynSpecs || viewName != "" {
			appendCode(codeBuilder, 0, "func (t %s) %s(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {\n", typeName, fnName)
			appendCode(codeBuilder, 1, ctx.exprVars.getCode())
			appendCode(codeBuilder, 1, codeBuf.String())
			appendCode(codeBuilder, 1, "return nil\n")
			appendCode(codeBuilder, 0, "}\n\n")
		} else {
			appendCode(codeBuilder, 0, "func (t %s) %s(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {\n", typeName, fnName)
			appendCode(codeBuilder, 1, "return t.HashTreeRootWith(hh)\n")
			appendCode(codeBuilder, 0, "}\n\n")
			genStaticFn = true
		}
	}

	return nil
}

// isPrimitive checks if a type is a primitive SSZ type that can be hashed directly.
func (ctx *hashTreeRootContext) isPrimitive(desc *ssztypes.TypeDescriptor) bool {
	return desc.SszType == ssztypes.SszBoolType || desc.SszType == ssztypes.SszUint8Type || desc.SszType == ssztypes.SszUint16Type || desc.SszType == ssztypes.SszUint32Type || desc.SszType == ssztypes.SszUint64Type || desc.SszType == ssztypes.SszUint128Type
}

// isInlineable checks if a type can be inlined directly into the hash tree root code
func (ctx *hashTreeRootContext) isInlineable(desc *ssztypes.TypeDescriptor) bool {
	if desc.SszType == ssztypes.SszBoolType || desc.SszType == ssztypes.SszUint8Type || desc.SszType == ssztypes.SszUint16Type || desc.SszType == ssztypes.SszUint32Type || desc.SszType == ssztypes.SszUint64Type {
		return true
	}

	if desc.SszType == ssztypes.SszVectorType || desc.SszType == ssztypes.SszListType {
		return desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0
	}

	return false
}

// getValVar generates a unique variable name for temporary values.
func (ctx *hashTreeRootContext) getValVar() string {
	ctx.valVarCounter++
	return fmt.Sprintf("val%d", ctx.valVarCounter)
}

// getValueVar returns the variable name for the value of a type, dereferencing pointer types and converting to the target type if needed
func (ctx *hashTreeRootContext) getValueVar(desc *ssztypes.TypeDescriptor, varName string, targetType string) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
		varName = fmt.Sprintf("*%s", varName)
	}

	if targetType != "" && ctx.typePrinter.InnerTypeString(desc) != targetType {
		varName = fmt.Sprintf("%s(%s)", targetType, varName)
	}

	return varName
}

// getPtrPrefix returns & for types that are heavy to copy
func (ctx *hashTreeRootContext) getPtrPrefix(desc *ssztypes.TypeDescriptor, prefix string) string {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		return ""
	}
	if desc.Kind == reflect.Array {
		return prefix
	}
	if desc.Kind == reflect.Struct {
		// use pointer to struct to avoid copying
		return prefix
	}
	return ""
}

// hashType generates hash tree root code for any SSZ type, delegating to specific hashers.
func (ctx *hashTreeRootContext) hashType(desc *ssztypes.TypeDescriptor, varName string, indent int, isRoot bool, pack bool) error {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		ctx.appendCode(indent, "if %s == nil {\n\t%s = new(%s)\n}\n", varName, varName, ctx.typePrinter.InnerTypeString(desc))
	}

	// Handle types that have generated methods we can call
	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicHashRoot != 0 && !isRoot {
		ctx.appendCode(indent, "if err := %s.HashTreeRootWithDyn(ds, hh); err != nil {\n\treturn err\n}\n", varName)
		ctx.usedDynSpecs = true
		return nil
	}

	isFastsszHasher := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZHasher != 0
	isFastsszHashWith := desc.SszCompatFlags&ssztypes.SszCompatFlagHashTreeRootWith != 0
	hasDynamicSize := desc.SszTypeFlags&ssztypes.SszTypeFlagHasSizeExpr != 0 && !ctx.options.WithoutDynamicExpressions
	hasDynamicMax := desc.SszTypeFlags&ssztypes.SszTypeFlagHasMaxExpr != 0 && !ctx.options.WithoutDynamicExpressions

	useFastSsz := !isRoot && !ctx.options.NoFastSsz && !hasDynamicSize && !hasDynamicMax && (isFastsszHasher || isFastsszHashWith)
	if !useFastSsz && desc.SszType == ssztypes.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz {
		if isFastsszHashWith {
			ctx.appendCode(indent, "if err := %s.HashTreeRootWith(hh); err != nil {\n\treturn err\n}\n", varName)
		} else {
			ctx.appendCode(indent, "if root, err := %s.HashTreeRoot(); err != nil {\n\treturn err\n} else {\n\thh.AppendBytes32(root[:])\n}\n", varName)
		}
		return nil
	}

	switch desc.SszType {
	case ssztypes.SszBoolType:
		if pack {
			ctx.appendCode(indent, "hh.AppendBool(%s)\n", ctx.getValueVar(desc, varName, "bool"))
		} else {
			ctx.appendCode(indent, "hh.PutBool(%s)\n", ctx.getValueVar(desc, varName, "bool"))
		}
	case ssztypes.SszUint8Type:
		if pack {
			ctx.appendCode(indent, "hh.AppendUint8(%s)\n", ctx.getValueVar(desc, varName, "uint8"))
		} else {
			ctx.appendCode(indent, "hh.PutUint8(%s)\n", ctx.getValueVar(desc, varName, "uint8"))
		}
	case ssztypes.SszUint16Type:
		if pack {
			ctx.appendCode(indent, "hh.AppendUint16(%s)\n", ctx.getValueVar(desc, varName, "uint16"))
		} else {
			ctx.appendCode(indent, "hh.PutUint16(%s)\n", ctx.getValueVar(desc, varName, "uint16"))
		}

	case ssztypes.SszUint32Type:
		if pack {
			ctx.appendCode(indent, "hh.AppendUint32(%s)\n", ctx.getValueVar(desc, varName, "uint32"))
		} else {
			ctx.appendCode(indent, "hh.PutUint32(%s)\n", ctx.getValueVar(desc, varName, "uint32"))
		}

	case ssztypes.SszUint64Type:
		var valVar string
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			valVar = ctx.getValueVar(desc, fmt.Sprintf("%s.Unix()", varName), "uint64")
		} else {
			valVar = ctx.getValueVar(desc, varName, "uint64")
		}
		if pack {
			ctx.appendCode(indent, "hh.AppendUint64(%s)\n", valVar)
		} else {
			ctx.appendCode(indent, "hh.PutUint64(%s)\n", valVar)
		}

	case ssztypes.SszTypeWrapperType:
		ctx.appendCode(indent, "{\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s.Data", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s.Data\n", ctx.getPtrPrefix(desc.ElemDesc, "&"), varName)
		}
		if err := ctx.hashType(desc.ElemDesc, valVar, indent+1, false, pack); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

	case ssztypes.SszContainerType:
		return ctx.hashContainer(desc, varName, indent)

	case ssztypes.SszProgressiveContainerType:
		return ctx.hashProgressiveContainer(desc, varName, indent)

	case ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return ctx.hashVector(desc, varName, indent, true)

	case ssztypes.SszVectorType, ssztypes.SszBitvectorType:
		return ctx.hashVector(desc, varName, indent, false)

	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		return ctx.hashList(desc, varName, indent)

	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		return ctx.hashBitlist(desc, varName, indent)

	case ssztypes.SszCompatibleUnionType:
		return ctx.hashUnion(desc, varName, indent)

	case ssztypes.SszCustomType:
		ctx.appendCode(indent, "// Custom type - hash unknown\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

// hashContainer generates hash tree root code for SSZ container (struct) types.
func (ctx *hashTreeRootContext) hashContainer(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	// Start container merkleization
	ctx.appendCode(indent, "idx := hh.Index()\n")

	// Hash each field
	for idx, field := range desc.ContainerDesc.Fields {
		ctx.appendCode(indent, "{ // Field #%d '%s'\n", idx, field.Name)
		valVar := "t"
		if ctx.isInlineable(field.Type) {
			valVar = fmt.Sprintf("%s.%s", varName, field.Name)
		} else {
			ctx.appendCode(indent, "\tt := %s%s.%s\n", ctx.getPtrPrefix(field.Type, "&"), varName, field.Name)
		}
		if err := ctx.hashType(field.Type, valVar, indent+1, false, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	// Finalize container
	ctx.appendCode(indent, "hh.Merkleize(idx)\n")

	return nil
}

// hashProgressiveContainer generates hash tree root code for progressive container types.
func (ctx *hashTreeRootContext) hashProgressiveContainer(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	// Start container merkleization
	ctx.appendCode(indent, "idx := hh.Index()\n")

	// Hash each field
	lastActiveField := -1

	for i := 0; i < len(desc.ContainerDesc.Fields); i++ {
		field := desc.ContainerDesc.Fields[i]

		if int(field.SszIndex) > lastActiveField+1 {
			// fill the gap with empty fields
			for j := lastActiveField + 1; j < int(field.SszIndex); j++ {
				ctx.appendCode(indent, "// Inactive field #%d\n", j)
				ctx.appendCode(indent, "hh.PutUint8(0)\n")
			}
		}

		lastActiveField = int(field.SszIndex)

		ctx.appendCode(indent, "{ // Field #%d '%s'\n", i, field.Name)
		valVar := "t"
		if ctx.isInlineable(field.Type) {
			valVar = fmt.Sprintf("%s.%s", varName, field.Name)
		} else {
			ctx.appendCode(indent, "\tt := %s%s.%s\n", ctx.getPtrPrefix(field.Type, "&"), varName, field.Name)
		}
		if err := ctx.hashType(field.Type, valVar, indent+1, false, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	activeFields := ctx.getActiveFieldsHex(desc)
	ctx.appendCode(indent, "hh.MerkleizeProgressiveWithActiveFields(idx, %s)\n", activeFields)

	return nil
}

// getActiveFieldsHex generates hex string representation of active fields for progressive containers.
func (ctx *hashTreeRootContext) getActiveFieldsHex(sourceType *ssztypes.TypeDescriptor) string {
	// Find the highest ssz-index to determine bitlist size
	maxIndex := uint16(0)
	for _, field := range sourceType.ContainerDesc.Fields {
		if field.SszIndex > maxIndex {
			maxIndex = field.SszIndex
		}
	}

	// Create bitlist with enough bytes to hold maxIndex+1 bits
	bytesNeeded := (int(maxIndex) + 8) / 8 // +7 for rounding up, +1 already included in maxIndex
	activeFields := make([]byte, bytesNeeded)

	// Set most significant bit for length bit
	i := uint8(1 << (maxIndex % 8))
	activeFields[maxIndex/8] |= i

	// Set bit for each field that has an ssz-index
	for _, field := range sourceType.ContainerDesc.Fields {
		byteIndex := field.SszIndex / 8
		bitIndex := field.SszIndex % 8
		if int(byteIndex) < len(activeFields) {
			activeFields[byteIndex] |= (1 << bitIndex)
		}
	}

	// Convert to hex string
	hex := "[]byte{"
	for i, b := range activeFields {
		if i > 0 {
			hex += ", "
		}
		hex += fmt.Sprintf("0x%02x", b)
	}
	hex += "}"
	return hex
}

// hashVector generates hash tree root code for SSZ vector (fixed-size array) types.
func (ctx *hashTreeRootContext) hashVector(desc *ssztypes.TypeDescriptor, varName string, indent int, pack bool) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	bitlimitVar := ""
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
			bitlimitVar = fmt.Sprintf("int(%s)", exprVar)
			limitVar = fmt.Sprintf("int((%s+7)/8)", exprVar)
		} else {
			limitVar = fmt.Sprintf("int(%s)", exprVar)
		}

		if desc.Kind == reflect.Array {
			// check if dynamic limit is greater than the length of the array
			ctx.appendCode(indent, "if %s > %d {\n", limitVar, desc.Len)
			ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
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
		ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
		ctx.appendCode(indent, "}\n")
		lenVar = "vlen"
	} else {
		lenVar = fmt.Sprintf("%d", desc.Len)
	}

	itemSize := 0

	// Handle byte arrays
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 || desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
		valVar := ""
		if desc.Kind != reflect.Array {
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
				ctx.appendCode(indent, "val := []byte(%s)\n", valueVar)
			} else {
				if strings.HasPrefix(valueVar, "*") {
					valueVar = fmt.Sprintf("(%s)", valueVar)
				}
				ctx.appendCode(indent, "val := %s[:]\n", valueVar)
			}
			valVar = "val"

			// append zero padding if we have less items than the limit
			ctx.appendCode(indent, "if %s < %s {\n", lenVar, limitVar)
			ctx.appendCode(indent, "\tval = sszutils.AppendZeroPadding(val, (%s-%s)*%d)\n", limitVar, lenVar, desc.ElemDesc.Size)
			ctx.appendCode(indent, "}\n")
		} else {
			valVar = varName
		}

		if bitlimitVar != "" {
			// check padding bits
			ctx.appendCode(indent, "paddingMask := uint8((uint16(0xff) << (%s %% 8)) & 0xff)\n", bitlimitVar)
			ctx.appendCode(indent, "if %s[%s-1] & paddingMask != 0 {\n", valVar, limitVar)
			ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
			ctx.appendCode(indent, "}\n")
		}

		if pack {
			ctx.appendCode(indent, "hh.Append(%s[:%s])\n", valVar, limitVar)
		} else {
			ctx.appendCode(indent, "hh.PutBytes(%s[:%s])\n", valVar, limitVar)
		}
		itemSize = 1
	} else {
		// Hash individual elements
		if !pack {
			// Start vector merkleization
			ctx.appendCode(indent, "idx := hh.Index()\n")
		}

		if ctx.isPrimitive(desc.ElemDesc) {
			itemSize = int(desc.ElemDesc.Size)
		} else {
			itemSize = 32
		}

		valVar := ctx.getValVar()
		valVarPtrPrefix := ctx.getPtrPrefix(desc.ElemDesc, "*")
		isPtrType := desc.ElemDesc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 || valVarPtrPrefix != ""
		emptyVarAddin := ""
		if !isPtrType {
			emptyVarAddin = fmt.Sprintf(", %sEmpty", valVar)
		}
		ctx.appendCode(indent, "var %s%s %s%s\n", valVar, emptyVarAddin, valVarPtrPrefix, ctx.typePrinter.TypeString(desc.ElemDesc))
		ctx.appendCode(indent, "for i := range %s {\n", limitVar)
		ctx.appendCode(indent, "\tif i < %s {\n", lenVar)
		ctx.appendCode(indent, "\t\t%s = %s%s[i]\n", valVar, ctx.getPtrPrefix(desc.ElemDesc, "&"), varName)
		ctx.appendCode(indent, "\t} else if i == %s {\n", lenVar)
		if isPtrType {
			ctx.appendCode(indent, "\t\t%s = new(%s)\n", valVar, ctx.typePrinter.InnerTypeString(desc.ElemDesc))
		} else {
			ctx.appendCode(indent, "\t\t%s = %sEmpty\n", valVar, valVar)
		}
		ctx.appendCode(indent, "\t}\n")

		if err := ctx.hashType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

		if !pack {
			if itemSize < 32 {
				ctx.appendCode(indent, "hh.FillUpTo32()\n")
			}

			// Finalize vector with bit limit
			ctx.appendCode(indent, "hh.Merkleize(idx)\n")
		}
	}

	return nil
}

// hashList generates hash tree root code for SSZ list (variable-size array) types.
func (ctx *hashTreeRootContext) hashList(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	hasMax := false
	maxVar := ""

	if maxExpression != nil {
		exprVar := ctx.exprVars.getExprVar(*maxExpression, desc.Limit)

		hasMax = true
		maxVar = exprVar
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
		ctx.appendCode(indent, "vlen := uint64(len(%s))\n", valueVar)
		hasVlen = true
	}

	if hasMax {
		addVlen()
		ctx.appendCode(indent, "if vlen > %s {\n", maxVar)
		ctx.appendCode(indent, "\treturn sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}

	// Start list merkleization
	ctx.appendCode(indent, "idx := hh.Index()\n")
	itemSize := 0

	// Handle byte slices
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString != 0 {
		ctx.appendCode(indent, "hh.AppendBytes32([]byte(%s))\n", valueVar)
		itemSize = 1
	} else if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray != 0 {
		if strings.HasPrefix(valueVar, "*") {
			valueVar = fmt.Sprintf("(%s)", valueVar)
		}
		ctx.appendCode(indent, "hh.AppendBytes32(%s[:])\n", valueVar)
		itemSize = 1
	} else {
		if ctx.isPrimitive(desc.ElemDesc) {
			itemSize = int(desc.ElemDesc.Size)
		} else {
			itemSize = 32
		}

		// Hash all elements
		addVlen()
		ctx.appendCode(indent, "for i := range int(vlen) {\n")
		valVar := "t"
		if ctx.isInlineable(desc.ElemDesc) {
			valVar = fmt.Sprintf("%s[i]", varName)
		} else {
			ctx.appendCode(indent, "\tt := %s%s[i]\n", ctx.getPtrPrefix(desc.ElemDesc, "&"), varName)
		}
		if err := ctx.hashType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

		if itemSize < 32 {
			ctx.appendCode(indent, "hh.FillUpTo32()\n")
		}
	}

	if desc.SszType == ssztypes.SszProgressiveListType {
		addVlen()
		ctx.appendCode(indent, "hh.MerkleizeProgressiveWithMixin(idx, vlen)\n")
	} else if maxVar != "0" {
		addVlen()
		if itemSize > 0 {
			ctx.appendCode(indent, "hh.MerkleizeWithMixin(idx, vlen, sszutils.CalculateLimit(%s, vlen, %d))\n", maxVar, itemSize)
		} else {
			ctx.appendCode(indent, "hh.MerkleizeWithMixin(idx, vlen, %s)\n", maxVar)
		}
	} else {
		ctx.appendCode(indent, "hh.Merkleize(idx)\n")
	}

	return nil
}

// hashBitlist generates hash tree root code for SSZ bitlist types.
func (ctx *hashTreeRootContext) hashBitlist(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	maxVar := ""
	if maxExpression != nil {
		exprVar := ctx.exprVars.getExprVar(*maxExpression, desc.Limit)

		maxVar = exprVar
	} else if desc.Limit > 0 {
		maxVar = fmt.Sprintf("%d", desc.Limit)
	}

	ctx.appendCode(indent, "idx := hh.Index()\n")

	hasherAlias := ctx.typePrinter.AddImport("github.com/pk910/dynamic-ssz/hasher", "hasher")
	sizeVar := "_"
	if maxVar != "" || maxVar != "" || desc.SszType == ssztypes.SszProgressiveBitlistType {
		sizeVar = "size"
	}
	ctx.appendCode(indent, "bitlist, %s := %s.ParseBitlistWithHasher(hh, %s[:])\n", sizeVar, hasherAlias, varName)

	if maxVar != "" {
		ctx.appendCode(indent, "if size > %s {\n", maxVar)
		ctx.appendCode(indent, "\treturn sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}
	ctx.appendCode(indent, "hh.AppendBytes32(bitlist)\n")

	if desc.SszType == ssztypes.SszProgressiveBitlistType {
		ctx.appendCode(indent, "hh.MerkleizeProgressiveWithMixin(idx, size)\n")
	} else if maxVar != "" {
		ctx.appendCode(indent, "hh.MerkleizeWithMixin(idx, size, (%s+255)/256)\n", maxVar)
	} else {
		ctx.appendCode(indent, "hh.Merkleize(idx)\n")
	}

	return nil
}

// hashUnion generates hash tree root code for SSZ union types.
func (ctx *hashTreeRootContext) hashUnion(desc *ssztypes.TypeDescriptor, varName string, indent int) error {
	ctx.appendCode(indent, "idx := hh.Index()\n")
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
		ctx.appendCode(indent, "\tif !ok {\n\t\treturn sszutils.ErrInvalidUnionVariant\n\t}\n")
		if err := ctx.hashType(variantDesc, "v", indent+1, false, false); err != nil {
			return err
		}
	}

	ctx.appendCode(indent, "default:\n")
	ctx.appendCode(indent, "\treturn sszutils.ErrInvalidUnionVariant\n")
	ctx.appendCode(indent, "}\n")

	ctx.appendCode(indent, "hh.PutUint8(%s.Variant)\n", varName)
	ctx.appendCode(indent, "hh.Merkleize(idx)\n")

	return nil
}
