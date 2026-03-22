package codegen

import (
	"github.com/holiman/uint256"
	"github.com/prysmaticlabs/go-bitfield"
)

//go:generate ./generate.sh

// base types
type ValidatorIndex uint64
type BLSSignature [96]byte
type Slot uint64
type CommitteeIndex uint64
type Root [32]byte
type Gwei uint64
type Epoch uint64
type Version [4]byte
type BLSPubKey [48]byte
type ParticipationFlags uint8
type ExecutionAddress [20]byte
type Hash32 [32]byte
type Transaction []byte
type WithdrawalIndex uint64
type KZGCommitment [48]byte
type BlobIndex uint64
type Blob [131072]byte
type KZGProof [48]byte
type KZGCommitmentInclusionProofElement [32]byte
type KZGCommitmentInclusionProof []KZGCommitmentInclusionProofElement

// types
type AggregateAndProof struct {
	AggregatorIndex ValidatorIndex
	Aggregate       *Attestation
	SelectionProof  BLSSignature
}

type Attestation struct {
	AggregationBits bitfield.Bitlist
	Data            *AttestationData
	Signature       BLSSignature
	CommitteeBits   bitfield.Bitvector64
}

type AttestationData struct {
	Slot            Slot
	Index           CommitteeIndex
	BeaconBlockRoot Root
	Source          *Checkpoint
	Target          *Checkpoint
}

type AttesterSlashing struct {
	Attestation1 *IndexedAttestation
	Attestation2 *IndexedAttestation
}

type BeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root
	StateRoot     Root
	Body          *BeaconBlockBody
}

type BeaconBlockBody struct {
	RANDAOReveal          BLSSignature
	ETH1Data              *ETH1Data
	Graffiti              [32]byte
	ProposerSlashings     []*ProposerSlashing
	AttesterSlashings     []*AttesterSlashing
	Attestations          []*Attestation
	Deposits              []*Deposit
	VoluntaryExits        []*SignedVoluntaryExit
	SyncAggregate         *SyncAggregate
	ExecutionPayload      *ExecutionPayload
	BLSToExecutionChanges []*SignedBLSToExecutionChange
	BlobKZGCommitments    []KZGCommitment
	ExecutionRequests     *ExecutionRequests
}

type BeaconBlockHeader struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root
	StateRoot     Root
	BodyRoot      Root
}

type BeaconState struct {
	GenesisTime                   uint64
	GenesisValidatorsRoot         Root
	Slot                          Slot
	Fork                          *Fork
	LatestBlockHeader             *BeaconBlockHeader
	BlockRoots                    []Root
	StateRoots                    []Root
	HistoricalRoots               []Root
	ETH1Data                      *ETH1Data
	ETH1DataVotes                 []*ETH1Data
	ETH1DepositIndex              uint64
	Validators                    []*Validator
	Balances                      []Gwei
	RANDAOMixes                   []Root
	Slashings                     []Gwei
	PreviousEpochAttestations     []*PendingAttestation
	CurrentEpochAttestations      []*PendingAttestation
	PreviousEpochParticipation    []ParticipationFlags
	CurrentEpochParticipation     []ParticipationFlags
	JustificationBits             bitfield.Bitvector4
	PreviousJustifiedCheckpoint   *Checkpoint
	CurrentJustifiedCheckpoint    *Checkpoint
	FinalizedCheckpoint           *Checkpoint
	InactivityScores              []uint64
	CurrentSyncCommittee          *SyncCommittee
	NextSyncCommittee             *SyncCommittee
	LatestExecutionPayloadHeader  *ExecutionPayloadHeader
	NextWithdrawalIndex           WithdrawalIndex
	NextWithdrawalValidatorIndex  ValidatorIndex
	HistoricalSummaries           []*HistoricalSummary
	DepositRequestsStartIndex     uint64
	DepositBalanceToConsume       Gwei
	ExitBalanceToConsume          Gwei
	EarliestExitEpoch             Epoch
	ConsolidationBalanceToConsume Gwei
	EarliestConsolidationEpoch    Epoch
	PendingDeposits               []*PendingDeposit
	PendingPartialWithdrawals     []*PendingPartialWithdrawal
	PendingConsolidations         []*PendingConsolidation
}

type Checkpoint struct {
	Epoch Epoch
	Root  Root
}

type Deposit struct {
	Proof [][]byte
	Data  *DepositData
}

type DepositData struct {
	PublicKey             BLSPubKey
	WithdrawalCredentials []byte
	Amount                Gwei
	Signature             BLSSignature
}

type DepositMessage struct {
	PublicKey             BLSPubKey
	WithdrawalCredentials []byte
	Amount                Gwei
}

type ETH1Data struct {
	DepositRoot  Root
	DepositCount uint64
	BlockHash    []byte
}

type Fork struct {
	PreviousVersion Version
	CurrentVersion  Version
	Epoch           Epoch
}

type ForkData struct {
	CurrentVersion        Version
	GenesisValidatorsRoot Root
}

type IndexedAttestation struct {
	AttestingIndices []uint64
	Data             *AttestationData
	Signature        BLSSignature
}

type PendingAttestation struct {
	AggregationBits bitfield.Bitlist
	Data            *AttestationData
	InclusionDelay  Slot
	ProposerIndex   ValidatorIndex
}

type ProposerSlashing struct {
	SignedHeader1 *SignedBeaconBlockHeader
	SignedHeader2 *SignedBeaconBlockHeader
}

