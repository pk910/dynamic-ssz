# Streaming Support

Dynamic SSZ includes streaming support for memory-efficient encoding and decoding of SSZ data. Streaming allows you to process large data structures without loading the entire serialized form into memory, making it ideal for handling large beacon states, network transmission, and file I/O operations.

## Overview

Streaming in Dynamic SSZ provides:

- **Memory Efficiency**: Process large SSZ data without allocating the entire buffer in memory
- **I/O Integration**: Direct integration with `io.Reader` and `io.Writer` interfaces
- **Code Generation Support**: Generate optimized streaming methods via `dynssz-gen`
- **Seamless Fallback**: Automatically falls back to buffer-based processing when needed

## API Methods

### MarshalSSZWriter

Serializes an object directly to an `io.Writer`:

```go
func (d *DynSsz) MarshalSSZWriter(source any, w io.Writer) error
```

**Parameters**:
- `source` - Object to serialize
- `w` - Destination writer (file, network connection, etc.)

**Example**:
```go
// Write directly to file
file, err := os.Create("beacon_state.ssz")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

err = ds.MarshalSSZWriter(state, file)
if err != nil {
    log.Fatal("Failed to write state:", err)
}
```

```go
// Stream over network
conn, err := net.Dial("tcp", "localhost:8080")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

err = ds.MarshalSSZWriter(block, conn)
```

### UnmarshalSSZReader

Deserializes an object directly from an `io.Reader`:

```go
func (d *DynSsz) UnmarshalSSZReader(target any, r io.Reader, size int) error
```

