package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
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
		// a hasher over the standard library's hash.Hash
		"stdlib": func() *hasher.Hasher { return hasher.NewHasherWithHash(sha256.New()) },
		// a hasher reused after Reset: the previous value must leave no trace
		"reset": func() *hasher.Hasher {
			h := hasher.NewHasher()
			h.PutUint64(0xdeadbeef)
			h.PutBytes(make([]byte, 100))
			h.Reset()
			return h
		},
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
		// The Append* family packs raw words; FillUpTo32 completes the chunk.
		{"BasicUint64Vector4/Append", func() any { return new(BasicUint64Vector4) }, func(h *hasher.Hasher, v any) error {
			for _, x := range *v.(*BasicUint64Vector4) {
				h.AppendUint64(x)
			}
			h.FillUpTo32()
			return nil
		}},
		{"BasicUint16Vector3/Append", func() any { return new(BasicUint16Vector3) }, func(h *hasher.Hasher, v any) error {
			for _, x := range *v.(*BasicUint16Vector3) {
				h.AppendUint16(x)
			}
			h.FillUpTo32()
			return nil
		}},
		{"BasicUint32/Append", func() any { return new(BasicUint32) }, func(h *hasher.Hasher, v any) error {
			h.AppendUint32(uint32(*v.(*BasicUint32)))
			h.FillUpTo32()
			return nil
		}},
		{"BasicUint8/Append", func() any { return new(BasicUint8) }, func(h *hasher.Hasher, v any) error {
			h.AppendUint8(uint8(*v.(*BasicUint8)))
			h.FillUpTo32()
			return nil
		}},
		{"BasicBool/Append", func() any { return new(BasicBool) }, func(h *hasher.Hasher, v any) error {
			h.AppendBool(bool(*v.(*BasicBool)))
			h.FillUpTo32()
			return nil
		}},
		{"BasicBytes32/Append", func() any { return new(BasicBytes32) }, func(h *hasher.Hasher, v any) error {
			b := *v.(*BasicBytes32)
			h.AppendBytes32(b[:])
			return nil
		}},
		// WithTemp lends scratch space; the value built there is appended.
		{"BasicBytes32/WithTemp", func() any { return new(BasicBytes32) }, func(h *hasher.Hasher, v any) error {
			b := *v.(*BasicBytes32)
			h.WithTemp(func(tmp []byte) []byte {
				tmp = append(tmp[:0], b[:]...)
				h.Append(tmp)
				return tmp
			})
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
				typeName, _, _ := strings.Cut(c.name, "/")
				ok, want := o.verdict(typeName, enc)
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
