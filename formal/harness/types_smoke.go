package harness

import (
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Go mirror types for tests/fillers/ssz/test_decode_failure_smoke.py

type SmokeBitList8 []byte

var _ = sszutils.Annotate[SmokeBitList8](`ssz-type:"bitlist" ssz-max:"8"`)

var smokeTypes = map[string]Entry{
	"SmokeBitList8": {New: func() any { return new(SmokeBitList8) }},
}
