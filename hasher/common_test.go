// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher_test

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"sync/atomic"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
	"github.com/pk910/dynamic-ssz/treeproof"
)

type isolationElem struct {
	X uint64
	Y [48]byte
}

// isolationRootOnly has a structure and hashes itself through the
// argument-less method only, which both engines call under SHA256 alone.
type isolationRootOnly struct {
	A uint64
	B uint64
}

func (r *isolationRootOnly) HashTreeRoot() ([32]byte, error) {
	return [32]byte{0xaa}, nil
}

type isolationValue struct {
	A uint64
	L []uint64        `ssz-max:"1024"`
	E []isolationElem `ssz-max:"16"`
	R isolationRootOnly
}

// A run under one hash touches nothing of another: it builds none of the
// other hash's zero subtree roots and never calls its function. Every entry
// point that takes an Algorithm is run here.
func TestAlgorithmsAreIsolated(t *testing.T) { //nolint:paralleltest // the built-in algorithms are process-wide
	other, err := hasher.AlgorithmFor(sha512256Fn)
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	// The zero subtree roots of a built-in hash are built on first use, so a
	// run that builds them is seen; those of another hash exist from the
	// moment its Algorithm is made.
	families := []struct {
		name  string
		algs  [2]*hasher.Algorithm
		built bool
	}{
		{"sha256", [2]*hasher.Algorithm{hasher.SHA256, hasher.SHA256Native}, false},
		{"sha512/256", [2]*hasher.Algorithm{other, other}, true},
	}

	for _, family := range families {
		for i, alg := range family.algs {
			if i == 1 && alg == family.algs[0] {
				continue
			}
			var others []*hasher.Algorithm
			lazy := map[*hasher.Algorithm]bool{}
			for _, otherFamily := range families {
				if otherFamily.name != family.name {
					others = append(others, otherFamily.algs[0])
					if otherFamily.algs[1] != otherFamily.algs[0] {
						others = append(others, otherFamily.algs[1])
					}
					lazy[otherFamily.algs[0]], lazy[otherFamily.algs[1]] = !otherFamily.built, !otherFamily.built
				}
			}

			hasher.ResetZeroTables()
			var calls, accelerated atomic.Int64
			restores := make([]func(), 0, len(others)+1)
			for _, o := range others {
				restores = append(restores, hasher.CountCompressions(o, &calls))
			}
			// A run of the native Algorithm stays out of the accelerated
			// backend of its own hash too.
			if alg == family.algs[1] && alg != family.algs[0] {
				restores = append(restores, hasher.CountCompressions(family.algs[0], &accelerated))
			}

			runEveryEntryPoint(t, family.name, alg)

			for _, restore := range restores {
				restore()
			}
			if n := calls.Load(); n != 0 {
				t.Errorf("%s: a run called the function of another hash %d times", family.name, n)
			}
			if n := accelerated.Load(); n != 0 {
				t.Errorf("%s: a native run called the accelerated backend %d times", family.name, n)
			}
			for _, o := range others {
				if lazy[o] && hasher.ZeroTableBuilt(o) {
					t.Errorf("%s: a run built the zero subtree roots of another hash", family.name)
				}
			}
		}
	}
	hasher.ResetZeroTables()
}

// sha512256Fn computes SHA-512/256, a second hash from the standard library.
func sha512256Fn(dst, input []byte) error {
	for i := 0; i+64 <= len(input); i += 64 {
		sum := sha512.Sum512_256(input[i : i+64])
		copy(dst[i/2:], sum[:])
	}
	return nil
}

