import Lean.Data.Json
import SszCert.Codec
import SszCert.Extended

/-! # Codecs built at run time

A shape in the JSON spelling of the ssz-specs conformance vectors, extended
with dynamic-ssz's kinds (`Union`, `Optional`, `BigInt`; signed integers,
floats and `Time` are their unsigned bit patterns), turned into a certified
`Codec`. Every combinator carries its theorems, so a codec built here is as
certified as one written in a certificate file: the oracle accepts such
declarations on its stdin (`T <name> <json>`). -/

namespace Ssz

open Lean

/-- A shape, as declared. -/
inductive Shape where
  | bool
  | uint (width : Nat)
  | byteVector (length : Nat)
  | byteList (limit : Nat)
  | bitVector (length : Nat)
  | bitList (limit : Nat)
  | progBitList (limit : Nat)
  | vector (element : Shape) (length : Nat)
  | list (element : Shape) (limit : Nat)
  | progList (element : Shape) (limit : Nat)
  | container (fields : List Shape)
  | progContainer (layout : List Bool) (fields : List Shape)
  | compatUnion (options : List (Nat × Shape))
  | classicUnion (hasNone : Bool) (options : List (Nat × Shape))
  | optional (element : Shape)
  | bigInt (limit : Nat)
  deriving Inhabited

/-- Whether the shape is a basic (packed) element. -/
def Shape.basic : Shape → Bool
  | .bool | .uint _ => true
  | _ => false

instance : Inhabited Codec := ⟨boolCodec⟩

/-- Build the certified codec of a shape. The combinators that need the
element width take it from the element codec's `kind` under the matching
equation, so the proof obligations are met by construction. -/
partial def Shape.codec : Shape → Codec
  | .bool => boolCodec
  | .uint w => uintCodec w
  | .byteVector n => byteVectorCodec n
  | .byteList lim => byteListCodec lim
  | .bitVector n => bitvectorCodec n
  | .bitList lim => bitlistCodec lim
  | .progBitList lim => progBitlistCodec lim
  | .vector e n =>
    let c := e.codec
    match hk : c.kind with
    | some k => if e.basic then vectorPackedCodec c k n hk else vectorFixedCodec c k n hk
    | none => vectorVarCodec c n
  | .list e lim =>
    let c := e.codec
    match hk : c.kind with
    | some k =>
      if hpos : 0 < k then
        if e.basic then listPackedCodec c k lim hk hpos else listFixedCodec c k lim hk hpos
      else listVarCodec c lim
    | none => listVarCodec c lim
  | .progList e lim =>
    let c := e.codec
    match hk : c.kind with
    | some k =>
      if hpos : 0 < k then
        if e.basic then progListPackedCodec c k hk hpos lim else progListFixedCodec c k hk hpos lim
      else progListVarCodec c lim
    | none => progListVarCodec c lim
  | .container fs => containerCodec (fs.map Shape.codec)
  | .progContainer layout fs => progContainerCodec (fs.map Shape.codec) layout
  | .compatUnion opts => unionCodec (opts.map fun (s, o) => (s, o.codec))
  | .classicUnion hasNone opts => classicUnionCodec hasNone (opts.map fun (s, o) => (s, o.codec))
  | .optional e => optionalCodec e.codec
  | .bigInt lim => bigIntCodec lim

/-! ## Reading the declaration -/

private def wholeAt (json : Json) (key : String) : Except String Nat :=
  match json.getObjVal? key with
  | .ok (.num n) =>
    if n.exponent == 0 && n.mantissa ≥ 0 then .ok n.mantissa.toNat
    else .error s!"{key}: not a whole number"
  | .ok _ => .error s!"{key}: not a number"
  | .error _ => .error s!"{key}: missing"

/-- A limit; `noLimit` where the declaration states none. -/
private def limitAt (json : Json) : Except String Nat :=
  match json.getObjVal? "limit" with
  | .ok .null => .ok noLimit
  | .error _ => .ok noLimit
  | .ok _ => wholeAt json "limit"

