import SszCert.Basic

/-!
# Bitlist sentinel canonicity

SSZ bitlists pack bits LSB-first and append a single **sentinel bit** after
the last data bit; the byte string is the minimal one containing all data
bits plus the sentinel. The sentinel is what makes the bit-length
recoverable — and it is a canonicity surface with a real bug history:
missing-sentinel inputs (all-zero last byte) sent Teku into an infinite loop
and crashed other clients (beacon-fuzz 2020), and dynamic-ssz itself lacked
sentinel validation before issue #56.

The formalization identifies a bitlist encoding with the little-endian
natural number it denotes: packing `bits ++ [true]` LSB-first is exactly
`encodeLE (bitsToNat (bits ++ [true]))`, and the sentinel becomes the
**highest set bit** of the decoded number. That reduces sentinel reasoning to
`Nat.log2`, and byte-level canonicity is inherited from the proven
`encodeLE`/`decodeLE` layer:

* sentinel present in the last byte ⇔ `2^(8*(len-1)) ≤ decodeLE bs` — a
  zero last byte (missing sentinel / trailing zero bytes) fails this;
* bit length = `(decodeLE bs).log2`;
* empty input is invalid (the empty bitlist is `[0x01]`, never `[]`).

Main theorems:
* `decodeBits_serializeBits` — roundtrip (P1); *no* representability side
  condition (bitlists have no offsets)
* `serializeBits_decodeBits` — decode-canonicity (P2), unconditional: an
  accepted byte string is exactly the canonical bitlist encoding, and the
  bit length respects the limit
* `serializeBits_injective`, `decodeBits_unique_encoding`
* `trailing_zero_rejected` — any input whose last byte is zero (in
  particular any missing-sentinel padding) is rejected
* `empty_rejected`
-/

namespace Ssz
namespace Bitlist

/-- LSB-first value of a bit string. -/
def bitsToNat : List Bool → Nat
  | [] => 0
  | b :: rest => (if b then 1 else 0) + 2 * bitsToNat rest

/-- The low `k` bits of `m`, LSB-first. -/
def natToBits : Nat → Nat → List Bool
  | 0, _ => []
  | k + 1, m => (m % 2 == 1) :: natToBits k (m / 2)

/-- Canonical SSZ bitlist serialization: pack `bits ++ [sentinel]` LSB-first
    into the minimal number of bytes. -/
def serializeBits (bits : List Bool) : List UInt8 :=
  encodeLE (bitsToNat (bits ++ [true])) (bits.length / 8 + 1)

/-- Strict SSZ bitlist decoding with bit limit `limit`. -/
def decodeBits (limit : Nat) (bs : List UInt8) : Option (List Bool) :=
  if bs = [] then
    none
  else if 2 ^ (8 * (bs.length - 1)) ≤ decodeLE bs then
    if (decodeLE bs).log2 ≤ limit then
      some (natToBits (decodeLE bs).log2 (decodeLE bs))
    else
      none
  else
    none

/-! ## Bit/number lemmas -/

theorem bitsToNat_lt (l : List Bool) : bitsToNat l < 2 ^ l.length := by
  induction l with
  | nil => simp [bitsToNat]
  | cons b l ih =>
    simp only [bitsToNat, List.length_cons, Nat.pow_succ]
    have hb : (if b then 1 else 0) ≤ 1 := by cases b <;> simp
    omega

theorem bitsToNat_append (xs ys : List Bool) :
    bitsToNat (xs ++ ys) = bitsToNat xs + 2 ^ xs.length * bitsToNat ys := by
  induction xs with
  | nil => simp [bitsToNat]
  | cons b xs ih =>
    simp only [List.cons_append, bitsToNat, ih, List.length_cons, Nat.pow_succ]
    have h : 2 ^ xs.length * 2 * bitsToNat ys
        = 2 * (2 ^ xs.length * bitsToNat ys) := by
      rw [Nat.mul_comm (2 ^ xs.length) 2, Nat.mul_assoc]
    rw [h]
    omega

@[simp]
theorem length_natToBits (k m : Nat) : (natToBits k m).length = k := by
  induction k generalizing m with
  | zero => rfl
  | succ k ih => simp [natToBits, ih]

/-- Reading back the bits of a packed prefix recovers the prefix, regardless
    of what follows. -/
