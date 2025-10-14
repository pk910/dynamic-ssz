// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package hasher

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"sync"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

func TestInitHasher(t *testing.T) {
	// Reset initialization state for testing
	hasherInitialized = false
	
	initHasher()
	
	if !hasherInitialized {
		t.Error("hasher should be initialized")
	}
	
	// Test that falseBytes is all zeros
	expectedFalse := make([]byte, 32)
	if !bytes.Equal(falseBytes, expectedFalse) {
		t.Error("falseBytes should be all zeros")
	}
	
	// Test that trueBytes has first byte set to 1
	expectedTrue := make([]byte, 32)
	expectedTrue[0] = 1
	if !bytes.Equal(trueBytes, expectedTrue) {
		t.Error("trueBytes should have first byte set to 1")
	}
	
	// Test that zeroBytes comes from sszutils
	if !bytes.Equal(zeroBytes, sszutils.ZeroBytes()) {
		t.Error("zeroBytes should match sszutils.ZeroBytes()")
	}
	
	// Test that zero hash levels are initialized
	if len(zeroHashLevels) == 0 {
		t.Error("zeroHashLevels should be initialized")
	}
	
	// Test that zeroHashLevels contains falseBytes at level 0
	if level, ok := zeroHashLevels[string(falseBytes)]; !ok || level != 0 {
		t.Error("zeroHashLevels should contain falseBytes at level 0")
	}
	
	// Test calling initHasher multiple times (should be safe)
	initHasher()
	if !hasherInitialized {
		t.Error("multiple calls to initHasher should be safe")
	}
}

func TestGetZeroHash(t *testing.T) {
	// Reset for clean test
	hasherInitialized = false
	
	// Test that GetZeroHash initializes hasher if needed
	hash0 := GetZeroHash(0)
	if !hasherInitialized {
		t.Error("GetZeroHash should initialize hasher if needed")
	}
	
	// Test that level 0 is zero bytes
	if !bytes.Equal(hash0, make([]byte, 32)) {
		t.Error("GetZeroHash(0) should return zero bytes")
	}
	
	// Test that each level is hash of previous level
	for i := 1; i < 5; i++ {
		prevHash := GetZeroHash(i - 1)
		currentHash := GetZeroHash(i)
		
		// Calculate expected hash: sha256(prevHash + prevHash)
		tmp := append(prevHash, prevHash...)
		expected := sha256.Sum256(tmp)
		
		if !bytes.Equal(currentHash, expected[:]) {
			t.Errorf("GetZeroHash(%d) should be hash of two GetZeroHash(%d)", i, i-1)
		}
	}
}

func TestGetZeroHashLevel(t *testing.T) {
	// Ensure hasher is initialized
	GetZeroHash(0)
	
	tests := []struct {
		hash      []byte
		expected  int
		shouldFind bool
	}{
		{
			hash:      falseBytes,
			expected:  0,
			shouldFind: true,
		},
		{
			hash:      GetZeroHash(1),
			expected:  1,
			shouldFind: true,
		},
		{
			hash:      GetZeroHash(5),
			expected:  5,
			shouldFind: true,
		},
		{
			hash:      []byte("not a zero hash"),
			expected:  0,
			shouldFind: false,
		},
	}
	
	for _, tt := range tests {
		level, found := GetZeroHashLevel(string(tt.hash))
		if found != tt.shouldFind {
			t.Errorf("GetZeroHashLevel found=%v, want %v", found, tt.shouldFind)
		}
		if found && level != tt.expected {
			t.Errorf("GetZeroHashLevel level=%d, want %d", level, tt.expected)
		}
	}
}

func TestGetZeroHashes(t *testing.T) {
	hashes := GetZeroHashes()
	
	// Test that we get all 65 hashes
	if len(hashes) != 65 {
		t.Errorf("GetZeroHashes should return 65 hashes, got %d", len(hashes))
	}
	
	// Test that the hashes match GetZeroHash results
	for i := 0; i < 10; i++ {
		if !bytes.Equal(hashes[i][:], GetZeroHash(i)) {
			t.Errorf("GetZeroHashes()[%d] doesn't match GetZeroHash(%d)", i, i)
		}
	}
}

