// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
)

// getSszValueSize calculates the absolute SSZ size of the specified targetValue, taking into account both simple and complex, nested types.
// It enhances performance by employing the "SizeSSZ" function from fastssz for calculating the size of structures that, along with all types
// they refer to, do not have dynamic specification values applied. This means that the size calculation defaults to the static, fastssz code path
// whenever the structure in question and its nested types are static and do not necessitate dynamic handling due to applied dynamic spec values.
//
// Parameters:
// - targetType: The TypeDescriptor of the value to be sized, which provides the necessary type information for accurately determining the size
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

func (d *DynSsz) getSszValueSize(targetType *TypeDescriptor, targetValue reflect.Value) (uint32, error) {
	staticSize := uint32(0)

	if targetType.IsPtr {
		targetValue = targetValue.Elem()
	}

	// use fastssz to calculate size if:
	// - struct implements fastssz Marshaler interface
	// - this structure or any child structure does not use spec specific field sizes
	useFastSsz := !d.NoFastSsz && targetType.IsFastSSZMarshaler

	if useFastSsz {
		marshaller, ok := targetValue.Addr().Interface().(fastsszMarshaler)
		if ok {
			staticSize = uint32(marshaller.SizeSSZ())
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz {
		// can't use fastssz, use dynamic size calculation
		switch targetType.Kind {
		case reflect.Struct:
			for i := 0; i < len(targetType.Fields); i++ {
				fieldType := targetType.Fields[i]
				fieldValue := targetValue.Field(i)

				if fieldType.Size < 0 {
					size, err := d.getSszValueSize(fieldType.Type, fieldValue)
					if err != nil {
						return 0, err
					}

					// dynamic field, add 4 bytes for offset
					staticSize += size + 4
				} else {
					// static field
					staticSize += uint32(fieldType.Size)
				}
			}
		case reflect.Array:
			if targetType.Len > 0 {
				fieldType := targetType.ElemDesc
				if fieldType.Kind == reflect.Uint8 {
					staticSize = targetType.Len
				} else {
					size, err := d.getSszValueSize(fieldType, targetValue.Index(0))
					if err != nil {
						return 0, err
					}
					staticSize = size * targetType.Len
				}
			}
		case reflect.Slice:
			fieldType := targetType.ElemDesc
			sliceLen := uint32(targetValue.Len())

			appendZero := uint32(0)
			if len(targetType.SizeHints) > 0 && !targetType.SizeHints[0].Dynamic {
				if sliceLen > targetType.SizeHints[0].Size {
					return 0, ErrListTooBig
				}
				if sliceLen < targetType.SizeHints[0].Size {
					appendZero = targetType.SizeHints[0].Size - uint32(sliceLen)
				}
			}

			if sliceLen > 0 {
				if fieldType.Kind == reflect.Uint8 {
					staticSize = uint32(sliceLen) + uint32(appendZero)
				} else if fieldType.Size < 0 {
					// slice with dynamic size items, so we have to go through each item
					for i := 0; i < int(sliceLen); i++ {
						size, err := d.getSszValueSize(fieldType, targetValue.Index(i))
						if err != nil {
							return 0, err
						}
						// add 4 bytes for offset in dynamic slice
						staticSize += size + 4
					}

					if appendZero > 0 {
						zeroVal := reflect.New(fieldType.Type).Elem()
						size, err := d.getSszValueSize(fieldType, zeroVal)
						if err != nil {
							return 0, err
						}

						staticSize += (size + 4) * appendZero
					}
				} else {
					staticSize = uint32(fieldType.Size) * (sliceLen + appendZero)
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
			return 0, fmt.Errorf("unhandled reflection kind in size check: %v", targetType.Kind)
		}
	}

	return staticSize, nil
}
