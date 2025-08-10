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
//   - pack: Whether to pack the value into a single tree leaf
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

func (d *DynSsz) buildRootFromType(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, pack bool, idt int) error {
	hashIndex := hh.Index()

	if sourceType.IsPtr {
		if sourceValue.IsNil() {
			sourceValue = reflect.New(sourceType.Type.Elem()).Elem()
		} else {
			sourceValue = sourceValue.Elem()
		}
	}

	useFastSsz := !d.NoFastSsz && sourceType.HasFastSSZHasher && !sourceType.HasDynamicSize && !sourceType.HasDynamicMax
	if !useFastSsz && sourceType.SszType == SszCustomType {
		useFastSsz = true
	}

	if d.Verbose {
		fmt.Printf("%stype: %s\t kind: %v\t fastssz: %v (compat: %v/ dynamic: %v/%v)\t index: %v\n", strings.Repeat(" ", idt), sourceType.Type.Name(), sourceType.Kind, useFastSsz, sourceType.HasFastSSZHasher, sourceType.HasDynamicSize, sourceType.HasDynamicMax, hashIndex)
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
		// Route to appropriate handler based on type
		switch sourceType.SszType {
		case SszContainerType, SszProgressiveContainerType:
			err := d.buildRootFromContainer(sourceType, sourceValue, hh, idt)
			if err != nil {
				return err
			}
		case SszVectorType, SszBitvectorType:
			err := d.buildRootFromVector(sourceType, sourceValue, hh, idt)
			if err != nil {
				return err
			}
		case SszListType, SszProgressiveListType:
			err := d.buildRootFromList(sourceType, sourceValue, hh, idt)
			if err != nil {
				return err
			}
		case SszBitlistType:
			maxSize := uint64(0)
			bytes := sourceValue.Bytes()
			if sourceType.HasLimit {
				maxSize = sourceType.Limit
			} else {
				maxSize = uint64(len(bytes) * 8)
			}

			hh.PutBitlist(bytes, maxSize)
		case SszProgressiveBitlistType:
			bytes := sourceValue.Bytes()
			hh.PutProgressiveBitlist(bytes)
		case SszCompatibleUnionType:
			err := d.buildRootFromCompatibleUnion(sourceType, sourceValue, hh, idt)
			if err != nil {
				return err
			}

		case SszBoolType:
			if pack {
				hh.AppendUint8(1)
			} else {
				hh.PutBool(sourceValue.Bool())
			}
		case SszUint8Type:
			if pack {
				hh.AppendUint8(uint8(sourceValue.Uint()))
			} else {
				hh.PutUint8(uint8(sourceValue.Uint()))
			}
		case SszUint16Type:
			if pack {
				hh.AppendUint16(uint16(sourceValue.Uint()))
			} else {
				hh.PutUint16(uint16(sourceValue.Uint()))
			}
		case SszUint32Type:
			if pack {
				hh.AppendUint32(uint32(sourceValue.Uint()))
			} else {
				hh.PutUint32(uint32(sourceValue.Uint()))
			}
		case SszUint64Type:
			if pack {
				hh.AppendUint64(uint64(sourceValue.Uint()))
			} else {
				hh.PutUint64(uint64(sourceValue.Uint()))
			}
		case SszUint128Type, SszUint256Type:
			isUint64 := sourceType.ElemDesc.Kind == reflect.Uint64
			if isUint64 {
				for i := 0; i < int(sourceType.Size/8); i++ {
					hh.AppendUint64(sourceValue.Index(i).Uint())
				}
			} else {
				hh.Append(sourceValue.Bytes())
			}
			if !pack {
				hh.FillUpTo32()
			}
		default:
			return fmt.Errorf("unknown type: %v", sourceType)
		}
	}

	if d.Verbose {
		fmt.Printf("%shash: 0x%x\n", strings.Repeat(" ", idt), hh.Hash())
	}

	return nil
}

// buildRootFromContainer computes the hash tree root for ssz containers.
//
// In SSZ, containers are hashed as follows:
//   - Each field is hashed independently to produce a 32-byte root
//   - All field roots are collected in order
//   - The collection is Merkleized to produce the container's root
//
// For progressive containers, the merkleization is done using the progressive
// algorithm with active fields mixing.
//
// The function uses the pre-computed TypeDescriptor to efficiently iterate through
// fields without repeated reflection calls.
//
// Parameters:
//   - sourceType: The TypeDescriptor containing container field metadata
//   - sourceValue: The reflect.Value of the container to hash
//   - hh: The Hasher instance for hash computation
//   - idt: Indentation level for verbose logging
//
// Returns:
//   - error: An error if any field hashing fails
//
// The Merkleize call at the end combines all field hashes into the final root
// using binary tree hashing with zero-padding to the next power of two.

