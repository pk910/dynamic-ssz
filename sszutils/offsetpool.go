// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "sync"

// offsetSlicePool manages reusable int slices to reduce allocations
type offsetSlicePool struct {
	mu    sync.Mutex
	slots [][]uint32
}

// defaultOffsetSlicePool is the default int slice pool instance
var defaultOffsetSlicePool = &offsetSlicePool{
	slots: make([][]uint32, 0, 64),
}

// Get returns an int slice from the pool, consumer can grow it as needed
func (p *offsetSlicePool) Get() []uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	if n := len(p.slots); n > 0 {
		// Reuse the last slice we got back instead of allocating a fresh buffer.
		slice := p.slots[n-1]
		p.slots = p.slots[:n-1]
		return slice[:0]
	}

	// Start small and let ExpandSlice grow it only when needed.
	return make([]uint32, 0, 32)
}

// Put returns an int slice to the pool
func (p *offsetSlicePool) Put(slice []uint32) {
	// Skip empty or very large buffers. They are not worth keeping around.
	if cap(slice) == 0 || cap(slice) > 4096 {
		return
	}

	slice = slice[:0]
	p.mu.Lock()
	defer p.mu.Unlock()

	// Keep the pool bounded so reuse stays cheap and predictable.
	if len(p.slots) < cap(p.slots) {
		p.slots = append(p.slots, slice)
	}
}

// GetOffsetSlice returns a uint32 slice of the given size from a shared pool,
// suitable for use as an SSZ offset buffer. The caller must return it via
// PutOffsetSlice when done.
func GetOffsetSlice(size int) []uint32 {
	buf := defaultOffsetSlicePool.Get()
	return ExpandSlice(buf, size)
}

// PutOffsetSlice returns a uint32 offset slice to the shared pool for reuse.
func PutOffsetSlice(slice []uint32) {
	defaultOffsetSlicePool.Put(slice)
}
