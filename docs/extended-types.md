# Extended Types

Dynamic SSZ supports an extended set of types beyond the standard SSZ specification. These types are **not part of the SSZ spec** and are **not compatible with other SSZ libraries** such as `fastssz`, `ssz` (Rust), or `py_ssz`.

Extended types are useful for applications that need SSZ-like serialization for non-Ethereum data structures, internal tooling, or prototyping with richer type systems.

> **Warning**: Data serialized using extended types cannot be deserialized by standard SSZ implementations. Only use extended types when interoperability with other SSZ libraries is not required.

## Enabling Extended Types

Extended types are disabled by default. You must explicitly enable them:

### Reflection-Based (Runtime)

```go
import dynssz "github.com/pk910/dynamic-ssz"

ds := dynssz.NewDynSsz(specs, dynssz.WithExtendedTypes())

// All operations now support extended types
data, err := ds.MarshalSSZ(myStruct)
err = ds.UnmarshalSSZ(&myStruct, data)
root, err := ds.HashTreeRoot(myStruct)
size, err := ds.SizeSSZ(myStruct)
```

### Code Generation (CLI)

```bash
dynssz-gen -package . -types MyStruct -output generated.go -with-extended-types
```

### Code Generation (Programmatic)

```go
generator := codegen.NewCodeGenerator(typeCache)
generator.BuildFile("generated.go",
    codegen.WithReflectType(reflect.TypeOf(MyStruct{})),
    codegen.WithExtendedTypes(),
)
```

When extended types are **not** enabled and a struct contains extended type fields, Dynamic SSZ returns an error:

```
signed integers are not supported in SSZ (use unsigned integers instead)
floating-point numbers are not supported in SSZ (use extended types option to enable it)
```

## Supported Extended Types

### Signed Integers

| Go Type | SSZ Type | Size | Encoding |
|---------|----------|------|----------|
| `int8`  | `int8`   | 1 byte | Little-endian, two's complement |
| `int16` | `int16`  | 2 bytes | Little-endian, two's complement |
| `int32` | `int32`  | 4 bytes | Little-endian, two's complement |
| `int64` | `int64`  | 8 bytes | Little-endian, two's complement |

Signed integers are serialized as fixed-size values using two's complement encoding in little-endian byte order, matching their in-memory representation in Go on little-endian architectures.

```go
type SignedData struct {
    Temperature int16
    Offset      int32
    Timestamp   int64
}

ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
data, _ := ds.MarshalSSZ(SignedData{
    Temperature: -15,
    Offset:      -1000,
    Timestamp:   1234567890,
})
```

> **Note**: `int` (architecture-dependent size) is **not** supported. Use explicitly-sized types (`int8`, `int16`, `int32`, `int64`).

### Floating-Point Numbers

| Go Type   | SSZ Type  | Size | Encoding |
|-----------|-----------|------|----------|
| `float32` | `float32` | 4 bytes | IEEE 754 binary32, little-endian |
| `float64` | `float64` | 8 bytes | IEEE 754 binary64, little-endian |

Floating-point numbers are serialized using their IEEE 754 binary representation in little-endian byte order.

```go
type Measurement struct {
    Latitude  float64
    Longitude float64
    Altitude  float32
}

ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
data, _ := ds.MarshalSSZ(Measurement{
    Latitude:  52.5200,
    Longitude: 13.4050,
    Altitude:  34.5,
})
```

### Big Integers (`math/big.Int`)

| Go Type    | SSZ Type | Size | Encoding |
|------------|----------|------|----------|
| `big.Int`  | `bigint` | Variable | Big-endian byte representation |

`big.Int` values are treated as **dynamic-size** types. They are serialized as variable-length byte arrays using `big.Int.Bytes()` (big-endian). The hash tree root uses `PutBytes` for proper merkleization.

```go
import "math/big"

type Account struct {
    Balance big.Int
    Nonce   uint64
}

ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())
acc := Account{
    Nonce: 42,
}
acc.Balance.SetString("1000000000000000000", 10) // 1 ETH in wei

data, _ := ds.MarshalSSZ(acc)
```

Because `big.Int` is dynamic-size, any container that includes a `big.Int` field becomes a dynamic container with offset-based encoding.

### Optional Types (Pointers)

| Go Type | SSZ Type   | Size | Encoding |
|---------|------------|------|----------|
| `*T`    | `optional` | Variable | 1-byte presence flag + value |

Optional types use Go pointer types to represent values that may or may not be present. They are always dynamic-size.

