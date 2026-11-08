package certlib

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// specDescExpr translates an SSZ type descriptor into a term of EthLib's
// `Desc` universe (github.com/ethereum/EthLib, `EthLib.Serialization.Ssz`), the
// official Lean specification of SSZ. Every well-formed `Desc` inherits
// EthLib's generic theorems (codec canonicality, roundtrip, injectivity,
// root totality, proof correctness), so a certified type needs only the
// `wellFormed` fact to instantiate them.
func SpecExpr(desc *ssztypes.TypeDescriptor) (string, error) {
	switch desc.SszType {
	case ssztypes.SszBoolType:
		return ".bool", nil
	case ssztypes.SszUint8Type:
		return "(.uint 1)", nil
	case ssztypes.SszUint16Type:
		return "(.uint 2)", nil
	case ssztypes.SszUint32Type:
		return "(.uint 4)", nil
	case ssztypes.SszUint64Type:
		return "(.uint 8)", nil
	case ssztypes.SszUint128Type:
		return "(.uint 16)", nil
	case ssztypes.SszUint256Type:
		return "(.uint 32)", nil

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		if desc.ContainerDesc == nil {
			return "", fmt.Errorf("container without field descriptors")
		}
		names := make([]string, 0, len(desc.ContainerDesc.Fields))
		fields := make([]string, 0, len(desc.ContainerDesc.Fields))
		maxIdx := -1
		for i := range desc.ContainerDesc.Fields {
			f := &desc.ContainerDesc.Fields[i]
			e, err := SpecExpr(f.Type)
			if err != nil {
				return "", fmt.Errorf("field %s: %w", f.Name, err)
			}
			names = append(names, strconv.Quote(f.Name))
			fields = append(fields, e)
			if idx := int(f.SszIndex); idx > maxIdx {
				maxIdx = idx
			}
		}
		if desc.SszType == ssztypes.SszProgressiveContainerType {
			layout := make([]string, maxIdx+1)
			for i := range layout {
				layout[i] = "false"
			}
			for i := range desc.ContainerDesc.Fields {
				layout[desc.ContainerDesc.Fields[i].SszIndex] = "true"
			}
			return "(.progressiveContainer [" + strings.Join(layout, ", ") + "] [" +
				strings.Join(names, ", ") + "] [" + strings.Join(fields, ", ") + "])", nil
		}
		return "(.container [" + strings.Join(names, ", ") + "] [" + strings.Join(fields, ", ") + "])", nil

	case ssztypes.SszVectorType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("vector without element descriptor")
		}
		if desc.Len <= 0 {
			return "", fmt.Errorf("vector with non-positive length %d", desc.Len)
		}
		if unwrap(desc.ElemDesc).SszType == ssztypes.SszUint8Type {
			return fmt.Sprintf("(.byteVector %d)", desc.Len), nil
		}
		e, err := SpecExpr(desc.ElemDesc)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(.vector %s %d)", e, desc.Len), nil

	case ssztypes.SszListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("list without element descriptor")
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
			return "", fmt.Errorf("list without a declared limit has no SSZ type")
		}
		return listSpecExpr(desc.ElemDesc, desc.Limit)

	case ssztypes.SszOptionalListType:
		// A pointer encoded as the canonical List[T, 1].
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("optional list without element descriptor")
		}
		return listSpecExpr(desc.ElemDesc, 1)

	case ssztypes.SszTypeWrapperType:
		// A wrapper is transparent: it encodes and roots as the wrapped type.
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("type wrapper without wrapped descriptor")
		}
		return SpecExpr(desc.ElemDesc)

	case ssztypes.SszUnionType, ssztypes.SszOptionalType, ssztypes.SszBigIntType,
		ssztypes.SszInt8Type, ssztypes.SszInt16Type, ssztypes.SszInt32Type, ssztypes.SszInt64Type,
		ssztypes.SszFloat32Type, ssztypes.SszFloat64Type:
		return "", fmt.Errorf("%w: %v", ErrNoSpec, desc.SszType)

	case ssztypes.SszProgressiveListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("progressive list without element descriptor")
		}
		e, err := SpecExpr(desc.ElemDesc)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(.progressiveList %s %s)", e, optLimitExpr(desc)), nil

	case ssztypes.SszBitvectorType:
		bits := desc.BitSize
		if bits <= 0 {
			bits = desc.Size * 8
		}
		if bits <= 0 {
			return "", fmt.Errorf("bitvector with unknown bit size")
		}
		return fmt.Sprintf("(.bitVector %d)", bits), nil

	case ssztypes.SszBitlistType:
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
			return "", fmt.Errorf("bitlist without a declared limit has no SSZ type")
		}
		return fmt.Sprintf("(.bitList %d)", desc.Limit), nil

	case ssztypes.SszProgressiveBitlistType:
		return fmt.Sprintf("(.progressiveBitList %s)", optLimitExpr(desc)), nil

	case ssztypes.SszCompatibleUnionType:
		if len(desc.UnionVariants) == 0 {
			return "", fmt.Errorf("compatible union without variants")
		}
		selectors := make([]int, 0, len(desc.UnionVariants))
		for sel := range desc.UnionVariants {
			selectors = append(selectors, int(sel))
		}
		sort.Ints(selectors)
		sels := make([]string, 0, len(selectors))
		opts := make([]string, 0, len(selectors))
		for _, sel := range selectors {
			e, err := SpecExpr(desc.UnionVariants[uint8(sel)])
			if err != nil {
				return "", fmt.Errorf("union variant %d: %w", sel, err)
			}
			sels = append(sels, strconv.Itoa(sel))
			opts = append(opts, e)
		}
		return "(.compatibleUnion [" + strings.Join(sels, ", ") + "] [" + strings.Join(opts, ", ") + "])", nil
	default:
		// Extended and legacy kinds have no specification to certify against.
	}
	return "", fmt.Errorf("unsupported SSZ type %v (EthLib covers the spec-defined SSZ surface)", desc.SszType)
}

