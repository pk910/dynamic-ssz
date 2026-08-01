// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// SszType identifies the SSZ encoding type for a field or value.
// It covers basic types (bool, uintN), complex types (container, list, vector,
// bitlist, bitvector), progressive types, unions, and extended non-standard
// types (signed integers, floats, bigint).
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
	SszUnionType
	SszProgressiveListType
	SszProgressiveBitlistType
	SszProgressiveContainerType
	SszCompatibleUnionType

	// extended types (not supported by SSZ spec)
	SszInt8Type
	SszInt16Type
	SszInt32Type
	SszInt64Type
	SszBigIntType
	SszFloat32Type
	SszFloat64Type
	SszOptionalType

	// canonical SSZ encodings expressed via Go conveniences
	SszOptionalListType // pointer encoded as canonical List[T, 1]
)

// Tag names for the two unbounded-by-tag types, shared with the messages that
// name them back to the user.
const (
	sszTypeNameList    = "list"
	sszTypeNameBitlist = "bitlist"
)

// SszTypeHint holds a parsed SSZ type hint from an ssz-type struct tag.
// Multiple hints may be present for nested types (e.g., a list of vectors).
type SszTypeHint struct {
	Type SszType
}

// ParseSszType converts an ssz-type tag string value (e.g., "container",
// "list", "uint64") into the corresponding SszType constant. Returns an
// error for unrecognized type strings.
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
	case sszTypeNameList:
		return SszListType, nil
	case "vector":
		return SszVectorType, nil
	case sszTypeNameBitlist:
		return SszBitlistType, nil
	case "bitvector":
		return SszBitvectorType, nil
	case "progressive-list":
		return SszProgressiveListType, nil
	case "progressive-bitlist":
		return SszProgressiveBitlistType, nil
	case "progressive-container":
		return SszProgressiveContainerType, nil
	case "compatible-union":
		return SszCompatibleUnionType, nil
	case "union":
		return SszUnionType, nil

	// extended types (not supported by SSZ spec)
	case "int8":
		return SszInt8Type, nil
	case "int16":
		return SszInt16Type, nil
	case "int32":
		return SszInt32Type, nil
	case "int64":
		return SszInt64Type, nil
	case "bigint":
		return SszBigIntType, nil
	case "float32":
		return SszFloat32Type, nil
	case "float64":
		return SszFloat64Type, nil
	case "optional":
		return SszOptionalType, nil

	// canonical SSZ encodings expressed via Go conveniences
	case "optional-list":
		return SszOptionalListType, nil

	default:
		return SszUnspecifiedType, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "invalid ssz-type tag '%v'", typeStr)
	}
}

// IsSszExcluded reports whether a struct field is excluded from SSZ processing
// via `ssz-type:"-"`. Such a field is omitted from the SSZ layout entirely (not
// encoded, decoded, sized or hashed) while remaining an ordinary Go field, so
// its type need not be SSZ-compatible.
func IsSszExcluded(tag reflect.StructTag) bool {
	if v, ok := tag.Lookup("ssz-type"); ok {
		return strings.TrimSpace(v) == "-"
	}
	// Honor the fastssz `ssz:"-"` exclude tag when no ssz-type is given.
	if v, ok := tag.Lookup("ssz"); ok {
		return strings.TrimSpace(v) == "-"
	}
	return false
}

