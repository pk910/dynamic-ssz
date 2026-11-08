package harness

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// mutateBytes produces a structured mutation of enc: truncation, extension,
// bit flip, byte insertion, or emptying — the classic decode-canonicity
// attack shapes.
func mutateBytes(rt *rapid.T, enc []byte) []byte {
	mut := append([]byte(nil), enc...)
	switch rapid.IntRange(0, 6).Draw(rt, "mutkind") {
	case 5: // append one element of a common width (lists one over their limit)
		w := rapid.SampledFrom([]int{1, 2, 4, 8, 16, 32, 48, 96}).Draw(rt, "elemw")
		for i := 0; i < w; i++ {
			mut = append(mut, byte(rapid.IntRange(0, 255).Draw(rt, "eb")))
		}
		return mut
	case 6: // drop one element of a common width
		w := rapid.SampledFrom([]int{1, 2, 4, 8, 16, 32}).Draw(rt, "elemw")
		if len(mut) >= w {
			return mut[:len(mut)-w]
		}
		return mut
	case 0: // truncate
		if len(mut) == 0 {
			return []byte{0}
		}
		n := rapid.IntRange(1, len(mut)).Draw(rt, "cut")
		return mut[:len(mut)-n]
	case 1: // append
		n := rapid.IntRange(1, 4).Draw(rt, "add")
		for i := 0; i < n; i++ {
			mut = append(mut, byte(rapid.IntRange(0, 255).Draw(rt, "b")))
		}
		return mut
	case 2: // flip a bit
		if len(mut) == 0 {
			return []byte{1}
		}
		i := rapid.IntRange(0, len(mut)-1).Draw(rt, "pos")
		mut[i] ^= byte(1 << rapid.IntRange(0, 7).Draw(rt, "bit"))
		return mut
	case 3: // insert a byte
		i := rapid.IntRange(0, len(mut)).Draw(rt, "ins")
		b := byte(rapid.IntRange(0, 255).Draw(rt, "ib"))
		mut = append(mut[:i], append([]byte{b}, mut[i:]...)...)
		return mut
	default: // empty
		return nil
	}
}

