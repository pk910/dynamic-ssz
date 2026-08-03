package tests

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/codegen/tests/views"
	"github.com/pk910/dynamic-ssz/sszutils"
)

type TestPayload struct {
	Name     string         // Test name
	Payload  any            // Test payload
	View     any            // Test view
	Specs    map[string]any // Dynamic specifications
	Hash     string         // Expected hash root
	Extended bool           // Payload needs extended types to hash
}

var testMatrix = []TestPayload{
	{
		Name:    "SimpleTypes1",
		Payload: SimpleTypes1_Payload,
		Specs:   map[string]any{},
		Hash:    "bec0d5229e4833e2d8c37b1faca5ef770809daee540caf5809951676ee234f58",
	},
	{
		Name:    "SimpleTypes2",
		Payload: SimpleTypes2_Payload,
		Specs:   map[string]any{},
		Hash:    "8026899f40abd06e808372e98a47af2d87cd60ed4d9b44a495a029b825ef2b34",
	},
	{
		Name:    "SimpleTypes3",
		Payload: SimpleTypes3_Payload,
		Specs:   map[string]any{},
		Hash:    "7ae2dd701191c23929aa6e665ec82840881d2ecdbc821852c70692584653b46b",
	},
	{
		Name:    "SimpleTypesWithSpecs",
		Payload: SimpleTypesWithSpecs_Payload,
		Specs:   SimpleTypesWithSpecs_Specs,
		Hash:    "92a6c92ba823ca5421ac5d4d5652c8a79c2be2fc9b7c27cac098257a4c04871d",
	},
	{
		Name:    "SimpleTypesWithSpecs2",
		Payload: SimpleTypesWithSpecs2_Payload,
		Specs:   SimpleTypesWithSpecs_Specs,
		Hash:    "9982ff6cd691c967e02d67c5e729cb071b0f2d54735a5c26c18202c6a7a51714",
	},
	{
		Name:    "ProgressiveTypes",
		Payload: ProgressiveTypes_Payload,
		Specs:   map[string]any{},
		Hash:    "38a69cbd79a59c60505dac63c0330a57737f891a352cda1acd879cd778ca8cff",
	},
	{
		// classic spec unions: None selected (U1), a dynamic variant (U2) and
		// a value-carrying selector 0 (U3). The mix_in_selector construction
		// itself is pinned independently in TestUnionHashTreeRoot (root pkg).
		Name:    "ClassicUnionTypes",
		Payload: ClassicUnionTypes_Payload,
		Specs:   map[string]any{},
		Hash:    "87141e56fc9d1fa6b2cc3cfdd0283bcc77b744cf7d9b9bbbb5b309d0d5a67bef",
	},
	{
		// progressive container auto-detected from ssz-index tags alone
		Name:    "ProgIndexOnly",
		Payload: ProgIndexOnly_Payload,
		Specs:   map[string]any{},
		Hash:    "0c3b77007ca813db8a3d0ced4634530d9db15785e4b7a2b6c21db45c9ccd6409",
	},
	{
		// ssz-max:"0" is a no-limit placeholder, not a zero limit
		Name:     "ZeroMaxList",
		Payload:  ZeroMaxList_Payload,
		Specs:    map[string]any{},
		Extended: true,
		Hash:     "8dfcc0c61e1cfbec317bfc62c874364d717f1ba3ca13cfe07d86864883c24093",
	},
	{
		// A list element whose fixed section shrinks with the spec preset: the
		// decoders may not bound the declared count by a generated constant.
		Name:    "SpecShrunkList",
		Payload: SpecShrunkList_Payload,
		Specs:   SpecShrunkList_Specs,
		Hash:    "1d01f38b53c776df6e3a72e9594dff762175c0411c619ee60519f718c71d0f7e",
	},
	{
		// A Go array whose SSZ length resolves above its static ssz-size.
		Name:    "VecSpecLen",
		Payload: VecSpecLen_Payload,
		Specs:   VecSpecLen_Specs,
		Hash:    "2a904e56b27cee459633a119d2413867dbe38405517bd33404bf5cf597de5291",
	},
	{
		// Lists of type wrappers around basic values: transparent, so they must
		// merkleize like the plain lists they alias.
		Name:    "WrappedElemLists",
		Payload: WrappedElemLists_Payload,
		Specs:   map[string]any{},
		Hash:    "c2286724d36b75bd9e5a20a3b0af93d41da4ebfe8951aac9018741e25b481724",
	},
	{
		// A list element that is a vector of dynamic containers with a
		// spec-driven length: its minimum is the vector's offset table plus each
		// entry's fixed section.
		Name:    "SpecVecList",
		Payload: SpecVecList_Payload,
		Specs:   SpecShrunkList_Specs,
		Hash:    "00111a3bcce33ac26823df94e590d1021c3c480c9d1a7a1cd0cc966259b1473e",
	},
	{
		// EIP-7916 progressive-bitlist with an all-zero top 256-bit chunk. The
		// golden root is cross-checked against ethereum/remerkleable and an
		// independent raw-SHA256 implementation. Reflection and codegen share the
		// hasher, so this golden (not the differential check) is what guards the
		// chunk-count regression. See ProgBitlistZeroTop in types.go.
		Name:    "ProgBitlistZeroTop",
		Payload: ProgBitlistZeroTop_Payload,
		Specs:   map[string]any{},
		Hash:    "b039aa14167fdfd184839eb032e714ef89e0b42478e2db1ed1353759c200dda5",
	},
	{
		Name:    "ViewTypes_View1",
		Payload: ViewTypes1_Payload,
		View:    (*ViewTypes1_View1)(nil),
		Specs:   map[string]any{},
		Hash:    "e356af1d78a71ba3c5d8dd1d513f58bb82f6640b413bf9648d0a0435f967a5fe",
	},
	{
		Name:    "ViewTypes_View2",
		Payload: ViewTypes1_Payload,
		View:    (*ViewTypes1_View2)(nil),
		Specs:   map[string]any{},
		Hash:    "3b00274c7a34ebc3e1d9b1e63d620569cf5278848be7b15b26a848ef4d975861",
	},
	{
		Name:    "ViewTypes_View3",
		Payload: ViewTypes1_Payload,
		View:    (*views.ViewTypes1_View3)(nil),
		Specs:   map[string]any{},
		Hash:    "1bee9de04dd4f275d8c785741e5ae754bc95d6cf3d6abf1f98c3a41d066f557f",
	},
	{
		Name:    "AnnotatedContainer",
		Payload: AnnotatedContainer_Payload,
		Specs:   map[string]any{},
		Hash:    "683902f02e8035c2301b0eac540d4e311d24638abd660e4fd8f580db8e63a89d",
	},
	{
		Name:    "AnnotatedOverrideContainer",
		Payload: AnnotatedOverrideContainer_Payload,
		Specs:   map[string]any{},
		Hash:    "54c8f24f17a7e2d9e94b9e85fa70732fa91682e2c4b674343ae1df7bd0d17c56",
	},
	{
		Name:    "AnnotatedSpecsContainer",
		Payload: AnnotatedSpecsContainer_Payload,
		Specs:   AnnotatedSpecs,
		Hash:    "909350b0e5b120f7adc6261f9e953fba9fdb14e6b92867d5d5b00483228f2517",
	},
	{
		Name:    "AnnotatedNestedContainer",
		Payload: AnnotatedNestedContainer_Payload,
		Specs:   map[string]any{},
		Hash:    "984701b6584a109df60dc555cc22d000b724f85c3391c915ef362be9898b4b54",
	},
	{
		Name:    "TopLevelStructWrapper",
		Payload: TopLevelStructWrapper_Payload,
		Specs:   map[string]any{},
		Hash:    "0dbb6d5f4c46a2b34546ba81531263e908ea2ead013bfc0d4117f94d68ee1691",
	},
}

func TestCodegenGeneration(t *testing.T) {
	for i := range testMatrix {
		payload := &testMatrix[i]
		t.Run(payload.Name, func(t *testing.T) {
			testCodegenPayload(t, payload)
		})
	}
}

func TestCodegenExtendedTypes(t *testing.T) {
	payloads := []struct {
		name    string
		payload ExtendedTypes1
	}{
		{"WithOptionals", ExtendedTypes1_Payload1},
		{"NilOptionals", ExtendedTypes1_Payload2},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			testCodegenPayloadByReflection(t, tc.payload, nil, dynssz.WithExtendedTypes())
		})
	}

	t.Run("OptionalCycle", func(t *testing.T) {
		testCodegenPayloadByReflection(t, RecursiveOptNode_Payload, nil, dynssz.WithExtendedTypes())
	})
}

// TestCodegenBigIntGolden pins the generated big.Int hash tree root against
// hardcoded golden values. The other codegen tests for extended types are purely
// differential (codegen vs reflection), so a simultaneous change to both engines
// would otherwise go unnoticed; these golden roots catch it. ExtendedTypes1 has a
// value big.Int field, CoverageTypes2 a pointer *big.Int.
func TestCodegenBigIntGolden(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		golden  string
	}{
		{"valueBigInt", ExtendedTypes1_Payload1, "29e70bed46a1d689e21122f9a425c56cf63c05c72c307488a0626205a1b5a412"},
		{"pointerBigInt", CoverageTypes2_Payload1, "9f187208f2264c56c945c495d2b170c40e9469de661fea65d07a6aa990824fff"},
	}

	ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := ds.HashTreeRoot(tc.payload)
			if err != nil {
				t.Fatalf("HashTreeRoot: %v", err)
			}
			if got := hex.EncodeToString(root[:]); got != tc.golden {
				t.Fatalf("codegen big.Int root changed: got %s want %s", got, tc.golden)
			}
		})
	}
}

func TestCodegenCoverageTypes1(t *testing.T) {
	testCodegenPayloadByReflection(t, CoverageTypes1_Payload, SimpleTypesWithSpecs_Specs)
}

// assertRequiresDelegation verifies a payload that is only expressible through
// its generated Dynamic* methods — it carries a structurally illegal innard
// ([0]uint64) guarded by an ssz-static annotation, so the type must be
// shallow-built and delegated rather than traversed. The delegating engine must
// handle it end to end, while disabling delegation (WithNoDelegation) must make
// the reflection engine reject it: forcing a full traversal reaches the illegal
// innard, proving the type genuinely depends on delegation and is never silently
// walked.
func assertRequiresDelegation(t *testing.T, payload any) {
	t.Helper()

	genDs := dynssz.NewDynSsz(nil)
	enc, err := genDs.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("delegating MarshalSSZ failed: %v", err)
	}
	root, err := genDs.HashTreeRoot(payload)
	if err != nil {
		t.Fatalf("delegating HashTreeRoot failed: %v", err)
	}
	roundtrip := reflect.New(reflect.TypeOf(payload)).Interface()
	if err = genDs.UnmarshalSSZ(roundtrip, enc); err != nil {
		t.Fatalf("delegating UnmarshalSSZ failed: %v", err)
	}
	rtRoot, err := genDs.HashTreeRoot(roundtrip)
	if err != nil {
		t.Fatalf("delegating HashTreeRoot (roundtrip) failed: %v", err)
	}
	if root != rtRoot {
		t.Fatalf("round-trip changed the hash tree root: %x != %x", root, rtRoot)
	}

	refDs := dynssz.NewDynSsz(nil, dynssz.WithNoDelegation())
	if _, err := refDs.MarshalSSZ(payload); err == nil {
		t.Fatal("WithNoDelegation MarshalSSZ unexpectedly succeeded; the type must be un-reflectable and require delegation")
	}
}

// TestCodegenNestedDelegated exercises the shallow-build gate. The containers
// reference fully-delegated types carrying a structurally illegal innard
// ([0]uint64). They only generate (parser gate) and delegate (reflection
// typecache gate) because both shallow-build the nested type via its ssz-static
// annotation instead of traversing — and rejecting — that innard. The Static case
// uses ssz-static:"true" (fixed, runtime delegated size); Dynamic uses "false".
func TestCodegenNestedDelegated(t *testing.T) {
	t.Run("Static", func(t *testing.T) {
		assertRequiresDelegation(t, NestedDelegatedContainer_Payload)
	})
	t.Run("Dynamic", func(t *testing.T) {
		assertRequiresDelegation(t, NestedDelegatedDynContainer_Payload)
	})
}

// TestCodegenRecursiveTypes compares the generated code against the reflection
// engine for recursive shapes: self-recursion through a bounded list, and a
// cycle closed through a container field with a spec-dependent limit inside
// the cycle.
func TestCodegenRecursiveTypes(t *testing.T) {
	t.Run("SelfRecursion", func(t *testing.T) {
		testCodegenPayloadByReflection(t, RecursiveNode_Payload, nil)
	})
	t.Run("ContainerClosedCycle", func(t *testing.T) {
		testCodegenPayloadByReflection(t, RecursiveTree_Payload, RecursiveTree_Specs)
	})
	t.Run("ContainerClosedCycleDefaultSpecs", func(t *testing.T) {
		testCodegenPayloadByReflection(t, RecursiveTree_Payload, nil)
	})
	t.Run("InlineMemberCycle", func(t *testing.T) {
		testCodegenPayloadByReflection(t, RecursiveInlineHolder_Payload, nil)
	})
}

// TestCodegenAnnotatedTypes tests root-level annotated non-struct types
// and containers that use annotated types as fields.
func TestCodegenAnnotatedTypes(t *testing.T) {
	// Root-level annotated lists
	t.Run("AnnotatedList", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedList{1, 2, 3, 4, 5}, nil)
	})
	t.Run("AnnotatedList2", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedList2{100, 200, 300}, nil)
	})
	t.Run("AnnotatedByteList", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedByteList{0xaa, 0xbb, 0xcc}, nil)
	})

	// Annotated type with dynamic specs as root
	t.Run("AnnotatedWithSpecs", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedWithSpecs{1, 2, 3}, AnnotatedSpecs)
	})

	// Container with annotated fields (no field tag overrides)
	t.Run("AnnotatedContainer", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedContainer_Payload, nil)
	})

	// Container where field tag overrides the type annotation
	t.Run("AnnotatedOverrideContainer", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedOverrideContainer_Payload, nil)
	})

	// Container with dynamic-spec annotated field
	t.Run("AnnotatedSpecsContainer", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedSpecsContainer_Payload, AnnotatedSpecs)
	})

	// Nested containers with annotated types at multiple levels
	t.Run("AnnotatedNestedContainer", func(t *testing.T) {
		testCodegenPayloadByReflection(t, AnnotatedNestedContainer_Payload, nil)
	})
}

func TestCodegenCoverageTypes2(t *testing.T) {
	payloads := []struct {
		name    string
		payload CoverageTypes2
	}{
		{"WithValues", CoverageTypes2_Payload1},
		{"NilPointers", CoverageTypes2_Payload2},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			testCodegenPayloadByReflection(t, tc.payload, nil, dynssz.WithExtendedTypes())
		})
	}
}

func TestCodegenCoverageTypes3(t *testing.T) {
	testCodegenPayloadByReflection(t, CoverageTypes3_Payload, CoverageTypes3_Specs, dynssz.WithExtendedTypes())
}

func TestCodegenCoverageTypes4(t *testing.T) {
	testCodegenPayloadByReflection(t, CoverageTypes4_Payload, nil, dynssz.WithExtendedTypes())
}

func TestCodegenCoverageTypes5(t *testing.T) {
	testCodegenPayloadByReflection(t, CoverageTypes5_Payload, nil, dynssz.WithExtendedTypes())
}

func TestCodegenCoverageTypes6(t *testing.T) {
	testCodegenPayloadByReflection(t, CoverageTypes6_Payload, nil, dynssz.WithExtendedTypes())
}

func TestCodegenCoverageTypes7(t *testing.T) {
	testCodegenPayloadByReflection(t, CoverageTypes7_Payload, CoverageTypes7_Specs)
}

func TestCodegenNoDynExprTypes(t *testing.T) {
	testCodegenPayloadByReflection(t, NoDynExprTypes_Payload, nil)
}

