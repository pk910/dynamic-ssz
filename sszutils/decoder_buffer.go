// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
)

// BufferDecoder is a seekable Decoder implementation backed by an in-memory
// byte buffer. It supports random-access offset reads via DecodeOffsetAt and
// byte skipping via SkipBytes.
type BufferDecoder struct {
	buffer    []byte
	limits    []int
	lastLimit int
	bufferLen int
	position  int
}

var _ Decoder = (*BufferDecoder)(nil)

// NewBufferDecoder creates a new BufferDecoder that reads SSZ data from the
// provided byte buffer.
func NewBufferDecoder(buffer []byte) *BufferDecoder {
	return &BufferDecoder{
		buffer:    buffer,
		limits:    make([]int, 0, 16),
		lastLimit: len(buffer),
		bufferLen: len(buffer),
		position:  0,
	}
}

// Seekable returns true, indicating that BufferDecoder supports random-access
// offset reads via DecodeOffsetAt and byte skipping via SkipBytes.
func (e *BufferDecoder) Seekable() bool {
	return true
}

// GetPosition returns the current read position in the buffer.
func (e *BufferDecoder) GetPosition() int {
	return e.position
}

// GetLength returns the number of remaining bytes available for reading,
// taking into account the current limit.
func (e *BufferDecoder) GetLength() int {
	return e.lastLimit - e.position
}

// PushLimit restricts reading to the next limit bytes from the current position.
// If the limit extends beyond the enclosing limit, it is clamped. Limits can be
// nested and must be removed with PopLimit.
func (e *BufferDecoder) PushLimit(limit int) {
	limitPos := e.position + limit
	if limitPos > e.lastLimit {
		limitPos = e.lastLimit
	}

	e.limits = append(e.limits, limitPos)
	e.lastLimit = limitPos
}

// PopLimit removes the most recently pushed limit and returns the number of
// unconsumed bytes that were remaining within that limit.
func (e *BufferDecoder) PopLimit() int {
	limitsLen := len(e.limits)
	if limitsLen == 0 {
		return 0
	}
	limit := e.limits[limitsLen-1]
	if limitsLen <= 1 {
		e.lastLimit = e.bufferLen
	} else {
		e.lastLimit = e.limits[limitsLen-2]
	}
	e.limits = e.limits[:limitsLen-1]
	return limit - e.position
}

// DecodeBool reads a single byte and returns its boolean value. Returns
// ErrUnexpectedEOF if no bytes remain, or ErrInvalidValueRange if the byte
// is not 0x00 or 0x01.
func (e *BufferDecoder) DecodeBool() (bool, error) {
	if e.GetLength() < 1 {
		return false, ErrUnexpectedEOF
	}
	val := e.buffer[e.position]
	if val != 1 && val != 0 {
		return false, ErrInvalidValueRange
	}
	e.position++
	return val == 1, nil
}

// DecodeUint8 reads a single byte and returns it as uint8. Returns
// ErrUnexpectedEOF if no bytes remain.
func (e *BufferDecoder) DecodeUint8() (uint8, error) {
	if e.GetLength() < 1 {
		return 0, ErrUnexpectedEOF
	}
	val := e.buffer[e.position]
	e.position++
	return val, nil
}

// DecodeUint16 reads 2 bytes in little-endian order and returns a uint16.
// Returns ErrUnexpectedEOF if fewer than 2 bytes remain.
func (e *BufferDecoder) DecodeUint16() (uint16, error) {
	if e.GetLength() < 2 {
		return 0, ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint16(e.buffer[e.position:])
	e.position += 2
	return val, nil
}

// DecodeUint32 reads 4 bytes in little-endian order and returns a uint32.
// Returns ErrUnexpectedEOF if fewer than 4 bytes remain.
func (e *BufferDecoder) DecodeUint32() (uint32, error) {
	if e.GetLength() < 4 {
		return 0, ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint32(e.buffer[e.position:])
	e.position += 4
	return val, nil
}

// DecodeUint64 reads 8 bytes in little-endian order and returns a uint64.
// Returns ErrUnexpectedEOF if fewer than 8 bytes remain.
func (e *BufferDecoder) DecodeUint64() (uint64, error) {
	if e.GetLength() < 8 {
		return 0, ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint64(e.buffer[e.position:])
	e.position += 8
	return val, nil
}

// DecodeBytes reads len(buf) bytes into the provided buffer and returns the
// filled slice. Returns ErrUnexpectedEOF if fewer bytes remain than requested.
func (e *BufferDecoder) DecodeBytes(buf []byte) ([]byte, error) {
	if e.GetLength() < len(buf) {
		return nil, ErrUnexpectedEOF
	}
	bufLen := len(buf)
	copy(buf, e.buffer[e.position:e.position+bufLen])
	e.position += bufLen
	return buf[:bufLen], nil
}

// DecodeBytesBuf returns a slice of the underlying buffer containing the next
// length bytes. If length is negative, all remaining bytes within the current
// limit are returned. The returned slice shares memory with the decoder's
// buffer and must not be modified. Returns ErrUnexpectedEOF if fewer bytes
// remain than requested.
func (e *BufferDecoder) DecodeBytesBuf(length int) ([]byte, error) {
	limit := e.lastLimit
	if length < 0 {
		length = limit - e.position
	} else if limit-e.position < length {
		return nil, ErrUnexpectedEOF
	}
	buf := e.buffer[e.position : e.position+length]
	e.position += length
	return buf, nil
}

// DecodeOffset reads a 4-byte little-endian SSZ offset from the current
// position and advances by 4 bytes. Returns ErrUnexpectedEOF if fewer than
// 4 bytes remain.
func (e *BufferDecoder) DecodeOffset() (uint32, error) {
	if e.GetLength() < 4 {
		return 0, ErrUnexpectedEOF
	}

	val := binary.LittleEndian.Uint32(e.buffer[e.position:])
	e.position += 4
	return val, nil
}

// DecodeOffsetAt reads a 4-byte little-endian SSZ offset at the given absolute
// position without advancing the current read position. The caller must ensure
// pos is within bounds.
func (e *BufferDecoder) DecodeOffsetAt(pos int) uint32 {
	return binary.LittleEndian.Uint32(e.buffer[pos:])
}

// SkipBytes advances the read position by n bytes without reading the data.
func (e *BufferDecoder) SkipBytes(n int) {
	e.position += n
}
