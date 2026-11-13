import SszCert.Hasher.Flush

/-!
# Calls on a scope with a deferred run

`StepOK s s' l ls L`: the innermost scope `l` of `s` became `l'` in `s'`,
which covers the leaves `L` with its run (`ScopeOK`), the enclosing scopes
kept their values and lost holes only, and no error was raised.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- The result of calls on the innermost scope. -/
def StepOK (cx : Ctx C) (s s' : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) : Prop :=
  ∃ l' ls', s'.stack = l' :: ls' ∧ ScopeOK cx l' L ∧ l'.incremental = l.incremental ∧
    (l'.incremental = false → l'.pend = []) ∧ TopOK s' ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
    s'.base.map Cell.erase = s.base.map Cell.erase ∧ s'.hashErr = s.hashErr ∧
    s'.below.length = s.below.length ∧ HolesSub s.below s'.below

theorem StepOK.refl (cx : Ctx C) {s : St C} {l : Layer C} {ls : List (Layer C)} {L : List C}
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (hn : l.incremental = false → l.pend = []) (ht : TopOK s) :
    StepOK cx s s l ls L :=
  ⟨l, ls, h, hs, rfl, hn, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩

theorem StepOK.trans (cx : Ctx C) {s1 s2 s3 : St C} {l1 : Layer C} {ls1 : List (Layer C)} {L L' : List C}
    (h12 : StepOK cx s1 s2 l1 ls1 L)
    (h23 : ∀ l2 ls2, s2.stack = l2 :: ls2 → ScopeOK cx l2 L → l2.incremental = l1.incremental →
      (l2.incremental = false → l2.pend = []) → TopOK s2 → ls2.map Layer.erase = ls1.map Layer.erase →
      StepOK cx s2 s3 l2 ls2 L') :
    StepOK cx s1 s3 l1 ls1 L' := by
  obtain ⟨l2, ls2, h2, hs2, hi2, hn2, ht2, hle2, hba2, hh2, hbl2, hsub2⟩ := h12
  obtain ⟨l3, ls3, h3, hs3, hi3, hn3, ht3, hle3, hba3, hh3, hbl3, hsub3⟩ := h23 l2 ls2 h2 hs2 hi2 hn2 ht2 hle2
  exact ⟨l3, ls3, h3, hs3, hi3.trans hi2, hn3, ht3, hle3.trans hle2, hba3.trans hba2, hh3.trans hh2,
    hbl3.trans hbl2, HolesSub.trans hsub2 hsub3⟩

/-- **`flushPending`** in either mode. -/
theorem flush_scope (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (hinc : l.incremental = true) (ht : TopOK s)
    (hL : L.length < 2 ^ 40) (aa a : Bool) :
    StepOK cx s (s.flush cx aa a) l ls L := by
  have hsync : StepOK cx s (s.flushSync cx) l ls L := by
    obtain ⟨l', h', hb', hi', _, ht', hbl', hba', hh'⟩ := flushSync_scope cx s l ls L h hs ht
    exact ⟨l', ls, h', scopeOK_of_bin cx hb', hi', fun _ => hb'.pend, ht', rfl, by rw [hba'], hh',
      by rw [hbl'], by rw [hbl']; exact HolesSub.refl _⟩
  unfold St.flush
  rw [h]
  simp only
  split
  · rename_i hc
    obtain ⟨_, hpne, _⟩ := hc
    obtain ⟨l', ls', h', hs', hi', _, ht', hle', hba', hh', hbl', hsub'⟩ :=
      flushAsync_scope cx hP.cap hP.batch_lt_cap s l ls L h hs hinc hpne ht hL
    cases hfa : s.flushAsync cx with
    | mk s' em =>
      rw [hfa] at h' ht' hba' hh' hbl' hsub'
      cases em
      · exact hsync
      · refine ⟨l', ls', h', hs', by rw [hi', hinc], fun hf => ?_, ht', hle', hba', hh', hbl', hsub'⟩
        rw [hi'] at hf; cases hf
  · exact hsync

/-- **Appending a leaf**: to the nodes, or behind a deferred run. -/
theorem push_scope (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) (v : C)
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (hn : l.incremental = false → l.pend = []) (ht : TopOK s) :
    StepOK cx s (s.push (.ready v)) l ls (L ++ [v]) := by
  by_cases hp : l.pend = []
  · have hb := bin_of_scopeOK cx hs hp
    obtain ⟨hs1, hb1, ht1, hbl1, hh1⟩ := push_bin cx s l ls L v h hb ht
    exact ⟨_, ls, hs1, scopeOK_of_bin cx hb1, rfl, fun _ => hp, ht1, rfl,
      by rw [push_base s l ls h], hh1, by rw [hbl1], by rw [hbl1]; exact HolesSub.refl _⟩
  obtain ⟨L1, hL, hb, hpo⟩ := hs
  let l' : Layer C := { l with tail := l.tail ++ [.ready v] }
  have hpush : s.push (.ready v) = s.setTop l' := by simp [St.push, St.setTop, h, hp, l']
  rw [hpush]
  have hsc : ScopeOK cx l' (L ++ [v]) := by
    refine ⟨L1, by rw [hL]; simp [pendLeaves, l', Cell.val], hb, ⟨fun e => absurd e hp, hpo.ready, ?_, hpo.width,
      hpo.pow⟩⟩
    intro c hc
    rcases List.mem_append.mp hc with hc | hc
    · exact hpo.tready c hc
    · simp at hc; rw [hc]; rfl
  have hlc : l'.cells = l.cells ++ [.ready v] := by simp [Layer.cells, l', List.append_assoc]
  refine ⟨l', ls, St.setTop_stack s l l' ls h, hsc, rfl, fun hf => absurd (hn hf) hp, ?_, rfl,
    by rw [St.setTop_base], St.setTop_hashErr s l', by rw [St.setTop_below s l l' ls h],
    by rw [St.setTop_below s l l' ls h]; exact HolesSub.refl _⟩
  refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
    fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩
  apply holesOK_setTop s l l' ls h ht.holes l.cells.length (Nat.le_refl _) (by rw [hlc]; simp)
    (by rw [hlc]; simp)
  · intro i hi hi'
    have : i = l.cells.length := by rw [hlc] at hi'; simp at hi'; omega
    subst this
    simp only [hlc, List.getElem_append_right (Nat.le_refl _), Nat.sub_self]
    rfl
  · intro j hj
    have := job_end_le s ht.holes j hj (ht.jobsne j hj)
    rw [St.cells_top s l ls h, List.length_append] at this; exact this

/-- A binary collapse in either mode, as a step. -/
theorem collapseBin_step (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l : Layer C) (ls : List (Layer C))
    (L : List C) (h : s.stack = l :: ls) (hb : BinOK cx l L) (hinc : l.incremental = true) (ht : TopOK s)
    (hL : L.length < 2 ^ 40) (a : Bool) :
    StepOK cx s (if a then (s.precollapse cx).1 else s.collapseBinary cx) l ls L := by
  cases a
  · simp only [Bool.false_eq_true, if_false]
    obtain ⟨l1, ls1, h1, hb1, hi1, ht1, hle1, hba1, hh1, hsub1⟩ :=
      collapseBinary_bin cx hP.batch_pos hP.batch_even s l ls L h hb hinc ht hL
    have hbl1 : (s.collapseBinary cx).below.length = s.below.length := by
      unfold St.below; rw [h, h1]
      simp only [List.drop_one, List.tail_cons, List.length_append]
      have := congrArg List.length hba1; simp only [List.length_map] at this
      rw [this]
      have e : ∀ xs : List (Layer C), ((xs.reverse.map Layer.cells).flatten).length =
          ((xs.map Layer.erase).reverse.map Layer.cells).flatten.length := by
        intro xs; simp [List.length_flatten, List.map_reverse, List.map_map, Function.comp_def, Layer.erase_cells]
      rw [e ls1, e ls, hle1]
    exact ⟨l1, ls1, h1, scopeOK_of_bin cx hb1, (by rw [hi1, hinc]), (fun hf => by rw [hi1] at hf; cases hf), ht1,
      hle1, hba1, hh1, hbl1, hsub1⟩
  · simp only [if_true]
    obtain ⟨l1, ls1, h1, hb1, hi1, ht1, hle1, hba1, hh1, hbl1, hsub1⟩ :=
      precollapse_bin cx hP.cap hP.capDepth_pos s l ls L h hb hinc ht hL
    exact ⟨l1, ls1, h1, scopeOK_of_bin cx hb1, (by rw [hi1, hinc]), (fun hf => by rw [hi1] at hf; cases hf), ht1,
      hle1, hba1, hh1, hbl1, hsub1⟩

/-- **`Collapse` on a scope**, with or without a deferred run. -/
theorem collapse_scope (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (hn : l.incremental = false → l.pend = []) (ht : TopOK s)
    (hL : L.length < 2 ^ 40) (a : Bool) :
    StepOK cx s (s.step cx (.collapse a)) l ls L := by
  have hs0 := hs
  obtain ⟨_, _, hb0, _⟩ := hs0
  have hnp : l.progressive = false := hb0.noprog
  cases hi : l.incremental
  · have : s.step cx (.collapse a) = s := by simp [St.step, h, hi]
    rw [this]; exact StepOK.refl cx h hs (fun _ => hn hi) ht
  by_cases hp : l.pend = []
  · have hb := bin_of_scopeOK cx hs hp
    have : s.step cx (.collapse a) = if a then (s.precollapse cx).1 else s.collapseBinary cx := by
      simp [St.step, h, hi, hp, hnp]
    rw [this]; exact collapseBin_step cx hP s l ls L h hb hi ht hL a
  by_cases hsm : a = true ∧ l.pend.length * l.pendC < cx.P.cap
  · have : s.step cx (.collapse a) = s := by simp [St.step, h, hi, hp, hsm.1, hsm.2]
    rw [this]; exact StepOK.refl cx h hs (fun hf => by rw [hi] at hf; cases hf) ht
  obtain ⟨l1, ls1, h1, hs1, hi1, hn1, ht1, hle1, hba1, hh1, hbl1, hsub1⟩ :=
    flush_scope cx hP s l ls L h hs hi ht hL a a
  have hs1' := hs1
  obtain ⟨_, _, hb1', _⟩ := hs1'
  have hnp1 : l1.progressive = false := hb1'.noprog
  have hstep : s.step cx (.collapse a) =
      if l1.pend ≠ [] then s.flush cx a a
      else if a then ((s.flush cx a a).precollapse cx).1 else (s.flush cx a a).collapseBinary cx := by
    have hsm' : ¬ (a = true ∧ l.pend.length * l.pendC < cx.P.cap) := hsm
    simp only [St.step, h, hi, Bool.not_true, Bool.false_eq_true, if_false, ne_eq, hp, not_false_eq_true,
      true_and, if_true]
    rw [if_neg (by simpa using hsm')]
    simp only [h1, hnp1, Bool.false_eq_true, if_false, ne_eq]
  rw [hstep]
  apply StepOK.trans cx ⟨l1, ls1, h1, hs1, hi1, hn1, ht1, hle1, hba1, hh1, hbl1, hsub1⟩
  intro l2 ls2 h2 hs2 hi2 hn2 ht2 _
  have hl12 : l2 = l1 := by rw [h1] at h2; exact (List.cons.inj h2).1.symm
  subst hl12
  by_cases hp1 : l2.pend = []
  · simp only [hp1, ne_eq, not_true_eq_false, if_false]
    exact collapseBin_step cx hP _ l2 ls2 L h2 (bin_of_scopeOK cx hs2 hp1) (by rw [hi2, hi]) ht2 hL a
  · simp only [hp1, ne_eq, not_false_eq_true, if_true]
    exact StepOK.refl cx h2 hs2 hn2 ht2

/-! ## Deferral -/

/-- **`deferrableElemChunks`** pads a scope of `n` chunks to the full subtree
    width `2^depthOf n`. -/
theorem deferrableWidth_spec (P : Params) (n c : Nat) (h : deferrableWidth P n = some c) :
    2 ≤ n ∧ n ≤ P.batch ∧ c = 2 ^ depthOf n := by
  unfold deferrableWidth at h
  split at h
  · cases h
  rename_i hr
  have h2 : 2 ≤ n := by omega
  have hb : n ≤ P.batch := by omega
  have hn0 : n ≠ 0 := by omega
  have hlo := Nat.log2_self_le hn0
  have hhi : n < 2 ^ (n.log2 + 1) := Nat.lt_log2_self
  generalize hm : n.log2 = m at hlo hhi h
  -- a number strictly between powers of two shares a bit with its predecessor
  have hbit : 2 ^ m < n → n &&& (n - 1) ≠ 0 := by
    intro hlt e
    have hpow : 2 ^ (m + 1) = 2 ^ m + 2 ^ m := by rw [Nat.pow_succ]; omega
    have e1 : 2 ^ m + (n - 2 ^ m) = n := by omega
    have e2 : 2 ^ m + (n - 1 - 2 ^ m) = n - 1 := by omega
    have t1 : (2 ^ m + (n - 2 ^ m)).testBit m = true := by
      rw [Nat.testBit_two_pow_add_eq, Nat.testBit_lt_two_pow (by omega)]; rfl
    have t2 : (2 ^ m + (n - 1 - 2 ^ m)).testBit m = true := by
      rw [Nat.testBit_two_pow_add_eq, Nat.testBit_lt_two_pow (by omega)]; rfl
    rw [e1] at t1; rw [e2] at t2
    have := Nat.testBit_and n (n - 1) m
    rw [e, Nat.zero_testBit, t1, t2] at this
    cases this
  split at h
  · rename_i hz
    cases h
    have heq : n = 2 ^ m := by
      rcases Nat.lt_or_ge (2 ^ m) n with hlt | hge
      · exact absurd hz (hbit hlt)
      · omega
    refine ⟨h2, hb, ?_⟩
    rw [heq, depthOf_two_pow]
  · rename_i hz
    cases h
    have hlt : 2 ^ m < n := by
      rcases Nat.lt_or_ge (2 ^ m) n with hlt | hge
      · exact hlt
      · exfalso; apply hz
        have heq : n = 2 ^ m := by omega
        rw [heq, Nat.and_two_pow_sub_one_eq_mod, Nat.mod_self]
    refine ⟨h2, hb, ?_⟩
    rw [depthOf_of_between hlt (by omega)]

/-- **Parking a child's chunks** as the next element of the parent's run. -/
theorem deferInto_scope (cx : Ctx C) (t : St C) (p : Layer C) (ps : List (Layer C)) (Lp : List C)
    (elem : List (Cell C)) (c k : Nat) (r : C)
    (h : t.stack = p :: ps) (hs : ScopeOK cx p Lp) (hinc : p.incremental = true) (ht : TopOK t)
    (hcond : p.pend = [] ∨ (p.pendC = c ∧ p.tail = []))
    (he : ∀ x ∈ elem, x.isReady = true) (hel : elem.length = c) (hc : c = 2 ^ k) (hk : 0 < k)
    (hkb : 2 ^ k ≤ cx.P.batch) (hr : reduceElem cx.H cx.z0 (elem.map Cell.val) = r) :
    StepOK cx t (t.setTop { p with pendC := c, pend := p.pend ++ [elem] }) p ps (Lp ++ [r]) := by
  obtain ⟨L1, hL, hb, hpo⟩ := hs
  have htl : p.tail = [] := by
    rcases hcond with hc0 | ⟨_, ht0⟩
    · exact hpo.tail hc0
    · exact ht0
  let p' : Layer C := { p with pendC := c, pend := p.pend ++ [elem] }
  have hsc : ScopeOK cx p' (Lp ++ [r]) := by
    refine ⟨L1, ?_, hb, ⟨fun e => by simp [p'] at e, ?_, by simp [p', htl], ?_, fun _ => ⟨k, hc, hk, hkb⟩⟩⟩
    · rw [hL]; simp [pendLeaves, p', htl, hr]
    · intro e he' x hx
      rcases List.mem_append.mp he' with he' | he'
      · exact hpo.ready e he' x hx
      · simp at he'; rw [he'] at hx; exact he x hx
    · intro e he'
      rcases List.mem_append.mp he' with he' | he'
      · rcases hcond with hc0 | ⟨hpc, _⟩
        · rw [hc0] at he'; cases he'
        · show e.length = c; rw [← hpc]; exact hpo.width e he'
      · simp at he'; rw [he']; exact hel
  have hlc : p'.cells = p.cells ++ elem := by simp [Layer.cells, p', htl, List.append_assoc]
  refine ⟨p', ps, St.setTop_stack t p p' ps h, hsc, rfl, (fun hf => by rw [hinc] at hf; cases hf), ?_, rfl,
    (by rw [St.setTop_base]), St.setTop_hashErr t p', (by rw [St.setTop_below t p p' ps h]),
    (by rw [St.setTop_below t p p' ps h]; exact HolesSub.refl _)⟩
  refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
    fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩
  apply holesOK_setTop t p p' ps h ht.holes p.cells.length (Nat.le_refl _) (by rw [hlc]; simp)
    (by rw [hlc]; simp)
  · intro i hi hi'
    have hi2 : i - p.cells.length < elem.length := by rw [hlc] at hi'; simp at hi'; omega
    simp only [hlc, List.getElem_append_right hi]
    exact he _ (List.getElem_mem _)
  · intro j hj
    have := job_end_le t ht.holes j hj (ht.jobsne j hj)
    rw [St.cells_top t p ps h, List.length_append] at this; exact this

end Ssz.Hasher