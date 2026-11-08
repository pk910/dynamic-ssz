import SszCert.Hasher.Top

/-!
# Binary scopes

An incremental binary scope (`StartTree(TreeTypeBinary)`): leaves are
appended, `Collapse` hints batch-collapse it (synchronously) or pre-collapse
cap-wide runs into background jobs (asynchronously), and `Merkleize` or
`MerkleizeWithMixin` close it. `BinOK` is the scope's invariant; each call
keeps it, and the close yields the specified root.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- The invariant of an open binary scope whose leaves so far are `L`. -/
structure BinOK (cx : Ctx C) (l : Layer C) (L : List C) : Prop where
  ninc : l.incremental = false → l.collapsed = false
  noprog : l.progressive = false
  nolegacy : l.legacy = false
  prog : l.prog = []
  pend : l.pend = []
  tail : l.tail = []
  cov : Cov cx.H cx.z0 true l.nodes L
  leaves : l.collapsed = false → ∀ n ∈ l.nodes, n.depth = 0
  shallow : ∀ n ∈ l.nodes, n.depth < 40
  holes : ∀ n ∈ l.nodes, n.cell.isReady = false → 0 < n.depth
  nonempty : l.collapsed = true → l.nodes ≠ []

/-- What the calls need of the rest of the state. -/
structure TopOK (s : St C) : Prop where
  holes : HolesOK s
  err : s.err = none
  jobsne : ∀ j ∈ s.jobs, j.out ≠ []
  ids : ∀ j ∈ s.jobs, j.id < s.nextJob

theorem job_end_le (s : St C) (hok : HolesOK s) (j : Job C) (hj : j ∈ s.jobs) (hne : j.out ≠ []) :
    j.dst + j.out.length ≤ s.cells.length := by
  have hpos : 0 < j.out.length := List.length_pos_iff.mpr hne
  obtain ⟨idx, hidx⟩ := hok.inplace j hj (j.out.length - 1) (by omega)
  have : j.dst + (j.out.length - 1) < s.cells.length := by
    rcases Nat.lt_or_ge (j.dst + (j.out.length - 1)) s.cells.length with h | h
    · exact h
    · rw [List.getElem?_eq_none h] at hidx; cases hidx
  omega

theorem layer_cells_bin (l : Layer C) (hp : l.prog = []) (hd : l.pend = []) (ht : l.tail = []) :
    l.cells = nodeCells l.nodes := by
  simp [Layer.cells, hp, hd, ht]

theorem St.push_eq_setTop (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hp : l.pend = []) (c : Cell C) :
    s.push c = s.setTop { l with nodes := l.nodes ++ [⟨0, c⟩] } := by
  simp [St.push, St.setTop, h, hp]

