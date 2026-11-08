// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

//go:build dynssz_smallhash

package hasher

// Test-only thresholds (build tag dynssz_smallhash): the same relationships
// as params.go at a scale where every batching and async path is reached by
// small inputs. Roots are independent of the thresholds.
const (
	incrementalBatchSize  = 4
	asyncMinChunks        = 8
	lazyFlushChunks       = 16
	asyncActiveChunks     = 64
	asyncCapDepth         = 4
	asyncPrecollapseLevel = 2
)
