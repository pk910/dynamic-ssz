package engine

// Independent, spec-first-principles SSZ reference implementation (serialization
// + hash_tree_root) via reflection over the same struct tags. It shares NO code
// with the dynssz reflection engine or its generated code, so agreement is
// genuine third-party corroboration: a reflection==codegen-but-both-wrong bug
// would be caught here where the self-differential checks cannot see it.
//
// It supports the well-modeled single-dimensional subset: primitives, byte/int
// arrays and vectors, byte/int lists, bitvector, nested containers, pointers to
// any of these, and single-dimensional collections of containers. Constructs
// whose independent reimplementation is error-prone or ambiguous are
// deliberately out of scope (refSupportsType returns false and the caller skips
// the reference check, leaving it to the self-differential oracles):
// multi-dimensional collections (mixed ssz-size/ssz-max dimension rules),
// bitlist (delimiter canonicalization), progressive lists/bitlists/containers,
// optionals, unions, type wrappers, uint128/256, spec-driven dynssz-size/max,
// and the extended scalar types int8..64 / float / big.Int / time.Time.

import (
	"crypto/sha256"
	"encoding/binary"
	"math/bits"
	"reflect"
	"strconv"
	"strings"
)

// SSZ ssz-type tag values used across the reference oracle.
const (
	tagBitlist   = "bitlist"
	tagBitvector = "bitvector"
)

type refTag struct {
	size []int // ssz-size dims (-1 for "?")
	max  []int // ssz-max dims
	typ  string
}

func parseRefTag(t reflect.StructTag) refTag {
	var s refTag
	s.typ = t.Get("ssz-type")
	if v := t.Get("ssz-size"); v != "" {
		for _, p := range strings.Split(v, ",") {
			if p == "?" {
				s.size = append(s.size, -1)
			} else {
				n, _ := strconv.Atoi(p)
				s.size = append(s.size, n)
			}
		}
	}
	if v := t.Get("ssz-max"); v != "" {
		for _, p := range strings.Split(v, ",") {
			n, _ := strconv.Atoi(p)
			s.max = append(s.max, n)
		}
	}
	return s
}

func (s refTag) shift() refTag {
	n := s
	if len(n.size) > 0 {
		n.size = n.size[1:]
	}
	if len(n.max) > 0 {
		n.max = n.max[1:]
	}
	return n
}

func (s refTag) curMax() int {
	if len(s.max) > 0 {
		return s.max[0]
	}
	return 0
}

// refSupportsType reports whether the reference oracle can faithfully handle a
// type. It is deliberately CONSERVATIVE: anything it is not certain about
// returns false, so the reference check can never produce a false positive on a
// construct outside its model.
func refSupportsType(t reflect.Type, tag reflect.StructTag) bool {
	rt := parseRefTag(tag)
	switch rt.typ {
	case "progressive-list", "progressive-bitlist", "optional", "wrapper", "uint128", "uint256", "union":
		return false
	}
	// Spec-driven sizing is out of scope (the oracle has no spec resolver).
	if tag.Get("dynssz-size") != "" || tag.Get("dynssz-max") != "" {
		return false
	}
	switch t.Kind() {
	case reflect.Bool, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Ptr:
		return refSupportsType(t.Elem(), tag)
	case reflect.Array, reflect.Slice:
		// bitvector is a fixed []byte; supported. bitlist canonicalization
		// (delimiter placement, trailing-zero trimming) is out of scope.
		if rt.typ == tagBitlist {
			return false
		}
		if rt.typ == tagBitvector {
			return t.Elem().Kind() == reflect.Uint8
		}
		// Single-dimensional collections only. Multi-dimensional collections
		// (an element that is itself an array/slice, with the mixed ssz-size/
		// ssz-max dimension rules) are out of scope: an independent
		// reimplementation of those nested offset/padding rules is exactly
		// where divergence-from-correct is most likely, so we skip them rather
		// than risk a false positive. Collections of containers are supported.
		elem := t.Elem()
		if elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Array || elem.Kind() == reflect.Slice {
			return false
		}
		return refSupportsType(t.Elem(), consumeDim(tag))
	case reflect.Struct:
		return refSupportsStruct(t)
	default:
		return false
	}
}

