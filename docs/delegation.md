# Method Delegation

Both engines reach a nested type that has SSZ methods of its own through those
methods instead of walking its fields. This page states which method is called
from which context, in the reflection engine at run time and in generated code
at generation time. The two engines apply the same rules.

## The surfaces a type can expose

| Surface | Methods | Takes specs | Produced by |
|---|---|---|---|
| static | `MarshalSSZ`, `MarshalSSZTo`, `UnmarshalSSZ`, `SizeSSZ`, `HashTreeRoot`, `HashTreeRootWith` | no | fastssz, `dynssz-gen -legacy`, `dynssz-gen -without-dynamic-expressions`, hand-written code |
| dynamic | `MarshalSSZDyn`, `UnmarshalSSZDyn`, `SizeSSZDyn`, `HashTreeRootWithDyn` | yes | `dynssz-gen` default, hand-written code |
| streaming | `MarshalSSZEncoder`, `UnmarshalSSZDecoder` | yes | `dynssz-gen -with-streaming`, hand-written code |
| view | `MarshalSSZDynView`, `UnmarshalSSZDynView`, `SizeSSZDynView`, `HashTreeRootWithDynView`, and the streaming view pair | yes | `dynssz-gen` with views |

A type may expose several surfaces at once. A default generation of a
spec-free type also emits static method bodies, but only `-legacy` and
`-without-dynamic-expressions` register the static surface for delegation; a
default generation registers the dynamic surface, and the streaming pair with
`-with-streaming`.

## The handlers that delegate

| Handler | Where | Specs available |
|---|---|---|
| dynamic buffer | `ds.MarshalSSZ`, `ds.UnmarshalSSZ`, `ds.SizeSSZ`, `ds.HashTreeRoot` in the reflection engine; generated `*Dyn` methods | yes |
| streaming | `ds.MarshalSSZWriter`, `ds.UnmarshalSSZReader`; generated `MarshalSSZEncoder`, `UnmarshalSSZDecoder` | yes |
| static | generated `MarshalSSZTo`, `UnmarshalSSZ`, `SizeSSZ`, `HashTreeRootWith` of a `-without-dynamic-expressions` build | no |
| view | generated view methods | yes |

## When the static surface is used

The static surface is taken whenever the analysis phase admits it: fastssz
delegation is on (`WithNoFastSsz` not set) and no size or limit below the
child depends on the spec. Under that condition the static method produces
the same bytes and root as the dynamic one and skips the spec argument, so it
is preferred on every path. A static method baked its tags into its output and
enforces them when it encodes, decodes and hashes, so a child whose size or
limit depends on the spec is reached through a spec-aware surface, with the
limitation described below. The two engines decide this
differently: the reflection engine resolves the spec values and only counts
one that differs from the static tag (`HasDynamicSize`, `HasDynamicMax`),
while generated code, which sees no spec values, counts every expression
(`HasSizeExpr`, `HasMaxExpr`).

**Known limitation.** A type generated with `-without-dynamic-expressions`
must not contain a type that is generated without that flag and uses spec
expressions (`dynssz-size`, `dynssz-max`, `dynssz-bitsize`). The outer type is
then always served by its static methods, which use the static tag values for
the inner type even when the spec values differ. The same applies to a
hand-written type with fastssz-style methods only.

Example: `Outer` is generated with `-without-dynamic-expressions` and holds
`Inner`, generated without it, where `Inner` has a list tagged
`ssz-max:"16" dynssz-max:"MAX_ITEMS"`. With `MAX_ITEMS` set to 4, `Outer`
still encodes, decodes and hashes with a limit of 16.

These combinations work with any spec values:

| Outer type | Inner type | Result |
|---|---|---|
| with the flag | with the flag | correct |
| without the flag | with the flag | correct |
| without the flag | without the flag | correct |
| with the flag | without the flag, uses spec expressions | static tag values are used |

## The order per handler

| Handler | Order |
|---|---|
| dynamic buffer marshal and unmarshal | static, then dynamic, then streaming through a buffer encoder or decoder |
| dynamic buffer size and hash | dynamic, then static, in generated code; static, then dynamic, in the reflection engine |
| streaming marshal and unmarshal | static through the buffer, then streaming, then dynamic through the buffer |
| static buffer methods | static, otherwise the child is inlined |
| static build's streaming methods | static through the buffer, then streaming, otherwise the child is inlined |
| view | the child's view method; a missing one is `ErrNotImplemented` in generated code and a reflection walk of the view schema in the engine |

"Through the buffer" means the child's whole region is read from the stream
into the decoder buffer, or written to the encoder's scratch buffer, and the
child's buffer method runs on it.

A view of a child that serves views is reached through the child's view
surface without walking the view schema, on both engines. Whether such a view
is fixed-size is the view type's own `ssz-static` declaration, not the data
type's: a static view is framed inline and sized by the child's view sizer, a
dynamic one is placed behind an offset.

