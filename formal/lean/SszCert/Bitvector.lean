import SszCert.Basic
import SszCert.Bitlist

/-!
# SSZ bitvectors

An `n`-bit vector packs its bits LSB-first into `⌈n/8⌉` bytes. The canonicity
surface is the padding discipline: the unused high bits of the last byte must
be zero, so `decodeLE bs < 2^n` characterizes exactly the canonical inputs.
All packing arithmetic is inherited from the bitlist layer
(`bitsToNat`/`natToBits` and the `encodeLE`/`decodeLE` primitives).
-/

namespace Ssz
namespace Bitvector

open Bitlist

/-- Number of bytes in an `n`-bit vector: `⌈n/8⌉`. -/
def byteLen (n : Nat) : Nat := (n + 7) / 8

/-- Canonical SSZ bitvector serialization: pack the bits LSB-first. -/
def serializeBitvec (bits : List Bool) : List UInt8 :=
  encodeLE (bitsToNat bits) (byteLen bits.length)

/-- Strict SSZ bitvector decoding for `n` bits: exact byte length, and all
    padding bits zero (`decodeLE bs < 2^n`). -/
def decodeBitvec (n : Nat) (bs : List UInt8) : Option (List Bool) :=
  if bs.length = byteLen n ∧ decodeLE bs < 2 ^ n then
    some (natToBits n (decodeLE bs))
  else
    none

theorem le_8_byteLen (n : Nat) : n ≤ 8 * byteLen n := by
  simp only [byteLen]
  omega

theorem length_serializeBitvec (bits : List Bool) :
    (serializeBitvec bits).length = byteLen bits.length := by
  simp [serializeBitvec]

/-- The packed value fits the byte width. -/
theorem bitsToNat_lt_pow_byteLen (bits : List Bool) :
    bitsToNat bits < 256 ^ byteLen bits.length := by
  have hlt : bitsToNat bits < 2 ^ bits.length := bitsToNat_lt bits
  have hle : (2:Nat) ^ bits.length ≤ 256 ^ byteLen bits.length := by
    rw [← two_pow_8mul]
    exact Nat.pow_le_pow_right (by omega) (le_8_byteLen _)
  omega

/-- Roundtrip (P1): decoding the canonical serialization recovers the bits. -/
theorem decodeBitvec_serializeBitvec (bits : List Bool) :
    decodeBitvec bits.length (serializeBitvec bits) = some bits := by
  have hdec : decodeLE (serializeBitvec bits) = bitsToNat bits :=
    decodeLE_encodeLE _ _ (bitsToNat_lt_pow_byteLen bits)
  have hbits : natToBits bits.length (bitsToNat bits) = bits := by
    have h := natToBits_bitsToNat bits []
    simpa using h
  simp only [decodeBitvec, length_serializeBitvec, hdec]
  rw [if_pos ⟨trivial, bitsToNat_lt bits⟩, hbits]

/-- Decode-canonicity (P2): an accepted byte string is exactly the canonical
    serialization of its decoded bits — nonzero padding bits and wrong-length
    inputs are impossible. -/
theorem serializeBitvec_decodeBitvec {n : Nat} {bs : List UInt8}
    {v : List Bool} (h : decodeBitvec n bs = some v) :
    v.length = n ∧ serializeBitvec v = bs := by
  simp only [decodeBitvec] at h
  by_cases hc : bs.length = byteLen n ∧ decodeLE bs < 2 ^ n
  · rw [if_pos hc] at h
    injection h with hv
    subst hv
    obtain ⟨hblen, hlt⟩ := hc
    refine ⟨by simp, ?_⟩
    have hbtn : bitsToNat (natToBits n (decodeLE bs)) = decodeLE bs := by
      rw [bitsToNat_natToBits, Nat.mod_eq_of_lt hlt]
    simp only [serializeBitvec, length_natToBits, hbtn]
    rw [← hblen]
    exact encodeLE_decodeLE bs
  · rw [if_neg hc] at h
    exact absurd h (by simp)

/-- Injectivity (P3) on `n`-bit vectors. -/
theorem serializeBitvec_injective {b₁ b₂ : List Bool}
    (h₁ : b₁.length = b₂.length)
    (h : serializeBitvec b₁ = serializeBitvec b₂) : b₁ = b₂ := by
  have e := congrArg (decodeBitvec b₁.length) h
  rw [decodeBitvec_serializeBitvec b₁, h₁, decodeBitvec_serializeBitvec b₂] at e
  injection e

/-- Non-malleability: a bitvector has at most one accepted encoding. -/
theorem decodeBitvec_unique_encoding {n : Nat} {b₁ b₂ : List UInt8}
    {v : List Bool} (h₁ : decodeBitvec n b₁ = some v)
    (h₂ : decodeBitvec n b₂ = some v) : b₁ = b₂ := by
  obtain ⟨_, e₁⟩ := serializeBitvec_decodeBitvec h₁
  obtain ⟨_, e₂⟩ := serializeBitvec_decodeBitvec h₂
  rw [← e₁, ← e₂]

end Bitvector
end Ssz
