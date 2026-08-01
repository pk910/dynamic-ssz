// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Test types for DynamicEncoder/DynamicDecoder/DynamicMarshaler/DynamicUnmarshaler paths

type testDynamicEncoder struct {
	Data  []byte `ssz-max:"64"`
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

func TestUnmarshalSSZReaderUnknownSize(t *testing.T) {
	type dynContainer struct {
		Value uint32
		List  []uint16 `ssz-max:"16"`
	}

	ds := NewDynSsz(nil, WithNoFastSsz())

	// A negative size reads the stream until EOF, so both fixed and dynamic
	// types must decode without a caller-supplied length.
	container := &testSimpleContainer{}
	err := ds.UnmarshalSSZReader(container, bytes.NewReader([]byte{0x2a, 0, 0, 0}), -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if container.Value != 42 {
		t.Fatalf("expected 42, got %d", container.Value)
	}

	src := &dynContainer{Value: 7, List: []uint16{1, 2, 3}}
	data, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decoded := &dynContainer{}
	err = ds.UnmarshalSSZReader(decoded, bytes.NewReader(data), -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded.Value != 7 || len(decoded.List) != 3 || decoded.List[2] != 3 {
		t.Fatalf("unexpected decode result: %+v", decoded)
	}

	// Trailing garbage still has to be rejected in unknown-size mode.
	err = ds.UnmarshalSSZReader(&testSimpleContainer{}, bytes.NewReader([]byte{0x2a, 0, 0, 0, 0xff}), -1)
	if err == nil {
		t.Fatal("expected error for trailing bytes in unknown-size mode")
	}

	// A read failure while draining the stream must surface as an error.
	err = ds.UnmarshalSSZReader(&testSimpleContainer{}, errorReader{errors.New("read boom")}, -1)
	if err == nil {
		t.Fatal("expected error when the reader fails in unknown-size mode")
	}
}

// errorReader always fails, exercising the io.ReadAll error path.
type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

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
	if !strings.Contains(err.Error(), "unsupported dynamic spec expression") {
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

// SizeSSZ platform behavior for >MaxInt32 sizes is covered by
// TestSizeAboveMaxInt32 (accepted on 64-bit, rejected on 32-bit).

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
	Data  []byte `ssz-max:"64"`
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
	Data  []byte `ssz-max:"64"`
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
	MarshalBuf []byte `ssz-max:"64"`
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
	MarshalBuf []byte `ssz-max:"64"`
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
	MarshalBuf []byte `ssz-max:"64"`
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

// foreignHasher stands in for a foreign concrete hasher (e.g. fastssz's
// *ssz.Hasher) that dynssz cannot supply to a HashTreeRootWith method.
type foreignHasher struct{}

// foreignHasherLeaf mimics standard fastssz sszgen output: it implements the type-safe
// HashTreeRoot() plus a HashTreeRootWith(hh *ConcreteForeignHasher) error whose
// parameter is a concrete foreign hasher type.
type foreignHasherLeaf struct {
	V uint64
}

func (l *foreignHasherLeaf) HashTreeRoot() ([32]byte, error) {
	var root [32]byte
	root[0] = 0xAB
	root[31] = 0xCD
	return root, nil
}

func (l *foreignHasherLeaf) HashTreeRootWith(_ *foreignHasher) error { return nil }

type foreignHasherContainer struct {
	Leaf foreignHasherLeaf
}

// HashTreeRoot must not panic for a nested type whose HashTreeRootWith takes a
// concrete foreign hasher; it falls back to the type-safe HashTreeRoot().
func TestHashTreeRootForeignHasherParamFallback(t *testing.T) {
	ds := NewDynSsz(nil)

	root, err := ds.HashTreeRoot(&foreignHasherContainer{Leaf: foreignHasherLeaf{V: 42}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The single-field container root equals the leaf's own HashTreeRoot(),
	// proving the type-safe fallback was used instead of the foreign method.
	var want [32]byte
	want[0] = 0xAB
	want[31] = 0xCD
	if root != want {
		t.Fatalf("root = %x; want %x (fastssz HashTreeRoot fallback)", root, want)
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

// --- Recursion: bounded (through a list) is legal; static is rejected ---

type recursiveType struct {
	Val      uint32
	Children []*recursiveType `ssz-max:"4"`
}

// Mutually recursive types where spec-dependence (dynssz-size) lives only on one
// side. The Has* flags must still propagate correctly around the cycle so both
// types size and hash correctly.
type mutRecA struct {
	Bs []*mutRecB `ssz-max:"4"`
}

type mutRecB struct {
	Tag []byte     `ssz-size:"2" dynssz-size:"TAGLEN"`
	As  []*mutRecA `ssz-max:"4"`
}

func TestMutualListRecursionRoundTrips(t *testing.T) {
	ds := NewDynSsz(map[string]any{"TAGLEN": uint64(3)}, WithNoFastSsz())

	src := &mutRecA{
		Bs: []*mutRecB{
			{Tag: []byte{1, 2, 3}},
			{Tag: []byte{4, 5, 6}, As: []*mutRecA{{Bs: []*mutRecB{{Tag: []byte{7, 8, 9}}}}}},
		},
	}

	buf, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var dst mutRecA
	if err = ds.UnmarshalSSZ(&dst, buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	buf2, err := ds.MarshalSSZ(&dst)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(buf, buf2) {
		t.Fatalf("round-trip mismatch:\n first=%x\n second=%x", buf, buf2)
	}
	if _, err := ds.HashTreeRoot(src); err != nil {
		t.Fatalf("hash tree root: %v", err)
	}
}

// A container recursive through a bounded list is a legal, finite SSZ type: the
// list is offset-encoded and terminates at runtime, so it must marshal,
// unmarshal and hash round-trip under reflection.
func TestListRecursiveTypeRoundTrips(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	src := &recursiveType{
		Val: 1,
		Children: []*recursiveType{
			{Val: 2},
			{Val: 3, Children: []*recursiveType{{Val: 4}}},
		},
	}

	buf, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var dst recursiveType
	if err = ds.UnmarshalSSZ(&dst, buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Byte-identical re-marshal proves the tree decoded correctly (avoids a
	// nil-vs-empty-slice DeepEqual mismatch on leaf nodes).
	buf2, err := ds.MarshalSSZ(&dst)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(buf, buf2) {
		t.Fatalf("round-trip mismatch:\n first=%x\n second=%x", buf, buf2)
	}

	if _, err := ds.HashTreeRoot(src); err != nil {
		t.Fatalf("hash tree root: %v", err)
	}
}

// Recursion that does not cross a variable-length collection has infinite static
// size and cannot be instantiated, so it must stay rejected.
func TestStaticRecursiveTypeRejected(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	t.Run("pointer", func(t *testing.T) {
		type ptrRecursive struct {
			V    uint64
			Next *ptrRecursive
		}
		_, err := ds.MarshalSSZ(&ptrRecursive{})
		if err == nil || !errors.Is(err, sszutils.ErrUnsupportedType) {
			t.Fatalf("expected ErrUnsupportedType, got %v", err)
		}
	})

	t.Run("vector", func(t *testing.T) {
		type vecRecursive struct {
			V   uint64
			Vec [2]*vecRecursive
		}
		_, err := ds.MarshalSSZ(&vecRecursive{})
		if err == nil || !errors.Is(err, sszutils.ErrUnsupportedType) {
			t.Fatalf("expected ErrUnsupportedType, got %v", err)
		}
	})
}

// A recursive type carrying a spec-dependent field must still round-trip when
// that field is declared AFTER the recursive list — the order where the
// in-progress element descriptor is most incomplete when the list build reads
// it. Correctness holds because the container uses a 4-byte offset for the
// dynamic list (never the element's static size) and drives element encoding
// from the fully back-patched element descriptor at runtime.
func TestSpecDependentListRecursionRoundTrips(t *testing.T) {
	type node struct {
		Children []*node `ssz-max:"4"`
		Tag      []byte  `ssz-size:"2" dynssz-size:"TAGLEN"` // spec-sized vector after the list
	}
	ds := NewDynSsz(map[string]any{"TAGLEN": uint64(3)}, WithNoFastSsz())

	src := &node{
		Tag: []byte{1, 2, 3},
		Children: []*node{
			{Tag: []byte{4, 5, 6}},
			{Tag: []byte{7, 8, 9}, Children: []*node{{Tag: []byte{10, 11, 12}}}},
		},
	}

	buf, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var dst node
	if err = ds.UnmarshalSSZ(&dst, buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	buf2, err := ds.MarshalSSZ(&dst)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(buf, buf2) {
		t.Fatalf("round-trip mismatch:\n first=%x\n second=%x", buf, buf2)
	}
	if _, err := ds.HashTreeRoot(src); err != nil {
		t.Fatalf("hash tree root: %v", err)
	}
}

// Cycle types where the loop closes through a container field (F3 *cycleA)
// rather than a list element. The container reading the in-progress cycle head
// must lay F3 out as a dynamic (offset) field, and the spec-dependent max on
// B.F2 must propagate to every cycle member including C, which completes its
// build before the head does.
type cycleA struct {
	F1 []cycleB `ssz-max:"4"`
}

type cycleB struct {
	F2 []cycleC `ssz-max:"4" dynssz-max:"CYCLE_MAX"`
}

type cycleC struct {
	F3 *cycleA
}

func TestContainerClosedRecursionRoundTrips(t *testing.T) {
	ds := NewDynSsz(map[string]any{"CYCLE_MAX": uint64(8)}, WithNoFastSsz())

	src := &cycleA{
		F1: []cycleB{
			{F2: []cycleC{
				{F3: nil},
				{F3: &cycleA{F1: []cycleB{{F2: []cycleC{{F3: nil}}}}}},
			}},
		},
	}

	buf, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst cycleA
	if err = ds.UnmarshalSSZ(&dst, buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	buf2, err := ds.MarshalSSZ(&dst)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(buf, buf2) {
		t.Fatalf("round-trip mismatch:\n first=%x\n second=%x", buf, buf2)
	}
	if _, err = ds.HashTreeRoot(src); err != nil {
		t.Fatalf("hash tree root: %v", err)
	}

	// The C descriptor completed before the cycle head; its layout and flags
	// must nonetheless reflect the finished graph.
	descC, err := ds.typeCache.GetTypeDescriptor(reflect.TypeOf(cycleC{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	if descC.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicMax == 0 {
		t.Error("cycleC descriptor is missing the dynamic-max flag from its subtree")
	}
	if descC.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
		t.Error("cycleC descriptor should be dynamic (contains a dynamic pointer field)")
	}
	if descC.ContainerDesc == nil || len(descC.ContainerDesc.DynFields) != 1 {
		t.Errorf("cycleC should have exactly 1 dynamic field, got %+v", descC.ContainerDesc)
	}
}

// The descriptor graph must come out identical no matter which type of the
// cycle is built first: entering through the value type makes the list key the
// cycle head, entering through the pointer makes the container the head.
func TestRecursionEntryOrderIndependence(t *testing.T) {
	src := &recursiveType{
		Val: 1,
		Children: []*recursiveType{
			{Val: 2},
			{Val: 3, Children: []*recursiveType{{Val: 4}}},
		},
	}

	// Pointer-entry reference bytes.
	dsPtr := NewDynSsz(nil, WithNoFastSsz())
	want, err := dsPtr.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("pointer-entry marshal: %v", err)
	}

	// Value-entry: prime the cache through the value type first.
	dsVal := NewDynSsz(nil, WithNoFastSsz())
	if _, err = dsVal.typeCache.GetTypeDescriptor(reflect.TypeOf(recursiveType{}), nil, nil, nil); err != nil {
		t.Fatalf("value-entry descriptor: %v", err)
	}
	got, err := dsVal.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("value-entry marshal: %v", err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("entry-order dependent encoding:\n pointer=%x\n value  =%x", want, got)
	}

	var dst recursiveType
	if err = dsVal.UnmarshalSSZ(&dst, got); err != nil {
		t.Fatalf("value-entry unmarshal: %v", err)
	}
	rootPtr, err := dsPtr.HashTreeRoot(src)
	if err != nil {
		t.Fatalf("pointer-entry root: %v", err)
	}
	rootVal, err := dsVal.HashTreeRoot(src)
	if err != nil {
		t.Fatalf("value-entry root: %v", err)
	}
	if rootPtr != rootVal {
		t.Fatalf("entry-order dependent root: %x != %x", rootPtr, rootVal)
	}
}

// Cycle types where a member implements the fastssz hasher interface. Its
// subtree carries a spec-dependent max, so delegation to the (preset-baking)
// fastssz method must be suppressed once the graph is complete.
type fhCycleA struct {
	F1 []fhCycleB `ssz-max:"4"`
}

type fhCycleB struct {
	F2 []fhCycleC `ssz-max:"4" dynssz-max:"FH_MAX"`
}

type fhCycleC struct {
	F3 *fhCycleA
}

// HashTreeRoot returns a sentinel root; it must never be used for this type
// because its subtree depends on a runtime spec value.
func (c *fhCycleC) HashTreeRoot() ([32]byte, error) {
	return [32]byte{0xde, 0xad, 0xbe, 0xef}, nil
}

func TestRecursionSuppressesFastsszDelegation(t *testing.T) {
	specs := map[string]any{"FH_MAX": uint64(8)}
	src := &fhCycleA{F1: []fhCycleB{{F2: []fhCycleC{{F3: nil}}}}}

	dsDefault := NewDynSsz(specs)
	rootDefault, err := dsDefault.HashTreeRoot(src)
	if err != nil {
		t.Fatalf("default root: %v", err)
	}

	dsRefl := NewDynSsz(specs, WithNoFastSsz())
	rootRefl, err := dsRefl.HashTreeRoot(src)
	if err != nil {
		t.Fatalf("reflection root: %v", err)
	}

	if rootDefault != rootRefl {
		t.Fatalf("fastssz delegation not suppressed: default=%x reflection=%x", rootDefault, rootRefl)
	}

	descC, err := dsDefault.typeCache.GetTypeDescriptor(reflect.TypeOf(fhCycleC{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	if descC.SszCompatFlags&ssztypes.SszCompatFlagFastSSZHasher != 0 {
		t.Error("fastssz hasher flag should be suppressed for a spec-dependent subtree")
	}
}

// A spec-dependent size (rather than max) inside the cycle must reach the
// early-completing member as well, gating the fastssz marshal path.
func TestRecursionPropagatesSpecSizeFlags(t *testing.T) {
	ds := NewDynSsz(map[string]any{"DS_TAG": uint64(3)}, WithNoFastSsz())

	src := &specSizeCycleA{
		F1: []specSizeCycleB{
			{Tag: []byte{1, 2, 3}, F2: []specSizeCycleC{{F3: nil}, {F3: &specSizeCycleA{}}}},
		},
	}
	buf, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst specSizeCycleA
	if err = ds.UnmarshalSSZ(&dst, buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	descC, err := ds.typeCache.GetTypeDescriptor(reflect.TypeOf(specSizeCycleC{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	if descC.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicSize == 0 {
		t.Error("cycle member is missing the dynamic-size flag from its subtree")
	}
}

type specSizeCycleA struct {
	F1 []specSizeCycleB `ssz-max:"4"`
}

type specSizeCycleB struct {
	Tag []byte           `ssz-size:"2" dynssz-size:"DS_TAG"`
	F2  []specSizeCycleC `ssz-max:"4"`
}

type specSizeCycleC struct {
	F3 *specSizeCycleA
}

// Recursion through an optional field is presence-gated and therefore legal.
func TestOptionalRecursionRoundTrips(t *testing.T) {
	type optNode struct {
		V    uint64
		Next *optNode `ssz-type:"optional"`
	}
	ds := NewDynSsz(nil, WithExtendedTypes(), WithNoFastSsz())

	src := &optNode{V: 1, Next: &optNode{V: 2, Next: &optNode{V: 3}}}
	buf, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst optNode
	if err = ds.UnmarshalSSZ(&dst, buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	buf2, err := ds.MarshalSSZ(&dst)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(buf, buf2) {
		t.Fatalf("round-trip mismatch:\n first=%x\n second=%x", buf, buf2)
	}
}

// recUnionNode is a recursive type carrying a union member, so the recursion
// flag fixup and the cyclic type-hash serialization both traverse union
// variants.
type recUnionNode struct {
	U CompatibleUnion[struct {
		V1 uint32
		V2 uint64
	}]
	Children []*recUnionNode `ssz-max:"4"`
}

func TestRecursiveTypeWithUnion(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	src := &recUnionNode{
		U: CompatibleUnion[struct {
			V1 uint32
			V2 uint64
		}]{Variant: 2, Data: uint64(7)},
		Children: []*recUnionNode{
			{U: CompatibleUnion[struct {
				V1 uint32
				V2 uint64
			}]{Variant: 1, Data: uint32(3)}},
		},
	}

	buf, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst recUnionNode
	if err = ds.UnmarshalSSZ(&dst, buf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	buf2, err := ds.MarshalSSZ(&dst)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(buf, buf2) {
		t.Fatalf("round-trip mismatch:\n first=%x\n second=%x", buf, buf2)
	}

	// The cyclic descriptor graph containing a union must hash deterministically
	// and not collapse to the empty-input hash.
	desc, err := ds.typeCache.GetTypeDescriptor(reflect.TypeOf(recUnionNode{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	hash := desc.GetTypeHash()
	if hash == ([32]byte{}) || hash != desc.GetTypeHash() {
		t.Fatal("cyclic union descriptor hash should be non-trivial and stable")
	}
	empty := sha256.Sum256(nil)
	if hash == empty {
		t.Fatal("cyclic union descriptor hashed to the empty-input hash")
	}
}

// hintedShareList carries a size annotation, so references to it resolve with
// external hints derived from the annotation.
type hintedShareList []uint16

var _ = sszutils.Annotate[hintedShareList](`ssz-max:"8"`)

type hintedShareParentA struct {
	L hintedShareList
	T []uint64 `ssz-max:"16"`
	U []uint64 `ssz-max:"32"`
}

type hintedShareParentB struct {
	L hintedShareList
	T []uint64 `ssz-max:"16"`
	U []uint64 `ssz-max:"64"`
}

// Hint-carrying references are cached per exact hint combination: the same
// (type, hints) pair resolves to one shared descriptor across all reference
// sites, while different hints keep distinct descriptors.
func TestHintedDescriptorSharing(t *testing.T) {
	ds := NewDynSsz(nil)

	da, err := ds.typeCache.GetTypeDescriptor(reflect.TypeOf(hintedShareParentA{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor A: %v", err)
	}
	db, err := ds.typeCache.GetTypeDescriptor(reflect.TypeOf(hintedShareParentB{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor B: %v", err)
	}

	if da.ContainerDesc.Fields[0].Type != db.ContainerDesc.Fields[0].Type {
		t.Error("annotation-derived field descriptors with identical hints should be shared")
	}
	if da.ContainerDesc.Fields[1].Type != db.ContainerDesc.Fields[1].Type {
		t.Error("field-tagged descriptors with identical hints should be shared")
	}
	if da.ContainerDesc.Fields[2].Type == db.ContainerDesc.Fields[2].Type {
		t.Error("descriptors with different hints must stay distinct")
	}
	if la, lb := da.ContainerDesc.Fields[2].Type.Limit, db.ContainerDesc.Fields[2].Type.Limit; la != 32 || lb != 64 {
		t.Errorf("distinct-hint descriptors carry wrong limits: %d/%d", la, lb)
	}
}

// Cycle types where a sibling after the recursive branch is not SSZ-encodable.
// Members that finished before the failure were built against an abandoned
// graph, so the failed build must not leave them cached.
type failCycleA struct {
	F1  []failCycleC `ssz-max:"4"`
	Bad map[string]int
}

type failCycleC struct {
	F3 *failCycleA
}

func TestFailedRecursiveBuildNotCached(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	if _, err := ds.MarshalSSZ(&failCycleA{}); err == nil {
		t.Fatal("expected error for map field")
	}

	// The member must not be served from a poisoned cache: building it fresh
	// reaches the map field again and must fail.
	if _, err := ds.typeCache.GetTypeDescriptor(reflect.TypeOf(failCycleC{}), nil, nil, nil); err == nil {
		t.Fatal("expected error when building the cycle member standalone")
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

// --- CalculateLimit overflow must not collide ---

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

// --- nil argument must error, not panic ---

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

// --- MarshalSSZTo must handle short-cap and non-empty (append) buffers ---

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

// --- HTR must enforce the element-count limit for primitive lists ---

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

// --- ssz-max:"0" is a no-limit placeholder, not a zero limit ---

// ssz-max:"0" is a "no limit" placeholder rather than a limit of zero: the real
// limit is expected from a dynssz-max expression. A list that ends up with no
// limit at all has no SSZ hash tree root -- List[T, N] needs N to merkleize --
// so it is only usable with extended types.
func TestZeroMaxTreatedAsNoLimit(t *testing.T) {
	type T struct {
		X []uint64 `ssz-max:"0"`
	}
	payload := &T{X: []uint64{1, 2, 3}}

	// A limit only bounds a list, so serialization never needs one; only the
	// root does.
	plain := NewDynSsz(nil, WithNoFastSsz(), WithNoFastHash())
	if _, err := plain.MarshalSSZ(payload); err != nil {
		t.Fatalf("ssz-max:0 marshal should succeed without extended types: %v", err)
	}
	if _, err := plain.HashTreeRoot(payload); !errors.Is(err, sszutils.ErrExtendedTypeDisabled) {
		t.Fatalf("err = %v, want the limit-less list to require extended types to hash", err)
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithNoFastHash(), WithExtendedTypes())
	if _, err := ds.MarshalSSZ(payload); err != nil {
		t.Fatalf("ssz-max:0 marshal should succeed (no limit): %v", err)
	}
	if _, err := ds.HashTreeRoot(payload); err != nil {
		t.Fatalf("ssz-max:0 HTR should succeed (no limit): %v", err)
	}

	// A dynssz-max expression is still a limit, just not one resolvable
	// statically, so the placeholder pattern needs no extension.
	type withExpr struct {
		X []uint64 `ssz-max:"0" dynssz-max:"LIMIT"`
	}
	specs := NewDynSsz(map[string]any{"LIMIT": uint64(8)}, WithNoFastSsz(), WithNoFastHash())
	if _, err := specs.HashTreeRoot(&withExpr{X: []uint64{1, 2, 3}}); err != nil {
		t.Fatalf("a dynssz-max limit should not require extended types: %v", err)
	}
}

// --- spec values above 2^53 keep full uint64 precision ---

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

// --- coverage: spec value type handling (specvals.go) ---

func TestResolveSpecValueTypes(t *testing.T) {
	cases := []struct {
		name    string
		val     any
		want    uint64
		ok      bool
		wantErr bool
	}{
		{name: "uint", val: uint(7), want: 7, ok: true},
		{name: "uint32", val: uint32(8), want: 8, ok: true},
		{name: "uint16", val: uint16(9), want: 9, ok: true},
		{name: "uint8", val: uint8(10), want: 10, ok: true},
		{name: "uintptr", val: uintptr(11), want: 11, ok: true},
		{name: "int32", val: int32(12), want: 12, ok: true},
		{name: "int16", val: int16(13), want: 13, ok: true},
		{name: "int8", val: int8(14), want: 14, ok: true},
		{name: "float32", val: float32(15), want: 15, ok: true},
		{name: "numericString", val: "64", want: 64, ok: true},
		// A referenced spec key carrying a non-numeric type must error instead of
		// silently falling back to the static limit.
		{name: "badString", val: "nope", wantErr: true},
		{name: "bool", val: true, wantErr: true},
		{name: "map", val: map[string]int{"a": 1}, wantErr: true},
		{name: "slice", val: []int{1, 2, 3}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := NewDynSsz(map[string]any{"X": tc.val})
			ok, val, err := ds.ResolveSpecValue("X")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %T value", tc.val)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tc.ok {
				t.Fatalf("resolved=%v, want %v", ok, tc.ok)
			}
			if tc.ok && val != tc.want {
				t.Fatalf("value=%d, want %d", val, tc.want)
			}
		})
	}
}

func TestResolveSpecValueExpressionDegenerate(t *testing.T) {
	// An expression (not a direct lookup) that evaluates to a negative value must
	// be rejected by specFloatToUint64.
	ds := NewDynSsz(map[string]any{"A": float64(1), "B": float64(2)})
	if _, _, err := ds.ResolveSpecValue("A - B"); err == nil {
		t.Fatal("expected error for negative expression result")
	}
}

// --- coverage: nil guards for HashTreeRootWith and GetTree (dynssz.go) ---

func TestNilArgumentRejectedHashWithAndTree(t *testing.T) {
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
	noPanic("HashTreeRootWith", func() { _ = ds.HashTreeRootWith(nil, nil) })
	noPanic("GetTree", func() { _, _ = ds.GetTree(nil) })

	if err := ds.HashTreeRootWith(nil, nil); err == nil {
		t.Error("HashTreeRootWith(nil) should error")
	}
	if _, err := ds.GetTree(nil); err == nil {
		t.Error("GetTree(nil) should error")
	}
}

// --- coverage: marshal/HTR error propagation paths (reflection) ---

// non-terminated bitlist: a leaf whose marshal and HTR both fail.
type covBadBitlist struct {
	X []byte `ssz-type:"bitlist" ssz-max:"16"`
}

func covBadBitlistValue() covBadBitlist { return covBadBitlist{X: []byte{0xff, 0x00}} }

func TestMarshalErrorPropagation(t *testing.T) {
	ds := NewDynSsz(nil, WithExtendedTypes())

	t.Run("typeWrapper", func(t *testing.T) {
		type desc struct {
			Data []byte `ssz-type:"bitlist" ssz-max:"16"`
		}
		w := &TypeWrapper[desc, []byte]{Data: []byte{0xff, 0x00}}
		if _, err := ds.MarshalSSZ(w); err == nil {
			t.Fatal("expected error from wrapped bitlist")
		}
	})

	t.Run("optional", func(t *testing.T) {
		type T struct {
			O *covBadBitlist `ssz-type:"optional"`
		}
		v := covBadBitlistValue()
		if _, err := ds.MarshalSSZ(&T{O: &v}); err == nil {
			t.Fatal("expected error from optional inner bitlist")
		}
	})

	t.Run("dynamicVector", func(t *testing.T) {
		type T struct {
			V [1]covBadBitlist
		}
		if _, err := ds.MarshalSSZ(&T{V: [1]covBadBitlist{covBadBitlistValue()}}); err == nil {
			t.Fatal("expected error from vector element bitlist")
		}
	})

	t.Run("dynamicList", func(t *testing.T) {
		type T struct {
			L []covBadBitlist `ssz-max:"4"`
		}
		if _, err := ds.MarshalSSZ(&T{L: []covBadBitlist{covBadBitlistValue()}}); err == nil {
			t.Fatal("expected error from list element bitlist")
		}
	})
}

// union marshal error paths require reaching marshalCompatibleUnion without a
// prior size pass, which only happens for a top-level union via MarshalSSZWriter.
func TestUnionMarshalErrorPaths(t *testing.T) {
	ds := NewDynSsz(nil)

	t.Run("invalidVariant", func(t *testing.T) {
		u := &CompatibleUnion[struct {
			V0 uint32
			V1 [16]byte
		}]{Variant: 99, Data: uint32(1)}
		var sb bytes.Buffer
		if err := ds.MarshalSSZWriter(u, &sb); err == nil {
			t.Fatal("expected error for invalid variant")
		}
	})

	t.Run("typeMismatch", func(t *testing.T) {
		u := &CompatibleUnion[struct {
			V0 uint32
			V1 [16]byte
		}]{Variant: 1, Data: "wrong type"}
		var sb bytes.Buffer
		if err := ds.MarshalSSZWriter(u, &sb); err == nil {
			t.Fatal("expected error for wrong-typed data")
		}
	})

	t.Run("variantMarshalError", func(t *testing.T) {
		u := &CompatibleUnion[struct {
			V0 []byte `ssz-type:"bitlist" ssz-max:"16"`
		}]{Variant: 1, Data: []byte{0xff, 0x00}}
		var sb bytes.Buffer
		if err := ds.MarshalSSZWriter(u, &sb); err == nil {
			t.Fatal("expected error from union variant bitlist")
		}
	})
}

func TestUnionHTRVariantError(t *testing.T) {
	ds := NewDynSsz(nil)
	u := &CompatibleUnion[struct {
		V0 []byte `ssz-type:"bitlist" ssz-max:"16"`
	}]{Variant: 1, Data: []byte{0xff, 0x00}}
	if _, err := ds.HashTreeRoot(u); err == nil {
		t.Fatal("expected HTR error from union variant bitlist")
	}
}

// MarshalSSZWriter must reject a nil writer with a clean error instead of
// panicking inside the stream encoder.
func TestMarshalSSZWriterNilWriter(t *testing.T) {
	type T struct{ A uint64 }
	ds := NewDynSsz(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MarshalSSZWriter panicked on nil writer: %v", r)
		}
	}()

	if err := ds.MarshalSSZWriter(&T{A: 1}, nil); err == nil {
		t.Fatal("expected error for nil writer, got nil")
	}
}

// UnmarshalSSZReader must reject a nil reader with a clean error.
func TestUnmarshalSSZReaderNilReader(t *testing.T) {
	type T struct{ A uint64 }
	ds := NewDynSsz(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("UnmarshalSSZReader panicked on nil reader: %v", r)
		}
	}()

	if err := ds.UnmarshalSSZReader(&T{}, nil, 8); err == nil {
		t.Fatal("expected error for nil reader, got nil")
	}
}

// HashTreeRootWith must reject a nil hash walker with a clean error.
func TestHashTreeRootWithNilWalker(t *testing.T) {
	type T struct{ A uint64 }
	ds := NewDynSsz(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("HashTreeRootWith panicked on nil walker: %v", r)
		}
	}()

	if err := ds.HashTreeRootWith(&T{A: 1}, nil); err == nil {
		t.Fatal("expected error for nil walker, got nil")
	}
}

// NewDynSsz must skip nil options in the variadic list instead of panicking.
func TestNewDynSszNilOption(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewDynSsz panicked on nil option: %v", r)
		}
	}()

	var nilOpt DynSszOption
	ds := NewDynSsz(nil, nilOpt, WithNoFastSsz())
	if ds == nil {
		t.Fatal("expected non-nil DynSsz instance")
	}
	if !ds.options.NoFastSsz {
		t.Error("non-nil option after nil option was not applied")
	}
}

// MarshalSSZ must skip nil CallOptions instead of panicking when applying them.
func TestApplyCallOptionsNilOption(t *testing.T) {
	type T struct{ A uint64 }
	ds := NewDynSsz(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MarshalSSZ panicked on nil call option: %v", r)
		}
	}()

	var nilCall CallOption
	got, err := ds.MarshalSSZ(&T{A: 1}, nilCall)
	if err != nil {
		t.Fatalf("MarshalSSZ with nil call option: %v", err)
	}

	want, err := ds.MarshalSSZ(&T{A: 1})
	if err != nil {
		t.Fatalf("MarshalSSZ reference: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("nil call option changed output: got %x want %x", got, want)
	}
}

// A bitlist whose ssz-max is near math.MaxUint64 must not collide with a small
// ssz-max bitlist, and HashTreeRoot must agree with the GetTree path.
func TestBitlistMaxSizeOverflowNoCollision(t *testing.T) {
	type Small struct {
		X []byte `ssz-type:"bitlist" ssz-max:"256"`
	}
	type Huge struct {
		X []byte `ssz-type:"bitlist" ssz-max:"18446744073709551615"`
	}

	ds := NewDynSsz(nil)
	x := []byte{0x05}

	hSmall, err := ds.HashTreeRoot(&Small{X: x})
	if err != nil {
		t.Fatalf("HashTreeRoot small: %v", err)
	}
	hHuge, err := ds.HashTreeRoot(&Huge{X: x})
	if err != nil {
		t.Fatalf("HashTreeRoot huge: %v", err)
	}
	if hSmall == hHuge {
		t.Error("different ssz-max bitlists must not produce identical roots")
	}

	// The GetTree path uses int-sized chunk limits, so this ssz-max only fits on
	// 64-bit platforms; on 32-bit it is clamped and cannot match the uint64 root.
	if ^uint(0)>>32 != 0 {
		tree, err := ds.GetTree(&Huge{X: x})
		if err != nil {
			t.Fatalf("GetTree huge: %v", err)
		}
		if !bytes.Equal(hHuge[:], tree.Hash()) {
			t.Errorf("HashTreeRoot and GetTree disagree: %x vs %x", hHuge, tree.Hash())
		}
	}
}

// HashTreeRoot must not panic when a list's ssz-max is large enough that the
// chunk limit clamps to math.MaxUint64.
func TestHashTreeRootSurvivesHugeMax(t *testing.T) {
	type T struct {
		V [][32]byte `ssz-max:"18446744073709551615"`
	}

	ds := NewDynSsz(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("HashTreeRoot panicked on huge ssz-max: %v", r)
		}
	}()

	if _, err := ds.HashTreeRoot(&T{V: [][32]byte{{1}}}); err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
}

// A TypeWrapper whose value type is incompatible with its descriptor must fail
// with a clean error at build time instead of panicking in the reflect package.
func TestTypeWrapperRejectsIncompatibleType(t *testing.T) {
	type wrapBytesDesc struct {
		Data []byte `ssz-max:"32"`
	}
	type container struct {
		W TypeWrapper[wrapBytesDesc, string]
	}

	ds := NewDynSsz(nil, WithExtendedTypes())
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MarshalSSZ panicked on mismatched TypeWrapper: %v", r)
		}
	}()

	if _, err := ds.MarshalSSZ(&container{W: TypeWrapper[wrapBytesDesc, string]{Data: "hello"}}); err == nil {
		t.Error("expected error for incompatible TypeWrapper value type")
	}
}

// Unexported struct fields must be skipped entirely (encode and decode) so the
// round-trip no longer panics and the wire layout omits them.
func TestUnexportedFieldsSkipped(t *testing.T) {
	type T struct {
		A uint64
		b uint64
		C uint64
	}
	ds := NewDynSsz(nil)
	src := &T{A: 1, b: 0xdead, C: 3}

	enc, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(enc) != 16 {
		t.Fatalf("expected 16 bytes (exported fields only), got %d", len(enc))
	}

	var dst T
	if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dst.A != 1 || dst.C != 3 {
		t.Errorf("exported fields mismatch: %+v", dst)
	}
	if dst.b != 0 {
		t.Errorf("unexported field should not be decoded, got %d", dst.b)
	}
}

// A list of fixed zero-size elements has no decodable count and must be rejected
// at build time rather than dividing by zero on decode.
func TestListOfZeroSizeElementRejected(t *testing.T) {
	type empty struct{}
	type container struct {
		V []empty `ssz-max:"4"`
	}
	ds := NewDynSsz(nil)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on zero-size list element: %v", r)
		}
	}()

	if _, err := ds.MarshalSSZ(&container{V: []empty{{}}}); err == nil {
		t.Error("expected marshal error for zero-size list element")
	}
	var dst container
	if err := ds.UnmarshalSSZ(&dst, []byte{4, 0, 0, 0}); err == nil {
		t.Error("expected unmarshal error for zero-size list element")
	}
}

// A zero-length array maps to Vector[T, 0], which the SSZ spec declares illegal.
// Such a type must be rejected with a clean error rather than panicking.
func TestZeroLengthArrayFieldRejected(t *testing.T) {
	type T struct {
		A uint64
		V []byte `ssz-max:"32"`
		Z [0]uint64
	}
	ds := NewDynSsz(nil)
	src := &T{A: 7, V: []byte{1, 2, 3}}

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("buffer marshal panicked instead of erroring: %v", r)
			}
		}()
		_, err = ds.MarshalSSZ(src)
	}()
	if err == nil {
		t.Fatal("expected error for zero-length array field")
	}
	if !strings.Contains(err.Error(), "zero length") {
		t.Errorf("unexpected error: %v", err)
	}

	// The streaming encoder must reject it the same way.
	var buf bytes.Buffer
	if err := ds.MarshalSSZWriter(src, &buf); err == nil {
		t.Fatal("expected stream marshal to reject zero-length array field")
	}
}

// A list of optional pointers with nil entries must round-trip without panicking
// on decode.
func TestListOfOptionalsRoundtrip(t *testing.T) {
	type inner struct {
		A uint32
		B uint64
	}
	type container struct {
		V []*inner `ssz-type:"list,optional" ssz-max:"4"`
	}
	ds := NewDynSsz(nil, WithExtendedTypes())

	for _, v := range [][]*inner{
		{{A: 1, B: 2}, nil, {A: 3, B: 4}},
		{nil, nil, nil},
		{{A: 9, B: 9}},
	} {
		src := &container{V: v}
		enc, err := ds.MarshalSSZ(src)
		if err != nil {
			t.Fatalf("marshal %v: %v", v, err)
		}

		var dst container
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("unmarshal panicked for %v: %v", v, r)
				}
			}()
			if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
				t.Fatalf("unmarshal %v: %v", v, err)
			}
		}()

		if len(dst.V) != len(v) {
			t.Fatalf("len mismatch: got %d want %d", len(dst.V), len(v))
		}
		for i := range v {
			if (v[i] == nil) != (dst.V[i] == nil) {
				t.Fatalf("nil mismatch at %d", i)
			}
			if v[i] != nil && *dst.V[i] != *v[i] {
				t.Fatalf("value mismatch at %d: %+v != %+v", i, *dst.V[i], *v[i])
			}
		}
	}
}

// big.Int hash tree roots must not collide for values that differ only by
// trailing zeros (N vs N<<8), and the marshal round-trip must be preserved.
func TestBigIntHTRNoCollisionAndRoundtrip(t *testing.T) {
	type T struct{ N big.Int }
	ds := NewDynSsz(nil, WithExtendedTypes())

	for n := int64(1); n < 100; n++ {
		a, err := ds.HashTreeRoot(&T{N: *big.NewInt(n)})
		if err != nil {
			t.Fatalf("HTR(%d): %v", n, err)
		}
		b, err := ds.HashTreeRoot(&T{N: *new(big.Int).Lsh(big.NewInt(n), 8)})
		if err != nil {
			t.Fatalf("HTR(%d<<8): %v", n, err)
		}
		if a == b {
			t.Fatalf("HTR(%d) collides with HTR(%d<<8)", n, n)
		}
	}

	for _, v := range []int64{0, 1, 255, 256, -7, 1 << 40} {
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
			t.Fatalf("roundtrip %d got %s", v, dst.N.String())
		}
	}
}

// Non-canonical big.Int encodings must be rejected on decode.
func TestBigIntCanonicalDecode(t *testing.T) {
	type T struct{ N big.Int }
	ds := NewDynSsz(nil, WithExtendedTypes())

	for _, bad := range [][]byte{
		{0x04, 0, 0, 0},          // empty payload (offset only)
		{0x04, 0, 0, 0, 0x01},    // negative zero (sign 1, no magnitude)
		{0x04, 0, 0, 0, 0, 0, 1}, // leading-zero magnitude
	} {
		var dst T
		if err := ds.UnmarshalSSZ(&dst, bad); err == nil {
			t.Errorf("expected error for non-canonical encoding %x", bad)
		}
	}
}

// A static ssz-max on a big.Int must be enforced consistently by MarshalSSZ,
// SizeSSZ and HashTreeRoot (which also drives GetTree), so none of them commits
// to a value no decoder can produce.
func TestBigIntMaxEnforced(t *testing.T) {
	type T struct {
		N big.Int `ssz-max:"2"`
	}
	ds := NewDynSsz(nil, WithExtendedTypes())

	within := &T{N: *big.NewInt(0xff)}
	if _, err := ds.MarshalSSZ(within); err != nil {
		t.Fatalf("value within max should marshal: %v", err)
	}
	if _, err := ds.SizeSSZ(within); err != nil {
		t.Errorf("value within max should size: %v", err)
	}
	if _, err := ds.HashTreeRoot(within); err != nil {
		t.Errorf("value within max should hash: %v", err)
	}

	over := &T{N: *big.NewInt(0xfffff)}
	if _, err := ds.MarshalSSZ(over); err == nil {
		t.Error("MarshalSSZ: expected error for big.Int exceeding ssz-max")
	}
	if _, err := ds.SizeSSZ(over); err == nil {
		t.Error("SizeSSZ: expected error for big.Int exceeding ssz-max, matching MarshalSSZ")
	}
	if _, err := ds.HashTreeRoot(over); err == nil {
		t.Error("HashTreeRoot: expected error for big.Int exceeding ssz-max, matching MarshalSSZ")
	}
}

// The decoder enforces the same static ssz-max as the encoder; without the
// check it would accept a payload whose decoded value can be neither
// re-encoded nor hashed.
func TestBigIntMaxEnforcedOnDecode(t *testing.T) {
	type T struct {
		N big.Int `ssz-max:"5"`
	}
	ds := NewDynSsz(nil, WithExtendedTypes())

	// offset + sign byte + 8 magnitude bytes: 9-byte payload > max 5.
	over := []byte{0x04, 0, 0, 0, 0x00, 1, 2, 3, 4, 5, 6, 7, 8}
	var dst T
	if err := ds.UnmarshalSSZ(&dst, over); err == nil {
		t.Error("UnmarshalSSZ: expected error for big.Int payload exceeding ssz-max")
	}
	if err := ds.UnmarshalSSZReader(&dst, bytes.NewReader(over), len(over)); err == nil {
		t.Error("UnmarshalSSZReader: expected error for big.Int payload exceeding ssz-max")
	}

	// sign byte + 4 magnitude bytes: exactly at the limit.
	within := []byte{0x04, 0, 0, 0, 0x00, 1, 2, 3, 4}
	if err := ds.UnmarshalSSZ(&dst, within); err != nil {
		t.Errorf("payload within max should decode: %v", err)
	}
}

// SizeSSZ must enforce a list's ssz-max like MarshalSSZ, so it never reports a
// size for a value that cannot be serialized.
func TestSizeSSZEnforcesListLimit(t *testing.T) {
	type T struct {
		L []uint64 `ssz-max:"2"`
	}
	ds := NewDynSsz(nil, WithNoFastSsz())

	within := &T{L: []uint64{1, 2}}
	if _, err := ds.SizeSSZ(within); err != nil {
		t.Errorf("list within max should size: %v", err)
	}

	over := &T{L: []uint64{1, 2, 3}}
	if _, err := ds.MarshalSSZ(over); err == nil {
		t.Fatal("MarshalSSZ should reject an over-limit list")
	}
	if _, err := ds.SizeSSZ(over); err == nil {
		t.Error("SizeSSZ should reject an over-limit list, matching MarshalSSZ")
	}
}

// The reflection engine honors the plain fastssz `ssz` tag as an ssz-type when
// no ssz-type is set, so it agrees with fastssz delegation instead of silently
// diverging. `ssz:"-"` excludes the field; setting both tags is rejected.
func TestFastsszSszTagHonored(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	t.Run("BitlistTreatedAsType", func(t *testing.T) {
		type viaSsz struct {
			B []byte `ssz:"bitlist" ssz-max:"16"`
		}
		type viaSszType struct {
			B []byte `ssz-type:"bitlist" ssz-max:"16"`
		}
		bl := []byte{0x0f, 0x01} // valid bitlist with terminator bit
		r1, err := ds.HashTreeRoot(&viaSsz{B: bl})
		if err != nil {
			t.Fatalf(`ssz:"bitlist": %v`, err)
		}
		r2, err := ds.HashTreeRoot(&viaSszType{B: bl})
		if err != nil {
			t.Fatalf(`ssz-type:"bitlist": %v`, err)
		}
		if r1 != r2 {
			t.Fatalf(`ssz:"bitlist" root %x != ssz-type:"bitlist" root %x`, r1, r2)
		}
	})

	t.Run("DashExcludesField", func(t *testing.T) {
		type excl struct {
			A     uint64
			Cache uint64 `ssz:"-"`
		}
		buf, err := ds.MarshalSSZ(&excl{A: 7, Cache: 99})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if len(buf) != 8 {
			t.Fatalf("excluded field should not be encoded; got %d bytes, want 8", len(buf))
		}
	})

	t.Run("BothTagsRejected", func(t *testing.T) {
		type both struct {
			B []byte `ssz:"bitlist" ssz-type:"bitlist" ssz-max:"16"`
		}
		if _, err := ds.HashTreeRoot(&both{B: []byte{0x01}}); err == nil {
			t.Fatal("setting both 'ssz' and 'ssz-type' should be rejected")
		}
	})
}

// inconsistentSizeCustom is a custom SSZ type whose reported size (8 bytes) disagrees
// with the 4 bytes it actually encodes. As a variable-size custom field its
// offset is computed from the lying size, so the marshaled length ends up
// shorter than the precomputed total.
type inconsistentSizeCustom struct{ X uint64 }

func (b *inconsistentSizeCustom) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 8 }

func (b *inconsistentSizeCustom) MarshalSSZEncoder(_ sszutils.DynamicSpecs, e sszutils.Encoder) error {
	e.EncodeUint32(uint32(b.X)) // writes only 4 bytes, contradicting SizeSSZDyn
	return nil
}

func (b *inconsistentSizeCustom) UnmarshalSSZDecoder(_ sszutils.DynamicSpecs, _ sszutils.Decoder) error {
	return nil
}

func (b *inconsistentSizeCustom) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, _ sszutils.HashWalker) error {
	return nil
}

// zeroSizeCustom is a custom static type whose sizer reports zero bytes. As a
// list element this makes the element count underivable from the wire format,
// which the descriptor build rejects.
type zeroSizeCustom struct{}

var _ = sszutils.Annotate[zeroSizeCustom](`ssz-type:"custom" ssz-static:"true"`)

func (z *zeroSizeCustom) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return 0 }

func (z *zeroSizeCustom) MarshalSSZEncoder(_ sszutils.DynamicSpecs, _ sszutils.Encoder) error {
	return nil
}

func (z *zeroSizeCustom) UnmarshalSSZDecoder(_ sszutils.DynamicSpecs, _ sszutils.Decoder) error {
	return nil
}

func (z *zeroSizeCustom) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, _ sszutils.HashWalker) error {
	return nil
}

// All three marshal entry points enforce the same length==SizeSSZ guard, so an
// inconsistent nested marshaler is rejected everywhere rather than silently
// returning malformed SSZ from one of them.
func TestMarshalSSZWriterLengthGuard(t *testing.T) {
	type outer struct {
		Inner inconsistentSizeCustom `ssz-type:"custom"`
	}
	ds := NewDynSsz(nil, WithExtendedTypes())
	v := &outer{Inner: inconsistentSizeCustom{X: 1}}

	if _, err := ds.MarshalSSZ(v); err == nil {
		t.Fatal("MarshalSSZ (buffer) should reject a size/length mismatch")
	}

	var buf bytes.Buffer
	if err := ds.MarshalSSZWriter(v, &buf); err == nil {
		t.Fatal("MarshalSSZWriter (stream) should reject a size/length mismatch")
	}

	if _, err := ds.MarshalSSZTo(v, nil); err == nil {
		t.Fatal("MarshalSSZTo (nil buffer) should reject a size/length mismatch")
	}

	// With spare capacity the encoder does not overflow, so only the length
	// guard catches the mismatch here.
	if _, err := ds.MarshalSSZTo(v, make([]byte, 0, 8192)); err == nil {
		t.Fatal("MarshalSSZTo (spare capacity) should reject a size/length mismatch")
	}
}

// MarshalSSZTo appends after any existing content, so its length guard is
// relative to the incoming buffer length.
func TestMarshalSSZToAppendsAfterPrefix(t *testing.T) {
	type simple struct {
		A uint64
		B uint32
	}
	ds := NewDynSsz(nil, WithNoFastSsz())

	prefix := []byte{0xAA, 0xBB, 0xCC}
	out, err := ds.MarshalSSZTo(&simple{A: 1, B: 2}, bytes.Clone(prefix))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(out[:3], prefix) {
		t.Errorf("prefix not preserved: %x", out[:3])
	}
	if len(out) != len(prefix)+12 {
		t.Errorf("unexpected output length %d, want %d", len(out), len(prefix)+12)
	}
}

// The reflection field-tag parser rejects a dimension that is dynamic (`?`) in
// ssz-size but fixed via dynssz-size, so it never silently turns a list into a
// vector (which diverges from the generated code).
func TestReflectionRejectsDynamicStaticSizeConflict(t *testing.T) {
	type T struct {
		M [][32]byte `ssz-size:"?,32" dynssz-size:"COMMITTEE,32" ssz-max:"64"`
	}
	ds := NewDynSsz(map[string]any{"COMMITTEE": uint64(4)}, WithNoFastSsz())

	v := &T{M: [][32]byte{{}, {}, {}, {}}}
	if _, err := ds.HashTreeRoot(v); err == nil {
		t.Fatal("expected conflicting size tags error")
	}
}

// A list of optionals interleaves deferred child subtrees (present elements)
// with raw zero chunks (nil elements) in the incremental hasher; the roots
// must match an independent manual merkleization for every ordering.
func TestHashTreeRootOptionalListOrdering(t *testing.T) {
	type optInner struct {
		A uint64
		B uint64
	}
	type optList struct {
		L []*optInner `ssz-max:"16" ssz-type:"?,optional"`
	}

	hashPair := func(a, b [32]byte) [32]byte {
		var buf [64]byte
		copy(buf[:32], a[:])
		copy(buf[32:], b[:])
		return sha256.Sum256(buf[:])
	}
	chunkU64 := func(v uint64) [32]byte {
		var c [32]byte
		binary.LittleEndian.PutUint64(c[:8], v)
		return c
	}
	manualRoot := func(elems []*optInner) [32]byte {
		leaves := make([][32]byte, 16)
		for i, e := range elems {
			if e != nil {
				leaves[i] = hashPair(chunkU64(e.A), chunkU64(e.B))
			}
		}
		level := leaves
		for len(level) > 1 {
			next := make([][32]byte, len(level)/2)
			for i := range next {
				next[i] = hashPair(level[2*i], level[2*i+1])
			}
			level = next
		}
		return hashPair(level[0], chunkU64(uint64(len(elems))))
	}

	ds := NewDynSsz(nil, WithExtendedTypes())
	cases := [][]*optInner{
		{{A: 1, B: 2}, nil},
		{nil, {A: 1, B: 2}},
		{{A: 1, B: 2}, nil, {A: 3, B: 4}},
		{{A: 1, B: 2}, {A: 3, B: 4}},
		{nil, nil},
	}
	for _, c := range cases {
		got, err := ds.HashTreeRoot(&optList{L: c})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := manualRoot(c); got != want {
			t.Errorf("case %v: got %x want %x", c, got[:8], want[:8])
		}
	}
}

// Spec expressions restricted to the arithmetic subset evaluate with exact
// uint64 arithmetic (full precision beyond 2^53); division rounds up (ceil),
// since partial bytes/bits cannot be serialized.
func TestResolveSpecValueIntegerExpressions(t *testing.T) {
	big := uint64(1) << 53
	ds := NewDynSsz(map[string]any{
		"BIG": big,
		"A":   uint64(10),
		"B":   uint64(3),
	})

	cases := []struct {
		expr     string
		resolved bool
		value    uint64
	}{
		{"BIG + 1", true, big + 1},
		{"BIG * 2 + 1", true, big*2 + 1},
		{"(A + B) * 2", true, 26},
		{"A / B", true, 4},  // ceil division
		{"12 / B", true, 4}, // exact division stays exact
		{"A % B", true, 1},
		{"A - B", true, 7},
		{"UNDEFINED + 1", false, 0}, // unknown identifier -> static fallback
	}
	for _, tc := range cases {
		ok, val, err := ds.ResolveSpecValue(tc.expr)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.expr, err)
			continue
		}
		if ok != tc.resolved || (ok && val != tc.value) {
			t.Errorf("%q: got ok=%v val=%d, want ok=%v val=%d", tc.expr, ok, val, tc.resolved, tc.value)
		}
	}

	// genuine evaluation errors
	for _, expr := range []string{"A / 0", "A % 0", "B - A", "BIG * BIG * BIG"} {
		if _, _, err := ds.ResolveSpecValue(expr); err == nil {
			t.Errorf("%q: expected error", expr)
		}
	}

	// expressions beyond the integer arithmetic subset are rejected
	for _, expr := range []string{"A > B ? A : B", "A == B", "A << 2"} {
		if _, _, err := ds.ResolveSpecValue(expr); err == nil {
			t.Errorf("%q: expected unsupported-expression error", expr)
		}
	}
}

// MarshalSSZ / MarshalSSZTo must enforce the same size ceiling SizeSSZ does,
// so a tiny input with a large ssz-size cannot force a giant allocation that
// SizeSSZ would reject.
// SSZ sizes are uint32 (offsets are 4 bytes), so a size above MaxInt32 is
// valid on 64-bit and must be accepted by SizeSSZ and the encode paths; only
// 32-bit platforms (where int cannot hold it) reject it.
func TestSizeAboveMaxInt32(t *testing.T) {
	type bigVec struct {
		V []byte `ssz-size:"2147483648"` // 2^31, just over MaxInt32
	}
	ds := NewDynSsz(nil)
	v := &bigVec{V: []byte{1, 2}}

	size, err := ds.SizeSSZ(v)
	if math.MaxInt == math.MaxInt32 {
		if err == nil {
			t.Fatal("expected SizeSSZ to reject a >MaxInt32 size on a 32-bit platform")
		}
		return
	}

	// 64-bit: the size is representable and must be reported exactly. Compare
	// via int64 so the literal does not overflow int at compile time on 32-bit
	// (this branch is unreachable there).
	if err != nil {
		t.Fatalf("SizeSSZ rejected a valid >MaxInt32 size on 64-bit: %v", err)
	}
	if int64(size) != int64(2147483648) {
		t.Fatalf("expected size 2147483648, got %d", size)
	}

	// Streaming marshal must produce the full padded output without a giant
	// buffer allocation; the trailing zero-padding is discarded to io.Discard.
	if err := ds.MarshalSSZWriter(v, io.Discard); err != nil {
		t.Fatalf("MarshalSSZWriter of a >MaxInt32 vector failed: %v", err)
	}
}

// delegationProbe is an ordinary container (one uint64 field) that also
// implements the generated Dynamic* SSZ methods with a deliberately
// non-canonical encoding. This lets a test tell which engine ran: delegation
// yields the sentinel output, while the reflection walk yields the canonical
// little-endian encoding of V.
type delegationProbe struct {
	V uint64
}

// delegationSentinel is the fixed output the delegated methods produce. Its
// length differs from the 8-byte reflection encoding so both size and content
// distinguish the two paths.
var delegationSentinel = []byte{0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB, 0xAB}

func (p *delegationProbe) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return len(delegationSentinel) }

func (p *delegationProbe) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, delegationSentinel...), nil
}

// UnmarshalSSZDyn ignores the input and stores a marker, so a decode that ran
// through delegation is distinguishable from a reflection decode of the input.
func (p *delegationProbe) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, _ []byte) error {
	p.V = 0xDECABE
	return nil
}

func (p *delegationProbe) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(0xDEAD)
	return nil
}

// TestNoDelegationForcesReflection verifies that WithNoDelegation bypasses a
// type's own generated Dynamic* methods and drives every operation through the
// reflection engine.
func TestNoDelegationForcesReflection(t *testing.T) {
	const v = uint64(0x0102030405060708)
	canonical := binary.LittleEndian.AppendUint64(nil, v)

	dsDelegate := NewDynSsz(nil)
	dsReflect := NewDynSsz(nil, WithNoDelegation())

	// Marshal + Size: delegation yields the sentinel, reflection the canonical
	// little-endian field encoding.
	if got, err := dsDelegate.MarshalSSZ(&delegationProbe{V: v}); err != nil || !bytes.Equal(got, delegationSentinel) {
		t.Fatalf("delegating MarshalSSZ = %x, err %v; want sentinel", got, err)
	}
	if got, err := dsReflect.MarshalSSZ(&delegationProbe{V: v}); err != nil || !bytes.Equal(got, canonical) {
		t.Fatalf("WithNoDelegation MarshalSSZ = %x, err %v; want %x (reflection)", got, err, canonical)
	}
	if got, _ := dsDelegate.SizeSSZ(&delegationProbe{V: v}); got != len(delegationSentinel) {
		t.Fatalf("delegating SizeSSZ = %d; want %d", got, len(delegationSentinel))
	}
	if got, _ := dsReflect.SizeSSZ(&delegationProbe{V: v}); got != len(canonical) {
		t.Fatalf("WithNoDelegation SizeSSZ = %d; want %d (reflection)", got, len(canonical))
	}

	// Unmarshal: delegation stores the marker, reflection decodes the input.
	var pDelegate, pReflect delegationProbe
	if err := dsDelegate.UnmarshalSSZ(&pDelegate, canonical); err != nil || pDelegate.V != 0xDECABE {
		t.Fatalf("delegating UnmarshalSSZ V = %#x, err %v; want marker", pDelegate.V, err)
	}
	if err := dsReflect.UnmarshalSSZ(&pReflect, canonical); err != nil || pReflect.V != v {
		t.Fatalf("WithNoDelegation UnmarshalSSZ V = %#x, err %v; want %#x (reflection)", pReflect.V, err, v)
	}

	// HashTreeRoot: the two paths produce different roots.
	rootDelegate, err := dsDelegate.HashTreeRoot(&delegationProbe{V: v})
	if err != nil {
		t.Fatalf("delegating HashTreeRoot: %v", err)
	}
	rootReflect, err := dsReflect.HashTreeRoot(&delegationProbe{V: v})
	if err != nil {
		t.Fatalf("WithNoDelegation HashTreeRoot: %v", err)
	}
	if rootDelegate == rootReflect {
		t.Fatalf("HashTreeRoot did not switch engines: both roots %x", rootDelegate)
	}
	if !bytes.Equal(rootReflect[:8], canonical) {
		t.Fatalf("WithNoDelegation HashTreeRoot = %x; want field %x in the first chunk", rootReflect, canonical)
	}
}

var (
	customFastMarker = []byte{0xFA, 0xFA, 0xFA, 0xFA}
	customDynMarker  = []byte{0xD9, 0xD9, 0xD9, 0xD9, 0xD9, 0xD9, 0xD9, 0xD9}
)

// customBoth is a custom SSZ type implementing both the fastssz and the dynssz
// method sets with distinct outputs, so a test can tell which one an engine
// chose.
type customBoth struct{ V uint64 }

var _ = sszutils.Annotate[customBoth](`ssz-type:"custom"`)

func (c *customBoth) MarshalSSZTo(buf []byte) ([]byte, error) {
	return append(buf, customFastMarker...), nil
}
func (c *customBoth) MarshalSSZ() ([]byte, error) {
	return append([]byte(nil), customFastMarker...), nil
}
func (c *customBoth) SizeSSZ() int                    { return len(customFastMarker) }
func (c *customBoth) UnmarshalSSZ(_ []byte) error     { c.V = 0xFA; return nil }
func (c *customBoth) HashTreeRoot() ([32]byte, error) { var r [32]byte; r[0] = 0xFA; return r, nil }

func (c *customBoth) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return len(customDynMarker) }
func (c *customBoth) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, customDynMarker...), nil
}
func (c *customBoth) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, _ []byte) error { c.V = 0xD9; return nil }
func (c *customBoth) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(0xD9)
	return nil
}

