// Package engine implements the core fuzzing logic for comparing SSZ
// reflection-based and codegen-based implementations.
package engine

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"sync/atomic"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/fuzz/corpus"
	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/reflection"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Stats tracks fuzzing statistics.
type Stats struct {
	Iterations        atomic.Uint64
	ValidFills        atomic.Uint64
	MutatedFills      atomic.Uint64
	RandomInputs      atomic.Uint64
	Panics            atomic.Uint64
	MarshalMismatches atomic.Uint64
	HTRMismatches     atomic.Uint64
	StreamMismatches  atomic.Uint64
	UnmarshalDiffs    atomic.Uint64
	Successes         atomic.Uint64
}

// Engine is the core fuzz testing engine.
// Each Engine instance owns a private RNG and Filler (not thread-safe),
// but shares DynSsz instances, Stats, and Reporter across workers.
type Engine struct {
	ds         *dynssz.DynSsz
	dsExtended *dynssz.DynSsz
	reporter   *Reporter
	stats      *Stats
	rng        *rand.Rand
	filler     *Filler
	maxDataLen int
}

// NewEngine creates a new fuzz engine with its own RNG but shared DynSsz
// instances. The DynSsz TypeCache is thread-safe (uses sync.RWMutex),
// so sharing avoids duplicating large type caches across workers.
func NewEngine(reporter *Reporter, stats *Stats, ds, dsExtended *dynssz.DynSsz, seed int64, maxDataLen int) *Engine {
	rng := rand.New(rand.NewSource(seed))

	return &Engine{
		ds:         ds,
		dsExtended: dsExtended,
		reporter:   reporter,
		stats:      stats,
		rng:        rng,
		filler:     NewFiller(rng),
		maxDataLen: maxDataLen,
	}
}

// FuzzEntry runs one fuzz iteration on a given type entry.
// It alternates between three modes:
//   - Valid fill: create a struct with random values, test all comparisons
//   - Mutated: valid data with random mutations
//   - Random bytes: purely random input data
func (e *Engine) FuzzEntry(entry corpus.TypeEntry) {
	e.stats.Iterations.Add(1)

	ds := e.ds
	if entry.Extended {
		ds = e.dsExtended
	}

	// Choose fuzz mode
	roll := e.rng.Intn(100)
	switch {
	case roll < 40:
		// Valid fill - create populated instances, compare directly
		e.stats.ValidFills.Add(1)
		e.fuzzValidInstance(entry, ds)
	case roll < 60:
		// Mutated valid data
		e.stats.MutatedFills.Add(1)
		e.fuzzMutatedValid(entry, ds)
	default:
		// Random bytes
		e.stats.RandomInputs.Add(1)
		e.fuzzRandomBytes(entry, ds)
	}
}

// FuzzEntryWithData runs a fuzz iteration with specific data.
func (e *Engine) FuzzEntryWithData(entry corpus.TypeEntry, data []byte) {
	e.stats.Iterations.Add(1)

	ds := e.ds
	if entry.Extended {
		ds = e.dsExtended
	}

	e.fuzzUnmarshalCompare(entry, ds, data)
}

// fuzzValidInstance creates a struct with random valid values and compares
// reflection vs codegen on marshal, HTR, streaming, and round-trip.
func (e *Engine) fuzzValidInstance(entry corpus.TypeEntry, ds *dynssz.DynSsz) {
	instance := entry.New()

	// Fill with random values
	fillErr := e.catchPanic(fmt.Sprintf("fill[%s]", entry.Name), entry, nil, func() error {
		e.filler.FillStruct(instance)
		return nil
	})
	if isPanicError(fillErr) {
		return
	}

	e.stats.Successes.Add(1)

	// Compare marshal: reflection vs codegen
	e.compareMarshal(entry, ds, instance, instance, nil)

	// Compare HTR: reflection vs codegen
	e.compareHTR(entry, ds, instance, instance, nil)

	// Compare streaming vs buffer
	e.compareStreaming(entry, ds, instance, nil)

	// Round-trip: marshal -> unmarshal -> marshal
	e.testRoundTrip(entry, ds, instance, nil)
}

