#!/usr/bin/env bash
# Helpers for comparing wall-clock benchmarks between two checkouts.
#
# Two builds of this repository place their functions at different addresses
# whenever anything above them changes size, and that shift alone moves tight
# benchmarks by several percent: the code is identical, but instruction
# delivery is not. Pinning function alignment does not fix it, because whole
# blocks still slide. Measuring every benchmark under several linker layouts
# and pooling the samples does: the placement a single build happened to get
# stops standing in for the change itself.
#
# The measurement budget is spread across the layouts rather than added to, so
# a comparison costs what it did when it used one.

# Layouts to measure under. Fixed values, so a rerun of the same commit pair
# measures the same thing.
BENCH_LAYOUT_SEEDS=${BENCH_LAYOUT_SEEDS:-"101 202 303 404"}
BENCH_LAYOUT_TIME=${BENCH_LAYOUT_TIME:-1s}

# run_layout_round runs one benchmark on both checkouts under every layout,
# appending to the two result files. Which side runs first alternates: run
# back to back under sustained load, the second run meets an already warmed,
# often frequency-throttled CPU, and always running the same side second bends
# every comparison the same way.
#
#   run_layout_round <bench> <packages> <module-subdir> <base-out> <pr-out> <rounds>
run_layout_round() {
    local bench=$1 packages=$2 subdir=$3 base_out=$4 pr_out=$5 rounds=$6
    local seed idx=0 base_dir="base" pr_dir="pr"

    if [ -n "${subdir}" ]; then
        base_dir="base/${subdir}"
        pr_dir="pr/${subdir}"
    fi

    for _ in $(seq 1 "${rounds}"); do
        for seed in ${BENCH_LAYOUT_SEEDS}; do
            local flags="-funcalign=64 -randlayout=${seed}"
            if [ $((idx % 2)) -eq 0 ]; then
                _bench_run "${base_dir}" "${bench}" "${packages}" "${flags}" >> "${base_out}" 2>&1 || true
                _bench_run "${pr_dir}" "${bench}" "${packages}" "${flags}" >> "${pr_out}" 2>&1
            else
                _bench_run "${pr_dir}" "${bench}" "${packages}" "${flags}" >> "${pr_out}" 2>&1
                _bench_run "${base_dir}" "${bench}" "${packages}" "${flags}" >> "${base_out}" 2>&1 || true
            fi
            idx=$((idx + 1))
        done
    done
}

_bench_run() {
    local dir=$1 bench=$2 packages=$3 flags=$4
    # shellcheck disable=SC2086 # packages is a deliberate word list, often empty
    (cd "${dir}" && go test -run='^$' -bench="^${bench}\$" -benchmem -count=1 \
        -benchtime="${BENCH_LAYOUT_TIME}" -timeout=15m -ldflags="${flags}" ${packages})
}

# flagged_benchmarks names the top-level benchmarks whose mean moved more than
# the threshold, which are the only ones worth more samples.
flagged_benchmarks() {
    local base_file=$1 pr_file=$2
    awk '
      FNR==NR && /^Benchmark.*ns\/op/ {
        for(i=2;i<=NF;i++) if($i=="ns/op"){base_sum[$1]+=$(i-1)+0; base_n[$1]++; break}; next
      }
      /^Benchmark.*ns\/op/ {
        for(i=2;i<=NF;i++) if($i=="ns/op"){pr_sum[$1]+=$(i-1)+0; pr_n[$1]++; break}
      }
      END {
        for(n in pr_sum){
          if(!(n in base_sum) || base_n[n]==0 || pr_n[n]==0) continue
          b=base_sum[n]/base_n[n]; p=pr_sum[n]/pr_n[n]
          if(b==0) continue
          d=(p-b)/b*100; if(d<0)d=-d
          if(d>5){t=n; s=index(t,"/"); if(s>0)t=substr(t,1,s-1); else sub(/-[0-9]+$/,"",t); print t}
        }
      }
    ' "${base_file}" "${pr_file}" | sort -u
}
