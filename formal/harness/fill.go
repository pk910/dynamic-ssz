package harness

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// Fill populates the Go value pointed to by target from a ssz-specs fixture
// "value" JSON document. The mapping follows the fixture conventions:
//   - bool           <- JSON bool
//   - uintN          <- JSON number or decimal string
//   - uint128/256    <- decimal string (little-endian byte array in Go)
//   - byte vectors   <- "0x..." hex string
//   - bitvectors     <- {"data": [bools]} packed LSB-first
//   - bitlists       <- {"data": [bools]} packed LSB-first + delimiter bit
//   - vectors/lists  <- {"data": [...]} (or bare array)
//   - containers     <- JSON object keyed by python field names (json tags)
//   - unions         <- {"selector": n, "data": ...}
func Fill(target any, raw json.RawMessage) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer {
		return fmt.Errorf("target must be a pointer")
	}
	return fillValue(v.Elem(), "", raw)
}

type descriptorProvider interface {
	GetDescriptorType() reflect.Type
}

func fillValue(v reflect.Value, tag reflect.StructTag, raw json.RawMessage) error {
	t := v.Type()

	// Merge type-level annotations (field tags take precedence: listed first).
	if at, ok := sszutils.LookupAnnotation(t); ok {
		tag = reflect.StructTag(strings.TrimSpace(string(tag) + " " + at))
	}

	switch tag.Get("ssz-type") {
	case "bitvector":
		return fillBits(v, raw, false)
	case "bitlist", "progressive-bitlist":
		return fillBits(v, raw, true)
	case "uint128", "uint256":
		return fillBigUint(v, raw)
	}

	// Union types (CompatibleUnion / Union) expose GetDescriptorType and have
	// a Variant field.
	if t.Kind() == reflect.Struct && v.CanAddr() {
		if dp, ok := v.Addr().Interface().(descriptorProvider); ok {
			if _, hasVariant := t.FieldByName("Variant"); hasVariant {
				return fillUnion(v, dp.GetDescriptorType(), raw)
			}
		}
	}

	switch t.Kind() {
	case reflect.Bool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return err
		}
		v.SetBool(b)
		return nil

	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
		n, err := parseUint(raw)
		if err != nil {
			return err
		}
		v.SetUint(n)
		return nil

	case reflect.Array, reflect.Slice:
		// Byte blobs are hex strings (possibly wrapped in {"data": ...}).
		if t.Elem().Kind() == reflect.Uint8 {
			if inner, err := unwrapData(raw); err == nil && bytes.HasPrefix(inner, []byte(`"`)) {
				return fillHexBytes(v, inner)
			}
		}
		elems, err := unwrapArray(raw)
		if err != nil {
			return err
		}
		if t.Kind() == reflect.Array {
			if len(elems) != t.Len() {
				return fmt.Errorf("array length mismatch: json %d, go %d", len(elems), t.Len())
			}
		} else {
			v.Set(reflect.MakeSlice(t, len(elems), len(elems)))
		}
		elemTag := shiftDims(tag)
		for i, e := range elems {
			if err := fillValue(v.Index(i), elemTag, e); err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
		}
		return nil

	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := f.Tag.Get("json")
			if name == "" {
				name = strings.ToLower(f.Name)
			}
			fraw, ok := fields[name]
			if !ok {
				continue // leave zero value
			}
			if err := fillValue(v.Field(i), f.Tag, fraw); err != nil {
				return fmt.Errorf("field %s: %w", f.Name, err)
			}
		}
		return nil

	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(t.Elem()))
		}
		return fillValue(v.Elem(), tag, raw)
	}

	return fmt.Errorf("unsupported kind %s", t.Kind())
}

func fillUnion(v reflect.Value, descriptor reflect.Type, raw json.RawMessage) error {
	var u struct {
		Selector uint8           `json:"selector"`
		Data     json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return err
	}
	// Find the descriptor field whose selector matches. Selectors are the
	// ssz-index tag when present, else 1-based field position.
	var variantField *reflect.StructField
	for i := 0; i < descriptor.NumField(); i++ {
		f := descriptor.Field(i)
		sel := uint64(i + 1)
		if idx := f.Tag.Get("ssz-index"); idx != "" {
			var err error
			sel, err = strconv.ParseUint(idx, 10, 8)
			if err != nil {
				return fmt.Errorf("bad ssz-index on %s: %w", f.Name, err)
			}
		}
		if uint8(sel) == u.Selector {
			variantField = &f
			break
		}
	}
	if variantField == nil {
		return fmt.Errorf("no descriptor variant for selector %d", u.Selector)
	}
	dataVal := reflect.New(variantField.Type)
	if err := fillValue(dataVal.Elem(), variantField.Tag, u.Data); err != nil {
		return fmt.Errorf("union data (selector %d): %w", u.Selector, err)
	}
	v.FieldByName("Variant").SetUint(uint64(u.Selector))
	v.FieldByName("Data").Set(reflect.ValueOf(dataVal.Elem().Interface()))
	return nil
}

