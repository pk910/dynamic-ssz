#!/usr/bin/env bash
# Counts instructions retired per benchmark operation for two checkouts.
#
# Wall time is not comparable between two builds of this repository: a change
# in one package's code size moves every function above it, and the resulting
# shift in op-cache and branch-predictor indices moves tight benchmarks by
# several percent while the executed instructions stay identical. Instruction
# counts do not move with placement, so they answer "did this change the work"
# where time answers "did this change the work or the addresses".
#
# Per-operation counts come from the difference between two iteration counts,
# which cancels process start-up, fixture loading and benchmark set-up exactly.
#
# Usage:
#   bench-instructions.sh probe
#   bench-instructions.sh run <base-root> <pr-root> <module-subdir> <out.tsv>

set -uo pipefail

# How long the measured half should run. The fixed cost of a run -- process
# start-up, loading the fixtures, building the type caches -- is cancelled by
# subtracting a short run from a long one, but only its mean is: what is left
# is its run-to-run variance. Measuring for a couple of seconds puts the
# benchmark's own work far above that variance even for microsecond-scale
# benchmarks, where a fixed iteration count would drown in it.
LONG_NANOS=${BENCH_INSTR_LONG_NS:-1500000000}
SHORT_DIVISOR=10
# Each count is a median of this many runs. The subtraction cancels the mean
# of the fixed cost but not its spread, and the garbage collector does not
# schedule itself identically from one run to the next; the median keeps a
# single unlucky collection out of the result.
REPEATS=${BENCH_INSTR_REPEATS:-3}

# probe reports whether this machine can count instructions at all. A
# virtualised runner usually hides the PMU, in which case the caller falls back
# to the time-based comparison alone.
probe() {
    if ! command -v perf >/dev/null 2>&1; then
        echo "perf is not installed"
        return 1
    fi
    sudo sysctl -w kernel.perf_event_paranoid=1 >/dev/null 2>&1 || true
    local out
    out=$(perf stat -x, -e instructions /bin/true 2>&1)
    local value
    value=$(awk -F, '$3=="instructions"{print $1}' <<<"$out")
    case "${value}" in
        ''|*'not supported'*|*'not counted'*)
            echo "the PMU does not expose instruction counts on this machine"
            return 1
            ;;
    esac
    echo "instruction counting available"
    return 0
}

# count runs one benchmark for a fixed number of iterations and reports the
# instructions the whole process retired.
count() {
    local bin=$1 bench=$2 iters=$3
    perf stat -x, -e instructions "${bin}" \
        -test.run '^$' -test.bench "${bench}" -test.benchtime="${iters}x" 2>&1 |
        awk -F, '$3=="instructions"{print $1}'
}

# nsPerOp times one benchmark briefly, to size the counted runs.
nsPerOp() {
    local bin=$1 bench=$2
    "${bin}" -test.run '^$' -test.bench "${bench}" -test.benchtime=200ms 2>/dev/null |
        awk '/^Benchmark/ { for (i = 2; i <= NF; i++) if ($i == "ns/op") { print $(i-1); exit } }'
}

# perOp isolates the cost of the benchmark body from everything the process
# does once.
perOp() {
    local bin=$1 bench=$2
    local ns hi_iters lo_iters lo hi

    ns=$(nsPerOp "${bin}" "${bench}")
    case "${ns}" in ''|0) return 1 ;; esac

    hi_iters=$(awk -v ns="${ns}" -v target="${LONG_NANOS}" \
        'BEGIN { n = int(target / ns); if (n < 10) n = 10; print n }')
    lo_iters=$(awk -v hi="${hi_iters}" -v d="${SHORT_DIVISOR}" \
        'BEGIN { n = int(hi / d); if (n < 2) n = 2; print n }')
    [ "${lo_iters}" -ge "${hi_iters}" ] && return 1

    # Estimate the per-operation cost several times over independent pairs.
    # Subtracting a short run from a long one assumes the fixed cost does not
    # depend on the iteration count; for a benchmark that allocates heavily
    # enough to change how often the collector runs, that assumption fails and
    # the estimates disagree. Reporting how far they spread lets the caller
    # tell a number it can compare from one it cannot, instead of the tool
    # pretending every benchmark suits the method.
    local estimates=()
    for _ in $(seq 1 "${REPEATS}"); do
        lo=$(count "${bin}" "${bench}" "${lo_iters}")
        hi=$(count "${bin}" "${bench}" "${hi_iters}")
        [ -z "${lo}" ] || [ -z "${hi}" ] && continue
        # Printed as a plain integer: awk renders large values in scientific
        # notation, which sort -n reads as the mantissa alone and orders wrongly.
        estimates+=("$(awk -v lo="${lo}" -v hi="${hi}" -v n="$((hi_iters - lo_iters))" \
            'BEGIN { printf "%.0f\n", (hi - lo) / n }')")
    done
    [ "${#estimates[@]}" -eq 0 ] && return 1

    printf '%s\n' "${estimates[@]}" | sort -n | awk '
        { v[NR] = $1 }
        END {
            if (!NR) exit 1
            median = v[int((NR + 1) / 2)]
            if (median <= 0) exit 1
            spread = NR > 1 ? (v[NR] - v[1]) / median * 100 : 0
            printf "%.0f\t%.2f\n", median, spread
        }'
}

