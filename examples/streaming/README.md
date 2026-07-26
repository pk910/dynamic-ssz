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
2. **`UnmarshalSSZReader`** — stream-decode with a known total size (from
   `os.Stat`, or an HTTP `Content-Length`). The size is required because SSZ
   offsets can't be interpreted without it.
3. **A size-known / size-unknown fallback** for HTTP downloads: stream when
   `Content-Length` is present, fall back to `io.ReadAll` + `UnmarshalSSZ`
   for chunked responses. (Tip: request with `Accept-Encoding: identity` to
   keep `Content-Length` available on beacon API downloads.)
4. **`io.Pipe` streaming** — encoder goroutine feeding a decoder with no
   intermediate buffer at all, the shape of direct network transmission.
5. **Allocation measurement** — buffered decode allocates payload + struct,
   streamed decode allocates the struct only.

## Notes

- **Untrusted input**: the unknown-size fallback reads to EOF unbounded. Wrap
  untrusted readers in an `io.LimitReader` and pass that limit as the size.
- **CPU trade-off**: streaming costs ~1.3x (encode) to ~2x (decode) CPU
  because stream encoders can't seek back to patch offsets. Use it for large
  payloads, not small messages.
- **Codegen**: `dynssz-gen -with-streaming` (or `with-streaming: true` in a
  config file) additionally emits `MarshalSSZEncoder`/`UnmarshalSSZDecoder`
  methods so generated types take the optimized path under
  `MarshalSSZWriter`/`UnmarshalSSZReader` too. See
  [docs/streaming.md](../../docs/streaming.md).
