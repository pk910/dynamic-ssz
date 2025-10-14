package tests

import (
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
		Hash:    "d1374d2c572d3f257fb1969ec9a0ad3dd20b480a124a18e26ab8d06a5369e9fe",
	},
	{
		Name:    "SimpleTypesWithSpecs",
		Payload: SimpleTypesWithSpecs_Payload,
		Specs:   SimpleTypesWithSpecs_Specs,
		Hash:    "81e24b3e3fc483146f660ae13ecab67961cf55ca2348d5b43e0892fdec6a3bdc",
	},
	{
		Name:    "ProgressiveTypes",
		Payload: ProgressiveTypes_Payload,
		Specs:   map[string]any{},
		Hash:    "d6896b089f7b899e395f580a8eedda62900494a038fb738feda76d0d613e2619",
	},
}

func TestCodegenGeneration(t *testing.T) {
	for _, payload := range testMatrix {
		t.Run(payload.Name, func(t *testing.T) {
			testCodegenPayload(t, payload)
		})
	}
}

func testCodegenPayload(t *testing.T, payload TestPayload) {
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
}
