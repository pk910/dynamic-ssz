// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package codegen provides code generation for dynamic SSZ types.
package codegen

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"reflect"

	dynssz "github.com/pk910/dynamic-ssz"
)

// TypeImport represents an import declaration for generated code files.
//
// This structure is used during code generation to track and organize import statements
// that need to be included in the generated Go files. It pairs import paths with their
// corresponding aliases to avoid naming conflicts and provide clean, readable generated code.
//
// Fields:
//   - Alias: The alias name for the import (empty string means no alias)
//   - Path: The full import path (e.g., "github.com/pkg/errors")
type TypeImport struct {
	Alias string
	Path  string
}

// CodeGeneratorOption is a functional option for configuring code generation behavior.
//
// This type follows the functional options pattern, allowing callers to customize
// various aspects of the code generation process through composable option functions.
// Options can control which methods are generated, performance optimizations,
// and compatibility settings.
//
// Example usage:
//
//	cg := NewCodeGenerator(dynSsz)
//	cg.BuildFile("generated.go",
//	    WithNoMarshalSSZ(),           // Skip marshal method generation
//	    WithCreateLegacyFn(),         // Generate legacy fastssz methods
//	    WithoutDynamicExpressions(),  // Use static sizes only
//	)
type CodeGeneratorOption func(*CodeGeneratorOptions)

// CodeGeneratorOptions contains all configuration settings for code generation.
//
// This structure controls every aspect of the code generation process, from which
// methods to generate to performance optimizations and compatibility modes.
// It serves as the central configuration hub for customizing generated SSZ code
// to meet specific requirements and constraints.
//
// Method Generation Controls:
//   - NoMarshalSSZ: Skip generating MarshalSSZTo and MarshalSSZDyn methods
//   - NoUnmarshalSSZ: Skip generating UnmarshalSSZ and UnmarshalSSZDyn methods
//   - NoSizeSSZ: Skip generating SizeSSZ and SizeSSZDyn methods
//   - NoHashTreeRoot: Skip generating HashTreeRoot and HashTreeRootDyn methods
//
// Compatibility and Performance Options:
//   - CreateLegacyFn: Generate legacy fastssz-compatible methods (MarshalSSZ, etc.)
//   - WithoutDynamicExpressions: Generate static-only code, ignoring dynamic expressions
//   - NoFastSsz: Disable use of fastssz optimizations in generated code
//
// Type Configuration:
//   - SizeHints: Static and dynamic size constraints for types
//   - MaxSizeHints: Maximum size limits for variable-length types
//   - TypeHints: Explicit SSZ type mappings for ambiguous Go types
//   - Types: Specific types to include in generation with per-type options
type CodeGeneratorOptions struct {
	NoMarshalSSZ              bool
	NoUnmarshalSSZ            bool
	NoSizeSSZ                 bool
	NoHashTreeRoot            bool
	CreateLegacyFn            bool
	WithoutDynamicExpressions bool
	NoFastSsz                 bool
	SizeHints                 []dynssz.SszSizeHint
	MaxSizeHints              []dynssz.SszMaxSizeHint
	TypeHints                 []dynssz.SszTypeHint
	Types                     []CodeGeneratorTypeOption
}

// CodeGeneratorTypeOption specifies a type to include in code generation with its specific options.
//
// This structure allows fine-grained control over individual types within a generation request.
// Each type can have its own set of generation options that override or extend the base
// configuration. This enables scenarios like generating different method sets for different
// types within the same output file.
//
// Type Specification (exactly one must be provided):
//   - ReflectType: Go reflection-based type representation (runtime type)
//   - GoTypesType: Go types package representation (compile-time type analysis)
//
// Configuration:
//   - Opts: Type-specific options that will be applied in addition to base options
//
// Example:
//
//	// Generate different method sets for different types
//	WithReflectType(reflect.TypeOf((*MyStruct)(nil)).Elem(),
//	    WithNoHashTreeRoot(), // Skip hash tree root for this type only
//	),
//	WithReflectType(reflect.TypeOf((*AnotherStruct)(nil)).Elem()),
type CodeGeneratorTypeOption struct {
	ReflectType reflect.Type
	GoTypesType types.Type
	Opts        []CodeGeneratorOption
}

