package perftests

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	ssz "github.com/pk910/dynamic-ssz"
	"gopkg.in/yaml.v2"
)

type metadata struct {
	HTR string `json:"htr"`
}

var (
	blockMainnetData []byte
	stateMainnetData []byte
	blockMinimalData []byte
	stateMinimalData []byte

	blockMainnetHTR [32]byte
	stateMainnetHTR [32]byte
	blockMinimalHTR [32]byte
	stateMinimalHTR [32]byte

	sliceVectorData []byte

	// Codegen mode: uses generated SSZ code when available
	dynSszCodegenMainnet *ssz.DynSsz
	dynSszCodegenMinimal *ssz.DynSsz

	// Reflection mode: pure reflection, no generated code
	dynSszReflMainnet *ssz.DynSsz
	dynSszReflMinimal *ssz.DynSsz
)

func init() {
	var err error

	blockMainnetData, err = os.ReadFile("res/block-mainnet.ssz")
	if err != nil {
		panic("failed to load res/block-mainnet.ssz: " + err.Error())
	}
	stateMainnetData, err = os.ReadFile("res/state-mainnet.ssz")
	if err != nil {
		panic("failed to load res/state-mainnet.ssz: " + err.Error())
	}
	blockMinimalData, err = os.ReadFile("res/block-minimal.ssz")
	if err != nil {
		panic("failed to load res/block-minimal.ssz: " + err.Error())
	}
	stateMinimalData, err = os.ReadFile("res/state-minimal.ssz")
	if err != nil {
		panic("failed to load res/state-minimal.ssz: " + err.Error())
	}

	blockMainnetHTR = loadHTR("res/block-mainnet-meta.json")
	stateMainnetHTR = loadHTR("res/state-mainnet-meta.json")
	blockMinimalHTR = loadHTR("res/block-minimal-meta.json")
	stateMinimalHTR = loadHTR("res/state-minimal-meta.json")

	minimalPresetBytes, err := os.ReadFile("minimal-preset.yaml")
	if err != nil {
		panic("failed to load minimal-preset.yaml: " + err.Error())
	}
	minimalSpecs := make(map[string]any)
	if err := yaml.Unmarshal(minimalPresetBytes, &minimalSpecs); err != nil {
		panic("failed to parse minimal-preset.yaml: " + err.Error())
	}

	dynSszCodegenMainnet = ssz.NewDynSsz(nil)
	dynSszCodegenMinimal = ssz.NewDynSsz(minimalSpecs)
	dynSszReflMainnet = ssz.NewDynSsz(nil, ssz.WithNoFastSsz(), ssz.WithNoDelegation())
	dynSszReflMinimal = ssz.NewDynSsz(minimalSpecs, ssz.WithNoFastSsz(), ssz.WithNoDelegation())

	sliceVectorData = buildSliceVectorData()
}

// buildSliceVectorData serializes a filled SliceVectorBundle. The values only
// have to be well formed and stable; the fixture exists for its field shapes.
func buildSliceVectorData() []byte {
	fill := func(count, width int) [][]byte {
		out := make([][]byte, count)
		for i := range out {
			elem := make([]byte, width)
			for j := range elem {
				elem[j] = byte(i + j)
			}
			out[i] = elem
		}
		return out
	}

	bundle := &SliceVectorBundle{
		Roots:      fill(4096, 32),
		Pubkeys:    fill(2048, 48),
		Signatures: fill(512, 96),
		Graffiti:   make([]byte, 32),
	}

	data, err := ssz.NewDynSsz(nil).MarshalSSZ(bundle)
	if err != nil {
		panic("failed to build slice vector fixture: " + err.Error())
	}
	return data
}

func loadHTR(path string) [32]byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic("failed to load " + path + ": " + err.Error())
	}
	var meta metadata
	if err = json.Unmarshal(data, &meta); err != nil {
		panic("failed to parse " + path + ": " + err.Error())
	}
	htrBytes, err := hex.DecodeString(meta.HTR)
	if err != nil {
		panic("failed to decode HTR from " + path + ": " + err.Error())
	}
	var htr [32]byte
	copy(htr[:], htrBytes)
	return htr
}

type testWriter struct {
	data []byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *testWriter) Reset() {
	w.data = w.data[:0]
}

