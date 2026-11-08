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

/-! ## The background flush of a progressive scope's run -/

/-- Between flush rounds: the progressive invariant, the run's cells ready
    and uniform (the tail may remain behind an emptied run). -/
structure PFlush (cx : Ctx C) (l : Layer C) (G : List (List C)) (Lb : List C) : Prop where
  prog : l.progressive = true
  inc : l.incremental = true
  nolegacy : l.legacy = false
  groups : GroupsOK cx l.prog G
  level : l.level = G.length
  cov : Cov cx.H cx.z0 true l.nodes Lb
  narrow : ∀ n ∈ l.nodes, n.depth ≤ 2 * l.level
  shallow : ∀ n ∈ l.nodes, n.depth < 40
  leaves : l.collapsed = false → l.level = 0 ∧ ∀ n ∈ l.nodes, n.depth = 0
  ready : ∀ c ∈ l.pend.flatten ++ l.tail, c.isReady = true
  width : ∀ e ∈ l.pend, e.length = l.pendC

theorem pflush_of_prog {cx : Ctx C} {l : Layer C} {G : List (List C)} {Lb : List C} (h : ProgOK cx l G Lb) :
    PFlush cx l G Lb :=
  ⟨h.prog, h.inc, h.nolegacy, h.groups, h.level, h.cov, h.narrow, h.shallow, h.leaves, by
    intro c hc
    rcases List.mem_append.mp hc with hc | hc
    · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc; exact h.pendOK.ready e he c hce
    · exact h.pendOK.tready c hc, h.pendOK.width⟩

theorem LayerLE.pflush (cx : Ctx C) {l l' : Layer C} (h : LayerLE l l') {G : List (List C)} {Lb : List C}
    (hp : PFlush cx l G Lb) : PFlush cx l' G Lb := by
  have hdep : ∀ n ∈ l'.nodes, ∃ m ∈ l.nodes, m.depth = n.depth := by
    intro n hn
    have : n.erase ∈ l.nodes.map Node.erase := by rw [← h.nodes_map]; exact List.mem_map_of_mem hn
    obtain ⟨m, hm, he'⟩ := List.mem_map.mp this
    exact ⟨m, hm, by have := congrArg Node.depth he'; exact this⟩
  have hlv : l'.level = l.level := by have := congrArg Layer.level h.erase; exact this
  have hlens : l'.pend.map List.length = l.pend.map List.length := by
    have := congrArg (List.map List.length) h.pend_map
    simpa [List.map_map, Function.comp_def] using this
  refine ⟨by rw [h.flag_progressive]; exact hp.prog, by rw [h.flag_incremental]; exact hp.inc,
    by rw [h.flag_legacy]; exact hp.nolegacy, groupsOK_erase cx hp.groups h.prog_map, by rw [hlv]; exact hp.level,
    cov_of_erase_eq cx.H cx.z0 h.nodes_map hp.cov, ?_, ?_, ?_, ?_, ?_⟩
  · intro n hn; obtain ⟨m, hm, hmd⟩ := hdep n hn; rw [← hmd, hlv]; exact hp.narrow m hm
  · intro n hn; obtain ⟨m, hm, hmd⟩ := hdep n hn; rw [← hmd]; exact hp.shallow m hm
  · intro hc
    obtain ⟨h1, h2⟩ := hp.leaves (by rw [← h.flag_collapsed]; exact hc)
    exact ⟨by rw [hlv]; exact h1, fun n hn => by obtain ⟨m, hm, hmd⟩ := hdep n hn; rw [← hmd]; exact h2 m hm⟩
  · intro c hc
    cases hcr : c.isReady with
    | true => rfl
    | false =>
      exfalso
      obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hc
      obtain ⟨c0, h0, h0r⟩ := h.parts.2.2 i _ (List.getElem?_eq_getElem hi) hcr
      rw [hp.ready c0 (List.mem_of_getElem? h0)] at h0r; cases h0r
  · intro e he
    rw [h.pendC]
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem he
    have hi0 : i < l.pend.length := by have := congrArg List.length hlens; simp at this; omega
    have := congrArg (fun xs => xs[i]?) hlens
    simp only [List.getElem?_map, List.getElem?_eq_getElem hi, List.getElem?_eq_getElem hi0, Option.map_some,
      Option.some.injEq] at this
    rw [this]; exact hp.width _ (List.getElem_mem _)

