package main

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

type fastsszHashRoot interface {
	HashTreeRoot() ([32]byte, error)
}

func testForkConsensusSpec(t *testing.T, fork string, preset string, tests []SpecTestStruct) {
	var dynssz *ssz.DynSsz
	if preset == "mainnet" {
		dynssz = dynSszOnlyMainnet
	} else {
		dynssz = dynSszOnlyMinimal
	}

	baseDir := filepath.Join(os.Getenv("CONSENSUS_SPEC_TESTS_DIR"), preset, fork, "ssz_static")
	for _, test := range tests {
		dir := filepath.Join(baseDir, test.name, "ssz_random")
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
					var specSSZ []byte
					specSSZ, err = snappy.Decode(specSSZ, compressedSpecSSZ)
					require.NoError(t, err)

					// Unmarshal the SSZ.
					err = dynssz.UnmarshalSSZ(s2, specSSZ)
					if err != nil {
						fmt.Printf("type: %v\n", test.name)
						err = dynssz.UnmarshalSSZ(s2, specSSZ)
					}
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

						s2.(fastsszHashRoot).HashTreeRoot()

						dynssz.Verbose = false
					}
					require.YAMLEq(t, string(specYAMLRoot), generatedRoot)
				})
			}

			return nil
		}))
	}
}
