package harness

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/formal/certify/certlib"
	"pgregory.net/rapid"
)

// declare submits a declaration to the oracle for this session (`T`), in the
// JSON spelling SpecJSON writes. The oracle evaluates EthLib's
// `Desc.wellFormed` before accepting it, so an answer of OK means EthLib's
// generic theorems hold for the declared type; ILLFORMED is returned as is.
func (o *specOracle) declare(name, declaration string) string {
	if strings.ContainsAny(declaration, " \t\r\n") {
		o.t.Fatalf("declaration of %s contains whitespace", name)
	}
	return o.ask("T " + name + " " + declaration)
}

// TestSpecLinkInline links every certified type through a declaration
// submitted at run time instead of its compiled certificate: the descriptor
// the reflection engine builds for the Go type is written as a JSON
// declaration, declared to the oracle, and the generative check of
// TestSpecLink runs against that declaration. On every value the inline and
// the compiled declaration must also agree on the root. This is the path a
// type never certified in a Lean file takes — generated type corpora in
// particular — so its correctness is what makes the link generic.
func TestSpecLinkInline(t *testing.T) {
	o := startSpecOracle(t)
	eng := newEngineSet(nil)
	rng := rand.New(rand.NewSource(20261113))
	st := &specStats{}
	declaredHere := 0

	for _, sub := range certifiedSubjects(t) {
		if !sub.spec {
			continue
		}
		sub := sub
		name := sub.name
		inline := "I_" + name
		desc, err := eng.refl.GetTypeCache().GetTypeDescriptor(reflect.TypeOf(sub.newFn()).Elem(), nil, nil, nil)
		if err != nil {
			t.Fatalf("descriptor of %s: %v", name, err)
		}
		declaration, err := certlib.SpecJSON(desc)
		if err != nil {
			t.Fatalf("declaration of %s: %v", name, err)
		}
		if ans := o.declare(inline, declaration); ans != "OK" {
			t.Fatalf("declare %s (%s): %s", name, declaration, ans)
		}
		declaredHere++
		t.Run("inline/"+name, func(t *testing.T) {
			o.t = t
			defer func() { o.t = t }()
			rapid.Check(t, func(rt *rapid.T) {
				v := sub.genValue(t, rt, eng)
				enc, ok := eng.encodePaths(failer(t, "inline/"+name), v)
				if !ok {
					t.Fatalf("inline/%s: generated value is not encodable", name)
				}
				accepted, specRoot := o.decode(inline, enc)
				compiledAccepted, compiledRoot := o.decode(name, enc)
				if accepted != compiledAccepted || specRoot != compiledRoot {
					t.Errorf("DECLARATION DIVERGENCE %s: inline (%v, %s) vs compiled (%v, %s) on 0x%s",
						name, accepted, specRoot, compiledAccepted, compiledRoot, shortHex(enc))
				}
				if !accepted {
					t.Errorf("VERDICT DIVERGENCE inline/%s: spec rejects generated encoding 0x%s", name, shortHex(enc))
				} else {
					o.checkValue("inline", inline, v, enc, specRoot, eng, rng, st)
				}

				mut := mutateBytes(rt, enc)
				goAccept, mutDecoded := eng.decodePaths(failer(t, "inline-mut/"+name), sub.newFn, mut)
				specAccept, mutRoot := o.decode(inline, mut)
				if goAccept != specAccept {
					t.Errorf("VERDICT DIVERGENCE inline/%s mutation: go=%v spec=%v (bytes 0x%s)",
						name, goAccept, specAccept, shortHex(mut))
				}
				if goAccept && specAccept {
					o.checkValue("inline-mut", inline, mutDecoded, mut, mutRoot, eng, rng, st)
				}
			})
		})
	}
	t.Logf("inline spec link: %d declarations; %d values rooted, %d branches, %d multiproofs, %d refusals agreed",
		declaredHere, st.vectors, st.proofs, st.multiproofs, st.refusedBoth)
}
