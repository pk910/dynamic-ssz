// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

// Package annotations resolves sszutils.Annotate registrations from Go source
// the way the runtime registry sees them. It is shared by dynssz-gen and the
// formal certifier (formal/certify), so both read a type's annotations with
// exactly the same semantics.
package annotations

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const SszutilsPkgPath = "github.com/pk910/dynamic-ssz/sszutils"

// Index resolves the sszutils.Annotate registrations of a named type
// the way the runtime registry sees them: the calls of every loaded package,
// in the order their init functions run, merged newest first. Every package
// in the generated package's import graph is linked into any binary running
// its code, and a view package loaded separately is linked wherever its views
// are served, so a registration in any of them applies at run time, wherever
// the type is declared.
type Index struct {
	order  []*packages.Package
	owners map[*types.Package]*packages.Package
	cache  map[*types.Named]string
	err    error
}

// NewIndex indexes root and extra (separately loaded view packages)
// with all their dependencies, ordered as Go initializes them: repeatedly the
// package first by import path among those whose imports are initialized.
// The generated code serving the views imports the extra packages, so they
// count as imports of root. A package both loads hold a copy of is read once,
// as it initializes once. A package that imports sszutils but was loaded
// without syntax could hold registrations the generator cannot read, so it is
// refused rather than treated as unannotated.
func NewIndex(root *packages.Package, extra ...*packages.Package) (*Index, error) {
	idx := &Index{
		owners: make(map[*types.Package]*packages.Package),
		cache:  make(map[*types.Named]string),
	}
	byPath := make(map[string]*packages.Package)
	imports := make(map[string][]string)
	var err error
	packages.Visit(append(append([]*packages.Package{}, extra...), root), nil, func(p *packages.Package) {
		if p.Types == nil {
			return
		}
		idx.owners[p.Types] = p
		if err == nil && p.Imports[SszutilsPkgPath] != nil && (len(p.Syntax) == 0 || p.TypesInfo == nil) {
			err = fmt.Errorf("package %s imports sszutils but was loaded without syntax: its Annotate registrations cannot be read", p.PkgPath)
		}
		if byPath[p.PkgPath] != nil {
			return
		}
		byPath[p.PkgPath] = p
		for path := range p.Imports {
			imports[p.PkgPath] = append(imports[p.PkgPath], path)
		}
	})
	if err != nil {
		return nil, err
	}
	for _, p := range extra {
		imports[root.PkgPath] = append(imports[root.PkgPath], p.PkgPath)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	initialized := make(map[string]bool, len(paths))
	for len(idx.order) < len(paths) {
		next := ""
		for _, path := range paths {
			if initialized[path] {
				continue
			}
			ready := true
			for _, dep := range imports[path] {
				if byPath[dep] != nil && !initialized[dep] {
					ready = false
					break
				}
			}
			if ready {
				next = path
				break
			}
		}
		if next == "" {
			// Import cycles do not compile; nothing is left to order.
			break
		}
		initialized[next] = true
		idx.order = append(idx.order, byPath[next])
	}
	return idx, nil
}

// resolve returns the merged annotation tag for t, or "". An alias is
// transparent and one pointer level is stripped, as the runtime registration
// does.
func (idx *Index) Resolve(t types.Type) string {
	named := NamedType(t)
	if named == nil || named.Obj().Pkg() == nil {
		return ""
	}
	if tag, ok := idx.cache[named]; ok {
		return tag
	}
	if idx.owners[named.Obj().Pkg()] == nil {
		// Every type the parser reaches comes from an indexed package; one
		// that does not cannot be vouched for, so generation is failed.
		if idx.err == nil {
			idx.err = fmt.Errorf("type %v: declaring package %s was not loaded, its Annotate registrations cannot be read", named, named.Obj().Pkg().Path())
		}
		return ""
	}
	var tags []string
	for _, pkg := range idx.order {
		tags = append(tags, CallTags(pkg, named)...)
	}
	tag := MergeTags(tags)
	idx.cache[named] = tag
	return tag
}

// NamedType returns the named type t stands for, or nil. An alias is
// transparent and one pointer level is stripped, as the runtime registration
// does.
func NamedType(t types.Type) *types.Named {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, _ := t.(*types.Named)
	return named
}

// TypeArgMatches reports whether the type argument of an Annotate call
// is target. With type information the argument is resolved the way the
// runtime registration resolves it (an alias is transparent, one pointer level
// is stripped) and compared by its qualified name, so instantiations of one
// generic type and same-named types of other packages stay apart while a
// separately loaded package's copy of the same type matches; without it the
// argument has to be spelled as the target's own name.
func TypeArgMatches(pkg *packages.Package, arg ast.Expr, target *types.Named) bool {
	if pkg != nil && pkg.TypesInfo != nil {
		if typ := pkg.TypesInfo.TypeOf(arg); typ != nil {
			typ = types.Unalias(typ)
			if ptr, ok := typ.(*types.Pointer); ok {
				typ = types.Unalias(ptr.Elem())
			}
			return types.Identical(typ, target) || types.TypeString(typ, nil) == types.TypeString(target, nil)
		}
	}
	ident, ok := arg.(*ast.Ident)
	return ok && ident.Name == target.Obj().Name()
}

// CallTags scans pkg's AST for sszutils.Annotate[target]("...")
// calls and returns their tags in the package's initialization order:
// package-level variables of every file in file order, then the init
// functions.
func CallTags(pkg *packages.Package, target *types.Named) []string {
	if pkg == nil || target == nil {
		return nil
	}
	var varTags, initTags []string

	for _, file := range pkg.Syntax {
		// Resolve which import alias (if any) maps to sszutils
		sszutilsAlias := ""
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "github.com/pk910/dynamic-ssz/sszutils" {
				if imp.Name != nil {
					sszutilsAlias = imp.Name.Name
				} else {
					sszutilsAlias = "sszutils"
				}

				break
			}
		}

		if sszutilsAlias == "" {
			continue
		}

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				varTags = append(varTags, VarDeclCalls(pkg, d, sszutilsAlias, target)...)
			case *ast.FuncDecl:
				initTags = append(initTags, InitCalls(pkg, d, sszutilsAlias, target)...)
			}
		}
	}

	ordered := make([]string, 0, len(varTags)+len(initTags))
	ordered = append(ordered, varTags...)
	return append(ordered, initTags...)
}

