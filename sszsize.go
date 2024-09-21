// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
)

type cachedSszSize struct {
	size    int
	specval bool
}

// getSszSize calculates the SSZ size of a given type, differentiating between static and dynamic sizes. It recursively
// analyzes the target type to determine its static size or identifies it as dynamically sized if it is inherently dynamic
// or contains dynamic elements at any level of its structure.
//
// Parameters:
// - targetType: The reflect.Type for which the SSZ size is being calculated. This type may be a simple, static-sized type,
//   a complex type with static-sized elements, or a dynamic type that contains dynamic elements or refers to dynamic types.
// - sizeHints: A slice of sszSizeHint, populated from 'ssz-size' and 'dynssz-size' tag annotations from parent structures,
//   which are essential for accurately calculating sizes for types with dynamic lengths or when specific instances
//   of types differ from their default specifications.
//
// Returns:
// - The calculated size of the type in its SSZ representation. This size is either a positive integer for static-sized types
//   or -1 for types identified as dynamically sized.
// - A boolean indicating whether a dynamic specification value, differing from the default, has been applied anywhere within
//   the type structure. This flag is crucial for downstream functions to decide between utilizing dynamic marshalling or,
//   when no dynamic values are applied and it's available, opting for the static code path.
// - An error, if the size calculation encounters any issues, such as an unsupported type or an inability to resolve sizes
//   based on the provided sizeHints.
//
// getSszSize plays a pivotal role in the marshalling and unmarshalling processes by identifying whether types are static
// or dynamic in size and signaling when dynamic encoding or decoding is necessary. This function ensures that the appropriate
// encoding or decoding path is chosen based on the type's nature and any dynamic specifications applied to it.

func (d *DynSsz) getSszSize(targetType reflect.Type, sizeHints []sszSizeHint) (int, bool, error) {
	staticSize := 0
	hasSpecValue := false
	isDynamicSize := false

	childSizeHints := []sszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	// resolve pointers to value type
	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}

	// get size from cache if not influenced by a parent sizeHint
	d.typeSizeMutex.RLock()
	if cachedSize := d.typeSizeCache[targetType]; cachedSize != nil && len(sizeHints) == 0 {
		d.typeSizeMutex.RUnlock()
		return cachedSize.size, cachedSize.specval, nil
	}
	d.typeSizeMutex.RUnlock()

	switch targetType.Kind() {
	case reflect.Struct:
		for i := 0; i < targetType.NumField(); i++ {
			field := targetType.Field(i)

			if i == 0 {
				stableMax, err := d.getSszStableMaxTag(&field)
				if err != nil {
					return 0, false, err
				}

				if stableMax > 0 {
					isDynamicSize = true // stable container are always dynamic
				}
			}

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

	// primitive types
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
		// cache size if it's static and not influenced by a parent sizeHint
		d.typeSizeMutex.Lock()
		d.typeSizeCache[targetType] = &cachedSszSize{
			size:    staticSize,
			specval: hasSpecValue,
		}
		d.typeSizeMutex.Unlock()
	}

	return staticSize, hasSpecValue, nil
}

// getSszFieldSize calculates the SSZ size of a specific struct field, assessing whether it's statically or dynamically sized.
// This function operates within a recursion loop with getSszSize, evaluating the size of individual fields within a struct
// by potentially invoking getSszSize for nested structures or composite types, thus forming part of the comprehensive type
// size determination process.
//
// Parameters:
// - targetField: A pointer to the reflect.StructField being analyzed. This provides the context to assess the field's type,
//   including its size characteristics and any tag annotations that might influence the size calculation.
//
// Returns:
// - The calculated size of the field in its SSZ representation, either as a positive integer for static-sized fields or -1
//   for fields identified as dynamically sized.
// - A boolean indicating whether the field is influenced by a dynamic specification value that differs from the default.
//   This information is critical for deciding between static or dynamic handling in the encoding or decoding process.
// - A slice of sszSizeHint that could be relevant for further size calculations of elements within the field that possess
//   dynamic sizes. These hints, derived from 'ssz-size' and 'dynssz-size' tag annotations, are indispensable for accurate
//   size calculations in types with variable lengths.
// - An error if the size calculation encounters challenges, such as unsupported field types or issues interpreting tag annotations.

func (d *DynSsz) getSszFieldSize(targetField *reflect.StructField) (int, bool, []sszSizeHint, error) {
	sszSizes, err := d.getSszSizeTag(targetField)
	if err != nil {
		return 0, false, nil, err
	}

	size, hasSpecVal, err := d.getSszSize(targetField.Type, sszSizes)
	return size, hasSpecVal, sszSizes, err
}

