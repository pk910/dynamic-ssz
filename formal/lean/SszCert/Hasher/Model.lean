/-!
# Executable model of dynamic-ssz's incremental and async hasher

A transcription of `hasher/hasher.go` and `hasher/async.go` at chunk
granularity. Every function of the Go hasher that shapes the tree — scope
opening and closing, the `Collapse` hint, binary batch collapse,
progressive group finalization and compaction, `collapseAllDepths` with its
odd-count padding and limit expansion, deferred child batching
(`flushPending`, the deferral branch of `Merkleize` with its power-of-two
padding), and the async paths (`flushPendingAsync`, `precollapseCapRun`,
async group reductions, `drainJobs`/`drainJobsFor`, ring-full drains) — has a
counterpart here with the same control flow and the same thresholds.

What is abstracted:

* **Hash values.** The model is generic in the chunk type `C`, the pair hash
  `H`, the zero chunk and the mix-in words. Theorems about it hold for every
  hash function, so they say nothing about SHA-256 and need nothing from it.
* **Byte offsets.** The Go hasher tracks one byte buffer and byte offsets;
  the model keeps each open scope's region as structured lists (completed
  progressive roots, collapse nodes with their depths, the deferred run, the
  raw tail) whose concatenation is that region, and the absolute chunk
  position of every cell is computed from them. Holes of in-flight jobs are
  recorded at absolute positions exactly as Go's `dstOff`, and a drain writes
  the results there: a job whose hole moved would land on the wrong cell, as
  in Go, and the model flags it.
* **Partial chunks.** Packed values are appended as whole chunks once they
  fill one; Go carries a partial tail opaquely through every reduction.

The Go hasher and this model are held together by a per-operation state
comparison (`hasher/formal_trace_test.go`); the proofs (`Hasher/Proofs`)
show the model always computes the specified root.
-/

namespace Ssz.Hasher

/-- The thresholds of the Go hasher (`hasher/params.go`). -/
structure Params where
  /-- `incrementalBatchSize`: same-depth node count that triggers a batch. -/
  batch : Nat := 256
  /-- `asyncMinChunks`: smallest group reduced in the background. -/
  asyncMin : Nat := 8192
  /-- `lazyFlushChunks`: job width (a power of two). -/
  cap : Nat := 32768
  /-- `asyncCapDepth`: `log2 cap`. -/
  capDepth : Nat := 15
  /-- `asyncActiveChunks`: progressive active region that triggers a round. -/
  active : Nat := 131072
  /-- `asyncPrecollapseLevel`: lowest progressive level whose groups are
      multiples of the cap. -/
  precollapseLevel : Nat := 8
  /-- Job ring size (twice the worker count): a full ring drains its oldest. -/
  ring : Nat := 8
  deriving Repr

/-- A chunk of the buffer: a value, or one chunk of an in-flight job's hole.
    A hole carries the value its job will write there: a job's result is
    fixed when it is enqueued (the worker reduces a private copy of its
    input), so values never depend on when jobs finish. Reading a hole — or
    a drain finding it moved — is a protocol violation the model flags. -/
inductive Cell (C : Type) where
  | ready (v : C)
  | hole (v : C) (job idx : Nat)
  deriving Repr, BEq, Inhabited

/-- A node of a collapse region: a subtree root at the given depth. -/
structure Node (C : Type) where
  depth : Nat
  cell : Cell C
  deriving Repr, BEq, Inhabited

/-- One open SSZ object scope (`treeLayer`). The region is
    `prog ++ nodes ++ flatten pend ++ tail`. -/
structure Layer (C : Type) where
  incremental : Bool
  legacy : Bool
  progressive : Bool
  packed : Bool
  collapsed : Bool := false
  /-- Completed progressive group roots (`progressiveCount` of them). -/
  prog : List (Cell C) := []
  /-- `progressiveLevel`. -/
  level : Nat := 0
  /-- Collapse region, deepest first. The first `counted` are reflected in
      Go's `counts`; the rest are depth-0 chunks appended since. -/
  nodes : List (Node C) := []
  counted : Nat := 0
  /-- Deferred child subtrees (`pendCount` of `pendElemChunks` chunks). -/
  pendC : Nat := 0
  pend : List (List (Cell C)) := []
  /-- Chunks appended after the deferred run. -/
  tail : List (Cell C) := []
  deriving Repr, Inhabited

/-- An in-flight background reduction: its results, and where its hole is. -/
structure Job (C : Type) where
  id : Nat
  dst : Nat
  out : List C
  deriving Repr

/-- Hasher state. `stack` lists open scopes innermost first; `base` holds
    the chunks outside every scope. -/
structure St (C : Type) where
  base : List (Cell C) := []
  stack : List (Layer C) := []
  /-- In-flight jobs, oldest first (the ring). -/
  jobs : List (Job C) := []
  nextJob : Nat := 0
  /-- `jobMaxEnd`: end of the highest hole since the last full drain. -/
  jobMaxEnd : Nat := 0
  /-- A protocol violation of the model: reading or moving a hole, or a
      state the Go code assumes impossible. The proofs show it stays `none`. -/
  err : Option String := none
  /-- `hashErr`: an error Go records with `setHashErr` (shape mismatch,
      chunk limit) and reports at `HashRoot`. -/
  hashErr : Option String := none
  deriving Repr

/-- Hash function and words the reduction uses. -/
structure Ctx (C : Type) where
  H : C → C → C
  /-- The zero chunk. -/
  z0 : C
  /-- Little-endian 32-byte word of a number (length and selector mix-ins). -/
  word : Nat → C
  P : Params

variable {C : Type}

/-! ## Values -/

/-- Root of the all-zero subtree of depth `d` (`zeroHashes[d]`). -/
def zeroH (H : C → C → C) (z0 : C) : Nat → C
  | 0 => z0
  | d + 1 => H (zeroH H z0 d) (zeroH H z0 d)

/-- One reduction level: hash consecutive pairs. -/
def pairUp (H : C → C → C) : List C → List C
  | a :: b :: rest => H a b :: pairUp H rest
  | _ => []