// customDynOnly is a custom SSZ type that provides only the dynssz method set —
// no fastssz methods at all — which the type cache must accept.
type customDynOnly struct{ V uint64 }

var _ = sszutils.Annotate[customDynOnly](`ssz-type:"custom"`)

func (c *customDynOnly) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return len(customDynMarker) }
func (c *customDynOnly) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, customDynMarker...), nil
}
func (c *customDynOnly) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, _ []byte) error {
	c.V = 0xD9
	return nil
}
func (c *customDynOnly) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint64(0xD9)
	return nil
}

// Holders embed the custom types as a field so the operation runs through the
// reflection walk's custom-type path (a top-level custom value would instead be
// dispatched directly to its own method, bypassing both the type-cache
// validation and the fastssz/dynssz selection under test).
type (
	customBothHolder    struct{ C customBoth }
	customDynOnlyHolder struct{ C customDynOnly }
)

// TestCustomTypeFastsszOrDynssz covers the custom-type contract: a custom type
// may implement either the fastssz or the dynssz methods per operation (at least
// one each), and when both are present the spec-aware dynssz methods win.
func TestCustomTypeFastsszOrDynssz(t *testing.T) {
	ds := NewDynSsz(nil)

	// A custom type providing only dynssz methods is accepted and usable.
	dynEnc, err := ds.MarshalSSZ(&customDynOnlyHolder{})
	if err != nil {
		t.Fatalf("dynssz-only custom rejected: %v", err)
	}
	if !bytes.Contains(dynEnc, customDynMarker) {
		t.Fatalf("dynssz-only custom marshal = %x, want it to contain %x", dynEnc, customDynMarker)
	}

	// A custom type implementing both prefers its dynssz methods across marshal,
	// size and unmarshal.
	enc, err := ds.MarshalSSZ(&customBothHolder{})
	if err != nil {
		t.Fatalf("custom MarshalSSZ failed: %v", err)
	}
	if !bytes.Contains(enc, customDynMarker) || bytes.Contains(enc, customFastMarker) {
		t.Fatalf("custom marshal = %x, want dynssz output %x (not fastssz %x)", enc, customDynMarker, customFastMarker)
	}

	var back customBothHolder
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("custom UnmarshalSSZ failed: %v", err)
	}
	if back.C.V != 0xD9 {
		t.Fatalf("custom UnmarshalSSZ used marker %#x, want dynssz marker 0xD9", back.C.V)
	}
}

