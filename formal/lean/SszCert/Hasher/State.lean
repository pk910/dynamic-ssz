import SszCert.Hasher.Nodes

/-!
# Hasher state: erasure, shapes and drains

Values of the hasher model never depend on holes (a hole carries the value
its job will write), so the reduction is reasoned about on the *erased*
state, every cell made ready. This file relates the buffer operations to it:
rebuilding a region from its cells (`setCells`) keeps its shape, and a drain
of a job whose hole is in place changes nothing but readiness.
-/

namespace Ssz.Hasher

variable {C : Type}

/-! ## Erasure -/

def Cell.erase (c : Cell C) : Cell C := .ready c.val

@[simp] theorem Cell.val_erase (c : Cell C) : c.erase.val = c.val := rfl

def Node.erase (n : Node C) : Node C := { n with cell := n.cell.erase }

/-- A layer with every cell ready. -/
def Layer.erase (l : Layer C) : Layer C :=
  { l with prog := l.prog.map Cell.erase, nodes := l.nodes.map Node.erase,
           pend := l.pend.map (·.map Cell.erase), tail := l.tail.map Cell.erase }

/-! ## `setCells` keeps a layer's shape -/

/-- The cells of a layer's deferred run, flattened. -/
theorem splitPend_spec (pend : List (List (Cell C))) (r : List (Cell C))
    (h : (pend.map List.length).sum ≤ r.length) :
    (Layer.setCells.splitPend pend r).1.map List.length = pend.map List.length ∧
    (Layer.setCells.splitPend pend r).1.flatten ++ (Layer.setCells.splitPend pend r).2 = r := by
  induction pend generalizing r with
  | nil => simp [Layer.setCells.splitPend]
  | cons e es ih =>
    simp only [List.map_cons, List.sum_cons] at h
    have h' : (es.map List.length).sum ≤ (r.drop e.length).length := by simp; omega
    obtain ⟨ih1, ih2⟩ := ih (r.drop e.length) h'
    simp only [Layer.setCells.splitPend, List.map_cons, List.flatten_cons, List.append_assoc, ih1, ih2]
    refine ⟨?_, List.take_append_drop _ _⟩
    simp; omega

theorem nodeCells_length (ns : List (Node C)) : (nodeCells ns).length = ns.length := by
  simp [nodeCells]

theorem Layer.cells_length (l : Layer C) :
    l.cells.length = l.prog.length + l.nodes.length + (l.pend.map List.length).sum + l.tail.length := by
  simp [Layer.cells, nodeCells_length, List.length_flatten, Nat.add_assoc]

/-- Two layers with the same shape: everything but the cells' contents
    (readiness and values) agrees. -/
