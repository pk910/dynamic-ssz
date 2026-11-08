package harness

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/treeproof"
	"pgregory.net/rapid"
)

// specOracle talks to ssz-spec-oracle (formal/spec), the decision oracle
// answered by the official Lean specification of SSZ (EthLib).
type specOracle struct {
	t  *testing.T
	wr *bufio.Writer
	rd *bufio.Reader
}

func startSpecOracle(t *testing.T) *specOracle {
	path := os.Getenv("SSZ_SPEC_ORACLE")
	if path == "" {
		path = filepath.Join("..", "spec", ".lake", "build", "bin", "ssz-spec-oracle")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("ssz-spec-oracle not built: %v", err)
	}
	cmd := exec.Command("bash", "-c", "ulimit -s unlimited 2>/dev/null; exec '"+path+"'")
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
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return &specOracle{t: t, wr: bufio.NewWriterSize(stdin, 1<<20), rd: bufio.NewReaderSize(stdout, 1<<20)}
}

func (o *specOracle) ask(line string) string {
	fmt.Fprintln(o.wr, line)
	if err := o.wr.Flush(); err != nil {
		o.t.Fatalf("write to oracle: %v", err)
	}
	ans, err := o.rd.ReadString('\n')
	if err != nil {
		o.t.Fatalf("oracle died on %q: %v", line, err)
	}
	ans = strings.TrimSpace(ans)
	if strings.HasPrefix(ans, "ERR") || strings.HasPrefix(ans, "MISMATCH") {
		o.t.Fatalf("oracle answered %q for %q", ans, line)
	}
	return ans
}

func hexOrDash(b []byte) string {
	if len(b) == 0 {
		return "-"
	}
	return hex.EncodeToString(b)
}

func hexListOrDash(bs [][]byte) string {
	if len(bs) == 0 {
		return "-"
	}
	parts := make([]string, len(bs))
	for i, b := range bs {
		parts[i] = hex.EncodeToString(b)
	}
	return strings.Join(parts, ",")
}

func intsCSV(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ",")
}

func parseHexListOrDash(s string) [][]byte {
	if s == "-" {
		return nil
	}
	var out [][]byte
	for _, p := range strings.Split(s, ",") {
		b, _ := hex.DecodeString(p)
		out = append(out, b)
	}
	return out
}

// decode answers the spec's verdict and root for raw bytes.
func (o *specOracle) decode(name string, raw []byte) (bool, string) {
	ans := o.ask("D " + name + " " + hexOrDash(raw))
	if root, ok := strings.CutPrefix(ans, "ACCEPT "); ok {
		return true, root
	}
	if strings.HasPrefix(ans, "REJECT") {
		return false, ""
	}
	o.t.Fatalf("unexpected oracle answer %q", ans)
	return false, ""
}

// treeIndices collects generalized indices of the explicit Go tree (BFS,
// capped), split into leaves and interior nodes.
func treeIndices(root *treeproof.Node, limit int) (leaves, inner []int) {
	type item struct {
		n  *treeproof.Node
		gi int
	}
	queue := []item{{root, 1}}
	for len(queue) > 0 && len(leaves)+len(inner) < limit {
		it := queue[0]
		queue = queue[1:]
		if it.n.IsLeaf() || it.n.Left() == nil || it.n.Right() == nil {
			if it.gi > 1 {
				leaves = append(leaves, it.gi)
			}
			continue
		}
		if it.gi > 1 {
			inner = append(inner, it.gi)
		}
		queue = append(queue, item{it.n.Left(), 2 * it.gi}, item{it.n.Right(), 2*it.gi + 1})
	}
	return
}

type specStats struct {
	vectors, proofs, multiproofs, refusedBoth, rootPolicy int
}

