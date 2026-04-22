// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pk910/dynamic-ssz/codegen"
	"gopkg.in/yaml.v3"
)

// FileConfig is the on-disk YAML schema for dynssz-gen's --config flag.
//
// All fields are optional at the YAML layer; required-field enforcement happens
// after CLI overrides are merged (so a required value can come from either
// side). Boolean fields use pointers so "unset" is distinguishable from
// "set to false" — that distinction is what makes per-type overrides work.
type FileConfig struct {
	Package     string `yaml:"package"`
	PackageName string `yaml:"package-name"`
	Output      string `yaml:"output"`
	Verbose     *bool  `yaml:"verbose"`

	Legacy                    *bool `yaml:"legacy"`
	WithoutDynamicExpressions *bool `yaml:"without-dynamic-expressions"`
	WithoutFastSsz            *bool `yaml:"without-fastssz"`
	WithStreaming             *bool `yaml:"with-streaming"`
	WithExtendedTypes         *bool `yaml:"with-extended-types"`

	Types []TypeEntry `yaml:"types"`
}

// TypeEntry describes one type in the `types:` list.
//
// Each entry may be provided either as a bare string (shorthand — just the
// type name, all other options default to the file-level values) or as a
// mapping. See UnmarshalYAML for the shorthand handling.
//
// The per-type codegen flag fields (Legacy, WithoutDynamicExpressions, …)
// override the file-level values when set. Because codegen's With* options
// only set booleans to true (never false), overrides are resolved into an
// effective boolean per type and applied only at the per-type level — never
// at the file level — so that a type can opt *out* of a globally enabled
// flag.
type TypeEntry struct {
	Name     string   `yaml:"name"`
	Output   string   `yaml:"output"`
	Views    []string `yaml:"views"`
	ViewOnly bool     `yaml:"view-only"`

	Legacy                    *bool `yaml:"legacy"`
	WithoutDynamicExpressions *bool `yaml:"without-dynamic-expressions"`
	WithoutFastSsz            *bool `yaml:"without-fastssz"`
	WithStreaming             *bool `yaml:"with-streaming"`
	WithExtendedTypes         *bool `yaml:"with-extended-types"`
}

// UnmarshalYAML accepts either a scalar (string) or a mapping for each type
// entry, so users can write shorthand entries:
//
//	types:
//	  - SimpleType                 # shorthand
//	  - name: OtherType            # full form
//	    output: other_ssz.go
func (t *TypeEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		name := strings.TrimSpace(node.Value)
		if name == "" {
			return fmt.Errorf("line %d: type entry shorthand must be a non-empty string", node.Line)
		}
		t.Name = name
		return nil
	}

	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: type entry must be a string or mapping", node.Line)
	}

	// yaml.v3's KnownFields strict-mode flag lives on the Decoder, not on the
	// node. Node.Decode() spins up a fresh internal decoder that doesn't
	// inherit the setting, so we have to check unknown keys ourselves to
	// keep parity with the top-level parser.
	knownKeys := map[string]bool{
		"name": true, "output": true, "views": true, "view-only": true,
		"legacy":                      true,
		"without-dynamic-expressions": true,
		"without-fastssz":             true,
		"with-streaming":              true,
		"with-extended-types":         true,
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !knownKeys[key] {
			return fmt.Errorf("line %d: unknown field %q in type entry", node.Content[i].Line, key)
		}
	}

	type typeEntryAlias TypeEntry
	var alias typeEntryAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*t = TypeEntry(alias)
	return nil
}

// LoadConfig parses a YAML config file from the given path.
//
// Unknown keys are rejected (strict mode) so that typos like `view_only`
// surface as errors instead of being silently ignored.
func LoadConfig(path string) (*FileConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file %s: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	cfg := &FileConfig{}
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}
	return cfg, nil
}

