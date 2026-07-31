// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"

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

// recursionCycleTypes returns the types that lie on a recursive cycle in the
// descriptor graph rooted at root, keyed as recursionTypeKey does.
//
// Only such a type can nest to a depth the input chooses, so only its generated
// methods need to carry and check a nesting depth. A type that merely contains
// a recursive type is not on a cycle: entering the cycle from outside starts a
// fresh count, and the outside of a cycle is finite by construction.
//
// This is computed here rather than while the descriptors are built because the
// two builders differ. The go/types parser could report cycles straight from
// its own recursion check, but descriptors built from reflect types come from
// the shared type cache, which has no such hook and no codegen state to record
// it in. Walking the finished graph covers both.
//
// Unlike validateEmittableGraph, this walk descends through descriptors that
// have their own methods: delegation is exactly how a legal cycle terminates,
// so stopping there would step over every cycle worth finding.
func recursionCycleTypes(root *ssztypes.TypeDescriptor) map[string]struct{} {
	cyclic := map[string]struct{}{}

	// A descriptor is on a cycle exactly when a walk from it can reach it
	// again, so a depth-first walk that remembers its current path collects
	// everything between a back edge's target and the node that closed it.
	var path []*ssztypes.TypeDescriptor
	onPath := map[*ssztypes.TypeDescriptor]int{}
	done := map[*ssztypes.TypeDescriptor]struct{}{}

	var visit func(desc *ssztypes.TypeDescriptor)
	visit = func(desc *ssztypes.TypeDescriptor) {
		if desc == nil {
			return
		}
		if start, cycling := onPath[desc]; cycling {
			for _, member := range path[start:] {
				if key := recursionTypeKey(member); key != "" {
					cyclic[key] = struct{}{}
				}
			}
			return
		}
		if _, settled := done[desc]; settled {
			return
		}

		onPath[desc] = len(path)
		path = append(path, desc)

		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				visit(desc.ContainerDesc.Fields[i].Type)
			}
		}
		visit(desc.ElemDesc)
		for _, variant := range desc.UnionVariants {
			visit(variant)
		}

		path = path[:len(path)-1]
		delete(onPath, desc)
		// Only a fully explored node that has left the path is settled; one
		// still on the path may yet be reached by a back edge.
		done[desc] = struct{}{}
	}
	visit(root)

	return cyclic
}

// recursionTypeKey identifies a descriptor by the Go type whose generated
// methods would carry the depth, with any pointer stripped.
//
// A pointer and its pointee are separate descriptors sharing one method set,
// and a cycle runs through whichever the graph happens to link. Keying by the
// stripped type marks both, so the type the methods are emitted for is never
// the one left out. The reflect type is preferred where present and the
// compile-time type used otherwise, matching describeDescriptor.
func recursionTypeKey(desc *ssztypes.TypeDescriptor) string {
	if desc == nil {
		return ""
	}

	if desc.Type != nil {
		t := desc.Type
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		return t.String()
	}

	if desc.CodegenInfo != nil {
		if info, ok := (*desc.CodegenInfo).(*CodegenInfo); ok && info.Type != nil {
			t := types.Unalias(info.Type)
			if ptr, isPtr := t.(*types.Pointer); isPtr {
				t = types.Unalias(ptr.Elem())
			}
			return t.String()
		}
	}

	return ""
}

// defaultRecursionDepth bounds how many times a recursive type may re-enter
// itself in generated code.
//
// 1024 is far deeper than any practical schema nests (Ethereum consensus types
// stay under 20) while costing well under a megabyte of stack. See
// WithRecursionDepth for why the bound exists at all.
const defaultRecursionDepth = 1024

// recursionBound carries what the emitters need to bound a recursive type: the
// types that lie on a cycle, and how deep those may nest.
type recursionBound struct {
	cycles   map[string]struct{}
	maxDepth int
}

// newRecursionBound resolves the bound for the descriptor graph rooted at root.
func newRecursionBound(root *ssztypes.TypeDescriptor, opts *CodeGeneratorOptions) *recursionBound {
	maxDepth := opts.RecursionDepth
	if maxDepth <= 0 {
		maxDepth = defaultRecursionDepth
	}

	return &recursionBound{cycles: recursionCycleTypes(root), maxDepth: maxDepth}
}

// applies reports whether desc's type lies on a recursive cycle, and therefore
// whether its generated methods carry a nesting depth.
func (b *recursionBound) applies(desc *ssztypes.TypeDescriptor) bool {
	if b == nil || len(b.cycles) == 0 {
		return false
	}
	key := recursionTypeKey(desc)
	if key == "" {
		return false
	}
	_, cyclic := b.cycles[key]

	return cyclic
}

// depthParam is the name of the nesting-depth argument threaded through the
// generated methods of a recursive type.
const depthParam = "depth"

