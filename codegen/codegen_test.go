package codegen

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
)

var (
	combinedCoverageFile = "../coverage_codegen.out"
	coverageMutex        sync.Mutex
)

// TestPayload represents a single test case for code generation
type TestPayload struct {
	Name   string         // Test name
	Type   reflect.Type   // Go type to generate code for
	SSZHex string         // SSZ encoding as hex string (without 0x prefix)
	Specs  map[string]any // Dynamic specifications
}

// TestResult represents the result of testing a single code path
type TestResult struct {
	Method       string `json:"method"`
	HashTreeRoot string `json:"hashTreeRoot"`
	SSZEqual     bool   `json:"sszEqual"`
	Error        string `json:"error,omitempty"`
}

// TestResults represents results from all code paths
type TestResults struct {
	Legacy  *TestResult `json:"legacy,omitempty"`
	Dynamic *TestResult `json:"dynamic,omitempty"`
	Hybrid  *TestResult `json:"hybrid,omitempty"`
}

// Test matrix containing various payload types and their expected SSZ encodings
var testMatrix = []TestPayload{
	{
		Name: "SimpleStruct",
		Type: reflect.TypeOf(struct {
			ID     uint64
			Active bool
		}{}),
		SSZHex: "393000000000000001", // ID: 12345, Active: true
		Specs:  map[string]any{},
	},
	{
		Name: "DynamicSlice",
		Type: reflect.TypeOf(struct {
			Items []byte `ssz-max:"64"`
		}{}),
		SSZHex: "0400000068656c6c6f20776f726c64", // "hello world"
		Specs:  map[string]any{},
	},
	{
		Name: "FixedArray",
		Type: reflect.TypeOf(struct {
			Hash [32]byte
			Size uint64
		}{}),
		SSZHex: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f200004000000000000", // Hash: 1-32, Size: 1024
		Specs:  map[string]any{},
	},
	{
		Name: "DynamicSpecStruct",
		Type: reflect.TypeOf(struct {
			Data []byte `ssz-max:"1024" dynssz-max:"MAX_SIZE"`
		}{}),
		SSZHex: "04000000deadbeef", // Data: [0xde, 0xad, 0xbe, 0xef]
		Specs:  map[string]any{"MAX_SIZE": uint64(512)},
	},
	{
		Name: "NestedStruct",
		Type: reflect.TypeOf(struct {
			Inner struct {
				Value uint32
				Flag  bool
			}
			Count uint64
		}{}),
		SSZHex: "7856341201e703000000000000", // Inner.Value: 0x12345678, Inner.Flag: true, Count: 999
		Specs:  map[string]any{},
	},
}

func TestCodegenGeneration(t *testing.T) {
	for _, payload := range testMatrix {
		t.Run(payload.Name, func(t *testing.T) {
			testCodegenPayload(t, payload)
		})
	}
}

func testCodegenPayload(t *testing.T, payload TestPayload) {
	// Create temporary directory for this test
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("dynssz_codegen_test_%s_", payload.Name))
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Step 1: Create types directory and write the type definition
	typesDir := filepath.Join(tempDir, "types")
	if err := os.MkdirAll(typesDir, 0755); err != nil {
		t.Fatalf("Failed to create types dir: %v", err)
	}

	typeFile := filepath.Join(typesDir, "types.go")
	if err := writeTypeFile(typeFile, payload); err != nil {
		t.Fatalf("Failed to write type file: %v", err)
	}

	// Step 2: Write the code generator
	codegenDir := filepath.Join(tempDir, "codegen")
	if err := os.MkdirAll(codegenDir, 0755); err != nil {
		t.Fatalf("Failed to create codegen dir: %v", err)
	}

	codegenFile := filepath.Join(codegenDir, "main.go")
	if err := writeCodegenFile(codegenFile, payload); err != nil {
		t.Fatalf("Failed to write codegen file: %v", err)
	}

	// Step 3: Write the test main.go
	mainFile := filepath.Join(tempDir, "main.go")
	if err := writeMainFile(mainFile, payload); err != nil {
		t.Fatalf("Failed to write main file: %v", err)
	}

	// Step 4: Write go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	if err := writeGoMod(goModFile, payload.Name); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	// Step 5: Run code generation
	if err := runCodegen(tempDir); err != nil {
		t.Fatalf("Code generation failed: %v", err)
	}

	// Check if generated file exists
	generatedFile := filepath.Join(tempDir, "types", "generated_ssz.go")
	if _, err := os.Stat(generatedFile); err != nil {
		t.Logf("Generated file not found: %v", err)
	}

	// Step 6: Run the test and capture output
	output, err := runTest(tempDir, payload.SSZHex)
	//fmt.Printf("Test output:\n%s", output)

	if err != nil {
		t.Fatalf("Test execution failed: %v", err)
	}

	// Step 7: Verify results with reflection
	if err := verifyResults(t, payload, output); err != nil {
		t.Fatalf("Result verification failed: %v", err)
	}
}

