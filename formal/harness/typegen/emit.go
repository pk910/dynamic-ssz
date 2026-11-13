package typegen

import (
	"fmt"
	"go/format"
	"sort"
	"strings"
)

// Root is one corpus type: a shape at the top level, under a name.
type Root struct {
	Name  string
	Shape *Shape // the shape as emitted: an embed-only shape is wrapped in a one-field container
	Desc  string // the shape as requested, described
	Stage string
	// IllFormed: the requested shape is one the specification forbids.
	IllFormed bool
}

// NewRoot returns a root for the shape, wrapping shapes that cannot stand
// alone as a Go type with methods in a one-field container.
func NewRoot(name string, s *Shape, stage string) *Root {
	r := &Root{Name: name, Shape: s, Desc: s.Describe(), Stage: stage, IllFormed: s.IllFormed}
	if embedOnly(s) {
		r.Shape = ContainerOf(s)
	}
	return r
}

// tagSet accumulates struct tags for one field or Annotate call.
type tagSet struct{ pairs []string }

func (t *tagSet) add(key, value string) { t.pairs = append(t.pairs, key+":"+fmt.Sprintf("%q", value)) }
func (t *tagSet) String() string        { return strings.Join(t.pairs, " ") }

// Emitter writes shapes as Go declarations.
type Emitter struct {
	decls   []string          // named type declarations, in dependency order
	names   map[*Shape]string // shapes that got a named type
	Names   []string          // every named type declared, roots and helpers
	count   int
	imports map[string]bool
}

// NewEmitter returns an empty emitter.
func NewEmitter() *Emitter {
	return &Emitter{names: map[*Shape]string{}, imports: map[string]bool{}}
}

func (e *Emitter) fresh(prefix string) string {
	e.count++
	return fmt.Sprintf("%s%d", prefix, e.count)
}

// embedOnly reports whether a shape cannot stand alone as a named Go type
// with generated methods (generic aliases, pointers): it is wrapped in a
// one-field container at the top level.
func embedOnly(s *Shape) bool {
	switch s.Kind {
	case CompatUnion, Union, Wrapper, Optional, OptionalList, BigInt, Time:
		return true
	default:
		return false
	}
}

// Root emits a root type under name and returns the Go expression that
// constructs a fresh pointer to it.
func (e *Emitter) Root(r *Root) {
	e.Names = append(e.Names, r.Name)
	s := r.Shape
	switch s.Kind {
	case Container, ProgContainer:
		e.decls = append(e.decls, fmt.Sprintf("// %s: %s\ntype %s %s\n", r.Name, r.Desc, r.Name, e.structBody(s)))
	default:
		typ, tags := e.typeExpr(s)
		d := fmt.Sprintf("// %s: %s\ntype %s %s\n", r.Name, r.Desc, r.Name, typ)
		if tags != "" {
			e.imports["github.com/pk910/dynamic-ssz/sszutils"] = true
			d += fmt.Sprintf("\nvar _ = sszutils.Annotate[%s](`%s`)\n", r.Name, tags)
		}
		e.decls = append(e.decls, d)
	}
}

// named returns a named type for s (declaring it once), carrying s's tags
// through sszutils.Annotate when it has any.
func (e *Emitter) named(s *Shape, prefix string) string {
	if n, ok := e.names[s]; ok {
		return n
	}
	name := e.fresh(prefix)
	e.names[s] = name
	switch s.Kind {
	case CompatUnion, Union, Wrapper:
		// generic aliases cannot carry methods
	default:
		e.Names = append(e.Names, name)
	}
	switch s.Kind {
	case Container, ProgContainer:
		e.decls = append(e.decls, fmt.Sprintf("// %s\ntype %s %s\n", s.Describe(), name, e.structBody(s)))
	case CompatUnion, Union, Wrapper:
		typ, _ := e.typeExpr(s)
		e.decls = append(e.decls, fmt.Sprintf("// %s\ntype %s = %s\n", s.Describe(), name, typ))
	default:
		typ, tags := e.typeExpr(s)
		d := fmt.Sprintf("// %s\ntype %s %s\n", s.Describe(), name, typ)
		if tags != "" {
			e.imports["github.com/pk910/dynamic-ssz/sszutils"] = true
			d += fmt.Sprintf("\nvar _ = sszutils.Annotate[%s](`%s`)\n", name, tags)
		}
		e.decls = append(e.decls, d)
	}
	return name
}