// fuzzMutatedValid creates valid data, then mutates it and tests unmarshal.
func (e *Engine) fuzzMutatedValid(entry corpus.TypeEntry, ds *dynssz.DynSsz) {
	instance := entry.New()

	// Fill with random values
	fillErr := e.catchPanic(fmt.Sprintf("fill-mutate[%s]", entry.Name), entry, nil, func() error {
		e.filler.FillStruct(instance)
		return nil
	})
	if isPanicError(fillErr) {
		return
	}

	// Marshal to get valid SSZ bytes
	var validData []byte
	marshalErr := e.catchPanic(fmt.Sprintf("marshal-for-mutate[%s]", entry.Name), entry, nil, func() error {
		var err error
		validData, err = ds.MarshalSSZ(instance)
		return err
	})
	if isPanicError(marshalErr) || marshalErr != nil {
		return
	}

	// Mutate the valid data
	mutated := e.mutateData(validData)

	// Fuzz with the mutated data
	e.fuzzUnmarshalCompare(entry, ds, mutated)
}

// fuzzRandomBytes generates random bytes and tests unmarshal comparison.
func (e *Engine) fuzzRandomBytes(entry corpus.TypeEntry, ds *dynssz.DynSsz) {
	dataLen := e.randomDataLen()
	data := make([]byte, dataLen)
	for i := range data {
		data[i] = byte(e.rng.Intn(256))
	}

	e.fuzzUnmarshalCompare(entry, ds, data)
}

func (e *Engine) randomDataLen() int {
	roll := e.rng.Intn(100)
	switch {
	case roll < 5:
		return 0
	case roll < 15:
		return 1 + e.rng.Intn(4)
	case roll < 40:
		return 1 + e.rng.Intn(64)
	case roll < 70:
		return 1 + e.rng.Intn(256)
	case roll < 90:
		return 1 + e.rng.Intn(1024)
	default:
		return 1 + e.rng.Intn(e.maxDataLen)
	}
}

// mutateData applies random mutations to a byte slice.
func (e *Engine) mutateData(data []byte) []byte {
	if len(data) == 0 {
		// Can't mutate empty data, return some random bytes
		n := 1 + e.rng.Intn(32)
		out := make([]byte, n)
		for i := range out {
			out[i] = byte(e.rng.Intn(256))
		}
		return out
	}

	mutated := make([]byte, len(data))
	copy(mutated, data)

	// Apply 1-5 mutations
	numMutations := 1 + e.rng.Intn(5)
	for range numMutations {
		roll := e.rng.Intn(100)
		switch {
		case roll < 30:
			// Flip random bits
			idx := e.rng.Intn(len(mutated))
			mutated[idx] ^= byte(1 << e.rng.Intn(8))
		case roll < 50:
			// Replace random byte
			idx := e.rng.Intn(len(mutated))
			mutated[idx] = byte(e.rng.Intn(256))
		case roll < 60:
			// Set byte to 0
			idx := e.rng.Intn(len(mutated))
			mutated[idx] = 0
		case roll < 70:
			// Set byte to 0xff
			idx := e.rng.Intn(len(mutated))
			mutated[idx] = 0xff
		case roll < 80:
			// Insert random byte
			idx := e.rng.Intn(len(mutated) + 1)
			mutated = append(mutated[:idx], append([]byte{byte(e.rng.Intn(256))}, mutated[idx:]...)...)
		case roll < 90:
			// Delete random byte
			if len(mutated) > 1 {
				idx := e.rng.Intn(len(mutated))
				mutated = append(mutated[:idx], mutated[idx+1:]...)
			}
		default:
			// Swap two random bytes
			if len(mutated) > 1 {
				i := e.rng.Intn(len(mutated))
				j := e.rng.Intn(len(mutated))
				mutated[i], mutated[j] = mutated[j], mutated[i]
			}
		}
	}

	return mutated
}

