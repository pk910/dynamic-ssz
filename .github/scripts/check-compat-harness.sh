#!/bin/sh
# Runs a freshly packed codegen harness the way CI runs every compat archive:
# from a directory named after a version, with no generated code regenerated.
# The harness must pass there, and it must stay cheap, because an archive is
# run on every CI job of every later release.
#
# Usage: check-compat-harness.sh <archive.tar.gz> <version>
#
# The archive is unpacked into codegen/compat-tests/<version>, tested, and the
# directory removed again. COMPAT_HARNESS_BUDGET overrides the 10 second
# budget on the package's own test time (compilation excluded).

set -e

archive=$1
version=$2
if [ -z "$archive" ] || [ -z "$version" ]; then
    echo "Usage: $0 <archive.tar.gz> <version>"
    exit 1
fi
budget=${COMPAT_HARNESS_BUDGET:-10}

dest="codegen/compat-tests/$version"
rm -rf "$dest"
mkdir -p "$dest"
tar -xzf "$archive" -C "$dest"
rm -f "$dest/generate.go"
trap 'rm -rf "$dest"' EXIT

echo "Running the packed harness from $dest"
report=$(mktemp)
if ! go test -count=1 -json "./$dest" > "$report"; then
    grep -E '"Action":"(fail|output)"' "$report" | jq -r 'select(.Output != null) | .Output' | tail -n 80
    echo "Error: the packed harness fails in $dest"
    rm -f "$report"
    exit 1
fi

elapsed=$(jq -r 'select(.Action == "pass" and .Test == null) | .Elapsed' "$report")
rm -f "$report"
echo "Packed harness passed in ${elapsed}s (budget ${budget}s)"
if awk -v elapsed="$elapsed" -v budget="$budget" 'BEGIN { exit !(elapsed > budget) }'; then
    echo "Error: the packed harness exceeds its ${budget}s budget; every later release runs it on every CI job"
    exit 1
fi
