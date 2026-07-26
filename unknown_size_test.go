// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"testing"
	"testing/iotest"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// Unknown-size decoding reads a payload to EOF, so it is sensitive to two things
// the rest of the suite never varies: how the reader chops up the data, and how
// big the decoder's read buffer is relative to the payload. Every other
// UnmarshalSSZReader test uses a bytes.Reader (which hands over everything in one
// call) with the default buffer, which only ever exercises the path where the
// initial fill observes EOF and the stream collapses to a known length.

// chunkReader delivers at most n bytes per Read, without ever returning io.EOF
// alongside data.
type chunkReader struct {
	data []byte
	pos  int
	n    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := min(min(len(p), r.n), len(r.data)-r.pos)
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

// stallReader interleaves a (0, nil) no-op read before every real read. Such a
// read is legal per the io.Reader contract and must not be mistaken for EOF.
type stallReader struct {
	r       io.Reader
	pending bool
}

func (r *stallReader) Read(p []byte) (int, error) {
	if !r.pending {
		r.pending = true
		return 0, nil
	}
	r.pending = false
	return r.r.Read(p)
}

func unknownSizeReaders(data []byte) map[string]func() io.Reader {
	return map[string]func() io.Reader{
		"whole":       func() io.Reader { return bytes.NewReader(data) },
		"oneByte":     func() io.Reader { return iotest.OneByteReader(bytes.NewReader(data)) },
		"chunk3":      func() io.Reader { return &chunkReader{data: data, n: 3} },
		"chunk7":      func() io.Reader { return &chunkReader{data: data, n: 7} },
		"dataWithEOF": func() io.Reader { return iotest.DataErrReader(bytes.NewReader(data)) },
		"stalling":    func() io.Reader { return &stallReader{r: bytes.NewReader(data)} },
	}
}

// Buffer sizes straddling the payload: the small ones force genuine open
// regions, the large one lets the initial fill observe EOF.
var unknownSizeBufSizes = []int{8, 16, 61, 4096}

type usInner struct {
	A uint32
	B []byte `ssz-max:"32"`
}

type usCases struct {
	Fixed        uint64
	FixedVec     [3]uint16
	ListFixed    []uint32 `ssz-max:"16"`
	ListUint64   []uint64 `ssz-max:"16"`
	Bits         []byte   `ssz-type:"bitlist" ssz-max:"64"`
	Nested       *usInner
	ListDynamic  []*usInner `ssz-max:"8"`
	Big          big.Int    `ssz-max:"33"`
	TrailingList []byte     `ssz-max:"128"`
}

// A fixed-size root: nothing is dynamic, so the decode must still terminate
// exactly at EOF.
type usFixed struct {
	A uint64
	B [4]byte
}

// A root whose trailing region is a list of dynamic elements — the deepest
// open-region chain: root -> last element -> that element's last field.
type usDeepTail struct {
	Head uint32
	Tail []*usInner `ssz-max:"8"`
}

func usSamples(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"fixed": &usFixed{A: 0x0102030405060708, B: [4]byte{1, 2, 3, 4}},
		"empty-tail": &usCases{
			Fixed: 1, Bits: []byte{0x01}, Nested: &usInner{A: 1},
			Big: *big.NewInt(0),
		},
		"full": &usCases{
			Fixed:        0xdeadbeefcafe,
			FixedVec:     [3]uint16{7, 8, 9},
			ListFixed:    []uint32{1, 2, 3, 4, 5},
			ListUint64:   []uint64{11, 22, 33},
			Bits:         []byte{0xff, 0x03},
			Nested:       &usInner{A: 42, B: []byte("nested payload")},
			ListDynamic:  []*usInner{{A: 1, B: []byte("a")}, {A: 2, B: []byte("bb")}},
			Big:          *big.NewInt(-1234567890),
			TrailingList: []byte("this trailing list is comfortably longer than the small read buffers"),
		},
		"deep-tail": &usDeepTail{
			Head: 9,
			Tail: []*usInner{{A: 1, B: []byte("xx")}, {A: 2}, {A: 3, B: []byte("longer trailing element payload")}},
		},
		"deep-tail-single": &usDeepTail{Head: 1, Tail: []*usInner{{A: 5, B: []byte("only")}}},
		"deep-tail-empty":  &usDeepTail{Head: 2, Tail: []*usInner{}},
	}
}

// Decoding with size < 0 must produce exactly what the buffer path produces, for
// every reader chunking behaviour and every buffer size.
func TestUnknownSizeMatchesBufferPath(t *testing.T) {
	for name, sample := range usSamples(t) {
		t.Run(name, func(t *testing.T) {
			ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
			want, err := ds.MarshalSSZ(sample)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			for rname, mkReader := range unknownSizeReaders(want) {
				for _, bufSize := range unknownSizeBufSizes {
					t.Run(fmt.Sprintf("%s/buf%d", rname, bufSize), func(t *testing.T) {
						dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))

						target := reflect.New(reflect.TypeOf(sample).Elem()).Interface()
						if err := dsr.UnmarshalSSZReader(target, mkReader(), -1); err != nil {
							t.Fatalf("unknown-size decode: %v", err)
						}

						got, err := dsr.MarshalSSZ(target)
						if err != nil {
							t.Fatalf("re-marshal: %v", err)
						}
						if !bytes.Equal(want, got) {
							t.Fatalf("round-trip mismatch:\n want %x\n  got %x", want, got)
						}
					})
				}
			}
		})
	}
}

