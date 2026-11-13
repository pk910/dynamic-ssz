package harness

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/formal/harness/corpus"
	"github.com/pk910/dynamic-ssz/formal/harness/typegen"
	"pgregory.net/rapid"
)

// recursionDepth is the nesting depth of values of root's own type inside
// v: 0 when v holds none, 1 when it holds children of its own type with no
// grandchildren, and so on. Pointers are followed; nil is nothing.
func recursionDepth(v reflect.Value, root reflect.Type) int {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return 0
		}
		v = v.Elem()
	}
	depth := 0
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			d := recursionDepth(f, root)
			if deref(f).Type() == root && (f.Kind() != reflect.Pointer || !f.IsNil()) {
				d++
			}
			depth = max(depth, d)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			el := v.Index(i)
			d := recursionDepth(el, root)
			if deref(el).Type() == root && (el.Kind() != reflect.Pointer || !el.IsNil()) {
				d++
			}
			depth = max(depth, d)
		}
	default:
	}
	return depth
}

// recursiveDeclarer declares the unrolling of a recursive corpus type at a
// given depth to both oracles, once per depth, and names it.
type recursiveDeclarer struct {
	t        *testing.T
	o        *specOracle
	c        *certOracle
	declared map[string]bool
}

// name returns the oracle name of e unrolled to depth, declaring it first
// if needed. It returns "" (and reports) when the depth exceeds what the
// registry carries.
func (d *recursiveDeclarer) name(e corpus.Entry, depth int, spec bool) string {
	if depth >= len(e.Unrolled) {
		d.t.Logf("%s: a value of recursion depth %d exceeds the %d unrollings carried; not checked", e.Name, depth, len(e.Unrolled))
		return ""
	}
	n := fmt.Sprintf("%s_d%d", e.Name, depth)
	key := n
	if spec {
		key += "/spec"
	}
	if d.declared[key] {
		return n
	}
	d.declared[key] = true
	if spec {
		if e.Unrolled[depth] == "" {
			return ""
		}
		if ans := d.o.declare(n, e.Unrolled[depth]); ans != "OK" {
			d.t.Errorf("%s depth %d: declaration refused: %s", e.Name, depth, ans)
			return ""
		}
	} else if ans := d.c.declare(n, e.CertUnrolled[depth]); ans != "OK" {
		d.t.Errorf("%s depth %d: in-house declaration refused: %s", e.Name, depth, ans)
		return ""
	}
	return n
}

// TestCorpusRecursive links the recursive corpus types. A certificate is a
// finite term, so a recursive type is declared by the unrolling at the
// depth of each value: a value of depth d is checked against the depth-d
// unrolling (whose innermost self-references are placeholders no value of
// that depth reaches). Mutations are decoded by Go first; an accepted
// mutation is checked at its own depth, a rejected one must be rejected at
// the deepest unrolling as well.
func TestCorpusRecursive(t *testing.T) {
	var entries []corpus.Entry
	for _, e := range corpusEntries(t) {
		if e.Recursive {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		t.Skip("no recursive types in the corpus (stage rec)")
	}
	o := startSpecOracle(t)
	c := startCertOracle(t)
	eng := newEngineSet(typegen.Specs())
	decl := &recursiveDeclarer{t: t, o: o, c: c, declared: map[string]bool{}}
	deepest := typegen.MaxUnroll
	byDepth := map[string]map[int]int{}

	for _, e := range entries {
		e := e
		root := reflect.TypeOf(e.New()).Elem()
		spec := !e.Extended
		byDepth[e.Name] = map[int]int{}
		t.Run("rec/"+e.Name, func(t *testing.T) {
			o.t, c.t, decl.t = t, t, t
			defer func() { o.t, c.t, decl.t = t, t, t }()
			ctx := "rec/" + e.Name
			check := func(what string, raw []byte, goAccept bool, decoded any) {
				depth := deepest
				if goAccept {
					depth = recursionDepth(reflect.ValueOf(decoded), root)
				}
				byDepth[e.Name][depth]++
				if n := decl.name(e, depth, false); n != "" {
					accepted, root := c.decode(n, raw)
					if accepted != goAccept {
						t.Errorf("VERDICT DIVERGENCE %s %s (in-house, depth %d): go=%v model=%v (bytes 0x%s)", ctx, what, depth, goAccept, accepted, shortHex(raw))
					} else if accepted {
						eng.rootPaths(failer(t, ctx+" "+what), decoded, root)
					}
				}
				if spec {
					if n := decl.name(e, depth, true); n != "" {
						accepted, specRoot := o.decode(n, raw)
						if accepted != goAccept {
							t.Errorf("VERDICT DIVERGENCE %s %s (spec, depth %d): go=%v spec=%v (bytes 0x%s)", ctx, what, depth, goAccept, accepted, shortHex(raw))
						} else if accepted {
							eng.rootPaths(failer(t, ctx+" "+what), decoded, specRoot)
						}
					}
				}
			}
			rapid.Check(t, func(rt *rapid.T) {
				v, err := GenValue(rt, eng.refl, e.New, GenConfig{MaxElems: 3, MaxBytes: 64, MaxDepth: 2 + rapid.IntRange(0, 4).Draw(rt, "depth"), Boundary: true})
				if err != nil {
					t.Fatalf("generate %s: %v", e.Name, err)
				}
				enc, ok := eng.encodePaths(failer(t, ctx), v)
				if !ok {
					t.Fatalf("%s: generated value is not encodable", ctx)
				}
				okDec, back := eng.decodePaths(failer(t, ctx), e.New, enc)
				if !okDec {
					t.Errorf("ROUNDTRIP %s: engines reject their own encoding 0x%s", ctx, shortHex(enc))
					return
				}
				check("value", enc, true, back)
				mut := mutateBytes(rt, enc)
				goAccept, mutDecoded := eng.decodePaths(failer(t, ctx+" mutation"), e.New, mut)
				check("mutation", mut, goAccept, mutDecoded)
			})
		})
	}
	var lines []string
	for _, e := range entries {
		var parts []string
		for d := 0; d <= deepest; d++ {
			if n := byDepth[e.Name][d]; n > 0 {
				parts = append(parts, fmt.Sprintf("d%d:%d", d, n))
			}
		}
		lines = append(lines, e.Name+" {"+strings.Join(parts, " ")+"}")
	}
	t.Logf("recursive corpus: %d types; values by recursion depth: %s", len(entries), strings.Join(lines, "; "))
}
