// Package apigen holds a few types the code generator's programmatic API and
// configuration file are driven over (TestCodegenAPI). The generated files
// are written here during the test and removed afterwards.
package apigen

import "math/big"

// Inner is a small variable-size container.
type Inner struct {
	A uint32
	B []uint16 `ssz-max:"20"`
}

// Outer nests Inner through every collection and a spec-parametric size.
type Outer struct {
	Flag   bool
	Roots  [][32]byte `ssz-max:"8"`
	Items  []*Inner   `ssz-max:"4"`
	Fixed  [3]uint64
	Bits   []byte   `ssz-type:"bitlist" ssz-max:"100"`
	Sized  []byte   `ssz-size:"4" dynssz-size:"SPEC_A"`
	Prog   []uint64 `ssz-type:"progressive-list"`
	Chosen Inner
}

// Ext uses the extended kinds.
type Ext struct {
	N   int64
	F   float32
	Big *big.Int `ssz-max:"16"`
	Opt *Inner   `ssz-type:"optional"`
}

// Rec is recursive through a list.
type Rec struct {
	V    uint8
	Kids []*Rec `ssz-max:"3"`
}

// OuterView is a schema view of Outer without its last two fields.
type OuterView struct {
	Flag  bool
	Roots [][32]byte `ssz-max:"8"`
	Items []*Inner   `ssz-max:"4"`
	Fixed [3]uint64
	Bits  []byte `ssz-type:"bitlist" ssz-max:"100"`
	Sized []byte `ssz-size:"4" dynssz-size:"SPEC_A"`
}
