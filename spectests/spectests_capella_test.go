// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package spectests

import (
	"os"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/pk910/dynamic-ssz/spectests/codegen"
)

// TestConsensusSpec tests the types against the Ethereum consensus spec tests.
func TestConsensusSpecCapella(t *testing.T) {
	if os.Getenv("CONSENSUS_SPEC_TESTS_DIR") == "" {
		t.Skip("CONSENSUS_SPEC_TESTS_DIR not supplied, not running spec tests")
	}

	tests := []SpecTestStruct{
		{
			name: "AggregateAndProof",
			s:    &phase0.AggregateAndProof{},
			s2:   &codegen.AggregateAndProof{},
		},
		{
			name: "Attestation",
			s:    &phase0.Attestation{},
			s2:   &codegen.Attestation{},
		},
		{
			name: "AttestationData",
			s:    &phase0.AttestationData{},
			s2:   &codegen.AttestationData{},
		},
		{
			name: "AttesterSlashing",
			s:    &phase0.AttesterSlashing{},
			s2:   &codegen.AttesterSlashing{},
		},
		{
			name: "BeaconBlock",
			s:    &capella.BeaconBlock{},
			s2:   &codegen.CapellaBeaconBlock{},
		},
		{
			name: "BeaconBlockBody",
			s:    &capella.BeaconBlockBody{},
			s2:   &codegen.CapellaBeaconBlockBody{},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
			s2:   &codegen.BeaconBlockHeader{},
		},
		{
			name: "BeaconState",
			s:    &capella.BeaconState{},
			s2:   &codegen.CapellaBeaconState{},
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
			name: "DepositMessage",
			s:    &phase0.DepositMessage{},
			s2:   &codegen.DepositMessage{},
		},
		{
			name: "Eth1Data",
			s:    &phase0.ETH1Data{},
			s2:   &codegen.Eth1Data{},
		},
		{
			name: "ExecutionPayload",
			s:    &capella.ExecutionPayload{},
			s2:   &codegen.CapellaExecutionPayload{},
		},
		{
			name: "ExecutionPayloadHeader",
			s:    &capella.ExecutionPayloadHeader{},
			s2:   &codegen.CapellaExecutionPayloadHeader{},
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
			s:    &phase0.IndexedAttestation{},
			s2:   &codegen.IndexedAttestation{},
		},
		{
			name: "PendingAttestation",
			s:    &phase0.PendingAttestation{},
			s2:   &codegen.PendingAttestation{},
		},
		{
			name: "ProposerSlashing",
			s:    &phase0.ProposerSlashing{},
			s2:   &codegen.ProposerSlashing{},
		},
		{
			name: "SignedAggregateAndProof",
			s:    &phase0.SignedAggregateAndProof{},
			s2:   &codegen.SignedAggregateAndProof{},
		},
		{
			name: "SignedBeaconBlock",
			s:    &capella.SignedBeaconBlock{},
			s2:   &codegen.CapellaSignedBeaconBlock{},
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
	}

	mainnetRes := runForkConsensusSpecTest(t, "capella", "mainnet", tests)
	minimalRes := runForkConsensusSpecTest(t, "capella", "minimal", tests)
	if !mainnetRes && !minimalRes {
		t.Skipf("Fork capella not found in test data")
	}
}
