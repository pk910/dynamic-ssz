// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
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

// findVariant returns the descriptor of the variant built with exactly the
// given hints, or nil.
func findVariant(variants []*hintedVariant, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) *TypeDescriptor {
	for _, variant := range variants {
		if variant.matchesHints(sizeHints, maxSizeHints, typeHints) {
			return variant.desc
		}
	}
	return nil
}

// typeEntry holds every published hint-free descriptor of one runtime type.
type typeEntry struct {
	// plain is the descriptor with the runtime type as its own schema.
	plain atomic.Pointer[TypeDescriptor]
	// views maps a schema type's identity word to the view descriptor of the
	// runtime type under that schema.
	views sync.Map
}

// TypeCache manages cached type descriptors.
//
// Rules of the cache:
//   - A TypeDescriptor is immutable once published. Only a builder modifies
//     descriptors, and only the ones it owns and has not published yet.
//   - The per-call lookup of a hint-free descriptor takes no lock.
//   - buildMu is held for every build, publication, removal and GetAllTypes,
//     and is released only around a static type's sizer (delegatedStaticSize).
//   - Publication is all-or-nothing per top-level build, and the first build
//     to publish a key wins.
//   - Build errors are not cached; a failing type is analysed again per call.
type TypeCache struct {
	specs sszutils.DynamicSpecs

	// published maps a runtime type's identity word (sszutils.TypeWord) to its
	// *typeEntry. Readers take no lock. An entry is created once per type; its
	// descriptors are written under buildMu and cleared by removals.
	published sync.Map

	// buildMu serialises descriptor builds, publication, removals and
	// GetAllTypes. The per-call lookup never takes it. A build releases it
	// around the one call into user code it makes, a static type's sizer (see
	// delegatedStaticSize); its own state lives in a builder, so nothing
	// half-built is reachable while it is released.
	buildMu sync.Mutex
	// hinted holds the descriptors built for references that carry hints.
	// It is only accessed under buildMu, by builders and removals.
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

// NewTypeCache creates a new type cache
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
// The method is thread-safe and ensures sequential processing to prevent duplicate computation
// of type descriptors when called concurrently for the same type.
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
// Type descriptors are only cached when no size hints are provided (i.e., for root types).
// When size hints are present, the descriptor is computed dynamically to accommodate the
// specific constraints.
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

	// The builder's first lookup is the double check: a build that held
	// buildMu before this one may have published the descriptor.
	b := newBuilder(tc)
	desc, err := b.getTypeDescriptor(runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
	if err != nil {
		// Nothing of a failed build is published.
		return nil, err
	}

	// A build that involved a recursive cycle has descriptors that read
	// in-progress children; re-derive the child-propagated flags now that the
	// graph is complete, before any of it is published. Non-recursive builds
	// are already exact.
	if b.recursion {
		FixupRecursiveFlags(desc)
	}
	if published := tc.publish(b); published != nil {
		desc = published
	}

	return desc, nil
}

// lookupPublished returns the published hint-free descriptor of a (runtime,
// schema) pair, or nil. It takes no lock.
func (tc *TypeCache) lookupPublished(runtimeType, schemaType reflect.Type) *TypeDescriptor {
	e, ok := tc.published.Load(sszutils.TypeWord(runtimeType))
	if !ok {
		return nil
	}
	entry := e.(*typeEntry)
	if runtimeType == schemaType {
		return entry.plain.Load()
	}
	if desc, ok := entry.views.Load(sszutils.TypeWord(schemaType)); ok {
		return desc.(*TypeDescriptor)
	}
	return nil
}

// entryFor returns the published entry of a runtime type, creating it on
// first use. The caller holds buildMu.
func (tc *TypeCache) entryFor(runtimeType reflect.Type) *typeEntry {
	word := sszutils.TypeWord(runtimeType)
	e, ok := tc.published.Load(word)
	if !ok {
		e, _ = tc.published.LoadOrStore(word, &typeEntry{})
	}
	return e.(*typeEntry)
}

// publish makes every descriptor a successful build completed visible to
// lookups, and returns the published descriptor of the build's root (the
// last one completed), or nil when the build completed none. Publication is
// all-or-nothing per top-level build. Builds are serialised by buildMu, so a
// key is already published only when another build ran while this one waited
// on a sizer; the first publisher wins, and both descriptors are equal. The
// caller holds buildMu.
func (tc *TypeCache) publish(b *builder) *TypeDescriptor {
	var root *TypeDescriptor
	for _, ref := range b.order {
		desc := ref.variant.desc
		switch {
		case ref.hinted:
			if existing := findVariant(tc.hinted[ref.key], ref.variant.sizeHints, ref.variant.maxSizeHints, ref.variant.typeHints); existing != nil {
				desc = existing
				break
			}
			tc.hinted[ref.key] = append(tc.hinted[ref.key], ref.variant)
		case ref.key.runtime == ref.key.schema:
			entry := tc.entryFor(ref.key.runtime)
			if !entry.plain.CompareAndSwap(nil, desc) {
				desc = entry.plain.Load()
			}
		default:
			entry := tc.entryFor(ref.key.runtime)
			if actual, loaded := entry.views.LoadOrStore(sszutils.TypeWord(ref.key.schema), desc); loaded {
				desc = actual.(*TypeDescriptor)
			}
		}
		root = desc
	}
	return root
}

func (tc *TypeCache) getCompatFlag(runtimeType, schemaType reflect.Type) SszCompatFlag {
	runtimeTypeName := runtimeType.Name()
	runtimeTypePkgPath := runtimeType.PkgPath()
	if runtimeTypePkgPath == "" && runtimeType.Kind() == reflect.Ptr {
		runtimeTypePkgPath = runtimeType.Elem().PkgPath()
	}

	runtimeTypeKey := runtimeTypeName
	if runtimeTypePkgPath != "" {
		runtimeTypeKey = runtimeTypePkgPath + "." + runtimeTypeName
	}

	schemaTypeName := schemaType.Name()
	schemaTypePkgPath := schemaType.PkgPath()
	if schemaTypePkgPath == "" && schemaType.Kind() == reflect.Ptr {
		schemaTypePkgPath = schemaType.Elem().PkgPath()
	}

	schemaTypeKey := schemaTypeName
	if schemaTypePkgPath != "" {
		schemaTypeKey = schemaTypePkgPath + "." + schemaTypeName
	}

	if runtimeTypeKey != schemaTypeKey {
		runtimeTypeKey = fmt.Sprintf("%v|%v", runtimeTypeKey, schemaTypeKey)
	}

	return tc.CompatFlags[runtimeTypeKey]
}

// customSpecFlags returns the flag a custom type raises for the spec values it
// may use. A custom type is not traversed, so what it does with a spec value
// is not seen; its methods say it instead. One that carries a spec-aware
// method takes the spec set and is treated as depending on it, which the
// types that hold it derive like any other child flag: none of them is served
// by a static method that baked the values in. A custom type with static
// methods only cannot depend on the spec and raises nothing.
func (tc *TypeCache) customSpecFlags(desc *TypeDescriptor) SszTypeFlag {
	if desc.SszType != SszCustomType || desc.SszCompatFlags&SszCompatFlagDynamicMethods == 0 {
		return 0
	}
	if tc.noSpecResolution {
		return SszTypeFlagHasMaxExpr
	}
	return SszTypeFlagHasDynamicMax
}

// SetMinSize records the smallest number of bytes a value of this type can
// serialize to. Decoders use it to reject an offset table that declares more
// elements than the region can hold, before that count sizes an allocation.
//
// A type with no floor -- a list, an optional list -- keeps 0, which states
// no bound rather than a wrong one. A bit list holds its termination bit, a
// union its selector, an optional its presence byte and a big.Int its sign
// byte, so each holds at least one byte.
//
// It is stored rather than derived at decode time because Len carries different
// meanings per type (bytes for a container's fixed section, elements for a
// vector), so the rule would otherwise be re-derived on every decode. Children
// are read from their own cached value, so a recursive type resolves without
// walking back into itself: a cycle's back edge simply contributes nothing.
//
// Both front ends record it, so the descriptor hashes agree. The reflection
// cache resolves every size against the active spec; the code generator's
// go/types parser records the static tag values and emits the bound as a
// runtime expression instead (see minSizeExpr), since a preset may resolve
// them smaller than the static defaults.
func (td *TypeDescriptor) SetMinSize() {
	// A static type serializes to exactly its size.
	if td.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
		td.MinSize = td.Size
		return
	}

	switch td.SszType {
	case SszTypeWrapperType:
		if td.ElemDesc != nil {
			td.MinSize = td.ElemDesc.MinSize
		}
	case SszContainerType, SszProgressiveContainerType:
		// The fixed section: every field's own size, and four offset bytes for
		// each dynamic one.
		td.MinSize = td.Len
	case SszBitlistType, SszProgressiveBitlistType, SszUnionType, SszCompatibleUnionType, SszOptionalType, SszBigIntType:
		// The termination bit's byte, the selector byte, the presence byte or
		// the sign byte.
		td.MinSize = 1
	case SszVectorType:
		// A vector of dynamic elements leads with one 4-byte offset per element,
		// and every element costs at least its own minimum on top of that. Len is
		// the element count here, not a byte size.
		if td.ElemDesc != nil {
			// An overflowing product would bound the region above the true floor
			// and refuse valid input, so it states no bound instead. The product
			// is formed in two words: checking it after it has wrapped would
			// accept the wrapped value as a floor.
			hi, lo := bits.Mul64(uint64(td.Len), 4+uint64(td.ElemDesc.MinSize))
			if hi == 0 && lo <= math.MaxInt64 {
				td.MinSize = int64(lo)
			}
		}
	default:
		// Everything else can serialize to nothing -- an empty list, an absent
		// optional list -- so it states no floor.
	}
}

