// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package treeproof

import (
	"sync"

	"github.com/pk910/dynamic-ssz/hasher"
)

// Option configures an entry point of this package.
type Option func(*options)

type options struct {
	alg *hasher.Algorithm
}

// WithAlgorithm names the hash Algorithm of a tree: the one a Wrapper builds
// its tree for, or the one a proof was taken under. Without it the entry
// points of this package work with hasher.SHA256.
func WithAlgorithm(alg *hasher.Algorithm) Option {
	return func(o *options) {
		o.alg = alg
	}
}

// algorithmOf returns the Algorithm the given options name, or nil when they
// name none.
func algorithmOf(opts []Option) *hasher.Algorithm {
	if len(opts) == 0 {
		return nil
	}
	o := &options{}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o.alg
}

// treeHash is the hash of a tree: its Algorithm, and whether that computes
// another hash than SHA256.
type treeHash struct {
	alg     *hasher.Algorithm
	foreign bool
}

// treeHashOf returns the hash of the trees of the given Algorithm.
func treeHashOf(alg *hasher.Algorithm) treeHash {
	return treeHash{alg: alg, foreign: !alg.Compatible(hasher.SHA256)}
}

// treeHashFor returns the hash the given options name.
func treeHashFor(opts []Option) treeHash {
	if alg := algorithmOf(opts); alg != nil {
		return treeHashOf(alg)
	}
	return treeHash{alg: hasher.SHA256}
}

// padding holds the zero-padding nodes of one hash, by depth. Each node
// links to the one a level below, which is how a walk descends into padding
// without knowing the hash.
type padding struct {
	alg   *hasher.Algorithm
	once  sync.Once
	nodes [65]*Node
}

// sha256Padding is the padding of every SHA256 tree.
var sha256Padding = &padding{alg: hasher.SHA256}

// paddingOf returns the padding for the trees of the given hash. SHA256 trees
// share theirs; a tree of another hash gets its own.
func paddingOf(h treeHash) *padding {
	if !h.foreign {
		return sha256Padding
	}
	return &padding{alg: h.alg}
}

// emptyNode returns the zero-padding node of the given depth. The nodes are
// built when the first one is needed.
func (p *padding) emptyNode(depth int) *Node {
	p.once.Do(func() {
		block := make([]Node, len(p.nodes))
		zero := p.alg.ZeroHashes()
		var below *Node
		for i := range block {
			node := &block[i]
			*node = Node{left: below, value: zero[i], hasValue: true, isEmpty: true}
			p.nodes[i] = node
			below = node
		}
	})

	return p.nodes[depth]
}