func getSszTypeTag(field *reflect.StructField) ([]SszTypeHint, error) {
	// parse `ssz-type`
	sszTypeHints := []SszTypeHint{}

	fieldSszTypeStr, fieldHasSszType := field.Tag.Lookup("ssz-type")
	fieldSszStr, fieldHasSsz := field.Tag.Lookup("ssz")

	// Honor the plain fastssz `ssz` tag as an ssz-type when no ssz-type is given,
	// so the reflection engine agrees with fastssz delegation and generated code
	// instead of silently ignoring it (e.g. `ssz:"bitlist"`). Setting both is
	// ambiguous — reject it.
	switch {
	case fieldHasSszType && fieldHasSsz:
		return sszTypeHints, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "field %q sets both 'ssz' and 'ssz-type' tags; use only one", field.Name)
	case !fieldHasSszType && fieldHasSsz:
		fieldSszTypeStr, fieldHasSszType = fieldSszStr, true
	}

	if fieldHasSszType {
		for _, sszTypeStr := range strings.Split(fieldSszTypeStr, ",") {
			sszType, err := ParseSszType(strings.TrimSpace(sszTypeStr))
			if err != nil {
				return sszTypeHints, sszutils.ErrorWithPath(err, field.Name)
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

			switch {
			case sszBitsizeStr != "?":
				sszSizeInt, err := strconv.ParseUint(sszBitsizeStr, 10, 32)
				if err != nil {
					return sszSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing ssz-bitsize tag for '%v' field: %v", field.Name, err)
				}
				sszSize.Size = uint32(sszSizeInt)
				sszSize.Bits = true
			case sszSizeStr != "?":
				sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 32)
				if err != nil {
					return sszSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing ssz-size tag for '%v' field: %v", field.Name, err)
				}
				sszSize.Size = uint32(sszSizeInt)
			default:
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

			// `?` is a placeholder: it declares the dimension dynamic rather
			// than giving it a length. A dimension is either sized or dynamic,
			// so the static and dynamic tags have to agree on which -- one
			// saying `?` while the other names a length describes two different
			// types (list vs vector), and the engines would encode it two ways.
			if i < len(sszSizes) && sszSizes[i].Dynamic != (sizeExpr == "?") {
				return sszSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "conflicting size tags for field %q dimension %d: %s", field.Name, i, placeholderMismatch("ssz-size", "dynssz-size", sszSizes[i].Dynamic))
			}

			if sizeExpr == "?" {
				sszSize.Dynamic = true
			} else if sszSizeInt, err := strconv.ParseUint(sizeExpr, 10, 32); err == nil {
				sszSize.Size = uint32(sszSizeInt)
			} else {
				ok, specVal, err := ds.ResolveSpecValue(sizeExpr)
				if err != nil {
					return sszSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing dynssz-size tag for '%v' field (%v): %v", field.Name, sizeExpr, err)
				}

				isExpr = true
				if ok {
					// dynamic value from spec
					if specVal > math.MaxUint32 {
						return sszSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "dynssz-size value %d for field %q exceeds the uint32 size range", specVal, field.Name)
					}
					if specVal == 0 {
						// A dynssz-size that resolves to 0 would form a zero-length
						// vector, which the SSZ spec forbids. Fall back to a positive
						// static ssz-size if one was given; otherwise there is no
						// valid length for this dimension.
						if i < len(sszSizes) && sszSizes[i].Size > 0 {
							continue
						}
						return sszSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz-size for field %q resolved to 0 with no positive static fallback", field.Name)
					}
					sszSize.Size = uint32(specVal)
					sszSize.Custom = true
				} else {
					// Unknown spec value: keep the fastssz default for this dimension
					// (or record it as dynamic when there is none), but keep resolving
					// the remaining dimensions independently (matching the dynssz-max
					// loop and codegen). The static fallback and the expression share
					// one hint (and one unit), so a unit mismatch between the two tag
					// families is unrepresentable and must be rejected.
					if i < len(sszSizes) {
						if sszSizes[i].Bits != sszSize.Bits {
							return sszSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "conflicting size units for field %q dimension %d: the static and dynamic size tags use different units (bits vs bytes)", field.Name, i)
						}
						sszSizes[i].Expr = sizeExpr
					} else {
						sszSize.Dynamic = true
						sszSize.Expr = sizeExpr
						sszSizes = append(sszSizes, sszSize)
					}
					continue
				}
			}

			if i >= len(sszSizes) {
				sszSizes = append(sszSizes, sszSize)
			} else {
				// The dynamic tag overrides the static hint entirely, including
				// its unit. Replacing only on a differing number made the
				// dimension's unit depend on whether the resolved value happened
				// to equal the static fallback.
				sszSizes[i] = sszSize
			}

			if isExpr {
				sszSizes[i].Expr = sizeExpr
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
			sszSizeStr = strings.TrimSpace(sszSizeStr)
			sszMaxSize := SszMaxSizeHint{}

			if sszSizeStr == "?" {
				sszMaxSize.NoValue = true
			} else {
				sszSizeInt, err := strconv.ParseUint(sszSizeStr, 10, 64)
				if err != nil {
					return sszMaxSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing ssz-max tag for '%v' field: %v", field.Name, err)
				}
				sszMaxSize.Size = sszSizeInt
			}

			sszMaxSizes = append(sszMaxSizes, sszMaxSize)
		}
	}

	fieldDynSszMaxStr, fieldHasDynSszMax := field.Tag.Lookup("dynssz-max")
	if fieldHasDynSszMax {
		for i, sszMaxSizeStr := range strings.Split(fieldDynSszMaxStr, ",") {
			sszMaxSizeStr = strings.TrimSpace(sszMaxSizeStr)
			sszMaxSize := SszMaxSizeHint{}
			isExpr := false

			if sszMaxSizeStr == "?" {
				sszMaxSize.NoValue = true
			} else if sszSizeInt, err := strconv.ParseUint(sszMaxSizeStr, 10, 64); err == nil {
				sszMaxSize.Size = sszSizeInt
			} else {
				ok, specVal, err := ds.ResolveSpecValue(sszMaxSizeStr)
				if err != nil {
					return sszMaxSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing dynssz-max tag for '%v' field (%v): %v", field.Name, sszMaxSizeStr, err)
				}

				isExpr = true
				if ok {
					// dynamic value from spec
					if specVal == 0 {
						// A dynssz-max that resolves to 0 falls back to a positive
						// static ssz-max if one was given; otherwise the list
						// capacity would be 0, which we reject as invalid.
						if i < len(sszMaxSizes) && sszMaxSizes[i].Size > 0 {
							continue
						}
						return sszMaxSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz-max for field %q resolved to 0 with no positive static fallback", field.Name)
					}
					sszMaxSize.Size = specVal
					sszMaxSize.Custom = true
				} else {
					// Unknown spec value: keep the fastssz default for this
					// dimension. A zero default is the ssz-max:"0" placeholder,
					// which is not a fallback at all -- the type said its limit
					// comes from the spec and the spec does not define it, so
					// there is no limit to encode or hash against. That is the
					// same dead end as resolving to zero, and the generated code
					// reports it too (ResolveSpecValueWithDefault).
					if i < len(sszMaxSizes) {
						if sszMaxSizes[i].Size == 0 && !sszMaxSizes[i].NoValue {
							return sszMaxSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz-max %q for field %q is not defined and has no positive static fallback", sszMaxSizeStr, field.Name)
						}
						sszMaxSizes[i].Expr = sszMaxSizeStr
					}
					continue
				}
			}

			// `?` is a placeholder: it declares the dimension unbounded rather
			// than giving it a limit. The static and dynamic tags have to agree
			// on which -- one saying `?` while the other names a limit would
			// otherwise silently drop the limit and leave a list with no hash
			// tree root.
			if i < len(sszMaxSizes) && sszMaxSizes[i].NoValue != sszMaxSize.NoValue {
				return sszMaxSizes, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "conflicting max tags for field %q dimension %d: %s", field.Name, i, placeholderMismatch("ssz-max", "dynssz-max", sszMaxSizes[i].NoValue))
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

// checkDimensionsDeclared rejects a dimension left as `?` by both tag families.
// `?` means "dynamic" to ssz-size and "unbounded" to ssz-max, so a dimension
// carrying both placeholders declares neither a length nor a limit -- it names
// no SSZ type, and a list without a limit has no hash tree root.
//
// Saying nothing is different from saying `?`: a field with no tag at all is
// described by its Go type, and an unbounded list remains expressible that way
// under extended types.
func checkDimensionsDeclared(sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, fieldName string) error {
	for i := range sizeHints {
		if i >= len(maxSizeHints) {
			break
		}
		if sizeHints[i].Dynamic && maxSizeHints[i].NoValue {
			if fieldName == "" {
				return fmt.Errorf("dimension %d is `?` in both ssz-size and ssz-max, so it has neither a length nor a limit", i)
			}

			return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "field %q dimension %d is `?` in both ssz-size and ssz-max, so it has neither a length nor a limit", fieldName, i)
		}
	}

	return nil
}

// placeholderMismatch describes a dimension whose static and dynamic tags
// disagree about being a placeholder. staticIsPlaceholder says which way round
// the disagreement runs, so the message names the tag that has to change.
func placeholderMismatch(staticTag, dynTag string, staticIsPlaceholder bool) string {
	if staticIsPlaceholder {
		return fmt.Sprintf("%s marks it `?` but %s gives it a value", staticTag, dynTag)
	}

	return fmt.Sprintf("%s gives it a value but %s marks it `?`", staticTag, dynTag)
}

func getSszIndexTag(field *reflect.StructField) (*uint16, error) {
	var sszIndex *uint16

	// ssz-index declares a field's position in a progressive container
	// (EIP-7495). Unlike ssz-size and ssz-max it is not a fastssz tag, so it
	// never arrives from foreign tooling: carrying one is what opts a container
	// into progressive merkleization.
	if fieldSszIndexStr, fieldHasSszIndex := field.Tag.Lookup("ssz-index"); fieldHasSszIndex {
		sszSizeInt, err := strconv.ParseUint(strings.TrimSpace(fieldSszIndexStr), 10, 16)
		if err != nil {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing ssz-index tag for '%v' field: %v", field.Name, err)
		}
		// EIP-7495 progressive containers support at most 256 active fields
		// (a larger bitvector also has no stable single-chunk mixin), and
		// union selectors are a single byte.
		if sszSizeInt > 255 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "ssz-index %d for field %q exceeds the supported maximum of 255", sszSizeInt, field.Name)
		}

		index := uint16(sszSizeInt)
		sszIndex = &index
	}

	return sszIndex, nil
}