// MergeTags merges tags registered in the given order newest first,
// dropping repeats, as the runtime registry does.
func MergeTags(ordered []string) string {
	merged := make([]string, 0, len(ordered))
	seen := make(map[string]struct{}, len(ordered))
	for i := len(ordered) - 1; i >= 0; i-- {
		if _, ok := seen[ordered[i]]; ok {
			continue
		}
		seen[ordered[i]] = struct{}{}
		merged = append(merged, ordered[i])
	}

	return strings.Join(merged, " ")
}

// VarDeclCalls returns the tags of every Annotate call for
// target among the initializers of a package-level var declaration.
func VarDeclCalls(pkg *packages.Package, d *ast.GenDecl, alias string, target *types.Named) []string {
	if d.Tok != token.VAR {
		return nil
	}
	var tags []string
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, val := range vs.Values {
			if tag := MatchCall(pkg, val, alias, target); tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// InitCalls returns the tags of every Annotate call for target
// among the statements of an init function.
func InitCalls(pkg *packages.Package, d *ast.FuncDecl, alias string, target *types.Named) []string {
	if d.Name.Name != "init" || d.Body == nil {
		return nil
	}
	var tags []string
	for _, stmt := range d.Body.List {
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}

		if tag := MatchCall(pkg, exprStmt.X, alias, target); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// MatchCall checks if an expression is sszutils.Annotate[target]("...tag...")
// and returns the tag string, or "" if it doesn't match (see
// TypeArgMatches for how the type argument is compared).
func MatchCall(pkg *packages.Package, expr ast.Expr, alias string, target *types.Named) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return ""
	}

	// The function expression should be an IndexExpr: sszutils.Annotate[TypeName]
	indexExpr, ok := call.Fun.(*ast.IndexExpr)
	if !ok {
		return ""
	}

	// Check that the selector is <alias>.Annotate
	sel, ok := indexExpr.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Annotate" {
		return ""
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != alias {
		return ""
	}

	// Check that the type argument matches
	if !TypeArgMatches(pkg, indexExpr.Index, target) {
		return ""
	}

	// Extract the string literal argument
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}

	// Unquote the string literal
	if strings.HasPrefix(lit.Value, "`") {
		// Raw string literal: just strip the backticks
		return strings.TrimPrefix(strings.TrimSuffix(lit.Value, "`"), "`")
	}

	// Interpreted string literal: use strconv.Unquote
	tag, unquoteErr := strconv.Unquote(lit.Value)
	if unquoteErr != nil {
		return ""
	}

	return tag
}

// Err reports the first type whose annotations the index could not vouch for
// (its declaring package was not loaded), or nil. Generation must fail on it.
func (idx *Index) Err() error {
	return idx.err
}

// Order returns the indexed packages in Go initialization order.
func (idx *Index) Order() []*packages.Package {
	return idx.order
}
