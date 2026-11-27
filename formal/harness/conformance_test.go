package harness

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

type vectorResult struct {
	TestID   string `json:"testId"`
	TypeName string `json:"typeName"`
	Status   string `json:"status"` // pass | fail | skip
	Detail   string `json:"detail,omitempty"`
}

var results []vectorResult

func TestMain(m *testing.M) {
	code := m.Run()
	if out := os.Getenv("RESULTS_OUT"); out != "" {
		data, _ := json.MarshalIndent(results, "", "  ")
		_ = os.WriteFile(out, data, 0644)
	}
	os.Exit(code)
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// TestConformanceReflection replays every ssz-specs fixture vector against the
// dynamic-ssz reflection engine and checks:
//   - serialize(fill(value)) == serialized       (spec-equivalent encoding)
//   - SizeSSZ == len(serialized)
//   - hash_tree_root == root
//   - unmarshal(serialized) succeeds and equals the filled value
//   - serialize(unmarshal(serialized)) == serialized  (decode canonicity)
//   - negative vectors are rejected
func TestConformanceReflection(t *testing.T) {
	// WithNoDelegation forces the pure reflection engine even when generated
	// methods are present in this package.
	runConformance(t, dynssz.NewDynSsz(nil, dynssz.WithNoDelegation()))
}

// TestConformanceCodegen exercises the generated-code path: the DynSsz runtime
// delegates to the dynssz-gen generated methods (generated_ssz.go).
func TestConformanceCodegen(t *testing.T) {
	if !generatedCodeAvailable {
		t.Skip("generated_ssz.go not present")
	}
	runConformance(t, dynssz.NewDynSsz(nil))
}

func runConformance(t *testing.T, ds *dynssz.DynSsz) {
	vectors := loadFixturesOrNone(t)
	if len(vectors) == 0 {
		t.Skip("SSZ_FIXTURES not set")
	}

	for _, vec := range vectors {
		vec := vec
		name := vec.Dir + "/" + shortID(vec.TestID)
		t.Run(name, func(t *testing.T) {
			status, detail := runVector(t, ds, vec)
			results = append(results, vectorResult{TestID: vec.TestID, TypeName: vec.TypeName, Status: status, Detail: detail})
		})
	}
}

func runVector(t *testing.T, ds *dynssz.DynSsz, vec Vector) (string, string) {
	reg, ok := Registries[vec.Dir]
	if !ok {
		t.Skipf("no registry for fixture dir %s", vec.Dir)
	}
	entry, ok := reg[vec.TypeName]
	if !ok {
		t.Errorf("COVERAGE GAP: no Go mirror type for %s/%s", vec.Dir, vec.TypeName)
		return "skip", "no mirror type"
	}

	// Negative vector: decoding rawBytes must fail.
	if vec.RejectionReason != "" {
		raw := mustHex(t, vec.RawBytes)
		v := entry.New()
		if err := ds.UnmarshalSSZ(v, raw); err == nil {
			detail := fmt.Sprintf("DIVERGENCE: spec rejects (%s) but dynamic-ssz accepted bytes %s as %#v", vec.RejectionReason, vec.RawBytes, v)
			t.Error(detail)
			return "fail", detail
		}
		return "pass", ""
	}

	serialized := mustHex(t, vec.Serialized)
	wantRoot := mustHex(t, vec.Root)

	// 1. Build the value from the fixture JSON and check the encoding.
	v1 := entry.New()
	if err := Fill(v1, vec.Value); err != nil {
		detail := fmt.Sprintf("fill: %v", err)
		t.Errorf("%s", detail)
		return "fail", detail
	}
	enc, err := ds.MarshalSSZ(v1)
	if err != nil {
		detail := fmt.Sprintf("marshal: %v", err)
		t.Errorf("%s", detail)
		return "fail", detail
	}
	if !equalBytes(enc, serialized) {
		detail := fmt.Sprintf("DIVERGENCE encode: got 0x%x want %s (value %#v)", enc, vec.Serialized, v1)
		t.Errorf("%s", detail)
		return "fail", detail
	}

	// 2. Size agreement.
	size, err := ds.SizeSSZ(v1)
	if err != nil {
		detail := fmt.Sprintf("size: %v", err)
		t.Errorf("%s", detail)
		return "fail", detail
	}
	if size != len(serialized) {
		detail := fmt.Sprintf("DIVERGENCE size: got %d want %d", size, len(serialized))
		t.Errorf("%s", detail)
		return "fail", detail
	}

	// 3. Hash tree root.
	root, err := ds.HashTreeRoot(v1)
	if err != nil {
		detail := fmt.Sprintf("htr: %v", err)
		t.Errorf("%s", detail)
		return "fail", detail
	}
	if !equalBytes(root[:], wantRoot) {
		detail := fmt.Sprintf("DIVERGENCE root: got 0x%x want %s", root, vec.Root)
		t.Errorf("%s", detail)
		return "fail", detail
	}

	// 4. Decode and compare to the filled value.
	v2 := entry.New()
	if err := ds.UnmarshalSSZ(v2, serialized); err != nil {
		detail := fmt.Sprintf("DIVERGENCE decode: spec accepts but dynamic-ssz rejected: %v", err)
		t.Errorf("%s", detail)
		return "fail", detail
	}
	if diff := EqualNormalized(v1, v2); diff != "" {
		detail := fmt.Sprintf("DIVERGENCE value: decoded value differs from spec value: %s", diff)
		t.Errorf("%s", detail)
		return "fail", detail
	}

	// 5. Decode canonicity: S(D(b)) == b.
	enc2, err := ds.MarshalSSZ(v2)
	if err != nil {
		detail := fmt.Sprintf("re-marshal: %v", err)
		t.Errorf("%s", detail)
		return "fail", detail
	}
	if !equalBytes(enc2, serialized) {
		detail := fmt.Sprintf("DIVERGENCE canonicity: S(D(b)) = 0x%x != b = %s", enc2, vec.Serialized)
		t.Errorf("%s", detail)
		return "fail", detail
	}

	// 6. Root agreement on the decoded value.
	root2, err := ds.HashTreeRoot(v2)
	if err != nil {
		detail := fmt.Sprintf("htr(decoded): %v", err)
		t.Errorf("%s", detail)
		return "fail", detail
	}
	if !equalBytes(root2[:], wantRoot) {
		detail := fmt.Sprintf("DIVERGENCE root(decoded): got 0x%x want %s", root2, vec.Root)
		t.Errorf("%s", detail)
		return "fail", detail
	}

	return "pass", ""
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func shortID(testID string) string {
	if i := strings.LastIndex(testID, "::"); i >= 0 {
		testID = testID[i+2:]
	}
	return strings.TrimSuffix(testID, "[ssz_test]")
}
