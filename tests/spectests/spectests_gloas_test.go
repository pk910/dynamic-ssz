// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package spectests

import (
	"os"
	"testing"

	"github.com/ethpandaops/go-eth2-client/spec/altair"
	"github.com/ethpandaops/go-eth2-client/spec/capella"
	"github.com/ethpandaops/go-eth2-client/spec/electra"
	"github.com/ethpandaops/go-eth2-client/spec/gloas"
	"github.com/ethpandaops/go-eth2-client/spec/phase0"
	"github.com/pk910/dynamic-ssz/spectests/codegen"
	codegen_views "github.com/pk910/dynamic-ssz/spectests/codegen-views"
)

// TestConsensusSpecGloas tests the types against the Ethereum consensus spec tests.
func TestConsensusSpecGloas(t *testing.T) {
	if os.Getenv("CONSENSUS_SPEC_TESTS_DIR") == "" {
		t.Skip("CONSENSUS_SPEC_TESTS_DIR not supplied, not running spec tests")
	}

	tests := []SpecTestStruct{
		{
			name: "AggregateAndProof",
			s:    &gloas.AggregateAndProof{},
			s2:   &codegen.GloasAggregateAndProof{},
			s3:   []any{&codegen_views.AggregateAndProof{}, &codegen_views.GloasAggregateAndProof{}},
		},
		{
			name: "Attestation",
			s:    &gloas.Attestation{},
			s2:   &codegen.GloasAttestation{},
			s3:   []any{&codegen_views.Attestation{}, &codegen_views.GloasAttestation{}},
		},
		{
			name: "AttestationData",
			s:    &phase0.AttestationData{},
			s2:   &codegen.AttestationData{},
			s3:   []any{&codegen_views.AttestationData{}, &codegen_views.Phase0AttestationData{}},
		},
		{
			name: "AttesterSlashing",
			s:    &gloas.AttesterSlashing{},
			s2:   &codegen.GloasAttesterSlashing{},
			s3:   []any{&codegen_views.AttesterSlashing{}, &codegen_views.GloasAttesterSlashing{}},
		},
		{
			name: "BeaconBlock",
			s:    &gloas.BeaconBlock{},
			s2:   &codegen.GloasBeaconBlock{},
			s3:   []any{&codegen_views.BeaconBlock{}, &codegen_views.GloasBeaconBlock{}},
		},
		{
			name: "BeaconBlockBody",
			s:    &gloas.BeaconBlockBody{},
			s2:   &codegen.GloasBeaconBlockBody{},
			s3:   []any{&codegen_views.BeaconBlockBody{}, &codegen_views.GloasBeaconBlockBody{}},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
			s2:   &codegen.BeaconBlockHeader{},
			s3:   []any{&codegen_views.BeaconBlockHeader{}, &codegen_views.Phase0BeaconBlockHeader{}},
		},
		{
			name: "BeaconState",
			s:    &gloas.BeaconState{},
			s2:   &codegen.GloasBeaconState{},
			s3:   []any{&codegen_views.BeaconState{}, &codegen_views.GloasBeaconState{}},
		},
		{
			name: "BLSToExecutionChange",
			s:    &capella.BLSToExecutionChange{},
			s2:   &codegen.CapellaBLSToExecutionChange{},
			s3:   []any{&codegen_views.BLSToExecutionChange{}, &codegen_views.CapellaBLSToExecutionChange{}},
		},
		{
			name: "Builder",
			s:    &gloas.Builder{},
			s2:   &codegen.GloasBuilder{},
			s3:   []any{&codegen_views.Builder{}, &codegen_views.GloasBuilder{}},
		},
		{
			name: "BuilderDepositRequest",
			s:    &gloas.BuilderDepositRequest{},
			s2:   &codegen.GloasBuilderDepositRequest{},
			s3:   []any{&codegen_views.BuilderDepositRequest{}, &codegen_views.GloasBuilderDepositRequest{}},
		},
		{
			name: "BuilderExitRequest",
			s:    &gloas.BuilderExitRequest{},
			s2:   &codegen.GloasBuilderExitRequest{},
			s3:   []any{&codegen_views.BuilderExitRequest{}, &codegen_views.GloasBuilderExitRequest{}},
		},
		{
			name: "BuilderPendingPayment",
			s:    &gloas.BuilderPendingPayment{},
			s2:   &codegen.GloasBuilderPendingPayment{},
			s3:   []any{&codegen_views.BuilderPendingPayment{}, &codegen_views.GloasBuilderPendingPayment{}},
		},
		{
			name: "BuilderPendingWithdrawal",
			s:    &gloas.BuilderPendingWithdrawal{},
			s2:   &codegen.GloasBuilderPendingWithdrawal{},
			s3:   []any{&codegen_views.BuilderPendingWithdrawal{}, &codegen_views.GloasBuilderPendingWithdrawal{}},
		},
		{
			name: "Checkpoint",
			s:    &phase0.Checkpoint{},
			s2:   &codegen.Checkpoint{},
			s3:   []any{&codegen_views.Checkpoint{}, &codegen_views.Phase0Checkpoint{}},
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
			name: "DepositMessage",
			s:    &phase0.DepositMessage{},
			s2:   &codegen.DepositMessage{},
			s3:   []any{&codegen_views.DepositMessage{}, &codegen_views.Phase0DepositMessage{}},
		},
		{
			name: "DepositRequest",
			s:    &electra.DepositRequest{},
			s2:   &codegen.ElectraDepositRequest{},
			s3:   []any{&codegen_views.DepositRequest{}, &codegen_views.ElectraDepositRequest{}},
		},
		{
			name: "Eth1Data",
			s:    &phase0.ETH1Data{},
			s2:   &codegen.ETH1Data{},
			s3:   []any{&codegen_views.ETH1Data{}, &codegen_views.Phase0ETH1Data{}},
		},
		{
			name: "ExecutionPayload",
			s:    &gloas.ExecutionPayload{},
			s2:   &codegen.GloasExecutionPayload{},
			s3:   []any{&codegen_views.ExecutionPayload{}, &codegen_views.GloasExecutionPayload{}},
		},
		{
			name: "ExecutionPayloadBid",
			s:    &gloas.ExecutionPayloadBid{},
			s2:   &codegen.GloasExecutionPayloadBid{},
			s3:   []any{&codegen_views.ExecutionPayloadBid{}, &codegen_views.GloasExecutionPayloadBid{}},
		},
		{
			name: "ExecutionPayloadEnvelope",
			s:    &gloas.ExecutionPayloadEnvelope{},
			s2:   &codegen.GloasExecutionPayloadEnvelope{},
			s3:   []any{&codegen_views.ExecutionPayloadEnvelope{}, &codegen_views.GloasExecutionPayloadEnvelope{}},
		},
		{
			name: "ExecutionRequests",
			s:    &gloas.ExecutionRequests{},
			s2:   &codegen.GloasExecutionRequests{},
			s3:   []any{&codegen_views.ExecutionRequests{}, &codegen_views.GloasExecutionRequests{}},
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
			s:    &gloas.IndexedAttestation{},
			s2:   &codegen.GloasIndexedAttestation{},
			s3:   []any{&codegen_views.IndexedAttestation{}, &codegen_views.GloasIndexedAttestation{}},
		},
		{
			name: "IndexedPayloadAttestation",
			s:    &gloas.IndexedPayloadAttestation{},
			s2:   &codegen.GloasIndexedPayloadAttestation{},
			s3:   []any{&codegen_views.IndexedPayloadAttestation{}, &codegen_views.GloasIndexedPayloadAttestation{}},
		},
		{
			name: "PayloadAttestation",
			s:    &gloas.PayloadAttestation{},
			s2:   &codegen.GloasPayloadAttestation{},
			s3:   []any{&codegen_views.PayloadAttestation{}, &codegen_views.GloasPayloadAttestation{}},
		},
		{
			name: "PayloadAttestationData",
			s:    &gloas.PayloadAttestationData{},
			s2:   &codegen.GloasPayloadAttestationData{},
			s3:   []any{&codegen_views.PayloadAttestationData{}, &codegen_views.GloasPayloadAttestationData{}},
		},
		{
			name: "PayloadAttestationMessage",
			s:    &gloas.PayloadAttestationMessage{},
			s2:   &codegen.GloasPayloadAttestationMessage{},
			s3:   []any{&codegen_views.PayloadAttestationMessage{}, &codegen_views.GloasPayloadAttestationMessage{}},
		},
		{
			name: "PendingConsolidation",
			s:    &electra.PendingConsolidation{},
			s2:   &codegen.ElectraPendingConsolidation{},
			s3:   []any{&codegen_views.PendingConsolidation{}, &codegen_views.ElectraPendingConsolidation{}},
		},
		{
			name: "PendingDeposit",
			s:    &electra.PendingDeposit{},
			s2:   &codegen.ElectraPendingDeposit{},
			s3:   []any{&codegen_views.PendingDeposit{}, &codegen_views.ElectraPendingDeposit{}},
		},
		{
			name: "PendingPartialWithdrawal",
			s:    &electra.PendingPartialWithdrawal{},
			s2:   &codegen.ElectraPendingPartialWithdrawal{},
			s3:   []any{&codegen_views.PendingPartialWithdrawal{}, &codegen_views.ElectraPendingPartialWithdrawal{}},
		},
		{
			name: "ProposerPreferences",
			s:    &gloas.ProposerPreferences{},
			s2:   &codegen.GloasProposerPreferences{},
			s3:   []any{&codegen_views.ProposerPreferences{}, &codegen_views.GloasProposerPreferences{}},
		},
		{
			name: "ProposerSlashing",
			s:    &phase0.ProposerSlashing{},
			s2:   &codegen.ProposerSlashing{},
			s3:   []any{&codegen_views.ProposerSlashing{}, &codegen_views.Phase0ProposerSlashing{}},
		},
		{
			name: "SignedAggregateAndProof",
			s:    &gloas.SignedAggregateAndProof{},
			s2:   &codegen.GloasSignedAggregateAndProof{},
			s3:   []any{&codegen_views.SignedAggregateAndProof{}, &codegen_views.GloasSignedAggregateAndProof{}},
		},
		{
			name: "SignedBeaconBlock",
			s:    &gloas.SignedBeaconBlock{},
			s2:   &codegen.GloasSignedBeaconBlock{},
			s3:   []any{&codegen_views.SignedBeaconBlock{}, &codegen_views.GloasSignedBeaconBlock{}},
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
			name: "SignedExecutionPayloadBid",
			s:    &gloas.SignedExecutionPayloadBid{},
			s2:   &codegen.GloasSignedExecutionPayloadBid{},
			s3:   []any{&codegen_views.SignedExecutionPayloadBid{}, &codegen_views.GloasSignedExecutionPayloadBid{}},
		},
		{
			name: "SignedExecutionPayloadEnvelope",
			s:    &gloas.SignedExecutionPayloadEnvelope{},
			s2:   &codegen.GloasSignedExecutionPayloadEnvelope{},
			s3:   []any{&codegen_views.SignedExecutionPayloadEnvelope{}, &codegen_views.GloasSignedExecutionPayloadEnvelope{}},
		},
		{
			name: "SignedProposerPreferences",
			s:    &gloas.SignedProposerPreferences{},
			s2:   &codegen.GloasSignedProposerPreferences{},
			s3:   []any{&codegen_views.SignedProposerPreferences{}, &codegen_views.GloasSignedProposerPreferences{}},
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

	mainnetRes := runForkConsensusSpecTest(t, "gloas", "mainnet", tests)
	minimalRes := runForkConsensusSpecTest(t, "gloas", "minimal", tests)
	if !mainnetRes && !minimalRes {
		t.Skipf("Fork gloas not found in test data")
	}
}
