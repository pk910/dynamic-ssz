// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"reflect"
)

func (d *DynSsz) checkDynamicMaxSize(targetType reflect.Type, sizeHints []sszMaxSizeHint) (bool, error) {
	hasSpecValue := false

	childSizeHints := []sszMaxSizeHint{}
	if len(sizeHints) > 1 {
		childSizeHints = sizeHints[1:]
	}

	// resolve pointers to value type
	if targetType.Kind() == reflect.Ptr {
		targetType = targetType.Elem()
	}

	// get size from cache if not influenced by a parent sizeHint
	d.typeDynMaxCacheMutex.RLock()
	if cachedDynMaxCheck := d.typeDynMaxCache[targetType]; cachedDynMaxCheck != nil {
		d.typeDynMaxCacheMutex.RUnlock()
		return *cachedDynMaxCheck, nil
	}
	d.typeDynMaxCacheMutex.RUnlock()

	switch targetType.Kind() {
	case reflect.Struct:
		for i := 0; i < targetType.NumField(); i++ {
			field := targetType.Field(i)
			sszMaxSizes, err := d.getSszMaxSizeTag(&field)
			if err != nil {
				return false, err
			}

			hasSpecVal, err := d.checkDynamicMaxSize(field.Type, sszMaxSizes)
			if err != nil {
				return false, err
			}
			if hasSpecVal {
				hasSpecValue = true
			}
		}
	case reflect.Array:
		fieldType := targetType.Elem()
		hasSpecVal, err := d.checkDynamicMaxSize(fieldType, childSizeHints)
		if err != nil {
			return false, err
		}
		if hasSpecVal {
			hasSpecValue = true
		}
	case reflect.Slice:
		fieldType := targetType.Elem()
		hasSpecVal, err := d.checkDynamicMaxSize(fieldType, childSizeHints)
		if err != nil {
			return false, err
		}
		if hasSpecVal {
			hasSpecValue = true
		}
	}

	if len(sizeHints) > 0 && sizeHints[0].specval {
		hasSpecValue = true
	}

	if len(sizeHints) == 0 {
		// cache check result if it's static maximum and not influenced by a parent maxSizeHint
		d.typeDynMaxCacheMutex.Lock()
		d.typeDynMaxCache[targetType] = &hasSpecValue
		d.typeDynMaxCacheMutex.Unlock()
	}

	return hasSpecValue, nil
}