func writeTypeFile(filename string, payload TestPayload) error {
	content := fmt.Sprintf(`package types

// TestPayload represents the test type
type TestPayload %s

// GetSpecs returns the dynamic specifications
func GetSpecs() map[string]any {
	return %#v
}
`,
		formatTypeString(payload.Type),
		payload.Specs,
	)

	return os.WriteFile(filename, []byte(content), 0644)
}

func writeCodegenFile(filename string, payload TestPayload) error {
	content := fmt.Sprintf(`package main

import (
	"log"
	"path/filepath"
	"reflect"
	"runtime"
	
	"github.com/pk910/dynamic-ssz/codegen"
	"codegen_test_%s/types"
)

func main() {
	// Create a code generator instance
	generator := codegen.NewCodeGenerator(nil)

	_, filePath, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filePath)
	
	// Generate SSZ methods for TestPayload type
	generator.BuildFile(
		currentDir+"/../types/generated_ssz.go",
		codegen.WithType(reflect.TypeOf(&types.TestPayload{})),
		codegen.WithCreateLegacyFn(), // Generate both dynamic and legacy methods
	)
	
	// Generate the code
	if err := generator.Generate(); err != nil {
		log.Fatal("Code generation failed:", err)
	}
	
	log.Println("Code generation completed successfully!")
}
`, payload.Name)

	return os.WriteFile(filename, []byte(content), 0644)
}

