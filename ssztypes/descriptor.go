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
	"reflect"
)

// SszTypeFlag is a flag indicating whether a type has a specific SSZ type feature
type SszTypeFlag uint16

const (
	SszTypeFlagIsDynamic       SszTypeFlag = 1 << iota // Whether the type is a dynamic type (or has nested dynamic types)
	SszTypeFlagHasLimit                                // Whether the type has a max size tag
	SszTypeFlagHasDynamicSize                          // Whether this type or any of its nested types uses dynamic spec size value that differs from the default
	SszTypeFlagHasDynamicMax                           // Whether this type or any of its nested types uses dynamic spec max value that differs from the default
	SszTypeFlagHasSizeExpr                             // Whether this type or any of its nested types uses a dynamic expression to calculate the size or max size
	SszTypeFlagHasMaxExpr                              // Whether this type or any of its nested types uses a dynamic expression to calculate the max size
	SszTypeFlagHasBitSize                              // Whether the type has a bit size tag
	SszTypeFlagHasNoneVariant                          // Whether a classic union declares the None option at selector 0
	SszTypeFlagNoSszRoot                               // Whether the type is a list or bitlist without a limit, which has no SSZ hash tree root
	SszTypeFlagRecursionMember                         // Whether the type lies on a recursive cycle and counts as a level against the nesting bound
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
