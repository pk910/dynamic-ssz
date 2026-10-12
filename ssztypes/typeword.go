// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"
	"unsafe"
)

// TypeWord returns the identity of a type: the type pointer a reflect.Type
// holds in its data word. Two reflect.Types are the same type exactly when
// they hold the same pointer, so the word serves as a map key for a type. It
// boxes into an interface without allocating and hashes as one pointer, which
// is cheaper than hashing the two words of a reflect.Type.
func TypeWord(t reflect.Type) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1]
}
