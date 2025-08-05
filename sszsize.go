// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
)

// getSszValueSize calculates the exact SSZ-encoded size of a value.
//
// This internal function is used by SizeSSZ to determine buffer requirements for serialization.
// It recursively traverses the value structure, calculating sizes based on SSZ encoding rules:
//   - Fixed-size types have predetermined sizes
//   - Dynamic types require 4-byte offset markers plus their content size
//   - Arrays multiply element size by length
//   - Slices account for actual length and any padding from size hints
//
// The function optimizes performance by delegating to fastssz's SizeSSZ method when:
//   - The type implements the fastssz Marshaler interface
//   - The type and all nested types have static sizes (no dynamic spec values)
//
// Parameters:
//   - targetType: The TypeDescriptor containing type metadata and size information
//   - targetValue: The reflect.Value containing the actual data to size
//
// Returns:
//   - uint32: The exact number of bytes needed to encode this value
//   - error: An error if sizing fails (e.g., slice exceeds maximum size)
//
// Special handling:
//   - Nil pointers are sized as zero-valued instances
//   - Dynamic slices include padding for size hint compliance
//   - Struct fields are sized based on their static/dynamic nature

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
				} else if fieldType.Size < 0 {
					// array with dynamic size items, so we have to go through each item
					for i := 0; i < int(targetType.Len); i++ {
						size, err := d.getSszValueSize(fieldType, targetValue.Index(i))
						if err != nil {
							return 0, err
						}
						// add 4 bytes for offset in dynamic array
						staticSize += size + 4
					}
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
		case reflect.String:
			// String size depends on whether it's fixed or dynamic
			if targetType.Size > 0 {
				// Fixed-size string: always return the fixed size
				staticSize = uint32(targetType.Size)
			} else {
				// Dynamic string: return the actual length
				staticSize = uint32(len(targetValue.String()))
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
