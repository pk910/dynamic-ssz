// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// typeKey represents a composite cache key for type descriptors.
// When schema type differs from runtime type (fork views), we need to cache
// descriptors based on both types since the same runtime type may have
// different SSZ layouts depending on the schema.
type typeKey struct {
	runtime reflect.Type // The type where actual data lives
	schema  reflect.Type // The type that defines SSZ layout (may differ for views)
}

// hintSet identifies a descriptor build variant by the external hints it was
// built with. A descriptor is a pure function of (type pair, hints, cache
// configuration), so identical hints always yield an identical descriptor.
type hintSet struct {
	sizeHints    []SszSizeHint
	maxSizeHints []SszMaxSizeHint
	typeHints    []SszTypeHint
}

func (h *hintSet) matchesHints(sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) bool {
	return slices.Equal(h.sizeHints, sizeHints) && slices.Equal(h.maxSizeHints, maxSizeHints) && slices.Equal(h.typeHints, typeHints)
}

// hintedVariant is a cached descriptor for a (type pair, hints) combination.
// Hint-carrying references (field tags, size/max/type annotations) are common —
// most Ethereum container fields carry ssz-size/ssz-max — and the same
// combination recurs across every container referencing the type, so these are
// cached alongside the hint-free descriptors instead of being rebuilt per
// reference site.
type hintedVariant struct {
	desc *TypeDescriptor
	hintSet
}

// findVariant returns the descriptor built with exactly the given hints, or nil.
func findVariant(variants []*hintedVariant, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) *TypeDescriptor {
	for _, variant := range variants {
		if variant.matchesHints(sizeHints, maxSizeHints, typeHints) {
			return variant.desc
		}
	}
	return nil
}

// typeEntry holds the published hint-free descriptors of one runtime type.
type typeEntry struct {
	plain atomic.Pointer[TypeDescriptor] // schema == runtime
	views sync.Map                       // TypeWord(schema) -> *TypeDescriptor
}

// TypeCache manages cached type descriptors.
//
//   - A published TypeDescriptor is immutable.
//   - Hint-free lookups take no lock.
//   - buildMu serialises builds, publication, removals and GetAllTypes. A
//     build releases it around a static type's sizer and a view type's
//     dispatcher, which may call back into the cache. Other user code (the
//     spec resolver, a union or wrapper type's GetDescriptorType) runs with
//     it held and must not.
//   - A build publishes a descriptor once everything it references is
//     complete, the members of a recursive cycle together. The first publisher
//     of a key wins.
//   - A failed build keeps what it published; the failing type is not cached.
type TypeCache struct {
	specs sszutils.DynamicSpecs

	// published maps TypeWord(runtime type) to its *typeEntry. Read without a
	// lock, written under buildMu.
	published sync.Map

	buildMu sync.Mutex
	// hinted holds the descriptors of hint-carrying references. Guarded by buildMu.
	hinted map[typeKey][]*hintedVariant

	CompatFlags   map[string]SszCompatFlag
	ExtendedTypes bool
	NoDelegation  bool

	// noSpecResolution marks a cache that builds descriptors for code generation
	// rather than for this process. See DisableSpecResolution.
	noSpecResolution bool

	// promotedDelegation memoizes PromotedDelegationMethods by the identity of
	// the type. The detection sits on the per-call delegation hot path, so
	// readers load an immutable map without a lock, and the first lookup of a
	// type publishes a copy under promotedDelegationMutex.
	promotedDelegation      atomic.Pointer[map[unsafe.Pointer]map[string]bool]
	promotedDelegationMutex sync.Mutex
}

// emptySpecs is a no-op DynamicSpecs used when a TypeCache is created without a
// spec provider, so dynssz-* tags resolve to their static fallback instead of
// dereferencing a nil interface.
type emptySpecs struct{}

func (emptySpecs) ResolveSpecValue(string) (bool, uint64, error) { return false, 0, nil }

// NewTypeCache creates a new type cache. specs must be safe for concurrent use.
func NewTypeCache(specs sszutils.DynamicSpecs) *TypeCache {
	if specs == nil {
		specs = emptySpecs{}
	}
	return &TypeCache{
		specs:         specs,
		hinted:        make(map[typeKey][]*hintedVariant, 64),
		CompatFlags:   map[string]SszCompatFlag{},
		ExtendedTypes: false,
	}
}

