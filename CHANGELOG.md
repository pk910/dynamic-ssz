# Changelog

All notable changes to the `dynamic-ssz` library are documented here.

## [v1.4.0] 2026-10-08 hardening & async hashing

This is a hardening release. The reflection engine, the code generator, the hasher and `treeproof` went through many rounds of differential fuzzing, external audits and conformance testing. Both engines now produce the same bytes, roots, trees and errors for the same schema, bad input returns an error instead of panicking or allocating without bound, and the code generator is no longer in beta.

### Breaking Changes
- `CompatibleUnion` selectors now follow EIP-8016. Selectors are 1-based (`1..127`), so the first untagged variant is written as `0x01` instead of `0x00`. Schemas and values that use 0-based selectors need to be migrated.
- The hash tree root of `ssz-type:"optional"` fields changed (extended types only). Presence is now mixed in as a length, so an absent value and a present zero value no longer have the same root. The root is now the same as for `optional-list`. Serialization is unchanged. Generated code keeps the old root until it is regenerated.
- Validation is stricter. Some malformed input and schemas that older versions accepted are now rejected with an error. This includes list counts that do not fit the region, non-zero padding bits in bitvectors, trailing or missing bytes, hash tree roots of lists without a limit, conflicting size or type tags, zero-field containers with their own SSZ methods (use `ssz-type:"custom"` for these) and spec-only bounds in code generated without dynamic expressions.

### Added
- Classic SSZ `Union[T]` with 0-based selectors and `dynssz.None`, next to the EIP-8016 `CompatibleUnion[T]`.
- Async hashing. Optional background hashing of large subtrees, enabled with `WithAsyncHashing(workers)` or `hasher.EnableAsyncHashing`.
- Recursive types, as long as every cycle goes through a variable-length field. Nesting depth is limited (default 1024) and can be set with `WithMaxNestingDepth`, `codegen.WithRecursionDepth` or `dynssz-gen -recursion-depth`.
- Batched tree finalization. `GetTree` now builds a finalized read-only tree through the hashtree backend. New `Node.Finalize`, `treeproof.WithHashFn` and `treeproof.NewWrapperWithHashFn`.
- Unknown-size stream decoding. `UnmarshalSSZReader` with a negative size reads until EOF and only allocates for bytes that actually arrived. Generated code needs to be regenerated for this.
- Limits for untrusted input: `WithMaxStreamSize`, the per-call `WithStreamSizeLimit` and `WithMaxNestingDepth`.
- `ssz-type:"-"` to skip a struct field, and `WithNoDelegation` to ignore the generated methods of a type.
- `dynssz-gen` options: custom header templates, per-type `skip-*` settings in the config file, `-recursion-depth` and `-remove`.
- New error values (`ErrStreamTooLarge`, `ErrSszSizeExceeded`, `ErrMaxDepthExceeded` and more), `HashErr()` on `Hasher` and `treeproof.Wrapper`, and `Reset` on all encoders and decoders.
- Big-endian (s390x) and 32-bit support, tested in CI. New examples (`chain-specs`, `streaming`, `merkle-proofs`, `fork-views`, `htr-caching`) and a docs page about method delegation.

### Changed
- Performance: generated code resolves the spec expressions of a type once per `DynSsz` instance instead of on every call (regenerate to get this). Encoders and decoders are pooled per instance. The reflection engine sizes byte lists by their length, decodes byte slices without boxing and handles `uint64` and byte-array elements as one block.
- All size, offset and limit math is done in unsigned 64-bit with explicit range checks, so both engines are safe on 32-bit targets.
- Spec expressions are evaluated as exact rationals and rounded up once. The `govaluate` dependency is gone. A `name:N` fallback syntax was added.
- Both engines use the same rules for when the fastssz, `*Dyn` or view methods of a type are called. Promoted methods from embedded fields are no longer used, and fastssz is only used when no resolved spec value differs from the static tags. `sszutils.Annotate` now works across packages and on views, type wrappers and union variants.
- Conformance tests: the consensus-spec vectors (including Gloas) also run as a 32-bit build, and a new test runs every vector of the official `ethereum/ssz-specs` suite on both engines. The differential fuzzer got an independent reference SSZ implementation and extra checks for trees, proofs, native hashing, sizes and streaming.
- The code generator is now considered production ready. `hashtree-bindings` bumped to 0.2.6.

