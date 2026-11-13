#!/usr/bin/env bash
# Generates the type corpus of the generic link (formal/harness/corpus) and
# its dynssz-gen methods, then runs the corpus tests. Stages: 1 every leaf
# shape as a root, 2 every child class under every parent, 3 the inherited
# properties through a middle type, random shapes from the grammar.
#   formal/corpus.sh [stages] [seed]        e.g. formal/corpus.sh 1,2,3,deleg,rec,views,random 7
set -euo pipefail
cd "$(dirname "$0")/harness"
STAGES="${1:-1,2,3,deleg,rec,views,random}"
SEED="${2:-1}"
go run ./cmd/gen-corpus -out corpus -stages "$STAGES" -seed "$SEED" -random "${CORPUS_RANDOM:-50}" -depth "${CORPUS_DEPTH:-4}"
# A stale generated file from another corpus would not compile; start from the types alone.
rm -f corpus/generated_ssz.go corpus/generated_check.go
TYPES="$(paste -sd, corpus/types.txt)"
# dynssz-gen reads its previous output; iterate to the fixed point.
for _ in 1 2 3 4 5 6 7 8; do
  before="$(sha256sum corpus/generated_ssz.go 2>/dev/null || true)"
  go run github.com/pk910/dynamic-ssz/dynssz-gen -with-extended-types -package ./corpus -types "$TYPES" -output corpus/generated_ssz.go
  [ "$before" = "$(sha256sum corpus/generated_ssz.go)" ] && break
done
go vet ./corpus/
go test -count=1 -timeout 120m ${CORPUS_TEST_FLAGS:-} -run 'TestCorpusDeclarations|TestCorpusBuildersAgree|TestCorpusLink|TestCorpusRecursive|TestCorpusMisuse|TestCorpusViews|TestCorpusTreeBuilders' . | grep -v -e '^=== ' -e '^    --- PASS'
