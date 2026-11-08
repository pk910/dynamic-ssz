import SszCert.Hasher.Model

/-!
# Merkleization, generic in the hash

The specification side of the hasher proofs: SSZ merkleization of a chunk
sequence into a tree of a given depth, for an arbitrary pair hash `H` and
zero chunk `z0`, and the laws every reduction path of the hasher relies on.
-/

namespace Ssz.Hasher

variable {C : Type} (H : C → C → C) (z0 : C)

/-- Merkle root of `xs` in a tree of depth `d` (callers keep
    `xs.length ≤ 2^d`), padding the missing right part with zero subtrees. -/
def mroot : Nat → List C → C
  | 0, [] => z0
  | 0, x :: _ => x
  | d + 1, xs => H (mroot d (xs.take (2 ^ d))) (mroot d (xs.drop (2 ^ d)))

@[simp] theorem mroot_zero_nil : mroot H z0 0 [] = z0 := rfl
@[simp] theorem mroot_zero_cons (x : C) (xs : List C) : mroot H z0 0 (x :: xs) = x := rfl

theorem mroot_succ (d : Nat) (xs : List C) :
    mroot H z0 (d + 1) xs = H (mroot H z0 d (xs.take (2 ^ d))) (mroot H z0 d (xs.drop (2 ^ d))) := rfl

/-- The empty tree is the zero subtree. -/
@[simp] theorem mroot_nil (d : Nat) : mroot H z0 d [] = zeroH H z0 d := by
  induction d with
  | zero => rfl
  | succ d ih => simp [mroot_succ, ih, zeroH]

/-- **Pairing**: a full left subtree and any right subtree. -/
theorem mroot_append {d : Nat} {xs ys : List C} (hx : xs.length = 2 ^ d) :
    mroot H z0 (d + 1) (xs ++ ys) = H (mroot H z0 d xs) (mroot H z0 d ys) := by
  rw [mroot_succ, List.take_append_of_le_length (by omega), List.drop_append_of_le_length (by omega),
    List.take_of_length_le (by omega), List.drop_of_length_le (by omega), List.nil_append]

/-- **Expansion**: a tree that fits one level lower, under a zero sibling. -/
theorem mroot_expand {d : Nat} {xs : List C} (hx : xs.length ≤ 2 ^ d) :
    mroot H z0 (d + 1) xs = H (mroot H z0 d xs) (zeroH H z0 d) := by
  rw [mroot_succ, List.take_of_length_le hx, List.drop_of_length_le hx, mroot_nil]

/-- **Zero padding**: trailing zero chunks do not change a root. -/
theorem mroot_pad (d k : Nat) (xs : List C) (h : xs.length + k ≤ 2 ^ d) :
    mroot H z0 d (xs ++ List.replicate k z0) = mroot H z0 d xs := by
  induction d generalizing xs k with
  | zero =>
    have : xs.length + k ≤ 1 := by simpa using h
    cases xs with
    | nil =>
      cases k with
      | zero => rfl
      | succ k => simp [List.replicate_succ, zeroH]
    | cons x xs => rfl
  | succ d ih =>
    rw [mroot_succ, mroot_succ, List.take_append, List.drop_append, List.take_replicate,
      List.drop_replicate]
    have h2 : 2 ^ (d + 1) = 2 ^ d + 2 ^ d := by rw [Nat.pow_succ]; omega
    by_cases hl : xs.length ≤ 2 ^ d
    · rw [List.take_of_length_le hl, List.drop_of_length_le hl, List.nil_append]
      rw [ih _ _ (by omega)]
      have e2 : ([] : List C).length + (k - (2 ^ d - xs.length)) ≤ 2 ^ d := by simp; omega
      have := ih (k - (2 ^ d - xs.length)) [] e2
      simp only [List.nil_append] at this
      rw [this]
    · have hl' : 2 ^ d ≤ xs.length := by omega
      have z1 : 2 ^ d - xs.length = 0 := by omega
      rw [z1, Nat.zero_min, List.replicate_zero, List.append_nil, Nat.sub_zero]
      have : (xs.drop (2 ^ d)).length + k ≤ 2 ^ d := by rw [List.length_drop]; omega
      rw [ih _ _ this]

/-! ## Chunks and reduction levels -/

