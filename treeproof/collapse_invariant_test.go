// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// shape is one hashing program: a list of elemChunks-sized elements, optionally
// preceded by raw chunks, closed by one of the merkleizations.
type shape struct {
	name        string
	rawPrefix   int
	elemChunks  int
	n           int
	limit       uint64
	progressive bool
	active      bool
	mixin       bool
}

func (s shape) String() string {
	return fmt.Sprintf("%s(raw=%d elem=%d n=%d limit=%d prog=%v act=%v mix=%v)",
		s.name, s.rawPrefix, s.elemChunks, s.n, s.limit, s.progressive, s.active, s.mixin)
}

// run drives one program, sending a Collapse hint every cadence elements
// (cadence 0 sends none), and returns the root.
func run(w sszutils.HashWalker, s shape, cadence int) {
	chunk := make([]byte, 32)
	counter := uint64(0)
	fill := func() {
		counter++
		binary.LittleEndian.PutUint64(chunk, counter)
		binary.LittleEndian.PutUint64(chunk[24:], ^counter)
	}

	treeType := sszutils.TreeTypeBinary
	if s.progressive || s.active {
		treeType = sszutils.TreeTypeProgressive
	}
	idx := w.StartTree(treeType)

	for i := 0; i < s.rawPrefix; i++ {
		fill()
		w.Append(chunk)
		if cadence > 0 && (i+1)%cadence == 0 {
			w.Collapse()
		}
	}
	for i := 0; i < s.n; i++ {
		if s.elemChunks == 0 {
			fill()
			w.Append(chunk)
		} else {
			ci := w.StartTree(sszutils.TreeTypeNone)
			for c := 0; c < s.elemChunks; c++ {
				fill()
				w.Append(chunk)
			}
			w.Merkleize(ci)
		}
		if cadence > 0 && (i+1)%cadence == 0 {
			w.Collapse()
		}
	}

	switch {
	case s.active:
		w.MerkleizeProgressiveWithActiveFields(idx, []byte{0xff, 0xff})
	case s.progressive:
		w.MerkleizeProgressiveWithMixin(idx, uint64(s.n))
	case s.mixin:
		w.MerkleizeWithMixin(idx, uint64(s.n), s.limit)
	default:
		w.Merkleize(idx)
	}
}

// A reduction handed more chunks than its limit holds runs over a tree deep
// enough for them and refuses the root. The hint must not move that root
// either, so it is read from the walk where the refusal withholds it.
func hasherRoot(t *testing.T, s shape, cadence int) [32]byte {
	t.Helper()
	h := hasher.NewHasher()
	run(h, s, cadence)
	root, err := h.HashRoot()
	if errors.Is(err, sszutils.ErrChunkLimitExceeded) {
		copy(root[:], h.Hash())
		err = nil
	}
	if err != nil {
		t.Fatalf("%s cadence=%d: hasher: %v", s, cadence, err)
	}
	return root
}

func wrapperRoot(t *testing.T, s shape, cadence int) [32]byte {
	t.Helper()
	w := NewWrapper()
	run(w, s, cadence)
	root, err := w.HashRoot()
	if errors.Is(err, sszutils.ErrChunkLimitExceeded) {
		copy(root[:], w.Hash())
		err = nil
	}
	if err != nil {
		t.Fatalf("%s cadence=%d: wrapper: %v", s, cadence, err)
	}
	return root
}

// A Collapse hint is optional. It must never change the root, on either
// walker, and the two walkers must agree.
func TestCollapseNeverMovesTheRoot(t *testing.T) {
	var shapes []shape
	for _, n := range []int{1, 2, 3, 5, 8, 9, 16, 17, 33, 64, 100} {
		for _, elem := range []int{0, 1, 2, 3, 8} {
			shapes = append(shapes,
				shape{name: "binary", elemChunks: elem, n: n, mixin: true, limit: 1 << 20},
				shape{name: "binary-nolimit", elemChunks: elem, n: n},
				shape{name: "progressive", elemChunks: elem, n: n, progressive: true},
			)
			if n <= 17 {
				shapes = append(shapes,
					shape{name: "over-limit", elemChunks: elem, n: n, mixin: true, limit: 3},
					shape{name: "raw-prefix", rawPrefix: 3, elemChunks: elem, n: n, mixin: true, limit: 1 << 20},
					shape{name: "active-fields", elemChunks: elem, n: n, active: true},
				)
			}
		}
	}

	cadences := []int{1, 2, 3, 7, 64}
	var moved, disagreed int
	for _, s := range shapes {
		base := hasherRoot(t, s, 0)
		wbase := wrapperRoot(t, s, 0)
		if base != wbase {
			disagreed++
			t.Errorf("%s: walkers disagree without any hint: hasher %x wrapper %x", s, base, wbase)
		}
		for _, cadence := range cadences {
			if got := hasherRoot(t, s, cadence); got != base {
				moved++
				t.Errorf("%s cadence=%d: hint moved the hasher root: %x, want %x", s, cadence, got, base)
			}
			if got := wrapperRoot(t, s, cadence); got != wbase {
				moved++
				t.Errorf("%s cadence=%d: hint moved the wrapper root: %x, want %x", s, cadence, got, wbase)
			}
		}
	}
	t.Logf("programs=%d cadences=%d roots compared=%d moved=%d walker disagreements=%d",
		len(shapes), len(cadences), len(shapes)*(1+2*len(cadences)), moved, disagreed)
}

