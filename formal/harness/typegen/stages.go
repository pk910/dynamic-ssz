package typegen

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
)

// Leaves is the full catalogue of leaf shapes: every kind in every
// representation and at every size class the engines distinguish (byte,
// chunk, power-of-two-chunk and batch boundaries; aligned and unaligned bit
// widths; static and spec-parametric sizes; bounded and unbounded).
func Leaves() []*Shape {
	var out []*Shape
	add := func(label string, s *Shape) { out = append(out, s.Labeled(label)) }

	add("Bool", Basic(Bool))
	for _, k := range []Kind{Uint8, Uint16, Uint32, Uint64} {
		add(k.String(), Basic(k))
	}
	for _, k := range []Kind{Uint128, Uint256} {
		add(k.String(), Basic(k))
		add(k.String()+"Words", Basic(k).WithRep(RepWords))
	}
	for _, k := range []Kind{Int8, Int16, Int32, Int64, Float32, Float64} {
		add(k.String(), Basic(k))
	}
	add("Time", Basic(Time))
	add("BigInt33", &Shape{Kind: BigInt, N: 33, Limited: true})
	add("BigIntUnbounded", &Shape{Kind: BigInt})

	for _, n := range []uint64{1, 31, 32, 33, 64, 96, 100, 2048} {
		add(fmt.Sprintf("Bytes%d", n), Bytes(n))
	}
	add("Bytes32Slice", Bytes(32).WithRep(RepSlice))
	add("Bytes20String", Bytes(20).WithRep(RepString))
	add("Dims3x16", VectorOf(Bytes(16), 3).WithRep(RepDims))
	for i := range SpecExprs {
		add(fmt.Sprintf("BytesSpec%d", i), Bytes(0).WithSpec(i))
	}
	for _, n := range []uint64{1, 31, 32, 33, 64, 1024, 1 << 20, 1 << 40} {
		add(fmt.Sprintf("ByteList%d", n), ByteListOf(n))
	}
	add("ByteList64String", ByteListOf(64).WithRep(RepString))
	add("Dims2xByteList40", VectorOf(ByteListOf(40), 2).WithRep(RepDims))
	add("ByteListSpec1", ByteListOf(0).WithSpec(1))

	for _, n := range []uint64{1, 7, 8, 9, 63, 64, 65, 255, 256, 257, 512} {
		add(fmt.Sprintf("Bits%d", n), Bits(n))
	}
	add("Bits12Slice", Bits(12).WithRep(RepSlice))
	add("BitsSpec2", Bits(0).WithSpec(2))
	for _, n := range []uint64{1, 8, 9, 64, 255, 256, 257, 2048, 1 << 30} {
		add(fmt.Sprintf("BitList%d", n), BitListOf(n))
	}
	add("BitListSpec1", BitListOf(0).WithSpec(1))
	add("ProgBitList", &Shape{Kind: ProgBitList})
	add("ProgBitList300", &Shape{Kind: ProgBitList, N: 300, Limited: true})

	for _, k := range []Kind{Bool, Uint16, Uint64, Uint256} {
		for _, n := range []uint64{1, 3, 4, 5, 33} {
			add(fmt.Sprintf("Vector%d%s", n, k), VectorOf(Basic(k), n))
		}
	}
	add("Vector4Uint32Slice", VectorOf(Basic(Uint32), 4).WithRep(RepSlice))
	add("Vector5Uint16Spec0", VectorOf(Basic(Uint16), 0).WithSpec(0))
	add("Vector3Bytes32", VectorOf(Bytes(32), 3))
	add("Vector2Bits5", VectorOf(Bits(5), 2))
	add("Vector3Int32", VectorOf(Basic(Int32), 3))
	add("Vector4Float64", VectorOf(Basic(Float64), 4))

	for _, k := range []Kind{Bool, Uint8, Uint16, Uint64, Uint128} {
		for _, n := range []uint64{1, 2, 4, 100, 1 << 20, 1 << 40} {
			if k == Uint8 && n < 4 {
				continue // byte lists have their own entries
			}
			add(fmt.Sprintf("List%d%s", n, k), ListOf(Basic(k), n))
		}
	}
	add("List300Uint64Spec3", ListOf(Basic(Uint64), 0).WithSpec(3))
	add("List5Bytes32", ListOf(Bytes(32), 5))
	add("List3Bits9", ListOf(Bits(9), 3))
	add("List4Int64", ListOf(Basic(Int64), 4))
	add("List4Float32", ListOf(Basic(Float32), 4))
	add("ProgListUint8", ProgListOf(Basic(Uint8), 0))
	add("ProgListUint64", ProgListOf(Basic(Uint64), 0))
	add("ProgListUint64Lim100", ProgListOf(Basic(Uint64), 100))
	add("ProgListBytes32", ProgListOf(Bytes(32), 0))
	add("ProgListBool", ProgListOf(Basic(Bool), 0))

	add("Container1", ContainerOf(Basic(Uint8)))
	add("ContainerFixed", ContainerOf(Basic(Uint64), Bytes(32), Basic(Bool), Basic(Uint16)))
	add("ContainerMixed", ContainerOf(Basic(Uint8), ByteListOf(10), Basic(Uint32), ListOf(Basic(Uint64), 4), Bytes(4)))
	add("ContainerAllVar", ContainerOf(ByteListOf(10), BitListOf(20), ListOf(Basic(Uint16), 3)))
	add("ContainerVarFirst", ContainerOf(ByteListOf(5), Basic(Uint64)))
	add("ContainerVarLast", ContainerOf(Basic(Uint64), ByteListOf(5)))
	wide := make([]*Shape, 40)
	for i := range wide {
		wide[i] = Basic(Uint8)
	}
	add("ContainerWide40", ContainerOf(wide...))
	add("ContainerExtended", ContainerOf(Basic(Int16), Basic(Float32), &Shape{Kind: BigInt, N: 8, Limited: true}, Basic(Time)))

	add("ProgContainerDense", ProgContainerOf([]int{0, 1, 2}, Basic(Uint64), Bytes(32), Basic(Bool)))
	add("ProgContainerLeadingGap", ProgContainerOf([]int{2, 3}, Basic(Uint64), ByteListOf(8)))
	add("ProgContainerGaps", ProgContainerOf([]int{0, 3, 7, 8}, Basic(Uint8), Basic(Uint16), ListOf(Basic(Uint32), 4), Bytes(4)))
	add("ProgContainerOne", ProgContainerOf([]int{5}, Basic(Uint64)))
	add("ProgContainerWide", ProgContainerOf([]int{0, 17, 64, 70}, Basic(Uint8), Basic(Uint8), Basic(Uint8), Basic(Uint8)))
	add("ProgContainerFirstVar", ProgContainerOf([]int{0, 1}, ByteListOf(8), Basic(Uint64)))

	// EIP-8016: the options of a CompatibleUnion must merkleize alike.
	add("CompatUnionDense", CompatUnionOf([]int{1, 2, 3}, Basic(Uint64), Basic(Uint64), Basic(Uint64)))
	add("CompatUnionSparse", CompatUnionOf([]int{1, 2, 42, 127}, Bytes(32), Bytes(32), Bytes(32), Bytes(32)))
	add("CompatUnionOne", CompatUnionOf([]int{1}, Basic(Uint32)))
	add("CompatUnionContainers", CompatUnionOf([]int{1, 2}, ContainerOf(Basic(Uint8), ByteListOf(4)), ContainerOf(Basic(Uint8), ByteListOf(4))))
	add("CompatUnionProgContainers", CompatUnionOf([]int{1, 2},
		ProgContainerOf([]int{0, 1}, Basic(Uint8), ByteListOf(4)), ProgContainerOf([]int{0, 1, 2}, Basic(Uint8), ByteListOf(4), Basic(Uint16))))
	add("CompatUnionProgLists", CompatUnionOf([]int{1, 2}, ProgListOf(Basic(Uint64), 0), ProgListOf(Basic(Uint64), 100)))
	add("CompatUnionIncompatible", CompatUnionOf([]int{1, 2, 3}, Basic(Uint8), Bytes(4), ByteListOf(8)).Illegal())
	add("UnionNone", UnionOf(true, Basic(Uint16), Bytes(4), ContainerOf(Basic(Uint32), ByteListOf(6))))
	add("UnionNoNone", UnionOf(false, Basic(Uint64), ByteListOf(40)))

	add("OptionalUint64", OptionalOf(Basic(Uint64)))
	add("OptionalBytes4", OptionalOf(Bytes(4)))
	add("OptionalByteList8", OptionalOf(ByteListOf(8)))
	add("OptionalByteList100", OptionalOf(ByteListOf(100)))
	add("OptionalList4Uint64", OptionalOf(ListOf(Basic(Uint64), 4)))
	add("OptionalContainerFixed", OptionalOf(ContainerOf(Basic(Uint32), Bytes(4))))
	add("OptionalContainerVar", OptionalOf(ContainerOf(Basic(Uint32), ByteListOf(6))))
	add("OptionalListUint32", OptionalListOf(Basic(Uint32)))
	add("OptionalListBytes32", OptionalListOf(Bytes(32)))
	add("OptionalListContainerVar", OptionalListOf(ContainerOf(Basic(Uint32), ByteListOf(6))))
	add("WrapperByteList64", WrapperOf(ByteListOf(64)))
	add("WrapperVector3Uint16", WrapperOf(VectorOf(Basic(Uint16), 3)))
	add("WrapperUint64", WrapperOf(Basic(Uint64)))
	add("WrapperList4Uint32Spec", WrapperOf(ListOf(Basic(Uint32), 0).WithSpec(0)))
	return out
}

