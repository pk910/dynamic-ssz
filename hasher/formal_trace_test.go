// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package hasher

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// TestFormalTrace holds the hasher to its formal model
// (formal/lean/SszCert/Hasher/Model.lean, executable as hasher-model): random
// walks of hasher calls — nested binary, none, progressive, packed and legacy
// scopes, uniform element runs that defer into their parent, Collapse hints at
// random points, async hashing switched on and off per call, partial packed
// chunks across hints — run on the real hasher and on the model, and after
// every call the full reduction state is compared: per scope its progressive
// roots and level, every node of the collapse region with its depth, the
// counted prefix, the deferred run and the tail, plus the in-flight jobs and
// their hole positions. The model's own protocol checks (no hole read or
// moved before its drain, no scratch write over a live chunk) must hold
// throughout, and the final roots must agree.
//
// Build with -tags dynssz_smallhash to shrink the thresholds so every
// batching and async path is reached; without it the production thresholds
// are used. The hash is a fast mixing function over chunk fingerprints (the
// model is generic in the hash); the zero-hash table is rebuilt from it.
//
// HASHER_MODEL overrides the model binary; the test is skipped when it is not
// built. HASHER_TRACE_WALKS sets the number of walks (default 400).
func TestFormalTrace(t *testing.T) {
	modelPath := os.Getenv("HASHER_MODEL")
	if modelPath == "" && incrementalBatchSize > 64 {
		// Production thresholds make walks large; only run when asked.
		t.Skip("production thresholds: set HASHER_MODEL (or build with -tags dynssz_smallhash)")
	}
	if modelPath == "" {
		modelPath = filepath.Join("..", "formal", "lean", ".lake", "build", "bin", "hasher-model")
	}
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("hasher-model not built: %v", err)
	}
	walks := 400
	if incrementalBatchSize > 64 {
		walks = 30 // production thresholds: a sanity pass; the small build covers the paths
	}
	if v := os.Getenv("HASHER_TRACE_WALKS"); v != "" {
		walks, _ = strconv.Atoi(v)
	}
	seed := int64(20261108)
	if v := os.Getenv("HASHER_TRACE_SEED"); v != "" {
		seed, _ = strconv.ParseInt(v, 10, 64)
	}

	installMixZeroHashes(t)
	const workers = 2
	EnableAsyncHashing(workers)
	t.Cleanup(DisableAsyncHashing)

	m := startModel(t, modelPath)
	m.ask(fmt.Sprintf("P %d %d %d %d %d %d %d", incrementalBatchSize, asyncMinChunks, lazyFlushChunks,
		asyncCapDepth, asyncActiveChunks, asyncPrecollapseLevel, 2*workers), false)

	rng := rand.New(rand.NewSource(seed))
	totalOps := 0
	for w := 0; w < walks; w++ {
		g := &walkGen{rng: rng, asyncMode: rng.Intn(3)}
		g.scope(0)
		ops := g.ops
		totalOps += len(ops)
		if !runWalk(t, m, w, ops) {
			return
		}
	}
	t.Logf("%d walks, %d hasher calls compared state-for-state with the model", walks, totalOps)
}

// --- hash -----------------------------------------------------------------

