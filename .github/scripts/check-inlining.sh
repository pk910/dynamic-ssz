#!/bin/bash
# Verifies that functions marked //ssz:mustinline still fit the compiler's
# inlining budget.
#
# Some of the hottest helpers in this library sit within a few cost units of the
# budget, and the shape of the code is what keeps them there -- a call node alone
# costs most of it. An innocuous-looking refactor can silently push one over,
# turning an inlined comparison into a function call on a path that runs per
# field or per element. That has happened: moving a bound check into a helper
# cost ~15% on streaming decode before it was caught by benchmarking.
#
# Mark such a function with //ssz:mustinline on the line above its doc comment
# or declaration, and this check will fail if it ever stops inlining.
#
# Usage: .github/scripts/check-inlining.sh [packages...]

set -uo pipefail

PACKAGES=("$@")
if [ ${#PACKAGES[@]} -eq 0 ]; then
    PACKAGES=("./...")
fi

MARKER='//ssz:mustinline'

# Collect the marked functions: after a marker, the next func declaration.
mapfile -t MARKED < <(
    grep -rl --include='*.go' -F "${MARKER}" . 2>/dev/null | while read -r file; do
        awk -v marker="${MARKER}" '
            index($0, marker) == 1 { want = 1; next }
            want && /^func / {
                line = $0
                if (match(line, /^func \([^)]*\) [A-Za-z0-9_]+/)) {
                    name = substr(line, RSTART, RLENGTH)
                    sub(/^func \([^)]*\) /, "", name)
                } else if (match(line, /^func [A-Za-z0-9_]+/)) {
                    name = substr(line, RSTART, RLENGTH)
                    sub(/^func /, "", name)
                } else {
                    name = ""
                }
                if (name != "") { print name }
                want = 0
            }
        ' "${file}"
    done | sort -u
)

if [ ${#MARKED[@]} -eq 0 ]; then
    echo "no ${MARKER} markers found; nothing to check"
    exit 0
fi

echo "Checking ${#MARKED[@]} function(s) marked ${MARKER}"

INLINE_LOG=$(mktemp)
trap 'rm -f "${INLINE_LOG}"' EXIT
go build -gcflags='-m=2' "${PACKAGES[@]}" 2>&1 >/dev/null | grep -E '(can|cannot) inline' > "${INLINE_LOG}"

rc=0
for fn in "${MARKED[@]}"; do
    # The compiler prints "can inline T.Name with cost N" or
    # "cannot inline T.Name: function too complex: cost N exceeds budget M".
    # A generic function appears once per instantiation, as Name[shape]; every
    # instantiation must inline, since a single stubborn one is a call on some
    # real call path.
    pattern="inline ([A-Za-z0-9_./*()]+\.)?${fn}(\[[^]]*\])?[ :]"
    matches=$(grep -E "${pattern}" "${INLINE_LOG}")

    if [ -z "${matches}" ]; then
        echo "  FAILED   ${fn}: no inlining decision reported (renamed or removed?)"
        rc=1
        continue
    fi

    if failed=$(echo "${matches}" | grep -E "cannot inline" | head -2); [ -n "${failed}" ]; then
        echo "  FAILED   ${fn}:"
        echo "${failed}" | sed 's/^/             /'
        rc=1
        continue
    fi

    cost=$(echo "${matches}" | grep -oE 'with cost [0-9]+' | grep -oE '[0-9]+' | sort -n | tail -1)
    count=$(echo "${matches}" | grep -cE 'can inline')
    if [ "${count}" -gt 1 ]; then
        echo "  ok       ${fn} (${count} instantiations, max cost ${cost})"
    else
        echo "  ok       ${fn} (cost ${cost})"
    fi
done

if [ "${rc}" -ne 0 ]; then
    cat <<'EOF'

A function marked //ssz:mustinline no longer inlines.

These sit close to the budget on purpose. The usual causes are introducing a
call (a call node costs most of the budget by itself), adding a branch, or
replacing a field read with something the compiler cannot fold. Keep the cold
work -- growth, error construction, refills -- in a separate out-of-line
function and leave only the fast-path test inline.

If the function genuinely no longer needs to be inlined, remove the marker in
the same change, and say why.
EOF
fi

exit "${rc}"
