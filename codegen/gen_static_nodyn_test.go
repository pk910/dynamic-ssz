// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// ---- deeply nested generated containers ----

type nsLeaf struct {
	A []byte `ssz-max:"8"`
	B uint8
}
type nsD5 struct {
	L nsLeaf
	X uint16
}
type nsD4 struct {
	N nsD5
	L []nsD5 `ssz-max:"4"`
}
type nsD3 struct {
	N nsD4
	V [2]nsD4
}
type nsD2 struct {
	N nsD3
	P []nsD3 `ssz-type:"progressive-list" ssz-max:"8"`
}
type nsD1 struct {
	N nsD2
	Z uint64
}

// ---- container-of-container as list/vector/progressive-list ----

type nsListOfC struct {
	L []nsLeaf `ssz-max:"16"`
}
type nsVecOfC struct {
	V [3]nsLeaf
}
type nsProgOfC struct {
	L []nsLeaf `ssz-type:"progressive-list" ssz-max:"16"`
}

// ---- TypeWrapper wrapping a nested generated container (unexported variant) ----

type nsWrapperHolder struct {
	W dynssz.TypeWrapper[struct {
		Data nsD4 `ssz-size:"?"`
	}, nsD4]
}

// ---- union whose variants are nested generated containers (unexported) ----

type nsUnionHolder struct {
	U dynssz.CompatibleUnion[struct {
		F1 nsLeaf
		F2 nsD4
	}]
}

// ---- optional / optional-list of nested generated containers ----

type nsOptHolder struct {
	Opt *nsD4 `ssz-type:"optional"`
}
type nsOptListHolder struct {
	Opt *nsD4 `ssz-type:"optional-list"`
}

// ---- self-referential + mutually recursive (all in generation set) ----

type nsSelfRec struct {
	V     []byte      `ssz-max:"4"`
	Peers []nsSelfRec `ssz-max:"4"`
}
type nsMutA struct {
	V []byte   `ssz-max:"4"`
	B []nsMutB `ssz-max:"4"`
}
type nsMutB struct {
	V []byte   `ssz-max:"4"`
	A []nsMutA `ssz-max:"4"`
}

var nsDynTokens = []string{"MarshalSSZDyn", "UnmarshalSSZDyn", "SizeSSZDyn", "HashTreeRootWithDyn"}

func nsAssertNoDyn(t *testing.T, name, code string) {
	t.Helper()
	if code == "" {
		t.Fatalf("%s: no code generated", name)
	}
	for _, tok := range nsDynTokens {
		if idx := strings.Index(code, tok); idx >= 0 {
			lo, hi := idx-140, idx+140
			if lo < 0 {
				lo = 0
			}
			if hi > len(code) {
				hi = len(code)
			}
			t.Errorf("%s: forbidden %s under without-dynamic-expressions near:\n...%s...", name, tok, code[lo:hi])
		}
	}
}

// nsCombos enumerates flag combinations that must all keep the generated buffer
// AND streaming paths free of *Dyn calls under WithoutDynamicExpressions.
func nsCombos() []struct {
	name string
	opts []CodeGeneratorOption
} {
	return []struct {
		name string
		opts []CodeGeneratorOption
	}{
		{"plain", nil},
		{"nofast", []CodeGeneratorOption{WithNoFastSsz()}},
		{"streaming", []CodeGeneratorOption{WithCreateEncoderFn(), WithCreateDecoderFn()}},
		{"nofast+streaming", []CodeGeneratorOption{WithNoFastSsz(), WithCreateEncoderFn(), WithCreateDecoderFn()}},
		{"legacy", []CodeGeneratorOption{WithCreateLegacyFn()}},
		{"all", []CodeGeneratorOption{WithNoFastSsz(), WithCreateEncoderFn(), WithCreateDecoderFn(), WithCreateLegacyFn()}},
	}
}

