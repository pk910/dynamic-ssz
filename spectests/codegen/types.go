package codegen

import (
	"github.com/attestantio/go-eth2-client/spec/altair"
	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/electra"
	"github.com/attestantio/go-eth2-client/spec/phase0"
)

//go:generate ./generate.sh

// Phase0 types
type AggregateAndProof phase0.AggregateAndProof
type Attestation phase0.Attestation
type AttestationData phase0.AttestationData
type AttesterSlashing phase0.AttesterSlashing
type BeaconBlock phase0.BeaconBlock
type BeaconBlockBody phase0.BeaconBlockBody
type BeaconBlockHeader phase0.BeaconBlockHeader
type BeaconState phase0.BeaconState
type Checkpoint phase0.Checkpoint
type Deposit phase0.Deposit
type DepositData phase0.DepositData
type DepositMessage phase0.DepositMessage
type Eth1Data phase0.ETH1Data
type Fork phase0.Fork
type ForkData phase0.ForkData
type IndexedAttestation phase0.IndexedAttestation
type PendingAttestation phase0.PendingAttestation
type ProposerSlashing phase0.ProposerSlashing
type SignedAggregateAndProof phase0.SignedAggregateAndProof
type SignedBeaconBlock phase0.SignedBeaconBlock
type SignedBeaconBlockHeader phase0.SignedBeaconBlockHeader
type SignedVoluntaryExit phase0.SignedVoluntaryExit
type Validator phase0.Validator
type VoluntaryExit phase0.VoluntaryExit

// Altair types
type AltairBeaconBlock altair.BeaconBlock
type AltairBeaconBlockBody altair.BeaconBlockBody
type AltairBeaconState altair.BeaconState
type AltairContributionAndProof altair.ContributionAndProof
type AltairSignedBeaconBlock altair.SignedBeaconBlock
type AltairSignedContributionAndProof altair.SignedContributionAndProof
type AltairSyncAggregate altair.SyncAggregate
type AltairSyncCommittee altair.SyncCommittee
type AltairSyncCommitteeContribution altair.SyncCommitteeContribution
type AltairSyncCommitteeMessage altair.SyncCommitteeMessage

// Bellatrix types
type BellatrixBeaconBlock bellatrix.BeaconBlock
type BellatrixBeaconBlockBody bellatrix.BeaconBlockBody
type BellatrixBeaconState bellatrix.BeaconState
type BellatrixExecutionPayload bellatrix.ExecutionPayload
type BellatrixExecutionPayloadHeader bellatrix.ExecutionPayloadHeader
type BellatrixSignedBeaconBlock bellatrix.SignedBeaconBlock

// Capella types
type CapellaBeaconBlock capella.BeaconBlock
type CapellaBeaconBlockBody capella.BeaconBlockBody
type CapellaBeaconState capella.BeaconState
type CapellaBLSToExecutionChange capella.BLSToExecutionChange
type CapellaExecutionPayload capella.ExecutionPayload
type CapellaExecutionPayloadHeader capella.ExecutionPayloadHeader
type CapellaHistoricalSummary capella.HistoricalSummary
type CapellaSignedBeaconBlock capella.SignedBeaconBlock
type CapellaSignedBLSToExecutionChange capella.SignedBLSToExecutionChange
type CapellaWithdrawal capella.Withdrawal

// Deneb types
type DenebBeaconBlock deneb.BeaconBlock
type DenebBeaconBlockBody deneb.BeaconBlockBody
type DenebBeaconState deneb.BeaconState
type DenebBlobIdentifier deneb.BlobIdentifier
type DenebBlobSidecar deneb.BlobSidecar
type DenebExecutionPayload deneb.ExecutionPayload
type DenebExecutionPayloadHeader deneb.ExecutionPayloadHeader
type DenebSignedBeaconBlock deneb.SignedBeaconBlock

// Electra types
type ElectraAggregateAndProof electra.AggregateAndProof
type ElectraAttestation electra.Attestation
type ElectraAttesterSlashing electra.AttesterSlashing
type ElectraBeaconBlock electra.BeaconBlock
type ElectraBeaconBlockBody electra.BeaconBlockBody
type ElectraBeaconState electra.BeaconState
type ElectraConsolidation electra.Consolidation
type ElectraConsolidationRequest electra.ConsolidationRequest
type ElectraDepositRequest electra.DepositRequest
type ElectraExecutionRequests electra.ExecutionRequests
type ElectraIndexedAttestation electra.IndexedAttestation
type ElectraPendingDeposit electra.PendingDeposit
type ElectraPendingConsolidation electra.PendingConsolidation
type ElectraPendingPartialWithdrawal electra.PendingPartialWithdrawal
type ElectraSignedAggregateAndProof electra.SignedAggregateAndProof
type ElectraSignedBeaconBlock electra.SignedBeaconBlock
type ElectraWithdrawalRequest electra.WithdrawalRequest

// Fulu types
type FuluBeaconState electra.BeaconState // TODO: Update to Fulu (latest spectests release still uses Electra)
