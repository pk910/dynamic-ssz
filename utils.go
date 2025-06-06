package dynssz

import (
	"encoding/binary"
	"fmt"
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
	return src[0] == 1
}

// ---- Marshal functions ----

// marshalUint64 marshals a little endian uint64 to dst
func marshalUint64(dst []byte, i uint64) []byte {
	return binary.LittleEndian.AppendUint64(dst, i)
}

// marshalUint32 marshals a little endian uint32 to dst
func marshalUint32(dst []byte, i uint32) []byte {
	return binary.LittleEndian.AppendUint32(dst, i)
}

// marshalUint16 marshals a little endian uint16 to dst
func marshalUint16(dst []byte, i uint16) []byte {
	return binary.LittleEndian.AppendUint16(dst, i)
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

// ---- offset functions ----

// ReadOffset reads an offset from buf
func readOffset(buf []byte) uint64 {
	return uint64(binary.LittleEndian.Uint32(buf))
}

// DivideInt divides the int fully
func divideInt(a, b int) (int, bool) {
	return a / b, a%b == 0
}

func calculateLimit(maxCapacity, numItems, size uint64) uint64 {
	limit := (maxCapacity*size + 31) / 32
	if limit != 0 {
		return limit
	}
	if numItems == 0 {
		return 1
	}
	return numItems
}
