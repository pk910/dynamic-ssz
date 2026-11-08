// Package certlib translates dynamic-ssz type descriptors into Lean 4
// certificate terms: codec expressions over the verified combinator library
// (formal/lean, SszCert) and declarations in the official specification's type
// universe (ethereum/EthLib, formal/spec). It is shared by the source-level
// certifier (dynssz-certify, go/types descriptors as dynssz-gen sees them) and
// the runtime certifier (reflection descriptors with resolved spec values), so
// both write a descriptor down identically.
package certlib

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// leanExpr translates an SSZ type descriptor into an expression over the
// verified codec combinators.
func CertExpr(desc *ssztypes.TypeDescriptor) (string, error) {
	switch desc.SszType {
	case ssztypes.SszBoolType:
		return "boolCodec", nil
	case ssztypes.SszUint8Type:
		return "(uintCodec 1)", nil
	case ssztypes.SszUint16Type:
		return "(uintCodec 2)", nil
	case ssztypes.SszUint32Type:
		return "(uintCodec 4)", nil
	case ssztypes.SszUint64Type:
		return "(uintCodec 8)", nil
	case ssztypes.SszUint128Type:
		return "(uintCodec 16)", nil
	case ssztypes.SszUint256Type:
		return "(uintCodec 32)", nil

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		// Progressive containers serialize exactly like plain containers of
		// their declared fields; the ssz-index layout only affects
		// merkleization (progressive spine + active-fields mix-in).
		if desc.ContainerDesc == nil {
			return "", fmt.Errorf("container without field descriptors")
		}
		fields := make([]string, 0, len(desc.ContainerDesc.Fields))
		maxIdx := -1
		for i := range desc.ContainerDesc.Fields {
			f, err := CertExpr(desc.ContainerDesc.Fields[i].Type)
			if err != nil {
				return "", fmt.Errorf("field %s: %w", desc.ContainerDesc.Fields[i].Name, err)
			}
			fields = append(fields, f)
			if idx := int(desc.ContainerDesc.Fields[i].SszIndex); idx > maxIdx {
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
			return "(progContainerCodec [" + strings.Join(fields, ", ") +
				"] [" + strings.Join(layout, ", ") + "])", nil
		}
		return "(containerCodec [" + strings.Join(fields, ", ") + "])", nil

	case ssztypes.SszVectorType:
		elem := desc.ElemDesc
		if elem == nil {
			return "", fmt.Errorf("vector without element descriptor")
		}
		n := desc.Len
		if n <= 0 {
			return "", fmt.Errorf("vector with non-positive length %d", n)
		}
		if elem.SszType == ssztypes.SszUint8Type {
			return fmt.Sprintf("(byteVectorCodec %d)", n), nil
		}
		e, err := CertExpr(elem)
		if err != nil {
			return "", err
		}
		if isFixedSize(elem) {
			if elem.Size <= 0 {
				return "", fmt.Errorf("fixed-size vector element with size %d", elem.Size)
			}
			if isBasic(elem) {
				return fmt.Sprintf("(vectorPackedCodec %s %d %d (by rfl))", e, elem.Size, n), nil
			}
			return fmt.Sprintf("(vectorFixedCodec %s %d %d (by rfl))", e, elem.Size, n), nil
		}
		return fmt.Sprintf("(vectorVarCodec %s %d)", e, n), nil

	case ssztypes.SszListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("list without element descriptor")
		}
		return listCertExpr(desc.ElemDesc, limitExpr(desc))

	case ssztypes.SszOptionalListType:
		// A pointer encoded as the canonical List[T, 1].
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("optional list without element descriptor")
		}
		return listCertExpr(desc.ElemDesc, "1")

	case ssztypes.SszProgressiveListType:
		// Progressive lists share the plain list wire format; the
		// progressive spine changes only merkleization.
		elem := desc.ElemDesc
		if elem == nil {
			return "", fmt.Errorf("progressive list without element descriptor")
		}
		if elem.SszType == ssztypes.SszUint8Type {
			return "(progListPackedCodec (uintCodec 1) 1 (by rfl) (by decide))", nil
		}
		e, err := CertExpr(elem)
		if err != nil {
			return "", err
		}
		if isFixedSize(elem) {
			if elem.Size <= 0 {
				return "", fmt.Errorf("fixed-size list element with size %d", elem.Size)
			}
			if isBasic(elem) {
				return fmt.Sprintf("(progListPackedCodec %s %d (by rfl) (by decide))", e, elem.Size), nil
			}
			return fmt.Sprintf("(progListFixedCodec %s %d (by rfl) (by decide))", e, elem.Size), nil
		}
		return fmt.Sprintf("(progListVarCodec %s)", e), nil

	case ssztypes.SszBitvectorType:
		bits := desc.BitSize
		if bits <= 0 {
			bits = desc.Size * 8
		}
		if bits <= 0 {
			return "", fmt.Errorf("bitvector with unknown bit size")
		}
		return fmt.Sprintf("(bitvectorCodec %d)", bits), nil

	case ssztypes.SszBitlistType:
		return fmt.Sprintf("(bitlistCodec %s)", limitExpr(desc)), nil

	case ssztypes.SszProgressiveBitlistType:
		return "progBitlistCodec", nil

	case ssztypes.SszCompatibleUnionType:
		if len(desc.UnionVariants) == 0 {
			return "", fmt.Errorf("compatible union without variants")
		}
		variants, err := unionVariantExprs(desc, CertExpr, "(%d, %s)")
		if err != nil {
			return "", err
		}
		return "(unionCodec [" + strings.Join(variants, ", ") + "])", nil

	// Extended types (formal/lean/SszCert/Extended.lean). Signed integers and
	// floats encode and root exactly as uintN over their bit pattern.
	case ssztypes.SszInt8Type:
		return "(uintCodec 1)", nil
	case ssztypes.SszInt16Type:
		return "(uintCodec 2)", nil
	case ssztypes.SszInt32Type, ssztypes.SszFloat32Type:
		return "(uintCodec 4)", nil
	case ssztypes.SszInt64Type, ssztypes.SszFloat64Type:
		return "(uintCodec 8)", nil

	case ssztypes.SszBigIntType:
		return fmt.Sprintf("(bigIntCodec %s)", limitExpr(desc)), nil

	case ssztypes.SszOptionalType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("optional without element descriptor")
		}
		e, err := CertExpr(desc.ElemDesc)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(optionalCodec %s)", e), nil

	case ssztypes.SszUnionType:
		hasNone := desc.SszTypeFlags&ssztypes.SszTypeFlagHasNoneVariant != 0
		if len(desc.UnionVariants) == 0 && !hasNone {
			return "", fmt.Errorf("union without variants")
		}
		variants, err := unionVariantExprs(desc, CertExpr, "(%d, %s)")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(classicUnionCodec %t [%s])", hasNone, strings.Join(variants, ", ")), nil

	case ssztypes.SszTypeWrapperType:
		// A wrapper is transparent: it encodes and roots as the wrapped type.
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("type wrapper without wrapped descriptor")
		}
		return CertExpr(desc.ElemDesc)

	default:
		// Custom types (user-provided methods) have no semantics to certify.
	}

	return "", fmt.Errorf("unsupported SSZ type %v (custom types carry no certifiable semantics)", desc.SszType)
}

