package tests

import (
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

type SimpleBool bool
type SimpleUint8 uint8
type SimpleUint16 uint16
type SimpleUint32 uint32
type SimpleUint64 uint64

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
	BitVec2  [8]byte     `ssz-type:"bitvector" ssz-bitsize:"12"`
	Lst8     []uint8     `ssz-max:"4"`
	Lst32    []uint32    `ssz-max:"4"`
	Lst128   [][2]uint64 `ssz-type:"?,uint128" ssz-max:"4"`
	BigLst8  []uint8     `ssz-max:"35"`
	BitLst   []byte      `ssz-max:"16"`
	F1       [2][]uint16
	F2       [10]uint8 `ssz-size:"5"`
	Str      string    `ssz-max:"8"`
	Wrapper1 dynssz.TypeWrapper[struct {
		Data []byte `ssz-size:"32"`
	}, []byte] `ssz-type:"wrapper"`
	Wrapper2 dynssz.TypeWrapper[struct {
		Data []uint16 `ssz-size:"2"`
	}, []uint16] `ssz-type:"wrapper"`
	S1  *SimpleTypes1_S1
	S2  [4][]*SimpleTypes1_S2
	C1  *SimpleTypes1_C1
	C2  SimpleTypes1_C1
	LC1 []SimpleTypes1_C1
	LC2 [][]*SimpleTypes1_C1
}

var SimpleTypes1_Payload = SimpleTypes1{
	B1:      true,
	I8:      1,
	I16:     2,
	I32:     3,
	I64:     4,
	I128:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	I256:    [4]uint64{1, 2, 3, 4},
	Vec8:    []uint8{1, 2, 3, 4},
	Vec32:   []uint32{1, 2, 3, 4},
	Vec128:  [][2]uint64{{1, 2}, {3, 4}},
	BitVec:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	BitVec2: [8]byte{0xff, 0x07},
	Lst8:    []uint8{1, 2, 3, 4},
	Lst32:   []uint32{1, 2, 3, 4},
	Lst128:  [][2]uint64{{1, 2}, {3, 4}},
	BigLst8: []uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35},
	BitLst:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	F1:      [2][]uint16{{1, 2}, {3, 4}},
	F2:      [10]uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	Str:     "hello",
	Wrapper1: dynssz.TypeWrapper[struct {
		Data []byte `ssz-size:"32"`
	}, []byte]{
		Data: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	},
	Wrapper2: dynssz.TypeWrapper[struct {
		Data []uint16 `ssz-size:"2"`
	}, []uint16]{
		Data: []uint16{1, 2},
	},
	S1: &SimpleTypes1_S1{
		F1: []uint16{1, 2, 3, 4},
	},
	S2: [4][]*SimpleTypes1_S2{
		{
			&SimpleTypes1_S2{
				F1: []uint16{1, 2, 3, 4},
			},
		},
	},
	C1: &SimpleTypes1_C1{
		F1: 1,
	},
	C2: SimpleTypes1_C1{
		F1: 2,
	},
	LC1: []SimpleTypes1_C1{SimpleTypes1_C1{F1: 1}},
	LC2: [][]*SimpleTypes1_C1{
		{
			&SimpleTypes1_C1{F1: 1},
		},
	},
}

type SimpleTypes1_S1 struct {
	Data []byte `ssz-size:"32"`
	F1   []uint16
}

type SimpleTypes1_S2 struct {
	F1 []uint16
}

type SimpleTypes1_C1 struct {
	F1 uint16
}

type SimpleTypes2 struct {
	F1 uint16
	F2 []*SimpleTypes2_C1 `ssz-size:"4"`
}

type SimpleTypes2_C1 struct {
	F1 []uint16   `ssz-size:"4"`
	F2 [][]uint16 `ssz-max:"4,4"`
}

var SimpleTypes2_Payload = SimpleTypes2{
	F1: 1,
	F2: []*SimpleTypes2_C1{
		{F1: []uint16{1, 2, 3, 4}},
	},
}

type TestBool bool
type TestUint8 uint8
type TestUint16 uint16
type TestUint32 uint32
type TestUint64 uint64

type SimpleTypes3 struct {
	B1       *bool
	B2       *TestBool
	I8       *uint8
	I82      *TestUint8
	I16      *uint16
	I162     *TestUint16
	I32      *uint32
	I322     *TestUint32
	I64      *uint64
	I642     *TestUint64
	I128     *[16]byte
	I256     *[4]uint64
	Vec8     []*uint8     `ssz-size:"4"`
	Vec32    []*uint32    `ssz-size:"4"`
	Vec128   []*[2]uint64 `ssz-type:"?,uint128" ssz-size:"4"`
	BitVec   [8]*byte     `ssz-type:"bitvector"`
	BitVec2  [8]*byte     `ssz-type:"bitvector" ssz-bitsize:"12"`
	Lst8     []*uint8     `ssz-max:"4"`
	Lst32    []*uint32    `ssz-max:"4"`
	Lst128   []*[2]uint64 `ssz-type:"?,uint128" ssz-max:"4"`
	BigLst8  []*uint8     `ssz-max:"35"`
	BitLst   []*byte      `ssz-max:"16"`
	F1       [2][]*uint16
	F2       [10]*uint8 `ssz-size:"5"`
	Str      *string    `ssz-max:"8"`
	Wrapper1 *dynssz.TypeWrapper[struct {
		Data []*byte `ssz-size:"32"`
	}, []*byte] `ssz-type:"wrapper"`
	Wrapper2 dynssz.TypeWrapper[struct {
		Data []*uint16 `ssz-size:"2"`
	}, []*uint16] `ssz-type:"wrapper"`
}

