// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"fmt"
	"testing"
)

// benchState stands in for a large beacon-state-shaped payload: a big trailing
// list of fixed-size records preceded by some dynamic fields. The trailing list
// is what an unknown-size decode has to consume without knowing where it ends.
type benchRecord struct {
	Index   uint64
	Balance uint64
	Key     [48]byte
}

type benchState struct {
	Slot    uint64
	Roots   [][32]byte    `ssz-max:"8192"`
	Records []benchRecord `ssz-max:"1048576"`
}

func benchStatePayload(tb testing.TB, records int) ([]byte, *DynSsz) {
	tb.Helper()
	ds := NewDynSsz(nil, WithNoFastSsz())

	st := &benchState{Slot: 12345}
	for i := range 64 {
		var r [32]byte
		r[0] = byte(i)
		st.Roots = append(st.Roots, r)
	}
	st.Records = make([]benchRecord, records)
	for i := range st.Records {
		st.Records[i] = benchRecord{Index: uint64(i), Balance: uint64(i) * 32}
	}

	data, err := ds.MarshalSSZ(st)
	if err != nil {
		tb.Fatalf("marshal: %v", err)
	}
	return data, ds
}

// BenchmarkUnmarshalReader compares the three decode paths on the same payload.
// The interesting column is B/op: the buffer path and the old unknown-size
// behaviour both have to hold the whole payload, while streaming does not.
func BenchmarkUnmarshalReader(b *testing.B) {
	for _, records := range []int{1000, 20000} {
		data, ds := benchStatePayload(b, records)
		dsStream := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(4096))

		b.Run(fmt.Sprintf("records=%d/buffer", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				var out benchState
				if err := ds.UnmarshalSSZ(&out, data); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("records=%d/stream-known", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				var out benchState
				if err := dsStream.UnmarshalSSZReader(&out, bytes.NewReader(data), len(data)); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("records=%d/stream-unknown", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				var out benchState
				if err := dsStream.UnmarshalSSZReader(&out, bytes.NewReader(data), -1); err != nil {
					b.Fatal(err)
				}
			}
		})

		// The behaviour unknown-size mode replaces: read it all, then decode.
		b.Run(fmt.Sprintf("records=%d/readall-then-buffer", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				var out benchState
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(bytes.NewReader(data)); err != nil {
					b.Fatal(err)
				}
				if err := ds.UnmarshalSSZ(&out, buf.Bytes()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
