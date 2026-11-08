package harness

import (
	"bytes"
	"reflect"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"pgregory.net/rapid"
)

// P9 preset-parametricity: a schema whose sizes come from dynssz-* spec-value
// expressions must behave exactly like a statically-specialized twin schema
// for any spec assignment. This is dynamic-ssz's core novel obligation — the
// ssz-specs fixtures cannot cover it.

// PresetStatic is the statically-specialized twin: sizes written out for
// REG_LIMIT=64, SYNC_SIZE=48, FIELD_SIZE=48, SHARDS=8, POINTS=24.
type PresetStatic struct {
	Roots [][32]byte `ssz-max:"64"`
	Bits  []byte     `ssz-type:"bitvector" ssz-size:"6" ssz-bitsize:"48"`
	Data  []byte     `ssz-size:"96"`
	Grid  [][]uint16 `ssz-max:"8,24"`
	Flags []byte     `ssz-type:"bitlist" ssz-max:"96"`
}

// PresetDynamic carries deliberately different static fallbacks; the dynssz-*
// expressions must override them under the test specs.
type PresetDynamic struct {
	Roots [][32]byte `ssz-max:"128" dynssz-max:"REG_LIMIT"`
	Bits  []byte     `ssz-type:"bitvector" ssz-size:"64" ssz-bitsize:"512" dynssz-size:"SYNC_SIZE/8" dynssz-bitsize:"SYNC_SIZE"`
	Data  []byte     `ssz-size:"32" dynssz-size:"FIELD_SIZE*2"`
	Grid  [][]uint16 `ssz-max:"4,2048" dynssz-max:"SHARDS,POINTS"`
	Flags []byte     `ssz-type:"bitlist" ssz-max:"2048" dynssz-max:"FIELD_SIZE*2"`
}

var presetSpecs = map[string]any{
	"REG_LIMIT":  uint64(64),
	"SYNC_SIZE":  uint64(48),
	"FIELD_SIZE": uint64(48),
	"SHARDS":     uint64(8),
	"POINTS":     uint64(24),
}

func TestPropPresetParametricity(t *testing.T) {
	dsStatic := dynssz.NewDynSsz(nil)
	dsDynamic := dynssz.NewDynSsz(presetSpecs)

	rapid.Check(t, func(rt *rapid.T) {
		gen, err := GenValue(rt, dynssz.NewDynSsz(nil, dynssz.WithNoDelegation()), func() any { return new(PresetStatic) }, DefaultGen)
		if err != nil {
			rt.Fatalf("gen: %v", err)
		}
		s := *gen.(*PresetStatic)
		// Same data, dynamic schema.
		var d PresetDynamic
		copyFields(&d, &s)

		bs, err := dsStatic.MarshalSSZ(&s)
		if err != nil {
			rt.Fatalf("static marshal: %v", err)
		}
		bd, err := dsDynamic.MarshalSSZ(&d)
		if err != nil {
			rt.Fatalf("dynamic marshal: %v", err)
		}
		if !bytes.Equal(bs, bd) {
			rt.Fatalf("P9 VIOLATION: static 0x%x != dynamic 0x%x", bs, bd)
		}

		rs, err := dsStatic.HashTreeRoot(&s)
		if err != nil {
			rt.Fatalf("static htr: %v", err)
		}
		rd, err := dsDynamic.HashTreeRoot(&d)
		if err != nil {
			rt.Fatalf("dynamic htr: %v", err)
		}
		if rs != rd {
			rt.Fatalf("P9 VIOLATION: static root 0x%x != dynamic root 0x%x", rs, rd)
		}

		// Cross-decode: the dynamic schema must accept the static encoding and
		// reproduce the same value.
		var d2 PresetDynamic
		if err := dsDynamic.UnmarshalSSZ(&d2, bs); err != nil {
			rt.Fatalf("dynamic decode of static encoding failed: %v", err)
		}
		if diff := EqualNormalized(&d, &d2); diff != "" {
			rt.Fatalf("P9: cross-decoded value differs: %s", diff)
		}
	})
}

// copyFields copies identically-typed exported fields from src to dst
// (different struct types, same field layout).
func copyFields(dst, src any) {
	dv := reflect.ValueOf(dst).Elem()
	sv := reflect.ValueOf(src).Elem()
	for i := 0; i < sv.NumField(); i++ {
		dv.Field(i).Set(sv.Field(i))
	}
}
