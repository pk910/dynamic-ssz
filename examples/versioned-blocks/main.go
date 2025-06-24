// Package main demonstrates versioned block handling patterns with dynamic-ssz.
// This example shows how to handle different Ethereum fork versions efficiently,
// similar to patterns used in production indexers like Dora.
package main

import (
	"fmt"
	"log"

	"github.com/attestantio/go-eth2-client/spec"
	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/electra"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	dynssz "github.com/pk910/dynamic-ssz"
)

// MarshalVersionedSignedBeaconBlockSSZ demonstrates how to handle different block versions
// This pattern is used by production indexers for efficient block storage
func MarshalVersionedSignedBeaconBlockSSZ(dynSsz *dynssz.DynSsz, block *spec.VersionedSignedBeaconBlock) (version uint64, ssz []byte, err error) {
	switch block.Version {
	case spec.DataVersionPhase0:
		version = uint64(block.Version)
		ssz, err = dynSsz.MarshalSSZ(block.Phase0)
	case spec.DataVersionAltair:
		version = uint64(block.Version)
		ssz, err = dynSsz.MarshalSSZ(block.Altair)
	case spec.DataVersionBellatrix:
		version = uint64(block.Version)
		ssz, err = dynSsz.MarshalSSZ(block.Bellatrix)
	case spec.DataVersionCapella:
		version = uint64(block.Version)
		ssz, err = dynSsz.MarshalSSZ(block.Capella)
	case spec.DataVersionDeneb:
		version = uint64(block.Version)
		ssz, err = dynSsz.MarshalSSZ(block.Deneb)
	case spec.DataVersionElectra:
		version = uint64(block.Version)
		ssz, err = dynSsz.MarshalSSZ(block.Electra)
	default:
		err = fmt.Errorf("unknown block version: %v", block.Version)
	}
	return
}