// elemExpr returns the Go type of a collection element or union option: a
// basic type inline, anything that needs tags or a struct as a named type.
func (e *Emitter) elemExpr(s *Shape) string {
	switch s.Kind {
	case Bool, Uint8, Uint16, Uint32, Uint64, Int8, Int16, Int32, Int64, Float32, Float64:
		typ, _ := e.typeExpr(s)
		return typ
	case ByteVector:
		if s.Rep == RepDefault && s.Spec == "" {
			return fmt.Sprintf("[%d]byte", s.N)
		}
	default:
	}
	name := e.named(s, "E")
	if s.Rep == RepPtrElem {
		return "*" + name
	}
	return name
}

func (e *Emitter) structBody(s *Shape) string {
	var b strings.Builder
	b.WriteString("struct {\n")
	for _, f := range s.Fields {
		typ, tags := e.fieldExpr(f.Shape)
		ts := tags
		if f.Index >= 0 {
			if ts != "" {
				ts += " "
			}
			ts += fmt.Sprintf("ssz-index:%q", fmt.Sprint(f.Index))
		}
		fmt.Fprintf(&b, "\t%s %s", f.Name, typ)
		if ts != "" {
			fmt.Fprintf(&b, " `%s`", ts)
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String()
}

// fieldExpr returns the Go type and tags of a container field.
func (e *Emitter) fieldExpr(s *Shape) (string, string) {
	switch s.Kind {
	case Container, ProgContainer:
		return e.named(s, "C"), ""
	case CompatUnion, Union, Wrapper:
		typ, tags := e.typeExpr(s)
		return typ, tags
	default:
		return e.typeExpr(s)
	}
}

// typeExpr returns the Go type expression and the tags that give it s's
// SSZ meaning (as struct tags or an Annotate string).
func (e *Emitter) typeExpr(s *Shape) (string, string) {
	if (s.Kind == Vector || s.Kind == List || s.Kind == ProgList) && dimmed(s.Elem) {
		return e.dimmedCollection(s)
	}
	var t tagSet
	spec := func(key string) {
		if s.Spec != "" {
			t.add("dynssz-"+key, s.Spec)
		}
	}
	switch s.Kind {
	case Bool:
		return "bool", ""
	case Uint8:
		return "uint8", ""
	case Uint16:
		return "uint16", ""
	case Uint32:
		return "uint32", ""
	case Uint64:
		return "uint64", ""
	case Int8:
		return "int8", ""
	case Int16:
		return "int16", ""
	case Int32:
		return "int32", ""
	case Int64:
		return "int64", ""
	case Float32:
		return "float32", ""
	case Float64:
		return "float64", ""
	case Time:
		e.imports["time"] = true
		return "time.Time", ""
	case Uint128:
		t.add("ssz-type", "uint128")
		if s.Rep == RepWords {
			return "[2]uint64", t.String()
		}
		return "[16]byte", t.String()
	case Uint256:
		t.add("ssz-type", "uint256")
		if s.Rep == RepWords {
			return "[4]uint64", t.String()
		}
		return "[32]byte", t.String()
	case BigInt:
		e.imports["math/big"] = true
		if s.Limited {
			t.add("ssz-max", fmt.Sprint(s.N))
		}
		return "*big.Int", t.String()
	case ByteVector:
		switch s.Rep {
		case RepString:
			t.add("ssz-size", fmt.Sprint(s.N))
			spec("size")
			return "string", t.String()
		case RepSlice:
			t.add("ssz-size", fmt.Sprint(s.N))
			spec("size")
			return "[]byte", t.String()
		default:
			if s.Spec != "" {
				t.add("ssz-size", fmt.Sprint(s.N))
				spec("size")
				return "[]byte", t.String()
			}
			return fmt.Sprintf("[%d]byte", s.N), ""
		}
	case ByteList:
		t.add("ssz-max", fmt.Sprint(s.N))
		spec("max")
		switch s.Rep {
		case RepString:
			return "string", t.String()
		default:
			return "[]byte", t.String()
		}
	case BitVector:
		t.add("ssz-type", "bitvector")
		t.add("ssz-bitsize", fmt.Sprint(s.N))
		spec("bitsize")
		if s.Rep == RepSlice || s.Spec != "" {
			t.add("ssz-size", fmt.Sprint((s.N+7)/8))
			if s.Spec != "" {
				t.add("dynssz-size", "("+s.Spec+"+7)/8")
			}
			return "[]byte", t.String()
		}
		return fmt.Sprintf("[%d]byte", (s.N+7)/8), t.String()
	case BitList:
		t.add("ssz-type", "bitlist")
		t.add("ssz-max", fmt.Sprint(s.N))
		spec("max")
		return "[]byte", t.String()
	case ProgBitList:
		t.add("ssz-type", "progressive-bitlist")
		if s.Limited {
			t.add("ssz-max", fmt.Sprint(s.N))
			spec("max")
		}
		return "[]byte", t.String()
	case Vector:
		if s.Rep == RepDims {
			// A two-dimensional byte collection declared with dimension tags.
			switch s.Elem.Kind {
			case ByteVector:
				t.add("ssz-size", fmt.Sprintf("%d,%d", s.N, s.Elem.N))
			case ByteList:
				t.add("ssz-size", fmt.Sprintf("%d,?", s.N))
				t.add("ssz-max", fmt.Sprintf("?,%d", s.Elem.N))
			default:
				panic("RepDims needs byte elements")
			}
			return "[][]byte", t.String()
		}
		elem := e.elemExpr(s.Elem)
		if s.Rep == RepSlice || s.Spec != "" {
			t.add("ssz-size", fmt.Sprint(s.N))
			spec("size")
			return "[]" + elem, t.String()
		}
		return fmt.Sprintf("[%d]%s", s.N, elem), ""
	case List:
		elem := e.elemExpr(s.Elem)
		t.add("ssz-max", fmt.Sprint(s.N))
		spec("max")
		return "[]" + elem, t.String()
	case ProgList:
		elem := e.elemExpr(s.Elem)
		t.add("ssz-type", "progressive-list")
		if s.Limited {
			t.add("ssz-max", fmt.Sprint(s.N))
			spec("max")
		}
		return "[]" + elem, t.String()
	case Optional:
		// The optional is a dimension of its own: the payload's size tags
		// move one dimension right and its kind joins ssz-type.
		typ, tags := e.optionalElem(s.Elem)
		return "*" + typ, shiftTags(tags, "optional", true)
	case OptionalList:
		// optional-list is transparent to dimensions: the payload's size tags
		// stay in place, only its kind joins ssz-type.
		typ, tags := e.optionalElem(s.Elem)
		return "*" + typ, shiftTags(tags, "optional-list", false)
	case Wrapper:
		e.imports["github.com/pk910/dynamic-ssz"] = true
		typ, tags := e.fieldExpr(s.Elem)
		inner := typ
		if tags != "" {
			inner += " `" + tags + "`"
		}
		t.add("ssz-type", "wrapper")
		return fmt.Sprintf("dynssz.TypeWrapper[struct {\n\tData %s\n}, %s]", inner, typ), t.String()
	case CompatUnion:
		e.imports["github.com/pk910/dynamic-ssz"] = true
		var b strings.Builder
		b.WriteString("dynssz.CompatibleUnion[struct {\n")
		for i, o := range s.Options {
			typ, tags := e.fieldExpr(o.Shape)
			fmt.Fprintf(&b, "\tV%d %s `%s`\n", i, typ, join(tags, fmt.Sprintf("ssz-index:%q", fmt.Sprint(o.Selector))))
		}
		b.WriteString("}]")
		return b.String(), ""
	case Union:
		e.imports["github.com/pk910/dynamic-ssz"] = true
		var b strings.Builder
		b.WriteString("dynssz.Union[struct {\n")
		for i, o := range s.Options {
			if o.Shape == nil {
				b.WriteString("\tNone dynssz.None\n")
				continue
			}
			typ, tags := e.fieldExpr(o.Shape)
			fmt.Fprintf(&b, "\tV%d %s", i, typ)
			if tags != "" {
				fmt.Fprintf(&b, " `%s`", tags)
			}
			b.WriteString("\n")
		}
		b.WriteString("}]")
		return b.String(), ""
	case Container, ProgContainer:
		return e.named(s, "C"), ""
	}
	panic("unreachable")
}

// optionalElem returns the pointee type of an optional and the tags that
// describe the pointee (they stay on the field, as dynamic-ssz reads them).
func (e *Emitter) optionalElem(s *Shape) (string, string) {
	switch s.Kind {
	case Container, ProgContainer:
		return e.named(s, "C"), ""
	default:
		return e.typeExpr(s)
	}
}

// dimmed reports whether a collection element must be declared through the
// collection's own dimension tags (it has no Go type of its own to annotate).
func dimmed(elem *Shape) bool {
	switch elem.Kind {
	case Optional, OptionalList, Wrapper, BigInt:
		return true
	default:
		return false
	}
}

// dimTags accumulates per-dimension tag values.
type dimTags struct {
	keys  []string
	vals  map[string][]string
	depth int
}

func (d *dimTags) set(dim int, key, value string) {
	if d.vals == nil {
		d.vals = map[string][]string{}
	}
	if _, ok := d.vals[key]; !ok {
		d.keys = append(d.keys, key)
	}
	for len(d.vals[key]) <= dim {
		d.vals[key] = append(d.vals[key], "?")
	}
	d.vals[key][dim] = value
	if dim+1 > d.depth {
		d.depth = dim + 1
	}
}

func (d *dimTags) String() string {
	var out []string
	for _, k := range d.keys {
		v := append([]string{}, d.vals[k]...)
		for len(v) < d.depth {
			v = append(v, "?")
		}
		// A dimension is either sized or dynamic: where a dynssz-* tag says
		// nothing about a dimension the static tag sizes, it repeats the size.
		if static, ok := strings.CutPrefix(k, "dyn"); ok {
			for i, sv := range d.vals[static] {
				if i < len(v) && v[i] == "?" && sv != "?" {
					v[i] = sv
				}
			}
		}
		// trailing placeholders carry no information
		for len(v) > 1 && v[len(v)-1] == "?" {
			v = v[:len(v)-1]
		}
		out = append(out, fmt.Sprintf("%s:%q", k, strings.Join(v, ",")))
	}
	return strings.Join(out, " ")
}

// dimmedCollection declares a collection of optional (or wrapped) elements:
// the collection is dimension 0, the optional dimension 1, its payload
// dimension 2, all in the field's tags.
func (e *Emitter) dimmedCollection(s *Shape) (string, string) {
	var d dimTags
	switch s.Kind {
	case Vector:
		d.set(0, "ssz-type", "vector")
		d.set(0, "ssz-size", fmt.Sprint(s.N))
		if s.Spec != "" {
			d.set(0, "dynssz-size", s.Spec)
		}
	case List:
		d.set(0, "ssz-type", "list")
		d.set(0, "ssz-max", fmt.Sprint(s.N))
		if s.Spec != "" {
			d.set(0, "dynssz-max", s.Spec)
		}
	case ProgList:
		d.set(0, "ssz-type", "progressive-list")
		if s.Limited {
			d.set(0, "ssz-max", fmt.Sprint(s.N))
			if s.Spec != "" {
				d.set(0, "dynssz-max", s.Spec)
			}
		}
	default:
		panic("not a collection")
	}
	elem := s.Elem
	var payloadType, payloadTags string
	payloadDim := 2
	switch elem.Kind {
	case Optional:
		d.set(1, "ssz-type", "optional")
		payloadType, payloadTags = e.optionalElem(elem.Elem)
		payloadType = "*" + payloadType
	case OptionalList:
		// transparent to dimensions: the payload's tags sit at the optional's dimension
		d.set(1, "ssz-type", "optional-list")
		payloadType, payloadTags = e.optionalElem(elem.Elem)
		payloadType = "*" + payloadType
		payloadDim = 1
	case Wrapper:
		d.set(1, "ssz-type", "wrapper")
		payloadType = e.named(elem, "E")
	case BigInt:
		e.imports["math/big"] = true
		payloadType = "*big.Int"
		if elem.Limited {
			d.set(1, "ssz-max", fmt.Sprint(elem.N))
		}
	default:
		panic("not a dimmed element")
	}
	for _, pair := range strings.Fields(payloadTags) {
		key, value, _ := strings.Cut(pair, ":")
		d.set(payloadDim, key, strings.Trim(value, `"`))
	}
	prefix := "[]"
	if s.Kind == Vector && s.Rep != RepSlice && s.Spec == "" {
		prefix = fmt.Sprintf("[%d]", s.N)
	}
	return prefix + payloadType, d.String()
}

// shiftTags rewrites a payload's tags for a field whose outer dimension is
// the given kind (optional, optional-list): size, limit and bit-size tags
// get a leading `?,`, and the payload's ssz-type follows the outer kind.
func shiftTags(tags, outer string, shift bool) string {
	var out []string
	kind := ""
	for _, pair := range strings.Fields(tags) {
		key, value, _ := strings.Cut(pair, ":")
		value = strings.Trim(value, `"`)
		switch key {
		case "ssz-type":
			kind = value
		case "ssz-size", "ssz-max", "ssz-bitsize", "dynssz-size", "dynssz-max", "dynssz-bitsize":
			if shift {
				value = "?," + value
			}
			out = append(out, fmt.Sprintf("%s:%q", key, value))
		default:
			out = append(out, pair)
		}
	}
	if kind != "" {
		outer += "," + kind
	}
	return strings.Join(append([]string{fmt.Sprintf("ssz-type:%q", outer)}, out...), " ")
}

func join(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + " " + b
	}
}

// Source renders the declarations as a Go file of package pkg.
func (e *Emitter) Source(pkg string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gen-corpus; DO NOT EDIT.\n\npackage %s\n\n", pkg)
	if len(e.imports) > 0 {
		imps := make([]string, 0, len(e.imports))
		for i := range e.imports {
			imps = append(imps, i)
		}
		sort.Strings(imps)
		b.WriteString("import (\n")
		for _, i := range imps {
			if i == "github.com/pk910/dynamic-ssz" {
				b.WriteString("\tdynssz \"github.com/pk910/dynamic-ssz\"\n")
			} else {
				fmt.Fprintf(&b, "\t%q\n", i)
			}
		}
		b.WriteString(")\n\n")
	}
	for _, d := range e.decls {
		b.WriteString(d)
		b.WriteString("\n")
	}
	out, err := format.Source([]byte(b.String()))
	if err != nil {
		return b.String(), fmt.Errorf("gofmt: %w", err)
	}
	return string(out), nil
}

// Registry renders the corpus registry file: one entry per root with its
// constructor and intended declaration.
func Registry(pkg string, roots []*Root) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by gen-corpus; DO NOT EDIT.\n\npackage %s\n\nfunc init() {\n\tRegistry = append(Registry,\n", pkg)
	for _, r := range roots {
		fmt.Fprintf(&b, "\t\tEntry{Name: %q, Stage: %q, Shape: %q, Decl: %q, DeclStatic: %q, Extended: %v, IllFormed: %v, New: func() any { return new(%s) }},\n",
			r.Name, r.Stage, r.Desc, r.Shape.Decl(), r.Shape.DeclStatic(), r.Shape.Extended(), r.IllFormed, r.Name)
	}
	b.WriteString("\t)\n}\n")
	out, err := format.Source([]byte(b.String()))
	if err != nil {
		return b.String(), fmt.Errorf("gofmt: %w", err)
	}
	return string(out), nil
}
