package harness

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"reflect"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// engineSet is every dynamic-ssz configuration the link holds to the
// verified models: both engines, each with synchronous and asynchronous
// hashing, the native sha256 backend next to the SIMD one, and tiny stream
// buffers that force the streaming decoder through open regions.
type engineSet struct {
	gen, refl             *dynssz.DynSsz
	genAsync, reflAsync   *dynssz.DynSsz
	reflNative            *dynssz.DynSsz
	genTiny, reflTiny     *dynssz.DynSsz
	genVerbose, reflVerb  *dynssz.DynSsz // verbose logging into a discarding callback
	reflDepth2            *dynssz.DynSsz // nesting bound of two levels (recursive values beyond it are refused)
	genStreamCap, reflCap *dynssz.DynSsz // unknown-size streams capped at 4 KiB

	// specParametric caches, per Go type, whether a resolved spec value
	// differs from its static default: the generated static methods
	// (MarshalSSZ, HashTreeRoot, …) then encode the defaults and are not
	// held to the engines; the *Dyn methods always are.
	specParametric map[reflect.Type]bool
}

// directStatic reports whether v's generated static methods must agree with
// the engines (no spec value in it differs from its default).
func (e *engineSet) directStatic(v any) bool {
	rt := reflect.TypeOf(v)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if e.specParametric == nil {
		e.specParametric = map[reflect.Type]bool{}
	}
	if p, ok := e.specParametric[rt]; ok {
		return !p
	}
	desc, err := e.refl.GetTypeCache().GetTypeDescriptor(rt, nil, nil, nil)
	p := err != nil || desc.SszTypeFlags&(ssztypes.SszTypeFlagHasDynamicSize|ssztypes.SszTypeFlagHasDynamicMax) != 0
	e.specParametric[rt] = p
	return !p
}

func newEngineSet(specs map[string]any) *engineSet {
	ext := dynssz.WithExtendedTypes()
	noDel := dynssz.WithNoDelegation()
	async := dynssz.WithAsyncHashing(4)
	tiny := []dynssz.DynSszOption{dynssz.WithStreamReaderBufferSize(8), dynssz.WithStreamWriterBufferSize(8)}
	quiet := dynssz.WithLogCb(func(string, ...any) {})
	return &engineSet{
		gen:          dynssz.NewDynSsz(specs, ext),
		refl:         dynssz.NewDynSsz(specs, ext, noDel),
		genAsync:     dynssz.NewDynSsz(specs, ext, async),
		reflAsync:    dynssz.NewDynSsz(specs, ext, noDel, async),
		reflNative:   dynssz.NewDynSsz(specs, ext, noDel, dynssz.WithNoFastHash()),
		genTiny:      dynssz.NewDynSsz(specs, append([]dynssz.DynSszOption{ext}, tiny...)...),
		reflTiny:     dynssz.NewDynSsz(specs, append([]dynssz.DynSszOption{ext, noDel}, tiny...)...),
		genVerbose:   dynssz.NewDynSsz(specs, ext, dynssz.WithVerbose(), quiet),
		reflVerb:     dynssz.NewDynSsz(specs, ext, noDel, dynssz.WithVerbose(), quiet),
		reflDepth2:   dynssz.NewDynSsz(specs, ext, noDel, dynssz.WithMaxNestingDepth(2)),
		genStreamCap: dynssz.NewDynSsz(specs, ext, dynssz.WithMaxStreamSize(4096)),
		reflCap:      dynssz.NewDynSsz(specs, ext, noDel, dynssz.WithMaxStreamSize(4096)),
	}
}

// failingWriter accepts limit bytes and then fails every Write: the encoders
// must propagate the error from wherever they are in the value.
type failingWriter struct {
	limit int
	n     int
}

var errWriterBroken = errors.New("writer broken")

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.n+len(p) > w.limit {
		room := w.limit - w.n
		if room < 0 {
			room = 0
		}
		w.n += room
		return room, errWriterBroken
	}
	w.n += len(p)
	return len(p), nil
}

// failingReader serves limit bytes of the data and then fails every Read.
type failingReader struct {
	data  []byte
	limit int
	pos   int
}

var errReaderBroken = errors.New("reader broken")

func (r *failingReader) Read(p []byte) (int, error) {
	if r.pos >= r.limit {
		return 0, errReaderBroken
	}
	n := copy(p, r.data[r.pos:min(r.limit, len(r.data))])
	r.pos += n
	if n == 0 {
		return 0, errReaderBroken
	}
	return n, nil
}

