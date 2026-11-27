import SszCert.Basic

/-!
# The variable-offset container — seed of the unproved theorem

A minimal SSZ container with one fixed field and one variable field:

    Ghost = { a : uint64, b : List[uint8] }

Serialization per the SSZ spec: `serializeLE(a) ++ offset(4 bytes) ++ b`,
where the offset of `b` must equal the fixed-part size (12).

This is precisely the shape whose decoder-side validation was missing in
fastssz/Prysm ("Ghost in the Block", 2024): accepting `offset > 12` leaves gap
bytes between the fixed part and `b` that are invisible to `hash_tree_root`,
splitting consensus. Neither eth2.0-dafny (issue #26) nor SizzLean has proved
the offset discipline for variable-size containers.

Theorems:

* `decode_serialize` — roundtrip (P1)
* `serialize_decode` — decode-canonicity (P2): any accepted byte string is
  *exactly* the serialization of its decoded value. This is the property whose
  violation was Ghost-in-the-Block; it implies gap bytes are rejected.
* `ghost_rejected` — the explicit corollary: any encoding with a nonempty
  ghost gap is rejected.
* `serialize_injective` — injectivity (P3), a corollary of roundtrip.
-/

namespace Ssz

/-- Fixed part: 8 bytes of `a` + 4 bytes of offset. -/
def fixedSize : Nat := 12

/-- The container value. `a` models a uint64 (bound carried in theorems where
    needed; the decoder only ever produces in-bounds values). -/
structure Ghost where
  a : Nat
  b : List UInt8
  deriving DecidableEq, Repr

/-- SSZ serialization: fixed field, then the 4-byte offset of the variable
    field (always `fixedSize` here), then the variable bytes. -/
def Ghost.serialize (c : Ghost) : List UInt8 :=
  encodeLE c.a 8 ++ encodeLE fixedSize 4 ++ c.b

/-- Strict SSZ decoding with the full offset discipline:
    * the input must cover the fixed part;
    * the first (only) offset must equal the fixed-part size — this is the
      check whose absence caused Ghost-in-the-Block. -/
def Ghost.decode (bs : List UInt8) : Option Ghost :=
  if bs.length < fixedSize then
    none
  else if decodeLE ((bs.drop 8).take 4) = fixedSize then
    some ⟨decodeLE (bs.take 8), bs.drop 12⟩
  else
    none

/-- Roundtrip (P1): decoding a serialization recovers the value. -/
theorem Ghost.decode_serialize (c : Ghost) (ha : c.a < 256 ^ 8) :
    Ghost.decode c.serialize = some c := by
  have hlen8 : (encodeLE c.a 8).length = 8 := length_encodeLE ..
  have hlen4 : (encodeLE fixedSize 4).length = 4 := length_encodeLE ..
  have hlen : c.serialize.length = 12 + c.b.length := by
    simp [Ghost.serialize, hlen8, hlen4]
    omega
  have htake8 : c.serialize.take 8 = encodeLE c.a 8 := by
    simp [Ghost.serialize, List.take_append, hlen8]
  have hdrop8 : c.serialize.drop 8 = encodeLE fixedSize 4 ++ c.b := by
    simp [Ghost.serialize, List.drop_append, hlen8]
  have hoff : (c.serialize.drop 8).take 4 = encodeLE fixedSize 4 := by
    simp [hdrop8, List.take_append, hlen4]
  have hdrop12 : c.serialize.drop 12 = c.b := by
    have h1 : c.serialize.drop 12 = (c.serialize.drop 8).drop 4 := by
      simp [List.drop_drop]
    simp [h1, hdrop8, List.drop_append, hlen4]
  have hoffval : decodeLE ((c.serialize.drop 8).take 4) = fixedSize := by
    rw [hoff]
    exact decodeLE_encodeLE fixedSize 4 (by decide)
  have haval : decodeLE (c.serialize.take 8) = c.a := by
    rw [htake8]
    exact decodeLE_encodeLE c.a 8 ha
  have hnl : ¬ (c.serialize.length < fixedSize) := by
    rw [hlen]
    simp only [fixedSize]
    omega
  unfold Ghost.decode
  rw [if_neg hnl, if_pos hoffval, haval, hdrop12]

/-- Decode-canonicity (P2, the Ghost-in-the-Block property): every accepted
    byte string is exactly the serialization of its decoded value. There is no
    room for gap bytes, non-minimal offsets, or any other non-canonical
    encoding. -/
theorem Ghost.serialize_decode {bs : List UInt8} {c : Ghost}
    (h : Ghost.decode bs = some c) : c.serialize = bs := by
  unfold Ghost.decode at h
  by_cases hlen : bs.length < fixedSize
  · rw [if_pos hlen] at h
    exact absurd h (by simp)
  · rw [if_neg hlen] at h
    by_cases hoff : decodeLE ((bs.drop 8).take 4) = fixedSize
    · rw [if_pos hoff] at h
      injection h with h
      have hlen' : 12 ≤ bs.length := by
        simp only [fixedSize] at hlen
        omega
      have htake8len : (bs.take 8).length = 8 := by
        rw [List.length_take]
        omega
      have hofflen : ((bs.drop 8).take 4).length = 4 := by
        rw [List.length_take, List.length_drop]
        omega
      -- The a-field bytes re-encode to themselves.
      have ha : encodeLE (decodeLE (bs.take 8)) 8 = bs.take 8 := by
        have h2 := encodeLE_decodeLE (bs.take 8)
        rwa [htake8len] at h2
      -- The offset bytes are exactly encodeLE 12 4.
      have hofb : encodeLE fixedSize 4 = (bs.drop 8).take 4 := by
        have h2 := encodeLE_decodeLE ((bs.drop 8).take 4)
        rw [hofflen, hoff] at h2
        exact h2
      have hsplit : bs.drop 12 = (bs.drop 8).drop 4 := by
        simp [List.drop_drop]
      rw [← h]
      show encodeLE (decodeLE (bs.take 8)) 8 ++ encodeLE fixedSize 4 ++ bs.drop 12 = bs
      rw [ha, hofb, hsplit, List.append_assoc, List.take_append_drop,
        List.take_append_drop]
    · rw [if_neg hoff] at h
      exact absurd h (by simp)

/-- Injectivity (P3) on the uint64-bounded domain, from the roundtrip. -/
theorem Ghost.serialize_injective {c₁ c₂ : Ghost}
    (h₁ : c₁.a < 256 ^ 8) (h₂ : c₂.a < 256 ^ 8)
    (h : c₁.serialize = c₂.serialize) : c₁ = c₂ := by
  have e := congrArg Ghost.decode h
  rw [Ghost.decode_serialize c₁ h₁, Ghost.decode_serialize c₂ h₂] at e
  injection e

/-- Decoded values always satisfy the uint64 bound: the decoder is closed over
    the domain on which serialize is injective. -/
theorem Ghost.decode_bound {bs : List UInt8} {c : Ghost}
    (h : Ghost.decode bs = some c) : c.a < 256 ^ 8 := by
  unfold Ghost.decode at h
  by_cases hlen : bs.length < fixedSize
  · rw [if_pos hlen] at h
    exact absurd h (by simp)
  · rw [if_neg hlen] at h
    by_cases hoff : decodeLE ((bs.drop 8).take 4) = fixedSize
    · rw [if_pos hoff] at h
      injection h with h
      have hlen' : 12 ≤ bs.length := by
        simp only [fixedSize] at hlen
        omega
      have htake8len : (bs.take 8).length = 8 := by
        rw [List.length_take]
        omega
      have hlt := decodeLE_lt (bs.take 8)
      rw [htake8len] at hlt
      rw [← h]
      exact hlt
    · rw [if_neg hoff] at h
      exact absurd h (by simp)

/-- Ghost-in-the-Block, explicitly: an encoding carrying a nonempty gap `g`
    between the fixed part and the variable bytes (declared via offset
    `12 + g.length`, representable in its 4 bytes) is rejected. -/
theorem Ghost.ghost_rejected (a : Nat) (g b : List UInt8) (hg : g ≠ [])
    (hrep : fixedSize + g.length < 256 ^ 4) :
    Ghost.decode
      (encodeLE a 8 ++ encodeLE (fixedSize + g.length) 4 ++ (g ++ b))
      = none := by
  have hlen8 : (encodeLE a 8).length = 8 := length_encodeLE ..
  have hlen4 : (encodeLE (fixedSize + g.length) 4).length = 4 := length_encodeLE ..
  have hdrop8 :
      (encodeLE a 8 ++ encodeLE (fixedSize + g.length) 4 ++ (g ++ b)).drop 8
        = encodeLE (fixedSize + g.length) 4 ++ (g ++ b) := by
    simp [List.drop_append, hlen8]
  have hoff :
      ((encodeLE a 8 ++ encodeLE (fixedSize + g.length) 4 ++ (g ++ b)).drop 8).take 4
        = encodeLE (fixedSize + g.length) 4 := by
    simp [hdrop8, List.take_append, hlen4]
  have hglen : 0 < g.length := List.length_pos_iff.mpr hg
  have hoffval :
      decodeLE (((encodeLE a 8 ++ encodeLE (fixedSize + g.length) 4
        ++ (g ++ b)).drop 8).take 4) = fixedSize + g.length := by
    rw [hoff]
    exact decodeLE_encodeLE _ 4 hrep
  unfold Ghost.decode
  by_cases hlen :
      (encodeLE a 8 ++ encodeLE (fixedSize + g.length) 4 ++ (g ++ b)).length
        < fixedSize
  · rw [if_pos hlen]
  · rw [if_neg hlen, if_neg (by rw [hoffval]; omega)]

end Ssz