// CodeGeneratorTypeOptions contains the resolved configuration for a specific type during generation.
//
// This structure represents the final, processed configuration for a type after all options
// have been applied and type analysis has been completed. It serves as the authoritative
// source of information about how a type should be generated, including its resolved
// type descriptor and effective generation options.
//
// Type Information:
//   - ReflectType: Runtime reflection type representation
//   - GoTypesType: Compile-time type analysis representation
//   - TypeName: Resolved type name for code generation
//
// Generation Configuration:
//   - Options: Final resolved options after applying base and type-specific settings
//   - Descriptor: Analyzed SSZ type descriptor containing encoding/decoding metadata
type CodeGeneratorTypeOptions struct {
	ReflectType reflect.Type
	GoTypesType types.Type
	TypeName    string
	Options     CodeGeneratorOptions
	Descriptor  *dynssz.TypeDescriptor
}

// CodeGeneratorFileOptions contains the configuration for generating a complete Go source file.
//
// This structure represents all the information needed to generate a single Go file containing
// SSZ methods for multiple types. It includes package information and the complete list of
// types to be included in the file with their individual configurations.
//
// File Structure:
//   - Package: The Go package path for the generated file
//   - Types: All types to include in the file with their resolved configurations
//
// The generator ensures all types belong to the same package and handles import management,
// method generation, and proper Go source code formatting for the entire file.
type CodeGeneratorFileOptions struct {
	Package string
	Types   []*CodeGeneratorTypeOptions
}

// WithNoMarshalSSZ creates an option to skip generating MarshalSSZTo and MarshalSSZDyn methods.
//
// When this option is applied, the code generator will not produce any marshaling methods
// for the target types. This is useful when:
//   - Types only need to be deserialized (read-only scenarios)
//   - Custom marshaling logic is implemented elsewhere
//   - Reducing generated code size for specific use cases
//
// Note that skipping marshal methods may limit interoperability with some SSZ libraries
// that expect the full method interface to be present.
//
// Returns:
//   - CodeGeneratorOption: A functional option that disables marshal method generation
func WithNoMarshalSSZ() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoMarshalSSZ = true
	}
}

// WithNoUnmarshalSSZ creates an option to skip generating UnmarshalSSZ and UnmarshalSSZDyn methods.
//
// When this option is applied, the code generator will not produce any unmarshaling methods
// for the target types. This is useful when:
//   - Types are only created programmatically (write-only scenarios)
//   - Custom unmarshaling logic is implemented elsewhere
//   - Reducing generated code size for specific use cases
//
// Note that skipping unmarshal methods may limit interoperability with some SSZ libraries
// that expect the full method interface to be present.
//
// Returns:
//   - CodeGeneratorOption: A functional option that disables unmarshal method generation
func WithNoUnmarshalSSZ() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoUnmarshalSSZ = true
	}
}

// WithNoSizeSSZ creates an option to skip generating SizeSSZ and SizeSSZDyn methods.
//
// When this option is applied, the code generator will not produce size calculation methods
// for the target types. This is useful when:
//   - Size calculation is not needed for the application
//   - Custom size calculation logic is implemented elsewhere
//   - Optimizing for code size in constrained environments
//
// Note that many SSZ operations benefit from accurate size pre-calculation for buffer
// allocation, so this option should be used carefully.
//
// Returns:
//   - CodeGeneratorOption: A functional option that disables size method generation
func WithNoSizeSSZ() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoSizeSSZ = true
	}
}

