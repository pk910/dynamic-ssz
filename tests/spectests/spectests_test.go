// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package spectests

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/golang/snappy"
	clone "github.com/huandu/go-clone/generic"
	ssz "github.com/pk910/dynamic-ssz"
	require "github.com/stretchr/testify/require"
)

type SpecTestStruct struct {
	name string
	s    any
	s2   any
	s3   []any
}

// unknownSizeVariant pairs a subtest name suffix with the DynSsz instance used
// for the unknown-length streaming round-trip. The stream reader buffer size
// selects which regime of the decoder is exercised.
type unknownSizeVariant struct {
	name   string
	dynssz *ssz.DynSsz
}

// specTestsDirCheck memoises the one-time validation of CONSENSUS_SPEC_TESTS_DIR.
var specTestsDirCheck struct {
	once sync.Once
	err  error
}

// checkSpecTestsDir reports whether CONSENSUS_SPEC_TESTS_DIR points at a
// spec-tests root.
//
// This exists because the failure mode is otherwise invisible: a wrong path
// makes every fork's directory lookup miss, each fork test calls t.Skipf, and a
// package whose tests all skip reports "ok". A misconfigured run then looks
// exactly like a passing one while verifying nothing. The usual mistake is
// including the preset in the path -- the variable must point at
// ".../consensus-spec-tests/tests", not ".../consensus-spec-tests/tests/mainnet",
// because the preset is appended by the tests themselves.
func checkSpecTestsDir(dir string) error {
	specTestsDirCheck.once.Do(func() {
		for _, preset := range []string{"mainnet", "minimal"} {
			matches, _ := filepath.Glob(filepath.Join(dir, preset, "*", "ssz_static"))
			if len(matches) > 0 {
				return
			}
		}
		specTestsDirCheck.err = fmt.Errorf(
			"CONSENSUS_SPEC_TESTS_DIR=%q contains no <preset>/<fork>/ssz_static directories; "+
				"it must point at the spec-tests root (.../consensus-spec-tests/tests), not at a "+
				"preset inside it, since the preset is appended by the tests", dir)
	})
	return specTestsDirCheck.err
}