// fillBits packs {"data": [bools]} LSB-first into a byte array/slice.
// For (progressive) bitlists a delimiter bit is appended after the last data bit.
func fillBits(v reflect.Value, raw json.RawMessage, withDelimiter bool) error {
	inner, err := unwrapData(raw)
	if err != nil {
		return err
	}
	var bools []bool
	if err := json.Unmarshal(inner, &bools); err != nil {
		return err
	}
	nbits := len(bools)
	nbytes := nbits / 8
	if withDelimiter {
		nbytes = (nbits + 1 + 7) / 8
	} else if nbits%8 != 0 {
		nbytes++
	}
	buf := make([]byte, nbytes)
	for i, b := range bools {
		if b {
			buf[i/8] |= 1 << (i % 8)
		}
	}
	if withDelimiter {
		buf[nbits/8] |= 1 << (nbits % 8)
	}
	switch v.Kind() {
	case reflect.Slice:
		v.SetBytes(buf)
	case reflect.Array:
		if len(buf) > v.Len() {
			return fmt.Errorf("bitvector bytes %d exceed array length %d", len(buf), v.Len())
		}
		for i, b := range buf {
			v.Index(i).SetUint(uint64(b))
		}
	default:
		return fmt.Errorf("bitfield target must be byte slice/array, got %s", v.Kind())
	}
	return nil
}

func fillBigUint(v reflect.Value, raw json.RawMessage) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		// May also be a plain number.
		s = string(bytes.TrimSpace(raw))
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return fmt.Errorf("invalid big uint %q", s)
	}
	if v.Kind() != reflect.Array || v.Type().Elem().Kind() != reflect.Uint8 {
		return fmt.Errorf("big uint target must be byte array, got %s", v.Type())
	}
	be := make([]byte, v.Len())
	n.FillBytes(be)
	// big.Int gives big-endian; SSZ wants little-endian.
	for i := 0; i < v.Len(); i++ {
		v.Index(i).SetUint(uint64(be[v.Len()-1-i]))
	}
	return nil
}

func fillHexBytes(v reflect.Value, raw json.RawMessage) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return err
	}
	switch v.Kind() {
	case reflect.Slice:
		v.SetBytes(b)
	case reflect.Array:
		if len(b) != v.Len() {
			return fmt.Errorf("byte vector length mismatch: json %d, go %d", len(b), v.Len())
		}
		for i, bb := range b {
			v.Index(i).SetUint(uint64(bb))
		}
	}
	return nil
}

func parseUint(raw json.RawMessage) (uint64, error) {
	s := string(bytes.TrimSpace(raw))
	s = strings.Trim(s, `"`)
	return strconv.ParseUint(s, 10, 64)
}

// unwrapData returns the "data" member if raw is an object containing one,
// else raw itself.
func unwrapData(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return nil, err
		}
		if d, ok := obj["data"]; ok {
			return d, nil
		}
		return nil, fmt.Errorf("object without data member")
	}
	return trimmed, nil
}

func unwrapArray(raw json.RawMessage) ([]json.RawMessage, error) {
	inner, err := unwrapData(raw)
	if err != nil {
		return nil, err
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(inner, &elems); err != nil {
		return nil, err
	}
	return elems, nil
}

// shiftDims drops the first dimension of comma-separated size annotations so
// element values are filled with the remaining dimensions. ssz-type and
// ssz-index never apply to elements.
func shiftDims(tag reflect.StructTag) reflect.StructTag {
	var parts []string
	for _, key := range []string{"ssz-size", "ssz-max", "dynssz-size", "dynssz-max"} {
		val, ok := tag.Lookup(key)
		if !ok {
			continue
		}
		dims := strings.SplitN(val, ",", 2)
		if len(dims) == 2 {
			parts = append(parts, fmt.Sprintf("%s:%q", key, dims[1]))
		}
	}
	return reflect.StructTag(strings.Join(parts, " "))
}
