/-!
# SHA-256

Executable SHA-256 (FIPS 180-4) over byte lists, used as the leaf hash of the
merkleization model. The theorems in this development never unfold it — no
cryptographic property is assumed or proved; it is an opaque function the
model and the implementation must share. Its executable correctness is
validated against the FIPS test vectors (see `Main.lean`) and continuously by
the differential link (any defect would make every root comparison fail).
-/

namespace Ssz
namespace SHA256

private def rotr (x : UInt32) (n : UInt32) : UInt32 :=
  (x >>> n) ||| (x <<< (32 - n))

private def K : Array UInt32 := #[
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5,
  0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
  0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc,
  0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
  0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
  0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3,
  0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5,
  0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
  0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2]

/-- Pad a message to a whole number of 64-byte blocks (FIPS 180-4 §5.1.1). -/
private def pad (msg : List UInt8) : List UInt8 :=
  let len := msg.length
  let bitLen := 8 * len
  let zeroCount := (55 + 64 - len % 64) % 64
  let lenBytes := (List.range 8).map fun i =>
    UInt8.ofNat (bitLen / 256 ^ (7 - i) % 256)
  msg ++ [0x80] ++ List.replicate zeroCount 0 ++ lenBytes

/-- Big-endian 32-bit words of a 64-byte block. -/
private def blockWords (block : Array UInt8) : Array UInt32 := Id.run do
  let mut w : Array UInt32 := Array.mkEmpty 64
  for i in [0:16] do
    let b0 := (block[4 * i]!).toUInt32
    let b1 := (block[4 * i + 1]!).toUInt32
    let b2 := (block[4 * i + 2]!).toUInt32
    let b3 := (block[4 * i + 3]!).toUInt32
    w := w.push ((b0 <<< 24) ||| (b1 <<< 16) ||| (b2 <<< 8) ||| b3)
  for i in [16:64] do
    let s0 := rotr w[i - 15]! 7 ^^^ rotr w[i - 15]! 18 ^^^ (w[i - 15]! >>> 3)
    let s1 := rotr w[i - 2]! 17 ^^^ rotr w[i - 2]! 19 ^^^ (w[i - 2]! >>> 10)
    w := w.push (w[i - 16]! + s0 + w[i - 7]! + s1)
  return w

private def compress (st : Array UInt32) (block : Array UInt8) :
    Array UInt32 := Id.run do
  let w := blockWords block
  let mut a := st[0]!
  let mut b := st[1]!
  let mut c := st[2]!
  let mut d := st[3]!
  let mut e := st[4]!
  let mut f := st[5]!
  let mut g := st[6]!
  let mut h := st[7]!
  for i in [0:64] do
    let s1 := rotr e 6 ^^^ rotr e 11 ^^^ rotr e 25
    let ch := (e &&& f) ^^^ ((~~~e) &&& g)
    let t1 := h + s1 + ch + K[i]! + w[i]!
    let s0 := rotr a 2 ^^^ rotr a 13 ^^^ rotr a 22
    let maj := (a &&& b) ^^^ (a &&& c) ^^^ (b &&& c)
    let t2 := s0 + maj
    h := g
    g := f
    f := e
    e := d + t1
    d := c
    c := b
    b := a
    a := t1 + t2
  return #[st[0]! + a, st[1]! + b, st[2]! + c, st[3]! + d,
           st[4]! + e, st[5]! + f, st[6]! + g, st[7]! + h]

private def initState : Array UInt32 := #[
  0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
  0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19]

/-- SHA-256 digest of a byte list, as 32 bytes. -/
def sha256 (msg : List UInt8) : List UInt8 := Id.run do
  let padded := (pad msg).toArray
  let mut st := initState
  let mut i := 0
  while i + 64 ≤ padded.size do
    st := compress st (padded.extract i (i + 64))
    i := i + 64
  let mut out : List UInt8 := []
  for j in [0:8] do
    let x := st[7 - j]!
    out := (x >>> 24).toUInt8 :: (x >>> 16).toUInt8
      :: (x >>> 8).toUInt8 :: x.toUInt8 :: out
  return out

end SHA256
end Ssz
