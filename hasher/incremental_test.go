// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package hasher

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// makeChunks generates n deterministic 32-byte chunks for testing.
func makeChunks(n int) [][32]byte {
	chunks := make([][32]byte, n)
	for i := range chunks {
		binary.LittleEndian.PutUint64(chunks[i][:], uint64(i))
	}
	return chunks
}

// referenceBinaryRoot computes the binary merkle root using the standard
// (non-incremental) path: Index() + Merkleize().
func referenceBinaryRoot(t *testing.T, chunks [][32]byte, limit uint64) [32]byte {
	t.Helper()
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	for i := range chunks {
		h.buf = append(h.buf, chunks[i][:]...)
	}
	h.FillUpTo32()
	if limit > 0 {
		h.MerkleizeWithMixin(indx, uint64(len(chunks)), limit)
	} else {
		h.Merkleize(indx)
	}
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("referenceBinaryRoot: HashRoot failed: %v", err)
	}
	return root
}

// referenceProgressiveRoot computes the progressive merkle root using the
// standard (non-incremental) recursive merkleizeProgressiveImpl path.
func referenceProgressiveRoot(t *testing.T, chunks [][32]byte, withMixin bool) [32]byte {
	t.Helper()
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	for i := range chunks {
		h.buf = append(h.buf, chunks[i][:]...)
	}
	h.FillUpTo32()
	if withMixin {
		h.MerkleizeProgressiveWithMixin(indx, uint64(len(chunks)))
	} else {
		h.MerkleizeProgressive(indx)
	}
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("referenceProgressiveRoot: HashRoot failed: %v", err)
	}
	return root
}

// noIncrementalBinaryRoot computes the binary merkle root using StartTree(None)
// which disables incremental hashing — used as a cross-check reference.
func noIncrementalBinaryRoot(t *testing.T, chunks [][32]byte, limit uint64) [32]byte {
	t.Helper()
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.StartTree(sszutils.TreeTypeNone)
	for i := range chunks {
		h.buf = append(h.buf, chunks[i][:]...)
	}
	h.FillUpTo32()
	if limit > 0 {
		h.MerkleizeWithMixin(indx, uint64(len(chunks)), limit)
	} else {
		h.Merkleize(indx)
	}
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("noIncrementalBinaryRoot: HashRoot failed: %v", err)
	}
	return root
}

// incrementalBinaryRoot computes the binary merkle root using the incremental
// path: StartTree(Binary) + per-chunk PutBytes + Merkleize/MerkleizeWithMixin.
func incrementalBinaryRoot(t *testing.T, chunks [][32]byte, limit uint64) [32]byte {
	t.Helper()
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.StartTree(sszutils.TreeTypeBinary)
	for i := range chunks {
		h.buf = append(h.buf, chunks[i][:]...)
	}
	h.FillUpTo32()
	if limit > 0 {
		h.MerkleizeWithMixin(indx, uint64(len(chunks)), limit)
	} else {
		h.Merkleize(indx)
	}
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("incrementalBinaryRoot: HashRoot failed: %v", err)
	}
	return root
}

// incrementalProgressiveRoot computes the progressive merkle root using the
// incremental path: StartTree(Progressive) + per-chunk PutBytes + MerkleizeProgressive*.
func incrementalProgressiveRoot(t *testing.T, chunks [][32]byte, withMixin bool) [32]byte {
	t.Helper()
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.StartTree(sszutils.TreeTypeProgressive)
	for i := range chunks {
		h.buf = append(h.buf, chunks[i][:]...)
	}
	h.FillUpTo32()
	if withMixin {
		h.MerkleizeProgressiveWithMixin(indx, uint64(len(chunks)))
	} else {
		h.MerkleizeProgressive(indx)
	}
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("incrementalProgressiveRoot: HashRoot failed: %v", err)
	}
	return root
}

func TestIncrementalBinarySmall(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 7, 8, 15, 16, 31, 32, 100, 255} {
		chunks := makeChunks(n)
		ref := referenceBinaryRoot(t, chunks, 0)
		noInc := noIncrementalBinaryRoot(t, chunks, 0)
		inc := incrementalBinaryRoot(t, chunks, 0)
		if ref != noInc {
			t.Fatalf("n=%d: noIncremental mismatch with reference: ref=%x noInc=%x", n, ref, noInc)
		}
		if ref != inc {
			t.Errorf("n=%d: incremental mismatch: ref=%x inc=%x", n, ref, inc)
		}
	}
}

