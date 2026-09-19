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
type BuilderIndex uint64
type BlockAccessList []byte

// Phase0 types
type AggregateAndProof struct {
	AggregatorIndex ValidatorIndex
	Aggregate       *Attestation
	SelectionProof  BLSSignature `ssz-size:"96"`
}

type Attestation struct {
	AggregationBits bitfield.Bitlist `dynssz-max:"MAX_VALIDATORS_PER_COMMITTEE" ssz-max:"2048"`
	Data            *AttestationData
	Signature       BLSSignature `ssz-size:"96"`
}

type AttestationData struct {
	Slot            Slot
	Index           CommitteeIndex
	BeaconBlockRoot Root `ssz-size:"32"`
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
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *BeaconBlockBody
}

type BeaconBlockBody struct {
	RANDAOReveal      BLSSignature `ssz-size:"96"`
	ETH1Data          *ETH1Data
	Graffiti          [32]byte               `ssz-size:"32"`
	ProposerSlashings []*ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings []*AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations      []*Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits          []*Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits    []*SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
}

type BeaconBlockHeader struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	BodyRoot      Root `ssz-size:"32"`
}

type BeaconState struct {
	GenesisTime                 uint64
	GenesisValidatorsRoot       Root `ssz-size:"32"`
	Slot                        Slot
	Fork                        *Fork
	LatestBlockHeader           *BeaconBlockHeader
	BlockRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots             []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                    *ETH1Data
	ETH1DataVotes               []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex            uint64
	Validators                  []*Validator          `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                    []Gwei                `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                 []Root                `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                   []Gwei                `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochAttestations   []*PendingAttestation `dynssz-max:"MAX_ATTESTATIONS*SLOTS_PER_EPOCH" ssz-max:"4096"`
	CurrentEpochAttestations    []*PendingAttestation `dynssz-max:"MAX_ATTESTATIONS*SLOTS_PER_EPOCH" ssz-max:"4096"`
	JustificationBits           bitfield.Bitvector4   `ssz-size:"1"`
	PreviousJustifiedCheckpoint *Checkpoint
	CurrentJustifiedCheckpoint  *Checkpoint
	FinalizedCheckpoint         *Checkpoint
}

type Checkpoint struct {
	Epoch Epoch
	Root  Root `ssz-size:"32"`
}

type Deposit struct {
	Proof [][]byte `dynssz-size:"DEPOSIT_CONTRACT_TREE_DEPTH+1,32" ssz-size:"33,32"`
	Data  *DepositData
}

type DepositData struct {
	PublicKey             BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials []byte    `ssz-size:"32"`
	Amount                Gwei
	Signature             BLSSignature `ssz-size:"96"`
}

type DepositMessage struct {
	PublicKey             BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials []byte    `ssz-size:"32"`
	Amount                Gwei
}

type ETH1Data struct {
	DepositRoot  Root `ssz-size:"32"`
	DepositCount uint64
	BlockHash    []byte `ssz-size:"32"`
}

type Fork struct {
	PreviousVersion Version `ssz-size:"4"`
	CurrentVersion  Version `ssz-size:"4"`
	Epoch           Epoch
}

type ForkData struct {
	CurrentVersion        Version `ssz-size:"4"`
	GenesisValidatorsRoot Root    `ssz-size:"32"`
}

type IndexedAttestation struct {
	AttestingIndices []uint64 `ssz-max:"2048"`
	Data             *AttestationData
	Signature        BLSSignature `ssz-size:"96"`
}

type PendingAttestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"2048"`
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
	Signature BLSSignature `ssz-size:"96"`
}

type SignedBeaconBlock struct {
	Message   *BeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

type SignedBeaconBlockHeader struct {
	Message   *BeaconBlockHeader
	Signature BLSSignature `ssz-size:"96"`
}

type SignedVoluntaryExit struct {
	Message   *VoluntaryExit
	Signature BLSSignature `ssz-size:"96"`
}

type Validator struct {
	PublicKey                  BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials      []byte    `ssz-size:"32"`
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
type AltairBeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *AltairBeaconBlockBody
}

type AltairBeaconBlockBody struct {
	RANDAOReveal      BLSSignature `ssz-size:"96"`
	ETH1Data          *ETH1Data
	Graffiti          [32]byte               `ssz-size:"32"`
	ProposerSlashings []*ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings []*AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations      []*Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits          []*Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits    []*SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate     *AltairSyncAggregate
}

type AltairBeaconState struct {
	GenesisTime                 uint64
	GenesisValidatorsRoot       Root `ssz-size:"32"`
	Slot                        Slot
	Fork                        *Fork
	LatestBlockHeader           *BeaconBlockHeader
	BlockRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots             []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                    *ETH1Data
	ETH1DataVotes               []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex            uint64
	Validators                  []*Validator         `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                    []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                 []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                   []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation  []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits           bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint *Checkpoint
	CurrentJustifiedCheckpoint  *Checkpoint
	FinalizedCheckpoint         *Checkpoint
	InactivityScores            []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentSyncCommittee        *AltairSyncCommittee
	NextSyncCommittee           *AltairSyncCommittee
}