// emitMethodHeader writes the opening of a generated method and returns the
// argument list a recursive self-call must pass.
//
// For an ordinary type it writes the method header unchanged. For a type on a
// recursive cycle it writes the public method as a one-line delegator entering
// at depth zero, then opens an unexported twin that carries the depth and
// checks it. The caller appends the body and the closing brace either way, so a
// call site only has to name its parameters.
//
// The twin exists because a caller emits a call to a child's *public* method
// without knowing how that child implements it, so the depth-carrying entry
// point needs a name derived from the public one rather than from whichever
// method happens to hold the body.
//
// params is the parameter list without the depth argument; args is the same
// list reduced to argument names, for the delegating call. failReturn is the
// return statement for a depth violation, which differs by method: a sizer has
// no error result and reports zero.
func emitMethodHeader(
	codeBuilder *strings.Builder,
	bound *recursionBound,
	desc *ssztypes.TypeDescriptor,
	typeName, fnName, params, args, results, failReturn string,
) {
	if !bound.applies(desc) {
		appendCode(codeBuilder, 0, "func (t %s) %s(%s) (%s) {\n", typeName, fnName, params, results)
		return
	}

	depthFn := depthMethodName(fnName)

	// An already-unexported method is an internal view method whose only caller
	// is the view dispatcher, and the dispatcher names the depth-carrying twin
	// directly. Emitting a public delegator for it would be dead code.
	if isExportedName(fnName) {
		callArgs := depthParam
		if args != "" {
			callArgs = args + ", " + depthParam
		}

		appendCode(codeBuilder, 0, "func (t %s) %s(%s) (%s) {\n", typeName, fnName, params, results)
		appendCode(codeBuilder, 1, "return t.%s(%s)\n", depthFn, strings.Replace(callArgs, depthParam, "0", 1))
		appendCode(codeBuilder, 0, "}\n\n")
	}

	depthParams := depthParam + " int"
	if params != "" {
		depthParams = params + ", " + depthParams
	}

	appendCode(codeBuilder, 0, "// %s carries the nesting depth for the recursive cycle %s lies on.\n", depthFn, typeName)
	appendCode(codeBuilder, 0, "func (t %s) %s(%s) (%s) {\n", typeName, depthFn, depthParams, results)
	appendCode(codeBuilder, 1, "if %s > %d {\n\treturn %s\n}\n", depthParam, bound.maxDepth, failReturn)
}

// isExportedName reports whether a generated method is part of the type's
// public surface, as opposed to an internal view method.
func isExportedName(fnName string) bool {
	return fnName != "" && strings.ToUpper(fnName[:1]) == fnName[:1]
}

// depthMethodName derives the unexported depth-carrying twin's name from the
// public method's, so a caller can name it knowing only the public method.
//
// A receiver-qualified reference (t.Method) keeps its receiver: only the method
// part is unexported.
func depthMethodName(fnName string) string {
	if dot := strings.LastIndex(fnName, "."); dot >= 0 {
		return fnName[:dot+1] + depthMethodName(fnName[dot+1:])
	}

	return strings.ToLower(fnName[:1]) + fnName[1:] + "AtDepth"
}

// depthForwardName returns the method a generated body should call when it
// forwards to another method on the same receiver: the depth-carrying twin when
// the emitting method has a depth to pass on, the public method otherwise.
func depthForwardName(fnName string, depthAware bool) string {
	if !depthAware {
		return fnName
	}

	return depthMethodName(fnName)
}

// depthForwardArg returns the trailing argument for such a call. Forwarding to
// a method on the same receiver stays at the same nesting level, so the depth
// is passed through unchanged; only descending into a child advances it.
func depthForwardArg(depthAware bool) string {
	if !depthAware {
		return ""
	}

	return ", " + depthParam
}

// descendCall names the child method to call, and the extra argument to pass,
// when an emitter delegates to a child's own methods.
//
// Delegation is how the emitter stops inlining, so it is also where a recursive
// cycle is traversed. When the emitting method carries a depth and the child
// lies on a cycle, the call advances it. Otherwise the child's public method is
// called and starts its own count, which is right: entering a cycle from
// outside is a fresh descent, and the outside of a cycle is finite.
func descendCall(depthAware bool, bound *recursionBound, desc *ssztypes.TypeDescriptor, fnName string) (string, string) {
	if !depthAware || !bound.applies(desc) {
		return fnName, ""
	}

	return depthMethodName(fnName), ", " + depthParam + "+1"
}

// depthFailErr is the return statement for a depth violation in a method whose
// only result is an error.
func depthFailErr(bound *recursionBound) string {
	return fmt.Sprintf("sszutils.ErrMaxDepthExceededFn(%d)", bound.maxDepth)
}

// depthFailNilErr is the same for a method that returns a buffer alongside the
// error; the buffer is reported as nil.
func depthFailNilErr(bound *recursionBound) string {
	return "nil, " + depthFailErr(bound)
}

// depthForwardArgBare is depthForwardArg for a call with no other arguments.
func depthForwardArgBare(depthAware bool) string {
	if !depthAware {
		return ""
	}

	return depthParam
}

// viewFnSignature describes the closure a view dispatcher returns, so the
// dispatcher can wrap it when the depth has to be supplied by the closure
// rather than by its caller.
type viewFnSignature struct {
	params  string
	results string
	args    string
}
