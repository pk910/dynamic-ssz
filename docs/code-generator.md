# Code Generator

Dynamic SSZ includes a code generation tool (`dynssz-gen`) that generates optimized SSZ marshaling code for your types, eliminating reflection overhead and improving performance.

## Installation

```bash
go install github.com/pk910/dynamic-ssz/dynssz-gen@latest
```

Verify installation:
```bash
dynssz-gen -help
```

## CLI Usage

### Basic Usage

Generate SSZ methods for specific types:

```bash
dynssz-gen -package /path/to/package -types BeaconBlock,BeaconState -output generated_ssz.go
```

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-package` | Go package path to analyze | Required |
| `-types` | Comma-separated list of types to generate | Required |
| `-output` | Output file path | Required if types don't specify files |
| `-v` | Verbose output | `false` |
| `-legacy` | Generate legacy compatibility methods | `false` |
| `-without-dynamic-expressions` | Generate only legacy methods, disable dynamic methods | `false` |

### Examples

#### Generate All Types to Single File

```bash
# In your project directory
dynssz-gen -package . -types Block,Transaction,Validator -output ssz_generated.go
```

#### Generate Types to Separate Files

```bash
# Each type to its own file
dynssz-gen -package . -types Block:block_ssz.go,Transaction:tx_ssz.go,Validator:validator_ssz.go
```

#### Mixed File Specification

```bash
# Some types to separate files, others to default output
dynssz-gen -package . -types Block:block_ssz.go,Transaction:tx_ssz.go,Validator,ValidatorData -output validators_ssz.go
```

#### Generate for External Package

```bash
dynssz-gen -package github.com/myproject/types -types BeaconBlock -output beacon_ssz.go
```

#### Generate with Verbose Output

```bash
dynssz-gen -v -package . -types State,Block -output ssz_methods.go
```

#### Generate Legacy Methods

For compatibility with existing fastssz code:

```bash
dynssz-gen -legacy -package . -types BeaconState -output legacy_ssz.go
```

#### Generate Static Methods Only

For maximum performance with default preset:

```bash
dynssz-gen -without-dynamic-expressions -package . -types BeaconState -output static_ssz.go
```

#### Generate Both Dynamic and Legacy

For maximum flexibility:

```bash
dynssz-gen -legacy -package . -types BeaconState -output full_ssz.go
```

## Programmatic API

### Basic Example

```go
package main

import (
    "github.com/pk910/dynamic-ssz/codegen"
    dynssz "github.com/pk910/dynamic-ssz"
)

func generateSSZ() error {
    // Create code generator
    codeGen := codegen.NewCodeGenerator(dynssz.NewDynSsz(nil))
    
    // Add types to single file
    codeGen.BuildFile("generated_ssz.go", 
        codegen.WithReflectType(reflect.TypeOf(BeaconBlock{})),
        codegen.WithReflectType(reflect.TypeOf(BeaconState{})),
    )
    
    // Generate code
    return codeGen.Generate()
}
```

### Multiple Files Example

```go
func generateSSZMultipleFiles() error {
    codeGen := codegen.NewCodeGenerator(dynssz.NewDynSsz(nil))
    
    // Block types to one file
    codeGen.BuildFile("block_ssz.go",
        codegen.WithReflectType(reflect.TypeOf(BeaconBlock{})),
        codegen.WithReflectType(reflect.TypeOf(BeaconBlockBody{})),
    )
    
    // State types to another file
    codeGen.BuildFile("state_ssz.go",
        codegen.WithReflectType(reflect.TypeOf(BeaconState{})),
        codegen.WithReflectType(reflect.TypeOf(Validator{})),
    )
    
    // Cross-references between types are handled automatically
    return codeGen.Generate()
}
```

### Generator Options

```go
import "github.com/pk910/dynamic-ssz/codegen"

// Create generator with options
codeGen := codegen.NewCodeGenerator(dynSsz)

