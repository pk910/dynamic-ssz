#!/usr/bin/env bash
set -euo pipefail

readonly VERSION="v0.1.0"
readonly SHA256="e2a65f032b59835c26127295293ea1bc07d7ca0ea1fe0e4f1128dffed333f878"
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly DATA_DIR="${SCRIPT_DIR}/fixtures"
readonly ARCHIVE="ssz-test-vectors-${VERSION}.tar.gz"
readonly URL="https://github.com/ethereum/ssz-specs/releases/download/${VERSION}/${ARCHIVE}"

case "${1:-setup}" in
  setup)
    if [[ -f "${DATA_DIR}/.version" ]] && [[ "$(<"${DATA_DIR}/.version")" == "${VERSION}" ]]; then
      echo "Official SSZ vectors ${VERSION} already installed"
      exit 0
    fi

    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "${tmp_dir}"' EXIT
    curl --fail --location --silent --show-error "${URL}" --output "${tmp_dir}/${ARCHIVE}"
    printf '%s  %s\n' "${SHA256}" "${tmp_dir}/${ARCHIVE}" | sha256sum --check --status

    rm -rf "${DATA_DIR}"
    mkdir -p "${DATA_DIR}"
    tar -xzf "${tmp_dir}/${ARCHIVE}" -C "${DATA_DIR}" --strip-components=3 fixtures/ssz/ssz
    printf '%s\n' "${VERSION}" > "${DATA_DIR}/.version"
    echo "Installed official SSZ vectors ${VERSION} in ${DATA_DIR}"
    ;;
  export)
    printf '%s\n' "${DATA_DIR}"
    ;;
  version)
    printf '%s\n' "${VERSION}"
    ;;
  clean)
    rm -rf "${DATA_DIR}"
    ;;
  *)
    echo "usage: $0 [setup|export|version|clean]" >&2
    exit 2
    ;;
esac
