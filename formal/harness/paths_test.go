package harness

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

// engineSet is every dynamic-ssz configuration the link holds to the
// verified models: both engines, each with synchronous and asynchronous
// hashing, the native sha256 backend next to the SIMD one, and tiny stream
// buffers that force the streaming decoder through open regions.
type engineSet struct {
	gen, refl           *dynssz.DynSsz
	genAsync, reflAsync *dynssz.DynSsz
	reflNative          *dynssz.DynSsz
	genTiny, reflTiny   *dynssz.DynSsz
}

func newEngineSet(specs map[string]any) *engineSet {
	ext := dynssz.WithExtendedTypes()
	noDel := dynssz.WithNoDelegation()
	async := dynssz.WithAsyncHashing(4)
	tiny := []dynssz.DynSszOption{dynssz.WithStreamReaderBufferSize(8), dynssz.WithStreamWriterBufferSize(8)}
	return &engineSet{
		gen:        dynssz.NewDynSsz(specs, ext),
		refl:       dynssz.NewDynSsz(specs, ext, noDel),
		genAsync:   dynssz.NewDynSsz(specs, ext, async),
		reflAsync:  dynssz.NewDynSsz(specs, ext, noDel, async),
		reflNative: dynssz.NewDynSsz(specs, ext, noDel, dynssz.WithNoFastHash()),
		genTiny:    dynssz.NewDynSsz(specs, append([]dynssz.DynSszOption{ext}, tiny...)...),
		reflTiny:   dynssz.NewDynSsz(specs, append([]dynssz.DynSszOption{ext, noDel}, tiny...)...),
	}
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

		size, err := eng.ds.SizeSSZ(v)
		if err == nil && enc != nil && size != len(enc) {
			fail("%s/SizeSSZ = %d, encoding has %d bytes", eng.name, size, len(enc))
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
		{"reflection-async", e.reflAsync}, {"reflection-native-sha256", e.reflNative}} {
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

// failer adapts testing.TB to a pathFailure with a context prefix.
func failer(t testing.TB, ctx string) pathFailure {
	return func(format string, args ...any) {
		t.Helper()
		t.Errorf("%s: "+format, append([]any{ctx}, args...)...)
	}
}
