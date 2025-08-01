package dynssz_test

import "encoding/hex"

var trueValue = true
var testUint8Arr = []uint8{1, 2, 3}
var testUint16 = uint16(0x1337)

type slug_DynStruct1 struct {
	F1 bool
	F2 []uint8 `ssz-max:"10"`
}

type slug_StaticStruct1 struct {
	F1 bool
	F2 []uint8 `ssz-size:"3"`
}

type slug_StableDynStruct1 struct {
	F1 *bool `ssz-container:"stable-container,16"`
	F2 *[]uint8
	F3 *uint16
}

type slug_StableStaticStruct1 struct {
	F1 *bool    `ssz-container:"stable-container,16"`
	F2 *[]uint8 `ssz-size:"3"`
	F3 *uint16
}

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

// Hex2Bytes returns the bytes represented by the hexadecimal string str.
func hex2Bytes(str string) []byte {
	h, _ := hex.DecodeString(str)
	return h
}