// getSszValueSize calculates the absolute SSZ size of the specified targetValue, taking into account both simple and complex, nested types.
// It enhances performance by employing the "SizeSSZ" function from fastssz for calculating the size of structures that, along with all types
// they refer to, do not have dynamic specification values applied. This means that the size calculation defaults to the static, fastssz code path
// whenever the structure in question and its nested types are static and do not necessitate dynamic handling due to applied dynamic spec values.
//
// Parameters:
// - targetType: The reflect.Type of the value to be sized, which provides the necessary type information for accurately determining the size
//   of the value. This detail is especially vital for composite types potentially containing nested dynamic elements.
// - targetValue: The reflect.Value containing the actual data to be sized. This function examines targetValue to calculate the size of the value itself
//   and any of its nested values, resorting to fastssz's "SizeSSZ" for static structures and their statically typed components to optimize performance.
//
// Returns:
// - An integer indicating the total size of targetValue in its SSZ-encoded form. This size encompasses the contributions from all nested
//   or composite elements within the value.
// - An error, if the size calculation process encounters challenges, such as unsupported types or intricate nested structures that defy
//   accurate sizing with the available information.
//
// By adeptly utilizing fastssz's "SizeSSZ" for structures without dynamic spec values and their statically typed references, getSszValueSize ensures
// efficient and accurate size calculations. This approach allows for the dynamic encoding process to proceed with precise size information, essential
// for correctly encoding data into the SSZ format across a broad spectrum of data types, ranging from straightforward primitives to elaborate nested structures.

func (d *DynSsz) getSszValueSize(targetType reflect.Type, targetValue reflect.Value, sizeHints []sszSizeHint) (int, error) {
	staticSize := 0

	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
		targetValue = targetValue.Elem()
	}

	// use fastssz to calculate size if:
	// - struct implements fastssz Marshaler interface
	// - this structure or any child structure does not use spec specific field sizes
	fastsszCompat, err := d.getFastsszCompatibility(targetType, sizeHints)
	if err != nil {
		return 0, fmt.Errorf("failed checking fastssz compatibility: %v", err)
	}

	useFastSsz := !d.NoFastSsz && fastsszCompat.isMarshaler && !fastsszCompat.hasDynamicSpecValues

	if useFastSsz {
		marshaller, ok := targetValue.Addr().Interface().(fastsszMarshaler)
		if ok {
			staticSize = marshaller.SizeSSZ()
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz {
		// can't use fastssz, use dynamic size calculation

		childSizeHints := []sszSizeHint{}
		if len(sizeHints) > 1 {
			childSizeHints = sizeHints[1:]
		}

		switch targetType.Kind() {
		case reflect.Struct:
			stableMax := uint64(0)

			for i := 0; i < targetType.NumField(); i++ {
				field := targetType.Field(i)

				if i == 0 {
					stableMax, err = d.getSszStableMaxTag(&field)
					if err != nil {
						return 0, err
					}

					if stableMax > 0 {
						staticSize += int((stableMax + 7) / 8)
					}
				}
				if stableMax > 0 && field.Type.Kind() == reflect.Pointer && targetValue.Field(i).IsNil() {
					continue // inactive field
				}

				fieldValue := targetValue.Field(i)

				fieldTypeSize, _, fieldSizeHints, err := d.getSszFieldSize(&field)
				if err != nil {
					return 0, err
				}

				if fieldTypeSize < 0 {
					size, err := d.getSszValueSize(field.Type, fieldValue, fieldSizeHints)
					if err != nil {
						return 0, err
					}

					// dynamic field, add 4 bytes for offset
					staticSize += size + 4
				} else {
					// static field
					staticSize += fieldTypeSize
				}
			}
		case reflect.Array:
			arrLen := targetType.Len()
			if arrLen > 0 {
				fieldType := targetType.Elem()
				if fieldType == byteType {
					staticSize = arrLen
				} else {
					size, err := d.getSszValueSize(fieldType, targetValue.Index(0), childSizeHints)
					if err != nil {
						return 0, err
					}
					staticSize = size * arrLen
				}
			}
		case reflect.Slice:
			fieldType := targetType.Elem()
			sliceLen := targetValue.Len()

			appendZero := 0
			if len(sizeHints) > 0 && !sizeHints[0].dynamic {
				if uint64(sliceLen) > sizeHints[0].size {
					return 0, ErrListTooBig
				}
				if uint64(sliceLen) < sizeHints[0].size {
					appendZero = int(sizeHints[0].size - uint64(sliceLen))
				}
			}

			if sliceLen > 0 {
				if fieldType == byteType {
					staticSize = sliceLen + appendZero
				} else {
					fieldTypeSize, _, err := d.getSszSize(fieldType, childSizeHints)
					if err != nil {
						return 0, err
					}

					if fieldTypeSize < 0 {
						// slice with dynamic size items, so we have to go through each item
						for i := 0; i < sliceLen; i++ {
							size, err := d.getSszValueSize(fieldType, targetValue.Index(i), childSizeHints)
							if err != nil {
								return 0, err
							}
							// add 4 bytes for offset in dynamic slice
							staticSize += size + 4
						}

						if appendZero > 0 {
							zeroVal := reflect.New(fieldType).Elem()
							size, err := d.getSszValueSize(fieldType, zeroVal, childSizeHints)
							if err != nil {
								return 0, err
							}

							staticSize += (size + 4) * appendZero
						}
					} else {
						staticSize = fieldTypeSize * (sliceLen + appendZero)
					}
				}
			}

		// primitive types
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
	}

	return staticSize, nil
}
