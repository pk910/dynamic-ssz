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

	codeTpl := GetTemplate("tmpl/unmarshal.tmpl")
	unmarshalFnMap := map[string]unmarshalFnEntry{}
	unmarshalFnIdx := 0
	usedDynSsz := false

	var genRecursive func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.UnmarshalFunction, error)

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
		if !useFastSsz && sourceType.HasSizeExpr {
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
			case dynssz.SszContainerType:
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
						SizeExpr:    sourceType.SizeExpression,
						IsArray:     sourceType.Kind == reflect.Array,
						IsByteArray: sourceType.IsByteArray,
						IsString:    sourceType.IsString,
					}

					if err := codeTpl.ExecuteTemplate(&code, "unmarshal_vector", vectorModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszListType, dynssz.SszBitlistType:
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

					if err := codeTpl.ExecuteTemplate(&code, "unmarshal_list", listModel); err != nil {
						return nil, err
					}
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

	unmarshalModel := tmpl.UnmarshalMain{
		TypeName:           typePrinter.TypeString(rootTypeDesc.Type),
		UnmarshalFunctions: unmarshalFnList,
		RootFnName:         rootFn.Name,
		CreateLegacyFn:     options.CreateLegacyFn,
		CreateDynamicFn:    options.CreateDynamicFn,
		UsedDynSsz:         usedDynSsz,
	}

	if err := codeTpl.ExecuteTemplate(codeBuilder, "unmarshal_main", unmarshalModel); err != nil {
		return false, err
	}

	return usedDynSsz, nil
}
