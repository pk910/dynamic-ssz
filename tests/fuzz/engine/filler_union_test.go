package engine

import (
	"math/rand"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

type fillerUnion2 struct {
	U dynssz.CompatibleUnion[struct {
		Variant0 uint32
		Variant1 uint64
	}]
}

type fillerUnion3 struct {
	U dynssz.CompatibleUnion[struct {
		Variant0 uint16
		Variant1 uint32
		Variant2 uint64
	}]
}

// TestFillerUnionSelectorsAreMarshalable pins that filled unions carry a
// selector the engines accept and that addresses the descriptor field the data
// was built from. A filler that numbers variants from 0 produces values that
// every operation rejects, so both engines agree on the error and the fuzzer
// reports no issues while covering nothing.
func TestFillerUnionSelectorsAreMarshalable(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz())

	cases := []struct {
		name string
		new  func() any
	}{
		{"two-variants", func() any { return &fillerUnion2{} }},
		{"three-variants", func() any { return &fillerUnion3{} }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for seed := int64(0); seed < 20; seed++ {
				rng := rand.New(rand.NewSource(seed))
				filler := NewFiller(rng)
				instance := tc.new()
				filler.FillStruct(instance)

				if _, err := ds.MarshalSSZ(instance); err != nil {
					t.Fatalf("seed %d: marshal: %v", seed, err)
				}

				if _, err := ds.HashTreeRoot(instance); err != nil {
					t.Fatalf("seed %d: hash tree root: %v", seed, err)
				}
			}
		})
	}
}
