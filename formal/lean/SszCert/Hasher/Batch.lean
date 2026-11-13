import SszCert.Hasher.State

/-!
# Batch collapse

`maybeCollapseBinary` hashes whole batches of the depth-`d` group in place
into nodes one deeper, for `d = 0, 1, …` while a depth holds a full batch.
Under a full cover the group sits between the deeper and the shallower
nodes, so a round keeps the cover full.
-/

namespace Ssz.Hasher

variable {C : Type} (H : C → C → C) (z0 : C)

/-- Full covers split at any point, empty parts allowed. -/
theorem cov_split_full {A B : List (Node C)} {L : List C} (h : Cov H z0 true (A ++ B) L) :
    ∃ L1 L2, L = L1 ++ L2 ∧ Cov H z0 true A L1 ∧ Cov H z0 true B L2 ∧
      (∀ (ha : A ≠ []) (hb : B ≠ []), (B.head hb).depth ≤ (A.getLast ha).depth) := by
  by_cases hA : A = []
  · subst hA; exact ⟨[], L, rfl, rfl, by simpa using h, fun ha => absurd rfl ha⟩
  by_cases hB : B = []
  · subst hB; exact ⟨L, [], by simp, by simpa using h, rfl, fun _ hb => absurd rfl hb⟩
  obtain ⟨L1, L2, hL, h1, h2, hs⟩ := cov_split H z0 hA hB h
  exact ⟨L1, L2, hL, h1, h2, fun _ _ => hs⟩

/-- Full covers join across a non-increasing seam, empty parts allowed. -/
theorem cov_join_full {A B : List (Node C)} {L1 L2 : List C} (h1 : Cov H z0 true A L1)
    (h2 : Cov H z0 true B L2) (hs : ∀ (ha : A ≠ []) (hb : B ≠ []), (B.head hb).depth ≤ (A.getLast ha).depth) :
    Cov H z0 true (A ++ B) (L1 ++ L2) := by
  by_cases hA : A = []
  · subst hA; have : L1 = [] := h1; subst this; simpa using h2
  by_cases hB : B = []
  · subst hB; have : L2 = [] := h2; subst this; simpa using h1
  exact cov_join H z0 hA hB h1 h2 (hs hA hB)

/-- A full cover around depth `d`: deeper nodes, the depth-`d` group, shallower nodes. -/
theorem cov_split3 {ns : List (Node C)} {L : List C} (d : Nat) (h : Cov H z0 true ns L) :
    ∃ A G B, ns = A ++ G ++ B ∧ (∀ n ∈ A, d < n.depth) ∧ (∀ n ∈ G, n.depth = d) ∧
      (∀ n ∈ B, n.depth < d) := by
  induction ns generalizing L with
  | nil => exact ⟨[], [], [], rfl, by simp, by simp, by simp⟩
  | cons n rest ih =>
    have hrest : ∃ L', Cov H z0 true rest L' := by
      cases rest with
      | nil => exact ⟨[], rfl⟩
      | cons m rest => obtain ⟨_, ys, _, _, _, _, hc⟩ := h; exact ⟨ys, hc⟩
    obtain ⟨L', hc⟩ := hrest
    obtain ⟨A, G, B, heq, hA, hG, hB⟩ := ih hc
    have hle : ∀ k ∈ rest, k.depth ≤ n.depth := cov_depth_le H z0 h
    rcases Nat.lt_trichotomy d n.depth with hlt | heq' | hgt
    · exact ⟨n :: A, G, B, by simp [heq], fun k hk => by
        rcases List.mem_cons.mp hk with rfl | hk; exact hlt; exact hA k hk, hG, hB⟩
    · -- n is in the group: nothing deeper follows
      have hA0 : A = [] := by
        cases A with
        | nil => rfl
        | cons a as =>
          have := hA a (by simp); have := hle a (by rw [heq]; simp); omega
      subst hA0
      exact ⟨[], n :: G, B, by simp [heq], by simp, fun k hk => by
        rcases List.mem_cons.mp hk with rfl | hk; exact heq'.symm; exact hG k hk, hB⟩
    · -- n is shallower: so is everything after it
      have hA0 : A = [] := by
        cases A with
        | nil => rfl
        | cons a as => have := hA a (by simp); have := hle a (by rw [heq]; simp); omega
      have hG0 : G = [] := by
        cases G with
        | nil => rfl
        | cons g gs => have := hG g (by simp); have := hle g (by rw [heq]; simp); omega
      subst hA0; subst hG0
      exact ⟨[], [], n :: B, by simp [heq], by simp, by simp, fun k hk => by
        rcases List.mem_cons.mp hk with rfl | hk; exact hgt; exact hB k hk⟩

