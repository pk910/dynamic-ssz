package harness

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"pgregory.net/rapid"
)

// allEntries returns the union of all registered mirror types, deduplicated by
// the concrete Go type, with stable ordering.
func allEntries() []struct {
	Name  string
	Entry Entry
} {
	seen := map[string]bool{}
	var out []struct {
		Name  string
		Entry Entry
	}
	var dirs []string
	for dir := range Registries {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		reg := Registries[dir]
		var names []string
		for n := range reg {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			e := reg[n]
			goType := fmt.Sprintf("%T", e.New())
			if seen[goType] {
				continue
			}
			seen[goType] = true
			out = append(out, struct {
				Name  string
				Entry Entry
			}{Name: goType[1:], Entry: e}) // strip leading '*'
		}
	}
	return out
}

var enginesCache map[string]*dynssz.DynSsz

func engines() map[string]*dynssz.DynSsz {
	if enginesCache != nil {
		return enginesCache
	}
	m := map[string]*dynssz.DynSsz{
		"reflection": dynssz.NewDynSsz(nil, dynssz.WithNoDelegation(), dynssz.WithExtendedTypes()),
	}
	if generatedCodeAvailable {
		m["codegen"] = dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	}
	enginesCache = m
	return m
}

// TestPropRoundtrip checks P1 (roundtrip) and P2 on valid encodings
// (S(D(S(v))) == S(v)), plus P4 (size agreement), for random values of every
// mirror type, on both engines.
func TestPropRoundtrip(t *testing.T) {
	for engName, ds := range engines() {
		for _, te := range allEntries() {
			te := te
			ds := ds
			t.Run(engName+"/"+te.Name, func(t *testing.T) {
				rapid.Check(t, func(rt *rapid.T) {
					v1, err := GenValue(rt, engines()["reflection"], te.Entry.New, DefaultGen)
					if err != nil {
						rt.Fatalf("gen: %v", err)
					}
					b, err := ds.MarshalSSZ(v1)
					if err != nil {
						rt.Fatalf("marshal: %v", err)
					}
					size, err := ds.SizeSSZ(v1)
					if err != nil {
						rt.Fatalf("size: %v", err)
					}
					if size != len(b) {
						rt.Fatalf("P4 size mismatch: SizeSSZ=%d len=%d", size, len(b))
					}
					v2 := te.Entry.New()
					if err := ds.UnmarshalSSZ(v2, b); err != nil {
						rt.Fatalf("P1 roundtrip: decode of own encoding failed: %v (bytes 0x%x, value %#v)", err, b, v1)
					}
					if diff := EqualNormalized(v1, v2); diff != "" {
						rt.Fatalf("P1 roundtrip: value mismatch: %s", diff)
					}
					b2, err := ds.MarshalSSZ(v2)
					if err != nil {
						rt.Fatalf("re-marshal: %v", err)
					}
					if !bytes.Equal(b, b2) {
						rt.Fatalf("P2 canonicity on valid encoding: S(D(b))=0x%x != b=0x%x", b2, b)
					}
				})
			})
		}
	}
}

// TestPropMutationCanonicity is the Ghost-in-the-Block property (P2/P5):
// take a valid encoding, mutate it arbitrarily, and require that the decoder
// either rejects the mutated bytes or decodes them to a value that re-encodes
// to exactly the same bytes. Any accept-but-recode-differently outcome means
// two representations decode to the same value — non-canonical acceptance.
// Decoder totality (no panic) is implicitly checked throughout.
func TestPropMutationCanonicity(t *testing.T) {
	for engName, ds := range engines() {
		for _, te := range allEntries() {
			te := te
			ds := ds
			t.Run(engName+"/"+te.Name, func(t *testing.T) {
				rapid.Check(t, func(rt *rapid.T) {
					v1, err := GenValue(rt, engines()["reflection"], te.Entry.New, DefaultGen)
					if err != nil {
						rt.Fatalf("gen: %v", err)
					}
					b, err := ds.MarshalSSZ(v1)
					if err != nil {
						rt.Fatalf("marshal: %v", err)
					}
					mutated := mutate(rt, b)
					v2 := te.Entry.New()
					if err := ds.UnmarshalSSZ(v2, mutated); err != nil {
						return // rejected: fine
					}
					b2, err := ds.MarshalSSZ(v2)
					if err != nil {
						rt.Fatalf("accepted 0x%x but re-marshal failed: %v", mutated, err)
					}
					if !bytes.Equal(b2, mutated) {
						rt.Fatalf("P2 VIOLATION: accepted non-canonical bytes 0x%x, re-encodes to 0x%x (value %#v)", mutated, b2, v2)
					}
				})
			})
		}
	}
}

