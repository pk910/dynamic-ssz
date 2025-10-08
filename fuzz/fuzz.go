// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package fuzz

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"time"

	dynssz "github.com/pk910/dynamic-ssz"
)

// Fuzzer provides fuzzing capabilities for dynssz marshal/unmarshal operations
type Fuzzer struct {
	r           *rand.Rand
	failureProb float64 // probability of generating edge case values
}

// NewFuzzer creates a new fuzzer with optional seed
func NewFuzzer(seed int64) *Fuzzer {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &Fuzzer{
		r:           rand.New(rand.NewSource(seed)),
		failureProb: 0.1, // 10% chance of edge cases
	}
}

// SetFailureProbability sets the probability of generating edge case values
func (f *Fuzzer) SetFailureProbability(prob float64) {
	f.failureProb = prob
}

// FuzzValue fills the given value with random data according to its type
func (f *Fuzzer) FuzzValue(v interface{}) {
	f.doFuzz(reflect.ValueOf(v), 0)
}

// FuzzMarshalUnmarshal tests marshal/unmarshal roundtrip for a value
func (f *Fuzzer) FuzzMarshalUnmarshal(original interface{}) error {
	ds := dynssz.NewDynSsz(nil)

	// Marshal the original value
	marshaled, err := ds.MarshalSSZ(original)
	if err != nil {
		fmt.Printf("marshal err: %v\n", err)
		return err
	}

	// Create a new instance of the same type for unmarshaling
	originalType := reflect.TypeOf(original)
	if originalType.Kind() == reflect.Ptr {
		originalType = originalType.Elem()
	}

	unmarshaled := reflect.New(originalType).Interface()

	// Unmarshal into the new instance
	err = ds.UnmarshalSSZ(unmarshaled, marshaled)
	if err != nil {
		return err
	}

	remarshaled, err := ds.MarshalSSZ(unmarshaled)
	if err != nil {
		return err
	}

	if !bytes.Equal(marshaled, remarshaled) {
		return fmt.Errorf("Marshal mismatch: %x != %x", marshaled, remarshaled)
	}

	return nil
}

// FuzzSize tests size calculation for a value
func (f *Fuzzer) FuzzSize(v interface{}) error {
	ds := dynssz.NewDynSsz(nil)
	_, err := ds.SizeSSZ(v)
	return err
}

// FuzzHashTreeRoot tests hash tree root calculation for a value
func (f *Fuzzer) FuzzHashTreeRoot(v interface{}) error {
	ds := dynssz.NewDynSsz(nil)
	_, err := ds.HashTreeRoot(v)
	return err
}

// TagHints contains parsed SSZ tag information
type TagHints struct {
	MaxSize uint64
	Size    uint64
	HasMax  bool
	HasSize bool
}

// doFuzz recursively fills a value with random data
func (f *Fuzzer) doFuzz(v reflect.Value, depth int) {
	f.doFuzzWithTags(v, depth, []TagHints{})
}

// doFuzzWithTags recursively fills a value with random data, with tag hints for SSZ constraints
func (f *Fuzzer) doFuzzWithTags(v reflect.Value, depth int, tagHints []TagHints) {
	if depth > 10 { // Prevent infinite recursion
		return
	}

	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(f.r.Intn(2) == 1)

	case reflect.Uint8:
		v.SetUint(uint64(f.getRandomUint8()))

	case reflect.Uint16:
		v.SetUint(uint64(f.getRandomUint16()))

	case reflect.Uint32:
		v.SetUint(uint64(f.getRandomUint32()))

	case reflect.Uint64:
		v.SetUint(f.getRandomUint64())

	case reflect.String:
		v.SetString(f.getRandomString())

	case reflect.Slice:
		f.fuzzSlice(v, depth, tagHints)

	case reflect.Array:
		f.fuzzArray(v, depth, tagHints)

	case reflect.Struct:
		f.fuzzStruct(v, depth)

	case reflect.Ptr:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		f.doFuzzWithTags(v.Elem(), depth+1, tagHints)

	case reflect.Interface:
		if !v.IsNil() {
			f.doFuzzWithTags(v.Elem(), depth+1, tagHints)
		}
	}
}

// fuzzSlice fills a slice with random data
func (f *Fuzzer) fuzzSlice(v reflect.Value, depth int, tagHints []TagHints) {
	elementType := v.Type().Elem()

	// Get max length from tag hints (consume first hint)
	maxLen := f.getMaxLengthFromHints(tagHints, v.Type())
	remainingHints := f.consumeTagHint(tagHints)

	// Create new slice with random length
	length := f.r.Intn(maxLen + 1)
	newSlice := reflect.MakeSlice(v.Type(), length, length)

	// Fill each element
	for i := 0; i < length; i++ {
		element := newSlice.Index(i)
		if elementType.Kind() == reflect.Uint8 {
			// Special handling for byte slices
			element.SetUint(uint64(f.getRandomUint8()))
		} else {
			f.doFuzzWithTags(element, depth+1, remainingHints)
		}
	}

	v.Set(newSlice)
}

