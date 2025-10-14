package dynssz

import "reflect"

func getPtr(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v.Addr()
	}

	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)

	return ptr
}
