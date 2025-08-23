package dynssz

import (
	"encoding/binary"
	"fmt"
)

var (
	ErrOffset       = fmt.Errorf("incorrect offset")
	ErrSize         = fmt.Errorf("incorrect size")
	ErrBytesLength  = fmt.Errorf("bytes array does not have the correct length")
	ErrVectorLength = fmt.Errorf("vector does not have the correct length")

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
	return src[0] == 1
}

// ---- offset functions ----

// ReadOffset reads an offset from buf
func readOffset(buf []byte) uint64 {
	return uint64(binary.LittleEndian.Uint32(buf))
}

// DivideInt divides the int fully
func divideInt(a, b int) (int, bool) {
	return a / b, a%b == 0
}
