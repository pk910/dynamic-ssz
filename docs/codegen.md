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

The generated file provides these methods for each type:

```go
// Marshaling
func (t *MyStruct) MarshalSSZ() ([]byte, error)
func (t *MyStruct) MarshalSSZTo(buf []byte) ([]byte, error)
func (t *MyStruct) SizeSSZ() int

// Unmarshaling
func (t *MyStruct) UnmarshalSSZ(buf []byte) error

// Hash Tree Root
func (t *MyStruct) HashTreeRoot() ([32]byte, error)
func (t *MyStruct) HashTreeRootWith(hh ssz.HashWalker) error
```

## Advanced Configuration

### Generator Options

```go
generator := codegen.NewCodeGenerator(nil)

// Generate code for multiple files
generator.BuildFile(
    "user_types_ssz.go",
    codegen.WithType(reflect.TypeOf(User{})),
    codegen.WithType(reflect.TypeOf(Account{})),
    codegen.WithCreateLegacyFn(), // Generate legacy method signatures
)

generator.BuildFile(
    "game_types_ssz.go",
    codegen.WithType(reflect.TypeOf(GameState{})),
    codegen.WithoutDynamicExpressions(), // Disable dynamic expressions
)
```

### Package Management

The generator automatically handles imports and package declarations:

```go
generator.BuildFile(
    "internal/generated/ssz.go", // Output path determines package
    codegen.WithType(reflect.TypeOf(MyType{})),
)
```

## Complete Example

### Project Structure

```
myproject/
├── types/
│   └── types.go        # Your type definitions
├── codegen/
│   └── main.go         # Code generator
├── generated/
│   └── types_ssz.go    # Generated code (git-ignored)
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
        "generated/types_ssz.go",
        codegen.WithType(reflect.TypeOf(types.Block{})),
        codegen.WithType(reflect.TypeOf(types.Transaction{})),
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
	@go run codegen/main.go

.PHONY: build
build: generate
	go build ./...

.PHONY: test
test: generate
	go test ./...
```

### Using Generated Code

```go
package main

import (
    "myproject/types"
    _ "myproject/generated" // Import for side effects
)

func main() {
    block := &types.Block{
        Number: 12345,
        // ...
    }
    
    // Use generated methods
    data, err := block.MarshalSSZ()
    if err != nil {
        panic(err)
    }
    
    // Unmarshal
    newBlock := &types.Block{}
    err = newBlock.UnmarshalSSZ(data)
    
    // Hash tree root
    root, err := block.HashTreeRoot()
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

## Performance Optimization

The code generator implements several optimizations:

### 1. Function Inlining

For simple types and FastSSZ-compatible types, the generator inlines function calls:

```go
// Instead of:
err = fn1(t.Field)

// Generated:
err = t.Field.HashTreeRootWith(hh)
```

### 2. Static Size Calculation

Sizes are pre-calculated where possible:

```go
// Dynamic calculation avoided
size := 184 + len(t.DynamicField)*32
```

### 3. Optimized Memory Allocation

```go
// Pre-allocate exact size
buf := make([]byte, 0, t.SizeSSZ())
data, err := t.MarshalSSZTo(buf)
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

## Migration Guide

### From FastSSZ

```go
// Before: fastssz with sszgen
//go:generate sszgen --path . --type Block

// After: dynamic-ssz codegen
//go:generate go run codegen/main.go
```

### From Reflection

```go
// Before: Pure reflection
data, err := dynssz.MarshalSSZ(block)

// After: With generated code
data, err := block.MarshalSSZ()
```

The generated methods are compatible with the reflection-based API, allowing gradual migration.