// WithNoHashTreeRoot creates an option to skip generating HashTreeRoot and HashTreeRootDyn methods.
//
// When this option is applied, the code generator will not produce hash tree root calculation
// methods for the target types. This is useful when:
//   - Hash tree roots are not needed for the application
//   - Custom hashing logic is implemented elsewhere
//   - Optimizing for code size when cryptographic commitments aren't required
//
// Note that hash tree roots are essential for many blockchain and cryptographic applications,
// particularly in Ethereum consensus where they're used for Merkle proofs and state roots.
//
// Returns:
//   - CodeGeneratorOption: A functional option that disables hash tree root method generation
func WithNoHashTreeRoot() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoHashTreeRoot = true
	}
}

// WithSizeHints creates an option to provide size constraints for dynamic types.
//
// Size hints guide the code generator in handling types with variable or dynamic sizes.
// They can specify exact sizes, dynamic sizing expressions, or size calculation formulas
// that reference runtime specification values.
//
// Common use cases:
//   - Fixed-size arrays where size depends on configuration: []Item with size "SLOTS_PER_EPOCH"
//   - Variable-size containers with known constraints
//   - Performance optimization by pre-calculating buffer sizes
//
// Parameters:
//   - hints: Array of size hints, typically corresponding to nested type levels
//
// Returns:
//   - CodeGeneratorOption: A functional option that applies the provided size hints
//
// Example:
//
//	WithSizeHints([]dynssz.SszSizeHint{
//	    {Size: 8192, Expr: "SLOTS_PER_HISTORICAL_ROOT"},
//	})
func WithSizeHints(hints []dynssz.SszSizeHint) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.SizeHints = hints
	}
}

// WithMaxSizeHints creates an option to provide maximum size constraints for variable-length types.
//
// Maximum size hints define upper bounds for variable-length types like slices and lists.
// These constraints are crucial for:
//   - Memory allocation optimization
//   - Security bounds checking
//   - SSZ encoding validation
//   - Buffer pre-allocation in performance-critical code
//
// The hints help generate efficient code that can pre-allocate appropriate buffer sizes
// and validate data doesn't exceed specified limits during encoding/decoding.
//
// Parameters:
//   - hints: Array of maximum size hints, typically corresponding to nested type levels
//
// Returns:
//   - CodeGeneratorOption: A functional option that applies the provided maximum size hints
//
// Example:
//
//	WithMaxSizeHints([]dynssz.SszMaxSizeHint{
//	    {Size: 1048576, Expr: "MAX_VALIDATORS_PER_COMMITTEE"},
//	})
func WithMaxSizeHints(hints []dynssz.SszMaxSizeHint) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.MaxSizeHints = hints
	}
}

// WithTypeHints creates an option to provide explicit SSZ type mappings for ambiguous Go types.
//
// Type hints resolve ambiguity when Go types can be mapped to multiple SSZ types.
// This is particularly important for:
//   - Generic types that could be containers, lists, or vectors
//   - Interface types that need concrete SSZ representations
//   - Custom types that implement multiple SSZ-compatible interfaces
//   - Ensuring consistent type interpretation across different contexts
//
// The hints allow precise control over how the generator interprets and encodes
// complex type hierarchies, ensuring compatibility with dynamic expressions and
// runtime specifications.
//
// Parameters:
//   - hints: Array of type hints mapping Go types to specific SSZ type interpretations
//
// Returns:
//   - CodeGeneratorOption: A functional option that applies the provided type hints
//
// Example:
//
//	WithTypeHints([]dynssz.SszTypeHint{
//	    {Type: dynssz.SszListType}, // Force slice to be treated as list not vector
//	    {Type: dynssz.SszContainerType}, // Force struct to be treated as container
//	})
func WithTypeHints(hints []dynssz.SszTypeHint) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.TypeHints = hints
	}
}

