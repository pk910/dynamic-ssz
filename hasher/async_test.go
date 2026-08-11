// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"bytes"
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

// runAsyncSequence drives hh through the sequence and returns the root. The
// hasher is gated into async hashing; whether reductions actually run in the
// background is controlled by the process-wide Enable/Disable toggle.
func runAsyncSequence(t *testing.T, hh *Hasher, s asyncSequence, seed int64) [32]byte {
	t.Helper()
	hh.SetAsyncHashing(true)
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
		if s.cadence > 0 && (i+1)%s.cadence == 0 {
			hh.Collapse()
		}
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
		for _, elemChunks := range []int{8, 4, 2, 3, 5, 7, 12} {
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
		// The partial last group left at finalization exceeds one job cap
		// (60000 - 21845 > lazyFlushChunks), so the finalization rounds
		// emit pre-collapse jobs whose holes the partial-remainder collapse
		// must drain before reading.
		asyncSequence{progressive: true, elemChunks: 8, n: 60000, cadence: 256},
		asyncSequence{progressive: true, elemChunks: 4, n: 60000, cadence: 100},
		// A raw prefix that happens to be node-aligned: a completed subtree
		// node must still not be appended behind plain depth-0 chunks — the
		// whole run has to reduce synchronously.
		asyncSequence{rawPrefix: 4096, elemChunks: 8, n: 8192, cadence: 256, limit: 1 << 40},
		// Element sizes outside the deferral bounds: a single-chunk scope is
		// below the two-chunk minimum and a 257-chunk scope exceeds
		// incrementalBatchSize, so both merkleize immediately instead of
		// joining a deferred run.
		asyncSequence{elemChunks: 1, n: 300, cadence: 64, limit: 1 << 40},
		asyncSequence{elemChunks: 257, n: 300, cadence: 64, limit: 1 << 40},
		// A raw prefix wide enough that its collapse emits a cap-depth node
		// and leaves depth-0 chunks in front of the element run: the run's
		// completed-subtree node is incompatible with those shallower leaves,
		// so the async flush must reject it and reduce synchronously.
		asyncSequence{rawPrefix: 33024, elemChunks: 8, n: 4200, cadence: 256, limit: 1 << 40},
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
		hh.SetAsyncHashing(true)
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
	hh.SetAsyncHashing(true)
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
	if hh.jobCount != 0 {
		t.Fatalf("job ring not drained by Reset: %d", hh.jobCount)
	}

	s := asyncSequence{elemChunks: 8, n: 8192, cadence: 256, limit: 1 << 40}
	got := runAsyncSequence(t, hh, s, 42)

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 42)
	if got != want {
		t.Errorf("root after in-flight Reset %x != sync root %x", got, want)
	}
}

// TestAsyncGate verifies the two-level gating: the process-wide toggle and
// the per-hasher SetAsyncHashing flag must both be set, and Reset clears the
// per-hasher flag so pooled hashers never leak the setting to another user.
func TestAsyncGate(t *testing.T) {
	defer DisableAsyncHashing()

	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	DisableAsyncHashing()
	if hh.asyncShared() != nil {
		t.Error("hasher must be gated out by default")
	}
	hh.SetAsyncHashing(true)
	if hh.asyncShared() != nil {
		t.Error("process-wide disable must override the per-hasher gate")
	}
	EnableAsyncHashing(4)
	if hh.asyncShared() == nil {
		t.Error("gated-in hasher must see the shared state")
	}
	hh.SetAsyncHashing(false)
	if hh.asyncShared() != nil {
		t.Error("gated-out hasher must not see the shared state")
	}
	hh.SetAsyncHashing(true)
	hh.Reset()
	if hh.asyncShared() != nil {
		t.Error("Reset must clear the per-hasher gate")
	}

	// An ungated hasher hashes synchronously while async is enabled, with
	// identical roots.
	s := asyncSequence{elemChunks: 8, n: 5000, cadence: 256, limit: 1 << 40}
	got := runAsyncSequence(t, hh, s, 9)

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 9)
	if got != want {
		t.Errorf("root %x != reference root %x", got, want)
	}
}

