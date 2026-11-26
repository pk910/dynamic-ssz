// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
	"golang.org/x/crypto/sha3"
)

// refPair is an independent compression of two chunks.
type refPair func(a, b [32]byte) [32]byte

func refSHA256(a, b [32]byte) [32]byte {
	return sha256.Sum256(append(a[:], b[:]...))
}

func refKeccak256(a, b [32]byte) (out [32]byte) {
	h := sha3.NewLegacyKeccak256()
	h.Write(a[:])
	h.Write(b[:])
	h.Sum(out[:0])
	return out
}

// saltedSHA256 is a hash of its own: SHA256 with the first byte of every
// digest flipped by salt.
func saltedSHA256(salt byte) HashFn {
	return func(dst, input []byte) error {
		for i := 0; i+64 <= len(input); i += 64 {
			sum := sha256.Sum256(input[i : i+64])
			sum[0] ^= salt
			copy(dst[i/2:], sum[:])
		}
		return nil
	}
}

func refSaltedSHA256(salt byte) refPair {
	return func(a, b [32]byte) [32]byte {
		sum := refSHA256(a, b)
		sum[0] ^= salt
		return sum
	}
}

type algorithmRef struct {
	name string
	alg  *Algorithm
	ref  refPair
}

func algorithmRefs(t *testing.T) []algorithmRef {
	t.Helper()
	custom, err := AlgorithmFor(saltedSHA256(0x01))
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	return []algorithmRef{
		{"sha256", SHA256, refSHA256},
		{"sha256 native", SHA256Native, refSHA256},
		{"keccak256", Keccak256, refKeccak256},
		{"keccak256 native", Keccak256Native, refKeccak256},
		{"custom", custom, refSaltedSHA256(0x01)},
	}
}

// refMerkleize is the binary merkleization of the specification: chunks
// padded with zero subtrees up to limit leaves.
func refMerkleize(h refPair, chunks [][32]byte, limit uint64) [32]byte {
	layer := append([][32]byte(nil), chunks...)
	var zero [32]byte
	for width := uint64(1); width < limit; width <<= 1 {
		if len(layer)%2 == 1 {
			layer = append(layer, zero)
		}
		next := make([][32]byte, len(layer)/2)
		for i := range next {
			next[i] = h(layer[2*i], layer[2*i+1])
		}
		layer = next
		zero = h(zero, zero)
	}
	if len(layer) == 0 {
		return zero
	}
	return layer[0]
}

// refMerkleizeProgressive is the progressive merkleization: subtrees of 1, 4,
// 16, ... leaves, each the left child of the node holding the rest.
func refMerkleizeProgressive(h refPair, chunks [][32]byte, base int) [32]byte {
	if len(chunks) == 0 {
		return [32]byte{}
	}
	split := min(base, len(chunks))
	left := refMerkleize(h, chunks[:split], uint64(base))
	right := refMerkleizeProgressive(h, chunks[split:], base*4)
	return h(left, right)
}

func refMixin(h refPair, root [32]byte, num uint64) [32]byte {
	var mix [32]byte
	binary.LittleEndian.PutUint64(mix[:], num)
	return h(root, mix)
}

func TestAlgorithmZeroHashKnownAnswers(t *testing.T) {
	tests := []struct {
		name  string
		alg   *Algorithm
		depth int
		want  string
	}{
		{"sha256", SHA256, 1, "f5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a92759fb4b"},
		{"sha256", SHA256, 2, "db56114e00fdd4c1f85c892bf35ac9a89289aaecb1ebd0a96cde606a748b5d71"},
		{"sha256 native", SHA256Native, 1, "f5a5fd42d16a20302798ef6ed309979b43003d2320d9f0e8ea9831a92759fb4b"},
		{"keccak256", Keccak256, 1, "ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"},
		{"keccak256", Keccak256, 2, "b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"},
		{"keccak256 native", Keccak256Native, 1, "ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"},
	}
	for _, tt := range tests {
		if got := hex.EncodeToString(tt.alg.ZeroHash(tt.depth)); got != tt.want {
			t.Errorf("%s zero hash at depth %d = %s, want %s", tt.name, tt.depth, got, tt.want)
		}
	}
}

