package spectests

import (
	"os"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

// TestConsensusSpecDeneb tests the types against the Ethereum consensus spec tests.
func TestConsensusSpecDeneb(t *testing.T) {
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
			s:    &deneb.BeaconBlock{},
		},
		{
			name: "BeaconBlockBody",
			s:    &deneb.BeaconBlockBody{},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
		},
		{
			name: "BeaconState",
			s:    &deneb.BeaconState{},
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
			s:    &deneb.ExecutionPayload{},
		},
		{
			name: "ExecutionPayloadHeader",
			s:    &deneb.ExecutionPayloadHeader{},
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
			s:    &deneb.SignedBeaconBlock{},
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
	}

	mainnetRes := testForkConsensusSpec(t, "deneb", "mainnet", tests)
	minimalRes := testForkConsensusSpec(t, "deneb", "minimal", tests)
	if !mainnetRes && !minimalRes {
		t.Skipf("Fork deneb not found in test data")
	}
}
