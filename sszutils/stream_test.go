// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"testing/iotest"
)

// errWriter is a writer that returns an error after writing a specified number of bytes.
// When errAfter is reached, it returns the full write length but with an error.
type errWriter struct {
	errAfter int
	written  int
	err      error
}

func (w *errWriter) Write(p []byte) (n int, err error) {
	if w.written >= w.errAfter {
		// Return full length with error to avoid triggering short write check
		return len(p), w.err
	}
	remaining := w.errAfter - w.written
	if len(p) <= remaining {
		w.written += len(p)
		return len(p), nil
	}
	w.written += remaining
	// Return remaining bytes written with error
	return remaining, w.err
}

// shortWriter is a writer that writes fewer bytes than requested.
type shortWriter struct {
	maxWrite int
}

func (w *shortWriter) Write(p []byte) (n int, err error) {
	if len(p) <= w.maxWrite {
		return len(p), nil
	}
	return w.maxWrite, nil
}

// errReader is a reader that returns an error after reading a specified number of bytes.
type errReader struct {
	data     []byte
	pos      int
	errAfter int
	err      error
}

func (r *errReader) Read(p []byte) (n int, err error) {
	if r.pos >= r.errAfter {
		return 0, r.err
	}
	remaining := r.errAfter - r.pos
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}
	if toRead > len(r.data)-r.pos {
		toRead = len(r.data) - r.pos
	}
	copy(p, r.data[r.pos:r.pos+toRead])
	r.pos += toRead
	if r.pos >= r.errAfter {
		return toRead, r.err
	}
	return toRead, nil
}

// shortReader is a reader that reads fewer bytes than requested.
type shortReader struct {
	data    []byte
	pos     int
	maxRead int
}

func (r *shortReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	toRead := len(p)
	if toRead > r.maxRead {
		toRead = r.maxRead
	}
	if toRead > len(r.data)-r.pos {
		toRead = len(r.data) - r.pos
	}
	copy(p, r.data[r.pos:r.pos+toRead])
	r.pos += toRead
	return toRead, nil
}

// negativeReader violates the io.Reader contract by reporting a negative count.
type negativeReader struct{}

func (negativeReader) Read(_ []byte) (int, error) { return -1, nil }

// A reader returning a negative byte count must not drive the decode loops into
// a negative slice index; the decoder must report ErrNegativeRead instead.
func TestStreamDecoder_NegativeReadNoPanic(t *testing.T) {
	// Buffered read path (small read).
	t.Run("buffered", func(t *testing.T) {
		dec := NewStreamDecoder(negativeReader{}, 8, 1024)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("negative read panicked instead of erroring: %v", r)
			}
		}()
		if _, err := dec.DecodeUint64(); !errors.Is(err, ErrNegativeRead) {
			t.Errorf("expected ErrNegativeRead, got: %v", err)
		}
	})

	// Direct read path (read larger than the buffer).
	t.Run("direct", func(t *testing.T) {
		dec := NewStreamDecoder(negativeReader{}, 4096, 64)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("negative read panicked instead of erroring: %v", r)
			}
		}()
		if _, err := dec.DecodeBytes(make([]byte, 4096)); !errors.Is(err, ErrNegativeRead) {
			t.Errorf("expected ErrNegativeRead, got: %v", err)
		}
	})
}

// ============================================================================
// StreamEncoder Tests
// ============================================================================

func TestStreamEncoder_NewStreamEncoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 0)

	if enc == nil {
		t.Fatal("expected non-nil encoder")
	}
	if enc.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", enc.GetPosition())
	}
	if enc.Seekable() {
		t.Error("expected Seekable to be false")
	}
}

// TestStreamEncoder_MinBufferSize verifies that a buffer size smaller than the
// largest atomic write (uint64, 8 bytes) does not cause primitive encodes to
// write past the internal buffer and panic.
func TestStreamEncoder_MinBufferSize(t *testing.T) {
	for _, sz := range []int{1, 3, 4, 7} {
		var buf bytes.Buffer
		enc := NewStreamEncoder(&buf, sz)

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("buffer size %d caused a panic: %v", sz, r)
				}
			}()
			enc.EncodeUint64(0x0102030405060708)
			enc.EncodeUint32(0x0a0b0c0d)
			enc.EncodeOffset(0x11223344)
			enc.Flush()
		}()

		if enc.GetWriteError() != nil {
			t.Fatalf("buffer size %d unexpected write error: %v", sz, enc.GetWriteError())
		}
		if buf.Len() != 16 {
			t.Errorf("buffer size %d: expected 16 bytes, got %d", sz, buf.Len())
		}
	}
}

func TestStreamEncoder_GetBuffer(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 0)

	buffer := enc.GetBuffer()
	if buffer == nil {
		t.Error("expected non-nil buffer")
	}
	if len(buffer) != 0 {
		t.Errorf("expected empty buffer, got length %d", len(buffer))
	}
}

func TestStreamEncoder_SetBuffer(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 0)

	testData := []byte{0x01, 0x02, 0x03}
	enc.SetBuffer(testData)

	if enc.GetPosition() != 3 {
		t.Errorf("expected position 3, got %d", enc.GetPosition())
	}
	enc.Flush()
	if !bytes.Equal(buf.Bytes(), testData) {
		t.Errorf("expected %v, got %v", testData, buf.Bytes())
	}
	if enc.GetWriteError() != nil {
		t.Errorf("unexpected error: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBool_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeBool(true)
	enc.Flush()

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBool_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 0}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeBool(true)
	enc.Flush()

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint8_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint8(42)
	enc.Flush()

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint8_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 0}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint8(42)
	enc.Flush()

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint16_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint16(1000)
	enc.Flush()

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint16_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 1}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint16(1000)
	enc.Flush()

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint32_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint32(100000)
	enc.Flush()

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint32_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 3}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint32(100000)
	enc.Flush()

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint64_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint64(1000000000)
	enc.Flush()

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint64_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 7}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeUint64(1000000000)
	enc.Flush()

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBytes_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeBytes([]byte{0x01, 0x02, 0x03})
	enc.Flush()

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBytes_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 2}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeBytes([]byte{0x01, 0x02, 0x03})
	enc.Flush()

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeOffset_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeOffset(100)
	enc.Flush()

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeOffset_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 3}
	enc := NewStreamEncoder(w, 0)

	enc.EncodeOffset(100)
	enc.Flush()

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeOffsetAt_NotSupported(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 0)

	enc.EncodeOffsetAt(0, 100)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "not supported") {
		t.Errorf("expected 'not supported' error, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeZeroPadding_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 0)

	// Write enough to trigger a flush (> buffer size)
	enc.EncodeZeroPadding(DefaultStreamEncoderBufSize + 10)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeZeroPadding_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 5}
	enc := NewStreamEncoder(w, 0)

	// Write enough to trigger a flush
	enc.EncodeZeroPadding(DefaultStreamEncoderBufSize + 10)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeZeroPadding_LargeBuffer(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 0)

	enc.EncodeZeroPadding(2048)
	enc.Flush()

	if enc.GetWriteError() != nil {
		t.Errorf("unexpected error: %v", enc.GetWriteError())
	}
	if enc.GetPosition() != 2048 {
		t.Errorf("expected position 2048, got %d", enc.GetPosition())
	}
	if buf.Len() != 2048 {
		t.Errorf("expected buffer length 2048, got %d", buf.Len())
	}
	for i, b := range buf.Bytes() {
		if b != 0 {
			t.Errorf("expected zero at position %d, got %d", i, b)
		}
	}
}

