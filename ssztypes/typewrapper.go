// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// wrapperDescriptorInfo contains type and annotation information for a wrapper
type wrapperDescriptorInfo struct {
	Type         reflect.Type
	FieldIndex   int // index of the SSZ field within the descriptor struct
	SizeHints    []SszSizeHint
	MaxSizeHints []SszMaxSizeHint
	TypeHints    []SszTypeHint
}

// ExtractWrapperDescriptorInfo extracts wrapper information from a wrapper descriptor type.
// This function validates that the descriptor has exactly one SSZ-eligible field (unexported
// and ssz-excluded fields are ignored) and extracts its annotations.
func extractWrapperDescriptorInfo(descriptorType reflect.Type, ds sszutils.DynamicSpecs) (*wrapperDescriptorInfo, error) {
	if descriptorType.Kind() != reflect.Struct {
		return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "wrapper descriptor must be a struct, got %v", descriptorType.Kind())
	}

	fieldIndex := -1
	for i := 0; i < descriptorType.NumField(); i++ {
		f := descriptorType.Field(i)
		if !f.IsExported() || IsSszExcluded(f.Tag) {
			continue
		}
		if fieldIndex >= 0 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "wrapper descriptor must have exactly 1 SSZ field, got multiple (%s, %s)", descriptorType.Field(fieldIndex).Name, f.Name)
		}
		fieldIndex = i
	}
	if fieldIndex < 0 {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "wrapper descriptor must have exactly 1 SSZ field, got 0")
	}

	field := descriptorType.Field(fieldIndex)

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

	return &wrapperDescriptorInfo{
		Type:         field.Type,
		FieldIndex:   fieldIndex,
		SizeHints:    sizeHints,
		MaxSizeHints: maxSizeHints,
		TypeHints:    typeHints,
	}, nil
}
