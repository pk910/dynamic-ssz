import SszCert.Hasher.Walk

/-!
# Deferred runs

A small child scope closed with `Merkleize` inside an incremental scope does
not reduce: its chunks, padded to a power of two, are parked in the parent
as one element of a uniform run (`pendStart/pendCount/pendElemChunks`), and
leaves appended behind the run form its raw tail. `flushPending` reduces the
run (each element to its root) synchronously, or turns cap-wide prefixes of
it into background jobs.

`ScopeOK` is the scope invariant with a deferred run: the settled part (the
scope without its run and tail) satisfies `BinOK` over the leaves before
the run, and the run's elements are ready, uniform and stand for their roots.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- A scope with its deferred run and tail set aside. -/
def Layer.settled (l : Layer C) : Layer C := { l with pend := [], pendC := 0, tail := [] }

/-- The leaves a deferred run and the raw tail behind it stand for. -/
def pendLeaves (cx : Ctx C) (l : Layer C) : List C :=
  l.pend.map (fun e => reduceElem cx.H cx.z0 (e.map Cell.val)) ++ l.tail.map Cell.val

structure PendOK (cx : Ctx C) (l : Layer C) : Prop where
  tail : l.pend = [] → l.tail = []
  ready : ∀ e ∈ l.pend, ∀ c ∈ e, c.isReady = true
  tready : ∀ c ∈ l.tail, c.isReady = true
  width : ∀ e ∈ l.pend, e.length = l.pendC
  pow : l.pend ≠ [] → ∃ k, l.pendC = 2 ^ k ∧ 0 < k ∧ 2 ^ k ≤ cx.P.batch

/-- **The scope invariant with a deferred run.** -/
def ScopeOK (cx : Ctx C) (l : Layer C) (L : List C) : Prop :=
  ∃ L1, L = L1 ++ pendLeaves cx l ∧ BinOK cx l.settled L1 ∧ PendOK cx l

theorem scopeOK_of_bin (cx : Ctx C) {l : Layer C} {L : List C} (hb : BinOK cx l L) : ScopeOK cx l L :=
  ⟨L, by simp [pendLeaves, hb.pend, hb.tail],
    ⟨hb.ninc, hb.noprog, hb.nolegacy, hb.prog, rfl, rfl, hb.cov, hb.leaves, hb.shallow, hb.holes, hb.nonempty⟩,
    ⟨fun _ => hb.tail, by simp [hb.pend], by simp [hb.tail], by simp [hb.pend], fun h => absurd hb.pend h⟩⟩

theorem bin_of_scopeOK (cx : Ctx C) {l : Layer C} {L : List C} (hs : ScopeOK cx l L) (hp : l.pend = []) :
    BinOK cx l L := by
  obtain ⟨L1, hL, hb, hpo⟩ := hs
  have ht := hpo.tail hp
  have : L = L1 := by simp [hL, pendLeaves, hp, ht]
  subst this
  exact ⟨hb.ninc, hb.noprog, hb.nolegacy, hb.prog, hp, ht, hb.cov, hb.leaves, hb.shallow, hb.holes, hb.nonempty⟩

/-- Ready elements are read without error. -/
theorem flushFold_ready (cx : Ctx C) : ∀ (es : List (List (Cell C))) (s : St C),
    (∀ e ∈ es, ∀ c ∈ e, c.isReady = true) →
    s.flushFold cx es = (s, es.map (fun e => reduceElem cx.H cx.z0 (e.map Cell.val)))
  | [], s, _ => rfl
  | e :: es, s, hr => by
    have h1 : s.vals e "flush" = (s, e.map Cell.val) := by
      simp only [St.vals, List.all_eq_true.mpr (hr e (by simp)), if_true]
    have ih := flushFold_ready cx es s (fun e' he' => hr e' (List.mem_cons_of_mem _ he'))
    simp only [St.flushFold, h1, ih, List.map_cons]

theorem cov_append_leaves (H : C → C → C) (z0 : C) {ns : List (Node C)} {L : List C} :
    ∀ cs : List (Cell C), Cov H z0 true ns L →
      Cov H z0 true (ns ++ cs.map (fun c => ⟨0, c⟩)) (L ++ cs.map Cell.val) := by
  intro cs
  induction cs generalizing ns L with
  | nil => intro h; simpa using h
  | cons c cs ih =>
    intro h
    have := ih (cov_push H z0 h c)
    simpa [List.append_assoc] using this

