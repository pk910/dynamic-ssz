// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"encoding/binary"
	"io"

	"github.com/pk910/dynamic-ssz/sszutils"
)

type StreamDecoder struct {
	reader    io.Reader
	limits    []int
	lastLimit int
	streamLen int
	position  int
	buffer    []byte
}

var _ sszutils.Decoder = (*StreamDecoder)(nil)

func NewStreamDecoder(reader io.Reader, totalLen int) *StreamDecoder {
	return &StreamDecoder{
		reader:    reader,
		limits:    make([]int, 0, 16),
		lastLimit: totalLen,
		streamLen: totalLen,
		position:  0,
	}
}

func (e *StreamDecoder) CanSeek() bool {
	return false
}

func (e *StreamDecoder) GetPosition() int {
	return e.position
}

func (e *StreamDecoder) GetLength() int {
	return e.lastLimit - e.position
}

func (e *StreamDecoder) PushLimit(limit int) {
	limitPos := e.position + limit
	if limitPos > e.lastLimit {
		limitPos = e.lastLimit
	}

	e.limits = append(e.limits, limitPos)
	e.lastLimit = limitPos
}

func (e *StreamDecoder) PopLimit() int {
	limitsLen := len(e.limits)
	if limitsLen == 0 {
		return 0
	}
	limit := e.limits[limitsLen-1]
	if limitsLen <= 1 {
		e.lastLimit = e.streamLen
	} else {
		e.lastLimit = e.limits[limitsLen-2]
	}
	e.limits = e.limits[:limitsLen-1]
	return limit - e.position
}

func (e *StreamDecoder) DecodeBool() (bool, error) {
	buf := [1]byte{}
	n, err := e.reader.Read(buf[:])
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, sszutils.ErrUnexpectedEOF
	}
	if buf[0] != 1 && buf[0] != 0 {
		return false, sszutils.ErrInvalidValueRange
	}
	e.position++
	return buf[0] == 1, nil
}

func (e *StreamDecoder) DecodeUint8() (uint8, error) {
	buf := [1]byte{}
	n, err := e.reader.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, sszutils.ErrUnexpectedEOF
	}
	e.position++
	return buf[0], nil
}

func (e *StreamDecoder) DecodeUint16() (uint16, error) {
	buf := [2]byte{}
	n, err := e.reader.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 2 {
		return 0, sszutils.ErrUnexpectedEOF
	}
	e.position += 2
	return binary.LittleEndian.Uint16(buf[:]), nil
}

func (e *StreamDecoder) DecodeUint32() (uint32, error) {
	buf := [4]byte{}
	n, err := e.reader.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 4 {
		return 0, sszutils.ErrUnexpectedEOF
	}
	e.position += 4
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func (e *StreamDecoder) DecodeUint64() (uint64, error) {
	buf := [8]byte{}
	n, err := e.reader.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 8 {
		return 0, sszutils.ErrUnexpectedEOF
	}
	e.position += 8
	return binary.LittleEndian.Uint64(buf[:]), nil
}

func (e *StreamDecoder) DecodeBytes(buf []byte) ([]byte, error) {
	n, err := e.reader.Read(buf)
	if err != nil {
		return nil, err
	}
	if n != len(buf) {
		return nil, sszutils.ErrUnexpectedEOF
	}
	e.position += n
	return buf[:n], nil
}

func (e *StreamDecoder) DecodeBytesBuf(l int) ([]byte, error) {
	limit := e.lastLimit
	if l < 0 {
		l = limit - e.position
	} else if limit-e.position < l {
		return nil, sszutils.ErrUnexpectedEOF
	}

	if len(e.buffer) < l {
		e.buffer = make([]byte, l)
	} else {
		e.buffer = e.buffer[:l]
	}

	n, err := e.reader.Read(e.buffer)
	if err != nil {
		return nil, err
	}
	if n != l {
		return nil, sszutils.ErrUnexpectedEOF
	}
	e.position += l
	return e.buffer[:n], nil
}

func (e *StreamDecoder) DecodeOffset() (uint32, error) {
	buf := [4]byte{}
	n, err := e.reader.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 4 {
		return 0, sszutils.ErrUnexpectedEOF
	}
	e.position += 4
	return binary.LittleEndian.Uint32(buf[:]), nil
}

func (e *StreamDecoder) DecodeOffsetAt(pos int) uint32 {
	// not supported
	return 0
}

func (e *StreamDecoder) SkipBytes(n int) {
	// not supported
}
