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

// unionFieldSelectors returns the selector assigned to each descriptor field
// in field order, validating the descriptor shape: field order starting at 1,
// or the fields' ssz-index tags when present (all-or-none, valid range 1..127,
// no duplicates).
func unionFieldSelectors(descriptorType reflect.Type) ([]uint8, error) {
	if descriptorType == nil {
		return nil, sszutils.NewSszError(sszutils.ErrTypeMismatch, "union descriptor type must not be nil")
	}
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

	selectors := make([]uint8, descriptorType.NumField())
	taken := make(map[uint8]bool, descriptorType.NumField())
	for i := 0; i < descriptorType.NumField(); i++ {
		field := descriptorType.Field(i)
		variantIndex := uint8(i) + 1 // Field order determines the default variant selector, starting at 1

		// A compatible union has no empty option; the None marker only names
		// selector 0 of a classic Union.
		if isNoneMarkerType(field.Type) {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz.None is not a valid compatible union variant (field %s): only the classic Union declares an empty option", field.Name)
		}

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
		if taken[variantIndex] {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "duplicate union selector %d (field %s)", variantIndex, field.Name)
		}
		taken[variantIndex] = true
		selectors[i] = variantIndex
	}

	return selectors, nil
}

// UnionVariantTypes returns the selector-to-variant-type mapping a compatible
// union descriptor struct declares: field order starting at 1, or the fields'
// ssz-index tags when present (all-or-none, valid range 1..127, no
// duplicates). It is the selector authority shared by descriptor extraction
// and the union constructors, so a selector accepted by one is accepted by
// the other.
func UnionVariantTypes(descriptorType reflect.Type) (map[uint8]reflect.Type, error) {
	selectors, err := unionFieldSelectors(descriptorType)
	if err != nil {
		return nil, err
	}

	variantTypes := make(map[uint8]reflect.Type, len(selectors))
	for i, variantIndex := range selectors {
		variantTypes[variantIndex] = descriptorType.Field(i).Type
	}

	return variantTypes, nil
}

// extractUnionDescriptorInfo extracts variant information from a union descriptor type.
// This function is used by the type cache to extract variant information including SSZ annotations.
func extractUnionDescriptorInfo(descriptorType reflect.Type, ds sszutils.DynamicSpecs) (map[uint8]unionVariantInfo, error) {
	selectors, err := unionFieldSelectors(descriptorType)
	if err != nil {
		return nil, err
	}

	variantInfo := make(map[uint8]unionVariantInfo, len(selectors))

	for i, variantIndex := range selectors {
		field := descriptorType.Field(i)

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

// isNoneMarkerType reports whether a descriptor field type is the classic
// union None marker (dynssz.None), detected by identity like the well-known
// generic types.
func isNoneMarkerType(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.NumField() == 0 &&
		t.Name() == "None" && t.PkgPath() == dynsszPkgPath
}

// extractClassicUnionDescriptorInfo extracts variant information from a
// classic union descriptor type. Selectors are the 0-based field positions per
// the SSZ spec: the None marker is only legal as the first field (making
// selector 0 the empty option, with at least one further variant), selectors
// above 127 are reserved, and positional numbering leaves no room for
// ssz-index tags. The returned map holds no entry for the None selector.
func extractClassicUnionDescriptorInfo(descriptorType reflect.Type, ds sszutils.DynamicSpecs) (map[uint8]unionVariantInfo, bool, error) {
	if descriptorType.Kind() != reflect.Struct {
		return nil, false, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "union descriptor must be a struct, got %v", descriptorType.Kind())
	}

	numFields := descriptorType.NumField()
	if numFields == 0 {
		return nil, false, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "union descriptor struct has no fields")
	}
	// Selectors above 127 are reserved, so positions 0..127 bound the count.
	if numFields > 128 {
		return nil, false, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "union descriptor has %d variants, but selectors are limited to 0..127", numFields)
	}

	hasNone := false
	variantInfo := make(map[uint8]unionVariantInfo, numFields)

	for i := 0; i < numFields; i++ {
		field := descriptorType.Field(i)

		if _, tagged := field.Tag.Lookup("ssz-index"); tagged {
			return nil, false, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "union selectors are positional; ssz-index tag on field %s is not allowed", field.Name)
		}

		if isNoneMarkerType(field.Type) {
			if i != 0 {
				return nil, false, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "the None option is only legal as the first union variant (field %s is at position %d)", field.Name, i)
			}
			if numFields < 2 {
				return nil, false, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "a union declaring None must offer at least one further variant")
			}
			hasNone = true
			continue
		}

		sizeHints, err := getSszSizeTag(ds, &field)
		if err != nil {
			return nil, false, err
		}
		maxSizeHints, err := getSszMaxSizeTag(ds, &field)
		if err != nil {
			return nil, false, err
		}
		typeHints, err := getSszTypeTag(&field)
		if err != nil {
			return nil, false, err
		}

		variantInfo[uint8(i)] = unionVariantInfo{
			Name:         field.Name,
			Type:         field.Type,
			SizeHints:    sizeHints,
			MaxSizeHints: maxSizeHints,
			TypeHints:    typeHints,
		}
	}

	return variantInfo, hasNone, nil
}
