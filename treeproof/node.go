// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package treeproof

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/pk910/dynamic-ssz/hasher"
)

// Option configures an entry point of this package.
type Option struct {
	alg *hasher.Algorithm
}

// WithAlgorithm names the hash Algorithm of a tree: the one a tree is built
// for, or the one a proof was taken under. Without it the entry points of
// this package work with hasher.SHA256.
func WithAlgorithm(alg *hasher.Algorithm) Option {
	return Option{alg: alg}
}

// algorithmOf returns the Algorithm the given options name.
func algorithmOf(opts []Option) *hasher.Algorithm {
	for i := len(opts) - 1; i >= 0; i-- {
		if opts[i].alg != nil {
			return opts[i].alg
		}
	}

	return hasher.SHA256
}

// Node is a node of a Merkle tree constructed from SSZ data, together with
// the hash Algorithm of that tree.
//
// A node is a leaf holding 32 bytes of data, a branch whose value is the hash
// of its children, or a zero-padding node standing for a zero subtree. The
// tree structure follows SSZ merkleization rules and supports both binary and
// progressive tree layouts.
//
// A branch belongs to the Algorithm its tree was built for, and every method
// that hashes or reads a root uses that Algorithm. A leaf carries only data
// and may be placed into a tree of any Algorithm.
type Node struct {
	n   *node
	alg *hasher.Algorithm
}

// zeroLeaf is the node of a Node made without a constructor: a leaf of zeros.
var zeroLeaf = node{hasValue: true}

// node returns the tree node the handle refers to.
func (n *Node) node() *node {
	if n.n == nil {
		return &zeroLeaf
	}
	return n.n
}

// Algorithm returns the Algorithm the node's tree is hashed with.
func (n *Node) Algorithm() *hasher.Algorithm {
	if n.alg == nil {
		return algorithmOf(nil)
	}
	return n.alg
}

// Same reports whether both handles refer to the same node of a tree. Get,
// Left and Right return a new handle on every call, so handles are compared
// with Same, not as pointers. Zero-padding nodes of one depth are one node,
// wherever they stand in a tree.
func (n *Node) Same(other *Node) bool {
	return n != nil && other != nil && n.node() == other.node()
}

// Show displays the tree structure in a human-readable format for debugging.
//
// This method prints the complete tree hierarchy starting from this node,
// showing generalized indices, hash values, and the tree structure. It's
// particularly useful for understanding how SSZ data maps to tree nodes
// and for debugging proof generation.
//
// Parameters:
//   - maxDepth: Maximum depth to display (0 for unlimited depth)
//
// Output format:
//   - INDEX: The generalized index of each node
//   - HASH: 32-byte hash for branch nodes (computed from children)
//   - VALUE: 32-byte data for leaf nodes (actual SSZ field data)
//   - EMPTY: Indicates zero-padding nodes with their depth level
//   - LEFT/RIGHT: Tree structure showing child relationships
//
// Example output:
//
//	--- Show node ---
//	INDEX: 1
//	HASH: a1b2c3d4...
//	LEFT:
//	    INDEX: 2
//	    VALUE: e5f6g7h8...
//	RIGHT:
//	    INDEX: 3
//	    HASH: i9j0k1l2...
//	    LEFT:
//	        INDEX: 6
//	        VALUE: m3n4o5p6...
//	    RIGHT:
//	        INDEX: 7
//	        EMPTY: true (depth: 0)
func (n *Node) Show(maxDepth int) {
	fmt.Printf("--- Show node ---\n")
	h := hashingOf(n.Algorithm())
	if err := n.node().finalize(&h); err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	n.node().show(&h, 0, maxDepth, 1) // Start with index 1 (root)
}

// Get fetches a node with the given general index.
func (n *Node) Get(index int) (*Node, error) {
	if n == nil && index <= 1 {
		if index < 1 {
			return nil, errors.New("Node not found in tree")
		}
		return nil, nil
	}
	found, err := n.node().get(index)
	if err != nil {
		return nil, err
	}
	return &Node{n: found, alg: n.Algorithm()}, nil
}

