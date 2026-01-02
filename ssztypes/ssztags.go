// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/sszutils"
)

type SszType uint8

const (
	SszUnspecifiedType SszType = iota
	SszCustomType
	SszTypeWrapperType

	// basic types
	SszBoolType
	SszUint8Type
	SszUint16Type
	SszUint32Type
	SszUint64Type
	SszUint128Type
	SszUint256Type

	// complex types
	SszContainerType
	SszListType
	SszVectorType
	SszBitlistType
	SszBitvectorType
	SszProgressiveListType
	SszProgressiveBitlistType
	SszProgressiveContainerType
	SszCompatibleUnionType
)

type SszTypeHint struct {
	Type SszType
}

func ParseSszType(typeStr string) (SszType, error) {
	switch typeStr {
	case "?", "auto":
		return SszUnspecifiedType, nil
	case "custom":
		return SszCustomType, nil
	case "wrapper", "type-wrapper":
		return SszTypeWrapperType, nil

	// basic types
	case "bool":
		return SszBoolType, nil
	case "uint8":
		return SszUint8Type, nil
	case "uint16":
		return SszUint16Type, nil
	case "uint32":
		return SszUint32Type, nil
	case "uint64":
		return SszUint64Type, nil
	case "uint128":
		return SszUint128Type, nil
	case "uint256":
		return SszUint256Type, nil

	// complex types
	case "container":
		return SszContainerType, nil
	case "list":
		return SszListType, nil
	case "vector":
		return SszVectorType, nil
	case "bitlist":
		return SszBitlistType, nil
	case "bitvector":
		return SszBitvectorType, nil
	case "progressive-list":
		return SszProgressiveListType, nil
	case "progressive-bitlist":
		return SszProgressiveBitlistType, nil
	case "progressive-container":
		return SszProgressiveContainerType, nil
	case "compatible-union", "union":
		return SszCompatibleUnionType, nil

	default:
		return SszUnspecifiedType, fmt.Errorf("invalid ssz-type tag '%v'", typeStr)
	}
}

func getSszTypeTag(field *reflect.StructField) ([]SszTypeHint, error) {
	// parse `ssz-type`
	sszTypeHints := []SszTypeHint{}

	if fieldSszTypeStr, fieldHasSszType := field.Tag.Lookup("ssz-type"); fieldHasSszType {
		for _, sszTypeStr := range strings.Split(fieldSszTypeStr, ",") {
			sszType, err := ParseSszType(sszTypeStr)
			if err != nil {
				return sszTypeHints, fmt.Errorf("error parsing ssz-type tag for '%v' field: %w", field.Name, err)
			}

			sszTypeHints = append(sszTypeHints, SszTypeHint{
				Type: sszType,
			})
		}
	}

	return sszTypeHints, nil
}

// SszSizeHint encapsulates size information for SSZ encoding and decoding, derived from 'ssz-size' and 'dynssz-size' tag annotations.
// It provides detailed insights into the size attributes of fields or types, particularly noting whether sizes are fixed or dynamic,
// and if special specification values are applied, differing from default assumptions.
//
// Fields:
//   - size: A uint64 value indicating the statically annotated size of the type or field, as specified by 'ssz-size' tag annotations.
//     For dynamic fields, where the size may vary depending on the instance of the data, this field is set to 0, and the dynamic flag
//     is used to indicate its dynamic nature.
//   - dynamic: A boolean flag indicating whether the field's size is dynamic, set to true for fields whose size can change or is not fixed
//     at compile time. This determination is based on the presence of 'dynssz-size' annotations or the inherent variability of the type.
//   - custom: A boolean indicating whether a non-default specification value has been applied to the type or field, typically through
//     'dynssz-size' annotations, suggesting a deviation from standard size expectations that might influence the encoding or decoding process.
//   - bits: A boolean flag indicating whether the size is in bits rather than bytes.
//   - expr: The dynamic expression used to calculate the size of the field, typically through 'dynssz-size' annotations.
type SszSizeHint struct {
	Size    uint32
	Dynamic bool
	Custom  bool
	Bits    bool
	Expr    string
}

