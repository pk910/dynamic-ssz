package codegen

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

type hashTreeRootContext struct {
	appendCode    func(indent int, code string, args ...any)
	typePrinter   *TypePrinter
	options       *CodeGeneratorOptions
	usedDynSsz    bool
	valVarCounter int
}

func generateHashTreeRoot(rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
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
	}

	// Generate main function signature
	typeName := typePrinter.TypeString(rootTypeDesc)

	// Generate hash tree root code
	if err := ctx.hashType(rootTypeDesc, "t", 1, true, false); err != nil {
		return false, err
	}

	genDynamicFn := !options.WithoutDynamicExpressions
	genStaticFn := options.WithoutDynamicExpressions || options.CreateLegacyFn

	if genDynamicFn {
		if ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {\n", typeName))
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {\n", typeName))
			codeBuilder.WriteString("\treturn t.HashTreeRootWith(hh)\n")
			codeBuilder.WriteString("}\n\n")
			genStaticFn = true
		}
	}

	if genStaticFn {
		if !ctx.usedDynSsz {
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) HashTreeRootWith(hh sszutils.HashWalker) error {\n", typeName))
			codeBuilder.WriteString(codeBuf.String())
			codeBuilder.WriteString("\treturn nil\n")
			codeBuilder.WriteString("}\n\n")
		} else {
			dynsszAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz", "dynssz")
			codeBuilder.WriteString(fmt.Sprintf("func (t %s) HashTreeRootWith(hh sszutils.HashWalker) error {\n", typeName))
			codeBuilder.WriteString(fmt.Sprintf("\treturn t.HashTreeRootWithDyn(%s.GetGlobalDynSsz(), hh)\n", dynsszAlias))
			codeBuilder.WriteString("}\n\n")
		}
	}

	// Dynamic hash tree root function
	if genDynamicFn {
		hasherAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz/hasher", "hasher")
		codeBuilder.WriteString(fmt.Sprintf("func (t %s) HashTreeRootDyn(ds sszutils.DynamicSpecs) ([32]byte, error) {\n", typeName))
		codeBuilder.WriteString(fmt.Sprintf("\tpool := &%s.FastHasherPool\n", hasherAlias))
		codeBuilder.WriteString("\thh := pool.Get()\n")
		codeBuilder.WriteString("\tdefer func() {\n\t\tpool.Put(hh)\n\t}()\n")
		codeBuilder.WriteString("\tif err := t.HashTreeRootWithDyn(ds, hh); err != nil {\n\t\treturn [32]byte{}, err\n\t}\n")
		codeBuilder.WriteString("\tr, _ := hh.HashRoot()\n")
		codeBuilder.WriteString("\treturn r, nil\n")
		codeBuilder.WriteString("}\n\n")
	}

	// Static hash tree root function
	if genStaticFn {
		hasherAlias := typePrinter.AddImport("github.com/pk910/dynamic-ssz/hasher", "hasher")
		codeBuilder.WriteString(fmt.Sprintf("func (t %s) HashTreeRoot() ([32]byte, error) {\n", typeName))
		codeBuilder.WriteString(fmt.Sprintf("\tpool := &%s.FastHasherPool\n", hasherAlias))
		codeBuilder.WriteString("\thh := pool.Get()\n")
		codeBuilder.WriteString("\tdefer func() {\n\t\tpool.Put(hh)\n\t}()\n")
		codeBuilder.WriteString("\tif err := t.HashTreeRootWith(hh); err != nil {\n\t\treturn [32]byte{}, err\n\t}\n")
		codeBuilder.WriteString("\tr, _ := hh.HashRoot()\n")
		codeBuilder.WriteString("\treturn r, nil\n")
		codeBuilder.WriteString("}\n")
	}

	return ctx.usedDynSsz, nil
}

func (ctx *hashTreeRootContext) isPrimitive(desc *dynssz.TypeDescriptor) bool {
	return desc.SszType == dynssz.SszBoolType || desc.SszType == dynssz.SszUint8Type || desc.SszType == dynssz.SszUint16Type || desc.SszType == dynssz.SszUint32Type || desc.SszType == dynssz.SszUint64Type || desc.SszType == dynssz.SszUint128Type
}