/-- `getDepth`: the depth of a tree holding `n` leaves. -/
def depthOf (n : Nat) : Nat :=
  if n ≤ 1 then 0 else Nat.log2 (n - 1) + 1

/-- One level of `merkleizeImpl`: pad an odd layer with the zero hash of
    that level, then pair. -/
def levelUp (H : C → C → C) (z : C) (xs : List C) : List C :=
  pairUp H (if xs.length % 2 = 1 then xs ++ [z] else xs)

/-- Iterate `levelUp` from level `i` for `n` levels. -/
def levelsUp (H : C → C → C) (z0 : C) : Nat → Nat → List C → List C
  | 0, _, xs => xs
  | n + 1, i, xs => levelsUp H z0 n (i + 1) (levelUp H (zeroH H z0 i) xs)

/-- `merkleizeImpl`: binary root of chunks with a chunk limit (`0` = as
    many as there are). Reports a limit overflow (reduced at the depth the
    chunks need, as Go does). -/
def merkleizeImpl (H : C → C → C) (z0 : C) (xs : List C) (limit : Nat) : C × Bool :=
  let count := xs.length
  let limit0 := if limit = 0 then count else limit
  let over := decide (count > limit0)
  let lim := if over then count else limit0
  if lim = 0 then (z0, over)
  else if lim = 1 then ((xs.head?.getD z0), over)
  else
    let d := depthOf lim
    if xs = [] then (zeroH H z0 d, over)
    else ((levelsUp H z0 d 0 xs).head?.getD z0, over)

/-- `merkleizeProgressiveImpl`: progressive root (EIP-7916 spine). -/
def progImpl (H : C → C → C) (z0 : C) : Nat → Nat → List C → C
  | 0, _, _ => z0
  | fuel + 1, depth, xs =>
    if xs = [] then z0
    else
      let base := 2 ^ depth
      let left := (merkleizeImpl H z0 (xs.take base) base).1
      let rest := xs.drop base
      let right := if rest = [] then z0 else progImpl H z0 fuel (depth + 2) rest
      H left right

/-! ## Cells -/

def Cell.isReady : Cell C → Bool
  | .ready _ => true
  | .hole _ _ _ => false

/-- The value a cell holds or will hold. -/
def Cell.val : Cell C → C
  | .ready v => v
  | .hole v _ _ => v

/-- The values of cells, if none is a hole. -/
def readyVals : List (Cell C) → Option (List C)
  | [] => some []
  | .ready v :: cs => (readyVals cs).map (v :: ·)
  | .hole _ _ _ :: _ => none

def nodeCells (ns : List (Node C)) : List (Cell C) := ns.map (·.cell)

/-- A layer's region, in buffer order. -/
def Layer.cells (l : Layer C) : List (Cell C) :=
  l.prog ++ nodeCells l.nodes ++ l.pend.flatten ++ l.tail

/-- The whole buffer: base, then scopes outermost first. -/
def St.cells (s : St C) : List (Cell C) :=
  s.base ++ (s.stack.reverse.map Layer.cells).flatten

/-- Absolute chunk position where the innermost scope's region begins. -/
def St.topStart (s : St C) : Nat :=
  s.base.length + ((s.stack.drop 1).reverse.map (fun l => l.cells.length)).sum

/-- Absolute position where the innermost scope's active region begins
    (after its progressive roots). -/
def St.activeStart (s : St C) : Nat :=
  s.topStart + (match s.stack with | l :: _ => l.prog.length | [] => 0)

def St.fail (s : St C) (msg : String) : St C :=
  match s.err with
  | none => { s with err := some msg }
  | some _ => s

def St.setHashErr (s : St C) (msg : String) : St C :=
  match s.hashErr with
  | none => { s with hashErr := some msg }
  | some _ => s

/-! ## Draining -/

/-- Write `vals` over cells starting at position `pos` of `cs`, requiring
    each overwritten cell to be the expected hole `(job, i)`. Returns the
    updated cells and whether every target was that hole. -/
