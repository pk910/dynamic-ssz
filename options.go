// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package dynssz provides dynamic SSZ encoding and decoding with runtime reflection support.
package dynssz

// DynSszOption is a functional option for configuring a DynSsz instance.
type DynSszOption func(*DynSszOptions)

// DynSszOptions holds the configuration options for a DynSsz instance.
type DynSszOptions struct {
	NoFastSsz              bool
	NoDelegation           bool
	NoFastHash             bool
	ExtendedTypes          bool
	Verbose                bool
	LogCb                  func(format string, args ...any)
	StreamWriterBufferSize int
	StreamReaderBufferSize int
	MaxStreamSize          int
	MaxNestingDepth        int
}

// WithNoFastSsz disables fastssz fallback for types that implement fastssz
// interfaces, forcing all operations through reflection-based encoding.
func WithNoFastSsz() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.NoFastSsz = true
	}
}

// WithNoDelegation disables delegation to a type's own generated Dynamic* SSZ
// methods (MarshalSSZDyn, UnmarshalSSZDyn, HashTreeRootWith and friends,
// including their DynamicView* variants), forcing every operation — plain and
// view-descriptor — through the generic reflection engine.
//
// This differs from WithNoFastSsz, which only disables the legacy fastssz
// fallback: WithNoFastSsz leaves generated dynamic methods in charge, whereas
// WithNoDelegation bypasses them as well. Combine both to run entirely on
// reflection, which is primarily useful for differential testing a type's
// generated code against the reflection implementation.
//
// Custom types (ssz-type:"custom" and types with no reflection representation)
// always delegate to their own methods regardless of this option, since the
// reflection engine cannot serialize them.
func WithNoDelegation() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.NoDelegation = true
	}
}

// WithNoFastHash disables the accelerated hashtree hashing library, falling
// back to the native Go sha256 implementation.
func WithNoFastHash() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.NoFastHash = true
	}
}

// WithExtendedTypes creates an option to enable extended type support.
//
// When this option is enabled, dynssz will support nun-specified types like signed integers, floating point numbers, big integers and more.
// Generated SSZ code is incompatible with other SSZ libraries like fastssz.
//
// Note: this gate only applies to reflection-based processing. Generated code
// opts into extended types at generation time (dynssz-gen's with-extended-types
// flag / the codegen WithExtendedTypes option) and is not re-gated at runtime:
// a type whose generated methods use extended types will marshal/unmarshal them
// regardless of this option on the consuming DynSsz instance.
func WithExtendedTypes() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.ExtendedTypes = true
	}
}

// WithVerbose enables verbose debug logging during SSZ operations.
func WithVerbose() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.Verbose = true
	}
}

// WithLogCb sets a custom logging callback for debug output during SSZ
// operations.
func WithLogCb(logCb func(format string, args ...any)) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.LogCb = logCb
	}
}

// WithStreamWriterBufferSize sets the internal buffer size for the streaming
// SSZ encoder used by MarshalSSZWriter. Defaults to 2KB if not set.
func WithStreamWriterBufferSize(size int) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.StreamWriterBufferSize = size
	}
}

// WithStreamReaderBufferSize sets the maximum internal buffer size for the
// streaming SSZ decoder used by UnmarshalSSZReader. Defaults to 2KB if not set.
func WithStreamReaderBufferSize(size int) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.StreamReaderBufferSize = size
	}
}

// WithMaxStreamSize sets the upper bound on the total size of an SSZ payload
// decoded by UnmarshalSSZReader without a known length (size < 0). Defaults to
// sszutils.DefaultMaxStreamSize (512 MiB) if not set or set to a non-positive
// value.
//
// Unknown-length decoding is always byte-bounded, and deliberately so: the
// allowance prevents an endless input from driving unbounded wire buffering and
// doubles as the remaining-length estimate reported to decode paths that have
// not been taught about regions of unknown extent. It cannot be disabled.
//
// This is not a deadline, cancellation mechanism, or decoded-object heap limit.
// Network callers must impose their own lifetime bound, and should choose the
// smallest maximum their protocol and schema permit.
func WithMaxStreamSize(size int) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.MaxStreamSize = size
	}
}

