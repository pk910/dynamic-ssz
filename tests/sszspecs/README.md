# Official SSZ specification vectors

This package runs the conformance vectors of
[`ethereum/ssz-specs`](https://github.com/ethereum/ssz-specs) against both
engines of the library: the generated SSZ methods and the reflection engine.

```bash
./setup_test_data.sh setup
SSZ_SPECS_TESTS_DIR="$(./setup_test_data.sh export)" go test -v -run TestOfficialSSZSpecs .
```

## Where the vectors come from

The `v0.1.0` release archive carries serialization and root vectors only. The
proof vectors exist on the upstream `main` branch and in no release yet, so
`setup_test_data.sh` pins a commit of `main` and generates the vectors from it
with the upstream tool, which needs [`uv`](https://docs.astral.sh/uv/). The
script checks the generated `index.json` against a pinned SHA-256 digest. That
file states the digest of every vector, and the test checks each vector
against it, so the whole set is pinned. The test also requires the index to
list as many cases as the set's own manifest states, and fails on a fixture
format it has no handling for, so neither a partial set nor a format upstream
adds later can silently reduce coverage.

To move to a newer upstream commit, change `COMMIT` and `INDEX_SHA256` in
`setup_test_data.sh`. No case count is written down anywhere else.

## How the types are built

A vector names its type with a descriptor, not with Go code. `setup` runs two
generators:

1. `./gen` reads the descriptors and writes one Go type per distinct
   declaration to `gen_types.go`, registered under the declaration's key. The
   declarations of the illegal-type vectors go to `gen_illegal.go`.
2. `dynssz-gen` generates the SSZ methods for the types in `gen_types.go` into
   `gen_ssz.go`.

All generated files are ignored by git. Without them the package compiles and
the test skips.

## What is run

| Format | What the test does |
| --- | --- |
| `ssz` | Valid: decode, re-encode, size, hash tree root, and the same through the stream reader (known and unknown size) and writer. Invalid: every decoder has to refuse the bytes. |
| `proof` | The value's tree has to give the stated root, hold the stated leaf at the index and prove it with the stated branch, and the branch has to verify. An invalid proof must not verify. |
| `multiproof` | The same for several indices: leaves, helper nodes and verification. |
| `ssz_type_rejection` | The library has to refuse the declaration. |
| `ssz_gindex` | Not run: the library resolves no path to a generalized index. |
| `ssz_json` | Not run: the library has no SSZ JSON mapping. |

Every case runs on both engines. The test ends with a tally per format, and
each case that is not a plain pass falls into one of these groups:

- **Feature not in the library.** The two formats above.
- **Type not expressible in the library.** A list, byte list or bit list with
  a capacity of zero. The library reads a limit of zero as "no limit stated".
- **Illegal type not expressible in Go.** Declarations such as a 24-bit
  integer or a progressive container layout that ends on a gap. Go has no
  spelling for them, so they cannot reach the library.
- **Spec-faithful where the suite is stricter.** Cases the vectors mark
  `stricterThanSpec`, where the library does what the specification says.
- **Known divergence.** Listed by case id in `divergences_test.go` with the
  reason. A listed case that starts to pass fails the test until it is
  removed, so the list cannot go stale.

## What is not asserted

- The decoded value is not compared against the vector's `value`. The
  re-encoded bytes and the root are compared instead.
- A refusal is required, but not the `rejectionReason` name the vector states.
- The `path` of a proof vector is not resolved; its `index` is used.
