// Progressive Merkleization Example
//
// This example demonstrates all 4 progressive merkleization features:
// - M1: ProgressiveList (EIP-7916)
// - M2: ProgressiveBitlist (EIP-7916)
// - M3: ProgressiveContainer (EIP-7495)
// - M4: CompatibleUnion (EIP-7495)

package main

import (
	"fmt"
	"log"

	dynssz "github.com/pk910/dynamic-ssz"
)

func main() {
	fmt.Println("Progressive Merkleization Example")
	fmt.Println("==================================")

	// Create DynSsz instance
	ds := dynssz.NewDynSsz(nil)

	// Example 1: M1 & M2 - Progressive List and Bitlist (standalone)
	fmt.Println("\n1. M1 & M2: Progressive List and Bitlist Example:")

	// Create a progressive list
	type ProgressiveListExample struct {
		ValidatorList []uint64 `ssz-type:"progressive-list" ssz-max:"1000000"`
	}

	// Create a progressive bitlist
	type ProgressiveBitlistExample struct {
		ParticipationBits []byte `ssz-type:"progressive-bitlist" ssz-max:"100000"`
	}

	// Create large validator list
	validators := make([]uint64, 10000)
	for i := range validators {
		validators[i] = uint64(i + 1000000)
	}

	listExample := ProgressiveListExample{ValidatorList: validators}

	// Create participation bitlist (every 3rd validator participates)
	bitCount := 10000
	bitlistBytes := make([]byte, (bitCount+8)/8)
	for i := 0; i < bitCount; i++ {
		if i%3 == 0 {
			bitlistBytes[i/8] |= 1 << (i % 8)
		}
	}
	bitlistBytes[bitCount/8] |= 1 << (bitCount % 8) // delimiter bit

	bitlistExample := ProgressiveBitlistExample{ParticipationBits: bitlistBytes}

	// Serialize both
	listEncoded, err := ds.MarshalSSZ(&listExample)
	if err != nil {
		log.Fatal("Failed to marshal progressive list:", err)
	}

	bitlistEncoded, err := ds.MarshalSSZ(&bitlistExample)
	if err != nil {
		log.Fatal("Failed to marshal progressive bitlist:", err)
	}

	// Compute hash tree roots
	listRoot, err := ds.HashTreeRoot(&listExample)
	if err != nil {
		log.Fatal("Failed to compute list root:", err)
	}

	bitlistRoot, err := ds.HashTreeRoot(&bitlistExample)
	if err != nil {
		log.Fatal("Failed to compute bitlist root:", err)
	}

	fmt.Printf("   - Progressive List: %d validators, %d bytes, root: %x\n", len(validators), len(listEncoded), listRoot)
	fmt.Printf("   - Progressive Bitlist: %d bits, %d bytes, root: %x\n", bitCount, len(bitlistEncoded), bitlistRoot)

	// Example 2: M3 - Progressive Container
	fmt.Println("\n2. M3: Progressive Container Example:")

	// M3: Progressive Container - simulates a beacon block with extensible fields
	type BeaconBlock struct {
		Slot          uint64   `ssz-index:"0"`
		ProposerIndex uint64   `ssz-index:"1"`
		ParentRoot    [32]byte `ssz-index:"3"`
		StateRoot     [32]byte `ssz-index:"4"`
		// Note: Not including progressive list/bitlist here to avoid the bug
		ExtraData []byte `ssz-index:"5" ssz-max:"1024"`
	}

	block := BeaconBlock{
		Slot:          12345,
		ProposerIndex: 67890,
		ParentRoot:    [32]byte{1, 2, 3, 4, 5},
		StateRoot:     [32]byte{6, 7, 8, 9, 10},
		ExtraData:     []byte("Some extra beacon block data"),
	}

	// Serialize the progressive container
	blockEncoded, err := ds.MarshalSSZ(&block)
	if err != nil {
		log.Fatal("Failed to marshal beacon block:", err)
	}

	fmt.Printf("   - Progressive Container: %d bytes encoded\n", len(blockEncoded))

	root, err := ds.HashTreeRoot(&block)
	if err != nil {
		log.Fatal("Failed to compute block root:", err)
	}
	fmt.Printf("   - Root: %x\n", root)
	fmt.Printf("   - Active field indices: 0, 1, 3, 4, 5 (5 fields total)\n")
	fmt.Printf("   - Forward-compatible: new fields can be added with higher ssz-index\n")

	// Example 3: M4 - Compatible Union embedded in container
	fmt.Println("\n3. M4: Compatible Union (embedded in container):")

	// Define execution payload variants for unions
	type ExecutionPayload struct {
		ParentHash   [32]byte
		FeeRecipient [20]byte
		GasLimit     uint64
	}

	type ExecutionPayloadWithBlobs struct {
		ParentHash         [32]byte
		FeeRecipient       [20]byte
		GasLimit           uint64
		BlobKzgCommitments [][]byte `ssz-max:"4096" ssz-size:"?,48"`
	}

	// M4: Compatible Union - union of execution payload variants
	type PayloadUnion = dynssz.CompatibleUnion[struct {
		ExecutionPayload
		ExecutionPayloadWithBlobs
	}]

	// Container that embeds the union via the generic type
	type BlockWithPayload struct {
		Slot          uint64       `ssz-index:"0"`
		ProposerIndex uint64       `ssz-index:"1"`
		ParentRoot    [32]byte     `ssz-index:"2"`
		StateRoot     [32]byte     `ssz-index:"20"`
		ExecutionData PayloadUnion `ssz-index:"28"` // Union embedded here
		Timestamp     uint64       `ssz-index:"55"`
	}

	// Create execution payload without blobs (variant 0)
	basicPayload := ExecutionPayload{
		ParentHash:   [32]byte{11, 12, 13, 14, 15},
		FeeRecipient: [20]byte{21, 22, 23, 24, 25},
		GasLimit:     15000000,
	}

	// Create execution payload with blobs (variant 1)
	blobPayload := ExecutionPayloadWithBlobs{
		ParentHash:   [32]byte{31, 32, 33, 34, 35},
		FeeRecipient: [20]byte{41, 42, 43, 44, 45},
		GasLimit:     20000000,
		BlobKzgCommitments: [][]byte{
			make([]byte, 48), // KZG commitment 1
			make([]byte, 48), // KZG commitment 2
		},
	}

	// Create blocks with different union variants
	blockWithBasic := BlockWithPayload{
		Slot:          54321,
		ProposerIndex: 11111,
		ParentRoot:    [32]byte{51, 52, 53, 54, 55},
		StateRoot:     [32]byte{61, 62, 63, 64, 65},
		ExecutionData: PayloadUnion{Variant: 0, Data: basicPayload},
		Timestamp:     1234567890,
	}

	blockWithBlobs := BlockWithPayload{
		Slot:          54322,
		ProposerIndex: 22222,
		ParentRoot:    [32]byte{71, 72, 73, 74, 75},
		StateRoot:     [32]byte{81, 82, 83, 84, 85},
		ExecutionData: PayloadUnion{Variant: 1, Data: blobPayload},
		Timestamp:     1234567891,
	}

	// Serialize both blocks
	basicBlockEncoded, err := ds.MarshalSSZ(&blockWithBasic)
	if err != nil {
		log.Fatal("Failed to marshal block with basic payload:", err)
	}

	blobBlockEncoded, err := ds.MarshalSSZ(&blockWithBlobs)
	if err != nil {
		log.Fatal("Failed to marshal block with blob payload:", err)
	}

	// Compute hash tree roots
	basicBlockRoot, err := ds.HashTreeRoot(&blockWithBasic)
	if err != nil {
		log.Fatal("Failed to compute basic block root:", err)
	}

	blobBlockRoot, err := ds.HashTreeRoot(&blockWithBlobs)
	if err != nil {
		log.Fatal("Failed to compute blob block root:", err)
	}

	fmt.Printf("   - Block with Basic Payload (variant 0): %d bytes, root: %x\n", len(basicBlockEncoded), basicBlockRoot)
	fmt.Printf("   - Block with Blob Payload (variant 1): %d bytes, root: %x\n", len(blobBlockEncoded), blobBlockRoot)
	fmt.Printf("   - Union embedded in progressive container with ssz-index:\"28\"\n")
	fmt.Printf("   - Type-safe variants with automatic selector assignment\n")

	// Demonstrate union serialization details
	union1 := PayloadUnion{Variant: 0, Data: basicPayload}
	union2 := PayloadUnion{Variant: 1, Data: blobPayload}

	unionData1, _ := ds.MarshalSSZ(&union1)
	unionData2, _ := ds.MarshalSSZ(&union2)

	fmt.Printf("   - Standalone union sizes: %d bytes (basic), %d bytes (blob)\n", len(unionData1), len(unionData2))
	fmt.Printf("   - Union format: 1-byte selector + serialized data\n")

	fmt.Println("\n4. Summary of Features:")
	fmt.Println("   - M1 (Progressive Lists): Efficient merkleization for growing lists")
	fmt.Println("   - M2 (Progressive Bitlists): Optimized for participation tracking")
	fmt.Println("   - M3 (Progressive Containers): ssz-index tags for forward compatibility")
	fmt.Println("   - M4 (Compatible Unions): Type-safe variants embedded in containers")

	fmt.Println("\nExample completed successfully! All 4 progressive merkleization features demonstrated.")
}
