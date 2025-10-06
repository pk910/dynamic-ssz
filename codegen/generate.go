// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

// analyzeTypes performs comprehensive type analysis and validation for all types in the generation request.
//
// This method is responsible for the critical pre-generation analysis phase, where all types
// are examined, validated, and prepared for code generation. It builds complete type descriptors,
// validates package consistency, and sets up compatibility flags for optimal code generation.
//
// The analysis process includes:
//   - Type name and package path resolution for both reflection and go/types inputs
//   - Package consistency validation (all types in a file must be from the same package)
//   - TypeDescriptor creation through either DynSsz type cache or Parser analysis
//   - SSZ compatibility flag assignment based on generation options
//   - Cross-reference setup for types that reference each other
//
// Type Analysis Modes:
//   - Reflection-based: Uses runtime type information via reflect.Type
//   - Compile-time: Uses go/types.Type for advanced static analysis
//   - Hybrid: Automatically selects the appropriate method based on available type information
//
// Validation and Error Conditions:
//   - Ensures all types have valid package paths
//   - Validates package consistency within each file
//   - Checks for SSZ compatibility and type constraint satisfaction
//   - Reports detailed errors for analysis failures
//
// Returns:
//   - error: An error if type analysis fails
//
// This method must be called before any code generation attempts, as it populates
// the essential type metadata that drives the entire generation process.
func (cg *CodeGenerator) analyzeTypes() error {
	var parser *Parser

	getTypeName := func(t *CodeGeneratorTypeOptions) (string, string) {
		var typeName, typePkgPath string
		if t.ReflectType != nil {
			typeName = t.ReflectType.Name()
			typePkgPath = t.ReflectType.PkgPath()
			if typePkgPath == "" && t.ReflectType.Kind() == reflect.Ptr {
				typePkgPath = t.ReflectType.Elem().PkgPath()
			}
		} else if t.GoTypesType != nil {
			typeName = t.GoTypesType.String()
			types.TypeString(t.GoTypesType, func(pkg *types.Package) string {
				typePkgPath = pkg.Path()
				return ""
			})
		}
		return typeName, typePkgPath
	}

	// add compat flags for generated types
	for _, file := range cg.files {
		for _, t := range file.Options.Types {
			typeKey, _ := getTypeName(t)

			var compatFlags dynssz.SszCompatFlag

			// set availability of dynamic methods (we will generate them in a bit and we want cross references)
			if !t.Options.NoMarshalSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= dynssz.SszCompatFlagDynamicMarshaler
			}
			if !t.Options.NoUnmarshalSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= dynssz.SszCompatFlagDynamicUnmarshaler
			}
			if !t.Options.NoSizeSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= dynssz.SszCompatFlagDynamicSizer
			}
			if !t.Options.NoHashTreeRoot && !t.Options.WithoutDynamicExpressions {
				compatFlags |= dynssz.SszCompatFlagDynamicHashRoot
			}

			if !t.Options.NoMarshalSSZ && !t.Options.NoUnmarshalSSZ && !t.Options.NoSizeSSZ && (t.Options.CreateLegacyFn || t.Options.WithoutDynamicExpressions) {
				compatFlags |= dynssz.SszCompatFlagFastSSZMarshaler
			}
			if !t.Options.NoHashTreeRoot && (t.Options.CreateLegacyFn || t.Options.WithoutDynamicExpressions) {
				if t.Options.CreateLegacyFn {
					compatFlags |= dynssz.SszCompatFlagFastSSZHasher
				}
				compatFlags |= dynssz.SszCompatFlagHashTreeRootWith
			}

			cg.compatFlags[typeKey] = compatFlags
		}
	}

	cg.dynSsz.GetTypeCache().CompatFlags = cg.compatFlags

	// analyze all types to build complete dependency graph
	for _, file := range cg.files {
		pkgPath := ""
		otherTypeName := ""
		for _, t := range file.Options.Types {
			typeName, typePkgPath := getTypeName(t)

			if typePkgPath == "" {
				return fmt.Errorf("type %s has no package path", typeName)
			}
			if pkgPath == "" {
				pkgPath = typePkgPath
			} else if pkgPath != typePkgPath {
				return fmt.Errorf("type %s has different package path than %s. cannot combine types from different packages in a single file", typeName, otherTypeName)
			}

			otherTypeName = typeName
			t.TypeName = typeName

			var desc *dynssz.TypeDescriptor
			var err error
			if t.ReflectType != nil {
				if t.ReflectType.Kind() == reflect.Struct {
					t.ReflectType = reflect.PointerTo(t.ReflectType)
				}
				desc, err = cg.dynSsz.GetTypeCache().GetTypeDescriptor(t.ReflectType, t.Options.SizeHints, t.Options.MaxSizeHints, t.Options.TypeHints)
			} else {
				if parser == nil {
					parser = NewParser()
					parser.CompatFlags = cg.compatFlags
				}
				baseType := t.GoTypesType
				if named, ok := baseType.(*types.Named); ok {
					baseType = named.Underlying()
				}
				if _, ok := baseType.(*types.Struct); ok {
					t.GoTypesType = types.NewPointer(t.GoTypesType)
				}
				desc, err = parser.GetTypeDescriptor(t.GoTypesType, t.Options.TypeHints, t.Options.SizeHints, t.Options.MaxSizeHints)
			}

			if err != nil {
				return fmt.Errorf("failed to analyze type %s: %w", typeName, err)
			}

			t.Descriptor = desc
		}

		file.Options.Package = pkgPath

		pkgName := pkgPath
		if cg.packageName != "" {
			pkgName = cg.packageName
		} else if slashIdx := strings.LastIndex(pkgName, "/"); slashIdx != -1 {
			pkgName = pkgName[slashIdx+1:]
		}
		file.Options.PackageName = pkgName
	}

	return nil
}

