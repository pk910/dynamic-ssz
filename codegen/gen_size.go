package codegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

func generateSize(ds *dynssz.DynSsz, rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGeneratorOptions) (bool, error) {
	type sizeFnEntry struct {
		Fn   *tmpl.SizeFunction
		Type *dynssz.TypeDescriptor
	}

	codeTpl := GetTemplate("tmpl/size.tmpl")
	sizeFnMap := map[string]sizeFnEntry{}
	sizeFnIdx := 0
	usedDynSsz := false

	var genRecursive func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.SizeFunction, error)

	genRecursive = func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.SizeFunction, error) {
		typeName := typePrinter.TypeString(sourceType.Type)
		typeKey := typeName
		if sourceType.Len > 0 {
			typeKey = fmt.Sprintf("%s:%d", typeKey, sourceType.Len)
		}
		if sourceType.SizeExpression != "" && !options.WithoutDynamicExpressions {
			typeKey = fmt.Sprintf("%s:%s", typeKey, sourceType.SizeExpression)
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
		}

		if fn, found := sizeFnMap[typeKey]; found {
			return fn.Fn, nil
		}

		useFastSsz := !ds.NoFastSsz && sourceType.HasFastSSZMarshaler && !sourceType.HasDynamicSize
		if useFastSsz && sourceType.HasSizeExpr {
			useFastSsz = false
		}
		if !useFastSsz && sourceType.SszType == dynssz.SszCustomType {
			useFastSsz = true
		}

		// Check if we should use dynamic sizer - can ALWAYS be used unlike fastssz
		useDynamicSize := sourceType.HasDynamicSizer

		code := strings.Builder{}
		sizeFn := &tmpl.SizeFunction{
			Index:     0,
			Key:       typeKey,
			TypeName:  typeName,
			IsPointer: sourceType.IsPtr,
		}

		if sourceType.IsPtr {
			sizeFn.InnerType = typePrinter.TypeString(sourceType.Type.Elem())
		}

		sizeFnMap[typeKey] = sizeFnEntry{
			Fn:   sizeFn,
			Type: sourceType,
		}

		if useFastSsz && !isRoot {
			if err := codeTpl.ExecuteTemplate(&code, "size_fastssz", nil); err != nil {
				return nil, err
			}
		} else if useDynamicSize && !isRoot {
			// Use dynamic sizer - create template for this
			if err := codeTpl.ExecuteTemplate(&code, "size_dynamic", nil); err != nil {
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

				wrapperModel := tmpl.SizeWrapper{
					TypeName: typePrinter.TypeString(sourceType.Type),
					SizeFn:   fn.Name,
				}

				if err := codeTpl.ExecuteTemplate(&code, "size_wrapper", wrapperModel); err != nil {
					return nil, err
				}
			case dynssz.SszContainerType, dynssz.SszProgressiveContainerType:
				structModel := tmpl.SizeStruct{
					TypeName: typePrinter.TypeString(sourceType.Type),
					Fields:   make([]tmpl.SizeField, 0, len(sourceType.ContainerDesc.Fields)),
					Size:     0,
				}
				for idx, field := range sourceType.ContainerDesc.Fields {
					if field.Type.IsDynamic {
						fn, err := genRecursive(field.Type, false)
						if err != nil {
							return nil, err
						}

						structModel.Fields = append(structModel.Fields, tmpl.SizeField{
							Index:     idx,
							Name:      field.Name,
							TypeName:  typePrinter.TypeString(field.Type.Type),
							IsDynamic: field.Type.IsDynamic,
							SizeFn:    fn.Name,
						})

						if field.Type.IsDynamic {
							structModel.HasDynamicFields = true
						}
					} else if field.Type.HasSizeExpr {
						fn, err := genRecursive(field.Type, false)
						if err != nil {
							return nil, err
						}

						structModel.Fields = append(structModel.Fields, tmpl.SizeField{
							Index:     idx,
							Name:      field.Name,
							TypeName:  typePrinter.TypeString(field.Type.Type),
							IsDynamic: false,
							SizeFn:    fn.Name,
						})
					} else {
						structModel.Size += int(field.Type.Size)
					}
				}
				if err := codeTpl.ExecuteTemplate(&code, "size_struct", structModel); err != nil {
					return nil, err
				}
			case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
				sizeExpression := sourceType.SizeExpression
				if options.WithoutDynamicExpressions {
					sizeExpression = ""
				}
				if sizeExpression != "" {
					usedDynSsz = true
				}

				if sourceType.ElemDesc.IsDynamic {
					emptyVal := reflect.New(sourceType.ElemDesc.Type).Elem()
					emptySize, err := ds.SizeSSZ(emptyVal.Interface())
					if err != nil {
						return nil, err
					}

					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					dynVectorModel := tmpl.SizeDynamicVector{
						TypeName:  typePrinter.TypeString(sourceType.Type),
						Length:    int(sourceType.Len),
						EmptySize: emptySize,
						SizeFn:    fn.Name,
						SizeExpr:  sizeExpression,
						IsArray:   sourceType.Kind == reflect.Array,
					}

					if err := codeTpl.ExecuteTemplate(&code, "size_dynamic_vector", dynVectorModel); err != nil {
						return nil, err
					}
				} else {
					sizeFn := ""
					if sourceType.ElemDesc.HasSizeExpr {
						fn, err := genRecursive(sourceType.ElemDesc, false)
						if err != nil {
							return nil, err
						}
						sizeFn = fn.Name
					}

					vectorModel := tmpl.SizeVector{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						Length:      int(sourceType.Len),
						ItemSize:    int(sourceType.ElemDesc.Size),
						SizeFn:      sizeFn,
						SizeExpr:    sizeExpression,
						IsArray:     sourceType.Kind == reflect.Array,
						IsByteArray: sourceType.IsByteArray,
						IsString:    sourceType.IsString,
					}

					if err := codeTpl.ExecuteTemplate(&code, "size_vector", vectorModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
				sizeExpression := sourceType.SizeExpression
				if options.WithoutDynamicExpressions {
					sizeExpression = ""
				}
				if sizeExpression != "" {
					usedDynSsz = true
				}

				if sourceType.ElemDesc.IsDynamic {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					dynListModel := tmpl.SizeDynamicList{
						TypeName: typePrinter.TypeString(sourceType.Type),
						SizeFn:   fn.Name,
						SizeExpr: sizeExpression,
					}

					if err := codeTpl.ExecuteTemplate(&code, "size_dynamic_list", dynListModel); err != nil {
						return nil, err
					}
				} else {
					sizeFn := ""
					if sourceType.ElemDesc.HasSizeExpr {
						fn, err := genRecursive(sourceType.ElemDesc, false)
						if err != nil {
							return nil, err
						}
						sizeFn = fn.Name
					}

					listModel := tmpl.SizeList{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						ItemSize:    int(sourceType.ElemDesc.Size),
						SizeFn:      sizeFn,
						SizeExpr:    sizeExpression,
						IsByteArray: sourceType.IsByteArray,
						IsString:    sourceType.IsString,
					}

					if err := codeTpl.ExecuteTemplate(&code, "size_list", listModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszCompatibleUnionType:
				compatibleUnionModel := tmpl.SizeCompatibleUnion{
					TypeName:   typePrinter.TypeString(sourceType.Type),
					VariantFns: make([]tmpl.SizeCompatibleUnionVariant, 0, len(sourceType.UnionVariants)),
				}
				for variant, variantDesc := range sourceType.UnionVariants {
					fn, err := genRecursive(variantDesc, false)
					if err != nil {
						return nil, err
					}

					compatibleUnionModel.VariantFns = append(compatibleUnionModel.VariantFns, tmpl.SizeCompatibleUnionVariant{
						Index:    int(variant),
						TypeName: typePrinter.TypeString(variantDesc.Type),
						SizeFn:   fn.Name,
					})
				}
				if err := codeTpl.ExecuteTemplate(&code, "size_compatible_union", compatibleUnionModel); err != nil {
					return nil, err
				}
			// primitive types
			case dynssz.SszBoolType:
				if err := codeTpl.ExecuteTemplate(&code, "size_bool", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint8Type:
				if err := codeTpl.ExecuteTemplate(&code, "size_uint8", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint16Type:
				if err := codeTpl.ExecuteTemplate(&code, "size_uint16", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint32Type:
				if err := codeTpl.ExecuteTemplate(&code, "size_uint32", nil); err != nil {
					return nil, err
				}
			case dynssz.SszUint64Type:
				if err := codeTpl.ExecuteTemplate(&code, "size_uint64", nil); err != nil {
					return nil, err
				}
			case dynssz.SszCustomType:
				code.WriteString("return errors.New(\"type does not implement ssziface.FastsszMarshaler\")\n")
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

	sizeFnList := make([]*tmpl.SizeFunction, 0, len(sizeFnMap))
	for _, fn := range sizeFnMap {
		sizeFnList = append(sizeFnList, fn.Fn)
	}

	sort.Slice(sizeFnList, func(i, j int) bool {
		return sizeFnList[i].Index < sizeFnList[j].Index
	})

	sizeModel := tmpl.SizeMain{
		TypeName:        typePrinter.TypeString(rootTypeDesc.Type),
		SizeFunctions:   sizeFnList,
		RootFnName:      rootFn.Name,
		CreateLegacyFn:  options.CreateLegacyFn,
		CreateDynamicFn: !options.WithoutDynamicExpressions,
		UsedDynSsz:      usedDynSsz,
	}

	if err := codeTpl.ExecuteTemplate(codeBuilder, "size_main", sizeModel); err != nil {
		return false, err
	}

	return usedDynSsz, nil
}