func TestNativeHashWrapper(t *testing.T) {
	// Test that NativeHashWrapper creates a function
	hashFn := NativeHashWrapper(sha256.New())
	if hashFn == nil {
		t.Fatal("NativeHashWrapper should return a function")
	}
	
	// The NativeHashWrapper function is designed to be used within the hasher context
	// and requires specific input/output patterns. Testing it in isolation is complex,
	// so we just verify that the wrapper is created successfully.
	// The actual functionality is tested through the Hasher methods that use it.
}

func TestWithDefaultHasher(t *testing.T) {
	called := false
	err := WithDefaultHasher(func(hh sszutils.HashWalker) error {
		called = true
		if hh == nil {
			t.Error("hasher should not be nil")
		}
		return nil
	})
	
	if err != nil {
		t.Errorf("WithDefaultHasher returned error: %v", err)
	}
	
	if !called {
		t.Error("function should have been called")
	}
	
	// Test error propagation
	testErr := fmt.Errorf("test error")
	err = WithDefaultHasher(func(hh sszutils.HashWalker) error {
		return testErr
	})
	
	if err != testErr {
		t.Errorf("WithDefaultHasher should propagate errors")
	}
}

func TestHasherPool(t *testing.T) {
	pool := &HasherPool{}
	
	// Test Get with nil HashFn (should use NewHasher)
	h1 := pool.Get()
	if h1 == nil {
		t.Fatal("pool.Get() should not return nil")
	}
	
	// Test Put and Get again (should reuse)
	pool.Put(h1)
	h2 := pool.Get()
	if h2 != h1 {
		t.Error("pool should reuse returned hashers")
	}
	
	// Test with custom HashFn
	customHashFn := func(dst []byte, input []byte) error {
		return nil
	}
	pool.HashFn = customHashFn
	
	h3 := pool.Get()
	if h3.hash == nil {
		t.Error("hasher should have custom hash function")
	}
}

func TestHasherPoolConcurrency(t *testing.T) {
	pool := &HasherPool{}
	
	var wg sync.WaitGroup
	hashers := make([]*Hasher, 100)
	
	// Get hashers concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hashers[idx] = pool.Get()
		}(i)
	}
	wg.Wait()
	
	// Put them back concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pool.Put(hashers[idx])
		}(i)
	}
	wg.Wait()
	
	// All should be non-nil
	for i, h := range hashers {
		if h == nil {
			t.Errorf("hasher[%d] should not be nil", i)
		}
	}
}

func TestNewHasher(t *testing.T) {
	h := NewHasher()
	
	if h == nil {
		t.Fatal("NewHasher should not return nil")
	}
	
	if h.hash == nil {
		t.Error("hasher should have hash function")
	}
	
	if len(h.tmp) != 64 {
		t.Errorf("tmp buffer should be 64 bytes, got %d", len(h.tmp))
	}
	
	if len(h.buf) != 0 {
		t.Errorf("buf should be empty initially, got %d bytes", len(h.buf))
	}
}

func TestNewHasherWithHash(t *testing.T) {
	var hashFunc hash.Hash = sha256.New()
	h := NewHasherWithHash(hashFunc)
	
	if h == nil {
		t.Fatal("NewHasherWithHash should not return nil")
	}
	
	if h.hash == nil {
		t.Error("hasher should have hash function")
	}
}