theorem filter_gt_split {A G B : List (Node C)} {d : Nat} (hA : ∀ n ∈ A, d < n.depth)
    (hG : ∀ n ∈ G, n.depth = d) (hB : ∀ n ∈ B, n.depth < d) :
    ((A ++ G ++ B).filter (·.depth > d)).length = A.length := by
  rw [List.filter_append, List.filter_append]
  rw [List.filter_eq_self.mpr (fun n hn => by simp; exact hA n hn),
    List.filter_eq_nil_iff.mpr (fun n hn => by simp; have := hG n hn; omega),
    List.filter_eq_nil_iff.mpr (fun n hn => by simp; have := hB n hn; omega)]
  simp

theorem filter_eq_split {A G B : List (Node C)} {d : Nat} (hA : ∀ n ∈ A, d < n.depth)
    (hG : ∀ n ∈ G, n.depth = d) (hB : ∀ n ∈ B, n.depth < d) :
    ((A ++ G ++ B).filter (·.depth == d)).length = G.length := by
  rw [List.filter_append, List.filter_append]
  rw [List.filter_eq_nil_iff.mpr (fun n hn => by simp; have := hA n hn; omega),
    List.filter_eq_self.mpr (fun n hn => by simp [hG n hn]),
    List.filter_eq_nil_iff.mpr (fun n hn => by simp; have := hB n hn; omega)]
  simp

theorem leavesOf_uniform {G : List (Node C)} {d : Nat} (hG : ∀ n ∈ G, n.depth = d) :
    leavesOf G = G.length * 2 ^ d := by
  induction G with
  | nil => simp [leavesOf]
  | cons n G ih =>
    simp only [leavesOf, List.map_cons, List.sum_cons, List.length_cons] at ih ⊢
    rw [hG n (by simp), ih (fun k hk => hG k (List.mem_cons_of_mem _ hk)), Nat.succ_mul]; omega

