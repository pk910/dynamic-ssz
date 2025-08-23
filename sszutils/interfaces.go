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
