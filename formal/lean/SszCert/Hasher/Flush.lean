import SszCert.Hasher.Defer

/-!
# `flushPending`, asynchronously

Cap-wide prefixes of a deferred run of uniform elements (`2^k` chunks each)
become background jobs: each reduces `cap / 2^k` elements to one root, a node
at depth `capDepth - k` of the scope, whose hole is placed where the run
began. A run that empties lets the raw tail move down.
-/

namespace Ssz.Hasher

variable {C : Type}

/-- Uniform pieces are their own chunks. -/
theorem chunks_flatten (k : Nat) : ∀ es : List (List C), (∀ e ∈ es, e.length = 2 ^ k) → chunks k es.flatten = es
  | [], _ => by simp
  | e :: es, hw => by
    have he := hw e (by simp)
    have ih := chunks_flatten k es (fun e' h' => hw e' (List.mem_cons_of_mem _ h'))
    obtain ⟨x, xs, rfl⟩ : ∃ x xs, e = x :: xs := by
      cases e with
      | nil => simp at he; have := Nat.one_le_two_pow (n := k); omega
      | cons x xs => exact ⟨x, xs, rfl⟩
    rw [List.flatten_cons, List.cons_append, chunks_cons, ← List.cons_append,
      List.take_left' he, List.drop_left' he, ih]

theorem length_flatten_uniform {m : Nat} : ∀ es : List (List C), (∀ e ∈ es, e.length = m) →
    es.flatten.length = es.length * m
  | [], _ => by simp
  | e :: es, hw => by
    rw [List.flatten_cons, List.length_append, hw e (by simp),
      length_flatten_uniform es (fun e' h' => hw e' (List.mem_cons_of_mem _ h')), List.length_cons, Nat.succ_mul]
    omega

/-- **A block of uniform elements reduces to the root over their roots.** -/
theorem reduceTo_flatten (H : C → C → C) (z0 : C) (k a : Nat) (es : List (List C))
    (hw : ∀ e ∈ es, e.length = 2 ^ k) (hn : es.length = 2 ^ a) (fuel : Nat) (hf : a + k ≤ fuel) :
    reduceTo H 1 fuel es.flatten = [mroot H z0 a (es.map (mroot H z0 k))] := by
  have hlen : es.flatten.length = 1 * 2 ^ (a + k) := by
    rw [length_flatten_uniform es hw, hn, Nat.pow_add, Nat.one_mul]
  have hne : es.flatten ≠ [] := by
    intro e; have := congrArg List.length e; rw [hlen] at this; simp at this
  have h1 := reduceTo_chunks H z0 1 (a + k) (by omega) es.flatten hlen 0 fuel (Nat.zero_le _) (by omega)
  rw [chunk_roots_zero, chunks_single hne (by rw [hlen]; simp)] at h1
  have h2 := reduceTo_chunks H z0 1 (a + k) (by omega) es.flatten hlen k fuel (by omega) (by omega)
  rw [chunks_flatten k es hw, chunks_single hne (by rw [hlen]; simp)] at h2
  have hR : (es.map (mroot H z0 k)).length = 1 * 2 ^ a := by simp [hn]
  have hRne : es.map (mroot H z0 k) ≠ [] := by
    intro e; have := congrArg List.length e; rw [hR] at this; simp at this
  have h3 := reduceTo_chunks H z0 1 a (by omega) (es.map (mroot H z0 k)) hR 0 fuel (Nat.zero_le _) (by omega)
  rw [chunk_roots_zero, chunks_single hRne (by rw [hR]; simp)] at h3
  rw [h1]
  simp only [List.map_cons, List.map_nil] at h2 h3 ⊢
  rw [← h2, h3]