// Build file with options
codeGen.BuildFile("output.go",
    // Skip marshal method generation
    codegen.WithNoMarshalSSZ(),
    
    // Skip unmarshal method generation
    codegen.WithNoUnmarshalSSZ(),
    
    // Skip size calculation
    codegen.WithNoSizeSSZ(),
    
    // Skip hash tree root generation
    codegen.WithNoHashTreeRoot(),
    
    // Generate legacy methods (adds MarshalSSZ, UnmarshalSSZ, etc.)
    codegen.WithCreateLegacyFn(),
    
    // Skip dynamic expressions (generates only static legacy methods)
    codegen.WithoutDynamicExpressions(),
    
    // Add type to generate
    codegen.WithReflectType(reflect.TypeOf(MyType{})),
)
```

## Generated Methods

The code generator produces different methods depending on the flags used:

### Default: Dynamic Methods (Spec-Aware)

By default, the generator creates dynamic methods that accept specification values:

#### MarshalSSZDyn

```go
func (b *BeaconBlock) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) (dst []byte, err error) {
    // Generated marshaling with dynamic expression support
}
```

#### UnmarshalSSZDyn

```go
func (b *BeaconBlock) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) (err error) {
    // Generated unmarshaling with dynamic expression support
}
```

#### SizeSSZDyn

```go
func (b *BeaconBlock) SizeSSZDyn(ds sszutils.DynamicSpecs) (size int) {
    // Size calculation with dynamic expression support
}
```

#### HashTreeRootWithDyn

```go
func (b *BeaconBlock) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
    // Hash tree root with dynamic expression support
}
```

### With `-legacy`: Additional Legacy Methods

When using `-legacy` flag, additional fastssz-compatible methods are generated:

#### MarshalSSZ & MarshalSSZTo

```go
func (b *BeaconBlock) MarshalSSZ() ([]byte, error) {
    return b.MarshalSSZDyn(dynssz.GetGlobalDynSsz(), nil)
}

func (b *BeaconBlock) MarshalSSZTo(buf []byte) ([]byte, error) {
    return b.MarshalSSZDyn(dynssz.GetGlobalDynSsz(), buf)
}
```

#### UnmarshalSSZ

```go
func (b *BeaconBlock) UnmarshalSSZ(buf []byte) error {
    return b.UnmarshalSSZDyn(dynssz.GetGlobalDynSsz(), buf)
}
```

#### SizeSSZ

```go
func (b *BeaconBlock) SizeSSZ() int {
    return b.SizeSSZDyn(dynssz.GetGlobalDynSsz())
}
```

#### HashTreeRoot

```go
func (b *BeaconBlock) HashTreeRoot() ([32]byte, error) {
    // Optimized merkle root calculation using global specs
}
```

### With `-without-dynamic-expressions`: Static Methods Only

When using `-without-dynamic-expressions`, only static legacy methods are generated (no `*Dyn` methods):

```go
func (b *BeaconBlock) MarshalSSZ() ([]byte, error) {
    // Static marshaling for default preset only
}

func (b *BeaconBlock) MarshalSSZTo(buf []byte) ([]byte, error) {
    // Static marshaling to buffer
}

func (b *BeaconBlock) UnmarshalSSZ(buf []byte) error {
    // Static unmarshaling
}

func (b *BeaconBlock) SizeSSZ() int {
    // Static size calculation
}

func (b *BeaconBlock) HashTreeRoot() ([32]byte, error) {
    // Static hash tree root
}
```

**Use case**: Generate optimized code for the default preset while falling back to reflection for other presets.

## Integration with Build Process

### Using go:generate

Add generation directives to your code:

```go
//go:generate dynssz-gen -package . -types BeaconBlock,BeaconState -output generated_ssz.go

package types

type BeaconBlock struct {
    Slot          uint64
    ProposerIndex uint64
    ParentRoot    [32]byte
    StateRoot     [32]byte
    Body          BeaconBlockBody
}
```

Run generation:
```bash
go generate ./...
```

### Makefile Integration

```makefile
.PHONY: generate-ssz
generate-ssz:
    dynssz-gen -package . -types Block,State,Transaction -output ssz_generated.go

.PHONY: build
build: generate-ssz
    go build ./...
```

### CI/CD Integration

GitHub Actions example:

```yaml
name: Build
on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Install dynssz-gen
        run: go install github.com/pk910/dynamic-ssz/dynssz-gen@latest
      
      - name: Generate SSZ code
        run: dynssz-gen -package . -types BeaconBlock,BeaconState -output generated_ssz.go
      
      - name: Build
        run: go build ./...
      
      - name: Test
        run: go test ./...
```

## Type Support

The generator supports all Dynamic SSZ types and annotations:

### Basic Types
- All unsigned integers: `uint8`, `uint16`, `uint32`, `uint64`
- Boolean: `bool`
- Fixed arrays: `[N]T`
- Slices: `[]T`
- Byte arrays: `[]byte`, `[N]byte`
- Strings: `string`

### Advanced Types
- Large integers: byte arrays or uint64 arrays with `ssz-type:"uint256"`
- Bitfields
- Progressive types
- Unions: `CompatibleUnion[T]`
- Type wrappers: `TypeWrapper[D, T]`

### Annotations
All SSZ annotations are supported:
- `ssz-size`
- `ssz-max`
- `ssz-type`
- `dynssz-size`
- `dynssz-max`
- `ssz-index`

## Performance Benefits

### Benchmark Comparison

```go
// Reflection-based (Dynamic SSZ runtime)
BenchmarkMarshalReflection-8       50000     30142 ns/op
BenchmarkUnmarshalReflection-8     30000     45231 ns/op

