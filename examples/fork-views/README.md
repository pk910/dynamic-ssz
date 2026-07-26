# Fork Views Example

Serialize one runtime type with different per-fork SSZ schemas — no per-fork
type explosion.

Ethereum containers evolve across hard forks: fields get added, layouts
change. With views, a single fork-agnostic "data type" holds the union of all
fields, per-fork "view types" define the wire schemas, and a `Version` field
selects the view at runtime. This is how fork-agnostic beacon and builder-API
types are handled in production.

## Run

```bash
go run .
```

## What it shows

1. **Data type vs view types** — `BuilderBid` (the data type) carries all
   fields across forks plus a non-serialized `Version` selector. Each view
   type (`BuilderBidBellatrixView`, ...) is a schema-only struct: never
   instantiated, matched to the data type's fields by name, nested structs
   resolved through their own view types.

2. **`WithViewDescriptor`** — the same object marshals, unmarshals and
   hashes under any view:

   ```go
   data, err := ds.MarshalSSZ(bid, dynssz.WithViewDescriptor((*BuilderBidDenebView)(nil)))
   ```

3. **Version dispatch** — a `viewFor(version)` switch mapping the fork to a
   view descriptor, bridging the data type's `Version` field to the right
   schema.

4. **`ValidateType` at startup** — verify every view matches the data type
   once, instead of discovering a mismatch mid-request.

5. **Views compose with dynamic specs** — the deneb view's
   `dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK"` resolves against the
   instance's spec map, so the same view enforces different limits per
   network.

## Code generation

Reflection handles views out of the box. For hot paths, `generate.yaml` in
this directory shows the view-only codegen config: it emits per-view codecs
plus dispatchers on the data type, which the runtime picks up automatically —
same API, faster.

```bash
go run github.com/pk910/dynamic-ssz/dynssz-gen@latest -config generate.yaml
```

See [docs/views.md](../../docs/views.md) and
[docs/code-generator-config.md](../../docs/code-generator-config.md).
