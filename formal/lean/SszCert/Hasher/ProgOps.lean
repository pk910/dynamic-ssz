import SszCert.Hasher.Prog

/-!
# Calls on a progressive scope

Leaf appends, the deferral of child scopes into a progressive scope and the
flushes of its deferred run (synchronous, and in the background, where each
element's root comes back from a job as a leaf), `Collapse` with Go's
active-region threshold, and the progressive closes.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- The result of calls on a progressive scope. -/
def PStep (cx : Ctx C) (s s' : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) : Prop :=
  ∃ l' ls', s'.stack = l' :: ls' ∧ ProgScope cx l' L ∧ TopOK s' ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
    s'.base.map Cell.erase = s.base.map Cell.erase ∧ s'.hashErr = s.hashErr ∧
    s'.below.length = s.below.length ∧ HolesSub s.below s'.below

theorem PStep.refl (cx : Ctx C) {s : St C} {l : Layer C} {ls : List (Layer C)} {L : List C}
    (h : s.stack = l :: ls) (hs : ProgScope cx l L) (ht : TopOK s) : PStep cx s s l ls L :=
  ⟨l, ls, h, hs, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩

theorem PStep.trans (cx : Ctx C) {s1 s2 s3 : St C} {l1 : Layer C} {ls1 : List (Layer C)} {L L' : List C}
    (h12 : PStep cx s1 s2 l1 ls1 L)
    (h23 : ∀ l2 ls2, s2.stack = l2 :: ls2 → ProgScope cx l2 L → TopOK s2 →
      ls2.map Layer.erase = ls1.map Layer.erase → PStep cx s2 s3 l2 ls2 L') :
    PStep cx s1 s3 l1 ls1 L' := by
  obtain ⟨l2, ls2, h2, hs2, ht2, hle2, hba2, hh2, hbl2, hsub2⟩ := h12
  obtain ⟨l3, ls3, h3, hs3, ht3, hle3, hba3, hh3, hbl3, hsub3⟩ := h23 l2 ls2 h2 hs2 ht2 hle2
  exact ⟨l3, ls3, h3, hs3, ht3, hle3.trans hle2, hba3.trans hba2, hh3.trans hh2, hbl3.trans hbl2,
    HolesSub.trans hsub2 hsub3⟩

theorem ProgScope.flags {cx : Ctx C} {l : Layer C} {L : List C} (h : ProgScope cx l L) :
    l.progressive = true ∧ l.incremental = true ∧ l.legacy = false := by
  obtain ⟨_, _, _, hp⟩ := h; exact ⟨hp.prog, hp.inc, hp.nolegacy⟩

theorem progCells (l : Layer C) : l.cells = (l.prog ++ nodeCells l.nodes) ++ (l.pend.flatten ++ l.tail) := by
  simp [Layer.cells, List.append_assoc]

/-! ## Leaves -/

/-- **A leaf appended to a progressive scope**: to the region, or behind its
    deferred run. -/
theorem push_prog (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) (v : C)
    (h : s.stack = l :: ls) (hs : ProgScope cx l L) (ht : TopOK s) :
    PStep cx s (s.push (.ready v)) l ls (L ++ [v]) := by
  obtain ⟨G, Lb, hL, hp⟩ := hs
  by_cases hpe : l.pend = []
  · have htl : l.tail = [] := hp.pendOK.tail hpe
    let l' : Layer C := { l with nodes := l.nodes ++ [⟨0, .ready v⟩] }
    have hpush : s.push (.ready v) = s.setTop l' := St.push_eq_setTop s l ls h hpe _
    rw [hpush]
    have hp' : ProgOK cx l' G (Lb ++ [v]) := by
      refine ⟨hp.prog, hp.inc, hp.nolegacy, hp.groups, hp.level, cov_push cx.H cx.z0 hp.cov (.ready v), ?_, ?_, ?_,
        ⟨fun _ => htl, by simp [l', hpe], by simp [l', htl], by simp [l', hpe], fun h => absurd hpe h⟩⟩
      · intro n hn; rcases List.mem_append.mp hn with hn | hn
        · exact hp.narrow n hn
        · simp at hn; rw [hn]; exact Nat.zero_le _
      · intro n hn; rcases List.mem_append.mp hn with hn | hn
        · exact hp.shallow n hn
        · simp at hn; rw [hn]; exact (by decide : (0:Nat) < 40)
      · intro hc
        obtain ⟨h0, hd⟩ := hp.leaves hc
        refine ⟨h0, fun n hn => ?_⟩
        rcases List.mem_append.mp hn with hn | hn
        · exact hd n hn
        · simp at hn; rw [hn]
    have hlc : l'.cells = l.cells ++ [.ready v] := by simp [l', Layer.cells, hpe, htl, nodeCells, List.append_assoc]
    refine ⟨l', ls, St.setTop_stack s l l' ls h, ⟨G, Lb ++ [v], ?_, hp'⟩, ?_, rfl, by rw [St.setTop_base],
      St.setTop_hashErr s l', by rw [St.setTop_below s l l' ls h], by rw [St.setTop_below s l l' ls h]; exact HolesSub.refl _⟩
    · rw [hL]; simp [pendLeaves, l', hpe, htl]
    · refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
        fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩
      apply holesOK_setTop s l l' ls h ht.holes l.cells.length (Nat.le_refl _) (by rw [hlc]; simp) (by rw [hlc]; simp)
      · intro i hi hi'
        have : i = l.cells.length := by rw [hlc] at hi'; simp at hi'; omega
        subst this
        simp only [hlc, List.getElem_append_right (Nat.le_refl _), Nat.sub_self]; rfl
      · intro j hj
        have := job_end_le s ht.holes j hj (ht.jobsne j hj)
        rw [St.cells_top s l ls h, List.length_append] at this; exact this
  · let l' : Layer C := { l with tail := l.tail ++ [.ready v] }
    have hpush : s.push (.ready v) = s.setTop l' := by simp [St.push, St.setTop, h, hpe, l']
    rw [hpush]
    have hlc : l'.cells = l.cells ++ [.ready v] := by simp [Layer.cells, l', List.append_assoc]
    have hp' : ProgOK cx l' G Lb := ⟨hp.prog, hp.inc, hp.nolegacy, hp.groups, hp.level, hp.cov, hp.narrow,
      hp.shallow, hp.leaves, ⟨fun e => absurd e hpe, hp.pendOK.ready, by
        intro c hc; rcases List.mem_append.mp hc with hc | hc
        · exact hp.pendOK.tready c hc
        · simp at hc; rw [hc]; rfl, hp.pendOK.width, hp.pendOK.pow⟩⟩
    refine ⟨l', ls, St.setTop_stack s l l' ls h, ⟨G, Lb, ?_, hp'⟩, ?_, rfl, by rw [St.setTop_base],
      St.setTop_hashErr s l', by rw [St.setTop_below s l l' ls h], by rw [St.setTop_below s l l' ls h]; exact HolesSub.refl _⟩
    · rw [hL]; simp [pendLeaves, l', Cell.val]
    · refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
        fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩
      apply holesOK_setTop s l l' ls h ht.holes l.cells.length (Nat.le_refl _) (by rw [hlc]; simp) (by rw [hlc]; simp)
      · intro i hi hi'
        have : i = l.cells.length := by rw [hlc] at hi'; simp at hi'; omega
        subst this
        simp only [hlc, List.getElem_append_right (Nat.le_refl _), Nat.sub_self]; rfl
      · intro j hj
        have := job_end_le s ht.holes j hj (ht.jobsne j hj)
        rw [St.cells_top s l ls h, List.length_append] at this; exact this

/-! ## The synchronous flush and the deferral -/

/-- **`flushPending` (synchronous) on a progressive scope**: the run's roots
    and the tail become leaves of the region. -/
theorem flushSync_prog (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : ProgScope cx l L) (ht : TopOK s) :
    ∃ l', (s.flushSync cx).stack = l' :: ls ∧ ProgScope cx l' L ∧ l'.pend = [] ∧
      l'.prog = l.prog ∧ l'.level = l.level ∧ l'.collapsed = l.collapsed ∧
      TopOK (s.flushSync cx) ∧ (s.flushSync cx).below = s.below ∧
      (s.flushSync cx).base = s.base ∧ (s.flushSync cx).hashErr = s.hashErr := by
  obtain ⟨G, Lb, hL, hp⟩ := hs
  by_cases hpe : l.pend = []
  · have hf : s.flushSync cx = s := by unfold St.flushSync; rw [h]; simp [hpe]
    rw [hf]
    exact ⟨l, h, ⟨G, Lb, hL, hp⟩, hpe, rfl, rfl, rfl, ht, rfl, rfl, rfl⟩
  have hff := flushFold_ready cx l.pend s hp.pendOK.ready
  generalize hrs : l.pend.map (fun e => reduceElem cx.H cx.z0 (e.map Cell.val)) = roots at hff
  let l' : Layer C := flushedLayer l roots
  have hf : s.flushSync cx = s.setTop l' := by
    unfold St.flushSync; rw [h]; simp only [hpe, if_false, hff]; rfl
  rw [hf]
  have hnew : ∀ n ∈ roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++ l.tail.map (fun c => ⟨0, c⟩),
      n.depth = 0 ∧ n.cell.isReady = true := by
    intro n hn
    rcases List.mem_append.mp hn with hn | hn
    · obtain ⟨v, _, rfl⟩ := List.mem_map.mp hn; exact ⟨rfl, rfl⟩
    · obtain ⟨c, hc, rfl⟩ := List.mem_map.mp hn; exact ⟨rfl, hp.pendOK.tready c hc⟩
  have hn' : l'.nodes = l.nodes ++ (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
      l.tail.map (fun c => ⟨0, c⟩)) := by simp [l', flushedLayer, List.append_assoc]
  have hcov : Cov cx.H cx.z0 true l'.nodes (Lb ++ pendLeaves cx l) := by
    have e1 : roots.map (fun v => (⟨0, .ready v⟩ : Node C)) = (roots.map Cell.ready).map (fun c => ⟨0, c⟩) := by
      simp [List.map_map, Function.comp_def]
    have h1 := cov_append_leaves cx.H cx.z0 (roots.map Cell.ready) hp.cov
    have h2 := cov_append_leaves cx.H cx.z0 l.tail h1
    have e2 : (roots.map Cell.ready).map Cell.val = roots := by simp [List.map_map, Function.comp_def, Cell.val]
    rw [e2] at h2
    show Cov cx.H cx.z0 true (l.nodes ++ roots.map (fun v => ⟨0, .ready v⟩) ++ l.tail.map (fun c => ⟨0, c⟩)) _
    rw [e1]
    simpa [pendLeaves, hrs, List.append_assoc] using h2
  have hp' : ProgOK cx l' G (Lb ++ pendLeaves cx l) := by
    refine ⟨hp.prog, hp.inc, hp.nolegacy, hp.groups, hp.level, hcov, ?_, ?_, ?_,
      ⟨fun _ => rfl, by simp [l', flushedLayer], by simp [l', flushedLayer], by simp [l', flushedLayer],
        fun h => absurd rfl h⟩⟩
    · intro n hn; rw [hn'] at hn; rcases List.mem_append.mp hn with hn | hn
      · exact hp.narrow n hn
      · rw [(hnew n hn).1]; exact Nat.zero_le _
    · intro n hn; rw [hn'] at hn; rcases List.mem_append.mp hn with hn | hn
      · exact hp.shallow n hn
      · rw [(hnew n hn).1]; exact (by decide : (0:Nat) < 40)
    · intro hc
      obtain ⟨h0, hd⟩ := hp.leaves hc
      refine ⟨h0, fun n hn => ?_⟩
      rw [hn'] at hn; rcases List.mem_append.mp hn with hn | hn
      · exact hd n hn
      · exact (hnew n hn).1
  have hl'c : l'.cells = (l.prog ++ nodeCells l.nodes) ++ nodeCells (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
      l.tail.map (fun c => ⟨0, c⟩)) := by
    simp only [Layer.cells, hn', nodeCells_append]; simp [l', flushedLayer, List.append_assoc]
  have hrdy : ∀ c ∈ l.pend.flatten ++ l.tail, c.isReady = true := by
    intro c hc
    rcases List.mem_append.mp hc with hc | hc
    · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc; exact hp.pendOK.ready e he c hce
    · exact hp.pendOK.tready c hc
  refine ⟨l', St.setTop_stack s l l' ls h, ⟨G, Lb ++ pendLeaves cx l, ?_, hp'⟩, rfl, rfl, rfl, rfl,
    ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
      fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩,
    St.setTop_below s l l' ls h, St.setTop_base s l', St.setTop_hashErr s l'⟩
  · rw [hL]; simp [pendLeaves, l', flushedLayer, List.append_assoc]
  · apply holesOK_setTop s l l' ls h ht.holes (l.prog ++ nodeCells l.nodes).length
      (by rw [progCells l]; simp) (by rw [progCells l, hl'c, List.take_left' rfl, List.take_left' rfl])
      (by rw [hl'c]; simp)
    · intro i hi hi'
      have hi2 : i - (l.prog ++ nodeCells l.nodes).length < (nodeCells (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
          l.tail.map (fun c => ⟨0, c⟩))).length := by
        have := hi'; rw [hl'c, List.length_append] at this; omega
      have : l'.cells[i]'hi' = (nodeCells (roots.map (fun v => (⟨0, .ready v⟩ : Node C)) ++
          l.tail.map (fun c => ⟨0, c⟩)))[i - (l.prog ++ nodeCells l.nodes).length]'hi2 := by
        simp only [hl'c]; rw [List.getElem_append_right (by omega)]
      rw [this]
      simp only [nodeCells, List.getElem_map]
      exact (hnew _ (List.getElem_mem _)).2
    · have := jobs_end_of_ready s ht (s.below ++ (l.prog ++ nodeCells l.nodes)) (l.pend.flatten ++ l.tail)
        (by rw [St.cells_top s l ls h, progCells l]; simp [List.append_assoc]) hrdy
      intro j hj; have := this j hj; simp only [List.length_append] at this ⊢; omega

/-- **A child parked in a progressive scope's deferred run.** -/
theorem deferInto_prog (cx : Ctx C) (t : St C) (p : Layer C) (ps : List (Layer C)) (Lp : List C)
    (elem : List (Cell C)) (c k : Nat) (r : C)
    (h : t.stack = p :: ps) (hs : ProgScope cx p Lp) (ht : TopOK t)
    (hcond : p.pend = [] ∨ (p.pendC = c ∧ p.tail = []))
    (he : ∀ x ∈ elem, x.isReady = true) (hel : elem.length = c) (hc : c = 2 ^ k) (hk : 0 < k)
    (hkb : 2 ^ k ≤ cx.P.batch) (hr : reduceElem cx.H cx.z0 (elem.map Cell.val) = r) :
    PStep cx t (t.setTop { p with pendC := c, pend := p.pend ++ [elem] }) p ps (Lp ++ [r]) := by
  obtain ⟨G, Lb, hL, hp⟩ := hs
  have htl : p.tail = [] := by
    rcases hcond with hc0 | ⟨_, ht0⟩
    · exact hp.pendOK.tail hc0
    · exact ht0
  let p' : Layer C := { p with pendC := c, pend := p.pend ++ [elem] }
  have hp' : ProgOK cx p' G Lb := ⟨hp.prog, hp.inc, hp.nolegacy, hp.groups, hp.level, hp.cov, hp.narrow, hp.shallow,
    hp.leaves, ⟨fun e => by simp [p'] at e, by
      intro e he' x hx
      rcases List.mem_append.mp he' with he' | he'
      · exact hp.pendOK.ready e he' x hx
      · simp at he'; rw [he'] at hx; exact he x hx, by simp [p', htl], by
      intro e he'
      rcases List.mem_append.mp he' with he' | he'
      · rcases hcond with hc0 | ⟨hpc, _⟩
        · rw [hc0] at he'; cases he'
        · show e.length = c; rw [← hpc]; exact hp.pendOK.width e he'
      · simp at he'; rw [he']; exact hel, fun _ => ⟨k, hc, hk, hkb⟩⟩⟩
  have hlc : p'.cells = p.cells ++ elem := by simp [Layer.cells, p', htl, List.append_assoc]
  refine ⟨p', ps, St.setTop_stack t p p' ps h, ⟨G, Lb, ?_, hp'⟩, ?_, rfl, (by rw [St.setTop_base]),
    St.setTop_hashErr t p', (by rw [St.setTop_below t p p' ps h]), (by rw [St.setTop_below t p p' ps h]; exact HolesSub.refl _)⟩
  · rw [hL]; simp [pendLeaves, p', htl, hr]
  · refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
      fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩
    apply holesOK_setTop t p p' ps h ht.holes p.cells.length (Nat.le_refl _) (by rw [hlc]; simp) (by rw [hlc]; simp)
    · intro i hi hi'
      have hi2 : i - p.cells.length < elem.length := by rw [hlc] at hi'; simp at hi'; omega
      simp only [hlc, List.getElem_append_right hi]
      exact he _ (List.getElem_mem _)
    · intro j hj
      have := job_end_le t ht.holes j hj (ht.jobsne j hj)
      rw [St.cells_top t p ps h, List.length_append] at this; exact this

end Ssz.Hasher