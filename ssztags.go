// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
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

func (d *DynSsz) getSszTypeTag(field *reflect.StructField) ([]SszTypeHint, error) {
	// parse `ssz-type`
	sszTypeHints := []SszTypeHint{}

	if fieldSszTypeStr, fieldHasSszType := field.Tag.Lookup("ssz-type"); fieldHasSszType {
		for _, sszTypeStr := range strings.Split(fieldSszTypeStr, ",") {
			sszType := SszTypeHint{}

			switch sszTypeStr {
			case "?", "auto":
				sszType.Type = SszUnspecifiedType
			case "custom":
				sszType.Type = SszCustomType
			case "wrapper", "type-wrapper":
				sszType.Type = SszTypeWrapperType

			// basic types
			case "bool":
				sszType.Type = SszBoolType
			case "uint8":
				sszType.Type = SszUint8Type
			case "uint16":
				sszType.Type = SszUint16Type
			case "uint32":
				sszType.Type = SszUint32Type
			case "uint64":
				sszType.Type = SszUint64Type
			case "uint128":
				sszType.Type = SszUint128Type
			case "uint256":
				sszType.Type = SszUint256Type

			// complex types
			case "container":
				sszType.Type = SszContainerType
			case "list":
				sszType.Type = SszListType
			case "vector":
				sszType.Type = SszVectorType
			case "bitlist":
				sszType.Type = SszBitlistType
			case "bitvector":
				sszType.Type = SszBitvectorType
			case "progressive-list":
				sszType.Type = SszProgressiveListType
			case "progressive-bitlist":
				sszType.Type = SszProgressiveBitlistType
			case "progressive-container":
				sszType.Type = SszProgressiveContainerType
			case "compatible-union", "union":
				sszType.Type = SszCompatibleUnionType

			default:
				return nil, fmt.Errorf("invalid ssz-type tag for '%v' field: %v", field.Name, sszTypeStr)
			}

			sszTypeHints = append(sszTypeHints, sszType)
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
//   - expr: The dynamic expression used to calculate the size of the field, typically through 'dynssz-size' annotations.
type SszSizeHint struct {
	Size    uint32
	Dynamic bool
	Custom  bool
	Expr    string
}

// getSszSizeTag parses the 'ssz-size' and 'dynssz-size' tag annotations from a struct field and returns size hints
// based on these annotations. This function is integral for understanding the expected size constraints of fields,
// particularly when dealing with slices or arrays that may have fixed or dynamic lengths specified through these tags.
//
// Parameters:
// - field: A pointer to the reflect.StructField being examined. The field's tags are inspected to extract 'ssz-size'
//   and 'dynssz-size' annotations, which provide crucial size information for encoding or decoding processes.
//
// Returns:
// - A slice of SszSizeHint, which are derived from the parsed tag annotations. These hints inform the marshalling
//   and unmarshalling functions about the size characteristics of the field, enabling accurate handling of both
//   static and dynamic sized elements within struct fields.
// - An error if the tag parsing encounters issues, such as malformed annotations or unsupported specifications within
//   the tags. This ensures that size calculations and subsequent encoding or decoding actions can rely on valid and
//   correctly interpreted size information.
//
// getSszSizeTag plays a pivotal role in the dynamic SSZ encoding/decoding process by translating tag-based size
// specifications into actionable size hints. By accurately parsing and interpreting these tags, the function ensures
// that the library can correctly manage fields with complex size requirements, facilitating precise and efficient
// data serialization.

func (d *DynSsz) getSszSizeTag(field *reflect.StructField) ([]SszSizeHint, error) {
	sszSizes := []SszSizeHint{}

	// parse `ssz-size` first, these are the default values used by fastssz
	if fieldSszSizeStr, fieldHasSszSize := field.Tag.Lookup("ssz-size"); fieldHasSszSize {
		for _, sszSizeStr := range strings.Split(fieldSszSizeStr, ",") {
			sszSize := SszSizeHint{}

			if sszSizeStr == "?" {
				sszSize.Dynamic = true
			} else {
				sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 32)
				if err != nil {
					return sszSizes, fmt.Errorf("error parsing ssz-size tag for '%v' field: %v", field.Name, err)
				}
				sszSize.Size = uint32(sszSizeInt)
			}

			sszSizes = append(sszSizes, sszSize)
		}
	}

	fieldDynSszSizeStr, fieldHasDynSszSize := field.Tag.Lookup("dynssz-size")
	if fieldHasDynSszSize {
		for i, sszSizeStr := range strings.Split(fieldDynSszSizeStr, ",") {
			sszSize := SszSizeHint{}
			isExpr := false

			if sszSizeStr == "?" {
				sszSize.Dynamic = true
			} else if sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 32); err == nil {
				sszSize.Size = uint32(sszSizeInt)
			} else {
				ok, specVal, err := d.ResolveSpecValue(sszSizeStr)
				if err != nil {
					return sszSizes, fmt.Errorf("error parsing dynssz-size tag for '%v' field (%v): %v", field.Name, sszSizeStr, err)
				}

				isExpr = true
				if ok {
					// dynamic value from spec
					sszSize.Size = uint32(specVal)
					sszSize.Custom = true
				} else {
					// unknown spec value? fallback to fastssz defaults
					if i < len(sszSizes) {
						sszSizes[i].Expr = sszSizeStr
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

type SszMaxSizeHint struct {
	Size    uint64
	NoValue bool
	Custom  bool
	Expr    string
}

func (d *DynSsz) getSszMaxSizeTag(field *reflect.StructField) ([]SszMaxSizeHint, error) {
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
					return sszMaxSizes, fmt.Errorf("error parsing ssz-max tag for '%v' field: %v", field.Name, err)
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
				ok, specVal, err := d.ResolveSpecValue(sszMaxSizeStr)
				if err != nil {
					return sszMaxSizes, fmt.Errorf("error parsing dynssz-max tag for '%v' field (%v): %v", field.Name, sszMaxSizeStr, err)
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
					break
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

func (d *DynSsz) getSszIndexTag(field *reflect.StructField) (*uint16, error) {
	var sszIndex *uint16

	// parse `ssz-index` first, these are the default values used by fastssz
	if fieldSszIndexStr, fieldHasSszIndex := field.Tag.Lookup("ssz-index"); fieldHasSszIndex {
		sszSizeInt, err := strconv.ParseUint(fieldSszIndexStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing ssz-index tag for '%v' field: %v", field.Name, err)
		}

		index := uint16(sszSizeInt)
		sszIndex = &index
	}

	return sszIndex, nil
}
