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

// pairFn compresses one pair of chunks.
type pairFn func(pair *[64]byte) ([32]byte, error)

// zeroTable holds the zero subtree roots of one hash.
type zeroTable struct {
	hashes [zeroHashDepths][32]byte
	levels map[[32]byte]int
}

// Algorithm is a merkleization hash: a function that compresses two 32-byte
// chunks into one, with the zero subtree roots and the hasher pool derived
// from it. The tree shape is the same under every Algorithm; only the node
// hashes differ, so a root is meaningful only together with the hash that
// produced it.
//
// An Algorithm is safe for concurrent use. Its zero subtree roots are
// computed when they are first needed.
type Algorithm struct {
	pool   *HasherPool
	pair   pairFn
	native *Algorithm
	// fingerprint is the hash of a pair of zero chunks, which tells one
	// hash from another.
	fingerprint [32]byte
	// shares names the Algorithm whose zero subtree roots this one uses,
	// where it has none of its own.
	shares *Algorithm
	table  atomic.Pointer[zeroTable]
}

// SHA256 is the SSZ merkleization hash of the consensus specification,
// computed by the accelerated hashtree backend. Its pool is FastHasherPool.
var SHA256 = &Algorithm{pool: &FastHasherPool, pair: sha256Pair, fingerprint: sha256Fingerprint}

// SHA256Native is SHA256 computed by the Go standard library. Its pool is
// DefaultHasherPool.
var SHA256Native = &Algorithm{pool: &DefaultHasherPool, pair: sha256Pair, fingerprint: sha256Fingerprint, shares: SHA256}

// Keccak256 merkleizes with Ethereum Keccak-256 (padding 0x01, not NIST
// SHA3-256), computed by the accelerated keccaktree backend.
var Keccak256 = &Algorithm{
	pool:        &HasherPool{HashFn: keccaktree.HashByteSlice},
	pair:        pairOf(keccaktree.HashByteSlice),
	fingerprint: keccak256Fingerprint,
	shares:      Keccak256Native,
}

// Keccak256Native is Keccak256 computed in pure Go.
var Keccak256Native = &Algorithm{
	pool:        &HasherPool{HashFn: keccak256Native},
	pair:        pairOf(keccak256Native),
	fingerprint: keccak256Fingerprint,
}

// keccak256Native is the Keccak-256 compression in pure Go.
var keccak256Native = NativeHashWrapperFactory(sha3.NewLegacyKeccak256)