// TestCodegenNoDynNest enforces the without-dynamic-expressions invariant on a
// set of parents nesting a generated child, generated with -with-streaming
// -without-fastssz -without-dynamic-expressions (gen_nodynnest.yaml). The
// generated file (compiled as part of this package) must contain no *Dyn buffer
// function and must round-trip against reflection for every parent shape.
func TestCodegenNoDynNest(t *testing.T) {
	code, err := os.ReadFile("gen_nodynnest.go")
	if os.IsNotExist(err) {
		// The gen_*.go files are gitignored; without go generate this job
		// exercises only the reflection engine and there is no generated file
		// to enforce the no-*Dyn invariant against.
		t.Skip("no generated code present")
	}
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	for _, tok := range []string{"MarshalSSZDyn", "UnmarshalSSZDyn", "SizeSSZDyn", "HashTreeRootWithDyn"} {
		if strings.Contains(string(code), tok) {
			t.Errorf("generated file references forbidden %s under without-dynamic-expressions", tok)
		}
	}

	testCodegenPayloadByReflection(t, NoDynRecursiveHolder_Payload, nil)
	testCodegenPayloadByReflection(t, NoDynNestProg_Payload, nil)
	testCodegenPayloadByReflection(t, NoDynNestList_Payload, nil)
	testCodegenPayloadByReflection(t, NoDynNestVec_Payload, nil)
	testCodegenPayloadByReflection(t, NoDynNestField_Payload, nil)
	testCodegenPayloadByReflection(t, NoDynNestChild{A: []byte{1, 2}, B: 9}, nil)
}

// TestCodegenAtkNest hammers the without-dynamic-expressions static/inlining
// path on richer shapes (gen_atknest.yaml, generated with -with-streaming
// -without-fastssz -without-dynamic-expressions -with-extended-types): deep
// containers-of-containers, union/wrapper/optional nesting of generated children,
// and an external well-behaved delegated type inlined from its structure. The
// generated file must contain no *Dyn buffer call (the buffer AND streaming
// paths), must round-trip byte/size/root-identical to reflection for every shape,
// and the inlined delegated region must equal what the delegated Dynamic* method
// produces.
func TestCodegenAtkNest(t *testing.T) {
	code, err := os.ReadFile("gen_atknest.go")
	if os.IsNotExist(err) {
		// The gen_*.go files are gitignored; the "without generated code" CI job
		// runs before go generate, so there is no generated file to enforce the
		// no-*Dyn invariant against here.
		t.Skip("no generated code present")
	}
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	for _, tok := range []string{"MarshalSSZDyn", "UnmarshalSSZDyn", "SizeSSZDyn", "HashTreeRootWithDyn"} {
		if strings.Contains(string(code), tok) {
			t.Errorf("generated file references forbidden %s under without-dynamic-expressions", tok)
		}
	}

	ext := dynssz.WithExtendedTypes()
	testCodegenPayloadByReflection(t, AtkNestD1_Payload, nil, ext)
	testCodegenPayloadByReflection(t, AtkNestUnion_Payload, nil, ext)
	testCodegenPayloadByReflection(t, AtkNestWrapper_Payload, nil, ext)
	testCodegenPayloadByReflection(t, AtkNestOpt_Payload, nil, ext)
	testCodegenPayloadByReflection(t, AtkNestOptList_Payload, nil, ext)
	testCodegenPayloadByReflection(t, AtkWellHolder_Payload, nil, ext)

	// The inlined static encoding of the external delegated child must be
	// byte-identical to what its own Dynamic* method produces. AtkWellHolder is
	// A(uint64) N(child) V([2]child); marshal it and check each 8-byte child
	// region equals child.MarshalSSZDyn.
	genDs := dynssz.NewDynSsz(nil, ext)
	got, err := genDs.MarshalSSZ(AtkWellHolder_Payload)
	if err != nil {
		t.Fatalf("marshal AtkWellHolder: %v", err)
	}
	if len(got) != 32 {
		t.Fatalf("AtkWellHolder expected 32 bytes, got %d", len(got))
	}
	children := []atkWellDelegated{AtkWellHolder_Payload.N, AtkWellHolder_Payload.V[0], AtkWellHolder_Payload.V[1]}
	for i, child := range children {
		want, err := child.MarshalSSZDyn(nil, nil)
		if err != nil {
			t.Fatalf("child.MarshalSSZDyn: %v", err)
		}
		region := got[8+i*8 : 8+i*8+8]
		if !bytes.Equal(region, want) {
			t.Errorf("inlined child region %d = %x, delegated method = %x", i, region, want)
		}
	}
}

// A dynssz expression that resolves to 0 must fall back to the static value in
// both engines. Previously the generated code applied the literal
// 0 limit and rejected the value while reflection fell back, diverging.
func TestCodegenResolvedZeroFallsBackToStatic(t *testing.T) {
	// dynssz-max ANNOTATED_MAX resolves to 0 -> both fall back to ssz-max:"10".
	testCodegenPayloadByReflection(t, AnnotatedWithSpecs{1, 2, 3}, map[string]any{"ANNOTATED_MAX": 0})
}

// When the dynssz expression resolves to 0 and the only static value is the
// placeholder 0 (no positive fallback), both engines must error rather than
// silently encode a zero-capacity list.
func TestCodegenZeroStaticMaxResolvesToZeroErrors(t *testing.T) {
	payload := AnnotatedZeroStaticMax{1, 2, 3}

	// A positive resolved value works in both engines and they agree.
	testCodegenPayloadByReflection(t, payload, map[string]any{"ZEROSTATIC_MAX": 8})

	// Resolving to 0 leaves no positive fallback: the reflection descriptor build
	// and the generated code must both error.
	zeroSpecs := map[string]any{"ZEROSTATIC_MAX": 0}
	refDs := dynssz.NewDynSsz(zeroSpecs, dynssz.WithNoFastSsz(), dynssz.WithNoFastHash())
	if _, err := refDs.MarshalSSZ(payload); err == nil {
		t.Error("reflection: expected error for max resolving to 0 with no positive static fallback")
	}
	genDs := dynssz.NewDynSsz(zeroSpecs)
	if _, err := genDs.MarshalSSZ(&payload); err == nil {
		t.Error("codegen: expected error for max resolving to 0 with no positive static fallback")
	}
}

// The same dead end reached the other way: the spec value is not defined at all
// rather than defined as 0. A placeholder ssz-max:"0" says the limit comes from
// the spec, so an absent key leaves nothing to encode or hash against, and both
// engines have to say so -- for an empty list as much as a full one, since it is
// the type that is unknowable, not the value.
func TestCodegenZeroStaticMaxUndefinedErrors(t *testing.T) {
	if _, generated := any(&AnnotatedZeroStaticMax{}).(sszutils.DynamicHashRoot); !generated {
		t.Skip("no generated code present")
	}

	noSpecs := map[string]any{} // ZEROSTATIC_MAX is not defined
	refDs := dynssz.NewDynSsz(noSpecs, dynssz.WithNoFastSsz(), dynssz.WithNoFastHash(), dynssz.WithNoDelegation())
	genDs := dynssz.NewDynSsz(noSpecs)

	for _, payload := range []AnnotatedZeroStaticMax{{}, {1, 2, 3}} {
		p := payload
		if _, err := refDs.HashTreeRoot(&p); !errors.Is(err, sszutils.ErrInvalidConstraint) {
			t.Errorf("reflection(%d elements): err = %v, want ErrInvalidConstraint", len(p), err)
		}
		if _, err := genDs.HashTreeRoot(&p); !errors.Is(err, sszutils.ErrInvalidConstraint) {
			t.Errorf("codegen(%d elements): err = %v, want ErrInvalidConstraint", len(p), err)
		}
	}

	// A positive static value is a real fallback, so an undefined spec key just
	// leaves it in place.
	testCodegenPayloadByReflection(t, AnnotatedWithSpecs{1, 2, 3}, noSpecs)
}

// Refusing an undefined limit must not take away the ways to say "unbounded":
// an ssz-max:"0" with no expression, and no tag at all, both still hash under
// extended types.
func TestCodegenLimitlessRemainsExpressible(t *testing.T) {
	if _, generated := any(&ZeroMaxList{}).(sszutils.DynamicHashRoot); !generated {
		t.Skip("no generated code present")
	}

	gen := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	refl := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes(), dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())

	for _, tc := range []struct {
		name    string
		payload any
	}{
		{"zero max, no expression", &ZeroMaxList_Payload},
		{"no tag", &UnboundedList_Payload},
		{"no tag, bitlist", &UnboundedBitlist_Payload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			genRoot, err := gen.HashTreeRoot(tc.payload)
			if err != nil {
				t.Fatalf("generated: %v", err)
			}
			reflRoot, err := refl.HashTreeRoot(tc.payload)
			if err != nil {
				t.Fatalf("reflection: %v", err)
			}
			if genRoot != reflRoot {
				t.Errorf("generated root %x differs from reflection %x", genRoot, reflRoot)
			}
		})
	}
}

// A multi-dimensional fixed vector whose inner size is dynssz-resolved must pad
// missing outer rows with the resolved inner byte size, not the static fallback
// . Previously codegen padded with the baked static inner size,
// producing a shorter encoding than reflection.
func TestCodegenMultiDimVectorPadding(t *testing.T) {
	testCodegenPayloadByReflection(t, SpecMatrix_Payload, SpecMatrix_Specs)
}

// The generated buffer unmarshal must reject trailing data after a fixed-size
// container, matching the streaming decoder and reflection paths.
func TestCodegenRejectsTrailingData(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	valid, err := ds.MarshalSSZ(&ProgIndexOnly_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	trailing := append(append([]byte{}, valid...), 0xde, 0xad)

	// Buffer path must reject the trailing bytes (previously accepted them).
	var a ProgIndexOnly
	if err := ds.UnmarshalSSZ(&a, trailing); err == nil {
		t.Error("UnmarshalSSZ accepted trailing data after a fixed container")
	}
	// Streaming path must reject them too, consistently.
	var b ProgIndexOnly
	if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(trailing), len(trailing)); err == nil {
		t.Error("UnmarshalSSZReader accepted trailing data after a fixed container")
	}
	// The exact-length buffer must still decode.
	var c ProgIndexOnly
	if err := ds.UnmarshalSSZ(&c, valid); err != nil {
		t.Errorf("UnmarshalSSZ rejected the valid buffer: %v", err)
	}
}

// Lists of variable-length elements absorb extra bytes as additional valid
// elements rather than rejecting them as trailing; codegen and reflection must
// agree on this.
func TestCodegenListOfListRoundtrip(t *testing.T) {
	testCodegenPayloadByReflection(t, ListOfList_Payload, nil)
	testCodegenPayloadByReflection(t, Bytes2D_Payload, nil)
}

// A nested list of variable-size elements whose inner list declares a first
// offset of 0 over a non-empty region claims zero items and would silently drop
// the remaining bytes. Both engines must reject the unconsumed trailing data.
func TestCodegenNestedListTrailingRejected(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	// Bytes2D = [][]byte: container offset 4, then a 7-byte inner region whose
	// first offset is 0 (zero items) followed by 3 stray bytes.
	in := []byte{0x04, 0, 0, 0, 0, 0, 0, 0, 0x55, 0xff, 0x05}

	var a Bytes2D
	if err := ds.UnmarshalSSZ(&a, in); err == nil {
		t.Error("buffer UnmarshalSSZ accepted malformed nested-list encoding")
	}
	var b Bytes2D
	if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(in), len(in)); err == nil {
		t.Error("stream UnmarshalSSZReader accepted malformed nested-list encoding")
	}
}

// A fixed-size vector of variable-size elements must validate its inner offset
// table the same way a dynamic list does: a first offset that does not point
// past the table leaves bytes unconsumed and must be rejected by both engines.
func TestCodegenVecOfListOffsetRejected(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	// VecOfList = [3][]uint16: container offset 4, then a 3-entry inner offset
	// table whose first offset is 0 instead of the required 12.
	in, err := hex.DecodeString("04000000000000000c0000001800000072a4cf1b0000b61e15ac428d16b9ed914edd477c")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var a VecOfList
	if err := ds.UnmarshalSSZ(&a, in); err == nil {
		t.Error("buffer UnmarshalSSZ accepted malformed vector inner offset table")
	}
	var b VecOfList
	if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(in), len(in)); err == nil {
		t.Error("stream UnmarshalSSZReader accepted malformed vector inner offset table")
	}
}

// A variable-size container ending in an optional field must also reject
// trailing data in both the buffer and streaming paths.
func TestCodegenRejectsTrailingDataVariable(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())

	// Round-trip consistency for the present and absent optional cases.
	testCodegenPayloadByReflection(t, OptU32_Payload, nil, dynssz.WithExtendedTypes())
	testCodegenPayloadByReflection(t, OptU32{Pre: 9}, nil, dynssz.WithExtendedTypes())

	valid, err := ds.MarshalSSZ(&OptU32_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	trailing := append(append([]byte{}, valid...), 0xde, 0xad)

	var a OptU32
	if err := ds.UnmarshalSSZ(&a, trailing); err == nil {
		t.Error("UnmarshalSSZ accepted trailing data after a variable container")
	}
	var b OptU32
	if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(trailing), len(trailing)); err == nil {
		t.Error("UnmarshalSSZReader accepted trailing data after a variable container")
	}
	var c OptU32
	if err := ds.UnmarshalSSZ(&c, valid); err != nil {
		t.Errorf("UnmarshalSSZ rejected the valid buffer: %v", err)
	}
}

// A bare top-level fixed-size vector receives the raw outer buffer, so its
// generated decoder must reject trailing bytes itself (the reflection path
// rejects them via its full-consumption check).
func TestCodegenTopLevelVectorTrailingRejected(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	oversized := make([]byte, 40)
	for i := range oversized {
		oversized[i] = byte(i + 1)
	}

	var v SimpleByteVec32
	if err := ds.UnmarshalSSZ(&v, oversized); err == nil {
		t.Error("byte vector accepted trailing data")
	}
	if err := ds.UnmarshalSSZ(&v, oversized[:30]); err == nil {
		t.Error("byte vector accepted a truncated buffer")
	}
	if err := ds.UnmarshalSSZ(&v, oversized[:32]); err != nil {
		t.Errorf("byte vector rejected the valid buffer: %v", err)
	}

	var u SimpleUint64Vec4
	if err := ds.UnmarshalSSZ(&u, oversized); err == nil {
		t.Error("uint64 vector accepted trailing data")
	}
	if err := ds.UnmarshalSSZ(&u, oversized[:30]); err == nil {
		t.Error("uint64 vector accepted a truncated buffer")
	}
	if err := ds.UnmarshalSSZ(&u, oversized[:32]); err != nil {
		t.Errorf("uint64 vector rejected the valid buffer: %v", err)
	}
}

// A fixed-size union variant occupies exactly 1+elemSize bytes of the union
// region; extra bytes in the region are trailing data and must be rejected
// like the reflection and streaming paths do.
func TestCodegenUnionVariantTrailingRejected(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())

	val := CoverageTypes4_Payload
	valid, err := ds.MarshalSSZ(&val)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// CoverageTypes4 has 5 dynamic fields (offsets at 0,4,8,12,16). Insert a
	// stray byte at the end of U1's region (variant 0 = uint32, fixed-size)
	// and shift the later offsets accordingly.
	off := make([]int, 5)
	for i := range off {
		off[i] = int(binary.LittleEndian.Uint32(valid[i*4 : i*4+4]))
	}
	tampered := make([]byte, 0, len(valid)+1)
	tampered = append(tampered, valid[:off[1]]...)
	tampered = append(tampered, 0xaa)
	tampered = append(tampered, valid[off[1]:]...)
	for i := 1; i < 5; i++ {
		binary.LittleEndian.PutUint32(tampered[i*4:i*4+4], uint32(off[i]+1))
	}

	var a CoverageTypes4
	if err := ds.UnmarshalSSZ(&a, tampered); err == nil {
		t.Error("UnmarshalSSZ accepted trailing data in a fixed-size union variant region")
	}
	var c CoverageTypes4
	if err := ds.UnmarshalSSZ(&c, valid); err != nil {
		t.Errorf("UnmarshalSSZ rejected the valid buffer: %v", err)
	}
}