/-- Split into pieces of `2^k` (the last one may be shorter). -/
def chunks (k : Nat) : List C → List (List C)
  | [] => []
  | x :: xs => (x :: xs).take (2 ^ k) :: chunks k ((x :: xs).drop (2 ^ k))
termination_by l => l.length
decreasing_by simp only [List.length_drop, List.length_cons]; have := Nat.one_le_two_pow (n := k); omega

@[simp] theorem chunks_nil (k : Nat) : chunks k ([] : List C) = [] := by simp [chunks]

theorem chunks_cons (k : Nat) (x : C) (xs : List C) :
    chunks k (x :: xs) = (x :: xs).take (2 ^ k) :: chunks k ((x :: xs).drop (2 ^ k)) := by
  rw [chunks]

/-- A non-empty list that fits one piece is one piece. -/
theorem chunks_single {k : Nat} {xs : List C} (hne : xs ≠ []) (h : xs.length ≤ 2 ^ k) :
    chunks k xs = [xs] := by
  cases xs with
  | nil => exact absurd rfl hne
  | cons x xs =>
    rw [chunks_cons, List.take_of_length_le h, List.drop_of_length_le h, chunks_nil]

/-- Pieces of `2^0` are singletons. -/
theorem chunks_zero (xs : List C) : chunks 0 xs = xs.map (fun x => [x]) := by
  induction xs with
  | nil => simp
  | cons x xs ih => rw [chunks_cons]; simp [ih]

/-- `pairUp` on two leading values. -/
theorem pairUp_cons2 (a b : C) (rest : List C) : pairUp H (a :: b :: rest) = H a b :: pairUp H rest := rfl

/-- `levelUp` peels a leading pair. -/
theorem levelUp_cons2 (z a b : C) (rest : List C) :
    levelUp H z (a :: b :: rest) = H a b :: levelUp H z rest := by
  unfold levelUp
  have : (a :: b :: rest).length % 2 = rest.length % 2 := by simp; omega
  rw [this]
  split <;> simp [pairUp]

theorem chunks_of_ne_nil {k : Nat} {L : List C} (h : L ≠ []) :
    chunks k L = L.take (2 ^ k) :: chunks k (L.drop (2 ^ k)) := by
  cases L with
  | nil => exact absurd rfl h
  | cons x xs => exact chunks_cons k x xs

theorem levelUp_nil (z : C) : levelUp H z [] = [] := by simp [levelUp, pairUp]

theorem levelUp_single (z a : C) : levelUp H z [a] = [H a z] := by simp [levelUp, pairUp]

theorem levelUp_pair (z a b : C) : levelUp H z [a, b] = [H a b] := by simp [levelUp, pairUp]

theorem two_pow_succ_eq (i : Nat) : 2 ^ (i + 1) = 2 ^ i + 2 ^ i := by rw [Nat.pow_succ]; omega

/-- **One reduction level**: padding an odd level with the zero hash of that
    level and pairing turns the roots of `2^i`-pieces into the roots of
    `2^(i+1)`-pieces. -/
