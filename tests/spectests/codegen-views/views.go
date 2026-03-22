package codegen

import (
	"github.com/holiman/uint256"
	"github.com/prysmaticlabs/go-bitfield"
)

// Phase0 views
type Phase0AggregateAndProof struct {
	AggregatorIndex ValidatorIndex
	Aggregate       *Phase0Attestation
	SelectionProof  BLSSignature `ssz-size:"96"`
}

type Phase0Attestation struct {
	AggregationBits bitfield.Bitlist `dynssz-max:"MAX_VALIDATORS_PER_COMMITTEE" ssz-max:"2048"`
	Data            *Phase0AttestationData
	Signature       BLSSignature `ssz-size:"96"`
}

type Phase0AttestationData struct {
	Slot            Slot
	Index           CommitteeIndex
	BeaconBlockRoot Root `ssz-size:"32"`
	Source          *Phase0Checkpoint
	Target          *Phase0Checkpoint
}

type Phase0AttesterSlashing struct {
	Attestation1 *Phase0IndexedAttestation
	Attestation2 *Phase0IndexedAttestation
}

type Phase0BeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *Phase0BeaconBlockBody
}

type Phase0BeaconBlockBody struct {
	RANDAOReveal      BLSSignature `ssz-size:"96"`
	ETH1Data          *Phase0ETH1Data
	Graffiti          [32]byte                     `ssz-size:"32"`
	ProposerSlashings []*Phase0ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings []*Phase0AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations      []*Phase0Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits          []*Phase0Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits    []*Phase0SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
}

type Phase0BeaconBlockHeader struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	BodyRoot      Root `ssz-size:"32"`
}

type Phase0BeaconState struct {
	GenesisTime                 uint64
	GenesisValidatorsRoot       Root `ssz-size:"32"`
	Slot                        Slot
	Fork                        *Phase0Fork
	LatestBlockHeader           *Phase0BeaconBlockHeader
	BlockRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots             []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                    *Phase0ETH1Data
	ETH1DataVotes               []*Phase0ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex            uint64
	Validators                  []*Phase0Validator          `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                    []Gwei                      `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                 []Root                      `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                   []Gwei                      `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochAttestations   []*Phase0PendingAttestation `dynssz-max:"MAX_ATTESTATIONS*SLOTS_PER_EPOCH" ssz-max:"4096"`
	CurrentEpochAttestations    []*Phase0PendingAttestation `dynssz-max:"MAX_ATTESTATIONS*SLOTS_PER_EPOCH" ssz-max:"4096"`
	JustificationBits           bitfield.Bitvector4         `ssz-size:"1"`
	PreviousJustifiedCheckpoint *Phase0Checkpoint
	CurrentJustifiedCheckpoint  *Phase0Checkpoint
	FinalizedCheckpoint         *Phase0Checkpoint
}

type Phase0Checkpoint struct {
	Epoch Epoch
	Root  Root `ssz-size:"32"`
}

type Phase0Deposit struct {
	Proof [][]byte `dynssz-size:"DEPOSIT_CONTRACT_TREE_DEPTH+1,32" ssz-size:"33,32"`
	Data  *Phase0DepositData
}

type Phase0DepositData struct {
	PublicKey             BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials []byte    `ssz-size:"32"`
	Amount                Gwei
	Signature             BLSSignature `ssz-size:"96"`
}

type Phase0DepositMessage struct {
	PublicKey             BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials []byte    `ssz-size:"32"`
	Amount                Gwei
}

type Phase0ETH1Data struct {
	DepositRoot  Root `ssz-size:"32"`
	DepositCount uint64
	BlockHash    []byte `ssz-size:"32"`
}

type Phase0Fork struct {
	PreviousVersion Version `ssz-size:"4"`
	CurrentVersion  Version `ssz-size:"4"`
	Epoch           Epoch
}

type Phase0ForkData struct {
	CurrentVersion        Version `ssz-size:"4"`
	GenesisValidatorsRoot Root    `ssz-size:"32"`
}

type Phase0IndexedAttestation struct {
	AttestingIndices []uint64 `ssz-max:"2048"`
	Data             *Phase0AttestationData
	Signature        BLSSignature `ssz-size:"96"`
}

