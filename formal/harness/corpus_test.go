package harness

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/formal/certify/certlib"
	"github.com/pk910/dynamic-ssz/formal/harness/corpus"
	"github.com/pk910/dynamic-ssz/formal/harness/typegen"
	"pgregory.net/rapid"
)

// knownIssues lists library defects the corpus has found that are not yet
// fixed (results/ISSUES.md of the verification project): a failure of the
// listed type whose message contains the listed text is reported as a known
// issue instead of failing the test, so the corpus stays a regression gate
// for everything else. Remove an entry when the defect is fixed.
var knownIssues = map[string][]string{
	// L7: the generated HashTreeRootWith of a root [2]uint64 uint128 leaves an
	// unpadded 16-byte chunk in the hasher.
	"S1_Uint128Words":   {"generated/HashTreeRootWith"},
	"W_S1_Uint128Words": {"generated/HashTreeRootWith"},
}

// corpusSpecs are the spec values the corpus is linked under.
func corpusSpecs() map[string]any { return typegen.Specs() }

// corpusEntries returns the generated corpus, or skips.
func corpusEntries(t *testing.T) []corpus.Entry {
	if len(corpus.Registry) == 0 {
		t.Skip("corpus not generated (formal/corpus.sh)")
	}
	return corpus.Registry
}

// TestCorpusDeclarations checks, for every spec-defined corpus type, that the
// declaration dynamic-ssz derives from the Go type's tags (through the
// reflection engine's descriptor and the certifier's translation) is the one
// the shape was generated to have — the tag language, read generically.
func TestCorpusDeclarations(t *testing.T) {
	eng := newEngineSet(typegen.Specs())
	for _, e := range corpusEntries(t) {
		desc, err := eng.refl.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(e.New()).Elem(), nil, nil, nil)
		if err != nil {
			t.Errorf("%s (%s): descriptor: %v", e.Name, e.Shape, err)
			continue
		}
		if e.Recursive {
			// a recursive type has no finite declaration; its unrollings are
			// declared per value by TestCorpusRecursive
			if _, err := certlib.CertJSON(desc); !errors.Is(err, certlib.ErrRecursive) {
				t.Errorf("%s (%s): recursive type should have no declaration, got err=%v", e.Name, e.Shape, err)
			}
			continue
		}
		if cert, err := certlib.CertJSON(desc); err != nil {
			t.Errorf("%s (%s): no in-house declaration: %v", e.Name, e.Shape, err)
		} else if cert != e.CertDecl {
			t.Errorf("DECLARATION DIVERGENCE (in-house) %s (%s):\n  dynamic-ssz %s\n  intended    %s", e.Name, e.Shape, cert, e.CertDecl)
		}
		got, err := certlib.SpecJSON(desc)
		switch {
		case e.IllFormed:
		case e.Extended:
			if err == nil {
				t.Errorf("%s (%s): extended shape has a spec declaration %s", e.Name, e.Shape, got)
			}
		case err != nil:
			t.Errorf("%s (%s): no declaration: %v", e.Name, e.Shape, err)
		case got != e.Decl:
			t.Errorf("DECLARATION DIVERGENCE %s (%s):\n  dynamic-ssz %s\n  intended    %s", e.Name, e.Shape, got, e.Decl)
		}
	}
}