// applyToConfig merges the file config into the CLI-shaped Config.
//
// cliProvided tells which CLI flags were explicitly set (via flag.Visit).
// Explicitly-set CLI flags override the file config; otherwise the file
// config wins over the zero value that the flag package defaults to.
//
// baseDir is used to resolve relative output paths against the config file's
// directory (not CWD), which keeps configs portable across invocations.
func (fc *FileConfig) applyToConfig(cfg *Config, cliProvided map[string]bool, baseDir string) ([]typeSpec, error) {
	// Scalars: CLI wins if provided, otherwise take the file value.
	if !cliProvided["package"] && fc.Package != "" {
		cfg.PackagePath = resolvePackagePath(fc.Package, baseDir)
	}
	if !cliProvided["package-name"] && fc.PackageName != "" {
		cfg.PackageName = fc.PackageName
	}
	if !cliProvided["output"] && fc.Output != "" {
		cfg.OutputFile = resolvePath(fc.Output, baseDir)
	}

	// Booleans: CLI wins if provided, otherwise take file value (if set).
	applyBool(&cfg.Verbose, fc.Verbose, cliProvided["v"])
	applyBool(&cfg.Legacy, fc.Legacy, cliProvided["legacy"])
	applyBool(&cfg.WithoutDynamicExpressions, fc.WithoutDynamicExpressions, cliProvided["without-dynamic-expressions"])
	applyBool(&cfg.WithoutFastSsz, fc.WithoutFastSsz, cliProvided["without-fastssz"])
	applyBool(&cfg.WithStreaming, fc.WithStreaming, cliProvided["with-streaming"])
	applyBool(&cfg.WithExtendedTypes, fc.WithExtendedTypes, cliProvided["with-extended-types"])

	// Types: if the CLI provided -types, it fully replaces the file list.
	// Merging the two was considered but felt confusing — users that want to
	// combine should just edit the file. The specs are parsed by the caller
	// in that case and this function returns nil.
	if cliProvided["types"] {
		return nil, nil
	}

	specs := make([]typeSpec, 0, len(fc.Types))
	for i, entry := range fc.Types {
		if entry.Name == "" {
			return nil, fmt.Errorf("types[%d]: name is required", i)
		}

		spec := typeSpec{
			TypeName:   entry.Name,
			OutputFile: entry.Output,
			ViewTypes:  append([]string(nil), entry.Views...),
			IsViewOnly: entry.ViewOnly,
		}
		if spec.OutputFile != "" {
			spec.OutputFile = resolvePath(spec.OutputFile, baseDir)
		} else {
			if cfg.OutputFile == "" {
				return nil, fmt.Errorf("types[%d] (%s): output file not set and no top-level output", i, entry.Name)
			}
			spec.OutputFile = cfg.OutputFile
		}

		spec.Legacy = resolveBool(entry.Legacy, cfg.Legacy)
		spec.WithoutDynamicExpressions = resolveBool(entry.WithoutDynamicExpressions, cfg.WithoutDynamicExpressions)
		spec.WithoutFastSsz = resolveBool(entry.WithoutFastSsz, cfg.WithoutFastSsz)
		spec.WithStreaming = resolveBool(entry.WithStreaming, cfg.WithStreaming)
		spec.WithExtendedTypes = resolveBool(entry.WithExtendedTypes, cfg.WithExtendedTypes)
		spec.HasPerTypeOverrides = true

		specs = append(specs, spec)
	}

	if len(specs) == 0 {
		return nil, errors.New("config must specify at least one entry in `types`")
	}

	return specs, nil
}

// applyBool copies src into dst, unless the CLI already explicitly set dst
// (in which case src is ignored). Nil src means "file config didn't specify".
func applyBool(dst, src *bool, cliSet bool) {
	if cliSet || src == nil {
		return
	}
	*dst = *src
}

// resolveBool returns override if non-nil, otherwise the fallback value.
// Used to compute per-type effective booleans from optional overrides.
func resolveBool(override *bool, fallback bool) bool {
	if override != nil {
		return *override
	}
	return fallback
}

// resolvePath returns path if absolute, otherwise joined against baseDir.
// An empty baseDir means paths are used as-is.
func resolvePath(path, baseDir string) string {
	if path == "" || baseDir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// resolvePackagePath resolves a go/packages.Load pattern relative to the
// config file's directory — but only when the pattern looks like a
// filesystem path. Go package patterns split into three categories:
//
//  1. Import paths (e.g. "github.com/foo/bar") — left untouched, since
//     joining them with a directory would turn a valid import path into a
//     nonexistent on-disk path.
//  2. Filesystem paths — anything starting with "./", "../", or equal to
//     "." / ".." / absolute paths. These are the ones we normalize.
//  3. "all"/"std" and similar meta-patterns — caught by the import-path
//     branch (unchanged).
//
// The distinction matches the go tool's own rule (see `go help packages`):
// "A pattern containing a filesystem path starts with './', '../', or '/'".
// Recursive patterns such as "./..." work because filepath.Join preserves
// the trailing "...".
func resolvePackagePath(pkg, baseDir string) string {
	if pkg == "" || baseDir == "" || filepath.IsAbs(pkg) {
		return pkg
	}
	if pkg == "." || pkg == ".." ||
		strings.HasPrefix(pkg, "./") || strings.HasPrefix(pkg, "../") {
		return filepath.Join(baseDir, pkg)
	}
	return pkg
}

// codegenFlagOptions turns a set of effective codegen booleans into the
// corresponding codegen.CodeGeneratorOption values. Only true-valued flags
// emit options — codegen's With* helpers can only set booleans to true, so
// "false" is represented by the absence of an option.
func codegenFlagOptions(legacy, noDynExpr, noFastSsz, streaming, extended bool) []codegen.CodeGeneratorOption {
	var opts []codegen.CodeGeneratorOption
	if legacy {
		opts = append(opts, codegen.WithCreateLegacyFn())
	}
	if noDynExpr {
		opts = append(opts, codegen.WithoutDynamicExpressions())
	}
	if noFastSsz {
		opts = append(opts, codegen.WithNoFastSsz())
	}
	if streaming {
		opts = append(opts, codegen.WithCreateEncoderFn(), codegen.WithCreateDecoderFn())
	}
	if extended {
		opts = append(opts, codegen.WithExtendedTypes())
	}
	return opts
}

// anyHasOverrides reports whether any spec in the slice carries per-type
// codegen flag overrides. Used to decide whether file-level flags should
// still be applied (they should be for the legacy CLI path, but not when
// the config file has already fully resolved per-type flags).
func anyHasOverrides(specs []typeSpec) bool {
	for _, s := range specs {
		if s.HasPerTypeOverrides {
			return true
		}
	}
	return false
}
