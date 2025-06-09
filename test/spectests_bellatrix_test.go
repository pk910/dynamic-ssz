package main

import (
	"os"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

// TestConsensusSpecBellatrix tests the types against the Ethereum consensus spec tests.
func TestConsensusSpecBellatrix(t *testing.T) {
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
			s:    &bellatrix.BeaconBlock{},
		},
		{
			name: "BeaconBlockBody",
			s:    &bellatrix.BeaconBlockBody{},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
		},
		{
			name: "BeaconState",
			s:    &bellatrix.BeaconState{},
		},
		{
			name: "Checkpoint",
			s:    &phase0.Checkpoint{},
		},
		{
			name: "ContributionAndProof",
			s:    &altair.ContributionAndProof{},
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
			name: "ExecutionPayload",
			s:    &bellatrix.ExecutionPayload{},
		},
		{
			name: "ExecutionPayloadHeader",
			s:    &bellatrix.ExecutionPayloadHeader{},
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
			s:    &bellatrix.SignedBeaconBlock{},
		},
		{
			name: "SignedBeaconBlockHeader",
			s:    &phase0.SignedBeaconBlockHeader{},
		},
		{
			name: "SignedContributionAndProof",
			s:    &altair.SignedContributionAndProof{},
		},
		{
			name: "SignedVoluntaryExit",
			s:    &phase0.SignedVoluntaryExit{},
		},
		{
			name: "SyncAggregate",
			s:    &altair.SyncAggregate{},
		},
		{
			name: "SyncCommitteeContribution",
			s:    &altair.SyncCommitteeContribution{},
		},
		{
			name: "SyncCommitteeMessage",
			s:    &altair.SyncCommitteeMessage{},
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

	testForkConsensusSpec(t, "bellatrix", "mainnet", tests)
	testForkConsensusSpec(t, "bellatrix", "minimal", tests)
}