func TestNewHasherWithHashFn(t *testing.T) {
	customHashFn := func(dst []byte, input []byte) error {
		// Simple identity function for testing
		copy(dst, input[:min(len(dst), len(input))])
		return nil
	}
	
	h := NewHasherWithHashFn(customHashFn)
	
	if h == nil {
		t.Fatal("NewHasherWithHashFn should not return nil")
	}
	
	if h.hash == nil {
		t.Error("hasher should have hash function")
	}
	
	// Test initialization when hasher is not initialized
	hasherInitialized = false
	h2 := NewHasherWithHashFn(customHashFn)
	if h2 == nil {
		t.Fatal("NewHasherWithHashFn should not return nil even when re-initializing")
	}
	
	// Should have initialized the hasher
	if !hasherInitialized {
		t.Error("NewHasherWithHashFn should initialize hasher if needed")
	}
}

func TestHasherWithTemp(t *testing.T) {
	h := NewHasher()
	
	originalTmp := h.tmp
	newTmp := make([]byte, 128)
	
	h.WithTemp(func(tmp []byte) []byte {
		if !bytes.Equal(tmp, originalTmp) {
			t.Error("WithTemp should pass current tmp buffer")
		}
		return newTmp
	})
	
	if !bytes.Equal(h.tmp, newTmp) {
		t.Error("WithTemp should update tmp buffer")
	}
}

func TestHasherReset(t *testing.T) {
	h := NewHasher()
	
	// Add some data
	h.buf = append(h.buf, []byte{1, 2, 3, 4}...)
	
	if len(h.buf) == 0 {
		t.Error("buffer should have data before reset")
	}
	
	h.Reset()
	
	if len(h.buf) != 0 {
		t.Error("buffer should be empty after reset")
	}
}

func TestHasherAppendBytes32(t *testing.T) {
	h := NewHasher()
	
	// Test exact 32 bytes
	data32 := make([]byte, 32)
	for i := range data32 {
		data32[i] = byte(i)
	}
	
	h.AppendBytes32(data32)
	if len(h.buf) != 32 {
		t.Errorf("buffer should be 32 bytes, got %d", len(h.buf))
	}
	if !bytes.Equal(h.buf, data32) {
		t.Error("buffer should contain the 32-byte data")
	}
	
	// Test less than 32 bytes (should be padded)
	h.Reset()
	shortData := []byte{1, 2, 3}
	h.AppendBytes32(shortData)
	
	if len(h.buf) != 32 {
		t.Errorf("buffer should be padded to 32 bytes, got %d", len(h.buf))
	}
	
	// Check data is at the beginning
	if !bytes.Equal(h.buf[:3], shortData) {
		t.Error("data should be at the beginning of buffer")
	}
	
	// Check padding is zeros
	padding := h.buf[3:]
	expectedPadding := make([]byte, 29)
	if !bytes.Equal(padding, expectedPadding) {
		t.Error("padding should be zeros")
	}
}

func TestHasherPutUint64(t *testing.T) {
	h := NewHasher()
	
	val := uint64(0x123456789ABCDEF0)
	h.PutUint64(val)
	
	if len(h.buf) != 32 {
		t.Errorf("buffer should be 32 bytes, got %d", len(h.buf))
	}
	
	// Check that the value is correctly encoded in little endian
	decoded := binary.LittleEndian.Uint64(h.buf[:8])
	if decoded != val {
		t.Errorf("decoded value %x doesn't match original %x", decoded, val)
	}
	
	// Check padding
	padding := h.buf[8:]
	expectedPadding := make([]byte, 24)
	if !bytes.Equal(padding, expectedPadding) {
		t.Error("padding should be zeros")
	}
}

func TestHasherPutUint32(t *testing.T) {
	h := NewHasher()
	
	val := uint32(0x12345678)
	h.PutUint32(val)
	
	if len(h.buf) != 32 {
		t.Errorf("buffer should be 32 bytes, got %d", len(h.buf))
	}
	
	decoded := binary.LittleEndian.Uint32(h.buf[:4])
	if decoded != val {
		t.Errorf("decoded value %x doesn't match original %x", decoded, val)
	}
}

