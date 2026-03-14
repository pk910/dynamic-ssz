# Changelog

All notable changes to the `dynamic-ssz` library are documented here.

## [v1.2.2] — 2026-03-12

### Added
- Fuzz testing framework with parallel workers and multi-dimensional list/vector support
- Extended type support (`int8`/`int16`/`int32`/`int64`, `float32`/`float64`, `bigint`, `optional`) for reflection and code generation
- Fuzzing CI workflow
- Smoke tests for fuzzer in CI
- Codecov exclusion for fuzzer code

### Fixed
- Concurrent map read/write panic in `specValueCache` (added mutex synchronization)
- Minor code generation formatting issues

### Changed
- Refactored `getZeroOrderHashes` for clarity

### Breaking Changes
None.

---

## [v1.2.1] — 2026-01-21

### Added
- Progressive tree shape change implementation (EIP-7916 compatibility)

### Fixed
- Code generation for primitive pointer types
- Unused `encoding/binary` import in generated code
- Codegen indentation and spacing (now `go fmt` compatible)

### Changed
- Use `for range` syntax in generated code

### Breaking Changes
None.

---

## [v1.2.0] — 2026-01-02

### Added
- **Streaming SSZ support** — new `MarshalSSZWriter` and `UnmarshalSSZReader` entry points
- `Encoder` / `Decoder` interfaces abstracting buffer vs. stream operations
- `BufferEncoder`, `StreamEncoder`, `BufferDecoder`, `StreamDecoder` implementations
- `DynamicEncoder` / `DynamicDecoder` interfaces for codegen streaming support
- `WithCreateEncoderFn()` / `WithCreateDecoderFn()` codegen options
- `WithStreamWriterBufferSize()` / `WithStreamReaderBufferSize()` options
- Fulu spec tests
- `OffchainLabs/go-bitfield.Bitlist` auto-detection
- OOM protection test for tree proofs

### Fixed
- Hash tree root calculation for dynamic byte slices with >32 bytes in generated code
- Heap allocation in bitlist HTR calculation
- Various tree proof optimizations

### Changed
- Major codebase reorganization (`sszutils` package extracted)
- Inlined primitive encoding/decoding in generated code for performance
- Deduplicated dynamic expression evaluation in codegen
- Renamed `CanSeek` to `Seekable` on encoder/decoder interfaces
- Improved test coverage significantly

### Breaking Changes
- **`sszutils` package extracted** — interfaces previously in root package moved to `sszutils`
- **`CanSeek` renamed to `Seekable`** on `Encoder` and `Decoder` interfaces
- **Codebase reorganization** — internal package structure changed; import paths for sub-packages may differ

---

## [v1.1.2] — 2025-12-08

### Added
- `ssz-bitsize` / `dynssz-bitsize` struct tags for bitvector types with padding bit validation
- Sentinel bit validation for `Bitlist` types during unmarshal
- Boolean value validation during unmarshal (rejects values other than 0 and 1)

### Changed
- Bumped Go version to 1.25
- Switched to custom `libhashtree` bindings to avoid misleading CGO build warning

### Breaking Changes
- **Stricter unmarshal validation** — previously accepted invalid booleans (>1), unterminated bitlists, and bitvectors with set padding bits now return errors. Code relying on lenient parsing may break.

---

## [v1.1.1] — 2025-10-18

### Added
- Comprehensive unit tests for `TypeCache`, codegen, `treeproof`, `hasher`, `CompatibleUnion`, `TypeWrapper`
- `nohashtree` build tag to exclude `OffchainLabs/hashtree` CGO dependency

### Fixed
- Version header in generated files
- Codegen: first offset check made more strict
- Codegen: pointer type resolution for cache keys
- Codegen: HTR method without dynamic expressions
- Codegen: limit checks in HTR generated code
- Codegen: `CompatibleUnion` generation
- CGO-less builds (avoid `hashtree` dependency without CGO)
- Named pointer type resolution

### Changed
- Generalized default hasher pool usage in codegen
- Improved marshal & HTR codegen to avoid temporary allocations
- Offloaded common slice expansion logic to `sszutils.ExpandSlice`
- Pointer optimizations in generated code
- Reordered generated code for readability
- Added `-package-name` flag to `dynssz-gen` CLI
- Simplified project structure

### Breaking Changes
None. Generated code from v1.1.0 should be regenerated for correctness fixes.

---

## [v1.1.0] — 2025-09-28 (prerelease)

### Added
- **Code generator** (`codegen` package) — compile-time SSZ method generation as alternative to reflection
- **`dynssz-gen` CLI tool** — command-line code generator using `go/packages`
- `DynamicMarshaler`, `DynamicUnmarshaler`, `DynamicSizer`, `DynamicHashRoot` interfaces for generated code
- **Merkle tree proofs** (`treeproof` package) — tree construction, single/multi proof generation and verification
- `DynSsz.GetTree()` method for building complete Merkle trees
- `CompatibleUnion` variant mixin in tree root calculation
- `time.Time` support (serialized as uint64)
- `nohashtree` build tag groundwork
- CodeGenerator options: `WithNoMarshalSSZ`, `WithNoUnmarshalSSZ`, `WithNoSizeSSZ`, `WithNoHashTreeRoot`, `WithCreateLegacyFn`, `WithoutDynamicExpressions`, `WithNoFastSsz`, `WithReflectType`, `WithGoTypesType`, size/max/type hint options
- Release workflow for automated builds

