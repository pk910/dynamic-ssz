# Formal certificates for dynamic-ssz

Machine-checked, per-type correctness certificates for SSZ serialization —
a proof-carrying companion to `dynssz-gen`.

For every Go type you certify, `dynssz-certify` builds the same go/types
descriptor that dynssz-gen's code generation consumes and translates it into
a *codec expression* over a verified combinator library written in Lean 4.
Each combinator carries machine-checked proofs, so type-checking the
generated certificate file (`lake build`) re-verifies, for that concrete
type layout:

* **Decode-canonicity (P2)** — every byte string the strict decoder accepts
  is *exactly* the canonical serialization of its decoded value. Ghost gaps,
  non-minimal or backward offsets, misaligned first offsets, nonzero
  bitvector padding, missing bitlist sentinels, trailing bytes: all
  impossible. This is the property behind the 2024 "Ghost-in-the-Block"
  consensus-split vulnerability class.
* **Roundtrip (P1)** — every well-typed, offset-representable value decodes
  back from its serialization.
* **Tree/root agreement (P7)** — the explicit Merkle tree the type
  merkleizes into (the structure a `GetTree`-style API walks) hashes to
  exactly the direct hash-tree-root. This is the property whose violation
  was issue #191 (`GetTree` reordering leaves against `HashTreeRoot` on
  Deneb+ `BeaconState`s), as a machine-checked theorem per type. The
  merkleization model covers bounded trees with zero-subtree padding,
  packed and composite leaves, the EIP-7916 progressive spine, EIP-7495
  progressive-container layouts, and the length/active-fields/selector
  mix-ins.
* Injectivity and unique-encoding (non-malleability) follow as corollaries
  inside the library.

## Layout

```
formal/
  lean/                  Lean 4 package (core Lean only, no Mathlib, no SMT)
    SszCert/Basic.lean       uintN little-endian primitives + theorems
    SszCert/Schema.lean      schema-generic variable-offset containers (the
                             theorem eth2.0-dafny#26 left open)
    SszCert/Container.lean   concrete two-field container (didactic seed)
    SszCert/VarList.lean     lists/vectors of variable-size elements
    SszCert/FixedSeq.lean    lists/vectors of fixed-size elements (packed)
    SszCert/Bitlist.lean     bitlist sentinel discipline
    SszCert/Bitvector.lean   bitvector padding discipline
    SszCert/Boolean.lean     strict 0x00/0x01 booleans
    SszCert/Sha256.lean      executable SHA-256 (opaque to all proofs)
    SszCert/Merkle.lean      merkleization: bounded + progressive trees,
                             mix-ins, explicit `Node` trees, and the
                             tree/root agreement theorems
    SszCert/Codec.lean       the composition layer: `Codec` structure with
                             proof fields + combinators for every SSZ shape
                             (containers, lists, vectors, bitfields,
                             CompatibleUnion, progressive types)
    SszCert/Certified.lean   generated certificates (see below)
    Main.lean                `ssz-oracle`: executable decision oracle
  certify/               the certificate generator (Go)
  harness/               the differential link (Go module; see harness/README.md)
  certify.sh             regenerates harness methods + every certificate (CI: no diff)
  audit-axioms.sh        prints/enforces the axioms every theorem depends on
  spec/                  anchoring on the official Lean specification
    lakefile.toml            requires ethereum/EthLib at the ssz-specs-pinned rev
    SszSpec/Certified.lean   generated `Desc` declarations + registry
    SszSpec/Theorems.lean    generated per-type instances of EthLib's theorems
    Main.lean                `ssz-spec-oracle`: decode/root/branch/multiproof oracle
```

Every theorem is `sorry`-free and depends only on Lean's three standard
axioms (`propext`, `Classical.choice`, `Quot.sound`); there is no SMT solver
and no Mathlib in the trusted base.

## Usage