func TestIncrementalBinaryLarge(t *testing.T) {
	// Large trees trigger incremental collapse
	for _, n := range []int{256, 257, 512, 1000, 1024, 4096, 10000, 100000} {
		chunks := makeChunks(n)
		ref := referenceBinaryRoot(t, chunks, 0)
		inc := incrementalBinaryRoot(t, chunks, 0)
		if ref != inc {
			t.Errorf("n=%d: binary mismatch: ref=%x inc=%x", n, ref, inc)
		}
	}
}

func TestIncrementalBinaryWithMixin(t *testing.T) {
	// Test with limit (MerkleizeWithMixin) — used for SSZ lists
	for _, tc := range []struct {
		n     int
		limit uint64
	}{
		{100, 1024},
		{256, 1024},
		{1000, 4096},
		{10000, 1099511627776}, // VALIDATOR_REGISTRY_LIMIT
		{100000, 1099511627776},
	} {
		chunks := makeChunks(tc.n)
		ref := referenceBinaryRoot(t, chunks, tc.limit)
		inc := incrementalBinaryRoot(t, chunks, tc.limit)
		if ref != inc {
			t.Errorf("n=%d limit=%d: binary+mixin mismatch: ref=%x inc=%x", tc.n, tc.limit, ref, inc)
		}
	}
}

func TestIncrementalProgressiveSmall(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5, 10, 16, 21, 50, 100, 255} {
		chunks := makeChunks(n)
		ref := referenceProgressiveRoot(t, chunks, false)
		inc := incrementalProgressiveRoot(t, chunks, false)
		if ref != inc {
			t.Errorf("n=%d: progressive mismatch: ref=%x inc=%x", n, ref, inc)
		}
	}
}

func TestIncrementalProgressiveLarge(t *testing.T) {
	for _, n := range []int{256, 257, 512, 1000, 1024, 4096, 10000} {
		chunks := makeChunks(n)
		ref := referenceProgressiveRoot(t, chunks, false)
		inc := incrementalProgressiveRoot(t, chunks, false)
		if ref != inc {
			t.Errorf("n=%d: progressive mismatch: ref=%x inc=%x", n, ref, inc)
		}
	}
}

func TestIncrementalProgressiveWithMixin(t *testing.T) {
	for _, n := range []int{1, 5, 21, 100, 256, 1000, 10000} {
		chunks := makeChunks(n)
		ref := referenceProgressiveRoot(t, chunks, true)
		inc := incrementalProgressiveRoot(t, chunks, true)
		if ref != inc {
			t.Errorf("n=%d: progressive+mixin mismatch: ref=%x inc=%x", n, ref, inc)
		}
	}
}

