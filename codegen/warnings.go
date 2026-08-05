// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"sort"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

// collectWarnings reports non-fatal problems with a type about to be generated.
//
// Warnings are collected from the finished descriptor graph rather than raised
// while it is built, so they cover both ways a descriptor can be produced: the
// go/types parser and the shared reflect-driven type cache.
func (cg *CodeGenerator) collectWarnings(typeName string, root *ssztypes.TypeDescriptor) {
	seen := map[*ssztypes.TypeDescriptor]struct{}{}

	var walk func(desc *ssztypes.TypeDescriptor, path string)
	walk = func(desc *ssztypes.TypeDescriptor, path string) {
		if desc == nil {
			return
		}
		if _, visited := seen[desc]; visited {
			return
		}
		seen[desc] = struct{}{}

		// A limit is part of the type in SSZ: List[T, N] and Bitlist[N] need N
		// to merkleize, so a list without one has no spec-defined root. The
		// library hashes it anyway -- merkleizing to the chunks the value
		// occupies and mixing in the length, so the root at least identifies the
		// value -- but no other implementation will agree on it.
		//
		// A progressive list or bitlist is unbounded by design (EIP-7916) and is
		// not reported. Neither is a limit that only exists as an unresolved
		// expression, which is a limit the generator cannot see yet.
		//
		// Without extended types this shape is rejected outright while the hash
		// method is emitted, so in practice this warns about what extended types
		// let through.
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 && desc.MaxExpression == nil {
			switch desc.SszType {
			case ssztypes.SszListType:
				cg.warn("%s has no ssz-max, so its hash tree root is not standard SSZ: the length is mixed in so the root still identifies the value, but it will not match another implementation", path)
			case ssztypes.SszBitlistType:
				cg.warn("%s has no ssz-max, so its hash tree root uses a limit derived from the value rather than the type: it will not match another implementation", path)
			default:
			}
		}

		if desc.ContainerDesc != nil {
			for i := range desc.ContainerDesc.Fields {
				field := &desc.ContainerDesc.Fields[i]
				walk(field.Type, path+"."+field.Name)
			}
		}
		walk(desc.ElemDesc, path+"[]")
		for selector, variant := range desc.UnionVariants {
			walk(variant, fmt.Sprintf("%s(variant:%d)", path, selector))
		}
	}

	walk(root, typeName)
}

// warn records a non-fatal problem, ignoring one already reported: the same
// type is analyzed once per file that names it, and a shared field type once
// per referencing type.
func (cg *CodeGenerator) warn(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if cg.warnings == nil {
		cg.warnings = map[string]struct{}{}
	}
	cg.warnings[message] = struct{}{}
}

// Warnings returns the non-fatal problems found while generating, sorted for a
// stable order. Generation succeeds regardless; callers are expected to surface
// them so the author can act on them.
func (cg *CodeGenerator) Warnings() []string {
	out := make([]string, 0, len(cg.warnings))
	for message := range cg.warnings {
		out = append(out, message)
	}
	sort.Strings(out)

	return out
}
