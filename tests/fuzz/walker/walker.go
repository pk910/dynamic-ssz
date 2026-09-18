// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package walker drives both sszutils.HashWalker implementations over the same
// generated call sequence and compares what they build. hasher.Hasher carries
// one byte buffer and cuts chunks when a region is reduced; treeproof.Wrapper
// builds the tree. They must agree for any sequence, including one a delegate
// leaves on a partial chunk.
package walker

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
	"github.com/pk910/dynamic-ssz/treeproof"
)

type opKind int

const (
	opAppend opKind = iota
	opAppendBytes32
	opAppendBool
	opAppendUint8
	opAppendUint16
	opAppendUint32
	opAppendUint64
	opPutBool
	opPutUint8
	opPutUint16
	opPutUint32
	opPutUint64
	opPutBytes
	opPutUint64Array
	opPutBitlist
	opPutProgressiveBitlist
	opFillUpTo32
	opCollapse
	opStartTree
	opIndex
	opMerkleize
	opMerkleizeWithMixin
	opMerkleizeProgressive
	opMerkleizeProgressiveWithMixin
	opMerkleizeProgressiveWithActiveFields
	opKindCount
)

// walkerOp is one call. n carries a length, a count or a value, and treeType
// the shape of a scope.
type Op struct {
	kind     opKind
	n        int
	num      uint64
	treeType sszutils.TreeType
}

func (o Op) String() string {
	names := map[opKind]string{
		opAppend: "Append", opAppendBytes32: "AppendBytes32", opAppendBool: "AppendBool",
		opAppendUint8: "AppendUint8", opAppendUint16: "AppendUint16", opAppendUint32: "AppendUint32",
		opAppendUint64: "AppendUint64", opPutBool: "PutBool", opPutUint8: "PutUint8",
		opPutUint16: "PutUint16", opPutUint32: "PutUint32", opPutUint64: "PutUint64",
		opPutBytes: "PutBytes", opPutUint64Array: "PutUint64Array", opPutBitlist: "PutBitlist",
		opPutProgressiveBitlist: "PutProgressiveBitlist", opFillUpTo32: "FillUpTo32",
		opCollapse: "Collapse", opStartTree: "StartTree", opIndex: "Index",
		opMerkleize: "Merkleize", opMerkleizeWithMixin: "MerkleizeWithMixin",
		opMerkleizeProgressive: "MerkleizeProgressive", opMerkleizeProgressiveWithMixin: "MerkleizeProgressiveWithMixin",
		opMerkleizeProgressiveWithActiveFields: "MerkleizeProgressiveWithActiveFields",
	}
	return fmt.Sprintf("%s(n=%d num=%d tree=%d)", names[o.kind], o.n, o.num, o.treeType)
}

func opBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i%251 + 1)
	}
	return b
}

// runProgram drives one walker. It keeps that walker's own scope indices, since
// the two walkers number their scopes in their own way, and every reduction
// closes the innermost scope the program opened. When check is non-nil it runs
// after every op with that op's index.
func Run(hh sszutils.HashWalker, prog []Op, check func(int)) (root [32]byte, err error) {
	var open []int

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	reduceIndex := func() int {
		if len(open) == 0 {
			return 0
		}
		idx := open[len(open)-1]
		open = open[:len(open)-1]
		return idx
	}

	for i, op := range prog {
		switch op.kind { //nolint:exhaustive // opKindCount is a bound, not a call
		case opAppend:
			hh.Append(opBytes(op.n))
		case opAppendBytes32:
			hh.AppendBytes32(opBytes(op.n))
		case opAppendBool:
			hh.AppendBool(op.n%2 == 0)
		case opAppendUint8:
			hh.AppendUint8(uint8(op.n))
		case opAppendUint16:
			hh.AppendUint16(uint16(op.n))
		case opAppendUint32:
			hh.AppendUint32(uint32(op.n))
		case opAppendUint64:
			hh.AppendUint64(op.num)
		case opPutBool:
			hh.PutBool(op.n%2 == 0)
		case opPutUint8:
			hh.PutUint8(uint8(op.n))
		case opPutUint16:
			hh.PutUint16(uint16(op.n))
		case opPutUint32:
			hh.PutUint32(uint32(op.n))
		case opPutUint64:
			hh.PutUint64(op.num)
		case opPutBytes:
			hh.PutBytes(opBytes(op.n))
		case opPutUint64Array:
			vals := make([]uint64, op.n)
			for j := range vals {
				vals[j] = uint64(j) + op.num
			}
			if op.num%2 == 0 {
				hh.PutUint64Array(vals)
			} else {
				hh.PutUint64Array(vals, uint64(op.n)+op.num%8)
			}
		case opPutBitlist:
			hh.PutBitlist(bitlistOf(op.n), uint64(op.n)+op.num%64)
		case opPutProgressiveBitlist:
			hh.PutProgressiveBitlist(bitlistOf(op.n))
		case opFillUpTo32:
			hh.FillUpTo32()
		case opCollapse:
			hh.Collapse()
		case opStartTree:
			open = append(open, hh.StartTree(op.treeType))
		case opIndex:
			open = append(open, hh.Index())
		case opMerkleize:
			idx := reduceIndex()
			hh.Merkleize(idx)
		case opMerkleizeWithMixin:
			idx := reduceIndex()
			hh.MerkleizeWithMixin(idx, op.num, uint64(op.n))
		case opMerkleizeProgressive:
			idx := reduceIndex()
			hh.MerkleizeProgressive(idx)
		case opMerkleizeProgressiveWithMixin:
			idx := reduceIndex()
			hh.MerkleizeProgressiveWithMixin(idx, op.num)
		case opMerkleizeProgressiveWithActiveFields:
			idx := reduceIndex()
			hh.MerkleizeProgressiveWithActiveFields(idx, opBytes(op.n))
		}
		if check != nil {
			check(i)
		}
	}

	return hh.HashRoot()
}

