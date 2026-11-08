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

end Ssz.Hasher
