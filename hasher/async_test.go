// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"fmt"
	"math/rand"
	"testing"

	hashtree "github.com/pk910/hashtree-bindings"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// asyncSequence describes one engine-style hashing sequence: a list of
// uniform container elements (elemChunks each), optionally preceded by raw
// chunks appended directly into the list scope, closed with the given
// merkleization. It mirrors what the reflection and codegen engines emit.
type asyncSequence struct {
	progressive  bool
	activeFields bool // progressive container closed with active-fields mixin
	rawPrefix    int  // raw chunks appended before the first element
	elemChunks   int  // chunks per element (0 = raw chunks instead of elements)
	n            int  // element count (or raw chunk count when elemChunks==0)
	cadence      int  // Collapse() every cadence elements; 0 = never
	limit        uint64
}

func (s asyncSequence) String() string {
	return fmt.Sprintf("prog=%v af=%v raw=%d elem=%d n=%d cadence=%d limit=%d",
		s.progressive, s.activeFields, s.rawPrefix, s.elemChunks, s.n, s.cadence, s.limit)
}

// runAsyncSequence drives hh through the sequence and returns the root.
func runAsyncSequence(t *testing.T, hh *Hasher, s asyncSequence, seed int64) [32]byte {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	chunk := make([]byte, 32)

	treeType := sszutils.TreeTypeBinary
	if s.progressive || s.activeFields {
		treeType = sszutils.TreeTypeProgressive
	}
	idx := hh.StartTree(treeType)

	for i := 0; i < s.rawPrefix; i++ {
		rng.Read(chunk)
		hh.Append(chunk)
	}
	for i := 0; i < s.n; i++ {
		if s.elemChunks == 0 {
			rng.Read(chunk)
			hh.Append(chunk)
		} else {
			ci := hh.StartTree(sszutils.TreeTypeNone)
			for c := 0; c < s.elemChunks; c++ {
				rng.Read(chunk)
				hh.Append(chunk)
			}
			hh.Merkleize(ci)
		}
		if s.cadence > 0 && (i+1)%s.cadence == 0 {
			hh.Collapse()
		}
	}

	switch {
	case s.activeFields:
		hh.MerkleizeProgressiveWithActiveFields(idx, []byte{0xff, 0xff})
	case s.progressive:
		hh.MerkleizeProgressiveWithMixin(idx, uint64(s.n))
	default:
		hh.MerkleizeWithMixin(idx, uint64(s.n), s.limit)
	}

	root, err := hh.HashRoot()
	if err != nil {
		t.Fatalf("%s: HashRoot: %v", s, err)
	}
	hh.Reset()
	return root
}

// TestAsyncMatchesSync verifies that background reduction produces exactly
// the roots the synchronous path produces, across list kinds, element
// widths, batch and progressive-group boundaries, and collapse cadences.
func TestAsyncMatchesSync(t *testing.T) {
	defer DisableAsyncHashing()

	cases := make([]asyncSequence, 0, 170)
	for _, prog := range []bool{false, true} {
		for _, elemChunks := range []int{8, 4, 2} {
			for _, n := range []int{0, 1, 255, 4095, 4096, 4097, 5460, 5461, 5462,
				8192, 16384, 21845, 50000} {
				for _, cadence := range []int{256, 100} {
					cases = append(cases, asyncSequence{
						progressive: prog,
						elemChunks:  elemChunks,
						n:           n,
						cadence:     cadence,
						limit:       1 << 40,
					})
				}
			}
		}
		// Raw packed chunks (uint64/byte lists): no deferred runs, but the
		// progressive variant exercises deferred group finalization and
		// background group reductions.
		for _, n := range []int{4096, 21845, 87381, 200000} {
			cases = append(cases, asyncSequence{
				progressive: prog,
				n:           n,
				cadence:     64,
				limit:       1 << 40,
			})
		}
	}
	// Raw chunks appended before the first element break run alignment in
	// the layer's leaf space; the async root flush must fall back to the
	// synchronous path for correctness.
	cases = append(cases,
		asyncSequence{rawPrefix: 3, elemChunks: 8, n: 8192, cadence: 256, limit: 1 << 40},
		asyncSequence{progressive: true, rawPrefix: 3, elemChunks: 8, n: 8192, cadence: 256, limit: 1 << 40},
		// Progressive container with an active-fields mixin.
		asyncSequence{activeFields: true, elemChunks: 4, n: 12000, cadence: 256},
		// Collapse never called: everything reduces at finalization.
		asyncSequence{elemChunks: 8, n: 10000, cadence: 0, limit: 1 << 40},
		asyncSequence{progressive: true, elemChunks: 8, n: 10000, cadence: 0},
	)

	for i, s := range cases {
		seed := int64(i + 1)

		DisableAsyncHashing()
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		want := runAsyncSequence(t, hh, s, seed)

		EnableAsyncHashing(4)
		got := runAsyncSequence(t, hh, s, seed)

		if got != want {
			t.Errorf("%s: async root %x != sync root %x", s, got, want)
		}
	}
}

