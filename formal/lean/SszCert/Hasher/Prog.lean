import SszCert.Hasher.Sim

/-!
# Progressive scopes

`StartTree(TreeTypeProgressive)`: the scope's chunks form the groups of
EIP-7916 (1, 4, 16, … chunks); `Collapse` turns every complete group into a
progressive root (`maybeCollapseProgressive`: Go's depth-greedy group count,
a background job for a large plain group, or `collapseAllDepths`), and the
close folds the roots with the partial last group
(`collapseProgressiveLayer`).
-/

namespace Ssz.Hasher

variable {C : Type}

/-! ## Go's depth-greedy group count -/

theorem takeFit_eq (base d : Nat) : ∀ f c k, c ≤ base →
    takeFit base d f c k = (c + min f ((base - c) / 2 ^ d) * 2 ^ d, k + min f ((base - c) / 2 ^ d)) := by
  intro f
  induction f with
  | zero => intro c k _; simp [takeFit]
  | succ f ih =>
    intro c k hc
    have hp : 0 < 2 ^ d := Nat.two_pow_pos d
    simp only [takeFit]
    split
    · rename_i hf
      rw [ih (c + 2 ^ d) (k + 1) hf]
      have hq : (base - c) / 2 ^ d = (base - (c + 2 ^ d)) / 2 ^ d + 1 := by
        rw [show base - c = (base - (c + 2 ^ d)) + 2 ^ d by omega, Nat.add_div_right _ hp]
      rw [hq, Nat.succ_min_succ]
      simp only [Prod.mk.injEq]
      constructor
      · rw [Nat.succ_mul]; omega
      · omega
    · rename_i hf
      have hq : (base - c) / 2 ^ d = 0 := Nat.div_eq_of_lt (by omega)
      simp [hq]

theorem dvd_leavesOf {d : Nat} : ∀ ns : List (Node C), (∀ n ∈ ns, d ≤ n.depth) → 2 ^ d ∣ leavesOf ns
  | [], _ => by simp [leavesOf]
  | n :: ns, h => by
    have h1 : 2 ^ d ∣ 2 ^ n.depth := Nat.pow_dvd_pow 2 (h n (by simp))
    have h2 := dvd_leavesOf ns (fun m hm => h m (List.mem_cons_of_mem _ hm))
    simp only [leavesOf, List.map_cons, List.sum_cons] at h2 ⊢
    exact Nat.dvd_add h1 h2

theorem leavesOf_append (a b : List (Node C)) : leavesOf (a ++ b) = leavesOf a + leavesOf b := by
  simp [leavesOf]

theorem leavesOf_uniform' {d : Nat} : ∀ ns : List (Node C), (∀ n ∈ ns, n.depth = d) → leavesOf ns = ns.length * 2 ^ d
  | [], _ => by simp [leavesOf]
  | n :: ns, h => by
    have := leavesOf_uniform' ns (fun m hm => h m (List.mem_cons_of_mem _ hm))
    simp only [leavesOf, List.map_cons, List.sum_cons, List.length_cons] at this ⊢
    rw [this, h n (by simp), Nat.succ_mul]; omega

/-- A deepest-first list splits by depth around any `d`. -/
theorem nonInc_split3 (d : Nat) : ∀ ns : List (Node C), NonInc ns →
    ns = ns.filter (·.depth > d) ++ ns.filter (·.depth == d) ++ ns.filter (·.depth < d)
  | [], _ => by simp
  | n :: ns, h => by
    have ih := nonInc_split3 d ns (nonInc_tail h)
    have hle := nonInc_le h
    rcases Nat.lt_trichotomy n.depth d with hlt | heq | hgt
    · have hA : ns.filter (·.depth > d) = [] := List.filter_eq_nil_iff.mpr (fun m hm => by
        have := hle m hm; simp; omega)
      have hM : ns.filter (·.depth == d) = [] := List.filter_eq_nil_iff.mpr (fun m hm => by
        have := hle m hm; simp; omega)
      have hB : ns.filter (·.depth < d) = ns := List.filter_eq_self.mpr (fun m hm => by
        have := hle m hm; simp; omega)
      simp [List.filter_cons, hlt, Nat.ne_of_lt hlt, Nat.not_lt.mpr (Nat.le_of_lt hlt), hA, hM, hB]
    · have hA : ns.filter (·.depth > d) = [] := List.filter_eq_nil_iff.mpr (fun m hm => by
        have := hle m hm; simp; omega)
      rw [List.filter_cons, List.filter_cons, List.filter_cons]
      simp only [heq, gt_iff_lt, Nat.lt_irrefl, decide_false, Bool.false_eq_true, if_false, beq_self_eq_true,
        if_true, List.nil_append, hA]
      rw [hA, List.nil_append] at ih
      simpa using congrArg (n :: ·) ih
    · rw [List.filter_cons, List.filter_cons, List.filter_cons]
      simp only [gt_iff_lt, hgt, decide_true, if_true, show ¬ n.depth = d by omega, show ¬ n.depth < d by omega,
        beq_iff_eq, decide_false, Bool.false_eq_true, if_false, List.cons_append]
      exact congrArg (n :: ·) ih

theorem foldl_max_spec : ∀ (ns : List (Node C)) (m : Nat),
    m ≤ ns.foldl (fun m n => max m n.depth) m ∧ ∀ n ∈ ns, n.depth ≤ ns.foldl (fun m n => max m n.depth) m
  | [], m => ⟨Nat.le_refl _, by simp⟩
  | n :: ns, m => by
    obtain ⟨h1, h2⟩ := foldl_max_spec ns (max m n.depth)
    simp only [List.foldl_cons]
    refine ⟨Nat.le_trans (Nat.le_max_left _ _) h1, ?_⟩
    intro k hk
    rcases List.mem_cons.mp hk with rfl | hk
    · exact Nat.le_trans (Nat.le_max_right _ _) h1
    · exact h2 k hk

theorem foldl_max_le {D : Nat} : ∀ (ns : List (Node C)) (m : Nat), m ≤ D → (∀ n ∈ ns, n.depth ≤ D) →
    ns.foldl (fun m n => max m n.depth) m ≤ D
  | [], m, hm, _ => hm
  | n :: ns, m, hm, h => by
    simp only [List.foldl_cons]
    exact foldl_max_le ns _ (Nat.max_le.mpr ⟨hm, h n (by simp)⟩) (fun k hk => h k (List.mem_cons_of_mem _ hk))

theorem filter_gt_pred (ns : List (Node C)) (hni : NonInc ns) (d : Nat) :
    ns.filter (·.depth > d) = ns.filter (·.depth > d + 1) ++ ns.filter (·.depth == d + 1) := by
  have h := nonInc_split3 (d + 1) ns hni
  have hA : ∀ n ∈ ns.filter (·.depth > d + 1), n.depth > d + 1 := fun n hn => by simpa using (List.mem_filter.mp hn).2
  have hM : ∀ n ∈ ns.filter (·.depth == d + 1), n.depth = d + 1 := fun n hn => by simpa using (List.mem_filter.mp hn).2
  have hB : ∀ n ∈ ns.filter (·.depth < d + 1), n.depth < d + 1 := fun n hn => by simpa using (List.mem_filter.mp hn).2
  generalize ns.filter (·.depth > d + 1) = A at h hA ⊢
  generalize ns.filter (·.depth == d + 1) = M at h hM ⊢
  generalize ns.filter (·.depth < d + 1) = B at h hB
  rw [h, List.filter_append, List.filter_append,
    List.filter_eq_self.mpr (fun n hn => by have := hA n hn; simp; omega),
    List.filter_eq_self.mpr (fun n hn => by have := hM n hn; simp; omega),
    List.filter_eq_nil_iff.mpr (fun n hn => by have := hB n hn; simp; omega), List.append_nil]

/-- One depth of Go's loop: the depth's nodes are taken while they fit;
    either the group is complete, or every node of that depth was taken. -/