// DisableSpecResolution switches the cache to building descriptors for code
// generation rather than for this process. Spec expressions are recorded as
// written instead of being resolved, because the generated code resolves them
// against whatever specs it runs under; resolving them here would bake the
// generator's own values into the output and would classify a dimension by
// whether a value happened to be loaded. The values are dropped rather than
// kept aside, so nothing can reach them by accident.
func (tc *TypeCache) DisableSpecResolution() {
	tc.noSpecResolution = true
	tc.specs = emptySpecs{}
}

// GetTypeDescriptor returns a cached type descriptor for the given type, computing it if necessary.
//
// This method is the primary interface for obtaining type descriptors, which contain optimized
// metadata about how to serialize, deserialize, and hash types according to SSZ specifications.
// Type descriptors are cached for performance, avoiding repeated reflection and analysis of the
// same types.
//
// The method is thread-safe: cached lookups take no lock, builds are serialised, and concurrent
// first calls for a type all get the same descriptor. A static type's sizer may run more than
// once and concurrently (see docs/delegation.md).
//
// Parameters:
//   - t: The reflect.Type for which to obtain a descriptor
//   - sizeHints: Optional size hints from parent structures' tags. Pass nil for top-level types.
//   - maxSizeHints: Optional max size hints from parent structures' tags. Pass nil for top-level types.
//   - typeHints: Optional type hints from parent structures' tags. Pass nil for top-level types.
//
// Returns:
//   - *TypeDescriptor: The type descriptor containing metadata for SSZ operations
//   - error: An error if the type cannot be analyzed or contains unsupported features
//
// Descriptors are cached per type and per exact combination of hints.
//
// Example:
//
//	typeDesc, err := cache.GetTypeDescriptor(reflect.TypeOf(myStruct), nil, nil, nil)
//	if err != nil {
//	    log.Fatal("Failed to get type descriptor:", err)
//	}
//	fmt.Printf("Type size: %d bytes (dynamic: %v)\n", typeDesc.Size,
//	    typeDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0)
func (tc *TypeCache) GetTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	// When no view descriptor is used, runtime and schema types are the same
	return tc.GetTypeDescriptorWithSchema(t, t, sizeHints, maxSizeHints, typeHints)
}

// GetTypeDescriptorWithSchema returns a cached type descriptor for a (runtime, schema) type pair.
//
// This method supports fork-dependent SSZ schemas (view descriptors) where the schema type
// defines the SSZ layout while the runtime type holds the actual data. This allows different
// SSZ serializations of the same runtime data based on the schema provided.
//
// When runtimeType == schemaType, this behaves identically to GetTypeDescriptor.
// When they differ, the descriptor is built using schema's field definitions (names, tags,
// order) but accessing data from the runtime type's fields.
//
// Parameters:
//   - runtimeType: The reflect.Type where actual data lives
//   - schemaType: The reflect.Type that defines SSZ layout (field order, tags, limits)
//   - sizeHints: Optional size hints from parent structures' tags
//   - maxSizeHints: Optional max size hints from parent structures' tags
//   - typeHints: Optional type hints from parent structures' tags
//
// Returns:
//   - *TypeDescriptor: The type descriptor for the (runtime, schema) pair
//   - error: An error if types are incompatible or analysis fails
//
// Example with view descriptor:
//
//	// Runtime type (superset)
//	type BeaconBlockBody struct { ... }
//	// Schema/view type for Altair fork
//	type BodyAltairView struct { ... }
//
//	desc, err := cache.GetTypeDescriptorWithSchema(
//	    reflect.TypeOf(BeaconBlockBody{}),
//	    reflect.TypeOf(BodyAltairView{}),
//	    nil, nil, nil,
//	)
func (tc *TypeCache) GetTypeDescriptorWithSchema(runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	if runtimeType == nil || schemaType == nil {
		return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "type must not be nil")
	}

	// Lock-free per-call path: a published hint-free descriptor.
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
		if desc := tc.lookupPublished(runtimeType, schemaType); desc != nil {
			return desc, nil
		}
	}

	tc.buildMu.Lock()
	defer tc.buildMu.Unlock()

	// The builder's first lookup is the double check, and it publishes what it
	// completes.
	return newBuilder(tc).getTypeDescriptor(runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
}

// lookupPublished returns the published hint-free descriptor of a (runtime,
// schema) pair, or nil. It takes no lock.
func (tc *TypeCache) lookupPublished(runtimeType, schemaType reflect.Type) *TypeDescriptor {
	e, ok := tc.published.Load(TypeWord(runtimeType))
	if !ok {
		return nil
	}
	entry, ok := e.(*typeEntry)
	if !ok {
		return nil
	}
	if runtimeType == schemaType {
		return entry.plain.Load()
	}
	view, _ := entry.views.Load(TypeWord(schemaType))
	desc, _ := view.(*TypeDescriptor)
	return desc
}

