// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"math/bits"

	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// ProofSchedule describes which subtrees of a value's Merkle tree must keep
// their structure for a set of proof paths. It is derived from the value's
// type descriptor and the requested generalized indices: a child appears in
// the schedule when a proof path descends to or below it, and retainAll
// marks subtrees whose internal tree shape depends on runtime data
// (progressive trees, unions, optionals, delegated types), so they are kept
// whole.
type ProofSchedule struct {
	children  map[uint64]*ProofSchedule
	retainAll bool
}

var retainAllSchedule = &ProofSchedule{retainAll: true}

// child returns the schedule for the child at the given ordinal, nil when
// the child's subtree carries no proof path and can collapse to its root.
// Schedules with retainAll never reach ordinal lookups: their whole subtree
// is captured through the tree wrapper instead.
func (s *ProofSchedule) child(ordinal uint64) *ProofSchedule {
	return s.children[ordinal]
}

// addChild returns the child schedule at the given ordinal, creating it if
// needed.
func (s *ProofSchedule) addChild(ordinal uint64) *ProofSchedule {
	if s.children == nil {
		s.children = make(map[uint64]*ProofSchedule, 1)
	}
	child := s.children[ordinal]
	if child == nil {
		child = &ProofSchedule{}
		s.children[ordinal] = child
	}
	return child
}

// BuildProofSchedule derives the retention schedule for proving the given
// generalized indices against a value of the given type. Chunk-level and
// interior targets are recorded as leaf-slot retentions so the pruned
// assembly keeps every node on their paths; shapes whose tree layout depends
// on runtime data are marked to be retained whole.
func BuildProofSchedule(desc *ssztypes.TypeDescriptor, gindices []int) (*ProofSchedule, error) {
	root := &ProofSchedule{}
	for _, gindex := range gindices {
		if gindex < 1 {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidValueRange, "invalid generalized index %d (must be >= 1)", gindex)
		}
		scheduleTarget(root, desc, uint64(gindex))
	}
	return root, nil
}

// scheduleTarget records the retention a single relative generalized index
// needs inside the subtree of the given type. A path ending at an interior
// node retains the leftmost leaf slot below it, which forces the pruned
// assembly to build every node on the way down.
func scheduleTarget(sched *ProofSchedule, desc *ssztypes.TypeDescriptor, rel uint64) {
	for {
		if rel == 1 || sched.retainAll {
			return
		}

		switch desc.SszType {
		case ssztypes.SszTypeWrapperType:
			// Wrappers are transparent to the walker's scope stream.
			desc = desc.ElemDesc
			continue
		case ssztypes.SszContainerType:
			fields := desc.ContainerDesc.Fields
			slot, ok := consumePath(&rel, chunkDepthSlots(uint64(len(fields))))
			child := sched.addChild(slot)
			if !ok || slot >= uint64(len(fields)) {
				// Interior node or zero padding: the retained slot pins the
				// path, nothing below it has structure.
				return
			}
			sched = child
			desc = fields[slot].Type
			continue
		case ssztypes.SszVectorType:
			if chunks, packed := staticChunkCount(desc); packed {
				slot, _ := consumePath(&rel, chunkDepthSlots(chunks))
				sched.addChild(slot)
				return
			}
			slot, ok := consumePath(&rel, chunkDepthSlots(uint64(desc.Len)))
			child := sched.addChild(slot)
			if !ok {
				return
			}
			sched = child
			desc = desc.ElemDesc
			continue
		case ssztypes.SszListType:
			if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
				// The tree depth follows the runtime length.
				sched.retainAll = true
				return
			}
			// Mixin level: left is the chunk tree, right the length chunk.
			// The mixin structure always exists in the pruned assembly.
			side, ok := consumePath(&rel, 1)
			if !ok || side != 0 {
				return
			}
			if limitChunks, packed := listLimitChunks(desc); packed {
				slot, _ := consumePath(&rel, chunkDepthSlots(limitChunks))
				sched.addChild(slot)
				return
			}
			slot, ok := consumePath(&rel, chunkDepthSlots(desc.Limit))
			child := sched.addChild(slot)
			if !ok {
				return
			}
			sched = child
			desc = desc.ElemDesc
			continue
		case ssztypes.SszBitvectorType:
			slot, _ := consumePath(&rel, chunkDepthSlots((uint64(desc.Len)+31)/32))
			sched.addChild(slot)
			return
		case ssztypes.SszBoolType, ssztypes.SszUint8Type, ssztypes.SszUint16Type,
			ssztypes.SszUint32Type, ssztypes.SszUint64Type, ssztypes.SszUint128Type, ssztypes.SszUint256Type,
			ssztypes.SszInt8Type, ssztypes.SszInt16Type, ssztypes.SszInt32Type, ssztypes.SszInt64Type,
			ssztypes.SszFloat32Type, ssztypes.SszFloat64Type:
			// Single-chunk leaves carry no structure below them.
			return
		default:
			// Progressive containers and lists, bitlists, unions, optionals,
			// custom and delegated types: the tree layout is not derivable
			// from the descriptor alone, keep the whole subtree.
			sched.retainAll = true
			return
		}
	}
}