// TestAsyncEnableDisable covers the toggle edge cases.
func TestAsyncEnableDisable(t *testing.T) {
	defer DisableAsyncHashing()

	EnableAsyncHashing(0)
	if asyncState.Load() != nil {
		t.Error("EnableAsyncHashing(0) must disable async hashing")
	}
	EnableAsyncHashing(-1)
	if asyncState.Load() != nil {
		t.Error("EnableAsyncHashing(-1) must disable async hashing")
	}
	EnableAsyncHashing(2)
	if st := asyncState.Load(); st == nil || cap(st.sem) != 2 || cap(st.bufs) != 4 {
		t.Error("EnableAsyncHashing(2) must install a 2-worker limiter with 4 buffer tokens")
	}
	DisableAsyncHashing()
	if asyncState.Load() != nil {
		t.Error("DisableAsyncHashing must remove the limiter")
	}

	// Worker-limit backstop: occupy every runner with a blocking job so none
	// is parked idle, then enable with a worker limit the pool has already
	// reached — no additional runner may spawn.
	release := make(chan struct{})
	started := make(chan struct{})
	blockFn := func(dst []byte, input []byte) error {
		started <- struct{}{}
		<-release
		return nil
	}
	EnableAsyncHashing(int(asyncRunners.Load()) + 8)
	st := asyncState.Load()
	jobs := make([]*asyncJob, 0, asyncRunners.Load())
	for int64(len(jobs)) < asyncRunners.Load() {
		st.sem <- struct{}{}
		job := &asyncJob{st: st, fn: blockFn, in: make([]byte, 64), outLen: 32,
			done: make(chan struct{}, 1)}
		asyncHandoff <- job
		<-started
		jobs = append(jobs, job)
	}
	runners := asyncRunners.Load()
	EnableAsyncHashing(1)
	if got := asyncRunners.Load(); got != runners {
		t.Errorf("runner spawned past the worker limit: %d != %d", got, runners)
	}
	close(release)
	for _, job := range jobs {
		<-job.done
	}
}

// TestAsyncCollapseSubCapRemainder drives Collapse over deferred runs wider
// than one job cap but not a multiple of it, so the capped async flush
// leaves a sub-cap remainder pending: a binary layer keeps the remainder
// pending for the next round, while a progressive layer reduces it
// synchronously before group finalization. The runs are registered on the
// layer directly, mirroring the state the Merkleize deferral path builds —
// that path self-flushes at exactly the cap, so it never presents an
// over-cap run to Collapse itself.
func TestAsyncCollapseSubCapRemainder(t *testing.T) {
	defer DisableAsyncHashing()

	registerRun := func(hh *Hasher, rng *rand.Rand, idx, elemChunks, n int) {
		chunk := make([]byte, 32)
		for i := 0; i < n*elemChunks; i++ {
			rng.Read(chunk)
			hh.Append(chunk)
		}
		layer := &hh.layers[hh.layerCount]
		layer.pendStart = idx
		layer.pendElemChunks = elemChunks
		layer.pendCount = n
	}

	t.Run("binary", func(t *testing.T) {
		const n = 4100 // one cap-sized job of 4096 elements plus a remainder of 4
		s := asyncSequence{elemChunks: 8, n: n, limit: 1 << 40}
		DisableAsyncHashing()
		want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 83)

		EnableAsyncHashing(4)
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		hh.SetAsyncHashing(true)
		idx := hh.StartTree(sszutils.TreeTypeBinary)
		registerRun(hh, rand.New(rand.NewSource(83)), idx, 8, n)
		hh.Collapse()
		if pend := hh.layers[hh.layerCount].pendCount; pend != 4 {
			t.Fatalf("remainder not kept pending after Collapse: %d != 4", pend)
		}
		hh.MerkleizeWithMixin(idx, n, 1<<40)
		got, err := hh.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		if got != want {
			t.Errorf("binary remainder root %x != sync root %x", got, want)
		}
	})

	t.Run("progressive", func(t *testing.T) {
		// Eight cap-sized jobs of 16384 two-chunk elements plus a remainder
		// of 4: the flush compacts the run to 131080 chunks of element
		// roots, just past the deferred-finalization bound
		// (asyncActiveChunks), so Collapse reduces the remainder and
		// finalizes instead of returning early.
		const n = 8*16384 + 4
		s := asyncSequence{progressive: true, elemChunks: 2, n: n}
		DisableAsyncHashing()
		want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 89)

		EnableAsyncHashing(4)
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		hh.SetAsyncHashing(true)
		idx := hh.StartTree(sszutils.TreeTypeProgressive)
		registerRun(hh, rand.New(rand.NewSource(89)), idx, 2, n)
		hh.Collapse()
		if pend := hh.layers[hh.layerCount].pendCount; pend != 0 {
			t.Fatalf("remainder not reduced before finalization: %d != 0", pend)
		}
		hh.MerkleizeProgressiveWithMixin(idx, n)
		got, err := hh.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		if got != want {
			t.Errorf("progressive remainder root %x != sync root %x", got, want)
		}
	})
}