def writeHole (job : Nat) : Nat → List C → List (Cell C) → List (Cell C) × Bool
  | _, [], cs => (cs, true)
  | _, _ :: _, [] => ([], false)
  | 0, v :: vs, c :: cs =>
    let ok := match c with | .hole _ j _ => j == job | _ => false
    let (rest, ok') := writeHole job 0 vs cs
    (.ready v :: rest, ok && ok')
  | p + 1, vs, c :: cs =>
    let (rest, ok) := writeHole job p vs cs
    (c :: rest, ok)

/-- Split a flat cell list back into a layer's region structure. -/
def Layer.setCells (l : Layer C) (cs : List (Cell C)) : Layer C :=
  let np := l.prog.length
  let nn := l.nodes.length
  let prog := cs.take np
  let rest := cs.drop np
  let nodes := (l.nodes.zip (rest.take nn)).map (fun (n, c) => { n with cell := c })
  let rest := rest.drop nn
  let rec splitPend : List (List (Cell C)) → List (Cell C) → List (List (Cell C)) × List (Cell C)
    | [], r => ([], r)
    | e :: es, r =>
      let (es', r') := splitPend es (r.drop e.length)
      (r.take e.length :: es', r')
  let (pend, tail) := splitPend l.pend rest
  { l with prog := prog, nodes := nodes, pend := pend, tail := tail }

/-- Replace the whole buffer, keeping the region structure. -/
def St.setCells (s : St C) (cs : List (Cell C)) : St C :=
  let nb := s.base.length
  let rec go : List (Layer C) → List (Cell C) → List (Layer C)
    | [], _ => []
    | l :: ls, r => l.setCells (r.take l.cells.length) :: go ls (r.drop l.cells.length)
  { s with base := cs.take nb, stack := (go s.stack.reverse (cs.drop nb)).reverse }

/-- Finish the oldest job: write its results into its hole. -/
def St.drainOldest (s : St C) : St C :=
  match s.jobs with
  | [] => s
  | j :: js =>
    let (cs, ok) := writeHole j.id j.dst j.out s.cells
    let s' := { s.setCells cs with jobs := js }
    if ok then s' else s'.fail s!"job {j.id}: hole at {j.dst} moved before its drain"

/-- `drainJobs`. -/
def St.drainAll (s : St C) : St C :=
  let rec go : Nat → St C → St C
    | 0, s => s
    | n + 1, s => go n s.drainOldest
  { go s.jobs.length s with jobMaxEnd := 0 }

/-- `drainJobsFor(indx)`: drain everything when a hole can lie at or after
    `indx`. -/
def St.drainFor (s : St C) (indx : Nat) : St C :=
  if s.jobs ≠ [] ∧ indx < s.jobMaxEnd then s.drainAll else s

/-! ## Layer access -/

def St.top? (s : St C) : Option (Layer C) := s.stack.head?

def St.setTop (s : St C) (l : Layer C) : St C :=
  match s.stack with
  | _ :: ls => { s with stack := l :: ls }
  | [] => s

/-- Append a cell at the end of the buffer (the innermost region's tail). -/
def St.push (s : St C) (c : Cell C) : St C :=
  match s.stack with
  | [] => { s with base := s.base ++ [c] }
  | l :: ls =>
    if l.pend = [] then { s with stack := { l with nodes := l.nodes ++ [⟨0, c⟩] } :: ls }
    else { s with stack := { l with tail := l.tail ++ [c] } :: ls }

/-- Read the values of cells, flagging a hole (a read before the drain). -/
def St.vals (s : St C) (cs : List (Cell C)) (what : String) : St C × List C :=
  (if cs.all Cell.isReady then s else s.fail s!"{what}: read a hole", cs.map Cell.val)

/-! ## Jobs -/

/-- Pair levels until at most `outChunks` remain (the worker's loop,
    `reduceAsyncJob`, and `flushPending`'s level loop). -/
def reduceTo (H : C → C → C) (outChunks : Nat) : Nat → List C → List C
  | 0, xs => xs
  | fuel + 1, xs => if xs.length > outChunks then reduceTo H outChunks fuel (pairUp H xs) else xs

/-- `enqueueReduce`: claim a ring slot (draining the oldest when the ring is
    full), snapshot the input (it must hold no hole), and return the holes
    that replace it. The job's results are fixed at enqueue: the worker
    reduces a private copy. -/
def St.enqueue (cx : Ctx C) (s : St C) (input : List (Cell C)) (outChunks dst : Nat) :
    St C × List (Cell C) :=
  let s := if s.jobs.length ≥ cx.P.ring then s.drainOldest else s
  let (s, vs) := s.vals input "job input"
  let out := reduceTo cx.H outChunks input.length vs
  let id := s.nextJob
  let holes := (List.range outChunks).map (fun k => Cell.hole (out.getD k cx.z0) id k)
  let s := { s with jobs := s.jobs ++ [⟨id, dst, out⟩], nextJob := id + 1,
                    jobMaxEnd := max s.jobMaxEnd (dst + outChunks) }
  (s, holes)

/-! ## Reductions on node lists -/

/-- Hash consecutive pairs of nodes of one depth into nodes one deeper. -/
def St.pairNodes (cx : Ctx C) (s : St C) (ns : List (Node C)) : St C × List (Node C) :=
  let (s, vs) := s.vals (nodeCells ns) "pair"
  (s, (pairUp cx.H vs).map (fun v => ⟨(ns.head?.map (·.depth)).getD 0 + 1, .ready v⟩))

/-- Number of nodes of depth `d` among the first `k` nodes (Go's counts). -/
def countAt (ns : List (Node C)) (k d : Nat) : Nat :=
  ((ns.take k).filter (·.depth == d)).length

def maxDepthOf (ns : List (Node C)) : Nat := ns.foldl (fun m n => max m n.depth) 0

/-- Leaves covered by nodes. -/
def leavesOf (ns : List (Node C)) : Nat := (ns.map (fun n => 2 ^ n.depth)).sum

/-- The loop of `collapseAllDepths`: reduce the lowest-depth group (padded
    to even with the zero hash of its depth) until one node remains. Go
    operates on the last `count` cells, the lowest-depth group of a
    deepest-first region. -/
def collapseLoop (cx : Ctx C) (live : Bool) : Nat → St C → List (Node C) → St C × List (Node C)
  | 0, s, ns => (s.fail "collapseAllDepths: no progress", ns)
  | fuel + 1, s, ns =>
    match ns.map (·.depth) |>.min? with
    | none => (s, ns)
    | some low =>
      if (ns.filter (·.depth == low)).length = 1 ∧ ns.all (fun n => n.depth == low || n.depth < low) then (s, ns)
      else
        let cnt := (ns.filter (·.depth == low)).length
        -- Go writes the zero pad at `bufEnd`: a live cell when more of the
        -- region follows the reduced span (progressive group rounds).
        let s := if cnt % 2 = 1 ∧ live then s.fail "collapseAllDepths: zero pad over a live cell" else s
        let ns := if cnt % 2 = 1 then ns ++ [⟨low, .ready (zeroH cx.H cx.z0 low)⟩] else ns
        let cnt := if cnt % 2 = 1 then cnt + 1 else cnt
        let keep := ns.take (ns.length - cnt)
        let grp := ns.drop (ns.length - cnt)
        if grp.all (·.depth == low) then
          let (s, paired) := s.pairNodes cx grp
          collapseLoop cx live fuel s (keep ++ paired)
        else (s.fail "collapseAllDepths: lowest group not at the end", ns)

/-- The end of `collapseAllDepths`: one node remains; expand it to the depth
    `limit` needs (`0` = none). -/
def collapseFinish (cx : Ctx C) (limit : Nat) (live : Bool) (s : St C) : List (Node C) → St C × Cell C
  | [n] =>
    if limit > 0 then
      let s := if n.depth < depthOf limit ∧ live then s.fail "collapseAllDepths: expansion over a live cell" else s
      ((s.vals [n.cell] "expand").1, .ready ((List.range (depthOf limit - n.depth)).foldl
        (fun acc i => cx.H acc (zeroH cx.H cx.z0 (n.depth + i))) n.cell.val))
    else (s, n.cell)
  | _ => (s.fail "collapseAllDepths: did not reduce to one node", .ready cx.z0)

/-- `collapseAllDepths`: reduce a node list to one root, then expand it to the
    depth `limit` needs (`0` = none). -/
def St.collapseAll (cx : Ctx C) (s : St C) (ns : List (Node C)) (limit : Nat)
    (live : Bool := false) : St C × Cell C :=
  collapseFinish cx limit live
    (collapseLoop cx live (2 * ns.length + 64)
      (if limit > 0 ∧ leavesOf ns > limit then s.setHashErr "chunk limit" else s) ns).1
    (collapseLoop cx live (2 * ns.length + 64)
      (if limit > 0 ∧ leavesOf ns > limit then s.setHashErr "chunk limit" else s) ns).2

/-- Treat every node of an uncollapsed scope as depth 0, as Go does when it
    (re)initializes counts from the buffer; a deeper node there is an
    impossible state, flagged. -/
def St.uncollapsedAsLeaves (s : St C) (l : Layer C) : St C × Layer C :=
  if l.collapsed then (s, { l with counted := l.nodes.length })
  else
    let s := if l.nodes.any (·.depth ≠ 0) then s.fail "uncollapsed scope with deep nodes" else s
    (s, { l with nodes := l.nodes.map (fun n => { n with depth := 0 }), counted := l.nodes.length })

/-- Flag a violated expectation. -/
def St.failUnless (s : St C) (ok : Bool) (msg : String) : St C := if ok then s else s.fail msg

/-- Number of counted nodes deeper than `d` (Go: the depth-`d` group starts
    after them). -/
def deeperCount (d : Nat) (ns : List (Node C)) : Nat := (ns.filter (·.depth > d)).length

/-- Nodes of depth `d` in whole batches. -/
def batchSize (P : Params) (d : Nat) (ns : List (Node C)) : Nat :=
  ((ns.filter (·.depth == d)).length / P.batch) * P.batch

/-- The nodes one batch round at depth `d` pairs, located as Go does. -/
def batchGroup (P : Params) (d : Nat) (ns : List (Node C)) : List (Node C) :=
  (ns.drop (deeperCount d ns)).take (batchSize P d ns)

/-- One batch round at depth `d`: the whole batches of depth-`d` nodes are
    hashed in place into nodes one deeper. -/
def batchStep (cx : Ctx C) (d : Nat) (s : St C) (ns : List (Node C)) : St C × List (Node C) :=
  (((s.failUnless ((batchGroup cx.P d ns).all (·.depth == d))
      "batch collapse: depth group not where the counts place it").pairNodes cx (batchGroup cx.P d ns)).1,
   ns.take (deeperCount d ns) ++ (s.pairNodes cx (batchGroup cx.P d ns)).2 ++
     (ns.drop (deeperCount d ns)).drop (batchSize cx.P d ns))

/-- Batch rounds from depth `d` up while a depth holds a full batch. -/
def batchLoop (cx : Ctx C) : Nat → Nat → St C → List (Node C) → St C × List (Node C)
  | 0, _, s, ns => (s, ns)
  | fuel + 1, d, s, ns =>
    if (ns.filter (·.depth == d)).length < cx.P.batch then (s, ns)
    else batchLoop cx fuel (d + 1) (batchStep cx d s ns).1 (batchStep cx d s ns).2

/-- The batch rounds of `maybeCollapseBinary` on the innermost scope (after
    its drain): every node becomes counted. -/
def St.collapseBinaryCore (cx : Ctx C) (s : St C) : St C :=
  match s.stack with
  | [] => s
  | l :: _ =>
    (batchLoop cx 64 0 (s.uncollapsedAsLeaves l).1 (s.uncollapsedAsLeaves l).2.nodes).1.setTop
      { (s.uncollapsedAsLeaves l).2 with
        collapsed := true,
        nodes := (batchLoop cx 64 0 (s.uncollapsedAsLeaves l).1 (s.uncollapsedAsLeaves l).2.nodes).2,
        counted := (batchLoop cx 64 0 (s.uncollapsedAsLeaves l).1 (s.uncollapsedAsLeaves l).2.nodes).2.length }

/-- Depth-0 nodes of a scope that count toward a batch (Go's `d0Chunks`). -/
def d0Chunks (l : Layer C) : Nat :=
  l.nodes.length - (if l.collapsed then (l.nodes.take l.counted).countP (·.depth ≥ 1) else 0)

/-- `maybeCollapseBinary` on the innermost scope. -/
def St.collapseBinary (cx : Ctx C) (s : St C) : St C :=
  match s.stack with
  | [] => s
  | l :: _ =>
    if l.pend ≠ [] then s.fail "binary collapse with a pending run"
    else if d0Chunks l < cx.P.batch then s
    else (s.drainFor s.activeStart).collapseBinaryCore cx

/-- Go (re)initializes counts from the buffer when a scope has not collapsed
    yet: the innermost scope with its nodes treated as leaves. -/
def St.normTop (s : St C) : St C :=
  match s.stack with
  | [] => s
  | l :: _ => (s.uncollapsedAsLeaves l).1.setTop (s.uncollapsedAsLeaves l).2

/-- The innermost scope, or `dflt`. -/
def St.topLayer (s : St C) (dflt : Layer C) : Layer C := (s.stack.head?).getD dflt

/-- The cap-depth nodes of a scope (they lead it in a deepest-first region). -/
def capCount (P : Params) (l : Layer C) : Nat := (l.nodes.filter (·.depth == P.capDepth)).length

/-- Replace the cap block after the first `ncap` nodes by its hole. -/
def capCompact (P : Params) (l : Layer C) (ncap : Nat) (holes : List (Cell C)) : Layer C :=
  { l with
    nodes := l.nodes.take ncap ++ holes.map (fun c => ⟨P.capDepth, c⟩) ++ (l.nodes.drop ncap).drop P.cap,
    counted := (l.nodes.take ncap ++ holes.map (fun c => (⟨P.capDepth, c⟩ : Node C)) ++
      (l.nodes.drop ncap).drop P.cap).length,
    collapsed := true }

/-- The job of one cap block of `precollapseCapRun`: the block after the
    first `ncap` nodes, reduced to one node. -/
def St.capJob (cx : Ctx C) (s : St C) (l : Layer C) (ncap : Nat) : St C × List (Cell C) :=
  s.enqueue cx (nodeCells ((l.nodes.drop ncap).take cx.P.cap)) 1 (s.activeStart + ncap)

/-- The rounds of `precollapseCapRun`. Go compacts after every job, so a
    drain triggered by the next enqueue (ring full) finds the earlier holes
    in place. -/
def precollapseLoop (cx : Ctx C) : Nat → St C → Nat → St C
  | 0, s, _ => s
  | fuel + 1, s, ncap =>
    match s.stack with
    | [] => s
    | l :: _ =>
      if (l.nodes.drop ncap).length < cx.P.cap then s
      else precollapseLoop cx fuel
        ((s.capJob cx l ncap).1.setTop (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2))
        (ncap + 1)

/-- `precollapseCapRun`: turn cap-wide spans of the trailing depth-0 run
    into cap-depth nodes reduced in the background. Returns whether jobs
    were emitted. -/
def St.precollapse (cx : Ctx C) (s : St C) : St C × Bool :=
  match s.normTop.stack with
  | [] => (s.normTop, false)
  | l :: _ =>
    if l.nodes.any (fun n => n.depth ≠ 0 ∧ n.depth ≠ cx.P.capDepth) then (s.normTop, false)
    else (precollapseLoop cx ((l.nodes.drop (capCount cx.P l)).length + 1)
            (s.normTop.failUnless ((l.nodes.take (capCount cx.P l)).all (·.depth == cx.P.capDepth) &&
              (l.nodes.drop (capCount cx.P l)).all (·.depth == 0))
              "precollapse: cap nodes not where the counts place them")
            (capCount cx.P l),
          decide ((l.nodes.drop (capCount cx.P l)).length ≥ cx.P.cap))

/-- Reduce a deferred element (a power-of-two run of chunks) to its root. -/
def reduceElem (H : C → C → C) (z0 : C) (xs : List C) : C :=
  (reduceTo H 1 xs.length xs).head?.getD z0

/-- Read the deferred elements in order, reducing each to its root. -/
def St.flushFold (cx : Ctx C) : St C → List (List (Cell C)) → St C × List C
  | s, [] => (s, [])
  | s, e :: es =>
    (((s.vals e "flush").1.flushFold cx es).1,
     reduceElem cx.H cx.z0 (s.vals e "flush").2 :: ((s.vals e "flush").1.flushFold cx es).2)

/-- `flushPending` (synchronous): every deferred element becomes one root;
    the raw tail moves down behind them. -/
def St.flushSync (cx : Ctx C) (s : St C) : St C :=
  match s.stack with
  | [] => s
  | l :: _ =>
    if l.pend = [] then s else
    (s.flushFold cx l.pend).1.setTop
      { l with nodes := l.nodes ++ (s.flushFold cx l.pend).2.map (fun v => ⟨0, .ready v⟩) ++
                          l.tail.map (fun c => ⟨0, c⟩),
               pend := [], pendC := 0, tail := [] }

/-- `asyncRootCompatible`. -/
def asyncRootCompatible (l : Layer C) (nodeDepth : Nat) : Bool :=
  if !l.collapsed then l.nodes.isEmpty
  else (l.nodes.take l.counted).all (fun n => n.depth ≥ nodeDepth) && l.nodes.length == l.counted

/-- The layer after one binary round of `flushPendingAsync`: the
    consumed elements become one node at the depth of their subtree. -/
def binRoundLayer (P : Params) (c : Nat) (l : Layer C) (holes : List (Cell C)) : Layer C :=
  { l with collapsed := true,
           nodes := (if l.collapsed then l.nodes else []) ++ holes.map (fun h => ⟨Nat.log2 (P.cap / c), h⟩),
           counted := ((if l.collapsed then l.nodes else []) ++
             holes.map (fun h => (⟨Nat.log2 (P.cap / c), h⟩ : Node C))).length,
           pend := l.pend.drop (P.cap / c) }

/-- One binary round: `cap / c` elements reduced in the background to one
    root, written where the run begins. -/
def St.binRound (cx : Ctx C) (c : Nat) (s : St C) (l : Layer C) : St C :=
  (s.enqueue cx (l.pend.take (cx.P.cap / c)).flatten 1 (s.activeStart + (nodeCells l.nodes).length)).1.setTop
    (binRoundLayer cx.P c
      ((s.enqueue cx (l.pend.take (cx.P.cap / c)).flatten 1 (s.activeStart + (nodeCells l.nodes).length)).1.topLayer l)
      (s.enqueue cx (l.pend.take (cx.P.cap / c)).flatten 1 (s.activeStart + (nodeCells l.nodes).length)).2)

/-- One progressive round: the elements' roots computed in the background
    land as depth-0 nodes. -/
def St.progRound (cx : Ctx C) (c : Nat) (s : St C) (l : Layer C) : St C :=
  (s.enqueue cx (l.pend.take (cx.P.cap / c)).flatten (cx.P.cap / c) (s.activeStart + (nodeCells l.nodes).length)).1.setTop
    (let l' := (s.enqueue cx (l.pend.take (cx.P.cap / c)).flatten (cx.P.cap / c)
                  (s.activeStart + (nodeCells l.nodes).length)).1.topLayer l
     { l' with nodes := l'.nodes ++ (s.enqueue cx (l.pend.take (cx.P.cap / c)).flatten (cx.P.cap / c)
                  (s.activeStart + (nodeCells l.nodes).length)).2.map (fun h => ⟨0, h⟩),
               pend := l'.pend.drop (cx.P.cap / c) })

/-- The rounds of `flushPendingAsync` (`dflt`: the scope it started on). -/
def flushAsyncLoop (cx : Ctx C) (dflt : Layer C) (c : Nat) : Nat → St C → Bool → St C × Bool
  | 0, s, e => (s, e)
  | fuel + 1, s, e =>
    if (s.topLayer dflt).pend.length < cx.P.cap / c then (s, e)
    else if (s.topLayer dflt).progressive then
      flushAsyncLoop cx dflt c fuel (s.progRound cx c (s.topLayer dflt)) true
    else if !asyncRootCompatible (s.topLayer dflt) (Nat.log2 (cx.P.cap / c)) then (s, e)
    else flushAsyncLoop cx dflt c fuel (s.binRound cx c (s.topLayer dflt)) true

/-- After the rounds: an emptied run lets the raw tail move down. -/
def flushFinish (l : Layer C) : Layer C :=
  if l.pend = [] then { l with pendC := 0, nodes := l.nodes ++ l.tail.map (fun c => ⟨0, c⟩), tail := [] } else l

/-- `flushPendingAsync`: cap-wide prefixes of the deferred run become
    background jobs. Returns whether any job was emitted. -/
def St.flushAsync (cx : Ctx C) (s : St C) : St C × Bool :=
  match s.stack with
  | [] => (s, false)
  | l :: _ =>
    ((flushAsyncLoop cx l l.pendC (l.pend.length + 1) s false).1.setTop
      (flushFinish ((flushAsyncLoop cx l l.pendC (l.pend.length + 1) s false).1.topLayer l)),
     (flushAsyncLoop cx l l.pendC (l.pend.length + 1) s false).2)

/-- `flushPending(layer, allowAsync)` on the innermost scope. -/
def St.flush (cx : Ctx C) (s : St C) (allowAsync async : Bool) : St C :=
  match s.stack with
  | [] => s
  | l :: _ =>
    let c := l.pendC
    if allowAsync ∧ l.pend ≠ [] ∧ c > 0 ∧ c ≤ cx.P.cap ∧ c &&& (c - 1) = 0 ∧ async then
      let (s', emitted) := s.flushAsync cx
      if emitted then s' else s.flushSync cx
    else s.flushSync cx

/-- Take nodes of depth `d` while they fit `base` (`avail` of them at most). -/
def takeFit (base d : Nat) : Nat → Nat → Nat → Nat × Nat
  | 0, c, k => (c, k)
  | f + 1, c, k => if c + 2 ^ d ≤ base then takeFit base d f (c + 2 ^ d) (k + 1) else (c, k)

/-- Go's depth-greedy count, deepest depth first. -/
def byDepth (ns : List (Node C)) (base : Nat) : Nat → Nat → Nat → Nat × Nat
  | 0, consumed, k => (consumed, k)
  | fuel + 1, consumed, k =>
    if (takeFit base fuel (ns.filter (·.depth == fuel)).length consumed k).1 = base
    then takeFit base fuel (ns.filter (·.depth == fuel)).length consumed k
    else byDepth ns base fuel (takeFit base fuel (ns.filter (·.depth == fuel)).length consumed k).1
      (takeFit base fuel (ns.filter (·.depth == fuel)).length consumed k).2

/-- Consume exactly `base` leaves from the front of the node list, by Go's
    depth-greedy count: returns the number of nodes taken. -/
def consumeCount (ns : List (Node C)) (base : Nat) : Nat := (byDepth ns base (maxDepthOf ns + 1) 0 0).2

/-- The root of a progressive group: a background job when the group is a
    plain run of leaves of a size the async path takes, else reduced here. -/
def St.groupRoot (cx : Ctx C) (async : Bool) (s : St C) (l : Layer C) (k : Nat) : St C × Cell C :=
  if async ∧ (l.nodes.take k).all (·.depth == 0) ∧ leavesOf (l.nodes.take k) = 4 ^ l.level ∧
      4 ^ l.level ≥ cx.P.asyncMin ∧ 4 ^ l.level ≤ cx.P.cap then
    ((s.enqueue cx (nodeCells (l.nodes.take k)) 1 (s.topStart + l.prog.length)).1,
     ((s.enqueue cx (nodeCells (l.nodes.take k)) 1 (s.topStart + l.prog.length)).2.head?.getD (.ready cx.z0)))
  else s.collapseAll cx (l.nodes.take k) (4 ^ l.level) (decide (l.nodes.length > k))

/-- The scope after a round: the group's root joins the progressive roots. -/
def roundLayer (l : Layer C) (k : Nat) (root : Cell C) : Layer C :=
  { l with prog := l.prog ++ [root], level := l.level + 1, nodes := l.nodes.drop k, counted := l.counted - k }

/-- The rounds of `maybeCollapseProgressive`: every complete group becomes a
    progressive root. Go updates the scope in place, so a round re-reads it
    after its enqueue (a ring-full drain may have readied cells). -/
def progRounds (cx : Ctx C) (async : Bool) : Nat → St C → Layer C → Bool → St C × Layer C × Bool
  | 0, s, l, f => (s, l, f)
  | fuel + 1, s, l, f =>
    if leavesOf (l.nodes.take l.counted) < 4 ^ l.level then (s, l, f) else
    if consumeCount (l.nodes.take l.counted) (4 ^ l.level) = 0 then (s, l, f) else
    progRounds cx async fuel
      ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.setTop
        (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2))
      (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2)
      true

/-- Step 2 of a synchronous progressive collapse: pair the nodes depth by
    depth, deepest first (the odd one kept). -/
def progCompact (cx : Ctx C) : Nat → St C → List (Node C) → St C × List (Node C)
  | 0, s, _ => (s, [])
  | fuel + 1, s, ns =>
    ((progCompact cx fuel (s.pairNodes cx ((ns.filter (·.depth == fuel)).take
        (2 * ((ns.filter (·.depth == fuel)).length / 2)))).1 ns).1,
     (s.pairNodes cx ((ns.filter (·.depth == fuel)).take (2 * ((ns.filter (·.depth == fuel)).length / 2)))).2 ++
       (ns.filter (·.depth == fuel)).drop (2 * ((ns.filter (·.depth == fuel)).length / 2)) ++
       (progCompact cx fuel (s.pairNodes cx ((ns.filter (·.depth == fuel)).take
        (2 * ((ns.filter (·.depth == fuel)).length / 2)))).1 ns).2)

/-- After the rounds. -/
def progFinish (cx : Ctx C) (async : Bool) (r : St C × Layer C × Bool) : St C :=
  if !r.2.2 then (if !async then r.1.collapseBinary cx else r.1)
  else if async then r.1.setTop { r.2.1 with collapsed := true }
  else
    ((progCompact cx (maxDepthOf r.2.1.nodes + 1) r.1 r.2.1.nodes).1.setTop
      { r.2.1 with nodes := (progCompact cx (maxDepthOf r.2.1.nodes + 1) r.1 r.2.1.nodes).2,
                   counted := (progCompact cx (maxDepthOf r.2.1.nodes + 1) r.1 r.2.1.nodes).2.length,
                   collapsed := true }).collapseBinary cx

/-- (Re)initialize a progressive scope's counts. -/
def St.progInit (s : St C) (l : Layer C) : St C :=
  ((if !l.collapsed ∧ (l.prog ≠ [] ∨ l.level ≠ 0) then s.fail "uncollapsed progressive scope with roots" else s).uncollapsedAsLeaves l).1.setTop
    ((if !l.collapsed ∧ (l.prog ≠ [] ∨ l.level ≠ 0) then s.fail "uncollapsed progressive scope with roots" else s).uncollapsedAsLeaves l).2

/-- The rounds and their finish, on the (re)initialized scope. -/
def St.progCore (cx : Ctx C) (async : Bool) (s : St C) : St C :=
  match s.stack with
  | [] => s
  | l :: _ => progFinish cx async (progRounds cx async (l.nodes.length + 1) s l false)

/-- `maybeCollapseProgressive` on the innermost scope. -/
def St.collapseProgressive (cx : Ctx C) (s : St C) (async : Bool) : St C :=
  match (s.drainFor s.topStart).stack with
  | [] => s.drainFor s.topStart
  | l :: _ =>
    if async ∧ l.level ≥ cx.P.precollapseLevel then
      (if (((s.drainFor s.topStart).progInit l).precollapse cx).2 then
        (((s.drainFor s.topStart).progInit l).precollapse cx).1
       else (((s.drainFor s.topStart).progInit l).precollapse cx).1.progCore cx async)
    else ((s.drainFor s.topStart).progInit l).progCore cx async

/-- The partial last group's root joins the roots. -/
def St.closePartial (cx : Ctx C) (s : St C) (l : Layer C) : St C × Layer C :=
  if l.nodes ≠ [] then
    ((s.collapseAll cx l.nodes (4 ^ l.level)).1,
     { l with prog := l.prog ++ [(s.collapseAll cx l.nodes (4 ^ l.level)).2], nodes := [], counted := 0 })
  else (s, l)

/-- The fold of the roots, right to left. -/
def St.closeFold (cx : Ctx C) (s : St C) : St C × Cell C :=
  match s.stack with
  | [] => (s, .ready cx.z0)
  | l :: _ => ((s.vals l.prog "fold").1, .ready ((s.vals l.prog "fold").2.foldr (fun r acc => cx.H r acc) cx.z0))

/-- After the collapses and the drain. -/
def St.closeTail (cx : Ctx C) (s : St C) : St C × Cell C :=
  match s.stack with
  | [] => (s, .ready cx.z0)
  | l :: _ => (((s.closePartial cx l).1.setTop (s.closePartial cx l).2).drainAll).closeFold cx

/-- `collapseProgressiveLayer`: finalize all groups, the partial remainder,
    and fold the roots right to left. Returns the root cell. -/
def St.closeProgressive (cx : Ctx C) (s : St C) (async : Bool) : St C × Cell C :=
  (if (s.collapseProgressive cx async).jobs ≠ [] then
      ((s.collapseProgressive cx async).drainAll).collapseProgressive cx async
    else s.collapseProgressive cx async).drainAll.closeTail cx

/-! ## Operations -/

inductive TreeType where
  | none
  | binary
  | progressive
  deriving Repr, BEq, DecidableEq

/-- Hasher API calls, as the engines make them. Every call carries whether
    async hashing is in effect (`asyncShared()` is read per call). -/
inductive Op (C : Type) where
  | startTree (t : TreeType) (packed : Bool)
  | index
  | leaf (v : C)
  | collapse (async : Bool)
  | merkleize (async : Bool)
  | merkleizeMixin (num limit : Nat) (async : Bool)
  | merkleizeProg (async : Bool)
  | merkleizeProgMixin (num : Nat) (async : Bool)
  | merkleizeProgActive (af : C) (async : Bool)
  deriving Repr

/-- `deferrableElemChunks`. -/
def deferrableWidth (P : Params) (n : Nat) : Option Nat :=
  if n < 2 ∨ n > P.batch then none
  else if n &&& (n - 1) = 0 then some n
  else some (2 ^ (Nat.log2 n + 1))

/-- Pop the innermost scope and append its root to the enclosing region. -/
def St.popWith (s : St C) (root : Cell C) : St C :=
  match s.stack with
  | [] => s
  | _ :: ls => ({ s with stack := ls } : St C).push root

/-- Record a chunk-limit overflow. -/
def St.setHashErrIf (s : St C) (b : Bool) : St C := if b then s.setHashErr "chunk limit" else s

/-- The binary root of a scope with a chunk limit (`0` = as many as there
    are): a collapsed scope reduces through `collapseAllDepths`, any other
    through `merkleizeImpl`. -/
def St.binRoot (cx : Ctx C) (s : St C) (l : Layer C) (limit : Nat) : St C × Cell C :=
  if l.collapsed then s.collapseAll cx l.nodes limit
  else (((s.vals l.cells "merkleize").1.setHashErrIf (merkleizeImpl cx.H cx.z0 (l.cells.map Cell.val) limit).2),
        .ready (merkleizeImpl cx.H cx.z0 (l.cells.map Cell.val) limit).1)

/-- The non-deferred close of `Merkleize`: flush, drain, root, pop. -/
def St.closeBinary (cx : Ctx C) (s : St C) (limit : Nat) (_async : Bool) : St C :=
  match ((s.flushSync cx).drainFor (s.flushSync cx).topStart).stack with
  | [] => (s.flushSync cx).drainFor (s.flushSync cx).topStart
  | l :: _ =>
    (((s.flushSync cx).drainFor (s.flushSync cx).topStart).binRoot cx l limit).1.popWith
      (((s.flushSync cx).drainFor (s.flushSync cx).topStart).binRoot cx l limit).2

/-- The close of `MerkleizeWithMixin` after its flush and drain: root, mix in
    the length, pop. -/
def St.mixinClose (cx : Ctx C) (s : St C) (num limit : Nat) : St C :=
  match s.stack with
  | [] => s
  | l :: _ =>
    ((s.binRoot cx l limit).1.vals [(s.binRoot cx l limit).2] "mixin").1.popWith
      (.ready (cx.H (s.binRoot cx l limit).2.val (cx.word num)))

/-- `MerkleizeProgressiveWithMixin` / `WithActiveFields`. -/
def St.closeProgWord (cx : Ctx C) (s : St C) (w : C) (async : Bool) : St C :=
  match s.stack with
  | [] => s.fail "merkleize without a scope"
  | l :: _ =>
    let s := if !l.progressive ∧ l.incremental then s.setHashErr "scope shape mismatch" else s
    let s := if l.pend ≠ [] then s.flushSync cx else s
    let s := s.drainFor s.topStart
    match s.stack with
    | [] => s
    | l :: _ =>
      let (s, root) :=
        if l.progressive ∧ l.prog ≠ [] then s.closeProgressive cx async
        else
          let (s, vs) := s.vals l.cells "merkleize"
          (s, .ready (progImpl cx.H cx.z0 (vs.length + 1) 0 vs))
      let (s, rv) := s.vals [root] "mixin"
      s.popWith (.ready (cx.H (rv.head?.getD cx.z0) w))

def St.step (cx : Ctx C) (s : St C) : Op C → St C
  | .startTree t packed =>
    let s := match s.stack with
      | l :: _ => if t ≠ TreeType.none ∧ l.pend ≠ [] then s.flushSync cx else s
      | [] => s
    { s with stack := { incremental := t ≠ TreeType.none, legacy := false, progressive := t == TreeType.progressive,
                        packed := packed } :: s.stack }
  | .index =>
    { s with stack := { incremental := false, legacy := true, progressive := false, packed := false } :: s.stack }
  | .leaf v => s.push (.ready v)
  | .collapse async =>
    match s.stack with
    | [] => s
    | l :: _ =>
      if !l.incremental then s else
      if l.pend ≠ [] ∧ async ∧ l.pend.length * l.pendC < cx.P.cap then s else
      let s := if l.pend ≠ [] then s.flush cx async async else s
      match s.stack with
      | [] => s
      | l :: _ =>
      if l.progressive then
        if async ∧ l.cells.length - l.prog.length < cx.P.active then s else
        let s := if l.pend ≠ [] then s.flushSync cx else s
        s.collapseProgressive cx async
      else
        if l.pend ≠ [] then s else
        if async then (s.precollapse cx).1 else s.collapseBinary cx
  | .merkleize async =>
    match s.stack with
    | [] => s.fail "merkleize without a scope"
    | l :: ls =>
      let s := if l.progressive then s.setHashErr "scope shape mismatch" else s
      let deferrable := !l.legacy && (!l.incremental || (!l.progressive && !l.collapsed && l.pend = []))
      let width := deferrableWidth cx.P (l.cells.length)
      match deferrable, ls, width with
      | true, p :: _, some c =>
        if !p.incremental then s.closeBinary cx 0 async else
        let elem := l.cells ++ List.replicate (c - l.cells.length) (.ready cx.z0)
        let s := ({ s with stack := ls } : St C)
        -- a run that does not extend contiguously or has another width flushes first
        let s := match s.stack with
          | p :: _ => if p.pend ≠ [] ∧ (p.pendC ≠ c ∨ p.tail ≠ []) then s.flushSync cx else s
          | [] => s
        match s.stack with
        | p :: _ =>
          let s := s.setTop { p with pendC := c, pend := p.pend ++ [elem] }
          if (p.pend.length + 1) * c ≥ cx.P.cap then s.flush cx true async else s
        | [] => s
      | _, _, _ => s.closeBinary cx 0 async
  | .merkleizeMixin num limit _ =>
    match s.stack with
    | [] => s.fail "merkleize without a scope"
    | l :: _ =>
      (((if l.progressive then s.setHashErr "scope shape mismatch" else s).flushSync cx).drainFor
        ((if l.progressive then s.setHashErr "scope shape mismatch" else s).flushSync cx).topStart).mixinClose cx num limit
  | .merkleizeProg async =>
    match s.stack with
    | [] => s.fail "merkleize without a scope"
    | l :: _ =>
      let s := if !l.progressive ∧ l.incremental then s.setHashErr "scope shape mismatch" else s
      let s := if l.pend ≠ [] then s.flushSync cx else s
      let s := s.drainFor s.topStart
      match s.stack with
      | [] => s
      | l :: _ =>
        if l.progressive then
          let (s, root) := s.closeProgressive cx async
          s.popWith root
        else
          let (s, vs) := s.vals l.cells "merkleize"
          s.popWith (.ready (progImpl cx.H cx.z0 (vs.length + 1) 0 vs))
  | .merkleizeProgMixin num async => s.closeProgWord cx (cx.word num) async
  | .merkleizeProgActive af async => s.closeProgWord cx af async

/-- Run a sequence of operations from the empty hasher. -/
def run (cx : Ctx C) (ops : List (Op C)) : St C :=
  ops.foldl (St.step cx) {}

/-- `HashRoot`: drain, then the single remaining chunk. -/
def St.root (s : St C) : Option C :=
  let s := s.drainAll
  if s.err.isSome ∨ s.hashErr.isSome ∨ s.stack ≠ [] then none
  else match readyVals s.base with
    | some [v] => some v
    | _ => none

end Ssz.Hasher