// TestFloat32SignalingNaNPreserved verifies the reflection engine preserves a
// float32's exact bit pattern across marshal/unmarshal/hash instead of
// normalizing a signaling NaN's payload through a float64 round-trip (which
// diverged from the generated code).
func TestFloat32SignalingNaNPreserved(t *testing.T) {
	ds := NewDynSsz(nil, WithExtendedTypes())

	type holder struct {
		F float32
	}
	const sigBits = uint32(0xffa045d0) // signaling NaN: exponent all-ones, quiet bit clear

	enc, err := ds.MarshalSSZ(&holder{F: math.Float32frombits(sigBits)})
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	if got := binary.LittleEndian.Uint32(enc); got != sigBits {
		t.Fatalf("marshal normalized the NaN: got %08x, want %08x", got, sigBits)
	}

	var back holder
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	if got := math.Float32bits(back.F); got != sigBits {
		t.Fatalf("unmarshal normalized the NaN: got %08x, want %08x", got, sigBits)
	}
}

// TestReflectionCoverageEdges exercises the custom-type hashing preference, the
// pointer-to-fixed-vector reflection path, and the unknown-size streaming
// unmarshal entry point.
func TestReflectionCoverageEdges(t *testing.T) {
	ds := NewDynSsz(nil)

	// A custom type hashed through a container prefers its dynssz hasher.
	if _, err := ds.HashTreeRoot(&customBothHolder{}); err != nil {
		t.Fatalf("custom container HashTreeRoot: %v", err)
	}

	// A pointer to a fixed vector round-trips through the reflection vector path.
	type ptrVec struct {
		F *[]uint16 `ssz-size:"2"`
	}
	v := []uint16{7, 8}
	enc, err := ds.MarshalSSZ(&ptrVec{F: &v})
	if err != nil {
		t.Fatalf("marshal ptrVec: %v", err)
	}
	var back ptrVec
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("unmarshal ptrVec: %v", err)
	}
	if back.F == nil || len(*back.F) != 2 || (*back.F)[0] != 7 || (*back.F)[1] != 8 {
		t.Fatalf("ptrVec round-trip lost data: %+v", back.F)
	}

	// Unknown-size streaming unmarshal (size < 0) reads the whole stream and
	// decodes through the buffer path.
	var back2 ptrVec
	if err := ds.UnmarshalSSZReader(&back2, bytes.NewReader(enc), -1); err != nil {
		t.Fatalf("UnmarshalSSZReader(size=-1): %v", err)
	}
	if back2.F == nil || len(*back2.F) != 2 {
		t.Fatalf("unknown-size reader lost data: %+v", back2.F)
	}
}