**Parameters**:
- `target` - Pointer to object to deserialize into
- `r` - Source reader
- `size` - Expected total size of the SSZ data in bytes. A negative size selects
  **unknown-size mode**: the payload is consumed to EOF without being buffered,
  so the memory savings of streaming still apply. See
  [Unknown-size decoding](#unknown-size-decoding).

**Example**:
```go
// Read from file
file, err := os.Open("beacon_state.ssz")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

info, _ := file.Stat()
var state BeaconState
err = ds.UnmarshalSSZReader(&state, file, int(info.Size()))
if err != nil {
    log.Fatal("Failed to read state:", err)
}
```

```go
// Read from network with known size
var block BeaconBlock
err = ds.UnmarshalSSZReader(&block, conn, expectedSize)
```

## Streaming Interfaces

Types can implement streaming interfaces for optimized encoding and decoding. These interfaces are particularly useful for generated code.

### DynamicEncoder Interface

For streaming-capable marshaling:

```go
type DynamicEncoder interface {
    MarshalSSZEncoder(ds DynamicSpecs, encoder Encoder) error
}
```

### DynamicDecoder Interface

For streaming-capable unmarshaling:

```go
type DynamicDecoder interface {
    UnmarshalSSZDecoder(ds DynamicSpecs, decoder Decoder) error
}
```

### Encoder Interface

The `Encoder` interface abstracts over buffer-based and stream-based encoding:

```go
type Encoder interface {
    Seekable() bool                    // Returns false for stream encoder
    GetPosition() int                 // Current write position
    GetBuffer() []byte                // Get output buffer (temp buffer for streams)
    SetBuffer(buffer []byte)          // Set/write buffer
    EncodeBool(v bool)
    EncodeUint8(v uint8)
    EncodeUint16(v uint16)
    EncodeUint32(v uint32)
    EncodeUint64(v uint64)
    EncodeBytes(v []byte)
    EncodeOffset(v uint32)
    EncodeOffsetAt(pos int, v uint32) // Not supported for streams
    EncodeZeroPadding(n int)
}
```

### Decoder Interface

The `Decoder` interface abstracts over buffer-based and stream-based decoding:

```go
type Decoder interface {
    Seekable() bool                        // Returns false for stream decoder
    GetPosition() int                     // Current read position
    GetLength() int                       // Remaining length (allowance in an open region)
    LengthKnown() bool                    // False inside an open region
    More() (bool, error)                  // Region holds at least one more byte
    DecodeRemaining(max int) ([]byte, error) // Consume to region end / EOF
    PushOpenLimit()                       // Push a region that ends at EOF
    PushLimit(limit int)
    PopLimit() int
    DecodeBool() (bool, error)
    DecodeUint8() (uint8, error)
    DecodeUint16() (uint16, error)
    DecodeUint32() (uint32, error)
    DecodeUint64() (uint64, error)
    DecodeBytes(buf []byte) ([]byte, error)
    DecodeBytesBuf(len int) ([]byte, error)
    DecodeOffset() (uint32, error)
    DecodeOffsetAt(pos int) uint32        // Not supported for streams
    SkipBytes(n int)                      // Not supported for streams
}
```

## Unknown-size decoding

Passing `size < 0` decodes a payload whose length is not known in advance —
a network stream, a pipe, an object whose framing does not carry a length.

### Why this is possible at all

SSZ is not self-delimiting, but the missing length is only needed in one place
per nesting level: the **trailing** region. Everything else is bounded by the
next offset or by a fixed size. So "unknown length" propagates down a single
chain — the last dynamic child at each level:

| Type | Where the length is needed |
|---|---|
| Container with dynamic fields | end of the **last** dynamic field |
| Vector of dynamic elements | end of the **last** element (count comes from the type) |
| List of dynamic elements | end of the **last** element (count comes from the first offset) |
| List of fixed-size elements | the whole region — consumed until the input runs out |
| Bitlist, byte list, `big.Int` | the whole region — the payload has no internal framing |
| Union, optional, type wrapper | the payload after the selector or flag |

At most one such region is open at any moment, and it is always the current
suffix of the stream.

### Buffer size matters

If the payload fits in the decoder's read buffer, the initial fill observes EOF,
the length becomes exact, and the decode runs entirely on the ordinary
known-length path — including all of its fail-fast validation. Sizing the buffer
to your typical payload is therefore worth doing:

```go
ds := dynssz.NewDynSsz(specs, dynssz.WithStreamReaderBufferSize(64*1024))
```

### Wire-size bound

Unknown-size decoding is **always bounded** and the bound cannot be disabled:

```go
ds := dynssz.NewDynSsz(specs, dynssz.WithMaxStreamSize(16*1024*1024))
```

The default is 512 MiB. This bounds bytes consumed from the wire and doubles as
the remaining-length estimate reported to code that predates unknown-size
decoding (see [Regenerating](#regenerating-generated-code)). `ssz-max` limits
are enforced while reading. Set the smallest value your application protocol
and schema permit; the default is deliberately general and is usually too
generous for peer-to-peer messages.

`WithMaxStreamSize` is not:

- a read deadline or cancellation mechanism;
- a bound on the lifetime of a connection or goroutine;
- a bound on decoded-object heap. A compact offset table can describe many Go
  values, and value elements with large inline arrays can occupy much more
  memory than their table does.

Schema `ssz-max` values and application-level concurrency/memory budgets still
matter.

### EOF framing, deadlines, and cancellation

In unknown-size mode, **EOF is the message boundary**. Use it only when EOF
unambiguously ends one SSZ payload, such as a file, a closed pipe, an HTTP
response body, or a connection dedicated to a single payload. Do not use it
directly on a long-lived connection carrying multiple messages; use the
protocol's trusted-and-capped length framing and pass that length instead.

A peer can send fewer than `WithMaxStreamSize` bytes and then withhold EOF
forever. Network callers must therefore set a deadline or arrange cancellation.
`io.Reader` has no context-aware read method, so cancellation normally closes
the response body, pipe, or connection:

```go
conn, err := net.Dial("tcp", address)
if err != nil {
    return err
}
defer conn.Close()

if err := conn.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
    return err
}

ds := dynssz.NewDynSsz(
    specs,
    dynssz.WithMaxStreamSize(4*1024*1024), // protocol-specific maximum
)
var message Message
if err := ds.UnmarshalSSZReader(&message, conn, -1); err != nil {
    return err
}
```

For HTTP, configure the request context and transport/client timeouts. Canceling
the request closes the response body and unblocks the decoder.

### What it costs and what it saves

Peak heap while decoding a 24.4 MB payload (a container whose trailing region is
a large list of fixed-size records):

| Path | Peak heap |
|---|---|
| Buffer, data already in hand | 24.4 MB |
| `io.ReadAll` + buffer | 76.6 MB |
| Stream, known size | 24.5 MB |
| **Stream, unknown size** | **56.1 MB** |

Unknown-size decoding roughly halves the peak of reading the stream into memory
first, but does not match a known-size decode. The reason is structural: a list
whose element count is not known has to grow, and a growing contiguous slice
holds both the old and the new backing array at the moment it grows — about
twice the result size, whatever the implementation. Pass the size when you know
it.

The saving is largest when the trailing region is *not* one huge list of
fixed-size elements. A list of **dynamic** elements takes its count from the
first offset. In an unknown-size region, the decoder reserves at most 64 KiB of
destination elements from that count, then grows the slice geometrically as it
reaches further element bodies. Known-size streams and buffer decoding retain
their exact allocation. Unusually wide Go value elements therefore no longer
materialize the schema's entire maximum before the first body arrives. Keep
list limits realistic: they remain the final result-size bound.

### Less precise errors

Checks that a known length catches up front — a truncated fixed section, an
offset past the end, a misaligned list — instead surface as `ErrUnexpectedEOF`
when the read runs out. **The accept/reject decision is unchanged**; only the
diagnostic differs. Everything that does not depend on the region length is
still checked exactly: first-offset agreement, offset monotonicity, per-element
consumption, bitlist termination, `big.Int` canonicality, and that the input was
consumed in full.

Note that "extra bytes are rejected" is not a property of either mode: if the
trailing region is a variable-length list, an extra element-sized chunk is simply
another element, and a known-size decode accepts it too.

### Regenerating generated code

Types whose SSZ methods were produced by **dynamic-ssz v1.3.2 or earlier must be
regenerated** to be decoded with `size < 0`. Older generated decoders size the
trailing region from the remaining-length estimate, so they fail cleanly with
`ErrUnexpectedEOF` rather than decoding. Passing an explicit size keeps working
with them unchanged.

## Stream Encoder and Decoder

Dynamic SSZ provides `StreamEncoder` and `StreamDecoder` implementations:

### StreamEncoder

Creates a new stream encoder:

```go
import "github.com/pk910/dynamic-ssz/sszutils"

encoder := sszutils.NewStreamEncoder(writer, 0) // 0 = default 2KB buffer
```

The `StreamEncoder`:
- Writes SSZ data directly to the underlying `io.Writer`
- Maintains internal position tracking
- Does not support seeking (`Seekable()` returns `false`)
- Reports write errors via `GetWriteError()`

### StreamDecoder

Creates a new stream decoder:

```go
decoder := sszutils.NewStreamDecoder(reader, totalSize, 0) // 0 = default 2KB buffer

// ... or without a known length, reading to EOF:
decoder := sszutils.NewUnknownStreamDecoder(reader, 0, 0) // 0, 0 = default buffer and max size
```

The `StreamDecoder`:
- Reads SSZ data directly from the underlying `io.Reader`
- Uses internal buffering (2KB default) for efficient small reads
- Supports limit-based reading for nested structures
- Does not support seeking (`Seekable()` returns `false`)

## Code Generation with Streaming

Generate streaming-capable SSZ methods using the `-with-streaming` flag:

### CLI Usage

```bash
# Generate with streaming support
dynssz-gen -package . -types BeaconBlock,BeaconState -output ssz_generated.go -with-streaming
```

### Programmatic API

```go
ds := dynssz.NewDynSsz(nil)
codeGen := codegen.NewCodeGenerator(ds)

codeGen.BuildFile("generated_ssz.go",
    codegen.WithReflectType(reflect.TypeOf(BeaconBlock{})),
    codegen.WithReflectType(reflect.TypeOf(BeaconState{})),
    codegen.WithCreateEncoderFn(),  // Generate MarshalSSZEncoder
    codegen.WithCreateDecoderFn(),  // Generate UnmarshalSSZDecoder
)

codeGen.Generate()
```

### Generated Methods

When streaming is enabled, the following additional methods are generated:

#### MarshalSSZEncoder

```go
func (b *BeaconBlock) MarshalSSZEncoder(ds sszutils.DynamicSpecs, encoder sszutils.Encoder) error {
    // Streaming-compatible marshaling
}
```

#### UnmarshalSSZDecoder

```go
func (b *BeaconBlock) UnmarshalSSZDecoder(ds sszutils.DynamicSpecs, decoder sszutils.Decoder) error {
    // Streaming-compatible unmarshaling
}
```

## How Streaming Works

### Encoding

For streaming encoding:

1. **Static Fields**: Written directly to the stream in order
2. **Dynamic Fields**: Require size pre-calculation since stream encoders cannot seek
   - The encoder calculates sizes upfront using `SizeSSZ`
   - Offsets are written with pre-computed values
   - Dynamic content follows in order

### Decoding

For streaming decoding:

1. **Static Fields**: Read directly from the stream
2. **Dynamic Fields**: Use offset tracking with limits
   - Offsets are read and stored for later use
   - Limits are pushed to constrain field boundaries
   - Data is read in order within limit boundaries

### Seek vs Non-Seek Mode

The streaming system adapts based on encoder/decoder capabilities:

**Buffer-based (can seek)**:
- Write placeholder offsets, fill in later
- Random access for offset resolution

**Stream-based (cannot seek)**:
- Pre-calculate all sizes before encoding
- Read offsets in order during decoding
- Use offset pool for efficient memory reuse

## Performance Considerations

### CPU vs Memory Trade-off

Streaming trades CPU time for memory efficiency. Because stream-based operations must process data linearly without seeking back:

| Operation | Streaming Overhead | Reason |
|-----------|-------------------|--------|
| **Unmarshal** | ~2x CPU time | Offsets must be stored separately since the decoder cannot seek back to read them later |
| **Marshal** | ~1.3x CPU time | Sizes must be pre-calculated before encoding since the encoder cannot update offsets retroactively |

**Why the overhead?**

- **Buffer-based encoding** can write placeholder offsets, then seek back to fill them in after encoding dynamic fields
- **Stream-based encoding** cannot seek, so it must calculate all field sizes upfront before writing any offsets
- **Buffer-based decoding** can jump to any offset position to read dynamic field data
- **Stream-based decoding** must read offsets into a temporary buffer first, then process fields in order

### When to Use Streaming

Streaming is beneficial when:

- **Large Data**: Processing beacon states or other large structures where memory savings outweigh CPU cost
- **Memory Constraints**: Running in memory-limited environments
- **Network I/O**: Directly transmitting/receiving SSZ data without intermediate buffering
- **File I/O**: Reading/writing large SSZ files without loading entirely into memory

### When to Use Buffers

Buffer-based processing is better when:

- **Small Data**: Overhead of streaming isn't justified for small structures
- **Random Access**: Need to modify or re-read parts of the data
- **Multiple Operations**: Need to marshal/unmarshal multiple times
- **CPU-Sensitive Workloads**: When CPU time is more critical than memory usage

### Memory Usage

The streaming encoder/decoder use minimal internal buffering:

- **StreamEncoder**: 32-byte internal buffer for numeric conversions
- **StreamDecoder**: 2KB internal buffer, grows dynamically for large reads

## Example: Full Streaming Workflow

```go
package main

import (
    "os"

    dynssz "github.com/pk910/dynamic-ssz"
)

func main() {
    specs := map[string]any{
        "MAX_VALIDATORS_PER_COMMITTEE": uint64(2048),
    }
    ds := dynssz.NewDynSsz(specs)

    // Create and populate a beacon state
    state := &BeaconState{
        Slot: 12345,
        // ... populate other fields
    }

    // Stream encode to file
    file, _ := os.Create("state.ssz")
    err := ds.MarshalSSZWriter(state, file)
    if err != nil {
        panic(err)
    }
    file.Close()

    // Stream decode from file
    file, _ = os.Open("state.ssz")
    info, _ := file.Stat()

    var decoded BeaconState
    err = ds.UnmarshalSSZReader(&decoded, file, int(info.Size()))
    if err != nil {
        panic(err)
    }
    file.Close()
}
```

## Related Documentation

- [Getting Started](getting-started.md) - Basic usage
- [API Reference](api-reference.md) - Complete API documentation
- [Code Generator](code-generator.md) - Code generation options
- [Performance Guide](performance.md) - Performance optimization tips
