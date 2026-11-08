import SszCert.Hasher.Spec

/-!
# Node covers

A collapse region of the hasher is a deepest-first list of nodes, each the
root of a consecutive slice of the scope's leaves. `Cov` states exactly that,
and the lemmas here show the region operations keep it.
-/

namespace Ssz.Hasher

variable {C : Type} (H : C → C → C) (z0 : C)

/-- `Cov full ns L`: the nodes `ns`, depths non-increasing, cover the leaves
    `L` in order; each holds the root of its non-empty slice at its depth.
    Every slice but the last holds exactly `2^depth` leaves; the last one
    too when `full`, otherwise at most that many. -/
def Cov (full : Bool) : List (Node C) → List C → Prop
  | [], L => L = []
  | [n], L => L ≠ [] ∧ (if full then L.length = 2 ^ n.depth else L.length ≤ 2 ^ n.depth) ∧
      n.cell.val = mroot H z0 n.depth L
  | n :: m :: ns, L => ∃ xs ys, L = xs ++ ys ∧ xs.length = 2 ^ n.depth ∧
      n.cell.val = mroot H z0 n.depth xs ∧ m.depth ≤ n.depth ∧ Cov full (m :: ns) ys

theorem cov_nil {full : Bool} {L : List C} : Cov H z0 full [] L ↔ L = [] := Iff.rfl

theorem cov_single {full : Bool} {n : Node C} {L : List C} :
    Cov H z0 full [n] L ↔ L ≠ [] ∧ (if full then L.length = 2 ^ n.depth else L.length ≤ 2 ^ n.depth) ∧
      n.cell.val = mroot H z0 n.depth L := Iff.rfl

theorem cov_cons2 {full : Bool} {n m : Node C} {ns : List (Node C)} {L : List C} :
    Cov H z0 full (n :: m :: ns) L ↔ ∃ xs ys, L = xs ++ ys ∧ xs.length = 2 ^ n.depth ∧
      n.cell.val = mroot H z0 n.depth xs ∧ m.depth ≤ n.depth ∧ Cov H z0 full (m :: ns) ys := Iff.rfl

/-- A full cover is a cover. -/
theorem cov_of_full {ns : List (Node C)} {L : List C} (h : Cov H z0 true ns L) : Cov H z0 false ns L := by
  induction ns generalizing L with
  | nil => exact h
  | cons n ns ih =>
    cases ns with
    | nil =>
      obtain ⟨hne, hlen, hv⟩ := h
      exact ⟨hne, by simp at hlen ⊢; omega, hv⟩
    | cons m ns =>
      obtain ⟨xs, ys, hL, hx, hv, hd, hc⟩ := h
      exact ⟨xs, ys, hL, hx, hv, hd, ih hc⟩

/-- The leaves of a cover: one per unit of `2^depth` (full covers). -/
theorem cov_full_length {ns : List (Node C)} {L : List C} (h : Cov H z0 true ns L) :
    L.length = leavesOf ns := by
  induction ns generalizing L with
  | nil => cases h; rfl
  | cons n ns ih =>
    cases ns with
    | nil => obtain ⟨_, hlen, _⟩ := h; simp at hlen; simp [leavesOf, hlen]
    | cons m ns =>
      obtain ⟨xs, ys, hL, hx, _, _, hc⟩ := h
      have := ih hc
      subst hL
      simp only [leavesOf, List.map_cons, List.sum_cons, List.length_append] at this ⊢
      omega

/-- A non-empty cover covers something. -/
theorem cov_ne_nil {full : Bool} {ns : List (Node C)} {L : List C} (h : Cov H z0 full ns L)
    (hne : ns ≠ []) : L ≠ [] := by
  cases ns with
  | nil => exact absurd rfl hne
  | cons n ns =>
    cases ns with
    | nil => exact h.1
    | cons m ns =>
      obtain ⟨xs, ys, hL, hx, _⟩ := h
      intro e; rw [hL] at e; have := congrArg List.length e
      rw [List.length_append, hx] at this; simp at this

/-- The head depth bounds every depth of a cover. -/
theorem cov_depth_le {full : Bool} {n : Node C} {ns : List (Node C)} {L : List C}
    (h : Cov H z0 full (n :: ns) L) : ∀ m ∈ ns, m.depth ≤ n.depth := by
  induction ns generalizing n L with
  | nil => simp
  | cons m ns ih =>
    obtain ⟨xs, ys, _, _, _, hd, hc⟩ := h
    intro k hk
    rcases List.mem_cons.mp hk with rfl | hk
    · exact hd
    · exact Nat.le_trans (ih hc k hk) hd

/-- **Appending a leaf** to a full cover. -/
theorem cov_push {ns : List (Node C)} {L : List C} (h : Cov H z0 true ns L) (c : Cell C) :
    Cov H z0 true (ns ++ [⟨0, c⟩]) (L ++ [c.val]) := by
  induction ns generalizing L with
  | nil =>
    cases h
    exact ⟨by simp, by simp, rfl⟩
  | cons n ns ih =>
    cases ns with
    | nil =>
      obtain ⟨_, hlen, hv⟩ := h
      simp at hlen
      exact ⟨L, [c.val], rfl, hlen, hv, Nat.zero_le _, ⟨by simp, by simp, rfl⟩⟩
    | cons m ns =>
      obtain ⟨xs, ys, hL, hx, hv, hd, hc⟩ := h
      exact ⟨xs, ys ++ [c.val], by rw [hL, List.append_assoc], hx, hv, hd, ih hc⟩

