// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

// bufSizes exercises the three regimes that matter for an unknown-length
// decode: a buffer far smaller than the payload (true open-region path), one
// straddling it, and one large enough that Prefill sees EOF immediately (the
// collapse-to-known-length fast path).
var bufSizes = []int{8, 16, 64, 4096}

func unknownReaders(data []byte) map[string]func() io.Reader {
	return map[string]func() io.Reader{
		"bytes":         func() io.Reader { return bytes.NewReader(data) },
		"drip":          func() io.Reader { return &dripReader{data: data} },
		"short3":        func() io.Reader { return &shortReader{data: data, maxRead: 3} },
		"partialEOF":    func() io.Reader { return &partialThenEOFReader{data: data} },
		"dataErr":       func() io.Reader { return iotest.DataErrReader(bytes.NewReader(data)) },
		"zeroInterleav": func() io.Reader { return &zeroInterleaveReader{r: bytes.NewReader(data)} },
		"oneByteAtTime": func() io.Reader { return iotest.OneByteReader(bytes.NewReader(data)) },
	}
}

// A bounded region must answer More/DecodeRemaining from its limit, for both
// decoder implementations.
func TestRegion_BoundedDecoders(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	decoders := map[string]func() Decoder{
		"buffer": func() Decoder { return NewBufferDecoder(data) },
		"stream": func() Decoder { return NewStreamDecoder(bytes.NewReader(data), len(data), 8) },
	}

	for name, mk := range decoders {
		t.Run(name, func(t *testing.T) {
			dec := mk()
			if !dec.LengthKnown() {
				t.Fatal("bounded decoder reports unknown length")
			}
			if got := dec.GetLength(); got != 10 {
				t.Fatalf("GetLength = %d, want 10", got)
			}

			dec.PushLimit(4)
			if more, err := dec.More(); err != nil || !more {
				t.Fatalf("More in region: %v %v", more, err)
			}
			got, err := dec.DecodeRemaining(-1)
			if err != nil {
				t.Fatalf("DecodeRemaining: %v", err)
			}
			if !bytes.Equal(got, data[:4]) {
				t.Fatalf("DecodeRemaining = %v, want %v", got, data[:4])
			}
			if more, err := dec.More(); err != nil || more {
				t.Fatalf("More at region end: %v %v", more, err)
			}
			if diff := dec.PopLimit(); diff != 0 {
				t.Fatalf("PopLimit = %d, want 0", diff)
			}

			// The rest of the stream is still readable.
			rest, err := dec.DecodeRemaining(-1)
			if err != nil {
				t.Fatalf("DecodeRemaining rest: %v", err)
			}
			if !bytes.Equal(rest, data[4:]) {
				t.Fatalf("rest = %v, want %v", rest, data[4:])
			}
		})
	}
}

// DecodeRemaining must refuse to allocate a payload larger than the cap.
func TestRegion_DecodeRemainingMax(t *testing.T) {
	data := make([]byte, 100)

	t.Run("buffer", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		if _, err := dec.DecodeRemaining(50); !errors.Is(err, ErrStreamTooLarge) {
			t.Fatalf("err = %v, want ErrStreamTooLarge", err)
		}
	})
	t.Run("stream-known", func(t *testing.T) {
		dec := NewStreamDecoder(bytes.NewReader(data), len(data), 8)
		if _, err := dec.DecodeRemaining(50); !errors.Is(err, ErrStreamTooLarge) {
			t.Fatalf("err = %v, want ErrStreamTooLarge", err)
		}
	})
	t.Run("stream-unknown", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		if _, err := dec.DecodeRemaining(50); !errors.Is(err, ErrStreamTooLarge) {
			t.Fatalf("err = %v, want ErrStreamTooLarge", err)
		}
	})
}

// An open region reads to EOF regardless of reader behaviour or buffer size.
func TestRegion_OpenDecodeRemaining(t *testing.T) {
	data := []byte("the quick brown fox jumps over the lazy dog, repeatedly and at length")

	for rname, mkReader := range unknownReaders(data) {
		for _, bufSize := range bufSizes {
			t.Run(rname, func(t *testing.T) {
				dec := NewUnknownStreamDecoder(mkReader(), bufSize, 0)
				if err := dec.Prefill(); err != nil {
					t.Fatalf("Prefill: %v", err)
				}
				dec.PushOpenLimit()
				got, err := dec.DecodeRemaining(-1)
				if err != nil {
					t.Fatalf("buf=%d DecodeRemaining: %v", bufSize, err)
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("buf=%d got %q, want %q", bufSize, got, data)
				}
				if err := FinishRegion(dec); err != nil {
					t.Fatalf("buf=%d FinishRegion: %v", bufSize, err)
				}
				if err := FinishStream(dec); err != nil {
					t.Fatalf("buf=%d FinishStream: %v", bufSize, err)
				}
			})
		}
	}
}

