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
	"regexp"
	"sort"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
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

	hasViews := func(t *CodeGeneratorTypeOptions) bool {
		return len(t.ViewGoTypesTypes) > 0 || len(t.ViewReflectTypes) > 0
	}

	getCompatFlags := func(t *CodeGeneratorTypeOptions) ssztypes.SszCompatFlag {
		var compatFlags ssztypes.SszCompatFlag

		// For view-only types, only set view compat flags
		if t.IsViewOnly {
			// View-only mode: only generate view methods
			if !t.Options.NoMarshalSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicViewMarshaler
			}
			if !t.Options.NoUnmarshalSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicViewUnmarshaler
			}
			if !t.Options.NoSizeSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicViewSizer
			}
			if !t.Options.NoHashTreeRoot && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicViewHashRoot
			}
			if t.Options.CreateEncoderFn {
				compatFlags |= ssztypes.SszCompatFlagDynamicViewEncoder
			}
			if t.Options.CreateDecoderFn {
				compatFlags |= ssztypes.SszCompatFlagDynamicViewDecoder
			}
		} else {
			// Data-only or data+views mode: set data compat flags
			if !t.Options.NoMarshalSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicMarshaler
			}
			if !t.Options.NoUnmarshalSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicUnmarshaler
			}
			if !t.Options.NoSizeSSZ && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicSizer
			}
			if !t.Options.NoHashTreeRoot && !t.Options.WithoutDynamicExpressions {
				compatFlags |= ssztypes.SszCompatFlagDynamicHashRoot
			}
			if t.Options.CreateEncoderFn {
				compatFlags |= ssztypes.SszCompatFlagDynamicEncoder
			}
			if t.Options.CreateDecoderFn {
				compatFlags |= ssztypes.SszCompatFlagDynamicDecoder
			}

			if !t.Options.NoMarshalSSZ && !t.Options.NoUnmarshalSSZ && !t.Options.NoSizeSSZ && (t.Options.CreateLegacyFn || t.Options.WithoutDynamicExpressions) {
				compatFlags |= ssztypes.SszCompatFlagFastSSZMarshaler
			}
			if !t.Options.NoHashTreeRoot && (t.Options.CreateLegacyFn || t.Options.WithoutDynamicExpressions) {
				if t.Options.CreateLegacyFn {
					compatFlags |= ssztypes.SszCompatFlagFastSSZHasher
				}
				compatFlags |= ssztypes.SszCompatFlagHashTreeRootWith
			}

			// Data+views mode: also set view compat flags for the data type
			if hasViews(t) {
				if !t.Options.NoMarshalSSZ && !t.Options.WithoutDynamicExpressions {
					compatFlags |= ssztypes.SszCompatFlagDynamicViewMarshaler
				}
				if !t.Options.NoUnmarshalSSZ && !t.Options.WithoutDynamicExpressions {
					compatFlags |= ssztypes.SszCompatFlagDynamicViewUnmarshaler
				}
				if !t.Options.NoSizeSSZ && !t.Options.WithoutDynamicExpressions {
					compatFlags |= ssztypes.SszCompatFlagDynamicViewSizer
				}
				if !t.Options.NoHashTreeRoot && !t.Options.WithoutDynamicExpressions {
					compatFlags |= ssztypes.SszCompatFlagDynamicViewHashRoot
				}
				if t.Options.CreateEncoderFn {
					compatFlags |= ssztypes.SszCompatFlagDynamicViewEncoder
				}
				if t.Options.CreateDecoderFn {
					compatFlags |= ssztypes.SszCompatFlagDynamicViewDecoder
				}
			}
		}

		return compatFlags
	}

	// add compat flags for generated types
	for _, file := range cg.files {
		for _, t := range file.Options.Types {
			typeKey := getFullTypeName(t.GoTypesType, t.ReflectType)
			compatFlags := getCompatFlags(t)

			cg.compatFlags[typeKey] = compatFlags
			if t.GoTypesType != nil {
				cg.aliases.AddGoTypesAliasFlags(t.GoTypesType, compatFlags)
			} else if t.ReflectType != nil {
				cg.aliases.AddReflectAliasFlags(t.ReflectType, compatFlags)
			}

			// Also set compat flags for view types (they will have view methods available)
			for _, viewType := range t.ViewGoTypesTypes {
				viewKey := fmt.Sprintf("%v|%v", typeKey, getFullTypeName(viewType, nil))
				cg.compatFlags[viewKey] = compatFlags
			}

			for _, viewType := range t.ViewReflectTypes {
				viewKey := fmt.Sprintf("%v|%v", typeKey, getFullTypeName(nil, viewType))
				cg.compatFlags[viewKey] = compatFlags
			}
		}
	}

	cg.typeCache.CompatFlags = cg.compatFlags

	// analyze all types to build complete dependency graph
	for _, file := range cg.files {
		pkgPath := ""
		otherTypeName := ""
		for _, t := range file.Options.Types {
			typeName, typePkgPath := getTypeName(t.GoTypesType, t.ReflectType)

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

			// create TypeDescriptor for the data type
			var desc *ssztypes.TypeDescriptor
			var err error

			if t.ReflectType != nil {
				if t.ReflectType.Kind() == reflect.Struct {
					t.ReflectType = reflect.PointerTo(t.ReflectType)
				}
				desc, err = cg.typeCache.GetTypeDescriptor(t.ReflectType, t.Options.SizeHints, t.Options.MaxSizeHints, t.Options.TypeHints)
			} else {
				if parser == nil {
					parser = NewParser()
					parser.CompatFlags = cg.compatFlags
				}
				baseType := t.GoTypesType
				if alias, ok := baseType.(*types.Alias); ok {
					baseType = alias.Underlying()
				} else if named, ok := baseType.(*types.Named); ok {
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

			// create TypeDescriptor for the view types
			if hasViews(t) {
				for _, viewType := range t.ViewReflectTypes {
					if viewType.Kind() == reflect.Struct {
						viewType = reflect.PointerTo(viewType)
					}
					viewDesc, err := cg.typeCache.GetTypeDescriptorWithSchema(t.ReflectType, viewType, t.Options.SizeHints, t.Options.MaxSizeHints, t.Options.TypeHints)
					if err != nil {
						return fmt.Errorf("failed to analyze view type %s: %w", viewType.String(), err)
					}
					t.ViewDescriptors = append(t.ViewDescriptors, viewDesc)
				}
				for _, viewType := range t.ViewGoTypesTypes {
					if parser == nil {
						parser = NewParser()
						parser.CompatFlags = cg.compatFlags
					}
					baseType := viewType
					if named, ok := baseType.(*types.Named); ok {
						baseType = named.Underlying()
					}
					if _, ok := baseType.(*types.Struct); ok {
						viewType = types.NewPointer(viewType)
					}
					viewDesc, err := parser.GetTypeDescriptorWithSchema(t.GoTypesType, viewType, t.Options.TypeHints, t.Options.SizeHints, t.Options.MaxSizeHints)
					if err != nil {
						return fmt.Errorf("failed to analyze view type %s: %w", viewType.String(), err)
					}
					t.ViewDescriptors = append(t.ViewDescriptors, viewDesc)
				}
			}
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
		if t.Descriptor == nil || (t.IsViewOnly && len(t.ViewDescriptors) == 0) {
			return "", fmt.Errorf("type %s has no descriptor or view descriptors", t.TypeName)
		}

		if !t.IsViewOnly {
			hash, err := t.Descriptor.GetTypeHash()
			if err != nil {
				return "", fmt.Errorf("failed to get type hash for %s: %w", t.TypeName, err)
			}
			hashParts = append(hashParts, hash[:])

			err = cg.generateSSZMethods(t.Descriptor, typePrinter, &codeBuilder, "", &t.Options)
			if err != nil {
				return "", fmt.Errorf("failed to generate code for %s: %w", t.TypeName, err)
			}
		}

		if len(t.ViewDescriptors) > 0 {
			for _, viewDesc := range t.ViewDescriptors {
				hash, err := viewDesc.GetTypeHash()
				if err != nil {
					return "", fmt.Errorf("failed to get view hash for %s: %w", t.TypeName, err)
				}
				hashParts = append(hashParts, hash[:])

			}

			err := cg.generateSSZViewMethods(t.Descriptor, t.ViewDescriptors, typePrinter, &codeBuilder, &t.Options)
			if err != nil {
				return "", fmt.Errorf("failed to generate code for view types of %s: %w", t.TypeName, err)
			}
		}
	}

	typesHash := sha256.Sum256(bytes.Join(hashParts, []byte{}))
	typePrinter.AddImport("github.com/pk910/dynamic-ssz/sszutils", "sszutils")

	// collect & sort imports
	importsMap := typePrinter.Imports()

	sysImports := make([]TypeImport, 0, len(importsMap))
	pkgImports := make([]TypeImport, 0, len(importsMap))
	for path, alias := range importsMap {
		if presetAlias := typePrinter.Aliases()[path]; presetAlias != "" {
			alias = presetAlias
		} else if defaultAlias := typePrinter.defaultAlias(path); alias == defaultAlias {
			alias = ""
		}

		if regexp.MustCompile(`^[^/]+\.[a-zA-Z]+/.*$`).MatchString(path) {
			pkgImports = append(pkgImports, TypeImport{
				Alias: alias,
				Path:  path,
			})
		} else {
			sysImports = append(sysImports, TypeImport{
				Alias: alias,
				Path:  path,
			})
		}
	}

	sort.Slice(sysImports, func(i, j int) bool {
		return sysImports[i].Path < sysImports[j].Path
	})

	sort.Slice(pkgImports, func(i, j int) bool {
		return pkgImports[i].Path < pkgImports[j].Path
	})

	// Build the file content directly
	mainCodeBuilder := strings.Builder{}

	// File header
	mainCodeBuilder.WriteString("// Code generated by dynamic-ssz. DO NOT EDIT.\n")
	mainCodeBuilder.WriteString(fmt.Sprintf("// Hash: %s\n", hex.EncodeToString(typesHash[:])))
	mainCodeBuilder.WriteString(fmt.Sprintf("// Version: v%s (https://github.com/pk910/dynamic-ssz)\n", Version))
	mainCodeBuilder.WriteString(fmt.Sprintf("package %s\n\n", opts.PackageName))

	// Imports
	if len(sysImports) > 0 || len(pkgImports) > 0 {
		mainCodeBuilder.WriteString("import (\n")
		for _, imp := range sysImports {
			if imp.Alias != "" {
				mainCodeBuilder.WriteString(fmt.Sprintf("\t%s \"%s\"\n", imp.Alias, imp.Path))
			} else {
				mainCodeBuilder.WriteString(fmt.Sprintf("\t\"%s\"\n", imp.Path))
			}
		}

		if len(sysImports) > 0 && len(pkgImports) > 0 {
			mainCodeBuilder.WriteString("\n")
		}

		for _, imp := range pkgImports {
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

// generateSSZMethods generates all SSZ methods for a single type (data or view).
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
//   - viewName: Name of the view type for function name postfix (empty string for data type)
//   - options: Generation options controlling which methods to generate
//
// Returns:
//   - bool: True if any generated code uses dynamic SSZ functionality
//   - error: An error if any method generation fails
func (cg *CodeGenerator) generateSSZMethods(desc *ssztypes.TypeDescriptor, typePrinter *TypePrinter, codeBuilder *strings.Builder, viewName string, options *CodeGeneratorOptions) error {
	// Generate the actual methods using flattened generators
	var err error

	if !options.NoMarshalSSZ {
		err = generateMarshal(desc, codeBuilder, typePrinter, viewName, options, cg.aliases)
		if err != nil {
			return fmt.Errorf("failed to generate marshal for %s: %w", desc.Type.Name(), err)
		}
	}

	if options.CreateEncoderFn {
		err = generateEncoder(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate encoder for %s: %w", desc.Type.Name(), err)
		}
	}

	if !options.NoUnmarshalSSZ {
		err = generateUnmarshal(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate unmarshal for %s: %w", desc.Type.Name(), err)
		}
	}

	if options.CreateDecoderFn {
		err = generateDecoder(desc, codeBuilder, typePrinter, viewName, options, cg.aliases)
		if err != nil {
			return fmt.Errorf("failed to generate decoder for %s: %w", desc.Type.Name(), err)
		}
	}

	if !options.NoSizeSSZ {
		err = generateSize(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate size for %s: %w", desc.Type.Name(), err)
		}
	}

	if !options.NoHashTreeRoot {
		err = generateHashTreeRoot(desc, codeBuilder, typePrinter, viewName, options, cg.aliases)
		if err != nil {
			return fmt.Errorf("failed to generate hash tree root for %s: %w", desc.Type.Name(), err)
		}
	}

	return nil
}

func (cg *CodeGenerator) generateSSZViewMethods(dataType *ssztypes.TypeDescriptor, views []*ssztypes.TypeDescriptor, typePrinter *TypePrinter, codeBuilder *strings.Builder, options *CodeGeneratorOptions) error {
	// Generate the actual methods using flattened generators
	var err error

	viewFnNameMap := make(map[*ssztypes.TypeDescriptor]string)

	getViewFnName := func(desc *ssztypes.TypeDescriptor) string {
		if fnName, ok := viewFnNameMap[desc]; ok {
			return fnName
		}

		typeName := typePrinter.TypeStringWithoutTracking(desc, true)
		if idx := strings.Index(typeName, "."); idx != -1 {
			typeName = typeName[idx+1:]
		}
		typeName = strings.ReplaceAll(typeName, "-", "_")
		typeName = strings.ReplaceAll(typeName, "*", "")

		fnName := typeName
		counter := 0
		for {
			exists := false
			for _, existingName := range viewFnNameMap {
				if existingName == fnName {
					exists = true
					break
				}
			}
			if !exists {
				break
			}
			fnName = fmt.Sprintf("%s_%d", typeName, counter)
			counter++
		}

		viewFnNameMap[desc] = fnName
		return fnName
	}

	buildViewDispatcher := func(fnPrefix string, mainFn func() string) {
		appendCode(codeBuilder, 1, "switch view.(type) {\n")

		if !options.ViewOnly {
			mainFnName := mainFn()
			if mainFnName != "" {
				appendCode(codeBuilder, 1, "case nil, %s:\n", typePrinter.TypeString(dataType))
				appendCode(codeBuilder, 2, "return %s\n", mainFnName)
			}
		}

		for _, view := range views {
			typeName := typePrinter.ViewTypeString(view)
			viewFnName := getViewFnName(view)
			appendCode(codeBuilder, 1, "case %s:\n", typeName)
			appendCode(codeBuilder, 2, "return t.%s_%s\n", fnPrefix, viewFnName)

		}
		appendCode(codeBuilder, 1, "}\n")
	}

	if !options.NoMarshalSSZ {
		appendCode(codeBuilder, 0, "func (t %s) MarshalSSZDynView(view any) func(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {\n", typePrinter.TypeString(dataType))
		buildViewDispatcher("marshalSSZView", func() string {
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagDynamicMarshaler != 0 {
				return "t.MarshalSSZDyn"
			}
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 {
				return "func(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {\n\treturn t.MarshalSSZTo(buf)\n\t}"
			}
			return ""
		})
		appendCode(codeBuilder, 1, "return nil\n")
		appendCode(codeBuilder, 0, "}\n")

		for _, desc := range views {
			viewName := getViewFnName(desc)
			err = generateMarshal(desc, codeBuilder, typePrinter, viewName, options, cg.aliases)
			if err != nil {
				return fmt.Errorf("failed to generate marshal for %s: %w", desc.Type.Name(), err)
			}
		}
	}

	if options.CreateEncoderFn {
		appendCode(codeBuilder, 0, "func (t %s) MarshalSSZEncoderView(view any) func(ds sszutils.DynamicSpecs, enc sszutils.Encoder) error {\n", typePrinter.TypeString(dataType))
		buildViewDispatcher("marshalSSZEncoderView", func() string {
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagDynamicEncoder != 0 {
				return "t.MarshalSSZEncoder"
			}
			return ""
		})
		appendCode(codeBuilder, 1, "return nil\n")
		appendCode(codeBuilder, 0, "}\n")

		for _, desc := range views {
			viewName := getViewFnName(desc)
			err = generateEncoder(desc, codeBuilder, typePrinter, viewName, options)
			if err != nil {
				return fmt.Errorf("failed to generate encoder for %s: %w", desc.Type.Name(), err)
			}
		}
	}

	if !options.NoUnmarshalSSZ {
		appendCode(codeBuilder, 0, "func (t %s) UnmarshalSSZDynView(view any) func(ds sszutils.DynamicSpecs, buf []byte) error {\n", typePrinter.TypeString(dataType))
		buildViewDispatcher("unmarshalSSZView", func() string {
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagDynamicUnmarshaler != 0 {
				return "t.UnmarshalSSZDyn"
			}
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 {
				return "func(_ sszutils.DynamicSpecs, buf []byte) error {\n\treturn t.UnmarshalSSZ(buf)\n\t}"
			}
			return ""
		})
		appendCode(codeBuilder, 1, "return nil\n")
		appendCode(codeBuilder, 0, "}\n")

		for _, desc := range views {
			viewName := getViewFnName(desc)
			err = generateUnmarshal(desc, codeBuilder, typePrinter, viewName, options)
			if err != nil {
				return fmt.Errorf("failed to generate unmarshal for %s: %w", desc.Type.Name(), err)
			}
		}
	}

	if options.CreateDecoderFn {
		appendCode(codeBuilder, 0, "func (t %s) UnmarshalSSZDecoderView(view any) func(ds sszutils.DynamicSpecs, dec sszutils.Decoder) error {\n", typePrinter.TypeString(dataType))
		buildViewDispatcher("unmarshalSSZDecoderView", func() string {
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagDynamicDecoder != 0 {
				return "t.UnmarshalSSZDecoder"
			}
			return ""
		})
		appendCode(codeBuilder, 1, "return nil\n")
		appendCode(codeBuilder, 0, "}\n")

		for _, desc := range views {
			viewName := getViewFnName(desc)
			err = generateDecoder(desc, codeBuilder, typePrinter, viewName, options, cg.aliases)
			if err != nil {
				return fmt.Errorf("failed to generate decoder for %s: %w", desc.Type.Name(), err)
			}
		}
	}

	if !options.NoSizeSSZ {
		appendCode(codeBuilder, 0, "func (t %s) SizeSSZDynView(view any) func(ds sszutils.DynamicSpecs) int {\n", typePrinter.TypeString(dataType))
		buildViewDispatcher("sizeSSZView", func() string {
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagDynamicSizer != 0 {
				return "t.SizeSSZDyn"
			}
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 {
				return "func(_ sszutils.DynamicSpecs) int {\n\treturn t.SizeSSZ()\n\t}"
			}
			return ""
		})
		appendCode(codeBuilder, 1, "return nil\n")
		appendCode(codeBuilder, 0, "}\n")

		for _, desc := range views {
			viewName := getViewFnName(desc)
			err = generateSize(desc, codeBuilder, typePrinter, viewName, options)
			if err != nil {
				return fmt.Errorf("failed to generate size for %s: %w", desc.Type.Name(), err)
			}
		}
	}

	if !options.NoHashTreeRoot {
		appendCode(codeBuilder, 0, "func (t %s) HashTreeRootWithDynView(view any) func(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {\n", typePrinter.TypeString(dataType))
		buildViewDispatcher("hashTreeRootView", func() string {
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagDynamicHashRoot != 0 {
				return "t.HashTreeRootWithDyn"
			}
			if dataType.SszCompatFlags&ssztypes.SszCompatFlagFastSSZHasher != 0 {
				if dataType.SszCompatFlags&ssztypes.SszCompatFlagHashTreeRootWith != 0 {
					return "func(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {\n\treturn t.HashTreeRootWith(hh)\n\t}"
				}
				return "func(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {\n\tif root, err := t.HashTreeRoot(); err != nil {\n\t\treturn err\n\t} else {\n\t\thh.AppendBytes32(root[:])\n\t}\n\treturn nil\n\t}"
			}
			return ""
		})
		appendCode(codeBuilder, 1, "return nil\n")
		appendCode(codeBuilder, 0, "}\n")

		for _, desc := range views {
			viewName := getViewFnName(desc)
			err = generateHashTreeRoot(desc, codeBuilder, typePrinter, viewName, options, cg.aliases)
			if err != nil {
				return fmt.Errorf("failed to generate hash tree root for %s: %w", desc.Type.Name(), err)
			}
		}
	}

	return nil
}
