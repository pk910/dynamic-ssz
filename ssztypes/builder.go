// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/pk910/dynamic-ssz/sszutils"
)

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

// builtRef records a descriptor a build completed. A hint-free descriptor
// carries an empty hint set and hinted is false.
type builtRef struct {
	key     typeKey
	variant *hintedVariant
	hinted  bool
}

// builder is the state of one top-level descriptor build. It belongs to the
// goroutine running the build and is never shared, so nothing it holds is
// reachable from another goroutine. Descriptors it owns (the ones on its build
// stack and the ones it completed) may be modified until it publishes them;
// a published descriptor is never modified.
type builder struct {
	tc *TypeCache
	// building is the build stack for cycle detection: one entry per (type
	// pair, hint set) with a build in flight.
	building map[typeKey][]*buildEntry
	// buildingTypes counts the pointer-stripped Go types with a build in
	// flight, so a fully-delegated type can tell whether its structure closes
	// a cycle with one of them.
	buildingTypes map[reflect.Type]int
	// dynDepth is the number of variable-length collections crossed on the
	// current path; it decides whether a re-entry is a legal cycle.
	dynDepth int
	// recursion records whether this build met a cycle.
	recursion bool
	// builtPlain and builtHinted hold every descriptor this build completed,
	// for publication once the whole build has succeeded; they also serve a
	// key the build reaches again (a diamond in the type graph).
	builtPlain  map[typeKey]*TypeDescriptor
	builtHinted map[typeKey][]*hintedVariant
	// order lists the completed descriptors in completion order, so the root
	// comes last.
	order []builtRef
	// startSeq is the cache's publication sequence when the build started;
	// the build uses only descriptors published at or before it (see lookup).
	startSeq uint64
}

// newBuilder returns the state for a new top-level build on tc. The caller
// holds buildMu.
func newBuilder(tc *TypeCache) *builder {
	return &builder{
		tc:            tc,
		startSeq:      tc.publishSeq,
		building:      make(map[typeKey][]*buildEntry, 8),
		buildingTypes: make(map[reflect.Type]int, 8),
		builtPlain:    make(map[typeKey]*TypeDescriptor, 8),
		builtHinted:   make(map[typeKey][]*hintedVariant, 8),
		order:         make([]builtRef, 0, 8),
	}
}

// dimensionKind returns the Go kind of the type at size dimension dim of t,
// walking through pointers and one array or slice level per dimension.
func dimensionKind(t reflect.Type, dim int) reflect.Kind {
	if dt := dimensionType(t, dim); dt != nil {
		return dt.Kind()
	}

	return reflect.Invalid
}

// customSizerAt reports whether the type at a tag dimension has a spec-aware
// sizer to supply a custom width from.
func customSizerAt(t reflect.Type, dim int) bool {
	dt := dimensionType(t, dim)

	return dt != nil && getDynamicSizerCompatibility(dt)
}

// dimensionType returns the type at a tag dimension, pointers stripped, or
// nil where the dimensions run out.
func dimensionType(t reflect.Type, dim int) reflect.Type {
	for {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if dim == 0 {
			return t
		}
		if t.Kind() != reflect.Array && t.Kind() != reflect.Slice {
			return nil
		}
		t = t.Elem()
		dim--
	}
}

// lookup returns the descriptor for key and hints that this build completed
// already or that was published before this build started, or nil. The
// caller holds buildMu.
//
// A build uses only descriptors published before it started; what other
// builds publish while it waits on a sizer is ignored, and the overlapping
// keys are resolved at publication. A build pushes a type onto its stack only
// after failing to look it up, and publication is all-or-nothing, so nothing
// published before the build started contains a type it is building. A graph
// published during a sizer window may contain one: taking any part of it
// would hand this build another build's copy of a type it is still building,
// and its own copy would never get its back-edge.
func (b *builder) lookup(key typeKey, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) *TypeDescriptor {
	if len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
		if desc := b.builtPlain[key]; desc != nil {
			return desc
		}
		if desc := b.tc.lookupPublished(key.runtime, key.schema); desc != nil && desc.publishSeq <= b.startSeq {
			return desc
		}
		return nil
	}
	// Hint-carrying builds are cached per exact hint combination: the same
	// (type, hints) pair recurs across every container referencing the type,
	// and the descriptor depends only on the type pair, the hints and the
	// cache configuration.
	if desc := findVariant(b.builtHinted[key], sizeHints, maxSizeHints, typeHints, b.startSeq); desc != nil {
		return desc
	}
	return findVariant(b.tc.hinted[key], sizeHints, maxSizeHints, typeHints, b.startSeq)
}

// derefType strips pointers from t.
func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// cycleWith returns a named type with a build in flight that the structure
// below t (its SSZ-visible struct fields and collection elements, through
// pointers) reaches, or nil. References back to t itself do not count: a
// cycle that stays within t is the business of t's own methods. Unnamed
// collections are the path between named types, not members of a cycle.
func (b *builder) cycleWith(t reflect.Type) reflect.Type {
	seen := map[reflect.Type]bool{derefType(t): true}
	var walk func(t reflect.Type) reflect.Type
	walk = func(t reflect.Type) reflect.Type {
		t = derefType(t)
		if seen[t] {
			return nil
		}
		if t.Name() != "" && b.buildingTypes[t] > 0 {
			return t
		}
		seen[t] = true
		return b.cycleBelow(t, walk)
	}
	return b.cycleBelow(derefType(t), walk)
}

// cycleBelow applies walk to the SSZ-visible fields or element of t and
// returns the first type it reports.
func (b *builder) cycleBelow(t reflect.Type, walk func(reflect.Type) reflect.Type) reflect.Type {
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || IsSszExcluded(f.Tag) {
				continue
			}
			if hit := walk(f.Type); hit != nil {
				return hit
			}
		}
		return nil
	case reflect.Slice, reflect.Array:
		return walk(t.Elem())
	default:
		return nil
	}
}

