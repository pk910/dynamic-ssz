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
	enc := NewStreamEncoder(&buf)

	if enc == nil {
		t.Fatal("expected non-nil encoder")
	}
	if enc.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", enc.GetPosition())
	}
	if enc.CanSeek() {
		t.Error("expected CanSeek to be false")
	}
}

func TestStreamEncoder_GetBuffer(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf)

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
	enc := NewStreamEncoder(&buf)

	testData := []byte{0x01, 0x02, 0x03}
	enc.SetBuffer(testData)

	if enc.GetPosition() != 3 {
		t.Errorf("expected position 3, got %d", enc.GetPosition())
	}
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
	enc := NewStreamEncoder(w)

	enc.EncodeBool(true)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBool_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 0}
	enc := NewStreamEncoder(w)

	enc.EncodeBool(true)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write 1 byte") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint8_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w)

	enc.EncodeUint8(42)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint8_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 0}
	enc := NewStreamEncoder(w)

	enc.EncodeUint8(42)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write 1 byte") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint16_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w)

	enc.EncodeUint16(1000)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint16_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 1}
	enc := NewStreamEncoder(w)

	enc.EncodeUint16(1000)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write 2 bytes") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint32_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w)

	enc.EncodeUint32(100000)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint32_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 3}
	enc := NewStreamEncoder(w)

	enc.EncodeUint32(100000)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write 4 bytes") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint64_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w)

	enc.EncodeUint64(1000000000)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeUint64_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 7}
	enc := NewStreamEncoder(w)

	enc.EncodeUint64(1000000000)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write 8 bytes") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBytes_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w)

	enc.EncodeBytes([]byte{0x01, 0x02, 0x03})

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeBytes_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 2}
	enc := NewStreamEncoder(w)

	enc.EncodeBytes([]byte{0x01, 0x02, 0x03})

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write 3 bytes") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeOffset_WriteError(t *testing.T) {
	testErr := errors.New("write error")
	w := &errWriter{errAfter: 0, err: testErr}
	enc := NewStreamEncoder(w)

	enc.EncodeOffset(100)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeOffset_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 3}
	enc := NewStreamEncoder(w)

	enc.EncodeOffset(100)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write 4 bytes") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeOffsetAt_NotSupported(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf)

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
	enc := NewStreamEncoder(w)

	enc.EncodeZeroPadding(10)

	if !errors.Is(enc.GetWriteError(), testErr) {
		t.Errorf("expected error %v, got %v", testErr, enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeZeroPadding_ShortWrite(t *testing.T) {
	w := &shortWriter{maxWrite: 5}
	enc := NewStreamEncoder(w)

	enc.EncodeZeroPadding(10)

	if enc.GetWriteError() == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(enc.GetWriteError().Error(), "expected to write") {
		t.Errorf("expected error about short write, got: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeZeroPadding_LargeBuffer(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf)

	enc.EncodeZeroPadding(2048)

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
	enc := NewStreamEncoder(w)

	enc.EncodeZeroPadding(2048)

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
			enc := NewStreamEncoder(&buf)

			enc.EncodeBool(tt.value)

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
	dec := NewStreamDecoder(reader, 1)

	if dec == nil {
		t.Fatal("expected non-nil decoder")
	}
	if dec.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", dec.GetPosition())
	}
	if dec.GetLength() != 1 {
		t.Errorf("expected length 1, got %d", dec.GetLength())
	}
	if dec.CanSeek() {
		t.Error("expected CanSeek to be false")
	}
}

func TestStreamDecoder_DecodeBool_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 1)

	_, err := dec.DecodeBool()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeBool_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01}, maxRead: 0}
	dec := NewStreamDecoder(r, 1)

	_, err := dec.DecodeBool()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBool_InvalidValue(t *testing.T) {
	reader := bytes.NewReader([]byte{0x02})
	dec := NewStreamDecoder(reader, 1)

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
			dec := NewStreamDecoder(reader, 1)

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
	dec := NewStreamDecoder(r, 1)

	_, err := dec.DecodeUint8()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint8_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01}, maxRead: 0}
	dec := NewStreamDecoder(r, 1)

	_, err := dec.DecodeUint8()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeUint16_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 2)

	_, err := dec.DecodeUint16()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint16_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01, 0x02}, maxRead: 1}
	dec := NewStreamDecoder(r, 2)

	_, err := dec.DecodeUint16()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeUint32_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02, 0x03, 0x04}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 4)

	_, err := dec.DecodeUint32()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint32_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01, 0x02, 0x03, 0x04}, maxRead: 3}
	dec := NewStreamDecoder(r, 4)

	_, err := dec.DecodeUint32()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeUint64_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: make([]byte, 8), errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 8)

	_, err := dec.DecodeUint64()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeUint64_ShortRead(t *testing.T) {
	r := &shortReader{data: make([]byte, 8), maxRead: 7}
	dec := NewStreamDecoder(r, 8)

	_, err := dec.DecodeUint64()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytes_ReadError(t *testing.T) {
	testErr := errors.New("read error")
	r := &errReader{data: []byte{0x01, 0x02, 0x03}, errAfter: 0, err: testErr}
	dec := NewStreamDecoder(r, 3)

	buf := make([]byte, 3)
	_, err := dec.DecodeBytes(buf)

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeBytes_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01, 0x02, 0x03}, maxRead: 2}
	dec := NewStreamDecoder(r, 3)

	buf := make([]byte, 3)
	_, err := dec.DecodeBytes(buf)

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_LengthExceedsLimit(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	dec := NewStreamDecoder(reader, 3)

	_, err := dec.DecodeBytesBuf(10)

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_NegativeLength(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	dec := NewStreamDecoder(reader, 3)

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
	dec := NewStreamDecoder(r, 3)

	_, err := dec.DecodeBytesBuf(3)

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01, 0x02, 0x03}, maxRead: 2}
	dec := NewStreamDecoder(r, 3)

	_, err := dec.DecodeBytesBuf(3)

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeBytesBuf_BufferReuse(t *testing.T) {
	// First call with larger buffer
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	reader := bytes.NewReader(data)
	dec := NewStreamDecoder(reader, 6)

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
	dec := NewStreamDecoder(r, 4)

	_, err := dec.DecodeOffset()

	if !errors.Is(err, testErr) {
		t.Errorf("expected error %v, got %v", testErr, err)
	}
}

