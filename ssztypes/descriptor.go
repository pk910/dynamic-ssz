// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package ssztypes provides SSZ type descriptors, caching, and struct tag
// parsing for the dynamic-ssz library.
//
// It defines the core type system used to describe how Go types map to SSZ
// encoding: containers, lists, vectors, bitlists, bitvectors, unions, and
// all basic SSZ types. Type descriptors are computed from Go reflect types
// and SSZ struct tags (ssz-size, ssz-max, dynssz-size, dynssz-max, etc.)
// and cached in a TypeCache for reuse.
package ssztypes

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"slices"
	"unsafe"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// SszTypeFlag is a flag indicating whether a type has a specific SSZ type feature
type SszTypeFlag uint16

const (
	SszTypeFlagIsDynamic       SszTypeFlag = 1 << iota // Whether the type is a dynamic type (or has nested dynamic types)
	SszTypeFlagHasLimit                                // Whether the type has a max size tag
	SszTypeFlagHasDynamicSize                          // Whether this type or any of its nested types uses dynamic spec size value that differs from the default
	SszTypeFlagHasDynamicMax                           // Whether this type or any of its nested types uses dynamic spec max value that differs from the default
	SszTypeFlagHasSizeExpr                             // Whether this type or any of its nested types uses a dynamic expression to calculate the size
	SszTypeFlagHasMaxExpr                              // Whether this type or any of its nested types uses a dynamic expression to calculate the max size
	SszTypeFlagHasBitSize                              // Whether the type has a bit size tag
	SszTypeFlagHasNoneVariant                          // Whether a classic union declares the None option at selector 0
	SszTypeFlagNoSszRoot                               // Whether the type is a list or bitlist without a limit, which has no SSZ hash tree root
	SszTypeFlagRecursionMember                         // Whether the type lies on a recursive cycle and counts as a level against the nesting bound
	SszTypeFlagSizerWidth                              // Whether this type or any of its nested types has a width read from a custom type's sizer rather than declared; such a width never packs
)

// SszCompatFlag is a flag indicating whether a type implements a specific SSZ compatibility
// interface. Each flag names one method, so a caller asks for the method it is about to
// call rather than for a family the two type front ends could read differently.
type SszCompatFlag uint32

const (
	SszCompatFlagFastsszValueMarshaler  SszCompatFlag = 1 << iota // Whether the type implements FastsszValueMarshaler
	SszCompatFlagFastsszBufferMarshaler                           // Whether the type implements FastsszBufferMarshaler
	SszCompatFlagFastsszSizer                                     // Whether the type implements FastsszSizer
	SszCompatFlagFastsszUnmarshaler                               // Whether the type implements FastsszUnmarshaler
	SszCompatFlagFastsszHashRoot                                  // Whether the type implements FastsszHashRoot
	SszCompatFlagFastsszHashRootWith                              // Whether the type implements FastsszHashRootWith, or its equivalent over another walker interface
	SszCompatFlagDynamicMarshaler                                 // Whether the type implements DynamicMarshaler
	SszCompatFlagDynamicUnmarshaler                               // Whether the type implements DynamicUnmarshaler
	SszCompatFlagDynamicSizer                                     // Whether the type implements DynamicSizer
	SszCompatFlagDynamicHashRoot                                  // Whether the type implements DynamicHashRoot
	SszCompatFlagDynamicEncoder                                   // Whether the type implements DynamicEncoder
	SszCompatFlagDynamicDecoder                                   // Whether the type implements DynamicDecoder
	SszCompatFlagDynamicViewMarshaler                             // Whether the type implements DynamicViewMarshaler
	SszCompatFlagDynamicViewUnmarshaler                           // Whether the type implements DynamicViewUnmarshaler
	SszCompatFlagDynamicViewSizer                                 // Whether the type implements DynamicViewSizer
	SszCompatFlagDynamicViewHashRoot                              // Whether the type implements DynamicViewHashRoot
	SszCompatFlagDynamicViewEncoder                               // Whether the type implements DynamicViewEncoder
	SszCompatFlagDynamicViewDecoder                               // Whether the type implements DynamicViewDecoder
)

// SszCompatFlagFastsszSurface is every fastssz-style method that answers for a whole
// value. A type serving every SSZ operation without its spec-aware methods provides
// all of them; the marshal half is satisfied by either marshal method.
const SszCompatFlagFastsszSurface = SszCompatFlagFastsszValueMarshaler | SszCompatFlagFastsszBufferMarshaler |
	SszCompatFlagFastsszSizer | SszCompatFlagFastsszUnmarshaler