theorem natToBits_bitsToNat (xs ys : List Bool) :
    natToBits xs.length (bitsToNat (xs ++ ys)) = xs := by
  induction xs with
  | nil => rfl
  | cons b xs ih =>
    simp only [List.cons_append, bitsToNat, List.length_cons, natToBits]
    cases b with
    | true =>
      have h1 : (1 + 2 * bitsToNat (xs ++ ys)) % 2 = 1 := by omega
      have h2 : (1 + 2 * bitsToNat (xs ++ ys)) / 2 = bitsToNat (xs ++ ys) := by
        omega
      simp [h1, h2, ih]
    | false =>
      have h1 : (0 + 2 * bitsToNat (xs ++ ys)) % 2 = 0 := by omega
      have h2 : (0 + 2 * bitsToNat (xs ++ ys)) / 2 = bitsToNat (xs ++ ys) := by
        omega
      simp [h1, h2, ih]

theorem mod_pow_succ (m k : Nat) :
    m % 2 ^ (k + 1) = m % 2 + 2 * (m / 2 % 2 ^ k) := by
  have hpow : (2:Nat) ^ (k + 1) = 2 * 2 ^ k := by
    rw [Nat.pow_succ, Nat.mul_comm]
  have hpos : 0 < 2 ^ k := Nat.two_pow_pos k
  have hsplit : 2 * (m / 2) + m % 2 = m := Nat.div_add_mod m 2
  have hmod2 : m % 2 < 2 := Nat.mod_lt _ (by omega)
  calc m % 2 ^ (k + 1)
      = (2 * (m / 2) + m % 2) % (2 * 2 ^ k) := by rw [hsplit, hpow]
    _ = ((2 * (m / 2)) % (2 * 2 ^ k) + (m % 2) % (2 * 2 ^ k)) % (2 * 2 ^ k) := by
        rw [← Nat.add_mod]
    _ = (2 * (m / 2 % 2 ^ k) + m % 2) % (2 * 2 ^ k) := by
        rw [Nat.mul_mod_mul_left, Nat.mod_eq_of_lt (by omega : m % 2 < 2 * 2 ^ k)]
    _ = m % 2 + 2 * (m / 2 % 2 ^ k) := by
        have hlt : m / 2 % 2 ^ k < 2 ^ k := Nat.mod_lt _ hpos
        rw [Nat.mod_eq_of_lt (by omega)]
        omega

/-- The value of the low `k` bits is `m` modulo `2^k`. -/
theorem bitsToNat_natToBits (k m : Nat) :
    bitsToNat (natToBits k m) = m % 2 ^ k := by
  induction k generalizing m with
  | zero => simp [natToBits, bitsToNat, Nat.mod_one]
  | succ k ih =>
    simp only [natToBits, bitsToNat, ih]
    have hbit : (if (m % 2 == 1) = true then 1 else 0) = m % 2 := by
      by_cases hp : m % 2 = 1
      · simp [hp]
      · have h0 : m % 2 = 0 := by omega
        simp [h0]
    rw [hbit, mod_pow_succ]

/-- `log2` characterization from a two-sided power sandwich. -/
theorem log2_eq_of {m n : Nat} (h1 : 2 ^ n ≤ m) (h2 : m < 2 ^ (n + 1)) :
    Nat.log2 m = n := by
  have hm0 : m ≠ 0 := by
    have := Nat.two_pow_pos n
    omega
  have hlo : n ≤ m.log2 := (Nat.le_log2 hm0).mpr h1
  have hhi : m.log2 < n + 1 := (Nat.log2_lt hm0).mpr h2
  omega

theorem two_pow_8mul (n : Nat) : (2:Nat) ^ (8 * n) = 256 ^ n := by
  rw [Nat.pow_mul, show ((2:Nat) ^ 8) = 256 from by decide]

theorem decodeLE_append (xs ys : List UInt8) :
    decodeLE (xs ++ ys) = decodeLE xs + 256 ^ xs.length * decodeLE ys := by
  induction xs with
  | nil => simp [decodeLE]
  | cons b xs ih =>
    simp only [List.cons_append, decodeLE, ih, List.length_cons, Nat.pow_succ]
    have h : 256 ^ xs.length * 256 * decodeLE ys
        = 256 * (256 ^ xs.length * decodeLE ys) := by
      rw [Nat.mul_comm (256 ^ xs.length) 256, Nat.mul_assoc]
    rw [h]
    omega

