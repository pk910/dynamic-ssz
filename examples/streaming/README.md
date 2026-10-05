# Streaming Example

Stream SSZ directly to/from files and network connections without holding the
serialized payload in memory next to the decoded struct.

The canonical use case is beacon state ingestion: a mainnet state is ~310 MB
serialized, and decoding it straight off the HTTP response body with
`UnmarshalSSZReader` drops the duplicate payload buffer:

```go
var SSZ = dynssz.NewDynSsz(nil, dynssz.WithStreamReaderBufferSize(1<<20))
```

## Run

```bash
go run .
```

## What it shows

1. **`MarshalSSZWriter`** — stream-encode a 100k-validator registry (~11 MB)
   straight into a file.
2. **`UnmarshalSSZReader`** — stream-decode with a known total size from
   `os.Stat`. A non-negative size is trusted input: the decoder sizes its
   allocations from it before the bytes arrive, so it must come from a source
   you control, never from a remote peer's `Content-Length`.
3. **Unknown-size decoding** for HTTP responses: pass a negative size and the
   payload is consumed to EOF without being buffered, bounded by
   `WithMaxStreamSize` or a per-call `WithStreamSizeLimit`. This is the path
   for any body whose length is the server's claim.
4. **`io.Pipe` streaming** — encoder goroutine feeding a decoder with no
   intermediate buffer at all, the shape of direct network transmission.
5. **Allocation measurement** — buffered decode allocates payload + struct,
   streamed decode allocates the struct only.

## Notes

- **Untrusted input**: an unknown-size decode is always bounded by
  `WithMaxStreamSize` (512 MiB by default), and `ssz-max` limits are enforced
  while reading. Lower the bound to the smallest value your protocol permits.
  This caps wire bytes, not decoded-object memory or read time.
- **EOF and cancellation**: EOF is the message boundary in unknown-size mode.
  Use request contexts/client timeouts for HTTP and read deadlines for raw
  connections. Canceling must close or otherwise unblock the `io.Reader`.
- **CPU trade-off**: streaming costs ~1.3x (encode) to ~2x (decode) CPU
  because stream encoders can't seek back to patch offsets. Use it for large
  payloads, not small messages.
- **Codegen**: `dynssz-gen -with-streaming` (or `with-streaming: true` in a
  config file) additionally emits `MarshalSSZEncoder`/`UnmarshalSSZDecoder`
  methods so generated types take the optimized path under
  `MarshalSSZWriter`/`UnmarshalSSZReader` too. See
  [docs/streaming.md](../../docs/streaming.md).