// TestStaticNoDynNested generates a large family of nested generated containers
// (deep containers-of-containers, list/vector/progressive-list nesting, union and
// TypeWrapper variants with unexported nested types, optional/optional-list)
// together under WithoutDynamicExpressions across every flag combo. The generated
// code must be valid Go and must never reference a *Dyn buffer function — in the
// buffer methods or in the streaming encoder/decoder.
func TestStaticNoDynNested(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeFor[nsLeaf](), reflect.TypeFor[nsD5](), reflect.TypeFor[nsD4](),
		reflect.TypeFor[nsD3](), reflect.TypeFor[nsD2](), reflect.TypeFor[nsD1](),
		reflect.TypeFor[nsListOfC](), reflect.TypeFor[nsVecOfC](), reflect.TypeFor[nsProgOfC](),
		reflect.TypeFor[nsWrapperHolder](), reflect.TypeFor[nsUnionHolder](),
		reflect.TypeFor[nsOptHolder](), reflect.TypeFor[nsOptListHolder](),
	}
	for _, combo := range nsCombos() {
		t.Run(combo.name, func(t *testing.T) {
			typeOpts := append([]CodeGeneratorOption{WithoutDynamicExpressions(), WithExtendedTypes()}, combo.opts...)
			cg := NewCodeGenerator(nil)
			buildOpts := make([]CodeGeneratorOption, 0, len(types))
			for _, rt := range types {
				buildOpts = append(buildOpts, WithReflectType(rt, typeOpts...))
			}
			cg.BuildFile("gen.go", buildOpts...)
			files, err := cg.GenerateToMap()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			nsAssertNoDyn(t, combo.name, files["gen.go"])
		})
	}
}

// TestStaticNoDynRecursion checks self-referential and mutually-recursive types
// terminate (no infinite generator loop) and stay *Dyn-free across combos.
func TestStaticNoDynRecursion(t *testing.T) {
	for _, combo := range nsCombos() {
		t.Run(combo.name, func(t *testing.T) {
			typeOpts := append([]CodeGeneratorOption{WithoutDynamicExpressions()}, combo.opts...)
			cg := NewCodeGenerator(nil)
			cg.BuildFile("gen.go",
				WithReflectType(reflect.TypeFor[nsSelfRec](), typeOpts...),
				WithReflectType(reflect.TypeFor[nsMutA](), typeOpts...),
				WithReflectType(reflect.TypeFor[nsMutB](), typeOpts...),
				// A pointer-element cycle plus a holder that merely contains
				// it: under the static build the streaming size closures must
				// delegate to the cycle member's static sizer — a walk that
				// inlines it instead grows the output without end, so this
				// generation completing at all is the regression pin.
				WithReflectType(reflect.TypeFor[nsPtrRec](), typeOpts...),
				WithReflectType(reflect.TypeFor[nsPtrRecHolder](), typeOpts...),
			)
			files, err := cg.GenerateToMap()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			nsAssertNoDyn(t, combo.name, files["gen.go"])
		})
	}
}

// nsPtrRec closes a cycle through a pointer-element list, the shape whose
// streaming size closure once inlined itself without end under the static
// build (see TestStaticNoDynRecursion).
type nsPtrRec struct {
	V uint8
	L []*nsPtrRec `ssz-max:"4"`
}

// nsPtrRecHolder threads the bound through a type that is not on the cycle.
type nsPtrRecHolder struct {
	Tag  uint32
	Node nsPtrRec
}

// TestStaticStreamingUnexportedGenericArg is a regression guard for the type-name
// printer: a generic type argument (a CompatibleUnion / TypeWrapper parameter)
// referencing an UNEXPORTED same-package type must be emitted as an unqualified
// identifier, never as its full import path. The reflect type-name cleanup regex
// previously only matched exported ([A-Z]) type names, so an unexported variant
// leaked "github.com/.../pkg.typeName" into the streaming encoder's sizeFn
// signatures, producing invalid Go.
func TestStaticStreamingUnexportedGenericArg(t *testing.T) {
	for _, rt := range []reflect.Type{reflect.TypeFor[nsUnionHolder](), reflect.TypeFor[nsWrapperHolder]()} {
		cg := NewCodeGenerator(nil)
		cg.BuildFile("gen.go", WithReflectType(rt, WithExtendedTypes(), WithCreateEncoderFn(), WithCreateDecoderFn()))
		files, err := cg.GenerateToMap()
		if err != nil {
			t.Fatalf("%s: generate: %v", rt, err)
		}
		if strings.Contains(files["gen.go"], "dynamic-ssz/codegen.") {
			t.Errorf("%s: import path leaked as a type name in generated code", rt)
		}
	}
}

