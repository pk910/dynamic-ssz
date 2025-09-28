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
	Name    string         // Test name
	Payload any            // Test payload
	Specs   map[string]any // Dynamic specifications
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
		Name: "SimpleTypes1",
		Payload: struct {
			B1       bool
			I8       uint8
			I16      uint16
			I32      uint32
			I64      uint64
			I128     [16]byte
			I256     [4]uint64
			Vec8     []uint8     `ssz-size:"4"`
			Vec32    []uint32    `ssz-size:"4"`
			Vec128   [][2]uint64 `ssz-type:"?,uint128" ssz-size:"4"`
			BitVec   [8]byte     `ssz-type:"bitvector"`
			Lst8     []uint8     `ssz-max:"4"`
			Lst32    []uint32    `ssz-max:"4"`
			Lst128   [][2]uint64 `ssz-type:"?,uint128" ssz-max:"4"`
			Str      string      `ssz-max:"8"`
			Wrapper1 dynssz.TypeWrapper[struct {
				Data []byte `ssz-size:"32"`
			}, []byte] `ssz-type:"wrapper"`
		}{
			B1:     true,
			I8:     1,
			I16:    2,
			I32:    3,
			I64:    4,
			I128:   [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			I256:   [4]uint64{1, 2, 3, 4},
			Vec8:   []uint8{1, 2, 3, 4},
			Vec32:  []uint32{1, 2, 3, 4},
			Vec128: [][2]uint64{{1, 2}, {3, 4}},
			BitVec: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Lst8:   []uint8{1, 2, 3, 4},
			Lst32:  []uint32{1, 2, 3, 4},
			Lst128: [][2]uint64{{1, 2}, {3, 4}},
			Str:    "hello",
			Wrapper1: dynssz.TypeWrapper[struct {
				Data []byte `ssz-size:"32"`
			}, []byte]{
				Data: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			},
		},
		Specs: map[string]any{},
	},
	{
		Name: "SimpleTypesWithSpecs",
		Payload: struct {
			Vec8   []uint8     `ssz-size:"4" dynssz-size:"VEC8_SIZE"`
			Vec32  []uint32    `ssz-size:"4" dynssz-size:"VEC32_SIZE"`
			Vec128 [][2]uint64 `ssz-type:"?,uint128" ssz-size:"4" dynssz-size:"VEC128_SIZE"`
			BitVec []byte      `ssz-type:"bitvector" ssz-size:"8" dynssz-size:"BITVEC_SIZE"`
			Lst8   []uint8     `ssz-max:"4" dynssz-max:"LST8_MAX"`
			Lst32  []uint32    `ssz-max:"4" dynssz-max:"LST32_MAX"`
			Lst128 [][2]uint64 `ssz-type:"?,uint128" ssz-max:"4" dynssz-max:"LST128_MAX"`
			Str    string      `ssz-max:"8" dynssz-max:"STR_MAX"`
		}{
			Vec8:   []uint8{1, 2, 3, 4, 5, 6},
			Vec32:  []uint32{1, 2, 3, 4, 5, 6, 7, 8},
			Vec128: [][2]uint64{{1, 2}, {3, 4}},
			BitVec: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			Lst8:   []uint8{1, 2, 3, 4, 5, 6},
			Lst32:  []uint32{1, 2, 3, 4, 5, 6, 7, 8},
			Lst128: [][2]uint64{{1, 2}, {3, 4}},
			Str:    "hello",
		},
		Specs: map[string]any{
			"VEC8_SIZE":   6,
			"VEC32_SIZE":  8,
			"VEC128_SIZE": 2,
			"BITVEC_SIZE": 10,
			"LST8_MAX":    6,
			"LST32_MAX":   8,
			"LST128_MAX":  2,
			"STR_MAX":     16,
		},
	},
	{
		Name: "ProgressiveTypes",
		Payload: struct {
			C1 struct {
				F1 uint64 `ssz-index:"0"`
				F3 uint64 `ssz-index:"2"`
				F7 uint8  `ssz-index:"6"`
			} `ssz-type:"progressive-container"`
			L1 []uint64 `ssz-type:"progressive-list"`
			L2 []byte   `ssz-type:"progressive-bitlist"`
			U1 dynssz.CompatibleUnion[struct {
				F1 uint32
				F2 [2][]uint8 `ssz-size:"2,5"`
			}]
		}{
			C1: struct {
				F1 uint64 `ssz-index:"0"`
				F3 uint64 `ssz-index:"2"`
				F7 uint8  `ssz-index:"6"`
			}{
				F1: 12345,
				F3: 67890,
				F7: 123,
			},
			L1: []uint64{12345, 67890},
			L2: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			U1: dynssz.CompatibleUnion[struct {
				F1 uint32
				F2 [2][]uint8 `ssz-size:"2,5"`
			}]{
				Variant: 0,
				Data:    uint32(0x12345678),
			},
		},
		Specs: map[string]any{},
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

	var sszHex, hashRootHex string
	defer func() {
		if t.Failed() {
			t.Logf("Payload %v data dir: %s", payload.Name, tempDir)
			t.Logf("Payload hex: %v", sszHex)
			t.Logf("Payload hash root: %v", hashRootHex)
		} else {
			os.RemoveAll(tempDir)
		}
	}()

	ds := dynssz.NewDynSsz(payload.Specs)

	sszBytes, err := ds.MarshalSSZ(payload.Payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}
	sszHex = hex.EncodeToString(sszBytes)

	hashRoot, err := ds.HashTreeRoot(payload.Payload)
	if err != nil {
		t.Fatalf("Failed to hash tree root: %v", err)
	}
	hashRootHex = hex.EncodeToString(hashRoot[:])

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
	output, err := runTest(tempDir, sszHex)
	//fmt.Printf("Test output:\n%s", output)

	if err != nil {
		t.Fatalf("Test execution failed: %v", err)
	}

	// Step 7: Verify results with reflection
	if err := verifyResults(t, payload, sszHex, hashRootHex, output); err != nil {
		t.Fatalf("Result verification failed: %v", err)
	}
}

