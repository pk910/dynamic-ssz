import Lean.Data.Json
import EthLib.Serialization.Ssz

/-! # Declarations read at run time

A declaration in the JSON spelling of the `ethereum/ssz-specs` conformance
vectors (`kind`, `bits`, `length`, `limit`, `elementType`, `fields`,
`activeFields`, `options`), turned into an EthLib `Desc`. The oracle accepts
such declarations on its stdin (`T <name> <json>`), so the Go side can submit
any type it generates without a certificate file: EthLib's theorems are
generic in the declaration, and the oracle evaluates `Desc.wellFormed` before
it answers for one. -/

namespace SszSpec

open Lean EthLib.Serialization.Ssz

/-- A whole number a declaration states under a key. -/
private def wholeAt (json : Json) (key : String) : Except String Nat :=
  match json.getObjVal? key with
  | .ok (.num n) =>
    if n.exponent == 0 && n.mantissa ≥ 0 then .ok n.mantissa.toNat
    else .error s!"{key}: not a whole number"
  | .ok _ => .error s!"{key}: not a number"
  | .error _ => .error s!"{key}: missing"

/-- The optional bound of a progressive shape. -/
private def boundAt (json : Json) : Except String (Option Nat) :=
  match json.getObjVal? "limit" with
  | .ok .null => .ok none
  | .error _ => .ok none
  | .ok _ => (some ·) <$> wholeAt json "limit"

/-- Whether a kind names an unsigned integer (`Uint8` … `Uint256`). -/
private def isUintKind (kind : String) : Bool :=
  kind.startsWith "Uint" && kind.length > 4 && (kind.toList.drop 4).all Char.isDigit

/-- Read a declaration. -/
partial def readDesc (json : Json) : Except String Desc := do
  let kind ← match json.getObjVal? "kind" with
    | .ok (.str k) => pure k
    | _ => throw "kind: missing"
  let element : Except String Desc :=
    match json.getObjVal? "elementType" with
    | .ok e => readDesc e
    | .error _ => throw "elementType: missing"
  let parts (key marker : String) : Except String (List (Json × Desc)) :=
    match json.getObjVal? key with
    | .ok (.arr items) => items.toList.mapM fun item => do
        let label ← match item.getObjVal? marker with
          | .ok l => pure l
          | .error _ => throw s!"{marker}: missing"
        let typ ← match item.getObjVal? "type" with
          | .ok t => pure t
          | .error _ => throw "type: missing"
        return (label, ← readDesc typ)
    | _ => throw s!"{key}: missing"
  let names (ps : List (Json × Desc)) : Except String (List String) :=
    ps.mapM fun (l, _) => match l with
      | .str s => pure s
      | _ => throw "name: not a string"
  match kind with
  | "Boolean" => return .bool
  | "Byte" => return .uint 1
  | "BitVector" => return .bitVector (← wholeAt json "length")
  | "BitList" => return .bitList (← wholeAt json "limit")
  | "ProgressiveBitList" => return .progressiveBitList (← boundAt json)
  | "ByteVector" => return .byteVector (← wholeAt json "length")
  | "ByteList" => return .byteList (← wholeAt json "limit")
  | "Vector" => return .vector (← element) (← wholeAt json "length")
  | "List" => return .list (← element) (← wholeAt json "limit")
  | "ProgressiveList" => return .progressiveList (← element) (← boundAt json)
  | "Container" =>
    let ps ← parts "fields" "name"
    return .container (← names ps) (ps.map (·.2))
  | "ProgressiveContainer" =>
    let bits ← match json.getObjVal? "activeFields" with
      | .ok (.arr bits) => pure bits.toList
      | _ => throw "activeFields: missing"
    let active ← bits.mapM fun b => match b with
      | .bool v => pure v
      | .num n => pure (n.mantissa != 0)
      | _ => throw "activeFields: not bits"
    let ps ← parts "fields" "name"
    return .progressiveContainer active (← names ps) (ps.map (·.2))
  | "CompatibleUnion" =>
    let ps ← parts "options" "selector"
    let sels ← ps.mapM fun (l, _) => match l with
      | .num n => if n.exponent == 0 && n.mantissa ≥ 0 then pure n.mantissa.toNat else throw "selector"
      | _ => throw "selector: not a number"
    return .compatibleUnion sels (ps.map (·.2))
  | k =>
    if isUintKind k then
      let bits ← wholeAt json "bits"
      if bits == 0 || bits % 8 != 0 then throw s!"bits: {bits} is not a byte width"
      return .uint (bits / 8)
    else throw s!"kind: unknown {k}"

/-- Parse and read a declaration from its text. -/
def parseDesc (text : String) : Except String Desc := do
  readDesc (← Json.parse text)

end SszSpec