// topViewSentinel is a valid reflection container whose top-level DynamicView*
// methods emit sentinels the reflection walk never produces, so a
// WithNoDelegation DynSsz must ignore them and encode/size/hash the fields by
// reflection instead.
type topViewSentinel struct {
	A uint32
	B uint16
}

func (v *topViewSentinel) MarshalSSZDynView(any) func(sszutils.DynamicSpecs, []byte) ([]byte, error) {
	return func(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
		return append(buf, 0xDE, 0xAD), nil
	}
}

func (v *topViewSentinel) MarshalSSZEncoderView(any) func(sszutils.DynamicSpecs, sszutils.Encoder) error {
	return func(_ sszutils.DynamicSpecs, enc sszutils.Encoder) error {
		enc.EncodeBytes([]byte{0xDE, 0xAD})
		return nil
	}
}

func (v *topViewSentinel) SizeSSZDynView(any) func(sszutils.DynamicSpecs) int {
	return func(_ sszutils.DynamicSpecs) int { return 2 }
}

func (v *topViewSentinel) UnmarshalSSZDynView(any) func(sszutils.DynamicSpecs, []byte) error {
	return func(_ sszutils.DynamicSpecs, _ []byte) error {
		return errors.New("sentinel view unmarshaler")
	}
}

func (v *topViewSentinel) UnmarshalSSZDecoderView(any) func(sszutils.DynamicSpecs, sszutils.Decoder) error {
	return func(_ sszutils.DynamicSpecs, _ sszutils.Decoder) error {
		return errors.New("sentinel view decoder")
	}
}

