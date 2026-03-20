// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package hasher

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// ---------------------------------------------------------------------------
// Known-answer tests
// ---------------------------------------------------------------------------

// TestKnownAnswerBinarySingleChunk verifies that a single 32-byte chunk
// merkleizes to itself (identity property of binary merkle trees).
func TestKnownAnswerBinarySingleChunk(t *testing.T) {
	var chunk [32]byte
	binary.LittleEndian.PutUint64(chunk[:], 42)

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	h.buf = append(h.buf, chunk[:]...)
	h.Merkleize(indx)
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	if root != chunk {
		t.Errorf("single chunk should merkleize to itself: got %x, want %x", root, chunk)
	}
}

// TestKnownAnswerBinaryTwoChunks verifies that two chunks merkleize to
// sha256(chunk0 || chunk1), computed independently with crypto/sha256.
func TestKnownAnswerBinaryTwoChunks(t *testing.T) {
	var chunk0, chunk1 [32]byte
	binary.LittleEndian.PutUint64(chunk0[:], 0)
	binary.LittleEndian.PutUint64(chunk1[:], 1)

	// Compute expected root independently
	pair := append(chunk0[:], chunk1[:]...)
	expected := sha256.Sum256(pair)

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	h.buf = append(h.buf, chunk0[:]...)
	h.buf = append(h.buf, chunk1[:]...)
	h.Merkleize(indx)
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	if root != expected {
		t.Errorf("two-chunk root mismatch: got %x, want %x", root, expected)
	}
}

// TestKnownAnswerBinaryFourChunks verifies that four chunks merkleize as
// sha256(sha256(c0||c1) || sha256(c2||c3)), computed independently.
func TestKnownAnswerBinaryFourChunks(t *testing.T) {
	chunks := makeChunks(4)

	// Compute expected root independently
	pair01 := append(chunks[0][:], chunks[1][:]...)
	pair23 := append(chunks[2][:], chunks[3][:]...)
	h01 := sha256.Sum256(pair01)
	h23 := sha256.Sum256(pair23)
	top := append(h01[:], h23[:]...)
	expected := sha256.Sum256(top)

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	for _, c := range chunks {
		h.buf = append(h.buf, c[:]...)
	}
	h.Merkleize(indx)
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	if root != expected {
		t.Errorf("four-chunk root mismatch: got %x, want %x", root, expected)
	}
}

// TestKnownAnswerBinaryThreeChunksWithZeroPad verifies that three chunks
// merkleize as sha256(sha256(c0||c1) || sha256(c2||zero)), matching the
// SSZ spec's zero-padding behavior.
func TestKnownAnswerBinaryThreeChunksWithZeroPad(t *testing.T) {
	chunks := makeChunks(3)
	var zero [32]byte

	pair01 := append(chunks[0][:], chunks[1][:]...)
	pair2z := append(chunks[2][:], zero[:]...)
	h01 := sha256.Sum256(pair01)
	h2z := sha256.Sum256(pair2z)
	top := append(h01[:], h2z[:]...)
	expected := sha256.Sum256(top)

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	for _, c := range chunks {
		h.buf = append(h.buf, c[:]...)
	}
	h.Merkleize(indx)
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	if root != expected {
		t.Errorf("three-chunk root mismatch: got %x, want %x", root, expected)
	}
}

// TestKnownAnswerIncrementalMatchesStandard verifies that the incremental
// path with explicit Collapse calls produces the same known-answer root as
// the standard path for 4 chunks.
func TestKnownAnswerIncrementalMatchesStandard(t *testing.T) {
	chunks := makeChunks(4)

	// Independent expected value
	pair01 := append(chunks[0][:], chunks[1][:]...)
	pair23 := append(chunks[2][:], chunks[3][:]...)
	h01 := sha256.Sum256(pair01)
	h23 := sha256.Sum256(pair23)
	top := append(h01[:], h23[:]...)
	expected := sha256.Sum256(top)

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.StartTree(sszutils.TreeTypeBinary)
	for _, c := range chunks {
		h.buf = append(h.buf, c[:]...)
		h.Collapse()
	}
	h.Merkleize(indx)
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	if root != expected {
		t.Errorf("incremental root mismatch: got %x, want %x", root, expected)
	}
}

// TestKnownAnswerWithMixin verifies the mixin formula:
// sha256(merkle_root || little_endian_64(num)).
func TestKnownAnswerWithMixin(t *testing.T) {
	chunks := makeChunks(2)
	num := uint64(2)
	limit := uint64(4)

	// Compute merkle root for 2 chunks with limit 4:
	// tree has depth 2 (ceil(log2(4)) = 2)
	// Level 0: [c0, c1, zero, zero]
	// Level 1: [sha256(c0||c1), sha256(zero||zero)]
	// Level 2: sha256(level1[0] || level1[1])
	pair01 := append(chunks[0][:], chunks[1][:]...)
	h01 := sha256.Sum256(pair01)
	var zero [32]byte
	pairZZ := append(zero[:], zero[:]...)
	hZZ := sha256.Sum256(pairZZ)
	topPair := append(h01[:], hZZ[:]...)
	merkleRoot := sha256.Sum256(topPair)

	// Mixin: sha256(merkle_root || num_le64 || zeros_24)
	var mixinBuf [64]byte
	copy(mixinBuf[:32], merkleRoot[:])
	binary.LittleEndian.PutUint64(mixinBuf[32:], num)
	expected := sha256.Sum256(mixinBuf[:])

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	for _, c := range chunks {
		h.buf = append(h.buf, c[:]...)
	}
	h.FillUpTo32()
	h.MerkleizeWithMixin(indx, num, limit)
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	if root != expected {
		t.Errorf("mixin root mismatch: got %x, want %x", root, expected)
	}
}

