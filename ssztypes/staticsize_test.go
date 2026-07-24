// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"reflect"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

func TestJoinFieldAnnotationTag(t *testing.T) {
	if got := JoinFieldAnnotationTag(`ssz-max:"8"`, ""); got != `ssz-max:"8"` {
		t.Errorf("empty annotation: got %q", got)
	}
	if got := JoinFieldAnnotationTag("", `ssz-type:"custom"`); got != `ssz-type:"custom"` {
		t.Errorf("empty field tag: got %q", got)
	}
	if got := JoinFieldAnnotationTag(`ssz-max:"8"`, `ssz-type:"custom"`); got != `ssz-max:"8" ssz-type:"custom"` {
		t.Errorf("both tags: got %q", got)
	}
}

// staticDynSizer sizes itself through the dynssz sizer.
type staticDynSizer struct{}

func (staticDynSizer) SizeSSZDyn(sszutils.DynamicSpecs) int { return 8 }

// staticFastSizer sizes itself through the fastssz sizer only.
type staticFastSizer struct{}

func (staticFastSizer) SizeSSZ() int                          { return 16 }
func (staticFastSizer) MarshalSSZ() ([]byte, error)           { return nil, nil }
func (staticFastSizer) MarshalSSZTo(b []byte) ([]byte, error) { return b, nil }

// staticNoSizer provides no usable sizer at all.
type staticNoSizer struct{}

func TestDelegatedStaticSize(t *testing.T) {
	tc := NewTypeCache(nil)

	sz, err := tc.delegatedStaticSize(&TypeDescriptor{}, reflect.TypeOf(staticDynSizer{}))
	if err != nil || sz != 8 {
		t.Errorf("dynssz sizer: size=%d err=%v; want 8", sz, err)
	}

	sz, err = tc.delegatedStaticSize(&TypeDescriptor{}, reflect.TypeOf(staticFastSizer{}))
	if err != nil || sz != 16 {
		t.Errorf("fastssz sizer: size=%d err=%v; want 16", sz, err)
	}

	if _, err = tc.delegatedStaticSize(&TypeDescriptor{}, reflect.TypeOf(staticNoSizer{})); err == nil {
		t.Error("no sizer: expected error")
	}
}
