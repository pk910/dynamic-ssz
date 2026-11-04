package engine

import (
	"bytes"
	"reflect"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

type oracleFixture struct {
	A uint16
	B []uint32 `ssz-max:"4"`
	C [2]uint64
}

func TestReferenceOracleChecksClassicSchema(t *testing.T) {
	value := &oracleFixture{A: 0x1234, B: []uint32{1, 2}, C: [2]uint64{3, 4}}
	wantEncoding := []byte{
		0x34, 0x12, 0x16, 0, 0, 0,
		3, 0, 0, 0, 0, 0, 0, 0,
		4, 0, 0, 0, 0, 0, 0, 0,
		1, 0, 0, 0, 2, 0, 0, 0,
	}
	gotEncoding := oracleMarshal(reflect.ValueOf(value), oracleTag{})
	if !bytes.Equal(gotEncoding, wantEncoding) {
		t.Fatalf("reference encoding = %x, want %x", gotEncoding, wantEncoding)
	}

	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
	root, err := ds.HashTreeRoot(value)
	if err != nil {
		t.Fatal(err)
	}
	if detail, supported, ok := referenceCheck(value, wantEncoding, root); !supported || !ok {
		t.Fatalf("reference check = supported %v, ok %v: %s", supported, ok, detail)
	}

	bad := append([]byte(nil), wantEncoding...)
	bad[len(bad)-1] ^= 1
	if _, supported, ok := referenceCheck(value, bad, root); !supported || ok {
		t.Fatalf("mutated encoding = supported %v, ok %v; want supported mismatch", supported, ok)
	}
}

func TestReferenceOracleSkipsModernSchema(t *testing.T) {
	type progressive struct {
		Values []uint64 `ssz-type:"progressive-list"`
	}
	if _, supported, _ := referenceCheck(&progressive{}, nil, [32]byte{}); supported {
		t.Fatal("progressive list unexpectedly treated as supported by classic oracle")
	}
}
