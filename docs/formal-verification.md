# Formal Verification

dynamic-ssz ships machine-checked certificates for its SSZ semantics, under
[`formal/`](../formal/README.md). This page says what that means for a user,
what is actually proved, and where the limits are.

## The idea in one paragraph

A proof about Go code that uses `reflect` and `unsafe` is out of reach of
every Go verifier, so the verification is layered. For a type, `dynssz-certify`
reads the *same* type descriptor the code generator consumes and writes it
down as a term of a verified model, twice: as a codec expression over an
in-house Lean 4 combinator library (`formal/lean`), and as a declaration of
the official Lean specification of SSZ, [`ethereum/EthLib`](https://github.com/ethereum/EthLib)
(`formal/spec`). Type-checking those files proves, per type, that the SSZ
layout is exactly right: every accepted byte string is the canonical
serialization of its value, every value survives a round trip, no value has
two encodings, the explicit Merkle tree hashes to the direct root, and every
readable node has a Merkle branch that verifies. A differential harness then
ties the library to those models: both engines, the generated code and the
reflection walker, must agree with the compiled oracles on verdicts, bytes,
roots, branches and multiproofs for every fixture and random value, on
every public API path (buffers, streams, sizes, sync and async hashing,
both hash backends, trees and proofs). The Ethereum consensus types are
certified per preset and their layouts are proved equal to EthLib's own
consensus containers. The incremental/async hasher has an executable Lean
model, proved to compute the specified root for every call sequence the
engines make, and the Go hasher is compared with that model state for state
after every call. The strength of all links is measured by mutation testing.

## What is proved, per certified type

| Property | In-house model (`lean/`) | Official spec (`spec/`) |
|----------|:-:|:-:|
| Decode-canonicity (`D(b)=v ⇒ S(v)=b`; the Ghost-in-the-Block class) | ✔ `C_T_canonicity` | ✔ `D_T_canonical` |
| Round trip (`D(S(v))=v`) | ✔ `C_T_roundtrip` | ✔ `D_T_roundTrip` |
| Injectivity / non-malleability | ✔ (library corollary) | ✔ `D_T_injective` |
| Decoded values fit the declaration | — | ✔ `D_T_fits` |
| Every admissible value has a 32-byte root | — | ✔ `D_T_root` |
| Explicit tree hashes to the direct root (issue #191 class) | ✔ `C_T_tree_root` | (by construction of EthLib's trees) |
| Merkle branch for every readable node verifies | — | ✔ `D_T_proof` |
| Well-formed declaration | (by construction) | ✔ `D_T_wf` |
| Layout = EthLib's consensus container (Ethereum types) | — | ✔ `X_<preset>_<fork>_<T>` |

All theorems are `sorry`-free and rest on Lean's three standard axioms;
`formal/audit-axioms.sh` enforces that in CI.

## What is proved about the hasher

`hasher.go`/`async.go` keep a reduction machine: chunks are batch-collapsed
by depth, small child scopes are parked in their parent's deferred run and
flushed later, cap-wide blocks and large progressive groups are reduced by
background workers whose results land in reserved holes of the buffer, and
drains synchronize. Its Lean model (`formal/lean/SszCert/Hasher/Model.lean`)
is held to the Go hasher call by call. On that model,
`run_stree` (`Hasher/Tree.lean`) proves:

> For every tree of calls the engines make — binary, non-incremental,
> `Index` and progressive (EIP-7916) scopes nested to any depth; leaves;
> `Collapse` hints anywhere, each synchronous or asynchronous; children
> closed with `Merkleize` (deferred into an incremental or progressive
> parent exactly when Go defers them), `MerkleizeWithMixin`,
> `MerkleizeProgressive`, `MerkleizeProgressiveWithMixin` or
> `MerkleizeProgressiveWithActiveFields` — `HashRoot` returns the root the
> SSZ specification assigns, and none of the model's protocol checks fires:
> no hole is read or moved before its drain, no scratch write lands on a
> live chunk, no impossible state is reached.

The theorem is generic in the hash function and holds for any thresholds
satisfying `ParamsOK` (Go's production constants and the `dynssz_smallhash`
test constants are both shown to). It covers batch collapse,
`precollapseCapRun` and its ring-full drains, deferral and its padding
(including the `c&(c-1)` width trick), `flushPending` in both modes,
`collapseAllDepths`, the progressive rounds with Go's depth-greedy group
count, background group jobs, compaction, and `collapseProgressiveLayer`.

## The generic link

The link is not confined to a hand-picked set of types. The spec oracle
accepts a declaration at run time and proves it well-formed by evaluation
before answering for it, and EthLib's theorems hold for every well-formed
declaration — so any Go type can be linked without a certificate file.
`formal/corpus.sh` generates a corpus from a grammar of SSZ shapes
(`formal/harness/typegen`): every kind dynamic-ssz encodes in every Go
representation and size class as a root type; every class of child under
every parent context (containers, progressive containers, vectors, lists,
progressive lists, both unions, optionals, wrappers); the inherited
properties — variable size, spec-parametric size — through a middle type;
and random shapes drawn deeper. Each generated type carries the declaration
it is meant to have, computed from the shape alone; both descriptor builders
of dynamic-ssz (reflection and go/types) must read exactly that declaration
out of the generated struct tags, and the type is then linked like a
certified one. Shapes the specification forbids but the library accepts are
declared as such and the oracle must refuse them.

## What the link checks

For every certified type, on ssz-specs fixtures (when configured), on
consensus-spec-tests cases (Ethereum types, against pyspec's recorded roots
too) and on boundary-biased random values plus byte mutations, through every
encoding, decoding and merkleization path of both engines: accept/reject
verdicts; byte-exact
re-serialization of accepted inputs (a theorem on the oracle side, so a
mismatch would be a compiler defect); `HashTreeRoot` and `GetTree().Hash()`
of both engines against the certified root; `treeproof.Prove`/`ProveMulti`
against EthLib's `buildProof`/`buildMultiproof`, with each side's proofs
verified by the other's verifier; agreement on which indices are refused.

## Certifying your own types

```bash
go run github.com/pk910/dynamic-ssz/formal/certify \
  -package ./mytypes -types BeaconBlock,BeaconState \
  -output formal/lean/SszCert/Certified.lean \
  -spec-output formal/spec/SszSpec/Certified.lean \
  -spec-theorems formal/spec/SszSpec/Theorems.lean
(cd formal/lean && lake build)                       # in-house certificates
(cd formal/spec && lake exe cache get && lake build) # EthLib instances
```

The certifier honours the same tags and `sszutils.Annotate` calls as
`dynssz-gen`, with delegation disabled, so the certified shape is the type's
own SSZ layout. Spec expressions (`dynssz-size`, `dynssz-max`) are resolved
with the default specs: a certificate is one concrete layout, so certify
once per preset you care about (the Ethereum types in `formal/harness` are
certified from the runtime descriptors under each preset; see
`TestEthCertificatesFresh`).

## Limits

* The Go code is not itself proved; it is linked to proved models. The
  reflection engine can never be verified deductively (no Go verifier
  accepts `reflect`), so the differential link is a permanent part of the
  assurance story, not a stopgap.
* SHA-256 is trusted (executable and FIPS-validated in Lean, never
  unfolded); the SIMD `hashtree` backend is exercised, not verified.
* dynamic-ssz's extended types (classic unions, `optional`, signed ints,
  floats, `big.Int`) are certified against the in-house model only: EthLib
  defines no such types. `custom` types and recursive types are not
  certifiable (the latter could be, as their depth-bounded unrolling).
* The hasher proof is about its model. That the model matches
  `hasher.go`/`async.go` rests on the call-by-call trace comparison
  (millions of calls at small thresholds, a production-threshold pass):
  linked, not proved. The model abstracts what does not affect the
  reduction: partial packed chunks (a chunk enters once complete), worker
  scheduling (a job's result is fixed at enqueue; drains are the
  synchronization points), and the multi-hasher pool. Misuse (a close that
  does not fit the scope, or more chunks than a mix-in's limit) is outside
  the theorem; Go reports it as an error.
* `treeproof` proof compression is outside the oracles' surface.

See [`formal/README.md`](../formal/README.md) for the full account, including
the numbers of the latest runs.
