// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

// FinishRegion closes the region most recently pushed with PushOpenLimit,
// asserting that it was fully consumed.
//
// For a bounded region this is equivalent to checking PopLimit() == 0. For an
// open region the end is only discoverable by probing the reader, so the check
// is an explicit "no more data" test. The test matters because a variable-size
// type can still stop short of the end — a union selecting a fixed-size
// variant, an absent optional, or an optional-list with a fixed element all
// consume less than the region they were given.
func FinishRegion(dec Decoder) error {
	more, err := dec.More()
	if err != nil {
		dec.PopLimit()
		return err
	}
	diff := dec.PopLimit()
	if !more {
		// No data left in the region, so nothing was left unconsumed: for a
		// bounded region More() and PopLimit() read the same limit, and for an
		// open region PopLimit() reports 0 by definition.
		return nil
	}
	if diff <= 0 {
		// Open region: the leftover count is unknown, but its presence is
		// established.
		diff = 1
	}
	return ErrTrailingDataFn(diff)
}

// FinishStream asserts that a decoder has consumed its entire input. It is the
// top-level counterpart to FinishRegion and is what ultimately catches
// under-consumption in unknown-length mode: an open region is always the suffix
// of the stream, so any bytes an inner region left behind are still pending
// here.
func FinishStream(dec Decoder) error {
	more, err := dec.More()
	if err != nil {
		return err
	}
	if more {
		remaining := dec.GetLength()
		if !dec.LengthKnown() || remaining <= 0 {
			remaining = 1
		}
		return ErrTrailingDataFn(remaining)
	}
	return nil
}