// SszCompatFlagDynamicMethods covers the spec-aware methods a type can carry
// for its own value. A custom type that carries any of them takes the spec
// set, and is treated as depending on it.
const SszCompatFlagDynamicMethods = SszCompatFlagDynamicMarshaler | SszCompatFlagDynamicUnmarshaler |
	SszCompatFlagDynamicSizer | SszCompatFlagDynamicHashRoot | SszCompatFlagDynamicEncoder | SszCompatFlagDynamicDecoder

// GoTypeFlag is a bitmask indicating Go-specific type properties that affect
// SSZ encoding behavior.
type GoTypeFlag uint8

const (
	GoTypeFlagIsPointer   GoTypeFlag = 1 << iota // Whether the type is a pointer type
	GoTypeFlagIsByteArray                        // Whether the type is a byte array
	GoTypeFlagIsString                           // Whether the type is a string type
	GoTypeFlagIsTime                             // Whether the type is a time.Time type
	GoTypeFlagIsView                             // Whether the type uses a view descriptor
)

// TypeDescriptor represents a cached, optimized descriptor for a type's SSZ encoding/decoding
//
// Field order is load-bearing: fields are grouped by how often the reflection
// engine reads them. The first four are read for every value, the next six for
// every composite; on amd64 the ten fill the first cache line. The rest are
// read only for the kind that carries them, or not at all.
//
// GetTypeHash marshals this struct as JSON in declaration order, so reordering
// moves every type hash and every generated "// Hash:" header.
// TestTypeDescriptorHotFieldsShareACacheLine pins the grouping.
type TypeDescriptor struct {
	// Read for every walked value.
	SszCompatFlags SszCompatFlag        `json:"compat"`              // SSZ compatibility flags, one per delegate method
	SszTypeFlags   SszTypeFlag          `json:"flags"`               // SSZ type flags
	SszType        SszType              `json:"type"`                // SSZ type of the type
	GoTypeFlags    GoTypeFlag           `json:"go_flags"`            // Additional go type flags
	ElemDesc       *TypeDescriptor      `json:"field,omitempty"`     // For slices/arrays
	ContainerDesc  *ContainerDescriptor `json:"container,omitempty"` // For structs
	Size           int64                `json:"size"`                // Serialized size of the fixed part; a dynamic type carries SszTypeFlagIsDynamic and sizes its tail at runtime
	Len            int64                `json:"len"`                 // Length of array/slice / static size of container
	Limit          uint64               `json:"limit"`               // Limit of array/slice (ssz-max tag)
	Type           reflect.Type         `json:"-"`                   // Reflect type (runtime type where data lives)

	// Read only for the SSZ kinds that carry them.
	Kind                   reflect.Kind              `json:"kind"`                    // Reflect kind of the type
	WrapperFieldIndex      uint32                    `json:"wrapper_field,omitempty"` // Index of the wrapped value field in a wrapper struct (excluded fields may precede it)
	HashAlgorithm          string                    `json:"hash,omitempty"`          // Name of the hash algorithm the type declares for its own tree (ssz-hash annotation)
	MinSize                int64                     `json:"min_size,omitempty"`      // Smallest serialization of this type; 0 when it has no floor (see SetMinSize)
	BitSize                int64                     `json:"bit_size,omitempty"`      // Bit size for bit vector types (ssz-bitsize tag)
	UnionVariants          map[uint8]*TypeDescriptor `json:"union,omitempty"`         // Union variant types by index (for CompatibleUnion)
	CodegenInfo            *any                      `json:"-"`                       // Codegen information or view pointer
	HashTreeRootWithMethod *reflect.Method           `json:"-"`                       // Cached HashTreeRootWith method for performance
	SizeExpression         *string                   `json:"size_expr,omitempty"`     // The dynamic expression used to calculate the size of the type
	MaxExpression          *string                   `json:"max_expr,omitempty"`      // The dynamic expression used to calculate the max size of the type
	SchemaType             reflect.Type              `json:"-"`                       // Schema type that defines SSZ layout (may differ from Type for view descriptors)
}

// ContainerDescriptor holds the field descriptors of a struct.
type ContainerDescriptor struct {
	Fields    []FieldDescriptor    `json:"fields"`     // For structs
	DynFields []DynFieldDescriptor `json:"dyn_fields"` // Dynamic struct fields
}

