// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// These helpers are called from generated code, so they are only reached
// indirectly from another package's tests. Exercising them directly keeps both
// regimes — known region length and open region — honest.

func TestGrowSlice(t *testing.T) {
	// A negative size yields an empty, non-nil slice rather than panicking.
	if got := GrowSlice([]int(nil), -1); got == nil || len(got) != 0 {
		t.Fatalf("GrowSlice(nil, -1) = %v, want empty non-nil", got)
	}

	// Growth is geometric, so repeated single-element growth must not
	// reallocate every time.
	s := GrowSlice([]int(nil), 1)
	if len(s) != 1 {
		t.Fatalf("len = %d, want 1", len(s))
	}
	if cap(s) < 8 {
		t.Fatalf("cap = %d, want at least the 8-element floor", cap(s))
	}
	firstCap := cap(s)
	for i := 2; i <= firstCap; i++ {
		s = GrowSlice(s, i)
	}
	if cap(s) != firstCap {
		t.Fatalf("cap changed to %d while growing within capacity", cap(s))
	}
	s[0] = 42
	s = GrowSlice(s, firstCap+1)
	if cap(s) < 2*firstCap {
		t.Fatalf("cap = %d, want at least %d (geometric growth)", cap(s), 2*firstCap)
	}
	if s[0] != 42 {
		t.Fatal("growth lost existing elements")
	}

	// Shrinking re-slices in place; re-growing must zero the reused tail so a
	// short decode cannot expose the previous contents.
	s = GrowSlice(s, 4)
	for i := range s {
		s[i] = 7
	}
	s = GrowSlice(s, 2)
	s = GrowSlice(s, 4)
	if s[2] != 0 || s[3] != 0 {
		t.Fatalf("regrown tail = %v, want zeroed", s[2:4])
	}
}

func TestDecodeByteListInto(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	t.Run("known-length reuses destination", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		dst := make([]byte, 0, 64)
		got, err := DecodeByteListInto(dec, dst, -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("got %v, want %v", got, data)
		}
		if &got[0] != &dst[:1][0] {
			t.Fatal("destination was not reused")
		}
	})

	t.Run("known-length rejects over max before allocating", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		if _, err := DecodeByteListInto(dec, nil, 4); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("known-length empty region", func(t *testing.T) {
		dec := NewBufferDecoder(nil)
		got, err := DecodeByteListInto(dec, nil, -1)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("known-length short read", func(t *testing.T) {
		dec := NewStreamDecoder(&shortReader{data: data[:4], maxRead: 2}, len(data), 8)
		if _, err := DecodeByteListInto(dec, nil, -1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("open region reads to EOF", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		got, err := DecodeByteListInto(dec, nil, -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("got %v, want %v", got, data)
		}
	})

	t.Run("open region reports over-max as a list error", func(t *testing.T) {
		// The cap is applied while reading, so the payload is never allocated,
		// but the error still has to read as a list-limit violation rather than
		// a stream-size one.
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeByteListInto(dec, nil, 4); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("open region surfaces read errors", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: data, errAfter: 2, err: want}, 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeByteListInto(dec, nil, -1); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}

func TestDecodeUint64ListInto(t *testing.T) {
	data := make([]byte, 24)
	for i, v := range []uint64{1, 2, 3} {
		for b := range 8 {
			data[i*8+b] = byte(v >> (8 * b))
		}
	}

	t.Run("known-length", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		got, err := DecodeUint64ListInto(dec, []uint64(nil), -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("known-length misaligned", func(t *testing.T) {
		dec := NewBufferDecoder(data[:20])
		// Misalignment is reported through the unexpected-EOF sentinel.
		_, err := DecodeUint64ListInto(dec, []uint64(nil), -1)
		if !errors.Is(err, ErrUnexpectedEOF) || !strings.Contains(err.Error(), "not a multiple") {
			t.Fatalf("err = %v, want a list-alignment error", err)
		}
	})

	t.Run("known-length over max", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), 2); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("known-length short read", func(t *testing.T) {
		dec := NewStreamDecoder(bytes.NewReader(data[:8]), len(data), 8)
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), -1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("open region reads to EOF", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		got, err := DecodeUint64ListInto(dec, []uint64(nil), -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("open region enforces max per element", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), 2); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("open region rejects a trailing partial element", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data[:20]), 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), -1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("open region surfaces read errors", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: data, errAfter: 2, err: want}, 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), -1); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})

	t.Run("named uint64 element type", func(t *testing.T) {
		type slot uint64
		dec := NewBufferDecoder(data)
		got, err := DecodeUint64ListInto(dec, []slot(nil), -1)
		if err != nil || len(got) != 3 || got[1] != slot(2) {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
}

func TestDecodeDelegateBuffer(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	t.Run("exact size", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		got, err := DecodeDelegateBuffer(dec, 4)
		if err != nil || !bytes.Equal(got, data[:4]) {
			t.Fatalf("got %v, err %v", got, err)
		}
		if dec.GetPosition() != 4 {
			t.Fatalf("position = %d, want 4", dec.GetPosition())
		}
	})

	t.Run("exact size past the end", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		if _, err := DecodeDelegateBuffer(dec, 99); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("whole known region", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		got, err := DecodeDelegateBuffer(dec, -1)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("whole open region", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		got, err := DecodeDelegateBuffer(dec, -1)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
}

func TestRegionEmpty(t *testing.T) {
	t.Run("bounded", func(t *testing.T) {
		dec := NewBufferDecoder([]byte{1})
		if empty, err := RegionEmpty(dec); err != nil || empty {
			t.Fatalf("empty = %v, err = %v", empty, err)
		}
		if _, err := dec.DecodeUint8(); err != nil {
			t.Fatal(err)
		}
		if empty, err := RegionEmpty(dec); err != nil || !empty {
			t.Fatalf("empty = %v, err = %v", empty, err)
		}
	})

	t.Run("open region at EOF", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(nil), 8, 0)
		dec.PushOpenLimit()
		if empty, err := RegionEmpty(dec); err != nil || !empty {
			t.Fatalf("empty = %v, err = %v", empty, err)
		}
	})

	t.Run("open region surfaces read errors", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: nil, errAfter: 0, err: want}, 8, 0)
		dec.PushOpenLimit()
		if _, err := RegionEmpty(dec); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}

func TestIsStreamTooLarge(t *testing.T) {
	if !isStreamTooLarge(ErrStreamTooLargeFn(10)) {
		t.Fatal("constructed stream-too-large error not recognised")
	}
	if !isStreamTooLarge(ErrPayloadTooLargeFn(11, 10)) {
		t.Fatal("payload-too-large error not recognised")
	}
	// It must recognise the error through the path-annotating wrapper too.
	if !isStreamTooLarge(ErrorWithPath(ErrStreamTooLargeFn(10), "Field")) {
		t.Fatal("wrapped stream-too-large error not recognised")
	}
	if isStreamTooLarge(ErrUnexpectedEOF) {
		t.Fatal("unrelated error misrecognised")
	}
	if isStreamTooLarge(nil) {
		t.Fatal("nil misrecognised")
	}
}