func (v *topViewSentinel) HashTreeRootWithDynView(any) func(sszutils.DynamicSpecs, sszutils.HashWalker) error {
	return func(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
		hh.PutUint8(0xEE)
		return nil
	}
}

// WithNoDelegation must also apply to the top-level view-descriptor dispatch,
// not only to plain (non-view) delegation, so generated view code can be
// validated against the reflection engine.
func TestNoDelegationBypassesTopLevelViewMethods(t *testing.T) {
	src := &topViewSentinel{A: 0x11223344, B: 0x5566}
	view := &topViewSentinel{}
	del := NewDynSsz(nil)
	nodel := NewDynSsz(nil, WithNoDelegation())

	want := []byte{0x44, 0x33, 0x22, 0x11, 0x66, 0x55}

	// Delegating dispatch hits the sentinel view methods.
	if got, err := del.MarshalSSZ(src, WithViewDescriptor(view)); err != nil || !bytes.Equal(got, []byte{0xDE, 0xAD}) {
		t.Fatalf("delegating marshal = %x, %v; want sentinel", got, err)
	}
	if got, err := del.MarshalSSZTo(src, nil, WithViewDescriptor(view)); err != nil || !bytes.Equal(got, []byte{0xDE, 0xAD}) {
		t.Fatalf("delegating marshalTo = %x, %v; want sentinel", got, err)
	}
	if sz, err := del.SizeSSZ(src, WithViewDescriptor(view)); err != nil || sz != 2 {
		t.Fatalf("delegating size = %d, %v; want 2", sz, err)
	}

	// No delegation: reflection encodes/sizes the actual fields.
	if got, err := nodel.MarshalSSZ(src, WithViewDescriptor(view)); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("no-delegation marshal = %x, %v; want %x", got, err, want)
	}
	if got, err := nodel.MarshalSSZTo(src, nil, WithViewDescriptor(view)); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("no-delegation marshalTo = %x, %v; want %x", got, err, want)
	}
	if sz, err := nodel.SizeSSZ(src, WithViewDescriptor(view)); err != nil || sz != len(want) {
		t.Fatalf("no-delegation size = %d, %v; want %d", sz, err, len(want))
	}

	// Stream marshal (top-level DynamicViewEncoder branch).
	var delBuf, nodelBuf bytes.Buffer
	if err := del.MarshalSSZWriter(src, &delBuf, WithViewDescriptor(view)); err != nil || !bytes.Equal(delBuf.Bytes(), []byte{0xDE, 0xAD}) {
		t.Fatalf("delegating stream marshal = %x, %v; want sentinel", delBuf.Bytes(), err)
	}
	if err := nodel.MarshalSSZWriter(src, &nodelBuf, WithViewDescriptor(view)); err != nil || !bytes.Equal(nodelBuf.Bytes(), want) {
		t.Fatalf("no-delegation stream marshal = %x, %v; want %x", nodelBuf.Bytes(), err, want)
	}

	// Unmarshal, buffer and reader paths: delegation surfaces the sentinel view
	// errors; no-delegation decodes the fields by reflection.
	var d1, n1, d2, n2 topViewSentinel
	if err := del.UnmarshalSSZ(&d1, want, WithViewDescriptor(view)); err == nil {
		t.Fatal("delegating unmarshal should surface the sentinel view unmarshaler error")
	}
	if err := nodel.UnmarshalSSZ(&n1, want, WithViewDescriptor(view)); err != nil || n1.A != 0x11223344 || n1.B != 0x5566 {
		t.Fatalf("no-delegation unmarshal = %+v, %v", n1, err)
	}
	if err := del.UnmarshalSSZReader(&d2, bytes.NewReader(want), len(want), WithViewDescriptor(view)); err == nil {
		t.Fatal("delegating reader unmarshal should surface the sentinel view decoder error")
	}
	if err := nodel.UnmarshalSSZReader(&n2, bytes.NewReader(want), len(want), WithViewDescriptor(view)); err != nil || n2.A != 0x11223344 || n2.B != 0x5566 {
		t.Fatalf("no-delegation reader unmarshal = %+v, %v", n2, err)
	}

	// Hash tree root: the sentinel and reflection roots differ, and the
	// no-delegation view root matches the plain reflection root of the fields.
	delRoot, err := del.HashTreeRoot(src, WithViewDescriptor(view))
	if err != nil {
		t.Fatalf("delegating htr: %v", err)
	}
	nodelRoot, err := nodel.HashTreeRoot(src, WithViewDescriptor(view))
	if err != nil {
		t.Fatalf("no-delegation htr: %v", err)
	}
	if delRoot == nodelRoot {
		t.Fatal("no-delegation htr must differ from the sentinel view root")
	}
	plainRoot, err := nodel.HashTreeRoot(src)
	if err != nil {
		t.Fatalf("plain htr: %v", err)
	}
	if nodelRoot != plainRoot {
		t.Fatalf("no-delegation view htr %x != plain reflection htr %x", nodelRoot, plainRoot)
	}
}

// A vector whose Go slice is shorter than its declared length is zero-padded;
// for pointer (optional) elements the padding must be sized as a present zero
// element on every pass so SizeSSZ, the buffer marshalers, and the stream
// writer agree on the byte layout.
func TestOptionalVectorPaddingSizeAgreement(t *testing.T) {
	type T struct {
		V    []*uint64 `ssz-size:"4" ssz-type:"vector,optional"`
		Tail []byte    `ssz-max:"64"`
	}
	ds := NewDynSsz(nil, WithExtendedTypes())

	val := uint64(7)
	cases := []struct {
		name string
		v    *T
	}{
		{"empty", &T{Tail: []byte{0xAA}}},
		{"partial", &T{V: []*uint64{&val}, Tail: []byte{0xAA, 0xBB}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, err := ds.SizeSSZ(tc.v)
			if err != nil {
				t.Fatalf("SizeSSZ: %v", err)
			}
			enc, err := ds.MarshalSSZ(tc.v)
			if err != nil {
				t.Fatalf("MarshalSSZ: %v", err)
			}
			if len(enc) != size {
				t.Errorf("MarshalSSZ produced %d bytes, SizeSSZ said %d", len(enc), size)
			}

			to, err := ds.MarshalSSZTo(tc.v, make([]byte, 0, 8192))
			if err != nil {
				t.Fatalf("MarshalSSZTo: %v", err)
			}
			if !bytes.Equal(to, enc) {
				t.Errorf("MarshalSSZTo diverges from MarshalSSZ:\n  to:  %x\n  ssz: %x", to, enc)
			}

			var w bytes.Buffer
			if err := ds.MarshalSSZWriter(tc.v, &w); err != nil {
				t.Fatalf("MarshalSSZWriter: %v", err)
			}
			if !bytes.Equal(w.Bytes(), enc) {
				t.Errorf("MarshalSSZWriter diverges from MarshalSSZ:\n  wr:  %x\n  ssz: %x", w.Bytes(), enc)
			}

			var dst T
			if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
				t.Errorf("roundtrip decode: %v", err)
			}
		})
	}
}

// Descriptor byte sizes are uint32; products and sums that wrap must be
// rejected at descriptor build instead of feeding wrapped sizes into length
// checks and allocations.
func TestDescriptorSizeOverflowRejected(t *testing.T) {
	ds := NewDynSsz(nil)

	t.Run("vector product wraps", func(t *testing.T) {
		// 8 (uint64) * 536870912 == 2^32
		type T struct {
			L [][]uint64 `ssz-max:"10" ssz-size:"?,536870912"`
		}
		var v T
		err := ds.UnmarshalSSZ(&v, []byte{4, 0, 0, 0, 0, 0, 0, 0})
		if err == nil {
			t.Fatal("expected descriptor error for wrapped vector size")
		}
	})

	t.Run("vector in container", func(t *testing.T) {
		type T struct {
			A uint64
			V []uint64 `ssz-size:"536870912"`
			B uint64
		}
		var v T
		err := ds.UnmarshalSSZ(&v, make([]byte, 16))
		if err == nil {
			t.Fatal("expected descriptor error for wrapped vector size")
		}
	})

	t.Run("container sum wraps", func(t *testing.T) {
		// Each field fits in uint32, the sum does not.
		type T struct {
			A []byte `ssz-size:"3000000000"`
			B []byte `ssz-size:"3000000000"`
		}
		var v T
		err := ds.UnmarshalSSZ(&v, make([]byte, 16))
		if err == nil {
			t.Fatal("expected descriptor error for wrapped container size")
		}
	})

	t.Run("multi-dim product wraps", func(t *testing.T) {
		// 8 * 65536 * 65536 wraps; each nesting level guards its own product,
		// and ValidateType shares the guarded build path.
		type T struct {
			V [][]uint64 `ssz-size:"65536,65536"`
		}
		if err := ds.ValidateType(reflect.TypeOf(T{})); err == nil {
			t.Fatal("ValidateType should reject the wrapped multi-dim size")
		}
		var v T
		if err := ds.UnmarshalSSZ(&v, make([]byte, 8)); err == nil {
			t.Fatal("expected descriptor error for wrapped multi-dim size")
		}
	})

	t.Run("three-dim product wraps", func(t *testing.T) {
		type T struct {
			V [][][]uint32 `ssz-size:"4096,4096,4096"`
		}
		if err := ds.ValidateType(reflect.TypeOf(T{})); err == nil {
			t.Fatal("ValidateType should reject the wrapped three-dim size")
		}
	})

	t.Run("zero-size list element", func(t *testing.T) {
		// A custom static type whose sizer reports 0 bytes is the only shape
		// that can reach a static zero-size element descriptor; a list of it
		// is undecodable (element count = region / 0) and must be rejected at
		// descriptor build.
		type T struct {
			L []zeroSizeCustom `ssz-max:"4"`
		}
		var v T
		err := ds.UnmarshalSSZ(&v, []byte{4, 0, 0, 0, 1, 2})
		if err == nil {
			t.Fatal("expected error for zero-size list element instead of a divide-by-zero panic")
		}
	})
}

// A bitlist region that cannot hold a valid encoding for the declared limit is
// rejected before it is allocated; on the reader path the region length comes
// from the caller-declared size, so allocating first would let an untrusted
// framing length force an arbitrarily large allocation.
func TestBitlistRegionExceedingLimitRejected(t *testing.T) {
	type T struct {
		B []byte `ssz-type:"bitlist" ssz-max:"64"` // at most 64/8+1 = 9 wire bytes
	}
	ds := NewDynSsz(nil)

	// Buffer path: a 20-byte region can never be a valid Bitlist[64].
	buf := append([]byte{4, 0, 0, 0}, make([]byte, 20)...)
	buf[len(buf)-1] = 0x01
	var dst T
	if err := ds.UnmarshalSSZ(&dst, buf); err == nil {
		t.Error("UnmarshalSSZ: expected error for bitlist region exceeding the limit")
	}

	// Reader path: 4 real bytes with a 400 MiB declared size.
	err := ds.UnmarshalSSZReader(&dst, bytes.NewReader([]byte{4, 0, 0, 0}), 400<<20)
	if err == nil {
		t.Error("UnmarshalSSZReader: expected error for bitlist region exceeding the limit")
	}
}

// Decoded time.Time values normalize to UTC in both engines; time.Time
// equality includes the Location, so a Local-zone reflection result would
// compare unequal to the generated decoders' UTC result for the same instant.
func TestTimeDecodeLocationUTC(t *testing.T) {
	type T struct {
		T time.Time
		X uint32
	}
	ds := NewDynSsz(nil, WithNoFastSsz())

	src := &T{T: time.Unix(1234567890, 0).UTC(), X: 7}
	enc, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}

	var dst T
	if err := ds.UnmarshalSSZ(&dst, enc); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	if dst.T.Location() != time.UTC {
		t.Errorf("decoded location = %v, want UTC", dst.T.Location())
	}
	if dst.T != src.T {
		t.Errorf("decoded time %v != source %v", dst.T, src.T)
	}
}

// HashTreeRoot must never write into the caller's memory: zero padding for a
// short vector goes into a library-owned buffer, and the root of a value must
// not depend on whether its fields alias a shared backing array.
func TestHashTreeRootDoesNotMutateCallerMemory(t *testing.T) {
	type VecHolder struct {
		Data []byte `ssz-size:"32"`
	}
	ds := NewDynSsz(nil)

	backing := make([]byte, 64)
	for i := range backing {
		backing[i] = 0xAA
	}
	before := bytes.Clone(backing)

	if _, err := ds.HashTreeRoot(&VecHolder{Data: backing[:10:64]}); err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	if !bytes.Equal(backing, before) {
		t.Fatalf("HashTreeRoot mutated caller memory:\n before: %x\n after:  %x", before, backing)
	}

	type TwoShortVecs struct {
		V []byte `ssz-size:"8"`
		W []byte `ssz-size:"8"`
	}
	shared := make([]byte, 32)
	for i := range shared {
		shared[i] = 0xAB
	}
	aliased := &TwoShortVecs{V: shared[0:2:32], W: shared[4:12:32]}
	unaliased := &TwoShortVecs{V: []byte{0xAB, 0xAB}, W: bytes.Repeat([]byte{0xAB}, 8)}

	rootA, err := ds.HashTreeRoot(aliased)
	if err != nil {
		t.Fatalf("HashTreeRoot aliased: %v", err)
	}
	rootU, err := ds.HashTreeRoot(unaliased)
	if err != nil {
		t.Fatalf("HashTreeRoot unaliased: %v", err)
	}
	if rootA != rootU {
		t.Errorf("aliasing changed the root of logically identical values: %x != %x", rootA, rootU)
	}
}

// The unit of a size dimension comes from the tag that produced the resolved
// value; it must not flip depending on whether the number happens to equal
// the static fallback.
func TestSizeTagUnitMerge(t *testing.T) {
	type T struct {
		V []byte `ssz-bitsize:"64" dynssz-size:"S"`
	}
	// dynssz-size names bytes: every resolved value yields a byte vector of
	// that many bytes, including S=64 (== the static bit count).
	for _, s := range []uint64{63, 64, 65} {
		ds := NewDynSsz(map[string]any{"S": s})
		sz, err := ds.SizeSSZ(&T{})
		if err != nil {
			t.Errorf("S=%d: %v", s, err)
			continue
		}
		if sz != int(s) {
			t.Errorf("S=%d: size %d, want %d bytes", s, sz, s)
		}
	}

	// An unresolvable expression shares the static hint (and its unit), so a
	// unit mismatch between the tag families is rejected.
	type U struct {
		V []byte `ssz-size:"8" dynssz-bitsize:"UNKNOWN_SPEC"`
	}
	ds := NewDynSsz(nil)
	if _, err := ds.SizeSSZ(&U{}); err == nil {
		t.Error("expected error for conflicting size units")
	}
}

// Value sizing accumulates in uint64 and rejects totals beyond the uint32 SSZ
// size range instead of wrapping; the wrap here involves a runtime slice
// length, so descriptor-build guards cannot catch it.
func TestSizeSSZValueOverflowRejected(t *testing.T) {
	type InnerHuge struct {
		Data []byte `ssz-size:"268435456"`
	}
	type OuterList struct {
		Items []InnerHuge `ssz-max:"64"`
	}
	ds := NewDynSsz(nil)

	sz, err := ds.SizeSSZ(&OuterList{Items: make([]InnerHuge, 1)})
	if err != nil || sz != 4+268435456 {
		t.Fatalf("n=1: size=%d err=%v", sz, err)
	}
	if _, err := ds.SizeSSZ(&OuterList{Items: make([]InnerHuge, 17)}); err == nil {
		t.Error("expected error for a value size exceeding the uint32 range")
	}
}

// SizeSSZ enforces the declared vector length like MarshalSSZ and
// HashTreeRoot do (arrays truncate instead, matching the other passes).
func TestSizeSSZRejectsOverLengthVector(t *testing.T) {
	ds := NewDynSsz(nil)

	type DynElem struct {
		D []byte `ssz-max:"8"`
	}
	type OverVecDyn struct {
		V []DynElem `ssz-size:"2"`
	}
	if _, err := ds.SizeSSZ(&OverVecDyn{V: make([]DynElem, 5)}); err == nil {
		t.Error("SizeSSZ should reject an over-length dynamic-element vector")
	}

	// A fully static field is sized from the descriptor without walking the
	// value, so the over-length slice surfaces at marshal (whose output-length
	// guard also protects the buffer), not in SizeSSZ.
	type OverVecFixed struct {
		V []uint64 `ssz-size:"2"`
	}
	if _, err := ds.MarshalSSZ(&OverVecFixed{V: []uint64{1, 2, 3}}); err == nil {
		t.Error("MarshalSSZ should reject an over-length fixed-element vector")
	}

	type ArrVec struct {
		F [10]uint8 `ssz-size:"5"`
	}
	if sz, err := ds.SizeSSZ(&ArrVec{}); err != nil || sz != 5 {
		t.Errorf("array truncation changed: size=%d err=%v", sz, err)
	}
}

