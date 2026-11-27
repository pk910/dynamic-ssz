package harness

import (
	"bytes"
	"encoding/hex"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/formal/harness/typegen"
	"pgregory.net/rapid"
)

// TestCorpusViews links the schema views of the corpus (stage views): a data
// type serialized through a view type that drops or reorders its fields
// (WithViewDescriptor). The view's declaration is declared to both oracles;
// both engines, through buffers and roots, with and without generated view
// methods, must agree with the oracles on the view's bytes, verdicts and
// roots, and decoding through a view must re-encode to the same bytes.
func TestCorpusViews(t *testing.T) {
	entries := corpusEntries(t)
	var withViews int
	for _, e := range entries {
		withViews += len(e.Views)
	}
	if withViews == 0 {
		t.Skip("no views in the corpus (stage views)")
	}
	o := startSpecOracle(t)
	c := startCertOracle(t)
	eng := newEngineSet(typegen.Specs())
	engines := []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"codegen", eng.gen}, {"reflection", eng.refl}}
	linked := 0

	for _, e := range entries {
		for _, view := range e.Views {
			e, view := e, view
			name := e.Name + "__" + view.Name
			if ans := c.declare(name, view.CertDecl); ans != "OK" {
				t.Errorf("%s: in-house declaration refused: %s", name, ans)
				continue
			}
			spec := !view.Extended
			if spec {
				if ans := o.declare(name, view.Decl); ans != "OK" {
					t.Errorf("%s: declaration refused: %s", name, ans)
					continue
				}
			}
			t.Run("views/"+name, func(t *testing.T) {
				o.t, c.t = t, t
				defer func() { o.t, c.t = t, t }()
				opt := dynssz.WithViewDescriptor(view.New())
				rapid.Check(t, func(rt *rapid.T) {
					v := subject{name: e.Name, newFn: e.New}.genValue(t, rt, eng)
					// encode through the view on both engines
					var enc []byte
					for i, en := range engines {
						out, err := en.ds.MarshalSSZ(v, opt)
						if err != nil {
							t.Fatalf("%s: %s/MarshalSSZ(view): %v", name, en.name, err)
						}
						size, err := en.ds.SizeSSZ(v, opt)
						if err != nil || size != len(out) {
							t.Errorf("%s: %s/SizeSSZ(view)=%d err=%v, encoding has %d bytes", name, en.name, size, err, len(out))
						}
						if i == 0 {
							enc = out
						} else if !bytes.Equal(out, enc) {
							t.Errorf("VIEW ENCODE DIVERGENCE %s: %s=%x %s=%x", name, engines[0].name, enc, en.name, out)
						}
					}
					// the oracles' verdict and root
					okCert, certRoot := c.decode(name, enc)
					if !okCert {
						t.Errorf("VERDICT DIVERGENCE %s: in-house model rejects the view encoding 0x%s", name, shortHex(enc))
						return
					}
					if spec {
						okSpec, specRoot := o.decode(name, enc)
						if !okSpec {
							t.Errorf("VERDICT DIVERGENCE %s: spec rejects the view encoding 0x%s", name, shortHex(enc))
						} else if specRoot != certRoot {
							t.Errorf("MODEL DIVERGENCE %s: in-house %s spec %s", name, certRoot, specRoot)
						}
					}
					// roots through the view on both engines, direct and via the tree
					for _, en := range engines {
						r, err := en.ds.HashTreeRoot(v, opt)
						if err != nil || hex.EncodeToString(r[:]) != certRoot {
							t.Errorf("VIEW ROOT DIVERGENCE %s: %s/HashTreeRoot=%x err=%v model=%s", name, en.name, r, err, certRoot)
						}
						tree, err := en.ds.GetTree(v, opt)
						if err != nil || hex.EncodeToString(tree.Hash()) != certRoot {
							t.Errorf("VIEW ROOT DIVERGENCE %s: %s/GetTree err=%v model=%s", name, en.name, err, certRoot)
						}
					}
					// decode through the view and re-encode
					for _, en := range engines {
						back := e.New()
						if err := en.ds.UnmarshalSSZ(back, enc, opt); err != nil {
							t.Errorf("VIEW ROUNDTRIP %s: %s/UnmarshalSSZ: %v", name, en.name, err)
							continue
						}
						again, err := en.ds.MarshalSSZ(back, opt)
						if err != nil || !bytes.Equal(again, enc) {
							t.Errorf("VIEW ROUNDTRIP %s: %s re-encodes 0x%s as 0x%s (%v)", name, en.name, shortHex(enc), shortHex(again), err)
						}
					}
					// a mutation: engines agree with the oracles on the verdict
					mut := mutateBytes(rt, enc)
					var goAccept bool
					for i, en := range engines {
						err := en.ds.UnmarshalSSZ(e.New(), mut, opt)
						if i == 0 {
							goAccept = err == nil
						} else if (err == nil) != goAccept {
							t.Errorf("VIEW DECODE DIVERGENCE %s on 0x%s: %s err=%v", name, shortHex(mut), en.name, err)
						}
					}
					if accepted, _ := c.decode(name, mut); accepted != goAccept {
						t.Errorf("VERDICT DIVERGENCE %s mutation (in-house): go=%v model=%v (bytes 0x%s)", name, goAccept, accepted, shortHex(mut))
					}
					if spec {
						if accepted, _ := o.decode(name, mut); accepted != goAccept {
							t.Errorf("VERDICT DIVERGENCE %s mutation (spec): go=%v spec=%v (bytes 0x%s)", name, goAccept, accepted, shortHex(mut))
						}
					}
				})
			})
			linked++
		}
	}
	t.Logf("views: %d views linked", linked)
}
