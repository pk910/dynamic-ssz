// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/sha3"
)

func refKeccak256(a, b [32]byte) (out [32]byte) {
	h := sha3.NewLegacyKeccak256()
	h.Write(a[:])
	h.Write(b[:])
	h.Sum(out[:0])
	return out
}

// keccak256Fn is a top-level Keccak-256 function, with code of its own.
func keccak256Fn(dst, input []byte) error {
	for i := 0; i+64 <= len(input); i += 64 {
		sum := refKeccak256([32]byte(input[i:i+32]), [32]byte(input[i+32:i+64]))
		copy(dst[i/2:], sum[:])
	}
	return nil
}

func TestKeccak256ZeroHashKnownAnswers(t *testing.T) {
	for _, tt := range []struct {
		name  string
		alg   *Algorithm
		depth int
		want  string
	}{
		{"keccak256", Keccak256, 1, "ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"},
		{"keccak256", Keccak256, 2, "b4c11951957c6f8f642c4af61cd6b24640fec6dc7fc607ee8206a99e92410d30"},
		{"keccak256 native", Keccak256Native, 1, "ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5"},
	} {
		if got := hex.EncodeToString(tt.alg.ZeroHash(tt.depth)); got != tt.want {
			t.Errorf("%s zero hash at depth %d = %s, want %s", tt.name, tt.depth, got, tt.want)
		}
	}
}

// Both Keccak-256 algorithms give the roots of an independent merkleization
// with Keccak-256, for padded, full and progressive trees.
func TestKeccak256Roots(t *testing.T) {
	for name, alg := range map[string]*Algorithm{"keccak256": Keccak256, "keccak256 native": Keccak256Native} {
		for _, n := range []int{0, 1, 2, 3, 5, 64, 65, 300} {
			chunks := makeChunks(n)
			limit := uint64(1024)

			hh := alg.Pool().Get()
			indx := hh.Index()
			for i := range chunks {
				hh.Append(chunks[i][:])
			}
			hh.MerkleizeWithMixin(indx, uint64(n), limit)
			got, err := hh.HashRoot()
			alg.Pool().Put(hh)

			want := refMixin(refKeccak256, refMerkleize(refKeccak256, chunks, limit), uint64(n))
			if err != nil || got != want {
				t.Errorf("%s, %d chunks: root %x, %v; want %x", name, n, got, err, want)
			}
		}

		var pair [64]byte
		pair[0], pair[32] = 1, 2
		if got, err := alg.HashPair(&pair); err != nil || got != refKeccak256([32]byte{1}, [32]byte{2}) {
			t.Errorf("%s: HashPair = %x, %v", name, got, err)
		}
	}
}

// Keccak256 is known by its name, by its function and by its hash.
func TestKeccak256Identity(t *testing.T) {
	if alg, ok := AlgorithmByName("keccak256"); !ok || alg != Keccak256 {
		t.Error("keccak256 does not name Keccak256")
	}
	if Keccak256.Native() != Keccak256Native || Keccak256Native.Native() != Keccak256Native {
		t.Error("Keccak256 does not name its native counterpart")
	}
	if !Keccak256.Compatible(Keccak256Native) || Keccak256.Compatible(SHA256) {
		t.Error("Keccak256 is compatible with its native counterpart only")
	}
	for name, alg := range map[string]*Algorithm{"Keccak256": Keccak256, "Keccak256Native": Keccak256Native} {
		if got, err := AlgorithmFor(alg.HashFn()); err != nil || got != alg {
			t.Errorf("%s: AlgorithmFor gives another Algorithm for its function: %v", name, err)
		}
	}

	// Another Keccak-256 function is an Algorithm of its own that shares the
	// zero subtree roots.
	own, err := AlgorithmFor(keccak256Fn)
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	if own == Keccak256 || !own.Compatible(Keccak256) || own.ZeroHashTable() != Keccak256.ZeroHashTable() {
		t.Error("a Keccak-256 function is not a separate Algorithm of the Keccak-256 family")
	}
	if hh := NewHasherWithHashFn(keccak256Fn); !hh.Algorithm().Compatible(Keccak256) {
		t.Error("the hasher of a Keccak-256 function is not compatible with Keccak256")
	}
}