### Fixed
- Panics and unbounded allocations on bad input (oversized declared lists, huge `bitsize` values, stream over-reads, offset overflows, recursive payloads).
- Cases where reflection and generated code framed, sized or hashed the same schema differently: views of view-serving types, custom types with spec-driven widths, overridden type hints, cross-package `Annotate`, packed large uints, union variants with spec-only widths and code generated without dynamic expressions.
- Hash tree roots of packed scopes with delegated elements, `Index`-opened scopes, progressive lists under async hashing, partial chunks, short `uint128` and `uint256` values, optionals and lists of type wrappers. `HashTreeRoot` and `GetTree` now agree and return backend errors.
- Streaming: the streaming encoder of a static build wrote correct data behind wrong offsets for children with spec values. Unions, optionals, custom types and views are now framed the same way on buffer and reader paths.
- `treeproof`: empty leaves from `ProveMulti`, caller memory being aliased, zero-padding gindices that could not be proven, 32-bit leaf overflow and an unbounded multiproof cache.
- Zero-padding no longer writes into caller memory, `float32` NaN bits are kept as is, `NewDynSsz` copies its spec map and `RemoveType` removes both the value and the pointer form of a type.

---

## [v1.3.3] 2026-08-26

### Fixed
- Fix `treeproof.Wrapper` building a non-canonical Merkle tree for containers that interleave buffered `Append*` fields with directly added `Put*`/`AddNode` fields - pending bytes are now flushed to leaf nodes before a node is added or a scope checkpoint is captured, so the leaf order matches `hasher.Hasher`
- Fix `GetTree(x).Hash()` diverging from `HashTreeRoot(x)`, and proofs derived from the tree failing to verify, for Deneb and later `BeaconState`, `BeaconBlock`, `SignedBeaconBlock`, `ExecutionPayload` and `ExecutionPayloadHeader`; this affects every release since `GetTree` was introduced in v1.1.1, while `HashTreeRoot`, `MarshalSSZ` and `UnmarshalSSZ` are unaffected

---

## [v1.3.2] 2026-06-21

### Added
- Add support for `ssz-type:"optional-list"` to encode Go pointer fields as canonical SSZ `List[T, 1]`

### Changed
- Performance improvements in codegen marshal/unmarshal and hash-tree-root operations
- Bump `github.com/pk910/hashtree-bindings` dependency to 0.2.2 for optimized riscv support
- Update CI actions including codeql-action, checkout, and codecov-action

### Fixed
- Fix hash-tree-root mismatch for progressive bitlists with all-zero top chunks
- Fix streaming reflection unmarshal panic on malformed nested lists
- Fix various marshal/HTR/stream inconsistencies found by differential fuzzing
- Fix unmarshaling empty lists to an empty slice
- Fix type tree traversal for fully delegated types

---

## [v1.3.1] 2026-04-22

### Added
- Add support for custom struct types as type-wrapper with `ssz-type:"wrapper"`, removing the need for a descriptor struct and `GetDescriptorType` method
- Add YAML config file support to `dynssz-gen` with `--config` flag for easier code generation management

### Changed
- Optimize treeproof multiproof verification with a faster path for indices on the same tree level, reducing allocations and improving performance

---

## [v1.3.0] 2026-03-31

### Added
- **SSZ Views** — custom view types that decouple the SSZ schema from the Go struct layout, allowing different field orders and custom type mappings. Views are supported in both reflection and codegen paths, including views from external packages.
- **Incremental hasher** — new hashing strategy for lists that processes elements in chunks rather than buffering all leaves in memory, significantly reducing memory consumption for large lists. Requires code regeneration to take effect; existing generated code continues to work with the previous hashing approach.
- **`SszError` wrapper** — structured error type with consistent error messages across reflection and codegen code paths
- **Error path tracking** in generated code — errors include the field path for easier debugging
- **Custom type annotations** via `sszutils.Annotate` — programmatic SSZ tag overrides for types that cannot use struct tags
- **Backward compatibility tests** — CI validates that code generated by older releases still compiles and works correctly
- **Internal write buffer** for marshal-to-stream, reducing syscall overhead
- **Function header documentation** in generated code
- Code scanning (CodeQL) and OpenSSF best practices