func (e *Engine) fuzzUnmarshalCompare(entry corpus.TypeEntry, ds *dynssz.DynSsz, data []byte) {
	reflTarget := entry.New()
	codegenTarget := entry.New()

	// Unmarshal via reflection (bypassing codegen interfaces)
	reflErr := e.catchPanic(fmt.Sprintf("reflection-unmarshal[%s]", entry.Name), entry, data, func() error {
		return e.unmarshalReflection(ds, reflTarget, data)
	})

	// Unmarshal via codegen (using generated methods)
	codegenErr := e.catchPanic(fmt.Sprintf("codegen-unmarshal[%s]", entry.Name), entry, data, func() error {
		return e.unmarshalCodegen(ds, codegenTarget, data)
	})

	if isPanicError(reflErr) || isPanicError(codegenErr) {
		return
	}

	reflOk := reflErr == nil
	codegenOk := codegenErr == nil

	if reflOk != codegenOk {
		e.stats.UnmarshalDiffs.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueUnmarshalDiff,
			TypeName: entry.Name,
			Data:     data,
			Details: fmt.Sprintf(
				"reflection unmarshal %s (err: %v), codegen unmarshal %s (err: %v)",
				boolStr(reflOk, "succeeded", "failed"), reflErr,
				boolStr(codegenOk, "succeeded", "failed"), codegenErr,
			),
		})
		return
	}

	if !reflOk {
		return
	}

	e.stats.Successes.Add(1)

	e.compareMarshal(entry, ds, reflTarget, codegenTarget, data)
	e.compareHTR(entry, ds, reflTarget, codegenTarget, data)
	e.compareStreaming(entry, ds, reflTarget, data)
	e.testRoundTrip(entry, ds, reflTarget, data)
}

func (e *Engine) compareMarshal(entry corpus.TypeEntry, ds *dynssz.DynSsz, reflTarget, codegenTarget any, origData []byte) {
	var reflBytes, codegenBytes []byte

	reflMarshalErr := e.catchPanic(fmt.Sprintf("reflection-marshal[%s]", entry.Name), entry, origData, func() error {
		var err error
		reflBytes, err = e.marshalReflection(ds, reflTarget)
		return err
	})

	codegenMarshalErr := e.catchPanic(fmt.Sprintf("codegen-marshal[%s]", entry.Name), entry, origData, func() error {
		var err error
		codegenBytes, err = e.marshalCodegen(ds, codegenTarget)
		return err
	})

	if isPanicError(reflMarshalErr) || isPanicError(codegenMarshalErr) {
		return
	}

	if reflMarshalErr != nil || codegenMarshalErr != nil {
		if (reflMarshalErr == nil) != (codegenMarshalErr == nil) {
			e.stats.MarshalMismatches.Add(1)
			e.reporter.Report(Issue{
				Type:     IssueMarshalMismatch,
				TypeName: entry.Name,
				Data:     origData,
				Details: fmt.Sprintf(
					"reflection marshal err: %v, codegen marshal err: %v",
					reflMarshalErr, codegenMarshalErr,
				),
			})
		}
		return
	}

	if !bytes.Equal(reflBytes, codegenBytes) {
		e.stats.MarshalMismatches.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueMarshalMismatch,
			TypeName: entry.Name,
			Data:     origData,
			Details: fmt.Sprintf(
				"marshal output differs: reflection=%d bytes, codegen=%d bytes",
				len(reflBytes), len(codegenBytes),
			),
			ReflectionOutput: reflBytes,
			CodegenOutput:    codegenBytes,
		})
	}
}

