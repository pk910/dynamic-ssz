import SszCert.Basic
import SszCert.Schema
import SszCert.VarList
import SszCert.Bitlist
import SszCert.Boolean
import SszCert.Bitvector
import SszCert.FixedSeq
import SszCert.Merkle

/-!
# The codec composition layer

Everything below composes the per-shape theorems (`Basic`, `Schema`,
`VarList`, `Bitlist`, `Boolean`, `Bitvector`, `FixedSeq`) into *nested* SSZ
types. A `Codec` packages an executable serializer/decoder pair for a
universal value type together with machine-checked proofs of:

* `canon` — decode-canonicity (P2): every accepted byte string is exactly
  the canonical serialization of its decoded, well-typed value;
* `round` — roundtrip (P1): every well-typed, offset-representable value
  decodes back from its serialization;
* `ser_len` — fixed-size codecs serialize to exactly their declared width.

Combinators build codecs for every SSZ shape from leaf codecs; the proof
obligations are discharged *compositionally*, so any type expression built
from the combinators carries canonicity and roundtrip theorems by
construction. This is the "nested composition" layer: a per-type certificate
is just a combinator expression, and type-checking it re-verifies the
theorems for that concrete type.
-/

namespace Ssz

/-- Universal SSZ value: the decoding target for every certified type. -/
inductive Val where
  | uint (n : Nat)
  | boolean (b : Bool)
  | bytes (bs : List UInt8)
  | bits (l : List Bool)
  | seq (vs : List Val)
  | union (sel : Nat) (v : Val)

/-- A certified codec: executable `ser`/`dec` plus the three proof fields.

`wt` is the typing invariant (shape, widths, limits); `rep` is the
offset-representability side condition needed only for `round` (real SSZ
offsets must fit 4 bytes); `kind = some n` marks a fixed-size type of `n`
bytes, `none` a variable-size type. -/
structure Codec where
  wt : Val → Prop
  rep : Val → Prop
  ser : Val → List UInt8
  dec : List UInt8 → Option Val
  kind : Option Nat
  htr : Val → Merkle.Hash
  tree : Val → Merkle.Node
  canon : ∀ {bs v}, dec bs = some v → wt v ∧ ser v = bs
  round : ∀ {v}, wt v → rep v → dec (ser v) = some v
  ser_len : ∀ {v n}, wt v → kind = some n → (ser v).length = n
  htr_tree : ∀ {v}, wt v → (tree v).root = htr v

/-! ## Leaf codecs -/

/-- `uintN` (`N = 8*w` bits, `w` bytes, little-endian). -/
def uintCodec (w : Nat) : Codec where
  wt v := match v with
    | .uint n => n < 256 ^ w
    | _ => False
  rep _ := True
  ser v := match v with
    | .uint n => encodeLE n w
    | _ => []
  dec bs := if bs.length = w then some (.uint (decodeLE bs)) else none
  kind := some w
  htr v := match v with
    | .uint n => Merkle.padChunk (encodeLE n w)
    | _ => []
  tree v := match v with
    | .uint n => .leaf (Merkle.padChunk (encodeLE n w))
    | _ => .leaf []
  htr_tree := by
    intro v _
    cases v <;> rfl
  canon := by
    intro bs v h
    by_cases hl : bs.length = w
    · rw [if_pos hl] at h
      injection h with hv
      subst hv
      refine ⟨?_, ?_⟩
      · show decodeLE bs < 256 ^ w
        rw [← hl]
        exact decodeLE_lt bs
      · show encodeLE (decodeLE bs) w = bs
        rw [← hl]
        exact encodeLE_decodeLE bs
    · rw [if_neg hl] at h
      exact absurd h (by simp)
  round := by
    intro v hwt _
    cases v
    case uint n =>
      show (if (encodeLE n w).length = w then
        some (Val.uint (decodeLE (encodeLE n w))) else none) = some (.uint n)
      rw [if_pos (length_encodeLE n w), decodeLE_encodeLE n w hwt]
    all_goals exact hwt.elim
  ser_len := by
    intro v n hwt hk
    injection hk with hk
    cases v
    case uint m =>
      show (encodeLE m w).length = n
      rw [length_encodeLE, hk]
    all_goals exact hwt.elim

/-- SSZ boolean: one byte, strictly `0`/`1`. -/
def boolCodec : Codec where
  wt v := match v with
    | .boolean _ => True
    | _ => False
  rep _ := True
  ser v := match v with
    | .boolean b => Boolean.serializeBool b
    | _ => []
  dec bs := (Boolean.decodeBool bs).map .boolean
  kind := some 1
  htr v := match v with
    | .boolean b => Merkle.padChunk (Boolean.serializeBool b)
    | _ => []
  tree v := match v with
    | .boolean b => .leaf (Merkle.padChunk (Boolean.serializeBool b))
    | _ => .leaf []
  htr_tree := by
    intro v _
    cases v <;> rfl
  canon := by
    intro bs v h
    cases hb : Boolean.decodeBool bs with
    | none => rw [hb] at h; exact absurd h (by simp)
    | some b =>
      rw [hb] at h
      injection h with hv
      subst hv
      exact ⟨trivial, Boolean.serializeBool_decodeBool hb⟩
  round := by
    intro v hwt _
    cases v
    case boolean b =>
      show (Boolean.decodeBool (Boolean.serializeBool b)).map Val.boolean
        = some (.boolean b)
      rw [Boolean.decodeBool_serializeBool]
      rfl
    all_goals exact hwt.elim
  ser_len := by
    intro v n hwt hk
    injection hk with hk
    cases v
    case boolean b =>
      show (Boolean.serializeBool b).length = n
      rw [← hk]
      cases b <;> rfl
    all_goals exact hwt.elim

/-- `ByteVector[n]` (`Bytes<n>`): raw bytes, exact length. -/
def byteVectorCodec (n : Nat) : Codec where
  wt v := match v with
    | .bytes bs => bs.length = n
    | _ => False
  rep _ := True
  ser v := match v with
    | .bytes bs => bs
    | _ => []
  dec bs := if bs.length = n then some (.bytes bs) else none
  kind := some n
  htr v := match v with
    | .bytes bs => Merkle.merkleRoot (Merkle.depthFor (Merkle.byteChunks n))
        (Merkle.packBytes bs)
    | _ => []
  tree v := match v with
    | .bytes bs => Merkle.merkleTree (Merkle.depthFor (Merkle.byteChunks n))
        (Merkle.chunkNodes (Merkle.packBytes bs))
    | _ => .leaf []
  htr_tree := by
    intro v _
    cases v
    case bytes bs => exact Merkle.root_merkleTree_chunks _ _
    all_goals rfl
  canon := by
    intro bs v h
    by_cases hl : bs.length = n
    · rw [if_pos hl] at h
      injection h with hv
      subst hv
      exact ⟨hl, rfl⟩
    · rw [if_neg hl] at h
      exact absurd h (by simp)
  round := by
    intro v hwt _
    cases v
    case bytes bs =>
      show (if bs.length = n then some (Val.bytes bs) else none) = some (.bytes bs)
      rw [if_pos hwt]
    all_goals exact hwt.elim
  ser_len := by
    intro v m hwt hk
    injection hk with hk
    cases v
    case bytes bs =>
      show bs.length = m
      rw [hwt, hk]
    all_goals exact hwt.elim

