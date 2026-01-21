// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"
)

// SszTypeFlag is a flag indicating whether a type has a specific SSZ type feature
type SszTypeFlag uint8

const (
	SszTypeFlagIsDynamic      SszTypeFlag = 1 << iota // Whether the type is a dynamic type (or has nested dynamic types)
	SszTypeFlagHasLimit                               // Whether the type has a max size tag
	SszTypeFlagHasDynamicSize                         // Whether this type or any of its nested types uses dynamic spec size value that differs from the default
	SszTypeFlagHasDynamicMax                          // Whether this type or any of its nested types uses dynamic spec max value that differs from the default
	SszTypeFlagHasSizeExpr                            // Whether this type or any of its nested types uses a dynamic expression to calculate the size or max size
	SszTypeFlagHasMaxExpr                             // Whether this type or any of its nested types uses a dynamic expression to calculate the max size
	SszTypeFlagHasBitSize                             // Whether the type has a bit size tag
)

// SszCompatFlag is a flag indicating whether a type implements a specific SSZ compatibility interface
type SszCompatFlag uint16

const (
	SszCompatFlagFastSSZMarshaler       SszCompatFlag = 1 << iota // Whether the type implements fastssz.Marshaler
	SszCompatFlagFastSSZHasher                                    // Whether the type implements fastssz.HashRoot
	SszCompatFlagHashTreeRootWith                                 // Whether the type implements HashTreeRootWith
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

type GoTypeFlag uint8

const (
	GoTypeFlagIsPointer   GoTypeFlag = 1 << iota // Whether the type is a pointer type
	GoTypeFlagIsByteArray                        // Whether the type is a byte array
	GoTypeFlagIsString                           // Whether the type is a string type
	GoTypeFlagIsTime                             // Whether the type is a time.Time type
)

// TypeDescriptor represents a cached, optimized descriptor for a type's SSZ encoding/decoding
type TypeDescriptor struct {
	Type                   reflect.Type              `json:"-"`                   // Reflect type (runtime type where data lives)
	SchemaType             reflect.Type              `json:"-"`                   // Schema type that defines SSZ layout (may differ from Type for view descriptors)
	CodegenInfo            *any                      `json:"-"`                   // Codegen information
	Kind                   reflect.Kind              `json:"kind"`                // Reflect kind of the type
	Size                   uint32                    `json:"size"`                // SSZ size (-1 if dynamic)
	Len                    uint32                    `json:"len"`                 // Length of array/slice / static size of container
	Limit                  uint64                    `json:"limit"`               // Limit of array/slice (ssz-max tag)
	ContainerDesc          *ContainerDescriptor      `json:"container,omitempty"` // For structs
	UnionVariants          map[uint8]*TypeDescriptor `json:"union,omitempty"`     // Union variant types by index (for CompatibleUnion)
	ElemDesc               *TypeDescriptor           `json:"field,omitempty"`     // For slices/arrays
	HashTreeRootWithMethod *reflect.Method           `json:"-"`                   // Cached HashTreeRootWith method for performance
	SizeExpression         *string                   `json:"size_expr,omitempty"` // The dynamic expression used to calculate the size of the type
	MaxExpression          *string                   `json:"max_expr,omitempty"`  // The dynamic expression used to calculate the max size of the type
	BitSize                uint32                    `json:"bit_size,omitempty"`  // Bit size for bit vector types (ssz-bitsize tag)
	SszType                SszType                   `json:"type"`                // SSZ type of the type
	SszTypeFlags           SszTypeFlag               `json:"flags"`               // SSZ type flags
	SszCompatFlags         SszCompatFlag             `json:"compat"`              // SSZ compatibility flags
	GoTypeFlags            GoTypeFlag                `json:"go_flags"`            // Additional go type flags
}

// FieldDescriptor represents a cached descriptor for a struct field
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
	FieldIndex uint16          `json:"field_index,omitempty"` // Index into the runtime struct's field list
}

// DynFieldDescriptor represents a dynamic field descriptor for a struct
type DynFieldDescriptor struct {
	Field        *FieldDescriptor `json:"field"`
	HeaderOffset uint32           `json:"offset"`
	Index        int16            `json:"index"` // Index of the field in the struct
}
