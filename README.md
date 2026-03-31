# Dynamic SSZ

[![Go Reference](https://pkg.go.dev/badge/github.com/pk910/dynamic-ssz.svg)](https://pkg.go.dev/github.com/pk910/dynamic-ssz)
[![Go Report Card](https://goreportcard.com/badge/github.com/pk910/dynamic-ssz)](https://goreportcard.com/report/github.com/pk910/dynamic-ssz)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/pk910/dynamic-ssz/badge)](https://securityscorecards.dev/viewer/?uri=github.com/pk910/dynamic-ssz)
[![codecov](https://codecov.io/gh/pk910/dynamic-ssz/branch/master/graph/badge.svg)](https://codecov.io/gh/pk910/dynamic-ssz)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Dynamic SSZ is a Go library for SSZ encoding/decoding with support for dynamic field sizes and code generation. It provides runtime flexibility while maintaining high performance through optional static code generation.

## Features

- **🔧 Dynamic Field Sizes** - Support for runtime-determined field sizes based on configuration
- **⚡ Reflection-Based Processing** - Works instantly with any SSZ-compatible types - no code generation required for prototyping
- **🏗️ Code Generation** - Optional static code generation for maximum performance (2-3x faster than dynamic processing)
- **🚀 CLI Tool** - Standalone `dynssz-gen` command for easy code generation from any Go package
- **📡 Streaming Support** - Memory-efficient streaming to/from `io.Reader`/`io.Writer` for large data
- **🔄 Hybrid Approach** - Seamlessly combines with fastssz for optimal efficiency
- **👁️ SSZ Views** - Support for multiple SSZ schemas on the same runtime type (useful for Ethereum fork handling)
- **📦 Minimal Dependencies** - Core library has minimal external dependencies
- **✅ Spec Compliant** - Fully compliant with SSZ specification and Ethereum consensus tests
- **🧩 Extended Types** - Optional support for signed integers, floats, big.Int, and optional types (non-standard)

## Production Readiness

- **✅ Reflection-based dynamic marshaling/unmarshaling/HTR**: Production ready - battle-tested in various toolings and stable
- **✅ Code generator**: Production ready - feature complete and functionally verified through extensive fuzz testing, though less battle-tested in production environments compared to the reflection code paths

## Quick Start

### Installation

```bash
go get github.com/pk910/dynamic-ssz
```

### Basic Usage

```go
import dynssz "github.com/pk910/dynamic-ssz"

// Define your types with SSZ tags
type MyStruct struct {
    FixedArray  [32]byte
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

The `ssz-max` and `dynssz-max` tags work together: `ssz-max` provides a static fallback, while `dynssz-max` references a spec value resolved at runtime. If the spec value is available it overrides the static default; otherwise the static value is used. This lets the same types work across different network presets (mainnet, minimal, custom testnets).

### Using Code Generation (Recommended for Production)

For maximum performance, use code generation with the `dynssz-gen` CLI tool:

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

Generated code produces optimized SSZ methods that eliminate reflection overhead. **Important**: Always use `ds.MarshalSSZ()`, `ds.UnmarshalSSZ()`, etc. as your entry points - the runtime automatically delegates to generated methods when available. Do not call generated methods (like `MarshalSSZDyn`) directly, as this creates a circular dependency that prevents regeneration. See the [Code Generation Guide](docs/code-generator.md) for details.

## Performance

Dynamic SSZ is benchmarked against other SSZ libraries (including `fastssz`) in a dedicated benchmark repository: [pk910/ssz-benchmark](https://github.com/pk910/ssz-benchmark) ([view graphs](https://pk910.github.io/ssz-benchmark/)).

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://pk910.github.io/ssz-benchmark/benchmark-table.svg">
  <source media="(prefers-color-scheme: light)" srcset="https://pk910.github.io/ssz-benchmark/benchmark-table-light.svg">
  <img alt="SSZ Benchmark Results" src="https://pk910.github.io/ssz-benchmark/benchmark-table-light.svg">
</picture>

The benchmarks compare encoding, decoding, and hash tree root performance across different SSZ libraries using common Ethereum consensus data structures.

View interactive benchmark results and historical trends at: https://pk910.github.io/ssz-benchmark/

## Testing

The library includes comprehensive testing infrastructure:

- **Unit Tests**: Fast, isolated tests for core functionality
- **Spec Tests**: Ethereum consensus specification compliance tests
- **Fuzz Testing**: Continuous fuzzing via CI that generates random SSZ type structures and verifies correctness by comparing reflection and codegen implementations across marshal, unmarshal, hash tree root, and streaming operations
- **Examples**: Working examples that are automatically tested
- **Performance Tests**: Benchmarking and regression testing

## Documentation

- [Getting Started Guide](docs/getting-started.md)
- [Supported Types](docs/supported-types.md)
- [Struct Tags & Annotations](docs/ssz-annotations.md)
- [Code Generation Guide](docs/code-generator.md)
- [SSZ Views](docs/views.md)
- [Merkle Proofs](docs/merkle-proofs.md)
- [Streaming Support](docs/streaming.md)
- [Type Wrapper](docs/type-wrapper.md)
- [Extended Types](docs/extended-types.md) (non-standard)
- [API Reference](docs/api-reference.md)
- [Performance Guide](docs/performance.md)
- [Troubleshooting](docs/troubleshooting.md)
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