// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszspecs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/tests/sszspecs/descriptor"
	"github.com/pk910/dynamic-ssz/treeproof"
)

// runFormats names the fixture formats this test runs.
var runFormats = map[string]bool{
	formatSSZ:           true,
	formatProof:         true,
	formatMultiproof:    true,
	formatTypeRejection: true,
}

// The fixture formats this test runs.
const (
	formatSSZ           = "ssz"
	formatProof         = "proof"
	formatMultiproof    = "multiproof"
	formatTypeRejection = "ssz_type_rejection"
)

// notApplicable names the fixture formats that exercise a feature the library
// does not have. Their cases are counted, not run.
var notApplicable = map[string]string{
	"ssz_gindex": "the library resolves no path to a generalized index",
	"ssz_json":   "the library has no SSZ JSON mapping",
}

type indexFile struct {
	Cases []indexCase `json:"cases"`
}

// manifest is what the vector set states about itself.
type manifest struct {
	CaseCount int `json:"caseCount"`
}

type indexCase struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Valid  bool   `json:"valid"`
	SHA256 string `json:"sha256"`
}

// vector is the union of the envelopes the run formats carry.
type vector struct {
	TypeName         string                 `json:"typeName"`
	TypeDescriptor   *descriptor.Descriptor `json:"typeDescriptor"`
	Valid            bool                   `json:"valid"`
	Serialized       string                 `json:"serialized"`
	Root             string                 `json:"root"`
	RejectionReason  string                 `json:"rejectionReason"`
	StricterThanSpec string                 `json:"stricterThanSpec"`

	// Single proofs.
	Index  string    `json:"index"`
	Leaf   *string   `json:"leaf"`
	Branch *[]string `json:"branch"`

	// Multiproofs.
	Indices []string `json:"indices"`
	Leaves  []string `json:"leaves"`
	Proof   []string `json:"proof"`
}

// outcome says how one case ended.
type outcome int

const (
	outcomePassed outcome = iota
	// outcomeStricter is a case the suite refuses beyond what the
	// specification requires, where the library follows the specification.
	outcomeStricter
	// outcomeUnsupported is a case whose legal type the library cannot spell.
	outcomeUnsupported
	// outcomeInexpressible is an illegal declaration Go cannot spell either.
	outcomeInexpressible
	// outcomeDiverges is a known divergence listed in knownDivergences.
	outcomeDiverges
	outcomeNotApplicable
)

var outcomeNames = map[outcome]string{
	outcomePassed:        "passed",
	outcomeStricter:      "spec-faithful where the suite is stricter",
	outcomeUnsupported:   "type not expressible in the library",
	outcomeInexpressible: "illegal type not expressible in Go",
	outcomeDiverges:      "known divergence",
	outcomeNotApplicable: "feature not in the library",
}

// engines are the two implementations every case runs on: the generated SSZ
// methods of the vector types, and the reflection engine walking the same
// types without them.
func engines() map[string]*dynssz.DynSsz {
	return map[string]*dynssz.DynSsz{
		"generated":  dynssz.NewDynSsz(nil),
		"reflection": dynssz.NewDynSsz(nil, dynssz.WithNoFastSsz(), dynssz.WithNoDelegation()),
	}
}

func TestOfficialSSZSpecs(t *testing.T) {
	root := os.Getenv("SSZ_SPECS_TESTS_DIR")
	if root == "" {
		t.Skip("SSZ_SPECS_TESTS_DIR is unset; run ./setup_test_data.sh setup")
	}

	if len(vectorTypes) == 0 {
		t.Fatal("no generated vector types; run ./setup_test_data.sh setup")
	}

	data, err := os.ReadFile(filepath.Join(root, "index.json")) //nolint:gosec // the caller names this read-only test-data tree
	if err != nil {
		t.Fatalf("read index: %v", err)
	}

	var index indexFile
	if err = json.Unmarshal(data, &index); err != nil {
		t.Fatalf("parse index: %v", err)
	}

	// The setup script pins the digest of index.json, and every case below is
	// checked against the digest the index states for it. What is left to
	// rule out is an index that lists fewer cases than the set was generated
	// with, and a format this test has no handling for.
	data, err = os.ReadFile(filepath.Join(root, "manifest.json")) //nolint:gosec // the caller names this read-only test-data tree
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var stated manifest
	if err = json.Unmarshal(data, &stated); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	if stated.CaseCount == 0 || len(index.Cases) != stated.CaseCount {
		t.Fatalf("index lists %d cases, the manifest states %d (wrong or incomplete ssz-specs fixture set)", len(index.Cases), stated.CaseCount)
	}

	dss := engines()
	tally := make(map[string]map[outcome]int, 8)

	for _, c := range index.Cases {
		format := formatOf(c.Path)
		if tally[format] == nil {
			tally[format] = make(map[outcome]int, 4)
		}

		t.Run(c.ID, func(t *testing.T) {
			result := runCase(t, dss, root, format, &c)
			if !t.Failed() {
				tally[format][result]++
			}
		})
	}

	formats := make([]string, 0, len(tally))
	for format := range tally {
		formats = append(formats, format)
	}

	sort.Strings(formats)

	for _, format := range formats {
		for result := outcomePassed; result <= outcomeNotApplicable; result++ {
			if n := tally[format][result]; n > 0 {
				t.Logf("%-18s %4d %s", format, n, outcomeNames[result])
			}
		}
	}
}

