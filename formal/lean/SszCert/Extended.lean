import SszCert.Codec

/-!
# Extended types: classic unions, optionals and big integers

dynamic-ssz supports encodings beyond the SSZ specification (no `Desc` in
EthLib covers them). Their semantics here are dynamic-ssz's own, transcribed
from the reflection engine (`reflection/unmarshal.go`, `marshal.go`,
`treeroot.go`); the link holds both engines to these codecs.

* **Classic `Union[...]`** (consensus-specs legacy union): selector byte plus the
  variant's payload, a `None` variant at selector 0 with an empty payload. The
  wire format and root are those of `unionCodec` with a `None` variant
  (`noneCodec`) added: root `mix_in_selector(root(value), selector)`, `None`
  rooting as the zero chunk.
* **`optional`** (`*T` with `ssz-type:"optional"`): availability byte `0`
  (absent, nothing follows) or `1` followed by the value. Rooted like
  `List[T, 1]`: `mix_in_length(root(value) | zero, present)`.
* **`big.Int`**: sign byte (`0` non-negative, `1` negative) followed by the
  minimal big-endian magnitude — no leading zero byte, no negative zero — with
  the payload length bounded by `ssz-max`. Rooted as the chunks of
  `uint64_le(len(payload)) ++ payload`, merkleized to the depth they need.
* **Signed integers and floats** need no codec of their own: their wire format
  and root are exactly `uintN`'s over the two's-complement or IEEE-754 bit
  pattern, so they are certified as `uintCodec w`; the value-level bit
  reinterpretation is Go's (`int64(uint64)`, `math.Float64frombits`).
* **`optional` list pointers** (`*T` as `List[T, 1]`) are canonical SSZ and use
  the list codecs with limit 1.

All three codecs carry the same proof fields as the specification codecs:
decode-canonicity with no side conditions, roundtrip, and tree/root agreement.
-/

namespace Ssz

/-! ## None variant -/

/-- The `None` variant of a classic union: the empty payload, decoding only
    from the empty byte string, rooted as the zero chunk. -/
def noneCodec : Codec where
  wt v := match v with
    | .seq [] => True
    | _ => False
  rep _ := True
  ser _ := []
  dec bs := match bs with
    | [] => some (.seq [])
    | _ :: _ => none
  kind := none
  htr _ := Merkle.zeroHash 0
  tree _ := .leaf (Merkle.zeroHash 0)
  htr_tree := by
    intro v _
    rfl
  canon := by
    intro bs v h
    match bs with
    | [] =>
      have h' : some (Val.seq []) = some v := h
      injection h' with hv
      subst hv
      exact ⟨trivial, rfl⟩
    | _ :: _ => exact absurd (show (none : Option Val) = some v from h) (by simp)
  round := by
    intro v hwt _
    cases v
    case seq vs =>
      cases vs
      case nil => rfl
      case cons => exact hwt.elim
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hk
    exact absurd hk (by simp)

/-- Classic `Union[...]`: `unionCodec` with an optional `None` variant at
    selector 0. -/
def classicUnionCodec (hasNone : Bool) (variants : List (Nat × Codec)) : Codec :=
  unionCodec (if hasNone then (0, noneCodec) :: variants else variants)

/-! ## Optional -/

/-- `optional` value: availability byte, then the value when present. An
    absent value is `.seq []`, a present one `.seq [v]`. -/
