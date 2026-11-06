package harness

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/sszutils"
	"pgregory.net/rapid"
)

// RandomFill populates the value pointed to by target with a random valid
// instance drawn from the rapid source, respecting SSZ tags/annotations.
func RandomFill(t *rapid.T, target any) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer {
		return fmt.Errorf("target must be a pointer")
	}
	return randomValue(t, v.Elem(), "")
}

// progressiveCap bounds progressive collection sizes; large enough to open
// multiple spine levels (1, 5, 21, 85 chunk boundaries for uint64 elements).
const progressiveCap = 120

func randomValue(t *rapid.T, v reflect.Value, tag reflect.StructTag) error {
	typ := v.Type()
	if at, ok := sszutils.LookupAnnotation(typ); ok {
		tag = reflect.StructTag(strings.TrimSpace(string(tag) + " " + at))
	}

	switch tag.Get("ssz-type") {
	case "bitvector":
		bits := tagBits(tag, typ)
		return randomBits(t, v, bits, false)
	case "bitlist":
		max := firstDim(tag.Get("ssz-max"), 64)
		bits := rapid.IntRange(0, max).Draw(t, "bits")
		return randomBits(t, v, bits, true)
	case "progressive-bitlist":
		bits := rapid.IntRange(0, progressiveCap*8).Draw(t, "pbits")
		return randomBits(t, v, bits, true)
	case "uint128", "uint256":
		return randomBytes(t, v)
	}

	if typ.Kind() == reflect.Struct && v.CanAddr() {
		if dp, ok := v.Addr().Interface().(descriptorProvider); ok {
			if _, hasVariant := typ.FieldByName("Variant"); hasVariant {
				return randomUnion(t, v, dp.GetDescriptorType())
			}
		}
	}

	switch typ.Kind() {
	case reflect.Bool:
		v.SetBool(rapid.Bool().Draw(t, "b"))
		return nil
	case reflect.Uint8:
		v.SetUint(uint64(rapid.Uint8().Draw(t, "u8")))
		return nil
	case reflect.Uint16:
		v.SetUint(uint64(rapid.Uint16().Draw(t, "u16")))
		return nil
	case reflect.Uint32:
		v.SetUint(uint64(rapid.Uint32().Draw(t, "u32")))
		return nil
	case reflect.Uint64, reflect.Uint:
		v.SetUint(rapid.Uint64().Draw(t, "u64"))
		return nil
	case reflect.Array:
		if typ.Elem().Kind() == reflect.Uint8 {
			return randomBytes(t, v)
		}
		elemTag := shiftDims(tag)
		for i := 0; i < v.Len(); i++ {
			if err := randomValue(t, v.Index(i), elemTag); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice:
		var n int
		if sz := tag.Get("ssz-size"); sz != "" {
			// Fixed-size slice (SSZ vector semantics).
			n = firstDim(sz, 0)
		} else if tag.Get("ssz-type") == "progressive-list" {
			n = rapid.IntRange(0, progressiveCap).Draw(t, "plen")
		} else {
			max := firstDim(tag.Get("ssz-max"), 32)
			if max > 64 {
				max = 64 // keep test values small; limits stress-tested by fixtures
			}
			n = rapid.IntRange(0, max).Draw(t, "len")
		}
		v.Set(reflect.MakeSlice(typ, n, n))
		if typ.Elem().Kind() == reflect.Uint8 {
			for i := 0; i < n; i++ {
				v.Index(i).SetUint(uint64(rapid.Uint8().Draw(t, "byte")))
			}
			return nil
		}
		elemTag := shiftDims(tag)
		for i := 0; i < n; i++ {
			if err := randomValue(t, v.Index(i), elemTag); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			if !typ.Field(i).IsExported() {
				continue
			}
			if err := randomValue(t, v.Field(i), typ.Field(i).Tag); err != nil {
				return err
			}
		}
		return nil
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(typ.Elem()))
		}
		return randomValue(t, v.Elem(), tag)
	}
	return fmt.Errorf("unsupported kind %s", typ.Kind())
}

func randomUnion(t *rapid.T, v reflect.Value, descriptor reflect.Type) error {
	i := rapid.IntRange(0, descriptor.NumField()-1).Draw(t, "variant")
	f := descriptor.Field(i)
	sel := uint64(i + 1)
	if idx := f.Tag.Get("ssz-index"); idx != "" {
		sel, _ = strconv.ParseUint(idx, 10, 8)
	}
	dataVal := reflect.New(f.Type)
	if err := randomValue(t, dataVal.Elem(), f.Tag); err != nil {
		return err
	}
	v.FieldByName("Variant").SetUint(sel)
	v.FieldByName("Data").Set(reflect.ValueOf(dataVal.Elem().Interface()))
	return nil
}

func randomBits(t *rapid.T, v reflect.Value, bits int, withDelimiter bool) error {
	nbytes := bits / 8
	if withDelimiter {
		nbytes = (bits + 1 + 7) / 8
	} else if bits%8 != 0 {
		nbytes++
	}
	buf := make([]byte, nbytes)
	for i := 0; i < bits; i++ {
		if rapid.Bool().Draw(t, "bit") {
			buf[i/8] |= 1 << (i % 8)
		}
	}
	if withDelimiter {
		buf[bits/8] |= 1 << (bits % 8)
	}
	switch v.Kind() {
	case reflect.Slice:
		v.SetBytes(buf)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			b := byte(0)
			if i < len(buf) {
				b = buf[i]
			}
			v.Index(i).SetUint(uint64(b))
		}
	}
	return nil
}

func randomBytes(t *rapid.T, v reflect.Value) error {
	for i := 0; i < v.Len(); i++ {
		v.Index(i).SetUint(uint64(rapid.Uint8().Draw(t, "byte")))
	}
	return nil
}

func tagBits(tag reflect.StructTag, typ reflect.Type) int {
	if bs := tag.Get("ssz-bitsize"); bs != "" {
		n, _ := strconv.Atoi(strings.SplitN(bs, ",", 2)[0])
		return n
	}
	if typ.Kind() == reflect.Array {
		return typ.Len() * 8
	}
	return 8
}

func firstDim(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(strings.SplitN(s, ",", 2)[0])
	if err != nil {
		return def
	}
	return n
}
