// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package main implements the dynssz-gen command.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"golang.org/x/tools/go/packages"
)

// Config holds the configuration for the code generator
type Config struct {
	PackagePath               string
	PackageName               string
	TypeNames                 string
	OutputFile                string
	HeaderTemplate            string
	Verbose                   bool
	Legacy                    bool
	WithoutDynamicExpressions bool
	WithoutFastSsz            bool
	WithStreaming             bool
	WithExtendedTypes         bool
	RecursionDepth            int
	// Remove moves the configured output files aside before the package is
	// loaded, so stale generated code cannot block the analysis; they are
	// deleted once generation succeeds and restored if it fails.
	Remove bool

	// Method exclusions, settable through the config file only (no CLI
	// flag counterparts).
	SkipMarshal      bool
	SkipUnmarshal    bool
	SkipSize         bool
	SkipHashTreeRoot bool
	SkipEncoder      bool
	SkipDecoder      bool

	// TypeSpecs, when non-nil, overrides TypeNames parsing. Populated by the
	// config-file path so per-type override booleans carry through.
	TypeSpecs []typeSpec
}

// typeSpec holds parsed information about a type specification
type typeSpec struct {
	TypeName   string
	OutputFile string
	ViewTypes  []string // view types for data+views mode (can include package paths)
	IsViewOnly bool     // whether this is a view-only type

	// Per-type effective codegen flags. When HasPerTypeOverrides is false
	// these are unused and the global Config values apply. When true, each
	// boolean is the resolved effective value (global default with optional
	// override) and is applied at the per-type level only — never the file
	// level — because codegen With* options can only set booleans to true.
	HasPerTypeOverrides       bool
	Legacy                    bool
	WithoutDynamicExpressions bool
	WithoutFastSsz            bool
	WithStreaming             bool
	WithExtendedTypes         bool
	RecursionDepth            int
	SkipMarshal               bool
	SkipUnmarshal             bool
	SkipSize                  bool
	SkipHashTreeRoot          bool
	SkipEncoder               bool
	SkipDecoder               bool

	// Resolved during run()'s validation pass; reused when building codegen
	// options so we don't look up or re-resolve the same symbols twice (and
	// so the second pass doesn't need defensive error handling for cases the
	// first pass already ruled out).
	resolvedGoType    types.Type
	resolvedViewTypes []types.Type
}

// loadPackages is the loader used by run() and its external-package helper.
// It wraps packages.Load so tests can substitute a failing loader to
// exercise the top-level error paths that packages.Load practically never
// hits in production.
var loadPackages = packages.Load

// viewTypeRef holds a parsed view type reference
type viewTypeRef struct {
	PackagePath string // empty for local types
	TypeName    string
}

// parseViewTypeRef parses a view type reference string.
// Format: "TypeName" for local types, "pkgpath.TypeName" for external types.
// The last dot separates the package path from the type name.
func parseViewTypeRef(ref string) viewTypeRef {
	// Find the last dot to split package path from type name
	lastDot := strings.LastIndex(ref, ".")
	if lastDot == -1 {
		// No dot means local type
		return viewTypeRef{TypeName: ref}
	}

	// Check if this looks like a package path (contains "/" or starts with known prefixes)
	pkgPath := ref[:lastDot]
	return viewTypeRef{
		PackagePath: pkgPath,
		TypeName:    ref[lastDot+1:],
	}
}

// getVersionString returns the full version string with build metadata.
func getVersionString() string {
	v := "v" + codegen.Version

	if codegen.BuildCommit != "" {
		v += " (commit: " + codegen.BuildCommit
		if codegen.BuildTime != "" {
			v += ", built: " + codegen.BuildTime
		}
		v += ")"
	}

	return v
}

