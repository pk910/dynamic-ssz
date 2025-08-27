package sszutils

// FastsszMarshaler is the interface implemented by types that can marshal themselves into valid SZZ using fastssz.
type FastsszMarshaler interface {
	MarshalSSZTo(dst []byte) ([]byte, error)
	MarshalSSZ() ([]byte, error)
	SizeSSZ() int
}

// FastsszUnmarshaler is the interface implemented by types that can unmarshal a SSZ description of themselves
type FastsszUnmarshaler interface {
	UnmarshalSSZ(buf []byte) error
}

type FastsszHashRoot interface {
	HashTreeRoot() ([32]byte, error)
}

// DynamicMarshaler is the interface implemented by types that can marshal themselves using dynamic SSZ
type DynamicMarshaler interface {
	MarshalSSZDyn(ds interface{}, buf []byte) ([]byte, error)
}

// DynamicUnmarshaler is the interface implemented by types that can unmarshal using dynamic SSZ
type DynamicUnmarshaler interface {
	UnmarshalSSZDyn(ds interface{}, buf []byte) error
}

// DynamicSizer is the interface implemented by types that can calculate their own SSZ size dynamically
type DynamicSizer interface {
	SizeSSZDyn(ds interface{}) int
}
