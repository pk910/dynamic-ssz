# Dynamic SSZ (dynssz)

Dynamic SSZ (`dynssz`) is a Go library designed to provide flexible and dynamic SSZ encoding/decoding for Ethereum data structures. It stands out by using runtime reflection to handle serialization and deserialization of types with variable field sizes, enabling it to support a wide range of Ethereum presets beyond the mainnet. `dynssz` integrates with `fastssz` to leverage static type information for encoding/decoding when possible, but its primary advantage lies in its ability to adapt to dynamic field sizes that are not well-suited to static code generation methods.

## Features

- **Dynamic Field Sizes:** Dynamically handles SSZ encoding/decoding with variable field sizes at runtime.
- **Integration with FastSSZ:** Uses `fastssz` for parts of the serialization process when static type information is applicable, offering a balanced approach to handling Ethereum data.
- **Support for Various Ethereum Presets:** Capable of working with non-mainnet Ethereum presets, facilitating a broader range of applications.
- **Minimal Performance Overhead:** Designed to minimize the performance impact of dynamic serialization.

## Installation

To install `dynssz`, use the `go get` command:

```shell
go get github.com/pk910/dynamic-ssz
```

This will download and install the `dynssz` package into your Go workspace.

## Usage

### Creating a New DynSsz Instance

```go
import "github.com/pk910/dynamic-ssz"

// Define your dynamic specifications
specs := map[string]any{
    "SYNC_COMMITTEE_SIZE": uint64(32),
    // ...
}

ds := dynssz.NewDynSsz(specs)
```

### Struct Tag Annotations for Dynamic Encoding/Decoding

`dynssz` utilizes struct tag annotations to indicate how fields should be encoded/decoded, supporting both static and dynamic field sizes:

- `ssz-size`: Defines static default field sizes. This tag follows the same format supported by `fastssz`, allowing seamless integration.
- `dynssz-size`: Specifies dynamic sizes derived from spec properties. Use this tag in conjunction with `ssz-size` for fields that require dynamic sizing. When the resolved size differs from the default, `dynssz` switches to dynamic handling for that field.

Fields with static sizes do not need the `dynssz-size` tag. Here's an example of a structure using both tags:

```go
type BeaconState struct {
    GenesisTime                  uint64
    GenesisValidatorsRoot        phase0.Root `ssz-size:"32"`
    Slot                         phase0.Slot
    Fork                         *phase0.Fork
    LatestBlockHeader            *phase0.BeaconBlockHeader
    BlockRoots                   []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    StateRoots                   []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    ...
}
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

The performance of `dynssz` has been benchmarked in comparison to `fastssz` under various scenarios using Ethereum network data structures like BeaconBlocks and BeaconStates, following both the mainnet and minimal presets. The tests evaluate three approaches: using `fastssz` exclusively, using `dynssz` exclusively, and a combined approach where `dynssz` defaults to `fastssz` for static types not requiring dynamic processing. Here are the results, illustrating the trade-offs between flexibility and speed:

### Mainnet Preset

#### BeaconBlock Decode + Encode (100 times)
- **fastssz only:** 4 ms (success)
- **dynssz only:** 42 ms (success)
- **dynssz + fastssz:** 19 ms (success)

#### BeaconState Decode + Encode (100 times)
- **fastssz only:** 102 ms (success)
- **dynssz only:** 553 ms (success)
- **dynssz + fastssz:** 184 ms (success)

### Minimal Preset

#### BeaconBlock Decode + Encode (100 times)
- **fastssz only:** 0 ms (failed due to unmarshal error)
- **dynssz only:** 14 ms (success)
- **dynssz + fastssz:** 9 ms (success)

#### BeaconState Decode + Encode (100 times)
- **fastssz only:** 0 ms (failed due to unmarshal error)
- **dynssz only:** 43 ms (success)
- **dynssz + fastssz:** 12 ms (success)

These results showcase the dynamic processing capabilities of `dynssz`, particularly its ability to handle data structures that `fastssz` cannot process due to its static nature. While `dynssz` introduces additional processing time, its flexibility allows it to successfully manage both mainnet and minimal presets. The combined `dynssz` and `fastssz` approach significantly improves performance while maintaining this flexibility, making it a viable solution for applications requiring dynamic SSZ processing.

## Contributing

We welcome contributions from the community! Please check out the [CONTRIBUTING.md](CONTRIBUTING.md) file for guidelines on how to contribute to `dynssz`.

## License

`dynssz` is licensed under the [LGPL License](LICENSE). See the LICENSE file for more details.

## Acknowledgements

Thanks to all the contributors and the Ethereum community for providing the inspiration and foundation for this project.
