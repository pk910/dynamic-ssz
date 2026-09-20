// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sizeguard

import (
	"bytes"
	"errors"
	"io"
	"math"
	"math/big"
	"reflect"
	"testing"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// codegen's TestSizeFormingEmitterSites counts the emitter sites this table
// stands for, so a new one cannot be added without a row reaching it.

// sizeGuardOutcome is one cell.
type sizeGuardOutcome struct {
	sentinel error  // the condition the path must report
	reason   string // why it reports nothing, or why the cell does not run
	skip     bool
}

// reports states that the path refuses, with the condition it names.
func reports(sentinel error) sizeGuardOutcome { return sizeGuardOutcome{sentinel: sentinel} }

// accepts states that the path returns no error, and why that is right.
func accepts(reason string) sizeGuardOutcome { return sizeGuardOutcome{reason: reason} }

// notRun states that driving the cell would materialise the encoding the
// construct describes, which is larger than the address space it runs in.
func notRun(reason string) sizeGuardOutcome {
	return sizeGuardOutcome{reason: reason, skip: true}
}

type sizeGuardPath struct {
	name string
	call func(ds *dynssz.DynSsz, v any) error
}

// The paths are reached through DynSsz so each one dispatches the way a caller
// reaches it, and through the encoder interface where DynSsz has no entry.
var sizeGuardPaths = []sizeGuardPath{
	{"SizeSSZ", func(ds *dynssz.DynSsz, v any) error { _, err := ds.SizeSSZ(v); return err }},
	{"MarshalSSZ", func(ds *dynssz.DynSsz, v any) error { _, err := ds.MarshalSSZ(v); return err }},
	{"MarshalSSZTo", func(ds *dynssz.DynSsz, v any) error { _, err := ds.MarshalSSZTo(v, nil); return err }},
	{"MarshalSSZEncoder", func(ds *dynssz.DynSsz, v any) error {
		m, ok := v.(sszutils.DynamicEncoder)
		if !ok {
			return errNoGeneratedMethod
		}
		return m.MarshalSSZEncoder(ds, sszutils.NewBufferEncoder(nil))
	}},
	{"MarshalSSZWriter", func(ds *dynssz.DynSsz, v any) error { return ds.MarshalSSZWriter(v, io.Discard) }},
	{"UnmarshalSSZ", func(ds *dynssz.DynSsz, v any) error {
		return ds.UnmarshalSSZ(reflect.New(reflect.TypeOf(v).Elem()).Interface(), nil)
	}},
	{"UnmarshalSSZReader", func(ds *dynssz.DynSsz, v any) error {
		return ds.UnmarshalSSZReader(reflect.New(reflect.TypeOf(v).Elem()).Interface(), bytes.NewReader(nil), 0)
	}},
	{"HashTreeRoot", func(ds *dynssz.DynSsz, v any) error { _, err := ds.HashTreeRoot(v); return err }},
}

var errNoGeneratedMethod = errors.New("the type has no generated method for this path")

type sizeGuardConstruct struct {
	name    string
	emitter string // the generator helper that bounds this construct
	value   func() any
	specs   map[string]any
	cells   map[string]sizeGuardOutcome // where the platform's int is the wider bound
	narrow  map[string]sizeGuardOutcome // overrides where it is the narrower one
}

// Three gibibytes fits the SSZ size limit and not a 32-bit int, so the first
// three constructs carry information only where the int is the narrower bound.
// On a wider one their encodings are legal and the write paths would produce
// them, which is why those cells do not run.
var sizeGuardMatrix = []sizeGuardConstruct{
	{
		name:    "container static size",
		emitter: "platformGuard(staticSize)",
		value:   func() any { return &WideContainer{} },
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":            accepts("three gibibytes is a size this target states"),
			"MarshalSSZ":         notRun("would produce the three gibibytes it states"),
			"MarshalSSZTo":       notRun("would produce the three gibibytes it states"),
			"MarshalSSZEncoder":  notRun("would produce the three gibibytes it states"),
			"MarshalSSZWriter":   notRun("would produce the three gibibytes it states"),
			"UnmarshalSSZ":       reports(sszutils.ErrUnexpectedEOF),
			"UnmarshalSSZReader": reports(sszutils.ErrUnexpectedEOF),
			"HashTreeRoot":       notRun("would hash the three gibibytes it states"),
		},
		narrow: map[string]sizeGuardOutcome{
			"SizeSSZ":            reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":         reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":       reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZEncoder":  reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZWriter":   reports(sszutils.ErrPlatformOverflow),
			"UnmarshalSSZ":       reports(sszutils.ErrPlatformOverflow),
			"UnmarshalSSZReader": reports(sszutils.ErrPlatformOverflow),
			"HashTreeRoot":       reports(sszutils.ErrPlatformOverflow),
		},
	},
	{
		name:    "declared vector size",
		emitter: "platformGuard(declared)",
		value:   func() any { return &WideVectorElem{} },
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":            accepts("three gibibytes is a size this target states"),
			"MarshalSSZ":         notRun("would produce the three gibibytes it states"),
			"MarshalSSZTo":       notRun("would produce the three gibibytes it states"),
			"MarshalSSZEncoder":  notRun("would produce the three gibibytes it states"),
			"MarshalSSZWriter":   notRun("would produce the three gibibytes it states"),
			"UnmarshalSSZ":       reports(sszutils.ErrUnexpectedEOF),
			"UnmarshalSSZReader": reports(sszutils.ErrUnexpectedEOF),
			"HashTreeRoot":       notRun("would hash the three gibibytes it states"),
		},
		narrow: map[string]sizeGuardOutcome{
			// The sizer and the path that pre-sizes through it answer from an
			// int return that holds no room for which limit was passed; the
			// rest keep the figure and name the platform's range.
			"SizeSSZ":            reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":         reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":       reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZEncoder":  reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZWriter":   reports(sszutils.ErrPlatformOverflow),
			"UnmarshalSSZ":       reports(sszutils.ErrPlatformOverflow),
			"UnmarshalSSZReader": reports(sszutils.ErrPlatformOverflow),
			"HashTreeRoot":       reports(sszutils.ErrPlatformOverflow),
		},
	},
	{
		name:    "list element size",
		emitter: "platformGuard(elemSize)",
		value:   func() any { return &WideListHolder{L: make([]WideListElem, 1)} },
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":            accepts("three gibibytes is a size this target states"),
			"MarshalSSZ":         notRun("would produce the three gibibytes it states"),
			"MarshalSSZTo":       notRun("would produce the three gibibytes it states"),
			"MarshalSSZEncoder":  notRun("would produce the three gibibytes it states"),
			"MarshalSSZWriter":   notRun("would produce the three gibibytes it states"),
			"UnmarshalSSZ":       reports(sszutils.ErrUnexpectedEOF),
			"UnmarshalSSZReader": reports(sszutils.ErrUnexpectedEOF),
			"HashTreeRoot":       notRun("would hash the three gibibytes it states"),
		},
		narrow: map[string]sizeGuardOutcome{
			"SizeSSZ":            reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":         reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":       reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZEncoder":  reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZWriter":   reports(sszutils.ErrSszSizeExceeded),
			"UnmarshalSSZ":       reports(sszutils.ErrUnexpectedEOF),
			"UnmarshalSSZReader": reports(sszutils.ErrUnexpectedEOF),
			"HashTreeRoot":       reports(sszutils.ErrPlatformOverflow),
		},
	},
	{
		name:    "list count times element width",
		emitter: "emitSizeReturn / appendVectorLenBound",
		value:   func() any { return &GiBList{L: make([]GiBElem, 4)} },
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":            reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":         reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":       notRun("takes no size, so it would produce the four gibibytes"),
			"MarshalSSZEncoder":  notRun("takes its offsets from the bytes written, so it would produce them too"),
			"MarshalSSZWriter":   reports(sszutils.ErrSszSizeExceeded),
			"UnmarshalSSZ":       reports(sszutils.ErrUnexpectedEOF),
			"UnmarshalSSZReader": reports(sszutils.ErrUnexpectedEOF),
			"HashTreeRoot":       notRun("would hash the four gibibytes the elements state"),
		},
	},
	{
		name:    "size expression past the limit",
		emitter: "getVectorLenExprVar",
		value:   func() any { return &SpecVector{} },
		specs:   map[string]any{"VEC32_SIZE": uint64(sszutils.MaxSszSize/4) + 1},
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":            reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":         reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":       reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZEncoder":  reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZWriter":   reports(sszutils.ErrSszSizeExceeded),
			"UnmarshalSSZ":       reports(sszutils.ErrSszSizeExceeded),
			"UnmarshalSSZReader": reports(sszutils.ErrSszSizeExceeded),
			"HashTreeRoot":       reports(sszutils.ErrSszSizeExceeded),
		},
		narrow: map[string]sizeGuardOutcome{
			// SizeSSZ and the path that pre-sizes through it answer from a
			// sizer, whose int return carries no room for which limit was
			// passed, so both name the size limit where the paths that keep
			// the figure name the platform's range.
			"SizeSSZ":            reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":         reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":       reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZEncoder":  reports(sszutils.ErrPlatformOverflow),
			"MarshalSSZWriter":   reports(sszutils.ErrPlatformOverflow),
			"UnmarshalSSZ":       reports(sszutils.ErrPlatformOverflow),
			"UnmarshalSSZReader": reports(sszutils.ErrPlatformOverflow),
			"HashTreeRoot":       reports(sszutils.ErrPlatformOverflow),
		},
	},
	{
		name:    "size expression product",
		emitter: "appendSizeLimitCheck",
		value:   func() any { return &SpecProduct{} },
		specs:   map[string]any{"OUTER": uint64(65536), "INNER": uint64(65536)},
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":            reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":         reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":       reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZEncoder":  reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZWriter":   reports(sszutils.ErrSszSizeExceeded),
			"UnmarshalSSZ":       reports(sszutils.ErrSszSizeExceeded),
			"UnmarshalSSZReader": reports(sszutils.ErrSszSizeExceeded),
			"HashTreeRoot":       reports(sszutils.ErrSszSizeExceeded),
		},
	},
	{
		name:    "bigint payload limit",
		emitter: "bigIntLimit",
		value:   func() any { return &BigIntMax{B: *new(big.Int).SetUint64(1 << 60)} },
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":            accepts("the size of the payload the value holds is what a sizer answers; the limit bounds the encoding"),
			"MarshalSSZ":         reports(sszutils.ErrListTooBig),
			"MarshalSSZTo":       reports(sszutils.ErrListTooBig),
			"MarshalSSZEncoder":  reports(sszutils.ErrListTooBig),
			"MarshalSSZWriter":   reports(sszutils.ErrListTooBig),
			"UnmarshalSSZ":       reports(sszutils.ErrUnexpectedEOF),
			"UnmarshalSSZReader": reports(sszutils.ErrUnexpectedEOF),
			"HashTreeRoot":       reports(sszutils.ErrListTooBig),
		},
	},
	{
		name:    "size a delegate reports",
		emitter: "checkDelegatedSize / appendDelegatedSize",
		value:   func() any { return &NegSizeHolder{} },
		cells: map[string]sizeGuardOutcome{
			"SizeSSZ":           reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZ":        reports(sszutils.ErrSszSizeExceeded),
			"MarshalSSZTo":      accepts("takes no size, so a sizer that refuses one never reaches it"),
			"MarshalSSZEncoder": accepts("takes its offsets from the bytes written, not from the size"),
			"MarshalSSZWriter":  reports(sszutils.ErrSszSizeExceeded),
			// A sizer bounds an encoding. The decode paths take their extents
			// from the input, so they run out of it before any size is taken.
			"UnmarshalSSZ":       reports(sszutils.ErrUnexpectedEOF),
			"UnmarshalSSZReader": reports(sszutils.ErrUnexpectedEOF),
			"HashTreeRoot":       accepts("merkleization forms no byte size"),
		},
	},
}