func TestStreamDecoder_DecodeOffset_ShortRead(t *testing.T) {
	r := &shortReader{data: []byte{0x01, 0x02, 0x03, 0x04}, maxRead: 3}
	dec := NewStreamDecoder(r, 4)

	_, err := dec.DecodeOffset()

	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestStreamDecoder_DecodeOffsetAt_NotSupported(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03, 0x04})
	dec := NewStreamDecoder(reader, 4)

	result := dec.DecodeOffsetAt(0)

	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

func TestStreamDecoder_SkipBytes_NotSupported(t *testing.T) {
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	dec := NewStreamDecoder(reader, 3)

	// SkipBytes does nothing but should not panic
	dec.SkipBytes(2)

	// Position should remain unchanged since SkipBytes is not supported
	if dec.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", dec.GetPosition())
	}
}

func TestStreamDecoder_PushLimit_ClampToLastLimit(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10)

	// Push a limit that exceeds the stream length
	dec.PushLimit(20)

	// The limit should be clamped to the stream length
	if dec.GetLength() != 10 {
		t.Errorf("expected length 10, got %d", dec.GetLength())
	}
}

func TestStreamDecoder_PopLimit_EmptyLimits(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10)

	// Pop from empty limits
	remaining := dec.PopLimit()

	if remaining != 0 {
		t.Errorf("expected 0, got %d", remaining)
	}
}

func TestStreamDecoder_PopLimit_SingleLimit(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 10))
	dec := NewStreamDecoder(reader, 10)

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
	dec := NewStreamDecoder(reader, 10)

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
	dec := NewStreamDecoder(reader, 2)

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
	dec := NewStreamDecoder(reader, 4)

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
	dec := NewStreamDecoder(reader, 8)

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
	dec := NewStreamDecoder(reader, 3)

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
	dec := NewStreamDecoder(reader, 4)

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
	dec := NewStreamDecoder(reader, 1)

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
	enc := NewStreamEncoder(&buf)

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

	if enc.GetWriteError() != nil {
		t.Errorf("unexpected error: %v", enc.GetWriteError())
	}
}

func TestStreamEncoder_EncodeZeroPadding_Zero(t *testing.T) {
	var buf bytes.Buffer
	enc := NewStreamEncoder(&buf)

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

func TestStreamDecoder_GetLength_WithLimits(t *testing.T) {
	reader := bytes.NewReader(make([]byte, 100))
	dec := NewStreamDecoder(reader, 100)

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
