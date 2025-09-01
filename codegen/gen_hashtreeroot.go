package codegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

func generateHashTreeRoot(ds *dynssz.DynSsz, rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
	type hashTreeRootFnEntry struct {
		Fn   *tmpl.HashTreeRootFunction
		Type *dynssz.TypeDescriptor
	}

	codeTpl := GetTemplate("tmpl/hashtreeroot.tmpl")
	hashTreeRootFnMap := map[string]hashTreeRootFnEntry{}
	hashTreeRootFnIdx := 0
	usedDynSsz := false

	var genRecursive func(sourceType *dynssz.TypeDescriptor, isRoot bool, pack bool) (*tmpl.HashTreeRootFunction, error)

	isBaseType := func(sourceType *dynssz.TypeDescriptor) bool {
		// Check if it's a primitive type or simple byte array/slice that can be inlined
		switch sourceType.SszType {
		case dynssz.SszBoolType, dynssz.SszUint8Type, dynssz.SszUint16Type, dynssz.SszUint32Type, dynssz.SszUint64Type:
			return true
		case dynssz.SszVectorType, dynssz.SszListType, dynssz.SszBitvectorType:
			// Inline byte arrays and byte slices, but not if they need dynamic spec resolution or have limits
			if sourceType.IsByteArray {
				// Don't inline if they have dynamic expressions that need spec resolution
				// Don't inline if they have limits (need individual merkleization with mixin)
				return sourceType.MaxExpression == "" && sourceType.SizeExpression == "" && !sourceType.HasLimit
			}
			return false
		default:
			return false
		}
	}

	getInlineBaseTypeHash := func(sourceType *dynssz.TypeDescriptor, varName string, pack bool) string {
		// Handle primitive types with packing support
		switch sourceType.SszType {
		case dynssz.SszBoolType:
			if pack {
				return fmt.Sprintf("hh.AppendBool(bool(%s))", varName)
			}
			return fmt.Sprintf("hh.PutBool(bool(%s))", varName)
		case dynssz.SszUint8Type:
			if pack {
				return fmt.Sprintf("hh.AppendUint8(uint8(%s))", varName)
			}
			return fmt.Sprintf("hh.PutUint8(uint8(%s))", varName)
		case dynssz.SszUint16Type:
			if pack {
				return fmt.Sprintf("hh.AppendUint16(uint16(%s))", varName)
			}
			return fmt.Sprintf("hh.PutUint16(uint16(%s))", varName)
		case dynssz.SszUint32Type:
			if pack {
				return fmt.Sprintf("hh.AppendUint32(uint32(%s))", varName)
			}
			return fmt.Sprintf("hh.PutUint32(uint32(%s))", varName)
		case dynssz.SszUint64Type:
			if pack {
				return fmt.Sprintf("hh.AppendUint64(uint64(%s))", varName)
			}
			return fmt.Sprintf("hh.PutUint64(uint64(%s))", varName)
		case dynssz.SszVectorType, dynssz.SszListType:
			if sourceType.IsByteArray {
				// Simple byte arrays/slices can be inlined directly
				// (complex cases with dynamic expressions are filtered out in isBaseType)
				if pack {
					// When packing (inside lists/vectors), use Append to avoid merkleization
					return fmt.Sprintf("hh.Append(%s[:])", varName)
				} else {
					// When not packing (standalone), use PutBytes which handles merkleization
					return fmt.Sprintf("hh.PutBytes(%s[:])", varName)
				}
			}
			return ""
		default:
			return ""
		}
	}

	genRecursive = func(sourceType *dynssz.TypeDescriptor, isRoot bool, pack bool) (*tmpl.HashTreeRootFunction, error) {
		// For base types that are not root, return a special inline function
		if !isRoot && isBaseType(sourceType) && !sourceType.IsPtr {
			return &tmpl.HashTreeRootFunction{
				IsInlined:  true,
				InlineCode: getInlineBaseTypeHash(sourceType, "VAR_NAME", pack),
			}, nil
		}

		// Generate type key first to check if we've seen this type before
		typeName := sourceType.Type.String() // Use basic string representation for key
		typeKey := typeName
		if sourceType.Len > 0 {
			typeKey = fmt.Sprintf("%s:%d", typeKey, sourceType.Len)
		}
		if sourceType.SizeExpression != "" && !options.WithoutDynamicExpressions {
			typeKey = fmt.Sprintf("%s:%s", typeKey, sourceType.SizeExpression)
		}
		if sourceType.MaxExpression != "" && !options.WithoutDynamicExpressions {
			typeKey = fmt.Sprintf("%s:%s", typeKey, sourceType.MaxExpression)
		}
		if pack {
			typeKey = fmt.Sprintf("%s:pack", typeKey)
		}

		childType := sourceType
		for {
			childType = childType.ElemDesc
			if childType == nil {
				break
			}

			if childType.Len > 0 {
				typeKey = fmt.Sprintf("%s:%d", typeKey, childType.Len)
			}
			if childType.SizeExpression != "" && !options.WithoutDynamicExpressions {
				typeKey = fmt.Sprintf("%s:%s", typeKey, childType.SizeExpression)
			}
			if childType.MaxExpression != "" && !options.WithoutDynamicExpressions {
				typeKey = fmt.Sprintf("%s:%s", typeKey, childType.MaxExpression)
			}
		}

		if fn, found := hashTreeRootFnMap[typeKey]; found {
			return fn.Fn, nil
		}

		useFastSsz := !ds.NoFastSsz && sourceType.HasFastSSZHasher && !sourceType.HasDynamicSize && !sourceType.HasDynamicMax && !isRoot
		if useFastSsz && (sourceType.HasMaxExpr || sourceType.HasSizeExpr) {
			useFastSsz = false
		}
		if !useFastSsz && sourceType.SszType == dynssz.SszCustomType {
			useFastSsz = true
		}

		// Check if we should use dynamic hash root - can ALWAYS be used unlike fastssz
		useDynamicHashRoot := sourceType.HasDynamicHashRoot

		code := strings.Builder{}
		hashTreeRootFn := &tmpl.HashTreeRootFunction{
			Index:     0,
			Key:       typeKey,
			TypeName:  typePrinter.TypeString(sourceType.Type),
			IsPointer: sourceType.IsPtr,
		}

		if sourceType.IsPtr {
			hashTreeRootFn.InnerType = typePrinter.TypeString(sourceType.Type.Elem())
		}

		hashTreeRootFnMap[typeKey] = hashTreeRootFnEntry{
			Fn:   hashTreeRootFn,
			Type: sourceType,
		}

		if useFastSsz && !isRoot {
			// Use the method availability information from the type cache
			if sourceType.HasHashTreeRootWith {
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_fastssz_with", nil); err != nil {
					return nil, err
				}
			} else {
				// Use HashTreeRoot method
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_fastssz_root", nil); err != nil {
					return nil, err
				}
			}
		} else if useDynamicHashRoot && !isRoot {
			// Use dynamic hash root
			if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_dynamic", nil); err != nil {
				return nil, err
			}
			usedDynSsz = true
		} else {
			switch sourceType.SszType {
			// complex types
			case dynssz.SszTypeWrapperType:
				fn, err := genRecursive(sourceType.ElemDesc, false, pack)
				if err != nil {
					return nil, err
				}

				wrapperModel := tmpl.HashTreeRootWrapper{
					TypeName:       typePrinter.TypeString(sourceType.Type),
					HashTreeRootFn: fn.Name,
				}

				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_wrapper", wrapperModel); err != nil {
					return nil, err
				}
			case dynssz.SszContainerType:
				structModel := tmpl.HashTreeRootStruct{
					TypeName: typePrinter.TypeString(sourceType.Type),
					Fields:   make([]tmpl.HashTreeRootField, 0, len(sourceType.ContainerDesc.Fields)),
				}
				for idx, field := range sourceType.ContainerDesc.Fields {
					fn, err := genRecursive(field.Type, false, false)
					if err != nil {
						return nil, err
					}

					fieldModel := tmpl.HashTreeRootField{
						Index:     idx,
						Name:      field.Name,
						IsDynamic: field.Type.IsDynamic,
					}

					if fn.IsInlined {
						// Use inline code for base types
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", fmt.Sprintf("t.%s", field.Name))
						fieldModel.InlineHashCode = inlineCode
					} else {
						fieldModel.TypeName = typePrinter.TypeString(field.Type.Type)
						fieldModel.HashTreeRootFn = fn.Name
					}

					structModel.Fields = append(structModel.Fields, fieldModel)

					if field.Type.IsDynamic {
						structModel.HasDynamicFields = true
					}
				}
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_struct", structModel); err != nil {
					return nil, err
				}
			case dynssz.SszProgressiveContainerType:
				// Progressive container needs special handling
				activeFields := getActiveFieldsHex(sourceType)
				structModel := tmpl.HashTreeRootProgressiveContainer{
					TypeName:     typePrinter.TypeString(sourceType.Type),
					ActiveFields: activeFields,
					Fields:       make([]tmpl.HashTreeRootField, 0, len(sourceType.ContainerDesc.Fields)),
				}
				for idx, field := range sourceType.ContainerDesc.Fields {
					fn, err := genRecursive(field.Type, false, false)
					if err != nil {
						return nil, err
					}

					fieldModel := tmpl.HashTreeRootField{
						Index:     idx,
						Name:      field.Name,
						IsDynamic: field.Type.IsDynamic,
						SszIndex:  field.SszIndex,
					}

					if fn.IsInlined {
						// Use inline code for base types
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", fmt.Sprintf("t.%s", field.Name))
						fieldModel.InlineHashCode = inlineCode
					} else {
						fieldModel.TypeName = typePrinter.TypeString(field.Type.Type)
						fieldModel.HashTreeRootFn = fn.Name
					}

					structModel.Fields = append(structModel.Fields, fieldModel)
				}
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_progressive_container", structModel); err != nil {
					return nil, err
				}
			case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
				hashTreeRootFn := ""
				inlineHashCode := ""
				if !sourceType.IsByteArray {
					// For vectors, items are packed
					fn, err := genRecursive(sourceType.ElemDesc, false, true)
					if err != nil {
						return nil, err
					}

					if fn.IsInlined {
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", "t[i]")
						hashTreeRootFn = ""
						inlineHashCode = inlineCode
					} else {
						hashTreeRootFn = fn.Name
					}
				}

				sizeExpression := sourceType.SizeExpression
				if options.WithoutDynamicExpressions {
					sizeExpression = ""
				}
				if sizeExpression != "" {
					usedDynSsz = true
				}

				vectorModel := tmpl.HashTreeRootVector{
					TypeName:           typePrinter.TypeString(sourceType.Type),
					Length:             int(sourceType.Len),
					ItemSize:           int(sourceType.ElemDesc.Size),
					HashTreeRootFn:     hashTreeRootFn,
					InlineItemHashCode: inlineHashCode,
					SizeExpr:           sizeExpression,
					IsArray:            sourceType.Kind == reflect.Array,
					IsByteArray:        sourceType.IsByteArray,
					IsString:           sourceType.IsString,
				}

				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_vector", vectorModel); err != nil {
					return nil, err
				}
			case dynssz.SszListType, dynssz.SszProgressiveListType:
				hashTreeRootFn := ""
				inlineHashCode := ""
				if !sourceType.IsByteArray {
					// For lists, items are packed
					fn, err := genRecursive(sourceType.ElemDesc, false, true)
					if err != nil {
						return nil, err
					}

					if fn.IsInlined {
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", "t[i]")
						hashTreeRootFn = ""
						inlineHashCode = inlineCode
					} else {
						hashTreeRootFn = fn.Name
					}
				}

				sizeExpression := sourceType.SizeExpression
				maxExpression := sourceType.MaxExpression
				if options.WithoutDynamicExpressions {
					sizeExpression = ""
					maxExpression = ""
				}
				if sizeExpression != "" || maxExpression != "" {
					usedDynSsz = true
				}

				listModel := tmpl.HashTreeRootList{
					TypeName:           typePrinter.TypeString(sourceType.Type),
					MaxLength:          int(sourceType.Limit),
					HashTreeRootFn:     hashTreeRootFn,
					InlineItemHashCode: inlineHashCode,
					SizeExpr:           sizeExpression,
					MaxExpr:            maxExpression,
					HasLimit:           sourceType.HasLimit,
					IsProgressive:      (sourceType.SszType == dynssz.SszProgressiveListType || sourceType.SszType == dynssz.SszProgressiveBitlistType),
					IsByteArray:        sourceType.IsByteArray,
					IsString:           sourceType.IsString,
				}

				switch sourceType.ElemDesc.SszType {
				case dynssz.SszBoolType:
					listModel.ItemSize = 1
				case dynssz.SszUint8Type:
					listModel.ItemSize = 1
				case dynssz.SszUint16Type:
					listModel.ItemSize = 2
				case dynssz.SszUint32Type:
					listModel.ItemSize = 4
				case dynssz.SszUint64Type:
					listModel.ItemSize = 8
				case dynssz.SszUint128Type:
					listModel.ItemSize = 16
				case dynssz.SszUint256Type:
					listModel.ItemSize = 32
				default:
					listModel.ItemSize = 0
				}

				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_list", listModel); err != nil {
					return nil, err
				}
			case dynssz.SszBitlistType, dynssz.SszProgressiveBitlistType:
				maxExpression := sourceType.MaxExpression
				if options.WithoutDynamicExpressions {
					maxExpression = ""
				}
				if maxExpression != "" {
					usedDynSsz = true
				}

				bitlistModel := tmpl.HashTreeRootBitlist{
					TypeName:      typePrinter.TypeString(sourceType.Type),
					MaxLength:     int(sourceType.Limit),
					MaxExpr:       maxExpression,
					HasLimit:      sourceType.HasLimit,
					IsProgressive: (sourceType.SszType == dynssz.SszProgressiveBitlistType),
				}

				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_bitlist", bitlistModel); err != nil {
					return nil, err
				}
			case dynssz.SszCompatibleUnionType:
				variantFns := make([]tmpl.HashTreeRootCompatibleUnionVariant, 0, len(sourceType.UnionVariants))
				for variant, variantDesc := range sourceType.UnionVariants {
					fn, err := genRecursive(variantDesc, false, false)
					if err != nil {
						return nil, err
					}

					variantModel := tmpl.HashTreeRootCompatibleUnionVariant{
						Index:    int(variant),
						TypeName: typePrinter.TypeString(variantDesc.Type),
					}

					if fn.IsInlined {
						// Generate inline code for union variants
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", "v")
						variantModel.InlineHashCode = inlineCode
					} else {
						variantModel.HashTreeRootFn = fn.Name
					}

					variantFns = append(variantFns, variantModel)
				}

				sort.Slice(variantFns, func(i, j int) bool {
					return variantFns[i].Index < variantFns[j].Index
				})

				compatibleUnionModel := tmpl.HashTreeRootCompatibleUnion{
					TypeName:   typePrinter.TypeString(sourceType.Type),
					VariantFns: variantFns,
				}

				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_compatible_union", compatibleUnionModel); err != nil {
					return nil, err
				}
			// primitive types
			case dynssz.SszBoolType:
				model := map[string]interface{}{"pack": pack}
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_bool", model); err != nil {
					return nil, err
				}
			case dynssz.SszUint8Type:
				model := map[string]interface{}{"pack": pack}
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_uint8", model); err != nil {
					return nil, err
				}
			case dynssz.SszUint16Type:
				model := map[string]interface{}{"pack": pack}
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_uint16", model); err != nil {
					return nil, err
				}
			case dynssz.SszUint32Type:
				model := map[string]interface{}{"pack": pack}
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_uint32", model); err != nil {
					return nil, err
				}
			case dynssz.SszUint64Type:
				model := map[string]interface{}{"pack": pack}
				if err := codeTpl.ExecuteTemplate(&code, "hashtreeroot_uint64", model); err != nil {
					return nil, err
				}
			case dynssz.SszCustomType:
				code.WriteString("return errors.New(\"type does not implement ssziface.FastsszHashRoot\")\n")
			}
		}

		hashTreeRootFnIdx++
		hashTreeRootFn.Index = hashTreeRootFnIdx
		hashTreeRootFn.Name = fmt.Sprintf("fn%d", hashTreeRootFnIdx)
		hashTreeRootFn.Code = code.String()

		return hashTreeRootFn, nil
	}

	rootFn, err := genRecursive(rootTypeDesc, true, false)
	if err != nil {
		return false, err
	}

	hashTreeRootFnList := make([]*tmpl.HashTreeRootFunction, 0, len(hashTreeRootFnMap))
	for _, fn := range hashTreeRootFnMap {
		hashTreeRootFnList = append(hashTreeRootFnList, fn.Fn)
	}

	sort.Slice(hashTreeRootFnList, func(i, j int) bool {
		return hashTreeRootFnList[i].Index < hashTreeRootFnList[j].Index
	})

	hashTreeRootModel := tmpl.HashTreeRootMain{
		TypeName:              typePrinter.TypeString(rootTypeDesc.Type),
		HashTreeRootFunctions: hashTreeRootFnList,
		RootFnName:            rootFn.Name,
		CreateLegacyFn:        options.CreateLegacyFn,
		CreateDynamicFn:       !options.WithoutDynamicExpressions,
		UsedDynSsz:            usedDynSsz,
		HasherAlias:           typePrinter.AddImport("github.com/pk910/dynamic-ssz/hasher", "hasher"),
	}

	usedDynSsz = usedDynSsz || !options.WithoutDynamicExpressions

	if err := codeTpl.ExecuteTemplate(codeBuilder, "hashtreeroot_main", hashTreeRootModel); err != nil {
		return false, err
	}

	return usedDynSsz, nil
}

// getActiveFieldsHex returns the hex representation of the active fields bitlist for a progressive container
func getActiveFieldsHex(sourceType *dynssz.TypeDescriptor) string {
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
