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
	"log"
	"os"
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
	Verbose                   bool
	Legacy                    bool
	WithoutDynamicExpressions bool
	WithoutFastSsz            bool
	WithStreaming             bool
	WithExtendedTypes         bool
}

// typeEntry holds a type and its per-type codegen options.
type typeEntry struct {
	goType types.Type
	opts   []codegen.CodeGeneratorOption
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
		_, _ = fmt.Fprintf(w, "dynssz-gen %s\n\n", getVersionString())
		_, _ = fmt.Fprintf(w, "Go code generator for dynamic SSZ marshaling, unmarshaling, and hash tree root.\n\n")
		_, _ = fmt.Fprintf(w, "Usage:\n")
		_, _ = fmt.Fprintf(w, "  dynssz-gen -package <path> -types <types> [-output <file>] [flags]\n\n")
		_, _ = fmt.Fprintf(w, "Types syntax:\n")
		_, _ = fmt.Fprintf(w, "  Comma-separated list of type names from the target package.\n")
		_, _ = fmt.Fprintf(w, "  Each type can optionally specify its output file with a colon:\n")
		_, _ = fmt.Fprintf(w, "    TypeName              uses the -output file\n")
		_, _ = fmt.Fprintf(w, "    TypeName:path/out.go  writes to a specific file\n\n")
		_, _ = fmt.Fprintf(w, "  Example: -types \"BeaconState:state_gen.go,BeaconBlock:block_gen.go\"\n\n")
		_, _ = fmt.Fprintf(w, "Required flags:\n")
		_, _ = fmt.Fprintf(w, "  -package string\n")
		_, _ = fmt.Fprintf(w, "        Go package path to analyze\n")
		_, _ = fmt.Fprintf(w, "  -types string\n")
		_, _ = fmt.Fprintf(w, "        Comma-separated list of type names to generate code for\n\n")
		_, _ = fmt.Fprintf(w, "Output flags:\n")
		_, _ = fmt.Fprintf(w, "  -output string\n")
		_, _ = fmt.Fprintf(w, "        Default output file path (used for types without a ':path' suffix)\n")
		_, _ = fmt.Fprintf(w, "  -package-name string\n")
		_, _ = fmt.Fprintf(w, "        Package name for generated code (default: same as source package)\n\n")
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

	// Parse type names
	requestedTypes := strings.Split(config.TypeNames, ",")
	for i, typeName := range requestedTypes {
		requestedTypes[i] = strings.TrimSpace(typeName)
	}

	// Find the requested types in the package
	generateFiles := make(map[string][]typeEntry)
	scope := pkg.Types.Scope()
	typeCount := 0

	for _, typeName := range requestedTypes {
		outFile := ""
		outFileParts := strings.Split(typeName, ":")
		if len(outFileParts) > 1 {
			outFile = outFileParts[1]
			typeName = outFileParts[0]
		}

		if outFile == "" {
			if config.OutputFile == "" {
				return errors.New("output file is required (-output)")
			}
			outFile = config.OutputFile
		}

		obj := scope.Lookup(typeName)
		if obj == nil {
			return fmt.Errorf("type %s not found in package %s", typeName, config.PackagePath)
		}

		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			return fmt.Errorf("object %s is not a type in package %s", typeName, config.PackagePath)
		}

		// Parse SSZ annotations from sszutils.Annotate[T]() calls in source
		var perTypeOpts []codegen.CodeGeneratorOption
		if tag := findAnnotateCall(pkg, typeName); tag != "" {
			annotateOpts, parseErr := parseAnnotateTag(tag)
			if parseErr != nil {
				return fmt.Errorf("failed to parse Annotate tag for type %s: %v", typeName, parseErr)
			}

			perTypeOpts = annotateOpts

			if config.Verbose && len(annotateOpts) > 0 {
				log.Printf("Found Annotate call for type %s", typeName)
			}
		}

		generateFiles[outFile] = append(generateFiles[outFile], typeEntry{
			goType: typeObj.Type(),
			opts:   perTypeOpts,
		})
		typeCount++

		if config.Verbose {
			log.Printf("Found type: %s", typeName)
		}
	}

	// Create codegen instance
	typeCache := ssztypes.NewTypeCache(nil)
	codeGen := codegen.NewCodeGenerator(typeCache)

	if config.PackageName != "" {
		codeGen.SetPackageName(config.PackageName)
	}

	// Build options for all types
	for outFile, entries := range generateFiles {
		var typeOptions []codegen.CodeGeneratorOption
		for _, entry := range entries {
			typeOptions = append(typeOptions, codegen.WithGoTypesType(entry.goType, entry.opts...))
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
			typeOptions = append(typeOptions, codegen.WithCreateEncoderFn(), codegen.WithCreateDecoderFn())
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
		err = os.WriteFile(outFile, []byte(generatedCode), 0o600)
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

// findAnnotateCall scans package AST for sszutils.Annotate[typeName]("...")
// calls and returns the tag string literal, or "" if not found.
func findAnnotateCall(pkg *packages.Package, typeName string) string {
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
			tag := findAnnotateCallInDecl(decl, sszutilsAlias, typeName)
			if tag != "" {
				return tag
			}
		}
	}

	return ""
}

// findAnnotateCallInDecl checks a single declaration for an Annotate call.
func findAnnotateCallInDecl(decl ast.Decl, alias, typeName string) string {
	switch d := decl.(type) {
	case *ast.GenDecl:
		if d.Tok != token.VAR {
			return ""
		}

		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for _, val := range vs.Values {
				if tag := matchAnnotateCall(val, alias, typeName); tag != "" {
					return tag
				}
			}
		}
	case *ast.FuncDecl:
		// Check init() functions
		if d.Name.Name != "init" || d.Body == nil {
			return ""
		}

		for _, stmt := range d.Body.List {
			exprStmt, ok := stmt.(*ast.ExprStmt)
			if !ok {
				continue
			}

			if tag := matchAnnotateCall(exprStmt.X, alias, typeName); tag != "" {
				return tag
			}
		}
	}

	return ""
}

// matchAnnotateCall checks if an expression is sszutils.Annotate[typeName]("...tag...")
// and returns the tag string, or "" if it doesn't match.
func matchAnnotateCall(expr ast.Expr, alias, typeName string) string {
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
	typeIdent, ok := indexExpr.Index.(*ast.Ident)
	if !ok || typeIdent.Name != typeName {
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
