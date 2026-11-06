import SszCert.Schema

/-!
# Lists of variable-size elements

SSZ `List[T, N]` with variable-size `T` serializes as an offset table (4 bytes
per element) followed by the element bytes — structurally a container whose
schema is `count × variable`. The twist that makes it a *different* theorem
from `Schema.lean`: the element count is not part of the schema; the decoder
must **derive it from the wire** as `firstOffset / 4`, checking:

* empty buffer ⇔ empty list (the only encoding of zero elements);
* the first offset is a multiple of 4 (else the "offset table" would end
  inside an offset cell — the misalignment rejection);
* the derived count respects the list limit;
* and then the full container offset discipline of `Schema.lean`, which in
  particular forces `firstOffset = 4 * count` — the self-consistency of the
  count derivation.

We formalize this by *instantiating* the schema-generic theorems at the
all-variable schema `schemaOf n := replicate n .var`, so every offset-table
property is inherited rather than re-proved.

Main results:
* `decodeList_serializeList` — roundtrip (P1)
* `serializeList_decodeList` — decode-canonicity (P2): an accepted buffer is
  exactly the canonical list serialization, and the decoded length respects
  the limit. Unconditional (no representability hypothesis).
* `serializeList_injective`, `decodeList_unique_encoding`
* Vectors of variable-size elements come for free (`serializeVector` /
  `decodeVector` corollaries): their count is schema-fixed, so they are
  *directly* the `Schema.lean` theorems at `schemaOf n`.
-/

namespace Ssz
namespace VarList

open Sch

/-- The all-variable container schema with `n` fields. -/
def schemaOf (n : Nat) : Schema := List.replicate n .var

/-- Canonical SSZ serialization of a list of variable-size elements. -/
def serializeList (vs : Values) : List UInt8 :=
  Sch.serialize (schemaOf vs.length) vs

/-- Strict SSZ decoding of a list of variable-size elements with limit
    `limit`. The element count is derived from the first offset. The offset
    is checked against the buffer before the derived schema is built: that
    check is redundant for the result (`parseFixed` needs `4 * count` bytes
    anyway) but keeps a hostile offset from materializing a huge schema. -/
def decodeList (limit : Nat) (bs : List UInt8) : Option Values :=
  if bs = [] then
    some []
  else if 4 ≤ bs.length then
    if decodeLE (bs.take 4) % 4 = 0 ∧ decodeLE (bs.take 4) / 4 ≤ limit
        ∧ decodeLE (bs.take 4) ≤ bs.length then
      Sch.decode (schemaOf (decodeLE (bs.take 4) / 4)) bs
    else
      none
  else
    none

/-! ## Schema instantiation lemmas -/

theorem fixedLen_schemaOf (n : Nat) : fixedLen (schemaOf n) = 4 * n := by
  induction n with
  | zero => rfl
  | succ n ih =>
    simp only [schemaOf, List.replicate_succ, fixedLen] at *
    omega

theorem wellTyped_schemaOf (vs : Values) : WellTyped (schemaOf vs.length) vs := by
  induction vs with
  | nil => exact WellTyped.nil
  | cons v vs ih =>
    simp only [List.length_cons, schemaOf, List.replicate_succ]
    exact WellTyped.var ih

theorem length_of_wellTyped_schemaOf {n : Nat} {vs : Values}
    (h : WellTyped (schemaOf n) vs) : vs.length = n := by
  induction n generalizing vs with
  | zero =>
    cases h with
    | nil => rfl
  | succ n ih =>
    rw [schemaOf, List.replicate_succ] at h
    cases h with
    | var h' => simp [ih h']

theorem varVals_schemaOf (vs : Values) : varVals (schemaOf vs.length) vs = vs := by
  induction vs with
  | nil => rfl
  | cons v vs ih =>
    simp only [List.length_cons, schemaOf, List.replicate_succ, varVals]
    rw [show List.replicate vs.length FieldTy.var = schemaOf vs.length from rfl, ih]

theorem heapLen_schemaOf (vs : Values) :
    heapLen (schemaOf vs.length) vs = (cat vs).length := by
  simp [heapLen, varVals_schemaOf]

/-- The first 4 bytes of a nonempty list serialization are the encoded first
    offset `4 * count`. -/
theorem take4_serializeList (v : List UInt8) (vs : Values) :
    (serializeList (v :: vs)).take 4 = encodeLE (4 * (v :: vs).length) 4 := by
  rw [← fixedLen_schemaOf (v :: vs).length]
  simp only [serializeList, Sch.serialize,
    show schemaOf (v :: vs).length = .var :: schemaOf vs.length by
      simp [schemaOf, List.replicate_succ]]
  simp only [serializeFixed, List.append_assoc]
  rw [List.take_append_of_le_length (by simp),
    List.take_of_length_le (by simp)]

theorem length_serializeList (vs : Values) :
    (serializeList vs).length = 4 * vs.length + (cat vs).length := by
  have h1 := length_serializeFixed (wellTyped_schemaOf vs) (fixedLen (schemaOf vs.length))
  rw [fixedLen_schemaOf] at h1
  simp [serializeList, Sch.serialize, fixedLen_schemaOf, h1, varVals_schemaOf]

/-! ## Roundtrip -/

/-- **Roundtrip (P1) for lists of variable-size elements**: decoding the
    canonical serialization recovers the list, for any count within the limit
    and offsets representable in 4 bytes. -/
