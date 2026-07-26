// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"errors"
	"io"
	"math"
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

// --- targeted coverage of the open-region edge cases ---

// GetLength must stay non-negative even once the position has run past the
// allowance, since callers size allocations from it.
func TestRegion_AllowanceExhausted(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 64)), 8, 16)
	dec.PushOpenLimit()
	if _, err := dec.DecodeBytesBuf(16); err != nil {
		t.Fatalf("read up to the allowance: %v", err)
	}
	if got := dec.GetLength(); got != 0 {
		t.Fatalf("GetLength at the allowance = %d, want 0", got)
	}
	// Force the position past the allowance and confirm the clamp holds.
	dec.position = dec.maxSize + 5
	if got := dec.GetLength(); got != 0 {
		t.Fatalf("GetLength past the allowance = %d, want 0", got)
	}
}

// A limit large enough to overflow the position must not wrap into a negative
// region, and inside an open region the allowance is the only backstop.
func TestRegion_PushLimitClamping(t *testing.T) {
	t.Run("overflow", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 8)), 8, 0)
		dec.PushLimit(math.MaxInt)
		if !dec.LengthKnown() {
			t.Fatal("a clamped limit should be a bounded region")
		}
		if got := dec.GetLength(); got < 0 {
			t.Fatalf("GetLength = %d, want non-negative", got)
		}
	})

	t.Run("clamped to the allowance in an open region", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 8)), 8, 32)
		dec.PushOpenLimit()
		dec.PushLimit(1000) // far past the allowance
		if got := dec.GetLength(); got != 32 {
			t.Fatalf("GetLength = %d, want the 32-byte allowance", got)
		}
	})
}

// EOF is sticky: observing it twice must not move the recorded stream length.
func TestRegion_EOFIsIdempotent(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader([]byte{1, 2, 3}), 8, 0)
	dec.PushOpenLimit()
	if _, err := dec.DecodeRemaining(-1); err != nil {
		t.Fatalf("DecodeRemaining: %v", err)
	}
	first := dec.streamLen
	dec.onEOF()
	if dec.streamLen != first {
		t.Fatalf("streamLen moved from %d to %d on a repeat EOF", first, dec.streamLen)
	}
	// Reads and probes after EOF stay consistent.
	if err := dec.readMore(); err != nil {
		t.Fatalf("readMore after EOF: %v", err)
	}
	if more, err := dec.More(); err != nil || more {
		t.Fatalf("More after EOF = %v, %v", more, err)
	}
	if err := dec.Prefill(); err != nil {
		t.Fatalf("Prefill after EOF: %v", err)
	}
}

// growBuffer is a no-op when the buffer is already large enough, and readMore
// makes no progress (rather than spinning) when the buffer is full.
func TestRegion_BufferManagement(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 64)), 16, 0)
	before := len(dec.buffer)
	dec.growBuffer(4)
	if len(dec.buffer) != before {
		t.Fatalf("growBuffer(4) resized from %d to %d", before, len(dec.buffer))
	}

	// Fill the buffer, then confirm another read cannot add to it.
	if err := dec.Prefill(); err != nil {
		t.Fatalf("Prefill: %v", err)
	}
	if dec.bufferLen != len(dec.buffer) {
		t.Fatalf("buffer not full after Prefill: %d of %d", dec.bufferLen, len(dec.buffer))
	}
	filled := dec.bufferLen
	if err := dec.readMore(); err != nil {
		t.Fatalf("readMore on a full buffer: %v", err)
	}
	if dec.bufferLen != filled {
		t.Fatalf("readMore grew a full buffer from %d to %d", filled, dec.bufferLen)
	}
}