### Changed
- **Reflection performance improvements** — fast path for static container marshal/unmarshal, bulk allocations to avoid descriptor copies, hoisted pointer checks, early skip of compat checks for primitive types
- **Codegen performance improvements** — reduced string builder and import overhead, batched offset insertion in marshaler, `HashUint64Slice` for uint64 arrays, reduced `Collapse()` calls to every 256th item
- **Treeproof optimizations** — refactored package, optimized tree construction with empty-node caching, faster proof verification, multiproof fast path for full-tree proofs, fixed mixed-depth multiproof indices
- **Normalized error messages** across codegen and reflection code paths
- **Consistent list length limit checks** in all marshal and unmarshal code paths
- Downgraded minimum Go version for the base library to 1.22
- Updated `go-hashtree` bindings and `go-bitfield` dependency
- Always generate streaming code with dynamic expressions enabled
- Always generate code for pointer types (lists & arrays)
- Improved test coverage across codegen, treeproof, hasher, and reflection

### Fixed
- String pointer unmarshaling
- Bitlist limit checks
- `ExpandSlice` reset and lazy typecache index setup
- Integer overflow conversions flagged by CodeQL (32-bit compatibility)
- Bitlist allocation in hash tree root calculation
- Conflicting annotation resolution (struct annotations take precedence over custom type annotations)
- Streaming codegen when dynamic expressions are disabled

---

## [v1.2.2] 2026-03-12

### Added
- Fuzz testing framework with parallel workers and multi-dimensional list/vector support
- Extended type support (`int8`/`int16`/`int32`/`int64`, `float32`/`float64`, `bigint`, `optional`) for reflection and code generation
- Fuzzing CI workflow
- Smoke tests for fuzzer in CI
- Codecov exclusion for fuzzer code

### Changed
- Refactored `getZeroOrderHashes` for clarity

### Fixed
- Concurrent map read/write panic in `specValueCache` (added mutex synchronization)
- Minor code generation formatting issues

---

## [v1.2.1] 2026-01-21

### Added
- Progressive tree shape change implementation (EIP-7916 compatibility)

### Changed
- Use `for range` syntax in generated code

### Fixed
- Code generation for primitive pointer types
- Unused `encoding/binary` import in generated code
- Codegen indentation and spacing (now `go fmt` compatible)

---

## [v1.2.0] 2026-01-02 Streaming support

### Breaking Changes
- **`sszutils` package extracted** — interfaces and utilities previously in the root package moved to `sszutils`. Import paths for `Encoder`, `Decoder`, `HashWalker`, `DynamicSpecs`, and related types must be updated.
- **`CanSeek` renamed to `Seekable`** on `Encoder` and `Decoder` interfaces. Implementations of these interfaces must update the method name.

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

### Changed
- Inlined primitive encoding/decoding in generated code for performance
- Deduplicated dynamic expression evaluation in codegen
- Improved test coverage significantly

### Fixed
- Hash tree root calculation for dynamic byte slices with >32 bytes in generated code
- Heap allocation in bitlist HTR calculation
- Various tree proof optimizations

---

## [v1.1.2] 2025-12-08

### Breaking Changes
- **Stricter unmarshal validation** — previously accepted invalid booleans (>1), unterminated bitlists, and bitvectors with set padding bits now return errors. Code relying on lenient parsing may break.

### Added
- `ssz-bitsize` / `dynssz-bitsize` struct tags for bitvector types with padding bit validation
- Sentinel bit validation for `Bitlist` types during unmarshal
- Boolean value validation during unmarshal (rejects values other than 0 and 1)

### Changed
- Bumped Go version to 1.25
- Switched to custom `libhashtree` bindings to avoid misleading CGO build warning

---

## [v1.1.1] 2025-10-18

### Added
- Comprehensive unit tests for `TypeCache`, codegen, `treeproof`, `hasher`, `CompatibleUnion`, `TypeWrapper`
- `nohashtree` build tag to exclude `OffchainLabs/hashtree` CGO dependency

### Changed
- Generalized default hasher pool usage in codegen
- Improved marshal & HTR codegen to avoid temporary allocations
- Offloaded common slice expansion logic to `sszutils.ExpandSlice`
- Pointer optimizations in generated code
- Reordered generated code for readability
- Added `-package-name` flag to `dynssz-gen` CLI
- Simplified project structure

