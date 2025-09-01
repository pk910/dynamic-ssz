package codegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

func generateUnmarshal(ds *dynssz.DynSsz, rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
	type unmarshalFnEntry struct {
		Fn   *tmpl.UnmarshalFunction
		Type *dynssz.TypeDescriptor
	}

	type sizeFnEntry struct {
		Fn   *tmpl.UnmarshalStaticSizeFunction
		Type *dynssz.TypeDescriptor
	}

	codeTpl := GetTemplate("tmpl/unmarshal.tmpl", "tmpl/size.tmpl")
	unmarshalFnMap := map[string]unmarshalFnEntry{}
	sizeFnMap := map[string]sizeFnEntry{}
	unmarshalFnIdx := 0
	sizeFnIdx := 0
	usedDynSsz := false

	var genRecursive func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.UnmarshalFunction, error)

	var genRecursiveStaticSize func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.UnmarshalStaticSizeFunction, error)

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

	getInlineBaseTypeUnmarshal := func(sourceType *dynssz.TypeDescriptor, varName string, bufExpr string, knownSize int) string {
		compactBufExpr := strings.ReplaceAll(bufExpr, " ", "")

		// Handle byte arrays/slices
		if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0 {
			if sourceType.Kind == reflect.Array {
				// For arrays, just copy
				return fmt.Sprintf("copy(%s[:], %s)", varName, compactBufExpr)
			} else {
				// For slices, allocate if needed and copy
				typeName := typePrinter.TypeString(sourceType.Type)
				if knownSize > 0 {
					// Use static size when known
					return fmt.Sprintf("if len(%s) < %d {\n\t%s = make(%s, %d)\n} else {\n\t%s = %s[:%d]\n}\ncopy(%s[:], %s)",
						varName, knownSize, varName, typeName, knownSize, varName, varName, knownSize, varName, compactBufExpr)
				} else {
					// Fall back to dynamic size calculation
					return fmt.Sprintf("if len(%s) < len(%s) {\n\t%s = make(%s, len(%s))\n} else {\n\t%s = %s[:len(%s)]\n}\ncopy(%s[:], %s)",
						varName, compactBufExpr, varName, typeName, compactBufExpr, varName, varName, compactBufExpr, varName, compactBufExpr)
				}
			}
		}

		// Handle primitive types
		typeName := typePrinter.TypeString(sourceType.Type)
		switch sourceType.SszType {
		case dynssz.SszBoolType:
			return fmt.Sprintf("%s = (%s)(sszutils.UnmarshalBool(%s))", varName, typeName, bufExpr)
		case dynssz.SszUint8Type:
			return fmt.Sprintf("%s = (%s)(sszutils.UnmarshallUint8(%s))", varName, typeName, bufExpr)
		case dynssz.SszUint16Type:
			return fmt.Sprintf("%s = (%s)(sszutils.UnmarshallUint16(%s))", varName, typeName, bufExpr)
		case dynssz.SszUint32Type:
			return fmt.Sprintf("%s = (%s)(sszutils.UnmarshallUint32(%s))", varName, typeName, bufExpr)
		case dynssz.SszUint64Type:
			return fmt.Sprintf("%s = (%s)(sszutils.UnmarshallUint64(%s))", varName, typeName, bufExpr)
		default:
			return ""
		}
	}

	genRecursive = func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.UnmarshalFunction, error) {
		// For base types that are not root, return a special inline function
		// Aggressive inlining: inline ALL base types including byte slices in vectors
		if !isRoot && isBaseType(sourceType) && sourceType.GoTypeFlags&dynssz.GoTypeFlagIsPointer == 0 {
			return &tmpl.UnmarshalFunction{
				IsInlined:  true,
				InlineCode: getInlineBaseTypeUnmarshal(sourceType, "VAR_NAME", "BUF_EXPR", 0),
			}, nil
		}

		typeName := typePrinter.TypeString(sourceType.Type)
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

		if fn, found := unmarshalFnMap[typeKey]; found {
			return fn.Fn, nil
		}

		hasDynamicSize := sourceType.SszTypeFlags&dynssz.SszTypeFlagHasDynamicSize != 0
		isFastsszUnmarshaler := sourceType.SszCompatFlags&dynssz.SszCompatFlagFastSSZMarshaler != 0
		useDynamicUnmarshal := sourceType.SszCompatFlags&dynssz.SszCompatFlagDynamicUnmarshaler != 0
		useFastSsz := !ds.NoFastSsz && isFastsszUnmarshaler && !hasDynamicSize
		if useFastSsz && sourceType.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 {
			useFastSsz = false
		}
		if !useFastSsz && sourceType.SszType == dynssz.SszCustomType {
			useFastSsz = true
		}

		code := strings.Builder{}
		unmarshalFn := &tmpl.UnmarshalFunction{
			Index:     0,
			Key:       typeKey,
			TypeName:  typeName,
			UsedValue: true,
		}
		if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsPointer != 0 {
			unmarshalFn.IsPointer = true
			unmarshalFn.InnerType = typePrinter.TypeString(sourceType.Type.Elem())
		}

		unmarshalFnMap[typeKey] = unmarshalFnEntry{
			Fn:   unmarshalFn,
			Type: sourceType,
		}

		if useFastSsz && !isRoot {
			if err := codeTpl.ExecuteTemplate(&code, "unmarshal_fastssz", nil); err != nil {
				return nil, err
			}
		} else if useDynamicUnmarshal && !isRoot {
			// Use dynamic unmarshaler - create template for this
			if err := codeTpl.ExecuteTemplate(&code, "unmarshal_dynamic", nil); err != nil {
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

				wrapperModel := tmpl.UnmarshalWrapper{
					TypeName:    typePrinter.TypeString(sourceType.Type),
					UnmarshalFn: fn.Name,
				}

				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_wrapper", wrapperModel); err != nil {
					return nil, err
				}
			case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
				structModel := tmpl.UnmarshalStruct{
					TypeName:      typePrinter.TypeString(sourceType.Type),
					Fields:        make([]tmpl.UnmarshalField, 0, len(sourceType.ContainerDesc.Fields)),
					StaticOffsets: make([]int, len(sourceType.ContainerDesc.Fields)),
				}
				lastDynamic := -1
				currentOffset := 0
				hasDynamicSizes := false

				// First pass: calculate static offsets and detect dynamic sizes
				for idx, field := range sourceType.ContainerDesc.Fields {
					structModel.StaticOffsets[idx] = currentOffset

					if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
						currentOffset += 4 // offset field
					} else if field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !options.WithoutDynamicExpressions {
						hasDynamicSizes = true
						// For dynamic sizes, we can't calculate static offsets beyond this point
						break
					} else {
						currentOffset += int(field.Type.Size)
					}
				}
				structModel.HasDynamicSizes = hasDynamicSizes

				// Second pass: generate field models
				for idx, field := range sourceType.ContainerDesc.Fields {
					fn, err := genRecursive(field.Type, false)
					if err != nil {
						return nil, err
					}

					fieldModel := tmpl.UnmarshalField{
						Index:     idx,
						Name:      field.Name,
						IsDynamic: field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0,
						Size:      int(field.Type.Size),
					}

					if fn.IsInlined {
						// Use inline code for base types
						varName := fmt.Sprintf("t.%s", field.Name)
						var inlineCode string

						if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
							// For dynamic fields, the buffer is already sliced as 'fieldSlice'
							inlineCode = getInlineBaseTypeUnmarshal(field.Type, varName, "fieldSlice", 0)
						} else if hasDynamicSizes {
							// For static fields with dynamic sizes, use bufpos tracking
							inlineCode = getInlineBaseTypeUnmarshal(field.Type, varName, "buf[bufpos : bufpos+fieldsize]", 0)
						} else {
							// For static fields with static sizes, use direct offsets with known size
							offset := structModel.StaticOffsets[idx]
							fieldSize := int(field.Type.Size)
							bufExpr := fmt.Sprintf("buf[%d:%d]", offset, offset+fieldSize)
							inlineCode = getInlineBaseTypeUnmarshal(field.Type, varName, bufExpr, fieldSize)
						}
						fieldModel.InlineUnmarshalCode = inlineCode
					} else {
						fieldModel.TypeName = typePrinter.TypeString(field.Type.Type)
						fieldModel.UnmarshalFn = fn.Name
					}

					structModel.Fields = append(structModel.Fields, fieldModel)

					if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
						structModel.HasDynamicFields = true
						structModel.Size += 4
						if lastDynamic > -1 {
							structModel.Fields[lastDynamic].NextDynamic = idx
						}
						lastDynamic = idx
					} else if field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !options.WithoutDynamicExpressions {
						sizeFn, err := genRecursiveStaticSize(field.Type, false)
						if err != nil {
							return nil, err
						}
						structModel.Fields[idx].SizeFn = sizeFn
					} else {
						structModel.Size += int(field.Type.Size)
					}
				}
				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_struct", structModel); err != nil {
					return nil, err
				}
			case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
				unmarshalFn := ""
				inlineUnmarshalCode := ""
				if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray == 0 {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					if fn.IsInlined {
						// Generate inline code for vectors with known element size
						elemSize := int(sourceType.ElemDesc.Size)
						inlineUnmarshalCode = getInlineBaseTypeUnmarshal(sourceType.ElemDesc, "t[i]", "buf[i*itemsize : (i+1)*itemsize]", elemSize)
					} else {
						unmarshalFn = fn.Name
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

					dynVectorModel := tmpl.UnmarshalDynamicVector{
						Length:      int(sourceType.Len),
						EmptySize:   emptySize,
						UnmarshalFn: unmarshalFn,
						SizeExpr:    "",
						IsArray:     sourceType.Kind == reflect.Array,
					}

					if sizeExpression != nil {
						dynVectorModel.SizeExpr = *sizeExpression
						usedDynSsz = true
					}

					if !dynVectorModel.IsArray {
						dynVectorModel.TypeName = typePrinter.TypeString(sourceType.Type)
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_dynamic_vector", dynVectorModel); err != nil {
						return nil, err
					}
				} else {
					vectorModel := tmpl.UnmarshalVector{
						Length:                  int(sourceType.Len),
						ItemSize:                int(sourceType.ElemDesc.Size),
						UnmarshalFn:             unmarshalFn,
						InlineItemUnmarshalCode: inlineUnmarshalCode,
						IsArray:                 sourceType.Kind == reflect.Array,
						IsByteArray:             sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0,
						IsString:                sourceType.GoTypeFlags&dynssz.GoTypeFlagIsString != 0,
					}

					if !vectorModel.IsArray && !vectorModel.IsString {
						vectorModel.TypeName = typePrinter.TypeString(sourceType.Type)
					}

					if sourceType.SizeExpression != nil && !options.WithoutDynamicExpressions {
						fn, err := genRecursiveStaticSize(sourceType, false)
						if err != nil {
							return nil, err
						}
						vectorModel.SizeFn = fn
					}

					if sourceType.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !options.WithoutDynamicExpressions {
						fn, err := genRecursiveStaticSize(sourceType.ElemDesc, false)
						if err != nil {
							return nil, err
						}
						vectorModel.ItemSizeFn = fn
					}

					if err := codeTpl.ExecuteTemplate(&code, "unmarshal_vector", vectorModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
				unmarshalFn := ""
				inlineUnmarshalCode := ""
				inlineUnmarshalCodeDynamic := ""
				if sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray == 0 {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					if fn.IsInlined {
						// Generate inline code for static lists with known element size
						elemSize := int(sourceType.ElemDesc.Size)
						inlineUnmarshalCode = getInlineBaseTypeUnmarshal(sourceType.ElemDesc, "t[i]", "buf[i*itemsize : (i+1)*itemsize]", elemSize)

						// Generate inline code for dynamic lists (unknown size at this point)
						inlineUnmarshalCodeDynamic = getInlineBaseTypeUnmarshal(sourceType.ElemDesc, "t[i]", "buf[offset : endOffset]", 0)
					} else {
						unmarshalFn = fn.Name
					}
				}

				sizeExpression := sourceType.SizeExpression
				if options.WithoutDynamicExpressions {
					sizeExpression = nil
				}

				if sourceType.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
					dynListModel := tmpl.UnmarshalDynamicList{
						TypeName:                typePrinter.TypeString(sourceType.Type),
						UnmarshalFn:             unmarshalFn,
						InlineItemUnmarshalCode: inlineUnmarshalCodeDynamic,
						SizeExpr:                "",
					}

					if sizeExpression != nil {
						dynListModel.SizeExpr = *sizeExpression
						usedDynSsz = true
					}

					if err := codeTpl.ExecuteTemplate(&code, "unmarshal_dynamic_list", dynListModel); err != nil {
						return nil, err
					}
				} else {
					listModel := tmpl.UnmarshalList{
						TypeName:                typePrinter.TypeString(sourceType.Type),
						ItemSize:                int(sourceType.ElemDesc.Size),
						UnmarshalFn:             unmarshalFn,
						InlineItemUnmarshalCode: inlineUnmarshalCode,
						SizeExpr:                "",
						IsByteArray:             sourceType.GoTypeFlags&dynssz.GoTypeFlagIsByteArray != 0,
						IsString:                sourceType.GoTypeFlags&dynssz.GoTypeFlagIsString != 0,
					}

					if sizeExpression != nil {
						listModel.SizeExpr = *sizeExpression
						usedDynSsz = true
					}

					if sourceType.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !options.WithoutDynamicExpressions {
						fn, err := genRecursiveStaticSize(sourceType.ElemDesc, false)
						if err != nil {
							return nil, err
						}
						listModel.SizeFn = fn
					}

					if err := codeTpl.ExecuteTemplate(&code, "unmarshal_list", listModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszCompatibleUnionType:
				compatibleUnionModel := tmpl.UnmarshalCompatibleUnion{
					TypeName:   typePrinter.TypeString(sourceType.Type),
					VariantFns: make([]tmpl.UnmarshalCompatibleUnionVariant, 0, len(sourceType.UnionVariants)),
				}
				for variant, variantDesc := range sourceType.UnionVariants {
					fn, err := genRecursive(variantDesc, false)
					if err != nil {
						return nil, err
					}

					variantModel := tmpl.UnmarshalCompatibleUnionVariant{
						Index:    int(variant),
						TypeName: typePrinter.TypeString(variantDesc.Type),
					}

					if fn.IsInlined {
						// Generate inline code for union variants
						variantSize := int(variantDesc.Size)
						variantModel.InlineUnmarshalCode = getInlineBaseTypeUnmarshal(variantDesc, "v", "buf[1:]", variantSize)
					} else {
						variantModel.UnmarshalFn = fn.Name
					}

					compatibleUnionModel.VariantFns = append(compatibleUnionModel.VariantFns, variantModel)
				}

				sort.Slice(compatibleUnionModel.VariantFns, func(i, j int) bool {
					return compatibleUnionModel.VariantFns[i].Index < compatibleUnionModel.VariantFns[j].Index
				})

				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_compatible_union", compatibleUnionModel); err != nil {
					return nil, err
				}
			// primitive types
			case dynssz.SszBoolType:
				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_bool", tmpl.UnmarshalPrimitive{
					TypeName: typePrinter.TypeString(sourceType.Type),
				}); err != nil {
					return nil, err
				}
				unmarshalFn.UsedValue = false
			case dynssz.SszUint8Type:
				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_uint8", tmpl.UnmarshalPrimitive{
					TypeName: typePrinter.TypeString(sourceType.Type),
				}); err != nil {
					return nil, err
				}
				unmarshalFn.UsedValue = false
			case dynssz.SszUint16Type:
				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_uint16", tmpl.UnmarshalPrimitive{
					TypeName: typePrinter.TypeString(sourceType.Type),
				}); err != nil {
					return nil, err
				}
				unmarshalFn.UsedValue = false
			case dynssz.SszUint32Type:
				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_uint32", tmpl.UnmarshalPrimitive{
					TypeName: typePrinter.TypeString(sourceType.Type),
				}); err != nil {
					return nil, err
				}
				unmarshalFn.UsedValue = false
			case dynssz.SszUint64Type:
				if err := codeTpl.ExecuteTemplate(&code, "unmarshal_uint64", tmpl.UnmarshalPrimitive{
					TypeName: typePrinter.TypeString(sourceType.Type),
				}); err != nil {
					return nil, err
				}
				unmarshalFn.UsedValue = false
			case dynssz.SszCustomType:
				code.WriteString("return errors.New(\"type does not implement ssziface.FastsszMarshaler\")\n")
			}
		}

		unmarshalFnIdx++
		unmarshalFn.Index = unmarshalFnIdx
		unmarshalFn.Name = fmt.Sprintf("fn%d", unmarshalFnIdx)
		unmarshalFn.Code = code.String()

		return unmarshalFn, nil
	}

	genRecursiveStaticSize = func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.UnmarshalStaticSizeFunction, error) {
		typeName := typePrinter.TypeString(sourceType.Type)
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

		if fn, found := sizeFnMap[typeKey]; found {
			return fn.Fn, nil
		}

		code := strings.Builder{}
		sizeFn := &tmpl.UnmarshalStaticSizeFunction{
			Index:    0,
			Key:      typeKey,
			TypeName: typeName,
		}

		sizeFnMap[typeKey] = sizeFnEntry{
			Fn:   sizeFn,
			Type: sourceType,
		}

		if sourceType.Type.Name() != "" && !isRoot {
			// do not recurse into non-root types

		}

		switch sourceType.SszType {
		// complex types
		case dynssz.SszTypeWrapperType:
			fn, err := genRecursiveStaticSize(sourceType.ElemDesc, false)
			if err != nil {
				return nil, err
			}

			wrapperModel := tmpl.UnmarshalStaticSizeWrapper{
				TypeName: typePrinter.TypeString(sourceType.Type),
				SizeFn:   fn.Name,
			}

			if err := codeTpl.ExecuteTemplate(&code, "unmarshal_size_wrapper", wrapperModel); err != nil {
				return nil, err
			}
		case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
			structModel := tmpl.UnmarshalStaticSizeStruct{
				TypeName: typePrinter.TypeString(sourceType.Type),
				Fields:   make([]tmpl.UnmarshalStaticSizeField, 0, len(sourceType.ContainerDesc.Fields)),
				Size:     0,
			}
			for idx, field := range sourceType.ContainerDesc.Fields {
				if field.Type.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
					return nil, fmt.Errorf("dynamic field not supported for static size calculation")
				} else if field.Type.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !options.WithoutDynamicExpressions {
					fn, err := genRecursiveStaticSize(field.Type, false)
					if err != nil {
						return nil, err
					}

					structModel.Fields = append(structModel.Fields, tmpl.UnmarshalStaticSizeField{
						Index:    idx,
						Name:     field.Name,
						TypeName: typePrinter.TypeString(field.Type.Type),
						SizeFn:   fn.Name,
					})
				} else {
					structModel.Size += int(field.Type.Size)
				}
			}
			if err := codeTpl.ExecuteTemplate(&code, "unmarshal_size_struct", structModel); err != nil {
				return nil, err
			}
		case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
			sizeExpression := sourceType.SizeExpression
			if options.WithoutDynamicExpressions {
				sizeExpression = nil
			}

			if sourceType.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagIsDynamic != 0 {
				return nil, fmt.Errorf("dynamic vector not supported for static size calculation")
			} else {
				sizeFn := ""
				if sourceType.ElemDesc.SszTypeFlags&dynssz.SszTypeFlagHasSizeExpr != 0 && !options.WithoutDynamicExpressions {
					fn, err := genRecursiveStaticSize(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}
					sizeFn = fn.Name
				}

				vectorModel := tmpl.UnmarshalStaticSizeVector{
					TypeName: typePrinter.TypeString(sourceType.Type),
					Length:   int(sourceType.Len),
					ItemSize: int(sourceType.ElemDesc.Size),
					SizeFn:   sizeFn,
					SizeExpr: "",
				}

				if sizeExpression != nil {
					vectorModel.SizeExpr = *sizeExpression
					usedDynSsz = true
				}

				if err := codeTpl.ExecuteTemplate(&code, "size_vector", vectorModel); err != nil {
					return nil, err
				}
			}
		}

		sizeFnIdx++
		sizeFn.Index = sizeFnIdx
		sizeFn.Name = fmt.Sprintf("sfn%d", sizeFnIdx)
		sizeFn.Code = code.String()

		return sizeFn, nil
	}

	rootFn, err := genRecursive(rootTypeDesc, true)
	if err != nil {
		return false, err
	}

	unmarshalFnList := make([]*tmpl.UnmarshalFunction, 0, len(unmarshalFnMap))
	for _, fn := range unmarshalFnMap {
		unmarshalFnList = append(unmarshalFnList, fn.Fn)
	}

	sort.Slice(unmarshalFnList, func(i, j int) bool {
		return unmarshalFnList[i].Index < unmarshalFnList[j].Index
	})

	sizeFnList := make([]*tmpl.UnmarshalStaticSizeFunction, 0, len(sizeFnMap))
	for _, fn := range sizeFnMap {
		sizeFnList = append(sizeFnList, fn.Fn)
	}

	sort.Slice(sizeFnList, func(i, j int) bool {
		return sizeFnList[i].Index < sizeFnList[j].Index
	})

	unmarshalModel := tmpl.UnmarshalMain{
		TypeName:            typePrinter.TypeString(rootTypeDesc.Type),
		StaticSizeFunctions: sizeFnList,
		UnmarshalFunctions:  unmarshalFnList,
		RootFnName:          rootFn.Name,
		CreateLegacyFn:      options.CreateLegacyFn,
		CreateDynamicFn:     !options.WithoutDynamicExpressions,
		UsedDynSsz:          usedDynSsz,
	}

	usedDynSsz = usedDynSsz || !options.WithoutDynamicExpressions

	if err := codeTpl.ExecuteTemplate(codeBuilder, "unmarshal_main", unmarshalModel); err != nil {
		return false, err
	}

	return usedDynSsz, nil
}
