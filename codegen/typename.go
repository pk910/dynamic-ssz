// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/types"
	"reflect"
	"regexp"
	"strings"

	dynssz "github.com/pk910/dynamic-ssz"
)

// TypePrinter manages type name formatting and import tracking for code generation.
//
// The TypePrinter provides intelligent type name formatting that handles package
// qualification, import management, and alias generation. It ensures that generated
// code has clean, readable type names while avoiding import conflicts.
//
// Key capabilities:
//   - Automatic package qualification for types from other packages
//   - Import path tracking and alias generation
//   - Conflict resolution for import aliases
//   - Support for both reflection and go/types type representations
//   - Generic type name formatting with proper package path handling
//
// Fields:
//   - CurrentPkg: The package path of the code being generated (types from this package are unqualified)
//   - imports: Map of import paths to their assigned aliases
//   - aliases: Map of import paths to their preferred aliases
//   - UseRune: Whether to prefer "rune" over "int32" in type names
type TypePrinter struct {
	CurrentPkg string
	imports    map[string]string
	aliases    map[string]string

	UseRune bool
}

// NewTypePrinter creates a new type printer for the specified package.
//
// The type printer is configured to generate type names relative to the specified
// current package. Types from the current package will be unqualified, while types
// from other packages will be properly qualified with import aliases.
//
// Parameters:
//   - currentPkg: The Go package path of the code being generated
//
// Returns:
//   - *TypePrinter: A new type printer ready for type formatting and import tracking
//
// Example:
//
//	printer := NewTypePrinter("github.com/example/mypackage")
//	typeName := printer.TypeString(myTypeDescriptor) // Will qualify external types
func NewTypePrinter(currentPkg string) *TypePrinter {
	return &TypePrinter{
		CurrentPkg: currentPkg,
		imports:    make(map[string]string),
		aliases:    make(map[string]string),
	}
}

// Imports returns the map of import paths to their assigned aliases.
//
// This method provides access to all import paths that have been registered
// during type formatting operations. The returned map can be used to generate
// the import section of a Go source file.
//
// Returns:
//   - map[string]string: Map of import paths to their assigned aliases
func (p *TypePrinter) Imports() map[string]string { return p.imports }

// AddImport registers an import path with a preferred alias and returns the assigned alias.
//
// This method adds an import path to the printer's import tracking system. If the path
// is already registered, it returns the existing alias. If the preferred alias conflicts
// with an existing import, it generates a unique alternative by appending numbers.
//
// Parameters:
//   - path: The import path to register (e.g., "github.com/pkg/errors")
//   - alias: The preferred alias for the import (e.g., "errors")
//
// Returns:
//   - string: The actual alias assigned to the import (may differ from preferred if conflicts occur)
//
// Example:
//
//	alias := printer.AddImport("github.com/pkg/errors", "errors")
//	// alias == "errors" if no conflict, or "errors1", "errors2", etc. if conflicts exist
func (p *TypePrinter) AddImport(path, alias string) string {
	if p.imports[path] == "" {
		// ensure alias uniqueness
		base := alias
		i := 1
		for containsValue(p.imports, alias) {
			alias = fmt.Sprintf("%s%d", base, i)
			i++
		}

		p.imports[path] = alias
	} else {
		alias = p.imports[path]
	}
	return alias
}

// Aliases returns the map of import paths to their preferred aliases.
//
// This method provides access to the preferred alias mappings that were set
// via AddAlias(). These aliases take precedence over automatically generated
// aliases when formatting import statements.
//
// Returns:
//   - map[string]string: Map of import paths to their preferred aliases
func (p *TypePrinter) Aliases() map[string]string { return p.aliases }

// AddAlias sets a preferred alias for an import path.
//
// This method establishes a preferred alias for a specific import path.
// When generating import statements, these preferred aliases will be used
// instead of automatically generated ones, providing consistent and
// predictable import formatting.
//
// Parameters:
//   - path: The import path to set an alias for
//   - alias: The preferred alias to use for this import path
//
// Example:
//
//	printer.AddAlias("github.com/pk910/dynamic-ssz", "dynssz")
//	// All references to dynamic-ssz types will use "dynssz" as the package qualifier
func (p *TypePrinter) AddAlias(path, alias string) {
	p.aliases[path] = alias
}

