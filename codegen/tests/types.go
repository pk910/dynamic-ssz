package tests

import (
	"encoding/binary"
	"math/big"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

type SimpleBool bool
type SimpleUint8 uint8
type SimpleUint16 uint16
type SimpleUint32 uint32
type SimpleUint64 uint64

// Standalone fixed-size vector types generated as top-level -types entries;
// their generated decoders receive the raw outer buffer and must enforce
// exact consumption themselves.
type SimpleByteVec32 [32]byte
type SimpleUint64Vec4 [4]uint64

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
	LC1: []SimpleTypes1_C1{{F1: 1}},
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
	Wrapper3 *SimpleTypes3_Wrapper3 `ssz-type:"wrapper"`
}

type SimpleTypes3_Wrapper3 struct {
	Test bool
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
	Wrapper3: &SimpleTypes3_Wrapper3{
		Test: true,
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
	// C1 []*SimpleTypesWithSpecs_C2 `ssz-size:"4" dynssz-size:"F1_MAX"`
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

// SpecMatrix is a multi-dimensional fixed vector whose inner dimension size is
// itself dynssz-resolved. It exercises the outer-row zero-padding, which must
// pad each missing row with the runtime-resolved inner byte size, not the
// static fallback.
type SpecMatrix struct {
	M [][]byte `ssz-size:"2,4" dynssz-size:"MATRIX_OUTER,MATRIX_INNER"`
}

var SpecMatrix_Payload = SpecMatrix{
	// Under-filled on both dimensions: 3 of MATRIX_OUTER rows, each shorter than
	// MATRIX_INNER, so both per-row and missing-row padding are exercised.
	M: [][]byte{{1, 2, 3}, {4, 5}, {6}},
}

var SpecMatrix_Specs = map[string]any{
	"MATRIX_OUTER": 8,
	"MATRIX_INNER": 16,
}

// OptU32 is a variable-size container ending in an optional field. Trailing
// bytes after its encoding must be rejected by both engines.
type OptU32 struct {
	Pre uint32
	Opt *uint32 `ssz-type:"optional"`
}

var OptU32_Payload = OptU32{Pre: 1, Opt: func() *uint32 { v := uint32(7); return &v }()}

// ListOfList and Bytes2D are variable-size containers ending in a list of
// variable-length elements, used to confirm trailing-data rejection.
type ListOfList struct {
	L [][]uint32 `ssz-max:"4,4"`
}

var ListOfList_Payload = ListOfList{L: [][]uint32{{1, 2}, {3}}}

type Bytes2D struct {
	B [][]byte `ssz-max:"4,8"`
}

var Bytes2D_Payload = Bytes2D{B: [][]byte{{1, 2}, {3}}}

// VecOfList is a fixed-size vector of variable-size elements, exercising the
// inner offset-table validation distinct from a dynamic list.
type VecOfList struct {
	V [3][]uint16
}

var VecOfList_Payload = VecOfList{V: [3][]uint16{{1, 2}, {3}, {4, 5}}}

// ProgIndexOnly is a progressive container detected purely from ssz-index tags
// (no explicit ssz-type:"progressive-container"), exercising codegen auto-detection.
type ProgIndexOnly struct {
	A uint8 `ssz-index:"0"`
	B uint8 `ssz-index:"5"`
	C uint8 `ssz-index:"7"`
}

var ProgIndexOnly_Payload = ProgIndexOnly{A: 1, B: 2, C: 3}

// ZeroMaxList exercises ssz-max:"0": the limit check must fire (codegen used to
// treat a zero limit as "no limit").
type ZeroMaxList struct {
	X []uint64 `ssz-max:"0"`
}

var ZeroMaxList_Payload = ZeroMaxList{X: []uint64{1, 2, 3}}

// ProgBitlistZeroTop reproduces the EIP-7916 progressive-bitlist HTR bug: a
// bitlist whose highest data bits are zero leaves the top 256-bit chunk all-zero.
// The progressive tree shape is defined by the chunk count (ceil(size/256)), so
// the all-zero top chunk must NOT be dropped. The golden root in
// TestCodegenProgressiveBitlistGolden is cross-checked against
// ethereum/remerkleable and an independent raw-SHA256 implementation; the
// reflection and codegen paths share the hasher, so only a golden (not a
// differential) test catches a regression here.
type ProgBitlistZeroTop struct {
	X []byte `ssz-type:"progressive-bitlist" ssz-max:"2000"`
}

// progBitlistZeroTopBits builds a 257-bit bitlist with only bit 0 set, so bit 256
// (the single data bit of the 2nd chunk) is zero and the top chunk is all-zero.
func progBitlistZeroTopBits() []byte {
	b := make([]byte, 257/8+1) // 33 bytes
	b[0] = 0x01                // bit 0 set
	b[257/8] |= 1 << (257 % 8) // termination bit at position 257
	return b
}

var ProgBitlistZeroTop_Payload = ProgBitlistZeroTop{X: progBitlistZeroTopBits()}

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
		Variant: 1,
		Data:    uint32(0x12345678),
	},
}

// ClassicUnionTypes exercises classic spec unions (dynssz.Union) through the
// generated code: a None-declaring union with the empty option selected (U1),
// the same union type with its dynamic variant selected (U2), and a union
// without None whose selector 0 carries a value (U3).
type ClassicUnionTypes struct {
	U1 dynssz.Union[struct {
		N  dynssz.None
		F1 uint32
		F2 []uint8 `ssz-max:"16"`
	}]
	U2 dynssz.Union[struct {
		N  dynssz.None
		F1 uint32
		F2 []uint8 `ssz-max:"16"`
	}]
	U3 dynssz.Union[struct {
		F1 uint64
		F2 [2][]uint16 `ssz-size:"2,5"`
	}]
}