/-! ## Splitting and joining covers -/

/-- A cover of `A ++ B` (both non-empty) is a full cover of `A` followed by a
    cover of `B`, the depth not increasing across the seam. -/
theorem cov_split {full : Bool} {A B : List (Node C)} {L : List C} (hA : A ≠ []) (hB : B ≠ [])
    (h : Cov H z0 full (A ++ B) L) :
    ∃ L1 L2, L = L1 ++ L2 ∧ Cov H z0 true A L1 ∧ Cov H z0 full B L2 ∧
      (B.head hB).depth ≤ (A.getLast hA).depth := by
  induction A generalizing L with
  | nil => exact absurd rfl hA
  | cons n A ih =>
    cases A with
    | nil =>
      cases B with
      | nil => exact absurd rfl hB
      | cons m B =>
        obtain ⟨xs, ys, hL, hx, hv, hd, hc⟩ := h
        refine ⟨xs, ys, hL, ⟨?_, by simpa using hx, hv⟩, hc, by simpa using hd⟩
        intro e; subst e; simp at hx; have := Nat.one_le_two_pow (n := n.depth); omega
    | cons a A =>
      obtain ⟨xs, ys, hL, hx, hv, hd, hc⟩ := h
      obtain ⟨L1, L2, h12, hA', hB', hseam⟩ := ih (by simp) hc
      refine ⟨xs ++ L1, L2, by rw [hL, h12, List.append_assoc], ⟨xs, L1, rfl, hx, hv, hd, hA'⟩, hB', ?_⟩
      simpa using hseam

/-- Joining a full cover and a cover across a non-increasing seam. -/
theorem cov_join {full : Bool} {A B : List (Node C)} {L1 L2 : List C} (hA : A ≠ []) (hB : B ≠ [])
    (h1 : Cov H z0 true A L1) (h2 : Cov H z0 full B L2) (hseam : (B.head hB).depth ≤ (A.getLast hA).depth) :
    Cov H z0 full (A ++ B) (L1 ++ L2) := by
  induction A generalizing L1 with
  | nil => exact absurd rfl hA
  | cons n A ih =>
    cases A with
    | nil =>
      cases B with
      | nil => exact absurd rfl hB
      | cons m B =>
        obtain ⟨_, hlen, hv⟩ := h1
        simp at hlen
        exact ⟨L1, L2, rfl, hlen, hv, by simpa using hseam, h2⟩
    | cons a A =>
      obtain ⟨xs, ys, hL, hx, hv, hd, hc⟩ := h1
      subst hL
      exact ⟨xs, ys ++ L2, by rw [List.append_assoc], hx, hv, hd, ih (by simp) hc (by simpa using hseam)⟩

/-! ## Same-depth groups are chunk roots -/

/-- A same-depth group covering `L` holds the roots of the `2^d`-pieces of `L`. -/
theorem cov_group {full : Bool} {d : Nat} {G : List (Node C)} {L : List C} (hG : ∀ n ∈ G, n.depth = d)
    (h : Cov H z0 full G L) :
    G.map (·.cell.val) = (chunks d L).map (mroot H z0 d) ∧ (full = true → L.length % 2 ^ d = 0) := by
  induction G generalizing L with
  | nil => cases h; simp
  | cons n G ih =>
    have hn : n.depth = d := hG n (by simp)
    cases G with
    | nil =>
      obtain ⟨hne, hlen, hv⟩ := h
      rw [hn] at hlen hv
      refine ⟨?_, ?_⟩
      · rw [chunks_single hne (by cases full <;> simp_all <;> omega)]; simp [hv]
      · intro hf; subst hf; simp at hlen; rw [hlen]; simp
    | cons m G =>
      obtain ⟨xs, ys, hL, hx, hv, _, hc⟩ := h
      rw [hn] at hx hv
      obtain ⟨ihv, ihf⟩ := ih (fun k hk => hG k (List.mem_cons_of_mem _ hk)) hc
      have hne : L ≠ [] := by
        intro e; rw [hL] at e; have := congrArg List.length e
        rw [List.length_append, hx] at this; simp at this
      refine ⟨?_, ?_⟩
      · rw [chunks_of_ne_nil hne, hL, List.take_append_of_le_length (by omega),
          List.drop_append_of_le_length (by omega), List.take_of_length_le (by omega),
          List.drop_of_length_le (by omega), List.nil_append]
        simp only [List.map_cons, hv, ihv]
      · intro hf
        have := ihf hf
        rw [hL, List.length_append, hx, Nat.add_mod_left]
        exact this

/-- A list of nodes holding the roots of the `2^d`-pieces of a non-empty `L`
    covers it (fully, when `L` is a whole number of pieces). -/
