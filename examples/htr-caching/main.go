// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

// Package main demonstrates application-level hash-tree-root caching on top of
// dynamic-ssz.
//
// dynssz deliberately does not cache hash tree roots itself: cache lifetime and
// invalidation depend on application state transitions, so they belong to the
// application. What the library provides is the sszutils.DynamicHashRoot
// delegation hook — any type implementing HashTreeRootWithDyn takes over its
// own merkleization.
//
// This example uses that hook on a large validator registry. Each list element
// is a thin wrapper holding the actual validator plus a cached root pointer
// that is excluded from the SSZ schema. On the first full-registry hash every
// element is hashed and its root persisted; subsequent hashes only recompute
// elements whose cache was invalidated via MarkDirty. The per-element work is
// skipped, while the list-level merkleization over the element roots still
// runs every time.
package main

import (
	"bytes"
	"fmt"
	"log"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// FarFutureEpoch marks a validator with no scheduled exit.
const FarFutureEpoch = ^uint64(0)

// Validator mirrors the consensus-spec validator container.
type Validator struct {
	Pubkey                     [48]byte
	WithdrawalCredentials      [32]byte
	EffectiveBalance           uint64
	Slashed                    bool
	ActivationEligibilityEpoch uint64
	ActivationEpoch            uint64
	ExitEpoch                  uint64
	WithdrawableEpoch          uint64
}

// CachedValidator wraps a Validator with a cached hash tree root. Root is
// excluded from the SSZ schema via ssz-type:"-", so the wrapper serializes to
// exactly the same bytes as a bare Validator. A nil Root means the cache is
// stale and the next hash recomputes it.
//
// The wrapper is also transparent for merkleization: an SSZ container with a
// single field merkleizes to that field's root, so contributing Data's root as
// the wrapper's root yields the same registry root as a list of bare
// validators.
type CachedValidator struct {
	Data Validator
	Root *[32]byte `ssz-type:"-"`
}

var _ = sszutils.Annotate[CachedValidator](`ssz-type:"wrapper"`)
var _ sszutils.DynamicHashRoot = (*CachedValidator)(nil)

// HashTreeRootWithDyn implements sszutils.DynamicHashRoot. When the registry
// is hashed, dynssz calls this per element instead of walking the wrapper
// itself: a cached root is contributed directly, otherwise the wrapped data is
// hashed through the same walker and the resulting root persisted for
// subsequent calls.
func (v *CachedValidator) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	if v.Root != nil {
		hh.PutBytes(v.Root[:])
		return nil
	}

	d, ok := ds.(*dynssz.DynSsz)
	if !ok {
		return fmt.Errorf("expected *dynssz.DynSsz specs, got %T", ds)
	}

	// Hash the wrapped data in place at the wrapper's position, then capture
	// the root it left on the walker: immediately after a completed scope,
	// hh.Hash() returns that scope's root.
	if err := d.HashTreeRootWith(v.Data, hh); err != nil {
		return fmt.Errorf("failed hashing wrapped validator: %w", err)
	}

	var root [32]byte
	copy(root[:], hh.Hash())
	v.Root = &root

	return nil
}

// MarkDirty invalidates the cached root. Call it after mutating Data; the next
// registry hash recomputes this element only.
func (v *CachedValidator) MarkDirty() {
	v.Root = nil
}