// UnmarshalVersionedSignedBeaconBlockSSZ demonstrates version-aware unmarshaling
func UnmarshalVersionedSignedBeaconBlockSSZ(dynSsz *dynssz.DynSsz, version uint64, ssz []byte) (*spec.VersionedSignedBeaconBlock, error) {
	block := &spec.VersionedSignedBeaconBlock{
		Version: spec.DataVersion(version),
	}

	switch block.Version {
	case spec.DataVersionPhase0:
		block.Phase0 = &phase0.SignedBeaconBlock{}
		if err := dynSsz.UnmarshalSSZ(block.Phase0, ssz); err != nil {
			return nil, fmt.Errorf("failed to decode phase0 signed beacon block: %v", err)
		}
	case spec.DataVersionAltair:
		block.Altair = &altair.SignedBeaconBlock{}
		if err := dynSsz.UnmarshalSSZ(block.Altair, ssz); err != nil {
			return nil, fmt.Errorf("failed to decode altair signed beacon block: %v", err)
		}
	case spec.DataVersionBellatrix:
		block.Bellatrix = &bellatrix.SignedBeaconBlock{}
		if err := dynSsz.UnmarshalSSZ(block.Bellatrix, ssz); err != nil {
			return nil, fmt.Errorf("failed to decode bellatrix signed beacon block: %v", err)
		}
	case spec.DataVersionCapella:
		block.Capella = &capella.SignedBeaconBlock{}
		if err := dynSsz.UnmarshalSSZ(block.Capella, ssz); err != nil {
			return nil, fmt.Errorf("failed to decode capella signed beacon block: %v", err)
		}
	case spec.DataVersionDeneb:
		block.Deneb = &deneb.SignedBeaconBlock{}
		if err := dynSsz.UnmarshalSSZ(block.Deneb, ssz); err != nil {
			return nil, fmt.Errorf("failed to decode deneb signed beacon block: %v", err)
		}
	case spec.DataVersionElectra:
		block.Electra = &electra.SignedBeaconBlock{}
		if err := dynSsz.UnmarshalSSZ(block.Electra, ssz); err != nil {
			return nil, fmt.Errorf("failed to decode electra signed beacon block: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown block version: %v", block.Version)
	}
	return block, nil
}

// InitializeDynSszFromChainState demonstrates spec initialization from chain state
func InitializeDynSszFromChainState(chainSpecs map[string]any) (*dynssz.DynSsz, error) {
	// In a real application, these would come from your beacon chain client
	if chainSpecs == nil {
		chainSpecs = map[string]any{
			"SLOTS_PER_HISTORICAL_ROOT":    uint64(8192),
			"SYNC_COMMITTEE_SIZE":          uint64(512),
			"MAX_VALIDATORS_PER_COMMITTEE": uint64(2048),
			"EPOCHS_PER_HISTORICAL_VECTOR": uint64(65536),
			"EPOCHS_PER_SLASHINGS_VECTOR":  uint64(8192),
			"MAX_ATTESTATIONS":             uint64(128),
			"MAX_DEPOSITS":                 uint64(16),
			"MAX_VOLUNTARY_EXITS":          uint64(16),
			"MAX_PROPOSER_SLASHINGS":       uint64(16),
			"MAX_ATTESTER_SLASHINGS":       uint64(2),
		}
	}

	return dynssz.NewDynSsz(chainSpecs), nil
}

func main() {
	fmt.Println("Dynamic SSZ Versioned Blocks Example")
	fmt.Println("===================================")

	// Initialize DynSsz from chain state specifications
	dynSsz, err := InitializeDynSszFromChainState(nil)
	if err != nil {
		log.Fatal("Failed to initialize DynSsz:", err)
	}

	// Example 1: Phase0 Block
	fmt.Println("\n1. Phase0 Block Example:")
	phase0Block := &phase0.SignedBeaconBlock{
		Message: &phase0.BeaconBlock{
			Slot:          12345,
			ProposerIndex: 42,
			ParentRoot:    [32]byte{1, 2, 3},
			StateRoot:     [32]byte{4, 5, 6},
			Body: &phase0.BeaconBlockBody{
				RANDAOReveal: [96]byte{7, 8, 9},
				ETH1Data: &phase0.ETH1Data{
					DepositRoot:  [32]byte{10, 11, 12},
					DepositCount: 100,
					BlockHash:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
				},
				Graffiti:          [32]byte{16, 17, 18},
				ProposerSlashings: []*phase0.ProposerSlashing{},
				AttesterSlashings: []*phase0.AttesterSlashing{},
				Attestations:      []*phase0.Attestation{},
				Deposits:          []*phase0.Deposit{},
				VoluntaryExits:    []*phase0.SignedVoluntaryExit{},
			},
		},
		Signature: [96]byte{19, 20, 21},
	}

	versionedBlock := &spec.VersionedSignedBeaconBlock{
		Version: spec.DataVersionPhase0,
		Phase0:  phase0Block,
	}

	// Marshal the versioned block
	version, ssz, err := MarshalVersionedSignedBeaconBlockSSZ(dynSsz, versionedBlock)
	if err != nil {
		log.Fatal("Failed to marshal block:", err)
	}
	fmt.Printf("Marshaled phase0 block - version: %d, size: %d bytes\n", version, len(ssz))

	// Unmarshal it back
	decoded, err := UnmarshalVersionedSignedBeaconBlockSSZ(dynSsz, version, ssz)
	if err != nil {
		log.Fatal("Failed to unmarshal block:", err)
	}
	fmt.Printf("Decoded block - version: %v, slot: %d\n",
		decoded.Version, decoded.Phase0.Message.Slot)

	// Example 2: Altair Block
	fmt.Println("\n2. Altair Block Example:")
	altairBlock := &altair.SignedBeaconBlock{
		Message: &altair.BeaconBlock{
			Slot:          23456,
			ProposerIndex: 84,
			ParentRoot:    [32]byte{21, 22, 23},
			StateRoot:     [32]byte{24, 25, 26},
			Body: &altair.BeaconBlockBody{
				RANDAOReveal: [96]byte{27, 28, 29},
				ETH1Data: &phase0.ETH1Data{
					DepositRoot:  [32]byte{30, 31, 32},
					DepositCount: 200,
					BlockHash:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
				},
				Graffiti:          [32]byte{36, 37, 38},
				ProposerSlashings: []*phase0.ProposerSlashing{},
				AttesterSlashings: []*phase0.AttesterSlashing{},
				Attestations:      []*phase0.Attestation{},
				Deposits:          []*phase0.Deposit{},
				VoluntaryExits:    []*phase0.SignedVoluntaryExit{},
				SyncAggregate: &altair.SyncAggregate{
					SyncCommitteeBits: []byte{
						0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
						0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
						0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f,
						0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f,
					},
					SyncCommitteeSignature: [96]byte{39, 40, 41},
				},
			},
		},
		Signature: [96]byte{42, 43, 44},
	}

	altairVersionedBlock := &spec.VersionedSignedBeaconBlock{
		Version: spec.DataVersionAltair,
		Altair:  altairBlock,
	}

	version, ssz, err = MarshalVersionedSignedBeaconBlockSSZ(dynSsz, altairVersionedBlock)
	if err != nil {
		log.Fatal("Failed to marshal altair block:", err)
	}
	fmt.Printf("Marshaled altair block - version: %d, size: %d bytes\n", version, len(ssz))

	decoded, err = UnmarshalVersionedSignedBeaconBlockSSZ(dynSsz, version, ssz)
	if err != nil {
		log.Fatal("Failed to unmarshal altair block:", err)
	}
	fmt.Printf("Decoded altair block - version: %v, slot: %d\n",
		decoded.Version, decoded.Altair.Message.Slot)

	// Example 3: Performance comparison
	fmt.Println("\n3. Performance Comparison:")
	iterations := 1000

	// Test serialization performance
	start := make([]byte, 0, 1024)
	for i := 0; i < iterations; i++ {
		versionedBlock.Phase0.Message.Slot = phase0.Slot(12345 + i)
		_, _, err = MarshalVersionedSignedBeaconBlockSSZ(dynSsz, versionedBlock)
		if err != nil {
			log.Fatal("Performance test failed:", err)
		}
		start = start[:0] // Reset for reuse
	}
	fmt.Printf("Successfully processed %d blocks\n", iterations)

	// Example 4: Hash tree root calculation
	fmt.Println("\n4. Hash Tree Root Example:")
	root, err := dynSsz.HashTreeRoot(phase0Block)
	if err != nil {
		log.Fatal("Failed to calculate hash tree root:", err)
	}
	fmt.Printf("Phase0 block hash tree root: %x\n", root)

	altairRoot, err := dynSsz.HashTreeRoot(altairBlock)
	if err != nil {
		log.Fatal("Failed to calculate altair hash tree root:", err)
	}
	fmt.Printf("Altair block hash tree root: %x\n", altairRoot)

	fmt.Println("\nVersioned blocks example completed successfully!")
}