// oneByteReader hands out one byte per Read: the stream decoder must
// assemble every primitive across reads.
type oneByteReader struct{ r io.Reader }

func (o *oneByteReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return o.r.Read(p[:1])
}

// oneByteWriter accepts one byte per Write call.
type oneByteWriter struct{ buf bytes.Buffer }

func (o *oneByteWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		o.buf.WriteByte(b)
	}
	return len(p), nil
}

// pathFailure records one disagreement between API paths or with the model.
type pathFailure func(format string, args ...any)

// encodePaths serializes v through every encoding path of both engines and
// demands byte-identical output and SizeSSZ agreement. It returns the
// encoding, or ok=false when the value is not encodable (all paths must then
// refuse).
func (e *engineSet) encodePaths(fail pathFailure, v any) ([]byte, bool) {
	type res struct {
		name string
		enc  []byte
		err  error
	}
	var out []res
	for _, eng := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"codegen", e.gen}, {"reflection", e.refl}, {"codegen-tiny", e.genTiny}, {"reflection-tiny", e.reflTiny}} {
		enc, err := eng.ds.MarshalSSZ(v)
		out = append(out, res{eng.name + "/MarshalSSZ", enc, err})

		prefix := []byte{0xde, 0xad}
		to, err := eng.ds.MarshalSSZTo(v, append([]byte(nil), prefix...))
		if err == nil {
			if !bytes.HasPrefix(to, prefix) {
				fail("%s/MarshalSSZTo clobbered the destination prefix", eng.name)
			}
			to = to[len(prefix):]
		}
		out = append(out, res{eng.name + "/MarshalSSZTo", to, err})

		var w bytes.Buffer
		err = eng.ds.MarshalSSZWriter(v, &w)
		out = append(out, res{eng.name + "/MarshalSSZWriter", w.Bytes(), err})

		var ow oneByteWriter
		err = eng.ds.MarshalSSZWriter(v, &ow)
		out = append(out, res{eng.name + "/MarshalSSZWriter(1B)", ow.buf.Bytes(), err})

		// A writer that breaks halfway: the error must surface (every encoder
		// error arm), never a truncated success.
		if err == nil && len(enc) > 0 {
			for _, limit := range []int{0, len(enc) / 2, len(enc) - 1} {
				fw := &failingWriter{limit: limit}
				if werr := eng.ds.MarshalSSZWriter(v, fw); werr == nil {
					fail("%s/MarshalSSZWriter reported success on a writer that failed after %d of %d bytes", eng.name, limit, len(enc))
				}
			}
		}

		size, err := eng.ds.SizeSSZ(v)
		if err == nil && enc != nil && size != len(enc) {
			fail("%s/SizeSSZ = %d, encoding has %d bytes", eng.name, size, len(enc))
		}
	}
	// Verbose engines log every step into a discarding callback and must
	// encode the same bytes.
	for _, eng := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"codegen-verbose", e.genVerbose}, {"reflection-verbose", e.reflVerb}} {
		enc, err := eng.ds.MarshalSSZ(v)
		out = append(out, res{eng.name + "/MarshalSSZ", enc, err})
	}
	// The generated methods called directly, as a user holding the value
	// would: the static ones (default specs) and the *Dyn ones (the engine's).
	if e.directStatic(v) {
		if m, ok := v.(sszutils.FastsszValueMarshaler); ok {
			enc, err := m.MarshalSSZ()
			out = append(out, res{"generated/MarshalSSZ", enc, err})
		}
		if m, ok := v.(sszutils.FastsszBufferMarshaler); ok {
			to, err := m.MarshalSSZTo([]byte{0xde, 0xad})
			if err == nil && len(to) >= 2 {
				to = to[2:]
			}
			out = append(out, res{"generated/MarshalSSZTo", to, err})
		}
		if m, ok := v.(sszutils.FastsszSizer); ok {
			if size := m.SizeSSZ(); out[0].err == nil && size != len(out[0].enc) {
				fail("generated/SizeSSZ = %d, encoding has %d bytes", size, len(out[0].enc))
			}
		}
	}
	if m, ok := v.(sszutils.DynamicMarshaler); ok {
		enc, err := m.MarshalSSZDyn(e.gen, nil)
		out = append(out, res{"generated/MarshalSSZDyn", enc, err})
	}
	if m, ok := v.(sszutils.DynamicSizer); ok {
		if size := m.SizeSSZDyn(e.gen); out[0].err == nil && size != len(out[0].enc) {
			fail("generated/SizeSSZDyn = %d, encoding has %d bytes", size, len(out[0].enc))
		}
	}
	ref := out[0]
	for _, r := range out[1:] {
		if (r.err == nil) != (ref.err == nil) {
			fail("ENCODE PATH DIVERGENCE: %s err=%v, %s err=%v", ref.name, ref.err, r.name, r.err)
			continue
		}
		if r.err == nil && !bytes.Equal(r.enc, ref.enc) {
			fail("ENCODE PATH DIVERGENCE: %s=%x %s=%x", ref.name, ref.enc, r.name, r.enc)
		}
	}
	return ref.enc, ref.err == nil
}