// generateFile creates the complete Go source code for a single file.
//
// This internal method handles the generation of one complete Go source file,
// including package declaration, imports, and all requested SSZ methods for
// the specified types. It manages import organization, code formatting,
// and ensures the generated code is valid Go.
//
// Parameters:
//   - fileName: The target file name (used for metadata in generated code)
//   - packagePath: The Go package path for the generated file
//   - opts: Complete file generation options with resolved type configurations
//
// Returns:
//   - string: The complete generated Go source code
//   - error: An error if generation fails due to code generation issues
//
// The generated file includes:
//   - File header with generation metadata and version information
//   - Package declaration
//   - Organized import statements
//   - Variable declarations for error handling
//   - Generated SSZ methods for all specified types
func (cg *CodeGenerator) generateFile(packagePath string, opts *CodeGeneratorFileOptions) (string, error) {
	if len(opts.Types) == 0 {
		return "", fmt.Errorf("no types requested for generation")
	}

	typePrinter := NewTypePrinter(packagePath)
	typePrinter.AddAlias("github.com/pk910/dynamic-ssz", "dynssz")
	codeBuilder := strings.Builder{}
	hashParts := [][]byte{}

	for _, t := range opts.Types {
		if t.Descriptor == nil {
			return "", fmt.Errorf("type %s has no descriptor", t.TypeName)
		}

		hash, err := t.Descriptor.GetTypeHash()
		if err != nil {
			return "", fmt.Errorf("failed to get type hash for %s: %w", t.TypeName, err)
		}
		hashParts = append(hashParts, hash[:])

		err = cg.generateCode(t.Descriptor, typePrinter, &codeBuilder, &t.Options)
		if err != nil {
			return "", fmt.Errorf("failed to generate code for %s: %w", t.TypeName, err)
		}
	}

	typesHash := sha256.Sum256(bytes.Join(hashParts, []byte{}))
	typePrinter.AddImport("github.com/pk910/dynamic-ssz/sszutils", "sszutils")

	// collect & sort imports
	importsMap := typePrinter.Imports()
	imports := make([]TypeImport, 0, len(importsMap))
	for path, alias := range importsMap {
		if presetAlias := typePrinter.Aliases()[path]; presetAlias != "" {
			alias = presetAlias
		} else if defaultAlias := typePrinter.defaultAlias(path); alias == defaultAlias {
			alias = ""
		}
		imports = append(imports, TypeImport{
			Alias: alias,
			Path:  path,
		})
	}

	sort.Slice(imports, func(i, j int) bool {
		return imports[i].Path < imports[j].Path
	})

	// Build the file content directly
	mainCodeBuilder := strings.Builder{}

	// File header
	mainCodeBuilder.WriteString("// Code generated by dynamic-ssz. DO NOT EDIT.\n")
	mainCodeBuilder.WriteString(fmt.Sprintf("// Hash: %s\n", hex.EncodeToString(typesHash[:])))
	mainCodeBuilder.WriteString(fmt.Sprintf("// Version: %s (https://github.com/pk910/dynamic-ssz)\n", Version))
	mainCodeBuilder.WriteString(fmt.Sprintf("package %s\n\n", opts.PackageName))

	// Imports
	if len(imports) > 0 {
		mainCodeBuilder.WriteString("import (\n")
		for _, imp := range imports {
			if imp.Alias != "" {
				mainCodeBuilder.WriteString(fmt.Sprintf("\t%s \"%s\"\n", imp.Alias, imp.Path))
			} else {
				mainCodeBuilder.WriteString(fmt.Sprintf("\t\"%s\"\n", imp.Path))
			}
		}
		mainCodeBuilder.WriteString(")\n\n")
	}

	// Variable declarations
	mainCodeBuilder.WriteString("var _ = sszutils.ErrListTooBig\n\n")

	// Generated code
	mainCodeBuilder.WriteString(codeBuilder.String())

	return mainCodeBuilder.String(), nil
}

