// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package tests

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"unsafe"

	dynssz "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// The generated methods are reached through interfaces, so this file compiles
// in a checkout without the generated fixtures; each case skips when the method
// it exercises is absent.
type (
	marshalSSZ   interface{ MarshalSSZ() ([]byte, error) }
	marshalSSZTo interface {
		MarshalSSZTo(buf []byte) ([]byte, error)
	}
	marshalSSZEncoder interface {
		MarshalSSZEncoder(ds sszutils.DynamicSpecs, enc sszutils.Encoder) error
	}
	unmarshalSSZ interface{ UnmarshalSSZ(buf []byte) error }
	hashTreeRoot interface{ HashTreeRoot() ([32]byte, error) }
	sizeSSZ      interface{ SizeSSZ() int }
)

// A declared size the target's int cannot hold is refused the same way by every
// generated path: with an error, not by exhausting the address space.
func TestGeneratedPathsRefuseSizesPastThePlatformRange(t *testing.T) {
	t.Parallel()

	if math.MaxInt > math.MaxInt32 {
		// The declaration fits here, so there is nothing to refuse; the check
		// under test only exists for a narrower int.
		t.Skip("requires a 32-bit platform")
	}

	value := any(&WideAggregate{})

	// Each case reports the error its method returned and whether the method is
	// there at all, so the table holds no test helpers.
	for _, tc := range []struct {
		name     string
		viaSizer bool
		call     func() (error, bool)
	}{
		// MarshalSSZ takes the size from the generated sizer, whose int return
		// holds no room for which of the two size conditions it refused on, so
		// it reports the size limit for both.
		{"MarshalSSZ", true, func() (error, bool) {
			v, ok := value.(marshalSSZ)
			if !ok {
				return nil, false
			}
			_, err := v.MarshalSSZ()

			return err, true
		}},
		{"MarshalSSZTo", false, func() (error, bool) {
			v, ok := value.(marshalSSZTo)
			if !ok {
				return nil, false
			}
			_, err := v.MarshalSSZTo(nil)

			return err, true
		}},
		{"MarshalSSZEncoder", false, func() (error, bool) {
			v, ok := value.(marshalSSZEncoder)
			if !ok {
				return nil, false
			}

			return v.MarshalSSZEncoder(dynssz.GetGlobalDynSsz(), sszutils.NewBufferEncoder(nil)), true
		}},
		{"UnmarshalSSZ", false, func() (error, bool) {
			v, ok := value.(unmarshalSSZ)
			if !ok {
				return nil, false
			}

			return v.UnmarshalSSZ(nil), true
		}},
		{"HashTreeRoot", false, func() (error, bool) {
			v, ok := value.(hashTreeRoot)
			if !ok {
				return nil, false
			}
			_, err := v.HashTreeRoot()

			return err, true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked instead of reporting the size: %v", r)
				}
			}()

			err, generated := tc.call()
			if !generated {
				t.Skip("generated methods are not present in this checkout")
			}
			if tc.viaSizer {
				if !errors.Is(err, sszutils.ErrSszSizeExceeded) {
					t.Errorf("err = %v, want the size refused", err)
				}
				return
			}
			if !errors.Is(err, sszutils.ErrPlatformOverflow) {
				t.Errorf("err = %v, want the platform range reported", err)
			}
		})
	}
}

// A size method answers for the value it is given: an empty list forms no
// product, so a width the target cannot hold does not make its size wrong.
func TestGeneratedSizeOfAnEmptyWideList(t *testing.T) {
	t.Parallel()

	value := any(&WideListHolder{})

	marshaller, ok := value.(marshalSSZ)
	sizer, sized := value.(sizeSSZ)
	if !ok || !sized {
		t.Skip("generated methods are not present in this checkout")
	}

	data, err := marshaller.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if size := sizer.SizeSSZ(); size != len(data) {
		t.Errorf("SizeSSZ = %d, marshalled %d bytes", size, len(data))
	}
}

// A list length times its element width can pass every size domain. The sizer
// sums into a wide accumulator and weighs the total once, where it narrows, so
// an unrepresentable total is reported as 0 rather than as the positive number
// the product wrapped into.
//
// The two widths cover the two targets. Three gibibytes is refused per element
// on a 32-bit target before any product is formed, so only a 64-bit one reaches
// the overflow; one gibibyte fits a 32-bit int comfortably, so there the
// product alone carries it past the domain.
func TestGeneratedSizeOfAWideListRefusesAnUnrepresentableTotal(t *testing.T) {
	t.Parallel()

	const listOffset = int64(4)

	for _, fixture := range []struct {
		name      string
		elemBytes int64
		build     func(count int) any
	}{
		{"3GiB elements", 3000000000, func(n int) any { return &WideListHolder{L: make([]WideListElem, n)} }},
		{"1GiB elements", 1073741824, func(n int) any { return &GiBList{L: make([]GiBElem, n)} }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			for _, count := range []int{0, 1, 2, 3} {
				t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
					t.Parallel()

					sizer, sized := fixture.build(count).(sizeSSZ)
					if !sized {
						t.Skip("generated methods are not present in this checkout")
					}

					want := listOffset + int64(count)*fixture.elemBytes
					got := sizer.SizeSSZ()

					if want > int64(sszutils.MaxSszSize) {
						// The size path has no error channel and refuses with
						// -1; zero is a size an empty value legitimately has.
						if got != -1 {
							t.Errorf("SizeSSZ = %d for a total of %d no size domain holds, want -1", got, want)
						}
						return
					}
					if int64(got) != want {
						t.Errorf("SizeSSZ = %d, want %d", got, want)
					}
				})
			}
		})
	}
}

// sliceHeader is the shape of a slice value, used to give one a length no
// allocation can reach. unsafe.Slice refuses such a length, and the sizer only
// reads the length: the elements are never addressed, and the element type
// holds no pointer, so nothing walks the array the length claims.
type sliceHeader struct {
	data unsafe.Pointer
	len  int
	cap  int
}

// A total that left the range it is summed in is refused, not narrowed and
// returned. The accumulator is an int64 and every bound is on its terms, so a
// product of an element count no allocation can reach wraps it negative; the
// terminal bound reads the accumulator unsigned, where a negative lies far
// past the limit.
func TestGeneratedSizerRefusesAWrappedTotal(t *testing.T) {
	t.Parallel()

	if math.MaxInt <= math.MaxInt32 {
		// The product is formed in an int64 from an int length, which cannot
		// leave the int64 range on a target of narrower ints.
		t.Skip("requires a 64-bit platform")
	}

	// BulkElems sums its first field as len(A) * 32.
	const count = math.MaxInt/32 + 1

	var elem [48]byte
	value := &BulkElems{}
	*(*sliceHeader)(unsafe.Pointer(&value.A)) = sliceHeader{
		data: unsafe.Pointer(&elem),
		len:  count,
		cap:  count,
	}

	sizer, generated := any(value).(sizeSSZ)
	if !generated {
		t.Skip("generated methods are not present in this checkout")
	}

	if got := sizer.SizeSSZ(); got != -1 {
		t.Errorf("SizeSSZ = %d for a total that wrapped, want -1", got)
	}
}