// isBasic reports whether a descriptor denotes an SSZ basic type (uintN or
// boolean) — the element kinds whose sequences merkleize packed.
func isBasic(d *ssztypes.TypeDescriptor) bool {
	d = unwrap(d)
	return d.SszType.IsBasic()
}

// unwrap strips type wrappers, which are transparent to SSZ.
func unwrap(d *ssztypes.TypeDescriptor) *ssztypes.TypeDescriptor {
	for d.SszType == ssztypes.SszTypeWrapperType && d.ElemDesc != nil {
		d = d.ElemDesc
	}
	return d
}

// listCertExpr renders a List[elem, limit] codec: byte lists, packed basic
// elements, fixed-size composite elements or variable-size elements.
func listCertExpr(elem *ssztypes.TypeDescriptor, limit string) (string, error) {
	if unwrap(elem).SszType == ssztypes.SszUint8Type {
		return fmt.Sprintf("(byteListCodec %s)", limit), nil
	}
	e, err := CertExpr(elem)
	if err != nil {
		return "", err
	}
	if isFixedSize(elem) {
		if elem.Size <= 0 {
			return "", fmt.Errorf("fixed-size list element with size %d", elem.Size)
		}
		if isBasic(elem) {
			return fmt.Sprintf("(listPackedCodec %s %d %s (by rfl) (by decide))", e, elem.Size, limit), nil
		}
		return fmt.Sprintf("(listFixedCodec %s %d %s (by rfl) (by decide))", e, elem.Size, limit), nil
	}
	return fmt.Sprintf("(listVarCodec %s %s)", e, limit), nil
}

