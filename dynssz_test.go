// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
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

// skipUnless32Bit skips the test on platforms where int is wider than 32 bits.
func skipUnless32Bit(t *testing.T) {
	t.Helper()
	if math.MaxInt > math.MaxInt32 {
		t.Skip("overflow checks are only active on 32-bit platforms")
	}
}

func TestMarshalSSZLargeObjectOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testLargeContainer{}

	_, err := ds.MarshalSSZ(container)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected 'exceeds platform int max' error, got: %v", err)
	}
}

func TestMarshalSSZToLargeObjectOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testLargeContainer{}

	_, err := ds.MarshalSSZTo(container, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected 'exceeds platform int max' error, got: %v", err)
	}
}

func TestMarshalSSZWriterLargeObjectOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testLargeContainer{}

	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(container, &buf)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected 'exceeds platform int max' error, got: %v", err)
	}
}

func TestUnmarshalSSZLargeObjectOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testLargeContainer{}

	// The container's 2GB vector field exceeds what 32-bit int can address.
	// The exact error depends on which check triggers first (size vs data length).
	err := ds.UnmarshalSSZ(container, make([]byte, 100))
	if err == nil {
		t.Fatal("expected error for large object unmarshal on 32-bit")
	}
}

func TestUnmarshalSSZReaderLargeObjectOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testLargeContainer{}

	data := make([]byte, 100)
	err := ds.UnmarshalSSZReader(container, bytes.NewReader(data), len(data))
	if err == nil {
		t.Fatal("expected error for large object unmarshal on 32-bit")
	}
}

func TestHashTreeRootLargeObjectOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testLargeContainer{}

	_, err := ds.HashTreeRoot(container)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected 'exceeds platform int max' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mock types for DynamicMarshaler / DynamicSizer / DynamicUnmarshaler / DynamicHashRoot
// ---------------------------------------------------------------------------

// testDynMarshaler implements DynamicMarshaler + DynamicSizer.
type testDynMarshaler struct {
	Data  []byte
	Size  int
	Error error
}

func (t *testDynMarshaler) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	if t.Error != nil {
		return nil, t.Error
	}
	return append(buf, t.Data...), nil
}

func (t *testDynMarshaler) SizeSSZDyn(_ sszutils.DynamicSpecs) int {
	return t.Size
}

// testDynMarshalerNoSizer implements only DynamicMarshaler (no DynamicSizer).
type testDynMarshalerNoSizer struct {
	Data  []byte
	Error error
}

func (t *testDynMarshalerNoSizer) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	if t.Error != nil {
		return nil, t.Error
	}
	return append(buf, t.Data...), nil
}

// testDynUnmarshaler implements DynamicUnmarshaler.
type testDynUnmarshaler struct {
	Error error
}

func (t *testDynUnmarshaler) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, _ []byte) error {
	return t.Error
}

// testDynHashRoot implements DynamicHashRoot.
type testDynHashRoot struct {
	Error error
}

func (t *testDynHashRoot) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint8(0x42)
	return t.Error
}

// ---------------------------------------------------------------------------
// Mock types for DynamicView* interfaces
// ---------------------------------------------------------------------------

// testViewType is an empty struct used as the view descriptor.
type testViewType struct{}

// testDynViewAll implements all 6 DynamicView* interfaces.
type testDynViewAll struct {
	MarshalBuf []byte
	Size       int
	Error      error
}

func (t *testDynViewAll) MarshalSSZDynView(view any) func(sszutils.DynamicSpecs, []byte) ([]byte, error) {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
		if t.Error != nil {
			return nil, t.Error
		}
		return append(buf, t.MarshalBuf...), nil
	}
}

func (t *testDynViewAll) MarshalSSZEncoderView(view any) func(sszutils.DynamicSpecs, sszutils.Encoder) error {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, enc sszutils.Encoder) error {
		if t.Error != nil {
			return t.Error
		}
		enc.EncodeBytes(t.MarshalBuf)
		return nil
	}
}

func (t *testDynViewAll) SizeSSZDynView(view any) func(sszutils.DynamicSpecs) int {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs) int {
		return t.Size
	}
}

func (t *testDynViewAll) UnmarshalSSZDynView(view any) func(sszutils.DynamicSpecs, []byte) error {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, _ []byte) error {
		return t.Error
	}
}

func (t *testDynViewAll) UnmarshalSSZDecoderView(view any) func(sszutils.DynamicSpecs, sszutils.Decoder) error {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, dec sszutils.Decoder) error {
		if t.Error != nil {
			return t.Error
		}
		// consume all bytes
		buf := make([]byte, dec.GetLength())
		_, _ = dec.DecodeBytes(buf)
		return nil
	}
}

func (t *testDynViewAll) HashTreeRootWithDynView(view any) func(sszutils.DynamicSpecs, sszutils.HashWalker) error {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
		hh.PutUint8(0x42)
		return t.Error
	}
}

// testDynViewNoSizer implements DynamicViewMarshaler but NOT DynamicViewSizer.
type testDynViewNoSizer struct {
	MarshalBuf []byte
	Error      error
}

func (t *testDynViewNoSizer) MarshalSSZDynView(view any) func(sszutils.DynamicSpecs, []byte) ([]byte, error) {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
		if t.Error != nil {
			return nil, t.Error
		}
		return append(buf, t.MarshalBuf...), nil
	}
}

// testDynViewNilSizeFn implements DynamicViewMarshaler + DynamicViewSizer,
// but SizeSSZDynView returns nil.
type testDynViewNilSizeFn struct {
	MarshalBuf []byte
	Error      error
}