// reflectQualify prints reflection.Type t using p.imports/p.aliases to track and assign package aliases.
// It qualifies types from other packages; types in CurrentPkg (or predeclared) are unqualified.
func (p *TypePrinter) reflectQualify(t reflect.Type, trackImports bool) string {
	pkg := t.PkgPath()
	name := t.Name()
	if pkg == "" { // predeclared or builtin (e.g., int, any)
		return name
	}
	if pkg == p.CurrentPkg {
		return name // same package: unqualified
	}

	if !trackImports {
		return name
	}

	alias := p.imports[pkg]
	if alias == "" {
		alias = normalizeAlias(p.defaultAlias(pkg))
		// ensure alias uniqueness
		base := alias
		i := 1
		for containsValue(p.imports, alias) {
			alias = fmt.Sprintf("%s%d", base, i)
			i++
		}
		p.imports[pkg] = alias
	}

	return alias + "." + name
}

// packageQualify prints types.Type t using p.imports/p.aliases to track and assign package aliases.
// It qualifies types from other packages; types in CurrentPkg (or predeclared) are unqualified.
func (p *TypePrinter) packageQualify(t types.Type, trackImports bool) string {
	qual := func(pkg *types.Package) string {
		if pkg == nil || !trackImports {
			// predeclared/builtin (e.g., int, any) or no tracking
			return ""
		}
		path := pkg.Path()
		if path == "" || path == p.CurrentPkg {
			// same package or no path: unqualified
			return ""
		}

		// already have an alias for this import path?
		if alias := p.imports[path]; alias != "" {
			return alias
		}

		// pick a default alias and ensure uniqueness across recorded imports
		alias := normalizeAlias(p.defaultAlias(path))
		base := alias
		i := 1
		for containsValue(p.imports, alias) || (p.aliases != nil && p.aliases[alias] != "" && p.aliases[alias] != path) {
			alias = fmt.Sprintf("%s%d", base, i)
			i++
		}

		// record alias
		p.imports[path] = alias
		return alias
	}

	s := types.TypeString(t, qual)

	// Optional: mimic your rune preference (rune over int32) if desired.
	if p.UseRune {
		if b, ok := t.Underlying().(*types.Basic); ok && b.Kind() == types.Int32 && s == "int32" {
			s = "rune"
		}
	}

	return s
}

func containsValue(m map[string]string, v string) bool {
	for _, vv := range m {
		if vv == v {
			return true
		}
	}
	return false
}

func (p *TypePrinter) defaultAlias(importPath string) string {
	if alias, ok := p.aliases[importPath]; ok {
		return alias
	}
	// naive but effective: last path element (handles stdlib + common cases)
	parts := strings.Split(importPath, "/")
	return parts[len(parts)-1]
}

func normalizeAlias(alias string) string {
	alias = strings.ReplaceAll(alias, "-", "_")
	return alias
}

// TypeString returns the qualified string representation of a type descriptor and tracks import usage.
//
// This method generates the appropriate Go type string for a TypeDescriptor, handling
// package qualification and import tracking automatically. It works with both types
// analyzed via go/types (compile-time) and reflection (runtime).
//
// The method automatically:
//   - Qualifies types from other packages with appropriate import aliases
//   - Tracks import usage for later import statement generation
//   - Handles generic types with complex package path references
//   - Prefers compile-time type information when available
//
// Parameters:
//   - t: The TypeDescriptor containing type information for formatting
//
// Returns:
//   - string: The qualified Go type string suitable for code generation
//
// Example:
//
//	typeName := printer.TypeString(descriptor)
//	// Result: "phase0.BeaconBlock" or "*MyStruct" depending on the type
func (p *TypePrinter) TypeString(t *dynssz.TypeDescriptor) string {
	if t.CodegenInfo != nil {
		if codegenInfo, ok := (*t.CodegenInfo).(*CodegenInfo); ok && codegenInfo.Type != nil {
			return p.packageQualify(codegenInfo.Type, true)
		}
	}
	return p.reflectTypeString(t.Type, true)
}

