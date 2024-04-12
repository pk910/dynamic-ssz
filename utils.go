package dynssz

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

var (
	ErrOffset                = fmt.Errorf("incorrect offset")
	ErrSize                  = fmt.Errorf("incorrect size")
	ErrBytesLength           = fmt.Errorf("bytes array does not have the correct length")
	ErrVectorLength          = fmt.Errorf("vector does not have the correct length")
	ErrListTooBig            = fmt.Errorf("list length is higher than max value")
	ErrEmptyBitlist          = fmt.Errorf("bitlist is empty")
	ErrInvalidVariableOffset = fmt.Errorf("invalid ssz encoding. first variable element offset indexes into fixed value data")
)

// ---- Unmarshal functions ----

// unmarshallUint64 unmarshals a little endian uint64 from the src input
func unmarshallUint64(src []byte) uint64 {
	return binary.LittleEndian.Uint64(src)
}

// unmarshallUint32 unmarshals a little endian uint32 from the src input
func unmarshallUint32(src []byte) uint32 {
	return binary.LittleEndian.Uint32(src[:4])
}

// unmarshallUint16 unmarshals a little endian uint16 from the src input
func unmarshallUint16(src []byte) uint16 {
	return binary.LittleEndian.Uint16(src[:2])
}

// unmarshallUint8 unmarshals a little endian uint8 from the src input
func unmarshallUint8(src []byte) uint8 {
	return uint8(src[0])
}

// unmarshalBool unmarshals a boolean from the src input
func unmarshalBool(src []byte) bool {
	if src[0] == 1 {
		return true
	}
	return false
}

// unmarshalTime unmarshals a time.Time from the src input
func unmarshalTime(src []byte) time.Time {
	return time.Unix(int64(unmarshallUint64(src)), 0).UTC()
}

// ---- Marshal functions ----

// marshalUint64 marshals a little endian uint64 to dst
func marshalUint64(dst []byte, i uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, i)
	dst = append(dst, buf...)
	return dst
}

// marshalUint32 marshals a little endian uint32 to dst
func marshalUint32(dst []byte, i uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, i)
	dst = append(dst, buf...)
	return dst
}

// marshalUint16 marshals a little endian uint16 to dst
func marshalUint16(dst []byte, i uint16) []byte {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, i)
	dst = append(dst, buf...)
	return dst
}

// marshalUint8 marshals a little endian uint8 to dst
func marshalUint8(dst []byte, i uint8) []byte {
	dst = append(dst, byte(i))
	return dst
}

// marshalBool marshals a boolean to dst
func marshalBool(dst []byte, b bool) []byte {
	if b {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}
	return dst
}

// marshalTime marshals a time to dst
func marshalTime(dst []byte, t time.Time) []byte {
	return marshalUint64(dst, uint64(t.Unix()))
}

// ---- offset functions ----

// WriteOffset writes an offset to dst
func writeOffset(dst []byte, i int) []byte {
	return marshalUint32(dst, uint32(i))
}

// ReadOffset reads an offset from buf
func readOffset(buf []byte) uint64 {
	return uint64(binary.LittleEndian.Uint32(buf))
}

// DivideInt divides the int fully
func divideInt(a, b int) (int, bool) {
	return a / b, a%b == 0
}

// ---- hex functions ----

// FromHex returns the bytes represented by the hexadecimal string s.
// s may be prefixed with "0x".
func fromHex(s string) []byte {
	if has0xPrefix(s) {
		s = s[2:]
	}
	if len(s)%2 == 1 {
		s = "0" + s
	}
	return hex2Bytes(s)
}

// has0xPrefix validates str begins with '0x' or '0X'.
func has0xPrefix(str string) bool {
	return len(str) >= 2 && str[0] == '0' && (str[1] == 'x' || str[1] == 'X')
}

// Bytes2Hex returns the hexadecimal encoding of d.
func bytes2Hex(d []byte) string {
	return hex.EncodeToString(d)
}

// Hex2Bytes returns the bytes represented by the hexadecimal string str.
func hex2Bytes(str string) []byte {
	h, _ := hex.DecodeString(str)
	return h
}
