# Code Generation Guide

Dynamic SSZ includes a powerful code generator that creates optimized static code for your types, providing 2-3x performance improvements over reflection-based encoding.

## Overview

The code generator creates specialized SSZ methods for your types at compile time, eliminating the reflection overhead. This is especially beneficial for performance-critical applications.

### Key Benefits

- **2-3x Performance Improvement** - Static code eliminates reflection overhead
- **Type Safety** - Compile-time verification of generated code
- **Debugging** - Generated code is readable and debuggable
- **Compatibility** - Works seamlessly with existing Dynamic SSZ features

## How It Works

### Reflection-Based Approach

Dynamic SSZ's code generator uses Go's reflection instead of AST parsing. This unique approach provides several advantages:

1. **Fully Resolved Types** - The Go linker provides complete type information, including types from external packages
2. **Simplified Generation** - No need to parse Go syntax or resolve imports manually
3. **Type Safety** - All types are validated by the Go compiler before generation

### Trade-offs

The main trade-off is that code generation cannot be done via a standalone CLI tool. The generator must be compiled together with your types, requiring a small Go program to invoke the generator.

## Basic Usage

### Step 1: Create a Generator Program

Create a new file (e.g., `codegen/main.go`):

```go
package main

import (
    "log"
    "reflect"
    
    "github.com/pk910/dynamic-ssz/codegen"
    "your-module/types"
)

func main() {
    // Create a code generator instance
    generator := codegen.NewCodeGenerator(nil)
    
    // Add types to generate code for
    generator.BuildFile(
        "generated/types_ssz.go",
        codegen.WithType(reflect.TypeOf(&types.MyStruct{})),
        codegen.WithType(reflect.TypeOf(&types.AnotherStruct{})),
    )
    
    // Generate the code
    if err := generator.Generate(); err != nil {
        log.Fatal("Generation failed:", err)
    }
}
```

### Step 2: Run the Generator

```bash
go run codegen/main.go
```

This creates `generated/types_ssz.go` with optimized SSZ methods for your types.

### Step 3: Use the Generated Code

By default, the generator creates dynamic methods that support runtime configuration:

```go
// Dynamic methods (default)
func (t *MyStruct) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error)
func (t *MyStruct) SizeSSZDyn(ds sszutils.DynamicSpecs) int
func (t *MyStruct) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) error
func (t *MyStruct) HashTreeRootDyn(ds sszutils.DynamicSpecs) ([32]byte, error)
func (t *MyStruct) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error
```

With `WithCreateLegacyFn()`, it also generates legacy methods:

```go
// Legacy methods (compatible with fastSSZ)
func (t *MyStruct) MarshalSSZ() ([]byte, error)
func (t *MyStruct) MarshalSSZTo(buf []byte) ([]byte, error) 
func (t *MyStruct) SizeSSZ() int
func (t *MyStruct) UnmarshalSSZ(buf []byte) error
func (t *MyStruct) HashTreeRoot() ([32]byte, error)
func (t *MyStruct) HashTreeRootWith(hh sszutils.HashWalker) error
```

## Method Types

### Dynamic vs Legacy Methods

Dynamic SSZ supports two types of generated methods:

#### Dynamic Methods (Default)
These methods accept a `DynamicSpecs` provider that resolves specification values at runtime:

```go
// Create a DynSSZ instance with your specifications
specs := map[string]any{
    "MAX_VALIDATORS": uint64(2048),
    "SYNC_COMMITTEE_SIZE": uint64(512),
}
ds := dynssz.NewDynSsz(specs)

// Dynamic methods take the DynamicSpecs provider
buf := make([]byte, 0, block.SizeSSZDyn(ds))
data, err := block.MarshalSSZDyn(ds, buf)
size := block.SizeSSZDyn(ds)
err = block.UnmarshalSSZDyn(ds, data)
root, err := block.HashTreeRootDyn(ds)
```

#### Legacy Methods (FastSSZ Compatible)
These methods use compile-time configuration and are compatible with fastSSZ:

```go
// Legacy methods (compile-time configuration)
data, err := block.MarshalSSZ()
size := block.SizeSSZ()
err = block.UnmarshalSSZ(data)
root, err := block.HashTreeRoot()
err = block.HashTreeRootWith(hasher)
```

### Choosing Method Types

- **Dynamic methods only (default)**: Maximum flexibility, supports all presets
- **Legacy methods only**: Maximum compatibility with fastSSZ ecosystem  
- **Both**: Best of both worlds - flexibility when needed, compatibility everywhere

## Advanced Configuration

### File-Level Options

```go
generator := codegen.NewCodeGenerator(nil)

// File 1: Full dynamic support
generator.BuildFile(
    "dynamic_types_ssz.go",
    codegen.WithType(reflect.TypeOf(BeaconState{})),
    codegen.WithType(reflect.TypeOf(BeaconBlock{})),
    codegen.WithCreateLegacyFn(), // Add legacy methods too
)

// File 2: Legacy-only for maximum performance on main preset
generator.BuildFile(
    "legacy_types_ssz.go",
    codegen.WithType(reflect.TypeOf(SimpleStruct{})),
    codegen.WithoutDynamicExpressions(), // Legacy only
)
```

### Type-Level Options

Control which methods are generated per type:

```go
generator.BuildFile(
    "selective_ssz.go",
    codegen.WithType(
        reflect.TypeOf(ReadOnlyType{}),
        codegen.WithNoMarshalSSZ(),   // Skip marshal methods
        codegen.WithNoSizeSSZ(),      // Skip size method
    ),
    codegen.WithType(
        reflect.TypeOf(HashOnlyType{}),
        codegen.WithNoMarshalSSZ(),   // Only generate hash methods
        codegen.WithNoUnmarshalSSZ(),
        codegen.WithNoSizeSSZ(),
    ),
)
```

### Available Options

- `WithCreateLegacyFn()` - Generate legacy methods alongside dynamic ones
- `WithoutDynamicExpressions()` - Generate legacy methods only, ignore all dynamic expressions
- `WithNoMarshalSSZ()` - Skip `MarshalSSZ`/`MarshalSSZDyn` methods
- `WithNoUnmarshalSSZ()` - Skip `UnmarshalSSZ`/`UnmarshalSSZDyn` methods
- `WithNoSizeSSZ()` - Skip `SizeSSZ`/`SizeSSZDyn` methods
- `WithNoHashTreeRoot()` - Skip all hash tree root methods

### DynamicSpecs Interface

The `sszutils.DynamicSpecs` interface provides specification value resolution:

```go
type DynamicSpecs interface {
    ResolveSpecValue(name string) (bool, uint64, error)
}
```

The main implementation is the `*DynSsz` struct, which resolves values from the specs map:

```go
// Create DynSSZ with specifications
ds := dynssz.NewDynSsz(map[string]any{
    "VALIDATOR_REGISTRY_LIMIT": uint64(1099511627776),
    "SYNC_COMMITTEE_SIZE": uint64(512),
})

// The DynSsz instance implements DynamicSpecs
// Generated code calls ds.ResolveSpecValue("VALIDATOR_REGISTRY_LIMIT")
```

### Preset Compatibility

#### With Dynamic Expressions (Default)
```go
// Supports all presets at runtime
type BeaconState struct {
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

// Usage with different presets
mainnetDS := dynssz.NewDynSsz(map[string]any{
    "VALIDATOR_REGISTRY_LIMIT": uint64(1099511627776),
})
minimalDS := dynssz.NewDynSsz(map[string]any{
    "VALIDATOR_REGISTRY_LIMIT": uint64(64),
})

buf := make([]byte, 0)
data1, _ := state.MarshalSSZDyn(mainnetDS, buf)  // Uses mainnet limit
data2, _ := state.MarshalSSZDyn(minimalDS, buf)  // Uses minimal limit
```

#### Without Dynamic Expressions
```go
// Generated code uses static values from struct tags
// Only works with default preset (values from ssz-max tags)
codegen.WithoutDynamicExpressions()

// The dynssz library automatically chooses:
// - Generated code for default preset
// - Reflection for other presets

buf := make([]byte, 0)
data1, _ := mainnetDS.MarshalSSZTo(state, buf)  // Uses generated code in behind
data2, _ := minimalDS.MarshalSSZTo(state, buf)  // Uses reflection in behind
```