type AltairContributionAndProof struct {
	AggregatorIndex ValidatorIndex
	Contribution    *AltairSyncCommitteeContribution
	SelectionProof  BLSSignature `ssz-size:"96"`
}

type AltairSignedBeaconBlock struct {
	Message   *AltairBeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

type AltairSignedContributionAndProof struct {
	Message   *AltairContributionAndProof
	Signature BLSSignature `ssz-size:"96"`
}

type AltairSyncAggregate struct {
	SyncCommitteeBits      bitfield.Bitvector512 `dynssz-size:"SYNC_COMMITTEE_SIZE/8" ssz-size:"64"`
	SyncCommitteeSignature BLSSignature          `ssz-size:"96"`
}

type AltairSyncCommittee struct {
	Pubkeys         []BLSPubKey `dynssz-size:"SYNC_COMMITTEE_SIZE,48" ssz-size:"512,48"`
	AggregatePubkey BLSPubKey   `ssz-size:"48"`
}

type AltairSyncCommitteeContribution struct {
	Slot              Slot
	BeaconBlockRoot   Root `ssz-size:"32"`
	SubcommitteeIndex uint64
	AggregationBits   bitfield.Bitvector128 `dynssz-size:"SYNC_COMMITTEE_SIZE/4/8" ssz-size:"16"`
	Signature         BLSSignature          `ssz-size:"96"`
}

type AltairSyncCommitteeMessage struct {
	Slot            Slot
	BeaconBlockRoot Root `ssz-size:"32"`
	ValidatorIndex  ValidatorIndex
	Signature       BLSSignature `ssz-size:"96"`
}

// Bellatrix types
type BellatrixBeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *BellatrixBeaconBlockBody
}

type BellatrixBeaconBlockBody struct {
	RANDAOReveal      BLSSignature `ssz-size:"96"`
	ETH1Data          *ETH1Data
	Graffiti          [32]byte               `ssz-size:"32"`
	ProposerSlashings []*ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings []*AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations      []*Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits          []*Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits    []*SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate     *AltairSyncAggregate
	ExecutionPayload  *BellatrixExecutionPayload
}

type BellatrixBeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        Root `ssz-size:"32"`
	Slot                         Slot
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots              []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *ETH1Data
	ETH1DataVotes                []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Validator         `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                     []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                  []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                    []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentSyncCommittee         *AltairSyncCommittee
	NextSyncCommittee            *AltairSyncCommittee
	LatestExecutionPayloadHeader *BellatrixExecutionPayloadHeader
}

type BellatrixExecutionPayload struct {
	ParentHash    Hash32           `ssz-size:"32"`
	FeeRecipient  ExecutionAddress `ssz-size:"20"`
	StateRoot     [32]byte         `ssz-size:"32"`
	ReceiptsRoot  [32]byte         `ssz-size:"32"`
	LogsBloom     [256]byte        `ssz-size:"256"`
	PrevRandao    [32]byte         `ssz-size:"32"`
	BlockNumber   uint64
	GasLimit      uint64
	GasUsed       uint64
	Timestamp     uint64
	ExtraData     []byte        `dynssz-max:"MAX_EXTRA_DATA_BYTES"                                   ssz-max:"32"`
	BaseFeePerGas [32]byte      `ssz-size:"32"`
	BlockHash     Hash32        `ssz-size:"32"`
	Transactions  []Transaction `dynssz-max:"MAX_TRANSACTIONS_PER_PAYLOAD,MAX_BYTES_PER_TRANSACTION" ssz-max:"1048576,1073741824"`
}

type BellatrixExecutionPayloadHeader struct {
	ParentHash       Hash32           `ssz-size:"32"`
	FeeRecipient     ExecutionAddress `ssz-size:"20"`
	StateRoot        [32]byte         `ssz-size:"32"`
	ReceiptsRoot     [32]byte         `ssz-size:"32"`
	LogsBloom        [256]byte        `ssz-size:"256"`
	PrevRandao       [32]byte         `ssz-size:"32"`
	BlockNumber      uint64
	GasLimit         uint64
	GasUsed          uint64
	Timestamp        uint64
	ExtraData        []byte   `ssz-max:"32"`
	BaseFeePerGas    [32]byte `ssz-size:"32"`
	BlockHash        Hash32   `ssz-size:"32"`
	TransactionsRoot Root     `ssz-size:"32"`
}

type BellatrixSignedBeaconBlock struct {
	Message   *BellatrixBeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

// Capella types
type CapellaBeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *CapellaBeaconBlockBody
}

type CapellaBeaconBlockBody struct {
	RANDAOReveal          BLSSignature `ssz-size:"96"`
	ETH1Data              *ETH1Data
	Graffiti              [32]byte               `ssz-size:"32"`
	ProposerSlashings     []*ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings     []*AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations          []*Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits              []*Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate         *AltairSyncAggregate
	ExecutionPayload      *CapellaExecutionPayload
	BLSToExecutionChanges []*CapellaSignedBLSToExecutionChange `dynssz-max:"MAX_BLS_TO_EXECUTION_CHANGES" ssz-max:"16"`
}

type CapellaBeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        Root `ssz-size:"32"`
	Slot                         Slot
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots              []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *ETH1Data
	ETH1DataVotes                []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Validator         `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                     []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                  []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                    []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentSyncCommittee         *AltairSyncCommittee
	NextSyncCommittee            *AltairSyncCommittee
	LatestExecutionPayloadHeader *CapellaExecutionPayloadHeader
	NextWithdrawalIndex          WithdrawalIndex
	NextWithdrawalValidatorIndex ValidatorIndex
	HistoricalSummaries          []*CapellaHistoricalSummary `dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216"`
}

