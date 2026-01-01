// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"io"
)

const (
	// maxDecoderBufferSize is the maximum buffer size for streaming decode
	maxDecoderBufferSize = 2 * 1024 // 2KB
)

type StreamDecoder struct {
	reader    io.Reader
	limits    []int
	lastLimit int
	streamLen int
	position  int

	// Internal buffer for reading from stream
	buffer    []byte
	bufferPos int // Current read position within buffer
	bufferLen int // Amount of valid data in buffer
}

var _ Decoder = (*StreamDecoder)(nil)

func NewStreamDecoder(reader io.Reader, totalLen int) *StreamDecoder {
	// Use smaller buffer for small streams
	bufferSize := maxDecoderBufferSize
	if totalLen < bufferSize {
		bufferSize = totalLen
	}
	if bufferSize < 8 {
		bufferSize = 8 // Minimum size to hold a uint64
	}

	return &StreamDecoder{
		reader:    reader,
		limits:    make([]int, 0, 16),
		lastLimit: totalLen,
		streamLen: totalLen,
		position:  0,
		buffer:    make([]byte, bufferSize),
		bufferPos: 0,
		bufferLen: 0,
	}
}

func (e *StreamDecoder) Seekable() bool {
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

// ensureBuffered ensures at least n bytes are available in the buffer.
// Returns error if not enough data can be read from the stream.
func (e *StreamDecoder) ensureBuffered(n int) error {
	available := e.bufferLen - e.bufferPos
	if available >= n {
		return nil
	}

	// Need to read more data
	needed := n - available

	// If buffer is too small, grow it
	if len(e.buffer) < n {
		newSize := len(e.buffer) * 2
		if newSize < n {
			newSize = n
		}
		newBuf := make([]byte, newSize)
		// Copy remaining data to start of new buffer
		copy(newBuf, e.buffer[e.bufferPos:e.bufferLen])
		e.buffer = newBuf
		e.bufferLen = available
		e.bufferPos = 0
	} else if e.bufferPos > 0 {
		// Shift remaining data to start of buffer
		copy(e.buffer, e.buffer[e.bufferPos:e.bufferLen])
		e.bufferLen = available
		e.bufferPos = 0
	}

	// Calculate how much to read - at least needed, but prefer larger chunks
	toRead := len(e.buffer) - e.bufferLen
	if toRead < needed {
		toRead = needed
	}

	// Don't read more than remaining in stream
	remaining := e.streamLen - e.position - available
	if toRead > remaining {
		toRead = remaining
	}

	if toRead < needed {
		return ErrUnexpectedEOF
	}

	// Read from stream using a loop that handles partial reads
	readBuf := e.buffer[e.bufferLen : e.bufferLen+toRead]
	totalRead := 0
	for totalRead < toRead {
		n, err := e.reader.Read(readBuf[totalRead:])
		totalRead += n
		e.bufferLen += n

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				if e.bufferLen-e.bufferPos >= needed {
					return nil
				}
				return ErrUnexpectedEOF
			}
			return err
		}

		// If reader returned 0 bytes without error, it's an unusual case
		// Check if we have enough data, otherwise return EOF
		if n == 0 {
			if e.bufferLen-e.bufferPos >= needed {
				return nil
			}
			return ErrUnexpectedEOF
		}
	}

	return nil
}

// readByte reads a single byte from the buffer
func (e *StreamDecoder) readByte() (byte, error) {
	if err := e.ensureBuffered(1); err != nil {
		return 0, err
	}
	b := e.buffer[e.bufferPos]
	e.bufferPos++
	e.position++
	return b, nil
}

