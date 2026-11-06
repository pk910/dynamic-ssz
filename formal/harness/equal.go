package harness

import (
	"fmt"
	"reflect"
)

// EqualNormalized compares two Go values structurally, tolerating
// nil-vs-empty slices, pointer-vs-value indirection, and interface wrapping.
// Returns a description of the first difference, or "" when equal.
func EqualNormalized(a, b any) string {
	return eqValue(reflect.ValueOf(a), reflect.ValueOf(b), "$")
}

func eqValue(a, b reflect.Value, path string) string {
	a = deref(a)
	b = deref(b)
	if !a.IsValid() || !b.IsValid() {
		if a.IsValid() != b.IsValid() {
			return fmt.Sprintf("%s: validity mismatch (%v vs %v)", path, a.IsValid(), b.IsValid())
		}
		return ""
	}
	if a.Type() != b.Type() {
		return fmt.Sprintf("%s: type mismatch (%s vs %s)", path, a.Type(), b.Type())
	}
	switch a.Kind() {
	case reflect.Bool:
		if a.Bool() != b.Bool() {
			return fmt.Sprintf("%s: %v != %v", path, a.Bool(), b.Bool())
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if a.Uint() != b.Uint() {
			return fmt.Sprintf("%s: %d != %d", path, a.Uint(), b.Uint())
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a.Int() != b.Int() {
			return fmt.Sprintf("%s: %d != %d", path, a.Int(), b.Int())
		}
	case reflect.String:
		if a.String() != b.String() {
			return fmt.Sprintf("%s: %q != %q", path, a.String(), b.String())
		}
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s: length %d != %d", path, a.Len(), b.Len())
		}
		for i := 0; i < a.Len(); i++ {
			if d := eqValue(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i)); d != "" {
				return d
			}
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if !a.Type().Field(i).IsExported() {
				continue
			}
			if d := eqValue(a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name); d != "" {
				return d
			}
		}
	default:
		return fmt.Sprintf("%s: unsupported kind %s", path, a.Kind())
	}
	return ""
}

func deref(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}