func (e *Engine) compareHTR(entry corpus.TypeEntry, ds *dynssz.DynSsz, reflTarget, codegenTarget any, origData []byte) {
	var reflRoot, codegenRoot [32]byte

	reflHTRErr := e.catchPanic(fmt.Sprintf("reflection-htr[%s]", entry.Name), entry, origData, func() error {
		var err error
		reflRoot, err = e.htrReflection(ds, reflTarget)
		return err
	})

	codegenHTRErr := e.catchPanic(fmt.Sprintf("codegen-htr[%s]", entry.Name), entry, origData, func() error {
		var err error
		codegenRoot, err = e.htrCodegen(ds, codegenTarget)
		return err
	})

	if isPanicError(reflHTRErr) || isPanicError(codegenHTRErr) {
		return
	}

	if reflHTRErr != nil || codegenHTRErr != nil {
		if (reflHTRErr == nil) != (codegenHTRErr == nil) {
			e.stats.HTRMismatches.Add(1)
			e.reporter.Report(Issue{
				Type:     IssueHTRMismatch,
				TypeName: entry.Name,
				Data:     origData,
				Details: fmt.Sprintf(
					"reflection HTR err: %v, codegen HTR err: %v",
					reflHTRErr, codegenHTRErr,
				),
			})
		}
		return
	}

	if reflRoot != codegenRoot {
		e.stats.HTRMismatches.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueHTRMismatch,
			TypeName: entry.Name,
			Data:     origData,
			Details: fmt.Sprintf(
				"HTR differs: reflection=%x, codegen=%x",
				reflRoot, codegenRoot,
			),
		})
	}
}

func (e *Engine) compareStreaming(entry corpus.TypeEntry, ds *dynssz.DynSsz, target any, origData []byte) {
	// Marshal via buffer
	var bufBytes []byte
	bufErr := e.catchPanic(fmt.Sprintf("buffer-marshal[%s]", entry.Name), entry, origData, func() error {
		var err error
		bufBytes, err = ds.MarshalSSZ(target)
		return err
	})

	if isPanicError(bufErr) || bufErr != nil {
		return
	}

	// Marshal via streaming
	var streamBuf bytes.Buffer
	streamErr := e.catchPanic(fmt.Sprintf("stream-marshal[%s]", entry.Name), entry, origData, func() error {
		return ds.MarshalSSZWriter(target, &streamBuf)
	})

	if isPanicError(streamErr) {
		return
	}

	if streamErr != nil {
		e.stats.StreamMismatches.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueStreamMismatch,
			TypeName: entry.Name,
			Data:     origData,
			Details:  fmt.Sprintf("buffer marshal succeeded but stream marshal failed: %v", streamErr),
		})
		return
	}

	if !bytes.Equal(bufBytes, streamBuf.Bytes()) {
		e.stats.StreamMismatches.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueStreamMismatch,
			TypeName: entry.Name,
			Data:     origData,
			Details: fmt.Sprintf(
				"stream marshal output differs from buffer: buffer=%d bytes, stream=%d bytes",
				len(bufBytes), len(streamBuf.Bytes()),
			),
			ReflectionOutput: bufBytes,
			CodegenOutput:    streamBuf.Bytes(),
		})
		return
	}

	// Unmarshal via streaming
	streamTarget := entry.New()
	streamUnmarshalErr := e.catchPanic(fmt.Sprintf("stream-unmarshal[%s]", entry.Name), entry, origData, func() error {
		reader := bytes.NewReader(bufBytes)
		return ds.UnmarshalSSZReader(streamTarget, reader, len(bufBytes))
	})

	if isPanicError(streamUnmarshalErr) {
		return
	}

	if streamUnmarshalErr != nil {
		e.stats.StreamMismatches.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueStreamMismatch,
			TypeName: entry.Name,
			Data:     origData,
			Details:  fmt.Sprintf("buffer unmarshal succeeded but stream unmarshal failed: %v", streamUnmarshalErr),
		})
		return
	}

	// Re-marshal the stream-unmarshaled value and compare
	var streamRemarshal []byte
	remarshalErr := e.catchPanic(fmt.Sprintf("stream-remarshal[%s]", entry.Name), entry, origData, func() error {
		var err error
		streamRemarshal, err = ds.MarshalSSZ(streamTarget)
		return err
	})

	if isPanicError(remarshalErr) || remarshalErr != nil {
		return
	}

	if !bytes.Equal(bufBytes, streamRemarshal) {
		e.stats.StreamMismatches.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueStreamMismatch,
			TypeName: entry.Name,
			Data:     origData,
			Details: fmt.Sprintf(
				"stream round-trip mismatch: original=%d bytes, after stream=%d bytes",
				len(bufBytes), len(streamRemarshal),
			),
		})
	}
}