type CapellaBLSToExecutionChange struct {
	ValidatorIndex     ValidatorIndex
	FromBLSPubkey      BLSPubKey        `ssz-size:"48"`
	ToExecutionAddress ExecutionAddress `ssz-size:"20"`
}

type CapellaExecutionPayload struct {
	ParentHash    Hash32           `ssz-size:"32"`
	FeeRecipient  ExecutionAddress `ssz-size:"20"`
	StateRoot     [32]byte         `ssz-size:"32"`
	ReceiptsRoot  [32]byte         `ssz-size:"32"`
	LogsBloom     [256]byte        `ssz-size:"256"`
	PrevRandao    [32]byte         `ssz-size:"32"`
	BlockNumber   uint64
	GasLimit      uint64
	GasUsed       uint64
	Timestamp     uint64
	ExtraData     []byte               `dynssz-max:"MAX_EXTRA_DATA_BYTES"                                   ssz-max:"32"`
	BaseFeePerGas [32]byte             `ssz-size:"32"`
	BlockHash     Hash32               `ssz-size:"32"`
	Transactions  []Transaction        `dynssz-max:"MAX_TRANSACTIONS_PER_PAYLOAD,MAX_BYTES_PER_TRANSACTION" ssz-max:"1048576,1073741824" ssz-size:"?,?"`
	Withdrawals   []*CapellaWithdrawal `dynssz-max:"MAX_WITHDRAWALS_PER_PAYLOAD"                            ssz-max:"16"`
}

type CapellaExecutionPayloadHeader struct {
	ParentHash       Hash32           `ssz-size:"32"`
	FeeRecipient     ExecutionAddress `ssz-size:"20"`
	StateRoot        [32]byte         `ssz-size:"32"`
	ReceiptsRoot     [32]byte         `ssz-size:"32"`
	LogsBloom        [256]byte        `ssz-size:"256"`
	PrevRandao       [32]byte         `ssz-size:"32"`
	BlockNumber      uint64
	GasLimit         uint64
	GasUsed          uint64
	Timestamp        uint64
	ExtraData        []byte   `ssz-max:"32"`
	BaseFeePerGas    [32]byte `ssz-size:"32"`
	BlockHash        Hash32   `ssz-size:"32"`
	TransactionsRoot Root     `ssz-size:"32"`
	WithdrawalsRoot  Root     `ssz-size:"32"`
}

type CapellaHistoricalSummary struct {
	BlockSummaryRoot Root `ssz-size:"32"`
	StateSummaryRoot Root `ssz-size:"32"`
}

type CapellaSignedBeaconBlock struct {
	Message   *CapellaBeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

type CapellaSignedBLSToExecutionChange struct {
	Message   *CapellaBLSToExecutionChange
	Signature BLSSignature `ssz-size:"96"`
}

type CapellaWithdrawal struct {
	Index          WithdrawalIndex
	ValidatorIndex ValidatorIndex
	Address        ExecutionAddress `ssz-size:"20"`
	Amount         Gwei
}

// Deneb types
type DenebBeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *DenebBeaconBlockBody
}

type DenebBeaconBlockBody struct {
	RANDAOReveal          BLSSignature `ssz-size:"96"`
	ETH1Data              *ETH1Data
	Graffiti              [32]byte               `ssz-size:"32"`
	ProposerSlashings     []*ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings     []*AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations          []*Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits              []*Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate         *AltairSyncAggregate
	ExecutionPayload      *DenebExecutionPayload
	BLSToExecutionChanges []*CapellaSignedBLSToExecutionChange `dynssz-max:"MAX_BLS_TO_EXECUTION_CHANGES"   ssz-max:"16"`
	BlobKZGCommitments    []KZGCommitment                      `dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK" ssz-max:"4096" ssz-size:"?,48"`
}

type DenebBeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        Root `ssz-size:"32"`
	Slot                         Slot
	Fork                         *Fork
	LatestBlockHeader            *BeaconBlockHeader
	BlockRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots              []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *ETH1Data
	ETH1DataVotes                []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Validator         `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                     []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                  []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                    []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Checkpoint
	CurrentJustifiedCheckpoint   *Checkpoint
	FinalizedCheckpoint          *Checkpoint
	InactivityScores             []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentSyncCommittee         *AltairSyncCommittee
	NextSyncCommittee            *AltairSyncCommittee
	LatestExecutionPayloadHeader *DenebExecutionPayloadHeader
	NextWithdrawalIndex          WithdrawalIndex
	NextWithdrawalValidatorIndex ValidatorIndex
	HistoricalSummaries          []*CapellaHistoricalSummary `dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216"`
}

type DenebBlobIdentifier struct {
	BlockRoot Root `ssz-size:"32"`
	Index     BlobIndex
}