var ClassicUnionTypes_Payload = ClassicUnionTypes{
	// U1 stays zero-valued: Variant 0 with nil Data is the None option.
	U2: dynssz.Union[struct {
		N  dynssz.None
		F1 uint32
		F2 []uint8 `ssz-max:"16"`
	}]{
		Variant: 2,
		Data:    []uint8{1, 2, 3},
	},
	U3: dynssz.Union[struct {
		F1 uint64
		F2 [2][]uint16 `ssz-size:"2,5"`
	}]{
		Variant: 0,
		Data:    uint64(0x1122334455667788),
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

type ViewTypes1_Base struct {
	F1 uint64
	F2 []uint64
	F3 [2][]uint64
	C1 *ViewTypes1_C1
}

type ViewTypes1_C1 struct {
	F1 uint64
	F2 []uint64
}

type ViewTypes1_View1 struct {
	F1 uint64
	F3 [2][]uint64 `ssz-size:"2,5"`
	C1 *ViewTypes1_View1_C1
}

type ViewTypes1_View1_C1 struct {
	F1 uint64
}

type ViewTypes1_View2 struct {
	F1 uint64
	F2 []uint64
	C1 *ViewTypes1_View2_C1
}

type ViewTypes1_View2_C1 struct {
	F2 []uint64
}

var ViewTypes1_Payload = ViewTypes1_Base{
	F1: 12345,
	F2: []uint64{12345, 67890},
	F3: [2][]uint64{{12345, 67890}, {12345, 67890}},
	C1: &ViewTypes1_C1{
		F1: 12345,
		F2: []uint64{12345, 67890},
	},
}

// OptionalListTypes exercises ssz-type:"optional-list" — pointers encoded as
// canonical List[T, 1]. Static and dynamic element pointers are covered, and
// optional-list works without extended types.
type OptionalListTypes struct {
	StaticOpt  *uint32                  `ssz-type:"optional-list"`
	DynamicOpt *OptionalListTypes_Inner `ssz-type:"optional-list"`
}

type OptionalListTypes_Inner struct {
	Tag  uint16
	Data []byte `ssz-max:"16"`
}

var optListInner = &OptionalListTypes_Inner{
	Tag:  0xbeef,
	Data: []byte{1, 2, 3, 4, 5},
}

var optListU32 = uint32(1337)

// Both pointers populated.
var OptionalListTypes_Payload1 = OptionalListTypes{
	StaticOpt:  &optListU32,
	DynamicOpt: optListInner,
}

// OptionalListSliceVector exercises ssz-type:"optional-list" over a pointer to a
// SLICE whose fixed length comes from ssz-size (a vector), plus the byte-vector
// form. These are spec-identical to an optional-list over a Go array
// (*[2]uint16 / *[4]byte): the optional-list frames the pointer as List[T, 1]
// where T is the fixed-size inner vector. The slice spelling only stays correct
// if the element's ssz-size survives into the element descriptor — otherwise the
// inner vector degrades to a variable list (wrong marshal + wrong root).
type OptionalListSliceVector struct {
	VecU16  *[]uint16 `ssz-size:"2" ssz-type:"optional-list"`
	VecByte *[]byte   `ssz-size:"4" ssz-type:"optional-list"`
}

var (
	optSliceVecU16  = []uint16{0x1234, 0x5678}
	optSliceVecByte = []byte{0xaa, 0xbb, 0xcc, 0xdd}
)

// Both optional vectors present.
var OptionalListSliceVector_Payload1 = OptionalListSliceVector{
	VecU16:  &optSliceVecU16,
	VecByte: &optSliceVecByte,
}

// Both nil.
var OptionalListSliceVector_Payload2 = OptionalListSliceVector{}

// Only the uint16 vector present.
var OptionalListSliceVector_Payload3 = OptionalListSliceVector{
	VecU16: &optSliceVecU16,
}

// UnionExprVariantSize is a regression type for the generated union size code.
// The union variant is a vector whose length comes from a dynssz-size
// expression, so its size is computed purely from that expression and never
// reads the type-asserted variant value. Without an explicit `_ = v`, the
// generated size code left `v` declared-and-unused and failed to compile.
type UnionExprVariantSize struct {
	U dynssz.CompatibleUnion[struct {
		F0 []uint16 `ssz-size:"4" dynssz-size:"UNION_VEC_SIZE"`
	}]
}

var UnionExprVariantSize_Specs = map[string]any{
	"UNION_VEC_SIZE": uint64(4),
}

var UnionExprVariantSize_Payload = UnionExprVariantSize{
	U: dynssz.CompatibleUnion[struct {
		F0 []uint16 `ssz-size:"4" dynssz-size:"UNION_VEC_SIZE"`
	}]{Variant: 1, Data: []uint16{1, 2, 3, 4}},
}

// VecDynElemExprSize is a regression type for the generated stream decoder: a
// vector of dynamic-size elements whose length comes from a dynssz-size
// expression. The decoder's first-offset check compares the uint32 offset
// against `<len-expr>*4`; when the length is a typed int expression the RHS must
// be cast to uint32, otherwise the generated code failed to compile.
type VecDynElemExprSize_Inner struct {
	D []byte `ssz-max:"8"`
}

type VecDynElemExprSize struct {
	V []VecDynElemExprSize_Inner `ssz-size:"2" dynssz-size:"VDE_SIZE"`
}

var VecDynElemExprSize_Specs = map[string]any{
	"VDE_SIZE": uint64(2),
}

var VecDynElemExprSize_Payload = VecDynElemExprSize{
	V: []VecDynElemExprSize_Inner{
		{D: []byte{1, 2, 3}},
		{D: []byte{4, 5}},
	},
}

// Both pointers nil.
var OptionalListTypes_Payload2 = OptionalListTypes{
	StaticOpt:  nil,
	DynamicOpt: nil,
}

// Mixed: static populated, dynamic nil.
var OptionalListTypes_Payload3 = OptionalListTypes{
	StaticOpt:  &optListU32,
	DynamicOpt: nil,
}

type ExtendedTypes1 struct {
	I8   int8
	I16  int16
	I32  int32
	I64  int64
	F32  float32
	F64  float64
	Opt1 *uint64 `ssz-type:"optional"`
	Opt2 *int32  `ssz-type:"optional"`
	Big1 big.Int
}

var (
	extOpt1 = uint64(12345)
	extOpt2 = int32(-42)
)

var ExtendedTypes1_Payload1 = ExtendedTypes1{
	I8:   -42,
	I16:  -1337,
	I32:  817482215,
	I64:  -848028848028,
	F32:  3.14,
	F64:  2.718281828,
	Opt1: &extOpt1,
	Opt2: &extOpt2,
	Big1: *big.NewInt(123456789),
}

var ExtendedTypes1_Payload2 = ExtendedTypes1{
	I8:   0,
	I16:  0,
	I32:  0,
	I64:  0,
	F32:  0,
	F64:  0,
	Opt1: nil,
	Opt2: nil,
	Big1: *big.NewInt(0),
}

// ExtendedBigIntMax carries a limit-bearing big.Int, exercising the ssz-max
// enforcement on the encode and decode paths of both engines.
type ExtendedBigIntMax struct {
	B big.Int `ssz-max:"5"`
}

var ExtendedBigIntMax_Payload = ExtendedBigIntMax{B: *big.NewInt(0x11223344)}

// CoverageTypes1 covers: regular bitlists, bitlists with spec max,
// dynamic vectors/lists (slice-based with zero-padding), bool vectors.
type CoverageTypes1 struct {
	BitLst1 []byte                     `ssz-type:"bitlist" ssz-max:"16"`                         // regular bitlist with static max
	BitLst2 []byte                     `ssz-type:"bitlist" ssz-max:"16" dynssz-max:"BITLST_MAX"` // regular bitlist with spec max
	DynVec1 []*CoverageTypes1_DynChild `ssz-size:"4"`                                            // slice vector of dynamic ptrs
	DynLst1 []*CoverageTypes1_DynChild `ssz-max:"4"`                                             // list of dynamic ptrs
	DynVec2 []CoverageTypes1_DynChild  `ssz-size:"2"`                                            // slice vector of dynamic values
	DynLst2 []CoverageTypes1_DynChild  `ssz-max:"4"`                                             // list of dynamic values
	VecBool [4]bool                    // array vector of bools (hash pack mode)
	LstBool []bool                     `ssz-max:"8"`  // list of bools
	VecU16  []uint16                   `ssz-size:"4"` // slice vector of uint16 (hash pack mode)
	LstU16  []uint16                   `ssz-max:"8"`  // list of uint16
}

type CoverageTypes1_DynChild struct {
	F1 uint32
	F2 []byte `ssz-max:"8"` // makes it dynamic
}

var CoverageTypes1_Payload = CoverageTypes1{
	BitLst1: []byte{0x03},
	BitLst2: []byte{0x07},
	DynVec1: []*CoverageTypes1_DynChild{
		{F1: 1, F2: []byte{1, 2}},
		{F1: 2, F2: []byte{3, 4, 5}},
	},
	DynLst1: []*CoverageTypes1_DynChild{
		{F1: 3, F2: []byte{6}},
	},
	DynVec2: []CoverageTypes1_DynChild{
		{F1: 4, F2: []byte{7, 8}},
	},
	DynLst2: []CoverageTypes1_DynChild{
		{F1: 5, F2: []byte{9, 10, 11}},
	},
	VecBool: [4]bool{true, false, true, false},
	LstBool: []bool{true, true, false},
	VecU16:  []uint16{100, 200, 300, 400},
	LstU16:  []uint16{500, 600},
}

// CoverageTypes2 covers: time.Time, pointer extended types, pointer big.Int,
// vectors/lists of extended types (pack-mode hashing, sizeType paths).
type CoverageTypes2 struct {
	T1     time.Time
	I8p    *int8
	I16p   *int16
	I32p   *int32
	I64p   *int64
	F32p   *float32
	F64p   *float64
	Bigp   *big.Int
	VecI8  [4]int8    // vector of int8 (hash pack mode)
	LstI16 []int16    `ssz-max:"8"`  // list of int16
	VecI32 []int32    `ssz-size:"4"` // vector of int32 (slice)
	LstI64 []int64    `ssz-max:"4"`  // list of int64
	VecF32 [2]float32 // vector of float32
	LstF64 []float64  `ssz-max:"4"` // list of float64
}

var (
	covI8  = int8(-42)
	covI16 = int16(-1337)
	covI32 = int32(817482215)
	covI64 = int64(-848028848028)
	covF32 = float32(3.14)
	covF64 = float64(2.718281828)
)

var CoverageTypes2_Payload1 = CoverageTypes2{
	T1:     time.Unix(1234567890, 0),
	I8p:    &covI8,
	I16p:   &covI16,
	I32p:   &covI32,
	I64p:   &covI64,
	F32p:   &covF32,
	F64p:   &covF64,
	Bigp:   big.NewInt(42),
	VecI8:  [4]int8{-1, 2, -3, 4},
	LstI16: []int16{100, -200, 300},
	VecI32: []int32{1000, -2000, 3000, -4000},
	LstI64: []int64{100000, -200000},
	VecF32: [2]float32{1.5, -2.5},
	LstF64: []float64{3.14, -2.718},
}

var CoverageTypes2_Payload2 = CoverageTypes2{
	T1:     time.Time{},
	I8p:    nil,
	I16p:   nil,
	I32p:   nil,
	I64p:   nil,
	F32p:   nil,
	F64p:   nil,
	Bigp:   nil,
	VecI8:  [4]int8{},
	LstI16: nil,
	VecI32: []int32{0, 0, 0, 0},
	LstI64: nil,
	VecF32: [2]float32{},
	LstF64: nil,
}

// Annotated non-struct types for testing sszutils.Annotate-based SSZ annotations.

// AnnotatedList is a basic annotated list of uint32 with max 20.
type AnnotatedList []uint32

var _ = sszutils.Annotate[AnnotatedList](`ssz-max:"20"`)

// AnnotatedList2 is a basic annotated list of uint64 with max 10.
type AnnotatedList2 []uint64

var _ = sszutils.Annotate[AnnotatedList2](`ssz-max:"10"`)

// AnnotatedByteList is an annotated byte list with max 32.
type AnnotatedByteList []byte

var _ = sszutils.Annotate[AnnotatedByteList](`ssz-max:"32"`)

// AnnotatedWithSpecs uses a dynamic spec expression for the max size.
type AnnotatedWithSpecs []uint32

var _ = sszutils.Annotate[AnnotatedWithSpecs](`ssz-max:"10" dynssz-max:"ANNOTATED_MAX"`)

// AnnotatedZeroStaticMax has only a placeholder static ssz-max (0); its real
// limit must come from the dynssz expression. When that expression resolves to 0
// there is no positive fallback, so both engines must error.
type AnnotatedZeroStaticMax []uint32

var _ = sszutils.Annotate[AnnotatedZeroStaticMax](`ssz-max:"0" dynssz-max:"ZEROSTATIC_MAX"`)

// AnnotatedFixedVec is an annotated FIXED-size byte vector. As a container
// field it is a static type and must be embedded inline without an offset.
type AnnotatedFixedVec []byte

var _ = sszutils.Annotate[AnnotatedFixedVec](`ssz-size:"8"`)

// AnnotatedFixedContainer embeds an annotated fixed-size type; wire layout
// must match the reflection descriptor (B inline, no offset).
type AnnotatedFixedContainer struct {
	A uint32
	B AnnotatedFixedVec
}

var AnnotatedFixedContainer_Payload = AnnotatedFixedContainer{
	A: 1,
	B: AnnotatedFixedVec{1, 2, 3, 4, 5, 6, 7, 8},
}

// AnnotatedContainer uses annotated types as fields WITHOUT field tags.
// The reflection path must resolve limits from the annotation registry;
// the codegen path delegates to each field's generated methods.
type AnnotatedContainer struct {
	F1 uint32
	L1 AnnotatedList     // limit 20 from annotation
	L2 AnnotatedList2    // limit 10 from annotation
	B1 AnnotatedByteList // limit 32 from annotation
}

var AnnotatedContainer_Payload = AnnotatedContainer{
	F1: 42,
	L1: AnnotatedList{1, 2, 3},
	L2: AnnotatedList2{100, 200},
	B1: AnnotatedByteList{0xaa, 0xbb, 0xcc, 0xdd},
}

// AnnotatedOverrideContainer has a field tag that overrides the annotation.
// AnnotatedList has ssz-max:"20" from its annotation, but the field tag
// narrows it to ssz-max:"5".
type AnnotatedOverrideContainer struct {
	F1 uint32
	L1 AnnotatedList `ssz-max:"5"`
}

var AnnotatedOverrideContainer_Payload = AnnotatedOverrideContainer{
	F1: 7,
	L1: AnnotatedList{10, 20, 30},
}

// AnnotatedSpecsContainer uses an annotated type with dynamic spec expressions.
type AnnotatedSpecsContainer struct {
	F1 uint32
	L1 AnnotatedWithSpecs // limit from ANNOTATED_MAX spec
}

var AnnotatedSpecsContainer_Payload = AnnotatedSpecsContainer{
	F1: 99,
	L1: AnnotatedWithSpecs{1, 2, 3, 4, 5},
}

var AnnotatedSpecs = map[string]any{
	"ANNOTATED_MAX": 20,
}

// AnnotatedNestedContainer uses annotated types at multiple nesting levels.
type AnnotatedNestedContainer struct {
	F1   uint32
	L1   AnnotatedList                // direct annotated field
	Lst  []AnnotatedList              `ssz-max:"4"` // list of annotated lists
	Sub  AnnotatedNestedContainer_S   // nested struct with annotated fields
	Subs []AnnotatedNestedContainer_S `ssz-max:"3"`
}

type AnnotatedNestedContainer_S struct {
	V1 uint16
	L1 AnnotatedByteList // annotated field inside nested struct
}

var AnnotatedNestedContainer_Payload = AnnotatedNestedContainer{
	F1: 1,
	L1: AnnotatedList{10, 20},
	Lst: []AnnotatedList{
		{1, 2, 3},
		{4, 5},
	},
	Sub: AnnotatedNestedContainer_S{
		V1: 100,
		L1: AnnotatedByteList{0x01, 0x02, 0x03},
	},
	Subs: []AnnotatedNestedContainer_S{
		{V1: 200, L1: AnnotatedByteList{0x0a}},
		{V1: 300, L1: AnnotatedByteList{0x0b, 0x0c}},
	},
}

// InitAnnotatedList tests Annotate calls inside init() functions.
type InitAnnotatedList []uint16

func init() {
	sszutils.Annotate[InitAnnotatedList](`ssz-max:"8"`)
}

// CoverageTypes7 exercises getStaticSizeVar's container recursion path:
// SC1 is a static container with a dynssz-size field, used before
// a dynamic field L1, forcing offset computation via getStaticSizeVar.
type CoverageTypes7 struct {
	SC1 CoverageTypes7_SC
	L1  []uint16 `ssz-max:"8"`
}

type CoverageTypes7_SC struct {
	F1 []uint8 `ssz-size:"4" dynssz-size:"SC_SIZE"`
	F2 uint32
}

var CoverageTypes7_Payload = CoverageTypes7{
	SC1: CoverageTypes7_SC{
		F1: []uint8{1, 2, 3, 4, 5, 6},
		F2: 42,
	},
	L1: []uint16{100, 200, 300},
}

var CoverageTypes7_Specs = map[string]any{
	"SC_SIZE": 6,
}

// NoDynExprTypes mirrors common SSZ patterns but is generated with
// -without-dynamic-expressions to exercise the expression-stripping
// branches in every generator.
type NoDynExprTypes struct {
	Vec8   []uint8  `ssz-size:"4" dynssz-size:"VEC8_SIZE"`
	Vec32  []uint32 `ssz-size:"4" dynssz-size:"VEC32_SIZE"`
	BitVec []byte   `ssz-type:"bitvector" ssz-size:"8" dynssz-size:"BITVEC_SIZE"`
	Lst8   []uint8  `ssz-max:"4" dynssz-max:"LST8_MAX"`
	Lst32  []uint32 `ssz-max:"4" dynssz-max:"LST32_MAX"`
	BitLst []byte   `ssz-max:"16" dynssz-max:"BITLST_MAX"`
	Str1   string   `ssz-max:"8" dynssz-max:"STR_MAX"`
}

var NoDynExprTypes_Payload = NoDynExprTypes{
	Vec8:   []uint8{1, 2, 3, 4},
	Vec32:  []uint32{1, 2, 3, 4},
	BitVec: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	Lst8:   []uint8{1, 2, 3, 4},
	Lst32:  []uint32{1, 2, 3, 4},
	BitLst: []byte{1, 2, 3, 4},
	Str1:   "hello",
}

// NoDynNestChild is a variable-size container nested by the NoDynNest* parents.
// Generated with -with-streaming -without-fastssz -without-dynamic-expressions,
// its parents must reach it through its static MarshalSSZTo/UnmarshalSSZ/SizeSSZ/
// HashTreeRootWith methods — never the *Dyn buffer variants and never by wrapping
// the streaming encoder into the static buffer path.
type NoDynNestChild struct {
	A []byte `ssz-max:"8"`
	B uint8
}

// NoDynNestProg nests the child as a progressive-list.
type NoDynNestProg struct {
	L []NoDynNestChild `ssz-type:"progressive-list" ssz-max:"100"`
}

// NoDynNestList nests the child as a bounded list.
type NoDynNestList struct {
	L []NoDynNestChild `ssz-max:"100"`
}

// NoDynNestVec nests the child as a fixed vector.
type NoDynNestVec struct {
	V [3]NoDynNestChild
}

// NoDynNestField nests the child as a plain container field.
type NoDynNestField struct {
	C NoDynNestChild
	N uint16
}

var (
	NoDynNestProg_Payload = NoDynNestProg{L: []NoDynNestChild{
		{A: []byte{1, 2, 3}, B: 7},
		{A: nil, B: 0},
		{A: []byte{9}, B: 255},
	}}
	NoDynNestList_Payload = NoDynNestList{L: []NoDynNestChild{
		{A: []byte{4, 5}, B: 1},
		{A: []byte{6, 7, 8}, B: 2},
	}}
	NoDynNestVec_Payload = NoDynNestVec{V: [3]NoDynNestChild{
		{A: []byte{1}, B: 10},
		{A: []byte{2, 2}, B: 20},
		{A: nil, B: 30},
	}}
	NoDynNestField_Payload = NoDynNestField{
		C: NoDynNestChild{A: []byte{3, 3, 3}, B: 42},
		N: 1234,
	}
)

// InterpretedAnnotatedList tests Annotate with an interpreted (double-quoted) string literal.
type InterpretedAnnotatedList []uint32

var _ = sszutils.Annotate[InterpretedAnnotatedList]("ssz-max:\"12\"")

// ViewTypes2_Base tests nested view dispatch: its Child field has
// view dispatch methods (from ViewTypes1_Base). When generating
// ViewTypes2's views, the Child field triggers the isView code path
// in all generators (marshal, unmarshal, encoder, decoder, size, hash).
type ViewTypes2_Base struct {
	F1    uint64
	Child *ViewTypes1_Base
}

// ViewTypes2_View1 is a view where Child is viewed through ViewTypes1_View1.
type ViewTypes2_View1 struct {
	F1    uint64
	Child *ViewTypes1_View1
}

// ViewTypes2_View2 is a view where Child is viewed through ViewTypes1_View2.
type ViewTypes2_View2 struct {
	F1    uint64
	Child *ViewTypes1_View2
}

var ViewTypes2_Payload = ViewTypes2_Base{
	F1: 42,
	Child: &ViewTypes1_Base{
		F1: 12345,
		F2: []uint64{12345, 67890},
		F3: [2][]uint64{{12345, 67890}, {12345, 67890}},
		C1: &ViewTypes1_C1{
			F1: 12345,
			F2: []uint64{12345, 67890},
		},
	},
}

// ViewTypes3_Base tests the view-only generation mode.
// It only generates view dispatch methods, no data methods.
type ViewTypes3_Base struct {
	F1 uint64
	F2 []uint64
}

// ViewTypes3_View1 is a view for ViewTypes3_Base.
type ViewTypes3_View1 struct {
	F1 uint64
}

var ViewTypes3_Payload = ViewTypes3_Base{
	F1: 42,
	F2: []uint64{1, 2, 3},
}

// CoverageTypes3 covers: uint256 type (both [32]byte and [4]uint64 forms),
// pointer bitlist, and string vector with dynssz-size.
type CoverageTypes3 struct {
	U256_1    [32]byte  `ssz-type:"uint256"`
	U256_2    [4]uint64 `ssz-type:"uint256"`
	StrVec    string    `ssz-size:"10" dynssz-size:"STR_VEC_SIZE"`
	PtrBool   *bool
	PtrStrVec *string `ssz-size:"8"`
	PtrStrLst *string `ssz-max:"16"`
}

var (
	covBool = true
	covStr1 = "teststr1"
	covStr2 = "hello"
)

var CoverageTypes3_Payload = CoverageTypes3{
	U256_1:    [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
	U256_2:    [4]uint64{1, 2, 3, 4},
	StrVec:    "helloworld",
	PtrBool:   &covBool,
	PtrStrVec: &covStr1,
	PtrStrLst: &covStr2,
}

var CoverageTypes3_Specs = map[string]any{
	"STR_VEC_SIZE": 12,
}

// EncoderOnlyType implements only dynamic encoder/decoder/sizer/hashroot
// interfaces (not FastSSZ or DynamicMarshaler/Unmarshaler). This exercises
// the DynamicEncoder/DynamicDecoder dispatch branches in codegen.
type EncoderOnlyType struct {
	Val uint64
}

var _ sszutils.DynamicEncoder = (*EncoderOnlyType)(nil)
var _ sszutils.DynamicDecoder = (*EncoderOnlyType)(nil)
var _ sszutils.DynamicSizer = (*EncoderOnlyType)(nil)
var _ sszutils.DynamicHashRoot = (*EncoderOnlyType)(nil)

func (e *EncoderOnlyType) MarshalSSZEncoder(ds sszutils.DynamicSpecs, enc sszutils.Encoder) error {
	enc.EncodeUint64(e.Val)
	return nil
}

func (e *EncoderOnlyType) UnmarshalSSZDecoder(_ sszutils.DynamicSpecs, dec sszutils.Decoder) error {
	var err error
	e.Val, err = dec.DecodeUint64()
	return err
}

func (e *EncoderOnlyType) SizeSSZDyn(_ sszutils.DynamicSpecs) int {
	return 8
}

func (e *EncoderOnlyType) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(e.Val)
	return nil
}

// CoverageTypes5 wraps EncoderOnlyType as a field to trigger the
// DynamicEncoder/DynamicDecoder dispatch branches in all generators.
type CoverageTypes5 struct {
	F1 uint64
	E1 EncoderOnlyType
}

var CoverageTypes5_Payload = CoverageTypes5{
	F1: 42,
	E1: EncoderOnlyType{Val: 12345},
}

// MarshalerOnlyType implements only DynamicMarshaler/Unmarshaler (not FastSSZ).
// This exercises the DynamicMarshaler/DynamicUnmarshaler dispatch branches.
type MarshalerOnlyType struct {
	Val uint64
}

var _ sszutils.DynamicMarshaler = (*MarshalerOnlyType)(nil)
var _ sszutils.DynamicUnmarshaler = (*MarshalerOnlyType)(nil)
var _ sszutils.DynamicSizer = (*MarshalerOnlyType)(nil)
var _ sszutils.DynamicHashRoot = (*MarshalerOnlyType)(nil)

func (m *MarshalerOnlyType) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return sszutils.MarshalUint64(buf, m.Val), nil
}

func (m *MarshalerOnlyType) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	m.Val = sszutils.UnmarshallUint64(buf)
	return nil
}

