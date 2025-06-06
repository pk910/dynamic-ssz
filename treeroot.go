// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"strings"
)

// buildRootFromType is the entry point for computing HashTreeRoot of Go values, using reflection to navigate
// the type tree and hash each value appropriately. It serves as the core function for the recursive hashing process,
// handling both primitive and composite types.
//
// Parameters:
// - sourceType: The reflect.Type of the value to be hashed. This provides the necessary type information to guide
//   the hashing process for both simple and complex types.
// - sourceValue: The reflect.Value that holds the data to be hashed. This function uses sourceValue to extract
//   the actual data for computing the HashTreeRoot.
// - hh: A Hasher instance that maintains the state of the hashing process and provides methods for hashing
//   different types of values according to SSZ specifications.
// - sizeHints: A slice of sszSizeHint, populated from 'ssz-size' and 'dynssz-size' tag annotations from parent
//   structures, crucial for hashing types that may have dynamic lengths.
// - maxSizeHints: A slice of sszMaxSizeHint, providing maximum size constraints for variable-length types like
//   lists and bitlists, ensuring compliance with SSZ specifications.
// - idt: An indentation level, primarily used for debugging or logging to help track the recursion depth and hashing
//   sequence of the data structure.
//
// Returns:
// - An error if the hashing process encounters any issues, such as an unsupported type or a mismatch between
//   the sourceValue and the expected type structure.
//
// This function serves as the primary dispatcher within the hashing process, computing hashes for primitive types directly
// and delegating the hashing of composite types to specialized functions. For composite types, buildRootFromType
// orchestrates the process by preparing the necessary context and parameters, then calling the appropriate specialized
// function based on the type of sourceValue.

func (d *DynSsz) buildRootFromType(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	hashIndex := hh.Index()

	if sourceType.IsPtr {
		sourceValue = sourceValue.Elem()
	}

	useFastSsz := !d.NoFastSsz && sourceType.IsFastSSZHasher && !sourceType.HasDynamicSize && !sourceType.HasDynamicMax
	if !useFastSsz && sourceType.IsFastSSZHasher && !sourceType.HasDynamicSize && !sourceType.HasDynamicMax && sourceValue.Type().Name() == "Int" {
		// hack for uint256.Int
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v/%v)\t index: %v\n", strings.Repeat(" ", idt), sourceType.Type.Name(), sourceType.Kind, useFastSsz, sourceType.IsFastSSZHasher, sourceType.HasDynamicSize, sourceType.HasDynamicMax, hashIndex)
	}

	if useFastSsz {
		if hasher, ok := sourceValue.Addr().Interface().(fastsszHashRoot); ok {
			hashBytes, err := hasher.HashTreeRoot()
			if err != nil {
				return fmt.Errorf("failed HashTreeRoot: %v", err)
			}

			hh.PutBytes(hashBytes[:])
		} else {
			useFastSsz = false
		}
	}

	if !useFastSsz {
		if strings.Contains(sourceType.Type.Name(), "Bitlist") {
			// hack for bitlists
			maxSize := uint64(0)
			bytes := sourceValue.Bytes()
			if len(sourceType.MaxSizeHints) > 0 {
				maxSize = uint64(sourceType.MaxSizeHints[0].Size)
			} else {
				maxSize = uint64(len(bytes) * 8)
			}

			hh.PutBitlist(bytes, maxSize)
		} else {

			switch sourceType.Kind {
			case reflect.Struct:
				err := d.buildRootFromStruct(sourceType, sourceValue, hh, idt)
				if err != nil {
					return err
				}
			case reflect.Array:
				err := d.buildRootFromSlice(sourceType, sourceValue, hh, true, idt)
				if err != nil {
					return err
				}

			case reflect.Slice:
				err := d.buildRootFromSlice(sourceType, sourceValue, hh, false, idt)
				if err != nil {
					return err
				}

			case reflect.Bool:
				hh.PutBool(sourceValue.Bool())
			case reflect.Uint8:
				hh.PutUint8(uint8(sourceValue.Uint()))
			case reflect.Uint16:
				hh.PutUint16(uint16(sourceValue.Uint()))
			case reflect.Uint32:
				hh.PutUint32(uint32(sourceValue.Uint()))
			case reflect.Uint64:
				hh.PutUint64(uint64(sourceValue.Uint()))
			default:
				return fmt.Errorf("unknown type: %v", sourceType)
			}
		}
	}

	if d.Verbose {
		fmt.Printf("%shash: 0x%x\n", strings.Repeat(" ", idt), hh.Hash())
	}

	return nil
}