// nsWellDelegated is an external fully-delegated type whose Go struct layout
// matches its wire form exactly. Under WithoutDynamicExpressions it is inlined
// from its structure; the inlined static output must equal its Dynamic* method.
type nsWellDelegated struct {
	V uint64
}

var _ = sszutils.Annotate[nsWellDelegated](`ssz-static:"true"`)

func (n *nsWellDelegated) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 8 }
func (n *nsWellDelegated) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint64(buf, n.V), nil
}
func (n *nsWellDelegated) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.V = binary.LittleEndian.Uint64(buf)
	return nil
}
func (n *nsWellDelegated) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(n.V)
	return nil
}

type nsWellHolder struct {
	A uint64
	N nsWellDelegated
	V [2]nsWellDelegated
}

// TestStaticStreamingInlinesExternalDelegated is the regression guard for the
// streaming encoder/decoder *Dyn leak: an external dynamic-only delegated child
// nested in a WithoutDynamicExpressions type must be inlined from its structure
// in BOTH the buffer methods and the streaming encoder/decoder, never reached via
// MarshalSSZDyn/UnmarshalSSZDyn. Previously the streaming generators cleared the
// WithoutDynamicExpressions flag wholesale and forwarded to the *Dyn buffer method.
func TestStaticStreamingInlinesExternalDelegated(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen.go", WithReflectType(reflect.TypeFor[nsWellHolder](),
		WithoutDynamicExpressions(), WithNoFastSsz(), WithCreateEncoderFn(), WithCreateDecoderFn()))
	files, err := cg.GenerateToMap()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	nsAssertNoDyn(t, "external-delegated-streaming", files["gen.go"])
	// The child's uint64 field must be inlined structurally in the streaming path.
	if !strings.Contains(files["gen.go"], "enc.EncodeUint64(t.V)") {
		t.Errorf("expected the streaming encoder to inline the delegated child's uint64 field")
	}
}

// nsIllegalDelegated mirrors the repo's nestedDelegatedInner: a delegated type
// with a structurally-invalid innard (zero-length array). Under
// WithoutDynamicExpressions the parser traverses it (NoDelegation) and must
// reject it with a clear error rather than emit a *Dyn call or panic.
type nsIllegalDelegated struct {
	Bad   [0]uint64
	Value uint32
}

var _ = sszutils.Annotate[nsIllegalDelegated](`ssz-static:"true"`)

func (n *nsIllegalDelegated) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 4 }
func (n *nsIllegalDelegated) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.LittleEndian.AppendUint32(buf, n.Value), nil
}
func (n *nsIllegalDelegated) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	n.Value = binary.LittleEndian.Uint32(buf)
	return nil
}
func (n *nsIllegalDelegated) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint32(n.Value)
	return nil
}

type nsIllegalHolder struct {
	A uint64
	N nsIllegalDelegated
}

// TestStaticRejectsUnInlinableDelegated confirms a delegated type that cannot be
// faithfully inlined (structurally-invalid innard) is rejected with a clear error
// instead of silently producing wrong code or panicking.
func TestStaticRejectsUnInlinableDelegated(t *testing.T) {
	cg := NewCodeGenerator(nil)
	cg.BuildFile("gen.go", WithReflectType(reflect.TypeFor[nsIllegalHolder](), WithoutDynamicExpressions()))
	if _, err := cg.GenerateToMap(); err == nil {
		t.Fatal("expected a clear error inlining a delegated type with an invalid innard")
	}
}