# benchPattern turns a reported benchmark name into the -test.bench pattern
# that selects exactly it, anchoring every path segment so a name that is a
# prefix of another one cannot pull its sibling in.
benchPattern() {
    local name=$1 pattern='' segment
    local IFS='/'
    for segment in ${name}; do
        pattern="${pattern}${pattern:+/}^${segment}\$"
    done
    printf '%s\n' "${pattern}"
}

# subBenchmarks lists the leaf benchmarks a binary reports, so a regression in
# one of them is not diluted by its siblings. BENCH_INSTR_FILTER narrows the
# list to those matching a regular expression, which is how a single family is
# re-measured without paying for the rest.
subBenchmarks() {
    local bin=$1
    "${bin}" -test.run '^$' -test.bench '.' -test.benchtime=1x 2>/dev/null |
        awk '/^Benchmark/ { sub(/-[0-9]+$/, "", $1); print $1 }' | sort -u |
        grep -E "${BENCH_INSTR_FILTER:-.}"
}

run() {
    local base_root=$1 pr_root=$2 subdir=$3 out=$4
    local base_dir="${base_root}/${subdir}" pr_dir="${pr_root}/${subdir}"
    base_dir=${base_dir%/} pr_dir=${pr_dir%/}

    local pkgs
    pkgs=$(cd "${pr_dir}" && git grep -l '^[[:space:]]*func Benchmark' -- '*_test.go' 2>/dev/null |
        xargs -r -n1 dirname | sort -u)
    [ -z "${pkgs}" ] && pkgs="."

    for pkg in ${pkgs}; do
        [ -d "${base_dir}/${pkg}" ] || continue

        local base_bin="${RUNNER_TEMP:-/tmp}/instr-base.test"
        local pr_bin="${RUNNER_TEMP:-/tmp}/instr-pr.test"
        (cd "${base_dir}/${pkg}" && go test -c -o "${base_bin}" -ldflags=-funcalign=64 .) >/dev/null 2>&1 || continue
        (cd "${pr_dir}/${pkg}" && go test -c -o "${pr_bin}" -ldflags=-funcalign=64 .) >/dev/null 2>&1 || continue

        local bench
        for bench in $(cd "${pr_dir}/${pkg}" && subBenchmarks "${pr_bin}"); do
            # The benchmark runs in its own package directory: the fixtures are
            # resolved relative to it.
            local base_val pr_val pattern
            pattern=$(benchPattern "${bench}")
            base_val=$(cd "${base_dir}/${pkg}" && perOp "${base_bin}" "${pattern}") || continue
            pr_val=$(cd "${pr_dir}/${pkg}" && perOp "${pr_bin}" "${pattern}") || continue

            local base_count base_spread pr_count pr_spread
            IFS=$'\t' read -r base_count base_spread <<< "${base_val}"
            IFS=$'\t' read -r pr_count pr_spread <<< "${pr_val}"
            printf '%s\t%s\t%s\t%s\t%s\n' \
                "${bench}" "${base_count}" "${pr_count}" "${base_spread}" "${pr_spread}" >> "${out}"
        done
    done
}

case "${1:-}" in
    probe) probe ;;
    run) shift; run "$@" ;;
    *) echo "usage: $0 probe | run <base-root> <pr-root> <module-subdir> <out.tsv>" >&2; exit 2 ;;
esac
