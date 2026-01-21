// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "fmt"

var (
	ErrListTooBig           = fmt.Errorf("list length is higher than max value")
	ErrUnexpectedEOF        = fmt.Errorf("unexpected end of SSZ")
	ErrOffset               = fmt.Errorf("incorrect offset")
	ErrInvalidValueRange    = fmt.Errorf("invalid value range")
	ErrInvalidUnionVariant  = fmt.Errorf("invalid union variant")
	ErrVectorLength         = fmt.Errorf("incorrect vector length")
	ErrNotImplemented       = fmt.Errorf("not implemented")
	ErrBitlistNotTerminated = fmt.Errorf("bitlist misses mandatory termination bit")

	// ErrNoCodeForView is returned by DynamicView* methods when no generated code exists
	// for the requested view. This signals that reflection-based processing should be used.
	ErrNoCodeForView = fmt.Errorf("no generated code for this view")
)