// TestOracleLink cross-checks the accept/reject decision of dynamic-ssz
// (generated-code engine and reflection engine) against the certified Lean
// oracle (formal/lean's ssz-oracle, built from the machine-checked codec
// certificates) on every fixture vector. Canonicity of accepted buffers is a
// theorem on the Lean side and re-executed byte-for-byte by the oracle's
// self-check, so verdict agreement here links the Go implementations to the
// verified model on this corpus.
func TestOracleLink(t *testing.T) {
	oraclePath := os.Getenv("SSZ_ORACLE")
	if oraclePath == "" {
		oraclePath = filepath.Join("..", "lean", ".lake", "build", "bin", "ssz-oracle")
	}
	if _, err := os.Stat(oraclePath); err != nil {
		t.Skipf("ssz-oracle not built: %v", err)
	}

	vectors := loadFixturesOrNone(t)

	// The model decoders recurse structurally; give the oracle an unbounded
	// stack so deep buffers cannot fault it.
	cmd := exec.Command("bash", "-c",
		"ulimit -s unlimited 2>/dev/null; exec '"+oraclePath+"'")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start oracle: %v", err)
	}
	defer cmd.Process.Kill()
	rd := bufio.NewReaderSize(stdout, 1<<20)
	wr := bufio.NewWriterSize(stdin, 1<<20)

	eng := newEngineSet(nil)

	// ask returns the certified Lean decoder's verdict for the given type
	// name and bytes, plus the certified hash tree root on accepts;
	// ERR/MISMATCH answers abort the test.
	ask := func(goName string, raw []byte) (bool, string) {
		hexStr := "-"
		if len(raw) > 0 {
			hexStr = hex.EncodeToString(raw)
		}
		fmt.Fprintf(wr, "%s %s\n", goName, hexStr)
		if err := wr.Flush(); err != nil {
			t.Fatalf("write to oracle: %v", err)
		}
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("oracle died on %s: %v", goName, err)
		}
		verdict := strings.TrimSpace(line)
		if root, ok := strings.CutPrefix(verdict, "ACCEPT "); ok {
			return true, root
		}
		if verdict == "REJECT" {
			return false, ""
		}
		t.Fatalf("oracle answered %q for %s (bytes %s)", verdict, goName, hexStr)
		return false, ""
	}

	checked := 0
	perType := map[string]int{}
	for _, vec := range vectors {
		reg, ok := Registries[vec.Dir]
		if !ok {
			continue
		}
		entry, ok := reg[vec.TypeName]
		if !ok {
			continue
		}
		goName := reflect.TypeOf(entry.New()).Elem().Name()
		// Standalone generic union aliases cannot carry generated methods
		// (Go limitation); they are certified through container fields.
		if strings.Contains(goName, "[") {
			continue
		}

		var raw []byte
		if vec.RejectionReason != "" {
			raw = mustHex(t, vec.RawBytes)
		} else {
			raw = mustHex(t, vec.Serialized)
		}

		ctx := vec.Dir + "/" + vec.TestID + " " + goName
		fail := failer(t, ctx)
		goAccept, decoded := eng.decodePaths(fail, entry.New, raw)
		oracleAccept, leanRoot := ask(goName, raw)
		if goAccept != oracleAccept {
			t.Errorf("VERDICT DIVERGENCE %s: go=%v lean=%v (bytes 0x%s)", ctx, goAccept, oracleAccept, shortHex(raw))
		}
		if goAccept && oracleAccept {
			eng.rootPaths(fail, decoded, leanRoot)
		}
		checked++
		perType[goName]++
	}

	// Phase 2: generative coverage. Every certified type gets random valid
	// values drawn from its descriptor, boundary-biased (must be accepted by
	// the oracle on every encoding path and root identically on every
	// merkleization path), and byte mutations (verdict agreement on every
	// decoding path).
	subjects := certifiedSubjects(t)
	names := make([]string, 0, len(subjects))
	for _, sub := range subjects {
		sub := sub
		names = append(names, sub.name)
		t.Run("gen/"+sub.name, func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				name := sub.name
				v := sub.genValue(t, rt, eng)
				fail := failer(t, "gen/"+name)
				enc, ok := eng.encodePaths(fail, v)
				if !ok {
					t.Fatalf("gen/%s: generated value is not encodable", name)
				}
				accepted, leanRoot := ask(name, enc)
				if !accepted {
					t.Errorf("VERDICT DIVERGENCE gen/%s: lean rejects generated encoding 0x%s", name, shortHex(enc))
				} else {
					eng.rootPaths(fail, v, leanRoot)
					if goAccept, decoded := eng.decodePaths(fail, sub.newFn, enc); !goAccept {
						t.Errorf("VERDICT DIVERGENCE gen/%s: go rejects its own encoding 0x%s", name, shortHex(enc))
					} else {
						eng.rootPaths(fail, decoded, leanRoot)
					}
				}
				perType[name]++
				checked++

				mut := mutateBytes(rt, enc)
				mfail := failer(t, "gen-mut/"+name)
				goAccept, mutDecoded := eng.decodePaths(mfail, sub.newFn, mut)
				oracleAccept, mutRoot := ask(name, mut)
				if goAccept != oracleAccept {
					t.Errorf("VERDICT DIVERGENCE gen/%s mutation: go=%v lean=%v (bytes 0x%s)",
						name, goAccept, oracleAccept, shortHex(mut))
				}
				if goAccept && oracleAccept {
					eng.rootPaths(mfail, mutDecoded, mutRoot)
				}
				checked++
			})
		})
	}

	// Every generated (certified) type must have been exercised.
	for _, name := range names {
		if perType[name] == 0 {
			t.Errorf("COVERAGE GAP: no oracle-link vector exercised %s", name)
		}
	}
	if len(names) == 0 {
		t.Error("no certified types")
	}
	t.Logf("oracle link: %d vectors checked across %d/%d types", checked, len(perType), len(names))
}