// decodePaths decodes raw through every decoding path of both engines:
// buffer, stream with known size, stream with unknown size, and a stream
// handing out one byte per read. All must agree on accepting; accepted values
// must re-encode to raw. It returns the verdict and the codegen-decoded value.
func (e *engineSet) decodePaths(fail pathFailure, newFn func() any, raw []byte) (bool, any) {
	type res struct {
		name string
		v    any
		err  error
	}
	var out []res
	for _, eng := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"codegen", e.gen}, {"reflection", e.refl}, {"codegen-tiny", e.genTiny}, {"reflection-tiny", e.reflTiny}} {
		v := newFn()
		out = append(out, res{eng.name + "/UnmarshalSSZ", v, eng.ds.UnmarshalSSZ(v, raw)})
		v = newFn()
		out = append(out, res{eng.name + "/UnmarshalSSZReader(size)", v,
			eng.ds.UnmarshalSSZReader(v, bytes.NewReader(raw), len(raw))})
		v = newFn()
		out = append(out, res{eng.name + "/UnmarshalSSZReader(unknown)", v,
			eng.ds.UnmarshalSSZReader(v, bytes.NewReader(raw), -1)})
		v = newFn()
		out = append(out, res{eng.name + "/UnmarshalSSZReader(1B,unknown)", v,
			eng.ds.UnmarshalSSZReader(v, &oneByteReader{bytes.NewReader(raw)}, -1)})
	}
	// Unknown-size streams under a size cap: accepted when the encoding fits,
	// refused when it does not — on both engines, by instance option and by
	// call option.
	if out[0].err == nil {
		for _, eng := range []struct {
			name string
			ds   *dynssz.DynSsz
		}{{"codegen-cap", e.genStreamCap}, {"reflection-cap", e.reflCap}} {
			err := eng.ds.UnmarshalSSZReader(newFn(), bytes.NewReader(raw), -1)
			if fits := len(raw) <= 4096; (err == nil) != fits {
				fail("%s/UnmarshalSSZReader(unknown, cap 4096) on %d bytes: err=%v", eng.name, len(raw), err)
			}
		}
		for _, eng := range []struct {
			name string
			ds   *dynssz.DynSsz
		}{{"codegen", e.gen}, {"reflection", e.refl}} {
			if len(raw) > 1 {
				if err := eng.ds.UnmarshalSSZReader(newFn(), bytes.NewReader(raw), -1, dynssz.WithStreamSizeLimit(len(raw)-1)); err == nil {
					fail("%s/UnmarshalSSZReader(unknown, limit %d) accepted %d bytes", eng.name, len(raw)-1, len(raw))
				}
			}
			if err := eng.ds.UnmarshalSSZReader(newFn(), bytes.NewReader(raw), -1, dynssz.WithStreamSizeLimit(len(raw))); err != nil {
				fail("%s/UnmarshalSSZReader(unknown, limit %d) refused %d bytes: %v", eng.name, len(raw), len(raw), err)
			}
		}
	}
	// A reader that breaks halfway through an accepted encoding: every
	// decoder must fail (its error arms), never accept a truncated value.
	if out[0].err == nil && len(raw) > 0 {
		for _, eng := range []struct {
			name string
			ds   *dynssz.DynSsz
		}{{"codegen", e.gen}, {"reflection", e.refl}, {"codegen-tiny", e.genTiny}, {"reflection-tiny", e.reflTiny}} {
			for _, limit := range []int{0, len(raw) / 2, len(raw) - 1} {
				for _, known := range []bool{true, false} {
					size := len(raw)
					if !known {
						size = -1
					}
					if rerr := eng.ds.UnmarshalSSZReader(newFn(), &failingReader{data: raw, limit: limit}, size); rerr == nil {
						fail("%s/UnmarshalSSZReader(known=%v) accepted a stream that failed after %d of %d bytes", eng.name, known, limit, len(raw))
					}
				}
			}
		}
	}
	if probe := newFn(); e.directStatic(probe) {
		if m, ok := probe.(sszutils.FastsszUnmarshaler); ok {
			out = append(out, res{"generated/UnmarshalSSZ", probe, m.UnmarshalSSZ(raw)})
		}
	}
	if v := newFn(); true {
		if m, ok := v.(sszutils.DynamicUnmarshaler); ok {
			out = append(out, res{"generated/UnmarshalSSZDyn", v, m.UnmarshalSSZDyn(e.gen, raw)})
		}
	}
	ref := out[0]
	for _, r := range out[1:] {
		if (r.err == nil) != (ref.err == nil) {
			fail("DECODE PATH DIVERGENCE on 0x%s: %s err=%v, %s err=%v",
				shortHex(raw), ref.name, ref.err, r.name, r.err)
			continue
		}
		if r.err == nil {
			enc, err := e.gen.MarshalSSZ(r.v)
			if err != nil || !bytes.Equal(enc, raw) {
				fail("DECODE PATH CANONICITY: %s accepted 0x%s but re-encodes to 0x%s (err %v)",
					r.name, shortHex(raw), shortHex(enc), err)
			}
		}
	}
	return ref.err == nil, ref.v
}

