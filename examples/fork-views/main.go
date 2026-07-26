// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

// Package main demonstrates SSZ views: one runtime type serialized with
// different per-fork schemas.
//
// Ethereum containers evolve across hard forks — fields get added, layouts
// change — but handler code doesn't want a per-fork type explosion. With
// views, a single "data type" holds the union of all fields, and lightweight
// per-fork "view types" define the SSZ schema (field set, order and tags).
// A Version field on the data type selects the view at runtime.
package main

import (
	"fmt"
	"log"
	"reflect"

	"github.com/holiman/uint256"
	dynssz "github.com/pk910/dynamic-ssz"
)

// DataVersion identifies the fork a bid belongs to.
type DataVersion uint8

const (
	DataVersionBellatrix DataVersion = iota
	DataVersionCapella
	DataVersionDeneb
)

func (v DataVersion) String() string {
	switch v {
	case DataVersionBellatrix:
		return "bellatrix"
	case DataVersionCapella:
		return "capella"
	case DataVersionDeneb:
		return "deneb"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(v))
	}
}

// ExecutionHeader is the data type for a simplified execution payload
// header: it carries the union of all fields across forks. Which fields end
// up on the wire is decided by the view, not by this struct.
type ExecutionHeader struct {
	ParentHash   [32]byte
	FeeRecipient [20]byte
	StateRoot    [32]byte
	BlockNumber  uint64
	GasLimit     uint64
	GasUsed      uint64
	Timestamp    uint64
	BaseFee      *uint256.Int
	// Capella+
	WithdrawalsRoot [32]byte
	// Deneb+
	BlobGasUsed   uint64
	ExcessBlobGas uint64
}

// BuilderBid is the fork-agnostic data type, modeled on the builder-API bid
// containers. Version selects the wire schema; it appears in no view and is
// therefore never serialized.
type BuilderBid struct {
	Version DataVersion
	Header  *ExecutionHeader
	Value   *uint256.Int
	Pubkey  [48]byte
	// Deneb+
	BlobCommitments [][48]byte
}

// The view types below define one SSZ schema each. Only their type
// information is used — they are never instantiated. Fields are matched to
// the data type by name; nested structs get their own view types.

// ExecutionHeaderBellatrixView is the pre-withdrawals header schema.
type ExecutionHeaderBellatrixView struct {
	ParentHash   [32]byte
	FeeRecipient [20]byte
	StateRoot    [32]byte
	BlockNumber  uint64
	GasLimit     uint64
	GasUsed      uint64
	Timestamp    uint64
	BaseFee      *uint256.Int `ssz-type:"uint256"`
}

// ExecutionHeaderCapellaView adds the withdrawals root.
type ExecutionHeaderCapellaView struct {
	ParentHash      [32]byte
	FeeRecipient    [20]byte
	StateRoot       [32]byte
	BlockNumber     uint64
	GasLimit        uint64
	GasUsed         uint64
	Timestamp       uint64
	BaseFee         *uint256.Int `ssz-type:"uint256"`
	WithdrawalsRoot [32]byte
}

// ExecutionHeaderDenebView adds the blob gas fields.
type ExecutionHeaderDenebView struct {
	ParentHash      [32]byte
	FeeRecipient    [20]byte
	StateRoot       [32]byte
	BlockNumber     uint64
	GasLimit        uint64
	GasUsed         uint64
	Timestamp       uint64
	BaseFee         *uint256.Int `ssz-type:"uint256"`
	WithdrawalsRoot [32]byte
	BlobGasUsed     uint64
	ExcessBlobGas   uint64
}

// BuilderBidBellatrixView is the bellatrix bid schema.
type BuilderBidBellatrixView struct {
	Header *ExecutionHeaderBellatrixView
	Value  *uint256.Int `ssz-type:"uint256"`
	Pubkey [48]byte
}

// BuilderBidCapellaView is the capella bid schema.
type BuilderBidCapellaView struct {
	Header *ExecutionHeaderCapellaView
	Value  *uint256.Int `ssz-type:"uint256"`
	Pubkey [48]byte
}

// BuilderBidDenebView adds blob commitments, with a preset-dependent limit.
type BuilderBidDenebView struct {
	Header          *ExecutionHeaderDenebView
	BlobCommitments [][48]byte   `ssz-max:"4096" dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK"`
	Value           *uint256.Int `ssz-type:"uint256"`
	Pubkey          [48]byte
}