### Package Management

The generator determines the package from the types being processed. **All types in a single `BuildFile()` call must be from the same package** - mixing types from different packages is not allowed.

The generated file will be placed in the same package as the input types:

```go
// ✅ Correct: All types from same package (types)
generator.BuildFile(
    "types/generated_ssz.go",  // Must be in same directory as types
    codegen.WithType(reflect.TypeOf(types.BeaconState{})),
    codegen.WithType(reflect.TypeOf(types.BeaconBlock{})),
)

// ❌ Error: Mixing packages not allowed
generator.BuildFile(
    "mixed_ssz.go",
    codegen.WithType(reflect.TypeOf(types.BeaconState{})),    // package types
    codegen.WithType(reflect.TypeOf(network.Message{})),     // package network - ERROR!
)

// ✅ Correct: Separate files for different packages
generator.BuildFile(
    "types/types_ssz.go",
    codegen.WithType(reflect.TypeOf(types.BeaconState{})),
    codegen.WithType(reflect.TypeOf(types.BeaconBlock{})),
)

generator.BuildFile(
    "network/network_ssz.go",
    codegen.WithType(reflect.TypeOf(network.Message{})),
    codegen.WithType(reflect.TypeOf(network.Request{})),
)
```

**Key Rules:**
- Generated file must be in the same directory as the source types
- All types in one file must belong to the same Go package
- Package name is automatically determined from the first type
- Import statements are automatically generated

## Multi-File Generation

### Cross-File Type Linking

The generator automatically links types across all generated files and existing fastSSZ/dynSSZ methods:

```go
// File 1: Core types
generator.BuildFile(
    "core_ssz.go",
    codegen.WithType(reflect.TypeOf(BeaconState{})),
    codegen.WithType(reflect.TypeOf(BeaconBlock{})), // References Transaction
)

// File 2: Transaction types  
generator.BuildFile(
    "tx_ssz.go",
    codegen.WithType(reflect.TypeOf(Transaction{})),
    codegen.WithType(reflect.TypeOf(TransactionPool{})), // References Transaction
)

// The generated code automatically uses:
// - Generated methods from other files when available
// - Existing fastSSZ methods when present
// - Reflection fallback when neither exists
```

## Complete Example

### Project Structure

```
myproject/
├── types/
│   ├── types.go        # Your type definitions
│   └── types_ssz.go    # Generated SSZ code
├── codegen/
│   └── main.go         # Code generator
├── main.go
└── go.mod
```

### types/types.go

```go
package types

import "github.com/your/imports"

type Block struct {
    Number    uint64
    Hash      [32]byte
    Txs       []Transaction `ssz-max:"1000000"`
    Validator [48]byte      `ssz-type:"bytes48"`
}

type Transaction struct {
    From   [20]byte
    To     [20]byte
    Value  [32]byte  `ssz-type:"uint256"`
    Data   []byte    `ssz-max:"1000000"`
}
```

### codegen/main.go

```go
//go:build ignore

package main

import (
    "log"
    "reflect"
    
    "github.com/pk910/dynamic-ssz/codegen"
    "myproject/types"
)

func main() {
    generator := codegen.NewCodeGenerator(nil)
    
    generator.BuildFile(
        "types/types_ssz.go",
        codegen.WithType(reflect.TypeOf(&types.Block{})),
        codegen.WithType(reflect.TypeOf(&types.Transaction{})),
        codegen.WithCreateLegacyFn(),
    )
    
    if err := generator.Generate(); err != nil {
        log.Fatal(err)
    }
    
    log.Println("Code generation completed successfully")
}
```

### Makefile

```makefile
.PHONY: generate
generate:
	@echo "Generating SSZ code..."
	@go run ./codegen

.PHONY: build
build: generate
	go build ./...

.PHONY: test
test: generate
	go test ./...
```

### Using Generated Code

#### Hybrid Usage with DynSSZ

