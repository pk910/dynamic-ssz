// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

//go:build 386 || amd64 || amd64p32 || arm || arm64 || loong64 || mipsle || mips64le || mips64p32le || ppc64le || riscv64 || wasm

package sszutils

// hostLittleEndian reports whether the target architecture stores integers in
// little-endian byte order. On the architectures listed above it is a
// compile-time constant, letting the compiler drop the per-element fallback
// branches from the bulk uint64 slice helpers entirely. Known big-endian
// architectures get a const false (see endianness_be.go); anything in neither
// list detects the byte order at package init (see endianness_generic.go), so
// these lists are a pure optimization: an architecture missing here stays
// correct and only loses the branch elimination.
const hostLittleEndian = true
