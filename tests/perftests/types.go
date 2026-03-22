package perftests

import (
	"github.com/prysmaticlabs/go-bitfield"
)

// Basic types - using plain types to ensure compatibility with generated code
type Slot = uint64
type Epoch = uint64
type ValidatorIndex = uint64
type Gwei = uint64
type Root = [32]byte
type Hash32 = [32]byte
type BLSPubKey = [48]byte
type BLSSignature = [96]byte
type WithdrawalIndex = uint64
type ParticipationFlags = uint8
type KZGCommitment = [48]byte
type ExecutionAddress = [20]byte
type LogsBloom = [256]byte
type Uint256 = [32]byte

// Fork represents a fork
type Fork struct {
	PreviousVersion [4]byte
	CurrentVersion  [4]byte
	Epoch           Epoch
}

// Checkpoint represents a checkpoint
type Checkpoint struct {
	Epoch Epoch
	Root  Root
}

// BeaconBlockHeader represents a beacon block header
type BeaconBlockHeader struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	BodyRoot      Root `ssz-size:"32"`
}

// SignedBeaconBlockHeader represents a signed beacon block header
type SignedBeaconBlockHeader struct {
	Message   *BeaconBlockHeader
	Signature BLSSignature `ssz-size:"96"`
}

// ETH1Data represents eth1 data
type ETH1Data struct {
	DepositRoot  Root `ssz-size:"32"`
	DepositCount uint64
	BlockHash    Hash32 `ssz-size:"32"`
}

// Validator represents a validator
type Validator struct {
	Pubkey                     BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials      Hash32    `ssz-size:"32"`
	EffectiveBalance           Gwei
	Slashed                    bool
	ActivationEligibilityEpoch Epoch
	ActivationEpoch            Epoch
	ExitEpoch                  Epoch
	WithdrawableEpoch          Epoch
}

// ProposerSlashing represents a proposer slashing
type ProposerSlashing struct {
	SignedHeader1 *SignedBeaconBlockHeader
	SignedHeader2 *SignedBeaconBlockHeader
}

// AttestationData represents attestation data
type AttestationData struct {
	Slot            Slot
	Index           uint64
	BeaconBlockRoot Root `ssz-size:"32"`
	Source          *Checkpoint
	Target          *Checkpoint
}

// IndexedAttestation represents an indexed attestation
type IndexedAttestation struct {
	AttestingIndices []uint64 `ssz-max:"2048"`
	Data             *AttestationData
	Signature        BLSSignature `ssz-size:"96"`
}

// AttesterSlashing represents an attester slashing
type AttesterSlashing struct {
	Attestation1 *IndexedAttestation
	Attestation2 *IndexedAttestation
}

