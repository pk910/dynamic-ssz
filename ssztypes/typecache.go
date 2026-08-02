// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"

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

// buildEntry tracks a descriptor currently under construction, for cycle
// detection. depth is the variable-length nesting depth at which the build
// started; a cycle re-entered at a strictly greater depth crossed a
// variable-length collection (list/optional) and is a legal, finite SSZ type.
// The hints the build started with identify the entry: the same Go type can be
// under construction with different hints at once (e.g. a plain pointer and an
// optional-hinted reference to it), and only a re-entry with identical hints is
// a genuine cycle — anything else is a differently-shaped sibling descriptor.
type buildEntry struct {
	desc  *TypeDescriptor
	depth int
	hintSet
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

// pendingKey records a cache insertion of the current top-level build so a
// failed build can purge it (entries cached during a failed recursive build
// were built against an abandoned graph).
type pendingKey struct {
	key    typeKey
	hinted bool
}

// TypeCache manages cached type descriptors
type TypeCache struct {
	specs             sszutils.DynamicSpecs
	mutex             sync.RWMutex
	descriptors       map[typeKey]*TypeDescriptor
	hintedDescriptors map[typeKey][]*hintedVariant
	building          map[typeKey][]*buildEntry
	dynDepth          int
	recursion         bool
	pendingKeys       []pendingKey
	CompatFlags       map[string]SszCompatFlag
	ExtendedTypes     bool
	NoDelegation      bool

	// promotedDelegation memoizes InheritsPromotedDelegation (reflect.Type ->
	// bool); the detection sits on the per-call delegation hot path.
	promotedDelegation sync.Map
}

// NewTypeCache creates a new type cache
// emptySpecs is a no-op DynamicSpecs used when a TypeCache is created without a
// spec provider, so dynssz-* tags resolve to their static fallback instead of
// dereferencing a nil interface.
type emptySpecs struct{}

func (emptySpecs) ResolveSpecValue(string) (bool, uint64, error) { return false, 0, nil }

func NewTypeCache(specs sszutils.DynamicSpecs) *TypeCache {
	if specs == nil {
		specs = emptySpecs{}
	}
	return &TypeCache{
		specs:             specs,
		descriptors:       make(map[typeKey]*TypeDescriptor),
		hintedDescriptors: make(map[typeKey][]*hintedVariant),
		building:          make(map[typeKey][]*buildEntry),
		CompatFlags:       map[string]SszCompatFlag{},
		ExtendedTypes:     false,
	}
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
//	typeDesc, err := cache.GetTypeDescriptor(reflect.TypeOf(myStruct), nil, nil)
//	if err != nil {
//	    log.Fatal("Failed to get type descriptor:", err)
//	}
//	fmt.Printf("Type size: %d bytes (dynamic: %v)\n", typeDesc.Size, typeDesc.Size < 0)
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

	key := typeKey{runtime: runtimeType, schema: schemaType}

	// Check cache first (read lock)
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
		tc.mutex.RLock()
		if desc, exists := tc.descriptors[key]; exists {
			tc.mutex.RUnlock()
			return desc, nil
		}
		tc.mutex.RUnlock()
	}

	// If not in cache, build and cache it (write lock)
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	return tc.getTypeDescriptor(runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
}

// getTypeDescriptor returns a cached type descriptor for a (runtime, schema) pair.
// When runtimeType == schemaType, this is the standard descriptor building.
// When they differ, it handles view descriptors where schema defines SSZ layout.
func (tc *TypeCache) getTypeDescriptor(runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	key := typeKey{runtime: runtimeType, schema: schemaType}
	cacheable := len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0

	if cacheable {
		if desc, exists := tc.descriptors[key]; exists {
			return desc, nil
		}
	} else {
		// Hint-carrying builds are cached per exact hint combination: the same
		// (type, hints) pair recurs across every container referencing the type,
		// and the descriptor depends only on the type pair, the hints and the
		// cache configuration.
		for _, variant := range tc.hintedDescriptors[key] {
			if variant.matchesHints(sizeHints, maxSizeHints, typeHints) {
				return variant.desc, nil
			}
		}
	}

	// Detect self-referential (recursive) types. The whole descriptor tree is
	// built under the cache write lock, so the build stack is tracked with a
	// simple map holding one entry per (type pair, hints) build in flight — the
	// same Go type can be under construction with different hints at once, and
	// only a re-entry with identical hints is a genuine cycle. A cycle is a
	// legal, finite SSZ type only when it crosses a variable-length collection
	// (list/optional): those are offset/presence encoded, so the type has finite
	// static size and terminates at runtime. A cycle through only fixed-size
	// fields (container/vector) has infinite static size and cannot be
	// serialized, so it is rejected.
	for _, entry := range tc.building[key] {
		if !entry.matchesHints(sizeHints, maxSizeHints, typeHints) {
			continue
		}
		if tc.dynDepth <= entry.depth {
			return nil, sszutils.NewSszErrorf(sszutils.ErrUnsupportedType, "recursive type %v is not supported", runtimeType)
		}
		// Legal cycle: hand back the descriptor still under construction. Its
		// child-derived fields are incomplete at this point, so consumers must not
		// rely on them; two measures keep the final graph correct:
		//  - Crossing a variable-length collection to legalize the cycle means
		//    every cycle member has a variable-size field on the cycle path, so
		//    the descriptor is provably dynamic. Setting IsDynamic here lets a
		//    container or vector reading it mid-build lay the field out as a
		//    dynamic (offset) field, which matches its final state.
		//  - The remaining child-derived flags are re-derived to a fixpoint by
		//    FixupRecursiveFlags once the whole graph is complete.
		entry.desc.SszTypeFlags |= SszTypeFlagIsDynamic
		tc.recursion = true
		return entry.desc, nil
	}

	// The top-level build of a descriptor tree is the one entered with an empty
	// build stack; recursion bookkeeping is scoped to it.
	topLevel := len(tc.building) == 0
	if topLevel {
		tc.recursion = false
		tc.pendingKeys = tc.pendingKeys[:0]
	}

	// Allocate the descriptor before building its subtree so a recursive
	// reference through a variable-length collection can back-patch to it.
	desc := &TypeDescriptor{Type: runtimeType, SchemaType: schemaType}
	tc.building[key] = append(tc.building[key], &buildEntry{desc: desc, depth: tc.dynDepth, hintSet: hintSet{sizeHints: sizeHints, maxSizeHints: maxSizeHints, typeHints: typeHints}})
	defer func() {
		// Builds nest strictly, so this build's entry is the last one pushed.
		entries := tc.building[key]
		if len(entries) <= 1 {
			delete(tc.building, key)
		} else {
			tc.building[key] = entries[:len(entries)-1]
		}
	}()

	if _, err := tc.buildTypeDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints); err != nil {
		// A cycle member completes and is cached before the cycle head finishes.
		// If the head's build fails afterwards, those members were built against
		// an abandoned graph and never flag-fixed; purge them so a later build
		// cannot read a poisoned cache entry. Entries appended to a hinted
		// variant list during this build sit at its tail, so reverse order pops
		// them correctly even with multiple appends per key.
		if topLevel && tc.recursion {
			for i := len(tc.pendingKeys) - 1; i >= 0; i-- {
				pending := tc.pendingKeys[i]
				if !pending.hinted {
					delete(tc.descriptors, pending.key)
					continue
				}
				variants := tc.hintedDescriptors[pending.key]
				if len(variants) <= 1 {
					delete(tc.hintedDescriptors, pending.key)
				} else {
					tc.hintedDescriptors[pending.key] = variants[:len(variants)-1]
				}
			}
		}
		return nil, err
	}

	if cacheable {
		tc.descriptors[key] = desc
		tc.pendingKeys = append(tc.pendingKeys, pendingKey{key: key})
	} else {
		// Clone the hint slices: they may alias a caller-owned parse result or a
		// sub-slice of a parent's hints, while the cached variant must own its key.
		variant := &hintedVariant{desc: desc, hintSet: hintSet{
			sizeHints:    slices.Clone(sizeHints),
			maxSizeHints: slices.Clone(maxSizeHints),
			typeHints:    slices.Clone(typeHints),
		}}
		tc.hintedDescriptors[key] = append(tc.hintedDescriptors[key], variant)
		tc.pendingKeys = append(tc.pendingKeys, pendingKey{key: key, hinted: true})
	}

	// A build that involved a recursive cycle has descriptors that read
	// in-progress children; re-derive the child-propagated flags now that the
	// graph is complete. Non-recursive builds are already exact.
	if topLevel && tc.recursion {
		FixupRecursiveFlags(desc)
	}

	return desc, nil
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

// buildTypeDescriptor computes a type descriptor for a (runtime, schema) type pair.
//
// When runtimeType == schemaType, this produces a standard descriptor.
// When they differ (view descriptor scenario), the schema type defines the SSZ layout
// (field order, tags, annotations) while the runtime type provides the actual data storage.
//
// The descriptor's Type field stores the runtime type (where data lives), while the
// SSZ structure (fields, sizes, limits) is derived from the schema type.
//
// desc is pre-allocated by the caller (getTypeDescriptor) and already carries
// Type/SchemaType, so a recursive reference registered before this call can
// back-patch to it; this function populates the rest and returns the same
// pointer.
//
//nolint:gocyclo // SSZ type descriptor builder is inherently complex
func (tc *TypeCache) buildTypeDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	// Verify runtime and schema types have compatible base kinds
	if runtimeType != schemaType {
		if runtimeType.Kind() != schemaType.Kind() {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "incompatible types: runtime kind %v != schema kind %v", runtimeType.Kind(), schemaType.Kind())
		}

		var view any
		if schemaType.Kind() == reflect.Ptr {
			view = reflect.Zero(schemaType).Interface()
		} else {
			view = reflect.Zero(reflect.PointerTo(schemaType)).Interface()
		}
		desc.CodegenInfo = &view
		desc.GoTypeFlags |= GoTypeFlagIsView
	}

	// Handle pointer types - dereference both runtime and schema
	if schemaType.Kind() == reflect.Ptr {
		desc.GoTypeFlags |= GoTypeFlagIsPointer
		schemaType = schemaType.Elem()
		runtimeType = runtimeType.Elem()

		if runtimeType != schemaType && runtimeType.Kind() != schemaType.Kind() {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "incompatible pointer types: runtime kind %v != schema kind %v", runtimeType.Kind(), schemaType.Kind())
		}
	}

	// Use schema type for determining the SSZ layout
	t := schemaType

	// Track whether hints were provided externally (from field tags) rather than
	// from the annotation registry. External hints override the type's own annotation,
	// so we must not delegate to generated methods that have the annotation baked in.
	hasExternalHints := len(sizeHints) > 0 || len(maxSizeHints) > 0

	// staticAnnotation captures the type's own ssz-static:"true/false" declaration
	// (true = fixed-size, false = variable-size). It gates the shallow-build path
	// for fully-delegated types below.
	var staticAnnotation *bool

	// Check annotation registry for type-level metadata when no external hints provided
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
		if tag, ok := sszutils.LookupAnnotation(t); ok {
			var parseErr error

			typeHints, sizeHints, maxSizeHints, parseErr = ParseTags(tag)
			if parseErr != nil {
				return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "failed to parse annotation for type %v: %v", t, parseErr)
			}

			if staticStr, hasStatic := reflect.StructTag(tag).Lookup("ssz-static"); hasStatic {
				switch staticStr {
				case "true":
					v := true
					staticAnnotation = &v
				case "false":
					v := false
					staticAnnotation = &v
				default:
					return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "invalid ssz-static value %q for type %v (must be \"true\" or \"false\")", staticStr, t)
				}
			}

			// ParseTags can't resolve dynamic expressions (no DynamicSpecs).
			// Resolve them now using tc.specs.
			if tc.specs != nil {
				// A dimension keeps its static value when the expression gives
				// nothing usable -- resolved to zero, undefined, or unresolvable.
				// A zero static value is the "0" placeholder rather than a
				// fallback, so there is nothing left to fall back to and the
				// annotation names a size or limit nothing supplies. The
				// generated code reports the same dead end at runtime, where its
				// expressions resolve (ResolveSpecValueWithDefault).
				for i := range sizeHints {
					if sizeHints[i].Expr == "" {
						continue
					}

					ok, val, resolveErr := tc.specs.ResolveSpecValue(sizeHints[i].Expr)
					if resolveErr == nil && ok && val > 0 {
						// The range check guards the conversion directly rather
						// than standing as a separate condition, so that what
						// makes the narrowing safe is visible at the narrowing.
						if val > math.MaxUint32 {
							return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "ssz-size value %d exceeds the uint32 size range", val)
						}

						sizeHints[i].Size = uint32(val)
						sizeHints[i].Custom = true

						continue
					}
					if sizeHints[i].Size == 0 {
						return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "ssz-size expression %q %s", sizeHints[i].Expr, unresolvedReason(ok, resolveErr))
					}
				}

				for i := range maxSizeHints {
					if maxSizeHints[i].Expr == "" {
						continue
					}

					ok, val, resolveErr := tc.specs.ResolveSpecValue(maxSizeHints[i].Expr)
					if resolveErr == nil && ok && val > 0 {
						maxSizeHints[i].Size = val
						maxSizeHints[i].Custom = true

						continue
					}
					if maxSizeHints[i].Size == 0 {
						return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "ssz-max expression %q %s", maxSizeHints[i].Expr, unresolvedReason(ok, resolveErr))
					}
				}
			}
		}
	}

	desc.Kind = t.Kind()

	// check dynamic size and max size
	if len(sizeHints) > 0 {
		if sizeHints[0].Expr != "" {
			desc.SizeExpression = &sizeHints[0].Expr
		}
		if sizeHints[0].Bits {
			desc.SszTypeFlags |= SszTypeFlagHasBitSize
		}
		for _, hint := range sizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= SszTypeFlagHasDynamicSize
			}

			if hint.Expr != "" {
				desc.SszTypeFlags |= SszTypeFlagHasSizeExpr
			}
		}
	}

	if len(maxSizeHints) > 0 {
		// A resolved limit of 0 is a "no limit" placeholder (ssz-max:"0" with the
		// real limit supplied dynamically via dynssz-max). Spec values are already
		// resolved at this point, so only treat a positive limit as a constraint.
		if !maxSizeHints[0].NoValue && maxSizeHints[0].Size > 0 {
			desc.SszTypeFlags |= SszTypeFlagHasLimit
			desc.Limit = maxSizeHints[0].Size
		}

		if maxSizeHints[0].Expr != "" {
			desc.MaxExpression = &maxSizeHints[0].Expr
		}

		for _, hint := range maxSizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= SszTypeFlagHasDynamicMax
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= SszTypeFlagHasMaxExpr
			}
		}
	}

	// determine ssz type
	sszType := SszUnspecifiedType
	if len(typeHints) > 0 {
		sszType = typeHints[0].Type
	}

	if desc.Kind == reflect.String {
		desc.GoTypeFlags |= GoTypeFlagIsString
	}
	if t.PkgPath() == "time" && t.Name() == "Time" {
		desc.GoTypeFlags |= GoTypeFlagIsTime
	}

	// auto-detect ssz type if not specified
	if sszType == SszUnspecifiedType {
		// detect some well-known and widely used types
		sszType = getWellKnownExternalType(t.PkgPath(), t.Name())
	}
	if sszType == SszUnspecifiedType {
		switch desc.Kind {
		// basic types
		case reflect.Bool:
			sszType = SszBoolType
		case reflect.Uint8:
			sszType = SszUint8Type
		case reflect.Uint16:
			sszType = SszUint16Type
		case reflect.Uint32:
			sszType = SszUint32Type
		case reflect.Uint64:
			sszType = SszUint64Type

		// complex types
		case reflect.Struct:
			sszType = SszContainerType
		case reflect.Array:
			sszType = SszVectorType
		case reflect.Slice:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = SszVectorType
			} else if err := rejectZeroSizeHint(sizeHints); err != nil {
				return nil, err
			} else {
				sszType = SszListType
			}
		case reflect.String:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = SszVectorType
			} else if err := rejectZeroSizeHint(sizeHints); err != nil {
				return nil, err
			} else {
				sszType = SszListType
			}

		// extended types (not supported by SSZ spec)
		case reflect.Int8:
			sszType = SszInt8Type
		case reflect.Int16:
			sszType = SszInt16Type
		case reflect.Int32:
			sszType = SszInt32Type
		case reflect.Int64:
			sszType = SszInt64Type
		case reflect.Float32:
			sszType = SszFloat32Type
		case reflect.Float64:
			sszType = SszFloat64Type

		// unsupported types
		case reflect.Int, reflect.Uint:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "signed or unsigned integers with unspecified size are not supported in SSZ")
		case reflect.Complex64, reflect.Complex128:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "complex numbers are not supported in SSZ (use unsigned integers instead)")
		case reflect.Map:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "maps are not supported in SSZ (use structs or arrays instead)")
		case reflect.Chan:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "channels are not supported in SSZ")
		case reflect.Func:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "functions are not supported in SSZ")
		case reflect.Interface:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "interfaces are not supported in SSZ (use concrete types)")
		case reflect.UnsafePointer:
			return nil, sszutils.NewSszError(sszutils.ErrUnsupportedType, "unsafe pointers are not supported in SSZ")
		default:
			break
		}

		// special case for bitlists
		if sszType == SszListType && strings.Contains(t.Name(), "Bitlist") {
			sszType = SszBitlistType
		}
	}

	desc.SszType = sszType

	// Fully-delegated types handle every SSZ operation through their own generated
	// code, so the descriptor subtree below them is never consulted. When such a
	// type also declares whether it is static (fixed-size) or dynamic via its own
	// ssz-static annotation, build a shallow descriptor and skip recursing into —
	// and validating — the subtree. For a static type the fixed byte size is read
	// straight from the type's own sizer (the authoritative, spec-correct source)
	// applied to a zero value. Field-level hints (hasExternalHints) opt out, since
	// they override the type's own annotation and require inline processing. View
	// descriptors qualify when they delegate through the dynamic view interface set.
	if staticAnnotation != nil && !hasExternalHints && !tc.NoDelegation {
		var fullyDelegated bool
		if desc.GoTypeFlags&GoTypeFlagIsView != 0 {
			fullyDelegated = fullyDelegatesSSZView(runtimeType)
		} else {
			fullyDelegated = fullyDelegatesSSZ(runtimeType)
		}
		if fullyDelegated {
			if *staticAnnotation {
				size, err := tc.delegatedStaticSize(desc, runtimeType)
				if err != nil {
					return nil, err
				}
				desc.Size = size
			} else {
				desc.SszTypeFlags |= SszTypeFlagIsDynamic
			}
			tc.detectCompatFlags(desc, runtimeType, schemaType)
			// Without traversal the descriptor cannot know whether the type uses
			// spec-dependent sizes, so it must not fall back to the type's fastssz
			// methods (which bake spec-independent values). The shallow path is
			// only taken for types that delegate through the spec-aware dynamic
			// (or dynamic view) interfaces, so suppress the fastssz family and let
			// those handle every operation correctly.
			desc.SszCompatFlags &^= SszCompatFlagFastSSZMarshaler | SszCompatFlagFastSSZHasher | SszCompatFlagHashTreeRootWith
			desc.HashTreeRootWithMethod = nil
			// A shallow descriptor has no traversed subtree: a static one still
			// knows its size, a dynamic one states no floor.
			setMinSize(desc)

			return desc, nil
		}
	}

	// A tag names one dimension per level of nesting, so a type that has no
	// element consumes the last of them. Anything past that describes a
	// dimension the type does not have: it was parsed, then dropped, which
	// leaves a tag that reads as if it did something.
	if len(typeHints) > 1 && !consumesDimension(sszType) {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidTag,
			"ssz-type declares %d dimensions for %v, which takes 1: drop the trailing %d",
			len(typeHints), t, len(typeHints)-1)
	}

	// Check type compatibility and compute size
	switch sszType {
	case SszUnspecifiedType:
		return nil, sszutils.NewSszErrorf(sszutils.ErrUnsupportedType, "unsupported type kind: %v", t.Kind())

	// basic types
	case SszBoolType:
		if desc.Kind != reflect.Bool {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bool ssz type can only be represented by bool types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "bool ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "bool ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint8 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint8 ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint16 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 2 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint16 ssz type must be ssz-size:2, got %v", sizeHints[0].Size)
		}
		desc.Size = 2
	case SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint32 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 4 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint32 ssz type must be ssz-size:4, got %v", sizeHints[0].Size)
		}
		desc.Size = 4
	case SszUint64Type:
		if desc.Kind != reflect.Uint64 && desc.GoTypeFlags&GoTypeFlagIsTime == 0 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint64 ssz type can only be represented by uint64 or time.Time types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint64 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
		}
		desc.Size = 8
	case SszUint128Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint128 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := tc.buildUintDescriptor(desc, t, 16, "uint128") // handle as [16]uint8 or [2]uint64
		if err != nil {
			return nil, err
		}
	case SszUint256Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint256 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := tc.buildUintDescriptor(desc, t, 32, "uint256") // handle as [32]uint8 or [4]uint64
		if err != nil {
			return nil, err
		}

	// complex types
	case SszTypeWrapperType:
		err := tc.buildTypeWrapperDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return nil, err
		}
	case SszContainerType, SszProgressiveContainerType:
		err := tc.buildContainerDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return nil, err
		}
	case SszVectorType, SszBitvectorType:
		err := tc.buildVectorDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszListType, SszBitlistType, SszProgressiveListType, SszProgressiveBitlistType:
		err := tc.buildListDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszCompatibleUnionType:
		err := tc.buildCompatibleUnionDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return nil, err
		}
	case SszUnionType:
		err := tc.buildUnionDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return nil, err
		}
	case SszCustomType:
		// A custom type serializes entirely through its own methods, so its Go
		// structure is never traversed and no child descriptors are built. It is
		// variable-size by default; an explicit ssz-size hint or an
		// ssz-static:"true" annotation pins a fixed size (read from the type's own
		// sizer), while ssz-static:"false" keeps it dynamic.
		switch {
		case len(sizeHints) > 0 && sizeHints[0].Size > 0:
			desc.Size = sizeHints[0].Size
		case staticAnnotation != nil && *staticAnnotation:
			size, err := tc.delegatedStaticSize(desc, runtimeType)
			if err != nil {
				return nil, err
			}
			desc.Size = size
		default:
			desc.Size = 0
			desc.SszTypeFlags |= SszTypeFlagIsDynamic
		}

	// extended types (not supported by SSZ spec)
	case SszInt8Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int8 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int8 ssz type can only be represented by int8 types, got %v", desc.Kind)
		}
		desc.Size = 1
	case SszInt16Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int16 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int16 ssz type can only be represented by int16 types, got %v", desc.Kind)
		}
		desc.Size = 2
	case SszInt32Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int32 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int32 ssz type can only be represented by int32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case SszInt64Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int64 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int64 ssz type can only be represented by int64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case SszFloat32Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float32 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "float32 ssz type can only be represented by float32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case SszFloat64Type:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float64 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "float64 ssz type can only be represented by float64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case SszOptionalType:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "optional types are not supported in SSZ (use extended types option to enable it)")
		}
		err := tc.buildOptionalDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszOptionalListType:
		// optional-list expresses a pointer as a canonical List[T, 1]; allowed without ExtendedTypes
		err := tc.buildOptionalListDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case SszBigIntType:
		if !tc.ExtendedTypes {
			return nil, sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "big integers are not supported in SSZ (use extended types option to enable it)")
		}
		err := tc.buildBigIntDescriptor(desc)
		if err != nil {
			return nil, err
		}
	}

	if desc.SszTypeFlags&SszTypeFlagHasBitSize != 0 && desc.SszType != SszBitvectorType && desc.SszType != SszBitlistType {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "bit size tag is only allowed for bitvector or bitlist types, got %v", desc.SszType)
	}

	tc.detectCompatFlags(desc, runtimeType, schemaType)

	// A plain container that only satisfies the delegation interfaces through a
	// method promoted from an embedded field must not delegate: the promoted
	// method serializes just the embedded field and drops the container's other
	// fields. Walk it as a container instead — the embedded field still
	// delegates correctly as one of the walked fields. Custom types are exempt
	// (they are opaque and have no walkable layout).
	if (desc.SszType == SszContainerType || desc.SszType == SszProgressiveContainerType) &&
		tc.InheritsPromotedDelegation(runtimeType) {
		desc.SszCompatFlags &^= SszCompatFlagDynamicMarshaler |
			SszCompatFlagDynamicUnmarshaler |
			SszCompatFlagDynamicSizer |
			SszCompatFlagDynamicHashRoot |
			SszCompatFlagDynamicEncoder |
			SszCompatFlagDynamicDecoder |
			SszCompatFlagFastSSZMarshaler |
			SszCompatFlagFastSSZHasher |
			SszCompatFlagHashTreeRootWith
		desc.HashTreeRootWithMethod = nil
	}

	// When field-level hints override the type's own annotation, don't delegate
	// to the type's generated methods — they have the annotation's limits baked in.
	// Process inline instead so the field-level hints are respected.
	if hasExternalHints && desc.SszType != SszCustomType {
		desc.SszCompatFlags &^= SszCompatFlagDynamicMarshaler |
			SszCompatFlagDynamicUnmarshaler |
			SszCompatFlagDynamicSizer |
			SszCompatFlagDynamicHashRoot |
			SszCompatFlagDynamicEncoder |
			SszCompatFlagDynamicDecoder |
			SszCompatFlagFastSSZMarshaler |
			SszCompatFlagFastSSZHasher |
			SszCompatFlagHashTreeRootWith
	}

	// Optional and optional-list reshape the encoding around the inner type
	// (presence byte / List[T,1] framing). The inner type's own SSZ methods
	// must not be invoked at this level — they would skip the framing and
	// emit the inner value as if it were the canonical encoding.
	if desc.SszType == SszOptionalType || desc.SszType == SszOptionalListType {
		desc.SszCompatFlags &^= SszCompatFlagDynamicMarshaler |
			SszCompatFlagDynamicUnmarshaler |
			SszCompatFlagDynamicSizer |
			SszCompatFlagDynamicHashRoot |
			SszCompatFlagDynamicEncoder |
			SszCompatFlagDynamicDecoder |
			SszCompatFlagFastSSZMarshaler |
			SszCompatFlagFastSSZHasher |
			SszCompatFlagHashTreeRootWith
	}

	// Per the SSZ spec, containers (including progressive containers) must have
	// at least one field. Reject a struct that would be encoded field-by-field
	// with no SSZ-encodable (exported) fields. Types that delegate to their own
	// SSZ methods (any compat flag set) are exempt: they do not use the plain
	// container layout, so a zero-field struct shell is legitimate for them.
	if desc.SszType == SszContainerType || desc.SszType == SszProgressiveContainerType {
		if desc.SszCompatFlags == 0 && desc.ContainerDesc != nil && len(desc.ContainerDesc.Fields) == 0 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "container type %v has no SSZ fields, which is invalid per the SSZ spec", schemaType)
		}
	}

	if desc.SszType == SszCustomType {
		// A custom type delegates every SSZ operation to its own methods. Each
		// operation may be served by either the fastssz method or the dynssz
		// (Dynamic*) equivalent, but at least one implementation per operation is
		// required. The fastssz marshaler interface bundles marshal, unmarshal and
		// size; the fastssz hasher covers the hash tree root.
		f := desc.SszCompatFlags
		var missing []string
		if f&(SszCompatFlagFastSSZMarshaler|SszCompatFlagDynamicMarshaler|SszCompatFlagDynamicEncoder) == 0 {
			missing = append(missing, "marshaler")
		}
		if f&(SszCompatFlagFastSSZMarshaler|SszCompatFlagDynamicUnmarshaler|SszCompatFlagDynamicDecoder) == 0 {
			missing = append(missing, "unmarshaler")
		}
		if f&(SszCompatFlagFastSSZMarshaler|SszCompatFlagDynamicSizer) == 0 {
			missing = append(missing, "sizer")
		}
		if f&(SszCompatFlagFastSSZHasher|SszCompatFlagHashTreeRootWith|SszCompatFlagDynamicHashRoot) == 0 {
			missing = append(missing, "hasher")
		}
		if len(missing) > 0 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "custom ssz type %v is missing a fastssz or dynssz %s implementation", schemaType, strings.Join(missing, ", "))
		}
	}

	setMinSize(desc)

	return desc, nil
}

