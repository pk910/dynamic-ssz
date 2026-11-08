import SszCert.Hasher.Async

/-!
# Closing a binary scope

`Merkleize` and `MerkleizeWithMixin` on a binary scope: after the flush (a
no-op without deferred elements) and the drain of every job that writes
into the scope, the scope's root is the bounded Merkle root of its leaves at
the depth of the limit, whether the scope was collapsed (`collapseAllDepths`)
or not (`merkleizeImpl`), and the close raises no error.
-/

namespace Ssz.Hasher

variable {C : Type}

theorem cell_eq_ready {c : Cell C} (h : c.isReady = true) : c = .ready c.val := by
  cases c with
  | ready v => rfl
  | hole _ _ _ => cases h

/-- A cover by depth-0 nodes lists the leaves. -/
theorem cov_leaves (H : C → C → C) (z0 : C) : ∀ (ns : List (Node C)) (L : List C),
    Cov H z0 true ns L → (∀ n ∈ ns, n.depth = 0) → ns.map (·.cell.val) = L
  | [], L, h, _ => by have : L = [] := h; simp [this]
  | [n], L, h, hd => by
    obtain ⟨_, hlen, hv⟩ := h
    have h0 := hd n (by simp)
    rw [h0] at hlen hv
    simp only [if_true, Nat.pow_zero] at hlen
    match L, hlen with
    | [x], _ => simp only [mroot_zero_cons] at hv; simp [hv]
  | n :: m :: ns, L, h, hd => by
    obtain ⟨xs, ys, hL, hxl, hv, _, hc⟩ := h
    have h0 := hd n (by simp)
    rw [h0] at hxl hv
    have ih := cov_leaves H z0 (m :: ns) ys hc (fun k hk => hd k (List.mem_cons_of_mem _ hk))
    subst hL
    match xs, hxl with
    | [x], _ =>
      simp only [mroot_zero_cons] at hv
      rw [List.map_cons, ih, hv]; rfl

theorem merkleizeImpl_over (H : C → C → C) (z0 : C) (xs : List C) (limit : Nat) :
    (merkleizeImpl H z0 xs limit).2 = decide (xs.length > if limit = 0 then xs.length else limit) := by
  unfold merkleizeImpl
  simp only
  repeat' split
  all_goals rfl

theorem flushSync_nopend (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hp : l.pend = []) : s.flushSync cx = s := by
  unfold St.flushSync; rw [h]; simp [hp]

theorem popWith_eq (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) (c : Cell C) :
    s.popWith c = ({ s with stack := ls } : St C).push c := by
  simp [St.popWith, h]

/-- The limit a close reduces to, with the leaves fitting it. -/
def closeDepth (n limit : Nat) : Nat := depthOf (if limit = 0 then n else limit)

/-- **The root of a drained binary scope** is the bounded Merkle root of its
    leaves; computing it changes nothing. -/
