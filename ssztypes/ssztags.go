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

var sszTypeMap = map[string]SszType{
	"?":            SszUnspecifiedType,
	"auto":         SszUnspecifiedType,
	"custom":       SszCustomType,
	"wrapper":      SszTypeWrapperType,
	"type-wrapper": SszTypeWrapperType,

	"bool":    SszBoolType,
	"uint8":   SszUint8Type,
	"uint16":  SszUint16Type,
	"uint32":  SszUint32Type,
	"uint64":  SszUint64Type,
	"uint128": SszUint128Type,
	"uint256": SszUint256Type,

	"container":             SszContainerType,
	"list":                  SszListType,
	"vector":                SszVectorType,
	"bitlist":               SszBitlistType,
	"bitvector":             SszBitvectorType,
	"progressive-list":      SszProgressiveListType,
	"progressive-bitlist":   SszProgressiveBitlistType,
	"progressive-container": SszProgressiveContainerType,
	"compatible-union":      SszCompatibleUnionType,
	"union":                 SszCompatibleUnionType,
}

func ParseSszType(typeStr string) (SszType, error) {
	if t, ok := sszTypeMap[typeStr]; ok {
		return t, nil
	}
	return SszUnspecifiedType, fmt.Errorf("invalid ssz-type tag '%v'", typeStr)
}