func TestAlgorithmZeroHashes(t *testing.T) {
	for _, tt := range algorithmRefs(t) {
		hashes := tt.alg.ZeroHashes()
		if hashes[0] != ([32]byte{}) {
			t.Errorf("%s: zero hash at depth 0 is not the zero chunk", tt.name)
		}
		for depth := 1; depth < len(hashes); depth++ {
			if want := tt.ref(hashes[depth-1], hashes[depth-1]); hashes[depth] != want {
				t.Fatalf("%s: zero hash at depth %d = %x, want %x", tt.name, depth, hashes[depth], want)
			}
			if level, ok := tt.alg.ZeroHashLevel(hashes[depth][:]); !ok || level != depth {
				t.Errorf("%s: ZeroHashLevel at depth %d = (%d, %v)", tt.name, depth, level, ok)
			}
		}

		if got := tt.alg.ZeroHash(-1); [32]byte(got) != hashes[0] {
			t.Errorf("%s: ZeroHash(-1) is not clamped to depth 0", tt.name)
		}
		if got := tt.alg.ZeroHash(1000); [32]byte(got) != hashes[64] {
			t.Errorf("%s: ZeroHash(1000) is not clamped to depth 64", tt.name)
		}
		if _, ok := tt.alg.ZeroHashLevel(hashes[1][:31]); ok {
			t.Errorf("%s: ZeroHashLevel accepted a short hash", tt.name)
		}
		unknown := [32]byte{0xff}
		if _, ok := tt.alg.ZeroHashLevel(unknown[:]); ok {
			t.Errorf("%s: ZeroHashLevel accepted an unknown hash", tt.name)
		}

		// The returned hash is the caller's.
		owned := tt.alg.ZeroHash(3)
		owned[0] ^= 0xff
		if [32]byte(tt.alg.ZeroHash(3)) != hashes[3] {
			t.Errorf("%s: ZeroHash returned shared memory", tt.name)
		}
	}
}

// Algorithms are compatible exactly when they compute the same hash, whatever
// computes it.
func TestAlgorithmCompatible(t *testing.T) {
	refs := algorithmRefs(t)
	probe := [32]byte{1}
	for _, a := range refs {
		for _, b := range refs {
			want := a.ref(probe, probe) == b.ref(probe, probe)
			if got := a.alg.Compatible(b.alg); got != want {
				t.Errorf("%s compatible with %s = %v, want %v", a.name, b.name, got, want)
			}
		}
	}

	for _, twins := range [][2]*Algorithm{{SHA256, SHA256Native}, {Keccak256, Keccak256Native}} {
		if twins[0].Native() != twins[1] || twins[1].Native() != twins[1] {
			t.Error("a built-in algorithm does not name its native counterpart")
		}
	}
	custom := refs[4].alg
	if custom.Native() != custom {
		t.Error("a custom algorithm is not its own native counterpart")
	}
}

