package codegen

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
)

// TestPayload represents a single test case for code generation
type TestPayload struct {
	Name   string         // Test name
	Type   reflect.Type   // Go type to generate code for
	SSZHex string         // SSZ encoding as hex string (without 0x prefix)
	Specs  map[string]any // Dynamic specifications
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
	"fmt"
	"log"
	"os"
	
	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
	"codegen_test_%s/types"
)

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
	
	// Create DynSSZ instance
	ds := dynssz.NewDynSsz(specs)
	
	fmt.Printf("=== Code Generation Test Results ===\n")
	fmt.Printf("SSZ Input: %%s\n", sszHex)
	
	// Test 1: Unmarshal with all available code paths
	fmt.Printf("\n--- Testing Unmarshal ---\n")
	
	// Try dynamic unmarshal
	var testValue2 types.TestPayload
	var unmarshalErr2 error
	if dynUnmarshaler, ok := interface{}(&testValue2).(sszutils.DynamicUnmarshaler); ok {
		unmarshalErr2 = dynUnmarshaler.UnmarshalSSZDyn(ds, expectedSSZ)
		fmt.Printf("Unmarshal Method: Dynamic (UnmarshalSSZDyn)\n")
		if unmarshalErr2 != nil {
			fmt.Printf("Dynamic Unmarshal Error: %%v\n", unmarshalErr2)
		}
	} else {
		fmt.Printf("Unmarshal Method: Dynamic not available\n")
	}
	
	// Always try reflection unmarshal
	var testValue3 types.TestPayload
	unmarshalErr3 := ds.UnmarshalSSZ(&testValue3, expectedSSZ)
	fmt.Printf("Unmarshal Method: Reflection (fallback)\n")
	if unmarshalErr3 != nil {
		fmt.Printf("Reflection Unmarshal Error: %%v\n", unmarshalErr3)
	}
	
	// Use the first successful unmarshal for further tests
	var testValue types.TestPayload
	var unmarshalSuccess bool
	if _, ok := interface{}(&testValue2).(sszutils.DynamicUnmarshaler); ok && unmarshalErr2 == nil {
		testValue = testValue2
		unmarshalSuccess = true
		fmt.Printf("Using Dynamic unmarshaled value\n")
	} else if unmarshalErr3 == nil {
		testValue = testValue3
		unmarshalSuccess = true
		fmt.Printf("Using Reflection unmarshaled value\n")
	} else {
		log.Fatal("All unmarshal methods failed")
	}
	
	fmt.Printf("Unmarshal Success: %%t\n", unmarshalSuccess)
	
	// Test 2: Marshal with generated code
	fmt.Printf("\n--- Testing Marshal ---\n")
	var marshalErr error
	var marshalData []byte
	
	if dynMarshaler, ok := interface{}(&testValue).(sszutils.DynamicMarshaler); ok {
		buf := make([]byte, 0)
		marshalData, marshalErr = dynMarshaler.MarshalSSZDyn(ds, buf)
		fmt.Printf("Marshal Method: Dynamic (MarshalSSZDyn)\n")
	} else {
		// Fallback to reflection
		marshalData, marshalErr = ds.MarshalSSZ(&testValue)
		fmt.Printf("Marshal Method: Reflection (fallback)\n")
	}
	
	if marshalErr != nil {
		log.Fatalf("Marshal failed: %%v", marshalErr)
	}
	
	fmt.Printf("Generated SSZ: %%s\n", hex.EncodeToString(marshalData))
	fmt.Printf("Expected SSZ:  %%s\n", hex.EncodeToString(expectedSSZ))
	fmt.Printf("SSZ Match: %%t\n", bytes.Equal(marshalData, expectedSSZ))
	
	// Test 3: Hash tree root with generated code
	fmt.Printf("\n--- Testing Hash Tree Root ---\n")
	var hashErr error
	var hashRoot [32]byte
	
	if dynHasher, ok := interface{}(&testValue).(sszutils.DynamicHashRoot); ok {
		// DynamicHashRoot uses HashTreeRootDyn which requires a hasher
		pool := &hasher.FastHasherPool
		hh := pool.Get()
		defer func() {
			pool.Put(hh)
		}()
		hashErr = dynHasher.HashTreeRootDyn(ds, hh)
		if hashErr == nil {
			hashRoot, _ = hh.HashRoot()
		}
		fmt.Printf("Hash Method: Dynamic (HashTreeRootDyn)\n") 
	} else {
		// Fallback to reflection
		hashRoot, hashErr = ds.HashTreeRoot(&testValue)
		fmt.Printf("Hash Method: Reflection (fallback)\n")
	}
	
	if hashErr != nil {
		log.Fatalf("Hash tree root failed: %%v", hashErr)
	}
	
	fmt.Printf("Generated Hash: %%s\n", hex.EncodeToString(hashRoot[:]))
	
	fmt.Printf("\n=== Test Completed ===\n")
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

	cmd := exec.CommandContext(ctx, "go", "run", "./codegen")
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("codegen execution failed: %v\nOutput: %s", err, string(output))
	}

	return nil
}

