package dynssz

import (
	"io"
)

// limitedWriter wraps an io.Writer to track bytes written and enforce limits using a stack-based approach.
// This design avoids nested writer chains by maintaining a single writer with multiple limit levels.
type limitedWriter struct {
	w            io.Writer
	bytesWritten uint64
	limits       []limitWriteInfo // Stack of limits
}

// limitWriteInfo tracks information about each write limit level
type limitWriteInfo struct {
	absoluteLimit uint64 // Absolute byte position where this limit ends
	startBytes    uint64 // Bytes written when this limit was pushed
}

// newLimitedWriter creates a writer that tracks bytes written
func newLimitedWriter(w io.Writer) *limitedWriter {
	return &limitedWriter{
		w:      w,
		limits: make([]limitWriteInfo, 0, 8), // Pre-allocate for typical nesting depth
	}
}

// pushLimit adds a new limit to the stack. The limit is relative to the current position.
// If the new limit would exceed a parent limit, it is capped to the parent limit.
// Returns the actual limit that was set.
func (lw *limitedWriter) pushLimit(limit uint64) uint64 {
	newAbsoluteLimit := lw.bytesWritten + limit

	// Cap to parent limit if necessary
	if len(lw.limits) > 0 {
		parentLimit := lw.limits[len(lw.limits)-1].absoluteLimit
		if newAbsoluteLimit > parentLimit {
			newAbsoluteLimit = parentLimit
		}
	}

	lw.limits = append(lw.limits, limitWriteInfo{
		absoluteLimit: newAbsoluteLimit,
		startBytes:    lw.bytesWritten,
	})

	return newAbsoluteLimit - lw.bytesWritten
}

// popLimit removes the most recent limit from the stack and returns the number of bytes
// written since that limit was pushed.
func (lw *limitedWriter) popLimit() uint64 {
	if len(lw.limits) == 0 {
		return lw.bytesWritten
	}

	limit := lw.limits[len(lw.limits)-1]
	lw.limits = lw.limits[:len(lw.limits)-1]

	return lw.bytesWritten - limit.startBytes
}

// bytesRemaining returns the number of bytes that can still be written before hitting the current limit
func (lw *limitedWriter) bytesRemaining() (uint64, bool) {
	if len(lw.limits) == 0 {
		return 0, false // No limit means unlimited, but we return 0 to indicate no specific limit
	}

	currentLimit := lw.limits[len(lw.limits)-1]
	if lw.bytesWritten >= currentLimit.absoluteLimit {
		return 0, true
	}
	return currentLimit.absoluteLimit - lw.bytesWritten, true
}

// Write implements io.Writer
func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	// Check current limit if any
	if len(lw.limits) > 0 {
		currentLimit := lw.limits[len(lw.limits)-1]
		remaining := currentLimit.absoluteLimit - lw.bytesWritten
		if remaining < uint64(len(p)) {
			return 0, io.ErrShortWrite
		}
		if uint64(len(p)) > remaining {
			p = p[:remaining]
		}
	}

	n, err = lw.w.Write(p)
	lw.bytesWritten += uint64(n)

	// If we've reached the current limit and tried to write more, return error
	if len(lw.limits) > 0 {
		currentLimit := lw.limits[len(lw.limits)-1]
		if lw.bytesWritten >= currentLimit.absoluteLimit && len(p) > n {
			err = io.ErrShortWrite
		}
	}

	return n, err
}
