// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

// Package main demonstrates Merkle tree construction and proof generation
// with dynamic-ssz's treeproof package.
//
// This is the pattern used to prove validator data out of the beacon state
// for on-chain verification: build the state tree once with GetTree, derive
// generalized indices for the fields to prove, generate proofs with
// Prove/ProveMulti, and sanity-check each proof leaf against the field's own
// hash tree root before trusting it.
package main

import (
	"bytes"
	"fmt"
	"log"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/treeproof"
)

// BeaconBlockHeader mirrors the consensus-spec block header container.
type BeaconBlockHeader struct {
	Slot          uint64
	ProposerIndex uint64
	ParentRoot    [32]byte
	StateRoot     [32]byte
	BodyRoot      [32]byte
}

// Validator mirrors the consensus-spec validator container (shortened).
type Validator struct {
	Pubkey                [48]byte
	WithdrawalCredentials [32]byte
	EffectiveBalance      uint64
	Slashed               bool
	ActivationEpoch       uint64
	ExitEpoch             uint64
}

// Checkpoint is a simplified epoch checkpoint.
type Checkpoint struct {
	Epoch uint64
	Root  [32]byte
}

// Generalized-index layout constants for BeaconState. With 8 fields the
// container merkleizes into 8 chunks, so field i sits at generalized index
// 8+i. The real beacon state works the same way with 2^5..2^6 fields
// depending on the fork.
const (
	stateChunkCeil       = 8    // next power of two >= field count
	slotFieldIndex       = 1    // BeaconState.Slot
	validatorsFieldIndex = 5    // BeaconState.Validators
	validatorsLimit      = 1024 // must match the ssz-max tag
)

// BeaconState is a reduced beacon-state-like container: exactly 8 fields, so
// the generalized-index math stays easy to follow.
type BeaconState struct {
	GenesisTime         uint64
	Slot                uint64
	LatestBlockHeader   BeaconBlockHeader
	BlockRoots          [][32]byte  `ssz-size:"64,32"`
	StateRoots          [][32]byte  `ssz-size:"64,32"`
	Validators          []Validator `ssz-max:"1024"`
	Balances            []uint64    `ssz-max:"1024"`
	FinalizedCheckpoint Checkpoint
}

