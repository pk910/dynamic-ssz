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
	codegen_views "github.com/pk910/dynamic-ssz/spectests/codegen-views"
)

// TestConsensusSpecFulu tests the types against the Ethereum consensus spec tests.
func TestConsensusSpecFulu(t *testing.T) {
	if os.Getenv("CONSENSUS_SPEC_TESTS_DIR") == "" {
		t.Skip("CONSENSUS_SPEC_TESTS_DIR not supplied, not running spec tests")
	}

	tests := []SpecTestStruct{
		{
			name: "AggregateAndProof",
			s:    &electra.AggregateAndProof{},
			s2:   &codegen.ElectraAggregateAndProof{},
			s3:   []any{&codegen_views.AggregateAndProof{}, &codegen_views.ElectraAggregateAndProof{}},
		},
		{
			name: "Attestation",
			s:    &electra.Attestation{},
			s2:   &codegen.ElectraAttestation{},
			s3:   []any{&codegen_views.Attestation{}, &codegen_views.ElectraAttestation{}},
		},
		{
			name: "AttestationData",
			s:    &phase0.AttestationData{},
			s2:   &codegen.AttestationData{},
			s3:   []any{&codegen_views.AttestationData{}, &codegen_views.Phase0AttestationData{}},
		},
		{
			name: "AttesterSlashing",
			s:    &electra.AttesterSlashing{},
			s2:   &codegen.ElectraAttesterSlashing{},
			s3:   []any{&codegen_views.AttesterSlashing{}, &codegen_views.ElectraAttesterSlashing{}},
		},
		{
			name: "BeaconBlock",
			s:    &electra.BeaconBlock{},
			s2:   &codegen.ElectraBeaconBlock{},
			s3:   []any{&codegen_views.BeaconBlock{}, &codegen_views.ElectraBeaconBlock{}},
		},
		{
			name: "BeaconBlockBody",
			s:    &electra.BeaconBlockBody{},
			s2:   &codegen.ElectraBeaconBlockBody{},
			s3:   []any{&codegen_views.BeaconBlockBody{}, &codegen_views.ElectraBeaconBlockBody{}},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
			s2:   &codegen.BeaconBlockHeader{},
			s3:   []any{&codegen_views.BeaconBlockHeader{}, &codegen_views.Phase0BeaconBlockHeader{}},
		},
		{
			name: "BeaconState",
			s:    &electra.BeaconState{}, // TODO: Update to Fulu (latest spectests release still uses Electra)
			s2:   &codegen.ElectraBeaconState{},
			s3:   []any{&codegen_views.BeaconState{}, &codegen_views.ElectraBeaconState{}},
		},
		{
			name: "BlobIdentifier",
			s:    &deneb.BlobIdentifier{},
			s2:   &codegen.DenebBlobIdentifier{},
			s3:   []any{&codegen_views.BlobIdentifier{}, &codegen_views.DenebBlobIdentifier{}},
		},
		{
			name: "BlobSidecar",
			s:    &deneb.BlobSidecar{},
			s2:   &codegen.DenebBlobSidecar{},
			s3:   []any{&codegen_views.BlobSidecar{}, &codegen_views.DenebBlobSidecar{}},
		},
		{
			name: "BLSToExecutionChange",
			s:    &capella.BLSToExecutionChange{},
			s2:   &codegen.CapellaBLSToExecutionChange{},
			s3:   []any{&codegen_views.BLSToExecutionChange{}, &codegen_views.CapellaBLSToExecutionChange{}},
		},
		{
			name: "Checkpoint",
			s:    &phase0.Checkpoint{},
			s2:   &codegen.Checkpoint{},
			s3:   []any{&codegen_views.Checkpoint{}, &codegen_views.Phase0Checkpoint{}},
		},
		{
			name: "Consolidation",
			s:    &electra.Consolidation{},
			s2:   &codegen.ElectraConsolidation{},
			s3:   []any{&codegen_views.Consolidation{}, &codegen_views.ElectraConsolidation{}},
		},
		{
			name: "ConsolidationRequest",
			s:    &electra.ConsolidationRequest{},
			s2:   &codegen.ElectraConsolidationRequest{},
			s3:   []any{&codegen_views.ConsolidationRequest{}, &codegen_views.ElectraConsolidationRequest{}},
		},
		{
			name: "ContributionAndProof",
			s:    &altair.ContributionAndProof{},
			s2:   &codegen.AltairContributionAndProof{},
			s3:   []any{&codegen_views.ContributionAndProof{}, &codegen_views.AltairContributionAndProof{}},
		},
		{
			name: "Deposit",
			s:    &phase0.Deposit{},
			s2:   &codegen.Deposit{},
			s3:   []any{&codegen_views.Deposit{}, &codegen_views.Phase0Deposit{}},
		},
		{
			name: "DepositData",
			s:    &phase0.DepositData{},
			s2:   &codegen.DepositData{},
			s3:   []any{&codegen_views.DepositData{}, &codegen_views.Phase0DepositData{}},
		},
		{
			name: "DepositRequest",
			s:    &electra.DepositRequest{},
			s2:   &codegen.ElectraDepositRequest{},
			s3:   []any{&codegen_views.DepositRequest{}, &codegen_views.ElectraDepositRequest{}},
		},
		{
			name: "DepositMessage",
			s:    &phase0.DepositMessage{},
			s2:   &codegen.DepositMessage{},
			s3:   []any{&codegen_views.DepositMessage{}, &codegen_views.Phase0DepositMessage{}},
		},
		{
			name: "Eth1Data",
			s:    &phase0.ETH1Data{},
			s2:   &codegen.ETH1Data{},
			s3:   []any{&codegen_views.ETH1Data{}, &codegen_views.Phase0ETH1Data{}},
		},
		{
			name: "ExecutionRequests",
			s:    &electra.ExecutionRequests{},
			s2:   &codegen.ElectraExecutionRequests{},
			s3:   []any{&codegen_views.ExecutionRequests{}, &codegen_views.ElectraExecutionRequests{}},
		},
		{
			name: "Fork",
			s:    &phase0.Fork{},
			s2:   &codegen.Fork{},
			s3:   []any{&codegen_views.Fork{}, &codegen_views.Phase0Fork{}},
		},
		{
			name: "ForkData",
			s:    &phase0.ForkData{},
			s2:   &codegen.ForkData{},
			s3:   []any{&codegen_views.ForkData{}, &codegen_views.Phase0ForkData{}},
		},
		{
			name: "HistoricalSummary",
			s:    &capella.HistoricalSummary{},
			s2:   &codegen.CapellaHistoricalSummary{},
			s3:   []any{&codegen_views.HistoricalSummary{}, &codegen_views.CapellaHistoricalSummary{}},
		},
		{
			name: "IndexedAttestation",
			s:    &electra.IndexedAttestation{},
			s2:   &codegen.ElectraIndexedAttestation{},
			s3:   []any{&codegen_views.IndexedAttestation{}, &codegen_views.ElectraIndexedAttestation{}},
		},
		{
			name: "PendingAttestation",
			s:    &phase0.PendingAttestation{},
			s2:   &codegen.PendingAttestation{},
			s3:   []any{&codegen_views.PendingAttestation{}, &codegen_views.Phase0PendingAttestation{}},
		},
		{
			name: "PendingDeposit",
			s:    &electra.PendingDeposit{},
			s2:   &codegen.ElectraPendingDeposit{},
			s3:   []any{&codegen_views.PendingDeposit{}, &codegen_views.ElectraPendingDeposit{}},
		},
		{
			name: "PendingConsolidation",
			s:    &electra.PendingConsolidation{},
			s2:   &codegen.ElectraPendingConsolidation{},
			s3:   []any{&codegen_views.PendingConsolidation{}, &codegen_views.ElectraPendingConsolidation{}},
		},
		{
			name: "PendingPartialWithdrawal",
			s:    &electra.PendingPartialWithdrawal{},
			s2:   &codegen.ElectraPendingPartialWithdrawal{},
			s3:   []any{&codegen_views.PendingPartialWithdrawal{}, &codegen_views.ElectraPendingPartialWithdrawal{}},
		},
		{
			name: "ProposerSlashing",
			s:    &phase0.ProposerSlashing{},
			s2:   &codegen.ProposerSlashing{},
			s3:   []any{&codegen_views.ProposerSlashing{}, &codegen_views.Phase0ProposerSlashing{}},
		},
		{
			name: "SignedAggregateAndProof",
			s:    &electra.SignedAggregateAndProof{},
			s2:   &codegen.ElectraSignedAggregateAndProof{},
			s3:   []any{&codegen_views.SignedAggregateAndProof{}, &codegen_views.ElectraSignedAggregateAndProof{}},
		},
		{
			name: "SignedBeaconBlock",
			s:    &electra.SignedBeaconBlock{},
			s2:   &codegen.ElectraSignedBeaconBlock{},
			s3:   []any{&codegen_views.SignedBeaconBlock{}, &codegen_views.ElectraSignedBeaconBlock{}},
		},
		{
			name: "SignedBeaconBlockHeader",
			s:    &phase0.SignedBeaconBlockHeader{},
			s2:   &codegen.SignedBeaconBlockHeader{},
			s3:   []any{&codegen_views.SignedBeaconBlockHeader{}, &codegen_views.Phase0SignedBeaconBlockHeader{}},
		},
		{
			name: "SignedBLSToExecutionChange",
			s:    &capella.SignedBLSToExecutionChange{},
			s2:   &codegen.CapellaSignedBLSToExecutionChange{},
			s3:   []any{&codegen_views.SignedBLSToExecutionChange{}, &codegen_views.CapellaSignedBLSToExecutionChange{}},
		},
		{
			name: "SignedContributionAndProof",
			s:    &altair.SignedContributionAndProof{},
			s2:   &codegen.AltairSignedContributionAndProof{},
			s3:   []any{&codegen_views.SignedContributionAndProof{}, &codegen_views.AltairSignedContributionAndProof{}},
		},
		{
			name: "SignedVoluntaryExit",
			s:    &phase0.SignedVoluntaryExit{},
			s2:   &codegen.SignedVoluntaryExit{},
			s3:   []any{&codegen_views.SignedVoluntaryExit{}, &codegen_views.Phase0SignedVoluntaryExit{}},
		},
		{
			name: "SyncAggregate",
			s:    &altair.SyncAggregate{},
			s2:   &codegen.AltairSyncAggregate{},
			s3:   []any{&codegen_views.SyncAggregate{}, &codegen_views.AltairSyncAggregate{}},
		},
		{
			name: "SyncCommittee",
			s:    &altair.SyncCommittee{},
			s2:   &codegen.AltairSyncCommittee{},
			s3:   []any{&codegen_views.SyncCommittee{}, &codegen_views.AltairSyncCommittee{}},
		},
		{
			name: "SyncCommitteeContribution",
			s:    &altair.SyncCommitteeContribution{},
			s2:   &codegen.AltairSyncCommitteeContribution{},
			s3:   []any{&codegen_views.SyncCommitteeContribution{}, &codegen_views.AltairSyncCommitteeContribution{}},
		},
		{
			name: "SyncCommitteeMessage",
			s:    &altair.SyncCommitteeMessage{},
			s2:   &codegen.AltairSyncCommitteeMessage{},
			s3:   []any{&codegen_views.SyncCommitteeMessage{}, &codegen_views.AltairSyncCommitteeMessage{}},
		},
		{
			name: "Validator",
			s:    &phase0.Validator{},
			s2:   &codegen.Validator{},
			s3:   []any{&codegen_views.Validator{}, &codegen_views.Phase0Validator{}},
		},
		{
			name: "VoluntaryExit",
			s:    &phase0.VoluntaryExit{},
			s2:   &codegen.VoluntaryExit{},
			s3:   []any{&codegen_views.VoluntaryExit{}, &codegen_views.Phase0VoluntaryExit{}},
		},
		{
			name: "Withdrawal",
			s:    &capella.Withdrawal{},
			s2:   &codegen.CapellaWithdrawal{},
			s3:   []any{&codegen_views.Withdrawal{}, &codegen_views.CapellaWithdrawal{}},
		},
		{
			name: "WithdrawalRequest",
			s:    &electra.WithdrawalRequest{},
			s2:   &codegen.ElectraWithdrawalRequest{},
			s3:   []any{&codegen_views.WithdrawalRequest{}, &codegen_views.ElectraWithdrawalRequest{}},
		},
	}

	mainnetRes := runForkConsensusSpecTest(t, "fulu", "mainnet", tests)
	minimalRes := runForkConsensusSpecTest(t, "fulu", "minimal", tests)
	if !mainnetRes && !minimalRes {
		t.Skipf("Fork fulu not found in test data")
	}
}
