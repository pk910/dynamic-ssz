package codegen

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type TypePrinter struct {
	CurrentPkg string
	imports    map[string]string

	UseRune bool
}

func NewTypePrinter(currentPkg string) *TypePrinter {
	return &TypePrinter{
		CurrentPkg: currentPkg,
		imports:    make(map[string]string),
	}
}

func (p *TypePrinter) Imports() map[string]string { return p.imports }

func (p *TypePrinter) AddImport(path, alias string) {
	p.imports[path] = alias
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
		alias = normalizeAlias(defaultAlias(pkg))
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

func defaultAlias(importPath string) string {
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

func escapeBackticks(s string) string {
	// Backticks cannot appear in raw string literals; encode as a quoted + strconv backtick
	if strings.Contains(s, "`") {
		return strconv.Quote(s)[1 : len(strconv.Quote(s))-1] // \"...\" sans outer quotes
	}
	return s
}