func writeMainFile(filename string, payload TestPayload) error {
	content := fmt.Sprintf(`package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
	"codegen_test_%s/types"
)

type TestResult struct {
	Method         string `+"`"+`json:"method"`+"`"+`
	HashTreeRoot   string `+"`"+`json:"hashTreeRoot"`+"`"+`
	SSZEqual       bool   `+"`"+`json:"sszEqual"`+"`"+`
	Error          string `+"`"+`json:"error,omitempty"`+"`"+`
}

type TestResults struct {
	Legacy  *TestResult `+"`"+`json:"legacy,omitempty"`+"`"+`
	Dynamic *TestResult `+"`"+`json:"dynamic,omitempty"`+"`"+`
	Hybrid  *TestResult `+"`"+`json:"hybrid,omitempty"`+"`"+`
}

func testCodePath(method string, testUnmarshal func() error, testMarshal func() ([]byte, error), testHashRoot func() ([32]byte, error), expectedSSZ []byte) *TestResult {
	result := &TestResult{Method: method}
	
	// Test unmarshal
	if err := testUnmarshal(); err != nil {
		result.Error = fmt.Sprintf("unmarshal error: %%v", err)
		return result
	}
	
	// Test marshal
	marshalData, err := testMarshal()
	if err != nil {
		result.Error = fmt.Sprintf("marshal error: %%v", err)
		return result
	}
	
	// Check SSZ equality
	result.SSZEqual = bytes.Equal(marshalData, expectedSSZ)
	
	// Test hash tree root
	hashRoot, err := testHashRoot()
	if err != nil {
		result.Error = fmt.Sprintf("hash tree root error: %%v", err)
		return result
	}
	
	result.HashTreeRoot = hex.EncodeToString(hashRoot[:])
	return result
}

func main() {
	// Get SSZ hex from command line
	if len(os.Args) != 2 {
		log.Fatal("Usage: program <ssz-hex>")
	}
	
	sszHex := os.Args[1]
	expectedSSZ, err := hex.DecodeString(sszHex)
	if err != nil {
		log.Fatalf("Invalid SSZ hex: %%v", err)
	}
	
	// Get specs
	specs := types.GetSpecs()
	dynssz.SetGlobalSpecs(specs)
	
	// Create DynSSZ instance
	ds := dynssz.NewDynSsz(specs)
	
	results := &TestResults{}
	
	// Test Legacy code path (if available)
	if _, ok := interface{}((*types.TestPayload)(nil)).(sszutils.FastsszMarshaler); ok {
		var legacyValue types.TestPayload
		results.Legacy = testCodePath("legacy",
			func() error {
				return legacyValue.UnmarshalSSZ(expectedSSZ)
			},
			func() ([]byte, error) {
				return legacyValue.MarshalSSZ()
			},
			func() ([32]byte, error) {
				return legacyValue.HashTreeRoot()
			},
			expectedSSZ,
		)
	}
	
	// Test Dynamic code path (if available)
	if _, ok := interface{}((*types.TestPayload)(nil)).(sszutils.DynamicMarshaler); ok {
		var dynamicValue types.TestPayload
		results.Dynamic = testCodePath("dynamic",
			func() error {
				return dynamicValue.UnmarshalSSZDyn(ds, expectedSSZ)
			},
			func() ([]byte, error) {
				buf := make([]byte, 0)
				return dynamicValue.MarshalSSZDyn(ds, buf)
			},
			func() ([32]byte, error) {
				return dynamicValue.HashTreeRootDyn(ds)
			},
			expectedSSZ,
		)
	}
	
	// Test Hybrid/Reflection code path (always available)
	var hybridValue types.TestPayload
	results.Hybrid = testCodePath("hybrid",
		func() error {
			return ds.UnmarshalSSZ(&hybridValue, expectedSSZ)
		},
		func() ([]byte, error) {
			return ds.MarshalSSZ(&hybridValue)
		},
		func() ([32]byte, error) {
			return ds.HashTreeRoot(&hybridValue)
		},
		expectedSSZ,
	)
	
	// Output results as JSON
	output, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal results: %%v", err)
	}
	
	fmt.Println(string(output))
}
`, payload.Name)

	return os.WriteFile(filename, []byte(content), 0644)
}

func writeGoMod(filename, testName string) error {
	// Get absolute path to dynamic-ssz root
	_, filePath, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filePath)
	rootPath := currentDir + "/../"

	content := fmt.Sprintf(`module codegen_test_%s

go 1.21

require github.com/pk910/dynamic-ssz v0.0.0

replace github.com/pk910/dynamic-ssz => %s
`, testName, rootPath)

	return os.WriteFile(filename, []byte(content), 0644)
}

