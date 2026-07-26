// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"io"
)

// DefaultStreamDecoderBufSize is the default maximum buffer size for
// StreamDecoder (2KB).
const DefaultStreamDecoderBufSize = 2 * 1024

// DefaultMaxStreamSize is the default upper bound on the total size of an SSZ
// payload decoded without a known length. Unknown-length decoding is always
// bounded: the bound is what keeps a peer that never closes the connection —
// and any decode site that has not been taught about open regions — from
// driving an unbounded allocation.
const DefaultMaxStreamSize = 512 * 1024 * 1024

// maxConsecutiveEmptyReads bounds retries of a (0, nil) read. Such a read is a
// valid no-op per the io.Reader contract and must be retried rather than
// treated as EOF, but a reader that only ever returns (0, nil) must not hang
// the decode. Progress resets the counter.
const maxConsecutiveEmptyReads = 100

// lengthUnknown marks a stream length that has not been discovered yet, and a
// region limit that extends to the end of such a stream.
const lengthUnknown = -1

// StreamDecoder is a non-seekable Decoder implementation that reads SSZ data
// from an io.Reader. It uses an internal buffer for efficient sequential reads
// but does not support DecodeOffsetAt or SkipBytes.
//
// A StreamDecoder can operate with a known total length, or — via
// NewUnknownStreamDecoder — discover the length at EOF. See the Decoder
// documentation for how open regions behave in the latter mode.
type StreamDecoder struct {
	reader    io.Reader
	limits    []int
	lastLimit int
	streamLen int
	position  int

	// maxSize bounds an unknown-length stream. It is always positive.
	maxSize int
	// eofSeen is sticky: once the reader reports EOF, streamLen is exact and
	// every open region has collapsed to a bounded one.
	eofSeen bool

	// Internal buffer for reading from stream
	buffer    []byte
	bufferPos int // Current read position within buffer
	bufferLen int // Amount of valid data in buffer
}

var _ Decoder = (*StreamDecoder)(nil)

// NewStreamDecoder creates a new StreamDecoder that reads SSZ data from the
// provided io.Reader. totalLen specifies the total expected byte length of the
// SSZ payload; a negative totalLen selects unknown-length mode with the default
// maximum stream size (see NewUnknownStreamDecoder). maxBufSize controls the
// maximum internal read buffer size; if <= 0, DefaultStreamDecoderBufSize is
// used.
func NewStreamDecoder(reader io.Reader, totalLen, maxBufSize int) *StreamDecoder {
	if totalLen < 0 {
		return NewUnknownStreamDecoder(reader, maxBufSize, 0)
	}
	return newStreamDecoder(reader, totalLen, maxBufSize, DefaultMaxStreamSize)
}

// NewUnknownStreamDecoder creates a StreamDecoder for an SSZ payload whose
// total length is not known in advance. The payload is read until EOF.
//
// maxBufSize controls the maximum internal read buffer size (<= 0 selects
// DefaultStreamDecoderBufSize). maxStreamSize bounds the total payload
// (<= 0 selects DefaultMaxStreamSize); it can never be unlimited, because it
// doubles as the allowance GetLength reports inside open regions.
func NewUnknownStreamDecoder(reader io.Reader, maxBufSize, maxStreamSize int) *StreamDecoder {
	if maxStreamSize <= 0 {
		maxStreamSize = DefaultMaxStreamSize
	}
	return newStreamDecoder(reader, lengthUnknown, maxBufSize, maxStreamSize)
}

