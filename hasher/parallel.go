// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"sync"
	"sync/atomic"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// Compile-time check to ensure Hasher implements ParallelWalker.
var _ sszutils.ParallelWalker = (*Hasher)(nil)

const (
	// parallelSpan is the number of elements one goroutine walks and hashes
	// to a subtree node. It is a power of two, so a span of single-leaf
	// elements is a complete subtree of the scope.
	parallelSpan = 1024

	// parallelSpanDepth is the depth of the node a span reduces to.
	parallelSpanDepth = 10
)

// subHashers holds the hashers the goroutines of a parallel walk hash their
// spans with. They take the hash function of the hasher they work for.
var subHashers = sync.Pool{
	New: func() any {
		return NewHasherWithHashFn(nil)
	},
}

// WalkParallel hashes the n elements of the innermost open scope, each of
// which leaves exactly one chunk in it, by calling walk for ranges of them.
// With async hashing on and a scope wide enough, the ranges are spans walked
// on several goroutines, each into a hasher of its own, and the scope takes
// the span roots as completed subtree nodes; what is left behind the last
// whole span is walked into this hasher. Otherwise walk is called once for
// all elements. walk must only read the value and must hash the elements
// [from, to) into the walker it is given.
func (h *Hasher) WalkParallel(n int, walk func(hh sszutils.HashWalker, from, to int) error) error {
	st := h.asyncShared()
	if st == nil || n < 2*parallelSpan || h.layerCount < 0 {
		return walk(h, 0, n)
	}
	layer := &h.layers[h.layerCount]
	if !layer.incremental || layer.progressive || layer.legacy || layer.packed ||
		layer.collapsed || layer.pendCount > 0 || len(h.buf) != layer.bufIdx {
		return walk(h, 0, n)
	}

	spans := n / parallelSpan
	base := len(h.buf)
	h.buf = sszutils.AppendZeroPadding(h.buf, spans*32)
	// Nothing is appended to the buffer while the spans are walked, so the
	// goroutines write their roots straight into it.
	roots := h.buf[base : base+spans*32]

	var (
		next     atomic.Int64
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		panicked any
	)
	fail := func(err error, r any) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		if panicked == nil {
			panicked = r
		}
		mu.Unlock()
		next.Store(int64(spans))
	}
	run := func() {
		sub := subHashers.Get().(*Hasher)
		sub.hash = h.hash
		defer func() {
			if r := recover(); r != nil {
				fail(nil, r)
			}
			sub.Reset()
			sub.hash = nil
			subHashers.Put(sub)
		}()
		for {
			k := int(next.Add(1)) - 1
			if k >= spans {
				return
			}
			idx := sub.StartTree(sszutils.TreeTypeBinary)
			err := walk(sub, k*parallelSpan, (k+1)*parallelSpan)
			sub.Merkleize(idx)
			root, hashErr := sub.HashRoot()
			if err == nil {
				err = hashErr
			}
			if err != nil {
				fail(err, nil)
				return
			}
			copy(roots[k*32:k*32+32], root[:])
			sub.Reset()
		}
	}

	helpers := min(cap(st.sem), spans-1)
	wg.Add(helpers)
	for range helpers {
		go func() {
			defer wg.Done()
			run()
		}()
	}
	run()
	wg.Wait()

	if panicked != nil {
		panic(panicked)
	}
	if firstErr != nil {
		return firstErr
	}

	layer = &h.layers[h.layerCount]
	layer.collapsed = true
	layer.counts = [maxTreeDepth]uint32{}
	layer.counts[parallelSpanDepth] = uint32(spans)
	layer.maxDepth = parallelSpanDepth

	return walk(h, spans*parallelSpan, n)
}
