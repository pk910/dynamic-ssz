// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package spectests

import (
	"os"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/electra"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/pk910/dynamic-ssz/spectests/codegen"
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
			s2:   &codegen.ElectraAggregateAndProof{},
		},
		{
			name: "Attestation",
			s:    &electra.Attestation{},
			s2:   &codegen.ElectraAttestation{},
		},
		{
			name: "AttestationData",
			s:    &phase0.AttestationData{},
			s2:   &codegen.AttestationData{},
		},
		{
			name: "AttesterSlashing",
			s:    &electra.AttesterSlashing{},
			s2:   &codegen.ElectraAttesterSlashing{},
		},
		{
			name: "BeaconBlock",
			s:    &electra.BeaconBlock{},
			s2:   &codegen.ElectraBeaconBlock{},
		},
		{
			name: "BeaconBlockBody",
			s:    &electra.BeaconBlockBody{},
			s2:   &codegen.ElectraBeaconBlockBody{},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
			s2:   &codegen.BeaconBlockHeader{},
		},
		{
			name: "BeaconState",
			s:    &electra.BeaconState{},
			s2:   &codegen.ElectraBeaconState{},
		},
		{
			name: "BlobIdentifier",
			s:    &deneb.BlobIdentifier{},
			s2:   &codegen.DenebBlobIdentifier{},
		},
		{
			name: "BlobSidecar",
			s:    &deneb.BlobSidecar{},
			s2:   &codegen.DenebBlobSidecar{},
		},
		{
			name: "BLSToExecutionChange",
			s:    &capella.BLSToExecutionChange{},
			s2:   &codegen.CapellaBLSToExecutionChange{},
		},
		{
			name: "Checkpoint",
			s:    &phase0.Checkpoint{},
			s2:   &codegen.Checkpoint{},
		},
		{
			name: "Consolidation",
			s:    &electra.Consolidation{},
			s2:   &codegen.ElectraConsolidation{},
		},
		{
			name: "ConsolidationRequest",
			s:    &electra.ConsolidationRequest{},
			s2:   &codegen.ElectraConsolidationRequest{},
		},
		{
			name: "ContributionAndProof",
			s:    &altair.ContributionAndProof{},
			s2:   &codegen.AltairContributionAndProof{},
		},
		{
			name: "Deposit",
			s:    &phase0.Deposit{},
			s2:   &codegen.Deposit{},
		},
		{
			name: "DepositData",
			s:    &phase0.DepositData{},
			s2:   &codegen.DepositData{},
		},
		{
			name: "DepositRequest",
			s:    &electra.DepositRequest{},
			s2:   &codegen.ElectraDepositRequest{},
		},
		{
			name: "DepositMessage",
			s:    &phase0.DepositMessage{},
			s2:   &codegen.DepositMessage{},
		},
		{
			name: "Eth1Data",
			s:    &phase0.ETH1Data{},
			s2:   &codegen.ETH1Data{},
		},
		{
			name: "ExecutionRequests",
			s:    &electra.ExecutionRequests{},
			s2:   &codegen.ElectraExecutionRequests{},
		},
		{
			name: "Fork",
			s:    &phase0.Fork{},
			s2:   &codegen.Fork{},
		},
		{
			name: "ForkData",
			s:    &phase0.ForkData{},
			s2:   &codegen.ForkData{},
		},
		{
			name: "HistoricalSummary",
			s:    &capella.HistoricalSummary{},
			s2:   &codegen.CapellaHistoricalSummary{},
		},
		{
			name: "IndexedAttestation",
			s:    &electra.IndexedAttestation{},
			s2:   &codegen.ElectraIndexedAttestation{},
		},
		{
			name: "PendingAttestation",
			s:    &phase0.PendingAttestation{},
			s2:   &codegen.PendingAttestation{},
		},
		{
			name: "PendingDeposit",
			s:    &electra.PendingDeposit{},
			s2:   &codegen.ElectraPendingDeposit{},
		},
		{
			name: "PendingConsolidation",
			s:    &electra.PendingConsolidation{},
			s2:   &codegen.ElectraPendingConsolidation{},
		},
		{
			name: "PendingPartialWithdrawal",
			s:    &electra.PendingPartialWithdrawal{},
			s2:   &codegen.ElectraPendingPartialWithdrawal{},
		},
		{
			name: "ProposerSlashing",
			s:    &phase0.ProposerSlashing{},
			s2:   &codegen.ProposerSlashing{},
		},
		{
			name: "SignedAggregateAndProof",
			s:    &electra.SignedAggregateAndProof{},
			s2:   &codegen.ElectraSignedAggregateAndProof{},
		},
		{
			name: "SignedBeaconBlock",
			s:    &electra.SignedBeaconBlock{},
			s2:   &codegen.ElectraSignedBeaconBlock{},
		},
		{
			name: "SignedBeaconBlockHeader",
			s:    &phase0.SignedBeaconBlockHeader{},
			s2:   &codegen.SignedBeaconBlockHeader{},
		},
		{
			name: "SignedBLSToExecutionChange",
			s:    &capella.SignedBLSToExecutionChange{},
			s2:   &codegen.CapellaSignedBLSToExecutionChange{},
		},
		{
			name: "SignedContributionAndProof",
			s:    &altair.SignedContributionAndProof{},
			s2:   &codegen.AltairSignedContributionAndProof{},
		},
		{
			name: "SignedVoluntaryExit",
			s:    &phase0.SignedVoluntaryExit{},
			s2:   &codegen.SignedVoluntaryExit{},
		},
		{
			name: "SyncAggregate",
			s:    &altair.SyncAggregate{},
			s2:   &codegen.AltairSyncAggregate{},
		},
		{
			name: "SyncCommittee",
			s:    &altair.SyncCommittee{},
			s2:   &codegen.AltairSyncCommittee{},
		},
		{
			name: "SyncCommitteeContribution",
			s:    &altair.SyncCommitteeContribution{},
			s2:   &codegen.AltairSyncCommitteeContribution{},
		},
		{
			name: "SyncCommitteeMessage",
			s:    &altair.SyncCommitteeMessage{},
			s2:   &codegen.AltairSyncCommitteeMessage{},
		},
		{
			name: "Validator",
			s:    &phase0.Validator{},
			s2:   &codegen.Validator{},
		},
		{
			name: "VoluntaryExit",
			s:    &phase0.VoluntaryExit{},
			s2:   &codegen.VoluntaryExit{},
		},
		{
			name: "Withdrawal",
			s:    &capella.Withdrawal{},
			s2:   &codegen.CapellaWithdrawal{},
		},
		{
			name: "WithdrawalRequest",
			s:    &electra.WithdrawalRequest{},
			s2:   &codegen.ElectraWithdrawalRequest{},
		},
	}

	mainnetRes := runForkConsensusSpecTest(t, "electra", "mainnet", tests)
	minimalRes := runForkConsensusSpecTest(t, "electra", "minimal", tests)
	if !mainnetRes && !minimalRes {
		t.Skipf("Fork electra not found in test data")
	}
}