// viewFor returns the view descriptor for a fork version — the dispatch
// pattern used with fork-agnostic types, keyed off the data type's Version
// field.
func viewFor(version DataVersion) (any, error) {
	switch version {
	case DataVersionBellatrix:
		return (*BuilderBidBellatrixView)(nil), nil
	case DataVersionCapella:
		return (*BuilderBidCapellaView)(nil), nil
	case DataVersionDeneb:
		return (*BuilderBidDenebView)(nil), nil
	default:
		return nil, fmt.Errorf("no view for version %s", version)
	}
}

func main() {
	fmt.Println("Fork Views Example — one type, one schema per fork")
	fmt.Println("===================================================")

	ds := dynssz.NewDynSsz(nil)

	// Validate every view against the data type once at startup — a cheap
	// check that catches schema drift before it corrupts wire data.
	for _, version := range []DataVersion{DataVersionBellatrix, DataVersionCapella, DataVersionDeneb} {
		view, err := viewFor(version)
		if err != nil {
			log.Fatal(err)
		}

		if err := ds.ValidateType(reflect.TypeOf((*BuilderBid)(nil)), dynssz.WithViewDescriptor(view)); err != nil {
			log.Fatalf("view for %s does not match BuilderBid: %v", version, err)
		}
	}

	fmt.Println("\nAll views validated against the data type.")

	// One bid, fully populated. The same object serializes under all three
	// fork schemas.
	bid := &BuilderBid{
		Header: &ExecutionHeader{
			BlockNumber:   19_000_000,
			GasLimit:      30_000_000,
			GasUsed:       12_345_678,
			Timestamp:     1700000000,
			BaseFee:       uint256.NewInt(7_500_000_000),
			BlobGasUsed:   131072,
			ExcessBlobGas: 393216,
		},
		Value:           uint256.NewInt(123_456_789_000_000_000),
		BlobCommitments: make([][48]byte, 3),
	}
	bid.Header.ParentHash[0] = 0xaa
	bid.Header.WithdrawalsRoot[0] = 0xbb
	bid.Pubkey[0] = 0xcc

	fmt.Println("\n1. Marshal the same object under each fork schema:")

	encoded := make(map[DataVersion][]byte, 3)

	for _, version := range []DataVersion{DataVersionBellatrix, DataVersionCapella, DataVersionDeneb} {
		view, err := viewFor(version)
		if err != nil {
			log.Fatal(err)
		}

		data, err := ds.MarshalSSZ(bid, dynssz.WithViewDescriptor(view))
		if err != nil {
			log.Fatalf("failed to marshal %s bid: %v", version, err)
		}

		root, err := ds.HashTreeRoot(bid, dynssz.WithViewDescriptor(view))
		if err != nil {
			log.Fatalf("failed to hash %s bid: %v", version, err)
		}

		encoded[version] = data

		fmt.Printf("  %-9s: %4d bytes, root 0x%x\n", version, len(data), root[:8])
	}

	fmt.Println("  Fields absent from a view (WithdrawalsRoot on bellatrix, blob data")
	fmt.Println("  before deneb) simply don't reach the wire — no per-fork structs needed.")

	// 2. Version-dispatched decoding: pick the view from the version that
	// arrived out of band (API header, stored version column, fork schedule).
	fmt.Println("\n2. Unmarshal with the version-selected view:")

	for _, version := range []DataVersion{DataVersionBellatrix, DataVersionDeneb} {
		view, err := viewFor(version)
		if err != nil {
			log.Fatal(err)
		}

		decoded := &BuilderBid{Version: version}
		if err := ds.UnmarshalSSZ(decoded, encoded[version], dynssz.WithViewDescriptor(view)); err != nil {
			log.Fatalf("failed to unmarshal %s bid: %v", version, err)
		}

		fmt.Printf("  %-9s: block %d, value %s wei, %d blob commitments, withdrawals root 0x%x...\n",
			version, decoded.Header.BlockNumber, decoded.Value, len(decoded.BlobCommitments),
			decoded.Header.WithdrawalsRoot[:2])
	}

	fmt.Println("  The bellatrix decode leaves WithdrawalsRoot and BlobCommitments at their")
	fmt.Println("  zero values — those fields are not part of its schema.")

	fmt.Println("\n3. Views compose with dynamic specs:")

	// The deneb view carries dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK", so
	// the same view resolves different limits per network.
	devnetSsz := dynssz.NewDynSsz(map[string]any{
		"MAX_BLOB_COMMITMENTS_PER_BLOCK": uint64(2),
	})

	_, err := devnetSsz.MarshalSSZ(bid, dynssz.WithViewDescriptor((*BuilderBidDenebView)(nil)))
	if err != nil {
		fmt.Printf("  3 commitments rejected on a devnet with MAX_BLOB_COMMITMENTS_PER_BLOCK=2:\n    %v\n", err)
	} else {
		log.Fatal("expected the devnet limit to reject 3 blob commitments")
	}
}