// A present optional-list value of fixed size occupies exactly the inner
// type's size. Truncated regions previously decoded zero-padded garbage or
// panicked with an out-of-range index; oversized regions silently dropped the
// extra bytes.
func TestCodegenOptionalListRegionSize(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	// OptionalListTypes: 2 dynamic fields -> 8-byte offset table. StaticOpt is
	// *uint32, so a present region must be exactly 4 bytes.
	short := []byte{
		0x08, 0, 0, 0, // offset StaticOpt = 8
		0x0b, 0, 0, 0, // offset DynamicOpt = 11 (region = 3 bytes)
		0x01, 0x02, 0x03,
	}
	var a OptionalListTypes
	if err := ds.UnmarshalSSZ(&a, short); err == nil {
		t.Error("UnmarshalSSZ accepted a truncated optional-list region")
	}

	long := []byte{
		0x08, 0, 0, 0,
		0x0d, 0, 0, 0, // region = 5 bytes
		0x01, 0x02, 0x03, 0x04, 0xaa,
	}
	var b OptionalListTypes
	if err := ds.UnmarshalSSZ(&b, long); err == nil {
		t.Error("UnmarshalSSZ accepted trailing data in an optional-list region")
	}

	valid := []byte{
		0x08, 0, 0, 0,
		0x0c, 0, 0, 0, // region = 4 bytes
		0x01, 0x02, 0x03, 0x04,
	}
	var c OptionalListTypes
	if err := ds.UnmarshalSSZ(&c, valid); err != nil {
		t.Errorf("UnmarshalSSZ rejected the valid buffer: %v", err)
	} else if c.StaticOpt == nil || *c.StaticOpt != 0x04030201 {
		t.Errorf("unexpected decoded value: %v", c.StaticOpt)
	}
}

// TestCodegenOptionalListTypes verifies generated code for ssz-type:"optional-list"
// matches the reflection implementation. Optional-list expresses a pointer as
// a canonical SSZ List[T, 1] and works without extended types.
func TestCodegenOptionalListTypes(t *testing.T) {
	payloads := []struct {
		name    string
		payload OptionalListTypes
	}{
		{"BothSet", OptionalListTypes_Payload1},
		{"BothNil", OptionalListTypes_Payload2},
		{"StaticOnly", OptionalListTypes_Payload3},
	}
	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			testCodegenPayloadByReflection(t, tc.payload, nil)
		})
	}
}

// TestCodegenOptionalListSliceVector is a regression test for two independent
// bugs in ssz-type:"optional-list" over a pointer to a slice made fixed-length
// via ssz-size (i.e. an inner vector, e.g. *[]uint16 ssz-size:"2"):
//
//  1. buildOptionalListDescriptor peeled the leading ssz-size/ssz-max dimension
//     before descending into the element (sizeHints[1:] / maxSizeHints[1:]),
//     dropping the element's size constraint so the inner vector degraded to a
//     variable list. That produced a wrong (12-byte) serialization and a wrong
//     root on BOTH engines — the golden checks below guard it, since a
//     reflection-vs-codegen comparison alone would not (both engines share the
//     descriptor and would agree on the wrong encoding).
//  2. the generated optional-list HTR reused the `vlen` local for its mixin
//     length, which the fixed-vector element's own `vlen := len(...)` shadowed,
//     so a present element mixed in a length of 0 — a codegen-only wrong root
//     caught by the reflection-vs-codegen comparison.
//
// Golden roots and serializations are cross-checked against remerkleable
// (List[Vector[T,N], 1]).
func TestCodegenOptionalListSliceVector(t *testing.T) {
	cases := []struct {
		name    string
		payload OptionalListSliceVector
		root    string
		ser     string
	}{
		{"BothSet", OptionalListSliceVector_Payload1, "7e7694aa13e4558ea4e91c3aaef6319a0409a80ad8ea79df3f6a2fd03d8dee92", "080000000c00000034127856aabbccdd"},
		{"BothNil", OptionalListSliceVector_Payload2, "db56114e00fdd4c1f85c892bf35ac9a89289aaecb1ebd0a96cde606a748b5d71", "0800000008000000"},
		{"U16Only", OptionalListSliceVector_Payload3, "53f263191c776497f3a21a93ec0d767753533479ce3a7e5847496c63c7baa905", "080000000c00000034127856"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reflection-vs-codegen agreement (marshal, size, HTR, roundtrip,
			// streaming) — catches the codegen-only HTR shadowing bug once the
			// generated methods exist.
			testCodegenPayloadByReflection(t, tc.payload, nil)

			// Absolute golden values — catch a descriptor regression that would
			// make both engines agree on the wrong (variable-list) encoding.
			ds := dynssz.NewDynSsz(nil)
			root, err := ds.HashTreeRoot(tc.payload)
			if err != nil {
				t.Fatalf("HashTreeRoot: %v", err)
			}
			if got := hex.EncodeToString(root[:]); got != tc.root {
				t.Fatalf("optional-list slice-vector root changed: got %s want %s", got, tc.root)
			}
			ser, err := ds.MarshalSSZ(tc.payload)
			if err != nil {
				t.Fatalf("MarshalSSZ: %v", err)
			}
			if got := hex.EncodeToString(ser); got != tc.ser {
				t.Fatalf("optional-list slice-vector serialization changed: got %s want %s", got, tc.ser)
			}
		})
	}
}

// TestCodegenUnionExprVariantSize is a regression test for the generated union
// size code: a union variant whose size is fully determined by a dynssz-size
// expression never reads the type-asserted variant value, which previously left
// `v` declared-and-unused so the generated size method failed to compile. This
// test only exercises the compile fix once the generated methods exist (via
// go generate); the reflection comparison still validates the sizing itself.
func TestCodegenUnionExprVariantSize(t *testing.T) {
	testCodegenPayloadByReflection(t, UnionExprVariantSize_Payload, UnionExprVariantSize_Specs, dynssz.WithExtendedTypes())
}

// TestCodegenUnionExprVariantSizeCrossPreset is a regression test for the
// generated buffer decoder of a union whose fixed-size variant length comes
// from a dynssz-size expression. The decoder baked the variant's byte length to
// the static ssz-size fallback (here 4 uint16 = 8 bytes) instead of the runtime
// resolved size, so any preset whose resolved size differed from the static
// value made the generated UnmarshalSSZ reject valid encodings with an
// "incorrect offset: N bytes trailing data" (resolved > static) or an
// unexpected-EOF (resolved < static). The static-equal case (matching the
// existing UnionExprVariantSize test) accidentally masked it. Both engines must
// round-trip identically for resolved sizes above and below the static one.
func TestCodegenUnionExprVariantSizeCrossPreset(t *testing.T) {
	mk := func(n int) UnionExprVariantSize {
		data := make([]uint16, n)
		for i := range data {
			data[i] = uint16(i + 1)
		}
		return UnionExprVariantSize{
			U: dynssz.CompatibleUnion[struct {
				F0 []uint16 `ssz-size:"4" dynssz-size:"UNION_VEC_SIZE"`
			}]{Variant: 1, Data: data},
		}
	}
	for _, tc := range []struct {
		name string
		size uint64
	}{
		{"resolved_gt_static", 6},
		{"resolved_lt_static", 2},
		{"resolved_eq_static", 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testCodegenPayloadByReflection(t, mk(int(tc.size)),
				map[string]any{"UNION_VEC_SIZE": tc.size}, dynssz.WithExtendedTypes())
		})
	}
}

// TestCodegenVecDynElemExprSize is a regression test for the generated stream
// decoder of a vector of dynamic-size elements whose length is a dynssz-size
// expression. The first-offset check compared a uint32 offset against a typed
// int length expression, which failed to compile without a uint32 cast. Only
// meaningful once the generated methods exist (via go generate); the reflection
// comparison still validates the decoding.
func TestCodegenVecDynElemExprSize(t *testing.T) {
	testCodegenPayloadByReflection(t, VecDynElemExprSize_Payload, VecDynElemExprSize_Specs)
}

// TestCodegenViewTypes2 tests nested view dispatch: a container whose child
// has view dispatch methods. This exercises the isView code paths in all
// generators (marshal, unmarshal, encoder, decoder, size, hash).
func TestCodegenViewTypes2(t *testing.T) {
	t.Run("View1", func(t *testing.T) {
		testCodegenPayloadWithView(t, ViewTypes2_Payload, (*ViewTypes2_View1)(nil))
	})
	t.Run("View2", func(t *testing.T) {
		testCodegenPayloadWithView(t, ViewTypes2_Payload, (*ViewTypes2_View2)(nil))
	})
}

// TestCodegenViewTypes3 tests view-only generation: the type only has
// view dispatch methods and no data methods.
func TestCodegenViewTypes3(t *testing.T) {
	testCodegenPayloadWithView(t, ViewTypes3_Payload, (*ViewTypes3_View1)(nil))
}

// TestCodegenViewTypes4 tests cross-command view detection, union views,
// and type wrapper views.
func TestCodegenViewTypes4(t *testing.T) {
	testCodegenPayloadWithView(t, ViewTypes4_Payload, (*ViewTypes4_View1)(nil))
}

// testCodegenPayloadWithView tests a payload serialized through a view.
// It marshals via the view, unmarshals, and verifies roundtrip hash consistency.
func testCodegenPayloadWithView(t *testing.T, payload, view any) {
	t.Helper()
	ds := dynssz.NewDynSsz(nil)
	opts := []dynssz.CallOption{dynssz.WithViewDescriptor(view)}

	// Hash
	hashRoot, err := ds.HashTreeRoot(payload, opts...)
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	// Marshal
	sszBytes, err := ds.MarshalSSZ(payload, opts...)
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Unmarshal roundtrip
	obj := &struct{ Data any }{}
	reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(payload)))
	err = ds.UnmarshalSSZ(obj.Data, sszBytes, opts...)
	if err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}

	// Verify roundtrip hash
	roundtripHash, err := ds.HashTreeRoot(obj.Data, opts...)
	if err != nil {
		t.Fatalf("roundtrip HashTreeRoot failed: %v", err)
	}
	if roundtripHash != hashRoot {
		t.Fatalf("roundtrip hash mismatch: expected=%x got=%x", hashRoot, roundtripHash)
	}

	// Streaming marshal
	var streamBuf bytes.Buffer
	err = ds.MarshalSSZWriter(payload, &streamBuf, opts...)
	if err != nil {
		t.Fatalf("MarshalSSZWriter failed: %v", err)
	}
	if !bytes.Equal(streamBuf.Bytes(), sszBytes) {
		t.Fatalf("streaming marshal mismatch:\n  buf=%x\n  stream=%x", sszBytes, streamBuf.Bytes())
	}

	// Streaming unmarshal
	reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(payload)))
	err = ds.UnmarshalSSZReader(obj.Data, bytes.NewReader(sszBytes), len(sszBytes), opts...)
	if err != nil {
		t.Fatalf("UnmarshalSSZReader failed: %v", err)
	}
	streamHash, err := ds.HashTreeRoot(obj.Data, opts...)
	if err != nil {
		t.Fatalf("stream roundtrip HashTreeRoot failed: %v", err)
	}
	if streamHash != hashRoot {
		t.Fatalf("stream roundtrip hash mismatch: expected=%x got=%x", hashRoot, streamHash)
	}
}

// testCodegenPayloadByReflection compares generated code output against
// reflection-based implementation. No pre-computed hash needed.
func testCodegenPayloadByReflection(t *testing.T, payload any, specs map[string]any, opts ...dynssz.DynSszOption) {
	t.Helper()

	// WithNoDelegation forces refDs through the reflection engine even for types
	// that implement the generated Dynamic* methods. Without it a fully-delegating
	// type would run its own generated code on both sides, turning this into a
	// codegen-vs-codegen comparison instead of reflection-vs-codegen.
	refOpts := append([]dynssz.DynSszOption{
		dynssz.WithNoFastSsz(),
		dynssz.WithNoFastHash(),
		dynssz.WithNoDelegation(),
	}, opts...)
	refDs := dynssz.NewDynSsz(specs, refOpts...)
	genDs := dynssz.NewDynSsz(specs, opts...)

	// Compare hash tree root
	refHash, err := refDs.HashTreeRoot(payload)
	if err != nil {
		t.Fatalf("reflection HashTreeRoot failed: %v", err)
	}
	genHash, err := genDs.HashTreeRoot(payload)
	if err != nil {
		t.Fatalf("generated HashTreeRoot failed: %v", err)
	}
	if refHash != genHash {
		t.Fatalf("HashTreeRoot mismatch: ref=%x gen=%x", refHash, genHash)
	}

	// Compare size
	refSize, err := refDs.SizeSSZ(payload)
	if err != nil {
		t.Fatalf("reflection SizeSSZ failed: %v", err)
	}
	genSize, err := genDs.SizeSSZ(payload)
	if err != nil {
		t.Fatalf("generated SizeSSZ failed: %v", err)
	}
	if refSize != genSize {
		t.Fatalf("SizeSSZ mismatch: ref=%d gen=%d", refSize, genSize)
	}

	// Compare marshal
	refBytes, err := refDs.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("reflection MarshalSSZ failed: %v", err)
	}
	genBytes, err := genDs.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("generated MarshalSSZ failed: %v", err)
	}
	if !bytes.Equal(refBytes, genBytes) {
		t.Fatalf("MarshalSSZ mismatch:\n  ref=%x\n  gen=%x", refBytes, genBytes)
	}

	// Unmarshal roundtrip
	unmarshaled := reflect.New(reflect.TypeOf(payload)).Interface()
	err = genDs.UnmarshalSSZ(unmarshaled, genBytes)
	if err != nil {
		t.Fatalf("generated UnmarshalSSZ failed: %v", err)
	}
	roundtripHash, err := genDs.HashTreeRoot(unmarshaled)
	if err != nil {
		t.Fatalf("roundtrip HashTreeRoot failed: %v", err)
	}
	if roundtripHash != genHash {
		t.Fatalf("roundtrip hash mismatch: expected=%x got=%x", genHash, roundtripHash)
	}

	// Streaming marshal
	var streamBuf bytes.Buffer
	err = genDs.MarshalSSZWriter(payload, &streamBuf)
	if err != nil {
		t.Fatalf("MarshalSSZWriter failed: %v", err)
	}
	if !bytes.Equal(streamBuf.Bytes(), genBytes) {
		t.Fatalf("streaming marshal mismatch:\n  ref=%x\n  gen=%x", streamBuf.Bytes(), genBytes)
	}

	// Streaming unmarshal
	streamUnmarshaled := reflect.New(reflect.TypeOf(payload)).Interface()
	err = genDs.UnmarshalSSZReader(streamUnmarshaled, bytes.NewReader(genBytes), len(genBytes))
	if err != nil {
		t.Fatalf("UnmarshalSSZReader failed: %v", err)
	}
	streamHash, err := genDs.HashTreeRoot(streamUnmarshaled)
	if err != nil {
		t.Fatalf("stream roundtrip HashTreeRoot failed: %v", err)
	}
	if streamHash != genHash {
		t.Fatalf("stream roundtrip hash mismatch: expected=%x got=%x", genHash, streamHash)
	}
}

func testCodegenPayload(t *testing.T, payload *TestPayload) {
	t.Helper()

	// The generated methods for an extended-types payload are emitted by the
	// extended batch, so they hash it whatever this DynSsz says. Reflection --
	// which is what runs before the generated code exists -- needs to be told.
	dsOpts := []dynssz.DynSszOption{}
	if payload.Extended {
		dsOpts = append(dsOpts, dynssz.WithExtendedTypes())
	}
	ds := dynssz.NewDynSsz(payload.Specs, dsOpts...)

	opts := []dynssz.CallOption{}
	if payload.View != nil {
		opts = append(opts, dynssz.WithViewDescriptor(payload.View))
	}

	hashRoot, err := ds.HashTreeRoot(payload.Payload, opts...)
	if err != nil {
		t.Fatalf("Failed to hash tree root: %v", err)
	}
	hashRootHex := hex.EncodeToString(hashRoot[:])
	if hashRootHex != payload.Hash {
		t.Fatalf("Hash root mismatch 1: expected %s, got %s", payload.Hash, hashRootHex)
	}

	sszBytes, err := ds.MarshalSSZ(payload.Payload, opts...)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	obj := &struct {
		Data any
	}{}
	reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(payload.Payload)))

	err = ds.UnmarshalSSZ(obj.Data, sszBytes, opts...)
	if err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	hashRoot, err = ds.HashTreeRoot(obj.Data, opts...)
	if err != nil {
		t.Fatalf("Failed to hash tree root: %v", err)
	}
	hashRootHex = hex.EncodeToString(hashRoot[:])
	if hashRootHex != payload.Hash {
		t.Fatalf("Hash root mismatch 2: expected %s, got %s", payload.Hash, hashRootHex)
	}

	memBuf := make([]byte, 0, len(sszBytes))
	memWriter := bytes.NewBuffer(memBuf)
	err = ds.MarshalSSZWriter(payload.Payload, memWriter, opts...)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}
	memBuf = memWriter.Bytes()
	if !bytes.Equal(memBuf, sszBytes) {
		t.Fatalf("MarshalSSZWriter mismatch: expected %x, got %x", sszBytes, memBuf)
	}

	reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(payload.Payload)))

	err = ds.UnmarshalSSZReader(obj.Data, bytes.NewReader(sszBytes), len(sszBytes), opts...)
	if err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	hashRoot, err = ds.HashTreeRoot(obj.Data, opts...)
	if err != nil {
		t.Fatalf("Failed to hash tree root: %v", err)
	}
	hashRootHex = hex.EncodeToString(hashRoot[:])
	if hashRootHex != payload.Hash {
		t.Fatalf("Hash root mismatch 2: expected %s, got %s", payload.Hash, hashRootHex)
	}
}