```bash
# 1. Generate certificates for your types (same package conventions as
#    dynssz-gen, including sszutils.Annotate tags):
go run github.com/pk910/dynamic-ssz/formal/certify \
  -package ./path/to/types -types TypeA,TypeB,... \
  -output <repo>/formal/lean/SszCert/Certified.lean

# 2. Machine-check them (this *is* the verification step):
cd formal/lean && lake build ssz-oracle

# 3. (Optional) differential link: the built ssz-oracle binary answers
#    "<TypeName> <hex|->" lines on stdin with ACCEPT/REJECT, letting a test
#    harness compare the verified model's accept/reject decision against
#    UnmarshalSSZ on any corpus. ACCEPT answers carry the certified hash
#    tree root, for comparison against HashTreeRoot and GetTree().Hash()
#    on both engines. On ACCEPT the oracle also re-serializes byte-for-byte
#    and re-hashes the explicit tree against the direct root (the theorems
#    make any mismatch a compiler bug, not a model question).
```

Requires [elan](https://github.com/leanprover/elan) (the Lean toolchain in
`formal/lean/lean-toolchain` is installed automatically on first build).

The checked-in `Certified.lean` carries the 57-type certificate set of the
dynamic-ssz conformance harness (mirrors of the ethereum/ssz-specs fixture
types: basics, boundary bitvectors, progressive lists/containers,
CompatibleUnions), so `lake build` in a fresh checkout re-verifies a
realistic certificate load end to end.

## Anchoring on the official specification (`spec/`)