func (t *testDynViewNilSizeFn) MarshalSSZDynView(view any) func(sszutils.DynamicSpecs, []byte) ([]byte, error) {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
		if t.Error != nil {
			return nil, t.Error
		}
		return append(buf, t.MarshalBuf...), nil
	}
}

func (t *testDynViewNilSizeFn) SizeSSZDynView(_ any) func(sszutils.DynamicSpecs) int {
	return nil
}

// ---------------------------------------------------------------------------
// A. Dynamic interface fast paths (no view)
// ---------------------------------------------------------------------------

// MarshalSSZ: DynamicMarshaler with DynamicSizer (lines 216-224)
func TestMarshalSSZDynMarshalerWithSizer(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynMarshaler{Data: []byte{0xAA, 0xBB}, Size: 2}

	data, err := ds.MarshalSSZ(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0xAA, 0xBB}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// MarshalSSZ: DynamicMarshaler without DynamicSizer (lines 221-223)
func TestMarshalSSZDynMarshalerNoSizer(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynMarshalerNoSizer{Data: []byte{0xCC}}

	data, err := ds.MarshalSSZ(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0xCC}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// MarshalSSZTo: DynamicMarshaler (lines 312-314)
func TestMarshalSSZToDynMarshaler(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynMarshaler{Data: []byte{0xDD, 0xEE}, Size: 2}

	buf := []byte{0x01}
	data, err := ds.MarshalSSZTo(m, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0x01, 0xDD, 0xEE}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// SizeSSZ: DynamicSizer (lines 484-486)
func TestSizeSSZDynSizer(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynMarshaler{Size: 42}

	size, err := ds.SizeSSZ(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 42 {
		t.Fatalf("expected 42, got %d", size)
	}
}

// UnmarshalSSZ: DynamicUnmarshaler (lines 555-557)
func TestUnmarshalSSZDynUnmarshaler(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynUnmarshaler{}

	err := ds.UnmarshalSSZ(m, []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalSSZDynUnmarshalerError(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynUnmarshaler{Error: errors.New("unmarshal fail")}

	err := ds.UnmarshalSSZ(m, []byte{1, 2, 3})
	if err == nil || err.Error() != "unmarshal fail" {
		t.Fatalf("expected 'unmarshal fail', got: %v", err)
	}
}

// HashTreeRootWith: DynamicHashRoot success + error (lines 831-836)
func TestHashTreeRootWithDynHashRootSuccess(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynHashRoot{}

	_, err := ds.HashTreeRoot(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHashTreeRootWithDynHashRootError(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynHashRoot{Error: errors.New("hash fail")}

	_, err := ds.HashTreeRoot(m)
	if err == nil || err.Error() != "hash fail" {
		t.Fatalf("expected 'hash fail', got: %v", err)
	}
}

// UnmarshalSSZ: non-pointer and nil pointer checks (lines 575-581)
func TestUnmarshalSSZNotPointer(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := testSimpleContainer{}

	err := ds.UnmarshalSSZ(container, []byte{0x2a, 0, 0, 0})
	if err == nil || !strings.Contains(err.Error(), "target must be a pointer") {
		t.Fatalf("expected 'target must be a pointer', got: %v", err)
	}
}

func TestUnmarshalSSZNilPointer(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	var container *testSimpleContainer

	err := ds.UnmarshalSSZ(container, []byte{0x2a, 0, 0, 0})
	if err == nil || !strings.Contains(err.Error(), "target pointer must not be nil") {
		t.Fatalf("expected 'target pointer must not be nil', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// B. View descriptor paths (DynamicView* interfaces)
// ---------------------------------------------------------------------------

// resolveSchemaType: pointer view + value runtime (line 162-163)
func TestResolveSchemaTypePointerViewValueRuntime(t *testing.T) {
	ds := NewDynSsz(nil)
	runtimeType := reflect.TypeOf(testSimpleContainer{})
	cfg := &callConfig{viewDescriptor: &testViewType{}}

	schemaType := ds.resolveSchemaType(runtimeType, cfg)
	// viewDescriptor is *testViewType, runtimeType is value -> strip pointer
	if schemaType.Kind() == reflect.Ptr {
		t.Fatal("expected non-pointer schema type for value runtime type")
	}
	if schemaType.Name() != "testViewType" {
		t.Fatalf("expected testViewType, got %s", schemaType.Name())
	}
}

// resolveSchemaType: value view + pointer runtime (line 164-166)
func TestResolveSchemaTypeValueViewPointerRuntime(t *testing.T) {
	ds := NewDynSsz(nil)
	runtimeType := reflect.TypeOf(&testSimpleContainer{})
	cfg := &callConfig{viewDescriptor: testViewType{}}

	schemaType := ds.resolveSchemaType(runtimeType, cfg)
	// viewDescriptor is testViewType (value), runtimeType is pointer -> wrap in pointer
	if schemaType.Kind() != reflect.Ptr {
		t.Fatal("expected pointer schema type for pointer runtime type")
	}
	if schemaType.Elem().Name() != "testViewType" {
		t.Fatalf("expected *testViewType, got %s", schemaType)
	}
}

// MarshalSSZ: DynamicViewMarshaler with DynamicViewSizer (lines 226-240)
func TestMarshalSSZViewMarshalerWithSizer(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{MarshalBuf: []byte{0x11, 0x22}, Size: 2}

	data, err := ds.MarshalSSZ(m, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0x11, 0x22}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// MarshalSSZ: DynamicViewMarshaler without DynamicViewSizer (lines 236-238)
func TestMarshalSSZViewMarshalerNoSizer(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewNoSizer{MarshalBuf: []byte{0x33}}

	data, err := ds.MarshalSSZ(m, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0x33}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// MarshalSSZ: DynamicViewMarshaler + DynamicViewSizer but sizeFn returns nil (lines 234-235)
func TestMarshalSSZViewMarshalerNilSizeFn(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewNilSizeFn{MarshalBuf: []byte{0x44}}

	data, err := ds.MarshalSSZ(m, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0x44}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// MarshalSSZTo: DynamicViewMarshaler (lines 316-319)
func TestMarshalSSZToViewMarshaler(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{MarshalBuf: []byte{0x55, 0x66}, Size: 2}

	buf := []byte{0x01}
	data, err := ds.MarshalSSZTo(m, buf, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0x01, 0x55, 0x66}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// MarshalSSZWriter: DynamicViewEncoder (flush + write error) (lines 413-421)
func TestMarshalSSZWriterViewEncoderSuccess(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{MarshalBuf: []byte{0x77, 0x88}}

	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(m, &buf, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), []byte{0x77, 0x88}) {
		t.Fatalf("unexpected output: %x", buf.Bytes())
	}
}

func TestMarshalSSZWriterViewEncoderError(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{Error: errors.New("enc view fail")}

	var buf bytes.Buffer
	err := ds.MarshalSSZWriter(m, &buf, WithViewDescriptor(&testViewType{}))
	if err == nil || err.Error() != "enc view fail" {
		t.Fatalf("expected 'enc view fail', got: %v", err)
	}
}

func TestMarshalSSZWriterViewEncoderWriteError(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{MarshalBuf: []byte{0x77, 0x88}}

	w := &errorWriter{err: fmt.Errorf("view write failed")}
	err := ds.MarshalSSZWriter(m, w, WithViewDescriptor(&testViewType{}))
	if err == nil || !strings.Contains(err.Error(), "view write failed") {
		t.Fatalf("expected 'view write failed', got: %v", err)
	}
}

// SizeSSZ: DynamicViewSizer (lines 487-491)
func TestSizeSSZViewSizer(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{Size: 99}

	size, err := ds.SizeSSZ(m, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 99 {
		t.Fatalf("expected 99, got %d", size)
	}
}

// UnmarshalSSZ: DynamicViewUnmarshaler (lines 558-561)
func TestUnmarshalSSZViewUnmarshaler(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{}

	err := ds.UnmarshalSSZ(m, []byte{1, 2}, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalSSZViewUnmarshalerError(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{Error: errors.New("view unmarshal fail")}

	err := ds.UnmarshalSSZ(m, []byte{1, 2}, WithViewDescriptor(&testViewType{}))
	if err == nil || err.Error() != "view unmarshal fail" {
		t.Fatalf("expected 'view unmarshal fail', got: %v", err)
	}
}

// UnmarshalSSZReader: DynamicViewDecoder (success, error, unconsumed) (lines 683-695)
func TestUnmarshalSSZReaderViewDecoderSuccess(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{}

	data := []byte{1, 2, 3, 4}
	err := ds.UnmarshalSSZReader(m, bytes.NewReader(data), len(data),
		WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnmarshalSSZReaderViewDecoderError(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{Error: errors.New("view dec fail")}

	data := []byte{1, 2, 3, 4}
	err := ds.UnmarshalSSZReader(m, bytes.NewReader(data), len(data),
		WithViewDescriptor(&testViewType{}))
	if err == nil || err.Error() != "view dec fail" {
		t.Fatalf("expected 'view dec fail', got: %v", err)
	}
}

// testDynViewDecoderNoConsume implements only DynamicViewDecoder that doesn't consume bytes.
type testDynViewDecoderNoConsume struct{}

func (t *testDynViewDecoderNoConsume) UnmarshalSSZDecoderView(view any) func(sszutils.DynamicSpecs, sszutils.Decoder) error {
	if _, ok := view.(*testViewType); !ok {
		return nil
	}
	return func(_ sszutils.DynamicSpecs, _ sszutils.Decoder) error {
		return nil // doesn't consume any bytes
	}
}

func TestUnmarshalSSZReaderViewDecoderUnconsumed(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewDecoderNoConsume{}

	data := []byte{1, 2, 3, 4}
	err := ds.UnmarshalSSZReader(m, bytes.NewReader(data), len(data),
		WithViewDescriptor(&testViewType{}))
	if err == nil || !strings.Contains(err.Error(), "did not consume full ssz range") {
		t.Fatalf("expected unconsumed error, got: %v", err)
	}
}

// HashTreeRootWith: DynamicViewHashRoot (lines 838-844)
func TestHashTreeRootWithViewHashRootSuccess(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{}

	_, err := ds.HashTreeRoot(m, WithViewDescriptor(&testViewType{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHashTreeRootWithViewHashRootError(t *testing.T) {
	ds := NewDynSsz(nil)
	m := &testDynViewAll{Error: errors.New("view hash fail")}

	_, err := ds.HashTreeRoot(m, WithViewDescriptor(&testViewType{}))
	if err == nil || err.Error() != "view hash fail" {
		t.Fatalf("expected 'view hash fail', got: %v", err)
	}
}

// HashTreeRoot: pool selection (NoFastHash true/false paths) (lines 773-787)
func TestHashTreeRootNoFastHashFalse(t *testing.T) {
	ds := NewDynSsz(nil) // NoFastHash defaults to false => FastHasherPool
	m := &testDynHashRoot{}

	_, err := ds.HashTreeRoot(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHashTreeRootNoFastHashTrue(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastHash()) // => DefaultHasherPool
	m := &testDynHashRoot{}

	_, err := ds.HashTreeRoot(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// C. Other uncovered paths
// ---------------------------------------------------------------------------

// MarshalSSZ reflection path: GetTypeDescriptorWithSchema error (line 251)
func TestMarshalSSZGetTypeDescriptorError(t *testing.T) {
	ds := NewDynSsz(nil)

	_, err := ds.MarshalSSZ(make(chan int))
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

// MarshalSSZ reflection path: marshal error after successful SizeSSZ (line 269)
// SizeSSZ computes size from the descriptor's Len, but marshalVector rejects
// slices longer than Len.
type testOversizedVec struct {
	Data []uint32 `ssz-size:"2"`
}

func TestMarshalSSZMarshalErrorAfterSize(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testOversizedVec{Data: []uint32{1, 2, 3}} // 3 > ssz-size 2

	_, err := ds.MarshalSSZ(container)
	if err == nil {
		t.Fatal("expected error for oversized vector")
	}
}

// MarshalSSZTo reflection path (lines 322-341)
func TestMarshalSSZToReflectionSuccess(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{Value: 42}

	buf := make([]byte, 0, 64)
	data, err := ds.MarshalSSZTo(container, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0x2a, 0, 0, 0}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

func TestMarshalSSZToGetTypeDescriptorError(t *testing.T) {
	ds := NewDynSsz(nil)

	_, err := ds.MarshalSSZTo(make(chan int), nil)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestMarshalSSZToReflectionMarshalError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{Value: 42}

	td, err := ds.typeCache.GetTypeDescriptor(
		reflect.TypeOf(container), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	origType := td.SszType
	td.SszType = 255
	defer func() { td.SszType = origType }()

	_, err = ds.MarshalSSZTo(container, nil)
	if err == nil {
		t.Fatal("expected error for corrupted type descriptor")
	}
}

// GetTree (lines 918-925)
func TestGetTreeSuccess(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{Value: 42}

	node, err := ds.GetTree(container)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node == nil {
		t.Fatal("expected non-nil tree node")
	}
}

func TestGetTreeError(t *testing.T) {
	ds := NewDynSsz(nil)

	_, err := ds.GetTree(make(chan int))
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

// global.go: first-time initialization (lines 32-35)
func TestGetGlobalDynSszFirstInit(t *testing.T) {
	// Reset global state to force the first-time init path
	globalDynSsz.Store(nil)
	defer globalDynSsz.Store(nil)

	ds := GetGlobalDynSsz()
	if ds == nil {
		t.Fatal("expected non-nil DynSsz from first init")
	}
}

// options.go: applyCallOptions with actual options (lines 97-99)
func TestApplyCallOptionsWithOptions(t *testing.T) {
	view := &testViewType{}
	cfg := applyCallOptions([]CallOption{WithViewDescriptor(view)})

	if cfg.viewDescriptor != view {
		t.Fatal("expected view descriptor to be set")
	}
}

// options.go: WithViewDescriptor (lines 135-138)
func TestWithViewDescriptor(t *testing.T) {
	view := &testViewType{}
	opt := WithViewDescriptor(view)

	cfg := &callConfig{}
	opt(cfg)

	if cfg.viewDescriptor != view {
		t.Fatal("expected view descriptor to be set")
	}
}

// specvals.go: cache hit path (lines 28-30)
func TestResolveSpecValueCacheHit(t *testing.T) {
	specs := map[string]any{
		"A": float64(10),
	}
	ds := NewDynSsz(specs)

	// First call: populates cache
	resolved1, value1, err1 := ds.ResolveSpecValue("A")
	if err1 != nil {
		t.Fatalf("unexpected error on first call: %v", err1)
	}
	if !resolved1 || value1 != 10 {
		t.Fatalf("expected (true, 10), got (%v, %d)", resolved1, value1)
	}

	// Second call: should hit cache (lines 28-30)
	resolved2, value2, err2 := ds.ResolveSpecValue("A")
	if err2 != nil {
		t.Fatalf("unexpected error on cache hit: %v", err2)
	}
	if !resolved2 || value2 != 10 {
		t.Fatalf("expected (true, 10) from cache, got (%v, %d)", resolved2, value2)
	}
}

// MarshalSSZ reflection path: successful full flow (lines 255-278)
func TestMarshalSSZReflectionSuccess(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{Value: 42}

	data, err := ds.MarshalSSZ(container)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(data, []byte{0x2a, 0, 0, 0}) {
		t.Fatalf("unexpected output: %x", data)
	}
}

// SizeSSZ view fallthrough: GetTypeDescriptorWithSchema error
// when view descriptor is set but DynamicViewSizer returns nil,
// causing fallthrough to reflection path with incompatible schema type.
func TestSizeSSZViewFallthroughDescriptorError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{Value: 42}

	// Use an incompatible view descriptor that doesn't implement
	// DynamicViewSizer, causing fallthrough to reflection with a
	// schema type that can't be resolved.
	_, err := ds.SizeSSZ(container, WithViewDescriptor(make(chan int)))
	if err == nil {
		t.Fatal("expected error for incompatible view descriptor")
	}
}

// UnmarshalSSZ view fallthrough: GetTypeDescriptorWithSchema error
func TestUnmarshalSSZViewFallthroughDescriptorError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{}

	_, err := ds.MarshalSSZ(container)
	if err != nil {
		t.Fatalf("unexpected error marshalling: %v", err)
	}

	err = ds.UnmarshalSSZ(container, []byte{0x2a, 0, 0, 0},
		WithViewDescriptor(make(chan int)))
	if err == nil {
		t.Fatal("expected error for incompatible view descriptor")
	}
}

// SizeSSZ reflection path: ctx.SizeSSZ error (line 508)
func TestSizeSSZReflectionError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{}

	td, err := ds.typeCache.GetTypeDescriptor(
		reflect.TypeOf(container), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	origType := td.SszType
	td.SszType = 255
	defer func() { td.SszType = origType }()

	_, err = ds.SizeSSZ(container)
	if err == nil {
		t.Fatal("expected error for corrupted type descriptor")
	}
}

// SizeSSZ reflection path: successful path (line 516)
func TestSizeSSZReflectionSuccess(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{Value: 42}

	size, err := ds.SizeSSZ(container)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != 4 {
		t.Fatalf("expected 4, got %d", size)
	}
}

// UnmarshalSSZ reflection path: ctx.UnmarshalSSZ error (line 589)
func TestUnmarshalSSZReflectionError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{}

	// Provide too few bytes so that the reflection unmarshal fails
	err := ds.UnmarshalSSZ(container, []byte{0x01})
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

// UnmarshalSSZReader reflection path: ctx.UnmarshalSSZ error (line 721)
func TestUnmarshalSSZReaderReflectionError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{}

	// Provide too few bytes so that the reflection unmarshal fails
	data := []byte{0x01}
	err := ds.UnmarshalSSZReader(container, bytes.NewReader(data), len(data))
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

// HashTreeRootWith reflection path: ctx.HashTreeRoot error (line 862)
func TestHashTreeRootWithReflectionError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	container := &testSimpleContainer{}

	td, err := ds.typeCache.GetTypeDescriptor(
		reflect.TypeOf(container), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	origType := td.SszType
	td.SszType = 255
	defer func() { td.SszType = origType }()

	_, err = ds.HashTreeRoot(container)
	if err == nil {
		t.Fatal("expected error for corrupted type descriptor")
	}
}

// --- Bitlist marshal/HTR fixes (empty, nested, missing terminator) ---

func TestEmptyBitlistMarshalDoesNotPanic(t *testing.T) {
	type T struct {
		X []byte `ssz-type:"bitlist" ssz-max:"16"`
	}

	ds := NewDynSsz(nil)

	cases := map[string][]byte{
		"nil":         nil,
		"emptyNonNil": {},
	}

	want := []byte{0x04, 0x00, 0x00, 0x00, 0x01}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			enc, err := ds.MarshalSSZ(&T{X: in})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(enc, want) {
				t.Fatalf("unexpected encoding: got %x want %x", enc, want)
			}
		})
	}
}

func TestEmptyProgressiveBitlistMarshalDoesNotPanic(t *testing.T) {
	type T struct {
		X []byte `ssz-type:"progressive-bitlist" ssz-max:"16"`
	}

	ds := NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&T{X: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []byte{0x04, 0x00, 0x00, 0x00, 0x01}
	if !bytes.Equal(enc, want) {
		t.Fatalf("unexpected encoding: got %x want %x", enc, want)
	}
}

func TestNilBitlistNestedInContainer(t *testing.T) {
	type T struct {
		Pre uint32
		X   []byte `ssz-type:"bitlist" ssz-max:"16"`
		Pst uint64
	}

	ds := NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&T{Pre: 1, Pst: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var dst T
	if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
		t.Fatalf("roundtrip unmarshal failed: %v", err)
	}
	if dst.Pre != 1 || dst.Pst != 2 {
		t.Fatalf("roundtrip mismatch: %+v", dst)
	}
}

func TestNilBitlistInListOfStructs(t *testing.T) {
	type Inner struct {
		X []byte `ssz-type:"progressive-bitlist" ssz-max:"16"`
	}
	type Outer struct {
		Inner []*Inner `ssz-max:"4"`
	}

	ds := NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&Outer{Inner: []*Inner{{X: nil}, {X: nil}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var dst Outer
	if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
		t.Fatalf("roundtrip unmarshal failed: %v", err)
	}
	if len(dst.Inner) != 2 {
		t.Fatalf("expected 2 inner items, got %d", len(dst.Inner))
	}
}

func TestNilBitlistStreamMatchesBuffer(t *testing.T) {
	type T struct {
		X []byte `ssz-type:"bitlist" ssz-max:"16"`
	}

	ds := NewDynSsz(nil)
	bufEnc, err := ds.MarshalSSZ(&T{X: nil})
	if err != nil {
		t.Fatalf("buffer marshal failed: %v", err)
	}

	var sb bytes.Buffer
	if err := ds.MarshalSSZWriter(&T{X: nil}, &sb); err != nil {
		t.Fatalf("stream marshal failed: %v", err)
	}

	if !bytes.Equal(bufEnc, sb.Bytes()) {
		t.Fatalf("stream/buffer mismatch: buffer=%x stream=%x", bufEnc, sb.Bytes())
	}
}

func TestNilBitlistHTRAndMarshalAgree(t *testing.T) {
	type T struct {
		X []byte `ssz-type:"bitlist" ssz-max:"16"`
	}

	ds := NewDynSsz(nil)

	if _, err := ds.MarshalSSZ(&T{X: nil}); err != nil {
		t.Fatalf("marshal of nil bitlist failed: %v", err)
	}

	r1, err := ds.HashTreeRoot(&T{X: nil})
	if err != nil {
		t.Fatalf("HTR of nil bitlist failed: %v", err)
	}
	r2, err := ds.HashTreeRoot(&T{X: []byte{0x01}})
	if err != nil {
		t.Fatalf("HTR of empty bitlist failed: %v", err)
	}
	if r1 != r2 {
		t.Fatalf("nil and empty bitlist should hash equally: %x != %x", r1, r2)
	}
}

func TestBitlistMissingTerminatorHTRRejected(t *testing.T) {
	type T struct {
		X []byte `ssz-type:"bitlist" ssz-max:"32"`
	}

	ds := NewDynSsz(nil)
	src := &T{X: []byte{0xff, 0x00}} // last byte 0x00 => no terminator

	if _, err := ds.MarshalSSZ(src); err == nil {
		t.Fatalf("marshal should reject a non-terminated bitlist")
	}

	if _, err := ds.HashTreeRoot(src); err == nil {
		t.Fatalf("HTR should reject a non-terminated bitlist")
	} else if !errors.Is(err, sszutils.ErrInvalidValueRange) {
		t.Fatalf("expected ErrInvalidValueRange, got %v", err)
	}
}

// --- Byte-aligned bitvector followed by another field must round-trip ---

func TestBitvectorByteAlignedRoundtrip(t *testing.T) {
	ds := NewDynSsz(nil)

	t.Run("bitsize8", func(t *testing.T) {
		type T struct {
			BV    []byte `ssz-type:"bitvector" ssz-bitsize:"8"`
			After uint64
		}
		src := &T{BV: []byte{0xff}, After: 1}
		enc, err := ds.MarshalSSZ(src)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		var dst T
		if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if !bytes.Equal(dst.BV, src.BV) || dst.After != src.After {
			t.Fatalf("roundtrip mismatch: %+v", dst)
		}
	})

	t.Run("bitsize16", func(t *testing.T) {
		type T struct {
			BV    []byte `ssz-type:"bitvector" ssz-bitsize:"16"`
			After uint64
		}
		src := &T{BV: []byte{0xff, 0xff}, After: 1}
		enc, err := ds.MarshalSSZ(src)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		var dst T
		if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if !bytes.Equal(dst.BV, src.BV) || dst.After != src.After {
			t.Fatalf("roundtrip mismatch: %+v", dst)
		}
	})

	t.Run("bitsize32", func(t *testing.T) {
		type T struct {
			BV    []byte `ssz-type:"bitvector" ssz-bitsize:"32"`
			After uint64
		}
		src := &T{BV: []byte{0xff, 0xff, 0xff, 0xff}, After: 1}
		enc, err := ds.MarshalSSZ(src)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		var dst T
		if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		if !bytes.Equal(dst.BV, src.BV) || dst.After != src.After {
			t.Fatalf("roundtrip mismatch: %+v", dst)
		}
	})

	t.Run("bitsize12_paddingStillChecked", func(t *testing.T) {
		// 12 bits => 2 bytes; the top 4 bits of the 2nd byte are padding and must be 0.
		type T struct {
			BV    []byte `ssz-type:"bitvector" ssz-bitsize:"12"`
			After uint64
		}
		good := []byte{0xff, 0x0f, 0, 0, 0, 0, 0, 0, 0, 0} // padding bits zero
		var dst T
		if err := ds.UnmarshalSSZ(&dst, good); err != nil {
			t.Fatalf("valid 12-bit bitvector should decode: %v", err)
		}
		bad := []byte{0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0} // padding bits set
		if err := ds.UnmarshalSSZ(&T{}, bad); err == nil {
			t.Fatalf("expected padding error for non-zero padding bits")
		}
	})
}

// --- Recursive type definitions must error instead of stack overflowing ---

type recursiveType struct {
	Val      uint32
	Children []*recursiveType `ssz-max:"4"`
}

func TestRecursiveTypeRejected(t *testing.T) {
	ds := NewDynSsz(nil)

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("recursive type caused a panic: %v", r)
			}
		}()
		_, err = ds.MarshalSSZ(&recursiveType{})
	}()

	if err == nil {
		t.Fatalf("expected an error for recursive type, got nil")
	}
	if !errors.Is(err, sszutils.ErrUnsupportedType) {
		t.Fatalf("expected ErrUnsupportedType, got %v", err)
	}
}

// --- Stream writer with a too-small buffer must not panic ---

func TestStreamWriterBufferTooSmall(t *testing.T) {
	for _, sz := range []int{1, 3, 4, 7} {
		ds := NewDynSsz(nil, WithStreamWriterBufferSize(sz))
		var sb bytes.Buffer
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("buffer size %d caused a panic: %v", sz, r)
				}
			}()
			if err := ds.MarshalSSZWriter(&struct{ A uint64 }{A: 1}, &sb); err != nil {
				t.Fatalf("buffer size %d marshal failed: %v", sz, err)
			}
		}()

		ref, err := ds.MarshalSSZ(&struct{ A uint64 }{A: 1})
		if err != nil {
			t.Fatalf("reference marshal failed: %v", err)
		}
		if !bytes.Equal(sb.Bytes(), ref) {
			t.Fatalf("buffer size %d output mismatch: got %x want %x", sz, sb.Bytes(), ref)
		}
	}
}

// --- big.Int sign preservation (extended types) ---

func TestBigIntSignRoundtrip(t *testing.T) {
	type T struct{ N big.Int }
	ds := NewDynSsz(nil, WithExtendedTypes())

	for _, v := range []int64{-1000000, -100, -1, 0, 1, 100, 1 << 40} {
		src := &T{N: *big.NewInt(v)}
		enc, err := ds.MarshalSSZ(src)
		if err != nil {
			t.Fatalf("marshal %d: %v", v, err)
		}
		var dst T
		if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
			t.Fatalf("unmarshal %d: %v", v, err)
		}
		if dst.N.Cmp(big.NewInt(v)) != 0 {
			t.Fatalf("roundtrip %d -> %s", v, dst.N.String())
		}
	}
}

// TestBigIntSignEncodingGolden pins the sign-magnitude wire format so a change to
// both engines at once is still caught (codegen tests are only differential).
func TestBigIntSignEncodingGolden(t *testing.T) {
	type T struct{ N big.Int }
	ds := NewDynSsz(nil, WithExtendedTypes())

	cases := map[int64][]byte{
		0:    {0x04, 0, 0, 0, 0x00},       // offset, sign(+), no magnitude
		100:  {0x04, 0, 0, 0, 0x00, 0x64}, // offset, sign(+), 0x64
		-100: {0x04, 0, 0, 0, 0x01, 0x64}, // offset, sign(-), 0x64
	}
	for v, want := range cases {
		enc, err := ds.MarshalSSZ(&T{N: *big.NewInt(v)})
		if err != nil {
			t.Fatalf("marshal %d: %v", v, err)
		}
		if !bytes.Equal(enc, want) {
			t.Fatalf("marshal %d: got %x want %x", v, enc, want)
		}
	}
}

func TestBigIntSignHTRDistinct(t *testing.T) {
	type T struct{ N big.Int }
	ds := NewDynSsz(nil, WithExtendedTypes())

	rPos, err := ds.HashTreeRoot(&T{N: *big.NewInt(100)})
	if err != nil {
		t.Fatalf("htr pos: %v", err)
	}
	rNeg, err := ds.HashTreeRoot(&T{N: *big.NewInt(-100)})
	if err != nil {
		t.Fatalf("htr neg: %v", err)
	}
	if rPos == rNeg {
		t.Fatal("positive and negative big.Int must not hash equally")
	}
}

func TestBigIntInvalidSignByteRejected(t *testing.T) {
	type T struct{ N big.Int }
	ds := NewDynSsz(nil, WithExtendedTypes())

	// offset=4, sign byte 0x02 (invalid), magnitude 0x64
	bad := []byte{0x04, 0, 0, 0, 0x02, 0x64}
	var dst T
	err := ds.UnmarshalSSZ(&dst, bad)
	if err == nil {
		t.Fatal("expected error for invalid big.Int sign byte")
	}
	if !errors.Is(err, sszutils.ErrInvalidValueRange) {
		t.Fatalf("expected ErrInvalidValueRange, got %v", err)
	}
}

// --- Optional availability byte must be canonical 0x00/0x01 ---

func TestOptionalNonCanonicalAvailabilityRejected(t *testing.T) {
	type T struct {
		Pre uint32
		Opt *uint32 `ssz-type:"optional"`
	}
	ds := NewDynSsz(nil, WithExtendedTypes())

	// availability byte 0xff is non-canonical
	malformed := []byte{0, 0, 0, 0, 8, 0, 0, 0, 0xff, 0x99, 0, 0, 0}
	if err := ds.UnmarshalSSZ(&T{}, malformed); err == nil {
		t.Fatal("expected error for non-canonical availability byte")
	} else if !errors.Is(err, sszutils.ErrInvalidValueRange) {
		t.Fatalf("expected ErrInvalidValueRange, got %v", err)
	}

	// canonical present (0x01) and absent (0x00) must decode
	present := []byte{0, 0, 0, 0, 8, 0, 0, 0, 0x01, 0x99, 0, 0, 0}
	var dst T
	if err := ds.UnmarshalSSZ(&dst, present); err != nil {
		t.Fatalf("availability=1 should decode: %v", err)
	}
	if dst.Opt == nil || *dst.Opt != 0x99 {
		t.Fatalf("unexpected Opt: %v", dst.Opt)
	}
	absent := []byte{0, 0, 0, 0, 8, 0, 0, 0, 0x00}
	var dst2 T
	if err := ds.UnmarshalSSZ(&dst2, absent); err != nil {
		t.Fatalf("availability=0 should decode: %v", err)
	}
	if dst2.Opt != nil {
		t.Fatalf("expected nil Opt, got %v", *dst2.Opt)
	}
}

// --- Spec value resolution validation ---

func TestResolveSpecValueRejectsDegenerate(t *testing.T) {
	for _, v := range []any{-1.0, math.NaN(), math.Inf(1), math.Inf(-1), float64(1e30), int(-5), int64(-1)} {
		ds := NewDynSsz(map[string]any{"X": v})
		ok, _, err := ds.ResolveSpecValue("X")
		if err == nil && ok {
			t.Errorf("spec value %v should be rejected, but resolved", v)
		}
	}
}

func TestResolveSpecValueUint64Precision(t *testing.T) {
	ds := NewDynSsz(map[string]any{"X": uint64(math.MaxUint64)})
	ok, val, err := ds.ResolveSpecValue("X")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || val != math.MaxUint64 {
		t.Fatalf("expected full-precision MaxUint64, got ok=%v val=%d", ok, val)
	}
}

// --- #22: CalculateLimit overflow must not collide ---

func TestCalculateLimitOverflowNoCollision(t *testing.T) {
	type ListMax1 struct {
		V []uint64 `ssz-max:"1"`
	}
	type ListMaxOverflow struct {
		V []uint64 `ssz-max:"2305843009213693953"` // 2^61+1; *8 overflows uint64
	}
	ds := NewDynSsz(nil)
	a, err := ds.HashTreeRoot(&ListMax1{V: []uint64{42}})
	if err != nil {
		t.Fatalf("ListMax1: %v", err)
	}
	b, err := ds.HashTreeRoot(&ListMaxOverflow{V: []uint64{42}})
	if err != nil {
		t.Fatalf("ListMaxOverflow: %v", err)
	}
	if a == b {
		t.Fatal("distinct list capacities must not share a hash tree root")
	}
}

// --- #24: nil argument must error, not panic ---

func TestNilArgumentRejected(t *testing.T) {
	ds := NewDynSsz(nil)
	noPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s panicked on nil: %v", name, r)
			}
		}()
		fn()
	}
	noPanic("MarshalSSZ", func() { _, _ = ds.MarshalSSZ(nil) })
	noPanic("MarshalSSZTo", func() { _, _ = ds.MarshalSSZTo(nil, nil) })
	noPanic("MarshalSSZWriter", func() { _ = ds.MarshalSSZWriter(nil, &bytes.Buffer{}) })
	noPanic("SizeSSZ", func() { _, _ = ds.SizeSSZ(nil) })
	noPanic("UnmarshalSSZ", func() { _ = ds.UnmarshalSSZ(nil, make([]byte, 8)) })
	noPanic("UnmarshalSSZReader", func() { _ = ds.UnmarshalSSZReader(nil, bytes.NewReader(nil), 0) })
	noPanic("HashTreeRoot", func() { _, _ = ds.HashTreeRoot(nil) })

	if _, err := ds.MarshalSSZ(nil); err == nil {
		t.Error("MarshalSSZ(nil) should return an error")
	}
	if _, err := ds.HashTreeRoot(nil); err == nil {
		t.Error("HashTreeRoot(nil) should return an error")
	}
	if err := ds.UnmarshalSSZ(nil, make([]byte, 8)); err == nil {
		t.Error("UnmarshalSSZ(nil) should return an error")
	}
}

// --- #25: MarshalSSZTo must handle short-cap and non-empty (append) buffers ---

func TestMarshalSSZToBuffer(t *testing.T) {
	ds := NewDynSsz(nil)
	type T struct{ A uint64 }

	// capacity smaller than the output
	enc, err := ds.MarshalSSZTo(&T{A: 1}, make([]byte, 0, 4))
	if err != nil {
		t.Fatalf("short-cap buffer: %v", err)
	}
	if len(enc) != 8 {
		t.Fatalf("expected 8 bytes, got %d", len(enc))
	}

	// non-empty buffer -> append after existing content
	prefix := []byte{0xfe, 0xed}
	out, err := ds.MarshalSSZTo(&T{A: 1}, prefix)
	if err != nil {
		t.Fatalf("append buffer: %v", err)
	}
	if len(out) != 2+8 {
		t.Fatalf("expected 10 bytes, got %d", len(out))
	}
	if out[0] != 0xfe || out[1] != 0xed {
		t.Fatalf("prefix was clobbered: %x", out)
	}
}

// --- #27: HTR must enforce the element-count limit for primitive lists ---

func TestHTRListLimitEnforced(t *testing.T) {
	ds := NewDynSsz(nil)

	t.Run("bytes", func(t *testing.T) {
		type T struct {
			X []byte `ssz-max:"4"`
		}
		if _, err := ds.HashTreeRoot(&T{X: make([]byte, 5)}); err == nil {
			t.Error("[]byte over max-4 should fail HTR")
		}
		if _, err := ds.HashTreeRoot(&T{X: make([]byte, 4)}); err != nil {
			t.Errorf("[]byte at max-4 should pass HTR: %v", err)
		}
	})
	t.Run("uint16", func(t *testing.T) {
		type T struct {
			X []uint16 `ssz-max:"4"`
		}
		if _, err := ds.HashTreeRoot(&T{X: make([]uint16, 5)}); err == nil {
			t.Error("[]uint16 over max-4 should fail HTR")
		}
	})
	t.Run("uint32", func(t *testing.T) {
		type T struct {
			X []uint32 `ssz-max:"4"`
		}
		if _, err := ds.HashTreeRoot(&T{X: make([]uint32, 5)}); err == nil {
			t.Error("[]uint32 over max-4 should fail HTR")
		}
	})
}

// --- #28: ssz-max:"0" is a no-limit placeholder, not a zero limit ---

func TestZeroMaxTreatedAsNoLimit(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithNoFastHash())
	type T struct {
		X []uint64 `ssz-max:"0"`
	}
	if _, err := ds.MarshalSSZ(&T{X: []uint64{1, 2, 3}}); err != nil {
		t.Fatalf("ssz-max:0 marshal should succeed (no limit): %v", err)
	}
	if _, err := ds.HashTreeRoot(&T{X: []uint64{1, 2, 3}}); err != nil {
		t.Fatalf("ssz-max:0 HTR should succeed (no limit): %v", err)
	}
}

// --- #26: spec values above 2^53 keep full uint64 precision ---

func TestResolveSpecValuePrecisionAbove2p53(t *testing.T) {
	v := uint64(9007199254740993) // 2^53 + 1, not representable as float64
	ds := NewDynSsz(map[string]any{"X": v})
	ok, val, err := ds.ResolveSpecValue("X")
	if err != nil || !ok {
		t.Fatalf("expected resolved, got ok=%v err=%v", ok, err)
	}
	if val != v {
		t.Fatalf("precision loss: got %d want %d", val, v)
	}
}