theorem group_cov {full : Bool} {d : Nat} {G : List (Node C)} {L : List C} (hG : ∀ n ∈ G, n.depth = d)
    (hne : L ≠ []) (hv : G.map (·.cell.val) = (chunks d L).map (mroot H z0 d))
    (hf : full = true → L.length % 2 ^ d = 0) : Cov H z0 full G L := by
  induction G generalizing L with
  | nil =>
    rw [chunks_of_ne_nil hne] at hv; simp at hv
  | cons n G ih =>
    have hn : n.depth = d := hG n (by simp)
    rw [chunks_of_ne_nil hne] at hv
    simp only [List.map_cons, List.cons.injEq] at hv
    obtain ⟨hv1, hv2⟩ := hv
    have hpos := Nat.one_le_two_pow (n := d)
    cases G with
    | nil =>
      -- the rest is empty, so L is a single piece
      have hd : L.drop (2 ^ d) = [] := by
        cases hdl : L.drop (2 ^ d) with
        | nil => rfl
        | cons a r => rw [hdl, chunks_of_ne_nil (by simp)] at hv2; simp at hv2
      have hlen : L.length ≤ 2 ^ d := by
        have := congrArg List.length hd; simp at this; omega
      refine ⟨hne, ?_, ?_⟩
      · rw [hn]; cases full
        · simpa using hlen
        · have := hf rfl; simp
          have hp : 0 < L.length := List.length_pos_iff.mpr hne
          rcases Nat.lt_or_ge L.length (2 ^ d) with hl | hl
          · rw [Nat.mod_eq_of_lt hl] at this; omega
          · omega
      · rw [hn, hv1, List.take_of_length_le hlen]
    | cons m G =>
      have hmd : m.depth = d := hG m (by simp)
      have hdne : L.drop (2 ^ d) ≠ [] := by
        intro e; rw [e] at hv2; simp at hv2
      have hlen : 2 ^ d ≤ L.length := by
        rcases Nat.lt_or_ge L.length (2 ^ d) with hl | hl
        · exact absurd (List.drop_of_length_le (Nat.le_of_lt hl)) hdne
        · exact hl
      refine ⟨L.take (2 ^ d), L.drop (2 ^ d), (List.take_append_drop _ _).symm, ?_, ?_, ?_, ?_⟩
      · rw [hn]; simp; omega
      · rw [hn, hv1]
      · rw [hmd, hn]; omega
      · apply ih (fun k hk => hG k (List.mem_cons_of_mem _ hk)) hdne hv2
        intro hfull; have := hf hfull; simp
        have e : L.length = 2 ^ d + (L.length - 2 ^ d) := by omega
        rw [e, Nat.add_mod_left] at this; exact this

/-! ## The trailing group -/

/-- A cover ends in its lowest-depth group; everything before is deeper. -/
theorem trailing_split {full : Bool} {ns : List (Node C)} {L : List C} (h : Cov H z0 full ns L)
    (hne : ns ≠ []) :
    ∃ keep grp low, ns = keep ++ grp ∧ grp ≠ [] ∧ (∀ n ∈ grp, n.depth = low) ∧
      (∀ n ∈ keep, low < n.depth) := by
  induction ns generalizing L with
  | nil => exact absurd rfl hne
  | cons n rest ih =>
    cases rest with
    | nil => exact ⟨[], [n], n.depth, rfl, by simp, by simp, by simp⟩
    | cons m rest =>
      obtain ⟨xs, ys, _, _, _, hd, hc⟩ := h
      obtain ⟨keep, grp, low, heq, hg, hgd, hkd⟩ := ih hc (by simp)
      have hle : ∀ k ∈ m :: rest, k.depth ≤ n.depth := by
        intro k hk
        rcases List.mem_cons.mp hk with rfl | hk
        · exact hd
        · exact Nat.le_trans (cov_depth_le H z0 hc k hk) hd
      by_cases hlow : n.depth = low
      · -- then keep is empty: its depths would exceed low = n.depth
        have hk : keep = [] := by
          cases keep with
          | nil => rfl
          | cons k ks =>
            have := hkd k (by simp)
            have := hle k (by rw [heq]; simp)
            omega
        subst hk
        exact ⟨[], n :: grp, low, by simp [heq], by simp,
          fun k hk => by rcases List.mem_cons.mp hk with rfl | hk; exact hlow; exact hgd k hk, by simp⟩
      · have : low < n.depth := by
          obtain ⟨g, hgm⟩ := List.exists_mem_of_ne_nil grp hg
          have := hle g (by rw [heq]; simp [hgm])
          have := hgd g hgm
          omega
        exact ⟨n :: keep, grp, low, by simp [heq], hg, hgd,
          fun k hk => by rcases List.mem_cons.mp hk with rfl | hk; exact this; exact hkd k hk⟩

theorem min_depth_split {keep grp : List (Node C)} {low : Nat} (hg : grp ≠ [])
    (hgd : ∀ n ∈ grp, n.depth = low) (hkd : ∀ n ∈ keep, low < n.depth) :
    ((keep ++ grp).map (·.depth)).min? = some low := by
  rw [List.min?_eq_some_iff]
  refine ⟨?_, ?_⟩
  · obtain ⟨g, hgm⟩ := List.exists_mem_of_ne_nil grp hg
    exact List.mem_map.mpr ⟨g, by simp [hgm], hgd g hgm⟩
  · intro b hb
    obtain ⟨k, hk, rfl⟩ := List.mem_map.mp hb
    rcases List.mem_append.mp hk with hk | hk
    · exact Nat.le_of_lt (hkd k hk)
    · exact Nat.le_of_eq (hgd k hk).symm

theorem count_low_split {keep grp : List (Node C)} {low : Nat}
    (hgd : ∀ n ∈ grp, n.depth = low) (hkd : ∀ n ∈ keep, low < n.depth) :
    ((keep ++ grp).filter (·.depth == low)).length = grp.length := by
  rw [List.filter_append, List.length_append]
  have h1 : keep.filter (·.depth == low) = [] := by
    rw [List.filter_eq_nil_iff]; intro k hk; have := hkd k hk; simp; omega
  have h2 : grp.filter (·.depth == low) = grp := by
    rw [List.filter_eq_self]; intro k hk; simp [hgd k hk]
  rw [h1, h2]; simp