/-- A group of `2^d`-nodes whose count is even pairs into a full group one deeper. -/
theorem pair_full_group {d : Nat} {G : List (Node C)} {L : List C} (hG : ∀ n ∈ G, n.depth = d)
    (hne : G ≠ []) (hev : G.length % 2 = 0) (hc : Cov H z0 true G L) :
    Cov H z0 true ((pairUp H (G.map (·.cell.val))).map (fun v => (⟨d + 1, .ready v⟩ : Node C))) L := by
  obtain ⟨hv, _⟩ := cov_group H z0 hG hc
  have hlen : L.length = G.length * 2 ^ d := by rw [cov_full_length H z0 hc, leavesOf_uniform hG]
  have hLne : L ≠ [] := cov_ne_nil H z0 hc hne
  have hev' : (G.map (·.cell.val)).length % 2 = 0 := by simpa using hev
  apply group_cov H z0 (fun n hn => by simp only [List.mem_map] at hn; obtain ⟨_, _, rfl⟩ := hn; rfl) hLne
  · simp only [List.map_map]
    rw [show ((fun x : Node C => x.cell.val) ∘ fun v => (⟨d + 1, .ready v⟩ : Node C)) = id from rfl,
      List.map_id, ← levelUp_even H (zeroH H z0 d) _ hev', hv, levelUp_chunks]
  · intro _
    rw [hlen]
    obtain ⟨k, hk⟩ : ∃ k, G.length = 2 * k := ⟨G.length / 2, by omega⟩
    rw [hk, Nat.pow_succ, show 2 * k * 2 ^ d = k * (2 ^ d * 2) by
      rw [Nat.mul_comm 2 k, Nat.mul_assoc, Nat.mul_comm 2 (2 ^ d)], Nat.mul_mod_left]

/-- **A batch round keeps a full cover.** -/
theorem batchStep_cov (cx : Ctx C) (d : Nat) (s : St C) (ns : List (Node C)) (L : List C)
    (hcov : Cov cx.H cx.z0 true ns L) (hb : 0 < cx.P.batch) (hb2 : cx.P.batch % 2 = 0)
    (hcnt : cx.P.batch ≤ (ns.filter (·.depth == d)).length) :
    Cov cx.H cx.z0 true (batchStep cx d s ns).2 L := by
  obtain ⟨A, G, B, rfl, hA, hG, hB⟩ := cov_split3 cx.H cx.z0 d hcov
  rw [filter_eq_split hA hG hB] at hcnt
  have hdc : deeperCount d (A ++ G ++ B) = A.length := filter_gt_split hA hG hB
  have hbs : batchSize cx.P d (A ++ G ++ B) = (G.length / cx.P.batch) * cx.P.batch := by
    unfold batchSize; rw [filter_eq_split hA hG hB]
  generalize hbc : (G.length / cx.P.batch) * cx.P.batch = bc at hbs
  have hbcle : bc ≤ G.length := by rw [← hbc]; exact Nat.div_mul_le_self _ _
  have hbcpos : 0 < bc := by
    rw [← hbc]; have : 1 ≤ G.length / cx.P.batch := (Nat.le_div_iff_mul_le hb).mpr (by omega)
    exact Nat.mul_pos (by omega) hb
  have hbcev : bc % 2 = 0 := by
    rw [← hbc]; obtain ⟨k, hk⟩ : ∃ k, cx.P.batch = 2 * k := ⟨cx.P.batch / 2, by omega⟩
    rw [hk, ← Nat.mul_assoc, Nat.mul_comm _ 2, Nat.mul_assoc]; simp
  have hdrop : (A ++ G ++ B).drop A.length = G ++ B := by simp
  have hgrp : batchGroup cx.P d (A ++ G ++ B) = G.take bc := by
    unfold batchGroup; rw [hdc, hbs, hdrop, List.take_append_of_le_length hbcle]
  have hrest : ((A ++ G ++ B).drop A.length).drop bc = G.drop bc ++ B := by
    rw [hdrop, List.drop_append_of_le_length hbcle]
  have htake : (A ++ G ++ B).take A.length = A := by simp
  have hG1 : ∀ n ∈ G.take bc, n.depth = d := fun n hn => hG n (List.mem_of_mem_take hn)
  have hG1l : (G.take bc).length = bc := by rw [List.length_take]; omega
  have hG1ne : G.take bc ≠ [] := by
    intro e; rw [e] at hG1l; simp at hG1l; omega
  have hG1ev : (G.take bc).length % 2 = 0 := by rw [hG1l]; exact hbcev
  unfold batchStep
  simp only
  rw [hgrp, hdc, htake, hbs, hrest, pairNodes_group cx _ hG1ne hG1]
  -- covers of the parts
  have hcov' : Cov cx.H cx.z0 true (A ++ (G.take bc ++ (G.drop bc ++ B))) L := by
    have e : G.take bc ++ (G.drop bc ++ B) = G ++ B := by rw [← List.append_assoc, List.take_append_drop]
    rw [e, ← List.append_assoc]; exact hcov
  obtain ⟨LA, LR, hL, hcA, hcR, hsA⟩ := cov_split_full cx.H cx.z0 hcov'
  obtain ⟨LG1, LR2, hLR, hcG1, hcR2, _⟩ := cov_split_full cx.H cx.z0 hcR
  have hpc := pair_full_group cx.H cx.z0 hG1 hG1ne hG1ev hcG1
  have hpdep : ∀ n ∈ (pairUp cx.H ((G.take bc).map (·.cell.val))).map
      (fun v => (⟨d + 1, .ready v⟩ : Node C)), n.depth = d + 1 := by
    intro n hn; simp only [List.mem_map] at hn; obtain ⟨_, _, rfl⟩ := hn; rfl
  rw [hL, hLR, List.append_assoc]
  apply cov_join_full cx.H cx.z0 hcA (cov_join_full cx.H cx.z0 hpc hcR2 ?_) ?_
  · intro hp hr
    rw [hpdep _ (List.getLast_mem hp)]
    have hm := List.head_mem hr
    rcases List.mem_append.mp hm with hm | hm
    · have := hG _ (List.mem_of_mem_drop hm); omega
    · have := hB _ hm; omega
  · intro ha hpr
    have hmem := List.head_mem hpr
    rcases List.mem_append.mp hmem with hm | hm
    · rw [hpdep _ hm]; exact hA _ (List.getLast_mem ha)
    · rcases List.mem_append.mp hm with hm | hm
      · have := hG _ (List.mem_of_mem_drop hm); have := hA _ (List.getLast_mem ha); omega
      · have := hB _ hm; have := hA _ (List.getLast_mem ha); omega

/-- **Batch rounds keep a full cover.** -/
theorem batchLoop_cov (cx : Ctx C) (hb : 0 < cx.P.batch) (hb2 : cx.P.batch % 2 = 0) :
    ∀ fuel d (s : St C) (ns : List (Node C)) (L : List C), Cov cx.H cx.z0 true ns L →
      Cov cx.H cx.z0 true (batchLoop cx fuel d s ns).2 L := by
  intro fuel
  induction fuel with
  | zero => intro d s ns L h; exact h
  | succ fuel ih =>
    intro d s ns L h
    simp only [batchLoop]
    split
    · exact h
    · rename_i hc
      exact ih _ _ _ L (batchStep_cov cx d s ns L h hb hb2 (by omega))

/-- Treating the nodes of an uncollapsed scope as leaves changes nothing when
    they are leaves. -/
theorem uncollapsedAsLeaves_nodes (s : St C) (l : Layer C)
    (h : l.collapsed = false → ∀ n ∈ l.nodes, n.depth = 0) :
    (s.uncollapsedAsLeaves l).2.nodes = l.nodes := by
  unfold St.uncollapsedAsLeaves
  split
  · rfl
  · rename_i hc
    simp only
    have h' := h (by simpa using hc)
    conv => rhs; rw [← List.map_id l.nodes]
    apply List.map_congr_left
    intro n hn
    have := h' n hn
    cases n; simp_all

theorem uncollapsedAsLeaves_layer (s : St C) (l : Layer C) :
    (s.uncollapsedAsLeaves l).2 = { l with nodes := (s.uncollapsedAsLeaves l).2.nodes, counted := l.nodes.length } := by
  unfold St.uncollapsedAsLeaves; split <;> simp

end Ssz.Hasher