// ---------------------------------------------------------------------------
// Mixed tree types in nested scopes
// ---------------------------------------------------------------------------

// TestNestedProgressiveOuterBinaryInner tests a progressive outer scope with
// binary inner scopes (e.g., a progressive container containing binary fields).
func TestNestedProgressiveOuterBinaryInner(t *testing.T) {
	fieldsPerContainer := 4
	for _, N := range []int{5, 21, 85, 341, 500, 1365} {
		t.Run(fmt.Sprintf("N=%d", N), func(t *testing.T) {
			h := FastHasherPool.Get()
			defer FastHasherPool.Put(h)

			// Reference: non-incremental
			outerRef := h.Index()
			for c := range N {
				innerRef := h.Index()
				for f := range fieldsPerContainer {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*fieldsPerContainer+f))
					h.buf = append(h.buf, chunk[:]...)
				}
				h.Merkleize(innerRef)
			}
			h.FillUpTo32()
			h.MerkleizeProgressiveWithMixin(outerRef, uint64(N))
			refRoot, err := h.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			h.Reset()

			// Incremental: progressive outer, binary inner
			outerInc := h.StartTree(sszutils.TreeTypeProgressive)
			for c := range N {
				innerInc := h.StartTree(sszutils.TreeTypeBinary)
				for f := range fieldsPerContainer {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*fieldsPerContainer+f))
					h.buf = append(h.buf, chunk[:]...)
				}
				h.Merkleize(innerInc)
				if (c+1)%128 == 0 {
					h.Collapse()
				}
			}
			h.FillUpTo32()
			h.MerkleizeProgressiveWithMixin(outerInc, uint64(N))
			incRoot, err := h.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// TestNestedBinaryOuterProgressiveInner tests a binary outer scope with