// ChildClasses are representatives of every class of child the engines tell
// apart when they encode a parent: one per leaf kind, fixed and variable,
// packed and composite, aligned and unaligned, static and spec-parametric,
// bounded and unbounded, extended and spec-defined.
func ChildClasses() []*Shape {
	var out []*Shape
	add := func(label string, s *Shape) { out = append(out, s.Labeled(label)) }
	add("Bool", Basic(Bool))
	add("Uint8", Basic(Uint8))
	add("Uint16", Basic(Uint16))
	add("Uint32", Basic(Uint32))
	add("Uint64", Basic(Uint64))
	add("Uint128", Basic(Uint128))
	add("Uint256Words", Basic(Uint256).WithRep(RepWords))
	add("Int32", Basic(Int32))
	add("Float64", Basic(Float64))
	add("BigInt33", &Shape{Kind: BigInt, N: 33, Limited: true})
	add("BigIntUnbounded", &Shape{Kind: BigInt})
	add("Time", Basic(Time))
	add("Bytes32", Bytes(32))
	add("Bytes20", Bytes(20))
	add("Bytes20String", Bytes(20).WithRep(RepString))
	add("BytesSpec0", Bytes(0).WithSpec(0))
	add("ByteList10", ByteListOf(10))
	add("ByteList1M", ByteListOf(1<<20))
	add("ByteListSpec1", ByteListOf(0).WithSpec(1))
	add("Bits8", Bits(8))
	add("Bits13", Bits(13))
	add("BitList9", BitListOf(9))
	add("ProgBitList", &Shape{Kind: ProgBitList})
	add("Vector3Uint16", VectorOf(Basic(Uint16), 3))
	add("Vector2ByteList6", VectorOf(ByteListOf(6), 2))
	add("Vector2Container", VectorOf(ContainerOf(Basic(Uint8), Bytes(4)), 2))
	add("Vector2ContainerPtr", VectorOf(ContainerOf(Basic(Uint8), Bytes(4)).WithRep(RepPtrElem), 2))
	add("List4Uint64", ListOf(Basic(Uint64), 4))
	add("List4ByteList6", ListOf(ByteListOf(6), 4))
	add("List3ContainerVar", ListOf(ContainerOf(Basic(Uint8), ByteListOf(4)), 3))
	add("List3ContainerVarPtr", ListOf(ContainerOf(Basic(Uint8), ByteListOf(4)).WithRep(RepPtrElem), 3))
	add("List1TUint64", ListOf(Basic(Uint64), 1<<40))
	add("ListSpec3Uint16", ListOf(Basic(Uint16), 0).WithSpec(3))
	add("ProgListUint64", ProgListOf(Basic(Uint64), 0))
	add("ProgListContainerVar", ProgListOf(ContainerOf(Basic(Uint8), ByteListOf(4)), 0))
	add("ContainerFixed", ContainerOf(Basic(Uint64), Bytes(4)))
	add("ContainerVar", ContainerOf(Basic(Uint8), ByteListOf(4)))
	add("ContainerSpec", ContainerOf(Basic(Uint8), Bytes(0).WithSpec(0)))
	add("ProgContainer", ProgContainerOf([]int{0, 2}, Basic(Uint8), ByteListOf(4)))
	add("CompatUnion", CompatUnionOf([]int{1, 2}, ContainerOf(Basic(Uint8), ByteListOf(4)), ContainerOf(Basic(Uint8), ByteListOf(4))))
	add("UnionNone", UnionOf(true, Basic(Uint16), ByteListOf(4)))
	add("UnionNoNone", UnionOf(false, Basic(Uint16), Bytes(4)))
	add("OptionalUint32", OptionalOf(Basic(Uint32)))
	add("OptionalContainerVar", OptionalOf(ContainerOf(Basic(Uint8), ByteListOf(4))))
	add("OptionalListUint32", OptionalListOf(Basic(Uint32)))
	add("OptionalListProgList", OptionalListOf(ProgListOf(Basic(Uint64), 0)))
	add("OptionalBitList9", OptionalOf(BitListOf(9)))
	add("WrapperByteList8", WrapperOf(ByteListOf(8)))
	return out
}

