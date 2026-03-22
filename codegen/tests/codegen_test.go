package tests

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

type TestPayload struct {
	Name    string         // Test name
	Payload any            // Test payload
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
		Hash:    "dea6e96a178704df1f792e0374fa60301d5fab10699687c4a395ecbfa9b78cc4",
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
		Name:    "AnnotatedContainer",
		Payload: AnnotatedContainer_Payload,
		Specs:   map[string]any{},
		Hash:    "683902f02e8035c2301b0eac540d4e311d24638abd660e4fd8f580db8e63a89d",
	},
	{
		Name:    "AnnotatedOverrideContainer",
		Payload: AnnotatedOverrideContainer_Payload,
		Specs:   map[string]any{},
		Hash:    "45aedf38b395a8e815bed8c5dadd89a0d853574e9d0d7b54ae9c2b62429ae4cd",
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

func TestCodegenCoverageTypes1(t *testing.T) {
	testCodegenPayloadByReflection(t, CoverageTypes1_Payload, SimpleTypesWithSpecs_Specs)
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

// testCodegenPayloadByReflection compares generated code output against
// reflection-based implementation. No pre-computed hash needed.
func testCodegenPayloadByReflection(t *testing.T, payload any, specs map[string]any, opts ...dynssz.DynSszOption) {
	t.Helper()

	refOpts := append([]dynssz.DynSszOption{
		dynssz.WithNoFastSsz(),
		dynssz.WithNoFastHash(),
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

	hashRoot, err := ds.HashTreeRoot(payload.Payload)
	if err != nil {
		t.Fatalf("Failed to hash tree root: %v", err)
	}
	hashRootHex := hex.EncodeToString(hashRoot[:])
	if hashRootHex != payload.Hash {
		t.Fatalf("Hash root mismatch 1: expected %s, got %s", payload.Hash, hashRootHex)
	}

	sszBytes, err := ds.MarshalSSZ(payload.Payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	obj := &struct {
		Data any
	}{}
	reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(payload.Payload)))

	err = ds.UnmarshalSSZ(obj.Data, sszBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	hashRoot, err = ds.HashTreeRoot(obj.Data)
	if err != nil {
		t.Fatalf("Failed to hash tree root: %v", err)
	}
	hashRootHex = hex.EncodeToString(hashRoot[:])
	if hashRootHex != payload.Hash {
		t.Fatalf("Hash root mismatch 2: expected %s, got %s", payload.Hash, hashRootHex)
	}

	memBuf := make([]byte, 0, len(sszBytes))
	memWriter := bytes.NewBuffer(memBuf)
	err = ds.MarshalSSZWriter(payload.Payload, memWriter)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}
	memBuf = memWriter.Bytes()
	if !bytes.Equal(memBuf, sszBytes) {
		t.Fatalf("MarshalSSZWriter mismatch: expected %x, got %x", sszBytes, memBuf)
	}

	reflect.ValueOf(obj).Elem().Field(0).Set(reflect.New(reflect.TypeOf(payload.Payload)))

	err = ds.UnmarshalSSZReader(obj.Data, bytes.NewReader(sszBytes), len(sszBytes))
	if err != nil {
		t.Fatalf("Failed to unmarshal payload: %v", err)
	}

	hashRoot, err = ds.HashTreeRoot(obj.Data)
	if err != nil {
		t.Fatalf("Failed to hash tree root: %v", err)
	}
	hashRootHex = hex.EncodeToString(hashRoot[:])
	if hashRootHex != payload.Hash {
		t.Fatalf("Hash root mismatch 2: expected %s, got %s", payload.Hash, hashRootHex)
	}
}
