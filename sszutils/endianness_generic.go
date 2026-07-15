// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

//go:build !(386 || amd64 || amd64p32 || arm || arm64 || loong64 || mipsle || mips64le || mips64p32le || ppc64le || riscv64 || wasm) && !(armbe || arm64be || m68k || mips || mips64 || mips64p32 || ppc || ppc64 || s390 || s390x || shbe || sparc || sparc64)

package sszutils

import "encoding/binary"

// hostLittleEndian reports whether the target architecture stores integers in
// little-endian byte order. Architectures whose endianness is not known at
// compile time (i.e. listed in neither endianness_le.go nor endianness_be.go —
// currently none of the toolchain-supported targets, so this covers future
// architectures) detect it at package init via binary.NativeEndian. Big-endian
// hosts take the per-element fallback in the bulk uint64 slice helpers,
// little-endian hosts use the bulk copy fast path, so an architecture missing
// from the compile-time lists stays correct and only loses branch elimination.
var hostLittleEndian = binary.NativeEndian.Uint16([]byte{0x34, 0x12}) == 0x1234
