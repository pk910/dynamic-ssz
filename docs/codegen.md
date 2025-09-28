# Code Generation Guide

Dynamic SSZ provides powerful code generation capabilities to create optimized SSZ methods for your Go types. This guide covers all available approaches and their use cases.

## Table of Contents

- [Overview](#overview)
- [CLI Tool (dynssz-gen)](#cli-tool-dynssz-gen)
- [Programmatic API](#programmatic-api)
- [Type Support](#type-support)
- [Tag Reference](#tag-reference)
- [Method Types](#method-types)
- [Advanced Configuration](#advanced-configuration)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)
- [Migration from FastSSZ](#migration-from-fastssz)

## Overview

Dynamic SSZ offers three approaches to code generation:

1. **CLI Tool** - Standalone `dynssz-gen` command for generating SSZ methods from any Go package
2. **Reflection-Based API** - Runtime type analysis for types available at compile time
3. **go/types API** - Compile-time type analysis for advanced build integrations

All approaches share the same underlying code generation engine and produce identical output.

### Key Benefits

- **2-3x Performance Improvement** - Static code eliminates reflection overhead
- **Type Safety** - Compile-time verification of generated code
- **Debugging** - Generated code is readable and debuggable
- **Compatibility** - Works seamlessly with existing Dynamic SSZ features
- **Flexibility** - Support for dynamic field sizes based on runtime configuration

## CLI Tool (dynssz-gen)

The CLI tool is the recommended approach for most users. It can analyze types from any Go package and generate optimized SSZ methods.

### Installation

```bash
go install github.com/pk910/dynamic-ssz/cmd/dynssz-gen@latest
```

### Basic Usage

```bash
# Generate for a single type
dynssz-gen -package ./types -types "MyStruct" -output generated.go

# Generate for multiple types
dynssz-gen -package ./types -types "Block,Transaction,Receipt" -output ssz_gen.go

# Generate from external package
dynssz-gen -package github.com/ethereum/types -types "Header" -output header_ssz.go

# Verbose output for debugging
dynssz-gen -package . -types "Config" -output config_ssz.go -v
```

### Command Line Options

- `-package` - Go package path to analyze (required)
  - Can be a relative path (`./types`) or full import path (`github.com/user/project/types`)
- `-types` - Comma-separated list of type names to generate code for (required)
- `-output` - Output file path for generated code (required)
- `-v` - Enable verbose output for debugging

### Integration with Build Systems

#### Makefile

```makefile
# Makefile
generate:
	dynssz-gen -package ./types -types "Block,State" -output types/generated_ssz.go
	go fmt ./...

build: generate
	go build ./...

.PHONY: generate build
```

#### go:generate

```go
//go:generate dynssz-gen -package . -types "BeaconBlock,BeaconState" -output generated_ssz.go
package types
```

## Programmatic API

### Reflection-Based Generation

Best for types available at compile time:

```go
//go:generate go run ./codegen/main.go

// codegen/main.go
package main

import (
    "log"
    "reflect"
    "github.com/pk910/dynamic-ssz/codegen"
    "myproject/types"
)

func main() {
    // Create generator
    generator := codegen.NewCodeGenerator(nil)
    
    // Add types with options
    generator.BuildFile(
        "types/generated_ssz.go",
        // Basic type
        codegen.WithReflectType(reflect.TypeOf(&types.Block{})),
        
        // Type with custom options
        codegen.WithReflectType(
            reflect.TypeOf(&types.State{}),
            codegen.WithCreateLegacyFn(), // Generate fastssz-compatible methods
        ),
        
        // Type without specific methods
        codegen.WithReflectType(
            reflect.TypeOf(&types.Config{}),
            codegen.WithNoHashTreeRoot(), // Skip HTR generation
        ),
    )
    
    // Generate code
    if err := generator.Generate(); err != nil {
        log.Fatal("Generation failed:", err)
    }
}
```

### go/types-Based Generation

For advanced build tools and compile-time analysis:

```go
package main

import (
    "go/types"
    "golang.org/x/tools/go/packages"
    "github.com/pk910/dynamic-ssz/codegen"
)

func generateFromPackage(pkgPath string) error {
    // Load package
    cfg := &packages.Config{
        Mode: packages.NeedTypes | packages.NeedTypesInfo,
    }
    pkgs, err := packages.Load(cfg, pkgPath)
    if err != nil {
        return err
    }
    
    // Create generator
    generator := codegen.NewCodeGenerator(nil)
    
    // Find and add types
    pkg := pkgs[0]
    scope := pkg.Types.Scope()
    
    for _, name := range scope.Names() {
        obj := scope.Lookup(name)
        if typeObj, ok := obj.(*types.TypeName); ok {
            generator.BuildFile(
                "generated.go",
                codegen.WithGoTypesType(typeObj.Type()),
            )
        }
    }
    
    return generator.Generate()
}
```

## Type Support

### Basic Types

All Go basic types are supported:
- `bool`
- `uint8`, `uint16`, `uint32`, `uint64`
- `[N]byte` arrays (fixed-size byte arrays)
- `[]byte` slices (dynamic byte arrays)

### Container Types

Structs are automatically detected as containers:

```go
type Block struct {
    Number    uint64
    ParentHash [32]byte
    Timestamp  uint64
    Data      []byte `ssz-max:"1024"`
}
```

### Vector and List Types

Arrays become vectors (fixed-size), slices become lists (dynamic):

```go
type Example struct {
    FixedArray  [10]uint32              // Vector: fixed 10 elements
    DynamicList []uint64 `ssz-max:"100"` // List: up to 100 elements
    ByteVector  [32]byte                // Byte vector: exactly 32 bytes
}
```

### Special Types

#### Bitlists and Bitvectors

```go
type Validators struct {
    Active   []byte `ssz-type:"bitlist" ssz-max:"1000000"`
    Slashed  [64]byte `ssz-type:"bitvector"`
}
```

#### Union Types

Using generics for sum types:

```go
type PayloadData = dynssz.CompatibleUnion[struct {
    Bellatrix *BeaconBlockBodyBellatrix
    Capella   *BeaconBlockBodyCapella  
    Deneb     *BeaconBlockBodyDeneb
}]

type Block struct {
    Slot    uint64
    Payload PayloadData
}
```

#### Type Wrappers

For custom SSZ behavior:

```go
type CustomRoot = dynssz.TypeWrapper[
    struct{ Root [32]byte `ssz-size:"32"` },
    [32]byte,
]
```

### External Types

Supported external types:
- `time.Time` - Encoded as uint64 (Unix timestamp)
- `github.com/holiman/uint256.Int` - Encoded as uint256 (32 bytes)

## Tag Reference

### Size Tags

#### `ssz-size`

Fixed size for vectors (arrays that should be treated as fixed-size lists):

```go
Field []byte `ssz-size:"32"`  // Fixed 32-byte vector
```

#### `ssz-max`

Maximum size for lists:

```go
Field []uint64 `ssz-max:"1000"`  // List with max 1000 elements
```

#### `dynssz-size`

Dynamic size with expressions (resolved at runtime):

```go
Field []byte `dynssz-size:"MAX_FIELD_SIZE"`  // Size from spec
```

#### `dynssz-max`

Dynamic maximum with expressions:

```go
Field []uint64 `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
```

### Type Tags

#### `ssz-type`

Override automatic type detection:

```go
Field []byte `ssz-type:"bitlist" ssz-max:"2048"`
Field [4]byte `ssz-type:"bitvector"`
```

Supported values:
- Basic: `bool`, `uint8`, `uint16`, `uint32`, `uint64`, `uint128`, `uint256`
- Complex: `container`, `vector`, `list`
- Special: `bitlist`, `bitvector`
- Advanced: `union`, `custom`

### Progressive Container Tags

#### `ssz-index`

For Ethereum's progressive merkleization:

```go
type ProgressiveState struct {
    Slot      uint64 `ssz-index:"0"`
    Validator uint64 `ssz-index:"1"`
    Balance   uint64 `ssz-index:"2"`
    // All fields must have sequential indices
}
```

## Method Types

### Dynamic Methods (Default)

Generated methods that support runtime configuration through `DynamicSpecs`:

```go
func (t *MyStruct) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error)
func (t *MyStruct) SizeSSZDyn(ds sszutils.DynamicSpecs) int
func (t *MyStruct) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) error
func (t *MyStruct) HashTreeRootDyn(ds sszutils.DynamicSpecs) ([32]byte, error)
func (t *MyStruct) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error
```

Usage:

```go
// Create DynSSZ with runtime specifications
specs := map[string]any{
    "MAX_VALIDATORS": uint64(2048),
    "SYNC_COMMITTEE_SIZE": uint64(512),
}
ds := dynssz.NewDynSsz(specs)

// Use dynamic methods
buf := make([]byte, 0, block.SizeSSZDyn(ds))
data, err := block.MarshalSSZDyn(ds, buf)
```

### Legacy Methods (FastSSZ Compatible)

Optional methods compatible with fastssz (use `WithCreateLegacyFn()`):

```go
func (t *MyStruct) MarshalSSZ() ([]byte, error)
func (t *MyStruct) MarshalSSZTo(buf []byte) ([]byte, error) 
func (t *MyStruct) SizeSSZ() int
func (t *MyStruct) UnmarshalSSZ(buf []byte) error
func (t *MyStruct) HashTreeRoot() ([32]byte, error)
func (t *MyStruct) HashTreeRootWith(hh sszutils.HashWalker) error
```

Usage:

```go
// Legacy methods use compile-time configuration
data, err := block.MarshalSSZ()
size := block.SizeSSZ()
err = block.UnmarshalSSZ(data)
root, err := block.HashTreeRoot()
```

## Advanced Configuration

### Generation Options

Control which methods are generated:

```go
generator.BuildFile(
    "output.go",
    codegen.WithReflectType(reflect.TypeOf(&MyType{})),
    
    // Method selection
    codegen.WithNoMarshalSSZ(),      // Skip MarshalSSZ methods
    codegen.WithNoUnmarshalSSZ(),    // Skip UnmarshalSSZ methods
    codegen.WithNoSizeSSZ(),         // Skip SizeSSZ methods
    codegen.WithNoHashTreeRoot(),    // Skip HashTreeRoot methods
    
    // Legacy compatibility
    codegen.WithCreateLegacyFn(),    // Generate fastssz-compatible methods
    
    // Performance options
    codegen.WithoutDynamicExpressions(), // Generate static code only
)
```

### Multi-File Generation

Generate different types to different files:

```go
generator := codegen.NewCodeGenerator(nil)

// Core types
generator.BuildFile(
    "core/generated.go",
    codegen.WithReflectType(reflect.TypeOf(&Block{})),
    codegen.WithReflectType(reflect.TypeOf(&Header{})),
)

// Network types
generator.BuildFile(
    "network/generated.go",
    codegen.WithReflectType(reflect.TypeOf(&Message{})),
    codegen.WithReflectType(reflect.TypeOf(&Request{})),
)

generator.Generate()
```

### Custom Specifications

When using dynamic expressions, provide specifications:

```go
specs := map[string]any{
    "MAX_VALIDATORS": uint64(1000000),
    "MAX_BLOCK_SIZE": uint64(1 << 20),
}

ds := dynssz.NewDynSsz(specs)
generator := codegen.NewCodeGenerator(ds)
```

## Best Practices

### 1. Use Pointer Types

Always generate code for pointer types to ensure proper method receivers:

```go
// ✅ Good
codegen.WithReflectType(reflect.TypeOf(&MyStruct{}))
dynssz-gen -types "MyStruct"  // Tool handles this automatically

// ❌ Bad - may not work correctly
codegen.WithReflectType(reflect.TypeOf(MyStruct{}))
```

### 2. Organize Generated Files

Keep generated files separate and use proper prefix of sufix:

```
types/
  block.go        # Source
  state.go        # Source
  gen_ssz.go      # Generated
```

### 3. Version Control

Add to `.gitignore` if regenerating frequently:

```gitignore
# Generated SSZ files
*_ssz.go
```

Or commit them for reproducible builds without generation step.

### 4. Validation

Before generation:
- Ensure all fields have proper tags
- Dynamic fields must have `ssz-max` tags
- Union types must have valid variants
- Bitlists/bitvectors must use byte arrays

## Troubleshooting

### Common Issues

#### "Type not found"

Ensure the type is:
- Exported (starts with capital letter)
- In the specified package
- Not in a test file

#### "Interface types not supported"

SSZ doesn't support interfaces. Use concrete types or unions:

```go
// ❌ Bad
type Container struct {
    Data interface{}
}

// ✅ Good - Concrete type
type Container struct {
    Data DataType
}

// ✅ Good - Union type
type Data = dynssz.CompatibleUnion[struct {
    TypeA *TypeA
    TypeB *TypeB
}]
```

#### Generated code doesn't compile

Check for:
- Circular dependencies
- Missing imports (tool should handle automatically)
- Invalid struct tags
- Unsupported field types

Please send code that doesn't compile as bug report.

### Debugging

Use verbose mode:

```bash
dynssz-gen -package ./types -types "Problem" -output debug.go -v
```

Verbose output shows:
- Type analysis results
- Detected SSZ types
- Tag parsing information
- Generation decisions

### Performance Tips

1. **Use code generation** for production - 2-3x faster than reflection
2. **Generate once** during build, not at runtime
4. **Use static expressions** (`WithoutDynamicExpressions()`) when specs don't change

## Migration from FastSSZ

### From sszgen

```bash
# Before: fastssz with sszgen
sszgen --path . --type Block --out block_ssz.go

# After: dynamic-ssz CLI
dynssz-gen -package . -types "Block" -output block_ssz.go
```

### From go:generate

```go
// Before: fastssz
//go:generate sszgen --path . --type Block

// After: dynamic-ssz
//go:generate dynssz-gen -package . -types "Block" -output generated_ssz.go
```

### API Compatibility

Generated legacy methods are compatible with fastssz:

```go
// These work with both fastssz and dynamic-ssz generated code
data, _ := block.MarshalSSZ()
size := block.SizeSSZ()
_ = block.UnmarshalSSZ(data)
root, _ := block.HashTreeRoot()
```

## Complete Example

### Project Structure

```
myproject/
├── types/
│   ├── types.go        # Type definitions
│   └── types_ssz.go    # Generated code
├── Makefile
└── go.mod
```

### types/types.go

```go
package types

type Block struct {
    Number    uint64
    Hash      [32]byte
    Txs       []Transaction `ssz-max:"1000000" dynssz-max:"MAX_TRANSACTIONS"`
    Validator [48]byte      
}

type Transaction struct {
    From   [20]byte
    To     [20]byte  
    Value  [32]byte
    Data   []byte `ssz-max:"1000000" dynssz-max:"MAX_TX_DATA"`
}
```

### Makefile

```makefile
generate:
	dynssz-gen -package ./types -types "Block,Transaction" -output types/types_ssz.go

build: generate
	go build ./...

test: generate
	go test ./...

.PHONY: generate build test
```

### Usage

```go
package main

import (
    dynssz "github.com/pk910/dynamic-ssz"
    "myproject/types"
)

func main() {
    // Create specs
    specs := map[string]any{
        "MAX_TRANSACTIONS": uint64(2048),
        "MAX_TX_DATA": uint64(131072),
    }
    ds := dynssz.NewDynSsz(specs)
    
    // Create and encode block
    block := &types.Block{
        Number: 12345,
        // ...
    }
    
    // Option 1: Use DynSsz wrapper (easiest)
    data, err := ds.MarshalSSZ(block)
    
    // Option 2: Use generated dynamic methods directly
    buf := make([]byte, 0, block.SizeSSZDyn(ds))
    data, err = block.MarshalSSZDyn(ds, buf)
    
    // Option 3: Use legacy methods (if generated)
    data, err = block.MarshalSSZ()
}
```

## See Also

- [Getting Started Guide](getting-started.md)
- [Struct Tags Reference](struct-tags.md)
- [API Reference](api-reference.md)
- [Examples](../examples/)