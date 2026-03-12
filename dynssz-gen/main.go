// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package main implements the dynssz-gen command.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/types"
	"log"
	"os"
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
	Verbose                   bool
	Legacy                    bool
	WithoutDynamicExpressions bool
	WithoutFastSsz            bool
	WithStreaming             bool
	WithExtendedTypes         bool
}

// typeSpec holds parsed information about a type specification
type typeSpec struct {
	TypeName   string
	OutputFile string
	ViewTypes  []string // view types for data+views mode (can include package paths)
	IsViewOnly bool     // whether this is a view-only type
}

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
		packagePath               = flag.String("package", "", "Go package path to analyze")
		packageName               = flag.String("package-name", "", "Package name for generated code")
		typeNames                 = flag.String("types", "", "Comma-separated list of type names to generate code for")
		outputFile                = flag.String("output", "", "Output file path for generated code")
		verbose                   = flag.Bool("v", false, "Verbose output")
		legacy                    = flag.Bool("legacy", false, "Generate legacy methods")
		withoutDynamicExpressions = flag.Bool("without-dynamic-expressions", false, "Generate code without dynamic expressions")
		withoutFastSsz            = flag.Bool("without-fastssz", false, "Generate code without using fast ssz generated methods")
		withStreaming             = flag.Bool("with-streaming", false, "Generate streaming functions")
		withExtendedTypes         = flag.Bool("with-extended-types", false, "Generate code with extended types")
		showVersion               = flag.Bool("version", false, "Print version and exit")
	)

	flag.Usage = func() {
		w := os.Stderr
		fmt.Fprintf(w, "dynssz-gen %s\n\n", getVersionString())
		fmt.Fprintf(w, "Go code generator for dynamic SSZ marshaling, unmarshaling, and hash tree root.\n\n")
		fmt.Fprintf(w, "Usage:\n")
		fmt.Fprintf(w, "  dynssz-gen -package <path> -types <types> [-output <file>] [flags]\n\n")
		fmt.Fprintf(w, "Types syntax:\n")
		fmt.Fprintf(w, "  Comma-separated list of type names from the target package.\n")
		fmt.Fprintf(w, "  Each type can have colon-separated options:\n")
		fmt.Fprintf(w, "    TypeName                              uses the -output file\n")
		fmt.Fprintf(w, "    TypeName:path/out.go                  writes to a specific file\n")
		fmt.Fprintf(w, "    TypeName:views=View1;View2            generates view-aware code\n")
		fmt.Fprintf(w, "    TypeName:out.go:views=View1:viewonly  combines options\n\n")
		fmt.Fprintf(w, "  View types can include package paths: views=pkg/path.ViewType\n")
		fmt.Fprintf(w, "  The 'viewonly' flag generates only view methods (no base methods).\n\n")
		fmt.Fprintf(w, "  Example: -types \"BeaconState:state_gen.go,BeaconBlock:block_gen.go\"\n")
		fmt.Fprintf(w, "  Example: -types \"Base:gen.go:views=ViewA;ViewB;pkg.ViewC\"\n\n")
		fmt.Fprintf(w, "Required flags:\n")
		fmt.Fprintf(w, "  -package string\n")
		fmt.Fprintf(w, "        Go package path to analyze\n")
		fmt.Fprintf(w, "  -types string\n")
		fmt.Fprintf(w, "        Comma-separated list of type names to generate code for\n\n")
		fmt.Fprintf(w, "Output flags:\n")
		fmt.Fprintf(w, "  -output string\n")
		fmt.Fprintf(w, "        Default output file path (used for types without a ':path' suffix)\n")
		fmt.Fprintf(w, "  -package-name string\n")
		fmt.Fprintf(w, "        Package name for generated code (default: same as source package)\n\n")
		fmt.Fprintf(w, "Code generation flags:\n")
		fmt.Fprintf(w, "  -legacy\n")
		fmt.Fprintf(w, "        Generate legacy MarshalSSZ/UnmarshalSSZ/HashTreeRoot methods\n")
		fmt.Fprintf(w, "  -with-streaming\n")
		fmt.Fprintf(w, "        Generate streaming encoder/decoder functions\n")
		fmt.Fprintf(w, "  -with-extended-types\n")
		fmt.Fprintf(w, "        Generate code with extended types\n")
		fmt.Fprintf(w, "  -without-dynamic-expressions\n")
		fmt.Fprintf(w, "        Generate code without dynamic expressions\n")
		fmt.Fprintf(w, "  -without-fastssz\n")
		fmt.Fprintf(w, "        Generate code without using fast ssz generated methods\n\n")
		fmt.Fprintf(w, "Other flags:\n")
		fmt.Fprintf(w, "  -v    Verbose output\n")
		fmt.Fprintf(w, "  -version\n")
		fmt.Fprintf(w, "        Print version and exit\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("dynssz-gen %s\n", getVersionString())
		return
	}

	if *packagePath == "" && *typeNames == "" {
		flag.Usage()
		return
	}

	config := Config{
		PackagePath:               *packagePath,
		PackageName:               *packageName,
		TypeNames:                 *typeNames,
		OutputFile:                *outputFile,
		Verbose:                   *verbose,
		Legacy:                    *legacy,
		WithoutDynamicExpressions: *withoutDynamicExpressions,
		WithoutFastSsz:            *withoutFastSsz,
		WithStreaming:             *withStreaming,
		WithExtendedTypes:         *withExtendedTypes,
	}

	if err := run(config); err != nil {
		log.Fatal(err)
	}
}