func TestStreamEncoder_EncodeZeroPadding_LargeBuffer_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 1024, err: testErr}
	enc := NewStreamEncoder(w, 0)

	// Write enough to trigger multiple flushes
	enc.EncodeZeroPadding(DefaultStreamEncoderBufSize + 1024)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBool_Values(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected byte
	}{
		{"true", true, 0x01},
		{"false", false, 0x00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewStreamEncoder(&buf, 0)

			enc.EncodeBool(tt.value)
			enc.Flush()

			if enc.GetWriteError() != nil {
				t.Errorf("unexpected error: %v", enc.GetWriteError())
			}
			if enc.GetPosition() != 1 {
				t.Errorf("expected position 1, got %d", enc.GetPosition())
			}
			if len(buf.Bytes()) != 1 {
				t.Fatalf("expected 1 byte, got %d", len(buf.Bytes()))
			}
			if buf.Bytes()[0] != tt.expected {
				t.Errorf("expected %x, got %x", tt.expected, buf.Bytes()[0])
			}
		})
	}
}

// ============================================================================
// StreamDecoder Tests
// ============================================================================

func TestStreamDecoder_NewStreamDecoder(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01})
	dec := NewStreamDecoder(reader, 1, 0)

	if dec == nil {
		t.Fatal("expected non-nil decoder")
	}
	if dec.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", dec.GetPosition())
	}
	if dec.GetLength() != 1 {
		t.Errorf("expected length 1, got %d", dec.GetLength())
	}
	if dec.Seekable() {
		t.Error("expected Seekable to be false")
	}
}

func TestStreamDecoder_DecodeBool_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 1, 0)

	_, err := dec.DecodeBool()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeBool_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01}, maxRead: 0}
	dec := NewStreamDecoder(r, 1, 0)

	_, err := dec.DecodeBool()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBool_InvalidValue(t *testing.T) {
	reader := bytes.NewReader([]byte{0x02})
	dec := NewStreamDecoder(reader, 1, 0)

	_, err := dec.DecodeBool()

	if !errors.Is(err, ErrInvalidValueRange) {
		t.Errorf("expected ErrInvalidValueRange, got %v", err)
	}
}

func TestStreamDecoder_DecodeBool_ValidValues(t *testing.T) {
	tests := []struct {
		name     string
		input    byte
		expected bool
	}{
		{"true", 0x01, true},
		{"false", 0x00, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader([]byte{tt.input})
			dec := NewStreamDecoder(reader, 1, 0)

			result, err := dec.DecodeBool()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
			if dec.GetPosition() != 1 {
				t.Errorf("expected position 1, got %d", dec.GetPosition())
			}
		})
	}
}

func TestStreamDecoder_DecodeUint8_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 1, 0)

	_, err := dec.DecodeUint8()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint8_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01}, maxRead: 0}
	dec := NewStreamDecoder(r, 1, 0)

	_, err := dec.DecodeUint8()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeUint16_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 2, 0)

	_, err := dec.DecodeUint16()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint16_ShortRead(t *testing.T) {
	// Reader has only 1 byte but we need 2
	r := &shortReader{data: []byte{0x01}, maxRead: 1}
	dec := NewStreamDecoder(r, 2, 0)

	_, err := dec.DecodeUint16()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeUint32_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02, 0x03, 0x04}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 4, 0)

	_, err := dec.DecodeUint32()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint32_ShortRead(t *testing.T) {
	// Reader has only 3 bytes but we need 4
	r := &shortReader{data: []byte{0x01, 0x02, 0x03}, maxRead: 3}
	dec := NewStreamDecoder(r, 4, 0)

	_, err := dec.DecodeUint32()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeUint64_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: make([]byte, 8), errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 8, 0)

	_, err := dec.DecodeUint64()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint64_ShortRead(t *testing.T) {
	// Reader has only 7 bytes but we need 8
	r := &shortReader{data: make([]byte, 7), maxRead: 7}
	dec := NewStreamDecoder(r, 8, 0)

	_, err := dec.DecodeUint64()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytes_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02, 0x03}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 3, 0)

	buf := make([]byte, 3)
	_, err := dec.DecodeBytes(buf)

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeBytes_ShortRead(t *testing.T) {
	// Reader has only 2 bytes but we need 3
	r := &shortReader{data: []byte{0x01, 0x02}, maxRead: 2}
	dec := NewStreamDecoder(r, 3, 0)

	buf := make([]byte, 3)
	_, err := dec.DecodeBytes(buf)

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_LengthExceedsLimit(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	dec := NewStreamDecoder(reader, 3, 0)

	_, err := dec.DecodeBytesBuf(10)

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_NegativeLength(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	dec := NewStreamDecoder(reader, 3, 0)

	result, err := dec.DecodeBytesBuf(-1)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 bytes, got %d", len(result))
	}
	if !bytes.Equal(result, []byte{0x01, 0x02, 0x03}) {
		t.Errorf("expected [0x01, 0x02, 0x03], got %v", result)
	}
}

func TestStreamDecoder_DecodeBytesBuf_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02, 0x03}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 3, 0)

	_, err := dec.DecodeBytesBuf(3)

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_ShortRead(t *testing.T) {
	// Reader has only 2 bytes but we need 3
	r := &shortReader{data: []byte{0x01, 0x02}, maxRead: 2}
	dec := NewStreamDecoder(r, 3, 0)

	_, err := dec.DecodeBytesBuf(3)

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_BufferReuse(t *testing.T) {
	// First call with larger buffer
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	reader := bytes.NewReader(data)
	dec := NewStreamDecoder(reader, 6, 0)

	result1, err := dec.DecodeBytesBuf(4)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result1) != 4 {
		t.Errorf("expected 4 bytes, got %d", len(result1))
	}

	// Reset reader for second call with smaller buffer (reuse existing)
	reader = bytes.NewReader([]byte{0x07, 0x08})
	dec.reader = reader

	result2, err := dec.DecodeBytesBuf(2)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result2) != 2 {
		t.Errorf("expected 2 bytes, got %d", len(result2))
	}
}

func TestStreamDecoder_DecodeOffset_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02, 0x03, 0x04}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 4, 0)

	_, err := dec.DecodeOffset()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeOffset_ShortRead(t *testing.T) {
	// Reader has only 3 bytes but we need 4
	r := &shortReader{data: []byte{0x01, 0x02, 0x03}, maxRead: 3}
	dec := NewStreamDecoder(r, 4, 0)

	_, err := dec.DecodeOffset()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeOffsetAt_NotSupported(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03, 0x04})
	dec := NewStreamDecoder(reader, 4, 0)

	result := dec.DecodeOffsetAt(0)

	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