// getTypeDescriptor returns a cached type descriptor for a (runtime, schema) pair.
// When runtimeType == schemaType, this is the standard descriptor building.
// When they differ, it handles view descriptors where schema defines SSZ layout.
func (b *builder) getTypeDescriptor(runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) (*TypeDescriptor, error) {
	tc := b.tc
	// A reference whose hints read the same as the type's own annotation says
	// nothing new about the type: it is built as a bare reference, so the
	// type's declaration and methods stand and its plain descriptor is shared.
	// The annotation is read through the same readers as a tag, so the two
	// sides compare like for like. Type hints that name, dimension by
	// dimension, what the annotated type resolves to say nothing new either,
	// beside the annotation's own sizes and limits.
	if len(sizeHints) > 0 || len(maxSizeHints) > 0 || len(typeHints) > 0 {
		annotatedType := schemaType
		if annotatedType.Kind() == reflect.Pointer {
			annotatedType = annotatedType.Elem()
		}
		if annTag, ok := sszutils.LookupAnnotation(annotatedType); ok {
			annField := reflect.StructField{Name: annotatedType.String(), Type: annotatedType, Tag: reflect.StructTag(annTag)}
			annSize, sizeErr := getSszSizeTag(tc.specs, &annField)
			annMax, maxErr := getSszMaxSizeTag(tc.specs, &annField)
			annType, typeErr := getSszTypeTag(&annField)
			switch {
			case sizeErr == nil && maxErr == nil && typeErr == nil && SameHints(typeHints, annType, sizeHints, annSize, maxSizeHints, annMax):
				sizeHints, maxSizeHints, typeHints = nil, nil, nil
			case sizeErr == nil && maxErr == nil && (len(sizeHints) == 0 || slices.Equal(sizeHints, annSize)) && (len(maxSizeHints) == 0 || slices.Equal(maxSizeHints, annMax)) && hintsMatchDefault(annotatedType, typeHints):
				sizeHints, maxSizeHints, typeHints = nil, nil, nil
			}
		}
	}

	key := typeKey{runtime: runtimeType, schema: schemaType}
	cacheable := len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0

	// Detect self-referential (recursive) types. The whole descriptor tree is
	// built by this builder, so the build stack is tracked with a simple map
	// holding one entry per (type pair, hints) build in flight — the same Go
	// type can be under construction with different hints at once, and only a
	// re-entry with identical hints is a genuine cycle. A cycle is a legal,
	// finite SSZ type only when it crosses a variable-length collection
	// (list/optional): those are offset/presence encoded, so the type has finite
	// static size and terminates at runtime. A cycle through only fixed-size
	// fields (container/vector) has infinite static size and cannot be
	// serialized, so it is rejected.
	for _, entry := range b.building[key] {
		if !entry.matchesHints(sizeHints, maxSizeHints, typeHints) {
			continue
		}
		if b.dynDepth <= entry.depth {
			return nil, sszutils.NewSszErrorf(sszutils.ErrUnsupportedType, "recursive type %v is not supported", runtimeType)
		}
		// Legal cycle: hand back the descriptor still under construction. Its
		// child-derived fields are incomplete at this point, so consumers must not
		// rely on them; two measures keep the final graph correct:
		//  - Crossing a variable-length collection to legalize the cycle means
		//    every cycle member has a variable-size field on the cycle path, so
		//    the descriptor is provably dynamic. Setting IsDynamic here lets a
		//    container or vector reading it mid-build lay the field out as a
		//    dynamic (offset) field, which matches its final state. The
		//    descriptor is on this builder's stack, so it is not published yet.
		//  - The remaining child-derived flags are re-derived to a fixpoint by
		//    FixupRecursiveFlags once the whole graph is complete.
		entry.desc.SszTypeFlags |= SszTypeFlagIsDynamic
		b.recursion = true
		return entry.desc, nil
	}

	// A type on the build stack is never looked up: the cycle check above
	// hands back this build's own in-flight descriptor for it.
	if desc := b.lookup(key, sizeHints, maxSizeHints, typeHints); desc != nil {
		return desc, nil
	}

	// Allocate the descriptor before building its subtree so a recursive
	// reference through a variable-length collection can back-patch to it.
	desc := &TypeDescriptor{Type: runtimeType, SchemaType: schemaType}
	b.building[key] = append(b.building[key], &buildEntry{desc: desc, depth: b.dynDepth, hintSet: hintSet{sizeHints: sizeHints, maxSizeHints: maxSizeHints, typeHints: typeHints}})
	structType := derefType(runtimeType)
	b.buildingTypes[structType]++
	defer func() {
		// Builds nest strictly, so this build's entry is the last one pushed.
		entries := b.building[key]
		if len(entries) <= 1 {
			delete(b.building, key)
		} else {
			b.building[key] = entries[:len(entries)-1]
		}
		b.buildingTypes[structType]--
		if b.buildingTypes[structType] == 0 {
			delete(b.buildingTypes, structType)
		}
	}()

	err := b.buildTypeDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
	if err != nil {
		return nil, err
	}

	// Record the descriptor for publication. A cycle member completes before
	// the cycle head does; it stays private to this build until the whole
	// graph is complete and flag-fixed.
	variant := &hintedVariant{desc: desc}
	if cacheable {
		b.builtPlain[key] = desc
	} else {
		// Clone the hint slices: they may alias a caller-owned parse result or a
		// sub-slice of a parent's hints, while the cached variant must own its key.
		variant.hintSet = hintSet{
			sizeHints:    slices.Clone(sizeHints),
			maxSizeHints: slices.Clone(maxSizeHints),
			typeHints:    slices.Clone(typeHints),
		}
		b.builtHinted[key] = append(b.builtHinted[key], variant)
	}
	b.order = append(b.order, builtRef{key: key, variant: variant, hinted: !cacheable})

	return desc, nil
}

// hintsMatchDefault reports whether a reference's type hints name, dimension
// by dimension, the SSZ types the Go type and its element types resolve to on
// their own, with the type's own annotation in force. Below the type, the
// hints in force are what that annotation passes down; an element type's own
// annotation takes over only where the inherited hints are consistent with
// it, as the descriptor build applies it. Otherwise a dimension resolves to
// the kind. An open dimension names its own. A hint past the last element
// type names nothing the type has.
func hintsMatchDefault(t reflect.Type, typeHints []SszTypeHint) bool {
	var inForceTypes []SszTypeHint
	var inForceSizes []SszSizeHint
	var inForceMax []SszMaxSizeHint
	for i, hint := range typeHints {
		if t != nil {
			for t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			if tag, ok := sszutils.LookupAnnotation(t); ok {
				annTypes, annSizes, annMax, err := ParseTags(tag)
				if err != nil {
					return false
				}
				if i == 0 || ((len(inForceSizes) == 0 || slices.Equal(inForceSizes, annSizes)) &&
					(len(inForceMax) == 0 || slices.Equal(inForceMax, annMax)) &&
					hintsMatchDefault(t, inForceTypes)) {
					inForceTypes, inForceSizes, inForceMax = annTypes, annSizes, annMax
				}
			}
		}
		natural := SszUnspecifiedType
		if len(inForceTypes) > 0 {
			natural = inForceTypes[0].Type
			inForceTypes = inForceTypes[1:]
		}
		if natural == SszUnspecifiedType && t != nil {
			natural = defaultSszType(t, inForceSizes)
		}
		if len(inForceSizes) > 0 {
			inForceSizes = inForceSizes[1:]
		}
		if len(inForceMax) > 0 {
			inForceMax = inForceMax[1:]
		}
		if hint.Type != SszUnspecifiedType && hint.Type != natural {
			return false
		}
		if t != nil {
			switch t.Kind() {
			case reflect.Array, reflect.Slice:
				t = t.Elem()
			default:
				t = nil
			}
		}
	}
	return true
}