// InnerTypeString returns the qualified string representation of the inner (dereferenced) type.
//
// This method is similar to TypeString but automatically dereferences pointer types
// to get the underlying type. It's particularly useful when generating code that
// needs to work with the actual value type rather than pointer types.
//
// For pointer types, this returns the element type. For non-pointer types,
// this behaves identically to TypeString.
//
// Parameters:
//   - t: The TypeDescriptor containing type information for formatting
//
// Returns:
//   - string: The qualified Go type string of the inner/dereferenced type
//
// Example:
//
//	// For *MyStruct
//	innerType := printer.InnerTypeString(descriptor)
//	// Result: "MyStruct" (without the pointer)
func (p *TypePrinter) InnerTypeString(t *dynssz.TypeDescriptor) string {
	if t.CodegenInfo != nil {
		if codegenInfo, ok := (*t.CodegenInfo).(*CodegenInfo); ok && codegenInfo.Type != nil {
			innerType := codegenInfo.Type
			if named, ok := innerType.(*types.Named); ok {
				ptrInnerType := named.Underlying()
				if ptr, ok := ptrInnerType.(*types.Pointer); ok {
					innerType = ptr.Elem()
				}
			}
			if named, ok := innerType.(*types.Pointer); ok {
				innerType = named.Elem()
			}
			return p.packageQualify(innerType, true)
		}
	}
	return p.reflectTypeString(t.Type.Elem(), true)
}

// TypeStringWithoutTracking returns the type string representation without tracking import usage.
//
// This method generates type strings without adding imports to the tracking system.
// It's useful for scenarios where you need type names for analysis or comparison
// but don't want to affect the import management (e.g., during validation or
// debugging).
//
// The generated string will still be properly qualified but won't register
// any imports for later code generation.
//
// Parameters:
//   - t: The TypeDescriptor containing type information for formatting
//
// Returns:
//   - string: The qualified Go type string without import tracking side effects
func (p *TypePrinter) TypeStringWithoutTracking(t *dynssz.TypeDescriptor) string {
	if t.CodegenInfo != nil {
		if codegenInfo, ok := (*t.CodegenInfo).(*CodegenInfo); ok && codegenInfo.Type != nil {
			return p.packageQualify(codegenInfo.Type, false)
		}
	}
	return p.reflectTypeString(t.Type, false)
}

func (p *TypePrinter) reflectTypeString(t reflect.Type, trackImports bool) string {
	// Named types first
	if t.Name() != "" {
		// Special-case predeclared aliases: byte and rune preferences
		if t.Kind() == reflect.Uint8 && t.PkgPath() == "" {
			return "byte"
		}
		if p.UseRune && t.Kind() == reflect.Int32 && t.PkgPath() == "" {
			return "rune"
		}
		// Check if this is a generic type with embedded package paths
		if strings.Contains(t.Name(), "[") && strings.Contains(t.Name(), "]") {
			return p.reflectGenericTypeName(t, trackImports)
		}
		return p.reflectQualify(t, trackImports)
	}

	// Unnamed kinds
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + p.reflectTypeString(t.Elem(), trackImports)
	case reflect.Slice:
		// []byte nicer than []uint8
		if t.Elem().Kind() == reflect.Uint8 && t.Elem().PkgPath() == "" {
			return "[]byte"
		}
		return "[]" + p.reflectTypeString(t.Elem(), trackImports)
	case reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 && t.Elem().PkgPath() == "" {
			return fmt.Sprintf("[%d]byte", t.Len())
		}
		return fmt.Sprintf("[%d]%s", t.Len(), p.reflectTypeString(t.Elem(), trackImports))
	case reflect.Struct:
		return p.reflectStructString(t, trackImports)
	default:
		// predeclared unnamed basics (shouldn’t happen except for unsafe types)
		return t.String()
	}
}

