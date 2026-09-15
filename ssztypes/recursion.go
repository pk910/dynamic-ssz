// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// childDerivedFlags are the type flags a descriptor accumulates from its
// children during the build (see buildContainerDescriptor and the collection
// builders). They mark spec-dependence anywhere in a type's subtree and gate
// fastssz delegation, which bakes in spec-independent preset values.
const childDerivedFlags = SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr

// FixupRecursiveFlags re-derives the child-propagated type flags across a
// descriptor graph that contains at least one recursive cycle. It is shared by
// the reflection type cache and the code generator's parser, which build their
// descriptor graphs independently but propagate flags by the same rules.
//
// During a recursive build, a back-edge hands out a descriptor whose
// child-derived flags are not final yet, and descriptors built inside the
// cycle complete (and are cached) before the cycle head does — so they can
// permanently miss flags that only become visible once the head is finished.
// This pass runs after the whole graph is built and back-patched: it walks
// every reachable descriptor and ORs the child-derived flags upward until no
// flag changes. Flags only accumulate (the pass never clears a type flag), so
// own-hint flags are preserved, the fixpoint is unique regardless of
// iteration order, and re-running over already-exact subtrees is a no-op.
//
// When a flag is raised on a descriptor, the fastssz compatibility flags that
// were detected under the old (incomplete) flags are suppressed to match how
// detectCompatFlags gates them: fastssz marshalling is only valid without
// spec-dependent sizes, and fastssz hashing only without spec-dependent
// limits. Custom types keep their compat flags — they have no traversed
// children, so their flags can never change here anyway.
func FixupRecursiveFlags(root *TypeDescriptor) {
	markRecursionMembers(root)

	// Collect all reachable descriptors once; the visited set is keyed by
	// pointer identity so cycles terminate.
	nodes := []*TypeDescriptor{}
	visited := map[*TypeDescriptor]struct{}{}

	var collect func(desc *TypeDescriptor)
	collect = func(desc *TypeDescriptor) {
		if desc == nil {
			return
		}
		if _, seen := visited[desc]; seen {
			return
		}
		visited[desc] = struct{}{}
		nodes = append(nodes, desc)

		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				collect(desc.ContainerDesc.Fields[i].Type)
			}
		}
		collect(desc.ElemDesc)
		for _, variant := range desc.UnionVariants {
			collect(variant)
		}
	}
	collect(root)

	// Iterate to the fixpoint. Each sweep can only raise flags, and there are
	// at most 4 flags per node, so the loop terminates after at most 4*len+1
	// sweeps (in practice one or two).
	for changed := true; changed; {
		changed = false

		for _, desc := range nodes {
			var derived SszTypeFlag

			// Mirror the build-time propagation rules exactly: containers OR the
			// flags of every field, unions the flags of every variant,
			// single-element collections their element's flags. Large uints
			// propagate nothing from their children, and leaves have nothing
			// to derive.
			switch desc.SszType {
			case SszContainerType, SszProgressiveContainerType:
				if desc.ContainerDesc != nil {
					for i := range desc.ContainerDesc.Fields {
						if fieldDesc := desc.ContainerDesc.Fields[i].Type; fieldDesc != nil {
							derived |= fieldDesc.SszTypeFlags
						}
					}
				}
			case SszListType, SszBitlistType, SszProgressiveListType, SszProgressiveBitlistType,
				SszVectorType, SszBitvectorType, SszOptionalType, SszOptionalListType, SszTypeWrapperType:
				if desc.ElemDesc != nil {
					derived |= desc.ElemDesc.SszTypeFlags
				}
			case SszUnionType, SszCompatibleUnionType:
				for _, variantDesc := range desc.UnionVariants {
					if variantDesc != nil {
						derived |= variantDesc.SszTypeFlags
					}
				}
			default:
				// Primitives, large uints and custom types derive no flags from
				// children.
			}

			raised := (derived & childDerivedFlags) &^ desc.SszTypeFlags
			if raised == 0 {
				continue
			}

			// Custom types can never reach this point: they derive nothing in the
			// switch above, so their compat flags are preserved unconditionally.
			desc.SszTypeFlags |= raised
			changed = true

			if raised&SszTypeFlagHasDynamicSize != 0 {
				desc.SszCompatFlags &^= SszCompatFlagFastSSZMarshaler
			}
			if raised&SszTypeFlagHasDynamicMax != 0 {
				desc.SszCompatFlags &^= SszCompatFlagFastSSZHasher | SszCompatFlagHashTreeRootWith
				desc.HashTreeRootWithMethod = nil
			}
		}
	}
}