// runBlockBenchmarks runs all block benchmark operations as sub-benchmarks.
func runBlockBenchmarks(b *testing.B, ds *ssz.DynSsz, data []byte, expectedHTR [32]byte) {
	b.Helper()

	b.Run("Unmarshal", func(b *testing.B) {
		var block *SignedBeaconBlock
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			block = new(SignedBeaconBlock)
			if err := ds.UnmarshalSSZ(block, data); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		verifyBlockHTR(b, ds, block, expectedHTR)
	})

	b.Run("UnmarshalReader", func(b *testing.B) {
		var block *SignedBeaconBlock
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			block = new(SignedBeaconBlock)
			reader := bytes.NewReader(data)
			if err := ds.UnmarshalSSZReader(block, reader, len(data)); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		verifyBlockHTR(b, ds, block, expectedHTR)
	})

	// Unknown size: the length is discovered at EOF, so the trailing dynamic
	// chain decodes as open regions and every declared extent is treated as
	// unwitnessed until the bytes arrive.
	b.Run("UnmarshalReaderUnknownSize", func(b *testing.B) {
		var block *SignedBeaconBlock
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			block = new(SignedBeaconBlock)
			reader := bytes.NewReader(data)
			if err := ds.UnmarshalSSZReader(block, reader, -1); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		verifyBlockHTR(b, ds, block, expectedHTR)
	})

	b.Run("Marshal", func(b *testing.B) {
		block := new(SignedBeaconBlock)
		if err := ds.UnmarshalSSZ(block, data); err != nil {
			b.Fatal(err)
		}
		var result []byte
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var err error
			result, err = ds.MarshalSSZ(block)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if !bytes.Equal(result, data) {
			b.Fatal("marshaled data does not match original")
		}
	})

	b.Run("MarshalWriter", func(b *testing.B) {
		block := new(SignedBeaconBlock)
		if err := ds.UnmarshalSSZ(block, data); err != nil {
			b.Fatal(err)
		}
		w := &testWriter{data: make([]byte, 0, len(data))}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w.Reset()
			if err := ds.MarshalSSZWriter(block, w); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if !bytes.Equal(w.data, data) {
			b.Fatal("marshaled data does not match original")
		}
	})

	b.Run("HashTreeRoot", func(b *testing.B) {
		block := new(SignedBeaconBlock)
		if err := ds.UnmarshalSSZ(block, data); err != nil {
			b.Fatal(err)
		}
		var htr [32]byte
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var err error
			htr, err = ds.HashTreeRoot(block.Message)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if htr != expectedHTR {
			b.Fatalf("HTR mismatch: got %x, want %x", htr, expectedHTR)
		}
	})
}

// runStateBenchmarks runs all state benchmark operations as sub-benchmarks.
func runStateBenchmarks(b *testing.B, ds *ssz.DynSsz, data []byte, expectedHTR [32]byte) {
	b.Helper()

	b.Run("Unmarshal", func(b *testing.B) {
		var state *BeaconState
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			state = new(BeaconState)
			if err := ds.UnmarshalSSZ(state, data); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		verifyStateHTR(b, ds, state, expectedHTR)
	})

	b.Run("UnmarshalReader", func(b *testing.B) {
		var state *BeaconState
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			state = new(BeaconState)
			reader := bytes.NewReader(data)
			if err := ds.UnmarshalSSZReader(state, reader, len(data)); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		verifyStateHTR(b, ds, state, expectedHTR)
	})

	// Unknown size: see the block benchmark above.
	b.Run("UnmarshalReaderUnknownSize", func(b *testing.B) {
		var state *BeaconState
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			state = new(BeaconState)
			reader := bytes.NewReader(data)
			if err := ds.UnmarshalSSZReader(state, reader, -1); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		verifyStateHTR(b, ds, state, expectedHTR)
	})

	b.Run("Marshal", func(b *testing.B) {
		state := new(BeaconState)
		if err := ds.UnmarshalSSZ(state, data); err != nil {
			b.Fatal(err)
		}
		var result []byte
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var err error
			result, err = ds.MarshalSSZ(state)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if !bytes.Equal(result, data) {
			b.Fatal("marshaled data does not match original")
		}
	})

	b.Run("MarshalWriter", func(b *testing.B) {
		state := new(BeaconState)
		if err := ds.UnmarshalSSZ(state, data); err != nil {
			b.Fatal(err)
		}
		w := &testWriter{data: make([]byte, 0, len(data))}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w.Reset()
			if err := ds.MarshalSSZWriter(state, w); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if !bytes.Equal(w.data, data) {
			b.Fatal("marshaled data does not match original")
		}
	})

	b.Run("HashTreeRoot", func(b *testing.B) {
		state := new(BeaconState)
		if err := ds.UnmarshalSSZ(state, data); err != nil {
			b.Fatal(err)
		}
		var htr [32]byte
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var err error
			htr, err = ds.HashTreeRoot(state)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if htr != expectedHTR {
			b.Fatalf("HTR mismatch: got %x, want %x", htr, expectedHTR)
		}
	})

	b.Run("GetTree", func(b *testing.B) {
		state := new(BeaconState)
		if err := ds.UnmarshalSSZ(state, data); err != nil {
			b.Fatal(err)
		}
		var root []byte
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tree, err := ds.GetTree(state)
			if err != nil {
				b.Fatal(err)
			}
			root = tree.Hash()
		}
		b.StopTimer()
		if !bytes.Equal(root, expectedHTR[:]) {
			b.Fatalf("tree root mismatch: got %x, want %x", root, expectedHTR)
		}
	})
}

