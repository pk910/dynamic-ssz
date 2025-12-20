package stream

type BufferWriter struct {
	bytes []byte
}

func NewBufferWriter(buf []byte) *BufferWriter {
	return &BufferWriter{
		bytes: buf,
	}
}

func (bw *BufferWriter) Write(p []byte) (n int, err error) {
	bw.bytes = append(bw.bytes, p...)
	return len(p), nil
}

func (bw *BufferWriter) Bytes() []byte {
	return bw.bytes
}
