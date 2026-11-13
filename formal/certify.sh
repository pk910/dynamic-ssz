#!/usr/bin/env bash
# Regenerates everything derived from the harness types: their dynssz-gen SSZ
# methods and the Lean certificates (in-house model) and EthLib declarations +
# theorem instances (official specification). CI runs this and requires a
# clean diff, so the checked certificates are exactly what the code generator
# sees.
set -euo pipefail
cd "$(dirname "$0")"

# The certified harness types, one per line (harness/certified-types.txt).
TYPES="$(grep -v '^\s*\(#\|$\)' harness/certified-types.txt | paste -sd,)"
[ -n "$TYPES" ] || { echo "no certified types listed in harness/certified-types.txt" >&2; exit 1; }

cd harness
# dynssz-gen reads its previous output when analysing the package, so one run
# is not always its own fixed point (see formal/README.md, Findings);
# regenerate until the output stops changing.
for _ in 1 2 3 4 5 6 7 8; do
  before="$(sha256sum generated_ssz.go 2>/dev/null || true)"
  go run github.com/pk910/dynamic-ssz/dynssz-gen -with-extended-types -package . -types "$TYPES" -output generated_ssz.go
  [ "$before" = "$(sha256sum generated_ssz.go)" ] && break
done
go run github.com/pk910/dynamic-ssz/formal/certify -package . -types "$TYPES" \
  -output ../lean/SszCert/Certified.lean \
  -spec-output ../spec/SszSpec/Certified.lean \
  -spec-theorems ../spec/SszSpec/Theorems.lean