// unresolvedReason says why an expression produced no usable value, so the
// error tells the reader whether to define the spec value or to correct it.
func unresolvedReason(resolved bool, err error) string {
	switch {
	case err != nil:
		return fmt.Sprintf("could not be resolved (%v) and has no positive static fallback", err)
	case resolved:
		return "resolved to 0 with no positive static fallback"
	default:
		return "is not defined and has no positive static fallback"
	}
}

// setMinSize records the smallest number of bytes a value of this type can
// serialize to. Decoders use it to reject an offset table that declares more
// elements than the region can hold, before that count sizes an allocation.
//
// A type with no floor -- a list, a union, an optional -- keeps 0, which states
// no bound rather than a wrong one.
//
// It is stored rather than derived at decode time because Len carries different
// meanings per type (bytes for a container's fixed section, elements for a
// vector), so the rule would otherwise be re-derived on every decode. Children
// are read from their own cached value, so a recursive type resolves without
// walking back into itself: a cycle's back edge simply contributes nothing.
//
// Only this cache fills it in, where every size is already resolved against the
// active spec. Descriptors built by the code generator's go/types parser keep 0:
// at generation time only the static tag values are known, and freezing those
// into a bound would refuse valid input under a preset that resolves them
// smaller. The generator emits the same minimum as a runtime expression instead
// (see minSizeExpr).
func setMinSize(desc *TypeDescriptor) {
	// A static type serializes to exactly its size.
	if desc.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
		desc.MinSize = desc.Size
		return
	}

	switch desc.SszType {
	case SszTypeWrapperType:
		if desc.ElemDesc != nil {
			desc.MinSize = desc.ElemDesc.MinSize
		}
	case SszContainerType, SszProgressiveContainerType:
		// The fixed section: every field's own size, and four offset bytes for
		// each dynamic one.
		desc.MinSize = desc.Len
	case SszVectorType:
		// A vector of dynamic elements leads with one 4-byte offset per element,
		// and every element costs at least its own minimum on top of that. Len is
		// the element count here, not a byte size.
		if desc.ElemDesc != nil {
			// An overflowing product would bound the region above the true floor
			// and refuse valid input, so it states no bound instead.
			minSize := uint64(desc.Len) * (4 + uint64(desc.ElemDesc.MinSize))
			if minSize <= math.MaxUint32 {
				desc.MinSize = uint32(minSize)
			}
		}
	default:
		// Everything else can serialize to nothing -- an empty list, an absent
		// optional, a union's smallest variant -- so it states no floor.
	}
}

