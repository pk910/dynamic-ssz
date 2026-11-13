import SszCert.Hasher.ProgOps

/-!
# All scopes, nested

The call grammar with every scope kind the engines open — binary,
non-incremental, `Index` and progressive — nested to any depth, and the end
theorem: the hasher's root is the specified root, and none of the model's
protocol checks fires.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- **The invariant of any scope**: progressive, or binary/non-incremental. -/
def LayerOK (cx : Ctx C) (l : Layer C) (L : List C) : Prop :=
  if l.progressive then ProgScope cx l L else ScopeOK cx l L ∧ (l.incremental = false → l.pend = [])

theorem layerOK_bin {cx : Ctx C} {l : Layer C} {L : List C} (h : LayerOK cx l L) (hp : l.progressive = false) :
    ScopeOK cx l L ∧ (l.incremental = false → l.pend = []) := by
  unfold LayerOK at h; rw [hp] at h; simpa using h

theorem layerOK_prog {cx : Ctx C} {l : Layer C} {L : List C} (h : LayerOK cx l L) (hp : l.progressive = true) :
    ProgScope cx l L := by
  unfold LayerOK at h; rw [hp] at h; simpa using h

theorem layerOK_of_bin {cx : Ctx C} {l : Layer C} {L : List C} (h : ScopeOK cx l L)
    (hn : l.incremental = false → l.pend = []) : LayerOK cx l L := by
  have hp : l.progressive = false := by obtain ⟨_, _, hb, _⟩ := h; exact hb.noprog
  unfold LayerOK; rw [hp]; exact ⟨h, hn⟩

theorem layerOK_of_prog {cx : Ctx C} {l : Layer C} {L : List C} (h : ProgScope cx l L) : LayerOK cx l L := by
  unfold LayerOK; rw [h.flags.1]; exact h

theorem LayerLE.layerOK (cx : Ctx C) {l l' : Layer C} (h : LayerLE l l') {L : List C} (hs : LayerOK cx l L) :
    LayerOK cx l' L := by
  cases hp : l.progressive
  · obtain ⟨hs1, hn⟩ := layerOK_bin hs hp
    apply layerOK_of_bin (h.scopeOK cx hs1)
    intro hf
    have hp1 := hn (by rw [← h.flag_incremental]; exact hf)
    have := h.pend_map; rw [hp1] at this; simpa using this
  · exact layerOK_of_prog (h.progScope cx (layerOK_prog hs hp))

