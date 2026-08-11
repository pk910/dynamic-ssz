# HTR Caching Example

dynamic-ssz does not cache hash tree roots itself — cache lifetime and invalidation depend on
application state transitions, so they belong to the application. This example shows how to build
per-element root caching on top of the library using the `sszutils.DynamicHashRoot` delegation
hook, on a registry mimicking the beacon-chain validator set.

## The pattern

Each list element is a thin wrapper around the actual data item:

```go
type CachedValidator struct {
    Data *Validator
    Root *[32]byte `ssz-type:"-"` // cached root, excluded from the SSZ schema; nil = dirty
}

func (v *CachedValidator) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
    if v.Root != nil {
        hh.PutBytes(v.Root[:]) // cache hit: contribute the root directly
        return nil
    }
    // cache miss: hash the wrapped data through the same walker, then capture
    // the root it left behind — right after a completed scope, hh.Hash()
    // returns that scope's root
    if err := ds.(*dynssz.DynSsz).HashTreeRootWith(v.Data, hh); err != nil {
        return err
    }
    var root [32]byte
    copy(root[:], hh.Hash())
    v.Root = &root
    return nil
}

func (v *CachedValidator) MarkDirty() { v.Root = nil }
```

When the registry is hashed, dynssz calls `HashTreeRootWithDyn` per element instead of walking the
wrapper itself: a cached root is contributed directly, otherwise the wrapped data is hashed in
place at the wrapper's position and the resulting root persisted. After the first full hash, a
re-hash only recomputes elements that were invalidated with `MarkDirty`.

## Why the wrapper is transparent

- **Merkleization**: an SSZ container with a single field merkleizes to that field's root, so
  contributing `Data`'s root as the wrapper's root yields the same registry root as a list of bare
  validators.
- **Serialization**: `Root` is excluded via `ssz-type:"-"` and `Validator` is static-sized, so the
  wrapper serializes to exactly the same bytes as a bare `Validator`. Marshal, unmarshal, and size
  all go through the normal reflection walk — only hashing is delegated.

## What the example demonstrates

1. **Cold hash** of 100,000 validators (empty cache), verified against an uncached registry
2. **Warm hash** — per-element hashing is skipped; only the list-level merkleization over the
   element roots still runs, so the speedup is bounded by that outer tree (~10x here)
3. **Incremental update** — mutate 25 validators, `MarkDirty` them, re-hash: only those 25 are
   recomputed, and the root matches the uncached reference
4. **The pitfall** — mutating `Data` without `MarkDirty` serves a stale root
5. **Round-trip** — cached and plain registries serialize to identical bytes; a decoded registry
   starts with empty caches and rebuilds them on the first hash

## Code generation

The example ships generated SSZ methods (`types_ssz.go`), produced by `dynssz-gen` from
`generate.yaml`. The wrapper entry uses `skip-hashtreeroot: true` so marshal/unmarshal/size are
generated while the hand-written caching `HashTreeRootWithDyn` stays in charge — the generated
registry code delegates to it per element:

```yaml
types:
  - Validator
  - name: CachedValidator
    skip-hashtreeroot: true
  - CachedRegistry
  - PlainRegistry
```

Regenerate with:

```bash
go generate .
```

## Running the Example

```bash
cd examples/htr-caching
go run .
```