// detectCompatFlags records which SSZ delegation interfaces (fastssz, dynamic,
// dynamic-view, and HashTreeRootWith) the type implements. The fastssz marshaler
// and hasher are only flagged when the type does not carry a dynamic size/max,
// since those use the static fastssz layout.
func (tc *TypeCache) detectCompatFlags(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) {
	if desc.SszTypeFlags&SszTypeFlagHasDynamicSize == 0 && getFastsszConvertCompatibility(runtimeType) {
		desc.SszCompatFlags |= SszCompatFlagFastSSZMarshaler
	}
	if desc.SszTypeFlags&SszTypeFlagHasDynamicMax == 0 {
		if getFastsszHashCompatibility(runtimeType) {
			desc.SszCompatFlags |= SszCompatFlagFastSSZHasher
		}
		if method := getHashTreeRootWithCompatibility(runtimeType); method != nil {
			desc.HashTreeRootWithMethod = method
			desc.SszCompatFlags |= SszCompatFlagHashTreeRootWith
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

// fullyDelegatesSSZ reports whether the type implements the complete set of
// dynamic SSZ operation interfaces (marshal, unmarshal, size, hash-tree-root).
// When it does, every SSZ operation is handled by the type's own generated code
// and the descriptor subtree below it is never consulted, so it does not need to
// be built or validated — provided the type also declares its size via an
// annotation (see the shallow-build path in buildTypeDescriptor).
func fullyDelegatesSSZ(runtimeType reflect.Type) bool {
	return (getDynamicMarshalerCompatibility(runtimeType) || getDynamicEncoderCompatibility(runtimeType)) &&
		(getDynamicUnmarshalerCompatibility(runtimeType) || getDynamicDecoderCompatibility(runtimeType)) &&
		getDynamicSizerCompatibility(runtimeType) &&
		getDynamicHashRootCompatibility(runtimeType)
}

// fullyDelegatesSSZView is the view-descriptor counterpart of fullyDelegatesSSZ:
// it reports whether the type implements the complete set of dynamic view SSZ
// operation interfaces, in which case a view descriptor also delegates every
// operation to the type's own code and its subtree need not be built.
func fullyDelegatesSSZView(runtimeType reflect.Type) bool {
	return (getDynamicViewMarshalerCompatibility(runtimeType) || getDynamicViewEncoderCompatibility(runtimeType)) &&
		(getDynamicViewUnmarshalerCompatibility(runtimeType) || getDynamicViewDecoderCompatibility(runtimeType)) &&
		getDynamicViewSizerCompatibility(runtimeType) &&
		getDynamicViewHashRootCompatibility(runtimeType)
}

// delegatedStaticSize returns the fixed SSZ byte size of a fully-delegated static
// type by invoking the type's own sizer on a zero value. A static type's sizer
// derives its result from constants and spec values only — never from field data
// — so a zero value yields the correct size, and spec-dependent fixed sizes are
// resolved against the cache's specs. View descriptors use the view sizer.
func (tc *TypeCache) delegatedStaticSize(desc *TypeDescriptor, runtimeType reflect.Type) (uint32, error) {
	specs := tc.specs // never nil: NewTypeCache substitutes emptySpecs{}
	zero := reflect.New(runtimeType).Interface()

	// A sizer returns int; a negative or >uint32 value would wrap on conversion
	// and corrupt downstream sizing/offset math, so validate the range.
	validate := func(n int) (uint32, error) {
		if n < 0 || int64(n) > math.MaxUint32 {
			return 0, sszutils.NewSszErrorf(sszutils.ErrInvalidValueRange, "sizer for static type %v returned out-of-range size %d", runtimeType, n)
		}
		return uint32(n), nil
	}

	if desc.GoTypeFlags&GoTypeFlagIsView != 0 {
		if sizer, ok := zero.(sszutils.DynamicViewSizer); ok && desc.CodegenInfo != nil {
			if sizeFn := sizer.SizeSSZDynView(*desc.CodegenInfo); sizeFn != nil {
				return validate(sizeFn(specs))
			}
		}
		return 0, sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "static view type %v does not provide a usable view sizer", runtimeType)
	}

	// Non-view static types may size themselves through either the dynssz sizer
	// (spec-aware) or the fastssz sizer. Fully-delegated callers always provide
	// the DynamicSizer; a static custom type may instead provide only fastssz.
	if sizer, ok := zero.(sszutils.DynamicSizer); ok {
		return validate(sizer.SizeSSZDyn(specs))
	}
	if marshaler, ok := zero.(sszutils.FastsszMarshaler); ok {
		return validate(marshaler.SizeSSZ())
	}
	return 0, sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "static type %v provides no usable sizer", runtimeType)
}

// buildTypeWrapperDescriptor builds a descriptor for TypeWrapper types with runtime/schema pairing.
//
// For TypeWrappers, the wrapped type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ annotations while the runtime type provides actual data access.
func (tc *TypeCache) buildTypeWrapperDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "TypeWrapper ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	// Extract schema wrapper information (determines SSZ layout)
	var schemaDescriptorType reflect.Type
	var isTypeWrapper = false

	schemaWrapperValue := reflect.New(schemaType)
	schemaMethod := schemaWrapperValue.MethodByName("GetDescriptorType")
	if schemaMethod.IsValid() {
		schemaResults := schemaMethod.Call(nil)
		if len(schemaResults) == 0 {
			return sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType returned no results for schema type")
		}

		var ok bool
		schemaDescriptorType, ok = schemaResults[0].Interface().(reflect.Type)
		if !ok {
			return sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType did not return a reflect.Type for schema type")
		}

		isTypeWrapper = true
	} else {
		schemaDescriptorType = schemaType
	}

	// Extract wrapper information from schema descriptor (includes SSZ annotations)
	schemaWrapperInfo, err := extractWrapperDescriptorInfo(schemaDescriptorType, tc.specs)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(wrapper)")
	}

	// Determine runtime wrapped type
	var runtimeWrappedType reflect.Type
	if runtimeType != schemaType {
		// Extract runtime wrapper information for the wrapped type
		var runtimeDescriptorType reflect.Type

		if isTypeWrapper {
			runtimeWrapperValue := reflect.New(runtimeType)
			runtimeMethod := runtimeWrapperValue.MethodByName("GetDescriptorType")
			if !runtimeMethod.IsValid() {
				return sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "GetDescriptorType method not found on runtime type %s", runtimeType)
			}

			runtimeResults := runtimeMethod.Call(nil)
			if len(runtimeResults) == 0 {
				return sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType returned no results for runtime type")
			}

			var ok bool
			runtimeDescriptorType, ok = runtimeResults[0].Interface().(reflect.Type)
			if !ok {
				return sszutils.NewSszError(sszutils.ErrMissingInterface, "GetDescriptorType did not return a reflect.Type for runtime type")
			}
		} else {
			runtimeDescriptorType = runtimeType
		}

		// Get the wrapped type from runtime descriptor
		runtimeWrapperInfo, err2 := extractWrapperDescriptorInfo(runtimeDescriptorType, tc.specs)
		if err2 != nil {
			return sszutils.ErrorWithPath(err2, "(wrapper)")
		}
		runtimeWrappedType = runtimeWrapperInfo.Type
	} else {
		runtimeWrappedType = schemaWrapperInfo.Type
	}

	// The wrapper's actual value field must be shape-compatible with the type the
	// descriptor expects. Otherwise the reflection-driven encode/decode would call
	// type-specific operations (Bytes, Uint, Len, ...) on an incompatible value
	// and panic deep in the reflect package.
	if runtimeType.NumField() > 0 {
		valueType := runtimeType.Field(0).Type
		if !wrapperTypeCompatible(valueType, runtimeWrappedType) {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch,
				"TypeWrapper value type %v is not compatible with descriptor type %v", valueType, runtimeWrappedType)
		}
	}

	// Build type descriptor for the wrapped type traversing both type trees
	wrappedDesc, err := tc.getTypeDescriptor(runtimeWrappedType, schemaWrapperInfo.Type, schemaWrapperInfo.SizeHints, schemaWrapperInfo.MaxSizeHints, schemaWrapperInfo.TypeHints)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(wrapper)")
	}

	// Store wrapper information
	desc.ElemDesc = wrappedDesc

	// The TypeWrapper inherits properties from the wrapped type
	desc.Size = wrappedDesc.Size
	desc.SszTypeFlags |= wrappedDesc.SszTypeFlags & (SszTypeFlagIsDynamic | SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	return nil
}