// ErrNoSpec marks a type outside the SSZ specification (dynamic-ssz extended
// types); such types are certified against the in-house model only.
var ErrNoSpec = errors.New("not an SSZ specification type")

// listSpecExpr renders List[elem, limit]: a byte list for byte elements.
func listSpecExpr(elem *ssztypes.TypeDescriptor, limit uint64) (string, error) {
	if unwrap(elem).SszType == ssztypes.SszUint8Type {
		return fmt.Sprintf("(.byteList %d)", limit), nil
	}
	e, err := SpecExpr(elem)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(.list %s %d)", e, limit), nil
}

// optLimitExpr renders the optional bound of a progressive collection.
func optLimitExpr(desc *ssztypes.TypeDescriptor) string {
	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 {
		return fmt.Sprintf("(some %d)", desc.Limit)
	}
	return "none"
}

// SpecEntry is one certified declaration: the Go package and type name and
// its EthLib `Desc` term.
type SpecEntry struct {
	PkgName, Name, Expr string
}

// writeSpecFiles emits the EthLib-anchored certificate files: the `Desc`
// declarations with a name-indexed registry (consumed by the executable
// oracle; depends only on the executable spec) and the per-type theorem
// instances (depend on EthLib's proof library).
func WriteSpecFiles(entries []SpecEntry, descFile, theoremFile string) error {
	return WriteSpecFilesAs(entries, descFile, theoremFile, "declared", "SszSpec.Certified")
}