/-- `ByteList[limit]`: raw bytes, length bounded by `limit`. -/
def byteListCodec (limit : Nat) : Codec where
  wt v := match v with
    | .bytes bs => bs.length ≤ limit
    | _ => False
  rep _ := True
  ser v := match v with
    | .bytes bs => bs
    | _ => []
  dec bs := if bs.length ≤ limit then some (.bytes bs) else none
  kind := none
  htr v := match v with
    | .bytes bs => Merkle.mixIn
        (Merkle.merkleRoot (Merkle.depthFor (Merkle.byteChunks limit))
          (Merkle.packBytes bs))
        (Merkle.lengthWord bs.length)
    | _ => []
  tree v := match v with
    | .bytes bs => .branch
        (Merkle.merkleTree (Merkle.depthFor (Merkle.byteChunks limit))
          (Merkle.chunkNodes (Merkle.packBytes bs)))
        (.leaf (Merkle.lengthWord bs.length))
    | _ => .leaf []
  htr_tree := by
    intro v _
    cases v
    case bytes bs =>
      show Merkle.hashPair _ _ = _
      rw [Merkle.root_merkleTree_chunks]
      rfl
    all_goals rfl
  canon := by
    intro bs v h
    by_cases hl : bs.length ≤ limit
    · rw [if_pos hl] at h
      injection h with hv
      subst hv
      exact ⟨hl, rfl⟩
    · rw [if_neg hl] at h
      exact absurd h (by simp)
  round := by
    intro v hwt _
    cases v
    case bytes bs =>
      show (if bs.length ≤ limit then some (Val.bytes bs) else none)
        = some (.bytes bs)
      rw [if_pos hwt]
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hk
    exact absurd hk (by simp)

/-- `Bitvector[n]`: `n` bits packed LSB-first, zero padding enforced. -/
def bitvectorCodec (n : Nat) : Codec where
  wt v := match v with
    | .bits l => l.length = n
    | _ => False
  rep _ := True
  ser v := match v with
    | .bits l => Bitvector.serializeBitvec l
    | _ => []
  dec bs := (Bitvector.decodeBitvec n bs).map .bits
  kind := some (Bitvector.byteLen n)
  htr v := match v with
    | .bits l => Merkle.merkleRoot (Merkle.depthFor (Merkle.bitChunks n))
        (Merkle.packBits l)
    | _ => []
  tree v := match v with
    | .bits l => Merkle.merkleTree (Merkle.depthFor (Merkle.bitChunks n))
        (Merkle.chunkNodes (Merkle.packBits l))
    | _ => .leaf []
  htr_tree := by
    intro v _
    cases v
    case bits l => exact Merkle.root_merkleTree_chunks _ _
    all_goals rfl
  canon := by
    intro bs v h
    cases hb : Bitvector.decodeBitvec n bs with
    | none => rw [hb] at h; exact absurd h (by simp)
    | some l =>
      rw [hb] at h
      injection h with hv
      subst hv
      obtain ⟨hlen, hser⟩ := Bitvector.serializeBitvec_decodeBitvec hb
      exact ⟨hlen, hser⟩
  round := by
    intro v hwt _
    cases v
    case bits l =>
      show (Bitvector.decodeBitvec n (Bitvector.serializeBitvec l)).map Val.bits
        = some (.bits l)
      have hlen : l.length = n := hwt
      rw [← hlen, Bitvector.decodeBitvec_serializeBitvec]
      rfl
    all_goals exact hwt.elim
  ser_len := by
    intro v m hwt hk
    injection hk with hk
    cases v
    case bits l =>
      show (Bitvector.serializeBitvec l).length = m
      rw [Bitvector.length_serializeBitvec, hwt, hk]
    all_goals exact hwt.elim

/-- `Bitlist[limit]`: delimited bit list, sentinel discipline enforced. -/
def bitlistCodec (limit : Nat) : Codec where
  wt v := match v with
    | .bits l => l.length ≤ limit
    | _ => False
  rep _ := True
  ser v := match v with
    | .bits l => Bitlist.serializeBits l
    | _ => []
  dec bs := (Bitlist.decodeBits limit bs).map .bits
  kind := none
  htr v := match v with
    | .bits l => Merkle.mixIn
        (Merkle.merkleRoot (Merkle.depthFor (Merkle.bitChunks limit))
          (Merkle.packBits l))
        (Merkle.lengthWord l.length)
    | _ => []
  tree v := match v with
    | .bits l => .branch
        (Merkle.merkleTree (Merkle.depthFor (Merkle.bitChunks limit))
          (Merkle.chunkNodes (Merkle.packBits l)))
        (.leaf (Merkle.lengthWord l.length))
    | _ => .leaf []
  htr_tree := by
    intro v _
    cases v
    case bits l =>
      show Merkle.hashPair _ _ = _
      rw [Merkle.root_merkleTree_chunks]
      rfl
    all_goals rfl
  canon := by
    intro bs v h
    cases hb : Bitlist.decodeBits limit bs with
    | none => rw [hb] at h; exact absurd h (by simp)
    | some l =>
      rw [hb] at h
      injection h with hv
      subst hv
      obtain ⟨hser, hlim⟩ := Bitlist.serializeBits_decodeBits hb
      exact ⟨hlim, hser⟩
  round := by
    intro v hwt _
    cases v
    case bits l =>
      show (Bitlist.decodeBits limit (Bitlist.serializeBits l)).map Val.bits
        = some (.bits l)
      rw [Bitlist.decodeBits_serializeBits l limit hwt]
      rfl
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hk
    exact absurd hk (by simp)


/-! ## Container fields (heterogeneous) -/

/-- Field-wise typing for a container's field codecs. -/
def WTFields : List Codec → List Val → Prop
  | [], [] => True
  | c :: cs, v :: vs => c.wt v ∧ WTFields cs vs
  | _, _ => False

/-- Field-wise representability. -/
def RepFields : List Codec → List Val → Prop
  | [], [] => True
  | c :: cs, v :: vs => c.rep v ∧ RepFields cs vs
  | _, _ => False

