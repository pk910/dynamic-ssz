import SszSpec.Certified
import SszSpec.Eth
import SszSpec.Declare

/-!
# ssz-spec-oracle

Decision oracle over the certified declarations, answered by the official Lean
specification of SSZ (`EthLib.Serialization.Ssz`). One request per stdin line:

* `D <type> <hex|->` — decode. `ACCEPT <root>` (the strict decoder accepts; the
  re-serialization is checked byte-for-byte, as canonicality guarantees) or
  `REJECT <reason>`.
* `P <type> <hex|-> <gindex>` — single branch. `PROOF <leaf> <h1,h2,...|->`
  (branch verified against the root on this side) or `REJECT <reason>`.
* `V <type> <hex|-> <gindex> <leaf> <h1,...|->` — verify a foreign branch
  against the value's root: `VERIFY true|false` or `REJECT <reason>`.
* `M <type> <hex|-> <i1,i2,...>` — multiproof. `MULTI <l1,...> <h1,...|->`
  (verified on this side) or `REJECT <reason>`.
* `W <type> <hex|-> <i1,...> <l1,...> <h1,...|->` — verify a foreign multiproof:
  `VERIFY true|false` or `REJECT <reason>`.
* `T <name> <json>` — declare a type for this session, in the JSON spelling
  of the ssz-specs conformance vectors (`SszSpec.Declare`), with no spaces.
  `OK` once `Desc.wellFormed` holds (EthLib's theorems then apply to it), or
  `ILLFORMED <reason>`. A declared name shadows a compiled one.
* `ERR ...` — malformed line or unknown type.
-/

open EthLib.Serialization.Ssz SszSpec

def hexNibble (c : Char) : Option Nat :=
  if '0' ≤ c ∧ c ≤ '9' then some (c.toNat - '0'.toNat)
  else if 'a' ≤ c ∧ c ≤ 'f' then some (c.toNat - 'a'.toNat + 10)
  else if 'A' ≤ c ∧ c ≤ 'F' then some (c.toNat - 'A'.toNat + 10)
  else none

def parseHex (s : String) : Option Bytes :=
  if s == "-" then some #[] else go s.toList #[]
where
  go : List Char → Array UInt8 → Option Bytes
  | [], acc => some acc
  | c1 :: c2 :: rest, acc =>
    match hexNibble c1, hexNibble c2 with
    | some h, some l => go rest (acc.push (UInt8.ofNat (h * 16 + l)))
    | _, _ => none
  | _, _ => none

def hexOf (bs : Bytes) : String :=
  bs.foldl (fun s b =>
    let d := "0123456789abcdef".toList
    (s.push d[b.toNat / 16]!).push d[b.toNat % 16]!) ""

def hexList (xs : List Bytes) : String :=
  if xs.isEmpty then "-" else ",".intercalate (xs.map hexOf)

def parseHexList (s : String) : Option (List Bytes) :=
  if s == "-" then some [] else (s.splitOn ",").mapM parseHex

def parseNats (s : String) : Option (List Nat) :=
  if s == "-" then some [] else (s.splitOn ",").mapM String.toNat?

def reject (fault : Err) : String := s!"REJECT {fault.reason}"

def decodeFor (shape : Desc) (bytes : Bytes) : Except Err (Value × Bytes) := do
  let value ← deserialize shape bytes
  let root ← hashTreeRoot shape value
  return (value, root)

def handle (declaredHere : List (String × Desc)) (words : List String) : String :=
  let lookup (name : String) : Option Desc :=
    (declaredHere.lookup name).orElse fun _ => (declared ++ declaredEth).lookup name
  match words with
  | ["D", name, hex] =>
    match lookup name, parseHex hex with
    | none, _ => s!"ERR unknown {name}"
    | _, none => "ERR badhex"
    | some shape, some bytes =>
      match decodeFor shape bytes with
      | .error fault => reject fault
      | .ok (value, root) =>
        match serialize shape value with
        | .ok again => if again == bytes then s!"ACCEPT {hexOf root}" else "MISMATCH"
        | .error fault => s!"MISMATCH {fault.reason}"
  | ["P", name, hex, idx] =>
    match lookup name, parseHex hex, idx.toNat? with
    | none, _, _ => s!"ERR unknown {name}"
    | _, none, _ => "ERR badhex"
    | _, _, none => "ERR badindex"
    | some shape, some bytes, some index =>
      match decodeFor shape bytes with
      | .error fault => reject fault
      | .ok (value, root) =>
        match nodeRoot shape value index, buildProof shape value index with
        | .error fault, _ | _, .error fault => reject fault
        | .ok leaf, .ok branch =>
          match verifyMerkleProof leaf branch index root with
          | .ok true => s!"PROOF {hexOf leaf} {hexList branch}"
          | .ok false => "MISMATCH-PROOF"
          | .error fault => s!"MISMATCH-PROOF {fault.reason}"
  | ["V", name, hex, idx, leafHex, branchHex] =>
    match lookup name, parseHex hex, idx.toNat?, parseHex leafHex, parseHexList branchHex with
    | some shape, some bytes, some index, some leaf, some branch =>
      match decodeFor shape bytes with
      | .error fault => reject fault
      | .ok (_, root) =>
        match verifyMerkleProof leaf branch index root with
        | .ok verdict => s!"VERIFY {verdict}"
        | .error fault => reject fault
    | _, _, _, _, _ => "ERR badline"
  | ["M", name, hex, idxs] =>
    match lookup name, parseHex hex, parseNats idxs with
    | some shape, some bytes, some indices =>
      match decodeFor shape bytes with
      | .error fault => reject fault
      | .ok (value, root) =>
        match indices.mapM (nodeRoot shape value), buildMultiproof shape value indices with
        | .error fault, _ | _, .error fault => reject fault
        | .ok leaves, .ok proof =>
          match verifyMerkleMultiproof leaves proof indices root with
          | .ok true => s!"MULTI {hexList leaves} {hexList proof}"
          | .ok false => "MISMATCH-MULTI"
          | .error fault => s!"MISMATCH-MULTI {fault.reason}"
    | _, _, _ => "ERR badline"
  | ["W", name, hex, idxs, leavesHex, proofHex] =>
    match lookup name, parseHex hex, parseNats idxs, parseHexList leavesHex, parseHexList proofHex with
    | some shape, some bytes, some indices, some leaves, some proof =>
      match decodeFor shape bytes with
      | .error fault => reject fault
      | .ok (_, root) =>
        match verifyMerkleMultiproof leaves proof indices root with
        | .ok verdict => s!"VERIFY {verdict}"
        | .error fault => reject fault
    | _, _, _, _, _ => "ERR badline"
  | _ => "ERR badline"

/-- Declare a type for the session: the answer and the registry it leaves. -/
def declare (declaredHere : List (String × Desc)) (name text : String) :
    String × List (String × Desc) :=
  match parseDesc text with
  | .error fault => (s!"ERR baddesc {fault}", declaredHere)
  | .ok shape =>
    match shape.wellFormed with
    | .ok () => ("OK", (name, shape) :: declaredHere)
    | .error fault => (s!"ILLFORMED {fault.reason}", declaredHere)

partial def loop (stdin : IO.FS.Stream) (stdout : IO.FS.Stream)
    (declaredHere : List (String × Desc)) : IO Unit := do
  let line ← stdin.getLine
  if line == "" then return
  let line := String.ofList ((line.toList.dropWhile Char.isWhitespace).reverse.dropWhile Char.isWhitespace).reverse
  if line == "" then
    loop stdin stdout declaredHere
  else
    match line.splitOn " " with
    | ["T", name, text] =>
      let (answer, declaredHere) := declare declaredHere name text
      stdout.putStrLn answer
      stdout.flush
      loop stdin stdout declaredHere
    | words =>
      stdout.putStrLn (handle declaredHere words)
      stdout.flush
      loop stdin stdout declaredHere

def main : IO Unit := do
  loop (← IO.getStdin) (← IO.getStdout) []
