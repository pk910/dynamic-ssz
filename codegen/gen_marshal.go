package codegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

func generateMarshal(ds *dynssz.DynSsz, rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
	type marshalFnEntry struct {
		Fn   *tmpl.MarshalFunction
		Type *dynssz.TypeDescriptor
	}

	codeTpl := GetTemplate("tmpl/marshal.tmpl")
	marshalFnMap := map[string]marshalFnEntry{}
	marshalFnIdx := 0
	usedDynSsz := false

	var genRecursive func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.MarshalFunction, error)

	isBaseType := func(sourceType *dynssz.TypeDescriptor) bool {
		// Check if it's a byte array/slice
		if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			// Don't inline if it has dynamic size expressions - these need spec resolution
			if sourceType.SizeExpression != nil || sourceType.MaxExpression != nil {
				return false
			}
			return true
		}

		// Check if it's a primitive type
		switch sourceType.SszType {
		case dynssz.SszBoolType, dynssz.SszUint8Type, dynssz.SszUint16Type, dynssz.SszUint32Type, dynssz.SszUint64Type:
			return true
		default:
			return false
		}
	}

	getInlineBaseTypeMarshal := func(sourceType *dynssz.TypeDescriptor, varName string) string {
		// Handle byte arrays/slices
		if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			if sourceType.Kind == reflect.Array {
				// For arrays, append all bytes (arrays have fixed size)
				return fmt.Sprintf("dst = append(dst, %s[:]...)", varName)
			} else if sourceType.Size > 0 {
				// For slices with fixed size, limit to expected field size to prevent buffer overflow
				expectedSize := int(sourceType.Size)
				return fmt.Sprintf("if len(%s) > %d {\n\tdst = append(dst, %s[:%d]...)\n} else {\n\tdst = append(dst, %s[:]...)\n\tif len(%s) < %d {\n\t\tdst = sszutils.AppendZeroPadding(dst, %d-len(%s))\n\t}\n}",
					varName, expectedSize, varName, expectedSize, varName, varName, expectedSize, expectedSize, varName)
			} else {
				// For dynamic slices (no fixed size), append all available bytes
				return fmt.Sprintf("dst = append(dst, %s[:]...)", varName)
			}
		}

		// Handle primitive types
		switch sourceType.SszType {
		case dynssz.SszBoolType:
			return fmt.Sprintf("dst = sszutils.MarshalBool(dst, bool(%s))", varName)
		case dynssz.SszUint8Type:
			return fmt.Sprintf("dst = sszutils.MarshalUint8(dst, uint8(%s))", varName)
		case dynssz.SszUint16Type:
			return fmt.Sprintf("dst = sszutils.MarshalUint16(dst, uint16(%s))", varName)
		case dynssz.SszUint32Type:
			return fmt.Sprintf("dst = sszutils.MarshalUint32(dst, uint32(%s))", varName)
		case dynssz.SszUint64Type:
			return fmt.Sprintf("dst = sszutils.MarshalUint64(dst, uint64(%s))", varName)
		default:
			return ""
		}
	}

	genRecursive = func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.MarshalFunction, error) {
		// For base types that are not root, return a special inline function
		if !isRoot && isBaseType(sourceType) && sourceType.GoTypeFlags&dynssz.GoTypeFlagIsPointer == 0 {
			return &tmpl.MarshalFunction{
				IsInlined:  true,
				InlineCode: getInlineBaseTypeMarshal(sourceType, "VAR_NAME"),
			}, nil
		}
		// Generate type key first to check if we've seen this type before
		typeName := sourceType.Type.String() // Use basic string representation for key
		typeKey := typeName
		if sourceType.Len > 0 {
			typeKey = fmt.Sprintf("%s:%d", typeKey, sourceType.Len)
		}
		if sourceType.SizeExpression != nil && !options.WithoutDynamicExpressions {
			typeKey = fmt.Sprintf("%s:%s", typeKey, *sourceType.SizeExpression)
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
			if childType.SizeExpression != nil && !options.WithoutDynamicExpressions {
				typeKey = fmt.Sprintf("%s:%s", typeKey, *childType.SizeExpression)
			}
		}

		if fn, found := marshalFnMap[typeKey]; found {
			return fn.Fn, nil
		}

		hasDynamicSize := sourceType.SszTypeFlags&dynssz.SszTypeFlagHasDynamicSize != 0
		isFastsszMarshaler := sourceType.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
		useDynamicMarshal := sourceType.SszCompatFlags&dynssz.SszCompatFlagDynamicMarshaler != 0
		useFastSsz := !ds.NoFastSsz && isFastsszMarshaler && !hasDynamicSize
		if useFastSsz && sourceType.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 {
			useFastSsz = false
		}
		if !useFastSsz && sourceType.SszType == dynssz.SszCustomType {
			useFastSsz = true
		}

		code := strings.Builder{}
		marshalFn := &tmpl.MarshalFunction{
			Index: 0,
			Key:   typeKey,
		}
		marshalFnMap[typeKey] = marshalFnEntry{
			Fn:   marshalFn,
			Type: sourceType,
		}

		if useFastSsz && !isRoot {
			if err := codeTpl.ExecuteTemplate(&code, "marshal_fastssz", nil); err != nil {
				return nil, err
			}
		} else if useDynamicMarshal && !isRoot {
			// Use dynamic marshaler - create template for this
			if err := codeTpl.ExecuteTemplate(&code, "marshal_dynamic", nil); err != nil {
				return nil, err
			}

			usedDynSsz = true
		} else {
			switch sourceType.SszType {
			// complex types
			case dynssz.SszTypeWrapperType:
				fn, err := genRecursive(sourceType.ElemDesc, false)
				if err != nil {
					return nil, err
				}

				wrapperModel := tmpl.MarshalWrapper{
					TypeName:  typePrinter.TypeString(sourceType.Type),
					MarshalFn: fn.Name,
				}

				if err := codeTpl.ExecuteTemplate(&code, "marshal_wrapper", wrapperModel); err != nil {
					return nil, err
				}
			case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
				structModel := tmpl.MarshalStruct{
					TypeName: typePrinter.TypeString(sourceType.Type),
					Fields:   make([]tmpl.MarshalField, 0, len(sourceType.ContainerDesc.Fields)),
				}
				for idx, field := range sourceType.ContainerDesc.Fields {
					fn, err := genRecursive(field.Type, false)
					if err != nil {
						return nil, err
					}

					fieldModel := tmpl.MarshalField{
						Index:     idx,
						Name:      field.Name,
						IsDynamic: field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0,
						Size:      int(field.Type.Size),
					}

					if fn.IsInlined {
						// Use inline code for base types
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", fmt.Sprintf("t.%s", field.Name))
						fieldModel.InlineMarshalCode = inlineCode
					} else {
						fieldModel.TypeName = typePrinter.TypeString(field.Type.Type)
						fieldModel.MarshalFn = fn.Name
					}

					structModel.Fields = append(structModel.Fields, fieldModel)

					if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
						structModel.HasDynamicFields = true
					}
				}
				if err := codeTpl.ExecuteTemplate(&code, "marshal_struct", structModel); err != nil {
					return nil, err
				}
			case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
				marshalFn := ""
				inlineMarshalCode := ""
				if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray == 0 {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					if fn.IsInlined {
						// Generate inline code for vectors
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", "t[i]")
						inlineMarshalCode = inlineCode
					} else {
						marshalFn = fn.Name
					}
				}

				sizeExpression := sourceType.SizeExpression
				if options.WithoutDynamicExpressions {
					sizeExpression = nil
				}

				if sourceType.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
					emptyVal := reflect.New(sourceType.ElemDesc.Type).Elem()
					emptySize, err := ds.SizeSSZ(emptyVal.Interface())
					if err != nil {
						return nil, err
					}

					dynVectorModel := tmpl.MarshalDynamicVector{
						TypeName:              typePrinter.TypeString(sourceType.Type),
						Length:                int(sourceType.Len),
						EmptySize:             emptySize,
						MarshalFn:             marshalFn,
						InlineItemMarshalCode: inlineMarshalCode,
						SizeExpr:              "",
						IsArray:               sourceType.Kind == reflect.Array,
					}

					if sizeExpression != nil {
						dynVectorModel.SizeExpr = *sizeExpression
						usedDynSsz = true
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_dynamic_vector", dynVectorModel); err != nil {
						return nil, err
					}
				} else {
					vectorModel := tmpl.MarshalVector{
						TypeName:              typePrinter.TypeString(sourceType.Type),
						Length:                int(sourceType.Len),
						ItemSize:              int(sourceType.ElemDesc.Size),
						MarshalFn:             marshalFn,
						InlineItemMarshalCode: inlineMarshalCode,
						SizeExpr:              "",
						IsArray:               sourceType.Kind == reflect.Array,
						IsByteArray:           sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0,
						IsString:              sourceType.GoTypeFlags&dynssz.GoTypeFlagIsString != 0,
					}

					if sizeExpression != nil {
						vectorModel.SizeExpr = *sizeExpression
						usedDynSsz = true
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_vector", vectorModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
				marshalFn := ""
				inlineMarshalCode := ""
				if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray == 0 {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					if fn.IsInlined {
						// Generate inline code for lists
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", "t[i]")
						inlineMarshalCode = inlineCode
					} else {
						marshalFn = fn.Name
					}
				}

				sizeExpression := sourceType.SizeExpression
				if options.WithoutDynamicExpressions {
					sizeExpression = nil
				}

				if sourceType.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
					dynListModel := tmpl.MarshalDynamicList{
						TypeName:              typePrinter.TypeString(sourceType.Type),
						MarshalFn:             marshalFn,
						InlineItemMarshalCode: inlineMarshalCode,
						SizeExpr:              "",
					}

					if sizeExpression != nil {
						dynListModel.SizeExpr = *sizeExpression
						usedDynSsz = true
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_dynamic_list", dynListModel); err != nil {
						return nil, err
					}
				} else {
					listModel := tmpl.MarshalList{
						TypeName:              typePrinter.TypeString(sourceType.Type),
						ItemSize:              int(sourceType.ElemDesc.Size),
						MarshalFn:             marshalFn,
						InlineItemMarshalCode: inlineMarshalCode,
						SizeExpr:              "",
						IsByteArray:           sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0,
						IsString:              sourceType.GoTypeFlags&dynssz.GoTypeFlagIsString != 0,
					}

					if sizeExpression != nil {
						listModel.SizeExpr = *sizeExpression
						usedDynSsz = true
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_list", listModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszCompatibleUnionType:
				variantFns := make([]tmpl.MarshalCompatibleUnionVariant, 0, len(sourceType.UnionVariants))
				for variant, variantDesc := range sourceType.UnionVariants {
					fn, err := genRecursive(variantDesc, false)
					if err != nil {
						return nil, err
					}

					variantModel := tmpl.MarshalCompatibleUnionVariant{
						Index:    int(variant),
						TypeName: typePrinter.TypeString(variantDesc.Type),
					}

					if fn.IsInlined {
						// Generate inline code for union variants
						inlineCode := fn.InlineCode
						inlineCode = strings.ReplaceAll(inlineCode, "VAR_NAME", "v")
						variantModel.InlineMarshalCode = inlineCode
					} else {
						variantModel.MarshalFn = fn.Name
					}

					variantFns = append(variantFns, variantModel)
				}

				sort.Slice(variantFns, func(i, j int) bool {
					return variantFns[i].Index < variantFns[j].Index
				})

				compatibleUnionModel := tmpl.MarshalCompatibleUnion{
					TypeName:   typePrinter.TypeString(sourceType.Type),
					VariantFns: variantFns,
				}

				if err := codeTpl.ExecuteTemplate(&code, "marshal_compatible_union", compatibleUnionModel); err != nil {
					return nil, err
				}
			// primitive types
			case dynssz.SszBoolType:
				if err := codeTpl.ExecuteTemplate(&code, "marshal_bool", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint8Type:
				if err := codeTpl.ExecuteTemplate(&code, "marshal_uint8", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint16Type:
				if err := codeTpl.ExecuteTemplate(&code, "marshal_uint16", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint32Type:
				if err := codeTpl.ExecuteTemplate(&code, "marshal_uint32", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint64Type:
				if err := codeTpl.ExecuteTemplate(&code, "marshal_uint64", nil); err != nil {
					return nil, err
				}
			case dynssz.SszCustomType:
				code.WriteString("return errors.New(\"type does not implement ssziface.FastsszMarshaler\")\n")
			}
		}

		marshalFnIdx++
		marshalFn.Index = marshalFnIdx
		marshalFn.Name = fmt.Sprintf("fn%d", marshalFnIdx)
		marshalFn.Code = code.String()
		marshalFn.TypeName = typePrinter.TypeString(sourceType.Type)

		return marshalFn, nil
	}

	rootFn, err := genRecursive(rootTypeDesc, true)
	if err != nil {
		return false, err
	}

	marshalFnList := make([]*tmpl.MarshalFunction, 0, len(marshalFnMap))
	for _, fn := range marshalFnMap {
		marshalFnList = append(marshalFnList, fn.Fn)
	}

	sort.Slice(marshalFnList, func(i, j int) bool {
		return marshalFnList[i].Index < marshalFnList[j].Index
	})

	marshalModel := tmpl.MarshalMain{
		TypeName:         typePrinter.TypeString(rootTypeDesc.Type),
		MarshalFunctions: marshalFnList,
		RootFnName:       rootFn.Name,
		CreateLegacyFn:   options.CreateLegacyFn,
		CreateDynamicFn:  !options.WithoutDynamicExpressions,
		UsedDynSsz:       usedDynSsz,
	}

	usedDynSsz = usedDynSsz || !options.WithoutDynamicExpressions

	if err := codeTpl.ExecuteTemplate(codeBuilder, "marshal_main", marshalModel); err != nil {
		return false, err
	}

	return usedDynSsz, nil
}