theorem all_low_split {keep grp : List (Node C)} {low : Nat}
    (hgd : ∀ n ∈ grp, n.depth = low) (hkd : ∀ n ∈ keep, low < n.depth) :
    (keep ++ grp).all (fun n => n.depth == low || n.depth < low) = keep.isEmpty := by
  cases keep with
  | nil =>
    simp only [List.nil_append, List.isEmpty_nil, List.all_eq_true]
    intro k hk; simp [hgd k hk]
  | cons k ks =>
    simp only [List.isEmpty_cons, List.all_eq_false]
    refine ⟨k, by simp, ?_⟩
    have := hkd k (by simp); simp; omega

/-- `pairNodes` on a same-depth group: one level up, pairs hashed. -/
theorem pairNodes_group (cx : Ctx C) (s : St C) {grp : List (Node C)} {low : Nat} (hg : grp ≠ [])
    (hgd : ∀ n ∈ grp, n.depth = low) :
    (s.pairNodes cx grp).2 = (pairUp cx.H (grp.map (·.cell.val))).map (fun v => ⟨low + 1, .ready v⟩) := by
  obtain ⟨g, gs, rfl⟩ := List.exists_cons_of_ne_nil hg
  simp only [St.pairNodes, St.vals, nodeCells, List.map_map, List.head?_cons, Option.map_some,
    Option.getD_some, hgd g (by simp)]
  rfl

/-! ## `collapseAllDepths` -/

/-- `pairUp` after the model's odd padding is one reduction level. -/
theorem pairUp_pad_eq_levelUp (z : C) (vs : List C) :
    pairUp H (if vs.length % 2 = 1 then vs ++ [z] else vs) = levelUp H z vs := rfl

/-- Two or more pieces mean more leaves than one piece holds. -/
theorem lt_of_two_chunks {k : Nat} {L : List C} (h : 2 ≤ (chunks k L).length) : 2 ^ k < L.length := by
  have hne : L ≠ [] := by intro e; subst e; simp at h
  rw [chunks_of_ne_nil hne] at h
  simp at h
  rcases Nat.lt_or_ge (2 ^ k) L.length with hl | hl
  · exact hl
  · rw [List.drop_of_length_le hl] at h; simp at h

/-- The number of pieces of a non-empty list fitting one is one. -/
theorem chunks_length_le_one {k : Nat} {L : List C} (hne : L ≠ []) (h : L.length ≤ 2 ^ k) :
    (chunks k L).length = 1 := by rw [chunks_single hne h]; rfl

theorem pairUp_length : ∀ xs : List C, (pairUp H xs).length = xs.length / 2
  | [] => by simp [pairUp]
  | [_] => by simp [pairUp]
  | a :: b :: rest => by simp only [pairUp, List.length_cons, pairUp_length rest]; omega

theorem levelUp_length (z : C) (xs : List C) : (levelUp H z xs).length = (xs.length + 1) / 2 := by
  unfold levelUp
  split
  · rw [pairUp_length, List.length_append, List.length_singleton]
  · rw [pairUp_length]; omega

/-- Depth of the first node (`0` for none). -/
def headDepth (ns : List (Node C)) : Nat := (ns.head?.map (·.depth)).getD 0

/-- Depth of the last node (`0` for none). -/
def lastDepth (ns : List (Node C)) : Nat := (ns.getLast?.map (·.depth)).getD 0

theorem headDepth_append {A B : List (Node C)} (h : A ≠ []) : headDepth (A ++ B) = headDepth A := by
  obtain ⟨a, as, rfl⟩ := List.exists_cons_of_ne_nil h; rfl

theorem headDepth_nil_append (B : List (Node C)) : headDepth ([] ++ B) = headDepth B := rfl

theorem lastDepth_append {A B : List (Node C)} (h : B ≠ []) : lastDepth (A ++ B) = lastDepth B := by
  unfold lastDepth
  rw [List.getLast?_append, List.getLast?_eq_some_getLast h]; rfl

theorem headDepth_uniform {G : List (Node C)} {d : Nat} (h : G ≠ []) (hd : ∀ n ∈ G, n.depth = d) :
    headDepth G = d := by
  obtain ⟨a, as, rfl⟩ := List.exists_cons_of_ne_nil h; exact hd a (by simp)

theorem lastDepth_uniform {G : List (Node C)} {d : Nat} (h : G ≠ []) (hd : ∀ n ∈ G, n.depth = d) :
    lastDepth G = d := by
  unfold lastDepth; rw [List.getLast?_eq_some_getLast h]; exact hd _ (List.getLast_mem h)

/-- The depth of `n` leaves strictly between two powers. -/
theorem depthOf_of_between {k n : Nat} (h1 : 2 ^ k < n) (h2 : n ≤ 2 ^ (k + 1)) : depthOf n = k + 1 := by
  have hpos := Nat.one_le_two_pow (n := k)
  unfold depthOf
  rw [if_neg (by omega)]
  have hne : n - 1 ≠ 0 := by omega
  have lo : k ≤ (n - 1).log2 := (Nat.le_log2 hne).mpr (by omega)
  have hi : (n - 1).log2 < k + 1 := (Nat.log2_lt hne).mpr (by omega)
  omega