// defaultSszType returns the SSZ type a Go type resolves to without a type
// hint: a well-known external type by its name, otherwise by its kind. A
// slice or string is a vector under a static size hint and a list otherwise
// (`?` is what makes a dimension a list; any other tag names a length, and
// reading the resolved value instead would let the same type encode as a
// vector or as a list depending on which spec values a process happened to
// have loaded), and a named list whose name contains "Bitlist" is a bitlist.
// A kind without an SSZ type stays unspecified.
func defaultSszType(t reflect.Type, sizeHints []SszSizeHint) SszType {
	if sszType := WellKnownExternalType(t.PkgPath(), t.Name()); sszType != SszUnspecifiedType {
		return sszType
	}
	switch t.Kind() {
	case reflect.Bool:
		return SszBoolType
	case reflect.Uint8:
		return SszUint8Type
	case reflect.Uint16:
		return SszUint16Type
	case reflect.Uint32:
		return SszUint32Type
	case reflect.Uint64:
		return SszUint64Type
	case reflect.Struct:
		return SszContainerType
	case reflect.Array:
		return SszVectorType
	case reflect.Slice, reflect.String:
		if len(sizeHints) > 0 && !sizeHints[0].Dynamic {
			return SszVectorType
		}
		if strings.Contains(t.Name(), "Bitlist") {
			return SszBitlistType
		}
		return SszListType
	case reflect.Int8:
		return SszInt8Type
	case reflect.Int16:
		return SszInt16Type
	case reflect.Int32:
		return SszInt32Type
	case reflect.Int64:
		return SszInt64Type
	case reflect.Float32:
		return SszFloat32Type
	case reflect.Float64:
		return SszFloat64Type
	default:
		return SszUnspecifiedType
	}
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
func (b *builder) buildTypeDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	tc := b.tc
	// Verify runtime and schema types have compatible base kinds
	if runtimeType != schemaType {
		if runtimeType.Kind() != schemaType.Kind() {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "incompatible types: runtime kind %v != schema kind %v", runtimeType.Kind(), schemaType.Kind())
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
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "incompatible pointer types: runtime kind %v != schema kind %v", runtimeType.Kind(), schemaType.Kind())
		}
	}

	// Use schema type for determining the SSZ layout
	t := schemaType

	// Track whether hints were provided externally (from field tags) rather than
	// from the annotation registry. External hints override the type's own annotation,
	// so we must not delegate to generated methods that have the annotation baked in.
	hasExternalHints := len(sizeHints) > 0 || len(maxSizeHints) > 0
	callerTypeHinted := len(typeHints) > 0

	// staticAnnotation captures the type's own ssz-static:"true/false" declaration
	// (true = fixed-size, false = variable-size). It gates the shallow-build path
	// for fully-delegated types below.
	var staticAnnotation *bool

	annotationTag, hasAnnotation := sszutils.LookupAnnotation(t)

	// ssz-static is declared by the type, for the type: it states how the type
	// frames itself, which a reference to it neither supplies nor replaces. It
	// is read from the type's own annotation whichever hints the reference
	// brings, including the ones a struct field carries from this very
	// annotation.
	if hasAnnotation {
		if staticStr, hasStatic := reflect.StructTag(annotationTag).Lookup("ssz-static"); hasStatic {
			switch staticStr {
			case "true":
				v := true
				staticAnnotation = &v
			case "false":
				v := false
				staticAnnotation = &v
			default:
				return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "invalid ssz-static value %q for type %v (must be \"true\" or \"false\")", staticStr, t)
			}
		}
	}

	// Check annotation registry for type-level metadata when no external hints provided
	if hasAnnotation && len(sizeHints) == 0 && len(maxSizeHints) == 0 && len(typeHints) == 0 {
		var parseErr error

		typeHints, sizeHints, maxSizeHints, parseErr = ParseTags(annotationTag)
		if parseErr != nil {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "failed to parse annotation for type %v: %v", t, parseErr)
		}

		// ParseTags can't resolve dynamic expressions (no DynamicSpecs).
		// Resolve them now, unless the descriptor is being built to generate
		// code -- then the expression is what the output needs, not a value.
		if !tc.noSpecResolution {
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
				if resolveErr != nil {
					return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing dynssz-size expression %q for type %v: %v", sizeHints[i].Expr, t, resolveErr)
				}
				if ok && val > 0 {
					// A spec value spans the full uint64 range; the size
					// domain is signed, so the value is narrowed only after
					// the comparison that shows it fits.
					if val > math.MaxInt64 || exceedsSizeLimit(val, sizeHints[i].Bits) {
						return sszutils.NewSszErrorf(sszutils.SizeLimitSentinel(val), "ssz-size annotation value %d exceeds the SSZ size limit", val)
					}

					// The annotation's static size is what a fastssz method
					// baked in, so the value only makes the dimension dynamic
					// when it differs from that fallback, or when there is no
					// fallback to agree with.
					sizeHints[i].Custom = sizeHints[i].Size != int64(val)
					sizeHints[i].Size = int64(val)

					continue
				}
				// A custom width nobody supplied a value for keeps its
				// expression when the type's own spec-aware sizer can supply
				// it: the custom builder sizes it from that sizer, as it does
				// for a field tag naming the same width. A value that resolved
				// to zero is a dead end on both paths.
				sizerWidth := !ok && i < len(typeHints) && typeHints[i].Type == SszCustomType && customSizerAt(t, i)
				// A bit size on a Go array falls back to the array's own
				// length in bits; the vector builder applies that, as it
				// does for field tags. A slice has no length to fall back
				// to and is rejected there.
				if sizeHints[i].Size == 0 && !sizerWidth && (!sizeHints[i].Bits || dimensionKind(t, i) != reflect.Array) {
					return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz-size expression %q %s", sizeHints[i].Expr, unresolvedReason(ok))
				}
			}

			for i := range maxSizeHints {
				if maxSizeHints[i].Expr == "" {
					continue
				}

				ok, val, resolveErr := tc.specs.ResolveSpecValue(maxSizeHints[i].Expr)
				if resolveErr != nil {
					return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "error parsing dynssz-max expression %q for type %v: %v", maxSizeHints[i].Expr, t, resolveErr)
				}
				if ok && val > 0 {
					// As for the size: dynamic only when the value differs
					// from the annotation's static limit, or there is none.
					maxSizeHints[i].Custom = maxSizeHints[i].Size != val
					maxSizeHints[i].Size = val

					continue
				}
				if maxSizeHints[i].Size == 0 {
					return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz-max expression %q %s", maxSizeHints[i].Expr, unresolvedReason(ok))
				}
			}
		}
	}

	// A reference that declares an SSZ type other than the type's own overrides
	// the type, so it is described inline and its own methods are not used, as
	// a size or limit the reference supplies is. The type's own SSZ type is
	// what its annotation declares, or what it resolves to on its own when the
	// annotation declares none; a reference naming that type says nothing new.
	// The field tag is joined in front of the annotation, so an
	// annotation-declared type arrives here unchanged.
	if !hasExternalHints && len(typeHints) > 0 {
		if _, _, _, parseErr := ParseTags(annotationTag); parseErr != nil {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "failed to parse annotation for type %v: %v", t, parseErr)
		}
		hasExternalHints = !hintsMatchDefault(t, typeHints)
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
			// A limit no value was supplied for is the same dead end as a length.
			if desc.Limit == 0 && (!tc.noSpecResolution || tc.NoDelegation) {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz-max %q is not defined and has no positive static fallback", maxSizeHints[0].Expr)
			}

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
		// A time value is read and written through its methods, so a view
		// over it needs the same runtime type.
		if runtimeType != t {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "incompatible types: schema %v needs the same runtime type, got %v", t, runtimeType)
		}
		desc.GoTypeFlags |= GoTypeFlagIsTime
	}

	// auto-detect ssz type if not specified
	if sszType == SszUnspecifiedType {
		sszType = defaultSszType(t, sizeHints)
	}
	if sszType == SszUnspecifiedType {
		switch desc.Kind {
		// unsupported types
		case reflect.Int, reflect.Uint:
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "signed or unsigned integers with unspecified size are not supported in SSZ")
		case reflect.Complex64, reflect.Complex128:
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "complex numbers are not supported in SSZ (use unsigned integers instead)")
		case reflect.Map:
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "maps are not supported in SSZ (use structs or arrays instead)")
		case reflect.Chan:
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "channels are not supported in SSZ")
		case reflect.Func:
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "functions are not supported in SSZ")
		case reflect.Interface:
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "interfaces are not supported in SSZ (use concrete types)")
		case reflect.UnsafePointer:
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "unsafe pointers are not supported in SSZ")
		default:
			break
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
	// A type generated in the same run is reached through its generated methods
	// and, when a reference declares it custom, frames as its own descriptor does.
	sameRun := tc.noSpecResolution && callerTypeHinted && tc.getCompatFlag(runtimeType, schemaType) != 0
	if staticAnnotation != nil && !hasExternalHints && !tc.NoDelegation && (!sameRun || sszType != SszCustomType) {
		var fullyDelegated bool
		promoted := tc.PromotedDelegationMethods(runtimeType)
		if desc.GoTypeFlags&GoTypeFlagIsView != 0 {
			// Only a view the type's dispatcher recognizes is delegated; any other
			// view is walked.
			fullyDelegated = fullyDelegatesSSZView(runtimeType, promoted) && servesView(runtimeType, *desc.CodegenInfo)
		} else {
			fullyDelegated = fullyDelegatesSSZ(runtimeType, promoted)
		}
		// A delegated type is not traversed, so a cycle through it and a type
		// described here would go unmarked and uncounted: the members of a
		// cycle are described together, by one generator run.
		if fullyDelegated {
			if partner := b.cycleWith(runtimeType); partner != nil {
				return sszutils.NewSszErrorf(sszutils.ErrUnsupportedType, "%v delegates to its own SSZ methods but forms a recursive cycle with %v, which is described here: the members of a cycle must be generated in one run", runtimeType, partner)
			}
			size, sized, err := int64(0), false, error(nil)
			sizerWidth := *staticAnnotation && sszType == SszCustomType && (len(sizeHints) == 0 || sizeHints[0].Size == 0)
			switch {
			case sizerWidth && tc.noSpecResolution:
				// Generated code reads the width at run time.
				sized = true
				desc.SszTypeFlags |= SszTypeFlagHasSizeExpr
			case *staticAnnotation:
				size, sized, err = tc.delegatedStaticSize(desc, runtimeType)
				if err != nil {
					return err
				}
			}
			if sized {
				desc.Size = size
				if sizerWidth {
					desc.SszTypeFlags |= SszTypeFlagSizerWidth
				}
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
			desc.SszCompatFlags &^= SszCompatFlagFastsszSurface | SszCompatFlagFastsszHashRoot | SszCompatFlagFastsszHashRootWith
			desc.HashTreeRootWithMethod = nil
			desc.SszTypeFlags |= tc.customSpecFlags(desc)
			// A shallow descriptor has no traversed subtree: a static one still
			// knows its size, a dynamic one holds the floor its generation
			// declared, resolved against the specs as generated code resolves
			// it (see ParseMinSizeDeclaration).
			desc.SetMinSize()
			if desc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				if literal, expr, ok := ParseMinSizeDeclaration(reflect.StructTag(annotationTag)); ok {
					desc.MinSize = EvaluateMinSize(tc.specs, literal, expr)
				}
			}

			return nil
		}
	}

	// Check type compatibility and compute size
	switch sszType {
	case SszUnspecifiedType:
		if t.Kind() == reflect.Pointer {
			return sszutils.NewSszError(sszutils.ErrUnsupportedType, "unsupported multi-level pointer type: only a single level of indirection is supported")
		}
		return sszutils.NewSszErrorf(sszutils.ErrUnsupportedType, "unsupported type kind: %v", t.Kind())

	// basic types
	case SszBoolType:
		if desc.Kind != reflect.Bool {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bool ssz type can only be represented by bool types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "bool ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "bool ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint8 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint8 ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint16 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 2 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint16 ssz type must be ssz-size:2, got %v", sizeHints[0].Size)
		}
		desc.Size = 2
	case SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint32 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 4 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint32 ssz type must be ssz-size:4, got %v", sizeHints[0].Size)
		}
		desc.Size = 4
	case SszUint64Type:
		if desc.Kind != reflect.Uint64 && desc.GoTypeFlags&GoTypeFlagIsTime == 0 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "uint64 ssz type can only be represented by uint64 or time.Time types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint64 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
		}
		desc.Size = 8
	case SszUint128Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint128 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := b.buildUintDescriptor(desc, runtimeType, t, 16, "uint128") // handle as [16]uint8 or [2]uint64
		if err != nil {
			return err
		}
	case SszUint256Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "uint256 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := b.buildUintDescriptor(desc, runtimeType, t, 32, "uint256") // handle as [32]uint8 or [4]uint64
		if err != nil {
			return err
		}

	// complex types
	case SszTypeWrapperType:
		// A wrapper's constraints live in its descriptor struct; a size or
		// limit on the field holding the wrapper would silently lose to them.
		if len(sizeHints) > 0 || len(maxSizeHints) > 0 {
			return sszutils.NewSszError(sszutils.ErrInvalidTag, "ssz-size/ssz-max on a TypeWrapper field are not applied: declare the constraint in the wrapper's descriptor struct")
		}
		err := b.buildTypeWrapperDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return err
		}
	case SszContainerType, SszProgressiveContainerType:
		err := b.buildContainerDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return err
		}
	case SszVectorType, SszBitvectorType:
		err := b.buildVectorDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return err
		}
	case SszListType, SszBitlistType, SszProgressiveListType, SszProgressiveBitlistType:
		err := b.buildListDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return err
		}
	case SszCompatibleUnionType:
		err := b.buildCompatibleUnionDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return err
		}
	case SszUnionType:
		err := b.buildUnionDescriptor(desc, runtimeType, schemaType)
		if err != nil {
			return err
		}
	case SszCustomType:
		// A custom type serializes entirely through its own methods, so its Go
		// structure is never traversed and no child descriptors are built. It is
		// variable-size by default; an explicit ssz-size hint or an
		// ssz-static:"true" annotation pins a fixed size, while
		// ssz-static:"false" keeps it dynamic. A width that is not a literal is
		// read from the type's sizer: here on a zero value, by generated code at
		// run time, so a descriptor built for generation refuses a width the
		// generated code cannot ask for.
		if tc.noSpecResolution && len(sizeHints) > 0 && sizeHints[0].Expr != "" && !getDynamicSizerCompatibility(runtimeType) && tc.getCompatFlag(runtimeType, schemaType)&SszCompatFlagDynamicSizer == 0 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidTag, "custom type %v declares its width from %q but has no spec-aware sizer: the generator cannot know that width, so declare it with a literal ssz-size", t, sizeHints[0].Expr)
		}
		switch {
		case sameRun && (len(sizeHints) == 0 || (sizeHints[0].Size == 0 && sizeHints[0].Expr == "")):
			// A type generated in the same run declares its shape through the
			// annotation the run emits, which this build cannot see yet; its
			// own descriptor says the same.
			own, err := b.getTypeDescriptor(runtimeType, schemaType, nil, nil, nil)
			if err != nil {
				return err
			}
			if own.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
				desc.SszTypeFlags |= SszTypeFlagIsDynamic
			} else {
				desc.Size = own.Size
				desc.SszTypeFlags |= SszTypeFlagHasSizeExpr | SszTypeFlagSizerWidth
			}
		case tc.noSpecResolution && len(sizeHints) > 0 && sizeHints[0].Expr != "":
			// The width is read at run time from the type's sizer; the size
			// expression flag the hint set makes the emitters ask for it, and a
			// literal beside the expression is the static fallback.
			if tc.NoDelegation && sizeHints[0].Size == 0 {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "custom type %v has no static width to bake for %q", t, sizeHints[0].Expr)
			}
			desc.Size = sizeHints[0].Size
		case len(sizeHints) > 0 && sizeHints[0].Size > 0:
			desc.Size = sizeHints[0].Size
		case staticAnnotation != nil && *staticAnnotation && tc.noSpecResolution:
			if tc.NoDelegation {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "custom type %v declares ssz-static:\"true\" but no ssz-size: there is no static width to bake", t)
			}
			desc.SszTypeFlags |= SszTypeFlagHasSizeExpr | SszTypeFlagSizerWidth
		case (staticAnnotation != nil && *staticAnnotation) || (len(sizeHints) > 0 && sizeHints[0].Expr != ""):
			// A static delegate, or one whose width an expression names that no
			// value was supplied for and no literal stands in, is as wide as its
			// sizer says: that is the width generated code frames it with.
			size, sized, err := tc.delegatedStaticSize(desc, runtimeType)
			if err != nil {
				return err
			}
			if !sized {
				desc.Size = 0
				desc.SszTypeFlags |= SszTypeFlagIsDynamic

				break
			}
			desc.Size = size
			desc.SszTypeFlags |= SszTypeFlagSizerWidth
		default:
			desc.Size = 0
			desc.SszTypeFlags |= SszTypeFlagIsDynamic
		}

	// extended types (not supported by SSZ spec)
	case SszInt8Type:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int8 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int8 ssz type can only be represented by int8 types, got %v", desc.Kind)
		}
		desc.Size = 1
	case SszInt16Type:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int16 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int16 ssz type can only be represented by int16 types, got %v", desc.Kind)
		}
		desc.Size = 2
	case SszInt32Type:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int32 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int32 ssz type can only be represented by int32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case SszInt64Type:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int64 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "int64 ssz type can only be represented by int64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case SszFloat32Type:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float32 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "float32 ssz type can only be represented by float32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case SszFloat64Type:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float64 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "float64 ssz type can only be represented by float64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case SszOptionalType:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "optional types are not supported in SSZ (use extended types option to enable it)")
		}
		err := b.buildOptionalDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return err
		}
	case SszOptionalListType:
		// optional-list expresses a pointer as a canonical List[T, 1]; allowed without ExtendedTypes
		err := b.buildOptionalListDescriptor(desc, runtimeType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return err
		}
	case SszBigIntType:
		if !tc.ExtendedTypes {
			return sszutils.NewSszError(sszutils.ErrExtendedTypeDisabled, "big integers are not supported in SSZ (use extended types option to enable it)")
		}
		err := tc.buildBigIntDescriptor(desc, runtimeType)
		if err != nil {
			return err
		}
	}

	if desc.SszTypeFlags&SszTypeFlagHasBitSize != 0 && desc.SszType != SszBitvectorType && desc.SszType != SszBitlistType {
		return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "bit size tag is only allowed for bitvector or bitlist types, got %v", desc.SszType)
	}

	tc.detectCompatFlags(desc, runtimeType, schemaType)

	// Under NoDelegation a type that carries the complete dynamic surface is
	// walked, static methods or not. A -legacy generation of a type with spec
	// expressions emits static methods that forward to the type's dynamic method
	// with the global instance's specs, and nothing but the method set tells
	// them from real static bodies: a type with both surfaces could reach the
	// global specs from an instance of its own, so its static surface is dropped
	// as the shallow path above drops it. The static surface of a type without a
	// dynamic one stays governed by NoFastSsz. The rule reads the type's own
	// methods, so a flag the recursion fix-up raises later cannot change it. A
	// cache that describes code for the generator (noSpecResolution) keeps the
	// surface: a static generation reaches such a child only through its static
	// methods, and they are what break its recursion there.
	if tc.NoDelegation && !tc.noSpecResolution && desc.SszType != SszCustomType && fullyDelegatesSSZ(runtimeType, tc.PromotedDelegationMethods(runtimeType)) {
		desc.SszCompatFlags &^= SszCompatFlagFastsszSurface | SszCompatFlagFastsszHashRoot | SszCompatFlagFastsszHashRootWith
		desc.HashTreeRootWithMethod = nil
	}

	// A plain container that only satisfies a delegation interface through a
	// method promoted from an embedded field must not delegate through it: the
	// promoted method serializes just the embedded field and drops the
	// container's other fields. The suppression is per interface — an
	// interface the container declares itself (including a declaration
	// shadowing an embedded method) keeps delegating, only the promoted ones
	// fall back to the container walk. Custom types are exempt (they are
	// opaque and have no walkable layout).
	if desc.SszType == SszContainerType || desc.SszType == SszProgressiveContainerType {
		if promoted := tc.PromotedDelegationMethods(runtimeType); promoted != nil {
			if promoted["MarshalSSZDyn"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicMarshaler
			}
			if promoted["UnmarshalSSZDyn"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicUnmarshaler
			}
			if promoted["SizeSSZDyn"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicSizer
			}
			if promoted["HashTreeRootWithDyn"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicHashRoot
			}
			if promoted["MarshalSSZEncoder"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicEncoder
			}
			if promoted["UnmarshalSSZDecoder"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicDecoder
			}
			if promoted["MarshalSSZDynView"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicViewMarshaler
			}
			if promoted["UnmarshalSSZDynView"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicViewUnmarshaler
			}
			if promoted["SizeSSZDynView"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicViewSizer
			}
			if promoted["HashTreeRootWithDynView"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicViewHashRoot
			}
			if promoted["MarshalSSZEncoderView"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicViewEncoder
			}
			if promoted["UnmarshalSSZDecoderView"] {
				desc.SszCompatFlags &^= SszCompatFlagDynamicViewDecoder
			}
			// A promoted method answers for the embedded value, so it is never
			// the outer type's. A method the type declares itself is, so it
			// stays: each is cleared on its own.
			if promoted["MarshalSSZ"] {
				desc.SszCompatFlags &^= SszCompatFlagFastsszValueMarshaler
			}
			if promoted["MarshalSSZTo"] {
				desc.SszCompatFlags &^= SszCompatFlagFastsszBufferMarshaler
			}
			if promoted["SizeSSZ"] {
				desc.SszCompatFlags &^= SszCompatFlagFastsszSizer
			}
			if promoted["UnmarshalSSZ"] {
				desc.SszCompatFlags &^= SszCompatFlagFastsszUnmarshaler
			}
			if promoted["HashTreeRoot"] {
				desc.SszCompatFlags &^= SszCompatFlagFastsszHashRoot
			}
			if promoted["HashTreeRootWith"] {
				desc.SszCompatFlags &^= SszCompatFlagFastsszHashRootWith
				desc.HashTreeRootWithMethod = nil
			}
		}
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
			SszCompatFlagFastsszSurface |
			SszCompatFlagFastsszHashRoot |
			SszCompatFlagFastsszHashRootWith
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
			SszCompatFlagFastsszSurface |
			SszCompatFlagFastsszHashRoot |
			SszCompatFlagFastsszHashRootWith
	}

	// Per the SSZ spec, containers (including progressive containers) must have
	// at least one field. A struct with no SSZ-encodable (exported) fields has
	// no layout to encode and no width the layout could state: a value its own
	// methods serialize is a custom type, and says so with an ssz-type:"custom"
	// annotation, which also declares its width and is delegated whatever the
	// instance's options. Which methods the struct carries does not exempt it:
	// the descriptor would still frame it at zero bytes, and an option that
	// leaves those methods uncalled would walk it and encode nothing.
	if desc.SszType == SszContainerType || desc.SszType == SszProgressiveContainerType {
		if desc.ContainerDesc != nil && len(desc.ContainerDesc.Fields) == 0 {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "container type %v has no SSZ fields, which is invalid per the SSZ spec; a type serialized by its own methods declares that with an ssz-type:\"custom\" annotation", schemaType)
		}
	}

	if desc.SszType == SszCustomType {
		// A custom type delegates every SSZ operation to its own methods. Each
		// operation may be served by either the fastssz-style method or the dynssz
		// (Dynamic*) equivalent, but at least one implementation per operation is
		// required. Marshalling accepts either fastssz-style marshal method.
		if missing := missingDelegatedOperations(desc.SszCompatFlags); len(missing) > 0 {
			// A static method the type does carry is left out of the flags when a
			// resolved spec value differs from what it baked in; the type then
			// has no method that can answer for that value.
			if desc.SszTypeFlags&(SszTypeFlagHasDynamicSize|SszTypeFlagHasDynamicMax) != 0 &&
				(getFastsszCompatFlags(runtimeType) != 0 || getFastsszHashCompatibility(runtimeType) || getHashTreeRootWithCompatibility(runtimeType) != nil) {
				return sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "custom ssz type %v serves %s only through static methods, which bake in a size or limit the spec resolves differently", schemaType, strings.Join(missing, ", "))
			}
			return sszutils.NewSszErrorf(sszutils.ErrMissingInterface, "custom ssz type %v is missing a fastssz or dynssz %s implementation", schemaType, strings.Join(missing, ", "))
		}
		desc.SszTypeFlags |= tc.customSpecFlags(desc)
	}

	desc.SetMinSize()

	return nil
}