func init() {
	for _, twins := range [][2]*Algorithm{{SHA256, SHA256Native}, {Keccak256, Keccak256Native}} {
		for _, alg := range twins {
			alg.pool.alg = alg
			alg.native = twins[1]
		}
	}
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

func sha256Pair(pair *[64]byte) ([32]byte, error) {
	return sha256.Sum256(pair[:]), nil
}

// pairOf adapts a HashFn to the compression of a single pair.
func pairOf(fn HashFn) pairFn {
	return func(pair *[64]byte) (out [32]byte, err error) {
		err = fn(out[:], pair[:])
		return out, err
	}
}

// buildZeroTable computes the zero subtree roots above the given hash of a
// pair of zero chunks.
func buildZeroTable(fingerprint [32]byte, pair pairFn) (*zeroTable, error) {
	t := &zeroTable{levels: make(map[[32]byte]int, zeroHashDepths)}
	t.levels[t.hashes[0]] = 0
	t.hashes[1] = fingerprint
	t.levels[fingerprint] = 1

	var buf [64]byte
	var err error
	for i := 2; i < zeroHashDepths; i++ {
		copy(buf[:32], t.hashes[i-1][:])
		copy(buf[32:], t.hashes[i-1][:])
		if t.hashes[i], err = pair(&buf); err != nil {
			return nil, err
		}
		t.levels[t.hashes[i]] = i
	}

	return t, nil
}

// zero returns the zero subtree roots of the Algorithm.
func (a *Algorithm) zero() *zeroTable {
	if t := a.table.Load(); t != nil {
		return t
	}
	if a.shares != nil {
		t := a.shares.zero()
		a.table.Store(t)
		return t
	}

	// Only a built-in Algorithm comes without its table, and its pair does
	// not fail. Hashers that get here together each build the table; the
	// first one stored is the one all of them use.
	t, _ := buildZeroTable(a.fingerprint, a.pair)
	a.table.CompareAndSwap(nil, t)

	return a.table.Load()
}

// algorithmEntry is an Algorithm AlgorithmFor has returned, with what
// identifies its function: the code it runs and its hash of a pair of zero
// chunks. The code pointer alone does not, since every closure of one
// function literal shares it.
type algorithmEntry struct {
	code        uintptr
	fingerprint [32]byte
	alg         *Algorithm
}

// knownAlgorithms holds the Algorithms AlgorithmFor has returned. The list is
// never changed in place: a longer copy replaces it.
var knownAlgorithms atomic.Pointer[[]algorithmEntry]

// AlgorithmFor returns the Algorithm that compresses with fn. fn hashes its
// input as consecutive 64-byte pairs into dst; it must be safe for concurrent
// use and support in-place hashing, where dst aliases the front of the input.
//
// The same function gives the same Algorithm. Finding it costs one call of
// fn, so a caller that hashes often keeps the Algorithm. An error fn reports
// while it is probed is returned.
func AlgorithmFor(fn HashFn) (*Algorithm, error) {
	if fn == nil {
		return nil, errors.New("hash function must not be nil")
	}

	pair := pairOf(fn)
	var buf [64]byte
	fingerprint, err := pair(&buf)
	if err != nil {
		return nil, err
	}
	code := reflect.ValueOf(fn).Pointer()

	var created *Algorithm
	for {
		known := knownAlgorithms.Load()
		if known != nil {
			for i := range *known {
				if entry := &(*known)[i]; entry.code == code && entry.fingerprint == fingerprint {
					return entry.alg, nil
				}
			}
		}

		if created == nil {
			if created, err = newAlgorithm(fn, pair, fingerprint); err != nil {
				return nil, err
			}
		}
		var longer []algorithmEntry
		if known != nil {
			longer = append(make([]algorithmEntry, 0, len(*known)+1), *known...)
		}
		longer = append(longer, algorithmEntry{code: code, fingerprint: fingerprint, alg: created})
		if knownAlgorithms.CompareAndSwap(known, &longer) {
			return created, nil
		}
	}
}

// newAlgorithm makes an Algorithm of its own for fn, whose hash of a pair of
// zero chunks is fingerprint. A function that computes a built-in hash uses
// that hash's zero subtree roots.
func newAlgorithm(fn HashFn, pair pairFn, fingerprint [32]byte) (*Algorithm, error) {
	alg := &Algorithm{pool: &HasherPool{HashFn: fn}, pair: pair, fingerprint: fingerprint}
	alg.pool.alg = alg
	alg.native = alg

	switch fingerprint {
	case sha256Fingerprint:
		alg.shares = SHA256
	case keccak256Fingerprint:
		alg.shares = Keccak256Native
	default:
		t, err := buildZeroTable(fingerprint, pair)
		if err != nil {
			return nil, err
		}
		alg.table.Store(t)
	}

	return alg, nil
}

// algorithmOfFn makes an Algorithm of its own for fn. A function that cannot
// be probed, or none at all, is taken to compute SHA256.
func algorithmOfFn(fn HashFn) *Algorithm {
	if fn != nil {
		pair := pairOf(fn)
		var buf [64]byte
		if fingerprint, err := pair(&buf); err == nil {
			if alg, algErr := newAlgorithm(fn, pair, fingerprint); algErr == nil {
				return alg
			}
		}
	}

	alg := &Algorithm{pool: &HasherPool{HashFn: fn}, pair: sha256Pair, fingerprint: sha256Fingerprint, shares: SHA256}
	alg.pool.alg = alg
	alg.native = alg

	return alg
}

// Compatible reports whether both Algorithms compute the same hash, so a tree
// has one root under either of them. Algorithms that differ only in their
// backend, as SHA256 and SHA256Native do, are compatible.
func (a *Algorithm) Compatible(other *Algorithm) bool {
	return a == other || a.fingerprint == other.fingerprint
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
	return a.pool.CurrentHashFn()
}

// HashPair compresses one pair of chunks.
func (a *Algorithm) HashPair(pair *[64]byte) ([32]byte, error) {
	return a.pair(pair)
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
