# Dynamic SSZ

[![Go Reference](https://pkg.go.dev/badge/github.com/pk910/dynamic-ssz.svg)](https://pkg.go.dev/github.com/pk910/dynamic-ssz)
[![Go Report Card](https://goreportcard.com/badge/github.com/pk910/dynamic-ssz)](https://goreportcard.com/report/github.com/pk910/dynamic-ssz)
[![codecov](https://codecov.io/gh/pk910/dynamic-ssz/branch/master/graph/badge.svg)](https://codecov.io/gh/pk910/dynamic-ssz)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Dynamic SSZ is a Go library for SSZ encoding/decoding with support for dynamic field sizes and code generation. It provides runtime flexibility while maintaining high performance through optional static code generation.

## Features

- **🔧 Dynamic Field Sizes** - Support for runtime-determined field sizes based on configuration
- **⚡ Reflection-Based Processing** - Works instantly with any SSZ-compatible types - no code generation required for prototyping
- **🏗️ Code Generation** - Optional static code generation for maximum performance (2-3x faster than dynamic processing)
- **🔄 Hybrid Approach** - Seamlessly combines with fastssz for optimal efficiency
- **📦 Minimal Dependencies** - Core library has minimal external dependencies
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
        codegen.WithReflectType(reflect.TypeOf(MyStruct{})),
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

The performance of `dynssz` has been benchmarked against `fastssz` using BeaconBlocks and BeaconStates from small kurtosis testnets, providing a consistent and comparable set of data. These benchmarks compare four scenarios: exclusively using `fastssz`, exclusively using `dynssz`, a hybrid approach where `dynssz` defaults to `fastssz` for static types, and `dynssz` with code generation for maximum performance. The results highlight the balance between flexibility and speed:

**Legend:**
- First number: Unmarshalling time in milliseconds.
- Second number: Marshalling time in milliseconds.
- Third number: Hash tree root time in milliseconds.

### Mainnet Preset

#### BeaconBlock Decode + Encode + Hash (10,000 times)
- **fastssz only:** [6 ms / 2 ms / 87 ms] success
- **dynssz only:** [29 ms / 15 ms / 57 ms] success
- **dynssz + fastssz:** [9 ms / 3 ms / 64 ms] success
- **dynssz + codegen:** [6 ms / 2 ms / 55 ms] success

#### BeaconState Decode + Encode + Hash (10,000 times)
- **fastssz only:** [5963 ms / 4026 ms / 70919 ms] success
- **dynssz only:** [15728 ms / 13841 ms / 49248 ms] success
- **dynssz + fastssz:** [6139 ms / 4094 ms / 36042 ms] success
- **dynssz + codegen:** [6344 ms / 4869 ms / 36084 ms] success

### Minimal Preset

#### BeaconBlock Decode + Encode + Hash (10,000 times)
- **fastssz only:** failed (unmarshal error: invalid ssz encoding. first variable element offset indexes into fixed value data)
- **dynssz only:** [34 ms / 20 ms / 78 ms] success
- **dynssz + fastssz:** [18 ms / 12 ms / 120 ms] success
- **dynssz + codegen:** [8 ms / 8 ms / 69 ms] success

#### BeaconState Decode + Encode + Hash (10,000 times)
- **fastssz only:** failed (unmarshal error: incorrect size)
- **dynssz only:** [762 ms / 434 ms / 1553 ms] success
- **dynssz + fastssz:** [413 ms / 264 ms / 3921 ms] success
- **dynssz + codegen:** [172 ms / 100 ms / 1329 ms] success

These results showcase the dynamic processing capabilities of `dynssz`, particularly its ability to handle data structures that `fastssz` cannot process due to its static nature. The code generation option provides the best of both worlds: the flexibility to handle any preset configuration while delivering performance that matches or exceeds `fastssz`. The hybrid approach with `fastssz` provides excellent performance for compatible types, while code generation delivers optimal performance across all scenarios.

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