/-- A block of uniform elements reduces to their roots. -/
theorem reduceTo_elems (H : C → C → C) (z0 : C) (k m : Nat) (es : List (List C)) (hw : ∀ e ∈ es, e.length = 2 ^ k)
    (hn : es.length = m) (hm : 0 < m) (fuel : Nat) (hf : k ≤ fuel) :
    reduceTo H m fuel es.flatten = es.map (mroot H z0 k) := by
  have hlen : es.flatten.length = m * 2 ^ k := by rw [length_flatten_uniform es hw, hn]
  have h1 := reduceTo_chunks H z0 m k hm es.flatten hlen 0 fuel (Nat.zero_le _) (by omega)
  rw [chunk_roots_zero, chunks_flatten k es hw] at h1
  exact h1

theorem map_getD_range (xs : List C) (d : C) : (List.range xs.length).map (fun i => xs.getD i d) = xs := by
  apply List.ext_getElem (by simp)
  intro i h1 h2
  simp [List.getElem_map, List.getElem_range, List.getD_eq_getElem?_getD, List.getElem?_eq_getElem h2]

/-- **One progressive round of `flushPendingAsync`**: a cap-wide block of the
    run becomes one background job; its element roots come back as leaves of
    the region, their holes where the block began. -/
theorem progRound_prog (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (s : St C) (l : Layer C)
    (ls : List (Layer C)) (G : List (List C)) (Lb : List C) (k : Nat)
    (h : s.stack = l :: ls) (hf : PFlush cx l G Lb) (ht : TopOK s) (hk : l.pendC = 2 ^ k) (hkc : k ≤ cx.P.capDepth)
    (hpl : cx.P.cap / l.pendC ≤ l.pend.length) :
    ∃ l' ls', (s.progRound cx l.pendC l).stack = l' :: ls' ∧
      PFlush cx l' G (Lb ++ (pendLeaves cx l).take (cx.P.cap / l.pendC)) ∧ l'.pendC = l.pendC ∧
      l'.pend.length = l.pend.length - cx.P.cap / l.pendC ∧
      TopOK (s.progRound cx l.pendC l) ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
      (s.progRound cx l.pendC l).base.map Cell.erase = s.base.map Cell.erase ∧
      (s.progRound cx l.pendC l).hashErr = s.hashErr ∧
      (s.progRound cx l.pendC l).below.length = s.below.length ∧
      HolesSub s.below (s.progRound cx l.pendC l).below := by
  have hbe : cx.P.cap / l.pendC = 2 ^ (cx.P.capDepth - k) := by rw [hk, cap_div cx hcap hkc]
  generalize hbev : cx.P.cap / l.pendC = be at hbe hpl ⊢
  have hbe0 : 0 < be := by rw [hbe]; exact Nat.two_pow_pos _
  generalize hB : l.pend.take be = B
  have hBlen : B.length = be := by rw [← hB, List.length_take]; omega
  have hBw : ∀ e ∈ B, e.length = 2 ^ k := by
    intro e he; rw [← hB] at he; rw [← hk]; exact hf.width e (List.mem_of_mem_take he)
  have hBr : ∀ c ∈ B.flatten, c.isReady = true := by
    intro c hc; rw [← hB] at hc
    obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc
    exact hf.ready c (List.mem_append_left _ (List.mem_flatten.mpr ⟨e, List.mem_of_mem_take he, hce⟩))
  have hinlen : B.flatten.length = cx.P.cap := by
    rw [length_flatten_uniform B hBw, hBlen, hbe, hcap, ← Nat.pow_add]; congr 1; omega
  generalize hX : B.map (fun e => reduceElem cx.H cx.z0 (e.map Cell.val)) = X
  have hXlen : X.length = be := by rw [← hX, List.length_map, hBlen]
  have hout : reduceTo cx.H be B.flatten.length (B.flatten.map Cell.val) = X := by
    rw [map_vals_flatten, reduceTo_elems cx.H cx.z0 k be (B.map (·.map Cell.val)) (by
        intro e he; obtain ⟨e', he', rfl⟩ := List.mem_map.mp he; simp [hBw e' he'])
      (by simp [hBlen]) hbe0 _ (by rw [hinlen, hcap]; have := Nat.lt_two_pow_self (n := cx.P.capDepth); omega)]
    rw [← hX, List.map_map]
    apply List.map_congr_left
    intro e he
    simp only [Function.comp_apply]
    exact (reduceElem_eq cx.H cx.z0 k _ (by simp [hBw e he])).symm
  -- the ring drain and the job
  have hact : s.activeStart = s.below.length + l.prog.length := by
    unfold St.activeStart; rw [h]; simp only; rw [St.topStart_eq s l ls h]
  obtain ⟨hv1, ht1, hn1, hh1, hlen1⟩ := ringDrain_topOK cx.P s ht
  obtain ⟨l1, ls1, hs1, hle, hls1⟩ := LayerLE.of_view hv1 h (ringDrain_holesSub cx.P s ht.holes)
  have hbl1 := below_length_of_view hv1 h hs1
  have hf1 : PFlush cx l1 G Lb := hle.pflush cx hf
  have hpl1 : l1.prog.length = l.prog.length := erase_map_length hle.prog_map
  have hnl1 : l1.nodes.length = l.nodes.length := hle.nodes_length
  let J : Job C := ⟨(s.ringDrain cx.P).nextJob, s.below.length + l.prog.length + l.nodes.length, X⟩
  let T : St C := (s.ringDrain cx.P).addJob J
  let holes : List (Cell C) := (List.range be).map (fun i => Cell.hole (X.getD i cx.z0) (s.ringDrain cx.P).nextJob i)
  have hen : s.enqueue cx B.flatten be (s.activeStart + (nodeCells l.nodes).length) = (T, holes) := by
    rw [enqueue_eq cx s _ be _ hBr, hout, hact, nodeCells_length]
    simp only [T, J, holes, St.addJob, hXlen]
  have hpr : s.progRound cx l.pendC l = T.setTop (progRoundLayer be (T.topLayer l) holes) := by
    unfold St.progRound; rw [hbev, hB, hen]
  rw [hpr]
  have hTs : T.stack = l1 :: ls1 := hs1
  have hTtop : T.topLayer l = l1 := by simp [St.topLayer, hTs]
  rw [hTtop]
  -- the new scope
  let l' := progRoundLayer be l1 holes
  have hhv : holes.map Cell.val = X := by
    simp only [holes, List.map_map, Function.comp_def, Cell.val]
    rw [← hXlen]; exact map_getD_range X cx.z0
  have hhl : holes.length = be := by simp [holes]
  have hpe1 : l1.pend.map List.length = l.pend.map List.length := by
    have := congrArg (List.map List.length) hle.pend_map; simpa [List.map_map, Function.comp_def] using this
  have hpend1 : l1.pend.length = l.pend.length := by have := congrArg List.length hpe1; simpa using this
  have hpl1' := hle.pendLeaves cx
  have hTake : (pendLeaves cx l).take be = X := by
    unfold pendLeaves
    rw [List.take_append_of_le_length (by simp; omega), ← List.map_take, hB, hX]
  have hdrop : pendLeaves cx l' = (pendLeaves cx l1).drop be := by
    unfold pendLeaves
    rw [List.drop_append_of_le_length (by simp; omega)]
    simp [l', progRoundLayer, List.map_drop]
  have hcov' : Cov cx.H cx.z0 true l'.nodes (Lb ++ X) := by
    have := cov_append_leaves cx.H cx.z0 holes hf1.cov
    rw [hhv] at this; exact this
  have hnew : ∀ n ∈ holes.map (fun h => (⟨0, h⟩ : Node C)), n.depth = 0 := by
    intro n hn; obtain ⟨_, _, rfl⟩ := List.mem_map.mp hn; rfl
  have hpf' : PFlush cx l' G (Lb ++ X) := by
    refine ⟨hf1.prog, hf1.inc, hf1.nolegacy, hf1.groups, hf1.level, hcov', ?_, ?_, ?_, ?_, ?_⟩
    · intro n hn; rcases List.mem_append.mp hn with hn | hn
      · exact hf1.narrow n hn
      · rw [hnew n hn]; exact Nat.zero_le _
    · intro n hn; rcases List.mem_append.mp hn with hn | hn
      · exact hf1.shallow n hn
      · rw [hnew n hn]; exact (by decide : (0:Nat) < 40)
    · intro hc
      obtain ⟨h0, hd⟩ := hf1.leaves hc
      refine ⟨h0, fun n hn => ?_⟩
      rcases List.mem_append.mp hn with hn | hn
      · exact hd n hn
      · exact hnew n hn
    · intro c hc
      apply hf1.ready
      rcases List.mem_append.mp hc with hc | hc
      · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc
        exact List.mem_append_left _ (List.mem_flatten.mpr ⟨e, List.mem_of_mem_drop he, hce⟩)
      · exact List.mem_append_right _ hc
    · intro e he; exact hf1.width e (List.mem_of_mem_drop he)
  -- the cells
  have hc1 : l1.cells = (l1.prog ++ nodeCells l1.nodes) ++ (l1.pend.flatten ++ l1.tail) := progCells l1
  have hc' : l'.cells = (l1.prog ++ nodeCells l1.nodes) ++ (holes ++ (l'.pend.flatten ++ l'.tail)) := by
    simp [Layer.cells, l', progRoundLayer, nodeCells, List.map_map, Function.comp_def, List.append_assoc]
  have hk0 : (l1.prog ++ nodeCells l1.nodes).length = l.prog.length + l.nodes.length := by
    simp [nodeCells_length, hpl1, hnl1]
  refine ⟨l', ls1, St.setTop_stack T l1 l' ls1 hTs, by rw [hTake]; exact hpf', hle.pendC,
    by simp [l', progRoundLayer, hpend1], ⟨?_, ?_, ?_, ?_⟩, hls1, ?_, ?_, ?_, ?_⟩
  · show HolesOK (St.setTop ((s.ringDrain cx.P).addJob J) l')
    unfold St.addJob
    apply holesOK_addJob _ l1 l' ls1 hs1 ht1.holes J (fun j' hj' => Nat.ne_of_lt (ht1.ids j' hj'))
      (l1.prog ++ nodeCells l1.nodes).length (by rw [hc1]; simp)
    · rw [hc1, hc', List.take_left' rfl, List.take_left' rfl]
    · show s.below.length + l.prog.length + l.nodes.length = _; rw [hbl1, hk0]; omega
    · intro i hi
      refine ⟨i, ?_⟩
      have hib : i < be := by simp [J] at hi; omega
      rw [hc', List.getElem?_append_right (by omega), Nat.add_sub_cancel_left,
        List.getElem?_append_left (by rw [hhl]; exact hib)]
      simp [holes, List.getElem?_map, List.getElem?_range hib, List.getD_eq_getElem?_getD,
        List.getElem?_eq_getElem (show i < X.length by omega), J]
    · rw [hc']; simp [J, hXlen, hhl]; omega
    · intro i hi hi'
      have hm : l'.cells[i]'hi' ∈ l'.pend.flatten ++ l'.tail := by
        have hc'' : l'.cells = ((l1.prog ++ nodeCells l1.nodes) ++ holes) ++ (l'.pend.flatten ++ l'.tail) := by
          rw [hc']; simp [List.append_assoc]
        simp only [hc'']
        rw [List.getElem_append_right (by simp [J, hXlen, hhl] at hi ⊢; omega)]
        exact List.getElem_mem _
      exact hpf'.ready _ hm
    · have := jobs_end_of_ready (s.ringDrain cx.P) ht1 ((s.ringDrain cx.P).below ++ (l1.prog ++ nodeCells l1.nodes))
        (l1.pend.flatten ++ l1.tail) (by rw [St.cells_top _ l1 ls1 hs1, hc1]; simp [List.append_assoc]) hf1.ready
      intro j hj; have := this j hj; simp only [List.length_append] at this ⊢; omega
  · rw [St.setTop_err]; exact ht1.err
  · intro j' hj'
    rw [St.setTop_jobs] at hj'
    change j' ∈ (s.ringDrain cx.P).jobs ++ [J] at hj'
    rcases List.mem_append.mp hj' with hj' | hj'
    · exact ht1.jobsne j' hj'
    · simp at hj'; subst hj'; show X ≠ []; intro e; rw [e] at hXlen; simp at hXlen; omega
  · intro j' hj'
    rw [St.setTop_jobs] at hj'
    change j' ∈ (s.ringDrain cx.P).jobs ++ [J] at hj'
    rw [St.setTop_nextJob]
    change j'.id < (s.ringDrain cx.P).nextJob + 1
    rcases List.mem_append.mp hj' with hj' | hj'
    · have := ht1.ids j' hj'; omega
    · simp at hj'; subst hj'; simp [J]
  · show (T.setTop l').base.map Cell.erase = s.base.map Cell.erase
    rw [St.setTop_base]
    exact congrArg Prod.fst hv1
  · rw [St.setTop_hashErr]; exact hh1
  · rw [St.setTop_below T l1 l' ls1 hTs]; exact hbl1
  · rw [St.setTop_below T l1 l' ls1 hTs]
    show HolesSub s.below (s.ringDrain cx.P).below
    exact holesSub_below hv1 h hs1 (ringDrain_holesSub cx.P s ht.holes)

end Ssz.Hasher