type DenebBlobSidecar struct {
	Index                       BlobIndex
	Blob                        Blob          `ssz-size:"131072"`
	KZGCommitment               KZGCommitment `ssz-size:"48"`
	KZGProof                    KZGProof      `ssz-size:"48"`
	SignedBlockHeader           *SignedBeaconBlockHeader
	KZGCommitmentInclusionProof KZGCommitmentInclusionProof `dynssz-size:"KZG_COMMITMENT_INCLUSION_PROOF_DEPTH,32" ssz-size:"17,32"`
}

type DenebExecutionPayload struct {
	ParentHash    Hash32           `ssz-size:"32"`
	FeeRecipient  ExecutionAddress `ssz-size:"20"`
	StateRoot     Root             `ssz-size:"32"`
	ReceiptsRoot  Root             `ssz-size:"32"`
	LogsBloom     [256]byte        `ssz-size:"256"`
	PrevRandao    [32]byte         `ssz-size:"32"`
	BlockNumber   uint64
	GasLimit      uint64
	GasUsed       uint64
	Timestamp     uint64
	ExtraData     []byte               `dynssz-max:"MAX_EXTRA_DATA_BYTES"                                   ssz-max:"32"`
	BaseFeePerGas *uint256.Int         `ssz-size:"32"`
	BlockHash     Hash32               `ssz-size:"32"`
	Transactions  []Transaction        `dynssz-max:"MAX_TRANSACTIONS_PER_PAYLOAD,MAX_BYTES_PER_TRANSACTION" ssz-max:"1048576,1073741824" ssz-size:"?,?"`
	Withdrawals   []*CapellaWithdrawal `dynssz-max:"MAX_WITHDRAWALS_PER_PAYLOAD"                            ssz-max:"16"`
	BlobGasUsed   uint64
	ExcessBlobGas uint64
}

type DenebExecutionPayloadHeader struct {
	ParentHash       Hash32           `ssz-size:"32"`
	FeeRecipient     ExecutionAddress `ssz-size:"20"`
	StateRoot        Root             `ssz-size:"32"`
	ReceiptsRoot     Root             `ssz-size:"32"`
	LogsBloom        [256]byte        `ssz-size:"256"`
	PrevRandao       [32]byte         `ssz-size:"32"`
	BlockNumber      uint64
	GasLimit         uint64
	GasUsed          uint64
	Timestamp        uint64
	ExtraData        []byte       `ssz-max:"32"`
	BaseFeePerGas    *uint256.Int `ssz-size:"32"`
	BlockHash        Hash32       `ssz-size:"32"`
	TransactionsRoot Root         `ssz-size:"32"`
	WithdrawalsRoot  Root         `ssz-size:"32"`
	BlobGasUsed      uint64
	ExcessBlobGas    uint64
}

type DenebSignedBeaconBlock struct {
	Message   *DenebBeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

// Electra types
type ElectraAggregateAndProof struct {
	AggregatorIndex ValidatorIndex
	Aggregate       *ElectraAttestation
	SelectionProof  BLSSignature `ssz-size:"96"`
}

type ElectraAttestation struct {
	AggregationBits bitfield.Bitlist `dynssz-max:"MAX_VALIDATORS_PER_COMMITTEE*MAX_COMMITTEES_PER_SLOT" ssz-max:"131072"`
	Data            *AttestationData
	Signature       BLSSignature         `ssz-size:"96"`
	CommitteeBits   bitfield.Bitvector64 `dynssz-size:"MAX_COMMITTEES_PER_SLOT/8" ssz-size:"8"`
}

type ElectraAttesterSlashing struct {
	Attestation1 *ElectraIndexedAttestation
	Attestation2 *ElectraIndexedAttestation
}

type ElectraBeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *ElectraBeaconBlockBody
}

type ElectraBeaconBlockBody struct {
	RANDAOReveal          BLSSignature `ssz-size:"96"`
	ETH1Data              *ETH1Data
	Graffiti              [32]byte                   `ssz-size:"32"`
	ProposerSlashings     []*ProposerSlashing        `dynssz-max:"MAX_PROPOSER_SLASHINGS"         ssz-max:"16"`
	AttesterSlashings     []*ElectraAttesterSlashing `dynssz-max:"MAX_ATTESTER_SLASHINGS_ELECTRA" ssz-max:"1"`
	Attestations          []*ElectraAttestation      `dynssz-max:"MAX_ATTESTATIONS_ELECTRA"       ssz-max:"8"`
	Deposits              []*Deposit                 `dynssz-max:"MAX_DEPOSITS"                   ssz-max:"16"`
	VoluntaryExits        []*SignedVoluntaryExit     `dynssz-max:"MAX_VOLUNTARY_EXITS"            ssz-max:"16"`
	SyncAggregate         *AltairSyncAggregate
	ExecutionPayload      *DenebExecutionPayload
	BLSToExecutionChanges []*CapellaSignedBLSToExecutionChange `dynssz-max:"MAX_BLS_TO_EXECUTION_CHANGES"   ssz-max:"16"`
	BlobKZGCommitments    []KZGCommitment                      `dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK" ssz-max:"4096" ssz-size:"?,48"`
	ExecutionRequests     *ElectraExecutionRequests
}

