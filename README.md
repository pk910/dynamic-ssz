# Dynamic SSZ (dynssz)

Dynamic SSZ (`dynssz`) is a Go library designed to provide flexible and dynamic SSZ encoding/decoding for any Go data structures. It stands out by using runtime reflection to handle serialization and deserialization of types with variable field sizes, enabling it to support dynamic specifications and configurations. While commonly used with Ethereum data structures and presets (mainnet, minimal, custom), it works with any SSZ-compatible types. `dynssz` integrates with `fastssz` to leverage static type information for encoding/decoding when possible, but its primary advantage lies in its ability to adapt to dynamic field sizes that are not well-suited to static code generation methods.

`dynssz` is designed to bridge the gap between the efficiency of static SSZ encoding/decoding and the flexibility required for handling dynamic data structures. It achieves this through a hybrid approach that combines the best of both worlds: leveraging `fastssz` for static types and dynamically processing types with variable sizes.

## Benefits

- **Flexibility**: Supports any SSZ-compatible data structures with custom and dynamic specifications, not limited to Ethereum types.
- **Hybrid Efficiency**: Balances the efficiency of static processing with the flexibility of dynamic handling, optimizing performance where possible.
- **Developer-Friendly**: Simplifies the handling of SSZ data for developers by abstracting the complexity of dynamic data processing.
- **General Purpose**: Works with any Go types that follow SSZ serialization rules, making it suitable for various applications beyond blockchain.

## Installation

To install `dynssz`, use the `go get` command:

```shell
go get github.com/pk910/dynamic-ssz
```

This will download and install the `dynssz` package into your Go workspace.

## Usage

### Supported Types

Dynamic SSZ supports only SSZ-compatible types as defined in the SSZ specification:

**Base Types:**
- `uint8`, `uint16`, `uint32`, `uint64` (unsigned integers)
- `bool` (boolean values)
- Fixed-size byte arrays (e.g., `[32]byte`)

**Composite Types:**
- Arrays and slices of supported types
- Structs containing only supported types
- Pointers to structs (treated as optional fields)

**Not Supported:**
- Signed integers (`int`, `int8`, `int16`, `int32`, `int64`)
- Floating-point numbers (`float32`, `float64`)
- Strings (`string`) - use `[]byte` instead
- Maps, channels, functions, complex numbers
- Interfaces (except when referring to concrete SSZ-compatible types)

### Struct Tag Annotations for Dynamic Encoding/Decoding

`dynssz` utilizes struct tag annotations to indicate how fields should be encoded/decoded, supporting both static and dynamic field sizes:

#### Size Tags

- `ssz-size`:
Defines field sizes. This tag follows the same format supported by `fastssz`, allowing seamless integration. Use `?` to indicate dynamic length dimensions, or specify a number for fixed-size arrays/slices. **Note**: Fixed-size fields (those with numeric values in `ssz-size`) do not use `ssz-max` tags.

- `dynssz-size`:
Specifies sizes based on specification properties, extending the flexibility of `dynssz` to adapt to various Ethereum presets. Unlike the straightforward `ssz-size`, `dynssz-size` supports not only direct references to specification values but also simple mathematical expressions. This feature allows for dynamic calculation of field sizes based on spec values, enhancing the dynamic capabilities of `dynssz`.

    The `dynssz-size` tag can interpret and evaluate expressions involving one or multiple spec values, offering a versatile approach to defining dynamic sizes. For example:
    
    - A direct reference to a single spec value might look like `dynssz-size:"SPEC_VALUE"`.
    - A simple mathematical expression based on a spec value could be `dynssz-size:"(SPEC_VALUE*2)-5"`, enabling the size to be dynamically adjusted according to the spec value.
    - For more complex scenarios involving multiple spec values, the tag can handle expressions like `dynssz-size:"(SPEC_VALUE1*SPEC_VALUE2)+SPEC_VALUE3"`, providing a powerful tool for defining sizes that depend on multiple dynamic specifications.

    When processing a field with a `dynssz-size` tag, `dynssz` evaluates the expression to determine the actual size. If the resolved size deviates from the default established by `ssz-size`, the library switches to dynamic handling for that field. This mechanism ensures that `dynssz` can accurately and efficiently encode or decode data structures, taking into account the intricate sizing requirements dictated by dynamic Ethereum presets.

#### Maximum Size Tags (Required for Dynamic Fields)

- `ssz-max`:
Defines the maximum number of elements for dynamic length fields. This tag is **required** for all dynamic length fields (slices with `?` in `ssz-size` or no `ssz-size` tag) to properly calculate the hash tree root. The maximum size determines the merkle tree depth and is essential for SSZ compliance.

- `dynssz-max`:
Similar to `dynssz-size`, this tag allows specification-based maximum sizes with support for mathematical expressions. This enables dynamic adjustment of maximum bounds based on specification values.