// WriteSpecFilesAs is WriteSpecFiles with the registry definition named
// registry; the theorem file imports the declarations as module declModule.
func WriteSpecFilesAs(entries []SpecEntry, descFile, theoremFile, registry, declModule string) error {
	var out strings.Builder
	out.WriteString("import EthLib.Serialization.Ssz\n\n")
	out.WriteString("/-! SSZ declarations of certified Go types, in the official Lean specification's\n")
	out.WriteString("    type universe (`EthLib.Serialization.Ssz.Desc`). Generated by dynssz-certify — do not edit. -/\n\n")
	out.WriteString("namespace SszSpec\n\nopen EthLib.Serialization.Ssz\n\n")
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		fmt.Fprintf(&out, "/-- SSZ declaration of `%s.%s`. -/\n", e.PkgName, e.Name)
		fmt.Fprintf(&out, "def D_%s : Desc :=\n  %s\n\n", e.Name, e.Expr)
		rows = append(rows, fmt.Sprintf("  (%q, D_%s)", e.Name, e.Name))
	}
	out.WriteString("/-- Name-indexed declaration registry (for the oracle CLI). -/\n")
	fmt.Fprintf(&out, "def %s : List (String × Desc) := [\n", registry)
	out.WriteString(strings.Join(rows, ",\n"))
	out.WriteString("]\n\nend SszSpec\n")
	if err := os.WriteFile(descFile, []byte(out.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", descFile, err)
	}

	if theoremFile == "" {
		return nil
	}
	var th strings.Builder
	th.WriteString("import " + declModule + "\n")
	th.WriteString("import SszSpec.WellFormed\n")
	th.WriteString("import EthLib.Serialization.Proofs.Ssz.Codec.Canonicality\n")
	th.WriteString("import EthLib.Serialization.Proofs.Ssz.Codec.RoundTrip\n")
	th.WriteString("import EthLib.Serialization.Proofs.Ssz.Codec.DecodeFits\n")
	th.WriteString("import EthLib.Serialization.Proofs.Ssz.Codec.RootDomain\n")
	th.WriteString("import EthLib.Serialization.Proofs.Ssz.Codec.ProofCorrectness\n\n")
	th.WriteString("/-! Per-type instances of EthLib's generic SSZ theorems. Generated by dynssz-certify — do not edit.\n\n")
	th.WriteString("    Each certified declaration is proved well-formed by evaluation; EthLib's\n")
	th.WriteString("    machine-checked results then apply to it by instantiation. -/\n\n")
	th.WriteString("-- Wide layouts need a deep evaluation stack in the instance proofs as well.\n")
	th.WriteString("set_option maxRecDepth 100000\n\n")
	th.WriteString("namespace SszSpec\n\nopen EthLib.Serialization.Ssz\n\n")
	for _, e := range entries {
		n := e.Name
		fmt.Fprintf(&th, "/-- `%s` is a well-formed SSZ declaration (by evaluation, `ssz_wf`). -/\n", n)
		fmt.Fprintf(&th, "theorem D_%s_wf : D_%s.wellFormed = .ok () := by ssz_wf D_%s\n\n", n, n, n)
		fmt.Fprintf(&th, "/-- Decode-canonicity for `%s`: every accepted byte string is exactly the\n    serialization of its decoded value. -/\n", n)
		fmt.Fprintf(&th, "theorem D_%s_canonical {data : Bytes} {value : Value}\n", n)
		fmt.Fprintf(&th, "    (read : deserialize D_%s data = .ok value) : serialize D_%s value = .ok data :=\n", n, n)
		fmt.Fprintf(&th, "  serialize_of_deserialize D_%s D_%s_wf data value read\n\n", n, n)
		fmt.Fprintf(&th, "/-- Roundtrip for `%s`: every encoding decodes back to the value it encodes. -/\n", n)
		fmt.Fprintf(&th, "theorem D_%s_roundTrip {value : Value} {bytes : Bytes}\n", n)
		fmt.Fprintf(&th, "    (wrote : serialize D_%s value = .ok bytes) : deserialize D_%s bytes = .ok value :=\n", n, n)
		fmt.Fprintf(&th, "  roundTrip_of_encoding D_%s_wf wrote\n\n", n)
		fmt.Fprintf(&th, "/-- Non-malleability for `%s`: no value has two accepted encodings. -/\n", n)
		fmt.Fprintf(&th, "theorem D_%s_injective {left right : Bytes} {value : Value}\n", n)
		fmt.Fprintf(&th, "    (first : deserialize D_%s left = .ok value) (second : deserialize D_%s right = .ok value) :\n", n, n)
		fmt.Fprintf(&th, "    left = right :=\n  deserialize_injective D_%s_wf first second\n\n", n)
		fmt.Fprintf(&th, "/-- Every value `%s` decodes fits its declaration. -/\n", n)
		fmt.Fprintf(&th, "theorem D_%s_fits {data : Bytes} {value : Value}\n", n)
		fmt.Fprintf(&th, "    (read : deserialize D_%s data = .ok value) : Fits D_%s value :=\n", n, n)
		fmt.Fprintf(&th, "  fits_of_deserialize D_%s data value read\n\n", n)
		fmt.Fprintf(&th, "/-- Every admissible `%s` value has a 32-byte hash tree root. -/\n", n)
		fmt.Fprintf(&th, "theorem D_%s_root {value : Value} (fitted : Fits D_%s value) :\n", n, n)
		fmt.Fprintf(&th, "    ∃ root, hashTreeRoot D_%s value = .ok root ∧ root.size = bytesPerChunk :=\n", n)
		fmt.Fprintf(&th, "  hashTreeRoot_total D_%s value D_%s_wf fitted\n\n", n, n)
		fmt.Fprintf(&th, "/-- Merkle proofs for `%s`: every readable node has a branch that verifies\n    against the value's root. -/\n", n)
		fmt.Fprintf(&th, "theorem D_%s_proof {value : Value} (fitted : Fits D_%s value) (index : Nat) {leaf : Bytes}\n", n, n)
		fmt.Fprintf(&th, "    (readLeaf : nodeRoot D_%s value index = .ok leaf) {indices : List Nat}\n", n)
		fmt.Fprintf(&th, "    (indexed : getBranchIndices index = .ok indices) :\n")
		fmt.Fprintf(&th, "    ∃ root branch, hashTreeRoot D_%s value = .ok root ∧\n", n)
		fmt.Fprintf(&th, "      buildProof D_%s value index = .ok branch ∧ verifyMerkleProof leaf branch index root = .ok true :=\n", n)
		fmt.Fprintf(&th, "  buildProof_fits_verifies D_%s value D_%s_wf fitted index leaf readLeaf indices indexed\n\n", n, n)
	}
	th.WriteString("end SszSpec\n")
	if err := os.WriteFile(theoremFile, []byte(th.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", theoremFile, err)
	}
	return nil
}