func (ctx *hashTreeRootContext) getValVar() string {
	ctx.valVarCounter++
	return fmt.Sprintf("val%d", ctx.valVarCounter)
}

func (ctx *hashTreeRootContext) hashType(desc *dynssz.TypeDescriptor, varName string, indent int, isRoot bool, pack bool) error {
	// Handle types that have generated methods we can call
	if desc.SszCompatFlags&dynssz.SszCompatFlagDynamicHashRoot != 0 && !isRoot {
		ctx.appendCode(indent, "if err := %s.HashTreeRootWithDyn(ds, hh); err != nil {\n\treturn err\n}\n", varName)
		ctx.usedDynSsz = true
		return nil
	}

	useFastSsz := !ctx.options.NoFastSsz && desc.SszCompatFlags&dynssz.SszCompatFlagHashTreeRootWith != 0
	if !useFastSsz && desc.SszType == dynssz.SszCustomType {
		useFastSsz = true
	}

	if useFastSsz && !isRoot {
		ctx.appendCode(indent, "if err := %s.HashTreeRootWith(hh); err != nil {\n\treturn err\n}\n", varName)
		return nil
	}

	switch desc.SszType {
	case dynssz.SszBoolType:
		if pack {
			ctx.appendCode(indent, "hh.AppendBool(%s)\n", varName)
		} else {
			ctx.appendCode(indent, "hh.PutBool(%s)\n", varName)
		}
	case dynssz.SszUint8Type:
		if pack {
			ctx.appendCode(indent, "hh.AppendUint8(uint8(%s))\n", varName)
		} else {
			ctx.appendCode(indent, "hh.PutUint8(uint8(%s))\n", varName)
		}
	case dynssz.SszUint16Type:
		if pack {
			ctx.appendCode(indent, "hh.AppendUint16(uint16(%s))\n", varName)
		} else {
			ctx.appendCode(indent, "hh.PutUint16(uint16(%s))\n", varName)
		}

	case dynssz.SszUint32Type:
		if pack {
			ctx.appendCode(indent, "hh.AppendUint32(uint32(%s))\n", varName)
		} else {
			ctx.appendCode(indent, "hh.PutUint32(uint32(%s))\n", varName)
		}

	case dynssz.SszUint64Type:
		var valVar string
		if desc.GoTypeFlags&dynssz.GoTypeFlagIsTime != 0 {
			valVar = fmt.Sprintf("uint64(%s.Unix())", varName)
		} else {
			valVar = fmt.Sprintf("uint64(%s)", varName)
		}
		if pack {
			ctx.appendCode(indent, "hh.AppendUint64(%s)\n", valVar)
		} else {
			ctx.appendCode(indent, "hh.PutUint64(%s)\n", valVar)
		}

	case dynssz.SszTypeWrapperType:
		ctx.appendCode(indent, "{\n\tt := %s.Data\n", varName)
		if err := ctx.hashType(desc.ElemDesc, "t", indent+1, false, pack); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")

	case dynssz.SszContainerType:
		return ctx.hashContainer(desc, varName, indent)

	case dynssz.SszProgressiveContainerType:
		return ctx.hashProgressiveContainer(desc, varName, indent)

	case dynssz.SszUint128Type, dynssz.SszUint256Type:
		return ctx.hashVector(desc, varName, indent, true)

	case dynssz.SszVectorType, dynssz.SszBitvectorType:
		return ctx.hashVector(desc, varName, indent, false)

	case dynssz.SszListType, dynssz.SszProgressiveListType:
		return ctx.hashList(desc, varName, indent)

	case dynssz.SszBitlistType, dynssz.SszProgressiveBitlistType:
		return ctx.hashBitlist(desc, varName, indent)

	case dynssz.SszCompatibleUnionType:
		return ctx.hashUnion(desc, varName, indent)

	case dynssz.SszCustomType:
		ctx.appendCode(indent, "// Custom type - hash unknown\n")

	default:
		return fmt.Errorf("unsupported SSZ type: %v", desc.SszType)
	}

	return nil
}

