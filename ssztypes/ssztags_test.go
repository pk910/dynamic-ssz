// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package ssztypes

import (
	"errors"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

func TestParseHashTag(t *testing.T) {
	for tag, want := range map[string]SszHashType{
		``:                                       SszHashUnspecified,
		`ssz-max:"4"`:                            SszHashUnspecified,
		`ssz-hash:"sha256"`:                      SszHashSha256,
		`ssz-static:"true" ssz-hash:"keccak256"`: SszHashKeccak256,
	} {
		got, err := ParseHashTag(tag)
		if err != nil || got != want {
			t.Errorf("ParseHashTag(%q) = %v, %v; want %v", tag, got, err, want)
		}
	}
	for _, tag := range []string{`ssz-hash:"keccak"`, `ssz-hash:""`, `ssz-hash:"SHA256"`} {
		if _, err := ParseHashTag(tag); !errors.Is(err, sszutils.ErrInvalidTag) {
			t.Errorf("ParseHashTag(%q): err = %v, want ErrInvalidTag", tag, err)
		}
	}
}
