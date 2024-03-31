// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	fastssz "github.com/ferranbt/fastssz"
)

type sszSizeHint struct {
	size    uint64
	dynamic bool
	specval bool
}

type sszSizeCache struct {
	size    int
	specval bool
}

func (d *DynSsz) getSszSizeTag(field *reflect.StructField) ([]sszSizeHint, error) {
	sszSizes := []sszSizeHint{}

	if fieldSszSizeStr, fieldHasSszSize := field.Tag.Lookup("ssz-size"); fieldHasSszSize {
		for _, sszSizeStr := range strings.Split(fieldSszSizeStr, ",") {
			sszSize := sszSizeHint{}

			if sszSizeStr == "?" {
				sszSize.dynamic = true
			} else {
				sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 32)
				if err != nil {
					return sszSizes, fmt.Errorf("error parsing ssz-size tag for '%v' field: %v", field.Name, err)
				}
				sszSize.size = sszSizeInt
			}

			sszSizes = append(sszSizes, sszSize)
		}
	}

	fieldDynSszSizeStr, fieldHasDynSszSize := field.Tag.Lookup("dynssz-size")
	if fieldHasDynSszSize {
		for i, sszSizeStr := range strings.Split(fieldDynSszSizeStr, ",") {
			sszSize := sszSizeHint{}

			if sszSizeStr == "?" {
				sszSize.dynamic = true
			} else if sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 32); err == nil {
				sszSize.size = sszSizeInt
			} else {
				ok, specVal, err := d.getSpecValue(sszSizeStr)
				if err != nil {
					return sszSizes, fmt.Errorf("error parsing dynssz-size tag for '%v' field (%v): %v", field.Name, sszSizeStr, err)
				}
				if ok {
					// dynamic value from spec
					sszSize.size = specVal
					sszSize.specval = true
				} else {
					// unknown spec value? fallback to fastssz
					break
				}
			}

			if sszSizes[i].size != sszSize.size {
				sszSizes[i] = sszSize
			}
		}
	}

	return sszSizes, nil
}

func (d *DynSsz) getSszSize(targetType reflect.Type, sizeHints []sszSizeHint) (int, bool, error) {
	staticSize := 0
	hasSpecValue := false
	isDynamicSize := false

	childSizeHints := []sszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}

	if cachedSize := d.typeSizeCache[targetType]; cachedSize != nil {
		return cachedSize.size, cachedSize.specval, nil
	}

	switch targetType.Kind() {
	case reflect.Struct:
		for i := 0; i < targetType.NumField(); i++ {
			field := targetType.Field(i)
			size, hasSpecVal, _, err := d.getSszFieldSize(&field)
			if err != nil {
				return 0, false, err
			}
			if size < 0 {
				isDynamicSize = true
			}
			if hasSpecVal {
				hasSpecValue = true
			}
			staticSize += size
		}
	case reflect.Array:
		arrLen := targetType.Len()
		fieldType := targetType.Elem()
		size, hasSpecVal, err := d.getSszSize(fieldType, childSizeHints)
		if err != nil {
			return 0, false, err
		}
		if size < 0 {
			isDynamicSize = true
		}
		if hasSpecVal {
			hasSpecValue = true
		}
		staticSize += size * arrLen
	case reflect.Slice:
		fieldType := targetType.Elem()
		size, hasSpecVal, err := d.getSszSize(fieldType, childSizeHints)
		if err != nil {
			return 0, false, err
		}
		if size < 0 {
			isDynamicSize = true
		}
		if hasSpecVal || (len(sizeHints) > 0 && sizeHints[0].specval) {
			hasSpecValue = true
		}

		if len(sizeHints) > 0 && sizeHints[0].size > 0 {
			staticSize += size * int(sizeHints[0].size)
		} else {
			isDynamicSize = true
		}
	case reflect.Bool:
		staticSize = 1
	case reflect.Uint8:
		staticSize = 1
	case reflect.Uint16:
		staticSize = 2
	case reflect.Uint32:
		staticSize = 4
	case reflect.Uint64:
		staticSize = 8
	default:
		return 0, false, fmt.Errorf("unhandled reflection kind in size check: %v", targetType.Kind())
	}

	if isDynamicSize {
		staticSize = -1
	} else if len(sizeHints) == 0 {
		d.typeSizeCache[targetType] = &sszSizeCache{
			size:    staticSize,
			specval: hasSpecValue,
		}
	}

	return staticSize, hasSpecValue, nil
}

