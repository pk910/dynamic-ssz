// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// Test types for DynamicEncoder/DynamicDecoder/DynamicMarshaler/DynamicUnmarshaler paths

type testDynamicEncoder struct {
	Data  []byte
	Error error
}

func (t *testDynamicEncoder) MarshalSSZEncoder(ds sszutils.DynamicSpecs, encoder sszutils.Encoder) error {
	if t.Error != nil {
		return t.Error
	}
	encoder.EncodeBytes(t.Data)
	return nil
}

type testDynamicDecoder struct {
	Size       int
	ConsumeAll bool
	Error      error
}

func (t *testDynamicDecoder) UnmarshalSSZDecoder(ds sszutils.DynamicSpecs, decoder sszutils.Decoder) error {
	if t.Error != nil {
		return t.Error
	}
	if t.ConsumeAll {
		buf := make([]byte, decoder.GetLength())
		_, _ = decoder.DecodeBytes(buf)
	}
	return nil
}

func TestDefaultLogUsesStructuredLogging(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	ds := NewDynSsz(nil)
	ds.options.LogCb("test message %d", 42)

	output := buf.String()
	if !strings.Contains(output, "test message 42") {
		t.Fatalf("expected slog debug output, got: %q", output)
	}
}

func TestWithOptions(t *testing.T) {
	ds := NewDynSsz(nil,
		WithNoFastSsz(),
		WithNoFastHash(),
		WithExtendedTypes(),
		WithVerbose(),
		WithLogCb(func(format string, args ...any) {}),
		WithStreamWriterBufferSize(4096),
		WithStreamReaderBufferSize(1024),
	)
	if !ds.options.NoFastSsz {
		t.Fatal("expected NoFastSsz")
	}
	if !ds.options.NoFastHash {
		t.Fatal("expected NoFastHash")
	}
	if !ds.options.ExtendedTypes {
		t.Fatal("expected ExtendedTypes")
	}
	if !ds.options.Verbose {
		t.Fatal("expected Verbose")
	}
	if ds.options.StreamWriterBufferSize != 4096 {
		t.Fatalf("expected StreamWriterBufferSize 4096, got %d", ds.options.StreamWriterBufferSize)
	}
	if ds.options.StreamReaderBufferSize != 1024 {
		t.Fatalf("expected StreamReaderBufferSize 1024, got %d", ds.options.StreamReaderBufferSize)
	}
}

// MarshalSSZWriter tests

func TestMarshalSSZWriterDynamicEncoderSuccess(t *testing.T) {
	ds := NewDynSsz(nil)
	enc := &testDynamicEncoder{Data: []byte{1, 2, 3}}

	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(enc, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), []byte{1, 2, 3}) {
		t.Fatalf("unexpected output: %x", buf.Bytes())
	}
}

func TestMarshalSSZWriterDynamicEncoderError(t *testing.T) {
	ds := NewDynSsz(nil)
	enc := &testDynamicEncoder{Error: errors.New("encode error")}

	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(enc, &buf)
	if err == nil || err.Error() != "encode error" {
		t.Fatalf("expected encode error, got: %v", err)
	}
}