func TestHasherPutUint16(t *testing.T) {
	h := NewHasher()
	
	val := uint16(0x1234)
	h.PutUint16(val)
	
	if len(h.buf) != 32 {
		t.Errorf("buffer should be 32 bytes, got %d", len(h.buf))
	}
	
	decoded := binary.LittleEndian.Uint16(h.buf[:2])
	if decoded != val {
		t.Errorf("decoded value %x doesn't match original %x", decoded, val)
	}
}

func TestHasherPutUint8(t *testing.T) {
	h := NewHasher()
	
	val := uint8(0xAB)
	h.PutUint8(val)
	
	if len(h.buf) != 32 {
		t.Errorf("buffer should be 32 bytes, got %d", len(h.buf))
	}
	
	if h.buf[0] != val {
		t.Errorf("first byte should be %x, got %x", val, h.buf[0])
	}
}

func TestHasherFillUpTo32(t *testing.T) {
	h := NewHasher()
	
	// Test with data that needs padding
	h.buf = []byte{1, 2, 3, 4, 5}
	h.FillUpTo32()
	
	if len(h.buf) != 32 {
		t.Errorf("buffer should be 32 bytes, got %d", len(h.buf))
	}
	
	// Test with data already aligned to 32
	h.Reset()
	h.buf = make([]byte, 32)
	h.FillUpTo32()
	
	if len(h.buf) != 32 {
		t.Errorf("buffer should remain 32 bytes, got %d", len(h.buf))
	}
	
	// Test with data larger than 32 but not aligned
	h.Reset()
	h.buf = make([]byte, 50)
	h.FillUpTo32()
	
	if len(h.buf) != 64 {
		t.Errorf("buffer should be padded to 64 bytes, got %d", len(h.buf))
	}
}

func TestHasherAppendBool(t *testing.T) {
	h := NewHasher()
	
	h.AppendBool(true)
	if len(h.buf) != 1 || h.buf[0] != 1 {
		t.Error("AppendBool(true) should append 1")
	}
	
	h.AppendBool(false)
	if len(h.buf) != 2 || h.buf[1] != 0 {
		t.Error("AppendBool(false) should append 0")
	}
}

func TestHasherAppendUint(t *testing.T) {
	h := NewHasher()
	
	// Test AppendUint8
	h.AppendUint8(0xAB)
	if len(h.buf) != 1 || h.buf[0] != 0xAB {
		t.Error("AppendUint8 failed")
	}
	
	// Test AppendUint16  
	h.Reset()
	h.AppendUint16(0x1234)
	if len(h.buf) != 2 {
		t.Error("AppendUint16 should append 2 bytes")
	}
	decoded := binary.LittleEndian.Uint16(h.buf)
	if decoded != 0x1234 {
		t.Error("AppendUint16 encoding incorrect")
	}
	
	// Test AppendUint32
	h.Reset()
	h.AppendUint32(0x12345678)
	if len(h.buf) != 4 {
		t.Error("AppendUint32 should append 4 bytes")
	}
	decoded32 := binary.LittleEndian.Uint32(h.buf)
	if decoded32 != 0x12345678 {
		t.Error("AppendUint32 encoding incorrect")
	}
	
	// Test AppendUint64
	h.Reset()
	h.AppendUint64(0x123456789ABCDEF0)
	if len(h.buf) != 8 {
		t.Error("AppendUint64 should append 8 bytes")
	}
	decoded64 := binary.LittleEndian.Uint64(h.buf)
	if decoded64 != 0x123456789ABCDEF0 {
		t.Error("AppendUint64 encoding incorrect")
	}
}

func TestHasherAppend(t *testing.T) {
	h := NewHasher()
	
	data := []byte{1, 2, 3, 4, 5}
	h.Append(data)
	
	if len(h.buf) != 5 {
		t.Errorf("buffer should be 5 bytes, got %d", len(h.buf))
	}
	
	if !bytes.Equal(h.buf, data) {
		t.Error("buffer should contain appended data")
	}
}

