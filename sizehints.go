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

// sszSizeHint encapsulates size information for SSZ encoding and decoding, derived from 'ssz-size' and 'dynssz-size' tag annotations.
// It provides detailed insights into the size attributes of fields or types, particularly noting whether sizes are fixed or dynamic,
// and if special specification values are applied, differing from default assumptions.
//
// Fields:
// - size: A uint64 value indicating the statically annotated size of the type or field, as specified by 'ssz-size' tag annotations.
//   For dynamic fields, where the size may vary depending on the instance of the data, this field is set to 0, and the dynamic flag
//   is used to indicate its dynamic nature.
// - dynamic: A boolean flag indicating whether the field's size is dynamic, set to true for fields whose size can change or is not fixed
//   at compile time. This determination is based on the presence of 'dynssz-size' annotations or the inherent variability of the type.
// - specval: A boolean indicating whether a non-default specification value has been applied to the type or field, typically through
//   'dynssz-size' annotations, suggesting a deviation from standard size expectations that might influence the encoding or decoding process.

type sszSizeHint struct {
	size    uint64
	dynamic bool
	specval bool
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
// - A slice of sszSizeHint, which are derived from the parsed tag annotations. These hints inform the marshalling
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

func (d *DynSsz) getSszSizeTag(field *reflect.StructField) ([]sszSizeHint, error) {
	sszSizes := []sszSizeHint{}

	// parse `ssz-size` first, these are the default values used by fastssz
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
					// unknown spec value? fallback to fastssz defaults
					break
				}
			}

			if i >= len(sszSizes) {
				sszSizes = append(sszSizes, sszSize)
			} else if sszSizes[i].size != sszSize.size {
				// update if resolved size differs from default
				sszSizes[i] = sszSize
			}
		}
	}

	return sszSizes, nil
}
