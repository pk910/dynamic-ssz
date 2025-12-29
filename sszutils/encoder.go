// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

type Encoder interface {
	CanSeek() bool // can use EncodeOffsetAt()
	GetPosition() int
	GetBuffer() []byte       // return the output buffer (or a temp buffer if CanSeek() is false)
	SetBuffer(buffer []byte) // set new output buffer (or write the buffer to the stream if CanSeek() is false)
	EncodeBool(v bool)
	EncodeUint8(v uint8)
	EncodeUint16(v uint16)
	EncodeUint32(v uint32)
	EncodeUint64(v uint64)
	EncodeBytes(v []byte)
	EncodeOffset(v uint32)
	EncodeOffsetAt(pos int, v uint32)
	EncodeZeroPadding(n int)
}