func (m *MarshalerOnlyType) SizeSSZDyn(_ sszutils.DynamicSpecs) int {
	return 8
}

func (m *MarshalerOnlyType) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(m.Val)
	return nil
}

// CoverageTypes6 wraps MarshalerOnlyType as a field to trigger the
// DynamicMarshaler/DynamicUnmarshaler dispatch branches.
type CoverageTypes6 struct {
	F1 uint64
	M1 MarshalerOnlyType
}

var CoverageTypes6_Payload = CoverageTypes6{
	F1: 99,
	M1: MarshalerOnlyType{Val: 67890},
}

// ViewTypes4_Base is generated in a SEPARATE go:generate command from
// ViewTypes1_Base. This forces the parser to discover ViewTypes1_Base's
// view methods via method set detection rather than the internal compat
// flag map. It also includes CompatibleUnion and TypeWrapper fields with
// view-dependent schemas to exercise the view descriptor paths in
// buildCompatibleUnionDescriptor and buildTypeWrapperDescriptor.
type ViewTypes4_Base struct {
	F1    uint64
	Child *ViewTypes1_Base
	U1    dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
	}]
	W1 dynssz.TypeWrapper[struct {
		Data []byte `ssz-size:"32"`
	}, []byte] `ssz-type:"wrapper"`
}