`spec/` ties the same certified types to the *official* Lean specification of
SSZ, [`ethereum/EthLib`](https://github.com/ethereum/EthLib)
(`EthLib.Consensus.Ssz`, at the revision `ethereum/ssz-specs` pins for its
own conformance harness). EthLib represents types as data (`Desc`) and
proves its results once for every well-formed declaration: codec
canonicality and round trip, decode admissibility, root totality, Merkle
branch correctness, multiproof completeness and binding (two accepted
proofs disagree only through a SHA-256 collision).

`dynssz-certify -spec-output ... -spec-theorems ...` therefore emits, per
Go type, its declaration `D_<T> : Desc` and the fact `D_<T>.wellFormed =
.ok ()` (proved by evaluation), followed by the instantiated theorems:
`D_<T>_canonical`, `D_<T>_roundTrip`, `D_<T>_injective`, `D_<T>_fits`,
`D_<T>_root`, `D_<T>_proof`. Checking `spec/` (`lake build
SszSpecTheorems`) is the verification step; it pulls EthLib's proof
library, which is Mathlib-based.

`ssz-spec-oracle` (`lake build ssz-spec-oracle`, Mathlib-free) answers
decode verdicts, roots, single branches and multiproofs for the declared
types straight from EthLib's executable functions, so a harness can link
`UnmarshalSSZ`/`HashTreeRoot`/`GetTree` *and* `treeproof.Prove`/
`ProveMulti`/`VerifyProof`/`VerifyMultiproof` to the specification —
including cross-verification of each side's proofs by the other. The
library's own `SszCert` model and EthLib are independent formalizations;
agreement of both oracles on the same corpora cross-checks the two.

```bash
go run github.com/pk910/dynamic-ssz/formal/certify \
  -package ./path/to/types -types TypeA,TypeB,... \
  -output <repo>/formal/lean/SszCert/Certified.lean \
  -spec-output <repo>/formal/spec/SszSpec/Certified.lean \
  -spec-theorems <repo>/formal/spec/SszSpec/Theorems.lean
cd formal/spec && lake build ssz-spec-oracle          # executable spec only
cd formal/spec && lake exe cache get && lake build SszSpecTheorems   # the proofs
```

## Results

Checked in CI (`.github/workflows/formal.yml`) and reproduced locally against
dynamic-ssz v1.4.0 on 2026-11-06:

* `lake build` of `lean/`: 171 per-type theorems (57 × canonicity, roundtrip,
  tree/root), 0 `sorry`, standard axioms only.
* `lake build SszSpecTheorems` of `spec/`: 399 per-type instances of EthLib's
  theorems (57 × well-formedness, canonicity, roundtrip, injectivity,
  admissibility, root totality, branch correctness), standard axioms only
  (well-formedness by evaluation — `decide`, or `simp` through the
  equations of EthLib's well-founded compatibility check for unions).
* Link, in-house oracle: ssz-specs release fixtures (11,504 checks) +
  Python-classified corpora (13,255 + 21,470) + generative rounds — zero
  divergences across both engines.
* Link, EthLib oracle: the same corpora, 46,229 checks; 27,025 values
  rooted; 117,911 Merkle branches and 18,555 multiproofs byte-identical
  with `buildProof`/`buildMultiproof` and cross-verified both ways; 32,450
  refusals of unprovable indices agreed. The single policy difference:
  EthLib refuses a branch for the root itself (`ROOT_HAS_NO_BRANCH`) where
  `treeproof.Prove(1)` returns the empty branch (which verifies trivially).
* Both oracles agree with each other on every vector (transitively through
  the Go engines) — two independent formalizations, one of them the
  official specification.

## What is proved vs. what is trusted

Machine-checked (per certified type): canonicity, roundtrip, injectivity,
and tree/root agreement of the *model* codec the certificate denotes — the
exact offset/limit/padding/sentinel discipline of SSZ serialization and the
chunking/padding/spine/mix-in discipline of its merkleization.

Trusted (the differential link carries these, not proofs):

* the descriptor→combinator translation in `certify/` (one screenful of
  structural mapping per SSZ kind, sharing the descriptor source that
  code generation itself consumes);
* the correspondence between the generated Go method text and the model —
  linked empirically by running both on corpora (fixture replay + random
  values + mutations), where the canonicity theorem collapses byte-level
  agreement to accept/reject agreement;
* the SHA-256 implementation (never unfolded by any proof; validated
  against the FIPS vectors and continuously by the root comparisons);
* Lean's kernel and compiler, and the Go toolchain.

## Gaps — what is not (or cannot be) covered

* **The library code itself is not proved.** The theorems are about the
  *model* each type denotes (the exact SSZ layout the code generator sees);
  the Go engines are tied to the model by the differential link. Go has no
  deductive verifier that accepts `reflect` or `unsafe`, so the reflection
  engine is empirically assured only, for good: the link is permanent, not
  a stopgap. Proof-carrying translation validation of the generated method
  *text* (Goose/Rocq style) would be the next rung and is grant-scale.
* **Hashing.** SHA-256 is executable and FIPS-validated on the Lean side and
  never unfolded by a proof; the SIMD backend (`hashtree`) and its pure-Go
  fallback are exercised by every root comparison but not verified
  (verification of those kernels is a separate effort).
* **Types outside the specification.** `ssz-specs`/EthLib define no classic
  positional `Union[T]`, `StableContainer`, `Optional[T]`, or dynssz's
  extended types (`optional`, signed ints, floats, `big.Int`, custom
  types). The certifier refuses them, so a type using them cannot be
  certified yet; a model of dynssz's `optional`/`optional-list` could be
  added to the in-house combinators (with no official spec to anchor it).
* **Recursive types** (v1.4.0): a certificate is a finite term, and so is
  an EthLib `Desc`. The library bounds nesting (`WithMaxNestingDepth`),
  so certifying the depth-bounded unrolling would match the implemented
  semantics exactly; not done yet.
* **Spec-parametric sizes** (`dynssz-size`/`dynssz-max` expressions) are
  certified per *resolved* preset: a certificate is one concrete layout.
  The harness's preset-parametricity property test covers the expression
  machinery empirically.
* **Views, type wrappers, delegation:** the certifier walks the same
  descriptor the generator does with delegation disabled, so what is
  certified is the SSZ shape — not which Go method ends up being called.
  Delegation precedence is covered by the library's own differential fuzzer.
* **Streaming mode** (`UnmarshalSSZReader` with unknown size) and
  `treeproof` compression are outside the oracles' surface.

Merkle *proof construction and verification* (`treeproof.Prove`/
`ProveMulti`/`VerifyProof`/`VerifyMultiproof`) is outside the `SszCert`
surface; it is covered by the EthLib anchoring in `spec/`, whose oracle
links the Go proofs to the specification's `buildProof`/`buildMultiproof`
and verifiers. Recursive Go types are rejected by the certifier (Lean
certificates are non-cyclic terms).
