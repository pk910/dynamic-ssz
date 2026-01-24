// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package spectests

import (
	"os"
	"testing"

	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/pk910/dynamic-ssz/spectests/codegen"
	codegen_views "github.com/pk910/dynamic-ssz/spectests/codegen-views"
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
			s2:   &codegen.AggregateAndProof{},
			s3:   []any{&codegen_views.AggregateAndProof{}, &codegen_views.Phase0AggregateAndProof{}},
		},
		{
			name: "Attestation",
			s:    &phase0.Attestation{},
			s2:   &codegen.Attestation{},
			s3:   []any{&codegen_views.Attestation{}, &codegen_views.Phase0Attestation{}},
		},
		{
			name: "AttestationData",
			s:    &phase0.AttestationData{},
			s2:   &codegen.AttestationData{},
			s3:   []any{&codegen_views.AttestationData{}, &codegen_views.Phase0AttestationData{}},
		},
		{
			name: "AttesterSlashing",
			s:    &phase0.AttesterSlashing{},
			s2:   &codegen.AttesterSlashing{},
			s3:   []any{&codegen_views.AttesterSlashing{}, &codegen_views.Phase0AttesterSlashing{}},
		},
		{
			name: "BeaconBlock",
			s:    &bellatrix.BeaconBlock{},
			s2:   &codegen.BellatrixBeaconBlock{},
			s3:   []any{&codegen_views.BeaconBlock{}, &codegen_views.BellatrixBeaconBlock{}},
		},
		{
			name: "BeaconBlockBody",
			s:    &bellatrix.BeaconBlockBody{},
			s2:   &codegen.BellatrixBeaconBlockBody{},
			s3:   []any{&codegen_views.BeaconBlockBody{}, &codegen_views.BellatrixBeaconBlockBody{}},
		},
		{
			name: "BeaconBlockHeader",
			s:    &phase0.BeaconBlockHeader{},
			s2:   &codegen.BeaconBlockHeader{},
			s3:   []any{&codegen_views.BeaconBlockHeader{}, &codegen_views.Phase0BeaconBlockHeader{}},
		},
		{
			name: "BeaconState",
			s:    &bellatrix.BeaconState{},
			s2:   &codegen.BellatrixBeaconState{},
			s3:   []any{&codegen_views.BeaconState{}, &codegen_views.BellatrixBeaconState{}},
		},
		{
			name: "Checkpoint",
			s:    &phase0.Checkpoint{},
			s2:   &codegen.Checkpoint{},
			s3:   []any{&codegen_views.Checkpoint{}, &codegen_views.Phase0Checkpoint{}},
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
			name: "Eth1Data",
			s:    &phase0.ETH1Data{},
			s2:   &codegen.ETH1Data{},
			s3:   []any{&codegen_views.ETH1Data{}, &codegen_views.Phase0ETH1Data{}},
		},
		{
			name: "ExecutionPayload",
			s:    &bellatrix.ExecutionPayload{},
			s2:   &codegen.BellatrixExecutionPayload{},
			s3:   []any{&codegen_views.ExecutionPayload{}, &codegen_views.BellatrixExecutionPayload{}},
		},
		{
			name: "ExecutionPayloadHeader",
			s:    &bellatrix.ExecutionPayloadHeader{},
			s2:   &codegen.BellatrixExecutionPayloadHeader{},
			s3:   []any{&codegen_views.ExecutionPayloadHeader{}, &codegen_views.BellatrixExecutionPayloadHeader{}},
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
			name: "IndexedAttestation",
			s:    &phase0.IndexedAttestation{},
			s2:   &codegen.IndexedAttestation{},
			s3:   []any{&codegen_views.IndexedAttestation{}, &codegen_views.Phase0IndexedAttestation{}},
		},
		{
			name: "PendingAttestation",
			s:    &phase0.PendingAttestation{},
			s2:   &codegen.PendingAttestation{},
			s3:   []any{&codegen_views.PendingAttestation{}, &codegen_views.Phase0PendingAttestation{}},
		},
		{
			name: "ProposerSlashing",
			s:    &phase0.ProposerSlashing{},
			s2:   &codegen.ProposerSlashing{},
			s3:   []any{&codegen_views.ProposerSlashing{}, &codegen_views.Phase0ProposerSlashing{}},
		},
		{
			name: "SignedAggregateAndProof",
			s:    &phase0.SignedAggregateAndProof{},
			s2:   &codegen.SignedAggregateAndProof{},
			s3:   []any{&codegen_views.SignedAggregateAndProof{}, &codegen_views.Phase0SignedAggregateAndProof{}},
		},
		{
			name: "SignedBeaconBlock",
			s:    &bellatrix.SignedBeaconBlock{},
			s2:   &codegen.BellatrixSignedBeaconBlock{},
			s3:   []any{&codegen_views.SignedBeaconBlock{}, &codegen_views.BellatrixSignedBeaconBlock{}},
		},
		{
			name: "SignedBeaconBlockHeader",
			s:    &phase0.SignedBeaconBlockHeader{},
			s2:   &codegen.SignedBeaconBlockHeader{},
			s3:   []any{&codegen_views.SignedBeaconBlockHeader{}, &codegen_views.Phase0SignedBeaconBlockHeader{}},
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
	}

	mainnetRes := runForkConsensusSpecTest(t, "bellatrix", "mainnet", tests)
	minimalRes := runForkConsensusSpecTest(t, "bellatrix", "minimal", tests)
	if !mainnetRes && !minimalRes {
		t.Skipf("Fork bellatrix not found in test data")
	}
}
