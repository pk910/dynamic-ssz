// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package dynssz provides dynamic SSZ encoding and decoding with runtime reflection support.
package dynssz

// DynSszOption is a functional option for configuring a DynSsz instance.
type DynSszOption func(*DynSszOptions)

// DynSszOptions holds the configuration options for a DynSsz instance.
type DynSszOptions struct {
	NoFastSsz     bool
	NoFastHash    bool
	ExtendedTypes bool
	Verbose       bool
	LogCb         func(format string, args ...any)
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