/-! ## Roundtrip -/

/-- **Roundtrip (P1) for bitlists**: decoding the canonical serialization
    recovers the bits. No representability hypothesis — bitlists carry no
    offsets. -/
theorem decodeBits_serializeBits (bits : List Bool) (limit : Nat)
    (hlim : bits.length ≤ limit) :
    decodeBits limit (serializeBits bits) = some bits := by
  have hv_lo : 2 ^ bits.length ≤ bitsToNat (bits ++ [true]) := by
    rw [bitsToNat_append]
    have h1 : bitsToNat [true] = 1 := rfl
    rw [h1]
    omega
  have hv_hi : bitsToNat (bits ++ [true]) < 2 ^ (bits.length + 1) := by
    have := bitsToNat_lt (bits ++ [true])
    simpa using this
  have hbound : bitsToNat (bits ++ [true]) < 256 ^ (bits.length / 8 + 1) := by
    have h1 : bits.length + 1 ≤ 8 * (bits.length / 8 + 1) := by omega
    have h2 : (2:Nat) ^ (bits.length + 1) ≤ 2 ^ (8 * (bits.length / 8 + 1)) :=
      Nat.pow_le_pow_right (by omega) h1
    rw [two_pow_8mul] at h2
    omega
  have hlenc : (serializeBits bits).length = bits.length / 8 + 1 :=
    length_encodeLE ..
  have hne : serializeBits bits ≠ [] := by
    intro he
    rw [he] at hlenc
    simp at hlenc
  have hm : decodeLE (serializeBits bits) = bitsToNat (bits ++ [true]) :=
    decodeLE_encodeLE _ _ hbound
  have hsent : 2 ^ (8 * ((serializeBits bits).length - 1))
      ≤ decodeLE (serializeBits bits) := by
    rw [hlenc, hm]
    have h1 : 8 * (bits.length / 8 + 1 - 1) ≤ bits.length := by omega
    have h2 : (2:Nat) ^ (8 * (bits.length / 8 + 1 - 1)) ≤ 2 ^ bits.length :=
      Nat.pow_le_pow_right (by omega) h1
    omega
  have hlog : (decodeLE (serializeBits bits)).log2 = bits.length := by
    rw [hm]
    exact log2_eq_of hv_lo hv_hi
  unfold decodeBits
  rw [if_neg hne, if_pos hsent,
    if_pos (by rw [hlog]; exact hlim), hlog, hm,
    natToBits_bitsToNat bits [true]]

/-! ## Canonicity -/

/-- **Decode-canonicity (P2) for bitlists — the sentinel discipline,
    unconditional**: any byte string the strict decoder accepts is exactly
    the canonical serialization of the decoded bits, whose length respects
    the limit. Missing sentinels, zero-padded last bytes, trailing zero
    bytes, and oversized buffers are all impossible. -/
