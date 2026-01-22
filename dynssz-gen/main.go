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
}

// typeSpec holds parsed information about a type specification
type typeSpec struct {
	TypeName   string
	OutputFile string
	ViewTypes  []string // view types for data+views mode
	IsViewOnly bool     // whether this is a view-only type
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
	)
	flag.Parse()

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
	// TypeName[:output.go][:views=View1,View2][:viewonly]
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

		// Process remaining parts
		for i := 1; i < len(parts); i++ {
			part := parts[i]
			if strings.HasPrefix(part, "views=") {
				// Parse view types: views=View1;View2 or views=View1,View2
				viewsStr := strings.TrimPrefix(part, "views=")
				// Use semicolon as separator since comma is used for type list
				spec.ViewTypes = strings.Split(viewsStr, ";")
				for j := range spec.ViewTypes {
					spec.ViewTypes[j] = strings.TrimSpace(spec.ViewTypes[j])
				}
			} else if part == "viewonly" {
				spec.IsViewOnly = true
			} else if spec.OutputFile == "" {
				// First non-special part after type name is the output file
				spec.OutputFile = part
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
	scope := pkg.Types.Scope()
	typeCount := 0

	for _, spec := range typeSpecs {
		// Validate that the main type exists
		obj := scope.Lookup(spec.TypeName)
		if obj == nil {
			return fmt.Errorf("type %s not found in package %s", spec.TypeName, config.PackagePath)
		}

		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			return fmt.Errorf("object %s is not a type in package %s", spec.TypeName, config.PackagePath)
		}

		// Validate view types exist
		for _, viewTypeName := range spec.ViewTypes {
			viewObj := scope.Lookup(viewTypeName)
			if viewObj == nil {
				return fmt.Errorf("view type %s not found in package %s", viewTypeName, config.PackagePath)
			}
			if _, ok := viewObj.(*types.TypeName); !ok {
				return fmt.Errorf("view object %s is not a type in package %s", viewTypeName, config.PackagePath)
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
			obj := scope.Lookup(spec.TypeName)
			typeObj := obj.(*types.TypeName)
			goType := typeObj.Type()

			// Build type-specific options
			var typeSpecificOpts []codegen.CodeGeneratorOption

			// Add view types if specified
			if len(spec.ViewTypes) > 0 {
				viewTypes := make([]types.Type, 0, len(spec.ViewTypes))
				for _, viewTypeName := range spec.ViewTypes {
					viewObj := scope.Lookup(viewTypeName)
					viewTypeObj := viewObj.(*types.TypeName)
					viewTypes = append(viewTypes, viewTypeObj.Type())
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
