// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// zeroHashDepths is the number of zero subtree roots a hash has: one per tree
// depth from 0 to 64.
const zeroHashDepths = 65

// zeroTable holds the zero subtree roots of one hash.
type zeroTable struct {
	hashes [zeroHashDepths][32]byte
	levels map[[32]byte]int
}

// hashFamily is one hash, whatever computes it: the Algorithms that compute
// the same hash share a family, and with it the zero subtree roots.
type hashFamily struct {
	// fingerprint is the hash of a pair of zero chunks, which tells one
	// hash from another.
	fingerprint [32]byte
	// build computes the zero subtree roots when they are first needed.
	build HashFn
	table atomic.Pointer[zeroTable]
	// unknown marks the hash of a function that failed while it was
	// identified: it is no hash another Algorithm computes.
	unknown bool
}

// sha256Family is the hash of the consensus specification.
var sha256Family = &hashFamily{fingerprint: sha256Fingerprint, build: defaultHash}

// Algorithm is a merkleization hash: a function that compresses two 32-byte
// chunks into one, with the zero subtree roots and the hasher pool derived
// from it. The tree shape is the same under every Algorithm; only the node
// hashes differ, so a root is meaningful only together with the hash that
// produced it.
//
// An Algorithm is safe for concurrent use. The zero subtree roots of a
// built-in hash are computed when they are first needed.
type Algorithm struct {
	// fn is the function the Algorithm compresses with. SHA256 and
	// SHA256Native have none: they compress with the function installed in
	// their pool.
	fn     HashFn
	pool   *HasherPool
	native *Algorithm
	family *hashFamily
}

// SHA256 is the SSZ merkleization hash of the consensus specification,
// computed by the accelerated hashtree backend. Its pool is FastHasherPool.
var SHA256 = &Algorithm{pool: &FastHasherPool, family: sha256Family}

// SHA256Native is SHA256 computed by the Go standard library. Its pool is
// DefaultHasherPool.
var SHA256Native = &Algorithm{pool: &DefaultHasherPool, family: sha256Family}

func init() {
	registerBuiltIn("sha256", SHA256, SHA256Native)
}

// registerBuiltIn makes an accelerated Algorithm and its pure Go counterpart
// known: each is the Algorithm of its pool and of its function, and name is
// what an ssz-hash annotation calls the accelerated one.
func registerBuiltIn(name string, alg, native *Algorithm) {
	for _, twin := range []*Algorithm{alg, native} {
		twin.pool.alg = twin
		twin.native = native
		addKnownAlgorithm(algorithmEntry{
			code:        reflect.ValueOf(twin.HashFn()).Pointer(),
			fingerprint: twin.family.fingerprint,
			alg:         twin,
		})
	}
	// A built-in name is registered once, during package initialization.
	_ = RegisterAlgorithm(name, alg)
}

// sha256Fingerprint is the SHA256 hash of a pair of zero chunks: what tells
// a function that computes SHA256 from any other.
var sha256Fingerprint = [32]byte{
	0xf5, 0xa5, 0xfd, 0x42, 0xd1, 0x6a, 0x20, 0x30, 0x27, 0x98, 0xef, 0x6e, 0xd3, 0x09, 0x97, 0x9b,
	0x43, 0x00, 0x3d, 0x23, 0x20, 0xd9, 0xf0, 0xe8, 0xea, 0x98, 0x31, 0xa9, 0x27, 0x59, 0xfb, 0x4b,
}

// fingerprintOf returns the hash fn gives for a pair of zero chunks.
func fingerprintOf(fn HashFn) ([32]byte, error) {
	buf := make([]byte, 64)
	if err := fn(buf[:32], buf); err != nil {
		return [32]byte{}, err
	}

	return [32]byte(buf[:32]), nil
}

// buildZeroTable computes the zero subtree roots above the given hash of a
// pair of zero chunks.
func buildZeroTable(fingerprint [32]byte, fn HashFn) (*zeroTable, error) {
	t := &zeroTable{levels: make(map[[32]byte]int, zeroHashDepths)}
	t.levels[t.hashes[0]] = 0
	t.hashes[1] = fingerprint
	t.levels[fingerprint] = 1

	buf := make([]byte, 64)
	for i := 2; i < zeroHashDepths; i++ {
		copy(buf[:32], t.hashes[i-1][:])
		copy(buf[32:], t.hashes[i-1][:])
		if err := fn(buf[:32], buf); err != nil {
			return nil, err
		}
		copy(t.hashes[i][:], buf[:32])
		t.levels[t.hashes[i]] = i
	}

	return t, nil
}

// zero returns the zero subtree roots of the hash.
func (f *hashFamily) zero() *zeroTable {
	if t := f.table.Load(); t != nil {
		return t
	}

	// Only a built-in hash comes without its table, and its function does
	// not fail. Hashers that get here together each build the table; the
	// first one stored is the one all of them use.
	t, _ := buildZeroTable(f.fingerprint, f.build)
	f.table.CompareAndSwap(nil, t)

	return f.table.Load()
}