func runCodegen(tempDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First, initialize go module in the temp directory
	initCmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	initCmd.Dir = tempDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy failed: %v\nOutput: %s", err, string(output))
	}

	// Build codegen binary with coverage
	codegenBinary := filepath.Join(tempDir, "codegen_executable")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-cover", "-coverpkg=...", "-covermode=atomic", "-o", codegenBinary, "./codegen")
	buildCmd.Dir = tempDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("codegen build failed: %v\nOutput: %s", err, string(output))
	}

	// Run codegen with coverage collection
	coverDir := filepath.Join(tempDir, "codegen_coverage")
	os.MkdirAll(coverDir, 0755)

	cmd := exec.CommandContext(ctx, codegenBinary)
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("GOCOVERDIR=%s", coverDir))
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("codegen execution failed: %v\nOutput: %s", err, string(output))
	}

	// Convert codegen coverage to text format and append
	coverFile := filepath.Join(tempDir, "codegen_coverage.out")
	convertCmd := exec.CommandContext(ctx, "go", "tool", "covdata", "textfmt",
		"-i="+coverDir, "-o="+coverFile)
	if convertErr := convertCmd.Run(); convertErr != nil {
		// Log but don't fail - coverage conversion is optional
		fmt.Printf("Warning: Codegen coverage conversion failed: %v\n", convertErr)
	} else {
		// Append to global coverage file
		appendCoverageFile(coverFile)
	}

	return nil
}

func runTest(tempDir string, sszHex string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build test executable (no coverage needed for test execution)
	testBinary := filepath.Join(tempDir, "test_executable")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", testBinary, ".")
	buildCmd.Dir = tempDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("test build failed: %v\nOutput: %s", err, string(output))
	}

	// Run test executable
	cmd := exec.CommandContext(ctx, testBinary, sszHex)
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("test execution failed: %v\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

func verifyResults(t *testing.T, payload TestPayload, output string) error {
	// Parse JSON output
	var results TestResults
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		return fmt.Errorf("failed to parse JSON output: %v", err)
	}

	// Calculate expected hash tree root using reflection
	ds := dynssz.NewDynSsz(payload.Specs)
	sszBytes, err := hex.DecodeString(payload.SSZHex)
	if err != nil {
		return fmt.Errorf("failed to decode SSZ hex: %v", err)
	}

	testValue := reflect.New(payload.Type).Interface()
	if err := ds.UnmarshalSSZ(testValue, sszBytes); err != nil {
		return fmt.Errorf("failed to unmarshal test data: %v", err)
	}

	expectedRoot, err := ds.HashTreeRoot(testValue)
	if err != nil {
		return fmt.Errorf("failed to calculate expected hash tree root: %v", err)
	}
	expectedHashHex := hex.EncodeToString(expectedRoot[:])

	// Verify at least one code path succeeded
	hasSuccess := false
	var testedMethods []string

	// Check legacy results
	if results.Legacy != nil {
		testedMethods = append(testedMethods, "legacy")
		if results.Legacy.Error == "" {
			hasSuccess = true
			if !results.Legacy.SSZEqual {
				return fmt.Errorf("legacy: SSZ encoding mismatch")
			}
			if results.Legacy.HashTreeRoot != expectedHashHex {
				return fmt.Errorf("legacy: hash mismatch: generated=%s, expected=%s", results.Legacy.HashTreeRoot, expectedHashHex)
			}
		}
	}

	// Check dynamic results
	if results.Dynamic != nil {
		testedMethods = append(testedMethods, "dynamic")
		if results.Dynamic.Error == "" {
			hasSuccess = true
			if !results.Dynamic.SSZEqual {
				return fmt.Errorf("dynamic: SSZ encoding mismatch")
			}
			if results.Dynamic.HashTreeRoot != expectedHashHex {
				return fmt.Errorf("dynamic: hash mismatch: generated=%s, expected=%s", results.Dynamic.HashTreeRoot, expectedHashHex)
			}
		}
	}

	// Check hybrid results (should always be present)
	if results.Hybrid != nil {
		testedMethods = append(testedMethods, "hybrid")
		if results.Hybrid.Error == "" {
			hasSuccess = true
			if !results.Hybrid.SSZEqual {
				return fmt.Errorf("hybrid: SSZ encoding mismatch")
			}
			if results.Hybrid.HashTreeRoot != expectedHashHex {
				return fmt.Errorf("hybrid: hash mismatch: generated=%s, expected=%s", results.Hybrid.HashTreeRoot, expectedHashHex)
			}
		}
	} else {
		return fmt.Errorf("hybrid results missing - should always be available")
	}

	if !hasSuccess {
		return fmt.Errorf("all code paths failed")
	}

	// Verify all successful paths have matching hash tree roots
	if results.Legacy != nil && results.Legacy.Error == "" && results.Dynamic != nil && results.Dynamic.Error == "" {
		if results.Legacy.HashTreeRoot != results.Dynamic.HashTreeRoot {
			return fmt.Errorf("hash tree root mismatch between legacy and dynamic: legacy=%s, dynamic=%s",
				results.Legacy.HashTreeRoot, results.Dynamic.HashTreeRoot)
		}
	}

	t.Logf("✅ Test %s passed - Methods tested: %v, SSZ and hash tree roots match", payload.Name, testedMethods)
	return nil
}

