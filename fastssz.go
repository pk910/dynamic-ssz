package dynssz

import (
	"reflect"
)

// fastsszMarshaler is the interface implemented by types that can marshal themselves into valid SZZ using fastssz.
type fastsszMarshaler interface {
	MarshalSSZTo(dst []byte) ([]byte, error)
	MarshalSSZ() ([]byte, error)
	SizeSSZ() int
}

// fastsszUnmarshaler is the interface implemented by types that can unmarshal a SSZ description of themselves
type fastsszUnmarshaler interface {
	UnmarshalSSZ(buf []byte) error
}

type fastsszHashRoot interface {
	HashTreeRoot() ([32]byte, error)
}

var sszMarshalerType = reflect.TypeOf((*fastsszMarshaler)(nil)).Elem()
var sszUnmarshalerType = reflect.TypeOf((*fastsszUnmarshaler)(nil)).Elem()
var sszHashRootType = reflect.TypeOf((*fastsszHashRoot)(nil)).Elem()

// getFastsszCompatibility evaluates the compatibility of a given type with fastssz, determining whether the type and its nested
// structures can be efficiently encoded/decoded using fastssz's static code generation approach.
//
// Parameters:
// - targetType: The reflect.Type of the value being assessed for fastssz compatibility. This type, along with its nested
//   or referenced types, is evaluated to ensure it aligns with fastssz's requirements for static encoding and decoding.
//
// Returns:
// - A boolean indicating whether the type is compatible with fastssz's static encoding and decoding.

func (d *DynSsz) getFastsszConvertCompatibility(targetType reflect.Type) bool {
	targetPtrType := reflect.New(targetType).Type()
	return targetPtrType.Implements(sszMarshalerType) && targetPtrType.Implements(sszUnmarshalerType)
}

// getFastsszHashCompatibility evaluates the compatibility of a given type with fastssz's HashRoot interface, determining whether
// the type can efficiently compute its hash tree root using fastssz's static code generation approach.
//
// Parameters:
// - targetType: The reflect.Type of the value being assessed for fastssz hash compatibility. This type, along with its nested
//   or referenced types, is evaluated to ensure it aligns with fastssz's requirements for static hash tree root computation.
//
// Returns:
// - A boolean indicating whether the type is compatible with fastssz's static hash tree root computation.

func (d *DynSsz) getFastsszHashCompatibility(targetType reflect.Type) bool {
	targetPtrType := reflect.New(targetType).Type()
	return targetPtrType.Implements(sszHashRootType)
}
