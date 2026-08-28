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

Merkle *proof verification* (`treeproof.Prove`/`VerifyProof` beyond the
root) is outside the certificate surface. Recursive Go types are rejected
by the certifier (Lean certificates are non-cyclic terms).