// generateCode generates all SSZ methods for a single type.
//
// This internal method orchestrates the generation of all requested SSZ methods
// for a specific type. It delegates to specialized generators for each method type
// and tracks whether dynamic SSZ functionality is used.
//
// The method generates methods in a specific order to ensure proper dependencies:
//  1. Marshal methods (MarshalSSZTo, MarshalSSZDyn)
//  2. Size methods (SizeSSZ, SizeSSZDyn)
//  3. Unmarshal methods (UnmarshalSSZ, UnmarshalSSZDyn)
//  4. Hash tree root methods (HashTreeRoot, HashTreeRootDyn)
//
// Parameters:
//   - desc: Type descriptor containing all metadata for SSZ code generation
//   - typePrinter: Type string formatter for import and alias management
//   - codeBuilder: String builder to append generated code to
//   - options: Generation options controlling which methods to generate
//
// Returns:
//   - bool: True if any generated code uses dynamic SSZ functionality
//   - error: An error if any method generation fails
func (cg *CodeGenerator) generateCode(desc *dynssz.TypeDescriptor, typePrinter *TypePrinter, codeBuilder *strings.Builder, options *CodeGeneratorOptions) error {
	// Generate the actual methods using flattened generators
	var err error

	if !options.NoMarshalSSZ {
		err = generateMarshal(desc, codeBuilder, typePrinter, options)
		if err != nil {
			return fmt.Errorf("failed to generate marshal for %s: %w", desc.Type.Name(), err)
		}
	}

	if !options.NoSizeSSZ {
		err = generateSize(desc, codeBuilder, typePrinter, options)
		if err != nil {
			return fmt.Errorf("failed to generate size for %s: %w", desc.Type.Name(), err)
		}
	}

	if !options.NoUnmarshalSSZ {
		err = generateUnmarshal(desc, codeBuilder, typePrinter, options)
		if err != nil {
			return fmt.Errorf("failed to generate unmarshal for %s: %w", desc.Type.Name(), err)
		}
	}

	if !options.NoHashTreeRoot {
		err = generateHashTreeRoot(desc, codeBuilder, typePrinter, options)
		if err != nil {
			return fmt.Errorf("failed to generate hash tree root for %s: %w", desc.Type.Name(), err)
		}
	}

	return nil
}
