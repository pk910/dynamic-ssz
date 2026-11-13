// Package corpus holds the generated type corpus of the generic link: every
// SSZ kind as a root type, every child class under every parent, the
// inherited properties through a middle type, and random shapes
// (formal/harness/typegen). The types and their dynssz-gen methods are
// generated (gen-corpus, formal/corpus.sh) and not committed; this file is.
package corpus

// Entry is one corpus type.
type Entry struct {
	Name  string
	Stage string
	Shape string // the shape, described
	Decl  string // the declaration the shape is meant to have (spec oracle JSON), "" if extended
	// DeclStatic is Decl with spec-parametric sizes at their static defaults
	// (the code generator's view before spec values are resolved).
	DeclStatic string
	// CertDecl is the declaration for the in-house oracle (spec spelling plus
	// the extended kinds); every shape has one.
	CertDecl string
	// CertDeclStatic is CertDecl with spec-parametric sizes at their static defaults.
	CertDeclStatic string
	Extended       bool
	// IllFormed: the SSZ specification declares the shape illegal (the spec
	// oracle must refuse its declaration) although dynamic-ssz accepts it.
	IllFormed bool
	// Recursive: the shape refers to itself; Decl and CertDecl are empty and
	// Unrolled/CertUnrolled hold the declarations of its unrollings by depth.
	Recursive    bool
	Unrolled     []string
	CertUnrolled []string
	// Opaque: the shape holds a custom type, whose declaration dynamic-ssz
	// cannot derive; the link relies on the intended one.
	Opaque bool
	// Views are the schema views of a container root (stage views).
	Views []ViewEntry
	New   func() any
}

// ViewEntry is one schema view of a root: a struct with a subset or a
// reordering of the root's fields, passed as WithViewDescriptor.
type ViewEntry struct {
	Name     string
	Shape    string
	Decl     string // "" if extended
	CertDecl string
	Extended bool
	New      func() any // a nil pointer of the view type
}

// Registry lists the generated types; empty when the corpus is not generated.
var Registry []Entry
