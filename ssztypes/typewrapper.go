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
	SizeHints    []SszSizeHint
	MaxSizeHints []SszMaxSizeHint
	TypeHints    []SszTypeHint
}

// ExtractWrapperDescriptorInfo extracts wrapper information from a wrapper descriptor type.
// This function validates that the descriptor has exactly one field and extracts its annotations.
func extractWrapperDescriptorInfo(descriptorType reflect.Type, ds sszutils.DynamicSpecs) (*wrapperDescriptorInfo, error) {
	if descriptorType.Kind() != reflect.Struct {
		return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "wrapper descriptor must be a struct, got %v", descriptorType.Kind())
	}

	if descriptorType.NumField() != 1 {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "wrapper descriptor must have exactly 1 field, got %d", descriptorType.NumField())
	}

	field := descriptorType.Field(0)

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
		SizeHints:    sizeHints,
		MaxSizeHints: maxSizeHints,
		TypeHints:    typeHints,
	}, nil
}
