package dynssz

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// A fixed outer dimension with an inner dimension sized by a resolved spec
// expression must serialize as Vector[Vector[byte, N], ArrayLen] -- the outer
// length is kept and the inner one comes from the spec.
//
// The outer dimension repeats its static length in the dynamic tag rather than
// marking it "?". The placeholder means "this dimension is dynamic", so pairing
// it with a static length declares the dimension two ways at once and is
// rejected; TestDimensionPlaceholderMustMatch covers that.
//
// Regression for a reflection bug where an outer dynamic hint of zero zeroed
// the array length, making SizeSSZ/MarshalSSZ fail with "vector type
// [2][]uint8 has zero length" while the codegen path (and the SSZ spec) treated
// it as a valid 10-byte vector-of-vectors.
func TestMultiDimArrayOuterDynSize(t *testing.T) {
	specs := map[string]any{"MAX_ATTESTATIONS": uint64(5)}
	ds := NewDynSsz(specs, WithNoFastSsz(), WithNoDelegation())

	type multiDim struct {
		F [2][]byte `ssz-size:"2,6" dynssz-size:"2,MAX_ATTESTATIONS"`
	}
	v := &multiDim{F: [2][]byte{{1, 2, 3, 4, 5}, {6, 7, 8, 9, 10}}}

	size, err := ds.SizeSSZ(v)
	if err != nil {
		t.Fatalf("SizeSSZ: %v", err)
	}
	if size != 10 {
		t.Fatalf("size: expected 10, got %d", size)
	}

	got, err := ds.MarshalSSZ(v)
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	want := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if !bytes.Equal(got, want) {
		t.Fatalf("marshal: expected %x, got %x", want, got)
	}

	// Round-trip.
	var out multiDim
	if uerr := ds.UnmarshalSSZ(&out, got); uerr != nil {
		t.Fatalf("UnmarshalSSZ: %v", uerr)
	}
	re, err := ds.MarshalSSZ(&out)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(re, want) {
		t.Fatalf("round-trip: expected %x, got %x", want, re)
	}
}

// `?` declares a dimension dynamic in a size tag and unbounded in a max tag, so
// it has to mean the same thing in the static tag and its dynamic counterpart.
// Declaring a length in one and a placeholder in the other describes two
// different types, and silently picking either one produced a field that no
// longer matched its own tags.
func TestDimensionPlaceholderMustMatch(t *testing.T) {
	specs := map[string]any{"SPEC": uint64(4)}
	ds := NewDynSsz(specs, WithNoFastSsz(), WithNoDelegation())

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "size static value, dynamic placeholder",
			value: &struct {
				F [2][]byte `ssz-size:"2,6" dynssz-size:"?,SPEC"`
			}{},
			want: "conflicting size tags",
		},
		{
			name: "size static placeholder, dynamic value",
			value: &struct {
				F [][]byte `ssz-size:"?,6" dynssz-size:"SPEC,6"`
			}{},
			want: "conflicting size tags",
		},
		{
			name: "max static value, dynamic placeholder",
			value: &struct {
				F []uint64 `ssz-max:"16" dynssz-max:"?"`
			}{},
			want: "conflicting max tags",
		},
		{
			name: "max static placeholder, dynamic value",
			value: &struct {
				F [][]uint64 `ssz-max:"?,8" dynssz-max:"SPEC,8"`
			}{},
			want: "conflicting max tags",
		},
		{
			name: "dimension declared by neither tag",
			value: &struct {
				F [][]byte `ssz-size:"?,6" ssz-max:"?,8"`
			}{},
			want: "neither a length nor a limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ds.SizeSSZ(tt.value)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tt.want)
			}
			if !errors.Is(err, sszutils.ErrInvalidTag) {
				t.Errorf("err = %v, want ErrInvalidTag", err)
			}
		})
	}

	// The placeholders line up here, so the pair is accepted.
	ok := &struct {
		F [][]uint64 `ssz-max:"?,8" dynssz-max:"?,SPEC"`
	}{F: [][]uint64{{1}}}
	if _, err := ds.SizeSSZ(ok); err != nil {
		t.Fatalf("matching placeholders should be accepted: %v", err)
	}
}
