# Code Generation Example

This example demonstrates how to use Dynamic SSZ's code generation feature to create highly optimized SSZ methods for your types.

## Overview

The code generator creates static SSZ methods that are 2-3x faster than reflection-based encoding. This example includes:

- Custom types with various SSZ features
- Code generation setup 
- Performance comparison
- Multi-dimensional arrays
- Bitfields and large integers

## Quick Start

1. **Run the example with reflection (slower):**
   ```bash
   go run .
   ```

2. **Generate optimized SSZ code:**
   ```bash
   go run ./codegen
   ```

3. **Run again with generated code (2-3x faster):**
   ```bash
   go run .
   ```

## What's Included

### Types (`types/types.go`)

- **User** - Basic struct with dynamic fields and bitlist
- **Transaction** - Ethereum-style transaction with uint256 values  
- **Block** - Block structure containing multiple transactions
- **GameState** - Complex example with 2D arrays and multi-dimensional slices
- **Player/Move/Tile** - Supporting types for the game state

### Generator (`cmd/main.go`) 

Shows how to set up code generation for multiple types:

```go
generator.BuildFile(
    "generated_ssz.go",
    codegen.WithType(reflect.TypeOf(User{})),
    codegen.WithType(reflect.TypeOf(Transaction{})),
    // ... more types
    codegen.WithCreateLegacyFn(), // Generate HashTreeRoot() methods
)
```

### Demo Application (`main.go`)

- Tests all generated types
- Shows encoding/decoding roundtrips  
- Computes hash tree roots
- Basic performance benchmarking

## Generated Methods

After running the generator, each type gets these methods:

```go
// Marshaling
func (t *User) MarshalSSZ() ([]byte, error)
func (t *User) MarshalSSZTo(buf []byte) ([]byte, error) 
func (t *User) SizeSSZ() int

// Unmarshaling  
func (t *User) UnmarshalSSZ(buf []byte) error

// Hash Tree Root
func (t *User) HashTreeRoot() ([32]byte, error)
func (t *User) HashTreeRootWith(hh ssz.HashWalker) error
```

## Key Features Demonstrated

### 1. Dynamic Fields
```go
type User struct {
    Name []byte `ssz-max:"64"`      // Variable length
    Roles []byte `ssz-type:"bitlist" ssz-max:"32"` // Bitfield
}
```

### 2. Large Integers
```go  
type Transaction struct {
    Value [32]byte `ssz-type:"uint256"` // 256-bit integer
}
```

### 3. Multi-dimensional Arrays
```go
type GameState struct {
    Board [8][8]uint8                           // Fixed 2D array
    MapTiles [][]Tile `ssz-size:"?,16" ssz-max:"256"` // Dynamic 2D
}
```

### 4. Performance Optimization

The generated code includes optimizations like:
- Function inlining for simple types
- Pre-calculated sizes where possible
- Direct method calls instead of wrapper functions

## File Structure

```
examples/codegen/
├── go.mod              # Module definition
├── types/
│   └── types.go        # Type definitions
├── cmd/
│   └── main.go         # Code generator
├── main.go             # Demo application
├── generated_ssz.go    # Generated code (after running generator)
└── README.md           # This file
```

## Next Steps

1. **Add your own types** - Define custom structs in `types/types.go`
2. **Update the generator** - Add your types to `cmd/main.go`  
3. **Regenerate** - Run `go run cmd/main.go`
4. **Use in your app** - Import and use the generated methods

## Integration Tips

- Add generated files to `.gitignore`
- Use `go:generate` for automatic generation
- Include generation step in build process
- Profile to identify which types benefit most from generation