/-- The collapse measure: nodes, plus the depth still to climb from the
    lowest to the deepest. -/
def collapseMeasure (ns : List (Node C)) : Nat :=
  ns.length + (headDepth ns - lastDepth ns)

/-- **The `collapseAllDepths` loop** reduces any cover to a single node: the
    root of all leaves at the depth they need. -/
theorem collapseLoop_cov (cx : Ctx C) (live : Bool) :
    ∀ fuel (s : St C) (ns : List (Node C)) (L : List C), Cov cx.H cx.z0 false ns L → ns ≠ [] →
      collapseMeasure ns ≤ fuel →
      (∀ m, ns = [m] → m.depth = depthOf L.length) →
      ∃ n, (collapseLoop cx live fuel s ns).2 = [n] ∧ Cov cx.H cx.z0 false [n] L ∧
        n.depth = depthOf L.length := by
  intro fuel
  induction fuel with
  | zero =>
    intro s ns L _ hne hm _
    have : ns.length = 0 := by unfold collapseMeasure at hm; omega
    exact absurd (List.eq_nil_of_length_eq_zero this) hne
  | succ fuel ih =>
    intro s ns L hcov hne hm hgood
    obtain ⟨keep, grp, low, rfl, hg, hgd, hkd⟩ := trailing_split cx.H cx.z0 hcov hne
    have hm0 : collapseMeasure (keep ++ grp) ≤ fuel + 1 := hm
    simp only [collapseLoop, min_depth_split hg hgd hkd, count_low_split hgd hkd,
      all_low_split hgd hkd]
    by_cases hstop : grp.length = 1 ∧ keep.isEmpty = true
    · -- a single node remains
      rw [if_pos hstop]
      obtain ⟨hl1, hke⟩ := hstop
      have hk : keep = [] := List.isEmpty_iff.mp hke
      obtain ⟨g, rfl⟩ : ∃ g, grp = [g] := by
        cases grp with
        | nil => simp at hl1
        | cons g gs => cases gs with
          | nil => exact ⟨g, rfl⟩
          | cons _ _ => simp at hl1
      subst hk
      exact ⟨g, rfl, hcov, hgood g rfl⟩
    rw [if_neg hstop]
    -- the padded group
    let pad : List (Node C) := if grp.length % 2 = 1 then [⟨low, .ready (zeroH cx.H cx.z0 low)⟩] else []
    have hns2 : (if grp.length % 2 = 1 then keep ++ grp ++ [⟨low, .ready (zeroH cx.H cx.z0 low)⟩]
        else keep ++ grp) = keep ++ (grp ++ pad) := by
      simp only [pad]; split <;> simp
    have hcnt2 : (if grp.length % 2 = 1 then grp.length + 1 else grp.length) = (grp ++ pad).length := by
      simp only [pad]; split <;> simp
    simp only [hns2, hcnt2]
    have hlen : (keep ++ (grp ++ pad)).length - (grp ++ pad).length = keep.length := by simp
    rw [hlen, List.take_left' rfl, List.drop_left' rfl]
    have hpadd : ∀ n ∈ pad, n.depth = low := by
      intro n hn; simp only [pad] at hn; split at hn
      · simp at hn; rw [hn]
      · simp at hn
    have hpd : ∀ n ∈ grp ++ pad, n.depth = low := by
      intro n hn
      rcases List.mem_append.mp hn with hn | hn
      · exact hgd n hn
      · exact hpadd n hn
    have hall : (grp ++ pad).all (·.depth == low) = true := by
      rw [List.all_eq_true]; intro n hn; simp [hpd n hn]
    rw [if_pos hall]
    have hpg : grp ++ pad ≠ [] := by simp [hg]
    rw [pairNodes_group cx _ hpg hpd]
    -- the paired values are the next level
    have hvals : (grp ++ pad).map (·.cell.val) =
        (if (grp.map (·.cell.val)).length % 2 = 1 then grp.map (·.cell.val) ++ [zeroH cx.H cx.z0 low]
          else grp.map (·.cell.val)) := by
      simp only [pad, List.length_map]; split <;> simp [Cell.val]
    rw [hvals, pairUp_pad_eq_levelUp]
    -- covers before and after
    let paired : List (Node C) :=
      (levelUp cx.H (zeroH cx.H cx.z0 low) (grp.map (·.cell.val))).map (fun v => ⟨low + 1, .ready v⟩)
    have hpdep : ∀ n ∈ paired, n.depth = low + 1 := by
      intro n hn; simp only [paired, List.mem_map] at hn; obtain ⟨_, _, rfl⟩ := hn; rfl
    -- the leaves of the group
    obtain ⟨L1, L2, hL, hk, hgc, hseamd⟩ : ∃ L1 L2, L = L1 ++ L2 ∧ Cov cx.H cx.z0 true keep L1 ∧
        Cov cx.H cx.z0 false grp L2 ∧ (keep ≠ [] → ∀ n ∈ keep, low < n.depth) := by
      by_cases hke : keep = []
      · subst hke; exact ⟨[], L, rfl, rfl, by simpa using hcov, by simp⟩
      · obtain ⟨L1, L2, hL, hk, hgc, _⟩ := cov_split cx.H cx.z0 hke hg hcov
        exact ⟨L1, L2, hL, hk, hgc, fun _ => hkd⟩
    obtain ⟨hgv, _⟩ := cov_group cx.H cx.z0 hgd hgc
    have hL2 : L2 ≠ [] := cov_ne_nil cx.H cx.z0 hgc hg
    have hpv : paired.map (·.cell.val) = (chunks (low + 1) L2).map (mroot cx.H cx.z0 (low + 1)) := by
      simp only [paired, List.map_map]
      rw [show ((fun x : Node C => x.cell.val) ∘ fun v => (⟨low + 1, .ready v⟩ : Node C)) = id from rfl,
        List.map_id, hgv, levelUp_chunks]
    have hpc : Cov cx.H cx.z0 false paired L2 := group_cov cx.H cx.z0 hpdep hL2 hpv (by simp)
    have hplen : paired.length = (chunks (low + 1) L2).length := by
      rw [← List.length_map (f := fun x : Node C => x.cell.val), hpv, List.length_map]
    have hpne : paired ≠ [] := by
      intro e; rw [e] at hplen; rw [chunks_of_ne_nil hL2] at hplen; simp at hplen
    have hcov' : Cov cx.H cx.z0 false (keep ++ paired) L := by
      by_cases hke : keep = []
      · subst hke
        have h1 : L1 = [] := hk
        rw [hL, h1, List.nil_append]; exact hpc
      · rw [hL]
        apply cov_join cx.H cx.z0 hke hpne hk hpc
        rw [hpdep _ (List.head_mem hpne)]
        exact hseamd hke _ (List.getLast_mem hke)
    -- the number of paired nodes
    have hglen : grp.length = (chunks low L2).length := by
      rw [← List.length_map (f := fun x : Node C => x.cell.val), hgv, List.length_map]
    refine ih _ _ _ hcov' (fun e => hpne (List.append_eq_nil_iff.mp e).2) ?_ ?_
    · -- the measure falls
      have hpl : paired.length = (grp.length + 1) / 2 := by
        simp only [paired, List.length_map, levelUp_length]
      have hlast_old : lastDepth (keep ++ grp) = low := by
        rw [lastDepth_append hg, lastDepth_uniform hg hgd]
      have hlast_new : lastDepth (keep ++ paired) = low + 1 := by
        rw [lastDepth_append hpne, lastDepth_uniform hpne hpdep]
      have hlo : (keep ++ grp).length = keep.length + grp.length := List.length_append
      have hln : (keep ++ paired).length = keep.length + paired.length := List.length_append
      unfold collapseMeasure at hm0
      show (keep ++ paired).length + (headDepth (keep ++ paired) - lastDepth (keep ++ paired)) ≤ fuel
      rw [hlast_old, hlo] at hm0
      rw [hlast_new, hln, hpl]
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
    · -- a single remaining node came from two real ones
      intro m hm
      have hke : keep = [] := (List.append_eq_singleton_iff.mp hm).elim (fun h => h.1)
        (fun h => absurd h.2 hpne)
      subst hke
      have hpm : paired = [m] := by rw [List.nil_append] at hm; exact hm
      have hpl : paired.length = (grp.length + 1) / 2 := by
        simp only [paired, List.length_map, levelUp_length]
      rw [hpm] at hpl
      have hg2 : grp.length = 2 := by
        have : grp.length ≠ 1 := by intro e; exact hstop ⟨e, rfl⟩
        simp at hpl
        have : 0 < grp.length := List.length_pos_iff.mpr hg
        omega
      have hL1 : L1 = [] := hk
      have hLL : L = L2 := by rw [hL, hL1, List.nil_append]
      rw [hLL]
      have h1 : 2 ^ low < L2.length := lt_of_two_chunks (by rw [← hglen, hg2]; exact Nat.le_refl _)
      have h2 : L2.length ≤ 2 ^ (low + 1) := by
        have := hpc
        rw [hpm] at this
        obtain ⟨_, hlen, _⟩ := this
        simpa [hpdep m (by rw [hpm]; simp)] using hlen
      rw [hpdep m (by rw [hpm]; simp), depthOf_of_between h1 h2]

