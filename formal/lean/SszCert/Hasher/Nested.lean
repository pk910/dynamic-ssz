import SszCert.Hasher.Scope

/-!
# Nested scopes

A tree of hasher calls as the engines make them for a value: a scope
(`StartTree`, binary or non-incremental), a body of leaves, `Collapse` hints
and child trees, and a close (`Merkleize`, `MerkleizeWithMixin`). Children
close into their parent: reduced on the spot, or parked in the parent's
deferred run.
-/

namespace Ssz.Hasher

variable {C : Type}

/-! ## Jumps: the same values, holes only removed -/

structure Jump (s t : St C) : Prop where
  view : t.view = s.view
  holes : HolesSub s.cells t.cells
  herr : t.hashErr = s.hashErr

theorem Jump.refl (s : St C) : Jump s s := ⟨rfl, HolesSub.refl _, rfl⟩

theorem Jump.trans {s t u : St C} (h1 : Jump s t) (h2 : Jump t u) : Jump s u :=
  ⟨h2.view.trans h1.view, HolesSub.trans h1.holes h2.holes, h2.herr.trans h1.herr⟩

theorem StepOK.jump (cx : Ctx C) {s sF t : St C} {p : Layer C} {ps : List (Layer C)} {L : List C}
    (h1 : StepOK cx s sF p ps L) (hj : Jump sF t) (ht : TopOK t) : StepOK cx s t p ps L := by
  obtain ⟨p1, ps1, h1s, hs1, hi1, hn1, _, hle1, hba1, hh1, hbl1, hsub1⟩ := h1
  obtain ⟨p3, ps3, h3, hle, hle3⟩ := LayerLE.of_view hj.view h1s hj.holes
  refine ⟨p3, ps3, h3, hle.scopeOK cx hs1, hle.flag_incremental.trans hi1, ?_, ht, hle3.trans hle1, ?_,
    hj.herr.trans hh1, ?_, HolesSub.trans hsub1 (holesSub_below hj.view h1s h3 hj.holes)⟩
  · intro hf
    have hp1 := hn1 (by rw [← hle.flag_incremental]; exact hf)
    have := hle.pend_map; rw [hp1] at this; simpa using this
  · have := congrArg Prod.fst hj.view; simp only [St.view] at this; rw [this]; exact hba1
  · rw [below_length_of_view hj.view h1s h3]; exact hbl1

/-- The state with the innermost scope removed (not drained). -/
def St.pop (s : St C) : St C := { s with stack := s.stack.tail }

