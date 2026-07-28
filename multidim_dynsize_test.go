package dynssz

import (
	"bytes"
	"testing"
)

// A fixed Go array whose outer dynssz-size dimension is the "?" placeholder,
// with the inner dimension sized by a resolved spec expression, must serialize
// as Vector[Vector[byte, N], ArrayLen] — the array's intrinsic length is kept.
//
// Regression for a reflection bug where the "?" placeholder produced a
// zero-size dynamic hint that zeroed the array length, making SizeSSZ/MarshalSSZ
// fail with "vector type [2][]uint8 has zero length" while the codegen path
// (and the SSZ spec) treated it as a valid 10-byte vector-of-vectors.
func TestMultiDimArrayOuterDynSizeQuestion(t *testing.T) {
	specs := map[string]any{"MAX_ATTESTATIONS": uint64(5)}
	ds := NewDynSsz(specs, WithNoFastSsz(), WithNoDelegation())

	type multiDim struct {
		F [2][]byte `ssz-size:"2,6" dynssz-size:"?,MAX_ATTESTATIONS"`
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