// detectCompatFlags records which SSZ delegation interfaces (fastssz, dynamic,
// dynamic-view, and HashTreeRootWith) the type implements. The fastssz family
// is only flagged when no resolved spec value below the type differs from the
// static tags: a fastssz method baked those in, enforces them on every
// operation, and so cannot answer for a value that resolves them differently.
func (tc *TypeCache) detectCompatFlags(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) {
	if desc.SszTypeFlags&(SszTypeFlagHasDynamicSize|SszTypeFlagHasDynamicMax) == 0 {
		desc.SszCompatFlags |= getFastsszCompatFlags(runtimeType)
		if getFastsszHashCompatibility(runtimeType) {
			desc.SszCompatFlags |= SszCompatFlagFastsszHashRoot
		}
		if method := getHashTreeRootWithCompatibility(runtimeType); method != nil {
			desc.HashTreeRootWithMethod = method
			desc.SszCompatFlags |= SszCompatFlagFastsszHashRootWith
		}
	}

	// Check for dynamic interface implementations
	if getDynamicMarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicMarshaler
	}
	if getDynamicUnmarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicUnmarshaler
	}
	if getDynamicEncoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicEncoder
	}
	if getDynamicDecoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicDecoder
	}
	if getDynamicSizerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicSizer
	}
	if getDynamicHashRootCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicHashRoot
	}

	// Check for dynamic view interface implementations (for fork-dependent SSZ schemas).
	// View interfaces are checked on runtimeType because the methods are implemented
	// on the runtime type, while schemaType only defines the SSZ layout.
	if getDynamicViewMarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewMarshaler
	}
	if getDynamicViewUnmarshalerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewUnmarshaler
	}
	if getDynamicViewEncoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewEncoder
	}
	if getDynamicViewDecoderCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewDecoder
	}
	if getDynamicViewSizerCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewSizer
	}
	if getDynamicViewHashRootCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagDynamicViewHashRoot
	}

	desc.SszCompatFlags |= tc.getCompatFlag(runtimeType, schemaType)
}