theorem binRoot_bin (cx : Ctx C) (s : St C) (l : Layer C) (L : List C) (limit : Nat)
    (hb : BinOK cx l L) (hr : ∀ c ∈ l.cells, c.isReady = true)
    (hlim : limit = 0 ∨ L.length ≤ limit) :
    (s.binRoot cx l limit).1 = s ∧
    (s.binRoot cx l limit).2 = .ready (mroot cx.H cx.z0 (closeDepth L.length limit) L) := by
  have hcl := layer_cells_bin l hb.prog hb.pend hb.tail
  have hnr : ∀ n ∈ l.nodes, n.cell.isReady = true := by
    intro n hn; apply hr; rw [hcl]; exact List.mem_map_of_mem hn
  unfold St.binRoot
  cases hc : l.collapsed with
  | true =>
    simp only [if_true]
    have hne := hb.nonempty hc
    have hd : headDepth l.nodes < 64 := by
      obtain ⟨n, ns, hns⟩ := List.exists_cons_of_ne_nil hne
      have := hb.shallow n (by rw [hns]; simp)
      simp [headDepth, hns]; omega
    obtain ⟨h1, h2⟩ := collapseAll_state cx s l.nodes limit L hb.cov hne hnr hd hlim
    have hv := collapseAll_val cx s l.nodes limit false L hb.cov hne hd hlim
    refine ⟨h1, ?_⟩
    rw [cell_eq_ready h2, hv]; rfl
  | false =>
    simp only [Bool.false_eq_true, if_false]
    have hvals : l.cells.map Cell.val = L := by
      rw [hcl]; unfold nodeCells; rw [List.map_map]
      exact cov_leaves cx.H cx.z0 l.nodes L hb.cov (hb.leaves hc)
    have hover : (merkleizeImpl cx.H cx.z0 (l.cells.map Cell.val) limit).2 = false := by
      rw [merkleizeImpl_over, hvals]
      rcases hlim with h0 | h0
      · simp [h0]
      · by_cases hz : limit = 0
        · simp [hz]
        · simp [hz]; omega
    refine ⟨?_, ?_⟩
    · simp only [St.setHashErrIf, hover, Bool.false_eq_true, if_false]
      exact (vals_ready s l.cells _ hr)
    · rw [merkleizeImpl_eq, hvals]
      unfold closeDepth implLimit
      congr 2
      by_cases hz : limit = 0
      · simp [hz]
      · rcases hlim with h0 | h0
        · exact absurd h0 hz
        · simp [hz, Nat.max_eq_left h0]

/-- The drain before a close: the scope becomes hole-free, everything else
    keeps its view. -/
