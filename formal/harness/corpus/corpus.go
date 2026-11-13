// Package corpus holds the generated type corpus of the generic link: every
// SSZ kind as a root type, every child class under every parent, the
// inherited properties through a middle type, and random shapes
// (formal/harness/typegen). The types and their dynssz-gen methods are
// generated (gen-corpus, formal/corpus.sh) and not committed; this file is.
package corpus

// Entry is one corpus type.
type Entry struct {
	Name     string
	Stage    string
	Shape    string // the shape, described
	Decl     string // the declaration the shape is meant to have (spec oracle JSON), "" if extended
	// DeclStatic is Decl with spec-parametric sizes at their static defaults
	// (the code generator's view before spec values are resolved).
	DeclStatic string
	Extended bool
	// IllFormed: the SSZ specification declares the shape illegal (the spec
	// oracle must refuse its declaration) although dynamic-ssz accepts it.
	IllFormed bool
	New       func() any
}

// Registry lists the generated types; empty when the corpus is not generated.
var Registry []Entry