// FieldDescriptor represents a cached descriptor for a struct field.
// When using view descriptors (schema type differs from runtime type), the
// FieldIndex points to the corresponding field in the runtime struct, while
// Name and Type come from the schema struct's field definition.
type FieldDescriptor struct {
	Name       string          `json:"name"`                  // Name of the field (from schema struct)
	Type       *TypeDescriptor `json:"type"`                  // Type descriptor (built from runtime/schema pair)
	SszIndex   uint16          `json:"index,omitempty"`       // SSZ index for progressive containers
	FieldIndex uint32          `json:"field_index,omitempty"` // Index into the runtime struct's field list
}

// DynFieldDescriptor represents a dynamic field descriptor for a struct
type DynFieldDescriptor struct {
	Field        *FieldDescriptor `json:"field"`
	HeaderOffset int64            `json:"offset"`
	Index        int32            `json:"index"` // Index of the field in the struct
}

// RootIsChunk reports whether the hash tree root of the type is its
// serialization padded to one chunk, so that no hash is computed for it: a
// basic value, or a vector of basic values or a bitvector of up to 32 bytes.
// The root of such a type is the same under every hash algorithm.
func (td *TypeDescriptor) RootIsChunk() bool {
	for td.SszType == SszTypeWrapperType && td.ElemDesc != nil {
		td = td.ElemDesc
	}
	if td.SszTypeFlags&(SszTypeFlagIsDynamic|SszTypeFlagHasSizeExpr) != 0 {
		return false
	}
	switch td.SszType {
	case SszBitvectorType:
		return td.Size <= 32
	case SszVectorType:
		return td.Size <= 32 && td.ElemDesc != nil && td.ElemDesc.SszType.IsBasic()
	default:
		return td.SszType.IsBasic()
	}
}

// SetMinSize records the smallest number of bytes a value of this type can
// serialize to. Decoders use it to reject an offset table that declares more
// elements than the region can hold, before that count sizes an allocation.
//
// A type with no floor -- a list, an optional list -- keeps 0, which states
// no bound rather than a wrong one. A bit list holds its termination bit, a
// union its selector, an optional its presence byte and a big.Int its sign
// byte, so each holds at least one byte.
//
// It is stored rather than derived at decode time because Len carries different
// meanings per type (bytes for a container's fixed section, elements for a
// vector), so the rule would otherwise be re-derived on every decode. Children
// are read from their own cached value, so a recursive type resolves without
// walking back into itself: a cycle's back edge simply contributes nothing.
//
// Both front ends record it, so the descriptor hashes agree. The reflection
// cache resolves every size against the active spec; the code generator's
// go/types parser records the static tag values and emits the bound as a
// runtime expression instead (see minSizeExpr), since a preset may resolve
// them smaller than the static defaults.
func (td *TypeDescriptor) SetMinSize() {
	// A static type serializes to exactly its size.
	if td.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
		td.MinSize = td.Size
		return
	}

	switch td.SszType {
	case SszTypeWrapperType:
		if td.ElemDesc != nil {
			td.MinSize = td.ElemDesc.MinSize
		}
	case SszContainerType, SszProgressiveContainerType:
		// The fixed section: every field's own size, and four offset bytes for
		// each dynamic one.
		td.MinSize = td.Len
	case SszBitlistType, SszProgressiveBitlistType, SszUnionType, SszCompatibleUnionType, SszOptionalType, SszBigIntType:
		// The termination bit's byte, the selector byte, the presence byte or
		// the sign byte.
		td.MinSize = 1
	case SszVectorType:
		// A vector of dynamic elements leads with one 4-byte offset per element,
		// and every element costs at least its own minimum on top of that. Len is
		// the element count here, not a byte size.
		if td.ElemDesc != nil {
			// An overflowing product would bound the region above the true floor
			// and refuse valid input, so it states no bound instead. The product
			// is formed in two words: checking it after it has wrapped would
			// accept the wrapped value as a floor.
			hi, lo := bits.Mul64(uint64(td.Len), 4+uint64(td.ElemDesc.MinSize))
			if hi == 0 && lo <= math.MaxInt64 {
				td.MinSize = int64(lo)
			}
		}
	default:
		// Everything else can serialize to nothing -- an empty list, an absent
		// optional list -- so it states no floor.
	}
}