// Generated code
BenchmarkMarshalGenerated-8       500000      2341 ns/op
BenchmarkUnmarshalGenerated-8     300000      4126 ns/op
```

### Memory Efficiency

Generated code:
- Eliminates reflection overhead
- Pre-calculates sizes
- Optimizes buffer allocation
- Reduces allocations

## Advanced Features

### Cross-Reference Handling

The code generator automatically handles cross-references between types when they're generated together. This prevents massive code duplication.

**How it works:**
1. When generating multiple types, the generator analyzes all types first
2. If a type references another type being generated, it calls the generated method on that type
3. If the generator doesn't know about a referenced type, it duplicates the entire marshaling logic inline
4. Cross-references work across files when types are generated in the same batch

**Example:**
```go
type BeaconBlock struct {
    Body BeaconBlockBody  // Reference to another type
}

type BeaconBlockBody struct {
    Attestations []Attestation `ssz-max:"128"`
}
```

**With cross-reference detection (generating together):**
```bash
dynssz-gen -package . -types BeaconBlock,BeaconBlockBody -output beacon_ssz.go
```

Generated code calls the method:
```go
func (b *BeaconBlock) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
    // ... other fields
    if dst, err = b.Body.MarshalSSZDyn(ds, dst); err != nil {
        return dst, err
    }
    // ...
}
```

**Without cross-reference detection (generating separately):**
```bash
dynssz-gen -package . -types BeaconBlock -output block_ssz.go  # BeaconBlockBody not included
```

Generated code duplicates the entire BeaconBlockBody marshaling logic inline, leading to massive code duplication.

**Multiple Files with Cross-References:**
```bash
# These types can reference each other without code duplication
dynssz-gen -package . -types BeaconBlock:block_ssz.go,BeaconBlockBody:block_ssz.go,Attestation:attestation_ssz.go
```

### Custom Type Handling

The generator recognizes custom SSZ interfaces:

```go
type CustomType struct {
    // If type implements MarshalSSZ/UnmarshalSSZ,
    // generator calls those methods
}