// unionVariantExprs renders a union's variants in selector order with the
// given translation and per-variant format (selector, expression).
func unionVariantExprs(desc *ssztypes.TypeDescriptor, tr func(*ssztypes.TypeDescriptor) (string, error), format string) ([]string, error) {
	selectors := make([]int, 0, len(desc.UnionVariants))
	for sel := range desc.UnionVariants {
		selectors = append(selectors, int(sel))
	}
	sort.Ints(selectors)
	variants := make([]string, 0, len(selectors))
	for _, sel := range selectors {
		e, err := tr(desc.UnionVariants[uint8(sel)])
		if err != nil {
			return nil, fmt.Errorf("union variant %d: %w", sel, err)
		}
		variants = append(variants, fmt.Sprintf(format, sel, e))
	}
	return variants, nil
}

// isFixedSize reports whether a descriptor denotes a fixed-size SSZ type
// (the authoritative signal is the IsDynamic flag, not the Size field).
func isFixedSize(d *ssztypes.TypeDescriptor) bool {
	return d.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0
}

// limitExpr renders a list/bitlist limit; types without a declared limit
// (progressive collections) get the unreachable stand-in bound.
func limitExpr(desc *ssztypes.TypeDescriptor) string {
	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 {
		return strconv.FormatUint(desc.Limit, 10)
	}
	return "noLimit"
}

// CertEntry is one certified type: the Go package and type name and its codec
// expression over the SszCert combinators.
type CertEntry struct {
	PkgName, Name, Expr string
}

// WriteCertFile emits the SszCert certificate file: per type the codec
// definition, its decode-canonicity, roundtrip and tree/root theorems (each an
// instance of the codec's proof fields), and a name-indexed registry for the
// oracle CLI.
func WriteCertFile(entries []CertEntry, outputFile string) error {
	return WriteCertFileAs(entries, outputFile, "certified")
}

// WriteCertFileAs is WriteCertFile with the registry definition named
// registry (several certificate files feed one oracle).
func WriteCertFileAs(entries []CertEntry, outputFile, registry string) error {
	var out strings.Builder
	out.WriteString("import SszCert.Codec\nimport SszCert.Extended\n\n")
	out.WriteString("/-! Machine-checked SSZ certificates. Generated by dynssz-certify — do not edit. -/\n\n")
	out.WriteString("namespace Ssz\n\n")
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name
		fmt.Fprintf(&out, "/-- Certificate for `%s.%s`. -/\n", e.PkgName, name)
		fmt.Fprintf(&out, "def C_%s : Codec :=\n  %s\n\n", name, e.Expr)
		fmt.Fprintf(&out, "/-- Decode-canonicity (P2) for `%s`: every accepted byte string is\n", name)
		fmt.Fprintf(&out, "    exactly the canonical serialization of its decoded value. -/\n")
		fmt.Fprintf(&out, "theorem C_%s_canonicity {bs : List UInt8} {v : Val}\n", name)
		fmt.Fprintf(&out, "    (h : C_%s.dec bs = some v) : C_%s.wt v ∧ C_%s.ser v = bs :=\n", name, name, name)
		fmt.Fprintf(&out, "  C_%s.canon h\n\n", name)
		fmt.Fprintf(&out, "/-- Roundtrip (P1) for `%s`. -/\n", name)
		fmt.Fprintf(&out, "theorem C_%s_roundtrip {v : Val} (hwt : C_%s.wt v) (hrep : C_%s.rep v) :\n", name, name, name)
		fmt.Fprintf(&out, "    C_%s.dec (C_%s.ser v) = some v :=\n", name, name)
		fmt.Fprintf(&out, "  C_%s.round hwt hrep\n\n", name)
		fmt.Fprintf(&out, "/-- Tree/root agreement for `%s`: hashing the explicit Merkle tree\n", name)
		fmt.Fprintf(&out, "    yields exactly the direct hash-tree-root. -/\n")
		fmt.Fprintf(&out, "theorem C_%s_tree_root {v : Val} (hwt : C_%s.wt v) :\n", name, name)
		fmt.Fprintf(&out, "    (C_%s.tree v).root = C_%s.htr v :=\n", name, name)
		fmt.Fprintf(&out, "  C_%s.htr_tree hwt\n\n", name)
		rows = append(rows, fmt.Sprintf("  (%q, C_%s)", name, name))
	}
	out.WriteString("/-- Name-indexed certificate registry (for the oracle CLI). -/\n")
	fmt.Fprintf(&out, "def %s : List (String × Codec) := [\n", registry)
	out.WriteString(strings.Join(rows, ",\n"))
	out.WriteString("]\n\nend Ssz\n")
	if err := os.WriteFile(outputFile, []byte(out.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputFile, err)
	}
	return nil
}