type Phase0PendingAttestation struct {
	AggregationBits bitfield.Bitlist `ssz-max:"2048"`
	Data            *Phase0AttestationData
	InclusionDelay  Slot
	ProposerIndex   ValidatorIndex
}

type Phase0ProposerSlashing struct {
	SignedHeader1 *Phase0SignedBeaconBlockHeader
	SignedHeader2 *Phase0SignedBeaconBlockHeader
}

type Phase0SignedAggregateAndProof struct {
	Message   *Phase0AggregateAndProof
	Signature BLSSignature `ssz-size:"96"`
}

type Phase0SignedBeaconBlock struct {
	Message   *Phase0BeaconBlock
	Signature BLSSignature `ssz-size:"96"`
}

type Phase0SignedBeaconBlockHeader struct {
	Message   *Phase0BeaconBlockHeader
	Signature BLSSignature `ssz-size:"96"`
}

type Phase0SignedVoluntaryExit struct {
	Message   *Phase0VoluntaryExit
	Signature BLSSignature `ssz-size:"96"`
}

type Phase0Validator struct {
	PublicKey                  BLSPubKey `ssz-size:"48"`
	WithdrawalCredentials      []byte    `ssz-size:"32"`
	EffectiveBalance           Gwei
	Slashed                    bool
	ActivationEligibilityEpoch Epoch
	ActivationEpoch            Epoch
	ExitEpoch                  Epoch
	WithdrawableEpoch          Epoch
}

type Phase0VoluntaryExit struct {
	Epoch          Epoch
	ValidatorIndex ValidatorIndex
}

// Altair views

type AltairBeaconBlock struct {
	Slot          Slot
	ProposerIndex ValidatorIndex
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          *AltairBeaconBlockBody
}

type AltairBeaconBlockBody struct {
	RANDAOReveal      BLSSignature `ssz-size:"96"`
	ETH1Data          *Phase0ETH1Data
	Graffiti          [32]byte                     `ssz-size:"32"`
	ProposerSlashings []*Phase0ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings []*Phase0AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations      []*Phase0Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits          []*Phase0Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits    []*Phase0SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate     *AltairSyncAggregate
}

type AltairBeaconState struct {
	GenesisTime                 uint64
	GenesisValidatorsRoot       Root `ssz-size:"32"`
	Slot                        Slot
	Fork                        *Phase0Fork
	LatestBlockHeader           *Phase0BeaconBlockHeader
	BlockRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                  []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots             []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                    *Phase0ETH1Data
	ETH1DataVotes               []*Phase0ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex            uint64
	Validators                  []*Phase0Validator   `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                    []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                 []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                   []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation  []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits           bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint *Phase0Checkpoint
	CurrentJustifiedCheckpoint  *Phase0Checkpoint
	FinalizedCheckpoint         *Phase0Checkpoint
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
	ETH1Data          *Phase0ETH1Data
	Graffiti          [32]byte                     `ssz-size:"32"`
	ProposerSlashings []*Phase0ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings []*Phase0AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations      []*Phase0Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits          []*Phase0Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits    []*Phase0SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate     *AltairSyncAggregate
	ExecutionPayload  *BellatrixExecutionPayload
}

type BellatrixBeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        Root `ssz-size:"32"`
	Slot                         Slot
	Fork                         *Phase0Fork
	LatestBlockHeader            *Phase0BeaconBlockHeader
	BlockRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots              []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *Phase0ETH1Data
	ETH1DataVotes                []*Phase0ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Phase0Validator   `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                     []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                  []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                    []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Phase0Checkpoint
	CurrentJustifiedCheckpoint   *Phase0Checkpoint
	FinalizedCheckpoint          *Phase0Checkpoint
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
	ETH1Data              *Phase0ETH1Data
	Graffiti              [32]byte                     `ssz-size:"32"`
	ProposerSlashings     []*Phase0ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings     []*Phase0AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations          []*Phase0Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits              []*Phase0Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits        []*Phase0SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate         *AltairSyncAggregate
	ExecutionPayload      *CapellaExecutionPayload
	BLSToExecutionChanges []*CapellaSignedBLSToExecutionChange `dynssz-max:"MAX_BLS_TO_EXECUTION_CHANGES" ssz-max:"16"`
}

type CapellaBeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        Root `ssz-size:"32"`
	Slot                         Slot
	Fork                         *Phase0Fork
	LatestBlockHeader            *Phase0BeaconBlockHeader
	BlockRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots              []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *Phase0ETH1Data
	ETH1DataVotes                []*Phase0ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Phase0Validator   `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                     []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                  []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                    []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Phase0Checkpoint
	CurrentJustifiedCheckpoint   *Phase0Checkpoint
	FinalizedCheckpoint          *Phase0Checkpoint
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
	ETH1Data              *Phase0ETH1Data
	Graffiti              [32]byte                     `ssz-size:"32"`
	ProposerSlashings     []*Phase0ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS" ssz-max:"16"`
	AttesterSlashings     []*Phase0AttesterSlashing    `dynssz-max:"MAX_ATTESTER_SLASHINGS" ssz-max:"2"`
	Attestations          []*Phase0Attestation         `dynssz-max:"MAX_ATTESTATIONS"       ssz-max:"128"`
	Deposits              []*Phase0Deposit             `dynssz-max:"MAX_DEPOSITS"           ssz-max:"16"`
	VoluntaryExits        []*Phase0SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"    ssz-max:"16"`
	SyncAggregate         *AltairSyncAggregate
	ExecutionPayload      *DenebExecutionPayload
	BLSToExecutionChanges []*CapellaSignedBLSToExecutionChange `dynssz-max:"MAX_BLS_TO_EXECUTION_CHANGES"   ssz-max:"16"`
	BlobKZGCommitments    []KZGCommitment                      `dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK" ssz-max:"4096" ssz-size:"?,48"`
}

