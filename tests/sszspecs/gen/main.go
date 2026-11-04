// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Command gen writes the Go types the official SSZ vectors declare.
//
// Every vector carries its type as a descriptor. This command reads the
// descriptors of the serialization, proof and multiproof vectors and writes
// one Go type per distinct declaration, each registered under the
// declaration's key, plus the list of types dynssz-gen generates SSZ methods
// for. The declarations the illegal-type vectors carry go to a file of their
// own: the library has to refuse those, so no methods are generated for them.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pk910/dynamic-ssz/tests/sszspecs/descriptor"
)

const (
	typesFile   = "gen_types.go"
	illegalFile = "gen_illegal.go"
	configFile  = "gen_ssz.yaml"
	codeFile    = "gen_ssz.go"
	packagePath = "github.com/pk910/dynamic-ssz/tests/sszspecs"

	// The two counts a declaration can state.
	countLength = "length"
	countLimit  = "limit"

	// legalPrefix names the types of the vectors the library has to pass.
	legalPrefix = "T"
)

// errInexpressible marks a declaration Go has no spelling for. Such a
// declaration cannot reach the library at all.
var errInexpressible = errors.New("inexpressible")

// errUnsupported marks a legal declaration the library has no spelling for.
// The test lists every vector that carries one instead of running it.
var errUnsupported = errors.New("unsupported")

type indexFile struct {
	Cases []indexCase `json:"cases"`
}

type indexCase struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type vector struct {
	TypeDescriptor *descriptor.Descriptor `json:"typeDescriptor"`
}

// emitter collects the type declarations of one generated file.
type emitter struct {
	prefix string
	names  map[string]string
	decls  []string
	// generated lists the declared types dynssz-gen can attach methods to.
	generated []string
	usesUnion bool
	usesAnnot bool
}

func newEmitter(prefix string) *emitter {
	return &emitter{prefix: prefix, names: make(map[string]string, 256)}
}

func main() {
	fixtures := flag.String("fixtures", "", "directory holding the generated ssz-specs fixtures")
	out := flag.String("out", ".", "directory the generated files are written to")
	flag.Parse()

	if err := run(*fixtures, *out); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run(fixtures, out string) error {
	if fixtures == "" {
		return errors.New("-fixtures is required")
	}

	data, err := os.ReadFile(filepath.Join(fixtures, "index.json"))
	if err != nil {
		return err
	}

	var index indexFile
	if err = json.Unmarshal(data, &index); err != nil {
		return fmt.Errorf("parse index.json: %w", err)
	}

	legal := newEmitter(legalPrefix)
	illegal := newEmitter("X")
	registry := make(map[string]string, 256)
	unsupported := make(map[string]string, 16)
	illegalRegistry := make(map[string]string, 32)
	inexpressible := make(map[string]string, 32)

	for _, c := range index.Cases {
		format, _, _ := strings.Cut(c.Path, "/")

		switch format {
		case "ssz", "proof", "multiproof":
			desc, key, readErr := readDescriptor(filepath.Join(fixtures, c.Path))
			if readErr != nil {
				return readErr
			}

			goType, emitErr := legal.emit(desc)

			switch {
			case errors.Is(emitErr, errUnsupported):
				unsupported[key] = emitErr.Error()
			case emitErr != nil:
				return fmt.Errorf("%s: %w", c.ID, emitErr)
			default:
				registry[key] = goType
			}
		case "ssz_type_rejection":
			desc, _, readErr := readDescriptor(filepath.Join(fixtures, c.Path))
			if readErr != nil {
				return readErr
			}

			goType, emitErr := illegal.emit(desc)

			switch {
			case errors.Is(emitErr, errInexpressible):
				inexpressible[c.ID] = emitErr.Error()
			case emitErr != nil:
				return fmt.Errorf("%s: %w", c.ID, emitErr)
			default:
				illegalRegistry[c.ID] = goType
			}
		default:
			// Path and JSON vectors exercise features the library does not
			// have; the test accounts for them without a type.
		}
	}

	if err := writeTypes(filepath.Join(out, typesFile), legal, registry, unsupported); err != nil {
		return err
	}

	if err := writeIllegal(filepath.Join(out, illegalFile), illegal, illegalRegistry, inexpressible); err != nil {
		return err
	}

	return writeConfig(filepath.Join(out, configFile), legal.generated)
}

func readDescriptor(path string) (*descriptor.Descriptor, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	var v vector
	if err = json.Unmarshal(data, &v); err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", path, err)
	}

	if v.TypeDescriptor == nil {
		return nil, "", fmt.Errorf("%s carries no typeDescriptor", path)
	}

	key, err := v.TypeDescriptor.Key()
	if err != nil {
		return nil, "", err
	}

	return v.TypeDescriptor, key, nil
}

