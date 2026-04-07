// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package dynssz provides dynamic SSZ encoding and decoding with runtime reflection support.
package dynssz

// DynSszOption is a functional option for configuring a DynSsz instance.
type DynSszOption func(*DynSszOptions)

// DynSszOptions holds the configuration options for a DynSsz instance.
type DynSszOptions struct {
	NoFastSsz                 bool
	NoFastHash                bool
	ExtendedTypes             bool
	Verbose                   bool
	LogCb                     func(format string, args ...any)
	StreamWriterBufferSize    int
	StreamWriterMaxBufferSize int
	StreamReaderBufferSize    int
	StreamReaderMaxBufferSize int
}

// WithNoFastSsz disables fastssz fallback for types that implement fastssz
// interfaces, forcing all operations through reflection-based encoding.
func WithNoFastSsz() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.NoFastSsz = true
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

// WithStreamWriterMaxBufferSize sets the maximum buffer size for delegating to
// buffer-based marshal methods during streaming SSZ encoding. When a type's
// serialized size exceeds this limit, the encoder falls through to
// reflection-based field-by-field marshalling instead of buffering the entire
// object. Defaults to 200KB if not set.
func WithStreamWriterMaxBufferSize(size int) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.StreamWriterMaxBufferSize = size
	}
}

// WithStreamReaderBufferSize sets the internal buffer size for the streaming
// SSZ decoder used by UnmarshalSSZReader. Defaults to 2KB if not set.
func WithStreamReaderBufferSize(size int) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.StreamReaderBufferSize = size
	}
}

// WithStreamReaderMaxBufferSize sets the maximum buffer size for delegating to
// buffer-based unmarshal methods during streaming SSZ decoding. When a type's
// serialized size exceeds this limit, the decoder falls through to
// reflection-based field-by-field unmarshalling instead of buffering the entire
// object. Defaults to 200KB if not set.
func WithStreamReaderMaxBufferSize(size int) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.StreamReaderMaxBufferSize = size
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
