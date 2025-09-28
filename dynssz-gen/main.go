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
		packagePath = flag.String("package", "", "Go package path to analyze")
		typeNames   = flag.String("types", "", "Comma-separated list of type names to generate code for")
		outputFile  = flag.String("output", "", "Output file path for generated code")
		verbose     = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	if *packagePath == "" {
		log.Fatal("Package path is required (-package)")
	}
	if *typeNames == "" {
		log.Fatal("Type names are required (-types)")
	}
	if *outputFile == "" {
		log.Fatal("Output file is required (-output)")
	}

	if *verbose {
		log.Printf("Analyzing package: %s", *packagePath)
		log.Printf("Looking for types: %s", *typeNames)
		log.Printf("Output file: %s", *outputFile)
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
	foundTypes := make(map[string]types.Type)
	scope := pkg.Types.Scope()

	for _, typeName := range requestedTypes {
		obj := scope.Lookup(typeName)
		if obj == nil {
			log.Fatalf("Type %s not found in package %s", typeName, *packagePath)
		}

		typeObj, ok := obj.(*types.TypeName)
		if !ok {
			log.Fatalf("Object %s is not a type in package %s", typeName, *packagePath)
		}

		foundTypes[typeName] = typeObj.Type()
		if *verbose {
			log.Printf("Found type: %s", typeName)
		}
	}

	// Create codegen instance
	codeGen := codegen.NewCodeGenerator(dynssz.NewDynSsz(nil))

	// Build options for all types
	var typeOptions []codegen.CodeGeneratorOption
	for _, goType := range foundTypes {
		typeOptions = append(typeOptions, codegen.WithGoTypesType(goType))
	}

	typeOptions = append(typeOptions, codegen.WithCreateLegacyFn())

	// Build the file with all types
	codeGen.BuildFile(*outputFile, typeOptions...)

	// Generate the code
	if *verbose {
		log.Printf("Generating code...")
	}

	codeMap, err := codeGen.GenerateToMap()
	if err != nil {
		log.Fatalf("Failed to generate code: %v", err)
	}

	// Get the generated code for our file
	generatedCode, exists := codeMap[*outputFile]
	if !exists {
		log.Fatalf("Generated code not found for file %s", *outputFile)
	}

	// Write to output file
	if *verbose {
		log.Printf("Writing output to %s", *outputFile)
	}

	err = os.WriteFile(*outputFile, []byte(generatedCode), 0644)
	if err != nil {
		log.Fatalf("Failed to write output file %s: %v", *outputFile, err)
	}

	if *verbose {
		log.Printf("Successfully generated %d bytes of code for %d types", len(generatedCode), len(foundTypes))
	} else {
		fmt.Printf("Generated SSZ code for %d types in %s\n", len(foundTypes), *outputFile)
	}
}