var (
	b1   = true
	i8   = uint8(1)
	i16  = uint16(2)
	i32  = uint32(3)
	i64  = uint64(4)
	str  = "hello"
	i128 = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	v128 = [2]uint64{1, 2}
	i256 = [4]uint64{1, 2, 3, 4}
)

var SimpleTypes3_Payload = SimpleTypes3{
	B1:      &b1,
	B2:      (*TestBool)(&b1),
	I8:      &i8,
	I82:     (*TestUint8)(&i8),
	I16:     &i16,
	I162:    (*TestUint16)(&i16),
	I32:     &i32,
	I322:    (*TestUint32)(&i32),
	I64:     &i64,
	I642:    (*TestUint64)(&i64),
	I128:    &i128,
	I256:    &i256,
	Vec8:    []*uint8{&i8, &i8, &i8, &i8},
	Vec32:   []*uint32{&i32, &i32, &i32, &i32},
	Vec128:  []*[2]uint64{&v128, &v128},
	BitVec:  [8]*byte{&i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8},
	BitVec2: [8]*byte{&i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8},
	Lst8:    []*uint8{&i8, &i8, &i8, &i8},
	Lst32:   []*uint32{&i32, &i32, &i32, &i32},
	Lst128:  []*[2]uint64{&v128, &v128},
	BigLst8: []*uint8{&i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8},
	BitLst:  []*byte{&i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8},
	F1:      [2][]*uint16{{&i16, &i16}, {&i16, &i16}},
	F2:      [10]*uint8{&i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8},
	Str:     &str,
	Wrapper1: &dynssz.TypeWrapper[struct {
		Data []*byte `ssz-size:"32"`
	}, []*byte]{
		Data: []*byte{&i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8, &i8},
	},
}

type SimpleTypesWithSpecs struct {
	Vec8    []uint8     `ssz-size:"4" dynssz-size:"VEC8_SIZE"`
	Vec32   []uint32    `ssz-size:"4" dynssz-size:"VEC32_SIZE"`
	Vec128  [][2]uint64 `ssz-type:"?,uint128" ssz-size:"4" dynssz-size:"VEC128_SIZE"`
	Vec2    [8]uint16   `ssz-size:"8" dynssz-size:"VEC2_SIZE"`
	BitVec  []byte      `ssz-type:"bitvector" ssz-size:"8" dynssz-size:"BITVEC_SIZE"`
	BitVec2 []byte      `ssz-type:"bitvector" ssz-bitsize:"12" dynssz-bitsize:"BITVEC2_SIZE"`
	Lst8    []uint8     `ssz-max:"4" dynssz-max:"LST8_MAX"`
	Lst32   []uint32    `ssz-max:"4" dynssz-max:"LST32_MAX"`
	Lst128  [][2]uint64 `ssz-type:"?,uint128" ssz-max:"4" dynssz-max:"LST128_MAX"`
	Lst2    [][]uint16  `ssz-max:"4,8" dynssz-max:"LST2_MAX"`
	BitLst  []byte      `ssz-max:"16" dynssz-max:"BITLST_MAX"`
	Str1    string      `ssz-max:"8" dynssz-max:"STR_MAX"`
	Str2    string      `ssz-size:"10" dynssz-size:"STR_SIZE"`
	C1      SimpleTypesWithSpecs_C1
	C2      []SimpleTypesWithSpecs_C2
	VC1     [2][]*SimpleTypesWithSpecs_C1
}

type SimpleTypesWithSpecs_C1 struct {
	F1 []uint16   `ssz-size:"4" dynssz-size:"F1_MAX"`
	F2 [][]uint16 `ssz-max:"4,4" dynssz-max:"F2_MAX,F2_MAX"`
	//C1 []*SimpleTypesWithSpecs_C2 `ssz-size:"4" dynssz-size:"F1_MAX"`
}

type SimpleTypesWithSpecs_C2 struct {
	F1 []uint16   `ssz-size:"4" dynssz-size:"F1_MAX"`
	F2 [][]uint16 `ssz-max:"4,4" dynssz-max:"F2_MAX,F2_MAX"`
}

type SimpleTypesWithSpecs2 struct {
	C3  [][4]*SimpleTypesWithSpecs_C3
	VC1 [2][]*SimpleTypesWithSpecs_C1
}

type SimpleTypesWithSpecs_C3 struct {
	F1 []uint16 `ssz-size:"4" dynssz-size:"F1_MAX"`
	F2 uint16
}

