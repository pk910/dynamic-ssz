// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package ssztypes

import (
	"testing"
)

func TestParseHashTag(t *testing.T) {
	for tag, want := range map[string]string{
		`ssz-hash:"sha256"`:                    "sha256",
		`ssz-static:"true" ssz-hash:"my-hash"`: "my-hash",
	} {
		if got, ok := ParseHashTag(tag); !ok || got != want {
			t.Errorf("ParseHashTag(%q) = %q, %v; want %q", tag, got, ok, want)
		}
	}
	for _, tag := range []string{``, `ssz-max:"4"`, `ssz-hash:""`} {
		if got, ok := ParseHashTag(tag); ok {
			t.Errorf("ParseHashTag(%q) = %q, want no hash", tag, got)
		}
	}
}