type ElectraBeaconState struct {
	GenesisTime                   uint64
	GenesisValidatorsRoot         Root `ssz-size:"32"`
	Slot                          Slot
	Fork                          *Fork
	LatestBlockHeader             *BeaconBlockHeader
	BlockRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots               []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                      *ETH1Data
	ETH1DataVotes                 []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex              uint64
	Validators                    []*Validator         `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                      []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                   []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                     []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation     []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits             bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Checkpoint
	CurrentJustifiedCheckpoint    *Checkpoint
	FinalizedCheckpoint           *Checkpoint
	InactivityScores              []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentSyncCommittee          *AltairSyncCommittee
	NextSyncCommittee             *AltairSyncCommittee
	LatestExecutionPayloadHeader  *DenebExecutionPayloadHeader
	NextWithdrawalIndex           WithdrawalIndex
	NextWithdrawalValidatorIndex  ValidatorIndex
	HistoricalSummaries           []*CapellaHistoricalSummary `dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216"`
	DepositRequestsStartIndex     uint64
	DepositBalanceToConsume       Gwei
	ExitBalanceToConsume          Gwei
	EarliestExitEpoch             Epoch
	ConsolidationBalanceToConsume Gwei
	EarliestConsolidationEpoch    Epoch
	PendingDeposits               []*ElectraPendingDeposit           `dynssz-max:"PENDING_DEPOSITS_LIMIT"            ssz-max:"134217728"`
	PendingPartialWithdrawals     []*ElectraPendingPartialWithdrawal `dynssz-max:"PENDING_PARTIAL_WITHDRAWALS_LIMIT" ssz-max:"134217728"`
	PendingConsolidations         []*ElectraPendingConsolidation     `dynssz-max:"PENDING_CONSOLIDATIONS_LIMIT"      ssz-max:"262144"`
}

type ElectraConsolidation struct {
	SourceIndex ValidatorIndex
	TargetIndex ValidatorIndex
	Epoch       Epoch
}

type ElectraConsolidationRequest struct {
	SourceAddress ExecutionAddress `ssz-size:"20"`
	SourcePubkey  BLSPubKey        `ssz-size:"48"`
	TargetPubkey  BLSPubKey        `ssz-size:"48"`
}

type ElectraDepositRequest struct {
	Pubkey                BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials []byte    `ssz-size:"32"`
	Amount                Gwei
	Signature             BLSSignature `ssz-size:"96"`
	Index                 uint64
}

type ElectraExecutionRequests struct {
	Deposits       []*ElectraDepositRequest       `dynssz-max:"MAX_DEPOSIT_REQUESTS_PER_PAYLOAD"       ssz-max:"8192"`
	Withdrawals    []*ElectraWithdrawalRequest    `dynssz-max:"MAX_WITHDRAWAL_REQUESTS_PER_PAYLOAD"    ssz-max:"16"`
	Consolidations []*ElectraConsolidationRequest `dynssz-max:"MAX_CONSOLIDATION_REQUESTS_PER_PAYLOAD" ssz-max:"2"`
}

type ElectraIndexedAttestation struct {
	AttestingIndices []uint64 `dynssz-max:"MAX_VALIDATORS_PER_COMMITTEE*MAX_COMMITTEES_PER_SLOT" ssz-max:"131072"`
	Data             *AttestationData
	Signature        BLSSignature `ssz-size:"96"`
}

type ElectraPendingDeposit struct {
	Pubkey                BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials []byte    `ssz-size:"32"`
	Amount                Gwei
	Signature             BLSSignature `ssz-size:"96"`
	Slot                  Slot
}

type ElectraPendingConsolidation struct {
	SourceIndex ValidatorIndex
	TargetIndex ValidatorIndex
}

type ElectraPendingPartialWithdrawal struct {
	ValidatorIndex    ValidatorIndex
	Amount            Gwei
	WithdrawableEpoch Epoch
}

type ElectraSignedAggregateAndProof struct {
	Message   *ElectraAggregateAndProof
	Signature BLSSignature `ssz-size:"96"`
}

