package dynssz

import (
	"io"
)

// limitedReader wraps an io.Reader to track bytes read and enforce limits using a stack-based approach.
// This design avoids nested reader chains by maintaining a single reader with multiple limit levels.
type limitedReader struct {
	r         io.Reader
	bytesRead uint64
	limits    []limitInfo // Stack of limits
}

// limitInfo tracks information about each limit level
type limitInfo struct {
	absoluteLimit uint64 // Absolute byte position where this limit ends
	startBytes    uint64 // Bytes read when this limit was pushed
}

// newLimitedReader creates a reader that tracks bytes read with stack-based limit management
func newLimitedReader(r io.Reader) *limitedReader {
	return &limitedReader{
		r:      r,
		limits: make([]limitInfo, 0, 8), // Pre-allocate for typical nesting depth
	}
}

// pushLimit adds a new limit to the stack. The limit is relative to the current position.
// If the new limit would exceed a parent limit, it is capped to the parent limit.
// Returns the actual limit that was set.
func (lr *limitedReader) pushLimit(limit uint64) uint64 {
	newAbsoluteLimit := lr.bytesRead + limit

	// Cap to parent limit if necessary
	if len(lr.limits) > 0 {
		parentLimit := lr.limits[len(lr.limits)-1].absoluteLimit
		if newAbsoluteLimit > parentLimit {
			newAbsoluteLimit = parentLimit
		}
	}

	lr.limits = append(lr.limits, limitInfo{
		absoluteLimit: newAbsoluteLimit,
		startBytes:    lr.bytesRead,
	})

	return newAbsoluteLimit - lr.bytesRead
}

// popLimit removes the most recent limit from the stack and returns the number of bytes
// read since that limit was pushed.
func (lr *limitedReader) popLimit() uint64 {
	if len(lr.limits) == 0 {
		return lr.bytesRead
	}

	limit := lr.limits[len(lr.limits)-1]
	lr.limits = lr.limits[:len(lr.limits)-1]

	return lr.bytesRead - limit.startBytes
}

func (lr *limitedReader) bytesRemaining() (uint64, bool) {
	if len(lr.limits) == 0 {
		return 0, false // No limit means unlimited, but we return 0 to indicate no specific limit
	}

	currentLimit := lr.limits[len(lr.limits)-1]
	if lr.bytesRead >= currentLimit.absoluteLimit {
		return 0, true
	}
	return currentLimit.absoluteLimit - lr.bytesRead, true
}

// Read implements io.Reader
func (lr *limitedReader) Read(p []byte) (n int, err error) {
	// Check current limit if any
	if len(lr.limits) > 0 {
		currentLimit := lr.limits[len(lr.limits)-1]
		remaining := currentLimit.absoluteLimit - lr.bytesRead
		if remaining == 0 {
			return 0, io.EOF
		}
		if uint64(len(p)) > remaining {
			p = p[:remaining]
		}
	}

	n, err = lr.r.Read(p)
	lr.bytesRead += uint64(n)

	// If we've reached the current limit, return EOF
	if len(lr.limits) > 0 {
		currentLimit := lr.limits[len(lr.limits)-1]
		if lr.bytesRead >= currentLimit.absoluteLimit && err == nil {
			err = io.EOF
		}
	}

	return n, err
}
