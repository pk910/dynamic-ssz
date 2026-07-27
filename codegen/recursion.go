// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// validateEmittableGraph checks that a descriptor graph can be emitted as
// terminating code. The generators inline the marshalling of children that do
// not delegate to their own SSZ methods, recursing through the descriptor
// graph at emission time — so every cycle must contain a member with generated
// or delegated methods (a type in the generation set always qualifies). A
// cycle consisting only of inline-emitted descriptors would recurse forever,
// so it is rejected with a pointer to the type that must be generated.
//
// A generation-set type delegates through its own generated methods either via
// the Dynamic* interface (default mode) or via the fastssz-style MarshalSSZTo /
// HashTreeRootWith methods (with-legacy or without-dynamic-expressions mode).
// A reference only terminates the recursion when the emitter actually delegates
// to a method rather than inlining the child:
//   - dynamicDelegation (!WithoutDynamicExpressions): the emitter calls the
//     Dynamic* methods. Under WithoutDynamicExpressions the emitter never calls a
//     *Dyn method — a dynamic-only child is inlined instead — so DynamicMarshaler
//     no longer terminates the cycle.
//   - staticDelegation (!NoFastSsz || WithoutDynamicExpressions): the emitter
//     calls the fastssz-style static methods (WithoutDynamicExpressions forces
//     the static path regardless of NoFastSsz).
func validateEmittableGraph(root *ssztypes.TypeDescriptor, staticDelegation, dynamicDelegation bool) error {
	const done = 2
	const onStack = 1
	state := map[*ssztypes.TypeDescriptor]int{}

	var visit func(desc *ssztypes.TypeDescriptor, isRoot bool) error
	visit = func(desc *ssztypes.TypeDescriptor, isRoot bool) error {
		if desc == nil || state[desc] == done {
			return nil
		}
		// A type in the generation set carries its own (future) methods as compat
		// flags, which exempts it from the type cache's zero-field container
		// validation for delegated shells — but its layout is generated from this
		// descriptor, so the SSZ rule still applies to it as a generation root.
		if isRoot && (desc.SszType == ssztypes.SszContainerType || desc.SszType == ssztypes.SszProgressiveContainerType) &&
			desc.ContainerDesc != nil && len(desc.ContainerDesc.Fields) == 0 {
			return fmt.Errorf("container type has no SSZ fields, which is invalid per the SSZ spec")
		}
		// Emission delegates non-root references with generated/delegated
		// methods instead of inlining them, which terminates the recursion. A
		// method only terminates the cycle when the emitter actually calls it:
		// Dynamic* methods when dynamicDelegation is set (never under
		// WithoutDynamicExpressions, where dynamic-only children are inlined),
		// fastssz-style methods when staticDelegation is set.
		if !isRoot {
			if dynamicDelegation && desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicMarshaler != 0 {
				return nil
			}
			if staticDelegation && desc.SszCompatFlags&ssztypes.SszCompatFlagFastSSZMarshaler != 0 {
				return nil
			}
		}
		if state[desc] == onStack {
			return fmt.Errorf("recursive type %s is only referenced inline; include it in the code generation set so its methods can be called instead", describeDescriptor(desc))
		}
		state[desc] = onStack
		defer func() { state[desc] = done }()

		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				if err := visit(desc.ContainerDesc.Fields[i].Type, false); err != nil {
					return err
				}
			}
		}
		if err := visit(desc.ElemDesc, false); err != nil {
			return err
		}
		for _, variant := range desc.UnionVariants {
			if err := visit(variant, false); err != nil {
				return err
			}
		}
		return nil
	}

	return visit(root, true)
}

// describeDescriptor returns a readable type name for error messages,
// tolerating descriptors that carry only compile-time type information.
func describeDescriptor(desc *ssztypes.TypeDescriptor) string {
	if desc.Type != nil {
		return desc.Type.String()
	}
	if desc.CodegenInfo != nil {
		if info, ok := (*desc.CodegenInfo).(*CodegenInfo); ok && info.Type != nil {
			return info.Type.String()
		}
	}
	return "<unnamed>"
}
