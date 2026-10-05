package sszspecs

import dynssz "github.com/pk910/dynamic-ssz"

type bytes4 [4]byte
type bytes32 [32]byte
type bytes52 [52]byte
type bytes64 [64]byte

type byteList512KiBDesc struct {
	Data []byte `ssz-max:"524288"`
}
type byteList512KiB = dynssz.TypeWrapper[byteList512KiBDesc, []byte]

type bitlist8Desc struct {
	Data []byte `ssz-type:"bitlist" ssz-max:"8"`
}
type bitlist16Desc struct {
	Data []byte `ssz-type:"bitlist" ssz-max:"16"`
}
type bitlist256Desc struct {
	Data []byte `ssz-type:"bitlist" ssz-max:"256"`
}
type bitlist8 = dynssz.TypeWrapper[bitlist8Desc, []byte]
type bitlist16 = dynssz.TypeWrapper[bitlist16Desc, []byte]
type bitlist256 = dynssz.TypeWrapper[bitlist256Desc, []byte]

type bitvector1Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"1" ssz-bitsize:"1"`
}
type bitvector7Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"1" ssz-bitsize:"7"`
}
type bitvector8Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"1" ssz-bitsize:"8"`
}
type bitvector9Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"2" ssz-bitsize:"9"`
}
type bitvector64Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"8" ssz-bitsize:"64"`
}
type bitvector255Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"32" ssz-bitsize:"255"`
}
type bitvector256Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"32" ssz-bitsize:"256"`
}
type bitvector257Desc struct {
	Data []byte `ssz-type:"bitvector" ssz-size:"33" ssz-bitsize:"257"`
}
type bitvector1 = dynssz.TypeWrapper[bitvector1Desc, []byte]
type bitvector7 = dynssz.TypeWrapper[bitvector7Desc, []byte]
type bitvector8 = dynssz.TypeWrapper[bitvector8Desc, []byte]
type bitvector9 = dynssz.TypeWrapper[bitvector9Desc, []byte]
type bitvector64 = dynssz.TypeWrapper[bitvector64Desc, []byte]
type bitvector255 = dynssz.TypeWrapper[bitvector255Desc, []byte]
type bitvector256 = dynssz.TypeWrapper[bitvector256Desc, []byte]
type bitvector257 = dynssz.TypeWrapper[bitvector257Desc, []byte]

type uint32List16Desc struct {
	Data []uint32 `ssz-max:"16"`
}
type bytes32List8Desc struct {
	Data [][32]byte `ssz-max:"8"`
}
type uint64List32Desc struct {
	Data []uint64 `ssz-max:"32"`
}
type uint32List16 = dynssz.TypeWrapper[uint32List16Desc, []uint32]
type bytes32List8 = dynssz.TypeWrapper[bytes32List8Desc, [][32]byte]
type uint64List32 = dynssz.TypeWrapper[uint64List32Desc, []uint64]

type progressiveUint64Desc struct {
	Data []uint64 `ssz-type:"progressive-list"`
}
type progressiveBytes32Desc struct {
	Data [][32]byte `ssz-type:"progressive-list"`
}
type progressiveUint16Desc struct {
	Data []uint16 `ssz-type:"progressive-list"`
}
type nestedProgressiveUint16Desc struct {
	Data [][]uint16 `ssz-type:"progressive-list,progressive-list"`
}
type progressiveBitlistDesc struct {
	Data []byte `ssz-type:"progressive-bitlist"`
}
type progressiveUint64 = dynssz.TypeWrapper[progressiveUint64Desc, []uint64]
type progressiveBytes32 = dynssz.TypeWrapper[progressiveBytes32Desc, [][32]byte]
type progressiveUint16 = dynssz.TypeWrapper[progressiveUint16Desc, []uint16]
type nestedProgressiveUint16 = dynssz.TypeWrapper[nestedProgressiveUint16Desc, [][]uint16]
type progressiveBitlist = dynssz.TypeWrapper[progressiveBitlistDesc, []byte]

type sampleContainerWithProgressiveList struct {
	A uint16
	B []uint64 `ssz-type:"progressive-list"`
	C uint8
}

type sampleSquare struct {
	Side  uint16 `ssz-index:"0"`
	Color uint8  `ssz-index:"2"`
}
type sampleCircle struct {
	Radius uint16 `ssz-index:"1"`
	Color  uint8  `ssz-index:"2"`
}
type sampleOneField struct {
	A uint16 `ssz-index:"0"`
}
type sampleLeadingGaps struct {
	C uint32 `ssz-index:"2"`
}
type sampleMultipleGaps struct {
	A uint8  `ssz-index:"0"`
	B uint16 `ssz-index:"3"`
	C uint32 `ssz-index:"5"`
}
type sampleWidestLayout struct {
	Tail uint8 `ssz-index:"255"`
}
type sampleLevelBoundary struct {
	First uint16 `ssz-index:"0"`
	Last  uint8  `ssz-index:"21"`
}
type sampleBoundedListField struct {
	Head uint64   `ssz-index:"0"`
	Body []uint16 `ssz-index:"2" ssz-max:"4"`
}
type sampleProgressiveFields struct {
	Head    uint64   `ssz-index:"0"`
	Numbers []uint64 `ssz-index:"1" ssz-type:"progressive-list"`
	Flags   []byte   `ssz-index:"2" ssz-type:"progressive-bitlist"`
}
type sampleInnerShape struct {
	X uint16 `ssz-index:"0"`
	Y uint8  `ssz-index:"2"`
}
type sampleOuterShape struct {
	Head  uint8            `ssz-index:"0"`
	Inner sampleInnerShape `ssz-index:"2"`
}

type squareProgressiveListDesc struct {
	Data []sampleSquare `ssz-type:"progressive-list"`
}
type circleProgressiveListDesc struct {
	Data []sampleCircle `ssz-type:"progressive-list"`
}
type squareProgressiveList = dynssz.TypeWrapper[squareProgressiveListDesc, []sampleSquare]
type circleProgressiveList = dynssz.TypeWrapper[circleProgressiveListDesc, []sampleCircle]

type progressiveShapeContainer struct {
	Tag   uint8
	Shape sampleSquare
}

type uint16List4Desc struct {
	Data []uint16 `ssz-max:"4"`
}
type uint16List4AliasDesc struct {
	Data []uint16 `ssz-max:"4"`
}
type uint16List4 = dynssz.TypeWrapper[uint16List4Desc, []uint16]
type uint16List4Alias = dynssz.TypeWrapper[uint16List4AliasDesc, []uint16]

type sampleShapeDesc struct {
	Square    sampleSquare `ssz-index:"1"`
	Circle    sampleCircle `ssz-index:"2"`
	Square127 sampleSquare `ssz-index:"127"`
}
type sampleShape = dynssz.CompatibleUnion[sampleShapeDesc]

type sampleNumbersDesc struct {
	Numbers1 uint16List4      `ssz-index:"1"`
	Numbers2 uint16List4Alias `ssz-index:"2"`
}
type sampleNumbers = dynssz.CompatibleUnion[sampleNumbersDesc]

type sampleEmptyProneDesc struct {
	Squares squareProgressiveList `ssz-index:"1"`
	Circles circleProgressiveList `ssz-index:"2"`
}
type sampleEmptyProne = dynssz.CompatibleUnion[sampleEmptyProneDesc]

type sampleSquareOnlyDesc struct {
	Square sampleSquare `ssz-index:"5"`
}
type sampleSquareOnly = dynssz.CompatibleUnion[sampleSquareOnlyDesc]

type sampleNestedShapeDesc struct {
	Shape      sampleShape      `ssz-index:"1"`
	SquareOnly sampleSquareOnly `ssz-index:"2"`
}
type sampleNestedShape = dynssz.CompatibleUnion[sampleNestedShapeDesc]

type unionShapeContainer struct {
	Tag  uint64
	Body sampleShape
}
type sampleShapeProgressiveContainer struct {
	Tag  uint64      `ssz-index:"0"`
	Body sampleShape `ssz-index:"2"`
}
type shapeProgressiveListDesc struct {
	Data []sampleShape `ssz-type:"progressive-list"`
}
type shapeProgressiveList = dynssz.TypeWrapper[shapeProgressiveListDesc, []sampleShape]
