// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file implements cached type descriptors with unsafe pointer optimization.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"reflect"
	"unsafe"
)

// UnsafeAccessor provides unsafe pointer access to struct fields and slice elements
type UnsafeAccessor struct {
	BasePtr unsafe.Pointer
	Desc    *TypeDescriptor
}

// NewUnsafeAccessor creates a new unsafe accessor for the given pointer and type descriptor
func NewUnsafeAccessor(ptr unsafe.Pointer, desc *TypeDescriptor) *UnsafeAccessor {
	return &UnsafeAccessor{
		BasePtr: ptr,
		Desc:    desc,
	}
}

// GetFieldAccessor returns an unsafe accessor for a struct field
func (ua *UnsafeAccessor) GetFieldAccessor(fieldIndex int) *UnsafeAccessor {
	if ua.Desc.Kind != reflect.Struct || fieldIndex >= len(ua.Desc.Fields) {
		return nil
	}

	field := ua.Desc.Fields[fieldIndex]
	fieldPtr := unsafe.Pointer(uintptr(ua.BasePtr) + field.Offset)

	if field.IsPtr {
		// Dereference pointer
		if *(*unsafe.Pointer)(fieldPtr) == nil {
			return nil
		}
		fieldPtr = *(*unsafe.Pointer)(fieldPtr)
	}

	return &UnsafeAccessor{
		BasePtr: fieldPtr,
		Desc:    field.Type,
	}
}

// GetSliceElement returns an unsafe accessor for a slice element
func (ua *UnsafeAccessor) GetSliceElement(index int) *UnsafeAccessor {
	if ua.Desc.Kind != reflect.Slice {
		return nil
	}

	// Cast BasePtr to slice of bytes to get length and data pointer
	slice := *(*[]byte)(ua.BasePtr)
	if index >= len(slice) {
		return nil
	}

	elemSize := ua.Desc.ElemDesc.Size
	if elemSize <= 0 {
		return nil // Can't use unsafe for dynamic-sized elements
	}

	elemPtr := unsafe.Pointer(uintptr(unsafe.Pointer(unsafe.SliceData(slice))) + uintptr(int32(index)*elemSize))
	return &UnsafeAccessor{
		BasePtr: elemPtr,
		Desc:    ua.Desc.ElemDesc,
	}
}

// GetArrayElement returns an unsafe accessor for an array element
func (ua *UnsafeAccessor) GetArrayElement(index int, arrayLen int) *UnsafeAccessor {
	if ua.Desc.Kind != reflect.Array {
		return nil
	}

	if index >= arrayLen {
		return nil
	}

	elemSize := ua.Desc.ElemDesc.Size
	if elemSize <= 0 {
		return nil // Can't use unsafe for dynamic-sized elements
	}

	elemPtr := unsafe.Pointer(uintptr(ua.BasePtr) + uintptr(index*int(elemSize)))
	return &UnsafeAccessor{
		BasePtr: elemPtr,
		Desc:    ua.Desc.ElemDesc,
	}
}

// ReadPrimitive reads a primitive value using unsafe pointer access
func (ua *UnsafeAccessor) ReadPrimitive() interface{} {
	switch ua.Desc.Kind {
	case reflect.Bool:
		return *(*bool)(ua.BasePtr)
	case reflect.Uint8:
		return *(*uint8)(ua.BasePtr)
	case reflect.Uint16:
		return *(*uint16)(ua.BasePtr)
	case reflect.Uint32:
		return *(*uint32)(ua.BasePtr)
	case reflect.Uint64:
		return *(*uint64)(ua.BasePtr)
	default:
		return nil
	}
}

// WritePrimitive writes a primitive value using unsafe pointer access
func (ua *UnsafeAccessor) WritePrimitive(value interface{}) {
	switch ua.Desc.Kind {
	case reflect.Bool:
		*(*bool)(ua.BasePtr) = value.(bool)
	case reflect.Uint8:
		*(*uint8)(ua.BasePtr) = value.(uint8)
	case reflect.Uint16:
		*(*uint16)(ua.BasePtr) = value.(uint16)
	case reflect.Uint32:
		*(*uint32)(ua.BasePtr) = value.(uint32)
	case reflect.Uint64:
		*(*uint64)(ua.BasePtr) = value.(uint64)
	}
}

// GetSliceLen returns the length of a slice using unsafe access
func (ua *UnsafeAccessor) GetSliceLen() int {
	if ua.Desc.Kind != reflect.Slice {
		return 0
	}
	return len(*(*[]byte)(ua.BasePtr))
}

// GetSliceBytes returns the underlying byte slice using unsafe access
func (ua *UnsafeAccessor) GetSliceBytes() []byte {
	if ua.Desc.Kind != reflect.Slice {
		return nil
	}
	slice := *(*[]byte)(ua.BasePtr)
	return unsafe.Slice(unsafe.SliceData(slice), len(slice))
}