// mutate applies a random byte-level mutation: flip bytes, truncate, extend,
// or splice. Empty inputs are extended only.
func mutate(rt *rapid.T, b []byte) []byte {
	out := append([]byte(nil), b...)
	kind := rapid.IntRange(0, 3).Draw(rt, "mutkind")
	switch {
	case kind == 0 && len(out) > 0: // flip random bytes
		n := rapid.IntRange(1, 4).Draw(rt, "flips")
		for i := 0; i < n; i++ {
			pos := rapid.IntRange(0, len(out)-1).Draw(rt, "pos")
			out[pos] ^= byte(rapid.IntRange(1, 255).Draw(rt, "xor"))
		}
	case kind == 1 && len(out) > 0: // truncate
		out = out[:rapid.IntRange(0, len(out)-1).Draw(rt, "cut")]
	case kind == 2: // extend with random bytes
		n := rapid.IntRange(1, 8).Draw(rt, "ext")
		for i := 0; i < n; i++ {
			out = append(out, byte(rapid.IntRange(0, 255).Draw(rt, "extb")))
		}
	default: // splice: insert bytes at random position
		if len(out) == 0 {
			out = append(out, byte(rapid.IntRange(0, 255).Draw(rt, "sb")))
			break
		}
		pos := rapid.IntRange(0, len(out)).Draw(rt, "spos")
		ins := []byte{byte(rapid.IntRange(0, 255).Draw(rt, "sb"))}
		out = append(out[:pos], append(ins, out[pos:]...)...)
	}
	return out
}

// TestPropPathAgreement checks P6: hash tree roots and encodings agree across
// the buffer, writer/reader (streaming), and GetTree paths, and across engines.
func TestPropPathAgreement(t *testing.T) {
	engs := engines()
	refl := engs["reflection"]
	for _, te := range allEntries() {
		te := te
		t.Run(te.Name, func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				v, err := GenValue(rt, refl, te.Entry.New, DefaultGen)
				if err != nil {
					rt.Fatalf("gen: %v", err)
				}
				b, err := refl.MarshalSSZ(v)
				if err != nil {
					rt.Fatalf("marshal: %v", err)
				}
				root, err := refl.HashTreeRoot(v)
				if err != nil {
					rt.Fatalf("htr: %v", err)
				}

				// Streaming writer path.
				var buf bytes.Buffer
				if err := refl.MarshalSSZWriter(v, &buf); err != nil {
					rt.Fatalf("writer marshal: %v", err)
				}
				if !bytes.Equal(buf.Bytes(), b) {
					rt.Fatalf("P6: writer bytes differ: 0x%x vs 0x%x", buf.Bytes(), b)
				}

				// Streaming reader path.
				v3 := te.Entry.New()
				if err := refl.UnmarshalSSZReader(v3, bytes.NewReader(b), len(b)); err != nil {
					rt.Fatalf("reader unmarshal: %v", err)
				}
				if diff := EqualNormalized(v, v3); diff != "" {
					rt.Fatalf("P6: reader value differs: %s", diff)
				}

				// GetTree path.
				tree, err := refl.GetTree(v)
				if err != nil {
					rt.Fatalf("GetTree: %v", err)
				}
				if !bytes.Equal(tree.Hash(), root[:]) {
					rt.Fatalf("P6: GetTree root 0x%x != HashTreeRoot 0x%x", tree.Hash(), root)
				}

				// Cross-engine agreement.
				if cg, ok := engs["codegen"]; ok {
					b2, err := cg.MarshalSSZ(v)
					if err != nil {
						rt.Fatalf("codegen marshal: %v", err)
					}
					if !bytes.Equal(b2, b) {
						rt.Fatalf("P10: codegen bytes differ: 0x%x vs 0x%x", b2, b)
					}
					root2, err := cg.HashTreeRoot(v)
					if err != nil {
						rt.Fatalf("codegen htr: %v", err)
					}
					if root2 != root {
						rt.Fatalf("P10: codegen root differs: 0x%x vs 0x%x", root2, root)
					}
				}
			})
		})
	}
}