/-- The result of calls on any scope. -/
def StepL (cx : Ctx C) (s s' : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) : Prop :=
  ∃ l' ls', s'.stack = l' :: ls' ∧ LayerOK cx l' L ∧ l'.progressive = l.progressive ∧
    l'.incremental = l.incremental ∧ TopOK s' ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
    s'.base.map Cell.erase = s.base.map Cell.erase ∧ s'.hashErr = s.hashErr ∧
    s'.below.length = s.below.length ∧ HolesSub s.below s'.below

theorem StepOK.toL {cx : Ctx C} {s s' : St C} {l : Layer C} {ls : List (Layer C)} {L : List C}
    (h : StepOK cx s s' l ls L) (hp : l.progressive = false) : StepL cx s s' l ls L := by
  obtain ⟨l', ls', h', hs', hi', hn', ht', hle', hba', hh', hbl', hsub'⟩ := h
  have hp' : l'.progressive = false := by obtain ⟨_, _, hb, _⟩ := hs'; exact hb.noprog
  exact ⟨l', ls', h', layerOK_of_bin hs' hn', by rw [hp', hp], hi', ht', hle', hba', hh', hbl', hsub'⟩

theorem PStep.toL {cx : Ctx C} {s s' : St C} {l : Layer C} {ls : List (Layer C)} {L : List C}
    (h : PStep cx s s' l ls L) (hp : l.progressive = true) (hi : l.incremental = true) : StepL cx s s' l ls L := by
  obtain ⟨l', ls', h', hs', ht', hle', hba', hh', hbl', hsub'⟩ := h
  exact ⟨l', ls', h', layerOK_of_prog hs', by rw [hs'.flags.1, hp], by rw [hs'.flags.2.1, hi], ht', hle', hba', hh',
    hbl', hsub'⟩

theorem StepL.refl (cx : Ctx C) {s : St C} {l : Layer C} {ls : List (Layer C)} {L : List C}
    (h : s.stack = l :: ls) (hs : LayerOK cx l L) (ht : TopOK s) : StepL cx s s l ls L :=
  ⟨l, ls, h, hs, rfl, rfl, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩

theorem StepL.trans (cx : Ctx C) {s1 s2 s3 : St C} {l1 : Layer C} {ls1 : List (Layer C)} {L L' : List C}
    (h12 : StepL cx s1 s2 l1 ls1 L)
    (h23 : ∀ l2 ls2, s2.stack = l2 :: ls2 → LayerOK cx l2 L → l2.progressive = l1.progressive →
      l2.incremental = l1.incremental → TopOK s2 → ls2.map Layer.erase = ls1.map Layer.erase →
      StepL cx s2 s3 l2 ls2 L') :
    StepL cx s1 s3 l1 ls1 L' := by
  obtain ⟨l2, ls2, h2, hs2, hp2, hi2, ht2, hle2, hba2, hh2, hbl2, hsub2⟩ := h12
  obtain ⟨l3, ls3, h3, hs3, hp3, hi3, ht3, hle3, hba3, hh3, hbl3, hsub3⟩ := h23 l2 ls2 h2 hs2 hp2 hi2 ht2 hle2
  exact ⟨l3, ls3, h3, hs3, hp3.trans hp2, hi3.trans hi2, ht3, hle3.trans hle2, hba3.trans hba2, hh3.trans hh2,
    hbl3.trans hbl2, HolesSub.trans hsub2 hsub3⟩

theorem StepL.jump (cx : Ctx C) {s sF t : St C} {p : Layer C} {ps : List (Layer C)} {L : List C}
    (h1 : StepL cx s sF p ps L) (hj : Jump sF t) (ht : TopOK t) : StepL cx s t p ps L := by
  obtain ⟨p1, ps1, h1s, hs1, hp1, hi1, _, hle1, hba1, hh1, hbl1, hsub1⟩ := h1
  obtain ⟨p3, ps3, h3, hle, hle3⟩ := LayerLE.of_view hj.view h1s hj.holes
  refine ⟨p3, ps3, h3, hle.layerOK cx hs1, hle.flag_progressive.trans hp1, hle.flag_incremental.trans hi1, ht,
    hle3.trans hle1, ?_, hj.herr.trans hh1, ?_, HolesSub.trans hsub1 (holesSub_below hj.view h1s h3 hj.holes)⟩
  · have := congrArg Prod.fst hj.view; simp only [St.view] at this; rw [this]; exact hba1
  · rw [below_length_of_view hj.view h1s h3]; exact hbl1

/-! ## Calls on any scope -/

theorem push_L (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C) (v : C)
    (h : s.stack = l :: ls) (hs : LayerOK cx l L) (ht : TopOK s) :
    StepL cx s (s.push (.ready v)) l ls (L ++ [v]) := by
  cases hp : l.progressive
  · obtain ⟨hs1, hn⟩ := layerOK_bin hs hp
    exact (push_scope cx s l ls L v h hs1 hn ht).toL hp
  · have hs1 := layerOK_prog hs hp
    exact (push_prog cx s l ls L v h hs1 ht).toL hp hs1.flags.2.1

theorem collapse_L (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : LayerOK cx l L) (ht : TopOK s) (hL : L.length < 2 ^ 40) (a : Bool) :
    StepL cx s (s.step cx (.collapse a)) l ls L := by
  cases hp : l.progressive
  · obtain ⟨hs1, hn⟩ := layerOK_bin hs hp
    exact (collapse_scope cx hP s l ls L h hs1 hn ht hL a).toL hp
  · have hs1 := layerOK_prog hs hp
    exact (collapse_prog cx hP s l ls L h hs1 ht hL a).toL hp hs1.flags.2.1

theorem flushSync_L (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : LayerOK cx l L) (ht : TopOK s) :
    ∃ l', (s.flushSync cx).stack = l' :: ls ∧ LayerOK cx l' L ∧ l'.pend = [] ∧ l'.progressive = l.progressive ∧
      l'.incremental = l.incremental ∧ TopOK (s.flushSync cx) ∧ (s.flushSync cx).below = s.below ∧
      (s.flushSync cx).base = s.base ∧ (s.flushSync cx).hashErr = s.hashErr := by
  cases hp : l.progressive
  · obtain ⟨hs1, _⟩ := layerOK_bin hs hp
    obtain ⟨l', h', hb', hi', hp', ht', hbl', hba', hh'⟩ := flushSync_scope cx s l ls L h hs1 ht
    exact ⟨l', h', layerOK_of_bin (scopeOK_of_bin cx hb') (fun _ => hb'.pend), hb'.pend, by rw [hp', hp], hi', ht',
      hbl', hba', hh'⟩
  · have hs1 := layerOK_prog hs hp
    obtain ⟨l', h', hs', hpe', hpg', hlv', hco', ht', hbl', hba', hh'⟩ := flushSync_prog cx s l ls L h hs1 ht
    exact ⟨l', h', layerOK_of_prog hs', hpe', by rw [hs'.flags.1], by rw [hs'.flags.2.1, hs1.flags.2.1], ht', hbl',
      hba', hh'⟩

theorem flush_L (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : LayerOK cx l L) (hinc : l.incremental = true) (ht : TopOK s)
    (hL : L.length < 2 ^ 40) (aa a : Bool) :
    StepL cx s (s.flush cx aa a) l ls L := by
  cases hp : l.progressive
  · obtain ⟨hs1, _⟩ := layerOK_bin hs hp
    exact (flush_scope cx hP s l ls L h hs1 hinc ht hL aa a).toL hp
  · have hs1 := layerOK_prog hs hp
    exact (flush_prog cx hP s l ls L h hs1 ht aa a).toL hp hinc

theorem deferInto_L (cx : Ctx C) (t : St C) (p : Layer C) (ps : List (Layer C)) (Lp : List C)
    (elem : List (Cell C)) (c k : Nat) (r : C)
    (h : t.stack = p :: ps) (hs : LayerOK cx p Lp) (hinc : p.incremental = true) (ht : TopOK t)
    (hcond : p.pend = [] ∨ (p.pendC = c ∧ p.tail = []))
    (he : ∀ x ∈ elem, x.isReady = true) (hel : elem.length = c) (hc : c = 2 ^ k) (hk : 0 < k)
    (hkb : 2 ^ k ≤ cx.P.batch) (hr : reduceElem cx.H cx.z0 (elem.map Cell.val) = r) :
    StepL cx t (t.setTop { p with pendC := c, pend := p.pend ++ [elem] }) p ps (Lp ++ [r]) := by
  by_cases hp : p.progressive = true
  · have hs1 := layerOK_prog hs hp
    exact (deferInto_prog cx t p ps Lp elem c k r h hs1 ht hcond he hel hc hk hkb hr).toL hp hinc
  · have hp' : p.progressive = false := by simpa using hp
    obtain ⟨hs1, _⟩ := layerOK_bin hs hp'
    exact (deferInto_scope cx t p ps Lp elem c k r h hs1 hinc ht hcond he hel hc hk hkb hr).toL hp'

/-- A closed child's root lands in any parent. -/
theorem push_after_jumpL (cx : Ctx C) (s t : St C) (l p : Layer C) (ps : List (Layer C)) (Lp : List C) (r : C)
    (h : s.stack = l :: p :: ps) (hj : Jump s.pop t) (ht : TopOK t) (hp : LayerOK cx p Lp) :
    ∃ q qs, t.stack = q :: qs ∧ LayerLE p q ∧ StepL cx t (t.push (.ready r)) q qs (Lp ++ [r]) := by
  have hps : s.pop.stack = p :: ps := by simp [St.pop, h]
  obtain ⟨q, qs, hq, hle, _⟩ := LayerLE.of_view hj.view hps hj.holes
  exact ⟨q, qs, hq, hle, push_L cx t q qs Lp r hq (hle.layerOK cx hp) ht⟩

/-! ## Closing a child into any parent -/

/-- **A child scope's close**, deferred into its parent or reduced. -/
theorem closeChildBin (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l p : Layer C) (ps : List (Layer C))
    (Lc Lp : List C) (cl : CloseOp C) (h : s.stack = l :: p :: ps) (hs : ScopeOK cx l Lc)
    (hn : l.incremental = false → l.pend = []) (ht : TopOK s) (hok : cl.ok l.incremental Lc.length)
    (hp : LayerOK cx p Lp)
    (hLp : (Lp ++ [cl.rootOf cx Lc]).length < 2 ^ 40) :
    ∃ t q qs, t.stack = q :: qs ∧ Jump s.pop t ∧ TopOK t ∧
      StepL cx t (s.step cx cl.op) q qs (Lp ++ [cl.rootOf cx Lc]) := by
  have hs0 := hs
  obtain ⟨_, _, hbs, _⟩ := hs0
  have hnp : l.progressive = false := hbs.noprog
  cases cl with
  | mixin num limit a =>
    have hstep : s.step cx (CloseOp.mixin num limit a).op =
        ((s.flushSync cx).drainFor (s.flushSync cx).topStart).mixinClose cx num limit := by
      simp only [CloseOp.op, St.step, h, hnp, Bool.false_eq_true, if_false]
    obtain ⟨t, hj, htt, he⟩ := mixin_scope cx s l (p :: ps) Lc num limit h hs ht hok
    obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
    refine ⟨t, q, qs, hq, hj, htt, ?_⟩
    rw [hstep, he]; exact hst
  | prog a =>
    obtain ⟨t, hj, htt, he⟩ := prog_none cx s l (p :: ps) Lc a h hs hok (hn hok) ht
    obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
    exact ⟨t, q, qs, hq, hj, htt, by show StepL cx t (s.step cx (.merkleizeProg a)) q qs _; rw [he]; exact hst⟩
  | progMixin num a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_none cx s l (p :: ps) Lc (cx.word num) a h hs hok (hn hok) ht
    obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
    exact ⟨t, q, qs, hq, hj, htt, by
      show StepL cx t (s.closeProgWord cx (cx.word num) a) q qs _; rw [he]; exact hst⟩
  | progActive af a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_none cx s l (p :: ps) Lc af a h hs hok (hn hok) ht
    obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
    exact ⟨t, q, qs, hq, hj, htt, by show StepL cx t (s.closeProgWord cx af a) q qs _; rw [he]; exact hst⟩
  | merkleize a =>
    -- the reduced close
    have hred : ∃ t q qs, t.stack = q :: qs ∧ Jump s.pop t ∧ TopOK t ∧
        StepL cx t (s.closeBinary cx 0 a) q qs (Lp ++ [(CloseOp.merkleize a).rootOf cx Lc]) := by
      obtain ⟨t, hj, htt, he⟩ := closeBinary_scope cx s l (p :: ps) Lc 0 a h hs ht (Or.inl rfl)
      obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
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
      have hfin : ∀ (T2 : St C) (p2 : Layer C), T2.stack = p2 :: ps → LayerOK cx p2 Lp → p2.incremental = true →
          TopOK T2 → (p2.pend = [] ∨ (p2.pendC = c ∧ p2.tail = [])) →
          StepL cx T2 (if (p2.pend.length + 1) * c ≥ cx.P.cap then
              (T2.setTop { p2 with pendC := c, pend := p2.pend ++ [elem] }).flush cx true a
            else T2.setTop { p2 with pendC := c, pend := p2.pend ++ [elem] }) p2 ps
            (Lp ++ [(CloseOp.merkleize a).rootOf cx Lc]) := by
        intro T2 p2 h2 hs2 hi2 ht2 hc2
        have hd := deferInto_L cx T2 p2 ps Lp elem c (depthOf Lc.length) _ h2 hs2 hi2 ht2 hc2 helem hell hcw
          hk0 hkb hroot
        split
        · apply StepL.trans cx hd
          intro l3 ls3 h3 hs3 _ hi3 ht3 _
          exact flush_L cx hP _ l3 ls3 _ h3 hs3 (hi3.trans hi2) ht3 hLp true a
        · exact hd
      by_cases hcond : p.pend ≠ [] ∧ (p.pendC ≠ c ∨ p.tail ≠ [])
      · rw [if_pos hcond]
        obtain ⟨p1, hF, hl1, hpe1, hpr1, hi1, htF, hbF, hbaF, hhF⟩ :=
          flushSync_L cx ({ s with stack := p :: ps } : St C) p ps Lp rfl hp hT
        rw [hF]
        simp only
        apply StepL.trans cx (s2 := St.flushSync cx { s with stack := p :: ps })
          ⟨p1, ps, hF, hl1, hpr1, hi1, htF, rfl, by rw [hbaF], hhF,
            by rw [hbF], by rw [hbF]; exact HolesSub.refl _⟩
        intro l2 ls2 h2 hs2 _ hi2 ht2 _
        rw [hF] at h2
        obtain ⟨rfl, rfl⟩ := List.cons.inj h2
        exact hfin _ _ hF hs2 (hi2.trans hpi') ht2 (Or.inl hpe1)
      · rw [if_neg hcond]
        simp only
        have hc2 : p.pend = [] ∨ (p.pendC = c ∧ p.tail = []) := by
          by_cases hpe : p.pend = []
          · exact Or.inl hpe
          · right; simp only [ne_eq, hpe, not_false_eq_true, true_and, not_or, Classical.not_not] at hcond
            exact hcond
        exact hfin _ _ rfl hp hpi' hT hc2
    · exact hred

/-- The progressive closes. -/
def CloseOp.isProg : CloseOp C → Prop
  | .prog _ => True
  | .progMixin _ _ => True
  | .progActive _ _ => True
  | _ => False

/-- **A progressive child's close** into any parent. -/
theorem closeChildProg (cx : Ctx C) (hP : ParamsOK cx.P) (s : St C) (l p : Layer C) (ps : List (Layer C))
    (Lc Lp : List C) (cl : CloseOp C) (h : s.stack = l :: p :: ps) (hs : ProgScope cx l Lc) (ht : TopOK s)
    (hok : cl.isProg) (hLc : Lc.length < 2 ^ 40) (hp : LayerOK cx p Lp) :
    ∃ t q qs, t.stack = q :: qs ∧ Jump s.pop t ∧ TopOK t ∧
      StepL cx t (s.step cx cl.op) q qs (Lp ++ [cl.rootOf cx Lc]) := by
  cases cl with
  | prog a =>
    obtain ⟨t, hj, htt, he⟩ := progClose_prog cx hP s l (p :: ps) Lc a h hs ht hLc
    obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
    exact ⟨t, q, qs, hq, hj, htt, by show StepL cx t (s.step cx (.merkleizeProg a)) q qs _; rw [he]; exact hst⟩
  | progMixin num a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_prog cx hP s l (p :: ps) Lc (cx.word num) a h hs ht hLc
    obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
    exact ⟨t, q, qs, hq, hj, htt, by
      show StepL cx t (s.closeProgWord cx (cx.word num) a) q qs _; rw [he]; exact hst⟩
  | progActive af a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_prog cx hP s l (p :: ps) Lc af a h hs ht hLc
    obtain ⟨q, qs, hq, _, hst⟩ := push_after_jumpL cx s t l p ps Lp _ h hj htt hp
    exact ⟨t, q, qs, hq, hj, htt, by show StepL cx t (s.closeProgWord cx af a) q qs _; rw [he]; exact hst⟩
  | merkleize _ => exact hok.elim
  | mixin _ _ _ => exact hok.elim

/-! ## The grammar, with every scope kind -/

/-- Every scope the engines open. -/
inductive SKind where
  | binary
  | none
  | index
  | progressive

def SKind.tt : SKind → TreeType
  | .binary => .binary
  | .none => .none
  | .index => .none
  | .progressive => .progressive

def SKind.openOp : SKind → Bool → Op C
  | .index, _ => .index
  | k, p => .startTree k.tt p

def SKind.layer : SKind → Bool → Layer C
  | .binary, p => { incremental := true, legacy := false, progressive := false, packed := p }
  | .none, p => { incremental := false, legacy := false, progressive := false, packed := p }
  | .index, _ => { incremental := false, legacy := true, progressive := false, packed := false }
  | .progressive, p => { incremental := true, legacy := false, progressive := true, packed := p }

/-- The close fits the scope: a progressive close on a progressive or
    non-incremental scope, a binary close on a non-progressive one, the
    leaves within a mix-in's limit. -/
def SKind.ok : SKind → CloseOp C → Nat → Prop
  | .progressive, cl, _ => cl.isProg
  | .binary, cl, n => ¬ cl.isProg ∧ cl.ok true n
  | _, cl, n => cl.ok false n

/-- The layer `StartTree` pushes, as the model writes it. -/
def SKind.rawLayer (k : SKind) (pk : Bool) : Layer C := { incremental := decide (k.tt ≠ TreeType.none), legacy := false, progressive := k.tt == TreeType.progressive, packed := pk }

theorem SKind.layer_cells (k : SKind) (p : Bool) : (k.layer p : Layer C).cells = [] := by cases k <;> rfl

theorem SKind.layer_ok (cx : Ctx C) (k : SKind) (p : Bool) : LayerOK cx (k.layer p : Layer C) [] := by
  cases k
  · exact layerOK_of_bin (Kind.layer_ok cx .binary p) (fun h => by cases h)
  · exact layerOK_of_bin (Kind.layer_ok cx .none p) (fun _ => rfl)
  · exact layerOK_of_bin (Kind.layer_ok cx .index p) (fun _ => rfl)
  · apply layerOK_of_prog
    refine ⟨[], [], rfl, ⟨rfl, rfl, rfl, ⟨rfl, fun i h => by simp at h⟩, rfl, rfl, fun n hn => by simp [SKind.layer] at hn,
      fun n hn => by simp [SKind.layer] at hn, fun _ => ⟨rfl, fun n hn => by simp [SKind.layer] at hn⟩,
      ⟨fun _ => rfl, by simp [SKind.layer], by simp [SKind.layer], by simp [SKind.layer], fun h => absurd rfl h⟩⟩⟩

/-- `StartTree` (or `Index`) on any scope. -/
theorem open_childL (cx : Ctx C) (s : St C) (p : Layer C) (ps : List (Layer C)) (Lp : List C) (k : SKind)
    (pk : Bool) (h : s.stack = p :: ps) (hs : LayerOK cx p Lp) (ht : TopOK s) :
    ∃ sF, StepL cx s sF p ps Lp ∧ s.step cx (k.openOp pk) = { sF with stack := k.layer pk :: sF.stack } := by
  by_cases hk : k = .index
  · subst hk; exact ⟨s, StepL.refl cx h hs ht, rfl⟩
  have hop : (k.openOp pk : Op C) = .startTree k.tt pk := by cases k <;> simp_all [SKind.openOp]
  rw [hop]
  have hl : ∀ t : St C, ({ t with stack := SKind.rawLayer k pk :: t.stack } : St C) =
      { t with stack := k.layer pk :: t.stack } := by
    intro t; cases k <;> first | rfl | exact absurd rfl hk
  by_cases hc : k.tt ≠ TreeType.none ∧ p.pend ≠ []
  · obtain ⟨p1, hF, hl1, _, hpr1, hi1, htF, hbF, hbaF, hhF⟩ := flushSync_L cx s p ps Lp h hs ht
    refine ⟨s.flushSync cx, ⟨p1, ps, hF, hl1, hpr1, hi1, htF, rfl, by rw [hbaF], hhF, by rw [hbF],
      by rw [hbF]; exact HolesSub.refl _⟩, ?_⟩
    simp only [St.step, h]; rw [if_pos hc]; exact hl _
  · refine ⟨s, StepL.refl cx h hs ht, ?_⟩
    simp only [St.step, h]; rw [if_neg hc]
    have := hl s; rw [h] at this ⊢; exact this

/-! ## Trees of calls with every scope kind -/

mutual
/-- A scope of any kind: opened, filled, closed. -/
inductive STree (C : Type) where
  | mk (kind : SKind) (packed : Bool) (body : SBody C) (close : CloseOp C)
/-- A body: leaves, `Collapse` hints (either mode) and child scopes. -/
inductive SBody (C : Type) where
  | nil
  | leaf (v : C) (rest : SBody C)
  | collapse (async : Bool) (rest : SBody C)
  | sub (t : STree C) (rest : SBody C)
end

mutual
def STree.ops : STree C → List (Op C)
  | .mk k p b c => k.openOp p :: b.ops ++ [c.op]
def SBody.ops : SBody C → List (Op C)
  | .nil => []
  | .leaf v r => .leaf v :: r.ops
  | .collapse a r => .collapse a :: r.ops
  | .sub t r => t.ops ++ r.ops
end

mutual
/-- **The specified root** of a scope: bounded, mixed-in, or progressive
    merkleization of its leaves, as its close says. -/
def STree.root (cx : Ctx C) : STree C → C
  | .mk _ _ b c => c.rootOf cx (b.leaves cx)
def SBody.leaves (cx : Ctx C) : SBody C → List C
  | .nil => []
  | .leaf v r => v :: r.leaves cx
  | .collapse _ r => r.leaves cx
  | .sub t r => t.root cx :: r.leaves cx
end

mutual
def STree.WF (cx : Ctx C) : STree C → Prop
  | .mk k _ b c => b.WF cx ∧ (b.leaves cx).length < 2 ^ 40 ∧ k.ok c (b.leaves cx).length
def SBody.WF (cx : Ctx C) : SBody C → Prop
  | .nil => True
  | .leaf _ r => r.WF cx
  | .collapse _ r => r.WF cx
  | .sub t r => t.WF cx ∧ r.WF cx
end

theorem SKind.layer_prog (k : SKind) (p : Bool) :
    (k.layer p : Layer C).progressive = (match k with | .progressive => true | _ => false) := by cases k <;> rfl

/-- A child's close, by the kind of the child. -/
theorem closeChildL (cx : Ctx C) (hP : ParamsOK cx.P) (k : SKind) (s : St C) (l p : Layer C) (ps : List (Layer C))
    (Lc Lp : List C) (cl : CloseOp C) (pk : Bool) (h : s.stack = l :: p :: ps) (hs : LayerOK cx l Lc)
    (hpr : l.progressive = (k.layer pk : Layer C).progressive) (hi : l.incremental = (k.layer pk : Layer C).incremental)
    (ht : TopOK s) (hok : k.ok cl Lc.length) (hLc : Lc.length < 2 ^ 40) (hp : LayerOK cx p Lp)
    (hLp : (Lp ++ [cl.rootOf cx Lc]).length < 2 ^ 40) :
    ∃ t q qs, t.stack = q :: qs ∧ Jump s.pop t ∧ TopOK t ∧
      StepL cx t (s.step cx cl.op) q qs (Lp ++ [cl.rootOf cx Lc]) := by
  cases k with
  | progressive =>
    exact closeChildProg cx hP s l p ps Lc Lp cl h (layerOK_prog hs hpr) ht hok hLc hp
  | binary =>
    obtain ⟨hs1, hn⟩ := layerOK_bin hs hpr
    exact closeChildBin cx hP s l p ps Lc Lp cl h hs1 hn ht (by rw [hi]; exact hok.2) hp hLp
  | none =>
    obtain ⟨hs1, hn⟩ := layerOK_bin hs hpr
    exact closeChildBin cx hP s l p ps Lc Lp cl h hs1 hn ht (by rw [hi]; exact hok) hp hLp
  | index =>
    obtain ⟨hs1, hn⟩ := layerOK_bin hs hpr
    exact closeChildBin cx hP s l p ps Lc Lp cl h hs1 hn ht (by rw [hi]; exact hok) hp hLp

mutual
/-- **A child scope of any kind, whatever its calls**, adds exactly its
    specified root to its parent's leaves. -/
theorem STree.step_ok (cx : Ctx C) (hP : ParamsOK cx.P) : ∀ (t : STree C) (s : St C) (p : Layer C)
    (ps : List (Layer C)) (Lp : List C), s.stack = p :: ps → LayerOK cx p Lp → TopOK s → t.WF cx →
    (Lp ++ [t.root cx]).length < 2 ^ 40 →
    StepL cx s (t.ops.foldl (St.step cx) s) p ps (Lp ++ [t.root cx])
  | .mk k pk b cl, s, p, ps, Lp, h, hs, ht, hwf, hlen => by
    obtain ⟨hwb, hwl, hok⟩ := hwf
    simp only [STree.ops, STree.root] at hlen ⊢
    rw [List.foldl_append, List.foldl_cons, List.foldl_cons, List.foldl_nil]
    obtain ⟨sF, hF, hstep⟩ := open_childL cx s p ps Lp k pk h hs ht
    rw [hstep]
    have hF0 := hF
    obtain ⟨p1, ps1, hF1, hs1, _, _, htF, _, _, _, _, _⟩ := hF0
    have ht1 := topOK_open sF (k.layer pk) (SKind.layer_cells k pk) htF
    have hb := SBody.step_ok cx hP b { sF with stack := k.layer pk :: sF.stack } (k.layer pk) sF.stack []
      rfl (SKind.layer_ok cx k pk) ht1 hwb (by simpa using hwl)
    simp only [List.nil_append] at hb
    obtain ⟨l2, ls2, h2, hs2, hpr2, hi2, ht2, hle2, hba2, hh2, _, hsub2⟩ := hb
    rw [hF1] at hle2
    obtain ⟨p2, ps2, rfl⟩ : ∃ p2 ps2, ls2 = p2 :: ps2 := by
      cases ls2 with
      | nil => simp at hle2
      | cons p2 ps2 => exact ⟨p2, ps2, rfl⟩
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
    obtain ⟨t, q, qs, hq, hj, htt, hst⟩ := closeChildL cx hP k _ l2 q2 ps2 (b.leaves cx) Lp cl pk h2 hs2 hpr2 hi2 ht2
      hok hwl (hle.layerOK cx hs1) hlen
    have hFt : StepL cx s t p ps Lp := StepL.jump cx hF (hj1.trans hj) htt
    apply StepL.trans cx hFt
    intro l3 ls3 h3 _ _ _ _ _
    rw [hq] at h3
    obtain ⟨rfl, rfl⟩ := List.cons.inj h3
    exact hst

/-- **A body of calls** on any scope appends its leaves. -/
theorem SBody.step_ok (cx : Ctx C) (hP : ParamsOK cx.P) : ∀ (b : SBody C) (s : St C) (l : Layer C)
    (ls : List (Layer C)) (L : List C), s.stack = l :: ls → LayerOK cx l L → TopOK s → b.WF cx →
    (L ++ b.leaves cx).length < 2 ^ 40 →
    StepL cx s (b.ops.foldl (St.step cx) s) l ls (L ++ b.leaves cx)
  | .nil, s, l, ls, L, h, hs, ht, _, _ => by
    simp only [SBody.ops, SBody.leaves, List.foldl_nil, List.append_nil]
    exact StepL.refl cx h hs ht
  | .leaf v r, s, l, ls, L, h, hs, ht, hwf, hlen => by
    simp only [SBody.ops, SBody.leaves, List.foldl_cons, step_leaf] at hlen ⊢
    apply StepL.trans cx (push_L cx s l ls L v h hs ht)
    intro l2 ls2 h2 hs2 _ _ ht2 _
    have := SBody.step_ok cx hP r _ l2 ls2 (L ++ [v]) h2 hs2 ht2 hwf (by simpa using hlen)
    simpa using this
  | .collapse a r, s, l, ls, L, h, hs, ht, hwf, hlen => by
    simp only [SBody.ops, SBody.leaves, List.foldl_cons] at hlen ⊢
    apply StepL.trans cx (collapse_L cx hP s l ls L h hs ht (by simp at hlen; omega) a)
    intro l2 ls2 h2 hs2 _ _ ht2 _
    exact SBody.step_ok cx hP r _ l2 ls2 L h2 hs2 ht2 hwf hlen
  | .sub t r, s, l, ls, L, h, hs, ht, hwf, hlen => by
    obtain ⟨hwt, hwr⟩ := hwf
    simp only [SBody.ops, SBody.leaves, List.foldl_append] at hlen ⊢
    apply StepL.trans cx (STree.step_ok cx hP t s l ls L h hs ht hwt (by simp at hlen ⊢; omega))
    intro l2 ls2 h2 hs2 _ _ ht2 _
    have := SBody.step_ok cx hP r _ l2 ls2 (L ++ [t.root cx]) h2 hs2 ht2 hwr (by simpa using hlen)
    simpa using this
end

/-! ## The hasher's root, every scope kind -/

theorem start_skind (cx : Ctx C) (k : SKind) (pk : Bool) :
    ({} : St C).step cx (k.openOp pk) = { stack := [k.layer pk] } := by
  cases k <;> rfl

/-- The top-level close of a non-progressive scope. -/
theorem topClose_bin (cx : Ctx C) (s2 : St C) (l2 : Layer C) (L : List C) (cl : CloseOp C)
    (h2 : s2.stack = [l2]) (hs2 : ScopeOK cx l2 L) (hn2 : l2.incremental = false → l2.pend = [])
    (ht2 : TopOK s2) (hba : s2.base.map Cell.erase = []) (hh : s2.hashErr = none) (hok : cl.ok l2.incremental L.length) :
    (s2.step cx cl.op).root = some (cl.rootOf cx L) := by
  have hs0 := hs2
  obtain ⟨_, _, hbs, _⟩ := hs0
  have hnp : l2.progressive = false := hbs.noprog
  cases cl with
  | prog a =>
    obtain ⟨t, hj, htt, he⟩ := prog_none cx s2 l2 [] _ a h2 hs2 hok (hn2 hok) ht2
    show (s2.step cx (.merkleizeProg a)).root = _
    rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
  | progMixin num a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_none cx s2 l2 [] _ (cx.word num) a h2 hs2 hok (hn2 hok) ht2
    show (s2.closeProgWord cx (cx.word num) a).root = _
    rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
  | progActive af a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_none cx s2 l2 [] _ af a h2 hs2 hok (hn2 hok) ht2
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
    have := root_after_jump s2 t l2 h2 hba hh hj htt (mroot cx.H cx.z0 (closeDepth L.length 0) L)
    simpa [CloseOp.rootOf, closeDepth] using this

/-- The top-level close of a progressive scope. -/
theorem topClose_prog (cx : Ctx C) (hP : ParamsOK cx.P) (s2 : St C) (l2 : Layer C) (L : List C) (cl : CloseOp C)
    (h2 : s2.stack = [l2]) (hs2 : ProgScope cx l2 L) (ht2 : TopOK s2) (hba : s2.base.map Cell.erase = [])
    (hh : s2.hashErr = none) (hok : cl.isProg) (hL : L.length < 2 ^ 40) :
    (s2.step cx cl.op).root = some (cl.rootOf cx L) := by
  cases cl with
  | prog a =>
    obtain ⟨t, hj, htt, he⟩ := progClose_prog cx hP s2 l2 [] L a h2 hs2 ht2 hL
    show (s2.step cx (.merkleizeProg a)).root = _
    rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
  | progMixin num a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_prog cx hP s2 l2 [] L (cx.word num) a h2 hs2 ht2 hL
    show (s2.closeProgWord cx (cx.word num) a).root = _
    rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
  | progActive af a =>
    obtain ⟨t, hj, htt, he⟩ := progWord_prog cx hP s2 l2 [] L af a h2 hs2 ht2 hL
    show (s2.closeProgWord cx af a).root = _
    rw [he]; exact root_after_jump s2 t l2 h2 hba hh hj htt _
  | merkleize _ => exact hok.elim
  | mixin _ _ _ => exact hok.elim

/-- **The hasher computes the specified root, for every scope kind.** For
    every tree of calls the engines make — binary, non-incremental, `Index`
    and progressive (EIP-7916) scopes nested to any depth; leaves; `Collapse`
    hints anywhere, each synchronous or asynchronous; children closed with
    `Merkleize` (deferred into an incremental or progressive parent exactly
    when Go defers them), `MerkleizeWithMixin`, `MerkleizeProgressive`,
    `MerkleizeProgressiveWithMixin` or `MerkleizeProgressiveWithActiveFields`
    — `HashRoot` returns the root the SSZ specification assigns, and none of
    the model's protocol checks fires: no hole is read or moved before its
    drain, no scratch write lands on a live chunk, no impossible state is
    reached. Generic in the hash; for any thresholds with `ParamsOK`, which
    Go's production and `dynssz_smallhash` constants both satisfy. -/
theorem run_stree (cx : Ctx C) (hP : ParamsOK cx.P) (t : STree C) (hwf : t.WF cx) :
    (run cx t.ops).root = some (t.root cx) := by
  cases t with
  | mk k pk b cl =>
    obtain ⟨hwb, hwl, hok⟩ := hwf
    unfold run
    simp only [STree.ops, STree.root]
    rw [List.foldl_append, List.foldl_cons, List.foldl_cons, List.foldl_nil, start_skind]
    have ht0 : TopOK ({ stack := [k.layer pk] } : St C) :=
      topOK_open ({} : St C) (k.layer pk) (SKind.layer_cells k pk) topOK_empty
    have hb := SBody.step_ok cx hP b { stack := [k.layer pk] } (k.layer pk) [] [] rfl (SKind.layer_ok cx k pk) ht0 hwb
      (by simpa using hwl)
    simp only [List.nil_append] at hb
    generalize (b.ops.foldl (St.step cx) { stack := [k.layer pk] }) = s2 at hb ⊢
    obtain ⟨l2, ls2, h2, hs2, hpr2, hi2, ht2, hle2, hba2, hh2, _, _⟩ := hb
    rw [erase_nil_layers hle2] at h2
    have hba : s2.base.map Cell.erase = [] := by simpa using hba2
    have hh : s2.hashErr = none := hh2
    cases k with
    | progressive =>
      exact topClose_prog cx hP s2 l2 _ cl h2 (layerOK_prog hs2 hpr2) ht2 hba hh hok hwl
    | binary =>
      obtain ⟨hs, hn⟩ := layerOK_bin hs2 hpr2
      exact topClose_bin cx s2 l2 _ cl h2 hs hn ht2 hba hh (by rw [hi2]; exact hok.2)
    | none =>
      obtain ⟨hs, hn⟩ := layerOK_bin hs2 hpr2
      exact topClose_bin cx s2 l2 _ cl h2 hs hn ht2 hba hh (by rw [hi2]; exact hok)
    | index =>
      obtain ⟨hs, hn⟩ := layerOK_bin hs2 hpr2
      exact topClose_bin cx s2 l2 _ cl h2 hs hn ht2 hba hh (by rw [hi2]; exact hok)

end Ssz.Hasher