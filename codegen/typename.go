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

type TypePrinter struct {
	CurrentPkg string
	imports    map[string]string
	aliases    map[string]string

	UseRune bool
}

func NewTypePrinter(currentPkg string) *TypePrinter {
	return &TypePrinter{
		CurrentPkg: currentPkg,
		imports:    make(map[string]string),
		aliases:    make(map[string]string),
	}
}

func (p *TypePrinter) Imports() map[string]string { return p.imports }

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

func (p *TypePrinter) Aliases() map[string]string { return p.aliases }

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

// TypeString returns the string representation of the type and tracks uses of imports.
func (p *TypePrinter) TypeString(t *dynssz.TypeDescriptor) string {
	if t.CodegenInfo != nil {
		if codegenInfo, ok := (*t.CodegenInfo).(*CodegenInfo); ok && codegenInfo.Type != nil {
			return p.packageQualify(codegenInfo.Type, true)
		}
	}
	return p.reflectTypeString(t.Type, true)
}

// InnerTypeString returns the string representation of the inner type of the type and tracks uses of imports.
func (p *TypePrinter) InnerTypeString(t *dynssz.TypeDescriptor) string {
	if t.CodegenInfo != nil {
		if codegenInfo, ok := (*t.CodegenInfo).(*CodegenInfo); ok && codegenInfo.Type != nil {
			innerType := codegenInfo.Type
			if named, ok := codegenInfo.Type.(*types.Pointer); ok {
				innerType = named.Elem()
			}
			return p.packageQualify(innerType, true)
		}
	}
	return p.reflectTypeString(t.Type.Elem(), true)
}

// TypeStringWithoutTracking returns the string representation of the type without tracking imports.
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