/-- Appending a full node no deeper than the cover's nodes. -/
theorem cov_append_node (H : C → C → C) (z0 : C) {d : Nat} {c : Cell C} {X : List C}
    (hX : X.length = 2 ^ d) (hv : c.val = mroot H z0 d X) :
    ∀ {ns : List (Node C)} {L : List C}, Cov H z0 true ns L → (∀ n ∈ ns, d ≤ n.depth) →
      Cov H z0 true (ns ++ [⟨d, c⟩]) (L ++ X)
  | [], L, h, _ => by
    have hL : L = [] := h
    subst hL
    have hne : X ≠ [] := by intro e; subst e; simp at hX; have := Nat.one_le_two_pow (n := d); omega
    exact ⟨hne, by simp [hX], hv⟩
  | [n], L, h, hd => by
    obtain ⟨_, hlen, hnv⟩ := h
    simp only [if_true] at hlen
    exact ⟨L, X, rfl, hlen, hnv, hd n (by simp), ⟨by intro e; subst e; simp at hX; have := Nat.one_le_two_pow (n := d); omega,
      by simp [hX], hv⟩⟩
  | n :: m :: ns, L, h, hd => by
    obtain ⟨xs, ys, hL, hx, hnv, hmn, hc⟩ := h
    have := cov_append_node H z0 hX hv hc (fun k hk => hd k (List.mem_cons_of_mem _ hk))
    exact ⟨xs, ys ++ X, by rw [hL, List.append_assoc], hx, hnv, hmn, this⟩

/-! ## The round invariant -/

/-- The invariant between rounds: like `ScopeOK`, with the raw tail allowed
    behind an emptied run (the last step moves it down). -/
structure RoundOK (cx : Ctx C) (l : Layer C) (L1 L : List C) : Prop where
  split : L = L1 ++ pendLeaves cx l
  bin : BinOK cx l.settled L1
  ready : ∀ c ∈ l.pend.flatten ++ l.tail, c.isReady = true
  width : ∀ e ∈ l.pend, e.length = l.pendC

theorem roundOK_of_scope (cx : Ctx C) {l : Layer C} {L : List C} (hs : ScopeOK cx l L) :
    ∃ L1, RoundOK cx l L1 L := by
  obtain ⟨L1, hL, hb, hpo⟩ := hs
  refine ⟨L1, hL, hb, ?_, hpo.width⟩
  intro c hc
  rcases List.mem_append.mp hc with hc | hc
  · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc; exact hpo.ready e he c hce
  · exact hpo.tready c hc

theorem LayerLE.roundOK (cx : Ctx C) {l l' : Layer C} (h : LayerLE l l') {L1 L : List C} (hs : RoundOK cx l L1 L) :
    RoundOK cx l' L1 L := by
  have hp : l.prog = [] := hs.bin.prog
  refine ⟨by rw [h.pendLeaves cx]; exact hs.split, h.binOK_settled cx hs.bin, h.rest_ready hp hs.ready, ?_⟩
  have hlens : l'.pend.map List.length = l.pend.map List.length := by
    have := congrArg (List.map List.length) h.pend_map
    simpa [List.map_map, Function.comp_def] using this
  intro e he
  rw [h.pendC]
  obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem he
  have hi0 : i < l.pend.length := by have := congrArg List.length hlens; simp at this; omega
  have := congrArg (fun xs => xs[i]?) hlens
  simp only [List.getElem?_map, List.getElem?_eq_getElem hi, List.getElem?_eq_getElem hi0, Option.map_some,
    Option.some.injEq] at this
  rw [this]; exact hs.width _ (List.getElem_mem _)

/-- What `asyncRootCompatible` grants. -/
theorem compat_spec {l : Layer C} {d : Nat} (h : asyncRootCompatible l d = true) :
    (l.collapsed = false → l.nodes = []) ∧ ∀ n ∈ l.nodes, d ≤ n.depth := by
  unfold asyncRootCompatible at h
  cases hc : l.collapsed with
  | false =>
    simp only [hc, Bool.not_false, if_true, List.isEmpty_iff] at h
    refine ⟨fun _ => h, ?_⟩
    intro n hn; rw [h] at hn; cases hn
  | true =>
    simp only [hc, Bool.not_true, Bool.false_eq_true, if_false, Bool.and_eq_true, beq_iff_eq] at h
    obtain ⟨h1, h2⟩ := h
    refine ⟨fun e => absurd e (by decide), ?_⟩
    rw [← h2, List.take_length] at h1
    intro n hn
    have := List.all_eq_true.mp h1 n hn
    simpa using this