func TestStreamDecoder_SkipBytes_NotSupported(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	dec := NewStreamDecoder(reader, 3, 0)

	// SkipBytes does nothing but should not panic
	dec.SkipBytes(2)

	// Position should remain unchanged since SkipBytes is not supported
	if dec.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", dec.GetPosition())
	}
}

func TestStreamDecoder_PushLimit_ClampToLastLimit(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10, 0)

	// Push a limit that exceeds the stream length
	dec.PushLimit(20)

	// The limit should be clamped to the stream length
	if dec.GetLength() != 10 {
		t.Errorf("expected length 10, got %d", dec.GetLength())
	}
}

func TestStreamDecoder_PushLimit_NegativeClamped(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10, 0)

	// A negative limit clamps to zero (like BufferDecoder); without the clamp
	// GetLength() would go negative and poison downstream reads.
	dec.PushLimit(-5)

	if dec.GetLength() != 0 {
		t.Errorf("expected length 0, got %d", dec.GetLength())
	}

	dec.PopLimit()
	if dec.GetLength() != 10 {
		t.Errorf("expected length 10 after pop, got %d", dec.GetLength())
	}
}

func TestStreamDecoder_PopLimit_EmptyLimits(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10, 0)

	// Pop from empty limits
	remaining := dec.PopLimit()

	if remaining != 0 {
		t.Errorf("expected 0, got %d", remaining)
	}
}

func TestStreamDecoder_PopLimit_SingleLimit(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10, 0)

	dec.PushLimit(5)
	if dec.GetLength() != 5 {
		t.Errorf("expected length 5, got %d", dec.GetLength())
	}

	remaining := dec.PopLimit()

	if remaining != 5 {
		t.Errorf("expected remaining 5, got %d", remaining)
	}
	if dec.GetLength() != 10 {
		t.Errorf("expected length 10, got %d", dec.GetLength())
	}
}

func TestStreamDecoder_PopLimit_MultipleLimits(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10, 0)

	dec.PushLimit(8) // limit at position 8
	dec.PushLimit(3) // limit at position 3

	if dec.GetLength() != 3 {
		t.Errorf("expected length 3, got %d", dec.GetLength())
	}

	// Pop inner limit
	remaining := dec.PopLimit()
	if remaining != 3 {
		t.Errorf("expected remaining 3, got %d", remaining)
	}
	if dec.GetLength() != 8 {
		t.Errorf("expected length 8, got %d", dec.GetLength())
	}

	// Pop outer limit
	remaining = dec.PopLimit()
	if remaining != 8 {
		t.Errorf("expected remaining 8, got %d", remaining)
	}
	if dec.GetLength() != 10 {
		t.Errorf("expected length 10, got %d", dec.GetLength())
	}
}

func TestStreamDecoder_Uint16_Success(t *testing.T) {
	// Little endian: 0x0102 = 258
	reader := bytes.NewReader([]byte{0x02, 0x01})
	dec := NewStreamDecoder(reader, 2, 0)

	result, err := dec.DecodeUint16()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 258 {
		t.Errorf("expected 258, got %d", result)
	}
	if dec.GetPosition() != 2 {
		t.Errorf("expected position 2, got %d", dec.GetPosition())
	}
}

func TestStreamDecoder_Uint32_Success(t *testing.T) {
	// Little endian: 0x01020304 = 16909060
	reader := bytes.NewReader([]byte{0x04, 0x03, 0x02, 0x01})
	dec := NewStreamDecoder(reader, 4, 0)

	result, err := dec.DecodeUint32()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 16909060 {
		t.Errorf("expected 16909060, got %d", result)
	}
	if dec.GetPosition() != 4 {
		t.Errorf("expected position 4, got %d", dec.GetPosition())
	}
}

func TestStreamDecoder_Uint64_Success(t *testing.T) {
	// Little endian value
	reader := bytes.NewReader([]byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01})
	dec := NewStreamDecoder(reader, 8, 0)

	result, err := dec.DecodeUint64()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 0x0102030405060708 {
		t.Errorf("expected 0x0102030405060708, got 0x%x", result)
	}
	if dec.GetPosition() != 8 {
		t.Errorf("expected position 8, got %d", dec.GetPosition())
	}
}

func TestStreamDecoder_DecodeBytes_Success(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	dec := NewStreamDecoder(reader, 3, 0)

	buf := make([]byte, 3)
	result, err := dec.DecodeBytes(buf)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, []byte{0x01, 0x02, 0x03}) {
		t.Errorf("expected [0x01, 0x02, 0x03], got %v", result)
	}
	if dec.GetPosition() != 3 {
		t.Errorf("expected position 3, got %d", dec.GetPosition())
	}
}

func TestStreamDecoder_DecodeOffset_Success(t *testing.T) {
	reader := bytes.NewReader([]byte{0x04, 0x03, 0x02, 0x01})
	dec := NewStreamDecoder(reader, 4, 0)

	result, err := dec.DecodeOffset()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 16909060 {
		t.Errorf("expected 16909060, got %d", result)
	}
	if dec.GetPosition() != 4 {
		t.Errorf("expected position 4, got %d", dec.GetPosition())
	}
}

func TestStreamDecoder_DecodeUint8_Success(t *testing.T) {
	reader := bytes.NewReader([]byte{0x42})
	dec := NewStreamDecoder(reader, 1, 0)

	result, err := dec.DecodeUint8()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 0x42 {
		t.Errorf("expected 0x42, got 0x%x", result)
	}
	if dec.GetPosition() != 1 {
		t.Errorf("expected position 1, got %d", dec.GetPosition())
	}
}

func TestStreamEncoder_Position_Tracking(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 0)

	if enc.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", enc.GetPosition())
	}

	enc.EncodeBool(true)
	if enc.GetPosition() != 1 {
		t.Errorf("expected position 1, got %d", enc.GetPosition())
	}

	enc.EncodeUint8(0x42)
	if enc.GetPosition() != 2 {
		t.Errorf("expected position 2, got %d", enc.GetPosition())
	}

	enc.EncodeUint16(1000)
	if enc.GetPosition() != 4 {
		t.Errorf("expected position 4, got %d", enc.GetPosition())
	}

	enc.EncodeUint32(100000)
	if enc.GetPosition() != 8 {
		t.Errorf("expected position 8, got %d", enc.GetPosition())
	}

	enc.EncodeUint64(1000000000)
	if enc.GetPosition() != 16 {
		t.Errorf("expected position 16, got %d", enc.GetPosition())
	}

	enc.EncodeBytes([]byte{0x01, 0x02, 0x03})
	if enc.GetPosition() != 19 {
		t.Errorf("expected position 19, got %d", enc.GetPosition())
	}

	enc.EncodeOffset(100)
	if enc.GetPosition() != 23 {
		t.Errorf("expected position 23, got %d", enc.GetPosition())
	}

	enc.EncodeZeroPadding(5)
	if enc.GetPosition() != 28 {
		t.Errorf("expected position 28, got %d", enc.GetPosition())
	}

	enc.Flush()
	if enc.GetWriteError() != nil {
		t.Errorf("unexpected error: %v", enc.GetWriteError())
	}
	if buf.Len() != 28 {
		t.Errorf("expected buffer length 28, got %d", buf.Len())
	}
}

