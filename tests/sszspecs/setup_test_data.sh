#!/usr/bin/env bash
set -euo pipefail

# The official vectors are generated from the upstream reference
# implementation. Upstream has released no archive that carries the proof
# vectors yet, so a commit of its main branch is pinned here and the vectors
# are generated from it. The digest is that of the generated index.json, which
# in turn states the digest of every vector, so it pins the whole set.
readonly REPOSITORY="https://github.com/ethereum/ssz-specs"
readonly COMMIT="f6935dced26085f3211a08fa94a16003f30f72d7"
readonly INDEX_SHA256="ff27ce9f60862330f468b1ed16a6d48717445d1fe53be922ef3d88181c2d7d6b"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly DATA_DIR="${SCRIPT_DIR}/fixtures"

# fetch generates the pinned vectors into DATA_DIR unless they are there.
fetch() {
  if [[ -f "${DATA_DIR}/.version" ]] && [[ "$(<"${DATA_DIR}/.version")" == "${COMMIT}" ]]; then
    echo "Official SSZ vectors ${COMMIT} already installed"
    return
  fi

  command -v uv >/dev/null || { echo "uv is required to generate the vectors: https://docs.astral.sh/uv/" >&2; exit 1; }

  # Not local: the exit trap runs after this function has returned.
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "${tmp_dir}"' EXIT

  git -C "${tmp_dir}" init --quiet ssz-specs
  git -C "${tmp_dir}/ssz-specs" fetch --quiet --depth 1 "${REPOSITORY}" "${COMMIT}"
  git -C "${tmp_dir}/ssz-specs" checkout --quiet FETCH_HEAD

  (cd "${tmp_dir}/ssz-specs" && SSZ_PARANOID_ROOTS=1 uv run --locked --group test fill --clean >"${tmp_dir}/fill.log" 2>&1) ||
    { cat "${tmp_dir}/fill.log" >&2; exit 1; }

  printf '%s  %s\n' "${INDEX_SHA256}" "${tmp_dir}/ssz-specs/fixtures/index.json" | sha256sum --check --status ||
    { echo "generated vectors do not match the pinned digest" >&2; exit 1; }

  rm -rf "${DATA_DIR}"
  mkdir -p "${DATA_DIR}"
  cp -R "${tmp_dir}/ssz-specs/fixtures/." "${DATA_DIR}/"
  printf '%s\n' "${COMMIT}" > "${DATA_DIR}/.version"
  echo "Installed official SSZ vectors ${COMMIT} in ${DATA_DIR}"
}

# generate writes the Go types the vectors declare and their SSZ methods.
generate() {
  cd "${SCRIPT_DIR}"
  # Generated code from an earlier vector set would no longer compile against
  # the new types, and the package has to compile for both generators to run.
  rm -f gen_types.go gen_illegal.go gen_ssz.go gen_ssz.yaml
  go run ./gen -fixtures "${DATA_DIR}" -out .
  go run ../../dynssz-gen -config gen_ssz.yaml
}

case "${1:-setup}" in
  setup)
    fetch
    generate
    ;;
  export)
    printf '%s\n' "${DATA_DIR}"
    ;;
  version)
    printf '%s\n' "${COMMIT}"
    ;;
  clean)
    rm -rf "${DATA_DIR}"
    rm -f "${SCRIPT_DIR}"/gen_types.go "${SCRIPT_DIR}"/gen_illegal.go "${SCRIPT_DIR}"/gen_ssz.go "${SCRIPT_DIR}"/gen_ssz.yaml
    ;;
  *)
    echo "usage: $0 [setup|export|version|clean]" >&2
    exit 2
    ;;
esac