// Once EOF is observed the decoder must behave exactly like a known-length one.
func TestRegion_EOFCollapsesToKnownLength(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	dec := NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
	dec.PushOpenLimit()
	if dec.LengthKnown() {
		t.Fatal("open region reports known length before EOF")
	}
	// The allowance is reported instead of a sentinel: finite and plausible.
	if got := dec.GetLength(); got != DefaultMaxStreamSize {
		t.Fatalf("open GetLength = %d, want allowance %d", got, DefaultMaxStreamSize)
	}

	if _, err := dec.DecodeRemaining(-1); err != nil {
		t.Fatalf("DecodeRemaining: %v", err)
	}
	if !dec.LengthKnown() {
		t.Fatal("length still unknown after EOF")
	}
	if got := dec.GetLength(); got != 0 {
		t.Fatalf("GetLength after EOF = %d, want 0", got)
	}
	if got := dec.GetPosition(); got != len(data) {
		t.Fatalf("position = %d, want %d", got, len(data))
	}
}

// Prefill collapses a payload that fits in the read buffer to known length, so
// the decode runs entirely on the fully-validated known-length path.
func TestRegion_PrefillCollapses(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	dec := NewUnknownStreamDecoder(bytes.NewReader(data), 4096, 0)
	if dec.LengthKnown() {
		t.Fatal("length known before Prefill")
	}
	if err := dec.Prefill(); err != nil {
		t.Fatalf("Prefill: %v", err)
	}
	if !dec.LengthKnown() {
		t.Fatal("Prefill did not collapse a fully-buffered payload")
	}
	if got := dec.GetLength(); got != len(data) {
		t.Fatalf("GetLength = %d, want %d", got, len(data))
	}

	// A payload larger than the buffer must stay open.
	big := make([]byte, 100)
	dec = NewUnknownStreamDecoder(bytes.NewReader(big), 8, 0)
	if err := dec.Prefill(); err != nil {
		t.Fatalf("Prefill: %v", err)
	}
	if dec.LengthKnown() {
		t.Fatal("Prefill collapsed a payload larger than the buffer")
	}
}

// Nested open regions: an open child of an open parent stays open; an open
// child of a bounded parent inherits the parent's bound.
func TestRegion_OpenNesting(t *testing.T) {
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
	dec.PushOpenLimit() // root
	dec.PushOpenLimit() // trailing child
	if dec.LengthKnown() {
		t.Fatal("open child of open parent is bounded")
	}
	dec.PopLimit()

	dec.PushLimit(10)
	if !dec.LengthKnown() {
		t.Fatal("bounded region reports unknown length")
	}
	dec.PushOpenLimit() // open child of a bounded parent
	if !dec.LengthKnown() {
		t.Fatal("open child of bounded parent should inherit the bound")
	}
	if got := dec.GetLength(); got != 10 {
		t.Fatalf("inherited length = %d, want 10", got)
	}
	got, err := dec.DecodeRemaining(-1)
	if err != nil {
		t.Fatalf("DecodeRemaining: %v", err)
	}
	if !bytes.Equal(got, data[:10]) {
		t.Fatalf("got %v, want %v", got, data[:10])
	}
	if err := FinishRegion(dec); err != nil {
		t.Fatalf("FinishRegion: %v", err)
	}
	if diff := dec.PopLimit(); diff != 0 {
		t.Fatalf("PopLimit = %d, want 0", diff)
	}
}

// Under-consumption of an open region must be caught by the top-level
// assertion, since an open region is always the suffix of the stream.
func TestRegion_UnderConsumptionCaught(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	dec := NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
	dec.PushOpenLimit()
	if _, err := dec.DecodeBytesBuf(4); err != nil {
		t.Fatalf("DecodeBytesBuf: %v", err)
	}
	// The region was left half-consumed.
	if err := FinishRegion(dec); !errors.Is(err, ErrOffset) {
		t.Fatalf("FinishRegion err = %v, want ErrOffset (trailing data)", err)
	}
}

func TestRegion_FinishStreamDetectsTrailing(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	dec := NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
	if _, err := dec.DecodeUint32(); err != nil {
		t.Fatalf("DecodeUint32: %v", err)
	}
	if err := FinishStream(dec); !errors.Is(err, ErrOffset) {
		t.Fatalf("FinishStream err = %v, want ErrOffset (trailing data)", err)
	}

	dec = NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
	if _, err := dec.DecodeUint64(); err != nil {
		t.Fatalf("DecodeUint64: %v", err)
	}
	if err := FinishStream(dec); err != nil {
		t.Fatalf("FinishStream at EOF: %v", err)
	}
}