// Prefill is a no-op for a known-length decoder, and stops rather than spinning
// when it cannot make progress.
func TestRegion_PrefillNoProgress(t *testing.T) {
	known := NewStreamDecoder(bytes.NewReader(make([]byte, 8)), 8, 8)
	if err := known.Prefill(); err != nil {
		t.Fatalf("Prefill on a known-length decoder: %v", err)
	}
	if known.bufferLen != 0 {
		t.Fatal("Prefill should be a no-op when the length is known")
	}

	// A payload larger than the allowance is rejected up front rather than
	// silently truncated.
	capped := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 4096)), 1024, 16)
	if err := capped.Prefill(); !errors.Is(err, ErrStreamTooLarge) {
		t.Fatalf("Prefill with a small allowance: err = %v, want ErrStreamTooLarge", err)
	}

	// An empty stream makes no progress and must stop immediately.
	empty := NewUnknownStreamDecoder(bytes.NewReader(nil), 64, 0)
	if err := empty.Prefill(); err != nil {
		t.Fatalf("Prefill on an empty stream: %v", err)
	}
	if !empty.LengthKnown() || empty.GetLength() != 0 {
		t.Fatalf("empty stream: LengthKnown=%v GetLength=%d", empty.LengthKnown(), empty.GetLength())
	}
}

// DecodeRemaining on a bounded region: the length clamp and the read-error path.
func TestRegion_DecodeRemainingBounded(t *testing.T) {
	t.Run("empty region", func(t *testing.T) {
		dec := NewStreamDecoder(bytes.NewReader(nil), 0, 8)
		got, err := dec.DecodeRemaining(-1)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("truncated stream", func(t *testing.T) {
		dec := NewStreamDecoder(bytes.NewReader([]byte{1, 2}), 8, 8)
		if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})
}

// An open region that is already at EOF yields an empty, non-nil slice, and a
// bounded inner limit still caps an open outer region.
func TestRegion_DecodeRemainingOpenEdges(t *testing.T) {
	t.Run("empty open region", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(nil), 8, 0)
		dec.PushOpenLimit()
		got, err := dec.DecodeRemaining(-1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("got %v, want an empty non-nil slice", got)
		}
	})

	t.Run("bounded inner limit inside an open outer region", func(t *testing.T) {
		data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
		dec.PushOpenLimit()
		dec.PushLimit(3)
		got, err := dec.DecodeRemaining(-1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !bytes.Equal(got, data[:3]) {
			t.Fatalf("got %v, want %v", got, data[:3])
		}
		if diff := dec.PopLimit(); diff != 0 {
			t.Fatalf("PopLimit = %d, want 0", diff)
		}
		rest, err := dec.DecodeRemaining(-1)
		if err != nil || !bytes.Equal(rest, data[3:]) {
			t.Fatalf("rest = %v, err %v", rest, err)
		}
	})

	t.Run("read error mid-region", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: make([]byte, 32), errAfter: 4, err: want}, 8, 0)
		dec.PushOpenLimit()
		if _, err := dec.DecodeRemaining(-1); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}

// DecodeBytesBuf(-1) means "the whole region", which for an open region means
// reading to EOF.
func TestRegion_DecodeBytesBufWholeRegion(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}

	open := NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
	open.PushOpenLimit()
	got, err := open.DecodeBytesBuf(-1)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("open: got %v, err %v", got, err)
	}

	known := NewStreamDecoder(bytes.NewReader(data), len(data), 4)
	got, err = known.DecodeBytesBuf(-1)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("known: got %v, err %v", got, err)
	}
}

// FinishRegion and FinishStream must propagate a probe failure rather than
// masking it as a trailing-data error.
func TestRegion_FinishPropagatesReadErrors(t *testing.T) {
	want := errors.New("boom")

	dec := NewUnknownStreamDecoder(&errReader{data: nil, errAfter: 0, err: want}, 8, 0)
	dec.PushOpenLimit()
	if err := FinishRegion(dec); !errors.Is(err, want) {
		t.Fatalf("FinishRegion err = %v, want %v", err, want)
	}
	if len(dec.limits) != 0 {
		t.Fatal("FinishRegion left the limit pushed after a probe failure")
	}

	dec2 := NewUnknownStreamDecoder(&errReader{data: nil, errAfter: 0, err: want}, 8, 0)
	if err := FinishStream(dec2); !errors.Is(err, want) {
		t.Fatalf("FinishStream err = %v, want %v", err, want)
	}
}

