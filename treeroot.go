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

func (d *DynSsz) buildRootFromStruct(sourceType reflect.Type, sourceValue reflect.Value, hh fastssz.HashWalker, idt int) error {
	hashIndex := hh.Index()

	if sourceType.Kind() == reflect.Ptr {
		sourceType = sourceType.Elem()
		sourceValue = sourceValue.Elem()
	}
	if sourceType.Kind() != reflect.Struct {
		return fmt.Errorf("source type is not of kind struct")
	}

	fmt.Printf("%stype: %s\t index: %v\n", strings.Repeat(" ", idt), sourceType.Name(), hashIndex)

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

		switch fieldType.Kind() {
		case reflect.Struct:
			err = d.buildRootFromStruct(fieldType, fieldValue, hh, idt+2)
		case reflect.Array:
			itemType := fieldType.Elem()
			if itemType == byteType {
				hh.PutBytes(fieldValue.Bytes())
			} else {
				// build byte array
				buf := make([]byte, 0)
				buf, err = d.marshalArray(fieldType, fieldValue, buf, sizeHints, idt+2)
				if err != nil {
					return err
				}

				fmt.Printf("non-byte array %v: 0x%x\n", itemType, buf)
				hh.PutBytes(buf)
			}
		case reflect.Slice:
			itemType := fieldType.Elem()
			if itemType == byteType {
				hh.PutBytes(fieldValue.Bytes())
			} else {
				err = d.buildRootFromSlice(fieldType, fieldValue, hh, sizeHints, maxSizeHints, idt+2)
			}
		case reflect.Bool:
			hh.PutBool(fieldValue.Bool())
		case reflect.Uint8:
			hh.PutUint8(uint8(fieldValue.Uint()))
		case reflect.Uint16:
			hh.PutUint16(uint16(fieldValue.Uint()))
		case reflect.Uint32:
			hh.PutUint32(uint32(fieldValue.Uint()))
		case reflect.Uint64:
			hh.PutUint64(uint64(fieldValue.Uint()))
		}

		if err != nil {
			return err
		}
	}

	hh.Merkleize(hashIndex)

	return nil
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

				hh.Append(fieldValue.Bytes())
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

				hh.Append(fieldValue.Bytes())
			}

		} else {
			fmt.Printf("non-byte slice in slice: %v\n", itemType)
		}
	case reflect.Uint8:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)
			if fieldIsPtr {
				fieldValue = fieldValue.Elem()
			}

			hh.AppendUint8(uint8(fieldValue.Uint()))
		}
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
