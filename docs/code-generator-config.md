# Code Generator Config File

`dynssz-gen` accepts a YAML config file via `--config <file>`. This is an
alternative to the CLI flag form that scales better once you have more than a
handful of types — especially when different types need different output
files, view types, or codegen flag overrides.

```bash
dynssz-gen --config gen.yaml
```

## When to use

Use a config file when any of the following apply:

- The `-types` string is getting long and hard to diff (colon-separated
  per-type options are hard to read).
- Different types need different codegen flags (e.g. `-legacy` only for some
  types).
- You want the generation settings checked into the repo as a first-class
  artifact rather than embedded in a `//go:generate` directive.

For a single-type one-liner the CLI form remains shorter and is still
supported.

## Full example

```yaml
# Target Go package (required)
package: github.com/myproject/beacon/types

# Optional overrides
package-name: types           # defaults to the source package name
output: generated_ssz.go      # default output for types that don't specify their own
verbose: false

# Code-generation flags (defaults — applied to every type unless overridden)
legacy: false
without-dynamic-expressions: false
without-fastssz: false
with-streaming: false
with-extended-types: false

# Types to generate (at least one required)
types:
  # Shorthand: just a name. Inherits the top-level output and flags.
  - BeaconBlockHeader

  # Full form. Any omitted field inherits the top-level value.
  - name: BeaconBlock
    output: block_ssz.go
    views:
      - Phase0BlockView
      - AltairBlockView
      - github.com/myproject/views.BellatrixBlockView

  # View-only: generates only the view methods (no base methods).
  - name: BeaconState
    output: state_ssz.go
    view-only: true
    views:
      - Phase0StateView

  # Per-type flag override: this type is generated without streaming methods
  # even though with-streaming is true globally.
  - name: Validator
    output: validator_ssz.go
    with-streaming: false
```

## Field reference

### Top-level fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `package` | string | yes | Go import path of the package to analyze. |
| `package-name` | string | no | Overrides the package name emitted into generated files. |
| `output` | string | conditional | Default output path. Required if any type entry omits `output`. |
| `verbose` | bool | no | Verbose logging during generation. |
| `legacy` | bool | no | Generate legacy (non-`*Dyn`) fastssz-compatible methods. |
| `without-dynamic-expressions` | bool | no | Emit only static legacy methods (no `*Dyn`). |
| `without-fastssz` | bool | no | Don't call third-party fastssz methods on referenced types. |
| `with-streaming` | bool | no | Emit streaming encoder/decoder methods. |
| `with-extended-types` | bool | no | Allow extended types (signed ints, floats, big.Int, optionals). |
| `types` | list | yes | At least one type entry (shorthand string or mapping). |

### Type entries

A `types:` list item is either **shorthand** (a bare string — the type name)
or **full form** (a mapping).

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Required in full form. Type name within the target package. |
| `output` | string | Output file override. Falls back to the top-level `output`. |
| `views` | list of strings | View types for view-aware generation. Local types are bare names; cross-package views use `pkg/path.TypeName`. |
| `view-only` | bool | Generate only view methods, not the base methods. |
| `legacy` | bool | Per-type override for the top-level `legacy`. |
| `without-dynamic-expressions` | bool | Per-type override. |
| `without-fastssz` | bool | Per-type override. |
| `with-streaming` | bool | Per-type override. |
| `with-extended-types` | bool | Per-type override. |

Per-type boolean overrides can flip a flag in either direction (e.g. turn off
`legacy` for one type while it is on globally, or vice versa).

## Path resolution

All relative `output` paths — both top-level and per-type — are resolved
**relative to the config file's directory**, not the current working
directory. Absolute paths are used as-is.

This means `dynssz-gen --config ./codegen/tests/gen.yaml` works identically
whether you run it from the repo root, from `codegen/`, or from anywhere
else.

## Combining with CLI flags

The config file is the baseline. Any CLI flag **explicitly passed** overrides
the corresponding config value. Flags that are *not* passed inherit the
config value.

Detection is based on whether the flag appears on the command line at all —
so `--config gen.yaml -legacy=false` overrides the config even if the config
had `legacy: true`, but `--config gen.yaml` alone leaves it untouched.

Special cases:

- `-types` on the CLI **fully replaces** the `types:` list from the config.
  This is deliberate: merging the two is confusing and surprising. If you
  need to add a one-off type ad hoc, use the CLI form directly.
- `-output` on the CLI replaces the top-level `output:`. It does *not*
  rewrite per-type `output` entries — those remain whatever the config
  specified.

## Validation

The parser is strict about unknown keys. A typo like `view_only` instead of
`view-only` is an error, not a silent skip:

```
failed to parse config file gen.yaml: line 8: unknown field "view_only" in type entry
```

At least one entry is required under `types:`; every entry must have a
`name`; and every entry must resolve to a non-empty output path (either its
own or the top-level default).

## Examples from this repo

The `codegen/tests/` directory uses config files for every generation batch.
See `codegen/tests/gen_ssz.yaml` for a realistic multi-file, multi-view
setup, and `codegen/tests/gen_nodynexpr.yaml` for a minimal static-only
example.

The generator is driven by `//go:generate` lines in
`codegen/tests/generate.go` that simply reference each YAML file.

## Related documentation

- [Code Generator](code-generator.md) — full CLI reference and codegen
  concepts.
- [SSZ Views](views.md) — view-aware generation.
- [SSZ Annotations](ssz-annotations.md) — the tags that the generator
  respects.
