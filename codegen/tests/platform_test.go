// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package tests

import (
	"errors"
	"math"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// The generated methods are reached through interfaces, so this file compiles
// in a checkout without the generated fixtures; each case skips when the method
// it exercises is absent.
type (
	marshalSSZ   interface{ MarshalSSZ() ([]byte, error) }
	marshalSSZTo interface {
		MarshalSSZTo(buf []byte) ([]byte, error)
	}
	marshalSSZEncoder interface {
		MarshalSSZEncoder(ds sszutils.DynamicSpecs, enc sszutils.Encoder) error
	}
	unmarshalSSZ interface{ UnmarshalSSZ(buf []byte) error }
	hashTreeRoot interface{ HashTreeRoot() ([32]byte, error) }
	sizeSSZ      interface{ SizeSSZ() int }
)

// A declared size the target's int cannot hold is refused the same way by every
// generated path: with an error, not by exhausting the address space.
func TestGeneratedPathsRefuseSizesPastThePlatformRange(t *testing.T) {
	t.Parallel()

	if math.MaxInt > math.MaxInt32 {
		// The declaration fits here, so there is nothing to refuse; the check
		// under test only exists for a narrower int.
		t.Skip("requires a 32-bit platform")
	}

	value := any(&WideAggregate{})

	for _, tc := range []struct {
		name string
		call func(t *testing.T) error
	}{
		{"MarshalSSZ", func(t *testing.T) error {
			return withGenerated(t, value, func(v marshalSSZ) error { _, err := v.MarshalSSZ(); return err })
		}},
		{"MarshalSSZTo", func(t *testing.T) error {
			return withGenerated(t, value, func(v marshalSSZTo) error { _, err := v.MarshalSSZTo(nil); return err })
		}},
		{"MarshalSSZEncoder", func(t *testing.T) error {
			return withGenerated(t, value, func(v marshalSSZEncoder) error {
				return v.MarshalSSZEncoder(dynssz.GetGlobalDynSsz(), sszutils.NewBufferEncoder(nil))
			})
		}},
		{"UnmarshalSSZ", func(t *testing.T) error {
			return withGenerated(t, value, func(v unmarshalSSZ) error { return v.UnmarshalSSZ(nil) })
		}},
		{"HashTreeRoot", func(t *testing.T) error {
			return withGenerated(t, value, func(v hashTreeRoot) error { _, err := v.HashTreeRoot(); return err })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked instead of reporting the size: %v", r)
				}
			}()

			if err := tc.call(t); !errors.Is(err, sszutils.ErrPlatformOverflow) {
				t.Errorf("err = %v, want the platform range reported", err)
			}
		})
	}
}

// withGenerated runs call against the generated method the type provides, or
// skips where the fixtures were not generated.
func withGenerated[T any](t *testing.T, value any, call func(T) error) error {
	t.Helper()

	method, ok := value.(T)
	if !ok {
		t.Skip("generated methods are not present in this checkout")
	}

	return call(method)
}

// A size method answers for the value it is given: an empty list forms no
// product, so a width the target cannot hold does not make its size wrong.
func TestGeneratedSizeOfAnEmptyWideList(t *testing.T) {
	t.Parallel()

	value := any(&WideListHolder{})

	marshaller, ok := value.(marshalSSZ)
	sizer, sized := value.(sizeSSZ)
	if !ok || !sized {
		t.Skip("generated methods are not present in this checkout")
	}

	data, err := marshaller.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if size := sizer.SizeSSZ(); size != len(data) {
		t.Errorf("SizeSSZ = %d, marshalled %d bytes", size, len(data))
	}
}