// ViewTypes4_View1 uses different union variants and wrapper sizes.
type ViewTypes4_View1 struct {
	F1    uint64
	Child *ViewTypes1_View1
	U1    dynssz.CompatibleUnion[struct {
		F1 uint32
	}]
	W1 dynssz.TypeWrapper[struct {
		Data []byte `ssz-size:"16"`
	}, []byte] `ssz-type:"wrapper"`
}

var ViewTypes4_Payload = ViewTypes4_Base{
	F1: 42,
	Child: &ViewTypes1_Base{
		F1: 12345,
		F2: []uint64{12345, 67890},
		F3: [2][]uint64{{12345, 67890}, {12345, 67890}},
		C1: &ViewTypes1_C1{
			F1: 12345,
			F2: []uint64{12345, 67890},
		},
	},
	U1: dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
	}]{
		Variant: 1,
		Data:    uint32(0x12345678),
	},
	W1: dynssz.TypeWrapper[struct {
		Data []byte `ssz-size:"32"`
	}, []byte]{
		Data: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	},
}

// CoverageTypes4 covers: dynamic vector of extended types with size expressions,
// union with multiple variants, additional optional type patterns.
type CoverageTypes4 struct {
	U1 dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
		F3 []uint16 `ssz-size:"4"`
	}]
	U1V1 dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
		F3 []uint16 `ssz-size:"4"`
	}]
	U1V2 dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
		F3 []uint16 `ssz-size:"4"`
	}]
	Opt3 *uint16 `ssz-type:"optional"`
	Opt4 *bool   `ssz-type:"optional"`
}

