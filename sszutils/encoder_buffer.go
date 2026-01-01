// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
)

type BufferEncoder struct {
	buffer []byte
}

var _ Encoder = (*BufferEncoder)(nil)

func NewBufferEncoder(buffer []byte) *BufferEncoder {
	return &BufferEncoder{
		buffer: buffer,
	}
}

func (e *BufferEncoder) Seekable() bool {
	return true
}

func (e *BufferEncoder) GetPosition() int {
	return len(e.buffer)
}

func (e *BufferEncoder) GetBuffer() []byte {
	return e.buffer
}

func (e *BufferEncoder) SetBuffer(buffer []byte) {
	e.buffer = buffer
}

func (e *BufferEncoder) EncodeBool(v bool) {
	if v {
		e.buffer = append(e.buffer, byte(0x01))
	} else {
		e.buffer = append(e.buffer, byte(0x00))
	}
}

func (e *BufferEncoder) EncodeUint8(v uint8) {
	e.buffer = append(e.buffer, v)
}

func (e *BufferEncoder) EncodeUint16(v uint16) {
	e.buffer = binary.LittleEndian.AppendUint16(e.buffer, v)
}

func (e *BufferEncoder) EncodeUint32(v uint32) {
	e.buffer = binary.LittleEndian.AppendUint32(e.buffer, v)
}

func (e *BufferEncoder) EncodeUint64(v uint64) {
	e.buffer = binary.LittleEndian.AppendUint64(e.buffer, v)
}

func (e *BufferEncoder) EncodeBytes(v []byte) {
	e.buffer = append(e.buffer, v...)
}

func (e *BufferEncoder) EncodeOffset(v uint32) {
	e.buffer = binary.LittleEndian.AppendUint32(e.buffer, v)
}

func (e *BufferEncoder) EncodeOffsetAt(pos int, v uint32) {
	binary.LittleEndian.PutUint32(e.buffer[pos:], v)
}

func (e *BufferEncoder) EncodeZeroPadding(n int) {
	e.buffer = AppendZeroPadding(e.buffer, n)
}
