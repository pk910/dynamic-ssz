package sszutils

var zeroBytes []byte

func ZeroBytes() []byte {
	if len(zeroBytes) == 0 {
		zeroBytes = make([]byte, 1024)
	}
	return zeroBytes
}

// AppendZeroPadding appends the specified number of zero bytes to buf
func AppendZeroPadding(buf []byte, count int) []byte {
	if len(zeroBytes) == 0 {
		zeroBytes = ZeroBytes()
	}
	for count > 0 {
		toCopy := count
		if toCopy > len(zeroBytes) {
			toCopy = len(zeroBytes)
		}
		buf = append(buf, zeroBytes[:toCopy]...)
		count -= toCopy
	}
	return buf
}
