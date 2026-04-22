// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempConfig writes content to a file in tempDir and returns its path.
// Using a shared tempDir lets each test keep multiple related files (the
// config itself plus referenced outputs) side by side for realistic path
// resolution.
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gen.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func TestLoadConfig_FullForm(t *testing.T) {
	path := writeTempConfig(t, `
package: github.com/pk910/dynamic-ssz/codegen/tests
package-name: tests
output: default_ssz.go
verbose: true
legacy: true
without-dynamic-expressions: false
without-fastssz: true
with-streaming: true
with-extended-types: false

types:
  - name: BeaconBlock
    output: block_ssz.go
    views:
      - Phase0View
      - github.com/some/pkg.AltairView
  - name: BeaconState
    view-only: true
    views: [StateView]
    with-streaming: false
  - Shorthand
`)

	fc, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if fc.Package != "github.com/pk910/dynamic-ssz/codegen/tests" {
		t.Errorf("Package = %q", fc.Package)
	}
	if fc.PackageName != "tests" {
		t.Errorf("PackageName = %q", fc.PackageName)
	}
	if fc.Output != "default_ssz.go" {
		t.Errorf("Output = %q", fc.Output)
	}
	if fc.Verbose == nil || !*fc.Verbose {
		t.Error("expected Verbose=true")
	}
	if fc.Legacy == nil || !*fc.Legacy {
		t.Error("expected Legacy=true")
	}
	if fc.WithoutDynamicExpressions == nil || *fc.WithoutDynamicExpressions {
		t.Error("expected WithoutDynamicExpressions=false")
	}
	if len(fc.Types) != 3 {
		t.Fatalf("len(types) = %d", len(fc.Types))
	}

	if fc.Types[0].Name != "BeaconBlock" {
		t.Errorf("Types[0].Name = %q", fc.Types[0].Name)
	}
	if fc.Types[0].Output != "block_ssz.go" {
		t.Errorf("Types[0].Output = %q", fc.Types[0].Output)
	}
	if len(fc.Types[0].Views) != 2 || fc.Types[0].Views[0] != "Phase0View" {
		t.Errorf("Types[0].Views = %v", fc.Types[0].Views)
	}
	if fc.Types[1].ViewOnly != true {
		t.Error("expected Types[1].ViewOnly=true")
	}
	if fc.Types[1].WithStreaming == nil || *fc.Types[1].WithStreaming {
		t.Error("expected Types[1].WithStreaming=false override")
	}
	if fc.Types[2].Name != "Shorthand" {
		t.Errorf("Types[2].Name = %q (shorthand)", fc.Types[2].Name)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/definitely/does/not/exist.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "failed to open config file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "package: foo\n  indented-wrong: bar\n")
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "failed to parse config file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_UnknownField(t *testing.T) {
	path := writeTempConfig(t, `
package: x
types:
  - name: T
    view_only: true
`) // note: view_only (snake) instead of view-only (kebab) → should error
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "view_only") {
		t.Fatalf("expected error to name the unknown field, got: %v", err)
	}
}

func TestLoadConfig_ShorthandEmpty(t *testing.T) {
	path := writeTempConfig(t, `
package: x
output: out.go
types:
  - ""
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for empty shorthand string")
	}
	if !strings.Contains(err.Error(), "non-empty string") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_TypeEntryInvalidNode(t *testing.T) {
	// A sequence value is not a valid type entry (must be scalar or mapping).
	path := writeTempConfig(t, `
package: x
types:
  - [1, 2, 3]
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid type entry shape")
	}
	if !strings.Contains(err.Error(), "must be a string or mapping") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_TypeEntryFieldTypeMismatch(t *testing.T) {
	// Known field, but the value type is wrong (views must be a list, not a scalar).
	// Passes our manual key allowlist but then fails the struct decode.
	path := writeTempConfig(t, `
package: x
output: o.go
types:
  - name: T
    views: "NotAList"
`)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for wrong value type")
	}
}