func newStreamDecoder(reader io.Reader, totalLen, maxBufSize, maxStreamSize int) *StreamDecoder {
	if maxBufSize <= 0 {
		maxBufSize = DefaultStreamDecoderBufSize
	}
	// Use smaller buffer for small streams
	bufferSize := maxBufSize
	if totalLen >= 0 && totalLen < bufferSize {
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
		maxSize:   maxStreamSize,
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

// GetLength returns the number of bytes remaining in the current region.
//
// Inside an open region the true remaining length is not knowable, so this
// reports the remaining allowance (maxSize - position) instead of a sentinel.
// That keeps arithmetic on the result finite and plausible for callers that
// predate open regions: they derive an over-large region, then fail cleanly
// with ErrUnexpectedEOF at the real end of input. Use LengthKnown to tell the
// two cases apart.
func (e *StreamDecoder) GetLength() int {
	if e.lastLimit >= 0 {
		return e.lastLimit - e.position
	}
	remaining := e.maxSize - e.position
	if remaining < 0 {
		return 0
	}
	return remaining
}

// LengthKnown reports whether GetLength returns the exact remaining length of
// the current region.
func (e *StreamDecoder) LengthKnown() bool {
	return e.lastLimit >= 0
}

// rootLimit returns the limit that applies when no region is pushed.
func (e *StreamDecoder) rootLimit() int {
	return e.streamLen
}

func (e *StreamDecoder) PushLimit(limit int) {
	// Clamp a negative limit to zero like BufferDecoder does; a limit below
	// the current position would make GetLength() negative and poison
	// downstream reads.
	if limit < 0 {
		limit = 0
	}

	limitPos := e.position + limit
	if limitPos < e.position {
		// integer overflow on a hostile limit
		limitPos = e.maxSize
	}
	if e.lastLimit >= 0 {
		if limitPos > e.lastLimit {
			limitPos = e.lastLimit
		}
	} else if limitPos > e.maxSize {
		// Inside an open region there is no enclosing bound to clamp against,
		// so the allowance is the only backstop.
		limitPos = e.maxSize
	}

	e.limits = append(e.limits, limitPos)
	e.lastLimit = limitPos
}

// PushOpenLimit pushes a region that extends to the end of the input. If the
// enclosing region is bounded, the pushed region simply spans the rest of it.
func (e *StreamDecoder) PushOpenLimit() {
	e.limits = append(e.limits, e.lastLimit)
	// lastLimit is unchanged: an open child of an open parent stays open, and
	// an open child of a bounded parent inherits the parent's bound.
}

func (e *StreamDecoder) PopLimit() int {
	limitsLen := len(e.limits)
	if limitsLen == 0 {
		return 0
	}
	limit := e.limits[limitsLen-1]
	if limitsLen <= 1 {
		e.lastLimit = e.rootLimit()
	} else {
		e.lastLimit = e.limits[limitsLen-2]
	}
	e.limits = e.limits[:limitsLen-1]
	if limit < 0 {
		// An open region has no known end, so there is no unconsumed count to
		// report. Under-consumption is still caught: an open region is always
		// the suffix of the stream, so the leftover bytes surface at the
		// top-level EOF assertion (see FinishRegion).
		return 0
	}
	return limit - e.position
}

// onEOF records that the reader is exhausted. The stream length becomes exact
// and every open region collapses to a bounded one, so from here on the decoder
// behaves exactly like a known-length decoder.
func (e *StreamDecoder) onEOF() {
	if e.eofSeen {
		return
	}
	e.eofSeen = true
	if e.streamLen >= 0 {
		// Known-length mode: the declared length stands. A short reader is
		// reported as ErrUnexpectedEOF by the read that needed the bytes.
		return
	}
	e.streamLen = e.position + (e.bufferLen - e.bufferPos)
	if e.lastLimit < 0 {
		e.lastLimit = e.streamLen
	}
	for i := range e.limits {
		if e.limits[i] < 0 {
			e.limits[i] = e.streamLen
		}
	}
}

// compact moves buffered data to the start of the buffer.
func (e *StreamDecoder) compact() {
	if e.bufferPos == 0 {
		return
	}
	copy(e.buffer, e.buffer[e.bufferPos:e.bufferLen])
	e.bufferLen -= e.bufferPos
	e.bufferPos = 0
}

// growBuffer ensures the internal buffer can hold at least n bytes of
// contiguous unread data.
func (e *StreamDecoder) growBuffer(n int) {
	if n <= len(e.buffer) {
		return
	}
	newSize := len(e.buffer) * 2
	if newSize < n {
		newSize = n
	}
	newBuf := make([]byte, newSize)
	available := e.bufferLen - e.bufferPos
	copy(newBuf, e.buffer[e.bufferPos:e.bufferLen])
	e.buffer = newBuf
	e.bufferLen = available
	e.bufferPos = 0
}

// readMore performs one round of reading into the free tail of the buffer.
// It returns once at least one byte was buffered, EOF was reached, or an error
// occurred. The caller must have compacted the buffer.
func (e *StreamDecoder) readMore() error {
	if e.eofSeen {
		return nil
	}

	toRead := len(e.buffer) - e.bufferLen
	if toRead <= 0 {
		return nil // buffer full; caller must consume or grow
	}

	if e.streamLen >= 0 {
		// Known length: never read past the declared end of the payload. The
		// reader may carry unrelated data behind it.
		remaining := e.streamLen - e.position - (e.bufferLen - e.bufferPos)
		if remaining <= 0 {
			e.onEOF()
			return nil
		}
		if toRead > remaining {
			toRead = remaining
		}
	} else {
		// Unknown length: the allowance is the only bound. Read one byte past
		// it, since a payload of exactly maxSize still needs a read that comes
		// back empty before EOF can be established. Running out of room here
		// therefore means the payload genuinely exceeds the allowance.
		remaining := e.maxSize + 1 - e.position - (e.bufferLen - e.bufferPos)
		if remaining <= 0 {
			return ErrStreamTooLargeFn(e.maxSize)
		}
		if toRead > remaining {
			toRead = remaining
		}
	}

	emptyReads := 0
	for {
		nr, err := e.reader.Read(e.buffer[e.bufferLen : e.bufferLen+toRead])
		if nr < 0 {
			return ErrNegativeRead
		}
		e.bufferLen += nr

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				e.onEOF()
				return nil
			}
			return err
		}
		if nr > 0 {
			return nil
		}
		// (0, nil) is a valid no-op read: retry up to the bound rather than
		// treating it as EOF (a reader stuck on (0, nil) must not hang).
		emptyReads++
		if emptyReads >= maxConsecutiveEmptyReads {
			return ErrUnexpectedEOF
		}
	}
}