// missingDelegatedOperations names the SSZ operations a set of compatibility
// flags cannot serve. Each may be served by either the fastssz-style method or
// the dynssz equivalent, and marshalling accepts either fastssz-style marshal
// method, so a type serves an operation when it has any one of them. A custom
// type has no layout to fall back on, so it must serve every one.
func missingDelegatedOperations(f SszCompatFlag) []string {
	var missing []string
	if f&(SszCompatFlagFastsszBufferMarshaler|SszCompatFlagFastsszValueMarshaler|SszCompatFlagDynamicMarshaler|SszCompatFlagDynamicEncoder) == 0 {
		missing = append(missing, "marshaler")
	}
	if f&(SszCompatFlagFastsszUnmarshaler|SszCompatFlagDynamicUnmarshaler|SszCompatFlagDynamicDecoder) == 0 {
		missing = append(missing, "unmarshaler")
	}
	if f&(SszCompatFlagFastsszSizer|SszCompatFlagDynamicSizer) == 0 {
		missing = append(missing, "sizer")
	}
	if f&(SszCompatFlagFastsszHashRoot|SszCompatFlagFastsszHashRootWith|SszCompatFlagDynamicHashRoot) == 0 {
		missing = append(missing, "hasher")
	}

	return missing
}

// unresolvedReason says why an expression produced no usable value, so the
// error tells the reader whether to define the spec value or to correct it.
func unresolvedReason(resolved bool) string {
	if resolved {
		return "resolved to 0 with no positive static fallback"
	}

	return "is not defined and has no positive static fallback"
}

