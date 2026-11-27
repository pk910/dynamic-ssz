# The link: dynamic-ssz ⇄ verified oracles ⇄ ssz-specs

The differential harness of the formal-verification effort (see
[`../README.md`](../README.md)). It links `github.com/pk910/dynamic-ssz` (both
engines) to the two machine-checked Lean oracles — `ssz-oracle` (the in-house
certified codecs, `../lean`) and `ssz-spec-oracle` (the official EthLib
specification, `../spec`) — and, when fixtures are configured, to the
executable Python specification `github.com/ethereum/ssz-specs`.

The 57 types in `types_*.go` are the certified types: `../certify.sh`
regenerates their `dynssz-gen` methods and all certificates from them.

| Test | Oracle | Checks |
|------|--------|--------|
| `TestOracleLink` | `ssz-oracle` | accept/reject verdicts, canonical re-serialization, 4-way root agreement (both engines' `HashTreeRoot` and `GetTree().Hash()`) |
| `TestSpecLink` | `ssz-spec-oracle` | the same, plus `treeproof.Prove`/`ProveMulti` against EthLib's `buildProof`/`buildMultiproof`, each side's proofs verified by the other (`VerifyProof`/`VerifyMultiproof` ⇄ `verifyMerkleProof`/`verifyMerkleMultiproof`), and agreement on refused indices |
| `TestConformance*`, `FuzzDecodeCanonicity` | Python fixtures | fixture replay on both engines (needs `SSZ_FIXTURES`) |
| `TestProp*` | — | rapid property tests: roundtrip, mutation canonicity, path agreement, preset parametricity |

Every link test has a fixture phase (skipped unless `SSZ_FIXTURES` points at
ssz-specs `just fill` output or a corpus from `oracle/gen_corpus.py`) and a
generative phase (random valid values + byte mutations of every certified
type) that needs nothing but the built oracles — that is what CI runs.

## What it checks

Properties (numbering from the project README):

| Property | Where |
|----------|-------|
| P1 roundtrip `D(S(v))=v` | conformance replay + `TestPropRoundtrip` |
| P2 decode-canonicity `D(b)=v ⟹ S(v)=b` (Ghost-in-the-Block) | every replayed vector, `TestPropMutationCanonicity`, `FuzzDecodeCanonicity` |
| P4 size agreement | conformance replay + `TestPropRoundtrip` |
| P5 decoder totality | fuzzing (mutation + native fuzz) |
| P6 path agreement (buffer/writer/reader/GetTree) | `TestPropPathAgreement` |
| P7 merkleization | root comparison on every vector |
| P9 preset-parametricity (dynssz-* spec expressions) | `TestPropPresetParametricity` |
| P10 engine equivalence (reflection ≡ codegen ≡ Python spec) | everything runs on both engines; fuzz cross-checks accept/reject |
| negative-vector rejection | conformance replay (`rejectionReason` vectors) |

## Layout

- `types_*.go` — Go mirror types for every ssz-specs fixture type, registered
  per fixture directory (type names collide across directories). Top-level
  non-struct types use `sszutils.Annotate[T]()` so both engines see the tags.
- `fill.go` — fixture-JSON → Go value bridge (containers, unions, bitfields,
  big uints, progressive types).
- `gen.go` — rapid-based random value generator honoring SSZ tags.
- `conformance_test.go` — replays fixture vectors; `SSZ_FIXTURES` selects the
  corpus, `RESULTS_OUT` dumps per-vector JSON results.
- `fuzz_test.go` — `FuzzDecodeCanonicity`, canonicity + cross-engine agreement
  on arbitrary bytes.
- `generated_ssz.go` — dynssz-gen output (57 types) exercising the codegen
  engine; conformance runs reflection via `WithNoDelegation()`.
- `../oracle/gen_corpus.py` — Python-spec-classified random + mutation corpus
  generator (see file header for usage).

## Running

```bash
# Build the oracles once (see ../README.md), then:
cd formal/harness
go test -count=1 -run 'TestProp|TestOracleLink|TestSpecLink' ./...

# Optional fixture replay: ssz-specs release fixtures (`just fill` in a
# checkout of ethereum/ssz-specs) or a Python-classified corpus:
SSZ_FIXTURES=/path/to/ssz-specs/fixtures/ssz/ssz go test -count=1 ./...
# corpus generation (run inside an ssz-specs checkout):
#   uv run --group test python /path/to/formal/harness/oracle/gen_corpus.py \
#     --out /tmp/corpus --seed 873767 --per-type 20 --mutations 10

# Oracle locations default to ../lean/.lake/build/bin/ssz-oracle and
# ../spec/.lake/build/bin/ssz-spec-oracle; override with SSZ_ORACLE /
# SSZ_SPEC_ORACLE.
```
