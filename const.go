// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"reflect"
)

var byteType = reflect.TypeOf(byte(0))
var typeWrapperType = reflect.TypeOf((*TypeWrapper[struct{}, interface{}])(nil)).Elem()