// An annotated FIXED-size type used as a container field must be embedded
// inline (static, no offset), matching the reflection layout.
func TestCodegenAnnotatedFixedContainer(t *testing.T) {
	testCodegenPayloadByReflection(t, AnnotatedFixedContainer_Payload, nil)

	ds := dynssz.NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&AnnotatedFixedContainer_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// 4-byte uint32 + 8-byte inline vector, no offset table
	if len(enc) != 12 {
		t.Errorf("expected 12-byte inline encoding, got %d bytes: %x", len(enc), enc)
	}
}

// SizeSSZ must match the marshaled length for a multi-dim vector with spec
// expressions even when the value is fully empty (missing rows are padded).
func TestCodegenMultiDimSpecVec(t *testing.T) {
	for _, specs := range []map[string]any{nil, {"SPEC_OUTER": uint64(2), "SPEC_INNER": uint64(4)}} {
		testCodegenPayloadByReflection(t, MultiDimSpecVec{M: [][]byte{}}, specs)
		testCodegenPayloadByReflection(t, MultiDimSpecVec{M: [][]byte{{1, 2, 3, 4}}}, specs)
		testCodegenPayloadByReflection(t, MultiDimSpecVec{M: [][]byte{{1, 2, 3, 4}, {5, 6, 7, 8}}}, specs)
	}
}

// A fixed-size vector of variable-size elements whose length is a dynssz
// expression must generate compiling streaming decoder code (the first-offset
// check compares a uint32 offset against limit*4, where limit is int(expr)).
// The package building at all is the compile regression guard; the differential
// confirms the value round-trips identically in both engines.
func TestCodegenStreamVecDynSize(t *testing.T) {
	for _, specs := range []map[string]any{nil, StreamVecDynSize_Specs} {
		testCodegenPayloadByReflection(t, StreamVecDynSize_Payload, specs)
		testCodegenPayloadByReflection(t, StreamVecDynSize{V: []StreamVecElem{}}, specs)
	}
}

// A compatible-union whose variant is an inline container sized purely by a size
// expression must still generate compiling SizeSSZ code: the asserted variant
// value would otherwise be declared-and-unused. The package compiling is the
// regression guard; the differential confirms sizing agreement.
func TestCodegenSizeUnionExprVariant(t *testing.T) {
	testCodegenPayloadByReflection(t, SizeUnionExprVariant_Payload, SizeUnionExprVariant_Specs)
}

// A bitlist without ssz-max must produce the same root in both engines
// (length mixin with a limit derived from the serialized bit length).
func TestCodegenNoMaxBitlist(t *testing.T) {
	testCodegenPayloadByReflection(t, NoMaxBitlist{B1: []byte{0x01}}, nil)
	testCodegenPayloadByReflection(t, NoMaxBitlist{B1: []byte{0xff, 0x03}}, nil)
}

// Named *Bitlist* types must be classified as bitlists by the codegen parser
// like the reflection typecache heuristic does.
func TestCodegenNamedBitlist(t *testing.T) {
	testCodegenPayloadByReflection(t, NamedBitlistContainer{B: NamedBitlistT{0xff, 0x03}}, nil)

	ds := dynssz.NewDynSsz(nil)

	// missing termination bit
	var a NamedBitlistContainer
	if err := ds.UnmarshalSSZ(&a, []byte{0x04, 0, 0, 0, 0x00}); err == nil {
		t.Error("accepted bitlist without termination bit")
	}

	// 399 bits exceeds the 100-bit limit
	big := make([]byte, 50)
	big[49] = 0x80
	var b NamedBitlistContainer
	if err := ds.UnmarshalSSZ(&b, append([]byte{0x04, 0, 0, 0}, big...)); err == nil {
		t.Error("accepted bitlist exceeding its bit limit")
	}
}

// Bitvector edge cases: empty and short values marshal zero-padded without
// panicking, and byte-aligned runtime bit sizes accept fully-populated
// last bytes. Invalid padding bits must still be rejected.
func TestCodegenBitvectorEdgeCases(t *testing.T) {
	specs := map[string]any{"BIT_SPEC": uint64(16)}
	payloads := []BitvecEdge{
		{BV1: []byte{}, BV2: []byte{}, BV3: []byte{}},
		{BV1: []byte{0xff}, BV2: []byte{0xff}, BV3: []byte{0xff}},
		{BV1: []byte{0xff, 0xff}, BV2: []byte{0xff, 0xff}, BV3: []byte{0xff, 0x0f}},
	}
	for _, p := range payloads {
		testCodegenPayloadByReflection(t, p, nil)
		testCodegenPayloadByReflection(t, p, specs)
	}

	// round-trip through the generated buffer and stream decoders
	ds := dynssz.NewDynSsz(specs)
	full := BitvecEdge{BV1: []byte{0xff, 0xff}, BV2: []byte{0xff, 0xff}, BV3: []byte{0xff, 0x0f}}
	enc, err := ds.MarshalSSZ(&full)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back BitvecEdge
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Errorf("buffer unmarshal rejected a valid encoding: %v", err)
	}
	var back2 BitvecEdge
	if err := ds.UnmarshalSSZReader(&back2, bytes.NewReader(enc), len(enc)); err != nil {
		t.Errorf("stream unmarshal rejected a valid encoding: %v", err)
	}

	// non-zero padding bits in the 12-bit vector must be rejected by both engines
	invalid := BitvecEdge{BV1: []byte{0xff, 0xff}, BV2: []byte{0xff, 0xff}, BV3: []byte{0xff, 0xff}}
	if _, err := ds.MarshalSSZ(&invalid); err == nil {
		t.Error("generated marshal accepted non-zero padding bits")
	}
	refDs := dynssz.NewDynSsz(specs, dynssz.WithNoFastSsz(), dynssz.WithNoFastHash())
	if _, err := refDs.MarshalSSZ(invalid); err == nil {
		t.Error("reflection marshal accepted non-zero padding bits")
	}
}

// A non-empty list-of-dynamic region whose first offset is 0 must be rejected
// by the streaming decoder just like the buffer decoder and reflection.
func TestCodegenStreamZeroFirstOffsetRejected(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	// Bytes2D: container offset table (4) + inner region with first offset 0.
	in := []byte{0x04, 0, 0, 0, 0x00, 0x00, 0x00, 0x00}

	var a Bytes2D
	if err := ds.UnmarshalSSZ(&a, in); err == nil {
		t.Error("buffer UnmarshalSSZ accepted a zero first offset in a non-empty region")
	}
	var b Bytes2D
	if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(in), len(in)); err == nil {
		t.Error("stream UnmarshalSSZReader accepted a zero first offset in a non-empty region")
	}
}

// A truncated union region must produce a clean error on the streaming path:
// the decoder must never read across the region limit (which previously led
// to bogus negative-trailing errors or out-of-range panics).
func TestCodegenStreamTruncatedUnionRegion(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())

	val := CoverageTypes4_Payload
	valid, err := ds.MarshalSSZ(&val)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// 5 dynamic fields, offsets at 0,4,8,12,16. Truncate U1's region to k
	// bytes and shift the later offsets accordingly.
	off := make([]int, 5)
	for i := range off {
		off[i] = int(binary.LittleEndian.Uint32(valid[i*4 : i*4+4]))
	}
	u1len := off[1] - off[0]

	for k := 0; k < u1len; k++ {
		in := make([]byte, 0, len(valid))
		in = append(in, valid[:off[0]+k]...)
		in = append(in, valid[off[1]:]...)
		for i := 1; i < 5; i++ {
			binary.LittleEndian.PutUint32(in[i*4:i*4+4], uint32(off[i]-(u1len-k)))
		}

		var a CoverageTypes4
		if err := ds.UnmarshalSSZ(&a, in); err == nil {
			t.Errorf("buffer UnmarshalSSZ accepted a %d-byte union region", k)
		}
		var b CoverageTypes4
		if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(in), len(in)); err == nil {
			t.Errorf("stream UnmarshalSSZReader accepted a %d-byte union region", k)
		}
	}
}

// Delegated static fields around a dynamic field must each be sized from
// their own type; two shallow delegated descriptors previously shared one
// size variable, making the decoder reject its own marshal output.
func TestCodegenMixedDelegatedContainer(t *testing.T) {
	assertRequiresDelegation(t, MixedDelegatedContainer_Payload)

	ds := dynssz.NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&MixedDelegatedContainer_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back MixedDelegatedContainer
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Errorf("generated decoder rejected its own marshal output: %v", err)
	}
	if back.D.Value != 8 || back.B[2].Value != 4 {
		t.Errorf("unexpected decoded values: %+v", back)
	}
}

// A truncated union region followed by another dynamic field: the stream
// decoder must not read the selector from the next region (previously the
// overrun produced a negative remaining length and an out-of-range panic
// for variable-size variants).
func TestCodegenStreamUnionDynVariantTruncated(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	testCodegenPayloadByReflection(t, UnionDynVariant_Payload, nil)

	valid, err := ds.MarshalSSZ(&UnionDynVariant_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// 2 dynamic fields, offsets at 0 and 4. Shrink U's region to 0 bytes; the
	// next region starts with 0x01, which would select the variable-size
	// variant if the selector were read across the region boundary.
	off0 := int(binary.LittleEndian.Uint32(valid[0:4]))
	off1 := int(binary.LittleEndian.Uint32(valid[4:8]))
	in := make([]byte, 0, len(valid))
	in = append(in, valid[:off0]...)
	in = append(in, valid[off1:]...)
	binary.LittleEndian.PutUint32(in[4:8], uint32(off0))

	var a UnionDynVariant
	if err := ds.UnmarshalSSZ(&a, in); err == nil {
		t.Error("buffer UnmarshalSSZ accepted an empty union region")
	}
	var b UnionDynVariant
	if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(in), len(in)); err == nil {
		t.Error("stream UnmarshalSSZReader accepted an empty union region")
	}
}

// Explicit selector values assigned via ssz-index tags on union variant
// fields must be honored identically by codegen and reflection.
func TestCodegenUnionTaggedSelectors(t *testing.T) {
	testCodegenPayloadByReflection(t, UnionTaggedSelectors_Payload, nil)

	ds := dynssz.NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&UnionTaggedSelectors_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// offset (4) + selector byte 1 + uint32 payload
	if len(enc) != 9 || enc[4] != 1 {
		t.Fatalf("unexpected encoding: %x", enc)
	}

	// selector 0 is not assigned and must be rejected by both paths
	bad := append([]byte{}, enc...)
	bad[4] = 0
	var a UnionTaggedSelectors
	if err := ds.UnmarshalSSZ(&a, bad); err == nil {
		t.Error("buffer UnmarshalSSZ accepted unassigned selector 0")
	}
	var b UnionTaggedSelectors
	if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(bad), len(bad)); err == nil {
		t.Error("stream UnmarshalSSZReader accepted unassigned selector 0")
	}
}

// Classic union wire semantics through the generated code: the None option
// round-trips as the bare selector byte, trailing bytes after it and
// out-of-range selectors are rejected, and a truncated union region fails
// cleanly on both decode paths.
func TestCodegenClassicUnionWireFormat(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	testCodegenPayloadByReflection(t, ClassicUnionDynVariant_Payload, nil)

	// The zero-valued union is the None option: its region is the bare
	// selector byte 0. Layout: offsets (8) + U region + L region.
	none := ClassicUnionDynVariant{L: []byte{7}}
	enc, err := ds.MarshalSSZ(&none)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(enc, []byte{8, 0, 0, 0, 9, 0, 0, 0, 0, 7}) {
		t.Fatalf("unexpected None encoding: %x", enc)
	}

	var back ClassicUnionDynVariant
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("buffer unmarshal: %v", err)
	}
	if back.U.Variant != 0 || back.U.Data != nil || !bytes.Equal(back.L, []byte{7}) {
		t.Fatalf("buffer None roundtrip mismatch: %+v", back)
	}
	var back2 ClassicUnionDynVariant
	if err := ds.UnmarshalSSZReader(&back2, bytes.NewReader(enc), len(enc)); err != nil {
		t.Fatalf("stream unmarshal: %v", err)
	}
	if back2.U.Variant != 0 || back2.U.Data != nil {
		t.Fatalf("stream None roundtrip mismatch: %+v", back2)
	}

	reject := func(name string, in []byte) {
		t.Run(name, func(t *testing.T) {
			var a ClassicUnionDynVariant
			if err := ds.UnmarshalSSZ(&a, in); err == nil {
				t.Errorf("buffer UnmarshalSSZ accepted %x", in)
			}
			var b ClassicUnionDynVariant
			if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(in), len(in)); err == nil {
				t.Errorf("stream UnmarshalSSZReader accepted %x", in)
			}
		})
	}

	// The None region must be exactly the selector byte.
	reject("trailingAfterNone", []byte{8, 0, 0, 0, 10, 0, 0, 0, 0, 99, 7})
	// A selector without a declared variant is rejected.
	reject("outOfRangeSelector", []byte{8, 0, 0, 0, 9, 0, 0, 0, 9, 7})
	// An empty union region has no selector byte; the stream decoder must not
	// read it from the next region.
	reject("emptyUnionRegion", []byte{8, 0, 0, 0, 8, 0, 0, 0, 7})
}

// A generated decoder must allocate a classic union's pointer variant before
// writing through it; decoding valid bytes must round-trip rather than
// nil-deref.
func TestCodegenClassicUnionPtrVariant(t *testing.T) {
	testCodegenPayloadByReflection(t, ClassicUnionPtrVariant_Payload, nil)

	ds := dynssz.NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&ClassicUnionPtrVariant_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var buf ClassicUnionPtrVariant
	if err := ds.UnmarshalSSZ(&buf, enc); err != nil {
		t.Fatalf("buffer unmarshal: %v", err)
	}
	uv, ok := buf.U.Data.(*uint64)
	if !ok || uv == nil || *uv != ptrUnionVal {
		t.Fatalf("buffer roundtrip mismatch: %+v", buf)
	}

	var strm ClassicUnionPtrVariant
	if err := ds.UnmarshalSSZReader(&strm, bytes.NewReader(enc), len(enc)); err != nil {
		t.Fatalf("stream unmarshal: %v", err)
	}
	sv, ok := strm.U.Data.(*uint64)
	if !ok || sv == nil || *sv != ptrUnionVal {
		t.Fatalf("stream roundtrip mismatch: %+v", strm)
	}
}

// Top-level standalone named composite types: every generated method receives
// the pointer receiver directly and must dereference it correctly on all
// paths (marshal/unmarshal/size/hash, buffer and stream).
func TestCodegenTopLevelCompositeTypes(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	roundtrip := func(name string, val any, mkEmpty func() any) {
		t.Run(name, func(t *testing.T) {
			testCodegenPayloadByReflection(t, reflect.ValueOf(val).Elem().Interface(), nil)

			enc, err := ds.MarshalSSZ(val)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			back := mkEmpty()
			if err := ds.UnmarshalSSZ(back, enc); err != nil {
				t.Fatalf("buffer unmarshal: %v", err)
			}
			if !reflect.DeepEqual(back, val) {
				t.Fatalf("buffer roundtrip mismatch: %v != %v", back, val)
			}
			back2 := mkEmpty()
			if err := ds.UnmarshalSSZReader(back2, bytes.NewReader(enc), len(enc)); err != nil {
				t.Fatalf("stream unmarshal: %v", err)
			}
			if !reflect.DeepEqual(back2, val) {
				t.Fatalf("stream roundtrip mismatch: %v != %v", back2, val)
			}
		})
	}

	str := TopLevelString("hello world")
	wrap := TopLevelWrapVarList{}
	wrap.V.Data = []OptionalListTypes_Inner{{Tag: 1, Data: []byte{9, 8}}}

	roundtrip("Bitlist", &TopLevelBitlist{0xff, 0x03}, func() any { return &TopLevelBitlist{} })
	roundtrip("ProgBitlist", &TopLevelProgBitlist{0xff, 0x03}, func() any { return &TopLevelProgBitlist{} })
	roundtrip("String", &str, func() any { return new(TopLevelString) })
	roundtrip("CtrList", &TopLevelCtrList{{F1: 1}, {F1: 2}}, func() any { return &TopLevelCtrList{} })
	roundtrip("CtrVec", &TopLevelCtrVec{{F1: 1}, {F1: 2}, {F1: 3}, {F1: 4}}, func() any { return &TopLevelCtrVec{} })
	roundtrip("VarList", &TopLevelVarList{{Tag: 1, Data: []byte{1, 2}}, {Tag: 2, Data: []byte{}}}, func() any { return &TopLevelVarList{} })
	roundtrip("ListOfList", &TopLevelListOfList{{1, 2, 3}, {}, {4}}, func() any { return &TopLevelListOfList{} })
	roundtrip("WrapVarList", &wrap, func() any { return &TopLevelWrapVarList{} })
}