// getTagPart returns the index-th comma-separated dimension of a size tag, or
// "?" when the tag declares fewer dimensions. Surrounding whitespace is trimmed
// so `ssz-size:"8, 16"` parses like `ssz-size:"8,16"`, matching ParseTags and
// the spec-expression evaluator.
func getTagPart(parts []string, index int) string {
	if index < len(parts) {
		return strings.TrimSpace(parts[index])
	}
	return "?"
}

// rejectZeroSizeHint rejects an explicit literal ssz-size:"0" on a slice or
// string: a zero-length vector is not valid SSZ, and silently degrading to an
// unbounded list would drop the intended constraint entirely. Dynamic ("?")
// and expression-based dimensions are unaffected.
func rejectZeroSizeHint(sizeHints []SszSizeHint) error {
	if len(sizeHints) > 0 && !sizeHints[0].Dynamic && sizeHints[0].Expr == "" && sizeHints[0].Size == 0 {
		return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "ssz-size 0 is not a valid vector size (zero-length vectors are illegal in SSZ)")
	}
	return nil
}

// JoinFieldAnnotationTag returns the effective SSZ tag for a struct field whose
// type carries a registered annotation: the field tag is joined in front of the
// annotation tag, so a key present in both resolves to the field's value
// (reflect.StructTag.Lookup returns the first occurrence) while annotation-only
// keys still apply.
func JoinFieldAnnotationTag(fieldTag reflect.StructTag, annotationTag string) reflect.StructTag {
	if annotationTag == "" {
		return fieldTag
	}
	if fieldTag == "" {
		return reflect.StructTag(annotationTag)
	}
	return reflect.StructTag(string(fieldTag) + " " + annotationTag)
}

