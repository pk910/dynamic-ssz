# Progressive Merkleization Example

This example demonstrates all four progressive merkleization features implemented in dynamic-ssz:

## Features Demonstrated

### M1: Progressive Lists (EIP-7916)
- **File**: `ValidatorRegistry []uint64` with `ssz-type:"progressive-list"`
- **Purpose**: Efficient merkleization for large lists that grow incrementally
- **Benefits**: Only rehashes minimal tree path when appending elements

### M2: Progressive Bitlists (EIP-7916)
- **File**: `AttestationBits []byte` with `ssz-type:"progressive-bitlist"`
- **Purpose**: Optimized merkleization for growing bitlist structures
- **Use Case**: Validator participation tracking, attestation aggregation

### M3: Progressive Containers (EIP-7495)
- **File**: `BeaconBlock` struct with `ssz-index` tags on fields
- **Purpose**: Forward-compatible containers with active field tracking
- **Benefits**: New fields can be added without breaking existing code

### M4: Compatible Unions (EIP-8016)
- **File**: `PayloadUnion = CompatibleUnion[struct{...}]`
- **Purpose**: Type-safe variant types with automatic selector management
- **Benefits**: Compile-time type safety with runtime variant selection

## Running the Example

```bash
cd examples/progressive-merkleization
go run main.go
```

## Example Output

The example will:

1. **Serialize and hash a progressive list and bitlist** standalone
2. **Serialize a progressive container** with sparse ssz-index fields and a progressive bitlist member
3. **Embed a compatible union in a container** and exercise both payload variants
4. **Compute hash tree roots** using progressive merkleization algorithms

## Key Concepts Illustrated

### Active Fields in Progressive Containers
```go
type BeaconBlock struct {
    Slot          uint64   `ssz-index:"0"` // bit 0 set
    ProposerIndex uint64   `ssz-index:"1"` // bit 1 set
    ParentRoot    [32]byte `ssz-index:"3"` // bit 3 set (bit 2 reserved for a future field)
    StateRoot     [32]byte `ssz-index:"4"` // bit 4 set
    ExtraData     []byte   `ssz-index:"5"` // bit 5 set
    Participation []byte   `ssz-index:"6"` // bit 6 set
}
// Active-fields bits: indexes 0,1,3,4,5,6 — the gap at 2 stays zero, which is
// what lets a later fork add a field there without moving the others.
```

### Union Selector Assignment
```go
// Union variants: ExecutionPayload, ExecutionPayloadWithBlobs
// Compatible-union selectors are the 1-based field positions:
// - ExecutionPayload: selector = 1
// - ExecutionPayloadWithBlobs: selector = 2
```

### Progressive Merkleization Benefits
- **Lists**: O(log n) updates instead of O(n) when appending
- **Bitlists**: Efficient bit manipulation for large participation sets
- **Containers**: Only hash active fields, skip unused ones
- **Unions**: Type-safe variants with minimal overhead

## Real-World Applications

This pattern is particularly useful for:
- **Ethereum beacon chain**: Block structures that evolve over forks
- **Validator registries**: Large lists that grow over time
- **Attestation aggregation**: Bitfields tracking validator participation
- **Fork-compatible data**: Structures that need to support multiple versions