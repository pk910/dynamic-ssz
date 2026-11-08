#!/usr/bin/env bash
# Prints the axioms every theorem of a built Lean module depends on and fails
# if any goes beyond Lean's three standard axioms (propext, Classical.choice,
# Quot.sound) — in particular `Lean.ofReduceBool` (native_decide) and `sorryAx`.
# Usage (from the package directory): ../audit-axioms.sh <Module> <Namespace>
set -u
module="${1:?module}"; ns="${2:?namespace}"
source="${module//.//}.lean"
[ -f "$source" ] || { echo "audit: no such module file: $source (run from the package directory)" >&2; exit 1; }
mapfile -t names < <(grep -o '^theorem [A-Za-z0-9_]*' "$source" | awk '{print $2}')
[ "${#names[@]}" -gt 0 ] || { echo "audit: no theorems found in $source" >&2; exit 1; }
{
  echo "import $module"
  for name in "${names[@]}"; do echo "#print axioms $ns.$name"; done
} > AxiomAudit.lean
trap 'rm -f AxiomAudit.lean' EXIT
out="$(lake env lean AxiomAudit.lean 2>&1)"; status=$?
# Long theorem names wrap the axiom list over several lines: join them.
out="$(printf '%s\n' "$out" | grep -v '^trace' | awk 'NR > 1 && /^ / { sub(/^ +/, " "); buf = buf $0; next } { if (NR > 1) print buf; buf = $0 } END { print buf }' || true)"
if [ "$status" -ne 0 ]; then
  echo "audit: lean failed (exit $status):" >&2; printf '%s\n' "$out" >&2; exit 1
fi
printf '%s\n' "$out" | sed 's/.*depends on axioms: //; s/.*does not depend on any axioms/[]/' | sort | uniq -c
standard='(depends on axioms: \[(propext|Classical\.choice|Quot\.sound)(, (propext|Classical\.choice|Quot\.sound))*\]|does not depend on any axioms)$'
audited="$(printf '%s\n' "$out" | grep -c -E 'depends on axioms|does not depend on any axioms' || true)"
if [ "$audited" -ne "${#names[@]}" ]; then
  echo "audit: expected ${#names[@]} theorems, lean reported $audited:" >&2; printf '%s\n' "$out" >&2; exit 1
fi
if printf '%s\n' "$out" | grep -v -E "$standard" | grep -q '.'; then
  echo "audit: non-standard axioms found:" >&2
  printf '%s\n' "$out" | grep -v -E "$standard" >&2
  exit 1
fi
echo "axiom audit passed: $audited theorems on standard axioms only"
