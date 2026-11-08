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

end Ssz.Hasher