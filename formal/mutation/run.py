#!/usr/bin/env python3
"""Mutation testing of the formal link.

Applies each mutant of mutants.json to a copy of the repository, runs the
suites it names, and reports whether the link kills it. A mutant must be
killed unless marked "equivalent" (then it must survive, so the catalogue
stays truthful). "optional" mutants whose code is no longer present are
skipped. Exit status 1 on any unexpected outcome.

Usage: python3 formal/mutation/run.py [mutant-id ...]
The Lean oracles must be built; their absolute paths are passed through
SSZ_ORACLE, SSZ_SPEC_ORACLE and HASHER_MODEL.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time

here = os.path.dirname(os.path.abspath(__file__))
repo = os.path.abspath(os.path.join(here, "..", ".."))
cfg = json.load(open(os.path.join(here, "mutants.json")))
only = set(sys.argv[1:])

env = dict(os.environ)
env.setdefault("SSZ_ORACLE", os.path.join(repo, "formal/lean/.lake/build/bin/ssz-oracle"))
env.setdefault("SSZ_SPEC_ORACLE", os.path.join(repo, "formal/spec/.lake/build/bin/ssz-spec-oracle"))
env.setdefault("HASHER_MODEL", os.path.join(repo, "formal/lean/.lake/build/bin/hasher-model"))
for k in ("SSZ_ORACLE", "SSZ_SPEC_ORACLE", "HASHER_MODEL"):
    if not os.path.exists(env[k]):
        sys.exit(f"{k}={env[k]} is not built")

def copy_tree(dst):
    ignore = shutil.ignore_patterns(".git", ".lake", "consensus-spec-tests", "node_modules", "*.test")
    shutil.copytree(repo, dst, ignore=ignore, symlinks=True)

def first_failure(out):
    for line in out.splitlines():
        if any(k in line for k in ("DIVERGENCE", "VIOLATION", "violation", "MISMATCH", "--- FAIL", "panic:")):
            return line.strip()[:160]
    return out.strip().splitlines()[-1][:160] if out.strip() else ""

bad = []
results = []

# Baseline: every suite must pass on the unmutated tree, or a kill means nothing.
with tempfile.TemporaryDirectory(prefix="mut-base-") as tmp:
    root = os.path.join(tmp, "repo")
    copy_tree(root)
    for name, cmd in cfg["suites"].items():
        r = subprocess.run(["bash", "-c", cmd], cwd=root, env=env,
                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        if r.returncode != 0:
            print(r.stdout[-3000:])
            sys.exit(f"baseline suite {name!r} fails on the unmutated tree")
        print(f"baseline {name}: ok", flush=True)

for m in cfg["mutants"]:
    if only and m["id"] not in only:
        continue
    with tempfile.TemporaryDirectory(prefix="mut-") as tmp:
        root = os.path.join(tmp, "repo")
        copy_tree(root)
        path = os.path.join(root, m["file"])
        src = open(path).read()
        n = src.count(m["old"])
        if n != 1:
            if m.get("optional") and n == 0:
                results.append((m["id"], "SKIPPED (code not present)"))
                continue
            bad.append(m["id"])
            results.append((m["id"], f"BROKEN: pattern occurs {n} times in {m['file']}"))
            continue
        open(path, "w").write(src.replace(m["old"], m["new"]))
        killed_by = []
        why = ""
        start = time.time()
        for s in m["suites"]:
            r = subprocess.run(["bash", "-c", cfg["suites"][s]], cwd=root, env=env,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            if r.returncode != 0:
                killed_by.append(s)
                why = why or first_failure(r.stdout)
        took = time.time() - start
        if m.get("equivalent"):
            outcome = "survived (equivalent, expected)" if not killed_by else f"KILLED by {killed_by} but marked equivalent"
            if killed_by:
                bad.append(m["id"])
        else:
            outcome = f"killed by {killed_by}: {why}" if killed_by else "SURVIVED"
            if not killed_by:
                bad.append(m["id"])
        results.append((m["id"], f"{outcome} ({took:.0f}s)"))
    print(f"{results[-1][0]}: {results[-1][1]}", flush=True)

print()
for rid, out in results:
    print(f"{rid:32s} {out}")
if bad:
    print(f"\nunexpected outcomes: {bad}")
    sys.exit(1)
print("\nall mutants behaved as catalogued")