func TestAlgorithmFor(t *testing.T) {
	fn := saltedSHA256(0x10)
	alg, err := AlgorithmFor(fn)
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	again, err := AlgorithmFor(fn)
	if err != nil || again != alg {
		t.Errorf("the same function gave another algorithm: %v", err)
	}
	// Another closure of the same literal that computes the same hash is the
	// same function to its callers.
	again, err = AlgorithmFor(saltedSHA256(0x10))
	if err != nil || again != alg {
		t.Errorf("an equal closure gave another algorithm: %v", err)
	}
	// One that computes another hash is not, though it shares the code.
	other, err := AlgorithmFor(saltedSHA256(0x11))
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	if other == alg || other.Compatible(alg) {
		t.Error("closures of different hashes share an algorithm")
	}

	// A SHA256 backend gets an algorithm of its own that is compatible with
	// the built-in one.
	backend, err := AlgorithmFor(NativeHashWrapperFactory(sha256.New))
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	if backend == SHA256 || !backend.Compatible(SHA256) || backend.Compatible(Keccak256) {
		t.Error("a SHA256 backend is not a separate algorithm compatible with SHA256")
	}

	if _, err = AlgorithmFor(nil); err == nil {
		t.Error("AlgorithmFor accepted a nil function")
	}

	refusal := errors.New("backend refused")
	if _, err = AlgorithmFor(func(_, _ []byte) error { return refusal }); !errors.Is(err, refusal) {
		t.Errorf("a failing function: err = %v, want the refusal", err)
	}
	// A function that fails after the first pair fails while the zero
	// subtree roots are built.
	calls := 0
	late := saltedSHA256(0x12)
	_, err = AlgorithmFor(func(dst, input []byte) error {
		calls++
		if calls > 1 {
			return refusal
		}
		return late(dst, input)
	})
	if !errors.Is(err, refusal) {
		t.Errorf("a function failing late: err = %v, want the refusal", err)
	}
}

func TestAlgorithmAccessors(t *testing.T) {
	if SHA256.Pool() != &FastHasherPool || SHA256Native.Pool() != &DefaultHasherPool {
		t.Error("the SHA256 pools are not FastHasherPool and DefaultHasherPool")
	}
	for _, tt := range algorithmRefs(t) {
		pool := tt.alg.Pool()
		if pool.Algorithm() != tt.alg {
			t.Errorf("%s: pool reports another algorithm", tt.name)
		}
		hh := pool.Get()
		if hh.Algorithm() != tt.alg {
			t.Errorf("%s: pooled hasher reports another algorithm", tt.name)
		}
		pool.Put(hh)
		// A hasher drawn again carries the pool's algorithm.
		hh = pool.Get()
		if hh.Algorithm() != tt.alg || AlgorithmOf(hh) != tt.alg {
			t.Errorf("%s: reused hasher reports another algorithm", tt.name)
		}
		pool.Put(hh)

		if hh = NewHasherWithAlgorithm(tt.alg); hh.Algorithm() != tt.alg {
			t.Errorf("%s: NewHasherWithAlgorithm reports another algorithm", tt.name)
		}

		a, b := [32]byte{1}, [32]byte{2}
		var pair [64]byte
		copy(pair[:32], a[:])
		copy(pair[32:], b[:])
		want := tt.ref(a, b)
		if got, err := tt.alg.HashPair(&pair); err != nil || got != want {
			t.Errorf("%s: HashPair = %x, %v, want %x", tt.name, got, err, want)
		}
		var dst [32]byte
		if err := tt.alg.HashFn()(dst[:], pair[:]); err != nil || dst != want {
			t.Errorf("%s: HashFn = %x, %v, want %x", tt.name, dst, err, want)
		}
	}

	// A pool without a function hashes with the Go standard library, and a
	// walker that names no algorithm with SHA256.
	var pool HasherPool
	if pool.Algorithm() != SHA256Native {
		t.Error("a zero pool is not a SHA256Native pool")
	}
	if hh := pool.Get(); hh.Algorithm() != SHA256Native {
		t.Error("the hasher of a zero pool is not a SHA256Native hasher")
	}
	if AlgorithmOf(plainWalker{}) != SHA256 {
		t.Error("a walker without Algorithm is not read as SHA256")
	}
	if NewHasher().Algorithm() != SHA256Native {
		t.Error("NewHasher is not a SHA256Native hasher")
	}
}

// plainWalker is a hash walker that does not report an Algorithm.
type plainWalker struct{ sszutils.HashWalker }