### Changed
- Reimplemented code generator with flat code style (removed recursive style)
- Improved codegen formatting for `go fmt` compatibility
- Added codegen header with hash and version

### Breaking Changes
- **New interfaces** — `DynamicMarshaler`, `DynamicUnmarshaler`, `DynamicSizer`, `DynamicHashRoot` added to the public API surface. Types implementing these are preferred over reflection.

---

## [v1.0.2] — 2025-09-02

### Added
- `TypeWrapper[D, T]` generic type for wrapping non-struct top-level SSZ types with tag annotations
- `ValidateType()` method on `DynSsz`
- Strict SSZ typing via `ssz-type` struct tag
- Progressive type support (`ProgressiveContainer`, `ProgressiveList` & `ProgressiveBitList`)
- `CompatibleUnion` support for SSZ unions
- Multi-dimensional slice/array support (marshal, unmarshal, hash tree root)
- `holiman/uint256.Int` auto-detection as uint256
- String support as progressive list
- Offset slice pool (`GetOffsetSlice`/`PutOffsetSlice`) to reduce allocations
- Static size specification for custom SSZ types

### Fixed
- Hash tree root calculation for multi-dimensional slices
- Panic when hashing lists exceeding the limit
- `HasDynamicSize` and `HasDynamicMax` for string descriptors
- Byte slice/array allocation performance
- Unmarshal performance for byte slices and arrays

### Changed
- Switched low-level hasher to `OffchainLabs/hashtree` (significant performance improvement)
- Bumped Go version requirement in CI
- Made `TypeDescriptor` more compact
- Cached `HashTreeRootWith` method for call performance
- Removed `remerkleable` dependency
- Improved error handling and test coverage

### Breaking Changes
- **Hasher backend changed** to `OffchainLabs/hashtree` (requires CGO by default). Use build tag `nohashtree` for pure-Go fallback.
- **`TypeDescriptor` restructured** — fields reorganized for compactness. Code accessing `TypeDescriptor` fields directly may need updates.
- **`ssz-type` tag introduced** — changes how types are resolved. Existing tags remain compatible.

---

## [v1.0.1] — 2025-08-06

### Changed
- Switched `govaluate` dependency to maintained fork (`Knetic/govaluate` → maintained alternative)

### Breaking Changes
None.

---

## [v1.0.0] — 2025-06-25

### Added
- Consensus spec test validation (static SSZ samples from `ethereum/consensus-spec-tests`)
- Performance optimization via `HashTreeRootWith` for fastssz types
- `gohashtree` integration for faster hashing
- Benchmark test suite
- Documentation and examples
- CI workflows for testing

### Changed
- Refactored hash tree root calculation
- Refactored type cache to minimize reflection overhead
- Fixed `ssz-max` overflow handling
- Improved slice creation in unmarshaler
- Removed unused code

### Breaking Changes
- **v1.0.0 stable release** — API considered stable from this point. Prior v0.x releases had no stability guarantees.

---

## [v0.0.6] — 2025-02-20

### Added
- Hash tree root calculation (`HashTreeRoot`, `HashTreeRootWith`)
- `hasher` package — SSZ Merkle hasher (ported from fastssz, removed fastssz dependency)

### Breaking Changes
- **`fastssz` dependency removed** — hasher implementation is now internal.

---

## [v0.0.5] — 2024-08-05

### Fixed
- Panic on concurrent use (concurrent map writes in type cache)
- FastSSZ compatibility check extended to all types (not just structs)

### Breaking Changes
None.

---

## [v0.0.4] — 2024-05-14

### Fixed
- Bitvector rounding issue — sizes not a multiple of 8 now correctly round up (can't serialize partial bytes)

### Breaking Changes
None.

---

## [v0.0.3] — 2024-05-03

### Added
- Unmarshal tests and marshal tests
- Offset validation in dynamic slice unmarshaling

### Fixed
- Size calculation errors
- Marshaling of nil pointers

### Changed
- License changed to Apache-2.0
- Removed direct `fastssz` dependency
- Refactored fastssz compatibility check

### Breaking Changes
- **License changed** from unspecified to Apache-2.0.

---

## [v0.0.2] — 2024-04-01

### Added
- Dynamic expression parser (`govaluate` integration) for evaluating spec values in struct tags (e.g., `ssz-size:"MAX_VALIDATORS_PER_COMMITTEE"`)

### Fixed
- `govaluate` import path

### Breaking Changes
None.

---

## [v0.0.1] — 2024-03-31

### Added
- Initial release — prototype ported from [go-eth2-client PR #123](https://github.com/attestantio/go-eth2-client/pull/123)
- `DynSsz` type with `MarshalSSZ`, `UnmarshalSSZ`, `SizeSSZ`
- Dynamic spec value resolution via `map[string]any`
- FastSSZ compatibility (delegates to `SizeSSZ` when no dynamic specs)
- Struct tag support: `ssz-size`, `ssz-max`, `dynssz-size`, `dynssz-max`
- Size caching for performance
- Basic test and example code

### Breaking Changes
N/A (initial release).
