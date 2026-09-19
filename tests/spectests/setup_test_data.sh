#!/bin/bash

# Setup script for downloading and extracting Ethereum consensus spec tests
# The vectors ship as release assets of ethereum/consensus-specs. The latest
# release is used, pre-releases included, because a new fork's vectors land in a
# beta long before the stable release.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
TEST_DATA_DIR="${SCRIPT_DIR}/consensus-spec-tests"

# Used when the release listing cannot be reached and nothing is on disk
FALLBACK_VERSION="v1.7.0-beta.0"

# Function to get the latest release (drafts excluded, pre-releases included)
get_latest_release() {
    local repo="ethereum/consensus-specs"
    local api_url="https://api.github.com/repos/${repo}/releases"

    echo "Fetching latest release information..." >&2

    local latest_tag=""

    if command -v jq >/dev/null 2>&1; then
        # Use jq for precise JSON parsing
        latest_tag=$(curl -s "${api_url}" | \
            jq -r '.[] | select(.draft == false) | .tag_name' | \
            head -1)
    else
        # Fallback method: the listing is ordered newest first, and an
        # unauthenticated listing carries no drafts, so the first tag wins
        latest_tag=$(curl -s "${api_url}" | \
            grep -o '"tag_name": *"[^"]*"' | \
            head -1 | \
            sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
    fi

    if [ -z "${latest_tag}" ]; then
        echo "Error: Could not fetch latest release tag" >&2
        echo "Available releases:" >&2
        curl -s "${api_url}" | grep '"tag_name":' | head -10 >&2
        return 0
    fi

    echo "Latest release: ${latest_tag}" >&2
    echo "${latest_tag}"
}

# Function to download and extract test data
download_and_extract() {
    local tag="$1"
    local preset="$2"
    local filename="${preset}.tar.gz"
    local download_url="https://github.com/ethereum/consensus-specs/releases/download/${tag}/${filename}"
    
    echo "Downloading ${filename} from ${download_url}..."
    
    # Create test data directory
    mkdir -p "${TEST_DATA_DIR}"
    cd "${TEST_DATA_DIR}"
    
    # Download the archive
    if ! curl -L -o "${filename}" "${download_url}"; then
        echo "Error: Failed to download ${filename}"
        exit 1
    fi
    
    echo "Extracting ${filename}..."
    
    # Extract the archive
    if ! tar -xzf "${filename}"; then
        echo "Error: Failed to extract ${filename}"
        exit 1
    fi
    
    # Remove the archive to save space
    rm "${filename}"
    
    echo "Test data extraction completed"
    
    # List the contents to verify
    echo "Available test directories:"
    ls -la
}

# Function to verify test data
verify_test_data() {
    local preset="$1"
    
    if [ ! -d "${TEST_DATA_DIR}/tests/${preset}" ]; then
        echo "Error: Test data directory ${TEST_DATA_DIR}/tests/${preset} not found" >&2
        return 1
    fi
    
    # Check if essential directories exist (at least one fork with ssz_static tests)
    local found_tests=0
    for fork in phase0 altair bellatrix capella deneb electra fulu gloas; do
        if [ -d "${TEST_DATA_DIR}/tests/${preset}/${fork}/ssz_static" ]; then
            found_tests=1
            break
        fi
    done
    
    if [ ${found_tests} -eq 0 ]; then
        echo "Error: No valid test data found in ${TEST_DATA_DIR}/tests/${preset}" >&2
        return 1
    fi
    
    echo "Test data verification completed"
    echo "Test data location: ${TEST_DATA_DIR}/tests/${preset}"
    return 0
}

# Main execution
main() {
    local preset="$1"
    
    echo "Setting up consensus spec tests for preset: ${preset}"
    
    # Validate preset
    if [[ "${preset}" != "mainnet" && "${preset}" != "minimal" ]]; then
        echo "Error: Invalid preset '${preset}'. Must be 'mainnet' or 'minimal'"
        exit 1
    fi
    
    local latest_tag=$(get_latest_release)
    local local_tag=""
    if [ -f "${TEST_DATA_DIR}/version.txt" ]; then
        local_tag=$(cat "${TEST_DATA_DIR}/version.txt")
    fi
    
    # An unreachable API leaves whatever is on disk in place rather than
    # replacing it with the fallback, which may well be older
    if [ -z "${latest_tag}" ]; then
        latest_tag="${local_tag:-${FALLBACK_VERSION}}"
        echo "Using ${latest_tag}"
    fi
    
    # Existing data from another release is discarded: the presets in presets/
    # and the fork type sets in the test files follow the release the vectors
    # come from, so a stale tree fails the suite rather than skipping it
    if [ -d "${TEST_DATA_DIR}/tests" ] && [ "${local_tag}" != "${latest_tag}" ]; then
        echo "Existing test data is ${local_tag:-of unknown version}, removing it"
        rm -rf "${TEST_DATA_DIR}/tests" "${TEST_DATA_DIR}/version.txt"
    fi
    
    # Check if test data already exists
    if [ -d "${TEST_DATA_DIR}/tests/${preset}" ]; then
        echo "Existing test data found, skipping download"
        echo "Spec tests version: ${latest_tag}"
        echo "CONSENSUS_SPEC_TESTS_DIR=${TEST_DATA_DIR}/tests/${preset}"
        return 0
    fi
    
    download_and_extract "${latest_tag}" "${preset}"
    verify_test_data "${preset}"
    
    # Save the version to a file for cache key usage
    echo "${latest_tag}" > "${TEST_DATA_DIR}/version.txt"
    
    echo "CONSENSUS_SPEC_TESTS_DIR=${TEST_DATA_DIR}/tests/${preset}"
}

# Export the test data directory for use by tests
export_test_dir() {
    local preset="$1"
    local test_dir="${TEST_DATA_DIR}/tests"
    if [ ! -z "${preset}" ]; then
        test_dir="${test_dir}/${preset}"
    fi
    echo "${test_dir}"
}

# Get the spec tests version
get_version() {
    if [ -f "${TEST_DATA_DIR}/version.txt" ]; then
        cat "${TEST_DATA_DIR}/version.txt"
    else
        echo "unknown"
    fi
}

# Handle command line arguments
COMMAND="${1:-setup}"
PRESET_ARG="${2:-mainnet}"

case "${COMMAND}" in
    "setup")
        main "${PRESET_ARG}"
        ;;
    "export")
        export_test_dir "${2}"
        ;;
    "version")
        get_version
        ;;
    "clean")
        echo "Cleaning test data..."
        rm -rf "${TEST_DATA_DIR}"
        echo "Test data cleaned"
        ;;
    *)
        echo "Usage: $0 [setup|export|version|clean] [mainnet|minimal]"
        echo "  setup: Download and setup test data (default)"
        echo "  export: Print the test data directory path"
        echo "  version: Print the spec tests version"
        echo "  clean: Remove all test data"
        echo ""
        echo "Examples:"
        echo "  $0 setup mainnet"
        echo "  $0 setup minimal" 
        echo "  $0 export mainnet"
        echo "  $0 version"
        echo "  $0 clean"
        exit 1
        ;;
esac