// TestAsyncRingFull runs enough flushes past a tiny worker limit that the
// job ring fills and enqueueing drains the oldest job to reuse its slot.
func TestAsyncRingFull(t *testing.T) {
	defer DisableAsyncHashing()

	s := asyncSequence{elemChunks: 8, n: 40960, cadence: 256, limit: 1 << 40}

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 21)

	EnableAsyncHashing(1)
	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	got := runAsyncSequence(t, hh, s, 21)
	if got != want {
		t.Errorf("ring-full async root %x != sync root %x", got, want)
	}
}

// TestAsyncReconfigure shrinks the worker count between computations, so a
// hasher whose ring was sized for the larger configuration must respect the
// smaller free list of the new shared state.
func TestAsyncReconfigure(t *testing.T) {
	defer DisableAsyncHashing()

	s := asyncSequence{elemChunks: 8, n: 40960, cadence: 256, limit: 1 << 40}

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 33)

	EnableAsyncHashing(8)
	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	if got := runAsyncSequence(t, hh, s, 33); got != want {
		t.Errorf("root with 8 workers %x != sync root %x", got, want)
	}
	EnableAsyncHashing(1)
	if got := runAsyncSequence(t, hh, s, 33); got != want {
		t.Errorf("root after shrink to 1 worker %x != sync root %x", got, want)
	}
}

// driveFlushes drives hh through count async flushes of 4096 8-chunk
// elements each inside an open binary list scope, without finalizing.
func driveFlushes(hh *Hasher, rng *rand.Rand, count int) {
	chunk := make([]byte, 32)
	for i := 0; i < count*4096; i++ {
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
}

// TestAsyncRingOverflow drives a hasher into a full ring that still finds a
// free token (possible when a tokenless job sits under tokened ones), so
// claiming the next slot has to drain the oldest job for ring space rather
// than for a token. The token distribution is orchestrated through a second
// hasher that parks while holding tokens.
func TestAsyncRingOverflow(t *testing.T) {
	EnableAsyncHashing(8) // ring and token count: 16
	defer DisableAsyncHashing()

	rng := rand.New(rand.NewSource(51))
	parker := NewHasherWithHashFn(hashtree.HashByteSlice)
	parker.SetAsyncHashing(true)
	worker := NewHasherWithHashFn(hashtree.HashByteSlice)
	worker.SetAsyncHashing(true)

	// parker takes all 16 tokens and parks.
	parker.StartTree(sszutils.TreeTypeBinary)
	driveFlushes(parker, rng, 16)

	// worker's first job finds no token: tokenless path.
	worker.StartTree(sszutils.TreeTypeBinary)
	driveFlushes(worker, rng, 1)

	// 15 tokens come free; worker stacks 15 tokened jobs on the tokenless
	// one, filling its ring.
	for i := 0; i < 15; i++ {
		parker.drainOldestJob()
	}
	driveFlushes(worker, rng, 15)
	if worker.jobCount != 16 {
		t.Fatalf("worker ring not full: %d", worker.jobCount)
	}

	// One more free token: the next enqueue gets it on the first try with a
	// full ring, forcing the ring-space drain in claimJobSlot.
	parker.drainOldestJob()
	driveFlushes(worker, rng, 1)

	parker.Reset()
	worker.Reset()
	if parker.jobCount != 0 || worker.jobCount != 0 {
		t.Fatalf("rings not drained: parker=%d worker=%d", parker.jobCount, worker.jobCount)
	}
}

// TestAsyncLargeWorkerCount exercises the ring allocation beyond the inline
// backing array.
func TestAsyncLargeWorkerCount(t *testing.T) {
	defer DisableAsyncHashing()

	s := asyncSequence{elemChunks: 8, n: 40960, cadence: 256, limit: 1 << 40}

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 61)

	EnableAsyncHashing(9) // ring size 18 > asyncRingInline
	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	if len(hh.jobRingBuf) >= 18 {
		t.Fatal("test requires ring larger than the inline backing")
	}
	got := runAsyncSequence(t, hh, s, 61)
	if got != want {
		t.Errorf("large-worker async root %x != sync root %x", got, want)
	}
}

