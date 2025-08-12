package dynssz

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
			slice := make([]int, 0, 32) // Start with capacity for 32 ints
			return &slice
		},
	},
}

// Get returns an int slice from the pool, consumer can grow it as needed
func (p *offsetSlicePool) Get() []int {
	return (*p.pool.Get().(*[]int))[:0] // Reset length to 0
}

// Put returns an int slice to the pool
func (p *offsetSlicePool) Put(slice []int) {
	if cap(slice) > 0 {
		p.pool.Put(&slice)
	}
}