// consumePath consumes depth bits of the relative generalized index and
// returns the child slot they select. ok is false when the index ends at an
// interior node of this range; the returned slot is then the leftmost leaf
// slot below that node.
func consumePath(rel *uint64, depth int) (uint64, bool) {
	length := bits.Len64(*rel) - 1
	if length < depth {
		slot := (*rel - 1<<length) << (depth - length)
		*rel = 1
		return slot, false
	}
	slot := (*rel >> (length - depth)) - 1<<depth
	*rel = 1<<(length-depth) | *rel&(1<<(length-depth)-1)
	return slot, true
}

// chunkDepthSlots returns the depth of a chunk tree padded to the given slot
// count (the next power of two, as merkleization pads).
func chunkDepthSlots(count uint64) int {
	if count <= 1 {
		return 0
	}
	return bits.Len64(count - 1)
}

// staticChunkCount reports the chunk count of a vector's packed data, with
// packed false when the elements are composite and merkleize to one chunk
// slot each. Byte data always reports through its uint8 element type.
func staticChunkCount(desc *ssztypes.TypeDescriptor) (uint64, bool) {
	if itemSize := packedElemSize(desc); itemSize > 0 {
		return (uint64(desc.Len)*itemSize + 31) / 32, true
	}
	return 0, false
}

// listLimitChunks reports the chunk-tree limit of a list's packed data, with
// packed false when the elements are composite. Byte data always reports
// through its uint8 element type.
func listLimitChunks(desc *ssztypes.TypeDescriptor) (uint64, bool) {
	if itemSize := packedElemSize(desc); itemSize > 0 {
		return sszutils.CalculateLimit(desc.Limit, 0, itemSize), true
	}
	return 0, false
}

// packedElemSize reports the packed byte size of a list's or vector's
// element type, unwrapping type wrappers; 0 means the elements are composite
// and merkleize to one chunk slot each.
func packedElemSize(desc *ssztypes.TypeDescriptor) uint64 {
	elemDesc := desc.ElemDesc
	for elemDesc.SszType == ssztypes.SszTypeWrapperType && elemDesc.ElemDesc != nil {
		elemDesc = elemDesc.ElemDesc
	}
	switch elemDesc.SszType {
	case ssztypes.SszBoolType, ssztypes.SszUint8Type, ssztypes.SszInt8Type:
		return 1
	case ssztypes.SszUint16Type, ssztypes.SszInt16Type:
		return 2
	case ssztypes.SszUint32Type, ssztypes.SszInt32Type, ssztypes.SszFloat32Type:
		return 4
	case ssztypes.SszUint64Type, ssztypes.SszInt64Type, ssztypes.SszFloat64Type:
		return 8
	case ssztypes.SszUint128Type:
		return 16
	case ssztypes.SszUint256Type:
		return 32
	default:
		return 0
	}
}