func TestStreamEncoder_EncodeZeroPadding_Zero(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 0)

	enc.EncodeZeroPadding(0)

	if enc.GetWriteError() != nil {
		t.Errorf("unexpected error: %v", enc.GetWriteError())
	}
	if enc.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", enc.GetPosition())
	}
	if buf.Len() != 0 {
		t.Errorf("expected buffer length 0, got %d", buf.Len())
	}
}

func TestStreamEncoder_TinyBuffer_FlushOnEncodeBool(t *testing.T) {
	var buf bytes.Buffer
	// Buffer size 1: first EncodeBool fills it, second triggers flush
	enc := NewStreamEncoder(&buf, 1)

	enc.EncodeBool(true)
	enc.EncodeBool(false)
	enc.Flush()

	if enc.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc.GetWriteError())
	}
	if enc.GetPosition() != 2 {
		t.Errorf("expected position 2, got %d", enc.GetPosition())
	}
	if buf.Len() != 2 {
		t.Errorf("expected 2 bytes written, got %d", buf.Len())
	}
	if buf.Bytes()[0] != 0x01 || buf.Bytes()[1] != 0x00 {
		t.Errorf("expected [0x01, 0x00], got %v", buf.Bytes())
	}
}

func TestStreamEncoder_TinyBuffer_FlushOnEncodeUint8(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 1)

	enc.EncodeUint8(0xAA)
	enc.EncodeUint8(0xBB)
	enc.Flush()

	if enc.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc.GetWriteError())
	}
	if buf.Len() != 2 {
		t.Errorf("expected 2 bytes, got %d", buf.Len())
	}
}

func TestStreamEncoder_TinyBuffer_FlushOnEncodeUint16(t *testing.T) {
	var buf bytes.Buffer
	// Buffer size 2: first uint16 fills it, second triggers flush
	enc := NewStreamEncoder(&buf, 2)

	enc.EncodeUint16(0x0102)
	enc.EncodeUint16(0x0304)
	enc.Flush()

	if enc.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc.GetWriteError())
	}
	if buf.Len() != 4 {
		t.Errorf("expected 4 bytes, got %d", buf.Len())
	}
}

func TestStreamEncoder_TinyBuffer_FlushOnEncodeUint32(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 4)

	enc.EncodeUint32(1)
	enc.EncodeUint32(2)
	enc.Flush()

	if enc.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc.GetWriteError())
	}
	if buf.Len() != 8 {
		t.Errorf("expected 8 bytes, got %d", buf.Len())
	}
}

func TestStreamEncoder_TinyBuffer_FlushOnEncodeUint64(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 8)

	enc.EncodeUint64(1)
	enc.EncodeUint64(2)
	enc.Flush()

	if enc.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc.GetWriteError())
	}
	if buf.Len() != 16 {
		t.Errorf("expected 16 bytes, got %d", buf.Len())
	}
}

func TestStreamEncoder_TinyBuffer_FlushOnEncodeOffset(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 4)

	enc.EncodeOffset(10)
	enc.EncodeOffset(20)
	enc.Flush()

	if enc.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc.GetWriteError())
	}
	if buf.Len() != 8 {
		t.Errorf("expected 8 bytes, got %d", buf.Len())
	}
}

func TestStreamEncoder_EncodeBytes_FlushError(t *testing.T) {
	testErr := errors.New("write error")
	// Buffer size 8 (the minimum): write 6 bytes, then 4 bytes to trigger flush+early return
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 8)

	enc.EncodeBytes(make([]byte, 6)) // fits in buffer
	enc.EncodeBytes(make([]byte, 4)) // triggers flush, flush fails

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBytes_LargeDirectWriteError(t *testing.T) {
	// Buffer size 8 (the minimum): writing 12 bytes goes to the direct write path.
	// errWriter writes the first 4 bytes then fails on the direct write.
	testErr := errors.New("disk full")
	w := &errWriter{errAfter: 4, err: testErr}
	enc := NewStreamEncoder(w, 8)

	enc.EncodeBytes(make([]byte, 12))

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBytes_LargeDirectShortWrite(t *testing.T) {
	// Buffer size 8 (the minimum): writing 12 bytes goes to the direct write path.
	// shortWriter writes fewer bytes than requested.
	w := &shortWriter{maxWrite: 3}
	enc := NewStreamEncoder(w, 8)

	enc.EncodeBytes(make([]byte, 12))

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected short write error, got: %v", enc.GetWriteError())
	}
}

// partialThenEOFReader returns data and EOF simultaneously when all data is consumed.
type partialThenEOFReader struct {
	data []byte
	pos  int
}

func (r *partialThenEOFReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	toRead := min(len(p), len(r.data)-r.pos)
	copy(p, r.data[r.pos:r.pos+toRead])
	r.pos += toRead
	if r.pos >= len(r.data) {
		return toRead, io.EOF
	}
	return toRead, nil
}

// zeroAfterNReader provides real data for the first N bytes, then returns 0 without error.
type zeroAfterNReader struct {
	data       []byte
	pos        int
	stallAfter int
}

func (r *zeroAfterNReader) Read(p []byte) (n int, err error) {
	if r.pos >= r.stallAfter {
		return 0, nil
	}
	toRead := min(len(p), r.stallAfter-r.pos)
	toRead = min(toRead, len(r.data)-r.pos)
	copy(p, r.data[r.pos:r.pos+toRead])
	r.pos += toRead
	return toRead, nil
}

