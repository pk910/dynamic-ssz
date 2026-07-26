// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/types"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

const (
	typeNameError     = "error"
	typeNameInt       = "int"
	typeNameByteSlice = "[]byte"
	typeNameString    = "string"
	typeNameTime      = "Time"
	pkgPathTime       = "time"
	pkgPathDynssz     = "github.com/pk910/dynamic-ssz"
)

var (
	byteType = types.Typ[types.Uint8]
)

// CodegenInfo contains type information specific to code generation from go/types analysis.
//
// This structure bridges the gap between compile-time type analysis (using go/types)
// and runtime code generation. It stores type information that was obtained through
// static analysis rather than runtime reflection, enabling more sophisticated
// code generation scenarios.
//
// Fields:
//   - Type: The types.Type from go/types package representing the analyzed type
//   - SchemaType: The types.Type representing the schema structure (may differ from Type for view descriptors)
//
// This information is embedded in TypeDescriptor.CodegenInfo to provide access
// to compile-time type information during code generation.
type CodegenInfo struct {
	Type       types.Type
	SchemaType types.Type
}

// Parser provides compile-time type analysis for SSZ code generation.
//
// The Parser analyzes Go types using the go/types package to create TypeDescriptors
// suitable for code generation. Unlike runtime reflection, this approach can analyze
// types that may not be available at runtime and provides richer type information
// for complex code generation scenarios.
//
// Key capabilities:
//   - Compile-time type analysis using go/types
//   - SSZ type inference and validation
//   - Struct tag parsing for SSZ annotations
//   - Interface compatibility checking
//   - Type descriptor caching for performance
//
// The parser handles all SSZ-compatible types including basic types, containers,
// vectors, lists, and custom types like unions and type wrappers.
//
// Fields:
//   - cache: Type descriptor cache to avoid recomputing analysis for the same types
//
// parserHintedVariant is a cached descriptor for a (type pair, hints)
// combination; the descriptor is a pure function of both plus the parser
// configuration, so identical hints always yield an identical descriptor.
type parserHintedVariant struct {
	desc         *ssztypes.TypeDescriptor
	typeHints    []ssztypes.SszTypeHint
	sizeHints    []ssztypes.SszSizeHint
	maxSizeHints []ssztypes.SszMaxSizeHint
}

func (v *parserHintedVariant) matchesHints(typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint) bool {
	return slices.Equal(v.typeHints, typeHints) && slices.Equal(v.sizeHints, sizeHints) && slices.Equal(v.maxSizeHints, maxSizeHints)
}

// parserPendingKey records a cache insertion of the current top-level build so
// a failed build can purge it.
type parserPendingKey struct {
	key    string
	hinted bool
}

type Parser struct {
	cache         map[string]*ssztypes.TypeDescriptor
	CompatFlags   map[string]ssztypes.SszCompatFlag
	ExtendedTypes bool

	// building tracks descriptors currently under construction, keyed like the
	// cache and mapped to the variable-length nesting depth at which each build
	// started. dynDepth is the current such depth: it only increases while
	// descending into the element of a variable-length collection (list,
	// progressive list, optional, optional-list), which forms a legal recursion
	// boundary. A cycle re-entered without crossing one of these is an infinite
	// (non-serializable) static type and is rejected. Parsing is single-threaded,
	// so plain fields are safe.
	building map[string]int
	dynDepth int

	// hintedCache caches builds with external hints, matched by exact hint
	// equality: the same (type pair, hints) combination recurs across every
	// container referencing the type.
	hintedCache map[string][]*parserHintedVariant

	// recursion is set when a build hands out an in-progress descriptor to close
	// a legal cycle; the top-level entry then re-derives the child-propagated
	// flags over the completed graph. pendingKeys records the cache insertions
	// of the current top-level build so a failed build can purge them — the
	// plain cache is populated before a descriptor is built, so entries from a
	// failed build are incomplete and must not be served later.
	recursion   bool
	pendingKeys []parserPendingKey

	// AnnotationResolver returns the merged ssz annotation tag for a type (or "").
	// It lets the parser read a referenced, fully-delegated type's ssz-static
	// declaration so its subtree need not be traversed or validated.
	AnnotationResolver func(types.Type) string
}

// NewParser creates a new compile-time type parser for code generation.
//
// The parser is initialized with an empty cache and is ready to analyze types
// using the go/types package. The parser can be reused across multiple type
// analysis operations to benefit from caching.
//
// Returns:
//   - *Parser: A new parser instance ready for type analysis
//
// Example:
//
//	parser := NewParser()
//	desc, err := parser.GetTypeDescriptor(myGoType, nil, nil, nil)
//	if err != nil {
//	    log.Fatal("Type analysis failed:", err)
//	}
func NewParser() *Parser {
	return &Parser{
		cache:       make(map[string]*ssztypes.TypeDescriptor),
		CompatFlags: map[string]ssztypes.SszCompatFlag{},
		building:    make(map[string]int),
		hintedCache: make(map[string][]*parserHintedVariant),
	}
}

// GetTypeDescriptor analyzes a Go type and creates an SSZ type descriptor for code generation.
//
// This method is the main entry point for type analysis. It examines the provided
// go/types.Type and creates a comprehensive TypeDescriptor containing all information
// needed for SSZ code generation, including size calculations, encoding strategies,
// and interface compatibility.
//
// The analysis process includes:
//   - Type structure examination and validation
//   - SSZ type inference and mapping
//   - Size and constraint analysis from hints
//   - Interface compatibility checking
//   - Nested type analysis for containers and collections
//
// Parameters:
//   - typ: The go/types.Type to analyze
//   - typeHints: Optional hints for explicit SSZ type mapping
//   - sizeHints: Optional size constraints and expressions
//   - maxSizeHints: Optional maximum size limits for variable-length types
//
// Returns:
//   - *ssztypes.TypeDescriptor: Complete type descriptor for code generation
//   - error: An error if the type is incompatible with SSZ or analysis fails
//
// Example:
//
//	parser := NewParser()
//	typeHints := []ssztypes.SszTypeHint{{Type: ssztypes.SszListType}}
//	sizeHints := []ssztypes.SszSizeHint{{Size: 1024}}
//
//	desc, err := parser.GetTypeDescriptor(structType, typeHints, sizeHints, nil)
//	if err != nil {
//	    return fmt.Errorf("failed to analyze type: %w", err)
//	}
func (p *Parser) GetTypeDescriptor(typ types.Type, typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint) (*ssztypes.TypeDescriptor, error) {
	// When no view descriptor is used, runtime and schema types are the same
	return p.GetTypeDescriptorWithSchema(typ, typ, typeHints, sizeHints, maxSizeHints)
}

// GetTypeDescriptorWithSchema analyzes Go types and creates an SSZ type descriptor with separate schema and data types.
//
// This method supports fork-dependent SSZ schemas (view descriptors) where the schema type
// defines the SSZ layout while the data type holds the actual data. This allows different
// SSZ serializations of the same data based on the schema provided.
//
// When dataType == schemaType, this behaves identically to GetTypeDescriptor.
// When they differ, the descriptor is built using schema's field definitions (names, tags,
// order) but code generation targets the data type's fields.
//
// Parameters:
//   - dataType: The types.Type where actual data lives (runtime type)
//   - schemaType: The types.Type that defines SSZ layout (field order, tags, limits)
//   - typeHints: Optional hints for explicit SSZ type mapping
//   - sizeHints: Optional size constraints and expressions
//   - maxSizeHints: Optional maximum size limits for variable-length types
//
// Returns:
//   - *ssztypes.TypeDescriptor: Complete type descriptor for code generation
//   - error: An error if the type is incompatible with SSZ or analysis fails
func (p *Parser) GetTypeDescriptorWithSchema(dataType, schemaType types.Type, typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint) (*ssztypes.TypeDescriptor, error) {
	// This is the top-level entry of a descriptor build (nested builds recurse
	// through buildTypeDescriptor directly); recursion bookkeeping is scoped here.
	p.recursion = false
	p.pendingKeys = p.pendingKeys[:0]

	desc, err := p.buildTypeDescriptor(dataType, schemaType, typeHints, sizeHints, maxSizeHints)
	if err != nil {
		// The plain cache is populated before a descriptor is built, so a failed
		// build leaves incomplete entries behind; purge everything cached on the
		// way. Hinted variants appended during this build sit at their list's
		// tail, so reverse order pops them correctly.
		for i := len(p.pendingKeys) - 1; i >= 0; i-- {
			pending := p.pendingKeys[i]
			if !pending.hinted {
				delete(p.cache, pending.key)
				continue
			}
			variants := p.hintedCache[pending.key]
			if len(variants) <= 1 {
				delete(p.hintedCache, pending.key)
			} else {
				p.hintedCache[pending.key] = variants[:len(variants)-1]
			}
		}
		return nil, err
	}

	// A build that involved a recursive cycle has descriptors that read
	// in-progress children; re-derive the child-propagated flags now that the
	// graph is complete. Non-recursive builds are already exact.
	if p.recursion {
		ssztypes.FixupRecursiveFlags(desc)
	}

	return desc, nil
}

func (p *Parser) getCompatFlag(dataType, schemaType types.Type) ssztypes.SszCompatFlag {
	typeName := dataType.String()
	if dataType != schemaType {
		typeName = fmt.Sprintf("%v|%v", dataType.String(), schemaType.String())
	}
	return p.CompatFlags[typeName]
}