theorem drainTop_bin (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hb : BinOK cx l L) (ht : TopOK s) :
    ∃ l' ls', (s.drainFor s.topStart).stack = l' :: ls' ∧ BinOK cx l' L ∧ TopOK (s.drainFor s.topStart) ∧
      (∀ c ∈ l'.cells, c.isReady = true) ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
      (s.drainFor s.topStart).base.map Cell.erase = s.base.map Cell.erase ∧
      (s.drainFor s.topStart).hashErr = s.hashErr := by
  obtain ⟨hv, ht2, hh2, _⟩ := drainFor_spec s ht s.topStart
  obtain ⟨l2, ls2, hs2, he2, hle2⟩ := stack_of_view hv h
  have htop : s.topStart = s.below.length := St.topStart_eq s l ls h
  have hbl := below_length_of_view hv h hs2
  have hr2 : ∀ c ∈ l2.cells, c.isReady = true := by
    intro c hc
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hc
    have hi' : (s.drainFor s.topStart).below.length + i < (s.drainFor s.topStart).cells.length := by
      rw [St.cells_top _ l2 ls2 hs2, List.length_append]; omega
    have := drainFor_no_holes s ht.holes s.topStart _ (by omega) hi'
    have hget : (s.drainFor s.topStart).cells[(s.drainFor s.topStart).below.length + i]? = some (l2.cells[i]) := by
      rw [St.cells_top _ l2 ls2 hs2, List.getElem?_append_right (by omega), Nat.add_sub_cancel_left,
        List.getElem?_eq_getElem hi]
    rw [List.getElem?_eq_getElem hi'] at hget
    simp only [Option.some.injEq] at hget
    rw [← hget]; exact this
  have hb2 : BinOK cx l2 L := binOK_of_erase cx hb he2 (by
    intro n hn hnr
    have : n.cell ∈ l2.cells := by
      simp only [Layer.cells, List.mem_append, nodeCells, List.mem_map]
      exact Or.inl (Or.inl (Or.inr ⟨n, hn, rfl⟩))
    rw [hr2 _ this] at hnr; cases hnr)
  exact ⟨l2, ls2, hs2, hb2, ht2, hr2, hle2, congrArg Prod.fst hv, hh2⟩

theorem St.cells_pop (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    ({ s with stack := ls } : St C).cells = s.below := by
  simp [St.cells, St.below, h]

/-- **Popping a hole-free innermost scope** keeps the invariants of the rest. -/
theorem topOK_pop (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) (ht : TopOK s)
    (hr : ∀ c ∈ l.cells, c.isReady = true) : TopOK ({ s with stack := ls } : St C) := by
  have hjb := jobs_below s l ls h ht hr
  have hc := St.cells_pop s l ls h
  have hc0 := St.cells_top s l ls h
  refine ⟨⟨?_, ?_, ht.holes.distinct, ht.holes.maxEnd⟩, ht.err, ht.jobsne, ht.ids⟩
  · intro j hj k hk
    obtain ⟨idx, hidx⟩ := ht.holes.inplace j hj k hk
    have := hjb j hj
    refine ⟨idx, ?_⟩
    show ({ s with stack := ls } : St C).cells[j.dst + k]? = _
    rw [hc]; rw [hc0, List.getElem?_append_left (by omega)] at hidx; exact hidx
  · intro p v id idx hp
    show ∃ j ∈ s.jobs, _
    have hpl : p < s.below.length := by
      rw [hc] at hp
      rcases Nat.lt_or_ge p s.below.length with h1 | h1
      · exact h1
      · rw [List.getElem?_eq_none h1] at hp; cases hp
    apply ht.holes.owned p v id idx
    rw [hc] at hp; rw [hc0, List.getElem?_append_left hpl]; exact hp

/-- **`Merkleize` (non-deferred) on a binary scope**: the scope is replaced
    by its root, the bounded Merkle root of its leaves, in the enclosing
    region; the rest of the state keeps its view and its invariants. -/
theorem closeBinary_bin (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (limit : Nat) (a : Bool) (h : s.stack = l :: ls) (hb : BinOK cx l L) (ht : TopOK s)
    (hlim : limit = 0 ∨ L.length ≤ limit) :
    ∃ s1 : St C, s1.stack.map Layer.erase = ls.map Layer.erase ∧ TopOK s1 ∧
      s1.base.map Cell.erase = s.base.map Cell.erase ∧ s1.hashErr = s.hashErr ∧
      s.closeBinary cx limit a = s1.push (.ready (mroot cx.H cx.z0 (closeDepth L.length limit) L)) := by
  have hfl := flushSync_nopend cx s l ls h hb.pend
  obtain ⟨l2, ls2, hs2, hb2, ht2, hr2, hle2, hba2, hh2⟩ := drainTop_bin cx s l ls L h hb ht
  obtain ⟨hst, hroot⟩ := binRoot_bin cx (s.drainFor s.topStart) l2 L limit hb2 hr2 hlim
  unfold St.closeBinary
  rw [hfl, hs2]
  simp only
  rw [hst, hroot, popWith_eq _ l2 ls2 hs2]
  exact ⟨_, hle2, topOK_pop _ l2 ls2 hs2 ht2 hr2, hba2, hh2, rfl⟩

/-- **`MerkleizeWithMixin` on a binary scope**: the root mixes the scope's
    bounded Merkle root with the length word. -/
theorem mixinClose_bin (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (num limit : Nat) (h : s.stack = l :: ls) (hb : BinOK cx l L) (ht : TopOK s)
    (hlim : limit = 0 ∨ L.length ≤ limit) :
    ∃ s1 : St C, s1.stack.map Layer.erase = ls.map Layer.erase ∧ TopOK s1 ∧
      s1.base.map Cell.erase = s.base.map Cell.erase ∧ s1.hashErr = s.hashErr ∧
      ((s.flushSync cx).drainFor (s.flushSync cx).topStart).mixinClose cx num limit =
        s1.push (.ready (cx.H (mroot cx.H cx.z0 (closeDepth L.length limit) L) (cx.word num))) := by
  have hfl := flushSync_nopend cx s l ls h hb.pend
  obtain ⟨l2, ls2, hs2, hb2, ht2, hr2, hle2, hba2, hh2⟩ := drainTop_bin cx s l ls L h hb ht
  obtain ⟨hst, hroot⟩ := binRoot_bin cx (s.drainFor s.topStart) l2 L limit hb2 hr2 hlim
  rw [hfl]
  unfold St.mixinClose
  rw [hs2]
  simp only
  rw [hst, hroot, vals_ready _ _ _ (by simp [Cell.isReady]), popWith_eq _ l2 ls2 hs2]
  exact ⟨_, hle2, topOK_pop _ l2 ls2 hs2 ht2 hr2, hba2, hh2, rfl⟩

end Ssz.Hasher