// TestSizeGuardMatrix drives every cell. A construct with no cell for a path
// fails rather than passing silently, so adding a path to sizeGuardPaths forces
// a verdict for it on every construct.
func TestSizeGuardMatrix(t *testing.T) {
	if _, generated := any(&WideContainer{}).(sszutils.DynamicMarshaler); !generated {
		t.Skip("no generated code present")
	}

	narrowInt := math.MaxInt == math.MaxInt32

	for _, construct := range sizeGuardMatrix {
		t.Run(construct.name, func(t *testing.T) {
			for _, path := range sizeGuardPaths {
				cell, ok := construct.cells[path.name]
				if narrowInt {
					if override, has := construct.narrow[path.name]; has {
						cell, ok = override, true
					}
				}
				if !ok {
					t.Errorf("%s: no cell records what this path does with a %s past the limit",
						path.name, construct.name)
					continue
				}

				t.Run(path.name, func(t *testing.T) {
					if cell.skip {
						t.Skip(cell.reason)
					}

					var err error
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Fatalf("panicked instead of answering: %v", r)
							}
						}()
						err = path.call(dynssz.NewDynSsz(construct.specs), construct.value())
					}()

					if errors.Is(err, errNoGeneratedMethod) {
						err = nil
					}
					switch {
					case cell.sentinel == nil && err != nil:
						t.Errorf("err = %v, want no error: %s", err, cell.reason)
					case cell.sentinel != nil && err == nil:
						t.Errorf("accepted a %s past the limit, want %v", construct.name, cell.sentinel)
					case cell.sentinel != nil && !errors.Is(err, cell.sentinel):
						t.Errorf("err = %v, want %v (guarded by %s)", err, cell.sentinel, construct.emitter)
					}
				})
			}
		})
	}
}