type DenebBeaconState struct {
	GenesisTime                  uint64
	GenesisValidatorsRoot        Root `ssz-size:"32"`
	Slot                         Slot
	Fork                         *Phase0Fork
	LatestBlockHeader            *Phase0BeaconBlockHeader
	BlockRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                   []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots              []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                     *Phase0ETH1Data
	ETH1DataVotes                []*Phase0ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex             uint64
	Validators                   []*Phase0Validator   `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                     []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                  []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                    []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation   []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits            bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint  *Phase0Checkpoint
	CurrentJustifiedCheckpoint   *Phase0Checkpoint
	FinalizedCheckpoint          *Phase0Checkpoint
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
	SignedBlockHeader           *Phase0SignedBeaconBlockHeader
	KZGCommitmentInclusionProof KZGCommitmentInclusionProof `dynssz-size:"KZG_COMMITMENT_INCLUSION_PROOF_DEPTH,32" ssz-size:"17,32"`
}

type DenebExecutionPayload struct {
	ParentHash           Hash32           `ssz-size:"32"`
	FeeRecipient         ExecutionAddress `ssz-size:"20"`
	StateRoot            Root             `ssz-size:"32"`
	ReceiptsRoot         Root             `ssz-size:"32"`
	LogsBloom            [256]byte        `ssz-size:"256"`
	PrevRandao           [32]byte         `ssz-size:"32"`
	BlockNumber          uint64
	GasLimit             uint64
	GasUsed              uint64
	Timestamp            uint64
	ExtraData            []byte               `dynssz-max:"MAX_EXTRA_DATA_BYTES"                                   ssz-max:"32"`
	BaseFeePerGasUint256 *uint256.Int         `ssz-size:"32"`
	BlockHash            Hash32               `ssz-size:"32"`
	Transactions         []Transaction        `dynssz-max:"MAX_TRANSACTIONS_PER_PAYLOAD,MAX_BYTES_PER_TRANSACTION" ssz-max:"1048576,1073741824" ssz-size:"?,?"`
	Withdrawals          []*CapellaWithdrawal `dynssz-max:"MAX_WITHDRAWALS_PER_PAYLOAD"                            ssz-max:"16"`
	BlobGasUsed          uint64
	ExcessBlobGas        uint64
}

type DenebExecutionPayloadHeader struct {
	ParentHash           Hash32           `ssz-size:"32"`
	FeeRecipient         ExecutionAddress `ssz-size:"20"`
	StateRoot            Root             `ssz-size:"32"`
	ReceiptsRoot         Root             `ssz-size:"32"`
	LogsBloom            [256]byte        `ssz-size:"256"`
	PrevRandao           [32]byte         `ssz-size:"32"`
	BlockNumber          uint64
	GasLimit             uint64
	GasUsed              uint64
	Timestamp            uint64
	ExtraData            []byte       `ssz-max:"32"`
	BaseFeePerGasUint256 *uint256.Int `ssz-size:"32"`
	BlockHash            Hash32       `ssz-size:"32"`
	TransactionsRoot     Root         `ssz-size:"32"`
	WithdrawalsRoot      Root         `ssz-size:"32"`
	BlobGasUsed          uint64
	ExcessBlobGas        uint64
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
	Data            *Phase0AttestationData
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
	ETH1Data              *Phase0ETH1Data
	Graffiti              [32]byte                     `ssz-size:"32"`
	ProposerSlashings     []*Phase0ProposerSlashing    `dynssz-max:"MAX_PROPOSER_SLASHINGS"         ssz-max:"16"`
	AttesterSlashings     []*ElectraAttesterSlashing   `dynssz-max:"MAX_ATTESTER_SLASHINGS_ELECTRA" ssz-max:"1"`
	Attestations          []*ElectraAttestation        `dynssz-max:"MAX_ATTESTATIONS_ELECTRA"       ssz-max:"8"`
	Deposits              []*Phase0Deposit             `dynssz-max:"MAX_DEPOSITS"                   ssz-max:"16"`
	VoluntaryExits        []*Phase0SignedVoluntaryExit `dynssz-max:"MAX_VOLUNTARY_EXITS"            ssz-max:"16"`
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
	Fork                          *Phase0Fork
	LatestBlockHeader             *Phase0BeaconBlockHeader
	BlockRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots               []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                      *Phase0ETH1Data
	ETH1DataVotes                 []*Phase0ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex              uint64
	Validators                    []*Phase0Validator   `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                      []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                   []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                     []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation     []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits             bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Phase0Checkpoint
	CurrentJustifiedCheckpoint    *Phase0Checkpoint
	FinalizedCheckpoint           *Phase0Checkpoint
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
	Data             *Phase0AttestationData
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
type FuluBeaconState struct { // TODO: Update to Fulu (latest spectests release still uses Electra)
	GenesisTime                   uint64
	GenesisValidatorsRoot         Root `ssz-size:"32"`
	Slot                          Slot
	Fork                          *Phase0Fork
	LatestBlockHeader             *Phase0BeaconBlockHeader
	BlockRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	StateRoots                    []Root `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32" ssz-size:"8192,32"`
	HistoricalRoots               []Root `dynssz-max:"HISTORICAL_ROOTS_LIMIT"        ssz-max:"16777216" ssz-size:"?,32"`
	ETH1Data                      *Phase0ETH1Data
	ETH1DataVotes                 []*Phase0ETH1Data `dynssz-max:"EPOCHS_PER_ETH1_VOTING_PERIOD*SLOTS_PER_EPOCH" ssz-max:"2048"`
	ETH1DepositIndex              uint64
	Validators                    []*Phase0Validator   `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	Balances                      []Gwei               `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	RANDAOMixes                   []Root               `dynssz-size:"EPOCHS_PER_HISTORICAL_VECTOR,32" ssz-size:"65536,32"`
	Slashings                     []Gwei               `dynssz-size:"EPOCHS_PER_SLASHINGS_VECTOR"     ssz-size:"8192"`
	PreviousEpochParticipation    []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	CurrentEpochParticipation     []ParticipationFlags `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"         ssz-max:"1099511627776"`
	JustificationBits             bitfield.Bitvector4  `ssz-size:"1"`
	PreviousJustifiedCheckpoint   *Phase0Checkpoint
	CurrentJustifiedCheckpoint    *Phase0Checkpoint
	FinalizedCheckpoint           *Phase0Checkpoint
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
