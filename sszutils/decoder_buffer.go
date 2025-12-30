// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
)

type BufferDecoder struct {
	buffer    []byte
	limits    []int
	lastLimit int
	bufferLen int
	position  int
}

var _ Decoder = (*BufferDecoder)(nil)

func NewBufferDecoder(buffer []byte) *BufferDecoder {
	return &BufferDecoder{
		buffer:    buffer,
		limits:    make([]int, 0, 16),
		lastLimit: len(buffer),
		bufferLen: len(buffer),
		position:  0,
	}
}

func (e *BufferDecoder) CanSeek() bool {
	return true
}

func (e *BufferDecoder) GetPosition() int {
	return e.position
}

func (e *BufferDecoder) GetLength() int {
	return e.lastLimit - e.position
}

func (e *BufferDecoder) PushLimit(limit int) {
	limitPos := e.position + limit
	if limitPos > e.lastLimit {
		limitPos = e.lastLimit
	}

	e.limits = append(e.limits, limitPos)
	e.lastLimit = limitPos
}

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

func (e *BufferDecoder) DecodeUint8() (uint8, error) {
	if e.GetLength() < 1 {
		return 0, ErrUnexpectedEOF
	}
	val := e.buffer[e.position]
	e.position++
	return val, nil
}

func (e *BufferDecoder) DecodeUint16() (uint16, error) {
	if e.GetLength() < 2 {
		return 0, ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint16(e.buffer[e.position:])
	e.position += 2
	return val, nil
}

func (e *BufferDecoder) DecodeUint32() (uint32, error) {
	if e.GetLength() < 4 {
		return 0, ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint32(e.buffer[e.position:])
	e.position += 4
	return val, nil
}

func (e *BufferDecoder) DecodeUint64() (uint64, error) {
	if e.GetLength() < 8 {
		return 0, ErrUnexpectedEOF
	}
	val := binary.LittleEndian.Uint64(e.buffer[e.position:])
	e.position += 8
	return val, nil
}

func (e *BufferDecoder) DecodeBytes(buf []byte) ([]byte, error) {
	if e.GetLength() < len(buf) {
		return nil, ErrUnexpectedEOF
	}
	bufLen := len(buf)
	copy(buf, e.buffer[e.position:e.position+bufLen])
	e.position += bufLen
	return buf[:bufLen], nil
}

func (e *BufferDecoder) DecodeBytesBuf(len int) ([]byte, error) {
	limit := e.lastLimit
	if len < 0 {
		len = limit - e.position
	} else if limit-e.position < len {
		return nil, ErrUnexpectedEOF
	}
	buf := e.buffer[e.position : e.position+len]
	e.position += len
	return buf, nil
}

func (e *BufferDecoder) DecodeOffset() (uint32, error) {
	if e.GetLength() < 4 {
		return 0, ErrUnexpectedEOF
	}

	val := binary.LittleEndian.Uint32(e.buffer[e.position:])
	e.position += 4
	return val, nil
}

func (e *BufferDecoder) DecodeOffsetAt(pos int) uint32 {
	return binary.LittleEndian.Uint32(e.buffer[pos:])
}

func (e *BufferDecoder) SkipBytes(n int) {
	e.position += n
}