// entryFor returns the published entry of a runtime type, creating it on
// first use. The caller holds buildMu.
func (tc *TypeCache) entryFor(runtimeType reflect.Type) *typeEntry {
	word := TypeWord(runtimeType)
	e, ok := tc.published.Load(word)
	if !ok {
		e, _ = tc.published.LoadOrStore(word, &typeEntry{})
	}
	entry, ok := e.(*typeEntry)
	if !ok {
		panic(fmt.Sprintf("ssztypes: type cache store holds %T, not *typeEntry", e))
	}
	return entry
}

// publish stores the descriptors b completed since its order held mark
// entries and returns the published descriptor of the last one. A key another
// build published meanwhile keeps its descriptor, and this build's references
// to its own copy are redirected to it, so every published graph holds one
// descriptor per key. Redirecting is complete before anything is stored:
// nothing is written once a lock-free reader can reach it. The caller holds
// buildMu.
//
// The descriptors referencing a copy are laid out with its size, so a
// published descriptor of another size cannot replace it: the build fails,
// and nothing of the batch is stored. Sizes differ only when a sizer returns
// different sizes for one type.
func (tc *TypeCache) publish(b *builder, mark int) (*TypeDescriptor, error) {
	batch := b.order[mark:]
	b.order = b.order[:mark]

	var replaced map[*TypeDescriptor]*TypeDescriptor
	for _, ref := range batch {
		var existing *TypeDescriptor
		if ref.hinted {
			existing = findVariant(tc.hinted[ref.key], ref.variant.sizeHints, ref.variant.maxSizeHints, ref.variant.typeHints)
		} else {
			existing = tc.lookupPublished(ref.key.runtime, ref.key.schema)
		}
		if existing != nil {
			desc := ref.variant.desc
			if existing.Size != desc.Size || (existing.SszTypeFlags^desc.SszTypeFlags)&SszTypeFlagIsDynamic != 0 {
				return nil, sszutils.NewSszErrorf(sszutils.ErrUnsupportedType, "type %v was described with size %d and with size %d: its sizer must return the same size on every call", desc.Type, existing.Size, desc.Size)
			}
			if replaced == nil {
				replaced = make(map[*TypeDescriptor]*TypeDescriptor, len(batch))
			}
			replaced[ref.variant.desc] = existing
		}
	}

	for _, ref := range batch {
		desc := ref.variant.desc
		if _, lost := replaced[desc]; lost || replaced == nil {
			continue
		}
		if existing, lost := replaced[desc.ElemDesc]; lost {
			desc.ElemDesc = existing
		}
		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				if existing, lost := replaced[desc.ContainerDesc.Fields[i].Type]; lost {
					desc.ContainerDesc.Fields[i].Type = existing
				}
			}
		}
		for index, variant := range desc.UnionVariants {
			if existing, lost := replaced[variant]; lost {
				desc.UnionVariants[index] = existing
			}
		}
	}

	var root *TypeDescriptor
	for _, ref := range batch {
		root = ref.variant.desc
		if existing, lost := replaced[root]; lost {
			// Later references of this build resolve to the published one.
			root = existing
			if ref.hinted {
				ref.variant.desc = existing
			} else {
				b.builtPlain[ref.key] = existing
			}
			continue
		}
		switch {
		case ref.hinted:
			tc.hinted[ref.key] = append(tc.hinted[ref.key], ref.variant)
		case ref.key.runtime == ref.key.schema:
			tc.entryFor(ref.key.runtime).plain.Store(root)
		default:
			tc.entryFor(ref.key.runtime).views.Store(TypeWord(ref.key.schema), root)
		}
	}
	return root, nil
}