var (
	covU16 = uint16(42)
	covB2  = true
)

var CoverageTypes4_Payload = CoverageTypes4{
	U1: dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
		F3 []uint16 `ssz-size:"4"`
	}]{
		Variant: 1,
		Data:    uint32(0x12345678),
	},
	U1V1: dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
		F3 []uint16 `ssz-size:"4"`
	}]{
		Variant: 2,
		Data:    uint64(0xdeadbeef),
	},
	U1V2: dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 uint64
		F3 []uint16 `ssz-size:"4"`
	}]{
		Variant: 3,
		Data:    []uint16{1, 2, 3, 4},
	},
	Opt3: &covU16,
	Opt4: &covB2,
}

// nestedDelegatedInner is a hand-written, fully-delegated SSZ type carrying a
// structurally-invalid innard (a zero-length array). When it is referenced by a
// generated type the parser must shallow-build it via its ssz-static annotation
// and never traverse into Bad, which the spec validations would otherwise reject.
type nestedDelegatedInner struct {
	Bad   [0]uint64 // illegal Vector[uint64, 0] if ever traversed
	Value uint32
}

var _ = sszutils.Annotate[nestedDelegatedInner](`ssz-static:"true"`)

func (n *nestedDelegatedInner) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 4 }

func (n *nestedDelegatedInner) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint32(buf, n.Value), nil
}

func (n *nestedDelegatedInner) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	// The caller (a fixed 4-byte static field) always slices exactly 4 bytes.
	n.Value = binary.LittleEndian.Uint32(buf)
	return nil
}

func (n *nestedDelegatedInner) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint32(n.Value)
	return nil
}

// NestedDelegatedContainer is generated and references the fully-delegated inner
// type as a static field. Generating it succeeds only because the parser
// shallow-builds the inner type instead of traversing its illegal innard.
type NestedDelegatedContainer struct {
	A uint64
	N nestedDelegatedInner
}

var NestedDelegatedContainer_Payload = NestedDelegatedContainer{
	A: 7,
	N: nestedDelegatedInner{Value: 0x11223344},
}

// nestedDelegatedDyn is the variable-size counterpart of nestedDelegatedInner: a
// fully-delegated type declaring ssz-static:"false". It also carries an illegal
// innard that the parser must not traverse.
type nestedDelegatedDyn struct {
	Bad   [0]uint64 // illegal Vector[uint64, 0] if ever traversed
	Items []uint32
}

var _ = sszutils.Annotate[nestedDelegatedDyn](`ssz-static:"false"`)

func (n *nestedDelegatedDyn) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return len(n.Items) * 4 }

func (n *nestedDelegatedDyn) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	for _, v := range n.Items {
		buf = binary.LittleEndian.AppendUint32(buf, v)
	}
	return buf, nil
}

func (n *nestedDelegatedDyn) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.Items = make([]uint32, len(buf)/4)
	for i := range n.Items {
		n.Items[i] = binary.LittleEndian.Uint32(buf[i*4:])
	}
	return nil
}

func (n *nestedDelegatedDyn) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	idx := hh.Index()
	for _, v := range n.Items {
		hh.AppendUint32(v)
	}
	hh.FillUpTo32()
	hh.MerkleizeWithMixin(idx, uint64(len(n.Items)), 1024)
	return nil
}

// NestedDelegatedDynContainer references the variable-size fully-delegated type
// as a dynamic field, exercising the parser gate's ssz-static:"false" branch.
type NestedDelegatedDynContainer struct {
	A uint64
	N nestedDelegatedDyn
}

var NestedDelegatedDynContainer_Payload = NestedDelegatedDynContainer{
	A: 9,
	N: nestedDelegatedDyn{Items: []uint32{1, 2, 3}},
}