func formatOf(path string) string {
	format, _, _ := strings.Cut(path, "/")
	return format
}

func runCase(t *testing.T, dss map[string]*dynssz.DynSsz, root, format string, c *indexCase) outcome {
	t.Helper()

	if _, skip := notApplicable[format]; skip {
		return outcomeNotApplicable
	}

	// A format upstream adds later must not pass by being ignored.
	if !runFormats[format] {
		t.Fatalf("fixture format %q has no runner and is not listed as not applicable", format)
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.Path))) //nolint:gosec // the caller names this read-only test-data tree
	if err != nil {
		t.Fatal(err)
	}

	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != c.SHA256 {
		t.Fatalf("fixture %s does not match the digest its index states", c.Path)
	}

	var v vector
	if err = json.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	if v.Valid != c.Valid {
		t.Fatalf("fixture and index disagree on validity")
	}

	reason, diverges := knownDivergences[c.ID]

	var result outcome

	failed := captureFailure(t, func(tb testing.TB) {
		tb.Helper()

		if format == formatTypeRejection {
			result = runIllegalType(tb, dss, c.ID)
			return
		}

		key, keyErr := v.TypeDescriptor.Key()
		if keyErr != nil {
			tb.Fatalf("descriptor key: %v", keyErr)
		}

		if _, unsupported := unsupportedTypes[key]; unsupported {
			result = outcomeUnsupported
			return
		}

		newValue, ok := vectorTypes[key]
		if !ok {
			tb.Fatalf("no generated Go type for the declaration of %s; rerun ./setup_test_data.sh setup", v.TypeName)
		}

		for name, ds := range dss {
			switch format {
			case formatSSZ:
				result = runSSZ(tb, name, ds, newValue, &v)
			case formatProof:
				result = runProof(tb, name, ds, newValue, &v)
			case formatMultiproof:
				result = runMultiproof(tb, name, ds, newValue, &v)
			default:
				tb.Fatalf("no runner for fixture format %s", format)
			}
		}
	})

	switch {
	case diverges && failed == "":
		t.Fatalf("case is listed as a known divergence (%s) but passes: remove it from knownDivergences", reason)
	case diverges:
		return outcomeDiverges
	case failed != "":
		t.Fatal(failed)
	}

	return result
}

// failure carries the first failure of a captured run.
type failure struct {
	testing.TB
	message string
}

type failNow struct{}

func (f *failure) Fatalf(format string, args ...any) {
	f.message = strings.TrimSpace(strings.ReplaceAll(fmt.Sprintf(format, args...), "\n", " "))
	panic(failNow{})
}

func (f *failure) Fatal(args ...any) {
	f.Fatalf("%s", fmt.Sprint(args...))
}

func (f *failure) Helper() {}

// captureFailure runs fn and returns its first fatal message, or "". A known
// divergence has to fail to stay on the list, so a failure is a value here.
func captureFailure(t *testing.T, fn func(tb testing.TB)) (message string) {
	t.Helper()

	f := &failure{TB: t}

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(failNow); !ok {
				panic(r)
			}

			message = f.message
		}
	}()

	fn(f)

	return ""
}

