// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

//go:build armbe || arm64be || m68k || mips || mips64 || mips64p32 || ppc || ppc64 || s390 || s390x || shbe || sparc || sparc64

package sszutils

// hostLittleEndian reports whether the target architecture stores integers in
// little-endian byte order. On the big-endian architectures listed above it is
// a compile-time constant, letting the compiler drop the unsafe bulk-copy fast
// path from the uint64 slice helpers entirely and keep only the per-element
// little-endian fallback. Architectures in neither this list nor the one in
// endianness_le.go detect the byte order at package init (see
// endianness_generic.go).
const hostLittleEndian = false