// progressive inner scopes.
func TestNestedBinaryOuterProgressiveInner(t *testing.T) {
	fieldsPerContainer := 5
	for _, N := range []int{10, 100, 256, 257, 500, 1000} {
		t.Run(fmt.Sprintf("N=%d", N), func(t *testing.T) {
			h := FastHasherPool.Get()
			defer FastHasherPool.Put(h)

			// Reference: non-incremental
			outerRef := h.Index()
			for c := range N {
				innerRef := h.Index()
				for f := range fieldsPerContainer {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*fieldsPerContainer+f))
					h.buf = append(h.buf, chunk[:]...)
				}
				h.FillUpTo32()
				h.MerkleizeProgressive(innerRef)
			}
			h.Merkleize(outerRef)
			refRoot, err := h.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			h.Reset()

			// Incremental: binary outer, progressive inner
			outerInc := h.StartTree(sszutils.TreeTypeBinary)
			for c := range N {
				innerInc := h.StartTree(sszutils.TreeTypeProgressive)
				for f := range fieldsPerContainer {
					var chunk [32]byte
					binary.LittleEndian.PutUint64(chunk[:], uint64(c*fieldsPerContainer+f))
					h.buf = append(h.buf, chunk[:]...)
				}
				h.FillUpTo32()
				h.MerkleizeProgressive(innerInc)
				if (c+1)%128 == 0 {
					h.Collapse()
				}
			}
			h.Merkleize(outerInc)
			incRoot, err := h.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Layer stack depth beyond inline buffer
// ---------------------------------------------------------------------------

// TestDeepLayerStack pushes more than 16 layers (exceeding the inline
// layerBuf) to verify dynamic layer stack growth.
func TestDeepLayerStack(t *testing.T) {
	depth := 20 // exceeds layerBuf[16]

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Reference: compute with Index (non-incremental layers)
	indices := make([]int, depth)
	for d := range depth {
		indices[d] = h.Index()
		var chunk [32]byte
		binary.LittleEndian.PutUint64(chunk[:], uint64(d))
		h.buf = append(h.buf, chunk[:]...)
	}
	// Add a final leaf
	var leaf [32]byte
	binary.LittleEndian.PutUint64(leaf[:], uint64(depth))
	h.buf = append(h.buf, leaf[:]...)
	// Close all layers from innermost to outermost
	for d := depth - 1; d >= 0; d-- {
		h.Merkleize(indices[d])
	}
	refRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	h.Reset()

	// Incremental: use StartTree for all layers
	for d := range depth {
		indices[d] = h.StartTree(sszutils.TreeTypeBinary)
		var chunk [32]byte
		binary.LittleEndian.PutUint64(chunk[:], uint64(d))
		h.buf = append(h.buf, chunk[:]...)
	}
	h.buf = append(h.buf, leaf[:]...)
	for d := depth - 1; d >= 0; d-- {
		h.Merkleize(indices[d])
	}
	incRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("deep stack mismatch (depth=%d): ref=%x inc=%x", depth, refRoot, incRoot)
	}
}

// ---------------------------------------------------------------------------
// Progressive Merkleize (without mixin) + Collapse
// ---------------------------------------------------------------------------

// TestProgressiveWithoutMixinCollapse verifies progressive merkleization
// (no mixin) with Collapse at various intervals.
func TestProgressiveWithoutMixinCollapse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		total    int
		interval int
	}{
		{"5items_every1", 5, 1},
		{"21items_every1", 21, 1},
		{"85items_every1", 85, 1},
		{"256items_every128", 256, 128},
		{"341items_every128", 341, 128},
		{"1000items_every128", 1000, 128},
		{"1365items_every128", 1365, 128},
		{"5000items_every128", 5000, 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := makeChunks(tc.total)

			// Reference: non-incremental
			hRef := FastHasherPool.Get()
			defer FastHasherPool.Put(hRef)

			refIdx := hRef.StartTree(sszutils.TreeTypeNone)
			for _, c := range chunks {
				hRef.buf = append(hRef.buf, c[:]...)
			}
			hRef.FillUpTo32()
			hRef.MerkleizeProgressive(refIdx)
			refRoot, err := hRef.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
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
			hRef.MerkleizeProgressive(incIdx)
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

// ---------------------------------------------------------------------------
// PutRootVector / PutUint64Array inside incremental scope
// ---------------------------------------------------------------------------

// TestPutRootVectorInsideIncremental verifies that PutRootVector (which uses
// internalMerkleize, bypassing the layer stack) works correctly within an
// active incremental layer.
func TestPutRootVectorInsideIncremental(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	roots := make([][]byte, 4)
	for i := range roots {
		roots[i] = make([]byte, 32)
		binary.LittleEndian.PutUint64(roots[i], uint64(i+100))
	}

	// Reference
	refIdx := h.Index()
	h.PutUint64(1)
	if err := h.PutRootVector(roots, 8); err != nil {
		t.Fatalf("PutRootVector: %v", err)
	}
	h.PutUint64(2)
	h.Merkleize(refIdx)
	refRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	h.Reset()

	// Incremental
	incIdx := h.StartTree(sszutils.TreeTypeBinary)
	h.PutUint64(1)
	h.Collapse()
	err = h.PutRootVector(roots, 8)
	if err != nil {
		t.Fatalf("PutRootVector: %v", err)
	}
	h.Collapse()
	h.PutUint64(2)
	h.Merkleize(incIdx)
	incRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("PutRootVector in incremental: ref=%x inc=%x", refRoot, incRoot)
	}
}

// TestPutUint64ArrayInsideIncremental verifies that PutUint64Array works
// correctly within an active incremental layer.
func TestPutUint64ArrayInsideIncremental(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	values := []uint64{10, 20, 30, 40, 50}

	// Reference
	refIdx := h.Index()
	h.PutUint64(1)
	h.PutUint64Array(values, 16)
	h.PutUint64(2)
	h.Merkleize(refIdx)
	refRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	h.Reset()

	// Incremental
	incIdx := h.StartTree(sszutils.TreeTypeBinary)
	h.PutUint64(1)
	h.Collapse()
	h.PutUint64Array(values, 16)
	h.Collapse()
	h.PutUint64(2)
	h.Merkleize(incIdx)
	incRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("PutUint64Array in incremental: ref=%x inc=%x", refRoot, incRoot)
	}
}

// ---------------------------------------------------------------------------
// Hasher reuse correctness via pool
// ---------------------------------------------------------------------------

// TestPoolReuseCorrectness verifies that a hasher returned to the pool and
// re-acquired produces correct results for a completely different input.
func TestPoolReuseCorrectness(t *testing.T) {
	// First use: hash 100 chunks
	h1 := FastHasherPool.Get()
	chunks100 := makeChunks(100)
	idx1 := h1.StartTree(sszutils.TreeTypeBinary)
	for i, c := range chunks100 {
		h1.buf = append(h1.buf, c[:]...)
		if (i+1)%64 == 0 {
			h1.Collapse()
		}
	}
	h1.Merkleize(idx1)
	root1, err := h1.HashRoot()
	if err != nil {
		t.Fatalf("first use: %v", err)
	}
	FastHasherPool.Put(h1)

	// Second use: hash 50 different chunks
	h2 := FastHasherPool.Get()
	chunks50 := make([][32]byte, 50)
	for i := range chunks50 {
		binary.LittleEndian.PutUint64(chunks50[i][:], uint64(i+1000))
	}
	idx2 := h2.StartTree(sszutils.TreeTypeBinary)
	for i, c := range chunks50 {
		h2.buf = append(h2.buf, c[:]...)
		if (i+1)%64 == 0 {
			h2.Collapse()
		}
	}
	h2.Merkleize(idx2)
	root2, err := h2.HashRoot()
	if err != nil {
		t.Fatalf("second use: %v", err)
	}
	FastHasherPool.Put(h2)

	// Verify second result against a fresh hasher
	hFresh := FastHasherPool.Get()
	defer FastHasherPool.Put(hFresh)
	idxFresh := hFresh.Index()
	for _, c := range chunks50 {
		hFresh.buf = append(hFresh.buf, c[:]...)
	}
	hFresh.Merkleize(idxFresh)
	freshRoot, err := hFresh.HashRoot()
	if err != nil {
		t.Fatalf("fresh: %v", err)
	}

	if root2 != freshRoot {
		t.Errorf("reused hasher mismatch: reused=%x fresh=%x", root2, freshRoot)
	}

	// Also verify first result hasn't been corrupted
	if root1 == root2 {
		t.Error("different inputs should produce different roots")
	}
}

