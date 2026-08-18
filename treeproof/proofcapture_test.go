// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// proofCaptureStream drives every HashWalker method the engines emit: two
// nested scopes, one Index-opened scope, chunk appends of all widths, and
// the Put-style subtree calls.
func proofCaptureStream(hh sszutils.HashWalker) {
	root := hh.StartTree(sszutils.TreeTypeNone)

	// child 0: a scope with mixed appends
	inner := hh.StartTree(sszutils.TreeTypeNone)
	hh.AppendUint64(1)
	hh.AppendUint32(2)
	hh.AppendUint16(3)
	hh.AppendUint8(4)
	hh.AppendBool(true)
	hh.FillUpTo32()
	hh.Append([]byte{9, 9})
	hh.FillUpTo32()
	hh.AppendBytes32([]byte{5, 6, 7})
	hh.Merkleize(inner)

	// child 1: an Index-opened scope (legacy style)
	legacy := hh.Index()
	hh.PutUint64(11)
	hh.PutUint32(12)
	hh.Merkleize(legacy)

	// children 2..8: single-chunk puts and subtree puts
	hh.PutBool(true)
	hh.PutUint8(21)
	hh.PutUint16(22)
	hh.PutUint64(23)
	hh.PutBytes([]byte{1, 2, 3})
	hh.PutBytes(bytes.Repeat([]byte{7}, 96))
	hh.PutBitlist([]byte{0xaa, 0x01}, 64)

	// child 9: a list scope with a mixin; elements carry a subtree put so
	// muted regions exercise the subtree-put path too
	list := hh.StartTree(sszutils.TreeTypeBinary)
	for i := range 5 {
		elem := hh.StartTree(sszutils.TreeTypeNone)
		hh.PutUint64(uint64(100 + i))
		hh.PutUint64(uint64(200 + i))
		hh.PutBitlist([]byte{byte(i), 0x01}, 32)
		hh.Merkleize(elem)
		hh.Collapse()
	}
	hh.MerkleizeWithMixin(list, 5, 8)

	// child 10: a progressive scope (retained whole by any schedule) with a
	// nested plain-progressive subscope
	prog := hh.StartTree(sszutils.TreeTypeProgressive)
	nested := hh.StartTree(sszutils.TreeTypeProgressive)
	hh.PutUint64(30)
	hh.MerkleizeProgressive(nested)
	hh.PutUint64(31)
	hh.PutUint64(32)
	hh.PutUint64(33)
	hh.MerkleizeProgressiveWithMixin(prog, 3)

	// child 11: a uint64 array subtree and a root vector subtree
	hh.PutUint64Array([]uint64{41, 42, 43}, 16)
	roots := [][]byte{bytes.Repeat([]byte{3}, 32), bytes.Repeat([]byte{4}, 32)}
	if rv, ok := hh.(interface {
		PutRootVector(b [][]byte, maxCapacity ...uint64) error
	}); ok {
		if err := rv.PutRootVector(roots, 8); err != nil {
			panic(err)
		}
	} else {
		// The tree wrapper has no PutRootVector; replay the same structure.
		idx := hh.Index()
		for _, r := range roots {
			hh.AppendBytes32(r)
		}
		hh.MerkleizeWithMixin(idx, uint64(len(roots)), sszutils.CalculateLimit(8, uint64(len(roots)), 32))
	}

	// children 12..13: a progressive bitlist subtree and an uncapped root
	// vector subtree
	hh.PutProgressiveBitlist([]byte{0x3c, 0x01})
	roots2 := [][]byte{bytes.Repeat([]byte{5}, 32)}
	if rv, ok := hh.(interface {
		PutRootVector(b [][]byte, maxCapacity ...uint64) error
	}); ok {
		if err := rv.PutRootVector(roots2); err != nil {
			panic(err)
		}
	} else {
		idx := hh.Index()
		hh.AppendBytes32(roots2[0])
		hh.Merkleize(idx)
	}

	hh.Merkleize(root)
}

// runProofCapture drives the stream through a proofCapture with the given
// schedule and returns the pruned tree.
func runProofCapture(t *testing.T, sched *ProofSchedule) *Node {
	t.Helper()
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	pc := newProofCapture(sched, hh, nil, 0)
	proofCaptureStream(pc)
	tree, err := pc.ProofTree()
	if err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	return tree
}

