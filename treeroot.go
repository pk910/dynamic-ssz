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
		// Special case for bitlists - hack
		if strings.Contains(sourceType.Type.Name(), "Bitlist") {
			maxSize := uint64(0)
			bytes := sourceValue.Bytes()
			if len(sourceType.MaxSizeHints) > 0 {
				maxSize = uint64(sourceType.MaxSizeHints[0].Size)
			} else {
				maxSize = uint64(len(bytes) * 8)
			}

			hh.PutBitlist(bytes, maxSize)
		} else {
			// Route to appropriate handler based on type
			switch sourceType.Kind {
			case reflect.Struct:
				err := d.buildRootFromStruct(sourceType, sourceValue, hh, idt)
				if err != nil {
					return err
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
			case reflect.String:
				err := d.buildRootFromString(sourceType, sourceValue, hh, idt)
				if err != nil {
					return err
				}
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
	hh.Merkleize(hashIndex)

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
		if fieldType.IsByteArray {
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
	case reflect.Uint64:
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)

			hh.AppendUint64(uint64(fieldValue.Uint()))
		}
		itemSize = 8
	case reflect.String:
		// Handle []string to match [][]byte behavior
		// When [][]byte doesn't have nested max size hints, it uses PutBytes
		// So we do the same for []string to ensure they hash identically
		for i := 0; i < sliceLen; i++ {
			fieldValue := sourceValue.Index(i)
			stringBytes := []byte(fieldValue.String())
			hh.PutBytes(stringBytes)
		}
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

	if len(sourceType.MaxSizeHints) > 0 && !sourceType.MaxSizeHints[0].NoValue {
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

// buildRootFromString computes the hash tree root for Go string values.
//
// Strings in SSZ can be either fixed-size or dynamic:
//   - Fixed-size strings (with ssz-size tag): Hash like fixed-size byte arrays with zero padding
//   - Dynamic strings with max size hints: Hash as lists with length mixin
//   - Dynamic strings without hints: Hash as basic byte sequences
//
// Parameters:
//   - sourceType: The TypeDescriptor containing string metadata and size constraints
//   - sourceValue: The reflect.Value of the string to hash
//   - hh: The Hasher instance for hash computation
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if hashing fails
//
// The function converts the string to bytes and then applies the appropriate
// hashing strategy based on the string's size constraints.
func (d *DynSsz) buildRootFromString(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	// Convert string to bytes
	stringBytes := []byte(sourceValue.String())
	
	if sourceType.Size > 0 {
		// Fixed-size string: hash like a fixed-size byte array
		fixedSize := int(sourceType.Size)
		paddedBytes := make([]byte, fixedSize)
		copy(paddedBytes, stringBytes)
		// The rest is already zero-filled
		hh.PutBytes(paddedBytes)
	} else if len(sourceType.MaxSizeHints) > 0 && !sourceType.MaxSizeHints[0].NoValue {
		// Dynamic string with max size hints: hash as a list with length mixin
		subIndex := hh.Index()
		hh.Append(stringBytes)
		hh.FillUpTo32()
		limit := uint64((sourceType.MaxSizeHints[0].Size + 31) / 32)
		hh.MerkleizeWithMixin(subIndex, uint64(len(stringBytes)), limit)
	} else {
		// Dynamic string without hints: hash as basic type
		hh.PutBytes(stringBytes)
	}
	
	return nil
}
