// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
	"reflect"
	"strings"
)

// buildRootFromType is the core recursive function for computing hash tree roots of Go values.
//
// This function serves as the primary dispatcher within the hashing process, handling both
// primitive and composite types. It uses the TypeDescriptor's metadata to determine the most
// efficient hashing path, automatically leveraging fastssz when possible for optimal performance.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing optimized metadata about the type to hash
//   - sourceValue: The reflect.Value holding the data to be hashed
//   - hh: The Hasher instance managing the hash computation state
//   - idt: Indentation level for verbose logging (when enabled)
//
// Returns:
//   - error: An error if hashing fails
//
// The function handles:
//   - Automatic nil pointer dereferencing
//   - FastSSZ delegation for compatible types (HashTreeRootWith or HashTreeRoot methods)
//   - Special handling for Bitlist types
//   - Primitive type hashing (bool, uint8, uint16, uint32, uint64)
//   - Delegation to specialized functions for composite types (structs, arrays, slices)

func (d *DynSsz) buildRootFromType(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	hashIndex := hh.Index()

	if sourceType.IsPtr {
		if sourceValue.IsNil() {
			sourceValue = reflect.New(sourceType.Type.Elem()).Elem()
		} else {
			sourceValue = sourceValue.Elem()
		}
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
		if sourceType.HasHashTreeRootWith {
			// Use HashTreeRootWith for better performance via reflection
			value := sourceValue.Addr()
			method := value.MethodByName("HashTreeRootWith")
			if method.IsValid() {
				// Call the method with our hasher
				results := method.Call([]reflect.Value{reflect.ValueOf(hh)})
				if len(results) > 0 && !results[0].IsNil() {
					return fmt.Errorf("failed HashTreeRootWith: %v", results[0].Interface())
				}
			} else {
				// Fall back to regular HashTreeRoot
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
		} else {
			// Use regular HashTreeRoot
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
	}

	if !useFastSsz {
		isBitlist := strings.Contains(sourceType.Type.Name(), "Bitlist")
		isProgressiveBitlist := false
		if len(sourceType.TypeHints) > 0 && (sourceType.TypeHints[0].Type == SszBitlistType || sourceType.TypeHints[0].Type == SszProgressiveBitlistType) {
			isBitlist = true
			isProgressiveBitlist = sourceType.TypeHints[0].Type == SszProgressiveBitlistType
		}
		// Special case for bitlists - hack
		if isBitlist {
			maxSize := uint64(0)
			bytes := sourceValue.Bytes()

			if isProgressiveBitlist {
				hh.PutProgressiveBitlist(bytes)
			} else {
				if len(sourceType.MaxSizeHints) > 0 {
					maxSize = uint64(sourceType.MaxSizeHints[0].Size)
				} else {
					maxSize = uint64(len(bytes) * 8)
				}

				hh.PutBitlist(bytes, maxSize)
			}
		} else {
			// Route to appropriate handler based on type
			switch sourceType.Kind {
			case reflect.Struct:
				// Check if this is a CompatibleUnion
				if sourceType.IsCompatibleUnion {
					err := d.buildRootFromCompatibleUnion(sourceType, sourceValue, hh, idt)
					if err != nil {
						return err
					}
				} else {
					err := d.buildRootFromStruct(sourceType, sourceValue, hh, idt)
					if err != nil {
						return err
					}
				}
			case reflect.Array:
				err := d.buildRootFromArray(sourceType, sourceValue, hh, idt)
				if err != nil {
					return err
				}
			case reflect.Slice:
				err := d.buildRootFromSlice(sourceType, sourceValue, hh, idt)
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

// buildRootFromStruct computes the hash tree root for Go struct values.
//
// In SSZ, struct hashing follows these rules:
//   - Each field is hashed independently to produce a 32-byte root
//   - All field roots are collected in order
//   - The collection is Merkleized to produce the struct's root
//
// For progressive containers, the merkleization is done using the progressive
// algorithm with active fields mixing.
//
// The function uses the pre-computed TypeDescriptor to efficiently iterate through
// fields without repeated reflection calls.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing struct field metadata
//   - sourceValue: The reflect.Value of the struct to hash
//   - hh: The Hasher instance for hash computation
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if any field hashing fails
//
// The Merkleize call at the end combines all field hashes into the final root
// using binary tree hashing with zero-padding to the next power of two.

func (d *DynSsz) buildRootFromStruct(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	hashIndex := hh.Index()

	for i := 0; i < len(sourceType.Fields); i++ {
		field := sourceType.Fields[i]
		fieldType := field.Type
		fieldValue := sourceValue.Field(i)

		if d.Verbose {
			fmt.Printf("%sfield %v\n", strings.Repeat(" ", idt), field.Name)
		}

		err := d.buildRootFromType(fieldType, fieldValue, hh, idt+2)
		if err != nil {
			return err
		}
	}

	// Use progressive merkleization for progressive containers
	if sourceType.IsProgressiveContainer {
		// Get active fields based on the struct value
		activeFields := d.getActiveFields(sourceType)

		// merkleize progressively with active fields
		hh.MerkleizeProgressiveWithActiveFields(hashIndex, activeFields)
	} else {
		hh.Merkleize(hashIndex)
	}

	return nil
}

// buildRootFromCompatibleUnion computes the hash tree root for CompatibleUnion values.
//
// According to the spec:
// - hash_tree_root(value.data) if value is of compatible union type
// - The selector is only used for serialization, it is not mixed in when Merkleizing
//
// Parameters:
//   - sourceType: The TypeDescriptor containing union metadata and variant descriptors
//   - sourceValue: The reflect.Value of the CompatibleUnion to hash
//   - hh: The Hasher instance for hash computation
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if hashing fails
func (d *DynSsz) buildRootFromCompatibleUnion(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	// We know CompatibleUnion has exactly 2 fields: Variant (uint8) and Data (interface{})
	// Field 0 is Variant, Field 1 is Data
	variant := uint8(sourceValue.Field(0).Uint())
	dataField := sourceValue.Field(1)
	
	// Get the variant descriptor
	variantDesc, ok := sourceType.UnionVariants[variant]
	if !ok {
		return fmt.Errorf("unknown union variant index: %d", variant)
	}
	
	// Hash only the data, not the selector
	err := d.buildRootFromType(variantDesc, dataField.Elem(), hh, idt+2)
	if err != nil {
		return fmt.Errorf("failed to hash union variant %d: %w", variant, err)
	}
	
	return nil
}

// buildRootFromArray computes the hash tree root for Go array values.
//
// Arrays in SSZ are hashed based on their element type:
//   - Byte arrays: Treated as a single value, chunked into 32-byte segments
//   - Other arrays: Each element is hashed individually, then Merkleized
//
// For arrays with max size hints, the function uses MerkleizeWithMixin to include
// the array length in the final hash computation.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing array metadata
//   - sourceValue: The reflect.Value of the array to hash
//   - hh: The Hasher instance for hash computation
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if element hashing fails
//
// Special handling:
//   - Byte arrays use PutBytes for efficient chunk-based hashing
//   - Arrays with max size hints include length mixing for proper limits

func (d *DynSsz) buildRootFromArray(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	fieldType := sourceType.ElemDesc

	// For byte arrays, handle as a single unit
	if fieldType.Kind == reflect.Uint8 {
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(sourceType.Type)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}

		hh.PutBytes(sourceValue.Bytes())
		return nil
	}

	// For other types, process each element
	hashIndex := hh.Index()
	arrayLen := sourceValue.Len()
	for i := 0; i < arrayLen; i++ {
		fieldValue := sourceValue.Index(i)

		err := d.buildRootFromType(fieldType, fieldValue, hh, idt+2)
		if err != nil {
			return err
		}
	}

	if len(sourceType.MaxSizeHints) > 0 && !sourceType.MaxSizeHints[0].NoValue {
		var limit uint64
		if fieldType.Size > 0 {
			limit = CalculateLimit(uint64(sourceType.MaxSizeHints[0].Size), uint64(arrayLen), uint64(fieldType.Size))
		} else {
			limit = uint64(sourceType.MaxSizeHints[0].Size)
		}
		hh.MerkleizeWithMixin(hashIndex, uint64(arrayLen), limit)
	} else {
		hh.Merkleize(hashIndex)
	}

	return nil
}

// buildRootFromSlice computes the hash tree root for Go slice values.
//
// Slices in SSZ are hashed as lists, which requires:
//   - Computing the root of the slice contents (as if it were an array)
//   - Mixing the slice length into the final hash for proper domain separation
//
// The function includes optimizations for common types:
//   - Byte slices: Direct appending with chunk padding
//   - Uint64 slices: Efficient 8-byte appending
//   - Nested byte arrays: Special handling for [][]byte patterns
//
// Parameters:
//   - sourceType: The TypeDescriptor containing slice metadata and limits
//   - sourceValue: The reflect.Value of the slice to hash
//   - hh: The Hasher instance for hash computation
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if element hashing fails
//
// For slices with max size hints, MerkleizeWithMixin ensures the length is
// properly mixed into the root, implementing the SSZ list hashing algorithm.

func (d *DynSsz) buildRootFromSlice(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	fieldType := sourceType.ElemDesc

	subIndex := hh.Index()
	sliceLen := sourceValue.Len()
	itemSize := 0

	switch fieldType.Kind {
	case reflect.Struct:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)

			err := d.buildRootFromType(fieldType, fieldValue, hh, idt+2)
			if err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		if fieldType.IsByteArray && (len(fieldType.TypeHints) == 0 || (fieldType.TypeHints[0].Type != SszBitlistType && fieldType.TypeHints[0].Type != SszProgressiveBitlistType)) {
			for i := 0; i < sliceLen; i++ {
				sliceSubIndex := hh.Index()

				fieldValue := sourceValue.Index(i)

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
			for i := 0; i < sliceLen; i++ {
				fieldValue := sourceValue.Index(i)

				err := d.buildRootFromType(fieldType, fieldValue, hh, idt+2)
				if err != nil {
					return err
				}
			}
		}
	case reflect.Uint8:
		hh.Append(sourceValue.Bytes())
		hh.FillUpTo32()
		itemSize = 1
	case reflect.Uint16:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)

			hh.AppendUint16(uint16(fieldValue.Uint()))
		}
		itemSize = 2
	case reflect.Uint32:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)

			hh.AppendUint32(uint32(fieldValue.Uint()))
		}
		itemSize = 4
	case reflect.Uint64:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)

			hh.AppendUint64(uint64(fieldValue.Uint()))
		}
		itemSize = 8
	default:
		// For other types, use the central dispatcher
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)

			err := d.buildRootFromType(fieldType, fieldValue, hh, idt+2)
			if err != nil {
				return err
			}
		}
	}

	if len(sourceType.TypeHints) > 0 && sourceType.TypeHints[0].Type == SszProgressiveListType {
		hh.MerkleizeProgressiveWithMixin(subIndex, uint64(sliceLen))
	} else if len(sourceType.MaxSizeHints) > 0 && !sourceType.MaxSizeHints[0].NoValue {
		var limit uint64
		if itemSize > 0 {
			limit = CalculateLimit(uint64(sourceType.MaxSizeHints[0].Size), uint64(sliceLen), uint64(itemSize))
		} else {
			limit = uint64(sourceType.MaxSizeHints[0].Size)
		}
		hh.MerkleizeWithMixin(subIndex, uint64(sliceLen), limit)
	} else {
		hh.Merkleize(subIndex)
	}

	return nil
}