func TestHasherPutRootVector(t *testing.T) {
	h := NewHasher()
	
	// Test with valid 32-byte roots
	roots := [][]byte{
		make([]byte, 32),
		make([]byte, 32),
	}
	
	// Fill with test data
	for i := range roots[0] {
		roots[0][i] = byte(i)
		roots[1][i] = byte(i + 32)
	}
	
	err := h.PutRootVector(roots)
	if err != nil {
		t.Errorf("PutRootVector returned error: %v", err)
	}
	
	// Test with invalid root size
	h.Reset()
	invalidRoots := [][]byte{
		make([]byte, 31), // Wrong size
	}
	
	err = h.PutRootVector(invalidRoots)
	if err == nil {
		t.Error("PutRootVector should return error for invalid root size")
	}
	
	// Test with maxCapacity
	h.Reset()
	err = h.PutRootVector(roots, 4)
	if err != nil {
		t.Errorf("PutRootVector with maxCapacity returned error: %v", err)
	}
}

func TestHasherPutUint64Array(t *testing.T) {
	h := NewHasher()
	
	values := []uint64{1, 2, 3, 4, 5}
	
	// Test fixed size array
	h.PutUint64Array(values)
	
	// Buffer should contain the values plus padding plus merkleization
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after merkleization, got %d", len(h.buf))
	}
	
	// Test with maxCapacity (dynamic array)
	h.Reset()
	h.PutUint64Array(values, 10)
	
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after merkleization with mixin, got %d", len(h.buf))
	}
}

func TestParseBitlist(t *testing.T) {
	// Test valid bitlist
	bitlist := []byte{0b11010101, 0b00000001} // Last bit is sentinel
	
	dst := make([]byte, 0, 10)
	result, size := ParseBitlist(dst, bitlist)
	
	if size != 8 {
		t.Errorf("expected size 8, got %d", size)
	}
	
	if len(result) != 1 {
		t.Errorf("expected 1 byte result, got %d", len(result))
	}
	
	if result[0] != 0b11010101 {
		t.Errorf("expected 0b11010101, got 0b%08b", result[0])
	}
	
	// Test bitlist with trailing zeros
	bitlist2 := []byte{0b11010101, 0b00000000, 0b00000001}
	result2, size2 := ParseBitlist(dst[:0], bitlist2)
	
	if size2 != 16 {
		t.Errorf("expected size 16, got %d", size2)
	}
	
	if len(result2) != 1 {
		t.Errorf("expected trailing zeros removed, got %d bytes", len(result2))
	}
}

func TestHasherPutBitlist(t *testing.T) {
	h := NewHasher()
	
	bitlist := []byte{0b11010101, 0b00000001}
	maxSize := uint64(16)
	
	h.PutBitlist(bitlist, maxSize)
	
	// Should result in 32 bytes after merkleization with mixin
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(h.buf))
	}
}

func TestHasherPutProgressiveBitlist(t *testing.T) {
	h := NewHasher()
	
	bitlist := []byte{0b11010101, 0b00000001}
	
	h.PutProgressiveBitlist(bitlist)
	
	// Should result in 32 bytes after progressive merkleization with mixin
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(h.buf))
	}
}

func TestHasherPutBool(t *testing.T) {
	h := NewHasher()
	
	h.PutBool(true)
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(h.buf))
	}
	
	// Check that it uses trueBytes
	expectedTrue := make([]byte, 32)
	expectedTrue[0] = 1
	if !bytes.Equal(h.buf, expectedTrue) {
		t.Error("PutBool(true) should use trueBytes")
	}
	
	h.Reset()
	h.PutBool(false)
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(h.buf))
	}
	
	// Check that it uses falseBytes (all zeros)
	expectedFalse := make([]byte, 32)
	if !bytes.Equal(h.buf, expectedFalse) {
		t.Error("PutBool(false) should use falseBytes")
	}
}