// detectCompatFlags records which SSZ delegation interfaces the type implements,
// checking both the value and pointer forms. The fastssz family is only flagged
// when the descriptor does not carry a dynamic size/max (those use the static
// fastssz layout). Mirrors the reflection typecache's detection.
func (p *Parser) detectCompatFlags(desc *ssztypes.TypeDescriptor, originalType, innerDataType, innerSchemaType types.Type) {
	otherType := originalType
	if ptr, ok := otherType.(*types.Pointer); ok {
		otherType = ptr.Elem()
	} else {
		otherType = types.NewPointer(otherType)
	}

	if (desc.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicSize == 0 || desc.SszType == ssztypes.SszCustomType) && (p.getFastsszConvertCompatibility(originalType) || p.getFastsszConvertCompatibility(otherType)) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagFastSSZMarshaler
	}
	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicMax == 0 || desc.SszType == ssztypes.SszCustomType {
		if p.getFastsszHashCompatibility(originalType) || p.getFastsszHashCompatibility(otherType) {
			desc.SszCompatFlags |= ssztypes.SszCompatFlagFastSSZHasher
		}
		if p.getHashTreeRootWithCompatibility(originalType) || p.getHashTreeRootWithCompatibility(otherType) {
			desc.SszCompatFlags |= ssztypes.SszCompatFlagHashTreeRootWith
		}
	}

	// Check for dynamic interface implementations
	if p.getDynamicMarshalerCompatibility(originalType) || p.getDynamicMarshalerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicMarshaler
	}
	if p.getDynamicUnmarshalerCompatibility(originalType) || p.getDynamicUnmarshalerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicUnmarshaler
	}
	if p.getDynamicEncoderCompatibility(originalType) || p.getDynamicEncoderCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicEncoder
	}
	if p.getDynamicDecoderCompatibility(originalType) || p.getDynamicDecoderCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicDecoder
	}
	if p.getDynamicSizerCompatibility(originalType) || p.getDynamicSizerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicSizer
	}
	if p.getDynamicHashRootCompatibility(originalType) || p.getDynamicHashRootCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicHashRoot
	}

	// Check for dynamic view interface implementations (for fork-dependent SSZ schemas)
	if p.getDynamicViewMarshalerCompatibility(originalType) || p.getDynamicViewMarshalerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicViewMarshaler
	}
	if p.getDynamicViewUnmarshalerCompatibility(originalType) || p.getDynamicViewUnmarshalerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicViewUnmarshaler
	}
	if p.getDynamicViewEncoderCompatibility(originalType) || p.getDynamicViewEncoderCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicViewEncoder
	}
	if p.getDynamicViewDecoderCompatibility(originalType) || p.getDynamicViewDecoderCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicViewDecoder
	}
	if p.getDynamicViewSizerCompatibility(originalType) || p.getDynamicViewSizerCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicViewSizer
	}
	if p.getDynamicViewHashRootCompatibility(originalType) || p.getDynamicViewHashRootCompatibility(otherType) {
		desc.SszCompatFlags |= ssztypes.SszCompatFlagDynamicViewHashRoot
	}

	desc.SszCompatFlags |= p.getCompatFlag(innerDataType, innerSchemaType)
}

// fullyDelegatesSSZ reports whether the type implements the complete set of
// dynamic SSZ operation interfaces (marshal, unmarshal, size, hash-tree-root),
// checking both value and pointer forms. Such a type handles every operation in
// its own generated code, so a reference to it need not be traversed.
func (p *Parser) fullyDelegatesSSZ(t types.Type) bool {
	other := t
	if ptr, ok := other.(*types.Pointer); ok {
		other = ptr.Elem()
	} else {
		other = types.NewPointer(other)
	}
	any2 := func(f func(types.Type) bool) bool { return f(t) || f(other) }
	return (any2(p.getDynamicMarshalerCompatibility) || any2(p.getDynamicEncoderCompatibility)) &&
		(any2(p.getDynamicUnmarshalerCompatibility) || any2(p.getDynamicDecoderCompatibility)) &&
		any2(p.getDynamicSizerCompatibility) &&
		any2(p.getDynamicHashRootCompatibility)
}

