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

	// An ssz-index tag assigns an explicit selector value, e.g. 1-based
	// selectors for EIP-8016 conformant unions. Mixing tagged and untagged
	// variants would silently renumber the untagged ones, so require
	// all-or-none up front.
	indexTagCount := 0
	for i := 0; i < descriptorType.NumField(); i++ {
		field := descriptorType.Field(i)
		if _, ok := field.Tag.Lookup("ssz-index"); ok {
			indexTagCount++
		}
	}
	if indexTagCount > 0 && indexTagCount != descriptorType.NumField() {
		return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "union descriptor mixes fields with and without ssz-index tags (all variants must carry one when any does)")
	}

	// EIP-8016 restricts union selectors to 1..127: 0 and the range above 127
	// are reserved. Default numbering follows field order starting at 1, so the
	// descriptor can hold at most 127 variants.
	if descriptorType.NumField() > 127 {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "union descriptor has %d variants, but selectors are limited to 1..127", descriptorType.NumField())
	}

	variantInfo := make(map[uint8]unionVariantInfo)

	for i := 0; i < descriptorType.NumField(); i++ {
		field := descriptorType.Field(i)
		variantIndex := uint8(i) + 1 // Field order determines the default variant selector, starting at 1

		sszIndex, err := getSszIndexTag(&field)
		if err != nil {
			return nil, err
		}
		if sszIndex != nil {
			if *sszIndex < 1 || *sszIndex > 127 {
				return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "union selector %d for field %s is outside the valid range 1..127", *sszIndex, field.Name)
			}
			variantIndex = uint8(*sszIndex)
		}

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

		if _, exists := variantInfo[variantIndex]; exists {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "duplicate union selector %d (field %s)", variantIndex, field.Name)
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
