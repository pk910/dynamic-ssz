package harness

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"pgregory.net/rapid"
)

// mutateBytes produces a structured mutation of enc: truncation, extension,
// bit flip, byte insertion, or emptying — the classic decode-canonicity
// attack shapes.
func mutateBytes(rt *rapid.T, enc []byte) []byte {
	mut := append([]byte(nil), enc...)
	switch rapid.IntRange(0, 4).Draw(rt, "mutkind") {
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

	dsGen := dynssz.NewDynSsz(nil)
	dsRefl := dynssz.NewDynSsz(nil, dynssz.WithNoDelegation())

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

	// checkRoots demands that both engines' HashTreeRoot and both engines'
	// GetTree(...).Hash() — the two independent Go merkleization paths —
	// equal the certified Lean root for the given decoded value.
	checkRoots := func(ctx, goName string, v any, leanRoot string) {
		rootGen, err := dsGen.HashTreeRoot(v)
		if err != nil {
			t.Errorf("%s %s: codegen HashTreeRoot: %v", ctx, goName, err)
			return
		}
		rootRefl, err := dsRefl.HashTreeRoot(v)
		if err != nil {
			t.Errorf("%s %s: reflection HashTreeRoot: %v", ctx, goName, err)
			return
		}
		treeGen, err := dsGen.GetTree(v)
		if err != nil {
			t.Errorf("%s %s: codegen GetTree: %v", ctx, goName, err)
			return
		}
		treeRefl, err := dsRefl.GetTree(v)
		if err != nil {
			t.Errorf("%s %s: reflection GetTree: %v", ctx, goName, err)
			return
		}
		hg := hex.EncodeToString(rootGen[:])
		hr := hex.EncodeToString(rootRefl[:])
		tg := hex.EncodeToString(treeGen.Hash())
		tr := hex.EncodeToString(treeRefl.Hash())
		if hg != leanRoot || hr != leanRoot || tg != leanRoot || tr != leanRoot {
			t.Errorf("ROOT DIVERGENCE %s %s: lean=%s codegenHTR=%s reflectionHTR=%s codegenTree=%s reflectionTree=%s",
				ctx, goName, leanRoot, hg, hr, tg, tr)
		}
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

		decoded := entry.New()
		genAccept := dsGen.UnmarshalSSZ(decoded, raw) == nil
		reflAccept := dsRefl.UnmarshalSSZ(entry.New(), raw) == nil
		oracleAccept, leanRoot := ask(goName, raw)

		if genAccept != oracleAccept || reflAccept != oracleAccept {
			t.Errorf("VERDICT DIVERGENCE %s/%s type %s: codegen=%v reflection=%v lean=%v (bytes 0x%x)",
				vec.Dir, vec.TestID, goName, genAccept, reflAccept, oracleAccept, raw)
		}
		if genAccept && oracleAccept {
			checkRoots(vec.Dir+"/"+vec.TestID, goName, decoded, leanRoot)
		}
		checked++
		perType[goName]++
	}

	// Phase 2: generative coverage. Every certified type gets random valid
	// values (must be accepted by the oracle) and byte mutations (verdict
	// agreement), including the types no release fixture exercises at the
	// top level.
	allTypes := map[string]func() any{
		"CUSquareProgList": func() any { return new(CUSquareProgList) },
		"CUCircleProgList": func() any { return new(CUCircleProgList) },
		"CUUint16List4":    func() any { return new(CUUint16List4) },
	}
	for _, reg := range Registries {
		for _, entry := range reg {
			entry := entry
			goName := reflect.TypeOf(entry.New()).Elem().Name()
			if strings.Contains(goName, "[") {
				continue
			}
			allTypes[goName] = entry.New
		}
	}
	names := make([]string, 0, len(allTypes))
	for name := range allTypes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		newFn := allTypes[name]
		t.Run("gen/"+name, func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				v := newFn()
				if err := RandomFill(rt, v); err != nil {
					t.Fatalf("random fill %s: %v", name, err)
				}
				enc, err := dsGen.MarshalSSZ(v)
				if err != nil {
					t.Fatalf("marshal %s: %v", name, err)
				}
				accepted, leanRoot := ask(name, enc)
				if !accepted {
					t.Errorf("VERDICT DIVERGENCE gen/%s: lean rejects generated encoding 0x%x", name, enc)
				} else {
					checkRoots("gen", name, v, leanRoot)
				}
				perType[name]++
				checked++

				mut := mutateBytes(rt, enc)
				mutDecoded := newFn()
				genAccept := dsGen.UnmarshalSSZ(mutDecoded, mut) == nil
				reflAccept := dsRefl.UnmarshalSSZ(newFn(), mut) == nil
				oracleAccept, mutRoot := ask(name, mut)
				if genAccept != oracleAccept || reflAccept != oracleAccept {
					t.Errorf("VERDICT DIVERGENCE gen/%s mutation: codegen=%v reflection=%v lean=%v (bytes 0x%x)",
						name, genAccept, reflAccept, oracleAccept, mut)
				}
				if genAccept && oracleAccept {
					checkRoots("gen-mut", name, mutDecoded, mutRoot)
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
	if len(names) != 57 {
		t.Errorf("expected 57 certified types, got %d", len(names))
	}
	t.Logf("oracle link: %d vectors checked across %d/%d types", checked, len(perType), len(names))
}