/-! ## The loop raises no error -/

/-- Depths never increase along the list. -/
def NonInc : List (Node C) → Prop
  | [] => True
  | [_] => True
  | n :: m :: ns => m.depth ≤ n.depth ∧ NonInc (m :: ns)

theorem nonInc_of_cov {full : Bool} {ns : List (Node C)} {L : List C} (h : Cov H z0 full ns L) : NonInc ns := by
  induction ns generalizing L with
  | nil => trivial
  | cons n ns ih =>
    cases ns with
    | nil => trivial
    | cons m ns => obtain ⟨_, ys, _, _, _, hd, hc⟩ := h; exact ⟨hd, ih hc⟩

theorem nonInc_le {n : Node C} {ns : List (Node C)} (h : NonInc (n :: ns)) : ∀ m ∈ ns, m.depth ≤ n.depth := by
  induction ns generalizing n with
  | nil => simp
  | cons m ns ih =>
    intro k hk
    rcases List.mem_cons.mp hk with rfl | hk
    · exact h.1
    · exact Nat.le_trans (ih h.2 k hk) h.1

theorem nonInc_tail {n : Node C} {ns : List (Node C)} (h : NonInc (n :: ns)) : NonInc ns := by
  cases ns with
  | nil => trivial
  | cons m ns => exact h.2

theorem nonInc_append_left : ∀ {A B : List (Node C)}, NonInc (A ++ B) → NonInc A
  | [], _, _ => trivial
  | [_], _, _ => trivial
  | a :: a' :: as, B, h => ⟨h.1, nonInc_append_left (B := B) h.2⟩