type ElectraSignedBeaconBlock struct {
	Message   *ElectraBeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

type ElectraWithdrawalRequest struct {
	SourceAddress   ExecutionAddress `ssz-size:"20"`
	ValidatorPubkey BLSPubKey        `ssz-size:"48"`
	Amount          Gwei
}

// Fulu types
type FuluBeaconState struct {
	GenesisTime                   uint64
	GenesisValidatorsRoot         Root `ssz-size:"32"`
	Slot                          Slot
	Fork                          *Fork
	LatestBlockHeader             *BeaconBlockHeader
	BlockRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots               []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                      *ETH1Data
	ETH1DataVotes                 []*ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex              uint64
	Validators                    []*Validator         `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                      []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                   []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                     []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation     []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits             bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Checkpoint
	CurrentJustifiedCheckpoint    *Checkpoint
	FinalizedCheckpoint           *Checkpoint
	InactivityScores              []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT" ssz-max:"1099511627776"`
	CurrentSyncCommittee          *AltairSyncCommittee
	NextSyncCommittee             *AltairSyncCommittee
	LatestExecutionPayloadHeader  *DenebExecutionPayloadHeader
	NextWithdrawalIndex           WithdrawalIndex
	NextWithdrawalValidatorIndex  ValidatorIndex
	HistoricalSummaries           []*CapellaHistoricalSummary `dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216"`
	DepositRequestsStartIndex     uint64
	DepositBalanceToConsume       Gwei
	ExitBalanceToConsume          Gwei
	EarliestExitEpoch             Epoch
	ConsolidationBalanceToConsume Gwei
	EarliestConsolidationEpoch    Epoch
	PendingDeposits               []*ElectraPendingDeposit           `dynssz-max:"PENDING_DEPOSITS_LIMIT"            ssz-max:"134217728"`
	PendingPartialWithdrawals     []*ElectraPendingPartialWithdrawal `dynssz-max:"PENDING_PARTIAL_WITHDRAWALS_LIMIT" ssz-max:"134217728"`
	PendingConsolidations         []*ElectraPendingConsolidation     `dynssz-max:"PENDING_CONSOLIDATIONS_LIMIT"            ssz-max:"262144"`
	ProposerLookahead             []ValidatorIndex                   `dynssz-size:"(MIN_SEED_LOOKAHEAD+1)*SLOTS_PER_EPOCH" ssz-size:"64"`
}

// Gloas types
type GloasAggregateAndProof struct {
	AggregatorIndex ValidatorIndex
	Aggregate       *GloasAttestation
	SelectionProof  BLSSignature `ssz-size:"96"`
}

type GloasAttestation struct {
	AggregationBits bitfield.Bitlist     `ssz-index:"0" ssz-type:"progressive-bitlist"`
	Data            *AttestationData     `ssz-index:"1"`
	Signature       BLSSignature         `ssz-index:"2" ssz-size:"96"`
	CommitteeBits   bitfield.Bitvector64 `ssz-index:"3" dynssz-size:"MAX_COMMITTEES_PER_SLOT/8" ssz-size:"8"`
}

type GloasAttesterSlashing struct {
	Attestation1 *GloasIndexedAttestation
	Attestation2 *GloasIndexedAttestation
}

type GloasBeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *GloasBeaconBlockBody
}

type GloasBeaconBlockBody struct {
	RANDAOReveal              BLSSignature                         `ssz-index:"0"  ssz-size:"96"`
	ETH1Data                  *ETH1Data                            `ssz-index:"1"`
	Graffiti                  [32]byte                             `ssz-index:"2"  ssz-size:"32"`
	ProposerSlashings         []*ProposerSlashing                  `ssz-index:"3"  ssz-type:"progressive-list"`
	AttesterSlashings         []*GloasAttesterSlashing             `ssz-index:"4"  ssz-type:"progressive-list"`
	Attestations              []*GloasAttestation                  `ssz-index:"5"  ssz-type:"progressive-list"`
	Deposits                  []*Deposit                           `ssz-index:"6"  ssz-type:"progressive-list"`
	VoluntaryExits            []*SignedVoluntaryExit               `ssz-index:"7"  ssz-type:"progressive-list"`
	SyncAggregate             *AltairSyncAggregate                 `ssz-index:"8"`
	BLSToExecutionChanges     []*CapellaSignedBLSToExecutionChange `ssz-index:"9"  ssz-type:"progressive-list"`
	SignedExecutionPayloadBid *GloasSignedExecutionPayloadBid      `ssz-index:"10"`
	PayloadAttestations       []*GloasPayloadAttestation           `ssz-index:"11" ssz-type:"progressive-list"`
	ParentExecutionRequests   *GloasExecutionRequests              `ssz-index:"12"`
}