// TestCorpusLink links every corpus type generically: spec-defined shapes
// are declared to the spec oracle at run time (which proves them well-formed
// by evaluation, so EthLib's theorems apply) and checked as TestSpecLink
// checks a certified type — verdicts, canonical bytes, roots, branches and
// multiproofs on every API path of both engines, for generated values, their
// mutations and the limit probes. Extended shapes, which have no
// specification, are checked for agreement across all paths and engines and
// for decode-canonicity.
func TestCorpusLink(t *testing.T) {
	entries := corpusEntries(t)
	o := startSpecOracle(t)
	c := startCertOracle(t)
	eng := newEngineSet(typegen.Specs())
	rng := rand.New(rand.NewSource(20261113))
	st := &specStats{}
	linked := map[string]int{}

	for _, e := range entries {
		e := e
		if e.Recursive {
			continue // TestCorpusRecursive declares these per value depth
		}
		desc, err := eng.refl.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(e.New()).Elem(), nil, nil, nil)
		if err != nil {
			t.Errorf("%s: descriptor: %v", e.Name, err)
			continue
		}
		sub := subject{name: e.Name, newFn: e.New, spec: !e.Extended && !e.IllFormed}
		// Every shape has an in-house codec, built from the proof-carrying
		// combinators at run time.
		if ans := c.declare(e.Name, e.CertDecl); ans != "OK" {
			t.Errorf("%s (%s): in-house declaration refused: %s", e.Name, e.Shape, ans)
			continue
		}
		if e.IllFormed {
			// A shape the specification forbids: the oracle must say so; the
			// engines are then only held to agree with each other.
			if ans := o.declare(e.Name, e.Decl); !strings.HasPrefix(ans, "ILLFORMED") {
				t.Errorf("%s (%s): the specification should refuse this declaration, oracle said %s", e.Name, e.Shape, ans)
				continue
			}
		} else if sub.spec {
			if ans := o.declare(e.Name, e.Decl); ans != "OK" {
				t.Errorf("%s (%s): declaration refused: %s", e.Name, e.Shape, ans)
				continue
			}
		}
		t.Run(e.Stage+"/"+e.Name, func(t *testing.T) {
			o.t = t
			c.t = t
			defer func() { o.t = t; c.t = t }()
			ctx := e.Stage + "/" + e.Name
			// inHouse holds the engines to the in-house certified codec:
			// verdict agreement, and the root on every path; for spec-defined
			// shapes also agreement of the two formalizations' roots.
			inHouse := func(what string, raw []byte, goAccept bool, decoded any, specRoot string) {
				accepted, root := c.decode(e.Name, raw)
				if accepted != goAccept {
					t.Errorf("VERDICT DIVERGENCE %s %s (in-house): go=%v model=%v (bytes 0x%s)", ctx, what, goAccept, accepted, shortHex(raw))
					return
				}
				if !accepted {
					return
				}
				if specRoot != "" && specRoot != root {
					t.Errorf("MODEL DIVERGENCE %s %s: in-house root %s, specification root %s", ctx, what, root, specRoot)
				}
				eng.rootPaths(failer(t, ctx+" "+what), decoded, root)
			}
			rapid.Check(t, func(rt *rapid.T) {
				v := sub.genValue(t, rt, eng)
				enc, ok := eng.encodePaths(failer(t, ctx), v)
				if !ok {
					t.Fatalf("%s: generated value is not encodable", ctx)
				}
				mut := mutateBytes(rt, enc)
				goAccept, mutDecoded := eng.decodePaths(failer(t, ctx+" mutation"), e.New, mut)
				specRoot, mutRoot := "", ""
				if sub.spec {
					accepted, root := o.decode(e.Name, enc)
					if !accepted {
						t.Errorf("VERDICT DIVERGENCE %s: spec rejects generated encoding 0x%s", ctx, shortHex(enc))
					} else {
						specRoot = root
						o.checkValue(e.Stage, e.Name, v, enc, specRoot, eng, rng, st)
					}
					specAccept, root := o.decode(e.Name, mut)
					if goAccept != specAccept {
						t.Errorf("VERDICT DIVERGENCE %s mutation: go=%v spec=%v (bytes 0x%s)", ctx, goAccept, specAccept, shortHex(mut))
					}
					if goAccept && specAccept {
						mutRoot = root
						o.checkValue(e.Stage+"-mut", e.Name, mutDecoded, mut, mutRoot, eng, rng, st)
					}
				} else {
					// No specification: the engines must round-trip the value
					// and accept only canonical bytes (the in-house codec then
					// answers for verdicts and roots).
					okDec, back := eng.decodePaths(failer(t, ctx), e.New, enc)
					if !okDec {
						t.Errorf("ROUNDTRIP %s: engines reject their own encoding 0x%s", ctx, shortHex(enc))
					} else if diff := EqualNormalized(v, back); diff != "" {
						t.Errorf("ROUNDTRIP %s: decoded value differs: %s", ctx, diff)
					}
					if goAccept {
						again, ok := eng.encodePaths(failer(t, ctx+" mutation re-encode"), mutDecoded)
						if ok && string(again) != string(mut) {
							t.Errorf("CANONICITY %s: accepted 0x%s but re-encodes as 0x%s", ctx, shortHex(mut), shortHex(again))
						}
					}
				}
				inHouse("value", enc, true, v, specRoot)
				inHouse("mutation", mut, goAccept, mutDecoded, mutRoot)
				linked[e.Name]++
			})
			for i, probe := range limitProbes(desc) {
				goAccept, decoded := eng.decodePaths(failer(t, ctx+" limit probe"), e.New, probe)
				root := ""
				if sub.spec {
					specAccept, specRoot := o.decode(e.Name, probe)
					if goAccept != specAccept {
						t.Errorf("VERDICT DIVERGENCE %s limit probe %d: go=%v spec=%v (%d bytes)", ctx, i, goAccept, specAccept, len(probe))
					}
					if goAccept && specAccept {
						root = specRoot
						o.checkValue(e.Stage+"-limit", e.Name, decoded, probe, root, eng, rng, st)
					}
				}
				inHouse(fmt.Sprintf("limit probe %d", i), probe, goAccept, decoded, root)
			}
		})
	}
	t.Logf("corpus link: %d/%d types linked; %d values rooted, %d branches, %d multiproofs, %d refusals agreed",
		len(linked), len(entries), st.vectors, st.proofs, st.multiproofs, st.refusedBoth)
}
