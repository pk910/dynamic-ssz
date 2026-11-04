// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// The validator ends a cycle on a child's static methods only where the
// emitters call them: not for a child with a spec expression in a build with
// dynamic expressions, whose static methods baked the tag values.
func TestValidateEmittableGraphStaticMethodsWithExpressions(t *testing.T) {
	build := func(flags ssztypes.SszTypeFlag) *ssztypes.TypeDescriptor {
		node := &ssztypes.TypeDescriptor{
			SszType:        ssztypes.SszContainerType,
			SszTypeFlags:   ssztypes.SszTypeFlagIsDynamic | flags,
			SszCompatFlags: ssztypes.SszCompatFlagFastsszBufferMarshaler | ssztypes.SszCompatFlagFastsszSizer | ssztypes.SszCompatFlagFastsszUnmarshaler | ssztypes.SszCompatFlagFastsszHashRoot,
		}
		kids := &ssztypes.TypeDescriptor{SszType: ssztypes.SszListType, SszTypeFlags: ssztypes.SszTypeFlagIsDynamic | flags, ElemDesc: node}
		node.ContainerDesc = &ssztypes.ContainerDescriptor{Fields: []ssztypes.FieldDescriptor{{Name: "Kids", Type: kids}}}
		return &ssztypes.TypeDescriptor{
			SszType:       ssztypes.SszContainerType,
			SszTypeFlags:  ssztypes.SszTypeFlagIsDynamic | flags,
			ContainerDesc: &ssztypes.ContainerDescriptor{Fields: []ssztypes.FieldDescriptor{{Name: "N", Type: node}}},
		}
	}

	if err := validateEmittableGraph(build(0), true, true); err != nil {
		t.Fatalf("static methods without expressions end the cycle: %v", err)
	}
	if err := validateEmittableGraph(build(ssztypes.SszTypeFlagHasMaxExpr), true, false); err != nil {
		t.Fatalf("static build calls the static methods whatever the expressions: %v", err)
	}
	err := validateEmittableGraph(build(ssztypes.SszTypeFlagHasMaxExpr), true, true)
	if err == nil || !strings.Contains(err.Error(), "only referenced inline") {
		t.Fatalf("static methods of a child with a limit expression are not called: err = %v, want the inline-cycle refusal", err)
	}
}
