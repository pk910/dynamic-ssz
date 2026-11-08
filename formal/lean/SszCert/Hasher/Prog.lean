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

end Ssz.Hasher