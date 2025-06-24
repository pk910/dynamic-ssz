# go-eth2-client Integration Guide

This guide demonstrates how to integrate dynamic-ssz with the go-eth2-client library for Ethereum beacon chain operations.

## Overview

The go-eth2-client library provides comprehensive types for Ethereum beacon chain data structures. Dynamic-ssz seamlessly integrates with these types, providing flexible SSZ encoding/decoding that adapts to different network presets.

## Installation

```bash
go get github.com/pk910/dynamic-ssz
go get github.com/attestantio/go-eth2-client
```

## Basic Integration

### Setting Up DynSsz with Chain Specifications

```go
package main

import (
    "context"
    "log"
    
    dynssz "github.com/pk910/dynamic-ssz"
    "github.com/attestantio/go-eth2-client"
    "github.com/attestantio/go-eth2-client/api"
    "github.com/attestantio/go-eth2-client/http"
)

func setupDynSszFromClient(client eth2client.Service) (*dynssz.DynSsz, error) {
    // Get chain specifications from the client
    specProvider := client.(eth2client.SpecProvider)
    specs, err := specProvider.Spec(context.Background(), &api.SpecOpts{})
    if err != nil {
        return nil, err
    }
    
    return dynssz.NewDynSsz(specs.Data), nil
}
```

### Working with Beacon Blocks

```go
import (
    "github.com/attestantio/go-eth2-client/spec"
    "github.com/attestantio/go-eth2-client/spec/phase0"
    "github.com/attestantio/go-eth2-client/spec/altair"
)

func processPhase0BeaconBlock(ds *dynssz.DynSsz, block *phase0.SignedBeaconBlock) error {
    // Serialize the block
    blockData, err := ds.MarshalSSZ(block)
    if err != nil {
        return fmt.Errorf("failed to marshal block: %w", err)
    }
    
    // Calculate hash tree root
    root, err := ds.HashTreeRoot(block)
    if err != nil {
        return fmt.Errorf("failed to calculate hash tree root: %w", err)
    }
    
    fmt.Printf("Block serialized: %d bytes, root: %x\n", len(blockData), root)
    
    // Store or process the block...
    return nil
}
```

### Handling Different Fork Versions

```go
func marshalVersionedBlock(ds *dynssz.DynSsz, block *spec.VersionedSignedBeaconBlock) ([]byte, error) {
    switch block.Version {
    case spec.DataVersionPhase0:
        return ds.MarshalSSZ(block.Phase0)
    case spec.DataVersionAltair:
        return ds.MarshalSSZ(block.Altair)
    case spec.DataVersionBellatrix:
        return ds.MarshalSSZ(block.Bellatrix)
    case spec.DataVersionCapella:
        return ds.MarshalSSZ(block.Capella)
    case spec.DataVersionDeneb:
        return ds.MarshalSSZ(block.Deneb)
    case spec.DataVersionElectra:
        return ds.MarshalSSZ(block.Electra)
    default:
        return nil, fmt.Errorf("unsupported block version: %v", block.Version)
    }
}
```

## Advanced Integration Patterns

### Beacon State Processing

```go
func processPhase0BeaconState(ds *dynssz.DynSsz, state *phase0.BeaconState) error {
    // Calculate state size
    size, err := ds.SizeSSZ(state)
    if err != nil {
        return fmt.Errorf("failed to calculate state size: %w", err)
    }
    
    // Pre-allocate buffer for efficiency
    buf := make([]byte, 0, size)
    
    // Marshal state
    stateData, err := ds.MarshalSSZTo(state, buf)
    if err != nil {
        return fmt.Errorf("failed to marshal state: %w", err)
    }
    
    // Calculate state root
    stateRoot, err := ds.HashTreeRoot(state)
    if err != nil {
        return fmt.Errorf("failed to calculate state root: %w", err)
    }
    
    fmt.Printf("State size: %d bytes, root: %x\n", len(stateData), stateRoot)
    return nil
}
```

### Attestation Processing

```go
func processAttestations(ds *dynssz.DynSsz, attestations []*phase0.Attestation) error {
    for i, attestation := range attestations {
        // Serialize attestation
        attData, err := ds.MarshalSSZ(attestation)
        if err != nil {
            return fmt.Errorf("failed to marshal attestation %d: %w", i, err)
        }
        
        // Calculate attestation root
        attRoot, err := ds.HashTreeRoot(attestation)
        if err != nil {
            return fmt.Errorf("failed to calculate attestation root %d: %w", i, err)
        }
        
        fmt.Printf("Attestation %d: %d bytes, root: %x\n", i, len(attData), attRoot)
    }
    return nil
}
```

### Sync Committee Integration (Altair+)

```go
func processSyncCommittee(ds *dynssz.DynSsz, syncCommittee *altair.SyncCommittee) error {
    // Serialize sync committee
    data, err := ds.MarshalSSZ(syncCommittee)
    if err != nil {
        return fmt.Errorf("failed to marshal sync committee: %w", err)
    }
    
    // Calculate sync committee root
    root, err := ds.HashTreeRoot(syncCommittee)
    if err != nil {
        return fmt.Errorf("failed to calculate sync committee root: %w", err)
    }
    
    fmt.Printf("Sync committee: %d bytes, root: %x\n", len(data), root)
    return nil
}
```