// The generated stream decoder must validate a list's first offset before
// allocating the offset table sized from it, so a tiny payload with a huge
// first offset is rejected with bounded allocation rather than allocating an
// offset table for the attacker-supplied item count.
func TestCodegenStreamListOffsetNoOverAlloc(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	// Bytes2D: outer container offset (4) to field B, then B's inner list whose
	// first offset is 0x02000000 (itemCount ~= 8.4M) — but the region is tiny.
	in := []byte{0x04, 0, 0, 0, 0x00, 0x00, 0x00, 0x02}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	var v Bytes2D
	err := ds.UnmarshalSSZReader(&v, bytes.NewReader(in), len(in))
	if err == nil {
		t.Fatal("expected the malicious first offset to be rejected")
	}

	runtime.ReadMemStats(&after)
	if delta := after.TotalAlloc - before.TotalAlloc; delta > 1<<20 {
		t.Errorf("stream decode allocated %d bytes for an 8-byte input (offset not validated before allocation)", delta)
	}
}

// A generated decoder must allocate a pointer union variant / wrapper-of-pointer
// before writing through it; decoding valid bytes must round-trip rather than
// nil-deref.
func TestCodegenPointerUnionVariantAndWrapper(t *testing.T) {
	testCodegenPayloadByReflection(t, PtrUnionVariant_Payload, nil)

	ds := dynssz.NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&PtrUnionVariant_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var buf PtrUnionVariant
	if err := ds.UnmarshalSSZ(&buf, enc); err != nil {
		t.Fatalf("buffer unmarshal: %v", err)
	}
	uv, ok := buf.U.Data.(*uint64)
	if !ok || uv == nil || *uv != ptrUnionVal || buf.W.Data == nil || *buf.W.Data != ptrUnionVal {
		t.Fatalf("buffer roundtrip mismatch: %+v", buf)
	}

	var strm PtrUnionVariant
	if err := ds.UnmarshalSSZReader(&strm, bytes.NewReader(enc), len(enc)); err != nil {
		t.Fatalf("stream unmarshal: %v", err)
	}
	if strm.W.Data == nil || *strm.W.Data != ptrUnionVal {
		t.Fatalf("stream roundtrip mismatch: %+v", strm)
	}
}

// Codegen compile-correctness shapes: each must generate compilable code that
// round-trips and matches the reflection engine (buffer + stream). These
// exercise pointer-receiver dereferencing, localized value naming, the
// pointer-element bulk fast-path guard, and zero-padding item typing.
func TestCodegenPointerAndPaddingShapes(t *testing.T) {
	u1, u2 := uint64(1), uint64(2)
	s := "ab"
	l := []uint16{7, 8}
	bl := [][]byte{{0x03}, {0x05}}

	uv := UnionSamePkgVariant{}
	uv.U.Variant = 2
	uv.U.Data = SimpleTypes1_C1{F1: 9}

	cuv := ClassicUnionSamePkgVariant{}
	cuv.U.Variant = 1
	cuv.U.Data = SimpleTypes1_C1{F1: 9}

	wu := WrapUnionField{}
	wu.W.Data.Variant = 1
	wu.W.Data.Data = uint32(42)

	wcu := WrapClassicUnionField{}
	wcu.W.Data.Variant = 1
	wcu.W.Data.Data = uint32(42)

	cases := []struct {
		name string
		val  any
	}{
		{"TopVecOfVar", &TopVecOfVar{{1, 2}, {3}, {}}},
		{"TopVecOfVar-underfill", &TopVecOfVar{{1, 2}}},
		{"UnionSamePkgVariant", &uv},
		{"ClassicUnionSamePkgVariant", &cuv},
		{"PtrPrimitiveList", &PtrPrimitiveList{F: []*uint64{&u1, &u2}}},
		{"FixedVecPtrStr", &FixedVecPtrStr{F: []*string{&s, &s}}},
		{"FixedVecPtrStr-underfill", &FixedVecPtrStr{F: []*string{&s}}},
		{"FixedVecPtrList", &FixedVecPtrList{F: []*[]uint16{&l, &l}}},
		{"FixedVecStr-underfill", &FixedVecStr{F: []string{"a"}}},
		{"PtrDynCollectionField", &PtrDynCollectionField{F: &bl}},
		{"WrapUnionField", &wu},
		{"WrapClassicUnionField", &wcu},
		{"ShortLargeUintVec", &ShortLargeUintVec{A: []byte{1, 2, 3}, B: []uint64{7}}},
		{"ShortLargeUintVec-full", &ShortLargeUintVec{A: make([]byte, 16), B: []uint64{1, 2, 3, 4}}},
		{"PtrSvecOfList", &PtrSvecOfList{F: &[][]uint16{{1, 2}}}},
		{"WrapPtrList", func() any { w := WrapPtrList{}; d := []uint16{5, 6}; w.W.Data = &d; return &w }()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testCodegenPayloadByReflection(t, reflect.ValueOf(tc.val).Elem().Interface(), nil)
		})
	}
}

// namedPtrType is a defined type whose underlying type is a pointer; methods
// cannot be declared on it, so top-level generation must error cleanly.
type namedPtrType *uint64

