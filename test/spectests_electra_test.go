package main

import (
	"os"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/electra"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

// TestConsensusSpecElectra tests the types against the Ethereum consensus spec tests.
func TestConsensusSpecElectra(t *testing.T) {
	if os.Getenv("CONSENSUS_SPEC_TESTS_DIR") == "" {
		t.Skip("CONSENSUS_SPEC_TESTS_DIR not supplied, not running spec tests")
	}

	tests := []SpecTestStruct{
		{
			name: "AggregateAndProof",
			s:    &electra.AggregateAndProof{},
		},
		{
			name: "Attestation",
			s:    &electra.Attestation{},
		},
		{
			name: "AttestationData",
			s:    &phase0.AttestationData{},
		},
		{
			name: "AttesterSlashing",
			s:    &electra.AttesterSlashing{},
		},
		{
			name: "BeaconBlock",
			s:    &electra.BeaconBlock{},
		},
		{
			name: "BeaconBlockBody",
			s:    &electra.BeaconBlockBody{},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
		},
		{
			name: "BeaconState",
			s:    &electra.BeaconState{},
		},
		{
			name: "BlobIdentifier",
			s:    &deneb.BlobIdentifier{},
		},
		{
			name: "BlobSidecar",
			s:    &deneb.BlobSidecar{},
		},
		{
			name: "BLSToExecutionChange",
			s:    &capella.BLSToExecutionChange{},
		},
		{
			name: "Checkpoint",
			s:    &phase0.Checkpoint{},
		},
		{
			name: "Consolidation",
			s:    &electra.Consolidation{},
		},
		{
			name: "ConsolidationRequest",
			s:    &electra.ConsolidationRequest{},
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
			name: "DepositRequest",
			s:    &electra.DepositRequest{},
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
			name: "ExecutionRequests",
			s:    &electra.ExecutionRequests{},
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
			name: "HistoricalSummary",
			s:    &capella.HistoricalSummary{},
		},
		{
			name: "IndexedAttestation",
			s:    &electra.IndexedAttestation{},
		},
		{
			name: "PendingAttestation",
			s:    &phase0.PendingAttestation{},
		},
		{
			name: "PendingDeposit",
			s:    &electra.PendingDeposit{},
		},
		{
			name: "PendingConsolidation",
			s:    &electra.PendingConsolidation{},
		},
		{
			name: "PendingPartialWithdrawal",
			s:    &electra.PendingPartialWithdrawal{},
		},
		{
			name: "ProposerSlashing",
			s:    &phase0.ProposerSlashing{},
		},
		{
			name: "SignedAggregateAndProof",
			s:    &electra.SignedAggregateAndProof{},
		},
		{
			name: "SignedBeaconBlock",
			s:    &electra.SignedBeaconBlock{},
		},
		{
			name: "SignedBeaconBlockHeader",
			s:    &phase0.SignedBeaconBlockHeader{},
		},
		{
			name: "SignedBLSToExecutionChange",
			s:    &capella.SignedBLSToExecutionChange{},
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
			name: "SyncCommittee",
			s:    &altair.SyncCommittee{},
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
		{
			name: "Withdrawal",
			s:    &capella.Withdrawal{},
		},
		{
			name: "WithdrawalRequest",
			s:    &electra.WithdrawalRequest{},
		},
	}

	//t.Skipf("Deneb spec tests are not implemented yet (%v)", len(tests))
	testForkConsensusSpec(t, "electra", "mainnet", tests)
	testForkConsensusSpec(t, "electra", "minimal", tests)
}