func runSSZ(tb testing.TB, engine string, ds *dynssz.DynSsz, newValue func() any, v *vector) outcome {
	tb.Helper()

	encoded := decodeHex(tb, v.Serialized)

	if !v.Valid {
		if err := ds.UnmarshalSSZ(newValue(), encoded); err == nil {
			if v.StricterThanSpec != "" {
				return outcomeStricter
			}

			tb.Fatalf("%s: accepted invalid %s bytes (%s)", engine, v.TypeName, v.RejectionReason)
		}

		for _, size := range []int{len(encoded), -1} {
			if err := ds.UnmarshalSSZReader(newValue(), bytes.NewReader(encoded), size); err == nil {
				tb.Fatalf("%s: stream decoder (size %d) accepted invalid %s bytes (%s)", engine, size, v.TypeName, v.RejectionReason)
			}
		}

		return outcomePassed
	}

	value := newValue()
	if err := ds.UnmarshalSSZ(value, encoded); err != nil {
		tb.Fatalf("%s: unmarshal %s: %v", engine, v.TypeName, err)
	}

	remarshaled, err := ds.MarshalSSZ(value)
	if err != nil {
		tb.Fatalf("%s: marshal %s: %v", engine, v.TypeName, err)
	}

	if !bytes.Equal(remarshaled, encoded) {
		tb.Fatalf("%s: round trip %s: got %x, want %x", engine, v.TypeName, remarshaled, encoded)
	}

	if size, sizeErr := ds.SizeSSZ(value); sizeErr != nil || size != len(encoded) {
		tb.Fatalf("%s: size %s: got %d (%v), want %d", engine, v.TypeName, size, sizeErr, len(encoded))
	}

	checkRoot(tb, engine, ds, value, v)

	for _, size := range []int{len(encoded), -1} {
		streamValue := newValue()
		if err = ds.UnmarshalSSZReader(streamValue, bytes.NewReader(encoded), size); err != nil {
			tb.Fatalf("%s: stream unmarshal %s (size %d): %v", engine, v.TypeName, size, err)
		}

		var stream bytes.Buffer
		if err = ds.MarshalSSZWriter(streamValue, &stream); err != nil {
			tb.Fatalf("%s: stream marshal %s: %v", engine, v.TypeName, err)
		}

		if !bytes.Equal(stream.Bytes(), encoded) {
			tb.Fatalf("%s: stream round trip %s (size %d): got %x, want %x", engine, v.TypeName, size, stream.Bytes(), encoded)
		}
	}

	return outcomePassed
}

func checkRoot(tb testing.TB, engine string, ds *dynssz.DynSsz, value any, v *vector) {
	tb.Helper()

	wantRoot := decodeHex(tb, v.Root)

	gotRoot, err := ds.HashTreeRoot(value)
	if err != nil {
		tb.Fatalf("%s: hash tree root %s: %v", engine, v.TypeName, err)
	}

	if !bytes.Equal(gotRoot[:], wantRoot) {
		tb.Fatalf("%s: hash tree root %s: got %x, want %x", engine, v.TypeName, gotRoot, wantRoot)
	}
}

// subjectTree decodes the value a proof case is about and returns its tree.
// Every proof case, valid or not, carries a well-formed subject.
func subjectTree(tb testing.TB, engine string, ds *dynssz.DynSsz, newValue func() any, v *vector) *treeproof.Node {
	tb.Helper()

	value := newValue()
	if err := ds.UnmarshalSSZ(value, decodeHex(tb, v.Serialized)); err != nil {
		tb.Fatalf("%s: unmarshal %s: %v", engine, v.TypeName, err)
	}

	checkRoot(tb, engine, ds, value, v)

	tree, err := ds.GetTree(value)
	if err != nil {
		tb.Fatalf("%s: tree of %s: %v", engine, v.TypeName, err)
	}

	if !bytes.Equal(tree.Hash(), decodeHex(tb, v.Root)) {
		tb.Fatalf("%s: tree root of %s: got %x, want %s", engine, v.TypeName, tree.Hash(), v.Root)
	}

	return tree
}

