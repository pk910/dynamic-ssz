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

### M4: Compatible Unions (EIP-7495)
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

1. **Create and serialize a beacon block** with 10,000 validators in a progressive list and 10,000-bit progressive bitlist
2. **Demonstrate union variants** by creating both basic and blob execution payloads
3. **Show combined usage** with an extended beacon block containing all features
4. **Compute hash tree roots** using progressive merkleization algorithms

## Key Concepts Illustrated

### Active Fields in Progressive Containers
```go
type BeaconBlock struct {
    Slot              uint64   `ssz-index:"0"`  // Bit 0 set
    ProposerIndex     uint64   `ssz-index:"1"`  // Bit 1 set
    ParentRoot        [32]byte `ssz-index:"2"`  // Bit 2 set
    StateRoot         [32]byte `ssz-index:"3"`  // Bit 3 set
    ValidatorRegistry []uint64 `ssz-index:"4"`  // Bit 4 set
    AttestationBits   []byte   `ssz-index:"5"`  // Bit 5 set
}
// Active fields bitlist: [0b00111111] (all 6 bits set, delimiter at bit 5)
```

### Union Selector Assignment
```go
// Union variants: ExecutionPayload, ExecutionPayloadWithBlobs
// Since first variant is NOT ProgressiveContainer, selectors start at 1:
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