// fullyDelegatesSSZ reports whether the type declares the complete set of
// dynamic SSZ operation interfaces (marshal, unmarshal, size, hash-tree-root).
// When it does, every SSZ operation is handled by the type's own generated code
// and the descriptor subtree below it is never consulted, so it does not need to
// be built or validated — provided the type also declares its size via an
// annotation (see the shallow-build path in buildTypeDescriptor). A method only
// promoted from an embedded field (see PromotedDelegationMethods) serializes
// that field alone and does not count.
func fullyDelegatesSSZ(runtimeType reflect.Type, promoted map[string]bool) bool {
	return ((getDynamicMarshalerCompatibility(runtimeType) && !promoted["MarshalSSZDyn"]) ||
		(getDynamicEncoderCompatibility(runtimeType) && !promoted["MarshalSSZEncoder"])) &&
		((getDynamicUnmarshalerCompatibility(runtimeType) && !promoted["UnmarshalSSZDyn"]) ||
			(getDynamicDecoderCompatibility(runtimeType) && !promoted["UnmarshalSSZDecoder"])) &&
		getDynamicSizerCompatibility(runtimeType) && !promoted["SizeSSZDyn"] &&
		getDynamicHashRootCompatibility(runtimeType) && !promoted["HashTreeRootWithDyn"]
}