def optionalCodec (c : Codec) : Codec where
  wt v := match v with
    | .seq [] => True
    | .seq [pv] => c.wt pv
    | _ => False
  rep v := match v with
    | .seq [pv] => c.rep pv
    | _ => True
  ser v := match v with
    | .seq [pv] => 1 :: c.ser pv
    | _ => [0]
  dec bs := match bs with
    | [] => none
    | b :: rest =>
      if b = 0 then (if rest = [] then some (.seq []) else none)
      else if b = 1 then (c.dec rest).map (fun pv => .seq [pv])
      else none
  kind := none
  htr v := match v with
    | .seq [pv] => Merkle.mixIn (c.htr pv) (Merkle.lengthWord 1)
    | _ => Merkle.mixIn (Merkle.zeroHash 0) (Merkle.lengthWord 0)
  tree v := match v with
    | .seq [pv] => .branch (c.tree pv) (.leaf (Merkle.lengthWord 1))
    | _ => .branch (.leaf (Merkle.zeroHash 0)) (.leaf (Merkle.lengthWord 0))
  htr_tree := by
    intro v hwt
    cases v
    case seq vs =>
      match vs, hwt with
      | [], _ => rfl
      | [pv], hwt =>
        show Merkle.hashPair (c.tree pv).root _ = _
        rw [c.htr_tree hwt]
        rfl
      | _ :: _ :: _, hwt => exact hwt.elim
    all_goals exact hwt.elim
  canon := by
    intro bs v h
    match bs with
    | [] => exact absurd (show (none : Option Val) = some v from h) (by simp)
    | b :: rest =>
      have h' : (if b = 0 then (if rest = [] then some (Val.seq []) else none)
          else if b = 1 then (c.dec rest).map (fun pv => Val.seq [pv])
          else none) = some v := h
      by_cases h0 : b = 0
      · rw [if_pos h0] at h'
        by_cases hr : rest = []
        · rw [if_pos hr] at h'
          injection h' with hv
          subst hv
          subst h0
          subst hr
          exact ⟨trivial, rfl⟩
        · rw [if_neg hr] at h'
          exact absurd h' (by simp)
      · rw [if_neg h0] at h'
        by_cases h1 : b = 1
        · rw [if_pos h1] at h'
          cases hd : c.dec rest with
          | none =>
            rw [hd] at h'
            exact absurd h' (by simp)
          | some pv =>
            rw [hd, Option.map_some, Option.some.injEq] at h'
            subst h'
            subst h1
            obtain ⟨hw, hs⟩ := c.canon hd
            exact ⟨hw, by show (1 : UInt8) :: c.ser pv = 1 :: rest; rw [hs]⟩
        · rw [if_neg h1] at h'
          exact absurd h' (by simp)
  round := by
    intro v hwt hrep
    cases v
    case seq vs =>
      match vs, hwt, hrep with
      | [], _, _ => rfl
      | [pv], hwt, hrep =>
        show (if (1 : UInt8) = 0 then (if c.ser pv = [] then some (Val.seq []) else none)
          else if (1 : UInt8) = 1 then (c.dec (c.ser pv)).map (fun pv => Val.seq [pv])
          else none) = some (.seq [pv])
        rw [if_neg (by decide), if_pos rfl, c.round hwt hrep, Option.map_some]
      | _ :: _ :: _, hwt, _ => exact hwt.elim
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hk
    exact absurd hk (by simp)

/-! ## Big integers -/

/-- Canonical `big.Int` payload: a sign byte `0`/`1`, a magnitude without a
    leading zero byte, no negative zero, and at most `lim` bytes in all. -/
def bigIntOk (lim : Nat) : List UInt8 → Bool
  | [] => false
  | s :: mag =>
    (s == 0 || s == 1) && mag.head? != some 0 && !(s == 1 && mag.isEmpty) &&
      decide ((s :: mag).length ≤ lim)

/-- The chunks a `big.Int` payload is rooted over: its length as a uint64,
    then the payload, zero-padded to whole chunks. -/
def bigIntChunks (bs : List UInt8) : List Merkle.Hash :=
  Merkle.packBytes (encodeLE bs.length 8 ++ bs)

/-- `big.Int` (`lim` bounds the payload length; `noLimit` when undeclared). -/
def bigIntCodec (lim : Nat) : Codec where
  wt v := match v with
    | .bytes bs => bigIntOk lim bs = true
    | _ => False
  rep _ := True
  ser v := match v with
    | .bytes bs => bs
    | _ => []
  dec bs := if bigIntOk lim bs then some (.bytes bs) else none
  kind := none
  htr v := match v with
    | .bytes bs =>
      Merkle.merkleRoot (Merkle.depthFor (bigIntChunks bs).length) (bigIntChunks bs)
    | _ => []
  tree v := match v with
    | .bytes bs =>
      Merkle.merkleTree (Merkle.depthFor (bigIntChunks bs).length)
        (Merkle.chunkNodes (bigIntChunks bs))
    | _ => .leaf []
  htr_tree := by
    intro v hwt
    cases v
    case bytes bs => exact Merkle.root_merkleTree_chunks _ _
    all_goals exact hwt.elim
  canon := by
    intro bs v h
    by_cases hok : bigIntOk lim bs = true
    · have h' : (if bigIntOk lim bs = true then some (Val.bytes bs) else none) = some v := h
      rw [if_pos hok] at h'
      injection h' with hv
      subst hv
      exact ⟨hok, rfl⟩
    · have h' : (if bigIntOk lim bs = true then some (Val.bytes bs) else none) = some v := h
      rw [if_neg hok] at h'
      exact absurd h' (by simp)
  round := by
    intro v hwt _
    cases v
    case bytes bs =>
      show (if bigIntOk lim bs = true then some (Val.bytes bs) else none) = some (.bytes bs)
      have hok : bigIntOk lim bs = true := hwt
      rw [if_pos hok]
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hk
    exact absurd hk (by simp)

end Ssz
