// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
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
	// Buffer size 4: write 2 bytes, then write 4 bytes to trigger flush+early return
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w, 4)

	enc.EncodeBytes([]byte{0x01, 0x02})       // fits in buffer
	enc.EncodeBytes([]byte{0x03, 0x04, 0x05}) // triggers flush, flush fails

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBytes_LargeDirectShortWrite(t *testing.T) {
	// Buffer size 4: writing 8 bytes goes to direct write path.
	// shortWriter writes fewer bytes than requested.
	w := &shortWriter{maxWrite: 3}
	enc := NewStreamEncoder(w, 4)

	enc.EncodeBytes(make([]byte, 8))

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
