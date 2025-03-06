package dynssz

import (
	"fmt"
	"reflect"
	"strconv"
)

func (d *DynSsz) getSszStableMaxTag(field *reflect.StructField) (uint64, error) {
	stableMax := uint64(0)

	if stableMaxStr, fieldHasSszStableMax := field.Tag.Lookup("ssz-stable-max"); fieldHasSszStableMax {

		stableMaxInt, err := strconv.ParseUint(stableMaxStr, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("error parsing ssz-stable-max tag for '%v' field: %v", field.Name, err)
		}
		stableMax = stableMaxInt
	}

	return stableMax, nil
}

func (d *DynSsz) getSszStableIndexTag(field *reflect.StructField) (*uint64, error) {
	var stableIndex *uint64

	if stableIndexStr, fieldFound := field.Tag.Lookup("ssz-stable-index"); fieldFound {
		stableIndexInt, err := strconv.ParseUint(stableIndexStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("error parsing ssz-stable-index tag for '%v' field: %v", field.Name, err)
		}
		stableIndex = &stableIndexInt
	} else if stableIndexStr, fieldFound := field.Tag.Lookup("ssz-index"); fieldFound {
		stableIndexInt, err := strconv.ParseUint(stableIndexStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("error parsing ssz-index tag for '%v' field: %v", field.Name, err)
		}
		stableIndex = &stableIndexInt
	}

	return stableIndex, nil
}