## Performance Optimization

### Reusing DynSsz Instances

```go
type BeaconProcessor struct {
    dynSsz *dynssz.DynSsz
    buffer []byte
}

func NewBeaconProcessor(client eth2client.Service) (*BeaconProcessor, error) {
    dynSsz, err := setupDynSszFromClient(client)
    if err != nil {
        return nil, err
    }
    
    return &BeaconProcessor{
        dynSsz: dynSsz,
        buffer: make([]byte, 0, 1024*1024), // 1MB buffer
    }, nil
}

func (bp *BeaconProcessor) ProcessBlock(block *spec.VersionedSignedBeaconBlock) error {
    // Reuse buffer for efficiency
    bp.buffer = bp.buffer[:0]
    
    var blockSsz []byte
    var err error

    switch block.Version {
    case spec.DataVersionPhase0:
        blockSsz, err = ds.MarshalSSZ(block.Phase0)
    case spec.DataVersionAltair:
        blockSsz, err = ds.MarshalSSZ(block.Altair)
    case spec.DataVersionBellatrix:
        blockSsz, err = ds.MarshalSSZ(block.Bellatrix)
    case spec.DataVersionCapella:
        blockSsz, err = ds.MarshalSSZ(block.Capella)
    case spec.DataVersionDeneb:
        blockSsz, err = ds.MarshalSSZ(block.Deneb)
    case spec.DataVersionElectra:
        blockSsz, err = ds.MarshalSSZ(block.Electra)
    default:
        return fmt.Errorf("unsupported block version: %v", block.Version)
    }

    if err != nil {
        return fmt.Errorf("failed mashaling block: %w", err)
    }
    
    // Process the serialized block...
    return nil
}
```

## Error Handling

### Robust Error Handling

```go
func safelyProcessBlock(ds *dynssz.DynSsz, block *spec.VersionedSignedBeaconBlock) error {
    // Validate block version
    if block.Version > spec.DataVersionElectra {
        return fmt.Errorf("unsupported block version: %v", block.Version)
    }
    
    // Try to get block data
    var blockData any
    switch block.Version {
    case spec.DataVersionPhase0:
        if block.Phase0 == nil {
            return fmt.Errorf("phase0 block is nil")
        }
        blockData = block.Phase0
    case spec.DataVersionAltair:
        if block.Altair == nil {
            return fmt.Errorf("altair block is nil")
        }
        blockData = block.Altair
    // ... other versions
    default:
        return fmt.Errorf("unsupported block version: %v", block.Version)
    }
    
    // Safely marshal
    data, err := ds.MarshalSSZ(blockData)
    if err != nil {
        return fmt.Errorf("failed to marshal block: %w", err)
    }
    
    // Validate unmarshaling
    switch block.Version {
    case spec.DataVersionPhase0:
        var decoded phase0.SignedBeaconBlock
        if err := ds.UnmarshalSSZ(&decoded, data); err != nil {
            return fmt.Errorf("failed to unmarshal phase0 block: %w", err)
        }
    // ... other versions
    }
    
    return nil
}
```

## Migration from fastssz

### Gradual Migration

If you're migrating from fastssz, you can use dynamic-ssz as a drop-in replacement:

```go
// Before (fastssz)
// import "github.com/ferranbt/fastssz"
// data, err := block.MarshalSSZ()

// After (dynamic-ssz)
import dynssz "github.com/pk910/dynamic-ssz"

ds := dynssz.NewDynSsz(specs)
data, err := ds.MarshalSSZ(block)
```

### Performance Comparison

```go
func benchmarkComparison(block *phase0.SignedBeaconBlock) {
    specs := map[string]any{
        "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
        // ... other specs
    }
    ds := dynssz.NewDynSsz(specs)
    
    // Dynamic-ssz will automatically use fastssz for static types
    // while providing flexibility for dynamic types
    
    start := time.Now()
    for i := 0; i < 10000; i++ {
        _, err := ds.MarshalSSZ(block)
        if err != nil {
            log.Fatal(err)
        }
    }
    duration := time.Since(start)
    
    fmt.Printf("10,000 marshals took: %v\n", duration)
}
```

## Best Practices

1. **Reuse DynSsz instances**: Type caching improves performance
2. **Pre-allocate buffers**: Use `MarshalSSZTo` with reused buffers
3. **Validate versions**: Always check block/state versions before processing
4. **Handle errors gracefully**: Provide meaningful error messages
5. **Use batch processing**: Process multiple items efficiently
6. **Monitor performance**: Compare with fastssz for performance-critical paths

## Complete Example

See [examples/go-eth2-client/](../examples/go-eth2-client/) for a complete working example that demonstrates:
- Client setup and configuration
- Block and state processing
- Performance optimization techniques
- Error handling patterns
- Integration with real beacon chain data