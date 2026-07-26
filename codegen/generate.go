// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/build"
	"go/types"
	"reflect"
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

	getTypeName := func(t *CodeGeneratorTypeOptions) (string, string, string) {
		var typeName, typePkgPath, typePkgName string
		if t.ReflectType != nil {
			// An unnamed pointer type carries no name of its own; the named type is
			// the element. Without the dereference every pointer-passed type would
			// share the empty name, colliding in name-keyed bookkeeping (e.g. the
			// duplicate-entry check) and producing nameless error messages. Named
			// pointer types keep their own identity.
			namedType := t.ReflectType
			if namedType.Name() == "" && namedType.Kind() == reflect.Pointer {
				namedType = namedType.Elem()
			}
			typeName = namedType.Name()
			typePkgPath = namedType.PkgPath()
			// Look up the actual package name from the import path
			if typePkgPath != "" {
				if pkg, err := build.Import(typePkgPath, "", build.FindOnly); err == nil {
					typePkgName = pkg.Name
				}
			}
		} else if t.GoTypesType != nil {
			typeName = t.GoTypesType.String()
			types.TypeString(t.GoTypesType, func(pkg *types.Package) string {
				typePkgPath = pkg.Path()
				typePkgName = pkg.Name()
				return ""
			})
		}
		return typeName, typePkgPath, typePkgName
	}

	getGoTypesTypeName := func(t types.Type) string {
		return t.String()
	}

	getReflectTypeName := func(t reflect.Type) string {
		return t.Name()
	}

	hasViews := func(t *CodeGeneratorTypeOptions) bool {
		return len(t.ViewGoTypesTypes) > 0 || len(t.ViewReflectTypes) > 0
	}

	// qualifiedReflectName returns the package-qualified name the reflection
	// type cache uses for compat flag lookups ("" when unavailable).
	qualifiedReflectName := func(t reflect.Type) string {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.PkgPath() == "" || t.Name() == "" {
			return ""
		}
		return t.PkgPath() + "." + t.Name()
	}

	// add compat flags for generated types
	for _, file := range cg.files {
		for _, t := range file.Options.Types {
			typeKey, _, _ := getTypeName(t)
			compatFlags := getCompatFlags(t)

			cg.compatFlags[typeKey] = compatFlags

			// Reflection-driven generation resolves compat flags through the type
			// cache, which keys by package-qualified names; register that key as
			// well so cross-references between generated types delegate.
			qualifiedKey := ""
			if t.ReflectType != nil {
				if qualifiedKey = qualifiedReflectName(t.ReflectType); qualifiedKey != "" {
					cg.compatFlags[qualifiedKey] = compatFlags
				}
			}

			// Also set compat flags for view types (they will have view methods available)
			for _, viewType := range t.ViewGoTypesTypes {
				viewKey := fmt.Sprintf("%v|%v", typeKey, getGoTypesTypeName(viewType))
				cg.compatFlags[viewKey] = compatFlags
			}

			for _, viewType := range t.ViewReflectTypes {
				viewKey := fmt.Sprintf("%v|%v", typeKey, getReflectTypeName(viewType))
				cg.compatFlags[viewKey] = compatFlags

				if qualifiedKey != "" {
					if qualifiedView := qualifiedReflectName(viewType); qualifiedView != "" {
						cg.compatFlags[fmt.Sprintf("%v|%v", qualifiedKey, qualifiedView)] = compatFlags
					}
				}
			}
		}
	}

	cg.typeCache.CompatFlags = cg.compatFlags

	// analyze all types to build complete dependency graph
	for _, file := range cg.files {
		pkgPath := ""
		pkgName := ""
		otherTypeName := ""
		for _, t := range file.Options.Types {
			typeName, typePkgPath, typePkgName := getTypeName(t)

			if typePkgPath == "" {
				return fmt.Errorf("type %s has no package path", typeName)
			}
			if pkgPath == "" {
				pkgPath = typePkgPath
				pkgName = typePkgName
			} else if pkgPath != typePkgPath {
				return fmt.Errorf("type %s has different package path than %s. cannot combine types from different packages in a single file", typeName, otherTypeName)
			}

			otherTypeName = typeName
			t.TypeName = typeName

			// create TypeDescriptor for the data type
			var desc *ssztypes.TypeDescriptor
			var err error

			if t.ReflectType != nil {
				// Always wrap in pointer so generated methods use pointer receivers
				// (needed for unmarshal to write back modified values).
				if t.ReflectType.Kind() != reflect.Pointer {
					t.ReflectType = reflect.PointerTo(t.ReflectType)
				}
				desc, err = cg.typeCache.GetTypeDescriptor(t.ReflectType, t.Options.SizeHints, t.Options.MaxSizeHints, t.Options.TypeHints)
			} else {
				if parser == nil {
					parser = NewParser()
					parser.CompatFlags = cg.compatFlags
					parser.ExtendedTypes = t.Options.ExtendedTypes
					parser.AnnotationResolver = cg.annotationResolver
				}
				if _, ok := t.GoTypesType.(*types.Pointer); !ok {
					t.GoTypesType = types.NewPointer(t.GoTypesType)
				}
				desc, err = parser.GetTypeDescriptor(t.GoTypesType, t.Options.TypeHints, t.Options.SizeHints, t.Options.MaxSizeHints)
			}

			if err != nil {
				return fmt.Errorf("failed to analyze type %s: %w", typeName, err)
			}

			// A fully-delegated type (existing Dynamic* methods + ssz-static
			// annotation, e.g. from a previous generation run) is built as a
			// shallow descriptor without a traversed subtree; generating code
			// from it would dereference nil descriptors. Fail with a clear
			// message instead of panicking.
			if isShallowDelegatedDescriptor(desc) {
				return fmt.Errorf("type %s already implements the generated dynamic SSZ methods; its descriptor cannot be traversed for regeneration - remove the previously generated code (and its ssz-static annotation) first", typeName)
			}

			if err := validateTopLevelType(t, desc, typeName); err != nil {
				return err
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
						parser.AnnotationResolver = cg.annotationResolver
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

		if cg.packageName != "" {
			pkgName = cg.packageName
		} else if pkgName == "" {
			// fallback: derive package name from path if not available from go/types
			pkgName = pkgPath
			if slashIdx := strings.LastIndex(pkgName, "/"); slashIdx != -1 {
				pkgName = pkgName[slashIdx+1:]
			}
		}
		file.Options.PackageName = pkgName
	}

	return nil
}

// validateTopLevelType rejects top-level types that cannot receive generated
// methods. Both work fine as struct fields, but not as standalone -types
// entries:
//   - a Union / CompatibleUnion / TypeWrapper: these are generic library types
//     nameable only via a transparent alias, so a method receiver would resolve
//     to the foreign generic type rather than a local named type.
//   - a named pointer type (type T *U): methods cannot be declared on a type
//     whose underlying type is a pointer.
func validateTopLevelType(t *CodeGeneratorTypeOptions, desc *ssztypes.TypeDescriptor, typeName string) error {
	if desc.SszType == ssztypes.SszUnionType || desc.SszType == ssztypes.SszCompatibleUnionType || desc.SszType == ssztypes.SszTypeWrapperType {
		return fmt.Errorf("cannot generate SSZ methods for top-level %s: a Union/CompatibleUnion/TypeWrapper is nameable only via a type alias and cannot receive methods; use it as a struct field instead", typeName)
	}

	if t.ReflectType != nil {
		if t.ReflectType.Kind() == reflect.Pointer && t.ReflectType.Name() != "" {
			return fmt.Errorf("cannot generate SSZ methods for named pointer type %s: methods cannot be declared on a pointer type", typeName)
		}
		return nil
	}

	base := t.GoTypesType
	if ptr, ok := base.(*types.Pointer); ok {
		base = ptr.Elem()
	}
	if named, ok := base.(*types.Named); ok {
		if _, isPtr := named.Underlying().(*types.Pointer); isPtr {
			return fmt.Errorf("cannot generate SSZ methods for named pointer type %s: methods cannot be declared on a pointer type", typeName)
		}
	}
	return nil
}

// isShallowDelegatedDescriptor reports whether a descriptor was shallow-built
// for a fully-delegated type (ssz-static annotation + existing Dynamic*
// methods): such descriptors carry no traversed subtree and cannot drive code
// generation.
func isShallowDelegatedDescriptor(desc *ssztypes.TypeDescriptor) bool {
	switch desc.SszType {
	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		return desc.ContainerDesc == nil
	case ssztypes.SszVectorType, ssztypes.SszListType, ssztypes.SszBitvectorType,
		ssztypes.SszBitlistType, ssztypes.SszProgressiveListType, ssztypes.SszProgressiveBitlistType:
		return desc.ElemDesc == nil
	case ssztypes.SszUnionType, ssztypes.SszCompatibleUnionType:
		return desc.UnionVariants == nil
	case ssztypes.SszUnspecifiedType:
		// The go/types parser gate returns shallow descriptors before type
		// classification, so a delegating unspecified descriptor is shallow.
		return desc.SszCompatFlags&(ssztypes.SszCompatFlagDynamicMarshaler|ssztypes.SszCompatFlagDynamicEncoder) != 0
	default:
		return false
	}
}

// getCompatFlags computes the SszCompatFlag set for a type based on its generation options.
func getCompatFlags(t *CodeGeneratorTypeOptions) ssztypes.SszCompatFlag {
	var compatFlags ssztypes.SszCompatFlag
	hasViews := len(t.ViewGoTypesTypes) > 0 || len(t.ViewReflectTypes) > 0

	// For view-only types, only set view compat flags
	if t.IsViewOnly {
		compatFlags |= viewCompatFlags(&t.Options)
	} else {
		// Data-only or data+views mode: set data compat flags
		compatFlags |= dataCompatFlags(&t.Options)

		// Data+views mode: also set view compat flags for the data type
		if hasViews {
			compatFlags |= viewCompatFlags(&t.Options)
		}
	}

	return compatFlags
}

// viewCompatFlags returns the view compat flags based on options.
func viewCompatFlags(opts *CodeGeneratorOptions) ssztypes.SszCompatFlag {
	var flags ssztypes.SszCompatFlag
	if !opts.NoMarshalSSZ && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicViewMarshaler
	}
	if !opts.NoUnmarshalSSZ && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicViewUnmarshaler
	}
	if !opts.NoSizeSSZ && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicViewSizer
	}
	if !opts.NoHashTreeRoot && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicViewHashRoot
	}
	if opts.CreateEncoderFn {
		flags |= ssztypes.SszCompatFlagDynamicViewEncoder
	}
	if opts.CreateDecoderFn {
		flags |= ssztypes.SszCompatFlagDynamicViewDecoder
	}
	return flags
}

