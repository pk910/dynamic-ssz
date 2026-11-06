import EthLib.Consensus.Ssz

/-! Evaluating well-formedness: a Boolean form of `Desc.wellFormed = .ok ()` that
    `decide` can discharge by evaluation, with no instance beyond `Bool`. -/

namespace SszSpec

open EthLib.Consensus.Ssz

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