// WithMaxNestingDepth bounds how deeply a value may nest while being encoded,
// decoded or hashed. Defaults to 1024 if not set or set to a non-positive
// value.
//
// The bound exists because stack exhaustion is fatal in Go: the runtime aborts
// the process and recover() cannot contain it, so a server could not isolate
// the failure to the request that caused it. Exceeding the bound returns
// sszutils.ErrMaxDepthExceeded instead.
//
// Only a recursive type -- one whose cycle closes through a variable-length
// field -- can nest to a depth the input controls, so the count advances only
// through the types that lie on such a cycle: one level per cycle member
// descended into, with the outermost value itself costing nothing. A trip
// around a cycle costs as many levels as the cycle has structural members (a
// type wrapper adds none). Every other type bottoms out at a depth fixed by
// its own structure and is unaffected, as is the cost of encoding it.
//
// Generated code counts by the same rule with its own bound, fixed at
// generation time (see the code generator's WithRecursionDepth option), so
// both engines accept and reject at identical nesting depths. The one caveat
// is a chain of distinct cycles spanning several packages: generated code
// restarts its count where such a chain crosses a package boundary (the
// depth-carrying methods are unexported), while the reflection walk counts it
// as one run. Each side stays bounded either way.
func WithMaxNestingDepth(depth int) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.MaxNestingDepth = depth
	}
}

// CallOption is a functional option for per-call configuration of MarshalSSZ,
// UnmarshalSSZ, and HashTreeRoot operations. These options allow runtime
// customization of SSZ encoding behavior without modifying the DynSsz instance.
type CallOption func(*callConfig)

// callConfig holds per-call configuration for SSZ operations.
// This struct is populated by CallOption functions and used during
// encoding, decoding, and hashing operations.
type callConfig struct {
	// viewDescriptor holds the view descriptor value provided via WithViewDescriptor.
	// When set, this defines the SSZ schema for the operation, allowing the same
	// runtime type to be serialized with different SSZ layouts (fork views).
	viewDescriptor any
}

// applyCallOptions applies all provided CallOptions to a callConfig and returns it.
// Returns nil when no options are provided to avoid heap allocation in the common case.
func applyCallOptions(opts []CallOption) *callConfig {
	if len(opts) == 0 {
		return nil
	}
	cfg := &callConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(cfg)
	}
	return cfg
}

// WithViewDescriptor specifies a view descriptor for fork-dependent SSZ schemas.
//
// The view descriptor defines the SSZ layout (field order, tags, sizes) while the
// actual data is read from/written to the runtime object. This enables a single
// runtime type to support multiple SSZ representations for different forks.
//
// The view parameter must be a struct or pointer to struct. Its fields are mapped
// to the runtime type's fields by name. The view's field types may differ from
// the runtime type's field types to support nested view descriptors.
//
// When no view descriptor is provided, the runtime type itself is used as the schema.
//
// Example usage:
//
//	// Define a view descriptor for Altair fork
//	type BodyAltairView struct {
//	    RandaoReveal   [96]byte
//	    SyncAggregate  SyncAggregateAltairView  // Nested view type
//	}
//
//	// Marshal with the Altair view
//	data, err := ds.MarshalSSZ(body, dynssz.WithViewDescriptor(&BodyAltairView{}))
//
//	// Unmarshal with the Altair view
//	err = ds.UnmarshalSSZ(&body, data, dynssz.WithViewDescriptor(&BodyAltairView{}))
//
//	// Compute hash tree root with the Altair view
//	root, err := ds.HashTreeRoot(body, dynssz.WithViewDescriptor(&BodyAltairView{}))
//
// Note: The view descriptor value itself is not used for data storage; only its
// type information is used to determine the SSZ schema. You can pass a nil pointer
// of the view type: WithViewDescriptor((*BodyAltairView)(nil))
func WithViewDescriptor(view any) CallOption {
	return func(cfg *callConfig) {
		cfg.viewDescriptor = view
	}
}
