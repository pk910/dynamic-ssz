import SszCert.Hasher.Model

/-!
# hasher-model

Replays hasher API calls on the executable model and prints its state after
every call, for the per-operation comparison with the Go hasher
(`hasher/formal_trace_test.go`).

Chunks are represented by 64-bit fingerprints and hashed with a mixing
function the Go test installs as its hash backend (the model is generic in
the hash, so any function serves; this one keeps traces fast).

Input lines:

* `P <batch> <asyncMin> <cap> <capDepth> <active> <precollapseLevel> <ring>`
  — set thresholds and reset;
* `R` — reset;
* an operation: `st <none|binary|progressive> <packed 0|1>`, `ix`,
  `lf <hex64>`, `co <a>`, `mz <a>`, `mm <num> <limit> <a>`, `mp <a>`,
  `mpm <num> <a>`, `mpa <hex64> <a>` (`<a>`: async 0|1);
* `HR` — print the root (`ROOT <hex64>` or `NOROOT <reason>`).

Every operation answers one line: the rendered state.
-/

open Ssz.Hasher

def mix (a b : UInt64) : UInt64 :=
  let x := a * 0x9E3779B97F4A7C15 + b + 0x632BE59BD9B4E019
  let x := (x ^^^ (x >>> 30)) * 0xBF58476D1CE4E5B9
  let x := (x ^^^ (x >>> 27)) * 0x94D049BB133111EB
  x ^^^ (x >>> 31)

def hex64 (x : UInt64) : String :=
  let d := "0123456789abcdef".toList
  String.mk ((List.range 16).reverse.map (fun i => d[((x >>> (4 * i).toUInt64) &&& 15).toNat]!))

def parseHex64 (s : String) : UInt64 :=
  s.foldl (fun acc c =>
    let v := if '0' ≤ c ∧ c ≤ '9' then c.toNat - '0'.toNat
      else if 'a' ≤ c ∧ c ≤ 'f' then c.toNat - 'a'.toNat + 10 else 0
    acc * 16 + v.toUInt64) 0

def ctxOf (P : Params) : Ctx UInt64 :=
  { H := mix, z0 := 0, word := fun n => n.toUInt64, P := P }

def renderCell : Cell UInt64 → String
  | .ready v => hex64 v
  | .hole _ _ _ => "?"

def renderCells (cs : List (Cell UInt64)) : String :=
  "[" ++ ",".intercalate (cs.map renderCell) ++ "]"

def b2s (b : Bool) : String := if b then "1" else "0"

def renderLayer (l : Layer UInt64) : String :=
  let flags := b2s l.incremental ++ b2s l.legacy ++ b2s l.progressive ++ b2s l.packed ++ b2s l.collapsed
  let nodes := "[" ++ ",".intercalate (l.nodes.map (fun n => s!"{n.depth}:{renderCell n.cell}")) ++ "]"
  let counted := if l.collapsed then s!"k{l.counted}" else ""
  let prog := if l.progressive then s!"P{renderCells l.prog}v{l.level}" else ""
  let pend := if l.pend = [] then "" else
    s!"C{l.pendC}E[" ++ ",".intercalate (l.pend.map renderCells) ++ "]T" ++ renderCells l.tail
  s!"L\{{flags}}{prog}N{nodes}{counted}{pend}"

def render (s : St UInt64) : String :=
  let layers := " ".intercalate (s.stack.reverse.map renderLayer)
  let jobs := "[" ++ ",".intercalate (s.jobs.map (fun j => s!"{j.dst}+{j.out.length}")) ++ "]"
  let err := match s.err with | some e => s!" ERR {e}" | none => ""
  s!"B{renderCells s.base} {layers} J{jobs} H{b2s s.hashErr.isSome}{err}"

def parseOp (ws : List String) : Option (Op UInt64) :=
  let a (x : String) : Bool := x == "1"
  match ws with
  | ["st", t, p] =>
    let tt := if t == "none" then TreeType.none else if t == "binary" then .binary else .progressive
    some (.startTree tt (a p))
  | ["ix"] => some .index
  | ["lf", v] => some (.leaf (parseHex64 v))
  | ["co", x] => some (.collapse (a x))
  | ["mz", x] => some (.merkleize (a x))
  | ["mm", n, l, x] => some (.merkleizeMixin n.toNat! l.toNat! (a x))
  | ["mp", x] => some (.merkleizeProg (a x))
  | ["mpm", n, x] => some (.merkleizeProgMixin n.toNat! (a x))
  | ["mpa", v, x] => some (.merkleizeProgActive (parseHex64 v) (a x))
  | _ => none

partial def loop (stdin stdout : IO.FS.Stream) (P : Params) (s : St UInt64) : IO Unit := do
  let line ← stdin.getLine
  if line == "" then return
  let ws := (line.trimAscii.toString.splitOn " ").filter (· ≠ "")
  match ws with
  | [] => loop stdin stdout P s
  | "P" :: rest =>
    match rest.map String.toNat! with
    | [b, am, c, cd, ac, pl, r] =>
      loop stdin stdout { batch := b, asyncMin := am, cap := c, capDepth := cd, active := ac,
                          precollapseLevel := pl, ring := r } {}
    | _ => stdout.putStrLn "ERR params"; stdout.flush; loop stdin stdout P s
  | ["R"] => loop stdin stdout P {}
  | ["HR"] =>
    let out := match s.root with
      | some v => s!"ROOT {hex64 v}"
      | none =>
        let s' := s.drainAll
        s!"NOROOT err={s'.err} herr={s'.hashErr} layers={s'.stack.length} base={s'.base.length}"
    stdout.putStrLn out; stdout.flush; loop stdin stdout P s
  | _ =>
    match parseOp ws with
    | some op =>
      let s := s.step (ctxOf P) op
      stdout.putStrLn (render s); stdout.flush
      loop stdin stdout P s
    | none => stdout.putStrLn s!"ERR badop {line.trimAscii}"; stdout.flush; loop stdin stdout P s

def main : IO Unit := do
  loop (← IO.getStdin) (← IO.getStdout) {} {}
