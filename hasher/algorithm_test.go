// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/pk910/dynamic-ssz/internal/zerohash"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// refPair is an independent compression of two chunks.
type refPair func(a, b [32]byte) [32]byte

func refSHA256(a, b [32]byte) [32]byte {
	return sha256.Sum256(append(a[:], b[:]...))
}

// refSHA512256 is SHA-512/256, a second hash from the standard library.
func refSHA512256(a, b [32]byte) [32]byte {
	return sha512.Sum512_256(append(a[:], b[:]...))
}

// sha512256Fn is a top-level SHA-512/256 function, with code of its own.
func sha512256Fn(dst, input []byte) error {
	for i := 0; i+64 <= len(input); i += 64 {
		sum := sha512.Sum512_256(input[i : i+64])
		copy(dst[i/2:], sum[:])
	}
	return nil
}

// sha512256Twin is another SHA-512/256 function.
func sha512256Twin(dst, input []byte) error {
	return sha512256Fn(dst, input)
}

// otherAlgorithm is the Algorithm of a hash that is not SHA256.
func otherAlgorithm(t *testing.T) *Algorithm {
	t.Helper()
	alg, err := AlgorithmFor(sha512256Fn)
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	return alg
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
		{"sha512/256", otherAlgorithm(t), refSHA512256},
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
		if table := zerohash.Table(tt.alg); table == nil || *table != hashes {
			t.Errorf("%s: the shared zero subtree roots are not those of the Algorithm", tt.name)
		}
		if zerohash.Table(nil) != nil || zerohash.Table((*Algorithm)(nil)) != nil {
			t.Error("a value that is no Algorithm has zero subtree roots")
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

	for _, twins := range [][2]*Algorithm{{SHA256, SHA256Native}} {
		if twins[0].Native() != twins[1] || twins[1].Native() != twins[1] {
			t.Error("a built-in algorithm does not name its native counterpart")
		}
	}
	custom := refs[3].alg
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
	// Another closure of the same literal is another function value: it gets
	// an Algorithm of its own, of the same hash.
	twin, err := AlgorithmFor(saltedSHA256(0x10))
	if err != nil || twin == alg || !twin.Compatible(alg) || twin.family != alg.family {
		t.Errorf("an equal closure is not a separate algorithm of the same hash: %v", err)
	}
	// Two functions around hash instances of their own keep their instances.
	first, second := sha256.New(), sha256.New()
	firstAlg, err := AlgorithmFor(NativeHashWrapper(first))
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	secondAlg, err := AlgorithmFor(NativeHashWrapper(second))
	if err != nil || firstAlg == secondAlg {
		t.Errorf("two functions of two hash instances share an algorithm: %v", err)
	}
	// One that computes another hash is not compatible, though it shares the
	// code.
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
	if backend == SHA256 || !backend.Compatible(SHA256) || backend.Compatible(otherAlgorithm(t)) {
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
		var got [32]byte
		if err := tt.alg.HashPair(&got, &pair); err != nil || got != want {
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

	other := NewHasherWithHash(sha512.New512_256())
	if got := root(other); got != want(refSHA512256) {
		t.Errorf("SHA-512/256 hash.Hash: root %x, want %x", got, want(refSHA512256))
	}
	if !other.Algorithm().Compatible(otherAlgorithm(t)) {
		t.Error("the hasher of a SHA-512/256 hash.Hash is not compatible with otherAlgorithm(t)")
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
	pool.HashFn = sha512256Fn
	if !pool.Algorithm().Compatible(otherAlgorithm(t)) {
		t.Error("a pool given another function kept the algorithm of the first")
	}

	// A function that fails while its hash is identified has no known hash:
	// the hasher reports the failure, and identifies the hash once the
	// function answers.
	refusal := errors.New("backend refused")
	refusing := true
	failing := NewHasherWithHashFn(func(dst, input []byte) error {
		if refusing {
			return refusal
		}
		return sha512256Fn(dst, input)
	})
	for _, known := range []*Algorithm{SHA256, otherAlgorithm(t), failing.Algorithm()} {
		if failing.Algorithm().Compatible(known) && known != failing.Algorithm() {
			t.Error("the hasher of a failing function names a known hash")
		}
	}
	if other := NewHasherWithHashFn(func(_, _ []byte) error { return refusal }); other.Algorithm().Compatible(failing.Algorithm()) {
		t.Error("two functions of unknown hashes are taken to compute the same one")
	}
	idx := failing.StartTree(sszutils.TreeTypeBinary)
	failing.Append(chunks[0][:])
	failing.Append(chunks[1][:])
	failing.Merkleize(idx)
	if _, err := failing.HashRoot(); !errors.Is(err, refusal) {
		t.Errorf("failing function: HashRoot err = %v, want the refusal", err)
	}
	refusing = false
	failing.Reset()
	if got := root(failing); got != want(refSHA512256) || !failing.Algorithm().Compatible(otherAlgorithm(t)) {
		t.Errorf("after the function answers: root %x, want the SHA-512/256 root %x", got, want(refSHA512256))
	}

	// A pool around such a function asks it again for every hasher.
	refusing = true
	refusingPool := &HasherPool{HashFn: func(dst, input []byte) error {
		if refusing {
			return refusal
		}
		return sha512256Fn(dst, input)
	}}
	hh = refusingPool.Get()
	idx = hh.StartTree(sszutils.TreeTypeBinary)
	hh.Append(chunks[0][:])
	hh.Append(chunks[1][:])
	hh.Merkleize(idx)
	if _, err := hh.HashRoot(); !errors.Is(err, refusal) {
		t.Errorf("pool of a failing function: HashRoot err = %v, want the refusal", err)
	}
	refusing = false
	if got := root(refusingPool.Get()); got != want(refSHA512256) {
		t.Errorf("pool after the function answers: root %x, want %x", got, want(refSHA512256))
	}

	// A function that identifies as a hash of its own and fails while the
	// zero subtree roots are computed has no known hash either.
	var calls int
	partial := NewHasherWithHashFn(func(dst, input []byte) error {
		calls++
		if calls > 10 {
			return refusal
		}
		return saltedSHA256(0x21)(dst, input)
	})
	if partial.Algorithm().Compatible(SHA256) {
		t.Error("a function that failed while its zero subtree roots were computed is taken to compute SHA256")
	}

	if !NewHasherWithHashFn(nil).Algorithm().Compatible(SHA256) {
		t.Error("the hasher of a nil function is not SHA256 compatible")
	}
	if !NewHasherWithHash(nil).Algorithm().Compatible(SHA256) {
		t.Error("the hasher of a nil hash is not SHA256 compatible")
	}
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
		runAsyncSequence(t, NewHasherWithAlgorithm(otherAlgorithm(t)), s, 7, false) {
		t.Error("both algorithms give the same root")
	}
}

// The functions below open the built-in Algorithms to the tests of the
// external test package, which drive them through dynssz and treeproof.

// ResetZeroTables drops the zero subtree roots of the built-in Algorithms,
// as if none had been used yet.
func ResetZeroTables() {
	for _, named := range *namedAlgorithms.Load() {
		named.alg.family.table.Store(nil)
	}
}

// ZeroTableBuilt reports whether the Algorithm holds its zero subtree roots.
func ZeroTableBuilt(alg *Algorithm) bool {
	return alg.family.table.Load() != nil
}

// CountCompressions counts the calls of the Algorithm's hash function until
// the returned function is called.
func CountCompressions(alg *Algorithm, calls *atomic.Int64) (restore func()) {
	fn := alg.HashFn()
	counting := func(dst, input []byte) error {
		calls.Add(1)
		return fn(dst, input)
	}
	if alg.fn != nil {
		alg.fn = counting
		return func() { alg.fn = fn }
	}
	installed := alg.pool.HashFn
	alg.pool.HashFn = counting

	return func() { alg.pool.HashFn = installed }
}

// The function of a built-in Algorithm gives that Algorithm.
func TestAlgorithmForBuiltIns(t *testing.T) {
	for name, alg := range map[string]*Algorithm{
		"SHA256": SHA256, "SHA256Native": SHA256Native,
	} {
		got, err := AlgorithmFor(alg.HashFn())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != alg {
			t.Errorf("%s: AlgorithmFor gives another Algorithm for its function", name)
		}
	}
}

// Algorithms of one hash share its zero subtree roots, whatever code computes
// it, and nothing is compatible with no Algorithm.
func TestAlgorithmsOfOneHashShareAFamily(t *testing.T) {
	inner := saltedSHA256(0x51)
	first, err := AlgorithmFor(inner)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AlgorithmFor(func(dst, input []byte) error { return inner(dst, input) })
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("functions of different code give one Algorithm")
	}
	if !first.Compatible(second) || first.family != second.family {
		t.Error("two Algorithms of one hash do not share its zero subtree roots")
	}
	if first.Compatible(nil) {
		t.Error("an Algorithm is compatible with none")
	}
}

// The function of an Algorithm is fixed: its pool's field does not replace it.
func TestAlgorithmFunctionIsFixed(t *testing.T) { //nolint:paralleltest // changes a built-in pool
	pool := otherAlgorithm(t).Pool()
	installed := pool.HashFn
	pool.HashFn = defaultHash
	defer func() { pool.HashFn = installed }()

	var a, b [32]byte
	a[0], b[0] = 1, 2
	want := refSHA512256(a, b)

	hh := pool.Get()
	defer pool.Put(hh)
	indx := hh.Index()
	hh.AppendBytes32(a[:])
	hh.AppendBytes32(b[:])
	hh.Merkleize(indx)
	got, err := hh.HashRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("root %x, want the SHA-512/256 root %x", got, want)
	}
}

// HashPair computes the pair with the Algorithm's hash, without a heap
// allocation for SHA256 and with one for the copy another hash is given.
func TestHashPair(t *testing.T) {
	var pair [64]byte
	pair[0], pair[32] = 1, 2
	a, b := [32]byte(pair[:32]), [32]byte(pair[32:])

	salted, err := AlgorithmFor(saltedSHA256(0x52))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		alg    *Algorithm
		ref    refPair
		allocs float64
	}{
		{"SHA256", SHA256, refSHA256, 0},
		{"SHA256Native", SHA256Native, refSHA256, 0},
		{"sha512/256", otherAlgorithm(t), refSHA512256, 1},
		{"salted", salted, refSaltedSHA256(0x52), 1},
	} {
		var got [32]byte
		hashErr := tc.alg.HashPair(&got, &pair)
		if hashErr != nil {
			t.Fatalf("%s: %v", tc.name, hashErr)
		}
		if got != tc.ref(a, b) {
			t.Errorf("%s: HashPair = %x, want %x", tc.name, got, tc.ref(a, b))
		}
		allocs := testing.AllocsPerRun(100, func() { _ = tc.alg.HashPair(&got, &pair) })
		if allocs > tc.allocs {
			t.Errorf("%s: HashPair allocates %v times, want at most %v", tc.name, allocs, tc.allocs)
		}
	}
}

// An Algorithm is registered under a name once: the name then finds it, and
// belongs to no other Algorithm.
func TestRegisterAlgorithm(t *testing.T) {
	if alg, ok := AlgorithmByName("sha256"); !ok || alg != SHA256 {
		t.Error("sha256 does not name SHA256")
	}
	if _, ok := AlgorithmByName("registry-test-unknown"); ok {
		t.Error("an unregistered name finds an Algorithm")
	}

	other := otherAlgorithm(t)
	if err := RegisterAlgorithm("registry-test", other); err != nil {
		t.Fatalf("RegisterAlgorithm: %v", err)
	}
	if err := RegisterAlgorithm("registry-test", other); err != nil {
		t.Errorf("registering the same Algorithm again: %v", err)
	}
	// Another Algorithm of the same hash may claim the name too; the one
	// registered first keeps it.
	sameHash, err := AlgorithmFor(sha512256Twin)
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	if err = RegisterAlgorithm("registry-test", sameHash); err != nil {
		t.Errorf("registering another Algorithm of the same hash: %v", err)
	}
	if alg, ok := AlgorithmByName("registry-test"); !ok || alg != other {
		t.Error("the registered name does not find its Algorithm")
	}
	for name, err := range map[string]error{
		"a name of another hash": RegisterAlgorithm("registry-test", SHA256),
		"sha256 for another":     RegisterAlgorithm("sha256", other),
		"no name":                RegisterAlgorithm("", other),
		"no Algorithm":           RegisterAlgorithm("registry-test-nil", nil),
	} {
		if err == nil {
			t.Errorf("%s: registered", name)
		}
	}

	var a, b [32]byte
	a[0], b[0] = 1, 2
	var got [32]byte
	err = WithNamedHasher("registry-test", func(hh sszutils.HashWalker) error {
		indx := hh.Index()
		hh.AppendBytes32(a[:])
		hh.AppendBytes32(b[:])
		hh.Merkleize(indx)
		var rootErr error
		got, rootErr = hh.HashRoot()
		return rootErr
	})
	if err != nil || got != refSHA512256(a, b) {
		t.Errorf("WithNamedHasher: root %x, %v; want %x", got, err, refSHA512256(a, b))
	}
	if err = WithNamedHasher("registry-test-unknown", func(sszutils.HashWalker) error { return nil }); !errors.Is(err, sszutils.ErrInvalidTag) {
		t.Errorf("WithNamedHasher of an unregistered name: err = %v, want ErrInvalidTag", err)
	}
}
