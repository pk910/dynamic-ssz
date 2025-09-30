// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

// Package main demonstrates basic usage of the dynamic-ssz library.
// This example shows how to encode, decode, and compute hash tree roots
// for Ethereum data structures using dynamic specifications.
package main

import (
	"fmt"
	"log"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	dynssz "github.com/pk910/dynamic-ssz"
)

func main() {
	fmt.Println("Dynamic SSZ Basic Example")
	fmt.Println("========================")

	// Create DynSsz instance with mainnet specifications
	specs := map[string]any{
		"SLOTS_PER_HISTORICAL_ROOT":    uint64(8192),
		"SYNC_COMMITTEE_SIZE":          uint64(512),
		"MAX_VALIDATORS_PER_COMMITTEE": uint64(2048),
	}
	ds := dynssz.NewDynSsz(specs)

	// Example 1: Encode a BeaconBlockHeader
	fmt.Println("\n1. BeaconBlockHeader Example:")
	header := &phase0.BeaconBlockHeader{
		Slot:          12345,
		ProposerIndex: 42,
		ParentRoot:    [32]byte{1, 2, 3, 4, 5},
		StateRoot:     [32]byte{6, 7, 8, 9, 10},
		BodyRoot:      [32]byte{11, 12, 13, 14, 15},
	}

	// Calculate size
	size, err := ds.SizeSSZ(header)
	if err != nil {
		log.Fatal("Failed to calculate size:", err)
	}
	fmt.Printf("Expected SSZ size: %d bytes\n", size)

	// Marshal to SSZ
	data, err := ds.MarshalSSZ(header)
	if err != nil {
		log.Fatal("Failed to marshal:", err)
	}
	fmt.Printf("Encoded %d bytes\n", len(data))

	// Unmarshal from SSZ
	var decoded phase0.BeaconBlockHeader
	err = ds.UnmarshalSSZ(&decoded, data)
	if err != nil {
		log.Fatal("Failed to unmarshal:", err)
	}
	fmt.Printf("Decoded slot: %d, proposer: %d\n", decoded.Slot, decoded.ProposerIndex)

	// Calculate hash tree root
	root, err := ds.HashTreeRoot(header)
	if err != nil {
		log.Fatal("Failed to calculate hash tree root:", err)
	}
	fmt.Printf("Hash tree root: %x\n", root)

	// Example 2: Working with attestations
	fmt.Println("\n2. Attestation Example:")
	attestation := &phase0.Attestation{
		AggregationBits: []byte{0xff, 0x01, 0x02, 0x03},
		Data: &phase0.AttestationData{
			Slot:            100,
			Index:           1,
			BeaconBlockRoot: [32]byte{1, 2, 3},
			Source: &phase0.Checkpoint{
				Epoch: 10,
				Root:  [32]byte{4, 5, 6},
			},
			Target: &phase0.Checkpoint{
				Epoch: 11,
				Root:  [32]byte{7, 8, 9},
			},
		},
		Signature: [96]byte{0x01, 0x02, 0x03},
	}

	attestationData, err := ds.MarshalSSZ(attestation)
	if err != nil {
		log.Fatal("Failed to marshal attestation:", err)
	}
	fmt.Printf("Encoded attestation: %d bytes\n", len(attestationData))

	var decodedAttestation phase0.Attestation
	err = ds.UnmarshalSSZ(&decodedAttestation, attestationData)
	if err != nil {
		log.Fatal("Failed to unmarshal attestation:", err)
	}
	fmt.Printf("Decoded attestation slot: %d, index: %d\n",
		decodedAttestation.Data.Slot, decodedAttestation.Data.Index)

	// Example 3: Using MarshalSSZTo with buffer reuse
	fmt.Println("\n3. Buffer Reuse Example:")
	buf := make([]byte, 0, 1024) // Pre-allocate buffer

	// Reuse buffer for multiple operations
	for i := 0; i < 3; i++ {
		header.Slot = phase0.Slot(12345 + i)
		buf, err = ds.MarshalSSZTo(header, buf[:0]) // Reset buffer length
		if err != nil {
			log.Fatal("Failed to marshal with buffer:", err)
		}
		fmt.Printf("Iteration %d: encoded %d bytes to buffer\n", i+1, len(buf))
	}

	fmt.Println("\nBasic example completed successfully!")
}