// GetAllTypes returns a slice of all type keys currently cached in the TypeCache.
//
// Deprecated: debugging aid only; may be removed in a future release.
//
// The returned slice contains (runtime, schema) type pairs in no particular
// order; the two differ for a view descriptor.
//
// Returns:
//   - [][2]reflect.Type: A slice of [runtime, schema] type pairs
//
// Example:
//
//	cachedTypes := cache.GetAllTypes()
//	fmt.Printf("TypeCache contains %d type pairs\n", len(cachedTypes))
//	for _, pair := range cachedTypes {
//	    if pair[0] == pair[1] {
//	        fmt.Printf("  - %s\n", pair[0].String())
//	    } else {
//	        fmt.Printf("  - %s (view: %s)\n", pair[0].String(), pair[1].String())
//	    }
//	}
func (tc *TypeCache) GetAllTypes() [][2]reflect.Type {
	tc.buildMu.Lock()
	defer tc.buildMu.Unlock()

	types := make([][2]reflect.Type, 0, len(tc.hinted))
	tc.published.Range(func(_, e any) bool {
		entry, ok := e.(*typeEntry)
		if !ok {
			return true
		}
		if desc := entry.plain.Load(); desc != nil {
			types = append(types, [2]reflect.Type{desc.Type, desc.SchemaType})
		}
		entry.views.Range(func(_, d any) bool {
			if desc, ok := d.(*TypeDescriptor); ok {
				types = append(types, [2]reflect.Type{desc.Type, desc.SchemaType})
			}
			return true
		})
		return true
	})
	// A type reached only through hint-carrying references has no plain entry.
	for key := range tc.hinted {
		if tc.lookupPublished(key.runtime, key.schema) == nil {
			types = append(types, [2]reflect.Type{key.runtime, key.schema})
		}
	}

	return types
}

// RemoveType removes a specific type (with runtime == schema) from the cache,
// in both its value and pointer form.
//
// Deprecated: debugging aid only; may be removed in a future release. Use a
// new DynSsz instance for a different configuration or spec set.
//
// Descriptors already handed out, and descriptors of other types that embed
// the removed one, are not affected. A build in flight may publish the type
// again.
// Note: This only removes entries where runtime type equals schema type.
// Use RemoveTypeKey to remove view descriptor entries.
//
// Parameters:
//   - t: The reflect.Type to remove from the cache
//
// Example:
//
//	// Remove a type to force recomputation
//	cache.RemoveType(reflect.TypeOf(MyStruct{}))
//
//	// Next call to GetTypeDescriptor will rebuild the descriptor
//	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(MyStruct{}), nil, nil, nil)
func (tc *TypeCache) RemoveType(t reflect.Type) {
	tc.RemoveTypeKey(t, t)
}

// RemoveTypeKey removes a specific (runtime, schema) type pair from the cache.
//
// Deprecated: debugging aid only, like RemoveType.
//
// This method supports removing view descriptor entries where runtime differs from schema.
//
// Parameters:
//   - runtimeType: The runtime type of the cache entry to remove
//   - schemaType: The schema type of the cache entry to remove
func (tc *TypeCache) RemoveTypeKey(runtimeType, schemaType reflect.Type) {
	tc.buildMu.Lock()
	defer tc.buildMu.Unlock()

	if runtimeType.Kind() == reflect.Ptr {
		runtimeType = runtimeType.Elem()
	}
	if schemaType.Kind() == reflect.Ptr {
		schemaType = schemaType.Elem()
	}

	// A type is cached in the form it was requested in, so both the value and
	// the pointer form are removed.
	for _, key := range []typeKey{
		{runtime: runtimeType, schema: schemaType},
		{runtime: reflect.PointerTo(runtimeType), schema: reflect.PointerTo(schemaType)},
	} {
		// The entry stays; the next build of the type fills it again.
		e, _ := tc.published.Load(TypeWord(key.runtime))
		if entry, ok := e.(*typeEntry); ok {
			if key.runtime == key.schema {
				entry.plain.Store(nil)
			} else {
				entry.views.Delete(TypeWord(key.schema))
			}
		}
		delete(tc.hinted, key)
	}
}

// RemoveAllTypes clears all cached type descriptors from the cache, both the
// plain entries and the hint-carrying variants.
//
// Deprecated: debugging aid only, for tests that want a cold cache; may be
// removed in a future release.
//
// A build in flight may publish its descriptors after the call. A changed
// spec set needs a new DynSsz rather than a cleared cache.
//
// Example:
//
//	cache.RemoveAllTypes()
//
//	// All types will be recomputed
//	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(MyStruct{}), nil, nil, nil)
func (tc *TypeCache) RemoveAllTypes() {
	tc.buildMu.Lock()
	defer tc.buildMu.Unlock()

	// Clear the plain entries and the hint-carrying variants.
	tc.published.Range(func(word, _ any) bool {
		tc.published.Delete(word)
		return true
	})
	tc.hinted = make(map[typeKey][]*hintedVariant, 64)
}
