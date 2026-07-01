// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// TestWrapperInterleavedAppendPut is a regression test for
// https://github.com/pk910/dynamic-ssz/issues/191: the Wrapper must produce
// the same root as hasher.Hasher when a container interleaves Append*-buffered
// fields with Put* fields or child scopes. The Wrapper used to flush buffered
// bytes only at Merkleize, reordering leaves relative to directly-added nodes.
func TestWrapperInterleavedAppendPut(t *testing.T) {
	fieldA := [32]byte{0x11}
	fieldB := [32]byte{0x22}

	tests := []struct {
		name string
		walk func(hh sszutils.HashWalker)
	}{
		{
			// e.g. a uint256 root emitted via AppendBytes32 followed by a
			// bytes32 field emitted via PutBytes (deneb.ExecutionPayloadHeader
			// BaseFeePerGas -> BlockHash layout)
			name: "append then put",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				hh.PutBytes(fieldB[:])
				hh.Merkleize(idx)
			},
		},
		{
			name: "append then put uint64",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				hh.PutUint64(42)
				hh.Merkleize(idx)
			},
		},
		{
			// an Append*-buffered field followed by a child scope: the buffered
			// bytes must become leaves of the parent, not of the child list
			name: "append then child scope",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				cidx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendUint64(42)
				hh.AppendUint64(43)
				hh.FillUpTo32()
				hh.MerkleizeWithMixin(cidx, 2, 4)
				hh.Merkleize(idx)
			},
		},
		{
			// deneb.ExecutionPayloadHeader-like tail: dynamic list scope,
			// buffered uint256 root, then direct bytes32/uint64 fields
			name: "payload header field tail",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.PutBytes(fieldA[:])                      // PrevRandao
				hh.PutUint64(1)                             // BlockNumber
				hh.PutUint64(2)                             // GasLimit
				cidx := hh.StartTree(sszutils.TreeTypeNone) // ExtraData
				hh.Append([]byte{0xff, 0xee})
				hh.FillUpTo32()
				hh.MerkleizeWithMixin(cidx, 2, 1)
				hh.AppendBytes32(fieldB[:]) // BaseFeePerGas root
				hh.PutBytes(fieldA[:])      // BlockHash
				hh.PutUint64(3)             // BlobGasUsed
				hh.Merkleize(idx)
			},
		},
		{
			name: "append then bitlist",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				hh.PutBitlist([]byte{0xff, 0x01}, 64)
				hh.Merkleize(idx)
			},
		},
		{
			name: "append then large put bytes",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				large := make([]byte, 96)
				large[0] = 0x33
				hh.PutBytes(large)
				hh.Merkleize(idx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hasher.FastHasherPool.Get()
			defer hasher.FastHasherPool.Put(h)

			tt.walk(h)
			expected, err := h.HashRoot()
			if err != nil {
				t.Fatalf("hasher HashRoot failed: %v", err)
			}

			w := NewWrapper()
			tt.walk(w)
			actual, err := w.HashRoot()
			if err != nil {
				t.Fatalf("wrapper HashRoot failed: %v", err)
			}

			if expected != actual {
				t.Errorf("wrapper root %x does not match hasher root %x", actual, expected)
			}
		})
	}
}
