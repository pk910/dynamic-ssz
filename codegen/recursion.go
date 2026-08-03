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

// descriptorPkgPath returns the package path of the named Go type behind a
// descriptor, with any pointer stripped ("" for unnamed types). Generated
// depth-carrying methods are unexported, so a caller may only name them on
// types of the package being generated.
func descriptorPkgPath(desc *ssztypes.TypeDescriptor) string {
	if desc == nil {
		return ""
	}

	if desc.Type != nil {
		t := desc.Type
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		return t.PkgPath()
	}

	if desc.CodegenInfo != nil {
		if info, ok := (*desc.CodegenInfo).(*CodegenInfo); ok && info.Type != nil {
			t := types.Unalias(info.Type)
			if ptr, isPtr := t.(*types.Pointer); isPtr {
				t = types.Unalias(ptr.Elem())
			}
			if named, isNamed := t.(*types.Named); isNamed && named.Obj().Pkg() != nil {
				return named.Obj().Pkg().Path()
			}
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

// recursionBound carries what the emitters need to bound recursive nesting:
// which descriptors count a level, how deep the count may run, and which
// package's unexported depth methods are callable.
//
// The count is defined by SszTypeFlagRecursionMember — one level per flagged
// descriptor entered — which is the same rule the reflection walkers apply, so
// the engines accept and reject at identical nesting depths.
type recursionBound struct {
	maxDepth int

	// pkgPath is the package being generated. Depth-carrying methods are
	// unexported, so only types of this package can be entered without
	// restarting the count.
	pkgPath string

	// contains memoizes whether a descriptor's subtree holds a cycle member,
	// which is what forces a type's methods to carry the depth through.
	contains map[*ssztypes.TypeDescriptor]bool
}

// newRecursionBound resolves the bound for the descriptor graph rooted at root.
func newRecursionBound(root *ssztypes.TypeDescriptor, opts *CodeGeneratorOptions) *recursionBound {
	maxDepth := opts.RecursionDepth
	if maxDepth <= 0 {
		maxDepth = defaultRecursionDepth
	}

	return &recursionBound{
		maxDepth: maxDepth,
		pkgPath:  descriptorPkgPath(root),
		contains: map[*ssztypes.TypeDescriptor]bool{},
	}
}

// countsLevel reports whether entering desc advances the nesting depth: it
// does exactly when desc lies on a recursive cycle. Everything off a cycle
// bottoms out at a depth fixed by the type and is never charged.
func (b *recursionBound) countsLevel(desc *ssztypes.TypeDescriptor) bool {
	return b != nil && desc != nil && desc.SszTypeFlags&ssztypes.SszTypeFlagRecursionMember != 0
}

// threads reports whether desc's generated methods carry a nesting depth:
// when the type itself counts a level, or when a cycle member sits anywhere in
// its subtree — the depth must pass through unbroken, or a chain running
// through this type would restart its count where the reflection walk does not.
func (b *recursionBound) threads(desc *ssztypes.TypeDescriptor) bool {
	if b == nil || desc == nil {
		return false
	}
	if b.countsLevel(desc) {
		return true
	}
	if found, seen := b.contains[desc]; seen {
		return found
	}

	// An in-progress marker cannot be wrong: a walk can only revisit a node by
	// running a cycle, every non-wrapper cycle member is flagged and answered
	// above, and a cycle cannot consist of wrappers alone.
	b.contains[desc] = false

	found := false
	if desc.ContainerDesc != nil {
		for i := range desc.ContainerDesc.Fields {
			if b.threads(desc.ContainerDesc.Fields[i].Type) {
				found = true
				break
			}
		}
	}
	if !found && b.threads(desc.ElemDesc) {
		found = true
	}
	if !found {
		for _, variant := range desc.UnionVariants {
			if b.threads(variant) {
				found = true
				break
			}
		}
	}
	b.contains[desc] = found

	return found
}

// callableDepthMethods reports whether desc's depth-carrying methods can be
// named from the generated code: they are unexported, so only within the
// package being generated. A reference that crosses a package boundary must
// go through the public methods and restarts the count — a cycle never spans
// packages (that would be an import cycle), so each side stays independently
// bounded.
func (b *recursionBound) callableDepthMethods(desc *ssztypes.TypeDescriptor) bool {
	return b != nil && b.pkgPath != "" && descriptorPkgPath(desc) == b.pkgPath
}

// maxEmitNesting bounds how deep an emission walk may nest. No legal type
// nests anywhere near this deep at emission time — a walk that does has met a
// recursive cycle it failed to delegate, and would otherwise grow the output
// without end. Failing keeps a generator bug an error instead of an
// out-of-memory kill.
const maxEmitNesting = 4096

// errEmitNesting names the descriptor an emission walk was stuck on.
func errEmitNesting(typePrinter *TypePrinter, desc *ssztypes.TypeDescriptor) error {
	return fmt.Errorf("code emission nests deeper than %d levels at %s: a recursive cycle is being inlined instead of delegated; this is a bug in the code generator", maxEmitNesting, typePrinter.TypeString(desc))
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
//
// dsUnused declares that the body the caller is about to append never touches
// the ds parameter (it forwards to a spec-independent method). The parameter
// is then blanked on whichever function carries that body — the plain method,
// or the depth twin — while a delegator keeps it named, since forwarding is a
// use. The substitution lives here because only this function knows which of
// those shapes it is emitting.
func emitMethodHeader(
	codeBuilder *strings.Builder,
	bound *recursionBound,
	desc *ssztypes.TypeDescriptor,
	typeName, fnName, params, args, results, failReturn string,
	dsUnused bool,
) {
	bodyParams := params
	if dsUnused {
		bodyParams = strings.Replace(params, "ds ", "_ ", 1)
	}

	if !bound.threads(desc) {
		appendCode(codeBuilder, 0, "func (t %s) %s(%s) (%s) {\n", typeName, fnName, bodyParams, results)
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
	if bodyParams != "" {
		depthParams = bodyParams + ", " + depthParams
	}

	appendCode(codeBuilder, 0, "// %s carries the nesting depth for the recursive cycle %s lies on.\n", depthFn, typeName)
	appendCode(codeBuilder, 0, "func (t %s) %s(%s) (%s) {\n", typeName, depthFn, depthParams, results)
	// The received depth counts the cycle levels above this value, the entry
	// into a flagged type included (the caller advanced it). A type that only
	// passes the depth through adds no level, so it has nothing to check.
	if bound.countsLevel(desc) {
		appendCode(codeBuilder, 1, "if %s > %d {\n\treturn %s\n}\n", depthParam, bound.maxDepth, failReturn)
	}
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
// Delegation is how the emitter stops inlining, so it is also where the count
// crosses into another type's code. A child on a cycle is entered through its
// depth twin with the count advanced; a child that merely contains a cycle is
// entered through its twin with the count unchanged, so the depth threads
// through unbroken. Only when the twin cannot be named — the child belongs to
// another package, or the emitting method carries no depth — is the public
// method called, which starts a fresh count: a cycle never spans packages, so
// each side stays independently bounded.
func descendCall(depthAware bool, bound *recursionBound, desc *ssztypes.TypeDescriptor, fnName string) (string, string) {
	if !depthAware || !bound.threads(desc) || !bound.callableDepthMethods(desc) {
		return fnName, ""
	}
	if bound.countsLevel(desc) {
		return depthMethodName(fnName), ", " + depthParam + "+1"
	}

	return depthMethodName(fnName), ", " + depthParam
}

// emitInlineDepthCharge writes the depth advance and check for a cycle member
// whose structure is being emitted inline: the shadowed depth is scoped to the
// block the caller opened for the value, and the incremented value is checked
// exactly as a depth twin checks what it receives, so an inlined member costs
// the same level a generated one does. A no-op unless the emitting method
// carries a depth and the descriptor counts a level (roots are charged by
// their caller; views delegate).
func emitInlineDepthCharge(depthAware, isRoot, isView bool, bound *recursionBound, desc *ssztypes.TypeDescriptor, appendCode func(int, string, ...any), indent int, failReturn string) {
	if !depthAware || isRoot || isView || !bound.countsLevel(desc) {
		return
	}

	appendCode(indent, "depth := depth + 1\n")
	appendCode(indent, "if depth > %d {\n\treturn %s\n}\n", bound.maxDepth, failReturn)
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
