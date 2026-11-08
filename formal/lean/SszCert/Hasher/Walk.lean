import SszCert.Hasher.Close

/-!
# Walks over a binary scope

The calls a generated or reflection hasher makes for a binary SSZ value
(`StartTree(TreeTypeBinary)`, `AppendChunk`/leaves, `Collapse` hints with
synchronous or asynchronous hashing in any pattern, then `Merkleize` or
`MerkleizeWithMixin`) compute the specified bounded Merkle root, and the
model's protocol checks (no hole read or moved, no scratch write over a live
chunk, no impossible state) never fire.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- The parameter facts the proofs use. -/
structure ParamsOK (P : Params) : Prop where
  batch_pos : 0 < P.batch
  batch_even : P.batch % 2 = 0
  cap : P.cap = 2 ^ P.capDepth
  capDepth_pos : 0 < P.capDepth
  batch_lt_cap : P.batch < P.cap

/-- Go's production thresholds (`hasher/params.go`). -/
theorem paramsOK_default : ParamsOK ({} : Params) := ⟨by decide, by decide, by decide, by decide, by decide⟩

/-- The `dynssz_smallhash` thresholds (`hasher/params_small.go`). -/
theorem paramsOK_small (ac pl r : Nat) :
    ParamsOK { batch := 4, asyncMin := 8, cap := 16, capDepth := 4, active := ac, precollapseLevel := pl, ring := r } :=
  ⟨show 0 < 4 by decide, show 4 % 2 = 0 by decide, show 16 = 2 ^ 4 by decide, show 0 < 4 by decide,
    show 4 < 16 by decide⟩

/-- The calls inside a binary scope. -/
inductive BinBody : Op C → Prop
  | leaf (v : C) : BinBody (.leaf v)
  | collapse (a : Bool) : BinBody (.collapse a)

/-- The leaves a call sequence appends. -/
def leafVals : List (Op C) → List C
  | [] => []
  | .leaf v :: ops => v :: leafVals ops
  | _ :: ops => leafVals ops

theorem step_collapse_bin (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hb : BinOK cx l L) (a : Bool) :
    s.step cx (.collapse a) =
      if l.incremental then (if a then (s.precollapse cx).1 else s.collapseBinary cx) else s := by
  cases hi : l.incremental <;> simp [St.step, h, hi, hb.pend, hb.noprog]

theorem push_base (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) (c : Cell C) :
    (s.push c).base = s.base := by
  simp only [St.push, h]; split <;> rfl

theorem push_stack_tail (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hp : l.pend = []) (c : Cell C) : ∃ l', (s.push c).stack = l' :: ls := by
  rw [St.push_eq_setTop s l ls h hp]; exact ⟨_, St.setTop_stack s l _ ls h⟩

/-- **Any body of leaves and `Collapse` hints** keeps a binary scope's
    invariant, its leaves being the ones appended, and leaves the rest of
    the state's view alone. -/