func main() {
	var (
		configPath                = flag.String("config", "", "Path to YAML config file")
		packagePath               = flag.String("package", "", "Go package path to analyze")
		packageName               = flag.String("package-name", "", "Package name for generated code")
		typeNames                 = flag.String("types", "", "Comma-separated list of type names to generate code for")
		outputFile                = flag.String("output", "", "Output file path for generated code")
		headerTemplate            = flag.String("header", "", "Custom header comment template for generated files ({hash} and {version} placeholders are substituted)")
		verbose                   = flag.Bool("v", false, "Verbose output")
		legacy                    = flag.Bool("legacy", false, "Generate legacy methods")
		withoutDynamicExpressions = flag.Bool("without-dynamic-expressions", false, "Generate code without dynamic expressions")
		withoutFastSsz            = flag.Bool("without-fastssz", false, "Generate code without using fast ssz generated methods")
		withStreaming             = flag.Bool("with-streaming", false, "Generate streaming functions")
		withExtendedTypes         = flag.Bool("with-extended-types", false, "Generate code with extended types")
		recursionDepth            = flag.Int("recursion-depth", 0, "Nesting depth at which generated code rejects a recursive value (0 = default)")
		remove                    = flag.Bool("remove", false, "Move the configured output files aside before generating; restored if generation fails")
		showVersion               = flag.Bool("version", false, "Print version and exit")
	)

	flag.Usage = func() {
		w := os.Stderr
		_, _ = fmt.Fprintf(w, "dynssz-gen %s\n\n", getVersionString())
		_, _ = fmt.Fprintf(w, "Go code generator for dynamic SSZ marshaling, unmarshaling, and hash tree root.\n\n")
		_, _ = fmt.Fprintf(w, "Usage:\n")
		_, _ = fmt.Fprintf(w, "  dynssz-gen -package <path> -types <types> [-output <file>] [flags]\n")
		_, _ = fmt.Fprintf(w, "  dynssz-gen -config <file> [flags]\n\n")
		_, _ = fmt.Fprintf(w, "See docs/code-generator-config.md for the config file format.\n\n")
		_, _ = fmt.Fprintf(w, "Types syntax:\n")
		_, _ = fmt.Fprintf(w, "  Comma-separated list of type names from the target package.\n")
		_, _ = fmt.Fprintf(w, "  Each type can have colon-separated options:\n")
		_, _ = fmt.Fprintf(w, "    TypeName                              uses the -output file\n")
		_, _ = fmt.Fprintf(w, "    TypeName:path/out.go                  writes to a specific file\n")
		_, _ = fmt.Fprintf(w, "    TypeName:views=View1;View2            generates view-aware code\n")
		_, _ = fmt.Fprintf(w, "    TypeName:out.go:views=View1:viewonly  combines options\n\n")
		_, _ = fmt.Fprintf(w, "  View types can include package paths: views=pkg/path.ViewType\n")
		_, _ = fmt.Fprintf(w, "  The 'viewonly' flag generates only view methods (no base methods).\n\n")
		_, _ = fmt.Fprintf(w, "  Example: -types \"BeaconState:state_gen.go,BeaconBlock:block_gen.go\"\n")
		_, _ = fmt.Fprintf(w, "  Example: -types \"Base:gen.go:views=ViewA;ViewB;pkg.ViewC\"\n\n")
		_, _ = fmt.Fprintf(w, "Required flags:\n")
		_, _ = fmt.Fprintf(w, "  -package string\n")
		_, _ = fmt.Fprintf(w, "        Go package path to analyze\n")
		_, _ = fmt.Fprintf(w, "  -types string\n")
		_, _ = fmt.Fprintf(w, "        Comma-separated list of type names to generate code for\n\n")
		_, _ = fmt.Fprintf(w, "Output flags:\n")
		_, _ = fmt.Fprintf(w, "  -output string\n")
		_, _ = fmt.Fprintf(w, "        Default output file path. A type overrides it with a\n")
		_, _ = fmt.Fprintf(w, "        ':output=file.go' suffix in -types.\n")
		_, _ = fmt.Fprintf(w, "  -config string\n")
		_, _ = fmt.Fprintf(w, "        YAML config file; CLI flags override its top-level values.\n")
		_, _ = fmt.Fprintf(w, "  -remove\n")
		_, _ = fmt.Fprintf(w, "        Move the configured output files aside before loading the package,\n")
		_, _ = fmt.Fprintf(w, "        so generated code from an earlier run cannot block the analysis;\n")
		_, _ = fmt.Fprintf(w, "        they are deleted once generation succeeds and restored if it fails.\n")
		_, _ = fmt.Fprintf(w, "        A stash left behind by an interrupted run (<output>.dynssz-gen.orig) is never overwritten.\n")
		_, _ = fmt.Fprintf(w, "  -package-name string\n")
		_, _ = fmt.Fprintf(w, "        Package name for generated code (default: same as source package)\n")
		_, _ = fmt.Fprintf(w, "  -header string\n")
		_, _ = fmt.Fprintf(w, "        Custom header comment template for generated files.\n")
		_, _ = fmt.Fprintf(w, "        {hash} and {version} placeholders are substituted. The first line\n")
		_, _ = fmt.Fprintf(w, "        should match `^// Code generated .* DO NOT EDIT\\.$` so tooling\n")
		_, _ = fmt.Fprintf(w, "        recognizes the files as generated.\n\n")
		_, _ = fmt.Fprintf(w, "Code generation flags:\n")
		_, _ = fmt.Fprintf(w, "  -legacy\n")
		_, _ = fmt.Fprintf(w, "        Generate legacy MarshalSSZ/UnmarshalSSZ/HashTreeRoot methods\n")
		_, _ = fmt.Fprintf(w, "  -with-streaming\n")
		_, _ = fmt.Fprintf(w, "        Generate streaming encoder/decoder functions\n")
		_, _ = fmt.Fprintf(w, "  -with-extended-types\n")
		_, _ = fmt.Fprintf(w, "        Generate code with extended types\n")
		_, _ = fmt.Fprintf(w, "  -without-dynamic-expressions\n")
		_, _ = fmt.Fprintf(w, "        Generate code without dynamic expressions\n")
		_, _ = fmt.Fprintf(w, "  -without-fastssz\n")
		_, _ = fmt.Fprintf(w, "        Generate code without using fast ssz generated methods\n\n")
		_, _ = fmt.Fprintf(w, "Other flags:\n")
		_, _ = fmt.Fprintf(w, "  -v    Verbose output\n")
		_, _ = fmt.Fprintf(w, "  -version\n")
		_, _ = fmt.Fprintf(w, "        Print version and exit\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("dynssz-gen %s\n", getVersionString())
		return
	}

	if *configPath == "" && *packagePath == "" && *typeNames == "" {
		flag.Usage()
		return
	}

	config := Config{
		PackagePath:               *packagePath,
		PackageName:               *packageName,
		TypeNames:                 *typeNames,
		OutputFile:                *outputFile,
		HeaderTemplate:            *headerTemplate,
		Verbose:                   *verbose,
		Legacy:                    *legacy,
		WithoutDynamicExpressions: *withoutDynamicExpressions,
		WithoutFastSsz:            *withoutFastSsz,
		WithStreaming:             *withStreaming,
		WithExtendedTypes:         *withExtendedTypes,
		RecursionDepth:            *recursionDepth,
		Remove:                    *remove,
	}

	if *configPath != "" {
		cliProvided := providedFlagSet()
		fc, err := LoadConfig(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		baseDir := filepath.Dir(*configPath)
		specs, err := fc.applyToConfig(&config, cliProvided, baseDir)
		if err != nil {
			log.Fatal(err)
		}
		config.TypeSpecs = specs
	}

	if err := run(&config); err != nil {
		log.Fatal(err)
	}
}

// providedFlagSet returns the set of flags that the user explicitly passed on
// the command line. The flag package otherwise exposes only the resolved
// value, which makes it impossible to tell "user passed -legacy=false" from
// "user did not pass -legacy at all" — we need that distinction for the
// config-file precedence rules.
func providedFlagSet() map[string]bool {
	provided := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		provided[f.Name] = true
	})
	return provided
}

