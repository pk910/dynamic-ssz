package harness

import (
	"errors"
	"go/types"
	"testing"

	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/dynssz-gen/annotations"
	"github.com/pk910/dynamic-ssz/formal/certify/certlib"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"golang.org/x/tools/go/packages"
)

// TestCorpusBuildersAgree checks, for every corpus type, that the descriptor
// the code generator builds from the Go source (go/types, as dynssz-gen does)
// has the declaration the shape was generated to have — the second
// descriptor builder of dynamic-ssz, read generically. With
// TestCorpusDeclarations (the reflection builder) this is the generic form of
// TestRuntimeCertificatesAgree. Extended shapes are compared between the two
// builders through the in-house certificate term instead.
func TestCorpusBuildersAgree(t *testing.T) {
	entries := corpusEntries(t)
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax |
			packages.NeedName | packages.NeedImports | packages.NeedDeps,
	}
	pkgs, err := packages.Load(cfg, "github.com/pk910/dynamic-ssz/formal/harness/corpus")
	if err != nil {
		t.Fatalf("load corpus package: %v", err)
	}
	var pkg *packages.Package
	for _, p := range pkgs {
		if p.ID == p.PkgPath {
			pkg = p
		}
	}
	if pkg == nil || len(pkg.Errors) > 0 {
		t.Fatalf("corpus package did not load cleanly: %v", pkgs)
	}
	annIdx, err := annotations.NewIndex(pkg)
	if err != nil {
		t.Fatalf("annotations: %v", err)
	}
	parser := codegen.NewParser()
	parser.NoDelegation = true
	parser.ExtendedTypes = true
	parser.AnnotationResolver = annIdx.Resolve
	// The go/types builder knows no spec values: spec-parametric sizes stay at
	// their static defaults and are resolved when a generated method runs.
	refl := reflectionInstance(nil)
	scope := pkg.Types.Scope()

	for _, e := range entries {
		obj := scope.Lookup(e.Name)
		if obj == nil {
			t.Errorf("%s: not in the corpus package", e.Name)
			continue
		}
		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			t.Errorf("%s: not a type", e.Name)
			continue
		}
		var typeHints []ssztypes.SszTypeHint
		var sizeHints []ssztypes.SszSizeHint
		var maxSizeHints []ssztypes.SszMaxSizeHint
		if tag := annIdx.Resolve(typeObj.Type()); tag != "" {
			typeHints, sizeHints, maxSizeHints, err = ssztypes.ParseTags(tag)
			if err != nil {
				t.Errorf("%s: annotation: %v", e.Name, err)
				continue
			}
		}
		desc, err := parser.GetTypeDescriptor(typeObj.Type(), typeHints, sizeHints, maxSizeHints)
		if err != nil {
			t.Errorf("%s (%s): go/types descriptor: %v", e.Name, e.Shape, err)
			continue
		}
		if e.Recursive {
			if _, err := certlib.CertJSON(desc); !errors.Is(err, certlib.ErrRecursive) {
				t.Errorf("%s (%s): recursive type should have no declaration, got err=%v", e.Name, e.Shape, err)
			}
			continue
		}
		// The in-house declaration, from the go/types builder with spec sizes
		// at their static defaults.
		if cert, err := certlib.CertJSON(desc); err != nil {
			t.Errorf("%s (%s): go/types in-house declaration: %v", e.Name, e.Shape, err)
		} else if cert != e.CertDeclStatic {
			t.Errorf("DECLARATION DIVERGENCE (go/types, in-house) %s (%s):\n  dynamic-ssz %s\n  intended    %s", e.Name, e.Shape, cert, e.CertDeclStatic)
		}
		if e.Extended || e.IllFormed {
			// No spec declaration: the two builders must agree on the in-house term.
			want, err1 := certlib.CertExpr(runtimeDescriptor(t, refl, e.New))
			got, err2 := certlib.CertExpr(desc)
			if err1 != nil || err2 != nil {
				t.Errorf("%s (%s): certificate terms: reflection %v, go/types %v", e.Name, e.Shape, err1, err2)
			} else if got != want {
				t.Errorf("BUILDER DIVERGENCE %s (%s):\n  go/types   %s\n  reflection %s", e.Name, e.Shape, got, want)
			}
			continue
		}
		got, err := certlib.SpecJSON(desc)
		if err != nil {
			t.Errorf("%s (%s): go/types declaration: %v", e.Name, e.Shape, err)
		} else if got != e.DeclStatic {
			t.Errorf("DECLARATION DIVERGENCE (go/types) %s (%s):\n  dynamic-ssz %s\n  intended    %s", e.Name, e.Shape, got, e.DeclStatic)
		}
	}
	if annErr := annIdx.Err(); annErr != nil {
		t.Errorf("annotations: %v", annErr)
	}
}
