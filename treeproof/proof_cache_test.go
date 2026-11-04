// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"slices"
	"testing"
)

// Rejected multiproofs over large, distinct index sets keep the required
// indices cache within its budget, a set over the budget is not stored, and
// small sets are still served from the cache.
func TestRequiredIndicesCacheBudget(t *testing.T) {
	root := make([]byte, 32)
	verify := func(indices []int) {
		leaves := make([][]byte, len(indices))
		for i := range leaves {
			leaves[i] = make([]byte, 32)
		}
		if ok, err := VerifyMultiproof(root, nil, leaves, indices); ok || err == nil {
			t.Fatalf("forged proof accepted (ok=%v, err=%v)", ok, err)
		}
	}
	sparse := func(n, offset int) []int {
		indices := make([]int, n)
		for i := range indices {
			indices[i] = 1<<28 + i*7 + offset
		}
		return indices
	}
	held := func() int {
		requiredIndicesCache.mu.RLock()
		defer requiredIndicesCache.mu.RUnlock()
		total := 0
		for i := range requiredIndicesCache.entries {
			total += len(requiredIndicesCache.entries[i].indices) + len(requiredIndicesCache.entries[i].required)
		}
		if total != requiredIndicesCache.ints {
			t.Fatalf("cache tracks %d ints but holds %d", requiredIndicesCache.ints, total)
		}
		return total
	}

	for k := range 2 * requiredIndicesCacheSize {
		verify(sparse(16384, k))
		if n := held(); n > requiredIndicesCacheMaxInts {
			t.Fatalf("cache holds %d ints, over the budget of %d", n, requiredIndicesCacheMaxInts)
		}
	}

	big := sparse(1<<17, 0)
	verify(big)
	for i := range requiredIndicesCache.entries {
		if slices.Equal(requiredIndicesCache.entries[i].indices, big) {
			t.Fatal("set over the budget was cached")
		}
	}

	small := []int{1<<16 + 3, 1<<16 + 1000}
	if first, second := getRequiredIndices(small), getRequiredIndices(small); &first[0] != &second[0] {
		t.Error("small set was not served from the cache")
	}
}
