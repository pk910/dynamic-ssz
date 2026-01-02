// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"reflect"

	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

type ReflectionCtx struct {
	ds        sszutils.DynamicSpecs
	logCb     func(format string, args ...any)
	verbose   bool
	noFastSsz bool
}

func NewReflectionCtx(ds sszutils.DynamicSpecs, logCb func(format string, args ...any), verbose bool, noFastSsz bool) *ReflectionCtx {
	return &ReflectionCtx{
		ds:        ds,
		logCb:     logCb,
		verbose:   verbose,
		noFastSsz: noFastSsz,
	}
}

func getPtr(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v.Addr()
	}

	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)

	return ptr
}

func (ctx *ReflectionCtx) SizeSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value) (uint32, error) {
	return ctx.getSszValueSize(targetType, targetValue)
}

func (ctx *ReflectionCtx) MarshalSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, encoder sszutils.Encoder) error {
	return ctx.marshalType(targetType, targetValue, encoder, 0)
}

func (ctx *ReflectionCtx) UnmarshalSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder) error {
	return ctx.unmarshalType(targetType, targetValue, decoder, 0)
}

func (ctx *ReflectionCtx) HashTreeRoot(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, hh sszutils.HashWalker) error {
	return ctx.buildRootFromType(targetType, targetValue, hh, false, 0)
}