// wrapperTypeCompatible reports whether a TypeWrapper's actual value type can be
// driven by a descriptor built for expected. Named types over the same
// underlying kind are accepted; differing kinds, element types, or array lengths
// are rejected so an incompatible wrapper fails with a clean error instead of a
// reflect panic.
func wrapperTypeCompatible(actual, expected reflect.Type) bool {
	if actual == expected {
		return true
	}
	if actual.Kind() != expected.Kind() {
		return false
	}
	switch actual.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.String:
		// Named types over the same scalar/string kind are interchangeable.
		return true
	case reflect.Array:
		return actual.Len() == expected.Len() && wrapperTypeCompatible(actual.Elem(), expected.Elem())
	case reflect.Slice, reflect.Pointer:
		return wrapperTypeCompatible(actual.Elem(), expected.Elem())
	default:
		// Composite kinds (struct, map, interface, ...) with a matching kind but
		// different element/field layout are not interchangeable; only the exact
		// same type (handled above) is compatible.
		return false
	}
}

// buildUint128Descriptor builds a descriptor for uint128 types
func (tc *TypeCache) buildUintDescriptor(desc *TypeDescriptor, t reflect.Type, byteLen uint32, typeName string) error {
	if desc.Kind != reflect.Slice && desc.Kind != reflect.Array {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "%s ssz type can only be represented by slice or array types, got %v", typeName, desc.Kind)
	}

	fieldType := t.Elem()
	elemKind := fieldType.Kind()
	if elemKind != reflect.Uint8 && elemKind != reflect.Uint64 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "%s ssz type can only be represented by slices or arrays of uint8 or uint64, got %v", typeName, elemKind)
	} else if elemKind == reflect.Uint8 {
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	}

	elemDesc, err := tc.getTypeDescriptor(fieldType, fieldType, nil, nil, nil)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.Size = byteLen
	desc.Len = desc.Size / elemDesc.Size

	if desc.Kind == reflect.Array {
		dstLen := uint32(t.Len())
		// A fixed-width uint (uint128/uint256) occupies exactly desc.Len array
		// elements. A smaller array cannot hold it; a larger array carries trailing
		// elements that marshal silently drops (truncating to desc.Len) while
		// HashTreeRoot rejects the length mismatch — an inconsistency the codegen
		// parser already refuses. Reject both here so the reflection path agrees.
		// Unlike a Vector/Bitvector (where an oversized backing array is a valid
		// preset pattern), a uint width is intrinsic and never preset-dependent.
		if dstLen < desc.Len {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "%s ssz type does not fit in array (%d < %d)", typeName, dstLen, desc.Len)
		}
		if dstLen > desc.Len {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "%s ssz type does not fill the array (%d > %d): trailing elements would be dropped", typeName, dstLen, desc.Len)
		}
	}

	return nil
}