```go
package main

import (
    dynssz "github.com/pk910/dynamic-ssz"
    "myproject/types"
)

func main() {
    block := &types.Block{
        Number: 12345,
        // ...
    }
    
    // Create DynSSZ instance with your specifications
    specs := map[string]any{
        "MAX_TRANSACTIONS": uint64(1048576),
        "MAX_BYTES_PER_TRANSACTION": uint64(131072),
    }
    ds := dynssz.NewDynSsz(specs)
    
    // Use dynamic generated methods
    data, err := ds.MarshalSSZ(block)
    if err != nil {
        panic(err)
    }
    
    // Unmarshal
    newBlock := &types.Block{}
    err = ds.UnmarshalSSZ(newBlock, data)
    
    // Hash tree root
    root, err := ds.HashTreeRoot(block)
}
```

#### With Dynamic Methods

```go
package main

import (
    dynssz "github.com/pk910/dynamic-ssz"
    "myproject/types"
)

func main() {
    block := &types.Block{
        Number: 12345,
        // ...
    }
    
    // Create DynSSZ instance with your specifications
    specs := map[string]any{
        "MAX_TRANSACTIONS": uint64(1048576),
        "MAX_BYTES_PER_TRANSACTION": uint64(131072),
    }
    ds := dynssz.NewDynSsz(specs)
    
    // Use dynamic generated methods
    buf := make([]byte, 0, block.SizeSSZDyn(ds))
    data, err := block.MarshalSSZDyn(ds, buf)
    if err != nil {
        panic(err)
    }
    
    // Unmarshal
    newBlock := &types.Block{}
    err = newBlock.UnmarshalSSZDyn(ds, data)
    
    // Hash tree root
    root, err := block.HashTreeRootDyn(ds)
}
```

#### With Legacy Methods

```go
func main() {
    block := &types.Block{
        Number: 12345,
        // ...
    }
    
    // Use legacy generated methods (fastSSZ compatible)
    data, err := block.MarshalSSZ()
    if err != nil {
        panic(err)
    }
    
    // Unmarshal
    newBlock := &types.Block{}
    err = newBlock.UnmarshalSSZ(data)
    
    // Hash tree root
    root, err := block.HashTreeRoot()
    
    // Or with custom hasher
    hasher := ssz.NewHasher()
    err = block.HashTreeRootWith(hasher)
    root = hasher.Hash()
}
```

## Integration with Build Tools

### go:generate

Add to your types file:

```go
//go:generate go run ../codegen/main.go
package types
```

Then run:
```bash
go generate ./...
```

### CI/CD Integration

```yaml
# .github/workflows/ci.yml
steps:
  - name: Generate code
    run: go run codegen/main.go
  
  - name: Build
    run: go build ./...
  
  - name: Test
    run: go test ./...
```

## Troubleshooting

### Common Issues

1. **Import Errors**
   ```
   Solution: Ensure all types are properly imported in your generator
   ```

2. **Type Not Found**
   ```
   Solution: Use the full type including package name
   ```

3. **Generated Code Doesn't Compile**
   ```
   Solution: Check that all struct tags are valid
   ```

### Debugging Generated Code

The generated code is designed to be readable:

```go
// Generated code includes helpful comments
fn3 := func(t []*Transaction) (err error) { // []*Transaction:MAX_TRANSACTIONS
    // ... implementation
}
```

## Best Practices

1. **Version Control**
   - Add generated files to `.gitignore`
   - Commit the generator code
   - Generate during build process

2. **Organization**
   - Keep generator in separate directory
   - Group related types in same generated file
   - Use clear naming conventions

3. **Performance**
   - Generate code for hot-path types
   - Use dynamic SSZ for rarely-used types
   - Profile to identify optimization targets

4. **Maintenance**
   - Regenerate when types change
   - Use go:generate for automation
   - Include generation in CI/CD

## Migration from FastSSZ

```go
// Before: fastssz with sszgen
//go:generate sszgen --path . --type Block

// After: dynamic-ssz codegen
//go:generate go run codegen/main.go
```

The generated legacy methods are compatible with the fastssz API, allowing gradual migration.
