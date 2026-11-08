import SszCert.Hasher.Binary

/-!
# Background jobs

A job snapshots its input when it is enqueued and leaves a hole carrying its
result; `capJob`/`capCompact` (one round of `precollapseCapRun`) replace a
cap-wide run of leaves by one such hole. This file proves the round keeps
the scope's cover and the hole invariant.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- **Adding a job** whose hole sits right after the first `k` cells of the
    innermost region (everything after it ready, every older job below it). -/
theorem holesOK_addJob (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hok : HolesOK s) (j : Job C) (hfresh : ∀ j' ∈ s.jobs, j'.id ≠ j.id)
    (k : Nat) (hk : k ≤ l.cells.length) (hpre : l'.cells.take k = l.cells.take k)
    (hdst : j.dst = s.below.length + k)
    (hhole : ∀ i (hi : i < j.out.length), ∃ idx, l'.cells[k + i]? = some (.hole (j.out[i]'hi) j.id idx))
    (hlen : k + j.out.length ≤ l'.cells.length)
    (hnew : ∀ i (hi : k + j.out.length ≤ i) (hi' : i < l'.cells.length), (l'.cells[i]'hi').isReady = true)
    (hjob : ∀ j' ∈ s.jobs, j'.dst + j'.out.length ≤ s.below.length + k) (m : Nat) :
    HolesOK (St.setTop { s with jobs := s.jobs ++ [j], nextJob := m, jobMaxEnd := max s.jobMaxEnd (j.dst + j.out.length) } l') := by
  let t : St C := { s with jobs := s.jobs ++ [j], nextJob := m, jobMaxEnd := max s.jobMaxEnd (j.dst + j.out.length) }
  have ht : t.stack = l :: ls := h
  have htb : t.below = s.below := rfl
  have hc := St.setTop_cells t l l' ls ht
  rw [htb] at hc
  have hc0 := St.cells_top s l ls h
  have hagree : ∀ i, i < s.below.length + k → (t.setTop l').cells[i]? = s.cells[i]? := by
    intro i hi
    rw [hc, hc0]
    by_cases hb : i < s.below.length
    · rw [List.getElem?_append_left hb, List.getElem?_append_left hb]
    · rw [List.getElem?_append_right (by omega), List.getElem?_append_right (by omega)]
      have hik : i - s.below.length < k := by omega
      rw [show l'.cells[i - s.below.length]? = (l'.cells.take k)[i - s.below.length]? by
            rw [List.getElem?_take, if_pos hik],
        show l.cells[i - s.below.length]? = (l.cells.take k)[i - s.below.length]? by
            rw [List.getElem?_take, if_pos hik], hpre]
  have hjobs : (t.setTop l').jobs = s.jobs ++ [j] := by rw [St.setTop_jobs]
  refine ⟨?_, ?_, ?_, ?_⟩
  · intro j' hj' kk hkk
    rw [hjobs] at hj'
    rcases List.mem_append.mp hj' with hj' | hj'
    · obtain ⟨idx, hidx⟩ := hok.inplace j' hj' kk hkk
      have := hjob j' hj'
      exact ⟨idx, by rw [hagree _ (by omega)]; exact hidx⟩
    · simp at hj'; subst hj'
      obtain ⟨idx, hidx⟩ := hhole kk hkk
      refine ⟨idx, ?_⟩
      rw [hc, hdst, List.getElem?_append_right (by omega)]
      rw [show s.below.length + k + kk - s.below.length = k + kk by omega]; exact hidx
  · intro p v id idx hp
    rw [hjobs]
    by_cases hpk : p < s.below.length + k
    · rw [hagree p hpk] at hp
      obtain ⟨j', hj', hid, h1, h2⟩ := hok.owned p v id idx hp
      exact ⟨j', List.mem_append_left _ hj', hid, h1, h2⟩
    · rw [hc, List.getElem?_append_right (by omega)] at hp
      have hlt : p - s.below.length < l'.cells.length := by
        rcases Nat.lt_or_ge (p - s.below.length) l'.cells.length with h1 | h1
        · exact h1
        · rw [List.getElem?_eq_none h1] at hp; cases hp
      by_cases hin : p - s.below.length < k + j.out.length
      · obtain ⟨idx2, h2⟩ := hhole (p - s.below.length - k) (by omega)
        rw [show k + (p - s.below.length - k) = p - s.below.length by omega, hp] at h2
        simp at h2
        exact ⟨j, by simp, h2.2.1.symm, by omega, by omega⟩
      · have := hnew (p - s.below.length) (by omega) hlt
        rw [List.getElem?_eq_getElem hlt] at hp
        simp only [Option.some.injEq] at hp
        rw [hp] at this; simp [Cell.isReady] at this
  · rw [hjobs, List.map_append, List.nodup_append]
    refine ⟨hok.distinct, by simp, ?_⟩
    intro a ha b hb
    simp at hb; subst hb
    obtain ⟨j', hj', rfl⟩ := List.mem_map.mp ha
    exact hfresh j' hj'
  · intro j' hj'
    rw [hjobs] at hj'
    rw [St.setTop_jobMaxEnd]
    show j'.dst + j'.out.length ≤ max s.jobMaxEnd (j.dst + j.out.length)
    rcases List.mem_append.mp hj' with hj' | hj'
    · have := hok.maxEnd j' hj'; omega
    · simp at hj'; subst hj'; omega

/-! ## Enqueueing -/

/-- Draining the oldest job keeps the view and the invariants. -/
theorem drainOldest_topOK (s : St C) (ht : TopOK s) :
    s.drainOldest.view = s.view ∧ TopOK s.drainOldest ∧ s.drainOldest.nextJob = s.nextJob ∧
    s.drainOldest.hashErr = s.hashErr ∧ s.drainOldest.cells.length = s.cells.length := by
  cases hj : s.jobs with
  | nil =>
    have : s.drainOldest = s := by unfold St.drainOldest; rw [hj]
    rw [this]; exact ⟨rfl, ht, rfl, rfl, rfl⟩
  | cons j js =>
    obtain ⟨hd, hok'⟩ := drainOldest_spec s j js hj ht.holes
    have hlen : (fillAt j.dst j.out s.cells).length = s.cells.length := fillAt_length _ _ _
    obtain ⟨_, _, hcells, _, herr, hherr, hnext, _⟩ := St.setCells_spec s _ hlen
    rw [hd]
    refine ⟨by rw [view_with_jobs, view_setCells s _ hlen (fillAt_vals _ _ _)], ⟨hok', ?_, ?_, ?_⟩, ?_, ?_, ?_⟩
    · show (s.setCells (fillAt j.dst j.out s.cells)).err = none; rw [herr]; exact ht.err
    · intro j' hj'; exact ht.jobsne j' (by rw [hj]; exact List.mem_cons_of_mem _ hj')
    · intro j' hj'; show j'.id < (s.setCells (fillAt j.dst j.out s.cells)).nextJob; rw [hnext]
      exact ht.ids j' (by rw [hj]; exact List.mem_cons_of_mem _ hj')
    · show (s.setCells (fillAt j.dst j.out s.cells)).nextJob = s.nextJob; exact hnext
    · show (s.setCells (fillAt j.dst j.out s.cells)).hashErr = s.hashErr; exact hherr
    · show (s.setCells (fillAt j.dst j.out s.cells)).cells.length = s.cells.length; rw [hcells, hlen]

/-- The ring-full drain at the start of an enqueue. -/
def St.ringDrain (P : Params) (s : St C) : St C := if s.jobs.length ≥ P.ring then s.drainOldest else s

theorem ringDrain_topOK (P : Params) (s : St C) (ht : TopOK s) :
    (s.ringDrain P).view = s.view ∧ TopOK (s.ringDrain P) ∧ (s.ringDrain P).nextJob = s.nextJob ∧
    (s.ringDrain P).hashErr = s.hashErr ∧ (s.ringDrain P).cells.length = s.cells.length := by
  unfold St.ringDrain; split
  · exact drainOldest_topOK s ht
  · exact ⟨rfl, ht, rfl, rfl, rfl⟩

theorem ringDrain_holesSub (P : Params) (s : St C) (hok : HolesOK s) : HolesSub s.cells (s.ringDrain P).cells := by
  unfold St.ringDrain; split
  · exact (drainOldest_holesSub s hok).1
  · exact HolesSub.refl _

/-- **An enqueue with a ready input**: the ring-full drain, then the job
    appended with the reduction of its input. -/
theorem enqueue_eq (cx : Ctx C) (s : St C) (input : List (Cell C)) (out dst : Nat)
    (hr : ∀ c ∈ input, c.isReady = true) :
    s.enqueue cx input out dst =
      ({ s.ringDrain cx.P with
          jobs := (s.ringDrain cx.P).jobs ++
            [⟨(s.ringDrain cx.P).nextJob, dst, reduceTo cx.H out input.length (input.map Cell.val)⟩],
          nextJob := (s.ringDrain cx.P).nextJob + 1,
          jobMaxEnd := max (s.ringDrain cx.P).jobMaxEnd (dst + out) },
       (List.range out).map (fun k => Cell.hole ((reduceTo cx.H out input.length (input.map Cell.val)).getD k cx.z0)
          (s.ringDrain cx.P).nextJob k)) := by
  unfold St.enqueue
  rw [show (if s.jobs.length ≥ cx.P.ring then s.drainOldest else s) = s.ringDrain cx.P from rfl]
  simp only [St.vals, if_pos (List.all_eq_true.mpr hr)]

/-- A cap-wide block reduces to its bounded root. -/
theorem reduceTo_cap (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (vs : List C) (hv : vs.length = cx.P.cap) :
    reduceTo cx.H 1 vs.length vs = [mroot cx.H cx.z0 cx.P.capDepth vs] := by
  have hne : vs ≠ [] := by intro e; subst e; simp at hv; have := Nat.one_le_two_pow (n := cx.P.capDepth); omega
  have := reduceTo_chunks cx.H cx.z0 1 cx.P.capDepth (by omega) vs (by rw [hv, hcap]; simp) 0 vs.length
    (Nat.zero_le _) (by rw [hv, hcap]; exact Nat.le_trans (Nat.sub_le _ _) (le_two_pow _))
  rw [chunk_roots_zero] at this
  rw [this, chunks_single hne (by rw [hv, hcap]; exact Nat.le_refl _)]
  rfl

/-- A drain only readies cells: a hole afterwards was a hole before. -/
theorem drainOldest_hole (s : St C) (ht : TopOK s) (i : Nat) (v : C) (id idx : Nat)
    (hh : s.drainOldest.cells[i]? = some (.hole v id idx)) : s.cells[i]? = some (.hole v id idx) := by
  cases hj : s.jobs with
  | nil =>
    have : s.drainOldest = s := by unfold St.drainOldest; rw [hj]
    rwa [this] at hh
  | cons j js =>
    obtain ⟨hd, _⟩ := drainOldest_spec s j js hj ht.holes
    have hlen : (fillAt j.dst j.out s.cells).length = s.cells.length := fillAt_length _ _ _
    obtain ⟨_, _, hcells, _⟩ := St.setCells_spec s _ hlen
    rw [hd] at hh
    have : (s.setCells (fillAt j.dst j.out s.cells)).cells[i]? = some (.hole v id idx) := hh
    rw [hcells, fillAt_getElem?] at this
    cases hc : s.cells[i]? with
    | none => rw [hc] at this; simp at this
    | some c =>
      rw [hc] at this
      simp only [Option.map_some, Option.some.injEq] at this
      split at this
      · cases this
      · rw [this]

theorem ringDrain_hole (P : Params) (s : St C) (ht : TopOK s) (i : Nat) (v : C) (id idx : Nat)
    (hh : (s.ringDrain P).cells[i]? = some (.hole v id idx)) : s.cells[i]? = some (.hole v id idx) := by
  unfold St.ringDrain at hh; split at hh
  · exact drainOldest_hole s ht i v id idx hh
  · exact hh

/-! ## A pre-collapse round -/

/-- A state with one more job (the record an enqueue builds). -/
def St.addJob (s : St C) (j : Job C) : St C :=
  { s with jobs := s.jobs ++ [j], nextJob := s.nextJob + 1, jobMaxEnd := max s.jobMaxEnd (j.dst + j.out.length) }

/-- The shape `precollapseCapRun` works on: `ncap` cap-depth nodes, then
    ready leaves. -/
structure PreShape (P : Params) (l : Layer C) (ncap : Nat) : Prop where
  le : ncap ≤ l.nodes.length
  caps : ∀ n ∈ l.nodes.take ncap, n.depth = P.capDepth
  leaves : ∀ n ∈ l.nodes.drop ncap, n.depth = 0 ∧ n.cell.isReady = true

theorem nodeCells_take (ns : List (Node C)) (k : Nat) : nodeCells (ns.take k) = (nodeCells ns).take k := by
  simp [nodeCells, List.map_take]

theorem nodeCells_drop (ns : List (Node C)) (k : Nat) : nodeCells (ns.drop k) = (nodeCells ns).drop k := by
  simp [nodeCells, List.map_drop]

theorem nodeCells_append (a b : List (Node C)) : nodeCells (a ++ b) = nodeCells a ++ nodeCells b := by
  simp [nodeCells]

theorem erase_take (ns : List (Node C)) (k : Nat) : (ns.take k).map Node.erase = (ns.map Node.erase).take k :=
  List.map_take

theorem erase_drop (ns : List (Node C)) (k : Nat) : (ns.drop k).map Node.erase = (ns.map Node.erase).drop k :=
  List.map_drop

/-- The cover after replacing a cap block by its root node. -/
theorem cov_capCompact (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (ns : List (Node C)) (L : List C)
    (ncap : Nat) (hcov : Cov cx.H cx.z0 true ns L) (hle : ncap ≤ ns.length)
    (hcaps : ∀ n ∈ ns.take ncap, n.depth = cx.P.capDepth) (hz : ∀ n ∈ ns.drop ncap, n.depth = 0)
    (hbig : cx.P.cap ≤ (ns.drop ncap).length) :
    Cov cx.H cx.z0 true (ns.take ncap ++
      [⟨cx.P.capDepth, .ready (mroot cx.H cx.z0 cx.P.capDepth
        (((ns.drop ncap).take cx.P.cap).map (·.cell.val)))⟩] ++ (ns.drop ncap).drop cx.P.cap) L := by
  have hcov' : Cov cx.H cx.z0 true (ns.take ncap ++ ((ns.drop ncap).take cx.P.cap ++ (ns.drop ncap).drop cx.P.cap)) L := by
    rw [List.take_append_drop, List.take_append_drop]; exact hcov
  obtain ⟨Lc, LR, hL, hc, hR, hs1⟩ := cov_split_full cx.H cx.z0 hcov'
  obtain ⟨Lb, Lr, hLR, hb, hr, _⟩ := cov_split_full cx.H cx.z0 hR
  have hbd : ∀ n ∈ (ns.drop ncap).take cx.P.cap, n.depth = 0 := fun n hn => hz n (List.mem_of_mem_take hn)
  have hblen : ((ns.drop ncap).take cx.P.cap).length = cx.P.cap := by rw [List.length_take]; omega
  have hpos : 0 < cx.P.cap := by rw [hcap]; exact Nat.one_le_two_pow
  have hbne : (ns.drop ncap).take cx.P.cap ≠ [] := by intro e; rw [e] at hblen; simp at hblen; omega
  obtain ⟨hv, _⟩ := cov_group cx.H cx.z0 hbd hb
  rw [chunk_roots_zero] at hv
  have hLblen : Lb.length = cx.P.cap := by rw [cov_full_length cx.H cx.z0 hb, leavesOf_uniform hbd, hblen]; simp
  have hnew : Cov cx.H cx.z0 true [(⟨cx.P.capDepth, .ready (mroot cx.H cx.z0 cx.P.capDepth
      (((ns.drop ncap).take cx.P.cap).map (·.cell.val)))⟩ : Node C)] Lb := by
    refine ⟨?_, ?_, ?_⟩
    · intro e; rw [e] at hLblen; simp at hLblen; omega
    · simp; rw [hLblen, hcap]
    · show mroot _ _ _ _ = mroot _ _ _ _; rw [hv]
  rw [hL, hLR, ← List.append_assoc]
  apply cov_join_full cx.H cx.z0 (cov_join_full cx.H cx.z0 hc hnew ?_) hr ?_
  · intro ha _
    simp only [List.head_cons]
    exact Nat.le_of_eq (hcaps _ (List.getLast_mem ha)).symm
  · intro _ hb'
    have := hz _ (List.mem_of_mem_drop (List.head_mem hb'))
    rw [this]; exact Nat.zero_le _

theorem layer_cells_of_erase {l l' : Layer C} (he : l'.erase = l.erase) (hp : l.prog = []) (hd : l.pend = [])
    (ht : l.tail = []) : l'.cells = nodeCells l'.nodes := by
  have h1 : l'.prog.map Cell.erase = [] := by have := congrArg Layer.prog he; simpa [Layer.erase, hp] using this
  have h2 : l'.pend.map (·.map Cell.erase) = [] := by have := congrArg Layer.pend he; simpa [Layer.erase, hd] using this
  have h3 : l'.tail.map Cell.erase = [] := by have := congrArg Layer.tail he; simpa [Layer.erase, ht] using this
  simp at h1 h2 h3
  simp [Layer.cells, h1, h2, h3]

/-- **One round of `precollapseCapRun`** on a binary scope: the cap block
    becomes one cap-depth node held by a background job whose hole is where
    Go's `dstOff` points; the scope keeps covering its leaves. -/
theorem capRound_bin (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (hcd : 0 < cx.P.capDepth)
    (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) (ncap : Nat)
    (h : s.stack = l :: ls) (hb : BinOK cx l L) (hinc : l.incremental = true) (ht : TopOK s)
    (hL : L.length < 2 ^ 40)
    (hsh : PreShape cx.P l ncap) (hbig : cx.P.cap ≤ (l.nodes.drop ncap).length) :
    ∃ l' ls', ((s.capJob cx l ncap).1.setTop
        (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2)).stack = l' :: ls' ∧
      BinOK cx l' L ∧ l'.incremental = true ∧
      TopOK ((s.capJob cx l ncap).1.setTop
        (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2)) ∧
      PreShape cx.P l' (ncap + 1) ∧ (l'.nodes.drop (ncap + 1)).length = (l.nodes.drop ncap).length - cx.P.cap ∧
      ls'.map Layer.erase = ls.map Layer.erase ∧
      ((s.capJob cx l ncap).1.setTop
        (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2)).base.map Cell.erase =
        s.base.map Cell.erase ∧
      ((s.capJob cx l ncap).1.setTop
        (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2)).hashErr = s.hashErr ∧
      ((s.capJob cx l ncap).1.setTop
        (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2)).below.length =
        s.below.length ∧
      HolesSub s.below ((s.capJob cx l ncap).1.setTop
        (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2)).below := by
  -- the block and its readiness
  have hcl : l.cells = nodeCells l.nodes := layer_cells_bin l hb.prog hb.pend hb.tail
  have hblk : ∀ c ∈ nodeCells ((l.nodes.drop ncap).take cx.P.cap), c.isReady = true := by
    intro c hc
    simp only [nodeCells, List.mem_map] at hc
    obtain ⟨n, hn, rfl⟩ := hc
    exact (hsh.leaves n (List.mem_of_mem_take hn)).2
  have hilen : (nodeCells ((l.nodes.drop ncap).take cx.P.cap)).length = cx.P.cap := by
    have := hbig; rw [List.length_drop] at this; simp [nodeCells_length]; omega
  -- the ring-full drain
  obtain ⟨hv1, ht1, hn1, hh1, hlen1⟩ := ringDrain_topOK cx.P s ht
  obtain ⟨l1, ls1, hs1, he1, hle1⟩ := stack_of_view hv1 h
  have hbl1 := below_length_of_view hv1 h hs1
  have hcl1 : l1.cells = nodeCells l1.nodes := layer_cells_of_erase he1 hb.prog hb.pend hb.tail
  have hne1 : l1.nodes.map Node.erase = l.nodes.map Node.erase := by have := congrArg Layer.nodes he1; exact this
  have hlenn : l1.nodes.length = l.nodes.length := by
    have := congrArg List.length hne1; simpa using this
  -- the job and its result
  have hout : reduceTo cx.H 1 (nodeCells ((l.nodes.drop ncap).take cx.P.cap)).length
      ((nodeCells ((l.nodes.drop ncap).take cx.P.cap)).map Cell.val) =
      [mroot cx.H cx.z0 cx.P.capDepth (((l.nodes.drop ncap).take cx.P.cap).map (·.cell.val))] := by
    have := reduceTo_cap cx hcap ((nodeCells ((l.nodes.drop ncap).take cx.P.cap)).map Cell.val) (by simp [hilen])
    simpa [nodeCells, List.map_map, Function.comp_def] using this
  generalize hvd : mroot cx.H cx.z0 cx.P.capDepth (((l.nodes.drop ncap).take cx.P.cap).map (·.cell.val)) = v at hout
  have hact : s.activeStart = s.below.length := by
    unfold St.activeStart; rw [h]; simp only; rw [hb.prog, St.topStart_eq s l ls h]; rfl
  let J : Job C := ⟨(s.ringDrain cx.P).nextJob, s.below.length + ncap, [v]⟩
  let T : St C := (s.ringDrain cx.P).addJob J
  have hcj : s.capJob cx l ncap = (T, [Cell.hole v (s.ringDrain cx.P).nextJob 0]) := by
    unfold St.capJob
    rw [enqueue_eq cx s _ 1 _ hblk, hout, hact]
    rfl
  rw [hcj]
  simp only
  have hTs : T.stack = l1 :: ls1 := hs1
  have hTtop : T.topLayer l = l1 := by simp [St.topLayer, hTs]
  rw [hTtop]
  -- the new layer
  let l' := capCompact cx.P l1 ncap [Cell.hole v (s.ringDrain cx.P).nextJob 0]
  have hl'n : l'.nodes = l1.nodes.take ncap ++ [⟨cx.P.capDepth, .hole v (s.ringDrain cx.P).nextJob 0⟩] ++
      (l1.nodes.drop ncap).drop cx.P.cap := rfl
  have hl'c : l'.cells = nodeCells l'.nodes := layer_cells_bin l' (by
      show l1.prog = []; have := congrArg Layer.prog he1; simpa [Layer.erase, hb.prog] using this) (by
      show l1.pend = []; have := congrArg Layer.pend he1; simpa [Layer.erase, hb.pend] using this) (by
      show l1.tail = []; have := congrArg Layer.tail he1; simpa [Layer.erase, hb.tail] using this)
  -- l1's leaves after the caps are ready (a drain only readies)
  have hz1 : ∀ i (hi : ncap ≤ i) (hi' : i < l1.nodes.length), (l1.nodes[i]'hi').cell.isReady = true := by
    intro i hi hi'
    cases hc : (l1.nodes[i]'hi').cell with
    | ready _ => rfl
    | hole w id idx =>
      exfalso
      have hp1 : (s.ringDrain cx.P).cells[(s.ringDrain cx.P).below.length + i]? = some (.hole w id idx) := by
        rw [St.cells_top _ l1 ls1 hs1, List.getElem?_append_right (by omega), Nat.add_sub_cancel_left, hcl1]
        simp only [nodeCells, List.getElem?_map, List.getElem?_eq_getElem hi', Option.map_some, hc]
      have hp0 := ringDrain_hole cx.P s ht _ w id idx hp1
      rw [hbl1, St.cells_top s l ls h, List.getElem?_append_right (by omega), Nat.add_sub_cancel_left, hcl] at hp0
      have hi0 : i < l.nodes.length := by omega
      simp only [nodeCells, List.getElem?_map, List.getElem?_eq_getElem hi0, Option.map_some,
        Option.some.injEq] at hp0
      have hmem : l.nodes[i]'hi0 ∈ l.nodes.drop ncap := by
        apply List.mem_of_getElem? (i := i - ncap)
        rw [List.getElem?_drop, show ncap + (i - ncap) = i by omega, List.getElem?_eq_getElem hi0]
      have hr := (hsh.leaves (l.nodes[i]'hi0) hmem).2
      rw [hp0] at hr; cases hr
  have hdep1 : l1.nodes.map (·.depth) = l.nodes.map (·.depth) := by
    have := congrArg (List.map (·.depth)) hne1; simpa [Node.erase, List.map_map, Function.comp_def] using this
  have hdepi : ∀ i (h1 : i < l1.nodes.length) (h0 : i < l.nodes.length), (l1.nodes[i]'h1).depth = (l.nodes[i]'h0).depth := by
    intro i h1 h0
    have := congrArg (fun xs => xs[i]?) hdep1
    simp only [List.getElem?_map, List.getElem?_eq_getElem h1, List.getElem?_eq_getElem h0, Option.map_some,
      Option.some.injEq] at this
    exact this
  -- depths of the parts of l1
  have hcaps1 : ∀ n ∈ l1.nodes.take ncap, n.depth = cx.P.capDepth := by
    intro n hn
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hn
    simp only [List.getElem_take]
    have hi1 : i < l1.nodes.length := by simp at hi; omega
    rw [hdepi i hi1 (by omega)]
    apply hsh.caps
    apply List.mem_of_getElem? (i := i)
    rw [List.getElem?_take, if_pos (by simp at hi; omega), List.getElem?_eq_getElem (by omega)]
  have hz1d : ∀ n ∈ l1.nodes.drop ncap, n.depth = 0 ∧ n.cell.isReady = true := by
    intro n hn
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hn
    simp only [List.getElem_drop]
    have hi1 : ncap + i < l1.nodes.length := by simp at hi; omega
    refine ⟨?_, hz1 _ (by omega) hi1⟩
    rw [hdepi _ hi1 (by omega)]
    apply (hsh.leaves _ _).1
    apply List.mem_of_getElem? (i := i)
    rw [List.getElem?_drop, List.getElem?_eq_getElem (by omega)]
  -- the new layer's nodes, cells and cover
  have hlen1n : ncap ≤ l1.nodes.length := by rw [hlenn]; exact hsh.le
  have hd1 : cx.P.cap ≤ (l1.nodes.drop ncap).length := by
    rw [List.length_drop, hlenn]; have := hbig; rw [List.length_drop] at this; exact this
  have hl'cells : l'.cells = (nodeCells l1.nodes).take ncap ++ [Cell.hole v (s.ringDrain cx.P).nextJob 0] ++
      ((nodeCells l1.nodes).drop ncap).drop cx.P.cap := by
    rw [hl'c, hl'n, nodeCells_append, nodeCells_append, nodeCells_take, nodeCells_drop, nodeCells_drop]; rfl
  have hd1' : cx.P.cap ≤ l1.nodes.length - ncap := by simpa using hd1
  have hl'len : l'.cells.length = l1.nodes.length - cx.P.cap + 1 := by
    rw [hl'cells]
    simp only [List.length_append, List.length_take, List.length_drop, nodeCells_length, List.length_singleton]
    omega
  have hNe : l'.nodes.map Node.erase = (l.nodes.take ncap ++ [(⟨cx.P.capDepth, .ready v⟩ : Node C)] ++
      (l.nodes.drop ncap).drop cx.P.cap).map Node.erase := by
    rw [hl'n]
    simp only [List.map_append, erase_take, erase_drop, hne1]
    rfl
  have hcov' : Cov cx.H cx.z0 true l'.nodes L := by
    apply cov_of_erase_eq cx.H cx.z0 hNe
    have := cov_capCompact cx hcap l.nodes L ncap hb.cov hsh.le hsh.caps (fun n hn => (hsh.leaves n hn).1) hbig
    rwa [hvd] at this
  have htk : (l1.nodes.take ncap).length = ncap := by simp; omega
  have hsplit : l'.nodes = (l1.nodes.take ncap ++ [⟨cx.P.capDepth, .hole v (s.ringDrain cx.P).nextJob 0⟩]) ++
      (l1.nodes.drop ncap).drop cx.P.cap := hl'n
  have hlen2 : (l1.nodes.take ncap ++ [(⟨cx.P.capDepth, .hole v (s.ringDrain cx.P).nextJob 0⟩ : Node C)]).length =
      ncap + 1 := by simp [htk]
  refine ⟨l', ls1, St.setTop_stack T l1 l' ls1 hTs, ⟨?_, ?_, ?_, ?_, ?_, ?_, hcov', (by intro hc; cases hc),
    cov_depth_lt cx.H cx.z0 hcov' hL, ?_, fun _ => by rw [hl'n]; simp⟩,
    (by have e := congrArg Layer.incremental he1; exact e.trans hinc), ⟨?_, ?_, ?_, ?_⟩, ⟨?_, ?_, ?_⟩, ?_, hle1,
    ?_, ?_, ?_, ?_⟩
  · intro hi; exfalso
    have e : l1.incremental = l.incremental := by have := congrArg Layer.incremental he1; exact this
    have : l1.incremental = false := hi
    rw [e, hinc] at this; cases this
  · show l1.progressive = false; have := congrArg Layer.progressive he1; exact this.trans hb.noprog
  · show l1.legacy = false; have := congrArg Layer.legacy he1; exact this.trans hb.nolegacy
  · show l1.prog = []; have := congrArg Layer.prog he1; simpa [Layer.erase, hb.prog] using this
  · show l1.pend = []; have := congrArg Layer.pend he1; simpa [Layer.erase, hb.pend] using this
  · show l1.tail = []; have := congrArg Layer.tail he1; simpa [Layer.erase, hb.tail] using this
  · -- holes only at cap depth
    intro n hn hnr
    rw [hl'n] at hn
    rcases List.mem_append.mp hn with hn | hn
    · rcases List.mem_append.mp hn with hn | hn
      · rw [hcaps1 n hn]; exact hcd
      · simp at hn; rw [hn]; exact hcd
    · rw [(hz1d n (List.mem_of_mem_drop hn)).2] at hnr; cases hnr
  · -- the hole invariant, with the new job
    show HolesOK (St.setTop ((s.ringDrain cx.P).addJob J) l')
    unfold St.addJob
    apply holesOK_addJob _ l1 l' ls1 hs1 ht1.holes J
      (fun j' hj' => Nat.ne_of_lt (ht1.ids j' hj')) ncap (by rw [hcl1, nodeCells_length]; exact hlen1n)
    · rw [hl'cells, hcl1]; simp [List.take_append_of_le_length, nodeCells_length, hlen1n]
    · show s.below.length + ncap = _; rw [hbl1]
    · intro i hi
      have hi0 : i = 0 := by simp [J] at hi; omega
      subst hi0
      refine ⟨0, ?_⟩
      rw [hl'cells, Nat.add_zero, List.getElem?_append_left (by simp [nodeCells_length]; omega),
        List.getElem?_append_right (by simp [nodeCells_length]; omega)]
      simp [nodeCells_length, Nat.min_eq_left hlen1n, J]
    · rw [hl'len]; simp [J]; omega
    · intro i hi hi'
      have hi2 : i - (ncap + 1) < (((nodeCells l1.nodes).drop ncap).drop cx.P.cap).length := by
        rw [hl'len] at hi'; simp [nodeCells_length]; simp [J] at hi; omega
      have : l'.cells[i]'hi' = (((nodeCells l1.nodes).drop ncap).drop cx.P.cap)[i - (ncap + 1)]'hi2 := by
        simp only [hl'cells]
        rw [List.getElem_append_right (by simp [nodeCells_length]; simp [J] at hi; omega)]
        congr 1; simp [nodeCells_length, Nat.min_eq_left hlen1n]
      rw [this]
      simp only [List.getElem_drop, nodeCells, List.getElem_map]
      exact hz1 _ (by omega) _
    · intro j' hj'
      have hne := ht1.jobsne j' hj'
      have hpos : 0 < j'.out.length := List.length_pos_iff.mpr hne
      obtain ⟨idx, hidx⟩ := ht1.holes.inplace j' hj' (j'.out.length - 1) (by omega)
      rcases Nat.lt_or_ge (j'.dst + (j'.out.length - 1)) ((s.ringDrain cx.P).below.length + ncap) with hlt | hge
      · omega
      · exfalso
        rw [St.cells_top _ l1 ls1 hs1, List.getElem?_append_right (by omega), hcl1] at hidx
        have hk1 : j'.dst + (j'.out.length - 1) - (s.ringDrain cx.P).below.length < l1.nodes.length := by
          rcases Nat.lt_or_ge (j'.dst + (j'.out.length - 1) - (s.ringDrain cx.P).below.length) l1.nodes.length with h1 | h1
          · exact h1
          · rw [List.getElem?_eq_none (by rw [nodeCells_length]; exact h1)] at hidx; cases hidx
        simp only [nodeCells, List.getElem?_map, List.getElem?_eq_getElem hk1, Option.map_some,
          Option.some.injEq] at hidx
        have := hz1 _ (by omega) hk1
        rw [hidx] at this; cases this
  · rw [St.setTop_err]; exact ht1.err
  · intro j' hj'
    rw [St.setTop_jobs] at hj'
    change j' ∈ (s.ringDrain cx.P).jobs ++ [J] at hj'
    rcases List.mem_append.mp hj' with hj' | hj'
    · exact ht1.jobsne j' hj'
    · simp at hj'; subst hj'; simp [J]
  · intro j' hj'
    rw [St.setTop_jobs] at hj'
    change j' ∈ (s.ringDrain cx.P).jobs ++ [J] at hj'
    rw [St.setTop_nextJob]
    change j'.id < (s.ringDrain cx.P).nextJob + 1
    rcases List.mem_append.mp hj' with hj' | hj'
    · have := ht1.ids j' hj'; omega
    · simp at hj'; subst hj'; simp [J]
  · -- the shape for the next round
    rw [hsplit, List.length_append, hlen2]; omega
  · intro n hn
    rw [hsplit, List.take_append_of_le_length (by rw [hlen2]; exact Nat.le_refl _), List.take_of_length_le (by rw [hlen2]; exact Nat.le_refl _)] at hn
    rcases List.mem_append.mp hn with hn | hn
    · exact hcaps1 n hn
    · simp at hn; rw [hn]
  · intro n hn
    rw [hsplit, List.drop_append_of_le_length (by rw [hlen2]; exact Nat.le_refl _), List.drop_of_length_le (by rw [hlen2]; exact Nat.le_refl _),
      List.nil_append] at hn
    exact hz1d n (List.mem_of_mem_drop hn)
  · rw [hsplit, List.drop_append_of_le_length (by rw [hlen2]; exact Nat.le_refl _), List.drop_of_length_le (by rw [hlen2]; exact Nat.le_refl _),
      List.nil_append]
    simp; omega
  · show (T.setTop l').base.map Cell.erase = s.base.map Cell.erase
    rw [St.setTop_base]
    exact congrArg Prod.fst hv1
  · rw [St.setTop_hashErr]; exact hh1
  · rw [St.setTop_below T l1 l' ls1 hTs]; exact hbl1
  · rw [St.setTop_below T l1 l' ls1 hTs]
    show HolesSub s.below (s.ringDrain cx.P).below
    exact holesSub_below hv1 h hs1 (ringDrain_holesSub cx.P s ht.holes)

/-! ## The whole pre-collapse -/

/-- Replacing the innermost layer by one with the same cells keeps everything. -/
theorem topOK_setTop_same (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (ht : TopOK s) (hc : l'.cells = l.cells) : TopOK (s.setTop l') := by
  refine ⟨?_, by rw [St.setTop_err]; exact ht.err, fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
    fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩
  apply holesOK_setTop s l l' ls h ht.holes l.cells.length (Nat.le_refl _) (by rw [hc]) (by rw [hc]; exact Nat.le_refl _)
  · intro i hi hi'; rw [hc] at hi'; omega
  · intro j hj
    have := job_end_le s ht.holes j hj (ht.jobsne j hj)
    rw [St.cells_top s l ls h, List.length_append] at this; exact this

/-- `normTop` on a binary scope: the nodes become counted, nothing else changes. -/
theorem normTop_bin (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hb : BinOK cx l L) (ht : TopOK s) :
    s.normTop.stack = { l with counted := l.nodes.length } :: ls ∧
    BinOK cx { l with counted := l.nodes.length } L ∧ TopOK s.normTop ∧
    s.normTop.below = s.below ∧ s.normTop.base = s.base ∧ s.normTop.hashErr = s.hashErr := by
  have hl : (s.uncollapsedAsLeaves l).2 = { l with counted := l.nodes.length } := by
    rw [uncollapsedAsLeaves_layer, uncollapsedAsLeaves_nodes s l hb.leaves]
  have hs : (s.uncollapsedAsLeaves l).1 = s := uncollapsedAsLeaves_state s l hb.leaves
  have hn : s.normTop = s.setTop { l with counted := l.nodes.length } := by
    unfold St.normTop; rw [h]; simp only; rw [hs, hl]
  rw [hn]
  refine ⟨St.setTop_stack s l _ ls h, ⟨hb.ninc, hb.noprog, hb.nolegacy, hb.prog, hb.pend, hb.tail, hb.cov, hb.leaves,
    hb.shallow, hb.holes, hb.nonempty⟩, topOK_setTop_same s l _ ls h ht rfl, St.setTop_below s l _ ls h,
    St.setTop_base s _, St.setTop_hashErr s _⟩

/-- A cover whose depths are `0` or `d > 0` leads with its depth-`d` nodes. -/
theorem preShape_of_cov (cx : Ctx C) (l : Layer C) (L : List C) (hb : BinOK cx l L) (hcd : 0 < cx.P.capDepth)
    (hd : ∀ n ∈ l.nodes, n.depth = 0 ∨ n.depth = cx.P.capDepth) :
    PreShape cx.P l (capCount cx.P l) ∧
    ((l.nodes.take (capCount cx.P l)).all (·.depth == cx.P.capDepth) &&
      (l.nodes.drop (capCount cx.P l)).all (·.depth == 0)) = true := by
  obtain ⟨A, G, B, heq, hA, hG, hB⟩ := cov_split3 cx.H cx.z0 cx.P.capDepth hb.cov
  have hA0 : A = [] := by
    cases A with
    | nil => rfl
    | cons a as =>
      have h1 := hA a (by simp)
      rcases hd a (by rw [heq]; simp) with h2 | h2 <;> omega
  subst hA0
  simp only [List.nil_append] at heq
  have hcc : capCount cx.P l = G.length := by
    unfold capCount; rw [heq, List.filter_append,
      List.filter_eq_self.mpr (fun n hn => by simp [hG n hn]),
      List.filter_eq_nil_iff.mpr (fun n hn => by simp; have := hB n hn; omega)]
    simp
  have htake : l.nodes.take (capCount cx.P l) = G := by rw [hcc, heq]; simp
  have hdrop : l.nodes.drop (capCount cx.P l) = B := by rw [hcc, heq]; simp
  have hB0 : ∀ n ∈ B, n.depth = 0 := by
    intro n hn; rcases hd n (by rw [heq]; simp [hn]) with h | h
    · exact h
    · have := hB n hn; omega
  refine ⟨⟨by rw [hcc, heq]; simp, by rw [htake]; exact hG, ?_⟩, ?_⟩
  · intro n hn
    rw [hdrop] at hn
    refine ⟨hB0 n hn, ?_⟩
    cases hr : n.cell.isReady with
    | true => rfl
    | false =>
      have := hb.holes n (by rw [heq]; simp [hn]) hr
      have := hB0 n hn; omega
  · rw [htake, hdrop]
    simp only [Bool.and_eq_true, List.all_eq_true]
    exact ⟨fun n hn => by simp [hG n hn], fun n hn => by simp [hB0 n hn]⟩

/-- **The rounds of `precollapseCapRun`** keep a binary scope's invariants. -/
theorem precollapseLoop_bin (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (hcd : 0 < cx.P.capDepth)
    (L : List C) (hL : L.length < 2 ^ 40) :
    ∀ fuel (s : St C) (l : Layer C) (ls : List (Layer C)) (ncap : Nat), s.stack = l :: ls → BinOK cx l L →
      l.incremental = true → TopOK s → PreShape cx.P l ncap →
      ∃ l' ls', (precollapseLoop cx fuel s ncap).stack = l' :: ls' ∧ BinOK cx l' L ∧ l'.incremental = true ∧
        TopOK (precollapseLoop cx fuel s ncap) ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
        (precollapseLoop cx fuel s ncap).base.map Cell.erase = s.base.map Cell.erase ∧
        (precollapseLoop cx fuel s ncap).hashErr = s.hashErr ∧
        (precollapseLoop cx fuel s ncap).below.length = s.below.length ∧
        HolesSub s.below (precollapseLoop cx fuel s ncap).below := by
  intro fuel
  induction fuel with
  | zero => intro s l ls ncap h hb hi ht _; exact ⟨l, ls, h, hb, hi, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩
  | succ fuel ih =>
    intro s l ls ncap h hb hi ht hsh
    simp only [precollapseLoop, h]
    split
    · exact ⟨l, ls, h, hb, hi, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩
    · rename_i hbig
      obtain ⟨l', ls', hs', hb', hi', ht', hsh', _, hle', hbase', hh', hbl', hsub'⟩ :=
        capRound_bin cx hcap hcd s l ls L ncap h hb hi ht hL hsh (by omega)
      obtain ⟨l'', ls'', hs'', hb'', hi'', ht'', hle'', hbase'', hh'', hbl'', hsub''⟩ :=
        ih _ l' ls' (ncap + 1) hs' hb' hi' ht' hsh'
      exact ⟨l'', ls'', hs'', hb'', hi'', ht'', by rw [hle'', hle'], by rw [hbase'', hbase'],
        by rw [hh'', hh'], by rw [hbl'', hbl'], HolesSub.trans hsub' hsub''⟩

/-- **Asynchronous `Collapse` on a binary scope** (`precollapseCapRun`)
    keeps every invariant: whatever cap blocks become background jobs, the
    scope covers its leaves and every hole is where its job will write. -/
theorem precollapse_bin (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (hcd : 0 < cx.P.capDepth)
    (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hb : BinOK cx l L) (hinc : l.incremental = true) (ht : TopOK s)
    (hL : L.length < 2 ^ 40) :
    ∃ l' ls', (s.precollapse cx).1.stack = l' :: ls' ∧ BinOK cx l' L ∧ l'.incremental = true ∧
      TopOK (s.precollapse cx).1 ∧
      ls'.map Layer.erase = ls.map Layer.erase ∧
      (s.precollapse cx).1.base.map Cell.erase = s.base.map Cell.erase ∧
      (s.precollapse cx).1.hashErr = s.hashErr ∧ (s.precollapse cx).1.below.length = s.below.length ∧
      HolesSub s.below (s.precollapse cx).1.below := by
  obtain ⟨hns, hb0, ht0, hbl0, hba0, hh0⟩ := normTop_bin cx s l ls L h hb ht
  unfold St.precollapse
  rw [hns]
  simp only
  split
  · exact ⟨_, ls, hns, hb0, hinc, ht0, rfl, by rw [hba0], hh0, by rw [hbl0], by rw [hbl0]; exact HolesSub.refl _⟩
  · rename_i hany
    have hd : ∀ n ∈ ({ l with counted := l.nodes.length } : Layer C).nodes,
        n.depth = 0 ∨ n.depth = cx.P.capDepth := by
      intro n hn
      by_cases h0 : n.depth = 0
      · exact Or.inl h0
      by_cases h1 : n.depth = cx.P.capDepth
      · exact Or.inr h1
      exfalso
      apply hany
      rw [List.any_eq_true]
      exact ⟨n, hn, by simp [h0, h1]⟩
    obtain ⟨hsh, hok⟩ := preShape_of_cov cx _ L hb0 hcd hd
    rw [hok]
    simp only [St.failUnless, if_true]
    obtain ⟨l', ls', hs', hb', hi', ht', hle', hba', hh', hbl', hsub'⟩ :=
      precollapseLoop_bin cx hcap hcd L hL _ s.normTop _ ls _ hns hb0 hinc ht0 hsh
    exact ⟨l', ls', hs', hb', hi', ht', hle', by rw [hba', hba0], by rw [hh', hh0], by rw [hbl', hbl0],
      by rw [← hbl0]; exact hsub'⟩

end Ssz.Hasher