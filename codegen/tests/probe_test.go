// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package tests

import (
	"bytes"
	"encoding/binary"
	"go/types"
	"reflect"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/ssztypes"

	"golang.org/x/tools/go/packages"
)

// probeShapes pairs each probe fixture with the delegate methods it carries.
var probeShapes = []struct {
	name  string
	typ   reflect.Type
	flags ssztypes.SszCompatFlag
}{
	{
		"ProbeMarshalOnly",
		reflect.TypeFor[ProbeMarshalOnly](),
		ssztypes.SszCompatFlagFastsszValueMarshaler,
	},
	{
		"ProbeStaticSurface",
		reflect.TypeFor[ProbeStaticSurface](),
		ssztypes.SszCompatFlagFastsszBufferMarshaler | ssztypes.SszCompatFlagFastsszSizer | ssztypes.SszCompatFlagFastsszUnmarshaler,
	},
	{
		"ProbeNoSizer",
		reflect.TypeFor[ProbeNoSizer](),
		ssztypes.SszCompatFlagFastsszBufferMarshaler | ssztypes.SszCompatFlagFastsszUnmarshaler,
	},
	{
		"ProbeFullFastssz",
		reflect.TypeFor[ProbeFullFastssz](),
		ssztypes.SszCompatFlagFastsszSurface | ssztypes.SszCompatFlagFastsszHashRoot,
	},
	{
		// Every method reaches this type by promotion, so none of them answers
		// for it and none is flagged.
		"ProbePromoted",
		reflect.TypeFor[ProbePromoted](),
		0,
	},
}

// The two type front ends must name the same methods for the same type: a
// family flag let them disagree, which delegated a type in generated code that
// reflection walked.
func TestFastsszProbesAgreeAcrossFrontEnds(t *testing.T) {
	t.Parallel()

	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedName,
		Dir:  ".",
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("load: %v", err)
	}
	scope := pkgs[0].Types.Scope()

	const probed = ssztypes.SszCompatFlagFastsszSurface | ssztypes.SszCompatFlagFastsszHashRoot

	for _, shape := range probeShapes {
		t.Run(shape.name, func(t *testing.T) {
			t.Parallel()

			reflDesc, err := ssztypes.NewTypeCache(nil).GetTypeDescriptor(reflect.PointerTo(shape.typ), nil, nil, nil)
			if err != nil {
				t.Fatalf("reflect descriptor: %v", err)
			}
			if got := reflDesc.SszCompatFlags & probed; got != shape.flags {
				t.Errorf("reflect front end flags = %b, want %b", got, shape.flags)
			}

			obj := scope.Lookup(shape.name)
			if obj == nil {
				t.Fatalf("%s not found in the tests package", shape.name)
			}
			goDesc, err := codegen.NewParser().GetTypeDescriptor(types.NewPointer(obj.Type()), nil, nil, nil)
			if err != nil {
				t.Fatalf("parser descriptor: %v", err)
			}
			if got := goDesc.SszCompatFlags & probed; got != shape.flags {
				t.Errorf("go/types front end flags = %b, want %b", got, shape.flags)
			}
		})
	}
}

// Each operation reaches the method that serves it, and the ones a type does
// not provide are walked instead. The encodings are identical either way, so
// only the call counts tell the paths apart -- and both engines must make the
// same choice for the same type, which a flag naming a family of methods did
// not guarantee.
func TestFastsszProbePathsPerMethod(t *testing.T) {
	holder := &ProbeHolder{
		A: ProbeMarshalOnly{V: 1},
		B: ProbeStaticSurface{V: 2},
		C: ProbeNoSizer{V: 3},
		D: ProbeFullFastssz{V: 4},
		E: ProbePromoted{ProbePromotedInner: ProbePromotedInner{V: 5}, W: 6},
		F: []byte{7, 8},
		G: ProbeDynamicSizer{V: []byte{9}},
	}

	want := make([]byte, 0, 61)
	for _, v := range []uint64{1, 2, 3, 4, 5, 6} {
		want = binary.LittleEndian.AppendUint64(want, v)
	}
	want = binary.LittleEndian.AppendUint32(want, 56)
	want = binary.LittleEndian.AppendUint32(want, 58)
	want = append(want, 7, 8)
	want = binary.LittleEndian.AppendUint32(want, 4)
	want = append(want, 9)

	type counts struct {
		marshalSSZ, marshalSSZTo, sizeSSZ, unmarshalSSZ int64
	}

	for _, engine := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{
		// The first reaches ProbeHolder's generated methods, the second walks
		// the holder and meets each probe as a field.
		{"generated", dynssz.NewDynSsz(nil)},
		{"reflection", dynssz.NewDynSsz(nil, dynssz.WithNoDelegation())},
	} {
		t.Run(engine.name, func(t *testing.T) {
			var got counts

			ResetProbeCounts()
			data, err := engine.ds.MarshalSSZ(holder)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !bytes.Equal(data, want) {
				t.Fatalf("marshal = %x, want %x", data, want)
			}
			got.marshalSSZ = ProbeMarshalSSZCalls.Load()
			got.marshalSSZTo = ProbeMarshalSSZToCalls.Load()

			ResetProbeCounts()
			size, err := engine.ds.SizeSSZ(holder)
			if err != nil || size != len(want) {
				t.Fatalf("size = %d, %v, want %d", size, err, len(want))
			}
			got.sizeSSZ = ProbeSizeSSZCalls.Load()

			ResetProbeCounts()
			decoded := &ProbeHolder{}
			if err := engine.ds.UnmarshalSSZ(decoded, data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(decoded, holder) {
				t.Fatalf("round trip = %+v, want %+v", decoded, holder)
			}
			got.unmarshalSSZ = ProbeUnmarshalSSZCalls.Load()

			// A has only the buffer-returning marshaller and is reached through
			// it; D prefers MarshalSSZTo over its own MarshalSSZ; B, C and G
			// marshal themselves; E is walked, because its methods are promoted
			// from the embedded value, which then serves itself.
			want := counts{marshalSSZ: 1, marshalSSZTo: 5, sizeSSZ: 1, unmarshalSSZ: 5}
			if got != want {
				t.Errorf("delegate calls = %+v, want %+v", got, want)
			}
		})
	}
}