// delegatedStaticSize returns the fixed SSZ byte size of a fully-delegated static
// type by invoking the type's own sizer on a zero value. A static type's sizer
// derives its result from constants and spec values only — never from field data
// — so a zero value yields the correct size, and spec-dependent fixed sizes are
// resolved against the cache's specs. View descriptors use the view sizer.
//
// The caller holds buildMu. The sizer is user code and may call back into the
// instance it is given (a wrapper sizing its payload, say), which builds and
// publishes other types, so it runs with buildMu released. A sizer whose
// answer depends, through the instance, on the type being built describes an
// infinitely sized type and recurses until the stack overflows.
func (tc *TypeCache) delegatedStaticSize(desc *TypeDescriptor, runtimeType reflect.Type) (int64, bool, error) {
	specs := tc.specs // never nil: NewTypeCache substitutes emptySpecs{}
	zero := reflect.New(runtimeType).Interface()

	// A sizer returns int; its result enters the size domain here, so it is
	// bounded to the SSZ size range like every other size. A refusal states no
	// size at all -- a spec value the cache cannot resolve reads the same as an
	// over-limit total through that one return -- so the type is described
	// without a fixed size rather than refused, and the size is asked for again
	// where a caller can be told why it cannot be given.
	validate := func(n int) (int64, bool, error) {
		if n < 0 {
			return 0, false, nil
		}
		if n > sszutils.MaxSszSize {
			return 0, false, sszutils.NewSszErrorf(sszutils.SizeLimitSentinel(uint64(n)), "sizer for static type %v returned out-of-range size %d", runtimeType, n)
		}

		return int64(n), true, nil
	}

	if desc.GoTypeFlags&GoTypeFlagIsView != 0 {
		if sizer, ok := zero.(sszutils.DynamicViewSizer); ok && desc.CodegenInfo != nil {
			view := *desc.CodegenInfo
			n, served := 0, false
			tc.withoutBuildLock(func() {
				if sizeFn := sizer.SizeSSZDynView(view); sizeFn != nil {
					n, served = sizeFn(specs), true
				}
			})
			if served {
				return validate(n)
			}
		}
		return 0, false, sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "static view type %v does not provide a usable view sizer", runtimeType)
	}

	// Non-view static types may size themselves through either the dynssz sizer
	// (spec-aware) or the fastssz sizer. Fully-delegated callers always provide
	// the DynamicSizer; a static custom type may instead provide only fastssz.
	if sizer, ok := zero.(sszutils.DynamicSizer); ok {
		var n int
		tc.withoutBuildLock(func() { n = sizer.SizeSSZDyn(specs) })
		return validate(n)
	}
	if sizer, ok := zero.(sszutils.FastsszSizer); ok {
		var n int
		tc.withoutBuildLock(func() { n = sizer.SizeSSZ() })
		return validate(n)
	}
	return 0, false, sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "static type %v provides no usable sizer", runtimeType)
}