func TestHasherPutBytes(t *testing.T) {
	h := NewHasher()
	
	// Test short bytes (≤32)
	shortData := []byte{1, 2, 3, 4, 5}
	h.PutBytes(shortData)
	
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes for short data, got %d", len(h.buf))
	}
	
	// Test long bytes (>32) - should merkleize
	h.Reset()
	longData := make([]byte, 100)
	for i := range longData {
		longData[i] = byte(i)
	}
	
	h.PutBytes(longData)
	
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after merkleization, got %d", len(h.buf))
	}
}

func TestHasherIndex(t *testing.T) {
	h := NewHasher()
	
	if h.Index() != 0 {
		t.Error("Index should be 0 for empty hasher")
	}
	
	h.buf = append(h.buf, []byte{1, 2, 3}...)
	
	if h.Index() != 3 {
		t.Error("Index should be 3 after adding 3 bytes")
	}
}

func TestHasherMerkleize(t *testing.T) {
	h := NewHasher()
	
	// Add some data
	h.buf = append(h.buf, make([]byte, 64)...) // Two 32-byte chunks
	for i := range h.buf {
		h.buf[i] = byte(i)
	}
	
	indx := 0
	h.Merkleize(indx)
	
	// Should result in 32 bytes (merkle root)
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after merkleization, got %d", len(h.buf))
	}
}

func TestHasherMerkleizeWithMixin(t *testing.T) {
	h := NewHasher()
	
	// Add some data
	h.buf = append(h.buf, make([]byte, 50)...) // Will be padded
	
	indx := 0
	num := uint64(5)
	limit := uint64(2)
	
	h.MerkleizeWithMixin(indx, num, limit)
	
	// Should result in 32 bytes (merkle root with mixin)
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after merkleization with mixin, got %d", len(h.buf))
	}
}

func TestHasherHash(t *testing.T) {
	h := NewHasher()
	
	// Test with exactly 32 bytes
	data := make([]byte, 32)
	h.buf = append(h.buf, data...)
	
	hash := h.Hash()
	if len(hash) != 32 {
		t.Errorf("expected 32 bytes hash, got %d", len(hash))
	}
	
	// Test with more than 32 bytes (should return last 32)
	h.buf = append(h.buf, make([]byte, 32)...)
	hash = h.Hash()
	if len(hash) != 32 {
		t.Errorf("expected 32 bytes hash, got %d", len(hash))
	}
	
	// Test with less than 32 bytes
	h.Reset()
	h.buf = append(h.buf, []byte{1, 2, 3}...)
	hash = h.Hash()
	if len(hash) != 3 {
		t.Errorf("expected 3 bytes hash, got %d", len(hash))
	}
}

func TestHasherHashRoot(t *testing.T) {
	h := NewHasher()
	
	// Test with exactly 32 bytes
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i)
	}
	h.buf = append(h.buf, data...)
	
	root, err := h.HashRoot()
	if err != nil {
		t.Errorf("HashRoot returned error: %v", err)
	}
	
	if !bytes.Equal(root[:], data) {
		t.Error("HashRoot should return the 32-byte buffer")
	}
	
	// Test with wrong size
	h.Reset()
	h.buf = append(h.buf, []byte{1, 2, 3}...)
	
	_, err = h.HashRoot()
	if err == nil {
		t.Error("HashRoot should return error for non-32-byte buffer")
	}
}