// The pruned tree must reproduce the full wrapper tree's root and proofs for
// every retained path, across all capture states: scheduled scopes, muted
// subtrees, retained subtrees, and Put-style subtree children.
func TestProofCaptureStreamParity(t *testing.T) {
	ref := NewWrapper()
	proofCaptureStream(ref)
	refTree := ref.Node()
	refRoot := refTree.Hash()

	var gindices []int
	collectStreamGindices(refTree, 1, &gindices)

	// Retain everything: the pruned tree must serve every proof of the full
	// tree.
	full := runProofCapture(t, retainAllSchedule)
	if !bytes.Equal(full.Hash(), refRoot) {
		t.Fatalf("retainAll root = %x, want %x", full.Hash(), refRoot)
	}
	for _, gindex := range gindices {
		want, refErr := refTree.Prove(gindex)
		got, err := full.Prove(gindex)
		if refErr != nil {
			if err == nil {
				t.Fatalf("gindex %d: reference errored (%v), pruned did not", gindex, refErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("gindex %d: %v", gindex, err)
		}
		assertSameProof(t, gindex, want, got)
	}

	// A partial schedule: descend into the mixed-append scope (child 0), the
	// bitlist subtree (child 8, via retainAll from a manual schedule), and
	// list element 3 (child 9 -> chunks -> slot 3). Everything else runs
	// muted through the hasher.
	sched := &ProofSchedule{children: map[uint64]*ProofSchedule{
		0:  {children: map[uint64]*ProofSchedule{1: {}}},
		7:  {children: map[uint64]*ProofSchedule{1: {}}},
		8:  {retainAll: true},
		9:  {children: map[uint64]*ProofSchedule{3: {children: map[uint64]*ProofSchedule{0: {}}}}},
		11: {retainAll: true},
		12: {retainAll: true},
		13: {retainAll: true},
	}}
	pruned := runProofCapture(t, sched)
	if !bytes.Equal(pruned.Hash(), refRoot) {
		t.Fatalf("pruned root = %x, want %x", pruned.Hash(), refRoot)
	}
	// The scheduled paths serve the same proofs as the full tree.
	scopeG := 16 + 0 // child 0 of the 14-child (16-slot) root scope
	bitlistG := 16 + 8
	listElemG := (16+9)*2*8 + 3
	putBytesG := 16 + 7
	arrayG := 16 + 11
	progBitsG := 16 + 12
	rootVecG := 16 + 13
	for _, gindex := range []int{1, scopeG, scopeG*2 + 1, bitlistG, bitlistG * 2, bitlistG*2 + 1, listElemG, listElemG * 2,
		putBytesG * 2, putBytesG*2 + 1, arrayG * 2, arrayG*2 + 1, progBitsG * 2, rootVecG * 2} {
		want, refErr := refTree.Prove(gindex)
		if refErr != nil {
			t.Fatalf("reference Prove(%d): %v", gindex, refErr)
		}
		got, err := pruned.Prove(gindex)
		if err != nil {
			t.Fatalf("pruned Prove(%d): %v", gindex, err)
		}
		assertSameProof(t, gindex, want, got)
	}
}

func collectStreamGindices(node *Node, gindex int, out *[]int) {
	if node == nil || gindex > 1<<20 {
		return
	}
	*out = append(*out, gindex)
	collectStreamGindices(node.left, gindex*2, out)
	collectStreamGindices(node.right, gindex*2+1, out)
}

func assertSameProof(t *testing.T, gindex int, want, got *Proof) {
	t.Helper()
	if !bytes.Equal(got.Leaf, want.Leaf) {
		t.Fatalf("gindex %d: leaf = %x, want %x", gindex, got.Leaf, want.Leaf)
	}
	if len(got.Hashes) != len(want.Hashes) {
		t.Fatalf("gindex %d: %d hashes, want %d", gindex, len(got.Hashes), len(want.Hashes))
	}
	for i := range got.Hashes {
		if !bytes.Equal(got.Hashes[i], want.Hashes[i]) {
			t.Fatalf("gindex %d: hash %d mismatch", gindex, i)
		}
	}
}

// ProofTree reports the walk's failure modes: an unclosed scope, a stream
// that never produced a root, and a shadow diverging from the hasher root.
func TestProofCaptureProofTreeErrors(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)

	// No content at all: HashRoot fails.
	pc := newProofCapture(&ProofSchedule{}, hh, nil, 0)
	if _, err := pc.ProofTree(); err == nil {
		t.Fatal("expected error for empty stream")
	}

	// A bare chunk without any scope: served as a leaf.
	hh.Reset()
	pc = newProofCapture(&ProofSchedule{}, hh, nil, 0)
	pc.PutUint64(7)
	tree, err := pc.ProofTree()
	if err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	if !tree.IsLeaf() {
		t.Fatal("expected leaf tree for bare chunk")
	}

	// A corrupted captured node diverges from the hasher root.
	hh.Reset()
	divergeSched := &ProofSchedule{}
	divergeSched.addChild(0)
	pc = newProofCapture(divergeSched, hh, nil, 0)
	idx := pc.StartTree(sszutils.TreeTypeNone)
	pc.PutUint64(7)
	pc.PutUint64(8)
	pc.Merkleize(idx)
	pc.scopes[0].childNodes[0].value[0] ^= 0xff
	if _, err := pc.ProofTree(); err == nil {
		t.Fatal("expected divergence error for corrupted capture")
	}
}

// reduceAligned reduces power-of-two segments in batched level order, and
// the pairwise fallback must produce the same root as the backend.
func TestProofCaptureReduceAligned(t *testing.T) {
	pc := &proofCapture{}
	seg := bytes.Repeat([]byte{1}, 4*32)

	root := pc.reduceAligned(seg)
	want := hashPair(hashPair(seg[0:32], seg[32:64]), hashPair(seg[64:96], seg[96:128]))
	if !bytes.Equal(root[:], want) {
		t.Fatalf("reduceAligned = %x, want %x", root, want)
	}

	broken := &proofCapture{hashFn: func(_, _ []byte) error { return errors.New("backend failure") }}
	viaFallback := broken.reduceAligned(seg)
	if root != viaFallback {
		t.Fatalf("fallback root %x != backend root %x", viaFallback, root)
	}
}

// foldRange pads missing ranges with zero subtrees: plain zero-hash values
// off-path, shared empty nodes where a scheduled slot needs structure below.
func TestProofCaptureFoldRangePadding(t *testing.T) {
	pc := &proofCapture{}
	scope := &proofCaptureScope{sched: &ProofSchedule{}}

	entry := pc.foldRange(scope, nil, 2, 4)
	if !bytes.Equal(entry.value[:], hasher.GetZeroHash(2)) || entry.retained {
		t.Fatalf("off-path padding = %x retained=%v, want zero hash value", entry.value, entry.retained)
	}

	scope.sched.addChild(5)
	entry = pc.foldRange(scope, nil, 2, 4)
	if !entry.retained || entry.node != getEmptyNode(2) {
		t.Fatalf("scheduled padding should retain the shared empty node")
	}
}

// A Merkleize on the base level without a tracked scope forwards untouched.
func TestProofCaptureBaseMerkleize(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	pc := newProofCapture(&ProofSchedule{}, hh, nil, 0)
	pc.PutUint64(1)
	pc.PutUint64(2)
	pc.Merkleize(0)
	tree, err := pc.ProofTree()
	if err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	if !tree.IsLeaf() {
		t.Fatal("expected leaf tree")
	}
}

// A capture whose bookkeeping diverges from the hash tree root must surface
// in Node instead of serving a wrong proof; the misuse of feeding the
// never running the walk against the returned walker lands here too.
func TestWrapperNodeCaptureDivergence(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)

	w := NewWrapper(WithProofCapture(&ProofSchedule{}, hh, nil))
	pc, ok := w.(*proofCapture)
	if !ok {
		t.Fatal("NewWrapper(WithProofCapture) should return the proof capture engine")
	}
	pc.registerChild(0, newOwnedLeaf(bytes.Repeat([]byte{0xAB}, 32)))

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for diverging capture")
		}
	}()
	_ = w.Node()
}