// PromotedInner has its own dynamic SSZ methods and encodes its Seconds field
// big-endian — a layout only its own method produces, so its bytes reveal
// whether the method was used.
type PromotedInner struct{ Seconds uint16 }

func (p *PromotedInner) SizeSSZDyn(sszutils.DynamicSpecs) int { return 2 }
func (p *PromotedInner) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return binary.BigEndian.AppendUint16(buf, p.Seconds), nil
}
func (p *PromotedInner) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, b []byte) error {
	if len(b) < 2 {
		return fmt.Errorf("short PromotedInner")
	}
	p.Seconds = binary.BigEndian.Uint16(b)
	return nil
}
func (p *PromotedInner) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint16(p.Seconds)
	return nil
}

// PromotedOuter embeds PromotedInner, so Go promotes its SSZ methods. dynssz
// must NOT delegate to the promoted method (which would serialize only the
// embedded Seconds and drop Label); it must walk the struct, delegating to the
// embedded field's own method and encoding Label alongside.
type PromotedOuter struct {
	PromotedInner
	Label uint64
}

func TestEmbeddedPromotionNoFalseDelegation(t *testing.T) {
	ds := NewDynSsz(nil)
	v := &PromotedOuter{PromotedInner: PromotedInner{Seconds: 0x0102}, Label: 0x33}

	size, err := ds.SizeSSZ(v)
	if err != nil || size != 10 {
		t.Fatalf("SizeSSZ = %d, %v; want 10 (2 embedded + 8 label)", size, err)
	}

	enc, err := ds.MarshalSSZ(v)
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	if len(enc) != 10 {
		t.Fatalf("encoded %d bytes (%x); want 10 — Label must not be dropped", len(enc), enc)
	}
	// Embedded field encoded via its own (big-endian) method, then Label.
	if enc[0] != 0x01 || enc[1] != 0x02 {
		t.Errorf("embedded field not encoded by its own method: %x", enc[:2])
	}
	if binary.LittleEndian.Uint64(enc[2:]) != 0x33 {
		t.Errorf("Label not encoded: %x", enc[2:])
	}

	var back PromotedOuter
	if err = ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	if back != *v {
		t.Errorf("round-trip mismatch: got %+v want %+v", back, *v)
	}

	// HashTreeRoot must succeed and be stable across a round-trip.
	r1, err := ds.HashTreeRoot(v)
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	r2, err := ds.HashTreeRoot(&back)
	if err != nil || r1 != r2 {
		t.Errorf("HashTreeRoot unstable across round-trip: %x vs %x (%v)", r1, r2, err)
	}
}

// shadowInner is a delegating inner type.
type shadowInner struct{ S uint16 }

func (s *shadowInner) SizeSSZDyn(sszutils.DynamicSpecs) int { return 2 }
func (s *shadowInner) MarshalSSZDyn(_ sszutils.DynamicSpecs, b []byte) ([]byte, error) {
	return append(b, byte(s.S), byte(s.S>>8)), nil
}
func (s *shadowInner) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, b []byte) error {
	if len(b) >= 2 {
		s.S = uint16(b[0]) | uint16(b[1])<<8
	}
	return nil
}
func (s *shadowInner) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutUint16(s.S)
	return nil
}

// shadowOuter embeds shadowInner but declares its OWN full delegation set
// (writing a 3-byte sentinel) and carries no ssz-static annotation. Because its
// methods are real declarations — not promotion wrappers — dynssz must use them
// (the promoted-wrapper detection distinguishes a shadowing declaration from an
// inherited method).
type shadowOuter struct {
	shadowInner
	Label uint64
}

func (o *shadowOuter) SizeSSZDyn(sszutils.DynamicSpecs) int { return 3 }
func (o *shadowOuter) MarshalSSZDyn(_ sszutils.DynamicSpecs, b []byte) ([]byte, error) {
	return append(b, 0xDE, 0xAD, 0xBE), nil
}
func (o *shadowOuter) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, b []byte) error { return nil }
func (o *shadowOuter) HashTreeRootWithDyn(_ sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	hh.PutBytes([]byte{0xDE, 0xAD, 0xBE})
	return nil
}

func TestEmbeddedShadowDeclarationDelegates(t *testing.T) {
	ds := NewDynSsz(nil)
	enc, err := ds.MarshalSSZ(&shadowOuter{shadowInner: shadowInner{S: 1}, Label: 2})
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	// The outer declares its own method (real source), so it is used even though
	// it also embeds a delegating type and has no ssz-static annotation.
	if len(enc) != 3 || enc[0] != 0xDE || enc[1] != 0xAD || enc[2] != 0xBE {
		t.Fatalf("shadow declaration not used: got %x, want deadbe", enc)
	}
}

// A large-uint (uint128/uint256) whose Go slice is shorter than its declared
// width is zero-padded on hash tree root, matching the marshal paths and the
// generated HTR, instead of being rejected. Over-length slices are still
// rejected.
func TestLargeUintHTRPadsShort(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithNoDelegation())

	t.Run("uint128 bytes", func(t *testing.T) {
		type U struct {
			V []byte `ssz-type:"uint128" ssz-size:"16"`
		}
		short, err := ds.HashTreeRoot(&U{V: []byte{1, 2, 3}})
		if err != nil {
			t.Fatalf("short uint128 should pad, got error: %v", err)
		}
		padded, err := ds.HashTreeRoot(&U{V: append([]byte{1, 2, 3}, make([]byte, 13)...)})
		if err != nil {
			t.Fatal(err)
		}
		if short != padded {
			t.Errorf("short root %x != padded root %x", short, padded)
		}
		if _, err := ds.HashTreeRoot(&U{V: make([]byte, 17)}); err == nil {
			t.Error("over-length uint128 should still be rejected")
		}
	})

	t.Run("uint256 uint64 words", func(t *testing.T) {
		type U struct {
			V []uint64 `ssz-type:"uint256" ssz-size:"4"`
		}
		short, err := ds.HashTreeRoot(&U{V: []uint64{7}})
		if err != nil {
			t.Fatalf("short uint256 should pad, got error: %v", err)
		}
		padded, err := ds.HashTreeRoot(&U{V: []uint64{7, 0, 0, 0}})
		if err != nil {
			t.Fatal(err)
		}
		if short != padded {
			t.Errorf("short root %x != padded root %x", short, padded)
		}
	})
}

// Unknown-size decoding reads a payload to EOF, so it is sensitive to two things
// the rest of the suite never varies: how the reader chops up the data, and how
// big the decoder's read buffer is relative to the payload. Every other
// UnmarshalSSZReader test uses a bytes.Reader (which hands over everything in one
// call) with the default buffer, which only ever exercises the path where the
// initial fill observes EOF and the stream collapses to a known length.

// chunkReader delivers at most n bytes per Read, without ever returning io.EOF
// alongside data.
type chunkReader struct {
	data []byte
	pos  int
	n    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := min(min(len(p), r.n), len(r.data)-r.pos)
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

// stallReader interleaves a (0, nil) no-op read before every real read. Such a
// read is legal per the io.Reader contract and must not be mistaken for EOF.
type stallReader struct {
	r       io.Reader
	pending bool
}

func (r *stallReader) Read(p []byte) (int, error) {
	if !r.pending {
		r.pending = true
		return 0, nil
	}
	r.pending = false
	return r.r.Read(p)
}

func unknownSizeReaders(data []byte) map[string]func() io.Reader {
	return map[string]func() io.Reader{
		"whole":       func() io.Reader { return bytes.NewReader(data) },
		"oneByte":     func() io.Reader { return iotest.OneByteReader(bytes.NewReader(data)) },
		"chunk3":      func() io.Reader { return &chunkReader{data: data, n: 3} },
		"chunk7":      func() io.Reader { return &chunkReader{data: data, n: 7} },
		"dataWithEOF": func() io.Reader { return iotest.DataErrReader(bytes.NewReader(data)) },
		"stalling":    func() io.Reader { return &stallReader{r: bytes.NewReader(data)} },
	}
}

// Buffer sizes straddling the payload: the small ones force genuine open
// regions, the large one lets the initial fill observe EOF.
var unknownSizeBufSizes = []int{8, 16, 61, 4096}

type usInner struct {
	A uint32
	B []byte `ssz-max:"32"`
}

type usCases struct {
	Fixed        uint64
	FixedVec     [3]uint16
	ListFixed    []uint32 `ssz-max:"16"`
	ListUint64   []uint64 `ssz-max:"16"`
	Bits         []byte   `ssz-type:"bitlist" ssz-max:"64"`
	Nested       *usInner
	ListDynamic  []*usInner `ssz-max:"8"`
	Big          big.Int    `ssz-max:"33"`
	TrailingList []byte     `ssz-max:"128"`
}

// A fixed-size root: nothing is dynamic, so the decode must still terminate
// exactly at EOF.
type usFixed struct {
	A uint64
	B [4]byte
}

// A root whose trailing region is a list of dynamic elements — the deepest
// open-region chain: root -> last element -> that element's last field.
type usDeepTail struct {
	Head uint32
	Tail []*usInner `ssz-max:"8"`
}

func usSamples(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"fixed": &usFixed{A: 0x0102030405060708, B: [4]byte{1, 2, 3, 4}},
		"empty-tail": &usCases{
			Fixed: 1, Bits: []byte{0x01}, Nested: &usInner{A: 1},
			Big: *big.NewInt(0),
		},
		"full": &usCases{
			Fixed:        0xdeadbeefcafe,
			FixedVec:     [3]uint16{7, 8, 9},
			ListFixed:    []uint32{1, 2, 3, 4, 5},
			ListUint64:   []uint64{11, 22, 33},
			Bits:         []byte{0xff, 0x03},
			Nested:       &usInner{A: 42, B: []byte("nested payload")},
			ListDynamic:  []*usInner{{A: 1, B: []byte("a")}, {A: 2, B: []byte("bb")}},
			Big:          *big.NewInt(-1234567890),
			TrailingList: []byte("this trailing list is comfortably longer than the small read buffers"),
		},
		"deep-tail": &usDeepTail{
			Head: 9,
			Tail: []*usInner{{A: 1, B: []byte("xx")}, {A: 2}, {A: 3, B: []byte("longer trailing element payload")}},
		},
		"deep-tail-single": &usDeepTail{Head: 1, Tail: []*usInner{{A: 5, B: []byte("only")}}},
		"deep-tail-empty":  &usDeepTail{Head: 2, Tail: []*usInner{}},
	}
}

// Decoding with size < 0 must produce exactly what the buffer path produces, for
// every reader chunking behaviour and every buffer size.
func TestUnknownSizeMatchesBufferPath(t *testing.T) {
	for name, sample := range usSamples(t) {
		t.Run(name, func(t *testing.T) {
			ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
			want, err := ds.MarshalSSZ(sample)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			for rname, mkReader := range unknownSizeReaders(want) {
				for _, bufSize := range unknownSizeBufSizes {
					t.Run(fmt.Sprintf("%s/buf%d", rname, bufSize), func(t *testing.T) {
						dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))

						target := reflect.New(reflect.TypeOf(sample).Elem()).Interface()
						if err := dsr.UnmarshalSSZReader(target, mkReader(), -1); err != nil {
							t.Fatalf("unknown-size decode: %v", err)
						}

						got, err := dsr.MarshalSSZ(target)
						if err != nil {
							t.Fatalf("re-marshal: %v", err)
						}
						if !bytes.Equal(want, got) {
							t.Fatalf("round-trip mismatch:\n want %x\n  got %x", want, got)
						}
					})
				}
			}
		})
	}
}

// Truncation must be rejected at every prefix, in every buffer regime. This is
// the property most at risk from a read-until-EOF decoder: a short read must not
// be mistaken for a well-formed end of input.
func TestUnknownSizeRejectsTruncation(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	sample := usSamples(t)["full"]
	full, err := ds.MarshalSSZ(sample)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))
		for cut := 0; cut < len(full); cut++ {
			target := &usCases{}
			streamErr := dsr.UnmarshalSSZReader(target, bytes.NewReader(full[:cut]), -1)

			// The buffer path is the oracle: unknown-size decoding may report a
			// different error, but never a different verdict.
			bufErr := dsr.UnmarshalSSZ(&usCases{}, full[:cut])
			if (bufErr == nil) != (streamErr == nil) {
				t.Fatalf("buf=%d cut=%d: verdict differs: buffer=%v stream=%v", bufSize, cut, bufErr, streamErr)
			}
		}
	}
}

