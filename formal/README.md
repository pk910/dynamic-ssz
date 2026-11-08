# Formal verification of dynamic-ssz

Machine-checked models of SSZ for the types dynamic-ssz encodes, the
official Lean specification they are anchored on, an executable model of the
incremental and async hasher, and the differential links that tie the Go
code to all of them.

Two kinds of evidence are combined, and the difference matters:

* **Proved** (Lean 4, no `sorry`, standard axioms only, checked by
  `audit-axioms.sh`): properties of models — per-type codecs, EthLib's
  generic theorems instantiated per type, the layout of every certified
  Ethereum type against EthLib's own consensus containers, and the
  merkleization laws the hasher relies on.
* **Linked** (Go tests against the compiled models): the Go code itself —
  both engines on every public API path, and the hasher state-for-state.
  No deductive Go verifier accepts `reflect`/`unsafe`, so the link is
  permanent, not a stopgap. Its strength is itself measured
  (`mutation/`).

## Layout

```
formal/
  lean/                    in-house model (core Lean 4, no Mathlib)
    SszCert/Basic … Bitvector, Schema, VarList, FixedSeq, Boolean
                           the SSZ wire disciplines and their proofs
    SszCert/Codec.lean     Codec structure with proof fields; combinators for
                           every SSZ shape (containers, lists, vectors,
                           bitfields, CompatibleUnion, progressive types)
    SszCert/Extended.lean  dynamic-ssz extended types: classic Union (None
                           variant), optional, big.Int (signed ints and floats
                           certify as uintN over their bit pattern)
    SszCert/Merkle.lean    merkleization (bounded, progressive, mix-ins, trees)
    SszCert/Certified.lean generated: the harness types (70)
    SszCert/Eth.lean       generated: the Ethereum types, per preset (204)
    SszCert/Hasher/Model.lean  executable model of hasher.go/async.go
    SszCert/Hasher/Spec.lean   merkleization generic in the hash + laws
    Main.lean              ssz-oracle (verdicts, canonical re-encoding, roots)
    HasherModel.lean       hasher-model (replays hasher calls, prints state)
  spec/                    anchoring on ethereum/EthLib (Mathlib-backed proofs)
    SszSpec/Certified.lean, Eth.lean          generated EthLib declarations
    SszSpec/Theorems.lean, EthTheorems.lean   EthLib's theorems per type
    SszSpec/EthCross.lean  layout = EthLib's typed consensus container
    SszSpec/Layout.lean    layout equality (names ignored)
    SszSpec/WellFormed.lean  ssz_wf: the one place well-formedness is evaluated
    Main.lean              ssz-spec-oracle (verdicts, roots, branches, multiproofs)
  certify/                 dynssz-certify (go/types descriptors, as dynssz-gen)
  certify/certlib/         descriptor → Lean term translation (shared)
  harness/                 the differential links (own Go module)
  mutation/                planted bugs the links must catch, and their runner
  certify.sh               regenerates harness methods + certificates (CI: no diff)
  audit-axioms.sh          enforces standard axioms for every theorem
```

## What is proved

**Per certified type, in the in-house model** (`lake build` in `lean/`):
decode-canonicity with no side conditions (every accepted byte string is
exactly the encoding of its value — ghost gaps, non-minimal offsets,
padding bits, missing bitlist sentinels, trailing bytes, non-minimal
big.Int magnitudes, stray bytes after an absent optional: all impossible),
roundtrip, and tree/root agreement (the explicit tree a `GetTree` walk sees
hashes to the direct root). The extended types are certified here only:
nothing in EthLib defines them.

**Per certified type, in the official specification** (`lake build
SszSpecTheorems` in `spec/`): the declaration's well-formedness, then
EthLib's codec canonicality, roundtrip, injectivity, decode admissibility,
root totality and Merkle-branch correctness instantiated for it.

**Ethereum types against EthLib's consensus containers**
(`SszSpec/EthCross.lean`, 598 theorems): for every Go mirror of
`tests/spectests` (phase0 to gloas) under both presets, the declaration
dynamic-ssz derives from the Go struct tags and the preset's spec values
has the layout of EthLib's own definition of that container at that preset
and fork. This is independent of dynamic-ssz's reading of its tags. Two
equivalences are admitted, with justification in `Layout.lean`: byte-aligned
bitvectors equal byte vectors, and outside list/vector elements `uintN`
equals `ByteVector[N/8]`. 18 theorems prove a divergence instead (see
Findings).

