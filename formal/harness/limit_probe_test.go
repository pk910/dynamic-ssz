package harness

import (
	"github.com/pk910/dynamic-ssz/ssztypes"
)

// limitProbeMaxBytes bounds the probe encodings (a limit this large is
// covered by the generative phase's boundary bias instead).
const limitProbeMaxBytes = 1 << 16

// limitProbes returns deterministic encodings of a top-level list or bitlist
// at limit−1, limit and limit+1 elements (bits), or nil when the type is not
// one. List elements must be fixed-size; they are all-zero, which every
// fixed-size SSZ element accepts. The generative phase reaches these
// boundaries only by chance (a length at the limit and an element-wide
// append in the same draw), so a limit check off by one would otherwise
// survive whenever the seed misses them.
func limitProbes(desc *ssztypes.TypeDescriptor) [][]byte {
	if desc == nil || desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 || desc.Limit > limitProbeMaxBytes*8 {
		return nil
	}
	limit := int(desc.Limit)
	counts := []int{limit, limit + 1}
	if limit > 0 {
		counts = append(counts, limit-1)
	}
	var out [][]byte
	switch desc.SszType {
	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		elem := desc.ElemDesc
		if elem == nil || elem.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 || elem.Size <= 0 {
			return nil
		}
		w := int(elem.Size)
		for _, n := range counts {
			if n*w <= limitProbeMaxBytes {
				out = append(out, make([]byte, n*w))
			}
		}
	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		for _, n := range counts {
			b := make([]byte, n/8+1)
			b[n/8] = 1 << (n % 8) // the length sentinel
			out = append(out, b)
		}
	default:
	}
	return out
}