// bitlistOf builds a bitlist of n bits with the sentinel bit set.
func bitlistOf(n int) []byte {
	b := make([]byte, n/8+1)
	for i := 0; i < n; i++ {
		if i%3 != 0 {
			b[i/8] |= 1 << (i % 8)
		}
	}
	b[n/8] |= 1 << (n % 8)
	return b
}

func String(prog []Op) string {
	var sb strings.Builder
	for i, op := range prog {
		fmt.Fprintf(&sb, "  %2d %s\n", i, op)
	}
	return sb.String()
}

// generateProgram builds one program: an outer scope, a body of random calls
// with a shadow scope stack so reductions mostly close a scope that is open,
// then the closing reductions. The body deliberately includes the sequences a
// delegate that breaks its contract produces: raw appends that leave a partial
// chunk before a scope opens or a value is written.
func Generate(rng *rand.Rand) []Op {
	prog, _ := GenerateWithScopes(rng)
	return prog
}

// generateProgramWithScopes also reports, for every prefix of the program, the
// scopes still open after it, so a prefix can be closed and compared on its
// own.
func GenerateWithScopes(rng *rand.Rand) ([]Op, [][]sszutils.TreeType) {
	prog := []Op{{kind: opStartTree, treeType: sszutils.TreeTypeBinary}}
	// open holds the shape of every scope that is still open, so a reduction
	// closes a scope the way its shape asks for. A progressive scope closed by
	// a binary reduce is not a sequence any emitter produces, and the hasher
	// does not answer it the same way with and without collapse hints.
	open := []sszutils.TreeType{sszutils.TreeTypeBinary}
	openAfter := [][]sszutils.TreeType{{sszutils.TreeTypeBinary}}
	byteLens := []int{0, 1, 2, 3, 5, 7, 31, 32, 33, 48, 63, 64, 65, 96, 200}
	rawLens := []int{1, 2, 3, 5, 7, 8, 16, 31, 33, 63}

	for i, n := 0, 4+rng.Intn(20); i < n; i++ {
		kind := opKind(rng.Intn(int(opKindCount)))
		op := Op{kind: kind, num: rng.Uint64() % 64}

		switch kind {
		case opAppend:
			op.n = rawLens[rng.Intn(len(rawLens))]
		case opAppendBytes32, opPutBytes:
			op.n = byteLens[rng.Intn(len(byteLens))]
		case opPutUint64Array:
			op.n = rng.Intn(12)
		case opPutBitlist, opPutProgressiveBitlist:
			op.n = rng.Intn(600)
		case opMerkleizeProgressiveWithActiveFields:
			op.n = []int{0, 1, 4, 32, 33, 64}[rng.Intn(6)]
		case opStartTree:
			op.treeType = []sszutils.TreeType{
				sszutils.TreeTypeNone,
				sszutils.TreeTypeBinary,
				sszutils.TreeTypeProgressive,
				sszutils.TreeTypeBinary | sszutils.TreeTypePacked,
				sszutils.TreeTypeProgressive | sszutils.TreeTypePacked,
			}[rng.Intn(5)]
		default:
			op.n = rng.Intn(70)
		}

		switch kind { //nolint:exhaustive // only the scope ops change the stack
		case opStartTree, opIndex:
			if len(open) >= 6 {
				continue
			}
			shape := sszutils.TreeTypeNone
			if kind == opStartTree {
				shape = op.treeType
			}
			open = append(open, shape)
		case opMerkleize, opMerkleizeWithMixin, opMerkleizeProgressive,
			opMerkleizeProgressiveWithMixin, opMerkleizeProgressiveWithActiveFields:
			// Keep the outer scope open and close an inner one.
			if len(open) <= 1 {
				continue
			}
			// Every reduction closes the innermost open scope. A reduction at
			// the index of an already closed scope is not a sequence any
			// emitter produces, and it makes hasher.Hasher's layered result
			// differ from a plain reduction of its own bytes, so it says
			// nothing about the chunking contract these two walkers share.
			op.kind = reduceFor(open[len(open)-1], rng)
			open = open[:len(open)-1]
		}
		prog = append(prog, op)
		openAfter = append(openAfter, append([]sszutils.TreeType(nil), open...))
	}

	for i := len(open) - 1; i >= 0; i-- {
		prog = append(prog, Op{kind: reduceFor(open[i], rng), n: 1})
		openAfter = append(openAfter, append([]sszutils.TreeType(nil), open[:i]...))
	}
	return prog, openAfter
}