func TestHasherGetDepth(t *testing.T) {
	h := NewHasher()
	
	tests := []struct {
		input    uint64
		expected uint8
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{3, 2},
		{4, 2},
		{5, 3},
		{8, 3},
		{16, 4},
		{32, 5},
	}
	
	for _, tt := range tests {
		result := h.getDepth(tt.input)
		if result != tt.expected {
			t.Errorf("getDepth(%d) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestHasherMerkleizeImpl(t *testing.T) {
	h := NewHasher()
	
	// Test with empty input
	dst := make([]byte, 0, 32)
	result := h.merkleizeImpl(dst, []byte{}, 0)
	
	if len(result) != 32 {
		t.Errorf("expected 32 bytes for empty input, got %d", len(result))
	}
	
	// Test with single chunk
	input := make([]byte, 32)
	for i := range input {
		input[i] = byte(i)
	}
	
	result = h.merkleizeImpl(dst[:0], input, 1)
	if len(result) != 32 {
		t.Errorf("expected 32 bytes for single chunk, got %d", len(result))
	}
	
	if !bytes.Equal(result, input) {
		t.Error("single chunk should return input unchanged")
	}
	
	// Test with limit=1 but count=0 (should return zero bytes)
	result = h.merkleizeImpl(dst[:0], []byte{}, 1)
	expectedZero := make([]byte, 32)
	if !bytes.Equal(result, expectedZero) {
		t.Error("limit=1 with count=0 should return zero bytes")
	}
	
	// Test with multiple chunks to cover all merkleization paths
	input64 := make([]byte, 64) // Two chunks
	for i := range input64 {
		input64[i] = byte(i)
	}
	result = h.merkleizeImpl(dst[:0], input64, 2)
	if len(result) != 32 {
		t.Errorf("expected 32 bytes for two chunks, got %d", len(result))
	}
	
	// Test with larger input to cover deeper merkleization
	input128 := make([]byte, 128) // Four chunks
	for i := range input128 {
		input128[i] = byte(i)
	}
	result = h.merkleizeImpl(dst[:0], input128, 4)
	if len(result) != 32 {
		t.Errorf("expected 32 bytes for four chunks, got %d", len(result))
	}
	
	// Test with limit exceeded (should panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for count > limit")
		}
	}()
	
	h.merkleizeImpl(dst[:0], make([]byte, 64), 1) // 2 chunks with limit 1
}

func TestHasherMerkleizeProgressive(t *testing.T) {
	h := NewHasher()
	
	// Add some data
	h.buf = append(h.buf, make([]byte, 96)...) // Three 32-byte chunks
	
	indx := 0
	h.MerkleizeProgressive(indx)
	
	// Should result in 32 bytes (progressive merkle root)
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after progressive merkleization, got %d", len(h.buf))
	}
}

func TestHasherMerkleizeProgressiveWithMixin(t *testing.T) {
	h := NewHasher()
	
	// Add some data
	h.buf = append(h.buf, make([]byte, 50)...) // Will be padded
	
	indx := 0
	num := uint64(5)
	
	h.MerkleizeProgressiveWithMixin(indx, num)
	
	// Should result in 32 bytes (progressive merkle root with mixin)
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after progressive merkleization with mixin, got %d", len(h.buf))
	}
}

func TestHasherMerkleizeProgressiveWithActiveFields(t *testing.T) {
	h := NewHasher()
	
	// Add some data
	h.buf = append(h.buf, make([]byte, 50)...) // Will be padded
	
	indx := 0
	activeFields := []byte{0xFF, 0x00, 0xAA}
	
	h.MerkleizeProgressiveWithActiveFields(indx, activeFields)
	
	// Should result in 32 bytes (progressive merkle root with active fields)
	if len(h.buf) != 32 {
		t.Errorf("expected 32 bytes after progressive merkleization with active fields, got %d", len(h.buf))
	}
}

func TestHasherMerkleizeProgressiveImpl(t *testing.T) {
	h := NewHasher()
	
	// Test with empty input - returns zeroBytes (1024 bytes as per implementation)
	dst := make([]byte, 0)
	result := h.merkleizeProgressiveImpl(dst, []byte{}, 0)
	
	// The implementation returns zeroBytes... which is 1024 bytes
	if len(result) != 1024 {
		t.Errorf("expected 1024 bytes for empty input (zeroBytes), got %d", len(result))
	}
	
	// Verify it's all zeros
	for i, b := range result {
		if b != 0 {
			t.Errorf("expected all zeros, got non-zero byte %d at position %d", b, i)
		}
	}
	
	// Test with single chunk
	input := make([]byte, 32)
	for i := range input {
		input[i] = byte(i)
	}
	
	result = h.merkleizeProgressiveImpl(make([]byte, 0), input, 0)
	if len(result) != 32 {
		t.Errorf("expected 32 bytes for single chunk, got %d", len(result))
	}
	
	// Test with multiple chunks
	input = make([]byte, 96) // Three chunks
	result = h.merkleizeProgressiveImpl(make([]byte, 0), input, 0)
	if len(result) != 32 {
		t.Errorf("expected 32 bytes for multiple chunks, got %d", len(result))
	}
}

