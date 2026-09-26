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
is preferred on every path. A static method baked its tags in and enforces
them on every operation, so a child whose size or limit depends on the spec is
always reached through a spec-aware surface. The two engines decide this
differently: the reflection engine resolves the spec values and only counts
one that differs from the static tag (`HasDynamicSize`, `HasDynamicMax`),
while generated code, which sees no spec values, counts every expression
(`HasSizeExpr`, `HasMaxExpr`).

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

A static build reaching a child that has no static surface inlines the child's
structure. A child with no traversable structure, a custom type or an
external fully-delegated type built without its subtree, cannot be inlined and
fails generation with a message naming the missing static method.

## Custom types

A `ssz-type:"custom"` value always delegates: it has no structure to walk. Its
dynamic or streaming surface is preferred over its static one whenever a
spec-aware call is allowed, since a static method may bake in preset values.
A static build reaches it through its static surface or fails.

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
  The static surface stays governed by `WithNoFastSsz`. This is how the
  differential tests compare generated code against the reflection walk.
- `-without-dynamic-expressions` produces static handlers, which follow the
  static rows above.

## Wrappers of a generated type

A default generation of a type with spec expressions emits static wrappers
only with `-legacy`; each forwards to the type's own dynamic method with the
global instance's specs. A spec-free generation emits real static bodies.
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
- `HashTreeRootWith` and `HashTreeRootWithDyn` leave one scope's root on the
  walker. A delegate that reads a root back with `Hash` keeps it only while
  `HashErr`, read after `Hash`, is nil.
- Nothing compares the bytes or the root a delegate produces against the
  schema.

Generated code keeps these rules by construction: a generated method that
breaks one is a bug in the generator, and worth reporting. A hand-written
delegate that breaks one encodes a wrong value, answers a wrong root, or
panics, and no error says so.
