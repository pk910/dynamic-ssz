package tests

import (
	"encoding/hex"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

// TestCodegenDynSizeVectorNoStatic is a regression test for a codegen parser
// bug: a slice sized purely by a dynssz-size expression (no static ssz-size
// fallback) was classified as a variable List instead of a Vector. The
// reflection typecache resolves the expression to a concrete size at
// descriptor-build time and correctly treats such a slice as Vector[T, N]; the
// codegen parser, which has no spec values at generation time, keyed the
// vector/list decision on the static size alone (0 here) and emitted a
// variable-list encoding — a 4-byte offset header plus contents, ignoring the
// spec entirely (MarshalSSZDyn dropped its DynamicSpecs argument). That diverged
// from reflection and the SSZ spec on size, serialization and hash tree root.
//
// The differential leg (reflection vs codegen) catches the divergence; the
// golden serializations and roots — cross-checked against ethereum/remerkleable
// (Container{V: Vector[uint16,N], AV: Vector[Vector[uint16,N],2]}) — guard
// against a future regression that made both engines agree on the wrong
// (variable-list) encoding.
func TestCodegenDynSizeVectorNoStatic(t *testing.T) {
	cases := []struct {
		name string
		n    int
		len  uint64
		ser  string
		root string
	}{
		{
			name: "mainnet",
			n:    3,
			len:  3,
			ser:  "010002000300010002000300650066006700",
			root: "aa5624c38c6705e5c2925da004ebd588149376b911a1e00d98841566a996ea50",
		},
		{
			name: "minimal",
			n:    5,
			len:  5,
			ser:  "010002000300040005000100020003000400050065006600670068006900",
			root: "c642c4f85bb485ac64d226a098b9f9bb2df187664a746f12d0042cb8f265d91e",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			specs := map[string]any{"DSV_LEN": tc.len}
			payload := DynSizeVectorNoStaticPayload(tc.n)

			// Reflection-vs-codegen agreement across marshal, size, HTR,
			// buffer/stream round-trips.
			testCodegenPayloadByReflection(t, payload, specs)

			// Absolute golden values cross-checked with remerkleable.
			ds := dynssz.NewDynSsz(specs)
			ser, err := ds.MarshalSSZ(payload)
			if err != nil {
				t.Fatalf("MarshalSSZ: %v", err)
			}
			if got := hex.EncodeToString(ser); got != tc.ser {
				t.Fatalf("serialization changed: got %s want %s", got, tc.ser)
			}
			root, err := ds.HashTreeRoot(payload)
			if err != nil {
				t.Fatalf("HashTreeRoot: %v", err)
			}
			if got := hex.EncodeToString(root[:]); got != tc.root {
				t.Fatalf("root changed: got %s want %s", got, tc.root)
			}
		})
	}
}
