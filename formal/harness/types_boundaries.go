package harness

import (
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Go mirror types for tests/fillers/ssz/test_merkleization_boundaries.py

type BoundaryBitVector1 [1]byte

var _ = sszutils.Annotate[BoundaryBitVector1](`ssz-type:"bitvector" ssz-bitsize:"1"`)

type BoundaryBitVector7 [1]byte

var _ = sszutils.Annotate[BoundaryBitVector7](`ssz-type:"bitvector" ssz-bitsize:"7"`)

type BoundaryBitVector9 [2]byte

var _ = sszutils.Annotate[BoundaryBitVector9](`ssz-type:"bitvector" ssz-bitsize:"9"`)

type BoundaryBitVector255 [32]byte

var _ = sszutils.Annotate[BoundaryBitVector255](`ssz-type:"bitvector" ssz-bitsize:"255"`)

type BoundaryBitVector256 [32]byte

var _ = sszutils.Annotate[BoundaryBitVector256](`ssz-type:"bitvector" ssz-bitsize:"256"`)

type BoundaryBitVector257 [33]byte

var _ = sszutils.Annotate[BoundaryBitVector257](`ssz-type:"bitvector" ssz-bitsize:"257"`)

type BoundaryBitList256 []byte

var _ = sszutils.Annotate[BoundaryBitList256](`ssz-type:"bitlist" ssz-max:"256"`)

type BoundaryUint64List32 []uint64

var _ = sszutils.Annotate[BoundaryUint64List32](`ssz-max:"32"`)

type BoundaryBool bool

var boundaryTypes = map[string]Entry{
	"BoundaryBitVector1":   {New: func() any { return new(BoundaryBitVector1) }},
	"BoundaryBitVector7":   {New: func() any { return new(BoundaryBitVector7) }},
	"BoundaryBitVector9":   {New: func() any { return new(BoundaryBitVector9) }},
	"BoundaryBitVector255": {New: func() any { return new(BoundaryBitVector255) }},
	"BoundaryBitVector256": {New: func() any { return new(BoundaryBitVector256) }},
	"BoundaryBitVector257": {New: func() any { return new(BoundaryBitVector257) }},
	"BoundaryBitList256":   {New: func() any { return new(BoundaryBitList256) }},
	"BoundaryUint64List32": {New: func() any { return new(BoundaryUint64List32) }},
	"Boolean":              {New: func() any { return new(BoundaryBool) }},
	"Uint64":               {New: func() any { return new(BasicUint64) }},
}