// Attestation represents an attestation
type Attestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"2048" ssz-type:"bitlist"`
	Data            *AttestationData
	Signature       BLSSignature `ssz-size:"96"`
}

// DepositData represents deposit data
type DepositData struct {
	Pubkey                BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials Hash32    `ssz-size:"32"`
	Amount                Gwei
	Signature             BLSSignature `ssz-size:"96"`
}

// Deposit represents a deposit
type Deposit struct {
	Proof [][]byte `ssz-size:"33,32"`
	Data  *DepositData
}

// VoluntaryExit represents a voluntary exit
type VoluntaryExit struct {
	Epoch          Epoch
	ValidatorIndex ValidatorIndex
}

// SignedVoluntaryExit represents a signed voluntary exit
type SignedVoluntaryExit struct {
	Message   *VoluntaryExit
	Signature BLSSignature `ssz-size:"96"`
}

// SyncAggregate represents a sync aggregate
type SyncAggregate struct {
	SyncCommitteeBits      bitfield.Bitvector512 `dynssz-size:"SYNC_COMMITTEE_SIZE/8" ssz-size:"64"`
	SyncCommitteeSignature BLSSignature          `ssz-size:"96"`
}

// SyncCommittee represents a sync committee
type SyncCommittee struct {
	Pubkeys         []BLSPubKey `dynssz-size:"SYNC_COMMITTEE_SIZE,48" ssz-size:"512,48"`
	AggregatePubkey BLSPubKey   `ssz-size:"48"`
}

// Withdrawal represents a withdrawal
type Withdrawal struct {
	Index          WithdrawalIndex
	ValidatorIndex ValidatorIndex
	Address        ExecutionAddress `ssz-size:"20"`
	Amount         Gwei
}

// BLSToExecutionChange represents a BLS to execution change
type BLSToExecutionChange struct {
	ValidatorIndex     ValidatorIndex
	FromBLSPubkey      BLSPubKey        `ssz-size:"48"`
	ToExecutionAddress ExecutionAddress `ssz-size:"20"`
}

// SignedBLSToExecutionChange represents a signed BLS to execution change
type SignedBLSToExecutionChange struct {
	Message   *BLSToExecutionChange
	Signature BLSSignature `ssz-size:"96"`
}

// HistoricalSummary represents a historical summary
type HistoricalSummary struct {
	BlockSummaryRoot Root `ssz-size:"32"`
	StateSummaryRoot Root `ssz-size:"32"`
}

// ExecutionPayload represents an execution payload (Deneb)
type ExecutionPayload struct {
	ParentHash    Hash32           `ssz-size:"32"`
	FeeRecipient  ExecutionAddress `ssz-size:"20"`
	StateRoot     Hash32           `ssz-size:"32"`
	ReceiptsRoot  Hash32           `ssz-size:"32"`
	LogsBloom     LogsBloom        `ssz-size:"256"`
	PrevRandao    Hash32           `ssz-size:"32"`
	BlockNumber   uint64
	GasLimit      uint64
	GasUsed       uint64
	Timestamp     uint64
	ExtraData     []byte        `dynssz-max:"MAX_EXTRA_DATA_BYTES" ssz-max:"32"`
	BaseFeePerGas Uint256       `ssz-size:"32"`
	BlockHash     Hash32        `ssz-size:"32"`
	Transactions  [][]byte      `dynssz-max:"MAX_TRANSACTIONS_PER_PAYLOAD,MAX_BYTES_PER_TRANSACTION" ssz-max:"1048576,1073741824" ssz-size:"?,?"`
	Withdrawals   []*Withdrawal `dynssz-max:"MAX_WITHDRAWALS_PER_PAYLOAD" ssz-max:"16"`
	BlobGasUsed   uint64
	ExcessBlobGas uint64
}

// ExecutionPayloadHeader represents an execution payload header (Deneb)
type ExecutionPayloadHeader struct {
	ParentHash       Hash32           `ssz-size:"32"`
	FeeRecipient     ExecutionAddress `ssz-size:"20"`
	StateRoot        Hash32           `ssz-size:"32"`
	ReceiptsRoot     Hash32           `ssz-size:"32"`
	LogsBloom        LogsBloom        `ssz-size:"256"`
	PrevRandao       Hash32           `ssz-size:"32"`
	BlockNumber      uint64
	GasLimit         uint64
	GasUsed          uint64
	Timestamp        uint64
	ExtraData        []byte  `dynssz-max:"MAX_EXTRA_DATA_BYTES" ssz-max:"32"`
	BaseFeePerGas    Uint256 `ssz-size:"32"`
	BlockHash        Hash32  `ssz-size:"32"`
	TransactionsRoot Root    `ssz-size:"32"`
	WithdrawalsRoot  Root    `ssz-size:"32"`
	BlobGasUsed      uint64
	ExcessBlobGas    uint64
}

// BeaconBlockBody represents a beacon block body (Deneb)
type BeaconBlockBody struct {
	RANDAOReveal          BLSSignature `ssz-size:"96"`
	ETH1Data              *ETH1Data
	Graffiti              Hash32                 `ssz-size:"32"`
	ProposerSlashings     []*ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings     []*AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations          []*Attestation         `dynssz-max:"MAX_ATTESTATIONS" ssz-max:"128"`
	Deposits              []*Deposit             `dynssz-max:"MAX_DEPOSITS" ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS" ssz-max:"16"`
	SyncAggregate         *SyncAggregate
	ExecutionPayload      *ExecutionPayload
	BLSToExecutionChanges []*SignedBLSToExecutionChange `dynssz-max:"MAX_BLS_TO_EXECUTION_CHANGES" ssz-max:"16"`
	BlobKZGCommitments    []KZGCommitment               `dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK" ssz-max:"4096" ssz-size:"?,48"`
}

// BeaconBlock represents a beacon block (Deneb)
type BeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *BeaconBlockBody
}

// SignedBeaconBlock represents a signed beacon block (Deneb)
type SignedBeaconBlock struct {
	Message   *BeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

// BeaconState represents a beacon state (Deneb)
type BeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        Root `ssz-size:"32"`
	Slot                         Slot
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots              []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *ETH1Data
	ETH1DataVotes                []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Validator         `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	Balances                     []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	RANDAOMixes                  []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                    []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR" ssz-size:"8192"`
	PreviousEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentSyncCommittee         *SyncCommittee
	NextSyncCommittee            *SyncCommittee
	LatestExecutionPayloadHeader *ExecutionPayloadHeader
	NextWithdrawalIndex          WithdrawalIndex
	NextWithdrawalValidatorIndex ValidatorIndex
	HistoricalSummaries          []*HistoricalSummary `dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216"`
}
