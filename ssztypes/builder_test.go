// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

func TestWrapperTypeCompatible(t *testing.T) {
	type myByte byte
	type myUint64 uint64
	type myString string
	type structA struct{ A uint64 }
	type structB struct{ B uint32 }

	cases := []struct {
		name             string
		actual, expected reflect.Type
		want             bool
	}{
		{"identical", reflect.TypeOf([]byte(nil)), reflect.TypeOf([]byte(nil)), true},
		{"namedScalar", reflect.TypeOf(myUint64(0)), reflect.TypeOf(uint64(0)), true},
		{"namedString", reflect.TypeOf(myString("")), reflect.TypeOf(""), true},
		{"scalarKindMismatch", reflect.TypeOf(int32(0)), reflect.TypeOf(uint64(0)), false},
		{"sliceVsString", reflect.TypeOf(""), reflect.TypeOf([]byte(nil)), false},
		{"sliceNamedElem", reflect.TypeOf([]myByte(nil)), reflect.TypeOf([]byte(nil)), true},
		{"sliceElemMismatch", reflect.TypeOf([]uint32(nil)), reflect.TypeOf([]byte(nil)), false},
		{"ptrVsValue", reflect.TypeOf((*uint64)(nil)), reflect.TypeOf(uint64(0)), false},
		{"ptrNamedElem", reflect.TypeOf((*myUint64)(nil)), reflect.TypeOf((*uint64)(nil)), true},
		{"arraySameLen", reflect.TypeOf([8]myByte{}), reflect.TypeOf([8]byte{}), true},
		{"arrayLenMismatch", reflect.TypeOf([16]byte{}), reflect.TypeOf([8]byte{}), false},
		{"arrayElemMismatch", reflect.TypeOf([8]uint16{}), reflect.TypeOf([8]byte{}), false},
		// Composite kinds with matching Kind but different layout must not match.
		{"structMismatch", reflect.TypeOf(structA{}), reflect.TypeOf(structB{}), false},
		{"sameStruct", reflect.TypeOf(structA{}), reflect.TypeOf(structA{}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wrapperTypeCompatible(tc.actual, tc.expected); got != tc.want {
				t.Errorf("wrapperTypeCompatible(%v, %v) = %v, want %v", tc.actual, tc.expected, got, tc.want)
			}
		})
	}
}

// The union carrier is what the size, marshal and hash paths read the selector
// and the data from, so a shape those paths cannot read is refused here.
func TestValidateUnionCarrier(t *testing.T) {
	type carrier struct {
		Selector uint8
		Data     any
	}
	type wideSelector struct {
		Selector uint64
		Data     any
	}
	type signedSelector struct {
		Selector int8
		Data     any
	}
	type selectorOnly struct {
		Selector uint8
	}

	for _, tt := range []struct {
		name     string
		typ      reflect.Type
		accepted bool
	}{
		{"a selector and a data field", reflect.TypeOf(carrier{}), true},
		{"through a pointer", reflect.TypeOf(&carrier{}), true},
		{"not a struct", reflect.TypeOf(uint32(0)), false},
		{"no data field", reflect.TypeOf(selectorOnly{}), false},
		{"a signed selector", reflect.TypeOf(signedSelector{}), false},
		// A wider selector carries bits the walks drop when they narrow
		// field 0 to a uint8, so the carrier never holds one.
		{"a wider selector", reflect.TypeOf(wideSelector{}), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUnionCarrier(tt.typ, "union")
			if tt.accepted {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, sszutils.ErrInvalidConstraint) {
				t.Fatalf("err = %v, want the carrier refused", err)
			}
		})
	}
}
