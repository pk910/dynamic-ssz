// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

//go:build amd64 || arm64

package tests

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// A Go array length past the SSZ size limit is refused by the type cache; the
// parser refuses the same type (see the generator's parser tests).
func TestTypeCacheRefusesHugeArrayLength(t *testing.T) {
	_, err := ssztypes.NewTypeCache(nil).GetTypeDescriptor(reflect.TypeFor[BadHugeArray](), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "SSZ size limit") {
		t.Fatalf("err = %v, want the SSZ size limit refusal", err)
	}
}
