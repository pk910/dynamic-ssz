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

	// Parse type names
	requestedTypes := strings.Split(config.TypeNames, ",")
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

		if _, ok := generateFiles[outFile]; !ok {
			generateFiles[outFile] = make([]types.Type, 0)
		}
		generateFiles[outFile] = append(generateFiles[outFile], typeObj.Type())
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
	for outFile, foundTypes := range generateFiles {
		var typeOptions []codegen.CodeGeneratorOption
		for _, goType := range foundTypes {
			typeOptions = append(typeOptions, codegen.WithGoTypesType(goType))
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