// withoutBuildLock runs fn, a call into user code, with buildMu released. The
// caller holds buildMu and touches neither the published store nor the hinted
// variants until fn returns; its own state lives in its builder, which no
// other goroutine reaches. buildMu is taken again in a defer, so a panicking
// fn leaves it held for the caller's deferred unlock.
func (tc *TypeCache) withoutBuildLock(fn func()) {
	tc.buildMu.Unlock()
	defer tc.buildMu.Lock()
	fn()
}

// RejectMaxOnVector rejects a limit declared for a dimension that is fixed
// with a different length. A vector's length is its type, so it has no
// capacity left to bound: a diverging limit describes nothing and was being
// ignored, which reads as a bounded list to anyone writing the tag. A limit
// equal to the fixed length is accepted — it states what the type already
// says, and tags are positional, so the common spelling that bounds an outer
// list repeats the inner vector's length in both families
// ([][48]byte `ssz-size:"?,48" ssz-max:"4096,48"`). It is shared by the
// reflection type cache and the code generator's parser so both read the same
// tags the same way.
//
// The dimension is fixed either because ssz-size gave it a length or because
// the Go type is an array, so this catches both spellings; arrayLen carries
// the Go array length (0 otherwise). Only the dimension being built is
// examined: an outer array of bounded lists carries `?` for its own dimension
// and its limits belong to the inner ones.
func RejectMaxOnVector(sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, arrayLen uint64, typeName string) error {
	if len(maxSizeHints) == 0 || maxSizeHints[0].NoValue {
		return nil
	}

	// A limit that only exists as an unresolved expression, or the ssz-max:"0"
	// placeholder, states no capacity either.
	if maxSizeHints[0].Size == 0 {
		return nil
	}

	// The length this dimension is fixed to, mirroring how the vector builders
	// resolve it: an explicit size hint wins over the Go array's own length. A
	// bit-sized hint counts bits where the limit counts elements, so it grants
	// no equality; a purely symbolic length cannot be compared either.
	fixedLen := arrayLen
	if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
		fixedLen = uint64(sizeHints[0].Size)
		if sizeHints[0].Bits {
			fixedLen = 0
		}
	}
	if fixedLen != 0 && maxSizeHints[0].Size == fixedLen {
		return nil
	}

	return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint,
		"ssz-max %d is declared for %s, whose length is fixed: a vector has no capacity to bound (a limit equal to the length is accepted; anything else needs ssz-size or ssz-max, not both, for one dimension)",
		maxSizeHints[0].Size, typeName)
}

