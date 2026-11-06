#!/usr/bin/env bash
# Prints the axioms every theorem of a built Lean module depends on and fails
# if any goes beyond Lean's three standard axioms (propext, Classical.choice,
# Quot.sound) — in particular `Lean.ofReduceBool` (native_decide) and `sorryAx`.
# Usage (from the package directory): ../audit-axioms.sh <Module> <Namespace>
set -euo pipefail
module="$1"; ns="$2"
file="$(grep -o '^theorem [A-Za-z0-9_]*' "${module//.//}.lean" | awk '{print $2}')"
{
  echo "import $module"
  for name in $file; do echo "#print axioms $ns.$name"; done
} > AxiomAudit.lean
trap 'rm -f AxiomAudit.lean' EXIT
out="$(lake env lean AxiomAudit.lean 2>&1 | grep -v '^trace')"
echo "$out" | sed 's/.*depends on axioms: //' | sort | uniq -c
if echo "$out" | grep -v -E "depends on axioms: \[(propext|Classical\.choice|Quot\.sound)(, (propext|Classical\.choice|Quot\.sound))*\]$" | grep -q "."; then
  echo "non-standard axioms or errors found:" >&2
  echo "$out" | grep -v -E "depends on axioms: \[(propext|Classical\.choice|Quot\.sound)(, (propext|Classical\.choice|Quot\.sound))*\]$" >&2
  exit 1
fi
echo "axiom audit passed: $(echo "$out" | grep -c 'depends on axioms') theorems on standard axioms only"