theorem levelUp_chunks (i : Nat) (L : List C) :
    levelUp H (zeroH H z0 i) ((chunks i L).map (mroot H z0 i)) = (chunks (i + 1) L).map (mroot H z0 (i + 1)) := by
  suffices ∀ n (L : List C), L.length ≤ n →
      levelUp H (zeroH H z0 i) ((chunks i L).map (mroot H z0 i)) = (chunks (i + 1) L).map (mroot H z0 (i + 1)) from
    this _ L (Nat.le_refl _)
  intro n
  induction n with
  | zero =>
    intro L hL
    have : L = [] := List.eq_nil_of_length_eq_zero (by omega)
    subst this; simp [levelUp_nil]
  | succ n ih =>
    intro L hL
    have hp := two_pow_succ_eq i
    have hpos := Nat.one_le_two_pow (n := i)
    by_cases h0 : L = []
    · subst h0; simp [levelUp_nil]
    by_cases h1 : L.length ≤ 2 ^ i
    · rw [chunks_single h0 h1, chunks_single h0 (by omega)]
      simp only [List.map_cons, List.map_nil, levelUp_single]
      rw [mroot_expand H z0 h1]
    by_cases h2 : L.length ≤ 2 ^ (i + 1)
    · have hd : L.drop (2 ^ i) ≠ [] := by
        intro e; have := congrArg List.length e; simp at this; omega
      have hdl : (L.drop (2 ^ i)).length ≤ 2 ^ i := by simp; omega
      rw [chunks_of_ne_nil h0, chunks_single hd hdl, chunks_single h0 h2]
      simp only [List.map_cons, List.map_nil, levelUp_pair]
      rw [mroot_succ]
    · have hd : L.drop (2 ^ i) ≠ [] := by
        intro e; have := congrArg List.length e; simp at this; omega
      have hd2 : L.drop (2 ^ (i + 1)) ≠ [] := by
        intro e; have := congrArg List.length e; simp at this; omega
      rw [chunks_of_ne_nil h0, chunks_of_ne_nil hd, chunks_of_ne_nil (k := i + 1) h0]
      simp only [List.map_cons, levelUp_cons2, List.drop_drop]
      have e1 : 2 ^ i + 2 ^ i = 2 ^ (i + 1) := hp.symm
      rw [e1, ih (L.drop (2 ^ (i + 1))) (by simp; omega), mroot_succ, List.take_take, List.drop_take]
      congr 2
      · rw [Nat.min_eq_left (by omega)]
      · rw [show 2 ^ (i + 1) - 2 ^ i = 2 ^ i by omega]

/-- Iterated reduction levels from level `i`. -/
theorem levelsUp_chunks (n i : Nat) (L : List C) :
    levelsUp H z0 n i ((chunks i L).map (mroot H z0 i)) = (chunks (i + n) L).map (mroot H z0 (i + n)) := by
  induction n generalizing i with
  | zero => rfl
  | succ n ih =>
    simp only [levelsUp]
    rw [levelUp_chunks, ih (i + 1), show i + 1 + n = i + (n + 1) by omega]

/-! ## Exact reductions -/

/-- A list of `m` full pieces splits into `m` pieces of `2^i`. -/
theorem chunks_length_exact (i m : Nat) (L : List C) (h : L.length = m * 2 ^ i) :
    (chunks i L).length = m := by
  induction m generalizing L with
  | zero =>
    have : L = [] := List.eq_nil_of_length_eq_zero (by simpa using h)
    subst this; simp
  | succ m ih =>
    have hpos := Nat.one_le_two_pow (n := i)
    have hne : L ≠ [] := by intro e; subst e; simp at h; rw [Nat.succ_mul] at h; omega
    rw [chunks_of_ne_nil hne, List.length_cons, ih]
    simp only [List.length_drop, h, Nat.succ_mul]; omega

/-- Without an odd element, a level needs no padding. -/
theorem levelUp_even (z : C) (xs : List C) (h : xs.length % 2 = 0) : levelUp H z xs = pairUp H xs := by
  simp [levelUp, h]

/-- The roots of `2^i`-pieces of a list of length `m * 2^i`. -/
theorem length_chunk_roots (i m : Nat) (L : List C) (h : L.length = m * 2 ^ i) :
    ((chunks i L).map (mroot H z0 i)).length = m := by
  rw [List.length_map, chunks_length_exact i m L h]

/-- **Exact reduction**: pairing levels of a list of `out * 2^j` chunks until
    `out` remain yields the roots of its `2^j`-pieces. This is the worker loop
    of a background job, `flushPending`'s level loop and `reduceElem`. -/
