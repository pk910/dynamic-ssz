// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"

	"github.com/pk910/dynamic-ssz/sszutils"
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
		if targetValue.IsNil() {
			targetValue = reflect.New(targetType.Type.Elem()).Elem()
		} else {
			targetValue = targetValue.Elem()
		}
	}

	// use fastssz to calculate size if:
	// - struct implements fastssz Marshaler interface
	// - this structure or any child structure does not use spec specific field sizes
	useFastSsz := !d.NoFastSsz && targetType.HasFastSSZMarshaler
	if !useFastSsz && targetType.SszType == SszCustomType {
		useFastSsz = true
	}

	if useFastSsz {
		marshaller, ok := targetValue.Addr().Interface().(sszutils.FastsszMarshaler)
		if ok {
			staticSize = uint32(marshaller.SizeSSZ())
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz {
		// can't use fastssz, use dynamic size calculation
		switch targetType.SszType {
		case SszTypeWrapperType:
			// Extract the Data field from the TypeWrapper
			dataField := targetValue.Field(0)
			if !dataField.IsValid() {
				return 0, fmt.Errorf("TypeWrapper missing 'Data' field")
			}

			// Calculate size for the wrapped value using its type descriptor
			size, err := d.getSszValueSize(targetType.ElemDesc, dataField)
			if err != nil {
				return 0, err
			}
			staticSize = size
		case SszContainerType:
			for i := 0; i < len(targetType.ContainerDesc.Fields); i++ {
				fieldType := targetType.ContainerDesc.Fields[i]
				fieldValue := targetValue.Field(i)

				if fieldType.Type.IsDynamic {
					size, err := d.getSszValueSize(fieldType.Type, fieldValue)
					if err != nil {
						return 0, err
					}

					// dynamic field, add 4 bytes for offset
					staticSize += size + 4
				} else {
					// static field
					staticSize += uint32(fieldType.Type.Size)
				}
			}
		case SszVectorType, SszBitvectorType:
			fieldType := targetType.ElemDesc
			if fieldType.Kind == reflect.Uint8 {
				staticSize = targetType.Len
			} else if fieldType.IsDynamic {
				// vector with dynamic size items, so we have to go through each item
				dataLen := targetValue.Len()

				for i := 0; i < dataLen; i++ {
					size, err := d.getSszValueSize(fieldType, targetValue.Index(i))
					if err != nil {
						return 0, err
					}
					// add 4 bytes for offset in dynamic array
					staticSize += size + 4
				}

				if dataLen < int(targetType.Len) {
					appendZero := targetType.Len - uint32(dataLen)
					zeroVal := reflect.New(fieldType.Type).Elem()
					size, err := d.getSszValueSize(fieldType, zeroVal)
					if err != nil {
						return 0, err
					}

					staticSize += (size + 4) * appendZero
				}
			} else {
				dataLen := targetValue.Len()

				if dataLen > 0 {
					size, err := d.getSszValueSize(fieldType, targetValue.Index(0))
					if err != nil {
						return 0, err
					}

					staticSize = size * targetType.Len
				} else {
					zeroVal := reflect.New(fieldType.Type).Elem()
					size, err := d.getSszValueSize(fieldType, zeroVal)
					if err != nil {
						return 0, err
					}

					staticSize += size * targetType.Len
				}
			}
		case SszListType, SszBitlistType:
			fieldType := targetType.ElemDesc
			sliceLen := uint32(targetValue.Len())

			if sliceLen > 0 {
				if fieldType.Kind == reflect.Uint8 {
					staticSize = uint32(sliceLen)
				} else if fieldType.IsDynamic {
					// slice with dynamic size items, so we have to go through each item
					for i := 0; i < int(sliceLen); i++ {
						size, err := d.getSszValueSize(fieldType, targetValue.Index(i))
						if err != nil {
							return 0, err
						}
						// add 4 bytes for offset in dynamic slice
						staticSize += size + 4
					}
				} else {
					staticSize = uint32(fieldType.Size) * sliceLen
				}
			}

		// primitive types
		case SszBoolType:
			staticSize = 1
		case SszUint8Type:
			staticSize = 1
		case SszUint16Type:
			staticSize = 2
		case SszUint32Type:
			staticSize = 4
		case SszUint64Type:
			staticSize = 8
		case SszUint128Type:
			staticSize = 16
		case SszUint256Type:
			staticSize = 32

		default:
			return 0, fmt.Errorf("unhandled reflection kind in size check: %v", targetType.Kind)
		}
	}

	return staticSize, nil
}