theorem body_bin (cx : Ctx C) (hP : ParamsOK cx.P) : ∀ (body : List (Op C)), (∀ op ∈ body, BinBody op) →
    ∀ (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C), s.stack = l :: ls → BinOK cx l L →
      TopOK s → (L ++ leafVals body).length < 2 ^ 40 →
      ∃ l' ls', (body.foldl (St.step cx) s).stack = l' :: ls' ∧ BinOK cx l' (L ++ leafVals body) ∧
        l'.incremental = l.incremental ∧ TopOK (body.foldl (St.step cx) s) ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
        (body.foldl (St.step cx) s).base.map Cell.erase = s.base.map Cell.erase ∧
        (body.foldl (St.step cx) s).hashErr = s.hashErr ∧ HolesSub s.below (body.foldl (St.step cx) s).below := by
  intro body
  induction body with
  | nil => intro _ s l ls L h hb ht _; exact ⟨l, ls, h, by simpa [leafVals] using hb, rfl, ht, rfl, rfl, rfl, HolesSub.refl _⟩
  | cons op rest ih =>
    intro hbody s l ls L h hb ht hL
    have hrest : ∀ op ∈ rest, BinBody op := fun o ho => hbody o (List.mem_cons_of_mem _ ho)
    rw [List.foldl_cons]
    cases hbody op (by simp) with
    | leaf v =>
      obtain ⟨hs1, hb1, ht1, hbl1, hh1⟩ := push_bin cx s l ls L v h hb ht
      have hL' : (L ++ [v] ++ leafVals rest).length < 2 ^ 40 := by simpa [leafVals] using hL
      obtain ⟨l', ls', hs', hb', hi', ht', hle', hba', hh', hsub'⟩ := ih hrest _ _ ls _ hs1 hb1 ht1 hL'
      refine ⟨l', ls', hs', by simpa [leafVals] using hb', hi', ht', hle', ?_, ?_, by rw [← hbl1]; exact hsub'⟩
      · show (List.foldl (St.step cx) (s.push (.ready v)) rest).base.map Cell.erase = _
        rw [hba', push_base s l ls h]
      · show (List.foldl (St.step cx) (s.push (.ready v)) rest).hashErr = _
        rw [hh', hh1]
    | collapse a =>
      have hL0 : L.length < 2 ^ 40 := by simp at hL; omega
      have hL' : (L ++ leafVals rest).length < 2 ^ 40 := by simpa [leafVals] using hL
      rw [step_collapse_bin cx s l ls L h hb a]
      cases hinc : l.incremental
      · simp only [Bool.false_eq_true, if_false]
        obtain ⟨l', ls', hs', hb', hi', ht', hle', hba', hh', hsub'⟩ := ih hrest s l ls L h hb ht hL'
        exact ⟨l', ls', hs', by simpa [leafVals] using hb', hi'.trans hinc, ht', hle', hba', hh', hsub'⟩
      simp only [if_true]
      have hstep : ∃ l1 ls1, (if a then (s.precollapse cx).1 else s.collapseBinary cx).stack = l1 :: ls1 ∧
          BinOK cx l1 L ∧ l1.incremental = true ∧ TopOK (if a then (s.precollapse cx).1 else s.collapseBinary cx) ∧
          ls1.map Layer.erase = ls.map Layer.erase ∧
          (if a then (s.precollapse cx).1 else s.collapseBinary cx).base.map Cell.erase = s.base.map Cell.erase ∧
          (if a then (s.precollapse cx).1 else s.collapseBinary cx).hashErr = s.hashErr ∧
          HolesSub s.below (if a then (s.precollapse cx).1 else s.collapseBinary cx).below := by
        cases a
        · simp only [Bool.false_eq_true, if_false]
          exact collapseBinary_bin cx hP.batch_pos hP.batch_even s l ls L h hb hinc ht hL0
        · simp only [if_true]
          obtain ⟨l1, ls1, h1, h2, h2', h3, h4, h5, h6, _, h8⟩ :=
            precollapse_bin cx hP.cap hP.capDepth_pos s l ls L h hb hinc ht hL0
          exact ⟨l1, ls1, h1, h2, h2', h3, h4, h5, h6, h8⟩
      obtain ⟨l1, ls1, hs1, hb1, hi1, ht1, hle1, hba1, hh1, hsub1⟩ := hstep
      obtain ⟨l', ls', hs', hb', hi', ht', hle', hba', hh', hsub'⟩ := ih hrest _ l1 ls1 L hs1 hb1 ht1 hL'
      exact ⟨l', ls', hs', by simpa [leafVals] using hb', hi'.trans hi1, ht', by rw [hle', hle1], by rw [hba', hba1],
        by rw [hh', hh1], HolesSub.trans hsub1 hsub'⟩

/-! ## The top-level tree -/

/-- The scope `StartTree(TreeTypeBinary)` opens. -/
def binLayer (p : Bool) : Layer C := { incremental := true, legacy := false, progressive := false, packed := p }

theorem start_binary (cx : Ctx C) (p : Bool) :
    ({} : St C).step cx (.startTree .binary p) = { stack := [binLayer p] } := by
  simp [St.step, binLayer]
  first | rfl | decide

theorem binOK_start (cx : Ctx C) (p : Bool) : BinOK cx (binLayer p) ([] : List C) :=
  ⟨fun _ => rfl, rfl, rfl, rfl, rfl, rfl, rfl, fun _ n hn => by simp [binLayer] at hn,
    fun n hn => by simp [binLayer] at hn, fun n hn => by simp [binLayer] at hn, fun hc => by cases hc⟩

theorem topOK_start (p : Bool) : TopOK ({ stack := [binLayer p] } : St C) := by
  refine ⟨⟨fun j hj => by simp at hj, fun q v id idx hq => ?_, by simp, fun j hj => by simp at hj⟩, rfl,
    fun j hj => by simp at hj, fun j hj => by simp at hj⟩
  simp [St.cells, binLayer, Layer.cells, nodeCells] at hq

/-- With an empty buffer there are no jobs. -/
theorem jobs_nil_of_cells (s : St C) (ht : TopOK s) (hc : s.cells = []) : s.jobs = [] := by
  cases hj : s.jobs with
  | nil => rfl
  | cons j js =>
    have hm : j ∈ s.jobs := by rw [hj]; simp
    have hpos : 0 < j.out.length := List.length_pos_iff.mpr (ht.jobsne j hm)
    obtain ⟨idx, hidx⟩ := ht.holes.inplace j hm 0 hpos
    rw [hc] at hidx; simp at hidx

/-- The root of a state whose stack is empty and whose base holds one ready
    chunk. -/
theorem root_single (s : St C) (ht : TopOK s) (hst : s.stack = []) (hb : s.base = []) (hh : s.hashErr = none)
    (v : C) : (s.push (.ready v)).root = some v := by
  have hj : s.jobs = [] := jobs_nil_of_cells s ht (by simp [St.cells, hst, hb])
  have hp : s.push (.ready v) = { s with base := [.ready v] } := by simp [St.push, hst, hb]
  rw [hp]
  unfold St.root St.drainAll
  simp [St.drainAll.go, hj, ht.err, hh, hst, readyVals]

theorem erase_nil_layers {ls : List (Layer C)} (h : ls.map Layer.erase = ([] : List (Layer C)).map Layer.erase) :
    ls = [] := by simpa using h

/-- The state after the scope's body. -/
theorem top_body (cx : Ctx C) (hP : ParamsOK cx.P) (p : Bool) (body : List (Op C))
    (hbody : ∀ op ∈ body, BinBody op) (hL : (leafVals body).length < 2 ^ 40) :
    ∃ l, (body.foldl (St.step cx) (({} : St C).step cx (.startTree .binary p))).stack = [l] ∧
      BinOK cx l (leafVals body) ∧ TopOK (body.foldl (St.step cx) (({} : St C).step cx (.startTree .binary p))) ∧
      (body.foldl (St.step cx) (({} : St C).step cx (.startTree .binary p))).base = [] ∧
      (body.foldl (St.step cx) (({} : St C).step cx (.startTree .binary p))).hashErr = none := by
  rw [start_binary]
  obtain ⟨l, ls, hs, hb, _, ht, hle, hba, hh, _⟩ :=
    body_bin cx hP body hbody { stack := [binLayer p] } (binLayer p) [] [] rfl (binOK_start cx p) (topOK_start p)
      (by simpa using hL)
  rw [erase_nil_layers hle] at hs
  exact ⟨l, hs, by simpa using hb, ht, by simpa using hba, hh⟩

/-- **A binary tree's root** (`HashTreeRoot` of a container, vector or
    unmixed list): whatever the leaves and wherever the synchronous and
    asynchronous `Collapse` hints fall, the hasher's root is the Merkle root
    of the leaves padded to the next power of two. -/
theorem run_binary (cx : Ctx C) (hP : ParamsOK cx.P) (p : Bool) (body : List (Op C))
    (hbody : ∀ op ∈ body, BinBody op) (a : Bool) (hL : (leafVals body).length < 2 ^ 40) :
    (run cx (.startTree .binary p :: body ++ [.merkleize a])).root =
      some (mroot cx.H cx.z0 (depthOf (leafVals body).length) (leafVals body)) := by
  unfold run
  rw [List.cons_append, List.foldl_cons, List.foldl_append, List.foldl_cons, List.foldl_nil]
  obtain ⟨l, hs, hb, ht, hba, hh⟩ := top_body cx hP p body hbody hL
  generalize (body.foldl (St.step cx) (({} : St C).step cx (.startTree .binary p))) = s at hs hb ht hba hh ⊢
  have hm : s.step cx (.merkleize a) = s.closeBinary cx 0 a := by
    simp only [St.step, hs, hb.noprog, Bool.false_eq_true, if_false]
    split <;> simp_all
  rw [hm]
  obtain ⟨s1, hle1, ht1, hba1, hh1, _, he⟩ := closeBinary_bin cx s l [] _ 0 a hs hb ht (Or.inl rfl)
  rw [he]
  exact root_single s1 ht1 (erase_nil_layers hle1) (by rw [hba] at hba1; simpa using hba1) (by rw [hh1, hh]) _

/-- **A mixed-in binary tree's root** (`HashTreeRoot` of a list or bitlist):
    the Merkle root of the leaves at the depth of the limit, mixed with the
    length. -/
theorem run_binary_mixin (cx : Ctx C) (hP : ParamsOK cx.P) (p : Bool) (body : List (Op C))
    (hbody : ∀ op ∈ body, BinBody op) (num limit : Nat) (a : Bool) (hL : (leafVals body).length < 2 ^ 40)
    (hlim : limit = 0 ∨ (leafVals body).length ≤ limit) :
    (run cx (.startTree .binary p :: body ++ [.merkleizeMixin num limit a])).root =
      some (cx.H (mroot cx.H cx.z0 (closeDepth (leafVals body).length limit) (leafVals body)) (cx.word num)) := by
  unfold run
  rw [List.cons_append, List.foldl_cons, List.foldl_append, List.foldl_cons, List.foldl_nil]
  obtain ⟨l, hs, hb, ht, hba, hh⟩ := top_body cx hP p body hbody hL
  generalize (body.foldl (St.step cx) (({} : St C).step cx (.startTree .binary p))) = s at hs hb ht hba hh ⊢
  have hm : s.step cx (.merkleizeMixin num limit a) =
      ((s.flushSync cx).drainFor (s.flushSync cx).topStart).mixinClose cx num limit := by
    simp only [St.step, hs, hb.noprog, Bool.false_eq_true, if_false]
  rw [hm]
  obtain ⟨s1, hle1, ht1, hba1, hh1, _, he⟩ := mixinClose_bin cx s l [] _ num limit hs hb ht hlim
  rw [he]
  exact root_single s1 ht1 (erase_nil_layers hle1) (by rw [hba] at hba1; simpa using hba1) (by rw [hh1, hh]) _

end Ssz.Hasher
