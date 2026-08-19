package engine

// Merkle tree-generation and proof fuzzing. The built-in differential engine
// never exercised GetTree / Prove / ProveMulti; this battery does, for every
// randomly generated type that produces a valid value:
//
//   - GetTree() on the reflection AND codegen engines; asserts both tree roots
//     equal HashTreeRoot and the two engines' trees are structurally identical
//     (node-by-node hashes)
//   - enumerates leaf generalized indices and, for each: Get(gi) leaf ==
//     Prove(gi).Leaf, VerifyProof(root, proof) == true (completeness), and every
//     tamper (leaf byte, a sibling hash, the wrong root) is rejected (soundness)
//   - ProveMulti over random index subsets: VerifyMultiproof true; each tamper
//     rejected
//   - proofs built on the reflection tree also verify against the codegen tree's
//     root

import (
	"bytes"
	"fmt"
	"math/rand"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/treeproof"
)

// capturePanic runs fn and returns a non-empty string (the recovered value) if
// it panicked, or "" otherwise. Used by the oracle checks to convert a panic into
// a reported issue instead of taking down the worker.
func capturePanic(fn func()) (p string) {
	defer func() {
		if r := recover(); r != nil {
			p = fmt.Sprintf("panic: %v", r)
		}
	}()
	fn()
	return ""
}

// collectLeaves walks the tree collecting up to `limit` leaf generalized indices.
func collectLeaves(n *treeproof.Node, gi, depth, maxDepth, limit int, out *[]int) {
	if n == nil || len(*out) >= limit || depth > maxDepth {
		return
	}
	if n.IsLeaf() {
		*out = append(*out, gi)
		return
	}
	collectLeaves(n.Left(), gi*2, depth+1, maxDepth, limit, out)
	collectLeaves(n.Right(), gi*2+1, depth+1, maxDepth, limit, out)
}

// treeEqual compares two trees structurally (shape + node hashes) up to maxDepth.
func treeEqual(a, b *treeproof.Node, depth, maxDepth int) bool {
	if depth > maxDepth {
		return true
	}
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	if a.IsLeaf() != b.IsLeaf() || !bytes.Equal(a.Hash(), b.Hash()) {
		return false
	}
	if a.IsLeaf() {
		return true
	}
	return treeEqual(a.Left(), b.Left(), depth+1, maxDepth) && treeEqual(a.Right(), b.Right(), depth+1, maxDepth)
}

func flipByte(b []byte, i int) []byte {
	c := append([]byte(nil), b...)
	if len(c) > 0 {
		c[i%len(c)] ^= 0x80
	}
	return c
}