// seedTransitiveImports records every transitively imported package with
// loaded type information in the cache, keyed by import path.
func seedTransitiveImports(cache map[string]*packages.Package, p *packages.Package) {
	for impPath, impPkg := range p.Imports {
		if _, ok := cache[impPath]; ok {
			continue
		}
		if impPkg == nil || impPkg.Types == nil {
			continue
		}
		cache[impPath] = impPkg
		seedTransitiveImports(cache, impPkg)
	}
}

func run(config *Config) error {
	if config.PackagePath == "" {
		return errors.New("package path is required (-package)")
	}
	if config.TypeNames == "" && len(config.TypeSpecs) == 0 {
		return errors.New("type names are required (-types)")
	}

	if config.Verbose {
		log.Printf("Analyzing package: %s", config.PackagePath)
		if config.TypeNames != "" {
			log.Printf("Looking for types: %s", config.TypeNames)
		} else {
			log.Printf("Looking for %d types (from config file)", len(config.TypeSpecs))
		}
	}

	var typeSpecs []typeSpec
	if len(config.TypeSpecs) > 0 {
		typeSpecs = config.TypeSpecs
	} else {
		specs, err := parseTypeSpecs(config.TypeNames, config.OutputFile)
		if err != nil {
			return err
		}
		typeSpecs = specs
	}
	if err := checkOutputCollisions(typeSpecs); err != nil {
		return err
	}
	if config.Remove {
		return withStashedOutputs(typeSpecs, func() error {
			return runGeneration(config, typeSpecs)
		})
	}
	return runGeneration(config, typeSpecs)
}

// checkOutputCollisions refuses two output paths that name one file. Every
// path is cleaned when it is parsed, so equal spellings already group
// together; two spellings of the same file that survive cleaning (a relative
// and an absolute one, or two directories linked to each other) would
// otherwise be generated separately and then written onto each other, keeping
// only the last.
func checkOutputCollisions(typeSpecs []typeSpec) error {
	seen := map[string]string{}
	for _, spec := range typeSpecs {
		if spec.OutputFile == "" {
			continue
		}
		abs, err := filepath.Abs(spec.OutputFile)
		if err != nil {
			return fmt.Errorf("resolve output %s: %w", spec.OutputFile, err)
		}
		// A link in the path names the same file under another spelling, which
		// only the filesystem can resolve. The output file itself is created by
		// this run, so only its directory can be resolved; a directory that
		// does not exist yet is created here and holds no links.
		if dir, linkErr := filepath.EvalSymlinks(filepath.Dir(abs)); linkErr == nil {
			abs = filepath.Join(dir, filepath.Base(abs))
		}
		if first, ok := seen[abs]; ok && first != spec.OutputFile {
			return fmt.Errorf("output files %s and %s name the same file: spell the target the same way in every type", first, spec.OutputFile)
		}
		seen[abs] = spec.OutputFile
	}
	return nil
}