// WithCreateLegacyFn creates an option to generate legacy fastssz-compatible methods.
//
// When this option is enabled, the generator creates additional methods that match
// the fastssz library interface, using a global DynSsz instance for compatibility.
// These methods include:
//   - MarshalSSZ() ([]byte, error)
//   - UnmarshalSSZ([]byte) error
//   - SizeSSZ() int
//   - HashTreeRoot() ([32]byte, error)
//
// This option is essential for:
//   - Drop-in replacement for fastssz-generated code
//   - Gradual migration from fastssz to dynamic-ssz
//   - Interoperability with existing codebases expecting fastssz interfaces
//   - Maintaining backward compatibility in public APIs
//
// The legacy methods delegate to the dynamic methods using the global DynSsz instance,
// so they inherit all dynamic sizing and expression capabilities.
//
// Returns:
//   - CodeGeneratorOption: A functional option that enables legacy method generation
func WithCreateLegacyFn() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.CreateLegacyFn = true
	}
}

// WithoutDynamicExpressions creates an option to generate static-only code that ignores dynamic expressions.
//
// When this option is enabled, the generator produces highly optimized code that uses only
// static, compile-time known sizes and completely ignores any dynamic size expressions.
// This results in:
//   - Maximum performance characteristics for the default/known preset
//   - Smaller generated code size
//   - No runtime expression evaluation overhead
//   - Compile-time size validation
//
// Trade-offs:
//   - Loss of flexibility for different presets/specifications
//   - Cannot handle runtime size variations
//   - Falls back to slower reflection-based methods for dynamic cases
//   - Not compatible with types that require dynamic expression evaluation
//
// This option is ideal for production deployments where:
//   - The specification preset is known at compile time
//   - Maximum performance is prioritized over flexibility
//   - Code size optimization is important
//
// Returns:
//   - CodeGeneratorOption: A functional option that enables static-only code generation
func WithoutDynamicExpressions() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.WithoutDynamicExpressions = true
	}
}

// WithNoFastSsz creates an option to skip generating fast ssz generated methods.
//
// When this option is enabled, the generator will not use any fast ssz generated methods for static types.
//
// Returns:
//   - CodeGeneratorOption: A functional option that disables fast ssz generated method generation
func WithNoFastSsz() CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.NoFastSsz = true
	}
}

// WithReflectType creates an option to include a specific type using runtime reflection.
//
// This function adds a type to the generation list using Go's reflection system to
// analyze the type structure. It's the most common way to specify types for generation
// when you have access to the actual Go types at runtime.
//
// The function accepts additional type-specific options that will be applied only to
// this type, allowing fine-grained control over generation behavior per type.
//
// Parameters:
//   - t: The reflect.Type to include in code generation (typically obtained via reflect.TypeOf)
//   - typeOpts: Optional type-specific generation options that override base settings
//
// Returns:
//   - CodeGeneratorOption: A functional option that adds the type to the generation list
//
// Example:
//
//	// Add a type with default options
//	WithReflectType(reflect.TypeOf((*MyStruct)(nil)).Elem()),
//
//	// Add a type with specific options
//	WithReflectType(reflect.TypeOf((*AnotherStruct)(nil)).Elem(),
//	    WithNoHashTreeRoot(), // Skip hash tree root for this type only
//	    WithCreateLegacyFn(), // But include legacy methods
//	),
func WithReflectType(t reflect.Type, typeOpts ...CodeGeneratorOption) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.Types = append(opts.Types, CodeGeneratorTypeOption{
			ReflectType: t,
			Opts:        typeOpts,
		})
	}
}

// WithGoTypesType creates an option to include a specific type using compile-time type analysis.
//
// This function adds a type to the generation list using Go's types package for
// compile-time type analysis. This approach is more powerful than reflection as it
// can analyze types that may not be available at runtime and provides richer
// type information for complex scenarios.
//
// This method is typically used in code analysis tools, compilers, or advanced
// code generation scenarios where you're working with the Go AST or type checker.
//
// Parameters:
//   - t: The types.Type to include in code generation (from go/types package)
//   - typeOpts: Optional type-specific generation options that override base settings
//
// Returns:
//   - CodeGeneratorOption: A functional option that adds the type to the generation list
//
// Example:
//
//	// In a compiler or analysis tool context
//	var structType *types.Named = ... // obtained from type analysis
//	WithGoTypesType(structType,
//	    WithCreateLegacyFn(),
//	    WithoutDynamicExpressions(),
//	)
func WithGoTypesType(t types.Type, typeOpts ...CodeGeneratorOption) CodeGeneratorOption {
	return func(opts *CodeGeneratorOptions) {
		opts.Types = append(opts.Types, CodeGeneratorTypeOption{
			GoTypesType: t,
			Opts:        typeOpts,
		})
	}
}

