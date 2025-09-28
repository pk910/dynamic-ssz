package main

import (
	"log"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/pk910/dynamic-ssz/codegen"
	"github.com/pk910/dynamic-ssz/examples/codegen/types"
)

func main() {
	// Create a code generator instance
	generator := codegen.NewCodeGenerator(nil)

	// Get the parent directory (where types are defined)
	_, currentFile, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(currentFile)
	parentDir := filepath.Dir(currentDir)

	// Generate SSZ methods for our types
	generator.BuildFile(
		filepath.Join(parentDir, "types", "types_ssz.go"),
		codegen.WithReflectType(reflect.TypeOf(&types.User{})),
		codegen.WithReflectType(reflect.TypeOf(&types.Transaction{})),
		codegen.WithReflectType(reflect.TypeOf(&types.Block{})),
		codegen.WithReflectType(reflect.TypeOf(&types.GameState{})),
		codegen.WithReflectType(reflect.TypeOf(&types.Player{})),
		codegen.WithReflectType(reflect.TypeOf(&types.Move{})),
		codegen.WithReflectType(reflect.TypeOf(&types.Tile{})),
		codegen.WithCreateLegacyFn(), // Generate legacy HashTreeRoot() methods
	)

	// Generate the code
	if err := generator.Generate(); err != nil {
		log.Fatal("Code generation failed:", err)
	}

	log.Println("Code generation completed successfully!")
}
