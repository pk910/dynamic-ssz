// Package typegen builds SSZ type shapes — every kind dynamic-ssz encodes,
// in every Go representation and property class the engines distinguish —
// and writes them as Go source with the declaration each one is meant to
// have. The corpus it produces is linked generically: the spec oracle takes
// the declaration at run time, so no type needs a certificate file.
package typegen

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is an SSZ kind dynamic-ssz encodes (the SSZ specification's kinds and
// dynamic-ssz's extended ones).
type Kind int

// The kinds.
const (
	Bool Kind = iota
	Uint8
	Uint16
	Uint32
	Uint64
	Uint128
	Uint256
	Int8
	Int16
	Int32
	Int64
	Float32
	Float64
	BigInt
	Time
	ByteVector
	ByteList
	BitVector
	BitList
	ProgBitList
	Vector
	List
	ProgList
	Container
	ProgContainer
	CompatUnion
	Union
	Optional
	OptionalList
	Wrapper
)

var kindNames = [...]string{"Bool", "Uint8", "Uint16", "Uint32", "Uint64", "Uint128", "Uint256",
	"Int8", "Int16", "Int32", "Int64", "Float32", "Float64", "BigInt", "Time",
	"ByteVector", "ByteList", "BitVector", "BitList", "ProgBitList", "Vector", "List", "ProgList",
	"Container", "ProgContainer", "CompatUnion", "Union", "Optional", "OptionalList", "Wrapper"}

func (k Kind) String() string { return kindNames[k] }

// Rep selects the Go representation of a shape where dynamic-ssz accepts
// several, each taking its own code path.
type Rep int

// The representations.
const (
	RepDefault Rep = iota
	RepSlice       // a vector as a slice with ssz-size
	RepString      // a byte vector or byte list as a string
	RepWords       // uint128/uint256 as [2]uint64 / [4]uint64
	RepPtrElem     // collection elements as pointers
	RepDims        // a two-dimensional byte collection declared with dimension tags
)

var repNames = [...]string{"", "Slice", "String", "Words", "PtrElem", "Dims"}

// Field is a container field.
type Field struct {
	Name  string
	Shape *Shape
	Index int // ssz-index of a progressive container field; -1 otherwise
}

// Option is a union option. A classic union's None option has a nil Shape.
type Option struct {
	Selector int
	Shape    *Shape
}

// Shape is one SSZ type shape.
type Shape struct {
	Kind    Kind
	N       uint64 // length (vectors, bit vectors: bits), limit (lists, bit lists), max bytes (big.Int)
	Limited bool   // progressive collections and big.Int: whether N bounds them
	Elem    *Shape
	Fields  []Field
	Options []Option
	Rep     Rep
	// Spec is a spec-value expression (dynssz-size / dynssz-max / dynssz-bitsize)
	// whose value under Specs is Resolved; N is the static default then.
	Spec     string
	Resolved uint64
	Label    string
	// IllFormed marks a shape the SSZ specification declares illegal although
	// dynamic-ssz accepts it; the spec oracle must refuse its declaration.
	IllFormed bool
}

// Specs are the spec values the corpus is linked under. Every expression's
// value differs from its static default, so the spec-parametric paths of both
// engines (and the *Dyn generated methods) are taken.
func Specs() map[string]any {
	return map[string]any{"SPEC_A": uint64(5), "SPEC_B": uint64(48), "SPEC_C": uint64(3), "SPEC_F": float64(3)}
}

// SpecExprs are expressions over Specs: expression, static default, value.
var SpecExprs = []struct {
	Expr     string
	Default  uint64
	Resolved uint64
}{
	{"SPEC_A", 4, 5},
	{"SPEC_A*8", 32, 40},
	{"(SPEC_B+SPEC_A+1)/2", 16, 27},
	{"SPEC_B%SPEC_A", 4, 3},
	{"SPEC_C*SPEC_C*SPEC_C-2", 20, 25},
	{"SPEC_B/SPEC_C/2", 6, 8},
	{"SPEC_F*4", 8, 12},
}

// Size returns the resolved length or limit.
func (s *Shape) Size() uint64 {
	if s.Spec != "" && !staticDecl {
		return s.Resolved
	}
	return s.N
}

// staticDecl switches Decl to the static defaults of spec-parametric sizes
// (what the code generator sees before any spec value is resolved).
var staticDecl bool

// DeclStatic is Decl with every spec-parametric size at its static default.
func (s *Shape) DeclStatic() string {
	staticDecl = true
	defer func() { staticDecl = false }()
	return s.Decl()
}

// Extended reports whether the shape, or anything in it, is outside the SSZ
// specification (so it has no EthLib declaration).
func (s *Shape) Extended() bool {
	switch s.Kind {
	case Int8, Int16, Int32, Int64, Float32, Float64, BigInt, Union, Optional:
		return true
	case Bool, Uint8, Uint16, Uint32, Uint64, Uint128, Uint256, Time, ByteVector, ByteList, BitVector, BitList, ProgBitList:
		return false
	case Vector, List, ProgList, OptionalList, Wrapper:
		return s.Elem.Extended()
	case Container, ProgContainer:
		for _, f := range s.Fields {
			if f.Shape.Extended() {
				return true
			}
		}
		return false
	case CompatUnion:
		for _, o := range s.Options {
			if o.Shape.Extended() {
				return true
			}
		}
		return false
	}
	panic("unreachable")
}

// Variable reports whether the shape has a variable-size encoding.
func (s *Shape) Variable() bool {
	switch s.Kind {
	case ByteList, BitList, ProgBitList, List, ProgList, CompatUnion, Union, Optional, OptionalList, BigInt:
		return true
	case Vector:
		return s.Elem.Variable()
	case Wrapper:
		return s.Elem.Variable()
	case Container, ProgContainer:
		for _, f := range s.Fields {
			if f.Shape.Variable() {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// SpecParametric reports whether the shape, or anything in it, carries a
// spec-value expression.
func (s *Shape) SpecParametric() bool {
	if s.Spec != "" {
		return true
	}
	if s.Elem != nil && s.Elem.SpecParametric() {
		return true
	}
	for _, f := range s.Fields {
		if f.Shape.SpecParametric() {
			return true
		}
	}
	for _, o := range s.Options {
		if o.Shape != nil && o.Shape.SpecParametric() {
			return true
		}
	}
	return false
}

// Decl is the declaration the shape is meant to have, in the JSON spelling
// the spec oracle reads ("" for an extended shape, which has none). It is
// computed from the shape alone, never from dynamic-ssz: the link compares it
// with the declaration dynamic-ssz derives from the Go type's tags.
func (s *Shape) Decl() string {
	if s.Extended() {
		return ""
	}
	return s.decl()
}

func (s *Shape) decl() string {
	switch s.Kind {
	case Bool:
		return `{"kind":"Boolean"}`
	case Uint8:
		return `{"kind":"Uint8","bits":8}`
	case Uint16:
		return `{"kind":"Uint16","bits":16}`
	case Uint32:
		return `{"kind":"Uint32","bits":32}`
	case Uint64, Time:
		return `{"kind":"Uint64","bits":64}`
	case Uint128:
		return `{"kind":"Uint128","bits":128}`
	case Uint256:
		return `{"kind":"Uint256","bits":256}`
	case ByteVector:
		return fmt.Sprintf(`{"kind":"ByteVector","length":%d}`, s.Size())
	case ByteList:
		return fmt.Sprintf(`{"kind":"ByteList","limit":%d}`, s.Size())
	case BitVector:
		return fmt.Sprintf(`{"kind":"BitVector","length":%d}`, s.Size())
	case BitList:
		return fmt.Sprintf(`{"kind":"BitList","limit":%d}`, s.Size())
	case ProgBitList:
		if s.Limited {
			return fmt.Sprintf(`{"kind":"ProgressiveBitList","limit":%d}`, s.Size())
		}
		return `{"kind":"ProgressiveBitList"}`
	case Vector:
		if s.Elem.Kind == Uint8 {
			return fmt.Sprintf(`{"kind":"ByteVector","length":%d}`, s.Size())
		}
		return fmt.Sprintf(`{"kind":"Vector","length":%d,"elementType":%s}`, s.Size(), s.Elem.decl())
	case List:
		if s.Elem.Kind == Uint8 {
			return fmt.Sprintf(`{"kind":"ByteList","limit":%d}`, s.Size())
		}
		return fmt.Sprintf(`{"kind":"List","limit":%d,"elementType":%s}`, s.Size(), s.Elem.decl())
	case OptionalList:
		if s.Elem.Kind == Uint8 {
			return `{"kind":"ByteList","limit":1}`
		}
		return fmt.Sprintf(`{"kind":"List","limit":1,"elementType":%s}`, s.Elem.decl())
	case ProgList:
		limit := ""
		if s.Limited {
			limit = fmt.Sprintf(`,"limit":%d`, s.Size())
		}
		return `{"kind":"ProgressiveList"` + limit + `,"elementType":` + s.Elem.decl() + `}`
	case Wrapper:
		return s.Elem.decl()
	case Container, ProgContainer:
		fields := make([]string, len(s.Fields))
		maxIdx := -1
		for i, f := range s.Fields {
			fields[i] = `{"name":` + strconv.Quote(f.Name) + `,"type":` + f.Shape.decl() + `}`
			if f.Index > maxIdx {
				maxIdx = f.Index
			}
		}
		if s.Kind == ProgContainer {
			layout := make([]string, maxIdx+1)
			for i := range layout {
				layout[i] = "0"
			}
			for _, f := range s.Fields {
				layout[f.Index] = "1"
			}
			return `{"kind":"ProgressiveContainer","activeFields":[` + strings.Join(layout, ",") +
				`],"fields":[` + strings.Join(fields, ",") + `]}`
		}
		return `{"kind":"Container","fields":[` + strings.Join(fields, ",") + `]}`
	case CompatUnion:
		opts := make([]string, len(s.Options))
		for i, o := range s.Options {
			opts[i] = fmt.Sprintf(`{"selector":%d,"type":%s}`, o.Selector, o.Shape.decl())
		}
		return `{"kind":"CompatibleUnion","options":[` + strings.Join(opts, ",") + `]}`
	}
	panic("extended shape has no declaration")
}

// Describe renders the shape for a report.
func (s *Shape) Describe() string {
	var b strings.Builder
	s.describe(&b)
	return b.String()
}

func (s *Shape) describe(b *strings.Builder) {
	b.WriteString(s.Kind.String())
	if s.Rep != RepDefault {
		b.WriteString("/" + repNames[s.Rep])
	}
	switch s.Kind {
	case ByteVector, ByteList, BitVector, BitList, BigInt:
		if s.Kind != BigInt || s.Limited {
			fmt.Fprintf(b, "[%d]", s.N)
		}
	case ProgBitList:
		if s.Limited {
			fmt.Fprintf(b, "[%d]", s.N)
		}
	case Vector, List:
		fmt.Fprintf(b, "[%d](", s.N)
		s.Elem.describe(b)
		b.WriteString(")")
	case ProgList:
		b.WriteString("(")
		s.Elem.describe(b)
		b.WriteString(")")
		if s.Limited {
			fmt.Fprintf(b, "[%d]", s.N)
		}
	case Optional, OptionalList, Wrapper:
		b.WriteString("(")
		s.Elem.describe(b)
		b.WriteString(")")
	case Container, ProgContainer:
		b.WriteString("{")
		for i, f := range s.Fields {
			if i > 0 {
				b.WriteString(",")
			}
			if f.Index >= 0 {
				fmt.Fprintf(b, "%d:", f.Index)
			}
			f.Shape.describe(b)
		}
		b.WriteString("}")
	case CompatUnion, Union:
		b.WriteString("{")
		for i, o := range s.Options {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(b, "%d:", o.Selector)
			if o.Shape == nil {
				b.WriteString("None")
			} else {
				o.Shape.describe(b)
			}
		}
		b.WriteString("}")
	default:
	}
	if s.Spec != "" {
		fmt.Fprintf(b, "<%s=%d>", s.Spec, s.Resolved)
	}
}

// Leaf constructors.

// Basic returns a basic shape.
func Basic(k Kind) *Shape { return &Shape{Kind: k} }

// Bytes returns ByteVector[n].
func Bytes(n uint64) *Shape { return &Shape{Kind: ByteVector, N: n} }

// ByteListOf returns ByteList[limit].
func ByteListOf(limit uint64) *Shape { return &Shape{Kind: ByteList, N: limit, Limited: true} }

// Bits returns Bitvector[n].
func Bits(n uint64) *Shape { return &Shape{Kind: BitVector, N: n} }

// BitListOf returns Bitlist[limit].
func BitListOf(limit uint64) *Shape { return &Shape{Kind: BitList, N: limit, Limited: true} }

// VectorOf returns Vector[elem, n].
func VectorOf(elem *Shape, n uint64) *Shape { return &Shape{Kind: Vector, N: n, Elem: elem} }

// ListOf returns List[elem, limit].
func ListOf(elem *Shape, limit uint64) *Shape {
	return &Shape{Kind: List, N: limit, Limited: true, Elem: elem}
}

// ProgListOf returns ProgressiveList[elem] with an optional limit.
func ProgListOf(elem *Shape, limit uint64) *Shape {
	return &Shape{Kind: ProgList, N: limit, Limited: limit > 0, Elem: elem}
}

// ContainerOf returns a container of the shapes, fields named F0, F1, ….
func ContainerOf(fields ...*Shape) *Shape {
	c := &Shape{Kind: Container}
	for i, f := range fields {
		c.Fields = append(c.Fields, Field{Name: fmt.Sprintf("F%d", i), Shape: f, Index: -1})
	}
	return c
}

// ProgContainerOf returns a progressive container whose fields sit at the
// given active indices.
func ProgContainerOf(indices []int, fields ...*Shape) *Shape {
	c := &Shape{Kind: ProgContainer}
	for i, f := range fields {
		c.Fields = append(c.Fields, Field{Name: fmt.Sprintf("F%d", i), Shape: f, Index: indices[i]})
	}
	return c
}

// CompatUnionOf returns a CompatibleUnion with the given selectors.
func CompatUnionOf(selectors []int, options ...*Shape) *Shape {
	u := &Shape{Kind: CompatUnion}
	for i, o := range options {
		u.Options = append(u.Options, Option{Selector: selectors[i], Shape: o})
	}
	return u
}

// IncompatibleUnion marks a CompatibleUnion whose options are not mutually
// compatible, which EIP-8016 forbids.
func (s *Shape) Illegal() *Shape { c := *s; c.IllFormed = true; return &c }

// UnionOf returns a classic Union; withNone adds the None option at 0.
func UnionOf(withNone bool, options ...*Shape) *Shape {
	u := &Shape{Kind: Union}
	sel := 0
	if withNone {
		u.Options = append(u.Options, Option{Selector: 0})
		sel = 1
	}
	for _, o := range options {
		u.Options = append(u.Options, Option{Selector: sel, Shape: o})
		sel++
	}
	return u
}

// OptionalOf returns optional[elem].
func OptionalOf(elem *Shape) *Shape { return &Shape{Kind: Optional, Elem: elem} }

// OptionalListOf returns a pointer encoded as List[elem, 1].
func OptionalListOf(elem *Shape) *Shape { return &Shape{Kind: OptionalList, Elem: elem} }

// WrapperOf returns a TypeWrapper around elem.
func WrapperOf(elem *Shape) *Shape { return &Shape{Kind: Wrapper, Elem: elem} }

// WithRep sets the representation.
func (s *Shape) WithRep(r Rep) *Shape { c := *s; c.Rep = r; return &c }

// WithSpec makes the length or limit spec-parametric.
func (s *Shape) WithSpec(i int) *Shape {
	c := *s
	c.Spec = SpecExprs[i].Expr
	c.N = SpecExprs[i].Default
	c.Resolved = SpecExprs[i].Resolved
	return &c
}

// Labeled names the shape's class.
func (s *Shape) Labeled(l string) *Shape { c := *s; c.Label = l; return &c }
