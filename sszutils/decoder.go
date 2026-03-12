// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

// Decoder is the interface for reading SSZ-encoded data. It supports both
// seekable (buffer-backed) and non-seekable (stream-backed) implementations.
type Decoder interface {
	Seekable() bool   // can use DecodeOffsetAt() and SkipBytes()
	GetPosition() int // return current position
	GetLength() int   // return remaining length
	PushLimit(limit int)
	PopLimit() int
	DecodeBool() (bool, error)
	DecodeUint8() (uint8, error)
	DecodeUint16() (uint16, error)
	DecodeUint32() (uint32, error)
	DecodeUint64() (uint64, error)
	DecodeBytes(buf []byte) ([]byte, error)
	DecodeBytesBuf(len int) ([]byte, error)
	DecodeOffset() (uint32, error)
	DecodeOffsetAt(pos int) uint32
	SkipBytes(n int)
}
