// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package dynssz provides dynamic SSZ encoding and decoding with runtime reflection support.
package dynssz

type DynSszOption func(*DynSszOptions)

type DynSszOptions struct {
	NoFastSsz     bool
	NoFastHash    bool
	ExtendedTypes bool
	Verbose       bool
	LogCb         func(format string, args ...any)
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

// WithExtendedTypes creates an option to enable extended type support.
//
// When this option is enabled, dynssz will support nun-specified types like signed integers, floating point numbers, big integers and more.
// Generated SSZ code is incompatible with other SSZ libraries like fastssz.
func WithExtendedTypes() DynSszOption {
	return func(opts *DynSszOptions) {
		opts.ExtendedTypes = true
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
