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
  inc : l.incremental = true
  noprog : l.progressive = false
  nolegacy : l.legacy = false
  prog : l.prog = []
  pend : l.pend = []
  tail : l.tail = []
  cov : Cov cx.H cx.z0 true l.nodes L
  leaves : l.collapsed = false → ∀ n ∈ l.nodes, n.depth = 0
  shallow : ∀ n ∈ l.nodes, n.depth < 40
  holes : ∀ n ∈ l.nodes, n.cell.isReady = false → n.depth = cx.P.capDepth

/-- What the calls need of the rest of the state. -/
structure TopOK (s : St C) : Prop where
  holes : HolesOK s
  err : s.err = none
  jobsne : ∀ j ∈ s.jobs, j.out ≠ []

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
  refine ⟨St.setTop_stack s l _ ls h, ⟨hb.inc, hb.noprog, hb.nolegacy, hb.prog, hb.pend, hb.tail,
    cov_push cx.H cx.z0 hb.cov (.ready v), ?_, ?_, ?_⟩, ⟨?_, ?_, ?_⟩, St.setTop_below s l _ ls h, ?_⟩
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
  refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj⟩
  apply holesOK_setTop s l l' ls h ht.holes 0 (Nat.zero_le _) (by simp) (Nat.zero_le _)
  · intro i _ hi'; exact hr' _ (List.getElem_mem hi')
  · intro j hj; have := jobs_below s l ls h ht hr j hj; omega

end Ssz.Hasher