func runTest(tempDir string, sszHex string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".", sszHex)
	cmd.Dir = tempDir
	output, _ := cmd.CombinedOutput()

	return string(output), nil
}

func verifyResults(t *testing.T, payload TestPayload, output string) error {
	// Unmarshal the test value and calculate expected hash tree root using reflection
	ds := dynssz.NewDynSsz(payload.Specs)
	sszBytes, err := hex.DecodeString(payload.SSZHex)
	if err != nil {
		return fmt.Errorf("failed to decode SSZ hex: %v", err)
	}

	// Create a new instance of the type and unmarshal
	testValue := reflect.New(payload.Type).Interface()
	if err := ds.UnmarshalSSZ(testValue, sszBytes); err != nil {
		return fmt.Errorf("failed to unmarshal test data: %v", err)
	}

	expectedRoot, err := ds.HashTreeRoot(testValue)
	if err != nil {
		return fmt.Errorf("failed to calculate expected hash tree root: %v", err)
	}

	// Parse output for verification
	lines := strings.Split(output, "\n")

	var unmarshalSuccess, sszMatch, hashFound bool
	var generatedHash string
	var unmarshalMethods []string

	for _, line := range lines {
		// Check unmarshal results
		if strings.Contains(line, "Unmarshal Success: true") {
			unmarshalSuccess = true
		}
		if strings.Contains(line, "Unmarshal Method: Legacy (UnmarshalSSZ)") && !strings.Contains(line, "not available") {
			unmarshalMethods = append(unmarshalMethods, "Legacy")
		}
		if strings.Contains(line, "Unmarshal Method: Dynamic (UnmarshalSSZDyn)") && !strings.Contains(line, "not available") {
			unmarshalMethods = append(unmarshalMethods, "Dynamic")
		}
		if strings.Contains(line, "Unmarshal Method: Reflection (fallback)") {
			unmarshalMethods = append(unmarshalMethods, "Reflection")
		}

		// Check marshal results
		if strings.Contains(line, "SSZ Match: true") {
			sszMatch = true
		}

		// Check hash results
		if strings.HasPrefix(line, "Generated Hash: ") {
			hashFound = true
			generatedHash = strings.TrimPrefix(line, "Generated Hash: ")
		}
	}

	if !unmarshalSuccess {
		return fmt.Errorf("unmarshal failed in output")
	}

	if !sszMatch {
		return fmt.Errorf("SSZ encoding mismatch in output")
	}

	if !hashFound {
		return fmt.Errorf("generated hash not found in output")
	}

	// Verify hash matches reflection calculation
	expectedHashHex := hex.EncodeToString(expectedRoot[:])
	if generatedHash != expectedHashHex {
		return fmt.Errorf("hash mismatch: generated=%s, expected=%s", generatedHash, expectedHashHex)
	}

	// Verify test completed successfully
	if !strings.Contains(output, "=== Test Completed ===") {
		return fmt.Errorf("test did not complete successfully")
	}

	t.Logf("✅ Test %s passed - Unmarshal methods: %v, SSZ and hash tree root match reflection", payload.Name, unmarshalMethods)
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
