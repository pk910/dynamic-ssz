// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package main implements the dynssz-gen command.
package main

import (
	"flag"
	"fmt"
	"go/types"
	"log"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/codegen"
)

func main() {
	var (
		packagePath               = flag.String("package", "", "Go package path to analyze")
		typeNames                 = flag.String("types", "", "Comma-separated list of type names to generate code for")
		outputFile                = flag.String("output", "", "Output file path for generated code")
		verbose                   = flag.Bool("v", false, "Verbose output")
		legacy                    = flag.Bool("legacy", false, "Generate legacy methods")
		withoutDynamicExpressions = flag.Bool("without-dynamic-expressions", false, "Generate code without dynamic expressions")
	)
	flag.Parse()

	if *packagePath == "" {
		log.Fatal("Package path is required (-package)")
	}
	if *typeNames == "" {
		log.Fatal("Type names are required (-types)")
	}

	if *verbose {
		log.Printf("Analyzing package: %s", *packagePath)
		log.Printf("Looking for types: %s", *typeNames)
	}

	// Parse the Go package
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedName,
	}

	pkgs, err := packages.Load(cfg, *packagePath)
	if err != nil {
		log.Fatalf("Failed to load package %s: %v", *packagePath, err)
	}

	if len(pkgs) == 0 {
		log.Fatalf("No packages found for %s", *packagePath)
	}

	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		for _, err := range pkg.Errors {
			log.Printf("Package error: %v", err)
		}
		log.Fatalf("Package %s has errors", *packagePath)
	}

	if *verbose {
		log.Printf("Successfully loaded package: %s", pkg.Name)
	}

	// Parse type names
	requestedTypes := strings.Split(*typeNames, ",")
	for i, typeName := range requestedTypes {
		requestedTypes[i] = strings.TrimSpace(typeName)
	}

	// Find the requested types in the package
	generateFiles := make(map[string][]types.Type)
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
			if *outputFile == "" {
				log.Fatal("Output file is required (-output)")
			}
			outFile = *outputFile
		}

		obj := scope.Lookup(typeName)
		if obj == nil {
			log.Fatalf("Type %s not found in package %s", typeName, *packagePath)
		}

		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			log.Fatalf("Object %s is not a type in package %s", typeName, *packagePath)
		}

		if _, ok := generateFiles[outFile]; !ok {
			generateFiles[outFile] = make([]types.Type, 0)
		}
		generateFiles[outFile] = append(generateFiles[outFile], typeObj.Type())
		typeCount++

		if *verbose {
			log.Printf("Found type: %s", typeName)
		}
	}

	// Create codegen instance
	codeGen := codegen.NewCodeGenerator(dynssz.NewDynSsz(nil))

	// Build options for all types
	for outFile, foundTypes := range generateFiles {
		var typeOptions []codegen.CodeGeneratorOption
		for _, goType := range foundTypes {
			typeOptions = append(typeOptions, codegen.WithGoTypesType(goType))
		}

		if *legacy {
			typeOptions = append(typeOptions, codegen.WithCreateLegacyFn())
		}
		if *withoutDynamicExpressions {
			typeOptions = append(typeOptions, codegen.WithoutDynamicExpressions())
		}

		// Build the file with all types
		codeGen.BuildFile(outFile, typeOptions...)
	}

	// Generate the code
	if *verbose {
		log.Printf("Generating code...")
	}

	codeMap, err := codeGen.GenerateToMap()
	if err != nil {
		log.Fatalf("Failed to generate code: %v", err)
	}

	// Write to output file
	if *verbose {
		log.Printf("Writing output to %s", *outputFile)
	}

	codeSize := 0
	for outFile, generatedCode := range codeMap {
		codeSize += len(generatedCode)
		err = os.WriteFile(outFile, []byte(generatedCode), 0644)
		if err != nil {
			log.Fatalf("Failed to write output file %s: %v", outFile, err)
		}
	}

	if *verbose {
		log.Printf("Successfully generated %d bytes of code for %d types to %d files", codeSize, typeCount, len(generateFiles))
	} else {
		fmt.Printf("Generated SSZ code for %d types to %d files\n", typeCount, len(generateFiles))
	}
}
