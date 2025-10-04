// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "fmt"

var (
	ErrListTooBig          = fmt.Errorf("list length is higher than max value")
	ErrUnexpectedEOF       = fmt.Errorf("unexpected end of SSZ")
	ErrOffset              = fmt.Errorf("incorrect offset")
	ErrInvalidUnionVariant = fmt.Errorf("invalid union variant")
	ErrVectorLength        = fmt.Errorf("incorrect vector length")
	ErrNotImplemented      = fmt.Errorf("not implemented")
)
