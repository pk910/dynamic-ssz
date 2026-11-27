// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"reflect"
	"sync/atomic"

	"github.com/pk910/dynamic-ssz/sszutils"
	"github.com/pk910/keccaktree"
	"golang.org/x/crypto/sha3"
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

var (
	sha256Family    = &hashFamily{fingerprint: sha256Fingerprint, build: defaultHash}
	keccak256Family = &hashFamily{fingerprint: keccak256Fingerprint, build: keccak256Native}
)

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

// Keccak256 merkleizes with Ethereum Keccak-256 (padding 0x01, not NIST
// SHA3-256), computed by the accelerated keccaktree backend.
var Keccak256 = &Algorithm{
	fn:     keccaktree.HashByteSlice,
	pool:   &HasherPool{HashFn: keccaktree.HashByteSlice},
	family: keccak256Family,
}

// Keccak256Native is Keccak256 computed in pure Go.
var Keccak256Native = &Algorithm{
	fn:     keccak256Native,
	pool:   &HasherPool{HashFn: keccak256Native},
	family: keccak256Family,
}

// keccak256Native is the Keccak-256 compression in pure Go.
var keccak256Native = NativeHashWrapperFactory(sha3.NewLegacyKeccak256)

func init() {
	known := make([]algorithmEntry, 0, 4)
	for _, twins := range [][2]*Algorithm{{SHA256, SHA256Native}, {Keccak256, Keccak256Native}} {
		for _, alg := range twins {
			alg.pool.alg = alg
			alg.native = twins[1]
			known = append(known, algorithmEntry{
				code:        reflect.ValueOf(alg.HashFn()).Pointer(),
				fingerprint: alg.family.fingerprint,
				alg:         alg,
			})
		}
	}
	knownAlgorithms.Store(&known)
}

// The hash of a pair of zero chunks under the built-in hashes: what tells a
// function that computes one of them from any other.
var (
	sha256Fingerprint = [32]byte{
		0xf5, 0xa5, 0xfd, 0x42, 0xd1, 0x6a, 0x20, 0x30, 0x27, 0x98, 0xef, 0x6e, 0xd3, 0x09, 0x97, 0x9b,
		0x43, 0x00, 0x3d, 0x23, 0x20, 0xd9, 0xf0, 0xe8, 0xea, 0x98, 0x31, 0xa9, 0x27, 0x59, 0xfb, 0x4b,
	}
	keccak256Fingerprint = [32]byte{
		0xad, 0x32, 0x28, 0xb6, 0x76, 0xf7, 0xd3, 0xcd, 0x42, 0x84, 0xa5, 0x44, 0x3f, 0x17, 0xf1, 0x96,
		0x2b, 0x36, 0xe4, 0x91, 0xb3, 0x0a, 0x40, 0xb2, 0x40, 0x58, 0x49, 0xe5, 0x97, 0xba, 0x5f, 0xb5,
	}
)

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
// fn computes. The zero subtree roots of a hash that is not built in are
// computed here, so a function that fails reports it.
func familyOf(fn HashFn, fingerprint [32]byte) (*hashFamily, error) {
	switch fingerprint {
	case sha256Fingerprint:
		return sha256Family, nil
	case keccak256Fingerprint:
		return keccak256Family, nil
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
			// A hash another Algorithm computes already has its family.
			for i := range *known {
				if entry := &(*known)[i]; entry.fingerprint == fingerprint {
					created.family = entry.alg.family
					break
				}
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
