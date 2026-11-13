package certlib

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// CertJSON renders the descriptor as a declaration for the in-house oracle
// (`T <name> <json>`, see formal/lean/SszCert/Declare.lean): the ssz-specs
// JSON spelling SpecJSON writes, extended with dynamic-ssz's kinds —
// `Union` (a None option is a null type at selector 0), `Optional`, `BigInt`
// (`limit` absent when unbounded), and the signed integers, floats and
// `time.Time` as the unsigned widths they encode as. It contains no
// whitespace. It describes exactly the codec CertExpr writes as a Lean term.
func CertJSON(desc *ssztypes.TypeDescriptor) (string, error) {
	if isRecursive(desc) {
		return "", ErrRecursive
	}
	switch desc.SszType {
	case ssztypes.SszBoolType:
		return `{"kind":"Boolean"}`, nil
	case ssztypes.SszUint8Type, ssztypes.SszInt8Type:
		return `{"kind":"Uint8","bits":8}`, nil
	case ssztypes.SszUint16Type, ssztypes.SszInt16Type:
		return `{"kind":"Uint16","bits":16}`, nil
	case ssztypes.SszUint32Type, ssztypes.SszInt32Type, ssztypes.SszFloat32Type:
		return `{"kind":"Uint32","bits":32}`, nil
	case ssztypes.SszUint64Type, ssztypes.SszInt64Type, ssztypes.SszFloat64Type:
		return `{"kind":"Uint64","bits":64}`, nil
	case ssztypes.SszUint128Type:
		return `{"kind":"Uint128","bits":128}`, nil
	case ssztypes.SszUint256Type:
		return `{"kind":"Uint256","bits":256}`, nil

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return containerJSON(desc, CertJSON)

	case ssztypes.SszVectorType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("vector without element descriptor")
		}
		if desc.Len <= 0 {
			return "", fmt.Errorf("vector with non-positive length %d", desc.Len)
		}
		if desc.ElemDesc.SszType == ssztypes.SszUint8Type {
			return fmt.Sprintf(`{"kind":"ByteVector","length":%d}`, desc.Len), nil
		}
		e, err := CertJSON(desc.ElemDesc)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"kind":"Vector","length":%d,"elementType":%s}`, desc.Len, e), nil

	case ssztypes.SszListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("list without element descriptor")
		}
		return listCertJSON(desc.ElemDesc, limitJSON(desc))

	case ssztypes.SszOptionalListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("optional list without element descriptor")
		}
		return listCertJSON(desc.ElemDesc, `"limit":1`)

	case ssztypes.SszProgressiveListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("progressive list without element descriptor")
		}
		if desc.ElemDesc.SszType == ssztypes.SszUint8Type {
			return `{"kind":"ProgressiveList"` + optLimitJSON(desc) + `,"elementType":{"kind":"Uint8","bits":8}}`, nil
		}
		e, err := CertJSON(desc.ElemDesc)
		if err != nil {
			return "", err
		}
		return `{"kind":"ProgressiveList"` + optLimitJSON(desc) + `,"elementType":` + e + `}`, nil

	case ssztypes.SszBitvectorType:
		bits := desc.BitSize
		if bits <= 0 {
			bits = desc.Size * 8
		}
		if bits <= 0 {
			return "", fmt.Errorf("bitvector with unknown bit size")
		}
		return fmt.Sprintf(`{"kind":"BitVector","length":%d}`, bits), nil

	case ssztypes.SszBitlistType:
		return `{"kind":"BitList",` + limitJSON(desc) + `}`, nil

	case ssztypes.SszProgressiveBitlistType:
		return `{"kind":"ProgressiveBitList"` + optLimitJSON(desc) + `}`, nil

	case ssztypes.SszCompatibleUnionType:
		if len(desc.UnionVariants) == 0 {
			return "", fmt.Errorf("compatible union without variants")
		}
		opts, err := unionVariantExprs(desc, CertJSON, `{"selector":%d,"type":%s}`)
		if err != nil {
			return "", err
		}
		return `{"kind":"CompatibleUnion","options":[` + strings.Join(opts, ",") + `]}`, nil

	case ssztypes.SszUnionType:
		hasNone := desc.SszTypeFlags&ssztypes.SszTypeFlagHasNoneVariant != 0
		if len(desc.UnionVariants) == 0 && !hasNone {
			return "", fmt.Errorf("union without variants")
		}
		opts, err := unionVariantExprs(desc, CertJSON, `{"selector":%d,"type":%s}`)
		if err != nil {
			return "", err
		}
		if hasNone {
			opts = append([]string{`{"selector":0,"type":null}`}, opts...)
		}
		return `{"kind":"Union","options":[` + strings.Join(opts, ",") + `]}`, nil

	case ssztypes.SszOptionalType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("optional without element descriptor")
		}
		e, err := CertJSON(desc.ElemDesc)
		if err != nil {
			return "", err
		}
		return `{"kind":"Optional","elementType":` + e + `}`, nil

	case ssztypes.SszBigIntType:
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 {
			return fmt.Sprintf(`{"kind":"BigInt","limit":%d}`, desc.Limit), nil
		}
		return `{"kind":"BigInt"}`, nil

	case ssztypes.SszTypeWrapperType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("type wrapper without wrapped descriptor")
		}
		return CertJSON(desc.ElemDesc)
	default:
	}
	return "", fmt.Errorf("unsupported SSZ type %v (custom types carry no certifiable semantics)", desc.SszType)
}

// limitJSON renders a list's `"limit":N` member, absent for an unbounded one.
func limitJSON(desc *ssztypes.TypeDescriptor) string {
	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 {
		return fmt.Sprintf(`"limit":%d`, desc.Limit)
	}
	return `"limit":null`
}

func listCertJSON(elem *ssztypes.TypeDescriptor, limit string) (string, error) {
	if unwrap(elem).SszType == ssztypes.SszUint8Type {
		return `{"kind":"ByteList",` + limit + `}`, nil
	}
	e, err := CertJSON(elem)
	if err != nil {
		return "", err
	}
	return `{"kind":"List",` + limit + `,"elementType":` + e + `}`, nil
}

var _ = sort.Ints