// consumeDim drops the leading ssz-size / ssz-max dimension from a struct tag,
// so a nested collection is checked against the remaining dimensions.
func consumeDim(tag reflect.StructTag) reflect.StructTag {
	rt := parseRefTag(tag).shift()
	var parts []string
	if tp := tag.Get("ssz-type"); tp != "" {
		// The ssz-type applies to the outer dimension; a nested collection has
		// no residual type qualifier in this generator's output.
		_ = tp
	}
	if len(rt.size) > 0 {
		ds := make([]string, len(rt.size))
		for i, d := range rt.size {
			if d < 0 {
				ds[i] = "?"
			} else {
				ds[i] = strconv.Itoa(d)
			}
		}
		parts = append(parts, `ssz-size:"`+strings.Join(ds, ",")+`"`)
	}
	if len(rt.max) > 0 {
		ms := make([]string, len(rt.max))
		for i, d := range rt.max {
			ms[i] = strconv.Itoa(d)
		}
		parts = append(parts, `ssz-max:"`+strings.Join(ms, ",")+`"`)
	}
	return reflect.StructTag(strings.Join(parts, " "))
}

func refSupportsStruct(t reflect.Type) bool {
	// Reject foreign container types (unions, wrappers, time.Time, big.Int, ...).
	switch t.PkgPath() {
	case "github.com/pk910/dynamic-ssz", "time", "math/big":
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Tag.Get("ssz-index") != "" { // progressive container
			return false
		}
		if !refSupportsType(f.Type, f.Tag) {
			return false
		}
	}
	return true
}

// zeroDeref returns the concrete value a pointer addresses, or a fresh zero
// value of the element type when the pointer is nil (SSZ encodes a nil pointer
// as its zero value, matching dynssz).
func zeroDeref(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Ptr {
		return v
	}
	if v.IsNil() {
		return reflect.New(v.Type().Elem()).Elem()
	}
	return v.Elem()
}

func refFixedSize(t reflect.Type, tag refTag) (int, bool) {
	if tag.typ == tagBitlist {
		return 0, false
	}
	switch t.Kind() {
	case reflect.Bool, reflect.Uint8:
		return 1, true
	case reflect.Uint16:
		return 2, true
	case reflect.Uint32:
		return 4, true
	case reflect.Uint64:
		return 8, true
	case reflect.Ptr:
		return refFixedSize(t.Elem(), tag)
	case reflect.Array:
		es, ok := refFixedSize(t.Elem(), tag.shift())
		if !ok {
			return 0, false
		}
		return es * t.Len(), true
	case reflect.Slice:
		if tag.typ == tagBitvector && len(tag.size) > 0 {
			return tag.size[0], true // bitvector: fixed N bytes
		}
		// A concrete ssz-size dim makes this a fixed Vector[E, N]; it is fixed
		// only when its element type is itself fixed.
		if len(tag.size) > 0 && tag.size[0] >= 0 {
			es, ok := refFixedSize(t.Elem(), tag.shift())
			if !ok {
				return 0, false
			}
			return es * tag.size[0], true
		}
		return 0, false // ssz-max lists are variable
	case reflect.Struct:
		total := 0
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fs, ok := refFixedSize(f.Type, parseRefTag(f.Tag))
			if !ok {
				return 0, false
			}
			total += fs
		}
		return total, true
	default:
		return 0, false
	}
}

func refMarshal(v reflect.Value, tag refTag) []byte {
	v = zeroDeref(v)
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return []byte{1}
		}
		return []byte{0}
	case reflect.Uint8:
		return []byte{uint8(v.Uint())}
	case reflect.Uint16:
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(v.Uint()))
		return b
	case reflect.Uint32:
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(v.Uint()))
		return b
	case reflect.Uint64:
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, v.Uint())
		return b
	case reflect.Array:
		et := v.Type().Elem()
		if _, ok := refFixedSize(et, tag.shift()); ok {
			var out []byte
			for i := 0; i < v.Len(); i++ {
				out = append(out, refMarshal(v.Index(i), tag.shift())...)
			}
			return out
		}
		// Fixed-length vector of VARIABLE-size elements: an offset table (one
		// 4-byte offset per element, no length prefix), then the bodies.
		return refOffsetEncode(v, tag)
	case reflect.Slice:
		if tag.typ == tagBitlist {
			return append([]byte(nil), v.Bytes()...) // raw incl. delimiter
		}
		if tag.typ == tagBitvector && len(tag.size) > 0 {
			out := make([]byte, tag.size[0])
			copy(out, v.Bytes())
			return out
		}
		et := v.Type().Elem()
		// A concrete ssz-size dim makes this a fixed Vector[E, N]: exactly N
		// elements, zero-padded or truncated, regardless of the value's length.
		if len(tag.size) > 0 && tag.size[0] >= 0 {
			return refVectorEncode(v, et, tag, tag.size[0])
		}
		// ssz-max list: elements as-is (offset table if variable-size).
		if _, ok := refFixedSize(et, tag.shift()); ok {
			var out []byte
			for i := 0; i < v.Len(); i++ {
				out = append(out, refMarshal(v.Index(i), tag.shift())...)
			}
			return out
		}
		return refOffsetEncode(v, tag)
	case reflect.Struct:
		return refMarshalContainer(v)
	default:
		return nil
	}
}

