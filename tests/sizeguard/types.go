// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sizeguard

import (
	"math/big"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// WideContainer sums three 1 GiB vectors: the total fits a 64-bit int and not
// a 32-bit one, where no value of the type can exist. It reaches the container
// static size bound.
type WideContainer struct {
	A []byte `ssz-size:"1073741824"`
	B []byte `ssz-size:"1073741824"`
	C []byte `ssz-size:"1073741824"`
}

// WideVectorElem declares a vector size no 32-bit target can hold, beside a
// dynamic sibling so the declaration also forms a byte offset. It reaches the
// declared vector size bound.
type WideVectorElem struct {
	X []byte `ssz-size:"3000000000"`
	D []byte `ssz-max:"8"`
}

// WideListElem carries the same declaration held in a list, so the element
// count is divided by that size rather than multiplied into it.
type WideListElem struct {
	X []byte `ssz-size:"3000000000"`
}

// WideListHolder reaches the list element size bound.
type WideListHolder struct {
	L []WideListElem `ssz-max:"2"`
}

// GiBElem is a gibibyte, which every target holds.
type GiBElem struct {
	X []byte `ssz-size:"1073741824"`
}

// GiBList holds enough of them that the product passes the SSZ size limit
// while each element alone stays well inside it: the total is what overflows,
// and neither the count nor the width says so.
type GiBList struct {
	L []GiBElem `ssz-max:"8"`
}

// SpecVector takes its vector length from the spec, so a resolved value past
// the limit reaches the size expression bound.
type SpecVector struct {
	V []uint32 `ssz-size:"32" dynssz-size:"VEC32_SIZE"`
}

// SpecProduct takes both dimensions from the spec: their byte product passes
// the limit while neither factor does, which is the size expression product
// bound.
type SpecProduct struct {
	M [][]byte `ssz-size:"1,1" dynssz-size:"OUTER,INNER"`
}

// BigIntMax bounds its payload, which is the bigint limit bound.
type BigIntMax struct {
	B big.Int `ssz-max:"5"`
}

// negSizer refuses to state a size, which every path that needs one must
// report rather than sum into a total.
type negSizer struct{}

func (n *negSizer) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return -1 }

func (n *negSizer) MarshalSSZEncoder(_ sszutils.DynamicSpecs, _ sszutils.Encoder) error {
	return nil
}

func (n *negSizer) UnmarshalSSZDecoder(_ sszutils.DynamicSpecs, _ sszutils.Decoder) error {
	return nil
}

func (n *negSizer) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, _ sszutils.HashWalker) error {
	return nil
}

// NegSizeHolder places that delegate where a size is taken.
type NegSizeHolder struct {
	A uint64
	N negSizer `ssz-type:"custom"`
}
