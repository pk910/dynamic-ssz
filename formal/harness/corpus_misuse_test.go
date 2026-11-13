package harness

import (
	"bytes"
	"reflect"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/formal/harness/typegen"
	"github.com/pk910/dynamic-ssz/ssztypes"
)

// TestCorpusMisuse drives both engines through values a caller can build but
// SSZ cannot encode — a nil root, a list beyond its limit, a slice-backed
// vector longer than declared, a bit list with too many bits — for every
// corpus type the misuse applies to. Both engines must refuse (never panic,
// never encode) and agree with each other; a slice-backed vector shorter
// than declared is zero-padded, identically by both.
func TestCorpusMisuse(t *testing.T) {
	entries := corpusEntries(t)
	eng := newEngineSet(typegen.Specs())
	engines := []struct {
		name string
		ds   *dynssz.DynSsz
	}{{"codegen", eng.gen}, {"reflection", eng.refl}}
	tried := 0

	// refuse checks that every engine refuses v on every encoding path.
	refuse := func(t *testing.T, what string, v any) {
		t.Helper()
		tried++
		// SizeSSZ: the engines currently disagree on unencodable values
		// (ISSUES L9: generated SizeSSZ does not check limits); report, do not fail.
		sizeErrs := make([]error, 0, len(engines))
		for _, e := range engines {
			_, err := e.ds.SizeSSZ(v)
			sizeErrs = append(sizeErrs, err)
		}
		if (sizeErrs[0] == nil) != (sizeErrs[1] == nil) {
			t.Logf("KNOWN ISSUE L9 %s: SizeSSZ codegen err=%v reflection err=%v", what, sizeErrs[0], sizeErrs[1])
		}
		for _, e := range engines {
			if enc, err := e.ds.MarshalSSZ(v); err == nil {
				t.Errorf("%s: %s/MarshalSSZ encoded it (%d bytes)", what, e.name, len(enc))
			}
			if _, err := e.ds.MarshalSSZTo(v, nil); err == nil {
				t.Errorf("%s: %s/MarshalSSZTo encoded it", what, e.name)
			}
			if err := e.ds.MarshalSSZWriter(v, &oneByteWriter{}); err == nil {
				t.Errorf("%s: %s/MarshalSSZWriter encoded it", what, e.name)
			}
			if _, err := e.ds.HashTreeRoot(v); err == nil {
				t.Errorf("%s: %s/HashTreeRoot rooted it", what, e.name)
			}
		}
	}

	for _, e := range entries {
		if e.Recursive {
			continue
		}
		rt := reflect.TypeOf(e.New()).Elem()
		desc, err := eng.refl.GetTypeCache().GetTypeDescriptor(rt, nil, nil, nil)
		if err != nil {
			continue
		}
		// A typed nil root: both engines must agree (they encode the zero
		// value or refuse alike).
		nilRoot := reflect.Zero(reflect.PointerTo(rt)).Interface()
		var first []byte
		var firstErr error
		for i, en := range engines {
			enc, err := en.ds.MarshalSSZ(nilRoot)
			if i == 0 {
				first, firstErr = enc, err
				continue
			}
			if (err == nil) != (firstErr == nil) || !bytes.Equal(enc, first) {
				t.Errorf("%s: nil root: %s=(%x,%v) %s=(%x,%v)", e.Name, engines[0].name, first, firstErr, en.name, enc, err)
			}
		}
		tried++

		// Lists beyond their limit; slice-backed vectors of the wrong length;
		// bit lists with too many bits. Each from a fresh zero value.
		switch desc.SszType {
		case ssztypes.SszListType, ssztypes.SszProgressiveListType:
			if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 || desc.Limit > 1<<12 || desc.Kind != reflect.Slice {
				break
			}
			if desc.ElemDesc != nil && desc.ElemDesc.SszType == ssztypes.SszCustomType {
				break
			}
			v := reflect.New(rt)
			v.Elem().Set(reflect.MakeSlice(rt, int(desc.Limit)+1, int(desc.Limit)+1))
			refuse(t, e.Name+": list of limit+1 elements", v.Interface())
		case ssztypes.SszVectorType:
			if desc.Kind != reflect.Slice || desc.Len <= 0 || desc.Len > 1<<12 {
				break
			}
			// A slice-backed vector: longer than declared is refused; shorter
			// is zero-padded to the declared length (documented), identically
			// by both engines.
			long := reflect.New(rt)
			long.Elem().Set(reflect.MakeSlice(rt, int(desc.Len)+1, int(desc.Len)+1))
			refuse(t, e.Name+": slice vector longer than declared", long.Interface())
			short := reflect.New(rt)
			short.Elem().Set(reflect.MakeSlice(rt, int(desc.Len)-1, int(desc.Len)-1))
			full := reflect.New(rt)
			full.Elem().Set(reflect.MakeSlice(rt, int(desc.Len), int(desc.Len)))
			want, werr := eng.gen.MarshalSSZ(full.Interface())
			for _, en := range engines {
				enc, err := en.ds.MarshalSSZ(short.Interface())
				if werr != nil || err != nil || !bytes.Equal(enc, want) {
					t.Errorf("%s: %s: a short slice vector should encode zero-padded (%v, %v)", e.Name, en.name, err, werr)
				}
			}
			tried++
		case ssztypes.SszBitlistType:
			if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 || desc.Limit > 1<<15 || desc.Kind != reflect.Slice {
				break
			}
			bits := int(desc.Limit) + 1
			b := make([]byte, bits/8+1)
			b[bits/8] = 1 << (bits % 8)
			v := reflect.New(rt)
			v.Elem().Set(reflect.ValueOf(b).Convert(rt))
			refuse(t, e.Name+": bit list of limit+1 bits", v.Interface())
		case ssztypes.SszBitvectorType:
			if desc.Kind != reflect.Slice || desc.Size <= 0 {
				break
			}
			v := reflect.New(rt)
			v.Elem().Set(reflect.MakeSlice(rt, int(desc.Size)+1, int(desc.Size)+1))
			refuse(t, e.Name+": slice bit vector longer than declared", v.Interface())
		default:
		}
	}
	t.Logf("misuse: %d values refused or agreed on across %d corpus types", tried, len(entries))
}
