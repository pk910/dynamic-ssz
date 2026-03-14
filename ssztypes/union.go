// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// unionVariantInfo contains type and annotation information for a union variant
type unionVariantInfo struct {
	Name         string
	Type         reflect.Type
	SizeHints    []SszSizeHint
	MaxSizeHints []SszMaxSizeHint
	TypeHints    []SszTypeHint
}

// extractUnionDescriptorInfo extracts variant information from a union descriptor type.
// This function is used by the type cache to extract variant information including SSZ annotations.
func extractUnionDescriptorInfo(descriptorType reflect.Type, ds sszutils.DynamicSpecs) (map[uint8]unionVariantInfo, error) {
	if descriptorType.Kind() != reflect.Struct {
		return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "union descriptor must be a struct, got %v", descriptorType.Kind())
	}

	variantInfo := make(map[uint8]unionVariantInfo)

	for i := 0; i < descriptorType.NumField(); i++ {
		field := descriptorType.Field(i)
		variantIndex := uint8(i) // Field order determines variant index

		// Extract SSZ annotations using existing DynSsz methods
		sizeHints, err := getSszSizeTag(ds, &field)
		if err != nil {
			return nil, err
		}

		maxSizeHints, err := getSszMaxSizeTag(ds, &field)
		if err != nil {
			return nil, err
		}

		typeHints, err := getSszTypeTag(&field)
		if err != nil {
			return nil, err
		}

		variantInfo[variantIndex] = unionVariantInfo{
			Name:         field.Name,
			Type:         field.Type,
			SizeHints:    sizeHints,
			MaxSizeHints: maxSizeHints,
			TypeHints:    typeHints,
		}
	}

	if len(variantInfo) == 0 {
		return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "union descriptor struct has no fields")
	}

	return variantInfo, nil
}
