// dynssz-certify emits Lean 4 certificates for SSZ types.
//
// For every requested Go type it builds the same go/types descriptor that
// dynssz-gen's code generation consumes (with dynssz-gen's own annotation
// resolver), translates it into a codec expression over the verified
// combinator library (formal/lean, SszCert) and, optionally, into a
// declaration in the official specification (formal/spec, EthLib), and emits
// per-type theorem instances. Type-checking the generated files with
// `lake build` machine-checks those theorems for the concrete type layout the
// generator saw. The translation itself lives in certlib, shared with the
// runtime certifier that reads the reflection engine's descriptors.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/types"
	"log"
	"strings"

	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/dynssz-gen/annotations"
	"github.com/pk910/dynamic-ssz/formal/certify/certlib"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"golang.org/x/tools/go/packages"
)

func main() {
	var (
		packagePath = flag.String("package", ".", "Go package to load")
		typeNames   = flag.String("types", "", "comma-separated type names to certify")
		outputFile  = flag.String("output", "Certified.lean", "output Lean file")
		specOutput  = flag.String("spec-output", "", "also emit EthLib `Desc` declarations to this Lean file")
		specThms    = flag.String("spec-theorems", "", "also emit per-type EthLib theorem instances to this Lean file (needs -spec-output)")
		verbose     = flag.Bool("v", false, "verbose output")
	)
	flag.Parse()

	if *typeNames == "" {
		log.Fatal("-types is required")
	}

	names := strings.Split(*typeNames, ",")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}

	if err := run(*packagePath, names, *outputFile, *specOutput, *specThms, *verbose); err != nil {
		log.Fatal(err)
	}
}

// loadPackage loads packagePath with syntax for its whole import graph (the
// annotation resolver reads registrations from every package), preferring the
// plain variant and falling back to the test variant for packages whose types
// only compile with their test files.
func loadPackage(packagePath string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax |
			packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %s: %w", packagePath, err)
	}
	var plain, testVariant *packages.Package
	for _, p := range pkgs {
		if p.Name == "main" || strings.HasSuffix(p.PkgPath, ".test") ||
			strings.HasSuffix(p.Name, "_test") {
			continue
		}
		switch {
		case p.ID == p.PkgPath:
			plain = p
		case strings.Contains(p.ID, " ["):
			testVariant = p
		}
	}
	pkg := plain
	if pkg == nil || len(pkg.Errors) > 0 {
		if testVariant != nil && len(testVariant.Errors) == 0 {
			pkg = testVariant
		}
	}
	if pkg == nil {
		return nil, fmt.Errorf("no suitable package found for %s", packagePath)
	}
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("package %s has errors: %v", packagePath, pkg.Errors[0])
	}
	return pkg, nil
}

func run(packagePath string, names []string, outputFile, specOutput, specThms string, verbose bool) error {
	pkg, err := loadPackage(packagePath)
	if err != nil {
		return err
	}
	annIdx, err := annotations.NewIndex(pkg)
	if err != nil {
		return err
	}

	parser := codegen.NewParser()
	// Certification needs the full structural descriptor, never a shallow
	// delegation stub for types that already carry generated methods.
	parser.NoDelegation = true
	// dynamic-ssz extended types (signed ints, floats, big ints, optionals,
	// classic unions) are certified against formal/lean/SszCert/Extended.lean.
	parser.ExtendedTypes = true
	parser.AnnotationResolver = annIdx.Resolve

	scope := pkg.Types.Scope()
	certs := make([]certlib.CertEntry, 0, len(names))
	var spec []certlib.SpecEntry
	for _, name := range names {
		obj := scope.Lookup(name)
		if obj == nil {
			return fmt.Errorf("type %s not found in package %s", name, pkg.PkgPath)
		}
		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			return fmt.Errorf("%s is not a type", name)
		}

		var typeHints []ssztypes.SszTypeHint
		var sizeHints []ssztypes.SszSizeHint
		var maxSizeHints []ssztypes.SszMaxSizeHint
		if tag := annIdx.Resolve(typeObj.Type()); tag != "" {
			typeHints, sizeHints, maxSizeHints, err = ssztypes.ParseTags(tag)
			if err != nil {
				return fmt.Errorf("failed to parse annotation tag for %s: %w", name, err)
			}
		}

		desc, err := parser.GetTypeDescriptor(typeObj.Type(), typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return fmt.Errorf("failed to build descriptor for %s: %w", name, err)
		}
		if annErr := annIdx.Err(); annErr != nil {
			return annErr
		}

		expr, err := certlib.CertExpr(desc)
		if err != nil {
			return fmt.Errorf("type %s: %w", name, err)
		}
		certs = append(certs, certlib.CertEntry{PkgName: pkg.Name, Name: name, Expr: expr})

		if specOutput != "" {
			sexpr, err := certlib.SpecExpr(desc)
			switch {
			case errors.Is(err, certlib.ErrNoSpec):
				// Extended types are certified against the in-house model only.
				if verbose {
					log.Printf("%s: no specification declaration (%v)", name, err)
				}
			case err != nil:
				return fmt.Errorf("type %s (spec): %w", name, err)
			default:
				spec = append(spec, certlib.SpecEntry{PkgName: pkg.Name, Name: name, Expr: sexpr})
			}
		}

		if verbose {
			log.Printf("certified %s", name)
		}
	}

	if err := certlib.WriteCertFile(certs, outputFile); err != nil {
		return err
	}
	fmt.Printf("wrote %d certificates to %s\n", len(certs), outputFile)
	if specOutput != "" {
		if err := certlib.WriteSpecFiles(spec, specOutput, specThms); err != nil {
			return err
		}
		fmt.Printf("wrote %d EthLib declarations to %s\n", len(spec), specOutput)
	}
	return nil
}