// getSszSizeTag parses the 'ssz-size'/'ssz-bitsize' and 'dynssz-size'/'dynssz-bitsize' tag annotations from a struct field and returns
// size hints based on these annotations. This function is integral for understanding the expected size constraints of fields,
// particularly when dealing with slices or arrays that may have fixed or dynamic lengths specified through these tags.
//
// Parameters:
//   - ds: The dynamic specs to use for resolving spec values.
//   - field: A pointer to the reflect.StructField being examined. The field's tags are inspected to extract 'ssz-size'/'ssz-bitsize'
//     and 'dynssz-size'/'dynssz-bitsize' annotations, which provide crucial size information for encoding or decoding processes.
//
// Returns:
//   - A slice of SszSizeHint, which are derived from the parsed tag annotations. These hints inform the marshalling
//     and unmarshalling functions about the size characteristics of the field, enabling accurate handling of both
//     static and dynamic sized elements within struct fields.
//   - An error if the tag parsing encounters issues, such as malformed annotations or unsupported specifications within
//     the tags. This ensures that size calculations and subsequent encoding or decoding actions can rely on valid and
//     correctly interpreted size information.
func getSszSizeTag(ds sszutils.DynamicSpecs, field *reflect.StructField) ([]SszSizeHint, error) {
	sszSizes := []SszSizeHint{}

	// parse `ssz-size` first, these are the default values used by fastssz
	var sszSizeParts, sszBitsizeParts []string

	sszSizeLen := 0

	if fieldSszSizeStr, fieldHasSszSize := field.Tag.Lookup("ssz-size"); fieldHasSszSize {
		sszSizeParts = strings.Split(fieldSszSizeStr, ",")
		sszSizeLen = len(sszSizeParts)
	}

	if fieldSszBitsizeStr, fieldHasSszBitsize := field.Tag.Lookup("ssz-bitsize"); fieldHasSszBitsize {
		sszBitsizeParts = strings.Split(fieldSszBitsizeStr, ",")
		if len(sszBitsizeParts) > sszSizeLen {
			sszSizeLen = len(sszBitsizeParts)
		}
	}

	if sszSizeLen > 0 {
		for i := 0; i < sszSizeLen; i++ {
			sszSizeStr := getTagPart(sszSizeParts, i)
			sszBitsizeStr := getTagPart(sszBitsizeParts, i)

			sszSize := SszSizeHint{}

			if sszBitsizeStr != "?" {
				sszSizeInt, err := strconv.ParseUint(sszBitsizeStr, 10, 32)
				if err != nil {
					return sszSizes, fmt.Errorf("error parsing ssz-bitsize tag for '%v' field: %w", field.Name, err)
				}
				sszSize.Size = uint32(sszSizeInt)
				sszSize.Bits = true
			} else if sszSizeStr != "?" {
				sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 32)
				if err != nil {
					return sszSizes, fmt.Errorf("error parsing ssz-size tag for '%v' field: %w", field.Name, err)
				}
				sszSize.Size = uint32(sszSizeInt)
			} else {
				sszSize.Dynamic = true
			}

			sszSizes = append(sszSizes, sszSize)
		}
	}

	// parse `dynssz-size`/`dynssz-bitsize` next, these are the dynamic values used by dynamic-ssz
	sszSizeParts, sszBitsizeParts = nil, nil
	sszSizeLen = 0

	if fieldSszSizeStr, fieldHasSszSize := field.Tag.Lookup("dynssz-size"); fieldHasSszSize {
		sszSizeParts = strings.Split(fieldSszSizeStr, ",")
		sszSizeLen = len(sszSizeParts)
	}

	if fieldSszBitsizeStr, fieldHasSszBitsize := field.Tag.Lookup("dynssz-bitsize"); fieldHasSszBitsize {
		sszBitsizeParts = strings.Split(fieldSszBitsizeStr, ",")
		if len(sszBitsizeParts) > sszSizeLen {
			sszSizeLen = len(sszBitsizeParts)
		}
	}

	if sszSizeLen > 0 {
		for i := 0; i < sszSizeLen; i++ {
			sszSizeStr := getTagPart(sszSizeParts, i)
			sszBitsizeStr := getTagPart(sszBitsizeParts, i)

			sszSize := SszSizeHint{}
			isExpr := false
			sizeExpr := "?"

			if sszBitsizeStr != "?" {
				sizeExpr = sszBitsizeStr
				sszSize.Bits = true
			} else if sszSizeStr != "?" {
				sizeExpr = sszSizeStr
			}

			if sizeExpr == "?" {
				sszSize.Dynamic = true
			} else if sszSizeInt, err := strconv.ParseUint(sizeExpr, 10, 32); err == nil {
				sszSize.Size = uint32(sszSizeInt)
			} else {
				ok, specVal, err := ds.ResolveSpecValue(sizeExpr)
				if err != nil {
					return sszSizes, fmt.Errorf("error parsing dynssz-size tag for '%v' field (%v): %w", field.Name, sizeExpr, err)
				}

				isExpr = true
				if ok {
					// dynamic value from spec
					sszSize.Size = uint32(specVal)
					sszSize.Custom = true
				} else {
					// unknown spec value? fallback to fastssz defaults
					if i < len(sszSizes) {
						sszSizes[i].Expr = sizeExpr
					}
					break
				}
			}

			if i >= len(sszSizes) {
				sszSizes = append(sszSizes, sszSize)
			} else if sszSizes[i].Size != sszSize.Size {
				// update if resolved size differs from default
				sszSizes[i] = sszSize
			}

			if isExpr {
				sszSizes[i].Expr = sszSizeStr
			}
		}
	}

	return sszSizes, nil
}