// A scope declares its tree shape when it is opened, and the reduction has to
// match it: the two shapes disagree about what the accumulated chunks mean.
// Both walkers refuse either mismatch rather than pick an answer, and every
// reduction is guarded, not only the binary pair.
func TestScopeShapeMismatchIsRefused(t *testing.T) {
	appendChunks := func(w sszutils.HashWalker, cadence int) {
		chunk := make([]byte, 32)
		for i := 0; i < 17; i++ {
			chunk[0] = byte(i)
			w.Append(chunk)
			if cadence > 0 && (i+1)%cadence == 0 {
				w.Collapse()
			}
		}
	}

	cases := []struct {
		name  string
		close func(w sszutils.HashWalker, idx int)
		open  sszutils.TreeType
	}{
		{"progressive closed binary", func(w sszutils.HashWalker, idx int) { w.Merkleize(idx) }, sszutils.TreeTypeProgressive},
		{"progressive closed binary with mixin", func(w sszutils.HashWalker, idx int) { w.MerkleizeWithMixin(idx, 17, 64) }, sszutils.TreeTypeProgressive},
		{"binary closed progressive", func(w sszutils.HashWalker, idx int) { w.MerkleizeProgressive(idx) }, sszutils.TreeTypeBinary},
		{"binary closed progressive with mixin", func(w sszutils.HashWalker, idx int) { w.MerkleizeProgressiveWithMixin(idx, 17) }, sszutils.TreeTypeBinary},
		{"binary closed progressive with active fields", func(w sszutils.HashWalker, idx int) {
			w.MerkleizeProgressiveWithActiveFields(idx, []byte{0xff, 0xff})
		}, sszutils.TreeTypeBinary},
	}

	for _, tc := range cases {
		for _, cadence := range []int{0, 1, 4} {
			h := hasher.NewHasher()
			runShapeCase(h, tc.open, cadence, appendChunks, tc.close)
			if _, err := h.HashRoot(); !errors.Is(err, sszutils.ErrScopeShapeMismatch) {
				t.Errorf("%s: hasher cadence=%d: err = %v, want %v", tc.name, cadence, err, sszutils.ErrScopeShapeMismatch)
			}

			w := NewWrapper()
			runShapeCase(w, tc.open, cadence, appendChunks, tc.close)
			if _, err := w.HashRoot(); !errors.Is(err, sszutils.ErrScopeShapeMismatch) {
				t.Errorf("%s: wrapper cadence=%d: err = %v, want %v", tc.name, cadence, err, sszutils.ErrScopeShapeMismatch)
			}
		}
	}
}

// runShapeCase opens a scope in one shape and reduces it in whichever the case
// names, which is the mismatch under test.
func runShapeCase(w sszutils.HashWalker, open sszutils.TreeType, cadence int,
	body func(sszutils.HashWalker, int), reduce func(sszutils.HashWalker, int)) {
	idx := w.StartTree(open)
	body(w, cadence)
	reduce(w, idx)
}

// A scope that declares no shape accepts either reduction: generated code
// written before StartTree opens a progressive container with Index, and code
// from the v1.3 line opens one with TreeTypeNone. Both still work, and the
// hint does not move those roots either.
func TestUndeclaredScopeClosedProgressiveStillWorks(t *testing.T) {
	root := func(cadence int, useIndex bool) [32]byte {
		h := hasher.NewHasher()
		var idx int
		if useIndex {
			idx = h.Index()
		} else {
			idx = h.StartTree(sszutils.TreeTypeNone)
		}
		chunk := make([]byte, 32)
		for i := 0; i < 17; i++ {
			chunk[0] = byte(i)
			h.Append(chunk)
			if cadence > 0 && (i+1)%cadence == 0 {
				h.Collapse()
			}
		}
		h.MerkleizeProgressiveWithMixin(idx, 17)
		r, err := h.HashRoot()
		if err != nil {
			t.Fatalf("cadence=%d: %v", cadence, err)
		}
		return r
	}

	for _, useIndex := range []bool{true, false} {
		base := root(0, useIndex)
		for _, cadence := range []int{1, 4} {
			if got := root(cadence, useIndex); got != base {
				t.Errorf("index=%v cadence=%d moved the root: %x, want %x", useIndex, cadence, got, base)
			}
		}
	}
}