func (c *CustomType) MarshalSSZ() ([]byte, error) {
    // Custom implementation
}
```

### Dynamic Expression Support

By default, dynamic methods support runtime specification values:

```go
type State struct {
    Validators []Validator `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

// Generated dynamic method
func (s *State) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
    maxValidators := ds.GetValue("VALIDATOR_REGISTRY_LIMIT")
    // ... use dynamic value for different presets
}
```

### Static Expression Optimization

With `-without-dynamic-expressions`, expressions are resolved at generation time:

```go
// Generated static method (assuming default preset values)
func (s *State) MarshalSSZ() ([]byte, error) {
    // Hard-coded limits for maximum performance
    // Falls back to reflection for non-default presets
}
```

### Method Generation Modes

#### Default Mode (Dynamic Only)
```bash
dynssz-gen -package . -types BeaconBlock -output block_ssz.go
```
Generates: `MarshalSSZDyn`, `UnmarshalSSZDyn`, `SizeSSZDyn`, `HashTreeRootWithDyn`

#### Legacy Mode (Dynamic + Legacy)
```bash
dynssz-gen -legacy -package . -types BeaconBlock -output block_ssz.go
```
Generates: All dynamic methods + `MarshalSSZ`, `UnmarshalSSZ`, `SizeSSZ`, `HashTreeRoot`, `MarshalSSZTo`

#### Static Mode (Legacy Only)
```bash
dynssz-gen -without-dynamic-expressions -package . -types BeaconBlock -output block_ssz.go
```
Generates: Only `MarshalSSZ`, `UnmarshalSSZ`, `SizeSSZ`, `HashTreeRoot`, `MarshalSSZTo` (no `*Dyn` methods)

**Key Point**: `-without-dynamic-expressions` is especially useful when you want to generate optimized code for the default preset and fall back to reflection-based Dynamic SSZ for other presets.

## Troubleshooting

### "Type not found" Error

Ensure the type is exported and in the correct package:

```bash
# Specify correct package path
dynssz-gen -package github.com/myproject/types -types MyType -output output.go
```

### Cross-Reference Issues

If types reference each other, generate them together to avoid code duplication:

```bash
# Problem: Generating types separately causes massive code duplication
dynssz-gen -package . -types Block -output block_ssz.go # Duplicates Transaction marshaling code inline
dynssz-gen -package . -types Transaction -output tx_ssz.go

# Solution: Generate related types together
dynssz-gen -package . -types Block,Transaction -output types_ssz.go

# Or use per-type files but generate in single command
dynssz-gen -package . -types Block:block_ssz.go,Transaction:tx_ssz.go
```

### "Invalid type" Error

Check that your type follows SSZ rules:
- No interfaces (except with `ssz-type`)
- No channels or functions
- Dynamic lists should have `ssz-max` tags (highly recommended for security)

### Generation Differences

If generated code differs from runtime behavior:

1. Regenerate with verbose mode:
   ```bash
   dynssz-gen -v -package . -types MyType -output debug_ssz.go
   ```

2. Check for custom interfaces that might affect generation

3. Verify all tags are correctly specified

### Build Errors

If generated code doesn't compile:

1. Ensure all imported types are available
2. Check for circular dependencies
3. Verify generated imports are correct

## Best Practices

### 1. Version Control

**Do commit** generated files to your repository:

This ensures:
- Code builds without requiring code generation
- No external tool dependencies for builds
- Consistent builds across environments
- Faster CI/build processes

### 2. Consistent Naming

Use consistent output file naming:
```bash
# Good patterns
types_ssz.go
generated_ssz.go
beacon_ssz_generated.go
```

### 3. Regular Regeneration

Set up automated regeneration:
```bash
# Pre-commit hook
#!/bin/sh
go generate ./...
git add *_ssz.go
```

### 4. Partial Generation

Generate only needed methods:
```go
codeGen.BuildFile("output.go",
    codegen.WithNoUnmarshalSSZ(),      // Only marshal and size
    codegen.WithNoHashTreeRoot(),
    codegen.WithReflectType(reflect.TypeOf(MyType{})),
)
```

### 5. Cross-Reference Handling

When generating multiple files, ensure proper cross-references:
```bash
# Generate all related types together for proper cross-references
dynssz-gen -package . -types Block:block_ssz.go,Transaction:tx_ssz.go,Header:block_ssz.go

# Or generate all at once
dynssz-gen -package . -types Block,Transaction,Header,Validator -output all_ssz.go
```

## Examples

### Ethereum Beacon Chain Types

```bash
# Generate all beacon chain types together for cross-references
dynssz-gen -package . -types BeaconBlock,BeaconState,Validator,Attestation,BeaconBlockBody -output consensus_types_ssz.go
```

### Separate Files with Cross-References

```bash
# Generate to separate files while maintaining cross-references
dynssz-gen -package . -types \
  BeaconBlock:block_ssz.go,BeaconBlockBody:block_ssz.go,\
  BeaconState:state_ssz.go,Validator:state_ssz.go,\
  Attestation:attestation_ssz.go,AttestationData:attestation_ssz.go
```

### Custom Protocol

```go
//go:generate dynssz-gen -package . -types Message,Header,Payload -output protocol_ssz.go

package protocol

type Message struct {
    Header  Header
    Payload Payload        `ssz-max:"65536"`
    Sig     [96]byte
}

type Header struct {
    Version   uint8
    Timestamp uint64
    Sender    [20]byte
}

type Payload struct {
    Type uint16
    Data []byte `dynssz-max:"MAX_PAYLOAD_SIZE"`
}
```

### With Progressive Types

```go
type ProgressiveState struct {
    // Progressive list for efficiency
    Validators []Validator `ssz-type:"progressive-list" dynssz-max:"VALIDATOR_LIMIT"`
    
    // Progressive container with indices
    Slot       uint64      `ssz-index:"0"`
    Extensions *Extensions `ssz-index:"100"`
}
```

## Migration from fastssz

To migrate from fastssz code generation:

1. **Install dynssz-gen**:
   ```bash
   go install github.com/pk910/dynamic-ssz/dynssz-gen@latest
   ```

2. **Update generation commands**:
   ```bash
   # Old: sszgen -path types -output generated.ssz.go
   # New:
   dynssz-gen -package . -types BeaconBlock,BeaconState -output generated_ssz.go
   ```

3. **Add legacy flag if needed**:
   ```bash
   dynssz-gen -legacy -package . -types BeaconBlock -output legacy_ssz.go
   ```

4. **Update imports** in your code:
   ```go
   // Old: github.com/ferranbt/fastssz
   // New: github.com/pk910/dynamic-ssz
   ```

## Related Documentation

- [Getting Started](getting-started.md) - Basic usage
- [API Reference](api-reference.md) - Generator API details
- [SSZ Annotations](ssz-annotations.md) - Supported tags
- [Supported Types](supported-types.md) - Type compatibility