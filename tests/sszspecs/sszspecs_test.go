package sszspecs

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

const expectedFixtureCount = 117

type fixture struct {
	TypeName        string `json:"typeName"`
	Serialized      string `json:"serialized"`
	RawBytes        string `json:"rawBytes"`
	Root            string `json:"root"`
	RejectionReason string `json:"rejectionReason"`
}

func TestOfficialSSZSpecsV010(t *testing.T) {
	root := os.Getenv("SSZ_SPECS_TESTS_DIR")
	if root == "" {
		t.Skip("SSZ_SPECS_TESTS_DIR is unset; run ./setup_test_data.sh setup")
	}

	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error { //nolint:gosec // The caller explicitly selects this read-only test-data tree.
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}
	if len(files) != expectedFixtureCount {
		t.Fatalf("found %d fixture files, want %d (wrong or incomplete ssz-specs release)", len(files), expectedFixtureCount)
	}

	ds := dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation())
	for _, path := range files {
		path := path
		rel, _ := filepath.Rel(root, path)
		t.Run(strings.TrimSuffix(filepath.ToSlash(rel), ".json"), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var cases map[string]fixture
			if err := json.Unmarshal(data, &cases); err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			if len(cases) != 1 {
				t.Fatalf("fixture contains %d cases, want 1", len(cases))
			}
			for name, tc := range cases {
				t.Run(name, func(t *testing.T) { runFixture(t, ds, rel, &tc) })
			}
		})
	}
}

func runFixture(t *testing.T, ds *dynssz.DynSsz, path string, tc *fixture) {
	t.Helper()
	encodedHex := tc.Serialized
	if tc.RawBytes != "" {
		encodedHex = tc.RawBytes
	}
	encoded := decodeHex(t, encodedHex)

	value, factoryErr := newFixtureValue(filepath.ToSlash(path), tc.TypeName)
	if factoryErr != nil {
		t.Fatal(factoryErr)
	}
	if tc.RejectionReason != "" {
		if err := ds.UnmarshalSSZ(value, encoded); err == nil {
			t.Fatalf("accepted invalid %s bytes (%s)", tc.TypeName, tc.RejectionReason)
		}
		streamValue, _ := newFixtureValue(filepath.ToSlash(path), tc.TypeName)
		if err := ds.UnmarshalSSZReader(streamValue, bytes.NewReader(encoded), len(encoded)); err == nil {
			t.Fatalf("stream decoder accepted invalid %s bytes (%s)", tc.TypeName, tc.RejectionReason)
		}
		return
	}

	if err := ds.UnmarshalSSZ(value, encoded); err != nil {
		t.Fatalf("unmarshal %s: %v", tc.TypeName, err)
	}
	remarshaled, err := ds.MarshalSSZ(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", tc.TypeName, err)
	}
	if !bytes.Equal(remarshaled, encoded) {
		t.Fatalf("round trip %s: got %x, want %x", tc.TypeName, remarshaled, encoded)
	}
	if size, sizeErr := ds.SizeSSZ(value); sizeErr != nil || size != len(encoded) {
		t.Fatalf("size %s: got %d (%v), want %d", tc.TypeName, size, sizeErr, len(encoded))
	}
	wantRoot := decodeHex(t, tc.Root)
	gotRoot, err := ds.HashTreeRoot(value)
	if err != nil {
		t.Fatalf("hash tree root %s: %v", tc.TypeName, err)
	}
	if !bytes.Equal(gotRoot[:], wantRoot) {
		t.Fatalf("hash tree root %s: got %x, want %x", tc.TypeName, gotRoot, wantRoot)
	}

	streamValue, _ := newFixtureValue(filepath.ToSlash(path), tc.TypeName)
	if err := ds.UnmarshalSSZReader(streamValue, bytes.NewReader(encoded), len(encoded)); err != nil {
		t.Fatalf("stream unmarshal %s: %v", tc.TypeName, err)
	}
	var stream bytes.Buffer
	if err := ds.MarshalSSZWriter(streamValue, &stream); err != nil {
		t.Fatalf("stream marshal %s: %v", tc.TypeName, err)
	}
	if !bytes.Equal(stream.Bytes(), encoded) {
		t.Fatalf("stream round trip %s: got %x, want %x", tc.TypeName, stream.Bytes(), encoded)
	}
}