// ParseTags parses SSZ annotations from a string in struct tag format.
// This is used for extracting SSZ annotations from sources other than
// struct fields, such as type-level annotations registered via
// sszutils.Annotate[T]().
func ParseTags(tag string) (typeHints []SszTypeHint, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, err error) {
	if tag == "" {
		return nil, nil, nil, nil
	}

	structTag := reflect.StructTag(tag)

	// Parse type hints
	if sszType, ok := structTag.Lookup("ssz-type"); ok {
		for _, typeStr := range strings.Split(sszType, ",") {
			typeStr = strings.TrimSpace(typeStr)
			hint := SszTypeHint{}

			hint.Type, err = ParseSszType(typeStr)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error parsing ssz-type tag: %v", err)
			}

			typeHints = append(typeHints, hint)
		}
	}

	// Parse size hints
	var sszSizeParts, sszBitsizeParts []string

	sszSizeLen := 0

	if fieldSszSizeStr, fieldHasSszSize := structTag.Lookup("ssz-size"); fieldHasSszSize {
		sszSizeParts = strings.Split(fieldSszSizeStr, ",")
		sszSizeLen = len(sszSizeParts)
	}

	if fieldSszBitsizeStr, fieldHasSszBitsize := structTag.Lookup("ssz-bitsize"); fieldHasSszBitsize {
		sszBitsizeParts = strings.Split(fieldSszBitsizeStr, ",")
		if len(sszBitsizeParts) > sszSizeLen {
			sszSizeLen = len(sszBitsizeParts)
		}
	}

	if sszSizeLen > 0 {
		for i := 0; i < sszSizeLen; i++ {
			sszSizeStr := getTagPart(sszSizeParts, i)
			sszBitsizeStr := getTagPart(sszBitsizeParts, i)

			hint := SszSizeHint{}

			switch {
			case sszBitsizeStr != "?":
				sizeInt, parseErr := strconv.ParseUint(strings.TrimSpace(sszBitsizeStr), 10, 32)
				if parseErr != nil {
					return nil, nil, nil, fmt.Errorf("error parsing ssz-size tag: %v", parseErr)
				}

				hint.Size = uint32(sizeInt)
				hint.Bits = true
			case sszSizeStr != "?":
				sizeInt, parseErr := strconv.ParseUint(strings.TrimSpace(sszSizeStr), 10, 32)
				if parseErr != nil {
					return nil, nil, nil, fmt.Errorf("error parsing ssz-size tag: %v", parseErr)
				}

				hint.Size = uint32(sizeInt)
			default:
				hint.Dynamic = true
			}

			sizeHints = append(sizeHints, hint)
		}
	}

	// Parse dynamic size hints
	sszSizeParts, sszBitsizeParts = nil, nil
	sszSizeLen = 0

	if fieldDynSszSizeStr, fieldHasDynSszSize := structTag.Lookup("dynssz-size"); fieldHasDynSszSize {
		sszSizeParts = strings.Split(fieldDynSszSizeStr, ",")
		sszSizeLen = len(sszSizeParts)
	}

	if fieldDynSszBitsizeStr, fieldHasDynSszBitsize := structTag.Lookup("dynssz-bitsize"); fieldHasDynSszBitsize {
		sszBitsizeParts = strings.Split(fieldDynSszBitsizeStr, ",")
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

			// See getSszSizeTag: the placeholder has to line up in both tags.
			if i < len(sizeHints) && sizeHints[i].Dynamic != (sizeExpr == "?") {
				return nil, nil, nil, fmt.Errorf("conflicting size tags for dimension %d: %s", i, placeholderMismatch("ssz-size", "dynssz-size", sizeHints[i].Dynamic))
			}

			if sizeExpr == "?" {
				sszSize.Dynamic = true
			} else if sszSizeInt, parseErr := strconv.ParseUint(sizeExpr, 10, 32); parseErr == nil {
				sszSize.Size = uint32(sszSizeInt)
			} else {
				isExpr = true
				sszSize.Dynamic = true
				sszSize.Custom = true

				if i < len(sizeHints) {
					// The static fallback and the expression share one hint (and
					// one unit); a unit mismatch between the two tag families is
					// unrepresentable and must be rejected.
					if sizeHints[i].Bits != sszSize.Bits {
						return nil, nil, nil, fmt.Errorf("conflicting size units for dimension %d: the static and dynamic size tags use different units (bits vs bytes)", i)
					}
					sizeHints[i].Expr = sizeExpr

					continue
				}
			}

			if i >= len(sizeHints) {
				sizeHints = append(sizeHints, sszSize)
			} else {
				// The dynamic tag overrides the static hint entirely, including
				// its unit (see the reflection merge above).
				sizeHints[i] = sszSize
			}

			if isExpr {
				sizeHints[i].Expr = sizeExpr
			}
		}
	}

	// Parse max size hints
	if sszMax, ok := structTag.Lookup("ssz-max"); ok {
		for _, maxStr := range strings.Split(sszMax, ",") {
			maxStr = strings.TrimSpace(maxStr)
			hint := SszMaxSizeHint{}

			if maxStr == "?" {
				hint.NoValue = true
			} else {
				maxInt, parseErr := strconv.ParseUint(maxStr, 10, 64)
				if parseErr != nil {
					return nil, nil, nil, fmt.Errorf("error parsing ssz-max tag: %v", parseErr)
				}

				hint.Size = maxInt
			}

			maxSizeHints = append(maxSizeHints, hint)
		}
	}

	// Parse dynamic max size hints
	fieldDynSszMaxStr, fieldHasDynSszMax := structTag.Lookup("dynssz-max")
	if fieldHasDynSszMax {
		for i, sszMaxSizeStr := range strings.Split(fieldDynSszMaxStr, ",") {
			sszMaxSizeStr = strings.TrimSpace(sszMaxSizeStr)
			sszMaxSize := SszMaxSizeHint{}
			isExpr := false

			// See getSszMaxSizeTag: the placeholder has to line up in both tags.
			if i < len(maxSizeHints) && maxSizeHints[i].NoValue != (sszMaxSizeStr == "?") {
				return nil, nil, nil, fmt.Errorf("conflicting max tags for dimension %d: %s", i, placeholderMismatch("ssz-max", "dynssz-max", maxSizeHints[i].NoValue))
			}

			if sszMaxSizeStr == "?" {
				sszMaxSize.NoValue = true
			} else if sszSizeInt, parseErr := strconv.ParseUint(sszMaxSizeStr, 10, 64); parseErr == nil {
				sszMaxSize.Size = sszSizeInt
			} else {
				isExpr = true
				sszMaxSize.Custom = true

				if i < len(maxSizeHints) {
					maxSizeHints[i].Expr = sszMaxSizeStr

					continue
				}
			}

			if i >= len(maxSizeHints) {
				maxSizeHints = append(maxSizeHints, sszMaxSize)
			} else if maxSizeHints[i].Size != sszMaxSize.Size {
				maxSizeHints[i] = sszMaxSize
			}

			if isExpr {
				maxSizeHints[i].Expr = sszMaxSizeStr
			}
		}
	}

	if err := checkDimensionsDeclared(sizeHints, maxSizeHints, ""); err != nil {
		return nil, nil, nil, err
	}

	return typeHints, sizeHints, maxSizeHints, nil
}