// The maximum stream size is load-bearing: it bounds an unknown-length decode
// and is what GetLength reports as the allowance inside an open region.
func TestRegion_MaxStreamSize(t *testing.T) {
	data := make([]byte, 1000)

	dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 100)
	dec.PushOpenLimit()
	if got := dec.GetLength(); got != 100 {
		t.Fatalf("allowance = %d, want 100", got)
	}
	if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrStreamTooLarge) {
		t.Fatalf("err = %v, want ErrStreamTooLarge", err)
	}

	// A bulk read beyond the allowance is refused up front.
	dec = NewUnknownStreamDecoder(bytes.NewReader(data), 8, 100)
	dec.PushOpenLimit()
	if _, err := dec.DecodeBytesBuf(200); !errors.Is(err, ErrStreamTooLarge) {
		t.Fatalf("err = %v, want ErrStreamTooLarge", err)
	}

	// A non-positive maximum falls back to the default; it is never unlimited.
	dec = NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
	if dec.maxSize != DefaultMaxStreamSize {
		t.Fatalf("maxSize = %d, want %d", dec.maxSize, DefaultMaxStreamSize)
	}
	dec = NewUnknownStreamDecoder(bytes.NewReader(data), 8, -1)
	if dec.maxSize != DefaultMaxStreamSize {
		t.Fatalf("maxSize = %d, want %d", dec.maxSize, DefaultMaxStreamSize)
	}
}

// Misbehaving readers must fail cleanly in unknown-length mode too.
func TestRegion_HostileReaders(t *testing.T) {
	t.Run("negative", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(negativeReader{}, 1024, 0)
		dec.PushOpenLimit()
		if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrNegativeRead) {
			t.Fatalf("err = %v, want ErrNegativeRead", err)
		}
	})
	t.Run("zeroForever", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(zeroForeverReader{}, 1024, 0)
		dec.PushOpenLimit()
		if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})
	t.Run("readError", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: make([]byte, 4), errAfter: 2, err: want}, 1024, 0)
		dec.PushOpenLimit()
		if _, err := dec.DecodeRemaining(-1); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
	t.Run("truncatedFixedRead", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader([]byte{1, 2, 3}), 1024, 0)
		dec.PushOpenLimit()
		if _, err := dec.DecodeUint64(); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})
}

// Sequential primitive reads must work identically in unknown-length mode
// across every reader and buffer size.
func TestRegion_PrimitivesUnknownLength(t *testing.T) {
	data := []byte{
		1,          // bool
		0x02, 0x03, // uint16
		0x04, 0x05, 0x06, 0x07, // uint32
		0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, // uint64
	}

	for rname, mkReader := range unknownReaders(data) {
		for _, bufSize := range bufSizes {
			t.Run(rname, func(t *testing.T) {
				dec := NewUnknownStreamDecoder(mkReader(), bufSize, 0)
				dec.PushOpenLimit()

				b, err := dec.DecodeBool()
				if err != nil || !b {
					t.Fatalf("buf=%d bool: %v %v", bufSize, b, err)
				}
				u16, err := dec.DecodeUint16()
				if err != nil || u16 != 0x0302 {
					t.Fatalf("buf=%d uint16: %#x %v", bufSize, u16, err)
				}
				u32, err := dec.DecodeUint32()
				if err != nil || u32 != 0x07060504 {
					t.Fatalf("buf=%d uint32: %#x %v", bufSize, u32, err)
				}
				u64, err := dec.DecodeUint64()
				if err != nil || u64 != 0x0f0e0d0c0b0a0908 {
					t.Fatalf("buf=%d uint64: %#x %v", bufSize, u64, err)
				}
				if err := FinishRegion(dec); err != nil {
					t.Fatalf("buf=%d FinishRegion: %v", bufSize, err)
				}
			})
		}
	}
}

// NewStreamDecoder with a negative length selects unknown-length mode.
func TestRegion_NegativeTotalLenSelectsUnknown(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	dec := NewStreamDecoder(bytes.NewReader(data), -1, 8)
	dec.PushOpenLimit()
	if dec.LengthKnown() {
		t.Fatal("negative totalLen did not select unknown-length mode")
	}
	got, err := dec.DecodeRemaining(-1)
	if err != nil {
		t.Fatalf("DecodeRemaining: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %v, want %v", got, data)
	}
}