// checkValue links one accepted value: 4-way root agreement with the spec
// root, then Merkle branches and multiproofs built by dynamic-ssz's
// treeproof against EthLib's buildProof/buildMultiproof, cross-verified in
// both directions (P8).
func (o *specOracle) checkValue(ctx, name string, v any, raw []byte, specRoot string,
	eng *engineSet, rng *rand.Rand, st *specStats) {
	t := o.t
	fail := failer(t, ctx+" "+name)
	errsBefore := t.Failed()
	eng.rootPaths(fail, v, specRoot)
	if t.Failed() && !errsBefore {
		return
	}
	treeGen, err := eng.gen.GetTree(v)
	if err != nil {
		t.Errorf("%s %s: codegen GetTree: %v", ctx, name, err)
		return
	}
	rootGen := treeGen.Hash()
	st.vectors++

	leaves, inner := treeIndices(treeGen, 4096)
	rng.Shuffle(len(leaves), func(i, j int) { leaves[i], leaves[j] = leaves[j], leaves[i] })
	rng.Shuffle(len(inner), func(i, j int) { inner[i], inner[j] = inner[j], inner[i] })
	pick := append([]int{}, leaves[:min(6, len(leaves))]...)
	pick = append(pick, inner[:min(2, len(inner))]...)
	// Also probe below the explicit tree (padding / packed interiors) and the
	// root: both sides must agree on refusing or on the answer.
	if len(leaves) > 0 {
		pick = append(pick, 2*leaves[0], 2*leaves[0]+1)
	}
	pick = append(pick, 1)

	rawHex := hexOrDash(raw)
	for _, gi := range pick {
		goProof, goErr := treeGen.Prove(gi)
		ans := o.ask(fmt.Sprintf("P %s %s %d", name, rawHex, gi))
		if strings.HasPrefix(ans, "REJECT") {
			switch {
			case goErr != nil:
				st.refusedBoth++
			case gi == 1 && strings.Contains(ans, "ROOT_HAS_NO_BRANCH"):
				// Documented policy difference: the specification refuses a
				// branch for the root itself; Go returns the empty branch,
				// which must still verify trivially against the root.
				if ok, err := treeproof.VerifyProof(rootGen, goProof); err != nil || !ok {
					t.Errorf("ROOT PROOF %s %s: Go's empty root branch does not verify (ok=%v err=%v)", ctx, name, ok, err)
				}
				st.rootPolicy++
			default:
				t.Errorf("PROOF POLICY DIVERGENCE %s %s gindex %d: Go proves, spec refuses (%s)", ctx, name, gi, ans)
			}
			continue
		}
		if goErr != nil {
			t.Errorf("PROOF POLICY DIVERGENCE %s %s gindex %d: spec proves, Go refuses (%v)", ctx, name, gi, goErr)
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(ans, "PROOF "))
		if len(fields) != 2 {
			t.Fatalf("bad PROOF answer %q", ans)
		}
		specLeaf, specBranch := fields[0], parseHexListOrDash(fields[1])
		if hex.EncodeToString(goProof.Leaf) != specLeaf {
			t.Errorf("PROOF LEAF DIVERGENCE %s %s gindex %d: go=%x spec=%s", ctx, name, gi, goProof.Leaf, specLeaf)
		}
		if hexListOrDash(goProof.Hashes) != hexListOrDash(specBranch) {
			t.Errorf("PROOF BRANCH DIVERGENCE %s %s gindex %d:\n  go=%s\n  spec=%s", ctx, name, gi,
				hexListOrDash(goProof.Hashes), hexListOrDash(specBranch))
		}
		// Cross-verification: the spec verifies Go's branch; Go verifies the spec's.
		vans := o.ask(fmt.Sprintf("V %s %s %d %s %s", name, rawHex, gi, hex.EncodeToString(goProof.Leaf), hexListOrDash(goProof.Hashes)))
		if vans != "VERIFY true" {
			t.Errorf("PROOF CROSS-VERIFY %s %s gindex %d: spec rejects Go branch: %s", ctx, name, gi, vans)
		}
		leafBytes, _ := hex.DecodeString(specLeaf)
		ok, err := treeproof.VerifyProof(rootGen, &treeproof.Proof{Index: gi, Leaf: leafBytes, Hashes: specBranch})
		if err != nil || !ok {
			t.Errorf("PROOF CROSS-VERIFY %s %s gindex %d: Go rejects spec branch (ok=%v err=%v)", ctx, name, gi, ok, err)
		}
		st.proofs++
	}

	// Multiproof over 2–4 distinct leaves (leaves are never ancestors of each other).
	if len(leaves) >= 2 {
		k := 2 + rng.Intn(min(3, len(leaves)-1))
		idx := append([]int{}, leaves[:k]...)
		sort.Ints(idx)
		goMulti, goErr := treeGen.ProveMulti(idx)
		ans := o.ask(fmt.Sprintf("M %s %s %s", name, rawHex, intsCSV(idx)))
		switch {
		case strings.HasPrefix(ans, "REJECT"):
			if goErr == nil {
				t.Errorf("MULTIPROOF POLICY DIVERGENCE %s %s %v: Go proves, spec refuses (%s)", ctx, name, idx, ans)
			} else {
				st.refusedBoth++
			}
		case goErr != nil:
			t.Errorf("MULTIPROOF POLICY DIVERGENCE %s %s %v: spec proves, Go refuses (%v)", ctx, name, idx, goErr)
		default:
			fields := strings.Fields(strings.TrimPrefix(ans, "MULTI "))
			if len(fields) != 2 {
				t.Fatalf("bad MULTI answer %q", ans)
			}
			specLeaves, specProof := parseHexListOrDash(fields[0]), parseHexListOrDash(fields[1])
			if hexListOrDash(goMulti.Leaves) != hexListOrDash(specLeaves) {
				t.Errorf("MULTIPROOF LEAF DIVERGENCE %s %s %v: go=%s spec=%s", ctx, name, idx, hexListOrDash(goMulti.Leaves), hexListOrDash(specLeaves))
			}
			if hexListOrDash(goMulti.Hashes) != hexListOrDash(specProof) {
				t.Errorf("MULTIPROOF DIVERGENCE %s %s %v:\n  go=%s\n  spec=%s", ctx, name, idx, hexListOrDash(goMulti.Hashes), hexListOrDash(specProof))
			}
			vans := o.ask(fmt.Sprintf("W %s %s %s %s %s", name, rawHex, intsCSV(idx), hexListOrDash(goMulti.Leaves), hexListOrDash(goMulti.Hashes)))
			if vans != "VERIFY true" {
				t.Errorf("MULTIPROOF CROSS-VERIFY %s %s %v: spec rejects Go multiproof: %s", ctx, name, idx, vans)
			}
			ok, err := treeproof.VerifyMultiproof(rootGen, specProof, specLeaves, idx)
			if err != nil || !ok {
				t.Errorf("MULTIPROOF CROSS-VERIFY %s %s %v: Go rejects spec multiproof (ok=%v err=%v)", ctx, name, idx, ok, err)
			}
			st.multiproofs++
		}
	}
}