// ensureBuffered ensures at least n bytes are available in the buffer.
// Returns error if not enough data can be read from the stream.
func (e *StreamDecoder) ensureBuffered(n int) error {
	if e.bufferLen-e.bufferPos >= n {
		return nil
	}

	if n > len(e.buffer) {
		e.growBuffer(n)
	} else {
		e.compact()
	}

	for e.bufferLen-e.bufferPos < n {
		if e.eofSeen {
			return ErrUnexpectedEOF
		}
		if err := e.readMore(); err != nil {
			return err
		}
	}
	return nil
}

// checkRegion verifies that n more bytes may be read from the current region.
// Inside an open region there is no limit to check against, so the request is
// only bounded by the allowance; a short input surfaces as EOF during the read.
func (e *StreamDecoder) checkRegion(n int) error {
	if e.lastLimit >= 0 {
		if e.position+n > e.lastLimit {
			return ErrUnexpectedEOF
		}
		return nil
	}
	if e.position+n > e.maxSize {
		return ErrStreamTooLargeFn(e.maxSize)
	}
	return nil
}

// readByte reads a single byte from the buffer
func (e *StreamDecoder) readByte() (byte, error) {
	// Never read across the current region limit; a malformed region must
	// fail cleanly instead of consuming bytes of subsequent regions.
	if err := e.checkRegion(1); err != nil {
		return 0, err
	}
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

	// Never read across the current region limit; a malformed region must
	// fail cleanly instead of consuming bytes of subsequent regions.
	if err := e.checkRegion(n); err != nil {
		return err
	}

	available := e.bufferLen - e.bufferPos

	// If we have enough buffered data, use it directly
	if available >= n {
		copy(buf, e.buffer[e.bufferPos:e.bufferPos+n])
		e.bufferPos += n
		e.position += n
		return nil
	}

	// Copy whatever is available from buffer
	if available > 0 {
		copy(buf, e.buffer[e.bufferPos:e.bufferLen])
		e.bufferPos = e.bufferLen
		e.position += available
	}

	if e.eofSeen {
		return ErrUnexpectedEOF
	}

	// Read remainder directly from stream
	remaining := n - available
	totalRead := 0
	emptyReads := 0
	for totalRead < remaining {
		toRead := remaining - totalRead

		nr, err := e.reader.Read(buf[available+totalRead : available+totalRead+toRead])
		if nr < 0 {
			return ErrNegativeRead
		}
		totalRead += nr
		e.position += nr

		// A reader may return the final bytes together with io.EOF; once the
		// request is satisfied the read succeeded regardless of that error.
		if totalRead >= remaining {
			break
		}
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				e.onEOF()
				return ErrUnexpectedEOF
			}
			return err
		}
		// (0, nil) is a valid no-op read: retry up to the bound rather than
		// treating it as EOF (a reader stuck on (0, nil) must not hang).
		if nr == 0 {
			emptyReads++
			if emptyReads >= maxConsecutiveEmptyReads {
				return ErrUnexpectedEOF
			}
			continue
		}
		emptyReads = 0
	}

	return nil
}

// readBytesRef returns a slice reference to n bytes in the buffer.
// The returned slice is only valid until the next read operation.
func (e *StreamDecoder) readBytesRef(n int) ([]byte, error) {
	// Never read across the current region limit; a malformed region must
	// fail cleanly instead of consuming bytes of subsequent regions.
	if err := e.checkRegion(n); err != nil {
		return nil, err
	}
	if err := e.ensureBuffered(n); err != nil {
		return nil, err
	}
	buf := e.buffer[e.bufferPos : e.bufferPos+n]
	e.bufferPos += n
	e.position += n
	return buf, nil
}