// withStashedOutputs runs generate with the output files moved aside. They
// are deleted when generate returns nil and restored otherwise, including
// when generate panics: the panic continues after the files are back.
func withStashedOutputs(typeSpecs []typeSpec, generate func() error) (err error) {
	stash, err := stashOutputs(typeSpecs)
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if done {
			stash.discard()
		} else {
			stash.restore()
		}
	}()
	if err := generate(); err != nil {
		return err
	}
	done = true
	return nil
}

// runGeneration loads the package and writes the generated files.
func runGeneration(config *Config, typeSpecs []typeSpec) error {
	// Parse the Go package. NeedImports + NeedDeps make the main package's
	// transitively-loaded dependencies (e.g. "spec/phase0" reached via
	// "spec/all") available with the same *types.Named instances the main
	// package uses. We seed them into the external-package cache below so
	// that view types pointing into those same packages share types,
	// preventing the parser from misclassifying nested fields as views.
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName |
			packages.NeedImports | packages.NeedDeps,
	}

	pkgs, err := loadPackages(cfg, config.PackagePath)
	if err != nil {
		return fmt.Errorf("failed to load package %s: %v", config.PackagePath, err)
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found for %s", config.PackagePath)
	}

	if len(pkgs) > 1 {
		names := make([]string, 0, len(pkgs))
		for _, matched := range pkgs {
			names = append(names, matched.PkgPath)
		}
		return fmt.Errorf("pattern %s matches %d packages (%s); name exactly one", config.PackagePath, len(pkgs), strings.Join(names, ", "))
	}

	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		for _, err := range pkg.Errors {
			log.Printf("Package error: %v", err)
		}
		return fmt.Errorf("package %s has errors", config.PackagePath)
	}

	if config.Verbose {
		log.Printf("Successfully loaded package: %s", pkg.Name)
	}

	// Find the requested types in the package
	// Map from output file to list of type specs
	generateFiles := make(map[string][]typeSpec)
	mainScope := pkg.Types.Scope()
	typeCount := 0

	// Cache for loaded external packages. Pre-populate it from the main
	// package's transitive imports so that any view type that lives in a
	// package the main package already depends on resolves to the same
	// *types.Named instance the main package uses. Without this, separate
	// packages.Load calls produce distinct *types.Named pointers for the
	// same logical type, which causes the parser's pointer-equality check
	// (data != schema) to misclassify every nested struct as a view —
	// preventing fastssz delegation to per-fork generated methods.
	externalPackages := make(map[string]*packages.Package)
	seedTransitiveImports(externalPackages, pkg)
	if config.Verbose {
		log.Printf("Seeded %d transitive packages from main", len(externalPackages))
	}

	// Helper to load and cache an external package. Lookups go through the
	// transitively-seeded cache first; on a miss we do a fresh packages.Load
	// and then re-check the cache by the canonical PkgPath so a relative
	// pattern like "../phase0" still resolves to the main-package's already
	// loaded copy of "github.com/.../phase0".
	loadExternalPackage := func(pkgPath string) (*packages.Package, error) {
		if cached, ok := externalPackages[pkgPath]; ok {
			return cached, nil
		}

		extPkgs, err2 := loadPackages(cfg, pkgPath)
		if err2 != nil {
			return nil, fmt.Errorf("failed to load external package %s: %w", pkgPath, err2)
		}
		if len(extPkgs) == 0 {
			return nil, fmt.Errorf("external package %s not found", pkgPath)
		}
		if len(extPkgs[0].Errors) > 0 {
			return nil, fmt.Errorf("external package %s has errors: %v", pkgPath, extPkgs[0].Errors[0])
		}

		// If the canonical import path is already in our seeded cache (because
		// the main package transitively imports it), use that copy so types
		// are pointer-identical to the ones the main package sees.
		canonical := extPkgs[0].PkgPath
		if seeded, ok := externalPackages[canonical]; ok {
			externalPackages[pkgPath] = seeded
			return seeded, nil
		}

		externalPackages[pkgPath] = extPkgs[0]
		externalPackages[canonical] = extPkgs[0]
		if config.Verbose {
			log.Printf("Loaded external package: %s", pkgPath)
		}
		return extPkgs[0], nil
	}

	// Helper to resolve a type reference (local or external)
	resolveTypeRef := func(ref viewTypeRef) (types.Type, error) {
		var scope *types.Scope
		var pkgPath string

		if ref.PackagePath == "" {
			// Local type
			scope = mainScope
			pkgPath = config.PackagePath
		} else {
			// External type
			extPkg, err2 := loadExternalPackage(ref.PackagePath)
			if err2 != nil {
				return nil, err2
			}
			scope = extPkg.Types.Scope()
			pkgPath = ref.PackagePath
		}

		obj := scope.Lookup(ref.TypeName)
		if obj == nil {
			return nil, fmt.Errorf("type %s not found in package %s", ref.TypeName, pkgPath)
		}

		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			return nil, fmt.Errorf("object %s is not a type in package %s", ref.TypeName, pkgPath)
		}

		return typeObj.Type(), nil
	}

	for i := range typeSpecs {
		spec := &typeSpecs[i]

		// Validate that the main type exists
		obj := mainScope.Lookup(spec.TypeName)
		if obj == nil {
			return fmt.Errorf("type %s not found in package %s", spec.TypeName, config.PackagePath)
		}

		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			return fmt.Errorf("object %s is not a type in package %s", spec.TypeName, config.PackagePath)
		}
		// An alias names another type; a receiver written for it would declare
		// methods on that other type, or not compile at all. The declaration
		// is checked here because go/types may resolve the alias away before
		// the generator sees it.
		if typeObj.IsAlias() {
			return fmt.Errorf("type %s in package %s is an alias: methods cannot be declared on an alias; generate the aliased type or declare a named type", spec.TypeName, config.PackagePath)
		}
		spec.resolvedGoType = typeObj.Type()

		// Resolve view types exactly once; cache on the spec so the second
		// pass doesn't need to repeat the lookup (or handle errors the first
		// pass already rejected).
		if len(spec.ViewTypes) > 0 {
			spec.resolvedViewTypes = make([]types.Type, 0, len(spec.ViewTypes))
			for _, viewTypeStr := range spec.ViewTypes {
				ref := parseViewTypeRef(viewTypeStr)
				viewType, err2 := resolveTypeRef(ref)
				if err2 != nil {
					return fmt.Errorf("view type %s: %w", viewTypeStr, err2)
				}
				spec.resolvedViewTypes = append(spec.resolvedViewTypes, viewType)
			}
		}

		if _, ok := generateFiles[spec.OutputFile]; !ok {
			generateFiles[spec.OutputFile] = make([]typeSpec, 0)
		}
		generateFiles[spec.OutputFile] = append(generateFiles[spec.OutputFile], *spec)
		typeCount++

		if config.Verbose {
			mode := "data-only"
			if spec.IsViewOnly {
				mode = "view-only"
			} else if len(spec.ViewTypes) > 0 {
				mode = fmt.Sprintf("data+views(%s)", strings.Join(spec.ViewTypes, ","))
			}
			log.Printf("Found type: %s [%s]", typeObj.Name(), mode)
		}
	}

	// Create codegen instance
	typeCache := ssztypes.NewTypeCache(nil)
	codeGen := codegen.NewCodeGenerator(typeCache)

	// Let the parser read same-package types' annotations (incl. generated
	// ssz-static declarations) so referenced fully-delegated types are
	// shallow-built rather than traversed and validated.
	codeGen.SetAnnotationResolver(annotationResolver(pkg))

	if config.PackageName != "" {
		if nameErr := codeGen.SetPackageName(config.PackageName); nameErr != nil {
			return nameErr
		}
	}

	if config.HeaderTemplate != "" {
		if headerErr := codeGen.SetHeaderTemplate(config.HeaderTemplate); headerErr != nil {
			if errors.Is(headerErr, codegen.ErrInvalidHeaderTemplate) {
				return headerErr
			}
			log.Printf("Warning: %v", headerErr)
		}
	}

	// Build options for all types, in a stable file order so a run's log and
	// its first reported error do not depend on map iteration.
	outFiles := make([]string, 0, len(generateFiles))
	for outFile := range generateFiles {
		outFiles = append(outFiles, outFile)
	}
	sort.Strings(outFiles)
	for _, outFile := range outFiles {
		specs := generateFiles[outFile]
		var typeOptions []codegen.CodeGeneratorOption

		for _, spec := range specs {
			// Type + view types were already resolved and cached on the spec
			// during the first validation pass, so no lookup/error handling
			// is needed here.
			goType := spec.resolvedGoType

			// Build type-specific options
			var typeSpecificOpts []codegen.CodeGeneratorOption

			// Parse SSZ annotations from sszutils.Annotate[T]() calls in source
			if tag := findAnnotateCall(pkg, annotatedNamedType(goType, pkg)); tag != "" {
				annotateOpts, parseErr := parseAnnotateTag(tag)
				if parseErr != nil {
					return fmt.Errorf("failed to parse Annotate tag for type %s: %v", spec.TypeName, parseErr)
				}
				typeSpecificOpts = append(typeSpecificOpts, annotateOpts...)

				if config.Verbose && len(annotateOpts) > 0 {
					log.Printf("Found Annotate call for type %s", spec.TypeName)
				}
			}

			// Add view types if any were resolved.
			if len(spec.resolvedViewTypes) > 0 {
				typeSpecificOpts = append(typeSpecificOpts, codegen.WithGoTypesViewTypes(spec.resolvedViewTypes...))
			}

			// Add view-only flag if specified
			if spec.IsViewOnly {
				typeSpecificOpts = append(typeSpecificOpts, codegen.WithViewOnly())
			}

			// When the config file populated per-type overrides, every codegen
			// boolean is applied here at the per-type level. This is the only
			// way for a specific type to opt *out* of a globally enabled flag:
			// codegen's With* options can only set a boolean to true, so if
			// we left the option on at the file level we could never turn it
			// off for one type.
			if spec.HasPerTypeOverrides {
				typeSpecificOpts = append(typeSpecificOpts, codegenFlagOptions(&spec)...)
			}

			// Add the type with its options
			typeOptions = append(typeOptions, codegen.WithGoTypesType(goType, typeSpecificOpts...))
		}

		// File-level flags are only used when no per-type overrides are in
		// play (i.e. the legacy CLI path). In the config-file path each type
		// already carries its fully-resolved per-type options above.
		if !anyHasOverrides(specs) {
			typeOptions = append(typeOptions, codegenFlagOptions(&typeSpec{
				Legacy:                    config.Legacy,
				WithoutDynamicExpressions: config.WithoutDynamicExpressions,
				WithoutFastSsz:            config.WithoutFastSsz,
				WithStreaming:             config.WithStreaming,
				WithExtendedTypes:         config.WithExtendedTypes,
				RecursionDepth:            config.RecursionDepth,
				SkipMarshal:               config.SkipMarshal,
				SkipUnmarshal:             config.SkipUnmarshal,
				SkipSize:                  config.SkipSize,
				SkipHashTreeRoot:          config.SkipHashTreeRoot,
				SkipEncoder:               config.SkipEncoder,
				SkipDecoder:               config.SkipDecoder,
			})...)
		}

		// Build the file with all types
		codeGen.BuildFile(outFile, typeOptions...)
	}

	// Generate the code
	if config.Verbose {
		log.Printf("Generating code...")
	}

	codeMap, err := codeGen.GenerateToMap()
	if err != nil {
		return fmt.Errorf("failed to generate code: %v", err)
	}

	codeSize, err := writeOutputFiles(codeMap, config.Verbose)
	if err != nil {
		return err
	}

	// Warnings do not fail generation, but the author has to see them: they
	// flag output that is valid Go yet will not interoperate. They go to stderr
	// with the tool's other diagnostics, so a caller reading stdout gets only
	// the generation summary.
	for _, warning := range codeGen.Warnings() {
		_, _ = fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	if config.Verbose {
		log.Printf("Successfully generated %d bytes of code for %d types to %d files", codeSize, typeCount, len(generateFiles))
	} else {
		fmt.Printf("Generated SSZ code for %d types to %d files\n", typeCount, len(generateFiles))
	}

	return nil
}