// Parent is a constructor placing a child in a parent shape.
type Parent struct {
	Label string
	Make  func(child *Shape) *Shape
	// Fits reports whether the child can sit in this parent.
	Fits func(child *Shape) bool
}

func always(*Shape) bool { return true }

// notPtrElem: a pointer-element representation only means something under a
// collection; elsewhere the plain container is used.
func plain(s *Shape) *Shape {
	if s.Rep == RepPtrElem {
		return s.WithRep(RepDefault)
	}
	return s
}

// singlePointer: an optional of a pointer-represented shape would be a
// multi-level pointer, which Go SSZ types do not have.
func singlePointer(s *Shape) bool {
	switch s.Kind {
	case Optional, OptionalList, BigInt:
		return false
	default:
		return s.Rep != RepDims
	}
}

// elementOK: collections take any element but an unbounded big.Int (no
// element width) and dimension-tagged shapes (their tags describe one level).
func elementOK(s *Shape) bool {
	if s.Rep == RepDims {
		return false
	}
	if s.Kind == BigInt && !s.Limited {
		return false
	}
	if s.Kind == Time {
		return false // time.Time is read only as a container field (ISSUES L1)
	}
	return true
}

// Parents are the contexts a child is nested in at stage 2.
func Parents() []Parent {
	return []Parent{
		{"InContainerAlone", func(c *Shape) *Shape { return ContainerOf(plain(c)) }, always},
		{"InContainerBetween", func(c *Shape) *Shape { return ContainerOf(Basic(Uint8), plain(c), Basic(Uint16)) }, always},
		{"InContainerAfterVar", func(c *Shape) *Shape { return ContainerOf(ByteListOf(3), plain(c)) }, always},
		{"InProgContainer", func(c *Shape) *Shape { return ProgContainerOf([]int{1, 3}, Basic(Uint8), plain(c)) }, always},
		{"InVector1", func(c *Shape) *Shape { return VectorOf(c, 1) }, elementOK},
		{"InVector3", func(c *Shape) *Shape { return VectorOf(c, 3) }, elementOK},
		{"InVector4Slice", func(c *Shape) *Shape { return VectorOf(c, 4).WithRep(RepSlice) }, elementOK},
		{"InList4", func(c *Shape) *Shape { return ListOf(c, 4) }, elementOK},
		{"InList1T", func(c *Shape) *Shape { return ListOf(c, 1<<40) }, elementOK},
		{"InListSpec0", func(c *Shape) *Shape { return ListOf(c, 0).WithSpec(0) }, elementOK},
		{"InProgList", func(c *Shape) *Shape { return ProgListOf(c, 0) }, elementOK},
		{"InCompatUnion", func(c *Shape) *Shape { return CompatUnionOf([]int{1, 2}, plain(c), plain(c)) },
			func(c *Shape) bool { return c.Rep != RepDims }},
		{"InUnion", func(c *Shape) *Shape { return UnionOf(true, plain(c)) }, func(c *Shape) bool { return c.Rep != RepDims }},
		{"InOptional", func(c *Shape) *Shape { return OptionalOf(plain(c)) }, singlePointer},
		{"InOptionalList", func(c *Shape) *Shape { return OptionalListOf(plain(c)) },
			func(c *Shape) bool { return singlePointer(c) && c.Kind != BitVector }}, // ISSUES L5
		{"InWrapper", func(c *Shape) *Shape { return WrapperOf(plain(c)) },
			func(c *Shape) bool {
				return c.Rep != RepDims && c.Kind != Wrapper && c.Kind != Union && c.Kind != CompatUnion && c.Kind != Optional && c.Kind != OptionalList
			}},
	}
}

