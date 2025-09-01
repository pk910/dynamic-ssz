package codegen

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
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

// Qualify a named type with an alias, recording the import.
func (p *TypePrinter) qualify(t reflect.Type) string {
	pkg := t.PkgPath()
	name := t.Name()
	if pkg == "" { // predeclared or builtin (e.g., int, any)
		return name
	}
	if pkg == p.CurrentPkg {
		return name // same package: unqualified
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

// Public entry point.
func (p *TypePrinter) TypeString(t reflect.Type) string {
	return p.typeString(t)
}

func (p *TypePrinter) typeString(t reflect.Type) string {
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
			return p.processGenericTypeName(t)
		}
		return p.qualify(t)
	}

	// Unnamed kinds
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + p.typeString(t.Elem())
	case reflect.Slice:
		// []byte nicer than []uint8
		if t.Elem().Kind() == reflect.Uint8 && t.Elem().PkgPath() == "" {
			return "[]byte"
		}
		return "[]" + p.typeString(t.Elem())
	case reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 && t.Elem().PkgPath() == "" {
			return fmt.Sprintf("[%d]byte", t.Len())
		}
		return fmt.Sprintf("[%d]%s", t.Len(), p.typeString(t.Elem()))
	case reflect.Struct:
		return p.structString(t)
	default:
		// predeclared unnamed basics (shouldn’t happen except for unsafe types)
		return t.String()
	}
}

func (p *TypePrinter) structString(t reflect.Type) string {
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
			b.WriteString(p.typeString(f.Type))
		} else {
			b.WriteString(f.Name)
			b.WriteByte(' ')
			b.WriteString(p.typeString(f.Type))
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

// processGenericTypeName handles generic types that may contain package paths in their type parameters
func (p *TypePrinter) processGenericTypeName(t reflect.Type) string {
	name := t.Name()

	// Extract and register package imports from the full name
	p.extractAndRegisterImports(name)

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
			p.qualify(t)
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

func escapeBackticks(s string) string {
	// Backticks cannot appear in raw string literals; encode as a quoted + strconv backtick
	if strings.Contains(s, "`") {
		return strconv.Quote(s)[1 : len(strconv.Quote(s))-1] // \"...\" sans outer quotes
	}
	return s
}