// NoSszRootError reports that a limit-less list or bitlist cannot be hashed.
// Both engines raise it where the root would be produced: the reflection engine
// at hash time, the code generator while emitting the hash method.
func NoSszRootError(desc *TypeDescriptor) error {
	kind := sszTypeNameList
	if desc.SszType == SszBitlistType {
		kind = sszTypeNameBitlist
	}

	return sszutils.NewSszErrorf(sszutils.ErrExtendedTypeDisabled,
		"%s has no ssz-max, so it has no SSZ hash tree root: add a limit, or enable extended types to hash it as an unbounded %s. If this is a view's data type, hash it through its view instead",
		kind, kind)
}

// GetTypeHash computes a SHA-256 hash of the TypeDescriptor's JSON
// representation. This hash uniquely identifies the type's SSZ layout and is
// used by the code generator to detect when a type's structure has changed and
// regeneration is needed.
//
// Recursive types form a cyclic descriptor graph that standard JSON
// marshalling cannot represent; those fall back to a deterministic
// reference-based serialization so distinct recursive layouts hash to
// distinct values. Acyclic descriptors keep the plain JSON form.
//
// The hash covers that encoding, not the wire format: encoding/json emits
// fields in declaration order, so reordering TypeDescriptor moves every hash
// without changing any serialization. It identifies a layout within one
// version of this package, not across versions.
func (td *TypeDescriptor) GetTypeHash() [32]byte {
	jsonDesc, err := json.Marshal(td)
	if err != nil {
		jsonDesc = marshalCyclicDescriptor(td)
	}
	hash := sha256.Sum256(jsonDesc)
	return hash
}

// marshalCyclicDescriptor serializes a descriptor graph that contains cycles
// into a deterministic byte representation for hashing. Standard JSON
// marshalling fails on cyclic graphs, so each reachable descriptor is assigned
// an index in a deterministic traversal order and serialized shallowly, with
// child descriptors written as index references. Field metadata that normally
// travels inside the nested JSON (names, indices) is emitted alongside the
// references; dynamic-field layout is fully derived from the fields and flags,
// so it needs no separate representation.
func marshalCyclicDescriptor(root *TypeDescriptor) []byte {
	index := map[*TypeDescriptor]int{}
	order := []*TypeDescriptor{}

	var walk func(desc *TypeDescriptor)
	walk = func(desc *TypeDescriptor) {
		if desc == nil {
			return
		}
		if _, seen := index[desc]; seen {
			return
		}
		index[desc] = len(order)
		order = append(order, desc)

		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				walk(desc.ContainerDesc.Fields[i].Type)
			}
		}
		walk(desc.ElemDesc)
		for _, key := range sortedVariantKeys(desc.UnionVariants) {
			walk(desc.UnionVariants[key])
		}
	}
	walk(root)

	ref := func(desc *TypeDescriptor) int {
		if desc == nil {
			return -1
		}
		return index[desc]
	}

	var buf bytes.Buffer
	for i, desc := range order {
		shallow := *desc
		shallow.ContainerDesc = nil
		shallow.UnionVariants = nil
		shallow.ElemDesc = nil
		nodeJSON, _ := json.Marshal(&shallow)

		fmt.Fprintf(&buf, "#%d:%s", i, nodeJSON)
		if desc.ContainerDesc != nil {
			for fi := range desc.ContainerDesc.Fields {
				field := &desc.ContainerDesc.Fields[fi]
				fmt.Fprintf(&buf, ";f:%s/%d/%d=%d", field.Name, field.SszIndex, field.FieldIndex, ref(field.Type))
			}
		}
		if desc.ElemDesc != nil {
			fmt.Fprintf(&buf, ";e=%d", ref(desc.ElemDesc))
		}
		for _, key := range sortedVariantKeys(desc.UnionVariants) {
			fmt.Fprintf(&buf, ";u:%d=%d", key, ref(desc.UnionVariants[key]))
		}
		buf.WriteByte('\n')
	}

	return buf.Bytes()
}

// sortedVariantKeys returns the union variant keys in ascending order so the
// traversal and serialization are deterministic across map iteration orders.
func sortedVariantKeys(variants map[uint8]*TypeDescriptor) []uint8 {
	if len(variants) == 0 {
		return nil
	}
	keys := make([]uint8, 0, len(variants))
	for key := range variants {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// TypeWord returns the identity of a type: the type pointer in the
// reflect.Type's data word. It is a cheaper map key than the reflect.Type.
func TypeWord(t reflect.Type) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1]
}
