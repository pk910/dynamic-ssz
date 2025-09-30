// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"

	"github.com/pk910/dynamic-ssz/sszutils"
)

var sszMarshalerType = reflect.TypeOf((*sszutils.FastsszMarshaler)(nil)).Elem()
var sszUnmarshalerType = reflect.TypeOf((*sszutils.FastsszUnmarshaler)(nil)).Elem()
var sszHashRootType = reflect.TypeOf((*sszutils.FastsszHashRoot)(nil)).Elem()
var dynamicMarshalerType = reflect.TypeOf((*sszutils.DynamicMarshaler)(nil)).Elem()
var dynamicUnmarshalerType = reflect.TypeOf((*sszutils.DynamicUnmarshaler)(nil)).Elem()
var dynamicSizerType = reflect.TypeOf((*sszutils.DynamicSizer)(nil)).Elem()
var dynamicHashRootType = reflect.TypeOf((*sszutils.DynamicHashRoot)(nil)).Elem()

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

// getHashTreeRootWithCompatibility evaluates the compatibility of a given type with fastssz's HashTreeRootWith method,
// determining whether the type can efficiently compute its hash tree root using fastssz's optimized hasher interface.
// This method uses reflection to detect the HashTreeRootWith method since actual implementations may use specific
// parameter types (ssz.HashWalker, *ssz.Hasher) rather than interface{}, ensuring compatibility across different
// fastssz implementations without requiring direct imports.
//
// Parameters:
//   - targetType: The reflect.Type of the value being assessed for fastssz HashTreeRootWith compatibility. This type
//     is evaluated to ensure it has a method with the signature pattern HashTreeRootWith(hasher) error, regardless
//     of the specific hasher parameter type used.
//
// Returns:
//   - The method if found and valid, nil otherwise. This allows caching for performance optimization.
func (d *DynSsz) getHashTreeRootWithCompatibility(targetType reflect.Type) *reflect.Method {
	targetPtrType := reflect.New(targetType).Type()

	// Check if the type has a method named "HashTreeRootWith"
	method, found := targetPtrType.MethodByName("HashTreeRootWith")
	if !found {
		return nil
	}

	// Check the method signature:
	// - Should have exactly 2 parameters (receiver + hasher parameter)
	// - Should return exactly 1 value (error)
	methodType := method.Type
	if methodType.NumIn() != 2 || methodType.NumOut() != 1 {
		return nil
	}

	// Check that it returns an error
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if !methodType.Out(0).AssignableTo(errorType) {
		return nil
	}

	// The method exists with the right signature pattern
	// We don't check the exact parameter type since it could be
	// ssz.HashWalker, *ssz.Hasher, or interface{}
	return &method
}

// getDynamicMarshalerCompatibility checks if a type implements the DynamicMarshaler interface
func (d *DynSsz) getDynamicMarshalerCompatibility(targetType reflect.Type) bool {
	targetPtrType := reflect.New(targetType).Type()
	return targetPtrType.Implements(dynamicMarshalerType)
}

// getDynamicUnmarshalerCompatibility checks if a type implements the DynamicUnmarshaler interface
func (d *DynSsz) getDynamicUnmarshalerCompatibility(targetType reflect.Type) bool {
	targetPtrType := reflect.New(targetType).Type()
	return targetPtrType.Implements(dynamicUnmarshalerType)
}

// getDynamicSizerCompatibility checks if a type implements the DynamicSizer interface
func (d *DynSsz) getDynamicSizerCompatibility(targetType reflect.Type) bool {
	targetPtrType := reflect.New(targetType).Type()
	return targetPtrType.Implements(dynamicSizerType)
}

// getDynamicHashRootCompatibility checks if a type implements the DynamicHashRoot interface
func (d *DynSsz) getDynamicHashRootCompatibility(targetType reflect.Type) bool {
	targetPtrType := reflect.New(targetType).Type()
	return targetPtrType.Implements(dynamicHashRootType)
}
