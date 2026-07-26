# Chain Specs Example

One binary, any preset: serialize the same Go types correctly under the
mainnet preset, the minimal preset, or any custom devnet configuration.

This is the core problem dynamic-ssz solves in production. Statically
generated SSZ code bakes preset constants (vector lengths, list limits) into
the generated methods, so anything but mainnet serializes and merkleizes to
the wrong bytes and wrong hash tree roots. Devnet tooling, beacon API
clients, and genesis generators instead load the chain config at runtime —
from a `config.yaml` or the beacon node's `/eth/v1/config/spec` endpoint —
and pass it to `dynssz.NewDynSsz`.

## Run

```bash
go run .
```

## What it shows

1. **Config loading** — a beacon-chain config YAML is parsed into the
   `map[string]any` spec format `dynssz.NewDynSsz` expects (integers →
   `uint64`, `0x...` strings → `[]byte`).

2. **The dual-tag convention** used on Ethereum consensus types:

   ```go
   BlockRoots  [][32]byte `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
   Validators  []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
   ```

   The `ssz-*` tag carries the mainnet fallback (used when no spec value is
   provided), the `dynssz-*` tag carries the expression resolved against the
   runtime spec map.

3. **Spec expressions** — `dynssz-size`/`dynssz-max` support arithmetic:

   ```go
   SyncCommitteeBits []byte   `ssz-size:"64" dynssz-size:"SYNC_COMMITTEE_SIZE/8" ssz-type:"bitvector"`
   ProposerLookahead []uint64 `ssz-size:"64" dynssz-size:"(MIN_SEED_LOOKAHEAD+1)*SLOTS_PER_EPOCH"`
   ```

4. **Fallback equivalence** — `NewDynSsz(nil)` encodes byte-identically to
   the mainnet-spec instance, so mainnet users pay no spec-resolution cost.

5. **Mismatch detection** — decoding minimal-preset bytes with a
   mainnet-preset instance fails with a size error instead of producing
   silently corrupt data.

## Choosing an instance strategy

| Pattern | When |
|---|---|
| `NewDynSsz(specs)` per spec set | multiple networks in one process |
| `SetGlobalSpecs` + `GetGlobalDynSsz()` | one network per process; types with generated `MarshalSSZ()`/`HashTreeRoot()` facades route through the global instance |
| `NewDynSsz(nil)` singleton | mainnet-only tools that just want dynamic-ssz's streaming/proof features |

Instances cache type descriptors — create them once and reuse them.