// buildContainerDescriptor builds a descriptor for ssz container types with runtime/schema pairing.
//
// This method supports fork-dependent SSZ schemas (view descriptors) where the schema type
// defines the SSZ layout (field order, tags, limits) while the runtime type holds the actual data.
//
// When runtimeType == schemaType, this behaves identically to the original buildContainerDescriptor.
// When they differ, the method:
//   - Iterates over schema fields to define SSZ layout
//   - Maps each schema field to the corresponding runtime field by name
//   - Stores the runtime field index in FieldIndex for direct field access
//   - Builds child descriptors using (runtimeFieldType, schemaFieldType) pairs
//
// Parameters:
//   - desc: The TypeDescriptor being built
//   - runtimeType: The type where actual data lives (must be a struct)
//   - schemaType: The type that defines SSZ layout (must be a struct)
//
// Returns:
//   - error: An error if schema fields cannot be mapped to runtime fields
func (tc *TypeCache) buildContainerDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	if desc.Kind != reflect.Struct {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "container ssz type can only be represented by struct types, got %v", desc.Kind)
	}

	// Determine if runtime and schema types differ (view descriptor mode)
	isViewDescriptor := runtimeType != schemaType

	// Pre-build a map of runtime field names to indices for efficient lookup in view descriptor mode
	var runtimeFieldMap map[string]int
	if isViewDescriptor {
		runtimeFieldMap = make(map[string]int, runtimeType.NumField())
		for i := 0; i < runtimeType.NumField(); i++ {
			runtimeFieldMap[runtimeType.Field(i).Name] = i
		}
	}

	schemaFieldCount := schemaType.NumField()

	// Unexported fields are skipped entirely (Go convention, matching
	// encoding/json and gob): they cannot be assigned via reflection on decode
	// and have no place in the SSZ layout. The descriptor only tracks exported
	// fields, while FieldIndex keeps the real reflect index for data access.
	exportedFieldCount := 0
	for i := 0; i < schemaFieldCount; i++ {
		f := schemaType.Field(i)
		if f.IsExported() && !IsSszExcluded(f.Tag) {
			exportedFieldCount++
		}
	}

	desc.ContainerDesc = &ContainerDescriptor{
		Fields:    make([]FieldDescriptor, exportedFieldCount),
		DynFields: make([]DynFieldDescriptor, 0),
	}

	totalSize := uint32(0)
	isDynamic := false

	// Check for progressive container detection
	hasAnyIndexTag := false
	var fieldIndices map[uint16]struct{}
	var sszIndexes []*uint16

	// Iterate over schema fields - they define the SSZ layout. fi is the
	// compacted position within the descriptor's exported-only field slices.
	fi := 0
	for i := 0; i < schemaFieldCount; i++ {
		schemaField := schemaType.Field(i)
		if !schemaField.IsExported() || IsSszExcluded(schemaField.Tag) {
			continue
		}
		fieldDesc := FieldDescriptor{
			Name: schemaField.Name,
		}

		// Resolve the corresponding runtime field
		var runtimeFieldIndex int

		if isViewDescriptor {
			// In view descriptor mode, map schema field to runtime field by name
			idx, found := runtimeFieldMap[schemaField.Name]
			if !found {
				return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "schema field %q not found in runtime type %s", schemaField.Name, runtimeType.Name())
			}
			runtimeFieldIndex = idx
		} else {
			// When schema == runtime, field indices match directly
			runtimeFieldIndex = i
		}

		// Store the runtime field index for direct field access during encode/decode/hash
		fieldDesc.FieldIndex = uint16(runtimeFieldIndex)
		runtimeFieldType := runtimeType.Field(runtimeFieldIndex).Type

		// Get ssz-index tag from schema field (for progressive containers)
		sszIndex, err := getSszIndexTag(&schemaField)
		if err != nil {
			return sszutils.ErrorWithPath(err, schemaField.Name)
		}

		if sszIndex != nil {
			if sszIndexes == nil {
				sszIndexes = make([]*uint16, exportedFieldCount)
				fieldIndices = make(map[uint16]struct{}, exportedFieldCount)
			}
			sszIndexes[fi] = sszIndex
			fieldDesc.SszIndex = *sszIndex
			hasAnyIndexTag = true
			if _, exists := fieldIndices[*sszIndex]; exists {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "duplicate ssz-index %d found in field %s", *sszIndex, schemaField.Name)
			}
			fieldIndices[*sszIndex] = struct{}{}
		}

		// Field-level tags override the type's registered annotation per key:
		// join the two (field tag first — Lookup returns the first occurrence)
		// so annotation keys the field does not override still apply.
		if annTag, ok := sszutils.LookupAnnotation(schemaField.Type); ok {
			schemaField.Tag = JoinFieldAnnotationTag(schemaField.Tag, annTag)
		}

		// Get size hints from schema field tags (schema defines SSZ constraints)
		sizeHints, err := getSszSizeTag(tc.specs, &schemaField)
		if err != nil {
			return sszutils.ErrorWithPath(err, schemaField.Name)
		}

		maxSizeHints, err := getSszMaxSizeTag(tc.specs, &schemaField)
		if err != nil {
			return sszutils.ErrorWithPath(err, schemaField.Name)
		}

		typeHints, err := getSszTypeTag(&schemaField)
		if err != nil {
			return sszutils.ErrorWithPath(err, schemaField.Name)
		}

		// Build child type descriptor using (runtimeFieldType, schemaFieldType) pair.
		// This is the key to supporting nested view descriptors: the schema field type
		// may itself be a view type that differs from the runtime field type.
		schemaFieldType := schemaField.Type
		fieldDesc.Type, err = tc.getTypeDescriptor(runtimeFieldType, schemaFieldType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return sszutils.ErrorWithPath(err, schemaField.Name)
		}

		sszSize := fieldDesc.Type.Size
		if fieldDesc.Type.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
			isDynamic = true
			sszSize = 4 // Offset size for dynamic fields

			desc.ContainerDesc.DynFields = append(desc.ContainerDesc.DynFields, DynFieldDescriptor{
				Field:        &desc.ContainerDesc.Fields[fi],
				HeaderOffset: totalSize,
				Index:        int16(runtimeFieldIndex), // Use runtime field index for data access
			})
		}

		desc.SszTypeFlags |= fieldDesc.Type.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)
		// SSZ sizes are uint32; a wrapped sum would defeat the fixed-section
		// length checks that are derived from it.
		if totalSize > math.MaxUint32-sszSize {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidValueRange, "container byte size exceeds the uint32 SSZ size range")
		}
		totalSize += sszSize
		desc.ContainerDesc.Fields[fi] = fieldDesc
		fi++
	}

	// A container is progressive if it carries ssz-index annotations on fields or
	// was declared progressive via the ssz-type hint. Assign the active-field
	// indices: a tagged field keeps its (strictly increasing) index; an untagged
	// field takes the previous field's index + 1. With no tags at all this yields
	// the default 0, 1, 2, ... sequence, so a progressive container never falls
	// back to all-zero indices (which would emit a 1-bit active_fields bitvector
	// for N field roots — illegal per EIP-7495).
	if hasAnyIndexTag || desc.SszType == SszProgressiveContainerType {
		nextIndex := uint16(0)
		for i := 0; i < len(desc.ContainerDesc.Fields); i++ {
			field := &desc.ContainerDesc.Fields[i]
			if i < len(sszIndexes) && sszIndexes[i] != nil {
				if *sszIndexes[i] < nextIndex {
					return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "progressive container requires increasing ssz-index values (field %s has index %d, expected >= %d)",
						field.Name, *sszIndexes[i], nextIndex)
				}
				field.SszIndex = *sszIndexes[i]
			} else {
				// The auto-increment must respect the same 255 ceiling
				// getSszIndexTag enforces for explicit tags: a higher index needs
				// an active-fields bitvector wider than the 32 bytes
				// getActiveFields supports.
				if nextIndex > 255 {
					return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "ssz-index %d assigned to field %q exceeds the supported maximum of 255", nextIndex, field.Name)
				}
				field.SszIndex = nextIndex
			}
			nextIndex = field.SszIndex + 1
		}

		desc.SszType = SszProgressiveContainerType
	}

	desc.Len = totalSize
	if isDynamic {
		desc.Size = 0
		desc.SszTypeFlags |= SszTypeFlagIsDynamic
	} else {
		desc.Size = totalSize
	}

	return nil
}

