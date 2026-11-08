package harness

import (
	"encoding/hex"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"gopkg.in/yaml.v2"
	"pgregory.net/rapid"
)

// TestEthLink links dynamic-ssz to both verified models on the Ethereum
// consensus types (phase0 to gloas, mainnet and minimal presets, certified in
// SszCert/Eth.lean and SszSpec/Eth.lean):
//
//   - every consensus-spec-tests ssz_static case (CONSENSUS_SPEC_TESTS_DIR,
//     the directory holding mainnet/ and minimal/): every decoding path must
//     accept it, every encoding path reproduce it, every merkleization path
//     produce the root pyspec recorded in roots.yaml, and both certified
//     decoders accept it with that same root — three independent anchors;
//   - generated values and byte mutations of every type (boundary-biased
//     lengths), with verdict and root agreement against both models.
//
// Cases larger than ETH_LINK_MAX_ORACLE_BYTES (default 2 MiB) are checked
// against pyspec's root and the Go paths only. ETH_LINK_FORKS and
// ETH_LINK_PRESETS restrict the scope (comma-separated).
func TestEthLink(t *testing.T) {
	cert := startCertOracle(t)
	spec := startSpecOracle(t)
	maxOracle := 2 << 20
	if v := os.Getenv("ETH_LINK_MAX_ORACLE_BYTES"); v != "" {
		maxOracle, _ = strconv.Atoi(v)
	}
	only := func(env, v string) bool {
		list := os.Getenv(env)
		if list == "" {
			return true
		}
		for _, x := range strings.Split(list, ",") {
			if x == v {
				return true
			}
		}
		return false
	}
	casesDir := os.Getenv("CONSENSUS_SPEC_TESTS_DIR")
	rng := rand.New(rand.NewSource(20261108))
	st := &specStats{}
	var replayed, oracleChecked, generated int
	for _, preset := range presets {
		if !only("ETH_LINK_PRESETS", preset) {
			continue
		}
		eng := newEngineSet(loadPreset(t, preset))
		for _, fork := range ethForks {
			if !only("ETH_LINK_FORKS", fork.Fork) {
				continue
			}
			for _, et := range fork.Types {
				name := ethCertName(preset, et.GoName)
				// Phase 1: consensus-spec-tests replay.
				if casesDir != "" {
					dir := filepath.Join(casesDir, preset, fork.Fork, "ssz_static", et.SpecName, "ssz_random")
					entries, _ := os.ReadDir(dir)
					sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
					for _, e := range entries {
						if !e.IsDir() {
							continue
						}
						ctx := preset + "/" + fork.Fork + "/" + et.SpecName + "/" + e.Name()
						raw, root := readSpecCase(t, filepath.Join(dir, e.Name()))
						fail := failer(t, ctx)
						ok, decoded := eng.decodePaths(fail, et.New, raw)
						if !ok {
							t.Errorf("%s: Go rejects a consensus-spec-tests encoding", ctx)
							continue
						}
						if enc, _ := eng.encodePaths(fail, decoded); hex.EncodeToString(enc) != hex.EncodeToString(raw) {
							t.Errorf("%s: re-encoding differs from the consensus-spec-tests encoding", ctx)
						}
						eng.rootPaths(fail, decoded, root)
						replayed++
						if len(raw) > maxOracle {
							continue
						}
						start := time.Now()
						if acc, r := cert.verdict(name, raw); !acc || r != root {
							t.Errorf("%s: SszCert model: accept=%v root=%s, pyspec root %s", ctx, acc, r, root)
						}
						if acc, r := spec.decode(name, raw); !acc || r != root {
							t.Errorf("%s: EthLib spec: accept=%v root=%s, pyspec root %s", ctx, acc, r, root)
						}
						if d := time.Since(start); d > 20*time.Second {
							t.Logf("%s: oracles took %v for %d bytes", ctx, d, len(raw))
						}
						oracleChecked++
					}
				}
				// Phase 2: generated values and mutations.
				sub := subject{name: name, newFn: et.New, spec: true}
				t.Run(ctx2(preset, fork.Fork, et.SpecName), func(t *testing.T) {
					cert.t, spec.t = t, t
					rapid.Check(t, func(rt *rapid.T) {
						v, err := GenValue(rt, eng.refl, sub.newFn, ethGen)
						if err != nil {
							t.Fatalf("generate %s: %v", name, err)
						}
						enc, ok := eng.encodePaths(failer(t, "gen/"+name), v)
						if !ok {
							t.Fatalf("gen/%s: generated value is not encodable", name)
						}
						acc, root := cert.verdict(name, enc)
						sacc, sroot := spec.decode(name, enc)
						if !acc || !sacc || root != sroot {
							t.Errorf("gen/%s: models disagree or reject (cert %v %s, spec %v %s) on 0x%s",
								name, acc, root, sacc, sroot, shortHex(enc))
							return
						}
						spec.checkValue("gen", name, v, enc, sroot, eng, rng, st)
						generated++
						mut := mutateBytes(rt, enc)
						goAcc, dec := eng.decodePaths(failer(t, "gen-mut/"+name), sub.newFn, mut)
						macc, mroot := cert.verdict(name, mut)
						msacc, msroot := spec.decode(name, mut)
						if goAcc != macc || goAcc != msacc {
							t.Errorf("VERDICT DIVERGENCE gen/%s mutation: go=%v cert=%v spec=%v (bytes 0x%s)",
								name, goAcc, macc, msacc, shortHex(mut))
						}
						if goAcc && macc && msacc {
							if mroot != msroot {
								t.Errorf("MODEL ROOT DIVERGENCE gen-mut/%s: cert=%s spec=%s", name, mroot, msroot)
							}
							eng.rootPaths(failer(t, "gen-mut/"+name), dec, msroot)
						}
					})
				})
				cert.t, spec.t = t, t
			}
		}
	}
	t.Logf("eth link: %d consensus-spec-tests cases replayed (%d also through both oracles), %d generated values; %d branches, %d multiproofs",
		replayed, oracleChecked, generated, st.proofs, st.multiproofs)
}

func ctx2(preset, fork, typ string) string { return preset + "/" + fork + "/" + typ }

// ethGen bounds generated Ethereum values (limits reach 2^40).
var ethGen = GenConfig{MaxElems: 24, MaxBytes: 3000, MaxDepth: 5, Boundary: true}

// readSpecCase loads one ssz_static case: the decoded serialization and the
// root pyspec recorded.
func readSpecCase(t *testing.T, dir string) ([]byte, string) {
	comp, err := os.ReadFile(filepath.Join(dir, "serialized.ssz_snappy"))
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	raw, err := snappy.Decode(nil, comp)
	if err != nil {
		t.Fatalf("%s: snappy: %v", dir, err)
	}
	rootsYaml, err := os.ReadFile(filepath.Join(dir, "roots.yaml"))
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	var roots struct {
		Root string `yaml:"root"`
	}
	if err := yaml.Unmarshal(rootsYaml, &roots); err != nil {
		t.Fatalf("%s: roots.yaml: %v", dir, err)
	}
	return raw, strings.TrimPrefix(roots.Root, "0x")
}