**Hasher, generic in the hash** (`SszCert/Hasher/`, on the executable model
of `hasher.go`/`async.go` that the Go hasher is held to call by call):

* `Spec.lean`: the bounded Merkle root and its laws (pairing, zero
  expansion, zero padding, chunk levels, exact reductions, `getDepth`,
  `merkleizeImpl` = bounded root at the depth of its limit).
* `run_tree` (`Nested.lean`): for every tree of calls the engines make for
  a value — binary and non-incremental (`TreeTypeNone`) scopes nested to
  any depth; leaves; `Collapse` hints anywhere, each synchronous or
  asynchronous; children closed with `Merkleize` (parked in an incremental
  parent's deferred run exactly when Go defers them) or
  `MerkleizeWithMixin` — `HashRoot` returns the root the SSZ
  specification assigns, and none of the model's protocol checks fires: no
  hole is read or moved before its drain, no scratch write lands on a live
  chunk, no impossible state is reached. This covers batch collapse,
  `precollapseCapRun` (background cap jobs, ring-full drains), deferral
  (`pendStart/pendCount`), `flushPending` synchronous and asynchronous
  (`flushPendingAsync` jobs per cap-wide block), `collapseAllDepths`, the
  drains before every close, and padding to the subtree width
  (`deferrableElemChunks`, including its bit trick). The proof holds for any
  hash function and any thresholds with `ParamsOK` (both Go's production
  constants and the `dynssz_smallhash` ones are shown to satisfy it).
* Not yet proved (held to Go by the trace link only): progressive scopes
  (`TreeTypeProgressive`, `MerkleizeProgressive*`), `Index`/legacy scopes,
  and misuse paths (shape mismatch errors).

## What is linked

`harness/` (Go module; `go test` with the oracles built):

* **Every public API path, both engines.** `MarshalSSZ`, `MarshalSSZTo`,
  `MarshalSSZWriter` (also with a one-byte writer), `SizeSSZ`,
  `UnmarshalSSZ`, `UnmarshalSSZReader` with known and unknown size and with
  one-byte reads, tiny stream buffers, `HashTreeRoot` with synchronous and
  asynchronous hashing, the native sha256 backend, `GetTree`, and
  `treeproof.Prove`/`ProveMulti` cross-verified with EthLib's
  `buildProof`/`buildMultiproof`. All must agree with each other and with
  the model's verdict, canonical bytes and root.
* **Values** are drawn from the reflection engine's own type descriptor,
  biased to length boundaries (empty, one, limit−1, limit, chunk and
  power-of-two-chunk boundaries, the 256-chunk batch, progressive group
  sizes), and mutated (truncation, extension, bit flips, insertions,
  element-wide appends and drops).
* `TestOracleLink`, `TestSpecLink`: the 70 harness types against both
  models. `TestExtendedEdgeCases`: handcrafted non-canonical encodings of
  the extended types. `TestHasherAPILink`: the public helpers no engine
  calls (`PutBitlist`, `PutUint64Array`, `PutRootVector`, …).
* `TestEthLink`: every consensus-spec-tests `ssz_static` case through every
  path, against pyspec's recorded root and both models (three independent
  anchors), plus generated values and mutations of every Ethereum type.
* `TestRuntimeCertificatesAgree`: every certificate written again from the
  reflection engine's runtime descriptor must equal the go/types one — the
  two descriptor builders of dynamic-ssz agree.

`hasher/formal_trace_test.go` (`TestFormalTrace`): random walks of hasher
calls (nested binary, none, progressive, packed and legacy scopes, uniform
element runs that defer into their parent, Collapse hints anywhere, async
hashing switched per call, partial packed chunks) run on the real hasher
and on the Lean model, and after **every call** the complete reduction
state is compared: per scope its progressive roots and level, every node
with its depth, the counted prefix, the deferred run and the tail, and the
in-flight jobs with their hole positions. The model flags a hole read or
moved before its drain and a scratch write over a live chunk. The
`dynssz_smallhash` build tag shrinks the thresholds (`hasher/params*.go`)
so that every batching and async path is reached; production thresholds
run as a second pass.