func runForkConsensusSpecTest(t *testing.T, fork string, preset string, tests []SpecTestStruct) bool {
	var dynssz *ssz.DynSsz
	var unknownSizeVariants []unknownSizeVariant
	if preset == "mainnet" {
		dynssz = dynSszOnlyMainnet
		unknownSizeVariants = []unknownSizeVariant{
			// Default buffer: small payloads are fully buffered on the first
			// fill, so the decoder observes EOF and collapses the stream to a
			// known length.
			{name: "defaultbuf", dynssz: dynSszOnlyMainnet},
			// Tiny buffer: EOF is never visible up front, so every dynamic
			// tail is decoded as a genuine open region.
			{name: "tinybuf", dynssz: dynSszTinyBufMainnet},
		}
	} else {
		dynssz = dynSszOnlyMinimal
		unknownSizeVariants = []unknownSizeVariant{
			{name: "defaultbuf", dynssz: dynSszOnlyMinimal},
			{name: "tinybuf", dynssz: dynSszTinyBufMinimal},
		}
	}

	specTestsDir := os.Getenv("CONSENSUS_SPEC_TESTS_DIR")
	if specTestsDir == "" {
		t.Skip("CONSENSUS_SPEC_TESTS_DIR not set")
	}
	if err := checkSpecTestsDir(specTestsDir); err != nil {
		t.Fatal(err)
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
				runTestWithType := func(name string, s any, opts ...ssz.CallOption) {
					t.Run(fmt.Sprintf("%s/%s/%s:%s", test.name, preset, info.Name(), name), func(t *testing.T) {
						// Obtain the struct from the SSZ.
						s2 := clone.Clone(s)
						compressedSpecSSZ, err := os.ReadFile(filepath.Join(path, "serialized.ssz_snappy"))
						require.NoError(t, err)
						specSSZ, err := snappy.Decode(nil, compressedSpecSSZ)
						require.NoError(t, err)

						// Unmarshal the SSZ.
						err = dynssz.UnmarshalSSZ(s2, specSSZ, opts...)
						require.NoError(t, err)

						// Confirm we can return to the SSZ.
						remarshalledSpecSSZ, err := dynssz.MarshalSSZ(s2, opts...)
						require.NoError(t, err)
						require.Equal(t, specSSZ, remarshalledSpecSSZ)

						// Obtain the hash tree root from the YAML.
						specYAMLRoot, err := os.ReadFile(filepath.Join(path, "roots.yaml"))
						require.NoError(t, err)
						// Confirm we calculate the same root.
						generatedRootBytes, err := dynssz.HashTreeRoot(s2, opts...)
						require.NoError(t, err)
						generatedRoot := fmt.Sprintf("root: '%#x'\n", string(generatedRootBytes[:]))
						if string(specYAMLRoot) != generatedRoot {
							fmt.Printf("\n\ngeneratedRoot: %v", generatedRoot)
							fmt.Printf("specYAMLRoot: %v\n", string(specYAMLRoot))
							require.NoError(t, err)
						}
						require.YAMLEq(t, string(specYAMLRoot), generatedRoot)
					})

					t.Run(fmt.Sprintf("%s/%s/%s-streaming:%s", test.name, preset, info.Name(), name), func(t *testing.T) {
						// Obtain the struct from the SSZ.
						s2 := clone.Clone(s)
						compressedSpecSSZ, err := os.ReadFile(filepath.Join(path, "serialized.ssz_snappy"))
						require.NoError(t, err)
						specSSZ, err := snappy.Decode(nil, compressedSpecSSZ)
						require.NoError(t, err)

						reader := bytes.NewReader(specSSZ)

						// Unmarshal the SSZ.
						err = dynssz.UnmarshalSSZReader(s2, reader, len(specSSZ), opts...)
						require.NoError(t, err)

						// Confirm we can return to the SSZ.
						writer := bytes.NewBuffer(make([]byte, 0, len(specSSZ)))
						err = dynssz.MarshalSSZWriter(s2, writer, opts...)
						require.NoError(t, err)
						require.Equal(t, specSSZ, writer.Bytes(), "specSSZ and writer.Bytes() are not equal")

						// Obtain the hash tree root from the YAML.
						specYAMLRoot, err := os.ReadFile(filepath.Join(path, "roots.yaml"))
						require.NoError(t, err)
						// Confirm we calculate the same root.
						generatedRootBytes, err := dynssz.HashTreeRoot(s2, opts...)
						require.NoError(t, err)
						generatedRoot := fmt.Sprintf("root: '%#x'\n", string(generatedRootBytes[:]))
						if string(specYAMLRoot) != generatedRoot {
							fmt.Printf("\n\ngeneratedRoot: %v", generatedRoot)
							fmt.Printf("specYAMLRoot: %v\n", string(specYAMLRoot))
							require.NoError(t, err)
						}
						require.YAMLEq(t, string(specYAMLRoot), generatedRoot)
					})

					for _, variant := range unknownSizeVariants {
						t.Run(fmt.Sprintf("%s/%s/%s-streaming-unknown-%s:%s", test.name, preset, info.Name(), variant.name, name), func(t *testing.T) {
							// Obtain the struct from the SSZ.
							s2 := clone.Clone(s)
							compressedSpecSSZ, err := os.ReadFile(filepath.Join(path, "serialized.ssz_snappy"))
							require.NoError(t, err)
							specSSZ, err := snappy.Decode(nil, compressedSpecSSZ)
							require.NoError(t, err)

							reader := bytes.NewReader(specSSZ)

							// Unmarshal the SSZ without telling the decoder how long the payload is.
							err = variant.dynssz.UnmarshalSSZReader(s2, reader, -1, opts...)
							require.NoError(t, err)

							// Confirm the whole payload was consumed.
							require.Equal(t, 0, reader.Len(), "reader not fully consumed")

							// Confirm we can return to the SSZ.
							writer := bytes.NewBuffer(make([]byte, 0, len(specSSZ)))
							err = variant.dynssz.MarshalSSZWriter(s2, writer, opts...)
							require.NoError(t, err)
							require.Equal(t, specSSZ, writer.Bytes(), "specSSZ and writer.Bytes() are not equal")

							// Obtain the hash tree root from the YAML.
							specYAMLRoot, err := os.ReadFile(filepath.Join(path, "roots.yaml"))
							require.NoError(t, err)
							// Confirm we calculate the same root.
							generatedRootBytes, err := variant.dynssz.HashTreeRoot(s2, opts...)
							require.NoError(t, err)
							generatedRoot := fmt.Sprintf("root: '%#x'\n", string(generatedRootBytes[:]))
							if string(specYAMLRoot) != generatedRoot {
								fmt.Printf("\n\ngeneratedRoot: %v", generatedRoot)
								fmt.Printf("specYAMLRoot: %v\n", string(specYAMLRoot))
								require.NoError(t, err)
							}
							require.YAMLEq(t, string(specYAMLRoot), generatedRoot)
						})
					}
				}

				runTestWithType("reflection", test.s)
				if test.s2 != nil {
					runTestWithType("codegen", test.s2)
				}
				if test.s3 != nil {
					runTestWithType("codegen+views", test.s3[0], ssz.WithViewDescriptor(test.s3[1]))
				}
			}

			return nil
		}))
	}

	return true
}