// markRecursionMembers flags every descriptor that lies on a recursive cycle
// with SszTypeFlagRecursionMember. The walkers and the code generator count one
// nesting level per flagged descriptor entered, so the flag defines what the
// nesting bound measures, and both engines mark from the same graph shape, so
// they count identically.
//
// Type wrappers are transparent: they attach tags to their element without
// adding a structural level, so they stay unflagged (the cycle they sit on is
// still counted through its other members, as a wrapper only wraps). Pointers
// never appear as separate descriptors (GoTypeFlagIsPointer folds them into
// their element), so a trip around a cycle costs exactly one level per
// structural member.
//
// A descriptor is on a cycle exactly when it belongs to a strongly connected
// component with more than one member, or has an edge to itself; the
// components are found with Tarjan's algorithm. Membership is a property of
// the descriptor graph alone, and a component lies within its members'
// reachable subgraph, which is complete once a member has been published, so
// the first build reaching a descriptor marks it and a later build finds the
// bit already set: a published descriptor is never written.
func markRecursionMembers(root *TypeDescriptor) {
	type node struct {
		index, lowlink int
		onStack        bool
	}
	nodes := map[*TypeDescriptor]*node{}
	var stack []*TypeDescriptor
	next := 0

	edges := func(desc *TypeDescriptor, visit func(child *TypeDescriptor)) {
		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				visit(desc.ContainerDesc.Fields[i].Type)
			}
		}
		visit(desc.ElemDesc)
		for _, variant := range desc.UnionVariants {
			visit(variant)
		}
	}

	var connect func(desc *TypeDescriptor)
	connect = func(desc *TypeDescriptor) {
		n := &node{index: next, lowlink: next, onStack: true}
		nodes[desc] = n
		next++
		stack = append(stack, desc)

		selfEdge := false
		edges(desc, func(child *TypeDescriptor) {
			if child == nil {
				return
			}
			if child == desc {
				selfEdge = true
				return
			}
			if childNode, seen := nodes[child]; !seen {
				connect(child)
				n.lowlink = min(n.lowlink, nodes[child].lowlink)
			} else if childNode.onStack {
				n.lowlink = min(n.lowlink, childNode.index)
			}
		})

		if n.lowlink != n.index {
			return
		}
		// desc is the root of a component: pop its members.
		var members []*TypeDescriptor
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			nodes[top].onStack = false
			members = append(members, top)
			if top == desc {
				break
			}
		}
		if len(members) == 1 && !selfEdge {
			return
		}
		for _, member := range members {
			if member.SszType != SszTypeWrapperType && member.SszTypeFlags&SszTypeFlagRecursionMember == 0 {
				member.SszTypeFlags |= SszTypeFlagRecursionMember
			}
		}
	}
	connect(root)
}

// marshalCyclicDescriptor serializes a descriptor graph that contains cycles
// into a deterministic byte representation for hashing. Standard JSON
// marshalling fails on cyclic graphs, so each reachable descriptor is assigned
// an index in a deterministic traversal order and serialized shallowly, with
// child descriptors written as index references. Field metadata that normally
// travels inside the nested JSON (names, indices) is emitted alongside the
// references; dynamic-field layout is fully derived from the fields and flags,
// so it needs no separate representation.
func marshalCyclicDescriptor(root *TypeDescriptor) []byte {
	index := map[*TypeDescriptor]int{}
	order := []*TypeDescriptor{}

	var walk func(desc *TypeDescriptor)
	walk = func(desc *TypeDescriptor) {
		if desc == nil {
			return
		}
		if _, seen := index[desc]; seen {
			return
		}
		index[desc] = len(order)
		order = append(order, desc)

		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				walk(desc.ContainerDesc.Fields[i].Type)
			}
		}
		walk(desc.ElemDesc)
		for _, key := range sortedVariantKeys(desc.UnionVariants) {
			walk(desc.UnionVariants[key])
		}
	}
	walk(root)

	ref := func(desc *TypeDescriptor) int {
		if desc == nil {
			return -1
		}
		return index[desc]
	}

	var buf bytes.Buffer
	for i, desc := range order {
		shallow := *desc
		shallow.ContainerDesc = nil
		shallow.UnionVariants = nil
		shallow.ElemDesc = nil
		nodeJSON, _ := json.Marshal(&shallow)

		fmt.Fprintf(&buf, "#%d:%s", i, nodeJSON)
		if desc.ContainerDesc != nil {
			for fi := range desc.ContainerDesc.Fields {
				field := &desc.ContainerDesc.Fields[fi]
				fmt.Fprintf(&buf, ";f:%s/%d/%d=%d", field.Name, field.SszIndex, field.FieldIndex, ref(field.Type))
			}
		}
		if desc.ElemDesc != nil {
			fmt.Fprintf(&buf, ";e=%d", ref(desc.ElemDesc))
		}
		for _, key := range sortedVariantKeys(desc.UnionVariants) {
			fmt.Fprintf(&buf, ";u:%d=%d", key, ref(desc.UnionVariants[key]))
		}
		buf.WriteByte('\n')
	}

	return buf.Bytes()
}

// sortedVariantKeys returns the union variant keys in ascending order so the
// traversal and serialization are deterministic across map iteration orders.
func sortedVariantKeys(variants map[uint8]*TypeDescriptor) []uint8 {
	if len(variants) == 0 {
		return nil
	}
	keys := make([]uint8, 0, len(variants))
	for key := range variants {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