/-- Every job ends before a region of ready cells that closes the buffer. -/
theorem jobs_end_of_ready (s : St C) (ht : TopOK s) (A B : List (Cell C)) (hc : s.cells = A ++ B)
    (hr : ∀ c ∈ B, c.isReady = true) : ∀ j ∈ s.jobs, j.dst + j.out.length ≤ A.length := by
  intro j hj
  have hpos : 0 < j.out.length := List.length_pos_iff.mpr (ht.jobsne j hj)
  obtain ⟨idx, hidx⟩ := ht.holes.inplace j hj (j.out.length - 1) (by omega)
  rcases Nat.lt_or_ge (j.dst + (j.out.length - 1)) A.length with hlt | hge
  · omega
  · exfalso
    rw [hc, List.getElem?_append_right hge] at hidx
    have := hr _ (List.mem_of_getElem? hidx)
    simp [Cell.isReady] at this

/-- The scope after a synchronous flush. -/
def flushedLayer (l : Layer C) (roots : List C) : Layer C :=
  { l with nodes := l.nodes ++ roots.map (fun v => ⟨0, .ready v⟩) ++ l.tail.map (fun c => ⟨0, c⟩), pend := [], pendC := 0, tail := [] }

/-- **`flushPending` (synchronous) on a scope**: the run's elements become
    their roots, the tail moves down behind them, and the scope is settled. -/
