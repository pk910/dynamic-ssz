# Type Wrapper

Type Wrapper is a generic pattern in Dynamic SSZ that allows you to apply SSZ annotations to types that cannot have struct tags, such as primitive types, type aliases, or third-party types.

## Problem It Solves

Consider these scenarios where you cannot add struct tags:

```go
// Type alias - can't add tags
type Epoch uint64  // How to add dynssz-max:"MAX_EPOCH"?

// Third-party type - can't modify
type ExternalType external.Type  // How to specify ssz-type?

// Primitive in a slice - no place for tags
type State struct {
    Epochs []uint64  // How to add per-element constraints?
}
```

Type Wrapper solves this by providing a generic wrapper that carries the annotations.

## Basic Usage

### Step 1: Define a Descriptor

Create a descriptor type that specifies the SSZ annotations:

```go
import "github.com/pk910/dynamic-ssz/types"

// Descriptor for Epoch type with dynamic max
type EpochDescriptor struct{}

func (EpochDescriptor) GetSszAnnotation() string {
    return `dynssz-max:"MAX_EPOCH"`
}
```

### Step 2: Use TypeWrapper

```go
type State struct {
    CurrentEpoch types.TypeWrapper[EpochDescriptor, uint64]
    Epochs       []types.TypeWrapper[EpochDescriptor, uint64] `ssz-max:"100"`
}
```

### Step 3: Access the Value

```go
state := State{
    CurrentEpoch: types.TypeWrapper[EpochDescriptor, uint64]{
        Value: 12345,
    },
}

// Access the wrapped value
epoch := state.CurrentEpoch.Value
```

## Advanced Examples

### Wrapping Large Integers

```go
// Descriptor for Gwei amounts using byte array
type GweiDescriptor struct{}

func (GweiDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint256" dynssz-max:"MAX_EFFECTIVE_BALANCE"`
}

type Validator struct {
    Balance types.TypeWrapper[GweiDescriptor, [32]byte]
}
```

### Multiple Annotations

```go
// Descriptor with multiple SSZ tags
type CommitteeIndexDescriptor struct{}

func (CommitteeIndexDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint64" dynssz-max:"MAX_COMMITTEES_PER_SLOT*SLOTS_PER_EPOCH"`
}

type Assignment struct {
    Index types.TypeWrapper[CommitteeIndexDescriptor, uint64]
}
```

### Wrapping External Types

```go
import "github.com/external/lib"

// Descriptor for external type
type ExternalDescriptor struct{}

func (ExternalDescriptor) GetSszAnnotation() string {
    return `ssz-type:"container" ssz-size:"48"`
}

type Data struct {
    External types.TypeWrapper[ExternalDescriptor, lib.ExternalType]
}
```

## Descriptor Interface

The descriptor must implement the annotation method:

```go
type Descriptor interface {
    GetSszAnnotation() string
}
```

The returned string should contain valid SSZ struct tags:
- `ssz-type:"type"` - Specify SSZ type
- `ssz-size:"size"` - Fixed size constraint
- `ssz-max:"max"` - Maximum size for lists
- `dynssz-size:"expression"` - Dynamic size with expressions
- `dynssz-max:"expression"` - Dynamic max with expressions

## Common Patterns

### Reusable Descriptors

Create a library of common descriptors:

```go
package ssz_descriptors

// Common Ethereum types
type SlotDescriptor struct{}
func (SlotDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint64"`
}

type EpochDescriptor struct{}
func (EpochDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint64" dynssz-max:"MAX_EPOCH"`
}

