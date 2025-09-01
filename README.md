# Dynamic SSZ

[![Go Reference](https://pkg.go.dev/badge/github.com/pk910/dynamic-ssz.svg)](https://pkg.go.dev/github.com/pk910/dynamic-ssz)
[![Go Report Card](https://goreportcard.com/badge/github.com/pk910/dynamic-ssz)](https://goreportcard.com/report/github.com/pk910/dynamic-ssz)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Dynamic SSZ is a Go library for SSZ encoding/decoding with support for dynamic field sizes and code generation. It provides runtime flexibility while maintaining high performance through optional static code generation.

## Features

- **🔧 Dynamic Field Sizes** - Support for runtime-determined field sizes based on configuration
- **🏗️ Code Generation** - Optional static code generation for maximum performance (2-3x faster than dynamic processing)
- **🔄 Hybrid Approach** - Seamlessly combines with fastssz for optimal efficiency
- **📦 Zero Dependencies** - Core library has minimal external dependencies
- **✅ Spec Compliant** - Fully compliant with SSZ specification and Ethereum consensus tests

## Quick Start

### Installation

```bash
go get github.com/pk910/dynamic-ssz
```

### Basic Usage

```go
import "github.com/pk910/dynamic-ssz"

// Define your types with SSZ tags
type MyStruct struct {
    FixedArray [32]byte
    DynamicList []uint64 `ssz-max:"1000"`
    ConfigBased []byte   `ssz-max:"1024" dynssz-max:"MAX_SIZE"`
}

// Create a DynSsz instance with your configuration
specs := map[string]any{
    "MAX_SIZE": uint64(2048),
}
ds := dynssz.NewDynSsz(specs)

// Marshal
data, err := ds.MarshalSSZ(myObject)

// Unmarshal
err = ds.UnmarshalSSZ(&myObject, data)

// Hash Tree Root
root, err := ds.HashTreeRoot(myObject)
```

### Using Code Generation (Recommended for Production)

For maximum performance, use the code generator:

```go
// codegen/main.go
package main

import (
    "github.com/pk910/dynamic-ssz/codegen"
    "reflect"
)

func main() {
    generator := codegen.NewCodeGenerator(nil)
    
    generator.BuildFile(
        "generated.go",
        codegen.WithType(reflect.TypeOf(MyStruct{})),
    )
    
    generator.Generate()
}
```

Run the generator:
```bash
go run codegen/main.go
```

This generates optimized SSZ methods that are 2-3x faster than reflection-based encoding.

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

## Documentation

- [Getting Started Guide](docs/getting-started.md)
- [API Reference](docs/api-reference.md)
- [Code Generation Guide](docs/codegen.md)
- [Struct Tags & Annotations](docs/struct-tags.md)
- [Performance Guide](docs/performance.md)
- [Examples](examples/)

## Examples

Check out the [examples](examples/) directory for:
- Basic encoding/decoding
- Code generation setup
- Ethereum types integration
- Custom specifications
- Multi-dimensional arrays

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Dynamic SSZ is licensed under the [Apache 2.0 License](LICENSE).