> **Important**: Pointer types are **not** automatically treated as optional. You **must** annotate pointer fields with `ssz-type:"optional"` to enable optional encoding. Without this annotation, pointer fields follow standard SSZ behavior where a `nil` pointer is expanded to a zero-valued instance of the pointed-to type.

**Encoding format**:
- If `nil`: `0x00` (1 byte)
- If present: `0x01` followed by the serialized value

```go
type Config struct {
    Name     uint64
    MaxSize  *uint32 `ssz-type:"optional"`  // Optional uint32
    Enabled  *bool   `ssz-type:"optional"`  // Optional bool
}

ds := dynssz.NewDynSsz(nil, dynssz.WithExtendedTypes())

// With values present
maxSize := uint32(1024)
enabled := true
cfg := Config{
    Name:    1,
    MaxSize: &maxSize,
    Enabled: &enabled,
}
data, _ := ds.MarshalSSZ(cfg)

// With nil values
cfg2 := Config{
    Name:    2,
    MaxSize: nil,      // Serialized as 0x00
    Enabled: nil,      // Serialized as 0x00
}
data2, _ := ds.MarshalSSZ(cfg2)
```

Optional types can wrap any SSZ-serializable type, including other extended types:

```go
type ExtendedOptional struct {
    Value *int16 `ssz-type:"optional"`  // Optional signed integer
}
```

Without the `ssz-type:"optional"` annotation, pointer fields behave as standard SSZ fields:

```go
type StandardPointer struct {
    // Standard behavior: nil pointer is treated as zero-valued uint32
    Data *uint32
}
```

## Type Detection and Annotations

Signed integers (`int8`-`int64`), floating-point numbers (`float32`, `float64`), and `big.Int` are **auto-detected** from their Go types when extended types are enabled. You can optionally use `ssz-type` tags for explicit control.

Optional types are **never** auto-detected. Pointer fields **must** be annotated with `ssz-type:"optional"` to use optional encoding. Without the annotation, pointer fields follow standard SSZ behavior (nil pointers expand to zero-valued instances).

```go
type MyStruct struct {
    // Auto-detected extended types
    A int8
    B int16
    C float32
    D float64
    E big.Int

    // Explicit annotations (equivalent to auto-detection for these)
    F int32   `ssz-type:"int32"`
    G float64 `ssz-type:"float64"`

    // Optional REQUIRES the annotation - not auto-detected
    H *uint32 `ssz-type:"optional"`
}
```

## Using Extended Types in Collections

Extended types can be used inside vectors, lists, and containers:

```go
type DataSet struct {
    // Fixed array of signed integers
    Offsets [4]int32

    // Dynamic list of floats
    Measurements []float64 `ssz-max:"1000"`

    // Container with mixed types
    Metadata struct {
        ID    uint64
        Score float32
        Delta int16
    }
}
```

## Hash Tree Root

Extended types are fully supported in hash tree root computation:

- **Fixed-size types** (`int8`-`int64`, `float32`, `float64`): Hashed as their serialized byte representation, same as standard unsigned integers.
- **`big.Int`**: Hashed using `PutBytes` for proper merkleization of variable-length data.
- **Optional types**: The hash tree root includes the presence flag and the child value's hash.

## Code Generation

The code generator fully supports extended types. Generated code handles:
- Marshaling and unmarshaling for all extended types
- Size computation
- Hash tree root calculation
- Streaming encoder/decoder support

Enable with the `-with-extended-types` CLI flag or `codegen.WithExtendedTypes()` programmatic option.

## Compatibility Notes

- Extended types are a **Dynamic SSZ extension** and are not part of the SSZ specification.
- Data encoded with extended types **cannot** be decoded by standard SSZ libraries (`fastssz`, Rust `ssz`, Python `py_ssz`, etc.).
- The `WithExtendedTypes()` option must be consistently used for both encoding and decoding.
- When extended types are disabled (the default), encountering signed integers, floats, or `big.Int` types produces a descriptive error. Optional types annotated with `ssz-type:"optional"` also require extended types to be enabled.
- `int` and `uint` (architecture-dependent sizes) are never supported, regardless of the extended types setting.
- `complex64` and `complex128` are not supported.

## Related Documentation

- [Supported Types](supported-types.md) - Standard SSZ type reference
- [SSZ Annotations](ssz-annotations.md) - Struct tag reference
- [Code Generator](code-generator.md) - Code generation guide
- [API Reference](api-reference.md) - Complete API documentation