// familyOf returns the family of the hash with the given fingerprint, which
// fn computes: the family of an Algorithm that computes the same hash, or a
// new one. The zero subtree roots of a new family are computed here, so a
// function that fails reports it.
func familyOf(fn HashFn, fingerprint [32]byte) (*hashFamily, error) {
	if known := knownAlgorithms.Load(); known != nil {
		for i := range *known {
			if entry := &(*known)[i]; entry.fingerprint == fingerprint {
				return entry.alg.family, nil
			}
		}
	}

	t, err := buildZeroTable(fingerprint, fn)
	if err != nil {
		return nil, err
	}
	family := &hashFamily{fingerprint: fingerprint, build: fn}
	family.table.Store(t)

	return family, nil
}

// algorithmEntry is an Algorithm AlgorithmFor returns, with what identifies
// its function: the code it runs and its hash of a pair of zero chunks. The
// code pointer alone does not, since every closure of one function literal
// shares it.
type algorithmEntry struct {
	code        uintptr
	fingerprint [32]byte
	alg         *Algorithm
}

// knownAlgorithms holds the built-in Algorithms and those AlgorithmFor has
// returned. The list is never changed in place: a longer copy replaces it.
var knownAlgorithms atomic.Pointer[[]algorithmEntry]

// addKnownAlgorithm appends an entry to the known Algorithms.
func addKnownAlgorithm(entry algorithmEntry) {
	for {
		known := knownAlgorithms.Load()
		var longer []algorithmEntry
		if known != nil {
			longer = append(make([]algorithmEntry, 0, len(*known)+1), *known...)
		}
		longer = append(longer, entry)
		if knownAlgorithms.CompareAndSwap(known, &longer) {
			return
		}
	}
}

// namedAlgorithm is an Algorithm an ssz-hash annotation can name.
type namedAlgorithm struct {
	name string
	alg  *Algorithm
}

// namedAlgorithms holds the registered names. The list is never changed in
// place: a longer copy replaces it.
var namedAlgorithms atomic.Pointer[[]namedAlgorithm]

// RegisterAlgorithm gives an Algorithm the name an ssz-hash annotation calls
// it by. "sha256" names SHA256. A name belongs to one Algorithm: registering
// it for another fails, registering it again for the same one does nothing.
// Register at package initialization, before a value is hashed.
func RegisterAlgorithm(name string, alg *Algorithm) error {
	if name == "" || alg == nil {
		return errors.New("a hash algorithm is registered with a name and an algorithm")
	}

	for {
		named := namedAlgorithms.Load()
		var longer []namedAlgorithm
		if named != nil {
			for i := range *named {
				if entry := &(*named)[i]; entry.name == name {
					if entry.alg == alg {
						return nil
					}
					return fmt.Errorf("hash algorithm name %q is registered for another algorithm", name)
				}
			}
			longer = append(make([]namedAlgorithm, 0, len(*named)+1), *named...)
		}
		longer = append(longer, namedAlgorithm{name: name, alg: alg})
		if namedAlgorithms.CompareAndSwap(named, &longer) {
			return nil
		}
	}
}

// AlgorithmByName returns the Algorithm registered under the given name.
func AlgorithmByName(name string) (*Algorithm, bool) {
	if named := namedAlgorithms.Load(); named != nil {
		for i := range *named {
			if entry := &(*named)[i]; entry.name == name {
				return entry.alg, true
			}
		}
	}

	return nil, false
}

// WithNamedHasher acquires a Hasher from the pool of the Algorithm registered
// under the given name, passes it to fn, and returns it to the pool when
// done. It fails when no Algorithm is registered under that name.
func WithNamedHasher(name string, fn func(hh sszutils.HashWalker) error) error {
	alg, ok := AlgorithmByName(name)
	if !ok {
		return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "no hash algorithm is registered as %q", name)
	}

	return alg.WithHasher(fn)
}

// AlgorithmFor returns the Algorithm that compresses with fn. fn hashes its
// input as consecutive 64-byte pairs into dst; it must be deterministic, safe
// for concurrent use and support in-place hashing, where dst aliases the
// front of the input.
//
// Functions that run the same code and compute the same hash give the same
// Algorithm, which keeps the first of them; the function of a built-in
// Algorithm gives that Algorithm. An Algorithm is kept for the life of the
// process. Finding it costs one call of fn, and 64 for a hash no Algorithm
// computes yet, whose zero subtree roots are computed then; a caller that
// hashes often keeps the Algorithm. An error fn reports in these calls is
// returned.
func AlgorithmFor(fn HashFn) (*Algorithm, error) {
	if fn == nil {
		return nil, errors.New("hash function must not be nil")
	}

	fingerprint, err := fingerprintOf(fn)
	if err != nil {
		return nil, err
	}
	code := reflect.ValueOf(fn).Pointer()

	var created *Algorithm
	for {
		known := knownAlgorithms.Load()
		for i := range *known {
			if entry := &(*known)[i]; entry.code == code && entry.fingerprint == fingerprint {
				return entry.alg, nil
			}
		}

		if created == nil {
			if created, err = newAlgorithm(fn, fingerprint); err != nil {
				return nil, err
			}
		}
		longer := append(make([]algorithmEntry, 0, len(*known)+1), *known...)
		longer = append(longer, algorithmEntry{code: code, fingerprint: fingerprint, alg: created})
		if knownAlgorithms.CompareAndSwap(known, &longer) {
			return created, nil
		}
	}
}