theorem St.pop_cells (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    s.pop.cells = s.below := by
  have := St.cells_pop s l ls h; simpa [St.pop, h] using this

/-- A closed child's root lands in the parent. -/
theorem push_after_jump (cx : Ctx C) (s t : St C) (l p : Layer C) (ps : List (Layer C)) (Lp : List C) (r : C)
    (h : s.stack = l :: p :: ps) (hj : Jump s.pop t) (ht : TopOK t) (hp : ScopeOK cx p Lp)
    (hpn : p.incremental = false → p.pend = []) :
    ∃ q qs, t.stack = q :: qs ∧ StepOK cx t (t.push (.ready r)) q qs (Lp ++ [r]) := by
  have hps : s.pop.stack = p :: ps := by simp [St.pop, h]
  obtain ⟨q, qs, hq, hle, _⟩ := LayerLE.of_view hj.view hps hj.holes
  refine ⟨q, qs, hq, push_scope cx t q qs Lp r hq (hle.scopeOK cx hp) ?_ ht⟩
  intro hf
  have hp1 := hpn (by rw [← hle.flag_incremental]; exact hf)
  have := hle.pend_map; rw [hp1] at this; simpa using this

/-! ## Closing a scope with a deferred run -/

/-- The close of `Merkleize` and `MerkleizeWithMixin` on a scope that may
    still hold a deferred run: its flush, then the closes of `Close.lean`. -/
theorem closeBinary_scope (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (limit : Nat) (a : Bool) (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (ht : TopOK s)
    (hlim : limit = 0 ∨ L.length ≤ limit) :
    ∃ t : St C, Jump s.pop t ∧ TopOK t ∧
      s.closeBinary cx limit a = t.push (.ready (mroot cx.H cx.z0 (closeDepth L.length limit) L)) := by
  obtain ⟨l', hF, hb', _, _, htF, hbF, hbaF, hhF⟩ := flushSync_scope cx s l ls L h hs ht
  have hff : (s.flushSync cx).flushSync cx = s.flushSync cx := flushSync_nopend cx _ l' ls hF hb'.pend
  have e : s.closeBinary cx limit a = (s.flushSync cx).closeBinary cx limit a := by
    unfold St.closeBinary; rw [hff]
  obtain ⟨t, hle, htt, hba, hh, hsub, he⟩ := closeBinary_bin cx (s.flushSync cx) l' ls L limit a hF hb' htF hlim
  refine ⟨t, ⟨?_, ?_, ?_⟩, htt, by rw [e, he]⟩
  · show (t.base.map Cell.erase, t.stack.map Layer.erase) = (s.pop.base.map Cell.erase, s.pop.stack.map Layer.erase)
    simp only [St.pop, h, List.tail_cons]; rw [hba, hbaF, hle]
  · rw [St.pop_cells s l ls h, ← hbF]; exact hsub
  · rw [hh, hhF]; rfl

theorem mixin_scope (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (num limit : Nat) (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (ht : TopOK s)
    (hlim : limit = 0 ∨ L.length ≤ limit) :
    ∃ t : St C, Jump s.pop t ∧ TopOK t ∧
      ((s.flushSync cx).drainFor (s.flushSync cx).topStart).mixinClose cx num limit =
        t.push (.ready (cx.H (mroot cx.H cx.z0 (closeDepth L.length limit) L) (cx.word num))) := by
  obtain ⟨l', hF, hb', _, _, htF, hbF, hbaF, hhF⟩ := flushSync_scope cx s l ls L h hs ht
  have hff : (s.flushSync cx).flushSync cx = s.flushSync cx := flushSync_nopend cx _ l' ls hF hb'.pend
  obtain ⟨t, hle, htt, hba, hh, hsub, he⟩ := mixinClose_bin cx (s.flushSync cx) l' ls L num limit hF hb' htF hlim
  rw [hff] at he
  refine ⟨t, ⟨?_, ?_, ?_⟩, htt, he⟩
  · show (t.base.map Cell.erase, t.stack.map Layer.erase) = (s.pop.base.map Cell.erase, s.pop.stack.map Layer.erase)
    simp only [St.pop, h, List.tail_cons]; rw [hba, hbaF, hle]
  · rw [St.pop_cells s l ls h, ← hbF]; exact hsub
  · rw [hh, hhF]; rfl

/-! ## Progressive closes of a non-incremental scope -/

/-- The drained, hole-free innermost scope's leaves. -/
theorem none_vals (cx : Ctx C) {l : Layer C} {L : List C} (hb : BinOK cx l L) (hinc : l.incremental = false) :
    l.cells.map Cell.val = L := by
  rw [layer_cells_bin l hb.prog hb.pend hb.tail]; unfold nodeCells; rw [List.map_map]
  exact cov_leaves cx.H cx.z0 l.nodes L hb.cov (hb.leaves (hb.ninc hinc))

/-- **`MerkleizeProgressiveWithMixin` / `WithActiveFields`** on a
    non-incremental scope. -/
theorem progWord_none (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) (w : C) (a : Bool)
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (hinc : l.incremental = false) (hn : l.pend = []) (ht : TopOK s) :
    ∃ t : St C, Jump s.pop t ∧ TopOK t ∧
      s.closeProgWord cx w a = t.push (.ready (cx.H (progRoot cx.H cx.z0 0 L) w)) := by
  have hb := bin_of_scopeOK cx hs hn
  obtain ⟨l2, ls2, hs2, hb2, ht2, hr2, hle2, hba2, hh2, hsub2⟩ := drainTop_bin cx s l ls L h hb ht
  have hi2 : l2.incremental = false := by
    have := (stack_of_view (drainFor_spec s ht s.topStart).1 h)
    obtain ⟨l', ls', h', he', _⟩ := this
    rw [hs2] at h'; obtain ⟨rfl, rfl⟩ := List.cons.inj h'
    have := congrArg Layer.incremental he'; exact this.trans hinc
  have hvals := none_vals cx hb2 hi2
  have hnp2 : l2.progressive = false := hb2.noprog
  have hroot : progImpl cx.H cx.z0 ((l2.cells.map Cell.val).length + 1) 0 (l2.cells.map Cell.val) =
      progRoot cx.H cx.z0 0 L := by rw [hvals]; exact progImpl_eq cx.H cx.z0 _ 0 L (by omega)
  refine ⟨{ s.drainFor s.topStart with stack := ls2 }, ⟨?_, ?_, ?_⟩, topOK_pop _ l2 ls2 hs2 ht2 hr2, ?_⟩
  · show (_, _) = (_, _)
    simp only [St.pop, h, List.tail_cons]; rw [hba2, hle2]
  · rw [St.pop_cells s l ls h, St.cells_pop _ l2 ls2 hs2]; exact hsub2
  · exact hh2
  · unfold St.closeProgWord
    rw [h]
    simp only [hb.noprog, hinc, hn, Bool.not_false, Bool.false_eq_true, and_false, if_false, ne_eq,
      not_true_eq_false]
    rw [hs2]
    simp only [hnp2, Bool.false_eq_true, false_and, if_false]
    rw [show (s.drainFor s.topStart).vals l2.cells "merkleize" = (s.drainFor s.topStart, l2.cells.map Cell.val) by
      simp [St.vals, List.all_eq_true.mpr hr2]]
    simp only [St.vals, List.all_cons, List.all_nil, Cell.isReady, Bool.and_true, if_true, List.map_cons,
      List.map_nil, List.head?_cons, Option.getD_some, Cell.val]
    rw [popWith_eq _ l2 ls2 hs2, hroot]

/-- **`MerkleizeProgressive`** on a non-incremental scope. -/
theorem prog_none (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) (a : Bool)
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (hinc : l.incremental = false) (hn : l.pend = []) (ht : TopOK s) :
    ∃ t : St C, Jump s.pop t ∧ TopOK t ∧
      s.step cx (.merkleizeProg a) = t.push (.ready (progRoot cx.H cx.z0 0 L)) := by
  have hb := bin_of_scopeOK cx hs hn
  obtain ⟨l2, ls2, hs2, hb2, ht2, hr2, hle2, hba2, hh2, hsub2⟩ := drainTop_bin cx s l ls L h hb ht
  have hi2 : l2.incremental = false := by
    have := (stack_of_view (drainFor_spec s ht s.topStart).1 h)
    obtain ⟨l', ls', h', he', _⟩ := this
    rw [hs2] at h'; obtain ⟨rfl, rfl⟩ := List.cons.inj h'
    have := congrArg Layer.incremental he'; exact this.trans hinc
  have hvals := none_vals cx hb2 hi2
  have hnp2 : l2.progressive = false := hb2.noprog
  have hroot : progImpl cx.H cx.z0 ((l2.cells.map Cell.val).length + 1) 0 (l2.cells.map Cell.val) =
      progRoot cx.H cx.z0 0 L := by rw [hvals]; exact progImpl_eq cx.H cx.z0 _ 0 L (by omega)
  refine ⟨{ s.drainFor s.topStart with stack := ls2 }, ⟨?_, ?_, ?_⟩, topOK_pop _ l2 ls2 hs2 ht2 hr2, ?_⟩
  · show (_, _) = (_, _)
    simp only [St.pop, h, List.tail_cons]; rw [hba2, hle2]
  · rw [St.pop_cells s l ls h, St.cells_pop _ l2 ls2 hs2]; exact hsub2
  · exact hh2
  · simp only [St.step, h, hb.noprog, hinc, hn, Bool.not_false, Bool.false_eq_true, and_false, if_false, ne_eq,
      not_true_eq_false]
    rw [hs2]
    simp only [hnp2, Bool.false_eq_true, if_false]
    rw [show (s.drainFor s.topStart).vals l2.cells "merkleize" = (s.drainFor s.topStart, l2.cells.map Cell.val) by
      simp [St.vals, List.all_eq_true.mpr hr2]]
    simp only
    rw [popWith_eq _ l2 ls2 hs2, hroot]

/-! ## The grammar -/

/-- The scopes the engines open for a value. -/
inductive Kind where
  | binary
  | none
  /-- `Index`: a legacy scope (reduced in place, never deferred). -/
  | index
  deriving DecidableEq

def Kind.tt : Kind → TreeType
  | .binary => .binary
  | .none => .none
  | .index => .none

/-- The call that opens the scope. -/
def Kind.openOp : Kind → Bool → Op C
  | .index, _ => .index
  | k, p => .startTree k.tt p

def Kind.layer : Kind → Bool → Layer C
  | .binary, p => { incremental := true, legacy := false, progressive := false, packed := p }
  | .none, p => { incremental := false, legacy := false, progressive := false, packed := p }
  | .index, _ => { incremental := false, legacy := true, progressive := false, packed := false }

/-- How a scope is closed. -/
inductive CloseOp (C : Type) where
  | merkleize (async : Bool)
  | mixin (num limit : Nat) (async : Bool)
  | prog (async : Bool)
  | progMixin (num : Nat) (async : Bool)
  | progActive (af : C) (async : Bool)

def CloseOp.op : CloseOp C → Op C
  | .merkleize a => .merkleize a
  | .mixin num limit a => .merkleizeMixin num limit a
  | .prog a => .merkleizeProg a
  | .progMixin num a => .merkleizeProgMixin num a
  | .progActive af a => .merkleizeProgActive af a

/-- The specified root of a closed scope over its leaves. -/
def CloseOp.rootOf (cx : Ctx C) : CloseOp C → List C → C
  | .merkleize _, L => mroot cx.H cx.z0 (depthOf L.length) L
  | .mixin num limit _, L => cx.H (mroot cx.H cx.z0 (closeDepth L.length limit) L) (cx.word num)
  | .prog _, L => progRoot cx.H cx.z0 0 L
  | .progMixin num _, L => cx.H (progRoot cx.H cx.z0 0 L) (cx.word num)
  | .progActive af _, L => cx.H (progRoot cx.H cx.z0 0 L) af

/-- The close fits the scope: the leaves fit the limit, and a progressive
    close is made on a non-incremental scope (an incremental binary scope
    closed progressively is a shape mismatch Go reports as an error). -/
def CloseOp.ok : CloseOp C → Bool → Nat → Prop
  | .merkleize _, _, _ => True
  | .mixin _ limit _, _, n => limit = 0 ∨ n ≤ limit
  | .prog _, inc, _ => inc = false
  | .progMixin _ _, inc, _ => inc = false
  | .progActive _ _, inc, _ => inc = false

def Kind.inc : Kind → Bool
  | .binary => true
  | .none => false
  | .index => false

theorem Kind.layer_inc (k : Kind) (p : Bool) : (k.layer p : Layer C).incremental = k.inc := by cases k <;> rfl

theorem depthOf_pos {n : Nat} (h : 2 ≤ n) : 0 < depthOf n := by
  unfold depthOf; rw [if_neg (by omega)]; omega

/-- **A child scope's close**, deferred into its parent or reduced. -/
theorem closeChild (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l p : Layer C) (ps : List (Layer C))
    (Lc Lp : List C) (cl : CloseOp C) (h : s.stack = l :: p :: ps) (hs : ScopeOK cx l Lc)
    (hn : l.incremental = false → l.pend = []) (ht : TopOK s) (hok : cl.ok l.incremental Lc.length)
    (hp : ScopeOK cx p Lp) (hpn : p.incremental = false → p.pend = [])
    (hLp : (Lp ++ [cl.rootOf cx Lc]).length < 2 ^ 40) :
    ∃ t q qs, t.stack = q :: qs ∧ Jump s.pop t ∧ TopOK t ∧
      StepOK cx t (s.step cx cl.op) q qs (Lp ++ [cl.rootOf cx Lc]) := by
  have hs0 := hs
  obtain ⟨_, _, hbs, _⟩ := hs0
  have hnp : l.progressive = false := hbs.noprog
  cases cl with
  | mixin num limit a =>
    have hstep : s.step cx (CloseOp.mixin num limit a).op =
        ((s.flushSync cx).drainFor (s.flushSync cx).topStart).mixinClose cx num limit := by
      simp only [CloseOp.op, St.step, h, hnp, Bool.false_eq_true, if_false]
    obtain ⟨t, hj, htt, he⟩ := mixin_scope cx s l (p :: ps) Lc num limit h hs ht hok
    obtain ⟨q, qs, hq, hst⟩ := push_after_jump cx s t l p ps Lp _ h hj htt hp hpn
    refine ⟨t, q, qs, hq, hj, htt, ?_⟩
    rw [hstep, he]; exact hst
  | prog a =>
    obtain ⟨t, hj, htt, he⟩ := prog_none cx s l (p :: ps) Lc a h hs hok (hn hok) ht
    obtain ⟨q, qs, hq, hst⟩ := push_after_jump cx s t l p ps Lp _ h hj htt hp hpn
    exact ⟨t, q, qs, hq, hj, htt, by show StepOK cx t (s.step cx (.merkleizeProg a)) q qs _; rw [he]; exact hst⟩
  | progMixin num a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_none cx s l (p :: ps) Lc (cx.word num) a h hs hok (hn hok) ht
    obtain ⟨q, qs, hq, hst⟩ := push_after_jump cx s t l p ps Lp _ h hj htt hp hpn
    exact ⟨t, q, qs, hq, hj, htt, by
      show StepOK cx t (s.closeProgWord cx (cx.word num) a) q qs _; rw [he]; exact hst⟩
  | progActive af a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_none cx s l (p :: ps) Lc af a h hs hok (hn hok) ht
    obtain ⟨q, qs, hq, hst⟩ := push_after_jump cx s t l p ps Lp _ h hj htt hp hpn
    exact ⟨t, q, qs, hq, hj, htt, by show StepOK cx t (s.closeProgWord cx af a) q qs _; rw [he]; exact hst⟩
  | merkleize a =>
    -- the reduced close
    have hred : ∃ t q qs, t.stack = q :: qs ∧ Jump s.pop t ∧ TopOK t ∧
        StepOK cx t (s.closeBinary cx 0 a) q qs (Lp ++ [(CloseOp.merkleize a).rootOf cx Lc]) := by
      obtain ⟨t, hj, htt, he⟩ := closeBinary_scope cx s l (p :: ps) Lc 0 a h hs ht (Or.inl rfl)
      obtain ⟨q, qs, hq, hst⟩ := push_after_jump cx s t l p ps Lp _ h hj htt hp hpn
      refine ⟨t, q, qs, hq, hj, htt, ?_⟩
      rw [he]; simpa [CloseOp.rootOf, closeDepth] using hst
    simp only [CloseOp.op, St.step, h, hnp, Bool.false_eq_true, if_false]
    split
    · rename_i _ _ _ p' rest c hdef hls hw
      obtain ⟨hpp, hrest⟩ := List.cons.inj hls
      subst hpp; subst hrest
      by_cases hpi : p.incremental = false
      · simp only [hpi, Bool.not_false, if_true]; exact hred
      have hpi' : p.incremental = true := by simpa using hpi
      simp only [hpi', Bool.not_true, Bool.false_eq_true, if_false]
      -- the child: plain ready leaves
      have hnl : l.legacy = false := by
        cases hlg : l.legacy
        · rfl
        · simp [hlg] at hdef
      have hlp : l.pend = [] := by
        cases hli : l.incremental
        · exact hn hli
        · simp [hli, hnl] at hdef; exact hdef.2
      have hlc : l.collapsed = false := by
        cases hli : l.incremental
        · exact hbs.ninc hli
        · simp [hli, hnl] at hdef; exact hdef.1
      have hb := bin_of_scopeOK cx hs hlp
      have hd0 : ∀ n ∈ l.nodes, n.depth = 0 := hb.leaves hlc
      have hcells : l.cells = nodeCells l.nodes := layer_cells_bin l hb.prog hb.pend hb.tail
      have hrdy : ∀ x ∈ l.cells, x.isReady = true := by
        intro x hx
        rw [hcells] at hx
        obtain ⟨n, hn', rfl⟩ := List.mem_map.mp hx
        cases hr : n.cell.isReady
        · have := hb.holes n hn' hr; rw [hd0 n hn'] at this; cases this
        · rfl
      have hvals : l.cells.map Cell.val = Lc := by
        rw [hcells]; unfold nodeCells; rw [List.map_map]
        exact cov_leaves cx.H cx.z0 l.nodes Lc hb.cov hd0
      have hn' : l.cells.length = Lc.length := by rw [← hvals, List.length_map]
      obtain ⟨hn2, hnb, hcw⟩ := deferrableWidth_spec cx.P _ c hw
      rw [hn'] at hn2 hnb hcw
      have hk0 : 0 < depthOf Lc.length := depthOf_pos hn2
      have hkb : 2 ^ depthOf Lc.length ≤ cx.P.batch := by
        obtain ⟨bb, hbb⟩ := hP.batch_pow
        rw [hbb] at hnb ⊢
        have := depthOf_mono hnb; rw [depthOf_two_pow] at this
        exact Nat.pow_le_pow_right (by decide) this
      have hle : Lc.length ≤ c := by rw [hcw]; exact le_two_pow_depthOf _
      -- the element and its root
      have helem : ∀ x ∈ l.cells ++ List.replicate (c - l.cells.length) (Cell.ready cx.z0), x.isReady = true := by
        intro x hx
        rcases List.mem_append.mp hx with hx | hx
        · exact hrdy x hx
        · rw [List.eq_of_mem_replicate hx]; rfl
      have hell : (l.cells ++ List.replicate (c - l.cells.length) (Cell.ready cx.z0)).length = c := by
        simp [hn']; omega
      have hroot : reduceElem cx.H cx.z0 ((l.cells ++ List.replicate (c - l.cells.length) (Cell.ready cx.z0)).map
          Cell.val) = (CloseOp.merkleize a).rootOf cx Lc := by
        rw [List.map_append, hvals, List.map_replicate]
        rw [reduceElem_eq cx.H cx.z0 (depthOf Lc.length) _ (by simp [hn']; omega)]
        show _ = mroot cx.H cx.z0 (depthOf Lc.length) Lc
        rw [show (Cell.ready cx.z0).val = cx.z0 from rfl, hn']
        exact mroot_pad cx.H cx.z0 _ _ _ (by omega)
      -- the parent without the child (nothing to drain: the child held no hole)
      have hT : TopOK ({ s with stack := p :: ps } : St C) := topOK_pop s l (p :: ps) h ht hrdy
      have hjT : Jump s.pop ({ s with stack := p :: ps } : St C) := by
        have : s.pop = ({ s with stack := p :: ps } : St C) := by simp [St.pop, h]
        rw [this]; exact Jump.refl _
      refine ⟨{ s with stack := p :: ps }, p, ps, rfl, hjT, hT, ?_⟩
      generalize hE : l.cells ++ List.replicate (c - l.cells.length) (Cell.ready cx.z0) = elem at helem hell hroot
      have hfin : ∀ (T2 : St C) (p2 : Layer C), T2.stack = p2 :: ps → ScopeOK cx p2 Lp → p2.incremental = true →
          TopOK T2 → (p2.pend = [] ∨ (p2.pendC = c ∧ p2.tail = [])) →
          StepOK cx T2 (if (p2.pend.length + 1) * c ≥ cx.P.cap then
              (T2.setTop { p2 with pendC := c, pend := p2.pend ++ [elem] }).flush cx true a
            else T2.setTop { p2 with pendC := c, pend := p2.pend ++ [elem] }) p2 ps
            (Lp ++ [(CloseOp.merkleize a).rootOf cx Lc]) := by
        intro T2 p2 h2 hs2 hi2 ht2 hc2
        have hd := deferInto_scope cx T2 p2 ps Lp elem c (depthOf Lc.length) _ h2 hs2 hi2 ht2 hc2 helem hell hcw
          hk0 hkb hroot
        split
        · apply StepOK.trans cx hd
          intro l3 ls3 h3 hs3 hi3 _ ht3 _
          exact flush_scope cx hP _ l3 ls3 _ h3 hs3 (hi3.trans hi2) ht3 hLp true a
        · exact hd
      by_cases hcond : p.pend ≠ [] ∧ (p.pendC ≠ c ∨ p.tail ≠ [])
      · rw [if_pos hcond]
        obtain ⟨p1, hF, hb1, hi1, _, htF, hbF, hbaF, hhF⟩ :=
          flushSync_scope cx ({ s with stack := p :: ps } : St C) p ps Lp rfl hp hT
        rw [hF]
        simp only
        apply StepOK.trans cx (s2 := St.flushSync cx { s with stack := p :: ps })
          ⟨p1, ps, hF, scopeOK_of_bin cx hb1, hi1, fun _ => hb1.pend, htF, rfl, by rw [hbaF], hhF,
            by rw [hbF], by rw [hbF]; exact HolesSub.refl _⟩
        intro l2 ls2 h2 hs2 hi2 _ ht2 _
        rw [hF] at h2
        obtain ⟨rfl, rfl⟩ := List.cons.inj h2
        exact hfin _ _ hF hs2 (hi2.trans hpi') ht2 (Or.inl hb1.pend)
      · rw [if_neg hcond]
        simp only
        have hc2 : p.pend = [] ∨ (p.pendC = c ∧ p.tail = []) := by
          by_cases hpe : p.pend = []
          · exact Or.inl hpe
          · right; simp only [ne_eq, hpe, not_false_eq_true, true_and, not_or, Classical.not_not] at hcond
            exact hcond
        exact hfin _ _ rfl hp hpi' hT hc2
    · exact hred

/-! ## Trees of calls -/

mutual
/-- A scope: opened, filled, closed. -/
inductive Tree (C : Type) where
  | mk (kind : Kind) (packed : Bool) (body : Body C) (close : CloseOp C)
/-- A scope's body: leaves, `Collapse` hints (either mode) and child scopes. -/
inductive Body (C : Type) where
  | nil
  | leaf (v : C) (rest : Body C)
  | collapse (async : Bool) (rest : Body C)
  | sub (t : Tree C) (rest : Body C)
end

mutual
def Tree.ops : Tree C → List (Op C)
  | .mk k p b c => k.openOp p :: b.ops ++ [c.op]
def Body.ops : Body C → List (Op C)
  | .nil => []
  | .leaf v r => .leaf v :: r.ops
  | .collapse a r => .collapse a :: r.ops
  | .sub t r => t.ops ++ r.ops
end

mutual
/-- **The specified root** of a scope. -/
def Tree.root (cx : Ctx C) : Tree C → C
  | .mk _ _ b c => c.rootOf cx (b.leaves cx)
/-- The leaves of a body: its chunks and its children's roots. -/
def Body.leaves (cx : Ctx C) : Body C → List C
  | .nil => []
  | .leaf v r => v :: r.leaves cx
  | .collapse _ r => r.leaves cx
  | .sub t r => t.root cx :: r.leaves cx
end

mutual
/-- Every scope's leaves fit its limit and the model's 2^40 chunk bound. -/
def Tree.WF (cx : Ctx C) : Tree C → Prop
  | .mk k _ b c => b.WF cx ∧ (b.leaves cx).length < 2 ^ 40 ∧ c.ok k.inc (b.leaves cx).length
def Body.WF (cx : Ctx C) : Body C → Prop
  | .nil => True
  | .leaf _ r => r.WF cx
  | .collapse _ r => r.WF cx
  | .sub t r => t.WF cx ∧ r.WF cx
end

theorem topOK_open (s : St C) (nl : Layer C) (hc : nl.cells = []) (ht : TopOK s) :
    TopOK ({ s with stack := nl :: s.stack } : St C) := by
  have hcells : ({ s with stack := nl :: s.stack } : St C).cells = s.cells := by
    simp [St.cells, hc]
  exact ⟨⟨fun j hj => by rw [hcells]; exact ht.holes.inplace j hj,
    fun q v id idx hq => by rw [hcells] at hq; exact ht.holes.owned q v id idx hq,
    ht.holes.distinct, ht.holes.maxEnd⟩, ht.err, ht.jobsne, ht.ids⟩

/-- The layer `StartTree` pushes, as the model writes it. -/
def Kind.rawLayer (k : Kind) (pk : Bool) : Layer C := { incremental := decide (k.tt ≠ TreeType.none), legacy := false, progressive := k.tt == TreeType.progressive, packed := pk }

theorem Kind.layer_cells (k : Kind) (p : Bool) : (k.layer p : Layer C).cells = [] := by
  cases k <;> rfl

theorem Kind.layer_ok (cx : Ctx C) (k : Kind) (p : Bool) : ScopeOK cx (k.layer p : Layer C) [] := by
  apply scopeOK_of_bin
  cases k
  · exact ⟨(fun h => by cases h), rfl, (fun h => by cases h), rfl, rfl, rfl, rfl, (fun _ n hn => by simp [Kind.layer] at hn),
      (fun n hn => by simp [Kind.layer] at hn), (fun n hn => by simp [Kind.layer] at hn), (fun hc => by cases hc)⟩
  · exact ⟨(fun _ => rfl), rfl, (fun h => by cases h), rfl, rfl, rfl, rfl, (fun _ n hn => by simp [Kind.layer] at hn),
      (fun n hn => by simp [Kind.layer] at hn), (fun n hn => by simp [Kind.layer] at hn), (fun hc => by cases hc)⟩
  · exact ⟨(fun _ => rfl), rfl, (fun _ => rfl), rfl, rfl, rfl, rfl, (fun _ n hn => by simp [Kind.layer] at hn),
      (fun n hn => by simp [Kind.layer] at hn), (fun n hn => by simp [Kind.layer] at hn), (fun hc => by cases hc)⟩

/-- `StartTree` (or `Index`) on a scope: the parent flushes its run when the
    child is incremental, then the child's empty scope opens. -/
theorem open_child (cx : Ctx C) (s : St C) (p : Layer C) (ps : List (Layer C)) (Lp : List C) (k : Kind)
    (pk : Bool) (h : s.stack = p :: ps) (hs : ScopeOK cx p Lp) (hpn : p.incremental = false → p.pend = [])
    (ht : TopOK s) :
    ∃ sF, StepOK cx s sF p ps Lp ∧ s.step cx (k.openOp pk) = { sF with stack := k.layer pk :: sF.stack } := by
  by_cases hk : k = .index
  · subst hk; exact ⟨s, StepOK.refl cx h hs hpn ht, rfl⟩
  have hop : (k.openOp pk : Op C) = .startTree k.tt pk := by cases k <;> simp_all [Kind.openOp]
  rw [hop]
  have hl : ∀ t : St C, ({ t with stack := Kind.rawLayer k pk :: t.stack } : St C) =
      { t with stack := k.layer pk :: t.stack } := by
    intro t; cases k <;> first | rfl | exact absurd rfl hk
  by_cases hc : k.tt ≠ TreeType.none ∧ p.pend ≠ []
  · obtain ⟨p1, hF, hb1, hi1, _, htF, hbF, hbaF, hhF⟩ := flushSync_scope cx s p ps Lp h hs ht
    refine ⟨s.flushSync cx, ⟨p1, ps, hF, scopeOK_of_bin cx hb1, hi1, fun _ => hb1.pend, htF, rfl,
      by rw [hbaF], hhF, by rw [hbF], by rw [hbF]; exact HolesSub.refl _⟩, ?_⟩
    simp only [St.step, h]; rw [if_pos hc]; exact hl _
  · refine ⟨s, StepOK.refl cx h hs hpn ht, ?_⟩
    simp only [St.step, h]; rw [if_neg hc]
    have := hl s; rw [h] at this ⊢; exact this

theorem St.below_open (s : St C) (nl : Layer C) : ({ s with stack := nl :: s.stack } : St C).below = s.cells := by
  simp [St.below, St.cells]

theorem step_leaf (cx : Ctx C) (s : St C) (v : C) : s.step cx (.leaf v) = s.push (.ready v) := rfl

mutual
/-- **A child scope, whatever its calls**, adds exactly its specified root
    to its parent's leaves. -/
theorem Tree.step_ok (cx : Ctx C) (hP : ParamsOK cx.P) : ∀ (t : Tree C) (s : St C) (p : Layer C)
    (ps : List (Layer C)) (Lp : List C), s.stack = p :: ps → ScopeOK cx p Lp →
    (p.incremental = false → p.pend = []) → TopOK s → t.WF cx → (Lp ++ [t.root cx]).length < 2 ^ 40 →
    StepOK cx s (t.ops.foldl (St.step cx) s) p ps (Lp ++ [t.root cx])
  | .mk k pk b cl, s, p, ps, Lp, h, hs, hpn, ht, hwf, hlen => by
    obtain ⟨hwb, hwl, hok⟩ := hwf
    simp only [Tree.ops, Tree.root] at hlen ⊢
    rw [List.foldl_append, List.foldl_cons, List.foldl_cons, List.foldl_nil]
    obtain ⟨sF, hF, hstep⟩ := open_child cx s p ps Lp k pk h hs hpn ht
    rw [hstep]
    have hF0 := hF
    obtain ⟨p1, ps1, hF1, hs1, _, hn1, htF, _, _, _, _, _⟩ := hF0
    have ht1 := topOK_open sF (k.layer pk) (Kind.layer_cells k pk) htF
    have hb := Body.step_ok cx hP b { sF with stack := k.layer pk :: sF.stack } (k.layer pk) sF.stack []
      rfl (Kind.layer_ok cx k pk) (fun _ => by cases k <;> rfl) ht1 hwb (by simpa using hwl)
    simp only [List.nil_append] at hb
    obtain ⟨l2, ls2, h2, hs2, hi2, hn2, ht2, hle2, hba2, hh2, _, hsub2⟩ := hb
    rw [Kind.layer_inc] at hi2
    rw [← hi2] at hok
    rw [hF1] at hle2
    obtain ⟨p2, ps2, rfl⟩ : ∃ p2 ps2, ls2 = p2 :: ps2 := by
      cases ls2 with
      | nil => simp at hle2
      | cons p2 ps2 => exact ⟨p2, ps2, rfl⟩
    -- the parent kept its values and lost holes only
    have hj1 : Jump sF (b.ops.foldl (St.step cx) { sF with stack := k.layer pk :: sF.stack }).pop := by
      refine ⟨?_, ?_, ?_⟩
      · show (_, _) = (_, _)
        simp only [St.pop, h2, List.tail_cons]
        rw [hba2, hle2, hF1]
      · rw [St.pop_cells _ l2 _ h2, ← St.below_open sF (k.layer pk)]; exact hsub2
      · exact hh2
    obtain ⟨q2, qs2, hq2, hle, _⟩ := LayerLE.of_view hj1.view hF1 hj1.holes
    have hq2' : q2 = p2 := by simp [St.pop, h2] at hq2; exact hq2.1.symm
    subst hq2'
    have hsp2 : ScopeOK cx q2 Lp := hle.scopeOK cx hs1
    have hpn2 : q2.incremental = false → q2.pend = [] := by
      intro hf
      have hp1 := hn1 (by rw [← hle.flag_incremental]; exact hf)
      have := hle.pend_map; rw [hp1] at this; simpa using this
    obtain ⟨t, q, qs, hq, hj, htt, hst⟩ := closeChild cx hP _ l2 q2 ps2 (b.leaves cx) Lp cl h2 hs2 hn2 ht2
      hok hsp2 hpn2 hlen
    have hFt : StepOK cx s t p ps Lp := StepOK.jump cx hF (hj1.trans hj) htt
    apply StepOK.trans cx hFt
    intro l3 ls3 h3 _ _ _ _ _
    rw [hq] at h3
    obtain ⟨rfl, rfl⟩ := List.cons.inj h3
    exact hst

/-- **A body of calls** appends its leaves (chunks and children's roots) to
    the scope. -/
theorem Body.step_ok (cx : Ctx C) (hP : ParamsOK cx.P) : ∀ (b : Body C) (s : St C) (l : Layer C)
    (ls : List (Layer C)) (L : List C), s.stack = l :: ls → ScopeOK cx l L →
    (l.incremental = false → l.pend = []) → TopOK s → b.WF cx → (L ++ b.leaves cx).length < 2 ^ 40 →
    StepOK cx s (b.ops.foldl (St.step cx) s) l ls (L ++ b.leaves cx)
  | .nil, s, l, ls, L, h, hs, hn, ht, _, _ => by
    simp only [Body.ops, Body.leaves, List.foldl_nil, List.append_nil]
    exact StepOK.refl cx h hs hn ht
  | .leaf v r, s, l, ls, L, h, hs, hn, ht, hwf, hlen => by
    simp only [Body.ops, Body.leaves, List.foldl_cons, step_leaf] at hlen ⊢
    apply StepOK.trans cx (push_scope cx s l ls L v h hs hn ht)
    intro l2 ls2 h2 hs2 _ hn2 ht2 _
    have := Body.step_ok cx hP r _ l2 ls2 (L ++ [v]) h2 hs2 hn2 ht2 hwf (by simpa using hlen)
    simpa using this
  | .collapse a r, s, l, ls, L, h, hs, hn, ht, hwf, hlen => by
    simp only [Body.ops, Body.leaves, List.foldl_cons] at hlen ⊢
    apply StepOK.trans cx (collapse_scope cx hP s l ls L h hs hn ht (by simp at hlen; omega) a)
    intro l2 ls2 h2 hs2 _ hn2 ht2 _
    exact Body.step_ok cx hP r _ l2 ls2 L h2 hs2 hn2 ht2 hwf hlen
  | .sub t r, s, l, ls, L, h, hs, hn, ht, hwf, hlen => by
    obtain ⟨hwt, hwr⟩ := hwf
    simp only [Body.ops, Body.leaves, List.foldl_append] at hlen ⊢
    apply StepOK.trans cx (Tree.step_ok cx hP t s l ls L h hs hn ht hwt (by simp at hlen ⊢; omega))
    intro l2 ls2 h2 hs2 _ hn2 ht2 _
    have := Body.step_ok cx hP r _ l2 ls2 (L ++ [t.root cx]) h2 hs2 hn2 ht2 hwr (by simpa using hlen)
    simpa using this
end

/-! ## The hasher's root -/

theorem start_kind (cx : Ctx C) (k : Kind) (pk : Bool) :
    ({} : St C).step cx (k.openOp pk) = { stack := [k.layer pk] } := by
  cases k <;> rfl

theorem topOK_empty : TopOK ({} : St C) :=
  ⟨⟨fun j hj => by simp at hj, fun q v id idx hq => by simp [St.cells] at hq, by simp, fun j hj => by simp at hj⟩,
    rfl, fun j hj => by simp at hj, fun j hj => by simp at hj⟩

/-- A top-level close leaves one ready root in an otherwise empty hasher. -/
theorem root_after_jump (s t : St C) (l : Layer C) (h : s.stack = [l]) (hb : s.base.map Cell.erase = [])
    (hh : s.hashErr = none) (hj : Jump s.pop t) (ht : TopOK t) (r : C) : (t.push (.ready r)).root = some r := by
  have hv := hj.view
  simp only [St.view, St.pop, h, List.tail_cons, List.map_nil, Prod.mk.injEq] at hv
  apply root_single t ht (erase_nil_layers hv.2) (by have := hv.1; rw [hb] at this; simpa using this)
    (by rw [hj.herr]; exact hh)

/-- **The hasher computes the specified root.** For every tree of calls the
    engines make (binary and non-incremental scopes nested to any depth;
    leaves; `Collapse` hints anywhere, each synchronous or asynchronous;
    children closed with `Merkleize` — deferred into an incremental parent
    whenever Go defers them — or `MerkleizeWithMixin`), `HashRoot` returns
    the root the SSZ specification assigns, and none of the model's
    protocol checks fires (no hole read or moved before its drain, no
    scratch write over a live chunk, no impossible state). -/
theorem run_tree (cx : Ctx C) (hP : ParamsOK cx.P) (t : Tree C) (hwf : t.WF cx) :
    (run cx t.ops).root = some (t.root cx) := by
  cases t with
  | mk k pk b cl =>
    obtain ⟨hwb, hwl, hok⟩ := hwf
    unfold run
    simp only [Tree.ops, Tree.root]
    rw [List.foldl_append, List.foldl_cons, List.foldl_cons, List.foldl_nil, start_kind]
    have ht0 : TopOK ({ stack := [k.layer pk] } : St C) :=
      topOK_open ({} : St C) (k.layer pk) (Kind.layer_cells k pk) topOK_empty
    have hb := Body.step_ok cx hP b { stack := [k.layer pk] } (k.layer pk) [] [] rfl (Kind.layer_ok cx k pk)
      (fun _ => by cases k <;> rfl) ht0 hwb (by simpa using hwl)
    simp only [List.nil_append] at hb
    generalize (b.ops.foldl (St.step cx) { stack := [k.layer pk] }) = s2 at hb ⊢
    obtain ⟨l2, ls2, h2, hs2, hi2, hn2, ht2, hle2, hba2, hh2, _, _⟩ := hb
    rw [Kind.layer_inc] at hi2
    rw [erase_nil_layers hle2] at h2
    have hba : s2.base.map Cell.erase = [] := by simpa using hba2
    have hh : s2.hashErr = none := hh2
    have hs0 := hs2
    obtain ⟨_, _, hbs, _⟩ := hs0
    have hnp : l2.progressive = false := hbs.noprog
    cases cl with
    | prog a =>
      have hi : l2.incremental = false := hi2.trans hok
      obtain ⟨t, hj, htt, he⟩ := prog_none cx s2 l2 [] _ a h2 hs2 hi (hn2 hi) ht2
      show (s2.step cx (.merkleizeProg a)).root = _
      rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
    | progMixin num a =>
      have hi : l2.incremental = false := hi2.trans hok
      obtain ⟨t, hj, htt, he⟩ := progWord_none cx s2 l2 [] _ (cx.word num) a h2 hs2 hi (hn2 hi) ht2
      show (s2.closeProgWord cx (cx.word num) a).root = _
      rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
    | progActive af a =>
      have hi : l2.incremental = false := hi2.trans hok
      obtain ⟨t, hj, htt, he⟩ := progWord_none cx s2 l2 [] _ af a h2 hs2 hi (hn2 hi) ht2
      show (s2.closeProgWord cx af a).root = _
      rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
    | mixin num limit a =>
      have hstep : s2.step cx (CloseOp.mixin num limit a).op =
          ((s2.flushSync cx).drainFor (s2.flushSync cx).topStart).mixinClose cx num limit := by
        simp only [CloseOp.op, St.step, h2, hnp, Bool.false_eq_true, if_false]
      obtain ⟨t, hj, htt, he⟩ := mixin_scope cx s2 l2 [] _ num limit h2 hs2 ht2 hok
      rw [hstep, he]
      exact root_after_jump s2 t l2 h2 hba hh hj htt _
    | merkleize a =>
      obtain ⟨t, hj, htt, he⟩ := closeBinary_scope cx s2 l2 [] _ 0 a h2 hs2 ht2 (Or.inl rfl)
      have hstep : s2.step cx (CloseOp.merkleize a).op = s2.closeBinary cx 0 a := by
        simp only [CloseOp.op, St.step, h2, hnp, Bool.false_eq_true, if_false]
        split
        · rename_i hls _; cases hls
        · rfl
      rw [hstep, he]
      have := root_after_jump s2 t l2 h2 hba hh hj htt (mroot cx.H cx.z0 (closeDepth (b.leaves cx).length 0) (b.leaves cx))
      simpa [CloseOp.rootOf, closeDepth] using this

end Ssz.Hasher