// servesView reports whether runtimeType's view dispatcher recognizes view.
func servesView(runtimeType reflect.Type, view any) bool {
	sizer, ok := reflect.New(runtimeType).Interface().(sszutils.DynamicViewSizer)
	return ok && sizer.SizeSSZDynView(view) != nil
}

// fullyDelegatesSSZView is the view-descriptor counterpart of fullyDelegatesSSZ:
// it reports whether the type declares the complete set of dynamic view SSZ
// operation interfaces, in which case a view descriptor also delegates every
// operation to the type's own code and its subtree need not be built.
func fullyDelegatesSSZView(runtimeType reflect.Type, promoted map[string]bool) bool {
	return ((getDynamicViewMarshalerCompatibility(runtimeType) && !promoted["MarshalSSZDynView"]) ||
		(getDynamicViewEncoderCompatibility(runtimeType) && !promoted["MarshalSSZEncoderView"])) &&
		((getDynamicViewUnmarshalerCompatibility(runtimeType) && !promoted["UnmarshalSSZDynView"]) ||
			(getDynamicViewDecoderCompatibility(runtimeType) && !promoted["UnmarshalSSZDecoderView"])) &&
		getDynamicViewSizerCompatibility(runtimeType) && !promoted["SizeSSZDynView"] &&
		getDynamicViewHashRootCompatibility(runtimeType) && !promoted["HashTreeRootWithDynView"]
}

// buildTypeWrapperDescriptor builds a descriptor for TypeWrapper types with runtime/schema pairing.
//
// For TypeWrappers, the wrapped type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ annotations while the runtime type provides actual data access.
func (b *builder) buildTypeWrapperDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	tc := b.tc
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

	// Index of the wrapped value field in the runtime struct. A generic
	// TypeWrapper always holds its value in field 0 (Data); a tagged wrapper
	// struct holds it at its single SSZ field, which excluded fields may
	// precede.
	wrappedFieldIndex := 0

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
		if !isTypeWrapper {
			wrappedFieldIndex = runtimeWrapperInfo.FieldIndex
		}
	} else {
		runtimeWrappedType = schemaWrapperInfo.Type
		if !isTypeWrapper {
			wrappedFieldIndex = schemaWrapperInfo.FieldIndex
		}
	}
	desc.WrapperFieldIndex = uint32(wrappedFieldIndex)

	// The wrapper's actual value field must be shape-compatible with the type the
	// descriptor expects. Otherwise the reflection-driven encode/decode would call
	// type-specific operations (Bytes, Uint, Len, ...) on an incompatible value
	// and panic deep in the reflect package.
	if wrappedFieldIndex < runtimeType.NumField() {
		valueType := runtimeType.Field(wrappedFieldIndex).Type
		if !wrapperTypeCompatible(valueType, runtimeWrappedType) {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch,
				"TypeWrapper value type %v is not compatible with descriptor type %v", valueType, runtimeWrappedType)
		}
	}

	// Build type descriptor for the wrapped type traversing both type trees
	wrappedDesc, err := b.getTypeDescriptor(runtimeWrappedType, schemaWrapperInfo.Type, schemaWrapperInfo.SizeHints, schemaWrapperInfo.MaxSizeHints, schemaWrapperInfo.TypeHints)
	if err != nil {
		return sszutils.ErrorWithPath(err, "(wrapper)")
	}

	// Store wrapper information
	desc.ElemDesc = wrappedDesc

	// The TypeWrapper inherits properties from the wrapped type
	desc.Size = wrappedDesc.Size
	desc.SszTypeFlags |= wrappedDesc.SszTypeFlags & (SszTypeFlagIsDynamic | SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr | SszTypeFlagSizerWidth)

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