func (e *Engine) testRoundTrip(entry corpus.TypeEntry, ds *dynssz.DynSsz, target any, origData []byte) {
	var marshaledBytes []byte
	marshalErr := e.catchPanic(fmt.Sprintf("roundtrip-marshal[%s]", entry.Name), entry, origData, func() error {
		var err error
		marshaledBytes, err = ds.MarshalSSZ(target)
		return err
	})

	if isPanicError(marshalErr) || marshalErr != nil {
		return
	}

	roundTripTarget := entry.New()
	unmarshalErr := e.catchPanic(fmt.Sprintf("roundtrip-unmarshal[%s]", entry.Name), entry, origData, func() error {
		return ds.UnmarshalSSZ(roundTripTarget, marshaledBytes)
	})

	if isPanicError(unmarshalErr) || unmarshalErr != nil {
		return
	}

	var remarshaledBytes []byte
	remarshalErr := e.catchPanic(fmt.Sprintf("roundtrip-remarshal[%s]", entry.Name), entry, origData, func() error {
		var err error
		remarshaledBytes, err = ds.MarshalSSZ(roundTripTarget)
		return err
	})

	if isPanicError(remarshalErr) || remarshalErr != nil {
		return
	}

	if !bytes.Equal(marshaledBytes, remarshaledBytes) {
		e.stats.MarshalMismatches.Add(1)
		e.reporter.Report(Issue{
			Type:     IssueMarshalMismatch,
			TypeName: entry.Name,
			Data:     origData,
			Details: fmt.Sprintf(
				"round-trip marshal mismatch: first=%d bytes, second=%d bytes",
				len(marshaledBytes), len(remarshaledBytes),
			),
		})
	}
}

// unmarshalReflection performs unmarshal using only the reflection engine,
// bypassing any codegen-generated methods.
// Note: we intentionally skip the "full consumption" check here because the
// codegen UnmarshalSSZDyn also does not verify trailing bytes. This makes
// the comparison fair. Round-trip tests catch real data corruption.
func (e *Engine) unmarshalReflection(ds *dynssz.DynSsz, target any, data []byte) error {
	targetType := reflect.TypeOf(target)
	targetValue := reflect.ValueOf(target)

	typeDesc, err := ds.GetTypeCache().GetTypeDescriptor(targetType, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("get type descriptor: %w", err)
	}

	ctx := reflection.NewReflectionCtx(ds, nil, false, true)

	decoder := sszutils.NewBufferDecoder(data)
	decoder.PushLimit(len(data))

	if err := ctx.UnmarshalSSZ(typeDesc, targetValue, decoder); err != nil {
		return err
	}

	decoder.PopLimit()

	return nil
}

// unmarshalCodegen performs unmarshal using the codegen-generated methods.
func (e *Engine) unmarshalCodegen(ds *dynssz.DynSsz, target any, data []byte) error {
	if um, ok := target.(sszutils.DynamicUnmarshaler); ok {
		return um.UnmarshalSSZDyn(ds, data)
	}
	return fmt.Errorf("type does not implement DynamicUnmarshaler")
}

