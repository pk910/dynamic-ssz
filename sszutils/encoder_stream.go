// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"fmt"
	"io"
)

type StreamEncoder struct {
	writer   io.Writer
	position int
	buffer   []byte
	writeErr error
}

var _ Encoder = (*StreamEncoder)(nil)

func NewStreamEncoder(writer io.Writer) *StreamEncoder {
	return &StreamEncoder{
		writer: writer,
		buffer: make([]byte, 0, 32),
	}
}

func (e *StreamEncoder) CanSeek() bool {
	return false
}

func (e *StreamEncoder) GetPosition() int {
	return e.position
}

func (e *StreamEncoder) GetBuffer() []byte {
	return e.buffer[:0]
}

func (e *StreamEncoder) SetBuffer(buffer []byte) {
	e.buffer = buffer
	e.EncodeBytes(buffer)
}

func (e *StreamEncoder) EncodeBool(v bool) {
	buf := []byte{0x00}
	if v {
		buf[0] = 0x01
	}
	written, err := e.writer.Write(buf)
	if err != nil {
		e.writeErr = err
	}
	if written != 1 {
		e.writeErr = fmt.Errorf("expected to write 1 byte, wrote %d", written)
	}
	e.position++
}

func (e *StreamEncoder) EncodeUint8(v uint8) {
	written, err := e.writer.Write([]byte{v})
	if err != nil {
		e.writeErr = err
	}
	if written != 1 {
		e.writeErr = fmt.Errorf("expected to write 1 byte, wrote %d", written)
	}
	e.position++
}

func (e *StreamEncoder) EncodeUint16(v uint16) {
	written, err := e.writer.Write(binary.LittleEndian.AppendUint16(e.buffer[:0], v))
	if err != nil {
		e.writeErr = err
	}
	if written != 2 {
		e.writeErr = fmt.Errorf("expected to write 2 bytes, wrote %d", written)
	}
	e.position += 2
}

func (e *StreamEncoder) EncodeUint32(v uint32) {
	written, err := e.writer.Write(binary.LittleEndian.AppendUint32(e.buffer[:0], v))
	if err != nil {
		e.writeErr = err
	}
	if written != 4 {
		e.writeErr = fmt.Errorf("expected to write 4 bytes, wrote %d", written)
	}
	e.position += 4
}

func (e *StreamEncoder) EncodeUint64(v uint64) {
	written, err := e.writer.Write(binary.LittleEndian.AppendUint64(e.buffer[:0], v))
	if err != nil {
		e.writeErr = err
	}
	if written != 8 {
		e.writeErr = fmt.Errorf("expected to write 8 bytes, wrote %d", written)
	}
	e.position += 8
}

func (e *StreamEncoder) EncodeBytes(v []byte) {
	written, err := e.writer.Write(v)
	if err != nil {
		e.writeErr = err
	}
	if written != len(v) {
		e.writeErr = fmt.Errorf("expected to write %d bytes, wrote %d", len(v), written)
	}
	e.position += len(v)
}

func (e *StreamEncoder) EncodeOffset(v uint32) {
	written, err := e.writer.Write(binary.LittleEndian.AppendUint32(e.buffer[:0], v))
	if err != nil {
		e.writeErr = err
	}
	if written != 4 {
		e.writeErr = fmt.Errorf("expected to write 4 bytes, wrote %d", written)
	}
	e.position += 4
}

func (e *StreamEncoder) EncodeOffsetAt(pos int, v uint32) {
	// not supported
	e.writeErr = fmt.Errorf("EncodeOffsetAt is not supported for stream encoder")
}

func (e *StreamEncoder) EncodeZeroPadding(n int) {
	zeroBytes := ZeroBytes()
	for n > 0 {
		buf := zeroBytes
		if n < 1024 {
			buf = buf[:n]
		}
		written, err := e.writer.Write(buf)
		if err != nil {
			e.writeErr = err
		}
		if written != len(buf) {
			e.writeErr = fmt.Errorf("expected to write %d bytes, wrote %d", len(buf), written)
		}
		e.position += len(buf)
		n -= len(buf)
	}
}

func (e *StreamEncoder) GetWriteError() error {
	return e.writeErr
}
