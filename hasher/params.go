// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

//go:build !dynssz_smallhash

package hasher

// Reduction thresholds. The formal model of the hasher
// (formal/lean/SszCert/Hasher) is parametric in them; the dynssz_smallhash
// build tag (params_small.go) shrinks them so that tests reach every
// batching and async path with small inputs.
const (
	// incrementalBatchSize is the number of same-depth chunks to accumulate before
	// collapsing them into the next depth level. Must be a power of 2.
	// 256 chunks = 128 pairs per hash call, fits in L1 cache, good cgo amortization.
	incrementalBatchSize = 256

	// asyncMinChunks is the minimum region width (in 32-byte chunks) worth
	// handing to a background reduction; smaller regions reduce inline.
	asyncMinChunks = 8192

	// lazyFlushChunks is the pending-run width at which Collapse flushes
	// deferred subtrees when async hashing is enabled. Runs this wide reduce
	// in a single background job. With async hashing disabled, Collapse
	// flushes on every call and the pending run stays at the cadence the
	// caller collapses with.
	lazyFlushChunks = 32768

	// asyncActiveChunks is the active-region width at which Collapse runs
	// progressive group finalization when async hashing is enabled.
	// Deferring finalization gives element-root jobs time to complete in the
	// background, so the drain at finalization rarely waits.
	asyncActiveChunks = 131072

	// asyncCapDepth is the subtree depth a full cap-sized reduction
	// produces: a lazyFlushChunks-wide run of leaves becomes one node at
	// this depth.
	asyncCapDepth = 15

	// asyncPrecollapseLevel is the lowest progressive level whose groups
	// (4^level chunks) are multiples of lazyFlushChunks, so cap-aligned
	// pre-collapse nodes never straddle a group boundary.
	asyncPrecollapseLevel = 8
)