// ---------------------------------------------------------------------------
// NativeHashWrapper vs FastHasherPool cross-validation
// ---------------------------------------------------------------------------

// TestNativeVsFastHasher verifies that the native SHA256 hasher and the
// fast (hashtree) hasher produce identical results for various tree sizes.
func TestNativeVsFastHasher(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 7, 8, 15, 16, 100, 255, 256, 1000} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			chunks := makeChunks(n)

			// Native SHA256
			hNative := NewHasher()
			nativeIdx := hNative.Index()
			for _, c := range chunks {
				hNative.buf = append(hNative.buf, c[:]...)
			}
			hNative.Merkleize(nativeIdx)
			nativeRoot, err := hNative.HashRoot()
			if err != nil {
				t.Fatalf("native: %v", err)
			}

			// Fast (hashtree)
			hFast := FastHasherPool.Get()
			defer FastHasherPool.Put(hFast)
			fastIdx := hFast.Index()
			for _, c := range chunks {
				hFast.buf = append(hFast.buf, c[:]...)
			}
			hFast.Merkleize(fastIdx)
			fastRoot, err := hFast.HashRoot()
			if err != nil {
				t.Fatalf("fast: %v", err)
			}

			if nativeRoot != fastRoot {
				t.Errorf("native vs fast: native=%x fast=%x", nativeRoot, fastRoot)
			}
		})
	}
}

// TestNativeVsFastHasherProgressive verifies native vs fast agreement for
// progressive merkleization.
func TestNativeVsFastHasherProgressive(t *testing.T) {
	for _, n := range []int{1, 5, 21, 85, 100, 341, 500} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			chunks := makeChunks(n)

			// Native
			hNative := NewHasher()
			nIdx := hNative.Index()
			for _, c := range chunks {
				hNative.buf = append(hNative.buf, c[:]...)
			}
			hNative.FillUpTo32()
			hNative.MerkleizeProgressiveWithMixin(nIdx, uint64(n))
			nativeRoot, err := hNative.HashRoot()
			if err != nil {
				t.Fatalf("native: %v", err)
			}

			// Fast
			hFast := FastHasherPool.Get()
			defer FastHasherPool.Put(hFast)
			fIdx := hFast.Index()
			for _, c := range chunks {
				hFast.buf = append(hFast.buf, c[:]...)
			}
			hFast.FillUpTo32()
			hFast.MerkleizeProgressiveWithMixin(fIdx, uint64(n))
			fastRoot, err := hFast.HashRoot()
			if err != nil {
				t.Fatalf("fast: %v", err)
			}

			if nativeRoot != fastRoot {
				t.Errorf("native vs fast progressive: native=%x fast=%x",
					nativeRoot, fastRoot)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Memory efficiency: incremental uses less peak buffer
// ---------------------------------------------------------------------------

// TestIncrementalReducesPeakBuffer verifies that incremental hashing with
// frequent Collapse calls uses a smaller peak buffer than non-incremental.
func TestIncrementalReducesPeakBuffer(t *testing.T) {
	total := 100000

	// Non-incremental: measure peak buffer size
	hRef := FastHasherPool.Get()
	refIdx := hRef.StartTree(sszutils.TreeTypeNone)
	for i := range total {
		var chunk [32]byte
		binary.LittleEndian.PutUint64(chunk[:], uint64(i))
		hRef.buf = append(hRef.buf, chunk[:]...)
	}
	peakNonIncremental := len(hRef.buf)
	hRef.Merkleize(refIdx)
	refRoot, err := hRef.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	FastHasherPool.Put(hRef)

	// Incremental: measure peak buffer size with Collapse every 256
	hInc := FastHasherPool.Get()
	incIdx := hInc.StartTree(sszutils.TreeTypeBinary)
	peakIncremental := 0
	for i := range total {
		var chunk [32]byte
		binary.LittleEndian.PutUint64(chunk[:], uint64(i))
		hInc.buf = append(hInc.buf, chunk[:]...)
		if (i+1)%256 == 0 {
			hInc.Collapse()
		}
		if len(hInc.buf) > peakIncremental {
			peakIncremental = len(hInc.buf)
		}
	}
	hInc.Merkleize(incIdx)
	incRoot, err := hInc.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}
	FastHasherPool.Put(hInc)

	// Verify correctness
	if refRoot != incRoot {
		t.Fatalf("root mismatch: ref=%x inc=%x", refRoot, incRoot)
	}

	// Verify memory savings: incremental should use significantly less
	// Non-incremental: 100000 * 32 = 3.2MB
	// Incremental: should stay around 256*32 = 8KB between collapses
	if peakIncremental >= peakNonIncremental/2 {
		t.Errorf(
			"incremental should use much less peak buffer: "+
				"non-inc=%d bytes, inc=%d bytes",
			peakNonIncremental, peakIncremental,
		)
	}

	t.Logf("peak buffer: non-incremental=%d, incremental=%d (%.1fx reduction)",
		peakNonIncremental, peakIncremental,
		float64(peakNonIncremental)/float64(peakIncremental))
}

// ---------------------------------------------------------------------------
// Cascading multi-depth collapse
// ---------------------------------------------------------------------------

// TestCascadingMultiDepthCollapse triggers a cascade where collapsing depth-0
// produces enough depth-1 chunks to trigger depth-1 collapse, which produces
// enough depth-2 chunks, etc. This requires enough items accumulated since
// the last collapse.
func TestCascadingMultiDepthCollapse(t *testing.T) {
	// incrementalBatchSize = 256. After 256 * 256 = 65536 items without
	// collapse, depth-0 collapse produces 32768 depth-1 items, which
	// cascades to depth-1 collapse producing 16384 depth-2 items, etc.
	// This test adds items in a single batch then collapses.
	total := 65536

	chunks := makeChunks(total)

	// Reference
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

	// Incremental: add all items, then single Collapse
	incIdx := hRef.StartTree(sszutils.TreeTypeBinary)
	for _, c := range chunks {
		hRef.buf = append(hRef.buf, c[:]...)
	}
	hRef.Collapse() // single collapse triggers cascade
	hRef.Merkleize(incIdx)
	incRoot, err := hRef.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("cascade mismatch: ref=%x inc=%x", refRoot, incRoot)
	}
}