// emit returns the Go type expression for a declaration, declaring a named
// type for it where its SSZ shape needs one.
func (e *emitter) emit(d *descriptor.Descriptor) (string, error) {
	key, err := d.Key()
	if err != nil {
		return "", err
	}

	if name, ok := e.names[key]; ok {
		return name, nil
	}

	switch d.Kind {
	case "Boolean":
		return "bool", nil
	case "Byte", "Uint8":
		return "uint8", nil
	case "Uint16":
		return "uint16", nil
	case "Uint32":
		return "uint32", nil
	case "Uint64":
		return "uint64", nil
	case "Uint128":
		return e.declare(key, d.Kind, "[16]byte", `ssz-type:"uint128"`), nil
	case "Uint256":
		return e.declare(key, d.Kind, "[32]byte", `ssz-type:"uint256"`), nil
	case "BitVector":
		if err := surplus(d, d.Limit, countLimit); err != nil {
			return "", err
		}

		length, lenErr := count(d.Length)
		if lenErr != nil {
			return "", lenErr
		}

		tag := fmt.Sprintf(`ssz-type:"bitvector" ssz-bitsize:"%d"`, length)

		return e.declare(key, d.Kind, fmt.Sprintf("[%d]byte", (length+7)/8), tag), nil
	case "ByteVector":
		if err := surplus(d, d.Limit, countLimit); err != nil {
			return "", err
		}

		length, lenErr := count(d.Length)
		if lenErr != nil {
			return "", lenErr
		}

		return e.declare(key, d.Kind, fmt.Sprintf("[%d]byte", length), ""), nil
	case "BitList":
		return e.declareList(key, d, "[]byte", `ssz-type:"bitlist"`, true)
	case "ProgressiveBitList":
		return e.declareList(key, d, "[]byte", `ssz-type:"progressive-bitlist"`, false)
	case "ByteList":
		return e.declareList(key, d, "[]byte", "", true)
	case "Vector":
		if err := surplus(d, d.Limit, countLimit); err != nil {
			return "", err
		}

		length, lenErr := count(d.Length)
		if lenErr != nil {
			return "", lenErr
		}

		elem, elemErr := e.element(d)
		if elemErr != nil {
			return "", elemErr
		}

		return e.declare(key, d.Kind, fmt.Sprintf("[%d]%s", length, elem), ""), nil
	case "List", "ProgressiveList":
		elem, elemErr := e.element(d)
		if elemErr != nil {
			return "", elemErr
		}

		typeTag := ""
		if d.Kind == "ProgressiveList" {
			typeTag = `ssz-type:"progressive-list"`
		}

		return e.declareList(key, d, "[]"+elem, typeTag, d.Kind == "List")
	case "Container":
		return e.declareContainer(key, d, false)
	case "ProgressiveContainer":
		return e.declareContainer(key, d, true)
	case "CompatibleUnion":
		return e.declareUnion(key, d)
	default:
		return "", fmt.Errorf("%w: Go has no type for kind %s", errInexpressible, d.Kind)
	}
}

func (e *emitter) element(d *descriptor.Descriptor) (string, error) {
	if d.ElementType == nil {
		return "", fmt.Errorf("%s declares no elementType", d.Kind)
	}

	return e.emit(d.ElementType)
}

