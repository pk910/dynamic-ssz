// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"github.com/pk910/keccaktree"
	"golang.org/x/crypto/sha3"
)

// keccak256Family is Ethereum Keccak-256. Its zero subtree roots are computed
// in pure Go, so building them does not touch the accelerated backend.
var keccak256Family = &hashFamily{fingerprint: keccak256Fingerprint, build: keccak256Native}

// Keccak256 merkleizes with Ethereum Keccak-256 (padding 0x01, not NIST
// SHA3-256), computed by the accelerated keccaktree backend. An ssz-hash
// annotation names it "keccak256".
var Keccak256 = &Algorithm{
	fn:     keccaktree.HashByteSlice,
	pool:   &HasherPool{HashFn: keccaktree.HashByteSlice},
	family: keccak256Family,
}

// Keccak256Native is Keccak256 computed in pure Go.
var Keccak256Native = &Algorithm{
	fn:     keccak256Native,
	pool:   &HasherPool{HashFn: keccak256Native},
	family: keccak256Family,
}

// keccak256Native is the Keccak-256 compression in pure Go.
var keccak256Native = NativeHashWrapperFactory(sha3.NewLegacyKeccak256)

// keccak256Fingerprint is the Keccak-256 hash of a pair of zero chunks.
var keccak256Fingerprint = [32]byte{
	0xad, 0x32, 0x28, 0xb6, 0x76, 0xf7, 0xd3, 0xcd, 0x42, 0x84, 0xa5, 0x44, 0x3f, 0x17, 0xf1, 0x96,
	0x2b, 0x36, 0xe4, 0x91, 0xb3, 0x0a, 0x40, 0xb2, 0x40, 0x58, 0x49, 0xe5, 0x97, 0xba, 0x5f, 0xb5,
}

func init() {
	registerBuiltIn("keccak256", Keccak256, Keccak256Native)
}