func runEveryEntryPoint(t *testing.T, name string, alg *hasher.Algorithm) {
	t.Helper()
	fail := func(step string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %s: %v", name, step, err)
		}
	}
	value := &isolationValue{A: 1, L: []uint64{1, 2, 3}, E: []isolationElem{{X: 1}, {X: 2}}, R: isolationRootOnly{A: 4, B: 5}}
	option := dynssz.WithHashAlgorithm(alg)
	proofOption := treeproof.WithAlgorithm(alg)

	for engine, ds := range map[string]*dynssz.DynSsz{
		"instance default": dynssz.NewDynSsz(nil, dynssz.WithDefaultHashAlgorithm(alg)),
		"call option":      dynssz.NewDynSsz(nil),
		"reflection":       dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation()),
	} {
		root, err := ds.HashTreeRoot(value, option)
		fail(engine+" HashTreeRoot", err)

		hh := alg.Pool().Get()
		fail(engine+" HashTreeRootWith", ds.HashTreeRootWith(value, hh, option))
		walked, err := hh.HashRoot()
		alg.Pool().Put(hh)
		fail(engine+" HashRoot", err)
		if walked != root {
			t.Fatalf("%s: %s: the walker root differs from the root", name, engine)
		}

		tree, err := ds.GetTree(value, option)
		fail(engine+" GetTree", err)
		if !bytes.Equal(tree.Hash(), root[:]) {
			t.Fatalf("%s: %s: the tree root differs from the root", name, engine)
		}

		// Field L is the second of four: gindex 5, its data tree 10 with 256
		// chunks. 10*256 is a value, 10*256+128 padding, twice that below a
		// zero chunk.
		for _, index := range []int{10 * 256, 10*256 + 128, 3} {
			proof, proveErr := tree.Prove(index)
			fail(engine+" Prove", proveErr)
			ok, verifyErr := treeproof.VerifyProof(root[:], proof, proofOption)
			fail(engine+" VerifyProof", verifyErr)
			if !ok {
				t.Fatalf("%s: %s: proof of %d does not verify", name, engine, index)
			}
		}
		if _, err = tree.Get((10*256 + 128) * 2); err == nil {
			t.Fatalf("%s: %s: found a node below a zero chunk", name, engine)
		}

		for _, indices := range [][]int{{10 * 256, 10*256 + 1}, {10 * 256, 3}, {10*256 + 128, 4}} {
			proof, proveErr := tree.ProveMulti(indices)
			fail(engine+" ProveMulti", proveErr)
			restored := proof.Compress(proofOption).Decompress(proofOption)
			ok, verifyErr := treeproof.VerifyMultiproof(root[:], restored.Hashes, restored.Leaves, restored.Indices, proofOption)
			fail(engine+" VerifyMultiproof", verifyErr)
			if !ok {
				t.Fatalf("%s: %s: multiproof of %v does not verify", name, engine, indices)
			}
		}
		tree.Show(1)
	}

	// A full leaf layer verifies without proof hashes, through a hasher.
	w := treeproof.NewWrapper(proofOption)
	idx := w.StartTree(sszutils.TreeTypeBinary)
	leaves := make([][]byte, 4)
	for i := range leaves {
		leaves[i] = make([]byte, 32)
		leaves[i][0] = byte(i + 1)
		w.Append(leaves[i])
	}
	w.Merkleize(idx)
	root, err := w.HashRoot()
	fail("wrapper HashRoot", err)
	ok, err := treeproof.VerifyMultiproof(root[:], nil, leaves, []int{4, 5, 6, 7}, proofOption)
	fail("VerifyMultiproof of a full leaf layer", err)
	if !ok {
		t.Fatalf("%s: the full leaf layer does not verify", name)
	}

	// A tree left unfinished is hashed with its own Algorithm when first
	// asked, and so is one built by hand.
	lazy := treeproof.NewWrapper(proofOption)
	lazyIdx := lazy.StartTree(sszutils.TreeTypeBinary)
	for i := range leaves {
		lazy.Append(leaves[i])
	}
	lazy.MerkleizeWithMixin(lazyIdx, 4, 64)
	lazyTree, err := lazy.Root()
	fail("wrapper Root", err)
	proof, err := lazyTree.Prove(2 * 64)
	fail("Prove on an unfinished tree", err)
	ok, err = treeproof.VerifyProof(lazyTree.Hash(), proof, proofOption)
	fail("VerifyProof", err)
	if !ok {
		t.Fatalf("%s: the proof of an unfinished tree does not verify", name)
	}

	handLeaves := make([]*treeproof.Node, len(leaves))
	for i := range leaves {
		handLeaves[i] = treeproof.NewNodeWithValue(leaves[i], proofOption) //nolint:staticcheck // the deprecated API is part of what is checked
	}
	handTree, err := treeproof.TreeFromNodesWithMixin(handLeaves, 4, 64, proofOption) //nolint:staticcheck // the deprecated API is part of what is checked
	fail("TreeFromNodesWithMixin", err)
	if !bytes.Equal(handTree.Hash(), lazyTree.Hash()) {
		t.Fatalf("%s: a tree built by hand differs from the walked one", name)
	}
	if _, err = handTree.Get(2*64 + 40); err != nil {
		t.Fatalf("%s: Get into the padding of a hand-built tree: %v", name, err)
	}

	// A subtree of this hash is taken by a wrapper of this hash.
	joined := treeproof.NewWrapper(proofOption)
	joinIdx := joined.Index()
	joined.AddNode(handTree)                         //nolint:staticcheck // the deprecated API is part of what is checked
	joined.AddNode(treeproof.EmptyLeaf(proofOption)) //nolint:staticcheck // the deprecated API is part of what is checked
	joined.Merkleize(joinIdx)
	_, err = joined.HashRoot()
	fail("AddNode of a subtree", err)

	// A hasher and a pool made around the bare function belong to its hash.
	pool := &hasher.HasherPool{HashFn: alg.HashFn()}
	if !pool.Algorithm().Compatible(alg) || !hasher.NewHasherWithHashFn(alg.HashFn()).Algorithm().Compatible(alg) {
		t.Fatalf("%s: a bare function was given another hash", name)
	}
	_ = alg.ZeroHash(3)
	_ = alg.Compatible(hasher.SHA256)
}

