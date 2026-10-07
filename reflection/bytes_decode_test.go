// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection_test

import (
	"bytes"
	"reflect"
	"testing"

	. "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

type namedBytes []byte

type annBytes []byte

var _ = sszutils.Annotate[annBytes](`ssz-max:"8"`)

type byteFields1 struct {
	A []byte `ssz-max:"16"`
}

type byteFields3 struct {
	A []byte `ssz-max:"16"`
	B []byte `ssz-size:"4"`
	C []byte `ssz-max:"16"`
}

// Byte lists and vectors decode to the same values through the buffer and the
// stream decoder, with an empty list decoding to an empty, non-nil slice.
func TestUnmarshalByteSlices(t *testing.T) {
	ds := NewDynSsz(nil)
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"list", &struct {
			L []byte `ssz-max:"16"`
		}{L: []byte{1, 2, 3}}},
		{"empty list", &struct {
			L []byte `ssz-max:"16"`
		}{L: []byte{}}},
		{"vector", &struct {
			V []byte `ssz-size:"5"`
		}{V: []byte{1, 2, 3, 4, 5}}},
		{"named", &struct {
			L namedBytes `ssz-max:"8"`
			V namedBytes `ssz-size:"4"`
		}{L: namedBytes{9}, V: namedBytes{1, 2, 3, 4}}},
		{"pointer root", func() *annBytes { v := annBytes{7, 8}; return &v }()},
		{"list of lists", &struct {
			Txs [][]byte `ssz-max:"4,8"`
		}{Txs: [][]byte{{1}, {}, {2, 3}}}},
		{"bitvector", &struct {
			V []byte `ssz-type:"bitvector" ssz-bitsize:"12"`
		}{V: []byte{0xff, 0x0f}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := ds.MarshalSSZ(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, stream := range []bool{false, true} {
				decoded := reflect.New(reflect.TypeOf(tc.value).Elem()).Interface()
				if stream {
					err = ds.UnmarshalSSZReader(decoded, bytes.NewReader(raw), len(raw))
				} else {
					err = ds.UnmarshalSSZ(decoded, raw)
				}
				if err != nil || !reflect.DeepEqual(decoded, tc.value) {
					t.Errorf("stream=%v: decoded %+v, %v; want %+v", stream, decoded, err, tc.value)
				}
			}
		})
	}
}

// A byte slice that fails to decode leaves the target field as it was.
func TestUnmarshalByteSliceErrorKeepsTarget(t *testing.T) {
	ds := NewDynSsz(nil)
	old := []byte{0xaa, 0xbb}

	vector := &struct {
		V []byte `ssz-size:"32"`
	}{V: old}
	if err := ds.UnmarshalSSZReader(vector, bytes.NewReader(make([]byte, 10)), 32); err == nil {
		t.Fatal("truncated vector decoded")
	}
	if !bytes.Equal(vector.V, old) {
		t.Errorf("vector target changed on error: %x", vector.V)
	}

	list := &struct {
		A uint32
		L []byte `ssz-max:"16"`
	}{L: old}
	raw := []byte{1, 0, 0, 0, 8, 0, 0, 0, 1, 2, 3}
	if err := ds.UnmarshalSSZReader(list, bytes.NewReader(raw), len(raw)+5); err == nil {
		t.Fatal("truncated list decoded")
	}
	if !bytes.Equal(list.L, old) {
		t.Errorf("list target changed on error: %x", list.L)
	}

	padded := &struct {
		V []byte `ssz-type:"bitvector" ssz-bitsize:"12"`
	}{V: old}
	if err := ds.UnmarshalSSZ(padded, []byte{0xff, 0xff}); err == nil {
		t.Fatal("bitvector padding bits accepted")
	}
	if !bytes.Equal(padded.V, old) {
		t.Errorf("bitvector target changed on error: %x", padded.V)
	}
}

// Each decoded byte slice costs exactly its own allocation.
func TestUnmarshalByteSliceAllocs(t *testing.T) {
	ds := NewDynSsz(nil)
	one, err := ds.MarshalSSZ(&byteFields1{A: []byte{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	three, err := ds.MarshalSSZ(&byteFields3{A: []byte{1, 2}, B: []byte{1, 2, 3, 4}, C: []byte{3}})
	if err != nil {
		t.Fatal(err)
	}
	var t1 byteFields1
	var t3 byteFields3
	allocs1 := testing.AllocsPerRun(100, func() { _ = ds.UnmarshalSSZ(&t1, one) })
	allocs3 := testing.AllocsPerRun(100, func() { _ = ds.UnmarshalSSZ(&t3, three) })
	if diff := allocs3 - allocs1; diff != 2 {
		t.Errorf("two more byte slices cost %v allocations, want 2", diff)
	}
}
