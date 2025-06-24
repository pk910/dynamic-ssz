package spectests

import (
	"os"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/phase0"
)

// TestConsensusSpecPhase0 tests the types against the Ethereum consensus spec tests.
func TestConsensusSpecPhase0(t *testing.T) {
	if os.Getenv("CONSENSUS_SPEC_TESTS_DIR") == "" {
		t.Skip("CONSENSUS_SPEC_TESTS_DIR not supplied, not running spec tests")
	}

	tests := []SpecTestStruct{
		{
			name: "AggregateAndProof",
			s:    &phase0.AggregateAndProof{},
		},
		{
			name: "Attestation",
			s:    &phase0.Attestation{},
		},
		{
			name: "AttestationData",
			s:    &phase0.AttestationData{},
		},
		{
			name: "AttesterSlashing",
			s:    &phase0.AttesterSlashing{},
		},
		{
			name: "BeaconBlock",
			s:    &phase0.BeaconBlock{},
		},
		{
			name: "BeaconBlockBody",
			s:    &phase0.BeaconBlockBody{},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
		},
		{
			name: "BeaconState",
			s:    &phase0.BeaconState{},
		},
		{
			name: "Checkpoint",
			s:    &phase0.Checkpoint{},
		},
		{
			name: "Deposit",
			s:    &phase0.Deposit{},
		},
		{
			name: "DepositData",
			s:    &phase0.DepositData{},
		},
		{
			name: "DepositMessage",
			s:    &phase0.DepositMessage{},
		},
		{
			name: "Eth1Data",
			s:    &phase0.ETH1Data{},
		},
		{
			name: "Fork",
			s:    &phase0.Fork{},
		},
		{
			name: "ForkData",
			s:    &phase0.ForkData{},
		},
		{
			name: "IndexedAttestation",
			s:    &phase0.IndexedAttestation{},
		},
		{
			name: "PendingAttestation",
			s:    &phase0.PendingAttestation{},
		},
		{
			name: "ProposerSlashing",
			s:    &phase0.ProposerSlashing{},
		},
		{
			name: "SignedAggregateAndProof",
			s:    &phase0.SignedAggregateAndProof{},
		},
		{
			name: "SignedBeaconBlock",
			s:    &phase0.SignedBeaconBlock{},
		},
		{
			name: "SignedBeaconBlockHeader",
			s:    &phase0.SignedBeaconBlockHeader{},
		},
		{
			name: "SignedVoluntaryExit",
			s:    &phase0.SignedVoluntaryExit{},
		},
		{
			name: "Validator",
			s:    &phase0.Validator{},
		},
		{
			name: "VoluntaryExit",
			s:    &phase0.VoluntaryExit{},
		},
	}

	testForkConsensusSpec(t, "phase0", "mainnet", tests)
	testForkConsensusSpec(t, "phase0", "minimal", tests)
}
