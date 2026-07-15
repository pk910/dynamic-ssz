// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

//go:build !(386 || amd64 || amd64p32 || arm || arm64 || loong64 || mipsle || mips64le || mips64p32le || ppc64le || riscv64 || wasm)

package sszutils

import "encoding/binary"

// hostLittleEndian reports whether the target architecture stores integers in
// little-endian byte order. On architectures not known to be little-endian at
// compile time (see endianness_le.go) it is detected at package init via
// binary.NativeEndian, so any architecture the Go toolchain supports resolves
// correctly: big-endian targets (s390x, ppc64, mips, sparc64, ...) take the
// per-element fallback in the bulk uint64 slice helpers, while a little-endian
// architecture missing from the compile-time list still uses the bulk copy
// fast path.
var hostLittleEndian = binary.NativeEndian.Uint16([]byte{0x34, 0x12}) == 0x1234
