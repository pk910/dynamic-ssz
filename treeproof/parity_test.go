// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// The two HashWalker implementations must build the same tree from any call
// sequence, including one a delegate leaves on a partial chunk. hasher.Hasher
// carries a single byte buffer and cuts chunks when a scope is reduced;
// treeproof.Wrapper keeps nodes. This harness drives both with one generated
// program and compares what they say after every step.

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
type walkerOp struct {
	kind     opKind
	n        int
	num      uint64
	treeType sszutils.TreeType
}

func (o walkerOp) String() string {
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
func runProgram(hh sszutils.HashWalker, prog []walkerOp, check func(int)) (root [32]byte, err error) {
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
		switch op.kind {
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

func programString(prog []walkerOp) string {
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
func generateProgram(rng *rand.Rand) []walkerOp {
	prog, _ := generateProgramWithScopes(rng)
	return prog
}

// generateProgramWithScopes also reports, for every prefix of the program, the
// scopes still open after it, so a prefix can be closed and compared on its
// own.
func generateProgramWithScopes(rng *rand.Rand) ([]walkerOp, [][]sszutils.TreeType) {
	prog := []walkerOp{{kind: opStartTree, treeType: sszutils.TreeTypeBinary}}
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
		op := walkerOp{kind: kind, num: rng.Uint64() % 64}

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

		switch kind {
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
		prog = append(prog, walkerOp{kind: reduceFor(open[i], rng), n: 1})
		openAfter = append(openAfter, append([]sszutils.TreeType(nil), open[:i]...))
	}
	return prog, openAfter
}

// closeScopes returns the reductions that close the given open scopes,
// innermost first, so a prefix of a program can be finished and compared.
func closeScopes(open []sszutils.TreeType) []walkerOp {
	ops := make([]walkerOp, 0, len(open))
	for i := len(open) - 1; i >= 0; i-- {
		kind := opMerkleize
		if open[i]&^sszutils.TreeTypePacked == sszutils.TreeTypeProgressive {
			kind = opMerkleizeProgressive
		}
		ops = append(ops, walkerOp{kind: kind, n: 1})
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

// Both walkers build the same tree from the same program. The comparison runs
// after every call, so a divergence names the call that caused it rather than
// only the root at the end.
func TestWalkerParity(t *testing.T) {
	for seed := int64(0); seed < 2000; seed++ {
		prog, openAfter := generateProgramWithScopes(rand.New(rand.NewSource(seed))) //nolint:gosec // deterministic test input
		// Every prefix is compared as a program of its own, closed off, so a
		// difference names the call that introduced it rather than the root at
		// the end. The whole program is the last prefix.
		for k := 1; k <= len(prog); k++ {
			full := append(append([]walkerOp(nil), prog[:k]...), closeScopes(openAfter[k-1])...)
			if err := compareWalkers(full); err != nil {
				t.Fatalf("seed %d, after step %d (%s): %v\nprogram:\n%s",
					seed, k-1, prog[k-1], err, programString(full))
			}
		}
	}
}

// The tree the walker builds is not only the right root: every leaf of it
// proves against that root. A program whose tree cannot be finalized is one
// the hasher also refuses, which compareWalkers already pins.
func TestWalkerParityProofs(t *testing.T) {
	for seed := int64(0); seed < 200; seed++ {
		prog := generateProgram(rand.New(rand.NewSource(seed))) //nolint:gosec // deterministic test input
		w := NewWrapper()
		if _, err := runProgram(w, prog, nil); err != nil {
			continue
		}
		root, err := w.Root()
		if err != nil {
			continue
		}
		if err := root.Finalize(); err != nil {
			t.Fatalf("seed %d: finalize: %v\nprogram:\n%s", seed, err, programString(prog))
		}
		hash := root.Hash()
		for _, gindex := range leafIndices(root, 1, 0) {
			proof, err := root.Prove(gindex)
			if err != nil {
				t.Fatalf("seed %d: prove %d: %v", seed, gindex, err)
			}
			ok, err := VerifyProof(hash, proof)
			if err != nil || !ok {
				t.Fatalf("seed %d: proof for %d verified = %v, %v\nprogram:\n%s", seed, gindex, ok, err, programString(prog))
			}
		}
	}
}

// leafIndices collects the generalized index of every leaf, bounded so a wide
// tree does not turn one program into thousands of proofs.
func leafIndices(n *Node, gindex, depth int) []int {
	if n == nil || depth > 12 {
		return nil
	}
	if n.left == nil && n.right == nil {
		return []int{gindex}
	}
	idx := leafIndices(n.left, gindex*2, depth+1)
	if len(idx) < 64 {
		idx = append(idx, leafIndices(n.right, gindex*2+1, depth+1)...)
	}
	return idx
}

// compareWalkers runs one program through both walkers twice: once comparing
// the tail after every call, which forces the hasher to flush its deferred
// reductions, and once without, which leaves the deferral path as callers see
// it. Both runs must agree on the root, and a panic must happen in both or in
// neither.
func compareWalkers(prog []walkerOp) error {
	// The hasher may hold several completed scopes as raw chunks, batching
	// their reduction, so the two buffers are not the same length at every
	// step; the tail each walker reports is what both must agree on.
	var tails [][2]string
	var lens [][2]int
	hasherRoot, hasherErr := runProgram(hasher.NewHasher(), prog, nil)
	wrapperRoot, wrapperErr := runProgram(NewWrapper(), prog, nil)

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

	h, w := hasher.NewHasher(), NewWrapper()
	var stepErr error
	_, _ = runProgram(h, prog, func(i int) {
		if stepErr != nil {
			return
		}
		lens = append(lens, [2]int{h.CurrentIndex(), 0})
		tails = append(tails, [2]string{fmt.Sprintf("%x", h.Hash()), ""})
		_ = i
	})
	_, _ = runProgram(w, prog, func(i int) {
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
			return fmt.Errorf("step %d (%s): hasher tail %s (%d bytes), wrapper tail %s (%d bytes)\n  hasher wrote %d bytes, wrapper buf %x",
				i, prog[i], pair[0], lens[i][0], pair[1], lens[i][1], h.CurrentIndex(), w.buffer())
		}
	}
	if stepErr != nil {
		return stepErr
	}

	// A collapse hint is a hint: the root must not depend on it.
	if tailSteps < len(prog) {
		plain := make([]walkerOp, 0, len(prog))
		for _, op := range prog {
			if op.kind != opCollapse {
				plain = append(plain, op)
			}
		}
		plainRoot, plainErr := runProgram(hasher.NewHasher(), plain, nil)
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

// FuzzWalkerParity drives the same comparison from fuzzer bytes.
func FuzzWalkerParity(f *testing.F) {
	for seed := int64(0); seed < 8; seed++ {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed int64) {
		prog := generateProgram(rand.New(rand.NewSource(seed))) //nolint:gosec // deterministic test input
		if err := compareWalkers(prog); err != nil {
			t.Fatalf("seed %d: %v\nprogram:\n%s", seed, err, programString(prog))
		}
	})
}