// MarkNoSszRoot flags a list or bitlist that carries no limit, unless extended
// types are enabled. It is shared by the reflection type cache and the code
// generator's parser so both classify the same types the same way.
//
// A limit is part of the type in SSZ: List[T, N] and Bitlist[N] need N to
// merkleize, so a list without one has no defined hash tree root. The library
// still hashes it -- merkleizing to the chunks the value occupies and mixing in
// the length, so the root at least identifies the value -- but that root is
// outside the spec and no other implementation will agree on it, which is why
// it is an opt-in extension rather than the default.
//
// Serialization is unaffected: a limit only bounds a list, and encoding and
// decoding never need it. So the flag is set here but only acted on where the
// root is produced, which also keeps a view's data type usable: it carries no
// tags because the layout lives in the view schema, and hashing it through the
// view uses the view's descriptor, which does have the limit.
//
// A progressive list or bitlist is unbounded by design (EIP-7916) and is
// unaffected.
func MarkNoSszRoot(extendedTypes bool, desc *TypeDescriptor) {
	if extendedTypes || desc.SszTypeFlags&SszTypeFlagHasLimit != 0 {
		return
	}

	// A limit carried by a dynssz-max expression is still a limit; it is just
	// not resolvable yet. That is the documented ssz-max:"0" placeholder
	// pattern, and it is how the code generator sees every spec-driven limit,
	// since spec values only exist at runtime.
	if desc.MaxExpression != nil {
		return
	}

	if desc.SszType == SszListType || desc.SszType == SszBitlistType {
		desc.SszTypeFlags |= SszTypeFlagNoSszRoot
	}
}

// NoSszRootError reports that a limit-less list or bitlist cannot be hashed.
// Both engines raise it where the root would be produced: the reflection engine
// at hash time, the code generator while emitting the hash method.
func NoSszRootError(desc *TypeDescriptor) error {
	kind := sszTypeNameList
	if desc.SszType == SszBitlistType {
		kind = sszTypeNameBitlist
	}

	return sszutils.NewSszErrorf(sszutils.ErrExtendedTypeDisabled,
		"%s has no ssz-max, so it has no SSZ hash tree root: add a limit, or enable extended types to hash it as an unbounded %s. If this is a view's data type, hash it through its view instead",
		kind, kind)
}

