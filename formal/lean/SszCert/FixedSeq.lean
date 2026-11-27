import SszCert.Basic
import SszCert.Schema

/-!
# SSZ sequences of fixed-size elements

Lists and vectors whose element type is fixed-size use packed encoding: the
serialization is the plain concatenation of the `k`-byte element encodings —
no offset table. The decoder derives the element count from the byte length
(`count = len / k`, requiring `len % k = 0`), so the canonicity discipline is
exact tiling: partial trailing chunks are impossible by the divisibility
check, and the chunker consumes the buffer exactly.

Elements are modeled as byte strings (as in `Schema.lean`); element-level
canonicity composes at the codec layer.
-/

namespace Ssz
namespace FixedSeq

open Sch (cat)

/-- All chunks have exactly `k` bytes. -/
def Sized (k : Nat) (vs : List (List UInt8)) : Prop :=
  ∀ v ∈ vs, v.length = k

/-- Split `bs` into exactly `count` chunks of `k` bytes, consuming the buffer
    exactly. -/
def chunkExact (k : Nat) : Nat → List UInt8 → Option (List (List UInt8))
  | 0, bs => if bs = [] then some [] else none
  | count + 1, bs =>
    if k ≤ bs.length then
      (chunkExact k count (bs.drop k)).map (bs.take k :: ·)
    else
      none

/-- Packed serialization: plain concatenation. -/
def serializeSeq (vs : List (List UInt8)) : List UInt8 := cat vs

/-- Strict decoding of a fixed-element list: divisibility, derived count,
    limit, exact tiling. -/
def decodeFixedList (k limit : Nat) (bs : List UInt8) :
    Option (List (List UInt8)) :=
  if bs.length % k = 0 ∧ bs.length / k ≤ limit then
    chunkExact k (bs.length / k) bs
  else
    none

/-- Strict decoding of a fixed-element vector: exact total length, `n`
    chunks. -/
def decodeFixedVector (k n : Nat) (bs : List UInt8) :
    Option (List (List UInt8)) :=
  if bs.length = k * n then chunkExact k n bs else none

/-! ## Chunking lemmas -/

theorem length_cat_sized {k : Nat} {vs : List (List UInt8)} (h : Sized k vs) :
    (cat vs).length = k * vs.length := by
  induction vs with
  | nil => simp [cat]
  | cons v vs ih =>
    have hv : v.length = k := h v (by simp)
    have hs : Sized k vs := fun w hw => h w (by simp [hw])
    simp only [cat, List.length_append, List.length_cons, hv, ih hs,
      Nat.mul_succ]
    omega

/-- Chunking a sized concatenation recovers the chunks. -/
theorem chunkExact_cat {k : Nat} {vs : List (List UInt8)} (h : Sized k vs) :
    chunkExact k vs.length (cat vs) = some vs := by
  induction vs with
  | nil => simp [chunkExact, cat]
  | cons v vs ih =>
    have hv : v.length = k := h v (by simp)
    have hs : Sized k vs := fun w hw => h w (by simp [hw])
    simp only [List.length_cons, chunkExact, cat]
    rw [if_pos (by simp [hv])]
    have htake : (v ++ cat vs).take k = v := by
      rw [List.take_append_of_le_length (by omega),
        List.take_of_length_le (by omega)]
    have hdrop : (v ++ cat vs).drop k = cat vs := by
      rw [List.drop_append_of_le_length (by omega)]
      have hnil : v.drop k = [] := List.drop_eq_nil_of_le (by omega)
      simp [hnil]
    rw [htake, hdrop, ih hs]
    rfl

/-- Soundness of chunking: the chunks are sized, tile the buffer exactly,
    and there are exactly `count` of them. -/