// fileGenerationRequest represents an internal request to generate a single file with SSZ methods.
//
// This structure is used internally by the CodeGenerator to track individual file generation
// requests. Each request corresponds to one output file and contains all the information
// needed to generate that file, including the target filename and all types to include.
//
// Fields:
//   - FileName: The target output file path for the generated code
//   - Options: Complete file generation configuration including package and type information
//
// This type is not exposed in the public API and is used only for internal organization
// of the generation pipeline.
type fileGenerationRequest struct {
	FileName string
	Options  *CodeGeneratorFileOptions
}

// CodeGenerator manages batch generation of SSZ methods for multiple types across multiple files.
//
// The CodeGenerator provides a high-level interface for generating SSZ encoding/decoding
// methods for Go types. It supports generating multiple files with different configurations,
// each containing optimized SSZ methods for specified types.
//
// Key capabilities:
//   - Batch generation of multiple files with different type sets
//   - Per-file and per-type configuration options
//   - Automatic dependency analysis and import management
//   - Support for both reflection-based and compile-time type analysis
//   - Integration with dynamic-ssz specifications and size expressions
//
// The generator uses a DynSsz instance for type analysis and descriptor creation,
// ensuring that generated code is compatible with the runtime library's type system.
//
// Fields:
//   - files: Internal list of file generation requests
//   - dynSsz: DynSsz instance used for type analysis and descriptor generation
//
// Typical workflow:
//  1. Create generator with NewCodeGenerator()
//  2. Add files with BuildFile()
//  3. Generate code with Generate() or GenerateToMap()
type CodeGenerator struct {
	files  []*fileGenerationRequest
	dynSsz *dynssz.DynSsz
}

// NewCodeGenerator creates a new code generator instance with the specified DynSsz configuration.
//
// The code generator requires a DynSsz instance to perform type analysis and create
// type descriptors for the types being generated. The DynSsz instance's specification
// values and configuration directly influence the generated code.
//
// Parameters:
//   - dynSsz: DynSsz instance for type analysis and descriptor creation.
//     If nil, a default instance with no specifications will be created.
//
// Returns:
//   - *CodeGenerator: A new code generator ready to accept file generation requests
//
// Example:
//
//	// Create with Ethereum mainnet specifications
//	specs := map[string]any{
//	    "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
//	    "SYNC_COMMITTEE_SIZE":       uint64(512),
//	}
//	dynSsz := dynssz.NewDynSsz(specs)
//	cg := NewCodeGenerator(dynSsz)
//
//	// Create with default configuration
//	cg := NewCodeGenerator(nil)
func NewCodeGenerator(dynSsz *dynssz.DynSsz) *CodeGenerator {
	if dynSsz == nil {
		dynSsz = dynssz.NewDynSsz(nil)
	}

	return &CodeGenerator{
		files:  make([]*fileGenerationRequest, 0),
		dynSsz: dynSsz,
	}
}

