// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"io"
	"math"
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

// lengthUnknown marks a stream length that has not been discovered yet.
const lengthUnknown = -1

// openLimitMarker marks a region-stack entry whose end is not yet known.
const openLimitMarker = -1

// maxAllowance caps the maximum stream size. Inside an open region the
// allowance is what GetLength reports, and callers routinely narrow a region
// length to uint32 (SSZ offsets are uint32, so a region cannot legitimately be
// larger). Capping here keeps that conversion lossless and keeps the
// "read one byte past the allowance to establish EOF" arithmetic from
// overflowing int.
//
// On a 32-bit platform math.MaxUint32 does not fit in an int, so the cap is
// whichever bound binds first.
const maxAllowance = min(math.MaxUint32, math.MaxInt-1)

// StreamDecoder is a non-seekable Decoder implementation that reads SSZ data
// from an io.Reader. It uses an internal buffer for efficient sequential reads
// but does not support DecodeOffsetAt or SkipBytes.
//
// A StreamDecoder can operate with a known total length, or — via
// NewUnknownStreamDecoder — discover the length at EOF. See the Decoder
// documentation for how open regions behave in the latter mode.
type StreamDecoder struct {
	reader io.Reader
	// limits is the region stack. A negative entry marks an open region.
	limits []int
	// lastLimit is the absolute end of the current region and is ALWAYS a
	// usable bound: for an open region it is the allowance. Keeping it
	// non-negative is what lets every read check it with a single comparison,
	// which is the decoder's hottest instruction.
	lastLimit int
	// lastOpen records whether the current region is open, since lastLimit can
	// no longer express that.
	lastOpen  bool
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
	// The allowance is reported as a remaining length and is read one byte past
	// to establish EOF, so it has to stay clear of both an int overflow and the
	// uint32 conversions that callers apply to a region length. An SSZ offset is
	// a uint32, so no single region can legitimately exceed this anyway.
	if maxStreamSize > maxAllowance {
		maxStreamSize = maxAllowance
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

	rootLimit, rootOpen := totalLen, false
	if totalLen < 0 {
		rootLimit, rootOpen = maxStreamSize, true
	}

	return &StreamDecoder{
		reader:    reader,
		limits:    make([]int, 0, 16),
		lastLimit: rootLimit,
		lastOpen:  rootOpen,
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
	remaining := e.lastLimit - e.position
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RegionOpen reports whether the current region runs to EOF rather than to a
// declared end.
func (e *StreamDecoder) RegionOpen() bool {
	return e.lastOpen
}

// LengthKnown reports whether GetLength is backed by input known to exist.
//
// A declared extent is not enough. Inside an open region an offset is validated
// against the allowance rather than against delivered bytes, so PushLimit can
// derive a bounded region far larger than the input -- a peer can declare a
// 500 MB field in a 12-byte payload. Sizing an allocation from that is a memory
// amplification attack.
//
// The extent is trustworthy in two cases: the region fits inside a total stream
// length that is established (supplied by the caller, or discovered at EOF), or
// it already fits in bytes that have been read -- which a peer cannot fake
// without sending them. Reaching EOF is not on its own enough: it makes the
// length exact but leaves any region derived from an earlier offset in place,
// still claiming bytes that never arrived.
func (e *StreamDecoder) LengthKnown() bool {
	if e.lastOpen {
		return false
	}
	if e.streamLen >= 0 {
		// A region derived from an offset before EOF can still end past the
		// stream: EOF makes streamLen exact but leaves that stale limit in
		// place. Only a region that fits inside the established length is
		// backed by input. (With a caller-supplied length every limit is
		// clamped to it, so this always holds.)
		return e.lastLimit <= e.streamLen
	}
	// The whole region is already in the read buffer, so its bytes exist.
	return e.lastLimit-e.position <= e.bufferLen-e.bufferPos
}

// Available reports how many bytes of the current region are already buffered.
func (e *StreamDecoder) Available() int {
	avail := e.bufferLen - e.bufferPos
	if room := e.lastLimit - e.position; room < avail {
		avail = room
	}
	if avail < 0 {
		return 0
	}
	return avail
}

// rootLimit returns the bound that applies when no region is pushed, and
// whether that region is open.
func (e *StreamDecoder) rootLimit() (int, bool) {
	if e.streamLen >= 0 {
		return e.streamLen, false
	}
	return e.maxSize, true
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
		limitPos = e.lastLimit
	}
	// lastLimit is always a real bound -- the allowance when the region is
	// open -- so one clamp covers both cases.
	if limitPos > e.lastLimit {
		limitPos = e.lastLimit
	}

	e.limits = append(e.limits, limitPos)
	e.lastLimit = limitPos
	e.lastOpen = false
}

// PushOpenLimit pushes a region that extends to the end of the input. If the
// enclosing region is bounded, the pushed region simply spans the rest of it.
func (e *StreamDecoder) PushOpenLimit() {
	// An open child of an open parent stays open; an open child of a bounded
	// parent simply spans the rest of it. Either way the effective bound is
	// unchanged, so only the stack entry differs.
	if e.lastOpen {
		e.limits = append(e.limits, openLimitMarker)
	} else {
		e.limits = append(e.limits, e.lastLimit)
	}
}

func (e *StreamDecoder) PopLimit() int {
	limitsLen := len(e.limits)
	if limitsLen == 0 {
		return 0
	}
	limit := e.limits[limitsLen-1]
	if limitsLen <= 1 {
		e.lastLimit, e.lastOpen = e.rootLimit()
	} else if parent := e.limits[limitsLen-2]; parent < 0 {
		e.lastLimit, e.lastOpen = e.rootLimit()
	} else {
		e.lastLimit, e.lastOpen = parent, false
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
	if e.lastOpen {
		e.lastLimit = e.streamLen
		e.lastOpen = false
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

	knownLen := e.streamLen >= 0
	emptyReads := 0
	for {
		nr, err := e.reader.Read(e.buffer[e.bufferLen : e.bufferLen+toRead])
		if nr < 0 {
			return ErrNegativeRead
		}
		e.bufferLen += nr

		// The allowance is read one byte past to establish EOF, and a reader may
		// hand that probe byte back together with data or with io.EOF. Re-test
		// the cap after every read rather than only when the next read finds no
		// room, so the maximum stays a hard bound.
		if !knownLen && e.position+(e.bufferLen-e.bufferPos) > e.maxSize {
			return ErrStreamTooLargeFn(e.maxSize)
		}

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

// regionOverrun reports the right error for a read that ran past the current
// region. It is only called once the bound test has already failed, so it stays
// off the hot path and out of the inlining budget of the read helpers.
func (e *StreamDecoder) regionOverrun() error {
	if e.lastOpen {
		// The bound was the allowance, so the payload is over the maximum
		// rather than merely past a region.
		return ErrStreamTooLargeFn(e.maxSize)
	}
	return ErrUnexpectedEOF
}

// readByte reads a single byte from the buffer
func (e *StreamDecoder) readByte() (byte, error) {
	// Never read across the current region limit; a malformed region must
	// fail cleanly instead of consuming bytes of subsequent regions.
	if e.position+1 > e.lastLimit {
		return 0, e.regionOverrun()
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
	if e.position+n > e.lastLimit {
		return e.regionOverrun()
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
	if e.position+n > e.lastLimit {
		return nil, e.regionOverrun()
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

// FinishRegion pops the current region, asserting it was fully consumed.
//
// A bounded region is answered from the limit alone, with no I/O -- that covers
// every known-length decode, and every region of an unknown-length decode once
// EOF has closed it. Only a genuinely open region has to probe the reader.
func (e *StreamDecoder) FinishRegion() error {
	if !e.lastOpen {
		if diff := e.PopLimit(); diff != 0 {
			return ErrTrailingDataFn(diff)
		}
		return nil
	}

	more, err := e.More()
	if err != nil {
		e.PopLimit()
		return err
	}
	diff := e.PopLimit()
	if more {
		if diff <= 0 {
			// Open region: the leftover count is unknown, but its presence is
			// established.
			diff = 1
		}
		return ErrTrailingDataFn(diff)
	}
	return nil
}

// More reports whether the current region holds at least one more byte. For a
// bounded region the answer comes from the limit; for an open region the reader
// is probed, which is what turns "no more data" into a discovered EOF.
func (e *StreamDecoder) More() (bool, error) {
	if !e.lastOpen {
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
// caller may retain. If maxLen is non-negative and the payload exceeds it, the call
// fails without having allocated the full payload.
//
// A bounded region that the input cannot fill fails with ErrUnexpectedEOF; only
// an open region legitimately ends where the input does.
func (e *StreamDecoder) DecodeRemaining(maxLen int) ([]byte, error) {
	// Only preallocate when the extent is backed by input known to exist; a
	// limit derived from an unverified offset must be consumed incrementally.
	if e.LengthKnown() {
		length := e.lastLimit - e.position
		if length < 0 {
			length = 0
		}
		if maxLen >= 0 && length > maxLen {
			return nil, ErrPayloadTooLargeFn(length, maxLen)
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

	// A region reaching this path that is not open is bounded but unverified:
	// its end was derived from an offset validated against the allowance, so it
	// may lie past the real end of input. Such a region must be consumed in
	// full -- running out of input first is ErrUnexpectedEOF, not a short
	// result, matching what the preallocating branch above reports. Capture the
	// state up front, since EOF collapses an open region to a bounded one.
	openRegion := e.lastOpen
	out := []byte{}
	for {
		available := e.bufferLen - e.bufferPos
		{
			// Stop at the region limit: past it the reader's data belongs to
			// whatever comes next, and refilling for it would never make
			// progress. For an open region the limit is the allowance, and
			// reaching it means the payload is over the maximum rather than
			// finished -- EOF would have closed the region first.
			room := e.lastLimit - e.position
			if room <= 0 {
				if !e.lastOpen {
					break
				}
				// The allowance is reached. That is only an overrun if input
				// actually remains, so probe: a payload of exactly the maximum
				// must still decode, and the probe is what discovers the EOF
				// that closes the region.
				more, err := e.More()
				if err != nil {
					return nil, err
				}
				if more {
					return nil, ErrStreamTooLargeFn(e.maxSize)
				}
				break
			}
			available = min(available, room)
		}

		if available <= 0 {
			if e.eofSeen {
				if !openRegion {
					return nil, ErrUnexpectedEOF
				}
				break
			}
			e.compact()
			if err := e.readMore(); err != nil {
				return nil, err
			}
			if e.bufferLen-e.bufferPos == 0 {
				if !openRegion {
					return nil, ErrUnexpectedEOF
				}
				break // EOF with nothing left
			}
			continue
		}

		if maxLen >= 0 && len(out)+available > maxLen {
			return nil, ErrPayloadTooLargeFn(len(out)+available, maxLen)
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
	// Fast path: the 2 bytes are within the region limit and already buffered,
	// so read them inline without the readBytesRef/ensureBuffered calls. This is
	// exactly readBytesRef's buffered case; anything else defers to it.
	if e.position+2 <= e.lastLimit && e.bufferLen-e.bufferPos >= 2 {
		v := binary.LittleEndian.Uint16(e.buffer[e.bufferPos : e.bufferPos+2])
		e.bufferPos += 2
		e.position += 2
		return v, nil
	}
	buf, err := e.readBytesRef(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf), nil
}

func (e *StreamDecoder) DecodeUint32() (uint32, error) {
	// Fast path: the 4 bytes are within the region limit and already buffered,
	// so read them inline without the readBytesRef/ensureBuffered calls. This is
	// exactly readBytesRef's buffered case; anything else defers to it.
	if e.position+4 <= e.lastLimit && e.bufferLen-e.bufferPos >= 4 {
		v := binary.LittleEndian.Uint32(e.buffer[e.bufferPos : e.bufferPos+4])
		e.bufferPos += 4
		e.position += 4
		return v, nil
	}
	buf, err := e.readBytesRef(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf), nil
}

func (e *StreamDecoder) DecodeUint64() (uint64, error) {
	// Fast path: the 8 bytes are within the region limit and already buffered,
	// so read them inline without the readBytesRef/ensureBuffered calls. This is
	// exactly readBytesRef's buffered case; anything else defers to it.
	if e.position+8 <= e.lastLimit && e.bufferLen-e.bufferPos >= 8 {
		v := binary.LittleEndian.Uint64(e.buffer[e.bufferPos : e.bufferPos+8])
		e.bufferPos += 8
		e.position += 8
		return v, nil
	}
	buf, err := e.readBytesRef(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

func (e *StreamDecoder) DecodeBytes(buf []byte) ([]byte, error) {
	// Fast path: within the region limit and already buffered - copy inline
	// instead of calling readBytes (which the inliner cannot take). This mirrors
	// readBytes' buffered case exactly; anything else defers to it.
	n := len(buf)
	if e.position+n <= e.lastLimit && e.bufferLen-e.bufferPos >= n {
		copy(buf, e.buffer[e.bufferPos:e.bufferPos+n])
		e.bufferPos += n
		e.position += n
		return buf, nil
	}
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
		if e.lastOpen {
			return e.DecodeRemaining(-1)
		}
		l = e.lastLimit - e.position
	} else if e.position+l > e.lastLimit {
		return nil, e.regionOverrun()
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
