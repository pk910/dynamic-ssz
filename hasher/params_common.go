// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

const (
	// asyncBufSize is the size of every job input buffer. Jobs are capped
	// at lazyFlushChunks, so all buffers are interchangeable and no other
	// size ever needs allocating: wider regions reduce as a sequence of
	// cap-sized jobs whose remainder carries over to the next round.
	asyncBufSize = lazyFlushChunks * 32

	// asyncRingInline is the job-ring size served by the hasher's inline
	// backing array; larger rings (workers > 8) allocate once.
	asyncRingInline = 16
)