func getSszTypeTag(field *reflect.StructField) ([]SszTypeHint, error) {
	tag, ok := field.Tag.Lookup("ssz-type")
	if !ok {
		return nil, nil
	}

	// Fast path: single value
	if !strings.Contains(tag, ",") {
		t, err := ParseSszType(tag)
		if err != nil {
			return nil, fmt.Errorf("error parsing ssz-type tag for '%v' field: %w", field.Name, err)
		}
		return []SszTypeHint{{Type: t}}, nil
	}

	parts := strings.Split(tag, ",")
	hints := make([]SszTypeHint, 0, len(parts))

	for _, p := range parts {
		t, err := ParseSszType(p)
		if err != nil {
			return hints, fmt.Errorf("error parsing ssz-type tag for '%v' field: %w", field.Name, err)
		}
		hints = append(hints, SszTypeHint{Type: t})
	}

	return hints, nil
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
	fieldSszSizeStr, hasSszSize := field.Tag.Lookup("ssz-size")
	fieldSszBitsizeStr, hasSszBitsize := field.Tag.Lookup("ssz-bitsize")
	fieldDynSszSizeStr, hasDynSszSize := field.Tag.Lookup("dynssz-size")
	fieldDynSszBitsizeStr, hasDynSszBitsize := field.Tag.Lookup("dynssz-bitsize")

	if !hasSszSize && !hasSszBitsize && !hasDynSszSize && !hasDynSszBitsize {
		return nil, nil
	}

	var sszSizeParts, sszBitsizeParts, dynSszSizeParts, dynSszBitsizeParts []string
	maxLen := 0

	if hasSszSize {
		sszSizeParts = strings.Split(fieldSszSizeStr, ",")
		maxLen = len(sszSizeParts)
	}
	if hasSszBitsize {
		sszBitsizeParts = strings.Split(fieldSszBitsizeStr, ",")
		if len(sszBitsizeParts) > maxLen {
			maxLen = len(sszBitsizeParts)
		}
	}
	if hasDynSszSize {
		dynSszSizeParts = strings.Split(fieldDynSszSizeStr, ",")
		if len(dynSszSizeParts) > maxLen {
			maxLen = len(dynSszSizeParts)
		}
	}
	if hasDynSszBitsize {
		dynSszBitsizeParts = strings.Split(fieldDynSszBitsizeStr, ",")
		if len(dynSszBitsizeParts) > maxLen {
			maxLen = len(dynSszBitsizeParts)
		}
	}

	if maxLen == 0 {
		return nil, nil
	}

	sszSizes := make([]SszSizeHint, maxLen)

	for i := 0; i < maxLen; i++ {
		sszSize := &sszSizes[i]

		sszBitsizeStr := getTagPart(sszBitsizeParts, i)
		sszSizeStr := getTagPart(sszSizeParts, i)

		if sszBitsizeStr != "?" {
			sszSizeInt, err := strconv.ParseUint(sszBitsizeStr, 10, 32)
			if err != nil {
				return sszSizes[:i], fmt.Errorf("error parsing ssz-bitsize tag for '%v' field: %w", field.Name, err)
			}
			sszSize.Size = uint32(sszSizeInt)
			sszSize.Bits = true
		} else if sszSizeStr != "?" {
			sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 32)
			if err != nil {
				return sszSizes[:i], fmt.Errorf("error parsing ssz-size tag for '%v' field: %w", field.Name, err)
			}
			sszSize.Size = uint32(sszSizeInt)
		} else {
			sszSize.Dynamic = true
		}

		dynSszBitsizeStr := getTagPart(dynSszBitsizeParts, i)
		dynSszSizeStr := getTagPart(dynSszSizeParts, i)

		var sizeExpr string
		var isBitsize bool

		if dynSszBitsizeStr != "?" {
			sizeExpr = dynSszBitsizeStr
			isBitsize = true
		} else if dynSszSizeStr != "?" {
			sizeExpr = dynSszSizeStr
		} else {
			continue
		}

		if sizeExpr == "?" {
			sszSize.Dynamic = true
			sszSize.Bits = isBitsize
		} else if sszSizeInt, err := strconv.ParseUint(sizeExpr, 10, 32); err == nil {
			sszSize.Size = uint32(sszSizeInt)
			sszSize.Bits = isBitsize
			sszSize.Expr = sizeExpr
		} else {
			ok, specVal, err := ds.ResolveSpecValue(sizeExpr)
			if err != nil {
				return sszSizes[:i], fmt.Errorf("error parsing dynssz-size tag for '%v' field (%v): %w", field.Name, sizeExpr, err)
			}

			if ok {
				sszSize.Size = uint32(specVal)
				sszSize.Bits = isBitsize
				sszSize.Custom = true
				sszSize.Expr = sizeExpr
			} else {
				sszSize.Expr = sizeExpr
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
	fieldSszMaxStr, hasSszMax := field.Tag.Lookup("ssz-max")
	fieldDynSszMaxStr, hasDynSszMax := field.Tag.Lookup("dynssz-max")

	if !hasSszMax && !hasDynSszMax {
		return nil, nil
	}

	var sszMaxParts, dynSszMaxParts []string
	maxLen := 0

	if hasSszMax {
		sszMaxParts = strings.Split(fieldSszMaxStr, ",")
		maxLen = len(sszMaxParts)
	}
	if hasDynSszMax {
		dynSszMaxParts = strings.Split(fieldDynSszMaxStr, ",")
		if len(dynSszMaxParts) > maxLen {
			maxLen = len(dynSszMaxParts)
		}
	}

	if maxLen == 0 {
		return nil, nil
	}

	sszMaxSizes := make([]SszMaxSizeHint, maxLen)

	for i := 0; i < maxLen; i++ {
		sszMaxSize := &sszMaxSizes[i]

		sszMaxStr := getTagPart(sszMaxParts, i)

		if sszMaxStr == "?" {
			sszMaxSize.NoValue = true
		} else if sszMaxStr != "" {
			sszSizeInt, err := strconv.ParseUint(sszMaxStr, 10, 64)
			if err != nil {
				return sszMaxSizes[:i], fmt.Errorf("error parsing ssz-max tag for '%v' field: %w", field.Name, err)
			}
			sszMaxSize.Size = sszSizeInt
		}

		dynSszMaxStr := getTagPart(dynSszMaxParts, i)
		if dynSszMaxStr == "?" || dynSszMaxStr == "" {
			if dynSszMaxStr == "?" {
				sszMaxSize.NoValue = true
			}
			continue
		}

		if sszSizeInt, err := strconv.ParseUint(dynSszMaxStr, 10, 64); err == nil {
			sszMaxSize.Size = sszSizeInt
			sszMaxSize.Expr = dynSszMaxStr
			continue
		}

		ok, specVal, err := ds.ResolveSpecValue(dynSszMaxStr)
		if err != nil {
			return sszMaxSizes[:i], fmt.Errorf("error parsing dynssz-max tag for '%v' field (%v): %w", field.Name, dynSszMaxStr, err)
		}

		if ok {
			sszMaxSize.Size = specVal
			sszMaxSize.Custom = true
			sszMaxSize.Expr = dynSszMaxStr
		} else {
			sszMaxSize.Expr = dynSszMaxStr
		}
	}

	return sszMaxSizes, nil
}

func getSszIndexTag(field *reflect.StructField) (*uint16, error) {
	fieldSszIndexStr, ok := field.Tag.Lookup("ssz-index")
	if !ok {
		return nil, nil
	}

	sszSizeInt, err := strconv.ParseUint(fieldSszIndexStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("error parsing ssz-index tag for '%v' field: %w", field.Name, err)
	}

	index := uint16(sszSizeInt)
	return &index, nil
}

func getTagPart(parts []string, index int) string {
	if index < len(parts) {
		return parts[index]
	}
	return "?"
}
