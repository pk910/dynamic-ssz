import SszCert
import SszCert.Certified
import SszCert.Eth

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

def handle (name : String) (hex : String) : String :=
  let bytes? := if hex == "-" then some [] else parseHex hex
  match bytes? with
  | none => "ERR badhex"
  | some bytes =>
    match (certified ++ certifiedEth).lookup name with
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

partial def loop (stdin : IO.FS.Stream) (stdout : IO.FS.Stream) : IO Unit := do
  let line ← stdin.getLine
  if line == "" then
    return
  let line := line.trim
  if line == "" then
    loop stdin stdout
  else
    match line.splitOn " " with
    | [name, hex] =>
      stdout.putStrLn (handle name hex)
      stdout.flush
      loop stdin stdout
    | _ =>
      stdout.putStrLn "ERR badline"
      stdout.flush
      loop stdin stdout

def main : IO Unit := do
  loop (← IO.getStdin) (← IO.getStdout)
