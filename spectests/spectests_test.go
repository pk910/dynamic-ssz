package spectests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/snappy"
	clone "github.com/huandu/go-clone/generic"
	ssz "github.com/pk910/dynamic-ssz"
	require "github.com/stretchr/testify/require"
)

type SpecTestStruct struct {
	name string
	s    any
}

func testForkConsensusSpec(t *testing.T, fork string, preset string, tests []SpecTestStruct) bool {
	var dynssz *ssz.DynSsz
	if preset == "mainnet" {
		dynssz = dynSszOnlyMainnet
	} else {
		dynssz = dynSszOnlyMinimal
	}

	specTestsDir := os.Getenv("CONSENSUS_SPEC_TESTS_DIR")
	if specTestsDir == "" {
		t.Skip("CONSENSUS_SPEC_TESTS_DIR not set")
	}

	baseDir := filepath.Join(specTestsDir, preset, fork, "ssz_static")

	// Check if the fork directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return false
	}

	for _, test := range tests {
		dir := filepath.Join(baseDir, test.name, "ssz_random")

		// Check if the test type directory exists
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Logf("Test type %s not found for fork %s, skipping", test.name, fork)
			continue
		}

		require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if path == dir {
				// Only interested in subdirectories.
				return nil
			}
			require.NoError(t, err)
			if info.IsDir() {
				t.Run(fmt.Sprintf("%s/%s", test.name, info.Name()), func(t *testing.T) {
					// Obtain the struct from the SSZ.
					s2 := clone.Clone(test.s)
					compressedSpecSSZ, err := os.ReadFile(filepath.Join(path, "serialized.ssz_snappy"))
					require.NoError(t, err)
					specSSZ, err := snappy.Decode(nil, compressedSpecSSZ)
					require.NoError(t, err)

					// Unmarshal the SSZ.
					err = dynssz.UnmarshalSSZ(s2, specSSZ)
					require.NoError(t, err)

					// Confirm we can return to the SSZ.
					remarshalledSpecSSZ, err := dynssz.MarshalSSZ(s2)
					require.NoError(t, err)
					require.Equal(t, specSSZ, remarshalledSpecSSZ)

					// Obtain the hash tree root from the YAML.
					specYAMLRoot, err := os.ReadFile(filepath.Join(path, "roots.yaml"))
					require.NoError(t, err)
					// Confirm we calculate the same root.
					generatedRootBytes, err := dynssz.HashTreeRoot(s2)
					require.NoError(t, err)
					generatedRoot := fmt.Sprintf("root: '%#x'\n", string(generatedRootBytes[:]))
					if string(specYAMLRoot) != generatedRoot {
						dynssz.Verbose = true
						fmt.Printf("\n\ngeneratedRoot: %v", generatedRoot)
						fmt.Printf("specYAMLRoot: %v\n", string(specYAMLRoot))
						generatedRootBytes, err = dynssz.HashTreeRoot(s2)
						require.NoError(t, err)

						dynssz.Verbose = false
					}
					require.YAMLEq(t, string(specYAMLRoot), generatedRoot)
				})
			}

			return nil
		}))
	}

	return true
}