// buildCompatibleUnionDescriptor builds a descriptor for CompatibleUnion types with runtime/schema pairing.
//
// For CompatibleUnion types, the variant types may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (variant order, annotations) while the runtime type provides
// the actual variant types for data access.
func (tc *TypeCache) buildCompatibleUnionDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	// CompatibleUnion is always dynamic size (1 byte for type + variable data)
	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	// Extract the schema descriptor type from the generic type parameter (determines SSZ layout)
	schemaDescriptorType, err := tc.extractGenericTypeParameter(schemaType)
	if err != nil {
		return err
	}

	// Populate union variants immediately since we have the descriptor type
	desc.UnionVariants = make(map[uint8]*TypeDescriptor)

	// Extract variant information from schema descriptor struct (includes SSZ annotations)
	schemaVariantInfo, err := extractUnionDescriptorInfo(schemaDescriptorType, tc.specs)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(union)")
	}

	// Check if we're using a view descriptor (runtime and schema types differ)
	isViewDescriptor := runtimeType != schemaType

	// Extract runtime variant info if using view descriptor
	var runtimeVariantMap map[string]reflect.Type
	if isViewDescriptor {
		runtimeDescriptorType, err := tc.extractGenericTypeParameter(runtimeType)
		if err != nil {
			return sszutils.ErrorWithPath(err, "(union)")
		}
		runtimeVariantInfo, err := extractUnionDescriptorInfo(runtimeDescriptorType, tc.specs)
		if err != nil {
			return sszutils.ErrorWithPath(err, "(union)")
		}
		// Build map of runtime variant names to types
		runtimeVariantMap = make(map[string]reflect.Type, len(runtimeVariantInfo))
		for _, info := range runtimeVariantInfo {
			runtimeVariantMap[info.Name] = info.Type
		}
	}

	// Build type descriptors for each variant using schema for layout, runtime for data
	for variantIndex, schemaInfo := range schemaVariantInfo {
		var runtimeVariantType reflect.Type
		if isViewDescriptor {
			var ok bool
			runtimeVariantType, ok = runtimeVariantMap[schemaInfo.Name]
			if !ok {
				return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "runtime union missing variant %q defined in schema", schemaInfo.Name)
			}
		} else {
			runtimeVariantType = schemaInfo.Type
		}

		variantDesc, err := tc.getTypeDescriptor(runtimeVariantType, schemaInfo.Type, schemaInfo.SizeHints, schemaInfo.MaxSizeHints, schemaInfo.TypeHints)
		if err != nil {
			return sszutils.ErrorWithPathf(err, "(variant:%d)", variantIndex)
		}

		desc.UnionVariants[variantIndex] = variantDesc
	}

	return nil
}