// refVectorEncode encodes a fixed Vector[E, n]: exactly n elements, drawing
// zero values past the value's length and ignoring any excess, then either
// concatenated (fixed element) or through an offset table (variable element).
func refVectorEncode(v reflect.Value, et reflect.Type, tag refTag, n int) []byte {
	elem := func(i int) reflect.Value {
		if i < v.Len() {
			return v.Index(i)
		}
		return reflect.Zero(et)
	}
	if _, ok := refFixedSize(et, tag.shift()); ok {
		var out []byte
		for i := 0; i < n; i++ {
			out = append(out, refMarshal(elem(i), tag.shift())...)
		}
		return out
	}
	parts := make([][]byte, n)
	for i := 0; i < n; i++ {
		parts[i] = refMarshal(elem(i), tag.shift())
	}
	off := 4 * n
	var head, body []byte
	for _, p := range parts {
		ob := make([]byte, 4)
		binary.LittleEndian.PutUint32(ob, uint32(off))
		head = append(head, ob...)
		off += len(p)
		body = append(body, p...)
	}
	return append(head, body...)
}

// refOffsetEncode encodes an array/slice of variable-size elements as an
// offset table (one 4-byte offset per element) followed by the element bodies.
func refOffsetEncode(v reflect.Value, tag refTag) []byte {
	parts := make([][]byte, v.Len())
	for i := 0; i < v.Len(); i++ {
		parts[i] = refMarshal(v.Index(i), tag.shift())
	}
	off := 4 * v.Len()
	var head, body []byte
	for _, p := range parts {
		ob := make([]byte, 4)
		binary.LittleEndian.PutUint32(ob, uint32(off))
		head = append(head, ob...)
		off += len(p)
		body = append(body, p...)
	}
	return append(head, body...)
}

func refMarshalContainer(v reflect.Value) []byte {
	t := v.Type()
	n := t.NumField()
	fixed := make([][]byte, n)
	variable := make([][]byte, n)
	isVar := make([]bool, n)
	fixedLen := 0
	for i := 0; i < n; i++ {
		f := t.Field(i)
		tg := parseRefTag(f.Tag)
		if _, ok := refFixedSize(f.Type, tg); ok {
			fixed[i] = refMarshal(v.Field(i), tg)
			fixedLen += len(fixed[i])
		} else {
			isVar[i] = true
			variable[i] = refMarshal(v.Field(i), tg)
			fixedLen += 4
		}
	}
	off := fixedLen
	var head, body []byte
	for i := 0; i < n; i++ {
		if isVar[i] {
			ob := make([]byte, 4)
			binary.LittleEndian.PutUint32(ob, uint32(off))
			head = append(head, ob...)
			off += len(variable[i])
			body = append(body, variable[i]...)
		} else {
			head = append(head, fixed[i]...)
		}
	}
	return append(head, body...)
}

func refHTR(v reflect.Value, tag refTag) [32]byte {
	v = zeroDeref(v)
	switch v.Kind() {
	case reflect.Bool, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var c [32]byte
		copy(c[:], refMarshal(v, tag))
		return c
	case reflect.Array:
		et := v.Type().Elem()
		if refIsBasicKind(et.Kind()) {
			return refMerkleize(refPack(refMarshal(v, tag)), 0)
		}
		roots := make([][32]byte, v.Len())
		for i := 0; i < v.Len(); i++ {
			roots[i] = refHTR(v.Index(i), tag.shift())
		}
		return refMerkleize(roots, 0)
	case reflect.Slice:
		if tag.typ == tagBitvector && len(tag.size) > 0 {
			out := make([]byte, tag.size[0])
			copy(out, v.Bytes())
			return refMerkleize(refChunkify(out), 0)
		}
		if tag.typ == tagBitlist {
			bl := v.Bytes()
			nbits := refBitlistLen(bl)
			limit := (tag.curMax() + 255) / 256
			return refMixIn(refMerkleize(refChunkify(refTrimDelimiter(bl, nbits)), limit), uint64(nbits))
		}
		et := v.Type().Elem()
		// A concrete ssz-size dim makes this a fixed Vector[E, N]: merkleized to
		// N (no length mixin), unlike an ssz-max List (mixin over the count).
		if len(tag.size) > 0 && tag.size[0] >= 0 {
			n := tag.size[0]
			if refIsBasicKind(et.Kind()) {
				es, _ := refFixedSize(et, tag.shift())
				chunks := (n*es + 31) / 32
				return refMerkleize(refPack(refMarshal(v, tag)), chunks)
			}
			roots := make([][32]byte, n)
			for i := 0; i < n; i++ {
				if i < v.Len() {
					roots[i] = refHTR(v.Index(i), tag.shift())
				} else {
					roots[i] = refHTR(reflect.Zero(et), tag.shift())
				}
			}
			return refMerkleize(roots, n)
		}
		limit := tag.curMax()
		if refIsBasicKind(et.Kind()) {
			es, _ := refFixedSize(et, tag.shift())
			limitChunks := (limit*es + 31) / 32
			return refMixIn(refMerkleize(refPack(refMarshal(v, tag)), limitChunks), uint64(v.Len()))
		}
		roots := make([][32]byte, v.Len())
		for i := 0; i < v.Len(); i++ {
			roots[i] = refHTR(v.Index(i), tag.shift())
		}
		return refMixIn(refMerkleize(roots, limit), uint64(v.Len()))
	case reflect.Struct:
		t := v.Type()
		roots := make([][32]byte, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			roots[i] = refHTR(v.Field(i), parseRefTag(t.Field(i).Tag))
		}
		return refMerkleize(roots, 0)
	default:
		return [32]byte{}
	}
}

func refIsBasicKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func refPack(b []byte) [][32]byte { return refChunkify(b) }

func refChunkify(b []byte) [][32]byte {
	if len(b) == 0 {
		return [][32]byte{{}}
	}
	n := (len(b) + 31) / 32
	out := make([][32]byte, n)
	for i := 0; i < n; i++ {
		copy(out[i][:], b[i*32:min(len(b), i*32+32)])
	}
	return out
}

func refTrimDelimiter(bl []byte, nbits int) []byte {
	out := make([]byte, (nbits+7)/8)
	copy(out, bl)
	if nbits%8 != 0 {
		out[len(out)-1] &= byte((1 << (uint(nbits) % 8)) - 1)
	}
	return out
}

func refBitlistLen(bl []byte) int {
	if len(bl) == 0 {
		return 0
	}
	last := bl[len(bl)-1]
	if last == 0 {
		return 0
	}
	return (len(bl)-1)*8 + bits.Len8(last) - 1
}

func refMixIn(root [32]byte, length uint64) [32]byte {
	var lb [32]byte
	binary.LittleEndian.PutUint64(lb[:8], length)
	return sha256.Sum256(append(root[:], lb[:]...))
}

func refMerkleize(chunks [][32]byte, limit int) [32]byte {
	count := len(chunks)
	if limit > count {
		count = limit
	}
	if count < 1 {
		count = 1
	}
	width := 1
	for width < count {
		width <<= 1
	}
	nodes := make([][32]byte, width)
	copy(nodes, chunks)
	for width > 1 {
		for i := 0; i < width/2; i++ {
			nodes[i] = sha256.Sum256(append(nodes[2*i][:], nodes[2*i+1][:]...))
		}
		width /= 2
	}
	return nodes[0]
}

// refCheck compares dynssz reflection bytes/root against the independent
// reference for a supported type. Returns ("", true) on agreement or an
// unsupported/undecodable value; a description + false on divergence.
func refCheck(t reflect.Type, val any, dynBytes []byte, dynRoot [32]byte) (string, bool) {
	if !refSupportsType(t, "") {
		return "", true
	}
	rv := reflect.ValueOf(val)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return "", true
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return "", true
	}
	var rb []byte
	var rr [32]byte
	ok := true
	if p := capturePanic(func() {
		rb = refMarshalContainer(rv)
		rr = refHTR(rv, refTag{})
	}); p != "" {
		return "", true // reference could not handle it → skip, never false-positive
	}
	if rb == nil {
		return "", true
	}
	if !equalBytes(rb, dynBytes) {
		return "reference-marshal-divergence: dynssz=" + hexShort(dynBytes) + " ref=" + hexShort(rb), false
	}
	if rr != dynRoot {
		return "reference-HTR-divergence: dynssz=" + hexShort(dynRoot[:]) + " ref=" + hexShort(rr[:]), false
	}
	_ = ok
	return "", true
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hexShort(b []byte) string {
	const h = "0123456789abcdef"
	n := len(b)
	if n > 256 {
		n = 48
	}
	sb := make([]byte, n*2)
	for i := 0; i < n; i++ {
		sb[i*2] = h[b[i]>>4]
		sb[i*2+1] = h[b[i]&0xf]
	}
	return string(sb)
}
