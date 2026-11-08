package harness

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"pgregory.net/rapid"
)

// GenConfig bounds the values GenValue draws.
type GenConfig struct {
	// MaxElems caps list lengths (declared limits reach 2^40). Lengths are
	// drawn up to min(limit, MaxElems).
	MaxElems int
	// MaxBytes caps byte-list lengths separately: byte lists reach much
	// larger sizes cheaply and are where the multi-chunk paths live.
	MaxBytes int
	// MaxDepth bounds recursion through composite values; deeper collections
	// are drawn empty and deeper optionals absent.
	MaxDepth int
	// Boundary biases lengths toward the boundaries the encoders and hashers
	// special-case: empty, one, the limit and one below it, and element counts
	// at and around chunk, power-of-two-chunk and batch boundaries.
	Boundary bool
}

// DefaultGen is the CI configuration: boundary-biased, moderately sized.
var DefaultGen = GenConfig{MaxElems: 300, MaxBytes: 20000, MaxDepth: 6, Boundary: true}

// GenValue draws a random value of the Go type behind newFn's pointer,
// walking the type descriptor of the given DynSsz instance (which must be
// built WithNoDelegation: delegating instances hold shallow descriptors for
// types with generated methods) — the descriptor
// the reflection engine itself uses — so lengths, limits, bit sizes, union
// variants and extended kinds follow the library's own reading of the type,
// with resolved spec values.
func GenValue(rt *rapid.T, ds *dynssz.DynSsz, newFn func() any, cfg GenConfig) (any, error) {
	v := newFn()
	rv := reflect.ValueOf(v)
	desc, err := ds.GetTypeCache().GetTypeDescriptor(rv.Type().Elem(), nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if err := genDesc(rt, desc, rv.Elem(), &cfg, 0); err != nil {
		return nil, err
	}
	return v, nil
}

// pickLen draws a collection length in [0, limit] (capped), biased toward
// boundaries when configured. perChunk is the number of elements one 32-byte
// chunk holds (1 for composite elements).
func pickLen(rt *rapid.T, limit uint64, maxN, perChunk int, cfg *GenConfig, label string) int {
	hi := maxN
	if limit < uint64(hi) {
		hi = int(limit)
	}
	if hi <= 0 {
		return 0
	}
	if !cfg.Boundary || rapid.IntRange(0, 3).Draw(rt, label+"-mode") == 0 {
		return rapid.IntRange(0, hi).Draw(rt, label)
	}
	cands := []int{0, 1, hi - 1, hi}
	if perChunk < 1 {
		perChunk = 1
	}
	for chunks := 1; chunks*perChunk <= hi+perChunk; chunks *= 2 {
		n := chunks * perChunk
		cands = append(cands, n-1, n, n+1)
	}
	// The hasher's batch threshold (256 chunks) and progressive group sizes.
	for _, chunks := range []int{255, 256, 257, 512, 513, 1, 5, 21, 85, 341, 1365} {
		n := chunks * perChunk
		cands = append(cands, n-1, n, n+1)
	}
	valid := cands[:0]
	for _, n := range cands {
		if n >= 0 && n <= hi {
			valid = append(valid, n)
		}
	}
	return rapid.SampledFrom(valid).Draw(rt, label)
}

// basicSize is the packed width of a basic descriptor, 0 otherwise.
func basicSize(d *ssztypes.TypeDescriptor) int {
	if d == nil {
		return 0
	}
	for d.SszType == ssztypes.SszTypeWrapperType && d.ElemDesc != nil {
		d = d.ElemDesc
	}
	if d.SszType.IsBasic() {
		return int(d.Size)
	}
	return 0
}

func perChunkOf(elem *ssztypes.TypeDescriptor) int {
	if w := basicSize(elem); w > 0 {
		return 32 / w
	}
	return 1
}

func randBytes(rt *rapid.T, n int, label string) []byte {
	return rapid.SliceOfN(rapid.Byte(), n, n).Draw(rt, label)
}

// genDesc fills v (addressable, of desc.Type) with a random value.
func genDesc(rt *rapid.T, desc *ssztypes.TypeDescriptor, v reflect.Value, cfg *GenConfig, depth int) error {
	// Plain pointers (not optional) are always populated.
	if v.Kind() == reflect.Pointer && desc.SszType != ssztypes.SszOptionalType &&
		desc.SszType != ssztypes.SszOptionalListType {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	switch desc.SszType {
	case ssztypes.SszBoolType:
		v.SetBool(rapid.Bool().Draw(rt, "bool"))
	case ssztypes.SszUint8Type, ssztypes.SszUint16Type, ssztypes.SszUint32Type, ssztypes.SszUint64Type:
		bits := 8 * int(desc.Size)
		n := rapid.Uint64().Draw(rt, "uint")
		if bits < 64 {
			n &= (1 << bits) - 1
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime != 0 {
			v.Set(reflect.ValueOf(time.Unix(int64(n>>2), 0).UTC()))
			return nil
		}
		v.SetUint(n)
	case ssztypes.SszInt8Type, ssztypes.SszInt16Type, ssztypes.SszInt32Type, ssztypes.SszInt64Type:
		bits := 8 * int(desc.Size)
		n := rapid.Int64().Draw(rt, "int")
		if bits < 64 {
			n = n << (64 - bits) >> (64 - bits)
		}
		v.SetInt(n)
	case ssztypes.SszFloat32Type:
		// Set through a same-width conversion: widening to float64 would quiet
		// a signaling NaN and change its bits.
		v.Set(reflect.ValueOf(randFloat32(rt)).Convert(v.Type()))
	case ssztypes.SszFloat64Type:
		v.Set(reflect.ValueOf(randFloat64(rt)).Convert(v.Type()))
	case ssztypes.SszUint128Type, ssztypes.SszUint256Type:
		return fillRaw(rt, v, int(desc.Size))
	case ssztypes.SszBigIntType:
		maxMag := 64
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 && desc.Limit >= 1 && desc.Limit-1 < uint64(maxMag) {
			maxMag = int(desc.Limit - 1)
		}
		mag := randBytes(rt, rapid.IntRange(0, maxMag).Draw(rt, "biglen"), "bigmag")
		b := new(big.Int).SetBytes(mag)
		if rapid.Bool().Draw(rt, "bigneg") {
			b.Neg(b)
		}
		v.Set(reflect.ValueOf(*b))

	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		for i := range desc.ContainerDesc.Fields {
			f := &desc.ContainerDesc.Fields[i]
			if err := genDesc(rt, f.Type, v.Field(int(f.FieldIndex)), cfg, depth+1); err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
		}

	case ssztypes.SszVectorType:
		return genSeq(rt, desc, v, int(desc.Len), cfg, depth)
	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		limit := desc.Limit
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
			limit = ^uint64(0)
		}
		maxN := cfg.MaxElems
		if basicSize(desc.ElemDesc) == 1 {
			maxN = cfg.MaxBytes
		}
		if depth >= cfg.MaxDepth {
			maxN = 0
		}
		n := pickLen(rt, limit, maxN, perChunkOf(desc.ElemDesc), cfg, "len")
		return genSeq(rt, desc, v, n, cfg, depth)

	case ssztypes.SszBitvectorType:
		bits := int(desc.BitSize)
		if bits <= 0 {
			bits = 8 * int(desc.Size)
		}
		return setBits(rt, v, bits, false)
	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		limit := desc.Limit
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
			limit = ^uint64(0)
		}
		n := pickLen(rt, limit, 8*cfg.MaxElems, 256, cfg, "bits")
		return setBits(rt, v, n, true)

	case ssztypes.SszCompatibleUnionType, ssztypes.SszUnionType:
		hasNone := desc.SszTypeFlags&ssztypes.SszTypeFlagHasNoneVariant != 0
		sels := make([]int, 0, len(desc.UnionVariants)+1)
		if hasNone {
			sels = append(sels, -1)
		}
		for sel := range desc.UnionVariants {
			sels = append(sels, int(sel))
		}
		sortInts(sels)
		pick := rapid.SampledFrom(sels).Draw(rt, "variant")
		if pick < 0 {
			v.Field(0).SetUint(0)
			v.Field(1).Set(reflect.Zero(v.Field(1).Type()))
			return nil
		}
		vd := desc.UnionVariants[uint8(pick)]
		pv := reflect.New(vd.Type).Elem()
		if err := genDesc(rt, vd, pv, cfg, depth+1); err != nil {
			return err
		}
		v.Field(0).SetUint(uint64(pick))
		v.Field(1).Set(pv)

	case ssztypes.SszOptionalType, ssztypes.SszOptionalListType:
		if depth >= cfg.MaxDepth || rapid.IntRange(0, 2).Draw(rt, "absent") == 0 {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		pv := reflect.New(v.Type().Elem())
		if err := genDesc(rt, desc.ElemDesc, pv.Elem(), cfg, depth+1); err != nil {
			return err
		}
		v.Set(pv)

	case ssztypes.SszTypeWrapperType:
		return genDesc(rt, desc.ElemDesc, v.Field(int(desc.WrapperFieldIndex)), cfg, depth)

	default:
		return fmt.Errorf("no generator for SSZ type %v (%v)", desc.SszType, desc.Type)
	}
	return nil
}

// genSeq fills an array (n must equal its length) or makes a slice of n
// elements. Byte sequences (and strings) are filled directly.
func genSeq(rt *rapid.T, desc *ssztypes.TypeDescriptor, v reflect.Value, n int, cfg *GenConfig, depth int) error {
	elem := desc.ElemDesc
	if v.Kind() == reflect.String {
		v.SetString(string(randBytes(rt, n, "str")))
		return nil
	}
	if v.Kind() == reflect.Slice {
		v.Set(reflect.MakeSlice(v.Type(), n, n))
	}
	if v.Type().Elem().Kind() == reflect.Uint8 && basicSize(elem) == 1 &&
		elem.SszType == ssztypes.SszUint8Type {
		bs := randBytes(rt, n, "bytes")
		for i := 0; i < n && i < v.Len(); i++ {
			v.Index(i).SetUint(uint64(bs[i]))
		}
		return nil
	}
	for i := 0; i < v.Len(); i++ {
		if err := genDesc(rt, elem, v.Index(i), cfg, depth+1); err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
	}
	return nil
}

// fillRaw fills a fixed-width numeric Go value (byte arrays, uint64 limb
// arrays such as holiman/uint256, or big-endian-agnostic structs of them).
func fillRaw(rt *rapid.T, v reflect.Value, size int) error {
	switch v.Kind() {
	case reflect.Array, reflect.Slice:
		if v.Kind() == reflect.Slice {
			n := size / int(v.Type().Elem().Size())
			v.Set(reflect.MakeSlice(v.Type(), n, n))
		}
		for i := 0; i < v.Len(); i++ {
			e := v.Index(i)
			switch e.Kind() {
			case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				x := rapid.Uint64().Draw(rt, "limb")
				if w := uint(8 * e.Type().Size()); w < 64 {
					x &= 1<<w - 1
				}
				e.SetUint(x)
			default:
				return fmt.Errorf("unsupported limb kind %s", e.Kind())
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported wide-uint kind %s", v.Kind())
	}
}

// setBits fills a bitvector (no delimiter) or bitlist (with delimiter) of n
// bits into a byte array or byte slice.
func setBits(rt *rapid.T, v reflect.Value, n int, delimiter bool) error {
	total := n
	if delimiter {
		total = n + 1
	}
	nbytes := (total + 7) / 8
	bs := make([]byte, nbytes)
	for i := 0; i < n; i++ {
		if rapid.Bool().Draw(rt, "bit") {
			bs[i/8] |= 1 << (i % 8)
		}
	}
	if delimiter {
		bs[n/8] |= 1 << (n % 8)
	}
	switch v.Kind() {
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), nbytes, nbytes))
	case reflect.Array:
		if v.Len() != nbytes {
			return fmt.Errorf("bit array of %d bytes for %d bits", v.Len(), total)
		}
	default:
		return fmt.Errorf("unsupported bit container kind %s", v.Kind())
	}
	for i := 0; i < nbytes; i++ {
		v.Index(i).SetUint(uint64(bs[i]))
	}
	return nil
}

// randFloat32 draws any bit pattern (NaN payloads, infinities, -0 included).
func randFloat32(rt *rapid.T) float32 {
	return math.Float32frombits(rapid.Uint32().Draw(rt, "f32"))
}

// randFloat64 draws any bit pattern (NaN payloads, infinities, -0 included).
func randFloat64(rt *rapid.T) float64 {
	return math.Float64frombits(rapid.Uint64().Draw(rt, "f64"))
}

func sortInts(xs []int) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}