func (d *DynSsz) getSszFieldSize(targetField *reflect.StructField) (int, bool, []sszSizeHint, error) {
	sszSizes, err := d.getSszSizeTag(targetField)
	if err != nil {
		return 0, false, nil, err
	}

	size, hasSpecVal, err := d.getSszSize(targetField.Type, sszSizes)
	return size, hasSpecVal, sszSizes, err
}

func (d *DynSsz) getSszValueSize(targetType reflect.Type, targetValue reflect.Value) (int, error) {
	staticSize := 0

	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
		targetValue = targetValue.Elem()
	}

	switch targetType.Kind() {
	case reflect.Struct:
		usedFastSsz := false

		hasSpecVals := d.typesWithSpecVals[targetType]
		if hasSpecVals == unknownSpecValued && !d.NoFastSsz {
			hasSpecVals = noSpecValues
			if targetValue.Addr().Type().Implements(sszMarshallerType) {
				_, hasSpecVals2, err := d.getSszSize(targetType, []sszSizeHint{})
				if err != nil {
					return 0, err
				}

				if hasSpecVals2 {
					hasSpecVals = hasSpecValues
				}
			}

			// fmt.Printf(" fastssz for type %s: %v\n", targetType.Name(), hasSpecVals)
			d.typesWithSpecVals[targetType] = hasSpecVals
		}
		if hasSpecVals == noSpecValues && !d.NoFastSsz {
			marshaller, ok := targetValue.Addr().Interface().(fastssz.Marshaler)
			if ok {
				staticSize = marshaller.SizeSSZ()
				usedFastSsz = true
			}
		}

		if !usedFastSsz {
			for i := 0; i < targetType.NumField(); i++ {
				field := targetType.Field(i)
				fieldValue := targetValue.Field(i)

				fieldTypeSize, _, _, err := d.getSszFieldSize(&field)
				if err != nil {
					return 0, err
				}

				if fieldTypeSize < 0 {
					size, err := d.getSszValueSize(field.Type, fieldValue)
					if err != nil {
						return 0, err
					}

					// dynamic field, add 4 bytes for offset
					staticSize += size + 4
				} else {
					staticSize += fieldTypeSize
				}
			}
		}
	case reflect.Array:
		arrLen := targetType.Len()
		if arrLen > 0 {
			fieldType := targetType.Elem()
			if fieldType == byteType {
				staticSize = arrLen
			} else {
				size, err := d.getSszValueSize(fieldType, targetValue.Index(0))
				if err != nil {
					return 0, err
				}
				staticSize = size * arrLen
			}
		}
	case reflect.Slice:
		fieldType := targetType.Elem()
		sliceLen := targetValue.Len()

		if sliceLen > 0 {
			if fieldType == byteType {
				staticSize = sliceLen
			} else {
				fieldTypeSize, _, err := d.getSszSize(fieldType, nil)
				if err != nil {
					return 0, err
				}

				if fieldTypeSize < 0 {
					// dyn size slice
					for i := 0; i < sliceLen; i++ {
						size, err := d.getSszValueSize(fieldType, targetValue.Index(i))
						if err != nil {
							return 0, err
						}
						staticSize += size + 4
					}
				} else {
					staticSize = fieldTypeSize * sliceLen
				}
			}
		}
	case reflect.Bool:
		staticSize = 1
	case reflect.Uint8:
		staticSize = 1
	case reflect.Uint16:
		staticSize = 2
	case reflect.Uint32:
		staticSize = 4
	case reflect.Uint64:
		staticSize = 8
	default:
		return 0, fmt.Errorf("unhandled reflection kind in size check: %v", targetType.Kind())
	}

	return staticSize, nil
}