func run(config Config) error {
	if config.PackagePath == "" {
		return errors.New("package path is required (-package)")
	}
	if config.TypeNames == "" {
		return errors.New("type names are required (-types)")
	}

	if config.Verbose {
		log.Printf("Analyzing package: %s", config.PackagePath)
		log.Printf("Looking for types: %s", config.TypeNames)
	}

	// Parse the Go package
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, config.PackagePath)
	if err != nil {
		return fmt.Errorf("failed to load package %s: %v", config.PackagePath, err)
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found for %s", config.PackagePath)
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

	// Parse type names with extended format:
	// TypeName[:output=file.go][:views=View1;View2][:viewonly]
	// All :xy args are optional and can be specified in any order.
	// Output file can also be specified without prefix for backward compatibility.
	requestedTypes := strings.Split(config.TypeNames, ",")
	typeSpecs := make([]typeSpec, 0, len(requestedTypes))

	for _, typeStr := range requestedTypes {
		typeStr = strings.TrimSpace(typeStr)
		if typeStr == "" {
			continue
		}

		spec := typeSpec{}
		parts := strings.Split(typeStr, ":")

		// First part is always the type name
		spec.TypeName = parts[0]

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
				}
			case strings.HasPrefix(part, "output="):
				// Explicit output file with prefix
				spec.OutputFile = strings.TrimPrefix(part, "output=")
			case part == "viewonly":
				spec.IsViewOnly = true
			default:
				// For backward compatibility: first unrecognized non-empty part
				// is treated as output file (only if not already set)
				if spec.OutputFile == "" {
					spec.OutputFile = part
				}
			}
		}

		// Use default output file if not specified
		if spec.OutputFile == "" {
			if config.OutputFile == "" {
				return errors.New("output file is required (-output)")
			}
			spec.OutputFile = config.OutputFile
		}

		typeSpecs = append(typeSpecs, spec)
	}

	// Find the requested types in the package
	// Map from output file to list of type specs
	generateFiles := make(map[string][]typeSpec)
	mainScope := pkg.Types.Scope()
	typeCount := 0

	// Cache for loaded external packages
	externalPackages := make(map[string]*packages.Package)

	// Helper to load and cache an external package
	loadExternalPackage := func(pkgPath string) (*packages.Package, error) {
		if cached, ok := externalPackages[pkgPath]; ok {
			return cached, nil
		}

		extPkgs, err := packages.Load(cfg, pkgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load external package %s: %w", pkgPath, err)
		}
		if len(extPkgs) == 0 {
			return nil, fmt.Errorf("external package %s not found", pkgPath)
		}
		if len(extPkgs[0].Errors) > 0 {
			return nil, fmt.Errorf("external package %s has errors: %v", pkgPath, extPkgs[0].Errors[0])
		}

		externalPackages[pkgPath] = extPkgs[0]
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
			extPkg, err := loadExternalPackage(ref.PackagePath)
			if err != nil {
				return nil, err
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

	for _, spec := range typeSpecs {
		// Validate that the main type exists
		obj := mainScope.Lookup(spec.TypeName)
		if obj == nil {
			return fmt.Errorf("type %s not found in package %s", spec.TypeName, config.PackagePath)
		}

		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			return fmt.Errorf("object %s is not a type in package %s", spec.TypeName, config.PackagePath)
		}

		// Validate view types exist (can be local or external)
		for _, viewTypeStr := range spec.ViewTypes {
			ref := parseViewTypeRef(viewTypeStr)
			if _, err := resolveTypeRef(ref); err != nil {
				return fmt.Errorf("view type %s: %w", viewTypeStr, err)
			}
		}

		if _, ok := generateFiles[spec.OutputFile]; !ok {
			generateFiles[spec.OutputFile] = make([]typeSpec, 0)
		}
		generateFiles[spec.OutputFile] = append(generateFiles[spec.OutputFile], spec)
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

	if config.PackageName != "" {
		codeGen.SetPackageName(config.PackageName)
	}

	// Build options for all types
	for outFile, specs := range generateFiles {
		var typeOptions []codegen.CodeGeneratorOption

		for _, spec := range specs {
			// Look up the main type
			obj := mainScope.Lookup(spec.TypeName)
			typeObj := obj.(*types.TypeName)
			goType := typeObj.Type()

			// Build type-specific options
			var typeSpecificOpts []codegen.CodeGeneratorOption

			// Add view types if specified
			if len(spec.ViewTypes) > 0 {
				viewTypes := make([]types.Type, 0, len(spec.ViewTypes))
				for _, viewTypeStr := range spec.ViewTypes {
					ref := parseViewTypeRef(viewTypeStr)
					viewType, err := resolveTypeRef(ref)
					if err != nil {
						return fmt.Errorf("failed to resolve view type %s: %w", viewTypeStr, err)
					}
					viewTypes = append(viewTypes, viewType)
				}
				typeSpecificOpts = append(typeSpecificOpts, codegen.WithGoTypesViewTypes(viewTypes...))
			}

			// Add view-only flag if specified
			if spec.IsViewOnly {
				typeSpecificOpts = append(typeSpecificOpts, codegen.WithViewOnly())
			}

			// Add the type with its options
			typeOptions = append(typeOptions, codegen.WithGoTypesType(goType, typeSpecificOpts...))
		}

		if config.Legacy {
			typeOptions = append(typeOptions, codegen.WithCreateLegacyFn())
		}
		if config.WithoutDynamicExpressions {
			typeOptions = append(typeOptions, codegen.WithoutDynamicExpressions())
		}
		if config.WithoutFastSsz {
			typeOptions = append(typeOptions, codegen.WithNoFastSsz())
		}
		if config.WithStreaming {
			typeOptions = append(typeOptions, codegen.WithCreateEncoderFn())
			typeOptions = append(typeOptions, codegen.WithCreateDecoderFn())
		}
		if config.WithExtendedTypes {
			typeOptions = append(typeOptions, codegen.WithExtendedTypes())
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

	// Write to output file
	if config.Verbose {
		log.Printf("Writing output to %s", config.OutputFile)
	}

	codeSize := 0
	for outFile, generatedCode := range codeMap {
		codeSize += len(generatedCode)
		err = os.WriteFile(outFile, []byte(generatedCode), 0644)
		if err != nil {
			return fmt.Errorf("failed to write output file %s: %v", outFile, err)
		}
	}

	if config.Verbose {
		log.Printf("Successfully generated %d bytes of code for %d types to %d files", codeSize, typeCount, len(generateFiles))
	} else {
		fmt.Printf("Generated SSZ code for %d types to %d files\n", typeCount, len(generateFiles))
	}

	return nil
}