// declare adds a named type and returns its name.
func (e *emitter) declare(key, kind, underlying, tag string) string {
	name := fmt.Sprintf("%s%03d%s", e.prefix, len(e.names)+1, kind)
	e.names[key] = name

	decl := fmt.Sprintf("// %s is a %s.\ntype %s %s\n", name, kind, name, underlying)
	if tag != "" {
		decl += fmt.Sprintf("\nvar _ = sszutils.Annotate[%s](`%s`)\n", name, tag)
		e.usesAnnot = true
	}

	e.decls = append(e.decls, decl)
	e.generated = append(e.generated, name)

	return name
}

// declareList declares a list-shaped type. A bounded shape has to state its
// limit; a progressive one may.
func (e *emitter) declareList(key string, d *descriptor.Descriptor, underlying, typeTag string, bounded bool) (string, error) {
	if err := surplus(d, d.Length, countLength); err != nil {
		return "", err
	}

	tags := make([]string, 0, 2)
	if typeTag != "" {
		tags = append(tags, typeTag)
	}

	switch {
	case d.Limit != nil && *d.Limit == 0 && e.prefix == legalPrefix:
		// The library reads a limit of zero as "no limit stated", so a list
		// that can hold nothing has no spelling in it.
		return "", fmt.Errorf("%w: a %s with a capacity of zero", errUnsupported, d.Kind)
	case d.Limit != nil:
		tags = append(tags, fmt.Sprintf(`ssz-max:"%d"`, *d.Limit))
	case bounded:
		return "", fmt.Errorf("%s declares no limit", d.Kind)
	}

	return e.declare(key, d.Kind, underlying, strings.Join(tags, " ")), nil
}

func (e *emitter) declareContainer(key string, d *descriptor.Descriptor, progressive bool) (string, error) {
	indices := make([]int, 0, len(d.Fields))

	if progressive {
		var err error

		indices, err = layout(d)
		if err != nil {
			return "", err
		}
	}

	fields := make([]string, 0, len(d.Fields))

	for i, field := range d.Fields {
		goType, err := e.emit(field.Type)
		if err != nil {
			return "", err
		}

		line := fmt.Sprintf("\tF%d %s", i, goType)
		if progressive {
			line += fmt.Sprintf(" `ssz-index:\"%d\"`", indices[i])
		}

		fields = append(fields, line+" // "+field.Name)
	}

	tag := ""
	if progressive {
		tag = `ssz-type:"progressive-container"`
	}

	return e.declare(key, d.Kind, "struct {\n"+strings.Join(fields, "\n")+"\n}", tag), nil
}

// layout returns the layout position of each field of a progressive
// container. Go spells a layout as the position of each field, so a layout
// that is not exactly that has no spelling.
func layout(d *descriptor.Descriptor) ([]int, error) {
	if d.ActiveFields == nil {
		return nil, fmt.Errorf("%w: a progressive container without a field layout", errInexpressible)
	}

	indices := make([]int, 0, len(d.Fields))

	for position, bit := range d.ActiveFields {
		value, ok := bit.(float64)

		switch {
		case !ok || (value != 0 && value != 1):
			return nil, fmt.Errorf("%w: layout position %d is not a bit", errInexpressible, position)
		case value == 1:
			indices = append(indices, position)
		}
	}

	if len(indices) != len(d.Fields) {
		return nil, fmt.Errorf("%w: layout sets %d positions for %d fields", errInexpressible, len(indices), len(d.Fields))
	}

	if n := len(d.ActiveFields); n > 0 && d.ActiveFields[n-1] != float64(1) {
		return nil, fmt.Errorf("%w: layout ends on a gap, which no field position can state", errInexpressible)
	}

	return indices, nil
}

func (e *emitter) declareUnion(key string, d *descriptor.Descriptor) (string, error) {
	options := make([]string, 0, len(d.Options))

	for i, option := range d.Options {
		selector, ok := option.Selector.(float64)
		if !ok || selector != float64(int64(selector)) {
			return "", fmt.Errorf("%w: union selector %v is not an integer", errInexpressible, option.Selector)
		}

		goType, err := e.emit(option.Type)
		if err != nil {
			return "", err
		}

		options = append(options, fmt.Sprintf("\tO%d %s `ssz-index:\"%d\"`", i, goType, int64(selector)))
	}

	// The name is reserved before the options type is declared, so the two
	// stay next to each other in the numbering.
	name := fmt.Sprintf("%s%03d%s", e.prefix, len(e.names)+1, d.Kind)
	e.names[key] = name
	e.usesUnion = true

	e.decls = append(e.decls, fmt.Sprintf(
		"// %sOptions lists the options of %s.\ntype %sOptions struct {\n%s\n}\n\n// %s is a %s.\ntype %s = dynssz.CompatibleUnion[%sOptions]\n",
		name, name, name, strings.Join(options, "\n"), name, d.Kind, name, name))

	return name, nil
}