type GloasBeaconState struct {
	GenesisTime                   uint64                             `ssz-index:"0"`
	GenesisValidatorsRoot         Root                               `ssz-index:"1"  ssz-size:"32"`
	Slot                          Slot                               `ssz-index:"2"`
	Fork                          *Fork                              `ssz-index:"3"`
	LatestBlockHeader             *BeaconBlockHeader                 `ssz-index:"4"`
	BlockRoots                    []Root                             `ssz-index:"5"  dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                    []Root                             `ssz-index:"6"  dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots               []Root                             `ssz-index:"7"  dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                      *ETH1Data                          `ssz-index:"8"`
	ETH1DataVotes                 []*ETH1Data                        `ssz-index:"9"  dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex              uint64                             `ssz-index:"10"`
	Validators                    []*Validator                       `ssz-index:"11" ssz-type:"progressive-list"`
	Balances                      []Gwei                             `ssz-index:"12" ssz-type:"progressive-list"`
	RANDAOMixes                   []Root                             `ssz-index:"13" dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                     []Gwei                             `ssz-index:"14" dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR" ssz-size:"8192"`
	PreviousEpochParticipation    []ParticipationFlags               `ssz-index:"15" ssz-type:"progressive-list"`
	CurrentEpochParticipation     []ParticipationFlags               `ssz-index:"16" ssz-type:"progressive-list"`
	JustificationBits             bitfield.Bitvector4                `ssz-index:"17" ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Checkpoint                        `ssz-index:"18"`
	CurrentJustifiedCheckpoint    *Checkpoint                        `ssz-index:"19"`
	FinalizedCheckpoint           *Checkpoint                        `ssz-index:"20"`
	InactivityScores              []uint64                           `ssz-index:"21" ssz-type:"progressive-list"`
	CurrentSyncCommittee          *AltairSyncCommittee               `ssz-index:"22"`
	NextSyncCommittee             *AltairSyncCommittee               `ssz-index:"23"`
	LatestBlockHash               Hash32                             `ssz-index:"24" ssz-size:"32"`
	NextWithdrawalIndex           WithdrawalIndex                    `ssz-index:"25"`
	NextWithdrawalValidatorIndex  ValidatorIndex                     `ssz-index:"26"`
	HistoricalSummaries           []*CapellaHistoricalSummary        `ssz-index:"27" dynssz-max:"HISTORICAL_ROOTS_LIMIT" ssz-max:"16777216"`
	DepositRequestsStartIndex     uint64                             `ssz-index:"28"`
	DepositBalanceToConsume       Gwei                               `ssz-index:"29"`
	ExitBalanceToConsume          Gwei                               `ssz-index:"30"`
	EarliestExitEpoch             Epoch                              `ssz-index:"31"`
	ConsolidationBalanceToConsume Gwei                               `ssz-index:"32"`
	EarliestConsolidationEpoch    Epoch                              `ssz-index:"33"`
	PendingDeposits               []*ElectraPendingDeposit           `ssz-index:"34" ssz-type:"progressive-list"`
	PendingPartialWithdrawals     []*ElectraPendingPartialWithdrawal `ssz-index:"35" ssz-type:"progressive-list"`
	PendingConsolidations         []*ElectraPendingConsolidation     `ssz-index:"36" ssz-type:"progressive-list"`
	ProposerLookahead             []ValidatorIndex                   `ssz-index:"37" dynssz-size:"(MIN_SEED_LOOKAHEAD+1)*SLOTS_PER_EPOCH" ssz-size:"64"`
	Builders                      []*GloasBuilder                    `ssz-index:"38" ssz-type:"progressive-list"`
	NextWithdrawalBuilderIndex    BuilderIndex                       `ssz-index:"39"`
	ExecutionPayloadAvailability  []uint8                            `ssz-index:"40" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT/8" ssz-size:"1024"`
	BuilderPendingPayments        []*GloasBuilderPendingPayment      `ssz-index:"41" dynssz-size:"SLOTS_PER_EPOCH*2" ssz-size:"64"`
	BuilderPendingWithdrawals     []*GloasBuilderPendingWithdrawal   `ssz-index:"42" ssz-type:"progressive-list"`
	LatestExecutionPayloadBid     *GloasExecutionPayloadBid          `ssz-index:"43"`
	PayloadExpectedWithdrawals    []*CapellaWithdrawal               `ssz-index:"44" ssz-type:"progressive-list"`
	PTCWindow                     [][]ValidatorIndex                 `ssz-index:"45" dynssz-size:"(2+MIN_SEED_LOOKAHEAD)*SLOTS_PER_EPOCH,PTC_SIZE" ssz-size:"96,512"`
}

type GloasBuilder struct {
	PublicKey         BLSPubKey `ssz-size:"48"`
	Version           uint8
	ExecutionAddress  ExecutionAddress `ssz-size:"20"`
	Balance           Gwei
	DepositEpoch      Epoch
	WithdrawableEpoch Epoch
}

type GloasBuilderDepositRequest struct {
	Pubkey                BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials []byte    `ssz-size:"32"`
	Amount                Gwei
	Signature             BLSSignature `ssz-size:"96"`
}

type GloasBuilderExitRequest struct {
	SourceAddress ExecutionAddress `ssz-size:"20"`
	Pubkey        BLSPubKey        `ssz-size:"48"`
}

type GloasBuilderPendingPayment struct {
	Weight        Gwei
	Withdrawal    *GloasBuilderPendingWithdrawal
	ProposerIndex ValidatorIndex
}

type GloasBuilderPendingWithdrawal struct {
	FeeRecipient ExecutionAddress `ssz-size:"20"`
	Amount       Gwei
	BuilderIndex BuilderIndex
}

type GloasExecutionPayload struct {
	ParentHash      Hash32               `ssz-index:"0"  ssz-size:"32"`
	FeeRecipient    ExecutionAddress     `ssz-index:"1"  ssz-size:"20"`
	StateRoot       Root                 `ssz-index:"2"  ssz-size:"32"`
	ReceiptsRoot    Root                 `ssz-index:"3"  ssz-size:"32"`
	LogsBloom       [256]byte            `ssz-index:"4"  ssz-size:"256"`
	PrevRandao      [32]byte             `ssz-index:"5"  ssz-size:"32"`
	BlockNumber     uint64               `ssz-index:"6"`
	GasLimit        uint64               `ssz-index:"7"`
	GasUsed         uint64               `ssz-index:"8"`
	Timestamp       uint64               `ssz-index:"9"`
	ExtraData       []byte               `ssz-index:"10" dynssz-max:"MAX_EXTRA_DATA_BYTES" ssz-max:"32"`
	BaseFeePerGas   *uint256.Int         `ssz-index:"11" ssz-size:"32"`
	BlockHash       Hash32               `ssz-index:"12" ssz-size:"32"`
	Transactions    []Transaction        `ssz-index:"13" ssz-type:"progressive-list,progressive-list"`
	Withdrawals     []*CapellaWithdrawal `ssz-index:"14" ssz-type:"progressive-list"`
	BlobGasUsed     uint64               `ssz-index:"15"`
	ExcessBlobGas   uint64               `ssz-index:"16"`
	BlockAccessList BlockAccessList      `ssz-index:"17" ssz-type:"progressive-list"`
	SlotNumber      uint64               `ssz-index:"18"`
}

