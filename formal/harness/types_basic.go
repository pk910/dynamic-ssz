package harness

import (
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Go mirror types for tests/fillers/ssz/test_basic_types.py

type BasicBool bool
type BasicUint8 uint8
type BasicUint16 uint16
type BasicUint32 uint32
type BasicUint64 uint64

type BasicUint128 [16]byte

var _ = sszutils.Annotate[BasicUint128](`ssz-type:"uint128"`)

type BasicUint256 [32]byte

var _ = sszutils.Annotate[BasicUint256](`ssz-type:"uint256"`)

type BasicBytes4 [4]byte
type BasicBytes32 [32]byte
type BasicBytes52 [52]byte
type BasicBytes64 [64]byte

type BasicByteList512KiB []byte

var _ = sszutils.Annotate[BasicByteList512KiB](`ssz-max:"524288"`)

type BasicBitVector8 [1]byte

var _ = sszutils.Annotate[BasicBitVector8](`ssz-type:"bitvector" ssz-bitsize:"8"`)

type BasicBitVector64 [8]byte

var _ = sszutils.Annotate[BasicBitVector64](`ssz-type:"bitvector" ssz-bitsize:"64"`)

type BasicBitList16 []byte

var _ = sszutils.Annotate[BasicBitList16](`ssz-type:"bitlist" ssz-max:"16"`)

type BasicUint16Vector3 [3]uint16
type BasicUint64Vector4 [4]uint64

type BasicUint32List16 []uint32

var _ = sszutils.Annotate[BasicUint32List16](`ssz-max:"16"`)

type BasicBytes32List8 [][32]byte

var _ = sszutils.Annotate[BasicBytes32List8](`ssz-max:"8"`)

var basicTypes = map[string]Entry{
	"Boolean":             {New: func() any { return new(BasicBool) }},
	"Uint8":               {New: func() any { return new(BasicUint8) }},
	"Uint16":              {New: func() any { return new(BasicUint16) }},
	"Uint32":              {New: func() any { return new(BasicUint32) }},
	"Uint64":              {New: func() any { return new(BasicUint64) }},
	"Uint128":             {New: func() any { return new(BasicUint128) }},
	"Uint256":             {New: func() any { return new(BasicUint256) }},
	"Bytes4":              {New: func() any { return new(BasicBytes4) }},
	"Bytes32":             {New: func() any { return new(BasicBytes32) }},
	"Bytes52":             {New: func() any { return new(BasicBytes52) }},
	"Bytes64":             {New: func() any { return new(BasicBytes64) }},
	"ByteList512KiB":      {New: func() any { return new(BasicByteList512KiB) }},
	"SampleBitVector8":    {New: func() any { return new(BasicBitVector8) }},
	"SampleBitVector64":   {New: func() any { return new(BasicBitVector64) }},
	"SampleBitList16":     {New: func() any { return new(BasicBitList16) }},
	"SampleUint16Vector3": {New: func() any { return new(BasicUint16Vector3) }},
	"SampleUint64Vector4": {New: func() any { return new(BasicUint64Vector4) }},
	"SampleUint32List16":  {New: func() any { return new(BasicUint32List16) }},
	"SampleBytes32List8":  {New: func() any { return new(BasicBytes32List8) }},
}