/-- **Appending a leaf** to a binary scope. -/
theorem push_bin (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) (v : C)
    (h : s.stack = l :: ls) (hb : BinOK cx l L) (ht : TopOK s) :
    (s.push (.ready v)).stack = { l with nodes := l.nodes ++ [⟨0, .ready v⟩] } :: ls ∧
    BinOK cx { l with nodes := l.nodes ++ [⟨0, .ready v⟩] } (L ++ [v]) ∧
    TopOK (s.push (.ready v)) ∧ (s.push (.ready v)).below = s.below ∧
    (s.push (.ready v)).hashErr = s.hashErr := by
  rw [St.push_eq_setTop s l ls h hb.pend]
  have hcl : l.cells = nodeCells l.nodes := layer_cells_bin l hb.prog hb.pend hb.tail
  have hcl' : ({ l with nodes := l.nodes ++ [⟨0, .ready v⟩] } : Layer C).cells = l.cells ++ [.ready v] := by
    rw [layer_cells_bin ({ l with nodes := l.nodes ++ [⟨0, .ready v⟩] } : Layer C) hb.prog hb.pend hb.tail, hcl]
    simp [nodeCells]
  refine ⟨St.setTop_stack s l _ ls h, ⟨hb.ninc, hb.noprog, hb.nolegacy, hb.prog, hb.pend, hb.tail,
    cov_push cx.H cx.z0 hb.cov (.ready v), ?_, ?_, ?_, fun _ => by simp⟩, ⟨?_, ?_, ?_, ?_⟩,
    St.setTop_below s l _ ls h, ?_⟩
  · intro hc n hn
    rcases List.mem_append.mp hn with hn | hn
    · exact hb.leaves hc n hn
    · simp at hn; rw [hn]
  · intro n hn
    rcases List.mem_append.mp hn with hn | hn
    · exact hb.shallow n hn
    · simp at hn; rw [hn]; show 0 < 40; decide
  · intro n hn hr
    rcases List.mem_append.mp hn with hn | hn
    · exact hb.holes n hn hr
    · simp at hn; rw [hn] at hr; simp [Cell.isReady] at hr
  · apply holesOK_setTop s l _ ls h ht.holes l.cells.length (Nat.le_refl _)
    · rw [hcl', List.take_append_of_le_length (Nat.le_refl _)]
    · rw [hcl']; simp
    · intro i hi hi'
      have hi2 : i = l.cells.length := by rw [hcl'] at hi'; simp at hi'; omega
      subst hi2
      simp only [hcl', List.getElem_append_right (Nat.le_refl _), Nat.sub_self, List.getElem_cons_zero]
      rfl
    · intro j hj
      have := job_end_le s ht.holes j hj (ht.jobsne j hj)
      rw [St.cells_top s l ls h, List.length_append] at this
      exact this
  · rw [St.setTop_err]; exact ht.err
  · intro j hj; rw [St.setTop_jobs] at hj; exact ht.jobsne j hj
  · intro j hj; rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj
  · unfold St.setTop; rw [h]

/-! ## Drains before a reduction -/

theorem cov_erase {full : Bool} {ns : List (Node C)} {L : List C} (H : C → C → C) (z0 : C) :
    Cov H z0 full (ns.map Node.erase) L ↔ Cov H z0 full ns L := by
  induction ns generalizing L with
  | nil => rfl
  | cons n ns ih =>
    cases ns with
    | nil => exact Iff.rfl
    | cons m ns =>
      simp only [List.map_cons]
      constructor
      · rintro ⟨xs, ys, hL, hx, hv, hd, hc⟩
        exact ⟨xs, ys, hL, hx, hv, hd, (ih (L := ys)).mp (by simpa using hc)⟩
      · rintro ⟨xs, ys, hL, hx, hv, hd, hc⟩
        exact ⟨xs, ys, hL, hx, hv, hd, by simpa using (ih (L := ys)).mpr hc⟩

/-- Nodes whose erasures agree cover the same leaves. -/
theorem cov_of_erase_eq {full : Bool} {ns ns' : List (Node C)} {L : List C} (H : C → C → C) (z0 : C)
    (he : ns'.map Node.erase = ns.map Node.erase) (h : Cov H z0 full ns L) : Cov H z0 full ns' L := by
  rw [← cov_erase H z0, he, cov_erase H z0]; exact h

theorem le_sum_of_mem' {xs : List Nat} {x : Nat} (h : x ∈ xs) : x ≤ xs.sum := by
  induction xs with
  | nil => simp at h
  | cons y ys ih =>
    rcases List.mem_cons.mp h with rfl | h
    · simp
    · simp; have := ih h; omega

/-- A leaf count below `2^40` keeps every node of a full cover below depth 40. -/
theorem cov_depth_lt {ns : List (Node C)} {L : List C} (H : C → C → C) (z0 : C)
    (h : Cov H z0 true ns L) (hL : L.length < 2 ^ 40) : ∀ n ∈ ns, n.depth < 40 := by
  intro n hn
  have hlen := cov_full_length H z0 h
  have : 2 ^ n.depth ≤ leavesOf ns := by
    unfold leavesOf
    exact le_sum_of_mem' (List.mem_map.mpr ⟨n, hn, rfl⟩)
  have h2 : 2 ^ n.depth < 2 ^ 40 := by omega
  exact (Nat.pow_lt_pow_iff_right (by omega)).mp h2

/-- After `drainJobsFor(p)`, no cell at or after `p` is a hole. -/
theorem drainFor_no_holes (s : St C) (hok : HolesOK s) (p : Nat) :
    ∀ i (_ : p ≤ i) (hi' : i < (s.drainFor p).cells.length), ((s.drainFor p).cells[i]'hi').isReady = true := by
  intro i hi hi'
  unfold St.drainFor at hi' ⊢
  split
  · rename_i hc
    obtain ⟨_, _, _, _, _, hnone, _⟩ := drainAll_spec s hok
    split at hi'
    · cases hc' : s.drainAll.cells[i] with
      | ready v => rfl
      | hole v id idx =>
        exfalso; apply hnone i v id idx
        rw [List.getElem?_eq_getElem hi', hc']
    · contradiction
  · rename_i hc
    split at hi'
    · contradiction
    · cases hc' : s.cells[i] with
      | ready v => rfl
      | hole v id idx =>
        exfalso
        have hget : s.cells[i]? = some (.hole v id idx) := by rw [List.getElem?_eq_getElem hi', hc']
        obtain ⟨j, hj, _, h1, h2⟩ := hok.owned i v id idx hget
        have := hok.maxEnd j hj
        apply hc
        refine ⟨?_, by omega⟩
        intro e; rw [e] at hj; simp at hj

/-- With the innermost region free of holes, every job lies below it. -/
theorem jobs_below (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) (ht : TopOK s)
    (hr : ∀ c ∈ l.cells, c.isReady = true) : ∀ j ∈ s.jobs, j.dst + j.out.length ≤ s.below.length := by
  intro j hj
  have hne := ht.jobsne j hj
  have hpos : 0 < j.out.length := List.length_pos_iff.mpr hne
  obtain ⟨idx, hidx⟩ := ht.holes.inplace j hj (j.out.length - 1) (by omega)
  rw [St.cells_top s l ls h] at hidx
  rcases Nat.lt_or_ge (j.dst + (j.out.length - 1)) s.below.length with hlt | hge
  · omega
  · exfalso
    rw [List.getElem?_append_right hge] at hidx
    have hm : Cell.hole (j.out[j.out.length - 1]'(by omega)) j.id idx ∈ l.cells := List.mem_of_getElem? hidx
    have := hr _ hm
    simp [Cell.isReady] at this

/-- **Replacing a hole-free innermost region** by another hole-free one keeps
    the rest of the state's invariants. -/
theorem topOK_setTop_ready (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (ht : TopOK s) (hr : ∀ c ∈ l.cells, c.isReady = true) (hr' : ∀ c ∈ l'.cells, c.isReady = true) :
    TopOK (s.setTop l') := by
  refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
    fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩
  apply holesOK_setTop s l l' ls h ht.holes 0 (Nat.zero_le _) (by simp) (Nat.zero_le _)
  · intro i _ hi'; exact hr' _ (List.getElem_mem hi')
  · intro j hj; have := jobs_below s l ls h ht hr j hj; omega

/-! ## Batch collapse on a hole-free region -/

/-- The group a batch round pairs is the depth's group. -/
theorem batchGroup_all (P : Params) (d : Nat) (ns : List (Node C)) (L : List C) (H : C → C → C) (z0 : C)
    (hcov : Cov H z0 true ns L) : (batchGroup P d ns).all (·.depth == d) = true := by
  obtain ⟨A, G, B, rfl, hA, hG, hB⟩ := cov_split3 H z0 d hcov
  have hdc : deeperCount d (A ++ G ++ B) = A.length := filter_gt_split hA hG hB
  have hbs : batchSize P d (A ++ G ++ B) = (G.length / P.batch) * P.batch := by
    unfold batchSize; rw [filter_eq_split hA hG hB]
  have hbcle : (G.length / P.batch) * P.batch ≤ G.length := Nat.div_mul_le_self _ _
  unfold batchGroup
  rw [hdc, hbs, show (A ++ G ++ B).drop A.length = G ++ B by simp,
    List.take_append_of_le_length hbcle, List.all_eq_true]
  intro n hn; simp [hG n (List.mem_of_mem_take hn)]

/-- Reading ready cells leaves the state alone. -/
theorem vals_ready (s : St C) (cs : List (Cell C)) (w : String) (hr : ∀ c ∈ cs, c.isReady = true) :
    (s.vals cs w).1 = s := by
  simp only [St.vals]
  rw [if_pos (List.all_eq_true.mpr hr)]

theorem pairNodes_ready (cx : Ctx C) (s : St C) (ns : List (Node C)) (hr : ∀ n ∈ ns, n.cell.isReady = true) :
    (s.pairNodes cx ns).1 = s ∧ ∀ n ∈ (s.pairNodes cx ns).2, n.cell.isReady = true := by
  refine ⟨?_, ?_⟩
  · simp only [St.pairNodes]
    exact vals_ready s _ _ (fun c hc => by
      simp only [nodeCells, List.mem_map] at hc; obtain ⟨n, hn, rfl⟩ := hc; exact hr n hn)
  · intro n hn
    simp only [St.pairNodes, List.mem_map] at hn
    obtain ⟨_, _, rfl⟩ := hn; rfl

/-- **A batch round on a hole-free cover** changes no state and keeps the
    region hole-free. -/
theorem batchStep_ready (cx : Ctx C) (d : Nat) (s : St C) (ns : List (Node C)) (L : List C)
    (hcov : Cov cx.H cx.z0 true ns L) (hr : ∀ n ∈ ns, n.cell.isReady = true) :
    (batchStep cx d s ns).1 = s ∧ ∀ n ∈ (batchStep cx d s ns).2, n.cell.isReady = true := by
  have hg := batchGroup_all cx.P d ns L cx.H cx.z0 hcov
  have hgr : ∀ n ∈ batchGroup cx.P d ns, n.cell.isReady = true := by
    intro n hn; exact hr n (List.mem_of_mem_drop (List.mem_of_mem_take hn))
  refine ⟨?_, ?_⟩
  · unfold batchStep
    simp only [St.failUnless, hg, if_true]
    exact (pairNodes_ready cx s _ hgr).1
  · intro n hn
    unfold batchStep at hn
    simp only [List.mem_append] at hn
    rcases hn with (hn | hn) | hn
    · exact hr n (List.mem_of_mem_take hn)
    · exact (pairNodes_ready cx s _ hgr).2 n hn
    · exact hr n (List.mem_of_mem_drop (List.mem_of_mem_drop hn))

theorem batchLoop_ready (cx : Ctx C) (hb : 0 < cx.P.batch) (hb2 : cx.P.batch % 2 = 0) :
    ∀ fuel d (s : St C) (ns : List (Node C)) (L : List C), Cov cx.H cx.z0 true ns L →
      (∀ n ∈ ns, n.cell.isReady = true) →
      (batchLoop cx fuel d s ns).1 = s ∧ ∀ n ∈ (batchLoop cx fuel d s ns).2, n.cell.isReady = true := by
  intro fuel
  induction fuel with
  | zero => intro d s ns L _ hr; exact ⟨rfl, hr⟩
  | succ fuel ih =>
    intro d s ns L hc hr
    simp only [batchLoop]
    split
    · exact ⟨rfl, hr⟩
    · rename_i hcnt
      obtain ⟨h1, h2⟩ := batchStep_ready cx d s ns L hc hr
      have hc' := batchStep_cov cx d s ns L hc hb hb2 (by omega)
      obtain ⟨i1, i2⟩ := ih (d + 1) (batchStep cx d s ns).1 _ L hc' h2
      refine ⟨?_, i2⟩
      rw [i1, h1]

/-! ## `drainJobsFor` -/

/-- `drainJobsFor` keeps the view, the invariants and the errors. -/
theorem drainFor_spec (s : St C) (ht : TopOK s) (p : Nat) :
    (s.drainFor p).view = s.view ∧ TopOK (s.drainFor p) ∧ (s.drainFor p).hashErr = s.hashErr ∧
    (s.drainFor p).cells.length = s.cells.length := by
  unfold St.drainFor
  split
  · obtain ⟨h1, h2, h3, h4, h5, _, h7⟩ := drainAll_spec s ht.holes
    exact ⟨h1, ⟨h3, by rw [h4]; exact ht.err, fun j hj => by rw [h2] at hj; simp at hj,
      fun j hj => by rw [h2] at hj; simp at hj⟩, h5, h7⟩
  · exact ⟨rfl, ht, rfl, rfl⟩

theorem drainFor_holesSub (s : St C) (hok : HolesOK s) (p : Nat) : HolesSub s.cells (s.drainFor p).cells := by
  unfold St.drainFor; split
  · exact drainAll_holesSub s hok
  · exact HolesSub.refl _

theorem view_stack {s s' : St C} (h : s'.view = s.view) : s'.stack.map Layer.erase = s.stack.map Layer.erase :=
  congrArg Prod.snd h

theorem map_erase_nil {xs : List (Cell C)} (h : xs.map Cell.erase = ([] : List (Cell C)).map Cell.erase) : xs = [] := by
  simpa using h

/-- The scope invariant depends only on the erased layer (and its holes). -/
theorem binOK_of_erase (cx : Ctx C) {l l' : Layer C} {L : List C} (hb : BinOK cx l L) (he : l'.erase = l.erase)
    (hh : ∀ n ∈ l'.nodes, n.cell.isReady = false → 0 < n.depth) : BinOK cx l' L := by
  have hinc : l'.incremental = l.incremental := by have := congrArg Layer.incremental he; exact this
  have hpr : l'.progressive = l.progressive := by have := congrArg Layer.progressive he; exact this
  have hle : l'.legacy = l.legacy := by have := congrArg Layer.legacy he; exact this
  have hco : l'.collapsed = l.collapsed := by have := congrArg Layer.collapsed he; exact this
  have hp : l'.prog.map Cell.erase = l.prog.map Cell.erase := by have := congrArg Layer.prog he; exact this
  have hd : l'.pend.map (·.map Cell.erase) = l.pend.map (·.map Cell.erase) := by have := congrArg Layer.pend he; exact this
  have hta : l'.tail.map Cell.erase = l.tail.map Cell.erase := by have := congrArg Layer.tail he; exact this
  have hn : l'.nodes.map Node.erase = l.nodes.map Node.erase := by have := congrArg Layer.nodes he; exact this
  have hdep : ∀ n ∈ l'.nodes, ∃ m ∈ l.nodes, m.depth = n.depth := by
    intro n hn'
    have : n.erase ∈ l.nodes.map Node.erase := by rw [← hn]; exact List.mem_map_of_mem hn'
    obtain ⟨m, hm, he'⟩ := List.mem_map.mp this
    exact ⟨m, hm, by have := congrArg Node.depth he'; exact this⟩
  refine ⟨fun hi => by rw [hco]; exact hb.ninc (by rw [← hinc]; exact hi), by rw [hpr]; exact hb.noprog, by rw [hle]; exact hb.nolegacy,
    ?_, ?_, ?_, cov_of_erase_eq cx.H cx.z0 hn hb.cov, ?_, ?_, hh, ?_⟩
  · rw [hb.prog] at hp; exact map_erase_nil hp
  · rw [hb.pend] at hd; simpa using hd
  · rw [hb.tail] at hta; exact map_erase_nil hta
  · intro hc n hn'; obtain ⟨m, hm, hmd⟩ := hdep n hn'; rw [← hmd]; exact hb.leaves (by rw [← hco]; exact hc) m hm
  · intro n hn'; obtain ⟨m, hm, hmd⟩ := hdep n hn'; rw [← hmd]; exact hb.shallow m hm
  · intro hc e
    have := congrArg List.length hn
    rw [e] at this; simp at this
    exact hb.nonempty (by rw [← hco]; exact hc) (List.eq_nil_of_length_eq_zero this.symm)

/-! ## Synchronous `Collapse` -/

theorem Layer.erase_cells (l : Layer C) : l.erase.cells = l.cells.map Cell.erase := by
  simp [Layer.erase, Layer.cells, nodeCells, Node.erase, List.map_flatten, List.map_map, Function.comp_def]

theorem below_length_of_view {s s' : St C} (h : s'.view = s.view) {l l' : Layer C} {ls ls' : List (Layer C)}
    (hs : s.stack = l :: ls) (hs' : s'.stack = l' :: ls') : s'.below.length = s.below.length := by
  have hb : s'.base.map Cell.erase = s.base.map Cell.erase := congrArg Prod.fst h
  have hst := view_stack h
  rw [hs, hs'] at hst
  simp only [List.map_cons, List.cons.injEq] at hst
  unfold St.below
  rw [hs, hs']
  simp only [List.drop_one, List.tail_cons, List.length_append, List.length_flatten, List.map_reverse,
    List.map_map, Function.comp_def]
  have := congrArg List.length hb
  simp only [List.length_map] at this
  rw [this, List.sum_reverse, List.sum_reverse]
  have e : ∀ (xs : List (Layer C)), (xs.map fun x => x.cells.length) = (xs.map fun x => x.erase.cells.length) := by
    intro xs; simp [Layer.erase_cells]
  rw [e ls', e ls]
  have h2 : (ls'.map fun x => x.erase.cells.length) = (ls.map fun x => x.erase.cells.length) := by
    have := congrArg (List.map fun l : Layer C => l.cells.length) hst.2
    simpa [List.map_map, Function.comp_def] using this
  rw [h2]

/-- Holes of the cells restrict to holes of the region below the innermost scope. -/
theorem holesSub_below {s s' : St C} (h : s'.view = s.view) {l l' : Layer C} {ls ls' : List (Layer C)}
    (hs : s.stack = l :: ls) (hs' : s'.stack = l' :: ls') (hc : HolesSub s.cells s'.cells) :
    HolesSub s.below s'.below := by
  rw [St.cells_top s l ls hs, St.cells_top s' l' ls' hs'] at hc
  exact holesSub_prefix (below_length_of_view h hs hs').symm hc

theorem uncollapsedAsLeaves_state (s : St C) (l : Layer C)
    (h : l.collapsed = false → ∀ n ∈ l.nodes, n.depth = 0) : (s.uncollapsedAsLeaves l).1 = s := by
  unfold St.uncollapsedAsLeaves
  split
  · rfl
  · rename_i hc
    simp only
    rw [if_neg]
    intro hany
    obtain ⟨n, hn, hd⟩ := List.any_eq_true.mp hany
    have := h (by simpa using hc) n hn
    simp [this] at hd

/-- The stack of a state whose view equals another's. -/
theorem stack_of_view {s s' : St C} (h : s'.view = s.view) {l : Layer C} {ls : List (Layer C)}
    (hs : s.stack = l :: ls) :
    ∃ l' ls', s'.stack = l' :: ls' ∧ l'.erase = l.erase ∧ ls'.map Layer.erase = ls.map Layer.erase := by
  have hst := view_stack h
  rw [hs] at hst
  cases hs' : s'.stack with
  | nil => rw [hs'] at hst; simp at hst
  | cons l' ls' =>
    rw [hs'] at hst
    simp only [List.map_cons, List.cons.injEq] at hst
    exact ⟨l', ls', rfl, hst.1, hst.2⟩

/-- **Synchronous `Collapse` on a binary scope** keeps every invariant: the
    scope still covers its leaves, the rest of the state keeps its view. -/
theorem collapseBinary_bin (cx : Ctx C) (hb0 : 0 < cx.P.batch) (hb2 : cx.P.batch % 2 = 0)
    (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hb : BinOK cx l L) (hinc : l.incremental = true) (ht : TopOK s)
    (hL : L.length < 2 ^ 40) :
    ∃ l' ls', (s.collapseBinary cx).stack = l' :: ls' ∧ BinOK cx l' L ∧ l'.incremental = true ∧
      TopOK (s.collapseBinary cx) ∧
      ls'.map Layer.erase = ls.map Layer.erase ∧
      (s.collapseBinary cx).base.map Cell.erase = s.base.map Cell.erase ∧
      (s.collapseBinary cx).hashErr = s.hashErr ∧ HolesSub s.below (s.collapseBinary cx).below := by
  unfold St.collapseBinary
  rw [h]
  simp only [hb.pend, ne_eq, not_true_eq_false, if_false]
  split
  · exact ⟨l, ls, h, hb, hinc, ht, rfl, rfl, rfl, HolesSub.refl _⟩
  rename_i hd0
  -- the drain before the rounds
  obtain ⟨hv, ht2, hh2, _⟩ := drainFor_spec s ht s.activeStart
  obtain ⟨l2, ls2, hs2, he2, hle2⟩ := stack_of_view hv h
  have hact : s.activeStart = s.below.length := by
    unfold St.activeStart; rw [h]; simp only; rw [hb.prog, St.topStart_eq s l ls h]; rfl
  have hbl := below_length_of_view hv h hs2
  have hr2 : ∀ c ∈ l2.cells, c.isReady = true := by
    intro c hc
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hc
    have hi' : (s.drainFor s.activeStart).below.length + i < (s.drainFor s.activeStart).cells.length := by
      rw [St.cells_top _ l2 ls2 hs2, List.length_append]; omega
    have := drainFor_no_holes s ht.holes s.activeStart _ (by omega) hi'
    have hget : (s.drainFor s.activeStart).cells[(s.drainFor s.activeStart).below.length + i]? = some (l2.cells[i]) := by
      rw [St.cells_top _ l2 ls2 hs2, List.getElem?_append_right (by omega), Nat.add_sub_cancel_left,
        List.getElem?_eq_getElem hi]
    rw [List.getElem?_eq_getElem hi'] at hget
    simp only [Option.some.injEq] at hget
    rw [← hget]; exact this
  have hb2' : BinOK cx l2 L := binOK_of_erase cx hb he2 (by
    intro n hn hnr
    have : n.cell ∈ l2.cells := by
      simp only [Layer.cells, List.mem_append, nodeCells, List.mem_map]
      exact Or.inl (Or.inl (Or.inr ⟨n, hn, rfl⟩))
    rw [hr2 _ this] at hnr; cases hnr)
  have hn2r : ∀ n ∈ l2.nodes, n.cell.isReady = true := by
    intro n hn
    apply hr2
    simp only [Layer.cells, List.mem_append, nodeCells, List.mem_map]
    exact Or.inl (Or.inl (Or.inr ⟨n, hn, rfl⟩))
  -- the rounds
  unfold St.collapseBinaryCore
  rw [hs2]
  simp only
  rw [uncollapsedAsLeaves_state _ l2 hb2'.leaves]
  have hnodes := uncollapsedAsLeaves_nodes (s.drainFor s.activeStart) l2 hb2'.leaves
  rw [hnodes]
  obtain ⟨hst, hnr⟩ := batchLoop_ready cx hb0 hb2 64 0 (s.drainFor s.activeStart) l2.nodes L hb2'.cov hn2r
  have hcov := batchLoop_cov cx hb0 hb2 64 0 (s.drainFor s.activeStart) l2.nodes L hb2'.cov
  rw [hst]
  have hfin : ∀ ns : List (Node C), ({ ((s.drainFor s.activeStart).uncollapsedAsLeaves l2).2 with
      collapsed := true, nodes := ns, counted := ns.length } : Layer C) =
      { l2 with collapsed := true, nodes := ns, counted := ns.length } := by
    intro ns; rw [uncollapsedAsLeaves_layer]
  rw [hfin]
  generalize hns : (batchLoop cx 64 0 (s.drainFor s.activeStart) l2.nodes).2 = ns at hnr hcov
  have hinc2 : l2.incremental = true := (congrArg Layer.incremental he2).trans hinc
  refine ⟨_, ls2, St.setTop_stack _ l2 _ ls2 hs2,
    ⟨fun hi => absurd (show l2.incremental = false from hi) (by rw [hinc2]; decide), hb2'.noprog, hb2'.nolegacy, hb2'.prog, hb2'.pend,
    hb2'.tail, hcov, by simp, cov_depth_lt cx.H cx.z0 hcov hL,
    (fun n hn hnr' => by rw [hnr n hn] at hnr'; cases hnr'), fun _ => ?_⟩, hinc2,
    ?_, hle2, ?_, ?_, ?_⟩
  · -- a batch ran, so the scope had nodes and has them still
    intro e
    have e' : ns = [] := e
    rw [e'] at hcov
    have hL0 : L = [] := hcov
    have hpos : 0 < l.nodes.length := by
      have : 0 < d0Chunks l := by omega
      unfold d0Chunks at this; omega
    have := cov_ne_nil cx.H cx.z0 hb.cov (List.ne_nil_of_length_pos hpos)
    exact this hL0
  · apply topOK_setTop_ready _ l2 _ ls2 hs2 ht2 hr2
    intro c hc
    simp only [Layer.cells, hb2'.prog, hb2'.pend, hb2'.tail, List.nil_append, List.flatten_nil,
      List.append_nil, nodeCells, List.mem_map] at hc
    obtain ⟨n, hn, rfl⟩ := hc
    exact hnr n hn
  · have := congrArg Prod.fst hv
    simp only [St.view] at this
    show ((s.drainFor s.activeStart).setTop _).base.map Cell.erase = s.base.map Cell.erase
    unfold St.setTop; rw [hs2]; exact this
  · show ((s.drainFor s.activeStart).setTop _).hashErr = s.hashErr
    unfold St.setTop; rw [hs2]; exact hh2
  · rw [St.setTop_below _ l2 _ ls2 hs2]
    exact holesSub_below hv h hs2 (drainFor_holesSub s ht.holes _)

end Ssz.Hasher
