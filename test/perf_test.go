package main

import (
	"io/ioutil"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/deneb"
	ssz "github.com/pk910/dynamic-ssz"
)

var (
	// Test data loaded once
	blockMainnetData []byte
	stateMainnetData []byte
	blockMinimalData []byte
	stateMinimalData []byte

	// SSZ instances
	dynSszMainnet     *ssz.DynSsz
	dynSszMinimal     *ssz.DynSsz
	dynSszOnlyMainnet *ssz.DynSsz
	dynSszOnlyMinimal *ssz.DynSsz
)

func init() {
	// Load test data
	blockMainnetData, _ = ioutil.ReadFile("../temp/block-mainnet.ssz")
	stateMainnetData, _ = ioutil.ReadFile("../temp/state-mainnet.ssz")
	blockMinimalData, _ = ioutil.ReadFile("../temp/block-minimal.ssz")
	stateMinimalData, _ = ioutil.ReadFile("../temp/state-minimal.ssz")

	// Minimal preset properties
	minimalSpecs := map[string]any{
		"SLOTS_PER_EPOCH":                uint64(8),
		"SYNC_COMMITTEE_SIZE":            uint64(32),
		"SYNC_COMMITTEE_SUBNET_COUNT":    uint64(4),
		"EPOCHS_PER_HISTORICAL_VECTOR":   uint64(64),
		"EPOCHS_PER_SLASHINGS_VECTOR":    uint64(64),
		"SLOTS_PER_HISTORICAL_ROOT":      uint64(64),
		"EPOCHS_PER_ETH1_VOTING_PERIOD":  uint64(4),
		"MAX_BLOB_COMMITMENTS_PER_BLOCK": uint64(16),
		"MAX_WITHDRAWALS_PER_PAYLOAD":    uint64(4),
	}

	// Create SSZ instances
	dynSszMainnet = ssz.NewDynSsz(nil)
	dynSszMinimal = ssz.NewDynSsz(minimalSpecs)
	dynSszOnlyMainnet = ssz.NewDynSsz(nil)
	dynSszOnlyMinimal = ssz.NewDynSsz(minimalSpecs)

	// Disable fastssz for pure dynssz tests
	dynSszOnlyMainnet.NoFastSsz = true
	dynSszOnlyMinimal.NoFastSsz = true
}

// ========================= BLOCK BENCHMARKS =========================

