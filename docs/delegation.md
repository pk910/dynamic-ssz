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
delegation is on (`WithNoFastSsz` not set), the child carries no size
expression, and for hashing no limit expression either. Under those conditions
the static method produces the same bytes and root as the dynamic one and
skips the spec argument, so it is preferred on every path. A child with spec
expressions is always reached through a spec-aware surface.

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