// dataCompatFlags returns the data compat flags based on options.
func dataCompatFlags(opts *CodeGeneratorOptions) ssztypes.SszCompatFlag {
	var flags ssztypes.SszCompatFlag
	if !opts.NoMarshalSSZ && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicMarshaler
	}
	if !opts.NoUnmarshalSSZ && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicUnmarshaler
	}
	if !opts.NoSizeSSZ && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicSizer
	}
	if !opts.NoHashTreeRoot && !opts.WithoutDynamicExpressions {
		flags |= ssztypes.SszCompatFlagDynamicHashRoot
	}
	if opts.CreateEncoderFn {
		flags |= ssztypes.SszCompatFlagDynamicEncoder
	}
	if opts.CreateDecoderFn {
		flags |= ssztypes.SszCompatFlagDynamicDecoder
	}

	if !opts.NoMarshalSSZ && !opts.NoUnmarshalSSZ && !opts.NoSizeSSZ && (opts.CreateLegacyFn || opts.WithoutDynamicExpressions) {
		flags |= ssztypes.SszCompatFlagFastSSZMarshaler
	}
	if !opts.NoHashTreeRoot && (opts.CreateLegacyFn || opts.WithoutDynamicExpressions) {
		if opts.CreateLegacyFn {
			flags |= ssztypes.SszCompatFlagFastSSZHasher
		}
		flags |= ssztypes.SszCompatFlagHashTreeRootWith
	}
	return flags
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
//
// staticAnnotationFor returns the ssz-static annotation declaring whether a
// generated type is fixed-size (static) or variable-size (dynamic). The
// reflection typecache uses it to shallow-build the fully-delegated type without
// descending into its subtree; for static types it reads the fixed size from the
// type's own sizer.
func staticAnnotationFor(desc *ssztypes.TypeDescriptor) string {
	if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
		return `ssz-static:"false"`
	}
	return `ssz-static:"true"`
}

