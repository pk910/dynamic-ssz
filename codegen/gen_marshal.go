package codegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen/tmpl"
)

func generateMarshal(ds *dynssz.DynSsz, rootTypeDesc *dynssz.TypeDescriptor, codeBuilder *strings.Builder, typePrinter *TypePrinter, options *CodeGenOptions) (bool, error) {
	type marshalFnEntry struct {
		Fn   *tmpl.MarshalFunction
		Type *dynssz.TypeDescriptor
	}

	codeTpl := GetTemplate("tmpl/marshal.tmpl")
	marshalFnMap := map[string]marshalFnEntry{}
	marshalFnIdx := 0
	usedDynSsz := false

	var genRecursive func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.MarshalFunction, error)

	genRecursive = func(sourceType *dynssz.TypeDescriptor, isRoot bool) (*tmpl.MarshalFunction, error) {
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

		if fn, found := marshalFnMap[typeKey]; found {
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
		marshalFn := &tmpl.MarshalFunction{
			Index:    0,
			Key:      typeKey,
			TypeName: typeName,
		}
		marshalFnMap[typeKey] = marshalFnEntry{
			Fn:   marshalFn,
			Type: sourceType,
		}

		if useFastSsz {
			if err := codeTpl.ExecuteTemplate(&code, "marshal_fastssz", nil); err != nil {
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

					structModel.Fields = append(structModel.Fields, tmpl.MarshalField{
						Index:     idx,
						Name:      field.Name,
						TypeName:  typePrinter.TypeString(field.Type.Type),
						IsDynamic: field.Type.IsDynamic,
						Size:      int(field.Type.Size),
						MarshalFn: fn.Name,
					})

					if field.Type.IsDynamic {
						structModel.HasDynamicFields = true
					}
				}
				if err := codeTpl.ExecuteTemplate(&code, "marshal_struct", structModel); err != nil {
					return nil, err
				}
			case dynssz.SszVectorType, dynssz.SszBitvectorType, dynssz.SszUint128Type, dynssz.SszUint256Type:
				marshalFn := ""
				if !sourceType.IsByteArray {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					marshalFn = fn.Name
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

					dynVectorModel := tmpl.MarshalDynamicVector{
						TypeName:  typePrinter.TypeString(sourceType.Type),
						Length:    int(sourceType.Len),
						EmptySize: emptySize,
						MarshalFn: marshalFn,
						SizeExpr:  sourceType.SizeExpression,
						IsArray:   sourceType.Kind == reflect.Array,
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_dynamic_vector", dynVectorModel); err != nil {
						return nil, err
					}
				} else {
					vectorModel := tmpl.MarshalVector{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						Length:      int(sourceType.Len),
						ItemSize:    int(sourceType.ElemDesc.Size),
						MarshalFn:   marshalFn,
						SizeExpr:    sourceType.SizeExpression,
						IsArray:     sourceType.Kind == reflect.Array,
						IsByteArray: sourceType.IsByteArray,
						IsString:    sourceType.IsString,
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_vector", vectorModel); err != nil {
						return nil, err
					}
				}
			case dynssz.SszListType, dynssz.SszBitlistType, dynssz.SszProgressiveListType, dynssz.SszProgressiveBitlistType:
				marshalFn := ""
				if !sourceType.IsByteArray {
					fn, err := genRecursive(sourceType.ElemDesc, false)
					if err != nil {
						return nil, err
					}

					marshalFn = fn.Name
				}

				if sourceType.SizeExpression != "" {
					usedDynSsz = true
				}

				if sourceType.ElemDesc.IsDynamic {
					dynListModel := tmpl.MarshalDynamicList{
						TypeName:  typePrinter.TypeString(sourceType.Type),
						MarshalFn: marshalFn,
						SizeExpr:  sourceType.SizeExpression,
					}

					if err := codeTpl.ExecuteTemplate(&code, "marshal_dynamic_list", dynListModel); err != nil {
						return nil, err
					}
				} else {
					listModel := tmpl.MarshalList{
						TypeName:    typePrinter.TypeString(sourceType.Type),
						ItemSize:    int(sourceType.ElemDesc.Size),
						MarshalFn:   marshalFn,
						SizeExpr:    sourceType.SizeExpression,
						IsByteArray: sourceType.IsByteArray,
						IsString:    sourceType.IsString,
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
					variantFns = append(variantFns, tmpl.MarshalCompatibleUnionVariant{
						Index:     int(variant),
						TypeName:  typePrinter.TypeString(variantDesc.Type),
						MarshalFn: fn.Name,
					})
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
		CreateDynamicFn:  options.CreateDynamicFn,
		UsedDynSsz:       usedDynSsz,
	}

	if err := codeTpl.ExecuteTemplate(codeBuilder, "marshal_main", marshalModel); err != nil {
		return false, err
	}

	return usedDynSsz, nil
}
