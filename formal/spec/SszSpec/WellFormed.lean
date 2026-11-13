import EthLib.Serialization.Ssz

/-! Evaluating well-formedness: a Boolean form of `Desc.wellFormed = .ok ()` that
    `decide` can discharge by evaluation, with no instance beyond `Bool`. -/

namespace SszSpec

open EthLib.Serialization.Ssz

/-- Whether a check succeeded. -/
def isOk : Except Err Unit → Bool
  | .ok () => true
  | .error _ => false

/-- A successful Boolean check is the equation EthLib's theorems take. -/
theorem ok_of_isOk {check : Except Err Unit} (passed : isOk check = true) : check = .ok () := by
  cases check with
  | ok u => cases u; rfl
  | error _ => simp [isOk] at passed

end SszSpec

/-! ## `ssz_wf`

The one place that knows how well-formedness is evaluated. Most declarations
are closed by `decide` (kernel evaluation of `Desc.wellFormed`). Compatible
unions reach EthLib's union-compatibility check, which is well-founded
recursion and does not reduce in the kernel; it is evaluated through its
equations with `simp +decide`. An EthLib refactor of those definitions is
absorbed here, not in the generated certificates. -/

namespace SszSpec
open EthLib.Serialization.Ssz

/-- Prove `D.wellFormed = .ok ()` for the declaration `D` by evaluation. -/
macro "ssz_wf " d:ident : tactic => `(tactic|
  exact ok_of_isOk (by
    first
    | (set_option maxRecDepth 100000 in decide)
    | (set_option maxRecDepth 100000 in
        simp +decide [$d:ident, isOk, Desc.wellFormed, Desc.allWellFormed, Desc.optionsCompatible,
          isCompatible, compatibleAt, layoutsAgree, layoutPairAgree, placedFields, placedOrdinals,
          Desc.nesting, Desc.deepestNesting, Desc.byteSequence, firstDuplicate, maxActiveFields,
          Bind.bind, Except.bind, pure, Except.pure, List.find?, List.range, List.range.loop,
          List.getElem!_eq_getElem?_getD, List.getElem?_cons_zero, List.getElem?_cons_succ,
          List.getD, List.headD])))

end SszSpec