theorem byDepth_step (ns : List (Node C)) (D d : Nat) (hni : NonInc ns) (hd : d ≤ D) (consumed k : Nat)
    (hc : consumed = leavesOf (ns.filter (·.depth > d))) (hk : k = (ns.filter (·.depth > d)).length)
    (hlt : consumed < 2 ^ D) :
    (∃ q, 0 < k + q ∧ byDepth ns (2 ^ D) (d + 1) consumed k = (2 ^ D, k + q) ∧
      leavesOf (ns.take (k + q)) = 2 ^ D) ∨
    (consumed + (ns.filter (·.depth == d)).length * 2 ^ d < 2 ^ D ∧
      byDepth ns (2 ^ D) (d + 1) consumed k = byDepth ns (2 ^ D) d (consumed + (ns.filter (·.depth == d)).length * 2 ^ d)
        (k + (ns.filter (·.depth == d)).length)) := by
  have hsplit := nonInc_split3 d ns hni
  have hMd : ∀ n ∈ ns.filter (·.depth == d), n.depth = d := fun n hn => by simpa using (List.mem_filter.mp hn).2
  have hdv1 : 2 ^ d ∣ consumed := by
    rw [hc]; exact dvd_leavesOf _ (fun n hn => by have := (List.mem_filter.mp hn).2; simp at this; omega)
  have hdv2 : 2 ^ d ∣ 2 ^ D := Nat.pow_dvd_pow 2 hd
  have hp : 0 < 2 ^ d := Nat.two_pow_pos d
  have hq : (2 ^ D - consumed) / 2 ^ d * 2 ^ d = 2 ^ D - consumed := Nat.div_mul_cancel (Nat.dvd_sub hdv2 hdv1)
  generalize hqq : (2 ^ D - consumed) / 2 ^ d = q at hq
  have hq1 : 0 < q := by
    rcases Nat.eq_zero_or_pos q with h0 | h0
    · rw [h0] at hq; omega
    · exact h0
  have htf := takeFit_eq (2 ^ D) d (ns.filter (·.depth == d)).length consumed k (by omega)
  rw [hqq] at htf
  simp only [byDepth]
  rw [htf]
  by_cases hm : q ≤ (ns.filter (·.depth == d)).length
  · left
    rw [Nat.min_eq_right hm]
    refine ⟨q, by omega, ?_, ?_⟩
    · rw [if_pos (by omega)]; simp; omega
    · conv => lhs; rw [hsplit]
      rw [List.append_assoc, hk, List.take_length_add_append, List.take_append_of_le_length hm, leavesOf_append,
        leavesOf_uniform' (d := d) _ (fun n hn => hMd n (List.mem_of_mem_take hn))]
      simp only [List.length_take, Nat.min_eq_left hm]
      rw [← hc]; omega
  · right
    rw [Nat.min_eq_left (by omega)]
    have hlt2 : consumed + (ns.filter (·.depth == d)).length * 2 ^ d < 2 ^ D := by
      have : (ns.filter (·.depth == d)).length * 2 ^ d < q * 2 ^ d := Nat.mul_lt_mul_of_pos_right (by omega) hp
      omega
    exact ⟨hlt2, by rw [if_neg (by omega)]⟩

/-- The invariant of Go's depth loop. -/
theorem byDepth_spec (ns : List (Node C)) (D : Nat) (hni : NonInc ns) (hD : ∀ n ∈ ns, n.depth ≤ D)
    (hL : 2 ^ D ≤ leavesOf ns) : ∀ d, d ≤ D → ∀ consumed k, consumed = leavesOf (ns.filter (·.depth > d)) →
      k = (ns.filter (·.depth > d)).length → consumed < 2 ^ D →
      0 < (byDepth ns (2 ^ D) (d + 1) consumed k).2 ∧
      leavesOf (ns.take (byDepth ns (2 ^ D) (d + 1) consumed k).2) = 2 ^ D := by
  intro d
  induction d with
  | zero =>
    intro _ consumed k hc hk hlt
    rcases byDepth_step ns D 0 hni (Nat.zero_le _) consumed k hc hk hlt with ⟨q, hq, he, hl⟩ | ⟨hlt2, _⟩
    · rw [he]; exact ⟨hq, hl⟩
    · exfalso
      have hsplit := nonInc_split3 0 ns hni
      have hB : ns.filter (·.depth < 0) = [] := List.filter_eq_nil_iff.mpr (fun n _ => by simp)
      have : leavesOf ns = consumed + (ns.filter (·.depth == 0)).length * 2 ^ 0 := by
        conv => lhs; rw [hsplit]
        rw [hB, List.append_nil, leavesOf_append, leavesOf_uniform' (d := 0) (ns.filter (·.depth == 0)) (fun n hn => by
          simpa using (List.mem_filter.mp hn).2), ← hc]
      omega
  | succ d ih =>
    intro hd consumed k hc hk hlt
    rcases byDepth_step ns D (d + 1) hni hd consumed k hc hk hlt with ⟨q, hq, he, hl⟩ | ⟨hlt2, he⟩
    · rw [he]; exact ⟨hq, hl⟩
    · rw [he]
      apply ih (by omega)
      · rw [filter_gt_pred ns hni d, leavesOf_append, hc,
          leavesOf_uniform' (d := d + 1) (ns.filter (·.depth == d + 1)) (fun n hn => by simpa using (List.mem_filter.mp hn).2)]
      · rw [filter_gt_pred ns hni d, List.length_append, hk]
      · exact hlt2

/-- **Go's depth-greedy count takes exactly one group**, as a prefix of the
    nodes, whenever the nodes are deepest-first, no node exceeds the group
    and the nodes cover at least the group. -/
theorem consumeCount_spec (ns : List (Node C)) (D : Nat) (hni : NonInc ns) (hD : ∀ n ∈ ns, n.depth ≤ D)
    (hL : 2 ^ D ≤ leavesOf ns) :
    0 < consumeCount ns (2 ^ D) ∧ leavesOf (ns.take (consumeCount ns (2 ^ D))) = 2 ^ D := by
  have hmax := foldl_max_spec ns 0
  have hmd : maxDepthOf ns ≤ D := foldl_max_le ns 0 (Nat.zero_le _) hD
  have hA : ns.filter (·.depth > maxDepthOf ns) = [] := List.filter_eq_nil_iff.mpr (fun n hn => by
    have := hmax.2 n hn; simp [maxDepthOf] at this ⊢; omega)
  exact byDepth_spec ns D hni hD hL (maxDepthOf ns) hmd 0 0 (by rw [hA]; rfl) (by rw [hA]; rfl)
    (Nat.two_pow_pos D)

/-! ## A complete group reduces without padding -/

theorem collapseLoop_exact (cx : Ctx C) (live : Bool) (D : Nat) :
    ∀ fuel (s : St C) (ns : List (Node C)), NonInc ns → ns ≠ [] → (∀ n ∈ ns, n.cell.isReady = true) →
      leavesOf ns = 2 ^ D → (∀ n ∈ ns, n.depth ≤ D) →
      collapseMeasure ns ≤ fuel → (collapseLoop cx live fuel s ns).1 = s ∧
        ∀ n ∈ (collapseLoop cx live fuel s ns).2, n.cell.isReady = true := by
  intro fuel
  induction fuel with
  | zero =>
    intro s ns _ hne _ _ _ hm
    have : ns.length = 0 := by unfold collapseMeasure at hm; omega
    exact absurd (List.eq_nil_of_length_eq_zero this) hne
  | succ fuel ih =>
    intro s ns hni hne hr hex hdD hm
    obtain ⟨keep, grp, low, rfl, hg, hgd, hkd⟩ := nonInc_split hni hne
    have hm0 : collapseMeasure (keep ++ grp) ≤ fuel + 1 := hm
    simp only [collapseLoop, min_depth_split hg hgd hkd, count_low_split hgd hkd, all_low_split hgd hkd]
    by_cases hstop : grp.length = 1 ∧ keep.isEmpty = true
    · rw [if_pos hstop]; exact ⟨rfl, hr⟩
    rw [if_neg hstop]
    -- an exact power-of-two cover pairs without padding
    have hgpos : 0 < grp.length := List.length_pos_iff.mpr hg
    have hlex : leavesOf keep + grp.length * 2 ^ low = 2 ^ D := by
      rw [← hex, leavesOf_append, leavesOf_uniform' (d := low) grp hgd]
    have hlowD : low < D := by
      obtain ⟨g0, hg0⟩ := List.exists_mem_of_ne_nil grp hg
      have h1 := hdD g0 (List.mem_append_right _ hg0); rw [hgd g0 hg0] at h1
      rcases Nat.lt_or_ge low D with h2 | h2
      · exact h2
      · exfalso
        have hk : keep = [] := by
          cases keep with
          | nil => rfl
          | cons k0 ks => have := hkd k0 (by simp); have := hdD k0 (by simp); omega
        subst hk
        have hl : low = D := by omega
        subst hl
        simp [leavesOf] at hlex
        have : grp.length = 1 := by
          have hp := Nat.two_pow_pos low
          rcases Nat.lt_or_ge grp.length 2 with h3 | h3
          · omega
          · have : 2 * 2 ^ low ≤ grp.length * 2 ^ low := Nat.mul_le_mul_right _ h3
            omega
        exact hstop ⟨this, rfl⟩
    have heven : grp.length % 2 = 0 := by
      have hd1 : 2 ^ (low + 1) ∣ leavesOf keep := dvd_leavesOf keep (fun n hn => hkd n hn)
      have hd2 : 2 ^ (low + 1) ∣ 2 ^ D := Nat.pow_dvd_pow 2 hlowD
      have hd3 : 2 ^ (low + 1) ∣ grp.length * 2 ^ low := by
        have := Nat.dvd_sub hd2 hd1
        rwa [show 2 ^ D - leavesOf keep = grp.length * 2 ^ low by omega] at this
      rw [Nat.pow_succ, Nat.mul_comm] at hd3
      have := Nat.dvd_of_mul_dvd_mul_right (Nat.two_pow_pos low) hd3
      omega
    have hno : ¬ (grp.length % 2 = 1 ∧ live = true) := fun h => by omega
    let pad : List (Node C) := if grp.length % 2 = 1 then [⟨low, .ready (zeroH cx.H cx.z0 low)⟩] else []
    have hns2 : (if grp.length % 2 = 1 then keep ++ grp ++ [⟨low, .ready (zeroH cx.H cx.z0 low)⟩]
        else keep ++ grp) = keep ++ (grp ++ pad) := by simp only [pad]; split <;> simp
    have hcnt2 : (if grp.length % 2 = 1 then grp.length + 1 else grp.length) = (grp ++ pad).length := by
      simp only [pad]; split <;> simp
    simp only [hns2, hcnt2, hno, if_false]
    have hlen : (keep ++ (grp ++ pad)).length - (grp ++ pad).length = keep.length := by simp
    rw [hlen, List.take_left' rfl, List.drop_left' rfl]
    have hpadd : ∀ n ∈ pad, n.depth = low := by
      intro n hn; simp only [pad] at hn; split at hn
      · simp at hn; rw [hn]
      · simp at hn
    have hpd : ∀ n ∈ grp ++ pad, n.depth = low := by
      intro n hn; rcases List.mem_append.mp hn with hn | hn
      · exact hgd n hn
      · exact hpadd n hn
    have hall : (grp ++ pad).all (·.depth == low) = true := by
      rw [List.all_eq_true]; intro n hn; simp [hpd n hn]
    rw [if_pos hall]
    have hpg : grp ++ pad ≠ [] := by simp [hg]
    have hpr : ∀ n ∈ grp ++ pad, n.cell.isReady = true := by
      intro n hn; rcases List.mem_append.mp hn with hn | hn
      · exact hr n (List.mem_append_right _ hn)
      · simp only [pad] at hn; split at hn
        · simp at hn; rw [hn]; rfl
        · simp at hn
    have hst : (s.pairNodes cx (grp ++ pad)).1 = s := by
      simp only [St.pairNodes, St.vals]
      rw [if_pos (List.all_eq_true.mpr (fun c hc => by
        simp only [nodeCells, List.mem_map] at hc; obtain ⟨n, hn, rfl⟩ := hc; exact hpr n hn))]
    rw [hst, pairNodes_group cx _ hpg hpd]
    let paired : List (Node C) :=
      (pairUp cx.H ((grp ++ pad).map (·.cell.val))).map (fun v => ⟨low + 1, .ready v⟩)
    have hpdep : ∀ n ∈ paired, n.depth = low + 1 := by
      intro n hn; simp only [paired, List.mem_map] at hn; obtain ⟨_, _, rfl⟩ := hn; rfl
    have hgp : (grp ++ pad).length = if grp.length % 2 = 1 then grp.length + 1 else grp.length := by
      simp only [pad]; split <;> simp
    have hplen : paired.length = (grp.length + 1) / 2 := by
      simp only [paired, List.length_map, pairUp_length, hgp]
      split <;> omega
    have hpne : paired ≠ [] := by
      intro e; have h1 := congrArg List.length e; rw [hplen] at h1
      have h2 := List.length_pos_iff.mpr hg; simp only [List.length_nil] at h1; omega
    have hkni : NonInc keep := nonInc_append_left hni
    have hpad0 : pad = [] := by simp only [pad]; rw [if_neg (by omega)]
    apply ih _ _ (nonInc_of_split hkni hkd hpdep hpne) (fun e => hpne (List.append_eq_nil_iff.mp e).2)
    · intro n hn
      rcases List.mem_append.mp hn with hn | hn
      · exact hr n (List.mem_append_left _ hn)
      · simp only [paired, List.mem_map] at hn; obtain ⟨_, _, rfl⟩ := hn; rfl
    · rw [leavesOf_append, leavesOf_uniform' (d := low + 1) paired hpdep, hplen, ← hlex, Nat.pow_succ]
      have : (grp.length + 1) / 2 * 2 = grp.length := by omega
      have e : (grp.length + 1) / 2 * (2 ^ low * 2) = ((grp.length + 1) / 2 * 2) * 2 ^ low := by
        rw [Nat.mul_comm (2 ^ low) 2, ← Nat.mul_assoc]
      rw [e, this]
    · intro n hn
      rcases List.mem_append.mp hn with hn | hn
      · exact hdD n (List.mem_append_left _ hn)
      · rw [hpdep n hn]; omega
    · unfold collapseMeasure at hm0 ⊢
      rw [lastDepth_append hg, lastDepth_uniform hg hgd] at hm0
      rw [lastDepth_append hpne, lastDepth_uniform hpne hpdep]
      simp only [List.length_append, hplen] at hm0 ⊢
      by_cases hke : keep = []
      · subst hke
        rw [headDepth_nil_append, headDepth_uniform hg hgd] at hm0
        rw [headDepth_nil_append, headDepth_uniform hpne hpdep]
        have : 2 ≤ grp.length := by
          cases grp with
          | nil => exact absurd rfl hg
          | cons a r => cases r with
            | nil => simp at hstop
            | cons _ _ => simp
        simp only [List.length_nil] at hm0 ⊢
        omega
      · rw [headDepth_append hke] at hm0 ⊢
        have : low < headDepth keep := by
          obtain ⟨k0, ks, rfl⟩ := List.exists_cons_of_ne_nil hke
          exact hkd k0 (by simp)
        have : (grp.length + 1) / 2 ≤ grp.length := by omega
        omega


/-- **A complete group** (exactly `2^D` leaves, no node deeper) reduces to
    its bounded root without padding or expansion, even when more of the
    region follows it (`live`). -/
theorem collapseAll_exact (cx : Ctx C) (s : St C) (ns : List (Node C)) (L : List C) (D : Nat) (live : Bool)
    (hcov : Cov cx.H cx.z0 true ns L) (hne : ns ≠ []) (hr : ∀ n ∈ ns, n.cell.isReady = true)
    (hL : L.length = 2 ^ D) (hdD : ∀ n ∈ ns, n.depth ≤ D) (hD : D < 64) :
    (s.collapseAll cx ns (2 ^ D) live).1 = s ∧ (s.collapseAll cx ns (2 ^ D) live).2 = .ready (mroot cx.H cx.z0 D L) := by
  have hgood : ∀ m, ns = [m] → m.depth = depthOf L.length := by
    intro m hm; subst hm
    obtain ⟨_, hlen, _⟩ := hcov
    simp at hlen; rw [hlen, depthOf_two_pow]
  have hd : headDepth ns < 64 := by
    obtain ⟨n0, ns0, rfl⟩ := List.exists_cons_of_ne_nil hne
    have := hdD n0 (by simp); simp [headDepth]; omega
  have hmeas : collapseMeasure ns ≤ 2 * ns.length + 64 := by unfold collapseMeasure; omega
  have hlv : leavesOf ns = 2 ^ D := by rw [← cov_full_length cx.H cx.z0 hcov, hL]
  have hchunks : ¬ (2 ^ D > 0 ∧ leavesOf ns > 2 ^ D) := by omega
  obtain ⟨n, hloop, _, hnd⟩ := collapseLoop_cov cx live _ s ns L (cov_of_full _ _ hcov) hne hmeas hgood
  obtain ⟨hst, hrd⟩ := collapseLoop_exact cx live D _ s ns (nonInc_of_cov _ _ hcov) hne hr hlv hdD hmeas
  have hnr : n.cell.isReady = true := hrd n (by rw [hloop]; simp)
  have hv := collapseAll_val cx s ns (2 ^ D) live L hcov hne hd (Or.inr (by omega))
  rw [if_neg (by have := Nat.two_pow_pos D; omega), depthOf_two_pow] at hv
  have hnD : n.depth = D := by rw [hnd, hL, depthOf_two_pow]
  unfold St.collapseAll at hv ⊢
  rw [if_neg hchunks] at hv ⊢
  rw [hloop, hst] at hv ⊢
  have hp : 2 ^ D > 0 := Nat.two_pow_pos D
  simp only [collapseFinish, if_pos hp, depthOf_two_pow, hnD, Nat.lt_irrefl, false_and, if_false, St.vals,
    List.all_cons, List.all_nil, hnr, Bool.and_true, if_true] at hv ⊢
  refine ⟨trivial, ?_⟩
  rw [← hv]; rfl

/-! ## Covers cut at a node -/

theorem cov_take_drop (H : C → C → C) (z0 : C) {ns : List (Node C)} {L : List C} (k : Nat)
    (h : Cov H z0 true ns L) :
    Cov H z0 true (ns.take k) (L.take (leavesOf (ns.take k))) ∧
      Cov H z0 true (ns.drop k) (L.drop (leavesOf (ns.take k))) := by
  have hsplit : ns = ns.take k ++ ns.drop k := (List.take_append_drop k ns).symm
  by_cases hA : ns.take k = []
  · rw [hA]; simp [leavesOf]
    have : ns.drop k = ns := by
      have := hsplit; rw [hA, List.nil_append] at this; exact this.symm
    rw [this]; exact ⟨rfl, h⟩
  by_cases hB : ns.drop k = []
  · have hn : ns.take k = ns := by
      have := hsplit; rw [hB, List.append_nil] at this; exact this.symm
    have hl := cov_full_length H z0 h
    rw [hB, hn, ← hl, List.take_length, List.drop_length]
    exact ⟨h, rfl⟩
  rw [hsplit] at h
  obtain ⟨L1, L2, hL, h1, h2, _⟩ := cov_split H z0 hA hB h
  have hl1 := cov_full_length H z0 h1
  rw [hL, ← hl1, List.take_left' rfl, List.drop_left' rfl]
  exact ⟨h1, h2⟩

/-- A power-of-two block reduces to its bounded root. -/
theorem reduceTo_pow (H : C → C → C) (z0 : C) (d : Nat) (vs : List C) (hv : vs.length = 2 ^ d) :
    reduceTo H 1 vs.length vs = [mroot H z0 d vs] := by
  have hne : vs ≠ [] := by intro e; subst e; simp at hv; have := Nat.one_le_two_pow (n := d); omega
  have := reduceTo_chunks H z0 1 d (by omega) vs (by rw [hv]; simp) 0 vs.length
    (Nat.zero_le _) (by rw [hv]; exact Nat.le_trans (Nat.sub_le _ _) (le_two_pow _))
  rw [chunk_roots_zero] at this
  rw [this, chunks_single hne (by rw [hv]; exact Nat.le_refl _)]
  rfl

/-! ## The progressive round invariant -/

/-- The progressive roots stand for the complete groups `G`: group `i` has
    `4^i` chunks and its root is their bounded root. -/
def GroupsOK (cx : Ctx C) (prog : List (Cell C)) (G : List (List C)) : Prop :=
  prog.length = G.length ∧ ∀ i (hi : i < G.length) (hp : i < prog.length),
    (G[i]).length = 2 ^ (2 * i) ∧ (prog[i]).val = mroot cx.H cx.z0 (2 * i) G[i]

/-- Between rounds: the region past the roots is a ready, counted cover of
    the remaining leaves, no node wider than the current group. -/
structure RoundP (cx : Ctx C) (l : Layer C) (G : List (List C)) (Lb : List C) : Prop where
  prog : l.progressive = true
  inc : l.incremental = true
  nolegacy : l.legacy = false
  pend : l.pend = []
  tail : l.tail = []
  groups : GroupsOK cx l.prog G
  level : l.level = G.length
  cov : Cov cx.H cx.z0 true l.nodes Lb
  narrow : ∀ n ∈ l.nodes, n.depth ≤ 2 * l.level
  ready : ∀ n ∈ l.nodes, n.cell.isReady = true
  counted : l.counted = l.nodes.length
  shallow : ∀ n ∈ l.nodes, n.depth < 40

theorem groupsOK_snoc (cx : Ctx C) {prog : List (Cell C)} {G : List (List C)} (h : GroupsOK cx prog G)
    (r : Cell C) (g : List C) (hg : g.length = 2 ^ (2 * G.length)) (hr : r.val = mroot cx.H cx.z0 (2 * G.length) g) :
    GroupsOK cx (prog ++ [r]) (G ++ [g]) := by
  obtain ⟨hl, hi⟩ := h
  refine ⟨by simp [hl], ?_⟩
  intro i h1 h2
  rcases Nat.lt_or_ge i G.length with hlt | hge
  · rw [List.getElem_append_left hlt, List.getElem_append_left (by omega)]
    exact hi i hlt (by omega)
  · have : i = G.length := by simp at h1; omega
    subst this
    rw [List.getElem_append_right (Nat.le_refl _), List.getElem_append_right (by omega)]
    simp only [Nat.sub_self, List.getElem_cons_zero, List.length_append, List.length_singleton]
    have e : G.length - prog.length = 0 := by omega
    simp only [e, List.getElem_cons_zero]
    exact ⟨hg, hr⟩

theorem groupsOK_erase (cx : Ctx C) {prog prog' : List (Cell C)} {G : List (List C)} (h : GroupsOK cx prog G)
    (he : prog'.map Cell.erase = prog.map Cell.erase) : GroupsOK cx prog' G := by
  obtain ⟨hl, hi⟩ := h
  have hl' := erase_map_length he
  refine ⟨by omega, ?_⟩
  intro i h1 h2
  obtain ⟨a, b⟩ := hi i h1 (by omega)
  refine ⟨a, ?_⟩
  have := congrArg (fun xs => xs[i]?) (erase_vals he)
  simp only [List.getElem?_map, List.getElem?_eq_getElem h2, List.getElem?_eq_getElem (show i < prog.length by omega),
    Option.map_some, Option.some.injEq] at this
  rw [this]; exact b

theorem four_pow (n : Nat) : 4 ^ n = 2 ^ (2 * n) := by rw [Nat.pow_mul]

/-- `LayerLE` on each part of a region with roots. -/
theorem LayerLE.parts {l l' : Layer C} (h : LayerLE l l') :
    HolesSub l.prog l'.prog ∧ HolesSub (nodeCells l.nodes) (nodeCells l'.nodes) ∧
    HolesSub (l.pend.flatten ++ l.tail) (l'.pend.flatten ++ l'.tail) := by
  have hc := h.holes
  have hp : l.prog.length = l'.prog.length := (erase_map_length h.prog_map).symm
  have hn : (nodeCells l.nodes).length = (nodeCells l'.nodes).length := by simp [nodeCells_length, h.nodes_length]
  have e : ∀ x : Layer C, x.cells = x.prog ++ (nodeCells x.nodes ++ (x.pend.flatten ++ x.tail)) := by
    intro x; simp [Layer.cells, List.append_assoc]
  rw [e l, e l'] at hc
  have h2 := holesSub_suffix hp hc
  exact ⟨holesSub_prefix hp hc, holesSub_prefix hn h2, holesSub_suffix hn h2⟩

theorem nodes_ready_of_le {l l' : Layer C} (h : LayerLE l l') (hr : ∀ n ∈ l.nodes, n.cell.isReady = true) :
    ∀ n ∈ l'.nodes, n.cell.isReady = true := by
  intro n hn
  cases hc : n.cell.isReady with
  | true => rfl
  | false =>
    exfalso
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hn
    have hi0 : i < l.nodes.length := by rw [← h.nodes_length]; exact hi
    obtain ⟨c0, h0, h0r⟩ := h.parts.2.1 i _ (by simp [nodeCells, List.getElem?_map, List.getElem?_eq_getElem hi]) hc
    simp only [nodeCells, List.getElem?_map, List.getElem?_eq_getElem hi0, Option.map_some, Option.some.injEq] at h0
    rw [← h0, hr _ (List.getElem_mem _)] at h0r; cases h0r

/-! ## One round -/

/-- The new layer of a round, unfolded. -/
theorem roundLayer_fields (l : Layer C) (k : Nat) (r : Cell C) :
    (roundLayer l k r).prog = l.prog ++ [r] ∧ (roundLayer l k r).level = l.level + 1 ∧
    (roundLayer l k r).nodes = l.nodes.drop k ∧ (roundLayer l k r).counted = l.counted - k := ⟨rfl, rfl, rfl, rfl⟩

/-- **One round of `maybeCollapseProgressive`**: the first complete group of
    the region becomes the next progressive root (computed here, or by a
    background job whose hole is that root), and the region keeps covering
    the rest. -/
theorem progRound_ok (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (G : List (List C))
    (Lb : List C) (async : Bool) (h : s.stack = l :: ls) (hr : RoundP cx l G Lb) (ht : TopOK s)
    (hL : Lb.length < 2 ^ 40) (hbig : 4 ^ l.level ≤ leavesOf (l.nodes.take l.counted)) (hlvl : 2 * l.level < 64) :
    0 < consumeCount (l.nodes.take l.counted) (4 ^ l.level) ∧
    ∃ l' ls', ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.setTop
        (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2)).stack = l' :: ls' ∧
      l' = roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2 ∧
      RoundP cx l' (G ++ [Lb.take (2 ^ (2 * l.level))]) (Lb.drop (2 ^ (2 * l.level))) ∧
      TopOK ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.setTop
        (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2)) ∧
      ls'.map Layer.erase = ls.map Layer.erase ∧
      ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.setTop
        (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2)).base.map Cell.erase =
        s.base.map Cell.erase ∧
      ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.setTop
        (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2)).hashErr = s.hashErr ∧
      ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.setTop
        (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2)).below.length =
        s.below.length ∧
      HolesSub s.below ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.setTop
        (roundLayer ((s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).1.topLayer l)
          (consumeCount (l.nodes.take l.counted) (4 ^ l.level))
          (s.groupRoot cx async l (consumeCount (l.nodes.take l.counted) (4 ^ l.level))).2)).below ∧
      l'.nodes.length < l.nodes.length ∧ l'.collapsed = l.collapsed := by
  have hcnt : l.nodes.take l.counted = l.nodes := by rw [hr.counted, List.take_length]
  have hlv : leavesOf l.nodes = Lb.length := (cov_full_length cx.H cx.z0 hr.cov).symm
  rw [hcnt] at hbig ⊢
  rw [four_pow] at hbig ⊢
  generalize hD : 2 * l.level = D at hbig hlvl ⊢
  have hnarrow : ∀ n ∈ l.nodes, n.depth ≤ D := by rw [← hD]; exact hr.narrow
  obtain ⟨hk0, hkl⟩ := consumeCount_spec l.nodes D (nonInc_of_cov _ _ hr.cov) hnarrow hbig
  refine ⟨hk0, ?_⟩
  generalize hK : consumeCount l.nodes (2 ^ D) = K at hk0 hkl ⊢
  obtain ⟨hc1, hc2⟩ := cov_take_drop cx.H cx.z0 K hr.cov
  rw [hkl] at hc1 hc2
  have hgne : l.nodes.take K ≠ [] := by
    intro e; rw [e] at hkl; simp [leavesOf] at hkl; have := Nat.two_pow_pos D; omega
  have hglen : (Lb.take (2 ^ D)).length = 2 ^ D := by simp; omega
  have hgr : ∀ n ∈ l.nodes.take K, n.cell.isReady = true := fun n hn => hr.ready n (List.mem_of_mem_take hn)
  have hgd : ∀ n ∈ l.nodes.take K, n.depth ≤ D := fun n hn => hnarrow n (List.mem_of_mem_take hn)
  have hp0 : l.prog.length = G.length := hr.groups.1
  have hlc : l.cells = l.prog ++ nodeCells l.nodes := by simp [Layer.cells, hr.pend, hr.tail]
  -- the round's new layer, in general
  have hnew : ∀ (l1 : Layer C) (r : Cell C), LayerLE l l1 → r.val = mroot cx.H cx.z0 D (Lb.take (2 ^ D)) →
      RoundP cx (roundLayer l1 K r) (G ++ [Lb.take (2 ^ D)]) (Lb.drop (2 ^ D)) := by
    intro l1 r hle hrv
    have hp1 : l1.pend = [] := by have := hle.pend_map; rw [hr.pend] at this; simpa using this
    have ht1 : l1.tail = [] := by have := hle.tail_map; rw [hr.tail] at this; simpa using this
    have hn1 := hle.nodes_map
    have hdrop : (l1.nodes.drop K).map Node.erase = (l.nodes.drop K).map Node.erase := by
      rw [List.map_drop, hn1, ← List.map_drop]
    have hdep : ∀ n ∈ l1.nodes, ∃ m ∈ l.nodes, m.depth = n.depth := by
      intro n hn
      have : n.erase ∈ l.nodes.map Node.erase := by rw [← hn1]; exact List.mem_map_of_mem hn
      obtain ⟨m, hm, he'⟩ := List.mem_map.mp this
      exact ⟨m, hm, by have := congrArg Node.depth he'; exact this⟩
    refine ⟨by rw [roundLayer, hle.flag_progressive]; exact hr.prog, by rw [roundLayer, hle.flag_incremental]; exact hr.inc,
      by rw [roundLayer, hle.flag_legacy]; exact hr.nolegacy, hp1, ht1, ?_, ?_, ?_, ?_, ?_, ?_, ?_⟩
    · rw [(roundLayer_fields l1 K r).1]
      apply groupsOK_snoc cx (groupsOK_erase cx hr.groups hle.prog_map) r _ (by rw [← hr.level, hD]; exact hglen)
      rw [← hr.level, hD]; exact hrv
    · show l1.level + 1 = _; rw [show l1.level = l.level from by have := congrArg Layer.level hle.erase; exact this,
        hr.level]; simp
    · rw [(roundLayer_fields l1 K r).2.2.1]; exact cov_of_erase_eq cx.H cx.z0 hdrop hc2
    · intro n hn
      rw [(roundLayer_fields l1 K r).2.2.1] at hn
      obtain ⟨m, hm, hmd⟩ := hdep n (List.mem_of_mem_drop hn)
      rw [(roundLayer_fields l1 K r).2.1, ← hmd,
        show l1.level = l.level from by have := congrArg Layer.level hle.erase; exact this]
      have := hr.narrow m hm; omega
    · intro n hn
      rw [(roundLayer_fields l1 K r).2.2.1] at hn
      exact nodes_ready_of_le hle hr.ready n (List.mem_of_mem_drop hn)
    · rw [(roundLayer_fields l1 K r).2.2.2, (roundLayer_fields l1 K r).2.2.1, List.length_drop,
        show l1.counted = l.counted from by have := congrArg Layer.counted hle.erase; exact this, hr.counted,
        hle.nodes_length]
    · intro n hn
      rw [(roundLayer_fields l1 K r).2.2.1] at hn
      obtain ⟨m, hm, hmd⟩ := hdep n (List.mem_of_mem_drop hn)
      rw [← hmd]; exact hr.shallow m hm
  have hlpos : 0 < l.nodes.length := by
    rcases Nat.eq_zero_or_pos l.nodes.length with h0 | h0
    · rw [List.eq_nil_of_length_eq_zero h0] at hgne; simp at hgne
    · exact h0
  have hshrink : ∀ l1 : Layer C, LayerLE l l1 → ∀ r, (roundLayer l1 K r).nodes.length < l.nodes.length := by
    intro l1 hle r
    rw [(roundLayer_fields l1 K r).2.2.1, List.length_drop, hle.nodes_length]; omega
  unfold St.groupRoot
  rw [four_pow, hD]
  split
  · -- a background job for the group
    rename_i hcond
    obtain ⟨_, hall, _, _, _⟩ := hcond
    have hall0 : ∀ n ∈ l.nodes.take K, n.depth = 0 := by
      intro n hn; have := List.all_eq_true.mp hall n hn; simpa using this
    have hvals : (nodeCells (l.nodes.take K)).map Cell.val = Lb.take (2 ^ D) := by
      unfold nodeCells; rw [List.map_map]; exact cov_leaves cx.H cx.z0 _ _ hc1 hall0
    have hinr : ∀ c ∈ nodeCells (l.nodes.take K), c.isReady = true := by
      intro c hc; simp only [nodeCells, List.mem_map] at hc; obtain ⟨n, hn, rfl⟩ := hc; exact hgr n hn
    have hinl : (nodeCells (l.nodes.take K)).length = 2 ^ D := by
      rw [← List.length_map (f := Cell.val), hvals, hglen]
    have hout : reduceTo cx.H 1 (nodeCells (l.nodes.take K)).length ((nodeCells (l.nodes.take K)).map Cell.val) =
        [mroot cx.H cx.z0 D (Lb.take (2 ^ D))] := by
      rw [show (nodeCells (l.nodes.take K)).length = ((nodeCells (l.nodes.take K)).map Cell.val).length by simp,
        reduceTo_pow cx.H cx.z0 D _ (by rw [hvals, hglen]), hvals]
    generalize hvd : mroot cx.H cx.z0 D (Lb.take (2 ^ D)) = v at hout hnew
    have hact : s.topStart + l.prog.length = s.below.length + l.prog.length := by rw [St.topStart_eq s l ls h]
    obtain ⟨hv1, ht1, hn1, hh1, hlen1⟩ := ringDrain_topOK cx.P s ht
    obtain ⟨l1, ls1, hs1, hle1, hls1⟩ := LayerLE.of_view hv1 h (ringDrain_holesSub cx.P s ht.holes)
    have hbl1 := below_length_of_view hv1 h hs1
    let J : Job C := ⟨(s.ringDrain cx.P).nextJob, s.below.length + l.prog.length, [v]⟩
    let T : St C := (s.ringDrain cx.P).addJob J
    have hen : s.enqueue cx (nodeCells (l.nodes.take K)) 1 (s.topStart + l.prog.length) =
        (T, [Cell.hole v (s.ringDrain cx.P).nextJob 0]) := by
      rw [enqueue_eq cx s _ 1 _ hinr, hout, hact]; rfl
    rw [hen]
    simp only [List.head?_cons, Option.getD_some]
    have hTs : T.stack = l1 :: ls1 := hs1
    have hTtop : T.topLayer l = l1 := by simp [St.topLayer, hTs]
    rw [hTtop]
    have hRP := hnew l1 (.hole v (s.ringDrain cx.P).nextJob 0) hle1 rfl
    have hp1 : l1.pend = [] := by have := hle1.pend_map; rw [hr.pend] at this; simpa using this
    have ht1' : l1.tail = [] := by have := hle1.tail_map; rw [hr.tail] at this; simpa using this
    have hpl1 : l1.prog.length = l.prog.length := erase_map_length hle1.prog_map
    have hc1' : l1.cells = l1.prog ++ nodeCells l1.nodes := by simp [Layer.cells, hp1, ht1']
    have hl'c : (roundLayer l1 K (.hole v (s.ringDrain cx.P).nextJob 0)).cells =
        l1.prog ++ ([Cell.hole v (s.ringDrain cx.P).nextJob 0] ++ nodeCells (l1.nodes.drop K)) := by
      simp [Layer.cells, roundLayer, hp1, ht1', List.append_assoc]
    have hr1 : ∀ n ∈ l1.nodes, n.cell.isReady = true := nodes_ready_of_le hle1 hr.ready
    refine ⟨_, ls1, St.setTop_stack T l1 _ ls1 hTs, rfl, hRP, ⟨?_, ?_, ?_, ?_⟩, hls1, ?_, ?_, ?_, ?_,
      hshrink l1 hle1 _, hle1.flag_collapsed⟩
    · show HolesOK (St.setTop ((s.ringDrain cx.P).addJob J) _)
      unfold St.addJob
      apply holesOK_addJob _ l1 _ ls1 hs1 ht1.holes J (fun j' hj' => Nat.ne_of_lt (ht1.ids j' hj'))
        l1.prog.length (by rw [hc1']; simp)
      · rw [hc1', hl'c, List.take_left' rfl, List.take_left' rfl]
      · show s.below.length + l.prog.length = _; rw [hbl1, hpl1]
      · intro i hi
        have hi0 : i = 0 := by simp [J] at hi; omega
        subst hi0
        refine ⟨0, ?_⟩
        rw [hl'c, Nat.add_zero, List.getElem?_append_right (Nat.le_refl _), Nat.sub_self]
        rfl
      · rw [hl'c]; simp [J]
      · intro i hi hi'
        have hm : (roundLayer l1 K (.hole v (s.ringDrain cx.P).nextJob 0)).cells[i]'hi' ∈
            nodeCells (l1.nodes.drop K) := by
          have hc'' : (roundLayer l1 K (.hole v (s.ringDrain cx.P).nextJob 0)).cells =
              (l1.prog ++ [Cell.hole v (s.ringDrain cx.P).nextJob 0]) ++ nodeCells (l1.nodes.drop K) := by
            rw [hl'c, List.append_assoc]
          simp only [hc'']
          rw [List.getElem_append_right (by simp [J] at hi; simp; omega)]
          exact List.getElem_mem _
        simp only [nodeCells, List.mem_map] at hm
        obtain ⟨n, hn, he⟩ := hm
        rw [← he]; exact hr1 n (List.mem_of_mem_drop hn)
      · have := jobs_end_of_ready (s.ringDrain cx.P) ht1 ((s.ringDrain cx.P).below ++ l1.prog) (nodeCells l1.nodes)
          (by rw [St.cells_top _ l1 ls1 hs1, hc1', List.append_assoc]) (by
            intro c hc; simp only [nodeCells, List.mem_map] at hc; obtain ⟨n, hn, he⟩ := hc; rw [← he]; exact hr1 n hn)
        intro j hj; have := this j hj; simp only [List.length_append] at this; exact this
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
    · show (T.setTop _).base.map Cell.erase = s.base.map Cell.erase
      rw [St.setTop_base]
      exact congrArg Prod.fst hv1
    · rw [St.setTop_hashErr]; exact hh1
    · rw [St.setTop_below T l1 _ ls1 hTs]; exact hbl1
    · rw [St.setTop_below T l1 _ ls1 hTs]
      show HolesSub s.below (s.ringDrain cx.P).below
      exact holesSub_below hv1 h hs1 (ringDrain_holesSub cx.P s ht.holes)
  · -- the group reduced here
    obtain ⟨hst, hroot⟩ := collapseAll_exact cx s (l.nodes.take K) (Lb.take (2 ^ D)) D
      (decide (l.nodes.length > K)) hc1 hgne hgr hglen hgd hlvl
    rw [hst, hroot]
    have htop : s.topLayer l = l := by simp [St.topLayer, h]
    rw [htop]
    have hRP := hnew l (.ready (mroot cx.H cx.z0 D (Lb.take (2 ^ D)))) (LayerLE.refl l) rfl
    have hl'c : (roundLayer l K (.ready (mroot cx.H cx.z0 D (Lb.take (2 ^ D))))).cells =
        l.prog ++ ([Cell.ready (mroot cx.H cx.z0 D (Lb.take (2 ^ D)))] ++ nodeCells (l.nodes.drop K)) := by
      simp [Layer.cells, roundLayer, hr.pend, hr.tail, List.append_assoc]
    refine ⟨_, ls, St.setTop_stack s l _ ls h, rfl, hRP, ⟨?_, by rw [St.setTop_err]; exact ht.err,
      fun j hj => by rw [St.setTop_jobs] at hj; exact ht.jobsne j hj,
      fun j hj => by rw [St.setTop_jobs] at hj; rw [St.setTop_nextJob]; exact ht.ids j hj⟩, rfl,
      by rw [St.setTop_base], St.setTop_hashErr s _, by rw [St.setTop_below s l _ ls h],
      by rw [St.setTop_below s l _ ls h]; exact HolesSub.refl _, hshrink l (LayerLE.refl l) _, rfl⟩
    apply holesOK_setTop s l _ ls h ht.holes l.prog.length (by rw [hlc]; simp)
      (by rw [hlc, hl'c, List.take_left' rfl, List.take_left' rfl]) (by rw [hl'c]; simp)
    · intro i hi hi'
      have hm : (roundLayer l K (.ready (mroot cx.H cx.z0 D (Lb.take (2 ^ D))))).cells[i]'hi' ∈
          [Cell.ready (mroot cx.H cx.z0 D (Lb.take (2 ^ D)))] ++ nodeCells (l.nodes.drop K) := by
        simp only [hl'c]
        rw [List.getElem_append_right hi]; exact List.getElem_mem _
      rcases List.mem_append.mp hm with hm | hm
      · simp at hm; rw [hm]; rfl
      · simp only [nodeCells, List.mem_map] at hm
        obtain ⟨n, hn, he⟩ := hm
        rw [← he]; exact hr.ready n (List.mem_of_mem_drop hn)
    · have := jobs_end_of_ready s ht (s.below ++ l.prog) (nodeCells l.nodes)
        (by rw [St.cells_top s l ls h, hlc, List.append_assoc]) (by
          intro c hc; simp only [nodeCells, List.mem_map] at hc; obtain ⟨n, hn, rfl⟩ := hc; exact hr.ready n hn)
      intro j hj; have := this j hj; simp at this; omega

/-! ## The rounds -/

theorem lvl_bound {lv n : Nat} (h1 : 4 ^ lv ≤ n) (h2 : n < 2 ^ 40) : 2 * lv < 64 := by
  rw [four_pow] at h1
  have : 2 ^ (2 * lv) < 2 ^ 40 := by omega
  have := (Nat.pow_lt_pow_iff_right (by decide : 1 < 2)).mp this
  omega

/-- **The rounds of `maybeCollapseProgressive`** take every complete group:
    afterwards fewer than a group's leaves remain in the region. -/
theorem progRounds_ok (cx : Ctx C) (async : Bool) : ∀ fuel (s : St C) (l : Layer C) (ls : List (Layer C))
    (G : List (List C)) (Lb : List C) (f : Bool), s.stack = l :: ls → RoundP cx l G Lb → TopOK s →
    Lb.length < 2 ^ 40 → l.nodes.length < fuel →
    ∃ G' Lb' ls', (progRounds cx async fuel s l f).1.stack = (progRounds cx async fuel s l f).2.1 :: ls' ∧
      RoundP cx (progRounds cx async fuel s l f).2.1 G' Lb' ∧ G.flatten ++ Lb = G'.flatten ++ Lb' ∧
      Lb'.length < 4 ^ (progRounds cx async fuel s l f).2.1.level ∧ TopOK (progRounds cx async fuel s l f).1 ∧
      ls'.map Layer.erase = ls.map Layer.erase ∧
      (progRounds cx async fuel s l f).1.base.map Cell.erase = s.base.map Cell.erase ∧
      (progRounds cx async fuel s l f).1.hashErr = s.hashErr ∧
      (progRounds cx async fuel s l f).1.below.length = s.below.length ∧
      HolesSub s.below (progRounds cx async fuel s l f).1.below ∧
      ((progRounds cx async fuel s l f).2.2 = false → progRounds cx async fuel s l f = (s, l, f)) ∧
      (f = true → (progRounds cx async fuel s l f).2.2 = true) ∧
      (progRounds cx async fuel s l f).2.1.collapsed = l.collapsed := by
  intro fuel
  induction fuel with
  | zero => intro s l ls G Lb f _ _ _ _ hf; omega
  | succ fuel ih =>
    intro s l ls G Lb f h hr ht hL hf
    have hcnt : l.nodes.take l.counted = l.nodes := by rw [hr.counted, List.take_length]
    have hlv : leavesOf l.nodes = Lb.length := (cov_full_length cx.H cx.z0 hr.cov).symm
    simp only [progRounds]
    split
    · rename_i hsm
      rw [hcnt, hlv] at hsm
      exact ⟨G, Lb, ls, h, hr, rfl, hsm, ht, rfl, rfl, rfl, rfl, HolesSub.refl _, fun _ => rfl, id, rfl⟩
    rename_i hbig
    have hbig' : 4 ^ l.level ≤ leavesOf (l.nodes.take l.counted) := by omega
    have hlvl : 2 * l.level < 64 := lvl_bound (by rw [hcnt, hlv] at hbig'; exact hbig') hL
    obtain ⟨hk0, l', ls', hs', hl', hr', ht', hle', hba', hh', hbl', hsub', hshr', hco'⟩ :=
      progRound_ok cx s l ls G Lb async h hr ht hL hbig' hlvl
    split
    · omega
    rw [← hl'] at hs' ht' hba' hh' hbl' hsub' ⊢
    have hLb' : (Lb.drop (2 ^ (2 * l.level))).length < 2 ^ 40 := by
      rw [List.length_drop]; exact Nat.lt_of_le_of_lt (Nat.sub_le _ _) hL
    obtain ⟨G'', Lb'', ls'', hs'', hr'', hsp'', hlt'', ht'', hle'', hba'', hh'', hbl'', hsub'', _, hft'', hco''⟩ :=
      ih _ l' ls' _ _ true hs' hr' ht' hLb' (by omega)
    refine ⟨G'', Lb'', ls'', hs'', hr'', ?_, hlt'', ht'', by rw [hle'', hle'], by rw [hba'', hba'],
      by rw [hh'', hh'], by rw [hbl'', hbl'], HolesSub.trans hsub' hsub'', ?_, fun _ => hft'' rfl, by rw [hco'', hco']⟩
    · rw [← hsp'', List.flatten_append, List.flatten_singleton, List.append_assoc, List.take_append_drop]
    · intro hfalse; rw [hft'' rfl] at hfalse; cases hfalse

/-! ## Step 2: compaction -/

theorem cov_join' (H : C → C → C) (z0 : C) {A B : List (Node C)} {L1 L2 : List C} (h1 : Cov H z0 true A L1)
    (h2 : Cov H z0 true B L2) (hseam : ∀ a b, A.getLast? = some a → B.head? = some b → b.depth ≤ a.depth) :
    Cov H z0 true (A ++ B) (L1 ++ L2) := by
  by_cases hA : A = []
  · subst hA; have : L1 = [] := h1; subst this; simpa using h2
  by_cases hB : B = []
  · subst hB; have : L2 = [] := h2; subst this; simpa using h1
  apply cov_join H z0 hA hB h1 h2
  apply hseam
  · exact List.getLast?_eq_getLast hA
  · exact List.head?_eq_head hB

theorem progCompact_filter (cx : Ctx C) : ∀ f (s : St C) (ns : List (Node C)),
    progCompact cx f s ns = progCompact cx f s (ns.filter (·.depth < f)) := by
  intro f
  induction f with
  | zero => intro s ns; rfl
  | succ f ih =>
    intro s ns
    have hf : (ns.filter (·.depth < f + 1)).filter (·.depth == f) = ns.filter (·.depth == f) := by
      rw [List.filter_filter]; congr 1; funext n; by_cases h : n.depth = f <;> simp [h]
    have hg : (ns.filter (·.depth < f + 1)).filter (·.depth < f) = ns.filter (·.depth < f) := by
      rw [List.filter_filter]; congr 1; funext n; by_cases h : n.depth < f <;> simp [h] <;> omega
    simp only [progCompact, hf]
    rw [ih _ ns, ih _ (ns.filter (·.depth < f + 1)), hg]

/-- **Compaction keeps the cover**: pairs of a depth become one node a level
    up, the odd one stays; nothing is read but ready nodes. -/
theorem progCompact_ok (cx : Ctx C) : ∀ f (s : St C) (ns : List (Node C)) (L : List C), NonInc ns →
    (∀ n ∈ ns, n.depth < f) → Cov cx.H cx.z0 true ns L → (∀ n ∈ ns, n.cell.isReady = true) →
    (progCompact cx f s ns).1 = s ∧ Cov cx.H cx.z0 true (progCompact cx f s ns).2 L ∧
      (∀ n ∈ (progCompact cx f s ns).2, n.cell.isReady = true) ∧ (∀ n ∈ (progCompact cx f s ns).2, n.depth ≤ f) := by
  intro f
  induction f with
  | zero =>
    intro s ns L _ hd hc _
    have : ns = [] := by
      cases ns with
      | nil => rfl
      | cons n _ => have := hd n (by simp); omega
    subst this
    exact ⟨rfl, hc, by simp [progCompact], by simp [progCompact]⟩
  | succ f ih =>
    intro s ns L hni hd hc hr
    have hsplit := nonInc_split3 f ns hni
    have hA0 : ns.filter (·.depth > f) = [] := List.filter_eq_nil_iff.mpr (fun n hn => by
      have := hd n hn; simp; omega)
    rw [hA0, List.nil_append] at hsplit
    generalize hAe : ns.filter (·.depth == f) = A at hsplit
    generalize hBe : ns.filter (·.depth < f) = B at hsplit
    have hAd : ∀ n ∈ A, n.depth = f := fun n hn => by rw [← hAe] at hn; simpa using (List.mem_filter.mp hn).2
    have hBd : ∀ n ∈ B, n.depth < f := fun n hn => by rw [← hBe] at hn; simpa using (List.mem_filter.mp hn).2
    have hAr : ∀ n ∈ A, n.cell.isReady = true := fun n hn => hr n (by rw [hsplit]; simp [hn])
    have hBr : ∀ n ∈ B, n.cell.isReady = true := fun n hn => hr n (by rw [hsplit]; simp [hn])
    -- the leaves of each part
    have hcs := cov_take_drop cx.H cx.z0 A.length hc
    rw [hsplit, List.take_left' rfl, List.drop_left' rfl] at hcs
    obtain ⟨hcA, hcB⟩ := hcs
    generalize hL1 : L.take (leavesOf A) = L1 at hcA
    generalize hL2 : L.drop (leavesOf A) = L2 at hcB
    have hL : L = L1 ++ L2 := by rw [← hL1, ← hL2, List.take_append_drop]
    -- the depth's pairs and its odd node
    generalize hm : 2 * (A.length / 2) = m
    have hpair := cov_take_drop cx.H cx.z0 m hcA
    obtain ⟨hcE, hcO⟩ := hpair
    have hEd : ∀ n ∈ A.take m, n.depth = f := fun n hn => hAd n (List.mem_of_mem_take hn)
    have hEr : ∀ n ∈ A.take m, n.cell.isReady = true := fun n hn => hAr n (List.mem_of_mem_take hn)
    have hEev : (A.take m).length % 2 = 0 := by simp; omega
    have hst : (s.pairNodes cx (A.take m)).1 = s := by
      simp only [St.pairNodes, St.vals]
      rw [if_pos (List.all_eq_true.mpr (fun c hc => by
        simp only [nodeCells, List.mem_map] at hc; obtain ⟨n, hn, rfl⟩ := hc; exact hEr n hn))]
    have hP : Cov cx.H cx.z0 true (s.pairNodes cx (A.take m)).2 (L1.take (leavesOf (A.take m))) ∧
        (∀ n ∈ (s.pairNodes cx (A.take m)).2, n.depth = f + 1 ∧ n.cell.isReady = true) := by
      by_cases hE : A.take m = []
      · rw [hE]; simp [St.pairNodes, St.vals, pairUp, leavesOf, nodeCells]; rfl
      · rw [pairNodes_group cx s hE hEd]
        refine ⟨pair_full_group cx.H cx.z0 hEd hE hEev hcE, ?_⟩
        intro n hn; simp only [List.mem_map] at hn; obtain ⟨_, _, rfl⟩ := hn; exact ⟨rfl, rfl⟩
    -- the lower depths
    have hrec : progCompact cx f s ns = progCompact cx f s B := by
      rw [progCompact_filter cx f s ns, hBe]
    obtain ⟨i1, i2, i3, i4⟩ := ih s B L2 (nonInc_of_cov _ _ hcB) hBd hcB hBr
    simp only [progCompact]
    rw [hAe, hm, hst, hrec]
    refine ⟨i1, ?_, ?_, ?_⟩
    · rw [hL, List.append_assoc]
      have hL1' : L1 = L1.take (leavesOf (A.take m)) ++ L1.drop (leavesOf (A.take m)) := (List.take_append_drop _ _).symm
      rw [hL1', List.append_assoc]
      apply cov_join' cx.H cx.z0 hP.1
      · apply cov_join' cx.H cx.z0 hcO i2
        intro a b ha hb
        have := i4 b (List.mem_of_mem_head? hb)
        have := hAd a (List.mem_of_mem_drop (List.mem_of_mem_getLast? ha)); omega
      · intro a b ha hb
        have hadep := (hP.2 a (List.mem_of_mem_getLast? ha)).1
        rcases List.mem_append.mp (List.mem_of_mem_head? hb) with hb' | hb'
        · have := hAd b (List.mem_of_mem_drop hb'); omega
        · have := i4 b hb'; omega
    · intro n hn
      rcases List.mem_append.mp hn with hn | hn
      · rcases List.mem_append.mp hn with hn | hn
        · exact (hP.2 n hn).2
        · exact hAr n (List.mem_of_mem_drop hn)
      · exact i3 n hn
    · intro n hn
      rcases List.mem_append.mp hn with hn | hn
      · rcases List.mem_append.mp hn with hn | hn
        · rw [(hP.2 n hn).1]; exact Nat.le_refl _
        · have := hAd n (List.mem_of_mem_drop hn); omega
      · have := i4 n hn; omega

/-! ## The progressive scope invariant -/

/-- **A progressive scope** between calls: its roots stand for the complete
    groups `G`, its region covers the leaves `Lb` that follow them (no node
    wider than the current group), a deferred run may follow. An
    uncollapsed scope has no roots and only leaves (Go's precondition for
    reinitializing its counts). -/
structure ProgOK (cx : Ctx C) (l : Layer C) (G : List (List C)) (Lb : List C) : Prop where
  prog : l.progressive = true
  inc : l.incremental = true
  nolegacy : l.legacy = false
  groups : GroupsOK cx l.prog G
  level : l.level = G.length
  cov : Cov cx.H cx.z0 true l.nodes Lb
  narrow : ∀ n ∈ l.nodes, n.depth ≤ 2 * l.level
  shallow : ∀ n ∈ l.nodes, n.depth < 40
  leaves : l.collapsed = false → l.level = 0 ∧ ∀ n ∈ l.nodes, n.depth = 0
  pendOK : PendOK cx l

/-- The leaves a progressive scope stands for. -/
def ProgScope (cx : Ctx C) (l : Layer C) (L : List C) : Prop :=
  ∃ G Lb, L = G.flatten ++ Lb ++ pendLeaves cx l ∧ ProgOK cx l G Lb

theorem LayerLE.pendOK' (cx : Ctx C) {l l' : Layer C} (h : LayerLE l l') (hpo : PendOK cx l) : PendOK cx l' := by
  have hpe : l'.pend = [] ↔ l.pend = [] := by
    have := h.pend_map
    constructor
    · intro e; rw [e] at this; simpa using this.symm
    · intro e; rw [e] at this; simpa using this
  have hlens : l'.pend.map List.length = l.pend.map List.length := by
    have := congrArg (List.map List.length) h.pend_map
    simpa [List.map_map, Function.comp_def] using this
  have hall : ∀ c ∈ l'.pend.flatten ++ l'.tail, c.isReady = true := by
    intro c hc
    cases hcr : c.isReady with
    | true => rfl
    | false =>
      exfalso
      obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hc
      obtain ⟨c0, h0, h0r⟩ := h.parts.2.2 i _ (List.getElem?_eq_getElem hi) hcr
      have hm := List.mem_of_getElem? h0
      have : c0.isReady = true := by
        rcases List.mem_append.mp hm with hm | hm
        · obtain ⟨e, he, hce⟩ := List.mem_flatten.mp hm; exact hpo.ready e he c0 hce
        · exact hpo.tready c0 hm
      rw [this] at h0r; cases h0r
  refine ⟨?_, ?_, ?_, ?_, ?_⟩
  · intro e
    have := h.tail_map; rw [hpo.tail (hpe.mp e)] at this; simpa using this
  · intro e he c hc; exact hall c (List.mem_append_left _ (List.mem_flatten.mpr ⟨e, he, hc⟩))
  · intro c hc; exact hall c (List.mem_append_right _ hc)
  · intro e he
    rw [h.pendC]
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem he
    have hi0 : i < l.pend.length := by have := congrArg List.length hlens; simp at this; omega
    have := congrArg (fun xs => xs[i]?) hlens
    simp only [List.getElem?_map, List.getElem?_eq_getElem hi, List.getElem?_eq_getElem hi0, Option.map_some,
      Option.some.injEq] at this
    rw [this]; exact hpo.width _ (List.getElem_mem _)
  · intro hne; rw [h.pendC]; exact hpo.pow (fun e => hne (hpe.mpr e))

/-- **A progressive scope's invariant survives drains.** -/
theorem LayerLE.progOK (cx : Ctx C) {l l' : Layer C} (h : LayerLE l l') {G : List (List C)} {Lb : List C}
    (hp : ProgOK cx l G Lb) : ProgOK cx l' G Lb := by
  have hdep : ∀ n ∈ l'.nodes, ∃ m ∈ l.nodes, m.depth = n.depth := by
    intro n hn
    have : n.erase ∈ l.nodes.map Node.erase := by rw [← h.nodes_map]; exact List.mem_map_of_mem hn
    obtain ⟨m, hm, he'⟩ := List.mem_map.mp this
    exact ⟨m, hm, by have := congrArg Node.depth he'; exact this⟩
  have hlv : l'.level = l.level := by have := congrArg Layer.level h.erase; exact this
  refine ⟨by rw [h.flag_progressive]; exact hp.prog, by rw [h.flag_incremental]; exact hp.inc,
    by rw [h.flag_legacy]; exact hp.nolegacy, groupsOK_erase cx hp.groups h.prog_map, by rw [hlv]; exact hp.level,
    cov_of_erase_eq cx.H cx.z0 h.nodes_map hp.cov, ?_, ?_, ?_, h.pendOK' cx hp.pendOK⟩
  · intro n hn; obtain ⟨m, hm, hmd⟩ := hdep n hn; rw [← hmd, hlv]; exact hp.narrow m hm
  · intro n hn; obtain ⟨m, hm, hmd⟩ := hdep n hn; rw [← hmd]; exact hp.shallow m hm
  · intro hc
    obtain ⟨h1, h2⟩ := hp.leaves (by rw [← h.flag_collapsed]; exact hc)
    exact ⟨by rw [hlv]; exact h1, fun n hn => by obtain ⟨m, hm, hmd⟩ := hdep n hn; rw [← hmd]; exact h2 m hm⟩

theorem LayerLE.progScope (cx : Ctx C) {l l' : Layer C} (h : LayerLE l l') {L : List C} (hp : ProgScope cx l L) :
    ProgScope cx l' L := by
  obtain ⟨G, Lb, hL, hpo⟩ := hp
  exact ⟨G, Lb, by rw [h.pendLeaves cx]; exact hL, h.progOK cx hpo⟩

/-! ## What pre-collapse does to the innermost scope's shape -/

theorem uncollapsedAsLeaves_state' (s : St C) (l : Layer C) : (s.uncollapsedAsLeaves l).1.stack = s.stack :=
  (uncollapsedAsLeaves_sim s l).2.2.1

/-- A property of the innermost scope's shape that drains keep. -/
def TopShape (P : Layer C → Prop) (t : St C) : Prop := ∀ l ls, t.stack = l :: ls → P l

theorem sameShapes_head {a b : List (Layer C)} (h : Layer.SameShapes a b) {l : Layer C} {ls : List (Layer C)}
    (ha : a = l :: ls) : ∃ l' ls', b = l' :: ls' ∧ Layer.SameShape l l' := by
  subst ha
  cases b with
  | nil => exact h.elim
  | cons l' ls' => exact ⟨l', ls', rfl, h.1⟩

theorem drainOldest_top (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) :
    ∃ l' ls', s.drainOldest.stack = l' :: ls' ∧ Layer.SameShape l l' := by
  unfold St.drainOldest
  cases hj : s.jobs with
  | nil => exact ⟨l, ls, h, ⟨rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl⟩⟩
  | cons j js =>
    simp only
    have hl := writeHole_length j.id j.dst j.out s.cells
    obtain ⟨_, hss, _⟩ := St.setCells_spec s (writeHole j.id j.dst j.out s.cells).1 hl
    obtain ⟨l', ls', h', hsh⟩ := sameShapes_head hss h
    split
    · exact ⟨l', ls', h', hsh⟩
    · exact ⟨l', ls', by rw [St.fail_stack]; exact h', hsh⟩

theorem enqueue_top (cx : Ctx C) (s : St C) (input : List (Cell C)) (out dst : Nat) (l : Layer C)
    (ls : List (Layer C)) (h : s.stack = l :: ls) :
    ∃ l' ls', (s.enqueue cx input out dst).1.stack = l' :: ls' ∧ Layer.SameShape l l' := by
  unfold St.enqueue
  rw [show (if s.jobs.length ≥ cx.P.ring then s.drainOldest else s) = s.ringDrain cx.P from rfl]
  have hr : ∃ l' ls', (s.ringDrain cx.P).stack = l' :: ls' ∧ Layer.SameShape l l' := by
    unfold St.ringDrain; split
    · exact drainOldest_top s l ls h
    · exact ⟨l, ls, h, ⟨rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl, rfl⟩⟩
  obtain ⟨l', ls', h', hsh⟩ := hr
  exact ⟨l', ls', by simp only; rw [vals_stack]; exact h', hsh⟩

theorem depths_of_same {l l' : Layer C} (h : Layer.SameShape l l') :
    ∀ n ∈ l'.nodes, ∃ m ∈ l.nodes, m.depth = n.depth := by
  intro n hn
  have : n.depth ∈ l.nodes.map (·.depth) := by rw [← h.depths]; exact List.mem_map_of_mem hn
  obtain ⟨m, hm, he⟩ := List.mem_map.mp this
  exact ⟨m, hm, he⟩

theorem precollapseLoop_top (cx : Ctx C) (B : Nat) (hcB : cx.P.capDepth ≤ B) : ∀ fuel (t : St C) (ncap : Nat),
    TopShape (fun l => (∀ n ∈ l.nodes, n.depth ≤ B) ∧ l.collapsed = true) t →
    TopShape (fun l => (∀ n ∈ l.nodes, n.depth ≤ B) ∧ l.collapsed = true) (precollapseLoop cx fuel t ncap) := by
  intro fuel
  induction fuel with
  | zero => intro t ncap ht; exact ht
  | succ fuel ih =>
    intro t ncap ht
    cases h : t.stack with
    | nil => simp only [precollapseLoop, h]; exact ht
    | cons l ls =>
      simp only [precollapseLoop, h]
      split
      · exact ht
      · apply ih
        obtain ⟨l', ls', h', hsh⟩ := enqueue_top cx t (nodeCells ((l.nodes.drop ncap).take cx.P.cap)) 1
          (t.activeStart + ncap) l ls h
        have hc : (t.capJob cx l ncap).1.stack = l' :: ls' := h'
        have htl : (t.capJob cx l ncap).1.topLayer l = l' := by simp [St.topLayer, hc]
        rw [htl]
        intro l2 ls2 h2
        rw [St.setTop_stack _ l' _ ls' hc] at h2
        obtain ⟨rfl, rfl⟩ := List.cons.inj h2
        obtain ⟨hB, _⟩ := ht l ls h
        refine ⟨?_, rfl⟩
        intro n hn
        simp only [capCompact, List.mem_append, List.mem_map] at hn
        rcases hn with (hn | ⟨c, _, rfl⟩) | hn
        · obtain ⟨m, hm, hmd⟩ := depths_of_same hsh n (List.mem_of_mem_take hn)
          rw [← hmd]; exact hB m hm
        · exact hcB
        · obtain ⟨m, hm, hmd⟩ := depths_of_same hsh n (List.mem_of_mem_drop (List.mem_of_mem_drop hn))
          rw [← hmd]; exact hB m hm

/-- **Pre-collapse keeps a scope collapsed and its nodes no wider than the
    group**, the cap being no wider either. -/
theorem precollapse_top (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (B : Nat) (hB : ∀ n ∈ l.nodes, n.depth ≤ B) (hcB : cx.P.capDepth ≤ B) (hc : l.collapsed = true) :
    TopShape (fun l => (∀ n ∈ l.nodes, n.depth ≤ B) ∧ l.collapsed = true) (s.precollapse cx).1 := by
  have hn : s.normTop.stack = { l with counted := l.nodes.length } :: ls := by
    unfold St.normTop; rw [h]; simp only
    rw [St.setTop_stack _ l _ ls (by rw [uncollapsedAsLeaves_state' s l]; exact h)]
    simp [St.uncollapsedAsLeaves, hc]
  have hT : TopShape (fun l => (∀ n ∈ l.nodes, n.depth ≤ B) ∧ l.collapsed = true) s.normTop := by
    intro l2 ls2 h2; rw [hn] at h2; obtain ⟨rfl, rfl⟩ := List.cons.inj h2; exact ⟨hB, hc⟩
  unfold St.precollapse
  rw [hn]
  simp only
  split
  · exact hT
  · apply precollapseLoop_top cx B hcB
    intro l2 ls2 h2
    rw [failUnless_stack] at h2
    exact hT l2 ls2 h2

/-! ## The drain and the reinitialization -/

/-- The drain before a progressive collapse or close: the innermost scope
    loses all holes. -/
theorem drainTop_any (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) (ht : TopOK s) :
    ∃ l1 ls1, (s.drainFor s.topStart).stack = l1 :: ls1 ∧ LayerLE l l1 ∧ (∀ c ∈ l1.cells, c.isReady = true) ∧
      TopOK (s.drainFor s.topStart) ∧ ls1.map Layer.erase = ls.map Layer.erase ∧
      (s.drainFor s.topStart).base.map Cell.erase = s.base.map Cell.erase ∧
      (s.drainFor s.topStart).hashErr = s.hashErr ∧ (s.drainFor s.topStart).below.length = s.below.length ∧
      HolesSub s.below (s.drainFor s.topStart).below := by
  obtain ⟨hv, ht2, hh2, _⟩ := drainFor_spec s ht s.topStart
  obtain ⟨l2, ls2, hs2, hle, hle2⟩ := LayerLE.of_view hv h (drainFor_holesSub s ht.holes _)
  have htop : s.topStart = s.below.length := St.topStart_eq s l ls h
  have hbl := below_length_of_view hv h hs2
  have hr2 : ∀ c ∈ l2.cells, c.isReady = true := by
    intro c hc
    obtain ⟨i, hi, rfl⟩ := List.getElem_of_mem hc
    have hi' : (s.drainFor s.topStart).below.length + i < (s.drainFor s.topStart).cells.length := by
      rw [St.cells_top _ l2 ls2 hs2, List.length_append]; omega
    have := drainFor_no_holes s ht.holes s.topStart _ (by omega) hi'
    have hget : (s.drainFor s.topStart).cells[(s.drainFor s.topStart).below.length + i]? = some (l2.cells[i]) := by
      rw [St.cells_top _ l2 ls2 hs2, List.getElem?_append_right (by omega), Nat.add_sub_cancel_left,
        List.getElem?_eq_getElem hi]
    rw [List.getElem?_eq_getElem hi'] at hget
    simp only [Option.some.injEq] at hget
    rw [← hget]; exact this
  exact ⟨l2, ls2, hs2, hle, hr2, ht2, hle2, congrArg Prod.fst hv, hh2, hbl,
    holesSub_below hv h hs2 (drainFor_holesSub s ht.holes _)⟩

theorem map_depth_zero (ns : List (Node C)) (h : ∀ n ∈ ns, n.depth = 0) :
    ns.map (fun n => { n with depth := 0 }) = ns := by
  induction ns with
  | nil => rfl
  | cons n ns ih =>
    simp only [List.map_cons]
    rw [ih (fun m hm => h m (List.mem_cons_of_mem _ hm))]
    have : n.depth = 0 := h n (by simp)
    cases n; simp_all

/-- The layer `progInit` installs. -/
theorem progInit_eq (d : St C) (l : Layer C) (ls : List (Layer C)) (h : d.stack = l :: ls)
    (hleaves : l.collapsed = false → l.level = 0 ∧ ∀ n ∈ l.nodes, n.depth = 0) (hpl : l.prog.length = l.level) :
    d.progInit l = d.setTop { l with counted := l.nodes.length } := by
  unfold St.progInit
  have hnf : ¬ (l.collapsed = false ∧ (l.prog ≠ [] ∨ l.level ≠ 0)) := by
    intro ⟨hc, hp⟩
    obtain ⟨h0, _⟩ := hleaves hc
    rcases hp with hp | hp
    · exact hp (List.eq_nil_of_length_eq_zero (by omega))
    · exact hp h0
  simp only [Bool.not_eq_true'] at hnf ⊢
  rw [if_neg hnf]
  unfold St.uncollapsedAsLeaves
  cases hc : l.collapsed
  · obtain ⟨_, hd0⟩ := hleaves hc
    simp only [Bool.false_eq_true, if_false]
    rw [if_neg (by intro hany; obtain ⟨n, hn, hd⟩ := List.any_eq_true.mp hany; simp [hd0 n hn] at hd),
      map_depth_zero l.nodes hd0]
  · rfl

/-- **`progInit`** on a drained progressive scope: counts reinitialized,
    ready for the rounds. -/
theorem progInit_ok (cx : Ctx C) (d : St C) (l : Layer C) (ls : List (Layer C)) (G : List (List C)) (Lb : List C)
    (h : d.stack = l :: ls) (hp : ProgOK cx l G Lb) (hr : ∀ c ∈ l.cells, c.isReady = true) (hpend : l.pend = [])
    (ht : TopOK d) :
    (d.progInit l).stack = { l with counted := l.nodes.length } :: ls ∧
      RoundP cx { l with counted := l.nodes.length } G Lb ∧ TopOK (d.progInit l) ∧
      (d.progInit l).below = d.below ∧ (d.progInit l).base = d.base ∧ (d.progInit l).hashErr = d.hashErr := by
  rw [progInit_eq d l ls h hp.leaves (by rw [hp.groups.1, ← hp.level])]
  have htl : l.tail = [] := hp.pendOK.tail hpend
  refine ⟨St.setTop_stack d l _ ls h, ⟨hp.prog, hp.inc, hp.nolegacy, hpend, htl, hp.groups, hp.level, hp.cov,
    hp.narrow, ?_, rfl, hp.shallow⟩, topOK_setTop_same d l _ ls h ht rfl, St.setTop_below d l _ ls h,
    St.setTop_base d _, St.setTop_hashErr d _⟩
  intro n hn; apply hr; simp only [Layer.cells, List.mem_append, nodeCells, List.mem_map]
  exact Or.inl (Or.inl (Or.inr ⟨n, hn, rfl⟩))

/-! ## More shape facts -/

theorem collapseBinaryCore_top (cx : Ctx C) (t : St C) :
    TopShape (fun l => l.collapsed = true) (t.collapseBinaryCore cx) := by
  intro l2 ls2 h2
  unfold St.collapseBinaryCore at h2
  cases ht : t.stack with
  | nil => rw [ht] at h2; simp only at h2; rw [ht] at h2; cases h2
  | cons l ls =>
    rw [ht] at h2
    simp only at h2
    obtain ⟨_, _, u3, _⟩ := uncollapsedAsLeaves_sim t l
    obtain ⟨_, _, b3⟩ := batchLoop_sim cx 64 0 (t.uncollapsedAsLeaves l).1 (t.uncollapsedAsLeaves l).2.nodes
    rw [St.setTop_stack _ l _ ls (by rw [b3, u3, ht])] at h2
    obtain ⟨rfl, rfl⟩ := List.cons.inj h2
    rfl

/-- Batch collapse keeps a collapsed scope collapsed. -/
theorem collapseBinary_top (cx : Ctx C) (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls)
    (hc : l.collapsed = true) : TopShape (fun l => l.collapsed = true) (s.collapseBinary cx) := by
  unfold St.collapseBinary
  rw [h]
  simp only
  split
  · intro l2 ls2 h2; rw [St.fail_stack, h] at h2; obtain ⟨rfl, rfl⟩ := List.cons.inj h2; exact hc
  split
  · intro l2 ls2 h2; rw [h] at h2; obtain ⟨rfl, rfl⟩ := List.cons.inj h2; exact hc
  · exact collapseBinaryCore_top cx _

theorem setTop_self (s : St C) (l : Layer C) (ls : List (Layer C)) (h : s.stack = l :: ls) : s.setTop l = s := by
  apply St.ext_fields <;> simp [St.setTop, h]

/-- A deepest-first list of cap-depth nodes and leaves leads with the cap nodes. -/
theorem caps_then_leaves (P : Params) (l : Layer C) (hni : NonInc l.nodes) (hcd : 0 < P.capDepth)
    (hd : ∀ n ∈ l.nodes, n.depth = 0 ∨ n.depth = P.capDepth) :
    ((l.nodes.take (capCount P l)).all (·.depth == P.capDepth) &&
      (l.nodes.drop (capCount P l)).all (·.depth == 0)) = true := by
  have hsplit := nonInc_split3 P.capDepth l.nodes hni
  have hA : l.nodes.filter (·.depth > P.capDepth) = [] := List.filter_eq_nil_iff.mpr (fun n hn => by
    rcases hd n hn with h | h <;> simp [h])
  rw [hA, List.nil_append] at hsplit
  have hcc : capCount P l = (l.nodes.filter (·.depth == P.capDepth)).length := rfl
  have hM : ∀ n ∈ l.nodes.filter (·.depth == P.capDepth), n.depth = P.capDepth := fun n hn => by
    simpa using (List.mem_filter.mp hn).2
  have hB : ∀ n ∈ l.nodes.filter (·.depth < P.capDepth), n.depth = 0 := fun n hn => by
    have h1 := (List.mem_filter.mp hn).2; have h2 := hd n (List.mem_filter.mp hn).1; simp at h1; omega
  rw [hcc]
  generalize l.nodes.filter (·.depth == P.capDepth) = M at hsplit hM ⊢
  generalize l.nodes.filter (·.depth < P.capDepth) = B at hsplit hB
  rw [hsplit, List.take_left' rfl, List.drop_left' rfl]
  simp only [Bool.and_eq_true, List.all_eq_true]
  exact ⟨fun n hn => by simp [hM n hn], fun n hn => by simp [hB n hn]⟩

/-- **A pre-collapse that emits no job changes nothing** on a scope whose
    counts are fresh and whose region is ready. -/
theorem precollapse_false (cx : Ctx C) (p : St C) (l : Layer C) (ls : List (Layer C)) (h : p.stack = l :: ls)
    (hcd : 0 < cx.P.capDepth) (hni : NonInc l.nodes) (hcnt : l.counted = l.nodes.length)
    (hd0 : l.collapsed = false → ∀ n ∈ l.nodes, n.depth = 0) (hf : (p.precollapse cx).2 = false) :
    (p.precollapse cx).1 = p := by
  have hlay : (p.uncollapsedAsLeaves l).2 = l ∧ (p.uncollapsedAsLeaves l).1 = p := by
    unfold St.uncollapsedAsLeaves
    cases hc : l.collapsed
    · have hd := hd0 hc
      simp only [Bool.false_eq_true, if_false]
      rw [if_neg (by intro hany; obtain ⟨n, hn, hdn⟩ := List.any_eq_true.mp hany; simp [hd n hn] at hdn),
        map_depth_zero l.nodes hd, ← hcnt]
      refine ⟨?_, rfl⟩
      cases l; simp_all
    · simp only [if_true]; rw [← hcnt]
      refine ⟨?_, trivial⟩
      cases l; simp_all
  have hn : p.normTop = p := by
    unfold St.normTop; rw [h]; simp only; rw [hlay.1, hlay.2, setTop_self p l ls h]
  unfold St.precollapse at hf ⊢
  rw [hn, h] at hf ⊢
  simp only at hf ⊢
  split
  · rfl
  · rename_i hany
    rw [if_neg hany] at hf
    simp only [decide_eq_false_iff_not, Nat.not_le] at hf
    have hd : ∀ n ∈ l.nodes, n.depth = 0 ∨ n.depth = cx.P.capDepth := by
      intro n hn'
      by_cases h0 : n.depth = 0
      · exact Or.inl h0
      by_cases h1 : n.depth = cx.P.capDepth
      · exact Or.inr h1
      exfalso; apply hany; rw [List.any_eq_true]; exact ⟨n, hn', by simp [h0, h1]⟩
    rw [caps_then_leaves cx.P l hni hcd hd]
    simp only [St.failUnless, if_true, precollapseLoop, h]
    rw [if_pos hf]

end Ssz.Hasher