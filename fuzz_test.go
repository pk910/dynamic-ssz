// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"testing"
)

// FuzzUnmarshalSSZ exercises the SSZ unmarshaling path with arbitrary input.
// This Go-native fuzz test is integrated with the Go fuzzing engine and makes
// the project's fuzzing efforts visible to OSS-Fuzz / OpenSSF Scorecard.
func FuzzUnmarshalSSZ(f *testing.F) {
	type SimpleContainer struct {
		A uint64
		B uint32
		C uint16
		D uint8
	}

	type BytesContainer struct {
		Data []byte `ssz-max:"256"`
	}

	type MixedContainer struct {
		Slot      uint64
		Data      []byte `ssz-max:"128"`
		Validator uint64
	}

	// Seed corpus with valid SSZ encodings.
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // SimpleContainer sized
	f.Add([]byte{4, 0, 0, 0})                                  // BytesContainer with empty data
	f.Add([]byte{})                                            // empty input
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})                      // high offset value
	f.Add([]byte{1, 0, 0, 0, 0, 0, 0, 0, 20, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0xAB, 0xCD})

	ds := NewDynSsz(nil)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Fuzz SimpleContainer unmarshal.
		var sc SimpleContainer
		_ = ds.UnmarshalSSZ(&sc, data)

		// Fuzz BytesContainer unmarshal.
		var bc BytesContainer
		_ = ds.UnmarshalSSZ(&bc, data)

		// Fuzz MixedContainer unmarshal.
		var mc MixedContainer
		_ = ds.UnmarshalSSZ(&mc, data)

		// Round-trip: if unmarshal succeeds, marshal and unmarshal again.
		var sc2 SimpleContainer
		if err := ds.UnmarshalSSZ(&sc2, data); err == nil {
			encoded, err := ds.MarshalSSZ(&sc2)
			if err == nil {
				var sc3 SimpleContainer
				_ = ds.UnmarshalSSZ(&sc3, encoded)
			}
		}
	})
}

// FuzzHashTreeRoot exercises the hash tree root computation with arbitrary input.
func FuzzHashTreeRoot(f *testing.F) {
	type HTRContainer struct {
		Slot  uint64
		Index uint32
		Data  []byte `ssz-max:"64"`
	}

	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16, 0, 0, 0, 0xAA})
	f.Add([]byte{})

	ds := NewDynSsz(nil)

	f.Fuzz(func(t *testing.T, data []byte) {
		var c HTRContainer
		if err := ds.UnmarshalSSZ(&c, data); err != nil {
			return
		}

		// If unmarshal succeeds, HTR should not panic.
		_, _ = ds.HashTreeRoot(&c)
	})
}