func TestStreamDecoder_EnsureBuffered_BufferShift(t *testing.T) {
	// totalLen > DefaultStreamDecoderBufSize so buffer=2048. Prime buffer with a small
	// read, then consume most of it, leaving 4 bytes. Next uint64 needs 8,
	// triggering buffer shift since bufferPos > 0 and available < needed.
	totalLen := DefaultStreamDecoderBufSize + 100
	data := make([]byte, totalLen)
	for i := range data {
		data[i] = byte(i % 256)
	}
	reader := bytes.NewReader(data)
	dec := NewStreamDecoder(reader, totalLen, 0)

	// Prime the buffer (ensureBuffered fills it to 2048 bytes)
	_, err := dec.DecodeUint8()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Consume most of the buffer via the buffered path (available=2047 >= 2043)
	buf := make([]byte, DefaultStreamDecoderBufSize-5)
	_, err = dec.DecodeBytes(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Now bufferPos=2044, available=4. Reading uint64 needs 8, triggers shift.
	val, err := dec.DecodeUint64()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == 0 {
		t.Error("expected non-zero value")
	}
}

func TestStreamDecoder_EnsureBuffered_StreamExhausted(t *testing.T) {
	data := []byte{0x01, 0x02}
	reader := bytes.NewReader(data)
	dec := NewStreamDecoder(reader, 2, 0)

	_, err := dec.DecodeUint32()
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_EnsureBuffered_EOFWithEnoughData(t *testing.T) {
	data := []byte{0x04, 0x03, 0x02, 0x01}
	reader := &partialThenEOFReader{data: data}
	dec := NewStreamDecoder(reader, 4, 0)

	val, err := dec.DecodeUint32()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 0x01020304 {
		t.Errorf("expected 0x01020304, got 0x%x", val)
	}
}

func TestStreamDecoder_EnsureBuffered_EOFInsufficientData(t *testing.T) {
	data := []byte{0x01, 0x02}
	reader := &partialThenEOFReader{data: data}
	dec := NewStreamDecoder(reader, 8, 0)

	_, err := dec.DecodeUint64()
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_EnsureBuffered_ZeroReadReturnsEOF(t *testing.T) {
	data := []byte{0x42}
	reader := &zeroAfterNReader{data: data, stallAfter: 0}
	dec := NewStreamDecoder(reader, 1, 0)

	_, err := dec.DecodeUint8()
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_EnsureBuffered_NonEOFError(t *testing.T) {
	testErr := errors.New("network error")
	reader := &errReader{data: make([]byte, 8), errAfter: 0, err: testErr}
	dec := NewStreamDecoder(reader, 8, 0)

	_, err := dec.DecodeUint32()
	if !errors.Is(err, testErr) {
		t.Errorf("expected %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_ReadBytes_ExceedsStreamLength(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	reader := bytes.NewReader(data)
	dec := NewStreamDecoder(reader, 3, 0)

	_, err := dec.DecodeUint8()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf := make([]byte, 5)
	_, err = dec.DecodeBytes(buf)
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_ReadBytes_ZeroByteRead(t *testing.T) {
	data := make([]byte, 16)
	for i := range data {
		data[i] = byte(i)
	}
	stallReader := &zeroAfterNReader{data: data, stallAfter: 8}
	dec := NewStreamDecoder(stallReader, 16, 0)

	_, err := dec.DecodeUint64()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	largeBuf := make([]byte, 8)
	_, err = dec.DecodeBytes(largeBuf)
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_ReadBytes_DirectReadWithPartialReads(t *testing.T) {
	data := make([]byte, 30)
	for i := range data {
		data[i] = byte(i)
	}
	reader := &shortReader{data: data, maxRead: 3}
	dec := NewStreamDecoder(reader, 30, 0)

	_, err := dec.DecodeUint64()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf := make([]byte, 20)
	result, err := dec.DecodeBytes(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := range 20 {
		if result[i] != byte(i+8) {
			t.Errorf("byte %d: expected %d, got %d", i, i+8, result[i])
		}
	}
}

func TestStreamDecoder_ReadBytes_EOFDuringDirectRead(t *testing.T) {
	data := make([]byte, 12)
	reader := &errReader{data: data, errAfter: 10, err: io.EOF}
	dec := NewStreamDecoder(reader, 12, 0)

	_, err := dec.DecodeUint64()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf := make([]byte, 4)
	_, err = dec.DecodeBytes(buf)
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_ReadBytes_NonEOFErrorDuringDirectRead(t *testing.T) {
	totalLen := DefaultStreamDecoderBufSize + 500
	data := make([]byte, totalLen)
	testErr := errors.New("disk error")
	reader := &errReader{data: data, errAfter: DefaultStreamDecoderBufSize + 100, err: testErr}
	dec := NewStreamDecoder(reader, totalLen, 0)

	buf := make([]byte, DefaultStreamDecoderBufSize)
	_, err := dec.DecodeBytes(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	buf2 := make([]byte, 400)
	_, err = dec.DecodeBytes(buf2)
	if !errors.Is(err, testErr) {
		t.Errorf("expected %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_LargeBufferGrowth(t *testing.T) {
	size := DefaultStreamDecoderBufSize + 100
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	reader := bytes.NewReader(data)
	dec := NewStreamDecoder(reader, size, 0)

	result, err := dec.DecodeBytesBuf(size)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != size {
		t.Errorf("expected %d bytes, got %d", size, len(result))
	}
}

func TestStreamDecoder_DecodeBytesBuf_LargeGrowthDoubling(t *testing.T) {
	// buffer=2048 (DefaultStreamDecoderBufSize). Request l=5000 > 2*2048=4096,
	// so newSize = buffer*2 = 4096 < 5000, triggering newSize=l.
	size := DefaultStreamDecoderBufSize*2 + 500
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	reader := bytes.NewReader(data)
	dec := NewStreamDecoder(reader, size, 0)

	result, err := dec.DecodeBytesBuf(size)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != size {
		t.Errorf("expected %d bytes, got %d", size, len(result))
	}
}

func TestStreamDecoder_GetLength_WithLimits(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 100))
	dec := NewStreamDecoder(reader, 100, 0)

	if dec.GetLength() != 100 {
		t.Errorf("expected length 100, got %d", dec.GetLength())
	}

	dec.PushLimit(50)
	if dec.GetLength() != 50 {
		t.Errorf("expected length 50, got %d", dec.GetLength())
	}

	dec.PushLimit(30)
	if dec.GetLength() != 30 {
		t.Errorf("expected length 30, got %d", dec.GetLength())
	}

	dec.PopLimit()
	if dec.GetLength() != 50 {
		t.Errorf("expected length 50, got %d", dec.GetLength())
	}

	dec.PopLimit()
	if dec.GetLength() != 100 {
		t.Errorf("expected length 100, got %d", dec.GetLength())
	}
}

// TestStreamEncoderFlushOnSmallPrimitives covers the flush branch in EncodeUint8
// and EncodeUint16 when the buffered data leaves too little room.
func TestStreamEncoderFlushOnSmallPrimitives(t *testing.T) {
	// EncodeUint8 flush: fill the 8-byte buffer exactly, then write one more byte.
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 8)
	enc.EncodeBytes(make([]byte, 8)) // bufPos = 8
	enc.EncodeUint8(0xaa)            // 8+1 > 8 -> flush
	enc.Flush()
	if enc.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc.GetWriteError())
	}
	if buf.Len() != 9 {
		t.Fatalf("expected 9 bytes, got %d", buf.Len())
	}

	// EncodeUint16 flush: leave 7 bytes buffered, then write 2 bytes.
	var buf2 bytes.Buffer
	enc2 := NewStreamEncoder(&buf2, 8)
	enc2.EncodeBytes(make([]byte, 7)) // bufPos = 7
	enc2.EncodeUint16(0xbbcc)         // 7+2 > 8 -> flush
	enc2.Flush()
	if enc2.GetWriteError() != nil {
		t.Fatalf("unexpected error: %v", enc2.GetWriteError())
	}
	if buf2.Len() != 9 {
		t.Fatalf("expected 9 bytes, got %d", buf2.Len())
	}
}

// Reads must never cross the current region limit: a malformed region has to
// fail with a clean EOF error instead of consuming bytes of the following
// regions (which would make GetLength go negative downstream).
func TestStreamDecoder_ReadsRespectRegionLimit(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	newDec := func(regionLen int) *StreamDecoder {
		dec := NewStreamDecoder(bytes.NewReader(data), len(data), 0)
		dec.PushLimit(len(data))
		dec.PushLimit(regionLen)
		return dec
	}

	// readByte path
	dec := newDec(0)
	if _, err := dec.DecodeUint8(); err == nil {
		t.Error("DecodeUint8 crossed an exhausted region limit")
	}
	if dec.GetLength() < 0 {
		t.Errorf("GetLength went negative: %d", dec.GetLength())
	}

	// readBytesRef path
	dec = newDec(2)
	if _, err := dec.DecodeUint32(); err == nil {
		t.Error("DecodeUint32 crossed a 2-byte region limit")
	}

	// readBytes path
	dec = newDec(3)
	buf := make([]byte, 4)
	if _, err := dec.DecodeBytes(buf); err == nil {
		t.Error("DecodeBytes crossed a 3-byte region limit")
	}

	// reads within the region still work and stop exactly at the boundary
	dec = newDec(4)
	if v, err := dec.DecodeUint32(); err != nil || v != 0x04030201 {
		t.Errorf("DecodeUint32 within region failed: v=%x err=%v", v, err)
	}
	if _, err := dec.DecodeUint8(); err == nil {
		t.Error("DecodeUint8 crossed the region boundary after full consumption")
	}
	if diff := dec.PopLimit(); diff != 0 {
		t.Errorf("expected fully consumed region, diff=%d", diff)
	}
}

// After a write error, continued fixed-width encoding must not panic: flush()
// resets bufPos so subsequent Encode* land in-bounds and the error surfaces
// via GetWriteError instead of an index-out-of-range panic.
func TestStreamEncoder_NoPanicAfterWriteError(t *testing.T) {
	w := &errWriter{errAfter: 0, err: errors.New("write failed")}
	enc := NewStreamEncoder(w, 8)

	// Encode well past a full buffer's worth of fixed-width values; the first
	// flush fails, and every subsequent flush must keep bufPos in range.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("encoding panicked after write error: %v", r)
			}
		}()
		for i := 0; i < 100; i++ {
			enc.EncodeUint64(uint64(i))
		}
		enc.Flush()
	}()

	if enc.GetWriteError() == nil {
		t.Error("expected the write error to be recorded")
	}
}

// zeroInterleaveReader returns (0, nil) before every real read, exercising the
// io.Reader "0 and nil is a no-op, not EOF" contract.
type zeroInterleaveReader struct {
	r       io.Reader
	pending bool
}

func (z *zeroInterleaveReader) Read(p []byte) (int, error) {
	if !z.pending {
		z.pending = true
		return 0, nil
	}
	z.pending = false
	return z.r.Read(p)
}

// A reader that returns its final bytes together with io.EOF (permitted by the
// io.Reader contract) must not cause a spurious decode failure.
func TestStreamDecoder_ReadBytesDataWithEOF(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	// Small buffer forces the direct-read path in readBytes.
	dec := NewStreamDecoder(iotest.DataErrReader(bytes.NewReader(data)), len(data), 8)
	dec.PushLimit(len(data))

	buf := make([]byte, len(data))
	if _, err := dec.DecodeBytes(buf); err != nil {
		t.Fatalf("DecodeBytes rejected a data+EOF reader: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("got %x, want %x", buf, data)
	}
}

// EOF is the only error that may be consumed after a reader has supplied all
// requested bytes. Other terminal errors can be integrity verdicts from a
// checksumming, decompressing, or authenticated reader and must survive both
// the direct-read and buffered-fill paths.
func TestStreamDecoder_PreservesTerminalDataErrors(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	for name, want := range map[string]error{
		"integrity":      errors.New("checksum failed"),
		"unexpected EOF": io.ErrUnexpectedEOF,
	} {
		t.Run(name, func(t *testing.T) {
			t.Run("direct read", func(t *testing.T) {
				dec := NewStreamDecoder(
					&errReader{data: data, errAfter: len(data), err: want},
					len(data),
					8,
				)
				dec.PushLimit(len(data))

				got := make([]byte, len(data))
				_, err := dec.DecodeBytes(got)
				if !errors.Is(err, want) {
					t.Fatalf("DecodeBytes error = %v, want %v", err, want)
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("reader data was not processed before its error: got %x, want %x", got, data)
				}
			})

			t.Run("buffer fill", func(t *testing.T) {
				dec := NewUnknownStreamDecoder(
					&errReader{data: data, errAfter: len(data), err: want},
					64,
					64,
				)
				if err := dec.Prefill(); !errors.Is(err, want) {
					t.Fatalf("Prefill error = %v, want %v", err, want)
				}
			})
		})
	}
}

// A (0, nil) no-op read must be retried, not treated as EOF, on both the
// buffered (ensureBuffered) and direct (readBytes) paths.
func TestStreamDecoder_ZeroNilReadRetried(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	// readBytes path (DecodeBytes, small buffer).
	dec := NewStreamDecoder(&zeroInterleaveReader{r: bytes.NewReader(data)}, len(data), 8)
	dec.PushLimit(len(data))
	buf := make([]byte, len(data))
	if _, err := dec.DecodeBytes(buf); err != nil {
		t.Fatalf("DecodeBytes aborted on a (0,nil) read: %v", err)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("got %x, want %x", buf, data)
	}

	// ensureBuffered path (DecodeUint64).
	dec2 := NewStreamDecoder(&zeroInterleaveReader{r: bytes.NewReader(data)}, len(data), 8)
	dec2.PushLimit(len(data))
	if _, err := dec2.DecodeUint64(); err != nil {
		t.Fatalf("DecodeUint64 aborted on a (0,nil) read: %v", err)
	}
}

// zeroForeverReader always returns (0, nil), simulating a reader that never
// delivers data.
type zeroForeverReader struct{}

func (zeroForeverReader) Read(p []byte) (int, error) { return 0, nil }

// TestStreamDecoderInsufficientStream covers the stream-underflow guards: a
// buffer fill and a bulk read that exceed the declared stream length, and a
// reader stalled on (0, nil) that gives up after the retry bound.
func TestStreamDecoderInsufficientStream(t *testing.T) {
	dec := NewStreamDecoder(bytes.NewReader(make([]byte, 4)), 4, 1024)
	if err := dec.ensureBuffered(10); err != ErrUnexpectedEOF {
		t.Errorf("ensureBuffered past stream: err=%v, want ErrUnexpectedEOF", err)
	}

	dec = NewStreamDecoder(bytes.NewReader(make([]byte, 4)), 4, 1024)
	if err := dec.readBytes(make([]byte, 8)); err != ErrUnexpectedEOF {
		t.Errorf("readBytes past stream: err=%v, want ErrUnexpectedEOF", err)
	}

	dec = NewStreamDecoder(zeroForeverReader{}, 8, 1024)
	if err := dec.readBytes(make([]byte, 8)); err != ErrUnexpectedEOF {
		t.Errorf("readBytes on stalled reader: err=%v, want ErrUnexpectedEOF", err)
	}
}

// dripReader delivers one byte per Read, forcing readBytes to loop with partial
// (non-empty) reads.
type dripReader struct {
	data []byte
	pos  int
}

func (r *dripReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p[:1], r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestStreamDecoderPartialReads(t *testing.T) {
	dec := NewStreamDecoder(&dripReader{data: []byte{1, 2, 3, 4}}, 4, 1024)
	buf := make([]byte, 4)
	if err := dec.readBytes(buf); err != nil {
		t.Fatalf("readBytes with drip reader: %v", err)
	}
	if !bytes.Equal(buf, []byte{1, 2, 3, 4}) {
		t.Fatalf("drip read = %v, want [1 2 3 4]", buf)
	}
}

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
			if more, mErr := dec.More(); mErr != nil || more {
				t.Fatalf("More at region end: %v %v", more, mErr)
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
				if err := dec.FinishRegion(); err != nil {
					t.Fatalf("buf=%d FinishRegion: %v", bufSize, err)
				}
				if more, err := dec.More(); err != nil || more {
					t.Fatalf("buf=%d input not fully consumed: more=%v err=%v", bufSize, more, err)
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

	// A limit pushed while the total length is still unknown bounds reads but
	// is not a verified extent, so it must not report a known length.
	dec.PushLimit(10)
	if dec.LengthKnown() {
		t.Fatal("a limit derived inside an open region must not report a known length")
	}
	dec.PushOpenLimit() // open child of a bounded parent
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
	if err := dec.FinishRegion(); err != nil {
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
	if err := dec.FinishRegion(); !errors.Is(err, ErrOffset) {
		t.Fatalf("FinishRegion err = %v, want ErrOffset (trailing data)", err)
	}
}

// Closing the root region must report input the decode did not consume.
func TestRegion_RootRegionDetectsTrailing(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	dec := NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
	dec.PushOpenLimit()
	if _, err := dec.DecodeUint32(); err != nil {
		t.Fatalf("DecodeUint32: %v", err)
	}
	if err := dec.FinishRegion(); !errors.Is(err, ErrOffset) {
		t.Fatalf("FinishRegion err = %v, want ErrOffset (trailing data)", err)
	}

	dec = NewUnknownStreamDecoder(bytes.NewReader(data), 4, 0)
	dec.PushOpenLimit()
	if _, err := dec.DecodeUint64(); err != nil {
		t.Fatalf("DecodeUint64: %v", err)
	}
	if err := dec.FinishRegion(); err != nil {
		t.Fatalf("FinishRegion at EOF: %v", err)
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
				if err := dec.FinishRegion(); err != nil {
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
		if got := dec.GetLength(); got < 0 {
			t.Fatalf("GetLength = %d, want non-negative", got)
		}
		// Bounded for reading, but not a verified extent.
		if dec.LengthKnown() {
			t.Fatal("a limit derived inside an open region must not report a known length")
		}
	})

	t.Run("clamped to the allowance in an open region", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 8)), 8, 32)
		dec.PushOpenLimit()
		dec.PushLimit(1000) // far past the allowance
		if got := dec.GetLength(); got != 32 {
			t.Fatalf("GetLength = %d, want the 32-byte allowance", got)
		}
		if dec.LengthKnown() {
			t.Fatal("a limit clamped to the allowance is not a verified length")
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

// FinishRegion must propagate a probe failure rather than masking it as a
// trailing-data error.
func TestRegion_FinishPropagatesReadErrors(t *testing.T) {
	want := errors.New("boom")

	dec := NewUnknownStreamDecoder(&errReader{data: nil, errAfter: 0, err: want}, 8, 0)
	dec.PushOpenLimit()
	if err := dec.FinishRegion(); !errors.Is(err, want) {
		t.Fatalf("FinishRegion err = %v, want %v", err, want)
	}
	if len(dec.limits) != 0 {
		t.Fatal("FinishRegion left the limit pushed after a probe failure")
	}

	dec2 := NewUnknownStreamDecoder(&errReader{data: nil, errAfter: 0, err: want}, 8, 0)
	dec2.PushOpenLimit()
	if err := dec2.FinishRegion(); !errors.Is(err, want) {
		t.Fatalf("FinishRegion err = %v, want %v", err, want)
	}
}

// A bounded region that was not fully consumed reports the unread remainder.
func TestRegion_FinishRegionBoundedTrailing(t *testing.T) {
	dec := NewBufferDecoder([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	dec.PushLimit(8)
	if _, err := dec.DecodeUint32(); err != nil {
		t.Fatal(err)
	}
	err := dec.FinishRegion()
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
		// The limit bounds reads, but its extent is unverified: it was derived
		// inside an open region, so it must not be reported as a known length.
		if dec.LengthKnown() {
			t.Fatal("a limit derived inside an open region must not report a known length")
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

// EOF makes the stream length exact but leaves any region derived earlier from
// an offset in place. Such a region can end far past the real input, so it must
// not start reporting a known length just because EOF was seen.
func TestRegion_DerivedLimitStaysUnknownAfterEOF(t *testing.T) {
	dec := NewUnknownStreamDecoder(&partialThenEOFReader{data: make([]byte, 16)}, 8, 0)
	dec.PushOpenLimit()
	dec.PushLimit(500_000_000) // declared by a hostile offset, clamped to the allowance

	// Consume the whole real payload; the final read establishes EOF while the
	// declared region still claims ~500 MB.
	for i := range 2 {
		if _, err := dec.DecodeUint64(); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}
	if got := dec.GetLength(); got < 400_000_000 {
		t.Fatalf("premise: the declared region should still claim ~500 MB, got %d", got)
	}
	if dec.LengthKnown() {
		t.Fatalf("LengthKnown = true with GetLength = %d, but only 16 bytes were ever delivered", dec.GetLength())
	}
}

// A region that was declared bounded must be consumed to its limit. Running out
// of input first is truncated input, not a short result -- the preallocating
// branch of DecodeRemaining already reports it that way.
func TestRegion_DecodeRemainingRejectsShortDeclaredRegion(t *testing.T) {
	dec := NewUnknownStreamDecoder(&partialThenEOFReader{data: make([]byte, 16)}, 8, 0)
	dec.PushOpenLimit()
	dec.PushLimit(1024)

	if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrUnexpectedEOF) {
		t.Fatalf("DecodeRemaining = %v, want ErrUnexpectedEOF", err)
	}
}

// The maximum stream size is a hard bound. The decoder reads one byte past the
// allowance to establish EOF, and a reader may return that probe byte together
// with io.EOF -- which must not turn into an accepted max+1 byte payload.
func TestRegion_MaxStreamSizeIsHardBound(t *testing.T) {
	const maxSize = 32

	t.Run("exact fits", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(&partialThenEOFReader{data: make([]byte, maxSize)}, 64, maxSize)
		dec.PushOpenLimit()
		got, err := dec.DecodeRemaining(-1)
		if err != nil || len(got) != maxSize {
			t.Fatalf("a payload of exactly the maximum must decode: len %d, err %v", len(got), err)
		}
	})

	t.Run("one over, EOF with data", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(&partialThenEOFReader{data: make([]byte, maxSize+1)}, 64, maxSize)
		dec.PushOpenLimit()
		if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrStreamTooLarge) {
			t.Fatalf("DecodeRemaining = %v, want ErrStreamTooLarge", err)
		}
	})

	t.Run("one over, EOF separate", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(make([]byte, maxSize+1)), 64, maxSize)
		dec.PushOpenLimit()
		if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrStreamTooLarge) {
			t.Fatalf("DecodeRemaining = %v, want ErrStreamTooLarge", err)
		}
	})
}

// --- region / allowance edge branches ---

// TestStreamDecoder_UnknownMaxStreamSizeClamped covers the clamp of an oversized
// maxStreamSize down to maxAllowance in NewUnknownStreamDecoder.
func TestStreamDecoder_UnknownMaxStreamSizeClamped(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader(nil), 64, math.MaxInt)
	if dec.maxSize != maxAllowance {
		t.Fatalf("maxSize = %d, want clamp to maxAllowance %d", dec.maxSize, maxAllowance)
	}
}

// TestStreamDecoder_AvailableClampedToRegion covers Available() returning the
// remaining region length when fewer region bytes remain than are buffered.
func TestStreamDecoder_AvailableClampedToRegion(t *testing.T) {
	dec := NewStreamDecoder(bytes.NewReader([]byte{1, 2, 3, 4, 5, 6, 7, 8}), 8, 64)
	if err := dec.ensureBuffered(8); err != nil {
		t.Fatalf("ensureBuffered: %v", err)
	}
	dec.PushLimit(2) // region shorter than the 8 buffered bytes
	if got := dec.Available(); got != 2 {
		t.Fatalf("Available() = %d, want 2 (clamped to region limit)", got)
	}
}

// TestStreamDecoder_AvailableNegativeReturnsZero covers the avail < 0 guard in
// Available() when the position has run past the region limit.
func TestStreamDecoder_AvailableNegativeReturnsZero(t *testing.T) {
	dec := NewStreamDecoder(bytes.NewReader([]byte{1, 2}), 2, 64)
	dec.lastLimit = 0
	dec.position = 5 // room = lastLimit - position = -5
	if got := dec.Available(); got != 0 {
		t.Fatalf("Available() = %d, want 0 for a negative remaining region", got)
	}
}

// TestStreamDecoder_ReadMoreBeyondAllowance covers the pre-loop allowance guard
// in readMore for an unknown-length stream (remaining <= 0).
func TestStreamDecoder_ReadMoreBeyondAllowance(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader([]byte{1, 2, 3, 4}), 64, 4)
	dec.position = dec.maxSize + 1 // whole allowance already consumed
	if err := dec.readMore(); !errors.Is(err, ErrStreamTooLarge) {
		t.Fatalf("readMore() = %v, want ErrStreamTooLarge", err)
	}
}

// TestStreamDecoder_DecodeRemainingOpenMoreAtAllowance covers the DecodeRemaining
// branch where an open region reaches its allowance with bytes still buffered:
// More() reports more and the payload is rejected as too large.
func TestStreamDecoder_DecodeRemainingOpenMoreAtAllowance(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader(nil), 64, 100)
	dec.PushOpenLimit()
	// Open region, region limit already reached (room == 0), but bytes still
	// buffered so More() returns (true, nil) without touching the reader.
	dec.lastOpen = true
	dec.lastLimit = 0
	dec.position = 0
	dec.buffer = []byte{9, 9}
	dec.bufferLen = 2
	dec.bufferPos = 0
	if _, err := dec.DecodeRemaining(-1); !errors.Is(err, ErrStreamTooLarge) {
		t.Fatalf("DecodeRemaining() = %v, want ErrStreamTooLarge", err)
	}
}

// TestStreamDecoder_DecodeRemainingOpenEOFWithData covers the open-region break
// when EOF arrives together with the final data bytes: the buffered remainder is
// returned and the loop stops on the sticky-EOF path.
func TestStreamDecoder_DecodeRemainingOpenEOFWithData(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5}
	dec := NewUnknownStreamDecoder(&partialThenEOFReader{data: data}, 64, 1024)
	dec.PushOpenLimit()
	got, err := dec.DecodeRemaining(-1)
	if err != nil {
		t.Fatalf("DecodeRemaining: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("DecodeRemaining = %x, want %x", got, data)
	}
}

// TestStreamDecoder_DecodeRemainingOpenEOFSeenBreak covers the defensive
// sticky-EOF break inside DecodeRemaining's open-region loop: it fires only when
// EOF has been seen (eofSeen) while the region is still marked open with the
// position short of the limit and nothing buffered. onEOF normally collapses an
// open region to a bounded one, so this state is not reachable through the
// public API; the state is constructed directly to exercise the guard.
func TestStreamDecoder_DecodeRemainingOpenEOFSeenBreak(t *testing.T) {
	dec := NewUnknownStreamDecoder(bytes.NewReader(nil), 64, 1024)
	dec.PushOpenLimit()
	dec.lastOpen = true // open region => loop-captured openRegion == true
	dec.lastLimit = 10  // room = lastLimit - position = 10 > 0
	dec.position = 0
	dec.eofSeen = true // EOF already observed
	dec.bufferLen = 0  // available == 0
	dec.bufferPos = 0
	got, err := dec.DecodeRemaining(-1)
	if err != nil {
		t.Fatalf("DecodeRemaining: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("DecodeRemaining = %x, want empty", got)
	}
}

// A bool or an offset arriving with the write buffer already full must flush
// before encoding, like every other fixed-width write.
func TestStreamEncoderFlushBeforeBoolAndOffset(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf, 8)
	enc.EncodeUint64(0x0102030405060708)
	enc.EncodeBool(true)
	enc.Flush()
	if err := enc.GetWriteError(); err != nil {
		t.Fatalf("write error: %v", err)
	}
	want := []byte{8, 7, 6, 5, 4, 3, 2, 1, 1}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("encoded % x, want % x", buf.Bytes(), want)
	}

	buf.Reset()
	enc = NewStreamEncoder(&buf, 8)
	enc.EncodeUint64(0x0102030405060708)
	enc.EncodeOffset(0x11223344)
	enc.Flush()
	if err := enc.GetWriteError(); err != nil {
		t.Fatalf("write error: %v", err)
	}
	want = []byte{8, 7, 6, 5, 4, 3, 2, 1, 0x44, 0x33, 0x22, 0x11}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("encoded % x, want % x", buf.Bytes(), want)
	}
}