var ident = regexp.MustCompile(`[^A-Za-z0-9_]`)

func name(parts ...string) string {
	return ident.ReplaceAllString(strings.Join(parts, "_"), "")
}

// Stage1 is every leaf as a root type.
func Stage1() []*Root {
	var out []*Root
	for _, l := range Leaves() {
		out = append(out, NewRoot(name("S1", l.Label), l, "1"))
	}
	return out
}

// Stage2 is every child class under every parent.
func Stage2() []*Root {
	var out []*Root
	for _, p := range Parents() {
		for _, c := range ChildClasses() {
			if !p.Fits(c) {
				continue
			}
			out = append(out, NewRoot(name("S2", p.Label, c.Label), p.Make(c), "2"))
		}
	}
	return out
}

// Stage3 exercises the properties a type inherits from below (variable
// size, spec-parametric size, recursion) through a middle type: every pair of
// parents around each property, plus recursive shapes under each parent.
func Stage3() []*Root {
	var out []*Root
	props := []*Shape{
		ByteListOf(6).Labeled("Var"),
		Bytes(0).WithSpec(0).Labeled("Spec"),
		ListOf(Basic(Uint16), 0).WithSpec(3).Labeled("SpecList"),
		ContainerOf(Basic(Uint8), ByteListOf(2)).Labeled("VarContainer"),
	}
	mid := Parents()
	for _, outer := range mid {
		for _, inner := range mid {
			for _, p := range props {
				if !inner.Fits(p) {
					continue
				}
				m := inner.Make(p)
				if !outer.Fits(m) {
					continue
				}
				out = append(out, NewRoot(name("S3", outer.Label, inner.Label, p.Label), outer.Make(m), "3"))
			}
		}
	}
	return out
}