private def isUintKind (kind : String) : Bool :=
  kind.startsWith "Uint" && kind.length > 4 && (kind.toList.drop 4).all Char.isDigit

private def isIntKind (kind : String) : Bool :=
  kind.startsWith "Int" && kind.length > 3 && (kind.toList.drop 3).all Char.isDigit

/-- Read a declaration. -/
partial def readShape (json : Json) : Except String Shape := do
  let kind ← match json.getObjVal? "kind" with
    | .ok (.str k) => pure k
    | _ => throw "kind: missing"
  let element : Except String Shape :=
    match json.getObjVal? "elementType" with
    | .ok e => readShape e
    | .error _ => throw "elementType: missing"
  let parts (key marker : String) : Except String (List (Json × Option Shape)) :=
    match json.getObjVal? key with
    | .ok (.arr items) => items.toList.mapM fun item => do
        let label ← match item.getObjVal? marker with
          | .ok l => pure l
          | .error _ => throw s!"{marker}: missing"
        match item.getObjVal? "type" with
        | .ok .null => return (label, none)
        | .ok t => return (label, some (← readShape t))
        | .error _ => throw "type: missing"
    | _ => throw s!"{key}: missing"
  let selectors (ps : List (Json × Option Shape)) : Except String (List (Nat × Option Shape)) :=
    ps.mapM fun (l, o) => match l with
      | .num n => if n.exponent == 0 && n.mantissa ≥ 0 then pure (n.mantissa.toNat, o) else throw "selector"
      | _ => throw "selector: not a number"
  let fields (ps : List (Json × Option Shape)) : Except String (List Shape) :=
    ps.mapM fun (_, o) => match o with
      | some s => pure s
      | none => throw "field without a type"
  match kind with
  | "Boolean" => return .bool
  | "Byte" => return .uint 1
  | "Time" => return .uint 8
  | "Float32" => return .uint 4
  | "Float64" => return .uint 8
  | "BitVector" => return .bitVector (← wholeAt json "length")
  | "BitList" => return .bitList (← wholeAt json "limit")
  | "ProgressiveBitList" => return .progBitList (← limitAt json)
  | "ByteVector" => return .byteVector (← wholeAt json "length")
  | "ByteList" => return .byteList (← wholeAt json "limit")
  | "Vector" => return .vector (← element) (← wholeAt json "length")
  | "List" => return .list (← element) (← wholeAt json "limit")
  | "ProgressiveList" => return .progList (← element) (← limitAt json)
  | "Optional" => return .optional (← element)
  | "BigInt" => return .bigInt (← limitAt json)
  | "Container" => return .container (← fields (← parts "fields" "name"))
  | "ProgressiveContainer" =>
    let bits ← match json.getObjVal? "activeFields" with
      | .ok (.arr bits) => pure bits.toList
      | _ => throw "activeFields: missing"
    let active ← bits.mapM fun b => match b with
      | .bool v => pure v
      | .num n => pure (n.mantissa != 0)
      | _ => throw "activeFields: not bits"
    return .progContainer active (← fields (← parts "fields" "name"))
  | "CompatibleUnion" =>
    let opts ← selectors (← parts "options" "selector")
    return .compatUnion (← opts.mapM fun (s, o) => match o with
      | some sh => pure (s, sh)
      | none => throw "compatible union option without a type")
  | "Union" =>
    let opts ← selectors (← parts "options" "selector")
    let hasNone := opts.any fun (s, o) => s == 0 && o.isNone
    return .classicUnion hasNone (opts.filterMap fun (s, o) => o.map fun sh => (s, sh))
  | k =>
    if isUintKind k || isIntKind k then
      let bits ← wholeAt json "bits"
      if bits == 0 || bits % 8 != 0 then throw s!"bits: {bits} is not a byte width"
      return .uint (bits / 8)
    else throw s!"kind: unknown {k}"

/-- Parse and read a declaration from its text. -/
def parseShape (text : String) : Except String Shape := do
  readShape (← Json.parse text)

end Ssz