theorem nonInc_split {ns : List (Node C)} (h : NonInc ns) (hne : ns ≠ []) :
    ∃ keep grp low, ns = keep ++ grp ∧ grp ≠ [] ∧ (∀ n ∈ grp, n.depth = low) ∧ (∀ n ∈ keep, low < n.depth) := by
  induction ns with
  | nil => exact absurd rfl hne
  | cons n rest ih =>
    cases rest with
    | nil => exact ⟨[], [n], n.depth, rfl, by simp, by simp, by simp⟩
    | cons m rest =>
      obtain ⟨keep, grp, low, heq, hg, hgd, hkd⟩ := ih h.2 (by simp)
      have hle := nonInc_le h
      by_cases hlow : n.depth = low
      · have hk : keep = [] := by
          cases keep with
          | nil => rfl
          | cons k ks => have := hkd k (by simp); have := hle k (by rw [heq]; simp); omega
        subst hk
        exact ⟨[], n :: grp, low, by simp [heq], by simp,
          fun k hk => by rcases List.mem_cons.mp hk with rfl | hk; exact hlow; exact hgd k hk, by simp⟩
      · have : low < n.depth := by
          obtain ⟨g, hgm⟩ := List.exists_mem_of_ne_nil grp hg
          have := hle g (by rw [heq]; simp [hgm]); have := hgd g hgm; omega
        exact ⟨n :: keep, grp, low, by simp [heq], hg, hgd,
          fun k hk => by rcases List.mem_cons.mp hk with rfl | hk; exact this; exact hkd k hk⟩

