package harness

import (
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Go mirror types for tests/fillers/ssz/test_progressive_containers.py
// ACTIVE_FIELDS tuples map to ssz-index tags: index = position of each 1 bit.

// ACTIVE_FIELDS = (1, 0, 1)
type PCSquare struct {
	Side  uint16 `ssz-index:"0" json:"side"`
	Color uint8  `ssz-index:"2" json:"color"`
}

// ACTIVE_FIELDS = (0, 1, 1)
type PCCircle struct {
	Radius uint16 `ssz-index:"1" json:"radius"`
	Color  uint8  `ssz-index:"2" json:"color"`
}

// ACTIVE_FIELDS = (1,)
type PCOneField struct {
	A uint16 `ssz-index:"0" json:"a"`
}

// ACTIVE_FIELDS = (0, 0, 1)
type PCLeadingGaps struct {
	C uint32 `ssz-index:"2" json:"c"`
}

// ACTIVE_FIELDS = (1, 0, 0, 1, 0, 1)
type PCMultipleGaps struct {
	A uint8  `ssz-index:"0" json:"a"`
	B uint16 `ssz-index:"3" json:"b"`
	C uint32 `ssz-index:"5" json:"c"`
}

// ACTIVE_FIELDS = (0×255, 1)
type PCWidestLayout struct {
	Tail uint8 `ssz-index:"255" json:"tail"`
}

// ACTIVE_FIELDS = (1, 0×20, 1)
type PCLevelBoundary struct {
	First uint16 `ssz-index:"0" json:"first"`
	Last  uint8  `ssz-index:"21" json:"last"`
}

// ACTIVE_FIELDS = (1, 0, 1); body: List[Uint16, 4]
type PCBoundedListField struct {
	Head uint64   `ssz-index:"0" json:"head"`
	Body []uint16 `ssz-index:"2" ssz-max:"4" json:"body"`
}

// ACTIVE_FIELDS = (1, 1, 1)
type PCProgressiveFields struct {
	Head    uint64         `ssz-index:"0" json:"head"`
	Numbers ProgUint64List `ssz-index:"1" json:"numbers"`
	Flags   ProgBitList    `ssz-index:"2" json:"flags"`
}

// ACTIVE_FIELDS = (1, 0, 1)
type PCInnerShape struct {
	X uint16 `ssz-index:"0" json:"x"`
	Y uint8  `ssz-index:"2" json:"y"`
}

// ACTIVE_FIELDS = (1, 0, 1)
type PCOuterShape struct {
	Head  uint8        `ssz-index:"0" json:"head"`
	Inner PCInnerShape `ssz-index:"2" json:"inner"`
}

type PCSquareProgList []PCSquare

var _ = sszutils.Annotate[PCSquareProgList](`ssz-type:"progressive-list"`)

// Ordinary container (no ssz-index)
type PCShapeContainer struct {
	Tag   uint8    `json:"tag"`
	Shape PCSquare `json:"shape"`
}

var progContainerTypes = map[string]Entry{
	"SampleSquare":                {New: func() any { return new(PCSquare) }},
	"SampleCircle":                {New: func() any { return new(PCCircle) }},
	"SampleOneField":              {New: func() any { return new(PCOneField) }},
	"SampleLeadingGaps":           {New: func() any { return new(PCLeadingGaps) }},
	"SampleMultipleGaps":          {New: func() any { return new(PCMultipleGaps) }},
	"SampleWidestLayout":          {New: func() any { return new(PCWidestLayout) }},
	"SampleLevelBoundary":         {New: func() any { return new(PCLevelBoundary) }},
	"SampleBoundedListField":      {New: func() any { return new(PCBoundedListField) }},
	"SampleProgressiveFields":     {New: func() any { return new(PCProgressiveFields) }},
	"SampleInnerShape":            {New: func() any { return new(PCInnerShape) }},
	"SampleOuterShape":            {New: func() any { return new(PCOuterShape) }},
	"SampleSquareProgressiveList": {New: func() any { return new(PCSquareProgList) }},
	"SampleShapeContainer":        {New: func() any { return new(PCShapeContainer) }},
	"SampleUint64ProgressiveList": {New: func() any { return new(ProgUint64List) }},
}