// CachedRegistry is the registry variant with per-element root caching.
type CachedRegistry struct {
	Validators []*CachedValidator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

// PlainRegistry is the uncached reference used to verify that the caching
// wrapper never changes serialization or roots.
type PlainRegistry struct {
	Validators []*Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

const validatorCount = 100_000

func main() {
	fmt.Println("HTR Caching Example — cached validator registry")
	fmt.Println("===============================================")

	ds := dynssz.NewDynSsz(map[string]any{
		"VALIDATOR_REGISTRY_LIMIT": uint64(1099511627776),
	})

	// The wrapper embeds the validator by value, so each registry holds its
	// own copy. Mutations below are applied to both so the plain registry
	// always yields the reference root for the current data.
	cached := &CachedRegistry{Validators: make([]*CachedValidator, validatorCount)}
	plain := &PlainRegistry{Validators: make([]*Validator, validatorCount)}

	for i := range plain.Validators {
		validator := buildValidator(uint64(i))
		plain.Validators[i] = validator
		cached.Validators[i] = &CachedValidator{Data: *validator}
	}

	fmt.Printf("\n1. Cold hash (%d validators, empty cache):\n", validatorCount)

	start := time.Now()
	coldRoot, err := ds.HashTreeRoot(cached)
	if err != nil {
		log.Fatal("Failed to hash cached registry: ", err)
	}
	coldTime := time.Since(start)

	plainRoot, err := ds.HashTreeRoot(plain)
	if err != nil {
		log.Fatal("Failed to hash plain registry: ", err)
	}

	fmt.Printf("Cached registry root: %x (%v)\n", coldRoot, coldTime)
	fmt.Printf("Plain registry root:  %x\n", plainRoot)
	if coldRoot != plainRoot {
		log.Fatal("Roots diverge — the wrapper must be transparent")
	}

	fmt.Println("\n2. Warm hash (all element roots cached):")

	start = time.Now()
	warmRoot, err := ds.HashTreeRoot(cached)
	if err != nil {
		log.Fatal("Failed to hash cached registry: ", err)
	}
	warmTime := time.Since(start)

	if warmRoot != coldRoot {
		log.Fatal("Warm root diverges from cold root")
	}
	fmt.Printf("Same root in %v (%.1fx faster — only the list-level merkleization runs)\n",
		warmTime, float64(coldTime)/float64(warmTime))

	fmt.Println("\n3. Incremental update (mutate 25 validators, mark them dirty):")

	for i := range 25 {
		index := i * 4000
		plain.Validators[index].EffectiveBalance += 1_000_000_000
		cached.Validators[index].Data = *plain.Validators[index]
		cached.Validators[index].MarkDirty()
	}

	start = time.Now()
	updatedRoot, err := ds.HashTreeRoot(cached)
	if err != nil {
		log.Fatal("Failed to hash cached registry: ", err)
	}
	updateTime := time.Since(start)

	plainRoot, err = ds.HashTreeRoot(plain)
	if err != nil {
		log.Fatal("Failed to hash plain registry: ", err)
	}

	fmt.Printf("Updated root: %x (%v, 25 of %d elements re-hashed)\n",
		updatedRoot, updateTime, validatorCount)
	if updatedRoot != plainRoot {
		log.Fatal("Updated root diverges from uncached reference")
	}
	if updatedRoot == coldRoot {
		log.Fatal("Updated root should differ from the pre-update root")
	}

	fmt.Println("\n4. Pitfall — mutating without MarkDirty serves a stale root:")

	plain.Validators[0].Slashed = true
	cached.Validators[0].Data.Slashed = true

	staleRoot, err := ds.HashTreeRoot(cached)
	if err != nil {
		log.Fatal("Failed to hash cached registry: ", err)
	}
	fmt.Printf("Root after silent mutation:  %x (unchanged — stale cache)\n", staleRoot)
	if staleRoot != updatedRoot {
		log.Fatal("Expected the stale cached root to mask the mutation")
	}

	cached.Validators[0].MarkDirty()
	freshRoot, err := ds.HashTreeRoot(cached)
	if err != nil {
		log.Fatal("Failed to hash cached registry: ", err)
	}
	fmt.Printf("Root after MarkDirty:        %x (mutation now visible)\n", freshRoot)
	if freshRoot == staleRoot {
		log.Fatal("Expected MarkDirty to surface the mutation")
	}

	fmt.Println("\n5. Serialization is unaffected by the wrapper:")

	cachedBytes, err := ds.MarshalSSZ(cached)
	if err != nil {
		log.Fatal("Failed to marshal cached registry: ", err)
	}
	plainBytes, err := ds.MarshalSSZ(plain)
	if err != nil {
		log.Fatal("Failed to marshal plain registry: ", err)
	}
	if !bytes.Equal(cachedBytes, plainBytes) {
		log.Fatal("Cached and plain registries must serialize identically")
	}
	fmt.Printf("Cached and plain registries serialize to identical bytes (%d bytes)\n",
		len(cachedBytes))

	// A decoded registry starts with empty caches: the first hash after
	// unmarshaling is a cold hash that rebuilds them.
	var decoded CachedRegistry
	if err = ds.UnmarshalSSZ(&decoded, cachedBytes); err != nil {
		log.Fatal("Failed to unmarshal registry: ", err)
	}

	decodedRoot, err := ds.HashTreeRoot(&decoded)
	if err != nil {
		log.Fatal("Failed to hash decoded registry: ", err)
	}
	if decodedRoot != freshRoot {
		log.Fatal("Decoded registry root diverges")
	}
	fmt.Println("Round-tripped registry rebuilds its caches and yields the same root")

	fmt.Println("\nHTR caching example completed successfully!")
}

// buildValidator creates a deterministic validator so runs are reproducible.
func buildValidator(index uint64) *Validator {
	validator := &Validator{
		EffectiveBalance:           32_000_000_000,
		ActivationEligibilityEpoch: index / 1000,
		ActivationEpoch:            index/1000 + 1,
		ExitEpoch:                  FarFutureEpoch,
		WithdrawableEpoch:          FarFutureEpoch,
	}

	for i := range 8 {
		validator.Pubkey[i] = byte(index >> (8 * i))
	}
	validator.WithdrawalCredentials[0] = 0x01
	copy(validator.WithdrawalCredentials[24:], validator.Pubkey[:8])

	return validator
}
