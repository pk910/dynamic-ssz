package harness

import (
	"encoding/hex"
	"testing"
)

// TestExtendedEdgeCases pins the non-canonical encodings of the extended
// types: every engine path must reach the certified model's verdict.
func TestExtendedEdgeCases(t *testing.T) {
	o := startCertOracle(t)
	eng := newEngineSet(nil)
	ctor := map[string]func() any{}
	for name, e := range extendedTypes {
		ctor[name] = e.New
	}
	cases := []struct {
		typ, desc, hex string
		accept         bool
	}{
		// ExtBig = {Bounded big.Int (offset), Unbounded big.Int (offset), Tag uint16}
		{"ExtBig", "zero, zero", "0a0000000b0000000500" + "00" + "00", true},
		{"ExtBig", "negative zero rejected", "0a0000000b0000000500" + "01" + "00", false},
		{"ExtBig", "leading zero magnitude rejected", "0a0000000c0000000500" + "0000" + "00", false},
		{"ExtBig", "sign byte 2 rejected", "0a0000000b0000000500" + "02" + "00", false},
		{"ExtBig", "empty payload rejected", "0a0000000a0000000500" + "" + "00", false},
		{"ExtBig", "negative one", "0a0000000c0000000500" + "0101" + "00", true},
		// ExtOptionals = {A *uint64 (off), B *[4]byte (off), C *ExtInner (off), D uint8}
		{"ExtOptionals", "all absent", "0d0000000e0000000f00000007" + "00" + "00" + "00", true},
		{"ExtOptionals", "absent with trailing byte rejected", "0d0000000f0000001000000007" + "00ff" + "00" + "00", false},
		{"ExtOptionals", "flag 2 rejected", "0d0000000e0000000f00000007" + "02" + "00" + "00", false},
		{"ExtOptionals", "flag 2 with a payload rejected", "0d000000160000001700000007" + "02" + "0102030405060708" + "00" + "00", false},
		{"ExtOptionals", "flag 0 with a payload rejected", "0d000000160000001700000007" + "00" + "0102030405060708" + "00" + "00", false},
		{"ExtOptionals", "empty region rejected", "0d0000000d0000000e00000007" + "" + "00" + "00", false},
		{"ExtOptionals", "present uint64", "0d000000160000001700000007" + "01" + "0102030405060708" + "00" + "00", true},
		{"ExtOptionals", "present uint64 short rejected", "0d000000150000001600000007" + "01" + "01020304050607" + "00" + "00", false},
		// ExtOptionalLists = {A (off), B (off), C (off)} as List[T,1]
		{"ExtOptionalLists", "all empty", "0c0000000c0000000c000000", true},
		{"ExtOptionalLists", "two uint32 rejected (limit 1)", "0c000000140000001400000001000000" + "02000000", false},
	}
	for _, c := range cases {
		raw, err := hex.DecodeString(c.hex)
		if err != nil {
			t.Fatalf("%s/%s: bad hex", c.typ, c.desc)
		}
		ctx := c.typ + "/" + c.desc
		fail := failer(t, ctx)
		goAccept, decoded := eng.decodePaths(fail, ctor[c.typ], raw)
		leanAccept, root := o.verdict(c.typ, raw)
		if leanAccept != c.accept {
			t.Errorf("%s: model verdict %v, case expects %v", ctx, leanAccept, c.accept)
		}
		if goAccept != leanAccept {
			t.Errorf("VERDICT DIVERGENCE %s: go=%v lean=%v", ctx, goAccept, leanAccept)
		}
		if goAccept && leanAccept {
			eng.rootPaths(fail, decoded, root)
		}
	}
}
