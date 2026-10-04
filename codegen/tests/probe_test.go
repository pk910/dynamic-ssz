// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package tests

import (
	"bytes"
	"encoding/binary"
	"errors"
	"go/types"
	"io"
	"reflect"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"

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
	{
		// One promoted method is enough, whichever it is.
		"ProbePromotedValue",
		reflect.TypeFor[ProbePromotedValue](),
		0,
	},
	{
		// The promoted sizer answers for the embedded value and is dropped;
		// the marshaller this type declares itself is kept.
		"ProbeMixedPromotion",
		reflect.TypeFor[ProbeMixedPromotion](),
		ssztypes.SszCompatFlagFastsszBufferMarshaler,
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

	ds := dynssz.NewDynSsz(nil)
	walked := &ProbeWalkHolder{
		A: holder.A, B: holder.B, C: holder.C, D: holder.D,
		E: holder.E, F: holder.F, G: holder.G,
	}

	for _, engine := range []struct {
		name  string
		value any
	}{
		// The first reaches ProbeHolder's generated methods. The second holds
		// the same values in a type with no methods of its own, so reflection
		// walks it and meets each probe as a field.
		{"generated", holder},
		{"walked", walked},
	} {
		t.Run(engine.name, func(t *testing.T) {
			var got counts

			ResetProbeCounts()
			data, err := ds.MarshalSSZ(engine.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !bytes.Equal(data, want) {
				t.Fatalf("marshal = %x, want %x", data, want)
			}
			got.marshalSSZ = ProbeMarshalSSZCalls.Load()
			got.marshalSSZTo = ProbeMarshalSSZToCalls.Load()

			ResetProbeCounts()
			size, err := ds.SizeSSZ(engine.value)
			if err != nil || size != len(want) {
				t.Fatalf("size = %d, %v, want %d", size, err, len(want))
			}
			got.sizeSSZ = ProbeSizeSSZCalls.Load()

			ResetProbeCounts()
			decoded := reflect.New(reflect.TypeOf(engine.value).Elem()).Interface()
			if err := ds.UnmarshalSSZ(decoded, data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(decoded, engine.value) {
				t.Fatalf("round trip = %+v, want %+v", decoded, engine.value)
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

// The error a delegate reports travels back to the caller through either
// engine, with the field that produced it named.
func TestFastsszProbeFallbackError(t *testing.T) {
	// The probe call counters are package state, so the tests that read them
	// run one at a time.
	ds := dynssz.NewDynSsz(nil)

	for _, tc := range []struct {
		name  string
		value any
	}{
		{"generated", &ProbeFailHolder{}},
		{"walked", &ProbeFailWalkHolder{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ds.MarshalSSZ(tc.value)
			if !errors.Is(err, ErrProbeMarshal) {
				t.Fatalf("marshal err = %v, want the delegate's error", err)
			}
			// The streaming form reaches the same fallback and must report the
			// same error.
			if err := ds.MarshalSSZWriter(tc.value, io.Discard); !errors.Is(err, ErrProbeMarshal) {
				t.Fatalf("stream marshal err = %v, want the delegate's error", err)
			}
		})
	}
}

// A promoted method answers for the embedded value, so a type that reaches one
// is walked and keeps every field it declares.
func TestFastsszPromotedSurfaceIsNotDelegated(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	holder := &ProbePromotionHolder{
		A: ProbePromotedValue{ProbePromotedValueInner: ProbePromotedValueInner{V: 1}, W: 2},
		B: ProbeMixedPromotion{ProbeMixedPromotionInner: ProbeMixedPromotionInner{V: 3}, W: 4},
	}

	want := make([]byte, 0, 32)
	for _, v := range []uint64{1, 2, 3, 4} {
		want = binary.LittleEndian.AppendUint64(want, v)
	}

	ResetProbeCounts()
	data, err := ds.MarshalSSZ(holder)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("marshal = %x, want %x", data, want)
	}
	// The embedded value is a field of its own and serves itself there, which
	// is the MarshalSSZ call. B marshals through the method it declares itself,
	// beside a promoted sizer that is not used. What must not happen is the
	// outer type being served by a method that answers for an embedded value.
	if got := ProbeMarshalSSZCalls.Load(); got != 1 {
		t.Errorf("MarshalSSZ calls = %d, want the embedded field serving itself", got)
	}
	if got := ProbeMarshalSSZToCalls.Load(); got != 1 {
		t.Errorf("MarshalSSZTo calls = %d, want the direct marshaller used once", got)
	}

	ResetProbeCounts()
	size, err := ds.SizeSSZ(holder)
	if err != nil || size != len(want) {
		t.Fatalf("size = %d, %v, want %d", size, err, len(want))
	}
	if got := ProbeSizeSSZCalls.Load(); got != 0 {
		t.Errorf("SizeSSZ calls = %d, want none: the sizer is promoted", got)
	}

	decoded := &ProbePromotionHolder{}
	if err := ds.UnmarshalSSZ(decoded, data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if *decoded != *holder {
		t.Fatalf("round trip = %+v, want %+v", decoded, holder)
	}
}

// A field type hint naming the SSZ type a method-carrying type resolves to on
// its own leaves the type's methods in charge; a hint naming another SSZ type
// describes the field inline. Both front ends decide alike, and both engines
// reach the methods on the same fields.
func TestTypeHintOverridesOnlyAnotherType(t *testing.T) {
	const surface = ssztypes.SszCompatFlagFastsszValueMarshaler | ssztypes.SszCompatFlagFastsszUnmarshaler | ssztypes.SszCompatFlagFastsszHashRoot
	want := map[string]ssztypes.SszCompatFlag{"Plain": surface, "Same": surface, "Other": 0, "GridSame": surface, "GridElem": 0, "GridOpen": 0, "OuterSame": surface, "OuterOpen": surface, "OuterList": 0, "AnnList": surface, "LimSame": surface, "LimOther": 0}

	for _, typ := range []reflect.Type{reflect.TypeFor[ProbeHintedWalked](), reflect.TypeFor[ProbeHintedOverride]()} {
		reflDesc, err := ssztypes.NewTypeCache(nil).GetTypeDescriptor(typ, nil, nil, nil)
		if err != nil {
			t.Fatalf("reflect descriptor of %v: %v", typ, err)
		}
		for _, field := range reflDesc.ContainerDesc.Fields {
			if got := field.Type.SszCompatFlags & surface; got != want[field.Name] {
				t.Errorf("reflect front end, %s: flags %b, want %b", field.Name, got, want[field.Name])
			}
		}
	}

	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedName,
		Dir:  ".",
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("load: %v", err)
	}
	parser := codegen.NewParser()
	// The generator reads the package's Annotate calls; the parser is handed
	// the one annotation these shapes carry.
	parser.AnnotationResolver = func(typ types.Type) string {
		if named, ok := typ.(*types.Named); ok {
			switch named.Obj().Name() {
			case "ProbeHintedElem":
				return `ssz-size:"4"`
			case "ProbeHintedOuterAnn":
				return `ssz-type:"vector,list" ssz-max:"?,4"`
			case "ProbeHintedLimited":
				return `ssz-max:"4"`
			}
		}
		return ""
	}
	for _, name := range []string{"ProbeHintedWalked", "ProbeHintedOverride"} {
		obj := pkgs[0].Types.Scope().Lookup(name)
		if obj == nil {
			t.Fatalf("%s not found in the tests package", name)
		}
		goDesc, descErr := parser.GetTypeDescriptor(obj.Type(), nil, nil, nil)
		if descErr != nil {
			t.Fatalf("parser descriptor of %s: %v", name, descErr)
		}
		for _, field := range goDesc.ContainerDesc.Fields {
			if got := field.Type.SszCompatFlags & surface; got != want[field.Name] {
				t.Errorf("go/types front end, %s: flags %b, want %b", field.Name, got, want[field.Name])
			}
		}
	}

	if _, generated := any(&ProbeHintedHolder{}).(sszutils.DynamicMarshaler); !generated {
		t.Skip("no generated code present")
	}
	ds := dynssz.NewDynSsz(nil)
	grid := ProbeHintedGrid{{1, 2, 3, 4}, {5, 6, 7, 8}}
	outer := ProbeHintedOuter{{9, 10, 11, 12}, {13, 14, 15, 16}}
	annotated := ProbeHintedOuterAnn{{17, 18}, {19}}
	limited := ProbeHintedLimited{{20}, {21, 22}}
	refRoot, refErr := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation()).HashTreeRoot(&ProbeHintedWalked{Plain: ProbeHinted{1}, Same: ProbeHinted{2}, Other: ProbeHinted{3}, GridSame: grid, GridElem: grid, GridOpen: grid, OuterSame: outer, OuterOpen: outer, AnnList: annotated, LimSame: limited, LimOther: limited})
	if refErr != nil {
		t.Fatalf("reflection HashTreeRoot: %v", refErr)
	}
	for _, holder := range []any{
		&ProbeHintedHolder{Plain: ProbeHinted{1}, Same: ProbeHinted{2}, Other: ProbeHinted{3}, GridSame: grid, GridElem: grid, GridOpen: grid, OuterSame: outer, OuterOpen: outer, AnnList: annotated, LimSame: limited, LimOther: limited},
		&ProbeHintedWalked{Plain: ProbeHinted{1}, Same: ProbeHinted{2}, Other: ProbeHinted{3}, GridSame: grid, GridElem: grid, GridOpen: grid, OuterSame: outer, OuterOpen: outer, AnnList: annotated, LimSame: limited, LimOther: limited},
	} {
		ResetProbeCounts()
		encoded, err := ds.MarshalSSZ(holder)
		if err != nil {
			t.Fatalf("%T: MarshalSSZ: %v", holder, err)
		}
		if got := ProbeMarshalSSZCalls.Load(); got != 7 {
			t.Errorf("%T: MarshalSSZ reached the methods %d times, want 7 (Plain, Same, GridSame, OuterSame, OuterOpen, AnnList and LimSame)", holder, got)
		}
		back := reflect.New(reflect.TypeOf(holder).Elem()).Interface()
		if err = ds.UnmarshalSSZ(back, encoded); err != nil {
			t.Fatalf("%T: UnmarshalSSZ: %v", holder, err)
		}
		if got := ProbeUnmarshalSSZCalls.Load(); got != 7 {
			t.Errorf("%T: UnmarshalSSZ reached the methods %d times, want 7 (Plain, Same, GridSame, OuterSame, OuterOpen, AnnList and LimSame)", holder, got)
		}
		if !reflect.DeepEqual(back, holder) {
			t.Errorf("%T: round trip changed the value", holder)
		}
		root, err := ds.HashTreeRoot(holder)
		if err != nil {
			t.Fatalf("%T: HashTreeRoot: %v", holder, err)
		}
		if got := ProbeHashTreeRootCalls.Load(); got != 7 {
			t.Errorf("%T: HashTreeRoot reached the methods %d times, want 7 (Plain, Same, GridSame, OuterSame, OuterOpen, AnnList and LimSame)", holder, got)
		}
		if root != refRoot {
			t.Errorf("%T: root %x != reflection root %x", holder, root[:8], refRoot[:8])
		}
	}
}