func annotationResolver(pkg *packages.Package) func(types.Type) string {
	return func(t types.Type) string {
		target := annotatedNamedType(t, pkg)
		if target == nil {
			return ""
		}
		return findAnnotateCall(pkg, target)
	}
}

// annotatedNamedType returns the named type t stands for when that type is
// declared in pkg, or nil. An alias is transparent and one pointer level is
// stripped, as the runtime registration does. Only same-package types are
// resolved, since annotations are scanned within pkg.
func annotatedNamedType(t types.Type, pkg *packages.Package) *types.Named {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Pkg().Path() != pkg.PkgPath {
		return nil
	}
	return named
}

// annotateTypeArgMatches reports whether the type argument of an Annotate call
// is target. With type information the argument is resolved the way the
// runtime registration resolves it (an alias is transparent, one pointer level
// is stripped) and compared as a type, so instantiations of one generic type
// and same-named types of other packages stay apart; without it the argument
// has to be spelled as the target's own name.
func annotateTypeArgMatches(pkg *packages.Package, arg ast.Expr, target *types.Named) bool {
	if pkg != nil && pkg.TypesInfo != nil {
		if typ := pkg.TypesInfo.TypeOf(arg); typ != nil {
			typ = types.Unalias(typ)
			if ptr, ok := typ.(*types.Pointer); ok {
				typ = types.Unalias(ptr.Elem())
			}
			return types.Identical(typ, target)
		}
	}
	ident, ok := arg.(*ast.Ident)
	return ok && ident.Name == target.Obj().Name()
}