structure Layer.SameShape (l l' : Layer C) : Prop where
  incremental : l'.incremental = l.incremental
  legacy : l'.legacy = l.legacy
  progressive : l'.progressive = l.progressive
  packed : l'.packed = l.packed
  collapsed : l'.collapsed = l.collapsed
  level : l'.level = l.level
  counted : l'.counted = l.counted
  pendC : l'.pendC = l.pendC
  prog : l'.prog.length = l.prog.length
  depths : l'.nodes.map (·.depth) = l.nodes.map (·.depth)
  pend : l'.pend.map List.length = l.pend.map List.length
  tail : l'.tail.length = l.tail.length

theorem zip_map_cells (ns : List (Node C)) (cs : List (Cell C)) (h : cs.length = ns.length) :
    nodeCells ((ns.zip cs).map (fun (n, c) => { n with cell := c })) = cs ∧
    ((ns.zip cs).map (fun (n, c) => ({ n with cell := c } : Node C))).map (·.depth) = ns.map (·.depth) := by
  induction ns generalizing cs with
  | nil => cases cs with
    | nil => simp [nodeCells]
    | cons _ _ => simp at h
  | cons n ns ih =>
    cases cs with
    | nil => simp at h
    | cons c cs =>
      simp only [List.length_cons, Nat.add_right_cancel_iff] at h
      obtain ⟨h1, h2⟩ := ih cs h
      simp only [List.zip_cons_cons, List.map_cons, nodeCells] at h1 h2 ⊢
      exact ⟨by rw [h1], by rw [h2]⟩

/-- **`setCells` keeps a layer's shape** and installs exactly the given cells. -/
theorem Layer.setCells_spec (l : Layer C) (cs : List (Cell C)) (h : cs.length = l.cells.length) :
    Layer.SameShape l (l.setCells cs) ∧ (l.setCells cs).cells = cs := by
  have hl := Layer.cells_length l
  have hprog : (cs.take l.prog.length).length = l.prog.length := by simp; omega
  have hnodes : ((cs.drop l.prog.length).take l.nodes.length).length = l.nodes.length := by simp; omega
  obtain ⟨hz1, hz2⟩ := zip_map_cells l.nodes _ hnodes
  have hrest : (l.pend.map List.length).sum ≤ ((cs.drop l.prog.length).drop l.nodes.length).length := by
    simp; omega
  obtain ⟨hp1, hp2⟩ := splitPend_spec l.pend _ hrest
  unfold Layer.setCells
  simp only
  refine ⟨⟨rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, hprog, hz2, hp1, ?_⟩, ?_⟩
  · have := congrArg List.length hp2
    simp only [List.length_append, List.length_flatten] at this
    have e : (List.map List.length (Layer.setCells.splitPend l.pend
        ((cs.drop l.prog.length).drop l.nodes.length)).1).sum = (l.pend.map List.length).sum := by
      rw [hp1]
    rw [e] at this
    simp at this ⊢; omega
  · simp only [Layer.cells, hz1, List.append_assoc, hp2]
    rw [List.take_append_drop, List.take_append_drop]

/-- Pointwise `SameShape` of two layer lists. -/
def Layer.SameShapes : List (Layer C) → List (Layer C) → Prop
  | [], [] => True
  | l :: ls, l' :: ls' => Layer.SameShape l l' ∧ Layer.SameShapes ls ls'
  | _, _ => False

theorem SameShapes_append {a b a' b' : List (Layer C)} (h1 : Layer.SameShapes a a')
    (h2 : Layer.SameShapes b b') : Layer.SameShapes (a ++ b) (a' ++ b') := by
  induction a generalizing a' with
  | nil => cases a' with
    | nil => simpa
    | cons _ _ => exact h1.elim
  | cons l ls ih => cases a' with
    | nil => exact h1.elim
    | cons l' ls' => exact ⟨h1.1, ih h1.2⟩

theorem SameShapes_reverse {a a' : List (Layer C)} (h : Layer.SameShapes a a') :
    Layer.SameShapes a.reverse a'.reverse := by
  induction a generalizing a' with
  | nil => cases a' with
    | nil => trivial
    | cons _ _ => exact h.elim
  | cons l ls ih => cases a' with
    | nil => exact h.elim
    | cons l' ls' =>
      simp only [List.reverse_cons]
      exact SameShapes_append (ih h.2) ⟨h.1, trivial⟩

theorem setCells_go_spec (ls : List (Layer C)) (r : List (Cell C))
    (h : r.length = (ls.map (fun l => l.cells.length)).sum) :
    Layer.SameShapes ls (St.setCells.go ls r) ∧
    ((St.setCells.go ls r).map Layer.cells).flatten = r := by
  induction ls generalizing r with
  | nil =>
    simp at h
    subst h
    simp [St.setCells.go, Layer.SameShapes]
  | cons l ls ih =>
    simp only [List.map_cons, List.sum_cons] at h
    have h1 : (r.take l.cells.length).length = l.cells.length := by simp; omega
    have h2 : (r.drop l.cells.length).length = (ls.map (fun l => l.cells.length)).sum := by simp; omega
    obtain ⟨hs, hc⟩ := Layer.setCells_spec l _ h1
    obtain ⟨ih1, ih2⟩ := ih _ h2
    simp only [St.setCells.go, List.map_cons, List.flatten_cons, hc, ih2]
    exact ⟨⟨hs, ih1⟩, List.take_append_drop _ _⟩

theorem St.cells_length (s : St C) :
    s.cells.length = s.base.length + (s.stack.reverse.map (fun l => l.cells.length)).sum := by
  simp only [St.cells, List.length_append, List.length_flatten, List.map_map]
  rfl

/-- **`St.setCells` keeps the state's shape** and installs exactly the given cells. -/
theorem St.setCells_spec (s : St C) (cs : List (Cell C)) (h : cs.length = s.cells.length) :
    (s.setCells cs).base.length = s.base.length ∧
    Layer.SameShapes s.stack (s.setCells cs).stack ∧
    (s.setCells cs).cells = cs ∧
    (s.setCells cs).jobs = s.jobs ∧ (s.setCells cs).err = s.err ∧ (s.setCells cs).hashErr = s.hashErr ∧
    (s.setCells cs).nextJob = s.nextJob ∧ (s.setCells cs).jobMaxEnd = s.jobMaxEnd := by
  have hl := St.cells_length s
  have hr : (cs.drop s.base.length).length = (s.stack.reverse.map (fun l => l.cells.length)).sum := by
    rw [List.length_drop, h, hl]; omega
  obtain ⟨hs, hc⟩ := setCells_go_spec s.stack.reverse _ hr
  unfold St.setCells
  simp only
  refine ⟨by rw [List.length_take, h, hl]; omega, ?_, ?_, by simp, by simp, by simp, by simp, by simp⟩
  · have := SameShapes_reverse hs
    simpa using this
  · simp only [St.cells, List.reverse_reverse, hc, List.take_append_drop]

/-! ## Writing a job's results into its hole -/

/-- The cells at `p, p+1, …` are holes of job `job`, carrying `vs`. -/
def HoleAt (job : Nat) (p : Nat) (vs : List C) (cs : List (Cell C)) : Prop :=
  ∀ k (hk : k < vs.length), ∃ idx, cs[p + k]? = some (.hole (vs[k]'hk) job idx)

/-- Readying the holes: what `writeHole` produces when they are in place. -/
def fillAt (p : Nat) (vs : List C) (cs : List (Cell C)) : List (Cell C) :=
  cs.mapIdx (fun i c => if p ≤ i ∧ i < p + vs.length then .ready c.val else c)

theorem fillAt_nil (p : Nat) (cs : List (Cell C)) : fillAt p ([] : List C) cs = cs := by
  apply List.ext_getElem (by simp [fillAt]); intro i _ _
  simp only [fillAt, List.getElem_mapIdx, List.length_nil, Nat.add_zero]
  rw [if_neg (by omega)]

theorem fillAt_cons_succ (p : Nat) (vs : List C) (c : Cell C) (cs : List (Cell C)) :
    fillAt (p + 1) vs (c :: cs) = c :: fillAt p vs cs := by
  apply List.ext_getElem (by simp [fillAt]); intro i _ _
  cases i with
  | zero => simp only [fillAt, List.getElem_mapIdx, List.getElem_cons_zero]; rw [if_neg (by omega)]
  | succ i =>
    simp only [fillAt, List.getElem_mapIdx, List.getElem_cons_succ]
    by_cases h : p ≤ i ∧ i < p + vs.length
    · rw [if_pos (by omega), if_pos h]
    · rw [if_neg (by omega), if_neg h]

theorem fillAt_cons_zero (v : C) (vs : List C) (c : Cell C) (cs : List (Cell C)) :
    fillAt 0 (v :: vs) (c :: cs) = .ready c.val :: fillAt 0 vs cs := by
  apply List.ext_getElem (by simp [fillAt]); intro i _ _
  cases i with
  | zero => simp only [fillAt, List.getElem_mapIdx, List.getElem_cons_zero]; rw [if_pos (by simp)]
  | succ i =>
    simp only [fillAt, List.getElem_mapIdx, List.getElem_cons_succ, List.length_cons]
    by_cases h : i < vs.length
    · rw [if_pos (by omega), if_pos (by omega)]
    · rw [if_neg (by omega), if_neg (by omega)]

theorem writeHole_spec (job : Nat) : ∀ (p : Nat) (vs : List C) (cs : List (Cell C)),
    HoleAt job p vs cs → writeHole job p vs cs = (fillAt p vs cs, true) := by
  intro p vs cs h
  induction cs generalizing p vs with
  | nil =>
    cases vs with
    | nil => simp [writeHole, fillAt]
    | cons v vs => obtain ⟨_, h0⟩ := h 0 (by simp); simp at h0
  | cons c cs ih =>
    cases vs with
    | nil => simp only [writeHole, fillAt_nil]
    | cons v vs =>
      cases p with
      | zero =>
        obtain ⟨idx, h0⟩ := h 0 (by simp)
        simp at h0
        subst h0
        have h' : HoleAt job 0 vs cs := by
          intro k hk
          obtain ⟨idx', hk'⟩ := h (k + 1) (by simp; omega)
          exact ⟨idx', by simpa [Nat.add_comm] using hk'⟩
        simp only [writeHole, ih 0 vs h', fillAt_cons_zero, Cell.val]
        simp
      | succ p =>
        have h' : HoleAt job p (v :: vs) cs := by
          intro k hk
          obtain ⟨idx', hk'⟩ := h k hk
          exact ⟨idx', by simpa [Nat.add_right_comm] using hk'⟩
        simp only [writeHole, ih p (v :: vs) h', fillAt_cons_succ]

theorem fillAt_length (p : Nat) (vs : List C) (cs : List (Cell C)) : (fillAt p vs cs).length = cs.length := by
  simp [fillAt]

theorem fillAt_vals (p : Nat) (vs : List C) (cs : List (Cell C)) :
    (fillAt p vs cs).map Cell.val = cs.map Cell.val := by
  apply List.ext_getElem (by simp [fillAt]); intro i _ _
  simp [fillAt]; split <;> rfl

/-! ## The hole invariant and drains -/

/-- Every in-flight job's hole is where it was carved, every hole belongs to
    a job, jobs are distinct, and `jobMaxEnd` bounds every hole. -/
structure HolesOK (s : St C) : Prop where
  inplace : ∀ j ∈ s.jobs, HoleAt j.id j.dst j.out s.cells
  owned : ∀ p v id idx, s.cells[p]? = some (.hole v id idx) →
    ∃ j ∈ s.jobs, j.id = id ∧ j.dst ≤ p ∧ p < j.dst + j.out.length
  distinct : (s.jobs.map (·.id)).Nodup
  maxEnd : ∀ j ∈ s.jobs, j.dst + j.out.length ≤ s.jobMaxEnd

theorem fillAt_getElem? (p : Nat) (vs : List C) (cs : List (Cell C)) (i : Nat) :
    (fillAt p vs cs)[i]? = (cs[i]?).map (fun c => if p ≤ i ∧ i < p + vs.length then .ready c.val else c) := by
  simp [fillAt, List.getElem?_mapIdx]

/-- **Draining the oldest job** of a state whose holes are in place readies
    exactly its hole: the state keeps its shape, its values and its error. -/
theorem drainOldest_spec (s : St C) (j : Job C) (js : List (Job C)) (hjobs : s.jobs = j :: js)
    (hok : HolesOK s) :
    s.drainOldest = { s.setCells (fillAt j.dst j.out s.cells) with jobs := js } ∧
    HolesOK { s.setCells (fillAt j.dst j.out s.cells) with jobs := js } := by
  have hin := hok.inplace j (by rw [hjobs]; simp)
  have hw := writeHole_spec j.id j.dst j.out s.cells hin
  have hlen : (fillAt j.dst j.out s.cells).length = s.cells.length := fillAt_length _ _ _
  obtain ⟨_, _, hcells, hjb, _, _, _, hme⟩ := St.setCells_spec s _ hlen
  have heq : s.drainOldest = { s.setCells (fillAt j.dst j.out s.cells) with jobs := js } := by
    unfold St.drainOldest
    rw [hjobs]; simp only [hw]; rfl
  have hc2 : ({ s.setCells (fillAt j.dst j.out s.cells) with jobs := js } : St C).cells =
      fillAt j.dst j.out s.cells := hcells
  refine ⟨heq, ⟨?_, ?_, ?_, ?_⟩⟩
  · -- the other jobs' holes are untouched
    intro j' hj' k hk
    simp only at hj'
    rw [hc2]
    obtain ⟨idx, hidx⟩ := hok.inplace j' (by rw [hjobs]; exact List.mem_cons_of_mem _ hj') k hk
    refine ⟨idx, ?_⟩
    rw [fillAt_getElem?, hidx]
    simp only [Option.map_some]
    rw [if_neg]
    intro ⟨h1, h2⟩
    -- that cell would be a hole of both jobs
    have hd : (j.id :: js.map (·.id)).Nodup := by have := hok.distinct; rw [hjobs] at this; simpa using this
    obtain ⟨idx2, h2'⟩ := hin (j'.dst + k - j.dst) (by omega)
    rw [show j.dst + (j'.dst + k - j.dst) = j'.dst + k by omega, hidx] at h2'
    simp at h2'
    have : j'.id ∈ js.map (·.id) := List.mem_map.mpr ⟨j', hj', rfl⟩
    rw [h2'.2.1] at this
    exact (List.nodup_cons.mp hd).1 this
  · -- every remaining hole belongs to a remaining job
    intro p v id idx hp
    rw [hc2, fillAt_getElem?] at hp
    cases hc : s.cells[p]? with
    | none => rw [hc] at hp; simp at hp
    | some c =>
      rw [hc] at hp
      simp only [Option.map_some, Option.some.injEq] at hp
      by_cases hr : j.dst ≤ p ∧ p < j.dst + j.out.length
      · rw [if_pos hr] at hp; cases hp
      · rw [if_neg hr] at hp
        subst hp
        obtain ⟨j', hj', hid, h1, h2⟩ := hok.owned p v id idx hc
        rw [hjobs] at hj'
        rcases List.mem_cons.mp hj' with rfl | hj'
        · exact absurd ⟨h1, h2⟩ hr
        · exact ⟨j', hj', hid, h1, h2⟩
  · have := hok.distinct; rw [hjobs] at this; simp only at this ⊢; exact (List.nodup_cons.mp this).2
  · intro j' hj'; simp only at hj' ⊢; rw [hme]
    exact hok.maxEnd j' (by rw [hjobs]; exact List.mem_cons_of_mem _ hj')

/-! ## Erased views -/

theorem erase_eq_of_vals {a b : List (Cell C)} (h : a.map Cell.val = b.map Cell.val) :
    a.map Cell.erase = b.map Cell.erase := by
  have : a.map Cell.erase = (a.map Cell.val).map Cell.ready := by simp [Cell.erase]
  rw [this, h]; simp [Cell.erase]

theorem map_append_split {a b a' b' : List (Cell C)} (hl : a'.length = a.length)
    (h : (a' ++ b').map Cell.val = (a ++ b).map Cell.val) :
    a'.map Cell.val = a.map Cell.val ∧ b'.map Cell.val = b.map Cell.val := by
  simp only [List.map_append] at h
  exact List.append_inj h (by simp [hl])

theorem nodes_erase_eq {ns ns' : List (Node C)} (hd : ns'.map (·.depth) = ns.map (·.depth))
    (hv : (nodeCells ns').map Cell.val = (nodeCells ns).map Cell.val) :
    ns'.map Node.erase = ns.map Node.erase := by
  induction ns generalizing ns' with
  | nil => cases ns' with
    | nil => rfl
    | cons _ _ => simp at hd
  | cons n ns ih => cases ns' with
    | nil => simp at hd
    | cons n' ns' =>
      simp only [List.map_cons, List.cons.injEq, nodeCells] at hd hv ⊢
      exact ⟨by simp [Node.erase, Cell.erase, hd.1, hv.1], ih hd.2 hv.2⟩

theorem pend_erase_eq {ps ps' : List (List (Cell C))} (hl : ps'.map List.length = ps.map List.length)
    (hv : ps'.flatten.map Cell.val = ps.flatten.map Cell.val) :
    ps'.map (·.map Cell.erase) = ps.map (·.map Cell.erase) := by
  induction ps generalizing ps' with
  | nil => cases ps' with
    | nil => rfl
    | cons _ _ => simp at hl
  | cons e es ih => cases ps' with
    | nil => simp at hl
    | cons e' es' =>
      simp only [List.map_cons, List.cons.injEq, List.flatten_cons] at hl hv ⊢
      obtain ⟨h1, h2⟩ := map_append_split hl.1 hv
      exact ⟨erase_eq_of_vals h1, ih hl.2 h2⟩

/-- Same shape and same values: the same layer once erased. -/
theorem Layer.erase_eq_of (l l' : Layer C) (hs : Layer.SameShape l l')
    (hv : l'.cells.map Cell.val = l.cells.map Cell.val) : l'.erase = l.erase := by
  unfold Layer.cells at hv
  have hn : (nodeCells l'.nodes).length = (nodeCells l.nodes).length := by
    simp only [nodeCells_length]
    have := congrArg List.length hs.depths; simpa using this
  have hp : (l'.pend.flatten).length = (l.pend.flatten).length := by
    simp only [List.length_flatten]; rw [hs.pend]
  rw [List.append_assoc, List.append_assoc, List.append_assoc, List.append_assoc] at hv
  obtain ⟨h1, hv⟩ := map_append_split hs.prog hv
  obtain ⟨h2, hv⟩ := map_append_split hn hv
  obtain ⟨h3, h4⟩ := map_append_split hp hv
  unfold Layer.erase
  rw [erase_eq_of_vals h1, nodes_erase_eq hs.depths h2, pend_erase_eq hs.pend h3, erase_eq_of_vals h4,
    hs.incremental, hs.legacy, hs.progressive, hs.packed, hs.collapsed, hs.level, hs.counted, hs.pendC]

theorem SameShape.cells_length {l l' : Layer C} (hs : Layer.SameShape l l') :
    l'.cells.length = l.cells.length := by
  rw [Layer.cells_length, Layer.cells_length, hs.prog, hs.pend, hs.tail]
  have := congrArg List.length hs.depths; simp at this; rw [this]

theorem layers_erase_eq {ls ls' : List (Layer C)} (hs : Layer.SameShapes ls ls')
    (hv : (ls'.map Layer.cells).flatten.map Cell.val = (ls.map Layer.cells).flatten.map Cell.val) :
    ls'.map Layer.erase = ls.map Layer.erase := by
  induction ls generalizing ls' with
  | nil => cases ls' with
    | nil => rfl
    | cons _ _ => exact hs.elim
  | cons l ls ih => cases ls' with
    | nil => exact hs.elim
    | cons l' ls' =>
      simp only [List.map_cons, List.flatten_cons] at hv ⊢
      obtain ⟨h1, h2⟩ := map_append_split (SameShape.cells_length hs.1) hv
      rw [Layer.erase_eq_of l l' hs.1 h1, ih hs.2 h2]

/-- The erased view of a state: its base and scopes with every cell ready. -/
def St.view (s : St C) : List (Cell C) × List (Layer C) := (s.base.map Cell.erase, s.stack.map Layer.erase)

/-- Rebuilding the buffer from cells of the same values keeps the view. -/
theorem view_setCells (s : St C) (cs : List (Cell C)) (hl : cs.length = s.cells.length)
    (hv : cs.map Cell.val = s.cells.map Cell.val) : (s.setCells cs).view = s.view := by
  obtain ⟨hb, hs, hc, _⟩ := St.setCells_spec s cs hl
  rw [← hc] at hv
  unfold St.cells at hv
  obtain ⟨h1, h2⟩ := map_append_split hb hv
  have hsr := SameShapes_reverse hs
  have := layers_erase_eq hsr h2
  unfold St.view
  rw [erase_eq_of_vals h1]
  congr 1
  have e1 := congrArg List.reverse this
  simpa [List.map_reverse] using e1

theorem view_with_jobs (s : St C) (js : List (Job C)) : ({ s with jobs := js } : St C).view = s.view := rfl

/-- **Draining all jobs** of a state whose holes are in place: no job and no
    hole remains, and the view, the shape, and the errors are unchanged. -/
theorem drainAll_spec (s : St C) (hok : HolesOK s) :
    s.drainAll.view = s.view ∧ s.drainAll.jobs = [] ∧ HolesOK s.drainAll ∧
    s.drainAll.err = s.err ∧ s.drainAll.hashErr = s.hashErr ∧
    (∀ (p : Nat) (v : C) (id idx : Nat), s.drainAll.cells[p]? ≠ some (Cell.hole v id idx)) ∧
    s.drainAll.cells.length = s.cells.length := by
  have key : ∀ n (t : St C), t.jobs.length = n → HolesOK t →
      (St.drainAll.go n t).view = t.view ∧ (St.drainAll.go n t).jobs = [] ∧ HolesOK (St.drainAll.go n t) ∧
      (St.drainAll.go n t).err = t.err ∧ (St.drainAll.go n t).hashErr = t.hashErr ∧
      (St.drainAll.go n t).cells.length = t.cells.length := by
    intro n
    induction n with
    | zero =>
      intro t hn ht
      exact ⟨rfl, List.eq_nil_of_length_eq_zero hn, ht, rfl, rfl, rfl⟩
    | succ n ih =>
      intro t hn ht
      obtain ⟨j, js, hjobs⟩ : ∃ j js, t.jobs = j :: js := by
        cases h : t.jobs with
        | nil => rw [h] at hn; simp at hn
        | cons j js => exact ⟨j, js, rfl⟩
      obtain ⟨hd, hok'⟩ := drainOldest_spec t j js hjobs ht
      have hlen : (fillAt j.dst j.out t.cells).length = t.cells.length := fillAt_length _ _ _
      obtain ⟨_, _, hcells, _, herr, hherr, _, _⟩ := St.setCells_spec t _ hlen
      have hjl : t.drainOldest.jobs.length = n := by rw [hd]; simp; rw [hjobs] at hn; simpa using hn
      obtain ⟨i1, i2, i3, i4, i5, i6⟩ := ih t.drainOldest hjl (by rw [hd]; exact hok')
      simp only [St.drainAll.go]
      refine ⟨?_, i2, i3, ?_, ?_, ?_⟩
      · rw [i1, hd, view_with_jobs, view_setCells t _ hlen (fillAt_vals _ _ _)]
      · rw [i4, hd]; exact herr
      · rw [i5, hd]; exact hherr
      · rw [i6, hd]
        show (t.setCells (fillAt j.dst j.out t.cells)).cells.length = t.cells.length
        rw [hcells, hlen]
  obtain ⟨k1, k2, k3, k4, k5, k6⟩ := key s.jobs.length s rfl hok
  have hview : s.drainAll.view = (St.drainAll.go s.jobs.length s).view := rfl
  have hjobs : s.drainAll.jobs = (St.drainAll.go s.jobs.length s).jobs := rfl
  have hcd : s.drainAll.cells = (St.drainAll.go s.jobs.length s).cells := rfl
  refine ⟨by rw [hview, k1], by rw [hjobs, k2], ?_, k4, k5, ?_, k6⟩
  · exact ⟨fun j hj => by rw [hjobs, k2] at hj; simp at hj,
      fun p v id idx hp => by
        rw [hcd] at hp
        obtain ⟨j, hj, _⟩ := k3.owned p v id idx hp
        rw [k2] at hj; simp at hj,
      by rw [hjobs, k2]; simp,
      fun j hj => by rw [hjobs, k2] at hj; simp at hj⟩
  · intro p v id idx hp
    rw [hcd] at hp
    obtain ⟨j, hj, _⟩ := k3.owned p v id idx hp
    rw [k2] at hj; simp at hj

end Ssz.Hasher