// A hasher or a pool made for a bare function gets the Algorithm of the hash
// that function computes, and pads with its zero subtree roots.
func TestHasherOfFunctionDerivesItsAlgorithm(t *testing.T) {
	chunks := makeChunks(3)
	root := func(hh *Hasher) [32]byte {
		t.Helper()
		idx := hh.StartTree(sszutils.TreeTypeBinary)
		for i := range chunks {
			hh.Append(chunks[i][:])
		}
		hh.MerkleizeWithMixin(idx, 3, 64)
		r, err := hh.HashRoot()
		if err != nil {
			t.Fatalf("HashRoot: %v", err)
		}
		hh.Reset()
		return r
	}
	want := func(ref refPair) [32]byte {
		return refMixin(ref, refMerkleize(ref, chunks, 64), 3)
	}

	salted := NewHasherWithHashFn(saltedSHA256(0x20))
	if got := root(salted); got != want(refSaltedSHA256(0x20)) {
		t.Errorf("custom function: root %x, want %x", got, want(refSaltedSHA256(0x20)))
	}
	if salted.Algorithm().Compatible(SHA256) {
		t.Error("the hasher of a custom function reports a SHA256 algorithm")
	}

	keccak := NewHasherWithHash(sha3.NewLegacyKeccak256())
	if got := root(keccak); got != want(refKeccak256) {
		t.Errorf("keccak256 hash.Hash: root %x, want %x", got, want(refKeccak256))
	}
	if !keccak.Algorithm().Compatible(Keccak256) {
		t.Error("the hasher of a keccak256 hash.Hash is not compatible with Keccak256")
	}

	sha := NewHasherWithHashFn(NativeHashWrapperFactory(sha256.New))
	if got := root(sha); got != want(refSHA256) || !sha.Algorithm().Compatible(SHA256) {
		t.Errorf("SHA256 function: root %x, want %x from a SHA256 compatible hasher", got, want(refSHA256))
	}

	// A pool made around a function derives its algorithm once, and again
	// when it is given another function.
	pool := &HasherPool{HashFn: saltedSHA256(0x20)}
	alg := pool.Algorithm()
	if alg != pool.Algorithm() || !alg.Compatible(salted.Algorithm()) {
		t.Error("a pool does not keep the algorithm of its function")
	}
	hh := pool.Get()
	if got := root(hh); got != want(refSaltedSHA256(0x20)) || hh.Algorithm() != alg {
		t.Errorf("pool of a custom function: root %x, want %x", got, want(refSaltedSHA256(0x20)))
	}
	pool.Put(hh)
	if hh = pool.Get(); hh.Algorithm() != alg {
		t.Error("a reused hasher lost the algorithm of its pool")
	}
	pool.HashFn = keccak256Fn
	if !pool.Algorithm().Compatible(Keccak256) {
		t.Error("a pool given another function kept the algorithm of the first")
	}

	// A function that cannot be probed, or none at all, is taken to compute
	// SHA256.
	refusal := errors.New("backend refused")
	failing := NewHasherWithHashFn(func(_, _ []byte) error { return refusal })
	if !failing.Algorithm().Compatible(SHA256) {
		t.Error("the hasher of a failing function is not SHA256 compatible")
	}
	idx := failing.StartTree(sszutils.TreeTypeBinary)
	failing.Append(chunks[0][:])
	failing.Append(chunks[1][:])
	failing.Merkleize(idx)
	if _, err := failing.HashRoot(); !errors.Is(err, refusal) {
		t.Errorf("failing function: HashRoot err = %v, want the refusal", err)
	}
	if !NewHasherWithHashFn(nil).Algorithm().Compatible(SHA256) {
		t.Error("the hasher of a nil function is not SHA256 compatible")
	}
}

// keccak256Fn is a top-level keccak256 function, with code of its own.
func keccak256Fn(dst, input []byte) error {
	for i := 0; i+64 <= len(input); i += 64 {
		sum := refKeccak256([32]byte(input[i:i+32]), [32]byte(input[i+32:i+64]))
		copy(dst[i/2:], sum[:])
	}
	return nil
}