// buildUintDescriptor builds a descriptor for a fixed-width integer (uint128,
// uint256) stored as a slice or array of uint8 or uint64 elements. The schema
// type fixes the layout; the runtime type must store it with the same element
// kind and, for arrays, the same length, and a view is checked the same way.
func (b *builder) buildUintDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, byteLen int64, typeName string) error {
	if desc.Kind != reflect.Slice && desc.Kind != reflect.Array {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "%s ssz type can only be represented by slice or array types, got %v", typeName, desc.Kind)
	}

	schemaElemType := schemaType.Elem()
	runtimeElemType := runtimeType.Elem()
	elemKind := schemaElemType.Kind()
	if elemKind != reflect.Uint8 && elemKind != reflect.Uint64 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "%s ssz type can only be represented by slices or arrays of uint8 or uint64, got %v", typeName, elemKind)
	}
	if runtimeElemType.Kind() != elemKind {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "%s ssz type stored as %v elements needs the same runtime element kind, got %v", typeName, elemKind, runtimeElemType.Kind())
	}
	// The bulk byte paths copy through a plain []byte, which is only
	// assignable to a slice whose element type is exactly byte; a named uint8
	// element decodes element-wise instead.
	if runtimeElemType == byteType {
		desc.GoTypeFlags |= GoTypeFlagIsByteArray
	}

	elemDesc, err := b.getTypeDescriptor(runtimeElemType, schemaElemType, nil, nil, nil)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.Size = byteLen
	desc.Len = desc.Size / elemDesc.Size

	if desc.Kind == reflect.Array {
		// A fixed-width uint occupies exactly desc.Len array elements. A smaller
		// array cannot hold it; a larger array carries trailing elements that
		// marshal would drop while HashTreeRoot rejects the length, so both are
		// refused, on the schema and on the runtime array alike. Unlike a
		// Vector/Bitvector (where an oversized backing array is a valid preset
		// pattern), a uint width is intrinsic and never preset-dependent.
		for _, arrayLen := range []int64{int64(schemaType.Len()), int64(runtimeType.Len())} {
			if arrayLen < desc.Len {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "%s ssz type does not fit in array (%d < %d)", typeName, arrayLen, desc.Len)
			}
			if arrayLen > desc.Len {
				return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "%s ssz type does not fill the array (%d > %d): trailing elements would be dropped", typeName, arrayLen, desc.Len)
			}
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
func (b *builder) buildContainerDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	tc := b.tc
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

	totalSize := int64(0)
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
		fieldDesc.FieldIndex = uint32(runtimeFieldIndex)
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

		// A field tag is joined in front of the type's registered annotation
		// (Lookup returns the first occurrence, so the field overrides per key)
		// and the joined tag is read like any other.
		joinFieldAnnotation(&schemaField)

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
		fieldDesc.Type, err = b.getTypeDescriptor(runtimeFieldType, schemaFieldType, sizeHints, maxSizeHints, typeHints)
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
				Index:        int32(runtimeFieldIndex), // Use runtime field index for data access
			})
		}

		desc.SszTypeFlags |= fieldDesc.Type.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr | SszTypeFlagSizerWidth)
		// A fixed section is addressed by 32-bit offsets, so it is bounded to
		// the SSZ size limit like every other size.
		if totalSize > sszutils.MaxSszSize-sszSize {
			return sszutils.NewSszErrorf(sszutils.SizeLimitSentinel(uint64(totalSize)+uint64(sszSize)), "container byte size %d exceeds the SSZ size limit", uint64(totalSize)+uint64(sszSize))
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

// validateUnionCarrier refuses a union carrier the operations cannot read. The
// size, marshal and hash paths take the selector from field 0 and the data from
// field 1, which a tag plus a GetDescriptorType method is enough to reach
// without being the generic carrier those paths assume.
//
// A selector is one byte on the wire and those paths narrow field 0 to a
// uint8, so a wider field would carry bits they drop: a selector of 256 reads
// as 0 and answers as that variant. The carrier holds it in a uint8, which is
// also what the codegen front end accepts.
func validateUnionCarrier(runtimeType reflect.Type, kind string) error {
	carrier := runtimeType
	if carrier.Kind() == reflect.Ptr {
		carrier = carrier.Elem()
	}
	if carrier.Kind() != reflect.Struct || carrier.NumField() < 2 {
		return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint,
			"%s carrier %v must be a struct of a selector and a data field", kind, runtimeType)
	}
	if k := carrier.Field(0).Type.Kind(); k != reflect.Uint8 {
		return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint,
			"%s carrier %v holds its selector in a %v, and a selector is one byte", kind, runtimeType, k)
	}

	return nil
}

// buildCompatibleUnionDescriptor builds a descriptor for CompatibleUnion types with runtime/schema pairing.
//
// For CompatibleUnion types, the variant types may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (variant order, annotations) while the runtime type provides
// the actual variant types for data access.
func (b *builder) buildCompatibleUnionDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	tc := b.tc
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

	if err := validateUnionCarrier(runtimeType, "compatible-union"); err != nil {
		return err
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

		variantDesc, err := b.getTypeDescriptor(runtimeVariantType, schemaInfo.Type, schemaInfo.SizeHints, schemaInfo.MaxSizeHints, schemaInfo.TypeHints)
		if err != nil {
			return sszutils.ErrorWithPathf(err, "(variant:%d)", variantIndex)
		}

		desc.UnionVariants[variantIndex] = variantDesc
		// The flags say whether any nested value depends on a spec value.
		desc.SszTypeFlags |= variantDesc.SszTypeFlags & childDerivedFlags
	}

	return nil
}

// buildUnionDescriptor builds a descriptor for classic spec unions with
// runtime/schema pairing. Selectors are the descriptor struct's 0-based field
// positions; a None marker declared first makes selector 0 the empty option
// (recorded via SszTypeFlagHasNoneVariant, with no variant entry).
func (b *builder) buildUnionDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type) error {
	tc := b.tc
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

	if err := validateUnionCarrier(runtimeType, "union"); err != nil {
		return err
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

		variantDesc, err := b.getTypeDescriptor(runtimeVariantType, schemaInfo.Type, schemaInfo.SizeHints, schemaInfo.MaxSizeHints, schemaInfo.TypeHints)
		if err != nil {
			return sszutils.ErrorWithPathf(err, "(variant:%d)", variantIndex)
		}

		desc.UnionVariants[variantIndex] = variantDesc
		// The flags say whether any nested value depends on a spec value.
		desc.SszTypeFlags |= variantDesc.SszTypeFlags & childDerivedFlags
	}

	return nil
}

// buildOptionalDescriptor builds a descriptor for optional types
func (b *builder) buildOptionalDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
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
	b.dynDepth++
	elemDesc, err := b.getTypeDescriptor(runtimeType, schemaType, childSizeHints, childMaxSizeHints, childTypeHints)
	b.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc

	// The Optional inherits properties from the child type
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagIsDynamic | SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr | SszTypeFlagSizerWidth)

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
func (b *builder) buildOptionalListDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
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
	b.dynDepth++
	elemDesc, err := b.getTypeDescriptor(runtimeType, schemaType, childSizeHints, childMaxSizeHints, childTypeHints)
	b.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr | SszTypeFlagSizerWidth)

	// A present element of zero size leaves the region as empty as an absent
	// one, so presence could never be decoded. Only reachable through custom
	// types whose sizer reports 0.
	if elemDesc.SszTypeFlags&(SszTypeFlagIsDynamic|SszTypeFlagHasSizeExpr) == 0 && elemDesc.Size == 0 {
		return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "optional-list element type %v has a static SSZ size of 0", schemaType)
	}

	return nil
}

