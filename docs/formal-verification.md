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
model that the Go hasher is compared with state for state after every
call, and the strength of all links is measured by mutation testing.

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
* The hasher's model is checked against the Go hasher call by call; the
  merkleization laws it relies on are proved, the theorem that the model
  as a whole always computes the specified root is in progress.
* `treeproof` proof compression is outside the oracles' surface.

See [`formal/README.md`](../formal/README.md) for the full account, including
the numbers of the latest runs.