// GetAllTypes returns a slice of all type keys currently cached in the TypeCache.
//
// This method is useful for cache inspection, debugging, and understanding which types
// have been processed and cached during the application's lifetime. The returned slice
// contains pairs of (runtime, schema) types in no particular order.
//
// When runtime == schema (normal usage), these represent standard type descriptors.
// When they differ, the pair represents a view descriptor for fork-dependent SSZ.
//
// The method holds the build mutex, so it never sees a build half published.
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
		entry := e.(*typeEntry)
		if desc := entry.plain.Load(); desc != nil {
			types = append(types, [2]reflect.Type{desc.Type, desc.SchemaType})
		}
		entry.views.Range(func(_, d any) bool {
			desc := d.(*TypeDescriptor)
			types = append(types, [2]reflect.Type{desc.Type, desc.SchemaType})
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
// This method is useful for cache management scenarios where you need to force
// recomputation of a type descriptor, such as after configuration changes or
// when testing different type configurations.
//
// The method holds the build mutex, so it never overlaps a publication. A
// build waiting on a static type's sizer while the type is removed publishes
// its descriptors when it resumes.
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
//	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(MyStruct{}), nil, nil)
func (tc *TypeCache) RemoveType(t reflect.Type) {
	tc.RemoveTypeKey(t, t)
}

// RemoveTypeKey removes a specific (runtime, schema) type pair from the cache.
//
// This method supports removing view descriptor entries where runtime differs from schema.
// Like RemoveType, it does not stop a build waiting on a static type's sizer from
// publishing the pair again when it resumes.
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
		// The entry itself stays: a reader may hold it, and the next build
		// of the type fills it again.
		if e, ok := tc.published.Load(sszutils.TypeWord(key.runtime)); ok {
			entry := e.(*typeEntry)
			if key.runtime == key.schema {
				entry.plain.Store(nil)
			} else {
				entry.views.Delete(sszutils.TypeWord(key.schema))
			}
		}
		delete(tc.hinted, key)
	}
}

// RemoveAllTypes clears all cached type descriptors from the cache, both the
// plain entries and the hint-carrying variants.
//
// This method is useful for:
//   - Memory management in long-running applications
//   - Testing scenarios requiring a clean cache state
//
// The method holds the build mutex, so it never overlaps a publication. After
// calling this method, all subsequent type descriptor requests will trigger
// recomputation, except that a build waiting on a static type's sizer
// publishes its descriptors when it resumes. Spec values are resolved and memoized by the
// owning DynSsz instance, so a changed spec set needs a new DynSsz rather
// than a cleared cache.
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

	// Clear all references, including the hint-carrying variants that
	// container fields and list elements are cached under.
	tc.published.Range(func(word, _ any) bool {
		tc.published.Delete(word)
		return true
	})
	tc.hinted = make(map[typeKey][]*hintedVariant, 64)
}

// extractGenericTypeParameter extracts the generic type parameter from a CompatibleUnion type.
// This uses reflection to call the GetDescriptorType method on the union type.
func (tc *TypeCache) extractGenericTypeParameter(unionType reflect.Type) (reflect.Type, error) {
	// Create a zero value of the union type to call methods on
	unionValue := reflect.New(unionType)

	// Get the GetDescriptorType method
	method := unionValue.MethodByName("GetDescriptorType")
	if !method.IsValid() {
		return nil, sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "GetDescriptorType method not found on type %s", unionType)
	}

	// Call the method to get the descriptor type
	results := method.Call(nil)
	if len(results) == 0 {
		return nil, sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType returned no results")
	}

	// Extract the reflect.Type from the result
	descriptorType, ok := results[0].Interface().(reflect.Type)
	if !ok {
		return nil, sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType did not return a reflect.Type")
	}

	return descriptorType, nil
}

// GetTypeHash computes a SHA-256 hash of the TypeDescriptor's JSON
// representation. This hash uniquely identifies the type's SSZ layout and is
// used by the code generator to detect when a type's structure has changed and
// regeneration is needed.
//
// Recursive types form a cyclic descriptor graph that standard JSON
// marshalling cannot represent; those fall back to a deterministic
// reference-based serialization so distinct recursive layouts hash to
// distinct values. Acyclic descriptors keep the plain JSON form.
//
// The hash covers that encoding, not the wire format: encoding/json emits
// fields in declaration order, so reordering TypeDescriptor moves every hash
// without changing any serialization. It identifies a layout within one
// version of this package, not across versions.
func (td *TypeDescriptor) GetTypeHash() [32]byte {
	jsonDesc, err := json.Marshal(td)
	if err != nil {
		jsonDesc = marshalCyclicDescriptor(td)
	}
	hash := sha256.Sum256(jsonDesc)
	return hash
}