// newAlgorithm makes an Algorithm of its own for fn, whose hash of a pair of
// zero chunks is fingerprint.
func newAlgorithm(fn HashFn, fingerprint [32]byte) (*Algorithm, error) {
	family, err := familyOf(fn, fingerprint)
	if err != nil {
		return nil, err
	}
	alg := &Algorithm{fn: fn, pool: &HasherPool{HashFn: fn}, family: family}
	alg.pool.alg = alg
	alg.native = alg

	return alg, nil
}

// algorithmOfFn makes an Algorithm of its own for fn. Without a function it
// is one of SHA256. A function that fails while its hash is identified gives
// an Algorithm of no known hash that answers every compression with that
// failure; its family keeps the function, so it can be identified again.
func algorithmOfFn(fn HashFn) *Algorithm {
	family := sha256Family
	if fn != nil {
		fingerprint, err := fingerprintOf(fn)
		if err == nil {
			var alg *Algorithm
			if alg, err = newAlgorithm(fn, fingerprint); err == nil {
				return alg
			}
		}

		family = &hashFamily{build: fn, unknown: true}
		family.table.Store(&zeroTable{})
		fn = func(_, _ []byte) error { return err }
	}

	alg := &Algorithm{fn: fn, pool: &HasherPool{HashFn: fn}, family: family}
	alg.pool.alg = alg
	alg.native = alg

	return alg
}

// Compatible reports whether both Algorithms compute the same hash, so a tree
// has one root under either of them. Algorithms that differ only in their
// backend, as SHA256 and SHA256Native do, are compatible.
func (a *Algorithm) Compatible(other *Algorithm) bool {
	if other == nil {
		return false
	}

	return a.family == other.family || (!a.family.unknown && a.family.fingerprint == other.family.fingerprint)
}

// Native returns the pure Go counterpart of a built-in Algorithm, and the
// Algorithm itself where it has none.
func (a *Algorithm) Native() *Algorithm {
	return a.native
}

// Pool returns the pool of hashers that compress with the Algorithm.
func (a *Algorithm) Pool() *HasherPool {
	return a.pool
}

// HashFn returns the function the Algorithm compresses with.
func (a *Algorithm) HashFn() HashFn {
	if a.fn != nil {
		return a.fn
	}

	return a.pool.CurrentHashFn()
}

// zero returns the zero subtree roots of the Algorithm.
func (a *Algorithm) zero() *zeroTable {
	return a.family.zero()
}

// HashPair compresses one pair of chunks. Every SHA256 Algorithm computes it
// with the Go standard library, which keeps the caller's pair on its stack;
// the other hashes go through their function, which gets a copy.
func (a *Algorithm) HashPair(pair *[64]byte) ([32]byte, error) {
	if a.family == sha256Family {
		return sha256.Sum256(pair[:]), nil
	}
	p := *pair
	err := a.HashFn()(p[:32], p[:])

	return [32]byte(p[:32]), err
}

// ZeroHash returns a copy of the zero subtree root at the given depth; the
// caller owns it. Depths outside the supported range are clamped to the
// nearest valid level.
func (a *Algorithm) ZeroHash(depth int) []byte {
	if depth < 0 {
		depth = 0
	} else if depth >= zeroHashDepths {
		depth = zeroHashDepths - 1
	}

	return bytes.Clone(a.zero().hashes[depth][:])
}

// ZeroHashes returns the zero subtree roots of all 65 depths.
func (a *Algorithm) ZeroHashes() [65][32]byte {
	return a.zero().hashes
}

// ZeroHashTable returns the zero subtree roots of all 65 depths without
// copying them. The table is shared and must not be modified.
func (a *Algorithm) ZeroHashTable() *[65][32]byte {
	return &a.zero().hashes
}

// ZeroHashLevel returns the depth of the zero subtree with the given root.
// It reports false when hash is not a zero subtree root of the Algorithm.
func (a *Algorithm) ZeroHashLevel(hash []byte) (int, bool) {
	if len(hash) != 32 {
		return 0, false
	}
	level, ok := a.zero().levels[[32]byte(hash)]

	return level, ok
}

// WithHasher acquires a Hasher from the pool of the Algorithm, passes it to
// fn, and returns it to the pool when done.
func (a *Algorithm) WithHasher(fn func(hh sszutils.HashWalker) error) error {
	hh := a.pool.Get()
	defer a.pool.Put(hh)

	return fn(hh)
}

// AlgorithmWalker is implemented by hash walkers that report the Algorithm
// they compress with. A walker without it compresses with SHA256.
type AlgorithmWalker interface {
	Algorithm() *Algorithm
}

// AlgorithmOf returns the Algorithm the given walker compresses with.
func AlgorithmOf(hw sszutils.HashWalker) *Algorithm {
	if aw, ok := hw.(AlgorithmWalker); ok {
		return aw.Algorithm()
	}

	return SHA256
}
