import SszCert.Basic

/-!
# Schema-generic variable-offset container theorems

Generalizes `Container.lean` from one concrete shape to *arbitrary* SSZ
container schemas: any interleaving of fixed-size fields and variable-size
fields, with the full SSZ offset table.

A schema is a list of field types; a value is one byte string per field
(leaf serialization is handled by the primitive layer — `Basic.lean` proves
its roundtrip/canonicity, and composition is byte-string concatenation, so
this layer captures exactly the offset mechanics that no prior formalization
has proved: eth2.0-dafny stopped before this case, SizzLean proves only a
fixed-size cut).

Serialization: the fixed part contains fixed fields inline and a 4-byte
offset per variable field; offsets are absolute positions of each variable
field's bytes, laid out contiguously after the fixed part.

The strict decoder enforces the full canonical offset discipline:
* first offset equals the fixed-part length (no ghost gap before the heap);
* offsets are monotone (no overlapping/backward regions);
* regions tile the remainder of the buffer exactly (no trailing bytes, no
  gaps between regions);
* with no variable fields, the buffer length equals the fixed length exactly.

Main theorems:
* `decode_serialize` — roundtrip (P1) for every schema
* `serialize_decode` — decode-canonicity (P2) for every schema: an accepted
  byte string *is* the serialization of its decoded value (the
  Ghost-in-the-Block property, schema-generically)
* `serialize_injective` — injectivity (P3)
* `decode_unique_encoding` — non-malleability: two buffers decoding to the
  same value are equal
* `ghost_rejected` — generic gap-byte rejection
-/

namespace Ssz
namespace Sch

/-- A container field: fixed-size of `n` bytes, or variable-size. -/
inductive FieldTy where
  | fx (n : Nat)
  | var
  deriving DecidableEq, Repr

abbrev Schema := List FieldTy

/-- One byte string per field. -/
abbrev Values := List (List UInt8)

/-- Length of the fixed part: fixed fields inline, 4 bytes per offset. -/
def fixedLen : Schema → Nat
  | [] => 0
  | .fx n :: s => n + fixedLen s
  | .var :: s => 4 + fixedLen s

/-- Well-typed values: one entry per field, fixed fields with exact length. -/
inductive WellTyped : Schema → Values → Prop where
  | nil : WellTyped [] []
  | fx {n : Nat} {s : Schema} {v : List UInt8} {vs : Values} :
      v.length = n → WellTyped s vs → WellTyped (.fx n :: s) (v :: vs)
  | var {s : Schema} {v : List UInt8} {vs : Values} :
      WellTyped s vs → WellTyped (.var :: s) (v :: vs)

/-- Concatenation of byte strings. -/
def cat : List (List UInt8) → List UInt8
  | [] => []
  | r :: rs => r ++ cat rs

/-- The variable-field values, in schema order. -/
def varVals : Schema → Values → Values
  | .var :: s, v :: vs => v :: varVals s vs
  | .fx _ :: s, _ :: vs => varVals s vs
  | _, _ => []

/-- Total length of the variable section (the heap). -/
def heapLen (s : Schema) (vs : Values) : Nat := (cat (varVals s vs)).length

/-- The fixed part of the serialization. `o` is the running absolute offset
    of the next variable field's bytes; the initial call uses `fixedLen s`. -/
def serializeFixed : Schema → Values → Nat → List UInt8
  | .fx _ :: s, v :: vs, o => v ++ serializeFixed s vs o
  | .var :: s, v :: vs, o => encodeLE o 4 ++ serializeFixed s vs (o + v.length)
  | _, _, _ => []

/-- SSZ container serialization: fixed part, then the heap. -/
def serialize (s : Schema) (vs : Values) : List UInt8 :=
  serializeFixed s vs (fixedLen s) ++ cat (varVals s vs)

/-! ## Decoder -/

/-- Parsed fixed-part item: a fixed cell's bytes, or a decoded offset. -/
inductive Pre where
  | cell (bytes : List UInt8)
  | off (o : Nat)
  deriving DecidableEq, Repr

/-- Phase 1: parse the fixed part left-to-right. Consumes exactly
    `fixedLen s` bytes of the prefix; fails on underflow. -/