func main() {
	fmt.Println("Merkle Proofs Example — prove fields out of an SSZ structure")
	fmt.Println("=============================================================")

	ds := dynssz.NewDynSsz(nil)
	state := buildState(100)

	// Build the complete Merkle tree once, then generate as many proofs from
	// it as needed. Rebuilding the tree per proof is the expensive mistake.
	tree, err := ds.GetTree(state)
	if err != nil {
		log.Fatal("Failed to build tree: ", err)
	}

	stateRoot, err := ds.HashTreeRoot(state)
	if err != nil {
		log.Fatal("Failed to hash state: ", err)
	}

	fmt.Printf("\nstate root:     0x%x\n", stateRoot)
	fmt.Printf("tree root hash: 0x%x (identical)\n", tree.Hash())

	// The tree structure determines the generalized indices. Show the top
	// levels — the 8 field subtrees sit at indices 8..15.
	fmt.Println("\n1. Tree structure (top 3 levels):")
	tree.Show(3)

	// 2. Prove a simple field: Slot is field 1, so its leaf lives at
	// generalized index 8+1 = 9.
	fmt.Println("\n2. Prove the Slot field (generalized index 9):")

	slotProof, err := tree.Prove(stateChunkCeil + slotFieldIndex)
	if err != nil {
		log.Fatal("Failed to prove slot: ", err)
	}

	fmt.Printf("  leaf (little-endian slot): 0x%x\n", slotProof.Leaf[:8])
	fmt.Printf("  sibling hashes: %d\n", len(slotProof.Hashes))

	valid, err := treeproof.VerifyProof(tree.Hash(), slotProof)
	if err != nil {
		log.Fatal("Failed to verify slot proof: ", err)
	}

	fmt.Printf("  verified against state root: %v\n", valid)

	// 3. Prove a single validator inside the Validators list. The
	// generalized index is derived step by step, the same walk used to
	// prove validators out of the real beacon state.
	validatorIndex := uint64(42)

	fmt.Printf("\n3. Prove validator %d inside the Validators list:\n", validatorIndex)

	gid := uint64(1)                                // start at the root
	gid = gid*stateChunkCeil + validatorsFieldIndex // descend to the Validators field
	gid *= 2                                        // lists mix in their length: data subtree is the left child
	gid = gid*validatorsLimit + validatorIndex      // descend to element 42 within the 1024-leaf data subtree

	fmt.Printf("  generalized index: 1 → %d (field) → %d (list data) → %d (element)\n",
		stateChunkCeil+validatorsFieldIndex, (stateChunkCeil+validatorsFieldIndex)*2, gid)

	validatorProof, err := tree.Prove(int(gid))
	if err != nil {
		log.Fatal("Failed to prove validator: ", err)
	}

	// Sanity-check the proof leaf against the validator's own hash tree root
	// before using it — always do this for proofs that get submitted
	// on-chain.
	validatorRoot, err := ds.HashTreeRoot(&state.Validators[validatorIndex])
	if err != nil {
		log.Fatal("Failed to hash validator: ", err)
	}

	if !bytes.Equal(validatorProof.Leaf, validatorRoot[:]) {
		log.Fatal("proof leaf does not match validator root")
	}

	fmt.Printf("  proof leaf matches HashTreeRoot(validator[%d]): true\n", validatorIndex)
	fmt.Printf("  proof depth: %d sibling hashes\n", len(validatorProof.Hashes))

	valid, err = treeproof.VerifyProof(tree.Hash(), validatorProof)
	if err != nil {
		log.Fatal("Failed to verify validator proof: ", err)
	}

	fmt.Printf("  verified against state root: %v\n", valid)

	// 4. Multiproof: prove several fields at once with shared sibling
	// hashes — cheaper to transmit and verify than independent proofs.
	fmt.Println("\n4. Multiproof over GenesisTime, Slot and FinalizedCheckpoint:")

	indices := []int{
		stateChunkCeil + 0, // GenesisTime
		stateChunkCeil + 1, // Slot
		stateChunkCeil + 7, // FinalizedCheckpoint
	}

	multiproof, err := tree.ProveMulti(indices)
	if err != nil {
		log.Fatal("Failed to build multiproof: ", err)
	}

	fmt.Printf("  %d leaves proven with %d shared hashes ", len(multiproof.Leaves), len(multiproof.Hashes))
	fmt.Printf("(three single proofs would need %d)\n", 3*len(slotProof.Hashes))

	valid, err = treeproof.VerifyMultiproof(tree.Hash(), multiproof.Hashes, multiproof.Leaves, multiproof.Indices)
	if err != nil {
		log.Fatal("Failed to verify multiproof: ", err)
	}

	fmt.Printf("  multiproof verified: %v\n", valid)

	// 5. Verification is standalone: root + proof, no tree, no original
	// data. This is what runs on the other side (a contract, a light
	// client, another process).
	fmt.Println("\n5. Standalone verification:")

	root := tree.Hash()
	tree = nil // the tree can be dropped now — for large states, nil it out to release memory early

	_ = tree

	valid, err = treeproof.VerifyProof(root, validatorProof)
	if err != nil {
		log.Fatal("Failed to verify: ", err)
	}

	fmt.Printf("  validator proof verified with only the root and the proof: %v\n", valid)
}

func buildState(validatorCount int) *BeaconState {
	state := &BeaconState{
		GenesisTime: 1606824023,
		Slot:        123456,
		LatestBlockHeader: BeaconBlockHeader{
			Slot:          123455,
			ProposerIndex: 7,
		},
		BlockRoots: make([][32]byte, 64),
		StateRoots: make([][32]byte, 64),
		Validators: make([]Validator, 0, validatorCount),
		Balances:   make([]uint64, 0, validatorCount),
		FinalizedCheckpoint: Checkpoint{
			Epoch: 3858,
			Root:  [32]byte{0x01},
		},
	}

	for i := 0; i < validatorCount; i++ {
		validator := Validator{
			EffectiveBalance: 32_000_000_000,
			ExitEpoch:        ^uint64(0),
		}
		validator.Pubkey[0] = byte(i)
		validator.WithdrawalCredentials[0] = 0x01

		state.Validators = append(state.Validators, validator)
		state.Balances = append(state.Balances, 32_000_000_000+uint64(i))
	}

	return state
}
