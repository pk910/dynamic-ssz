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