// Hash returns the hash of the subtree with the given Node as its root.
// If root has no children, it returns root's value (not its hash).
// Hash finalizes the subtree, so afterwards every branch node holds its
// cached hash and the subtree is read-only. It panics on an incomplete
// tree; Finalize reports that as an error instead.
// A copy is returned, so the caller is free to change it.
//
// It returns nil when the hash backend refuses a compression, since there is
// no root to return and nothing is cached: the subtree is left as it was, so a
// later call answers it once the backend does. Finalize reports the refusal.
func (n *Node) Hash() []byte {
	h := hashingOf(n.Algorithm())
	// A backend refusal has to stop here: there is no root to answer with.
	// The malformed-tree error travels on: nodeValue reports the incomplete
	// tree through its documented panic.
	if err := n.node().finalize(&h); err != nil && !errors.Is(err, errMalformedTree) {
		return nil
	}
	return bytes.Clone(nodeValue(n.node(), &h))
}

// FinalizeOption configures Finalize.
type FinalizeOption func(*finalizeConfig)

type finalizeConfig struct {
	fn hasher.HashFn
}

// Finalize computes and caches every node hash in the subtree, so afterwards
// the subtree is read-only and safe for concurrent Prove/ProveMulti/Hash/
// Value calls. Hash, Prove and ProveMulti finalize implicitly.
//
// Any error — a hashing backend failure or a malformed tree (a branch with
// a single nil child) — aborts finalization immediately and is returned.
// Every hash cached before the abort is valid, so a later Finalize resumes
// from it.
func (n *Node) Finalize(opts ...FinalizeOption) error {
	if n == nil {
		return nil
	}
	cfg := finalizeConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	h := hashingOf(n.Algorithm())
	if cfg.fn != nil {
		h.fn = cfg.fn
	}
	return n.node().finalize(&h)
}

// child wraps a child node, or returns nil where there is none.
func (n *Node) child(c *node) *Node {
	if c == nil {
		return nil
	}
	return &Node{n: c, alg: n.Algorithm()}
}

// Left returns the left child node, or nil if this is a leaf.
func (n *Node) Left() *Node {
	return n.child(n.node().left)
}

// Right returns the right child node, or nil if this is a leaf.
func (n *Node) Right() *Node {
	return n.child(n.node().right)
}

// IsLeaf returns true if this node has no children (is a leaf node).
func (n *Node) IsLeaf() bool {
	return n.node().left == nil && n.node().right == nil
}

// IsEmpty returns true if this node represents zero-padding: a position the
// tree holds no value for. A leaf holding an all-zero value is not empty.
func (n *Node) IsEmpty() bool {
	return n.node().isEmpty
}

// Value returns a copy of the node's 32-byte value — leaf data, a finalized
// branch hash or the zero subtree root of a zero-padding node — or nil for a
// branch that has not been hashed yet.
func (n *Node) Value() []byte {
	if n.n == nil {
		return nil
	}
	switch {
	case n.node().isEmpty && !n.node().hasValue:
		h := hashingOf(n.Algorithm())
		if h.err != nil {
			return nil
		}
		return bytes.Clone(h.value(n.node())[:])
	case n.node().hasValue:
		return bytes.Clone(n.node().value[:])
	default:
		return nil
	}
}

// Prove returns a list of sibling values and hashes needed
// to compute the root hash for a given general index.
//
// Thread-safety: the first Prove/ProveMulti/Hash call on a freshly built tree
// finalizes it (computes and caches all node hashes) and must not run
// concurrently with other calls on the same tree. A finalized tree is
// read-only, so concurrent calls on it are safe.
func (n *Node) Prove(index int) (*Proof, error) {
	h := hashingOf(n.Algorithm())
	return n.node().prove(&h, index)
}

// ProveMulti generates a Multiproof for the given set of generalized indices.
// It collects the leaf values at each index and the minimal set of auxiliary
// hashes needed to reconstruct the root. Returns an error if any index cannot
// be found in the tree.
//
// Thread-safety: like Prove, this finalizes the tree on first use, so the
// first call must not run concurrently with other calls on the same tree
// (see Prove).
func (n *Node) ProveMulti(indices []int) (*Multiproof, error) {
	h := hashingOf(n.Algorithm())
	return n.node().proveMulti(&h, indices)
}