// buildUnionDescriptor builds a descriptor for classic spec unions with
// runtime/schema pairing. Selectors are the descriptor struct's 0-based field
// positions; a None marker declared first makes selector 0 the empty option
// (recorded via SszTypeFlagHasNoneVariant, with no variant entry).
func (tc *TypeCache) buildUnionDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	// A union is always dynamic size (1 selector byte + variable data).
	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	schemaDescriptorType, err := tc.extractGenericTypeParameter(schemaType)
	if err != nil {
		return err
	}

	schemaVariantInfo, hasNone, err := extractClassicUnionDescriptorInfo(schemaDescriptorType, tc.specs)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(union)")
	}
	if hasNone {
		desc.SszTypeFlags |= SszTypeFlagHasNoneVariant
	}

	isViewDescriptor := runtimeType != schemaType

	var runtimeVariantMap map[string]reflect.Type
	if isViewDescriptor {
		runtimeDescriptorType, err := tc.extractGenericTypeParameter(runtimeType)
		if err != nil {
			return sszutils.ErrorWithPath(err, "(union)")
		}
		runtimeVariantInfo, _, err := extractClassicUnionDescriptorInfo(runtimeDescriptorType, tc.specs)
		if err != nil {
			return sszutils.ErrorWithPath(err, "(union)")
		}
		runtimeVariantMap = make(map[string]reflect.Type, len(runtimeVariantInfo))
		for _, info := range runtimeVariantInfo {
			runtimeVariantMap[info.Name] = info.Type
		}
	}

	desc.UnionVariants = make(map[uint8]*TypeDescriptor, len(schemaVariantInfo))
	for variantIndex, schemaInfo := range schemaVariantInfo {
		var runtimeVariantType reflect.Type
		if isViewDescriptor {
			var ok bool
			runtimeVariantType, ok = runtimeVariantMap[schemaInfo.Name]
			if !ok {
				return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "runtime union missing variant %q defined in schema", schemaInfo.Name)
			}
		} else {
			runtimeVariantType = schemaInfo.Type
		}

		variantDesc, err := tc.getTypeDescriptor(runtimeVariantType, schemaInfo.Type, schemaInfo.SizeHints, schemaInfo.MaxSizeHints, schemaInfo.TypeHints)
		if err != nil {
			return sszutils.ErrorWithPathf(err, "(variant:%d)", variantIndex)
		}

		desc.UnionVariants[variantIndex] = variantDesc
	}

	return nil
}

// buildOptionalDescriptor builds a descriptor for optional types
func (tc *TypeCache) buildOptionalDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	// Optional is always dynamic size (1 byte for presence + variable data)
	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	if desc.GoTypeFlags&GoTypeFlagIsPointer == 0 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "optional ssz type can only be represented by pointer types, got %v", desc.Kind)
	}

	childSizeHints := []SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	childTypeHints := []SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	// Optional is presence-gated (dynamic), a legal recursion boundary.
	tc.dynDepth++
	elemDesc, err := tc.getTypeDescriptor(runtimeType, schemaType, childSizeHints, childMaxSizeHints, childTypeHints)
	tc.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc

	// The Optional inherits properties from the child type
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagIsDynamic | SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	return nil
}

// buildOptionalListDescriptor builds a descriptor for optional-list types.
//
// An optional-list expresses a Go pointer as a canonical SSZ List[T, 1]:
//   - nil pointer encodes as an empty list (no bytes)
//   - non-nil pointer encodes as a list with a single element
//
// Unlike SszOptionalType, this is a canonical SSZ encoding with no custom
// presence flag and is allowed regardless of the ExtendedTypes setting.
func (tc *TypeCache) buildOptionalListDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	if desc.GoTypeFlags&GoTypeFlagIsPointer == 0 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "optional-list ssz type can only be represented by pointer types, got %v", desc.Kind)
	}

	// The optional-list frames the Go pointer as List[T, 1]. That framing consumes
	// the leading ssz-type dimension (the "optional-list" hint itself), but it has
	// no size or max of its own — the list limit is fixed at 1 — so it consumes no
	// ssz-size / ssz-max dimension. The remaining size/max hints belong to the
	// pointed-to element T and must be forwarded in full, exactly as a plain
	// pointer forwards them (e.g. `*[]uint16 ssz-size:"2"` -> Vector[uint16,2]).
	// Peeling them here dropped the element's constraint and mis-classified a
	// fixed inner vector as a variable list.
	childSizeHints := sizeHints

	childMaxSizeHints := maxSizeHints

	childTypeHints := []SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	// canonical List[T, 1]: always dynamic, limit fixed at 1 element. Mark it
	// before descending into the element so a recursive back-edge landing on this
	// descriptor already sees its final layout classification.
	desc.Size = 0
	desc.Limit = 1
	desc.SszTypeFlags |= SszTypeFlagIsDynamic | SszTypeFlagHasLimit

	// Optional-list is a canonical List[T, 1] (variable-length), a legal boundary.
	tc.dynDepth++
	elemDesc, err := tc.getTypeDescriptor(runtimeType, schemaType, childSizeHints, childMaxSizeHints, childTypeHints)
	tc.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	return nil
}

// buildBigIntDescriptor builds a descriptor for ssz big int types
func (tc *TypeCache) buildBigIntDescriptor(desc *TypeDescriptor) error {
	if desc.Kind != reflect.Struct {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bigint type can only be represented by struct types, got %v", desc.Kind)
	}

	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	return nil
}

// buildVectorDescriptor builds a descriptor for ssz vector types with runtime/schema pairing.
//
// For vectors, the element type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (element type, size hints) while the runtime type provides
// the actual element type for data access.
func (tc *TypeCache) buildVectorDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	// Use schema type for SSZ layout determination
	t := schemaType

	if desc.Kind != reflect.Array && desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "vector ssz type can only be represented by array or slice types, got %v", desc.Kind)
	}

	if err := RejectMaxOnVector(maxSizeHints, schemaType.String()); err != nil {
		return err
	}

	switch {
	case desc.Kind == reflect.Array:
		// With a view descriptor the schema (view) type defines the SSZ layout while
		// the runtime type provides the backing storage. A schema array must not
		// declare MORE elements than the backing array holds: there is no storage for
		// the extra elements, so the reflection path would marshal/hash nonexistent
		// zero-padding and the codegen path would emit an out-of-bounds slice that
		// fails to compile. A SMALLER schema stays allowed — the Go array is sized for
		// the largest preset and a view may use only a prefix of it. (When no view is
		// used, runtimeType == schemaType, so this is a no-op.)
		if runtimeType.Kind() == reflect.Array && runtimeType.Len() < t.Len() {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "view schema array length (%d) exceeds the backing array length (%d)", t.Len(), runtimeType.Len())
		}
		desc.Len = uint32(t.Len())
		// A dynamic placeholder hint — e.g. dynssz-size:"?" on an outer dimension
		// whose Go type is a fixed array — carries Size 0 (and no Bits) and must
		// not zero the array's intrinsic length: a Go array cannot be relaxed to
		// a variable-length list, so it keeps its intrinsic length (matching the
		// codegen path). A concrete hint (Size > 0) or an explicit bit-size hint
		// (Bits set, incl. ssz-bitsize:"0", which must still be rejected as a
		// zero-length bitvector) is applied.
		if len(sizeHints) > 0 && (sizeHints[0].Size > 0 || sizeHints[0].Bits) {
			byteLen := sizeHints[0].Size
			if sizeHints[0].Bits {
				desc.BitSize = sizeHints[0].Size
				byteLen = (byteLen + 7) / 8 // ceil up to the next multiple of 8
			}
			if byteLen > desc.Len {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "size hint for vector type is greater than the length of the array (%d > %d)", byteLen, desc.Len)
			}
			desc.Len = byteLen
		}
	case len(sizeHints) > 0 && sizeHints[0].Size > 0:
		byteLen := sizeHints[0].Size
		if sizeHints[0].Bits {
			desc.BitSize = sizeHints[0].Size
			byteLen = (byteLen + 7) / 8 // ceil up to the next multiple of 8
		}
		desc.Len = byteLen
	default:
		return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "missing size hint for vector type")
	}

	// Per the SSZ spec, Vector[type, 0] and Bitvector[0] are illegal: a vector
	// must have a length greater than zero. This also catches dimensions whose
	// dynssz-size resolved to 0 with no positive static fallback. A bit size
	// misused on a non-bitvector also yields length 0, but that has a more
	// specific error reported after the descriptor is built, so don't mask it.
	if desc.Len == 0 && (desc.SszTypeFlags&SszTypeFlagHasBitSize == 0 || desc.SszType == SszBitvectorType) {
		return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "vector type %v has zero length, which is invalid per the SSZ spec", t)
	}

	childSizeHints := []SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	childTypeHints := []SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	// Determine element types for runtime and schema
	var runtimeElemType, schemaElemType reflect.Type
	if desc.Kind == reflect.String {
		// Strings are treated as []byte
		runtimeElemType = byteType
		schemaElemType = byteType
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	} else {
		// Get element type from both runtime and schema types
		schemaElemType = t.Elem()
		runtimeElemType = runtimeType.Elem()
		if schemaElemType == byteType {
			desc.GoTypeFlags |= GoTypeFlagIsByteArray
		}
	}

	// Build element descriptor using (runtimeElemType, schemaElemType) pair
	// This supports nested view descriptors within vector elements
	elemDesc, err := tc.getTypeDescriptor(runtimeElemType, schemaElemType, childSizeHints, childMaxSizeHints, childTypeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	if desc.SszType == SszBitvectorType && desc.ElemDesc.Kind != reflect.Uint8 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitvector ssz type can only be represented by byte slices or arrays, got %v", desc.ElemDesc.Kind.String())
	}

	if elemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
		desc.Size = 0
		desc.SszTypeFlags |= SszTypeFlagIsDynamic
	} else {
		// SSZ sizes are uint32; an unchecked product would wrap silently and
		// downstream length checks would then divide by or allocate from a
		// bogus size.
		totalSize := uint64(elemDesc.Size) * uint64(desc.Len)
		if totalSize > math.MaxUint32 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidValueRange, "vector byte size %d exceeds the uint32 SSZ size range", totalSize)
		}
		desc.Size = uint32(totalSize)
	}

	return nil
}

