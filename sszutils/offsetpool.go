// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"sync"
)

// offsetSlicePool manages reusable int slices to reduce allocations
type offsetSlicePool struct {
	pool sync.Pool
}

// defaultOffsetSlicePool is the default int slice pool instance
var defaultOffsetSlicePool = &offsetSlicePool{
	pool: sync.Pool{
		New: func() interface{} {
			slice := make([]uint32, 0, 32) // Start with capacity for 32 ints
			return &slice
		},
	},
}

// Get returns an int slice from the pool, consumer can grow it as needed
func (p *offsetSlicePool) Get() []uint32 {
	return (*p.pool.Get().(*[]uint32))[:0] // Reset length to 0
}

// Put returns an int slice to the pool
func (p *offsetSlicePool) Put(slice []uint32) {
	if cap(slice) > 0 {
		p.pool.Put(&slice)
	}
}

func GetOffsetSlice(size int) []uint32 {
	buf := defaultOffsetSlicePool.Get()
	return ExpandSlice(buf, size)
}

func PutOffsetSlice(slice []uint32) {
	defaultOffsetSlicePool.Put(slice)
}