// rootPaths computes the hash tree root of v through every merkleization
// path: HashTreeRoot of both engines with synchronous and asynchronous
// hashing, the native sha256 backend, and GetTree().Hash() of both engines.
// All must equal want (hex); it returns the codegen root.
func (e *engineSet) rootPaths(fail pathFailure, v any, want string) string {
	type res struct {
		name string
		root string
		err  error
	}
	var out []res
	for _, eng := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"codegen", e.gen}, {"reflection", e.refl}, {"codegen-async", e.genAsync},
		{"reflection-async", e.reflAsync}, {"reflection-native-sha256", e.reflNative},
		{"codegen-verbose", e.genVerbose}, {"reflection-verbose", e.reflVerb}} {
		r, err := eng.ds.HashTreeRoot(v)
		out = append(out, res{eng.name + "/HashTreeRoot", hex.EncodeToString(r[:]), err})
	}
	for _, eng := range []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"codegen", e.gen}, {"reflection", e.refl}} {
		tree, err := eng.ds.GetTree(v)
		root := ""
		if err == nil {
			root = hex.EncodeToString(tree.Hash())
		}
		out = append(out, res{eng.name + "/GetTree", root, err})
	}
	if e.directStatic(v) {
		if m, ok := v.(sszutils.FastsszHashRoot); ok {
			r, err := m.HashTreeRoot()
			out = append(out, res{"generated/HashTreeRoot", hex.EncodeToString(r[:]), err})
		}
		if m, ok := v.(sszutils.FastsszHashRootWith); ok {
			hh := hasher.NewHasher()
			err := m.HashTreeRootWith(hh)
			root := ""
			if err == nil {
				var r [32]byte
				r, err = hh.HashRoot()
				root = hex.EncodeToString(r[:])
			}
			out = append(out, res{"generated/HashTreeRootWith", root, err})
		}
	}
	if m, ok := v.(sszutils.DynamicHashRoot); ok {
		hh := hasher.NewHasher()
		err := m.HashTreeRootWithDyn(e.gen, hh)
		root := ""
		if err == nil {
			var r [32]byte
			r, err = hh.HashRoot()
			root = hex.EncodeToString(r[:])
		}
		out = append(out, res{"generated/HashTreeRootWithDyn", root, err})
	}
	for _, r := range out {
		if r.err != nil {
			fail("ROOT PATH ERROR %s: %v", r.name, r.err)
			continue
		}
		if want != "" && r.root != want {
			fail("ROOT DIVERGENCE %s=%s model=%s", r.name, r.root, want)
		}
	}
	return out[0].root
}

func shortHex(b []byte) string {
	h := hex.EncodeToString(b)
	if len(h) > 200 {
		return h[:200] + fmt.Sprintf("…(%d bytes)", len(b))
	}
	return h
}

// failer adapts testing.TB to a pathFailure with a context prefix. A failure
// of a type listed in knownIssues whose message contains the listed text is
// logged as a known issue instead of failing the test.
func failer(t testing.TB, ctx string) pathFailure {
	return func(format string, args ...any) {
		t.Helper()
		msg := fmt.Sprintf(format, args...)
		for name, needles := range knownIssues {
			if !strings.Contains(ctx, name) {
				continue
			}
			for _, needle := range needles {
				if strings.Contains(msg, needle) {
					t.Logf("KNOWN ISSUE %s: %s", ctx, msg)
					return
				}
			}
		}
		t.Errorf("%s: %s", ctx, msg)
	}
}