// A scheduled scope closing progressively means the stream and the
// descriptor disagree: the fold cannot reproduce the progressive shape from
// its reduced entries, so the divergence surfaces at ProofTree instead of a
// wrong proof.
func TestProofCaptureProgressiveMismatch(t *testing.T) {
	for _, variant := range []string{"plain", "mixin", "activeFields"} {
		t.Run(variant, func(t *testing.T) {
			hh := hasher.FastHasherPool.Get()
			defer hasher.FastHasherPool.Put(hh)

			sched := &ProofSchedule{}
			sched.addChild(0)
			pc := newProofCapture(sched, hh, nil, 0)

			idx := pc.StartTree(sszutils.TreeTypeNone)
			pc.AppendBytes32(bytes.Repeat([]byte{1}, 32))
			pc.AppendBytes32(bytes.Repeat([]byte{2}, 32))
			switch variant {
			case "mixin":
				pc.MerkleizeProgressiveWithMixin(idx, 2)
			case "activeFields":
				pc.MerkleizeProgressiveWithActiveFields(idx, []byte{0x03})
			default:
				pc.MerkleizeProgressive(idx)
			}

			if _, err := pc.ProofTree(); err == nil {
				t.Fatal("expected divergence error for progressive mismatch")
			}
		})
	}
}