theorem reduceTo_chunks (out j : Nat) (hout : 0 < out) (L : List C) (hL : L.length = out * 2 ^ j) :
    ∀ t fuel, t ≤ j → j - t ≤ fuel →
      reduceTo H out fuel ((chunks t L).map (mroot H z0 t)) = (chunks j L).map (mroot H z0 j) := by
  intro t fuel ht hf
  induction fuel generalizing t with
  | zero =>
    have : t = j := by omega
    subst this; rfl
  | succ fuel ih =>
    have hlen : ((chunks t L).map (mroot H z0 t)).length = out * 2 ^ (j - t) := by
      apply length_chunk_roots
      rw [hL, Nat.mul_assoc, ← Nat.pow_add, Nat.sub_add_cancel ht]
    simp only [reduceTo]
    by_cases htj : t = j
    · subst htj
      rw [if_neg (by rw [hlen]; simp)]
    · have hlt : t < j := by omega
      have hgt : ((chunks t L).map (mroot H z0 t)).length > out := by
        rw [hlen]
        have : 2 ≤ 2 ^ (j - t) := by
          have := Nat.pow_le_pow_right (n := 2) (by omega) (show 1 ≤ j - t by omega)
          simpa using this
        have := Nat.mul_le_mul_left out this
        omega
      rw [if_pos hgt]
      have hev : ((chunks t L).map (mroot H z0 t)).length % 2 = 0 := by
        rw [hlen]
        have : 2 ^ (j - t) = 2 * 2 ^ (j - t - 1) := by
          rw [← Nat.pow_succ']; congr 1; omega
        rw [this, ← Nat.mul_assoc, Nat.mul_comm out 2, Nat.mul_assoc]; simp
      rw [← levelUp_even H (zeroH H z0 t) _ hev, levelUp_chunks]
      exact ih (t + 1) (by omega) (by omega)

/-- Pieces of `2^0` are the values themselves. -/
theorem chunk_roots_zero (L : List C) : (chunks 0 L).map (mroot H z0 0) = L := by
  rw [chunks_zero]; induction L with
  | nil => rfl
  | cons x xs ih => simp [ih]

theorem le_two_pow (k : Nat) : k ≤ 2 ^ k := by
  induction k with
  | zero => simp
  | succ k ih => rw [Nat.pow_succ]; have := Nat.one_le_two_pow (n := k); omega

/-- `reduceElem` of a power-of-two element is its root. -/
theorem reduceElem_eq (k : Nat) (xs : List C) (h : xs.length = 2 ^ k) :
    reduceElem H z0 xs = mroot H z0 k xs := by
  have hne : xs ≠ [] := by intro e; subst e; simp at h; have := Nat.one_le_two_pow (n := k); omega
  have := reduceTo_chunks H z0 1 k (by omega) xs (by simp [h]) 0 xs.length (Nat.zero_le _)
    (by rw [h]; exact Nat.le_trans (Nat.sub_le _ _) (le_two_pow k))
  rw [chunk_roots_zero] at this
  have hs : chunks k xs = [xs] := chunks_single hne (by rw [h]; exact Nat.le_refl _)
  simp only [reduceElem, this, hs, List.map_cons, List.map_nil, List.head?_cons, Option.getD_some]

/-! ## Depth and `merkleizeImpl` -/

/-- `getDepth`'s tree holds its leaves. -/
theorem le_two_pow_depthOf (n : Nat) : n ≤ 2 ^ depthOf n := by
  unfold depthOf
  split
  · simp; omega
  · rename_i h
    have := Nat.lt_log2_self (n := n - 1)
    omega

/-- The depth of a power of two is its exponent. -/
theorem depthOf_two_pow (k : Nat) : depthOf (2 ^ k) = k := by
  unfold depthOf
  split
  · rename_i h
    cases k with
    | zero => rfl
    | succ k => have := Nat.one_le_two_pow (n := k); rw [Nat.pow_succ] at h; omega
  · rename_i h
    cases k with
    | zero => simp at h
    | succ k =>
      have hne : 2 ^ (k + 1) - 1 ≠ 0 := by have := Nat.one_le_two_pow (n := k); rw [Nat.pow_succ]; omega
      have lo : k ≤ (2 ^ (k + 1) - 1).log2 := by
        rw [Nat.le_log2 hne]; rw [Nat.pow_succ]; have := Nat.one_le_two_pow (n := k); omega
      have hi : (2 ^ (k + 1) - 1).log2 < k + 1 := by
        rw [Nat.log2_lt hne]; have := Nat.one_le_two_pow (n := k + 1); omega
      omega

/-- The chunk capacity `merkleizeImpl` reduces to: the limit (`0` = the
    count), widened to the count when the chunks overflow it. -/
def implLimit (count limit : Nat) : Nat := max (if limit = 0 then count else limit) count

/-- **`merkleizeImpl`** is the bounded Merkle root at the depth of its
    (possibly widened) limit. -/
theorem merkleizeImpl_eq (xs : List C) (limit : Nat) :
    (merkleizeImpl H z0 xs limit).1 = mroot H z0 (depthOf (implLimit xs.length limit)) xs := by
  have hcap := le_two_pow_depthOf (implLimit xs.length limit)
  have hle : xs.length ≤ implLimit xs.length limit := by unfold implLimit; omega
  unfold merkleizeImpl
  simp only
  have elim : (if decide (xs.length > (if limit = 0 then xs.length else limit)) = true
      then xs.length else (if limit = 0 then xs.length else limit)) = implLimit xs.length limit := by
    unfold implLimit
    by_cases hz : limit = 0
    · simp [hz]
    · by_cases hc : xs.length > limit
      · simp [hz, hc, Nat.max_def]; omega
      · simp [hz, hc, Nat.max_def]; intro; omega
  rw [elim]
  by_cases h0 : implLimit xs.length limit = 0
  · rw [if_pos h0]
    have : xs = [] := List.eq_nil_of_length_eq_zero (by omega)
    subst this; rw [h0]; rfl
  rw [if_neg h0]
  by_cases h1 : implLimit xs.length limit = 1
  · rw [if_pos h1, h1]
    cases xs <;> rfl
  rw [if_neg h1]
  by_cases hn : xs = []
  · rw [if_pos hn, hn, mroot_nil]
  rw [if_neg hn]
  have := levelsUp_chunks H z0 (depthOf (implLimit xs.length limit)) 0 xs
  rw [chunk_roots_zero, Nat.zero_add] at this
  rw [this, chunks_single hn (Nat.le_trans hle hcap)]
  rfl

/-! ## Progressive merkleization (EIP-7916) -/

/-- `merkleize_progressive(chunks, 2^depth)`: the first `2^depth` chunks as a
    bounded tree, the rest progressively with four times the width. -/
def progRoot (depth : Nat) (xs : List C) : C :=
  if xs = [] then z0
  else H (mroot H z0 depth (xs.take (2 ^ depth))) (progRoot (depth + 2) (xs.drop (2 ^ depth)))
termination_by xs.length
decreasing_by
  simp only [List.length_drop]
  have : 0 < xs.length := List.length_pos_iff.mpr (by assumption)
  have := Nat.one_le_two_pow (n := depth)
  omega

theorem progRoot_nil (depth : Nat) : progRoot H z0 depth [] = z0 := by
  rw [progRoot]; simp

theorem progRoot_cons {depth : Nat} {xs : List C} (h : xs ≠ []) :
    progRoot H z0 depth xs = H (mroot H z0 depth (xs.take (2 ^ depth))) (progRoot H z0 (depth + 2) (xs.drop (2 ^ depth))) := by
  rw [progRoot]; simp [h]

/-- **`merkleizeProgressiveImpl`** is progressive merkleization, given fuel. -/
theorem progImpl_eq : ∀ (fuel depth : Nat) (xs : List C), xs.length < fuel →
    progImpl H z0 fuel depth xs = progRoot H z0 depth xs := by
  intro fuel
  induction fuel with
  | zero => intro _ xs h; omega
  | succ fuel ih =>
    intro depth xs h
    by_cases hx : xs = []
    · subst hx; simp [progImpl, progRoot_nil]
    rw [progRoot_cons H z0 hx]
    simp only [progImpl, hx, if_false]
    have hl : (merkleizeImpl H z0 (xs.take (2 ^ depth)) (2 ^ depth)).1 = mroot H z0 depth (xs.take (2 ^ depth)) := by
      rw [merkleizeImpl_eq]
      congr 1
      unfold implLimit
      have : (xs.take (2 ^ depth)).length ≤ 2 ^ depth := by simp; omega
      have h2 : 2 ^ depth ≠ 0 := by have := Nat.one_le_two_pow (n := depth); omega
      simp only [h2, if_false, Nat.max_eq_left this, depthOf_two_pow]
    rw [hl]
    congr 1
    by_cases hr : xs.drop (2 ^ depth) = []
    · rw [if_pos hr, hr, progRoot_nil]
    · rw [if_neg hr]
      apply ih
      have : 0 < xs.length := List.length_pos_iff.mpr hx
      have := Nat.one_le_two_pow (n := depth)
      simp only [List.length_drop]; omega

end Ssz.Hasher