package tests

import dynssz "github.com/pk910/dynamic-ssz"

type SimpleTypes1 struct {
	B1       bool
	I8       uint8
	I16      uint16
	I32      uint32
	I64      uint64
	I128     [16]byte
	I256     [4]uint64
	Vec8     []uint8     `ssz-size:"4"`
	Vec32    []uint32    `ssz-size:"4"`
	Vec128   [][2]uint64 `ssz-type:"?,uint128" ssz-size:"4"`
	BitVec   [8]byte     `ssz-type:"bitvector"`
	Lst8     []uint8     `ssz-max:"4"`
	Lst32    []uint32    `ssz-max:"4"`
	Lst128   [][2]uint64 `ssz-type:"?,uint128" ssz-max:"4"`
	F1       [2][]uint16
	F2       [10]uint8 `ssz-size:"5"`
	Str      string    `ssz-max:"8"`
	Wrapper1 dynssz.TypeWrapper[struct {
		Data []byte `ssz-size:"32"`
	}, []byte] `ssz-type:"wrapper"`
}

var SimpleTypes1_Payload = SimpleTypes1{
	B1:     true,
	I8:     1,
	I16:    2,
	I32:    3,
	I64:    4,
	I128:   [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	I256:   [4]uint64{1, 2, 3, 4},
	Vec8:   []uint8{1, 2, 3, 4},
	Vec32:  []uint32{1, 2, 3, 4},
	Vec128: [][2]uint64{{1, 2}, {3, 4}},
	BitVec: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	Lst8:   []uint8{1, 2, 3, 4},
	Lst32:  []uint32{1, 2, 3, 4},
	Lst128: [][2]uint64{{1, 2}, {3, 4}},
	F1:     [2][]uint16{{1, 2}, {3, 4}},
	F2:     [10]uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	Str:    "hello",
	Wrapper1: dynssz.TypeWrapper[struct {
		Data []byte `ssz-size:"32"`
	}, []byte]{
		Data: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	},
}

type SimpleTypesWithSpecs struct {
	Vec8   []uint8     `ssz-size:"4" dynssz-size:"VEC8_SIZE"`
	Vec32  []uint32    `ssz-size:"4" dynssz-size:"VEC32_SIZE"`
	Vec128 [][2]uint64 `ssz-type:"?,uint128" ssz-size:"4" dynssz-size:"VEC128_SIZE"`
	BitVec []byte      `ssz-type:"bitvector" ssz-size:"8" dynssz-size:"BITVEC_SIZE"`
	Lst8   []uint8     `ssz-max:"4" dynssz-max:"LST8_MAX"`
	Lst32  []uint32    `ssz-max:"4" dynssz-max:"LST32_MAX"`
	Lst128 [][2]uint64 `ssz-type:"?,uint128" ssz-max:"4" dynssz-max:"LST128_MAX"`
	Str1   string      `ssz-max:"8" dynssz-max:"STR_MAX"`
	Str2   string      `ssz-size:"10" dynssz-size:"STR_SIZE"`
}

var SimpleTypesWithSpecs_Payload = SimpleTypesWithSpecs{
	Vec8:   []uint8{1, 2, 3, 4, 5, 6},
	Vec32:  []uint32{1, 2, 3, 4, 5, 6, 7, 8},
	Vec128: [][2]uint64{{1, 2}, {3, 4}},
	BitVec: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	Lst8:   []uint8{1, 2, 3, 4, 5, 6},
	Lst32:  []uint32{1, 2, 3, 4, 5, 6, 7, 8},
	Lst128: [][2]uint64{{1, 2}, {3, 4}},
	Str1:   "hello",
	Str2:   "hello2",
}
var SimpleTypesWithSpecs_Specs = map[string]any{
	"VEC8_SIZE":   6,
	"VEC32_SIZE":  8,
	"VEC128_SIZE": 2,
	"BITVEC_SIZE": 10,
	"LST8_MAX":    6,
	"LST32_MAX":   8,
	"LST128_MAX":  2,
	"STR_MAX":     16,
	"STR_SIZE":    11,
}

type ProgressiveTypes struct {
	C1 struct {
		F1 uint64 `ssz-index:"0"`
		F3 uint64 `ssz-index:"2"`
		F7 uint8  `ssz-index:"6"`
	} `ssz-type:"progressive-container"`
	L1 []uint64 `ssz-type:"progressive-list"`
	L2 []byte   `ssz-type:"progressive-bitlist"`
	U1 dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 [2][]uint8 `ssz-size:"2,5"`
	}]
}

var ProgressiveTypes_Payload = ProgressiveTypes{
	C1: struct {
		F1 uint64 `ssz-index:"0"`
		F3 uint64 `ssz-index:"2"`
		F7 uint8  `ssz-index:"6"`
	}{
		F1: 12345,
		F3: 67890,
		F7: 123,
	},
	L1: []uint64{12345, 67890},
	L2: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	U1: dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 [2][]uint8 `ssz-size:"2,5"`
	}]{
		Variant: 0,
		Data:    uint32(0x12345678),
	},
}