// A reader that fails mid-payload must surface that error, not a decode error
// that hides it.
func TestUnknownSizeSurfacesReadError(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	full, err := ds.MarshalSSZ(usSamples(t)["full"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	boom := errors.New("read boom")
	dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(8))
	target := &usCases{}
	err = dsr.UnmarshalSSZReader(target, iotest.TimeoutReader(&chunkReader{data: full, n: 4}), -1)
	if err == nil {
		t.Fatal("expected an error from a failing reader")
	}

	err = dsr.UnmarshalSSZReader(&usCases{}, errorReader{boom}, -1)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// The maximum stream size bounds an unknown-size decode and cannot be disabled.
func TestUnknownSizeMaxStreamSize(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	full, err := ds.MarshalSSZ(usSamples(t)["full"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// A cap below the payload size must reject rather than truncate.
	dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(8), WithMaxStreamSize(len(full)-1))
	if err := dsr.UnmarshalSSZReader(&usCases{}, bytes.NewReader(full), -1); err == nil {
		t.Fatal("expected an error when the payload exceeds the maximum stream size")
	}

	// A cap at exactly the payload size still decodes.
	dsr = NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(8), WithMaxStreamSize(len(full)))
	target := &usCases{}
	if err := dsr.UnmarshalSSZReader(target, bytes.NewReader(full), -1); err != nil {
		t.Fatalf("decode at exact cap: %v", err)
	}

	// A non-positive maximum falls back to the default rather than meaning
	// "unlimited" — the bound is load-bearing and cannot be switched off.
	dsr = NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithMaxStreamSize(0))
	if err := dsr.UnmarshalSSZReader(&usCases{}, bytes.NewReader(full), -1); err != nil {
		t.Fatalf("decode with default cap: %v", err)
	}
}

// ssz-max must be enforced while reading, so an over-long list is rejected
// before it is allocated rather than after.
func TestUnknownSizeEnforcesListLimit(t *testing.T) {
	type small struct {
		L []uint32 `ssz-max:"2"`
	}
	type big struct {
		L []uint32 `ssz-max:"64"`
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	over, err := ds.MarshalSSZ(&big{L: []uint32{1, 2, 3, 4, 5}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))
		err := dsr.UnmarshalSSZReader(&small{}, bytes.NewReader(over), -1)
		if !errors.Is(err, sszutils.ErrListTooBig) {
			t.Fatalf("buf=%d: err = %v, want ErrListTooBig", bufSize, err)
		}
	}
}

// A bitlist's terminator lives in the last byte of its region, so an unknown
// length must not lose it.
func TestUnknownSizeBitlistTermination(t *testing.T) {
	type bl struct {
		B []byte `ssz-type:"bitlist" ssz-max:"64"`
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))

		valid, err := ds.MarshalSSZ(&bl{B: []byte{0xff, 0x01}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		target := &bl{}
		if err := dsr.UnmarshalSSZReader(target, bytes.NewReader(valid), -1); err != nil {
			t.Fatalf("buf=%d: valid bitlist: %v", bufSize, err)
		}
		if !bytes.Equal(target.B, []byte{0xff, 0x01}) {
			t.Fatalf("buf=%d: got %x", bufSize, target.B)
		}

		// A zero last byte carries no terminator and must be rejected.
		unterminated := append(append([]byte{}, valid[:4]...), 0xff, 0x00)
		if err := dsr.UnmarshalSSZReader(&bl{}, bytes.NewReader(unterminated), -1); err == nil {
			t.Fatalf("buf=%d: expected an error for an unterminated bitlist", bufSize)
		}
	}
}

// benchState stands in for a large beacon-state-shaped payload: a big trailing
// list of fixed-size records preceded by some dynamic fields. The trailing list
// is what an unknown-size decode has to consume without knowing where it ends.
type benchRecord struct {
	Index   uint64
	Balance uint64
	Key     [48]byte
}

type benchState struct {
	Slot    uint64
	Roots   [][32]byte    `ssz-max:"8192"`
	Records []benchRecord `ssz-max:"1048576"`
}

func benchStatePayload(tb testing.TB, records int) ([]byte, *DynSsz) {
	tb.Helper()
	ds := NewDynSsz(nil, WithNoFastSsz())

	st := &benchState{Slot: 12345}
	for i := range 64 {
		var r [32]byte
		r[0] = byte(i)
		st.Roots = append(st.Roots, r)
	}
	st.Records = make([]benchRecord, records)
	for i := range st.Records {
		st.Records[i] = benchRecord{Index: uint64(i), Balance: uint64(i) * 32}
	}

	data, err := ds.MarshalSSZ(st)
	if err != nil {
		tb.Fatalf("marshal: %v", err)
	}
	return data, ds
}

// BenchmarkUnmarshalReader compares the three decode paths on the same payload.
// The interesting column is B/op: the buffer path and the old unknown-size
// behaviour both have to hold the whole payload, while streaming does not.
func BenchmarkUnmarshalReader(b *testing.B) {
	for _, records := range []int{1000, 20000} {
		data, ds := benchStatePayload(b, records)
		dsStream := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(4096))

		b.Run(fmt.Sprintf("records=%d/buffer", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var out benchState
				if err := ds.UnmarshalSSZ(&out, data); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("records=%d/stream-known", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var out benchState
				if err := dsStream.UnmarshalSSZReader(&out, bytes.NewReader(data), len(data)); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("records=%d/stream-unknown", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var out benchState
				if err := dsStream.UnmarshalSSZReader(&out, bytes.NewReader(data), -1); err != nil {
					b.Fatal(err)
				}
			}
		})

		// The behaviour unknown-size mode replaces: read it all, then decode.
		b.Run(fmt.Sprintf("records=%d/readall-then-buffer", records), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var out benchState
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(bytes.NewReader(data)); err != nil {
					b.Fatal(err)
				}
				if err := ds.UnmarshalSSZ(&out, buf.Bytes()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The trailing region is the only one whose extent is unknown, so a construct
// is only decoded by the read-to-EOF path when it sits last. These types put
// each such construct in that position; the sample types above all end in a
// byte list or a dynamic list, which take other paths.

type usTailElems struct {
	Head uint32
	Tail []uint32 `ssz-max:"16"` // list of fixed non-byte elements
}

type usTailStructs struct {
	Head uint32
	Tail []usFixed `ssz-max:"8"` // list of fixed-size structs
}

type usTailPtrs struct {
	Head uint32
	Tail []*usFixed `ssz-max:"8"` // pointer elements are allocated per item
}

type usTailUnlimited struct {
	Head uint32
	Tail []uint32 `ssz-max:"64"` // limit far above what the stream carries
}

type usTailString struct {
	Head uint32
	Tail string `ssz-max:"64"`
}

type usTailBitlist struct {
	Head uint32
	Tail []byte `ssz-type:"bitlist" ssz-max:"64"`
}

type usTailBigInt struct {
	Head uint32
	Tail big.Int `ssz-max:"33"`
}

type usTailOptional struct {
	Head uint32
	Tail *usFixed `ssz-type:"optional-list"`
}

// Each trailing construct must decode identically to the buffer path, across
// buffer sizes that both force and avoid an open region.
func TestUnknownSizeTrailingConstructs(t *testing.T) {
	samples := map[string]any{
		"list-of-uint32":       &usTailElems{Head: 1, Tail: []uint32{1, 2, 3, 4, 5, 6, 7}},
		"list-of-uint32-empty": &usTailElems{Head: 2, Tail: []uint32{}},
		"list-of-structs":      &usTailStructs{Head: 3, Tail: []usFixed{{A: 1, B: [4]byte{1}}, {A: 2}}},
		"list-of-pointers":     &usTailPtrs{Head: 4, Tail: []*usFixed{{A: 9, B: [4]byte{9}}, {A: 8}}},
		"list-unlimited":       &usTailUnlimited{Head: 5, Tail: []uint32{10, 20, 30, 40, 50, 60}},
		"string":               &usTailString{Head: 6, Tail: "a trailing string longer than the small read buffers"},
		"string-empty":         &usTailString{Head: 7, Tail: ""},
		"bitlist":              &usTailBitlist{Head: 8, Tail: []byte{0xff, 0x03}},
		"bigint":               &usTailBigInt{Head: 9, Tail: *big.NewInt(-987654321)},
		"optional-present":     &usTailOptional{Head: 10, Tail: &usFixed{A: 5, B: [4]byte{5}}},
		"optional-absent":      &usTailOptional{Head: 11, Tail: nil},
	}

	for name, sample := range samples {
		t.Run(name, func(t *testing.T) {
			ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())
			want, err := ds.MarshalSSZ(sample)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			for _, bufSize := range unknownSizeBufSizes {
				dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))
				target := reflect.New(reflect.TypeOf(sample).Elem()).Interface()
				if err := dsr.UnmarshalSSZReader(target, bytes.NewReader(want), -1); err != nil {
					t.Fatalf("buf=%d: %v", bufSize, err)
				}
				got, err := dsr.MarshalSSZ(target)
				if err != nil {
					t.Fatalf("buf=%d re-marshal: %v", bufSize, err)
				}
				if !bytes.Equal(want, got) {
					t.Fatalf("buf=%d: want %x, got %x", bufSize, want, got)
				}

				// Every truncation must reach the same verdict as the buffer path.
				for cut := 0; cut < len(want); cut++ {
					tt := reflect.New(reflect.TypeOf(sample).Elem()).Interface()
					se := dsr.UnmarshalSSZReader(tt, bytes.NewReader(want[:cut]), -1)
					be := dsr.UnmarshalSSZ(reflect.New(reflect.TypeOf(sample).Elem()).Interface(), want[:cut])
					if (se == nil) != (be == nil) {
						t.Fatalf("buf=%d cut=%d: verdict differs: stream=%v buffer=%v", bufSize, cut, se, be)
					}
				}
			}
		})
	}
}

// ssz-max on a trailing list of fixed elements is enforced per element while
// reading, so an over-long list is rejected before it is allocated.
func TestUnknownSizeTrailingListLimit(t *testing.T) {
	type wide struct {
		Head uint32
		Tail []uint32 `ssz-max:"64"`
	}
	ds := NewDynSsz(nil, WithNoFastSsz())
	over, err := ds.MarshalSSZ(&wide{Head: 1, Tail: []uint32{1, 2, 3, 4, 5}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
		if err := dsr.UnmarshalSSZReader(&usTailElems{}, bytes.NewReader(over), -1); err != nil {
			t.Fatalf("buf=%d: within limit should decode: %v", bufSize, err)
		}

		type narrow struct {
			Head uint32
			Tail []uint32 `ssz-max:"2"`
		}
		err := dsr.UnmarshalSSZReader(&narrow{}, bytes.NewReader(over), -1)
		if !errors.Is(err, sszutils.ErrListTooBig) {
			t.Fatalf("buf=%d: err = %v, want ErrListTooBig", bufSize, err)
		}
	}
}

// A trailing list whose elements do not divide the remaining bytes evenly must
// be rejected: the partial element runs into EOF.
func TestUnknownSizeTrailingListMisaligned(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	valid, err := ds.MarshalSSZ(&usTailElems{Head: 1, Tail: []uint32{1, 2}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	misaligned := append(append([]byte{}, valid...), 0xff, 0xff)

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
		streamErr := dsr.UnmarshalSSZReader(&usTailElems{}, bytes.NewReader(misaligned), -1)
		bufErr := dsr.UnmarshalSSZ(&usTailElems{}, misaligned)
		if (streamErr == nil) != (bufErr == nil) {
			t.Fatalf("buf=%d: verdict differs: stream=%v buffer=%v", bufSize, streamErr, bufErr)
		}
		if streamErr == nil {
			t.Fatalf("buf=%d: a misaligned trailing list was accepted", bufSize)
		}
	}
}

// A trailing big.Int or bitlist over its ssz-max must be rejected while
// reading, and report the type's own limit error rather than a stream-size one.
func TestUnknownSizeTrailingPayloadLimits(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes())

	t.Run("bigint", func(t *testing.T) {
		type wide struct {
			Head uint32
			Tail big.Int `ssz-max:"64"`
		}
		big64 := new(big.Int).Lsh(big.NewInt(1), 8*40)
		over, err := ds.MarshalSSZ(&wide{Head: 1, Tail: *big64})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, bufSize := range unknownSizeBufSizes {
			dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))
			err := dsr.UnmarshalSSZReader(&usTailBigInt{}, bytes.NewReader(over), -1)
			if !errors.Is(err, sszutils.ErrListTooBig) {
				t.Fatalf("buf=%d: err = %v, want ErrListTooBig", bufSize, err)
			}
		}
	})

	t.Run("bitlist", func(t *testing.T) {
		type wide struct {
			Head uint32
			Tail []byte `ssz-type:"bitlist" ssz-max:"512"`
		}
		bits := make([]byte, 40)
		bits[39] = 0x01
		over, err := ds.MarshalSSZ(&wide{Head: 1, Tail: bits})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, bufSize := range unknownSizeBufSizes {
			dsr := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(), WithStreamReaderBufferSize(bufSize))
			err := dsr.UnmarshalSSZReader(&usTailBitlist{}, bytes.NewReader(over), -1)
			if err == nil {
				t.Fatalf("buf=%d: an over-limit trailing bitlist was accepted", bufSize)
			}
		}
	})
}

// A trailing list long enough to outgrow the initial allocation exercises the
// geometric growth path, including the point where EOF makes the remaining
// element count exact and the slice is sized to fit instead of doubled.
type usTailLong struct {
	Head uint32
	Tail []uint32 `ssz-max:"1024"`
}

// A pointer-to-slice trailing list, an array-backed one, and a list whose
// ssz-max is below the initial allocation.
type usTailPtrSlice struct {
	Head uint32
	Tail *[]uint32 `ssz-max:"16"`
}

type usTailTinyMax struct {
	Head uint32
	Tail []uint32 `ssz-max:"3"`
}

func TestUnknownSizeTrailingListGrowth(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	// A dense sweep: whether the slice is grown by doubling or sized exactly
	// depends on where EOF lands relative to the growth boundaries, which is a
	// function of both the element count and the read buffer size.
	growthBufSizes := []int{8, 12, 16, 24, 32, 48, 64, 96, 128}
	for n := 1; n <= 40; n++ {
		tail := make([]uint32, n)
		for i := range tail {
			tail[i] = uint32(i * 7)
		}
		want, err := ds.MarshalSSZ(&usTailLong{Head: 1, Tail: tail})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		// Both reader shapes matter here. A plain reader signals EOF only on a
		// separate zero-byte read, so the length becomes known just after the
		// last element; DataErrReader returns the final bytes together with
		// io.EOF, so the length can become known *at* a growth boundary, which
		// is what lets the slice be sized exactly instead of doubled.
		readers := map[string]func() io.Reader{
			"plain":   func() io.Reader { return bytes.NewReader(want) },
			"dataEOF": func() io.Reader { return iotest.DataErrReader(bytes.NewReader(want)) },
		}
		for rname, mkReader := range readers {
			for _, bufSize := range growthBufSizes {
				dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
				got := &usTailLong{}
				if err := dsr.UnmarshalSSZReader(got, mkReader(), -1); err != nil {
					t.Fatalf("n=%d %s buf=%d: %v", n, rname, bufSize, err)
				}
				if len(got.Tail) != n {
					t.Fatalf("n=%d %s buf=%d: decoded %d elements", n, rname, bufSize, len(got.Tail))
				}
				for i := range tail {
					if got.Tail[i] != tail[i] {
						t.Fatalf("n=%d %s buf=%d: element %d = %d, want %d", n, rname, bufSize, i, got.Tail[i], tail[i])
					}
				}
			}
		}
	}
}

