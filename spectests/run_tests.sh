#!/bin/bash

# Local test runner for consensus spec tests
# Usage: ./run_tests.sh [preset] [fork]
#   preset: mainnet or minimal (default: mainnet)
#   fork: specific fork to test (default: all)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &> /dev/null && pwd)"
PRESET="${1:-mainnet}"
FORK="${2:-}"

echo "Running consensus spec tests for preset: ${PRESET}"

# Setup test data
echo "Setting up test data..."
./setup_test_data.sh setup "${PRESET}"

# Export test directory
export CONSENSUS_SPEC_TESTS_DIR=$(./setup_test_data.sh export "")
echo "Using test data from: ${CONSENSUS_SPEC_TESTS_DIR}"

# Verify test data exists
if [ ! -d "${CONSENSUS_SPEC_TESTS_DIR}" ]; then
    echo "Error: Test data directory not found: ${CONSENSUS_SPEC_TESTS_DIR}"
    exit 1
fi

echo "Available test directories:"
ls -la "${CONSENSUS_SPEC_TESTS_DIR}"

# Download dependencies
echo "Downloading Go dependencies..."
go mod download

# Run tests
echo "Running tests..."
if [ -n "${FORK}" ]; then
    echo "Running tests for fork: ${FORK}"
    # Convert fork name to match test function naming (capitalize first letter)
    FORK_CAPITALIZED="$(echo ${FORK} | sed 's/^./\U&/')"
    go test -v -timeout=30m -coverprofile=spec-coverage.out -coverpkg=github.com/pk910/dynamic-ssz/... -run="TestConsensusSpec${FORK_CAPITALIZED}" ./...
else
    echo "Running all tests for preset: ${PRESET}"
    go test -v -timeout=30m -coverprofile=spec-coverage.out -coverpkg=github.com/pk910/dynamic-ssz/... -run="TestConsensusSpec" ./...
fi

echo "Tests completed!"