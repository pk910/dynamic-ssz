# Ethereum Integration Guide

This guide demonstrates integration patterns with Ethereum beacon chain types, particularly using go-eth2-client.

## go-eth2-client Integration

Dynamic SSZ works seamlessly with go-eth2-client types, providing flexible encoding/decoding that adapts to different network presets.

### Setup with Chain Specifications

```go
import (
    dynssz "github.com/pk910/dynamic-ssz"
    "github.com/attestantio/go-eth2-client"
)

func setupFromBeaconClient(client eth2client.Service) (*dynssz.DynSsz, error) {
    specProvider := client.(eth2client.SpecProvider)
    specs, err := specProvider.Spec(context.Background(), &api.SpecOpts{})
    if err != nil {
        return nil, err
    }
    return dynssz.NewDynSsz(specs.Data), nil
}
```

### Versioned Block Handling

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

### Performance Optimized Processor

```go
type BeaconProcessor struct {
    dynSsz *dynssz.DynSsz
    buffer []byte
}

func NewBeaconProcessor(client eth2client.Service) (*BeaconProcessor, error) {
    dynSsz, err := setupFromBeaconClient(client)
    if err != nil {
        return nil, err
    }
    
    return &BeaconProcessor{
        dynSsz: dynSsz,
        buffer: make([]byte, 0, 2*1024*1024), // 2MB buffer
    }, nil
}

func (bp *BeaconProcessor) ProcessBlock(block *spec.VersionedSignedBeaconBlock) error {
    bp.buffer = bp.buffer[:0]
    data, err := marshalVersionedBlock(bp.dynSsz, block)
    if err != nil {
        return err
    }
    // Process data...
    return nil
}
```

### Working with Different Preset Networks

```go
// Mainnet configuration
func createMainnetSpecs() map[string]interface{} {
    return map[string]interface{}{
        "MAX_VALIDATORS_PER_COMMITTEE": uint64(2048),
        "SLOTS_PER_EPOCH":              uint64(32),
        "SYNC_COMMITTEE_SIZE":          uint64(512),
        "MAX_ATTESTATIONS":             uint64(128),
    }
}

// Minimal testnet configuration
func createMinimalSpecs() map[string]interface{} {
    return map[string]interface{}{
        "MAX_VALIDATORS_PER_COMMITTEE": uint64(4),
        "SLOTS_PER_EPOCH":              uint64(8),
        "SYNC_COMMITTEE_SIZE":          uint64(32),
        "MAX_ATTESTATIONS":             uint64(8),
    }
}

// Use appropriate specs for your network
ds := dynssz.NewDynSsz(createMainnetSpecs())
```

## Common Patterns

### Batch Processing
```go
func processBatch(ds *dynssz.DynSsz, blocks []*phase0.SignedBeaconBlock) error {
    for _, block := range blocks {
        data, err := ds.MarshalSSZ(block)
        if err != nil {
            return fmt.Errorf("block %d failed: %w", block.Message.Slot, err)
        }
        // Process data...
    }
    return nil
}
```

### Round-trip Validation
```go
func validateRoundTrip(ds *dynssz.DynSsz, original *phase0.SignedBeaconBlock) error {
    data, err := ds.MarshalSSZ(original)
    if err != nil {
        return err
    }
    
    var decoded phase0.SignedBeaconBlock
    err = ds.UnmarshalSSZ(&decoded, data)
    if err != nil {
        return err
    }
    
    // Compare hash tree roots
    origRoot, _ := ds.HashTreeRoot(original)
    decodedRoot, _ := ds.HashTreeRoot(&decoded)
    
    if origRoot != decodedRoot {
        return fmt.Errorf("round-trip validation failed")
    }
    return nil
}
```

## Examples

See the [versioned-blocks example](../examples/versioned-blocks/) for complete working code demonstrating Ethereum fork handling patterns.