func TestApplyToConfig_BaselineFromFile(t *testing.T) {
	path := writeTempConfig(t, `
package: fmt
package-name: mypkg
output: out.go
verbose: true
legacy: true
without-dynamic-expressions: true
without-fastssz: true
with-streaming: true
with-extended-types: true
types:
  - Stringer
`)
	fc, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	cfg := Config{}
	specs, err := fc.applyToConfig(&cfg, map[string]bool{}, filepath.Dir(path))
	if err != nil {
		t.Fatalf("applyToConfig: %v", err)
	}

	if cfg.PackagePath != "fmt" {
		t.Errorf("PackagePath = %q", cfg.PackagePath)
	}
	if cfg.PackageName != "mypkg" {
		t.Errorf("PackageName = %q", cfg.PackageName)
	}
	if !filepath.IsAbs(cfg.OutputFile) {
		t.Errorf("expected OutputFile resolved to absolute path, got %q", cfg.OutputFile)
	}
	if !strings.HasSuffix(cfg.OutputFile, "out.go") {
		t.Errorf("OutputFile = %q", cfg.OutputFile)
	}
	for name, b := range map[string]bool{
		"Verbose":                   cfg.Verbose,
		"Legacy":                    cfg.Legacy,
		"WithoutDynamicExpressions": cfg.WithoutDynamicExpressions,
		"WithoutFastSsz":            cfg.WithoutFastSsz,
		"WithStreaming":             cfg.WithStreaming,
		"WithExtendedTypes":         cfg.WithExtendedTypes,
	} {
		if !b {
			t.Errorf("expected cfg.%s=true", name)
		}
	}

	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d", len(specs))
	}
	spec := specs[0]
	if spec.TypeName != "Stringer" {
		t.Errorf("TypeName = %q", spec.TypeName)
	}
	if !spec.HasPerTypeOverrides {
		t.Error("expected HasPerTypeOverrides=true for config-file specs")
	}
	if !spec.Legacy || !spec.WithoutDynamicExpressions || !spec.WithoutFastSsz || !spec.WithStreaming || !spec.WithExtendedTypes {
		t.Errorf("expected effective per-type flags to inherit globals, got %+v", spec)
	}
	if spec.OutputFile != cfg.OutputFile {
		t.Errorf("expected spec to inherit default output, got %q", spec.OutputFile)
	}
}

func TestApplyToConfig_CLIOverrides(t *testing.T) {
	path := writeTempConfig(t, `
package: fmt
output: out.go
legacy: true
with-streaming: true
types:
  - Stringer
`)
	fc, _ := LoadConfig(path)

	cfg := Config{
		PackagePath: "github.com/foo/bar", // CLI gave a different package
		OutputFile:  "/cli/out.go",        // CLI provided an output
		Legacy:      false,                // CLI explicitly set -legacy=false
	}
	cliProvided := map[string]bool{
		"package": true,
		"output":  true,
		"legacy":  true,
	}

	specs, err := fc.applyToConfig(&cfg, cliProvided, filepath.Dir(path))
	if err != nil {
		t.Fatalf("applyToConfig: %v", err)
	}

	if cfg.PackagePath != "github.com/foo/bar" {
		t.Errorf("CLI package should win, got %q", cfg.PackagePath)
	}
	if cfg.OutputFile != "/cli/out.go" {
		t.Errorf("CLI output should win, got %q", cfg.OutputFile)
	}
	if cfg.Legacy {
		t.Error("CLI -legacy=false should win over file's legacy=true")
	}
	if !cfg.WithStreaming {
		t.Error("file with-streaming=true should apply (not CLI-provided)")
	}

	// Per-type flag should reflect effective merged booleans too.
	if specs[0].Legacy {
		t.Error("per-type effective Legacy should be false (CLI override)")
	}
	if !specs[0].WithStreaming {
		t.Error("per-type effective WithStreaming should be true")
	}
}

func TestApplyToConfig_CLIReplacesTypes(t *testing.T) {
	path := writeTempConfig(t, `
package: fmt
output: out.go
types:
  - Stringer
  - Other
`)
	fc, _ := LoadConfig(path)

	cfg := Config{TypeNames: "CLIProvided", OutputFile: "out.go"}
	specs, err := fc.applyToConfig(&cfg, map[string]bool{"types": true}, filepath.Dir(path))
	if err != nil {
		t.Fatalf("applyToConfig: %v", err)
	}
	if specs != nil {
		t.Errorf("when -types is CLI-provided, config specs should be ignored, got %v", specs)
	}
}