// Top-level CompatibleUnion / TypeWrapper and named pointer types cannot
// receive generated methods; the generator must reject them with a clear
// error instead of emitting uncompilable code.
func TestCodegenRejectsUngeneratableTopLevelTypes(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{
			name: "named pointer type",
			typ:  reflect.TypeFor[namedPtrType](),
			want: "named pointer type",
		},
		{
			name: "top-level union",
			typ: reflect.TypeFor[dynssz.CompatibleUnion[struct {
				A uint32
				B []byte `ssz-max:"8"`
			}]](),
			want: "CompatibleUnion/TypeWrapper",
		},
		{
			name: "top-level classic union",
			typ: reflect.TypeFor[dynssz.Union[struct {
				A uint32
				B []byte `ssz-max:"8"`
			}]](),
			want: "Union/CompatibleUnion/TypeWrapper",
		},
		{
			name: "top-level wrapper",
			typ: reflect.TypeFor[dynssz.TypeWrapper[struct {
				Data []uint16 `ssz-max:"6"`
			}, []uint16]](),
			want: "CompatibleUnion/TypeWrapper",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cg := codegen.NewCodeGenerator(nil)
			cg.BuildFile("reject_test.go", codegen.WithReflectType(tc.typ))
			_, err := cg.GenerateToMap()
			if err == nil {
				t.Fatalf("expected a rejection error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// A field tagged ssz-type:"-" is excluded from the SSZ layout: the encoding,
// size and root match the same struct without that field, and it round-trips
// without being restored. Non-SSZ types (maps) are allowed on excluded fields.
func TestCodegenExcludedFields(t *testing.T) {
	// The generated code and reflection must agree on the excluded layout.
	testCodegenPayloadByReflection(t, ExcludedFields_Payload, nil)

	ds := dynssz.NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&ExcludedFields_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Encoding must equal a struct with only the included fields.
	type included struct {
		A uint32
		B uint64
		L []uint16 `ssz-max:"8"`
	}
	ref, err := ds.MarshalSSZ(&included{A: 1, B: 2, L: []uint16{3, 4}})
	if err != nil {
		t.Fatalf("ref marshal: %v", err)
	}
	if !bytes.Equal(enc, ref) {
		t.Fatalf("excluded encoding mismatch:\n got=%x\n want=%x", enc, ref)
	}

	// Round-trip must not restore the excluded fields.
	var back ExcludedFields
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Cache != [32]byte{} || back.Meta != nil {
		t.Errorf("excluded fields were populated on decode: %+v", back)
	}
	if back.A != 1 || back.B != 2 || len(back.L) != 2 {
		t.Errorf("included fields wrong after roundtrip: %+v", back)
	}
}

// The generated sizer cannot return an error, so an unknown selector reports
// size 0 (matching its mismatched-data convention) instead of a
// plausible-looking partial size; the marshalers reject the value outright.
// Without generated code (this suite also runs before go generate), the
// reflection sizer serves the call and reports the invalid selector as an
// error instead.
func TestCodegenUnionInvalidSelectorSize(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	v := UnionDynVariant{}
	v.U.Variant = 99
	v.U.Data = uint32(1)

	size, err := ds.SizeSSZ(&v)
	if _, hasGeneratedCode := any(&v).(sszutils.DynamicSizer); hasGeneratedCode {
		if err != nil {
			t.Fatalf("size: %v", err)
		}
		// The whole value reports size 0: an un-sizable union aborts the
		// sizer, matching the mismatched-data convention.
		if size != 0 {
			t.Errorf("expected size 0 for an un-sizable union, got %d", size)
		}
	} else if err == nil {
		t.Fatal("reflection sizing should reject the invalid selector")
	}

	if _, err := ds.MarshalSSZ(&v); err == nil {
		t.Fatal("marshal should reject the invalid selector")
	}
}

// Scalar top-level types occupy exactly their fixed size; the generated
// unmarshal must reject trailing bytes like the reflection engine does.
func TestCodegenScalarRootTrailingRejected(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	refDs := dynssz.NewDynSsz(nil, dynssz.WithNoDelegation(), dynssz.WithNoFastSsz())

	cases := []struct {
		name  string
		fresh func() any
		valid []byte
	}{
		{"bool", func() any { return new(SimpleBool) }, []byte{1}},
		{"uint8", func() any { return new(SimpleUint8) }, []byte{7}},
		{"uint64", func() any { return new(SimpleUint64) }, []byte{1, 0, 0, 0, 0, 0, 0, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trailing := append(bytes.Clone(tc.valid), 0xAA, 0xBB)

			if err := ds.UnmarshalSSZ(tc.fresh(), trailing); err == nil {
				t.Error("generated UnmarshalSSZ accepted trailing data after a scalar root")
			}
			if err := ds.UnmarshalSSZReader(tc.fresh(), bytes.NewReader(trailing), len(trailing)); err == nil {
				t.Error("generated UnmarshalSSZReader accepted trailing data after a scalar root")
			}
			if err := refDs.UnmarshalSSZ(tc.fresh(), trailing); err == nil {
				t.Error("reflection accepted trailing data after a scalar root")
			}

			if err := ds.UnmarshalSSZ(tc.fresh(), tc.valid); err != nil {
				t.Errorf("valid buffer rejected: %v", err)
			}
			if err := ds.UnmarshalSSZ(tc.fresh(), tc.valid[:len(tc.valid)-1]); err == nil {
				t.Error("generated UnmarshalSSZ accepted a short buffer")
			}
		})
	}
}

// A legal empty list of variable-size elements decodes through the generated
// streaming decoder on a seekable source; skipping the (empty) offset table
// must not move the read position backwards.
func TestCodegenDecoderEmptyDynListSeekable(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	enc, err := ds.MarshalSSZ(&ListOfList{L: [][]uint32{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var dst ListOfList
	decoder, generated := any(&dst).(sszutils.DynamicDecoder)
	if !generated {
		// Without generated code there is no streaming decoder to exercise;
		// the reflection path reads offsets directly instead of skipping.
		t.Skip("no generated code present")
	}
	dec := sszutils.NewBufferDecoder(enc)
	dec.PushLimit(len(enc))
	if err := decoder.UnmarshalSSZDecoder(ds, dec); err != nil {
		t.Fatalf("generated decoder rejected a valid empty dynamic list: %v", err)
	}
	if diff := dec.PopLimit(); diff != 0 {
		t.Errorf("decoder left %d bytes unconsumed", diff)
	}
	if len(dst.L) != 0 {
		t.Errorf("expected empty list, got %d elements", len(dst.L))
	}
}

// An unknown-size dynamic-list offset table proves its item count before it
// proves that any item body exists. A generated stream decoder must not turn a
// small, body-less table into the full backing array for a very wide Go value.
func TestCodegenUnknownSizeDynamicListWideValueAllocationIsIncremental(t *testing.T) {
	if _, generated := any(&WideDynamicList{}).(sszutils.DynamicDecoder); !generated {
		t.Skip("no generated code present")
	}

	const itemCount = 512
	offsetTable := make([]byte, itemCount*4)
	for pos := 0; pos < len(offsetTable); pos += 4 {
		binary.LittleEndian.PutUint32(offsetTable[pos:pos+4], uint32(len(offsetTable)))
	}
	payload := make([]byte, 4+len(offsetTable))
	binary.LittleEndian.PutUint32(payload[:4], 4)
	copy(payload[4:], offsetTable)

	ds := dynssz.NewDynSsz(
		nil,
		dynssz.WithStreamReaderBufferSize(8),
		dynssz.WithMaxStreamSize(128<<20),
	)
	if _, err := ds.SizeSSZ(&WideDynamicList{}); err != nil {
		t.Fatalf("warm generated methods: %v", err)
	}

	valid := WideDynamicList{Items: make([]WideDynamicElement, 2)}
	valid.Items[0].Fixed[0] = 1
	valid.Items[0].Tail = []byte{2}
	valid.Items[1].Fixed[0] = 3
	validPayload, err := ds.MarshalSSZ(&valid)
	if err != nil {
		t.Fatalf("marshal valid list: %v", err)
	}
	var roundTrip WideDynamicList
	if err := ds.UnmarshalSSZReader(&roundTrip, bytes.NewReader(validPayload), -1); err != nil {
		t.Fatalf("incrementally decode valid list: %v", err)
	}
	if len(roundTrip.Items) != 2 ||
		cap(roundTrip.Items) != 2 ||
		roundTrip.Items[0].Fixed[0] != 1 ||
		!bytes.Equal(roundTrip.Items[0].Tail, []byte{2}) ||
		roundTrip.Items[1].Fixed[0] != 3 {
		t.Fatal("incrementally grown list did not round-trip")
	}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	var out WideDynamicList
	if err := ds.UnmarshalSSZReader(&out, bytes.NewReader(payload), -1); err == nil {
		t.Fatal("offset table without element bodies decoded successfully")
	}

	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	const allocationLimit = 4 << 20
	if allocated > allocationLimit {
		t.Fatalf(
			"%d bytes of malformed input allocated %d bytes, want at most %d",
			len(payload),
			allocated,
			allocationLimit,
		)
	}
}

// The generated decoders enforce the big.Int ssz-max and canonicality rules on
// both the buffer and stream paths, matching the reflection engine.
func TestCodegenBigIntDecodeValidation(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	refDs := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes(), dynssz.WithNoDelegation(), dynssz.WithNoFastSsz())

	valid, err := ds.MarshalSSZ(&ExtendedBigIntMax_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var rt ExtendedBigIntMax
	if err := ds.UnmarshalSSZ(&rt, valid); err != nil {
		t.Fatalf("generated buffer decode of valid payload: %v", err)
	}
	if err := ds.UnmarshalSSZReader(&rt, bytes.NewReader(valid), len(valid)); err != nil {
		t.Fatalf("generated stream decode of valid payload: %v", err)
	}
	if rt.B.Cmp(&ExtendedBigIntMax_Payload.B) != 0 {
		t.Errorf("roundtrip value mismatch: %s", rt.B.String())
	}

	cases := []struct {
		name string
		enc  []byte
	}{
		{"overLimit", []byte{4, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{"emptyPayload", []byte{4, 0, 0, 0}},
		{"negativeZero", []byte{4, 0, 0, 0, 1}},
		{"leadingZero", []byte{4, 0, 0, 0, 0, 0, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a, b, c ExtendedBigIntMax
			if err := ds.UnmarshalSSZ(&a, tc.enc); err == nil {
				t.Error("generated buffer decoder accepted the payload")
			}
			if err := ds.UnmarshalSSZReader(&b, bytes.NewReader(tc.enc), len(tc.enc)); err == nil {
				t.Error("generated stream decoder accepted the payload")
			}
			if err := refDs.UnmarshalSSZ(&c, tc.enc); err == nil {
				t.Error("reflection accepted the payload")
			}
		})
	}
}

// Generated HashTreeRoot must pad short vectors into library-owned buffers:
// the caller's backing array stays untouched and the root is independent of
// whether the fields alias one array. Checked against reflection.
func TestCodegenAliasedVectorHashing(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)
	refDs := dynssz.NewDynSsz(nil, dynssz.WithNoDelegation(), dynssz.WithNoFastSsz())

	shared := make([]byte, 32)
	for i := range shared {
		shared[i] = 0xAB
	}
	before := bytes.Clone(shared)

	aliased := &AliasedVecPair{V: shared[0:2:32], W: shared[4:12:32]}
	unaliased := &AliasedVecPair{V: []byte{0xAB, 0xAB}, W: bytes.Repeat([]byte{0xAB}, 8)}

	rootAliased, err := ds.HashTreeRoot(aliased)
	if err != nil {
		t.Fatalf("HashTreeRoot aliased: %v", err)
	}
	if !bytes.Equal(shared, before) {
		t.Fatalf("generated HashTreeRoot mutated caller memory:\n before: %x\n after:  %x", before, shared)
	}

	rootUnaliased, err := ds.HashTreeRoot(unaliased)
	if err != nil {
		t.Fatalf("HashTreeRoot unaliased: %v", err)
	}
	if rootAliased != rootUnaliased {
		t.Errorf("aliasing changed the generated root: %x != %x", rootAliased, rootUnaliased)
	}

	refRoot, err := refDs.HashTreeRoot(&AliasedVecPair{V: shared[0:2:32], W: shared[4:12:32]})
	if err != nil {
		t.Fatalf("reflection HashTreeRoot: %v", err)
	}
	if refRoot != rootUnaliased {
		t.Errorf("generated root diverges from reflection: %x != %x", rootUnaliased, refRoot)
	}
	if !bytes.Equal(shared, before) {
		t.Fatalf("reflection HashTreeRoot mutated caller memory")
	}
}

// Generated SizeSSZ cannot return errors, so invalid union values size as the
// 0 sentinel — including a fixed-size variant holding mismatched data, which
// previously fabricated the declared fixed size. Reflection errors instead.
func TestCodegenUnionFixedVariantSizeValidation(t *testing.T) {
	ds := dynssz.NewDynSsz(nil)

	var v ClassicUnionPtrVariant
	v.U.Variant = 0 // V1 uint64: a fixed-size variant
	v.U.Data = "wrong"

	if sizer, generated := any(&v).(sszutils.DynamicSizer); generated {
		if got := sizer.SizeSSZDyn(ds); got != 0 {
			t.Errorf("generated SizeSSZ = %d for mismatched fixed-variant data, want 0", got)
		}
	}

	refDs := dynssz.NewDynSsz(nil, dynssz.WithNoDelegation(), dynssz.WithNoFastSsz())
	if _, err := refDs.SizeSSZ(&v); err == nil {
		t.Error("reflection should reject mismatched fixed-variant data")
	}

	// A matching value sizes identically on both engines.
	var ok ClassicUnionPtrVariant
	ok.U.Variant = 0
	ok.U.Data = uint64(7)
	genSize, err := ds.SizeSSZ(&ok)
	if err != nil {
		t.Fatalf("generated SizeSSZ: %v", err)
	}
	refSize, err := refDs.SizeSSZ(&ok)
	if err != nil {
		t.Fatalf("reflection SizeSSZ: %v", err)
	}
	if genSize != refSize {
		t.Errorf("size mismatch for valid value: generated=%d reflection=%d", genSize, refSize)
	}
}

// A dynamic-list offset table proves its item count before it proves that any
// item body exists. The unknown-size path grows incrementally (see
// TestCodegenUnknownSizeDynamicListWideValueAllocationIsIncremental); the
// buffer and known-size paths keep exact allocation, so they instead have to
// reject a count the region cannot physically hold. Without that check a
// compact table sized the full backing array for a very wide Go value: 2 KB of
// offsets materialized 33 MB of WideDynamicElement.
func TestCodegenDynamicListRejectsUnbackedElementCount(t *testing.T) {
	if _, generated := any(&WideDynamicList{}).(sszutils.DynamicUnmarshaler); !generated {
		t.Skip("no generated code present")
	}

	ds := dynssz.NewDynSsz(nil)
	if _, err := ds.SizeSSZ(&WideDynamicList{}); err != nil {
		t.Fatalf("warm generated methods: %v", err)
	}

	// 512 offsets, each pointing past the table: 512 declared elements, no
	// bodies. Each element's fixed section is 64 KiB + 4, so the region cannot
	// hold even one of them.
	const itemCount = 512
	offsetTable := make([]byte, itemCount*4)
	for pos := 0; pos < len(offsetTable); pos += 4 {
		binary.LittleEndian.PutUint32(offsetTable[pos:pos+4], uint32(len(offsetTable)))
	}
	payload := make([]byte, 4+len(offsetTable))
	binary.LittleEndian.PutUint32(payload[:4], 4)
	copy(payload[4:], offsetTable)

	// A two-element list that really does carry its bodies must still decode,
	// so the bound cannot be conservative.
	valid := WideDynamicList{Items: make([]WideDynamicElement, 2)}
	valid.Items[0].Fixed[0] = 1
	valid.Items[0].Tail = []byte{2}
	valid.Items[1].Fixed[0] = 3
	validPayload, err := ds.MarshalSSZ(&valid)
	if err != nil {
		t.Fatalf("marshal valid list: %v", err)
	}

	decodes := []struct {
		mode string
		fn   func(any, []byte) error
	}{
		{"buffer", func(v any, b []byte) error { return ds.UnmarshalSSZ(v, b) }},
		{"known-size", func(v any, b []byte) error {
			return ds.UnmarshalSSZReader(v, bytes.NewReader(b), len(b))
		}},
	}

	for _, d := range decodes {
		t.Run(d.mode, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)

			if err := d.fn(new(WideDynamicList), payload); err == nil {
				t.Fatal("offset table without element bodies decoded successfully")
			}

			runtime.ReadMemStats(&after)
			const allocationLimit = 4 << 20
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > allocationLimit {
				t.Fatalf(
					"%d bytes of malformed input allocated %d bytes, want at most %d",
					len(payload), allocated, allocationLimit,
				)
			}

			var roundTrip WideDynamicList
			if err := d.fn(&roundTrip, validPayload); err != nil {
				t.Fatalf("well-formed list rejected: %v", err)
			}
			if len(roundTrip.Items) != 2 ||
				roundTrip.Items[0].Fixed[0] != 1 ||
				!bytes.Equal(roundTrip.Items[0].Tail, []byte{2}) ||
				roundTrip.Items[1].Fixed[0] != 3 {
				t.Fatal("well-formed list did not round-trip")
			}
		})
	}

	// The generated decoders must reject exactly what the reflection engine
	// rejects; a divergence here means one engine allocates where the other
	// refuses.
	t.Run("matches_reflection", func(t *testing.T) {
		refl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
		for _, in := range [][]byte{payload, validPayload} {
			reflErr := refl.UnmarshalSSZ(new(WideDynamicList), in) != nil
			cgErr := ds.UnmarshalSSZ(new(WideDynamicList), in) != nil
			if reflErr != cgErr {
				t.Fatalf("engines disagree on a %d byte input: reflection rejected=%v, codegen rejected=%v",
					len(in), reflErr, cgErr)
			}
		}
	})
}

// A user-declared struct that carries type-wrapper semantics through a
// type-level annotation must generate methods like any other top-level entry.
//
// Wrapper semantics used to be conflated with the library's generic
// TypeWrapper/Union: those are nameable only through a transparent alias, so a
// method receiver would name the foreign generic and the generator rejects
// them. The gate keyed on the SSZ type rather than on the Go type, so it also
// rejected an ordinary named struct that can perfectly well receive methods —
// a type that generated fine before the gate was introduced.
func TestCodegenTopLevelStructWrapper(t *testing.T) {
	if _, generated := any(&TopLevelStructWrapper{}).(sszutils.DynamicMarshaler); !generated {
		t.Skip("no generated code present")
	}

	// Every generated method set must be present, not just the marshaler: a
	// partial emission would still satisfy the gate.
	for name, ok := range map[string]bool{
		"DynamicMarshaler":   func() bool { _, ok := any(&TopLevelStructWrapper{}).(sszutils.DynamicMarshaler); return ok }(),
		"DynamicUnmarshaler": func() bool { _, ok := any(&TopLevelStructWrapper{}).(sszutils.DynamicUnmarshaler); return ok }(),
		"DynamicHashRoot":    func() bool { _, ok := any(&TopLevelStructWrapper{}).(sszutils.DynamicHashRoot); return ok }(),
		"DynamicEncoder":     func() bool { _, ok := any(&TopLevelStructWrapper{}).(sszutils.DynamicEncoder); return ok }(),
		"DynamicDecoder":     func() bool { _, ok := any(&TopLevelStructWrapper{}).(sszutils.DynamicDecoder); return ok }(),
	} {
		if !ok {
			t.Errorf("generated code does not implement %s", name)
		}
	}

	// The wrapper is transparent: it must serialize and hash exactly as its
	// single field does, and the generated methods must agree with reflection.
	refl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
	cg := dynssz.NewDynSsz(nil)

	payload := TopLevelStructWrapper_Payload

	reflBytes, err := refl.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("reflection marshal: %v", err)
	}
	cgBytes, err := cg.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("generated marshal: %v", err)
	}
	if !bytes.Equal(reflBytes, cgBytes) {
		t.Fatalf("marshal mismatch:\n reflection %x\n codegen    %x", reflBytes, cgBytes)
	}

	reflRoot, err := refl.HashTreeRoot(payload)
	if err != nil {
		t.Fatalf("reflection hash tree root: %v", err)
	}
	cgRoot, err := cg.HashTreeRoot(payload)
	if err != nil {
		t.Fatalf("generated hash tree root: %v", err)
	}
	if reflRoot != cgRoot {
		t.Fatalf("root mismatch: reflection %x, codegen %x", reflRoot, cgRoot)
	}

	if reflSize, _ := refl.SizeSSZ(payload); reflSize != len(reflBytes) {
		t.Fatalf("reflection size %d != marshal length %d", reflSize, len(reflBytes))
	}
	if cgSize, _ := cg.SizeSSZ(payload); cgSize != len(cgBytes) {
		t.Fatalf("generated size %d != marshal length %d", cgSize, len(cgBytes))
	}

	// Buffer and stream decode must both round-trip, and both engines must
	// reach the same value.
	decoded := map[string]*TopLevelStructWrapper{}
	for _, e := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"reflection", refl}, {"codegen", cg}} {
		buffered := new(TopLevelStructWrapper)
		if err := e.ds.UnmarshalSSZ(buffered, reflBytes); err != nil {
			t.Fatalf("%s buffer decode: %v", e.name, err)
		}
		streamed := new(TopLevelStructWrapper)
		if err := e.ds.UnmarshalSSZReader(streamed, bytes.NewReader(reflBytes), len(reflBytes)); err != nil {
			t.Fatalf("%s stream decode: %v", e.name, err)
		}
		if !reflect.DeepEqual(buffered, streamed) {
			t.Fatalf("%s buffer and stream decode disagree", e.name)
		}
		if !reflect.DeepEqual(*buffered, payload) {
			t.Fatalf("%s did not round-trip: got %+v, want %+v", e.name, *buffered, payload)
		}
		decoded[e.name] = buffered
	}
	if !reflect.DeepEqual(decoded["reflection"], decoded["codegen"]) {
		t.Fatal("engines decoded the same bytes to different values")
	}
}

// Generated methods for a type on a recursive cycle carry a nesting depth and
// refuse to descend past the configured bound.
//
// The bound exists because stack exhaustion is fatal in Go: the runtime aborts
// the process and recover() cannot contain it, so a server could not isolate
// the failure to the request that caused it. Each level costs only a handful of
// wire bytes, so a small payload can otherwise declare nesting deep enough to
// exhaust the stack.
//
// Depth counts trips round the cycle, not nesting levels: it advances where the
// emitter delegates to a child's own methods, which is the only place the
// generated code grows the stack. Inlined children are emitted into the same
// function and add no frame, and a cycle can never be inline-only -- that is
// rejected at generation time.
func TestCodegenRecursionDepthBound(t *testing.T) {
	if _, generated := any(&RecursiveNode{}).(sszutils.DynamicUnmarshaler); !generated {
		t.Skip("no generated code present")
	}

	// RecursiveNode is Val uint64 + a Children offset, so each level costs
	// 12 bytes of fixed section plus the 4-byte offset of a one-element list.
	deepPayload := func(levels int) []byte {
		buf := make([]byte, 12)
		binary.LittleEndian.PutUint32(buf[8:12], 12)
		for range levels {
			next := make([]byte, 0, len(buf)+16)
			next = binary.LittleEndian.AppendUint64(next, 1)
			next = binary.LittleEndian.AppendUint32(next, 12)
			next = binary.LittleEndian.AppendUint32(next, 4)
			next = append(next, buf...)
			buf = next
		}
		return buf
	}

	deepValue := func(levels int) *RecursiveNode {
		node := &RecursiveNode{Val: 1}
		for range levels {
			node = &RecursiveNode{Val: 1, Children: []*RecursiveNode{node}}
		}
		return node
	}

	// Past the 1024 default, but far too shallow to exhaust a real stack: the
	// test must show the bound firing, not the process dying.
	const tooDeep = 1200

	ds := dynssz.NewDynSsz(nil)

	t.Run("decode", func(t *testing.T) {
		payload := deepPayload(tooDeep)

		t.Run("buffer", func(t *testing.T) {
			err := ds.UnmarshalSSZ(new(RecursiveNode), payload)
			if !errors.Is(err, sszutils.ErrMaxDepthExceeded) {
				t.Fatalf("err = %v, want ErrMaxDepthExceeded", err)
			}
		})

		t.Run("stream", func(t *testing.T) {
			err := ds.UnmarshalSSZReader(new(RecursiveNode), bytes.NewReader(payload), len(payload))
			if !errors.Is(err, sszutils.ErrMaxDepthExceeded) {
				t.Fatalf("err = %v, want ErrMaxDepthExceeded", err)
			}
		})
	})

	t.Run("encode", func(t *testing.T) {
		value := deepValue(tooDeep)

		if _, err := ds.MarshalSSZ(value); !errors.Is(err, sszutils.ErrMaxDepthExceeded) {
			t.Errorf("MarshalSSZ err = %v, want ErrMaxDepthExceeded", err)
		}
		if _, err := ds.HashTreeRoot(value); !errors.Is(err, sszutils.ErrMaxDepthExceeded) {
			t.Errorf("HashTreeRoot err = %v, want ErrMaxDepthExceeded", err)
		}

		var buf bytes.Buffer
		if err := ds.MarshalSSZWriter(value, &buf); !errors.Is(err, sszutils.ErrMaxDepthExceeded) {
			t.Errorf("MarshalSSZWriter err = %v, want ErrMaxDepthExceeded", err)
		}
	})

	t.Run("mutual_cycle", func(t *testing.T) {
		// Both members of a two-type cycle carry the depth; if either were
		// left out, a call through it would restart the count.
		specs := RecursiveTree_Specs
		treeDs := dynssz.NewDynSsz(specs)

		tree := &RecursiveTree{Depth: 1}
		for range tooDeep {
			tree = &RecursiveTree{Depth: 1, Branches: []RecursiveTreeBranch{{Leaf: tree}}}
		}
		if _, err := treeDs.MarshalSSZ(tree); !errors.Is(err, sszutils.ErrMaxDepthExceeded) {
			t.Fatalf("err = %v, want ErrMaxDepthExceeded", err)
		}
	})

	t.Run("engine_parity", func(t *testing.T) {
		// Both engines count one level per cycle member entered, so the first
		// rejected chain length must be identical — a value one engine emits
		// within the bound must never be refused by the other. Measured, not
		// assumed: the counts have diverged before without any test noticing.
		refl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
		depthErr := func(err error) bool { return errors.Is(err, sszutils.ErrMaxDepthExceeded) }

		firstFail := func(fails func(n int) bool) int {
			lo, hi := 0, 4096
			for lo < hi {
				mid := (lo + hi) / 2
				if fails(mid) {
					hi = mid
				} else {
					lo = mid + 1
				}
			}
			return lo
		}

		t.Run("self_cycle", func(t *testing.T) {
			r := firstFail(func(n int) bool { _, err := refl.MarshalSSZ(deepValue(n)); return depthErr(err) })
			c := firstFail(func(n int) bool { _, err := ds.MarshalSSZ(deepValue(n)); return depthErr(err) })
			if r != c {
				t.Fatalf("first rejected chain: reflection %d, codegen %d", r, c)
			}
		})

		t.Run("decode", func(t *testing.T) {
			r := firstFail(func(n int) bool { return depthErr(refl.UnmarshalSSZ(new(RecursiveNode), deepPayload(n))) })
			c := firstFail(func(n int) bool { return depthErr(ds.UnmarshalSSZ(new(RecursiveNode), deepPayload(n))) })
			if r != c {
				t.Fatalf("first rejected payload: reflection %d, codegen %d", r, c)
			}
		})

		t.Run("optional_cycle", func(t *testing.T) {
			// A cycle closing through an optional edge charges the same levels
			// in both engines, like the list edge.
			extRefl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation(), dynssz.WithExtendedTypes())
			extDs := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
			deepOpt := func(n int) *RecursiveOptNode {
				cur := &RecursiveOptNode{Val: 1}
				for range n {
					cur = &RecursiveOptNode{Val: 1, Next: cur}
				}
				return cur
			}
			r := firstFail(func(n int) bool { _, err := extRefl.MarshalSSZ(deepOpt(n)); return depthErr(err) })
			c := firstFail(func(n int) bool { _, err := extDs.MarshalSSZ(deepOpt(n)); return depthErr(err) })
			if r != c {
				t.Fatalf("first rejected chain: reflection %d, codegen %d", r, c)
			}
		})

		t.Run("inline_member_cycle", func(t *testing.T) {
			// The cycle member without generated methods is inlined into the
			// holder's code; the level it counts must be charged there too, or
			// the generated engine would accept deeper values than reflection.
			deepInline := func(n int) *RecursiveInlineHolder {
				cur := &RecursiveInlineHolder{Val: 1}
				for range n {
					cur = &RecursiveInlineHolder{Val: 1, Next: RecursiveInlineMember{Links: []*RecursiveInlineHolder{cur}}}
				}
				return cur
			}
			r := firstFail(func(n int) bool { _, err := refl.MarshalSSZ(deepInline(n)); return depthErr(err) })
			c := firstFail(func(n int) bool { _, err := ds.MarshalSSZ(deepInline(n)); return depthErr(err) })
			if r != c {
				t.Fatalf("first rejected chain: reflection %d, codegen %d", r, c)
			}
		})
	})

	t.Run("within_bound_still_works", func(t *testing.T) {
		// The bound is a limit, not a rejection of recursive types: a value
		// inside it must round-trip, and match the reflection engine.
		value := deepValue(8)

		encoded, err := ds.MarshalSSZ(value)
		if err != nil {
			t.Fatalf("marshal a value within the bound: %v", err)
		}

		decoded := new(RecursiveNode)
		if decodeErr := ds.UnmarshalSSZ(decoded, encoded); decodeErr != nil {
			t.Fatalf("decode a value within the bound: %v", decodeErr)
		}
		reencoded, err := ds.MarshalSSZ(decoded)
		if err != nil {
			t.Fatalf("re-marshal: %v", err)
		}
		if !bytes.Equal(encoded, reencoded) {
			t.Fatal("a value within the bound did not round-trip")
		}

		refl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
		reflEncoded, err := refl.MarshalSSZ(value)
		if err != nil {
			t.Fatalf("reflection marshal: %v", err)
		}
		if !bytes.Equal(encoded, reflEncoded) {
			t.Fatal("generated and reflection encodings differ")
		}

		cgRoot, err := ds.HashTreeRoot(value)
		if err != nil {
			t.Fatalf("generated root: %v", err)
		}
		reflRoot, err := refl.HashTreeRoot(value)
		if err != nil {
			t.Fatalf("reflection root: %v", err)
		}
		if cgRoot != reflRoot {
			t.Fatalf("root mismatch: generated %x, reflection %x", cgRoot, reflRoot)
		}
	})

	t.Run("only_cyclic_types_carry_a_depth", func(t *testing.T) {
		// A type that is not on a cycle keeps its plain method set, so ordinary
		// schemas pay nothing for the bound.
		source, err := os.ReadFile("gen_recursive.go")
		if err != nil {
			t.Fatalf("read generated recursive file: %v", err)
		}
		if !strings.Contains(string(source), "unmarshalSSZAtDepth") {
			t.Error("a cyclic type must carry depth-bearing methods")
		}

		// Every other generated file holds types that are not on a cycle, so
		// none of them may carry the bound. Checked by scanning rather than by
		// naming one, since which file a type lands in is only a grouping.
		others, err := filepath.Glob("gen_*.go")
		if err != nil {
			t.Fatalf("list generated files: %v", err)
		}
		checked := 0
		for _, name := range others {
			// gen_nodynnest.go and gen_extended.go carry recursive types on
			// purpose: they pin the static-only build and the optional edge
			// against cycles.
			if name == "gen_recursive.go" || name == "gen_nodynnest.go" || name == "gen_extended.go" {
				continue
			}
			plain, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			checked++
			if strings.Contains(string(plain), "AtDepth") {
				t.Errorf("%s holds no cyclic type but carries depth-bearing methods", name)
			}
		}
		if checked == 0 {
			t.Fatal("no other generated files were found to check")
		}
	})
}

// A recursive type generated with a view analyzes and carries the depth bound
// through its view methods.
//
// Building a view means building the data type against a schema type, which is
// not a cacheable build, so the parser's cycle detection has to cover the
// hinted build path as well. Without that it never recognised the cycle and the
// generator itself died with a stack overflow.
func TestCodegenRecursiveViewDepthBound(t *testing.T) {
	if _, generated := any(&RecursiveViewNode{}).(sszutils.DynamicUnmarshaler); !generated {
		t.Skip("no generated code present")
	}

	source, err := os.ReadFile("gen_recursive.go")
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	code := string(source)

	// The view method set has to carry the depth too; a view method entering at
	// zero would restart the count halfway round the cycle.
	for _, fn := range []string{
		"marshalSSZView_RecursiveViewNode_View1AtDepth",
		"unmarshalSSZView_RecursiveViewNode_View1AtDepth",
		"sizeSSZView_RecursiveViewNode_View1AtDepth",
		"hashTreeRootView_RecursiveViewNode_View1AtDepth",
	} {
		if !strings.Contains(code, fn) {
			t.Errorf("view method %s does not carry a nesting depth", fn)
		}
	}

	ds := dynssz.NewDynSsz(nil)

	t.Run("view_round_trips", func(t *testing.T) {
		payload := RecursiveViewNode_Payload

		encoded, err := ds.MarshalSSZ(&payload, dynssz.WithViewDescriptor((*RecursiveViewNode_View1)(nil)))
		if err != nil {
			t.Fatalf("marshal through the view: %v", err)
		}

		decoded := new(RecursiveViewNode)
		if err := ds.UnmarshalSSZ(decoded, encoded, dynssz.WithViewDescriptor((*RecursiveViewNode_View1)(nil))); err != nil {
			t.Fatalf("decode through the view: %v", err)
		}
		if decoded.Val != payload.Val || len(decoded.Children) != len(payload.Children) {
			t.Fatalf("view round-trip changed the value: %+v", decoded)
		}
	})

	t.Run("view_bounds_depth", func(t *testing.T) {
		// The view caps children at 2, so a chain of single children is a legal
		// value for it; nesting past the bound must error rather than abort.
		const tooDeep = 1200
		node := &RecursiveViewNode{Val: 1}
		for range tooDeep {
			node = &RecursiveViewNode{Val: 1, Children: []*RecursiveViewNode{node}}
		}

		_, err := ds.MarshalSSZ(node, dynssz.WithViewDescriptor((*RecursiveViewNode_View1)(nil)))
		if !errors.Is(err, sszutils.ErrMaxDepthExceeded) {
			t.Fatalf("err = %v, want ErrMaxDepthExceeded", err)
		}
	})
}

// A list with no declared limit mixes in its length in generated code too, so
// the two engines agree and the root identifies the value.
//
// SSZ has no root for such a list -- List[T, N] needs N to merkleize -- and it
// used to be merkleized as a vector with no mixin, which made the root blind to
// the length: values differing only by trailing zeros shared a root.
// The offset table declares how many elements follow, but only the region can
// prove their bodies exist, so a count the remaining bytes cannot cover is
// rejected before it sizes a slice. The bound is the element's fixed section,
// which here resolves from a spec value: the generator sees 12 bytes per
// element (ssz-size:"4"), the SHRUNK_SIZE preset makes it 6, and a bound baked
// in as a constant would reject the valid encoding of the smaller one.
func TestCodegenSpecShrunkElementRegionBound(t *testing.T) {
	if _, generated := any(&SpecShrunkList{}).(sszutils.DynamicUnmarshaler); !generated {
		t.Skip("no generated code present")
	}

	ds := dynssz.NewDynSsz(SpecShrunkList_Specs)

	// Four elements, each the 6-byte minimum: the region holds them exactly, so
	// this is the encoding a constant bound of 12 rejected.
	valid, err := ds.MarshalSSZ(&SpecShrunkList_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out SpecShrunkList
	if err := ds.UnmarshalSSZ(&out, valid); err != nil {
		t.Fatalf("a region holding exactly its elements must decode: %v", err)
	}
	if len(out.Items) != len(SpecShrunkList_Payload.Items) {
		t.Fatalf("decoded %d items, want %d", len(out.Items), len(SpecShrunkList_Payload.Items))
	}

	// Same declared count, region cut to 8 body bytes: 4 elements of 6 bytes do
	// not fit, and the count must be refused rather than sized into a slice.
	const declared = 4
	const tableBytes = declared * 4
	short := make([]byte, 4+tableBytes+8)
	binary.LittleEndian.PutUint32(short, 4)
	for i := range declared {
		binary.LittleEndian.PutUint32(short[4+i*4:], tableBytes)
	}

	shortErr := ds.UnmarshalSSZ(new(SpecShrunkList), short)
	if !errors.Is(shortErr, sszutils.ErrOffset) || !strings.Contains(shortErr.Error(), "elements of at least 6 bytes") {
		t.Fatalf("err = %v, want the region bound to reject 4 elements of 6 bytes", shortErr)
	}
}

// A list element that is itself a vector of dynamic entries costs one offset
// per entry plus each entry's own fixed section, and the vector's length comes
// from a spec value. Reading that length as a byte count -- it is an element
// count -- would bound the region twelve times too loosely here.
func TestCodegenSpecVecElementRegionBound(t *testing.T) {
	if _, generated := any(&SpecVecList{}).(sszutils.DynamicUnmarshaler); !generated {
		t.Skip("no generated code present")
	}

	ds := dynssz.NewDynSsz(SpecShrunkList_Specs)

	valid, err := ds.MarshalSSZ(&SpecVecList_Payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out SpecVecList
	if err = ds.UnmarshalSSZ(&out, valid); err != nil {
		t.Fatalf("the payload's own encoding must decode: %v", err)
	}
	if len(out.Items) != len(SpecVecList_Payload.Items) {
		t.Fatalf("decoded %d items, want %d", len(out.Items), len(SpecVecList_Payload.Items))
	}

	// VEC_COUNT resolves to 3, so an element is 3*(4+4) = 24 bytes at minimum.
	// Three of them need 72 bytes; the region offers 24.
	const declared = 3
	const tableBytes = declared * 4
	short := make([]byte, 4+tableBytes+24)
	binary.LittleEndian.PutUint32(short, 4)
	for i := range declared {
		binary.LittleEndian.PutUint32(short[4+i*4:], tableBytes)
	}

	err = ds.UnmarshalSSZ(new(SpecVecList), short)
	if !errors.Is(err, sszutils.ErrOffset) || !strings.Contains(err.Error(), "elements of at least 24 bytes") {
		t.Fatalf("err = %v, want the region bound to reject 3 elements of 24 bytes", err)
	}
}

// Streaming writes bytes as it produces them, so a value that fails partway
// through leaves part of its encoding on the writer. How much depends on the
// value, the buffer size, and whether the type has generated code -- the
// reflection path happens to reject this one before writing, because it sizes
// the value first. Neither is a guarantee, which is what the documentation
// says.
//
// What a caller can rely on is the buffer path: it returns the error and no
// bytes, so nothing partial can reach a peer.
func TestCodegenStreamingMayWritePartialOutput(t *testing.T) {
	if _, generated := any(&SimpleTypes1{}).(sszutils.DynamicEncoder); !generated {
		t.Skip("no generated code present")
	}

	invalid := SimpleTypes1_Payload
	invalid.Str = "far longer than its ssz-max of 8"

	for _, tc := range []struct {
		name string
		opts []dynssz.DynSszOption
	}{
		{"generated", []dynssz.DynSszOption{dynssz.WithStreamWriterBufferSize(64)}},
		{"reflection", []dynssz.DynSszOption{dynssz.WithStreamWriterBufferSize(64), dynssz.WithNoFastSsz(), dynssz.WithNoDelegation()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := dynssz.NewDynSsz(nil, tc.opts...)

			// The buffer path is all-or-nothing.
			encoded, err := ds.MarshalSSZ(&invalid)
			if err == nil {
				t.Fatal("marshal accepted an over-long string")
			}
			if len(encoded) != 0 {
				t.Errorf("the buffer path returned %d bytes alongside its error", len(encoded))
			}

			// The streaming path reports the same failure; how many bytes
			// reached the writer is not part of the contract.
			var written bytes.Buffer
			if err := ds.MarshalSSZWriter(&invalid, &written); err == nil {
				t.Fatal("streaming accepted an over-long string")
			}
			t.Logf("%d bytes reached the writer before the error", written.Len())
		})
	}
}

// SizeSSZ is exact for a value that encodes, and that is the guarantee callers
// pre-allocate against. For a value that does not encode it means nothing: the
// generated sizer returns a bare int, so it reports 0 where reflection reports
// an error. Marshaling is what rejects such a value, in both engines.
func TestCodegenSizeIsExactForEncodableValues(t *testing.T) {
	if _, generated := any(&UnionSamePkgVariant{}).(sszutils.DynamicSizer); !generated {
		t.Skip("no generated code present")
	}

	engines := []struct {
		name string
		ds   *dynssz.DynSsz
	}{
		{"generated", dynssz.NewDynSsz(nil)},
		{"reflection", dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())},
	}

	valid := UnionSamePkgVariant{}
	valid.U.Variant = 1
	valid.U.Data = uint64(42)

	for _, engine := range engines {
		t.Run("exact/"+engine.name, func(t *testing.T) {
			size, err := engine.ds.SizeSSZ(&valid)
			if err != nil {
				t.Fatalf("size: %v", err)
			}
			encoded, err := engine.ds.MarshalSSZ(&valid)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if size != len(encoded) {
				t.Errorf("size %d does not match the %d bytes encoded", size, len(encoded))
			}
		})
	}

	// A selector with no variant behind it cannot be encoded. Sizing it is not
	// what says so -- marshaling is, in both engines.
	invalid := UnionSamePkgVariant{}
	invalid.U.Variant = 99
	invalid.U.Data = uint32(1)

	for _, engine := range engines {
		t.Run("rejected at marshal/"+engine.name, func(t *testing.T) {
			if _, err := engine.ds.MarshalSSZ(&invalid); err == nil {
				t.Error("marshal accepted a value with no such variant")
			}
		})
	}
}

// without-dynamic-expressions emits buffer methods that bake the static tag
// values and take no spec set, so they are only correct for the defaults. The
// streaming methods keep resolving from the spec set, because there is no
// expression-less streaming form -- an Encoder method is always handed a
// DynSsz.
//
// The two therefore disagree by construction when the specs are not the
// defaults, and it is the entrypoint that keeps that from mattering: it must
// not serve a value from the spec-independent methods to a spec-laden DynSsz.
func TestCodegenWithoutDynamicExpressionsRouting(t *testing.T) {
	// Held through the interface: without generated code the method does not
	// exist, so naming it directly would not compile.
	sizer, generated := any(&NoDynExprTypes_Payload).(interface{ SizeSSZ() int })
	if !generated {
		t.Skip("no generated code present")
	}

	specs := map[string]any{
		"VEC8_SIZE": uint64(6), "VEC32_SIZE": uint64(4), "BITVEC_SIZE": uint64(8),
		"LST8_MAX": uint64(4), "LST32_MAX": uint64(4), "BITLST_MAX": uint64(16), "STR_MAX": uint64(8),
	}
	payload := &NoDynExprTypes_Payload

	// The generated buffer method knows only the static values.
	baked := sizer.SizeSSZ()

	refl := dynssz.NewDynSsz(specs, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
	want, err := refl.SizeSSZ(payload)
	if err != nil {
		t.Fatalf("reflection size: %v", err)
	}
	if baked == want {
		t.Skip("the chosen spec values do not differ from the static ones")
	}

	ds := dynssz.NewDynSsz(specs)
	got, err := ds.SizeSSZ(payload)
	if err != nil {
		t.Fatalf("routed size: %v", err)
	}
	if got != want {
		t.Errorf("entrypoint sized %d, want %d: it used a method that bakes the static values", got, want)
	}

	encoded, err := ds.MarshalSSZ(payload)
	if err != nil {
		t.Fatalf("routed marshal: %v", err)
	}
	if len(encoded) != want {
		t.Errorf("entrypoint encoded %d bytes, want %d", len(encoded), want)
	}
}

// Decoding reuses what the target already holds: a slice that fits keeps its
// backing array, and a non-nil pointer is decoded into rather than replaced.
// Both engines do this, in both positions -- a struct field and a slice
// element -- and the decoded value is the same either way.
func TestCodegenDecodeReusesTargetPointers(t *testing.T) {
	if _, generated := any(&SimpleTypes2{}).(sszutils.DynamicUnmarshaler); !generated {
		t.Skip("no generated code present")
	}

	source := SimpleTypes2{F1: 7, F2: []*SimpleTypes2_C1{
		{F1: []uint16{1, 2, 3, 4}}, {F1: []uint16{5, 6, 7, 8}},
		{F1: []uint16{9, 10, 11, 12}}, {F1: []uint16{13, 14, 15, 16}},
	}}

	for _, tc := range []struct {
		name string
		opts []dynssz.DynSszOption
	}{
		{"generated", nil},
		{"reflection", []dynssz.DynSszOption{dynssz.WithNoFastSsz(), dynssz.WithNoDelegation()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := dynssz.NewDynSsz(nil, tc.opts...)
			encoded, err := ds.MarshalSSZ(&source)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			// A target already holding elements, with a reference kept to one.
			target := &SimpleTypes2{F2: []*SimpleTypes2_C1{
				{F1: []uint16{99, 99, 99, 99}}, {F1: []uint16{99, 99, 99, 99}},
				{F1: []uint16{99, 99, 99, 99}}, {F1: []uint16{99, 99, 99, 99}},
			}}
			held := target.F2[0]

			if err = ds.UnmarshalSSZ(target, encoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if target.F2[0] != held {
				t.Errorf("element pointer was replaced instead of reused")
			}
			if held.F1[0] != 1 {
				t.Errorf("reused element holds %d, want the decoded value 1", held.F1[0])
			}

			// Reuse must keep allocations, not data: the result has to match a
			// decode into an empty target.
			fresh := new(SimpleTypes2)
			if err = ds.UnmarshalSSZ(fresh, encoded); err != nil {
				t.Fatalf("unmarshal into a fresh target: %v", err)
			}
			reusedBytes, err := ds.MarshalSSZ(target)
			if err != nil {
				t.Fatalf("re-marshal reused: %v", err)
			}
			freshBytes, err := ds.MarshalSSZ(fresh)
			if err != nil {
				t.Fatalf("re-marshal fresh: %v", err)
			}
			if !bytes.Equal(reusedBytes, freshBytes) {
				t.Errorf("decoding into a populated target gave a different value than into an empty one")
			}
		})
	}
}

// A Go array's SSZ length comes from the resolved dynssz-size. The static
// ssz-size is only the fallback for an unresolved expression, so a spec value
// above it is legitimate and the array -- which may be larger still -- is what
// bounds it. The generated code used to bound and iterate by the static value,
// so a preset that resolved higher was rejected outright while reflection
// encoded it.
func TestCodegenSpecSizedArrayUsesResolvedLength(t *testing.T) {
	if _, generated := any(&VecSpecLen{}).(sszutils.DynamicMarshaler); !generated {
		t.Skip("no generated code present")
	}

	for _, tc := range []struct {
		name string
		size uint64
		want int // serialized bytes: size*8 for V plus size for B
	}{
		{"above the static fallback", 8, 8*8 + 8},
		{"at the static fallback", 4, 4*8 + 4},
		{"below the static fallback", 2, 2*8 + 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			specs := map[string]any{"VECSPEC_LEN": tc.size}
			testCodegenPayloadByReflection(t, VecSpecLen_Payload, specs)

			ds := dynssz.NewDynSsz(specs)
			encoded, err := ds.MarshalSSZ(&VecSpecLen_Payload)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(encoded) != tc.want {
				t.Errorf("encoded %d bytes, want %d", len(encoded), tc.want)
			}
		})
	}

	// The backing array is the real bound, and exceeding it is still refused --
	// by both engines, since neither can read past the array.
	tooBig := map[string]any{"VECSPEC_LEN": 9}
	for _, tc := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{
		{"generated", dynssz.NewDynSsz(tooBig)},
		{"reflection", dynssz.NewDynSsz(tooBig, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())},
	} {
		t.Run("beyond the array/"+tc.name, func(t *testing.T) {
			if _, err := tc.ds.MarshalSSZ(&VecSpecLen_Payload); !errors.Is(err, sszutils.ErrInvalidConstraint) {
				t.Errorf("err = %v, want ErrInvalidConstraint", err)
			}
		})
	}
}

// A type wrapper is transparent to SSZ, so a list of wrappers around a basic
// value describes the same type as the plain list and must produce the same
// root. It used to merkleize one chunk per element rather than under the packed
// chunk count, so the two disagreed at every length -- in both engines, which
// agreed with each other and with neither the spec nor any other implementation.
func TestCodegenWrappedElementListRoot(t *testing.T) {
	if _, generated := any(&WrappedElemLists{}).(sszutils.DynamicHashRoot); !generated {
		t.Skip("no generated code present")
	}

	for _, tc := range []struct {
		name string
		opts []dynssz.DynSszOption
	}{
		{"generated", nil},
		{"reflection", []dynssz.DynSszOption{dynssz.WithNoFastSsz(), dynssz.WithNoDelegation()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := dynssz.NewDynSsz(nil, tc.opts...)

			wrapped, err := ds.HashTreeRoot(&WrappedElemLists_Payload)
			if err != nil {
				t.Fatalf("wrapped: %v", err)
			}
			plain, err := ds.HashTreeRoot(&PlainElemLists_Payload)
			if err != nil {
				t.Fatalf("plain: %v", err)
			}
			if wrapped != plain {
				t.Errorf("wrapped root %x differs from plain %x", wrapped, plain)
			}

			// The wrappers are transparent on the wire too, so the same bytes
			// have to come out.
			wrappedBytes, err := ds.MarshalSSZ(&WrappedElemLists_Payload)
			if err != nil {
				t.Fatalf("marshal wrapped: %v", err)
			}
			plainBytes, err := ds.MarshalSSZ(&PlainElemLists_Payload)
			if err != nil {
				t.Fatalf("marshal plain: %v", err)
			}
			if !bytes.Equal(wrappedBytes, plainBytes) {
				t.Errorf("wrapped encoding %x differs from plain %x", wrappedBytes, plainBytes)
			}
		})
	}
}

// A view's data type carries no tags because its layout lives in the view
// schema, so every list on it looks limit-less. That must not make the type
// unanalyzable: the limit exists, it is just written on the view. Hashing
// through the view works, hashing the bare data type does not.
func TestCodegenViewDataTypeNeedsNoLimits(t *testing.T) {
	if _, generated := any(&ViewTypes5_Base{}).(sszutils.DynamicViewHashRoot); !generated {
		t.Skip("no generated code present")
	}

	view := (*ViewTypes5_View1)(nil)
	cg := dynssz.NewDynSsz(nil)
	refl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())

	cgRoot, err := cg.HashTreeRoot(&ViewTypes5_Payload, dynssz.WithViewDescriptor(view))
	if err != nil {
		t.Fatalf("generated root through the view: %v", err)
	}
	reflRoot, err := refl.HashTreeRoot(&ViewTypes5_Payload, dynssz.WithViewDescriptor(view))
	if err != nil {
		t.Fatalf("reflection root through the view: %v", err)
	}
	if cgRoot != reflRoot {
		t.Fatalf("generated root %x differs from reflection %x", cgRoot, reflRoot)
	}

	// The view's limits are what make the root defined, so the same value has no
	// root without it -- and the error says where to look.
	_, err = refl.HashTreeRoot(&ViewTypes5_Payload)
	if !errors.Is(err, sszutils.ErrExtendedTypeDisabled) {
		t.Fatalf("err = %v, want ErrExtendedTypeDisabled", err)
	}
	if !strings.Contains(err.Error(), "through its view") {
		t.Errorf("error %q does not point at the view", err)
	}

	// Serialization never needs a limit, so the data type round-trips on its own.
	encoded, err := refl.MarshalSSZ(&ViewTypes5_Payload)
	if err != nil {
		t.Fatalf("marshal without a view: %v", err)
	}
	decoded := new(ViewTypes5_Base)
	if err := refl.UnmarshalSSZ(decoded, encoded); err != nil {
		t.Fatalf("unmarshal without a view: %v", err)
	}
	if len(decoded.F2) != len(ViewTypes5_Payload.F2) {
		t.Fatalf("decoded %d elements, want %d", len(decoded.F2), len(ViewTypes5_Payload.F2))
	}
}

// A limit-less bitlist derives its merkleization limit from the value rather
// than the type, so the two engines have to derive it identically -- and the
// root still has to commit to the bit length, or bitlists differing only in
// their terminator position would collide.
func TestCodegenLimitlessBitlistRootMatchesReflection(t *testing.T) {
	if _, generated := any(&UnboundedBitlist{}).(sszutils.DynamicHashRoot); !generated {
		t.Skip("no generated code present")
	}

	// A limit-less bitlist has no SSZ root, so hashing one is an extension.
	cg := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	refl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation(), dynssz.WithExtendedTypes())

	// Terminator bit at a different position each time, so every value is a
	// distinct bitlist rather than the same bits re-padded.
	values := [][]byte{{0x01}, {0x02}, {0x03}, {0x0f}, {0xff, 0x01}, {0x00, 0x02}}

	roots := map[[32]byte][]string{}
	for _, v := range values {
		payload := UnboundedBitlist{B: v}

		cgRoot, err := cg.HashTreeRoot(payload)
		if err != nil {
			t.Fatalf("%x generated root: %v", v, err)
		}
		reflRoot, err := refl.HashTreeRoot(payload)
		if err != nil {
			t.Fatalf("%x reflection root: %v", v, err)
		}
		if cgRoot != reflRoot {
			t.Fatalf("%x: generated root %x differs from reflection %x", v, cgRoot, reflRoot)
		}
		roots[cgRoot] = append(roots[cgRoot], fmt.Sprintf("%x", v))
	}

	for _, colliding := range roots {
		if len(colliding) > 1 {
			t.Errorf("distinct bitlists share a root: %v", colliding)
		}
	}

	// Without extended types the root has no definition, so it is refused
	// rather than silently derived.
	plain := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
	if _, err := plain.HashTreeRoot(UnboundedBitlist_Payload); !errors.Is(err, sszutils.ErrExtendedTypeDisabled) {
		t.Fatalf("err = %v, want ErrExtendedTypeDisabled", err)
	}
}

func TestCodegenLimitlessListRootCommitsToLength(t *testing.T) {
	if _, generated := any(&ZeroMaxList{}).(sszutils.DynamicHashRoot); !generated {
		t.Skip("no generated code present")
	}

	// A limit-less list has no SSZ root, so hashing one is an extension.
	cg := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
	refl := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation(), dynssz.WithExtendedTypes())

	values := [][]uint64{{}, {1}, {1, 0}, {1, 0, 0, 0}, {1, 2}, {1, 2, 0, 0}}

	roots := map[[32]byte][]string{}
	for _, v := range values {
		payload := ZeroMaxList{X: v}

		cgRoot, err := cg.HashTreeRoot(payload)
		if err != nil {
			t.Fatalf("%v generated root: %v", v, err)
		}
		reflRoot, err := refl.HashTreeRoot(payload)
		if err != nil {
			t.Fatalf("%v reflection root: %v", v, err)
		}
		if cgRoot != reflRoot {
			t.Fatalf("%v: generated root %x differs from reflection %x", v, cgRoot, reflRoot)
		}
		roots[cgRoot] = append(roots[cgRoot], fmt.Sprint(v))
	}

	for _, colliding := range roots {
		if len(colliding) > 1 {
			t.Errorf("distinct values share a root: %v", colliding)
		}
	}
}