func mixU64(a, b uint64) uint64 {
	x := a*0x9E3779B97F4A7C15 + b + 0x632BE59BD9B4E019
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

// fingerprint folds a chunk into the 64-bit value the model calls it by.
func fingerprint(c []byte) uint64 {
	return binary.LittleEndian.Uint64(c[0:]) +
		binary.LittleEndian.Uint64(c[8:])*0xC2B2AE3D27D4EB4F +
		binary.LittleEndian.Uint64(c[16:])*0x165667B19E3779F9 +
		binary.LittleEndian.Uint64(c[24:])*0x27D4EB2F165667C5
}

func chunkOf(v uint64) [32]byte {
	var c [32]byte
	binary.LittleEndian.PutUint64(c[:], v)
	return c
}

// mixHash is the trace hash backend: each 64-byte pair becomes the chunk
// whose fingerprint is mix(fp(left), fp(right)). In place, as the backends.
func mixHash(dst, input []byte) error {
	n := len(input) / 64
	for i := 0; i < n; i++ {
		a := fingerprint(input[64*i : 64*i+32])
		b := fingerprint(input[64*i+32 : 64*i+64])
		c := chunkOf(mixU64(a, b))
		copy(dst[32*i:32*i+32], c[:])
	}
	return nil
}

// installMixZeroHashes rebuilds the zero-hash table from mixHash for the
// test (the reduction pads with it), restoring the sha256 table afterwards.
func installMixZeroHashes(t *testing.T) {
	initHasher()
	saved := zeroHashes
	for i := 0; i < 64; i++ {
		zeroHashes[i+1] = chunkOf(mixU64(fingerprint(zeroHashes[i][:]), fingerprint(zeroHashes[i][:])))
	}
	t.Cleanup(func() { zeroHashes = saved })
}

// --- model process --------------------------------------------------------

type modelProc struct {
	t  *testing.T
	wr *bufio.Writer
	rd *bufio.Reader
}

func startModel(t *testing.T, path string) *modelProc {
	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start hasher-model: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return &modelProc{t: t, wr: bufio.NewWriterSize(stdin, 1<<16), rd: bufio.NewReaderSize(stdout, 1<<20)}
}

func (m *modelProc) ask(line string, answer bool) string {
	fmt.Fprintln(m.wr, line)
	if err := m.wr.Flush(); err != nil {
		m.t.Fatalf("write to model: %v", err)
	}
	if !answer {
		return ""
	}
	ans, err := m.rd.ReadString('\n')
	if err != nil {
		m.t.Fatalf("model died: %v", err)
	}
	return strings.TrimRight(ans, "\n")
}

// --- walks ----------------------------------------------------------------

// traceOp is one hasher call: its model line and its Go execution. A
// Go-only step (the first half of a packed chunk) has no model line.
type traceOp struct {
	model string
	apply func(r *walkRun)
	// skip: the model step has no Go counterpart yet (a dangling packed
	// half chunk the close pads); the next call is compared.
	skip bool
}

type walkRun struct {
	h       *Hasher
	indices []int
}

func (r *walkRun) top() int { return r.indices[len(r.indices)-1] }
func (r *walkRun) pop() int {
	i := r.top()
	r.indices = r.indices[:len(r.indices)-1]
	return i
}

type walkGen struct {
	rng       *rand.Rand
	ops       []traceOp
	asyncMode int // 0 off, 1 on, 2 random per call
	leafSeq   uint64
}

// opBudget bounds a walk: past it, scopes get no further items.
const opBudget = 30000

func (g *walkGen) async() bool {
	switch g.asyncMode {
	case 0:
		return false
	case 1:
		return true
	default:
		return g.rng.Intn(2) == 0
	}
}

func b01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func (g *walkGen) emit(model string, apply func(r *walkRun)) {
	g.ops = append(g.ops, traceOp{model: model, apply: apply})
}

func (g *walkGen) leafChunk() [32]byte {
	var c [32]byte
	g.rng.Read(c[:])
	g.leafSeq++
	return c
}

func (g *walkGen) collapse() {
	a := g.async()
	g.emit("co "+b01(a), func(r *walkRun) { r.h.SetAsyncHashing(a); r.h.Collapse() })
}

// itemCount draws a scope width with weight on the batch, cap and
// progressive-group boundaries of the active thresholds.
func (g *walkGen) itemCount(depth int) int {
	if len(g.ops) > opBudget {
		return 0
	}
	if depth >= 3 {
		return g.rng.Intn(6)
	}
	edges := []int{0, 1, 2, 3, incrementalBatchSize - 1, incrementalBatchSize, incrementalBatchSize + 1,
		2 * incrementalBatchSize, asyncMinChunks, lazyFlushChunks - 1, lazyFlushChunks, lazyFlushChunks + 1,
		5, 21, 85, 2 * lazyFlushChunks, asyncActiveChunks, asyncActiveChunks + 3}
	n := edges[g.rng.Intn(len(edges))]
	if g.rng.Intn(3) == 0 {
		n = g.rng.Intn(3 * lazyFlushChunks)
	}
	if n > 300 {
		n = 300 // production thresholds: keep walks bounded
	}
	return n
}

// scope generates one scope and its contents.
func (g *walkGen) scope(depth int) {
	kind := g.rng.Intn(10)
	n := g.itemCount(depth)
	collapseP := g.rng.Intn(4) // 0: never, else every item with prob 1/collapseP
	switch {
	case kind < 4: // binary (incremental), container-of-elements style
		packed := depth > 0 && g.rng.Intn(4) == 0
		g.open(sszutils.TreeTypeBinary, packed)
		chunks := g.items(depth, n, packed, collapseP)
		g.closeBinary(chunks)
	case kind < 5: // non-incremental (TreeTypeNone): a container
		g.open(sszutils.TreeTypeNone, false)
		chunks := g.items(depth, min(n, 2*incrementalBatchSize+3), false, collapseP)
		if g.rng.Intn(4) == 0 {
			g.closeProgressive(chunks) // the non-incremental progressive path
		} else {
			g.closeBinary(chunks)
		}
	case kind < 8: // progressive list / container
		packed := depth > 0 && g.rng.Intn(4) == 0
		g.open(sszutils.TreeTypeProgressive, packed)
		chunks := g.items(depth, n, packed, collapseP)
		g.closeProgressive(chunks)
	default: // legacy Index scope (fastssz-generated code)
		g.emit("ix", func(r *walkRun) { r.indices = append(r.indices, r.h.Index()) })
		g.items(depth, min(n, incrementalBatchSize+2), false, 0)
		a := g.async()
		g.emit("mz "+b01(a), func(r *walkRun) { r.h.SetAsyncHashing(a); r.h.Merkleize(r.pop()) })
	}
}

func (g *walkGen) open(tt sszutils.TreeType, packed bool) {
	name := map[sszutils.TreeType]string{sszutils.TreeTypeNone: "none", sszutils.TreeTypeBinary: "binary",
		sszutils.TreeTypeProgressive: "progressive"}[tt]
	full := tt
	if packed {
		full |= sszutils.TreeTypePacked
	}
	g.emit("st "+name+" "+b01(packed), func(r *walkRun) { r.indices = append(r.indices, r.h.StartTree(full)) })
}

// items generates n items; returns the chunk count of the scope. Packed
// scopes append half chunks (two per chunk). Elements are either all raw
// chunks, a uniform run of child scopes (which defer into an incremental
// parent), or a random mix.
func (g *walkGen) items(depth, n int, packed bool, collapseP int) int {
	maybeCollapse := func() {
		if collapseP > 0 && g.rng.Intn(collapseP) == 0 {
			g.collapse()
		}
	}
	if packed {
		var half []byte
		chunks := 0
		for i := 0; i < n; i++ {
			c := g.leafChunk()
			h1 := append([]byte(nil), c[:16]...)
			if half == nil {
				half = h1
				g.emit("", func(r *walkRun) { r.h.Append(h1) })
			} else {
				full := append(append([]byte(nil), half...), h1...)
				fp := fingerprint(full)
				half = nil
				chunks++
				g.emit(fmt.Sprintf("lf %016x", fp), func(r *walkRun) { r.h.Append(h1) })
			}
			maybeCollapse()
		}
		if half != nil {
			padded := append(append([]byte(nil), half...), make([]byte, 16)...)
			// The close pads the region; the model sees the padded chunk now.
			g.ops = append(g.ops, traceOp{model: fmt.Sprintf("lf %016x", fingerprint(padded)),
				apply: func(r *walkRun) {}, skip: true})
			chunks++
		}
		return chunks
	}
	style := g.rng.Intn(3)
	if depth >= 3 {
		style = 0
	}
	var tmpl func()
	if style == 1 {
		// uniform child shape: k chunks each, a container scope
		k := 1 + g.rng.Intn(6)
		tmpl = func() {
			g.open(sszutils.TreeTypeNone, false)
			for j := 0; j < k; j++ {
				g.leaf()
			}
			a := g.async()
			g.emit("mz "+b01(a), func(r *walkRun) { r.h.SetAsyncHashing(a); r.h.Merkleize(r.pop()) })
		}
	}
	for i := 0; i < n && len(g.ops) < 2*opBudget; i++ {
		switch {
		case style == 0:
			g.leaf()
		case style == 1:
			tmpl()
		default:
			if g.rng.Intn(3) == 0 {
				g.scope(depth + 1)
			} else {
				g.leaf()
			}
		}
		maybeCollapse()
	}
	return n
}

func (g *walkGen) leaf() {
	if g.rng.Intn(6) == 0 {
		// Longer values are merkleized in place to one chunk.
		b := make([]byte, 1+g.rng.Intn(200))
		g.rng.Read(b)
		g.emit(fmt.Sprintf("lf %016x", refBytesRoot(b)), func(r *walkRun) { r.h.PutBytes(b) })
		return
	}
	c := g.leafChunk()
	g.emit(fmt.Sprintf("lf %016x", fingerprint(c[:])), func(r *walkRun) { r.h.PutBytes(c[:]) })
}

// refBytesRoot is an independent reference for PutBytes outside packed
// scopes: chunks zero-padded, merkleized to the depth their count needs.
func refBytesRoot(b []byte) uint64 {
	n := (len(b) + 31) / 32
	layer := make([]uint64, n)
	for i := 0; i < n; i++ {
		var c [32]byte
		copy(c[:], b[32*i:min(len(b), 32*i+32)])
		layer[i] = fingerprint(c[:])
	}
	z := uint64(0)
	for len(layer) > 1 {
		if len(layer)%2 == 1 {
			layer = append(layer, z)
		}
		next := make([]uint64, len(layer)/2)
		for i := range next {
			next[i] = mixU64(layer[2*i], layer[2*i+1])
		}
		layer = next
		z = mixU64(z, z)
	}
	return layer[0]
}

func (g *walkGen) closeBinary(chunks int) {
	a := g.async()
	if g.rng.Intn(2) == 0 {
		g.emit("mz "+b01(a), func(r *walkRun) { r.h.SetAsyncHashing(a); r.h.Merkleize(r.pop()) })
		return
	}
	num := uint64(g.rng.Intn(1 << 20))
	limit := uint64(chunks + g.rng.Intn(3*lazyFlushChunks+2))
	if chunks > 1 && g.rng.Intn(25) == 0 {
		limit = uint64(g.rng.Intn(chunks)) // over capacity: a recorded hash error
	}
	if limit == 0 && g.rng.Intn(2) == 0 {
		limit = 1
	}
	g.emit(fmt.Sprintf("mm %d %d %s", num, limit, b01(a)), func(r *walkRun) {
		r.h.SetAsyncHashing(a)
		r.h.MerkleizeWithMixin(r.pop(), num, limit)
	})
}

func (g *walkGen) closeProgressive(int) {
	a := g.async()
	switch g.rng.Intn(3) {
	case 0:
		g.emit("mp "+b01(a), func(r *walkRun) { r.h.SetAsyncHashing(a); r.h.MerkleizeProgressive(r.pop()) })
	case 1:
		num := uint64(g.rng.Intn(1 << 20))
		g.emit(fmt.Sprintf("mpm %d %s", num, b01(a)), func(r *walkRun) {
			r.h.SetAsyncHashing(a)
			r.h.MerkleizeProgressiveWithMixin(r.pop(), num)
		})
	default:
		af := make([]byte, 1+g.rng.Intn(80))
		g.rng.Read(af)
		g.emit(fmt.Sprintf("mpa %016x %s", refBytesRoot(af), b01(a)), func(r *walkRun) {
			r.h.SetAsyncHashing(a)
			r.h.MerkleizeProgressiveWithActiveFields(r.pop(), af)
		})
	}
}

// --- execution and comparison ----------------------------------------------

func runWalk(t *testing.T, m *modelProc, w int, ops []traceOp) bool {
	h := NewHasherWithHashFn(mixHash)
	r := &walkRun{h: h}
	m.ask("R", false)
	var history []string
	for i, op := range ops {
		op.apply(r)
		if op.model == "" {
			continue
		}
		history = append(history, op.model)
		want := m.ask(op.model, true)
		if op.skip {
			continue
		}
		got := renderHasher(h)
		if strings.Contains(want, " ERR ") || strings.HasPrefix(want, "ERR") {
			t.Errorf("walk %d op %d (%s): the model flags a protocol violation:\n  model: %s\n  go:    %s\n  ops: %s",
				w, i, op.model, want, got, strings.Join(history, " ; "))
			return false
		}
		if got != want {
			t.Errorf("walk %d op %d (%s): STATE DIVERGENCE\n  model: %s\n  go:    %s\n  ops: %s",
				w, i, op.model, want, got, strings.Join(history, " ; "))
			return false
		}
	}
	root, err := h.HashRoot()
	want := m.ask("HR", true)
	gotLine := fmt.Sprintf("ROOT %016x", fingerprint(root[:]))
	if err != nil {
		gotLine = "NOROOT " + err.Error()
	}
	if (err == nil) != strings.HasPrefix(want, "ROOT ") || (err == nil && gotLine != want) {
		t.Errorf("walk %d: ROOT DIVERGENCE model=%q go=%q\n  ops: %s", w, want, gotLine, strings.Join(history, " ; "))
		return false
	}
	return true
}

// renderHasher renders the Go hasher's reduction state in the model's
// format (HasherModel.lean, render).
func renderHasher(h *Hasher) string {
	holes := map[int]bool{}
	var jobs []string
	for i := 0; i < h.jobCount; i++ {
		j := &h.jobRing[(h.jobHead+i)%len(h.jobRing)]
		for k := 0; k < j.outLen/32; k++ {
			holes[j.dstOff/32+k] = true
		}
		jobs = append(jobs, fmt.Sprintf("%d+%d", j.dstOff/32, j.outLen/32))
	}
	nChunks := len(h.buf) / 32
	cell := func(i int) string {
		if holes[i] {
			return "?"
		}
		return fmt.Sprintf("%016x", fingerprint(h.buf[32*i:32*i+32]))
	}
	cells := func(from, to int) string {
		parts := make([]string, 0, to-from)
		for i := from; i < to; i++ {
			parts = append(parts, cell(i))
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	baseEnd := nChunks
	if h.layerCount >= 0 {
		baseEnd = h.layers[0].bufIdx / 32
	}
	var layers []string
	for li := 0; li <= h.layerCount; li++ {
		l := &h.layers[li]
		start := l.bufIdx / 32
		end := nChunks
		if li < h.layerCount {
			end = h.layers[li+1].bufIdx / 32
		}
		flags := b01(l.incremental) + b01(l.legacy) + b01(l.progressive) + b01(l.packed) + b01(l.collapsed)
		active := start
		prog := ""
		if l.progressive {
			active = start + l.progressiveCount
			prog = "P" + cells(start, active) + "v" + strconv.Itoa(l.progressiveLevel)
		}
		nodesEnd, pend := end, ""
		if l.pendCount > 0 {
			runStart := l.pendStart / 32
			nodesEnd = runStart
			var elems []string
			for e := 0; e < l.pendCount; e++ {
				s := runStart + e*l.pendElemChunks
				elems = append(elems, cells(s, s+l.pendElemChunks))
			}
			runEnd := runStart + l.pendCount*l.pendElemChunks
			pend = fmt.Sprintf("C%dE[%s]T%s", l.pendElemChunks, strings.Join(elems, ","), cells(runEnd, end))
		}
		var depths []int
		counted := 0
		if l.collapsed {
			for d := maxTreeDepth - 1; d >= 0; d-- {
				for k := 0; k < int(l.counts[d]); k++ {
					depths = append(depths, d)
				}
			}
			counted = len(depths)
		}
		var nodes []string
		for i := active; i < nodesEnd; i++ {
			d := 0
			if i-active < len(depths) {
				d = depths[i-active]
			}
			nodes = append(nodes, fmt.Sprintf("%d:%s", d, cell(i)))
		}
		k := ""
		if l.collapsed {
			k = "k" + strconv.Itoa(counted)
		}
		layers = append(layers, fmt.Sprintf("L{%s}%sN[%s]%s%s", flags, prog, strings.Join(nodes, ","), k, pend))
	}
	return fmt.Sprintf("B%s %s J[%s] H%s", cells(0, baseEnd), strings.Join(layers, " "),
		strings.Join(jobs, ","), b01(h.hashErr != nil))
}