func TestAlgorithmWithHasher(t *testing.T) {
	for _, tt := range algorithmRefs(t) {
		chunks := makeChunks(3)
		var root [32]byte
		err := tt.alg.WithHasher(func(hh sszutils.HashWalker) (err error) {
			if AlgorithmOf(hh) != tt.alg {
				t.Errorf("%s: WithHasher handed out a hasher of another algorithm", tt.name)
			}
			idx := hh.StartTree(sszutils.TreeTypeBinary)
			for i := range chunks {
				hh.Append(chunks[i][:])
			}
			hh.Merkleize(idx)
			root, err = hh.HashRoot()
			return err
		})
		if err != nil {
			t.Fatalf("%s: WithHasher: %v", tt.name, err)
		}
		if want := refMerkleize(tt.ref, chunks, 4); root != want {
			t.Errorf("%s: root = %x, want %x", tt.name, root, want)
		}
	}
}

func TestAlgorithmBinaryRoots(t *testing.T) {
	counts := []int{0, 1, 2, 3, 5, 8, 17, 255, 256, 257, 1000}
	for _, tt := range algorithmRefs(t) {
		for name, hh := range map[string]*Hasher{tt.name: tt.alg.Pool().Get()} {
			for _, count := range counts {
				chunks := makeChunks(count)
				tight := sszutils.NextPowerOfTwo(uint64(max(count, 1)))
				for _, limit := range []uint64{tight, tight * 8, 1 << 40} {
					want := refMixin(tt.ref, refMerkleize(tt.ref, chunks, limit), uint64(count))

					for _, cadence := range []int{0, 64} {
						idx := hh.StartTree(sszutils.TreeTypeBinary)
						for i := range chunks {
							hh.Append(chunks[i][:])
							if cadence > 0 && (i+1)%cadence == 0 {
								hh.Collapse()
							}
						}
						hh.MerkleizeWithMixin(idx, uint64(count), limit)
						got, err := hh.HashRoot()
						if err != nil {
							t.Fatalf("%s: HashRoot: %v", name, err)
						}
						hh.Reset()
						if got != want {
							t.Fatalf("%s: %d chunks, limit %d, cadence %d: root = %x, want %x",
								name, count, limit, cadence, got, want)
						}
					}
				}
			}
		}
	}
}

func TestAlgorithmContainerListRoots(t *testing.T) {
	// Elements of three chunks are padded to four with a zero chunk, and the
	// list of them with zero subtrees up to its limit.
	const elemChunks = 3
	for _, tt := range algorithmRefs(t) {
		for name, hh := range map[string]*Hasher{tt.name: tt.alg.Pool().Get()} {
			for _, count := range []int{0, 1, 2, 7, 300} {
				chunks := makeChunks(count * elemChunks)
				elemRoots := make([][32]byte, count)
				for i := range elemRoots {
					elemRoots[i] = refMerkleize(tt.ref, chunks[i*elemChunks:(i+1)*elemChunks], 4)
				}
				want := refMixin(tt.ref, refMerkleize(tt.ref, elemRoots, 1024), uint64(count))

				idx := hh.StartTree(sszutils.TreeTypeBinary)
				for i := 0; i < count; i++ {
					ci := hh.StartTree(sszutils.TreeTypeNone)
					for c := 0; c < elemChunks; c++ {
						hh.Append(chunks[i*elemChunks+c][:])
					}
					hh.Merkleize(ci)
					if (i+1)%64 == 0 {
						hh.Collapse()
					}
				}
				hh.MerkleizeWithMixin(idx, uint64(count), 1024)
				got, err := hh.HashRoot()
				if err != nil {
					t.Fatalf("%s: HashRoot: %v", name, err)
				}
				hh.Reset()
				if got != want {
					t.Fatalf("%s: %d elements: root = %x, want %x", name, count, got, want)
				}
			}
		}
	}
}