// PutRootVector rejects malformed roots through the hasher.
func TestProofCapturePutRootVectorError(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)

	sched := &ProofSchedule{}
	sched.addChild(0)
	pc := newProofCapture(sched, hh, nil, 0)
	if err := pc.PutRootVector([][]byte{{1, 2, 3}}); err == nil {
		t.Fatal("expected error for malformed root")
	}
}

// A force sweep consumes an unaligned trailing partial chunk zero-extended,
// both alone and behind a run of complete chunks, matching the padding the
// hasher itself applies.
func TestProofCaptureSweepPartialTail(t *testing.T) {
	run := func(t *testing.T, payload []byte) {
		t.Helper()
		ref := newTreeBuilder()
		refIdx := ref.StartTree(sszutils.TreeTypeNone)
		ref.Append(payload)
		ref.Merkleize(refIdx)
		refTree := ref.Node()
		refTree.Finalize()

		hh := hasher.FastHasherPool.Get()
		defer hasher.FastHasherPool.Put(hh)
		sched := &ProofSchedule{}
		sched.addChild(0)
		pc := newProofCapture(sched, hh, nil, 0)
		idx := pc.StartTree(sszutils.TreeTypeNone)
		pc.Append(payload)
		pc.Merkleize(idx)
		tree, err := pc.ProofTree()
		if err != nil {
			t.Fatalf("ProofTree: %v", err)
		}
		if !bytes.Equal(tree.value, refTree.value) {
			t.Fatalf("pruned root %x != reference %x", tree.value, refTree.value)
		}
	}
	t.Run("partial only", func(t *testing.T) { run(t, bytes.Repeat([]byte{7}, 8)) })
	t.Run("run with partial tail", func(t *testing.T) { run(t, bytes.Repeat([]byte{7}, 40)) })
	t.Run("multi-chunk run with partial tail", func(t *testing.T) { run(t, bytes.Repeat([]byte{7}, 100)) })
}

// Bulk appends into a scheduled scope land block-sized: the hasher's buffer
// stays bounded regardless of the append size, and the split changes no
// roots.
func TestProofCaptureBulkAppendBounded(t *testing.T) {
	payload := bytes.Repeat([]byte{7}, 4<<20)

	ref := newTreeBuilder()
	refIdx := ref.StartTree(sszutils.TreeTypeNone)
	ref.Append(payload)
	ref.Merkleize(refIdx)
	refTree := ref.Node()
	refTree.Finalize()

	run := func() (*Node, int) {
		hh := hasher.NewHasher()
		sched := &ProofSchedule{}
		sched.addChild(5)
		pc := newProofCapture(sched, hh, nil, 0)
		idx := pc.StartTree(sszutils.TreeTypeNone)
		pc.Append(payload)
		pc.Merkleize(idx)
		tree, err := pc.ProofTree()
		if err != nil {
			t.Fatalf("ProofTree: %v", err)
		}
		return tree, hh.BufferCap()
	}
	tree, bufCap := run()
	if !bytes.Equal(tree.value, refTree.value) {
		t.Fatalf("bulk-append root %x != reference %x", tree.value, refTree.value)
	}
	if bufCap > 4*captureBlockChunks*32 {
		t.Fatalf("hasher buffer grew to %d bytes despite block-bounded sweeps", bufCap)
	}
}