func TestMarshalSSZWriterGetTypeDescriptorError(t *testing.T) {
	ds := NewDynSsz(nil)

	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(make(chan int), &buf)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

type testSimpleContainer struct {
	Value uint32 `ssz-size:"4"`
}

func TestMarshalSSZWriterMarshalError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := testSimpleContainer{Value: 42}

	// Populate the type cache for non-pointer type, then corrupt SszType
	td, err := ds.typeCache.GetTypeDescriptor(reflect.TypeOf(container), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	origType := td.SszType
	td.SszType = 255
	defer func() { td.SszType = origType }()

	var buf bytes.Buffer
	err = ds.MarshalSSZWriter(container, &buf)
	if err == nil {
		t.Fatal("expected error for corrupted type descriptor")
	}
}

// UnmarshalSSZReader tests

func TestUnmarshalSSZReaderDynamicDecoderError(t *testing.T) {
	ds := NewDynSsz(nil)
	dec := &testDynamicDecoder{Error: errors.New("decode error")}

	data := []byte{1, 2, 3, 4}
	err := ds.UnmarshalSSZReader(dec, bytes.NewReader(data), len(data))
	if err == nil || err.Error() != "decode error" {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestUnmarshalSSZReaderDynamicDecoderUnconsumed(t *testing.T) {
	ds := NewDynSsz(nil)
	dec := &testDynamicDecoder{ConsumeAll: false} // doesn't consume anything

	data := []byte{1, 2, 3, 4}
	err := ds.UnmarshalSSZReader(dec, bytes.NewReader(data), len(data))
	if err == nil || !strings.Contains(err.Error(), "did not consume full ssz range") {
		t.Fatalf("expected unconsumed error, got: %v", err)
	}
}

func TestUnmarshalSSZReaderDynamicDecoderSuccess(t *testing.T) {
	ds := NewDynSsz(nil)
	dec := &testDynamicDecoder{ConsumeAll: true}

	data := []byte{1, 2, 3, 4}
	err := ds.UnmarshalSSZReader(dec, bytes.NewReader(data), len(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalSSZReaderGetTypeDescriptorError(t *testing.T) {
	ds := NewDynSsz(nil)

	target := make(chan int)
	data := []byte{1, 2, 3, 4}
	err := ds.UnmarshalSSZReader(&target, bytes.NewReader(data), len(data))
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestUnmarshalSSZReaderNotPointer(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	container := testSimpleContainer{}
	data := []byte{0x2a, 0, 0, 0}
	err := ds.UnmarshalSSZReader(container, bytes.NewReader(data), len(data))
	if err == nil || !strings.Contains(err.Error(), "target must be a pointer") {
		t.Fatalf("expected 'target must be a pointer' error, got: %v", err)
	}
}

func TestUnmarshalSSZReaderNilPointer(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	var container *testSimpleContainer
	data := []byte{0x2a, 0, 0, 0}
	err := ds.UnmarshalSSZReader(container, bytes.NewReader(data), len(data))
	if err == nil || !strings.Contains(err.Error(), "target pointer must not be nil") {
		t.Fatalf("expected nil pointer error, got: %v", err)
	}
}

func TestUnmarshalSSZReaderReflectionUnconsumed(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	container := &testSimpleContainer{}
	// Provide extra bytes beyond the struct's 4-byte size
	data := []byte{0x2a, 0, 0, 0, 0xff, 0xff}
	err := ds.UnmarshalSSZReader(container, bytes.NewReader(data), len(data))
	if err == nil || !strings.Contains(err.Error(), "did not consume full ssz range") {
		t.Fatalf("expected unconsumed error, got: %v", err)
	}
}

func TestUnmarshalSSZReaderReflectionSuccess(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	container := &testSimpleContainer{}
	data := []byte{0x2a, 0, 0, 0}
	err := ds.UnmarshalSSZReader(container, bytes.NewReader(data), len(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if container.Value != 42 {
		t.Fatalf("expected 42, got %d", container.Value)
	}
}

// ValidateType tests

func TestValidateTypeSuccess(t *testing.T) {
	ds := NewDynSsz(nil)

	err := ds.ValidateType(reflect.TypeOf(testSimpleContainer{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTypeFailure(t *testing.T) {
	ds := NewDynSsz(nil)

	err := ds.ValidateType(reflect.TypeOf(make(chan int)))
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "type validation failed") {
		t.Fatalf("expected 'type validation failed' prefix, got: %v", err)
	}
}

// Verify the default LogCb option is set
func TestNewDynSszDefaultLogCbIsSet(t *testing.T) {
	ds := NewDynSsz(nil)
	if ds.options.LogCb == nil {
		t.Fatal("expected default LogCb to be set")
	}
	// Call it to ensure it doesn't panic
	ds.options.LogCb("test %s %d", "hello", 123)
}

// Verify nil specs defaults to empty map
func TestNewDynSszNilSpecs(t *testing.T) {
	ds := NewDynSsz(nil)
	if ds.specValues == nil {
		t.Fatal("expected non-nil specValues")
	}
}

func TestMarshalSSZWriterWriteError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{Value: 42}

	w := &errorWriter{err: fmt.Errorf("write failed")}
	err := ds.MarshalSSZWriter(container, w)
	if err == nil {
		t.Fatal("expected write error")
	}
}

type errorWriter struct {
	err error
}

func (w *errorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

// global.go tests

func TestGetGlobalDynSszDoubleCheck(t *testing.T) {
	// Reset global state
	globalDynSsz.Store(nil)
	defer globalDynSsz.Store(nil)

	// Hold the lock, start a goroutine that will block on it,
	// then store a value so the double-check at line 28 finds it.
	globalMu.Lock()

	ready := make(chan struct{})
	done := make(chan *DynSsz, 1)
	go func() {
		// Signal that we've started (line 20 check will see nil)
		close(ready)
		// This will block on globalMu.Lock() at line 24
		done <- GetGlobalDynSsz()
	}()

	// Wait for goroutine to start, then yield to let it reach the lock
	<-ready
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}

	// Store a value while the goroutine is blocked on the lock.
	// When it acquires the lock, the double-check at line 28 will find it.
	preSet := NewDynSsz(nil)
	globalDynSsz.Store(preSet)
	globalMu.Unlock()

	result := <-done
	if result != preSet {
		t.Fatal("expected the pre-stored instance from double-check path")
	}
}

// specvals.go tests

func TestResolveSpecValueInvalidExpression(t *testing.T) {
	ds := NewDynSsz(nil)

	_, _, err := ds.ResolveSpecValue("!!!invalid[")
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}
	if !strings.Contains(err.Error(), "error parsing dynamic spec expression") {
		t.Fatalf("expected parsing error, got: %v", err)
	}
}

func TestResolveSpecValueRoundsUp(t *testing.T) {
	// Use specs where the expression evaluates to a non-integer (e.g., 7/2 = 3.5)
	// to exercise the rounding-up branch
	specs := map[string]any{
		"A": float64(7),
		"B": float64(2),
	}
	ds := NewDynSsz(specs)

	resolved, value, err := ds.ResolveSpecValue("A / B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resolved {
		t.Fatal("expected resolved=true")
	}
	// 7/2 = 3.5, uint64(3.5) = 3, but should round up to 4
	if value != 4 {
		t.Fatalf("expected 4 (rounded up from 3.5), got %d", value)
	}
}

// SizeSSZ overflow test

type testLargeContainer struct {
	Data []byte `ssz-size:"2147483648"` // MaxInt32 + 1
}

func TestSizeSSZExceedsMaxInt32(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testLargeContainer{}

	_, err := ds.SizeSSZ(container)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum int32") {
		t.Fatalf("expected 'exceeds maximum int32' error, got: %v", err)
	}
}