// A bounded region that was not fully consumed reports the unread remainder.
func TestRegion_FinishRegionBoundedTrailing(t *testing.T) {
	dec := NewBufferDecoder([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	dec.PushLimit(8)
	if _, err := dec.DecodeUint32(); err != nil {
		t.Fatal(err)
	}
	err := FinishRegion(dec)
	if !errors.Is(err, ErrOffset) {
		t.Fatalf("err = %v, want a trailing-data error", err)
	}
	if len(dec.limits) != 0 {
		t.Fatal("FinishRegion left the limit pushed")
	}
}

// BufferDecoder's region helpers: the negative-length clamp and PushOpenLimit.
func TestRegion_BufferDecoderEdges(t *testing.T) {
	dec := NewBufferDecoder([]byte{1, 2, 3, 4})
	dec.PushLimit(2)
	dec.position = 4 // consume past the limit, as a malformed decode could
	got, err := dec.DecodeRemaining(-1)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, err %v", got, err)
	}

	dec2 := NewBufferDecoder([]byte{1, 2, 3, 4})
	dec2.PushOpenLimit()
	if !dec2.LengthKnown() {
		t.Fatal("a buffer decoder always knows its length")
	}
	if got := dec2.GetLength(); got != 4 {
		t.Fatalf("GetLength = %d, want 4", got)
	}
}

// The remaining defensive paths: an overflowing limit, a zero-length inner
// region inside an open one, and a position pushed past its limit.
func TestRegion_DefensiveClamps(t *testing.T) {
	t.Run("PushLimit overflow", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 32)), 8, 64)
		dec.PushOpenLimit()
		if _, err := dec.DecodeBytesBuf(4); err != nil {
			t.Fatal(err)
		}
		// position is now non-zero, so this addition wraps.
		dec.PushLimit(math.MaxInt)
		if got := dec.GetLength(); got < 0 {
			t.Fatalf("GetLength = %d, want non-negative after an overflowing limit", got)
		}
		if !dec.LengthKnown() {
			t.Fatal("an overflowing limit should clamp to a bounded region")
		}
	})

	t.Run("zero-length inner region", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader([]byte{1, 2, 3, 4}), 8, 0)
		dec.PushOpenLimit()
		if more, err := dec.More(); err != nil || !more {
			t.Fatalf("More = %v, %v", more, err)
		}
		dec.PushLimit(0) // buffered data present, but this region admits none
		got, err := dec.DecodeRemaining(-1)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
		if diff := dec.PopLimit(); diff != 0 {
			t.Fatalf("PopLimit = %d, want 0", diff)
		}
	})

	t.Run("position past the limit", func(t *testing.T) {
		dec := NewStreamDecoder(bytes.NewReader(make([]byte, 16)), 16, 8)
		dec.PushLimit(4)
		dec.position = 9 // as a mis-paired limit stack could leave it
		got, err := dec.DecodeRemaining(-1)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
}

// SkipBytes and DecodeOffsetAt are not supported on a stream and must be inert
// rather than corrupting the read position.
func TestRegion_StreamSeekOpsAreInert(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader([]byte{1, 2, 3, 4}), 8, 0)
	dec.PushOpenLimit()
	before := dec.GetPosition()
	dec.SkipBytes(3)
	if dec.GetPosition() != before {
		t.Fatalf("SkipBytes moved the position from %d to %d", before, dec.GetPosition())
	}
	if got := dec.DecodeOffsetAt(0); got != 0 {
		t.Fatalf("DecodeOffsetAt = %d, want 0", got)
	}
	got, err := dec.DecodeRemaining(-1)
	if err != nil || !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("got %v, err %v", got, err)
	}
}
