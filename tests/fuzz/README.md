# dynamic-ssz differential fuzzer

A generate-then-fuzz harness that invents random SSZ type trees, generates
matching codegen for them, and then differentially exercises every SSZ code
path against each other and against independent oracles.

## How it works

1. **Generate** (`cmd/generate`) emits `NumTypes` random Go structs spanning the
   full breadth of SSZ shapes — primitives, byte/int arrays, vectors and lists,
   bitvectors and bitlists, progressive lists/bitlists/containers, unions, type
   wrappers, `uint128`/`uint256`, optionals, multi-dimensional mixed
   `ssz-size`/`ssz-max` arrays, and nested containers — writes them to
   `corpus/types_gen.go` + `corpus/registry_gen.go`, and runs `dynssz-gen` to
   produce `corpus/codegen_gen.go`. The generated `*_gen.go` files are
   git-ignored and regenerated each run, so every round explores a fresh type
   universe.
2. **Fuzz** (`cmd/fuzz`) runs N workers over the generated registry. Each
   iteration picks one of three input modes — valid fill, mutated-valid, or
   random bytes — and runs the check battery below.

## Check battery

For every type, per iteration:

### Reflection-vs-codegen differential (the core)
The reflection engine and the generated code are two independent
implementations of the same SSZ type. They are cross-checked on:

- **marshal** — reflection bytes == codegen bytes
- **hash-tree-root** — reflection root == codegen root
- **streaming** — buffer marshal == stream writer; stream reader (known and
  unknown/open-region length, with a tiny read buffer forcing genuine open
  regions) round-trips to the buffer bytes
- **unmarshal** — reflection and codegen agree on accept/reject; on accept, both
  re-marshal identically
- **round-trip** and **decode-into-dirty-target** reuse
- **mutation** — bit flips, byte edits, insert/delete, targeted 4-byte
  offset-smashing, and byte swaps, routed through both the buffer and the
  unknown-length (offsets-at-EOF) verdict paths

### Deep-oracle battery (`-oracles`, default on)
Layered on top of the differential, for each valid instance:

- **native-hash HTR** — HashTreeRoot on the standard-library sha256 backend must
  equal the accelerated (hashtree) backend: a third hashing engine
- **size == len** — `SizeSSZ` must equal `len(MarshalSSZ)`
- **tree generation + proofs** (`-proofs`, default on) — `GetTree()` on both the
  reflection and codegen engines must equal `HashTreeRoot` and produce
  structurally identical trees; sampled leaf `Prove`/`ProveMulti` proofs must
  verify (completeness) and every tamper — leaf byte, sibling hash, wrong root —
  must be rejected (soundness)
- **HashTreeRootWith** — a caller-supplied hasher, fresh and reused-after-`Reset`,
  must match `HashTreeRoot`
- **metamorphic HTR** — two distinct serializations of the same type must not
  share a root
- **determinism** — `HashTreeRoot` is stable across repeats
- **independent reference oracle** (`-reference`, default on) — a
  first-principles reflection SSZ reimplementation (`engine/oracle_ref.go`) that
  shares no code with dynssz, catching a reflection==codegen-but-both-wrong bug
  the self-differential checks cannot see. It covers the well-modeled
  single-dimensional subset (primitives, byte/int vectors and lists, bitvector,
  nested containers, single-dim collections of containers) and skips constructs
  whose independent reimplementation is ambiguous (multi-dimensional collections,
  bitlist delimiter canonicalization, progressive/optional/union/wrapper,
  uint128/256, spec-driven `dynssz-*`, and extended scalars).

Any divergence is written to `report-dir` as an `issue-*` directory with the
input bytes and a hex dump; CI fails the round on any issue.

## Usage

```bash
# one-shot: generate a corpus then fuzz it (see Makefile for parameters)
make fuzz DURATION=60s NUM_TYPES=100 MAX_DEPTH=4

# or by hand
go run ./cmd/generate -num-types 100 -max-fields 8 -max-depth 4 -seed 1 -extended=true
go run ./cmd/fuzz -duration 60s -workers 8 -report-dir fuzz-reports

# toggle sub-batteries
go run ./cmd/fuzz -oracles=false                 # differential only
go run ./cmd/fuzz -proofs=false -reference=false  # oracles minus tree/proofs and reference
```

The stats line reports, per interval: iterations/s, the input-mode split, `ok`
(valid fills), `oracle` (deep-oracle checks run), and a counter per issue class
(`panic`, `marshal`, `htr`, `stream`, `unmarshal`).
