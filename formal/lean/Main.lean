import SszCert
import SszCert.Certified

/-!
# ssz-oracle

Executable decision oracle over the certified codecs. Reads lines of the form
`<typeName> <hex|->` from stdin (`-` denotes the empty byte string) and
answers per line:

* `ACCEPT` — the certified strict decoder accepts the bytes (and the
  re-serialization self-check reproduced the input byte-for-byte, as the
  canonicity theorem guarantees; a `MISMATCH` answer would indicate a
  compiler/runtime defect, since the theorem rules it out for the model);
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

def handle (name : String) (hex : String) : String :=
  let bytes? := if hex == "-" then some [] else parseHex hex
  match bytes? with
  | none => "ERR badhex"
  | some bytes =>
    match certified.lookup name with
    | none => s!"ERR unknown {name}"
    | some c =>
      match c.dec bytes with
      | some v => if c.ser v == bytes then "ACCEPT" else "MISMATCH"
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