// getActiveFields returns the active fields for a progressive container.
// Per the specification: Given a value of type ProgressiveContainer(active_fields)
// return value.__class__.active_fields.
//
// The active fields are determined by the ssz-index tags in the struct definition:
// - The highest ssz-index determines the size of the bitlist
// - Each field with an ssz-index has its corresponding bit set to 1
// - All other bits are set to 0
//
// Parameters:
//   - sourceType: The TypeDescriptor containing progressive container metadata
//
// Returns:
//   - []byte: The active fields bitlist as bytes (≤256 bits, so max 32 bytes)
func (d *DynSsz) getActiveFields(sourceType *TypeDescriptor) []byte {
	// Find the highest ssz-index to determine bitlist size
	maxIndex := uint16(0)
	for _, field := range sourceType.Fields {
		if field.SszIndex > maxIndex {
			maxIndex = field.SszIndex
		}
	}

	// Create bitlist with enough bytes to hold maxIndex+1 bits
	bytesNeeded := (int(maxIndex) + 8) / 8 // +7 for rounding up, +1 already included in maxIndex
	activeFields := make([]byte, bytesNeeded)

	// Set most significant bit for length bit.
	i := uint8(1 << (maxIndex % 8))
	activeFields[maxIndex/8] |= i

	// Set bit for each field that has an ssz-index
	for _, field := range sourceType.Fields {
		byteIndex := field.SszIndex / 8
		bitIndex := field.SszIndex % 8
		if int(byteIndex) < len(activeFields) {
			activeFields[byteIndex] |= (1 << bitIndex)
		}
	}

	return activeFields
}