func TestUnknownSizeTrailingListShapes(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	slice := []uint32{1, 2, 3, 4, 5}

	t.Run("pointer to slice", func(t *testing.T) {
		want, err := ds.MarshalSSZ(&usTailPtrSlice{Head: 1, Tail: &slice})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, bufSize := range unknownSizeBufSizes {
			dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
			got := &usTailPtrSlice{}
			if err := dsr.UnmarshalSSZReader(got, bytes.NewReader(want), -1); err != nil {
				t.Fatalf("buf=%d: %v", bufSize, err)
			}
			if got.Tail == nil || len(*got.Tail) != len(slice) {
				t.Fatalf("buf=%d: got %v", bufSize, got.Tail)
			}
		}
	})

	// An ssz-max below the initial allocation must clamp it rather than
	// over-allocating and then failing.
	t.Run("ssz-max below the initial allocation", func(t *testing.T) {
		want, err := ds.MarshalSSZ(&usTailTinyMax{Head: 1, Tail: []uint32{1, 2, 3}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, bufSize := range unknownSizeBufSizes {
			dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
			got := &usTailTinyMax{}
			if err := dsr.UnmarshalSSZReader(got, bytes.NewReader(want), -1); err != nil {
				t.Fatalf("buf=%d: %v", bufSize, err)
			}
			if len(got.Tail) != 3 {
				t.Fatalf("buf=%d: got %v", bufSize, got.Tail)
			}
		}
	})

	// A trailing string over its ssz-max is rejected while reading.
	t.Run("string over ssz-max", func(t *testing.T) {
		type wide struct {
			Head uint32
			Tail string `ssz-max:"512"`
		}
		over, err := ds.MarshalSSZ(&wide{Head: 1, Tail: string(make([]byte, 200))})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, bufSize := range unknownSizeBufSizes {
			dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
			err := dsr.UnmarshalSSZReader(&usTailString{}, bytes.NewReader(over), -1)
			if !errors.Is(err, sszutils.ErrListTooBig) {
				t.Fatalf("buf=%d: err = %v, want ErrListTooBig", bufSize, err)
			}
		}
	})
}

// Growth must clamp to ssz-max rather than overshooting it, and an array-backed
// trailing list (which cannot grow) falls back to materialising the region.
func TestUnknownSizeTrailingListClampAndArray(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())

	t.Run("growth clamped to ssz-max", func(t *testing.T) {
		type clamped struct {
			Head uint32
			Tail []uint32 `ssz-max:"12"`
		}
		tail := make([]uint32, 12)
		for i := range tail {
			tail[i] = uint32(i)
		}
		want, err := ds.MarshalSSZ(&clamped{Head: 1, Tail: tail})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, bufSize := range []int{8, 16, 24, 32} {
			dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
			got := &clamped{}
			if err := dsr.UnmarshalSSZReader(got, bytes.NewReader(want), -1); err != nil {
				t.Fatalf("buf=%d: %v", bufSize, err)
			}
			if len(got.Tail) != 12 {
				t.Fatalf("buf=%d: decoded %d elements", bufSize, len(got.Tail))
			}
		}
	})

	// A reader that fails partway through a trailing byte list must surface the
	// read error rather than a decode error.
	t.Run("read error in a trailing byte list", func(t *testing.T) {
		want := errors.New("boom")
		payload, err := ds.MarshalSSZ(&usTailString{Head: 1, Tail: "some trailing text"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(8))
		err = dsr.UnmarshalSSZReader(&usTailString{}, &failAfterReader{data: payload, after: 10, err: want}, -1)
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})

	// The same for a trailing list of fixed elements, which reads element by
	// element rather than in bulk.
	t.Run("read error in a trailing element list", func(t *testing.T) {
		want := errors.New("boom")
		payload, err := ds.MarshalSSZ(&usTailElems{Head: 1, Tail: []uint32{1, 2, 3, 4, 5, 6}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(8))
		err = dsr.UnmarshalSSZReader(&usTailElems{}, &failAfterReader{data: payload, after: 12, err: want}, -1)
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}

// failAfterReader serves data up to a point, then fails.
type failAfterReader struct {
	data  []byte
	pos   int
	after int
	err   error
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.pos >= r.after {
		return 0, r.err
	}
	n := min(len(p), r.after-r.pos)
	n = min(n, len(r.data)-r.pos)
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

// A delegate takes a []byte, so it cannot consume an open region incrementally:
// as a trailing field it forces the region to be materialised first.
type usDynDelegate struct {
	Data []byte `ssz-max:"64"`
}

func (d *usDynDelegate) MarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return append(buf, d.Data...), nil
}

func (d *usDynDelegate) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	d.Data = append([]byte(nil), buf...)
	return nil
}

func (d *usDynDelegate) SizeSSZDyn(_ sszutils.DynamicSpecs) int { return len(d.Data) }

type usTailDelegate struct {
	Head uint32
	Tail *usDynDelegate
}

func TestUnknownSizeTrailingDelegate(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	src := &usTailDelegate{Head: 7, Tail: &usDynDelegate{Data: []byte("a delegated trailing payload")}}
	want, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
		got := &usTailDelegate{}
		if err := dsr.UnmarshalSSZReader(got, bytes.NewReader(want), -1); err != nil {
			t.Fatalf("buf=%d: %v", bufSize, err)
		}
		if got.Tail == nil || !bytes.Equal(got.Tail.Data, src.Tail.Data) {
			t.Fatalf("buf=%d: got %q", bufSize, got.Tail)
		}
	}
}

// A fixed-length vector of dynamic elements as the trailing region: the vector
// length comes from the type, but the last element still runs to EOF.
type usTailDynVector struct {
	Head uint32
	Tail [3]*usInner
}

func TestUnknownSizeTrailingDynamicVector(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	src := &usTailDynVector{Head: 4, Tail: [3]*usInner{
		{A: 1, B: []byte("one")},
		{A: 2, B: nil},
		{A: 3, B: []byte("a longer trailing payload")},
	}}
	want, err := ds.MarshalSSZ(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
		got := &usTailDynVector{}
		if err := dsr.UnmarshalSSZReader(got, bytes.NewReader(want), -1); err != nil {
			t.Fatalf("buf=%d: %v", bufSize, err)
		}
		re, err := dsr.MarshalSSZ(got)
		if err != nil {
			t.Fatalf("buf=%d re-marshal: %v", bufSize, err)
		}
		if !bytes.Equal(want, re) {
			t.Fatalf("buf=%d: want %x got %x", bufSize, want, re)
		}

		// Every truncation must reach the same verdict as the buffer path.
		for cut := 0; cut < len(want); cut++ {
			se := dsr.UnmarshalSSZReader(&usTailDynVector{}, bytes.NewReader(want[:cut]), -1)
			be := dsr.UnmarshalSSZ(&usTailDynVector{}, want[:cut])
			if (se == nil) != (be == nil) {
				t.Fatalf("buf=%d cut=%d: verdict differs: stream=%v buffer=%v", bufSize, cut, se, be)
			}
		}
	}
}

// A trailing region that its type does not fully consume must be rejected. An
// optional-list with a fixed-size element reads exactly one element, so any
// bytes after it are trailing data that the end-of-input assertion has to catch.
func TestUnknownSizeTrailingUnderConsumption(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	want, err := ds.MarshalSSZ(&usTailOptional{Head: 1, Tail: &usFixed{A: 9, B: [4]byte{1, 2, 3, 4}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	over := append(append([]byte{}, want...), 0xff)

	for _, bufSize := range unknownSizeBufSizes {
		dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
		streamErr := dsr.UnmarshalSSZReader(&usTailOptional{}, bytes.NewReader(over), -1)
		bufErr := dsr.UnmarshalSSZ(&usTailOptional{}, over)
		if (streamErr == nil) != (bufErr == nil) {
			t.Fatalf("buf=%d: verdict differs: stream=%v buffer=%v", bufSize, streamErr, bufErr)
		}
		if streamErr == nil {
			t.Fatalf("buf=%d: an under-consumed trailing region was accepted", bufSize)
		}
	}
}

// A reader that fails while the decoder is probing the trailing region must
// surface that error. The probe happens at several points: the emptiness test
// for dynamic and optional lists, and the end-of-input assertion that closes an
// open region. Sweeping the failure point covers all of them.
func TestUnknownSizeTrailingProbeReadErrors(t *testing.T) {
	ds := NewDynSsz(nil, WithNoFastSsz())
	boom := errors.New("boom")

	samples := map[string]any{
		"dynamic-list":   &usDeepTail{Head: 1, Tail: []*usInner{{A: 1, B: []byte("xx")}, {A: 2, B: []byte("yy")}}},
		"dynamic-vector": &usTailDynVector{Head: 2, Tail: [3]*usInner{{A: 1}, {A: 2}, {A: 3, B: []byte("zz")}}},
		"optional-list":  &usTailOptional{Head: 3, Tail: &usFixed{A: 9}},
		"element-list":   &usTailElems{Head: 4, Tail: []uint32{1, 2, 3, 4}},
	}

	for name, sample := range samples {
		t.Run(name, func(t *testing.T) {
			payload, err := ds.MarshalSSZ(sample)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, bufSize := range []int{8, 16} {
				for after := 1; after <= len(payload); after++ {
					dsr := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(bufSize))
					target := reflect.New(reflect.TypeOf(sample).Elem()).Interface()
					err := dsr.UnmarshalSSZReader(target, &failAfterReader{data: payload, after: after, err: boom}, -1)
					if err == nil {
						t.Fatalf("buf=%d after=%d: a truncated-by-error stream decoded cleanly", bufSize, after)
					}
					// The reader's error must not be swallowed by a decode error
					// when it happens before the payload is exhausted.
					if after < len(payload) && !errors.Is(err, boom) && !errors.Is(err, sszutils.ErrUnexpectedEOF) {
						t.Fatalf("buf=%d after=%d: unexpected err %v", bufSize, after, err)
					}
				}
			}
		})
	}
}

type terminalDataErrorReader struct {
	data []byte
	pos  int
	err  error
}

func (r *terminalDataErrorReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos == len(r.data) {
		return n, r.err
	}
	return n, nil
}

// A reader may return data and an error together. io.EOF means those bytes are
// the clean end of input, but every other error is part of the reader's result:
// accepting the SSZ value would discard an integrity or decompression verdict.
func TestUnmarshalSSZReaderPreservesTerminalDataErrors(t *testing.T) {
	type fixed struct {
		Data [16]byte
	}

	payload := make([]byte, 16)
	for i := range payload {
		payload[i] = byte(i + 1)
	}

	for name, want := range map[string]error{
		"integrity":      errors.New("checksum failed"),
		"unexpected EOF": io.ErrUnexpectedEOF,
	} {
		t.Run(name, func(t *testing.T) {
			for _, size := range []int{len(payload), -1} {
				mode := "known"
				if size < 0 {
					mode = "unknown"
				}
				t.Run(mode, func(t *testing.T) {
					reader := &terminalDataErrorReader{data: payload, err: want}
					ds := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(8))
					var out fixed
					err := ds.UnmarshalSSZReader(&out, reader, size)
					if !errors.Is(err, want) {
						t.Fatalf("UnmarshalSSZReader error = %v, want %v", err, want)
					}
				})
			}
		})
	}
}

// A peer that declares a huge field in a tiny payload must not be able to make
// the decoder allocate from that declaration.
//
// Inside an open region an offset is validated against the allowance rather than
// against bytes that exist, and pushing a limit for a non-trailing dynamic field
// turns the open region into a bounded one. If that bounded extent were treated
// as known, the field's own decode would size an allocation from it before
// reading any payload: 12 crafted bytes pinned ~500 MB of heap while the peer
// simply kept the connection open.
func TestUnknownSizeDoesNotAllocateFromDeclaredOffsets(t *testing.T) {
	type twoDyn struct {
		A []byte `ssz-max:"1073741824"` // 1 GiB, as ExecutionPayload transactions are
		B []byte `ssz-max:"1073741824"`
	}

	// offset(A)=8, offset(B)=500 MB: A's region is declared as ~500 MB. The
	// tail pads past the initial buffer fill so the decode starts in an open
	// region rather than collapsing to a known length.
	padded := make([]byte, 12+4096)
	binary.LittleEndian.PutUint32(padded[0:4], 8)
	binary.LittleEndian.PutUint32(padded[4:8], 500_000_000)

	ds := NewDynSsz(nil, WithNoFastSsz())

	// The bad allocation is freed as soon as the decode fails, so measuring
	// afterwards sees nothing. What matters is the peak while the decode is
	// running — that is the memory an idle peer can pin by staying connected.
	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)

	var peak uint64
	stop := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak {
					peak = m.HeapAlloc
				}
			}
		}
	}()

	_ = ds.UnmarshalSSZReader(&twoDyn{}, &blockingReader{data: padded}, -1)

	close(stop)
	<-sampled

	const limit = 64 << 20 // generous: the honest cost here is kilobytes
	if peak > base.HeapAlloc+limit {
		t.Fatalf("declared offsets drove a %d byte peak heap from a %d byte payload",
			peak-base.HeapAlloc, len(padded))
	}
}

// blockingReader serves its data and then reports a read failure, standing in
// for a peer that delivers a little data and then goes quiet. It fails rather
// than blocking so the test terminates — by then any allocation driven by the
// declared offsets has already happened.
type blockingReader struct {
	data []byte
	pos  int
}

func (r *blockingReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// A limit derived inside an open region bounds reads but is not a verified
// extent, so it must not be reported as a known length.
func TestUnknownSizeDerivedLimitIsNotKnownLength(t *testing.T) {
	dec := sszutils.NewUnknownStreamDecoder(bytes.NewReader(make([]byte, 16)), 8, 0)
	dec.PushOpenLimit()
	dec.PushLimit(500_000_000)
	if dec.LengthKnown() {
		t.Fatal("a limit derived from an unverified offset must not report a known length")
	}
	if got := dec.GetLength(); got != 500_000_000 {
		t.Fatalf("GetLength = %d, want the declared limit for offset arithmetic", got)
	}
	// Consuming it must not preallocate from the declaration, and must fail at
	// the real end of input: the region was declared bounded, so stopping short
	// of its limit is truncated input rather than a short result.
	if _, err := dec.DecodeRemaining(-1); !errors.Is(err, sszutils.ErrUnexpectedEOF) {
		t.Fatalf("DecodeRemaining = %v, want ErrUnexpectedEOF", err)
	}
}

// A dynamic list takes its element count from the first offset, so the count is
// declared by the input rather than witnessed by it. Sizing the offset table
// from that declaration let a peer turn a few delivered bytes into a large
// allocation, independently of the region-length vector above: with a realistic
// ssz-max (BeaconState.Validators is 2^40) the limit check does not constrain it.
func TestUnknownSizeDoesNotAllocateFromDeclaredElementCount(t *testing.T) {
	type inner struct {
		Data []byte `ssz-max:"1024"`
	}
	type outer struct {
		Items []*inner `ssz-max:"1099511627776"`
	}

	// offset(Items)=4, first element offset 400 MB => 100 million declared
	// elements from eight crafted bytes.
	payload := make([]byte, 8+4096)
	binary.LittleEndian.PutUint32(payload[0:4], 4)
	binary.LittleEndian.PutUint32(payload[4:8], 400_000_000)

	ds := NewDynSsz(nil, WithNoFastSsz())

	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)

	var peak uint64
	stop, sampled := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sampled)
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak {
					peak = m.HeapAlloc
				}
			}
		}
	}()

	_ = ds.UnmarshalSSZReader(&outer{}, &blockingReader{data: payload}, -1)

	close(stop)
	<-sampled

	const limit = 64 << 20
	if peak > base.HeapAlloc+limit {
		t.Fatalf("a declared element count drove a %d byte peak heap from a %d byte payload",
			peak-base.HeapAlloc, len(payload))
	}
}

// In an unknown-size region, a dynamic-list offset table proves the element
// count but it does not prove that even the first element body exists. The
// reflection stream decoder must reserve only a small prefix of a wide []T
// before attempting that body, rather than materialising the schema's entire
// maximum result.
func TestUnknownSizeDynamicListWideValueAllocationIsIncremental(t *testing.T) {
	type wideDynamicElement struct {
		Fixed [64 << 10]byte
		Tail  []byte `ssz-max:"1"`
	}
	type wideDynamicList struct {
		Items []wideDynamicElement `ssz-max:"512"`
	}

	const itemCount = 512
	offsetTable := make([]byte, itemCount*4)
	for pos := 0; pos < len(offsetTable); pos += 4 {
		binary.LittleEndian.PutUint32(offsetTable[pos:pos+4], uint32(len(offsetTable)))
	}
	payload := make([]byte, 4+len(offsetTable))
	binary.LittleEndian.PutUint32(payload[:4], 4)
	copy(payload[4:], offsetTable)

	ds := NewDynSsz(
		nil,
		WithNoFastSsz(),
		WithStreamReaderBufferSize(8),
		WithMaxStreamSize(128<<20),
	)
	if _, err := ds.SizeSSZ(&wideDynamicList{}); err != nil {
		t.Fatalf("warm type cache: %v", err)
	}

	valid := wideDynamicList{Items: make([]wideDynamicElement, 2)}
	valid.Items[0].Fixed[0] = 1
	valid.Items[0].Tail = []byte{2}
	valid.Items[1].Fixed[0] = 3
	validPayload, err := ds.MarshalSSZ(&valid)
	if err != nil {
		t.Fatalf("marshal valid list: %v", err)
	}
	var roundTrip wideDynamicList
	if err := ds.UnmarshalSSZReader(&roundTrip, bytes.NewReader(validPayload), -1); err != nil {
		t.Fatalf("incrementally decode valid list: %v", err)
	}
	if len(roundTrip.Items) != 2 ||
		cap(roundTrip.Items) != 2 ||
		roundTrip.Items[0].Fixed[0] != 1 ||
		!bytes.Equal(roundTrip.Items[0].Tail, []byte{2}) ||
		roundTrip.Items[1].Fixed[0] != 3 {
		t.Fatal("incrementally grown list did not round-trip")
	}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	var out wideDynamicList
	if err := ds.UnmarshalSSZReader(&out, bytes.NewReader(payload), -1); err == nil {
		t.Fatal("offset table without element bodies decoded successfully")
	}

	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	const allocationLimit = 4 << 20
	if allocated > allocationLimit {
		t.Fatalf(
			"%d bytes of malformed input allocated %d bytes, want at most %d",
			len(payload),
			allocated,
			allocationLimit,
		)
	}
}

// eofWithDataReader delivers its final bytes together with io.EOF, which the
// io.Reader contract permits. That makes the decoder observe EOF while data is
// still buffered, so the stream length becomes exact while a region derived
// from an earlier offset still claims far more.
type eofWithDataReader struct {
	data []byte
	pos  int
}

func (r *eofWithDataReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

// Discovering EOF must not retroactively make a region declared by a hostile
// offset trustworthy. The list decoder sizes its slice exactly once the length
// is known, so a region still claiming 400 MB after a 264 byte stream ended
// would drive the allocation the incremental growth exists to prevent.
func TestUnknownSizeStaleRegionDoesNotSizeAllocation(t *testing.T) {
	type twoDyn struct {
		A []uint32 `ssz-max:"1099511627776"`
		B []byte   `ssz-max:"1024"`
	}

	// offset(A)=8, offset(B)=400 MB: A's region is declared as ~400 MB while
	// the payload holds 64 elements. A tiny read buffer keeps the decode in an
	// open region, and the reader hands its last bytes back with io.EOF.
	const items = 64
	payload := make([]byte, 8+items*4)
	binary.LittleEndian.PutUint32(payload[0:4], 8)
	binary.LittleEndian.PutUint32(payload[4:8], 400_000_000)

	ds := NewDynSsz(nil, WithNoFastSsz(), WithStreamReaderBufferSize(16))

	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)

	var peak uint64
	stop, sampled := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sampled)
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak {
					peak = m.HeapAlloc
				}
			}
		}
	}()

	err := ds.UnmarshalSSZReader(&twoDyn{}, &eofWithDataReader{data: payload}, -1)

	close(stop)
	<-sampled

	if err == nil {
		t.Fatal("a region declared past the end of the stream must not decode")
	}
	const limit = 64 << 20
	if peak > base.HeapAlloc+limit {
		t.Fatalf("a stale declared region drove a %d byte peak heap from a %d byte payload",
			peak-base.HeapAlloc, len(payload))
	}
}

// A big.Int with no static ssz-max has no read cap, so an over-long stream
// surfaces as the decoder's own limit error. It must not be mapped through the
// ssz-max check, which has nothing to report and would return nil.
func TestUnknownSizeBigIntWithoutLimitSurfacesStreamLimit(t *testing.T) {
	type T struct{ N big.Int }

	payload := make([]byte, 4+200)
	binary.LittleEndian.PutUint32(payload[0:4], 4)
	for i := 5; i < len(payload); i++ {
		payload[i] = 0x01
	}

	ds := NewDynSsz(nil, WithNoFastSsz(), WithExtendedTypes(),
		WithStreamReaderBufferSize(16), WithMaxStreamSize(64))

	var out T
	err := ds.UnmarshalSSZReader(&out, &eofWithDataReader{data: payload}, -1)
	if !errors.Is(err, sszutils.ErrStreamTooLarge) {
		t.Fatalf("UnmarshalSSZReader = %v, want ErrStreamTooLarge", err)
	}
}