// proofCheck runs the full tree/proof battery for one valid value. reflDs is the
// reflection engine, cgDs the codegen (delegating) engine; htr is the value's
// HashTreeRoot. Issues are reported through fail; ck counts a completed check.
func proofCheck(reflDs, cgDs *dynssz.DynSsz, val any, htr [32]byte, rng *rand.Rand,
	fail func(kind, detail string), ck func()) {

	const maxDepth = 22 // bound traversal for very large / deep trees

	var reflTree, cgTree *treeproof.Node
	if p := capturePanic(func() { reflTree, _ = reflDs.GetTree(val) }); p != "" {
		fail("tree-gettree-refl-panic", p)
		return
	}
	if cgDs != nil {
		if p := capturePanic(func() { cgTree, _ = cgDs.GetTree(val) }); p != "" {
			fail("tree-gettree-cg-panic", p)
			return
		}
	}
	if reflTree == nil {
		return
	}
	ck()

	root := reflTree.Hash()
	if !bytes.Equal(root, htr[:]) {
		fail("tree-root-neq-htr", fmt.Sprintf("tree=%x htr=%x", root, htr))
	}
	if cgTree != nil {
		if !bytes.Equal(cgTree.Hash(), root) {
			fail("tree-refl-cg-root-divergence", fmt.Sprintf("refl=%x cg=%x", root, cgTree.Hash()))
		}
		if !treeEqual(reflTree, cgTree, 0, maxDepth) {
			fail("tree-refl-cg-structure-divergence", "trees differ in shape/node-hash")
		}
	}
	ck()

	var leaves []int
	collectLeaves(reflTree, 1, 0, maxDepth, 96, &leaves)
	if len(leaves) == 0 {
		return
	}

	// --- single proofs: completeness + soundness at each sampled leaf ---
	for _, gi := range leaves {
		var node *treeproof.Node
		var gerr error
		if p := capturePanic(func() { node, gerr = reflTree.Get(gi) }); p != "" {
			fail("proof-get-panic", fmt.Sprintf("gi=%d %s", gi, p))
			continue
		}
		var pr *treeproof.Proof
		var perr error
		if p := capturePanic(func() { pr, perr = reflTree.Prove(gi) }); p != "" {
			fail("proof-prove-panic", fmt.Sprintf("gi=%d %s", gi, p))
			continue
		}
		if perr != nil || pr == nil {
			continue
		}
		if gerr == nil && node != nil && !bytes.Equal(node.Hash(), pr.Leaf) {
			fail("proof-leaf-neq-get", fmt.Sprintf("gi=%d get=%x prove=%x", gi, node.Hash(), pr.Leaf))
		}
		// COMPLETENESS
		if ok, verr := treeproof.VerifyProof(root, pr); verr == nil && !ok {
			fail("proof-completeness-fail", fmt.Sprintf("gi=%d valid proof did not verify", gi))
		}
		ck()
		if cgTree != nil {
			if ok2, verr2 := treeproof.VerifyProof(cgTree.Hash(), pr); verr2 == nil && !ok2 {
				fail("proof-cross-engine-verify-fail", fmt.Sprintf("gi=%d refl proof fails vs cg root", gi))
			}
		}
		// SOUNDNESS: tampered leaf
		bad := &treeproof.Proof{Index: pr.Index, Leaf: flipByte(pr.Leaf, rng.Intn(32)), Hashes: pr.Hashes}
		if ok, _ := treeproof.VerifyProof(root, bad); ok && !bytes.Equal(bad.Leaf, pr.Leaf) {
			fail("proof-soundness-tampered-leaf-accepted", fmt.Sprintf("gi=%d", gi))
		}
		ck()
		// SOUNDNESS: tampered sibling hash
		if len(pr.Hashes) > 0 {
			hi := rng.Intn(len(pr.Hashes))
			th := make([][]byte, len(pr.Hashes))
			copy(th, pr.Hashes)
			th[hi] = flipByte(th[hi], 0)
			if ok, _ := treeproof.VerifyProof(root, &treeproof.Proof{Index: pr.Index, Leaf: pr.Leaf, Hashes: th}); ok {
				fail("proof-soundness-tampered-hash-accepted", fmt.Sprintf("gi=%d hi=%d", gi, hi))
			}
			ck()
		}
		// SOUNDNESS: wrong root
		if ok, _ := treeproof.VerifyProof(flipByte(root, 0), pr); ok {
			fail("proof-soundness-wrong-root-accepted", fmt.Sprintf("gi=%d", gi))
		}
		ck()
	}

	// --- multiproofs: completeness + soundness over random subsets ---
	for s := 0; s < 3 && len(leaves) >= 2; s++ {
		k := 1 + rng.Intn(min(len(leaves), 8))
		idxset := map[int]bool{}
		var indices []int
		for len(indices) < k {
			g := leaves[rng.Intn(len(leaves))]
			if !idxset[g] {
				idxset[g] = true
				indices = append(indices, g)
			}
		}
		var mp *treeproof.Multiproof
		var merr error
		if p := capturePanic(func() { mp, merr = reflTree.ProveMulti(indices) }); p != "" {
			fail("multiproof-provemulti-panic", p)
			continue
		}
		if merr != nil || mp == nil {
			continue
		}
		if ok, verr := treeproof.VerifyMultiproof(root, mp.Hashes, mp.Leaves, mp.Indices); verr == nil && !ok {
			fail("multiproof-completeness-fail", fmt.Sprintf("indices=%v did not verify", indices))
		}
		ck()
		if len(mp.Leaves) > 0 {
			tl := make([][]byte, len(mp.Leaves))
			copy(tl, mp.Leaves)
			li := rng.Intn(len(tl))
			tl[li] = flipByte(tl[li], rng.Intn(32))
			if ok, _ := treeproof.VerifyMultiproof(root, mp.Hashes, tl, mp.Indices); ok && !bytes.Equal(tl[li], mp.Leaves[li]) {
				fail("multiproof-soundness-tampered-leaf-accepted", fmt.Sprintf("indices=%v", indices))
			}
			ck()
		}
		if len(mp.Hashes) > 0 {
			th := make([][]byte, len(mp.Hashes))
			copy(th, mp.Hashes)
			hi := rng.Intn(len(th))
			th[hi] = flipByte(th[hi], 0)
			if ok, _ := treeproof.VerifyMultiproof(root, th, mp.Leaves, mp.Indices); ok {
				fail("multiproof-soundness-tampered-hash-accepted", fmt.Sprintf("indices=%v", indices))
			}
			ck()
		}
		if ok, _ := treeproof.VerifyMultiproof(flipByte(root, 0), mp.Hashes, mp.Leaves, mp.Indices); ok {
			fail("multiproof-soundness-wrong-root-accepted", fmt.Sprintf("indices=%v", indices))
		}
		ck()
	}
}