def parseFixed : Schema → List UInt8 → Option (List Pre)
  | [], _ => some []
  | .fx n :: s, bs =>
    if n ≤ bs.length then
      (parseFixed s (bs.drop n)).map (Pre.cell (bs.take n) :: ·)
    else none
  | .var :: s, bs =>
    if 4 ≤ bs.length then
      (parseFixed s (bs.drop 4)).map (Pre.off (decodeLE (bs.take 4)) :: ·)
    else none

/-- The declared offsets, in order. -/
def offsetsOf : List Pre → List Nat
  | [] => []
  | .off o :: ps => o :: offsetsOf ps
  | .cell _ :: ps => offsetsOf ps

/-- Phase 2: slice the variable regions between consecutive offsets (the last
    region runs to the end of the buffer), enforcing monotonicity and bounds. -/
def sliceRegions : List Nat → List UInt8 → Option (List (List UInt8))
  | [], _ => some []
  | [o], bs => if o ≤ bs.length then some [bs.drop o] else none
  | o₁ :: o₂ :: os, bs =>
    if o₁ ≤ o₂ then
      (sliceRegions (o₂ :: os) bs).map (((bs.take o₂).drop o₁) :: ·)
    else none

/-- Reassemble the value list: fixed cells verbatim, variable slots filled
    from the sliced regions in order. -/
def fillVars : List Pre → List (List UInt8) → Values
  | .cell b :: ps, rs => b :: fillVars ps rs
  | .off _ :: ps, r :: rs => r :: fillVars ps rs
  | _, _ => []

/-- Strict, canonical SSZ container decoding. -/
def decode (s : Schema) (bs : List UInt8) : Option Values :=
  match parseFixed s bs with
  | none => none
  | some pres =>
    match offsetsOf pres with
    | [] => if bs.length = fixedLen s then some (fillVars pres []) else none
    | o₁ :: os =>
      if o₁ = fixedLen s then
        (sliceRegions (o₁ :: os) bs).map (fillVars pres)
      else none

/-! ## Specification helpers -/

/-- The offset table a canonical encoder emits for regions `rs` based at `o`.
    `offsFrom` is the heart of the canonicity argument: the decoder accepts a
    buffer iff the *declared* offsets equal these *recomputed* ones. -/
def offsFrom (o : Nat) : List (List UInt8) → List Nat
  | [] => []
  | r :: rs => o :: offsFrom (o + r.length) rs

/-- The parse result a canonical encoding produces. -/
def presOf : Schema → Values → Nat → List Pre
  | .fx _ :: s, v :: vs, o => .cell v :: presOf s vs o
  | .var :: s, v :: vs, o => .off o :: presOf s vs (o + v.length)
  | _, _, _ => []

/-- Byte image of a parsed fixed part. -/
def bytesOf : List Pre → List UInt8
  | [] => []
  | .cell b :: ps => b ++ bytesOf ps
  | .off o :: ps => encodeLE o 4 ++ bytesOf ps

/-- Shape agreement between a schema and a parse result. -/
inductive Matches : Schema → List Pre → Prop where
  | nil : Matches [] []
  | cell {n : Nat} {s : Schema} {b : List UInt8} {ps : List Pre} :
      b.length = n → Matches s ps → Matches (.fx n :: s) (.cell b :: ps)
  | off {s : Schema} {o : Nat} {ps : List Pre} :
      Matches s ps → Matches (.var :: s) (.off o :: ps)

/-! ## Basic lemmas -/

theorem cat_append (rs₁ rs₂ : List (List UInt8)) :
    cat (rs₁ ++ rs₂) = cat rs₁ ++ cat rs₂ := by
  induction rs₁ with
  | nil => simp [cat]
  | cons r rs ih => simp [cat, ih]

@[simp]
theorem heapLen_fx (n : Nat) (s : Schema) (v : List UInt8) (vs : Values) :
    heapLen (.fx n :: s) (v :: vs) = heapLen s vs := by
  simp [heapLen, varVals]

@[simp]
theorem heapLen_var (s : Schema) (v : List UInt8) (vs : Values) :
    heapLen (.var :: s) (v :: vs) = v.length + heapLen s vs := by
  simp [heapLen, varVals, cat]

theorem length_serializeFixed {s : Schema} {vs : Values}
    (h : WellTyped s vs) (o : Nat) :
    (serializeFixed s vs o).length = fixedLen s := by
  induction h generalizing o with
  | nil => rfl
  | fx hlen _ ih => simp [serializeFixed, fixedLen, hlen, ih]
  | var _ ih => simp [serializeFixed, fixedLen, ih]