// Prefill performs one read into the internal buffer. In unknown-length mode
// this is worth doing before decoding starts: if the whole payload fits in the
// buffer, EOF is observed immediately, the stream length becomes exact and no
// open region is ever created — the decode then runs entirely on the
// known-length code path, with all of its fail-fast validation intact.
//
// It is a no-op once the length is known or EOF has been seen.
//
// Prefill reads until the internal buffer is full or the reader reports EOF,
// so it can block on a slow reader. That is not a new constraint: an
// unknown-length decode consumes the input to EOF regardless, so the producer
// has to close the stream for the decode to complete at all.
func (e *StreamDecoder) Prefill() error {
	if e.eofSeen || e.streamLen >= 0 {
		return nil
	}
	e.compact()
	for !e.eofSeen && e.bufferLen < len(e.buffer) {
		before := e.bufferLen
		if err := e.readMore(); err != nil {
			return err
		}
		if e.bufferLen == before {
			// No progress: EOF with nothing buffered, or readMore declining to
			// read further. Stop rather than spin.
			break
		}
	}
	return nil
}

// More reports whether the current region holds at least one more byte. For a
// bounded region the answer comes from the limit; for an open region the reader
// is probed, which is what turns "no more data" into a discovered EOF.
func (e *StreamDecoder) More() (bool, error) {
	if e.lastLimit >= 0 {
		return e.lastLimit-e.position > 0, nil
	}
	if e.bufferLen-e.bufferPos > 0 {
		return true, nil
	}
	// No separate eofSeen check: readMore returns immediately once EOF is
	// sticky, and after EOF an open region has already collapsed to a bounded
	// one, so this path is only reached with the reader still live.
	e.compact()
	if err := e.readMore(); err != nil {
		return false, err
	}
	return e.bufferLen-e.bufferPos > 0, nil
}

// DecodeRemaining consumes the rest of the current region — to the region limit,
// or to EOF for an open region — and returns it in a newly allocated slice the
// caller may retain. If max is non-negative and the payload exceeds it, the call
// fails without having allocated the full payload.
func (e *StreamDecoder) DecodeRemaining(max int) ([]byte, error) {
	if e.lastLimit >= 0 {
		length := e.lastLimit - e.position
		if length < 0 {
			length = 0
		}
		if max >= 0 && length > max {
			return nil, ErrPayloadTooLargeFn(length, max)
		}
		out := make([]byte, length)
		if length > 0 {
			if err := e.readBytes(out); err != nil {
				return nil, err
			}
		}
		return out, nil
	}

	// Open region: grow incrementally rather than trusting any declared size.
	//
	// The loop refills directly instead of asking More(), so every iteration
	// either consumes at least one byte or breaks. Routing this through More()
	// would make progress depend on its "reported data implies buffered data"
	// invariant, which is exactly the kind of coupling that turns into a spin.
	out := []byte{}
	for {
		available := e.bufferLen - e.bufferPos
		if e.lastLimit >= 0 {
			// EOF collapsed this region to a bounded one mid-read; never
			// consume past the limit.
			available = min(available, e.lastLimit-e.position)
		}

		if available <= 0 {
			if e.eofSeen {
				break
			}
			e.compact()
			if err := e.readMore(); err != nil {
				return nil, err
			}
			if e.bufferLen-e.bufferPos == 0 {
				break // EOF with nothing left
			}
			continue
		}

		if max >= 0 && len(out)+available > max {
			return nil, ErrPayloadTooLargeFn(len(out)+available, max)
		}

		out = append(out, e.buffer[e.bufferPos:e.bufferPos+available]...)
		e.bufferPos += available
		e.position += available
	}
	return out, nil
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
	if l < 0 {
		// "All remaining" in the current region. For an open region the extent
		// is only known at EOF, so fall back to the growing path and hand back
		// the freshly allocated slice.
		if e.lastLimit < 0 {
			return e.DecodeRemaining(-1)
		}
		l = e.lastLimit - e.position
	} else if err := e.checkRegion(l); err != nil {
		return nil, err
	}

	// For large reads that exceed the buffer capacity, we need to grow the buffer
	// to accommodate the request. The returned slice is temporary and callers
	// must copy if they need to retain the data.
	if l > len(e.buffer) {
		e.growBuffer(l)
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