// ---------------------------------------------------------------------------
// Progressive with active fields at scale with incremental
// ---------------------------------------------------------------------------

// TestProgressiveActiveFieldsLargeIncremental verifies
// MerkleizeProgressiveWithActiveFields with incremental collapse and many items.
func TestProgressiveActiveFieldsLargeIncremental(t *testing.T) {
	for _, total := range []int{21, 85, 341, 1000} {
		t.Run(fmt.Sprintf("total=%d", total), func(t *testing.T) {
			chunks := makeChunks(total)

			// Build active fields bitvector (all fields active)
			activeBytes := (total + 7) / 8
			activeFields := make([]byte, activeBytes)
			for i := range total {
				activeFields[i/8] |= 1 << uint(i%8)
			}

			h := FastHasherPool.Get()
			defer FastHasherPool.Put(h)

			// Reference: non-incremental
			refIdx := h.StartTree(sszutils.TreeTypeNone)
			for _, c := range chunks {
				h.buf = append(h.buf, c[:]...)
			}
			h.FillUpTo32()
			h.MerkleizeProgressiveWithActiveFields(refIdx, activeFields)
			refRoot, err := h.HashRoot()
			if err != nil {
				t.Fatalf("ref: %v", err)
			}
			h.Reset()

			// Incremental with Collapse
			incIdx := h.StartTree(sszutils.TreeTypeProgressive)
			for i, c := range chunks {
				h.buf = append(h.buf, c[:]...)
				if (i+1)%128 == 0 {
					h.Collapse()
				}
			}
			h.FillUpTo32()
			h.MerkleizeProgressiveWithActiveFields(incIdx, activeFields)
			incRoot, err := h.HashRoot()
			if err != nil {
				t.Fatalf("inc: %v", err)
			}

			if refRoot != incRoot {
				t.Errorf("mismatch: ref=%x inc=%x", refRoot, incRoot)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Collapse on TreeTypeNone is a no-op
// ---------------------------------------------------------------------------

// TestCollapseNoopOnNoneLayer verifies that Collapse is a no-op when the
// current layer is TreeTypeNone, leaving the buffer unchanged.
func TestCollapseNoopOnNoneLayer(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	_ = h.StartTree(sszutils.TreeTypeNone)
	for i := range 1000 {
		var chunk [32]byte
		binary.LittleEndian.PutUint64(chunk[:], uint64(i))
		h.buf = append(h.buf, chunk[:]...)
		h.Collapse() // should be no-op
	}

	// Buffer should still contain all 1000 chunks untouched
	if len(h.buf) != 1000*32 {
		t.Errorf("Collapse on None layer modified buffer: got %d, want %d",
			len(h.buf), 1000*32)
	}

	// Verify each chunk is intact
	for i := range 1000 {
		offset := i * 32
		val := binary.LittleEndian.Uint64(h.buf[offset:])
		if val != uint64(i) {
			t.Fatalf("chunk %d corrupted: got %d, want %d", i, val, i)
		}
	}
}

// ---------------------------------------------------------------------------
// PutBitlist / PutProgressiveBitlist inside incremental scope
// ---------------------------------------------------------------------------

// TestPutBitlistInsideIncremental verifies PutBitlist works correctly within
// an incremental scope, simulating a container with a bitlist field.
func TestPutBitlistInsideIncremental(t *testing.T) {
	bitlist := []byte{0b11010101, 0b10101010, 0b00000001}
	maxSize := uint64(24)

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Reference: non-incremental
	refIdx := h.Index()
	h.PutUint64(42)
	h.PutBitlist(bitlist, maxSize)
	h.PutUint64(99)
	h.Merkleize(refIdx)
	refRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	h.Reset()

	// Incremental
	incIdx := h.StartTree(sszutils.TreeTypeBinary)
	h.PutUint64(42)
	h.Collapse()
	h.PutBitlist(bitlist, maxSize)
	h.Collapse()
	h.PutUint64(99)
	h.Merkleize(incIdx)
	incRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("PutBitlist in incremental: ref=%x inc=%x", refRoot, incRoot)
	}
}

// TestPutProgressiveBitlistInsideIncremental verifies PutProgressiveBitlist
// works correctly within an incremental scope.
func TestPutProgressiveBitlistInsideIncremental(t *testing.T) {
	bitlist := []byte{0b11010101, 0b10101010, 0b00000001}

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Reference: non-incremental
	refIdx := h.Index()
	h.PutUint64(42)
	h.PutProgressiveBitlist(bitlist)
	h.PutUint64(99)
	h.Merkleize(refIdx)
	refRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	h.Reset()

	// Incremental
	incIdx := h.StartTree(sszutils.TreeTypeBinary)
	h.PutUint64(42)
	h.Collapse()
	h.PutProgressiveBitlist(bitlist)
	h.Collapse()
	h.PutUint64(99)
	h.Merkleize(incIdx)
	incRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("PutProgressiveBitlist in incremental: ref=%x inc=%x", refRoot, incRoot)
	}
}

// ---------------------------------------------------------------------------
// Exact progressive level boundaries
// ---------------------------------------------------------------------------

// TestProgressiveLevelBoundaryExact verifies correctness at the exact
// boundary where each progressive level completes (1, 1+4, 1+4+16, ...).
func TestProgressiveLevelBoundaryExact(t *testing.T) {
	boundaries := []int{1, 5, 21, 85, 341, 1365, 5461}

	for _, total := range boundaries {
		for _, delta := range []int{-1, 0, 1} {
			n := total + delta
			if n <= 0 {
				continue
			}
			t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
				chunks := makeChunks(n)

				// Reference
				hRef := FastHasherPool.Get()
				defer FastHasherPool.Put(hRef)
				refIdx := hRef.StartTree(sszutils.TreeTypeNone)
				for _, c := range chunks {
					hRef.buf = append(hRef.buf, c[:]...)
				}
				hRef.FillUpTo32()
				hRef.MerkleizeProgressiveWithMixin(refIdx, uint64(n))
				refRoot, err := hRef.HashRoot()
				if err != nil {
					t.Fatalf("ref: %v", err)
				}
				hRef.Reset()

				// Incremental: collapse after every item
				incIdx := hRef.StartTree(sszutils.TreeTypeProgressive)
				for _, c := range chunks {
					hRef.buf = append(hRef.buf, c[:]...)
					hRef.Collapse()
				}
				hRef.FillUpTo32()
				hRef.MerkleizeProgressiveWithMixin(incIdx, uint64(n))
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
}

// ---------------------------------------------------------------------------
// PutBytes >32 inside incremental scope
// ---------------------------------------------------------------------------

// TestPutBytesLongInsideIncremental verifies that PutBytes with >32 bytes
// (triggering inline merkleization) works correctly inside an incremental scope.
func TestPutBytesLongInsideIncremental(t *testing.T) {
	longData := make([]byte, 100)
	for i := range longData {
		longData[i] = byte(i)
	}

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Reference
	refIdx := h.Index()
	h.PutUint64(1)
	h.PutBytes(longData)
	h.PutUint64(2)
	h.Merkleize(refIdx)
	refRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	h.Reset()

	// Incremental
	incIdx := h.StartTree(sszutils.TreeTypeBinary)
	h.PutUint64(1)
	h.Collapse()
	h.PutBytes(longData)
	h.Collapse()
	h.PutUint64(2)
	h.Merkleize(incIdx)
	incRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("PutBytes(>32) in incremental: ref=%x inc=%x", refRoot, incRoot)
	}
}

// ---------------------------------------------------------------------------
// Multiple sequential hashing operations on same hasher
// ---------------------------------------------------------------------------

// TestMultipleSequentialOperations verifies that a hasher can correctly
// perform multiple independent hashing operations sequentially (Reset between).
func TestMultipleSequentialOperations(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	results := make([][32]byte, 5)
	for op := range 5 {
		chunks := makeChunks(100 + op*50)
		idx := h.StartTree(sszutils.TreeTypeBinary)
		for i, c := range chunks {
			h.buf = append(h.buf, c[:]...)
			if (i+1)%128 == 0 {
				h.Collapse()
			}
		}
		h.Merkleize(idx)
		root, err := h.HashRoot()
		if err != nil {
			t.Fatalf("op %d: %v", op, err)
		}
		results[op] = root
		h.Reset()
	}

	// Verify each result independently
	for op := range 5 {
		hCheck := FastHasherPool.Get()
		chunks := makeChunks(100 + op*50)
		checkIdx := hCheck.Index()
		for _, c := range chunks {
			hCheck.buf = append(hCheck.buf, c[:]...)
		}
		hCheck.Merkleize(checkIdx)
		checkRoot, err := hCheck.HashRoot()
		if err != nil {
			t.Fatalf("check op %d: %v", op, err)
		}
		FastHasherPool.Put(hCheck)

		if results[op] != checkRoot {
			t.Errorf("op %d: sequential=%x independent=%x", op, results[op], checkRoot)
		}
	}
}

// ---------------------------------------------------------------------------
// ParseBitlistWithHasher non-Hasher fallback
// ---------------------------------------------------------------------------

// TestParseBitlistWithHasherFallback verifies the non-Hasher path of
// ParseBitlistWithHasher using a mock HashWalker.
func TestParseBitlistWithHasherFallback(t *testing.T) {
	mock := &mockHashWalker{tmp: make([]byte, 64)}
	bitlist := []byte{0b11010101, 0b00000001}

	result, size := ParseBitlistWithHasher(mock, bitlist)
	if size != 8 {
		t.Errorf("expected size 8, got %d", size)
	}
	if len(result) != 1 || result[0] != 0b11010101 {
		t.Errorf("unexpected bitlist: %v", result)
	}
}

// mockHashWalker implements just enough of HashWalker for ParseBitlistWithHasher.
type mockHashWalker struct {
	tmp []byte
}

func (m *mockHashWalker) WithTemp(fn func(tmp []byte) []byte) {
	m.tmp = fn(m.tmp)
}

func (m *mockHashWalker) Hash() []byte                                         { return nil }
func (m *mockHashWalker) AppendBool(_ bool)                                    {}
func (m *mockHashWalker) AppendUint8(_ uint8)                                  {}
func (m *mockHashWalker) AppendUint16(_ uint16)                                {}
func (m *mockHashWalker) AppendUint32(_ uint32)                                {}
func (m *mockHashWalker) AppendUint64(_ uint64)                                {}
func (m *mockHashWalker) AppendBytes32(_ []byte)                               {}
func (m *mockHashWalker) PutUint64Array(_ []uint64, _ ...uint64)               {}
func (m *mockHashWalker) PutUint64(_ uint64)                                   {}
func (m *mockHashWalker) PutUint32(_ uint32)                                   {}
func (m *mockHashWalker) PutUint16(_ uint16)                                   {}
func (m *mockHashWalker) PutUint8(_ uint8)                                     {}
func (m *mockHashWalker) PutBitlist(_ []byte, _ uint64)                        {}
func (m *mockHashWalker) PutProgressiveBitlist(_ []byte)                       {}
func (m *mockHashWalker) PutBool(_ bool)                                       {}
func (m *mockHashWalker) PutBytes(_ []byte)                                    {}
func (m *mockHashWalker) FillUpTo32()                                          {}
func (m *mockHashWalker) Append(_ []byte)                                      {}
func (m *mockHashWalker) Index() int                                           { return 0 }
func (m *mockHashWalker) CurrentIndex() int                                    { return 0 }
func (m *mockHashWalker) StartTree(_ sszutils.TreeType) int                    { return 0 }
func (m *mockHashWalker) Collapse()                                            {}
func (m *mockHashWalker) Merkleize(_ int)                                      {}
func (m *mockHashWalker) MerkleizeWithMixin(_ int, _, _ uint64)                {}
func (m *mockHashWalker) MerkleizeProgressive(_ int)                           {}
func (m *mockHashWalker) MerkleizeProgressiveWithMixin(_ int, _ uint64)        {}
func (m *mockHashWalker) MerkleizeProgressiveWithActiveFields(_ int, _ []byte) {}
func (m *mockHashWalker) HashRoot() ([32]byte, error)                          { return [32]byte{}, nil }
func (m *mockHashWalker) PutRootVector(_ [][]byte, _ ...uint64) error          { return nil }

// ---------------------------------------------------------------------------
// ParseBitlist: various sentinel positions
// ---------------------------------------------------------------------------

// TestParseBitlistVariousSentinels verifies ParseBitlist with sentinel bits
// at different positions within the final byte.
func TestParseBitlistVariousSentinels(t *testing.T) {
	tests := []struct {
		name         string
		input        []byte
		expectedSize uint64
		expectedData byte
	}{
		{"sentinel_bit0", []byte{0b00000001}, 0, 0},               // sentinel at bit 0, 0 data bits
		{"sentinel_bit1", []byte{0b00000011}, 1, 1},               // sentinel at bit 1, 1 data bit (set)
		{"sentinel_bit7", []byte{0b10000000}, 7, 0},               // sentinel at bit 7, 7 data bits (all clear)
		{"sentinel_bit7_data", []byte{0b11111111}, 7, 0b01111111}, // all bits set before sentinel
		{"two_bytes", []byte{0xFF, 0b00000010}, 9, 0xFF},          // sentinel at bit 1 of byte 2, byte 0 is all data
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := make([]byte, 0, 10)
			result, size := ParseBitlist(dst, tt.input)
			if size != tt.expectedSize {
				t.Errorf("size: got %d, want %d", size, tt.expectedSize)
			}
			if tt.expectedSize > 0 && len(result) > 0 && result[0] != tt.expectedData {
				t.Errorf("data: got 0b%08b, want 0b%08b", result[0], tt.expectedData)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Incremental with PutBool fields
// ---------------------------------------------------------------------------

// TestIncrementalWithPutBoolFields simulates a realistic container with
// mixed PutBool, PutUint64, PutBytes fields in an incremental scope.
func TestIncrementalWithPutBoolFields(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	buildContainer := func(startTree bool) [32]byte {
		var idx int
		if startTree {
			idx = h.StartTree(sszutils.TreeTypeBinary)
		} else {
			idx = h.Index()
		}
		h.PutBool(true)
		h.PutUint64(12345)
		h.PutBool(false)
		h.PutUint32(42)
		h.PutBytes([]byte{1, 2, 3, 4, 5})
		h.PutBool(true)
		h.PutUint16(999)
		h.PutUint8(0xFF)
		if startTree {
			h.Collapse()
		}
		h.Merkleize(idx)
		root, err := h.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		h.Reset()
		return root
	}

	refRoot := buildContainer(false)
	incRoot := buildContainer(true)

	if refRoot != incRoot {
		t.Errorf("mixed fields: ref=%x inc=%x", refRoot, incRoot)
	}
}

// ---------------------------------------------------------------------------
// Deterministic progressive root for 5 items (levels 0,1 boundary)
// ---------------------------------------------------------------------------

// TestKnownAnswerProgressiveFiveChunks verifies progressive merkleization
// of 5 chunks against an independently computed value. Progressive splits as:
// level 0 (base=1): chunk[0] → left
// level 2 (base=4): chunk[1..4] → right, recursively:
//
//	level 2 (base=4): chunk[1..4] → left
//	level 4 (base=16): empty → right = zero_node(0)
func TestKnownAnswerProgressiveFiveChunks(t *testing.T) {
	chunks := makeChunks(5)

	// Compute independently:
	// Level 0: left = merkle(chunk[0], base=1) = chunk[0]
	//          right = progressive(chunk[1..4], depth=2)
	//
	// progressive(chunk[1..4], depth=2):
	//   left = merkle(chunk[1..4], base=4) = sha256(sha256(c1||c2)||sha256(c3||c4))
	//   right = zero_node(0) = [0..0]32
	//   return sha256(left || right)
	//
	// Top: sha256(chunk[0] || progressive_right)

	// merkle(c1..c4, base=4)
	p12 := sha256.Sum256(append(chunks[1][:], chunks[2][:]...))
	p34 := sha256.Sum256(append(chunks[3][:], chunks[4][:]...))
	merkle1234 := sha256.Sum256(append(p12[:], p34[:]...))

	var zeroNode [32]byte
	progressiveRight := sha256.Sum256(append(merkle1234[:], zeroNode[:]...))

	expected := sha256.Sum256(append(chunks[0][:], progressiveRight[:]...))

	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	indx := h.Index()
	for _, c := range chunks {
		h.buf = append(h.buf, c[:]...)
	}
	h.FillUpTo32()
	h.MerkleizeProgressive(indx)
	root, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	if root != expected {
		t.Errorf("progressive 5 chunks: got %x, want %x", root, expected)
	}
}

// ---------------------------------------------------------------------------
// Interleaved incremental and non-incremental scopes
// ---------------------------------------------------------------------------

// TestInterleavedIncrementalAndLegacy verifies correct behavior when mixing
// StartTree (incremental) and Index (legacy) calls in the same tree.
func TestInterleavedIncrementalAndLegacy(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Reference: all legacy
	outerRef := h.Index()
	for c := range 300 {
		innerRef := h.Index()
		for f := range 4 {
			var chunk [32]byte
			binary.LittleEndian.PutUint64(chunk[:], uint64(c*4+f))
			h.buf = append(h.buf, chunk[:]...)
		}
		h.Merkleize(innerRef)
	}
	h.Merkleize(outerRef)
	refRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	h.Reset()

	// Mixed: outer incremental, alternating inner between Index and StartTree
	outerInc := h.StartTree(sszutils.TreeTypeBinary)
	for c := range 300 {
		var innerIdx int
		if c%2 == 0 {
			innerIdx = h.StartTree(sszutils.TreeTypeBinary)
		} else {
			innerIdx = h.Index()
		}
		for f := range 4 {
			var chunk [32]byte
			binary.LittleEndian.PutUint64(chunk[:], uint64(c*4+f))
			h.buf = append(h.buf, chunk[:]...)
		}
		h.Merkleize(innerIdx)
		if c%64 == 0 {
			h.Collapse()
		}
	}
	h.Merkleize(outerInc)
	incRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("inc: %v", err)
	}

	if refRoot != incRoot {
		t.Errorf("interleaved mismatch: ref=%x inc=%x", refRoot, incRoot)
	}
}

// ---------------------------------------------------------------------------
// PutRootVector empty / single root
// ---------------------------------------------------------------------------

// TestPutRootVectorEdgeCases tests PutRootVector with zero and one root.
func TestPutRootVectorEdgeCases(t *testing.T) {
	h := FastHasherPool.Get()
	defer FastHasherPool.Put(h)

	// Zero roots
	idx := h.Index()
	h.PutUint64(1)
	err := h.PutRootVector(nil)
	if err != nil {
		t.Fatalf("PutRootVector(nil): %v", err)
	}
	h.PutUint64(2)
	h.Merkleize(idx)
	_, err = h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}
	h.Reset()

	// Single root
	root := make([]byte, 32)
	binary.LittleEndian.PutUint64(root, 42)
	idx = h.Index()
	err = h.PutRootVector([][]byte{root})
	if err != nil {
		t.Fatalf("PutRootVector single: %v", err)
	}
	h.Merkleize(idx)
	got, err := h.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	// Single root should merkleize to itself
	if !bytes.Equal(got[:], root) {
		t.Errorf("single root: got %x, want %x", got, root)
	}
}