// TestAsyncStaleSiblingLayer runs two sibling scopes at the same stack depth
// inside one walk: the first (a collapsed raw-chunk vector of non-aligned
// length) leaves stale counts in the reused layer slot, and the second (a
// large deferred-element list) must not let that stale state influence its
// async flush decisions. Mirrors a beacon state where historical roots
// precede the validator registry.
func TestAsyncStaleSiblingLayer(t *testing.T) {
	defer DisableAsyncHashing()

	run := func() [32]byte {
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		hh.SetAsyncHashing(true)
		rng := rand.New(rand.NewSource(23))
		chunk := make([]byte, 32)

		outer := hh.StartTree(sszutils.TreeTypeNone)

		vec := hh.StartTree(sszutils.TreeTypeBinary)
		for i := 0; i < 758; i++ {
			rng.Read(chunk)
			hh.Append(chunk)
			if (i+1)%256 == 0 {
				hh.Collapse()
			}
		}
		hh.Merkleize(vec)

		list := hh.StartTree(sszutils.TreeTypeBinary)
		for i := 0; i < 12288; i++ {
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
		hh.MerkleizeWithMixin(list, 12288, 1<<40)

		hh.Merkleize(outer)
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
		t.Errorf("stale-sibling async root %x != sync root %x", got, want)
	}
}

// TestAsyncAccessorsDrain covers the accessor boundary: after an async flush
// compacts a run to a single placeholder chunk, a mid-scope HashRoot must
// not mistake the 32-byte buffer for a finished computation, and Hash must
// never expose unfilled hole bytes.
func TestAsyncAccessorsDrain(t *testing.T) {
	EnableAsyncHashing(4)
	defer DisableAsyncHashing()

	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	hh.SetAsyncHashing(true)
	rng := rand.New(rand.NewSource(37))
	chunk := make([]byte, 32)

	elems := make([][]byte, 4096)
	idx := hh.Index()
	for i := range elems {
		ci := hh.Index()
		elems[i] = make([]byte, 0, 8*32)
		for c := 0; c < 8; c++ {
			rng.Read(chunk)
			elems[i] = append(elems[i], chunk...)
			hh.Append(chunk)
		}
		hh.Merkleize(ci)
	}
	if hh.jobCount == 0 {
		t.Fatal("expected an async reduction in flight")
	}

	// Mid-scope: the buffer holds exactly one placeholder chunk, but the
	// computation is not finished — HashRoot must refuse.
	if _, err := hh.HashRoot(); err == nil {
		t.Error("HashRoot must reject unfinished scopes")
	}

	// Hash drains before reading, so it returns the completed subtree root,
	// not the stale first chunk of the reduced run.
	got := make([]byte, 32)
	copy(got, hh.Hash())

	ref := NewHasherWithHashFn(hashtree.HashByteSlice)
	refIdx := ref.StartTree(sszutils.TreeTypeBinary)
	for _, e := range elems {
		ci := ref.StartTree(sszutils.TreeTypeNone)
		ref.Append(e)
		ref.Merkleize(ci)
	}
	ref.Merkleize(refIdx)
	if !bytes.Equal(got, ref.Hash()) {
		t.Errorf("mid-scope Hash %x != subtree root %x", got, ref.Hash())
	}

	// Finishing the scope still yields a valid root.
	hh.MerkleizeWithMixin(idx, 4096, 1<<40)
	if _, err := hh.HashRoot(); err != nil {
		t.Errorf("finished HashRoot: %v", err)
	}
	hh.Reset()
}

// TestAsyncLegacyIndexDriven drives scopes the way fastssz-generated code
// does — Index/Merkleize with no Collapse hints — and verifies the deferral
// path batches and self-flushes those runs with identical roots in both
// modes, matching the StartTree-driven root.
func TestAsyncLegacyIndexDriven(t *testing.T) {
	defer DisableAsyncHashing()

	const n = 20000
	legacy := func() [32]byte {
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		hh.SetAsyncHashing(true)
		rng := rand.New(rand.NewSource(29))
		chunk := make([]byte, 32)

		idx := hh.Index()
		for i := 0; i < n; i++ {
			ci := hh.Index()
			for c := 0; c < 8; c++ {
				rng.Read(chunk)
				hh.Append(chunk)
			}
			hh.Merkleize(ci)
		}
		hh.MerkleizeWithMixin(idx, n, 1<<40)
		root, err := hh.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		return root
	}

	DisableAsyncHashing()
	want := legacy()
	s := asyncSequence{elemChunks: 8, n: n, cadence: 256, limit: 1 << 40}
	if ref := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 29); ref != want {
		t.Errorf("legacy sync root %x != StartTree-driven root %x", want, ref)
	}

	EnableAsyncHashing(4)
	if got := legacy(); got != want {
		t.Errorf("legacy async root %x != sync root %x", got, want)
	}
}