theorem nonInc_of_split {keep G : List (Node C)} {low : Nat} (hk : NonInc keep) (hkd : ∀ n ∈ keep, low < n.depth)
    (hG : ∀ n ∈ G, n.depth = low + 1) (hGne : G ≠ []) : NonInc (keep ++ G) := by
  induction keep with
  | nil =>
    simp only [List.nil_append]
    induction G with
    | nil => trivial
    | cons g gs ih =>
      cases gs with
      | nil => trivial
      | cons g' gs' =>
        refine ⟨by rw [hG g' (by simp), hG g (by simp)]; exact Nat.le_refl _,
          ih (fun n hn => hG n (List.mem_cons_of_mem _ hn)) (by simp)⟩
  | cons k ks ih =>
    cases ks with
    | nil =>
      obtain ⟨g, gs, rfl⟩ := List.exists_cons_of_ne_nil hGne
      simp only [List.cons_append, List.nil_append]
      refine ⟨by rw [hG g (by simp)]; exact hkd k (by simp), ?_⟩
      have := ih trivial (fun n hn => by simp at hn) 
      simpa using this
    | cons k' ks' =>
      refine ⟨hk.1, ?_⟩
      have := ih hk.2 (fun n hn => hkd n (List.mem_cons_of_mem _ hn))
      simpa using this

/-- **The loop of `collapseAllDepths` raises no error** on a deepest-first list
    of ready nodes outside group rounds (`live = false`), given its fuel. -/
theorem collapseLoop_state (cx : Ctx C) :
    ∀ fuel (s : St C) (ns : List (Node C)), NonInc ns → ns ≠ [] → (∀ n ∈ ns, n.cell.isReady = true) →
      collapseMeasure ns ≤ fuel → (collapseLoop cx false fuel s ns).1 = s ∧
        ∀ n ∈ (collapseLoop cx false fuel s ns).2, n.cell.isReady = true := by
  intro fuel
  induction fuel with
  | zero =>
    intro s ns _ hne _ hm
    have : ns.length = 0 := by unfold collapseMeasure at hm; omega
    exact absurd (List.eq_nil_of_length_eq_zero this) hne
  | succ fuel ih =>
    intro s ns hni hne hr hm
    obtain ⟨keep, grp, low, rfl, hg, hgd, hkd⟩ := nonInc_split hni hne
    have hm0 : collapseMeasure (keep ++ grp) ≤ fuel + 1 := hm
    simp only [collapseLoop, min_depth_split hg hgd hkd, count_low_split hgd hkd, all_low_split hgd hkd]
    by_cases hstop : grp.length = 1 ∧ keep.isEmpty = true
    · rw [if_pos hstop]; exact ⟨rfl, hr⟩
    rw [if_neg hstop]
    let pad : List (Node C) := if grp.length % 2 = 1 then [⟨low, .ready (zeroH cx.H cx.z0 low)⟩] else []
    have hns2 : (if grp.length % 2 = 1 then keep ++ grp ++ [⟨low, .ready (zeroH cx.H cx.z0 low)⟩]
        else keep ++ grp) = keep ++ (grp ++ pad) := by simp only [pad]; split <;> simp
    have hcnt2 : (if grp.length % 2 = 1 then grp.length + 1 else grp.length) = (grp ++ pad).length := by
      simp only [pad]; split <;> simp
    simp only [hns2, hcnt2, Bool.false_eq_true, and_false, if_false]
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
    apply ih _ _ (nonInc_of_split hkni hkd hpdep hpne) (fun e => hpne (List.append_eq_nil_iff.mp e).2)
    · intro n hn
      rcases List.mem_append.mp hn with hn | hn
      · exact hr n (List.mem_append_left _ hn)
      · simp only [paired, List.mem_map] at hn; obtain ⟨_, _, rfl⟩ := hn; rfl
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

/-- `getDepth` is monotone. -/
theorem depthOf_mono {a b : Nat} (h : a ≤ b) : depthOf a ≤ depthOf b := by
  unfold depthOf
  split
  · exact Nat.zero_le _
  · rename_i ha
    rw [if_neg (by omega)]
    have hb : b - 1 ≠ 0 := by omega
    have : (a - 1).log2 ≤ (b - 1).log2 := by
      rw [Nat.le_log2 hb]
      exact Nat.le_trans (Nat.log2_self_le (by omega)) (by omega)
    omega

/-- Expanding a root from depth `d` to `d + k` under zero siblings. -/
theorem foldl_expand (d k : Nat) (L : List C) (hL : L.length ≤ 2 ^ d) :
    (List.range k).foldl (fun acc i => H acc (zeroH H z0 (d + i))) (mroot H z0 d L) = mroot H z0 (d + k) L := by
  induction k with
  | zero => rfl
  | succ k ih =>
    rw [List.range_succ, List.foldl_append, ih]
    simp only [List.foldl_cons, List.foldl_nil]
    have : L.length ≤ 2 ^ (d + k) := Nat.le_trans hL (Nat.pow_le_pow_right (by omega) (by omega))
    rw [← mroot_expand H z0 this, show d + (k + 1) = d + k + 1 by omega]

/-- **`collapseAllDepths`** reduces a full cover of `L` to the bounded root at
    the depth of its limit (`0`: the depth the leaves need). -/
theorem collapseAll_val (cx : Ctx C) (s : St C) (ns : List (Node C)) (limit : Nat) (live : Bool)
    (L : List C) (hcov : Cov cx.H cx.z0 true ns L) (hne : ns ≠ []) (hd : headDepth ns < 64)
    (hlim : limit = 0 ∨ L.length ≤ limit) :
    (s.collapseAll cx ns limit live).2.val =
      mroot cx.H cx.z0 (depthOf (if limit = 0 then L.length else limit)) L := by
  have hgood : ∀ m, ns = [m] → m.depth = depthOf L.length := by
    intro m hm; subst hm
    obtain ⟨_, hlen, _⟩ := hcov
    simp at hlen; rw [hlen, depthOf_two_pow]
  have hmeas : collapseMeasure ns ≤ 2 * ns.length + 64 := by
    unfold collapseMeasure; omega
  obtain ⟨n, hloop, hc, hnd⟩ := collapseLoop_cov cx live _
    (if limit > 0 ∧ leavesOf ns > limit then s.setHashErr "chunk limit" else s) ns L
    (cov_of_full _ _ hcov) hne hmeas hgood
  obtain ⟨hLne, hLlen, hv⟩ := hc
  rw [if_neg (by decide)] at hLlen
  unfold St.collapseAll
  rw [hloop]
  by_cases hl0 : limit = 0
  · simp only [collapseFinish, if_neg (show ¬ limit > 0 by omega), if_pos hl0, hv, hnd]
  · simp only [collapseFinish, if_pos (show limit > 0 by omega), if_neg hl0]
    show List.foldl _ n.cell.val _ = _
    have hle : n.depth ≤ depthOf limit := by
      rw [hnd]; exact depthOf_mono (by omega)
    rw [hv, foldl_expand cx.H cx.z0 _ _ L hLlen, Nat.add_sub_cancel' hle]

/-- **`collapseAllDepths` raises no error** on a ready full cover outside group
    rounds when the leaves fit the limit, and its root is ready. -/
theorem collapseAll_state (cx : Ctx C) (s : St C) (ns : List (Node C)) (limit : Nat) (L : List C)
    (hcov : Cov cx.H cx.z0 true ns L) (hne : ns ≠ []) (hr : ∀ n ∈ ns, n.cell.isReady = true)
    (hd : headDepth ns < 64) (hlim : limit = 0 ∨ L.length ≤ limit) :
    (s.collapseAll cx ns limit false).1 = s ∧ (s.collapseAll cx ns limit false).2.isReady = true := by
  have hgood : ∀ m, ns = [m] → m.depth = depthOf L.length := by
    intro m hm; subst hm
    obtain ⟨_, hlen, _⟩ := hcov
    simp at hlen; rw [hlen, depthOf_two_pow]
  have hmeas : collapseMeasure ns ≤ 2 * ns.length + 64 := by unfold collapseMeasure; omega
  have hchunks : ¬ (limit > 0 ∧ leavesOf ns > limit) := by
    rw [← cov_full_length cx.H cx.z0 hcov]; omega
  obtain ⟨n, hloop, _, _⟩ := collapseLoop_cov cx false _ s ns L (cov_of_full _ _ hcov) hne hmeas hgood
  obtain ⟨hst, hrd⟩ := collapseLoop_state cx _ s ns (nonInc_of_cov _ _ hcov) hne hr hmeas
  have hnr : n.cell.isReady = true := hrd n (by rw [hloop]; simp)
  unfold St.collapseAll
  rw [if_neg hchunks, hloop, hst]
  by_cases hl0 : limit = 0
  · simp only [collapseFinish, if_neg (show ¬ limit > 0 by omega)]
    exact ⟨trivial, hnr⟩
  · simp only [collapseFinish, if_pos (show limit > 0 by omega), Bool.false_eq_true, and_false, if_false,
      St.vals, List.all_cons, List.all_nil, hnr, Bool.and_true, if_true]
    exact ⟨trivial, rfl⟩

end Ssz.Hasher