theorem decodeList_serializeList (vs : Values) (limit : Nat)
    (hlim : vs.length ≤ limit)
    (hrep : 4 * vs.length + (cat vs).length < 256 ^ 4) :
    decodeList limit (serializeList vs) = some vs := by
  cases vs with
  | nil =>
    simp [decodeList, serializeList, schemaOf, Sch.serialize, serializeFixed,
      varVals, cat]
  | cons v vs' =>
    have hlen : (serializeList (v :: vs')).length
        = 4 * (v :: vs').length + (cat (v :: vs')).length :=
      length_serializeList _
    have h4 : 4 ≤ (serializeList (v :: vs')).length := by
      simp only [List.length_cons] at hlen
      omega
    have hne : serializeList (v :: vs') ≠ [] := by
      intro he
      rw [he] at h4
      simp at h4
    have ho : decodeLE ((serializeList (v :: vs')).take 4) = 4 * (v :: vs').length := by
      rw [take4_serializeList]
      exact decodeLE_encodeLE _ 4 (by omega)
    have hdiv : 4 * (v :: vs').length / 4 = (v :: vs').length := by omega
    have hdec : Sch.decode (schemaOf (v :: vs').length) (serializeList (v :: vs'))
        = some (v :: vs') := by
      apply Sch.decode_serialize (wellTyped_schemaOf _)
      rw [fixedLen_schemaOf, heapLen_schemaOf]
      exact hrep
    unfold decodeList
    rw [if_neg hne, if_pos h4, ho,
      if_pos ⟨by omega, by omega, by simp only [List.length_cons] at hlen ⊢; omega⟩, hdiv]
    exact hdec

/-! ## Canonicity -/

/-- **Decode-canonicity (P2) for lists of variable-size elements — no side
    conditions**: any buffer the strict decoder accepts is exactly the
    canonical serialization of the decoded list, whose length respects the
    limit. In particular the wire-derived element count (`firstOffset / 4`)
    is forced to be self-consistent: misaligned first offsets, offset tables
    disagreeing with the derived count, gap bytes, and trailing bytes are all
    impossible. -/
theorem serializeList_decodeList {limit : Nat} {bs : List UInt8} {vs : Values}
    (h : decodeList limit bs = some vs) :
    serializeList vs = bs ∧ vs.length ≤ limit := by
  unfold decodeList at h
  by_cases hnil : bs = []
  · rw [if_pos hnil] at h
    simp only [Option.some.injEq] at h
    subst h
    exact ⟨by simp [serializeList, schemaOf, Sch.serialize, serializeFixed,
      varVals, cat, hnil], by simp⟩
  · rw [if_neg hnil] at h
    by_cases h4 : 4 ≤ bs.length
    · rw [if_pos h4] at h
      by_cases hcond : decodeLE (bs.take 4) % 4 = 0
          ∧ decodeLE (bs.take 4) / 4 ≤ limit ∧ decodeLE (bs.take 4) ≤ bs.length
      · rw [if_pos hcond] at h
        obtain ⟨hwt, hser⟩ := Sch.serialize_decode h
        have hlen : vs.length = decodeLE (bs.take 4) / 4 :=
          length_of_wellTyped_schemaOf hwt
        constructor
        · rw [serializeList, hlen]
          exact hser
        · rw [hlen]
          exact hcond.2.1
      · rw [if_neg hcond] at h
        exact absurd h (by simp)
    · rw [if_neg h4] at h
      exact absurd h (by simp)

/-! ## Corollaries -/

/-- Injectivity (P3) for lists. -/
theorem serializeList_injective {vs₁ vs₂ : Values} (limit : Nat)
    (h₁ : vs₁.length ≤ limit) (h₂ : vs₂.length ≤ limit)
    (hr₁ : 4 * vs₁.length + (cat vs₁).length < 256 ^ 4)
    (hr₂ : 4 * vs₂.length + (cat vs₂).length < 256 ^ 4)
    (h : serializeList vs₁ = serializeList vs₂) : vs₁ = vs₂ := by
  have e := congrArg (decodeList limit) h
  rw [decodeList_serializeList vs₁ limit h₁ hr₁,
    decodeList_serializeList vs₂ limit h₂ hr₂] at e
  injection e

/-- Non-malleability: a list has at most one accepted encoding. -/
theorem decodeList_unique_encoding {limit : Nat} {b₁ b₂ : List UInt8}
    {vs : Values} (h₁ : decodeList limit b₁ = some vs)
    (h₂ : decodeList limit b₂ = some vs) : b₁ = b₂ := by
  obtain ⟨e₁, _⟩ := serializeList_decodeList h₁
  obtain ⟨e₂, _⟩ := serializeList_decodeList h₂
  rw [← e₁, ← e₂]

/-! ## Vectors of variable-size elements

A vector's element count is fixed by the schema, so the container theorems
apply directly at the all-variable schema — no wire-derived count, no new
proof content. -/

def serializeVector (n : Nat) (vs : Values) : List UInt8 :=
  Sch.serialize (schemaOf n) vs

def decodeVector (n : Nat) (bs : List UInt8) : Option Values :=
  Sch.decode (schemaOf n) bs

theorem decodeVector_serializeVector {n : Nat} {vs : Values}
    (hlen : vs.length = n)
    (hrep : 4 * n + (cat vs).length < 256 ^ 4) :
    decodeVector n (serializeVector n vs) = some vs := by
  subst hlen
  apply Sch.decode_serialize (wellTyped_schemaOf vs)
  rw [fixedLen_schemaOf, heapLen_schemaOf]
  exact hrep

theorem serializeVector_decodeVector {n : Nat} {bs : List UInt8} {vs : Values}
    (h : decodeVector n bs = some vs) :
    vs.length = n ∧ serializeVector n vs = bs := by
  obtain ⟨hwt, hser⟩ := Sch.serialize_decode h
  exact ⟨length_of_wellTyped_schemaOf hwt, hser⟩

end VarList
end Ssz