theorem offsFrom_eq_nil {o : Nat} {rs : List (List UInt8)}
    (h : offsFrom o rs = []) : rs = [] := by
  cases rs with
  | nil => rfl
  | cons r rs => simp [offsFrom] at h

/-! ## Encoding-side lemmas (towards roundtrip) -/

/-- Parsing a canonical encoding (with any trailing bytes) succeeds and
    produces the canonical parse result. -/
theorem parseFixed_serializeFixed {s : Schema} {vs : Values}
    (h : WellTyped s vs) (o : Nat) (rest : List UInt8)
    (hrep : o + heapLen s vs < 256 ^ 4) :
    parseFixed s (serializeFixed s vs o ++ rest) = some (presOf s vs o) := by
  induction h generalizing o rest with
  | nil => rfl
  | @fx n s v vs hlen hwt ih =>
    have hle : n ≤ (serializeFixed (.fx n :: s) (v :: vs) o ++ rest).length := by
      simp [serializeFixed, hlen]
    simp only [parseFixed, serializeFixed, List.append_assoc]
    rw [if_pos (by simpa [serializeFixed, hlen, List.append_assoc] using hle)]
    have htake : (v ++ (serializeFixed s vs o ++ rest)).take n = v := by
      rw [List.take_append_of_le_length (by omega), List.take_of_length_le (by omega)]
    have hdrop : (v ++ (serializeFixed s vs o ++ rest)).drop n
        = serializeFixed s vs o ++ rest := by
      rw [List.drop_append_of_le_length (by omega)]
      have : v.drop n = [] := by
        apply List.drop_eq_nil_of_le
        omega
      simp [this]
    rw [htake, hdrop, ih o rest (by simpa using hrep)]
    simp [presOf]
  | @var s v vs hwt ih =>
    have hrep' : (o + v.length) + heapLen s vs < 256 ^ 4 := by
      have := hrep
      simp only [heapLen_var] at this
      omega
    have hlen4 : (encodeLE o 4).length = 4 := length_encodeLE ..
    simp only [parseFixed, serializeFixed, List.append_assoc]
    rw [if_pos (by simp [hlen4])]
    have htake : (encodeLE o 4 ++ (serializeFixed s vs (o + v.length) ++ rest)).take 4
        = encodeLE o 4 := by
      rw [List.take_append_of_le_length (by omega), List.take_of_length_le (by omega)]
    have hdrop : (encodeLE o 4 ++ (serializeFixed s vs (o + v.length) ++ rest)).drop 4
        = serializeFixed s vs (o + v.length) ++ rest := by
      rw [List.drop_append_of_le_length (by omega)]
      have : (encodeLE o 4).drop 4 = [] := by
        apply List.drop_eq_nil_of_le
        omega
      simp [this]
    rw [htake, hdrop, ih (o + v.length) rest hrep']
    have hoval : decodeLE (encodeLE o 4) = o :=
      decodeLE_encodeLE o 4 (by simp only [heapLen_var] at hrep; omega)
    simp [presOf, hoval]

/-- The canonical parse result declares exactly the recomputed offsets. -/
theorem offsetsOf_presOf {s : Schema} {vs : Values}
    (h : WellTyped s vs) (o : Nat) :
    offsetsOf (presOf s vs o) = offsFrom o (varVals s vs) := by
  induction h generalizing o with
  | nil => rfl
  | fx _ _ ih => simp [presOf, offsetsOf, varVals, ih]
  | var _ ih => simp [presOf, offsetsOf, varVals, offsFrom, ih]

/-- Refilling the canonical parse result with the variable values restores
    the original values. -/
theorem fillVars_presOf {s : Schema} {vs : Values}
    (h : WellTyped s vs) (o : Nat) :
    fillVars (presOf s vs o) (varVals s vs) = vs := by
  induction h generalizing o with
  | nil => rfl
  | fx _ _ ih => simp [presOf, varVals, fillVars, ih]
  | var _ ih => simp [presOf, varVals, fillVars, ih]

/-- Slicing recomputed offsets over a buffer whose tail is the concatenated
    regions recovers exactly the regions. -/
theorem sliceRegions_offsFrom (rs : List (List UInt8)) (o : Nat)
    (bs : List UInt8) (ho : o ≤ bs.length) (hcat : bs.drop o = cat rs)
    (hne : rs ≠ []) :
    sliceRegions (offsFrom o rs) bs = some rs := by
  induction rs generalizing o with
  | nil => exact absurd rfl hne
  | cons r rs ih =>
    cases rs with
    | nil =>
      simp only [offsFrom, sliceRegions, if_pos ho]
      simp only [cat] at hcat
      simp [hcat]
    | cons r' rs' =>
      have hdropo : bs.drop o = r ++ cat (r' :: rs') := by
        rw [hcat]; rfl
      have hbound : o + r.length ≤ bs.length := by
        have hc := congrArg List.length hdropo
        simp only [List.length_drop, List.length_append] at hc
        omega
      have hdrop' : bs.drop (o + r.length) = cat (r' :: rs') := by
        have h1 : (bs.drop o).drop r.length = bs.drop (o + r.length) :=
          List.drop_drop
        rw [← h1, hdropo, List.drop_append_of_le_length (Nat.le_refl r.length),
          List.drop_length, List.nil_append]
      have ihr := ih (o + r.length) hbound hdrop' (by simp)
      simp only [offsFrom] at ihr
      have hslice : (bs.take (o + r.length)).drop o = r := by
        rw [List.drop_take]
        have h2 : o + r.length - o = r.length := by omega
        rw [h2, hdropo, List.take_append_of_le_length (Nat.le_refl r.length),
          List.take_length]
      simp only [offsFrom, sliceRegions]
      rw [if_pos (by omega : o ≤ o + r.length), ihr]
      simp [hslice]

/-! ## Roundtrip -/

/-- **Roundtrip (P1), schema-generic**: decoding a canonical serialization of
    any well-typed value recovers the value. The representability bound
    requires the offsets to fit their 4 bytes, exactly as in real SSZ. -/
theorem decode_serialize {s : Schema} {vs : Values} (h : WellTyped s vs)
    (hrep : fixedLen s + heapLen s vs < 256 ^ 4) :
    decode s (serialize s vs) = some vs := by
  have hparse : parseFixed s (serialize s vs) = some (presOf s vs (fixedLen s)) :=
    parseFixed_serializeFixed h (fixedLen s) (cat (varVals s vs)) hrep
  have hoffs := offsetsOf_presOf h (fixedLen s)
  have hfill := fillVars_presOf h (fixedLen s)
  have hflen : (serializeFixed s vs (fixedLen s)).length = fixedLen s :=
    length_serializeFixed h _
  cases hvv : varVals s vs with
  | nil =>
    have hlen : (serialize s vs).length = fixedLen s := by
      simp [serialize, hvv, cat, hflen]
    have hfill' : fillVars (presOf s vs (fixedLen s)) [] = vs := by
      rw [show ([] : Values) = varVals s vs from hvv.symm]
      exact hfill
    simp only [decode, hparse, hoffs, hvv, offsFrom, hlen, if_pos, hfill']
  | cons r rs =>
    have hdropF : (serialize s vs).drop (fixedLen s) = cat (varVals s vs) := by
      simp only [serialize]
      have h1 : (serializeFixed s vs (fixedLen s)).drop (fixedLen s) = [] := by
        apply List.drop_eq_nil_of_le
        omega
      rw [List.drop_append_of_le_length (Nat.le_of_eq hflen.symm), h1,
        List.nil_append]
    have hFle : fixedLen s ≤ (serialize s vs).length := by
      simp [serialize, hflen]
    have hslice := sliceRegions_offsFrom (r :: rs) (fixedLen s) (serialize s vs)
      hFle (by rw [hdropF, hvv]) (by simp)
    simp only [offsFrom] at hslice
    have hfill' : fillVars (presOf s vs (fixedLen s)) (r :: rs) = vs := by
      rw [show (r :: rs : Values) = varVals s vs from hvv.symm]
      exact hfill
    simp only [decode, hparse, hoffs, hvv, offsFrom]
    rw [if_pos trivial, hslice]
    simp [hfill']

/-! ## Decoding-side lemmas (towards canonicity) -/

/-- Soundness of fixed-part parsing: the parse matches the schema, its byte
    image is exactly the input's fixed prefix, and the input covers it. -/
theorem parseFixed_sound {s : Schema} {bs : List UInt8} {pres : List Pre}
    (h : parseFixed s bs = some pres) :
    Matches s pres ∧ bytesOf pres = bs.take (fixedLen s)
      ∧ fixedLen s ≤ bs.length := by
  induction s generalizing bs pres with
  | nil =>
    simp only [parseFixed, Option.some.injEq] at h
    subst h
    exact ⟨Matches.nil, by simp [bytesOf, fixedLen], by simp [fixedLen]⟩
  | cons f s ih =>
    cases f with
    | fx n =>
      simp only [parseFixed] at h
      by_cases hle : n ≤ bs.length
      · rw [if_pos hle] at h
        cases hp : parseFixed s (bs.drop n) with
        | none => rw [hp] at h; simp at h
        | some ps =>
          rw [hp] at h
          simp only [Option.map_some, Option.some.injEq] at h
          subst h
          obtain ⟨hm, hb, hl⟩ := ih hp
          refine ⟨Matches.cell ?_ hm, ?_, ?_⟩
          · rw [List.length_take]; omega
          · simp only [bytesOf, hb, fixedLen]
            exact List.take_add.symm
          · simp only [fixedLen]
            rw [List.length_drop] at hl
            omega
      · rw [if_neg hle] at h
        exact absurd h (by simp)
    | var =>
      simp only [parseFixed] at h
      by_cases hle : 4 ≤ bs.length
      · rw [if_pos hle] at h
        cases hp : parseFixed s (bs.drop 4) with
        | none => rw [hp] at h; simp at h
        | some ps =>
          rw [hp] at h
          simp only [Option.map_some, Option.some.injEq] at h
          subst h
          obtain ⟨hm, hb, hl⟩ := ih hp
          have htlen : (bs.take 4).length = 4 := by
            rw [List.length_take]; omega
          refine ⟨Matches.off hm, ?_, ?_⟩
          · simp only [bytesOf, hb, fixedLen]
            have henc : encodeLE (decodeLE (bs.take 4)) 4 = bs.take 4 := by
              have h2 := encodeLE_decodeLE (bs.take 4)
              rwa [htlen] at h2
            rw [henc]
            exact List.take_add.symm
          · simp only [fixedLen]
            rw [List.length_drop] at hl
            omega
      · rw [if_neg hle] at h
        exact absurd h (by simp)

/-- Soundness of region slicing: the regions tile the buffer from the first
    offset to the end, and — the canonicity crux — the *declared* offsets are
    exactly the offsets *recomputed* from the region lengths. -/
theorem sliceRegions_sound {os : List Nat} {o₁ : Nat} {bs : List UInt8}
    {rs : List (List UInt8)}
    (h : sliceRegions (o₁ :: os) bs = some rs) :
    o₁ ≤ bs.length ∧ cat rs = bs.drop o₁ ∧ offsFrom o₁ rs = o₁ :: os := by
  induction os generalizing o₁ rs with
  | nil =>
    simp only [sliceRegions] at h
    by_cases hle : o₁ ≤ bs.length
    · rw [if_pos hle] at h
      simp only [Option.some.injEq] at h
      subst h
      exact ⟨hle, by simp [cat], by simp [offsFrom]⟩
    · rw [if_neg hle] at h
      exact absurd h (by simp)
  | cons o₂ os ih =>
    simp only [sliceRegions] at h
    by_cases hle : o₁ ≤ o₂
    · rw [if_pos hle] at h
      cases hp : sliceRegions (o₂ :: os) bs with
      | none => rw [hp] at h; simp at h
      | some rs' =>
        rw [hp] at h
        simp only [Option.map_some, Option.some.injEq] at h
        subst h
        obtain ⟨hb, hc, ho⟩ := ih hp
        have hslicelen : ((bs.take o₂).drop o₁).length = o₂ - o₁ := by
          rw [List.length_drop, List.length_take]
          omega
        have hsplit : (bs.take o₂).drop o₁ ++ bs.drop o₂ = bs.drop o₁ := by
          have h1 : (bs.take o₂).drop o₁ = (bs.drop o₁).take (o₂ - o₁) :=
            List.drop_take
          have h2 : bs.drop o₂ = (bs.drop o₁).drop (o₂ - o₁) := by
            rw [List.drop_drop]
            congr 1
            omega
          rw [h1, h2, List.take_append_drop]
        refine ⟨by omega, ?_, ?_⟩
        · simp only [cat, hc]
          exact hsplit
        · simp only [offsFrom, hslicelen]
          have h1 : o₁ + (o₂ - o₁) = o₂ := by omega
          rw [h1, ho]
    · rw [if_neg hle] at h
      exact absurd h (by simp)

/-- Reassembled fixed part: refilling a parse result and re-serializing with
    the recomputed offsets reproduces the parsed bytes — *provided* the
    declared offsets equal the recomputed ones. -/
theorem serializeFixed_fillVars {s : Schema} {pres : List Pre}
    (hm : Matches s pres) :
    ∀ {o : Nat} {rs : List (List UInt8)},
      offsetsOf pres = offsFrom o rs →
      serializeFixed s (fillVars pres rs) o = bytesOf pres := by
  induction hm with
  | nil =>
    intro o rs _
    simp [fillVars, serializeFixed, bytesOf]
  | @cell n s b ps hb hm ih =>
    intro o rs hoffs
    simp only [offsetsOf] at hoffs
    simp only [fillVars, serializeFixed, bytesOf]
    rw [ih hoffs]
  | @off s od ps hm ih =>
    intro o rs hoffs
    cases rs with
    | nil => simp [offsetsOf, offsFrom] at hoffs
    | cons r rs' =>
      simp only [offsetsOf, offsFrom, List.cons.injEq] at hoffs
      obtain ⟨rfl, hoffs'⟩ := hoffs
      simp only [fillVars, serializeFixed, bytesOf]
      rw [ih hoffs']

/-- Refilled variable values are exactly the regions. -/
theorem varVals_fillVars {s : Schema} {pres : List Pre}
    (hm : Matches s pres) :
    ∀ {o : Nat} {rs : List (List UInt8)},
      offsetsOf pres = offsFrom o rs →
      varVals s (fillVars pres rs) = rs := by
  induction hm with
  | nil =>
    intro o rs hoffs
    simp only [offsetsOf] at hoffs
    have := offsFrom_eq_nil hoffs.symm
    subst this
    simp [fillVars, varVals]
  | cell hb hm ih =>
    intro o rs hoffs
    simp only [offsetsOf] at hoffs
    simp only [fillVars, varVals]
    exact ih hoffs
  | @off s od ps hm ih =>
    intro o rs hoffs
    cases rs with
    | nil => simp [offsetsOf, offsFrom] at hoffs
    | cons r rs' =>
      simp only [offsetsOf, offsFrom, List.cons.injEq] at hoffs
      obtain ⟨rfl, hoffs'⟩ := hoffs
      simp only [fillVars, varVals]
      rw [ih hoffs']

/-- Refilled values are well-typed. -/
theorem wellTyped_fillVars {s : Schema} {pres : List Pre}
    (hm : Matches s pres) :
    ∀ {o : Nat} {rs : List (List UInt8)},
      offsetsOf pres = offsFrom o rs →
      WellTyped s (fillVars pres rs) := by
  induction hm with
  | nil =>
    intro o rs _
    simp only [fillVars]
    exact WellTyped.nil
  | cell hb hm ih =>
    intro o rs hoffs
    simp only [offsetsOf] at hoffs
    exact WellTyped.fx hb (ih hoffs)
  | @off s od ps hm ih =>
    intro o rs hoffs
    cases rs with
    | nil => simp [offsetsOf, offsFrom] at hoffs
    | cons r rs' =>
      simp only [offsetsOf, offsFrom, List.cons.injEq] at hoffs
      obtain ⟨rfl, hoffs'⟩ := hoffs
      exact WellTyped.var (ih hoffs')

/-! ## Canonicity -/

/-- **Decode-canonicity (P2), schema-generic — the Ghost-in-the-Block
    property**: any byte string accepted by the strict decoder is *exactly*
    the canonical serialization of its decoded value, for every container
    schema. No gap bytes, no non-minimal offsets, no overlapping regions, no
    trailing bytes — no non-canonical encoding of any shape is accepted. -/
theorem serialize_decode {s : Schema} {bs : List UInt8} {vs : Values}
    (h : decode s bs = some vs) :
    WellTyped s vs ∧ serialize s vs = bs := by
  unfold decode at h
  cases hp : parseFixed s bs with
  | none => simp [hp] at h
  | some pres =>
    simp only [hp] at h
    obtain ⟨hm, hbytes, hFle⟩ := parseFixed_sound hp
    cases hoc : offsetsOf pres with
    | nil =>
      simp only [hoc] at h
      by_cases hlen : bs.length = fixedLen s
      · rw [if_pos hlen] at h
        simp only [Option.some.injEq] at h
        subst h
        have hoffs : offsetsOf pres = offsFrom 0 ([] : List (List UInt8)) := by
          simp [hoc, offsFrom]
        refine ⟨wellTyped_fillVars hm hoffs, ?_⟩
        have hser := serializeFixed_fillVars hm (o := fixedLen s)
          (rs := ([] : List (List UInt8))) (by simp [hoc, offsFrom])
        have hvv := varVals_fillVars hm (o := fixedLen s)
          (rs := ([] : List (List UInt8))) (by simp [hoc, offsFrom])
        simp only [serialize, hser, hvv, cat, List.append_nil]
        rw [hbytes, ← hlen]
        exact List.take_length
      · rw [if_neg hlen] at h
        exact absurd h (by simp)
    | cons o₁ os =>
      simp only [hoc] at h
      by_cases ho₁ : o₁ = fixedLen s
      · rw [if_pos ho₁] at h
        cases hs : sliceRegions (o₁ :: os) bs with
        | none => simp [hs] at h
        | some rs =>
          simp only [hs, Option.map_some, Option.some.injEq] at h
          subst h
          obtain ⟨hob, hcat, hoffs⟩ := sliceRegions_sound hs
          have hoffs' : offsetsOf pres = offsFrom o₁ rs := by
            rw [hoc, hoffs]
          refine ⟨wellTyped_fillVars hm hoffs', ?_⟩
          have hser := serializeFixed_fillVars hm hoffs'
          have hvv := varVals_fillVars hm hoffs'
          simp only [serialize, hvv, ← ho₁, hser]
          rw [hbytes, ← ho₁, hcat]
          exact List.take_append_drop o₁ bs
      · rw [if_neg ho₁] at h
        exact absurd h (by simp)

/-! ## Corollaries -/

/-- Injectivity (P3) on well-typed, offset-representable values. -/
theorem serialize_injective {s : Schema} {vs₁ vs₂ : Values}
    (h₁ : WellTyped s vs₁) (h₂ : WellTyped s vs₂)
    (hr₁ : fixedLen s + heapLen s vs₁ < 256 ^ 4)
    (hr₂ : fixedLen s + heapLen s vs₂ < 256 ^ 4)
    (h : serialize s vs₁ = serialize s vs₂) : vs₁ = vs₂ := by
  have e := congrArg (decode s) h
  rw [decode_serialize h₁ hr₁, decode_serialize h₂ hr₂] at e
  injection e

/-- Non-malleability: a value has at most one accepted encoding. -/
theorem decode_unique_encoding {s : Schema} {b₁ b₂ : List UInt8} {vs : Values}
    (h₁ : decode s b₁ = some vs) (h₂ : decode s b₂ = some vs) : b₁ = b₂ := by
  obtain ⟨_, e₁⟩ := serialize_decode h₁
  obtain ⟨_, e₂⟩ := serialize_decode h₂
  rw [← e₁, ← e₂]

/-- Schema-generic Ghost-in-the-Block rejection: shifting the offsets by a
    nonempty gap `g` (inserted between fixed part and heap) yields a buffer
    the decoder rejects — for every schema with at least one variable field. -/
theorem ghost_rejected {s : Schema} {vs : Values} (h : WellTyped s vs)
    (g : List UInt8) (hg : g ≠ [])
    (hvar : varVals s vs ≠ [])
    (hrep : fixedLen s + g.length + heapLen s vs < 256 ^ 4) :
    decode s (serializeFixed s vs (fixedLen s + g.length)
      ++ (g ++ cat (varVals s vs))) = none := by
  have hparse := parseFixed_serializeFixed h (fixedLen s + g.length)
    (g ++ cat (varVals s vs)) (by omega)
  have hoffs := offsetsOf_presOf h (fixedLen s + g.length)
  cases hvv : varVals s vs with
  | nil => exact absurd hvv hvar
  | cons r rs =>
    have hne : ¬ (fixedLen s + g.length = fixedLen s) := by
      have hpos : 0 < g.length := List.length_pos_iff.mpr hg
      omega
    rw [hvv] at hparse hoffs
    simp only [decode, hparse, hoffs, offsFrom]
    simp only [hne, if_false]

end Sch
end Ssz
