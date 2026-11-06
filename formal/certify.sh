#!/usr/bin/env bash
# Regenerates everything derived from the harness types: their dynssz-gen SSZ
# methods and the Lean certificates (in-house model) and EthLib declarations +
# theorem instances (official specification). CI runs this and requires a
# clean diff, so the checked certificates are exactly what the code generator
# sees.
set -euo pipefail
cd "$(dirname "$0")"

TYPES="$(grep -o '^def C_[A-Za-z0-9_]* ' lean/SszCert/Certified.lean | sed 's/def C_//; s/ $//' | paste -sd,)"
[ -n "$TYPES" ] || { echo "no certified types found in lean/SszCert/Certified.lean" >&2; exit 1; }

cd harness
go run github.com/pk910/dynamic-ssz/dynssz-gen -package . -types "$TYPES" -output generated_ssz.go
go run github.com/pk910/dynamic-ssz/formal/certify -package . -types "$TYPES" \
  -output ../lean/SszCert/Certified.lean \
  -spec-output ../spec/SszSpec/Certified.lean \
  -spec-theorems ../spec/SszSpec/Theorems.lean
