package engine

import (
	"math/rand"
	"reflect"
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

// fillerRecPtr is self-referential through a pointer. The optional-list tag is
// what the corpus generator emits for this shape; fillPointer only considers
// leaving a pointer nil for `ssz-type:"optional"`, so before the depth budget
// this descended forever and killed the worker with a stack overflow instead of
// testing the entry.
type fillerRecPtr struct {
	V    uint8
	Next *fillerRecPtr `ssz-type:"optional-list"`
}

// fillerRecList recurses through a bounded list instead of a pointer.
type fillerRecList struct {
	V uint8
	L []fillerRecList `ssz-max:"2"`
}

// fillerRecMutualA and fillerRecMutualB close the cycle through two types.
type fillerRecMutualA struct {
	V uint8
	B *fillerRecMutualB `ssz-type:"optional-list"`
}

type fillerRecMutualB struct {
	V uint8
	A *fillerRecMutualA `ssz-type:"optional-list"`
}

// TestFillerBoundsRecursionDepth pins that Fill terminates on self-referential
// types. A stack overflow is fatal (recover cannot catch it), so a single
// recursive corpus entry took the whole fuzz worker down.
func TestFillerBoundsRecursionDepth(t *testing.T) {
	tests := []struct {
		name string
		new  func() any
	}{
		{"pointer_recursion", func() any { return new(fillerRecPtr) }},
		{"list_recursion", func() any { return new(fillerRecList) }},
		{"mutual_recursion", func() any { return new(fillerRecMutualA) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for seed := range 20 {
				filler := NewFiller(rand.New(rand.NewSource(int64(seed))))
				filler.FillStruct(tc.new())

				// The budget must be released again, or a long fuzz run would
				// stop filling anything after the first recursive entry.
				if filler.depth != 0 {
					t.Fatalf("seed %d: depth = %d after Fill, want 0", seed, filler.depth)
				}
			}
		})
	}
}

// TestFillerDepthBudgetStillFillsShallowValues guards against the budget being
// so tight that ordinary corpus types stop getting data.
func TestFillerDepthBudgetStillFillsShallowValues(t *testing.T) {
	type inner struct {
		X uint64
		Y []byte `ssz-max:"8"`
	}
	type outer struct {
		A inner
		B []inner `ssz-max:"4"`
	}

	// A value that cannot be set is left alone rather than panicking.
	NewFiller(rand.New(rand.NewSource(1))).Fill(reflect.ValueOf(uint64(1)))

	filled := false
	for seed := range 20 {
		v := new(outer)
		NewFiller(rand.New(rand.NewSource(int64(seed)))).FillStruct(v)
		if v.A.X != 0 || len(v.A.Y) > 0 || len(v.B) > 0 {
			filled = true
			break
		}
	}
	if !filled {
		t.Fatal("depth budget left a shallow value entirely empty")
	}
}