`mutation/run.py` plants each bug of `mutation/mutants.json` (decoders,
generated code, hasher batching, progressive consumption, deferred runs,
mixins, async root compatibility and pre-collapse, proof generation) in a
copy of the tree and requires the links to kill it.

## Results (2026-11-08, dynamic-ssz v1.4.0 + this branch)

* Proofs: 213 in-house per-type theorems (harness) + 612 (Ethereum), 441 +
  1428 EthLib instances, 598 cross-check theorems, 26 hasher-spec
  theorems; all on standard axioms.
* Harness link: 70 types × every path, both models — zero divergences.
* Hasher trace: 3 seeds × 3000 walks (2.3 M calls) with small thresholds and
  30 walks (268 k calls) at production thresholds — zero divergences.
* Ethereum link, all forks, both presets (consensus-spec-tests
  v1.7.0-beta.4): 10,185 cases replayed through every path against pyspec's
  roots, 10,145 of them also through both models; 14,850 generated values
  with mutations; 106,575 Merkle branches and 14,850 multiproofs
  cross-verified with EthLib — zero divergences.
* Mutation testing: all 16 catalogued mutants killed.

## Findings

* **Minimal-preset `committee_bits` (consumer type annotations).** The
  Electra+ `CommitteeBits` of `tests/spectests` and of
  `ethpandaops/go-eth2-client` is tagged `bitfield.Bitvector64
  dynssz-size:"MAX_COMMITTEES_PER_SLOT/8"` without a bitvector type or bit
  size. dynamic-ssz therefore reads it as a byte vector; under the minimal
  preset (`MAX_COMMITTEES_PER_SLOT = 4`) that is one byte whose four padding
  bits go unchecked, where the specification has `Bitvector[4]`. An
  attestation with those bits set decodes (and re-encodes identically) in
  dynamic-ssz and is rejected by every spec client. Mainnet (64 bits) is
  unaffected. Fix in the type: `ssz-type:"bitvector"
  dynssz-bitsize:"MAX_COMMITTEES_PER_SLOT"`. `EthCross.lean` proves the 18
  affected declarations differ.
* **dynssz-gen is not idempotent in one run.** It analyses the package
  including its own previous output, and the methods found there change the
  analysis of other types: from scratch the harness output takes four runs
  to reach its fixed point. `certify.sh` iterates to it.

## Trust base

The descriptor → term translation in `certify/certlib` (cross-checked: both
descriptor builders, and the Ethereum layouts against EthLib); the link's
coverage (measured by mutation testing); SHA-256 and the SIMD backend
(exercised by every root comparison, never verified); Lean's kernel and
compiler; the Go toolchain.

## Not covered

* `custom` types (user-written methods): no semantics to certify.
* Recursive types: a certificate is a finite term; the depth-bounded
  unrolling (`WithMaxNestingDepth`) would match the implementation.
* Spec-parametric sizes are certified per resolved preset (mainnet and
  minimal for the Ethereum types).
* The Go source of the generated methods is linked, not proved (Grackle-
  style translation validation would be the next rung).

## Usage

```bash
./formal/certify.sh                                  # regenerate everything
(cd formal/harness && FORMAL_UPDATE=1 go test -run 'TestEthCertificatesFresh|TestEthCrossCheckFresh' .)
(cd formal/lean && lake build)                       # in-house proofs + oracles
(cd formal/spec && lake exe cache get && lake build && lake build SszSpecTheorems)
(cd formal/harness && go test -run 'TestOracleLink|TestSpecLink|TestExtendedEdgeCases|TestHasherAPILink' .)
HASHER_TRACE_WALKS=3000 go test -tags dynssz_smallhash -run TestFormalTrace ./hasher/
CONSENSUS_SPEC_TESTS_DIR=… go test -run TestEthLink ./formal/harness/   # tests/spectests/setup_test_data.sh
python3 formal/mutation/run.py
```

To certify your own types: `go run ./formal/certify -package <pkg> -types
A,B -output <lean file> [-spec-output <file> -spec-theorems <file>]`, then
`lake build` the packages.