type ValidatorIndexDescriptor struct{}
func (ValidatorIndexDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint64" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

type GweiDescriptor struct{}
func (GweiDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint256"`
}
```

### Conditional Annotations

Use descriptor fields for conditional behavior:

```go
type ConditionalDescriptor struct {
    MaxSize string
}

func (d ConditionalDescriptor) GetSszAnnotation() string {
    if d.MaxSize != "" {
        return fmt.Sprintf(`dynssz-max:"%s"`, d.MaxSize)
    }
    return `ssz-type:"uint64"`
}

// Usage
type Data struct {
    Value types.TypeWrapper[ConditionalDescriptor, uint64]
}
```

### Nested Wrappers

Wrap complex types:

```go
type ComplexDescriptor struct{}

func (ComplexDescriptor) GetSszAnnotation() string {
    return `ssz-type:"container"`
}

type NestedData struct {
    Field1 string
    Field2 []byte `ssz-max:"1024"`
}

type Container struct {
    Nested types.TypeWrapper[ComplexDescriptor, NestedData]
}
```

## Integration with Code Generation

TypeWrapper is fully supported by the code generator:

```go
//go:generate dynssz-gen -types State -output state_ssz.go

type State struct {
    Epoch types.TypeWrapper[EpochDescriptor, uint64]
}
```

The generated code will properly handle the wrapper types.

## Performance Considerations

TypeWrapper has minimal overhead:
- Zero allocation for value types
- Single pointer dereference for reference types
- Descriptor methods are called during type analysis, not runtime

## API Reference

### TypeWrapper[D, T]

```go
type TypeWrapper[D Descriptor, T any] struct {
    Value T
}
```

Generic parameters:
- `D` - Descriptor type implementing GetSszAnnotation()
- `T` - The wrapped type

### Methods

TypeWrapper implements standard SSZ interfaces when the wrapped type supports them:

```go
// If T implements MarshalSSZ
func (tw TypeWrapper[D, T]) MarshalSSZ() ([]byte, error)

// If T implements UnmarshalSSZ
func (tw *TypeWrapper[D, T]) UnmarshalSSZ(data []byte) error

// If T implements SizeSSZ
func (tw TypeWrapper[D, T]) SizeSSZ() int

// If T implements HashTreeRoot
func (tw TypeWrapper[D, T]) HashTreeRoot() ([32]byte, error)
```

## Examples

### Ethereum Validator Registry

```go
// Descriptors for Ethereum types
type ValidatorIndexDescriptor struct{}
func (ValidatorIndexDescriptor) GetSszAnnotation() string {
    return `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

type GweiAmountDescriptor struct{}
func (GweiAmountDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint64"`
}

// Validator with wrapped types
type Validator struct {
    Index              types.TypeWrapper[ValidatorIndexDescriptor, uint64]
    EffectiveBalance   types.TypeWrapper[GweiAmountDescriptor, uint64]
    ActivationEpoch    types.TypeWrapper[EpochDescriptor, uint64]
}
```

### Dynamic Committee Assignments

```go
type CommitteeSizeDescriptor struct{}
func (CommitteeSizeDescriptor) GetSszAnnotation() string {
    return `dynssz-max:"MAX_VALIDATORS_PER_COMMITTEE"`
}

type CommitteeAssignment struct {
    Validators []types.TypeWrapper[ValidatorIndexDescriptor, uint64] `ssz-type:"?" dynssz-max:"MAX_VALIDATORS_PER_COMMITTEE"`
    Index      types.TypeWrapper[CommitteeIndexDescriptor, uint64]
}
```

## Best Practices

1. **Create descriptive descriptor names** that indicate the purpose
2. **Reuse descriptors** across your codebase for consistency
3. **Document descriptors** with their constraints and usage
4. **Use TypeWrapper sparingly** - prefer struct tags when possible
5. **Consider code generation** for better performance

## Troubleshooting

### "Invalid descriptor" Error

Ensure your descriptor implements GetSszAnnotation():
```go
// Correct
type MyDescriptor struct{}
func (MyDescriptor) GetSszAnnotation() string {
    return `ssz-type:"uint64"`
}

// Incorrect - missing method
type BadDescriptor struct{}
```

### "Type mismatch" Error

Ensure the annotation matches the wrapped type:
```go
// Correct - uint64 annotation for uint64 type
type GoodWrapper = types.TypeWrapper[UintDescriptor, uint64]

// Incorrect - uint256 annotation for uint64 type
type BadWrapper = types.TypeWrapper[BigIntDescriptor, uint64]
```

## Related Documentation

- [Supported Types](supported-types.md) - Complete type reference
- [SSZ Annotations](ssz-annotations.md) - Tag syntax reference
- [API Reference](api-reference.md) - TypeWrapper API details