// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// ProofHashWalker is the walker NewWrapper returns: a HashWalker whose walk
// additionally yields a proof-capable node tree. Without options it serves
// the full tree; with WithProofCapture it serves a pruned proof tree.
type ProofHashWalker interface {
	sszutils.HashWalker

	// Node returns the root of the tree the walk produced. See
	// treeBuilder.Node for the full-tree contract; a proof capture returns the
	// pruned proof tree instead, fully hashed by construction, and panics
	// when the capture diverged from the hash tree root.
	Node() *Node
}

// WrapperOption configures NewWrapper.
type WrapperOption func(*wrapperConfig)

type wrapperConfig struct {
	sched   *ProofSchedule
	hh      *hasher.Hasher
	hashFn  hasher.HashFn
	workers int
}

// WithProofCapture makes NewWrapper return a proof capture instead of a
// full-tree wrapper: the hashing walk runs through hh while only the
// subtrees the schedule's proof paths descend into keep their structure;
// everything off-path collapses into single value nodes carrying the subtree
// hash. hh must be freshly reset; hashFn serves the capture's sibling-range
// reductions and must be safe for concurrent use, nil selects the fast
// hashing backend.
func WithProofCapture(sched *ProofSchedule, hh *hasher.Hasher, hashFn hasher.HashFn) WrapperOption {
	return func(cfg *wrapperConfig) {
		cfg.sched = sched
		cfg.hh = hh
		cfg.hashFn = hashFn
	}
}

// WithProofWorkers hashes the capture's rebuilt subtrees (Put-style
// replays, retained shapes) on the given number of pipeline workers; see
// WithAsyncHashing. Only meaningful together with WithProofCapture.
func WithProofWorkers(workers int) WrapperOption {
	return func(cfg *wrapperConfig) {
		cfg.workers = workers
	}
}

// NewWrapper creates the walker for a tree-producing hashing walk. Without
// options it materializes the full Merkle tree (concrete type *treeBuilder);
// with WithProofCapture it captures a pruned proof tree instead. Run the
// hashing walk against the returned walker, then read the tree via Node.
func NewWrapper(opts ...WrapperOption) ProofHashWalker {
	cfg := wrapperConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.sched != nil {
		return newProofCapture(cfg.sched, cfg.hh, cfg.hashFn, cfg.workers)
	}
	return newTreeBuilder()
}

// Wrapper is the full-tree walker's former exported name.
//
// Deprecated: the concrete walker types are implementation details; use
// NewWrapper and the ProofHashWalker interface instead.
type Wrapper = treeBuilder