/-- Serialize each field with its codec. -/
def serFields : List Codec → List Val → List (List UInt8)
  | c :: cs, v :: vs => c.ser v :: serFields cs vs
  | _, _ => []

/-- Decode each field byte string with its codec. -/
def decFields : List Codec → List (List UInt8) → Option (List Val)
  | [], [] => some []
  | c :: cs, b :: bs =>
    match c.dec b, decFields cs bs with
    | some v, some vs => some (v :: vs)
    | _, _ => none
  | _, _ => none

/-- The container schema induced by the field codecs' kinds. -/
def schemaOfCodecs (cs : List Codec) : Sch.Schema :=
  cs.map fun c => match c.kind with
    | some n => .fx n
    | none => .var

/-- Fixed-size containers (all fields fixed) have kind `fixedLen`. -/
def kindOfCodecs (cs : List Codec) : Option Nat :=
  if cs.all fun c => c.kind.isSome then
    some (Sch.fixedLen (schemaOfCodecs cs))
  else
    none

theorem decFields_canon {cs : List Codec} {fields : List (List UInt8)}
    {vs : List Val} (h : decFields cs fields = some vs) :
    WTFields cs vs ∧ serFields cs vs = fields := by
  induction cs generalizing fields vs with
  | nil =>
    cases fields with
    | nil =>
      have h' : some ([] : List Val) = some vs := h
      injection h' with h'
      subst h'
      exact ⟨trivial, rfl⟩
    | cons b fs => exact absurd (show (none : Option (List Val)) = some vs from h) (by simp)
  | cons c cs ih =>
    cases fields with
    | nil => exact absurd (show (none : Option (List Val)) = some vs from h) (by simp)
    | cons b fs =>
      simp only [decFields] at h
      cases hd : c.dec b with
      | none =>
        simp only [hd] at h
        exact absurd h (by simp)
      | some v' =>
        cases hds : decFields cs fs with
        | none =>
          simp only [hd, hds] at h
          exact absurd h (by simp)
        | some vs' =>
          simp only [hd, hds, Option.some.injEq] at h
          subst h
          obtain ⟨hwt, hser⟩ := c.canon hd
          obtain ⟨hwts, hsers⟩ := ih hds
          exact ⟨⟨hwt, hwts⟩, by simp only [serFields, hser, hsers]⟩

theorem decFields_serFields {cs : List Codec} {vs : List Val}
    (hwt : WTFields cs vs) (hrep : RepFields cs vs) :
    decFields cs (serFields cs vs) = some vs := by
  induction cs generalizing vs with
  | nil =>
    cases vs with
    | nil => rfl
    | cons v vs => exact hwt.elim
  | cons c cs ih =>
    cases vs with
    | nil => exact hwt.elim
    | cons v vs =>
      obtain ⟨hw, hws⟩ := hwt
      obtain ⟨hr, hrs⟩ := hrep
      show decFields (c :: cs) (c.ser v :: serFields cs vs) = some (v :: vs)
      simp only [decFields, c.round hw hr, ih hws hrs]

theorem wellTyped_serFields {cs : List Codec} {vs : List Val}
    (hwt : WTFields cs vs) :
    Sch.WellTyped (schemaOfCodecs cs) (serFields cs vs) := by
  induction cs generalizing vs with
  | nil =>
    cases vs with
    | nil => exact Sch.WellTyped.nil
    | cons v vs => exact hwt.elim
  | cons c cs ih =>
    cases vs with
    | nil => exact hwt.elim
    | cons v vs =>
      obtain ⟨hw, hws⟩ := hwt
      show Sch.WellTyped
        ((match c.kind with | some n => .fx n | none => .var) :: schemaOfCodecs cs)
        (c.ser v :: serFields cs vs)
      cases hk : c.kind with
      | some n =>
        simp only [hk]
        exact Sch.WellTyped.fx (c.ser_len hw hk) (ih hws)
      | none =>
        simp only [hk]
        exact Sch.WellTyped.var (ih hws)

theorem schema_no_var_of_all_fixed {cs : List Codec}
    (h : (cs.all fun c => c.kind.isSome) = true) :
    ∀ f ∈ schemaOfCodecs cs, f ≠ Sch.FieldTy.var := by
  induction cs with
  | nil =>
    intro f hf
    simp [schemaOfCodecs] at hf
  | cons c cs ih =>
    simp only [List.all_cons, Bool.and_eq_true] at h
    intro f hf
    simp only [schemaOfCodecs, List.map_cons, List.mem_cons] at hf
    cases hf with
    | inl h1 =>
      subst h1
      cases hk : c.kind with
      | some n => simp
      | none =>
        have := h.1
        rw [hk] at this
        simp at this
    | inr h2 => exact ih h.2 f h2

theorem varVals_no_var {s : Sch.Schema}
    (hs : ∀ f ∈ s, f ≠ Sch.FieldTy.var) :
    ∀ vs, Sch.varVals s vs = [] := by
  induction s with
  | nil =>
    intro vs
    cases vs <;> rfl
  | cons f s ih =>
    intro vs
    cases vs with
    | nil => cases f <;> rfl
    | cons v vs =>
      cases f with
      | fx n =>
        simp only [Sch.varVals]
        exact ih (fun g hg => hs g (by simp [hg])) vs
      | var => exact absurd rfl (hs .var (by simp))

/-- Field roots, per field codec. -/
def htrFields : List Codec → List Val → List Merkle.Hash
  | c :: cs, v :: vs => c.htr v :: htrFields cs vs
  | _, _ => []

/-- Field subtrees, per field codec. -/
def treeFields : List Codec → List Val → List Merkle.Node
  | c :: cs, v :: vs => c.tree v :: treeFields cs vs
  | _, _ => []

theorem map_root_treeFields {cs : List Codec} {vs : List Val}
    (hwt : WTFields cs vs) :
    (treeFields cs vs).map Merkle.Node.root = htrFields cs vs := by
  induction cs generalizing vs with
  | nil => cases vs <;> rfl
  | cons c cs ih =>
    cases vs with
    | nil => rfl
    | cons v vs =>
      obtain ⟨hw, hws⟩ := hwt
      simp only [treeFields, htrFields, List.map_cons, c.htr_tree hw, ih hws]

/-- SSZ container of heterogeneous fields: fixed fields inline, variable
    fields behind a 4-byte offset table (all offset discipline inherited from
    `Schema.lean`). -/