func writeTypeFile(filename string, payload TestPayload) error {
	content := fmt.Sprintf(`package types

import dynssz "github.com/pk910/dynamic-ssz"

var _ dynssz.SszType

// TestPayload represents the test type
type TestPayload %s

// GetSpecs returns the dynamic specifications
func GetSpecs() map[string]any {
	return %#v
}
`,
		formatTypeString(reflect.TypeOf(payload.Payload)),
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
		codegen.WithReflectType(reflect.TypeOf(&types.TestPayload{})),
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

func verifyResults(t *testing.T, payload TestPayload, sszHex string, hashRootHex string, output string) error {
	// Parse JSON output
	var results TestResults
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		return fmt.Errorf("failed to parse JSON output: %v", err)
	}

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
			if results.Legacy.HashTreeRoot != hashRootHex {
				return fmt.Errorf("legacy: hash mismatch: generated=%s, expected=%s", results.Legacy.HashTreeRoot, hashRootHex)
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
			if results.Dynamic.HashTreeRoot != hashRootHex {
				return fmt.Errorf("dynamic: hash mismatch: generated=%s, expected=%s", results.Dynamic.HashTreeRoot, hashRootHex)
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
			if results.Hybrid.HashTreeRoot != hashRootHex {
				return fmt.Errorf("hybrid: hash mismatch: generated=%s, expected=%s", results.Hybrid.HashTreeRoot, hashRootHex)
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

		if t.PkgPath() == "github.com/pk910/dynamic-ssz" && strings.HasPrefix(t.Name(), "TypeWrapper[") {
			wrapperValue := reflect.New(t)
			method := wrapperValue.MethodByName("GetDescriptorType")
			if !method.IsValid() {
				return ""
			}

			// Call the method to get the descriptor type
			results := method.Call(nil)
			if len(results) == 0 {
				return ""
			}

			descriptorType, ok := results[0].Interface().(reflect.Type)
			if !ok {
				return ""
			}

			fieldType := descriptorType.Field(0)

			return fmt.Sprintf("dynssz.TypeWrapper[%s, %s]", formatTypeString(descriptorType), formatTypeString(fieldType.Type))
		} else if t.PkgPath() == "github.com/pk910/dynamic-ssz" && strings.HasPrefix(t.Name(), "CompatibleUnion[") {
			wrapperValue := reflect.New(t)
			method := wrapperValue.MethodByName("GetDescriptorType")
			if !method.IsValid() {
				return ""
			}

			// Call the method to get the descriptor type
			results := method.Call(nil)
			if len(results) == 0 {
				return ""
			}

			descriptorType, ok := results[0].Interface().(reflect.Type)
			if !ok {
				return ""
			}

			return fmt.Sprintf("dynssz.CompatibleUnion[%s]", formatTypeString(descriptorType))
		}

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
		fmt.Fprintln(combined, "mode: atomic")
	} else {
		// Append to existing file
		combined, err = os.OpenFile(combinedFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open combined coverage file: %v", err)
		}
		defer combined.Close()
	}

	// Copy coverage data (skip "mode: atomic" line from new file)
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