func TestApplyToConfig_PerTypeOverrides(t *testing.T) {
	path := writeTempConfig(t, `
package: fmt
output: out.go
legacy: true
with-streaming: true
types:
  - name: A
    legacy: false
    with-streaming: false
  - name: B   # inherits globals
  - name: C
    without-fastssz: true
`)
	fc, _ := LoadConfig(path)
	cfg := Config{}
	specs, err := fc.applyToConfig(&cfg, map[string]bool{}, filepath.Dir(path))
	if err != nil {
		t.Fatalf("applyToConfig: %v", err)
	}

	if specs[0].Legacy || specs[0].WithStreaming {
		t.Errorf("A should have overridden both flags off, got %+v", specs[0])
	}
	if !specs[1].Legacy || !specs[1].WithStreaming {
		t.Errorf("B should inherit globals, got %+v", specs[1])
	}
	if specs[1].WithoutFastSsz {
		t.Error("B should have WithoutFastSsz=false (global default)")
	}
	if !specs[2].WithoutFastSsz {
		t.Error("C should have WithoutFastSsz=true override")
	}
	if !specs[2].Legacy || !specs[2].WithStreaming {
		t.Error("C should still inherit the global legacy/streaming flags")
	}
}

func TestApplyToConfig_MissingTypeName(t *testing.T) {
	path := writeTempConfig(t, `
package: fmt
output: out.go
types:
  - output: foo.go
`)
	fc, _ := LoadConfig(path)
	cfg := Config{}
	_, err := fc.applyToConfig(&cfg, map[string]bool{}, filepath.Dir(path))
	if err == nil {
		t.Fatal("expected error for missing type name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyToConfig_MissingOutput(t *testing.T) {
	path := writeTempConfig(t, `
package: fmt
types:
  - Stringer
`)
	fc, _ := LoadConfig(path)
	cfg := Config{}
	_, err := fc.applyToConfig(&cfg, map[string]bool{}, filepath.Dir(path))
	if err == nil {
		t.Fatal("expected error for missing output")
	}
	if !strings.Contains(err.Error(), "output file not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyToConfig_NoTypes(t *testing.T) {
	path := writeTempConfig(t, `
package: fmt
output: out.go
`)
	fc, _ := LoadConfig(path)
	cfg := Config{}
	_, err := fc.applyToConfig(&cfg, map[string]bool{}, filepath.Dir(path))
	if err == nil {
		t.Fatal("expected error for empty types list")
	}
	if !strings.Contains(err.Error(), "at least one entry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyToConfig_AbsoluteOutputNotRewritten(t *testing.T) {
	tmpDir := t.TempDir()
	absOut := filepath.Join(tmpDir, "abs_out.go")
	path := writeTempConfig(t, `
package: fmt
output: `+absOut+`
types:
  - name: A
    output: `+absOut+`
`)
	fc, _ := LoadConfig(path)
	cfg := Config{}
	specs, err := fc.applyToConfig(&cfg, map[string]bool{}, filepath.Dir(path))
	if err != nil {
		t.Fatalf("applyToConfig: %v", err)
	}
	if cfg.OutputFile != absOut {
		t.Errorf("absolute output mangled: %q vs %q", cfg.OutputFile, absOut)
	}
	if specs[0].OutputFile != absOut {
		t.Errorf("absolute per-type output mangled: %q vs %q", specs[0].OutputFile, absOut)
	}
}

func TestResolvePackagePath(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		baseDir string
		want    string
	}{
		{"empty pkg stays empty", "", "/base", ""},
		{"empty base leaves as-is", "./foo", "", "./foo"},
		{"absolute path preserved", "/abs/pkg", "/base", "/abs/pkg"},
		{"import path left untouched", "github.com/foo/bar", "/base", "github.com/foo/bar"},
		{"dot resolves to baseDir", ".", "/base", "/base"},
		{"dotdot resolves to parent", "..", "/base/sub", "/base"},
		{"./sub joined", "./sub", "/base", "/base/sub"},
		{"../sibling joined", "../sibling", "/base/sub", "/base/sibling"},
		{"recursive pattern preserved", "./...", "/base", "/base/..."},
		{"single-segment non-path name untouched", "mypkg", "/base", "mypkg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePackagePath(tt.pkg, tt.baseDir)
			if got != tt.want {
				t.Errorf("resolvePackagePath(%q, %q) = %q, want %q", tt.pkg, tt.baseDir, got, tt.want)
			}
		})
	}
}

// End-to-end: a config with `package: ./sub` resolves against the config's
// directory, not the caller's CWD. Uses the real codegen/tests package via
// a relative ref from its parent directory so we can verify packages.Load
// actually accepts the rewritten path.
func TestApplyToConfig_RelativePackagePath(t *testing.T) {
	repoTests, err := filepath.Abs("../codegen/tests")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	baseDir := filepath.Dir(repoTests) // .../codegen

	fc := &FileConfig{Package: "./tests"}
	cfg := &Config{}
	// applyToConfig will error on the missing types list; that's fine — the
	// package path rewrite happens before the types check.
	if _, err := fc.applyToConfig(cfg, map[string]bool{}, baseDir); err != nil &&
		!strings.Contains(err.Error(), "at least one entry") {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(baseDir, "tests")
	if cfg.PackagePath != want {
		t.Errorf("PackagePath = %q, want %q", cfg.PackagePath, want)
	}
}

func TestApplyToConfig_ImportPathPackageUntouched(t *testing.T) {
	fc := &FileConfig{Package: "github.com/pk910/dynamic-ssz/codegen/tests"}
	cfg := &Config{}
	_, _ = fc.applyToConfig(cfg, map[string]bool{}, "/does/not/matter")
	if cfg.PackagePath != "github.com/pk910/dynamic-ssz/codegen/tests" {
		t.Errorf("import path should pass through untouched, got %q", cfg.PackagePath)
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		baseDir string
		want    string
	}{
		{"empty path stays empty", "", "/base", ""},
		{"empty base leaves as-is", "rel.go", "", "rel.go"},
		{"absolute path preserved", "/abs/out.go", "/base", "/abs/out.go"},
		{"relative gets joined", "sub/out.go", "/base", "/base/sub/out.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePath(tt.path, tt.baseDir)
			if got != tt.want {
				t.Errorf("resolvePath(%q, %q) = %q, want %q", tt.path, tt.baseDir, got, tt.want)
			}
		})
	}
}

func TestApplyBool(t *testing.T) {
	tests := []struct {
		name    string
		initial bool
		src     *bool
		cliSet  bool
		want    bool
	}{
		{"nil src leaves dst", false, nil, false, false},
		{"CLI set blocks file", false, boolPtr(true), true, false},
		{"file sets when CLI absent", false, boolPtr(true), false, true},
		{"file sets false when CLI absent", true, boolPtr(false), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := tt.initial
			applyBool(&dst, tt.src, tt.cliSet)
			if dst != tt.want {
				t.Errorf("dst = %v, want %v", dst, tt.want)
			}
		})
	}
}

func TestResolveBool(t *testing.T) {
	if resolveBool(nil, true) != true {
		t.Error("nil override should fall back")
	}
	if resolveBool(boolPtr(false), true) != false {
		t.Error("override should win over fallback")
	}
	if resolveBool(boolPtr(true), false) != true {
		t.Error("override should win over fallback")
	}
}

func TestCodegenFlagOptions(t *testing.T) {
	// Empty set produces no options.
	if opts := codegenFlagOptions(false, false, false, false, false); len(opts) != 0 {
		t.Errorf("expected 0 options for all-false, got %d", len(opts))
	}
	// Streaming emits two options (encoder + decoder).
	if opts := codegenFlagOptions(false, false, false, true, false); len(opts) != 2 {
		t.Errorf("expected 2 options for streaming, got %d", len(opts))
	}
	// All flags on → 6 options total (legacy, no-dyn, no-fastssz, encoder, decoder, extended).
	if opts := codegenFlagOptions(true, true, true, true, true); len(opts) != 6 {
		t.Errorf("expected 6 options, got %d", len(opts))
	}
}

func TestAnyHasOverrides(t *testing.T) {
	if anyHasOverrides(nil) {
		t.Error("nil slice: expected false")
	}
	if anyHasOverrides([]typeSpec{{TypeName: "A"}}) {
		t.Error("no overrides: expected false")
	}
	specs := []typeSpec{
		{TypeName: "A"},
		{TypeName: "B", HasPerTypeOverrides: true},
	}
	if !anyHasOverrides(specs) {
		t.Error("expected true when one spec has overrides")
	}
}

// TestProvidedFlagSet verifies flag.Visit-based detection of explicit flags.
func TestProvidedFlagSet(t *testing.T) {
	oldCmd := flag.CommandLine
	defer func() { flag.CommandLine = oldCmd }()

	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	_ = flag.String("foo", "", "")
	_ = flag.Bool("bar", false, "")
	_ = flag.Bool("baz", false, "")
	if err := flag.CommandLine.Parse([]string{"-foo", "x", "-bar=true"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := providedFlagSet()
	if !got["foo"] || !got["bar"] {
		t.Errorf("expected foo and bar, got %v", got)
	}
	if got["baz"] {
		t.Errorf("baz should not be marked as provided")
	}
}

// End-to-end: main() --config path loads and runs successfully.
func TestMain_ConfigFlag_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen.go")
	cfgPath := filepath.Join(tmpDir, "gen.yaml")
	cfg := `package: github.com/pk910/dynamic-ssz/codegen/tests
package-name: tests
output: ` + outFile + `
types:
  - SimpleTypes1
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldArgs := os.Args
	oldCmd := flag.CommandLine
	defer func() { os.Args = oldArgs; flag.CommandLine = oldCmd }()

	flag.CommandLine = flag.NewFlagSet("dynssz-gen", flag.ContinueOnError)
	os.Args = []string{"dynssz-gen", "-config", cfgPath}
	main()

	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("expected output file generated: %v", err)
	}
}

// TestRun_ConfigPathVerbose covers the verbose-logging branch taken when
// run() was invoked via the config-file path (TypeSpecs populated).
func TestRun_ConfigPathVerbose(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen.go")
	runCfg := Config{
		PackagePath: "github.com/pk910/dynamic-ssz/codegen/tests",
		PackageName: "tests",
		Verbose:     true,
		TypeSpecs: []typeSpec{{
			TypeName:            "SimpleTypes1",
			OutputFile:          outFile,
			HasPerTypeOverrides: true,
			Legacy:              true,
		}},
	}
	if err := run(&runCfg); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("expected output: %v", err)
	}
}

// End-to-end: per-type override flips a global flag off for a specific type.
func TestRun_ConfigWithPerTypeOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "gen.go")
	cfgPath := filepath.Join(tmpDir, "gen.yaml")
	cfg := `package: github.com/pk910/dynamic-ssz/codegen/tests
package-name: tests
output: ` + outFile + `
legacy: true
types:
  - name: SimpleTypes1
    legacy: false
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	fc, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	runCfg := Config{}
	specs, err := fc.applyToConfig(&runCfg, map[string]bool{}, filepath.Dir(cfgPath))
	if err != nil {
		t.Fatalf("applyToConfig: %v", err)
	}
	runCfg.TypeSpecs = specs

	if runErr := run(&runCfg); runErr != nil {
		t.Fatalf("run: %v", runErr)
	}

	// With Legacy=false overridden at the type level, the generated file
	// should contain only the dynamic marshal method, not a legacy one.
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "MarshalSSZDyn") {
		t.Error("expected MarshalSSZDyn in output")
	}
	// The legacy (non-Dyn) MarshalSSZ method should NOT appear because we
	// overrode -legacy off for this type.
	if strings.Contains(content, "func (") && strings.Contains(content, ") MarshalSSZ() (") {
		t.Error("expected no legacy MarshalSSZ method when override disables it")
	}
}

// End-to-end: config with an unknown field causes main() to log.Fatal.
// Uses the subprocess technique used elsewhere in this package (the child
// re-runs itself with TEST_* env set and exits non-zero from log.Fatal).
func TestMain_ConfigFlag_LoadError(t *testing.T) {
	if os.Getenv("TEST_DYNSSZ_MAIN_CFG_LOAD_ERR") == "1" {
		cfgPath := os.Getenv("TEST_DYNSSZ_CFG_PATH")
		flag.CommandLine = flag.NewFlagSet("dynssz-gen", flag.ContinueOnError)
		os.Args = []string{"dynssz-gen", "-config", cfgPath}
		main() // log.Fatal → os.Exit(1)
		return
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	if err := os.WriteFile(cfgPath, []byte("unknown-field: 123\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_ConfigFlag_LoadError$") //nolint:gosec // G204: test binary path, controlled input
	cmd.Env = append(os.Environ(), "TEST_DYNSSZ_MAIN_CFG_LOAD_ERR=1", "TEST_DYNSSZ_CFG_PATH="+cfgPath)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit from main() with bad config")
	}
}

// End-to-end: valid YAML but applyToConfig fails (missing type name).
func TestMain_ConfigFlag_ApplyError(t *testing.T) {
	if os.Getenv("TEST_DYNSSZ_MAIN_CFG_APPLY_ERR") == "1" {
		cfgPath := os.Getenv("TEST_DYNSSZ_CFG_PATH")
		flag.CommandLine = flag.NewFlagSet("dynssz-gen", flag.ContinueOnError)
		os.Args = []string{"dynssz-gen", "-config", cfgPath}
		main()
		return
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	content := `package: fmt
output: out.go
types:
  - output: x.go
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_ConfigFlag_ApplyError$") //nolint:gosec // G204: test binary path, controlled input
	cmd.Env = append(os.Environ(), "TEST_DYNSSZ_MAIN_CFG_APPLY_ERR=1", "TEST_DYNSSZ_CFG_PATH="+cfgPath)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit from main() with bad config apply")
	}
}

func boolPtr(b bool) *bool { return &b }
