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

/-! ## The grammar -/

/-- The scopes the engines open for a value. -/
inductive Kind where
  | binary
  | none
  deriving DecidableEq

def Kind.tt : Kind → TreeType
  | .binary => .binary
  | .none => .none

def Kind.layer : Kind → Bool → Layer C
  | .binary, p => { incremental := true, legacy := false, progressive := false, packed := p }
  | .none, p => { incremental := false, legacy := false, progressive := false, packed := p }

/-- How a scope is closed. -/
inductive CloseOp where
  | merkleize (async : Bool)
  | mixin (num limit : Nat) (async : Bool)

def CloseOp.op : CloseOp → Op C
  | .merkleize a => .merkleize a
  | .mixin num limit a => .merkleizeMixin num limit a

/-- The specified root of a closed scope over its leaves. -/
def CloseOp.rootOf (cx : Ctx C) : CloseOp → List C → C
  | .merkleize _, L => mroot cx.H cx.z0 (depthOf L.length) L
  | .mixin num limit _, L => cx.H (mroot cx.H cx.z0 (closeDepth L.length limit) L) (cx.word num)

/-- The leaves fit the limit. -/
def CloseOp.ok : CloseOp → Nat → Prop
  | .merkleize _, _ => True
  | .mixin _ limit _, n => limit = 0 ∨ n ≤ limit

theorem depthOf_pos {n : Nat} (h : 2 ≤ n) : 0 < depthOf n := by
  unfold depthOf; rw [if_neg (by omega)]; omega

/-- **A child scope's close**, deferred into its parent or reduced. -/
theorem closeChild (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l p : Layer C) (ps : List (Layer C))
    (Lc Lp : List C) (cl : CloseOp) (h : s.stack = l :: p :: ps) (hs : ScopeOK cx l Lc)
    (hn : l.incremental = false → l.pend = []) (ht : TopOK s) (hok : cl.ok Lc.length)
    (hp : ScopeOK cx p Lp) (hpn : p.incremental = false → p.pend = [])
    (hLp : (Lp ++ [cl.rootOf cx Lc]).length < 2 ^ 40) :
    ∃ t q qs, t.stack = q :: qs ∧ Jump s.pop t ∧ TopOK t ∧
      StepOK cx t (s.step cx cl.op) q qs (Lp ++ [cl.rootOf cx Lc]) := by
  have hs0 := hs
  obtain ⟨_, _, hbs, _⟩ := hs0
  have hnp : l.progressive = false := hbs.noprog
  have hnl : l.legacy = false := hbs.nolegacy
  cases cl with
  | mixin num limit a =>
    have hstep : s.step cx (CloseOp.mixin num limit a).op =
        ((s.flushSync cx).drainFor (s.flushSync cx).topStart).mixinClose cx num limit := by
      simp only [CloseOp.op, St.step, h, hnp, Bool.false_eq_true, if_false]
    obtain ⟨t, hj, htt, he⟩ := mixin_scope cx s l (p :: ps) Lc num limit h hs ht hok
    obtain ⟨q, qs, hq, hst⟩ := push_after_jump cx s t l p ps Lp _ h hj htt hp hpn
    refine ⟨t, q, qs, hq, hj, htt, ?_⟩
    rw [hstep, he]; exact hst
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

end Ssz.Hasher