// TestSpecLink links dynamic-ssz (both engines) to the official Lean SSZ
// specification: decode verdicts, canonical re-serialization (a theorem on
// the spec side, re-executed by the oracle), hash tree roots, and Merkle
// proofs/multiproofs on every fixture vector plus generative coverage of
// every certified type.
func TestSpecLink(t *testing.T) {
	o := startSpecOracle(t)
	vectors := loadFixturesOrNone(t)
	eng := newEngineSet(nil)
	rng := rand.New(rand.NewSource(20261106))
	st := &specStats{}
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
		if strings.Contains(goName, "[") {
			continue
		}
		var raw []byte
		if vec.RejectionReason != "" {
			raw = mustHex(t, vec.RawBytes)
		} else {
			raw = mustHex(t, vec.Serialized)
		}
		ctx := vec.Dir + "/" + vec.TestID
		goAccept, decoded := eng.decodePaths(failer(t, ctx+" "+goName), entry.New, raw)
		specAccept, specRoot := o.decode(goName, raw)
		if goAccept != specAccept {
			t.Errorf("VERDICT DIVERGENCE %s type %s: go=%v spec=%v (bytes 0x%s)", ctx, goName, goAccept, specAccept, shortHex(raw))
		}
		if goAccept && specAccept {
			o.checkValue(ctx, goName, decoded, raw, specRoot, eng, rng, st)
		}
		checked++
		perType[goName]++
	}

	var names []string
	for _, sub := range certifiedSubjects(t) {
		if !sub.spec {
			continue // extended types: certified against the in-house model only
		}
		sub := sub
		name := sub.name
		names = append(names, name)
		t.Run("gen/"+name, func(t *testing.T) {
			o.t = t
			defer func() { o.t = t }()
			rapid.Check(t, func(rt *rapid.T) {
				v := sub.genValue(t, rt, eng)
				enc, ok := eng.encodePaths(failer(t, "gen/"+name), v)
				if !ok {
					t.Fatalf("gen/%s: generated value is not encodable", name)
				}
				accepted, specRoot := o.decode(name, enc)
				if !accepted {
					t.Errorf("VERDICT DIVERGENCE gen/%s: spec rejects generated encoding 0x%s", name, shortHex(enc))
				} else {
					o.checkValue("gen", name, v, enc, specRoot, eng, rng, st)
				}
				perType[name]++
				checked++

				mut := mutateBytes(rt, enc)
				goAccept, mutDecoded := eng.decodePaths(failer(t, "gen-mut/"+name), sub.newFn, mut)
				specAccept, mutRoot := o.decode(name, mut)
				if goAccept != specAccept {
					t.Errorf("VERDICT DIVERGENCE gen/%s mutation: go=%v spec=%v (bytes 0x%s)",
						name, goAccept, specAccept, shortHex(mut))
				}
				if goAccept && specAccept {
					o.checkValue("gen-mut", name, mutDecoded, mut, mutRoot, eng, rng, st)
				}
				checked++
			})
		})
	}

	for _, name := range names {
		if perType[name] == 0 {
			t.Errorf("COVERAGE GAP: no spec-link vector exercised %s", name)
		}
	}
	t.Logf("spec link: %d vectors checked across %d/%d types; %d values rooted, %d branches, %d multiproofs, %d refusals agreed, %d root-branch policy cases",
		checked, len(perType), len(names), st.vectors, st.proofs, st.multiproofs, st.refusedBoth, st.rootPolicy)
}