A generated variable-size type declares, beside `ssz-static:"false"`, the
bytes every value holds in its fixed section as its code frames it:
`ssz-minsize` with the floor when no spec value is defined, and
`dynssz-minsize` with the expression the spec resolves to it, resolved with
`ssz-minsize` as its fallback, as `dynssz-size` is resolved with `ssz-size`.
In a generated declaration every spec-decided part carries the `:fallback`
the type's own code resolves it with, such as
`(SPEC_A):32+(SPEC_B):4*8+(SPEC_BITS/8):3+4`, so the parts resolve on their
own however the spec defines them (a static build declares the literal it
baked). A fixed-size type declares its size the same way, for the floors of
the types that hold it. A type that holds a child described without its
subtree imports the child's declaration into its own. A hand-written type
may declare its floor the same way.
Generated code decoding a list of such children reads that declaration and
refuses an offset table whose element count the region cannot hold, before
the count sizes an allocation, with the same region gate every other list
has. A child without the declaration, from an older generation or written by
hand, bounds nothing there; regenerate a child's package before the packages
that list it.

The reflection engine reads the same declaration when it describes such a
child, resolved against its own specs, so a reflection-decoded container
refuses the same offset table with the same error.

A static build reaching a child that has no static surface inlines the child's
structure. A child with no traversable structure, a custom type or an
external fully-delegated type built without its subtree, cannot be inlined and
fails generation with a message naming the missing static method.

## Custom types

A `ssz-type:"custom"` value always delegates: it has no structure to walk.
What it does with a spec value cannot be seen either, so its methods say it:

- A custom type that carries a spec-aware method (dynamic or streaming) takes
  the spec set and counts as depending on the spec. It is reached through its
  spec-aware methods, and every type that holds it is treated like a type with
  a spec expression below it: none of them is served by a static method.
- A custom type with static methods only cannot depend on the spec and is
  reached through those.

A build without dynamic expressions is the exception for its buffer methods:
they take no spec set, so they reach a custom type through its static surface,
also when it carries both, and fail when it has none. Its streaming methods
carry the spec set and follow the rule above.

A custom type is static with a literal `ssz-size`, with a spec expression, or
when its annotation declares `ssz-static:"true"`. A width that is not a
literal is read from the type's sizer on a zero value: by the reflection
engine when it describes the type, by generated code where it needs the
width. A static type's size does not depend on its value, so its sizer must
not either. Such a width never packs.

Being opaque also exempts it from the rule that a method promoted from an
embedded field never stands for the outer type. A custom type whose surface is
entirely promoted delegates to it, so the embedded value is what gets encoded
and hashed and the outer type's other fields do not appear.

## Options that change the rule

- `WithNoFastSsz` removes the static surface from consideration for every
  non-custom child. Such a child is reached through its dynamic or streaming
  surface, or inlined when it has neither.
- `WithNoDelegation` removes the dynamic, streaming and view surfaces for
  every non-custom child, so the reflection engine walks generated types too.
  A child with the complete dynamic surface is walked whether or not it also
  has static methods: a `-legacy` generation's static methods are wrappers over
  the global instance's specs, and only the method set tells them from real
  static bodies. The static surface of a child without a dynamic surface stays
  governed by `WithNoFastSsz`. This is how the differential tests compare
  generated code against the reflection walk.
- `-without-dynamic-expressions` produces static handlers, which follow the
  static rows above.

## Wrappers of a generated type

A default generation of a type with spec expressions emits static wrappers
only with `-legacy`; each forwards to the type's own dynamic method with the
global instance's specs. A spec-free generation emits real static bodies.
The reflection engine does not reach a wrapper from an instance of its own
when it delegates: a type with the complete dynamic surface is served by that
surface. Under `WithNoDelegation` such a type is walked, so the global
instance's specs decide only for callers of the wrappers themselves.
The `ds.*` entry points are the supported way in; calling a child's generated
method directly bypasses the rule and the recursion bound.

## What is checked, and what is trusted

A delegate's output is trusted. The engines call the method and use what comes
back; checking every delegate on every call would cost more than delegating
saves.

Two things are enforced, because the entry points have already paid for them:

| Check | Where |
|---|---|
| a delegated size is neither negative nor past the SSZ size limit | every `ds.*` entry point that sizes |
| the encoded length equals the size the sizing pass computed | `ds.MarshalSSZ`, `ds.MarshalSSZTo`, `ds.MarshalSSZWriter` |

The rest is the delegate's contract:

- `MarshalSSZTo` and `MarshalSSZDyn` append to the buffer they are given and
  return it. One that returns a buffer of its own moves the encoder back, and
  the offset write that follows panics.
- `SizeSSZ` and `SizeSSZDyn` report the exact byte count the marshal writes.
  Two size errors that cancel pass the length check and encode a wrong value.
- `HashTreeRootWith` and `HashTreeRootWithDyn` of a composite leave one scope's
  root on the walker. Those of a basic-shaped value (a basic value, a large uint
  held as words, or a wrapper around either) leave the value's packed bytes,
  which the engines pad to a chunk outside a packed scope. A delegate that
  reads a root back with `Hash` keeps it only while `HashErr`, read after
  `Hash`, is nil.
- Nothing compares the bytes or the root a delegate produces against the
  schema.

Generated code keeps these rules by construction: a generated method that
breaks one is a bug in the generator, and worth reporting. A hand-written
delegate that breaks one encodes a wrong value, answers a wrong root, or
panics, and no error says so.