func TestIncrementalBinaryNested(t *testing.T) {
	// Simulate a list of containers: outer StartTree for the list, inner
	// StartTree for each "container" (8 chunks each), then outer Merkleize.
	// This mimics the Validators list pattern.
	// Test various sizes including boundary cases around incrementalBatchSize.
	for _, N := range []int{10, 100, 200, 255, 256, 257, 300, 500, 512, 1000, 10000} {
		t.Run(fmt.Sprintf("N=%d", N), func(t *testing.T) {
			h := FastHasherPool.Get()
			defer FastHasherPool.Put(h)

			// Reference: compute without StartTree
			outerIndx := h.Index()
			for c := 0; c < N; c++ {
				innerIndx := h.Index()
				for f := 0; f < 8; f++ {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*8+f))
					h.buf = append(h.buf, chunk[:]...)
				}
				h.Merkleize(innerIndx)
			}
			h.Merkleize(outerIndx)
			refRoot, _ := h.HashRoot()
			h.Reset()

			// Incremental: compute with StartTree
			outerIndx = h.StartTree(sszutils.TreeTypeBinary)
			for c := 0; c < N; c++ {
				innerIndx := h.StartTree(sszutils.TreeTypeBinary)
				for f := 0; f < 8; f++ {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*8+f))
					h.buf = append(h.buf, chunk[:]...)
				}
				h.Merkleize(innerIndx)
			}
			h.Merkleize(outerIndx)
			incRoot, _ := h.HashRoot()

			if refRoot != incRoot {
				t.Errorf("ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestCollapseAtOddCounts verifies that calling Collapse() at various odd
// item counts produces correct results by comparing against a TreeTypeNone
// hasher that does no incremental hashing.
func TestCollapseAtOddCounts(t *testing.T) {
	// Test with various total sizes and Collapse() call intervals.
	// The intervals are deliberately odd/prime to stress remainder handling.
	for _, tc := range []struct {
		name     string
		total    int
		interval int // call Collapse() every this many items
	}{
		{"130items_every1", 130, 1},
		{"130items_every3", 130, 3},
		{"130items_every7", 130, 7},
		{"130items_every127", 130, 127},
		{"130items_every129", 130, 129},
		{"256items_every1", 256, 1},
		{"256items_every13", 256, 13},
		{"256items_every128", 256, 128},
		{"257items_every1", 257, 1},
		{"257items_every128", 257, 128},
		{"300items_every17", 300, 17},
		{"300items_every128", 300, 128},
		{"512items_every33", 512, 33},
		{"1000items_every1", 1000, 1},
		{"1000items_every64", 1000, 64},
		{"1000items_every128", 1000, 128},
		{"1000items_every255", 1000, 255},
		{"10000items_every128", 10000, 128},
		{"10000items_every333", 10000, 333},
		// Edge: exactly one batch
		{"256items_every256", 256, 256},
		// Edge: single item between collapses
		{"513items_every1", 513, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := makeChunks(tc.total)

			// Reference: TreeTypeNone (no incremental, no layers)
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)
			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for i, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse() // no-op for TreeTypeNone
				}
			}
			hRef.Merkleize(refIdx)
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("reference HashRoot failed: %v", err)
			}

			hRef.Reset()

			// Incremental: TreeTypeBinary with Collapse() at intervals
			incIdx := hRef.StartTree(sszutils.TreeTypeBinary)
			for i, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.Merkleize(incIdx)
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("incremental HashRoot failed: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestCollapseNestedAtOddCounts tests incremental collapse with nested scopes
// (simulating a list of containers) and Collapse() called at odd intervals.
func TestCollapseNestedAtOddCounts(t *testing.T) {
	fieldsPerContainer := 8

	for _, tc := range []struct {
		name       string
		containers int
		interval   int // call Collapse() every this many containers
	}{
		{"100containers_every3", 100, 3},
		{"256containers_every7", 256, 7},
		{"257containers_every1", 257, 1},
		{"257containers_every128", 257, 128},
		{"300containers_every13", 300, 13},
		{"500containers_every128", 500, 128},
		{"1000containers_every33", 1000, 33},
		{"1000containers_every128", 1000, 128},
		{"10000containers_every128", 10000, 128},
		{"10000containers_every255", 10000, 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Reference: TreeTypeNone for outer, Index() for inner
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			outerRef := hRef.StartTree(sszutils.TreeTypeNone)
			for c := 0; c < tc.containers; c++ {
				innerRef := hRef.Index()
				for f := 0; f < fieldsPerContainer; f++ {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*fieldsPerContainer+f))
					hRef.buf = append(hRef.buf, chunk[:]...)
				}
				hRef.Merkleize(innerRef)
			}
			hRef.Merkleize(outerRef)
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("reference HashRoot failed: %v", err)
			}
			hRef.Reset()

			// Incremental: TreeTypeBinary for outer, Index() for inner,
			// Collapse() every N containers
			outerInc := hRef.StartTree(sszutils.TreeTypeBinary)
			for c := 0; c < tc.containers; c++ {
				innerInc := hRef.Index()
				for f := 0; f < fieldsPerContainer; f++ {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*fieldsPerContainer+f))
					hRef.buf = append(hRef.buf, chunk[:]...)
				}
				hRef.Merkleize(innerInc)
				if (c+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.Merkleize(outerInc)
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("incremental HashRoot failed: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestCollapseWithMixin tests incremental collapse with MerkleizeWithMixin
// (SSZ lists with limit) and Collapse() at odd intervals.
func TestCollapseWithMixin(t *testing.T) {
	for _, tc := range []struct {
		name     string
		total    int
		limit    uint64
		interval int
	}{
		{"100items_limit1024_every7", 100, 1024, 7},
		{"256items_limit1024_every128", 256, 1024, 128},
		{"257items_limit4096_every1", 257, 4096, 1},
		{"1000items_limit1099511627776_every128", 1000, 1099511627776, 128},
		{"10000items_limit1099511627776_every128", 10000, 1099511627776, 128},
		{"10000items_limit1099511627776_every255", 10000, 1099511627776, 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := makeChunks(tc.total)

			// Reference: TreeTypeNone
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for _, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeWithMixin(refIdx, uint64(tc.total), tc.limit)
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("reference HashRoot failed: %v", err)
			}
			hRef.Reset()

			// Incremental with Collapse()
			incIdx := hRef.StartTree(sszutils.TreeTypeBinary)
			for i, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.FillUpTo32()
			hRef.MerkleizeWithMixin(incIdx, uint64(tc.total), tc.limit)
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("incremental HashRoot failed: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestProgressiveLargeLevel tests progressive merkleization where the active
// subtree at a large progressive level (base_size >= 256) triggers binary
// collapse within the subtree before the progressive batch completes.
// This verifies the fix for binary collapse within progressive subtrees.
func TestProgressiveLargeLevel(t *testing.T) {
	// Progressive base sizes: level 0=1, 1=4, 2=16, 3=64, 4=256, 5=1024
	// At level 4, the active subtree needs 256 leaves to complete.
	// At level 5, it needs 1024 leaves — binary collapse should trigger
	// during filling.
	//
	// Total chunks to reach level 5: 1 + 4 + 16 + 64 + 256 + some into level 5
	// = 341 to fill levels 0-4, then need 1024 for level 5 = 1365 total.
	// Let's test with totals that span multiple large progressive levels.
	for _, total := range []int{
		5,     // only levels 0,1 (1+4)
		21,    // levels 0,1,2 (1+4+16)
		85,    // levels 0,1,2,3 (1+4+16+64)
		341,   // levels 0-4 (1+4+16+64+256)
		342,   // 341 + 1 into level 5
		500,   // well into level 5 (needs 1024, has 159)
		600,   // more into level 5
		1365,  // levels 0-5 complete (1+4+16+64+256+1024)
		1366,  // 1 into level 6 (needs 4096)
		2000,  // deep into level 6
		5461,  // levels 0-6 complete (+ 4096)
		10000, // deep into level 7
	} {
		t.Run(fmt.Sprintf("total=%d", total), func(t *testing.T) {
			chunks := makeChunks(total)

			// Reference: TreeTypeNone (no incremental)
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for _, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(refIdx, uint64(total))
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref HashRoot: %v", err)
			}
			hRef.Reset()

			// Incremental: TreeTypeProgressive with Collapse every 128
			incIdx := hRef.StartTree(sszutils.TreeTypeProgressive)
			for i, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
				if (i+1)%128 == 0 {
					hRef.Collapse()
				}
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(incIdx, uint64(total))
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("inc HashRoot: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestProgressiveLeafCountAfterCollapse verifies that the progressive batch
// completion check correctly counts leaves after binary collapse has happened.
// Without the fix, collapsed depth-1 entries would be counted as 1 leaf each
// instead of 2, causing the progressive batch to never complete.
func TestProgressiveLeafCountAfterCollapse(t *testing.T) {
	// Level 4 base_size = 256. If we add 256 leaves with Collapse every 128,
	// the binary collapse produces 128 depth-1 entries (representing 256 leaves).
	// The buffer has 128 chunks but 256 leaves — the progressive batch should
	// complete. Without the leaf-count fix, it wouldn't (128 < 256).
	//
	// After levels 0-3 consume 1+4+16+64=85 leaves, level 4 needs 256.
	// Total = 85 + 256 = 341. Then level 5 needs 1024.
	// Test at exactly 341 (level 4 just completed) and 341+256=597 (well into level 5
	// where binary collapse is active within the level-5 subtree).
	for _, tc := range []struct {
		name     string
		total    int
		interval int
	}{
		// Level 4 exactly fills, binary collapse active during fill
		{"level4_exact_collapse128", 341, 128},
		{"level4_exact_collapse64", 341, 64},
		// Into level 5 with binary collapse
		{"into_level5_collapse128", 600, 128},
		{"into_level5_collapse64", 600, 64},
		// Level 5 exactly fills
		{"level5_exact_collapse128", 1365, 128},
		// Deep levels with aggressive collapse
		{"level6_collapse128", 3000, 128},
		{"level7_collapse128", 10000, 128},
		// Collapse every single item — maximum collapse pressure
		{"level5_collapse1", 1365, 1},
		{"level6_collapse1", 5461, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := makeChunks(tc.total)

			// Reference without incremental
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for _, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(refIdx, uint64(tc.total))
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref HashRoot: %v", err)
			}
			hRef.Reset()

			// Incremental with Collapse
			incIdx := hRef.StartTree(sszutils.TreeTypeProgressive)
			for i, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(incIdx, uint64(tc.total))
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("inc HashRoot: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestBinaryRemainderPreservation tests that the binary collapse correctly
// preserves remainder chunks when collapsing odd counts. Without the fix,
// remainders were lost (buffer truncated without moving them).
func TestBinaryRemainderPreservation(t *testing.T) {
	// Test cases designed to produce odd counts at each depth:
	// 257 = 256+1: depth-0 collapses 256→128, remainder 1
	// 513 = 512+1: two collapses, remainder at different levels
	// 130 = 128+2: even collapse + 2 remainder, but 130 rounds to 130 even pairs = 65 d1
	for _, tc := range []struct {
		name     string
		total    int
		interval int
	}{
		{"257_every128", 257, 128},
		{"257_every1", 257, 1},
		{"513_every128", 513, 128},
		{"513_every1", 513, 1},
		{"129_every1", 129, 1},
		{"130_every1", 130, 1},
		{"131_every1", 131, 1},
		{"255_every128", 255, 128},
		{"1025_every128", 1025, 128},
		{"10001_every128", 10001, 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := makeChunks(tc.total)

			// Reference: TreeTypeNone
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for _, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
			}
			hRef.Merkleize(refIdx)
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			hRef.Reset()

			// Incremental with Collapse at interval
			incIdx := hRef.StartTree(sszutils.TreeTypeBinary)
			for i, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.Merkleize(incIdx)
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestIncrementalBinaryVeryLarge tests binary incremental with 2M+ entries.
func TestIncrementalBinaryVeryLarge(t *testing.T) {
	for _, tc := range []struct {
		name     string
		total    int
		interval int
	}{
		{"2M_every128", 2_000_000, 128},
		{"2M_every255", 2_000_000, 255},
		{"3M_every128", 3_000_000, 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Reference: TreeTypeNone
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeWithMixin(refIdx, uint64(tc.total), uint64(tc.total))
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			hRef.Reset()

			// Incremental
			incIdx := hRef.StartTree(sszutils.TreeTypeBinary)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.FillUpTo32()
			hRef.MerkleizeWithMixin(incIdx, uint64(tc.total), uint64(tc.total))
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestProgressiveVeryLarge tests progressive incremental with 2M+ entries.
func TestProgressiveVeryLarge(t *testing.T) {
	for _, tc := range []struct {
		name     string
		total    int
		interval int
	}{
		{"2M_every128", 2_000_000, 128},
		{"2M_every255", 2_000_000, 255},
		{"3M_every128", 3_000_000, 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			// Reference
			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(refIdx, uint64(tc.total))
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			hRef.Reset()

			// Incremental
			incIdx := hRef.StartTree(sszutils.TreeTypeProgressive)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(incIdx, uint64(tc.total))
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestIncrementalBinaryVeryLargeOdd tests binary incremental with 2M+ odd entries
// to exercise remainder handling at every depth level.
func TestIncrementalBinaryVeryLargeOdd(t *testing.T) {
	for _, tc := range []struct {
		name     string
		total    int
		interval int
	}{
		{"2000001_every128", 2_000_001, 128},
		{"2000001_every255", 2_000_001, 255},
		{"2000001_every1", 2_000_001, 1},
		{"2097153_every128", 2_097_153, 128}, // 2^21 + 1
		{"3000001_every128", 3_000_001, 128},
		{"2000003_every17", 2_000_003, 17}, // prime total, prime interval
	} {
		t.Run(tc.name, func(t *testing.T) {
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeWithMixin(refIdx, uint64(tc.total), uint64(tc.total))
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			hRef.Reset()

			incIdx := hRef.StartTree(sszutils.TreeTypeBinary)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.FillUpTo32()
			hRef.MerkleizeWithMixin(incIdx, uint64(tc.total), uint64(tc.total))
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestProgressiveVeryLargeOdd tests progressive incremental with odd totals
// at various progressive level boundaries.
func TestProgressiveVeryLargeOdd(t *testing.T) {
	for _, tc := range []struct {
		name     string
		total    int
		interval int
	}{
		{"2000001_every128", 2_000_001, 128},
		{"2000001_every255", 2_000_001, 255},
		{"2000003_every17", 2_000_003, 17},
		{"3000001_every128", 3_000_001, 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(refIdx, uint64(tc.total))
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			hRef.Reset()

			incIdx := hRef.StartTree(sszutils.TreeTypeProgressive)
			for i := 0; i < tc.total; i++ {
				var chunk [32]byte
				binary.LittleEndian.PutUint64(chunk[:], uint64(i))
				hRef.buf = append(hRef.buf, chunk[:]...)
				if (i+1)%tc.interval == 0 {
					hRef.Collapse()
				}
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressiveWithMixin(incIdx, uint64(tc.total))
			incRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestCollapseProgressiveEmpty tests progressive merkleization with zero items.
func TestCollapseProgressiveEmpty(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Reference
	refIdx := h.StartTree(sszutils.TreeTypeNone)
	h.FillUpTo32()
	h.MerkleizeProgressiveWithMixin(refIdx, 0)
	refRoot, _ := h.HashRoot()
	h.Reset()

	// Incremental
	incIdx := h.StartTree(sszutils.TreeTypeProgressive)
	h.Collapse()
	h.FillUpTo32()
	h.MerkleizeProgressiveWithMixin(incIdx, 0)
	incRoot, _ := h.HashRoot()

	if refRoot != incRoot {
		t.Errorf("empty progressive mismatch: ref=%x inc=%x", refRoot, incRoot)
	}
}

// TestCollapseProgressiveWithActiveFields tests the incremental path of
// MerkleizeProgressiveWithActiveFields.
func TestCollapseProgressiveWithActiveFields(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	activeFields := []byte{0x07} // 3 active fields

	// Reference
	refIdx := h.StartTree(sszutils.TreeTypeNone)
	h.PutUint64(1)
	h.PutUint64(2)
	h.PutUint64(3)
	h.MerkleizeProgressiveWithActiveFields(refIdx, activeFields)
	refRoot, _ := h.HashRoot()
	h.Reset()

	// Incremental with Collapse
	incIdx := h.StartTree(sszutils.TreeTypeProgressive)
	h.PutUint64(1)
	h.Collapse()
	h.PutUint64(2)
	h.Collapse()
	h.PutUint64(3)
	h.Collapse()
	h.MerkleizeProgressiveWithActiveFields(incIdx, activeFields)
	incRoot, _ := h.HashRoot()

	if refRoot != incRoot {
		t.Errorf("progressive active fields mismatch: ref=%x inc=%x", refRoot, incRoot)
	}
}

// TestCollapseProgressiveLayerCollapsedRemainder tests collapseProgressiveLayer
// when the partial remainder has binary collapse state.
func TestCollapseProgressiveLayerCollapsedRemainder(t *testing.T) {
	// Fill 85 + 300 items = 385. Level 0-3 consume 85, level 4 (256) consumes 256,
	// leaving 44 items. With Collapse every 128, binary collapse runs on those 44+
	// items at level 4 time.
	total := 385
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Reference
	refIdx := h.StartTree(sszutils.TreeTypeNone)
	for i := 0; i < total; i++ {
		var chunk [32]byte
		binary.LittleEndian.PutUint64(chunk[:], uint64(i))
		h.buf = append(h.buf, chunk[:]...)
	}
	h.FillUpTo32()
	h.MerkleizeProgressiveWithMixin(refIdx, uint64(total))
	refRoot, _ := h.HashRoot()
	h.Reset()

	// Incremental
	incIdx := h.StartTree(sszutils.TreeTypeProgressive)
	for i := 0; i < total; i++ {
		var chunk [32]byte
		binary.LittleEndian.PutUint64(chunk[:], uint64(i))
		h.buf = append(h.buf, chunk[:]...)
		if (i+1)%128 == 0 {
			h.Collapse()
		}
	}
	h.FillUpTo32()
	h.MerkleizeProgressiveWithMixin(incIdx, uint64(total))
	incRoot, _ := h.HashRoot()

	if refRoot != incRoot {
		t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
	}
}

// TestNativeHasherOddChunks tests the NativeHashWrapper with odd chunk counts.
func TestNativeHasherOddChunks(t *testing.T) {
	h := NewHasher() // uses native SHA256, not hashtree
	defer h.Reset()

	indx := h.Index()
	// 3 chunks — odd, exercises the layerLen%2==1 branch in NativeHashWrapper
	h.PutUint64(1)
	h.PutUint64(2)
	h.PutUint64(3)
	h.Merkleize(indx)

	_, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot failed: %v", err)
	}
}

// TestMerkleizeProgressiveNonIncremental tests MerkleizeProgressive without
// StartTree (using Index instead) to cover the non-incremental fallback path.
func TestMerkleizeProgressiveNonIncremental(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index() // NOT StartTree — no layer pushed
	h.PutUint64(1)
	h.PutUint64(2)
	h.PutUint64(3)
	h.FillUpTo32()
	h.MerkleizeProgressive(indx)

	_, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot failed: %v", err)
	}
}