// Types below exercise codegen-vs-reflection layout/root agreement for
// tricky descriptor shapes (multi-dim spec sizes, bitlists without limits,
// name-detected bitlists, bitvector edge cases).

// MultiDimSpecVec has a fixed-size multi-dim vector with spec expressions;
// SizeSSZ must equal len(MarshalSSZ) even for a fully-empty value (the
// missing rows are zero-padded on the wire).
type MultiDimSpecVec struct {
	M [][]byte `ssz-size:"2,4" dynssz-size:"SPEC_OUTER,SPEC_INNER"`
}

// NoMaxBitlist is a bitlist without any ssz-max limit; codegen and
// reflection must produce the same hash tree root.
type NoMaxBitlist struct {
	B1 []byte `ssz-type:"bitlist"`
}

// NamedBitlistT must be detected as a bitlist by its type name, matching
// the reflection typecache heuristic.
type NamedBitlistT []byte

// NamedBitlistContainer references the name-detected bitlist with a limit.
type NamedBitlistContainer struct {
	B NamedBitlistT `ssz-max:"100"`
}

// BitvecEdge covers bitvector edge cases: empty values, byte-aligned
// dynamic bit sizes, and short values for a bit-aligned size.
type BitvecEdge struct {
	BV1 []byte `ssz-type:"bitvector" ssz-bitsize:"16"`
	BV2 []byte `ssz-type:"bitvector" ssz-bitsize:"16" dynssz-bitsize:"BIT_SPEC"`
	BV3 []byte `ssz-type:"bitvector" ssz-bitsize:"12"`
}

// nestedDelegatedInner8 is a second fully-delegated static type with a
// DIFFERENT size (8 bytes) than nestedDelegatedInner (4 bytes). Two shallow
// delegated descriptors must never share a size variable in generated code.
type nestedDelegatedInner8 struct {
	Bad   [0]uint64 // illegal Vector[uint64, 0] if ever traversed
	Value uint64
}

var _ = sszutils.Annotate[nestedDelegatedInner8](`ssz-static:"true"`)

func (n *nestedDelegatedInner8) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 8 }

func (n *nestedDelegatedInner8) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint64(buf, n.Value), nil
}

func (n *nestedDelegatedInner8) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.Value = binary.LittleEndian.Uint64(buf)
	return nil
}

func (n *nestedDelegatedInner8) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(n.Value)
	return nil
}

// MixedDelegatedContainer places delegated static fields around a dynamic
// field; each delegated field's region must be sized from its OWN type.
type MixedDelegatedContainer struct {
	A uint64
	B [3]nestedDelegatedInner
	C []uint16 `ssz-max:"8"`
	D nestedDelegatedInner8
	E []byte `ssz-max:"8"`
}

var MixedDelegatedContainer_Payload = MixedDelegatedContainer{
	A: 1,
	B: [3]nestedDelegatedInner{{Value: 2}, {Value: 3}, {Value: 4}},
	C: []uint16{5, 6, 7},
	D: nestedDelegatedInner8{Value: 8},
	E: []byte{9, 10},
}

// UnionDynVariant has a union with a variable-size variant followed by
// another dynamic field; a truncated union region must fail cleanly on the
// stream path (the selector read must not cross into the next region).
type UnionDynVariant struct {
	U dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 []byte `ssz-max:"16"`
	}]
	L []byte `ssz-max:"16"`
}

var UnionDynVariant_Payload = UnionDynVariant{
	U: dynssz.CompatibleUnion[struct {
		F1 uint32
		F2 []byte `ssz-max:"16"`
	}]{Variant: 2, Data: []byte{1, 2, 3}},
	L: []byte{1, 4, 5},
}

// ClassicUnionDynVariant has a classic union (None declared first) with a
// variable-size variant followed by another dynamic field: the None region
// must be exactly the bare selector byte, and a truncated union region must
// fail cleanly on the stream path.
type ClassicUnionDynVariant struct {
	U dynssz.Union[struct {
		N  dynssz.None
		F1 uint32
		F2 []byte `ssz-max:"16"`
	}]
	L []byte `ssz-max:"16"`
}

var ClassicUnionDynVariant_Payload = ClassicUnionDynVariant{
	U: dynssz.Union[struct {
		N  dynssz.None
		F1 uint32
		F2 []byte `ssz-max:"16"`
	}]{Variant: 2, Data: []byte{1, 2, 3}},
	L: []byte{1, 4, 5},
}

// ClassicUnionPtrVariant covers a classic union with a pointer variant — the
// generated decoder must allocate the pointer before writing through it.
type ClassicUnionPtrVariant struct {
	U dynssz.Union[struct {
		V1 uint64
		V2 *uint64
	}]
}

var ClassicUnionPtrVariant_Payload = func() ClassicUnionPtrVariant {
	p := ClassicUnionPtrVariant{}
	p.U.Variant = 1
	p.U.Data = &ptrUnionVal
	return p
}()

// PromotedDelegInner declares a full set of dynamic SSZ methods. A type that
// embeds it inherits those methods by promotion; the codegen parser must treat
// only declared methods as delegation, so PromotedDelegOuter is walked as a
// container (encoding Label) rather than delegating to the promoted methods
// (which would drop Label).
type PromotedDelegInner struct{ Seconds uint16 }

func (p *PromotedDelegInner) SizeSSZDyn(sszutils.DynamicSpecs) int { return 2 }

func (p *PromotedDelegInner) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, byte(p.Seconds), byte(p.Seconds>>8)), nil
}

func (p *PromotedDelegInner) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, b []byte) error {
	if len(b) >= 2 {
		p.Seconds = uint16(b[0]) | uint16(b[1])<<8
	}
	return nil
}

func (p *PromotedDelegInner) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint16(p.Seconds)
	return nil
}

type PromotedDelegOuter struct {
	PromotedDelegInner
	Label uint64
}

// GenericBoxFixture is a generic struct used only as a codegen test fixture
// (not listed in any gen_*.yaml, so never generated): an instantiation of it
// must be rejected as a top-level generation target.
type GenericBoxFixture[T any] struct {
	V T
}

// AliasedVecPair holds two short-paddable byte vectors; hashing must pad into
// library-owned buffers so the root stays independent of whether the two
// fields alias one backing array.
type AliasedVecPair struct {
	V []byte `ssz-size:"8"`
	W []byte `ssz-size:"8"`
}

// ShortLargeUintVec uses slice-typed large-uints so the Go value can be
// shorter than the declared width, exercising the zero-padding parity between
// the generated and reflection hash tree roots.
type ShortLargeUintVec struct {
	A []byte   `ssz-type:"uint128" ssz-size:"16"`
	B []uint64 `ssz-type:"uint256" ssz-size:"4"`
}

// UnionTaggedSelectors assigns explicit 1-based selector values via ssz-index
// tags on the union variant fields (EIP-8016 conformant numbering).
type UnionTaggedSelectors struct {
	U dynssz.CompatibleUnion[struct {
		F1 uint32 `ssz-index:"1"`
		F2 []byte `ssz-max:"16" ssz-index:"2"`
	}]
}

var UnionTaggedSelectors_Payload = UnionTaggedSelectors{
	U: dynssz.CompatibleUnion[struct {
		F1 uint32 `ssz-index:"1"`
		F2 []byte `ssz-max:"16" ssz-index:"2"`
	}]{Variant: 1, Data: uint32(7)},
}

// Top-level standalone named composite types generated as -types entries.
// Their generated methods receive the pointer receiver directly, so every
// generator path must dereference it correctly.

type TopLevelBitlist []byte

var _ = sszutils.Annotate[TopLevelBitlist](`ssz-type:"bitlist" ssz-max:"128"`)

