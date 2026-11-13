package certlib

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// SpecJSON renders the SSZ type descriptor as a declaration in the JSON
// spelling of the ethereum/ssz-specs conformance vectors, the form the
// spec oracle reads at run time (`T <name> <json>`, see
// formal/spec/SszSpec/Declare.lean). It describes exactly the `Desc` SpecExpr
// writes as a Lean term, and contains no whitespace. Types outside the SSZ
// specification return ErrNoSpec, as SpecExpr does.
func SpecJSON(desc *ssztypes.TypeDescriptor) (string, error) {
	switch desc.SszType {
	case ssztypes.SszBoolType:
		return `{"kind":"Boolean"}`, nil
	case ssztypes.SszUint8Type:
		return `{"kind":"Uint8","bits":8}`, nil
	case ssztypes.SszUint16Type:
		return `{"kind":"Uint16","bits":16}`, nil
	case ssztypes.SszUint32Type:
		return `{"kind":"Uint32","bits":32}`, nil
	case ssztypes.SszUint64Type:
		return `{"kind":"Uint64","bits":64}`, nil
	case ssztypes.SszUint128Type:
		return `{"kind":"Uint128","bits":128}`, nil
	case ssztypes.SszUint256Type:
		return `{"kind":"Uint256","bits":256}`, nil

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		if desc.ContainerDesc == nil {
			return "", fmt.Errorf("container without field descriptors")
		}
		fields := make([]string, 0, len(desc.ContainerDesc.Fields))
		maxIdx := -1
		for i := range desc.ContainerDesc.Fields {
			f := &desc.ContainerDesc.Fields[i]
			e, err := SpecJSON(f.Type)
			if err != nil {
				return "", fmt.Errorf("field %s: %w", f.Name, err)
			}
			if strings.ContainsAny(f.Name, " \t\r\n\"\\") {
				return "", fmt.Errorf("field name %q cannot be declared inline", f.Name)
			}
			fields = append(fields, `{"name":`+strconv.Quote(f.Name)+`,"type":`+e+`}`)
			if idx := int(f.SszIndex); idx > maxIdx {
				maxIdx = idx
			}
		}
		if desc.SszType == ssztypes.SszProgressiveContainerType {
			layout := make([]string, maxIdx+1)
			for i := range layout {
				layout[i] = "0"
			}
			for i := range desc.ContainerDesc.Fields {
				layout[desc.ContainerDesc.Fields[i].SszIndex] = "1"
			}
			return `{"kind":"ProgressiveContainer","activeFields":[` + strings.Join(layout, ",") +
				`],"fields":[` + strings.Join(fields, ",") + `]}`, nil
		}
		return `{"kind":"Container","fields":[` + strings.Join(fields, ",") + `]}`, nil

	case ssztypes.SszVectorType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("vector without element descriptor")
		}
		if desc.Len <= 0 {
			return "", fmt.Errorf("vector with non-positive length %d", desc.Len)
		}
		if unwrap(desc.ElemDesc).SszType == ssztypes.SszUint8Type {
			return fmt.Sprintf(`{"kind":"ByteVector","length":%d}`, desc.Len), nil
		}
		e, err := SpecJSON(desc.ElemDesc)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"kind":"Vector","length":%d,"elementType":%s}`, desc.Len, e), nil

	case ssztypes.SszListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("list without element descriptor")
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
			return "", fmt.Errorf("list without a declared limit has no SSZ type")
		}
		return listSpecJSON(desc.ElemDesc, desc.Limit)

	case ssztypes.SszOptionalListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("optional list without element descriptor")
		}
		return listSpecJSON(desc.ElemDesc, 1)

	case ssztypes.SszTypeWrapperType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("type wrapper without wrapped descriptor")
		}
		return SpecJSON(desc.ElemDesc)

	case ssztypes.SszUnionType, ssztypes.SszOptionalType, ssztypes.SszBigIntType,
		ssztypes.SszInt8Type, ssztypes.SszInt16Type, ssztypes.SszInt32Type, ssztypes.SszInt64Type,
		ssztypes.SszFloat32Type, ssztypes.SszFloat64Type:
		return "", fmt.Errorf("%w: %v", ErrNoSpec, desc.SszType)

	case ssztypes.SszProgressiveListType:
		if desc.ElemDesc == nil {
			return "", fmt.Errorf("progressive list without element descriptor")
		}
		e, err := SpecJSON(desc.ElemDesc)
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
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
			return "", fmt.Errorf("bitlist without a declared limit has no SSZ type")
		}
		return fmt.Sprintf(`{"kind":"BitList","limit":%d}`, desc.Limit), nil

	case ssztypes.SszProgressiveBitlistType:
		return `{"kind":"ProgressiveBitList"` + optLimitJSON(desc) + `}`, nil

	case ssztypes.SszCompatibleUnionType:
		if len(desc.UnionVariants) == 0 {
			return "", fmt.Errorf("compatible union without variants")
		}
		selectors := make([]int, 0, len(desc.UnionVariants))
		for sel := range desc.UnionVariants {
			selectors = append(selectors, int(sel))
		}
		sort.Ints(selectors)
		opts := make([]string, 0, len(selectors))
		for _, sel := range selectors {
			e, err := SpecJSON(desc.UnionVariants[uint8(sel)])
			if err != nil {
				return "", fmt.Errorf("union variant %d: %w", sel, err)
			}
			opts = append(opts, fmt.Sprintf(`{"selector":%d,"type":%s}`, sel, e))
		}
		return `{"kind":"CompatibleUnion","options":[` + strings.Join(opts, ",") + `]}`, nil
	default:
		// Extended and legacy kinds have no specification to certify against.
	}
	return "", fmt.Errorf("unsupported SSZ type %v (EthLib covers the spec-defined SSZ surface)", desc.SszType)
}

func listSpecJSON(elem *ssztypes.TypeDescriptor, limit uint64) (string, error) {
	if unwrap(elem).SszType == ssztypes.SszUint8Type {
		return fmt.Sprintf(`{"kind":"ByteList","limit":%d}`, limit), nil
	}
	e, err := SpecJSON(elem)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"kind":"List","limit":%d,"elementType":%s}`, limit, e), nil
}

func optLimitJSON(desc *ssztypes.TypeDescriptor) string {
	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 {
		return fmt.Sprintf(`,"limit":%d`, desc.Limit)
	}
	return ""
}
