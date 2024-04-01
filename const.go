// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"reflect"

	fastssz "github.com/ferranbt/fastssz"
)

var byteType = reflect.TypeOf(byte(0))
var sszMarshalerType = reflect.TypeOf((*fastssz.Marshaler)(nil)).Elem()
var sszUnmarshalerType = reflect.TypeOf((*fastssz.Unmarshaler)(nil)).Elem()
var sszHashRootType = reflect.TypeOf((*fastssz.HashRoot)(nil)).Elem()