func BenchmarkBlockMainnet_FastSSZ_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := new(deneb.SignedBeaconBlock)
		if err := block.UnmarshalSSZ(blockMainnetData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_FastSSZ_Marshal(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	block.UnmarshalSSZ(blockMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := block.MarshalSSZ(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_FastSSZ_HashTreeRoot(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	block.UnmarshalSSZ(blockMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := block.Message.HashTreeRoot(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_DynSSZ_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := new(deneb.SignedBeaconBlock)
		if err := dynSszMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_DynSSZ_Marshal(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	dynSszMainnet.UnmarshalSSZ(block, blockMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszMainnet.MarshalSSZ(block); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_DynSSZ_HashTreeRoot(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	dynSszMainnet.UnmarshalSSZ(block, blockMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszMainnet.HashTreeRoot(block.Message); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_DynSSZOnly_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := new(deneb.SignedBeaconBlock)
		if err := dynSszOnlyMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_DynSSZOnly_Marshal(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	dynSszOnlyMainnet.UnmarshalSSZ(block, blockMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszOnlyMainnet.MarshalSSZ(block); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_DynSSZOnly_HashTreeRoot(b *testing.B) {
	block := new(deneb.SignedBeaconBlock)
	dynSszOnlyMainnet.UnmarshalSSZ(block, blockMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszOnlyMainnet.HashTreeRoot(block.Message); err != nil {
			b.Fatal(err)
		}
	}
}

// ========================= MINIMAL BLOCK BENCHMARKS =========================

func BenchmarkBlockMinimal_DynSSZ_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := new(deneb.SignedBeaconBlock)
		if err := dynSszMinimal.UnmarshalSSZ(block, blockMinimalData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMinimal_DynSSZOnly_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := new(deneb.SignedBeaconBlock)
		if err := dynSszOnlyMinimal.UnmarshalSSZ(block, blockMinimalData); err != nil {
			b.Fatal(err)
		}
	}
}

// ========================= STATE BENCHMARKS =========================

func BenchmarkStateMainnet_FastSSZ_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := new(deneb.BeaconState)
		if err := state.UnmarshalSSZ(stateMainnetData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_FastSSZ_Marshal(b *testing.B) {
	state := new(deneb.BeaconState)
	state.UnmarshalSSZ(stateMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.MarshalSSZ(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_FastSSZ_HashTreeRoot(b *testing.B) {
	state := new(deneb.BeaconState)
	state.UnmarshalSSZ(stateMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.HashTreeRoot(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_DynSSZ_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := new(deneb.BeaconState)
		if err := dynSszMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_DynSSZ_Marshal(b *testing.B) {
	state := new(deneb.BeaconState)
	dynSszMainnet.UnmarshalSSZ(state, stateMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszMainnet.MarshalSSZ(state); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_DynSSZ_HashTreeRoot(b *testing.B) {
	state := new(deneb.BeaconState)
	dynSszMainnet.UnmarshalSSZ(state, stateMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszMainnet.HashTreeRoot(state); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_DynSSZOnly_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := new(deneb.BeaconState)
		if err := dynSszOnlyMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_DynSSZOnly_Marshal(b *testing.B) {
	state := new(deneb.BeaconState)
	dynSszOnlyMainnet.UnmarshalSSZ(state, stateMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszOnlyMainnet.MarshalSSZ(state); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_DynSSZOnly_HashTreeRoot(b *testing.B) {
	state := new(deneb.BeaconState)
	dynSszOnlyMainnet.UnmarshalSSZ(state, stateMainnetData)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dynSszOnlyMainnet.HashTreeRoot(state); err != nil {
			b.Fatal(err)
		}
	}
}

// ========================= MINIMAL STATE BENCHMARKS =========================

func BenchmarkStateMinimal_DynSSZ_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := new(deneb.BeaconState)
		if err := dynSszMinimal.UnmarshalSSZ(state, stateMinimalData); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMinimal_DynSSZOnly_Unmarshal(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := new(deneb.BeaconState)
		if err := dynSszOnlyMinimal.UnmarshalSSZ(state, stateMinimalData); err != nil {
			b.Fatal(err)
		}
	}
}

// ========================= COMBINED OPERATIONS =========================

func BenchmarkBlockMainnet_FastSSZ_FullCycle(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := new(deneb.SignedBeaconBlock)
		if err := block.UnmarshalSSZ(blockMainnetData); err != nil {
			b.Fatal(err)
		}
		if _, err := block.MarshalSSZ(); err != nil {
			b.Fatal(err)
		}
		if _, err := block.Message.HashTreeRoot(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlockMainnet_DynSSZ_FullCycle(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := new(deneb.SignedBeaconBlock)
		if err := dynSszMainnet.UnmarshalSSZ(block, blockMainnetData); err != nil {
			b.Fatal(err)
		}
		if _, err := dynSszMainnet.MarshalSSZ(block); err != nil {
			b.Fatal(err)
		}
		if _, err := dynSszMainnet.HashTreeRoot(block.Message); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_FastSSZ_FullCycle(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := new(deneb.BeaconState)
		if err := state.UnmarshalSSZ(stateMainnetData); err != nil {
			b.Fatal(err)
		}
		if _, err := state.MarshalSSZ(); err != nil {
			b.Fatal(err)
		}
		if _, err := state.HashTreeRoot(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMainnet_DynSSZ_FullCycle(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := new(deneb.BeaconState)
		if err := dynSszMainnet.UnmarshalSSZ(state, stateMainnetData); err != nil {
			b.Fatal(err)
		}
		if _, err := dynSszMainnet.MarshalSSZ(state); err != nil {
			b.Fatal(err)
		}
		if _, err := dynSszMainnet.HashTreeRoot(state); err != nil {
			b.Fatal(err)
		}
	}
}