// findAnnotateCall scans package AST for sszutils.Annotate[target]("...")
// calls and returns the merged tag, or "" if not found. The calls are taken
// in the package's initialization order (package-level variables of every
// file in file order, then the init functions) and merged newest first, as
// the runtime registration does, so a key registered twice resolves to the
// same registration in both.
func findAnnotateCall(pkg *packages.Package, target *types.Named) string {
	if target == nil {
		return ""
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
				varTags = append(varTags, findAnnotateCallsInVarDecl(pkg, d, sszutilsAlias, target)...)
			case *ast.FuncDecl:
				initTags = append(initTags, findAnnotateCallsInInit(pkg, d, sszutilsAlias, target)...)
			}
		}
	}

	ordered := make([]string, 0, len(varTags)+len(initTags))
	ordered = append(ordered, varTags...)
	ordered = append(ordered, initTags...)
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

// findAnnotateCallsInVarDecl returns the tags of every Annotate call for
// target among the initializers of a package-level var declaration.
func findAnnotateCallsInVarDecl(pkg *packages.Package, d *ast.GenDecl, alias string, target *types.Named) []string {
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
			if tag := matchAnnotateCall(pkg, val, alias, target); tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// findAnnotateCallsInInit returns the tags of every Annotate call for target
// among the statements of an init function.
func findAnnotateCallsInInit(pkg *packages.Package, d *ast.FuncDecl, alias string, target *types.Named) []string {
	if d.Name.Name != "init" || d.Body == nil {
		return nil
	}
	var tags []string
	for _, stmt := range d.Body.List {
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}

		if tag := matchAnnotateCall(pkg, exprStmt.X, alias, target); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// matchAnnotateCall checks if an expression is sszutils.Annotate[target]("...tag...")
// and returns the tag string, or "" if it doesn't match (see
// annotateTypeArgMatches for how the type argument is compared).
func matchAnnotateCall(pkg *packages.Package, expr ast.Expr, alias string, target *types.Named) string {
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
	if !annotateTypeArgMatches(pkg, indexExpr.Index, target) {
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

// parseAnnotateTag parses an SSZ tag string into codegen options.
func parseAnnotateTag(tag string) ([]codegen.CodeGeneratorOption, error) {
	typeHints, sizeHints, maxSizeHints, err := ssztypes.ParseTags(tag)
	if err != nil {
		return nil, err
	}

	var opts []codegen.CodeGeneratorOption
	if len(typeHints) > 0 {
		opts = append(opts, codegen.WithTypeHints(typeHints))
	}

	if len(sizeHints) > 0 {
		opts = append(opts, codegen.WithSizeHints(sizeHints))
	}

	if len(maxSizeHints) > 0 {
		opts = append(opts, codegen.WithMaxSizeHints(maxSizeHints))
	}

	return opts, nil
}

// writeOutputFiles writes the generated file set atomically: every file lands
// next to its target as a temp file first, and the renames happen only after
// all writes succeeded, so a failure leaves no partial mix of old and new
// output. Files are written in stable order; the total byte count is returned.
func writeOutputFiles(codeMap map[string]string, verbose bool) (int, error) {
	outFiles := make([]string, 0, len(codeMap))
	for outFile := range codeMap {
		outFiles = append(outFiles, outFile)
	}
	sort.Strings(outFiles)

	codeSize := 0
	tempFiles := make(map[string]string, len(codeMap))
	cleanup := func() {
		for _, tempFile := range tempFiles {
			_ = os.Remove(tempFile)
		}
	}
	for _, outFile := range outFiles {
		generatedCode := codeMap[outFile]
		if verbose {
			log.Printf("Writing output to %s", outFile)
		}
		// Renaming onto a directory would fail with a filesystem-dependent
		// errno; name the actual problem instead.
		if fi, statErr := os.Stat(outFile); statErr == nil && fi.IsDir() {
			cleanup()
			return 0, fmt.Errorf("failed to write output file %s: the target is a directory", outFile)
		}
		codeSize += len(generatedCode)
		tempFile, err := writeTempFile(outFile, []byte(generatedCode))
		if err != nil {
			cleanup()
			return 0, fmt.Errorf("failed to write output file %s: %v", outFile, err)
		}
		tempFiles[outFile] = tempFile
	}
	for _, outFile := range outFiles {
		if err := renameFile(tempFiles[outFile], outFile); err != nil {
			cleanup()
			return 0, fmt.Errorf("failed to write output file %s: %v", outFile, err)
		}
		delete(tempFiles, outFile)
	}

	return codeSize, nil
}

// writeTempFile writes data to a fresh temp file in the target's directory
// and returns its path; renaming it onto the target is then atomic on the
// same filesystem.
func writeTempFile(target string, data []byte) (string, error) {
	f, err := createTempFile(filepath.Dir(target), filepath.Base(target)+".tmp*")
	if err != nil {
		return "", errCause(err)
	}
	name := f.Name()
	_, writeErr := f.Write(data)
	if closeErr := f.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(name)
		return "", writeErr
	}
	return name, nil
}

// Filesystem seams so tests can inject failures the real filesystem cannot
// produce deterministically.
var (
	createTempFile = os.CreateTemp
	renameFile     = os.Rename
)

// errCause strips the temp-file path a *fs.PathError carries, so
// creation-failure messages name only the target file the caller reports.
func errCause(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}

// stashSuffix is appended to an output file moved aside by -remove. The
// suffix does not end in .go, so a stashed file is not part of the package.
const stashSuffix = ".dynssz-gen.orig"

// outputStash holds the output files -remove moved aside: generated code from
// an earlier run is part of the package the generator type-checks, so it must
// be out of the way before the package is loaded when the types it was
// generated for changed. The files come back if generation fails.
type outputStash struct {
	moved []string
}

// stashOutputs moves every distinct existing output file the type specs name
// aside. A file that does not exist is skipped.
func stashOutputs(typeSpecs []typeSpec) (*outputStash, error) {
	stash := &outputStash{}
	seen := map[string]bool{}
	for _, spec := range typeSpecs {
		if spec.OutputFile == "" || seen[spec.OutputFile] {
			continue
		}
		seen[spec.OutputFile] = true
		// A stash left behind by an interrupted run is the only copy of that
		// run's input; it is never overwritten.
		if _, statErr := os.Lstat(spec.OutputFile + stashSuffix); statErr == nil {
			stash.restore()
			return nil, fmt.Errorf("%s exists: a previous -remove run was interrupted; restore or delete it", spec.OutputFile+stashSuffix)
		}
		err := os.Rename(spec.OutputFile, spec.OutputFile+stashSuffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			stash.restore()
			return nil, fmt.Errorf("move %s aside: %w", spec.OutputFile, err)
		}
		stash.moved = append(stash.moved, spec.OutputFile)
	}
	return stash, nil
}

// restore moves the stashed files back, replacing whatever a failed run left.
func (s *outputStash) restore() {
	for _, f := range s.moved {
		if err := os.Rename(f+stashSuffix, f); err != nil {
			log.Printf("Restoring %s: %v", f, err)
		}
	}
	s.moved = nil
}

// discard deletes the stashed files once generation succeeded.
func (s *outputStash) discard() {
	for _, f := range s.moved {
		if err := os.Remove(f + stashSuffix); err != nil {
			log.Printf("Removing %s: %v", f+stashSuffix, err)
		}
	}
	s.moved = nil
}

// parseTypeSpecs parses the comma-separated type names string into typeSpec structs.
// Each type can have colon-separated options: TypeName[:output=file.go][:views=View1;View2][:viewonly]
func parseTypeSpecs(typeNames, defaultOutput string) ([]typeSpec, error) {
	requestedTypes := strings.Split(typeNames, ",")
	typeSpecs := make([]typeSpec, 0, len(requestedTypes))

	for _, typeStr := range requestedTypes {
		typeStr = strings.TrimSpace(typeStr)
		if typeStr == "" {
			continue
		}

		spec := typeSpec{}
		parts := strings.Split(typeStr, ":")

		// First part is always the type name
		spec.TypeName = strings.TrimSpace(parts[0])
		if spec.TypeName == "" {
			return nil, fmt.Errorf("invalid type spec %q: empty type name", typeStr)
		}

		// Process remaining parts - all are optional and can be in any order
		for i := 1; i < len(parts); i++ {
			part := parts[i]

			// Skip empty parts (from consecutive colons like TypeName::views=X)
			if part == "" {
				continue
			}

			switch {
			case strings.HasPrefix(part, "views="):
				// Parse view types: views=View1;View2
				viewsStr := strings.TrimPrefix(part, "views=")
				// Use semicolon as separator since comma is used for type list
				spec.ViewTypes = strings.Split(viewsStr, ";")
				for j := range spec.ViewTypes {
					spec.ViewTypes[j] = strings.TrimSpace(spec.ViewTypes[j])
					if spec.ViewTypes[j] == "" {
						return nil, fmt.Errorf("invalid type spec %q: empty view type name", typeStr)
					}
				}
			case strings.HasPrefix(part, "output="):
				// Explicit output file with prefix
				spec.OutputFile = strings.TrimPrefix(part, "output=")
			case part == "viewonly":
				spec.IsViewOnly = true
			default:
				// A bare segment names the output file (legacy positional
				// form) — but only one, and only one that looks like a Go
				// file. Anything else is a typo the caller has to see, not a
				// filename to silently create.
				if spec.OutputFile == "" && strings.HasSuffix(part, ".go") {
					spec.OutputFile = part
					continue
				}
				return nil, fmt.Errorf("invalid type spec segment %q in %q (expected output=<file.go>, views=<A;B>, or viewonly)", part, typeStr)
			}
		}

		if spec.IsViewOnly && len(spec.ViewTypes) == 0 {
			return nil, fmt.Errorf("invalid type spec %q: viewonly needs view types (views=<A;B>)", typeStr)
		}

		// Use default output file if not specified
		if spec.OutputFile == "" {
			if defaultOutput == "" {
				return nil, errors.New("output file is required (-output)")
			}
			spec.OutputFile = defaultOutput
		}
		spec.OutputFile = filepath.Clean(spec.OutputFile)

		typeSpecs = append(typeSpecs, spec)
	}

	return typeSpecs, nil
}