// A slice standing in for a fixed vector may hold fewer elements than the
// vector: the ones it does not hold are the zeros the vector is defined to
// contain. That is what lets a zero value encode at all -- an empty slice in a
// vector field is the all-zero vector, not a missing one. Holding more than the
// vector's length is rejected instead, since encoding it would drop data.
//
// Both engines have to agree byte for byte, or a value would encode differently
// depending on whether its type had generated methods.
func TestVectorSliceShorterThanItsLength(t *testing.T) {
	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())

	// Held through the interface: without generated code the method does not
	// exist, so naming it directly would not compile.
	type fastsszMarshaler interface {
		MarshalSSZTo(buf []byte) ([]byte, error)
		HashTreeRoot() ([32]byte, error)
	}

	marshalBoth := func(t *testing.T, value *SimpleTypes1) ([]byte, error) {
		t.Helper()

		encoded, err := ds.MarshalSSZ(value)
		generated, hasMethods := any(value).(fastsszMarshaler)
		if !hasMethods {
			return encoded, err
		}

		fromCode, codeErr := generated.MarshalSSZTo(nil)
		if (err == nil) != (codeErr == nil) {
			t.Fatalf("reflection err = %v, generated err = %v", err, codeErr)
		}
		if err == nil && !bytes.Equal(encoded, fromCode) {
			t.Fatalf("reflection encoded %x, generated %x", encoded, fromCode)
		}
		if err == nil {
			root, rootErr := ds.HashTreeRoot(value)
			codeRoot, codeRootErr := generated.HashTreeRoot()
			if rootErr != nil || codeRootErr != nil {
				t.Fatalf("hash tree root: %v / %v", rootErr, codeRootErr)
			}
			if root != codeRoot {
				t.Fatalf("reflection root %x, generated root %x", root, codeRoot)
			}
		}

		return encoded, err
	}

	// A zero value has an empty slice in every vector field and must encode.
	zero := SimpleTypes1{}
	encodedZero, err := marshalBoth(t, &zero)
	if err != nil {
		t.Fatalf("a zero value must encode: %v", err)
	}

	// Vec8 is ssz-size:"4"; two elements leave two zeros.
	short := SimpleTypes1{Vec8: []uint8{1, 2}}
	encodedShort, err := marshalBoth(t, &short)
	if err != nil {
		t.Fatalf("a short vector slice must encode: %v", err)
	}

	// Padding is what the spelled-out zeros would have produced.
	padded := SimpleTypes1{Vec8: []uint8{1, 2, 0, 0}}
	encodedPadded, err := marshalBoth(t, &padded)
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	if !bytes.Equal(encodedShort, encodedPadded) {
		t.Errorf("short slice encoded %x, want the padded %x", encodedShort, encodedPadded)
	}
	if len(encodedZero) != len(encodedPadded) {
		t.Errorf("zero value encoded %d bytes, want the fixed %d", len(encodedZero), len(encodedPadded))
	}

	// More elements than the vector holds would have to be dropped.
	long := SimpleTypes1{Vec8: []uint8{1, 2, 3, 4, 5}}
	if _, err := marshalBoth(t, &long); !errors.Is(err, sszutils.ErrVectorLength) {
		t.Errorf("err = %v, want ErrVectorLength", err)
	}
}