func verifyBlockHTR(b *testing.B, ds *ssz.DynSsz, block *SignedBeaconBlock, expected [32]byte) {
	b.Helper()
	htr, err := ds.HashTreeRoot(block.Message)
	if err != nil {
		b.Fatal(err)
	}
	if htr != expected {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, expected)
	}
}

func verifyStateHTR(b *testing.B, ds *ssz.DynSsz, state *BeaconState, expected [32]byte) {
	b.Helper()
	htr, err := ds.HashTreeRoot(state)
	if err != nil {
		b.Fatal(err)
	}
	if htr != expected {
		b.Fatalf("HTR mismatch: got %x, want %x", htr, expected)
	}
}

// ========================= CODEGEN BENCHMARKS =========================

func BenchmarkCodegen_BlockMainnet(b *testing.B) {
	runBlockBenchmarks(b, dynSszCodegenMainnet, blockMainnetData, blockMainnetHTR)
}

func BenchmarkCodegen_StateMainnet(b *testing.B) {
	runStateBenchmarks(b, dynSszCodegenMainnet, stateMainnetData, stateMainnetHTR)
}

func BenchmarkCodegen_BlockMinimal(b *testing.B) {
	runBlockBenchmarks(b, dynSszCodegenMinimal, blockMinimalData, blockMinimalHTR)
}

func BenchmarkCodegen_StateMinimal(b *testing.B) {
	runStateBenchmarks(b, dynSszCodegenMinimal, stateMinimalData, stateMinimalHTR)
}

// ========================= REFLECTION BENCHMARKS =========================

func BenchmarkReflection_BlockMainnet(b *testing.B) {
	runBlockBenchmarks(b, dynSszReflMainnet, blockMainnetData, blockMainnetHTR)
}

func BenchmarkReflection_StateMainnet(b *testing.B) {
	runStateBenchmarks(b, dynSszReflMainnet, stateMainnetData, stateMainnetHTR)
}

func BenchmarkReflection_BlockMinimal(b *testing.B) {
	runBlockBenchmarks(b, dynSszReflMinimal, blockMinimalData, blockMinimalHTR)
}

func BenchmarkReflection_StateMinimal(b *testing.B) {
	runStateBenchmarks(b, dynSszReflMinimal, stateMinimalData, stateMinimalHTR)
}

// runSliceVectorBenchmarks runs the operations over a fixture whose fields are
// all slice-backed vectors, so every element decoded is one reservation.
func runSliceVectorBenchmarks(b *testing.B, ds *ssz.DynSsz, data []byte) {
	b.Helper()

	b.Run("Unmarshal", func(b *testing.B) {
		var bundle *SliceVectorBundle
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bundle = new(SliceVectorBundle)
			if err := ds.UnmarshalSSZ(bundle, data); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if len(bundle.Roots) != 4096 {
			b.Fatalf("decoded %d roots, want 4096", len(bundle.Roots))
		}
	})

	b.Run("UnmarshalReader", func(b *testing.B) {
		var bundle *SliceVectorBundle
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bundle = new(SliceVectorBundle)
			reader := bytes.NewReader(data)
			if err := ds.UnmarshalSSZReader(bundle, reader, len(data)); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if len(bundle.Roots) != 4096 {
			b.Fatalf("decoded %d roots, want 4096", len(bundle.Roots))
		}
	})

	b.Run("Marshal", func(b *testing.B) {
		bundle := new(SliceVectorBundle)
		if err := ds.UnmarshalSSZ(bundle, data); err != nil {
			b.Fatal(err)
		}
		var result []byte
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var err error
			result, err = ds.MarshalSSZ(bundle)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if !bytes.Equal(result, data) {
			b.Fatal("marshaled data does not match original")
		}
	})

	b.Run("HashTreeRoot", func(b *testing.B) {
		bundle := new(SliceVectorBundle)
		if err := ds.UnmarshalSSZ(bundle, data); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := ds.HashTreeRoot(bundle); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCodegen_SliceVectors(b *testing.B) {
	runSliceVectorBenchmarks(b, dynSszCodegenMainnet, sliceVectorData)
}

func BenchmarkReflection_SliceVectors(b *testing.B) {
	runSliceVectorBenchmarks(b, dynSszReflMainnet, sliceVectorData)
}