func (p *TypePrinter) reflectStructString(t reflect.Type, trackImports bool) string {
	if t.NumField() == 0 {
		return "struct{}"
	}
	var b strings.Builder
	b.WriteString("struct{ ")
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if i > 0 {
			b.WriteByte(' ')
		}
		if f.Anonymous {
			// Embedded: just the type
			b.WriteString(p.reflectTypeString(f.Type, trackImports))
		} else {
			b.WriteString(f.Name)
			b.WriteByte(' ')
			b.WriteString(p.reflectTypeString(f.Type, trackImports))
		}
		if tag := string(f.Tag); tag != "" {
			b.WriteByte(' ')
			b.WriteByte('`')
			b.WriteString(escapeBackticks(tag))
			b.WriteByte('`')
		}
		b.WriteByte(';')
	}
	b.WriteString(" }")
	return b.String()
}

// reflectGenericTypeName handles generic types that may contain package paths in their type parameters
func (p *TypePrinter) reflectGenericTypeName(t reflect.Type, trackImports bool) string {
	name := t.Name()

	// Extract and register package imports from the full name
	if trackImports {
		p.extractAndRegisterImports(name)
	}

	// Clean up the full name by replacing full package paths with qualified type names
	cleanedName := p.cleanGenericTypeName(name)

	// Now handle the qualification of the base type
	pkgPath := t.PkgPath()
	if pkgPath != "" && pkgPath != p.CurrentPkg {
		// Extract just the base type name (before generic params)
		baseName := cleanedName
		if idx := strings.Index(cleanedName, "["); idx != -1 {
			baseName = cleanedName[:idx]
		}

		// Get the alias for this package
		alias := p.imports[pkgPath]
		if alias == "" {
			// Ensure the type is qualified (this will add it to imports)
			p.reflectQualify(t, trackImports)
			alias = p.imports[pkgPath]
		}

		// Replace the base name with qualified version
		if idx := strings.Index(cleanedName, "["); idx != -1 {
			return alias + "." + cleanedName
		}
		return alias + "." + baseName
	}

	return cleanedName
}

// extractAndRegisterImports finds package paths in the type string and registers them as imports
func (p *TypePrinter) extractAndRegisterImports(typeStr string) {
	// Match package paths like github.com/attestantio/go-eth2-client/spec/phase0
	pkgPattern := regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_]*(?:\.[a-zA-Z][a-zA-Z0-9_]*)*(?:/[a-zA-Z][a-zA-Z0-9_.-]*)+)\.([A-Z][a-zA-Z0-9_]*)`)
	matches := pkgPattern.FindAllStringSubmatch(typeStr, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			pkgPath := match[1]
			// Register this import
			if p.imports[pkgPath] == "" {
				alias := normalizeAlias(p.defaultAlias(pkgPath))
				// ensure alias uniqueness
				base := alias
				i := 1
				for containsValue(p.imports, alias) {
					alias = fmt.Sprintf("%s%d", base, i)
					i++
				}
				p.imports[pkgPath] = alias
			}
		}
	}
}

// cleanGenericTypeName replaces full package paths with qualified type names using registered aliases
func (p *TypePrinter) cleanGenericTypeName(genericStr string) string {
	result := genericStr

	// Replace full package paths with alias.Type format
	pkgPattern := regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_]*(?:\.[a-zA-Z][a-zA-Z0-9_]*)*(?:/[a-zA-Z][a-zA-Z0-9_.-]*)+)\.([A-Z][a-zA-Z0-9_]*)`)

	result = pkgPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := pkgPattern.FindStringSubmatch(match)
		if len(submatches) >= 3 {
			pkgPath := submatches[1]
			typeName := submatches[2]
			if alias, ok := p.imports[pkgPath]; ok {
				return alias + "." + typeName
			}
		}
		return match
	})

	return result
}