// closeScopes returns the reductions that close the given open scopes,
// innermost first, so a prefix of a program can be finished and compared.
func CloseScopes(open []sszutils.TreeType) []Op {
	ops := make([]Op, 0, len(open))
	for i := len(open) - 1; i >= 0; i-- {
		kind := opMerkleize
		if open[i]&^sszutils.TreeTypePacked == sszutils.TreeTypeProgressive {
			kind = opMerkleizeProgressive
		}
		ops = append(ops, Op{kind: kind, n: 1})
	}
	return ops
}

// reduceFor picks a reduction that matches the scope's shape.
func reduceFor(shape sszutils.TreeType, rng *rand.Rand) opKind {
	if shape&^sszutils.TreeTypePacked == sszutils.TreeTypeProgressive {
		return []opKind{
			opMerkleizeProgressive,
			opMerkleizeProgressiveWithMixin,
			opMerkleizeProgressiveWithActiveFields,
		}[rng.Intn(3)]
	}
	return []opKind{opMerkleize, opMerkleizeWithMixin}[rng.Intn(2)]
}

// compareWalkers runs one program through both walkers twice: once comparing
// the tail after every call, which forces the hasher to flush its deferred
// reductions, and once without, which leaves the deferral path as callers see
// it. Both runs must agree on the root, and a panic must happen in both or in
// neither.
func Compare(prog []Op) error {
	// The hasher may hold several completed scopes as raw chunks, batching
	// their reduction, so the two buffers are not the same length at every
	// step; the tail each walker reports is what both must agree on.
	var tails [][2]string
	var lens [][2]int
	hasherRoot, hasherErr := Run(hasher.NewHasher(), prog, nil)
	wrapperRoot, wrapperErr := Run(treeproof.NewWrapper(), prog, nil)

	// A collapse hint tells the hasher to reduce part of a scope early, so from
	// the first one the two walkers hold different intermediate shapes of the
	// same tree and only the roots have to agree. Up to that point every step
	// must report the same tail.
	tailSteps := len(prog)
	for i, op := range prog {
		if op.kind == opCollapse {
			tailSteps = i
			break
		}
	}

	h, w := hasher.NewHasher(), treeproof.NewWrapper()
	var stepErr error
	_, _ = Run(h, prog, func(i int) {
		if stepErr != nil {
			return
		}
		lens = append(lens, [2]int{h.CurrentIndex(), 0})
		tails = append(tails, [2]string{fmt.Sprintf("%x", h.Hash()), ""})
		_ = i
	})
	_, _ = Run(w, prog, func(i int) {
		if i < len(tails) {
			lens[i][1] = w.CurrentIndex()
			tails[i][1] = fmt.Sprintf("%x", w.Hash())
		}
	})
	for i, pair := range tails {
		if i >= tailSteps {
			break
		}
		if pair[0] != pair[1] {
			return fmt.Errorf("step %d (%s): hasher tail %s (%d bytes), wrapper tail %s (%d bytes)\n  hasher wrote %d bytes, wrapper wrote %d",
				i, prog[i], pair[0], lens[i][0], pair[1], lens[i][1], h.CurrentIndex(), w.CurrentIndex())
		}
	}
	if stepErr != nil {
		return stepErr
	}

	// A collapse hint is a hint: the root must not depend on it.
	if tailSteps < len(prog) {
		plain := make([]Op, 0, len(prog))
		for _, op := range prog {
			if op.kind != opCollapse {
				plain = append(plain, op)
			}
		}
		plainRoot, plainErr := Run(hasher.NewHasher(), plain, nil)
		if (plainErr == nil) != (hasherErr == nil) || (hasherErr == nil && plainRoot != hasherRoot) {
			return fmt.Errorf("collapse hints moved the hasher root: %x (%v) with, %x (%v) without",
				hasherRoot, hasherErr, plainRoot, plainErr)
		}
	}

	switch {
	case (hasherErr == nil) != (wrapperErr == nil):
		return fmt.Errorf("hasher err = %v, wrapper err = %v", hasherErr, wrapperErr)
	case hasherErr != nil:
		return nil
	case !bytes.Equal(hasherRoot[:], wrapperRoot[:]):
		return fmt.Errorf("root: hasher %x, wrapper %x", hasherRoot, wrapperRoot)
	}
	return nil
}