### Fixed
- Version header in generated files
- Codegen: first offset check made more strict
- Codegen: pointer type resolution for cache keys
- Codegen: HTR method without dynamic expressions
- Codegen: limit checks in HTR generated code
- Codegen: `CompatibleUnion` generation
- CGO-less builds (avoid `hashtree` dependency without CGO)
- Named pointer type resolution

---

## [v1.1.0] 2025-09-28 Code generator

### Added
- **Code generator** (`codegen` package) — compile-time SSZ method generation as alternative to reflection
- **`dynssz-gen` CLI tool** — command-line code generator using `go/packages`
- `DynamicMarshaler`, `DynamicUnmarshaler`, `DynamicSizer`, `DynamicHashRoot` interfaces for generated code
- **Merkle tree proofs** (`treeproof` package) — tree construction, single/multi proof generation and verification
- `DynSsz.GetTree()` method for building complete Merkle trees
- `CompatibleUnion` variant mixin in tree root calculation
- `time.Time` support (serialized as uint64)
- CodeGenerator options: `WithNoMarshalSSZ`, `WithNoUnmarshalSSZ`, `WithNoSizeSSZ`, `WithNoHashTreeRoot`, `WithCreateLegacyFn`, `WithoutDynamicExpressions`, `WithNoFastSsz`, `WithReflectType`, `WithGoTypesType`, size/max/type hint options
- Release workflow for automated builds

### Changed
- Reimplemented code generator with flat code style (removed recursive style)
- Improved codegen formatting for `go fmt` compatibility
- Added codegen header with hash and version

---

## [v1.0.2] 2025-09-02

### Breaking Changes
- **Hasher backend changed** to `OffchainLabs/hashtree` which requires CGO by default. Use build tag `nohashtree` for pure-Go fallback.
- **`TypeDescriptor` restructured** — fields reorganized for compactness. Code accessing `TypeDescriptor` fields directly may need updates.

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

### Changed
- Bumped Go version requirement in CI
- Made `TypeDescriptor` more compact
- Cached `HashTreeRootWith` method for call performance
- Removed `remerkleable` dependency
- Improved error handling and test coverage

### Fixed
- Hash tree root calculation for multi-dimensional slices
- Panic when hashing lists exceeding the limit
- `HasDynamicSize` and `HasDynamicMax` for string descriptors
- Byte slice/array allocation performance
- Unmarshal performance for byte slices and arrays

---

## [v1.0.1] 2025-08-06

### Changed
- Switched `govaluate` dependency to maintained fork

---

## [v1.0.0] 2025-06-25

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
- Improved slice creation in unmarshaler
- Removed unused code

### Fixed
- `ssz-max` overflow handling

---

## [v0.0.6] 2025-02-20

### Added
- Hash tree root calculation (`HashTreeRoot`, `HashTreeRootWith`)
- `hasher` package — SSZ Merkle hasher (ported from fastssz, removed fastssz dependency)

---

## [v0.0.5] 2024-08-05

### Fixed
- Panic on concurrent use (concurrent map writes in type cache)
- FastSSZ compatibility check extended to all types (not just structs)

---

## [v0.0.4] 2024-05-14

### Fixed
- Bitvector rounding issue — sizes not a multiple of 8 now correctly round up

---

## [v0.0.3] 2024-05-03

### Added
- Unmarshal tests and marshal tests
- Offset validation in dynamic slice unmarshaling

### Changed
- License changed to Apache-2.0
- Removed direct `fastssz` dependency
- Refactored fastssz compatibility check

### Fixed
- Size calculation errors
- Marshaling of nil pointers

---

## [v0.0.2] 2024-04-01

### Added
- Dynamic expression parser (`govaluate` integration) for evaluating spec values in struct tags

### Fixed
- `govaluate` import path

---

## [v0.0.1] 2024-03-31

### Added
- Initial release — prototype ported from [go-eth2-client PR #123](https://github.com/attestantio/go-eth2-client/pull/123)
- `DynSsz` type with `MarshalSSZ`, `UnmarshalSSZ`, `SizeSSZ`
- Dynamic spec value resolution via `map[string]any`
- FastSSZ compatibility (delegates to `SizeSSZ` when no dynamic specs)
- Struct tag support: `ssz-size`, `ssz-max`, `dynssz-size`, `dynssz-max`
- Size caching for performance
- Basic test and example code