theorem chunkExact_sound {k count : Nat} {bs : List UInt8}
    {vs : List (List UInt8)} (h : chunkExact k count bs = some vs) :
    Sized k vs ∧ cat vs = bs ∧ vs.length = count := by
  induction count generalizing bs vs with
  | zero =>
    simp only [chunkExact] at h
    by_cases hb : bs = []
    · rw [if_pos hb] at h
      injection h with hv
      subst hv hb
      exact ⟨fun v hv => absurd hv (by simp), by simp [cat], rfl⟩
    · rw [if_neg hb] at h
      exact absurd h (by simp)
  | succ count ih =>
    simp only [chunkExact] at h
    by_cases hk : k ≤ bs.length
    · rw [if_pos hk] at h
      cases hc : chunkExact k count (bs.drop k) with
      | none => rw [hc] at h; simp at h
      | some vs' =>
        rw [hc] at h
        simp only [Option.map_some, Option.some.injEq] at h
        subst h
        obtain ⟨hsz, hcat, hlen⟩ := ih hc
        refine ⟨?_, ?_, by simp [hlen]⟩
        · intro v hv
          simp only [List.mem_cons] at hv
          cases hv with
          | inl h1 =>
            subst h1
            rw [List.length_take]
            omega
          | inr h2 => exact hsz v h2
        · simp only [cat, hcat, List.take_append_drop]
    · rw [if_neg hk] at h
      exact absurd h (by simp)

/-! ## Lists -/

/-- Roundtrip (P1) for fixed-element lists. -/
theorem decodeFixedList_serializeSeq {k limit : Nat}
    {vs : List (List UInt8)} (hk : 0 < k) (h : Sized k vs)
    (hlim : vs.length ≤ limit) :
    decodeFixedList k limit (serializeSeq vs) = some vs := by
  have hlen := length_cat_sized h
  have hmod : (cat vs).length % k = 0 := by
    rw [hlen]
    simp [Nat.mul_mod_right]
  have hdiv : (cat vs).length / k = vs.length := by
    rw [hlen]
    exact Nat.mul_div_cancel_left _ hk
  simp only [decodeFixedList, serializeSeq]
  rw [if_pos ⟨hmod, by rw [hdiv]; exact hlim⟩, hdiv]
  exact chunkExact_cat h

/-- Decode-canonicity (P2) for fixed-element lists: an accepted buffer tiles
    exactly into sized chunks whose concatenation is the buffer, within the
    limit — partial chunks and trailing bytes are impossible. -/
theorem serializeSeq_decodeFixedList {k limit : Nat} {bs : List UInt8}
    {vs : List (List UInt8)} (h : decodeFixedList k limit bs = some vs) :
    Sized k vs ∧ serializeSeq vs = bs ∧ vs.length ≤ limit := by
  simp only [decodeFixedList] at h
  by_cases hc : bs.length % k = 0 ∧ bs.length / k ≤ limit
  · rw [if_pos hc] at h
    obtain ⟨hsz, hcat, hlen⟩ := chunkExact_sound h
    exact ⟨hsz, hcat, by rw [hlen]; exact hc.2⟩
  · rw [if_neg hc] at h
    exact absurd h (by simp)

/-! ## Vectors -/

/-- Roundtrip (P1) for fixed-element vectors. -/
theorem decodeFixedVector_serializeSeq {k n : Nat} {vs : List (List UInt8)}
    (h : Sized k vs) (hn : vs.length = n) :
    decodeFixedVector k n (serializeSeq vs) = some vs := by
  subst hn
  simp only [decodeFixedVector, serializeSeq]
  rw [if_pos (length_cat_sized h)]
  exact chunkExact_cat h

/-- Decode-canonicity (P2) for fixed-element vectors. -/
theorem serializeSeq_decodeFixedVector {k n : Nat} {bs : List UInt8}
    {vs : List (List UInt8)} (h : decodeFixedVector k n bs = some vs) :
    Sized k vs ∧ serializeSeq vs = bs ∧ vs.length = n := by
  simp only [decodeFixedVector] at h
  by_cases hc : bs.length = k * n
  · rw [if_pos hc] at h
    exact chunkExact_sound h
  · rw [if_neg hc] at h
    exact absurd h (by simp)

end FixedSeq
end Ssz