var SimpleTypesWithSpecs_Payload = SimpleTypesWithSpecs{
	Vec8:    []uint8{1, 2, 3, 4, 5, 6},
	Vec32:   []uint32{1, 2, 3, 4, 5, 6, 7, 8},
	Vec128:  [][2]uint64{{1, 2}, {3, 4}},
	Vec2:    [8]uint16{1, 2, 3},
	BitVec:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	BitVec2: []byte{0xff, 0x07},
	Lst8:    []uint8{1, 2, 3, 4, 5, 6},
	Lst32:   []uint32{1, 2, 3, 4, 5, 6, 7, 8},
	Lst128:  [][2]uint64{{1, 2}, {3, 4}},
	Lst2:    [][]uint16{{1, 2}, {3, 4}},
	BitLst:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
	Str1:    "hello",
	Str2:    "hello2",
	C1:      SimpleTypesWithSpecs_C1{F1: []uint16{1, 2, 3, 4}},
	C2:      []SimpleTypesWithSpecs_C2{{F1: []uint16{1, 2, 3, 4}}},
	VC1: [2][]*SimpleTypesWithSpecs_C1{
		{
			&SimpleTypesWithSpecs_C1{F1: []uint16{1, 2, 3, 4}},
		},
		{
			&SimpleTypesWithSpecs_C1{F1: []uint16{1, 2, 3, 6}},
		},
	},
}
var SimpleTypesWithSpecs2_Payload = SimpleTypesWithSpecs2{
	C3: [][4]*SimpleTypesWithSpecs_C3{{{F1: []uint16{1, 2, 3, 4}}}},
	VC1: [2][]*SimpleTypesWithSpecs_C1{
		{
			&SimpleTypesWithSpecs_C1{F1: []uint16{1, 2, 3, 4}},
		},
	},
}
var SimpleTypesWithSpecs_Specs = map[string]any{
	"VEC8_SIZE":    6,
	"VEC32_SIZE":   8,
	"VEC128_SIZE":  2,
	"VEC2_SIZE":    4,
	"BITVEC_SIZE":  10,
	"BITVEC2_SIZE": 12,
	"LST8_MAX":     6,
	"LST32_MAX":    8,
	"LST128_MAX":   2,
	"LST2_MAX":     8,
	"BITLST_MAX":   20,
	"STR_MAX":      16,
	"STR_SIZE":     11,
	"F1_MAX":       4,
	"F2_MAX":       8,
}

type ProgressiveTypes struct {
	C1 struct {
		F1 uint64      `ssz-index:"0"`
		F3 uint64      `ssz-index:"2"`
		F7 uint8       `ssz-index:"6"`
		F8 [2][]uint16 `ssz-size:"2,5" ssz-index:"9"`
	} `ssz-type:"progressive-container"`
	L1 []uint64 `ssz-type:"progressive-list"`
	L2 []byte   `ssz-type:"progressive-bitlist"`
	U1 dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 [2][]uint8 `ssz-size:"2,5"`
		F3 [4]*SimpleTypesWithSpecs_C1
	}]
}

var ProgressiveTypes_Payload = ProgressiveTypes{
	C1: struct {
		F1 uint64      `ssz-index:"0"`
		F3 uint64      `ssz-index:"2"`
		F7 uint8       `ssz-index:"6"`
		F8 [2][]uint16 `ssz-size:"2,5" ssz-index:"9"`
	}{
		F1: 12345,
		F3: 67890,
		F7: 123,
		F8: [2][]uint16{{1, 2}, {3, 4, 5}},
	},
	L1: []uint64{12345, 67890},
	L2: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	U1: dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 [2][]uint8 `ssz-size:"2,5"`
		F3 [4]*SimpleTypesWithSpecs_C1
	}]{
		Variant: 0,
		Data:    uint32(0x12345678),
	},
}

type CustomTypes1 struct {
	F1 CustomType1 `ssz-type:"custom"`
}

var CustomTypes1_Payload = CustomTypes1{
	F1: CustomType1(12345),
}

type CustomType1 uint64

var _ = sszutils.FastsszMarshaler(new(CustomType1))
var _ = sszutils.FastsszUnmarshaler(new(CustomType1))
var _ = sszutils.FastsszHashRoot(new(CustomType1))

func (c *CustomType1) MarshalSSZ() ([]byte, error) {
	buf := make([]byte, 8)
	return c.MarshalSSZTo(buf)
}

func (c *CustomType1) MarshalSSZTo(buf []byte) ([]byte, error) {
	return sszutils.MarshalUint64(buf, uint64(*c)), nil
}

func (c *CustomType1) SizeSSZ() int {
	return 8
}

func (c *CustomType1) UnmarshalSSZ(data []byte) error {
	*c = CustomType1(sszutils.UnmarshallUint64(data))
	return nil
}

func (c *CustomType1) HashTreeRoot() ([32]byte, error) {
	buf := make([]byte, 32)
	sszutils.MarshalUint64(buf, uint64(*c))
	return [32]byte(buf), nil
}
