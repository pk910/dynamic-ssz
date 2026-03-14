# SSZ Views

SSZ Views allow a single runtime type to support multiple SSZ schemas. This is particularly useful for Ethereum consensus types that evolve across hard forks, enabling one data structure to be serialized with different SSZ layouts depending on the fork.

## Overview

In Ethereum, consensus types like `BeaconBlockBody` change between forks (Phase0, Altair, Bellatrix, Capella, Deneb, Electra, etc.). Each fork may add new fields, change field order, or modify SSZ annotations. Without views, you would need separate Go types for each fork version.

With views, you can:
- Define a single "data type" containing all possible fields across forks
- Define separate "view types" that specify the SSZ schema for each fork
- Serialize/deserialize the same runtime object with different SSZ layouts

## Concepts

### Data Type vs View Type

- **Data Type**: The actual Go struct where your data lives. Contains all fields that might be needed across different forks.
- **View Type**: A struct that defines the SSZ schema (field order, tags, sizes). The view type's field values are never used; only its type information matters.

### How Views Work

When you provide a view descriptor:
1. The view type defines the SSZ schema (field names, order, and annotations)
2. Data is read from/written to the runtime object's fields
3. Fields are matched by name between the view type and runtime type
4. Nested struct fields can have their own view types for recursive schema mapping

## Basic Usage

### Runtime API

Pass a view descriptor using the `WithViewDescriptor` option:

```go
import dynssz "github.com/pk910/dynamic-ssz"

// Define your data type (contains all fields across forks)
type BeaconBlockBody struct {
    RANDAOReveal      [96]byte
    ETH1Data          *ETH1Data
    Graffiti          [32]byte
    ProposerSlashings []*ProposerSlashing
    AttesterSlashings []*AttesterSlashing
    Attestations      []*Attestation
    Deposits          []*Deposit
    VoluntaryExits    []*SignedVoluntaryExit
    // Altair+ fields
    SyncAggregate     *SyncAggregate
    // Bellatrix+ fields
    ExecutionPayload  *ExecutionPayload
}

// Define a view for the Phase0 fork (no SyncAggregate or ExecutionPayload)
type Phase0BeaconBlockBodyView struct {
    RANDAOReveal      [96]byte                     `ssz-size:"96"`
    ETH1Data          *Phase0ETH1DataView
    Graffiti          [32]byte                     `ssz-size:"32"`
    ProposerSlashings []*Phase0ProposerSlashingView `ssz-max:"16"`
    AttesterSlashings []*Phase0AttesterSlashingView `ssz-max:"2"`
    Attestations      []*Phase0AttestationView      `ssz-max:"128"`
    Deposits          []*Phase0DepositView          `ssz-max:"16"`
    VoluntaryExits    []*Phase0SignedVoluntaryExitView `ssz-max:"16"`
}

// Define a view for the Altair fork (adds SyncAggregate)
type AltairBeaconBlockBodyView struct {
    RANDAOReveal      [96]byte                     `ssz-size:"96"`
    ETH1Data          *AltairETH1DataView
    Graffiti          [32]byte                     `ssz-size:"32"`
    ProposerSlashings []*AltairProposerSlashingView `ssz-max:"16"`
    AttesterSlashings []*AltairAttesterSlashingView `ssz-max:"2"`
    Attestations      []*AltairAttestationView      `ssz-max:"128"`
    Deposits          []*AltairDepositView          `ssz-max:"16"`
    VoluntaryExits    []*AltairSignedVoluntaryExitView `ssz-max:"16"`
    SyncAggregate     *AltairSyncAggregateView
}

func main() {
    ds := dynssz.NewDynSsz(specs)
    body := &BeaconBlockBody{/* ... */}

    // Marshal with Phase0 schema
    phase0Data, err := ds.MarshalSSZ(body, dynssz.WithViewDescriptor(&Phase0BeaconBlockBodyView{}))

    // Marshal with Altair schema
    altairData, err := ds.MarshalSSZ(body, dynssz.WithViewDescriptor(&AltairBeaconBlockBodyView{}))

    // Unmarshal Phase0 data
    var decoded BeaconBlockBody
    err = ds.UnmarshalSSZ(&decoded, phase0Data, dynssz.WithViewDescriptor(&Phase0BeaconBlockBodyView{}))

    // Hash tree root with specific fork schema
    root, err := ds.HashTreeRoot(body, dynssz.WithViewDescriptor(&AltairBeaconBlockBodyView{}))
}
```

### View Descriptor Options

You can pass the view descriptor in several equivalent ways:

```go
// As a pointer to an instance (recommended)
dynssz.WithViewDescriptor(&Phase0BeaconBlockBodyView{})

// As a nil pointer (only type information is used)
dynssz.WithViewDescriptor((*Phase0BeaconBlockBodyView)(nil))

// As a value (will be converted internally)
dynssz.WithViewDescriptor(Phase0BeaconBlockBodyView{})
```

## Code Generation with Views

The code generator supports views through two modes:

### Data+Views Mode

Generate methods for a data type that can work with multiple view types:

```bash
dynssz-gen -package . \
    -types "BeaconBlockBody:body_ssz.go:views=Phase0BeaconBlockBodyView;AltairBeaconBlockBodyView" \
    -output default.go
```

This generates:
- Standard dynamic methods (`MarshalSSZDyn`, `UnmarshalSSZDyn`, etc.)
- View-aware methods (`MarshalSSZDynView`, `UnmarshalSSZDynView`, etc.)

The view-aware methods accept a view parameter and return the appropriate function:

```go
// Generated view-aware method
func (b *BeaconBlockBody) MarshalSSZDynView(view any) func(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
    switch view.(type) {
    case *Phase0BeaconBlockBodyView:
        return b.marshalSSZDynPhase0BeaconBlockBodyView
    case *AltairBeaconBlockBodyView:
        return b.marshalSSZDynAltairBeaconBlockBodyView
    }
    return nil
}
```

### View-Only Mode

Generate methods only for view-based operations (no standard methods):

```bash
dynssz-gen -package . \
    -types "BeaconBlockBody:body_ssz.go:views=Phase0View;AltairView:viewonly" \
    -output default.go
```

This is useful when the data type already has generated SSZ methods and you only want to add view support.

### CLI Syntax

The extended type specification format is:

```
TypeName[:output.go][:views=View1;View2;...][:viewonly]
```

- `TypeName`: The data type name (required)
- `output.go`: Output file for this type (optional, uses `-output` flag if not specified)
- `views=...`: Semicolon-separated list of view types (optional)
- `viewonly`: Generate only view methods, not standard methods (optional)

#### External View Types

View types can be from external packages:

```bash
dynssz-gen -package . \
    -types "BeaconBlockBody:body_ssz.go:views=github.com/myproject/views.Phase0View" \
    -output default.go
```

### Programmatic API

```go
import "github.com/pk910/dynamic-ssz/codegen"

codeGen := codegen.NewCodeGenerator(dynSsz)

codeGen.BuildFile("body_ssz.go",
    codegen.WithReflectType(
        reflect.TypeOf(BeaconBlockBody{}),
        // Add view types
        codegen.WithReflectViewTypes(
            reflect.TypeOf(Phase0BeaconBlockBodyView{}),
            reflect.TypeOf(AltairBeaconBlockBodyView{}),
        ),
    ),
)

// Or for view-only mode
codeGen.BuildFile("body_views_ssz.go",
    codegen.WithReflectType(
        reflect.TypeOf(BeaconBlockBody{}),
        codegen.WithReflectViewTypes(
            reflect.TypeOf(Phase0BeaconBlockBodyView{}),
        ),
        codegen.WithViewOnly(), // Only generate view methods
    ),
)

codeGen.Generate()
```

## View Interfaces

When code generation is used with views, the data type implements these interfaces:

```go
// Marshal with a view
type DynamicViewMarshaler interface {
    MarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) ([]byte, error)
}

// Unmarshal with a view
type DynamicViewUnmarshaler interface {
    UnmarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) error
}

// Size calculation with a view
type DynamicViewSizer interface {
    SizeSSZDynView(view any) func(ds DynamicSpecs) int
}

// Hash tree root with a view
type DynamicViewHashRoot interface {
    HashTreeRootWithDynView(view any) func(ds DynamicSpecs, hh HashWalker) error
}

// Streaming encoder with a view
type DynamicViewEncoder interface {
    MarshalSSZEncoderView(view any) func(ds DynamicSpecs, encoder Encoder) error
}

// Streaming decoder with a view
type DynamicViewDecoder interface {
    UnmarshalSSZDecoderView(view any) func(ds DynamicSpecs, decoder Decoder) error
}
```

These methods return `nil` if the view type is not recognized, causing Dynamic SSZ to fall back to reflection-based processing.

## Field Matching Rules

Views match fields between the schema (view) type and runtime (data) type:

1. **By Name**: Fields are matched by their Go field name (exact match required)
2. **Type Compatibility**: The runtime field type must be compatible with the view field type:
   - Basic types must match exactly
   - Pointer types: both or neither must be pointers
   - Slice/array types: element types must be compatible
   - Struct types: can have different view types for nested schemas
3. **Order**: The SSZ field order is determined by the view type, not the runtime type
4. **Missing Fields**: If a view field doesn't exist in the runtime type, an error is returned

### Nested View Types

For nested structs, the view type's field type defines the nested schema:

```go
// Runtime types
type Container struct {
    Header *Header
    Body   *Body
}

type Header struct {
    Slot      uint64
    Timestamp uint64  // Added in a later fork
}

// Phase0 view - Header has only Slot
type Phase0ContainerView struct {
    Header *Phase0HeaderView
    Body   *Phase0BodyView
}

type Phase0HeaderView struct {
    Slot uint64
}

// Altair view - Header has Slot and Timestamp
type AltairContainerView struct {
    Header *AltairHeaderView
    Body   *AltairBodyView
}

type AltairHeaderView struct {
    Slot      uint64
    Timestamp uint64
}
```

## Practical Example: Ethereum Forks

Here's a realistic example showing how to handle Ethereum's evolving `BeaconState`:

