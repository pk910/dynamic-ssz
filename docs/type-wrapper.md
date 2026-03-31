# Type Wrapper

TypeWrapper is a generic type that applies SSZ annotations to values that cannot carry struct tags directly, such as primitive types or type aliases used as standalone fields.

## Problem It Solves

Go struct tags can only be placed on struct fields. If you need to annotate a non-struct value used inside a container (e.g., a `uint64` that needs a `dynssz-max` constraint, or a `[]byte` that needs a fixed size), TypeWrapper lets you attach those annotations through a descriptor struct.

## How It Works

`TypeWrapper[D, T]` has two generic type parameters:
- `D` - A **descriptor struct** with exactly one field. That field carries the SSZ struct tags and must have the same type as `T`.
- `T` - The actual value type being wrapped.

The descriptor struct is never instantiated at runtime. Only its type information (field type and tags) is used during type analysis.

## Basic Usage

### Step 1: Define a Descriptor

The descriptor is a struct with exactly one field. The field carries the SSZ tags:

```go
type ByteSliceDescriptor struct {
    Data []byte `ssz-size:"32"`
}
```

### Step 2: Use TypeWrapper in Your Container

```go
import dynssz "github.com/pk910/dynamic-ssz"

type MyContainer struct {
    Hash dynssz.TypeWrapper[ByteSliceDescriptor, []byte] `ssz-type:"wrapper"`
}
```

The `ssz-type:"wrapper"` tag on the container field tells the SSZ engine to unwrap the TypeWrapper and use the descriptor's annotations.

### Step 3: Access the Value

```go
container := MyContainer{
    Hash: dynssz.TypeWrapper[ByteSliceDescriptor, []byte]{
        Data: []byte{1, 2, 3, 4, 5, 6, 7, 8},
    },
}

// Read the wrapped value
value := container.Hash.Get()

// Set a new value
container.Hash.Set([]byte{9, 10, 11, 12})
```

## Examples

### Fixed-Size Byte Slice

```go
type Hash32Descriptor struct {
    Data []byte `ssz-size:"32"`
}

type Block struct {
    Root dynssz.TypeWrapper[struct {
        Data []byte `ssz-size:"32"`
    }, []byte] `ssz-type:"wrapper"`
}
```

Inline anonymous descriptors work too, as shown above.

### Dynamic Max with Expression

```go
type ValidatorLimitDescriptor struct {
    Data []uint64 `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

type State struct {
    Balances dynssz.TypeWrapper[ValidatorLimitDescriptor, []uint64] `ssz-type:"wrapper"`
}
```

### Uint16 Array with Fixed Size

```go
type MyContainer struct {
    Values dynssz.TypeWrapper[struct {
        Data []uint16 `ssz-size:"2"`
    }, []uint16] `ssz-type:"wrapper"`
}
```

## API

### TypeWrapper[D, T]

```go
type TypeWrapper[D, T any] struct {
    Data T
}
```

**Type Parameters:**
- `D` - Descriptor struct. Must have exactly one field with the same type as `T`. The field carries SSZ struct tags.
- `T` - The wrapped value type.

**Methods:**

```go
// Get returns the wrapped value.
func (w *TypeWrapper[D, T]) Get() T

// Set sets the wrapped value.
func (w *TypeWrapper[D, T]) Set(value T)

// GetDescriptorType returns the reflect.Type of the descriptor struct D.
func (w *TypeWrapper[D, T]) GetDescriptorType() reflect.Type
```

## Code Generation

TypeWrapper is fully supported by the code generator. Include types that use TypeWrapper in your generation command:

```bash
dynssz-gen -package . -types MyContainer -output ssz_generated.go
```

The generated code handles the wrapper transparently.

## When to Use TypeWrapper vs Other Approaches

- **Struct field tags**: Preferred when your value is already a struct field. No wrapper needed.
- **`sszutils.Annotate[T]()`**: Preferred for annotating named non-struct types (e.g., `type Blobs []*Blob`). Works at the type level.
- **TypeWrapper**: Use when you need per-field annotations on a non-struct value inside a container, and the value type itself doesn't have annotations.

## Related Documentation

- [Supported Types](supported-types.md) - Complete type reference
- [SSZ Annotations](ssz-annotations.md) - Tag syntax reference
- [API Reference](api-reference.md) - TypeWrapper API details
