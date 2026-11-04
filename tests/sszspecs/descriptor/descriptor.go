// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package descriptor reads the type descriptors the official SSZ vectors
// carry. The type generator and the conformance test both use it, so a
// descriptor names the same generated Go type on either side.
package descriptor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Descriptor is one SSZ type declaration as a vector states it. Which of the
// fields are set depends on Kind. A pointer distinguishes a count the
// declaration leaves out from one it states as zero.
type Descriptor struct {
	Kind         string      `json:"kind"`
	Bits         *int64      `json:"bits,omitempty"`
	Length       *int64      `json:"length,omitempty"`
	Limit        *int64      `json:"limit,omitempty"`
	ElementType  *Descriptor `json:"elementType,omitempty"`
	Fields       []Field     `json:"fields,omitempty"`
	ActiveFields []any       `json:"activeFields,omitempty"`
	Options      []Option    `json:"options,omitempty"`

	// raw keeps the declaration as it was read, so two declarations that
	// differ in a key this struct does not model still get different keys.
	raw json.RawMessage
}

// Field is one field of a container or progressive container.
type Field struct {
	Name string      `json:"name"`
	Type *Descriptor `json:"type"`
}

// Option is one option of a compatible union.
type Option struct {
	Selector any         `json:"selector"`
	Type     *Descriptor `json:"type"`
}

// UnmarshalJSON reads a descriptor and keeps its source text for Key.
func (d *Descriptor) UnmarshalJSON(data []byte) error {
	type plain Descriptor

	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}

	*d = Descriptor(p)
	d.raw = append(json.RawMessage(nil), data...)

	return nil
}

// Key identifies the declaration: equal declarations give equal keys whatever
// order their JSON keys were written in.
func (d *Descriptor) Key() (string, error) {
	var generic any
	if err := json.Unmarshal(d.raw, &generic); err != nil {
		return "", err
	}

	// encoding/json writes map keys sorted, which makes this canonical.
	canonical, err := json.Marshal(generic)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(canonical)

	return hex.EncodeToString(sum[:8]), nil
}
