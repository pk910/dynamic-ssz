package dynssz

import (
	"io"
)

// countingWriter wraps an io.Writer to track total bytes written
type countingWriter struct {
	w            io.Writer
	bytesWritten int64
}

// newCountingWriter creates a writer that tracks bytes written
func newCountingWriter(w io.Writer) *countingWriter {
	return &countingWriter{
		w: w,
	}
}

// Write implements io.Writer
func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	cw.bytesWritten += int64(n)
	return n, err
}

// BytesWritten returns the total number of bytes written
func (cw *countingWriter) BytesWritten() int64 {
	return cw.bytesWritten
}