**Important**: Every dynamic length field (those with `?` in `ssz-size` or without `ssz-size`) must have either an `ssz-max` or `dynssz-max` tag. Without these tags, the hash tree root calculation cannot determine the appropriate merkle tree structure. Fixed-size fields (e.g., `ssz-size:"32"`) should not have max tags.

#### Multi-dimensional Slices

`dynssz` supports multi-dimensional slices with different size constraints at each dimension. When using multi-dimensional arrays or slices, you can specify sizes and maximums for each dimension using comma-separated values:

```go
// Two-dimensional byte slice: outer dynamic up to 100, inner fixed at 32 bytes
Field1 [][]byte `ssz-size:"?,32" ssz-max:"100"`

// Two-dimensional uint8 slice: both dimensions dynamic
Field2 [][]uint8 `ssz-size:"?,?" ssz-max:"64,256"`

// Mixed fixed and dynamic dimensions
Field3 [][4]byte `ssz-size:"?" ssz-max:"128" dynssz-max:"MAX_ITEMS"`
```

Key points for multi-dimensional fields:
- Sizes and maximums are specified in order from outermost to innermost dimension
- Use `?` in `ssz-size` to indicate dynamic length dimensions
- Each dynamic dimension requires a corresponding maximum value
- Empty values in comma-separated lists can be used for fixed-size dimensions

Multi-dimensional slices are fully supported for all operations including hash tree root calculations, encoding, and decoding.

Fields with static sizes do not need the `dynssz-size` tag. Here's an example of a structure using various tag combinations:

```go
type Example struct {
    // Fixed-size fields (no ssz-max needed)
    FixedArray     [32]byte                    // Fixed array, no tags needed
    FixedSlice     []byte    `ssz-size:"32"`   // Fixed-size slice of 32 bytes
    Fixed2D        [][]byte  `ssz-size:"4,32"` // Fixed 4x32 byte matrix
    
    // Dynamic-size fields (ssz-max required)
    DynamicSlice   []byte    `ssz-max:"1024"`                           // Dynamic slice, max 1024 bytes
    DynamicSlice2  []byte    `ssz-size:"?" ssz-max:"2048"`              // Explicit dynamic marker
    Dynamic2D      [][]byte  `ssz-size:"?,32" ssz-max:"100"`            // Dynamic outer, fixed inner
    FullyDynamic   [][]byte  `ssz-size:"?,?" ssz-max:"64,256"`          // Both dimensions dynamic
    
    // With dynamic specifications
    SpecDynamic    []uint64  `ssz-max:"1000" dynssz-max:"MAX_ITEMS"`    // Dynamic max from spec
    SpecFixed      []byte    `ssz-size:"256" dynssz-size:"BUFFER_SIZE"` // Fixed size from spec
}

// Real-world Ethereum example
type BeaconState struct {
    GenesisTime                  uint64
    GenesisValidatorsRoot        phase0.Root `ssz-size:"32"`
    Slot                         phase0.Slot
    Fork                         *phase0.Fork
    LatestBlockHeader            *phase0.BeaconBlockHeader
    BlockRoots                   []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    StateRoots                   []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    HistoricalRoots              []phase0.Root `ssz-size:"?,32" ssz-max:"16777216" dynssz-max:"HISTORICAL_ROOTS_LIMIT"`
    Validators                   []Validator   `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    PreviousEpochParticipation   []byte        `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    ...
}
```

### Creating a New DynSsz Instance

```go
import "github.com/pk910/dynamic-ssz"

// Define your dynamic specifications
// For Ethereum use case:
ethSpecs := map[string]any{
    "SYNC_COMMITTEE_SIZE": uint64(32),
    "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
    // ...
}

// For custom application use case:
customSpecs := map[string]any{
    "MAX_ITEMS": uint64(1000),
    "BUFFER_SIZE": uint64(4096),
    // ...
}

ds := dynssz.NewDynSsz(ethSpecs)
// or
ds := dynssz.NewDynSsz(customSpecs)
```

### Marshaling an Object

```go
data, err := ds.MarshalSSZ(myObject)
if err != nil {
    log.Fatalf("Failed to marshal SSZ: %v", err)
}
```

### Unmarshaling an Object

```go
err := ds.UnmarshalSSZ(&myObject, data)
if err != nil {
    log.Fatalf("Failed to unmarshal SSZ: %v", err)
}
```

## Performance

The performance of `dynssz` has been benchmarked against `fastssz` using BeaconBlocks and BeaconStates from small kurtosis testnets, providing a consistent and comparable set of data. These benchmarks compare three scenarios: exclusively using `fastssz`, exclusively using `dynssz`, and a combined approach where `dynssz` defaults to `fastssz` for static types that do not require dynamic processing. The results highlight the balance between flexibility and speed:

**Legend:**
- First number: Unmarshalling time in milliseconds.
- Second number: Marshalling time in milliseconds.
- Third number: Hash tree root time in milliseconds.

### Mainnet Preset

#### BeaconBlock Decode + Encode + Hash (10,000 times)
- **fastssz only:** [8 ms / 3 ms / 88 ms] success
- **dynssz only:** [27 ms / 12 ms / 63 ms] success
- **dynssz + fastssz:** [8 ms / 3 ms / 64 ms] success

#### BeaconState Decode + Encode + Hash (10,000 times)
- **fastssz only:** [5849 ms / 4960 ms / 73087 ms] success
- **dynssz only:** [22544 ms / 12256 ms / 40181 ms] success
- **dynssz + fastssz:** [5728 ms / 4857 ms / 37191 ms] success

### Minimal Preset

#### BeaconBlock Decode + Encode + Hash (10,000 times)
- **fastssz only:** [0 ms / 0 ms / 0 ms] failed (unmarshal error)
- **dynssz only:** [44 ms / 29 ms / 90 ms] success
- **dynssz + fastssz:** [22 ms / 13 ms / 151 ms] success

#### BeaconState Decode + Encode + Hash (10,000 times)
- **fastssz only:** [0 ms / 0 ms / 0 ms] failed (unmarshal error)
- **dynssz only:** [796 ms / 407 ms / 1816 ms] success
- **dynssz + fastssz:** [459 ms / 244 ms / 4712 ms] success

These results showcase the dynamic processing capabilities of `dynssz`, particularly its ability to handle data structures that `fastssz` cannot process due to its static nature. While `dynssz` introduces additional processing time, its flexibility allows it to successfully manage both mainnet and minimal presets. The combined `dynssz` and `fastssz` approach significantly improves performance while maintaining this flexibility, making it a viable solution for applications requiring dynamic SSZ processing.

## Testing

The library includes comprehensive testing infrastructure:

- **Unit Tests**: Fast, isolated tests for core functionality
- **Spec Tests**: Ethereum consensus specification compliance tests
- **Examples**: Working examples that are automatically tested
- **Performance Tests**: Benchmarking and regression testing

### Running Spec Tests

```bash
cd spectests
./run_tests.sh mainnet  # Run mainnet preset tests
./run_tests.sh minimal  # Run minimal preset tests
```

The spec tests automatically download the latest consensus spec test data and validate the library against the official Ethereum test vectors.

## Internal Technical Overview

### Key Components

- **Type and Value Size Calculation**: The library distinguishes between type sizes (static sizes of types or -1 for dynamic types) and value sizes (the absolute size of an instance in SSZ representation), utilizing recursive functions to accurately determine these sizes based on reflection and tag annotations (`ssz-size`, `dynssz-size`).

- **Encoding/Decoding Dispatch**: Central to the library's architecture are the `marshalType` and `unmarshalType` functions. These serve as entry points to the encoding and decoding processes, respectively, dynamically dispatching tasks to specialized functions based on the nature of the data (e.g., `marshalStruct`, `unmarshalArray`).

- **Dynamic Handling with Static Efficiency**: For types that do not necessitate dynamic processing (neither the type nor its nested types have dynamic specifications), `dynssz` optimizes performance by invoking corresponding `fastssz` functions. This ensures minimal overhead for types compatible with static processing.

- **Size Hints and Spec Values**: `dynssz` intelligently handles sizes through `sszSizeHint` structures, derived from field tag annotations. These hints inform the library whether to process data statically or dynamically, allowing for precise and efficient data serialization.

### Architecture Flow

1. **Size Calculation**: Upon receiving a data structure for encoding or decoding, `dynssz` first calculates its size. For encoding, it determines whether the structure can be processed statically or requires dynamic handling. For decoding, it assesses the expected size of the incoming SSZ data.

2. **Dynamic vs. Static Path Selection**: Based on the size calculation and the presence of dynamic specifications, the library selects the appropriate processing path. Static paths leverage `fastssz` for efficiency, while dynamic paths use runtime reflection.

3. **Recursive Encoding/Decoding**: The library recursively processes each field or element of the data structure. It dynamically navigates through nested structures, applying the correct encoding or decoding method based on the data type and size characteristics.

4. **Specialized Function Dispatch**: For complex types (e.g., slices, arrays, structs), `dynssz` dispatches tasks to specialized functions tailored to handle specific encoding or decoding needs, ensuring accurate and efficient processing.


## Contributing

We welcome contributions from the community! Please check out the [CONTRIBUTING.md](CONTRIBUTING.md) file for guidelines on how to contribute to `dynssz`.

## License

`dynssz` is licensed under the [Apache-2.0 License](LICENSE). See the LICENSE file for more details.

## Acknowledgements

Thanks to all the contributors and the Ethereum community for providing the inspiration and foundation for this project.