func runProof(tb testing.TB, engine string, ds *dynssz.DynSsz, newValue func() any, v *vector) outcome {
	tb.Helper()

	tree := subjectTree(tb, engine, ds, newValue, v)
	rootHash := decodeHex(tb, v.Root)
	index := decodeIndex(tb, v.Index)

	// A case without a leaf claims the value's tree holds no node there.
	if v.Leaf == nil || v.Branch == nil {
		if v.Valid {
			tb.Fatalf("%s: valid proof case carries no leaf", engine)
		}

		if node, err := tree.Get(index); err == nil && node != nil {
			if _, proveErr := tree.Prove(index); proveErr == nil {
				tb.Fatalf("%s: tree of %s holds a provable node at index %d, which the value does not reach (%s)", engine, v.TypeName, index, v.RejectionReason)
			}
		}

		return outcomePassed
	}

	claim := &treeproof.Proof{Index: index, Leaf: decodeHex(tb, *v.Leaf), Hashes: decodeHexList(tb, *v.Branch)}
	ok, verifyErr := treeproof.VerifyProof(rootHash, claim)

	if !v.Valid {
		if ok && verifyErr == nil {
			if v.StricterThanSpec != "" {
				return outcomeStricter
			}

			tb.Fatalf("%s: verified an invalid proof for %s at index %d (%s)", engine, v.TypeName, index, v.RejectionReason)
		}

		return outcomePassed
	}

	if verifyErr != nil || !ok {
		tb.Fatalf("%s: valid proof for %s at index %d does not verify (%v)", engine, v.TypeName, index, verifyErr)
	}

	node, err := tree.Get(index)
	if err != nil {
		tb.Fatalf("%s: tree of %s holds no node at index %d: %v", engine, v.TypeName, index, err)
	}

	if !bytes.Equal(node.Hash(), claim.Leaf) {
		tb.Fatalf("%s: node at index %d of %s: got %x, want %x", engine, index, v.TypeName, node.Hash(), claim.Leaf)
	}

	proof, err := tree.Prove(index)
	if err != nil {
		tb.Fatalf("%s: prove index %d of %s: %v", engine, index, v.TypeName, err)
	}

	if !bytes.Equal(proof.Leaf, claim.Leaf) {
		tb.Fatalf("%s: proven leaf at index %d of %s: got %x, want %x", engine, index, v.TypeName, proof.Leaf, claim.Leaf)
	}

	compareNodes(tb, engine+": branch of "+v.TypeName, proof.Hashes, claim.Hashes)

	return outcomePassed
}

func runMultiproof(tb testing.TB, engine string, ds *dynssz.DynSsz, newValue func() any, v *vector) outcome {
	tb.Helper()

	tree := subjectTree(tb, engine, ds, newValue, v)
	rootHash := decodeHex(tb, v.Root)
	leaves := decodeHexList(tb, v.Leaves)
	helpers := decodeHexList(tb, v.Proof)

	indices := make([]int, 0, len(v.Indices))
	for _, index := range v.Indices {
		indices = append(indices, decodeIndex(tb, index))
	}

	ok, verifyErr := treeproof.VerifyMultiproof(rootHash, helpers, leaves, indices)

	if !v.Valid {
		if ok && verifyErr == nil {
			if v.StricterThanSpec != "" {
				return outcomeStricter
			}

			tb.Fatalf("%s: verified an invalid multiproof for %s (%s)", engine, v.TypeName, v.RejectionReason)
		}

		return outcomePassed
	}

	if verifyErr != nil || !ok {
		tb.Fatalf("%s: valid multiproof for %s does not verify (%v)", engine, v.TypeName, verifyErr)
	}

	proof, err := tree.ProveMulti(indices)
	if err != nil {
		tb.Fatalf("%s: prove indices %v of %s: %v", engine, indices, v.TypeName, err)
	}

	compareNodes(tb, engine+": leaves of "+v.TypeName, proof.Leaves, leaves)
	compareNodes(tb, engine+": helper nodes of "+v.TypeName, proof.Hashes, helpers)

	return outcomePassed
}

// runIllegalType requires the library to refuse an illegal declaration. A
// refusal at any point before a value is produced counts.
func runIllegalType(tb testing.TB, dss map[string]*dynssz.DynSsz, id string) outcome {
	tb.Helper()

	if _, inexpressible := inexpressibleTypes[id]; inexpressible {
		return outcomeInexpressible
	}

	illegal, ok := illegalTypes[id]
	if !ok {
		tb.Fatalf("no generated Go type for the illegal declaration; rerun ./setup_test_data.sh setup")
	}

	if err := dss["reflection"].ValidateType(illegal); err == nil {
		tb.Fatalf("the library accepts the illegal declaration %s", illegal)
	}

	return outcomePassed
}

func compareNodes(tb testing.TB, what string, got, want [][]byte) {
	tb.Helper()

	if len(got) != len(want) {
		tb.Fatalf("%s: got %d nodes, want %d", what, len(got), len(want))
	}

	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			tb.Fatalf("%s: node %d: got %x, want %x", what, i, got[i], want[i])
		}
	}
}

func decodeIndex(tb testing.TB, value string) int {
	tb.Helper()

	index, err := strconv.Atoi(value)
	if err != nil {
		tb.Fatalf("generalized index %q: %v", value, err)
	}

	return index
}

func decodeHexList(tb testing.TB, values []string) [][]byte {
	tb.Helper()

	decoded := make([][]byte, 0, len(values))
	for _, value := range values {
		decoded = append(decoded, decodeHex(tb, value))
	}

	return decoded
}

func decodeHex(tb testing.TB, value string) []byte {
	tb.Helper()

	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
	if err != nil {
		tb.Fatalf("decode %q: %v", value, err)
	}

	return decoded
}