// SszMaxSizeHint encapsulates max size information for SSZ encoding and decoding, derived from 'ssz-max'/'ssz-bitmax' and 'dynssz-max'/'dynssz-bitmax' tag annotations.
// It provides detailed insights into the max size attributes of fields or types, particularly noting whether max sizes are fixed or dynamic,
// and if special specification values are applied, differing from default assumptions.
//
// Fields:
//   - size: A uint64 value indicating the statically annotated max size of the type or field, as specified by 'ssz-max'/'ssz-bitmax' tag annotations.
//     For dynamic fields, where the max size may vary depending on the instance of the data, this field is set to 0, and the dynamic flag
//     is used to indicate its dynamic nature.
//   - dynamic: A boolean flag indicating whether the field's max size is dynamic, set to true for fields whose max size can change or is not fixed
//     at compile time. This determination is based on the presence of 'dynssz-max'/'dynssz-bitmax' annotations or the inherent variability of the type.
//   - custom: A boolean indicating whether a non-default specification value has been applied to the type or field, typically through
//     'dynssz-max'/'dynssz-bitmax' annotations, suggesting a deviation from standard max size expectations that might influence the encoding or decoding process.
//   - expr: The dynamic expression used to calculate the max size of the field, typically through 'dynssz-max'/'dynssz-bitmax' annotations.
type SszMaxSizeHint struct {
	Size    uint64
	NoValue bool
	Custom  bool
	Expr    string
}