// packageScopeNames returns the top-level identifier names declared in the
// package that owns t (pointers and aliases unwrapped). Returns nil when the
// package cannot be determined (e.g. reflection-driven types with no go/types
// information), in which case no names are reserved.
func packageScopeNames(t types.Type) []string {
	if t == nil {
		return nil
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return nil
	}
	return named.Obj().Pkg().Scope().Names()
}

func (cg *CodeGenerator) generateFile(packagePath string, opts *CodeGeneratorFileOptions) (string, error) {
	if len(opts.Types) == 0 {
		return "", fmt.Errorf("no types requested for generation")
	}

	typePrinter := NewTypePrinter(packagePath)
	typePrinter.AddAlias("github.com/pk910/dynamic-ssz", "dynssz")
	// Reserve the target package's top-level identifiers so a generated import
	// alias (e.g. sszutils, hasher, binary, dynssz) never collides with one.
	for _, t := range opts.Types {
		if names := packageScopeNames(t.GoTypesType); names != nil {
			typePrinter.ReserveNames(names)
			break
		}
	}
	codeBuilder := strings.Builder{}
	annotationsBuilder := strings.Builder{}
	hashParts := [][]byte{}

	// A type listed twice for the same output would emit its method set twice and
	// fail to compile ("method already declared"). Reject the duplicate with a
	// clear error instead of producing broken code that reports success.
	seenTypes := make(map[string]struct{}, len(opts.Types))

	for _, t := range opts.Types {
		if _, dup := seenTypes[t.TypeName]; dup {
			return "", fmt.Errorf("type %s is listed more than once for the same output; remove the duplicate entry", t.TypeName)
		}
		seenTypes[t.TypeName] = struct{}{}

		if t.Descriptor == nil || (t.IsViewOnly && len(t.ViewDescriptors) == 0) {
			return "", fmt.Errorf("type %s has no descriptor or view descriptors", t.TypeName)
		}

		// A recursive descriptor graph is only emittable when every cycle can be
		// broken by a delegated method call; verify before emission so an
		// unsupported shape produces a clear error instead of unbounded recursion.
		if err := validateEmittableGraph(t.Descriptor); err != nil {
			return "", fmt.Errorf("cannot generate code for %s: %w", t.TypeName, err)
		}
		for _, viewDesc := range t.ViewDescriptors {
			if err := validateEmittableGraph(viewDesc); err != nil {
				return "", fmt.Errorf("cannot generate code for view of %s: %w", t.TypeName, err)
			}
		}

		if !t.IsViewOnly {
			hash := t.Descriptor.GetTypeHash()
			hashParts = append(hashParts, hash[:])

			err := cg.generateSSZMethods(t.Descriptor, typePrinter, &codeBuilder, "", &t.Options)
			if err != nil {
				return "", fmt.Errorf("failed to generate code for %s: %w", t.TypeName, err)
			}

			// Declare static/dynamic so the reflection typecache can shallow-build
			// this fully-delegated type without descending into its subtree.
			fmt.Fprintf(&annotationsBuilder, "var _ = sszutils.Annotate[%s](`%s`)\n", typePrinter.InnerTypeString(t.Descriptor), staticAnnotationFor(t.Descriptor))
		}

		if len(t.ViewDescriptors) > 0 {
			for _, viewDesc := range t.ViewDescriptors {
				hash := viewDesc.GetTypeHash()
				hashParts = append(hashParts, hash[:])
			}

			err := cg.generateSSZViewMethods(t.Descriptor, t.ViewDescriptors, typePrinter, &codeBuilder, &t.Options)
			if err != nil {
				return "", fmt.Errorf("failed to generate code for view types of %s: %w", t.TypeName, err)
			}

			// Each view schema type may have its own static/dynamic shape, so the
			// annotation is emitted per view schema type (not the base type).
			for _, viewDesc := range t.ViewDescriptors {
				fmt.Fprintf(&annotationsBuilder, "var _ = sszutils.Annotate[%s](`%s`)\n", typePrinter.ViewTypeString(viewDesc, false), staticAnnotationFor(viewDesc))
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

		// Split third-party imports from stdlib/local paths without paying the cost
		// of compiling and running a regexp in this hot codegen path.
		if isThirdPartyImport(path) {
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
	headerTemplate := cg.headerTemplate
	if headerTemplate == "" {
		headerTemplate = DefaultHeaderTemplate
	}
	header := strings.ReplaceAll(headerTemplate, "{hash}", hex.EncodeToString(typesHash[:]))
	header = strings.ReplaceAll(header, "{version}", Version)
	mainCodeBuilder.WriteString(header)
	if !strings.HasSuffix(header, "\n") {
		mainCodeBuilder.WriteString("\n")
	}
	fmt.Fprintf(&mainCodeBuilder, "package %s\n\n", opts.PackageName)

	// Imports
	if len(sysImports) > 0 || len(pkgImports) > 0 {
		mainCodeBuilder.WriteString("import (\n")
		for _, imp := range sysImports {
			if imp.Alias != "" {
				fmt.Fprintf(&mainCodeBuilder, "\t%s \"%s\"\n", imp.Alias, imp.Path)
			} else {
				fmt.Fprintf(&mainCodeBuilder, "\t\"%s\"\n", imp.Path)
			}
		}

		if len(sysImports) > 0 && len(pkgImports) > 0 {
			mainCodeBuilder.WriteString("\n")
		}

		for _, imp := range pkgImports {
			if imp.Alias != "" {
				fmt.Fprintf(&mainCodeBuilder, "\t%s \"%s\"\n", imp.Alias, imp.Path)
			} else {
				fmt.Fprintf(&mainCodeBuilder, "\t\"%s\"\n", imp.Path)
			}
		}
		mainCodeBuilder.WriteString(")\n\n")
	}

	// Variable declarations
	mainCodeBuilder.WriteString("var _ = sszutils.ErrListTooBig\n\n")

	// Type annotations (ssz-static declarations) up front so the reflection
	// typecache can shallow-build these fully-delegated types.
	if annotationsBuilder.Len() > 0 {
		mainCodeBuilder.WriteString(annotationsBuilder.String())
		mainCodeBuilder.WriteString("\n")
	}

	// Generated code
	generatedCode := codeBuilder.String()
	if strings.HasSuffix(generatedCode, "\n\n") {
		generatedCode = generatedCode[:len(generatedCode)-1] // trim final double line break
	}
	mainCodeBuilder.WriteString(generatedCode)

	// Normalize the assembled file to canonical gofmt formatting so template
	// spacing does not have to replicate gofmt's context-dependent rules.
	formattedCode, err := formatGoSource(mainCodeBuilder.String())
	if err != nil {
		return "", fmt.Errorf("failed to format generated code for package %s: %w", opts.PackageName, err)
	}

	return formattedCode, nil
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
	typeName := typePrinter.TypeStringWithoutTracking(desc, viewName != "")

	if !options.NoMarshalSSZ {
		err = generateMarshal(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate marshal for %s: %w", typeName, err)
		}
	}

	if options.CreateEncoderFn {
		err = generateEncoder(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate encoder for %s: %w", typeName, err)
		}
	}

	if !options.NoUnmarshalSSZ {
		err = generateUnmarshal(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate unmarshal for %s: %w", typeName, err)
		}
	}

	if options.CreateDecoderFn {
		err = generateDecoder(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate decoder for %s: %w", typeName, err)
		}
	}

	if !options.NoSizeSSZ {
		err = generateSize(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate size for %s: %w", typeName, err)
		}
	}

	if !options.NoHashTreeRoot {
		err = generateHashTreeRoot(desc, codeBuilder, typePrinter, viewName, options)
		if err != nil {
			return fmt.Errorf("failed to generate hash tree root for %s: %w", typeName, err)
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
			typeName := typePrinter.ViewTypeString(view, true)
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
			err = generateMarshal(desc, codeBuilder, typePrinter, viewName, options)
			if err != nil {
				return fmt.Errorf("failed to generate marshal for %s: %w", viewName, err)
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
				return fmt.Errorf("failed to generate encoder for %s: %w", viewName, err)
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
				return fmt.Errorf("failed to generate unmarshal for %s: %w", viewName, err)
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
			err = generateDecoder(desc, codeBuilder, typePrinter, viewName, options)
			if err != nil {
				return fmt.Errorf("failed to generate decoder for %s: %w", viewName, err)
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
				return fmt.Errorf("failed to generate size for %s: %w", viewName, err)
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
			err = generateHashTreeRoot(desc, codeBuilder, typePrinter, viewName, options)
			if err != nil {
				return fmt.Errorf("failed to generate hash tree root for %s: %w", viewName, err)
			}
		}
	}

	return nil
}

func isThirdPartyImport(path string) bool {
	before, _, ok := strings.Cut(path, "/")
	if !ok {
		return strings.Contains(path, ".")
	}
	return strings.Contains(before, ".")
}