// Rebuilt subtrees (retained shapes, Put-style replays) finalize through the
// capture's configured hash function instead of the accelerated default.
func TestProofCaptureFallbackFinalizeBackend(t *testing.T) {
	var calls int
	countFn := func(dst, input []byte) error {
		calls++
		return hasher.FastHasherPool.HashFn(dst, input)
	}

	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	sched := &ProofSchedule{children: map[uint64]*ProofSchedule{0: retainAllSchedule}}
	pc := newProofCapture(sched, hh, countFn, 0)

	root := pc.StartTree(sszutils.TreeTypeNone)
	retained := pc.StartTree(sszutils.TreeTypeBinary)
	for i := range 64 {
		pc.PutUint64(uint64(i))
	}
	pc.MerkleizeWithMixin(retained, 64, 64)
	pc.Merkleize(root)

	if _, err := pc.ProofTree(); err != nil {
		t.Fatalf("ProofTree: %v", err)
	}
	if calls == 0 {
		t.Fatal("retained subtree finalization bypassed the configured hash function")
	}
}

// Node serves the pruned tree directly for callers that do not use the
// error-returning ProofTree access.
func TestProofCaptureNodeSuccess(t *testing.T) {
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	sched := &ProofSchedule{}
	sched.addChild(0)
	pc := newProofCapture(sched, hh, nil, 0)
	idx := pc.StartTree(sszutils.TreeTypeNone)
	pc.PutUint64(7)
	pc.PutUint64(8)
	pc.Merkleize(idx)

	tree := pc.Node()
	if tree == nil || tree.value == nil {
		t.Fatal("Node returned no finalized tree")
	}
}

// Bulk payloads behind a partial leading chunk must still sweep: the split
// tops the current chunk up to alignment first. Also covers AppendBytes32
// splitting.
func TestProofCaptureMisalignedBulkAppend(t *testing.T) {
	payload := bytes.Repeat([]byte{9}, 4<<20)

	run := func(feed func(hh sszutils.HashWalker)) *Node {
		t.Helper()
		ref := newTreeBuilder()
		refIdx := ref.StartTree(sszutils.TreeTypeNone)
		feed(ref)
		ref.Merkleize(refIdx)
		refTree := ref.Node()
		refTree.Finalize()

		hh := hasher.NewHasher()
		sched := &ProofSchedule{}
		sched.addChild(3)
		pc := newProofCapture(sched, hh, nil, 0)
		idx := pc.StartTree(sszutils.TreeTypeNone)
		feed(pc)
		pc.Merkleize(idx)
		tree, err := pc.ProofTree()
		if err != nil {
			t.Fatalf("ProofTree: %v", err)
		}
		if !bytes.Equal(tree.value, refTree.value) {
			t.Fatalf("root %x != reference %x", tree.value, refTree.value)
		}
		if bufCap := hh.BufferCap(); bufCap > 4*captureBlockChunks*32 {
			t.Fatalf("hasher buffer grew to %d bytes despite alignment-aware splitting", bufCap)
		}
		return tree
	}

	run(func(hh sszutils.HashWalker) {
		hh.Append([]byte{1})
		hh.Append(payload)
		hh.FillUpTo32()
	})
	run(func(hh sszutils.HashWalker) {
		hh.AppendBytes32(payload)
	})
}