// Truncation must be rejected at every prefix, in every buffer regime. This is
// the property most at risk from a read-until-EOF decoder: a short read must not
// be mistaken for a well-formed end of input.
func TestUnknownSizeRejectsTruncation(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	sample := usSamples(t)["full"]
	full, err := ds.MarshalSSZ(sample)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))
		for cut := 0; cut < len(full); cut++ {
			target := &usCases{}
			streamErr := dsr.UnmarshalSSZReader(target, bytes.NewReader(full[:cut]), -1)

			// The buffer path is the oracle: unknown-size decoding may report a
			// different error, but never a different verdict.
			bufErr := dsr.UnmarshalSSZ(&usCases{}, full[:cut])
			if (bufErr == nil) != (streamErr == nil) {
				t.Fatalf("buf=%d cut=%d: verdict differs: buffer=%v stream=%v", bufSize, cut, bufErr, streamErr)
			}
		}
	}
}

// A reader that fails mid-payload must surface that error, not a decode error
// that hides it.
func TestUnknownSizeSurfacesReadError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	full, err := ds.MarshalSSZ(usSamples(t)["full"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	boom := errors.New("read boom")
	dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(8))
	target := &usCases{}
	err = dsr.UnmarshalSSZReader(target, iotest.TimeoutReader(&chunkReader{data: full, n: 4}), -1)
	if err == nil {
		t.Fatal("expected an error from a failing reader")
	}

	err = dsr.UnmarshalSSZReader(&usCases{}, errorReader{boom}, -1)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// The maximum stream size bounds an unknown-size decode and cannot be disabled.
func TestUnknownSizeMaxStreamSize(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	full, err := ds.MarshalSSZ(usSamples(t)["full"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// A cap below the payload size must reject rather than truncate.
	dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(8), WithMaxStreamSize(len(full)-1))
	if err := dsr.UnmarshalSSZReader(&usCases{}, bytes.NewReader(full), -1); err == nil {
		t.Fatal("expected an error when the payload exceeds the maximum stream size")
	}

	// A cap at exactly the payload size still decodes.
	dsr = NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(8), WithMaxStreamSize(len(full)))
	target := &usCases{}
	if err := dsr.UnmarshalSSZReader(target, bytes.NewReader(full), -1); err != nil {
		t.Fatalf("decode at exact cap: %v", err)
	}

	// A non-positive maximum falls back to the default rather than meaning
	// "unlimited" — the bound is load-bearing and cannot be switched off.
	dsr = NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithMaxStreamSize(0))
	if err := dsr.UnmarshalSSZReader(&usCases{}, bytes.NewReader(full), -1); err != nil {
		t.Fatalf("decode with default cap: %v", err)
	}
}

// ssz-max must be enforced while reading, so an over-long list is rejected
// before it is allocated rather than after.
func TestUnknownSizeEnforcesListLimit(t *testing.T) {
	type small struct {
		L []uint32 `ssz-max:"2"`
	}
	type big struct {
		L []uint32 `ssz-max:"64"`
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	over, err := ds.MarshalSSZ(&big{L: []uint32{1, 2, 3, 4, 5}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))
		err := dsr.UnmarshalSSZReader(&small{}, bytes.NewReader(over), -1)
		if !errors.Is(err, sszutils.ErrListTooBig) {
			t.Fatalf("buf=%d: err = %v, want ErrListTooBig", bufSize, err)
		}
	}
}

// A bitlist's terminator lives in the last byte of its region, so an unknown
// length must not lose it.
func TestUnknownSizeBitlistTermination(t *testing.T) {
	type bl struct {
		B []byte `ssz-type:"bitlist" ssz-max:"64"`
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))

		valid, err := ds.MarshalSSZ(&bl{B: []byte{0xff, 0x01}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		target := &bl{}
		if err := dsr.UnmarshalSSZReader(target, bytes.NewReader(valid), -1); err != nil {
			t.Fatalf("buf=%d: valid bitlist: %v", bufSize, err)
		}
		if !bytes.Equal(target.B, []byte{0xff, 0x01}) {
			t.Fatalf("buf=%d: got %x", bufSize, target.B)
		}

		// A zero last byte carries no terminator and must be rejected.
		unterminated := append(append([]byte{}, valid[:4]...), 0xff, 0x00)
		if err := dsr.UnmarshalSSZReader(&bl{}, bytes.NewReader(unterminated), -1); err == nil {
			t.Fatalf("buf=%d: expected an error for an unterminated bitlist", bufSize)
		}
	}
}