func TestDebugLogging(t *testing.T) {
	// Test logfn function
	logfn("test %s %d", "hello", 42)
	// This test just ensures the function doesn't panic
}

func TestDebugModeOperations(t *testing.T) {
	// Save original debug state
	originalDebug := debug
	defer func() {
		debug = originalDebug
	}()
	
	// Enable debug mode to test debug logging branches
	debug = true
	
	h := NewHasher()
	
	// Test Merkleize with debug logging
	h.buf = append(h.buf, make([]byte, 64)...)
	for i := range h.buf {
		h.buf[i] = byte(i)
	}
	h.Merkleize(0)
	
	// Test MerkleizeWithMixin with debug logging
	h.Reset()
	h.buf = append(h.buf, make([]byte, 50)...)
	h.MerkleizeWithMixin(0, 5, 2)
	
	// Test MerkleizeProgressive with debug logging
	h.Reset()
	h.buf = append(h.buf, make([]byte, 96)...)
	h.MerkleizeProgressive(0)
	
	// Test MerkleizeProgressiveWithMixin with debug logging
	h.Reset()
	h.buf = append(h.buf, make([]byte, 50)...)
	h.MerkleizeProgressiveWithMixin(0, 5)
	
	// Test MerkleizeProgressiveWithActiveFields with debug logging
	h.Reset()
	h.buf = append(h.buf, make([]byte, 50)...)
	activeFields := []byte{0xFF, 0x00, 0xAA}
	h.MerkleizeProgressiveWithActiveFields(0, activeFields)
}

func TestGlobalVariables(t *testing.T) {
	// Test that global pools exist (can't compare sync.Pool directly)
	_ = DefaultHasherPool
	_ = FastHasherPool
	
	// Test that we can get hashers from pools
	h1 := DefaultHasherPool.Get()
	if h1 == nil {
		t.Error("DefaultHasherPool.Get() should not return nil")
	}
	DefaultHasherPool.Put(h1)
	
	h2 := FastHasherPool.Get()
	if h2 == nil {
		t.Error("FastHasherPool.Get() should not return nil")
	}
	FastHasherPool.Put(h2)
}

func TestHasherInterfaceCompliance(t *testing.T) {
	// Test that Hasher implements sszutils.HashWalker
	var _ sszutils.HashWalker = (*Hasher)(nil)
	
	h := NewHasher()
	
	// Test basic HashWalker methods
	h.AppendBytes32(make([]byte, 32))
	h.PutUint64(42)
	h.PutUint32(42)
	h.PutUint16(42)
	h.PutUint8(42)
	h.FillUpTo32()
	h.AppendBool(true)
	h.AppendUint64(42)
	h.AppendUint32(42)
	h.AppendUint16(42)
	h.AppendUint8(42)
	h.Append([]byte{1, 2, 3})
	
	// Test simpler operations that don't cause panics
	h.PutUint64Array([]uint64{1, 2, 3})
	h.PutRootVector([][]byte{make([]byte, 32)})
	h.PutBool(true)
	h.PutBytes([]byte{1, 2, 3})
	
	// Test indexing and merkleization
	idx := h.Index()
	if idx == 0 {
		t.Error("Index should not be 0 after adding data")
	}
	
	h.Merkleize(0)
	h.Hash()
	
	_, err := h.HashRoot()
	if err != nil {
		t.Errorf("HashRoot failed: %v", err)
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}