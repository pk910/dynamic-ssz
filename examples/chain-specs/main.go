// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

// Package main demonstrates preset-aware SSZ serialization: one binary that
// produces spec-correct SSZ for mainnet, minimal, or any custom devnet preset.
//
// This is the core problem dynamic-ssz solves in production. Statically
// generated SSZ code bakes preset constants (vector lengths, list limits) into
// the generated methods, so a minimal-preset beacon state serializes and
// merkleizes to the wrong bytes/roots. Tools that must support devnets
// therefore load the chain config at runtime and pass it to dynssz.NewDynSsz.
//
// The example mirrors that workflow:
//  1. Load a beacon-chain config YAML into a spec map.
//  2. Annotate types with dual tags: ssz-size/ssz-max carry the mainnet
//     fallback, dynssz-size/dynssz-max carry the spec expression.
//  3. Serialize the same type under both presets and compare.
package main

import (
	_ "embed"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	dynssz "github.com/pk910/dynamic-ssz"
)

//go:embed specs/mainnet.yaml
var mainnetConfig []byte

//go:embed specs/minimal.yaml
var minimalConfig []byte

// Checkpoint is a simplified epoch checkpoint.
type Checkpoint struct {
	Epoch uint64
	Root  [32]byte
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

// SyncAggregate has a bitvector whose length is preset-dependent:
// SYNC_COMMITTEE_SIZE is 512 on mainnet (64 bytes) but 32 on minimal (4 bytes).
type SyncAggregate struct {
	SyncCommitteeBits      []byte `ssz-size:"64" dynssz-size:"SYNC_COMMITTEE_SIZE/8" ssz-type:"bitvector"`
	SyncCommitteeSignature [96]byte
}

// BeaconStateSnapshot is a reduced beacon-state-like container showing the
// dual-tag convention used for Ethereum consensus types:
// the ssz-* tag holds the mainnet value (used when no specs are provided or a
// spec value is missing), the dynssz-* tag holds the expression resolved
// against the runtime spec map. Expressions support +, -, *, / and
// parentheses, e.g. "(MIN_SEED_LOOKAHEAD+1)*SLOTS_PER_EPOCH".
type BeaconStateSnapshot struct {
	GenesisTime         uint64
	Slot                uint64
	FinalizedCheckpoint Checkpoint
	BlockRoots          [][32]byte   `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
	RandaoMixes         [][32]byte   `ssz-size:"65536,32" dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32"`
	Validators          []Validator  `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
	Balances            []uint64     `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
	ETH1DataVotes       []Checkpoint `ssz-max:"2048" dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH"`
	LatestSyncAggregate SyncAggregate
	ProposerLookahead   []uint64 `ssz-size:"64" dynssz-size:"(MIN_SEED_LOOKAHEAD+1)*SLOTS_PER_EPOCH"`
}

func main() {
	fmt.Println("Chain Specs Example — one binary, any preset")
	fmt.Println("=============================================")

	mainnetSpecs, err := loadSpecs(mainnetConfig)
	if err != nil {
		log.Fatal("Failed to load mainnet specs: ", err)
	}

	minimalSpecs, err := loadSpecs(minimalConfig)
	if err != nil {
		log.Fatal("Failed to load minimal specs: ", err)
	}

	// One DynSsz instance per spec set. Instances cache type descriptors, so
	// create them once and reuse them — either one per network, or a single
	// process-wide instance (see dynssz.SetGlobalSpecs / GetGlobalDynSsz).
	mainnetSsz := dynssz.NewDynSsz(mainnetSpecs)
	minimalSsz := dynssz.NewDynSsz(minimalSpecs)

	fmt.Println("\n1. Serialize the same type under both presets:")

	mainnetState := buildState(mainnetSpecs)
	mainnetBytes, err := mainnetSsz.MarshalSSZ(mainnetState)
	if err != nil {
		log.Fatal("Failed to marshal mainnet state: ", err)
	}

	mainnetRoot, err := mainnetSsz.HashTreeRoot(mainnetState)
	if err != nil {
		log.Fatal("Failed to hash mainnet state: ", err)
	}

	fmt.Printf("  mainnet: %8d bytes, root 0x%x\n", len(mainnetBytes), mainnetRoot[:8])

	minimalState := buildState(minimalSpecs)
	minimalBytes, err := minimalSsz.MarshalSSZ(minimalState)
	if err != nil {
		log.Fatal("Failed to marshal minimal state: ", err)
	}

	minimalRoot, err := minimalSsz.HashTreeRoot(minimalState)
	if err != nil {
		log.Fatal("Failed to hash minimal state: ", err)
	}

	fmt.Printf("  minimal: %8d bytes, root 0x%x\n", len(minimalBytes), minimalRoot[:8])
	fmt.Println("  The vectors shrink from 8192/65536 entries to 64/64 — same Go type,")
	fmt.Println("  completely different wire layout and hash tree root.")

	fmt.Println("\n2. The ssz-* tags are the mainnet fallback:")

	// With no specs at all, the static ssz-size/ssz-max values apply — which
	// are exactly the mainnet preset. The encoding matches the mainnet
	// instance byte for byte, so mainnet users pay no spec-resolution cost.
	staticSsz := dynssz.NewDynSsz(nil)

	staticBytes, err := staticSsz.MarshalSSZ(mainnetState)
	if err != nil {
		log.Fatal("Failed to marshal with static fallback: ", err)
	}

	fmt.Printf("  NewDynSsz(nil) encoding matches mainnet encoding: %v\n", string(staticBytes) == string(mainnetBytes))

	fmt.Println("\n3. Preset mismatches are detected, not silently mis-decoded:")

	var wrongPreset BeaconStateSnapshot
	if err := mainnetSsz.UnmarshalSSZ(&wrongPreset, minimalBytes); err != nil {
		fmt.Printf("  decoding minimal bytes with the mainnet instance fails: %v\n", firstLine(err.Error()))
	} else {
		log.Fatal("expected preset mismatch to fail decoding")
	}

	var roundTrip BeaconStateSnapshot
	if err := minimalSsz.UnmarshalSSZ(&roundTrip, minimalBytes); err != nil {
		log.Fatal("Failed to unmarshal minimal state: ", err)
	}

	fmt.Printf("  decoding with the minimal instance succeeds: %d validators, %d-byte sync bits\n",
		len(roundTrip.Validators), len(roundTrip.LatestSyncAggregate.SyncCommitteeBits))

	fmt.Println("\n4. Spec expressions resolved at runtime:")
	fmt.Printf("  SYNC_COMMITTEE_SIZE/8:               mainnet %3d bytes, minimal %2d bytes\n",
		len(mainnetState.LatestSyncAggregate.SyncCommitteeBits), len(minimalState.LatestSyncAggregate.SyncCommitteeBits))
	fmt.Printf("  (MIN_SEED_LOOKAHEAD+1)*SLOTS_PER_EPOCH: mainnet %3d slots, minimal %2d slots\n",
		len(mainnetState.ProposerLookahead), len(minimalState.ProposerLookahead))
}

// buildState creates a state whose preset-dependent fields are sized from the
// loaded spec map — the same values dynssz resolves the tag expressions
// against, so the constructed value always matches what the tags expect.
func buildState(specs map[string]any) *BeaconStateSnapshot {
	slotsPerEpoch := specUint(specs, "SLOTS_PER_EPOCH")
	minSeedLookahead := specUint(specs, "MIN_SEED_LOOKAHEAD")

	state := &BeaconStateSnapshot{
		GenesisTime: 1606824023,
		Slot:        123456,
		FinalizedCheckpoint: Checkpoint{
			Epoch: 3858,
			Root:  [32]byte{0x01},
		},
		BlockRoots:        make([][32]byte, specUint(specs, "SLOTS_PER_HISTORICAL_ROOT")),
		RandaoMixes:       make([][32]byte, specUint(specs, "EPOCHS_PER_HISTORICAL_VECTOR")),
		ProposerLookahead: make([]uint64, (minSeedLookahead+1)*slotsPerEpoch),
	}

	state.LatestSyncAggregate.SyncCommitteeBits = make([]byte, specUint(specs, "SYNC_COMMITTEE_SIZE")/8)

	validatorCount := 128
	state.Validators = make([]Validator, 0, validatorCount)
	state.Balances = make([]uint64, 0, validatorCount)

	for i := 0; i < validatorCount; i++ {
		validator := Validator{
			EffectiveBalance: 32_000_000_000,
			ActivationEpoch:  0,
			ExitEpoch:        ^uint64(0),
		}
		validator.Pubkey[0] = byte(i)

		state.Validators = append(state.Validators, validator)
		state.Balances = append(state.Balances, 32_000_000_000+uint64(i))
	}

	return state
}

// loadSpecs parses a beacon-chain config YAML into the spec map format dynssz
// expects: integers become uint64, 0x-prefixed strings become []byte,
// everything else stays a string.
func loadSpecs(configYaml []byte) (map[string]any, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(configYaml, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config yaml: %w", err)
	}

	specs := make(map[string]any, len(raw))

	for name, value := range raw {
		switch typedValue := value.(type) {
		case int:
			specs[name] = uint64(typedValue)
		case int64:
			specs[name] = uint64(typedValue)
		case uint64:
			specs[name] = typedValue
		case string:
			if strings.HasPrefix(typedValue, "0x") {
				specs[name] = hexToBytes(typedValue)
			} else if parsed, err := strconv.ParseUint(typedValue, 10, 64); err == nil {
				specs[name] = parsed
			} else {
				specs[name] = typedValue
			}
		default:
			specs[name] = value
		}
	}

	return specs, nil
}

func specUint(specs map[string]any, name string) uint64 {
	value, ok := specs[name].(uint64)
	if !ok {
		log.Fatalf("spec value %s missing or not a uint64", name)
	}

	return value
}

func hexToBytes(hexString string) []byte {
	hexString = strings.TrimPrefix(hexString, "0x")

	result := make([]byte, len(hexString)/2)
	for i := 0; i < len(result); i++ {
		parsed, err := strconv.ParseUint(hexString[i*2:i*2+2], 16, 8)
		if err != nil {
			log.Fatalf("invalid hex value %q: %v", hexString, err)
		}

		result[i] = byte(parsed)
	}

	return result
}

func firstLine(message string) string {
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		return message[:idx]
	}

	return message
}
