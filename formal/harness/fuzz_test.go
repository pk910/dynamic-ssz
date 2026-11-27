package harness

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

// FuzzDecodeCanonicity feeds arbitrary bytes to every mirror type's decoder on
// both engines and enforces:
//   - no panic / hang / unbounded allocation (P5 totality, via the fuzz runtime)
//   - accepted bytes re-encode identically: S(D(b)) == b (P2 canonicity)
//   - HashTreeRoot of any accepted value succeeds
//   - both engines agree on accept/reject and on the re-encoding (P10)
func FuzzDecodeCanonicity(f *testing.F) {
	entries := allEntries()
	refl := dynssz.NewDynSsz(nil, dynssz.WithNoDelegation())
	gen := dynssz.NewDynSsz(nil)

	// Seed with the ssz-specs fixture encodings.
	if vectors, err := LoadFixtures(FixturesRoot()); err == nil && FixturesRoot() != "" {
		for i, vec := range vectors {
			if vec.Serialized != "" {
				b, err := hexDecode(vec.Serialized)
				if err == nil {
					f.Add(uint16(i%len(entries)), b)
				}
			}
		}
	}

	f.Fuzz(func(t *testing.T, typeIdx uint16, data []byte) {
		entry := entries[int(typeIdx)%len(entries)].Entry

		v1 := entry.New()
		err1 := refl.UnmarshalSSZ(v1, data)

		v2 := entry.New()
		err2 := gen.UnmarshalSSZ(v2, data)

		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("P10 VIOLATION: engines disagree on accept/reject: reflection=%v codegen=%v (bytes 0x%x)", err1, err2, data)
		}
		if err1 != nil {
			return
		}

		b1, err := refl.MarshalSSZ(v1)
		if err != nil {
			t.Fatalf("accepted 0x%x but reflection re-marshal failed: %v", data, err)
		}
		if !bytes.Equal(b1, data) {
			t.Fatalf("P2 VIOLATION (reflection): accepted 0x%x, re-encodes to 0x%x", data, b1)
		}
		b2, err := gen.MarshalSSZ(v2)
		if err != nil {
			t.Fatalf("accepted 0x%x but codegen re-marshal failed: %v", data, err)
		}
		if !bytes.Equal(b2, data) {
			t.Fatalf("P2 VIOLATION (codegen): accepted 0x%x, re-encodes to 0x%x", data, b2)
		}

		r1, err := refl.HashTreeRoot(v1)
		if err != nil {
			t.Fatalf("htr(reflection) failed on accepted value: %v", err)
		}
		r2, err := gen.HashTreeRoot(v2)
		if err != nil {
			t.Fatalf("htr(codegen) failed on accepted value: %v", err)
		}
		if r1 != r2 {
			t.Fatalf("P10 VIOLATION: roots differ: 0x%x vs 0x%x", r1, r2)
		}
	})
}

func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(s, "0x"))
}