// poolRoot hashes two chunks into a list with room for four with a hasher of
// the pool.
func poolRoot(t *testing.T, pool *hasher.HasherPool) [32]byte {
	t.Helper()
	hh := pool.Get()
	defer pool.Put(hh)

	indx := hh.Index()
	hh.PutUint64(1)
	hh.PutUint64(2)
	hh.MerkleizeWithMixin(indx, 2, 4)
	root, err := hh.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot: %v", err)
	}

	return root
}

// A pool made by the caller hashes with the function its field holds now,
// also where the new function runs the code of the one before.
func TestPoolFollowsItsHashFn(t *testing.T) {
	other, err := hasher.AlgorithmFor(sha512256Fn)
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	want := map[string][32]byte{
		"sha256":     poolRoot(t, hasher.SHA256Native.Pool()),
		"sha512/256": poolRoot(t, other.Pool()),
	}
	constructors := map[string]func() hash.Hash{"sha256": sha256.New, "sha512/256": sha512.New512_256}
	algorithms := map[string]*hasher.Algorithm{"sha256": hasher.SHA256, "sha512/256": other}

	pool := &hasher.HasherPool{}
	for _, name := range []string{"sha256", "sha512/256", "sha256"} {
		pool.HashFn = hasher.NativeHashWrapperFactory(constructors[name])
		if got := poolRoot(t, pool); got != want[name] {
			t.Errorf("HashFn set to %s: root %x, want %x", name, got, want[name])
		}
		if !pool.Algorithm().Compatible(algorithms[name]) {
			t.Errorf("HashFn set to %s: the pool names another hash", name)
		}
	}
}

// A function installed in DefaultHasherPool is used by that pool only:
// NewHasher and a pool without a function compress with the built-in.
func TestDefaultPoolHashFnStaysInItsPool(t *testing.T) { //nolint:paralleltest // changes a built-in pool
	want := poolRoot(t, &hasher.HasherPool{})

	var calls atomic.Int64
	restore := hasher.CountCompressions(hasher.SHA256Native, &calls)
	defer restore()

	if got := poolRoot(t, &hasher.HasherPool{}); got != want || calls.Load() != 0 {
		t.Errorf("a pool without a function: root %x, want %x; %d calls of the installed function", got, want, calls.Load())
	}

	hh := hasher.NewHasher()
	indx := hh.Index()
	hh.PutUint64(1)
	hh.PutUint64(2)
	hh.MerkleizeWithMixin(indx, 2, 4)
	if got, err := hh.HashRoot(); err != nil || got != want || calls.Load() != 0 {
		t.Errorf("NewHasher: root %x, %v, want %x; %d calls of the installed function", got, err, want, calls.Load())
	}

	if got := poolRoot(t, &hasher.DefaultHasherPool); got != want || calls.Load() == 0 {
		t.Errorf("DefaultHasherPool: root %x, want %x; %d calls of the installed function", got, want, calls.Load())
	}
}

// FastHasherPool and DefaultHasherPool stay the pools of SHA256 and
// SHA256Native when a caller replaces them as a whole.
func TestReplacedBuiltInPoolsKeepTheirAlgorithm(t *testing.T) { //nolint:paralleltest // replaces the built-in pools
	want := poolRoot(t, &hasher.FastHasherPool)

	hasher.FastHasherPool = hasher.HasherPool{HashFn: hasher.FastHasherPool.HashFn}
	hasher.DefaultHasherPool = hasher.HasherPool{}

	if hasher.FastHasherPool.Algorithm() != hasher.SHA256 || hasher.DefaultHasherPool.Algorithm() != hasher.SHA256Native {
		t.Error("a replaced built-in pool names another Algorithm")
	}
	for name, pool := range map[string]*hasher.HasherPool{"FastHasherPool": &hasher.FastHasherPool, "DefaultHasherPool": &hasher.DefaultHasherPool} {
		if got := poolRoot(t, pool); got != want {
			t.Errorf("%s: root %x, want %x", name, got, want)
		}
	}
}
