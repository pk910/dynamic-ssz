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

// Tags are positional, so a dimension whose length comes from its Go type has
// to be skipped in both families whenever an inner dimension needs a length and
// another needs a limit. The array below can only be written with `?` in both,
// and it describes exactly the same SSZ type as the spelling that needs no
// placeholder at all -- so the two must agree byte for byte.
func TestDimensionPlaceholderOnFixedDimension(t *testing.T) {
	ds := NewDynSsz(nil)

	placeholders := &struct {
		F [2][][]byte `ssz-size:"?,?,4" ssz-max:"?,8"`
	}{F: [2][][]byte{{{1, 2, 3, 4}}, {{5, 6, 7, 8}}}}
	spelledOut := &struct {
		F [2][][4]byte `ssz-max:"?,8"`
	}{F: [2][][4]byte{{{1, 2, 3, 4}}, {{5, 6, 7, 8}}}}

	withPlaceholders, err := ds.MarshalSSZ(placeholders)
	if err != nil {
		t.Fatalf("a fixed dimension may be skipped by both tag families: %v", err)
	}
	want, err := ds.MarshalSSZ(spelledOut)
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	if !bytes.Equal(withPlaceholders, want) {
		t.Errorf("encoded %x, want %x", withPlaceholders, want)
	}

	gotRoot, err := ds.HashTreeRoot(placeholders)
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	wantRoot, err := ds.HashTreeRoot(spelledOut)
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	if gotRoot != wantRoot {
		t.Errorf("root %x, want %x", gotRoot, wantRoot)
	}
}

// `?` in both families on a slice dimension says "dynamic, no limit", which is
// what leaving the tags off says. Both spellings therefore describe the same
// unbounded list: they encode, and they have a hash tree root only once
// extended types allow one.
func TestDimensionPlaceholderOnUnboundedList(t *testing.T) {
	tagged := &struct {
		F []byte `ssz-size:"?" ssz-max:"?"`
	}{F: []byte{1, 2, 3}}
	untagged := &struct {
		F []byte
	}{F: []byte{1, 2, 3}}

	for _, tc := range []struct {
		name     string
		ds       *DynSsz
		hashable bool
	}{
		{name: "plain", ds: NewDynSsz(nil)},
		{name: "extended", ds: NewDynSsz(nil, WithExtendedTypes()), hashable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.ds.MarshalSSZ(tagged)
			if err != nil {
				t.Fatalf("an unbounded list spelled with placeholders must encode: %v", err)
			}
			want, err := tc.ds.MarshalSSZ(untagged)
			if err != nil {
				t.Fatalf("MarshalSSZ: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("encoded %x, want %x", got, want)
			}

			_, taggedErr := tc.ds.HashTreeRoot(tagged)
			_, untaggedErr := tc.ds.HashTreeRoot(untagged)
			if tc.hashable {
				if taggedErr != nil {
					t.Errorf("HashTreeRoot: %v", taggedErr)
				}
			} else if !errors.Is(taggedErr, sszutils.ErrExtendedTypeDisabled) {
				t.Errorf("HashTreeRoot err = %v, want ErrExtendedTypeDisabled", taggedErr)
			}
			if (taggedErr == nil) != (untaggedErr == nil) {
				t.Errorf("tagged err = %v, untagged err = %v; the two spellings must agree", taggedErr, untaggedErr)
			}
		})
	}
}

// specSizedVector takes its length from a spec value, with nothing static to
// fall back to.
type specSizedVector struct {
	V []uint16 `dynssz-size:"SPEC_LEN"`
}

// Whether a slice is a vector or a list follows from its tags, not from the
// specs a process happens to have loaded. With the value absent this type used
// to encode as a variable list -- 4 bytes of offset plus contents, a layout no
// other implementation produces for it -- and only failed later, when hashed.
func TestSpecSizedVectorNeedsItsSpecValue(t *testing.T) {
	value := &specSizedVector{V: []uint16{1, 2, 3, 4}}

	resolved := NewDynSsz(map[string]any{"SPEC_LEN": uint64(4)}, WithNoFastSsz(), WithNoDelegation())
	encoded, err := resolved.MarshalSSZ(value)
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	if len(encoded) != 8 {
		t.Errorf("encoded %d bytes, want the 8 bytes of a vector: %x", len(encoded), encoded)
	}

	absent := NewDynSsz(map[string]any{}, WithNoFastSsz(), WithNoDelegation())
	if _, err := absent.MarshalSSZ(value); !errors.Is(err, sszutils.ErrInvalidConstraint) {
		t.Errorf("err = %v, want ErrInvalidConstraint rather than a second encoding", err)
	}
	if _, err := absent.HashTreeRoot(value); !errors.Is(err, sszutils.ErrInvalidConstraint) {
		t.Errorf("err = %v, want ErrInvalidConstraint", err)
	}
}
