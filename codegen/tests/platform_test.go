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

// A declared size the target's int cannot hold is refused the same way by every
// generated path: with an error, not by exhausting the address space.
func TestGeneratedPathsRefuseSizesPastThePlatformRange(t *testing.T) {
	t.Parallel()

	if math.MaxInt > math.MaxInt32 {
		// The declaration fits here, so there is nothing to refuse; the check
		// under test only exists for a narrower int.
		t.Skip("requires a 32-bit platform")
	}

	var value WideAggregate

	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"MarshalSSZ", func() error { _, err := value.MarshalSSZ(); return err }},
		{"MarshalSSZTo", func() error { _, err := value.MarshalSSZTo(nil); return err }},
		{"MarshalSSZEncoder", func() error {
			return value.MarshalSSZEncoder(dynssz.GetGlobalDynSsz(), sszutils.NewBufferEncoder(nil))
		}},
		{"UnmarshalSSZ", func() error { return value.UnmarshalSSZ(nil) }},
		{"HashTreeRoot", func() error { _, err := value.HashTreeRoot(); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked instead of reporting the size: %v", r)
				}
			}()

			if err := tc.call(); !errors.Is(err, sszutils.ErrPlatformOverflow) {
				t.Errorf("err = %v, want the platform range reported", err)
			}
		})
	}
}

// A size method answers for the value it is given: an empty list forms no
// product, so a width the target cannot hold does not make its size wrong.
func TestGeneratedSizeOfAnEmptyWideList(t *testing.T) {
	t.Parallel()

	var value WideListHolder

	data, err := value.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if size := value.SizeSSZ(); size != len(data) {
		t.Errorf("SizeSSZ = %d, marshalled %d bytes", size, len(data))
	}
}
