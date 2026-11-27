import SszCert
import SszCert.Certified
import SszCert.Eth
import SszCert.Declare

/-!
# ssz-oracle

Executable decision oracle over the certified codecs. Reads lines of the form
`<typeName> <hex|->` from stdin (`-` denotes the empty byte string) and
answers per line:

* `ACCEPT <root>` — the certified strict decoder accepts the bytes; `<root>`
  is the hash tree root of the decoded value. Two self-checks run on every
  accept — the re-serialization must reproduce the input byte-for-byte, and
  the explicit Merkle tree must hash to the direct root — both guaranteed by
  the theorems, so a `MISMATCH`/`MISMATCH-TREE` answer would indicate a
  compiler/runtime defect, not a model question;
* `REJECT` — the certified strict decoder rejects the bytes;
* `T <name> <json>` — declare a type for this session (`SszCert.Declare`:
  the ssz-specs JSON spelling plus dynamic-ssz's extended kinds), answered
  `OK`; its codec is built from the certified combinators, so it carries the
  theorems. A declared name shadows a compiled one.
* `ERR ...` — malformed input line or unknown type name.
-/

open Ssz

def hexDigit (c : Char) : Option Nat :=
  if '0' ≤ c ∧ c ≤ '9' then some (c.toNat - '0'.toNat)
  else if 'a' ≤ c ∧ c ≤ 'f' then some (c.toNat - 'a'.toNat + 10)
  else if 'A' ≤ c ∧ c ≤ 'F' then some (c.toNat - 'A'.toNat + 10)
  else none

def parseHex (s : String) : Option (List UInt8) :=
  go s.toList []
where
  go : List Char → List UInt8 → Option (List UInt8)
  | [], acc => some acc.reverse
  | c1 :: c2 :: rest, acc =>
    match hexDigit c1, hexDigit c2 with
    | some h, some l => go rest (UInt8.ofNat (h * 16 + l) :: acc)
    | _, _ => none
  | _, _ => none

def hexOf (bs : List UInt8) : String :=
  bs.foldl (fun s b =>
    let d := "0123456789abcdef".toList
    (s.push d[b.toNat / 16]!).push d[b.toNat % 16]!) ""

def handle (declaredHere : List (String × Codec)) (name : String) (hex : String) : String :=
  let bytes? := if hex == "-" then some [] else parseHex hex
  match bytes? with
  | none => "ERR badhex"
  | some bytes =>
    match (declaredHere.lookup name).orElse fun _ => (certified ++ certifiedEth).lookup name with
    | none => s!"ERR unknown {name}"
    | some c =>
      match c.dec bytes with
      | some v =>
        if c.ser v == bytes then
          let root := c.htr v
          if (c.tree v).root == root then
            s!"ACCEPT {hexOf root}"
          else
            "MISMATCH-TREE"
        else
          "MISMATCH"
      | none => "REJECT"

partial def loop (stdin : IO.FS.Stream) (stdout : IO.FS.Stream)
    (declaredHere : List (String × Codec)) : IO Unit := do
  let line ← stdin.getLine
  if line == "" then
    return
  let line := line.trim
  if line == "" then
    loop stdin stdout declaredHere
  else
    match line.splitOn " " with
    | ["T", name, text] =>
      match parseShape text with
      | .error fault =>
        stdout.putStrLn s!"ERR baddesc {fault}"
        stdout.flush
        loop stdin stdout declaredHere
      | .ok shape =>
        stdout.putStrLn "OK"
        stdout.flush
        loop stdin stdout ((name, shape.codec) :: declaredHere)
    | [name, hex] =>
      stdout.putStrLn (handle declaredHere name hex)
      stdout.flush
      loop stdin stdout declaredHere
    | _ =>
      stdout.putStrLn "ERR badline"
      stdout.flush
      loop stdin stdout declaredHere

def main : IO Unit := do
  loop (← IO.getStdin) (← IO.getStdout) []