theorem cap_div (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) {k : Nat} (hk : k ≤ cx.P.capDepth) :
    cx.P.cap / 2 ^ k = 2 ^ (cx.P.capDepth - k) := by
  rw [hcap, Nat.pow_div hk (by decide)]

/-! ## One binary round -/

theorem map_vals_flatten (es : List (List (Cell C))) :
    es.flatten.map Cell.val = (es.map (·.map Cell.val)).flatten := by
  simp [List.map_flatten]

/-- **One round of `flushPendingAsync` on a binary scope**: the first `cap/c`
    elements become a background job whose hole is the new node, one level
    above their roots per halving; the leaves stay the same. -/
theorem binRound_bin (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (s : St C) (l : Layer C)
    (ls : List (Layer C)) (L1 L : List C) (k : Nat)
    (h : s.stack = l :: ls) (hs : RoundOK cx l L1 L) (hinc : l.incremental = true) (ht : TopOK s)
    (hL : L.length < 2 ^ 40) (hk : l.pendC = 2 ^ k) (hk0 : k < cx.P.capDepth)
    (hpl : cx.P.cap / l.pendC ≤ l.pend.length)
    (hcomp : asyncRootCompatible l (Nat.log2 (cx.P.cap / l.pendC)) = true) :
    ∃ l' ls', (s.binRound cx l.pendC l).stack = l' :: ls' ∧
      RoundOK cx l' (L1 ++ (pendLeaves cx l).take (cx.P.cap / l.pendC)) L ∧
      l'.incremental = true ∧ l'.pendC = l.pendC ∧ l'.progressive = l.progressive ∧
      l'.pend.length = l.pend.length - cx.P.cap / l.pendC ∧
      TopOK (s.binRound cx l.pendC l) ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
      (s.binRound cx l.pendC l).base.map Cell.erase = s.base.map Cell.erase ∧
      (s.binRound cx l.pendC l).hashErr = s.hashErr ∧
      (s.binRound cx l.pendC l).below.length = s.below.length ∧
      HolesSub s.below (s.binRound cx l.pendC l).below := by
  -- the block's shape
  have hp0 : l.prog = [] := hs.bin.prog
  have hnd : Nat.log2 (cx.P.cap / l.pendC) = cx.P.capDepth - k := by
    rw [hk, cap_div cx hcap (by omega), Nat.log2_two_pow]
  have hbe : cx.P.cap / l.pendC = 2 ^ (cx.P.capDepth - k) := by rw [hk, cap_div cx hcap (by omega)]
  rw [hnd] at hcomp
  generalize hndv : cx.P.capDepth - k = nd at hnd hbe hcomp
  have hnd0 : 0 < nd := by omega
  obtain ⟨hcol, hdeep⟩ := compat_spec hcomp
  generalize hB : l.pend.take (cx.P.cap / l.pendC) = B
  have hBlen : B.length = 2 ^ nd := by rw [← hB, List.length_take, hbe]; omega
  have hBw : ∀ e ∈ B, e.length = 2 ^ k := by
    intro e he; rw [← hB] at he; rw [← hk]; exact hs.width e (List.mem_of_mem_take he)
  have hBr : ∀ c ∈ B.flatten, c.isReady = true := by
    intro c hc; rw [← hB] at hc
    obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc
    exact hs.ready c (List.mem_append_left _ (List.mem_flatten.mpr ⟨e, List.mem_of_mem_take he, hce⟩))
  have hinlen : B.flatten.length = cx.P.cap := by
    rw [length_flatten_uniform B hBw, hBlen, hcap, ← Nat.pow_add]; congr 1; omega
  -- the job's result: the root over the block's element roots
  generalize hX : B.map (fun e => reduceElem cx.H cx.z0 (e.map Cell.val)) = X
  have hXlen : X.length = 2 ^ nd := by rw [← hX, List.length_map, hBlen]
  have hout : reduceTo cx.H 1 B.flatten.length (B.flatten.map Cell.val) = [mroot cx.H cx.z0 nd X] := by
    rw [map_vals_flatten, reduceTo_flatten cx.H cx.z0 k nd (B.map (·.map Cell.val)) (by
        intro e he; obtain ⟨e', he', rfl⟩ := List.mem_map.mp he; simp [hBw e' he'])
      (by simp [hBlen]) _ (by rw [hinlen, hcap]; have := Nat.lt_two_pow_self (n := cx.P.capDepth); omega)]
    congr 2
    rw [← hX, List.map_map]
    apply List.map_congr_left
    intro e he
    simp only [Function.comp_apply]
    exact (reduceElem_eq cx.H cx.z0 k _ (by simp [hBw e he])).symm
  generalize hvd : mroot cx.H cx.z0 nd X = v at hout
  -- the ring drain and the job
  have hact : s.activeStart = s.below.length := by
    unfold St.activeStart; rw [h]; simp only; rw [hp0, St.topStart_eq s l ls h]; rfl
  obtain ⟨hv1, ht1, hn1, hh1, hlen1⟩ := ringDrain_topOK cx.P s ht
  obtain ⟨l1, ls1, hs1, hle, hls1⟩ := LayerLE.of_view hv1 h (ringDrain_holesSub cx.P s ht.holes)
  have hbl1 := below_length_of_view hv1 h hs1
  have hr1 : RoundOK cx l1 L1 L := hle.roundOK cx hs
  have hp1 : l1.prog = [] := hle.prog_nil hp0
  let J : Job C := ⟨(s.ringDrain cx.P).nextJob, s.below.length + l.nodes.length, [v]⟩
  let T : St C := (s.ringDrain cx.P).addJob J
  have hen : s.enqueue cx B.flatten 1 (s.activeStart + (nodeCells l.nodes).length) =
      (T, [Cell.hole v (s.ringDrain cx.P).nextJob 0]) := by
    rw [enqueue_eq cx s _ 1 _ hBr, hout, hact, nodeCells_length]; rfl
  have hbr : s.binRound cx l.pendC l =
      T.setTop (binRoundLayer cx.P l.pendC (T.topLayer l) [Cell.hole v (s.ringDrain cx.P).nextJob 0]) := by
    unfold St.binRound; rw [hB, hen]
  rw [hbr]
  have hTs : T.stack = l1 :: ls1 := hs1
  have hTtop : T.topLayer l = l1 := by simp [St.topLayer, hTs]
  rw [hTtop]
  -- the new scope
  have hn1len : l1.nodes.length = l.nodes.length := hle.nodes_length
  have hcol1 : l1.collapsed = false → l1.nodes = [] := by
    intro hc; rw [hle.flag_collapsed] at hc
    have := hcol hc; rw [this] at hn1len; exact List.eq_nil_of_length_eq_zero hn1len
  have hif : (if l1.collapsed then l1.nodes else []) = l1.nodes := by
    cases hc : l1.collapsed
    · simp only [Bool.false_eq_true, if_false]; exact (hcol1 hc).symm
    · rfl
  let l' := binRoundLayer cx.P l.pendC l1 [Cell.hole v (s.ringDrain cx.P).nextJob 0]
  have hl'n : l'.nodes = l1.nodes ++ [⟨nd, .hole v (s.ringDrain cx.P).nextJob 0⟩] := by
    show (if l1.collapsed then l1.nodes else []) ++ _ = _; rw [hif, hnd]; rfl
  have hl'p : l'.pend = l1.pend.drop (cx.P.cap / l.pendC) := rfl
  have hl't : l'.tail = l1.tail := rfl
  have hp'0 : l'.prog = [] := hp1
  have hpend1 : l1.pend.length = l.pend.length := by
    have := congrArg List.length hle.pend_map; simpa using this
  -- the leaves
  have hpl1 := hle.pendLeaves cx
  have hTake : (pendLeaves cx l).take (cx.P.cap / l.pendC) = X := by
    unfold pendLeaves
    rw [List.take_append_of_le_length (by simp; omega), ← List.map_take, hB, hX]
  have hdrop : pendLeaves cx l' = (pendLeaves cx l1).drop (cx.P.cap / l.pendC) := by
    unfold pendLeaves
    rw [List.drop_append_of_le_length (by simp; omega), hl'p, hl't, List.map_drop]
  have hsplit' : L = (L1 ++ X) ++ pendLeaves cx l' := by
    rw [hr1.split, hdrop, ← hTake, hpl1, List.append_assoc, List.take_append_drop]
  have hdepth1 : ∀ n ∈ l1.nodes, nd ≤ n.depth := by
    intro n hn
    have : n.erase ∈ l.nodes.map Node.erase := by rw [← hle.nodes_map]; exact List.mem_map_of_mem hn
    obtain ⟨m, hm, he'⟩ := List.mem_map.mp this
    have := congrArg Node.depth he'
    rw [show n.depth = m.depth from this.symm]; exact hdeep m hm
  have hcov' : Cov cx.H cx.z0 true l'.nodes (L1 ++ X) := by
    rw [hl'n]
    exact cov_append_node cx.H cx.z0 hXlen (show (Cell.hole v (s.ringDrain cx.P).nextJob 0).val = _ from hvd.symm)
      hr1.bin.cov hdepth1
  have hLX : (L1 ++ X).length < 2 ^ 40 := by
    have := congrArg List.length hsplit'; simp only [List.length_append] at this ⊢; omega
  have hinc1 : l1.incremental = true := hle.flag_incremental.trans hinc
  have hbin' : BinOK cx l'.settled (L1 ++ X) := by
    refine ⟨fun hi => absurd ((show l1.incremental = false from hi).symm.trans hinc1) (by decide),
      hr1.bin.noprog, hr1.bin.nolegacy, hp1, rfl, rfl, hcov', (fun hc => Bool.noConfusion hc),
      cov_depth_lt cx.H cx.z0 hcov' hLX, ?_, fun _ => by show l'.nodes ≠ []; rw [hl'n]; simp⟩
    intro n hn hr
    have hn' : n ∈ l'.nodes := hn
    rw [hl'n] at hn'
    rcases List.mem_append.mp hn' with hn' | hn'
    · exact hr1.bin.holes n hn' hr
    · simp at hn'; rw [hn']; exact hnd0
  have hrd' : ∀ c ∈ l'.pend.flatten ++ l'.tail, c.isReady = true := by
    intro c hc
    rw [hl'p, hl't] at hc
    apply hr1.ready
    rcases List.mem_append.mp hc with hc | hc
    · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hc
      exact List.mem_append_left _ (List.mem_flatten.mpr ⟨e, List.mem_of_mem_drop he, hce⟩)
    · exact List.mem_append_right _ hc
  have hround' : RoundOK cx l' (L1 ++ (pendLeaves cx l).take (cx.P.cap / l.pendC)) L := by
    rw [hTake]
    exact ⟨hsplit', hbin', hrd', fun e he => by rw [hl'p] at he; exact hr1.width e (List.mem_of_mem_drop he)⟩
  -- the cells
  have hc1 : l1.cells = nodeCells l1.nodes ++ (l1.pend.flatten ++ l1.tail) := cells_split l1 hp1
  have hc' : l'.cells = nodeCells l1.nodes ++ ([Cell.hole v (s.ringDrain cx.P).nextJob 0] ++
      (l'.pend.flatten ++ l'.tail)) := by
    rw [cells_split l' hp'0, hl'n, nodeCells_append, List.append_assoc]; rfl
  have hk0 : (nodeCells l1.nodes).length = l1.nodes.length := nodeCells_length _
  refine ⟨l', ls1, St.setTop_stack T l1 l' ls1 hTs, hround', hinc1, hle.pendC, hle.flag_progressive,
    by rw [hl'p, List.length_drop, hpend1], ⟨?_, ?_, ?_, ?_⟩, hls1, ?_, ?_, ?_, ?_⟩
  · -- the hole invariant, with the new job
    show HolesOK (St.setTop ((s.ringDrain cx.P).addJob J) l')
    unfold St.addJob
    apply holesOK_addJob _ l1 l' ls1 hs1 ht1.holes J (fun j' hj' => Nat.ne_of_lt (ht1.ids j' hj'))
      l1.nodes.length (by rw [hc1]; simp [hk0])
    · rw [hc1, hc', List.take_left' hk0, List.take_left' hk0]
    · show s.below.length + l.nodes.length = _; rw [hbl1, hn1len]
    · intro i hi
      have hi0 : i = 0 := by simp [J] at hi; omega
      subst hi0
      refine ⟨0, ?_⟩
      rw [hc', Nat.add_zero, List.getElem?_append_right (by omega), hk0, Nat.sub_self]
      rfl
    · rw [hc']; simp [J, hk0]
    · intro i hi hi'
      have hm : l'.cells[i]'hi' ∈ l'.pend.flatten ++ l'.tail := by
        have hlen' : (nodeCells l1.nodes ++ [Cell.hole v (s.ringDrain cx.P).nextJob 0]).length = l1.nodes.length + 1 := by
          simp [hk0]
        have hc'' : l'.cells = (nodeCells l1.nodes ++ [Cell.hole v (s.ringDrain cx.P).nextJob 0]) ++
            (l'.pend.flatten ++ l'.tail) := by rw [hc', List.append_assoc]
        have := List.getElem_mem hi'
        simp only [hc''] at this ⊢
        rw [List.getElem_append_right (by rw [hlen']; simp [J] at hi; omega)]
        exact List.getElem_mem _
      exact hrd' _ hm
    · have := jobs_end_of_ready (s.ringDrain cx.P) ht1 ((s.ringDrain cx.P).below ++ nodeCells l1.nodes)
        (l1.pend.flatten ++ l1.tail) (by rw [St.cells_top _ l1 ls1 hs1, hc1, List.append_assoc]) hr1.ready
      intro j hj; have := this j hj; simp only [List.length_append, hk0] at this; exact this
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
  · show (T.setTop l').base.map Cell.erase = s.base.map Cell.erase
    rw [St.setTop_base]
    exact congrArg Prod.fst hv1
  · rw [St.setTop_hashErr]; exact hh1
  · rw [St.setTop_below T l1 l' ls1 hTs]; exact hbl1
  · rw [St.setTop_below T l1 l' ls1 hTs]
    show HolesSub s.below (s.ringDrain cx.P).below
    exact holesSub_below hv1 h hs1 (ringDrain_holesSub cx.P s ht.holes)

/-! ## The rounds and the whole flush -/

theorem flushAsyncLoop_bin (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (dflt : Layer C) (k : Nat)
    (hk0 : k < cx.P.capDepth) (L : List C) (hL : L.length < 2 ^ 40) :
    ∀ fuel (s : St C) (l : Layer C) (ls : List (Layer C)) (L1 : List C) (e : Bool),
      s.stack = l :: ls → RoundOK cx l L1 L → l.incremental = true → l.progressive = false → l.pendC = 2 ^ k →
      TopOK s →
      ∃ l' ls' L1', (flushAsyncLoop cx dflt (2 ^ k) fuel s e).1.stack = l' :: ls' ∧ RoundOK cx l' L1' L ∧
        l'.incremental = true ∧ l'.progressive = false ∧ l'.pendC = 2 ^ k ∧
        TopOK (flushAsyncLoop cx dflt (2 ^ k) fuel s e).1 ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
        (flushAsyncLoop cx dflt (2 ^ k) fuel s e).1.base.map Cell.erase = s.base.map Cell.erase ∧
        (flushAsyncLoop cx dflt (2 ^ k) fuel s e).1.hashErr = s.hashErr ∧
        (flushAsyncLoop cx dflt (2 ^ k) fuel s e).1.below.length = s.below.length ∧
        HolesSub s.below (flushAsyncLoop cx dflt (2 ^ k) fuel s e).1.below := by
  intro fuel
  induction fuel with
  | zero =>
    intro s l ls L1 e h hr hi hp hc ht
    exact ⟨l, ls, L1, h, hr, hi, hp, hc, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩
  | succ fuel ih =>
    intro s l ls L1 e h hr hi hp hc ht
    have htop : s.topLayer dflt = l := by simp [St.topLayer, h]
    simp only [flushAsyncLoop, htop, hp, Bool.false_eq_true, if_false]
    split
    · exact ⟨l, ls, L1, h, hr, hi, hp, hc, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩
    split
    · exact ⟨l, ls, L1, h, hr, hi, hp, hc, ht, rfl, rfl, rfl, rfl, HolesSub.refl _⟩
    rename_i hpl hcomp
    have hcomp' : asyncRootCompatible l (Nat.log2 (cx.P.cap / l.pendC)) = true := by
      rw [hc]; simpa using hcomp
    obtain ⟨l', ls', hs', hr', hi', hc', hp', _, ht', hle', hba', hh', hbl', hsub'⟩ :=
      binRound_bin cx hcap s l ls L1 L k h hr hi ht hL hc hk0 (by rw [hc]; omega) hcomp'
    rw [hc] at hs' ht' hba' hh' hbl' hsub'
    obtain ⟨l'', ls'', L1'', hs'', hr'', hi'', hp'', hc'', ht'', hle'', hba'', hh'', hbl'', hsub''⟩ :=
      ih _ l' ls' _ true hs' hr' hi' (by rw [hp']; exact hp) (by rw [hc', hc]) ht'
    exact ⟨l'', ls'', L1'', hs'', hr'', hi'', hp'', hc'', ht'', by rw [hle'', hle'], by rw [hba'', hba'],
      by rw [hh'', hh'], by rw [hbl'', hbl'], HolesSub.trans hsub' hsub''⟩

/-- The last step of `flushPendingAsync`: an emptied run lets the tail move
    down as leaves; the scope is then `ScopeOK`. -/
theorem flushFinish_scope (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (L1 L : List C) (k : Nat)
    (h : s.stack = l :: ls) (hr : RoundOK cx l L1 L) (ht : TopOK s) (hk : l.pendC = 2 ^ k) (hk0 : 0 < k)
    (hkb : 2 ^ k ≤ cx.P.batch) :
    (s.setTop (flushFinish l)).stack = flushFinish l :: ls ∧ ScopeOK cx (flushFinish l) L ∧
      (flushFinish l).incremental = l.incremental ∧ (flushFinish l).progressive = l.progressive ∧
      TopOK (s.setTop (flushFinish l)) ∧ (s.setTop (flushFinish l)).below = s.below ∧
      (s.setTop (flushFinish l)).base = s.base ∧ (s.setTop (flushFinish l)).hashErr = s.hashErr := by
  have hp0 : l.prog = [] := hr.bin.prog
  refine ⟨St.setTop_stack s l _ ls h, ?_, by unfold flushFinish; split <;> rfl, by unfold flushFinish; split <;> rfl,
    topOK_setTop_same s l _ ls h ht ?_, St.setTop_below s l _ ls h, St.setTop_base s _, St.setTop_hashErr s _⟩
  · unfold flushFinish
    split
    · rename_i hpe
      have htr : ∀ c ∈ l.tail, c.isReady = true := fun c hc => hr.ready c (List.mem_append_right _ hc)
      refine ⟨L1 ++ l.tail.map Cell.val, ?_, ?_, ?_⟩
      · rw [hr.split]; simp [pendLeaves, hpe]
      · have hcov := cov_append_leaves cx.H cx.z0 l.tail hr.bin.cov
        refine ⟨hr.bin.ninc, hr.bin.noprog, hr.bin.nolegacy, hp0, rfl, rfl, hcov, ?_, ?_, ?_, ?_⟩
        · intro hc n hn
          rcases List.mem_append.mp hn with hn | hn
          · exact hr.bin.leaves hc n hn
          · obtain ⟨c, _, rfl⟩ := List.mem_map.mp hn; rfl
        · intro n hn
          rcases List.mem_append.mp hn with hn | hn
          · exact hr.bin.shallow n hn
          · obtain ⟨c, _, rfl⟩ := List.mem_map.mp hn; exact (by decide : (0 : Nat) < 40)
        · intro n hn hnr
          rcases List.mem_append.mp hn with hn | hn
          · exact hr.bin.holes n hn hnr
          · obtain ⟨c, hc, rfl⟩ := List.mem_map.mp hn; rw [htr c hc] at hnr; cases hnr
        · intro hc e
          exact hr.bin.nonempty hc (List.append_eq_nil_iff.mp e).1
      · exact ⟨fun _ => rfl, by simp [hpe], by simp, by simp [hpe], fun h => absurd hpe h⟩
    · rename_i hpe
      refine ⟨L1, hr.split, hr.bin, ⟨fun e => absurd e hpe, ?_, ?_, hr.width, fun _ => ⟨k, hk, hk0, hkb⟩⟩⟩
      · intro e he c hc; exact hr.ready c (List.mem_append_left _ (List.mem_flatten.mpr ⟨e, he, hc⟩))
      · intro c hc; exact hr.ready c (List.mem_append_right _ hc)
  · unfold flushFinish
    split
    · rename_i hpe
      simp [Layer.cells, hpe, nodeCells, List.map_map, Function.comp_def, List.append_assoc]
    · rfl

/-- **`flushPendingAsync` on a binary scope** keeps its invariant, whatever
    number of cap-wide blocks it turns into jobs. -/
theorem flushAsync_scope (cx : Ctx C) (hcap : cx.P.cap = 2 ^ cx.P.capDepth) (hlt : cx.P.batch < cx.P.cap)
    (s : St C) (l : Layer C) (ls : List (Layer C)) (L : List C)
    (h : s.stack = l :: ls) (hs : ScopeOK cx l L) (hinc : l.incremental = true) (hpne : l.pend ≠ [])
    (ht : TopOK s) (hL : L.length < 2 ^ 40) :
    ∃ l' ls', (s.flushAsync cx).1.stack = l' :: ls' ∧ ScopeOK cx l' L ∧ l'.incremental = true ∧
      l'.progressive = l.progressive ∧ TopOK (s.flushAsync cx).1 ∧ ls'.map Layer.erase = ls.map Layer.erase ∧
      (s.flushAsync cx).1.base.map Cell.erase = s.base.map Cell.erase ∧
      (s.flushAsync cx).1.hashErr = s.hashErr ∧ (s.flushAsync cx).1.below.length = s.below.length ∧
      HolesSub s.below (s.flushAsync cx).1.below := by
  have hs0 := hs
  obtain ⟨_, _, hb0, hpo0⟩ := hs0
  obtain ⟨k, hk, hk0, hkb⟩ := hpo0.pow hpne
  have hkc : k < cx.P.capDepth := by
    have : 2 ^ k < 2 ^ cx.P.capDepth := by rw [← hcap]; omega
    exact (Nat.pow_lt_pow_iff_right (by decide)).mp this
  obtain ⟨L1, hr⟩ := roundOK_of_scope cx hs
  have hnp : l.progressive = false := hb0.noprog
  unfold St.flushAsync
  rw [h]
  simp only
  rw [hk]
  obtain ⟨l', ls', L1', hs', hr', hi', hp', hc', ht', hle', hba', hh', hbl', hsub'⟩ :=
    flushAsyncLoop_bin cx hcap l k hkc L hL (l.pend.length + 1) s l ls L1 false h hr hinc hnp hk ht
  have htop : (flushAsyncLoop cx l (2 ^ k) (l.pend.length + 1) s false).1.topLayer l = l' := by
    simp [St.topLayer, hs']
  rw [htop]
  obtain ⟨hs2, hsc2, hi2, hp2, ht2, hb2, hba2, hh2⟩ :=
    flushFinish_scope cx _ l' ls' L1' L k hs' hr' ht' hc' hk0 hkb
  exact ⟨_, ls', hs2, hsc2, by rw [hi2, hi'], by rw [hp2, hp', hnp], ht2, hle', by rw [hba2, hba'],
    by rw [hh2, hh'], by rw [hb2, hbl'], by rw [hb2]; exact hsub'⟩

end Ssz.Hasher