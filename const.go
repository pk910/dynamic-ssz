// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"
)

var byteType = reflect.TypeOf(byte(0))
var typeWrapperType = reflect.TypeOf((*TypeWrapper[struct{}, interface{}])(nil)).Elem()