// TestAsyncTailAfterRun appends a raw chunk after the deferred run before
// Collapse fires, so the async compaction has to move trailing bytes down
// behind the hole.
func TestAsyncTailAfterRun(t *testing.T) {
	defer DisableAsyncHashing()

	run := func() [32]byte {
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		rng := rand.New(rand.NewSource(11))
		chunk := make([]byte, 32)

		idx := hh.StartTree(sszutils.TreeTypeBinary)
		for i := 0; i < 4096; i++ {
			ci := hh.StartTree(sszutils.TreeTypeNone)
			for c := 0; c < 8; c++ {
				rng.Read(chunk)
				hh.Append(chunk)
			}
			hh.Merkleize(ci)
		}
		rng.Read(chunk)
		hh.Append(chunk)
		hh.Collapse()
		hh.MerkleizeWithMixin(idx, 4097, 1<<40)
		root, err := hh.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		return root
	}

	DisableAsyncHashing()
	want := run()
	EnableAsyncHashing(4)
	got := run()
	if got != want {
		t.Errorf("async root %x != sync root %x", got, want)
	}
}

// TestAsyncResetInFlight resets a hasher while background reductions are
// outstanding and verifies it is reusable and leaks nothing into the next
// computation.
func TestAsyncResetInFlight(t *testing.T) {
	EnableAsyncHashing(4)
	defer DisableAsyncHashing()

	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	rng := rand.New(rand.NewSource(7))
	chunk := make([]byte, 32)

	hh.StartTree(sszutils.TreeTypeBinary)
	for i := 0; i < 5000; i++ {
		ci := hh.StartTree(sszutils.TreeTypeNone)
		for c := 0; c < 8; c++ {
			rng.Read(chunk)
			hh.Append(chunk)
		}
		hh.Merkleize(ci)
		if (i+1)%256 == 0 {
			hh.Collapse()
		}
	}
	// Abandon the computation with jobs likely in flight.
	hh.Reset()
	if len(hh.pendingJobs) != 0 {
		t.Fatalf("pendingJobs not drained by Reset: %d", len(hh.pendingJobs))
	}

	s := asyncSequence{elemChunks: 8, n: 8192, cadence: 256, limit: 1 << 40}
	got := runAsyncSequence(t, hh, s, 42)

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 42)
	if got != want {
		t.Errorf("root after in-flight Reset %x != sync root %x", got, want)
	}
}

// TestAsyncNativeHasherExcluded verifies that hashers wrapping a stateful
// hash.Hash never take the async path (their hash function is not safe for
// concurrent use) and still produce correct roots while async hashing is
// enabled globally.
func TestAsyncNativeHasherExcluded(t *testing.T) {
	EnableAsyncHashing(4)
	defer DisableAsyncHashing()

	hh := NewHasher()
	if hh.asyncSlots() != nil {
		t.Fatal("native-hash hasher must not participate in async hashing")
	}

	s := asyncSequence{elemChunks: 8, n: 5000, cadence: 256, limit: 1 << 40}
	got := runAsyncSequence(t, hh, s, 9)

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 9)
	if got != want {
		t.Errorf("native root %x != reference root %x", got, want)
	}
}

// TestAsyncEnableDisable covers the toggle edge cases.
func TestAsyncEnableDisable(t *testing.T) {
	defer DisableAsyncHashing()

	EnableAsyncHashing(0)
	if asyncSem.Load() != nil {
		t.Error("EnableAsyncHashing(0) must disable async hashing")
	}
	EnableAsyncHashing(-1)
	if asyncSem.Load() != nil {
		t.Error("EnableAsyncHashing(-1) must disable async hashing")
	}
	EnableAsyncHashing(2)
	if sem := asyncSem.Load(); sem == nil || cap(*sem) != 2 {
		t.Error("EnableAsyncHashing(2) must install a 2-slot limiter")
	}
	DisableAsyncHashing()
	if asyncSem.Load() != nil {
		t.Error("DisableAsyncHashing must remove the limiter")
	}
}

// TestAsyncConcurrentHashers runs several hashers concurrently with async
// hashing enabled, sharing the global worker limit.
func TestAsyncConcurrentHashers(t *testing.T) {
	EnableAsyncHashing(4)
	defer DisableAsyncHashing()

	s := asyncSequence{progressive: true, elemChunks: 8, n: 21845, cadence: 256}

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 5)
	EnableAsyncHashing(4)

	done := make(chan [32]byte, 8)
	for i := 0; i < 8; i++ {
		go func() {
			hh := FastHasherPool.Get()
			defer FastHasherPool.Put(hh)
			done <- runAsyncSequence(t, hh, s, 5)
		}()
	}
	for i := 0; i < 8; i++ {
		if got := <-done; got != want {
			t.Errorf("concurrent hasher root %x != sync root %x", got, want)
		}
	}
}