func (ctx *hashTreeRootContext) hashContainer(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	// Start container merkleization
	ctx.appendCode(indent, "idx := hh.Index()\n")

	// Hash each field
	for idx, field := range desc.ContainerDesc.Fields {
		ctx.appendCode(indent, "{ // Field #%d '%s'\n", idx, field.Name)
		ctx.appendCode(indent, "\tt := %s.%s\n", varName, field.Name)
		if err := ctx.hashType(field.Type, "t", indent+1, false, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	// Finalize container
	ctx.appendCode(indent, "hh.Merkleize(idx)\n")

	return nil
}

func (ctx *hashTreeRootContext) hashProgressiveContainer(desc *dynssz.TypeDescriptor, varName string, indent int) error {
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
		ctx.appendCode(indent, "\tt := %s.%s\n", varName, field.Name)
		if err := ctx.hashType(field.Type, "t", indent+1, false, false); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	activeFields := ctx.getActiveFieldsHex(desc)
	ctx.appendCode(indent, "hh.MerkleizeProgressiveWithActiveFields(idx, %s)\n", activeFields)

	return nil
}

func (ctx *hashTreeRootContext) getActiveFieldsHex(sourceType *dynssz.TypeDescriptor) string {
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

func (ctx *hashTreeRootContext) hashVector(desc *dynssz.TypeDescriptor, varName string, indent int, pack bool) error {
	sizeExpression := desc.SizeExpression
	if ctx.options.WithoutDynamicExpressions {
		sizeExpression = nil
	}

	limitVar := ""
	if sizeExpression != nil {
		ctx.usedDynSsz = true
		ctx.appendCode(indent, "hasLimit, limit, err := ds.ResolveSpecValue(\"%s\")\n", *sizeExpression)
		ctx.appendCode(indent, "if err != nil {\n\treturn err\n}\n")
		ctx.appendCode(indent, "if !hasLimit {\n\tlimit = %d\n}\n", desc.Len)
		limitVar = "int(limit)"
	} else {
		limitVar = fmt.Sprintf("%d", desc.Len)
	}

	if !pack {
		// Start vector merkleization
		ctx.appendCode(indent, "idx := hh.Index()\n")
	}

	hasVLen := false
	if desc.Kind != reflect.Array {
		ctx.appendCode(indent, "vlen := len(%s)\n", varName)
		ctx.appendCode(indent, "if vlen > %s {\n", limitVar)
		ctx.appendCode(indent, "\treturn sszutils.ErrVectorLength\n")
		ctx.appendCode(indent, "}\n")
		hasVLen = true
	}

	// Handle byte arrays
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
		ctx.appendCode(indent, "hh.PutBytes([]byte(%s))\n", varName)
	} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		ctx.appendCode(indent, "hh.PutBytes(%s[:])\n", varName)
	} else {
		// Hash elements
		if !hasVLen {
			ctx.appendCode(indent, "vlen := len(%s)\n", varName)
		}
		valVar := ctx.getValVar()
		ctx.appendCode(indent, "for i := 0; i < %s; i++ {\n", limitVar)
		ctx.appendCode(indent, "\tvar %s %s\n", valVar, ctx.typePrinter.TypeString(desc.ElemDesc))
		ctx.appendCode(indent, "\tif i < vlen {\n")
		ctx.appendCode(indent, "\t\t%s = %s[i]\n", valVar, varName)
		ctx.appendCode(indent, "\t}\n")

		if err := ctx.hashType(desc.ElemDesc, valVar, indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	if !pack {
		// Finalize vector with bit limit
		ctx.appendCode(indent, "hh.Merkleize(idx)\n")
	}

	return nil
}

func (ctx *hashTreeRootContext) hashList(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	maxVar := ""
	if maxExpression != nil {
		ctx.usedDynSsz = true
		ctx.appendCode(indent, "hasMax, max, err := ds.ResolveSpecValue(\"%s\")\n", *maxExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn err\n")
		ctx.appendCode(indent, "}\n")
		if desc.Limit > 0 {
			ctx.appendCode(indent, "if !hasMax {\n")
			ctx.appendCode(indent, "\tmax = %d\n", desc.Limit)
			ctx.appendCode(indent, "}\n")
		}
		maxVar = "uint64(max)"
	} else if desc.Limit > 0 {
		maxVar = fmt.Sprintf("%d", desc.Limit)
	}

	// Start list merkleization
	ctx.appendCode(indent, "idx := hh.Index()\n")
	ctx.appendCode(indent, "vlen := uint64(len(%s))\n", varName)

	itemSize := 0

	// Handle byte slices
	if desc.GoTypeFlags&dynssz.GoTypeFlagIsString != 0 {
		ctx.appendCode(indent, "hh.PutBytes([]byte(%s))\n", varName)
		itemSize = 1
	} else if desc.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
		ctx.appendCode(indent, "hh.PutBytes(%s[:])\n", varName)
		itemSize = 1
	} else {
		if ctx.isPrimitive(desc.ElemDesc) {
			itemSize = int(desc.ElemDesc.Size)
		} else {
			itemSize = 32
		}

		// Hash all elements
		ctx.appendCode(indent, "for i := 0; i < int(vlen); i++ {\n")
		ctx.appendCode(indent, "\tt := %s[i]\n", varName)
		if err := ctx.hashType(desc.ElemDesc, "t", indent+1, false, true); err != nil {
			return err
		}
		ctx.appendCode(indent, "}\n")
	}

	if desc.SszType == dynssz.SszProgressiveListType {
		ctx.appendCode(indent, "hh.MerkleizeProgressiveWithMixin(idx, vlen)\n")
	} else if maxVar != "" {
		if itemSize > 0 {
			ctx.appendCode(indent, "limit := sszutils.CalculateLimit(%s, vlen, %d)\n", maxVar, itemSize)
			ctx.appendCode(indent, "hh.MerkleizeWithMixin(idx, vlen, limit)\n")
		} else {
			ctx.appendCode(indent, "hh.MerkleizeWithMixin(idx, vlen, %s)\n", maxVar)
		}
	} else {
		ctx.appendCode(indent, "hh.Merkleize(idx)\n")
	}

	return nil
}

func (ctx *hashTreeRootContext) hashBitlist(desc *dynssz.TypeDescriptor, varName string, indent int) error {
	maxExpression := desc.MaxExpression
	if ctx.options.WithoutDynamicExpressions {
		maxExpression = nil
	}

	maxVar := ""
	if maxExpression != nil {
		ctx.usedDynSsz = true
		ctx.appendCode(indent, "hasMax, max, err := ds.ResolveSpecValue(\"%s\")\n", *maxExpression)
		ctx.appendCode(indent, "if err != nil {\n")
		ctx.appendCode(indent, "\treturn err\n")
		ctx.appendCode(indent, "}\n")
		if desc.Limit > 0 {
			ctx.appendCode(indent, "if !hasMax {\n")
			ctx.appendCode(indent, "\tmax = %d\n", desc.Limit)
			ctx.appendCode(indent, "}\n")
		}
		maxVar = "uint64(max)"
	} else if desc.Limit > 0 {
		maxVar = fmt.Sprintf("%d", desc.Limit)
	}

	ctx.appendCode(indent, "idx := hh.Index()\n")
	ctx.appendCode(indent, "var size uint64\n")
	ctx.appendCode(indent, "var bitlist []byte\n")
	ctx.appendCode(indent, "hh.WithTemp(func(tmp []byte) []byte {\n")
	ctx.appendCode(indent, "\ttmp, size = hasher.ParseBitlist(tmp[:0], %s[:])\n", varName)
	ctx.appendCode(indent, "\tbitlist = tmp\n")
	ctx.appendCode(indent, "\treturn tmp\n")
	ctx.appendCode(indent, "})\n")

	if maxVar != "" {
		ctx.appendCode(indent, "if size > %s {\n", maxVar)
		ctx.appendCode(indent, "\treturn sszutils.ErrListTooBig\n")
		ctx.appendCode(indent, "}\n")
	}
	ctx.appendCode(indent, "hh.AppendBytes32(bitlist)\n")

	if desc.SszType == dynssz.SszProgressiveBitlistType {
		ctx.appendCode(indent, "hh.MerkleizeProgressiveWithMixin(idx, size)\n")
	} else if maxVar != "" {
		ctx.appendCode(indent, "hh.MerkleizeWithMixin(idx, size, (%s+255)/256)\n", maxVar)
	} else {
		ctx.appendCode(indent, "hh.Merkleize(idx)\n")
	}

	return nil
}

func (ctx *hashTreeRootContext) hashUnion(desc *dynssz.TypeDescriptor, varName string, indent int) error {
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
