package dynssz

import (
	"io"
)

// limitedReader wraps an io.Reader to track bytes read and enforce a limit
type limitedReader struct {
	r         io.Reader
	bytesRead int64
	limit     int64 // -1 for unlimited
}

// newLimitedReader creates a reader that tracks bytes read and optionally limits reads
func newLimitedReader(r io.Reader, limit int64) *limitedReader {
	return &limitedReader{
		r:     r,
		limit: limit,
	}
}

// Read implements io.Reader
func (lr *limitedReader) Read(p []byte) (n int, err error) {
	if lr.limit >= 0 {
		remaining := lr.limit - lr.bytesRead
		if remaining <= 0 {
			return 0, io.EOF
		}
		if int64(len(p)) > remaining {
			p = p[:remaining]
		}
	}

	n, err = lr.r.Read(p)
	lr.bytesRead += int64(n)
	
	// If we've reached the limit, return EOF on the next call
	if lr.limit >= 0 && lr.bytesRead >= lr.limit && err == nil {
		err = io.EOF
	}
	
	return n, err
}

// BytesRead returns the total number of bytes read
func (lr *limitedReader) BytesRead() int64 {
	return lr.bytesRead
}