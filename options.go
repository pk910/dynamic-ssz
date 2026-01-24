// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package dynssz provides dynamic SSZ encoding and decoding with runtime reflection support.
package dynssz

type DynSszOption func(*DynSszOptions)

type DynSszOptions struct {
	NoFastSsz  bool
	NoFastHash bool
	Verbose    bool
	LogCb      func(format string, args ...any)
}

func WithNoFastSsz() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.NoFastSsz = true
	}
}

func WithNoFastHash() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.NoFastHash = true
	}
}

func WithVerbose() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.Verbose = true
	}
}

func WithLogCb(logCb func(format string, args ...any)) DynSszOption {
	return func(opts *DynSszOptions) {
		opts.LogCb = logCb
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
func applyCallOptions(opts []CallOption) *callConfig {
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