// TestAsyncNativeFactory runs async hashing on the default native-hash
// hasher: NativeHashWrapperFactory draws per-call instances from a pool, so
// workers and walker never share hash state.
func TestAsyncNativeFactory(t *testing.T) {
	defer DisableAsyncHashing()

	s := asyncSequence{elemChunks: 8, n: 12288, cadence: 256, limit: 1 << 40}

	DisableAsyncHashing()
	want := runAsyncSequence(t, NewHasher(), s, 13)

	EnableAsyncHashing(4)
	got := runAsyncSequence(t, NewHasher(), s, 13)
	if got != want {
		t.Errorf("native async root %x != sync root %x", got, want)
	}

	DisableAsyncHashing()
	if ref := runAsyncSequence(t, NewHasherWithHashFn(hashtree.HashByteSlice), s, 13); ref != want {
		t.Errorf("native root %x != fast root %x", want, ref)
	}
}

// TestAsyncMidStreamEnable toggles the process-wide switch on in the middle
// of a progressive computation: the synchronous rounds leave pair-compacted
// nodes at depths the pre-collapse pass cannot mix with cap-depth nodes, so
// its shape guard must reject the run and the root must still match.
func TestAsyncMidStreamEnable(t *testing.T) {
	defer DisableAsyncHashing()

	const n = 400000
	run := func(enableAt int) [32]byte {
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		hh.SetAsyncHashing(true)
		rng := rand.New(rand.NewSource(77))
		chunk := make([]byte, 32)

		idx := hh.StartTree(sszutils.TreeTypeProgressive)
		for i := 0; i < n; i++ {
			rng.Read(chunk)
			hh.Append(chunk)
			if (i+1)%64 == 0 {
				hh.Collapse()
			}
			if i == enableAt {
				EnableAsyncHashing(4)
			}
		}
		hh.MerkleizeProgressiveWithMixin(idx, n)
		root, err := hh.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		hh.Reset()
		return root
	}

	DisableAsyncHashing()
	want := run(-1)
	DisableAsyncHashing()
	got := run(200000)
	if got != want {
		t.Errorf("mid-stream enable root %x != sync root %x", got, want)
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

// TestAsyncHashDrainsTailHole carves an exactly cap-sized deferred run into a
// background job via Collapse, leaving the job's hole as the buffer tail, and
// verifies Hash() waits for the reduction instead of returning unfilled hole
// bytes. The final root must match a fully synchronous run of the same
// sequence.
func TestAsyncHashDrainsTailHole(t *testing.T) {
	defer DisableAsyncHashing()

	const elemChunks = 8
	elems := lazyFlushChunks / elemChunks

	chunks := make([]byte, lazyFlushChunks*32)
	rand.New(rand.NewSource(7)).Read(chunks)

	// Reference: the job reduces the raw run pairwise to a single node, which
	// equals the plain binary merkleization of the run.
	ref := NewHasherWithHashFn(hashtree.HashByteSlice)
	refIdx := ref.Index()
	ref.Append(chunks)
	ref.Merkleize(refIdx)
	wantHash, err := ref.HashRoot()
	if err != nil {
		t.Fatalf("reference HashRoot: %v", err)
	}

	run := func(async bool) [32]byte {
		hh := NewHasherWithHashFn(hashtree.HashByteSlice)
		hh.SetAsyncHashing(true)

		listIdx := hh.StartTree(sszutils.TreeTypeBinary)
		for off := 0; off < len(chunks); off += elemChunks * 32 {
			ci := hh.StartTree(sszutils.TreeTypeNone)
			hh.Append(chunks[off : off+elemChunks*32])
			hh.Merkleize(ci)
		}
		hh.Collapse()

		// Only the async run carves the whole run into a job whose hole is
		// the tail; the sync control flushes to element roots instead and
		// its Hash() value differs legitimately.
		var got [32]byte
		copy(got[:], hh.Hash())
		if async && got != wantHash {
			t.Errorf("Hash() over tail hole: got %x, want %x", got, wantHash)
		}

		hh.MerkleizeWithMixin(listIdx, uint64(elems), 1<<40)
		root, err := hh.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		return root
	}

	DisableAsyncHashing()
	want := run(false)
	EnableAsyncHashing(4)
	got := run(true)
	if got != want {
		t.Errorf("async root %x != sync root %x", got, want)
	}
}

// TestAsyncHashReturnsDeferredScopeRoot runs the per-element root capture
// pattern (Merkleize a child scope, read its root back via Hash()) with async
// hashing enabled. The per-element flush keeps the pending run narrow, so
// background jobs never overlap the tail and Hash() must not stall on or
// corrupt the captured roots.
func TestAsyncHashReturnsDeferredScopeRoot(t *testing.T) {
	defer DisableAsyncHashing()
	EnableAsyncHashing(4)

	const elemChunks = 8
	const elems = 600

	expElemRoots := make([][32]byte, elems)
	for i := range expElemRoots {
		eh := NewHasherWithHashFn(hashtree.HashByteSlice)
		idx := eh.Index()
		hashScopeElem(eh, elemChunks, i)
		eh.Merkleize(idx)
		root, err := eh.HashRoot()
		if err != nil {
			t.Fatalf("element %d HashRoot: %v", i, err)
		}
		expElemRoots[i] = root
	}

	rh := NewHasherWithHashFn(hashtree.HashByteSlice)
	refIdx := rh.Index()
	for i := range expElemRoots {
		rh.PutBytes(expElemRoots[i][:])
	}
	rh.MerkleizeWithMixin(refIdx, elems, 1024)
	expListRoot, err := rh.HashRoot()
	if err != nil {
		t.Fatalf("reference HashRoot: %v", err)
	}

	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	hh.SetAsyncHashing(true)

	listIdx := hh.StartTree(sszutils.TreeTypeBinary)
	for i := 0; i < elems; i++ {
		idx := hh.StartTree(sszutils.TreeTypeNone)
		hashScopeElem(hh, elemChunks, i)
		hh.Merkleize(idx)

		var got [32]byte
		copy(got[:], hh.Hash())
		if got != expElemRoots[i] {
			t.Fatalf("element %d root via Hash(): got %x, want %x", i, got, expElemRoots[i])
		}

		if (i+1)%256 == 0 {
			hh.Collapse()
		}
	}

	hh.MerkleizeWithMixin(listIdx, elems, 1024)
	root, err := hh.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}
	if root != expListRoot {
		t.Errorf("list root: got %x, want %x", root, expListRoot)
	}
}

// TestAsyncHashSkipsDrainBelowTail verifies Hash() leaves background
// reductions outstanding when their holes lie below the tail: after an async
// flush carves a hole, one more deferred element is appended and captured via
// Hash(), which must reduce only that element and keep the job pending.
func TestAsyncHashSkipsDrainBelowTail(t *testing.T) {
	defer DisableAsyncHashing()
	EnableAsyncHashing(4)

	const elemChunks = 8
	batchElems := lazyFlushChunks / elemChunks

	hh := NewHasherWithHashFn(hashtree.HashByteSlice)
	hh.SetAsyncHashing(true)

	listIdx := hh.StartTree(sszutils.TreeTypeBinary)
	for i := 0; i < batchElems; i++ {
		ci := hh.StartTree(sszutils.TreeTypeNone)
		hashScopeElem(hh, elemChunks, i)
		hh.Merkleize(ci)
	}
	hh.Collapse() // carves the run into a background job
	if hh.jobCount == 0 {
		t.Fatal("expected an outstanding background job after Collapse")
	}

	eh := NewHasherWithHashFn(hashtree.HashByteSlice)
	ei := eh.Index()
	hashScopeElem(eh, elemChunks, batchElems)
	eh.Merkleize(ei)
	wantElem, err := eh.HashRoot()
	if err != nil {
		t.Fatalf("element HashRoot: %v", err)
	}

	ci := hh.StartTree(sszutils.TreeTypeNone)
	hashScopeElem(hh, elemChunks, batchElems)
	hh.Merkleize(ci)

	var got [32]byte
	copy(got[:], hh.Hash())
	if got != wantElem {
		t.Errorf("element root via Hash(): got %x, want %x", got, wantElem)
	}
	if hh.jobCount == 0 {
		t.Error("Hash() drained a job whose hole lies below the tail")
	}

	hh.MerkleizeWithMixin(listIdx, uint64(batchElems)+1, 1<<40)
	if _, err := hh.HashRoot(); err != nil {
		t.Fatalf("HashRoot: %v", err)
	}
}