// getSszMaxSizeTag parses the 'ssz-max'/'ssz-bitmax' and 'dynssz-max'/'dynssz-bitmax' tag annotations from a struct field and returns
// max size hints based on these annotations. This function is integral for understanding the expected max size constraints of fields,
// particularly when dealing with slices or arrays that may have fixed or dynamic lengths specified through these tags.
//
// Parameters:
//   - ds: The dynamic specs to use for resolving spec values.
//   - field: A pointer to the reflect.StructField being examined. The field's tags are inspected to extract 'ssz-max'/'ssz-bitmax'
//     and 'dynssz-max'/'dynssz-bitmax' annotations, which provide crucial max size information for encoding or decoding processes.
//
// Returns:
//   - A slice of SszMaxSizeHint, which are derived from the parsed tag annotations. These hints inform the marshalling
//     and unmarshalling functions about the max size characteristics of the field, enabling accurate handling of both
//     static and dynamic sized elements within struct fields.
//   - An error if the tag parsing encounters issues, such as malformed annotations or unsupported specifications within
//     the tags. This ensures that max size calculations and subsequent encoding or decoding actions can rely on valid and
//     correctly interpreted max size information.
func getSszMaxSizeTag(ds sszutils.DynamicSpecs, field *reflect.StructField) ([]SszMaxSizeHint, error) {
	sszMaxSizes := []SszMaxSizeHint{}

	// parse `ssz-max` first, these are the default values used by fastssz
	if fieldSszMaxStr, fieldHasSszMax := field.Tag.Lookup("ssz-max"); fieldHasSszMax {
		for _, sszSizeStr := range strings.Split(fieldSszMaxStr, ",") {
			sszMaxSize := SszMaxSizeHint{}

			if sszSizeStr == "?" {
				sszMaxSize.NoValue = true
			} else {
				sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 64)
				if err != nil {
					return sszMaxSizes, fmt.Errorf("error parsing ssz-max tag for '%v' field: %w", field.Name, err)
				}
				sszMaxSize.Size = sszSizeInt
			}

			sszMaxSizes = append(sszMaxSizes, sszMaxSize)
		}
	}

	fieldDynSszMaxStr, fieldHasDynSszMax := field.Tag.Lookup("dynssz-max")
	if fieldHasDynSszMax {
		for i, sszMaxSizeStr := range strings.Split(fieldDynSszMaxStr, ",") {
			sszMaxSize := SszMaxSizeHint{}
			isExpr := false

			if sszMaxSizeStr == "?" {
				sszMaxSize.NoValue = true
			} else if sszSizeInt, err := strconv.ParseUint(sszMaxSizeStr, 10, 64); err == nil {
				sszMaxSize.Size = sszSizeInt
			} else {
				ok, specVal, err := ds.ResolveSpecValue(sszMaxSizeStr)
				if err != nil {
					return sszMaxSizes, fmt.Errorf("error parsing dynssz-max tag for '%v' field (%v): %w", field.Name, sszMaxSizeStr, err)
				}

				isExpr = true
				if ok {
					// dynamic value from spec
					sszMaxSize.Size = specVal
					sszMaxSize.Custom = true
				} else {
					// unknown spec value? fallback to fastssz defaults
					if i < len(sszMaxSizes) {
						sszMaxSizes[i].Expr = sszMaxSizeStr
					}
					continue
				}
			}

			if i >= len(sszMaxSizes) {
				sszMaxSizes = append(sszMaxSizes, sszMaxSize)
			} else if sszMaxSizes[i].Size != sszMaxSize.Size {
				// update if resolved max size differs from default
				sszMaxSizes[i] = sszMaxSize
			}

			if isExpr {
				sszMaxSizes[i].Expr = sszMaxSizeStr
			}
		}
	}

	return sszMaxSizes, nil
}

func getSszIndexTag(field *reflect.StructField) (*uint16, error) {
	var sszIndex *uint16

	// parse `ssz-index` first, these are the default values used by fastssz
	if fieldSszIndexStr, fieldHasSszIndex := field.Tag.Lookup("ssz-index"); fieldHasSszIndex {
		sszSizeInt, err := strconv.ParseUint(fieldSszIndexStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing ssz-index tag for '%v' field: %w", field.Name, err)
		}

		index := uint16(sszSizeInt)
		sszIndex = &index
	}

	return sszIndex, nil
}

func getTagPart(parts []string, index int) string {
	if index < len(parts) {
		return parts[index]
	}
	return "?"
}