theorem flushSync_scope (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (ht : TopOK s) :
    ∃ l', (s.flushSync cx).stack = l' :: ls ∧ BinOK cx l' L ∧ l'.incremental = l.incremental ∧
      l'.progressive = l.progressive ∧ TopOK (s.flushSync cx) ∧ (s.flushSync cx).below = s.below ∧
      (s.flushSync cx).base = s.base ∧ (s.flushSync cx).hashErr = s.hashErr := by
  by_cases hp : l.pend = []
  · have hf : s.flushSync cx = s := by unfold St.flushSync; rw [h]; simp [hp]
    rw [hf]
    exact ⟨l, h, bin_of_scopeOK cx hs hp, rfl, rfl, ht, rfl, rfl, rfl⟩
  obtain ⟨L1, hL, hb, hpo⟩ := hs
  have hff := flushFold_ready cx l.pend s hpo.ready
  generalize hrs : l.pend.map (fun e => reduceElem cx.H cx.z0 (e.map Cell.val)) = roots at hff
  let l' : Layer C := flushedLayer l roots
  have hf : s.flushSync cx = s.setTop l' := by
    unfold St.flushSync; rw [h]; simp only [hp, if_false, hff]; rfl
  rw [hf]
  have hprog : l.prog = [] := hb.prog
  have hnew : ∀ n ∈ roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++ l.tail.map (fun c => ⟨0, c⟩),
      n.depth = 0 ∧ n.cell.isReady = true := by
    intro n hn
    rcases List.mem_append.mp hn with hn | hn
    · obtain ⟨v, _, rfl⟩ := List.mem_map.mp hn; exact ⟨rfl, rfl⟩
    · obtain ⟨c, hc, rfl⟩ := List.mem_map.mp hn; exact ⟨rfl, hpo.tready c hc⟩
  have hn' : l'.nodes = l.nodes ++ (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
      l.tail.map (fun c => ⟨0, c⟩)) := by simp [l', flushedLayer, List.append_assoc]
  have hcov : Cov cx.H cx.z0 true l'.nodes L := by
    have e1 : roots.map (fun v => (⟨0, .ready v⟩ : Node C)) = (roots.map Cell.ready).map (fun c => ⟨0, c⟩) := by
      simp [List.map_map, Function.comp_def]
    have hc0 : Cov cx.H cx.z0 true l.nodes L1 := hb.cov
    have h1 := cov_append_leaves cx.H cx.z0 (roots.map Cell.ready) hc0
    have h2 := cov_append_leaves cx.H cx.z0 l.tail h1
    rw [hL]
    show Cov cx.H cx.z0 true (l.nodes ++ roots.map (fun v => ⟨0, .ready v⟩) ++ l.tail.map (fun c => ⟨0, c⟩)) _
    rw [e1]
    have e2 : (roots.map Cell.ready).map Cell.val = roots := by simp [List.map_map, Function.comp_def, Cell.val]
    rw [e2] at h2
    simpa [pendLeaves, hrs, List.append_assoc] using h2
  have hbin : BinOK cx l' L := by
    refine ⟨hb.ninc, hb.noprog, hb.nolegacy, hprog, rfl, rfl, hcov, ?_, ?_, ?_, ?_⟩
    · intro hc n hn; rw [hn'] at hn
      rcases List.mem_append.mp hn with hn | hn
      · exact hb.leaves hc n hn
      · exact (hnew n hn).1
    · intro n hn; rw [hn'] at hn
      rcases List.mem_append.mp hn with hn | hn
      · exact hb.shallow n hn
      · rw [(hnew n hn).1]; decide
    · intro n hn hr; rw [hn'] at hn
      rcases List.mem_append.mp hn with hn | hn
      · exact hb.holes n hn hr
      · rw [(hnew n hn).2] at hr; cases hr
    · intro hc e; rw [hn'] at e
      exact hb.nonempty hc (List.append_eq_nil_iff.mp e).1
  have hlc : l.cells = nodeCells l.nodes ++ (l.pend.flatten ++ l.tail) := by
    simp [Layer.cells, hprog, List.append_assoc]
  have hl'c : l'.cells = nodeCells l.nodes ++ nodeCells (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
      l.tail.map (fun c => ⟨0, c⟩)) := by
    simp only [Layer.cells, hn', nodeCells_append]; simp [l', flushedLayer, hprog]
  have hrdy : ∀ c ∈ l.pend.flatten ++ l.tail, c.isReady = true := by
    intro c hc
    rcases List.mem_append.mp hc with hc | hc
    · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc; exact hpo.ready e he c hce
    · exact hpo.tready c hc
  refine ⟨l', St.setTop_stack s l l' ls h, hbin, rfl, rfl, ⟨?_, by rw [St.setTop_err]; exact ht.err,
    fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
    fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩,
    St.setTop_below s l l' ls h, St.setTop_base s l', St.setTop_hashErr s l'⟩
  apply holesOK_setTop s l l' ls h ht.holes (nodeCells l.nodes).length
    (by rw [hlc]; simp) (by rw [hlc, hl'c]; simp) (by rw [hl'c]; simp)
  · intro i hi hi'
    have hi2 : i - (nodeCells l.nodes).length < (nodeCells (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
        l.tail.map (fun c => ⟨0, c⟩))).length := by
      have := hi'; rw [hl'c, List.length_append] at this; omega
    have : l'.cells[i]'hi' = (nodeCells (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
        l.tail.map (fun c => ⟨0, c⟩)))[i - (nodeCells l.nodes).length]'hi2 := by
      simp only [hl'c]; rw [List.getElem_append_right (by omega)]
    rw [this]
    simp only [nodeCells, List.getElem_map]
    exact (hnew _ (List.getElem_mem _)).2
  · have := jobs_end_of_ready s ht (s.below ++ nodeCells l.nodes) (l.pend.flatten ++ l.tail)
      (by rw [St.cells_top s l ls h, hlc, List.append_assoc]) hrdy
    intro j hj; have := this j hj; simp at this; omega

/-! ## Transport across drains -/

theorem layer_holesSub {s s' : St C} (hv : s'.view = s.view) {l l' : Layer C} {ls ls' : List (Layer C)}
    (h : s.stack = l :: ls) (h' : s'.stack = l' :: ls') (hc : HolesSub s.cells s'.cells) :
    HolesSub l.cells l'.cells := by
  rw [St.cells_top s l ls h, St.cells_top s' l' ls' h'] at hc
  exact holesSub_suffix (below_length_of_view hv h h').symm hc

theorem cells_split (l : Layer C) (hp : l.prog = []) : l.cells = nodeCells l.nodes ++ (l.pend.flatten ++ l.tail) := by
  simp [Layer.cells, hp, List.append_assoc]

theorem erase_vals {a b : List (Cell C)} (h : a.map Cell.erase = b.map Cell.erase) : a.map Cell.val = b.map Cell.val := by
  have := congrArg (List.map Cell.val) h; simpa [List.map_map, Function.comp_def] using this

structure LayerLE (l l' : Layer C) : Prop where
  erase : l'.erase = l.erase
  holes : HolesSub l.cells l'.cells

theorem LayerLE.of_view {s s' : St C} (hv : s'.view = s.view) {l : Layer C} {ls : List (Layer C)}
    (h : s.stack = l :: ls) (hc : HolesSub s.cells s'.cells) :
    ∃ l' ls', s'.stack = l' :: ls' ∧ LayerLE l l' ∧ ls'.map Layer.erase = ls.map Layer.erase := by
  obtain ⟨l', ls', h', he, hle⟩ := stack_of_view hv h
  exact ⟨l', ls', h', ⟨he, layer_holesSub hv h h' hc⟩, hle⟩

theorem LayerLE.refl (l : Layer C) : LayerLE l l := ⟨rfl, HolesSub.refl _⟩

section
variable {l l' : Layer C}

theorem LayerLE.nodes_map (h : LayerLE l l') : l'.nodes.map Node.erase = l.nodes.map Node.erase := by
  have := congrArg Layer.nodes h.erase; exact this

theorem LayerLE.pend_map (h : LayerLE l l') : l'.pend.map (·.map Cell.erase) = l.pend.map (·.map Cell.erase) := by
  have := congrArg Layer.pend h.erase; exact this

theorem LayerLE.tail_map (h : LayerLE l l') : l'.tail.map Cell.erase = l.tail.map Cell.erase := by
  have := congrArg Layer.tail h.erase; exact this

theorem LayerLE.prog_map (h : LayerLE l l') : l'.prog.map Cell.erase = l.prog.map Cell.erase := by
  have := congrArg Layer.prog h.erase; exact this

theorem LayerLE.prog_nil (h : LayerLE l l') (hp : l.prog = []) : l'.prog = [] := by
  have := h.prog_map; rw [hp] at this; simpa using this

theorem LayerLE.nodes_length (h : LayerLE l l') : l'.nodes.length = l.nodes.length := by
  have := congrArg List.length h.nodes_map; simpa using this

theorem LayerLE.pendC (h : LayerLE l l') : l'.pendC = l.pendC := by
  have := congrArg Layer.pendC h.erase; exact this

theorem LayerLE.flag_incremental (h : LayerLE l l') : l'.incremental = l.incremental := by
  have := congrArg Layer.incremental h.erase; exact this

theorem LayerLE.flag_progressive (h : LayerLE l l') : l'.progressive = l.progressive := by
  have := congrArg Layer.progressive h.erase; exact this

theorem LayerLE.flag_collapsed (h : LayerLE l l') : l'.collapsed = l.collapsed := by
  have := congrArg Layer.collapsed h.erase; exact this

theorem LayerLE.flag_legacy (h : LayerLE l l') : l'.legacy = l.legacy := by
  have := congrArg Layer.legacy h.erase; exact this

theorem LayerLE.trans {l1 l2 l3 : Layer C} (h1 : LayerLE l1 l2) (h2 : LayerLE l2 l3) : LayerLE l1 l3 :=
  ⟨h2.erase.trans h1.erase, HolesSub.trans h1.holes h2.holes⟩

theorem erase_map_length {a b : List (Cell C)} (h : a.map Cell.erase = b.map Cell.erase) : a.length = b.length := by
  have := congrArg List.length h; simpa using this

theorem pend_flatten_length {a b : List (List (Cell C))} (h : a.map (·.map Cell.erase) = b.map (·.map Cell.erase)) :
    a.flatten.length = b.flatten.length := by
  have := congrArg (fun x => x.flatten.length) h
  simpa [List.length_flatten, List.map_map, Function.comp_def] using this

theorem LayerLE.split_holes (h : LayerLE l l') (hp : l.prog = []) :
    HolesSub (nodeCells l.nodes) (nodeCells l'.nodes) ∧
    HolesSub (l.pend.flatten ++ l.tail) (l'.pend.flatten ++ l'.tail) := by
  have hc := h.holes
  rw [cells_split l hp, cells_split l' (h.prog_nil hp)] at hc
  have hl : (nodeCells l.nodes).length = (nodeCells l'.nodes).length := by
    simp [nodeCells_length, h.nodes_length]
  exact ⟨holesSub_prefix hl hc, holesSub_suffix hl hc⟩

/-- A hole among the nodes afterwards was a hole of a node of the same depth. -/
theorem LayerLE.node_hole (h : LayerLE l l') (hp : l.prog = []) :
    ∀ n' ∈ l'.nodes, n'.cell.isReady = false → ∃ n ∈ l.nodes, n.cell.isReady = false ∧ n.depth = n'.depth := by
  intro n' hn' hr
  obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hn'
  have hi0 : i < l.nodes.length := by rw [← h.nodes_length]; exact hi
  obtain ⟨c0, h0, h0r⟩ := (h.split_holes hp).1 i (l'.nodes[i]).cell
    (by simp [nodeCells, List.getElem?_map, List.getElem?_eq_getElem hi]) hr
  simp only [nodeCells, List.getElem?_map, List.getElem?_eq_getElem hi0, Option.map_some, Option.some.injEq] at h0
  refine ⟨l.nodes[i], List.getElem_mem _, by rw [h0]; exact h0r, ?_⟩
  have := congrArg (fun xs => xs[i]?) h.nodes_map
  simp only [List.getElem?_map, List.getElem?_eq_getElem hi, List.getElem?_eq_getElem hi0, Option.map_some,
    Option.some.injEq] at this
  have := congrArg Node.depth this
  exact this.symm

/-- Ready deferred cells stay ready. -/
theorem LayerLE.rest_ready (h : LayerLE l l') (hp : l.prog = [])
    (hr : ∀ c ∈ l.pend.flatten ++ l.tail, c.isReady = true) : ∀ c ∈ l'.pend.flatten ++ l'.tail, c.isReady = true := by
  intro c hc
  cases hcr : c.isReady with
  | true => rfl
  | false =>
    exfalso
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hc
    obtain ⟨c0, h0, h0r⟩ := (h.split_holes hp).2 i _ (List.getElem?_eq_getElem hi) hcr
    rw [hr c0 (List.mem_of_getElem? h0)] at h0r; cases h0r

theorem LayerLE.binOK_settled (cx : Ctx C) (h : LayerLE l l') {L : List C} (hb : BinOK cx l.settled L) :
    BinOK cx l'.settled L := by
  have hp : l.prog = [] := hb.prog
  apply binOK_of_erase cx hb
  · show ({ l'.erase with pend := [], pendC := 0, tail := [] } : Layer C) = { l.erase with pend := [], pendC := 0, tail := [] }
    rw [h.erase]
  · intro n hn hr
    obtain ⟨m, hm, hmr, hmd⟩ := h.node_hole hp n hn hr
    rw [← hmd]; exact hb.holes m hm hmr

theorem LayerLE.pendLeaves (cx : Ctx C) (h : LayerLE l l') : pendLeaves cx l' = pendLeaves cx l := by
  have hp : l'.pend.map (·.map Cell.val) = l.pend.map (·.map Cell.val) := by
    have := congrArg (List.map (List.map Cell.val)) h.pend_map
    simpa [List.map_map, Function.comp_def] using this
  have ht := erase_vals h.tail_map
  unfold Ssz.Hasher.pendLeaves
  rw [ht]
  have : l'.pend.map (fun e => reduceElem cx.H cx.z0 (e.map Cell.val)) =
      (l'.pend.map (·.map Cell.val)).map (reduceElem cx.H cx.z0) := by simp [List.map_map, Function.comp_def]
  rw [this, hp]; simp [List.map_map, Function.comp_def]

theorem LayerLE.pendOK (cx : Ctx C) (h : LayerLE l l') (hp : l.prog = []) (hpo : PendOK cx l) : PendOK cx l' := by
  have hpe : l'.pend = [] ↔ l.pend = [] := by
    have := h.pend_map
    constructor
    · intro e; rw [e] at this; simpa using this.symm
    · intro e; rw [e] at this; simpa using this
  have hlens : l'.pend.map List.length = l.pend.map List.length := by
    have := congrArg (List.map List.length) h.pend_map
    simpa [List.map_map, Function.comp_def] using this
  have hall : ∀ c ∈ l'.pend.flatten ++ l'.tail, c.isReady = true := h.rest_ready hp (by
    intro c hc
    rcases List.mem_append.mp hc with hc | hc
    · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc; exact hpo.ready e he c hce
    · exact hpo.tready c hc)
  refine ⟨?_, ?_, ?_, ?_, ?_⟩
  · intro e
    have := h.tail_map; rw [hpo.tail (hpe.mp e)] at this; simpa using this
  · intro e he c hc; exact hall c (List.mem_append_left _ (List.mem_flatten.mpr ⟨e, he, hc⟩))
  · intro c hc; exact hall c (List.mem_append_right _ hc)
  · intro e he
    rw [h.pendC]
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem he
    have hi0 : i < l.pend.length := by have := congrArg List.length hlens; simp at this; omega
    have := congrArg (fun xs => xs[i]?) hlens
    simp only [List.getElem?_map, List.getElem?_eq_getElem hi, List.getElem?_eq_getElem hi0, Option.map_some,
      Option.some.injEq] at this
    rw [this]; exact hpo.width _ (List.getElem_mem _)
  · intro hne; rw [h.pendC]; exact hpo.pow (fun e => hne (hpe.mpr e))

/-- **A scope's invariant survives drains.** -/
theorem LayerLE.scopeOK (cx : Ctx C) (h : LayerLE l l') {L : List C} (hs : ScopeOK cx l L) : ScopeOK cx l' L := by
  obtain ⟨L1, hL, hb, hpo⟩ := hs
  exact ⟨L1, by rw [h.pendLeaves cx]; exact hL, h.binOK_settled cx hb, h.pendOK cx hb.prog hpo⟩

end

end Ssz.Hasher