// buildBigIntDescriptor builds a descriptor for ssz big int types
func (tc *TypeCache) buildBigIntDescriptor(desc *TypeDescriptor, runtimeType reflect.Type) error {
	// A big integer is read and written through big.Int's methods, whether
	// the type was detected or hinted, so the runtime type has to be big.Int.
	if runtimeType != bigIntType {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bigint ssz type can only be represented by math/big.Int, got %v", runtimeType)
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
func (b *builder) buildVectorDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	tc := b.tc
	// Use schema type for SSZ layout determination
	t := schemaType

	if desc.Kind != reflect.Array && desc.Kind != reflect.Slice && desc.Kind != reflect.String {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "vector ssz type can only be represented by array or slice types, got %v", desc.Kind)
	}

	arrayLen := uint64(0)
	if t.Kind() == reflect.Array {
		arrayLen = uint64(t.Len())
	}
	if err := RejectMaxOnVector(sizeHints, maxSizeHints, arrayLen, schemaType.String()); err != nil {
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
		desc.Len = int64(t.Len())
		// A hint without a size leaves the array's intrinsic length in place:
		// a dynamic placeholder (dynssz-size:"?") cannot relax a Go array to a
		// list, and a bit size named by an expression nothing supplied a value
		// for falls back to the array's own length in bits, as the code
		// generator does. A concrete hint (Size > 0) or a literal bit size
		// (incl. ssz-bitsize:"0", which is still rejected as a zero-length
		// bitvector) is applied.
		if len(sizeHints) > 0 && (sizeHints[0].Size > 0 || (sizeHints[0].Bits && sizeHints[0].Expr == "")) {
			byteLen := sizeHints[0].Size
			if sizeHints[0].Bits {
				desc.BitSize = sizeHints[0].Size
				byteLen = int64((uint64(byteLen) + 7) / 8) // ceil up to the next multiple of 8
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
			byteLen = int64((uint64(byteLen) + 7) / 8) // ceil up to the next multiple of 8
		}
		desc.Len = byteLen
	case len(sizeHints) > 0 && sizeHints[0].Expr != "":
		// The length is named by an expression no value was supplied for. That is
		// what generated code needs -- it resolves the expression itself -- so the
		// length stays symbolic. Anywhere else the type named a length nothing can
		// provide, the same dead end as one resolving to zero.
		if !tc.noSpecResolution {
			return sszutils.NewSszErrorf(sszutils.ErrInvalidConstraint, "dynssz-size %q is not defined and has no positive static fallback", sizeHints[0].Expr)
		}
	default:
		// A dimension is a vector because its tag is not `?`, so a tag naming
		// zero arrives here rather than as a list; say so in those terms.
		if err := rejectZeroSizeHint(sizeHints); err != nil {
			return err
		}

		return sszutils.NewSszError(sszutils.ErrInvalidConstraint, "missing size hint for vector type")
	}

	// Per the SSZ spec, Vector[type, 0] and Bitvector[0] are illegal: a vector
	// must have a length greater than zero. This also catches dimensions whose
	// dynssz-size resolved to 0 with no positive static fallback. A bit size
	// misused on a non-bitvector also yields length 0, but that has a more
	// specific error reported after the descriptor is built, so don't mask it.
	// A length supplied purely by an expression is legitimately 0 here while
	// generating code, so only a genuine static zero is rejected (matching the
	// code generator's own parser).
	if desc.Len == 0 && (desc.SizeExpression == nil || (tc.NoDelegation && tc.noSpecResolution)) && (desc.SszTypeFlags&SszTypeFlagHasBitSize == 0 || desc.SszType == SszBitvectorType) {
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
		// The bulk byte paths copy through the runtime value as a plain
		// []byte, so the flag follows the runtime element type: a named uint8
		// element (or a view whose runtime element is one) decodes element-wise.
		if runtimeElemType == byteType {
			desc.GoTypeFlags |= GoTypeFlagIsByteArray
		}
	}

	// Build element descriptor using (runtimeElemType, schemaElemType) pair
	// This supports nested view descriptors within vector elements
	elemDesc, err := b.getTypeDescriptor(runtimeElemType, schemaElemType, childSizeHints, childMaxSizeHints, childTypeHints)
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr | SszTypeFlagSizerWidth)

	// A bitvector is a sequence of bits stored in bytes; a string holds text.
	if desc.SszType == SszBitvectorType && desc.GoTypeFlags&GoTypeFlagIsString != 0 {
		return sszutils.NewSszError(sszutils.ErrTypeMismatch, "bitvector ssz type can only be represented by byte slices or arrays, got string")
	}
	if desc.SszType == SszBitvectorType && desc.ElemDesc.Kind != reflect.Uint8 {
		return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitvector ssz type can only be represented by byte slices or arrays, got %v", desc.ElemDesc.Kind.String())
	}

	// A vector's length is a size itself; a vector of variable-size elements
	// also leads with one 4-byte offset per element inside its fixed section.
	if desc.Len > sszutils.MaxSszSize {
		return sszutils.NewSszErrorf(sszutils.SizeLimitSentinel(uint64(desc.Len)), "vector length %d exceeds the SSZ size limit", desc.Len)
	}
	if elemDesc.SszTypeFlags&SszTypeFlagIsDynamic != 0 {
		if desc.Len > sszutils.MaxSszSize/4 {
			return sszutils.NewSszErrorf(sszutils.SizeLimitSentinel(uint64(desc.Len)*4), "vector length %d exceeds the SSZ offset table limit", desc.Len)
		}
		desc.Size = 0
		desc.SszTypeFlags |= SszTypeFlagIsDynamic
	} else {
		// A vector's byte size is bounded to the SSZ size limit like every
		// other size. The bound is checked by division so the product itself
		// cannot wrap first.
		if desc.Len > 0 && elemDesc.Size > sszutils.MaxSszSize/desc.Len {
			return sszutils.NewSszErrorf(sszutils.SizeLimitSentinel(uint64(elemDesc.Size)*uint64(desc.Len)), "vector byte size %d*%d exceeds the SSZ size limit", elemDesc.Size, desc.Len)
		}
		desc.Size = elemDesc.Size * desc.Len
	}

	return nil
}

// buildListDescriptor builds a descriptor for ssz list types with runtime/schema pairing.
//
// For lists, the element type may differ between runtime and schema when using view descriptors.
// The schema type defines the SSZ layout (element type, size hints) while the runtime type provides
// the actual element type for data access.
func (b *builder) buildListDescriptor(desc *TypeDescriptor, runtimeType, schemaType reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint, typeHints []SszTypeHint) error {
	tc := b.tc
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
		// The bulk byte paths copy through the runtime value as a plain
		// []byte, so the flag follows the runtime element type: a named uint8
		// element (or a view whose runtime element is one) decodes element-wise.
		if runtimeElemType == byteType {
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
	b.dynDepth++
	elemDesc, err := b.getTypeDescriptor(runtimeElemType, schemaElemType, childSizeHints, childMaxSizeHints, childTypeHints)
	b.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr | SszTypeFlagSizerWidth)

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
		// A named uint8 element is viewed as a byte without copying; a pointer
		// element has no such view.
		if desc.ElemDesc.Kind != reflect.Uint8 || desc.ElemDesc.GoTypeFlags&GoTypeFlagIsPointer != 0 {
			return sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "bitlist ssz type can only be represented by byte slices, got []%v", runtimeElemType)
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
