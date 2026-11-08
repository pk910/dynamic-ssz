import SszCert.Hasher.Nested

/-!
# Progressive roots moved below

The binary-scope lemmas assume a scope's region holds no progressive roots.
A progressive scope's region begins with them (`prog`), and its batch
collapse, pre-collapse, leaf appends, deferral and synchronous flushes work
on the rest of the region only. `St.sim` moves the roots into a separate
scope just below: the buffer, the jobs and every position the calls compute
(`activeStart`) are unchanged, so those calls commute with the move, and
their binary-scope lemmas apply to the moved state.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- The scope without its progressive roots. -/
def Layer.flat (l : Layer C) : Layer C := { l with prog := [], progressive := false, level := 0 }

/-- A passive scope holding a scope's progressive roots as plain nodes (and
    recording its progressive flag and level). -/
def progLayer (l : Layer C) : Layer C :=
  { incremental := false, legacy := true, progressive := l.progressive, packed := false, level := l.level,
    nodes := l.prog.map (fun c => ⟨0, c⟩) }

/-- The roots moved below the innermost scope. -/
def St.sim (s : St C) : St C :=
  match s.stack with
  | [] => s
  | l :: ls => { s with stack := l.flat :: progLayer l :: ls }

theorem St.sim_stack (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    s.sim.stack = l.flat :: progLayer l :: ls := by simp [St.sim, h]

theorem progLayer_cells (l : Layer C) : (progLayer l).cells = l.prog := by
  simp [progLayer, Layer.cells, nodeCells, List.map_map, Function.comp_def]

theorem Layer.flat_cells (l : Layer C) : l.prog ++ l.flat.cells = l.cells := by
  simp [Layer.flat, Layer.cells, List.append_assoc]

theorem St.sim_cells (s : St C) : s.sim.cells = s.cells := by
  unfold St.sim
  split
  · rfl
  · rename_i l ls h
    simp only [St.cells, h, List.reverse_cons, List.map_append, List.map_cons, List.map_nil, List.flatten_append,
      List.flatten_cons, List.flatten_nil, List.append_nil, progLayer_cells, List.append_assoc]
    rw [Layer.flat_cells]

theorem St.sim_activeStart (s : St C) : s.sim.activeStart = s.activeStart := by
  unfold St.sim
  split
  · rfl
  · rename_i l ls h
    simp only [St.activeStart, St.topStart, h, List.drop_one, List.tail_cons, List.reverse_cons, List.map_append,
      List.map_cons, List.map_nil, List.sum_append, List.sum_cons, List.sum_nil, progLayer_cells]
    simp [Layer.flat]; omega

/-! ## Writing the buffer back commutes with the move -/

theorem setCells_go_append (A B : List (Layer C)) (r : List (Cell C)) :
    St.setCells.go (A ++ B) r = St.setCells.go A r ++ St.setCells.go B (r.drop (A.map (fun l => l.cells.length)).sum) := by
  induction A generalizing r with
  | nil => simp [St.setCells.go]
  | cons a A ih => simp [St.setCells.go, ih, List.drop_drop, Nat.add_comm]

theorem Layer.flat_setCells (l : Layer C) (X : List (Cell C)) :
    l.flat.setCells (X.drop l.prog.length) = (l.setCells X).flat := by
  simp [Layer.setCells, Layer.flat]

theorem progLayer_setCells' (P Z : List (Cell C)) (pr : Bool) (lv : Nat) (hz : Z.length = P.length) :
    ({ incremental := false, legacy := true, progressive := pr, packed := false, level := lv,
       nodes := P.map (fun c => ⟨0, c⟩) } : Layer C).setCells Z =
    { incremental := false, legacy := true, progressive := pr, packed := false, level := lv,
      nodes := Z.map (fun c => ⟨0, c⟩) } := by
  have hz' : (Z.take (P.map (fun c => (⟨0, c⟩ : Node C))).length).length = (P.map (fun c => (⟨0, c⟩ : Node C))).length := by
    simp; omega
  have hzip : ((P.map (fun c => (⟨0, c⟩ : Node C))).zip (Z.take (P.map (fun c => (⟨0, c⟩ : Node C))).length)).map
      (fun (n, c) => ({ n with cell := c } : Node C)) = Z.map (fun c => ⟨0, c⟩) := by
    rw [List.take_of_length_le (by simp; omega)]
    clear hz'
    induction P generalizing Z with
    | nil => cases Z with
      | nil => rfl
      | cons _ _ => simp at hz
    | cons p P ih => cases Z with
      | nil => simp at hz
      | cons z Z => simp only [List.length_cons, Nat.add_right_cancel_iff] at hz; simp [ih Z hz]
  unfold Layer.setCells
  simp only [List.length_nil, List.take_zero, List.drop_zero] at hzip ⊢
  rw [hzip]
  have hle : Z.length ≤ P.length := by omega
  simp [Layer.setCells.splitPend, hle]

theorem progLayer_setCells (l : Layer C) (Z : List (Cell C)) (hz : Z.length = l.prog.length) :
    (progLayer l).setCells Z = progLayer { l with prog := Z } :=
  progLayer_setCells' l.prog Z l.progressive l.level hz

@[simp] theorem St.sim_base (t : St C) : t.sim.base = t.base := by unfold St.sim; split <;> rfl
@[simp] theorem St.sim_jobs (t : St C) : t.sim.jobs = t.jobs := by unfold St.sim; split <;> rfl
@[simp] theorem St.sim_nextJob (t : St C) : t.sim.nextJob = t.nextJob := by unfold St.sim; split <;> rfl
@[simp] theorem St.sim_jobMaxEnd (t : St C) : t.sim.jobMaxEnd = t.jobMaxEnd := by unfold St.sim; split <;> rfl
@[simp] theorem St.sim_err (t : St C) : t.sim.err = t.err := by unfold St.sim; split <;> rfl
@[simp] theorem St.sim_hashErr (t : St C) : t.sim.hashErr = t.hashErr := by unfold St.sim; split <;> rfl

@[simp] theorem St.setCells_base (t : St C) (cs : List (Cell C)) : (t.setCells cs).base = cs.take t.base.length := rfl
@[simp] theorem St.setCells_jobs (t : St C) (cs : List (Cell C)) : (t.setCells cs).jobs = t.jobs := rfl
@[simp] theorem St.setCells_nextJob (t : St C) (cs : List (Cell C)) : (t.setCells cs).nextJob = t.nextJob := rfl
@[simp] theorem St.setCells_jobMaxEnd (t : St C) (cs : List (Cell C)) : (t.setCells cs).jobMaxEnd = t.jobMaxEnd := rfl
@[simp] theorem St.setCells_err (t : St C) (cs : List (Cell C)) : (t.setCells cs).err = t.err := rfl
@[simp] theorem St.setCells_hashErr (t : St C) (cs : List (Cell C)) : (t.setCells cs).hashErr = t.hashErr := rfl

theorem St.ext_fields {a b : St C} (hb : a.base = b.base) (hs : a.stack = b.stack) (hj : a.jobs = b.jobs)
    (hn : a.nextJob = b.nextJob) (hm : a.jobMaxEnd = b.jobMaxEnd) (he : a.err = b.err) (hh : a.hashErr = b.hashErr) :
    a = b := by
  cases a; cases b; simp_all

theorem setCells_go_one (l : Layer C) (r : List (Cell C)) :
    St.setCells.go [l] r = [l.setCells (r.take l.cells.length)] := by simp [St.setCells.go]

theorem setCells_go_two (l1 l2 : Layer C) (r : List (Cell C)) :
    St.setCells.go [l1, l2] r = [l1.setCells (r.take l1.cells.length),
      l2.setCells ((r.drop l1.cells.length).take l2.cells.length)] := by simp [St.setCells.go]

theorem setCells_sim (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (cs : List (Cell C)) (hl : cs.length = s.cells.length) : s.sim.setCells cs = (s.setCells cs).sim := by
  have hcl : s.cells.length = s.base.length + (ls.reverse.map (fun l => l.cells.length)).sum + l.cells.length := by
    have := St.cells_length s
    rw [h, List.reverse_cons, List.map_append, List.sum_append] at this
    simp only [List.map_cons, List.map_nil, List.sum_cons, List.sum_nil, Nat.add_zero] at this
    omega
  have hpl : l.prog.length ≤ l.cells.length := by
    have := congrArg List.length (Layer.flat_cells l); rw [List.length_append] at this; omega
  generalize hR : (ls.reverse.map (fun l => l.cells.length)).sum = nR at hcl
  generalize hr : (cs.drop s.base.length).drop nR = r
  have hrl : l.cells.length ≤ r.length := by rw [← hr, List.length_drop, List.length_drop]; omega
  have hfl : l.flat.cells.length = l.cells.length - l.prog.length := by
    have := congrArg List.length (Layer.flat_cells l); rw [List.length_append] at this; omega
  -- the two stacks
  have e2 : (s.setCells cs).stack =
      l.setCells (r.take l.cells.length) :: (St.setCells.go ls.reverse (cs.drop s.base.length)).reverse := by
    show (St.setCells.go s.stack.reverse (cs.drop s.base.length)).reverse = _
    rw [h, List.reverse_cons, setCells_go_append, setCells_go_one, hR, hr, List.reverse_append]; rfl
  have e1 : (s.sim.setCells cs).stack =
      l.flat.setCells ((r.drop l.prog.length).take l.flat.cells.length) ::
      (progLayer l).setCells (r.take l.prog.length) ::
      (St.setCells.go ls.reverse (cs.drop s.base.length)).reverse := by
    show (St.setCells.go s.sim.stack.reverse (cs.drop s.sim.base.length)).reverse = _
    have hb : s.sim.base = s.base := by simp [St.sim, h]
    rw [St.sim_stack s l ls h, hb, List.reverse_cons, List.reverse_cons, List.append_assoc, List.singleton_append,
      setCells_go_append, setCells_go_two, progLayer_cells, hR, hr, List.reverse_append]; rfl
  have hX : (r.take l.cells.length).drop l.prog.length = (r.drop l.prog.length).take l.flat.cells.length := by
    rw [hfl, List.drop_take]
  have hP : (l.setCells (r.take l.cells.length)).prog = r.take l.prog.length := by
    simp [Layer.setCells, List.take_take, Nat.min_eq_left hpl]
  have hsim : (s.setCells cs).sim.stack = (l.setCells (r.take l.cells.length)).flat ::
      progLayer (l.setCells (r.take l.cells.length)) :: (St.setCells.go ls.reverse (cs.drop s.base.length)).reverse :=
    St.sim_stack _ _ _ e2
  apply St.ext_fields
  · simp [St.sim_stack s l ls h]
  · rw [e1, hsim, ← hX, Layer.flat_setCells, progLayer_setCells _ _ (by simp; omega)]
    have : progLayer { l with prog := r.take l.prog.length } = progLayer (l.setCells (r.take l.cells.length)) := by
      simp only [progLayer]; rw [hP]; rfl
    rw [this]
  all_goals simp

/-! ## Calls commute with the move -/

theorem St.sim_stack_eq {s t : St C} (h : t.stack = s.stack) : t.sim.stack = s.sim.stack := by
  unfold St.sim; rw [h]; split <;> simp_all

theorem St.sim_eq {s t : St C} (hs : t.stack = s.stack) (hb : t.base = s.base) (hj : t.jobs = s.jobs)
    (hn : t.nextJob = s.nextJob) (hm : t.jobMaxEnd = s.jobMaxEnd) (he : t.err = s.err) (hh : t.hashErr = s.hashErr) :
    t.sim = s.sim := St.ext_fields (by simp [hb]) (St.sim_stack_eq hs) (by simp [hj]) (by simp [hn]) (by simp [hm])
      (by simp [he]) (by simp [hh])

theorem St.fail_sim (s : St C) (m : String) : (s.fail m).sim = s.sim.fail m := by
  unfold St.fail
  cases he : s.err with
  | none =>
    simp only [St.sim_err, he]
    apply St.ext_fields <;> simp [St.sim_stack_eq (show ({ s with err := some m } : St C).stack = s.stack from rfl)]
  | some _ => simp [he]

theorem St.setHashErr_sim (s : St C) (m : String) : (s.setHashErr m).sim = s.sim.setHashErr m := by
  unfold St.setHashErr
  cases he : s.hashErr with
  | none =>
    simp only [St.sim_hashErr, he]
    apply St.ext_fields <;> simp
    exact St.sim_stack_eq rfl
  | some _ => simp [he]

theorem St.fail_stack (s : St C) (m : String) : (s.fail m).stack = s.stack := by unfold St.fail; split <;> rfl

theorem vals_sim (s : St C) (cs : List (Cell C)) (w : String) :
    (s.vals cs w).1.sim = (s.sim.vals cs w).1 ∧ (s.vals cs w).2 = (s.sim.vals cs w).2 := by
  unfold St.vals; split <;> simp [St.fail_sim]

theorem vals_stack (s : St C) (cs : List (Cell C)) (w : String) : (s.vals cs w).1.stack = s.stack := by
  unfold St.vals; split <;> simp [St.fail_stack]

theorem pairNodes_sim (cx : Ctx C) (s : St C) (ns : List (Node C)) :
    (s.pairNodes cx ns).1.sim = (s.sim.pairNodes cx ns).1 ∧ (s.pairNodes cx ns).2 = (s.sim.pairNodes cx ns).2 := by
  have := vals_sim s (nodeCells ns) "pair"
  simp only [St.pairNodes]; exact ⟨this.1, by rw [this.2]⟩

theorem pairNodes_stack (cx : Ctx C) (s : St C) (ns : List (Node C)) : (s.pairNodes cx ns).1.stack = s.stack :=
  vals_stack _ _ _

theorem failUnless_sim (s : St C) (b : Bool) (m : String) : (s.failUnless b m).sim = s.sim.failUnless b m := by
  unfold St.failUnless; split <;> simp [St.fail_sim]

theorem failUnless_stack (s : St C) (b : Bool) (m : String) : (s.failUnless b m).stack = s.stack := by
  unfold St.failUnless; split <;> simp [St.fail_stack]

theorem batchStep_sim (cx : Ctx C) (d : Nat) (s : St C) (ns : List (Node C)) :
    (batchStep cx d s ns).1.sim = (batchStep cx d s.sim ns).1 ∧ (batchStep cx d s ns).2 = (batchStep cx d s.sim ns).2 := by
  simp only [batchStep]
  refine ⟨?_, by rw [(pairNodes_sim cx s _).2]⟩
  rw [(pairNodes_sim cx _ _).1, failUnless_sim]

theorem batchStep_stack (cx : Ctx C) (d : Nat) (s : St C) (ns : List (Node C)) :
    (batchStep cx d s ns).1.stack = s.stack := by
  simp only [batchStep]; rw [pairNodes_stack, failUnless_stack]

theorem batchLoop_sim (cx : Ctx C) : ∀ fuel d (s : St C) (ns : List (Node C)),
    (batchLoop cx fuel d s ns).1.sim = (batchLoop cx fuel d s.sim ns).1 ∧
    (batchLoop cx fuel d s ns).2 = (batchLoop cx fuel d s.sim ns).2 ∧
    (batchLoop cx fuel d s ns).1.stack = s.stack := by
  intro fuel
  induction fuel with
  | zero => intro d s ns; exact ⟨rfl, rfl, rfl⟩
  | succ fuel ih =>
    intro d s ns
    simp only [batchLoop]
    split
    · exact ⟨rfl, rfl, rfl⟩
    · obtain ⟨h1, h2⟩ := batchStep_sim cx d s ns
      obtain ⟨i1, i2, i3⟩ := ih (d + 1) (batchStep cx d s ns).1 (batchStep cx d s ns).2
      rw [h1, h2] at i1 i2
      exact ⟨i1, i2, by rw [i3, batchStep_stack]⟩

theorem uncollapsedAsLeaves_sim (s : St C) (l : Layer C) :
    (s.uncollapsedAsLeaves l).1.sim = (s.sim.uncollapsedAsLeaves l.flat).1 ∧
    (s.sim.uncollapsedAsLeaves l.flat).2 = (s.uncollapsedAsLeaves l).2.flat ∧
    (s.uncollapsedAsLeaves l).1.stack = s.stack ∧ progLayer (s.uncollapsedAsLeaves l).2 = progLayer l := by
  unfold St.uncollapsedAsLeaves
  have hc : l.flat.collapsed = l.collapsed := rfl
  have hn : l.flat.nodes = l.nodes := rfl
  rw [hc, hn]
  split
  · exact ⟨rfl, rfl, rfl, rfl⟩
  · simp only
    split
    · exact ⟨St.fail_sim _ _, rfl, St.fail_stack _ _, by simp [progLayer]⟩
    · exact ⟨rfl, rfl, rfl, by simp [progLayer]⟩

theorem St.setTop_sim (s : St C) (l l' : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hp : progLayer l' = progLayer l) : (s.setTop l').sim = s.sim.setTop l'.flat := by
  apply St.ext_fields <;> simp [St.setTop, St.sim, h, hp]

theorem writeHole_length (job : Nat) : ∀ (p : Nat) (vs : List C) (cs : List (Cell C)),
    (writeHole job p vs cs).1.length = cs.length
  | _, [], cs => by simp [writeHole]
  | _, _ :: _, [] => by simp [writeHole]
  | 0, v :: vs, c :: cs => by simp [writeHole, writeHole_length job 0 vs cs]
  | p + 1, v :: vs, c :: cs => by simp [writeHole, writeHole_length job p (v :: vs) cs]

theorem St.sim_nil (s : St C) (h : s.stack = []) : s.sim = s := by simp [St.sim, h]

theorem setCells_sim_any (s : St C) (cs : List (Cell C)) (hl : cs.length = s.cells.length) :
    s.sim.setCells cs = (s.setCells cs).sim := by
  cases h : s.stack with
  | nil =>
    have h2 : (s.setCells cs).stack = [] := by simp [St.setCells, h, St.setCells.go]
    rw [St.sim_nil s h, St.sim_nil _ h2]
  | cons l ls => exact setCells_sim s l ls h cs hl

theorem drainOldest_sim (s : St C) : s.sim.drainOldest = s.drainOldest.sim := by
  unfold St.drainOldest
  rw [St.sim_jobs]
  cases hj : s.jobs with
  | nil => rfl
  | cons j js =>
    simp only [St.sim_cells]
    cases hw : writeHole j.id j.dst j.out s.cells with
    | mk cs ok =>
      simp only
      have hl : cs.length = s.cells.length := by
        have := writeHole_length j.id j.dst j.out s.cells; rw [hw] at this; exact this
      have hset : s.sim.setCells cs = (s.setCells cs).sim := setCells_sim_any s cs hl
      have hj' : ({ s.sim.setCells cs with jobs := js } : St C) = ({ s.setCells cs with jobs := js } : St C).sim := by
        rw [hset]; apply St.ext_fields <;> simp
        exact (St.sim_stack_eq (s := s.setCells cs) (t := { s.setCells cs with jobs := js }) rfl).symm
      split
      · exact hj'
      · rw [hj', St.fail_sim]

theorem drainAll_sim (s : St C) : s.sim.drainAll = s.drainAll.sim := by
  have key : ∀ n (t : St C), St.drainAll.go n t.sim = (St.drainAll.go n t).sim := by
    intro n
    induction n with
    | zero => intro t; rfl
    | succ n ih => intro t; simp only [St.drainAll.go]; rw [drainOldest_sim, ih]
  unfold St.drainAll
  rw [St.sim_jobs, key]
  apply St.ext_fields <;> simp
  exact St.sim_stack_eq rfl

theorem drainFor_sim (s : St C) (p : Nat) : s.sim.drainFor p = (s.drainFor p).sim := by
  unfold St.drainFor; simp only [St.sim_jobs, St.sim_jobMaxEnd]; split
  · exact drainAll_sim s
  · rfl

theorem ringDrain_sim (P : Params) (s : St C) : s.sim.ringDrain P = (s.ringDrain P).sim := by
  unfold St.ringDrain; simp only [St.sim_jobs]; split
  · exact drainOldest_sim s
  · rfl

theorem enqueue_sim (cx : Ctx C) (s : St C) (input : List (Cell C)) (out dst : Nat) :
    (s.sim.enqueue cx input out dst).1 = (s.enqueue cx input out dst).1.sim ∧
    (s.sim.enqueue cx input out dst).2 = (s.enqueue cx input out dst).2 := by
  unfold St.enqueue
  rw [show (if s.sim.jobs.length ≥ cx.P.ring then s.sim.drainOldest else s.sim) = s.sim.ringDrain cx.P from rfl,
    show (if s.jobs.length ≥ cx.P.ring then s.drainOldest else s) = s.ringDrain cx.P from rfl, ringDrain_sim]
  obtain ⟨h1, h2⟩ := vals_sim (s.ringDrain cx.P) input "job input"
  simp only [← h1, ← h2]
  refine ⟨?_, by simp⟩
  apply St.ext_fields <;> simp
  exact St.sim_stack_eq rfl

/-- The innermost scope of a moved state, as a default. -/
theorem topLayer_sim (t : St C) (l : Layer C) : t.sim.topLayer l.flat = (t.topLayer l).flat := by
  unfold St.sim St.topLayer; split <;> simp_all

theorem push_cons (t : St C) (l : Layer C) (ls : List (Layer C)) (h : t.stack = l :: ls) (c : Cell C) :
    t.push c = { t with stack := (if l.pend = [] then { l with nodes := l.nodes ++ [⟨0, c⟩] }
      else { l with tail := l.tail ++ [c] }) :: ls } := by
  unfold St.push; rw [h]; simp only; split <;> rfl

theorem push_sim (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) (c : Cell C) :
    (s.push c).sim = s.sim.push c := by
  rw [push_cons s l ls h c, push_cons s.sim l.flat (progLayer l :: ls) (St.sim_stack s l ls h) c]
  have hp : l.flat.pend = l.pend := rfl
  rw [hp]
  apply St.ext_fields
  · simp
  · rw [St.sim_stack _ _ _ rfl]; by_cases hq : l.pend = [] <;> simp [hq, Layer.flat, progLayer]
  all_goals simp

theorem collapseBinaryCore_sim (cx : Ctx C) (s : St C) :
    s.sim.collapseBinaryCore cx = (s.collapseBinaryCore cx).sim := by
  cases h : s.stack with
  | nil => simp [St.collapseBinaryCore, St.sim_nil s h, h]
  | cons l ls =>
    obtain ⟨u1, u2, u3, u4⟩ := uncollapsedAsLeaves_sim s l
    obtain ⟨b1, b2, b3⟩ := batchLoop_sim cx 64 0 (s.uncollapsedAsLeaves l).1 (s.uncollapsedAsLeaves l).2.nodes
    unfold St.collapseBinaryCore
    rw [St.sim_stack s l ls h, h]
    simp only
    rw [← u1, u2]
    have hn : ((s.uncollapsedAsLeaves l).2.flat).nodes = (s.uncollapsedAsLeaves l).2.nodes := rfl
    rw [hn, ← b1, ← b2]
    rw [St.setTop_sim _ l _ ls (by rw [b3, u3, h]) (by rw [← u4]; rfl)]
    rfl

theorem collapseBinary_sim (cx : Ctx C) (s : St C) : s.sim.collapseBinary cx = (s.collapseBinary cx).sim := by
  cases h : s.stack with
  | nil => simp [St.collapseBinary, St.sim_nil s h, h]
  | cons l ls =>
    unfold St.collapseBinary
    rw [St.sim_stack s l ls h, h]
    simp only
    have hp : l.flat.pend = l.pend := rfl
    have hd : d0Chunks l.flat = d0Chunks l := rfl
    rw [hp, hd]
    split
    · exact St.fail_sim s _ |>.symm
    split
    · rfl
    rw [St.sim_activeStart, drainFor_sim, collapseBinaryCore_sim]

theorem normTop_sim (s : St C) : s.sim.normTop = s.normTop.sim := by
  cases h : s.stack with
  | nil => simp [St.normTop, St.sim_nil s h, h]
  | cons l ls =>
    obtain ⟨u1, u2, u3, u4⟩ := uncollapsedAsLeaves_sim s l
    unfold St.normTop
    rw [St.sim_stack s l ls h, h]
    simp only
    rw [← u1, u2, St.setTop_sim _ l _ ls (by rw [u3, h]) u4]

theorem capJob_sim (cx : Ctx C) (s : St C) (l : Layer C) (ncap : Nat) :
    (s.sim.capJob cx l.flat ncap).1 = (s.capJob cx l ncap).1.sim ∧
    (s.sim.capJob cx l.flat ncap).2 = (s.capJob cx l ncap).2 := by
  unfold St.capJob
  rw [St.sim_activeStart]
  exact enqueue_sim cx s _ _ _

theorem capCompact_flat (P : Params) (l : Layer C) (ncap : Nat) (hs : List (Cell C)) :
    capCompact P l.flat ncap hs = (capCompact P l ncap hs).flat := rfl

theorem St.setTop_sim_any (t : St C) (l' : Layer C) (hp : ∀ u us, t.stack = u :: us → progLayer l' = progLayer u) :
    (t.setTop l').sim = t.sim.setTop l'.flat := by
  cases h : t.stack with
  | nil =>
    have h2 : (t.setTop l').stack = [] := by simp [St.setTop, h]
    rw [St.sim_nil _ h2, St.sim_nil t h]; simp [St.setTop, h]
  | cons u us => exact St.setTop_sim t u l' us h (hp u us h)

theorem topLayer_prog (t : St C) (d : Layer C) : ∀ u us, t.stack = u :: us → (t.topLayer d).prog = u.prog := by
  intro u us h; simp [St.topLayer, h]

theorem precollapseLoop_sim (cx : Ctx C) : ∀ fuel (s : St C) (ncap : Nat),
    precollapseLoop cx fuel s.sim ncap = (precollapseLoop cx fuel s ncap).sim := by
  intro fuel
  induction fuel with
  | zero => intro s ncap; rfl
  | succ fuel ih =>
    intro s ncap
    cases h : s.stack with
    | nil => simp [precollapseLoop, St.sim_nil s h, h]
    | cons l ls =>
      simp only [precollapseLoop, St.sim_stack s l ls h, h]
      have hn : l.flat.nodes = l.nodes := rfl
      rw [hn]
      split
      · rfl
      · obtain ⟨c1, c2⟩ := capJob_sim cx s l ncap
        rw [c1, c2, topLayer_sim, capCompact_flat,
          ← St.setTop_sim_any (s.capJob cx l ncap).1
            (capCompact cx.P ((s.capJob cx l ncap).1.topLayer l) ncap (s.capJob cx l ncap).2)
            (fun u us hu => by simp [capCompact, St.topLayer, hu, progLayer]), ih]

theorem precollapse_sim (cx : Ctx C) (s : St C) :
    (s.sim.precollapse cx).1 = (s.precollapse cx).1.sim ∧ (s.sim.precollapse cx).2 = (s.precollapse cx).2 := by
  unfold St.precollapse
  rw [normTop_sim]
  cases h : s.normTop.stack with
  | nil => rw [St.sim_nil _ h, h]; exact ⟨rfl, rfl⟩
  | cons l ls =>
    rw [St.sim_stack _ l ls h]
    simp only
    have hn : l.flat.nodes = l.nodes := rfl
    have hc : capCount cx.P l.flat = capCount cx.P l := rfl
    rw [hn, hc]
    split
    · exact ⟨rfl, rfl⟩
    · exact ⟨by rw [← failUnless_sim, precollapseLoop_sim], rfl⟩

theorem flushFold_sim (cx : Ctx C) : ∀ (es : List (List (Cell C))) (s : St C),
    (s.sim.flushFold cx es).1 = (s.flushFold cx es).1.sim ∧ (s.sim.flushFold cx es).2 = (s.flushFold cx es).2 ∧
    (s.flushFold cx es).1.stack = s.stack
  | [], s => ⟨rfl, rfl, rfl⟩
  | e :: es, s => by
    obtain ⟨v1, v2⟩ := vals_sim s e "flush"
    simp only [St.flushFold]
    rw [← v1, ← v2]
    obtain ⟨i1, i2, i3⟩ := flushFold_sim cx es (s.vals e "flush").1
    exact ⟨i1, by rw [i2], by rw [i3, vals_stack]⟩

theorem flushSync_sim (cx : Ctx C) (s : St C) : s.sim.flushSync cx = (s.flushSync cx).sim := by
  cases h : s.stack with
  | nil => simp [St.flushSync, St.sim_nil s h, h]
  | cons l ls =>
    unfold St.flushSync
    rw [St.sim_stack s l ls h, h]
    simp only
    have hp : l.flat.pend = l.pend := rfl
    rw [hp]
    split
    · rfl
    · obtain ⟨f1, f2, f3⟩ := flushFold_sim cx l.pend s
      rw [f1, f2, St.setTop_sim (s.flushFold cx l.pend).1 l
        { l with nodes := l.nodes ++ (s.flushFold cx l.pend).2.map (fun v => ⟨0, .ready v⟩) ++ l.tail.map (fun c => ⟨0, c⟩), pend := [], pendC := 0, tail := [] }
        ls (by rw [f3, h]) (by simp [progLayer])]
      rfl

end Ssz.Hasher