type TopLevelProgBitlist []byte

var _ = sszutils.Annotate[TopLevelProgBitlist](`ssz-type:"progressive-bitlist"`)

type TopLevelString string

var _ = sszutils.Annotate[TopLevelString](`ssz-max:"64"`)

type TopLevelCtrList []SimpleTypes1_C1

var _ = sszutils.Annotate[TopLevelCtrList](`ssz-max:"16"`)

type TopLevelCtrVec []SimpleTypes1_C1

var _ = sszutils.Annotate[TopLevelCtrVec](`ssz-size:"4"`)

type TopLevelVarList []OptionalListTypes_Inner

var _ = sszutils.Annotate[TopLevelVarList](`ssz-max:"16"`)

type TopLevelListOfList [][]byte

var _ = sszutils.Annotate[TopLevelListOfList](`ssz-max:"8,32"`)

// TopLevelWrapVarList wraps a variable-element list in a TypeWrapper; the
// streaming size closure must not collide with the wrapper unwrap local.
type TopLevelWrapVarList struct {
	V dynssz.TypeWrapper[struct {
		X []OptionalListTypes_Inner `ssz-max:"8"`
	}, []OptionalListTypes_Inner] `ssz-type:"wrapper"`
}

// PtrUnionVariant covers a CompatibleUnion with a pointer variant, and a
// TypeWrapper whose Data is a pointer — the generated decoder must allocate
// the pointer before writing through it.
type PtrUnionVariant struct {
	U dynssz.CompatibleUnion[struct {
		V1 uint64
		V2 *uint64
	}]
	W dynssz.TypeWrapper[struct{ Data *uint64 }, *uint64] `ssz-type:"wrapper"`
}

var ptrUnionVal = uint64(0xdeadbeef)

var PtrUnionVariant_Payload = func() PtrUnionVariant {
	p := PtrUnionVariant{}
	p.U.Variant = 2
	p.U.Data = &ptrUnionVal
	p.W.Data = &ptrUnionVal
	return p
}()

// --- codegen compile-correctness shapes (pointer-receiver deref, localized
// value naming, pointer-element fast-path guard, zero-padding item typing) ---

// TopVecOfVar is a top-level named vector of variable-size elements; its
// pointer receiver must be parenthesized before indexing in SizeSSZ.
type TopVecOfVar [3][]uint16

var _ = sszutils.Annotate[TopVecOfVar](`ssz-size:"3" ssz-max:"?,8"`)

// UnionSamePkgVariant uses a same-package named type as a union variant.
type UnionSamePkgVariant struct {
	U dynssz.CompatibleUnion[struct {
		V1 uint64
		V2 SimpleTypes1_C1
	}]
}

// ClassicUnionSamePkgVariant uses a same-package named type as a classic
// union variant.
type ClassicUnionSamePkgVariant struct {
	U dynssz.Union[struct {
		V1 uint64
		V2 SimpleTypes1_C1
	}]
}

// PtrPrimitiveList is a list of pointer-to-primitive; the bulk uint64
// fast-path must not fire for pointer elements.
type PtrPrimitiveList struct {
	F []*uint64 `ssz-max:"3"`
}

// FixedVecPtrStr / FixedVecPtrList / FixedVecStr exercise the under-fill
// zero-padding item for pointer, string and list elements.
type FixedVecPtrStr struct {
	F []*string `ssz-size:"2" ssz-max:"?,8"`
}
type FixedVecPtrList struct {
	F []*[]uint16 `ssz-size:"2" ssz-max:"?,4"`
}
type FixedVecStr struct {
	F []string `ssz-size:"2" ssz-max:"?,8"`
}

// PtrDynCollectionField is a pointer to a dynamic collection; its SizeSSZ
// must not double-declare the localized value.
type PtrDynCollectionField struct {
	F *[][]byte `ssz-max:"3" ssz-type:"?,bitlist" ssz-bitmax:"?,10"`
}

// WrapUnionField wraps a CompatibleUnion; the streaming size closure must
// not collide with its parameter name.
type WrapUnionField struct {
	W dynssz.TypeWrapper[struct {
		Data dynssz.CompatibleUnion[struct {
			A uint32
			B []byte `ssz-max:"4"`
		}]
	}, dynssz.CompatibleUnion[struct {
		A uint32
		B []byte `ssz-max:"4"`
	}]] `ssz-type:"wrapper"`
}

// WrapClassicUnionField wraps a classic Union; the streaming size closure
// must not collide with its parameter name.
type WrapClassicUnionField struct {
	W dynssz.TypeWrapper[struct {
		Data dynssz.Union[struct {
			N dynssz.None
			A uint32
		}]
	}, dynssz.Union[struct {
		N dynssz.None
		A uint32
	}]] `ssz-type:"wrapper"`
}

// ExcludedFields exercises ssz-type:"-": excluded fields are omitted from the
// SSZ layout entirely (not encoded, decoded, sized or hashed) and may hold
// non-SSZ types.
type ExcludedFields struct {
	A     uint32
	Cache [32]byte `ssz-type:"-"`
	B     uint64
	Meta  map[string]int `ssz-type:"-"`
	L     []uint16       `ssz-max:"8"`
}

var ExcludedFields_Payload = ExcludedFields{
	A:     1,
	Cache: [32]byte{9, 9, 9},
	B:     2,
	Meta:  map[string]int{"x": 1},
	L:     []uint16{3, 4},
}

// PtrSvecOfList is a pointer to a fixed vector of variable-size elements; the
// streaming encoder's under-fill zero-padding item must be the element type,
// not new(element) (which would be a pointer to it).
type PtrSvecOfList struct {
	F *[][]uint16 `ssz-size:"2" ssz-max:"?,4"`
}

// WrapPtrList wraps a pointer type; the size closure's nil-pointer guard must
// localize into a fresh variable instead of shadowing the closure's own
// parameter.
type WrapPtrList struct {
	W dynssz.TypeWrapper[struct {
		Data *[]uint16 `ssz-max:"8"`
	}, *[]uint16] `ssz-type:"wrapper"`
}

// RecursiveNode is a self-referential container: recursion through a bounded
// list is a legal, finite SSZ shape. The generated code must call the type's
// own methods for the recursive reference so emission and encoding terminate.
type RecursiveNode struct {
	Val      uint64
	Children []*RecursiveNode `ssz-max:"4"`
}

var RecursiveNode_Payload = RecursiveNode{
	Val: 1,
	Children: []*RecursiveNode{
		{Val: 2},
		{Val: 3, Children: []*RecursiveNode{{Val: 4}}},
	},
}

// RecursiveTree and RecursiveTreeBranch form a cycle that closes through a
// container field (Branch.Leaf) rather than a list element, with a
// spec-dependent limit inside the cycle. Both members generate methods, so the
// recursive references delegate; the spec-dependence must reach every cycle
// member so neither engine falls back to preset-baking fastssz paths.
type RecursiveTree struct {
	Depth    uint64
	Branches []RecursiveTreeBranch `ssz-max:"4" dynssz-max:"RECURSIVE_BRANCH_LIMIT"`
}

type RecursiveTreeBranch struct {
	Leaf *RecursiveTree
}

var RecursiveTree_Specs = map[string]any{
	"RECURSIVE_BRANCH_LIMIT": uint64(8),
}

var RecursiveTree_Payload = RecursiveTree{
	Depth: 1,
	Branches: []RecursiveTreeBranch{
		{Leaf: nil},
		{Leaf: &RecursiveTree{
			Depth:    2,
			Branches: []RecursiveTreeBranch{{Leaf: nil}},
		}},
	},
}