```go
// Data type with all fields across forks
type BeaconState struct {
    // Phase0 fields
    GenesisTime           uint64
    GenesisValidatorsRoot [32]byte
    Slot                  uint64
    Fork                  *Fork
    Validators            []*Validator
    Balances              []uint64

    // Altair+ fields
    PreviousEpochParticipation []byte
    CurrentEpochParticipation  []byte
    InactivityScores           []uint64

    // Bellatrix+ fields
    LatestExecutionPayloadHeader *ExecutionPayloadHeader

    // Capella+ fields
    NextWithdrawalIndex          uint64
    NextWithdrawalValidatorIndex uint64
    HistoricalSummaries          []*HistoricalSummary
}

// Phase0 view
type Phase0BeaconStateView struct {
    GenesisTime           uint64
    GenesisValidatorsRoot [32]byte   `ssz-size:"32"`
    Slot                  uint64
    Fork                  *Phase0ForkView
    Validators            []*Phase0ValidatorView `ssz-max:"1099511627776"`
    Balances              []uint64               `ssz-max:"1099511627776"`
}

// Altair view (adds participation tracking)
type AltairBeaconStateView struct {
    GenesisTime                uint64
    GenesisValidatorsRoot      [32]byte   `ssz-size:"32"`
    Slot                       uint64
    Fork                       *AltairForkView
    Validators                 []*AltairValidatorView `ssz-max:"1099511627776"`
    Balances                   []uint64               `ssz-max:"1099511627776"`
    PreviousEpochParticipation []byte                 `ssz-max:"1099511627776"`
    CurrentEpochParticipation  []byte                 `ssz-max:"1099511627776"`
    InactivityScores           []uint64               `ssz-max:"1099511627776"`
}

// Usage
func ProcessState(state *BeaconState, fork string) ([32]byte, error) {
    ds := dynssz.NewDynSsz(mainnetSpecs)

    var view any
    switch fork {
    case "phase0":
        view = &Phase0BeaconStateView{}
    case "altair":
        view = &AltairBeaconStateView{}
    // ... other forks
    }

    return ds.HashTreeRoot(state, dynssz.WithViewDescriptor(view))
}
```

## Type Validation

You can validate that a view type is compatible with a runtime type:

```go
ds := dynssz.NewDynSsz(specs)

// Validate without view (runtime type only)
err := ds.ValidateType(reflect.TypeOf(&BeaconState{}))

// Validate with view descriptor
err = ds.ValidateType(
    reflect.TypeOf(&BeaconState{}),
    dynssz.WithViewDescriptor(&Phase0BeaconStateView{}),
)
if err != nil {
    // View is incompatible with runtime type
    log.Fatal(err)
}
```

## Best Practices

### 1. Organize View Types by Fork

Keep view types organized by fork in separate files or packages:

```
types/
├── data.go           # Runtime data types
├── views_phase0.go   # Phase0 view types
├── views_altair.go   # Altair view types
└── views_bellatrix.go # Bellatrix view types
```

### 2. Use Consistent Naming

Follow a naming convention for view types:

```go
// Pattern: {Fork}{TypeName}View
type Phase0BeaconBlockView struct { ... }
type AltairBeaconBlockView struct { ... }
type BellatrixBeaconBlockView struct { ... }
```

### 3. Generate Views Together

When using code generation, include all view types in a single generation command to enable cross-reference optimization:

```bash
dynssz-gen -package . \
    -types "BeaconState:state_ssz.go:views=Phase0View;AltairView;BellatrixView" \
    -output default.go
```

### 4. Cache View Descriptors

If you're repeatedly using the same view, cache the descriptor instance:

```go
var (
    phase0View  = &Phase0BeaconStateView{}
    altairView  = &AltairBeaconStateView{}
)

func HashWithFork(state *BeaconState, fork string) ([32]byte, error) {
    var view any
    switch fork {
    case "phase0":
        view = phase0View
    case "altair":
        view = altairView
    }
    return ds.HashTreeRoot(state, dynssz.WithViewDescriptor(view))
}
```

### 5. Test All View Combinations

Ensure your views are tested with the Ethereum consensus spec tests:

```go
func TestStateViews(t *testing.T) {
    testCases := []struct {
        fork string
        view any
    }{
        {"phase0", &Phase0BeaconStateView{}},
        {"altair", &AltairBeaconStateView{}},
        // ...
    }

    for _, tc := range testCases {
        t.Run(tc.fork, func(t *testing.T) {
            // Load spec test data
            // Unmarshal with view
            // Verify hash tree root matches expected
        })
    }
}
```

## Performance Considerations

- **Generated code**: View-aware generated methods provide the best performance by avoiding reflection
- **Reflection fallback**: If a view type is not recognized by generated code, Dynamic SSZ falls back to reflection-based processing
- **Type caching**: View type descriptors are cached after first use, so repeated operations with the same view are efficient

## Related Documentation

- [API Reference](api-reference.md) - Complete method signatures
- [Code Generator](code-generator.md) - Generating optimized code
- [Ethereum Integration](go-eth2-client-integration.md) - Working with Ethereum types
- [Supported Types](supported-types.md) - Type compatibility details