type GloasExecutionPayloadBid struct {
	ParentBlockHash       Hash32           `ssz-index:"0"  ssz-size:"32"`
	ParentBlockRoot       Root             `ssz-index:"1"  ssz-size:"32"`
	BlockHash             Hash32           `ssz-index:"2"  ssz-size:"32"`
	PrevRandao            Root             `ssz-index:"3"  ssz-size:"32"`
	FeeRecipient          ExecutionAddress `ssz-index:"4"  ssz-size:"20"`
	GasLimit              uint64           `ssz-index:"5"`
	BuilderIndex          BuilderIndex     `ssz-index:"6"`
	Slot                  Slot             `ssz-index:"7"`
	Value                 Gwei             `ssz-index:"8"`
	ExecutionPayment      Gwei             `ssz-index:"9"`
	BlobKZGCommitments    []KZGCommitment  `ssz-index:"10" ssz-type:"progressive-list"`
	ExecutionRequestsRoot Root             `ssz-index:"11" ssz-size:"32"`
}

type GloasExecutionPayloadEnvelope struct {
	Payload               *GloasExecutionPayload  `ssz-index:"0"`
	ExecutionRequests     *GloasExecutionRequests `ssz-index:"1"`
	BuilderIndex          BuilderIndex            `ssz-index:"2"`
	BeaconBlockRoot       Root                    `ssz-index:"3" ssz-size:"32"`
	ParentBeaconBlockRoot Root                    `ssz-index:"4" ssz-size:"32"`
}

type GloasExecutionRequests struct {
	Deposits        []*ElectraDepositRequest       `ssz-index:"0" ssz-type:"progressive-list"`
	Withdrawals     []*ElectraWithdrawalRequest    `ssz-index:"1" ssz-type:"progressive-list"`
	Consolidations  []*ElectraConsolidationRequest `ssz-index:"2" ssz-type:"progressive-list"`
	BuilderDeposits []*GloasBuilderDepositRequest  `ssz-index:"3" ssz-type:"progressive-list"`
	BuilderExits    []*GloasBuilderExitRequest     `ssz-index:"4" ssz-type:"progressive-list"`
}

type GloasIndexedAttestation struct {
	AttestingIndices []uint64         `ssz-index:"0" ssz-type:"progressive-list"`
	Data             *AttestationData `ssz-index:"1"`
	Signature        BLSSignature     `ssz-index:"2" ssz-size:"96"`
}

type GloasIndexedPayloadAttestation struct {
	AttestingIndices []ValidatorIndex             `ssz-index:"0" dynssz-max:"PTC_SIZE" ssz-max:"512"`
	Data             *GloasPayloadAttestationData `ssz-index:"1"`
	Signature        BLSSignature                 `ssz-index:"2" ssz-size:"96"`
}

type GloasPayloadAttestation struct {
	AggregationBits bitfield.Bitvector512        `ssz-index:"0" dynssz-size:"PTC_SIZE/8" ssz-size:"64"`
	Data            *GloasPayloadAttestationData `ssz-index:"1"`
	Signature       BLSSignature                 `ssz-index:"2" ssz-size:"96"`
}

type GloasPayloadAttestationData struct {
	BeaconBlockRoot   Root `ssz-size:"32"`
	Slot              Slot
	PayloadPresent    bool
	BlobDataAvailable bool
}

type GloasPayloadAttestationMessage struct {
	ValidatorIndex ValidatorIndex
	Data           *GloasPayloadAttestationData
	Signature      BLSSignature `ssz-size:"96"`
}

type GloasProposerPreferences struct {
	DependentRoot  Root `ssz-size:"32"`
	ProposalSlot   Slot
	ValidatorIndex ValidatorIndex
	FeeRecipient   ExecutionAddress `ssz-size:"20"`
	TargetGasLimit uint64
}

type GloasSignedAggregateAndProof struct {
	Message   *GloasAggregateAndProof
	Signature BLSSignature `ssz-size:"96"`
}

type GloasSignedBeaconBlock struct {
	Message   *GloasBeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

type GloasSignedExecutionPayloadBid struct {
	Message   *GloasExecutionPayloadBid
	Signature BLSSignature `ssz-size:"96"`
}

type GloasSignedExecutionPayloadEnvelope struct {
	Message   *GloasExecutionPayloadEnvelope
	Signature BLSSignature `ssz-size:"96"`
}

type GloasSignedProposerPreferences struct {
	Message   *GloasProposerPreferences
	Signature BLSSignature `ssz-size:"96"`
}
