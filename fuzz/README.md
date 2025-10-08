# Fuzzing for Dynamic SSZ

This directory contains fuzzing infrastructure for the dynamic-ssz library, testing marshal/unmarshal operations, size calculations, and hash tree root computations.

## Features

The fuzzer tests:
- **Marshal/Unmarshal**: Roundtrip encoding/decoding operations
- **Size Calculation**: Verifies size computation accuracy  
- **Hash Tree Root**: Tests HTR calculation for various data structures
- **Edge Cases**: Generates boundary values and invalid data patterns

## Usage

### Run individual fuzz tests (one at a time):
```bash
go test -fuzz=FuzzMarshalUnmarshal -fuzztime=60s
go test -fuzz=FuzzSize -fuzztime=30s
go test -fuzz=FuzzHashTreeRoot -fuzztime=30s
```

### Run all fuzz tests sequentially:
```bash
go test -fuzz=FuzzMarshalUnmarshal -fuzztime=30s && \
go test -fuzz=FuzzSize -fuzztime=30s && \
go test -fuzz=FuzzHashTreeRoot -fuzztime=30s
```

### Run standard tests:
```bash
go test ./...
```

### Run benchmarks:
```bash
go test -bench=.
```

## Test Structures

The fuzzer includes several test structures:
- `SimpleStruct`: Basic primitive types
- `ComplexStruct`: Nested structures with slices and arrays
- `VariableStruct`: Variable-length lists and byte arrays
- `ByteArray32`: Fixed-size byte arrays

## Customization

The fuzzer supports:
- Custom seed values for reproducible testing
- Configurable edge case probability
- Extensible type support for new structures