package codegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

func generateUnmarshal(ds *dynssz.DynSsz, rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGenOptions) (bool, error) {
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

	genRecursive = func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.UnmarshalFunction, error) {
		typeName := typePrinter.TypeString(sourceType.Type)
		typeKey := typeName
		if sourceType.Len > 0 {
			typeKey = fmt.Sprintf("%s:%d", typeKey, sourceType.Len)
		}
		if sourceType.SizeExpression != "" {
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
			if childType.SizeExpression != "" {
				typeKey = fmt.Sprintf("%s:%s", typeKey, childType.SizeExpression)
			}
		}

		if fn, found := unmarshalFnMap[typeKey]; found {
			return fn.Fn, nil
		}

		useFastSsz := !ds.NoFastSsz && sourceType.HasFastSSZMarshaler && !sourceType.HasDynamicSize
		if useFastSsz && sourceType.HasSizeExpr {
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
			IsPointer: sourceType.IsPtr,
			UsedValue: true,
		}
		if sourceType.IsPtr {
			unmarshalFn.InnerType = typePrinter.TypeString(sourceType.Type.Elem())
		}

		unmarshalFnMap[typeKey] = unmarshalFnEntry{
			Fn:   unmarshalFn,
			Type: sourceType,
		}

		if useFastSsz {
			if err := codeTpl.ExecuteTemplate(&code, "unmarshal_fastssz", nil); err != nil {
				return nil, err
			}
		} else {

			if sourceType.Type.Name() != "" && !isRoot {
				// do not recurse into non-root types

			}

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
					TypeName: typePrinter.TypeString(sourceType.Type),
					Fields:   make([]tmpl.UnmarshalField, 0, len(sourceType.ContainerDesc.Fields)),
				}
				lastDynamic := -1
				for idx, field := range sourceType.ContainerDesc.Fields {
					fn, err := genRecursive(field.Type, false)
					if err != nil {
						return nil, err
					}

					structModel.Fields = append(structModel.Fields, tmpl.UnmarshalField{
						Index:       idx,
						Name:        field.Name,
						TypeName:    typePrinter.TypeString(field.Type.Type),
						IsDynamic:   field.Type.IsDynamic,
						Size:        int(field.Type.Size),
						UnmarshalFn: fn.Name,
					})

					if field.Type.IsDynamic {
						structModel.HasDynamicFields = true
						structModel.Size += 4
						if lastDynamic > -1 {
							structModel.Fields[lastDynamic].NextDynamic = idx
						}
						lastDynamic = idx
					} else if field.Type.HasSizeExpr {
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
				if !sourceType.IsByteArray {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					unmarshalFn = fn.Name
				}

				if sourceType.SizeExpression != "" {
					usedDynSsz = true
				}

				if sourceType.ElemDesc.IsDynamic {
					emptyVal := reflect.New(sourceType.ElemDesc.Type).Elem()
					emptySize, err := ds.SizeSSZ(emptyVal.Interface())
					if err != nil {
						return nil, err
					}

					dynVectorModel := tmpl.UnmarshalDynamicVector{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						Length:      int(sourceType.Len),
						EmptySize:   emptySize,
						UnmarshalFn: unmarshalFn,
						SizeExpr:    sourceType.SizeExpression,
						IsArray:     sourceType.Kind == reflect.Array,
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_dynamic_vector", dynVectorModel); err != nil {
						return nil, err
					}
				} else {
					vectorModel := tmpl.UnmarshalVector{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						Length:      int(sourceType.Len),
						ItemSize:    int(sourceType.ElemDesc.Size),
						UnmarshalFn: unmarshalFn,
						IsArray:     sourceType.Kind == reflect.Array,
						IsByteArray: sourceType.IsByteArray,
						IsString:    sourceType.IsString,
					}

					if sourceType.SizeExpression != "" {
						fn, err := genRecursiveStaticSize(sourceType, false)
						if err != nil {
							return nil, err
						}
						vectorModel.SizeFn = fn

					}

					if sourceType.ElemDesc.HasSizeExpr {
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
				if !sourceType.IsByteArray {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					unmarshalFn = fn.Name
				}

				if sourceType.SizeExpression != "" {
					usedDynSsz = true
				}

				if sourceType.ElemDesc.IsDynamic {
					dynListModel := tmpl.UnmarshalDynamicList{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						UnmarshalFn: unmarshalFn,
						SizeExpr:    sourceType.SizeExpression,
					}

					if err := codeTpl.ExecuteTemplate(&code, "unmarshal_dynamic_list", dynListModel); err != nil {
						return nil, err
					}
				} else {
					listModel := tmpl.UnmarshalList{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						ItemSize:    int(sourceType.ElemDesc.Size),
						UnmarshalFn: unmarshalFn,
						SizeExpr:    sourceType.SizeExpression,
						IsByteArray: sourceType.IsByteArray,
						IsString:    sourceType.IsString,
					}

					if sourceType.ElemDesc.HasSizeExpr {
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

					compatibleUnionModel.VariantFns = append(compatibleUnionModel.VariantFns, tmpl.UnmarshalCompatibleUnionVariant{
						Index:       int(variant),
						TypeName:    typePrinter.TypeString(variantDesc.Type),
						UnmarshalFn: fn.Name,
					})
				}
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
		if sourceType.SizeExpression != "" {
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
			if childType.SizeExpression != "" {
				typeKey = fmt.Sprintf("%s:%s", typeKey, childType.SizeExpression)
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
				if field.Type.IsDynamic {
					return nil, fmt.Errorf("dynamic field not supported for static size calculation")
				} else if field.Type.HasSizeExpr {
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
			if sourceType.SizeExpression != "" {
				usedDynSsz = true
			}

			if sourceType.ElemDesc.IsDynamic {
				return nil, fmt.Errorf("dynamic vector not supported for static size calculation")
			} else {
				sizeFn := ""
				if sourceType.ElemDesc.HasSizeExpr {
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
					SizeExpr: sourceType.SizeExpression,
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
		CreateDynamicFn:     options.CreateDynamicFn,
		UsedDynSsz:          usedDynSsz,
	}

	if err := codeTpl.ExecuteTemplate(codeBuilder, "unmarshal_main", unmarshalModel); err != nil {
		return false, err
	}

	return usedDynSsz, nil
}
