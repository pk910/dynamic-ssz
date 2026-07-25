// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

// childDerivedFlags are the type flags a descriptor accumulates from its
// children during the build (see buildContainerDescriptor and the collection
// builders). They mark spec-dependence anywhere in a type's subtree and gate
// fastssz delegation, which bakes in spec-independent preset values.
const childDerivedFlags = SszTypeFlagHasDynamicSize | SszTypeFlagHasDynamicMax | SszTypeFlagHasSizeExpr | SszTypeFlagHasMaxExpr

// fixupRecursiveFlags re-derives the child-propagated type flags across a
// descriptor graph that contains at least one recursive cycle.
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
func fixupRecursiveFlags(root *TypeDescriptor) {
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
			// flags of every field, single-element collections OR their element's
			// flags. Unions and large uints propagate nothing from their children
			// (matching the builders), and leaves have nothing to derive.
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
			default:
				// Primitives, large uints, unions and custom types derive no flags
				// from children (matching the builders).
			}

			raised := (derived & childDerivedFlags) &^ desc.SszTypeFlags
			if raised == 0 {
				continue
			}

			desc.SszTypeFlags |= raised
			changed = true

			if desc.SszType == SszCustomType {
				continue
			}
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
