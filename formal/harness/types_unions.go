package harness

import (
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Go mirror types for tests/fillers/ssz/test_compatible_unions.py

// ACTIVE_FIELDS = (1, 0, 1)
type CUSquare struct {
	Side  uint16 `ssz-index:"0" json:"side"`
	Color uint8  `ssz-index:"2" json:"color"`
}

// ACTIVE_FIELDS = (0, 1, 1)
type CUCircle struct {
	Radius uint16 `ssz-index:"1" json:"radius"`
	Color  uint8  `ssz-index:"2" json:"color"`
}

// List[Uint16, 4]
type CUUint16List4 []uint16

var _ = sszutils.Annotate[CUUint16List4](`ssz-max:"4"`)

type CUSquareProgList []CUSquare

var _ = sszutils.Annotate[CUSquareProgList](`ssz-type:"progressive-list"`)

type CUCircleProgList []CUCircle

var _ = sszutils.Annotate[CUCircleProgList](`ssz-type:"progressive-list"`)

// OPTIONS = {1: SampleSquare, 2: SampleCircle, 127: SampleSquare}
type CUShape = dynssz.CompatibleUnion[struct {
	V1   CUSquare `ssz-index:"1"`
	V2   CUCircle `ssz-index:"2"`
	V127 CUSquare `ssz-index:"127"`
}]

// OPTIONS = {1: SampleUint16List4, 2: SampleUint16List4Alias}
type CUNumbers = dynssz.CompatibleUnion[struct {
	V1 CUUint16List4 `ssz-index:"1"`
	V2 CUUint16List4 `ssz-index:"2"`
}]

// OPTIONS = {1: SampleSquareProgressiveList, 2: SampleCircleProgressiveList}
type CUEmptyProne = dynssz.CompatibleUnion[struct {
	V1 CUSquareProgList `ssz-index:"1"`
	V2 CUCircleProgList `ssz-index:"2"`
}]

// OPTIONS = {5: SampleSquare}
type CUSquareOnly = dynssz.CompatibleUnion[struct {
	V5 CUSquare `ssz-index:"5"`
}]

// OPTIONS = {1: SampleShape, 2: SampleSquareOnly}
type CUNestedShape = dynssz.CompatibleUnion[struct {
	V1 CUShape      `ssz-index:"1"`
	V2 CUSquareOnly `ssz-index:"2"`
}]

// Ordinary container reaching a union through an offset.
type CUShapeContainer struct {
	Tag  uint64  `json:"tag"`
	Body CUShape `json:"body"`
}

// Progressive container reaching a union, ACTIVE_FIELDS = (1, 0, 1)
type CUShapeProgressiveContainer struct {
	Tag  uint64  `ssz-index:"0" json:"tag"`
	Body CUShape `ssz-index:"2" json:"body"`
}

type CUShapeProgList []CUShape

var _ = sszutils.Annotate[CUShapeProgList](`ssz-type:"progressive-list"`)

var unionTypes = map[string]Entry{
	"SampleSquare":                    {New: func() any { return new(CUSquare) }},
	"SampleCircle":                    {New: func() any { return new(CUCircle) }},
	"SampleShape":                     {New: func() any { return new(CUShape) }},
	"SampleNumbers":                   {New: func() any { return new(CUNumbers) }},
	"SampleEmptyProne":                {New: func() any { return new(CUEmptyProne) }},
	"SampleSquareOnly":                {New: func() any { return new(CUSquareOnly) }},
	"SampleNestedShape":               {New: func() any { return new(CUNestedShape) }},
	"SampleShapeContainer":            {New: func() any { return new(CUShapeContainer) }},
	"SampleShapeProgressiveContainer": {New: func() any { return new(CUShapeProgressiveContainer) }},
	"SampleShapeProgressiveList":      {New: func() any { return new(CUShapeProgList) }},
}
