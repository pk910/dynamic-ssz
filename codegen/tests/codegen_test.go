package tests

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"runtime"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/codegen/tests/views"
)

type TestPayload struct {
	Name    string         // Test name
	Payload any            // Test payload
	View    any            // Test view
	Specs   map[string]any // Dynamic specifications
	Hash    string         // Expected hash root
}

var testMatrix = []TestPayload{
	{
		Name:    "SimpleTypes1",
		Payload: SimpleTypes1_Payload,
		Specs:   map[string]any{},
		Hash:    "b528ffea01ddd484a9c1e6d16063512f9ec3097803dbf50dcdfa68effb1508df",
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
		Hash:    "53aa7926e7d5b0b409990cde59849a85047431ce8d30b4e5b499754dcb438c48",
	},
	{
		Name:    "SimpleTypesWithSpecs",
		Payload: SimpleTypesWithSpecs_Payload,
		Specs:   SimpleTypesWithSpecs_Specs,
		Hash:    "893aca6e960e166d2bde84c27e39db72ad85e271e40a92160b017ebf551334a8",
	},
	{
		Name:    "SimpleTypesWithSpecs2",
		Payload: SimpleTypesWithSpecs2_Payload,
		Specs:   SimpleTypesWithSpecs_Specs,
		Hash:    "966912b4d9e6b44fbebce56369fa255b76cd777d76e4dac2d396df93916ac077",
	},
	{
		Name:    "ProgressiveTypes",
		Payload: ProgressiveTypes_Payload,
		Specs:   map[string]any{},
		Hash:    "317f412cd2d042f367c4f2fb6447828ef9524396428eb2ed0837524bcc70433c",
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
		Name:    "ZeroMaxList",
		Payload: ZeroMaxList_Payload,
		Specs:   map[string]any{},
		Hash:    "0100000000000000020000000000000003000000000000000000000000000000",
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
		Hash:    "82acb108812798107c2bed326c83a2881c90f942883a6e3de6144f30b2987959",
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
}

func TestCodegenGeneration(t *testing.T) {
	for _, payload := range testMatrix {
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
		{"valueBigInt", ExtendedTypes1_Payload1, "d43c9c95a419854cc80d68260f4f3777ec1f5f6a699575f5d9ddaf38fb5c86a0"},
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

func testCodegenPayload(t *testing.T, payload TestPayload) {
	t.Helper()
	ds := dynssz.NewDynSsz(payload.Specs)

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
	uv.U.Variant = 1
	uv.U.Data = SimpleTypes1_C1{F1: 9}

	wu := WrapUnionField{}
	wu.W.Data.Variant = 0
	wu.W.Data.Data = uint32(42)

	cases := []struct {
		name string
		val  any
	}{
		{"TopVecOfVar", &TopVecOfVar{{1, 2}, {3}, {}}},
		{"TopVecOfVar-underfill", &TopVecOfVar{{1, 2}}},
		{"UnionSamePkgVariant", &uv},
		{"PtrPrimitiveList", &PtrPrimitiveList{F: []*uint64{&u1, &u2}}},
		{"FixedVecPtrStr", &FixedVecPtrStr{F: []*string{&s, &s}}},
		{"FixedVecPtrStr-underfill", &FixedVecPtrStr{F: []*string{&s}}},
		{"FixedVecPtrList", &FixedVecPtrList{F: []*[]uint16{&l, &l}}},
		{"FixedVecStr-underfill", &FixedVecStr{F: []string{"a"}}},
		{"PtrDynCollectionField", &PtrDynCollectionField{F: &bl}},
		{"WrapUnionField", &wu},
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