// StreamVecDynSize is a fixed-size vector (slice with ssz-size + dynssz-size) of
// variable-size elements. With -with-streaming the decoder emits a first-offset
// check `startOffset != <limit>*4`; when the limit is a dynssz expression the
// limit renders as a typed int(expr), so the comparison against the uint32
// startOffset must be uint32-converted or the generated code fails to compile.
type StreamVecDynSize struct {
	V []StreamVecElem `ssz-size:"2" dynssz-size:"STREAMVEC_LEN"`
}

type StreamVecElem struct {
	X []byte `ssz-max:"8"`
}

var StreamVecDynSize_Specs = map[string]any{
	"STREAMVEC_LEN": uint64(2),
}

var StreamVecDynSize_Payload = StreamVecDynSize{
	V: []StreamVecElem{
		{X: []byte{1, 2, 3}},
		{X: []byte{4, 5}},
	},
}

// SizeUnionExprVariant has a compatible-union whose second variant is an inline
// container sized entirely by a size expression (a fixed-size string with a
// dynssz-size). The SizeSSZ generator asserts the variant value but the
// expression-only size never reads it, which must not leave the asserted
// variable declared-and-unused in the generated code.
type SizeUnionExprVariant struct {
	U dynssz.CompatibleUnion[struct {
		A uint32
		B struct {
			S string `ssz-size:"8" dynssz-size:"SUEV_LEN"`
		}
	}]
}

var SizeUnionExprVariant_Specs = map[string]any{
	"SUEV_LEN": uint64(8),
}

var SizeUnionExprVariant_Payload = SizeUnionExprVariant{
	U: dynssz.CompatibleUnion[struct {
		A uint32
		B struct {
			S string `ssz-size:"8" dynssz-size:"SUEV_LEN"`
		}
	}]{
		Variant: 2,
		Data: struct {
			S string `ssz-size:"8" dynssz-size:"SUEV_LEN"`
		}{S: "abcdefgh"},
	},
}

// DynSizeVectorNoStatic covers slices whose length is fixed purely by a
// dynssz-size expression, with no static ssz-size fallback. The reflection
// typecache resolves the expression to a concrete size and classifies the slice
// as a Vector; the codegen parser has no spec values at generation time and must
// reach the same classification from the presence of the expression alone.
// Previously the generator saw a static size of 0 and mis-encoded the field as a
// variable List (a 4-byte offset plus contents), diverging from reflection and
// the SSZ spec on size, serialization and hash tree root. AV additionally nests
// the dynssz-only inner vector under a fixed outer array whose dimension is the
// '?' placeholder.
type DynSizeVectorNoStatic struct {
	V  []uint16    `dynssz-size:"DSV_LEN"`
	AV [2][]uint16 `dynssz-size:"?,DSV_LEN"`
}

var DynSizeVectorNoStatic_Specs = map[string]any{
	"DSV_LEN": uint64(3),
}

func DynSizeVectorNoStaticPayload(n int) DynSizeVectorNoStatic {
	v := make([]uint16, n)
	for i := range v {
		v[i] = uint16(i + 1)
	}
	var av [2][]uint16
	for r := range av {
		row := make([]uint16, n)
		for i := range row {
			row[i] = uint16(r*100 + i + 1)
		}
		av[r] = row
	}
	return DynSizeVectorNoStatic{V: v, AV: av}
}

// ---------------------------------------------------------------------------
// Attack shapes for the without-dynamic-expressions static/inlining path.
// All generated with -with-streaming -without-fastssz -without-dynamic-expressions
// -with-extended-types (gen_atknest.yaml). Every parent must reach nested
// generated children through their static methods (never a *Dyn buffer call)
// and must round-trip byte/size/root-identical to reflection.
// ---------------------------------------------------------------------------

// AtkNestLeaf is a variable-size leaf container nested at several depths.
type AtkNestLeaf struct {
	A []byte `ssz-max:"8"`
	B uint8
}

// AtkNestD3/D2/D1 build a depth-4 chain of variable-size containers-of-containers
// mixing list, vector, and progressive-list nesting.
type AtkNestD3 struct {
	L AtkNestLeaf
	V [2]AtkNestLeaf
	X uint16
}
type AtkNestD2 struct {
	N AtkNestD3
	L []AtkNestD3 `ssz-max:"4"`
}
type AtkNestD1 struct {
	N AtkNestD2
	P []AtkNestD2 `ssz-type:"progressive-list" ssz-max:"8"`
	Z uint64
}

// AtkNestUnion nests generated containers as union variants.
type AtkNestUnion struct {
	U dynssz.CompatibleUnion[struct {
		F1 AtkNestLeaf
		F2 AtkNestD2
	}]
}

// AtkNestWrapper wraps a nested generated container in a TypeWrapper.
type AtkNestWrapper struct {
	W dynssz.TypeWrapper[struct {
		Data AtkNestD2 `ssz-size:"?"`
	}, AtkNestD2]
}

// AtkNestOpt / AtkNestOptList nest a generated container behind an optional and
// an optional-list pointer.
type AtkNestOpt struct {
	Opt *AtkNestD2 `ssz-type:"optional"`
	N   uint16
}
type AtkNestOptList struct {
	Opt *AtkNestD2 `ssz-type:"optional-list"`
}

var (
	atkNestLeafA = AtkNestLeaf{A: []byte{1, 2, 3}, B: 7}
	atkNestLeafB = AtkNestLeaf{A: nil, B: 0}
	atkNestD3v   = AtkNestD3{L: atkNestLeafA, V: [2]AtkNestLeaf{atkNestLeafA, atkNestLeafB}, X: 9}
	atkNestD2v   = AtkNestD2{N: atkNestD3v, L: []AtkNestD3{atkNestD3v, atkNestD3v}}

	AtkNestD1_Payload = AtkNestD1{
		N: atkNestD2v,
		P: []AtkNestD2{atkNestD2v, atkNestD2v},
		Z: 0xdeadbeef,
	}
	AtkNestUnion_Payload = AtkNestUnion{U: dynssz.CompatibleUnion[struct {
		F1 AtkNestLeaf
		F2 AtkNestD2
	}]{Variant: 1, Data: atkNestLeafA}}
	AtkNestWrapper_Payload = AtkNestWrapper{W: dynssz.TypeWrapper[struct {
		Data AtkNestD2 `ssz-size:"?"`
	}, AtkNestD2]{Data: atkNestD2v}}
	AtkNestOpt_Payload     = AtkNestOpt{Opt: &atkNestD2v, N: 4321}
	AtkNestOptList_Payload = AtkNestOptList{Opt: &atkNestD2v}
)

// atkWellDelegated is an external fully-delegated type whose Go struct layout
// matches its wire form exactly (a single uint64 field). Under
// without-dynamic-expressions the parser (NoDelegation) traverses it and the
// static generators inline its structure. The inlined static encoding must be
// byte/root-identical to what its own Dynamic* methods produce.
type atkWellDelegated struct {
	V uint64
}

var _ = sszutils.Annotate[atkWellDelegated](`ssz-static:"true"`)

func (n *atkWellDelegated) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 8 }
func (n *atkWellDelegated) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint64(buf, n.V), nil
}
func (n *atkWellDelegated) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.V = binary.LittleEndian.Uint64(buf)
	return nil
}
func (n *atkWellDelegated) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(n.V)
	return nil
}

// AtkWellHolder nests the well-behaved delegated type as a static field and a
// vector; generated statically it inlines the delegated type's structure.
type AtkWellHolder struct {
	A uint64
	N atkWellDelegated
	V [2]atkWellDelegated
}

var AtkWellHolder_Payload = AtkWellHolder{
	A: 0x1122334455667788,
	N: atkWellDelegated{V: 0x99},
	V: [2]atkWellDelegated{{V: 0xaa}, {V: 0xbb}},
}