// BuildFile adds a file generation request to the code generator.
//
// This method configures a single output file with its associated types and generation
// options. The file will be generated when Generate() or GenerateToMap() is called.
// All types specified in the options must belong to the same Go package.
//
// The method processes all provided options to build the final configuration for the file,
// including resolving type-specific options and validating type compatibility.
//
// Parameters:
//   - fileName: The output file path for the generated code
//   - opts: Variable number of options controlling file generation behavior
//
// Example:
//
//	cg.BuildFile("ethereum_types_generated.go",
//	    WithReflectType(reflect.TypeOf((*BeaconBlock)(nil)).Elem()),
//	    WithReflectType(reflect.TypeOf((*BeaconState)(nil)).Elem(),
//	        WithNoHashTreeRoot(), // Skip hash tree root for this type only
//	    ),
//	    WithCreateLegacyFn(),     // Generate legacy fastssz methods
//	    WithSizeHints(sizeHints), // Apply size constraints
//	)
func (cg *CodeGenerator) BuildFile(fileName string, opts ...CodeGeneratorOption) {
	baseCodeOpts := CodeGeneratorOptions{}
	for _, opt := range opts {
		opt(&baseCodeOpts)
	}

	fileOpts := CodeGeneratorFileOptions{}

	types := baseCodeOpts.Types
	baseCodeOpts.Types = nil

	for _, t := range types {
		codeOpts := baseCodeOpts
		for _, opt := range t.Opts {
			opt(&codeOpts)
		}

		fileOpts.Types = append(fileOpts.Types, &CodeGeneratorTypeOptions{
			ReflectType: t.ReflectType,
			GoTypesType: t.GoTypesType,
			Options:     codeOpts,
		})
	}

	cg.files = append(cg.files, &fileGenerationRequest{
		FileName: fileName,
		Options:  &fileOpts,
	})
}

// GenerateToMap generates code for all requested files and returns the results as a map.
//
// This method processes all file generation requests added via BuildFile() and produces
// the complete Go source code for each file. The returned map contains the file names
// as keys and the generated source code as values.
//
// The generation process includes:
//   - Type analysis and descriptor creation for all specified types
//   - Dependency resolution and import management
//   - SSZ method generation (marshal, unmarshal, size, hash tree root)
//   - Go source code formatting and organization
//   - Cross-reference resolution for types that reference each other
//
// Returns:
//   - map[string]string: Map of file names to their generated Go source code
//   - error: An error if generation fails due to:
//   - Type analysis errors
//   - Package path conflicts
//   - Invalid type configurations
//   - Code generation errors
//
// Example:
//
//	cg := NewCodeGenerator(dynSsz)
//	cg.BuildFile("types.go", WithReflectType(myType))
//	cg.BuildFile("more_types.go", WithReflectType(anotherType))
//
//	files, err := cg.GenerateToMap()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for fileName, code := range files {
//	    fmt.Printf("Generated %s:\n%s\n", fileName, code)
//	}
func (cg *CodeGenerator) GenerateToMap() (map[string]string, error) {
	if len(cg.files) == 0 {
		return nil, fmt.Errorf("no types requested for generation")
	}

	// analyze types
	if err := cg.analyzeTypes(); err != nil {
		return nil, fmt.Errorf("failed to analyze types: %w", err)
	}

	// generate code for each file
	results := make(map[string]string)
	for _, file := range cg.files {
		code, err := cg.generateFile(file.Options.Package, file.Options)
		if err != nil {
			return nil, fmt.Errorf("failed to generate code for %s: %w", file.FileName, err)
		}

		results[file.FileName] = code
	}

	return results, nil
}

// Generate processes all file generation requests and writes the results to disk.
//
// This method is a convenience wrapper around GenerateToMap() that automatically
// writes all generated files to their specified paths. It handles directory creation
// as needed and ensures proper file permissions.
//
// The method will create any necessary intermediate directories and write each
// generated file with standard Go source file permissions (0644).
//
// Returns:
//   - error: An error if generation or file writing fails due to:
//   - Any errors from GenerateToMap()
//   - File system permission issues
//   - Directory creation failures
//   - File write failures
//
// Example:
//
//	cg := NewCodeGenerator(dynSsz)
//	cg.BuildFile("./generated/types.go", WithReflectType(myType))
//	cg.BuildFile("./generated/helpers.go", WithReflectType(anotherType))
//
//	err := cg.Generate()
//	if err != nil {
//	    log.Fatal("Failed to generate files:", err)
//	}
func (cg *CodeGenerator) Generate() error {
	results, err := cg.GenerateToMap()
	if err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	for fileName, code := range results {
		dir := filepath.Dir(fileName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		if err := os.WriteFile(fileName, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write code to file %s: %w", fileName, err)
		}
	}

	return nil
}