// Direct coverage of the capture's chunk-built subtree paths: root-only
// skips, uncapped arrays and root vectors, out-of-range scheduled slots,
// odd-tail and fallback range reduction, unaligned entry pairs, and the
// plain progressive close.
func TestProofCaptureChunkBuiltPaths(t *testing.T) {
	// Root-only Put targets register nothing; the sweep serves the chunk.
	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)
	rootOnly := &ProofSchedule{children: map[uint64]*ProofSchedule{0: {targets: []uint64{1}}}}
	pc := newProofCapture(rootOnly, hh, nil, 0)
	idx := pc.StartTree(sszutils.TreeTypeNone)
	inner := pc.StartTree(sszutils.TreeTypeNone)
	pc.PutProgressiveBitlist([]byte{0x3c, 0x01})
	pc.PutUint64Array([]uint64{1, 2, 3})
	if err := pc.PutRootVector([][]byte{bytes.Repeat([]byte{3}, 32)}); err != nil {
		t.Fatal(err)
	}
	pc.Merkleize(inner)
	pc.Merkleize(idx)
	if _, err := pc.ProofTree(); err != nil {
		t.Fatal(err)
	}

	// Deep targets into uncapped arrays and root vectors build from chunks.
	hh2 := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh2)
	deep := &ProofSchedule{children: map[uint64]*ProofSchedule{
		0: {children: map[uint64]*ProofSchedule{1: {}}, targets: []uint64{5}},
		1: {children: map[uint64]*ProofSchedule{0: {}}, targets: []uint64{2}},
	}}
	pc2 := newProofCapture(deep, hh2, nil, 0)
	idx2 := pc2.StartTree(sszutils.TreeTypeNone)
	pc2.PutUint64Array([]uint64{10, 11, 12, 13, 14})
	if err := pc2.PutRootVector([][]byte{bytes.Repeat([]byte{4}, 32), bytes.Repeat([]byte{5}, 32)}); err != nil {
		t.Fatal(err)
	}
	pc2.Merkleize(idx2)
	if _, err := pc2.ProofTree(); err != nil {
		t.Fatal(err)
	}

	// Scheduled slots beyond the data collapse to shared empty nodes.
	beyond := &ProofSchedule{children: map[uint64]*ProofSchedule{6: {}, 7: {}}}
	pc3 := &proofCapture{}
	node := pc3.prunedChunkTree(beyond, bytes.Repeat([]byte{1}, 2*32), 3, 0)
	if node == nil || node.value == nil {
		t.Fatal("prunedChunkTree returned no tree")
	}
	if leaf, err := (&Node{left: node, right: getEmptyNode(0), value: hashPair(node.value, hasher.GetZeroHash(0))}).Prove(2*8 + 6); err != nil {
		t.Fatalf("empty-slot path not retained: %v", err)
	} else if !bytes.Equal(leaf.Leaf, hasher.GetZeroHash(0)) {
		t.Fatalf("empty slot leaf = %x, want zero hash", leaf.Leaf)
	}

	// Odd-tail padding and the pairwise fallback agree with the backend.
	chunks := bytes.Repeat([]byte{2}, 3*32)
	viaBackend := pc3.chunkRangeRoot(chunks, 0, 2, 3)
	broken := &proofCapture{hashFn: func(_, _ []byte) error { return errors.New("backend failure") }}
	viaFallback := broken.chunkRangeRoot(chunks, 0, 2, 3)
	if viaBackend != viaFallback {
		t.Fatalf("fallback root %x != backend root %x", viaFallback, viaBackend)
	}

	// Unaligned equal-depth neighbors must not merge.
	scope := &proofCaptureScope{sched: &ProofSchedule{}}
	pc3.pushEntry(scope, captureEntry{slot: 1, depth: 0})
	pc3.pushEntry(scope, captureEntry{slot: 2, depth: 0})
	if len(scope.entries) != 2 {
		t.Fatalf("unaligned pair merged: %d entries", len(scope.entries))
	}

	// A plain progressive close of a progressively scheduled scope folds
	// without a mixin.
	hh4 := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh4)
	progSched := &ProofSchedule{progressive: true, children: map[uint64]*ProofSchedule{0: {}}}
	pc4 := newProofCapture(progSched, hh4, nil, 0)
	idx4 := pc4.StartTree(sszutils.TreeTypeProgressive)
	pc4.AppendBytes32(bytes.Repeat([]byte{1}, 32))
	pc4.AppendBytes32(bytes.Repeat([]byte{2}, 32))
	pc4.MerkleizeProgressive(idx4)
	if _, err := pc4.ProofTree(); err != nil {
		t.Fatal(err)
	}
}

// consumeProgressivePath pins over-deep spine positions and progGroupStart
// saturates at the last representable group.
func TestProofScheduleProgressiveCorners(t *testing.T) {
	// 40 right-turns down the spine: beyond any representable chunk count.
	rel := uint64(1)<<41 - 1
	slot, ok := consumeProgressivePath(&rel)
	if ok {
		t.Fatalf("over-deep spine unexpectedly resolved to slot %d", slot)
	}

	huge := progGroupStart(^uint64(0) >> 1)
	if span := groupSpan(huge); span == 0 {
		t.Fatal("saturated group has no span")
	}
}
