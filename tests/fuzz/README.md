# Differential fuzzer

The fuzzer generates a fresh Go type corpus, generates SSZ methods for it, and
then compares reflection, code generation, buffer, streaming, known-length,
and unknown-length paths over valid, mutated, and random inputs.

Valid values also run an oracle battery by default:

- a small independent SSZ encoder and hash-tree-root implementation checks the
  classic, single-dimensional subset without calling dynamic-ssz internals;
- native SHA-256 and accelerated hashing must agree;
- `SizeSSZ` must equal the marshaled length;
- `HashTreeRootWith` and repeated root calculations must agree.

The status line prints reference coverage as `ref: checked/eligible`. Skipped
schemas are visible rather than being counted as successful oracle checks. The
independent oracle deliberately skips progressive types, unions, wrappers,
multi-dimensional collections, dynamic spec expressions, and extended types;
the official `ethereum/ssz-specs` suite in `tests/sszspecs` supplies the external
oracle for progressive containers/lists/bitlists and compatible unions.

The generated corpus now includes both classic `Union` and `CompatibleUnion`
(including explicit sparse selectors), plus progressive lists of composite
elements.

```bash
make fuzz DURATION=60s NUM_TYPES=100 MAX_DEPTH=4

# Differential paths only, or keep invariants but disable the reference model:
go run ./cmd/fuzz -oracles=false
go run ./cmd/fuzz -reference=false
```
