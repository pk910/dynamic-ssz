package harness

import (
	"math/big"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Extended types: dynamic-ssz encodings outside the SSZ specification
// (formal/lean/SszCert/Extended.lean). No Python fixtures exist for them;
// the generative link covers them.

// ExtInts holds every signed width (two's complement, little-endian).
type ExtInts struct {
	A int8
	B int16
	C int32
	D int64
}

// ExtFloats holds both IEEE-754 widths (raw bit patterns).
type ExtFloats struct {
	F float32
	G float64
}

// ExtInt64List packs signed elements like uint64.
type ExtInt64List []int64

var _ = sszutils.Annotate[ExtInt64List](`ssz-max:"300"`)

// ExtFloat32Vector packs float bit patterns.
type ExtFloat32Vector [9]float32

// ExtBig holds bounded and unbounded big integers.
type ExtBig struct {
	Bounded   *big.Int `ssz-max:"33"`
	Unbounded *big.Int
	Tag       uint16
}

// ExtInner is a variable-size container used as an optional/union payload.
type ExtInner struct {
	X uint32
	Y []uint16 `ssz-max:"20"`
}

// ExtOptionals covers optional basic, fixed and variable payloads.
type ExtOptionals struct {
	A *uint64   `ssz-type:"optional"`
	B *[4]byte  `ssz-type:"optional"`
	C *ExtInner `ssz-type:"optional"`
	D uint8
}

// ExtOptionalLists covers pointers encoded as the canonical List[T, 1].
type ExtOptionalLists struct {
	A *uint32   `ssz-type:"optional-list"`
	B *ExtInner `ssz-type:"optional-list"`
	C *[32]byte `ssz-type:"optional-list"`
}

// ExtUnion is a classic union with a None variant.
type ExtUnion = dynssz.Union[struct {
	None  dynssz.None
	Small uint16
	Fixed [4]byte
	Inner ExtInner
}]

// ExtUnionNoNone is a classic union without a None variant.
type ExtUnionNoNone = dynssz.Union[struct {
	Num   uint64
	Bytes []byte `ssz-max:"40"`
}]

// ExtUnionHolder reaches classic unions through offsets and a list.
type ExtUnionHolder struct {
	Head  uint8
	U     ExtUnion
	V     ExtUnionNoNone
	Many  []ExtUnion `ssz-max:"6"`
	Trail uint32
}

// ExtWrapped is a type wrapper around a byte list.
type ExtWrapped = dynssz.TypeWrapper[struct {
	Data []byte `ssz-max:"64"`
}, []byte]

// ExtWrapperHolder carries type-wrapped fields.
type ExtWrapperHolder struct {
	W ExtWrapped
	N uint64
}

// ExtMixed nests extended types inside spec collections.
type ExtMixed struct {
	Ints    ExtInt64List
	Floats  ExtFloat32Vector
	Opts    []ExtOptionals `ssz-max:"5"`
	Big     ExtBig
	Wrapped ExtWrapperHolder
}

var extendedTypes = map[string]Entry{
	"ExtInts":          {New: func() any { return new(ExtInts) }},
	"ExtFloats":        {New: func() any { return new(ExtFloats) }},
	"ExtInt64List":     {New: func() any { return new(ExtInt64List) }},
	"ExtFloat32Vector": {New: func() any { return new(ExtFloat32Vector) }},
	"ExtBig":           {New: func() any { return new(ExtBig) }},
	"ExtInner":         {New: func() any { return new(ExtInner) }},
	"ExtOptionals":     {New: func() any { return new(ExtOptionals) }},
	"ExtOptionalLists": {New: func() any { return new(ExtOptionalLists) }},
	"ExtUnionHolder":   {New: func() any { return new(ExtUnionHolder) }},
	"ExtWrapperHolder": {New: func() any { return new(ExtWrapperHolder) }},
	"ExtMixed":         {New: func() any { return new(ExtMixed) }},
}
