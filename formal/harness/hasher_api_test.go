package harness

import (
	"encoding/hex"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"pgregory.net/rapid"
)

// TestHasherAPILink links the public hasher helpers that neither engine
// calls (generated code inlines its reductions) but external callers such as
// fastssz-style methods use: PutBitlist, PutProgressiveBitlist,
// PutUint64Array (vector and list), PutRootVector (vector and list),
// PutBytes, and the basic Put* — each against the certified root of the SSZ
// type it stands for, on random values, with the SIMD and the native sha256
// backends.
func TestHasherAPILink(t *testing.T) {
	o := startCertOracle(t)
	eng := newEngineSet(nil)
	hashers := map[string]func() *hasher.Hasher{
		"fast":   func() *hasher.Hasher { return hasher.FastHasherPool.Get() },
		"native": hasher.NewHasher,
	}
	type apiCase struct {
		name  string
		newFn func() any
		put   func(h *hasher.Hasher, v any) error
	}
	cases := []apiCase{
		{"BasicBitList16", func() any { return new(BasicBitList16) }, func(h *hasher.Hasher, v any) error {
			h.PutBitlist(*v.(*BasicBitList16), 16)
			return nil
		}},
		{"ProgBitList", func() any { return new(ProgBitList) }, func(h *hasher.Hasher, v any) error {
			h.PutProgressiveBitlist(*v.(*ProgBitList))
			return nil
		}},
		{"BasicUint64Vector4", func() any { return new(BasicUint64Vector4) }, func(h *hasher.Hasher, v any) error {
			a := *v.(*BasicUint64Vector4)
			h.PutUint64Array(a[:])
			return nil
		}},
		{"APIUint64List100", func() any { return new(APIUint64List100) }, func(h *hasher.Hasher, v any) error {
			h.PutUint64Array(*v.(*APIUint64List100), 100)
			return nil
		}},
		{"APIRoots5", func() any { return new(APIRoots5) }, func(h *hasher.Hasher, v any) error {
			a := v.(*APIRoots5)
			roots := make([][]byte, len(a))
			for i := range a {
				roots[i] = a[i][:]
			}
			return h.PutRootVector(roots)
		}},
		{"BasicBytes32List8", func() any { return new(BasicBytes32List8) }, func(h *hasher.Hasher, v any) error {
			l := *v.(*BasicBytes32List8)
			roots := make([][]byte, len(l))
			for i := range l {
				roots[i] = l[i][:]
			}
			return h.PutRootVector(roots, 8)
		}},
		{"BasicBytes52", func() any { return new(BasicBytes52) }, func(h *hasher.Hasher, v any) error {
			b := *v.(*BasicBytes52)
			h.PutBytes(b[:])
			return nil
		}},
		{"BasicBytes64", func() any { return new(BasicBytes64) }, func(h *hasher.Hasher, v any) error {
			b := *v.(*BasicBytes64)
			h.PutBytes(b[:])
			return nil
		}},
		{"BasicUint64", func() any { return new(BasicUint64) }, func(h *hasher.Hasher, v any) error {
			h.PutUint64(uint64(*v.(*BasicUint64)))
			return nil
		}},
		{"BasicUint32", func() any { return new(BasicUint32) }, func(h *hasher.Hasher, v any) error {
			h.PutUint32(uint32(*v.(*BasicUint32)))
			return nil
		}},
		{"BasicUint16", func() any { return new(BasicUint16) }, func(h *hasher.Hasher, v any) error {
			h.PutUint16(uint16(*v.(*BasicUint16)))
			return nil
		}},
		{"BasicUint8", func() any { return new(BasicUint8) }, func(h *hasher.Hasher, v any) error {
			h.PutUint8(uint8(*v.(*BasicUint8)))
			return nil
		}},
		{"BasicBool", func() any { return new(BasicBool) }, func(h *hasher.Hasher, v any) error {
			h.PutBool(bool(*v.(*BasicBool)))
			return nil
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			o.t = t
			rapid.Check(t, func(rt *rapid.T) {
				v, err := GenValue(rt, eng.refl, c.newFn, DefaultGen)
				if err != nil {
					t.Fatalf("generate: %v", err)
				}
				enc, err := eng.gen.MarshalSSZ(v)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				ok, want := o.verdict(c.name, enc)
				if !ok {
					t.Fatalf("model rejects 0x%s", shortHex(enc))
				}
				for hn, mk := range hashers {
					h := mk()
					if err := c.put(h, v); err != nil {
						t.Errorf("%s/%s: %v", c.name, hn, err)
						continue
					}
					root, err := h.HashRoot()
					if err != nil {
						t.Errorf("%s/%s: HashRoot: %v", c.name, hn, err)
						continue
					}
					if got := hex.EncodeToString(root[:]); got != want {
						t.Errorf("HASHER API DIVERGENCE %s/%s: got %s, model %s (value 0x%s)", c.name, hn, got, want, shortHex(enc))
					}
				}
			})
		})
	}
}