func formatTypeString(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Struct:
		var fields []string
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldType := formatTypeString(field.Type)
			tag := ""
			if field.Tag != "" {
				tag = fmt.Sprintf(" `%s`", field.Tag)
			}
			fields = append(fields, fmt.Sprintf("\t%s %s%s", field.Name, fieldType, tag))
		}
		return fmt.Sprintf("struct {\n%s\n}", strings.Join(fields, "\n"))
	case reflect.Slice:
		return fmt.Sprintf("[]%s", formatTypeString(t.Elem()))
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), formatTypeString(t.Elem()))
	default:
		return t.String()
	}
}

// appendCoverageFile appends coverage data from a test run to the combined coverage file
func appendCoverageFile(coverFile string) error {
	coverageMutex.Lock()
	defer coverageMutex.Unlock()

	_, filePath, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filePath)

	// Read the new coverage data
	newData, err := os.Open(coverFile)
	if err != nil {
		return fmt.Errorf("failed to open coverage file %s: %v", coverFile, err)
	}
	defer newData.Close()

	// Check if combined coverage file exists
	combinedFile := filepath.Join(currentDir, combinedCoverageFile)
	var combined *os.File

	if _, err := os.Stat(combinedFile); os.IsNotExist(err) {
		// Create new combined file
		combined, err = os.Create(combinedFile)
		if err != nil {
			return fmt.Errorf("failed to create combined coverage file: %v", err)
		}
		defer combined.Close()

		// Write header
		fmt.Fprintln(combined, "mode: set")
	} else {
		// Append to existing file
		combined, err = os.OpenFile(combinedFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open combined coverage file: %v", err)
		}
		defer combined.Close()
	}

	// Copy coverage data (skip "mode: set" line from new file)
	scanner := bufio.NewScanner(newData)
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		// Skip mode line from individual coverage files
		if firstLine && strings.HasPrefix(line, "mode:") {
			firstLine = false
			continue
		}
		firstLine = false

		// Only add non-empty lines and filter out temporary test packages
		if strings.TrimSpace(line) != "" {
			// Only include coverage from github.com/pk910/dynamic-ssz, exclude temporary test packages
			if strings.Contains(line, "github.com/pk910/dynamic-ssz") && !strings.Contains(line, "codegen_test_") {
				fmt.Fprintln(combined, line)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading coverage file: %v", err)
	}

	return nil
}

// TestMain handles setup and cleanup for the code generation tests
func TestMain(m *testing.M) {
	// Clean up any previous coverage file
	os.Remove(combinedCoverageFile)

	// Run tests
	exitCode := m.Run()

	// Print coverage file location (only if not running from Makefile)
	if _, err := os.Stat(combinedCoverageFile); err == nil {
		// Check if we're running from make (quieter output)
		if os.Getenv("MAKEFLAGS") == "" {
			fmt.Printf("\n📊 Combined coverage file created: %s\n", combinedCoverageFile)
			fmt.Printf("📊 To view coverage: go tool cover -html=%s\n", combinedCoverageFile)
			fmt.Printf("📊 To get coverage percentage: go tool cover -func=%s\n", combinedCoverageFile)
		}
	}

	os.Exit(exitCode)
}