// buildRootFromStruct handles the computation of HashTreeRoot for Go struct values. It iterates through each field
// of the struct, leveraging reflection to access field types and values, and delegates the hashing of each field
// to buildRootFromType.
//
// Parameters:
// - sourceType: The reflect.Type of the struct to be hashed, providing the necessary type information to guide
//   the hashing process for the struct's fields.
// - sourceValue: The reflect.Value that holds the struct data to be hashed. buildRootFromStruct iterates over
//   each field of the struct and uses sourceValue to extract the data for hashing.
// - hh: A Hasher instance that maintains the state of the hashing process and provides methods for hashing
//   different types of values according to SSZ specifications.
// - idt: An indentation level, primarily used for debugging or logging to help track the recursion depth and hashing
//   sequence of the struct fields.
//
// Returns:
// - An error if the hashing process encounters any issues, such as an unsupported field type or a mismatch between
//   the sourceValue's actual data and what is expected for SSZ hashing.
//
// buildRootFromStruct specifically focuses on the structural aspects of hashing a struct, ensuring that each field
// is properly hashed and combined according to SSZ specifications. The function processes each field sequentially,
// building up the final hash by combining the hashes of individual fields using the Merkleization process.

func (d *DynSsz) buildRootFromStruct(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	hashIndex := hh.Index()

	for i := 0; i < len(sourceType.Fields); i++ {
		field := sourceType.Fields[i]
		fieldType := field.Type
		fieldValue := sourceValue.Field(i)

		if d.Verbose {
			fmt.Printf("%vfield %v\n", strings.Repeat(" ", idt), field.Name)
		}

		err := d.buildRootFromType(fieldType, fieldValue, hh, idt+2)
		if err != nil {
			return err
		}
	}
	hh.Merkleize(hashIndex)

	return nil
}

// buildRootFromSlice handles the computation of HashTreeRoot for Go slice and array values. It processes each element
// of the collection, using reflection to access element types and values, and delegates the hashing of individual
// elements to buildRootFromType.
//
// Parameters:
// - sourceType: The reflect.Type of the slice/array to be hashed, providing the type information needed to hash
//   each element within the collection correctly.
// - sourceValue: The reflect.Value that holds the slice/array data to be hashed. buildRootFromSlice iterates over
//   each element, using sourceValue to extract the data for hashing.
// - hh: A Hasher instance that maintains the state of the hashing process and provides methods for hashing
//   different types of values according to SSZ specifications.
// - maxSizeHints: A slice of sszMaxSizeHint, providing maximum size constraints for variable-length types,
//   ensuring compliance with SSZ specifications during the hashing process.
// - isArray: A boolean indicating whether the source is an array (true) or slice (false), affecting how the
//   function handles the collection's length and memory layout.
// - idt: An indentation level used for debugging or logging to track the hashing depth and sequence.
//
// Returns:
// - An error if the hashing process encounters any issues, such as an unsupported element type or size constraint
//   violations.
//
// buildRootFromSlice specializes in hashing collections by properly handling both fixed-size arrays and variable-length
// slices. It implements specific optimizations for common types like byte slices and uint64 arrays while maintaining
// the ability to process complex nested structures through recursive calls to buildRootFromType.

func (d *DynSsz) buildRootFromSlice(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, isArray bool, idt int) error {
	fieldType := sourceType.ElemDesc

	subIndex := hh.Index()
	sliceLen := sourceValue.Len()
	itemSize := 0

	switch fieldType.Kind {
	case reflect.Struct:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)
			if fieldType.IsPtr {
				fieldValue = fieldValue.Elem()
			}

			err := d.buildRootFromStruct(fieldType, fieldValue, hh, idt+2)
			if err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		itemType := fieldType.ElemDesc
		if itemType.Kind == reflect.Uint8 {
			for i := 0; i < sliceLen; i++ {
				sliceSubIndex := hh.Index()

				fieldValue := sourceValue.Index(i)
				if itemType.IsPtr {
					fieldValue = fieldValue.Elem()
				}

				fieldBytes := fieldValue.Bytes()
				byteLen := uint64(len(fieldBytes))

				// we might need to merkelize the child array too.
				// check if we have size hints
				if len(sourceType.MaxSizeHints) > 1 {
					hh.AppendBytes32(fieldBytes)
					hh.MerkleizeWithMixin(sliceSubIndex, byteLen, uint64((sourceType.MaxSizeHints[1].Size+31)/32))
				} else {
					hh.PutBytes(fieldBytes)
				}
			}
		} else {
			return fmt.Errorf("non-byte slice/array in slice: %v", itemType.Type.Name())
		}
	case reflect.Uint8:
		if isArray {
			hh.PutBytes(sourceValue.Bytes())
			return nil
		}

		hh.Append(sourceValue.Bytes())
		hh.FillUpTo32()
		itemSize = 1
	case reflect.Uint64:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)
			if fieldType.IsPtr {
				fieldValue = fieldValue.Elem()
			}

			hh.AppendUint64(uint64(fieldValue.Uint()))
		}
		itemSize = 8
	}

	if len(sourceType.MaxSizeHints) > 0 {
		var limit uint64
		if itemSize > 0 {
			limit = calculateLimit(uint64(sourceType.MaxSizeHints[0].Size), uint64(sliceLen), uint64(itemSize))
		} else {
			limit = uint64(sourceType.MaxSizeHints[0].Size)
		}
		hh.MerkleizeWithMixin(subIndex, uint64(sliceLen), limit)
	} else {
		hh.Merkleize(subIndex)
	}

	return nil
}