def containerCodec (cs : List Codec) : Codec where
  wt v := match v with
    | .seq vs => WTFields cs vs
    | _ => False
  rep v := match v with
    | .seq vs => RepFields cs vs ∧
        Sch.fixedLen (schemaOfCodecs cs)
          + Sch.heapLen (schemaOfCodecs cs) (serFields cs vs) < 256 ^ 4
    | _ => False
  ser v := match v with
    | .seq vs => Sch.serialize (schemaOfCodecs cs) (serFields cs vs)
    | _ => []
  dec bs :=
    match Sch.decode (schemaOfCodecs cs) bs with
    | some fields => (decFields cs fields).map .seq
    | none => none
  kind := kindOfCodecs cs
  htr v := match v with
    | .seq vs => Merkle.merkleRoot (Merkle.depthFor cs.length) (htrFields cs vs)
    | _ => []
  tree v := match v with
    | .seq vs => Merkle.merkleTree (Merkle.depthFor cs.length) (treeFields cs vs)
    | _ => .leaf []
  htr_tree := by
    intro v hwt
    cases v
    case seq vs =>
      have hwt' : WTFields cs vs := hwt
      show (Merkle.merkleTree _ _).root = _
      rw [Merkle.root_merkleTree, map_root_treeFields hwt']
    all_goals rfl
  canon := by
    intro bs v h
    cases hd : Sch.decode (schemaOfCodecs cs) bs with
    | none =>
      simp only [hd] at h
      exact absurd h (by simp)
    | some fields =>
      simp only [hd] at h
      cases hdf : decFields cs fields with
      | none => simp only [hdf] at h; exact absurd h (by simp)
      | some vs =>
        simp only [hdf, Option.map_some, Option.some.injEq] at h
        subst h
        obtain ⟨hwt, hser⟩ := decFields_canon hdf
        obtain ⟨_, hbs⟩ := Sch.serialize_decode hd
        refine ⟨hwt, ?_⟩
        show Sch.serialize (schemaOfCodecs cs) (serFields cs vs) = bs
        rw [hser]
        exact hbs
  round := by
    intro v hwt hrep
    cases v
    case seq vs =>
      have hrep' : RepFields cs vs ∧
          Sch.fixedLen (schemaOfCodecs cs)
            + Sch.heapLen (schemaOfCodecs cs) (serFields cs vs) < 256 ^ 4 := hrep
      obtain ⟨hreps, hbound⟩ := hrep'
      have hwt' : WTFields cs vs := hwt
      show (match Sch.decode (schemaOfCodecs cs)
          (Sch.serialize (schemaOfCodecs cs) (serFields cs vs)) with
        | some fields => (decFields cs fields).map Val.seq
        | none => none) = some (.seq vs)
      simp only [Sch.decode_serialize (wellTyped_serFields hwt') hbound,
        decFields_serFields hwt' hreps, Option.map_some]
    all_goals exact hwt.elim
  ser_len := by
    intro v n hwt hk
    cases v
    case seq vs =>
      have hwt' : WTFields cs vs := hwt
      simp only [kindOfCodecs] at hk
      by_cases hall : (cs.all fun c => c.kind.isSome) = true
      · rw [if_pos hall] at hk
        injection hk with hk
        have hnovar := schema_no_var_of_all_fixed hall
        have hwtf := wellTyped_serFields hwt'
        show (Sch.serialize (schemaOfCodecs cs) (serFields cs vs)).length = n
        simp only [Sch.serialize, List.length_append]
        rw [Sch.length_serializeFixed hwtf, varVals_no_var hnovar]
        simp only [Sch.cat, List.length_nil, Nat.add_zero]
        exact hk
      · rw [if_neg hall] at hk
        exact absurd hk (by simp)
    all_goals exact hwt.elim

/-! ## Homogeneous elements (lists and vectors) -/

/-- Element-wise typing. -/
def WTElems (c : Codec) (vs : List Val) : Prop := ∀ v ∈ vs, c.wt v

/-- Element-wise representability. -/
def RepElems (c : Codec) (vs : List Val) : Prop := ∀ v ∈ vs, c.rep v

/-- Serialize every element. -/
def serElems (c : Codec) (vs : List Val) : List (List UInt8) := vs.map c.ser

/-- Decode every element byte string. -/
def decElems (c : Codec) : List (List UInt8) → Option (List Val)
  | [] => some []
  | b :: bs =>
    match c.dec b, decElems c bs with
    | some v, some vs => some (v :: vs)
    | _, _ => none

theorem decElems_canon {c : Codec} {fields : List (List UInt8)}
    {vs : List Val} (h : decElems c fields = some vs) :
    WTElems c vs ∧ serElems c vs = fields := by
  induction fields generalizing vs with
  | nil =>
    have h' : some ([] : List Val) = some vs := h
    injection h' with h'
    subst h'
    exact ⟨fun v hv => absurd hv (by simp), rfl⟩
  | cons b fs ih =>
    simp only [decElems] at h
    cases hd : c.dec b with
    | none =>
      simp only [hd] at h
      exact absurd h (by simp)
    | some v' =>
      cases hds : decElems c fs with
      | none =>
        simp only [hd, hds] at h
        exact absurd h (by simp)
      | some vs' =>
        simp only [hd, hds, Option.some.injEq] at h
        subst h
        obtain ⟨hwt, hser⟩ := c.canon hd
        obtain ⟨hwts, hsers⟩ := ih hds
        refine ⟨?_, by simp only [serElems, List.map_cons] at *; rw [hser, hsers]⟩
        intro v hv
        simp only [List.mem_cons] at hv
        cases hv with
        | inl h1 => subst h1; exact hwt
        | inr h2 => exact hwts v h2

theorem decElems_serElems {c : Codec} {vs : List Val}
    (hwt : WTElems c vs) (hrep : RepElems c vs) :
    decElems c (serElems c vs) = some vs := by
  induction vs with
  | nil => rfl
  | cons v vs ih =>
    have hw : c.wt v := hwt v (by simp)
    have hr : c.rep v := hrep v (by simp)
    have hws : WTElems c vs := fun w hwm => hwt w (by simp [hwm])
    have hrs : RepElems c vs := fun w hwm => hrep w (by simp [hwm])
    show decElems c (c.ser v :: serElems c vs) = some (v :: vs)
    simp only [decElems, c.round hw hr, ih hws hrs]

theorem sized_serElems {c : Codec} {vs : List Val} {k : Nat}
    (hwt : WTElems c vs) (hk : c.kind = some k) :
    FixedSeq.Sized k (serElems c vs) := by
  intro b hb
  simp only [serElems, List.mem_map] at hb
  obtain ⟨v, hv, rfl⟩ := hb
  exact c.ser_len (hwt v hv) hk

theorem length_serElems (c : Codec) (vs : List Val) :
    (serElems c vs).length = vs.length := by
  simp [serElems]

theorem length_decElems {c : Codec} {fields : List (List UInt8)}
    {vs : List Val} (h : decElems c fields = some vs) :
    vs.length = fields.length := by
  have hser := (decElems_canon h).2
  have hlen := congrArg List.length hser
  rw [length_serElems] at hlen
  exact hlen

theorem map_root_treeElems {c : Codec} {vs : List Val}
    (hwt : WTElems c vs) :
    (vs.map c.tree).map Merkle.Node.root = vs.map c.htr := by
  induction vs with
  | nil => rfl
  | cons v vs ih =>
    simp only [List.map_cons, c.htr_tree (hwt v (by simp)),
      ih (fun w hw => hwt w (by simp [hw]))]

/-- SSZ `List[T, limit]` with fixed-size `T` (`k` bytes per element): packed
    encoding, count derived from the byte length. -/
def listFixedCodec (c : Codec) (k limit : Nat) (hk : c.kind = some k)
    (hkpos : 0 < k) : Codec where
  wt v := match v with
    | .seq vs => WTElems c vs ∧ vs.length ≤ limit
    | _ => False
  rep v := match v with
    | .seq vs => RepElems c vs
    | _ => False
  ser v := match v with
    | .seq vs => FixedSeq.serializeSeq (serElems c vs)
    | _ => []
  dec bs :=
    match FixedSeq.decodeFixedList k limit bs with
    | some fields => (decElems c fields).map .seq
    | none => none
  kind := none
  htr v := match v with
    | .seq vs => Merkle.mixIn
        (Merkle.merkleRoot (Merkle.depthFor limit) (vs.map c.htr))
        (Merkle.lengthWord vs.length)
    | _ => []
  tree v := match v with
    | .seq vs => .branch
        (Merkle.merkleTree (Merkle.depthFor limit) (vs.map c.tree))
        (.leaf (Merkle.lengthWord vs.length))
    | _ => .leaf []
  htr_tree := by
    intro v hwt
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length ≤ limit := hwt
      show Merkle.hashPair _ _ = _
      rw [Merkle.root_merkleTree, map_root_treeElems hwt'.1]
      rfl
    all_goals rfl
  canon := by
    intro bs v h
    cases hd : FixedSeq.decodeFixedList k limit bs with
    | none =>
      simp only [hd] at h
      exact absurd h (by simp)
    | some fields =>
      simp only [hd] at h
      cases hdf : decElems c fields with
      | none =>
        simp only [hdf] at h
        exact absurd h (by simp)
      | some vs =>
        simp only [hdf, Option.map_some, Option.some.injEq] at h
        subst h
        obtain ⟨_, hcat, hlim⟩ := FixedSeq.serializeSeq_decodeFixedList hd
        obtain ⟨hwt, hser⟩ := decElems_canon hdf
        have hclen := congrArg List.length hcat
        refine ⟨⟨hwt, ?_⟩, ?_⟩
        · rw [length_decElems hdf]
          exact hlim
        · show FixedSeq.serializeSeq (serElems c vs) = bs
          rw [hser]
          exact hcat
  round := by
    intro v hwt hrep
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length ≤ limit := hwt
      have hrep' : RepElems c vs := hrep
      obtain ⟨hwts, hlim⟩ := hwt'
      have hsized := sized_serElems hwts hk
      have hslen : (serElems c vs).length ≤ limit := by
        rw [length_serElems]
        exact hlim
      show (match FixedSeq.decodeFixedList k limit
          (FixedSeq.serializeSeq (serElems c vs)) with
        | some fields => (decElems c fields).map Val.seq
        | none => none) = some (.seq vs)
      simp only [FixedSeq.decodeFixedList_serializeSeq hkpos hsized hslen,
        decElems_serElems hwts hrep', Option.map_some]
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hkn
    exact absurd hkn (by simp)

/-- SSZ `List[T, limit]` with variable-size `T`: offset-table encoding,
    count derived from the first offset (`VarList.lean`). -/
def listVarCodec (c : Codec) (limit : Nat) : Codec where
  wt v := match v with
    | .seq vs => WTElems c vs ∧ vs.length ≤ limit
    | _ => False
  rep v := match v with
    | .seq vs => RepElems c vs ∧
        4 * vs.length + (Sch.cat (serElems c vs)).length < 256 ^ 4
    | _ => False
  ser v := match v with
    | .seq vs => VarList.serializeList (serElems c vs)
    | _ => []
  dec bs :=
    match VarList.decodeList limit bs with
    | some fields => (decElems c fields).map .seq
    | none => none
  kind := none
  htr v := match v with
    | .seq vs => Merkle.mixIn
        (Merkle.merkleRoot (Merkle.depthFor limit) (vs.map c.htr))
        (Merkle.lengthWord vs.length)
    | _ => []
  tree v := match v with
    | .seq vs => .branch
        (Merkle.merkleTree (Merkle.depthFor limit) (vs.map c.tree))
        (.leaf (Merkle.lengthWord vs.length))
    | _ => .leaf []
  htr_tree := by
    intro v hwt
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length ≤ limit := hwt
      show Merkle.hashPair _ _ = _
      rw [Merkle.root_merkleTree, map_root_treeElems hwt'.1]
      rfl
    all_goals rfl
  canon := by
    intro bs v h
    cases hd : VarList.decodeList limit bs with
    | none =>
      simp only [hd] at h
      exact absurd h (by simp)
    | some fields =>
      simp only [hd] at h
      cases hdf : decElems c fields with
      | none =>
        simp only [hdf] at h
        exact absurd h (by simp)
      | some vs =>
        simp only [hdf, Option.map_some, Option.some.injEq] at h
        subst h
        obtain ⟨hser, hlim⟩ := VarList.serializeList_decodeList hd
        obtain ⟨hwt, hserf⟩ := decElems_canon hdf
        refine ⟨⟨hwt, by rw [length_decElems hdf]; exact hlim⟩, ?_⟩
        show VarList.serializeList (serElems c vs) = bs
        rw [hserf]
        exact hser
  round := by
    intro v hwt hrep
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length ≤ limit := hwt
      have hrep' : RepElems c vs ∧
          4 * vs.length + (Sch.cat (serElems c vs)).length < 256 ^ 4 := hrep
      obtain ⟨hwts, hlim⟩ := hwt'
      obtain ⟨hreps, hbound⟩ := hrep'
      have hslim : (serElems c vs).length ≤ limit := by
        rw [length_serElems]
        exact hlim
      have hsbound : 4 * (serElems c vs).length
          + (Sch.cat (serElems c vs)).length < 256 ^ 4 := by
        rw [length_serElems]
        exact hbound
      show (match VarList.decodeList limit
          (VarList.serializeList (serElems c vs)) with
        | some fields => (decElems c fields).map Val.seq
        | none => none) = some (.seq vs)
      simp only [VarList.decodeList_serializeList (serElems c vs) limit
        hslim hsbound, decElems_serElems hwts hreps, Option.map_some]
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hkn
    exact absurd hkn (by simp)

/-- SSZ `Vector[T, n]` with fixed-size `T` (`k` bytes per element): packed
    encoding, fixed total size `k * n`. -/
def vectorFixedCodec (c : Codec) (k n : Nat) (hk : c.kind = some k) :
    Codec where
  wt v := match v with
    | .seq vs => WTElems c vs ∧ vs.length = n
    | _ => False
  rep v := match v with
    | .seq vs => RepElems c vs
    | _ => False
  ser v := match v with
    | .seq vs => FixedSeq.serializeSeq (serElems c vs)
    | _ => []
  dec bs :=
    match FixedSeq.decodeFixedVector k n bs with
    | some fields => (decElems c fields).map .seq
    | none => none
  kind := some (k * n)
  htr v := match v with
    | .seq vs => Merkle.merkleRoot (Merkle.depthFor n) (vs.map c.htr)
    | _ => []
  tree v := match v with
    | .seq vs => Merkle.merkleTree (Merkle.depthFor n) (vs.map c.tree)
    | _ => .leaf []
  htr_tree := by
    intro v hwt
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length = n := hwt
      show (Merkle.merkleTree _ _).root = _
      rw [Merkle.root_merkleTree, map_root_treeElems hwt'.1]
    all_goals rfl
  canon := by
    intro bs v h
    cases hd : FixedSeq.decodeFixedVector k n bs with
    | none =>
      simp only [hd] at h
      exact absurd h (by simp)
    | some fields =>
      simp only [hd] at h
      cases hdf : decElems c fields with
      | none =>
        simp only [hdf] at h
        exact absurd h (by simp)
      | some vs =>
        simp only [hdf, Option.map_some, Option.some.injEq] at h
        subst h
        obtain ⟨_, hcat, hn⟩ := FixedSeq.serializeSeq_decodeFixedVector hd
        obtain ⟨hwt, hser⟩ := decElems_canon hdf
        refine ⟨⟨hwt, by rw [length_decElems hdf]; exact hn⟩, ?_⟩
        show FixedSeq.serializeSeq (serElems c vs) = bs
        rw [hser]
        exact hcat
  round := by
    intro v hwt hrep
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length = n := hwt
      have hrep' : RepElems c vs := hrep
      obtain ⟨hwts, hn⟩ := hwt'
      have hsized := sized_serElems hwts hk
      have hslen : (serElems c vs).length = n := by
        rw [length_serElems]
        exact hn
      show (match FixedSeq.decodeFixedVector k n
          (FixedSeq.serializeSeq (serElems c vs)) with
        | some fields => (decElems c fields).map Val.seq
        | none => none) = some (.seq vs)
      simp only [FixedSeq.decodeFixedVector_serializeSeq hsized hslen,
        decElems_serElems hwts hrep', Option.map_some]
    all_goals exact hwt.elim
  ser_len := by
    intro v m hwt hkn
    injection hkn with hkn
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length = n := hwt
      obtain ⟨hwts, hn⟩ := hwt'
      show (FixedSeq.serializeSeq (serElems c vs)).length = m
      simp only [FixedSeq.serializeSeq]
      rw [FixedSeq.length_cat_sized (sized_serElems hwts hk),
        length_serElems, hn, hkn]
    all_goals exact hwt.elim

/-- SSZ `Vector[T, n]` with variable-size `T`: offset-table encoding at the
    schema-fixed count (`VarList.lean` vectors). -/
def vectorVarCodec (c : Codec) (n : Nat) : Codec where
  wt v := match v with
    | .seq vs => WTElems c vs ∧ vs.length = n
    | _ => False
  rep v := match v with
    | .seq vs => RepElems c vs ∧
        4 * n + (Sch.cat (serElems c vs)).length < 256 ^ 4
    | _ => False
  ser v := match v with
    | .seq vs => VarList.serializeVector n (serElems c vs)
    | _ => []
  dec bs :=
    match VarList.decodeVector n bs with
    | some fields => (decElems c fields).map .seq
    | none => none
  kind := none
  htr v := match v with
    | .seq vs => Merkle.merkleRoot (Merkle.depthFor n) (vs.map c.htr)
    | _ => []
  tree v := match v with
    | .seq vs => Merkle.merkleTree (Merkle.depthFor n) (vs.map c.tree)
    | _ => .leaf []
  htr_tree := by
    intro v hwt
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length = n := hwt
      show (Merkle.merkleTree _ _).root = _
      rw [Merkle.root_merkleTree, map_root_treeElems hwt'.1]
    all_goals rfl
  canon := by
    intro bs v h
    cases hd : VarList.decodeVector n bs with
    | none =>
      simp only [hd] at h
      exact absurd h (by simp)
    | some fields =>
      simp only [hd] at h
      cases hdf : decElems c fields with
      | none =>
        simp only [hdf] at h
        exact absurd h (by simp)
      | some vs =>
        simp only [hdf, Option.map_some, Option.some.injEq] at h
        subst h
        obtain ⟨hn, hser⟩ := VarList.serializeVector_decodeVector hd
        obtain ⟨hwt, hserf⟩ := decElems_canon hdf
        refine ⟨⟨hwt, by rw [length_decElems hdf]; exact hn⟩, ?_⟩
        show VarList.serializeVector n (serElems c vs) = bs
        rw [hserf]
        exact hser
  round := by
    intro v hwt hrep
    cases v
    case seq vs =>
      have hwt' : WTElems c vs ∧ vs.length = n := hwt
      have hrep' : RepElems c vs ∧
          4 * n + (Sch.cat (serElems c vs)).length < 256 ^ 4 := hrep
      obtain ⟨hwts, hn⟩ := hwt'
      obtain ⟨hreps, hbound⟩ := hrep'
      have hslen : (serElems c vs).length = n := by
        rw [length_serElems]
        exact hn
      show (match VarList.decodeVector n
          (VarList.serializeVector n (serElems c vs)) with
        | some fields => (decElems c fields).map Val.seq
        | none => none) = some (.seq vs)
      simp only [VarList.decodeVector_serializeVector hslen hbound,
        decElems_serElems hwts hreps, Option.map_some]
    all_goals exact hwt.elim
  ser_len := by
    intro v m _ hkn
    exact absurd hkn (by simp)

/-! ## CompatibleUnion (EIP-8016) -/

/-- SSZ `CompatibleUnion`: a one-byte selector followed by the selected
    variant's payload, consumed exactly. -/
def unionCodec (variants : List (Nat × Codec)) : Codec where
  wt v := match v with
    | .union sel pv =>
      match variants.lookup sel with
      | some c => c.wt pv ∧ sel < 256
      | none => False
    | _ => False
  rep v := match v with
    | .union sel pv =>
      match variants.lookup sel with
      | some c => c.rep pv
      | none => True
    | _ => False
  ser v := match v with
    | .union sel pv =>
      match variants.lookup sel with
      | some c => UInt8.ofNat sel :: c.ser pv
      | none => []
    | _ => []
  dec bs := match bs with
    | [] => none
    | s :: rest =>
      match variants.lookup s.toNat with
      | some c => (c.dec rest).map (.union s.toNat)
      | none => none
  kind := none
  htr v := match v with
    | .union sel pv =>
      match variants.lookup sel with
      | some c => Merkle.mixIn (c.htr pv) (Merkle.selectorWord sel)
      | none => []
    | _ => []
  tree v := match v with
    | .union sel pv =>
      match variants.lookup sel with
      | some c => .branch (c.tree pv) (.leaf (Merkle.selectorWord sel))
      | none => .leaf []
    | _ => .leaf []
  htr_tree := by
    intro v hwt
    cases v
    case union sel pv =>
      have hwt' : (match variants.lookup sel with
        | some c => c.wt pv ∧ sel < 256
        | none => False) := hwt
      cases hl : variants.lookup sel with
      | none => simp only [hl] at hwt'
      | some c =>
        simp only [hl] at hwt'
        simp only [hl]
        show Merkle.hashPair (c.tree pv).root _ = _
        rw [c.htr_tree hwt'.1]
        rfl
    all_goals rfl
  canon := by
    intro bs v h
    match bs with
    | [] => exact absurd (show (none : Option Val) = some v from h) (by simp)
    | s :: rest =>
      have h' : (match variants.lookup s.toNat with
        | some c => (c.dec rest).map (Val.union s.toNat)
        | none => none) = some v := h
      cases hl : variants.lookup s.toNat with
      | none =>
        simp only [hl] at h'
        exact absurd h' (by simp)
      | some c =>
        simp only [hl] at h'
        cases hd : c.dec rest with
        | none =>
          simp only [hd] at h'
          exact absurd h' (by simp)
        | some pv =>
          simp only [hd, Option.map_some, Option.some.injEq] at h'
          subst h'
          obtain ⟨hwt, hser⟩ := c.canon hd
          constructor
          · show (match variants.lookup s.toNat with
              | some c => c.wt pv ∧ s.toNat < 256
              | none => False)
            simp only [hl]
            exact ⟨hwt, s.toNat_lt⟩
          · show (match variants.lookup s.toNat with
              | some c => UInt8.ofNat s.toNat :: c.ser pv
              | none => []) = s :: rest
            simp only [hl, UInt8.ofNat_toNat, hser]
  round := by
    intro v hwt hrep
    cases v
    case union sel pv =>
      have hwt' : (match variants.lookup sel with
        | some c => c.wt pv ∧ sel < 256
        | none => False) := hwt
      have hrep' : (match variants.lookup sel with
        | some c => c.rep pv
        | none => True) := hrep
      cases hl : variants.lookup sel with
      | none =>
        simp only [hl] at hwt'
      | some c =>
        simp only [hl] at hwt' hrep'
        obtain ⟨hw, hsel⟩ := hwt'
        have htn : (UInt8.ofNat sel).toNat = sel := by
          have h256 : (UInt8.ofNat sel).toNat = sel % 2 ^ 8 := rfl
          rw [h256, show (2:Nat) ^ 8 = 256 from by decide]
          exact Nat.mod_eq_of_lt hsel
        show (match (match variants.lookup sel with
            | some c => UInt8.ofNat sel :: c.ser pv
            | none => []) with
          | [] => none
          | s :: rest =>
            match variants.lookup s.toNat with
            | some c => (c.dec rest).map (Val.union s.toNat)
            | none => none) = some (.union sel pv)
        simp only [hl, htn, c.round hw hrep', Option.map_some]
    all_goals exact hwt.elim
  ser_len := by
    intro v n _ hk
    exact absurd hk (by simp)

/-! ## Packed-merkleization and progressive variants

Same wire formats as their base combinators (the `with`-update keeps
`ser`/`dec` and their canonicity/roundtrip proofs untouched); only the
merkleization differs. -/

/-- Stand-in bound for SSZ types without a declared limit (progressive
    lists/bitlists). Unreachable: offsets cap wire-derived counts far below. -/
def noLimit : Nat := 2 ^ 64

/-- SSZ `List[T, limit]` with *basic* `T` (uintN/bool, `k` bytes each):
    merkleization packs the serialized elements into shared chunks. -/
def listPackedCodec (c : Codec) (k limit : Nat) (hk : c.kind = some k)
    (hkpos : 0 < k) : Codec :=
  { listFixedCodec c k limit hk hkpos with
    htr := fun v => match v with
      | .seq vs => Merkle.mixIn
          (Merkle.merkleRoot (Merkle.depthFor (Merkle.byteChunks (limit * k)))
            (Merkle.packBytes (Sch.cat (serElems c vs))))
          (Merkle.lengthWord vs.length)
      | _ => []
    tree := fun v => match v with
      | .seq vs => .branch
          (Merkle.merkleTree (Merkle.depthFor (Merkle.byteChunks (limit * k)))
            (Merkle.chunkNodes (Merkle.packBytes (Sch.cat (serElems c vs)))))
          (.leaf (Merkle.lengthWord vs.length))
      | _ => .leaf []
    htr_tree := by
      intro v _
      cases v
      case seq vs =>
        show Merkle.hashPair _ _ = _
        rw [Merkle.root_merkleTree_chunks]
        rfl
      all_goals rfl }

/-- SSZ `Vector[T, n]` with *basic* `T` (`k` bytes each): packed
    merkleization. -/
def vectorPackedCodec (c : Codec) (k n : Nat) (hk : c.kind = some k) :
    Codec :=
  { vectorFixedCodec c k n hk with
    htr := fun v => match v with
      | .seq vs => Merkle.merkleRoot (Merkle.depthFor (Merkle.byteChunks (n * k)))
          (Merkle.packBytes (Sch.cat (serElems c vs)))
      | _ => []
    tree := fun v => match v with
      | .seq vs => Merkle.merkleTree (Merkle.depthFor (Merkle.byteChunks (n * k)))
          (Merkle.chunkNodes (Merkle.packBytes (Sch.cat (serElems c vs))))
      | _ => .leaf []
    htr_tree := by
      intro v _
      cases v
      case seq vs => exact Merkle.root_merkleTree_chunks _ _
      all_goals rfl }

/-- EIP-7916 `ProgressiveList[T]` with basic `T`: list wire format (bounded
    by `lim`, unbounded by default), progressive packed merkleization. -/
def progListPackedCodec (c : Codec) (k : Nat) (hk : c.kind = some k)
    (hkpos : 0 < k) (lim : Nat := noLimit) : Codec :=
  { listFixedCodec c k lim hk hkpos with
    htr := fun v => match v with
      | .seq vs => Merkle.mixIn
          (Merkle.progRoot (Merkle.packBytes (Sch.cat (serElems c vs))))
          (Merkle.lengthWord vs.length)
      | _ => []
    tree := fun v => match v with
      | .seq vs => .branch
          (Merkle.progTree (Merkle.chunkNodes
            (Merkle.packBytes (Sch.cat (serElems c vs)))))
          (.leaf (Merkle.lengthWord vs.length))
      | _ => .leaf []
    htr_tree := by
      intro v _
      cases v
      case seq vs =>
        show Merkle.hashPair _ _ = _
        rw [Merkle.root_progTree_chunks]
        rfl
      all_goals rfl }

/-- EIP-7916 `ProgressiveList[T]` with fixed-size composite `T`: one root
    per element on the progressive spine. -/
def progListFixedCodec (c : Codec) (k : Nat) (hk : c.kind = some k)
    (hkpos : 0 < k) (lim : Nat := noLimit) : Codec :=
  { listFixedCodec c k lim hk hkpos with
    htr := fun v => match v with
      | .seq vs => Merkle.mixIn (Merkle.progRoot (vs.map c.htr))
          (Merkle.lengthWord vs.length)
      | _ => []
    tree := fun v => match v with
      | .seq vs => .branch (Merkle.progTree (vs.map c.tree))
          (.leaf (Merkle.lengthWord vs.length))
      | _ => .leaf []
    htr_tree := by
      intro v hwt
      cases v
      case seq vs =>
        have hwt' : WTElems c vs ∧ vs.length ≤ lim := hwt
        show Merkle.hashPair _ _ = _
        rw [Merkle.root_progTree, map_root_treeElems hwt'.1]
        rfl
      all_goals rfl }

/-- EIP-7916 `ProgressiveList[T]` with variable-size `T`. -/
def progListVarCodec (c : Codec) (lim : Nat := noLimit) : Codec :=
  { listVarCodec c lim with
    htr := fun v => match v with
      | .seq vs => Merkle.mixIn (Merkle.progRoot (vs.map c.htr))
          (Merkle.lengthWord vs.length)
      | _ => []
    tree := fun v => match v with
      | .seq vs => .branch (Merkle.progTree (vs.map c.tree))
          (.leaf (Merkle.lengthWord vs.length))
      | _ => .leaf []
    htr_tree := by
      intro v hwt
      cases v
      case seq vs =>
        have hwt' : WTElems c vs ∧ vs.length ≤ lim := hwt
        show Merkle.hashPair _ _ = _
        rw [Merkle.root_progTree, map_root_treeElems hwt'.1]
        rfl
      all_goals rfl }

/-- EIP-7916 `ProgressiveBitList`. -/
def progBitlistCodec (lim : Nat := noLimit) : Codec :=
  { bitlistCodec lim with
    htr := fun v => match v with
      | .bits l => Merkle.mixIn (Merkle.progRoot (Merkle.packBits l))
          (Merkle.lengthWord l.length)
      | _ => []
    tree := fun v => match v with
      | .bits l => .branch (Merkle.progTree (Merkle.chunkNodes (Merkle.packBits l)))
          (.leaf (Merkle.lengthWord l.length))
      | _ => .leaf []
    htr_tree := by
      intro v _
      cases v
      case bits l =>
        show Merkle.hashPair _ _ = _
        rw [Merkle.root_progTree_chunks]
        rfl
      all_goals rfl }

/-! ## Progressive containers (EIP-7495) -/

/-- Layout-position roots: a cleared bit keeps its zero leaf, a set bit
    consumes the next field. -/
def progFieldRoots : List Bool → List Codec → List Val → List Merkle.Hash
  | [], _, _ => []
  | false :: rest, cs, vs => Merkle.zeroHash 0 :: progFieldRoots rest cs vs
  | true :: rest, c :: cs, v :: vs => c.htr v :: progFieldRoots rest cs vs
  | true :: _, _, _ => []

/-- Layout-position subtrees. -/
def progFieldNodes : List Bool → List Codec → List Val → List Merkle.Node
  | [], _, _ => []
  | false :: rest, cs, vs =>
    .leaf (Merkle.zeroHash 0) :: progFieldNodes rest cs vs
  | true :: rest, c :: cs, v :: vs => c.tree v :: progFieldNodes rest cs vs
  | true :: _, _, _ => []

theorem map_root_progFieldNodes (layout : List Bool) {cs : List Codec}
    {vs : List Val} (hwt : WTFields cs vs) :
    (progFieldNodes layout cs vs).map Merkle.Node.root
      = progFieldRoots layout cs vs := by
  induction layout generalizing cs vs with
  | nil => rfl
  | cons b rest ih =>
    cases b with
    | false =>
      simp only [progFieldNodes, progFieldRoots, List.map_cons,
        Merkle.Node.root, ih hwt]
    | true =>
      cases cs with
      | nil =>
        cases vs with
        | nil => rfl
        | cons v vs => exact hwt.elim
      | cons c cs' =>
        cases vs with
        | nil => exact hwt.elim
        | cons v vs' =>
          obtain ⟨hw, hws⟩ := hwt
          simp only [progFieldNodes, progFieldRoots, List.map_cons,
            c.htr_tree hw, ih hws]

/-- EIP-7495 `ProgressiveContainer`: container wire format over the declared
    fields; merkleization spreads the field roots over the layout positions
    on a progressive spine and mixes in the active-fields word. -/
def progContainerCodec (cs : List Codec) (layout : List Bool) : Codec :=
  { containerCodec cs with
    htr := fun v => match v with
      | .seq vs => Merkle.mixIn (Merkle.progRoot (progFieldRoots layout cs vs))
          (Merkle.activeFieldsWord layout)
      | _ => []
    tree := fun v => match v with
      | .seq vs => .branch (Merkle.progTree (progFieldNodes layout cs vs))
          (.leaf (Merkle.activeFieldsWord layout))
      | _ => .leaf []
    htr_tree := by
      intro v hwt
      cases v
      case seq vs =>
        have hwt' : WTFields cs vs := hwt
        show Merkle.hashPair _ _ = _
        rw [Merkle.root_progTree, map_root_progFieldNodes layout hwt']
        rfl
      all_goals rfl }

end Ssz