// readBytes reads n bytes into the provided buffer.
// For large reads, it copies available buffered data and reads the rest directly
// from the stream to avoid unnecessary buffering overhead.
func (e *StreamDecoder) readBytes(buf []byte) error {
	n := len(buf)
	available := e.bufferLen - e.bufferPos

	// If we have enough buffered data, use it directly
	if available >= n {
		copy(buf, e.buffer[e.bufferPos:e.bufferPos+n])
		e.bufferPos += n
		e.position += n
		return nil
	}

	// Check if request exceeds stream length
	streamRemaining := e.streamLen - e.position
	if n > streamRemaining {
		return ErrUnexpectedEOF
	}

	// Copy whatever is available from buffer
	if available > 0 {
		copy(buf, e.buffer[e.bufferPos:e.bufferLen])
		e.bufferPos = e.bufferLen
		e.position += available
	}

	// Read remainder directly from stream
	remaining := n - available
	totalRead := 0
	for totalRead < remaining {
		// Cap read to not exceed streamLen
		toRead := remaining - totalRead
		streamLeft := e.streamLen - e.position
		if toRead > streamLeft {
			toRead = streamLeft
		}
		if toRead <= 0 {
			return ErrUnexpectedEOF
		}

		nr, err := e.reader.Read(buf[available+totalRead : available+totalRead+toRead])
		totalRead += nr
		e.position += nr

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return ErrUnexpectedEOF
			}
			return err
		}

		// If reader returned 0 bytes without error, return EOF
		if nr == 0 {
			return ErrUnexpectedEOF
		}
	}

	return nil
}

// readBytesRef returns a slice reference to n bytes in the buffer.
// The returned slice is only valid until the next read operation.
func (e *StreamDecoder) readBytesRef(n int) ([]byte, error) {
	if err := e.ensureBuffered(n); err != nil {
		return nil, err
	}
	buf := e.buffer[e.bufferPos : e.bufferPos+n]
	e.bufferPos += n
	e.position += n
	return buf, nil
}

func (e *StreamDecoder) DecodeBool() (bool, error) {
	b, err := e.readByte()
	if err != nil {
		return false, err
	}
	if b != 1 && b != 0 {
		return false, ErrInvalidValueRange
	}
	return b == 1, nil
}

func (e *StreamDecoder) DecodeUint8() (uint8, error) {
	return e.readByte()
}

func (e *StreamDecoder) DecodeUint16() (uint16, error) {
	buf, err := e.readBytesRef(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf), nil
}

func (e *StreamDecoder) DecodeUint32() (uint32, error) {
	buf, err := e.readBytesRef(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func (e *StreamDecoder) DecodeUint64() (uint64, error) {
	buf, err := e.readBytesRef(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

func (e *StreamDecoder) DecodeBytes(buf []byte) ([]byte, error) {
	if err := e.readBytes(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (e *StreamDecoder) DecodeBytesBuf(l int) ([]byte, error) {
	limit := e.lastLimit
	if l < 0 {
		l = limit - e.position
	} else if limit-e.position < l {
		return nil, ErrUnexpectedEOF
	}

	// For large reads that exceed the buffer capacity, we need to grow the buffer
	// to accommodate the request. The returned slice is temporary and callers
	// must copy if they need to retain the data.
	if l > len(e.buffer) {
		// Grow buffer to accommodate the request
		newSize := len(e.buffer) * 2
		if newSize < l {
			newSize = l
		}
		newBuf := make([]byte, newSize)
		// Copy remaining data to start of new buffer
		available := e.bufferLen - e.bufferPos
		copy(newBuf, e.buffer[e.bufferPos:e.bufferLen])
		e.buffer = newBuf
		e.bufferLen = available
		e.bufferPos = 0
	}

	// Use the internal buffer - returned slice is temporary
	return e.readBytesRef(l)
}

func (e *StreamDecoder) DecodeOffset() (uint32, error) {
	return e.DecodeUint32()
}

func (e *StreamDecoder) DecodeOffsetAt(pos int) uint32 {
	// not supported for streaming decoder
	return 0
}

func (e *StreamDecoder) SkipBytes(n int) {
	// not supported for streaming decoder
}
