// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"strings"

	fastssz "github.com/ferranbt/fastssz"
)

func (d *DynSsz) buildRootFromType(sourceType reflect.Type, sourceValue reflect.Value, hh fastssz.HashWalker, sizeHints []sszSizeHint, maxSizeHints []sszMaxSizeHint, idt int) error {
	hashIndex := hh.Index()

	if sourceType.Kind() == reflect.Ptr {
		sourceType = sourceType.Elem()
		sourceValue = sourceValue.Elem()
	}

	// use fastssz to hash types if:
	// - type implements fastssz HashRoot interface
	// - this type or any child type does not use spec specific field sizes
	fastsszCompat, err := d.getFastsszHashCompatibility(sourceType, sizeHints, maxSizeHints)
	if err != nil {
		return fmt.Errorf("failed checking fastssz compatibility: %v", err)
	}

	useFastSsz := !d.NoFastSsz && fastsszCompat.isHashRoot && !fastsszCompat.hasDynamicSpecSizes && !fastsszCompat.hasDynamicSpecMax
	if !useFastSsz && fastsszCompat.isHashRoot && !fastsszCompat.hasDynamicSpecSizes && !fastsszCompat.hasDynamicSpecMax && sourceType.Name() == "Int" {
		// hack for uint256.Int
		useFastSsz = true
	}

	fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v/%v)\t index: %v\n", strings.Repeat(" ", idt), sourceType.Name(), sourceType.Kind(), useFastSsz, fastsszCompat.isHashRoot, fastsszCompat.hasDynamicSpecSizes, fastsszCompat.hasDynamicSpecMax, hashIndex)

	if useFastSsz {
		hasher, ok := sourceValue.Addr().Interface().(fastsszHashRoot)
		if ok {
			//fmt.Printf("%stype: %s\t index: %v\t fastssz\n", strings.Repeat(" ", idt), sourceType.Name(), hashIndex)
			hashBytes, err := hasher.HashTreeRoot()
			if err != nil {
				return fmt.Errorf("failed HashTreeRoot: %v", err)
			}

			hh.PutBytes(hashBytes[:])
		} else {
			useFastSsz = false
		}
	}

	//fmt.Printf("%stype: %s\t index: %v\n", strings.Repeat(" ", idt), sourceType.Name(), hashIndex)

	if !useFastSsz {
		if strings.Contains(sourceType.Name(), "Bitlist") {
			// hack for bitlists
			maxSize := uint64(0)
			bytes := sourceValue.Bytes()
			if len(maxSizeHints) > 0 {
				maxSize = maxSizeHints[0].size
			} else {
				maxSize = uint64(len(bytes) * 8)
			}

			hh.PutBitlist(bytes, maxSize)
		} else {

			switch sourceType.Kind() {
			case reflect.Struct:
				err := d.buildRootFromStruct(sourceType, sourceValue, hh, idt)
				if err != nil {
					return err
				}
			case reflect.Array:
				err := d.buildRootFromArray(sourceType, sourceValue, hh, sizeHints, maxSizeHints, idt)
				if err != nil {
					return err
				}

			case reflect.Slice:
				err := d.buildRootFromSlice(sourceType, sourceValue, hh, sizeHints, maxSizeHints, idt)
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

	return nil
}

func (d *DynSsz) buildRootFromStruct(sourceType reflect.Type, sourceValue reflect.Value, hh fastssz.HashWalker, idt int) error {
	hashIndex := hh.Index()

	if sourceType.Kind() == reflect.Ptr {
		sourceType = sourceType.Elem()
		sourceValue = sourceValue.Elem()
	}

	for i := 0; i < sourceType.NumField(); i++ {
		field := sourceType.Field(i)
		fieldType := field.Type
		fieldValue := sourceValue.Field(i)

		fieldIsPtr := fieldType.Kind() == reflect.Ptr
		if fieldIsPtr {
			fieldType = fieldType.Elem()
			fieldValue = fieldValue.Elem()
		}

		_, _, sizeHints, err := d.getSszFieldSize(&field)
		if err != nil {
			return err
		}
		maxSizeHints, err := d.getSszMaxSizeTag(&field)
		if err != nil {
			return err
		}

		err = d.buildRootFromType(fieldType, fieldValue, hh, sizeHints, maxSizeHints, idt+2)
		if err != nil {
			return err
		}
	}

	fmt.Printf("merkelize struct %v (%v)\n", sourceType.Name(), hashIndex)
	hh.Merkleize(hashIndex)

	return nil
}

func (d *DynSsz) buildRootFromArray(sourceType reflect.Type, sourceValue reflect.Value, hh fastssz.HashWalker, sizeHints []sszSizeHint, maxSizeHints []sszMaxSizeHint, idt int) error {
	fieldType := sourceType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	if fieldType == byteType {
		hh.PutBytes(sourceValue.Bytes())
		return nil
	}

	return d.buildRootFromSlice(sourceType, sourceValue, hh, sizeHints, maxSizeHints, idt)
}

func (d *DynSsz) buildRootFromSlice(sourceType reflect.Type, sourceValue reflect.Value, hh fastssz.HashWalker, sizeHints []sszSizeHint, maxSizeHints []sszMaxSizeHint, idt int) error {
	fieldType := sourceType.Elem()
	fieldIsPtr := fieldType.Kind() == reflect.Ptr
	if fieldIsPtr {
		fieldType = fieldType.Elem()
	}

	subIndex := hh.Index()
	sliceLen := sourceValue.Len()
	itemSize := 0

	switch fieldType.Kind() {
	case reflect.Struct:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)
			if fieldIsPtr {
				fieldValue = fieldValue.Elem()
			}

			err := d.buildRootFromStruct(fieldType, fieldValue, hh, idt+2)
			if err != nil {
				return err
			}
		}
	case reflect.Array:
		itemType := fieldType.Elem()
		if itemType == byteType {
			for i := 0; i < sliceLen; i++ {
				fieldValue := sourceValue.Index(i)
				if fieldIsPtr {
					fieldValue = fieldValue.Elem()
				}

				hh.PutBytes(fieldValue.Bytes())
			}

		} else {
			fmt.Printf("non-byte array in slice: %v\n", itemType)
		}
	case reflect.Slice:
		itemType := fieldType.Elem()
		if itemType == byteType {
			for i := 0; i < sliceLen; i++ {
				fieldValue := sourceValue.Index(i)
				if fieldIsPtr {
					fieldValue = fieldValue.Elem()
				}

				hh.PutBytes(fieldValue.Bytes())
			}

		} else {
			fmt.Printf("non-byte slice in slice: %v\n", itemType)
		}
	case reflect.Uint8:
		hh.Append(sourceValue.Bytes())
		hh.FillUpTo32()
		itemSize = 1
	case reflect.Uint64:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)
			if fieldIsPtr {
				fieldValue = fieldValue.Elem()
			}

			hh.AppendUint64(uint64(fieldValue.Uint()))
		}
		itemSize = 8
	}

	if len(maxSizeHints) > 0 {
		var limit uint64
		if itemSize > 0 {
			limit = fastssz.CalculateLimit(maxSizeHints[0].size, uint64(sliceLen), uint64(itemSize))
		} else {
			limit = maxSizeHints[0].size
		}
		hh.MerkleizeWithMixin(subIndex, uint64(sliceLen), limit)
	} else {
		hh.Merkleize(subIndex)
	}

	return nil
}
