package harness

import (
	"reflect"
	"strings"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
)

// TestTagRefusals drives the descriptor builder through declarations it must
// refuse — every documented class of invalid tag — built at run time with
// reflect.StructOf. Each must fail with an error (never panic, never a
// descriptor), and the error must name the fault. The refusal paths of the
// tag parser and the type cache are otherwise unreachable from valid types.
//
// Two declarations are deliberately absent because the library accepts them
// by design: a spec name the spec set does not define falls back to the
// static size (`dynssz-max:"NOT_A_SPEC"` with `ssz-max:"4"` is List[…, 4]),
// and a list without a limit still has a (non-standard, length-mixed) root.
func TestTagRefusals(t *testing.T) {
	ds := dynssz.NewDynSsz(typegenSpecs(), dynssz.WithNoDelegation(), dynssz.WithExtendedTypes())
	type fault struct {
		name  string
		typ   reflect.Type
		tag   string
		wants string // a fragment the error must contain
	}
	field := func(typ reflect.Type, tag string) reflect.Type {
		return reflect.StructOf([]reflect.StructField{{Name: "F", Type: typ, Tag: reflect.StructTag(tag)}})
	}
	bytesT := reflect.TypeOf([]byte(nil))
	u64T := reflect.TypeOf(uint64(0))
	arr2T := reflect.TypeOf([2]byte{})
	ptrU64 := reflect.TypeOf((*uint64)(nil))
	faults := []fault{
		{"unknown ssz-type", bytesT, `ssz-type:"frobnicate"`, "ssz-type"},
		{"bitsize on a non-bitfield", u64T, `ssz-bitsize:"13"`, "bits"},
		{"ssz-size of a byte vector on uint64", u64T, `ssz-size:"32"`, "size"},
		{"ssz-size and dynssz-size disagree in kind", bytesT, `ssz-size:"32" dynssz-size:"?"`, "conflicting"},
		{"dynssz-size literal not repeating ssz-size", bytesT, `ssz-size:"32" dynssz-size:"33"`, "conflicting"},
		{"malformed expression", bytesT, `ssz-max:"4" dynssz-max:"SPEC_A*"`, ""},
		{"division by zero", bytesT, `ssz-max:"4" dynssz-max:"SPEC_A/0"`, "zero"},
		{"modulo by zero", bytesT, `ssz-max:"4" dynssz-max:"SPEC_A%0"`, "zero"},
		{"negative size", bytesT, `ssz-size:"-1"`, ""},
		{"non-numeric size", bytesT, `ssz-size:"many"`, ""},
		{"optional on a non-pointer", u64T, `ssz-type:"optional"`, "pointer"},
		{"optional-list on a non-pointer", u64T, `ssz-type:"optional-list"`, "pointer"},
		{"uint128 on two bytes", arr2T, `ssz-type:"uint128"`, "uint128"},
		{"uint256 on two bytes", arr2T, `ssz-type:"uint256"`, "uint256"},
		{"bitvector without a bit size", bytesT, `ssz-type:"bitvector"`, ""},
		{"bitvector bits beyond the bytes", arr2T, `ssz-type:"bitvector" ssz-bitsize:"17"`, ""},
		{"too many dimensions", bytesT, `ssz-size:"1,2,3"`, ""},
		{"custom without methods", u64T, `ssz-type:"custom"`, "custom"},
		{"progressive-list on a non-slice", arr2T, `ssz-type:"progressive-list"`, ""},
		{"bitlist on a fixed array", arr2T, `ssz-type:"bitlist" ssz-max:"9"`, ""},
		{"container on uint64", u64T, `ssz-type:"container"`, ""},
		{"vector on a pointer", ptrU64, `ssz-type:"vector" ssz-size:"3"`, ""},
	}
	refused := 0
	for _, f := range faults {
		f := f
		t.Run(f.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic for %q: %v", f.tag, r)
				}
			}()
			typ := field(f.typ, f.tag)
			desc, err := ds.GetTypeCache().GetTypeDescriptor(typ, nil, nil, nil)
			if err == nil {
				t.Errorf("declaration accepted: %s %q -> %v", f.typ, f.tag, desc.SszType)
				return
			}
			if f.wants != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(f.wants)) {
				t.Errorf("refusal of %q does not name %q: %v", f.tag, f.wants, err)
			}
			refused++
		})
	}
	t.Logf("tag refusals: %d/%d invalid declarations refused", refused, len(faults))
}

func typegenSpecs() map[string]any { return map[string]any{"SPEC_A": uint64(5)} }
