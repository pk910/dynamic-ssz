// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

type joinAnnRoots [][]byte

var _ = sszutils.Annotate[joinAnnRoots](`ssz-size:"?,32" ssz-max:"8"`)

// Each struct is used as a plain container and as the descriptor of a
// TypeWrapper, a Union and a CompatibleUnion, so all four read the same tag.
type (
	joinAnnPartial struct {
		Data joinAnnRoots `ssz-max:"4"`
	}
	joinAnnUntagged struct{ Data joinAnnRoots }
	joinAnnOverride struct {
		Data joinAnnRoots `ssz-size:"?,64" ssz-max:"4"`
	}
	joinAnnDuplicate struct {
		Data joinAnnRoots `ssz-size:"?,32" ssz-max:"8"`
	}
)

// TestAnnotationJoinInWrappersAndUnions checks that a TypeWrapper descriptor
// field and union variants merge their tag with the type's annotation per key,
// like a container field does.
func TestAnnotationJoinInWrappersAndUnions(t *testing.T) {
	roots := func(size int) joinAnnRoots {
		return joinAnnRoots{bytes.Repeat([]byte{1}, size), bytes.Repeat([]byte{2}, size)}
	}

	// List[Vector[byte,32],4] of two roots, built by hand: no inner offsets.
	partialList := append(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)...)
	var zero [32]byte
	partialRoot := sha256.Sum256(append(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)...))
	partialRoot = sha256.Sum256(append(partialRoot[:], sha256Pair(zero, zero)...))
	var length [32]byte
	length[0] = 2
	partialRoot = sha256.Sum256(append(partialRoot[:], length[:]...))

	t.Run("partial", func(t *testing.T) {
		checkAnnotationJoin[joinAnnPartial](t, roots(32), partialList, partialRoot)
	})
	t.Run("untagged", func(t *testing.T) {
		checkAnnotationJoin[joinAnnUntagged](t, roots(32), nil, [32]byte{})
	})
	t.Run("override", func(t *testing.T) {
		checkAnnotationJoin[joinAnnOverride](t, roots(64), nil, [32]byte{})
	})
	t.Run("duplicate", func(t *testing.T) {
		checkAnnotationJoin[joinAnnDuplicate](t, roots(32), nil, [32]byte{})
	})
}

func sha256Pair(a, b [32]byte) []byte {
	sum := sha256.Sum256(append(a[:], b[:]...))
	return sum[:]
}

// checkAnnotationJoin encodes value as the Data field of D, then through
// TypeWrapper[D], Union[D] and CompatibleUnion[D], and expects the same list
// bytes and root everywhere. wantList and wantRoot pin the list when set.
func checkAnnotationJoin[D any](t *testing.T, value joinAnnRoots, wantList []byte, wantRoot [32]byte) {
	t.Helper()
	ds := NewDynSsz(nil)

	container := new(D)
	reflect.ValueOf(container).Elem().Field(0).Set(reflect.ValueOf(value))
	containerBytes, err := ds.MarshalSSZ(container)
	if err != nil {
		t.Fatalf("container marshal: %v", err)
	}
	list := containerBytes[4:]
	root, err := ds.HashTreeRoot(container)
	if err != nil {
		t.Fatalf("container root: %v", err)
	}
	if wantList != nil && (!bytes.Equal(list, wantList) || root != wantRoot) {
		t.Fatalf("container = %x / %x, want %x / %x", list, root, wantList, wantRoot)
	}

	mixSelector := func(selector byte) [32]byte {
		var sel [32]byte
		sel[0] = selector
		return sha256.Sum256(append(root[:], sel[:]...))
	}
	cases := []struct {
		name      string
		value     any
		wantBytes []byte
		wantRoot  [32]byte
	}{
		{"TypeWrapper", &TypeWrapper[D, joinAnnRoots]{Data: value}, list, root},
		{"Union", &Union[D]{Variant: 0, Data: value}, append([]byte{0}, list...), mixSelector(0)},
		{"CompatibleUnion", &CompatibleUnion[D]{Variant: 1, Data: value}, append([]byte{1}, list...), mixSelector(1)},
	}
	for _, c := range cases {
		got, err := ds.MarshalSSZ(c.value)
		if err != nil || !bytes.Equal(got, c.wantBytes) {
			t.Errorf("%s: MarshalSSZ = %x, %v; want %x", c.name, got, err, c.wantBytes)
		}
		if size, err := ds.SizeSSZ(c.value); err != nil || size != len(c.wantBytes) {
			t.Errorf("%s: SizeSSZ = %d, %v; want %d", c.name, size, err, len(c.wantBytes))
		}
		if got, err := ds.HashTreeRoot(c.value); err != nil || got != c.wantRoot {
			t.Errorf("%s: HashTreeRoot = %x, %v; want %x", c.name, got, err, c.wantRoot)
		}
		decoded := reflect.New(reflect.TypeOf(c.value).Elem()).Interface()
		if err := ds.UnmarshalSSZ(decoded, c.wantBytes); err != nil || !reflect.DeepEqual(decoded, c.value) {
			t.Errorf("%s: UnmarshalSSZ = %+v, %v; want %+v", c.name, decoded, err, c.value)
		}
	}
}