// count reads a declared length Go can use as an array length.
func count(value *int64) (int64, error) {
	switch {
	case value == nil:
		return 0, fmt.Errorf("declaration states no %s", countLength)
	case *value < 0:
		return 0, fmt.Errorf("%w: a negative %s", errInexpressible, countLength)
	default:
		return *value, nil
	}
}

// surplus refuses a count the shape has no use for: Go cannot state it.
func surplus(d *descriptor.Descriptor, value *int64, what string) error {
	if value != nil {
		return fmt.Errorf("%w: a %s declaring a %s", errInexpressible, d.Kind, what)
	}

	return nil
}

func header(e *emitter, extraImports ...string) string {
	imports := append([]string{}, extraImports...)
	if e.usesUnion {
		imports = append(imports, `dynssz "github.com/pk910/dynamic-ssz"`)
	}

	if e.usesAnnot {
		imports = append(imports, `"github.com/pk910/dynamic-ssz/sszutils"`)
	}

	sort.Strings(imports)

	var b strings.Builder

	b.WriteString("// Code generated by tests/sszspecs/gen. DO NOT EDIT.\n\npackage sszspecs\n\n")

	if len(imports) > 0 {
		b.WriteString("import (\n")

		for _, imp := range imports {
			fmt.Fprintf(&b, "\t%s\n", imp)
		}

		b.WriteString(")\n\n")
	}

	return b.String()
}

func writeTypes(path string, e *emitter, registry, unsupported map[string]string) error {
	var b strings.Builder

	b.WriteString(header(e))
	b.WriteString(strings.Join(e.decls, "\n"))
	b.WriteString("\nfunc init() {\n")

	for _, key := range sortedKeys(registry) {
		fmt.Fprintf(&b, "\tvectorTypes[%q] = func() any { return new(%s) }\n", key, registry[key])
	}

	for _, key := range sortedKeys(unsupported) {
		fmt.Fprintf(&b, "\tunsupportedTypes[%q] = %q\n", key, unsupported[key])
	}

	b.WriteString("}\n")

	return writeGo(path, b.String())
}

func writeIllegal(path string, e *emitter, registry, inexpressible map[string]string) error {
	var b strings.Builder

	extra := []string{}
	if len(registry) > 0 {
		extra = append(extra, `"reflect"`)
	}

	b.WriteString(header(e, extra...))
	b.WriteString(strings.Join(e.decls, "\n"))
	b.WriteString("\nfunc init() {\n")

	for _, id := range sortedKeys(registry) {
		fmt.Fprintf(&b, "\tillegalTypes[%q] = reflect.TypeFor[%s]()\n", id, registry[id])
	}

	for _, id := range sortedKeys(inexpressible) {
		fmt.Fprintf(&b, "\tinexpressibleTypes[%q] = %q\n", id, inexpressible[id])
	}

	b.WriteString("}\n")

	return writeGo(path, b.String())
}

func writeConfig(path string, types []string) error {
	var b strings.Builder

	b.WriteString("# Code generated by tests/sszspecs/gen. DO NOT EDIT.\n")
	fmt.Fprintf(&b, "package: %s\noutput: %s\nwith-streaming: true\n\ntypes:\n", packagePath, codeFile)

	for _, name := range types {
		fmt.Fprintf(&b, "  - %s\n", name)
	}

	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func writeGo(path, source string) error {
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return fmt.Errorf("format %s: %w\n%s", path, err, source)
	}

	return os.WriteFile(path, bytes.TrimLeft(formatted, "\n"), 0o600)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