// fuzzArray fills an array with random data
func (f *Fuzzer) fuzzArray(v reflect.Value, depth int, tagHints []TagHints) {
	remainingHints := f.consumeTagHint(tagHints)
	for i := 0; i < v.Len(); i++ {
		f.doFuzzWithTags(v.Index(i), depth+1, remainingHints)
	}
}

// fuzzStruct fills a struct with random data
func (f *Fuzzer) fuzzStruct(v reflect.Value, depth int) {
	structType := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}

		structField := structType.Field(i)

		// Parse SSZ tags for this field
		tagHints := f.parseFieldTags(&structField)

		f.doFuzzWithTags(field, depth+1, tagHints)
	}
}

// parseFieldTags parses SSZ tags from a struct field and returns tag hints
func (f *Fuzzer) parseFieldTags(field *reflect.StructField) []TagHints {
	var hints []TagHints

	// Parse ssz-max tag first
	if sszMaxStr, hasSszMax := field.Tag.Lookup("ssz-max"); hasSszMax {
		for _, maxStr := range strings.Split(sszMaxStr, ",") {
			maxStr = strings.TrimSpace(maxStr)
			if maxStr == "?" {
				hints = append(hints, TagHints{})
				continue
			}
			if maxSize, err := strconv.ParseUint(maxStr, 10, 64); err == nil {
				hints = append(hints, TagHints{
					MaxSize: maxSize,
					HasMax:  true,
				})
			}
		}
	}

	// Parse ssz-size tag for fixed sizes
	if sszSizeStr, hasSszSize := field.Tag.Lookup("ssz-size"); hasSszSize {
		idx := 0
		for _, sizeStr := range strings.Split(sszSizeStr, ",") {
			sizeStr = strings.TrimSpace(sizeStr)
			if sizeStr == "?" {
				if idx < len(hints) {
					// Keep existing hint
				} else {
					hints = append(hints, TagHints{})
				}
				idx++
				continue
			}
			if size, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
				if idx < len(hints) {
					hints[idx].Size = size
					hints[idx].HasSize = true
				} else {
					hints = append(hints, TagHints{
						Size:    size,
						HasSize: true,
					})
				}
			}
			idx++
		}
	}

	return hints
}

// consumeTagHint removes and returns the first tag hint from the slice
func (f *Fuzzer) consumeTagHint(tagHints []TagHints) []TagHints {
	if len(tagHints) <= 1 {
		return []TagHints{}
	}
	return tagHints[1:]
}

// getMaxLengthFromHints gets the maximum length from tag hints or defaults
func (f *Fuzzer) getMaxLengthFromHints(tagHints []TagHints, sliceType reflect.Type) int {
	if len(tagHints) > 0 {
		hint := tagHints[0]

		// Use fixed size if available
		if hint.HasSize {
			return int(hint.Size)
		}

		// Use max size if available
		if hint.HasMax {
			maxLen := int(hint.MaxSize)
			if maxLen > 10000 {
				maxLen = 10000 // Cap for fuzzing performance
			}
			return maxLen
		}
	}

	// Default to reasonable limits for fuzzing based on element type
	switch sliceType.Elem().Kind() {
	case reflect.Uint8:
		return 1024 // byte slices
	default:
		return 100 // other slice types
	}
}

// Random value generators with edge cases
func (f *Fuzzer) getRandomUint8() uint8 {
	if f.shouldGenerateEdgeCase() {
		switch f.r.Intn(3) {
		case 0:
			return 0
		case 1:
			return 255
		default:
			return uint8(f.r.Intn(256))
		}
	}
	return uint8(f.r.Intn(256))
}

func (f *Fuzzer) getRandomUint16() uint16 {
	if f.shouldGenerateEdgeCase() {
		switch f.r.Intn(3) {
		case 0:
			return 0
		case 1:
			return 65535
		default:
			return uint16(f.r.Intn(65536))
		}
	}
	return uint16(f.r.Intn(65536))
}

func (f *Fuzzer) getRandomUint32() uint32 {
	if f.shouldGenerateEdgeCase() {
		switch f.r.Intn(3) {
		case 0:
			return 0
		case 1:
			return 4294967295
		default:
			return f.r.Uint32()
		}
	}
	return f.r.Uint32()
}

func (f *Fuzzer) getRandomUint64() uint64 {
	if f.shouldGenerateEdgeCase() {
		switch f.r.Intn(3) {
		case 0:
			return 0
		case 1:
			return 18446744073709551615
		default:
			return f.r.Uint64()
		}
	}
	return f.r.Uint64()
}

func (f *Fuzzer) getRandomString() string {
	if f.shouldGenerateEdgeCase() {
		switch f.r.Intn(4) {
		case 0:
			return ""
		case 1:
			return "\x00"
		case 2:
			return "🚀" // Unicode
		default:
			return f.generateRandomString()
		}
	}
	return f.generateRandomString()
}

func (f *Fuzzer) generateRandomString() string {
	length := f.r.Intn(50)
	bytes := make([]byte, length)
	for i := range bytes {
		bytes[i] = byte(f.r.Intn(256))
	}
	return string(bytes)
}

func (f *Fuzzer) shouldGenerateEdgeCase() bool {
	return f.r.Float64() < f.failureProb
}