type SignedAggregateAndProof struct {
	Message   *AggregateAndProof
	Signature BLSSignature
}

type SignedBeaconBlock struct {
	Message   *BeaconBlock
	Signature BLSSignature
}

type SignedBeaconBlockHeader struct {
	Message   *BeaconBlockHeader
	Signature BLSSignature
}

type SignedVoluntaryExit struct {
	Message   *VoluntaryExit
	Signature BLSSignature
}

type Validator struct {
	PublicKey                  BLSPubKey
	WithdrawalCredentials      []byte
	EffectiveBalance           Gwei
	Slashed                    bool
	ActivationEligibilityEpoch Epoch
	ActivationEpoch            Epoch
	ExitEpoch                  Epoch
	WithdrawableEpoch          Epoch
}

type VoluntaryExit struct {
	Epoch          Epoch
	ValidatorIndex ValidatorIndex
}

// Altair types
type ContributionAndProof struct {
	AggregatorIndex ValidatorIndex
	Contribution    *SyncCommitteeContribution
	SelectionProof  BLSSignature
}

type SignedContributionAndProof struct {
	Message   *ContributionAndProof
	Signature BLSSignature
}

type SyncAggregate struct {
	SyncCommitteeBits      bitfield.Bitvector512
	SyncCommitteeSignature BLSSignature
}

type SyncCommittee struct {
	Pubkeys         []BLSPubKey
	AggregatePubkey BLSPubKey
}

type SyncCommitteeContribution struct {
	Slot              Slot
	BeaconBlockRoot   Root
	SubcommitteeIndex uint64
	AggregationBits   bitfield.Bitvector128
	Signature         BLSSignature
}

type SyncCommitteeMessage struct {
	Slot            Slot
	BeaconBlockRoot Root
	ValidatorIndex  ValidatorIndex
	Signature       BLSSignature
}

// Bellatrix types
type ExecutionPayload struct {
	ParentHash           Hash32
	FeeRecipient         ExecutionAddress
	StateRoot            Root
	ReceiptsRoot         Root
	LogsBloom            [256]byte
	PrevRandao           [32]byte
	BlockNumber          uint64
	GasLimit             uint64
	GasUsed              uint64
	Timestamp            uint64
	ExtraData            []byte
	BaseFeePerGas        [32]byte
	BaseFeePerGasUint256 *uint256.Int
	BlockHash            Hash32
	Transactions         []Transaction
	Withdrawals          []*Withdrawal
	BlobGasUsed          uint64
	ExcessBlobGas        uint64
}

type ExecutionPayloadHeader struct {
	ParentHash           Hash32
	FeeRecipient         ExecutionAddress
	StateRoot            Root
	ReceiptsRoot         Root
	LogsBloom            [256]byte
	PrevRandao           [32]byte
	BlockNumber          uint64
	GasLimit             uint64
	GasUsed              uint64
	Timestamp            uint64
	ExtraData            []byte
	BaseFeePerGas        [32]byte
	BaseFeePerGasUint256 *uint256.Int
	BlockHash            Hash32
	TransactionsRoot     Root
	WithdrawalsRoot      Root
	BlobGasUsed          uint64
	ExcessBlobGas        uint64
}

// Capella types
type BLSToExecutionChange struct {
	ValidatorIndex     ValidatorIndex
	FromBLSPubkey      BLSPubKey
	ToExecutionAddress ExecutionAddress
}

type HistoricalSummary struct {
	BlockSummaryRoot Root
	StateSummaryRoot Root
}

type SignedBLSToExecutionChange struct {
	Message   *BLSToExecutionChange
	Signature BLSSignature
}

type Withdrawal struct {
	Index          WithdrawalIndex
	ValidatorIndex ValidatorIndex
	Address        ExecutionAddress
	Amount         Gwei
}

// Deneb types
type BlobIdentifier struct {
	BlockRoot Root
	Index     BlobIndex
}

type BlobSidecar struct {
	Index                       BlobIndex
	Blob                        Blob
	KZGCommitment               KZGCommitment
	KZGProof                    KZGProof
	SignedBlockHeader           *SignedBeaconBlockHeader
	KZGCommitmentInclusionProof KZGCommitmentInclusionProof
}

// Electra types
type Consolidation struct {
	SourceIndex ValidatorIndex
	TargetIndex ValidatorIndex
	Epoch       Epoch
}

type ConsolidationRequest struct {
	SourceAddress ExecutionAddress
	SourcePubkey  BLSPubKey
	TargetPubkey  BLSPubKey
}

type DepositRequest struct {
	Pubkey                BLSPubKey
	WithdrawalCredentials []byte
	Amount                Gwei
	Signature             BLSSignature
	Index                 uint64
}

type ExecutionRequests struct {
	Deposits       []*DepositRequest
	Withdrawals    []*WithdrawalRequest
	Consolidations []*ConsolidationRequest
}

type PendingDeposit struct {
	Pubkey                BLSPubKey
	WithdrawalCredentials []byte
	Amount                Gwei
	Signature             BLSSignature
	Slot                  Slot
}

type PendingConsolidation struct {
	SourceIndex ValidatorIndex
	TargetIndex ValidatorIndex
}

type PendingPartialWithdrawal struct {
	ValidatorIndex    ValidatorIndex
	Amount            Gwei
	WithdrawableEpoch Epoch
}

type WithdrawalRequest struct {
	SourceAddress   ExecutionAddress
	ValidatorPubkey BLSPubKey
	Amount          Gwei
}
