import SszCert.Basic

/-!
# SSZ booleans

One byte, strictly `0x00` or `0x01`. The interesting content is canonicity:
every other byte value (`0x02`–`0xff`) is rejected, so a boolean has exactly
one accepted encoding.
-/

namespace Ssz
namespace Boolean

/-- Canonical SSZ boolean serialization. -/
def serializeBool (b : Bool) : List UInt8 :=
  [if b then 1 else 0]

/-- Strict SSZ boolean decoding: exactly one byte, strictly `0` or `1`. -/
def decodeBool : List UInt8 → Option Bool
  | [x] => if x = 0 then some false else if x = 1 then some true else none
  | _ => none

/-- Roundtrip (P1). -/
theorem decodeBool_serializeBool (b : Bool) :
    decodeBool (serializeBool b) = some b := by
  cases b <;> rfl

/-- Decode-canonicity (P2): an accepted byte string is exactly the canonical
    encoding — `0x02`–`0xff` and every non-1-byte input are rejected. -/
theorem serializeBool_decodeBool {bs : List UInt8} {b : Bool}
    (h : decodeBool bs = some b) : serializeBool b = bs := by
  match bs with
  | [] => simp [decodeBool] at h
  | x :: y :: rest => simp [decodeBool] at h
  | [x] =>
    simp only [decodeBool] at h
    by_cases h0 : x = 0
    · rw [if_pos h0] at h
      injection h with hb
      subst hb h0
      rfl
    · rw [if_neg h0] at h
      by_cases h1 : x = 1
      · rw [if_pos h1] at h
        injection h with hb
        subst hb h1
        rfl
      · rw [if_neg h1] at h
        exact absurd h (by simp)

end Boolean
end Ssz