func TestAlgorithmProgressiveRoots(t *testing.T) {
	for _, tt := range algorithmRefs(t) {
		for name, hh := range map[string]*Hasher{tt.name: tt.alg.Pool().Get()} {
			for _, count := range []int{0, 1, 2, 5, 6, 21, 22, 100, 1500} {
				chunks := makeChunks(count)
				want := refMixin(tt.ref, refMerkleizeProgressive(tt.ref, chunks, 1), uint64(count))

				idx := hh.StartTree(sszutils.TreeTypeProgressive)
				for i := range chunks {
					hh.Append(chunks[i][:])
					if (i+1)%64 == 0 {
						hh.Collapse()
					}
				}
				hh.MerkleizeProgressiveWithMixin(idx, uint64(count))
				got, err := hh.HashRoot()
				if err != nil {
					t.Fatalf("%s: HashRoot: %v", name, err)
				}
				hh.Reset()
				if got != want {
					t.Fatalf("%s: %d chunks: root = %x, want %x", name, count, got, want)
				}
			}
		}
	}
}

// Background reduction and the native implementation give the root the
// accelerated backend gives, under either Algorithm.
func TestAlgorithmAsyncMatchesSync(t *testing.T) { //nolint:paralleltest // async hashing is a process-wide switch
	defer DisableAsyncHashing()

	sequences := []asyncSequence{
		{elemChunks: 8, n: 12288, cadence: 256, limit: 1 << 40},
		{elemChunks: 3, n: 20000, cadence: 256, limit: 1 << 40},
		{n: 70000, cadence: 1024, limit: 1 << 40},
		{progressive: true, elemChunks: 8, n: 12288, cadence: 256},
	}
	for _, tt := range algorithmRefs(t) {
		for _, s := range sequences {
			DisableAsyncHashing()
			want := runAsyncSequence(t, NewHasherWithAlgorithm(tt.alg), s, 7, false)
			if native := runAsyncSequence(t, NewHasherWithAlgorithm(tt.alg.Native()), s, 7, false); native != want {
				t.Errorf("%s %s: native root %x != root %x", tt.name, s, native, want)
			}

			EnableAsyncHashing(4)
			if got := runAsyncSequence(t, NewHasherWithAlgorithm(tt.alg), s, 7, true); got != want {
				t.Errorf("%s %s: async root %x != sync root %x", tt.name, s, got, want)
			}
		}
	}

	DisableAsyncHashing()
	s := sequences[0]
	if runAsyncSequence(t, NewHasherWithAlgorithm(SHA256), s, 7, false) ==
		runAsyncSequence(t, NewHasherWithAlgorithm(Keccak256), s, 7, false) {
		t.Error("both algorithms give the same root")
	}
}

// The functions below open the built-in Algorithms to the tests of the
// external test package, which drive them through dynssz and treeproof.

// ResetZeroTables drops the zero subtree roots of the built-in Algorithms,
// as if none had been used yet.
func ResetZeroTables() {
	for _, alg := range []*Algorithm{SHA256, SHA256Native, Keccak256, Keccak256Native} {
		alg.table.Store(nil)
	}
}

// ZeroTableBuilt reports whether the Algorithm holds its zero subtree roots.
func ZeroTableBuilt(alg *Algorithm) bool {
	return alg.table.Load() != nil
}

// CountCompressions counts the calls of the Algorithm's hash function until
// the returned function is called.
func CountCompressions(alg *Algorithm, calls *atomic.Int64) (restore func()) {
	installed := alg.pool.HashFn
	fn := alg.pool.CurrentHashFn()
	alg.pool.HashFn = func(dst, input []byte) error {
		calls.Add(1)
		return fn(dst, input)
	}

	return func() { alg.pool.HashFn = installed }
}
