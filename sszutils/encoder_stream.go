// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"fmt"
	"io"
)

// DefaultStreamEncoderBufSize is the default internal write buffer size for
// StreamEncoder (2KB).
const DefaultStreamEncoderBufSize = 2 * 1024

// StreamEncoder is a non-seekable Encoder implementation that writes SSZ data
// to an io.Writer with internal buffering. It does not support EncodeOffsetAt.
type StreamEncoder struct {
	writer       io.Writer
	position     int
	scratch      []byte   // small scratch buffer for GetBuffer/SetBuffer
	scratchStack [32]byte // inline backing array for scratch
	writeBuf     []byte
	bufPos       int
	writeErr     error
}

var _ Encoder = (*StreamEncoder)(nil)

// NewStreamEncoder creates a new StreamEncoder that writes SSZ data to the
// provided io.Writer. bufSize controls the internal write buffer size; if <= 0,
// DefaultStreamEncoderBufSize is used.
func NewStreamEncoder(writer io.Writer, bufSize int) *StreamEncoder {
	if bufSize <= 0 {
		bufSize = DefaultStreamEncoderBufSize
	}
	e := &StreamEncoder{
		writer:   writer,
		writeBuf: make([]byte, bufSize),
	}
	e.scratch = e.scratchStack[:0]
	return e
}

func (e *StreamEncoder) Seekable() bool {
	return false
}

func (e *StreamEncoder) GetPosition() int {
	return e.position
}

func (e *StreamEncoder) GetBuffer() []byte {
	return e.scratch[:0]
}

func (e *StreamEncoder) SetBuffer(buffer []byte) {
	e.scratch = buffer
	e.EncodeBytes(buffer)
}

// flush writes the internal buffer to the underlying io.Writer.
func (e *StreamEncoder) flush() {
	if e.bufPos == 0 || e.writeErr != nil {
		return
	}
	written, err := e.writer.Write(e.writeBuf[:e.bufPos])
	if err != nil {
		e.writeErr = err
	} else if written != e.bufPos {
		e.writeErr = fmt.Errorf(
			"expected to write %d bytes, wrote %d", e.bufPos, written,
		)
	}
	e.bufPos = 0
}

// Flush flushes any buffered data to the underlying io.Writer.
func (e *StreamEncoder) Flush() {
	e.flush()
}

func (e *StreamEncoder) EncodeBool(v bool) {
	if e.bufPos+1 > len(e.writeBuf) {
		e.flush()
	}
	if v {
		e.writeBuf[e.bufPos] = 0x01
	} else {
		e.writeBuf[e.bufPos] = 0x00
	}
	e.bufPos++
	e.position++
}

func (e *StreamEncoder) EncodeUint8(v uint8) {
	if e.bufPos+1 > len(e.writeBuf) {
		e.flush()
	}
	e.writeBuf[e.bufPos] = v
	e.bufPos++
	e.position++
}

func (e *StreamEncoder) EncodeUint16(v uint16) {
	if e.bufPos+2 > len(e.writeBuf) {
		e.flush()
	}
	binary.LittleEndian.PutUint16(e.writeBuf[e.bufPos:], v)
	e.bufPos += 2
	e.position += 2
}

func (e *StreamEncoder) EncodeUint32(v uint32) {
	if e.bufPos+4 > len(e.writeBuf) {
		e.flush()
	}
	binary.LittleEndian.PutUint32(e.writeBuf[e.bufPos:], v)
	e.bufPos += 4
	e.position += 4
}

func (e *StreamEncoder) EncodeUint64(v uint64) {
	if e.bufPos+8 > len(e.writeBuf) {
		e.flush()
	}
	binary.LittleEndian.PutUint64(e.writeBuf[e.bufPos:], v)
	e.bufPos += 8
	e.position += 8
}

func (e *StreamEncoder) EncodeBytes(v []byte) {
	e.position += len(v)

	// Fast path: fits entirely in the buffer
	if e.bufPos+len(v) <= len(e.writeBuf) {
		copy(e.writeBuf[e.bufPos:], v)
		e.bufPos += len(v)
		return
	}

	// Flush existing buffer first
	e.flush()
	if e.writeErr != nil {
		return
	}

	// If data fits in buffer now, copy it
	if len(v) <= len(e.writeBuf) {
		copy(e.writeBuf[e.bufPos:], v)
		e.bufPos += len(v)
		return
	}

	// Large data: write directly to avoid extra copy
	written, err := e.writer.Write(v)
	if err != nil {
		e.writeErr = err
	} else if written != len(v) {
		e.writeErr = fmt.Errorf(
			"expected to write %d bytes, wrote %d", len(v), written,
		)
	}
}

func (e *StreamEncoder) EncodeOffset(v uint32) {
	if e.bufPos+4 > len(e.writeBuf) {
		e.flush()
	}
	binary.LittleEndian.PutUint32(e.writeBuf[e.bufPos:], v)
	e.bufPos += 4
	e.position += 4
}

func (e *StreamEncoder) EncodeOffsetAt(pos int, v uint32) {
	// not supported
	e.writeErr = fmt.Errorf(
		"EncodeOffsetAt is not supported for stream encoder",
	)
}

func (e *StreamEncoder) EncodeZeroPadding(n int) {
	e.position += n

	for n > 0 {
		if e.bufPos >= len(e.writeBuf) {
			e.flush()
			if e.writeErr != nil {
				return
			}
		}
		available := len(e.writeBuf) - e.bufPos
		toWrite := n
		if toWrite > available {
			toWrite = available
		}
		clear(e.writeBuf[e.bufPos : e.bufPos+toWrite])
		e.bufPos += toWrite
		n -= toWrite
	}
}

// GetWriteError returns the first write error encountered during encoding,
// or nil if no errors occurred.
func (e *StreamEncoder) GetWriteError() error {
	return e.writeErr
}