func decodeHex(t *testing.T, value string) []byte {
	t.Helper()
	value = strings.TrimPrefix(value, "0x")
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode %q: %v", value, err)
	}
	return decoded
}

func newFixtureValue(path, name string) (any, error) {
	switch name {
	case "Boolean":
		return new(bool), nil
	case "Uint8":
		return new(uint8), nil
	case "Uint16":
		return new(uint16), nil
	case "Uint32":
		return new(uint32), nil
	case "Uint64":
		return new(uint64), nil
	case "Uint128":
		return new([16]byte), nil
	case "Uint256":
		return new([32]byte), nil
	case "Bytes4":
		return new(bytes4), nil
	case "Bytes32":
		return new(bytes32), nil
	case "Bytes52":
		return new(bytes52), nil
	case "Bytes64":
		return new(bytes64), nil
	case "ByteList512KiB":
		return new(byteList512KiB), nil
	case "SampleBitVector8":
		return new(bitvector8), nil
	case "SampleBitVector64":
		return new(bitvector64), nil
	case "SampleBitList16":
		return new(bitlist16), nil
	case "SampleUint16Vector3":
		return new([3]uint16), nil
	case "SampleUint64Vector4":
		return new([4]uint64), nil
	case "SampleUint32List16":
		return new(uint32List16), nil
	case "SampleBytes32List8":
		return new(bytes32List8), nil
	case "SmokeBitList8":
		return new(bitlist8), nil
	case "BoundaryBitVector1":
		return new(bitvector1), nil
	case "BoundaryBitVector7":
		return new(bitvector7), nil
	case "BoundaryBitVector9":
		return new(bitvector9), nil
	case "BoundaryBitVector255":
		return new(bitvector255), nil
	case "BoundaryBitVector256":
		return new(bitvector256), nil
	case "BoundaryBitVector257":
		return new(bitvector257), nil
	case "BoundaryBitList256":
		return new(bitlist256), nil
	case "BoundaryUint64List32":
		return new(uint64List32), nil
	case "SampleUint64ProgressiveList":
		return new(progressiveUint64), nil
	case "SampleBytes32ProgressiveList":
		return new(progressiveBytes32), nil
	case "SampleUint16ProgressiveList":
		return new(progressiveUint16), nil
	case "SampleNestedProgressiveList":
		return new(nestedProgressiveUint16), nil
	case "SampleContainerWithProgressiveList":
		return new(sampleContainerWithProgressiveList), nil
	case "ProgressiveBitList":
		return new(progressiveBitlist), nil
	case "SampleSquare":
		return new(sampleSquare), nil
	case "SampleCircle":
		return new(sampleCircle), nil
	case "SampleOneField":
		return new(sampleOneField), nil
	case "SampleLeadingGaps":
		return new(sampleLeadingGaps), nil
	case "SampleMultipleGaps":
		return new(sampleMultipleGaps), nil
	case "SampleWidestLayout":
		return new(sampleWidestLayout), nil
	case "SampleLevelBoundary":
		return new(sampleLevelBoundary), nil
	case "SampleBoundedListField":
		return new(sampleBoundedListField), nil
	case "SampleProgressiveFields":
		return new(sampleProgressiveFields), nil
	case "SampleOuterShape":
		return new(sampleOuterShape), nil
	case "SampleSquareProgressiveList":
		return new(squareProgressiveList), nil
	case "SampleShape":
		return new(sampleShape), nil
	case "SampleNumbers":
		return new(sampleNumbers), nil
	case "SampleEmptyProne":
		return new(sampleEmptyProne), nil
	case "SampleNestedShape":
		return new(sampleNestedShape), nil
	case "SampleShapeProgressiveContainer":
		return new(sampleShapeProgressiveContainer), nil
	case "SampleShapeProgressiveList":
		return new(shapeProgressiveList), nil
	case "SampleShapeContainer":
		if strings.Contains(path, "test_compatible_unions") {
			return new(unionShapeContainer), nil
		}
		return new(progressiveShapeContainer), nil
	default:
		return nil, fmt.Errorf("no Go type mapping for official fixture type %q in %s", name, path)
	}
}