// Delegating returns, for every stage-1 root, a method-less container
// holding it. Both engines walk such a root by reflection; the codegen
// engine then delegates the held value to its generated methods, which is
// the reflection engine's delegation path (tryMarshalCompat and friends).
// The declaration is the container of the root's declaration.
func Delegating(roots []*Root) []*Root {
	var out []*Root
	for _, r := range roots {
		if r.NoMethods {
			continue
		}
		inner := r.Shape
		w := &Root{Name: "W_" + r.Name, Shape: ContainerOf(inner), Desc: "Deleg(" + r.Desc + ")", Stage: "deleg", IllFormed: r.IllFormed, NoMethods: true, Inner: r.Name}
		w.Shape.Fields[0].Name = "V"
		out = append(out, w)
	}
	return out
}

// Recursive returns the recursive roots: a container whose field refers
// back to the root through every kind of variable-size position dynamic-ssz
// admits a cycle in — lists, progressive lists, optionals, optional-lists,
// classic unions — plus a cycle through a second container.
func Recursive() []*Root {
	var out []*Root
	add := func(label string, s *Shape) {
		out = append(out, NewRoot(name("R", label), s, "rec"))
	}
	self := SelfReference()
	add("List3", ContainerOf(Basic(Uint8), ListOf(self, 3)))
	add("List1T", ContainerOf(Basic(Uint8), ListOf(self, 1<<40)))
	add("ProgList", ContainerOf(Basic(Uint8), ProgListOf(self, 0)))
	add("Optional", ContainerOf(Basic(Uint8), OptionalOf(self)))
	add("OptionalList", ContainerOf(Basic(Uint8), OptionalListOf(self)))
	add("UnionNone", ContainerOf(Basic(Uint8), UnionOf(true, self)))
	add("ViaContainer", ContainerOf(Basic(Uint8), ListOf(ContainerOf(Basic(Uint16), self), 2)))
	add("TwoWays", ContainerOf(Basic(Uint8), ListOf(self, 2), OptionalListOf(self), ByteListOf(4)))
	add("ProgContainer", ProgContainerOf([]int{0, 2}, Basic(Uint8), ListOf(self, 2)))
	return out
}

// Random draws n shapes from the grammar with nesting up to depth, for the
// combinations the stages do not enumerate.
func Random(seed int64, n, depth int) []*Root {
	rng := rand.New(rand.NewSource(seed))
	var out []*Root
	for i := 0; i < n; i++ {
		s := randomShape(rng, depth)
		out = append(out, NewRoot(fmt.Sprintf("R%d_%d", seed, i), s, "random"))
	}
	return out
}

func randomShape(rng *rand.Rand, depth int) *Shape {
	leaves := ChildClasses()
	if depth <= 0 {
		return leaves[rng.Intn(len(leaves))]
	}
	parents := Parents()
	for {
		p := parents[rng.Intn(len(parents))]
		child := randomShape(rng, depth-1)
		if rng.Intn(3) == 0 {
			child = leaves[rng.Intn(len(leaves))]
		}
		if p.Fits(child) {
			return p.Make(child)
		}
	}
}
