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
- **🚀 CLI Tool** - Standalone `dynssz-gen` command for easy code generation from any Go package
- **🔄 Hybrid Approach** - Seamlessly combines with fastssz for optimal efficiency
- **📦 Minimal Dependencies** - Core library has minimal external dependencies
- **✅ Spec Compliant** - Fully compliant with SSZ specification and Ethereum consensus tests

## Production Readiness

- **✅ Reflection-based dynamic marshaling/unmarshaling/HTR**: Production ready - battle-tested in various toolings and stable
- **🚧 Code generator**: Feature complete but in beta stage - hasn't been extensively tested in production environments

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

For maximum performance, use code generation. You can use either the CLI tool or the programmatic API:

#### Option 1: CLI Tool (Recommended)

Install the CLI tool:
```bash
go install github.com/pk910/dynamic-ssz/dynssz-gen@latest
```

Generate SSZ methods:
```bash
# Generate for types in current package
dynssz-gen -package . -types "MyStruct,OtherType" -output generated.go

# Generate for types in external package
dynssz-gen -package github.com/example/types -types "Block" -output block_ssz.go
```

#### Option 2: Programmatic API

For integration with build systems:

```go
//go:generate go run codegen.go

// codegen.go
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

Both approaches generate optimized SSZ methods that are faster than reflection-based encoding.

## Performance

Dynamic SSZ is benchmarked against other SSZ libraries (including `fastssz`) in a dedicated benchmark repository: [pk910/ssz-benchmark](https://github.com/pk910/ssz-benchmark).

The benchmarks compare encoding, decoding, and hash tree root performance across different SSZ libraries using real Ethereum consensus data structures.

## Testing

The library includes comprehensive testing infrastructure:

- **Unit Tests**: Fast, isolated tests for core functionality
- **Spec Tests**: Ethereum consensus specification compliance tests
- **Examples**: Working examples that are automatically tested
- **Performance Tests**: Benchmarking and regression testing

## Documentation

- [Getting Started Guide](docs/getting-started.md)
- [API Reference](docs/api-reference.md)
- [Supported Types](docs/supported-types.md)
- [Code Generation Guide](docs/code-generator.md)
- [Struct Tags & Annotations](docs/ssz-annotations.md)
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