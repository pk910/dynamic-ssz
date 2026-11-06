package harness

import (
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Go mirror types for tests/fillers/ssz/test_progressive_types.py

type ProgBytes32 [32]byte

type ProgUint64List []uint64

var _ = sszutils.Annotate[ProgUint64List](`ssz-type:"progressive-list"`)

type ProgBytes32List [][32]byte

var _ = sszutils.Annotate[ProgBytes32List](`ssz-type:"progressive-list"`)

type ProgUint16List []uint16

var _ = sszutils.Annotate[ProgUint16List](`ssz-type:"progressive-list"`)

type ProgNestedList []ProgUint16List

var _ = sszutils.Annotate[ProgNestedList](`ssz-type:"progressive-list"`)

type ProgBitList []byte

var _ = sszutils.Annotate[ProgBitList](`ssz-type:"progressive-bitlist"`)

type ProgContainerWithList struct {
	A uint16         `json:"a"`
	B ProgUint64List `json:"b"`
	C uint8          `json:"c"`
}

var progressiveTypes = map[string]Entry{
	"Bytes32":                            {New: func() any { return new(ProgBytes32) }},
	"SampleUint64ProgressiveList":        {New: func() any { return new(ProgUint64List) }},
	"SampleBytes32ProgressiveList":       {New: func() any { return new(ProgBytes32List) }},
	"SampleUint16ProgressiveList":        {New: func() any { return new(ProgUint16List) }},
	"SampleNestedProgressiveList":        {New: func() any { return new(ProgNestedList) }},
	"ProgressiveBitList":                 {New: func() any { return new(ProgBitList) }},
	"SampleContainerWithProgressiveList": {New: func() any { return new(ProgContainerWithList) }},
	"Boolean":                            {New: func() any { return new(BasicBool) }},
}