func (d *DynSsz) buildRootFromContainer(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	hashIndex := hh.Index()

	for i := 0; i < len(sourceType.ContainerDesc.Fields); i++ {
		field := sourceType.ContainerDesc.Fields[i]
		fieldType := field.Type
		fieldValue := sourceValue.Field(i)

		if d.Verbose {
			fmt.Printf("%sfield %v\n", strings.Repeat(" ", idt), field.Name)
		}

		err := d.buildRootFromType(fieldType, fieldValue, hh, false, idt+2)
		if err != nil {
			return err
		}
	}

	// Use progressive merkleization for progressive containers
	if sourceType.SszType == SszProgressiveContainerType {
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
	err := d.buildRootFromType(variantDesc, dataField.Elem(), hh, false, idt+2)
	if err != nil {
		return fmt.Errorf("failed to hash union variant %d: %w", variant, err)
	}

	return nil
}

// buildRootFromVector computes the hash tree root for ssz vectors.
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

func (d *DynSsz) buildRootFromVector(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	hashIndex := hh.Index()

	sliceLen := sourceValue.Len()
	if uint32(sliceLen) > sourceType.Len {
		return ErrListTooBig
	}

	appendZero := 0
	if uint32(sliceLen) < sourceType.Len {
		appendZero = int(sourceType.Len) - sliceLen
	}

	// For byte arrays, handle as a single unit
	if sourceType.IsByteArray {
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(sourceType.Type)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}

		var bytes []byte
		if sourceType.IsString {
			bytes = []byte(sourceValue.String())
		} else {
			bytes = sourceValue.Bytes()
		}

		if appendZero > 0 {
			zeroBytes := make([]byte, appendZero)
			bytes = append(bytes, zeroBytes...)
		}

		hh.AppendBytes32(bytes)
	} else {
		// For other types, process each element
		arrayLen := sourceValue.Len()
		for i := 0; i < arrayLen; i++ {
			fieldValue := sourceValue.Index(i)

			err := d.buildRootFromType(sourceType.ElemDesc, fieldValue, hh, true, idt+2)
			if err != nil {
				return err
			}
		}

		if appendZero > 0 {
			var zeroVal reflect.Value
			if sourceType.ElemDesc.IsPtr {
				zeroVal = reflect.New(sourceType.ElemDesc.Type.Elem())
			} else {
				zeroVal = reflect.New(sourceType.ElemDesc.Type).Elem()
			}

			index := hh.Index()
			err := d.buildRootFromType(sourceType.ElemDesc, zeroVal, hh, true, idt+2)
			if err != nil {
				return err
			}

			zeroLen := hh.Index() - index
			zeroBytes := hh.Hash()
			if len(zeroBytes) > zeroLen {
				zeroBytes = zeroBytes[len(zeroBytes)-zeroLen:]
			}

			for i := 1; i < appendZero; i++ {
				hh.Append(zeroBytes)
			}
		}

		hh.FillUpTo32()
	}

	hh.Merkleize(hashIndex)

	return nil
}

// buildRootFromList computes the hash tree root for ssz lists.
//
// Lists in SSZ are hashed as follows:
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

func (d *DynSsz) buildRootFromList(sourceType *TypeDescriptor, sourceValue reflect.Value, hh *Hasher, idt int) error {
	hashIndex := hh.Index()

	sliceLen := sourceValue.Len()

	// For byte arrays, handle as a single unit
	if sourceType.IsByteArray {
		if !sourceValue.CanAddr() {
			// workaround for unaddressable static arrays
			sourceValPtr := reflect.New(sourceType.Type)
			sourceValPtr.Elem().Set(sourceValue)
			sourceValue = sourceValPtr.Elem()
		}

		var bytes []byte
		if sourceType.IsString {
			bytes = []byte(sourceValue.String())
		} else {
			bytes = sourceValue.Bytes()
		}

		hh.AppendBytes32(bytes)
	} else {
		// For other types, process each element
		arrayLen := sourceValue.Len()
		for i := 0; i < arrayLen; i++ {
			fieldValue := sourceValue.Index(i)

			err := d.buildRootFromType(sourceType.ElemDesc, fieldValue, hh, true, idt+2)
			if err != nil {
				return err
			}
		}

		hh.FillUpTo32()
	}

	if sourceType.SszType == SszProgressiveListType {
		hh.MerkleizeProgressiveWithMixin(hashIndex, uint64(sliceLen))
	} else if sourceType.HasLimit {
		var limit, itemSize uint64

		switch sourceType.ElemDesc.SszType {
		case SszBoolType:
			itemSize = 1
		case SszUint8Type:
			itemSize = 1
		case SszUint16Type:
			itemSize = 2
		case SszUint32Type:
			itemSize = 4
		case SszUint64Type:
			itemSize = 8
		case SszUint128Type:
			itemSize = 16
		case SszUint256Type:
			itemSize = 32
		default:
			itemSize = 0
		}

		if itemSize > 0 {
			limit = CalculateLimit(sourceType.Limit, uint64(sliceLen), uint64(itemSize))
		} else {
			limit = sourceType.Limit
		}
		hh.MerkleizeWithMixin(hashIndex, uint64(sliceLen), limit)
	} else {
		hh.Merkleize(hashIndex)
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
	for _, field := range sourceType.ContainerDesc.Fields {
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
	for _, field := range sourceType.ContainerDesc.Fields {
		byteIndex := field.SszIndex / 8
		bitIndex := field.SszIndex % 8
		if int(byteIndex) < len(activeFields) {
			activeFields[byteIndex] |= (1 << bitIndex)
		}
	}

	return activeFields
}
