// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
)

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

func (e *BufferEncoder) Seekable() bool {
	return true
}

func (e *BufferEncoder) GetPosition() int {
	return e.pos
}

func (e *BufferEncoder) GetBuffer() []byte {
	return e.buffer[:e.pos]
}

func (e *BufferEncoder) SetBuffer(buffer []byte) {
	e.buffer = buffer[:cap(buffer)]
	e.pos = len(buffer)
}

func (e *BufferEncoder) EncodeBool(v bool) {
	if v {
		e.buffer[e.pos] = 0x01
	} else {
		e.buffer[e.pos] = 0x00
	}
	e.pos++
}

func (e *BufferEncoder) EncodeUint8(v uint8) {
	e.buffer[e.pos] = v
	e.pos++
}

func (e *BufferEncoder) EncodeUint16(v uint16) {
	binary.LittleEndian.PutUint16(e.buffer[e.pos:], v)
	e.pos += 2
}

func (e *BufferEncoder) EncodeUint32(v uint32) {
	binary.LittleEndian.PutUint32(e.buffer[e.pos:], v)
	e.pos += 4
}

func (e *BufferEncoder) EncodeUint64(v uint64) {
	binary.LittleEndian.PutUint64(e.buffer[e.pos:], v)
	e.pos += 8
}

func (e *BufferEncoder) EncodeBytes(v []byte) {
	copy(e.buffer[e.pos:], v)
	e.pos += len(v)
}

func (e *BufferEncoder) EncodeOffset(v uint32) {
	binary.LittleEndian.PutUint32(e.buffer[e.pos:], v)
	e.pos += 4
}

func (e *BufferEncoder) EncodeOffsetAt(pos int, v uint32) {
	binary.LittleEndian.PutUint32(e.buffer[pos:], v)
}

func (e *BufferEncoder) EncodeZeroPadding(n int) {
	// Use clear for efficient zeroing (Go 1.21+)
	clear(e.buffer[e.pos : e.pos+n])
	e.pos += n
}
