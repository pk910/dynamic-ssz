// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
)

// BufferEncoder is a seekable Encoder implementation backed by an in-memory
// byte buffer. It supports random-access offset writes via EncodeOffsetAt.
type BufferEncoder struct {
	buffer []byte
	pos    int
}

var _ Encoder = (*BufferEncoder)(nil)

// NewBufferEncoder creates a new BufferEncoder using the provided buffer.
// The buffer should have sufficient capacity for the expected output.
func NewBufferEncoder(buffer []byte) *BufferEncoder {
	// Use the full capacity for direct indexing
	return &BufferEncoder{
		buffer: buffer[:cap(buffer)],
		pos:    len(buffer),
	}
}

// Seekable returns true, indicating that BufferEncoder supports random-access
// offset writes via EncodeOffsetAt.
func (e *BufferEncoder) Seekable() bool {
	return true
}

// GetPosition returns the current write position in the buffer.
func (e *BufferEncoder) GetPosition() int {
	return e.pos
}

// GetBuffer returns the buffer contents written so far (from start to current position).
func (e *BufferEncoder) GetBuffer() []byte {
	return e.buffer[:e.pos]
}

// SetBuffer replaces the underlying buffer and resets the write position to the
// end of the provided buffer's length, using its full capacity for future writes.
func (e *BufferEncoder) SetBuffer(buffer []byte) {
	e.buffer = buffer[:cap(buffer)]
	e.pos = len(buffer)
}

// EncodeBool writes a single-byte boolean value (0x00 or 0x01) at the current
// position and advances by 1 byte.
func (e *BufferEncoder) EncodeBool(v bool) {
	if v {
		e.buffer[e.pos] = 0x01
	} else {
		e.buffer[e.pos] = 0x00
	}
	e.pos++
}

// EncodeUint8 writes a single byte at the current position and advances by 1 byte.
func (e *BufferEncoder) EncodeUint8(v uint8) {
	e.buffer[e.pos] = v
	e.pos++
}

// EncodeUint16 writes a little-endian uint16 at the current position and
// advances by 2 bytes.
func (e *BufferEncoder) EncodeUint16(v uint16) {
	binary.LittleEndian.PutUint16(e.buffer[e.pos:], v)
	e.pos += 2
}

// EncodeUint32 writes a little-endian uint32 at the current position and
// advances by 4 bytes.
func (e *BufferEncoder) EncodeUint32(v uint32) {
	binary.LittleEndian.PutUint32(e.buffer[e.pos:], v)
	e.pos += 4
}

// EncodeUint64 writes a little-endian uint64 at the current position and
// advances by 8 bytes.
func (e *BufferEncoder) EncodeUint64(v uint64) {
	binary.LittleEndian.PutUint64(e.buffer[e.pos:], v)
	e.pos += 8
}

// EncodeBytes copies the given byte slice into the buffer at the current
// position and advances by len(v) bytes.
func (e *BufferEncoder) EncodeBytes(v []byte) {
	copy(e.buffer[e.pos:], v)
	e.pos += len(v)
}

// EncodeOffset writes a 4-byte little-endian SSZ offset at the current position
// and advances by 4 bytes.
func (e *BufferEncoder) EncodeOffset(v uint32) {
	binary.LittleEndian.PutUint32(e.buffer[e.pos:], v)
	e.pos += 4
}

// EncodeOffsetAt writes a 4-byte little-endian SSZ offset at the given absolute
// position without advancing the current write position. This is used for
// back-patching offsets after dynamic-length fields have been written.
func (e *BufferEncoder) EncodeOffsetAt(pos int, v uint32) {
	binary.LittleEndian.PutUint32(e.buffer[pos:], v)
}

// EncodeZeroPadding writes n zero bytes at the current position and advances
// by n bytes. Uses clear for efficient zeroing.
func (e *BufferEncoder) EncodeZeroPadding(n int) {
	// Use clear for efficient zeroing (Go 1.21+)
	clear(e.buffer[e.pos : e.pos+n])
	e.pos += n
}
