/-!
# SSZ core primitives: little-endian fixed-width encoding

Executable model of SSZ's `uintN` serialization with the three fundamental
theorems:

* `decodeLE_encodeLE` — roundtrip: decoding an encoding recovers the value
* `encodeLE_decodeLE` — canonicity: every byte string of width `len` is the
  encoding of exactly the value it decodes to (so the decoder accepts no
  non-canonical representations — for fixed-width integers every input is
  canonical)
* `decodeLE_lt` — decoded values respect the width bound
-/

namespace Ssz

/-- Little-endian encoding of `n` into `len` bytes (SSZ `uintN`, `N = 8*len`). -/
def encodeLE : Nat → Nat → List UInt8
  | _, 0 => []
  | n, len + 1 => UInt8.ofNat (n % 256) :: encodeLE (n / 256) len

/-- Little-endian decoding of a byte string to a natural number. -/
def decodeLE : List UInt8 → Nat
  | [] => 0
  | b :: bs => b.toNat + 256 * decodeLE bs

@[simp]
theorem length_encodeLE (n len : Nat) : (encodeLE n len).length = len := by
  induction len generalizing n with
  | zero => rfl
  | succ len ih => simp [encodeLE, ih]

theorem decodeLE_lt (bs : List UInt8) : decodeLE bs < 256 ^ bs.length := by
  induction bs with
  | nil => simp [decodeLE]
  | cons b bs ih =>
    have hb : b.toNat < 256 := b.toNat_lt
    simp only [decodeLE, List.length_cons, Nat.pow_succ]
    have h2 : 256 * (decodeLE bs + 1) ≤ 256 * 256 ^ bs.length :=
      Nat.mul_le_mul_left 256 ih
    omega

/-- Roundtrip: decoding the `len`-byte encoding of `n < 256^len` yields `n`. -/
theorem decodeLE_encodeLE (n len : Nat) (h : n < 256 ^ len) :
    decodeLE (encodeLE n len) = n := by
  induction len generalizing n with
  | zero =>
    simp [Nat.pow_zero] at h
    simp [encodeLE, decodeLE, h]
  | succ len ih =>
    have hdiv : n / 256 < 256 ^ len := by
      rw [Nat.pow_succ] at h
      exact Nat.div_lt_of_lt_mul (by omega)
    simp only [encodeLE, decodeLE]
    have hb : (UInt8.ofNat (n % 256)).toNat = n % 256 := by simp
    rw [hb, ih _ hdiv]
    omega

/-- Canonicity: every byte string is the encoding of its decoded value.
    Fixed-width integer decoding admits no non-canonical inputs. -/
theorem encodeLE_decodeLE (bs : List UInt8) :
    encodeLE (decodeLE bs) bs.length = bs := by
  induction bs with
  | nil => rfl
  | cons b bs ih =>
    have hb : b.toNat < 256 := b.toNat_lt
    simp only [decodeLE, List.length_cons, encodeLE, List.cons.injEq]
    refine ⟨?_, ?_⟩
    · have hmod : (b.toNat + 256 * decodeLE bs) % 256 = b.toNat := by
        omega
      rw [hmod, UInt8.ofNat_toNat]
    · have hdiv : (b.toNat + 256 * decodeLE bs) / 256 = decodeLE bs := by
        omega
      rw [hdiv]
      exact ih

/-- Injectivity of the encoder on the width-bounded domain. -/
theorem encodeLE_injective (len : Nat) {m n : Nat}
    (hm : m < 256 ^ len) (hn : n < 256 ^ len)
    (h : encodeLE m len = encodeLE n len) : m = n := by
  have := congrArg decodeLE h
  rwa [decodeLE_encodeLE m len hm, decodeLE_encodeLE n len hn] at this

end Ssz
