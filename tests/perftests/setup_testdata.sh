#!/bin/bash
# Downloads test data payloads from the ssz-benchmark repository.
# Usage: ./setup_testdata.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RES_DIR="${SCRIPT_DIR}/res"

REPO="pk910/ssz-benchmark"
BRANCH="master"
BASE_URL="https://raw.githubusercontent.com/${REPO}/${BRANCH}/res"

FILES=(
    "block-mainnet.ssz"
    "block-mainnet-meta.json"
    "state-mainnet.ssz"
    "state-mainnet-meta.json"
    "block-minimal.ssz"
    "block-minimal-meta.json"
    "state-minimal.ssz"
    "state-minimal-meta.json"
)

mkdir -p "${RES_DIR}"

echo "Downloading test data from ${REPO}..."
for file in "${FILES[@]}"; do
    if [ -f "${RES_DIR}/${file}" ]; then
        echo "  ${file} (exists, skipping)"
        continue
    fi
    echo "  Downloading ${file}..."
    curl -fsSL -o "${RES_DIR}/${file}" "${BASE_URL}/${file}"
done

echo "Test data ready in ${RES_DIR}"