// buildListDescriptor builds a descriptor for ssz list types with runtime/schema pairing.
//
// For lists, the element type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (element type, size hints) while the runtime type provides
// the actual element type for data access.
func (tc *TypeCache) buildListDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	// Use schema type for SSZ layout determination
	t := schemaType

	if desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "list ssz type can only be represented by slice types, got %v", desc.Kind)
	}

	childSizeHints := []SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	childTypeHints := []SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	// Determine element types for runtime and schema
	var runtimeElemType, schemaElemType reflect.Type
	if desc.Kind == reflect.String {
		// Strings are treated as []byte
		runtimeElemType = byteType
		schemaElemType = byteType
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	} else {
		// Get element type from both runtime and schema types
		schemaElemType = t.Elem()
		runtimeElemType = runtimeType.Elem()
		if schemaElemType == byteType {
			desc.GoTypeFlags |= GoTypeFlagIsByteArray
		}
	}

	// A list is always dynamic (offset-encoded). Mark it before descending into
	// the element so a recursive back-edge landing on this descriptor already
	// sees its final layout classification.
	desc.Size = 0
	desc.SszTypeFlags |= SszTypeFlagIsDynamic

	// Build element descriptor using (runtimeElemType, schemaElemType) pair.
	// A list is offset-encoded (variable-length), so descending into its element
	// is a legal recursion boundary — bump the dynamic-nesting depth so a cycle
	// back to an enclosing type is accepted rather than rejected.
	tc.dynDepth++
	elemDesc, err := tc.getTypeDescriptor(runtimeElemType, schemaElemType, childSizeHints, childMaxSizeHints, childTypeHints)
	tc.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr)

	// A static element of zero size makes the element count underivable from
	// the wire format (region length / element size), so such a list can never
	// be decoded. Only reachable through custom types whose sizer reports 0;
	// every other zero-size shape is already rejected on its own.
	if elemDesc.SszTypeFlags&(SszTypeFlagIsDynamic|SszTypeFlagHasSizeExpr) == 0 && elemDesc.Size == 0 {
		return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "list element type %v has a static SSZ size of 0", schemaElemType)
	}

	if desc.SszType == SszBitlistType || desc.SszType == SszProgressiveBitlistType {
		if desc.Kind != reflect.Slice {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitlist ssz type can only be represented by byte slices, got %v", desc.Kind.String())
		}
		if desc.ElemDesc.Kind != reflect.Uint8 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitlist ssz type can only be represented by byte slices, got []%v", desc.ElemDesc.Kind.String())
		}
	}

	if len(sizeHints) > 0 && sizeHints[0].Size > 0 && !sizeHints[0].Dynamic {
		// Lists cannot have a fixed size; that's a vector.
		// Lists use ssz-max to specify the maximum length.
		// Name the tag the user actually wrote: a bit-unit hint came from
		// ssz-bitsize/dynssz-bitsize, and reporting it as ssz-size sends the
		// reader looking for a tag that is not there.
		if sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "list types cannot have a fixed ssz-bitsize (use ssz-max for lists, or ssz-bitsize with bitvector type)")
		}
		return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "list types cannot have a fixed ssz-size (use ssz-max for lists, or ssz-size with vector type)")
	}

	MarkNoSszRoot(tc.ExtendedTypes, desc)

	return nil
}

// RejectMaxOnVector rejects a limit declared for a dimension that is fixed. A
// vector's length is its type, so it has no capacity left to bound: the limit
// describes nothing and was being ignored, which reads as a bounded list to
// anyone writing the tag. It is shared by the reflection type cache and the
// code generator's parser so both refuse the same tags.
//
// The dimension is fixed either because ssz-size gave it a length or because
// the Go type is an array, so this catches both spellings. Only the dimension
// being built is examined: an outer array of bounded lists carries `?` for its
// own dimension and its limits belong to the inner ones.
func RejectMaxOnVector(maxSizeHints []SszMaxSizeHint, typeName string) error {
	if len(maxSizeHints) == 0 || maxSizeHints[0].NoValue {
		return nil
	}

	// A limit that only exists as an unresolved expression, or the ssz-max:"0"
	// placeholder, states no capacity either.
	if maxSizeHints[0].Size == 0 {
		return nil
	}

	return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint,
		"ssz-max %d is declared for %s, whose length is fixed: a vector has no capacity to bound (use ssz-size or ssz-max, not both, for one dimension)",
		maxSizeHints[0].Size, typeName)
}

// consumesDimension reports whether a type has an element for a tag dimension to
// describe. A collection passes the remaining dimensions down to what it holds;
// everything else is where the dimensions run out.
//
// A container is included: its fields carry their own tags rather than
// continuing the parent's, so a dimension past it belongs to nothing. Wrappers
// and unions are excluded from the check entirely -- they forward tags to a
// wrapped or selected type in ways a dimension count does not describe.
func consumesDimension(sszType SszType) bool {
	switch sszType {
	case SszListType, SszVectorType, SszBitlistType, SszBitvectorType,
		SszProgressiveListType, SszProgressiveBitlistType,
		SszOptionalType, SszOptionalListType,
		SszTypeWrapperType, SszCompatibleUnionType, SszUnionType,
		SszUint128Type, SszUint256Type, SszCustomType, SszUnspecifiedType:
		return true
	default:
		return false
	}
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
// The method acquires a read lock to ensure thread-safe access to the cache.
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
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	types := make([][2]reflect.Type, 0, len(tc.descriptors))
	for key := range tc.descriptors {
		types = append(types, [2]reflect.Type{key.runtime, key.schema})
	}

	return types
}

// RemoveType removes a specific type (with runtime == schema) from the cache.
//
// This method is useful for cache management scenarios where you need to force
// recomputation of a type descriptor, such as after configuration changes or
// when testing different type configurations.
//
// The method acquires a write lock to ensure thread-safe removal.
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
//
// Parameters:
//   - runtimeType: The runtime type of the cache entry to remove
//   - schemaType: The schema type of the cache entry to remove
func (tc *TypeCache) RemoveTypeKey(runtimeType, schemaType reflect.Type) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	if runtimeType.Kind() == reflect.Ptr {
		runtimeType = runtimeType.Elem()
	}
	if schemaType.Kind() == reflect.Ptr {
		schemaType = schemaType.Elem()
	}

	delete(tc.descriptors, typeKey{runtime: runtimeType, schema: schemaType})
}

// RemoveAllTypes clears all cached type descriptors from the cache.
//
// This method is useful for:
//   - Resetting the cache after configuration changes
//   - Memory management in long-running applications
//   - Testing scenarios requiring a clean cache state
//
// The method acquires a write lock to ensure thread-safe clearing.
// After calling this method, all subsequent type descriptor requests
// will trigger recomputation.
//
// Example:
//
//	// Clear cache after updating specifications
//	ds.UpdateSpecs(newSpecs)
//	cache.RemoveAllTypes()
//
//	// All types will be recomputed with new specs
//	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(MyStruct{}), nil, nil)
func (tc *TypeCache) RemoveAllTypes() {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	// Create new map to clear all references
	tc.descriptors = make(map[typeKey]*TypeDescriptor)
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
// distinct values. Acyclic descriptors keep the plain JSON form and their
// historical hash values.
func (td *TypeDescriptor) GetTypeHash() [32]byte {
	jsonDesc, err := json.Marshal(td)
	if err != nil {
		jsonDesc = marshalCyclicDescriptor(td)
	}
	hash := sha256.Sum256(jsonDesc)
	return hash
}