// marshalReflection performs marshal using only the reflection engine.
func (e *Engine) marshalReflection(ds *dynssz.DynSsz, source any) ([]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	typeDesc, err := ds.GetTypeCache().GetTypeDescriptor(sourceType, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get type descriptor: %w", err)
	}

	ctx := reflection.NewReflectionCtx(ds, nil, false, true)

	size, err := ctx.SizeSSZ(typeDesc, sourceValue)
	if err != nil {
		return nil, fmt.Errorf("size: %w", err)
	}

	buf := make([]byte, 0, size)
	encoder := sszutils.NewBufferEncoder(buf)

	if err := ctx.MarshalSSZ(typeDesc, sourceValue, encoder); err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	return encoder.GetBuffer(), nil
}

// marshalCodegen performs marshal using the codegen-generated methods.
func (e *Engine) marshalCodegen(ds *dynssz.DynSsz, source any) ([]byte, error) {
	if m, ok := source.(sszutils.DynamicMarshaler); ok {
		buf := make([]byte, 0, 1024)
		return m.MarshalSSZDyn(ds, buf)
	}
	return nil, fmt.Errorf("type does not implement DynamicMarshaler")
}

// htrReflection computes hash tree root using only the reflection engine.
func (e *Engine) htrReflection(ds *dynssz.DynSsz, source any) ([32]byte, error) {
	sourceType := reflect.TypeOf(source)
	sourceValue := reflect.ValueOf(source)

	typeDesc, err := ds.GetTypeCache().GetTypeDescriptor(sourceType, nil, nil, nil)
	if err != nil {
		return [32]byte{}, fmt.Errorf("get type descriptor: %w", err)
	}

	ctx := reflection.NewReflectionCtx(ds, nil, false, true)

	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)

	if err := ctx.HashTreeRoot(typeDesc, sourceValue, hh); err != nil {
		return [32]byte{}, err
	}

	return hh.HashRoot()
}

// htrCodegen computes hash tree root using the codegen-generated methods.
func (e *Engine) htrCodegen(ds *dynssz.DynSsz, source any) ([32]byte, error) {
	if h, ok := source.(sszutils.DynamicHashRoot); ok {
		hh := hasher.FastHasherPool.Get()
		defer hasher.FastHasherPool.Put(hh)

		if err := h.HashTreeRootWithDyn(ds, hh); err != nil {
			return [32]byte{}, err
		}

		return hh.HashRoot()
	}
	return [32]byte{}, fmt.Errorf("type does not implement DynamicHashRoot")
}

type panicError struct {
	value any
}

func (e *panicError) Error() string {
	return fmt.Sprintf("panic: %v", e.value)
}

func isPanicError(err error) bool {
	_, ok := err.(*panicError)
	return ok
}

func (e *Engine) catchPanic(context string, entry corpus.TypeEntry, data []byte, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			e.stats.Panics.Add(1)
			e.reporter.Report(Issue{
				Type:     IssuePanic,
				TypeName: entry.Name,
				Data:     data,
				Details:  fmt.Sprintf("panic in %s: %v", context, r),
			})
			err = &panicError{value: r}
		}
	}()

	return fn()
}

func boolStr(b bool, trueStr, falseStr string) string {
	if b {
		return trueStr
	}
	return falseStr
}

// PrintStats prints current fuzzing statistics to stdout.
func PrintStats(stats *Stats, elapsed time.Duration) {
	iters := stats.Iterations.Load()
	rate := float64(0)
	if elapsed.Seconds() > 0 {
		rate = float64(iters) / elapsed.Seconds()
	}

	fmt.Printf(
		"\r[%s] iters: %d (%.0f/s) | valid: %d mutated: %d random: %d | "+
			"ok: %d panic: %d marshal: %d htr: %d stream: %d unmarshal: %d",
		elapsed.Truncate(time.Second),
		iters, rate,
		stats.ValidFills.Load(),
		stats.MutatedFills.Load(),
		stats.RandomInputs.Load(),
		stats.Successes.Load(),
		stats.Panics.Load(),
		stats.MarshalMismatches.Load(),
		stats.HTRMismatches.Load(),
		stats.StreamMismatches.Load(),
		stats.UnmarshalDiffs.Load(),
	)
}