theorem serializeBits_decodeBits {limit : Nat} {bs : List UInt8}
    {bits : List Bool} (h : decodeBits limit bs = some bits) :
    serializeBits bits = bs ∧ bits.length ≤ limit := by
  unfold decodeBits at h
  by_cases hne : bs = []
  · rw [if_pos hne] at h
    exact absurd h (by simp)
  · rw [if_neg hne] at h
    by_cases hsent : 2 ^ (8 * (bs.length - 1)) ≤ decodeLE bs
    · rw [if_pos hsent] at h
      by_cases hlim2 : (decodeLE bs).log2 ≤ limit
      · rw [if_pos hlim2] at h
        simp only [Option.some.injEq] at h
        subst h
        have hm0 : decodeLE bs ≠ 0 := by
          have := Nat.two_pow_pos (8 * (bs.length - 1))
          omega
        have hlo : 2 ^ (decodeLE bs).log2 ≤ decodeLE bs :=
          Nat.log2_self_le hm0
        have hhi : decodeLE bs < 2 ^ ((decodeLE bs).log2 + 1) :=
          Nat.lt_log2_self
        have hlen_pos : 1 ≤ bs.length := by
          cases bs with
          | nil => exact absurd rfl hne
          | cons _ _ => simp
        have hup : decodeLE bs < 2 ^ (8 * bs.length) := by
          have h1 := decodeLE_lt bs
          rw [← two_pow_8mul] at h1
          exact h1
        have hn_lo : 8 * (bs.length - 1) ≤ (decodeLE bs).log2 := by
          have h1 : (2:Nat) ^ (8 * (bs.length - 1))
              < 2 ^ ((decodeLE bs).log2 + 1) := by omega
          have := (Nat.pow_lt_pow_iff_right (by omega : 1 < 2)).mp h1
          omega
        have hn_hi : (decodeLE bs).log2 < 8 * bs.length := by
          have h1 : (2:Nat) ^ (decodeLE bs).log2 < 2 ^ (8 * bs.length) := by
            omega
          exact (Nat.pow_lt_pow_iff_right (by omega : 1 < 2)).mp h1
        refine ⟨?_, by rw [length_natToBits]; exact hlim2⟩
        have hval : bitsToNat (natToBits (decodeLE bs).log2 (decodeLE bs)
            ++ [true]) = decodeLE bs := by
          rw [bitsToNat_append, bitsToNat_natToBits, length_natToBits,
            show bitsToNat [true] = 1 from rfl]
          have hmod : decodeLE bs % 2 ^ (decodeLE bs).log2
              = decodeLE bs - 2 ^ (decodeLE bs).log2 := by
            have h2 : decodeLE bs < 2 * 2 ^ (decodeLE bs).log2 := by
              have h3 : (2:Nat) ^ ((decodeLE bs).log2 + 1)
                  = 2 * 2 ^ (decodeLE bs).log2 := by
                rw [Nat.pow_succ, Nat.mul_comm]
              omega
            rw [Nat.mod_eq_sub_mod hlo, Nat.mod_eq_of_lt (by omega)]
          omega
        have hL : (natToBits (decodeLE bs).log2 (decodeLE bs)).length / 8 + 1
            = bs.length := by
          rw [length_natToBits]
          omega
        rw [serializeBits, hval, hL]
        exact encodeLE_decodeLE bs
      · rw [if_neg hlim2] at h
        exact absurd h (by simp)
    · rw [if_neg hsent] at h
      exact absurd h (by simp)

/-! ## Corollaries -/

/-- Injectivity (P3) for bitlists — needs only the limit hypotheses. -/
theorem serializeBits_injective {b₁ b₂ : List Bool} (limit : Nat)
    (h₁ : b₁.length ≤ limit) (h₂ : b₂.length ≤ limit)
    (h : serializeBits b₁ = serializeBits b₂) : b₁ = b₂ := by
  have e := congrArg (decodeBits limit) h
  rw [decodeBits_serializeBits b₁ limit h₁,
    decodeBits_serializeBits b₂ limit h₂] at e
  injection e

/-- Non-malleability: a bitlist has at most one accepted encoding. -/
theorem decodeBits_unique_encoding {limit : Nat} {b₁ b₂ : List UInt8}
    {bits : List Bool} (h₁ : decodeBits limit b₁ = some bits)
    (h₂ : decodeBits limit b₂ = some bits) : b₁ = b₂ := by
  obtain ⟨e₁, _⟩ := serializeBits_decodeBits h₁
  obtain ⟨e₂, _⟩ := serializeBits_decodeBits h₂
  rw [← e₁, ← e₂]

/-- The missing-sentinel/beacon-fuzz property: any input ending in a zero
    byte — in particular every sentinel-free zero padding — is rejected. -/
theorem trailing_zero_rejected (limit : Nat) (bs : List UInt8) :
    decodeBits limit (bs ++ [0]) = none := by
  have hm : decodeLE (bs ++ [0]) = decodeLE bs := by
    rw [decodeLE_append]
    simp [decodeLE]
  have hlt : decodeLE (bs ++ [0]) < 2 ^ (8 * ((bs ++ [0]).length - 1)) := by
    rw [hm]
    have h1 := decodeLE_lt bs
    rw [← two_pow_8mul] at h1
    have h2 : (bs ++ [0]).length - 1 = bs.length := by simp
    rw [h2]
    exact h1
  unfold decodeBits
  rw [if_neg (by simp : ¬(bs ++ [0] = [])), if_neg (by omega)]

/-- Empty input is invalid: the empty bitlist is `[0x01]`, never `[]`. -/
theorem empty_rejected (limit : Nat) : decodeBits limit [] = none := by
  simp [decodeBits]

end Bitlist
end Ssz