//nolint:gocyclo // SSZ type descriptor builder is inherently complex
func (p *Parser) buildTypeDescriptor(dataType, schemaType types.Type, typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint) (*ssztypes.TypeDescriptor, error) {
	// Only cache in the plain descriptor cache when types match and no hints
	// are provided; hint-carrying builds are cached per exact hint combination.
	cacheable := dataType == schemaType && len(typeHints) == 0 && len(sizeHints) == 0 && len(maxSizeHints) == 0
	typeKey := fmt.Sprintf("%v|%v", dataType.String(), schemaType.String())
	if cacheable && p.cache[typeKey] != nil {
		if startDepth, inProgress := p.building[typeKey]; inProgress {
			if p.dynDepth <= startDepth {
				// Re-entering a type still under construction without crossing a
				// variable-length collection means it contributes to its own static
				// size — an infinite, non-serializable SSZ type. Reject it like the
				// reflection engine does instead of emitting infinitely recursive code.
				return nil, fmt.Errorf("recursive type %v is not supported", dataType.String())
			}
			// Legal cycle: hand back the descriptor still under construction. Its
			// child-derived fields are incomplete at this point; two measures keep
			// the final graph correct:
			//  - Crossing a variable-length collection to legalize the cycle means
			//    every cycle member has a variable-size field on the cycle path, so
			//    the descriptor is provably dynamic. Setting IsDynamic here lets a
			//    container or vector reading it mid-build lay the field out as a
			//    dynamic (offset) field, which matches its final state.
			//  - The remaining child-derived flags are re-derived to a fixpoint by
			//    ssztypes.FixupRecursiveFlags once the whole graph is complete.
			p.cache[typeKey].SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
			p.recursion = true
		}
		return p.cache[typeKey], nil
	}
	if !cacheable {
		// The same (type pair, hints) combination recurs across every reference
		// site (most container fields carry ssz-size/ssz-max tags), and the
		// descriptor depends only on the type pair, the hints and the parser
		// configuration — reuse instead of re-analyzing per reference.
		for _, variant := range p.hintedCache[typeKey] {
			if variant.matchesHints(typeHints, sizeHints, maxSizeHints) {
				return variant.desc, nil
			}
		}
	}

	// The hint parameters may be reassigned below when a type-level annotation
	// supplies them; the cache variant must be keyed by the hints the CALLER
	// passed, so capture them before any reassignment.
	callerTypeHints, callerSizeHints, callerMaxSizeHints := typeHints, sizeHints, maxSizeHints

	// Create descriptor with both data and schema types
	codegenInfo := &CodegenInfo{Type: dataType, SchemaType: schemaType}
	var anyCodegenInfo any = codegenInfo
	desc := &ssztypes.TypeDescriptor{
		CodegenInfo: &anyCodegenInfo,
	}

	if cacheable {
		p.cache[typeKey] = desc
		p.pendingKeys = append(p.pendingKeys, parserPendingKey{key: typeKey})
		p.building[typeKey] = p.dynDepth
		defer delete(p.building, typeKey)
	}

	// Use schemaType for SSZ layout analysis, dataType for interface checks

	originalType := dataType
	innerSchemaType := schemaType
	innerDataType := dataType

	var schemaNamedType, dataNamedType *types.Named

	var ptrType *types.Pointer

	for {
		// Resolve named types - allow independent unwrapping since view and data types
		// may have different naming structures (e.g., schema: [32]byte, data: Root where Root = [32]byte)
		schemaIsNamed := false
		if named, ok := schemaType.(*types.Named); ok {
			schemaType = named.Underlying()
			schemaNamedType = named
			schemaIsNamed = true
		}
		if named, ok := dataType.(*types.Named); ok {
			dataType = named.Underlying()
			dataNamedType = named
		} else if schemaIsNamed {
			// Schema was named but data wasn't - this is an error
			return nil, fmt.Errorf("incompatible types: data kind %v != schema kind %v", dataType.String(), schemaType.String())
		}
		if schemaIsNamed {
			continue
		}

		// Resolve pointers - must match on both sides
		if ptr, ok := schemaType.(*types.Pointer); ok {
			// Reject multi-level pointers (**T). Reflection only dereferences a
			// single level; flattening the extra indirection here would emit
			// accessors against a pointer type that do not compile.
			if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
				return nil, fmt.Errorf("unsupported multi-level pointer type %v", originalType)
			}
			ptrType = ptr
			schemaType = ptr.Elem()
			desc.GoTypeFlags |= ssztypes.GoTypeFlagIsPointer
			if ptr, ok := dataType.(*types.Pointer); ok {
				dataType = ptr.Elem()
			} else {
				return nil, fmt.Errorf("incompatible types: data kind %v != schema kind %v", dataType.String(), schemaType.String())
			}
			innerSchemaType = schemaType
			innerDataType = dataType
			continue
		}

		// Resolve aliases - allow independent unwrapping. Unalias (not Underlying)
		// so an alias to a generic instantiation like `type W = TypeWrapper[X]`
		// resolves to the *types.Named instantiation the next iteration records,
		// instead of skipping straight to the raw struct and losing the wrapper /
		// union identity and any type-level annotations.
		schemaIsAlias := false
		if alias, ok := schemaType.(*types.Alias); ok {
			schemaType = types.Unalias(alias)
			schemaIsAlias = true
		}
		if alias, ok := dataType.(*types.Alias); ok {
			dataType = types.Unalias(alias)
		} else if schemaIsAlias {
			// Schema was alias but data wasn't - this is an error
			return nil, fmt.Errorf("incompatible types: data kind %v != schema kind %v", dataType.String(), schemaType.String())
		}
		if schemaIsAlias {
			continue
		}

		// If data type is still a named type or alias but schema is not, unwrap data
		if named, ok := dataType.(*types.Named); ok {
			dataType = named.Underlying()
			dataNamedType = named
			continue
		}
		if alias, ok := dataType.(*types.Alias); ok {
			dataType = types.Unalias(alias)
			continue
		}

		break
	}

	// Verify data and schema types have compatible base kinds
	if dataType != schemaType {
		schemaKindStr := p.getTypeKindString(schemaType)
		dataKindStr := p.getTypeKindString(dataType)
		if schemaKindStr != dataKindStr {
			return nil, fmt.Errorf("incompatible types: data kind %v != schema kind %v", dataKindStr, schemaKindStr)
		}
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsView
	}

	// A fully-delegated, already-implemented type handles every SSZ operation in
	// its own code, so the descriptor subtree below it is never consulted. When
	// such a type declares ssz-static, build a shallow descriptor from that
	// declaration and skip traversing — and validating — its subtree.
	//
	// This only applies to EXTERNAL types: a type that is itself being generated
	// in this run is registered in CompatFlags and must be traversed so its own
	// code can be emitted, regardless of whether it appears at the top level or as
	// a field of another generated type. A generated type registers both its plain
	// data key and any data|view pairings, so it is recognized when reached through
	// either a plain or a view reference; external fully-delegated types are
	// registered under neither. Field-level hints are already handled inline by the
	// caller (it strips delegation flags).
	beingGenerated := p.getCompatFlag(innerDataType, innerSchemaType) != 0 || p.getCompatFlag(innerDataType, innerDataType) != 0
	if p.AnnotationResolver != nil && len(typeHints) == 0 && len(sizeHints) == 0 && len(maxSizeHints) == 0 && !beingGenerated && p.fullyDelegatesSSZ(originalType) {
		if staticStr, ok := reflect.StructTag(p.AnnotationResolver(originalType)).Lookup("ssz-static"); ok {
			switch staticStr {
			case "true":
				// Static: the generated code resolves the exact size at runtime via
				// the type's own sizer, so drive sizing/offsets at runtime rather
				// than from a (here unknown) compile-time constant.
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasSizeExpr
			case "false":
				desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
			default:
				return nil, fmt.Errorf("invalid ssz-static value %q for type %v (must be \"true\" or \"false\")", staticStr, originalType)
			}
			p.detectCompatFlags(desc, originalType, innerDataType, innerSchemaType)
			// Delegate through the spec-aware dynamic methods, not the fastssz ones.
			desc.SszCompatFlags &^= ssztypes.SszCompatFlagFastSSZMarshaler | ssztypes.SszCompatFlagFastSSZHasher | ssztypes.SszCompatFlagHashTreeRootWith
			// A shallow descriptor must not be cached as the type's full descriptor.
			if cacheable {
				delete(p.cache, typeKey)
			}
			return desc, nil
		}
	}

	// Named types can carry SSZ annotations registered via sszutils.Annotate.
	// When the reference itself provides no hints, apply the annotation's hints
	// so the layout classification matches the reflection typecache (e.g. an
	// ssz-size annotated type is a fixed-size vector and must be embedded
	// inline in containers). References with explicit field-level hints keep
	// those (they override the annotation).
	if p.AnnotationResolver != nil && len(typeHints) == 0 && len(sizeHints) == 0 && len(maxSizeHints) == 0 {
		annotationType := originalType
		if ptr, ok := annotationType.(*types.Pointer); ok {
			annotationType = ptr.Elem()
		}
		if tag := p.AnnotationResolver(annotationType); tag != "" {
			annTypeHints, annSizeHints, annMaxSizeHints, err := ssztypes.ParseTags(tag)
			if err != nil {
				return nil, fmt.Errorf("failed to parse ssz annotation for type %v: %v", annotationType, err)
			}
			typeHints, sizeHints, maxSizeHints = annTypeHints, annSizeHints, annMaxSizeHints
		}
	}

	// Set kind based on underlying type
	switch t := schemaType.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			desc.Kind = reflect.Bool
		case types.Uint8:
			desc.Kind = reflect.Uint8
		case types.Uint16:
			desc.Kind = reflect.Uint16
		case types.Uint32:
			desc.Kind = reflect.Uint32
		case types.Uint64, types.Uint:
			desc.Kind = reflect.Uint64
		case types.String:
			desc.Kind = reflect.String
			desc.GoTypeFlags |= ssztypes.GoTypeFlagIsString
		default:
			if p.ExtendedTypes {
				switch t.Kind() {
				case types.Int8:
					desc.Kind = reflect.Int8
				case types.Int16:
					desc.Kind = reflect.Int16
				case types.Int32:
					desc.Kind = reflect.Int32
				case types.Int64:
					desc.Kind = reflect.Int64
				case types.Float32:
					desc.Kind = reflect.Float32
				case types.Float64:
					desc.Kind = reflect.Float64
				default:
					desc.Kind = reflect.Invalid
				}
			} else {
				desc.Kind = reflect.Invalid
			}
		}
	case *types.Array:
		desc.Kind = reflect.Array
	case *types.Slice:
		desc.Kind = reflect.Slice
	case *types.Struct:
		desc.Kind = reflect.Struct
	default:
		desc.Kind = reflect.Invalid
	}

	// Check dynamic size and max size hints (like reflection code)
	if len(sizeHints) > 0 {
		if sizeHints[0].Expr != "" {
			desc.SizeExpression = &sizeHints[0].Expr
		}
		if sizeHints[0].Bits {
			desc.SszTypeFlags |= ssztypes.SszTypeFlagHasBitSize
			desc.BitSize = sizeHints[0].Size
		}
		for _, hint := range sizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasDynamicSize
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasSizeExpr
			}
		}
	}

	if len(maxSizeHints) > 0 {
		if !maxSizeHints[0].NoValue {
			desc.SszTypeFlags |= ssztypes.SszTypeFlagHasLimit
			desc.Limit = maxSizeHints[0].Size
		}
		if maxSizeHints[0].Expr != "" {
			desc.MaxExpression = &maxSizeHints[0].Expr
		}
		for _, hint := range maxSizeHints {
			if hint.Custom {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasDynamicMax
			}
			if hint.Expr != "" {
				desc.SszTypeFlags |= ssztypes.SszTypeFlagHasMaxExpr
			}
		}
	}

	// Determine SSZ type - first use type hints if specified
	sszType := ssztypes.SszUnspecifiedType
	if len(typeHints) > 0 {
		sszType = typeHints[0].Type
	}

	if desc.Kind == reflect.String {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsString
	}

	// Auto-detect ssz type if not specified
	if sszType == ssztypes.SszUnspecifiedType {
		// Detect well-known types first (named types)
		var obj *types.TypeName
		if alias, ok := innerSchemaType.(*types.Alias); ok {
			innerSchemaType = types.Unalias(alias)
			if alias, ok := innerDataType.(*types.Alias); ok {
				innerDataType = types.Unalias(alias)
			} else {
				return nil, fmt.Errorf("incompatible types: data kind %v != schema kind %v", innerDataType.String(), innerSchemaType.String())
			}
		}
		if named, ok := innerSchemaType.(*types.Named); ok {
			obj = named.Obj()
		}

		if obj != nil && obj.Pkg() != nil {
			pkgPath := obj.Pkg().Path()
			typeName := obj.Name()

			switch {
			case pkgPath == pkgPathTime && typeName == typeNameTime:
				sszType = ssztypes.SszUint64Type
				desc.GoTypeFlags |= ssztypes.GoTypeFlagIsTime
			case pkgPath == "math/big" && typeName == "Int":
				if p.ExtendedTypes {
					sszType = ssztypes.SszBigIntType
				} else {
					return nil, fmt.Errorf("big.Int is not supported in SSZ (use unsigned integers instead)")
				}
			case pkgPath == "github.com/holiman/uint256" && typeName == "Int":
				sszType = ssztypes.SszUint256Type
			case pkgPath == "github.com/prysmaticlabs/go-bitfield" && typeName == "Bitlist":
				sszType = ssztypes.SszBitlistType
			case pkgPath == "github.com/OffchainLabs/go-bitfield" && typeName == "Bitlist":
				sszType = ssztypes.SszBitlistType
			case pkgPath == pkgPathDynssz && typeName == "CompatibleUnion":
				sszType = ssztypes.SszCompatibleUnionType
			case pkgPath == pkgPathDynssz && typeName == "Union":
				sszType = ssztypes.SszUnionType
			case pkgPath == pkgPathDynssz && typeName == "TypeWrapper":
				sszType = ssztypes.SszTypeWrapperType
			}
		}
	}

	if sszType == ssztypes.SszUnspecifiedType {
		switch desc.Kind {
		// basic types
		case reflect.Bool:
			sszType = ssztypes.SszBoolType
		case reflect.Uint8:
			sszType = ssztypes.SszUint8Type
		case reflect.Uint16:
			sszType = ssztypes.SszUint16Type
		case reflect.Uint32:
			sszType = ssztypes.SszUint32Type
		case reflect.Uint64:
			sszType = ssztypes.SszUint64Type

		// complex types
		case reflect.Struct:
			sszType = ssztypes.SszContainerType
		case reflect.Array:
			sszType = ssztypes.SszVectorType
		case reflect.Slice:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = ssztypes.SszVectorType
			} else if err := rejectZeroSizeHint(sizeHints); err != nil {
				return nil, err
			} else {
				sszType = ssztypes.SszListType
			}
		case reflect.String:
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				sszType = ssztypes.SszVectorType
			} else if err := rejectZeroSizeHint(sizeHints); err != nil {
				return nil, err
			} else {
				sszType = ssztypes.SszListType
			}

		// extended types
		case reflect.Int8:
			sszType = ssztypes.SszInt8Type
		case reflect.Int16:
			sszType = ssztypes.SszInt16Type
		case reflect.Int32:
			sszType = ssztypes.SszInt32Type
		case reflect.Int64:
			sszType = ssztypes.SszInt64Type
		case reflect.Float32:
			sszType = ssztypes.SszFloat32Type
		case reflect.Float64:
			sszType = ssztypes.SszFloat64Type

		// unsupported types
		default:
			// Check for unsupported basic types
			if basic, ok := schemaType.(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int, types.Uint:
					return nil, fmt.Errorf("signed or unsigned integers with unspecified size are not supported in SSZ")
				case types.Float32, types.Float64:
					return nil, fmt.Errorf("floating-point numbers are not supported in SSZ")
				case types.Complex64, types.Complex128:
					return nil, fmt.Errorf("complex numbers are not supported in SSZ")
				default:
				}
			}
			// Check for other unsupported types
			switch schemaType.(type) {
			case *types.Map:
				return nil, fmt.Errorf("maps are not supported in SSZ (use structs or arrays instead)")
			case *types.Chan:
				return nil, fmt.Errorf("channels are not supported in SSZ")
			case *types.Signature:
				return nil, fmt.Errorf("functions are not supported in SSZ")
			case *types.Interface:
				return nil, fmt.Errorf("interfaces are not supported in SSZ (use concrete types)")
			default:
				return nil, fmt.Errorf("unsupported type kind: %v", desc.Kind)
			}
		}

		// Special case for bitlists: named list types whose name contains
		// "Bitlist" are treated as bitlists, matching the reflection typecache
		// heuristic (ssztypes/typecache.go).
		if sszType == ssztypes.SszListType && schemaNamedType != nil &&
			strings.Contains(schemaNamedType.Obj().Name(), "Bitlist") {
			sszType = ssztypes.SszBitlistType
		}
	}

	desc.SszType = sszType

	// Check type compatibility and build descriptor based on SSZ type
	switch sszType {
	// basic types
	case ssztypes.SszBoolType:
		if desc.Kind != reflect.Bool {
			return nil, fmt.Errorf("bool ssz type can only be represented by bool types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("bool ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("bool ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case ssztypes.SszUint8Type:
		if desc.Kind != reflect.Uint8 {
			return nil, fmt.Errorf("uint8 ssz type can only be represented by uint8 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint8 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 1 {
			return nil, fmt.Errorf("uint8 ssz type must be ssz-size:1, got %v", sizeHints[0].Size)
		}
		desc.Size = 1
	case ssztypes.SszUint16Type:
		if desc.Kind != reflect.Uint16 {
			return nil, fmt.Errorf("uint16 ssz type can only be represented by uint16 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint16 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 2 {
			return nil, fmt.Errorf("uint16 ssz type must be ssz-size:2, got %v", sizeHints[0].Size)
		}
		desc.Size = 2
	case ssztypes.SszUint32Type:
		if desc.Kind != reflect.Uint32 {
			return nil, fmt.Errorf("uint32 ssz type can only be represented by uint32 types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint32 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 4 {
			return nil, fmt.Errorf("uint32 ssz type must be ssz-size:4, got %v", sizeHints[0].Size)
		}
		desc.Size = 4
	case ssztypes.SszUint64Type:
		if desc.Kind != reflect.Uint64 && desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
			return nil, fmt.Errorf("uint64 ssz type can only be represented by uint64 or time.Time types, got %v", desc.Kind)
		}
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint64 ssz type cannot be limited by bits, use regular size tag instead")
		}
		if len(sizeHints) > 0 && sizeHints[0].Size != 8 {
			return nil, fmt.Errorf("uint64 ssz type must be ssz-size:8, got %v", sizeHints[0].Size)
		}
		desc.Size = 8
	case ssztypes.SszUint128Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint128 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := p.buildUint128Descriptor(desc, schemaType)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszUint256Type:
		if len(sizeHints) > 0 && sizeHints[0].Bits {
			return nil, fmt.Errorf("uint256 ssz type cannot be limited by bits, use regular size tag instead")
		}
		err := p.buildUint256Descriptor(desc, schemaType)
		if err != nil {
			return nil, err
		}

	// complex types
	case ssztypes.SszTypeWrapperType:
		// Resolve both data and schema types to named types
		if dataNamedType == nil {
			return nil, fmt.Errorf("data TypeWrapper must be a named type")
		}
		err := p.buildTypeWrapperDescriptor(desc, dataNamedType, schemaNamedType, typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszContainerType, ssztypes.SszProgressiveContainerType:
		schemaStruct, ok := schemaType.(*types.Struct)
		if !ok {
			return nil, fmt.Errorf("container ssz type can only be represented by struct types, got %v", desc.Kind)
		}
		// Resolve data type to underlying struct
		dataStruct, ok := dataType.(*types.Struct)
		if !ok {
			return nil, fmt.Errorf("data container must be a struct type")
		}
		err := p.buildContainerDescriptor(desc, dataStruct, schemaStruct)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszVectorType, ssztypes.SszBitvectorType:
		// Resolve data type for element type traversal
		err := p.buildVectorDescriptor(desc, dataType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszListType, ssztypes.SszProgressiveListType:
		// Resolve data type for element type traversal
		err := p.buildListDescriptor(desc, dataType, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszBitlistType, ssztypes.SszProgressiveBitlistType:
		err := p.buildBitlistDescriptor(desc, schemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszCompatibleUnionType:
		// Resolve both data and schema types to named types
		if dataNamedType == nil {
			return nil, fmt.Errorf("data CompatibleUnion must be a named type")
		}
		err := p.buildCompatibleUnionDescriptor(desc, dataNamedType, schemaNamedType)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszUnionType:
		if dataNamedType == nil {
			return nil, fmt.Errorf("data Union must be a named type")
		}
		err := p.buildUnionDescriptor(desc, dataNamedType, schemaNamedType)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszCustomType:
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			desc.Size = sizeHints[0].Size
			if sizeHints[0].Bits {
				desc.BitSize = sizeHints[0].Size
				desc.Size = (desc.Size + 7) / 8 // ceil up to the next multiple of 8
			}
		} else {
			desc.Size = 0
			desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
		}

	// extended types
	case ssztypes.SszInt8Type:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int8 {
			return nil, fmt.Errorf("int8 ssz type can only be represented by int8 types, got %v", desc.Kind)
		}
		desc.Size = 1
	case ssztypes.SszInt16Type:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int16 {
			return nil, fmt.Errorf("int16 ssz type can only be represented by int16 types, got %v", desc.Kind)
		}
		desc.Size = 2
	case ssztypes.SszInt32Type:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int32 {
			return nil, fmt.Errorf("int32 ssz type can only be represented by int32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case ssztypes.SszInt64Type:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("signed integers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Int64 {
			return nil, fmt.Errorf("int64 ssz type can only be represented by int64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case ssztypes.SszFloat32Type:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float32 {
			return nil, fmt.Errorf("float32 ssz type can only be represented by float32 types, got %v", desc.Kind)
		}
		desc.Size = 4
	case ssztypes.SszFloat64Type:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("floating-point numbers are not supported in SSZ (use unsigned integers instead)")
		}
		if desc.Kind != reflect.Float64 {
			return nil, fmt.Errorf("float64 ssz type can only be represented by float64 types, got %v", desc.Kind)
		}
		desc.Size = 8
	case ssztypes.SszOptionalType:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("optional types are not supported in SSZ (use unsigned integers instead)")
		}
		if ptrType == nil {
			return nil, fmt.Errorf("optional ssz type can only be represented by pointer types, got %v", desc.Kind)
		}
		err := p.buildOptionalDescriptor(desc, innerDataType, innerSchemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszOptionalListType:
		// optional-list expresses a pointer as a canonical List[T, 1]; allowed without ExtendedTypes
		if ptrType == nil {
			return nil, fmt.Errorf("optional-list ssz type can only be represented by pointer types, got %v", desc.Kind)
		}
		err := p.buildOptionalListDescriptor(desc, innerDataType, innerSchemaType, sizeHints, maxSizeHints, typeHints)
		if err != nil {
			return nil, err
		}
	case ssztypes.SszBigIntType:
		if !p.ExtendedTypes {
			return nil, fmt.Errorf("big.Int is not supported in SSZ (use unsigned integers instead)")
		}
		err := p.buildBigIntDescriptor(desc, dataType)
		if err != nil {
			return nil, err
		}
	default:
	}

	if desc.SszTypeFlags&ssztypes.SszTypeFlagHasBitSize != 0 && desc.SszType != ssztypes.SszBitvectorType && desc.SszType != ssztypes.SszBitlistType {
		return nil, fmt.Errorf("bit size tag is only allowed for bitvector or bitlist types, got %v", desc.SszType)
	}

	p.detectCompatFlags(desc, originalType, innerDataType, innerSchemaType)

	// When caller-level hints override the type's own annotation, don't delegate
	// to the type's generated methods — they have the annotation's limits baked
	// in. Process inline instead so the hints are respected. Gated on the hints
	// the caller passed (annotation-derived hints assigned above do not count),
	// mirroring the reflection typecache.
	if (len(callerSizeHints) > 0 || len(callerMaxSizeHints) > 0) && desc.SszType != ssztypes.SszCustomType {
		desc.SszCompatFlags &^= ssztypes.SszCompatFlagDynamicMarshaler |
			ssztypes.SszCompatFlagDynamicUnmarshaler |
			ssztypes.SszCompatFlagDynamicSizer |
			ssztypes.SszCompatFlagDynamicHashRoot |
			ssztypes.SszCompatFlagDynamicEncoder |
			ssztypes.SszCompatFlagDynamicDecoder |
			ssztypes.SszCompatFlagFastSSZMarshaler |
			ssztypes.SszCompatFlagFastSSZHasher |
			ssztypes.SszCompatFlagHashTreeRootWith
	}

	// Optional and optional-list reshape the encoding around the inner type
	// (presence byte / List[T,1] framing). The inner type's own SSZ methods
	// must not be invoked at this level — they would skip the framing and
	// emit the inner value as if it were the canonical encoding.
	if desc.SszType == ssztypes.SszOptionalType || desc.SszType == ssztypes.SszOptionalListType {
		desc.SszCompatFlags &^= ssztypes.SszCompatFlagDynamicMarshaler |
			ssztypes.SszCompatFlagDynamicUnmarshaler |
			ssztypes.SszCompatFlagDynamicSizer |
			ssztypes.SszCompatFlagDynamicHashRoot |
			ssztypes.SszCompatFlagDynamicEncoder |
			ssztypes.SszCompatFlagDynamicDecoder |
			ssztypes.SszCompatFlagFastSSZMarshaler |
			ssztypes.SszCompatFlagFastSSZHasher |
			ssztypes.SszCompatFlagHashTreeRootWith
	}

	// Per the SSZ spec, containers (including progressive containers) must have
	// at least one field. Reject a struct that would be encoded field-by-field
	// with no SSZ-encodable (exported) fields. Types that delegate to their own
	// SSZ methods (any compat flag set) are exempt: they do not use the plain
	// container layout.
	if desc.SszType == ssztypes.SszContainerType || desc.SszType == ssztypes.SszProgressiveContainerType {
		if desc.SszCompatFlags == 0 && desc.ContainerDesc != nil && len(desc.ContainerDesc.Fields) == 0 {
			return nil, fmt.Errorf("container type has no SSZ fields, which is invalid per the SSZ spec")
		}
	}

	if desc.SszType == ssztypes.SszCustomType {
		isCompatible := desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZHasher != 0
		// isCompatible = isCompatible || (desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicMarshaler != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicUnmarshaler != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicSizer != 0 && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicHashRoot != 0)

		if !isCompatible {
			return nil, fmt.Errorf("custom ssz type requires fastssz marshaler, unmarshaler and hasher implementations")
		}
	}

	if !cacheable {
		// Cache the hinted build under the caller's hints. Clone the slices: they
		// may alias a caller-owned parse result, while the cached variant must
		// own its key.
		variant := &parserHintedVariant{
			desc:         desc,
			typeHints:    slices.Clone(callerTypeHints),
			sizeHints:    slices.Clone(callerSizeHints),
			maxSizeHints: slices.Clone(callerMaxSizeHints),
		}
		p.hintedCache[typeKey] = append(p.hintedCache[typeKey], variant)
		p.pendingKeys = append(p.pendingKeys, parserPendingKey{key: typeKey, hinted: true})
	}

	return desc, nil
}

//nolint:dupl // intentionally similar to buildUint256Descriptor but handles 128-bit types
func (p *Parser) buildUint128Descriptor(desc *ssztypes.TypeDescriptor, typ types.Type) error {
	// Handle as [16]uint8, [2]uint64
	var elemType types.Type
	switch t := typ.(type) {
	case *types.Array:
		elemType = t.Elem()
		if t.Len() == 16 {
			if elem, ok := types.Unalias(t.Elem()).(*types.Basic); ok && elem.Kind() == types.Uint8 {
				desc.Size = 16
			}
		} else if t.Len() == 2 {
			if elem, ok := types.Unalias(t.Elem()).(*types.Basic); ok && elem.Kind() == types.Uint64 {
				desc.Size = 16
			}
		}
	case *types.Slice:
		elemType = t.Elem()
		if elem, ok := types.Unalias(t.Elem()).(*types.Basic); ok {
			if elem.Kind() == types.Uint8 {
				desc.Size = 16
			} else if elem.Kind() == types.Uint64 {
				desc.Size = 16
			}
		}
	}

	if desc.Size == 0 {
		return fmt.Errorf("uint128 ssz type can only be represented by [16]uint8 or [2]uint64 types")
	}

	// Build element descriptor (element types use same type for data and schema)
	elemDesc, err := p.buildTypeDescriptor(elemType, elemType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to build vector element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc
	desc.Len = desc.Size / elemDesc.Size

	// Set byte array flag for byte types
	if p.isByteType(elemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	return nil
}

//nolint:dupl // intentionally similar to buildUint128Descriptor but handles 256-bit types
func (p *Parser) buildUint256Descriptor(desc *ssztypes.TypeDescriptor, typ types.Type) error {
	// Handle as [32]uint8, [4]uint64
	var elemType types.Type
	switch t := typ.(type) {
	case *types.Array:
		elemType = t.Elem()
		if t.Len() == 32 {
			if elem, ok := types.Unalias(t.Elem()).(*types.Basic); ok && elem.Kind() == types.Uint8 {
				desc.Size = 32
			}
		} else if t.Len() == 4 {
			if elem, ok := types.Unalias(t.Elem()).(*types.Basic); ok && elem.Kind() == types.Uint64 {
				desc.Size = 32
			}
		}
	case *types.Slice:
		elemType = t.Elem()
		if elem, ok := types.Unalias(t.Elem()).(*types.Basic); ok {
			if elem.Kind() == types.Uint8 {
				desc.Size = 32
			} else if elem.Kind() == types.Uint64 {
				desc.Size = 32
			}
		}
	}

	if desc.Size == 0 {
		return fmt.Errorf("uint256 ssz type can only be represented by [32]uint8 or [4]uint64 types")
	}

	// Build element descriptor (element types use same type for data and schema)
	elemDesc, err := p.buildTypeDescriptor(elemType, elemType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to build vector element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc
	desc.Len = desc.Size / elemDesc.Size

	// Set byte array flag for byte types
	if p.isByteType(elemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	return nil
}

func (p *Parser) buildContainerDescriptor(desc *ssztypes.TypeDescriptor, dataStruct, schemaStruct *types.Struct) error {
	fields := []ssztypes.FieldDescriptor{}
	dynFields := []ssztypes.DynFieldDescriptor{}
	size := uint32(0)
	isDynamic := false

	// Progressive-container detection: tracks whether any field carries an
	// ssz-index tag, and (parallel to fields) which ones do.
	hasAnyIndexTag := false
	fieldHasIndex := []bool{}

	// Check if we're using a view descriptor (data and schema types differ)
	isViewDescriptor := dataStruct != schemaStruct

	// Build a map of data field names to their types when using view descriptors
	var dataFieldMap map[string]types.Type
	if isViewDescriptor {
		dataFieldMap = make(map[string]types.Type, dataStruct.NumFields())
		for i := 0; i < dataStruct.NumFields(); i++ {
			dataField := dataStruct.Field(i)
			dataFieldMap[dataField.Name()] = dataField.Type()
		}
	}

	// Iterate over schema fields (determines SSZ layout)
	for i := 0; i < schemaStruct.NumFields(); i++ {
		schemaField := schemaStruct.Field(i)
		fieldName := schemaField.Name()
		if !schemaField.Exported() || fieldName == "_" || ssztypes.IsSszExcluded(reflect.StructTag(schemaStruct.Tag(i))) {
			continue
		}

		// Determine data and schema field types
		schemaFieldType := schemaField.Type()

		// Field-level tags override the type's registered annotation per key:
		// join the two (field tag first — Lookup returns the first occurrence)
		// so annotation keys the field does not override still apply.
		fieldTag := schemaStruct.Tag(i)
		if p.AnnotationResolver != nil {
			annotationType := schemaFieldType
			if ptr, ok := annotationType.(*types.Pointer); ok {
				annotationType = ptr.Elem()
			}
			if annTag := p.AnnotationResolver(annotationType); annTag != "" {
				fieldTag = string(ssztypes.JoinFieldAnnotationTag(reflect.StructTag(fieldTag), annTag))
			}
		}

		typeHints, sizeHints, maxSizeHints, err := p.parseFieldTags(fieldTag)
		if err != nil {
			return fmt.Errorf("failed to parse tags for field %v: %v", schemaField.Name(), err)
		}
		var dataFieldType types.Type
		if isViewDescriptor {
			// Look up corresponding data field by name
			var ok bool
			dataFieldType, ok = dataFieldMap[fieldName]
			if !ok {
				return fmt.Errorf("data type missing field %q defined in schema", fieldName)
			}
		} else {
			dataFieldType = schemaFieldType
		}

		// Build type descriptor traversing both type trees
		typeDesc, err := p.buildTypeDescriptor(dataFieldType, schemaFieldType, typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return fmt.Errorf("failed to build field %v descriptor: %v", schemaField.Name(), err)
		}

		fieldDesc := ssztypes.FieldDescriptor{
			Name: schemaField.Name(),
			Type: typeDesc,
		}

		// Handle ssz-index for progressive containers - extract from original tag parsing
		hasIndex := false
		if indexStr := p.extractSszIndex(schemaStruct.Tag(i)); indexStr != "" {
			idx, err := strconv.ParseUint(indexStr, 10, 16)
			if err != nil {
				return fmt.Errorf("invalid ssz-index: %v", indexStr)
			}
			// EIP-7495 progressive containers support at most 256 active fields
			// (a larger bitvector also has no stable single-chunk mixin), and
			// union selectors are a single byte.
			if idx > 255 {
				return fmt.Errorf("ssz-index %d for field %q exceeds the supported maximum of 255", idx, schemaField.Name())
			}
			fieldDesc.SszIndex = uint16(idx)
			hasIndex = true
			hasAnyIndexTag = true
		}

		if typeDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
			// Dynamic field
			dynFieldDesc := ssztypes.DynFieldDescriptor{
				Field:        &fieldDesc,
				HeaderOffset: size,
				Index:        int16(len(fields)),
			}
			dynFields = append(dynFields, dynFieldDesc)
			isDynamic = true
			size += 4
		} else {
			size += typeDesc.Size
		}

		desc.SszTypeFlags |= fieldDesc.Type.SszTypeFlags & (ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)
		fields = append(fields, fieldDesc)
		fieldHasIndex = append(fieldHasIndex, hasIndex)
	}

	// A container must have at least one SSZ-encodable (exported, non-excluded)
	// field. A zero-field container serializes to 0 bytes with an ambiguous root,
	// which the SSZ spec forbids; reflection rejects it too. Reject it here so the
	// generator cannot emit methods that bypass that validation.
	if len(fields) == 0 {
		return fmt.Errorf("container type has no SSZ fields, which is invalid per the SSZ spec")
	}

	containerDesc := &ssztypes.ContainerDescriptor{
		Fields:    fields,
		DynFields: dynFields,
	}
	desc.ContainerDesc = containerDesc

	// A container is progressive if it carries ssz-index tags or was declared
	// progressive via the ssz-type hint. Assign active-field indices: a tagged
	// field keeps its (strictly increasing) index; an untagged field takes the
	// previous field's index + 1. With no tags this yields the default 0, 1, 2, ...
	// sequence, so a progressive container never falls back to all-zero indices
	// (a 1-bit active_fields bitvector for N field roots is illegal per EIP-8016).
	if hasAnyIndexTag || desc.SszType == ssztypes.SszProgressiveContainerType {
		nextIndex := uint16(0)
		for i := range fields {
			if i < len(fieldHasIndex) && fieldHasIndex[i] {
				if fields[i].SszIndex < nextIndex {
					return fmt.Errorf("progressive container requires increasing ssz-index values (field %s has index %d, expected >= %d)",
						fields[i].Name, fields[i].SszIndex, nextIndex)
				}
			} else {
				fields[i].SszIndex = nextIndex
			}
			nextIndex = fields[i].SszIndex + 1
		}
		desc.SszType = ssztypes.SszProgressiveContainerType
	}

	desc.Len = size
	if isDynamic {
		desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
		desc.Size = 0
	} else {
		desc.Size = size
	}

	return nil
}

func (p *Parser) buildVectorDescriptor(desc *ssztypes.TypeDescriptor, dataType, schemaType types.Type, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, typeHints []ssztypes.SszTypeHint) error {
	var schemaElemType types.Type
	var dataElemType types.Type
	var length uint32

	// Extract element type from schema (determines SSZ layout)
	switch t := schemaType.(type) {
	case *types.Array:
		schemaElemType = t.Elem()
		length = uint32(t.Len())
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			byteSize := sizeHints[0].Size
			if sizeHints[0].Bits {
				byteSize = (byteSize + 7) / 8 // ceil up to the next multiple of 8
			}
			if byteSize > length {
				return fmt.Errorf("size hint for vector type is greater than the length of the array (%d > %d)", byteSize, length)
			}
			length = byteSize
		}
	case *types.Slice:
		schemaElemType = t.Elem()
		if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
			length = sizeHints[0].Size
			if sizeHints[0].Bits {
				length = (length + 7) / 8 // ceil up to the next multiple of 8
			}
		} else {
			return fmt.Errorf("vector slice type requires explicit size hint")
		}
	case *types.Basic:
		if t.Kind() == types.String {
			// String as vector
			if len(sizeHints) > 0 && sizeHints[0].Size > 0 {
				length = sizeHints[0].Size
				if sizeHints[0].Bits {
					length = (length + 7) / 8 // ceil up to the next multiple of 8
				}
				desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
				schemaElemType = byteType
			} else {
				return fmt.Errorf("string vector type requires explicit size hint")
			}
		} else {
			return fmt.Errorf("unsupported vector base type: %v", t.Kind())
		}
	default:
		return fmt.Errorf("unsupported vector type: %T", schemaType)
	}

	// Extract element type from data type
	switch t := dataType.(type) {
	case *types.Array:
		dataElemType = t.Elem()
	case *types.Slice:
		dataElemType = t.Elem()
	case *types.Basic:
		if t.Kind() == types.String {
			dataElemType = byteType
		} else {
			dataElemType = schemaElemType // fallback to schema for primitives
		}
	default:
		dataElemType = schemaElemType // fallback to schema
	}

	childTypeHints := []ssztypes.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}
	childSizeHints := []ssztypes.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}
	childMaxSizeHints := []ssztypes.SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	// Build element descriptor traversing both type trees
	elemDesc, err := p.buildTypeDescriptor(dataElemType, schemaElemType, childTypeHints, childSizeHints, childMaxSizeHints)
	if err != nil {
		return fmt.Errorf("failed to build vector element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc
	desc.Len = length

	// Per the SSZ spec, Vector[type, 0] and Bitvector[0] are illegal: a vector
	// must have a length greater than zero (e.g. a [0]T array).
	if desc.Len == 0 {
		return fmt.Errorf("vector type %v has zero length, which is invalid per the SSZ spec", schemaType)
	}

	// Set byte array flag for byte types
	if p.isByteType(schemaElemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	// Calculate size
	if elemDesc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic != 0 {
		desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
		desc.Size = 0
	} else {
		desc.Size = length * elemDesc.Size
	}

	return nil
}

func (p *Parser) buildListDescriptor(desc *ssztypes.TypeDescriptor, dataType, schemaType types.Type, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, typeHints []ssztypes.SszTypeHint) error {
	var schemaElemType types.Type
	var dataElemType types.Type

	// Extract element type from schema (determines SSZ layout)
	switch t := schemaType.(type) {
	case *types.Slice:
		schemaElemType = t.Elem()
	case *types.Basic:
		if t.Kind() == types.String {
			// String as list - set byte array flag and make dynamic
			desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
			desc.Size = 0
			desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
			schemaElemType = byteType
		} else {
			return fmt.Errorf("unsupported list base type: %v", t.Kind())
		}
	default:
		return fmt.Errorf("unsupported list type: %T", schemaType)
	}

	// Extract element type from data type
	switch t := dataType.(type) {
	case *types.Slice:
		dataElemType = t.Elem()
	case *types.Basic:
		if t.Kind() == types.String {
			dataElemType = byteType
		} else {
			dataElemType = schemaElemType // fallback to schema for primitives
		}
	default:
		dataElemType = schemaElemType // fallback to schema
	}

	childTypeHints := []ssztypes.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}
	childSizeHints := []ssztypes.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}
	childMaxSizeHints := []ssztypes.SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	// Lists cannot have a fixed ssz-size; that's a vector.
	if len(sizeHints) > 0 && sizeHints[0].Size > 0 && !sizeHints[0].Dynamic {
		return fmt.Errorf("list types cannot have a fixed ssz-size (use ssz-max for lists, or ssz-size with vector type)")
	}

	// A list is always dynamic (offset-encoded). Mark it before descending into
	// the element so a recursive back-edge landing on this descriptor already
	// sees its final layout classification.
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	desc.Size = 0

	// Build element descriptor traversing both type trees. A list is offset-encoded
	// (variable-length), so descending into its element is a legal recursion
	// boundary — bump the dynamic-nesting depth for cycle detection.
	p.dynDepth++
	elemDesc, err := p.buildTypeDescriptor(dataElemType, schemaElemType, childTypeHints, childSizeHints, childMaxSizeHints)
	p.dynDepth--
	if err != nil {
		return fmt.Errorf("failed to build list element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc

	// Set byte array flag for byte types
	if p.isByteType(schemaElemType) {
		desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray
	}

	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	return nil
}

func (p *Parser) buildBitlistDescriptor(desc *ssztypes.TypeDescriptor, typ types.Type, _ []ssztypes.SszSizeHint, _ []ssztypes.SszMaxSizeHint, _ []ssztypes.SszTypeHint) error {
	var elemType types.Type

	switch t := typ.(type) {
	case *types.Slice:
		elemType = t.Elem()
	default:
		return fmt.Errorf("bitlist type can only be represented by slice types, got %T", typ)
	}

	// Build element descriptor (element types use same type for data and schema)
	elemDesc, err := p.buildTypeDescriptor(elemType, elemType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to build bitlist element descriptor: %v", err)
	}
	desc.ElemDesc = elemDesc

	// Bitlist must use byte (uint8) elements
	if elemDesc.Kind != reflect.Uint8 {
		return fmt.Errorf("bitlist ssz type can only be represented by byte slices, got []%v", elemDesc.Kind)
	}

	// Bitlists are always dynamic
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	desc.Size = 0
	desc.GoTypeFlags |= ssztypes.GoTypeFlagIsByteArray

	return nil
}

func (p *Parser) buildCompatibleUnionDescriptor(desc *ssztypes.TypeDescriptor, dataNamed, schemaNamed *types.Named) error {
	// Extract generic type arguments from CompatibleUnion[T] for schema (determines SSZ layout)
	schemaTypeArgs := schemaNamed.TypeArgs()
	if schemaTypeArgs == nil || schemaTypeArgs.Len() != 1 {
		return fmt.Errorf("CompatibleUnion must have exactly 1 type argument")
	}

	schemaDescriptorType := schemaTypeArgs.At(0) // T - the schema descriptor struct

	// The descriptor must be a struct type
	schemaDescriptorStruct, ok := schemaDescriptorType.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("CompatibleUnion descriptor must be a struct, got %T", schemaDescriptorType.Underlying())
	}

	// Check if we're using a view descriptor (data and schema types differ)
	isViewDescriptor := dataNamed != schemaNamed

	// Extract data descriptor struct if using view descriptor
	var dataDescriptorStruct *types.Struct
	var dataVariantMap map[string]types.Type
	if isViewDescriptor {
		dataTypeArgs := dataNamed.TypeArgs()
		if dataTypeArgs == nil || dataTypeArgs.Len() != 1 {
			return fmt.Errorf("data CompatibleUnion must have exactly 1 type argument")
		}
		dataDescriptorType := dataTypeArgs.At(0)
		var ok bool
		dataDescriptorStruct, ok = dataDescriptorType.Underlying().(*types.Struct)
		if !ok {
			return fmt.Errorf("data CompatibleUnion descriptor must be a struct, got %T", dataDescriptorType.Underlying())
		}
		// Build map of data variant field names to types
		dataVariantMap = make(map[string]types.Type, dataDescriptorStruct.NumFields())
		for i := 0; i < dataDescriptorStruct.NumFields(); i++ {
			field := dataDescriptorStruct.Field(i)
			dataVariantMap[field.Name()] = field.Type()
		}
	}

	// An ssz-index tag assigns an explicit selector value, e.g. 1-based
	// selectors for EIP-8016 conformant unions. Mixing tagged and untagged
	// variants would silently renumber the untagged ones, so require
	// all-or-none up front.
	indexTagCount := 0
	for i := 0; i < schemaDescriptorStruct.NumFields(); i++ {
		if p.extractSszIndex(schemaDescriptorStruct.Tag(i)) != "" {
			indexTagCount++
		}
	}
	if indexTagCount > 0 && indexTagCount != schemaDescriptorStruct.NumFields() {
		return fmt.Errorf("union descriptor mixes fields with and without ssz-index tags (all variants must carry one when any does)")
	}

	// EIP-8016 restricts union selectors to 1..127: 0 and the range above 127
	// are reserved. Default numbering follows field order starting at 1, so the
	// descriptor can hold at most 127 variants.
	if schemaDescriptorStruct.NumFields() > 127 {
		return fmt.Errorf("union descriptor has %d variants, but selectors are limited to 1..127", schemaDescriptorStruct.NumFields())
	}

	// Build union variants iterating over schema (determines SSZ layout)
	variantInfo := make(map[uint8]*ssztypes.TypeDescriptor)

	for i := 0; i < schemaDescriptorStruct.NumFields(); i++ {
		schemaField := schemaDescriptorStruct.Field(i)
		variantIndex := uint8(i) + 1 // Field order determines the default variant selector, starting at 1

		if indexStr := p.extractSszIndex(schemaDescriptorStruct.Tag(i)); indexStr != "" {
			idx, err := strconv.ParseUint(indexStr, 10, 8)
			if err != nil {
				return fmt.Errorf("invalid ssz-index for union variant %s: %v", schemaField.Name(), indexStr)
			}
			if idx < 1 || idx > 127 {
				return fmt.Errorf("union selector %d for field %s is outside the valid range 1..127", idx, schemaField.Name())
			}
			variantIndex = uint8(idx)
		}

		if _, exists := variantInfo[variantIndex]; exists {
			return fmt.Errorf("duplicate union selector %d (field %s)", variantIndex, schemaField.Name())
		}

		// Extract SSZ annotations from the schema field
		typeHints, sizeHints, maxSizeHints, err := p.parseFieldTags(schemaDescriptorStruct.Tag(i))
		if err != nil {
			return fmt.Errorf("failed to parse union variant field %s tags: %v", schemaField.Name(), err)
		}

		// Determine data and schema variant types
		schemaVariantType := schemaField.Type()
		var dataVariantType types.Type
		if isViewDescriptor {
			var ok bool
			dataVariantType, ok = dataVariantMap[schemaField.Name()]
			if !ok {
				return fmt.Errorf("data union missing variant %q defined in schema", schemaField.Name())
			}
		} else {
			dataVariantType = schemaVariantType
		}

		// Build variant type descriptor traversing both type trees
		variantDesc, err := p.buildTypeDescriptor(dataVariantType, schemaVariantType, typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return fmt.Errorf("failed to build union variant %d descriptor: %v", variantIndex, err)
		}

		variantInfo[variantIndex] = variantDesc
	}

	if len(variantInfo) == 0 {
		return fmt.Errorf("union descriptor struct has no fields")
	}

	desc.UnionVariants = variantInfo
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	desc.Size = 0

	return nil
}

// isNoneMarkerType reports whether a descriptor field type is the classic
// union None marker (dynssz.None).
func (p *Parser) isNoneMarkerType(typ types.Type) bool {
	named, ok := types.Unalias(typ).(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "None" && named.Obj().Pkg().Path() == pkgPathDynssz
}

// buildUnionDescriptor builds a descriptor for classic spec unions. Selectors
// are the descriptor struct's 0-based field positions per the SSZ spec: the
// None marker is only legal as the first field (making selector 0 the empty
// option, with at least one further variant), selectors above 127 are
// reserved, and positional numbering leaves no room for ssz-index tags.
func (p *Parser) buildUnionDescriptor(desc *ssztypes.TypeDescriptor, dataNamed, schemaNamed *types.Named) error {
	schemaTypeArgs := schemaNamed.TypeArgs()
	if schemaTypeArgs == nil || schemaTypeArgs.Len() != 1 {
		return fmt.Errorf("union must have exactly 1 type argument")
	}

	schemaDescriptorType := schemaTypeArgs.At(0)
	schemaDescriptorStruct, ok := schemaDescriptorType.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("union descriptor must be a struct, got %T", schemaDescriptorType.Underlying())
	}

	isViewDescriptor := dataNamed != schemaNamed

	var dataVariantMap map[string]types.Type
	if isViewDescriptor {
		dataTypeArgs := dataNamed.TypeArgs()
		if dataTypeArgs == nil || dataTypeArgs.Len() != 1 {
			return fmt.Errorf("data Union must have exactly 1 type argument")
		}
		dataDescriptorStruct, ok := dataTypeArgs.At(0).Underlying().(*types.Struct)
		if !ok {
			return fmt.Errorf("data Union descriptor must be a struct, got %T", dataTypeArgs.At(0).Underlying())
		}
		dataVariantMap = make(map[string]types.Type, dataDescriptorStruct.NumFields())
		for i := 0; i < dataDescriptorStruct.NumFields(); i++ {
			field := dataDescriptorStruct.Field(i)
			dataVariantMap[field.Name()] = field.Type()
		}
	}

	numFields := schemaDescriptorStruct.NumFields()
	if numFields == 0 {
		return fmt.Errorf("union descriptor struct has no fields")
	}
	// Selectors above 127 are reserved, so positions 0..127 bound the count.
	if numFields > 128 {
		return fmt.Errorf("union descriptor has %d variants, but selectors are limited to 0..127", numFields)
	}

	variantInfo := make(map[uint8]*ssztypes.TypeDescriptor, numFields)

	for i := 0; i < numFields; i++ {
		schemaField := schemaDescriptorStruct.Field(i)

		if p.extractSszIndex(schemaDescriptorStruct.Tag(i)) != "" {
			return fmt.Errorf("union selectors are positional; ssz-index tag on field %s is not allowed", schemaField.Name())
		}

		if p.isNoneMarkerType(schemaField.Type()) {
			if i != 0 {
				return fmt.Errorf("the None option is only legal as the first union variant (field %s is at position %d)", schemaField.Name(), i)
			}
			if numFields < 2 {
				return fmt.Errorf("a union declaring None must offer at least one further variant")
			}
			desc.SszTypeFlags |= ssztypes.SszTypeFlagHasNoneVariant
			continue
		}

		typeHints, sizeHints, maxSizeHints, err := p.parseFieldTags(schemaDescriptorStruct.Tag(i))
		if err != nil {
			return fmt.Errorf("failed to parse union variant field %s tags: %v", schemaField.Name(), err)
		}

		schemaVariantType := schemaField.Type()
		var dataVariantType types.Type
		if isViewDescriptor {
			var ok bool
			dataVariantType, ok = dataVariantMap[schemaField.Name()]
			if !ok {
				return fmt.Errorf("data union missing variant %q defined in schema", schemaField.Name())
			}
		} else {
			dataVariantType = schemaVariantType
		}

		variantDesc, err := p.buildTypeDescriptor(dataVariantType, schemaVariantType, typeHints, sizeHints, maxSizeHints)
		if err != nil {
			return fmt.Errorf("failed to build union variant %d descriptor: %v", i, err)
		}

		variantInfo[uint8(i)] = variantDesc
	}

	desc.UnionVariants = variantInfo
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	desc.Size = 0

	return nil
}

func (p *Parser) buildTypeWrapperDescriptor(desc *ssztypes.TypeDescriptor, dataNamed, schemaNamed *types.Named, _ []ssztypes.SszTypeHint, _ []ssztypes.SszSizeHint, _ []ssztypes.SszMaxSizeHint) error {
	// Extract generic type arguments from TypeWrapper[D, T] for schema (determines SSZ layout)
	var schemaDescriptorType types.Type
	var schemaWrappedType types.Type
	var isTypeWrapper = false

	schemaTypeArgs := schemaNamed.TypeArgs()
	if schemaTypeArgs != nil && schemaTypeArgs.Len() == 2 {
		schemaDescriptorType = schemaTypeArgs.At(0) // D - the schema descriptor struct
		schemaWrappedType = schemaTypeArgs.At(1)    // T - the schema wrapped type
		isTypeWrapper = true
	} else {
		schemaDescriptorType = schemaNamed
	}

	// The descriptor must be a struct type
	schemaDescriptorStruct, ok := schemaDescriptorType.Underlying().(*types.Struct)
	if !ok {
		return fmt.Errorf("TypeWrapper descriptor must be a struct, got %T", schemaDescriptorType.Underlying())
	}

	// The descriptor must have exactly 1 field
	if schemaDescriptorStruct.NumFields() != 1 {
		return fmt.Errorf("TypeWrapper descriptor must have exactly 1 field, got %d", schemaDescriptorStruct.NumFields())
	}

	// Extract SSZ annotations from the schema descriptor field
	schemaField := schemaDescriptorStruct.Field(0)
	fieldTypeHints, fieldSizeHints, fieldMaxSizeHints, err := p.parseFieldTags(schemaDescriptorStruct.Tag(0))
	if err != nil {
		return fmt.Errorf("failed to parse TypeWrapper descriptor field tags: %v", err)
	}

	// Verify the schema field type matches the schema wrapped type
	if !isTypeWrapper {
		schemaWrappedType = schemaField.Type()
	} else if !types.Identical(schemaField.Type(), schemaWrappedType) {
		return fmt.Errorf("TypeWrapper descriptor field type %v does not match wrapped type %v", schemaField.Type(), schemaWrappedType)
	}

	// Determine data wrapped type
	var dataWrappedType types.Type
	if dataNamed != schemaNamed {
		// Extract data wrapped type from data TypeWrapper
		if isTypeWrapper {
			dataTypeArgs := dataNamed.TypeArgs()
			if dataTypeArgs == nil || dataTypeArgs.Len() != 2 {
				return fmt.Errorf("data TypeWrapper must have exactly 2 type arguments")
			}

			dataWrappedType = dataTypeArgs.At(1) // T - the data wrapped type
		} else {
			dataStruct, ok := dataNamed.Underlying().(*types.Struct)
			if !ok {
				return fmt.Errorf("data TypeWrapper descriptor must be a struct, got %T", dataNamed.Underlying())
			}
			if dataStruct.NumFields() != 1 {
				return fmt.Errorf("data TypeWrapper descriptor must have exactly 1 field, got %d", dataStruct.NumFields())
			}

			dataField := dataStruct.Field(0)
			dataWrappedType = dataField.Type()
		}
	} else {
		dataWrappedType = schemaWrappedType
	}

	// Build the wrapped type descriptor traversing both type trees
	wrappedDesc, err := p.buildTypeDescriptor(dataWrappedType, schemaWrappedType, fieldTypeHints, fieldSizeHints, fieldMaxSizeHints)
	if err != nil {
		return fmt.Errorf("failed to build TypeWrapper wrapped type descriptor: %v", err)
	}

	// Store wrapper information
	desc.ElemDesc = wrappedDesc

	// The TypeWrapper inherits properties from the wrapped type
	desc.Size = wrappedDesc.Size
	desc.SszTypeFlags |= wrappedDesc.SszTypeFlags & (ssztypes.SszTypeFlagIsDynamic | ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	return nil
}

func (p *Parser) buildOptionalDescriptor(desc *ssztypes.TypeDescriptor, dataType, schemaType types.Type, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, typeHints []ssztypes.SszTypeHint) error {
	// Optional is always dynamic size (1 byte for presence + variable data)
	desc.Size = 0
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic

	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer == 0 {
		return fmt.Errorf("optional ssz type can only be represented by pointer types, got %v", desc.Kind)
	}

	childSizeHints := []ssztypes.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []ssztypes.SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	childTypeHints := []ssztypes.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	// Optional is dynamic-size (presence-gated), a legal recursion boundary.
	p.dynDepth++
	elemDesc, err := p.buildTypeDescriptor(dataType, schemaType, childTypeHints, childSizeHints, childMaxSizeHints)
	p.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc

	// The Optional inherits properties from the child type
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (ssztypes.SszTypeFlagIsDynamic | ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	return nil
}

// buildOptionalListDescriptor builds a descriptor for optional-list types.
//
// An optional-list expresses a Go pointer as a canonical SSZ List[T, 1]:
// nil encodes as an empty list (no bytes), non-nil encodes as a list with a
// single element. Unlike SszOptionalType, this is a canonical SSZ encoding
// with no custom presence flag and is allowed regardless of ExtendedTypes.
func (p *Parser) buildOptionalListDescriptor(desc *ssztypes.TypeDescriptor, dataType, schemaType types.Type, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, typeHints []ssztypes.SszTypeHint) error {
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer == 0 {
		return fmt.Errorf("optional-list ssz type can only be represented by pointer types, got %v", desc.Kind)
	}

	childSizeHints := []ssztypes.SszSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	childMaxSizeHints := []ssztypes.SszMaxSizeHint{}
	if len(maxSizeHints) > 1 {
		childMaxSizeHints = maxSizeHints[1:]
	}

	childTypeHints := []ssztypes.SszTypeHint{}
	if len(typeHints) > 1 {
		childTypeHints = typeHints[1:]
	}

	// canonical List[T, 1]: always dynamic, limit fixed at 1 element. Mark it
	// before descending into the element so a recursive back-edge landing on this
	// descriptor already sees its final layout classification.
	desc.Size = 0
	desc.Limit = 1
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic | ssztypes.SszTypeFlagHasLimit

	// Optional-list is a canonical List[T, 1] (variable-length), a legal boundary.
	p.dynDepth++
	elemDesc, err := p.buildTypeDescriptor(dataType, schemaType, childTypeHints, childSizeHints, childMaxSizeHints)
	p.dynDepth--
	if err != nil {
		return err
	}

	desc.ElemDesc = elemDesc
	desc.SszTypeFlags |= elemDesc.SszTypeFlags & (ssztypes.SszTypeFlagHasDynamicSize | ssztypes.SszTypeFlagHasDynamicMax | ssztypes.SszTypeFlagHasSizeExpr | ssztypes.SszTypeFlagHasMaxExpr)

	return nil
}

func (p *Parser) buildBigIntDescriptor(desc *ssztypes.TypeDescriptor, _ types.Type) error {
	if desc.Kind != reflect.Struct {
		return fmt.Errorf("bigint type can only be represented by struct types, got %v", desc.Kind)
	}
	desc.Size = 0
	desc.SszTypeFlags |= ssztypes.SszTypeFlagIsDynamic
	return nil
}

func (p *Parser) parseFieldTags(tag string) (typeHints []ssztypes.SszTypeHint, sizeHints []ssztypes.SszSizeHint, maxSizeHints []ssztypes.SszMaxSizeHint, err error) {
	return ssztypes.ParseTags(tag)
}

// ParseTags parses SSZ annotations from a string in struct tag format.
// This is a convenience re-export of ssztypes.ParseTags for backward
// compatibility with code that imports codegen.ParseTags.
func ParseTags(tag string) ([]ssztypes.SszTypeHint, []ssztypes.SszSizeHint, []ssztypes.SszMaxSizeHint, error) {
	return ssztypes.ParseTags(tag)
}

// rejectZeroSizeHint rejects an explicit literal ssz-size:"0" on a slice or
// string: a zero-length vector is not valid SSZ, and silently degrading to an
// unbounded list would drop the intended constraint entirely. Dynamic ("?")
// and expression-based dimensions are unaffected. Mirrors the reflection
// typecache check.
func rejectZeroSizeHint(sizeHints []ssztypes.SszSizeHint) error {
	if len(sizeHints) > 0 && !sizeHints[0].Dynamic && sizeHints[0].Expr == "" && sizeHints[0].Size == 0 {
		return fmt.Errorf("ssz-size 0 is not a valid vector size (zero-length vectors are illegal in SSZ)")
	}
	return nil
}

func (p *Parser) extractSszIndex(tag string) string {
	if tag == "" {
		return ""
	}
	structTag := reflect.StructTag(tag)
	if index, ok := structTag.Lookup("ssz-index"); ok {
		return index
	}
	return ""
}

func (p *Parser) isByteType(typ types.Type) bool {
	basic, ok := types.Unalias(typ).(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

// getTypeKindString returns a string representation of a go/types type's kind.
func (p *Parser) getTypeKindString(typ types.Type) string {
	switch t := typ.(type) {
	case *types.Basic:
		return fmt.Sprintf("basic:%v", t.Kind())
	case *types.Array:
		return "array"
	case *types.Slice:
		return "slice"
	case *types.Struct:
		return "struct"
	case *types.Pointer:
		return "pointer"
	case *types.Map:
		return "map"
	case *types.Chan:
		return "chan"
	case *types.Signature:
		return "func"
	case *types.Interface:
		return "interface"
	default:
		return "unknown"
	}
}

// Interface compatibility checks using proper go/types interface implementation checking

func (p *Parser) getFastsszConvertCompatibility(typ types.Type) bool {
	methodSet := types.NewMethodSet(typ)
	return (p.hasMethodWithSignature(methodSet, "MarshalSSZTo", []string{typeNameByteSlice}, []string{typeNameByteSlice, "error"}) &&
		p.hasMethodWithSignature(methodSet, "SizeSSZ", []string{}, []string{"int"}) &&
		p.hasMethodWithSignature(methodSet, "UnmarshalSSZ", []string{typeNameByteSlice}, []string{"error"}))
}

func (p *Parser) getFastsszHashCompatibility(typ types.Type) bool {
	methodSet := types.NewMethodSet(typ)
	return (p.hasMethodWithSignature(methodSet, "HashTreeRoot", []string{}, []string{"[32]byte", "error"}))
}

func (p *Parser) getHashTreeRootWithCompatibility(typ types.Type) bool {
	// Check if type has HashTreeRootWith method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "HashTreeRootWith", []string{"-"}, []string{"error"})
}

func (p *Parser) getDynamicMarshalerCompatibility(typ types.Type) bool {
	// Check if type has MarshalSSZDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "MarshalSSZDyn", []string{"DynamicSpecs", typeNameByteSlice}, []string{typeNameByteSlice, "error"})
}

func (p *Parser) getDynamicUnmarshalerCompatibility(typ types.Type) bool {
	// Check if type has UnmarshalSSZDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "UnmarshalSSZDyn", []string{"DynamicSpecs", typeNameByteSlice}, []string{"error"})
}

func (p *Parser) getDynamicEncoderCompatibility(typ types.Type) bool {
	// Check if type has MarshalSSZEncoder method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "MarshalSSZEncoder", []string{"DynamicSpecs", "Encoder"}, []string{"error"})
}

func (p *Parser) getDynamicDecoderCompatibility(typ types.Type) bool {
	// Check if type has UnmarshalSSZDecoder method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "UnmarshalSSZDecoder", []string{"DynamicSpecs", "Decoder"}, []string{"error"})
}

func (p *Parser) getDynamicSizerCompatibility(typ types.Type) bool {
	// Check if type has SizeSSZDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "SizeSSZDyn", []string{"DynamicSpecs"}, []string{"int"})
}

func (p *Parser) getDynamicHashRootCompatibility(typ types.Type) bool {
	// Check if type has HashTreeRootWithDyn method
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "HashTreeRootWithDyn", []string{"DynamicSpecs", "HashWalker"}, []string{"error"})
}

// View interface compatibility checks for fork-dependent SSZ schemas.
// These interfaces return function pointers that can be used if the view is supported.

func (p *Parser) getDynamicViewMarshalerCompatibility(typ types.Type) bool {
	// Check if type has MarshalSSZDynView method: func(view any) func(DynamicSpecs, []byte) ([]byte, error)
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "MarshalSSZDynView", []string{"any"}, []string{"func"})
}

func (p *Parser) getDynamicViewUnmarshalerCompatibility(typ types.Type) bool {
	// Check if type has UnmarshalSSZDynView method: func(view any) func(DynamicSpecs, []byte) error
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "UnmarshalSSZDynView", []string{"any"}, []string{"func"})
}

func (p *Parser) getDynamicViewEncoderCompatibility(typ types.Type) bool {
	// Check if type has MarshalSSZEncoderView method: func(view any) func(DynamicSpecs, Encoder) error
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "MarshalSSZEncoderView", []string{"any"}, []string{"func"})
}

func (p *Parser) getDynamicViewDecoderCompatibility(typ types.Type) bool {
	// Check if type has UnmarshalSSZDecoderView method: func(view any) func(DynamicSpecs, Decoder) error
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "UnmarshalSSZDecoderView", []string{"any"}, []string{"func"})
}

func (p *Parser) getDynamicViewSizerCompatibility(typ types.Type) bool {
	// Check if type has SizeSSZDynView method: func(view any) func(DynamicSpecs) int
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "SizeSSZDynView", []string{"any"}, []string{"func"})
}

func (p *Parser) getDynamicViewHashRootCompatibility(typ types.Type) bool {
	// Check if type has HashTreeRootWithDynView method: func(view any) func(DynamicSpecs, HashWalker) error
	methodSet := types.NewMethodSet(typ)
	return p.hasMethodWithSignature(methodSet, "HashTreeRootWithDynView", []string{"any"}, []string{"func"})
}

// Interface implementation checks using go/types proper interface checking

// Simple helper to check if a type has required methods
func (p *Parser) hasMethodWithSignature(methodSet *types.MethodSet, methodName string, paramTypes, returnTypes []string) bool {
	for i := 0; i < methodSet.Len(); i++ {
		method := methodSet.At(i)
		if method.Obj().Name() != methodName {
			continue
		}

		// Check method signature
		sig, ok := method.Type().(*types.Signature)
		if !ok {
			continue
		}

		// Check parameter count and types
		if sig.Params().Len() != len(paramTypes) {
			continue
		}

		// Check return value count and types
		if sig.Results().Len() != len(returnTypes) {
			continue
		}

		// Check parameter types
		for j := 0; j < sig.Params().Len() && j < len(paramTypes); j++ {
			paramType := sig.Params().At(j).Type()
			expectedType := paramTypes[j]
			if !p.typeMatches(paramType, expectedType) {
				goto nextMethod
			}
		}

		// Check return types
		for j := 0; j < sig.Results().Len() && j < len(returnTypes); j++ {
			returnType := sig.Results().At(j).Type()
			expectedType := returnTypes[j]
			if !p.typeMatches(returnType, expectedType) {
				goto nextMethod
			}
		}

		return true

	nextMethod:
	}
	return false
}

func (p *Parser) typeMatches(typ types.Type, expectedTypeStr string) bool {
	switch expectedTypeStr {
	case "-":
		return true
	case typeNameByteSlice:
		if slice, ok := typ.(*types.Slice); ok {
			if basic, ok := types.Unalias(slice.Elem()).(*types.Basic); ok {
				return basic.Kind() == types.Uint8
			}
		}
	case "[32]byte":
		if array, ok := typ.(*types.Array); ok && array.Len() == 32 {
			if basic, ok := types.Unalias(array.Elem()).(*types.Basic); ok {
				return basic.Kind() == types.Uint8
			}
		}
	case typeNameError:
		if named, ok := typ.(*types.Named); ok {
			return named.Obj().Name() == typeNameError && named.Obj().Pkg() == nil
		}
	case typeNameInt:
		if basic, ok := types.Unalias(typ).(*types.Basic); ok {
			return basic.Kind() == types.Int
		}
	case "DynamicSpecs", "HashWalker", "Encoder", "Decoder":
		return true
	case "any":
		// Check for interface{} or any type
		if iface, ok := typ.(*types.Interface); ok {
			return iface.Empty()
		}
	case "func":
		// Check if it's a function type (signature)
		_, ok := typ.(*types.Signature)
		return ok
	}
	return false
}
