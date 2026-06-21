#!/usr/bin/env bash
# Interleaved, CPU-pinned benchmark runner with reference-ratio normalization.
#
# Shared GitHub-hosted runners suffer CPU steal, host-level frequency scaling and
# noisy neighbours, which produce spurious +-10% wall-clock swings between
# otherwise-identical base/PR runs. This script reduces that noise without any
# dedicated hardware by combining several techniques:
#
#   * taskset pins every run to a single CPU   -> no scheduler-migration jitter
#   * GOMAXPROCS=1                              -> no GC-goroutine / parallelism noise
#   * setarch -R disables ASLR                  -> stable code/data memory layout
#   * base and PR are interleaved per sample    -> slow machine drift is common-mode
#                                                  and cancels in the comparison
#   * BenchmarkReference ratio normalization    -> cancels machine-wide multiplicative
#                                                  noise (steal, frequency scaling):
#                                                  every timing is divided by a fixed
#                                                  reference workload measured in the
#                                                  same process, so an N%-slower host
#                                                  scales both numbers equally.
#
# It emits two benchstat-parseable outputs per side:
#   <side>-<prefix>-raw.txt   raw `go test -benchmem` output (ns/op, B/op, allocs/op)
#   <side>-<prefix>-norm.txt  ns/op divided by the per-process BenchmarkReference
#                             ns/op, expressed as a dimensionless `ratio` (cost in
#                             reference-workload units); reference rows are dropped.
#
# Usage:
#   run-benchmarks.sh <base_dir> <pr_dir> <test_pkg> <out_prefix> <bench1> [bench2 ...]
#
# Tunables via env: BENCH_COUNT (samples/side, default 10), BENCH_TIME
# (per-sample -benchtime, default 1s), BENCH_TIMEOUT (default 10m),
# BENCH_OUTDIR (default $GITHUB_WORKSPACE/bench-results).

set -uo pipefail

if [ "$#" -lt 5 ]; then
  echo "usage: $0 <base_dir> <pr_dir> <test_pkg> <out_prefix> <bench...>" >&2
  exit 2
fi

BASE_DIR=$1
PR_DIR=$2
TEST_PKG=$3
OUT_PREFIX=$4
shift 4
BENCHMARKS=("$@")

OUTDIR="${BENCH_OUTDIR:-$GITHUB_WORKSPACE/bench-results}"
COUNT="${BENCH_COUNT:-10}"
BENCHTIME="${BENCH_TIME:-1s}"
TIMEOUT="${BENCH_TIMEOUT:-10m}"
mkdir -p "$OUTDIR"

# Pin to the highest-numbered CPU: lower cores tend to service more device IRQs.
LASTCPU=$(($(nproc) - 1))
[ "$LASTCPU" -lt 0 ] && LASTCPU=0
PIN=(taskset -c "$LASTCPU")

# Disable ASLR when setarch is available; fall back gracefully if it is not.
NORAND=()
if setarch "$(uname -m)" -R true 2>/dev/null; then
  NORAND=(setarch "$(uname -m)" -R)
fi

echo "Runner config: cpu=$LASTCPU gomaxprocs=1 aslr_off=$([ ${#NORAND[@]} -gt 0 ] && echo yes || echo no) count=$COUNT benchtime=$BENCHTIME pkg=$TEST_PKG"

# normalize divides every benchmark's ns/op by the BenchmarkReference ns/op found
# in the SAME process output and prints benchstat-parseable lines carrying a
# dimensionless `ratio` unit (cost in reference-workload units). A non-time unit
# keeps benchstat from rescaling the value as seconds. If no reference is present
# in the block (e.g. a package without a reference benchmark) it emits nothing
# rather than unnormalized numbers.
normalize() {
  awk '
    /[ \t]ns\/op([ \t]|$)/ {
      name[++n]=$1; iters[n]=$2
      for (i = 2; i <= NF; i++) if ($i == "ns/op") { ns[n]=$(i-1)+0; break }
      if ($1 ~ /^BenchmarkReference/) ref=ns[n]
    }
    END {
      if (ref == "" || ref <= 0) exit
      for (k = 1; k <= n; k++) {
        if (name[k] ~ /^BenchmarkReference/) continue
        printf "%s %s %.6f ratio\n", name[k], iters[k], ns[k] / ref
      }
    }
  ' "$1"
}

# run_side runs one (side, benchmark) sample together with BenchmarkReference in a
# single pinned process, appending raw and normalized lines to the side's files.
run_side() {
  local dir=$1 side=$2 bench=$3 tmp
  tmp=$(mktemp)
  ( cd "$dir" && "${PIN[@]}" "${NORAND[@]}" env GOMAXPROCS=1 \
      go test -run='^$' -bench="^(${bench}|BenchmarkReference)\$" \
        -benchmem -count=1 -benchtime="$BENCHTIME" -timeout="$TIMEOUT" "$TEST_PKG" ) >"$tmp" 2>&1 || true
  cat "$tmp" >>"$OUTDIR/${side}-${OUT_PREFIX}-raw.txt"
  normalize "$tmp" >>"$OUTDIR/${side}-${OUT_PREFIX}-norm.txt"
  rm -f "$tmp"
}

for bench in "${BENCHMARKS[@]}"; do
  echo "::group::${OUT_PREFIX}: ${bench} (${COUNT}x interleaved)"
  for i in $(seq 1 "$COUNT"); do
    # Alternate which side runs first each round so any residual ordering bias
    # (e.g. the second run hitting an already-warmed, throttled CPU) cancels.
    if [ $((i % 2)) -eq 1 ]; then
      run_side "$BASE_DIR" base "$bench"
      run_side "$PR_DIR"   pr   "$bench"
    else
      run_side "$PR_DIR"   pr   "$bench"
      run_side "$BASE_DIR" base "$bench"
    fi
  done
  echo "::endgroup::"
done
